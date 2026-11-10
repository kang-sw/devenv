package wsagent

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// uiResult is a tracker.ui answer in the shape the ws tool writes it: the
// ui line, then the note for the agent.
const uiResult = "ui focus ada.c89-3 ada.c89-7\nThe user's chat moves their view: it opens item ada.c89-3 at comment ada.c89-7.\n"

func uiCommands(events []Event) []wslive.UICommand {
	var out []wslive.UICommand
	for _, ev := range events {
		if ev.UI != nil {
			out = append(out, *ev.UI)
		}
	}
	return out
}

func TestUIEventReadsTheUILine(t *testing.T) {
	for result, want := range map[string]wslive.UICommand{
		uiResult:                            {Op: "focus", Arg: "ada.c89-3", Comment: "ada.c89-7"},
		"ui focus ada.c89-3\nnote\n":        {Op: "focus", Arg: "ada.c89-3"},
		"ui search sync  ribbon flicker\n":  {Op: "search", Arg: "sync  ribbon flicker"},
		"ui scope team":                     {Op: "scope", Arg: "team"},
		"ui view kanban\nThe chat moves.\n": {Op: "view", Arg: "kanban"},
	} {
		ev := uiEvent(result)
		if ev == nil || ev.UI == nil || *ev.UI != want {
			t.Errorf("uiEvent(%q) = %+v, want %+v", result, ev, want)
		}
	}
	for _, result := range []string{"card live ada.c89-3\n", "tracker.ui: unknown op \"press\"\n", "", "note\nui scope team\n"} {
		if ev := uiEvent(result); ev != nil {
			t.Errorf("uiEvent(%q) raised %+v", result, ev.UI)
		}
	}
}

// TestClaudeMapperRaisesUICommands pins which Claude tool results become
// UI commands: a successful ws tracker.ui whose result opens with the ui
// line. An error, another ws tool, and another server's tool of the same
// name raise none.
func TestClaudeMapperRaisesUICommands(t *testing.T) {
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	ui := claudeMCPToolName(ReservedMCPServer, UITool)
	got := uiCommands(claudeLines(t, m,
		claudeCall("t1", ui), claudeResult("t1", uiResult, false),
		claudeCall("t2", ui), claudeResult("t2", []any{map[string]any{"type": "text", "text": "ui scope all\n"}}, false),
		claudeCall("t3", ui), claudeResult("t3", uiResult, true),
		claudeCall("t4", claudeMCPToolName(ReservedMCPServer, CardTool)), claudeResult("t4", uiResult, false),
		claudeCall("t5", "mcp__other__tracker_ui"), claudeResult("t5", uiResult, false),
	))
	if len(got) != 2 || got[0].Comment != "ada.c89-7" || got[1] != (wslive.UICommand{Op: "scope", Arg: "all"}) {
		t.Fatalf("ui commands: %+v", got)
	}
}

// TestPiMapperRaisesUICommands is the Pi form.
func TestPiMapperRaisesUICommands(t *testing.T) {
	m := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	ui := piMCPToolName(ReservedMCPServer, UITool)
	var events []Event
	for _, l := range []map[string]any{
		{"type": "tool_execution_end", "toolCallId": "c1", "toolName": ui, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ui search ribbon\n"}}}, "isError": false},
		{"type": "tool_execution_end", "toolCallId": "c2", "toolName": ui, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": uiResult}}}, "isError": true},
		{"type": "tool_execution_end", "toolCallId": "c3", "toolName": "read", "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": uiResult}}}, "isError": false},
		{"type": "tool_execution_end", "toolCallId": "c4", "toolName": ui, "result": "not an object", "isError": false},
	} {
		raw, _ := json.Marshal(l)
		events = append(events, m.mapLine(raw, nopReplies{})...)
	}
	if got := uiCommands(events); len(got) != 1 || got[0] != (wslive.UICommand{Op: "search", Arg: "ribbon"}) {
		t.Fatalf("ui commands: %+v", got)
	}
}

// TestLiveWriterWritesUILines pins the runner's path: a UI command lands
// in the conversation's ring as a ui entry; one outside the tracker view's
// offer is dropped.
func TestLiveWriterWritesUILines(t *testing.T) {
	dir := t.TempDir()
	w := newLiveWriter(dir, "ui-test-key", "claude", time.Now, io.Discard)
	if !w.ui(wslive.UICommand{Op: "scope", Arg: "given"}) {
		t.Fatal("a ui command should write")
	}
	if w.ui(wslive.UICommand{Op: "press", Arg: "accept"}) {
		t.Fatal("an unknown op should be dropped")
	}
	ring, err := wslive.ReadRing(wslive.RingDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(ring) != 1 || ring[0].Kind != wslive.RingUI || ring[0].UI == nil || ring[0].UI.Arg != "given" {
		t.Fatalf("ring: %+v", ring)
	}
}
