// Claude lead compaction tests.

package wsagent

import (
	"bytes"
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestClaudeLeadCompaction: a slash command's num_turns 0 result ends its
// turn, a typed /compact is a turn whose re-emitted assistant line adds no
// second reply, and a compaction the harness starts by itself is a turn
// with no send, opened by its compacting status and closed by an
// unsolicited result.
func TestClaudeLeadCompaction(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-compaction.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	// Turns: the first reply, /context, /compact, the request_compaction
	// call, the compaction that call asked for, and a last question.
	want := []string{"send", "send", "send", "send", "status", "send"}
	if !slices.Equal(r.opened, want) || r.ends != len(want) || r.turn {
		t.Fatalf("turns opened by %v, ended %d (running %v); want %v", r.opened, r.ends, r.turn, want)
	}
	zero := 0
	for _, l := range lines {
		if !l.in && l.field("type") == "result" && bytes.Contains(l.raw, []byte(`"num_turns":0`)) {
			zero++
		}
	}
	if zero != 3 || countEvents(r.events, isTurnEnd) != len(want) {
		t.Fatalf("the fixture should hold three num_turns 0 results (has %d), each a TurnEnd (%d in all)", zero, countEvents(r.events, isTurnEnd))
	}
	replies := ringTexts(r.ring, wslive.RingReply)
	if len(replies) != 4 || replies[0] != "OK" || !strings.HasPrefix(replies[1], "## Context Usage") || replies[2] != "DONE" || replies[3] != "PLUM" {
		t.Fatalf("ring replies = %q; the compaction's re-emitted message must not repeat", replies)
	}
	// Local /context output and its compaction replay must not replace the
	// physical answering model or manufacture a permanent transition.
	own := r.st.Snapshot(time.Now().UnixMilli()).Lead.Observed
	if own == nil || own.Current.Model != "claude-haiku-5-5" || len(own.History) != 1 {
		t.Fatalf("synthetic command/replay changed observed model: %+v", own)
	}
	for _, o := range observedEvents(t, r.events) {
		if o.Model == "<synthetic>" || o.ID == "3839c735-efdf-43ca-ac30-17eb1e26f2c9" {
			t.Fatalf("synthetic command became response evidence: %+v", o)
		}
	}
	// The fixture does hold the /context reply twice, under one line uuid.
	seen := map[string]int{}
	for _, l := range lines {
		if !l.in && l.field("type") == "assistant" {
			seen[l.field("uuid")]++
		}
	}
	if seen["f0ac3f01-9dfd-45fa-be27-aec69fb587a1"] != 2 {
		t.Fatalf("the fixture should re-emit the /context reply: %v", seen)
	}
	var compacts []wslive.RingEntry
	for _, e := range r.ring {
		if e.Kind == wslive.RingCompact {
			compacts = append(compacts, e)
		}
	}
	if len(compacts) != 2 {
		t.Fatalf("two compactions should reach the ring, got %+v", compacts)
	}
	if len(r.turnErrs) != 0 || len(r.replies.sent) != 0 {
		t.Fatalf("turn errors %v, sent %v", r.turnErrs, r.replies.sent)
	}
}

// leverTurnEnds lists, by TurnEnd, whether each ended a turn that requested
// a compaction (Event.CompactRequested).
func leverTurnEnds(evs []Event) []bool {
	var out []bool
	for _, ev := range evs {
		if ev.TurnEnd {
			out = append(out, ev.CompactRequested)
		}
	}
	return out
}

// leverFixture is the compaction fixture with edit applied to the
// request_compaction call's tool_result line (lines 29-30 pair it by
// tool_use_id) and to the result line that ends that call's turn.
func leverFixture(t *testing.T, editResult, editTurnResult func(map[string]any)) []leadLine {
	t.Helper()
	lines := readLeadFixture(t, "claude-lead-compaction.jsonl")
	const id = "toolu_01P23Tfe64xau7hs1gsv1hu5"
	call := findLine(t, lines, 0, "request_compaction tool_use", func(l leadLine) bool {
		return !l.in && l.field("type") == "assistant" && bytes.Contains(l.raw, []byte(`"name":"mcp__ws__request_compaction"`))
	})
	res := findLine(t, lines, call, "request_compaction tool_result", func(l leadLine) bool {
		return !l.in && l.field("type") == "user" && bytes.Contains(l.raw, []byte(`"tool_use_id":"`+id+`"`))
	})
	end := findLine(t, lines, res, "lever turn result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	rewrite := func(i int, edit func(map[string]any)) {
		if edit == nil {
			return
		}
		var m map[string]any
		if err := json.Unmarshal(lines[i].raw, &m); err != nil {
			t.Fatal(err)
		}
		edit(m)
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		lines[i].raw = raw
	}
	rewrite(res, editResult)
	rewrite(end, editTurnResult)
	return lines
}

// TestClaudeLeadCompactionLeverDetected: the turn carrying a successful
// request_compaction call (the fixture's fourth turn) ends with
// CompactRequested, exactly once; every other turn, the compaction's own
// among them, ends without it.
func TestClaudeLeadCompactionLeverDetected(t *testing.T) {
	r := newLeadReplay(t)
	r.all(leverFixture(t, nil, nil))
	got := leverTurnEnds(r.events)
	want := []bool{false, false, false, true, false, false}
	if !slices.Equal(got, want) {
		t.Fatalf("CompactRequested by turn end = %v; want %v", got, want)
	}
}

// TestClaudeLeadCompactionLeverNotDetected: an error tool_result, or an
// interrupted lever turn (an aborted_* terminal reason, which ends the turn
// with no error), requests nothing.
func TestClaudeLeadCompactionLeverNotDetected(t *testing.T) {
	cases := map[string][2]func(map[string]any){
		"error result": {func(m map[string]any) {
			blocks := m["message"].(map[string]any)["content"].([]any)
			blocks[0].(map[string]any)["is_error"] = true
		}, nil},
		"interrupted turn": {nil, func(m map[string]any) {
			m["subtype"], m["is_error"], m["terminal_reason"] = "error_during_execution", true, "aborted_tools"
		}},
	}
	for name, edits := range cases {
		t.Run(name, func(t *testing.T) {
			r := newLeadReplay(t)
			r.all(leverFixture(t, edits[0], edits[1]))
			if got := leverTurnEnds(r.events); slices.Contains(got, true) {
				t.Fatalf("CompactRequested by turn end = %v; want none", got)
			}
			if len(r.turnErrs) != 0 {
				t.Fatalf("turn errors %v", r.turnErrs)
			}
		})
	}
}

// TestClaudeLeadCompactionLeverAtIdle: on a harness that ends a turn at its
// idle state, the lever turn's result hands the request to that idle's
// TurnEnd; an interrupted result does not.
func TestClaudeLeadCompactionLeverAtIdle(t *testing.T) {
	line := func(v map[string]any) []byte {
		raw, _ := json.Marshal(v)
		return raw
	}
	for _, aborted := range []bool{false, true} {
		m := newClaudeMapper(leadSpec())
		var evs []Event
		feed := func(v map[string]any) { evs = append(evs, m.mapLine(line(v), nopReplies{})...) }
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessRunning})
		feed(map[string]any{"type": "assistant", "message": map[string]any{"id": "m1", "content": []map[string]any{{"type": "tool_use", "id": "lv", "name": "mcp__plugin_ws_ws__request_compaction", "input": map[string]any{}}}}})
		feed(map[string]any{"type": "user", "message": map[string]any{"content": []map[string]any{{"type": "tool_result", "tool_use_id": "lv", "content": "Compaction requested."}}}})
		result := map[string]any{"type": "result", "subtype": "success", "terminal_reason": "completed"}
		if aborted {
			result = map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true, "terminal_reason": "aborted_streaming"}
		}
		feed(result)
		if n := countEvents(evs, isTurnEnd); n != 0 {
			t.Fatalf("aborted=%v: the result ended the turn (%d TurnEnds); the idle should", aborted, n)
		}
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessIdle})
		if got := leverTurnEnds(evs); !slices.Equal(got, []bool{!aborted}) {
			t.Fatalf("aborted=%v: CompactRequested by turn end = %v", aborted, got)
		}
		// The next turn, with no call, requests nothing.
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessRunning})
		feed(map[string]any{"type": "result", "subtype": "success", "terminal_reason": "completed"})
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessIdle})
		if got := leverTurnEnds(evs); !slices.Equal(got, []bool{!aborted, false}) {
			t.Fatalf("aborted=%v: CompactRequested by turn end = %v", aborted, got)
		}
	}
}

// TestClaudeLeadKeepsARealMessageAfterCompaction: only a line uuid seen
// before the boundary is dropped after it; the drop ends with the turn.
func TestClaudeLeadKeepsARealMessageAfterCompaction(t *testing.T) {
	assistant := func(uuid, text string) []byte {
		raw, _ := json.Marshal(map[string]any{"type": "assistant", "uuid": uuid, "message": map[string]any{"id": "msg-" + uuid, "content": []map[string]any{{"type": "text", "text": text}}}})
		return raw
	}
	replies := func(evs []Event) []string {
		var out []string
		for _, ev := range evs {
			if ev.Live != nil && ev.Live.Kind == wslive.KindReply {
				out = append(out, ev.Live.Text)
			}
		}
		return out
	}
	m := newClaudeMapper(leadSpec())
	if got := replies(m.mapLine(assistant("u1", "before"), nopReplies{})); !slices.Equal(got, []string{"before"}) {
		t.Fatalf("replies = %q", got)
	}
	// With no compaction a repeated uuid is still a message.
	if got := replies(m.mapLine(assistant("u1", "before"), nopReplies{})); len(got) != 1 {
		t.Fatalf("a repeated line outside a compaction is kept, got %q", got)
	}
	m.mapLine([]byte(`{"type":"system","subtype":"status","status":"compacting"}`), nopReplies{})
	m.mapLine([]byte(`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"auto"}}`), nopReplies{})
	if got := m.mapLine(assistant("u1", "before"), nopReplies{}); len(got) != 0 {
		t.Fatalf("the re-emitted line is dropped whole, got %+v", got)
	}
	if got := replies(m.mapLine(assistant("u2", "after"), nopReplies{})); !slices.Equal(got, []string{"after"}) {
		t.Fatalf("a new message after the boundary is kept, got %q", got)
	}
	m.mapLine([]byte(`{"type":"result","subtype":"success","num_turns":0}`), nopReplies{})
	if got := replies(m.mapLine(assistant("u2", "after"), nopReplies{})); len(got) != 1 {
		t.Fatalf("the drop ends with the compaction's turn, got %q", got)
	}
}
