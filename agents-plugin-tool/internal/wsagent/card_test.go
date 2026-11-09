package wsagent

import (
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// cardResult is a tracker.show answer for an item of the conversation's
// store, in the shape the ws tool writes it.
const cardResult = "card live ada.c89-3\nada.c89-3 task \"Fix the \\\"sync\\\" ribbon\"\nstatus doing, priority 2 (high), due none\nassignees: Ada doing\nThe chat shows this item as a card the user can open; refer to it rather than restating its fields.\n"

func cards(events []Event) []wslive.CardRef {
	var out []wslive.CardRef
	for _, ev := range events {
		if ev.Card != nil {
			out = append(out, *ev.Card)
		}
	}
	return out
}

func TestCardEventReadsTheCardLine(t *testing.T) {
	ev := cardEvent(cardResult)
	if ev == nil || ev.Card == nil || *ev.Card != (wslive.CardRef{Store: "live", Alias: "ada.c89-3", Title: `Fix the "sync" ribbon`}) {
		t.Fatalf("card event: %+v", ev)
	}
	// An item of another store is answered as text: no card.
	if ev := cardEvent("ada.c89-3 is an item of store team, not of this conversation's store live, so no card is shown; answer from this text.\n"); ev != nil {
		t.Fatalf("a text answer drew a card: %+v", ev.Card)
	}
	// An item line that does not parse keeps the card, without a title.
	if ev := cardEvent("card live ada.c89-3\nsomething else\n"); ev == nil || ev.Card.Title != "" || ev.Card.Alias != "ada.c89-3" {
		t.Fatalf("card without item line: %+v", ev)
	}
}

func claudeLines(t *testing.T, m *claudeMapper, lines ...any) []Event {
	t.Helper()
	var out []Event
	for _, l := range lines {
		raw, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m.mapLine(raw, nopReplies{})...)
	}
	return out
}

func claudeCall(id, name string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"id": "m-" + id, "content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{"item": "ada.c89-3"}}}}}
}

func claudeResult(id string, content any, isError bool) map[string]any {
	return map[string]any{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isError}}}}
}

// TestClaudeMapperDrawsTrackerShowCards pins which Claude tool results become
// card events: a successful ws tracker.show whose result, a string or text
// blocks, opens with the card line. An error, another tool, and another
// server's tool of the same name draw none.
func TestClaudeMapperDrawsTrackerShowCards(t *testing.T) {
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	show := claudeMCPToolName(ReservedMCPServer, CardTool)
	got := cards(claudeLines(t, m,
		claudeCall("t1", show), claudeResult("t1", cardResult, false),
		claudeCall("t2", show), claudeResult("t2", []any{map[string]any{"type": "text", "text": cardResult}}, false),
		claudeCall("t3", show), claudeResult("t3", cardResult, true),
		claudeCall("t4", claudeMCPToolName(ReservedMCPServer, "tracker.query")), claudeResult("t4", cardResult, false),
		claudeCall("t5", "mcp__other__tracker_show"), claudeResult("t5", cardResult, false),
	))
	if len(got) != 2 || got[0].Alias != "ada.c89-3" || got[1].Store != "live" {
		t.Fatalf("cards: %+v", got)
	}
}

// TestPiMapperDrawsTrackerShowCards is the Pi form: tool_execution_end names
// the tool and carries the result's text blocks.
func TestPiMapperDrawsTrackerShowCards(t *testing.T) {
	m := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	show := piMCPToolName(ReservedMCPServer, CardTool)
	var events []Event
	for _, l := range []map[string]any{
		{"type": "tool_execution_end", "toolCallId": "c1", "toolName": show, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": cardResult}}}, "isError": false},
		{"type": "tool_execution_end", "toolCallId": "c2", "toolName": show, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": cardResult}}}, "isError": true},
		{"type": "tool_execution_end", "toolCallId": "c3", "toolName": "read", "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": cardResult}}}, "isError": false},
		{"type": "tool_execution_end", "toolCallId": "c4", "toolName": show, "result": "not an object", "isError": false},
	} {
		raw, _ := json.Marshal(l)
		events = append(events, m.mapLine(raw, nopReplies{})...)
	}
	if got := cards(events); len(got) != 1 || got[0].Title != `Fix the "sync" ribbon` {
		t.Fatalf("cards: %+v", got)
	}
}

// TestLiveWriterWritesCardLines pins the runner's path: a card lands in the
// conversation's ring as a card entry.
func TestLiveWriterWritesCardLines(t *testing.T) {
	dir := t.TempDir()
	w := newLiveWriter(dir, "card-test-key", "claude", time.Now, io.Discard)
	if !w.card(wslive.CardRef{Store: "live", Alias: "ada.c89-3", Title: "Fix"}) {
		t.Fatal("a card should write")
	}
	if w.card(wslive.CardRef{Store: "live", Alias: "<b>"}) {
		t.Fatal("a malformed card should be dropped")
	}
	ring, err := wslive.ReadRing(wslive.RingDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(ring) != 1 || ring[0].Kind != wslive.RingCard || ring[0].Card == nil || ring[0].Card.Alias != "ada.c89-3" {
		t.Fatalf("ring: %+v", ring)
	}
}

func trackerWrites(events []Event) int {
	n := 0
	for _, ev := range events {
		if ev.TrackerWrite {
			n++
		}
	}
	return n
}

// TestClaudeMapperRaisesTrackerWrites pins which Claude tool results raise a
// tracker write: a successful ws tracker.update. An error, another ws
// tool, and another server's tool of the same name raise none.
func TestClaudeMapperRaisesTrackerWrites(t *testing.T) {
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	update := claudeMCPToolName(ReservedMCPServer, TrackerWriteTool)
	events := claudeLines(t, m,
		claudeCall("t1", update), claudeResult("t1", "created ada.c89-4", false),
		claudeCall("t2", update), claudeResult("t2", "refused", true),
		claudeCall("t3", claudeMCPToolName(ReservedMCPServer, CardTool)), claudeResult("t3", cardResult, false),
		claudeCall("t4", "mcp__other__tracker_update"), claudeResult("t4", "created", false),
	)
	if n := trackerWrites(events); n != 1 {
		t.Fatalf("tracker writes: %d, want 1", n)
	}
	if got := cards(events); len(got) != 1 {
		t.Fatalf("the card next to a tracker write: %+v", got)
	}
}

// TestPiMapperRaisesTrackerWrites is the Pi form.
func TestPiMapperRaisesTrackerWrites(t *testing.T) {
	m := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	update := piMCPToolName(ReservedMCPServer, TrackerWriteTool)
	var events []Event
	for _, l := range []map[string]any{
		{"type": "tool_execution_end", "toolCallId": "c1", "toolName": update, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "created"}}}, "isError": false},
		{"type": "tool_execution_end", "toolCallId": "c2", "toolName": update, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "refused"}}}, "isError": true},
		{"type": "tool_execution_end", "toolCallId": "c3", "toolName": "read", "result": map[string]any{"content": []any{}}, "isError": false},
	} {
		raw, _ := json.Marshal(l)
		events = append(events, m.mapLine(raw, nopReplies{})...)
	}
	if n := trackerWrites(events); n != 1 {
		t.Fatalf("tracker writes: %d, want 1", n)
	}
}

// TestRunnerCallsTrackerChangedOnTrackerWrite pins the runner's path: a
// TrackerWrite event calls Options.TrackerChanged; a runner without one
// ignores it.
func TestRunnerCallsTrackerChangedOnTrackerWrite(t *testing.T) {
	var calls atomic.Int64
	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	h.config.TrackerChanged = func() { calls.Add(1) }
	h.start()
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "make three tasks"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "process start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	p.events <- Event{TrackerWrite: true}
	p.reply("done")
	eventually(t, "the tracker-changed call", func() bool { return calls.Load() == 1 })

	bare := newHarness(t, map[string]string{"chat": chatProfile})
	id = bare.create("chat").ID
	if _, err := bare.runner.Send(id, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "process start", func() bool { return bare.backend.startCount() == 1 })
	q := bare.backend.proc(0)
	q.nextSent(t)
	q.events <- Event{TrackerWrite: true}
	q.reply("done")
	eventually(t, "idle", func() bool { return bare.state(id).Status == StatusIdle })
}
