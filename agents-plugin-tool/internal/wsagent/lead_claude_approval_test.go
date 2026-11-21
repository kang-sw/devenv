// Claude lead approval and session state tests.

package wsagent

import (
	"bytes"
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"slices"
	"strings"
	"testing"
)

// TestClaudeLeadApprovalAllow: a can_use_tool control request becomes one
// waiting prompt, an allow is the control_response the recording's driver
// sent (the tool call's input handed back as updatedInput), and the tool
// call's result settles the prompt. Recorded with Claude 2.1.322.
func TestClaudeLeadApprovalAllow(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	r := newLeadReplay(t)
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	r.all(lines[:ask])
	if len(r.approvals) != 0 || !r.turn {
		t.Fatalf("before the request a turn runs with no prompt: turn %v, %+v", r.turn, r.approvals)
	}
	evs := r.line(lines[ask])
	if len(evs) != 1 || evs[0].Approval == nil {
		t.Fatalf("a can_use_tool request is one Approval event, got %+v", evs)
	}
	a := *evs[0].Approval
	if a.ID != "84855f98-7ecb-4ead-97c4-9c43864c898a" || a.Tool != "Bash" || a.ToolUseID != "toolu_01WUaebN1z2LLW13S1FFrkUu" || a.AgentID != "" || a.StartedAt == 0 ||
		a.Description == "" || !sameJSON(t, a.Input, []byte(`{"command":"echo hi > probe-a.txt","description":"Write \"hi\" to probe-a.txt in the working directory"}`)) {
		t.Fatalf("approval = %+v (input %s)", a, a.Input)
	}
	if len(r.replies.sent) != 0 {
		t.Fatalf("a permission prompt waits for the user; the mapper sent %v", r.replies.sent)
	}
	answer := findLine(t, lines, ask, "sent control_response", isAnswer)
	if !sameJSON(t, claudeApprovalResponse(a, true, ""), lines[answer].raw) {
		raw, _ := json.Marshal(claudeApprovalResponse(a, true, ""))
		t.Fatalf("allow line =\n%s\nwant the recorded\n%s", raw, lines[answer].raw)
	}
	// The prompt stays listed until the tool call's result arrives.
	result := findLine(t, lines, answer, "tool result", func(l leadLine) bool { return !l.in && l.field("type") == "user" })
	r.all(lines[answer:result])
	if len(r.approvals) != 1 {
		t.Fatalf("the prompt waits until its tool call returns: %+v", r.approvals)
	}
	evs = r.line(lines[result])
	if len(evs) == 0 || evs[0].ApprovalDone != a.ID {
		t.Fatalf("the tool result settles the prompt first, got %+v", evs)
	}
	r.all(lines[result+1:])
	if len(r.approvals) != 0 || r.turn {
		t.Fatalf("after the fixture no prompt waits and no turn runs: %+v, turn %v", r.approvals, r.turn)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 {
		t.Fatalf("turns opened by %v, ended %d; want two sent turns", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || !strings.HasPrefix(got[0], "DONE") || !strings.HasPrefix(got[1], "DONE") {
		t.Fatalf("ring replies = %q", got)
	}
	if len(r.sessions) == 0 || r.sessions[0] != "07125467-ab3c-4594-95ab-5358610cf91d" || !slices.Contains(r.modes, "default") {
		t.Fatalf("init should report the session %v and the permission mode %v", r.sessions, r.modes)
	}
}

// TestClaudeLeadApprovalDenyAndSessionState runs a recording made with
// session-state lines on (Claude 2.1.322): a running state opens the turn
// before any other line, the requires_action state beside a can_use_tool
// request adds no second prompt, a deny is the control_response the driver
// sent, the running state after the answer opens no second turn, and the
// turn ends at its idle state, not at its result.
func TestClaudeLeadApprovalDenyAndSessionState(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	r := newLeadReplay(t)
	// The first output line is the running state: it alone opens a turn.
	first := findLine(t, lines, 0, "output", func(l leadLine) bool { return !l.in })
	if evs := r.m.mapLine(lines[first].raw, r.replies); len(evs) != 2 || evs[0].HarnessState != HarnessRunning || !evs[1].TurnStart {
		t.Fatalf("a running state is the state and a TurnStart, got %+v", evs)
	}
	r = newLeadReplay(t)

	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	state := findLine(t, lines, 0, "requires_action", isState(HarnessWaiting))
	if state > ask {
		t.Fatalf("the fixture should hold requires_action (line %d) before can_use_tool (line %d)", state, ask)
	}
	r.all(lines[:ask+1])
	if len(r.approvals) != 1 || countEvents(r.events, func(ev Event) bool { return ev.Approval != nil }) != 1 {
		t.Fatalf("requires_action beside can_use_tool leaves one prompt: %+v", r.approvals)
	}
	a := r.approvals[0]
	answer := findLine(t, lines, ask, "sent control_response", isAnswer)
	if a.ID != "dedd1842-7da7-40fc-91f4-a30bf7f23279" || !sameJSON(t, claudeApprovalResponse(a, false, "probe"), lines[answer].raw) {
		raw, _ := json.Marshal(claudeApprovalResponse(a, false, "probe"))
		t.Fatalf("deny line =\n%s\nwant the recorded\n%s", raw, lines[answer].raw)
	}

	// requires_action, can_use_tool, deny, running: still the one turn.
	again := findLine(t, lines, answer, "running after the answer", isState(HarnessRunning))
	r.all(lines[ask+1 : again])
	if evs := r.line(lines[again]); len(evs) != 2 || !evs[1].TurnStart {
		t.Fatalf("the running state after an answer = %+v", evs)
	}
	if !slices.Equal(r.opened, []string{"send"}) || !r.turn || r.ends != 0 {
		t.Fatalf("a running state after requires_action opens no second turn: opened %v, ended %d", r.opened, r.ends)
	}

	// The result settles the turn's batch and usage, and the idle state
	// that follows it ends the turn.
	result := findLine(t, lines, again, "result", isResult)
	idle := findLine(t, lines, result, "idle", isState(HarnessIdle))
	r.all(lines[again+1 : result])
	if evs := r.line(lines[result]); countEvents(evs, isTurnEnd) != 0 || !r.turn {
		t.Fatalf("with session states on a result ends no turn: %+v", evs)
	}
	r.all(lines[result+1 : idle])
	if evs := r.line(lines[idle]); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" || r.turn {
		t.Fatalf("the idle state ends the turn: %+v", evs)
	}

	r.all(lines[idle+1:])
	if len(r.approvals) != 0 || r.turn {
		t.Fatalf("after the fixture no prompt waits and no turn runs: %+v, turn %v", r.approvals, r.turn)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || countEvents(r.events, isTurnEnd) != 2 {
		t.Fatalf("turns opened by %v, ended %d (%d TurnEnd events); want two sent turns, each ended once", r.opened, r.ends, countEvents(r.events, isTurnEnd))
	}
	if want := []string{HarnessRunning, HarnessWaiting, HarnessRunning, HarnessIdle, HarnessRunning, HarnessIdle}; !slices.Equal(r.states, want) {
		t.Fatalf("harness states = %v, want %v", r.states, want)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 3 || got[2] != "ok" {
		t.Fatalf("ring replies = %q", got)
	}
	if len(r.replies.sent) != 0 || len(r.turnErrs) != 0 {
		t.Fatalf("the mapper sent %v, turn errors %v", r.replies.sent, r.turnErrs)
	}
	// The recording answered no get_context_usage, so each turn's init
	// asked for the compaction window again.
	if inits := countLines(lines, func(l leadLine) bool { return !l.in && l.field("subtype") == "init" }); inits == 0 || len(r.asks) != inits {
		t.Fatalf("context asks = %v, want one per init (%d)", r.asks, inits)
	}
}

// countLines counts the fixture lines match holds for.
func countLines(lines []leadLine, match func(leadLine) bool) int {
	n := 0
	for _, l := range lines {
		if match(l) {
			n++
		}
	}
	return n
}

// TestClaudeLeadHeldSendFlushesAtIdle: a message sent during a turn is
// submitted when the idle state ends that turn, not at the result before
// it, so the idle state cannot end the turn the submitted message began.
// The recording's second message was typed after the idle state; here it
// is held from the middle of the first turn, as the runner holds one.
func TestClaudeLeadHeldSendFlushesAtIdle(t *testing.T) {
	var lines []leadLine
	sends := 0
	for _, l := range readLeadFixture(t, "claude-lead-approval-deny.jsonl") {
		if l.in && l.field("type") == "user" {
			if sends++; sends == 2 {
				continue
			}
		}
		lines = append(lines, l)
	}
	result := findLine(t, lines, 0, "result", isResult)
	idle := findLine(t, lines, result, "idle", isState(HarnessIdle))
	lastIdle := findLine(t, lines, idle+1, "second idle", isState(HarnessIdle))

	r := newLeadReplay(t)
	r.all(lines[:result])
	r.held = 1
	r.line(lines[result])
	if r.held != 1 || !r.turn || r.ends != 0 {
		t.Fatalf("the result must not submit the held message: held %d, turn %v, ended %d", r.held, r.turn, r.ends)
	}
	r.all(lines[result+1 : idle+1])
	if r.held != 0 || !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the idle state ends the turn and submits the held message: held %d, turn %v, ended %d, opened %v", r.held, r.turn, r.ends, r.opened)
	}
	// Nothing of the first turn is left to end the second one.
	r.all(lines[idle+1 : lastIdle])
	if !r.turn || r.ends != 1 {
		t.Fatalf("the submitted message's turn stays open until its own idle state: turn %v, ended %d", r.turn, r.ends)
	}
	r.line(lines[lastIdle])
	if r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("after the second idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
}

// TestClaudeLeadSessionStateOpensAndCloses: a running state opens a turn no
// send began, an idle state closes one with no result (settling a prompt
// left open), and a result's error reaches the turn's end at the idle.
func TestClaudeLeadSessionStateOpensAndCloses(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-session-state.jsonl")
	running := lines[findLine(t, lines, 0, "running", isState(HarnessRunning))]
	idle := lines[findLine(t, lines, 0, "idle", isState(HarnessIdle))]

	// The recording: running, the turn, its result, idle. No send line is
	// in it, so the running state is what opens the turn.
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.opened, []string{"session_state_changed"}) || r.ends != 1 || r.turn || countEvents(r.events, isTurnEnd) != 1 {
		t.Fatalf("opened by %v, ended %d (TurnEnd events %d)", r.opened, r.ends, countEvents(r.events, isTurnEnd))
	}
	if !slices.Equal(r.states, []string{HarnessRunning, HarnessIdle}) {
		t.Fatalf("harness states = %v", r.states)
	}

	// A turn the harness began and ended with no result line.
	r = newLeadReplay(t)
	r.line(running)
	if !r.turn {
		t.Fatal("a running state opens a turn")
	}
	deny := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	r.line(deny[findLine(t, deny, 0, "can_use_tool", isCanUseTool)])
	if len(r.approvals) != 1 {
		t.Fatalf("approvals = %+v", r.approvals)
	}
	evs := r.line(idle)
	if r.turn || r.ends != 1 || len(r.approvals) != 0 || len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("an idle state with no result closes the turn and its prompt: turn %v, ends %d, %+v, events %+v", r.turn, r.ends, r.approvals, evs)
	}

	// A failed turn: the result's error is the turn's, told at the idle
	// state, once. An interrupt is no failure.
	r = newLeadReplay(t)
	r.line(running)
	failed := leadLine{raw: []byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`)}
	if evs := r.line(failed); countEvents(evs, isTurnEnd) != 0 {
		t.Fatalf("a failed result ends no turn while states arrive: %+v", evs)
	}
	if evs := r.line(idle); !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "boom" {
		t.Fatalf("the idle state carries the result's error: %+v", evs)
	}
	r.line(running)
	r.line(leadLine{raw: []byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"terminal_reason":"aborted_streaming"}`)})
	if evs := r.line(idle); !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("an earlier error must not reach a later turn, and an interrupt is none: %+v", evs)
	}
	if !slices.Equal(r.turnErrs, []string{"boom"}) || r.ends != 2 {
		t.Fatalf("turn errors %v, ended %d", r.turnErrs, r.ends)
	}

	// Not a lead: the state lines are not read.
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	if evs := m.mapLine(running.raw, nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation ignores session states, got %+v", evs)
	}
}

// TestClaudeLeadLocalCommandsWithSessionStates: with session states on, a
// slash command the harness answers itself (a num_turns 0 result) is a
// turn like any other, from its running state to its idle state. Recorded
// with Claude 2.1.322.
func TestClaudeLeadLocalCommandsWithSessionStates(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-local-commands.jsonl")
	r := newLeadReplay(t)
	for _, l := range lines {
		evs := r.line(l)
		if isResult(l) && (!bytes.Contains(l.raw, []byte(`"num_turns":0`)) || countEvents(evs, isTurnEnd) != 0 || !r.turn) {
			t.Fatalf("a local command's result should be num_turns 0 and end no turn here: %+v", evs)
		}
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || !strings.HasPrefix(got[0], "## Context Usage") {
		t.Fatalf("ring replies = %q", got)
	}
}

// TestClaudeLeadResultEndsTheTurnWithoutSessionStates: a harness that
// emits no session states (the three recordings below have none) ends a
// turn at its result: a sent turn's, a slash command's num_turns 0 one,
// and the unsolicited one of a compaction or a background task's turn.
func TestClaudeLeadResultEndsTheTurnWithoutSessionStates(t *testing.T) {
	for _, name := range []string{"claude-lead-approval-allow.jsonl", "claude-lead-compaction.jsonl", "claude-lead-background-task.jsonl"} {
		r := newLeadReplay(t)
		results := 0
		for _, l := range readLeadFixture(t, name) {
			if !l.in && l.field("subtype") == "session_state_changed" {
				t.Fatalf("%s should hold no session state line", name)
			}
			evs := r.line(l)
			if isResult(l) {
				results++
				if len(evs) == 0 || !evs[len(evs)-1].TurnEnd || r.turn {
					t.Fatalf("%s: result %d should end the turn: %+v", name, results, evs)
				}
			}
		}
		if r.ends != results || len(r.states) != 0 {
			t.Fatalf("%s: %d turns ended for %d results (states %v)", name, r.ends, results, r.states)
		}
	}
}

// TestClaudeLeadSteeredMessageJoinsTheTurn: a user message written while a
// turn runs is taken up inside that turn (recorded on 2.1.322 with user
// messages echoed: the second message's echo follows the running tool's
// result). Its queue lines, its echo, and the one result for both
// messages open no second turn, and the turn ends once, at the idle state;
// on a harness that reports no states, at the one result.
func TestClaudeLeadSteeredMessageJoinsTheTurn(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-steer.jsonl")
	sends := 0
	for _, l := range lines {
		if l.in && l.field("type") == "user" {
			sends++
		}
	}
	idle := findLine(t, lines, 0, "idle", isState(HarnessIdle))
	if sends != 2 || idle != len(lines)-1 {
		t.Fatalf("the recording should hold two messages and end idle: %d sends, idle at %d of %d", sends, idle, len(lines))
	}
	r := newLeadReplay(t)
	r.all(lines[:idle])
	if !r.turn || r.ends != 0 || !slices.Equal(r.opened, []string{"send"}) {
		t.Fatalf("one turn runs up to the idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	r.line(lines[idle])
	if r.turn || r.ends != 1 || len(r.turnErrs) != 0 || !slices.Equal(r.opened, []string{"send"}) {
		t.Fatalf("the idle state ends the one turn: turn %v, ended %d, errors %v, opened %v", r.turn, r.ends, r.turnErrs, r.opened)
	}
	if got := ringTexts(r.ring, wslive.RingReply); !slices.Equal(got, []string{"PINEAPPLE"}) {
		t.Fatalf("ring replies = %q", got)
	}
	// The echo of a message is no entry: the runner wrote one at the send.
	if got := ringTexts(r.ring, wslive.RingUser); len(got) != 0 {
		t.Fatalf("the mapper writes no user entry: %q", got)
	}

	plain := newLeadReplay(t)
	for _, l := range lines {
		if !l.in && l.field("subtype") == "session_state_changed" {
			continue
		}
		evs := plain.line(l)
		if isResult(l) && (len(evs) == 0 || !evs[len(evs)-1].TurnEnd) {
			t.Fatalf("with no state lines the result ends the turn: %+v", evs)
		}
	}
	if plain.turn || plain.ends != 1 || !slices.Equal(plain.opened, []string{"send"}) {
		t.Fatalf("with no state lines: turn %v, ended %d, opened %v", plain.turn, plain.ends, plain.opened)
	}
}

// TestClaudeLeadActionWithNoPrompt: the harness says it waits on the user
// (requires_action) and what arrives is no can_use_tool request. The mapper
// reports the state, so the runner can show a wait it has no prompt for,
// and refuses a control request it has no handler for with an error
// answer. The fixture is the denied-prompt recording's first turn with its
// can_use_tool request replaced by an MCP elicitation request, the answer
// by the error this mapper sends, and the tool's lines left out.
func TestClaudeLeadActionWithNoPrompt(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-action-mismatch.jsonl")
	r := newLeadReplay(t)
	request := findLine(t, lines, 0, "control request", func(l leadLine) bool { return !l.in && l.field("type") == "control_request" })
	r.all(lines[:request])
	if len(r.states) == 0 || r.states[len(r.states)-1] != HarnessWaiting {
		t.Fatalf("the harness should say it waits before the request: %v", r.states)
	}
	evs := r.line(lines[request])
	if len(evs) != 1 || evs[0].Notice == "" || !strings.Contains(evs[0].Notice, "elicitation") {
		t.Fatalf("an unhandled control request is a note for the log, got %+v", evs)
	}
	answer := findLine(t, lines, request, "sent control_response", isAnswer)
	if len(r.replies.sent) != 1 || !sameJSON(t, r.replies.sent[0], lines[answer].raw) {
		t.Fatalf("the request should be refused with\n%s\ngot %v", lines[answer].raw, r.replies.sent)
	}
	r.all(lines[request+1:])
	if len(r.approvals) != 0 || countEvents(r.events, func(ev Event) bool { return ev.Approval != nil }) != 0 {
		t.Fatalf("no prompt may wait: %+v", r.approvals)
	}
	if want := []string{HarnessRunning, HarnessWaiting, HarnessRunning, HarnessIdle}; !slices.Equal(r.states, want) {
		t.Fatalf("harness states = %v, want %v", r.states, want)
	}
	if !slices.Equal(r.opened, []string{"send"}) || r.ends != 1 || r.turn {
		t.Fatalf("turns opened by %v, ended %d", r.opened, r.ends)
	}

	// A wait with no request at all (a prompt the process inherited): the
	// state alone.
	m := newClaudeMapper(leadSpec())
	waiting := lines[findLine(t, lines, 0, "requires_action", isState(HarnessWaiting))]
	if evs := m.mapLine(waiting.raw, nopReplies{}); len(evs) != 1 || evs[0].HarnessState != HarnessWaiting {
		t.Fatalf("a requires_action state = %+v", evs)
	}
	// An unreadable request is refused too; one with no id cannot be.
	replies := &recordedReplies{}
	if evs := m.mapLine([]byte(`{"type":"control_request","request_id":"r-9","request":"?"}`), replies); len(evs) != 1 || evs[0].Notice == "" || len(replies.sent) != 1 {
		t.Fatalf("an unreadable control request: events %+v, sent %v", evs, replies.sent)
	}
	if evs := m.mapLine([]byte(`{"type":"control_request","request":{"subtype":"elicitation"}}`), replies); len(evs) != 0 || len(replies.sent) != 1 {
		t.Fatalf("a control request with no id: events %+v, sent %v", evs, replies.sent)
	}
}
