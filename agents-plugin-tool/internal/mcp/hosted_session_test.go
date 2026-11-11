package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// hostedHarnessPID stands for the harness process of a hosted session in
// these tests.
const hostedHarnessPID = 4242

// writeHostedRow writes a registration file at path that names hostPID as
// the session's harness process.
func writeHostedRow(t *testing.T, path string, hostPID int) {
	t.Helper()
	raw, err := json.Marshal(wsdashboard.Registration{SchemaVersion: 1, ReplyID: "hostedrow", Hosted: true, HostPID: hostPID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// hostedSessionEnv marks this process's caller as the harness of a session
// the daemon hosts under key: the flag, the key, a registration that names
// the harness process, and that process as the launcher reports it. A
// server resolves it once, at its first hosted-session read, so a test
// builds the server it means to be hosted after this call.
func hostedSessionEnv(t *testing.T, key string) {
	t.Helper()
	row := filepath.Join(t.TempDir(), "registration.json")
	writeHostedRow(t, row, hostedHarnessPID)
	t.Setenv(envHostedSession, "1")
	t.Setenv(envHostedSessionKey, key)
	t.Setenv(envHostedSessionRow, row)
	t.Setenv(envHarnessPID, strconv.Itoa(hostedHarnessPID))
}

func hostedLoginEnv(t *testing.T) (root string, server *Server) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root = t.TempDir()
	initGit(t, root)
	return root, NewServer(root, "test")
}

func TestMintLeadWritesAParentlessLeadKeyAtTheCanonicalRoot(t *testing.T) {
	root, server := hostedLoginEnv(t)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	key, err := NewAgentKeyStore().MintLead(sub + string(filepath.Separator))
	if err != nil {
		t.Fatalf("MintLead: %v", err)
	}
	entry, ok := server.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q has no record", key)
	}
	if entry.scope != roleLead || entry.parent != "" {
		t.Fatalf("record = scope %q parent %q, want a parent-less lead", entry.scope, entry.parent)
	}
	if want := canonicalRootForTest(t, root); entry.root != want {
		t.Fatalf("record root = %q, want the canonical root %q", entry.root, want)
	}

	hostedSessionEnv(t, key)
	before := sessionKeyFileCount(t, server)
	resp := callLogin(t, server, 1, root, map[string]any{"relogin_session_key": key})
	if toolIsError(t, resp) {
		t.Fatalf("hosted re-login with the minted key refused: %s", resp)
	}
	if got, _ := parseLoginResponse(t, resp); got != key {
		t.Fatalf("re-login returned %q, want %q", got, key)
	}
	if after := sessionKeyFileCount(t, server); after != before {
		t.Fatalf("re-login minted a key: before=%d after=%d", before, after)
	}
}

func TestMintLeadRefusesARootOutsideGit(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	if _, err := NewAgentKeyStore().MintLead(t.TempDir()); err == nil {
		t.Fatal("MintLead outside a git worktree should fail")
	}
}

func TestHostedFerruleHoldsTheSessionToItsKey(t *testing.T) {
	root, server := hostedLoginEnv(t)
	hosted, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	otherLead, _ := parseLoginResponse(t, callLogin(t, NewServer(root, "test"), 1, root, nil))
	hostedSessionEnv(t, hosted)
	before := sessionKeyFileCount(t, server)

	// A different (otherwise valid) lead key is refused, naming the hosted key.
	resp := callLogin(t, server, 2, root, map[string]any{"relogin_session_key": otherLead})
	if !toolIsError(t, resp) {
		t.Fatalf("re-login with another key should be refused: %s", resp)
	}
	text := toolText(t, resp)
	for _, want := range []string{hosted, envHostedSessionKey, otherLead} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal %q should name %q", text, want)
		}
	}

	// The same key is accepted.
	resp = callLogin(t, server, 3, root, map[string]any{"relogin_session_key": hosted})
	if got, _ := parseLoginResponse(t, resp); got != hosted {
		t.Fatalf("same-key re-login returned %q, want %q", got, hosted)
	}

	// A parent-less lead bootstrap returns the hosted key and mints nothing.
	resp = callLogin(t, server, 4, root, nil)
	if got, _ := parseLoginResponse(t, resp); got != hosted {
		t.Fatalf("hosted top-lead bootstrap returned %q, want %q", got, hosted)
	}
	if after := sessionKeyFileCount(t, server); after != before {
		t.Fatalf("hosted calls so far minted keys: before=%d after=%d", before, after)
	}

	// Child mints stay fresh: parent-carrying, delegate, leaf.
	seen := map[string]bool{hosted: true}
	for i, extra := range []map[string]any{
		{"parent_session_key": hosted},
		{"capability": "delegate"},
		{"capability": "leaf"},
	} {
		key, _ := parseLoginResponse(t, callLogin(t, server, 10+i, root, extra))
		if seen[key] {
			t.Fatalf("mint %v returned a reused key %q", extra, key)
		}
		seen[key] = true
	}
	if after := sessionKeyFileCount(t, server); after != before+3 {
		t.Fatalf("child mints: before=%d after=%d, want +3", before, after)
	}
}

func TestHostedFerruleWithAnUnusableKeyMintsNothing(t *testing.T) {
	root, _ := hostedLoginEnv(t)
	otherRoot := t.TempDir()
	initGit(t, otherRoot)
	elsewhere, err := NewAgentKeyStore().MintLead(otherRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, key, want string }{
		{"unknown", "never-minted-key", "is not a known session key"},
		{"other-root", elsewhere, "is bound to another root"},
	} {
		hostedSessionEnv(t, tc.key)
		server := NewServer(root, "test")
		before := sessionKeyFileCount(t, server)
		resp := callLogin(t, server, 1, root, nil)
		if !toolIsError(t, resp) {
			t.Fatalf("%s: bootstrap should fail, got %s", tc.name, resp)
		}
		text := toolText(t, resp)
		for _, want := range []string{tc.key, envHostedSessionKey, tc.want} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s: error %q should contain %q", tc.name, text, want)
			}
		}
		if after := sessionKeyFileCount(t, server); after != before {
			t.Fatalf("%s: failed bootstrap minted a key: before=%d after=%d", tc.name, before, after)
		}
	}
}

func TestHostedFlagWithoutAKeyChangesNothing(t *testing.T) {
	root, server := hostedLoginEnv(t)
	hostedSessionEnv(t, "  ")
	before := sessionKeyFileCount(t, server)
	first, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	second, _ := parseLoginResponse(t, callLogin(t, server, 2, root, nil))
	if first == second {
		t.Fatalf("two plain mints returned the same key %q", first)
	}
	if after := sessionKeyFileCount(t, server); after != before+2 {
		t.Fatalf("plain mints: before=%d after=%d, want +2", before, after)
	}
}

func TestUnhostedFerruleMintsAndRelogsInAsBefore(t *testing.T) {
	root, server := hostedLoginEnv(t)
	t.Setenv(envHostedSession, "")
	t.Setenv(envHostedSessionKey, "")
	first, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	second, _ := parseLoginResponse(t, callLogin(t, server, 2, root, nil))
	if first == second {
		t.Fatalf("two plain mints returned the same key %q", first)
	}
	if got, _ := parseLoginResponse(t, callLogin(t, server, 3, root, map[string]any{"relogin_session_key": first})); got != first {
		t.Fatalf("re-login returned %q, want %q", got, first)
	}
}

func TestHostedWorkflowManualHoldsTheSessionToItsKey(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	hosted, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	otherLead, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	child, _ := parseLoginResponse(t, callLogin(t, s, 3, root, map[string]any{"parent_session_key": hosted}))
	hostedSessionEnv(t, hosted)
	s = NewServer(root, "test")
	before := sessionKeyFileCount(t, s)

	for i, tc := range []struct {
		name string
		key  string
		args map[string]any
	}{
		{"other-lead", otherLead, nil},
		{"unknown", "never-minted-key", nil},
		{"sentinel", freshBootstrapKey, nil},
		{"sentinel-with-root", freshBootstrapKey, map[string]any{"root": root}},
	} {
		args := map[string]any{"session_key": tc.key}
		for k, v := range tc.args {
			args[k] = v
		}
		resp := callToolOnce(t, s, 10+i, "workflow_manual", args)
		if !toolIsError(t, resp) {
			t.Fatalf("%s: workflow_manual should be refused: %s", tc.name, resp)
		}
		text := toolText(t, resp)
		if !strings.Contains(text, hosted) || !strings.Contains(text, envHostedSessionKey) {
			t.Fatalf("%s: refusal %q should name the hosted key", tc.name, text)
		}
	}
	if after := sessionKeyFileCount(t, s); after != before {
		t.Fatalf("refused manuals minted a key: before=%d after=%d", before, after)
	}

	for i, key := range []string{hosted, child} {
		resp := callToolOnce(t, s, 20+i, "workflow_manual", map[string]any{"session_key": key})
		if toolIsError(t, resp) {
			t.Fatalf("workflow_manual(%q) refused: %s", key, resp)
		}
		if text := toolText(t, resp); !strings.Contains(text, "## Session Key\n"+key) {
			t.Fatalf("workflow_manual(%q) should render the manual for it", key)
		}
	}
}

func TestHostedSessionLeavesTheRegistrationToTheDaemon(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	if !s.DashboardSettings().Enabled {
		t.Fatal("precondition: the ws dashboard manifest enables the dashboard")
	}
	// A row another session left long past retention, which a registering
	// server would prune.
	stale := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID("stale-other-key"), Project: root, HeartbeatAt: dashboardStamp(time.Now().Add(-30 * 24 * time.Hour))}
	if err := wsdashboard.WriteRegistration(dir, stale); err != nil {
		t.Fatal(err)
	}

	hostedSessionEnv(t, key)
	s = NewServer(root, "test")
	s.sessionHarness = "claude"
	if !s.DashboardSettings().Enabled {
		t.Fatal("the hosted flag must not turn the dashboard settings off")
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("a hosted session's keyed call must not register it")
	}

	daemonRow := wsdashboard.Registration{SchemaVersion: 1, ReplyID: wsmailbox.ReplyID(key), Project: loginRoot, Harness: "claude", HeartbeatAt: dashboardStamp(time.Now().Add(-time.Hour)), Hosted: true, HostPID: hostedHarnessPID}
	if err := wsdashboard.WriteRegistration(dir, daemonRow); err != nil {
		t.Fatal(err)
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if resp := dashCall(t, s, "config.tune", map[string]any{"session_key": key, "key": DashboardEnabledKey, "value": "false", "scope": "session"}); toolIsError(t, resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	s.dashboardHeartbeat()
	s.DashboardEnd()
	got, ok := readRegistration(t, dir, key)
	if !ok {
		t.Fatal("a hosted session's keyed call removed the daemon's registration")
	}
	if got != daemonRow {
		t.Fatalf("the daemon's registration was rewritten:\n got %+v\nwant %+v", got, daemonRow)
	}
	if _, err := wsdashboard.ReadRegistration(dir, stale.ReplyID); err != nil {
		t.Fatalf("a hosted session's server pruned the registry: %v", err)
	}
}

// TestHostedSessionNudgesTheDaemonAndNamesItsPage: the daemon registered
// the session, so its server writes no row, yet a store change still
// reaches the daemon and whiteboard.update names the page the daemon
// serves.
func TestHostedSessionNudgesTheDaemonAndNamesItsPage(t *testing.T) {
	dir := dashboardSessionEnv(t)
	fake := startFakeDaemon(t, dir, http.StatusNoContent, false)
	ep, err := wsdashboard.ReadEndpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := dashboardGitRoot(t)
	_, key, loginRoot := dashboardSessionServer(t, root, true)
	replyID := wsmailbox.ReplyID(key)
	hostedSessionEnv(t, key)
	s := NewServer(root, "test")

	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	waitForNudge(t, fake, replyID)
	if got := fake.received()[0]; got != (wsdashboard.NudgeBody{ReplyID: replyID, Project: loginRoot}) {
		t.Fatalf("nudge = %+v, want the session's reply-id and root", got)
	}
	waitFor(t, "the nudge's answer", func() bool { _, ok := s.dashboardPageURL(key); return ok })
	// Only the first keyed call nudges by itself; a store change is the next.
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "k", "title": "x"})
	waitFor(t, "the nudge after a store change", func() bool { return len(fake.received()) == 2 })
	resp := dashCall(t, s, "whiteboard.update", map[string]any{"session_key": key, "commands": ""})
	if isToolError(resp) {
		t.Fatalf("whiteboard.update failed: %s", resp)
	}
	if got, want := resultLine(t, toolText(t, resp), "full: "), fmt.Sprintf("http://127.0.0.1:%d/s/%s", ep.Port, replyID); got != want {
		t.Fatalf("full: = %q, want the daemon's page %q", got, want)
	}
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("a hosted session's server wrote a registration")
	}
}

// hostedCaller builds a server for a caller whose environment carries the
// hosted-session variables in vars (an empty value leaves one out), and
// returns it with what it warned.
func hostedCaller(t *testing.T, vars map[string]string) (*Server, *bytes.Buffer) {
	t.Helper()
	env := []string{"WS_CACHE_HOME=" + filepath.Join(t.TempDir(), "cache")}
	for name, value := range vars {
		if value != "" {
			env = append(env, name+"="+value)
		}
	}
	diag := &bytes.Buffer{}
	return NewServerWithOptions(ServerOptions{Root: t.TempDir(), Version: "test", Env: env, Diag: diag}), diag
}

// hostedRowPace shortens the wait for a pending registration.
func hostedRowPace(t *testing.T, poll, wait time.Duration) {
	t.Helper()
	oldPoll, oldWait := hostedRowPoll, hostedRowWait
	hostedRowPoll, hostedRowWait = poll, wait
	t.Cleanup(func() { hostedRowPoll, hostedRowWait = oldPoll, oldWait })
}

// TestHostedFlagHoldsOnlyForTheHarnessTheRegistrationNames: the variables
// reach every process below the hosted harness, so a server honours them
// only when its own harness is the registration's host_pid.
func TestHostedFlagHoldsOnlyForTheHarnessTheRegistrationNames(t *testing.T) {
	// A wait would outlast the test: every case here answers at once.
	hostedRowPace(t, 10*time.Millisecond, time.Minute)
	row := filepath.Join(t.TempDir(), "registration.json")
	writeHostedRow(t, row, hostedHarnessPID)
	self := strconv.Itoa(hostedHarnessPID)
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"the harness the row names", map[string]string{envHostedSession: "1", envHostedSessionKey: " hosted-key ", envHostedSessionRow: row, envHarnessPID: self}, "hosted-key"},
		{"another harness below it", map[string]string{envHostedSession: "1", envHostedSessionKey: "hosted-key", envHostedSessionRow: row, envHarnessPID: "5555"}, ""},
		{"no registration path", map[string]string{envHostedSession: "1", envHostedSessionKey: "hosted-key", envHarnessPID: self}, ""},
		{"no harness PID from a launcher", map[string]string{envHostedSession: "1", envHostedSessionKey: "hosted-key", envHostedSessionRow: row}, ""},
		{"a harness PID that is no PID", map[string]string{envHostedSession: "1", envHostedSessionKey: "hosted-key", envHostedSessionRow: row, envHarnessPID: "claude"}, ""},
		{"no key", map[string]string{envHostedSession: "1", envHostedSessionRow: row, envHarnessPID: self}, ""},
		{"the flag off", map[string]string{envHostedSession: "0", envHostedSessionKey: "hosted-key", envHostedSessionRow: row, envHarnessPID: self}, ""},
	} {
		s, diag := hostedCaller(t, tc.vars)
		start := time.Now()
		if got := s.hostedSessionKey(); got != tc.want {
			t.Fatalf("%s: hosted key = %q, want %q", tc.name, got, tc.want)
		}
		if waited := time.Since(start); waited > 10*time.Second {
			t.Fatalf("%s: waited %s for a registration that answers at once", tc.name, waited)
		}
		if diag.Len() != 0 {
			t.Fatalf("%s: warned %q; only a registration that never names a harness warns", tc.name, diag.String())
		}
	}
}

// TestHostedFlagWaitsForTheRegistrationToNameAHarness: the daemon writes
// host_pid right after it starts the harness, so a server that reads none
// yet (or no file yet) waits for it; one that never reads one serves an
// ordinary session, warns once, and stays ordinary.
func TestHostedFlagWaitsForTheRegistrationToNameAHarness(t *testing.T) {
	vars := func(row string) map[string]string {
		return map[string]string{envHostedSession: "1", envHostedSessionKey: "hosted-key", envHostedSessionRow: row, envHarnessPID: strconv.Itoa(hostedHarnessPID)}
	}
	hostedRowPace(t, 5*time.Millisecond, time.Minute)
	for _, tc := range []struct {
		name    string
		pending func(t *testing.T, row string)
	}{
		{"host_pid zero", func(t *testing.T, row string) { writeHostedRow(t, row, 0) }},
		{"no file yet", func(*testing.T, string) {}},
		{"a file that does not parse", func(t *testing.T, row string) {
			if err := os.WriteFile(row, []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		row := filepath.Join(t.TempDir(), "registration.json")
		tc.pending(t, row)
		s, diag := hostedCaller(t, vars(row))
		resolved := make(chan string, 1)
		go func() { resolved <- s.hostedSessionKey() }()
		select {
		case got := <-resolved:
			t.Fatalf("%s: resolved %q before the registration named a harness", tc.name, got)
		case <-time.After(100 * time.Millisecond):
		}
		writeHostedRow(t, row, hostedHarnessPID)
		select {
		case got := <-resolved:
			if got != "hosted-key" {
				t.Fatalf("%s: hosted key = %q once the registration names this harness", tc.name, got)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: never resolved after the registration was written", tc.name)
		}
		if diag.Len() != 0 {
			t.Fatalf("%s: warned %q", tc.name, diag.String())
		}
	}

	// Pending, then another harness: ordinary, with no warning.
	row := filepath.Join(t.TempDir(), "registration.json")
	writeHostedRow(t, row, 0)
	s, diag := hostedCaller(t, vars(row))
	resolved := make(chan string, 1)
	go func() { resolved <- s.hostedSessionKey() }()
	time.Sleep(50 * time.Millisecond)
	writeHostedRow(t, row, 5555)
	select {
	case got := <-resolved:
		if got != "" || diag.Len() != 0 {
			t.Fatalf("another harness's registration: key %q, warned %q", got, diag.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("never resolved after the registration named another harness")
	}

	// Never written: ordinary after the wait, one warning, and no second
	// look once the registration does name this harness.
	hostedRowPace(t, 5*time.Millisecond, 60*time.Millisecond)
	row = filepath.Join(t.TempDir(), "registration.json")
	writeHostedRow(t, row, 0)
	s, diag = hostedCaller(t, vars(row))
	if got := s.hostedSessionKey(); got != "" {
		t.Fatalf("hosted key = %q for a registration that names no harness", got)
	}
	warning := diag.String()
	if strings.Count(warning, "\n") != 1 || !strings.Contains(warning, envHostedSession) || !strings.Contains(warning, row) {
		t.Fatalf("one warning that names the flag and the registration, got %q", warning)
	}
	writeHostedRow(t, row, hostedHarnessPID)
	if got := s.hostedSessionKey(); got != "" {
		t.Fatalf("a server that resolved as ordinary became hosted: %q", got)
	}
	if diag.String() != warning {
		t.Fatalf("warned again: %q", diag.String())
	}
}

// TestHostedVariablesUnderAnotherHarnessLeaveAnOrdinarySession: a second
// harness started inside a hosted session inherits its variables; its
// server mints, re-logs in, and registers as any session's does.
func TestHostedVariablesUnderAnotherHarnessLeaveAnOrdinarySession(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	_, hosted, _ := dashboardSessionServer(t, root, true)
	hostedSessionEnv(t, hosted)
	t.Setenv(envHarnessPID, "5555")
	s := NewServer(root, "test")
	s.sessionHarness = "claude"

	own, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	if own == hosted {
		t.Fatalf("the nested harness's bootstrap returned the hosted key %q", hosted)
	}
	other, _ := parseLoginResponse(t, callLogin(t, NewServer(root, "test"), 2, root, nil))
	if got, _ := parseLoginResponse(t, callLogin(t, s, 3, root, map[string]any{"relogin_session_key": other})); got != other {
		t.Fatalf("re-login with another key returned %q, want %q", got, other)
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": own})
	if _, ok := readRegistration(t, dir, own); !ok {
		t.Fatal("the nested harness's session did not register itself")
	}
}

// TestDropHarnessSessionEnvLeavesNoSessionToInherit: the daemon's own
// environment came from one session's launcher; after the drop, a server
// reading it, or a process spawned from it, sees no hosted session.
func TestDropHarnessSessionEnvLeavesNoSessionToInherit(t *testing.T) {
	hostedSessionEnv(t, "hosted-key")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	if got := NewServer(t.TempDir(), "test").hostedSessionKey(); got != "hosted-key" {
		t.Fatalf("precondition: hosted key = %q", got)
	}
	DropHarnessSessionEnv()
	for _, name := range []string{envHostedSession, envHostedSessionKey, envHostedSessionRow, envHarnessPID} {
		if value, ok := os.LookupEnv(name); ok {
			t.Fatalf("%s still set to %q", name, value)
		}
	}
	if got := NewServer(t.TempDir(), "test").hostedSessionKey(); got != "" {
		t.Fatalf("hosted key after the drop = %q", got)
	}
}
