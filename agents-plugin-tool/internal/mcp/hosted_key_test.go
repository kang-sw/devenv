package mcp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// recordSpawned writes the daemon runner's conversation record of key under
// the dashboard state directory s reads: an agent.spawn session at root
// whose authority parent (Record.Parent) is the session of reply-id parent.
func recordSpawned(t *testing.T, s *Server, root, key, parent, kind string) {
	t.Helper()
	dir, err := s.dashboard.stateDir()
	if err != nil {
		t.Fatal(err)
	}
	rec := wsagent.Record{
		ReplyID: wsmailbox.ReplyID(key), WSKey: key, Root: root, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Backend: wsagent.BackendClaude, Profile: wsagent.AgentWorkerProfile(wsagent.BackendClaude),
		Parent: &wsagent.ParentRef{Kind: kind, ReplyID: parent},
	}
	if err := wsagent.WriteRecordForTest(dir, rec); err != nil {
		t.Fatal(err)
	}
}

// spawnedKey mints a key as the daemon's key store does for an agent.spawn
// child (no key-record parent) and records its conversation under the
// session of parentKey.
func spawnedKey(t *testing.T, s *Server, root, parentKey, kind string) string {
	t.Helper()
	key, err := NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	recordSpawned(t, s, root, key, wsmailbox.ReplyID(parentKey), kind)
	return key
}

// spawnedLeadKey is spawnedKey for a lead-scoped key: workflow_manual is a
// lead-only tool, so only such a descendant reaches its own key check.
func spawnedLeadKey(t *testing.T, s *Server, root, parentKey, kind string) string {
	t.Helper()
	key, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	recordSpawned(t, s, root, key, wsmailbox.ReplyID(parentKey), kind)
	return key
}

// hostedKeyFixture is a project whose hosted lead key the daemon minted,
// with the keys and Runner records around it. Every key is minted before
// connect: a ferrule call on the hosted connection answers with its key.
type hostedKeyFixture struct {
	root, hosted string
	// child and grandchild descend from the hosted session, and so does
	// leadChild, a lead-scoped key; peer is another lead, and peerChild
	// its agent.spawn child.
	child, grandchild, leadChild, peer, peerChild string
	// ferruleChild is a key a ferrule call minted with the hosted key as
	// its parent: a key-record child with no Runner record.
	ferruleChild string
}

func newHostedKeyFixture(t *testing.T) *hostedKeyFixture {
	t.Helper()
	setupWorkflowManualNoteEnv(t)
	t.Setenv(envHostedSession, "")
	t.Setenv(envHostedSessionKey, "")
	f := &hostedKeyFixture{root: t.TempDir()}
	initGit(t, f.root)
	var err error
	if f.hosted, err = NewAgentKeyStore().MintLead(f.root); err != nil {
		t.Fatal(err)
	}
	if f.peer, err = NewAgentKeyStore().MintLead(f.root); err != nil {
		t.Fatal(err)
	}
	s := NewServer(f.root, "test")
	f.child = spawnedKey(t, s, f.root, f.hosted, wsagent.ParentLead)
	f.grandchild = spawnedKey(t, s, f.root, f.child, wsagent.ParentConversation)
	f.leadChild = spawnedLeadKey(t, s, f.root, f.hosted, wsagent.ParentLead)
	f.peerChild = spawnedKey(t, s, f.root, f.peer, wsagent.ParentLead)
	f.ferruleChild, _ = parseLoginResponse(t, callLogin(t, s, 1, f.root, map[string]any{"parent_session_key": f.hosted}))
	return f
}

// connect is the hosted lead's own connection.
func (f *hostedKeyFixture) connect(t *testing.T) *Server {
	t.Helper()
	hostedSessionEnv(t, f.hosted)
	s := NewServer(f.root, "test")
	if s.hostedSessionKey() != f.hosted {
		t.Fatal("precondition: the connection is the hosted session's")
	}
	return s
}

// TestHostedSessionKeyFillsAnOmittedKey: on a hosted lead connection a
// call that omits session_key (or leaves it blank) runs as the
// connection's own session, for a tool whose schema declares the
// argument; a tool without it (ferrule) gets none, and a non-string value
// is left to the tool.
func TestHostedSessionKeyFillsAnOmittedKey(t *testing.T) {
	f := newHostedKeyFixture(t)
	s := f.connect(t)

	for _, args := range []map[string]any{{}, {"session_key": nil}, {"session_key": ""}, {"session_key": " \t "}} {
		if refusal := s.hostedSessionKeyRule("todo.list", args); refusal != "" || args["session_key"] != f.hosted {
			t.Fatalf("todo.list: refusal %q, session_key %q; want the hosted key filled in", refusal, args["session_key"])
		}
	}
	args := map[string]any{}
	if refusal := s.hostedSessionKeyRule("ferrule", args); refusal != "" || len(args) != 0 {
		t.Fatalf("ferrule declares no session_key: refusal %q, args %v", refusal, args)
	}
	args = map[string]any{"session_key": 7}
	if refusal := s.hostedSessionKeyRule("todo.list", args); refusal != "" || args["session_key"] != 7 {
		t.Fatalf("a non-string session_key is the tool's to answer: refusal %q, args %v", refusal, args)
	}

	if resp := callToolOnce(t, s, 1, "todo.list", map[string]any{}); toolIsError(t, resp) {
		t.Fatalf("todo.list without session_key: %s", resp)
	}
	resp := callToolOnce(t, s, 2, "workflow_manual", map[string]any{})
	if toolIsError(t, resp) {
		t.Fatalf("workflow_manual without session_key: %s", resp)
	}
	if text := toolText(t, resp); !strings.Contains(text, "## Session Key\n"+f.hosted) {
		t.Fatalf("workflow_manual without session_key should render the hosted session's manual")
	}
}

// TestHostedSessionKeyAdmitsItsOwnAndDescendants: on a hosted lead
// connection an explicit key passes when it is the connection's own or
// belongs to a session whose Runner lineage (Record.Parent) reaches the
// hosted session: its agent.spawn child and that child's child. The
// workflow_manual gate admits the same lineage (a lead-scoped descendant:
// the role gate keeps the lead-only tool from delegate keys).
func TestHostedSessionKeyAdmitsItsOwnAndDescendants(t *testing.T) {
	f := newHostedKeyFixture(t)
	s := f.connect(t)
	for i, key := range []string{f.hosted, " " + f.hosted + " ", f.child, f.grandchild, f.leadChild} {
		if refusal := s.hostedSessionKeyRule("todo.list", map[string]any{"session_key": key}); refusal != "" {
			t.Fatalf("session_key %q refused: %s", key, refusal)
		}
		if resp := callToolOnce(t, s, 10+i, "todo.list", map[string]any{"session_key": key}); toolIsError(t, resp) {
			t.Fatalf("todo.list(%q) refused: %s", key, resp)
		}
	}
	for i, key := range []string{f.hosted, " " + f.hosted + " ", f.leadChild} {
		resp := callToolOnce(t, s, 20+i, "workflow_manual", map[string]any{"session_key": key})
		if toolIsError(t, resp) {
			t.Fatalf("workflow_manual(%q) refused: %s", key, resp)
		}
		if text := toolText(t, resp); !strings.Contains(text, "## Session Key\n"+strings.TrimSpace(key)) {
			t.Fatalf("workflow_manual(%q) should render the manual for it", key)
		}
	}
}

// TestHostedSessionKeyRefusesOtherSessions: on a hosted lead connection a
// key outside the hosted session's lineage is refused before the tool
// runs, with an error naming the tool, both keys, the rule, and the
// hosted-key variable: a peer lead's key, a peer's descendant's, an
// unknown key and a lineage that loops without reaching the hosted session.
func TestHostedSessionKeyRefusesOtherSessions(t *testing.T) {
	f := newHostedKeyFixture(t)
	s := NewServer(f.root, "test")
	loopA, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	loopB, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	recordSpawned(t, s, f.root, loopA, wsmailbox.ReplyID(loopB), wsagent.ParentConversation)
	recordSpawned(t, s, f.root, loopB, wsmailbox.ReplyID(loopA), wsagent.ParentConversation)
	s = f.connect(t)
	before := sessionKeyFileCount(t, s)

	id := 1
	for name, key := range map[string]string{
		"peer lead":    f.peer,
		"peer child":   f.peerChild,
		"unknown":      "never-minted-key",
		"lineage loop": loopA,
	} {
		for _, tool := range []string{"todo.list", "workflow_manual"} {
			id++
			resp := callToolOnce(t, s, id, tool, map[string]any{"session_key": key})
			if !toolIsError(t, resp) {
				t.Fatalf("%s: %s should be refused: %s", name, tool, resp)
			}
			text := toolText(t, resp)
			for _, want := range []string{tool + ":", strconv.Quote(key), strconv.Quote(f.hosted), envHostedSessionKey, "descendant sessions"} {
				if !strings.Contains(text, want) {
					t.Fatalf("%s: %s refusal %q should name %q", name, tool, text, want)
				}
			}
		}
	}
	if after := sessionKeyFileCount(t, s); after != before {
		t.Fatalf("refused calls minted keys: before=%d after=%d", before, after)
	}
}

func TestHostedSessionKeyRecordLineage(t *testing.T) {
	f := newHostedKeyFixture(t)
	s := f.connect(t)
	mint := func(parent string, scope toolRole) string {
		t.Helper()
		key, err := s.sessions.mint(f.root, scope, parent)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	renderChild := mint(f.hosted, roleDelegate)
	workerChild := mint(f.hosted, roleLead)
	grandchild := mint(renderChild, roleDelegate)
	atCap := f.hosted
	for i := 0; i < hostedLineageDepth; i++ {
		atCap = mint(atCap, roleDelegate)
	}
	tooDeep := mint(atCap, roleDelegate)
	unrelated := mint(f.peer, roleDelegate)
	for _, key := range []string{f.ferruleChild, renderChild, workerChild, grandchild, atCap} {
		if refusal := s.hostedSessionKeyRule("todo.list", map[string]any{"session_key": key}); refusal != "" {
			t.Fatalf("key %q refused: %s", key, refusal)
		}
		if resp := callToolOnce(t, s, 1, "todo.list", map[string]any{"session_key": key}); toolIsError(t, resp) {
			t.Fatalf("todo.list(%q) refused: %s", key, resp)
		}
	}
	for _, key := range []string{unrelated, tooDeep} {
		got := s.hostedSessionKeyRule("todo.list", map[string]any{"session_key": key})
		want := hostedDescendantRefusal("todo.list", f.hosted, key).Error()
		if got != want {
			t.Fatalf("key %q: refusal %q, want %q", key, got, want)
		}
	}
}

func TestHostedSessionKeyCachesOnlyProvenRecordLineage(t *testing.T) {
	f := newHostedKeyFixture(t)
	s := f.connect(t)
	key, err := s.sessions.mint(f.root, roleDelegate, f.hosted)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := s.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dirs.sessions, key, "key.json")
	record, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"session_key": key}
	if refusal := s.hostedSessionKeyRule("todo.list", args); refusal == "" {
		t.Fatal("missing key should be refused")
	}
	if err := os.WriteFile(path, record, 0600); err != nil {
		t.Fatal(err)
	}
	if refusal := s.hostedSessionKeyRule("todo.list", args); refusal != "" {
		t.Fatalf("restored key refused: %s", refusal)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if refusal := s.hostedSessionKeyRule("todo.list", args); refusal != "" {
		t.Fatalf("proven key was read again: %s", refusal)
	}
	if _, ok := s.lookupSession(key); ok {
		t.Fatal("downstream lookup must still reject the deleted key")
	}
}

// TestHostedSessionKeyLeavesOtherConnectionsAlone: a connection that is
// not a hosted lead's keeps its calls as they are: an omitted key is not
// filled and stays refused by the tool, and any known key is the tool's to
// judge. A child connection carries no hosted identity, and a connection
// whose hosted variables were inherited from the hosted harness (its row
// names another process) is no hosted connection.
func TestHostedSessionKeyLeavesOtherConnectionsAlone(t *testing.T) {
	for _, connection := range []string{"not hosted", "child", "inherited variables"} {
		t.Run(connection, func(t *testing.T) {
			f := newHostedKeyFixture(t)
			key := f.peer
			if connection == "child" {
				key = f.child
			}
			if connection == "inherited variables" {
				hostedSessionEnv(t, f.hosted)
				t.Setenv(envHarnessPID, strconv.Itoa(hostedHarnessPID+1))
			}
			s := NewServer(f.root, "test")
			if s.hostedSessionKey() != "" {
				t.Fatal("precondition: the connection is not hosted")
			}
			args := map[string]any{}
			if refusal := s.hostedSessionKeyRule("todo.list", args); refusal != "" || len(args) != 0 {
				t.Fatalf("an omitted key is left alone: refusal %q, args %v", refusal, args)
			}
			resp := callToolOnce(t, s, 1, "todo.list", map[string]any{})
			if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "session_key is required") {
				t.Fatalf("todo.list without session_key stays refused by the tool: %s", resp)
			}
			// A child may still pass its parent's key: refusing that needs a
			// child-connection identity (Decision 11).
			for i, k := range []string{key, f.hosted, f.ferruleChild, f.peerChild} {
				resp := callToolOnce(t, s, 10+i, "todo.list", map[string]any{"session_key": k})
				if toolIsError(t, resp) {
					t.Fatalf("todo.list(%q) refused on a connection that is not hosted: %s", k, resp)
				}
			}
		})
	}
}

// TestRenderLeadManual: the daemon's render of a hosted lead's manual is
// the manual workflow_manual serves for the lead's minted key, without the
// read-full notice that tool result opens with. It never adopts the hosted
// session of the environment it renders under, and a key with no record
// is an error.
func TestRenderLeadManual(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	key, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	// The daemon's environment may carry another session's hosted
	// variables; as that session's connection the render would refuse key.
	hostedSessionEnv(t, other)
	text, err := RenderLeadManual(os.Environ(), root, HostedRender{Harness: "claude"}, key)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, workflowManualReadFullNotice) || text != strings.TrimSpace(text) {
		t.Fatalf("the render keeps the notice or surrounding space:\n%.300s", text)
	}
	if !strings.Contains(text, "## Session Key\n"+key) || strings.Contains(text, other) {
		t.Fatalf("the render is not the manual for %q:\n%s", key, text)
	}

	if _, err := RenderLeadManual(os.Environ(), root, HostedRender{Harness: "claude"}, "never-minted-key"); err == nil || !strings.Contains(err.Error(), "has no record") {
		t.Fatalf("an unknown key must fail the render: %v", err)
	}
}
