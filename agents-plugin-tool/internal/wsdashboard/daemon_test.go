package wsdashboard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func testClient() *http.Client {
	return &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
}

func startTestDaemon(t *testing.T, dir, version string) *Daemon {
	t.Helper()
	d, err := StartDaemon(DaemonOptions{Dir: dir, Port: 0, Version: version, LockWait: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("StartDaemon(%s): %v", version, err)
	}
	t.Cleanup(d.Close)
	return d
}

func waitDone(t *testing.T, d *Daemon) {
	t.Helper()
	select {
	case <-d.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestSecondDaemonIsRefusedByTheLock(t *testing.T) {
	dir := t.TempDir()
	first := startTestDaemon(t, dir, "1.0.0")
	if _, err := StartDaemon(DaemonOptions{Dir: dir, Version: "1.0.0", LockWait: 100 * time.Millisecond}); !errors.Is(err, ErrLocked) {
		t.Fatalf("second StartDaemon err = %v, want ErrLocked", err)
	}
	// The refused start left the first daemon's endpoint file alone.
	if ep, err := ReadEndpoint(dir); err != nil || ep.Token != first.Endpoint().Token {
		t.Fatalf("endpoint after refused start = %+v, %v", ep, err)
	}
	first.Close()
	waitDone(t, first)
	second := startTestDaemon(t, dir, "1.0.0")
	if second.Endpoint().Token == first.Endpoint().Token {
		t.Fatal("second daemon reused the first daemon's token")
	}
}

func TestEndpointFileRoundTripIgnoresUnknownFields(t *testing.T) {
	dir := t.TempDir()
	d := startTestDaemon(t, dir, "1.2.3")
	got, err := ReadEndpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != d.Endpoint() || got.Port == 0 || got.PID != os.Getpid() || got.Version != "1.2.3" || len(got.Token) != 64 {
		t.Fatalf("endpoint = %+v, daemon wrote %+v", got, d.Endpoint())
	}

	other := t.TempDir()
	body := `{"schema_version": 9, "port": 4242, "future_field": {"nested": true}, "token": "abc"}`
	if err := os.WriteFile(filepath.Join(other, endpointFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadEndpoint(other)
	if err != nil {
		t.Fatalf("ReadEndpoint with unknown fields: %v", err)
	}
	if want := (Endpoint{SchemaVersion: 9, Port: 4242, Token: "abc"}); got != want {
		t.Fatalf("endpoint = %+v, want %+v (absent fields empty)", got, want)
	}
}

func TestHealthAnswersFromTheRecordedDaemonOnly(t *testing.T) {
	dir := t.TempDir()
	d := startTestDaemon(t, dir, "2.0.0")
	info, err := Health(context.Background(), testClient(), d.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "2.0.0" || info.PID != os.Getpid() || info.Port != d.Endpoint().Port || info.Service != healthService {
		t.Fatalf("health = %+v", info)
	}
	stale := d.Endpoint()
	stale.PID++
	if _, err := Health(context.Background(), testClient(), stale); err == nil {
		t.Fatal("Health accepted an answer whose pid does not match the endpoint file")
	}
}

func TestShutdownRequiresTheEndpointToken(t *testing.T) {
	dir := t.TempDir()
	d := startTestDaemon(t, dir, "1.0.0")
	ep := d.Endpoint()
	url := "http://127.0.0.1:" + strconv.Itoa(ep.Port) + "/shutdown"
	for name, header := range map[string]string{
		"missing":   "",
		"wrong":     "Bearer " + ep.Token[:63] + "0",
		"no scheme": ep.Token,
	} {
		req, _ := http.NewRequest(http.MethodPost, url, nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s token: status %d, want 401", name, resp.StatusCode)
		}
	}
	select {
	case <-d.Done():
		t.Fatal("daemon stopped on an unauthenticated shutdown")
	default:
	}
	if err := RequestShutdown(context.Background(), testClient(), ep); err != nil {
		t.Fatal(err)
	}
	waitDone(t, d)
	if _, err := os.Stat(filepath.Join(dir, endpointFileName)); !os.IsNotExist(err) {
		t.Fatalf("endpoint file after shutdown: %v", err)
	}
	// The lock is free again.
	startTestDaemon(t, dir, "1.0.0")
}

func TestNonLoopbackHostIsRefused(t *testing.T) {
	d := startTestDaemon(t, t.TempDir(), "1.0.0")
	handler := d.Handler()
	for host, want := range map[string]int{
		"127.0.0.1:29417":        http.StatusOK,
		"localhost:29417":        http.StatusOK,
		"LOCALHOST":              http.StatusOK,
		"[::1]:29417":            http.StatusOK,
		"evil.example:29417":     http.StatusForbidden,
		"127.0.0.1.nip.io":       http.StatusForbidden,
		"192.168.0.10:29417":     http.StatusForbidden,
		"localhost.evil.example": http.StatusForbidden,
		"":                       http.StatusForbidden,
	} {
		for _, path := range []string{"/health", "/shutdown"} {
			method := http.MethodGet
			if path == "/shutdown" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, path, nil)
			req.Host = host
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			wantPath := want
			if path == "/shutdown" && want == http.StatusOK {
				wantPath = http.StatusUnauthorized // passed the guard, no token
			}
			if rec.Code != wantPath {
				t.Errorf("%s %s Host %q: status %d, want %d", method, path, host, rec.Code, wantPath)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s Host %q sent a CORS header", path, host)
			}
		}
	}
}

func TestTakenPortFallsBackToAnOSAssignedPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	taken := busy.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	d, err := StartDaemon(DaemonOptions{Dir: dir, Port: taken, Version: "1.0.0", LockWait: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got := d.Endpoint().Port; got == taken || got == 0 {
		t.Fatalf("port = %d with %d taken", got, taken)
	}
	if ep, _ := ReadEndpoint(dir); ep.Port != d.Endpoint().Port {
		t.Fatalf("endpoint file port = %d, daemon port %d", ep.Port, d.Endpoint().Port)
	}

	// A free preferred port is used as given.
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	freePort := free.Addr().(*net.TCPAddr).Port
	free.Close()
	d.Close()
	waitDone(t, d)
	d2, err := StartDaemon(DaemonOptions{Dir: dir, Port: freePort, Version: "1.0.0", LockWait: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if d2.Endpoint().Port != freePort {
		t.Fatalf("port = %d, want the free preferred port %d", d2.Endpoint().Port, freePort)
	}
}

// inProcessSpawner starts daemons in this process the way a detached
// `ws-mcp dashboard --port N` would, recording each call's arguments.
type inProcessSpawner struct {
	t        *testing.T
	dir      string
	versions []string // version per call; the last repeats
	lockWait time.Duration

	mu      sync.Mutex
	calls   [][]string
	daemons []*Daemon
	errs    []error
}

func (s *inProcessSpawner) spawn(args []string) error {
	s.mu.Lock()
	n := len(s.calls)
	s.calls = append(s.calls, append([]string(nil), args...))
	s.mu.Unlock()
	port, err := strconv.Atoi(args[len(args)-1])
	if err != nil || len(args) != 3 || args[0] != "dashboard" || args[1] != "--port" {
		s.t.Errorf("spawn args = %v", args)
		return errors.New("bad args")
	}
	version := s.versions[min(n, len(s.versions)-1)]
	wait := s.lockWait
	if wait == 0 {
		wait = 2 * time.Second
	}
	go func() {
		d, err := StartDaemon(DaemonOptions{Dir: s.dir, Port: port, Version: version, LockWait: wait})
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.errs = append(s.errs, err)
			return
		}
		s.daemons = append(s.daemons, d)
	}()
	return nil
}

func (s *inProcessSpawner) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.daemons {
		d.Close()
	}
}

func ensureOpts(dir, version string, spawn func([]string) error) EnsureOptions {
	return EnsureOptions{Dir: dir, Version: version, Port: 0, Spawn: spawn, HTTP: testClient(), StartWait: 3 * time.Second, PollInterval: 10 * time.Millisecond}
}

func TestEnsureStartsADaemonWithTheConfiguredPort(t *testing.T) {
	dir := t.TempDir()
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"1.0.0"}}
	defer sp.closeAll()
	opts := ensureOpts(dir, "1.0.0", sp.spawn)
	opts.Port = port
	outcome, err := Ensure(context.Background(), opts)
	if err != nil || outcome != OutcomeStarted {
		t.Fatalf("Ensure = %s, %v", outcome, err)
	}
	if len(sp.calls) != 1 || sp.calls[0][2] != strconv.Itoa(port) {
		t.Fatalf("spawn calls = %v, want one with --port %d", sp.calls, port)
	}
	if ep, _ := ReadEndpoint(dir); ep.Port != port {
		t.Fatalf("daemon port = %d, want %d", ep.Port, port)
	}
	// A second Ensure finds it running and starts nothing.
	if outcome, err := Ensure(context.Background(), opts); err != nil || outcome != OutcomeRunning || len(sp.calls) != 1 {
		t.Fatalf("second Ensure = %s, %v, %d spawns", outcome, err, len(sp.calls))
	}
}

func TestEnsureHandoffReplacesAnOlderDaemon(t *testing.T) {
	dir := t.TempDir()
	old := startTestDaemon(t, dir, "0.47.0")
	sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.48.0"}}
	defer sp.closeAll()
	outcome, err := Ensure(context.Background(), ensureOpts(dir, "0.48.0", sp.spawn))
	if err != nil || outcome != OutcomeReplaced {
		t.Fatalf("Ensure = %s, %v", outcome, err)
	}
	waitDone(t, old) // the older daemon exited and released the lock
	ep, err := ReadEndpoint(dir)
	if err != nil || ep.Version != "0.48.0" || ep.Token == old.Endpoint().Token {
		t.Fatalf("endpoint after handoff = %+v, %v", ep, err)
	}
	if info, err := Health(context.Background(), testClient(), ep); err != nil || info.Version != "0.48.0" {
		t.Fatalf("new daemon health = %+v, %v", info, err)
	}
}

func TestEnsureFromAnOlderWsMcpLeavesANewerDaemonRunning(t *testing.T) {
	for _, tc := range []struct{ daemon, client string }{
		{"0.49.0", "0.48.0"},
		{"0.48.0", "0.48.0"},
		{"0.48.0", "0.48.0-dev"},
	} {
		dir := t.TempDir()
		d := startTestDaemon(t, dir, tc.daemon)
		spawned := 0
		outcome, err := Ensure(context.Background(), ensureOpts(dir, tc.client, func([]string) error { spawned++; return nil }))
		if err != nil || outcome != OutcomeRunning || spawned != 0 {
			t.Fatalf("daemon %s, ws-mcp %s: Ensure = %s, %v, %d spawns", tc.daemon, tc.client, outcome, err, spawned)
		}
		select {
		case <-d.Done():
			t.Fatalf("daemon %s stopped for ws-mcp %s", tc.daemon, tc.client)
		default:
		}
	}
}

func TestEnsureReplacesAConcurrentlyStartedOlderDaemon(t *testing.T) {
	dir := t.TempDir()
	// The first start loses the race to an older ws-mcp's daemon.
	sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.47.0", "0.48.0"}}
	defer sp.closeAll()
	outcome, err := Ensure(context.Background(), ensureOpts(dir, "0.48.0", sp.spawn))
	if err != nil || outcome != OutcomeReplaced || len(sp.calls) != 2 {
		t.Fatalf("Ensure = %s, %v, %d spawns", outcome, err, len(sp.calls))
	}
	if ep, _ := ReadEndpoint(dir); ep.Version != "0.48.0" {
		t.Fatalf("running daemon = %s, want 0.48.0", ep.Version)
	}
}

// TestHungDaemonIsLeftAlone: a holder that keeps the lock and accepts
// connections without answering /health is not treated as gone. Ensure starts
// a daemon, which cannot take the lock and exits; the holder keeps its lock
// and its connection, and Ensure reports the dashboard unavailable.
func TestHungDaemonIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	holder := flock.New(filepath.Join(dir, lockFileName))
	if ok, err := holder.TryLock(); !ok || err != nil {
		t.Fatalf("hold lock: %v, %v", ok, err)
	}
	defer holder.Unlock()
	hung, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hung.Close()
	accepted := make(chan net.Conn, 4)
	go func() {
		for {
			c, err := hung.Accept()
			if err != nil {
				return
			}
			accepted <- c // read nothing, answer nothing
		}
	}()
	stale := Endpoint{SchemaVersion: 1, Port: hung.Addr().(*net.TCPAddr).Port, PID: os.Getpid(), Version: "0.1.0", Token: "hung-token"}
	if err := writeEndpoint(dir, stale); err != nil {
		t.Fatal(err)
	}

	sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.48.0"}, lockWait: 200 * time.Millisecond}
	defer sp.closeAll()
	opts := ensureOpts(dir, "0.48.0", sp.spawn)
	opts.HTTP = &http.Client{Timeout: 200 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
	opts.StartWait = time.Second
	outcome, err := Ensure(context.Background(), opts)
	if err != nil || outcome != OutcomeUnavailable {
		t.Fatalf("Ensure = %s, %v, want unavailable", outcome, err)
	}
	if len(sp.calls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(sp.calls))
	}
	sp.mu.Lock()
	errs := append([]error(nil), sp.errs...)
	daemons := len(sp.daemons)
	sp.mu.Unlock()
	if daemons != 0 || len(errs) != 1 || !errors.Is(errs[0], ErrLocked) {
		t.Fatalf("started daemon: %d running, errs %v; want one ErrLocked", daemons, errs)
	}
	if !holder.Locked() {
		t.Fatal("holder lost its lock")
	}
	if ep, _ := ReadEndpoint(dir); ep.Token != "hung-token" {
		t.Fatalf("endpoint file rewritten while the holder lives: %+v", ep)
	}
	select {
	case c := <-accepted:
		c.Close()
	default:
		t.Fatal("hung holder never saw the health connection")
	}
}

func TestEnsureStartsOneWhenTheRecordedDaemonIsGone(t *testing.T) {
	dir := t.TempDir()
	// A crashed daemon leaves its endpoint file; the kernel freed its lock.
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	stalePort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	if err := writeEndpoint(dir, Endpoint{Port: stalePort, PID: 999999, Version: "0.48.0", Token: "stale"}); err != nil {
		t.Fatal(err)
	}
	sp := &inProcessSpawner{t: t, dir: dir, versions: []string{"0.48.0"}}
	defer sp.closeAll()
	outcome, err := Ensure(context.Background(), ensureOpts(dir, "0.48.0", sp.spawn))
	if err != nil || outcome != OutcomeStarted {
		t.Fatalf("Ensure = %s, %v", outcome, err)
	}
	if ep, _ := ReadEndpoint(dir); ep.Token == "stale" || ep.PID != os.Getpid() {
		t.Fatalf("endpoint = %+v", ep)
	}
}

func TestEnsureReportsASpawnFailure(t *testing.T) {
	outcome, err := Ensure(context.Background(), ensureOpts(t.TempDir(), "1.0.0", func([]string) error { return errors.New("no exec") }))
	if err == nil || outcome != OutcomeUnavailable {
		t.Fatalf("Ensure = %s, %v", outcome, err)
	}
}
