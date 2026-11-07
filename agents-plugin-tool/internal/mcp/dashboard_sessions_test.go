package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// dashboardSessionEnv isolates the cache (and so the session registry and the
// daemon directory) and config homes, and returns the daemon directory.
func dashboardSessionEnv(t *testing.T) string {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	t.Setenv("WS_SKILLS_ROOT", filepath.Join(t.TempDir(), "missing-skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func dashboardGitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	return root
}

// dashboardSessionServer builds a server whose launcher listed the ws
// manifest beside the shipped ws (enabled) or wsflow (disabled) dashboard
// manifest, and logs a top-level lead session in at root.
func dashboardSessionServer(t *testing.T, root string, enabled bool) (s *Server, key, loginRoot string) {
	t.Helper()
	dashboard := dashboardPackages[0].dashboard
	if !enabled {
		dashboard = dashboardPackages[1].dashboard
	}
	t.Setenv(envConfigManifests, claudeManifestPath+";"+dashboard)
	s = NewServer(root, "test")
	s.sessionHarness = "claude"
	key, loginRoot = parseLoginResponse(t, callLogin(t, s, 900000, root, nil))
	return s, key, loginRoot
}

// dashCall runs one tool call through the request handler without a serve
// loop, whose end would mark the session ended.
func dashCall(t *testing.T, s *Server, name string, args map[string]any) string {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.handle(context.Background(), request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
	line, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}

func readRegistration(t *testing.T, dir, key string) (wsdashboard.Registration, bool) {
	t.Helper()
	reg, err := wsdashboard.ReadRegistration(dir, wsmailbox.ReplyID(key))
	if os.IsNotExist(err) {
		return wsdashboard.Registration{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return reg, true
}

// fakeDaemon records the nudges it receives and answers with status, or
// blocks every request until the test ends when hang is set.
type fakeDaemon struct {
	mu     sync.Mutex
	nudges []wsdashboard.NudgeBody
}

func (f *fakeDaemon) received() []wsdashboard.NudgeBody {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wsdashboard.NudgeBody(nil), f.nudges...)
}

func startFakeDaemon(t *testing.T, dir string, status int, hang bool) *fakeDaemon {
	t.Helper()
	fake := &fakeDaemon{}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hang {
			<-release
			return
		}
		var body wsdashboard.NudgeBody
		_ = json.NewDecoder(r.Body).Decode(&body)
		fake.mu.Lock()
		fake.nudges = append(fake.nudges, body)
		fake.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	_, portText, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portText)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf(`{"schema_version": 1, "port": %d, "pid": 1, "version": "test", "token": "t"}`, port)
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), []byte(endpoint), 0o600); err != nil {
		t.Fatal(err)
	}
	return fake
}

func waitForNudge(t *testing.T, fake *fakeDaemon, replyID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, n := range fake.received() {
			if n.ReplyID == replyID {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no nudge for %s; received %+v", replyID, fake.received())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDashboardRegistersATopLevelLeadSessionWithTheKeyOn(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	s.dashboard.now = func() time.Time { return now }

	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("registered before any keyed call")
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	reg, ok := readRegistration(t, dir, key)
	if !ok {
		t.Fatal("keyed call did not register the session")
	}
	if reg.ReplyID != wsmailbox.ReplyID(key) || reg.Project != loginRoot || reg.Harness != "claude" || reg.HeartbeatAt != dashboardStamp(now) || !reg.Live(now) {
		t.Fatalf("registration = %+v", reg)
	}
	raw, err := os.ReadFile(filepath.Join(wsdashboard.SessionDir(dir, reg.ReplyID), "registration.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), key) {
		t.Fatalf("registration file carries the session key: %s", raw)
	}

	// A spawned (child) session is never registered.
	child, _ := parseLoginResponse(t, callLogin(t, s, 900001, root, map[string]any{"parent_session_key": key}))
	dashCall(t, s, "todo.list", map[string]any{"session_key": child})
	if _, ok := readRegistration(t, dir, child); ok {
		t.Fatal("a child session was registered")
	}
}

func TestDashboardKeyOffAndKeyOnServersOnOneRoot(t *testing.T) {
	dir := dashboardSessionEnv(t)
	fake := startFakeDaemon(t, dir, http.StatusNoContent, false)
	root := dashboardGitRoot(t)
	sOff, keyOff, _ := dashboardSessionServer(t, root, false)
	sOn, keyOn, loginRoot := dashboardSessionServer(t, root, true)

	for _, c := range []struct {
		s   *Server
		key string
	}{{sOff, keyOff}, {sOn, keyOn}} {
		resp := dashCall(t, c.s, "todo.add", map[string]any{"session_key": c.key, "key": "step", "title": "a step"})
		if isToolError(resp) {
			t.Fatalf("todo.add: %s", resp)
		}
	}
	waitForNudge(t, fake, wsmailbox.ReplyID(keyOn))
	// The key-off session's calls queued nothing; the key-on nudges arrived,
	// so a key-off nudge sent at the same time would have too.
	time.Sleep(100 * time.Millisecond)
	for _, n := range fake.received() {
		if n.ReplyID != wsmailbox.ReplyID(keyOn) || n.Project != loginRoot {
			t.Fatalf("unexpected nudge %+v", n)
		}
	}
	if _, ok := readRegistration(t, dir, keyOff); ok {
		t.Fatal("the key-off session was registered")
	}

	// The daemon's index, read through the real session source, lists only
	// the key-on session, even though it has no board yet.
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: NewDashboardSessionSource()})
	idx.Refresh()
	sessions := idx.Sessions()
	if len(sessions) != 1 || sessions[0].ReplyID != wsmailbox.ReplyID(keyOn) || sessions[0].HasBoard || !sessions[0].Live {
		t.Fatalf("index = %+v", sessions)
	}

	// A board write is seen on the next re-read of that session.
	resp := dashCall(t, sOn, "whiteboard.update", map[string]any{"session_key": keyOn, "commands": `<wb:add class="technical" topic="t"><wb:title>a question</wb:title></wb:add>`})
	if isToolError(resp) {
		t.Fatalf("whiteboard.update: %s", resp)
	}
	if !idx.Nudge(wsmailbox.ReplyID(keyOn), loginRoot) {
		t.Fatal("nudge for the key-on session was ignored")
	}
	if s, _ := idx.Session(wsmailbox.ReplyID(keyOn)); !s.HasBoard || s.Rev != 2 {
		t.Fatalf("session after board write = %+v", s)
	}
}

func TestDashboardNudgeFailureLeavesTheToolResultUnchanged(t *testing.T) {
	old := dashboardNudgeTimeout
	dashboardNudgeTimeout = 3 * time.Second
	t.Cleanup(func() { dashboardNudgeTimeout = old })

	run := func(t *testing.T, mode string) string {
		dir := dashboardSessionEnv(t)
		var fake *fakeDaemon
		switch mode {
		case "ok":
			fake = startFakeDaemon(t, dir, http.StatusNoContent, false)
		case "error":
			fake = startFakeDaemon(t, dir, http.StatusInternalServerError, false)
		case "hung":
			startFakeDaemon(t, dir, 0, true)
		}
		root := dashboardGitRoot(t)
		s, key, loginRoot := dashboardSessionServer(t, root, true)
		started := time.Now()
		resp := dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "step", "title": "a step"})
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("%s: todo.add took %v; the nudge must not delay it", mode, elapsed)
		}
		if isToolError(resp) {
			t.Fatalf("%s: todo.add failed: %s", mode, resp)
		}
		if fake != nil {
			waitForNudge(t, fake, wsmailbox.ReplyID(key))
			for _, n := range fake.received() {
				if n.Project != loginRoot {
					t.Fatalf("nudge project = %q, want %q", n.Project, loginRoot)
				}
			}
		}
		return toolText(t, resp)
	}
	var want string
	t.Run("no daemon", func(t *testing.T) { want = run(t, "none") })
	for _, mode := range []string{"ok", "error", "hung"} {
		t.Run(mode, func(t *testing.T) {
			if got := run(t, mode); got != want {
				t.Fatalf("result with a %s daemon:\n%s\nwant:\n%s", mode, got, want)
			}
		})
	}
}

func TestDashboardSessionScopeTuneOffUnregisters(t *testing.T) {
	dir := dashboardSessionEnv(t)
	fake := startFakeDaemon(t, dir, http.StatusNoContent, false)
	root := dashboardGitRoot(t)
	s, key, _ := dashboardSessionServer(t, root, true)
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if _, ok := readRegistration(t, dir, key); !ok {
		t.Fatal("not registered with the key on")
	}
	waitForNudge(t, fake, wsmailbox.ReplyID(key))

	resp := dashCall(t, s, "config.tune", map[string]any{"session_key": key, "key": DashboardEnabledKey, "value": "false", "scope": "session"})
	if isToolError(resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("a session-scope tune off left the registration")
	}
	before := len(fake.received())
	dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "step", "title": "a step"})
	time.Sleep(100 * time.Millisecond)
	if after := len(fake.received()); after != before {
		t.Fatalf("a key-off session sent %d nudges", after-before)
	}
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("a key-off session re-registered")
	}
}

func TestDashboardHeartbeatIdleCutoffAndEnd(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, _ := dashboardSessionServer(t, root, true)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	s.dashboard.now = func() time.Time { return now }
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	registeredAt := dashboardStamp(now)

	now = now.Add(3 * time.Minute)
	s.dashboardHeartbeat()
	reg, _ := readRegistration(t, dir, key)
	if reg.HeartbeatAt != dashboardStamp(now) || reg.RegisteredAt != registeredAt {
		t.Fatalf("after a heartbeat: %+v", reg)
	}

	// Past the idle cutoff with no keyed call, the heartbeat stops and the
	// session reads as ended once its last heartbeat ages out.
	now = now.Add(dashboardIdleCutoff)
	s.dashboardHeartbeat()
	reg, _ = readRegistration(t, dir, key)
	if reg.HeartbeatAt == dashboardStamp(now) || reg.Live(now) {
		t.Fatalf("idle session still heartbeating: %+v", reg)
	}
	// Its next keyed call makes it live again.
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	reg, _ = readRegistration(t, dir, key)
	if reg.HeartbeatAt != dashboardStamp(now) || !reg.Live(now) || reg.RegisteredAt != registeredAt {
		t.Fatalf("after resuming: %+v", reg)
	}

	// The end of the serve loop marks it ended; a later keyed call (a host
	// that reconnects) clears the mark.
	s.dashboardEnd()
	reg, _ = readRegistration(t, dir, key)
	if reg.EndedAt != dashboardStamp(now) || reg.Live(now) {
		t.Fatalf("after end: %+v", reg)
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	reg, _ = readRegistration(t, dir, key)
	if reg.EndedAt != "" || !reg.Live(now) {
		t.Fatalf("after a call past the end: %+v", reg)
	}
}

func TestDashboardFirstRegistrationPrunesExpiredEntries(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	old := time.Now().Add(-wsdashboard.RegistryRetention - time.Hour).UTC().Format(time.RFC3339)
	if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{ReplyID: "expired", Project: root, HeartbeatAt: old}); err != nil {
		t.Fatal(err)
	}
	s, key, _ := dashboardSessionServer(t, root, true)
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if _, err := os.Stat(wsdashboard.SessionDir(dir, "expired")); !os.IsNotExist(err) {
		t.Fatalf("expired registration kept: %v", err)
	}
	if _, ok := readRegistration(t, dir, key); !ok {
		t.Fatal("not registered")
	}
}
