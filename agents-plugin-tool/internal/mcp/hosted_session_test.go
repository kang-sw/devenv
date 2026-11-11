package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// hostedSessionEnv marks this process's caller as a session the daemon hosts under
// key.
func hostedSessionEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(envHostedSession, "1")
	t.Setenv(envHostedSessionKey, key)
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
	otherLead, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
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
	root, server := hostedLoginEnv(t)
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
	if !s.DashboardSettings().Enabled {
		t.Fatal("the hosted flag must not turn the dashboard settings off")
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("a hosted session's keyed call must not register it")
	}

	daemonRow := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(key), Project: loginRoot, Harness: "claude", HeartbeatAt: dashboardStamp(time.Now().Add(-time.Hour))}
	if err := wsdashboard.WriteRegistration(dir, daemonRow); err != nil {
		t.Fatal(err)
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if resp := dashCall(t, s, "config.tune", map[string]any{"session_key": key, "key": DashboardEnabledKey, "value": "false", "scope": "session"}); toolIsError(t, resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	got, ok := readRegistration(t, dir, key)
	if !ok {
		t.Fatal("a hosted session's keyed call removed the daemon's registration")
	}
	if got.HeartbeatAt != daemonRow.HeartbeatAt {
		t.Fatalf("the daemon's registration was rewritten: heartbeat %q, want %q", got.HeartbeatAt, daemonRow.HeartbeatAt)
	}
	if _, err := wsdashboard.ReadRegistration(dir, stale.ReplyID); err != nil {
		t.Fatalf("a hosted session's server pruned the registry: %v", err)
	}
}
