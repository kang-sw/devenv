package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func recapBoard() *wswhiteboard.State {
	return &wswhiteboard.State{
		Seq: 1, EntrySeq: 6, PanelCounter: 1,
		Topics: []wswhiteboard.TopicDoc{
			{Name: "alpha", Entries: []wswhiteboard.Entry{
				{ID: "e1", Kind: wswhiteboard.EntryAgent, Text: "outdated agent line"},
				{ID: "e2", Kind: wswhiteboard.EntryAgent, Text: "decided A; waiting on B\nsecond line", Imported: true},
				{ID: "e3", Kind: wswhiteboard.EntrySystem, Text: "newer system event"},
				{ID: "e4", Kind: wswhiteboard.EntryPanel, PanelID: "p1", Recap: "newer panel recap", Text: "hidden panel body"},
			}},
			{Name: "beta", Body: "body only"},
			{Name: "gamma", Entries: []wswhiteboard.Entry{{ID: "e5", Kind: wswhiteboard.EntryAgent, Text: "closed agent line"}}},
			{Name: "delta", Entries: []wswhiteboard.Entry{{ID: "e6", Kind: wswhiteboard.EntrySystem, Text: "system only"}}},
		},
		ClosedTopics: []string{"gamma"},
	}
}

func TestRenderSessionStateListsLatestAgentLines(t *testing.T) {
	got := renderSessionState(sessionRecord{Whiteboard: recapBoard()})
	want := "\n### Whiteboard topics\n- alpha: decided A; waiting on B\n  second line\n"
	if !strings.Contains(got, want) || strings.Count(got, "\n- ") != 1 {
		t.Fatalf("summary missing or malformed:\n%s", got)
	}
	for _, absent := range []string{"beta", "gamma", "delta", "closed agent line", "outdated agent line", "newer system event", "newer panel recap", "hidden panel body", "Whiteboard recaps"} {
		if strings.Contains(got, absent) {
			t.Errorf("summary contains %q:\n%s", absent, got)
		}
	}
	if strings.Index(got, "### Whiteboard topics") > strings.Index(got, "### Todos") {
		t.Fatal("whiteboard summaries must precede todos")
	}
}

func TestRenderSessionStateAgentLineResolvesItemReferences(t *testing.T) {
	board := &wswhiteboard.State{
		Items:  []wswhiteboard.Item{{ID: 313, Topic: "alpha", Class: wswhiteboard.ClassTechnical, Title: "x", Status: wswhiteboard.StatusOpen}},
		Topics: []wswhiteboard.TopicDoc{{Name: "alpha", Entries: []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntryAgent, Text: "waiting on {{313}}, not {{9}}"}}}},
	}
	got := renderSessionState(sessionRecord{Whiteboard: board})
	if !strings.Contains(got, "- alpha: waiting on (313), not {{9}}\n") {
		t.Fatalf("agent references not resolved:\n%s", got)
	}
}

func TestRenderSessionStateNoAgentLineNoHeading(t *testing.T) {
	boards := map[string]*wswhiteboard.State{
		"nil":                            nil,
		"body only":                      {Topics: []wswhiteboard.TopicDoc{{Name: "beta", Body: "b"}}},
		"legacy recap is not a fallback": {Topics: []wswhiteboard.TopicDoc{{Name: "legacy", Recap: "must be imported by loader, not read here"}}},
		"closed agent":                   {Topics: []wswhiteboard.TopicDoc{{Name: "g", Entries: []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntryAgent, Text: "closed"}}}}, ClosedTopics: []string{"g"}},
		"system only":                    {Topics: []wswhiteboard.TopicDoc{{Name: "s", Entries: []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntrySystem, Text: "not an agent"}}}}},
	}
	for name, board := range boards {
		if got := renderSessionState(sessionRecord{Whiteboard: board}); strings.Contains(got, "### Whiteboard") {
			t.Errorf("%s: unexpected heading:\n%s", name, got)
		}
	}
}

func TestWorkflowStateByteIdenticalToWorkflowManualWithAgentLines(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5520, root, nil))
	if err := server.sessions.mutateRecordParts(key, partAll, partWhiteboard, func(r *sessionRecord) error { r.Whiteboard = recapBoard(); return nil }); err != nil {
		t.Fatal(err)
	}
	manualResp := callToolWithKey(t, server, 5521, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5522, key, "workflow_state", nil)
	index := strings.Index(manualResp, "## Session State")
	if index < 0 {
		t.Fatalf("missing Session State:\n%s", manualResp)
	}
	if manualResp[index:] != stateResp {
		t.Fatalf("state/manual mismatch:\n%s\n%s", stateResp, manualResp[index:])
	}
	if !strings.Contains(stateResp, "### Whiteboard topics\n- alpha: decided A; waiting on B") || strings.Contains(stateResp, "closed agent line") {
		t.Fatalf("wrong revive summary:\n%s", stateResp)
	}
}
