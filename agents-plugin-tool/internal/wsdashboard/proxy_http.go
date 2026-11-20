package wsdashboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// proxy_http.go is the daemon's second MCP hosting route: MCP Streamable
// HTTP with JSON responses (no SSE stream), for a hosted harness session the
// agent runner (AgentMCPTokenHost) started with a per-session token. The
// harness talks to the daemon directly, with no launcher between them. The
// token is the session's identity: it resolves, on every request, to the
// hello a stdio launcher would have sent, and each token has one session,
// built exactly as the raw-stream route builds one (acceptProxy) and served
// over an in-memory newline-delimited stream, so the session cannot tell the
// two routes apart. Each POST carries one JSON-RPC message; a request is
// answered in the POST's response body once the session writes the answer
// with the same id. Only a loopback peer may connect, in LAN mode too, and
// never a browser.

// ProxyHTTPPath is the HTTP hosting route.
const ProxyHTTPPath = "/mcp/session"

const (
	// httpMCPBodyLimit keeps a POST, with the newline the stream adds, within
	// the session's line scanner (bufio.Scanner's default token cap): a longer
	// line would end the session, not just fail its request.
	httpMCPBodyLimit = bufio.MaxScanTokenSize - 1
	// httpMCPReplayProtocol is the protocol version of a replayed initialize.
	httpMCPReplayProtocol = "2025-06-18"
)

// httpMCPSession is one token's session: the stream its Serve reads request
// lines from and writes answer lines to, and the requests awaiting an answer.
type httpMCPSession struct {
	d     *Daemon
	token string
	hello ProxyHello
	pc    *proxyConn
	// inR/inW carry request lines into the session; outR/outW carry its
	// answers out to the reader goroutine.
	inR  *io.PipeReader
	inW  *io.PipeWriter
	outR *io.PipeReader
	outW *io.PipeWriter
	// writeMu keeps each line whole and the replayed initialize first.
	writeMu sync.Mutex
	mu      sync.Mutex
	// pending maps a compacted request id to its waiter; a nil channel
	// drops the answer (a replayed initialize, or a request whose HTTP
	// client went away).
	pending map[string]chan []byte
	// ended closes once Serve returned and every answer was routed.
	ended    chan struct{}
	isEnded  bool
	readDone chan struct{}
}

// httpMCPStream is the session's side of its pipes. Close ends only the
// input, so handlers still running can write their answers.
type httpMCPStream struct{ s *httpMCPSession }

func (st httpMCPStream) Read(b []byte) (int, error)  { return st.s.inR.Read(b) }
func (st httpMCPStream) Write(b []byte) (int, error) { return st.s.outW.Write(b) }
func (st httpMCPStream) Close() error                { return st.s.inW.Close() }

// handleProxyHTTP is the HTTP hosting route.
func (d *Daemon) handleProxyHTTP(w http.ResponseWriter, r *http.Request) {
	if !loopbackPeer(r.RemoteAddr) {
		http.Error(w, "the MCP route serves this machine only", http.StatusForbidden)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "the MCP route serves no browser", http.StatusForbidden)
		return
	}
	tokens, _ := d.agents.(AgentMCPTokenHost)
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tokens == nil || !bearer || token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Resolved on every request: a voided token stops at once, one voided
	// right after it resolved too.
	hello, void, ok := tokens.ResolveMCPToken(token)
	if ok {
		select {
		case <-void:
			ok = false
		default:
		}
	}
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		// A client asks GET for a server stream and DELETE at exit; this
		// route has neither.
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, httpMCPBodyLimit+1))
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "parse error")
		return
	}
	if len(body) > httpMCPBodyLimit {
		writeRPCError(w, http.StatusRequestEntityTooLarge, nil, -32600, "message exceeds "+strconv.Itoa(httpMCPBodyLimit)+" bytes")
		return
	}
	if !json.Valid(body) {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "parse error")
		return
	}
	trimmed := bytes.TrimSpace(body)
	if trimmed[0] == '[' {
		writeRPCError(w, http.StatusBadRequest, nil, -32600, "batches are not supported")
		return
	}
	if trimmed[0] != '{' {
		writeRPCError(w, http.StatusBadRequest, nil, -32600, "invalid request")
		return
	}
	var line bytes.Buffer
	if err := json.Compact(&line, trimmed); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "parse error")
		return
	}
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method *string         `json:"method"`
	}
	if err := json.Unmarshal(line.Bytes(), &msg); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32600, "invalid request")
		return
	}
	var id json.RawMessage
	if msg.Method != nil && len(msg.ID) > 0 {
		id = msg.ID
	}
	initialize := msg.Method != nil && *msg.Method == "initialize"
	s, status, err := d.httpSession(token, hello, void, !initialize)
	if err != nil {
		writeRPCError(w, status, id, -32000, err.Error())
		return
	}
	if id == nil {
		// A notification, or the client's answer to a server request.
		if err := s.writeLine(line.Bytes()); err != nil {
			writeRPCError(w, http.StatusServiceUnavailable, nil, -32000, "ws-mcp session ended")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	key := compactID(id)
	ch, ended := s.await(key)
	if ended {
		writeRPCError(w, http.StatusOK, id, -32000, "ws-mcp session ended")
		return
	}
	if ch == nil {
		writeRPCError(w, http.StatusOK, id, -32600, "request id already in flight")
		return
	}
	if err := s.writeLine(line.Bytes()); err != nil {
		s.abandon(key, ch)
		writeRPCError(w, http.StatusOK, id, -32000, "ws-mcp session ended")
		return
	}
	select {
	case resp := <-ch:
		writeRPCBody(w, http.StatusOK, resp)
	case <-r.Context().Done():
		if s.abandon(key, ch) {
			_ = s.writeLine(cancelledLine(id, "HTTP request closed"))
		}
	case <-s.ended:
		select {
		case resp := <-ch:
			writeRPCBody(w, http.StatusOK, resp)
		default:
			writeRPCError(w, http.StatusOK, id, -32000, "ws-mcp session ended")
		}
	}
}

// httpSession returns token's session, building and starting it on the
// first request. replay writes a synthetic initialize first: the client
// initialized against an earlier daemon (or an earlier session of this
// one) and this one has no session for its token. A refusal carries the
// HTTP status to answer with.
func (d *Daemon) httpSession(token string, hello ProxyHello, void <-chan struct{}, replay bool) (*httpMCPSession, int, error) {
	d.httpMu.Lock()
	if s := d.httpSessions[token]; s != nil {
		d.httpMu.Unlock()
		return s, 0, nil
	}
	session, err := d.acceptProxy(hello)
	if err == nil && hello.Mode != ProxyModeMCP {
		err = fmt.Errorf("unsupported mode %q for an HTTP session", hello.Mode)
	}
	if err != nil {
		d.httpMu.Unlock()
		fmt.Fprintf(d.log, "dashboard: MCP HTTP session refused (pid %d): %v\n", hello.OwnerPID, err)
		return nil, http.StatusForbidden, fmt.Errorf("session refused: %v", err)
	}
	s := &httpMCPSession{d: d, token: token, hello: hello, pending: map[string]chan []byte{}, ended: make(chan struct{}), readDone: make(chan struct{})}
	var replayID []byte
	if replay {
		// Its answer is dropped. Registered while nothing else can reach
		// the session: before it is published to this token's other
		// requests, and before it runs (it may end at once and take its
		// pending map with it).
		replayID, _ = json.Marshal("ws-mcp-http-replay-" + strconv.FormatUint(d.httpReplay.Add(1), 10))
		s.pending[compactID(replayID)] = nil
	}
	s.inR, s.inW = io.Pipe()
	s.outR, s.outW = io.Pipe()
	s.pc = &proxyConn{stream: httpMCPStream{s}, session: session}
	if !d.addProxyConn(s.pc) {
		d.httpMu.Unlock()
		return nil, http.StatusServiceUnavailable, fmt.Errorf("daemon is shutting down")
	}
	// Held until the replay is written, so no other request of this token
	// reaches the session before it.
	s.writeMu.Lock()
	if d.httpSessions == nil {
		d.httpSessions = map[string]*httpMCPSession{}
	}
	d.httpSessions[token] = s
	d.httpMu.Unlock()
	fmt.Fprintf(d.log, "dashboard: MCP HTTP session opened (pid %d, root %s)\n", hello.OwnerPID, hello.ProjectRoot)
	go s.readAnswers()
	go s.serve()
	go s.watchVoid(void)
	if replay {
		initialize, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(replayID),
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": httpMCPReplayProtocol,
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": replayClientName(hello.Harness), "version": "replay"},
			},
		})
		if s.writeLineLocked(initialize) == nil {
			_ = s.writeLineLocked([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
		}
	}
	s.writeMu.Unlock()
	return s, 0, nil
}

// replayClientName is a clientInfo name from which the MCP server detects
// harness (internal/mcp detectHarnessFromInitializeParams).
func replayClientName(harness string) string {
	switch harness {
	case "pi":
		return "ws-pi-bridge"
	case "claude":
		return "claude-code"
	case "codex":
		return "codex"
	case "agy":
		return "agy"
	}
	return "ws-mcp-http-replay"
}

// serve runs the session until its input ends, then retires it: the
// answers still in the output pipe are routed first, and every request
// still waiting then gets the session's end.
func (s *httpMCPSession) serve() {
	s.pc.session.Serve(context.Background(), s.pc.stream)
	_ = s.inW.Close()
	_ = s.inR.Close()
	_ = s.outW.Close()
	<-s.readDone
	s.d.dropHTTPSession(s)
	s.mu.Lock()
	s.isEnded = true
	s.pending = nil
	close(s.ended)
	s.mu.Unlock()
	s.d.removeProxyConn(s.pc)
	fmt.Fprintf(s.d.log, "dashboard: MCP HTTP session closed (pid %d)\n", s.hello.OwnerPID)
}

// watchVoid ends the session once its token is void. A void token means its
// client is gone, so every request still in flight is cancelled first: the
// client's disconnect may race the void, and its own cancellation would then
// find the input closed and leave a long wait running to its timeout.
func (s *httpMCPSession) watchVoid(void <-chan struct{}) {
	select {
	case <-void:
		s.d.dropHTTPSession(s)
		s.mu.Lock()
		ids := make([]string, 0, len(s.pending))
		for key := range s.pending {
			ids = append(ids, key)
		}
		s.mu.Unlock()
		for _, key := range ids {
			_ = s.writeLine(cancelledLine(json.RawMessage(key), "MCP session token void"))
		}
		_ = s.pc.stream.Close()
	case <-s.ended:
	}
}

func cancelledLine(id json.RawMessage, reason string) []byte {
	line, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": id, "reason": reason},
	})
	return line
}

// readAnswers routes each answer line to the request with its id. It reads
// with no line cap: a tool result may be large.
func (s *httpMCPSession) readAnswers() {
	defer close(s.readDone)
	r := bufio.NewReader(s.outR)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			s.route(line)
		}
		if err != nil {
			return
		}
	}
}

func (s *httpMCPSession) route(line []byte) {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(line, &msg); err != nil || len(msg.ID) == 0 {
		fmt.Fprintf(s.d.log, "dashboard: MCP HTTP session (pid %d) dropped a line with no id\n", s.hello.OwnerPID)
		return
	}
	key := compactID(msg.ID)
	s.mu.Lock()
	ch, ok := s.pending[key]
	if ok {
		delete(s.pending, key)
	}
	s.mu.Unlock()
	switch {
	case !ok:
		fmt.Fprintf(s.d.log, "dashboard: MCP HTTP session (pid %d) dropped an answer for unknown id %s\n", s.hello.OwnerPID, key)
	case ch != nil:
		ch <- line
	}
}

// await registers a waiter for key: nil when key is already in flight;
// ended when the session already ended.
func (s *httpMCPSession) await(key string) (ch chan []byte, ended bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isEnded {
		return nil, true
	}
	if _, busy := s.pending[key]; busy {
		return nil, false
	}
	ch = make(chan []byte, 1)
	s.pending[key] = ch
	return ch, false
}

// abandon turns key's waiter ch into one that drops its answer; false when
// the answer was already routed (or the session ended).
func (s *httpMCPSession) abandon(key string, ch chan []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isEnded || s.pending[key] != ch {
		return false
	}
	s.pending[key] = nil
	return true
}

func (s *httpMCPSession) writeLine(line []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writeLineLocked(line)
}

func (s *httpMCPSession) writeLineLocked(line []byte) error {
	buf := make([]byte, 0, len(line)+1)
	_, err := s.inW.Write(append(append(buf, line...), '\n'))
	return err
}

// dropHTTPSession forgets s, unless its token already has a newer session.
func (d *Daemon) dropHTTPSession(s *httpMCPSession) {
	d.httpMu.Lock()
	if d.httpSessions[s.token] == s {
		delete(d.httpSessions, s.token)
	}
	d.httpMu.Unlock()
}

func compactID(id json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, id) != nil {
		return string(id)
	}
	return b.String()
}

func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, message string) {
	if id == nil {
		id = json.RawMessage("null")
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
	writeRPCBody(w, status, body)
}

func writeRPCBody(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
