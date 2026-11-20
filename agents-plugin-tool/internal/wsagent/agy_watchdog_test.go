package wsagent

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
)

func agyWatchdogHarness(t *testing.T, timeoutSeconds string) (*harness, *leverTimers, *reports) {
	t.Helper()
	previous := suiteBackend
	suiteBackend = BackendAgy
	t.Cleanup(func() { suiteBackend = previous })

	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	writeStructured(t, filepath.Join(h.root, ".ws-workflow", "config.json"), map[string]string{
		ProfilePrefix + "chat": strings.Replace(chatProfile, `"backend":"claude"`, `"backend":"agy"`, 1),
	})
	resolver := wsconfig.NewResolver(h.config.Config, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemAgyModelWaitTimeoutSeconds, timeoutSeconds, wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	timers := &leverTimers{}
	got := &reports{}
	h.config.Timer = timers.start
	h.config.Report = got.add
	h.start()
	return h, timers, got
}

func startAgyWatchdogTurn(t *testing.T, h *harness, withParent bool) (string, *fakeProc) {
	t.Helper()
	id := h.create("chat").ID
	if withParent {
		c := h.runner.conv(id)
		c.do(func() {
			parent := leadParent
			c.update(func(rec *Record) { rec.Parent = &parent })
		})
	}
	if _, err := h.runner.Send(id, "work"); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	return id, p
}

func procInterrupts(p *fakeProc) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interrupts
}

func fireWatchdog(t *testing.T, timers *leverTimers, index int) {
	t.Helper()
	eventually(t, "watchdog timer", func() bool {
		started, _ := timers.counts()
		return started > index
	})
	timers.fire(t, index)
}

// (a), (b): the first silent interval is cancelled and retried in the same
// logical turn; the second is a failed turn reported to the parent.
func TestAgyModelWaitWatchdogRetriesOnceAndReportsSecondStall(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	id, p := startAgyWatchdogTurn(t, h, true)

	fireWatchdog(t, timers, 0)
	eventually(t, "first interrupt", func() bool { return procInterrupts(p) == 1 })
	if st := h.state(id); st.Status != StatusRunning {
		t.Fatalf("the logical turn must stay open while cancellation lands: %+v", st)
	}
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	if got := p.nextSent(t); got != "continue" {
		t.Fatalf("retry prompt = %q, want continue", got)
	}
	if st := h.state(id); st.Status != StatusRunning || len(got.lead()) != 0 {
		t.Fatalf("the retry stays in the same unreported logical turn: state=%+v reports=%+v", st, got.lead())
	}

	fireWatchdog(t, timers, 1)
	eventually(t, "second interrupt", func() bool { return procInterrupts(p) == 2 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "failed parent report", func() bool { return len(got.lead()) == 1 })
	report := got.lead()[0]
	if report.Result.Status != TurnFailed || report.Result.Error != "agy model stream stalled for 45s twice" {
		t.Fatalf("second-stall report = %+v", report.Result)
	}
	if rec := h.record(id); rec.TurnOpen || rec.LastTurn == nil || rec.LastTurn.Result.Status != TurnFailed {
		t.Fatalf("the failed logical turn must be settled in the record: %+v", rec)
	}
}

// (c): every open tool call pauses the timer, including an IsDaemon
// run_command that the older background-command set deliberately omits.
func TestAgyModelWaitWatchdogPausesForPendingDaemonTool(t *testing.T) {
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	m.promptSent("ws-p-watchdog")
	started := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"tool_call","toolCallId":"daemon1","name":"run_command","status":"in_progress","args":{"CommandLine":"npm run dev","IsDaemon":true}}}}`), rr)
	if !slices.ContainsFunc(started, func(ev Event) bool { return ev.ModelActivity != nil && *ev.ModelActivity == 1 }) {
		t.Fatalf("daemon tool start must count as pending model activity: %+v", started)
	}
	completed := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"tool_call_update","toolCallId":"daemon1","name":"run_command","status":"completed"}}}`), rr)
	if !slices.ContainsFunc(completed, func(ev Event) bool { return ev.ModelActivity != nil && *ev.ModelActivity == 0 }) {
		t.Fatalf("daemon tool completion must resume model waiting: %+v", completed)
	}

	h, timers, _ := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, false)
	one := 1
	p.events <- Event{ModelActivity: &one}
	eventually(t, "paused watchdog", func() bool {
		_, stopped := timers.counts()
		return stopped > 0
	})
	timers.fire(t, 0) // a cancelled fake timer may still deliver; it is no longer selected
	time.Sleep(60 * time.Millisecond)
	if got := procInterrupts(p); got != 0 {
		t.Fatalf("pending daemon tool triggered %d interrupts", got)
	}
	zero := 0
	p.events <- Event{ModelActivity: &zero}
	fireWatchdog(t, timers, 1)
	eventually(t, "interrupt after daemon completion", func() bool { return procInterrupts(p) == 1 })
}

func TestAgyModelWaitWatchdogFailureDoesNotRetryPendingToolOrApproval(t *testing.T) {
	for _, wait := range []string{"tool", "approval"} {
		t.Run(wait, func(t *testing.T) {
			h, timers, got := agyWatchdogHarness(t, "45")
			_, p := startAgyWatchdogTurn(t, h, true)
			if wait == "tool" {
				one := 1
				p.events <- Event{ModelActivity: &one}
			} else {
				p.events <- Event{Approval: &ApprovalRequest{ID: "approval-1", Tool: "run_command"}}
			}
			failure := "⚠ API error (attempt 1): while waiting"
			p.events <- Event{ModelFailureUpdate: &failure}
			time.Sleep(60 * time.Millisecond)
			if started, _ := timers.counts(); started != 1 {
				t.Fatalf("failure while %s waiting started %d timers", wait, started)
			}
			p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: failure, Final: true}}
			if wait == "approval" {
				p.events <- Event{ApprovalDone: "approval-1"}
			}
			p.events <- Event{TurnEnd: true, ModelFailure: failure}
			eventually(t, "excluded failure report", func() bool { return len(got.lead()) == 1 })
			if queued := len(p.sent); queued != 0 {
				t.Fatalf("failure while %s waiting sent continue", wait)
			}
			if result := got.lead()[0].Result; result.Status != TurnDone || result.Message != failure {
				t.Fatalf("excluded %s failure result = %+v", wait, result)
			}
		})
	}
}

// (d): raw activity buys a fresh full interval; an event from the stopped
// timer is ignored and only the replacement timer can interrupt.
func TestAgyModelWaitWatchdogActivityResetsTimer(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, false)
	zero := 0
	p.events <- Event{ModelActivity: &zero}
	eventually(t, "replacement watchdog", func() bool {
		started, _ := timers.counts()
		return started == 2
	})
	timers.fire(t, 0)
	time.Sleep(60 * time.Millisecond)
	if got := procInterrupts(p); got != 0 {
		t.Fatalf("the replaced interval triggered %d interrupts", got)
	}
	timers.fire(t, 1)
	eventually(t, "replacement interval interrupt", func() bool { return procInterrupts(p) == 1 })
}

// (e): the reply-tail matcher joins split chunks, requires an anchored attempt
// line, and clears a candidate when later model output proves recovery.
func TestAgyModelWaitWatchdogMatchesAnchoredReplyTailAcrossChunks(t *testing.T) {
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	m.promptSent("ws-p-api-1")
	first := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"\n⚠ API er"}}}}`), rr)
	if slices.ContainsFunc(first, func(ev Event) bool { return ev.ModelFailureUpdate != nil }) {
		t.Fatalf("partial marker triggered a failure: %+v", first)
	}
	second := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ror (attempt 1): No capacity"}}}}`), rr)
	if !slices.ContainsFunc(second, func(ev Event) bool {
		return ev.ModelFailureUpdate != nil && strings.HasPrefix(*ev.ModelFailureUpdate, agyAPIErrorPrefix)
	}) {
		t.Fatalf("split API marker did not trigger from the accumulated tail: %+v", second)
	}
	ended := m.mapLine([]byte(`{"jsonrpc":"2.0","result":{"stopReason":"end_turn"},"id":"ws-p-api-1"}`), rr)
	if !slices.ContainsFunc(ended, func(ev Event) bool { return ev.TurnEnd && strings.HasPrefix(ev.ModelFailure, agyAPIErrorPrefix) }) {
		t.Fatalf("turn end must carry the visible API error: %+v", ended)
	}

	m.promptSent("ws-p-api-2")
	m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"\n⚠ API error (attempt 2): timeout"}}}}`), rr)
	recovered := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"\nOK."}}}}`), rr)
	if !slices.ContainsFunc(recovered, func(ev Event) bool { return ev.ModelFailureUpdate != nil && *ev.ModelFailureUpdate == "" }) {
		t.Fatalf("later reply text did not clear the terminal candidate: %+v", recovered)
	}
	if !slices.ContainsFunc(recovered, func(ev Event) bool { return strings.Contains(ev.Notice, "API error (attempt 2)") }) {
		t.Fatalf("recovered API failure was not retained as a diagnostic: %+v", recovered)
	}
	ended = m.mapLine([]byte(`{"jsonrpc":"2.0","result":{"stopReason":"end_turn"},"id":"ws-p-api-2"}`), rr)
	if slices.ContainsFunc(ended, func(ev Event) bool { return ev.TurnEnd && ev.ModelFailure != "" }) {
		t.Fatalf("a recovered turn retained the old API failure: %+v", ended)
	}
	if !slices.ContainsFunc(ended, func(ev Event) bool {
		return ev.Live != nil && ev.Live.Final && strings.TrimSpace(ev.Live.Text) == "OK." && !strings.Contains(ev.Live.Text, agyAPIErrorPrefix)
	}) {
		t.Fatalf("recovered final reply retained the failed attempt: %+v", ended)
	}

	if got := agyReplyTailFailure("prefix\n" + agyStreamInterrupted); got != agyStreamInterrupted {
		t.Fatalf("stream interruption tail = %q", got)
	}
	if got := agyReplyTailFailure("prefix\n" + agyStreamInterrupted + "\ncontinued"); got != "" {
		t.Fatalf("non-terminal stream interruption matched as %q", got)
	}
}

// The runner lets the failed prompt drain before cancelling it. A natural end
// during that window is retried without cancellation, and a successful retry
// replaces (rather than concatenates) the failed attempt's visible diagnostic.
func TestAgyModelWaitWatchdogDrainsAndStripsSuccessfulAPIErrorRetry(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, true)
	failure := "⚠ API error (attempt 1): No capacity"
	p.events <- Event{ModelFailureUpdate: &failure}
	eventually(t, "API failure drain timer", func() bool {
		started, _ := timers.counts()
		return started == 2
	})
	timers.mu.Lock()
	drainDuration := timers.durs[1]
	timers.mu.Unlock()
	if drainDuration != time.Second {
		t.Fatalf("failure drain = %v, want 1s", drainDuration)
	}
	if interrupts := procInterrupts(p); interrupts != 0 {
		t.Fatalf("failure was interrupted before its drain: %d", interrupts)
	}
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: failure, Final: true}}
	p.events <- Event{TurnEnd: true, ModelFailure: failure}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("API-error retry prompt = %q", sent)
	}
	if interrupts := procInterrupts(p); interrupts != 0 {
		t.Fatalf("naturally ended failure was interrupted %d times", interrupts)
	}
	p.reply("retry complete")
	eventually(t, "successful retry report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnDone || result.Message != "retry complete" || strings.Contains(result.Message, failure) {
		t.Fatalf("successful retry leaked failed-attempt text: %+v", result)
	}
}

// If the failed prompt does not end during the drain, cancellation lands
// first and continue is sent only after its TurnEnd makes steering safe.
func TestAgyModelWaitWatchdogCancelsOnlyAfterFailureDrain(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, false)
	failure := "⚠ API error (attempt 1): timeout"
	p.events <- Event{ModelFailureUpdate: &failure}
	fireWatchdog(t, timers, 1)
	eventually(t, "failure drain interrupt", func() bool { return procInterrupts(p) == 1 })
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("continue steered the still-running failed prompt: %d queued sends", queued)
	}
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted, ModelFailure: failure}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("post-cancel retry prompt = %q", sent)
	}
}

func TestAgyModelWaitWatchdogLaterReplyCancelsFailureDrain(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, true)
	failure := "⚠ API error (attempt 1): temporary"
	p.events <- Event{ModelFailureUpdate: &failure}
	eventually(t, "failure drain timer", func() bool {
		started, _ := timers.counts()
		return started == 2
	})
	cleared := ""
	p.events <- Event{ModelFailureUpdate: &cleared}
	eventually(t, "ordinary timer restored", func() bool {
		started, _ := timers.counts()
		return started == 3
	})
	timers.fire(t, 1) // a stopped drain may still deliver; it is no longer selected
	time.Sleep(60 * time.Millisecond)
	if interrupts := procInterrupts(p); interrupts != 0 {
		t.Fatalf("recovered failure triggered %d interrupts", interrupts)
	}
	p.reply("OK.")
	eventually(t, "recovered turn report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnDone || result.Message != "OK." {
		t.Fatalf("recovered turn result = %+v", result)
	}
}

// An API failure and silence spend one shared retry budget: after the API
// retry, a silent interval fails instead of sending another continue.
func TestAgyModelWaitWatchdogAPIErrorUsesSharedRetry(t *testing.T) {

	h, timers, got := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, true)
	failure := "⚠ API error (attempt 1): No capacity"
	p.events <- Event{ModelFailureUpdate: &failure}
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: failure, Final: true}}
	p.events <- Event{TurnEnd: true, ModelFailure: failure}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("API-error retry prompt = %q", sent)
	}
	fireWatchdog(t, timers, 2)
	eventually(t, "post-API-error interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "post-API-error failure report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnFailed || result.Error != "agy model stream stalled for 45s after API error retry" || result.Message != failure {
		t.Fatalf("shared retry result = %+v", result)
	}
}

// The inverse trigger order spends the same budget: after a silence retry, a
// terminal API failure is drained and failed, never retried a second time.
func TestAgyModelWaitWatchdogSilenceThenAPIErrorDoesNotRetryAgain(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, true)
	fireWatchdog(t, timers, 0)
	eventually(t, "silence interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("silence retry prompt = %q", sent)
	}
	failure := "⚠ API error (attempt 2): TLS timeout"
	p.events <- Event{ModelFailureUpdate: &failure}
	fireWatchdog(t, timers, 2)
	eventually(t, "second-trigger interrupt", func() bool { return procInterrupts(p) == 2 })
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: failure, Final: true}}
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted, ModelFailure: failure}
	eventually(t, "shared-budget failure report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnFailed || !strings.Contains(result.Error, "after retry") || !strings.Contains(result.Message, failure) {
		t.Fatalf("second-trigger result = %+v", result)
	}
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("second trigger sent another retry: %d queued sends", queued)
	}
}

// (f): native /compact has its own watchdog and never arms model waiting.
func TestAgyModelWaitWatchdogDoesNotArmForCompact(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	id, _ := startAgyWatchdogTurn(t, h, false)
	c := h.runner.conv(id)
	c.do(func() {
		c.disarmModelWait()
		c.compactCommand = true
		c.startModelWait()
		if c.modelWaitArmed || c.modelWaitC != nil {
			t.Fatalf("compact command armed model waiting")
		}
	})
	if started, _ := timers.counts(); started != 1 {
		t.Fatalf("compact guard started another timer: %d", started)
	}
}

// (g): zero is the explicit off switch.
func TestAgyModelWaitWatchdogZeroDisables(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "0")
	id, _ := startAgyWatchdogTurn(t, h, false)
	if started, _ := timers.counts(); started != 0 {
		t.Fatalf("disabled watchdog started %d timers", started)
	}
	c := h.runner.conv(id)
	c.do(func() {
		if c.modelWaitTimeout != 0 || c.modelWaitArmed {
			t.Fatalf("disabled watchdog state: timeout=%v armed=%v", c.modelWaitTimeout, c.modelWaitArmed)
		}
	})
}
