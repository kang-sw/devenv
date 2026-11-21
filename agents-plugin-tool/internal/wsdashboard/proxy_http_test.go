package wsdashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcpframe"
)

// tokenAgents is a test AgentHost that issues per-session MCP tokens.
type tokenAgents struct {
	mu     sync.Mutex
	tokens map[string]tokenGrant
}

type tokenGrant struct {
	hello ProxyHello
	void  chan struct{}
}

func (a *tokenAgents) Handler() http.Handler               { return http.NotFoundHandler() }
func (a *tokenAgents) Start(func(replyID, project string)) {}
func (a *tokenAgents) InboxChanged(string)                 {}
func (a *tokenAgents) Close()                              {}
func (a *tokenAgents) issue(token string, hello ProxyHello) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tokens == nil {
		a.tokens = map[string]tokenGrant{}
	}
	a.tokens[token] = tokenGrant{hello: hello, void: make(chan struct{})}
}

// voidToken closes token's void channel and keeps resolving it, as a
// resolution that raced the void would: the route must refuse it itself.
func (a *tokenAgents) voidToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if g, ok := a.tokens[token]; ok {
		select {
		case <-g.void:
		default:
			close(g.void)
		}
	}
}

func (a *tokenAgents) ResolveMCPToken(token string) (ProxyHello, <-chan struct{}, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	g, ok := a.tokens[token]
	return g.hello, g.void, ok
}

// rpcHandler is a test ProxyHandler whose sessions answer JSON-RPC: a
// request is answered with {"method": <its method>}; "hold" waits until a
// "release" request, a notifications/cancelled for its id, or the drain,
// and says which in "how".
type rpcHandler struct {
	mu       sync.Mutex
	sessions []*rpcSession
}

func (h *rpcHandler) Accept(hello ProxyHello) (ProxySession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if reason := hello.Env["REFUSE"]; reason != "" {
		return nil, errors.New(reason)
	}
	s := &rpcSession{release: make(chan struct{}), drained: make(chan struct{}), served: make(chan struct{}), holds: map[string]chan struct{}{}}
	h.sessions = append(h.sessions, s)
	return s, nil
}

func (h *rpcHandler) session(t *testing.T, i int) *rpcSession {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		if i < len(h.sessions) {
			s := h.sessions[i]
			h.mu.Unlock()
			return s
		}
		n := len(h.sessions)
		h.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("session %d never accepted (%d sessions)", i, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *rpcHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sessions)
}

type rpcSession struct {
	mu          sync.Mutex
	got         []string
	holds       map[string]chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	drained     chan struct{}
	drainOnce   sync.Once
	served      chan struct{}
	inflight    atomic.Int32
}

func (s *rpcSession) Serve(ctx context.Context, stream io.ReadWriteCloser) {
	defer close(s.served)
	var wmu sync.Mutex
	var wg sync.WaitGroup
	write := func(v any) {
		raw, _ := json.Marshal(v)
		wmu.Lock()
		defer wmu.Unlock()
		_, _ = stream.Write(append(raw, '\n'))
	}
	r := bufio.NewReader(stream)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			s.mu.Lock()
			s.got = append(s.got, strings.TrimSuffix(string(line), "\n"))
			s.mu.Unlock()
			var m struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params struct {
					RequestID json.RawMessage `json:"requestId"`
				} `json:"params"`
			}
			_ = json.Unmarshal(line, &m)
			switch {
			case m.Method == "notifications/cancelled":
				s.mu.Lock()
				if c, ok := s.holds[string(m.Params.RequestID)]; ok {
					close(c)
					delete(s.holds, string(m.Params.RequestID))
				}
				s.mu.Unlock()
			case len(m.ID) > 0 && m.Method != "":
				s.inflight.Add(1)
				wg.Add(1)
				cancelled := make(chan struct{})
				if m.Method == "hold" {
					s.mu.Lock()
					s.holds[string(m.ID)] = cancelled
					s.mu.Unlock()
				}
				go func(id json.RawMessage, method string) {
					defer wg.Done()
					defer s.inflight.Add(-1)
					result := map[string]any{"method": method}
					switch method {
					case "hold":
						select {
						case <-s.release:
							result["how"] = "released"
						case <-cancelled:
							result["how"] = "cancelled"
						case <-s.drained:
							result["how"] = "drained"
						}
					case "release":
						s.releaseOnce.Do(func() { close(s.release) })
					}
					write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
				}(m.ID, m.Method)
			}
		}
		if err != nil {
			break
		}
	}
	wg.Wait()
}

func (s *rpcSession) Drain(ctx context.Context) bool {
	s.drainOnce.Do(func() { close(s.drained) })
	for s.inflight.Load() > 0 {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(5 * time.Millisecond):
		}
	}
	return true
}

func (s *rpcSession) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

// waitLines waits until the session saw at least n lines.
func (s *rpcSession) waitLines(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := s.lines()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("session saw %d lines, want %d: %q", len(got), n, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *rpcSession) waitServed(t *testing.T) {
	t.Helper()
	select {
	case <-s.served:
	case <-time.After(3 * time.Second):
		t.Fatal("the session's Serve did not return")
	}
}

func startHTTPProxyDaemon(t *testing.T) (*Daemon, *rpcHandler, *tokenAgents) {
	t.Helper()
	h, a := &rpcHandler{}, &tokenAgents{}
	d, err := StartDaemon(DaemonOptions{Dir: t.TempDir(), Port: 0, Version: "1.2.3", LockWait: 200 * time.Millisecond, Proxy: h, Agents: a})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d, h, a
}

func httpTestHello(harness string) ProxyHello {
	h := testHello(map[string]string{"WS_RSRC_ROOT": "/plugin/rsrc", "WS_MCP_ALLOWED_TOOLS": "mailbox.send"})
	h.Harness = harness
	return h
}

// mcpPost posts body to the HTTP route with a machine client that holds no
// browser cookie, and returns the status, the body, and the response.
func mcpPost(t *testing.T, ctx context.Context, d *Daemon, method, token, body string, headers ...string) (int, string, *http.Response) {
	t.Helper()
	url := "http://127.0.0.1:" + strconv.Itoa(d.Endpoint().Port) + ProxyHTTPPath
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error(), nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(raw)), resp
}

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result struct {
		Method string `json:"method"`
		How    string `json:"how"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeReply(t *testing.T, body string) rpcReply {
	t.Helper()
	var r rpcReply
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("reply %q: %v", body, err)
	}
	return r
}

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"claude-code","version":"2"}}}`

func TestProxyHTTPRouteRefusesAMissingUnknownOrVoidToken(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody); status != http.StatusOK {
		t.Fatalf("valid token: %d %s", status, body)
	}
	a.voidToken("tok-a")
	for name, token := range map[string]string{"missing": "", "unknown": "tok-x", "void": "tok-a"} {
		if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, token, initializeBody); status != http.StatusUnauthorized {
			t.Fatalf("%s token: %d %s, want 401", name, status, body)
		}
	}
	if h.count() != 1 {
		t.Fatalf("sessions accepted = %d, want 1", h.count())
	}
	// The route exists only with a token host.
	plain := startProxyTestDaemon(t, &echoHandler{})
	if status, _, _ := mcpPost(t, context.Background(), plain, http.MethodPost, "tok-a", initializeBody); status != http.StatusNotFound {
		t.Fatalf("without a token host: status %d, want 404", status)
	}
}

// TestProxyHTTPRouteRefusesAForeignHost: the route's access exemption
// holds for a loopback peer that addressed a loopback Host only; a
// rebound name gets the access guard's refusal and opens no session.
func TestProxyHTTPRouteRefusesAForeignHost(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	serve := func(host string) int {
		req := httptest.NewRequest(http.MethodPost, ProxyHTTPPath, strings.NewReader(initializeBody))
		req.Host = host
		req.RemoteAddr = "127.0.0.1:50000"
		req.Header.Set("Authorization", "Bearer tok-a")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		localhostTestHandler(t, d).ServeHTTP(rec, req)
		return rec.Code
	}
	if code := serve("evil.example"); code == http.StatusOK {
		t.Fatal("a rebound Host reached the MCP route")
	}
	if h.count() != 0 {
		t.Fatal("a foreign Host opened a session")
	}
	if code := serve("127.0.0.1:" + strconv.Itoa(d.Endpoint().Port)); code != http.StatusOK {
		t.Fatalf("a loopback Host: %d, want 200", code)
	}
}

func TestProxyHTTPRouteRefusesABrowserOriginAndANonLoopbackPeer(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody, "Origin", "http://127.0.0.1:"+strconv.Itoa(d.Endpoint().Port)); status != http.StatusForbidden {
		t.Fatalf("browser origin: %d %s, want 403", status, body)
	}
	req := httptest.NewRequest(http.MethodPost, ProxyHTTPPath, strings.NewReader(initializeBody))
	req.RemoteAddr = "192.168.1.20:5555"
	req.Header.Set("Authorization", "Bearer tok-a")
	rec := httptest.NewRecorder()
	d.handleProxyHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback peer: status %d, want 403", rec.Code)
	}
	if h.count() != 0 {
		t.Fatal("a refused request opened a session")
	}
}

func TestProxyHTTPRouteAnswersOnlyPost(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		status, _, resp := mcpPost(t, context.Background(), d, method, "tok-a", "")
		if status != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != http.MethodPost {
			t.Fatalf("%s: status %d, Allow %q; want 405 with Allow: POST", method, status, resp.Header.Get("Allow"))
		}
	}
	if h.count() != 0 {
		t.Fatal("a refused method opened a session")
	}
}

func TestProxyHTTPRoutesAnswersByIDAndForwardsNotifications(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	status, body, resp := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody)
	if status != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Mcp-Session-Id") != "" {
		t.Fatalf("initialize: %d %q %v", status, body, resp.Header)
	}
	if r := decodeReply(t, body); string(r.ID) != "1" || r.Result.Method != "initialize" {
		t.Fatalf("initialize reply = %s", body)
	}
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); status != http.StatusAccepted || body != "" {
		t.Fatalf("notification: %d %q, want 202 with no body", status, body)
	}
	s := h.session(t, 0)
	if got := s.waitLines(t, 2); got[1] != `{"jsonrpc":"2.0","method":"notifications/initialized"}` {
		t.Fatalf("session lines = %q", got)
	}
	// Two requests answered out of order: the held one is answered only
	// after the later one releases it.
	held := make(chan string, 1)
	go func() {
		_, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":"h","method":"hold"}`)
		held <- body
	}()
	s.waitLines(t, 3)
	select {
	case body := <-held:
		t.Fatalf("hold answered before its release: %s", body)
	case <-time.After(50 * time.Millisecond):
	}
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":2,"method":"release"}`)
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "2" || r.Result.Method != "release" {
		t.Fatalf("release reply = %d %s", status, body)
	}
	select {
	case body := <-held:
		if r := decodeReply(t, body); string(r.ID) != `"h"` || r.Result.How != "released" {
			t.Fatalf("hold reply = %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hold never answered")
	}
	if d.ProxySessions() != 1 || h.count() != 1 {
		t.Fatalf("sessions: daemon %d, handler %d; want one session per token", d.ProxySessions(), h.count())
	}
}

func TestProxyHTTPReissuedTokenEndsTheOldSession(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	hello := httpTestHello("claude")
	a.issue("tok-a", hello)
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody); status != http.StatusOK {
		t.Fatalf("A initialize: %d %s", status, body)
	}
	first := h.session(t, 0)
	// A call in flight when the token goes void is cancelled, so the
	// session ends at once instead of after the call's own timeout.
	held := make(chan string, 1)
	go func() {
		_, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":3,"method":"hold"}`)
		held <- body
	}()
	first.waitLines(t, 2)
	a.voidToken("tok-a")
	a.issue("tok-b", hello)
	first.waitServed(t)
	if r := decodeReply(t, <-held); string(r.ID) != "3" || r.Result.How != "cancelled" {
		t.Fatalf("held call at void = %+v", r)
	}
	if status, _, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":2,"method":"ping"}`); status != http.StatusUnauthorized {
		t.Fatalf("A after re-issue: status %d, want 401", status)
	}
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-b", initializeBody); status != http.StatusOK {
		t.Fatalf("B initialize: %d %s", status, body)
	}
	second := h.session(t, 1)
	if second == first {
		t.Fatal("B reused A's session")
	}
	deadline := time.Now().Add(2 * time.Second)
	for d.ProxySessions() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.ProxySessions() != 1 {
		t.Fatalf("sessions = %d, want B's alone", d.ProxySessions())
	}
}

func TestProxyHTTPReplaysInitializeForARestoredToken(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"mailbox.send"}}`)
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "7" || r.Result.Method != "tools/call" {
		t.Fatalf("tools/call reply = %d %s", status, body)
	}
	got := h.session(t, 0).waitLines(t, 3)
	var init struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name string `json:"name"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(got[0]), &init); err != nil || init.Method != "initialize" || !strings.HasPrefix(init.ID, "ws-mcp-http-replay-") || init.Params.ProtocolVersion != "2025-06-18" || init.Params.ClientInfo.Name != "claude-code" {
		t.Fatalf("first line = %s (%v), want a replayed initialize", got[0], err)
	}
	if got[1] != `{"jsonrpc":"2.0","method":"notifications/initialized"}` || !strings.Contains(got[2], `"method":"tools/call"`) {
		t.Fatalf("session lines = %q", got)
	}
	if replayClientName("pi") != "ws-pi-bridge" || replayClientName("codex") != "codex" || replayClientName("agy") != "agy" || replayClientName("") != "ws-mcp-http-replay" {
		t.Fatal("replay client names do not map to their harnesses")
	}
}

// TestProxyHTTPConcurrentFirstRequestsShareOneReplay: a restored token's
// first requests arriving together open one session with one replayed
// initialize, and each gets its own answer.
func TestProxyHTTPConcurrentFirstRequestsShareOneReplay(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	const n = 8
	replies := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replies[i], _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":`+strconv.Itoa(10+i)+`,"method":"ping"}`)
		}()
	}
	wg.Wait()
	for i, body := range replies {
		if r := decodeReply(t, body); string(r.ID) != strconv.Itoa(10+i) || r.Result.Method != "ping" {
			t.Fatalf("request %d answered %s", i, body)
		}
	}
	if h.count() != 1 {
		t.Fatalf("sessions = %d, want one", h.count())
	}
	initializes := 0
	for _, line := range h.session(t, 0).waitLines(t, n+2) {
		if strings.Contains(line, `"method":"initialize"`) {
			initializes++
		}
	}
	if initializes != 1 {
		t.Fatalf("replayed initializes = %d, want one", initializes)
	}
}

func TestProxyHTTPCancelsARequestWhoseClientLeft(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody); status != http.StatusOK {
		t.Fatalf("initialize: %d %s", status, body)
	}
	s := h.session(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		mcpPost(t, ctx, d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":9,"method":"hold"}`)
	}()
	s.waitLines(t, 2)
	cancel()
	<-done
	got := s.waitLines(t, 3)
	if got[2] != `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"reason":"HTTP request closed","requestId":9}}` {
		t.Fatalf("after the client left the session saw %q", got[2])
	}
	// The late answer is dropped; the id is free again for the session.
	deadline := time.Now().Add(2 * time.Second)
	for s.inflight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":10,"method":"ping"}`)
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "10" {
		t.Fatalf("next request = %d %s", status, body)
	}
}

func TestProxyHTTPBodyLimitsAndShapes(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody); status != http.StatusOK {
		t.Fatalf("initialize: %d %s", status, body)
	}
	// A representative tool call above the old 64 KiB scanner limit (and below
	// the shared 64 MiB bound) succeeds over the HTTP route.
	largeSVG := `<wb:topic name="diagram"><wb:body><svg>` + strings.Repeat(`<rect width="10" height="10"/>`, 3000) + `</svg></wb:body></wb:topic>`
	largeCall, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      11,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "whiteboard.update",
			"arguments": map[string]any{"commands": largeSVG},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(largeCall) <= 64<<10 {
		t.Fatalf("largeCall len = %d, want > 64 KiB", len(largeCall))
	}
	status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", string(largeCall))
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "11" || r.Result.Method != "tools/call" {
		t.Fatalf("large whiteboard.update: %d %.200s, want HTTP 200 answer", status, body)
	}
	// A body of exactly the shared bound reaches the session as one line; one
	// byte more gets HTTP 200 with a JSON-RPC -32600 error (recovering id when
	// parseable from the bounded prefix, else null) without ending the session.
	padded := func(n int) string {
		head, tail := `{"jsonrpc":"2.0","id":2,"method":"ping","params":{"pad":"`, `"}}`
		return head + strings.Repeat("x", n-len(head)-len(tail)) + tail
	}
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", padded(mcpframe.MaxRequestBytes))
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "2" || r.Result.Method != "ping" {
		t.Fatalf("at the limit: %d %.200s, want the session's answer", status, body)
	}
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", padded(mcpframe.MaxRequestBytes+1))
	if r := decodeReply(t, body); status != http.StatusOK || r.Error == nil || r.Error.Code != -32600 || r.Error.Message != mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes) || string(r.ID) != "2" {
		t.Fatalf("too large with recoverable id: %d %s, want HTTP 200 -32600 with id=2", status, body)
	}
	tailIDHead, tailIDTail := `{"jsonrpc":"2.0","method":"ping","params":{"pad":"`, `"},"id":22}`
	tailIDBody := tailIDHead + strings.Repeat("x", mcpframe.MaxRequestBytes+1-len(tailIDHead)-len(tailIDTail)) + tailIDTail
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", tailIDBody)
	if r := decodeReply(t, body); status != http.StatusOK || r.Error == nil || r.Error.Code != -32600 || r.Error.Message != mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes) || string(r.ID) != "null" {
		t.Fatalf("too large with unrecoverable id: %d %s, want HTTP 200 -32600 with id=null", status, body)
	}
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `[{"jsonrpc":"2.0","id":3,"method":"ping"}]`)
	if r := decodeReply(t, body); status != http.StatusBadRequest || r.Error == nil || r.Error.Code != -32600 || r.Error.Message != "batches are not supported" {
		t.Fatalf("batch: %d %s", status, body)
	}
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0",`)
	if r := decodeReply(t, body); status != http.StatusBadRequest || r.Error == nil || r.Error.Code != -32700 {
		t.Fatalf("parse error: %d %s", status, body)
	}
	// Newlines between tokens and an escaped one in a string arrive as one
	// compact line, proving the session stayed alive after the rejected bodies.
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", "{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 4,\n  \"method\": \"echo\",\n  \"params\": {\"text\": \"a\\nb\"}\n}\n")
	if r := decodeReply(t, body); status != http.StatusOK || string(r.ID) != "4" || r.Result.Method != "echo" {
		t.Fatalf("multi-line body: %d %s", status, body)
	}
	if h.count() != 1 {
		t.Fatalf("sessions = %d, want 1 (oversize errors never terminate the session)", h.count())
	}
	got := h.session(t, 0).lines()
	if last := got[len(got)-1]; last != `{"jsonrpc":"2.0","id":4,"method":"echo","params":{"text":"a\nb"}}` {
		t.Fatalf("session line = %q", last)
	}
	if len(got) != 4 {
		t.Fatalf("session saw %d lines, want 4 (refused bodies never forwarded)", len(got))
	}
	// A duplicate in-flight id is refused without reaching the session.
	held := make(chan string, 1)
	go func() {
		_, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":5,"method":"hold"}`)
		held <- body
	}()
	h.session(t, 0).waitLines(t, 5)
	status, body, _ = mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":5,"method":"ping"}`)
	if r := decodeReply(t, body); status != http.StatusOK || r.Error == nil || r.Error.Code != -32600 || r.Error.Message != "request id already in flight" {
		t.Fatalf("duplicate id: %d %s", status, body)
	}
	mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":6,"method":"release"}`)
	if r := decodeReply(t, <-held); r.Result.How != "released" {
		t.Fatalf("held request answer = %+v", r)
	}
	if n := len(h.session(t, 0).lines()); n != 6 {
		t.Fatalf("session saw %d lines, want 6 (the duplicate is never forwarded)", n)
	}
}

func TestProxyHTTPSessionDrainsWithTheDaemon(t *testing.T) {
	d, h, a := startHTTPProxyDaemon(t)
	a.issue("tok-a", httpTestHello("claude"))
	if status, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", initializeBody); status != http.StatusOK {
		t.Fatalf("initialize: %d %s", status, body)
	}
	s := h.session(t, 0)
	if d.ProxySessions() != 1 {
		t.Fatalf("sessions = %d, want the HTTP session counted", d.ProxySessions())
	}
	held := make(chan string, 1)
	go func() {
		_, body, _ := mcpPost(t, context.Background(), d, http.MethodPost, "tok-a", `{"jsonrpc":"2.0","id":2,"method":"hold"}`)
		held <- body
	}()
	s.waitLines(t, 2)
	d.Close()
	waitDone(t, d)
	// The drain cancels the long call; its answer reaches the client.
	select {
	case body := <-held:
		if r := decodeReply(t, body); string(r.ID) != "2" || r.Result.How != "drained" {
			t.Fatalf("held answer = %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the held call was never answered")
	}
	s.waitServed(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !d.WaitProxy(ctx) || d.ProxySessions() != 0 {
		t.Fatalf("sessions after Close = %d", d.ProxySessions())
	}
	d.httpMu.Lock()
	left := len(d.httpSessions)
	d.httpMu.Unlock()
	if left != 0 {
		t.Fatalf("%d HTTP sessions left in the map", left)
	}
	// A request after the drain began opens no session.
	if _, _, err := d.httpSession("tok-a", httpTestHello("claude"), make(chan struct{}), false); err == nil {
		t.Fatal("a session was admitted after the drain")
	}
}
