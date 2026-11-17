package wsagent

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
)

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

// replayStatusGate keeps the rejoin's status answer pending until the test
// has observed the loop drain the replay. No holder call timeout runs while
// the gate is closed: draining a slow replay is not itself a deadlock.
type replayStatusGate struct {
	HolderHost
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (g *replayStatusGate) Status(id string) (wsholder.AttachInfo, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.HolderHost.Status(id)
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
	target := uint64(h.holders.records(holder))
	entered, gate := make(chan struct{}), make(chan struct{})
	var released sync.Once
	release := func() { released.Do(func() { close(gate) }) }
	defer release()
	h.opts.Holders = &replayStatusGate{HolderHost: h.holders, entered: entered, release: gate}
	h.start()
	wait(t, "the rejoin's status call", entered)
	// This bounded round trip fails if Status holds the conversation loop.
	h.quiesce(id)
	c := h.runner.conv(id)
	eventually(t, "the loop to drain the replay while status waits", func() bool {
		var seq uint64
		c.do(func() { seq = c.seq })
		return seq >= target
	})
	release()
	h.send(id, "two")
	eventually(t, "the rejoin to end and take the message", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
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
	eventually(t, "the held messages", func() bool {
		return slices.Equal(h.userInputs("h2"), []string{h.envelope("two") + "\n\n" + h.envelope("three")})
	})
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
	eventually(t, "the next daemon to rejoin the process", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
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
	eventually(t, "the rejoin to take the message", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
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

// TestPiLeadIdleStopWaitsForBackgroundShell verifies the hosted count gates idle stop.
func TestPiLeadIdleStopWaitsForBackgroundShell(t *testing.T) {
	h := newLeadHarness(t)
	h.globalProfile(LeadProfilePi, `{"backend":"pi","lead":true,"idle_timeout_seconds":1}`)
	h.start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":1,"monitors":0}`)), `{"type":"agent_settled"}`)
	h.idle(id)
	eventually(t, "background shell count", func() bool { return h.state(id).BackgroundTasks == 1 })
	time.Sleep(1600 * time.Millisecond)
	if st := h.state(id); !st.Process || st.BackgroundTasks != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a background shell keeps the process alive: %+v, holder calls %v", st, h.holders.opList())
	}
	h.holders.out(holder, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":0,"monitors":0}`)))
	eventually(t, "the idle stop", func() bool { return !h.state(id).Process && h.holders.opCount("stop "+holder) == 1 })
}

// TestPiLeadIdleStopAndMailWake verifies a stopped Pi lead wakes for mail.
func TestPiLeadIdleStopAndMailWake(t *testing.T) {
	h := newLeadHarness(t)
	h.globalProfile(LeadProfilePi, `{"backend":"pi","lead":true,"idle_timeout_seconds":1}`)
	h.start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	eventually(t, "the idle stop", func() bool { return !h.state(id).Process && h.holders.opCount("stop "+holder) == 1 })
	h.mail.deliver(h.record(id).WSKey, "From a@x: ping")
	eventually(t, "mail to restart the lead", func() bool {
		newHolder := h.record(id).HolderID
		if newHolder == "" || newHolder == holder {
			return false
		}
		prompts := h.holders.inputs(newHolder, "prompt")
		return len(prompts) == 1 && sameJSON(t, json.RawMessage(prompts[0]), []byte(`{"type":"prompt","message":"From a@x: ping"}`))
	})
}

// TestPiLeadTurnsAndCompact: a pi lead runs the user's own loadout (no
// off switches on its command line, no tool-loadout check), a run that
// starts with no send opens a turn, and a typed /compact is the RPC compact
// command whose answer ends the turn.
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
	if got := prompts(); len(got) != 1 || !sameJSON(t, json.RawMessage(got[0]), mustJSON(t, map[string]string{"type": "prompt", "message": h.envelope("hello")})) {
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

	// Pi leads do not idle stop unless their profile explicitly opts in.
	var idleAfter time.Duration
	c := h.runner.conv(id)
	c.do(func() { idleAfter = c.idleAfter })
	if idleAfter != 0 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a pi lead's idle stop = %v, want 0; holder calls %v", idleAfter, h.holders.opList())
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
	if evs := lead.mapLine([]byte(`{"type":"response","command":"compact","success":true}`), nopReplies{}); len(evs) != 0 {
		t.Fatalf("a compact answer must not settle the lead's active native run: %+v", evs)
	}
	lead.mapLine([]byte(`{"type":"agent_settled"}`), nopReplies{})
	if evs := lead.mapLine([]byte(`{"type":"response","command":"compact","success":true}`), nopReplies{}); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("an idle lead's compact answer = %+v", evs)
	}
	loadout := []byte(`{"type":"message_end","message":{"role":"system","toolsAdded":[{"name":"some_extension_tool"}]}}`)
	if evs := lead.mapLine(loadout, nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.Fatal != nil }) != 0 {
		t.Fatalf("a lead's loadout is not checked: %+v", evs)
	}

	plain := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	if evs := plain.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{}); len(evs) != 1 || !evs[0].TurnStart {
		t.Fatalf("a worker's agent_start rebuilds its held turn: %+v", evs)
	}
	if evs := plain.mapLine([]byte(`{"type":"response","command":"compact","success":true}`), nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation sends no compact command: %+v", evs)
	}
	if evs := plain.mapLine(loadout, nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.Fatal != nil }) != 1 {
		t.Fatalf("a profile conversation's loadout is checked: %+v", evs)
	}

	// Explicit timeouts, including zero, override the lead default.
	seconds, zero := 90, 0
	for _, tc := range []struct {
		profile Profile
		want    time.Duration
	}{
		{Profile{Backend: BackendPi, Lead: true, IdleTimeoutSeconds: &zero}, 0},
		{Profile{Backend: BackendPi, Lead: true, IdleTimeoutSeconds: &seconds}, 90 * time.Second},
		{Profile{Backend: BackendPi, Lead: true}, 0},
	} {
		if got := tc.profile.IdleTimeout(); got != tc.want {
			t.Errorf("%+v idle timeout = %v, want %v", tc.profile, got, tc.want)
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
		return second != "" && slices.Equal(h.userInputs(second), h.envelopes("one"))
	})
	if n := h.holders.opCount("spawn"); n != 2 {
		t.Fatalf("one failed start and one retry: %d spawns (%v)", n, h.holders.opList())
	}
	h.holders.out(second, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	ring := h.ring(id)
	if got := ringTexts(ring, wslive.RingUser); !slices.Equal(got, h.envelopes("one")) || ringKinds(ring, wslive.RingTurn) != 1 {
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

// TestLeadTurnEndsAtResultWhileABackgroundTaskRuns: a claude lead whose
// turn leaves a background task running gets no idle state until the task
// ends (Claude 2.1.323), so the result ends the turn: the lead is idle
// with the task still counted, the next message goes at once as a turn of
// its own, each turn writes one turn entry, and the idle state the task's
// completion turn ends with adds none.
func TestLeadTurnEndsAtResultWhileABackgroundTaskRuns(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), tasksLine(1), replyLine("u1", "started"), resultLine())
	h.idle(id)
	snap := h.snapshot(id)
	if snap.Lead.TurnStartedAt != 0 || snap.Lead.Phase != wslive.PhaseIdle || snap.Host == nil || snap.Host.BackgroundTasks != 1 {
		t.Fatalf("the lead is idle beside its running task: lead %+v, host %+v", snap.Lead, snap.Host)
	}

	h.send(id, "two")
	eventually(t, "the second message as a turn", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
	h.holders.out(holder, initLine("sess-1", "auto"), replyLine("u2", "ok2"), resultLine())
	h.idle(id)
	if got := ringKinds(h.ring(id), wslive.RingTurn); got != 2 {
		t.Fatalf("one turn entry per message, got %d", got)
	}

	// The task's completion turn: its result comes with no task left, and
	// the idle state after it ends it.
	h.holders.out(holder, tasksLine(0), initLine("sess-1", "auto"), replyLine("u3", "the task finished"), resultLine())
	eventually(t, "the completion turn", func() bool { return ringKinds(h.ring(id), wslive.RingReply) == 3 })
	if st := h.state(id); st.Status != StatusRunning {
		t.Fatalf("with no task left the result defers to the idle state: %+v", st)
	}
	h.holders.out(holder, stateLine(HarnessIdle))
	h.idle(id)
	ring := h.ring(id)
	if got := ringKinds(ring, wslive.RingTurn); got != 3 {
		t.Fatalf("three turns, three turn entries, got %d: %+v", got, ring)
	}
	if got := ringTexts(ring, wslive.RingUser); !slices.Equal(got, h.envelopes("one", "two")) {
		t.Fatalf("user entries = %q", got)
	}
	if st := h.state(id); st.Error != "" || st.Held != 0 || st.BackgroundTasks != 0 {
		t.Fatalf("after the completion turn: %+v", st)
	}
}
