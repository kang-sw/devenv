package mcp

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

var anthropicToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// agyServerPrefix is what agy puts before a ws tool name it forwards.
const agyServerPrefix = "mcp_ws_"

func canonicalToolNames() []string {
	var names []string
	for _, tool := range tools() {
		if name, _ := tool["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// TestToolAliasTableRoundTrip: the sanitize scheme spells every ws tool in
// a name the Anthropic API accepts, one-to-one (the builder refuses
// otherwise), and maps each back to its canonical name; a canonical name
// passes the reverse map unchanged.
func TestToolAliasTableRoundTrip(t *testing.T) {
	names := canonicalToolNames()
	table, err := buildToolAliasTable(wsagent.ToolNameSchemeSanitize, names)
	if err != nil {
		t.Fatalf("the real tool inventory has no alias collision: %v", err)
	}
	if got := table.advertised("tickets.query"); got != "tickets_query" {
		t.Fatalf("tickets.query is advertised as %q", got)
	}
	for _, name := range names {
		alias := table.advertised(name)
		if !anthropicToolName.MatchString(alias) {
			t.Errorf("%q is advertised as %q, which the API refuses", name, alias)
		}
		// agy forwards the tool as mcp_ws_<alias>; that must fit too.
		if len(agyServerPrefix+alias) > 128 {
			t.Errorf("%q as forwarded by agy is %d characters", name, len(agyServerPrefix+alias))
		}
		if back := table.canonical(alias); back != name {
			t.Errorf("%q -> %q -> %q does not round trip", name, alias, back)
		}
		if same := table.canonical(name); strings.Contains(name, ".") && same != name {
			t.Errorf("canonical name %q mapped to %q", name, same)
		}
	}
}

// TestToolAliasTableIdentity: the empty scheme builds no table, and a nil
// table leaves every name as it is.
func TestToolAliasTableIdentity(t *testing.T) {
	table, err := buildToolAliasTable("", canonicalToolNames())
	if err != nil || table != nil {
		t.Fatalf("identity scheme: table %v, err %v", table, err)
	}
	if table.advertised("tickets.query") != "tickets.query" || table.canonical("tickets.query") != "tickets.query" {
		t.Fatal("a nil table must be the identity")
	}
}

// TestToolAliasTableRefusesCollisions: two names that share an alias, an
// alias that is another tool's canonical name, and an unknown scheme are
// refused when the table is built.
func TestToolAliasTableRefusesCollisions(t *testing.T) {
	for name, names := range map[string][]string{
		"shared alias":          {"a.b", "a_b"},
		"shared alias reversed": {"a_b", "a.b"},
	} {
		if _, err := buildToolAliasTable(wsagent.ToolNameSchemeSanitize, names); err == nil {
			t.Errorf("%s: %v built a table", name, names)
		}
	}
	if _, err := buildToolAliasTable("nonesuch", []string{"a.b"}); err == nil {
		t.Error("an unknown scheme built a table")
	}
}

// hostedBackendConnection is the hosted lead connection of a project whose
// hosted key's Runner record names backend, served under a runner that
// registers regs. The server for another harness is built the same way.
func hostedBackendConnection(t *testing.T, backend string, regs map[string]wsagent.BackendRegistration) (*Server, *hostedKeyFixture) {
	t.Helper()
	f := newHostedKeyFixture(t)
	dir, err := NewServer(f.root, "test").dashboard.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	rec := wsagent.Record{
		ReplyID: wsmailbox.ReplyID(f.hosted), WSKey: f.hosted, Root: f.root, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Backend: backend, Lead: true,
	}
	if err := wsagent.WriteRecordForTest(dir, rec); err != nil {
		t.Fatal(err)
	}
	s := f.connect(t)
	s.pi.agents = wsagent.NewRunner(wsagent.Options{Backends: regs})
	return s, f
}

func descriptorRegistrations() map[string]wsagent.BackendRegistration {
	return map[string]wsagent.BackendRegistration{
		wsagent.BackendClaude: {LeadProfile: wsagent.LeadProfileClaude, ManualHarness: "claude"},
		wsagent.BackendPi:     {LeadProfile: wsagent.LeadProfilePi},
		wsagent.BackendAgy: {LeadProfile: wsagent.LeadProfileAgy, PermissionModes: wsagent.AgyPermissionModes,
			ToolNameScheme: wsagent.ToolNameSchemeSanitize, ManualHarness: wsagent.BackendAgy, PlaybookCommands: true},
	}
}

func listedToolNames(t *testing.T, s *Server) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, tool := range s.filteredTools() {
		out[tool["name"].(string)] = true
	}
	return out
}

// TestHostedAgyConnectionSeesAliases: an agy lead's connection lists every
// dotted ws tool under its alias only, accepts a call by alias (the omitted
// session_key still fills in, so the alias is mapped before the key rule),
// still accepts the canonical name, and renders playbooks under harness
// agy.
func TestHostedAgyConnectionSeesAliases(t *testing.T) {
	s, f := hostedBackendConnection(t, wsagent.BackendAgy, descriptorRegistrations())

	listed := listedToolNames(t, s)
	if !listed["todo_list"] || listed["todo.list"] {
		t.Fatalf("todo.list must be listed as its alias alone: %v", listed)
	}
	for name := range listed {
		if !anthropicToolName.MatchString(name) {
			t.Errorf("listed tool %q is not an API-legal name", name)
		}
	}

	if resp := callToolOnce(t, s, 1, "todo_list", map[string]any{}); toolIsError(t, resp) || strings.Contains(resp, `"error"`) {
		t.Fatalf("an alias call without session_key: %s", resp)
	}
	if resp := callToolOnce(t, s, 2, "todo.list", map[string]any{}); toolIsError(t, resp) || strings.Contains(resp, `"error"`) {
		t.Fatalf("the canonical name is still accepted: %s", resp)
	}
	resp := callToolOnce(t, s, 3, "workflow_manual", map[string]any{})
	if toolIsError(t, resp) {
		t.Fatalf("workflow_manual: %s", resp)
	}
	if text := toolText(t, resp); !strings.Contains(text, "## Session Key\n"+f.hosted) {
		t.Fatalf("workflow_manual is not the hosted session's: %.200s", text)
	}
	if got := s.playbookHarness(); got != "agy" {
		t.Fatalf("playbook harness %q, want agy", got)
	}
	if !s.connectionDescriptor().playbookCommands || s.runtimeNamespace() != wsagent.ReservedMCPServer {
		t.Fatal("an agy lead's connection renders for a session without ws skills, under the ws server")
	}
	if got := s.currentHarness(); got != "" {
		t.Fatalf("detected harness %q: the descriptor must not change it", got)
	}
}

// TestHostedIdentityConnectionKeepsDottedNames: a hosted lead whose backend
// declares no tool-name scheme (Claude) lists dotted names and refuses an
// alias like any unknown tool; its manual harness comes from the
// descriptor. A connection with no hosted lead record keeps dotted names
// and no harness, whatever the runner registers.
func TestHostedIdentityConnectionKeepsDottedNames(t *testing.T) {
	s, f := hostedBackendConnection(t, wsagent.BackendClaude, descriptorRegistrations())
	listed := listedToolNames(t, s)
	if !listed["todo.list"] || listed["todo_list"] {
		t.Fatalf("a claude lead lists dotted names: %v", listed)
	}
	if resp := callToolOnce(t, s, 1, "todo_list", map[string]any{}); !strings.Contains(resp, `"error"`) {
		t.Fatalf("an alias must stay unknown on a claude connection: %s", resp)
	}
	if got := s.playbookHarness(); got != "claude" {
		t.Fatalf("playbook harness %q, want claude", got)
	}

	// Not hosted: the connection stays ordinary.
	t.Setenv(envHostedSession, "")
	plain := NewServer(f.root, "test")
	plain.pi.agents = wsagent.NewRunner(wsagent.Options{Backends: descriptorRegistrations()})
	if listed := listedToolNames(t, plain); !listed["todo.list"] || listed["todo_list"] {
		t.Fatalf("an ordinary connection lists dotted names: %v", listed)
	}
	if got := plain.playbookHarness(); got != "" {
		t.Fatalf("an ordinary connection has no declared harness, got %q", got)
	}
}

// TestRenderLeadManualUnderDeclaredHarness: a harness the detection list
// does not know (agy) still renders the lead's manual, and the Claude
// render is unchanged by it.
func TestRenderLeadManualUnderDeclaredHarness(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	key, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"claude", "agy", ""} {
		text, err := RenderLeadManual(os.Environ(), root, HostedRender{Harness: harness}, key)
		if err != nil {
			t.Fatalf("harness %q: %v", harness, err)
		}
		if !strings.Contains(text, "## Session Key\n"+key) {
			t.Fatalf("harness %q: the render is not the manual for %q", harness, key)
		}
	}
}

// TestAgyWorkerConnectionSeesAliases pins worker spelling without granting
// the hosted lead's key substitution or widening the canonical allowlist.
func TestAgyWorkerConnectionSeesAliases(t *testing.T) {
	_, f := hostedBackendConnection(t, wsagent.BackendAgy, descriptorRegistrations())
	t.Setenv(envHostedSession, "")
	t.Setenv(wsagent.EnvAgentSessionKey, f.child)
	t.Setenv(envAllowedTools, "git.status,agent.tiers")
	s := NewServer(f.root, "test")
	s.pi.agents = wsagent.NewRunner(wsagent.Options{Backends: descriptorRegistrations()})
	dir, err := s.dashboard.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	rec := wsagent.Record{
		ReplyID: wsmailbox.ReplyID(f.child), WSKey: f.child, Root: f.root, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Backend: wsagent.BackendAgy, Profile: wsagent.AgentWorkerProfile(wsagent.BackendAgy),
		Parent: &wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: wsmailbox.ReplyID(f.hosted)},
	}
	if err := wsagent.WriteRecordForTest(dir, rec); err != nil {
		t.Fatal(err)
	}
	listed := listedToolNames(t, s)
	if !listed["git_status"] || !listed["agent_tiers"] || listed["git.status"] || listed["git_commit"] || listed["workflow_manual"] {
		t.Fatalf("worker's aliased allowlist: %v", listed)
	}
	if s.hostedSessionKey() != "" {
		t.Fatal("a worker must not become a hosted lead")
	}
	if s.connectionDescriptor().playbookCommands {
		t.Fatal("a worker's connection renders as before: only the lead runs without ws skills")
	}
	if resp := callToolOnce(t, s, 1, "git_status", map[string]any{}); !toolIsError(t, resp) {
		t.Fatalf("a worker still needs an explicit delegate key: %s", resp)
	}
	if resp := callToolOnce(t, s, 2, "git_status", map[string]any{"session_key": f.child}); toolIsError(t, resp) {
		t.Fatalf("worker alias dispatch: %s", resp)
	}
	if resp := callToolOnce(t, s, 3, "git_commit", map[string]any{"session_key": f.child}); !strings.Contains(resp, "not available") {
		t.Fatalf("reverse mapping must not bypass the allowlist: %s", resp)
	}
}

// TestRegistrationToolNameSchemes: the descriptor's scheme names resolve
// to the Pi bridge's spelling, the empty scheme is the identity, and an
// unknown scheme is not silently identity.
func TestRegistrationToolNameSchemes(t *testing.T) {
	if name, ok := wsagent.ApplyToolNameScheme(wsagent.ToolNameSchemeSanitize, "tickets.sage_stamp"); !ok || name != "tickets_sage_stamp" {
		t.Fatalf("sanitize: %q %v", name, ok)
	}
	if name, ok := wsagent.ApplyToolNameScheme("", "tickets.query"); !ok || name != "tickets.query" {
		t.Fatalf("identity: %q %v", name, ok)
	}
	if _, ok := wsagent.ApplyToolNameScheme("nonesuch", "x"); ok {
		t.Fatal("an unknown scheme must not apply")
	}
}
