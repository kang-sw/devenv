package wsagent

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func specEnv(spec wsholder.SpawnSpec, name string) (string, bool) {
	v, ok := spec.Env[name]
	return v, ok
}

// controls are the request bodies of the control requests written to the
// holder's child, oldest first, without the get_context_usage asks the
// mapper sends at a turn's init on its own.
func (h *leadHarness) controls(holder string) []any {
	var out []any
	for _, line := range h.holders.inputs(holder, "control_request") {
		id, request := controlRequest(h.t, []byte(line))
		if strings.HasPrefix(id, claudeContextAsk) {
			continue
		}
		out = append(out, request)
	}
	return out
}

func shortActionGrace(t *testing.T) {
	restore := actionGrace
	actionGrace = 30 * time.Millisecond
	t.Cleanup(func() { actionGrace = restore })
}

// TestLeadSpawnRunsUnderAHolder: a lead's first message launches a holder
// and spawns the harness under it with the hosted-session environment, the
// session's row names the spawned process, and a turn runs end to end
// through the real backend and mapper.
func TestLeadSpawnRunsUnderAHolder(t *testing.T) {
	h := newLeadHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{Model: "opus", Effort: "high", PermissionMode: "plan"})
	if !st.Lead || st.PermissionMode != "plan" || st.Model != "opus" || st.Effort != "high" || st.Process || st.Profile != LeadProfileClaude {
		t.Fatalf("created lead = %+v", st)
	}
	if ops := h.holders.opList(); len(ops) != 0 {
		t.Fatalf("creating a lead starts nothing, got %v", ops)
	}
	reg, err := wsdashboard.ReadRegistration(h.dir, st.ID)
	if err != nil || !reg.Hosted || reg.Agent || reg.Unlisted || reg.HostPID != 0 {
		t.Fatalf("a lead's row before its process: %+v (%v)", reg, err)
	}

	h.send(st.ID, "hello")
	holder := h.holder(st.ID)
	if ops := h.holders.opList(); !slices.Equal(ops, []string{"launch " + holder, "spawn " + holder}) {
		t.Fatalf("holder calls = %v", ops)
	}
	spec := h.holders.spawned(holder)
	rec := h.record(st.ID)
	if spec.Argv[0] != "/opt/harness/claude" || spec.Cwd != h.root || spec.Mode != "pipe" {
		t.Fatalf("spawn = %+v", spec)
	}
	for flag, want := range map[string]string{"--permission-prompt-tool": "stdio", "--permission-mode": "plan", "--model": "opus", "--effort": "high"} {
		if got, ok := flagValue(spec.Argv, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q: %v", flag, got, ok, want, spec.Argv)
		}
	}
	if slices.Contains(spec.Argv, "--resume") {
		t.Fatalf("a new session resumes nothing: %v", spec.Argv)
	}
	for name, want := range map[string]string{
		EnvHostedSession: "1", EnvHostedSessionKey: rec.WSKey, EnvHostedSessionRow: wsdashboard.RegistrationPath(h.dir, st.ID),
		"PATH": "/bin", "HOME": "/h", "CLAUDE_CONFIG_DIR": "/claude",
	} {
		if got, ok := specEnv(spec, name); !ok || got != want {
			t.Fatalf("env %s = %q (set %v), want %q", name, got, ok, want)
		}
	}
	// Nothing of the daemon's own session reaches the spawned one.
	for _, name := range []string{"WS_MAILBOX", "WS_MAILBOX_AUTO", "WS_MCP_ROOT", "WS_RSRC", "CLAUDECODE"} {
		if v, ok := specEnv(spec, name); ok {
			t.Fatalf("env %s=%q must not reach a lead's harness", name, v)
		}
	}
	if rec.WSKey == "" || rec.WSKey == "the-daemons-own" || !slices.Contains(h.keys.ensured, rec.WSKey) {
		t.Fatalf("the lead should run on its own minted key, kept usable: %q, ensured %v", rec.WSKey, h.keys.ensured)
	}
	var session holderSession
	for _, info := range h.holders.List().Holders {
		if info.ID == holder {
			_ = json.Unmarshal(info.Session, &session)
		}
	}
	if session.Kind != "lead" || session.ReplyID != st.ID || session.Cwd != h.root {
		t.Fatalf("holder descriptor = %+v", session)
	}
	reg, err = h.row(st.ID), nil
	if err != nil || !reg.Hosted || reg.Agent || reg.HostPID == 0 || reg.HostPID != rec.HostPID || rec.HolderID != holder {
		t.Fatalf("the row should name the spawned process: %+v (%v), record holder %q pid %d", reg, err, rec.HolderID, rec.HostPID)
	}
	if got := h.userInputs(holder); !slices.Equal(got, []string{"hello"}) {
		t.Fatalf("user turns written = %q", got)
	}

	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "plan"), replyLine("u1", "hi there"), resultLine(), stateLine(HarnessIdle))
	h.idle(st.ID)
	ring := h.ring(st.ID)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), []string{"hello"}) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"hi there"}) {
		t.Fatalf("ring = %+v", ring)
	}
	if st := h.state(st.ID); !st.Process || st.ActiveMode != "plan" || st.Error != "" || st.Held != 0 {
		t.Fatalf("after a turn: %+v", st)
	}
	if got := h.record(st.ID).SessionID; got != "sess-1" {
		t.Fatalf("recorded backend session = %q", got)
	}
}

// TestLeadProfileEnvSetsSessionStateEvents: a claude lead's harness is
// started with CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1, which is what makes
// it report its running, requires_action, and idle states.
func TestLeadProfileEnvSetsSessionStateEvents(t *testing.T) {
	h := newLeadHarness(t).start()
	_, holder := h.runLead("hello")
	if got, ok := specEnv(h.holders.spawned(holder), "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS"); !ok || got != "1" {
		t.Fatalf("CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS = %q (set %v), want 1", got, ok)
	}
	// Whatever the daemon's own environment says.
	spec := leadSpec()
	spec.Env = []string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=0", "PATH=/bin"}
	if env := claudeLeadEnv(spec); !slices.Contains(env, "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1") || slices.Contains(env, "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=0") {
		t.Fatalf("claudeLeadEnv = %v", env)
	}
	// A profile conversation's harness is not asked for them.
	if env := claudeEnv([]string{"PATH=/bin"}); slices.ContainsFunc(env, func(v string) bool { return strings.HasPrefix(v, "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=") }) {
		t.Fatalf("claudeEnv = %v", env)
	}
}

// TestLeadApprovalIsAnsweredOnce: a can_use_tool request is listed and
// shown as the session's permission wait, an answer writes one
// control_response, a second answer is refused, and a stop or the
// process's exit settles what still waits.
func TestLeadApprovalIsAnsweredOnce(t *testing.T) {
	shortActionGrace(t)
	h := newLeadHarness(t).start()
	id, holder := h.runLead("go")
	ask := func(holder, uuid, toolUse, request string) {
		t.Helper()
		h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine(uuid, toolUse), stateLine(HarnessWaiting), canUseToolLine(request, toolUse))
		eventually(t, "the prompt "+request, func() bool {
			st := h.state(id)
			return len(st.Approvals) == 1 && st.Approvals[0].ID == request
		})
	}
	ask(holder, "u1", "toolu_1", "req-1")
	st := h.state(id)
	if a := st.Approvals[0]; st.Status != StatusRunning || a.Tool != "Bash" || a.ToolUseID != "toolu_1" || a.Description != "Print hi" || !sameJSON(t, a.Input, []byte(`{"command":"echo hi"}`)) {
		t.Fatalf("state with a waiting prompt = %+v", st)
	}
	eventually(t, "the permission wait in the snapshot", func() bool {
		lead := h.snapshot(id).Lead
		return lead.Permission != nil && lead.Permission.ToolName == "Bash" && lead.Phase == wslive.PhasePermission
	})
	// The harness's wait is this prompt's: nothing else needs the user.
	time.Sleep(4 * actionGrace)
	if st := h.state(id); st.ActionNeeded != "" {
		t.Fatalf("a waiting prompt is no unanswerable wait: %q", st.ActionNeeded)
	}

	if _, err := h.runner.AnswerApproval(id, "req-1", true, ""); err != nil {
		t.Fatalf("AnswerApproval: %v", err)
	}
	answers := h.holders.inputs(holder, "control_response")
	if len(answers) != 1 || !sameJSON(t, json.RawMessage(answers[0]), []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req-1","response":{"behavior":"allow","updatedInput":{"command":"echo hi"}}}}`)) {
		t.Fatalf("answers written = %q", answers)
	}
	if _, err := h.runner.AnswerApproval(id, "req-1", true, ""); requestStatus(err) != 409 {
		t.Fatalf("a second answer should be a conflict, got %v", err)
	}
	if _, err := h.runner.AnswerApproval(id, "req-0", false, ""); requestStatus(err) != 409 {
		t.Fatalf("an answer to no prompt should be a conflict, got %v", err)
	}
	if got := h.holders.inputs(holder, "control_response"); len(got) != 1 {
		t.Fatalf("a refused answer writes nothing: %q", got)
	}
	if st := h.state(id); len(st.Approvals) != 0 || st.Status != StatusRunning {
		t.Fatalf("after the answer: %+v", st)
	}
	h.holders.out(holder, stateLine(HarnessRunning), toolResultLine("toolu_1"))
	h.sync(id, holder)
	eventually(t, "the permission wait to end", func() bool { return h.snapshot(id).Lead.Permission == nil })
	if st := h.state(id); st.ActionNeeded != "" || st.Status != StatusRunning {
		t.Fatalf("an answered prompt leaves no wait: %+v", st)
	}
	h.holders.out(holder, replyLine("u2", "done"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)

	// A deny carries the user's message.
	h.send(id, "again")
	ask(holder, "u3", "toolu_2", "req-2")
	if _, err := h.runner.AnswerApproval(id, "req-2", false, "not now"); err != nil {
		t.Fatal(err)
	}
	answers = h.holders.inputs(holder, "control_response")
	if len(answers) != 2 || !sameJSON(t, json.RawMessage(answers[1]), []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req-2","response":{"behavior":"deny","message":"not now"}}}`)) {
		t.Fatalf("answers written = %q", answers)
	}
	h.holders.out(holder, stateLine(HarnessRunning), replyLine("u4", "ok"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)

	// A stop ends the process and the prompt with it.
	h.send(id, "third")
	ask(holder, "u5", "toolu_3", "req-3")
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	st = h.state(id)
	rec := h.record(id)
	if len(st.Approvals) != 0 || st.Process || st.Status != StatusIdle || rec.HolderID != "" || rec.HostPID != 0 || h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("after a stop: %+v, record holder %q, holder calls %v", st, rec.HolderID, h.holders.opList())
	}
	if reg := h.row(id); reg.HostPID != 0 || !reg.Hosted {
		t.Fatalf("a stopped session's row names no process: %+v", reg)
	}
	if h.snapshot(id).Lead.Permission != nil {
		t.Fatal("a stopped session waits on no permission")
	}
	if _, err := h.runner.AnswerApproval(id, "req-3", true, ""); requestStatus(err) != 409 {
		t.Fatalf("an answer after the stop should be a conflict, got %v", err)
	}

	// So does the process's own exit.
	h.send(id, "fourth")
	var second string
	eventually(t, "a new holder", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	if got, ok := flagValue(h.holders.spawned(second).Argv, "--resume"); !ok || got != "sess-1" {
		t.Fatalf("the next process resumes the session: %v", h.holders.spawned(second).Argv)
	}
	ask(second, "u6", "toolu_4", "req-4")
	h.holders.exit(second, 1)
	eventually(t, "the exit to end the process and its prompt", func() bool {
		st := h.state(id)
		return !st.Process && len(st.Approvals) == 0 && st.Status == StatusIdle && st.Error != "" && h.record(id).HolderID == ""
	})
}

// TestLeadHeldSendFlushesAtIdleNotAtResult: on a harness that reports its
// states, a message sent during a turn is submitted when the idle state
// ends the turn. The result before it submits nothing, so that idle state
// cannot end the turn the submitted message began.
func TestLeadHeldSendFlushesAtIdleNotAtResult(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), replyLine("u1", "first"))
	h.sync(id, holder)
	h.send(id, "two")
	h.holders.out(holder, resultLine())
	h.sync(id, holder)
	if st := h.state(id); st.Held != 1 || st.Status != StatusRunning || len(h.userInputs(holder)) != 1 {
		t.Fatalf("the result must not submit the held message: %+v, written %q", st, h.userInputs(holder))
	}
	h.holders.out(holder, stateLine(HarnessIdle))
	eventually(t, "the held message to be submitted", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	h.sync(id, holder)
	if st := h.state(id); st.Held != 0 || st.Status != StatusRunning {
		t.Fatalf("the submitted message's turn should run: %+v", st)
	}
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), replyLine("u2", "second"), resultLine())
	h.sync(id, holder)
	if st := h.state(id); st.Status != StatusRunning {
		t.Fatalf("the turn runs until its idle state: %+v", st)
	}
	h.holders.out(holder, stateLine(HarnessIdle))
	h.idle(id)
	ring := h.ring(id)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), []string{"one", "two"}) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"first", "second"}) {
		t.Fatalf("ring = %+v", ring)
	}
}

// TestLeadHeldSendFlushesAtAnUnsolicitedTurnsEnd: on a harness that reports
// no states, a turn nobody sent (a background task's completion) opens at
// its first line and ends at its result, where a message held meanwhile is
// submitted.
func TestLeadHeldSendFlushesAtAnUnsolicitedTurnsEnd(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, initLine("sess-1", "auto"), replyLine("u1", "first"), resultLine())
	h.idle(id)
	h.holders.out(holder, initLine("sess-1", "auto"), replyLine("u2", "the task finished"))
	eventually(t, "the unsolicited turn", func() bool { return h.state(id).Status == StatusRunning })
	h.send(id, "two")
	if st := h.state(id); st.Held != 1 || len(h.userInputs(holder)) != 1 {
		t.Fatalf("a message sent during the turn is held: %+v", st)
	}
	h.holders.out(holder, resultLine())
	eventually(t, "the held message to be submitted", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if st := h.state(id); st.Status != StatusRunning || st.Held != 0 {
		t.Fatalf("the submitted message's turn should run: %+v", st)
	}
	h.holders.out(holder, initLine("sess-1", "auto"), replyLine("u3", "second"), resultLine())
	h.idle(id)
	if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, []string{"first", "the task finished", "second"}) {
		t.Fatalf("ring replies = %q", got)
	}
}

// steerHeld marks the conversation's held message for steering, as the
// session page's steer route does, without telling the runner.
func (h *leadHarness) steerHeld(id string) {
	h.t.Helper()
	inbox := wsinbox.Dir(wsdashboard.SessionDir(h.dir, id))
	held, err := wsinbox.Held(inbox)
	if err != nil || len(held) != 1 {
		h.t.Fatalf("one held message to steer: %d (%v)", len(held), err)
	}
	if _, err := wsinbox.Steer(inbox, held[0].Name); err != nil {
		h.t.Fatalf("steer: %v", err)
	}
}

// TestLeadSteeredMessageReachesTheRunningTurn: a message the user marked
// for steering is written to the process while its turn runs, as a user
// entry of that turn; a message not marked stays held until the turn ends.
func TestLeadSteeredMessageReachesTheRunningTurn(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine("u1", "toolu_1"))
	h.sync(id, holder)
	h.send(id, "now")
	h.runner.conv(id).do(func() {})
	if st := h.state(id); st.Held != 1 || len(h.userInputs(holder)) != 1 {
		t.Fatalf("a message sent during a turn is held: %+v, written %q", st, h.userInputs(holder))
	}
	// The second send is what tells the runner: both are held when it looks.
	h.steerHeld(id)
	h.send(id, "later")
	eventually(t, "the steered message", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "now"}) })
	h.runner.conv(id).do(func() {})
	if st := h.state(id); st.Held != 1 || st.Status != StatusRunning || !st.Process {
		t.Fatalf("the unmarked message stays held and the turn runs on: %+v", st)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, []string{"one", "now"}) {
		t.Fatalf("ring user entries = %q", got)
	}
	// The harness takes the message up inside the turn; its one result ends
	// nothing while it reports states.
	h.holders.out(holder, toolResultLine("toolu_1"), replyLine("u2", "changed course"), resultLine())
	h.sync(id, holder)
	if st := h.state(id); st.Held != 1 || st.Status != StatusRunning || len(h.userInputs(holder)) != 2 {
		t.Fatalf("before the idle state: %+v, written %q", st, h.userInputs(holder))
	}
	h.holders.out(holder, stateLine(HarnessIdle))
	eventually(t, "the held message at the turn's end", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "now", "later"})
	})
	h.holders.out(holder, turnLines("sess-1", "u3", "done")...)
	h.idle(id)
	ring := h.ring(id)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), []string{"one", "now", "later"}) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"changed course", "done"}) {
		t.Fatalf("ring = %+v", ring)
	}
	if h.holders.opCount("stop") != 0 || h.holders.opCount("spawn") != 1 {
		t.Fatalf("holder calls = %v", h.holders.opList())
	}
}

// TestPiLeadSteeredMessageIsTheSteerCommand: pi takes a steered message as
// its steer command, not as a prompt, and a refusal is logged.
func TestPiLeadSteeredMessageIsTheSteerCommand(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "hello")
	holder := h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`)
	h.send(id, "now")
	h.runner.conv(id).do(func() {})
	if got := h.holders.inputs(holder, "steer"); len(got) != 0 || h.state(id).Held != 1 {
		t.Fatalf("a message not marked is held: steer lines %q, state %+v", got, h.state(id))
	}
	h.steerHeld(id)
	h.runner.InboxChanged(id)
	eventually(t, "the steer command", func() bool { return len(h.holders.inputs(holder, "steer")) == 1 })
	if got := h.holders.inputs(holder, "steer")[0]; !sameJSON(t, json.RawMessage(got), []byte(`{"type":"steer","message":"now"}`)) || len(h.holders.inputs(holder, "prompt")) != 1 {
		t.Fatalf("steer line = %s, prompts %q", got, h.holders.inputs(holder, "prompt"))
	}
	if st := h.state(id); st.Held != 0 || st.Status != StatusRunning {
		t.Fatalf("after the steer: %+v", st)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, []string{"hello", "now"}) {
		t.Fatalf("ring user entries = %q", got)
	}
	h.holders.out(holder, `{"type":"response","command":"steer","success":true,"data":{"disposition":"queued"}}`)
	h.holders.out(holder, `{"type":"response","command":"steer","success":false,"error":"no run to steer"}`)
	eventually(t, "the refusal to be logged", func() bool { return strings.Contains(h.log.String(), "no run to steer") })
	if st := h.state(id); st.Status != StatusRunning || st.Error != "" {
		t.Fatalf("a steer's answer ends no turn: %+v", st)
	}
	h.holders.out(holder, `{"type":"agent_settled"}`)
	h.idle(id)
}

// TestSteeredMessageWaitsInAProfileConversation: steering mid-turn is a
// lead conversation's; any other conversation keeps a marked message held
// until its turn ends, as before.
func TestSteeredMessageWaitsInAProfileConversation(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "first"); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, 0)
	if got := p.nextSent(t); got != "first" {
		t.Fatalf("first turn = %q", got)
	}
	if _, err := h.runner.Send(id, "now"); err != nil {
		t.Fatal(err)
	}
	inbox := wsinbox.Dir(wsdashboard.SessionDir(h.dir, id))
	held, err := wsinbox.Held(inbox)
	if err != nil || len(held) != 1 {
		t.Fatalf("held = %d (%v)", len(held), err)
	}
	if _, err := wsinbox.Steer(inbox, held[0].Name); err != nil {
		t.Fatal(err)
	}
	h.runner.InboxChanged(id)
	h.runner.conv(id).do(func() {})
	select {
	case text := <-p.sent:
		t.Fatalf("a profile conversation's turn takes no message: %q", text)
	default:
	}
	if st := h.state(id); st.Held != 1 || st.Status != StatusRunning {
		t.Fatalf("the marked message stays held: %+v", st)
	}
	p.reply("answer")
	if got := p.nextSent(t); got != "now" {
		t.Fatalf("the held message is the next turn, got %q", got)
	}
}

// TestLeadMailIsDeliveredAsATurn: claimed mail is the session's to deliver.
// It stays in the record until a process took it, across a failed start and
// a daemon restart, and merges with a dashboard message held beside it.
func TestLeadMailIsDeliveredAsATurn(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	key := h.record(id).WSKey
	h.holders.set(func() { h.holders.spawnErr = errors.New("no room") })
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the failed start", func() bool { st := h.state(id); return st.Held == 1 && st.Error != "" })
	if rec := h.record(id); !slices.Equal(rec.Mail, []string{"From a@x: ping"}) || rec.HolderID != "" {
		t.Fatalf("claimed mail waits in the record: %+v", rec)
	}

	// The mail is delivered by the next daemon, not by a second check of
	// this one after the failure is cleared.
	h.mailSettled(id)
	h.holders.set(func() { h.holders.spawnErr = nil })
	h.restart()
	// A daemon that ends stops its watch, which clears its listening marker.
	if got := h.mail.stopped(); got != 1 {
		t.Fatalf("mailbox watches stopped by the daemon's end = %d, want 1", got)
	}
	holder := h.holder(id)
	eventually(t, "the mail turn", func() bool { return slices.Equal(h.userInputs(holder), []string{"From a@x: ping"}) })
	eventually(t, "the record to drop the delivered mail", func() bool { return len(h.record(id).Mail) == 0 })
	if st := h.state(id); st.Status != StatusRunning || st.Held != 0 {
		t.Fatalf("a mail turn runs: %+v", st)
	}
	// Mail shows through the session's mail history, not as a user entry.
	if got := ringTexts(h.ring(id), wslive.RingUser); len(got) != 0 {
		t.Fatalf("mail is no user entry: %q", got)
	}
	eventually(t, "the turn's origin", func() bool { return h.snapshot(id).Lead.TurnOrigin == wslive.OriginMail })
	h.holders.out(holder, turnLines("sess-1", "u1", "pong")...)
	h.idle(id)

	// Mail that arrives during a turn waits with the dashboard's message.
	h.send(id, "question")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.mail.deliver(key, "From b@x: more")
	h.send(id, "also")
	eventually(t, "both to be held", func() bool { return h.state(id).Held == 2 })
	h.holders.out(holder, replyLine("u2", "answer"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the merged turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"From a@x: ping", "question", "also\n\nFrom b@x: more"})
	})
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, []string{"question", "also"}) {
		t.Fatalf("ring user entries = %q", got)
	}

	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the mailbox watch to end", func() bool { return h.mail.stopped() == 2 })
}

// TestLeadMailClaimedDuringATurnTellsThePage: mail claimed while a turn
// runs is not submitted until the turn ends, so the claim itself must tell
// the page: its mail history and held count changed.
func TestLeadMailClaimedDuringATurnTellsThePage(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.heartbeat = time.Hour // no heartbeat notification in this test
	h.start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	key := h.record(id).WSKey
	h.send(id, "question")
	holder := h.holder(id)
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	// Let the notifications of the lines above settle.
	settled := h.notes.Load()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		time.Sleep(30 * time.Millisecond)
		now := h.notes.Load()
		if now == settled {
			break
		}
		settled = now
	}
	h.mail.deliver(key, "From b@x: more")
	eventually(t, "the claim to be held", func() bool { return h.state(id).Held == 1 })
	eventually(t, "the page to be told of the claim", func() bool { return h.notes.Load() > settled })
	if got := h.userInputs(holder); !slices.Equal(got, []string{"question"}) {
		t.Fatalf("mail claimed during a turn waits for its end: %q", got)
	}
}

// TestLeadMailWaitsForTheRejoin: mail the session holds, and mail that
// waits in its mailbox when a daemon starts, must not start a process
// while the session's own still runs under its holder: it is delivered to
// that process at the turn's end.
func TestLeadMailWaitsForTheRejoin(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the mail to be claimed", func() bool { return slices.Equal(h.record(id).Mail, []string{"From a@x: ping"}) })
	h.runner.Close()
	h.holders.restart()
	h.mail.deliver(key, "From b@x: more")
	h.start()
	eventually(t, "the waiting mail to be claimed", func() bool {
		return slices.Equal(h.record(id).Mail, []string{"From a@x: ping", "From b@x: more"})
	})
	eventually(t, "the running turn to be back", func() bool { st := h.state(id); return st.Process && st.Status == StatusRunning })
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 2 {
		t.Fatalf("mail waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	h.holders.out(holder, replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the mail turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "From a@x: ping\n\nFrom b@x: more"})
	})
	if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("the session keeps its one process: %v", h.holders.opList())
	}
}

// TestLeadSettingsChangeInStream: a model, effort, or permission-mode
// change reaches a running lead on its input and leaves the process alone,
// also a return to a default; and the next launch runs with the session's
// current values, not the ones it was created with.
func TestLeadSettingsChangeInStream(t *testing.T) {
	h := newLeadHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{Model: "haiku", Effort: "medium"})
	id := st.ID
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, turnLines("sess-1", "u1", "ok")...)
	h.idle(id)

	if _, err := h.runner.SetSettings(id, ptr("opus"), ptr("low")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.SetPermissionMode(id, "plan"); err != nil {
		t.Fatal(err)
	}
	want := []any{
		map[string]any{"subtype": "set_model", "model": "opus"},
		map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "low"}},
		map[string]any{"subtype": "set_permission_mode", "mode": "plan"},
	}
	if got := h.controls(holder); !reflect.DeepEqual(got, want) {
		t.Fatalf("control requests = %v, want %v", got, want)
	}
	h.holders.out(holder, statusModeLine("plan"))
	eventually(t, "the reported mode", func() bool { return h.state(id).ActiveMode == "plan" })

	// A return to the defaults clears the overrides in-stream.
	if _, err := h.runner.SetSettings(id, ptr(""), ptr("")); err != nil {
		t.Fatal(err)
	}
	want = append(want,
		map[string]any{"subtype": "set_model"},
		map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": nil}},
	)
	if got := h.controls(holder); !reflect.DeepEqual(got, want) {
		t.Fatalf("control requests = %v, want %v", got, want)
	}
	if _, err := h.runner.SetSettings(id, ptr("sonnet"), ptr("high")); err != nil {
		t.Fatal(err)
	}
	if st := h.state(id); !st.Process || st.Model != "sonnet" || st.Effort != "high" || st.PermissionMode != "plan" || h.holders.opCount("stop") != 0 || h.holders.opCount("spawn") != 1 {
		t.Fatalf("an in-stream change must not restart the process: %+v, holder calls %v", st, h.holders.opList())
	}
	if rec := h.record(id); rec.HolderID != holder {
		t.Fatalf("the session should still run under %s: %+v", holder, rec)
	}

	// Stop and send: the resumed process runs with the changed values.
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "two")
	var second string
	eventually(t, "the resumed process", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	argv := h.holders.spawned(second).Argv
	for flag, want := range map[string]string{"--model": "sonnet", "--effort": "high", "--permission-mode": "plan", "--resume": "sess-1"} {
		if got, ok := flagValue(argv, flag); !ok || got != want {
			t.Fatalf("resumed %s = %q (present %v), want %q: %v", flag, got, ok, want, argv)
		}
	}
}

// TestLeadSettingsRestartOnlyWithoutAnInStreamForm: a change the harness
// refuses as unsupported is the one case that restarts the process, on the
// same backend session.
func TestLeadSettingsRestartOnlyWithoutAnInStreamForm(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "ok")...)
	h.idle(id)
	h.holders.set(func() { h.holders.refuse["set_model"] = "Unsupported control request subtype: set_model" })
	if _, err := h.runner.SetSettings(id, ptr("opus"), nil); err != nil {
		t.Fatal(err)
	}
	if st := h.state(id); st.Process || h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("a change with no in-stream form stops the process: %+v, holder calls %v", st, h.holders.opList())
	}
	h.send(id, "two")
	var second string
	eventually(t, "the restarted process", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	argv := h.holders.spawned(second).Argv
	if model, _ := flagValue(argv, "--model"); model != "opus" {
		t.Fatalf("the restart carries the new model: %v", argv)
	}
	if session, _ := flagValue(argv, "--resume"); session != "sess-1" {
		t.Fatalf("the restart resumes the session: %v", argv)
	}
}

// TestLeadResumesInTheModeTheHarnessReports: the mode the harness says it
// runs in becomes the session's, so a later launch resumes in it; a mode
// that is none of a lead's is not taken, and a change of the user's that
// waits for the turn's end stands against what the harness reports
// meanwhile.
func TestLeadResumesInTheModeTheHarnessReports(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "ok")...)
	h.idle(id)
	h.holders.out(holder, statusModeLine("acceptEdits"))
	eventually(t, "the reported mode to be the session's, and recorded", func() bool {
		st := h.state(id)
		return st.ActiveMode == "acceptEdits" && st.PermissionMode == "acceptEdits" && h.record(id).PermissionMode == "acceptEdits"
	})
	h.holders.out(holder, statusModeLine("someFutureMode"))
	eventually(t, "the unknown mode to be reported", func() bool { return h.state(id).ActiveMode == "someFutureMode" })
	if got := h.record(id).PermissionMode; got != "acceptEdits" {
		t.Fatalf("a mode no lead may be asked for is not recorded: %q", got)
	}

	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning))
	h.sync(id, holder)
	if _, err := h.runner.SetPermissionMode(id, "plan"); err != nil {
		t.Fatal(err)
	}
	h.holders.out(holder, initLine("sess-1", "acceptEdits"))
	h.sync(id, holder)
	if st := h.state(id); st.PermissionMode != "plan" || st.ActiveMode != "acceptEdits" || len(h.controls(holder)) != 0 {
		t.Fatalf("a change made during a turn waits for its end: %+v, controls %v", st, h.controls(holder))
	}
	h.holders.out(holder, replyLine("u2", "ok"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	want := []any{map[string]any{"subtype": "set_permission_mode", "mode": "plan"}}
	eventually(t, "the mode change at the turn's end", func() bool { return reflect.DeepEqual(h.controls(holder), want) })

	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "three")
	var second string
	eventually(t, "the resumed process", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	if mode, _ := flagValue(h.holders.spawned(second).Argv, "--permission-mode"); mode != "plan" {
		t.Fatalf("the resumed process runs in the session's mode: %v", h.holders.spawned(second).Argv)
	}
}

// TestLeadWaitWithNoPromptIsShown: a harness that reports it waits on the
// user while no permission prompt waits here must not look like it works.
// The session says so, in its state and as its live permission wait, once
// the grace a prompt's own line gets has passed, and until the harness
// moves on or its process ends. A control request the runner has no form
// for is answered with an error, so the harness does not wait on it.
func TestLeadWaitWithNoPromptIsShown(t *testing.T) {
	shortActionGrace(t)
	h := newLeadHarness(t).start()
	id, holder := h.runLead("go")
	shown := func() {
		t.Helper()
		eventually(t, "the wait to show", func() bool {
			lead := h.snapshot(id).Lead
			return h.state(id).ActionNeeded == actionNeededText && lead.Permission != nil && lead.Permission.ToolName == actionNeededTool && lead.Phase == wslive.PhasePermission
		})
	}
	gone := func() {
		t.Helper()
		eventually(t, "the wait to end", func() bool {
			return h.state(id).ActionNeeded == "" && h.snapshot(id).Lead.Permission == nil
		})
	}
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), stateLine(HarnessWaiting))
	shown()
	if st := h.state(id); st.Status != StatusRunning || len(st.Approvals) != 0 {
		t.Fatalf("the turn runs on under the wait: %+v", st)
	}
	if n := strings.Count(h.log.String(), "requires_action"); n != 1 {
		t.Fatalf("the wait should be logged once, got %d:\n%s", n, h.log.String())
	}
	h.holders.out(holder, stateLine(HarnessRunning))
	gone()

	// A request with no form here: answered with an error, and logged.
	h.holders.out(holder, jsonLine(map[string]any{"type": "control_request", "request_id": "req-x", "request": map[string]any{"subtype": "elicitation", "message": "pick one"}}))
	eventually(t, "the error answer", func() bool { return len(h.holders.inputs(holder, "control_response")) == 1 })
	if got := h.holders.inputs(holder, "control_response")[0]; !sameJSON(t, json.RawMessage(got), []byte(`{"type":"control_response","response":{"subtype":"error","request_id":"req-x","error":"this session's host does not handle the control request \"elicitation\""}}`)) {
		t.Fatalf("answer = %s", got)
	}
	eventually(t, "the request to be logged", func() bool { return strings.Contains(h.log.String(), `"elicitation"`) })

	h.holders.out(holder, stateLine(HarnessWaiting))
	shown()
	h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
	gone()
	h.idle(id)

	h.send(id, "again")
	h.holders.out(holder, stateLine(HarnessRunning), stateLine(HarnessWaiting))
	shown()
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	gone()
	if st := h.state(id); st.Process || st.Status != StatusIdle {
		t.Fatalf("after the stop: %+v", st)
	}
}

// TestLeadRejoinsItsProcessAfterADaemonRestart: the daemon's end leaves a
// lead's process running under its holder. The next daemon attaches to it
// and rebuilds the conversation from the holder's replay of its whole
// ring: no second process, no entry written twice, the running turn and
// its waiting prompt back and answerable, the lines the harness wrote
// while no daemon ran applied once, and a wait with no prompt shown again.
func TestLeadRejoinsItsProcessAfterADaemonRestart(t *testing.T) {
	shortActionGrace(t)
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine("u2", "toolu_1"), stateLine(HarnessWaiting), canUseToolLine("req-1", "toolu_1"))
	eventually(t, "the prompt", func() bool { return len(h.state(id).Approvals) == 1 })
	ringBefore := h.ring(id)
	inputsBefore := h.holders.inputs(holder, "")

	h.runner.Close()
	rec := h.record(id)
	if rec.HolderID != holder || rec.HolderSeq != uint64(h.holders.records(holder)) || h.holders.opCount("stop") != 0 {
		t.Fatalf("the daemon's end leaves the process running, with the applied sequence recorded: record %+v, %d holder records, holder calls %v", rec, h.holders.records(holder), h.holders.opList())
	}
	h.holders.restart()
	h.start()
	eventually(t, "the running turn and its prompt to be back", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning && len(st.Approvals) == 1 && st.Approvals[0].ID == "req-1"
	})
	eventually(t, "the permission wait in the snapshot", func() bool {
		lead := h.snapshot(id).Lead
		return lead.Permission != nil && lead.Permission.ToolName == "Bash"
	})
	// The prompt can be answered once the rejoin is over.
	eventually(t, "the answer to be taken", func() bool {
		_, err := h.runner.AnswerApproval(id, "req-1", true, "")
		if err != nil && requestStatus(err) != 409 {
			t.Fatalf("AnswerApproval: %v", err)
		}
		return err == nil
	})
	if got := h.holders.inputs(holder, "control_response"); len(got) != 1 || !strings.Contains(got[0], `"req-1"`) {
		t.Fatalf("answers written = %q", got)
	}
	if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a rejoin starts and ends no process: %v", h.holders.opList())
	}
	if got := h.ring(id); !reflect.DeepEqual(got, ringBefore) {
		t.Fatalf("a rejoin writes no conversation entry:\n%+v\nbefore\n%+v", got, ringBefore)
	}
	if got := h.holders.inputs(holder, ""); !slices.Equal(got[:len(inputsBefore)], inputsBefore) || len(got) != len(inputsBefore)+1 {
		t.Fatalf("a rejoin writes nothing to the process but the answer: %q", got)
	}

	// The harness works on while no daemon runs.
	h.runner.Close()
	h.holders.restart()
	h.holders.out(holder, stateLine(HarnessRunning), toolResultLine("toolu_1"), replyLine("u3", "second"), resultLine(), stateLine(HarnessIdle))
	h.start()
	eventually(t, "the lines written meanwhile", func() bool {
		return slices.Equal(ringTexts(h.ring(id), wslive.RingReply), []string{"first", "second"})
	})
	h.send(id, "three")
	eventually(t, "the next turn after the rejoin", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two", "three"}) })
	if st := h.state(id); len(st.Approvals) != 0 || st.Status != StatusRunning || st.Error != "" {
		t.Fatalf("after the second rejoin: %+v", st)
	}

	// A wait with no prompt to answer: shown, and shown again by the next
	// daemon, until the harness moves on.
	h.holders.out(holder, stateLine(HarnessRunning), toolUseLine("u4", "toolu_2"), stateLine(HarnessWaiting))
	eventually(t, "the unanswerable wait", func() bool { return h.state(id).ActionNeeded != "" })
	logged := strings.Count(h.log.String(), "requires_action")
	h.restart()
	eventually(t, "the unanswerable wait after the rejoin", func() bool {
		st := h.state(id)
		return st.ActionNeeded != "" && st.Status == StatusRunning && len(st.Approvals) == 0
	})
	eventually(t, "the wait in the snapshot", func() bool {
		lead := h.snapshot(id).Lead
		return lead.Permission != nil && lead.Permission.ToolName == actionNeededTool
	})
	if strings.Count(h.log.String(), "requires_action") != logged+1 {
		t.Fatalf("the rejoined wait should be logged once more:\n%s", h.log.String())
	}
	h.holders.out(holder, stateLine(HarnessRunning), replyLine("u5", "third"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	eventually(t, "the wait to end", func() bool { return h.state(id).ActionNeeded == "" && h.snapshot(id).Lead.Permission == nil })

	// Three daemons applied the stream; each entry is there once.
	ring := h.ring(id)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), []string{"one", "two", "three"}) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"first", "second", "third"}) {
		t.Fatalf("ring = %+v", ring)
	}
	if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("holder calls = %v", h.holders.opList())
	}

	// A holder that ended while no daemon ran: the process is over, and
	// the next message starts a new one on the same backend session.
	h.runner.Close()
	h.holders.restart()
	h.holders.vanish(holder)
	h.start()
	eventually(t, "the vanished process to be noticed", func() bool { return !h.state(id).Process && h.record(id).HolderID == "" })
	if st := h.state(id); st.Status != StatusIdle || len(st.Approvals) != 0 {
		t.Fatalf("after the holder vanished: %+v", st)
	}
	if reg := h.row(id); reg.HostPID != 0 {
		t.Fatalf("the row names no process: %+v", reg)
	}
	h.send(id, "four")
	var second string
	eventually(t, "a new holder", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	eventually(t, "the message on the new process", func() bool { return slices.Equal(h.userInputs(second), []string{"four"}) })
	if session, _ := flagValue(h.holders.spawned(second).Argv, "--resume"); session != "sess-1" {
		t.Fatalf("the new process resumes the session: %v", h.holders.spawned(second).Argv)
	}
}

// TestLeadRejoinOfAWrappedRingHoldsInput: a lead's holder keeps a ring
// larger than the manager's default, and a rejoin whose ring no longer
// holds the turn's start (its first record is past the one the record
// names) takes the turn as running: a message and mail sent meanwhile wait
// for the harness's next turn end, then go once.
func TestLeadRejoinOfAWrappedRingHoldsInput(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	if got := h.holders.launchRing(holder); got != leadRingBytes {
		t.Fatalf("a lead's holder ring = %d bytes, want %d", got, leadRingBytes)
	}
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()

	// The turn works on while no daemon runs, and its ring drops all but
	// the last record: a tool result, which tells nothing of the turn.
	h.holders.out(holder, toolResultLine("toolu_1"), toolResultLine("toolu_2"))
	h.holders.trim(holder, h.holders.records(holder)-1)
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "three")
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the mail to be claimed", func() bool { return slices.Equal(h.record(id).Mail, []string{"From a@x: ping"}) })
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, []string{"one", "two"}) || h.state(id).Held != 2 {
		t.Fatalf("input waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}

	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held input as the next turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "two", "three\n\nFrom a@x: ping"})
	})
	h.sync(id, holder)
	if got := h.userInputs(holder); len(got) != 3 || h.state(id).Held != 0 {
		t.Fatalf("the held input goes once: written %q, state %+v", got, h.state(id))
	}
	if strings.Count(h.log.String(), "left its ring") != 1 {
		t.Fatalf("the loss should be logged once:\n%s", h.log.String())
	}
	if strings.Contains(h.log.String(), "whether a turn runs") || len(h.holders.inputs(holder, "get_state")) != 0 {
		t.Fatalf("a claude lead is not asked whether a turn runs:\n%s", h.log.String())
	}
}

// TestLeadRejoinOfARingThatWrappedWhileAttachedHoldsInput: a long turn
// pushes its own start out of the ring while the daemon is still attached,
// so the next daemon's rejoin finds the ring beginning no later than the
// record after the one the record names: nothing left it unseen, yet the
// replay shows no turn boundary. The rejoin takes the turn as running: a
// message sent meanwhile waits for the harness's next turn end, then goes
// once.
func TestLeadRejoinOfARingThatWrappedWhileAttachedHoldsInput(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolResultLine("toolu_1"), toolResultLine("toolu_2"))
	h.sync(id, holder)
	h.holders.trim(holder, h.holders.records(holder)-1)
	h.runner.Close()
	first, applied := uint64(h.holders.records(holder)), h.record(id).HolderSeq
	if first <= 1 || first > applied+1 {
		t.Fatalf("the ring should begin past 1 and no later than the record after %d, begins at %d", applied, first)
	}
	h.holders.restart()
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "three")
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, []string{"one", "two"}) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}

	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held message as the next turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "two", "three"})
	})
	h.sync(id, holder)
	if got := h.userInputs(holder); len(got) != 3 || h.state(id).Held != 0 {
		t.Fatalf("the held message goes once: written %q, state %+v", got, h.state(id))
	}
}

// TestLeadRejoinOfAWrappedRingTrustsATurnBoundary: when the records after
// the loss show the turn's end, the rejoin takes the state they show, and a
// message sent after it goes at once.
func TestLeadRejoinOfAWrappedRingTrustsATurnBoundary(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()
	h.holders.out(holder, toolResultLine("toolu_1"), replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	h.holders.trim(holder, h.holders.records(holder)-2)
	h.start()
	eventually(t, "the rejoined session to be idle", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusIdle
	})
	h.send(id, "two")
	eventually(t, "the message to go at once", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if log := h.log.String(); !strings.Contains(log, "the turn state is the one the records after them show") || strings.Contains(log, "with no turn boundary after them") {
		t.Fatalf("the loss should be logged as covered by a turn boundary:\n%s", log)
	}
}

// TestLeadRejoinGapHoldsInputAndDropsAPartialLine: records that leave the
// ring while the replay streams arrive as a gap. A line in progress before
// it is dropped, not spliced onto the line after it; and the turn boundary
// before the gap (an idle state) does not count, so the turn is taken as
// running until the harness ends one.
func TestLeadRejoinGapHoldsInputAndDropsAPartialLine(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()

	// While no daemon runs: a line read in two parts, then a turn starts;
	// all of it but the line's first part leaves the ring mid-replay.
	h.holders.outRaw(holder, `{"type":"assistant","uuid":"u9","mess`, true)
	partial := uint64(h.holders.records(holder))
	h.holders.outRaw(holder, `age":{"id":"msg-u9","content":[]}}`+"\n", false)
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), tasksLine(7))
	h.holders.gapOnReplay(holder, partial+1, partial+4)
	h.start()
	eventually(t, "the line after the gap and the turn taken as running", func() bool {
		st := h.state(id)
		return st.Process && st.BackgroundTasks == 7 && st.Status == StatusRunning
	})
	h.send(id, "two")
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}
	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
}

// piHeldRejoin runs a pi lead through one turn, restarts the daemon while
// the idle lead's subagent widget resends push every turn boundary out of
// the ring, and rejoins: the rejoin takes the turn as running, a message
// sent then is held, and the rejoin asks pi whether a turn runs. It
// returns the get_state ask's request id.
func piHeldRejoin(t *testing.T) (h *leadHarness, id, holder, ask string) {
	t.Helper()
	h = newLeadHarness(t).start()
	id = h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder = h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled"}`)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	resend := string(piWidgetLine(piAgentsWidgetKey, `{"kind":"agent","agentId":"child-1","label":"audit","status":"done","caption":"worker"}`))
	h.holders.out(holder, resend, resend, resend)
	h.holders.trim(holder, h.holders.records(holder)-2)
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "two")
	eventually(t, "the get_state ask", func() bool { return len(h.holders.inputs(holder, "get_state")) == 1 })
	var d struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(h.holders.inputs(holder, "get_state")[0]), &d); err != nil || d.ID == "" {
		t.Fatalf("get_state ask = %s", h.holders.inputs(holder, "get_state")[0])
	}
	if got := piPrompts(h, holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the answer: prompts %q, state %+v", got, h.state(id))
	}
	return h, id, holder, d.ID
}

// piPrompts are the messages of the prompt lines written to the holder's
// child.
func piPrompts(h *leadHarness, holder string) []string {
	var out []string
	for _, line := range h.holders.inputs(holder, "prompt") {
		var d struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal([]byte(line), &d)
		out = append(out, d.Message)
	}
	return out
}

func piStateAnswer(ask string, streaming bool) string {
	return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": true, "data": map[string]any{"isStreaming": streaming, "isCompacting": false, "messageCount": 2}})
}

// TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns: a pi lead's rejoin
// that cannot tell from its replay whether a turn runs asks pi, and an idle
// answer ends the held turn: the held message goes once.
func TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(piPrompts(h, holder), []string{"one", "two"}) })
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	if got := piPrompts(h, holder); len(got) != 2 || h.state(id).Held != 0 || len(h.holders.inputs(holder, "get_state")) != 1 {
		t.Fatalf("the held message goes once, and pi is asked once: prompts %q, state %+v", got, h.state(id))
	}
	if !strings.Contains(h.log.String(), "the harness reports no turn running") {
		t.Fatalf("the answer should be logged:\n%s", h.log.String())
	}
}

// TestPiLeadRejoinProbeIdleAnswerAfterATurnStartKeepsTheHold: a turn that
// starts after the ask (its start may follow pi's answer on the stream)
// overrides an idle answer; the turn's own end releases the held message,
// once.
func TestPiLeadRejoinProbeIdleAnswerAfterATurnStartKeepsTheHold(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	h.holders.out(holder, `{"type":"agent_start"}`,
		string(piWidgetLine(piAgentsWidgetKey, `{"kind":"agent","agentId":"sync-1","label":"sync","status":"running","caption":"worker","batches":[]}`)))
	// The widget's row shows once the loop handled the turn start before it.
	eventually(t, "the turn start to be handled", func() bool {
		for _, a := range h.snapshot(id).Agents {
			if a.ID == "sync-1" {
				return true
			}
		}
		return false
	})
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the answer to be taken", func() bool { return strings.Contains(h.log.String(), "the harness reports") })
	h.runner.conv(id).do(func() {})
	if got := piPrompts(h, holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning || !strings.Contains(h.log.String(), "but one started since") {
		t.Fatalf("the message stays held: prompts %q, state %+v, log:\n%s", got, h.state(id), h.log.String())
	}
	h.holders.out(holder, `{"type":"agent_settled"}`)
	eventually(t, "the held message at the turn's end", func() bool { return slices.Equal(piPrompts(h, holder), []string{"one", "two"}) })
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	if got := piPrompts(h, holder); len(got) != 2 || h.state(id).Held != 0 {
		t.Fatalf("the held message goes once: prompts %q, state %+v", got, h.state(id))
	}
}

// TestPiLeadRejoinProbeAnswerForAnotherProcessIsDropped: an answer that
// arrives for a process the conversation no longer runs changes nothing;
// the current process's own answer still releases the hold.
func TestPiLeadRejoinProbeAnswerForAnotherProcessIsDropped(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	c := h.runner.conv(id)
	c.do(func() { c.wrapProbed(&fakeProc{}, false, nil) })
	if got := piPrompts(h, holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning {
		t.Fatalf("a stale answer keeps the hold: prompts %q, state %+v", got, h.state(id))
	}
	if strings.Contains(h.log.String(), "the harness reports") {
		t.Fatalf("a stale answer is not taken:\n%s", h.log.String())
	}
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(piPrompts(h, holder), []string{"one", "two"}) })
}

// TestPiLeadRejoinProbeThatSaysRunningOrFailsKeepsTheHold: a running answer
// or a refused ask keeps the message held until pi ends the turn.
func TestPiLeadRejoinProbeThatSaysRunningOrFailsKeepsTheHold(t *testing.T) {
	for name, answer := range map[string]func(ask string) string{
		"running": func(ask string) string { return piStateAnswer(ask, true) },
		"refused": func(ask string) string {
			return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": false, "error": "boom"})
		},
		"no run state": func(ask string) string {
			return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": true, "data": map[string]any{"messageCount": 2}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, id, holder, ask := piHeldRejoin(t)
			h.holders.out(holder, answer(ask))
			eventually(t, "the answer to be taken", func() bool {
				log := h.log.String()
				return strings.Contains(log, "the harness reports a turn running") || strings.Contains(log, "ask the harness whether a turn runs")
			})
			// The log line comes first in the loop step that takes the
			// answer; a round trip lets that step finish.
			h.runner.conv(id).do(func() {})
			if got := piPrompts(h, holder); !slices.Equal(got, []string{"one"}) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning {
				t.Fatalf("the message stays held: prompts %q, state %+v", got, h.state(id))
			}
			h.holders.out(holder, `{"type":"agent_settled"}`)
			eventually(t, "the held message at the turn's end", func() bool { return slices.Equal(piPrompts(h, holder), []string{"one", "two"}) })
		})
	}
}

// TestLeadAnsweredPromptStaysAnsweredAcrossADaemonRestart: the answer to a
// prompt goes to the process's input, which the holder's ring does not
// hold, so the next daemon's replay shows the prompt's request with no sign
// of its answer. The record keeps it answered: it does not wait again and
// takes no second answer, and it is forgotten once its tool call returns.
func TestLeadAnsweredPromptStaysAnsweredAcrossADaemonRestart(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine("u1", "toolu_1"), stateLine(HarnessWaiting), canUseToolLine("req-1", "toolu_1"))
	eventually(t, "the prompt", func() bool { return len(h.state(id).Approvals) == 1 })
	if _, err := h.runner.AnswerApproval(id, "req-1", true, ""); err != nil {
		t.Fatalf("AnswerApproval: %v", err)
	}
	// The tool runs on; the daemon restarts before its result.
	h.holders.out(holder, stateLine(HarnessRunning))
	h.sync(id, holder)
	if got := h.record(id).Answered; !slices.Equal(got, []string{"req-1"}) {
		t.Fatalf("answered ids in the record = %v", got)
	}
	h.restart()
	eventually(t, "the rejoined turn", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	eventually(t, "the rejoin to take answers again", func() bool {
		_, err := h.runner.AnswerApproval(id, "req-1", true, "")
		if err == nil {
			t.Fatal("a prompt answered before the restart took a second answer")
		}
		return requestStatus(err) == 409 && strings.Contains(err.Error(), "not waiting")
	})
	if st := h.state(id); len(st.Approvals) != 0 {
		t.Fatalf("the answered prompt waits again after the rejoin: %+v", st.Approvals)
	}
	if got := h.holders.inputs(holder, "control_response"); len(got) != 1 {
		t.Fatalf("answers written = %q", got)
	}
	h.holders.out(holder, toolResultLine("toolu_1"), replyLine("u2", "done"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	eventually(t, "the settled prompt to be forgotten", func() bool { return len(h.record(id).Answered) == 0 })
}

// TestLeadProcessThatExitedWhileNoDaemonRan: a harness that ended on its
// own while no daemon ran left its exit in the holder's ring. The next
// daemon's rejoin finds it at the replay's end: the turn and its prompt
// are over, the holder is ended, and what waited for the session (mail
// here) goes to a new process on the same backend session.
func TestLeadProcessThatExitedWhileNoDaemonRan(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), toolUseLine("u2", "toolu_1"), stateLine(HarnessWaiting), canUseToolLine("req-1", "toolu_1"))
	eventually(t, "the prompt", func() bool { return len(h.state(id).Approvals) == 1 })
	h.runner.Close()
	h.holders.restart()
	h.holders.exit(holder, 0)
	h.mail.deliver(key, "From a@x: ping")
	h.start()

	var second string
	eventually(t, "a new process for the waiting mail", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	eventually(t, "the mail turn", func() bool { return slices.Equal(h.userInputs(second), []string{"From a@x: ping"}) })
	if st := h.state(id); len(st.Approvals) != 0 || st.Status != StatusRunning || !st.Process {
		t.Fatalf("the ended process's prompt is gone and the mail's turn runs: %+v", st)
	}
	if session, _ := flagValue(h.holders.spawned(second).Argv, "--resume"); session != "sess-1" {
		t.Fatalf("the new process resumes the session: %v", h.holders.spawned(second).Argv)
	}
	want := []string{"launch " + holder, "spawn " + holder, "stop " + holder, "launch " + second, "spawn " + second}
	if ops := h.holders.opList(); !slices.Equal(ops, want) {
		t.Fatalf("holder calls = %v, want %v", ops, want)
	}
	if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("ring replies = %q", got)
	}
}

// TestLeadRejoinDoesNotStallBehindALongReplay: a holder answers a call on
// the connection its records arrive on, in order. A rejoin that asked the
// holder where its replay ends from the conversation's own loop would wait
// for an answer queued behind records only that loop can take.
func TestLeadRejoinDoesNotStallBehindALongReplay(t *testing.T) {
	h := newLeadHarness(t)
	h.holders.callTimeout = time.Second
	h.start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	for i := 0; i < 600; i++ {
		h.holders.out(holder, tasksLine(1+i%2))
	}
	h.holders.out(holder, tasksLine(0))
	h.start()
	h.send(id, "two")
	eventually(t, "the rejoin to end and take the message", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if n := h.holders.timedOut(); n != 0 {
		t.Fatalf("%d holder calls waited out their timeout during the rejoin", n)
	}
	if st := h.state(id); st.BackgroundTasks != 0 || st.Error != "" {
		t.Fatalf("after the rejoin: %+v", st)
	}
}

// TestLeadShutdownFinishesTheLineItStandsIn: the daemon's end may come
// between two events of one output line. The sequence it records must
// cover the whole line, or the next daemon would apply the line again and
// write its entry twice.
func TestLeadShutdownFinishesTheLineItStandsIn(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	c := h.runner.conv(id)
	c.do(func() {
		// The reply line is a turn start and the reply: take the first
		// event only, as the loop would have when the shutdown came.
		h.holders.out(holder, replyLine("u1", "first"))
		for {
			ev := <-c.events
			c.handle(ev)
			if ev.TurnStart {
				break
			}
		}
		if c.lineOpen == 0 {
			t.Errorf("the loop should stand inside the reply's line")
		}
		c.letGo()
		// The loop returns after letGo; it reads the stream no more.
		c.events = nil
	})
	if rec := h.record(id); rec.HolderSeq != uint64(h.holders.records(holder)) {
		t.Fatalf("recorded sequence = %d with %d records applied or begun", rec.HolderSeq, h.holders.records(holder))
	}
	h.restart()
	h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
	h.sync(id, holder)
	h.idle(id)
	if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("ring replies = %q, want the reply once", got)
	}
}

// TestLeadStartEndsThePreviousHolderFirst: a holder that could not be ended
// stays in the record, and no new process starts on the session before it
// is gone, since two processes would run one backend session.
func TestLeadStartEndsThePreviousHolderFirst(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "ok")...)
	h.idle(id)
	h.holders.set(func() { h.holders.stopErr = errors.New("busy") })
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	if st, rec := h.state(id), h.record(id); st.Process || !strings.Contains(st.Error, "busy") || rec.HolderID != holder {
		t.Fatalf("a holder that would not end stays recorded: %+v, record holder %q", st, rec.HolderID)
	}
	h.send(id, "two")
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(id).Error, "could not be ended") })
	if st := h.state(id); st.Process || st.Held != 1 || h.holders.opCount("launch") != 1 {
		t.Fatalf("no process may start beside the old one: %+v, holder calls %v", st, h.holders.opList())
	}

	h.holders.set(func() { h.holders.stopErr = nil })
	h.send(id, "three")
	eventually(t, "the new process", func() bool { return h.record(id).HolderID == "h2" && h.record(id).HostPID != 0 })
	ops := h.holders.opList()
	if n := len(ops); n < 3 || !slices.Equal(ops[n-3:], []string{"stop " + holder, "launch h2", "spawn h2"}) {
		t.Fatalf("the old holder ends before the new one starts: %v", ops)
	}
	eventually(t, "the held messages", func() bool { return slices.Equal(h.userInputs("h2"), []string{"two\n\nthree"}) })
}

// TestLeadOrphanHolderIsEndedAfterTheFirstScan: a lead holder no
// conversation owns runs a process nobody can reach; it is ended once the
// first scan has shown every holder. A holder of another kind is not the
// runner's to end.
func TestLeadOrphanHolderIsEndedAfterTheFirstScan(t *testing.T) {
	h := newLeadHarness(t)
	orphan := h.holders.add(holderSession{Kind: "lead", ReplyID: "gone"})
	other := h.holders.add(map[string]any{"kind": "terminal"})
	h.start()
	eventually(t, "the orphan to be ended", func() bool { return h.holders.opCount("stop "+orphan) == 1 })
	time.Sleep(100 * time.Millisecond)
	if ops := h.holders.opList(); !slices.Equal(ops, []string{"stop " + orphan}) {
		t.Fatalf("holder calls = %v; %s is not a lead's", ops, other)
	}
	if !strings.Contains(h.log.String(), orphan) {
		t.Fatalf("ending an orphan should be logged:\n%s", h.log.String())
	}
}

// TestLeadShutdownDuringTheFirstScanEndsNoHolder: a daemon that closes
// while its first holder scan runs lets each lead's process go under its
// holder for the next daemon, so the conversation's loop has ended by the
// time the scan returns. The sweep after the scan must not read those
// holders as owned by no conversation and end them.
func TestLeadShutdownDuringTheFirstScanEndsNoHolder(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "the first scan", entered)

	c := h.runner.conv(id)
	closed := make(chan struct{})
	go func() { h.runner.Close(); close(closed) }()
	wait(t, "the conversation to be let go", c.done)
	release()
	wait(t, "the daemon to close", closed)
	if n := h.holders.opCount("stop"); n != 0 {
		t.Fatalf("a closing daemon ended a running lead's holder: %v", h.holders.opList())
	}

	h.holders.restart()
	h.start()
	h.send(id, "two")
	eventually(t, "the next daemon to rejoin the process", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if h.holders.opCount("launch") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("the session keeps its one process: %v", h.holders.opList())
	}
}

// TestLeadRejoinWaitsForTheFirstScan: until the first holder scan has
// shown every holder, a rejoined lead's holder is missing from the listing
// because it is not attached yet, not because it is gone. A heartbeat in
// that window must not take the process for gone, and a message sent
// meanwhile must not start a second process beside it.
func TestLeadRejoinWaitsForTheFirstScan(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "the first scan", entered)

	h.send(id, "two")
	time.Sleep(6 * h.opts.heartbeat)
	if rec := h.record(id); h.holders.opCount("launch") != 1 || rec.HolderID != holder {
		t.Fatalf("a process started before the first scan: record holder %q, holder calls %v", rec.HolderID, h.holders.opList())
	}
	release()
	eventually(t, "the rejoin to take the message", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if h.holders.opCount("launch") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("the session keeps its one process: %v", h.holders.opList())
	}
}

// TestLeadIdleStopWaitsForBackgroundTasks: the idle timer does not stop a
// process whose harness reports a running background task, whose work the
// stop would end; it does once the list empties.
func TestLeadIdleStopWaitsForBackgroundTasks(t *testing.T) {
	h := newLeadHarness(t)
	h.globalProfile(LeadProfileClaude, `{"backend":"claude","lead":true,"idle_timeout_seconds":1}`)
	h.start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), tasksLine(1), replyLine("u1", "started"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	time.Sleep(1600 * time.Millisecond)
	if st := h.state(id); !st.Process || st.BackgroundTasks != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a background task keeps the process past the idle timeout: %+v, holder calls %v", st, h.holders.opList())
	}
	h.holders.out(holder, tasksLine(0))
	eventually(t, "the idle stop", func() bool { return !h.state(id).Process && h.holders.opCount("stop "+holder) == 1 })
	if st := h.state(id); st.Error != "" || st.Status != StatusIdle {
		t.Fatalf("an idle stop is no failure: %+v", st)
	}
}

// TestPiLeadTurnsAndCompact: a pi lead runs the user's own loadout (no
// off switches on its command line, no tool-loadout check), a run that
// starts with no send opens a turn, a typed /compact is the RPC compact
// command whose answer ends the turn, and idle stop is off by default.
func TestPiLeadTurnsAndCompact(t *testing.T) {
	h := newLeadHarness(t).start()
	st := h.lead(BackendPi, LeadOptions{})
	id := st.ID
	if !st.Lead || st.Profile != LeadProfilePi || st.PermissionMode != "" {
		t.Fatalf("created pi lead = %+v", st)
	}
	h.send(id, "hello")
	holder := h.holder(id)
	spec := h.holders.spawned(holder)
	if mode, _ := flagValue(spec.Argv, "--mode"); spec.Argv[0] != "/opt/harness/pi" || mode != "rpc" {
		t.Fatalf("spawn argv = %v", spec.Argv)
	}
	for _, banned := range []string{"--no-extensions", "--no-skills", "--no-tools", "--tools", "--no-context-files", "--system-prompt", "--append-system-prompt"} {
		if slices.Contains(spec.Argv, banned) {
			t.Fatalf("a pi lead's args must not carry %s: %v", banned, spec.Argv)
		}
	}
	for name, want := range map[string]string{EnvHostedSession: "1", EnvHostedSessionKey: h.record(id).WSKey, EnvHostedSessionRow: wsdashboard.RegistrationPath(h.dir, id)} {
		if got, _ := specEnv(spec, name); got != want {
			t.Fatalf("env %s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"WS_PI_ROLE", "WS_MAILBOX", "WS_MCP_ROOT"} {
		if v, ok := specEnv(spec, name); ok {
			t.Fatalf("env %s=%q must not reach a lead's harness", name, v)
		}
	}
	prompts := func() []string { return h.holders.inputs(holder, "prompt") }
	if got := prompts(); len(got) != 1 || !sameJSON(t, json.RawMessage(got[0]), []byte(`{"type":"prompt","message":"hello"}`)) {
		t.Fatalf("prompt lines = %q", got)
	}
	// A tool loadout no profile lists is the user's own, not a refusal.
	h.holders.out(holder,
		`{"type":"agent_start"}`,
		`{"type":"message_end","message":{"role":"system","toolsAdded":[{"name":"bash"},{"name":"some_extension_tool"}]}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled"}`)
	h.idle(id)
	if st := h.state(id); !st.Process || st.Error != "" {
		t.Fatalf("after a pi turn: %+v", st)
	}
	if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, []string{"hi"}) {
		t.Fatalf("ring replies = %q", got)
	}

	// A run nobody sent (a pushed batch) is a turn.
	h.holders.out(holder, `{"type":"agent_start"}`)
	eventually(t, "the unsolicited run", func() bool { return h.state(id).Status == StatusRunning })
	h.holders.out(holder, `{"type":"agent_settled"}`)
	h.idle(id)

	// /compact is the compact command, and its answer ends the turn.
	h.send(id, "/compact keep the names")
	eventually(t, "the compact command", func() bool { return len(h.holders.inputs(holder, "compact")) == 1 })
	if got := h.holders.inputs(holder, "compact")[0]; !sameJSON(t, json.RawMessage(got), []byte(`{"type":"compact","customInstructions":"keep the names"}`)) || len(prompts()) != 1 {
		t.Fatalf("compact line = %s (prompts %q)", got, prompts())
	}
	if st := h.state(id); st.Status != StatusRunning {
		t.Fatalf("a compaction is a turn: %+v", st)
	}
	h.holders.out(holder, `{"type":"response","command":"compact","success":true}`)
	h.idle(id)
	h.send(id, "/compact")
	eventually(t, "the second compact command", func() bool { return len(h.holders.inputs(holder, "compact")) == 2 })
	h.holders.out(holder, `{"type":"response","command":"compact","success":false,"error":"nothing to compact"}`)
	eventually(t, "the refused compaction to end the turn with its error", func() bool {
		st := h.state(id)
		return st.Status == StatusIdle && strings.Contains(st.Error, "nothing to compact")
	})

	// Idle stop is off for a pi lead unless its profile sets one.
	var idleAfter time.Duration
	c := h.runner.conv(id)
	c.do(func() { idleAfter = c.idleAfter })
	if idleAfter != 0 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a pi lead has no idle stop by default: %v, holder calls %v", idleAfter, h.holders.opList())
	}
}

// TestPiLeadSubagentRows: the subagent widget a hosted pi lead sets
// reaches the session's snapshot as subagent rows, through the runner's
// own projection, and the rows still running end failed when the lead's
// process exits, as a claude lead's subagents do.
func TestPiLeadSubagentRows(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "hello")
	holder := h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`,
		string(piWidgetLine(piAgentsWidgetKey,
			`{"kind":"agent","agentId":"child-1","label":"audit","status":"running","caption":"worker","batches":[],"model":"gpt-x","effort":"high"}`,
			`{"kind":"agent","agentId":"child-2","label":"probe","status":"running","caption":"explore","batches":[]}`,
			`{"kind":"reset"}`,
		)),
		string(piWidgetLine(piAgentsWidgetKey, `{"kind":"agent","agentId":"child-2","status":"done"}`)),
		`{"type":"agent_settled"}`)
	h.idle(id)
	rows := func() map[string]wslive.Agent {
		out := map[string]wslive.Agent{}
		for _, a := range h.snapshot(id).Agents {
			out[a.ID] = a
		}
		return out
	}
	got := rows()
	// The widget's reset line was skipped: it would have dropped both rows.
	if a := got["child-1"]; len(got) != 2 || a.Status != wslive.StatusRunning || a.Label != "audit" || a.Model != "gpt-x" || a.Effort != "high" || got["child-2"].Status != wslive.StatusDone {
		t.Fatalf("subagent rows = %+v", got)
	}

	h.holders.exit(holder, 0)
	eventually(t, "the process's end", func() bool { return !h.state(id).Process })
	got = rows()
	if got["child-1"].Status != wslive.StatusFailed || got["child-2"].Status != wslive.StatusDone {
		t.Fatalf("rows after the process ended = %+v", got)
	}
}

// TestPiLeadMapper pins what a pi lead's mapper does differently from a
// profile conversation's.
func TestPiLeadMapper(t *testing.T) {
	spec := StartSpec{Root: "/work", Dir: "/sess", Profile: Profile{Backend: BackendPi, Lead: true}, Lead: &LeadSpec{}}
	args := piLeadArgs(spec)
	if session, _ := flagValue(args, "--session"); len(args) != 6 || args[0] != "--mode" || args[1] != "rpc" || session != piSessionFile("/sess") {
		t.Fatalf("piLeadArgs = %v", args)
	}
	spec.Model, spec.Effort = "anthropic/claude-x", "high"
	args = piLeadArgs(spec)
	if model, _ := flagValue(args, "--model"); model != "anthropic/claude-x" {
		t.Fatalf("piLeadArgs = %v", args)
	}
	if thinking, _ := flagValue(args, "--thinking"); thinking != "high" {
		t.Fatalf("piLeadArgs = %v", args)
	}

	lead := newPiMapper(spec)
	if evs := lead.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{}); len(evs) != 1 || !evs[0].TurnStart {
		t.Fatalf("a lead's agent_start = %+v", evs)
	}
	if evs := lead.mapLine([]byte(`{"type":"response","command":"compact","success":true}`), nopReplies{}); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("a lead's compact answer = %+v", evs)
	}
	loadout := []byte(`{"type":"message_end","message":{"role":"system","toolsAdded":[{"name":"some_extension_tool"}]}}`)
	if evs := lead.mapLine(loadout, nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.Fatal != nil }) != 0 {
		t.Fatalf("a lead's loadout is not checked: %+v", evs)
	}

	plain := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	if evs := plain.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation's turns start at its sends: %+v", evs)
	}
	if evs := plain.mapLine([]byte(`{"type":"response","command":"compact","success":true}`), nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation sends no compact command: %+v", evs)
	}
	if evs := plain.mapLine(loadout, nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.Fatal != nil }) != 1 {
		t.Fatalf("a profile conversation's loadout is checked: %+v", evs)
	}

	// Idle stop is off for a pi lead unless its profile sets a timeout: pi
	// reports no background tasks to wait for.
	seconds := 90
	for want, p := range map[time.Duration]Profile{
		0:                  {Backend: BackendPi, Lead: true},
		90 * time.Second:   {Backend: BackendPi, Lead: true, IdleTimeoutSeconds: &seconds},
		DefaultIdleTimeout: {Backend: BackendClaude, Lead: true},
	} {
		if got := p.IdleTimeout(); got != want {
			t.Errorf("%+v idle timeout = %v, want %v", p, got, want)
		}
	}

	for text, want := range map[string]string{"/compact": "", "/compact keep x": "keep x", " /compact ": ""} {
		if rest, ok := piCompactCommand(text); !ok || rest != want {
			t.Errorf("piCompactCommand(%q) = %q, %v", text, rest, ok)
		}
	}
	for _, text := range []string{"/compacted", "please /compact", "/compact\nand more"} {
		if _, ok := piCompactCommand(text); ok {
			t.Errorf("piCompactCommand(%q) should not match", text)
		}
	}
}

// ringKinds counts the ring's entries of kind.
func ringKinds(ring []wslive.RingEntry, kind string) int {
	n := 0
	for _, e := range ring {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// TestLeadFailedSendShowsTheMessageOnce: a send that fails leaves no user
// entry or turn line of its own, so its retry on a new process shows the
// message once, in one turn.
func TestLeadFailedSendShowsTheMessageOnce(t *testing.T) {
	h := newLeadHarness(t).start()
	h.holders.failInputs("user", 1)
	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "one")
	var second string
	eventually(t, "the message on the retry's process", func() bool {
		second = h.record(id).HolderID
		return second != "" && slices.Equal(h.userInputs(second), []string{"one"})
	})
	if n := h.holders.opCount("spawn"); n != 2 {
		t.Fatalf("one failed start and one retry: %d spawns (%v)", n, h.holders.opList())
	}
	h.holders.out(second, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	ring := h.ring(id)
	if got := ringTexts(ring, wslive.RingUser); !slices.Equal(got, []string{"one"}) || ringKinds(ring, wslive.RingTurn) != 1 {
		t.Fatalf("one user entry and one turn: users %q, ring %+v", got, ring)
	}
	if st := h.state(id); st.Error != "" || st.Held != 0 {
		t.Fatalf("after the retried turn: %+v", st)
	}
}

// TestLeadFailedMailOnlySendRetriesOnce: a turn of mail alone whose send
// fails keeps its mail and is retried once on a new process; the send that
// goes through clears it. A send that always fails is retried once, then
// waits for the next signal.
func TestLeadFailedMailOnlySendRetriesOnce(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	key := h.record(id).WSKey
	eventually(t, "the mail watch", func() bool { return h.mail.watched(key) })
	h.holders.failInputs("user", 1)
	h.mail.deliver(key, "From a@x: ping")
	var holder string
	eventually(t, "the mail on the retry's process", func() bool {
		holder = h.record(id).HolderID
		return holder != "" && slices.Equal(h.userInputs(holder), []string{"From a@x: ping"})
	})
	if n := h.holders.opCount("spawn"); n != 2 {
		t.Fatalf("one failed start and one retry: %d spawns (%v)", n, h.holders.opList())
	}
	eventually(t, "the mail to be cleared", func() bool { return len(h.record(id).Mail) == 0 })
	h.holders.out(holder, turnLines("sess-1", "u1", "pong")...)
	h.idle(id)
	ring := h.ring(id)
	if got := ringTexts(ring, wslive.RingUser); len(got) != 0 || ringKinds(ring, wslive.RingTurn) != 1 {
		t.Fatalf("a mail turn has no user entry and shows once: users %q, ring %+v", got, ring)
	}

	h.holders.failInputs("user", -1)
	h.mail.deliver(key, "From a@x: again")
	spawns := h.holders.opCount("spawn")
	// The idle process takes the mail, fails, and ends; the retry starts a
	// new one, which fails too.
	eventually(t, "the retry to fail", func() bool {
		st := h.state(id)
		return h.holders.opCount("spawn") == spawns+1 && !st.Process && strings.Contains(st.Error, "send turn")
	})
	// A retry the second failure armed would be signaled before this
	// request, and handled first.
	c := h.runner.conv(id)
	c.do(func() {})
	if n := h.holders.opCount("spawn"); n != spawns+1 || !slices.Equal(h.record(id).Mail, []string{"From a@x: again"}) {
		t.Fatalf("a send that always fails is retried once: %d spawns after %d, mail %q", n, spawns, h.record(id).Mail)
	}
	if st := h.state(id); st.Status == StatusRunning {
		t.Fatalf("a failed send runs no turn: %+v", st)
	}
}

// TestPiLeadFailedCommandRetriesOnce: a dashboard command whose send fails
// (runCommand) stays queued and runs once on a new process, shown once.
func TestPiLeadFailedCommandRetriesOnce(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.holders.failInputs("compact", 1)
	h.sendCommand(id, "compact", "keep the names")
	var holder string
	eventually(t, "the compact command on the retry's process", func() bool {
		holder = h.record(id).HolderID
		return holder != "" && len(h.holders.inputs(holder, "compact")) == 1
	})
	if n := h.holders.opCount("spawn"); n != 2 {
		t.Fatalf("one failed start and one retry: %d spawns (%v)", n, h.holders.opList())
	}
	h.holders.out(holder, `{"type":"response","command":"compact","success":true}`)
	h.idle(id)
	ring := h.ring(id)
	if got := ringTexts(ring, wslive.RingUser); !slices.Equal(got, []string{"/compact keep the names"}) || ringKinds(ring, wslive.RingTurn) != 1 || h.state(id).Held != 0 {
		t.Fatalf("the command shows once, in one turn: users %q, state %+v", got, h.state(id))
	}
}
