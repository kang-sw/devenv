package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestRenderSessionStateLatestSummarySelection(t *testing.T) {
	const earlier = "2026-11-13T12:00:00Z"
	const later = "2026-11-13T13:00:00Z"
	for _, tc := range []struct {
		name, agentID, panelID, agentAt, panelAt, want string
	}{
		{"panel only", "", "e2", "", earlier, "panel recap"},
		{"agent only", "e2", "", earlier, "", "agent body"},
		{"newer panel", "e10", "e2", earlier, later, "panel recap"},
		{"newer agent", "e2", "e10", later, earlier, "agent body"},
		{"equal panel wins", "e9", "e10", earlier, earlier, "panel recap"},
		{"equal agent wins", "e10", "e9", earlier, earlier, "agent body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := wswhiteboard.TopicDoc{Name: "summary"}
			if tc.agentID != "" {
				doc.Entries = append(doc.Entries, wswhiteboard.Entry{ID: tc.agentID, Kind: wswhiteboard.EntryAgent, At: tc.agentAt, Text: "agent body {{313}}\ncontinuation"})
			}
			if tc.panelID != "" {
				doc.Entries = append(doc.Entries, wswhiteboard.Entry{ID: tc.panelID, Kind: wswhiteboard.EntryPanel, PanelID: "p1", At: tc.panelAt, Recap: "panel recap {{313}}\ncontinuation", Text: "secret panel payload"})
			}
			doc.Entries = append(doc.Entries, wswhiteboard.Entry{ID: "e99", Kind: wswhiteboard.EntrySystem, At: later, Text: "system event"})
			board := &wswhiteboard.State{
				Topics: []wswhiteboard.TopicDoc{doc},
				Items:  []wswhiteboard.Item{{ID: 313, Topic: "summary", Status: wswhiteboard.StatusOpen}},
			}
			got := renderSessionState(sessionRecord{Whiteboard: board})
			want := "\n### Whiteboard topics\n- summary: " + tc.want + " (313)\n  continuation\n"
			if !strings.Contains(got, want) || strings.Count(got, "\n- summary:") != 1 || strings.Contains(got, "secret panel payload") || strings.Contains(got, "system event") {
				t.Fatalf("wrong session summary:\n%s", got)
			}
			other := "agent body"
			if tc.want == other {
				other = "panel recap"
			}
			if strings.Contains(got, other) {
				t.Fatalf("losing entry leaked into summary:\n%s", got)
			}
		})
	}
}

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

func TestRenderSessionStateListsLatestSummaries(t *testing.T) {
	got := renderSessionState(sessionRecord{Whiteboard: recapBoard()})
	want := "\n### Whiteboard topics\n- alpha: newer panel recap\n"
	if !strings.Contains(got, want) || strings.Count(got, "\n- ") != 1 {
		t.Fatalf("summary missing or malformed:\n%s", got)
	}
	for _, absent := range []string{"beta", "gamma", "delta", "closed agent line", "outdated agent line", "newer system event", "decided A; waiting on B", "hidden panel body", "Whiteboard recaps"} {
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

func TestRenderSessionStateNoSummaryNoHeading(t *testing.T) {
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

func TestWorkflowStateByteIdenticalToWorkflowManualWithSummaries(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	server := newTestBoardOwner(t, root, "test")
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
	if !strings.Contains(stateResp, "### Whiteboard topics\n- alpha: newer panel recap") || strings.Contains(stateResp, "closed agent line") {
		t.Fatalf("wrong revive summary:\n%s", stateResp)
	}
}
