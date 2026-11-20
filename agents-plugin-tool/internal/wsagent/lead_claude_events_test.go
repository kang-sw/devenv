// Claude lead background task and event tests.

package wsagent

import (
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestClaudeLeadBackgroundTasks: background_tasks_changed carries the whole
// current list, and the turn a finished task brings starts with no send.
func TestClaudeLeadBackgroundTasks(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-background-task.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.tasks, []int{1, 0}) {
		t.Fatalf("background task counts = %v, want [1 0]", r.tasks)
	}
	if !slices.Equal(r.opened, []string{"send", "init"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d; the task's completion starts a turn nobody sent", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || got[0] != "STARTED" {
		t.Fatalf("ring replies = %q", got)
	}
	// A background shell command is no subagent.
	if agents := r.st.Snapshot(r.clock).Agents; len(agents) != 0 {
		t.Fatalf("a background bash task should add no subagent: %+v", agents)
	}
	// Not a lead: the count is not read.
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	for _, l := range lines {
		if l.field("subtype") == "background_tasks_changed" {
			if evs := m.mapLine(l.raw, nopReplies{}); len(evs) != 0 {
				t.Fatalf("a profile conversation ignores the task list, got %+v", evs)
			}
		}
	}
}

// TestClaudeLeadResultEndsTheTurnWhileBackgroundTasksRun replays a
// recording with session states on (Claude 2.1.323): a turn that leaves a
// background Agent task running gets its result but no idle state, and a
// prompt sent while the task runs gets a result with no running or idle
// state around it; the one idle comes after the task's completion turn.
// Each of those results ends its turn, so the lead shows idle between
// them and every turn writes its own turn entry.
func TestClaudeLeadResultEndsTheTurnWhileBackgroundTasksRun(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-background-agent-states.jsonl")
	first := findLine(t, lines, 0, "result", isResult)
	second := findLine(t, lines, first+1, "second result", isResult)
	third := findLine(t, lines, second+1, "third result", isResult)
	idle := findLine(t, lines, 0, "idle", isState(HarnessIdle))
	if idle != third+1 || countLines(lines, isState(HarnessRunning)) != 1 {
		t.Fatalf("the recording should hold one running state and one idle state, right after the third result (idle at %d, third result at %d)", idle, third)
	}

	r := newLeadReplay(t)
	r.all(lines[:first+1])
	lead := r.st.Snapshot(r.clock).Lead
	if r.turn || r.ends != 1 || lead.TurnStartedAt != 0 || lead.Phase != wslive.PhaseIdle {
		t.Fatalf("a result while a background task runs ends the turn: turn %v, ended %d, lead %+v", r.turn, r.ends, lead)
	}
	r.all(lines[first+1 : second+1])
	if r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "send"}) {
		t.Fatalf("the second message's result ends its turn: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	r.all(lines[second+1 : idle])
	if !r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "send", "init"}) {
		t.Fatalf("the completion turn's result, with no task left, defers to the idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	evs := r.line(lines[idle])
	if r.turn || r.ends != 3 || countEvents(r.events, isTurnEnd) != 3 || len(evs) == 0 || !evs[len(evs)-1].TurnEnd {
		t.Fatalf("the idle state ends the completion turn, whose result came with no task left: ended %d, TurnEnd events %d, %+v", r.ends, countEvents(r.events, isTurnEnd), evs)
	}
	if got := ringKinds(r.ring, wslive.RingTurn); got != 3 {
		t.Fatalf("one turn entry per turn, got %d", got)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 3 || got[0] != "STARTED" || got[1] != "ok2" {
		t.Fatalf("ring replies = %q", got)
	}
	if !slices.Equal(r.tasks, []int{1, 0}) || !slices.Equal(r.states, []string{HarnessRunning, HarnessIdle}) {
		t.Fatalf("background task counts %v, harness states %v", r.tasks, r.states)
	}

	// The second message held from the first turn instead: the first
	// result submits it, and its turn ends at its own result.
	var held []leadLine
	sends := 0
	for _, l := range lines {
		if l.in && l.field("type") == "user" {
			if sends++; sends == 2 {
				continue
			}
		}
		held = append(held, l)
	}
	r = newLeadReplay(t)
	r.all(held[:first])
	r.held = 1
	r.line(held[first])
	if r.held != 0 || !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the result submits the held message: held %d, turn %v, ended %d, opened %v", r.held, r.turn, r.ends, r.opened)
	}
	r.all(held[first+1:])
	if r.turn || r.ends != 3 || !slices.Equal(r.opened, []string{"send", "flush", "init"}) {
		t.Fatalf("turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
}

// TestClaudeLeadIdleAfterAnEndedTurnEndsNothing: an idle state that
// follows a result which ended the turn (a background task ran) belongs to
// that turn: it ends nothing, not the turn a held message began at the
// result. A failed result carries its error itself, and output after the
// result with no running state still opens a turn.
func TestClaudeLeadIdleAfterAnEndedTurnEndsNothing(t *testing.T) {
	line := func(s string) leadLine { return leadLine{raw: []byte(s)} }
	running, idle, result := line(stateLine(HarnessRunning)), line(stateLine(HarnessIdle)), line(resultLine())

	r := newLeadReplay(t)
	r.line(leadLine{in: true, raw: []byte(`{"type":"user","message":{"role":"user","content":"one"}}`)})
	r.all([]leadLine{running, line(tasksLine(1))})
	r.held = 1
	r.line(result)
	if !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the result ends the turn and submits the held message: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	if evs := r.line(idle); len(evs) != 1 || evs[0].HarnessState != HarnessIdle || !r.turn || r.ends != 1 {
		t.Fatalf("the idle after the ended turn ends nothing: %+v, turn %v, ended %d", evs, r.turn, r.ends)
	}
	r.all([]leadLine{running, line(replyLine("u2", "two"))})
	if evs := r.line(line(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`)); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "boom" || r.turn {
		t.Fatalf("a failed result while the task runs ends the turn with its error: %+v", evs)
	}
	r.line(line(replyLine("u3", "more")))
	if !r.turn || !slices.Equal(r.opened, []string{"send", "flush", "assistant"}) {
		t.Fatalf("output after the result opens a turn: turn %v, opened %v", r.turn, r.opened)
	}
	r.line(result)
	r.line(idle)
	if r.turn || r.ends != 3 || !slices.Equal(r.turnErrs, []string{"boom"}) {
		t.Fatalf("turn %v, ended %d, errors %v", r.turn, r.ends, r.turnErrs)
	}

	// With the task list empty again, the result defers to the idle state.
	r.all([]leadLine{line(tasksLine(0)), running, line(replyLine("u4", "four"))})
	if evs := r.line(result); countEvents(evs, isTurnEnd) != 0 || !r.turn {
		t.Fatalf("with no task running the result ends no turn: %+v", evs)
	}
	if evs := r.line(idle); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || r.turn || r.ends != 4 {
		t.Fatalf("the idle state ends it: %+v, ended %d", evs, r.ends)
	}
}

// TestClaudeLeadSettingsStream: the harness's answers to apply_flag_settings
// and set_permission_mode release their requests, the status line after a
// mode change reports the mode, and neither opens a turn.
func TestClaudeLeadSettingsStream(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-settings.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.replies.answers, []string{"probe-0", "probe-1", "probe-2"}) {
		t.Fatalf("answered requests = %v", r.replies.answers)
	}
	if !slices.Equal(r.modes, []string{"auto", "acceptEdits", "acceptEdits"}) {
		t.Fatalf("reported modes = %v, want the init's, the status line's, and the next init's", r.modes)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d; a settings answer opens none", r.opened, r.ends)
	}
	status := lines[findLine(t, lines, 0, "status", func(l leadLine) bool { return l.field("subtype") == "status" })]
	evs := newClaudeMapper(leadSpec()).mapLine(status.raw, nopReplies{})
	if len(evs) != 1 || evs[0].PermissionMode != "acceptEdits" {
		t.Fatalf("a status line with a permission mode is that mode alone, got %+v", evs)
	}
}

// canUseTool is the recorded can_use_tool line with its request changed by
// edit.
func canUseTool(t *testing.T, edit func(request map[string]any)) []byte {
	t.Helper()
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	var line map[string]any
	if err := json.Unmarshal(lines[findLine(t, lines, 0, "can_use_tool", isCanUseTool)].raw, &line); err != nil {
		t.Fatal(err)
	}
	edit(line["request"].(map[string]any))
	raw, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestClaudeLeadDeniesWhatItCannotAsk: a question tool's prompt and a
// malformed request are answered at once with a deny and its message, and
// leave no prompt waiting.
func TestClaudeLeadDeniesWhatItCannotAsk(t *testing.T) {
	const id = "84855f98-7ecb-4ead-97c4-9c43864c898a"
	const malformed = claudeMalformedDenial
	cases := []struct {
		name    string
		edit    func(map[string]any)
		message string
	}{
		{"AskUserQuestion", func(r map[string]any) { r["tool_name"] = "AskUserQuestion" }, claudeChatOnlyDenial},
		{"ExitPlanMode", func(r map[string]any) { r["tool_name"] = "ExitPlanMode" }, claudePlanDenial},
		{"requiresUserInteraction", func(r map[string]any) { r["requiresUserInteraction"] = true }, claudeChatOnlyDenial},
		{"requires_user_interaction", func(r map[string]any) { r["requires_user_interaction"] = true }, claudeChatOnlyDenial},
		{"no tool name", func(r map[string]any) { delete(r, "tool_name") }, malformed},
		{"no tool call id", func(r map[string]any) { delete(r, "tool_use_id") }, malformed},
		{"no input", func(r map[string]any) { delete(r, "input") }, malformed},
		{"input not an object", func(r map[string]any) { r["input"] = "echo hi" }, malformed},
		{"null input", func(r map[string]any) { r["input"] = nil }, malformed},
	}
	for _, c := range cases {
		m := newClaudeMapper(leadSpec())
		replies := &recordedReplies{}
		if evs := m.mapLine(canUseTool(t, c.edit), replies); len(evs) != 0 {
			t.Errorf("%s: no prompt may wait, got %+v", c.name, evs)
		}
		want := map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": map[string]any{"behavior": "deny", "message": c.message}}}
		if len(replies.sent) != 1 || !reflect.DeepEqual(replies.sent[0], want) {
			t.Errorf("%s: sent %v, want one deny %v", c.name, replies.sent, want)
		}
		if len(m.approvals) != 0 {
			t.Errorf("%s: the mapper should track no prompt: %v", c.name, m.approvals)
		}
	}
	if !strings.Contains(claudeChatOnlyDenial, "chat") || !strings.Contains(claudePlanDenial, "plan mode") || claudePlanDenial == claudeChatOnlyDenial {
		t.Fatalf("a question is to be asked in the chat, and a plan presented there with the way out of plan mode named:\n%q\n%q", claudeChatOnlyDenial, claudePlanDenial)
	}

	// A profile conversation has no permission prompts and answers none.
	replies := &recordedReplies{}
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	if evs := m.mapLine(canUseTool(t, func(map[string]any) {}), replies); len(evs) != 0 || len(replies.sent) != 0 {
		t.Fatalf("a profile conversation has no permission prompts: events %+v, sent %v", evs, replies.sent)
	}
}

// TestClaudeLeadApprovalWithdrawn: a control_cancel_request settles its
// prompt with no answer written, and the prompt is settled once.
func TestClaudeLeadApprovalWithdrawn(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	result := findLine(t, lines, ask, "tool result", func(l leadLine) bool { return !l.in && l.field("type") == "user" })
	end := findLine(t, lines, result, "result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	r := newLeadReplay(t)
	r.all(lines[:ask+1])
	id := r.approvals[0].ID
	cancel := leadLine{raw: []byte(`{"type":"control_cancel_request","request_id":"` + id + `"}`)}
	evs := r.line(cancel)
	if len(evs) != 1 || evs[0].ApprovalDone != id || len(r.approvals) != 0 || len(r.replies.sent) != 0 {
		t.Fatalf("a withdrawn prompt is one ApprovalDone and no answer: %+v, sent %v", evs, r.replies.sent)
	}
	before := countEvents(r.events, func(ev Event) bool { return ev.ApprovalDone != "" })
	r.all(lines[result : end+1])
	if got := countEvents(r.events, func(ev Event) bool { return ev.ApprovalDone != "" }); got != before {
		t.Fatalf("a withdrawn prompt is settled once; %d more ApprovalDone followed", got-before)
	}
	// A cancel of a prompt nobody waits on still names it, so the runner
	// drops whatever it holds under that id.
	if evs := r.line(cancel); len(evs) != 1 || evs[0].ApprovalDone != id || len(r.replies.sent) != 0 {
		t.Fatalf("a repeated cancel: %+v, sent %v", evs, r.replies.sent)
	}
}

// TestClaudeLeadResultSettlesOpenPrompts: a turn that ends with a prompt
// still open (an interrupt) settles it, since no answer can follow.
func TestClaudeLeadResultSettlesOpenPrompts(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	end := findLine(t, lines, ask, "result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	r := newLeadReplay(t)
	r.all(lines[:ask+1])
	id := r.approvals[0].ID
	evs := r.line(lines[end])
	if len(r.approvals) != 0 || len(evs) == 0 || evs[0].ApprovalDone != id || !evs[len(evs)-1].TurnEnd {
		t.Fatalf("a result settles the open prompt before it ends the turn: %+v", evs)
	}
}
