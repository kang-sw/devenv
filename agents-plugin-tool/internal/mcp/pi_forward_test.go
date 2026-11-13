package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// fakeTokenRoute is the dashboard daemon's token route as a Pi worker's
// ws-mcp reaches it: it records each call and answers per op.
type fakeTokenRoute struct {
	mu    sync.Mutex
	calls []fakeTokenCall
}

type fakeTokenCall struct {
	path, auth string
	body       map[string]any
}

func (f *fakeTokenRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.calls = append(f.calls, fakeTokenCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
	f.mu.Unlock()
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer tok123" {
		http.Error(w, "the capability token is not valid", http.StatusUnauthorized)
		return
	}
	answer := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	kid := wsagent.State{ID: "kid1", Status: wsagent.StatusRunning, Root: "/repo", Model: "prov/small", Effort: "low", Tier: "small", Label: "sub"}
	switch strings.TrimPrefix(r.URL.Path, wsagent.RoutePrefix+"/by-token/") {
	case "spawn":
		if body["tier"] == "large" {
			http.Error(w, `pi.spawn: tier "large" is above this conversation's own tier "medium", which caps its subagents' tier; pick "medium" or lower`, http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusCreated)
		answer(kid)
	case "list":
		answer(map[string]any{"conversations": []wsagent.State{kid}})
	case "send", "stop":
		answer(kid)
	case "transcript":
		answer(map[string]any{"records": []any{map[string]any{"kind": "reply", "text": "kid says hi", "at": "2026-11-13T00:00:00Z"}}})
	case "wait":
		answer(wsagent.TurnResult{ID: "kid1", Status: wsagent.TurnDone, Message: "kid finished"})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeTokenRoute) last() fakeTokenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// TestPiWorkerForwardsPiToolsOnItsToken: a Pi worker's attached ws-mcp
// (no runner, a delegate key) lists the pi.* tools with the worker's
// descriptions and forwards each call to the daemon's token route on its
// token; every other lead-only tool stays refused to its key, and without
// a token pi.* is refused like any lead-only tool.
func TestPiWorkerForwardsPiToolsOnItsToken(t *testing.T) {
	// The worker's server checks neither the knob nor the Pi version: the
	// daemon does.
	setPiVersion(t, "", errors.New("no pi here"))
	setupMailboxTestEnv(t)
	route := &fakeTokenRoute{}
	daemon := httptest.NewServer(route)
	defer daemon.Close()
	t.Setenv(envAllowedTools, "git.status,workflow_state,pi.spawn,pi.send,pi.list,pi.stop,pi.transcript,pi.wait")
	root := t.TempDir()
	initGit(t, root)

	// Without a token: pi.* is a lead-only tool to a delegate key.
	plain := NewServer(root, "test")
	defer plain.Close()
	plainKey, err := plain.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedPiTools(plain); len(got) != 0 {
		t.Fatalf("no token lists no pi.* tool: %v", got)
	}
	if raw := callToolOnce(t, plain, 2, "pi.list", map[string]any{"session_key": plainKey}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("no token refuses pi.*: %s", raw)
	}

	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, daemon.URL+"/")
	s := NewServer(root, "test")
	defer s.Close()
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedPiTools(s); strings.Join(got, ",") != strings.Join(piToolNames, ",") {
		t.Fatalf("listed = %v", got)
	}
	for _, tool := range s.filteredTools() {
		if name, _ := tool["name"].(string); name == "pi.spawn" {
			if desc, _ := tool["description"].(string); !strings.Contains(desc, "Spawn a Pi subagent of your own") || strings.Contains(desc, "mailbox") {
				t.Fatalf("pi.spawn reads as a worker's: %q", desc)
			}
			props, _ := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if cwd := props["cwd"].(map[string]string)["description"]; !strings.Contains(cwd, "relative to your own working directory") || strings.Contains(cwd, "root") {
				t.Fatalf("pi.spawn's cwd reads as a worker's: %q", cwd)
			}
		}
	}

	out := callToolWithKey(t, s, 3, key, "pi.spawn", map[string]any{"prompt": "sub task", "tier": "small", "cwd": "sub", "label": "sub"})
	if !strings.Contains(out, "spawned Pi subagent kid1 (tier small: prov/small, effort low)") || !strings.Contains(out, "steered into your running turn") {
		t.Fatalf("pi.spawn = %q", out)
	}
	call := route.last()
	if call.path != "/agents/by-token/spawn" || call.auth != "Bearer tok123" || call.body["prompt"] != "sub task" || call.body["tier"] != "small" || call.body["cwd"] != "sub" || call.body["label"] != "sub" {
		t.Fatalf("forwarded spawn = %+v", call)
	}
	if out := callToolWithKey(t, s, 4, key, "pi.spawn", map[string]any{"prompt": "x", "tier": "large"}); !strings.Contains(out, `tier "large" is above this conversation's own tier "medium"`) {
		t.Fatalf("a daemon refusal reaches the worker: %q", out)
	}
	if out := callToolWithKey(t, s, 5, key, "pi.list", nil); !strings.Contains(out, "kid1  running  prov/small/low") {
		t.Fatalf("pi.list = %q", out)
	}
	if out := callToolWithKey(t, s, 6, key, "pi.send", map[string]any{"id": "kid1", "message": "more"}); !strings.Contains(out, "steers its running turn at the next tool boundary") || !strings.Contains(out, "report covers the message") || route.last().body["message"] != "more" {
		t.Fatalf("pi.send = %q", out)
	}
	if out := callToolWithKey(t, s, 7, key, "pi.transcript", map[string]any{"id": "kid1"}); !strings.Contains(out, "kid says hi") {
		t.Fatalf("pi.transcript = %q", out)
	}
	if out := callToolWithKey(t, s, 8, key, "pi.wait", map[string]any{"id": "kid1", "timeout_s": 5}); !strings.Contains(out, "Pi subagent kid1: turn done") || !strings.Contains(out, "kid finished") || route.last().body["timeout_s"] != float64(5) {
		t.Fatalf("pi.wait = %q (%+v)", out, route.last())
	}
	if out := callToolWithKey(t, s, 9, key, "pi.stop", map[string]any{"id": "kid1"}); !strings.Contains(out, "closed Pi subagent kid1") || route.last().path != "/agents/by-token/stop" {
		t.Fatalf("pi.stop = %q", out)
	}

	// The token authorizes pi.* alone: another lead-only tool stays
	// refused to the worker's delegate key.
	if raw := callToolOnce(t, s, 10, "workflow_state", map[string]any{"session_key": key}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("a lead-only tool with a token: %s", raw)
	}

	// An unreachable daemon is named.
	daemon.Close()
	if out := callToolWithKey(t, s, 11, key, "pi.list", nil); !strings.Contains(out, "the dashboard daemon") || !strings.Contains(out, "did not answer") {
		t.Fatalf("an unreachable daemon = %q", out)
	}
}

type piDeadlineTransport struct{ t *testing.T }

func (p piDeadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body struct {
		Timeout float64 `json:"timeout_s"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		p.t.Fatal(err)
	}
	deadline, ok := req.Context().Deadline()
	remaining := time.Until(deadline)
	want := 1000*time.Second + piForwardSlack
	if body.Timeout != 1000 || !ok || remaining > want || remaining < want-time.Second {
		p.t.Fatalf("forwarded wait: timeout=%v deadline remaining=%v, want %v", body.Timeout, remaining, want)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"kid1","status":"running"}`)), Header: http.Header{}}, nil
}

func TestPiForwardWaitDeadlineIncludesRequestedBudget(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, "http://127.0.0.1:1")
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	key, err := s.sessions.mint(s.root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: piDeadlineTransport{t}}
	t.Cleanup(func() { http.DefaultClient = old })
	out := callToolWithKey(t, s, 1, key, "pi.wait", map[string]any{"id": "kid1", "timeout_s": 1000})
	if !strings.Contains(out, "still running") {
		t.Fatalf("pi.wait = %q", out)
	}
}

// setPiForwardEndpoint publishes a fake daemon in the server's own cache.
func setPiForwardEndpoint(t *testing.T, s *Server, endpoint string) {
	t.Helper()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(endpoint, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := s.dashboard.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(wsdashboard.Endpoint{SchemaVersion: 1, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPiForwardRediscoversEndpointOnEachCall(t *testing.T) {
	setupMailboxTestEnv(t)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"endpoint":"first"}`)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			http.Error(w, "wrong capability", http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"endpoint":"second"}`)
	}))
	defer second.Close()
	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, first.URL)
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	call := func(want string) {
		t.Helper()
		var answer struct{ Endpoint string }
		if err := s.piCall(context.Background(), "pi.list", map[string]any{}, time.Second, &answer); err != nil {
			t.Fatal(err)
		}
		if answer.Endpoint != want {
			t.Fatalf("forwarded to %q, want %q", answer.Endpoint, want)
		}
	}
	call("first") // No discovery file: retain the inherited endpoint fallback.
	setPiForwardEndpoint(t, s, second.URL)
	call("second")
	setPiForwardEndpoint(t, s, first.URL)
	call("first")
}

func TestPiForwardDisconnectedWaitReissuesOnNewEndpoint(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial_body=%v", partial), func(t *testing.T) {
			setupMailboxTestEnv(t)
			arrived := make(chan float64, 1)
			disconnect := make(chan struct{})
			old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Timeout float64 `json:"timeout_s"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if partial {
					w.Header().Set("Content-Length", "1000")
					io.WriteString(w, `{"id":`)
					w.(http.Flusher).Flush()
				}
				arrived <- body.Timeout
				<-disconnect
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
			}))
			defer old.Close()
			var release sync.Once
			releaseOld := func() { release.Do(func() { close(disconnect) }) }
			defer releaseOld()
			retried := make(chan fakeTokenCall, 1)
			next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				retried <- fakeTokenCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
				json.NewEncoder(w).Encode(wsagent.TurnResult{ID: "kid1", Status: wsagent.TurnDone, Message: "survived"})
			}))
			defer next.Close()
			t.Setenv(wsagent.EnvPiToken, "tok123")
			t.Setenv(wsagent.EnvPiEndpoint, old.URL)
			s := NewServer(t.TempDir(), "test")
			defer s.Close()
			setPiForwardEndpoint(t, s, old.URL)
			body := map[string]any{"id": "kid1", "timeout_s": float64(2)}
			var answer wsagent.TurnResult
			done := make(chan error, 1)
			go func() {
				done <- s.piCall(context.Background(), "pi.wait", body, 2*time.Second+piForwardSlack, &answer)
			}()
			select {
			case timeout := <-arrived:
				if timeout != 2 {
					t.Fatalf("initial wait = %v", timeout)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("initial wait never arrived")
			}
			setPiForwardEndpoint(t, s, next.URL)
			releaseOld()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("disconnected wait was not reissued")
			}
			call := <-retried
			remaining, _ := call.body["timeout_s"].(float64)
			if call.path != "/agents/by-token/wait" || call.auth != "Bearer tok123" || call.body["id"] != "kid1" || remaining <= 0 || remaining >= 2 {
				t.Fatalf("reissued wait did not retain identity/budget: %+v", call)
			}
			if answer.Status != wsagent.TurnDone || answer.Message != "survived" || body["timeout_s"] != float64(2) {
				t.Fatalf("wait result or caller's arguments changed: %+v, %+v", answer, body)
			}
		})
	}
}

func TestPiForwardDisconnectedWaitHonorsContext(t *testing.T) {
	setupMailboxTestEnv(t)
	var calls atomic.Int32
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer old.Close()
	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, old.URL)
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	var answer wsagent.TurnResult
	err := s.piCall(ctx, "pi.wait", map[string]any{"id": "kid1", "timeout_s": float64(5)}, 5*time.Second+piForwardSlack, &answer)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() == 0 {
		t.Fatalf("wait did not stop on its original context: calls=%d, err=%v", calls.Load(), err)
	}
}

func TestPiForwardDisconnectedSideEffectsAreNotRetried(t *testing.T) {
	for _, op := range []string{"spawn", "send", "stop"} {
		t.Run(op, func(t *testing.T) {
			setupMailboxTestEnv(t)
			var originalCalls, replacementCalls atomic.Int32
			next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				replacementCalls.Add(1)
				io.WriteString(w, `{}`)
			}))
			defer next.Close()
			arrived := make(chan struct{}, 1)
			disconnect := make(chan struct{})
			old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originalCalls.Add(1)
				io.Copy(io.Discard, r.Body)
				arrived <- struct{}{}
				<-disconnect
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
			}))
			defer old.Close()
			var release sync.Once
			releaseOld := func() { release.Do(func() { close(disconnect) }) }
			defer releaseOld()
			t.Setenv(wsagent.EnvPiToken, "tok123")
			t.Setenv(wsagent.EnvPiEndpoint, old.URL)
			s := NewServer(t.TempDir(), "test")
			defer s.Close()
			done := make(chan error, 1)
			go func() {
				var answer wsagent.State
				done <- s.piCall(context.Background(), "pi."+op, map[string]any{"id": "kid1", "message": "once"}, time.Second, &answer)
			}()
			select {
			case <-arrived:
			case <-time.After(5 * time.Second):
				t.Fatal("side effect never arrived")
			}
			// The daemon took the side effect, then its answer was lost.
			setPiForwardEndpoint(t, s, next.URL)
			releaseOld()
			select {
			case err := <-done:
				if err == nil || originalCalls.Load() != 1 || replacementCalls.Load() != 0 {
					t.Fatalf("ambiguous %s was retried: original=%d replacement=%d err=%v", op, originalCalls.Load(), replacementCalls.Load(), err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("disconnected side effect did not return")
			}
		})
	}
}

func TestPiForwardDaemonDetachRetriesOnlyWait(t *testing.T) {
	for _, tool := range []string{"pi.wait", "pi.send"} {
		t.Run(tool, func(t *testing.T) {
			setupMailboxTestEnv(t)
			var calls atomic.Int32
			retried := make(chan float64, 1)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Timeout float64 `json:"timeout_s"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				if calls.Add(1) == 1 {
					http.Error(w, "the dashboard daemon detached; reissue the wait", http.StatusServiceUnavailable)
					return
				}
				retried <- body.Timeout
				json.NewEncoder(w).Encode(wsagent.TurnResult{ID: "kid1", Status: wsagent.TurnDone, Message: "resumed"})
			}))
			defer daemon.Close()
			t.Setenv(wsagent.EnvPiToken, "tok123")
			t.Setenv(wsagent.EnvPiEndpoint, daemon.URL)
			s := NewServer(t.TempDir(), "test")
			defer s.Close()
			var answer wsagent.TurnResult
			err := s.piCall(context.Background(), tool, map[string]any{"id": "kid1", "message": "once", "timeout_s": float64(2)}, 2*time.Second+piForwardSlack, &answer)
			if tool == "pi.wait" {
				var remaining float64
				select {
				case remaining = <-retried:
				default:
				}
				if err != nil || calls.Load() != 2 || answer.Status != wsagent.TurnDone || answer.Message != "resumed" || remaining <= 0 || remaining >= 2 {
					t.Fatalf("503 wait retry: calls=%d remaining=%v answer=%+v err=%v", calls.Load(), remaining, answer, err)
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatalf("503 must not retry send: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

// routeBackend is a Pi backend for a real wsagent runner: each start hands
// out a routeProc the test drives.
type routeBackend struct {
	mu     sync.Mutex
	starts []wsagent.StartSpec
	procs  []*routeProc
}

func (b *routeBackend) Start(_ context.Context, spec wsagent.StartSpec) (wsagent.Process, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := &routeProc{events: make(chan wsagent.Event, 64), sent: make(chan string, 16)}
	b.starts = append(b.starts, spec)
	b.procs = append(b.procs, p)
	return p, nil
}

// waitStart waits for the i-th process start.
func (b *routeBackend) waitStart(t *testing.T, i int) (wsagent.StartSpec, *routeProc) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		if len(b.starts) > i {
			spec, p := b.starts[i], b.procs[i]
			b.mu.Unlock()
			return spec, p
		}
		b.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d did not start", i)
	return wsagent.StartSpec{}, nil
}

type routeProc struct {
	events chan wsagent.Event
	sent   chan string
	once   sync.Once
}

func (p *routeProc) Events() <-chan wsagent.Event { return p.events }
func (p *routeProc) Send(text string) error       { p.sent <- text; return nil }
func (p *routeProc) Interrupt() error             { return nil }
func (p *routeProc) SetModel(string) error        { return wsagent.ErrUnsupported }
func (p *routeProc) SetEffort(string) error       { return wsagent.ErrUnsupported }
func (p *routeProc) Close() error {
	p.once.Do(func() { close(p.events) })
	return nil
}

// reply sends a final reply and ends the turn.
func (p *routeProc) reply(text string) {
	p.events <- wsagent.Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: text, Final: true}}
	p.events <- wsagent.Event{TurnEnd: true}
}

func (p *routeProc) nextSent(t *testing.T) string {
	t.Helper()
	select {
	case s := <-p.sent:
		return s
	case <-time.After(5 * time.Second):
		t.Fatalf("no turn was sent")
		return ""
	}
}

type routeKeys struct {
	mu sync.Mutex
	n  int
}

func (k *routeKeys) Mint(string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.n++
	return fmt.Sprintf("route-test-key-%d", k.n), nil
}

func (k *routeKeys) Ensure(string, string) error { return nil }

// routeLog records the token route's answers: each call's op, status, and
// body, and announces each call as it arrives.
type routeLog struct {
	next    http.Handler
	arrived chan string
	mu      sync.Mutex
	calls   []routeAnswer
}

type routeAnswer struct {
	op   string
	code int
	body string
}

func (l *routeLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	op := strings.TrimPrefix(r.URL.Path, wsagent.RoutePrefix+"/by-token/")
	select {
	case l.arrived <- op:
	default:
	}
	rec := httptest.NewRecorder()
	l.next.ServeHTTP(rec, r)
	l.mu.Lock()
	l.calls = append(l.calls, routeAnswer{op: op, code: rec.Code, body: rec.Body.String()})
	l.mu.Unlock()
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

func (l *routeLog) last(t *testing.T, op string) routeAnswer {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.calls) - 1; i >= 0; i-- {
		if l.calls[i].op == op {
			return l.calls[i]
		}
	}
	t.Fatalf("no %s call reached the route", op)
	return routeAnswer{}
}

// TestPiForwardDrivesTheRealTokenRoute: a Pi worker's ws-mcp forwards each
// pi.* call to the daemon's real token route (a wsagent runner behind
// httptest) on the token the runner issued that worker: spawn, list, wait,
// send, transcript, and stop, with the route's status codes and the JSON
// the forwarder decodes and returns.
func TestPiForwardDrivesTheRealTokenRoute(t *testing.T) {
	setPiVersion(t, "", errors.New("no pi here"))
	setupMailboxTestEnv(t)
	cfg := piTierOptions()
	for tier, model := range map[string]string{"small": "prov/small-model", "medium": "prov/medium-model"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(cfg, tier, "", model, wsagent.PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	initGit(t, root)
	backend := &routeBackend{}
	dir := t.TempDir()
	runner := wsagent.NewRunner(wsagent.Options{
		Dir:      dir,
		Backends: map[string]wsagent.Backend{wsagent.BackendPi: backend},
		Keys:     &routeKeys{},
		Config:   cfg,
		WSMCP: func(root string) (wsagent.MCPCommand, error) {
			return wsagent.MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio", "--root", root}}, nil
		},
	})
	log := &routeLog{next: runner.Handler(), arrived: make(chan string, 1)}
	daemon := httptest.NewServer(log)
	defer daemon.Close()
	runner.SetEndpoint(daemon.URL)
	runner.Start(nil)
	defer runner.Close()

	w, err := runner.Spawn(wsagent.SpawnSpec{Profile: wsagent.PiWorkerProfile, Root: root, Prompt: "work", Tier: "medium", Label: "worker", Parent: &wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: "leadreplyid"}})
	if err != nil {
		t.Fatal(err)
	}
	spec, wp := backend.waitStart(t, 0)
	wp.nextSent(t)
	token, endpoint := spec.WSMCP.Env[wsagent.EnvPiToken], spec.WSMCP.Env[wsagent.EnvPiEndpoint]
	if token == "" || endpoint != daemon.URL {
		t.Fatalf("the worker's ws-mcp env = %v", spec.WSMCP.Env)
	}

	t.Setenv(envAllowedTools, "pi.spawn,pi.send,pi.list,pi.stop,pi.transcript,pi.wait")
	t.Setenv(wsagent.EnvPiToken, token)
	t.Setenv(wsagent.EnvPiEndpoint, endpoint)
	s := NewServer(root, "test")
	defer s.Close()
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	asJSON := func(id int, tool string, args map[string]any, out any) {
		t.Helper()
		if args == nil {
			args = map[string]any{}
		}
		args["format"] = "json"
		raw := callToolWithKey(t, s, id, key, tool, args)
		if err := json.Unmarshal([]byte(raw), out); err != nil {
			t.Fatalf("%s did not answer JSON: %q", tool, raw)
		}
	}

	// spawn: 201 with the child's state.
	var kid piChildOut
	asJSON(2, "pi.spawn", map[string]any{"prompt": "sub task", "tier": "small", "label": "sub"}, &kid)
	if a := log.last(t, "spawn"); a.code != http.StatusCreated {
		t.Fatalf("spawn = %d %s", a.code, a.body)
	}
	if kid.ID == "" || kid.Status == wsagent.StatusClosed || kid.Tier != "small" || kid.Model != "prov/small-model" || kid.Effort != "low" || kid.Label != "sub" || kid.Cwd != root {
		t.Fatalf("pi.spawn = %+v", kid)
	}
	_, kp := backend.waitStart(t, 1)
	if got := kp.nextSent(t); got != "sub task" {
		t.Fatalf("the child's first turn = %q", got)
	}
	if st, err := runner.Get(kid.ID); err != nil || st.Parent == nil || st.Parent.Kind != wsagent.ParentConversation || st.Parent.ReplyID != w.ID {
		t.Fatalf("the child's parent = %+v, %v", st, err)
	}

	// A refusal reaches the worker naming the tool once.
	out := callToolWithKey(t, s, 3, key, "pi.spawn", map[string]any{"prompt": "x", "tier": "large"})
	if a := log.last(t, "spawn"); a.code != http.StatusForbidden {
		t.Fatalf("an over-tier spawn = %d %s", a.code, a.body)
	}
	if !strings.Contains(out, `pi.spawn: tier "large" is above`) || strings.Contains(out, "pi.spawn: pi.spawn") {
		t.Fatalf("the refusal = %q", out)
	}

	// list: 200 with the holder's own children.
	var listed []piChildOut
	asJSON(4, "pi.list", nil, &listed)
	if a := log.last(t, "list"); a.code != http.StatusOK || !strings.Contains(a.body, `"conversations":[`) {
		t.Fatalf("list = %d %s", a.code, a.body)
	}
	if len(listed) != 1 || listed[0].ID != kid.ID {
		t.Fatalf("pi.list = %+v", listed)
	}

	// wait on a turn whose report already reached the worker: its status
	// alone, marked delivered.
	kp.reply("first result")
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec, err := wsagent.ReadRecord(dir, w.ID)
		if err == nil && len(rec.Reports) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child's report was not kept for the worker: %+v, %v", rec, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	var handed wsagent.TurnResult
	asJSON(5, "pi.wait", map[string]any{"id": kid.ID, "timeout_s": 2}, &handed)
	if a := log.last(t, "wait"); a.code != http.StatusOK {
		t.Fatalf("wait = %d %s", a.code, a.body)
	}
	if handed.ID != kid.ID || handed.Status != wsagent.TurnDone || !handed.Delivered || handed.Message != "" {
		t.Fatalf("a wait on a handed report = %+v", handed)
	}
	if out := callToolWithKey(t, s, 6, key, "pi.wait", map[string]any{"id": kid.ID, "timeout_s": 2}); !strings.Contains(out, "already reached you") || strings.Contains(out, "first result") {
		t.Fatalf("pi.wait text on a handed report = %q", out)
	}

	// send: 200; the wait for the turn it starts is that turn's report.
	var sent piChildOut
	asJSON(7, "pi.send", map[string]any{"id": kid.ID, "message": "more"}, &sent)
	if a := log.last(t, "send"); a.code != http.StatusOK || sent.ID != kid.ID {
		t.Fatalf("send = %d %s (%+v)", a.code, a.body, sent)
	}
	if got := kp.nextSent(t); got != "more" {
		t.Fatalf("the sent turn = %q", got)
	}
	for len(log.arrived) > 0 {
		<-log.arrived
	}
	waited := make(chan wsagent.TurnResult, 1)
	go func() {
		var res wsagent.TurnResult
		asJSON(8, "pi.wait", map[string]any{"id": kid.ID, "timeout_s": 5}, &res)
		waited <- res
	}()
	if op := <-log.arrived; op != "wait" {
		t.Fatalf("the route got %q", op)
	}
	// The route registers the wait right after the request arrives.
	time.Sleep(50 * time.Millisecond)
	kp.reply("second result")
	if res := <-waited; res.ID != kid.ID || res.Status != wsagent.TurnDone || res.Message != "second result" || res.Delivered {
		t.Fatalf("the wait = %+v", res)
	}

	// transcript: 200 with the ring's records.
	var transcript []map[string]any
	asJSON(9, "pi.transcript", map[string]any{"id": kid.ID}, &transcript)
	if a := log.last(t, "transcript"); a.code != http.StatusOK || !strings.Contains(a.body, `"records":[`) {
		t.Fatalf("transcript = %d %s", a.code, a.body)
	}
	if len(transcript) == 0 {
		t.Fatalf("pi.transcript is empty")
	}

	// stop: 200 with the closed child; the list shows it closed.
	var stopped piChildOut
	asJSON(10, "pi.stop", map[string]any{"id": kid.ID}, &stopped)
	if a := log.last(t, "stop"); a.code != http.StatusOK || stopped.Status != wsagent.StatusClosed {
		t.Fatalf("stop = %d %s (%+v)", a.code, a.body, stopped)
	}
	asJSON(11, "pi.list", nil, &listed)
	if len(listed) != 1 || listed[0].Status != wsagent.StatusClosed {
		t.Fatalf("pi.list after the stop = %+v", listed)
	}

	// An id that is not the holder's own child: 404.
	if out := callToolWithKey(t, s, 12, key, "pi.wait", map[string]any{"id": w.ID}); !strings.Contains(out, "is not one of this conversation's Pi subagents") {
		t.Fatalf("a wait on a foreign id = %q", out)
	}
	if a := log.last(t, "wait"); a.code != http.StatusNotFound {
		t.Fatalf("a foreign id = %d %s", a.code, a.body)
	}

	// A token the runner did not issue: 401.
	resp, err := http.Post(daemon.URL+wsagent.RoutePrefix+"/by-token/list", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token = %d", resp.StatusCode)
	}
}
