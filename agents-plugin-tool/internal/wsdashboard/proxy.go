package wsdashboard

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// proxy.go is the daemon's MCP hosting route. The launcher that a harness
// starts as its MCP server does not run ws-mcp itself: it connects here,
// hands over its process context in one hello line, and copies bytes
// between the harness's stdio and the stream, so one daemon executable
// serves every session on the machine and an endpoint protector sees one
// executable run per version. The route is a token-authenticated HTTP
// Upgrade on the daemon's port: after the 101 the connection is a raw byte
// stream with no further framing, newline-delimited JSON-RPC exactly as
// stdio carries it, in both directions. Only a loopback peer may connect,
// in LAN mode too: the launcher is on this machine by construction, and
// the stream carries the caller's whole environment.

// ProxyPath is the hosting route; ProxyUpgrade is its Upgrade token.
const (
	ProxyPath    = "/mcp"
	ProxyUpgrade = "ws-mcp"
	// ProxyProtocol is the hello line's protocol version. A hello with
	// another version is refused, and the launcher falls back to running
	// ws-mcp directly.
	ProxyProtocol = 1
	// ProxyModeMCP serves an MCP session; ProxyModeCLI runs a one-shot CLI
	// command (mailbox wait) and answers with its output and exit code.
	ProxyModeMCP = "mcp"
	ProxyModeCLI = "cli"
	// proxyHelloTimeout bounds the wait for the hello line after the 101.
	proxyHelloTimeout = 10 * time.Second
	// proxyDrainBudget bounds how long Close lets calls in flight finish
	// before it releases the lock and endpoint: strictly less than
	// DefaultLockWait - 1s, so the successor daemon that waits for the lock
	// always gets it. A call still running then keeps its stream until it
	// ends (WaitProxy); it is never cut.
	proxyDrainBudget = DefaultLockWait - 1800*time.Millisecond
	// proxyCloseTail bounds the wait for the session goroutines after their
	// streams are closed.
	proxyCloseTail = 300 * time.Millisecond
)

// ProxyHello is the first line the launcher sends after the 101: the
// caller process's context, from which the daemon builds the session.
type ProxyHello struct {
	Protocol int    `json:"protocol"`
	Mode     string `json:"mode"`
	// Argv is the CLI command in ProxyModeCLI ("mailbox", "wait", ...).
	Argv []string `json:"argv,omitempty"`
	// Cwd is the launcher's working directory; ProjectRoot the project root
	// it detected, when any.
	Cwd         string `json:"cwd"`
	ProjectRoot string `json:"project_root,omitempty"`
	// Env is the launcher's whole environment, the one the session's
	// subprocesses run under.
	Env map[string]string `json:"env"`
	// Harness is the harness kind the launcher detected, when any.
	Harness string `json:"harness,omitempty"`
	// OwnerPID is the launcher's PID: the owner of the session's mailbox
	// presence and listening markers.
	OwnerPID int `json:"owner_pid"`
	// Version is the launcher's plugin version, when known; the daemon
	// refuses a hello from another minor version.
	Version string `json:"version,omitempty"`
}

// ProxyAccept is the daemon's one-line answer to the hello: accepted, and
// the raw stream follows, or refused with a reason, and the daemon closes.
type ProxyAccept struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason,omitempty"`
	Version string `json:"version,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// ProxyCLIFrame is one line the daemon writes on a ProxyModeCLI stream:
// output for one of the caller's streams, or, as the last line, the
// command's exit code with its whole stdout.
type ProxyCLIFrame struct {
	// Stream is "stdout" or "stderr" for an output frame; empty for the
	// final frame.
	Stream string `json:"stream,omitempty"`
	Data   string `json:"data,omitempty"`
	// ExitCode is meaningful on the final frame only. It is never omitted:
	// a success is code 0, and a launcher that saw no code would have to
	// guess (and guessed 1, which made a delivered mail look like an error).
	Final    bool `json:"final,omitempty"`
	ExitCode int  `json:"exit_code"`
}

// ProxyHandler builds the session for an accepted hello. The daemon cannot
// build an MCP server itself (the server package depends on this one), so
// the command that runs the daemon supplies it.
type ProxyHandler interface {
	// Accept validates hello and returns its session, or the reason the
	// daemon refuses it; the reason goes to the launcher, which then runs
	// ws-mcp directly.
	Accept(hello ProxyHello) (ProxySession, error)
}

// ProxySession is one hosted session over one stream.
type ProxySession interface {
	// Serve runs the session on stream until the stream ends. The daemon
	// closes the stream when Serve returns.
	Serve(ctx context.Context, stream io.ReadWriteCloser)
	// Drain tells the session the daemon stops: it cancels long waits at
	// once, lets calls in flight finish, and returns when idle (true) or
	// when ctx ends first (false). The daemon closes the stream once the
	// session is idle. Drain may be called again on a session that was not
	// idle, to wait for it without a deadline.
	Drain(ctx context.Context) bool
}

// proxyConn is one accepted stream and its session.
type proxyConn struct {
	stream  io.ReadWriteCloser
	session ProxySession
}

// proxyStream is the hijacked connection: reads go through the buffered
// reader that may already hold bytes past the hello line, writes straight
// to the connection.
type proxyStream struct {
	r    *bufio.Reader
	conn net.Conn
}

func (p *proxyStream) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *proxyStream) Write(b []byte) (int, error) { return p.conn.Write(b) }
func (p *proxyStream) Close() error                { return p.conn.Close() }

// handleProxy is the hosting route.
func (d *Daemon) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !loopbackPeer(r.RemoteAddr) {
		http.Error(w, "the MCP route serves this machine only", http.StatusForbidden)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "the MCP route serves no browser", http.StatusForbidden)
		return
	}
	got, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(d.endpoint.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), ProxyUpgrade) || !headerHasToken(r.Header, "Connection", "upgrade") {
		w.Header().Set("Upgrade", ProxyUpgrade)
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	if d.proxy == nil {
		http.Error(w, "this daemon hosts no MCP session", http.StatusServiceUnavailable)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "upgrade unsupported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	d.serveProxyConn(conn, rw.Reader)
}

// serveProxyConn runs the stream after the hijack: the 101, the hello, the
// accept line, then the session.
func (d *Daemon) serveProxyConn(conn net.Conn, reader *bufio.Reader) {
	if _, err := io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: "+ProxyUpgrade+"\r\nConnection: Upgrade\r\n\r\n"); err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(proxyHelloTimeout))
	line, err := reader.ReadBytes('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil && len(line) == 0 {
		_ = conn.Close()
		return
	}
	var hello ProxyHello
	if err := json.Unmarshal(line, &hello); err != nil {
		writeProxyAccept(conn, ProxyAccept{Reason: "malformed hello: " + err.Error()})
		_ = conn.Close()
		return
	}
	session, err := d.acceptProxy(hello)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: MCP session refused (pid %d, mode %s): %v\n", hello.OwnerPID, hello.Mode, err)
		writeProxyAccept(conn, ProxyAccept{Reason: err.Error()})
		_ = conn.Close()
		return
	}
	stream := &proxyStream{r: reader, conn: conn}
	pc := &proxyConn{stream: stream, session: session}
	if !d.addProxyConn(pc) {
		writeProxyAccept(conn, ProxyAccept{Reason: "daemon is shutting down"})
		_ = conn.Close()
		return
	}
	defer d.removeProxyConn(pc)
	if err := writeProxyAccept(conn, ProxyAccept{OK: true, Version: d.endpoint.Version, PID: d.endpoint.PID}); err != nil {
		_ = conn.Close()
		return
	}
	fmt.Fprintf(d.log, "dashboard: MCP session opened (pid %d, mode %s, root %s)\n", hello.OwnerPID, hello.Mode, hello.ProjectRoot)
	session.Serve(context.Background(), stream)
	_ = conn.Close()
	fmt.Fprintf(d.log, "dashboard: MCP session closed (pid %d, mode %s)\n", hello.OwnerPID, hello.Mode)
}

// acceptProxy validates the hello's shape, then asks the handler.
func (d *Daemon) acceptProxy(hello ProxyHello) (ProxySession, error) {
	if hello.Protocol != ProxyProtocol {
		return nil, fmt.Errorf("unsupported hello protocol %d (this daemon speaks %d)", hello.Protocol, ProxyProtocol)
	}
	if hello.Mode != ProxyModeMCP && hello.Mode != ProxyModeCLI {
		return nil, fmt.Errorf("unsupported mode %q", hello.Mode)
	}
	if hello.Cwd == "" {
		return nil, errors.New("hello carries no cwd")
	}
	if hello.Env == nil {
		return nil, errors.New("hello carries no environment")
	}
	if hello.OwnerPID <= 0 {
		return nil, errors.New("hello carries no owner pid")
	}
	if hello.Mode == ProxyModeCLI && len(hello.Argv) == 0 {
		return nil, errors.New("CLI mode carries no argv")
	}
	if hello.Version != "" && !SameMinor(hello.Version, d.endpoint.Version) {
		return nil, fmt.Errorf("launcher version %s is not this daemon's minor (%s)", hello.Version, d.endpoint.Version)
	}
	return d.proxy.Accept(hello)
}

func writeProxyAccept(w io.Writer, accept ProxyAccept) error {
	raw, err := json.Marshal(accept)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

// headerHasToken reports whether a comma-separated header carries token.
func headerHasToken(h http.Header, name, token string) bool {
	for _, value := range h.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func (d *Daemon) addProxyConn(pc *proxyConn) bool {
	d.proxyMu.Lock()
	defer d.proxyMu.Unlock()
	if d.proxyDraining {
		return false
	}
	if d.proxyConns == nil {
		d.proxyConns = map[*proxyConn]struct{}{}
	}
	d.proxyConns[pc] = struct{}{}
	d.proxyWG.Add(1)
	return true
}

func (d *Daemon) removeProxyConn(pc *proxyConn) {
	d.proxyMu.Lock()
	delete(d.proxyConns, pc)
	d.proxyMu.Unlock()
	d.proxyWG.Done()
}

// ProxySessions counts the hosted sessions open now.
func (d *Daemon) ProxySessions() int {
	d.proxyMu.Lock()
	defer d.proxyMu.Unlock()
	return len(d.proxyConns)
}

// drainProxy is Close's hosted-session step: no new session is admitted,
// and every session drains (long waits cancelled at once, calls in flight
// finished). A session idle within proxyDrainBudget has its stream closed;
// the launcher sees the close and reconnects to the successor. A session
// with a call still running when the budget ends keeps its stream: Close
// goes on to release the lock and endpoint (the successor must not wait),
// and the stream closes when that call has ended, so a git commit, merge,
// or tickets.close is never cut midway. The process that runs the daemon
// waits for those sessions (WaitProxy) before it exits.
func (d *Daemon) drainProxy() {
	d.proxyMu.Lock()
	d.proxyDraining = true
	conns := make([]*proxyConn, 0, len(d.proxyConns))
	for pc := range d.proxyConns {
		conns = append(conns, pc)
	}
	d.proxyMu.Unlock()
	if len(conns) == 0 {
		return
	}
	fmt.Fprintf(d.log, "dashboard: draining %d MCP session(s)\n", len(conns))
	ctx, cancel := context.WithTimeout(context.Background(), proxyDrainBudget)
	defer cancel()
	var wg sync.WaitGroup
	for _, pc := range conns {
		wg.Add(1)
		go func(pc *proxyConn) {
			defer wg.Done()
			if pc.session.Drain(ctx) {
				_ = pc.stream.Close()
				return
			}
			fmt.Fprintf(d.log, "dashboard: a hosted call outlives the drain budget; its session closes when it ends\n")
			go func() {
				pc.session.Drain(context.Background())
				_ = pc.stream.Close()
			}()
		}(pc)
	}
	wg.Wait()
	done := make(chan struct{})
	go func() {
		d.proxyWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(proxyCloseTail):
	}
}

// WaitProxy waits for every hosted session to end, or for ctx; it reports
// whether all ended. After Close (Done), the sessions left are the ones
// whose call outlived the drain budget; the daemon's process waits for them
// before it exits.
func (d *Daemon) WaitProxy(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		d.proxyWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}
