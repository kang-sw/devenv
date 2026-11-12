package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// recapBoard has two open topics, one with a recap and one without, and a
// closed topic with a recap: only the open topic with a recap may render.
func recapBoard() *wswhiteboard.State {
	return &wswhiteboard.State{
		Seq: 1,
		Topics: []wswhiteboard.TopicDoc{
			{Name: "alpha", Recap: "decided A; waiting on B\nsecond line"},
			{Name: "beta", Body: "body only"},
			{Name: "gamma", Recap: "closed recap"},
		},
		ClosedTopics: []string{"gamma"},
	}
}

func TestRenderSessionStateListsOpenWhiteboardRecaps(t *testing.T) {
	got := renderSessionState(sessionRecord{Whiteboard: recapBoard()})
	want := "\n### Whiteboard recaps\n- alpha: decided A; waiting on B\n  second line\n"
	if !strings.Contains(got, want) {
		t.Fatalf("recap block missing or malformed:\n%s", got)
	}
	if strings.Count(got, "\n- ") != 1 {
		t.Fatalf("want exactly one recap line:\n%s", got)
	}
	if strings.Contains(got, "beta") || strings.Contains(got, "gamma") || strings.Contains(got, "closed recap") {
		t.Fatalf("recap-less or closed topic rendered:\n%s", got)
	}
	if strings.Index(got, "### Whiteboard recaps") > strings.Index(got, "### Todos") {
		t.Fatalf("recaps must precede the todo summary:\n%s", got)
	}
}

// TestRenderSessionStateRecapResolvesItemReferences: a recap's {{nn}}
// reference reads as the board shows it, (nn), when the store holds the
// item, and stays verbatim otherwise.
func TestRenderSessionStateRecapResolvesItemReferences(t *testing.T) {
	board := &wswhiteboard.State{
		Seq:    1,
		Items:  []wswhiteboard.Item{{ID: 313, Topic: "alpha", Class: wswhiteboard.ClassTechnical, Title: "x", Status: wswhiteboard.StatusOpen}},
		Topics: []wswhiteboard.TopicDoc{{Name: "alpha", Recap: "waiting on {{313}}, not {{9}}"}},
	}
	got := renderSessionState(sessionRecord{Whiteboard: board})
	if !strings.Contains(got, "- alpha: waiting on (313), not {{9}}\n") {
		t.Fatalf("recap references not resolved:\n%s", got)
	}
}

func TestRenderSessionStateNoRecapsNoHeading(t *testing.T) {
	boards := map[string]*wswhiteboard.State{
		"nil board": nil,
		"no recap":  {Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "beta", Body: "b"}}},
		"closed only": {Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "g", Recap: "r"}},
			ClosedTopics: []string{"g"}},
	}
	for name, board := range boards {
		if got := renderSessionState(sessionRecord{Whiteboard: board}); strings.Contains(got, "Whiteboard recaps") {
			t.Errorf("%s: heading rendered without an open recap:\n%s", name, got)
		}
	}
}

func TestWorkflowStateByteIdenticalToWorkflowManualWithRecaps(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5520, root, nil))
	if err := server.sessions.mutateRecordParts(key, partAll, partWhiteboard, func(r *sessionRecord) error {
		r.Whiteboard = recapBoard()
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	manualResp := callToolWithKey(t, server, 5521, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5522, key, "workflow_state", nil)

	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual response missing Session State section:\n%s", manualResp)
	}
	if want := manualResp[manualIdx:]; stateResp != want {
		t.Fatalf("workflow_state mismatch.\ngot:\n%s\nwant:\n%s", stateResp, want)
	}
	if !strings.Contains(stateResp, "### Whiteboard recaps\n- alpha: decided A; waiting on B") {
		t.Fatalf("workflow_state missing the open topic's recap:\n%s", stateResp)
	}
	if strings.Contains(stateResp, "closed recap") {
		t.Fatalf("workflow_state rendered a closed topic's recap:\n%s", stateResp)
	}
}
