package wsdashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// echoHandler is a test ProxyHandler: it refuses a hello whose env carries
// REFUSE, and otherwise serves a session that echoes every line, taking
// SLOW_MS milliseconds per line when the env says so, or blocks until
// drained when the env carries LONG_WAIT.
type echoHandler struct {
	mu     sync.Mutex
	hellos []ProxyHello
}

func (h *echoHandler) Accept(hello ProxyHello) (ProxySession, error) {
	h.mu.Lock()
	h.hellos = append(h.hellos, hello)
	h.mu.Unlock()
	if reason := hello.Env["REFUSE"]; reason != "" {
		return nil, errors.New(reason)
	}
	s := &echoSession{longWait: hello.Env["LONG_WAIT"] != "", drained: make(chan struct{})}
	if ms := hello.Env["SLOW_MS"]; ms != "" {
		n, _ := strconv.Atoi(ms)
		s.slow = time.Duration(n) * time.Millisecond
	}
	return s, nil
}

type echoSession struct {
	slow     time.Duration
	longWait bool
	drained  chan struct{}
	once     sync.Once
	inflight sync.WaitGroup
	// drainedAt records when Drain began; endedAt when the long wait ended.
	drainedAt, endedAt time.Time
}

func (s *echoSession) Serve(ctx context.Context, stream io.ReadWriteCloser) {
	r := bufio.NewReader(stream)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		if s.longWait {
			<-s.drained
			s.endedAt = time.Now()
			_, _ = stream.Write([]byte("cancelled\n"))
			continue
		}
		s.inflight.Add(1)
		if s.slow > 0 {
			time.Sleep(s.slow)
		}
		_, _ = stream.Write(line)
		s.inflight.Done()
	}
}

func (s *echoSession) Drain(ctx context.Context) {
	s.drainedAt = time.Now()
	s.once.Do(func() { close(s.drained) })
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func startProxyTestDaemon(t *testing.T, h ProxyHandler) *Daemon {
	t.Helper()
	d, err := StartDaemon(DaemonOptions{Dir: t.TempDir(), Port: 0, Version: "1.2.3", LockWait: 200 * time.Millisecond, Proxy: h})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// proxyUpgrade dials the daemon and sends the upgrade request with the
// given token; it returns the connection and the response status line.
func proxyUpgrade(t *testing.T, d *Daemon, token string, extraHeaders ...string) (net.Conn, *bufio.Reader, string) {
	t.Helper()
	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(d.Endpoint().Port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	req := "GET " + ProxyPath + " HTTP/1.1\r\nHost: 127.0.0.1:" + strconv.Itoa(d.Endpoint().Port) + "\r\nUpgrade: " + ProxyUpgrade + "\r\nConnection: Upgrade\r\n"
	if token != "" {
		req += "Authorization: Bearer " + token + "\r\n"
	}
	for _, h := range extraHeaders {
		req += h + "\r\n"
	}
	req += "\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	// Skip the headers.
	for {
		line, err := r.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}
	return conn, r, strings.TrimSpace(status)
}

func sendHello(t *testing.T, conn net.Conn, r *bufio.Reader, hello ProxyHello) ProxyAccept {
	t.Helper()
	raw, _ := json.Marshal(hello)
	if _, err := conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("accept line: %v", err)
	}
	var accept ProxyAccept
	if err := json.Unmarshal(line, &accept); err != nil {
		t.Fatalf("accept line %q: %v", line, err)
	}
	return accept
}

func testHello(env map[string]string) ProxyHello {
	if env == nil {
		env = map[string]string{}
	}
	return ProxyHello{Protocol: ProxyProtocol, Mode: ProxyModeMCP, Cwd: "/work", ProjectRoot: "/work", Env: env, OwnerPID: 4242, Version: "1.2.9"}
}

func TestProxyRouteRefusesAMissingOrWrongToken(t *testing.T) {
	d := startProxyTestDaemon(t, &echoHandler{})
	ep := d.Endpoint()
	last := "0"
	if strings.HasSuffix(ep.Token, "0") {
		last = "1"
	}
	for name, token := range map[string]string{"missing": "", "wrong": ep.Token[:63] + last} {
		_, _, status := proxyUpgrade(t, d, token)
		if !strings.Contains(status, "401") {
			t.Fatalf("%s token: %s, want 401", name, status)
		}
	}
	if d.ProxySessions() != 0 {
		t.Fatal("a refused upgrade opened a session")
	}
}

func TestProxyRouteRefusesABrowserOriginAndANonLoopbackPeer(t *testing.T) {
	d := startProxyTestDaemon(t, &echoHandler{})
	_, _, status := proxyUpgrade(t, d, d.Endpoint().Token, "Origin: http://127.0.0.1:"+strconv.Itoa(d.Endpoint().Port))
	if !strings.Contains(status, "403") {
		t.Fatalf("browser origin: %s, want 403", status)
	}
	// A non-loopback peer cannot reach a loopback listener in a test, so
	// the handler is called directly with a LAN remote address; the check
	// runs before the token, so the right token does not help.
	for _, lan := range []bool{false, true} {
		d := startProxyTestDaemonLAN(t, &echoHandler{}, lan)
		req := httptest.NewRequest(http.MethodGet, ProxyPath, nil)
		req.RemoteAddr = "192.168.1.20:5555"
		req.Host = "127.0.0.1:" + strconv.Itoa(d.Endpoint().Port)
		req.Header.Set("Authorization", "Bearer "+d.Endpoint().Token)
		req.Header.Set("Upgrade", ProxyUpgrade)
		req.Header.Set("Connection", "Upgrade")
		rec := httptest.NewRecorder()
		d.handleProxy(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("lan %v: non-loopback peer: status %d, want 403", lan, rec.Code)
		}
	}
}

func startProxyTestDaemonLAN(t *testing.T, h ProxyHandler, lan bool) *Daemon {
	t.Helper()
	if !lan {
		return startProxyTestDaemon(t, h)
	}
	d, err := StartDaemon(DaemonOptions{Dir: t.TempDir(), Port: 0, Version: "1.2.3", LockWait: 200 * time.Millisecond, Proxy: h, LAN: true, bindHost: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func TestProxyRouteNeedsTheUpgradeHeaders(t *testing.T) {
	d := startProxyTestDaemon(t, &echoHandler{})
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(d.Endpoint().Port)+ProxyPath, nil)
	req.Header.Set("Authorization", "Bearer "+d.Endpoint().Token)
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain GET: status %d, want 426", resp.StatusCode)
	}
}

func TestProxyHelloIsAnsweredThenTheStreamIsRaw(t *testing.T) {
	h := &echoHandler{}
	d := startProxyTestDaemon(t, h)
	conn, r, status := proxyUpgrade(t, d, d.Endpoint().Token)
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade: %s", status)
	}
	// The hello and the first line arrive in one write: the session must
	// see the line the hello reader buffered past the hello.
	hello := testHello(map[string]string{"PATH": "/usr/bin", "WS_RSRC_ROOT": "/plugin/rsrc"})
	raw, _ := json.Marshal(hello)
	if _, err := conn.Write(append(append(raw, '\n'), []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")...)); err != nil {
		t.Fatal(err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var accept ProxyAccept
	if err := json.Unmarshal(line, &accept); err != nil || !accept.OK || accept.Version != "1.2.3" || accept.PID != d.Endpoint().PID {
		t.Fatalf("accept = %s (%v)", line, err)
	}
	echoed, err := r.ReadString('\n')
	if err != nil || echoed != `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n" {
		t.Fatalf("echo = %q, %v", echoed, err)
	}
	if _, err := conn.Write([]byte("second\n")); err != nil {
		t.Fatal(err)
	}
	if echoed, _ := r.ReadString('\n'); echoed != "second\n" {
		t.Fatalf("second echo = %q", echoed)
	}
	if got := h.hellos[0]; got.Env["PATH"] != "/usr/bin" || got.OwnerPID != 4242 || got.ProjectRoot != "/work" || got.Version != "1.2.9" {
		t.Fatalf("handler saw hello %+v", got)
	}
	if d.ProxySessions() != 1 {
		t.Fatalf("sessions = %d, want 1", d.ProxySessions())
	}
	conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for d.ProxySessions() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if d.ProxySessions() != 0 {
		t.Fatal("the session outlived its stream")
	}
}

func TestProxyRefusedHelloGetsAReasonLine(t *testing.T) {
	d := startProxyTestDaemon(t, &echoHandler{})
	for name, hello := range map[string]ProxyHello{
		"handler":     testHello(map[string]string{"REFUSE": "no resources"}),
		"protocol":    {Protocol: 2, Mode: ProxyModeMCP, Cwd: "/w", Env: map[string]string{}, OwnerPID: 1},
		"mode":        {Protocol: ProxyProtocol, Mode: "shell", Cwd: "/w", Env: map[string]string{}, OwnerPID: 1},
		"no env":      {Protocol: ProxyProtocol, Mode: ProxyModeMCP, Cwd: "/w", OwnerPID: 1},
		"no pid":      {Protocol: ProxyProtocol, Mode: ProxyModeMCP, Cwd: "/w", Env: map[string]string{}},
		"other minor": {Protocol: ProxyProtocol, Mode: ProxyModeMCP, Cwd: "/w", Env: map[string]string{}, OwnerPID: 1, Version: "1.3.0"},
		"cli no argv": {Protocol: ProxyProtocol, Mode: ProxyModeCLI, Cwd: "/w", Env: map[string]string{}, OwnerPID: 1},
	} {
		conn, r, _ := proxyUpgrade(t, d, d.Endpoint().Token)
		accept := sendHello(t, conn, r, hello)
		if accept.OK || accept.Reason == "" {
			t.Fatalf("%s: accept = %+v, want a refusal with a reason", name, accept)
		}
		if name == "handler" && accept.Reason != "no resources" {
			t.Fatalf("handler refusal reason = %q", accept.Reason)
		}
		if _, err := r.ReadByte(); err != io.EOF {
			t.Fatalf("%s: stream after refusal: %v, want EOF", name, err)
		}
	}
}

func TestProxyDrainFinishesCallsInFlightAndCancelsLongWaits(t *testing.T) {
	h := &echoHandler{}
	d := startProxyTestDaemon(t, h)
	slowConn, slowR, _ := proxyUpgrade(t, d, d.Endpoint().Token)
	if accept := sendHello(t, slowConn, slowR, testHello(map[string]string{"SLOW_MS": "400"})); !accept.OK {
		t.Fatal(accept.Reason)
	}
	waitConn, waitR, _ := proxyUpgrade(t, d, d.Endpoint().Token)
	if accept := sendHello(t, waitConn, waitR, testHello(map[string]string{"LONG_WAIT": "1"})); !accept.OK {
		t.Fatal(accept.Reason)
	}
	if _, err := slowConn.Write([]byte("in-flight\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := waitConn.Write([]byte("wait\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	started := time.Now()
	go d.Close()
	// The call in flight finishes and its answer arrives before the stream
	// closes.
	if echoed, err := slowR.ReadString('\n'); err != nil || echoed != "in-flight\n" {
		t.Fatalf("in-flight call answer = %q, %v", echoed, err)
	}
	// The long wait was cancelled at once, not after the in-flight call.
	if line, err := waitR.ReadString('\n'); err != nil || line != "cancelled\n" {
		t.Fatalf("long wait answer = %q, %v", line, err)
	}
	waitDone(t, d)
	if elapsed := time.Since(started); elapsed >= DefaultLockWait-time.Second {
		t.Fatalf("Close took %s, must stay under %s", elapsed, DefaultLockWait-time.Second)
	}
	if _, err := slowR.ReadByte(); err != io.EOF {
		t.Fatalf("stream after drain: %v, want EOF", err)
	}
	// No session is admitted while draining or after.
	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(d.Endpoint().Port))
	if err == nil {
		conn.Close()
		t.Fatal("the port still accepts after Close")
	}
}

func TestProxyDrainGivesUpOnACallThatOutlivesTheBudget(t *testing.T) {
	h := &echoHandler{}
	d := startProxyTestDaemon(t, h)
	conn, r, _ := proxyUpgrade(t, d, d.Endpoint().Token)
	if accept := sendHello(t, conn, r, testHello(map[string]string{"SLOW_MS": "20000"})); !accept.OK {
		t.Fatal(accept.Reason)
	}
	if _, err := conn.Write([]byte("stuck\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	started := time.Now()
	d.Close()
	if elapsed := time.Since(started); elapsed >= DefaultLockWait-time.Second {
		t.Fatalf("Close took %s, must stay under %s", elapsed, DefaultLockWait-time.Second)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		t.Fatalf("stream after drain: %v, want EOF", err)
	}
}

func TestSameMinor(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.48.16", "0.48.17", true},
		{"0.48.16", "0.48.16-dev", true},
		{"0.48.16", "0.49.0", false},
		{"1.48.16", "0.48.16", false},
		{"", "0.48.16", false},
		{"junk", "0.48.16", false},
	} {
		if got := SameMinor(tc.a, tc.b); got != tc.want {
			t.Errorf("SameMinor(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
