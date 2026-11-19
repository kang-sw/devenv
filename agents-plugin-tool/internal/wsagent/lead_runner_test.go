package wsagent

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
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
// mapper sends at a turn's init and the initialize ask a launch sends, both
// on their own.
func (h *leadHarness) controls(holder string) []any {
	var out []any
	for _, line := range h.holders.inputs(holder, "control_request") {
		id, request := controlRequest(h.t, []byte(line))
		if strings.HasPrefix(id, claudeContextAsk) || id == claudeCommandsAsk {
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
// and spawns the harness under it with the hosted-session environment and
// without the native subagent tool, the session's row names the spawned
// process, and a turn runs end to end through the real backend and mapper.
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
	for flag, want := range map[string]string{"--permission-prompt-tool": "stdio", "--permission-mode": "plan", "--model": "opus", "--effort": "high", "--disallowedTools": "Agent,Task"} {
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
	if rec.WSKey == "" || rec.WSKey == "the-daemons-own" || !slices.Contains(h.ensured(), rec.WSKey) {
		t.Fatalf("the lead should run on its own minted key, kept usable: %q, ensured %v", rec.WSKey, h.ensured())
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
	if got := h.userInputs(holder); !slices.Equal(got, h.envelopes("hello")) {
		t.Fatalf("user turns written = %q", got)
	}

	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "plan"), replyLine("u1", "hi there"), resultLine(), stateLine(HarnessIdle))
	h.idle(st.ID)
	ring := h.ring(st.ID)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), h.envelopes("hello")) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"hi there"}) {
		t.Fatalf("ring = %+v", ring)
	}
	if st := h.state(st.ID); !st.Process || st.ActiveMode != "plan" || st.Error != "" || st.Held != 0 {
		t.Fatalf("after a turn: %+v", st)
	}
	if got := h.record(st.ID).SessionID; got != "sess-1" {
		t.Fatalf("recorded backend session = %q", got)
	}
}

// TestClaudeLeadPromptCarriesHostedKey: a hosted Claude lead learns its
// session key from its own system prompt, not only from the plugin's hooks
// (which a policy may block): every launch, the resume included, appends
// an instruction file naming the key and the workflow manual call.
func TestClaudeLeadPromptCarriesHostedKey(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	prompt := func(argv []string) string {
		t.Helper()
		path, ok := flagValue(argv, "--append-system-prompt-file")
		if !ok {
			t.Fatalf("a Claude lead's launch appends no instruction file: %v", argv)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	check := func(text string) {
		t.Helper()
		for _, want := range []string{"`" + key + "`", "`session_key`", "do not mint a replacement", "workflow_manual", "after any compaction"} {
			if !strings.Contains(text, want) {
				t.Fatalf("instruction file lacks %q:\n%s", want, text)
			}
		}
	}
	check(prompt(h.holders.spawned(holder).Argv))

	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "two")
	var second string
	eventually(t, "a new holder", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	argv := h.holders.spawned(second).Argv
	if got, ok := flagValue(argv, "--resume"); !ok || got != "sess-1" {
		t.Fatalf("the next process resumes the session: %v", argv)
	}
	check(prompt(argv))
}

// leadManualRender is a fake Options.LeadManual: every call renders text
// (or fails with err) and is kept with its arguments.
type leadManualRender struct {
	mu    sync.Mutex
	calls [][3]string
	text  string
	err   error
}

func (m *leadManualRender) render(backend, dir, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, [3]string{backend, dir, key})
	return m.text, m.err
}

func (m *leadManualRender) list() [][3]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.calls)
}

// appendedPrompt is the instruction file a Claude lead's launch appends to
// its system prompt.
func appendedPrompt(t *testing.T, argv []string) string {
	t.Helper()
	path, ok := flagValue(argv, "--append-system-prompt-file")
	if !ok {
		t.Fatalf("a Claude lead's launch appends no instruction file: %v", argv)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// leadKeySentence opens every hosted lead's appended prompt.
func leadKeySentence(key string) string {
	return "Your ws session key is `" + key + "`. Pass it as the `session_key` argument of every ws tool call. " +
		"This hosted session already has a key; do not mint a replacement (no ferrule call, no fresh-bootstrap key). "
}

// leadManualOnly closes the appended prompt of a lead whose launch has no
// rendered manual: the lead is told to call for it.
const leadManualOnly = "At session start, call the workflow_manual tool with this key and follow it; after any compaction, call workflow_state for fresh session state.\n"

// TestClaudeLeadPromptCarriesTheWorkflowManual: a hosted Claude lead's
// appended prompt holds its key sentence, the binding-manual sentence,
// and the manual the daemon rendered for its harness, working directory,
// and key. The render runs once per process start, a resume included: a
// later turn and a rejoin render nothing.
func TestClaudeLeadPromptCarriesTheWorkflowManual(t *testing.T) {
	h := newLeadHarness(t)
	manual := &leadManualRender{text: "\n# Workflow Manual\n\n## Session Key\nrendered\n\n"}
	h.opts.LeadManual = manual.render
	h.start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	check := func(argv []string) {
		t.Helper()
		text := appendedPrompt(t, argv)
		if !strings.HasPrefix(text, leadKeySentence(key)+"The workflow manual below is binding;") ||
			!strings.Contains(text, "After any compaction, or when you need fresh session state, call the workflow_state tool with this key; the manual stays in this prompt.") ||
			!strings.HasSuffix(text, ".\n\n# Workflow Manual\n\n## Session Key\nrendered\n") || strings.Contains(text, leadManualOnly) {
			t.Fatalf("the appended prompt is not the key sentence and the manual:\n%s", text)
		}
	}
	check(h.holders.spawned(holder).Argv)
	calls := manual.list()
	if len(calls) != 1 || calls[0][0] != BackendClaude || !sameDir(calls[0][1], h.root) || calls[0][2] != key {
		t.Fatalf("the manual renders once, for the lead's harness, directory, and key: %v", calls)
	}

	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.restart()
	if got := h.holder(id); got != holder {
		t.Fatalf("the rejoin replaced the process: %q -> %q", holder, got)
	}
	h.send(id, "two")
	eventually(t, "the second message", func() bool { return len(h.userInputs(holder)) == 2 })
	if n := len(manual.list()); n != 1 {
		t.Fatalf("a rejoin or a turn rendered the manual again: %d renders", n)
	}

	h.holders.out(holder, turnLines("sess-1", "u2", "second")...)
	h.idle(id)
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "three")
	var second string
	eventually(t, "a new holder", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	check(h.holders.spawned(second).Argv)
	if n := len(manual.list()); n != 2 {
		t.Fatalf("the next process start renders the manual again: %d renders", n)
	}
}

// TestLeadPromptWithoutAManualKeepsTheKeySentence: a lead whose manual
// does not render (the render fails, or renders nothing) starts with its
// key sentence and the call to fetch the manual, and the failure is
// logged, not a launch error.
func TestLeadPromptWithoutAManualKeepsTheKeySentence(t *testing.T) {
	for name, manual := range map[string]*leadManualRender{
		"error": {text: "partial", err: errors.New("manual render broke")},
		"blank": {text: " \n\t\n"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newLeadHarness(t)
			h.opts.LeadManual = manual.render
			h.start()
			id, holder := h.runLead("one")
			if text := appendedPrompt(t, h.holders.spawned(holder).Argv); text != leadKeySentence(h.record(id).WSKey)+leadManualOnly {
				t.Fatalf("the appended prompt is not the key sentence alone:\n%s", text)
			}
			if st := h.state(id); !st.Process || st.Error != "" {
				t.Fatalf("a manual that did not render is no launch error: %+v", st)
			}
			if logged := strings.Contains(h.log.String(), "workflow manual not rendered, the lead's prompt carries its key alone: manual render broke"); logged != (manual.err != nil) {
				t.Fatalf("render failure logged = %v:\n%s", logged, h.log.String())
			}
		})
	}
}

// TestClaudeLeadWithoutWSMCPIsRefused: a Claude lead loads no ws plugin,
// so a start the daemon has no ws-mcp for (no launcher configured, or one
// that does not resolve) is refused before any process runs, its reason
// logged and shown.
func TestClaudeLeadWithoutWSMCPIsRefused(t *testing.T) {
	for name, launcher := range map[string]func(string) (MCPCommand, error){
		"none":       nil,
		"unresolved": func(string) (MCPCommand, error) { return MCPCommand{}, errors.New("launcher unresolvable") },
	} {
		t.Run(name, func(t *testing.T) {
			h := newLeadHarness(t)
			h.opts.WSMCP = launcher
			h.start()
			id := h.lead(BackendClaude, LeadOptions{}).ID
			h.send(id, "one")
			eventually(t, "the refused start", func() bool { return strings.Contains(h.state(id).Error, "no ws-mcp to attach") })
			h.quiesce(id)
			if n := h.holders.opCount("spawn"); n != 0 || h.state(id).Process {
				t.Fatalf("a refused start runs no process: %v", h.holders.opList())
			}
			if !strings.Contains(h.log.String(), "ws-mcp unavailable, the start is refused") {
				t.Fatalf("the refusal is not logged:\n%s", h.log.String())
			}
			// The shown refusal names why the ws-mcp did not resolve.
			if launcher != nil && !strings.Contains(h.state(id).Error, "launcher unresolvable") {
				t.Fatalf("the refusal hides its cause: %q", h.state(id).Error)
			}
		})
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
	eventually(t, "the held message to be submitted", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
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
	if !slices.Equal(ringTexts(ring, wslive.RingUser), h.envelopes("one", "two")) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"first", "second"}) {
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
	eventually(t, "the held message to be submitted", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
	// Input is visible before its holder call returns and kick clears the
	// queued message. Observe the completed loop step, not that early write.
	h.quiesce(id)
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
	eventually(t, "the steered message", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "now")) })
	h.runner.conv(id).do(func() {})
	if st := h.state(id); st.Held != 1 || st.Status != StatusRunning || !st.Process {
		t.Fatalf("the unmarked message stays held and the turn runs on: %+v", st)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, h.envelopes("one", "now")) {
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
		return slices.Equal(h.userInputs(holder), h.envelopes("one", "now", "later"))
	})
	h.holders.out(holder, turnLines("sess-1", "u3", "done")...)
	h.idle(id)
	ring := h.ring(id)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), h.envelopes("one", "now", "later")) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"changed course", "done"}) {
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
	if got := h.holders.inputs(holder, "steer")[0]; !sameJSON(t, json.RawMessage(got), mustJSON(t, map[string]string{"type": "steer", "message": h.envelope("now")})) || len(h.holders.inputs(holder, "prompt")) != 1 {
		t.Fatalf("steer line = %s, prompts %q", got, h.holders.inputs(holder, "prompt"))
	}
	if st := h.state(id); st.Held != 0 || st.Status != StatusRunning {
		t.Fatalf("after the steer: %+v", st)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, h.envelopes("hello", "now")) {
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
	if got := p.nextSent(t); got != h.envelope("first") {
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
	if got := p.nextSent(t); got != h.envelope("now") {
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

	// Mail that arrives during a turn is steered into it; the dashboard's
	// message waits for the turn's end.
	h.send(id, "question")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.mail.deliver(key, "From b@x: more")
	eventually(t, "the steered mail", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"From a@x: ping", h.envelope("question"), "From b@x: more"})
	})
	h.send(id, "also")
	eventually(t, "the message to be held", func() bool { return h.state(id).Held == 1 })
	if rec := h.record(id); len(rec.Mail) != 0 {
		t.Fatalf("steered mail is not kept for the next turn: %+v", rec.Mail)
	}
	h.holders.out(holder, replyLine("u2", "answer"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the next turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"From a@x: ping", h.envelope("question"), "From b@x: more", h.envelope("also")})
	})
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, h.envelopes("question", "also")) {
		t.Fatalf("ring user entries = %q", got)
	}

	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the mailbox watch to end", func() bool { return h.mail.stopped() == 2 })
}

// TestLeadMailClaimedDuringATurnTellsThePage: mail claimed while a turn
// runs is steered into it and starts no turn, so the claim itself must
// tell the page: its mail history changed.
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
	eventually(t, "the claim to be steered", func() bool {
		return slices.Equal(h.userInputs(holder), append(h.envelopes("question"), "From b@x: more"))
	})
	eventually(t, "the page to be told of the claim", func() bool { return h.notes.Load() > settled })
	if st := h.state(id); st.Held != 0 {
		t.Fatalf("steered mail is not held: %+v", st)
	}
}

// TestLeadMailWaitsForTheRejoin: mail that waits in the session's mailbox
// when a daemon starts must not start a process while the session's own
// still runs under its holder, and is not steered into a turn the replay
// has not shown yet: it is delivered to that process at the turn's end.
func TestLeadMailWaitsForTheRejoin(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()
	h.mail.deliver(key, "From a@x: ping")
	h.mail.deliver(key, "From b@x: more")
	h.start()
	eventually(t, "the waiting mail to be claimed", func() bool {
		return slices.Equal(h.record(id).Mail, []string{"From a@x: ping", "From b@x: more"})
	})
	eventually(t, "the running turn to be back", func() bool { st := h.state(id); return st.Process && st.Status == StatusRunning })
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 2 {
		t.Fatalf("mail waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	h.holders.out(holder, replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the mail turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{h.envelope("one"), "From a@x: ping\n\nFrom b@x: more"})
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
