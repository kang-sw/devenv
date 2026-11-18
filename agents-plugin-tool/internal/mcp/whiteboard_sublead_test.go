package mcp

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

const subleadPlaybookContent = `---
kind: render
delegates: true
role: sublead
tier: large
---
# Sub-lead Playbook

Run the flow.
`

// mintSublead mints a lead-scoped child of parent at root and marks it as a
// sub-lead minted by author, as a role: sublead render does.
func mintSublead(t *testing.T, s *Server, root, parent, author string) string {
	t.Helper()
	key, err := s.sessions.mint(root, roleLead, parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sessions.writeSubleadMarker(key, author); err != nil {
		t.Fatal(err)
	}
	return key
}

// seedLeadBoard writes the lead's board straight into its record, so the
// lead's whiteboard knob stays at its builtin off.
func seedLeadBoard(t *testing.T, s *Server, key, commands string) {
	t.Helper()
	var observed boardMutationObservation
	err := s.sessions.mutateRecordPartsObserved(key, partAll, partWhiteboard, func(rec *sessionRecord) error {
		if rec.Whiteboard == nil {
			rec.Whiteboard = &wswhiteboard.State{}
		}
		if _, problems := wswhiteboard.Apply(rec.Whiteboard, commands, wswhiteboard.Env{Now: time.Now(), IsAbs: filepath.IsAbs}); len(problems) > 0 {
			t.Fatalf("seed: %v", problems)
		}
		return nil
	}, &observed)
	if err != nil {
		t.Fatal(err)
	}
}

func boardTopicTexts(t *testing.T, s *Server, key, topic string) []string {
	t.Helper()
	rec, err := s.sessions.readWhiteboard(key)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, doc := range rec.Whiteboard.Topics {
		if doc.Name != topic {
			continue
		}
		for _, e := range doc.Entries {
			switch e.Kind {
			case wswhiteboard.EntryAgent:
				texts = append(texts, "log:"+e.Text)
			case wswhiteboard.EntryPanel:
				texts = append(texts, "panel:"+e.Recap+"|"+e.Text)
			}
		}
	}
	return texts
}

const subleadSeed = `<wb:topic name="work" label="Work"><wb:log>lead start</wb:log><wb:panel name="plan" recap="Plan">steps</wb:panel></wb:topic>`

func TestSubleadWritesStatusOntoTopLevelLeadBoardWithKnobOff(t *testing.T) {
	f := newWhiteboardFixture(t)
	seedLeadBoard(t, f.s, f.key, subleadSeed)
	// The sub-lead is a grandchild: lead -> sub-lead -> sub-sub-lead.
	mid := mintSublead(t, f.s, f.root, f.key, "ticket-review")
	sub := mintSublead(t, f.s, f.root, mid, "ticket-runner")
	var changed []string
	f.s.whiteboardChanged = func(_, _, key string) { changed = append(changed, key) }

	resp := f.call(t, "whiteboard.update", map[string]any{"session_key": sub, "commands": `<wb:topic name="work"><wb:log>phase 1 green</wb:log><wb:panel name="report" recap="Phase 1">done</wb:panel></wb:topic>`})
	if isToolError(resp) {
		t.Fatalf("sub-lead status write refused: %s", toolText(t, resp))
	}
	if got, want := toolText(t, resp), "posted: 2 status commands to your lead's whiteboard.\n"; got != want {
		t.Fatalf("sub-lead write output = %q, want the one acknowledgement line %q", got, want)
	}
	want := []string{"log:lead start", "panel:Plan|steps", "log:[ticket-runner] phase 1 green", "panel:[ticket-runner] Phase 1|done"}
	if got := boardTopicTexts(t, f.s, f.key, "work"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lead board = %q, want %q", got, want)
	}
	if len(changed) != 1 || changed[0] != f.key {
		t.Fatalf("board refresh targeted %q, want the top-level lead's key only", changed)
	}
	if rec, err := f.s.sessions.readWhiteboard(sub); err == nil && rec.Whiteboard != nil && len(rec.Whiteboard.Topics) > 0 {
		t.Fatalf("the sub-lead's own record grew a board")
	}
	resolver := f.s.sessionResolver(f.key, builtinConfigDefaults())
	if on, err := f.s.whiteboardEnabled(&resolver, f.key); err != nil || on {
		t.Fatalf("the lead's whiteboard knob = %v, %v; a sub-lead write must leave it off", on, err)
	}

	// whiteboard.query reads the lead's board.
	query := f.call(t, "whiteboard.query", map[string]any{"session_key": sub})
	if isToolError(query) || !strings.Contains(toolText(t, query), "work") {
		t.Fatalf("sub-lead whiteboard.query: %s", toolText(t, query))
	}
}

func TestSubleadRefusalsNameWhiteboardStatusPlaybook(t *testing.T) {
	f := newWhiteboardFixture(t)
	seedLeadBoard(t, f.s, f.key, subleadSeed+`<wb:topic name="done" label="Done"><wb:log>x</wb:log><wb:close result="ok"/></wb:topic>`)
	sub := mintSublead(t, f.s, f.root, f.key, "ticket-runner")
	before := boardTopicTexts(t, f.s, f.key, "work")
	for _, tc := range []struct{ name, commands, want string }{
		{"no open topic", `<wb:topic name="missing"><wb:log>x</wb:log></wb:topic>`, `no open topic "missing"`},
		{"closed topic", `<wb:topic name="done"><wb:log>x</wb:log></wb:topic>`, `no open topic "done"`},
		{"label group", `<wb:topic name="work" label="Relabel"><wb:log>x</wb:log></wb:topic>`, "cannot create, relabel, or relink"},
		{"tickets group", `<wb:topic name="work" tickets="261116-feat-x"><wb:log>x</wb:log></wb:topic>`, "cannot create, relabel, or relink"},
		{"ask", `<wb:topic name="work"><wb:add class="policy"><wb:title>t</wb:title></wb:add></wb:topic>`, "writes status only"},
		{"panel id", `<wb:topic name="work"><wb:panel id="p1" recap="r">x</wb:panel></wb:topic>`, "omit id"},
		{"existing panel", `<wb:topic name="work"><wb:panel name="plan" recap="r">x</wb:panel></wb:topic>`, `panel "plan" already exists`},
		{"legacy payload", ``, "payload: no longer accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"session_key": sub, "commands": tc.commands}
			if tc.commands == "" {
				args = map[string]any{"session_key": sub, "payload": map[string]any{}}
			}
			resp := f.call(t, "whiteboard.update", args)
			text := toolText(t, resp)
			if !isToolError(resp) || !strings.Contains(text, tc.want) || !strings.Contains(text, "`whiteboard-status` playbook") {
				t.Fatalf("refusal = %q; want an error naming %q and the whiteboard-status playbook", text, tc.want)
			}
			if strings.Contains(text, "`whiteboard` playbook") {
				t.Fatalf("a sub-lead refusal must not point at the lead's whiteboard playbook: %q", text)
			}
		})
	}
	if after := boardTopicTexts(t, f.s, f.key, "work"); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("refused writes changed the board: %q", after)
	}

	// A partly refused batch commits the admissible command, names the
	// pointer, and returns the failed command to resend.
	resp := f.call(t, "whiteboard.update", map[string]any{"session_key": sub, "commands": `<wb:topic name="work"><wb:log>ok line</wb:log><wb:body>no</wb:body></wb:topic>`})
	text := toolText(t, resp)
	if !isToolError(resp) || !strings.HasPrefix(text, "posted: 1 status commands") || !strings.Contains(text, "`whiteboard-status` playbook") || !strings.Contains(text, "<wb:failed>") {
		t.Fatalf("partial sub-lead write = %q", text)
	}
	if strings.Contains(text, "full:") || strings.Contains(text, "embed:") {
		t.Fatalf("a sub-lead result must carry no lead directives: %q", text)
	}
}

// Unmarked parented keys keep today's denial verbatim, the sub-lead pointer
// included nowhere.
func TestWhiteboardUnmarkedParentedKeysKeepLeadOnlyDenial(t *testing.T) {
	f := newWhiteboardFixture(t)
	seedLeadBoard(t, f.s, f.key, subleadSeed)
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	// A marked delegate is not a lead and is refused too.
	delegate, err := f.s.sessions.mint(f.root, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.sessions.writeSubleadMarker(delegate, "ticket-runner"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{worker, delegate} {
		for _, tool := range []string{"whiteboard.update", "whiteboard.query"} {
			resp := f.call(t, tool, map[string]any{"session_key": key, "commands": `<wb:topic name="work"><wb:log>x</wb:log></wb:topic>`})
			if !strings.Contains(resp, whiteboardLeadOnlyDenial(tool)) || strings.Contains(resp, "whiteboard-status") {
				t.Fatalf("%s by an unmarked or non-lead key = %s; want today's denial", tool, resp)
			}
		}
	}
}

// The marker is its own file in the key directory: rewriting key.json
// through writeParts keeps it.
func TestSubleadMarkerSurvivesKeyRecordRewrite(t *testing.T) {
	f := newWhiteboardFixture(t)
	sub := mintSublead(t, f.s, f.root, f.key, "ticket-runner")
	if err := f.s.sessions.setAgenda(sub, "rewrite", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := f.s.sessions.mutateRecordParts(sub, partAll, partKey, func(rec *sessionRecord) error {
		rec.Harness = "codex"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if author, ok := f.s.sessions.subleadPlaybook(sub); !ok || author != "ticket-runner" {
		t.Fatalf("marker after a key.json rewrite = %q, %v", author, ok)
	}
	entry, _ := f.s.lookupSession(sub)
	if board, ok := f.s.subleadBoardFor(sub, entry); !ok || board.boardKey != f.key {
		t.Fatalf("sub-lead board after rewrite = %+v, %v", board, ok)
	}
}

func TestSubleadWriteNudgesTopLevelLeadDashboard(t *testing.T) {
	dir := dashboardSessionEnv(t)
	fake := startFakeDaemon(t, dir, http.StatusNoContent, false)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	if resp := dashCall(t, s, "todo.list", map[string]any{"session_key": key}); isToolError(resp) {
		t.Fatalf("register: %s", resp)
	}
	seedLeadBoard(t, s, key, subleadSeed)
	sub := mintSublead(t, s, loginRoot, key, "ticket-runner")
	before := len(fake.received())
	if resp := dashCall(t, s, "whiteboard.update", map[string]any{"session_key": sub, "commands": `<wb:topic name="work"><wb:log>status</wb:log></wb:topic>`}); isToolError(resp) {
		t.Fatalf("sub-lead write: %s", resp)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := fake.received()
		if len(got) > before {
			for _, n := range got[before:] {
				if n.ReplyID != wsmailbox.ReplyID(key) {
					t.Fatalf("nudge %+v; want the top-level lead's reply id", n)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a sub-lead write sent no dashboard nudge for its lead")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRenderSubleadPlaybookMintsMarkedLeadKey(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"sublead-pb/sublead-pb.md": subleadPlaybookContent,
		"worker-pb/worker-pb.md":   workerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	parent, err := s.sessions.mint("/work/tree-a", roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := renderPlaybookBody(s, root, "sublead-pb", nil, wsconfig.Options{}, "/work/tree-a", parent, "", nil, "")
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok || entry.scope != roleLead || entry.parent != parent {
		t.Fatalf("minted sub-lead entry = %+v, %v", entry, ok)
	}
	if author, ok := s.sessions.subleadPlaybook(key); !ok || author != "sublead-pb" {
		t.Fatalf("sub-lead marker = %q, %v; want the minting playbook", author, ok)
	}
	workerBody, _, err := renderPlaybookBody(s, root, "worker-pb", nil, wsconfig.Options{}, "/work/tree-a", parent, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.sessions.subleadPlaybook(extractSplicedKey(t, workerBody)); ok {
		t.Fatal("a role: worker render must not mark its key")
	}
}
