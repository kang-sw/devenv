package wsagent

import (
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestAgyActivityCallbackOnSettledHostedChildTurn(t *testing.T) {
	previous := suiteBackend
	suiteBackend = BackendAgy
	t.Cleanup(func() { suiteBackend = previous })

	var calls atomic.Int64
	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	h.config.AgyActivity = func() { calls.Add(1) }
	h.start()
	id := h.spawn("work", leadParent).ID
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	p.reply("done")
	eventually(t, "agy activity callback", func() bool { return calls.Load() == 1 })

	c := h.runner.conv(id)
	c.do(func() {
		c.handle(Event{TurnStart: true, Replay: true})
		c.handle(Event{TurnEnd: true, Replay: true})
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("replayed turn end raised %d activity callbacks, want 1 total", got)
	}
}

func TestAgyActivityCallbackSkipsWatchdogContinuedTurnEnd(t *testing.T) {
	var calls atomic.Int64
	h, timers, _ := agyWatchdogHarnessWithRetries(t, "45", "1")
	h.runner.Close()
	h.config.AgyActivity = func() { calls.Add(1) }
	h.start()
	_, p := startAgyWatchdogTurn(t, h, false)

	fireWatchdog(t, timers, 0)
	eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("retry prompt = %q, want continue", sent)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("watchdog-continued turn end raised %d activity callbacks", got)
	}
	p.reply("done")
	eventually(t, "settled agy activity callback", func() bool { return calls.Load() == 1 })
}

func TestAgyActivityCallbackSkipsCompactCommandTurn(t *testing.T) {
	var calls atomic.Int64
	h, _, _ := agyWatchdogHarness(t, "45")
	h.runner.Close()
	h.config.AgyActivity = func() { calls.Add(1) }
	h.start()
	id, p := startAgyWatchdogTurn(t, h, false)
	p.reply("done")
	eventually(t, "ordinary agy activity callback", func() bool { return calls.Load() == 1 })
	c := h.runner.conv(id)
	c.do(func() {})
	calls.Store(0)

	key := h.record(id).WSKey
	resume := false
	if result := h.runner.Compact(key, "", &resume); result.Status != "queued" {
		t.Fatalf("compact result = %+v", result)
	}
	if sent := p.nextSent(t); sent != "/compact" {
		t.Fatalf("compact command = %q, want /compact", sent)
	}
	p.reply("compacted")
	c.do(func() {})
	if got := calls.Load(); got != 0 {
		t.Fatalf("compact command turn raised %d activity callbacks", got)
	}
}

func TestAgyActivityCallbackSkipsNonAgyTurn(t *testing.T) {
	var calls atomic.Int64
	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	h.config.AgyActivity = func() { calls.Add(1) }
	h.start()
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "work"); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	p.reply("done")
	eventually(t, "non-agy idle", func() bool { return h.state(id).Status == StatusIdle })
	c := h.runner.conv(id)
	c.do(func() {})
	if got := calls.Load(); got != 0 {
		t.Fatalf("non-agy turn raised %d activity callbacks", got)
	}
}

func agyWatchdogHarness(t *testing.T, timeoutSeconds string) (*harness, *leverTimers, *reports) {
	return agyWatchdogHarnessWithRetries(t, timeoutSeconds, "1")
}

func agyWatchdogHarnessWithRetries(t *testing.T, timeoutSeconds, retries string) (*harness, *leverTimers, *reports) {
	return agyWatchdogHarnessWithSilence(t, timeoutSeconds, retries, true)
}

func agyWatchdogHarnessWithSilence(t *testing.T, timeoutSeconds, retries string, silence bool) (*harness, *leverTimers, *reports) {
	t.Helper()
	previousSilence := agySilenceWatchdogEnabled
	agySilenceWatchdogEnabled = silence
	t.Cleanup(func() { agySilenceWatchdogEnabled = previousSilence })
	previous := suiteBackend
	suiteBackend = BackendAgy
	t.Cleanup(func() { suiteBackend = previous })

	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	reg := h.config.Backends[BackendAgy]
	reg.Catalog = AgyCatalog()
	h.config.Backends[BackendAgy] = reg
	writeStructured(t, filepath.Join(h.root, ".ws-workflow", "config.json"), map[string]string{
		ProfilePrefix + "chat": strings.Replace(chatProfile, `"backend":"claude"`, `"backend":"agy"`, 1),
	})
	resolver := wsconfig.NewResolver(h.config.Config, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemAgyModelWaitTimeoutSeconds, timeoutSeconds, wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if retries != "" {
		if err := resolver.Set(wsconfig.ItemAgyModelWaitRetries, retries, wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
			t.Fatal(err)
		}
	}
	timers := &leverTimers{}
	got := &reports{}
	h.config.Timer = timers.start
	h.config.AgyModelWaitBackoff = func(int) time.Duration { return 0 }
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

func exhaustSingleAgyWatchdogRetry(t *testing.T, timers *leverTimers, p *fakeProc) {
	t.Helper()
	fireWatchdog(t, timers, 0)
	eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("retry prompt = %q, want continue", sent)
	}
	fireWatchdog(t, timers, 2)
	eventually(t, "exhaustion interrupt", func() bool { return procInterrupts(p) == 2 })
}

func agyWatchdogDiagnosticCount(t *testing.T, h *harness, id, diagnostic string) int {
	t.Helper()
	c := h.runner.conv(id)
	c.do(func() {})
	records := readEventually(t, "watchdog diagnostic in the ring", func() ([]wslive.RingRecord, error) {
		return wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(h.dir, id)))
	})
	count := 0
	for _, record := range records {
		if record.Kind == wslive.RingReply && record.Text == diagnostic {
			count++
		}
	}
	return count
}

func TestAgyModelWaitWatchdogSilenceOffKeepsInBandRetry(t *testing.T) {
	if agySilenceWatchdogEnabled {
		t.Fatal("silence watchdog must be off by default")
	}
	h, timers, _ := agyWatchdogHarnessWithSilence(t, "1", "1", false)
	id, p := startAgyWatchdogTurn(t, h, false)
	c := h.runner.conv(id)
	c.do(func() {
		if !c.modelWaitArmed || c.modelWaitTimeout != time.Second {
			t.Fatalf("in-band retry must stay armed: armed=%v timeout=%v", c.modelWaitArmed, c.modelWaitTimeout)
		}
	})
	time.Sleep(1100 * time.Millisecond)
	if started, _ := timers.counts(); started != 0 {
		t.Fatalf("silent turn started %d watchdog timers", started)
	}
	if interrupts := procInterrupts(p); interrupts != 0 || len(p.sent) != 0 {
		t.Fatalf("silent turn interrupted %d times and queued %d prompts", interrupts, len(p.sent))
	}

	failure := "⚠ API error (attempt 1): request failed: stream idle timeout (10m0s): context deadline exceeded"
	if got := agyReplyTailFailure("reply\n" + failure); got != failure {
		t.Fatalf("in-band failure tail = %q, want %q", got, failure)
	}
	p.events <- Event{ModelFailureUpdate: &failure}
	fireWatchdog(t, timers, 0) // in-band failure drain, not a silence timer
	eventually(t, "in-band failure interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted, ModelFailure: failure}
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("in-band retry prompt = %q, want continue", sent)
	}
}

// The built-in retry budget is spent inside one logical turn. Each silent
// interval is cancelled before continue is sent, and the turn fails visibly
// after exactly the configured number of attempts.
func TestAgyModelWaitWatchdogRetriesConfiguredBudgetAndReportsExhaustion(t *testing.T) {
	h, timers, got := agyWatchdogHarnessWithRetries(t, "45", "")
	id, p := startAgyWatchdogTurn(t, h, true)

	for attempt := 1; attempt <= DefaultAgyModelWaitRetries; attempt++ {
		fireWatchdog(t, timers, (attempt-1)*2)
		eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == attempt })
		if st := h.state(id); st.Status != StatusRunning {
			t.Fatalf("attempt %d closed the logical turn while cancellation landed: %+v", attempt, st)
		}
		p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
		if sent := p.nextSent(t); sent != "continue" {
			t.Fatalf("attempt %d prompt = %q, want continue", attempt, sent)
		}
		if st := h.state(id); st.Status != StatusRunning || len(got.lead()) != 0 {
			t.Fatalf("attempt %d left the logical turn: state=%+v reports=%+v", attempt, st, got.lead())
		}
	}

	fireWatchdog(t, timers, DefaultAgyModelWaitRetries*2)
	eventually(t, "exhaustion interrupt", func() bool { return procInterrupts(p) == DefaultAgyModelWaitRetries+1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "failed parent report", func() bool { return len(got.lead()) == 1 })
	report := got.lead()[0]
	if report.Result.Status != TurnFailed || report.Result.Error != "agy model stream stalled for 45s after 3 automatic continue attempts" {
		t.Fatalf("exhausted report = %+v", report.Result)
	}
	if want := "ws: agy stalled; sent 3 automatic continue attempts, all failed. Failure classes: no-event silence."; !strings.Contains(report.Result.Message, want) {
		t.Fatalf("exhausted report message = %q, want %q", report.Result.Message, want)
	}
	if rec := h.record(id); rec.TurnOpen || rec.LastTurn == nil || rec.LastTurn.Result.Status != TurnFailed {
		t.Fatalf("the failed logical turn must be settled in the record: %+v", rec)
	}
}

func TestAgyModelWaitWatchdogZeroRetriesDisablesAutomaticContinue(t *testing.T) {
	h, timers, got := agyWatchdogHarnessWithRetries(t, "45", "0")
	_, p := startAgyWatchdogTurn(t, h, true)

	fireWatchdog(t, timers, 0)
	eventually(t, "zero-budget interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "zero-budget parent report", func() bool { return len(got.lead()) == 1 })
	result := got.lead()[0].Result
	if result.Status != TurnFailed || result.Error != "agy model stream stalled for 45s after 0 automatic continue attempts" {
		t.Fatalf("zero-budget result = %+v", result)
	}
	if want := "automatic continue retries are disabled (0 attempts configured)"; !strings.Contains(result.Message, want) {
		t.Fatalf("zero-budget message = %q, want %q", result.Message, want)
	}
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("zero retry budget sent %d continue prompts", queued)
	}
}

func TestAgyModelWaitWatchdogPublishesExhaustionToHostedLeadConversation(t *testing.T) {
	previousSilence := agySilenceWatchdogEnabled
	agySilenceWatchdogEnabled = true
	t.Cleanup(func() { agySilenceWatchdogEnabled = previousSilence })
	h, f := newAgyLeadHarness(t)
	timers := &leverTimers{}
	timeout, retries := 45, 1
	h.opts.Timer = timers.start
	h.opts.AgyModelWaitBackoff = func(int) time.Duration { return 0 }
	h.opts.AgyModelWaitTimeoutSeconds = &timeout
	h.opts.AgyModelWaitRetries = &retries
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{}).ID
	f.replyID = id
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "hosted lead first prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })

	fireWatchdog(t, timers, 0)
	ids := agyPromptIDs(h, holder)
	h.holders.out(holder, `{"jsonrpc":"2.0","result":{"stopReason":"cancelled"},"id":"`+ids[0]+`"}`)
	eventually(t, "hosted lead continue prompt", func() bool { return len(agyPromptIDs(h, holder)) == 2 })
	fireWatchdog(t, timers, 2)
	ids = agyPromptIDs(h, holder)
	h.holders.out(holder, `{"jsonrpc":"2.0","result":{"stopReason":"cancelled"},"id":"`+ids[1]+`"}`)
	h.idle(id)

	const diagnostic = "ws: agy stalled; sent 1 automatic continue attempts, all failed. Failure classes: no-event silence."
	if replies := ringTexts(h.ring(id), wslive.RingReply); !slices.Contains(replies, diagnostic) {
		t.Fatalf("hosted lead replies = %q, want visible diagnostic %q", replies, diagnostic)
	}
	if st := h.state(id); !strings.Contains(st.Error, "after 1 automatic continue attempts") {
		t.Fatalf("hosted lead state = %+v", st)
	}
}

func TestAgyModelWaitWatchdogForceStopsWhenInterruptIsIgnored(t *testing.T) {
	// Exercise the shipped default-three budget: a cancellation that never
	// lands must still fail on its own grace timer before any retry can start.
	h, timers, got := agyWatchdogHarnessWithRetries(t, "45", "")
	id, p := startAgyWatchdogTurn(t, h, true)

	fireWatchdog(t, timers, 0)
	eventually(t, "first interrupt", func() bool { return procInterrupts(p) == 1 })
	eventually(t, "cancel grace timer", func() bool {
		started, _ := timers.counts()
		return started == 2
	})
	timers.mu.Lock()
	grace := timers.durs[1]
	timers.mu.Unlock()
	if grace != modelWaitCancelGrace {
		t.Fatalf("cancel grace = %v, want %v", grace, modelWaitCancelGrace)
	}

	// The fake deliberately ignores Interrupt and never emits TurnEnd.
	timers.fire(t, 1)
	eventually(t, "force-stopped process", p.isClosed)
	eventually(t, "failed parent report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnFailed || result.Error != "agy model stream did not stop within 5s after interrupt" {
		t.Fatalf("cancel-grace result = %+v", result)
	}

	// A holder may still deliver its EOF and an already-buffered cancelled
	// TurnEnd after stopProcess settled the child. Run those exact handlers in
	// order and prove the report fence makes both stale events idempotent.
	c := h.runner.conv(id)
	c.do(func() {
		c.processExited()
		c.handle(Event{TurnEnd: true, TurnReason: wslive.TurnAborted})
	})
	if reports := got.lead(); len(reports) != 1 {
		t.Fatalf("late EOF/TurnEnd produced %d parent reports: %+v", len(reports), reports)
	}
	if rec := h.record(id); rec.TurnOpen || rec.ReportID == "" || rec.ReportID != rec.LastReportedTurn {
		t.Fatalf("late EOF/TurnEnd unsettled or duplicated the child report: %+v", rec)
	}
}

func TestAgyModelWaitWatchdogExhaustionPublishesDiagnosticOnEOF(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	id, p := startAgyWatchdogTurn(t, h, true)
	exhaustSingleAgyWatchdogRetry(t, timers, p)

	// The exhausted prompt exits without TurnEnd after accepting the interrupt.
	p.exit()
	eventually(t, "failed parent report", func() bool { return len(got.lead()) == 1 })

	const diagnostic = "ws: agy stalled; sent 1 automatic continue attempts, all failed. Failure classes: no-event silence."
	report := got.lead()[0]
	if report.Result.Status != TurnFailed || report.Result.Error != "agy model stream stalled for 45s after 1 automatic continue attempts" {
		t.Fatalf("EOF exhaustion result = %+v", report.Result)
	}
	if count := strings.Count(report.Result.Message, diagnostic); count != 1 {
		t.Fatalf("EOF exhaustion message contains diagnostic %d times: %q", count, report.Result.Message)
	}

	// A buffered TurnEnd after EOF must not publish or report the turn again.
	c := h.runner.conv(id)
	c.do(func() { c.handle(Event{TurnEnd: true, TurnReason: wslive.TurnAborted}) })
	if reports := got.lead(); len(reports) != 1 {
		t.Fatalf("EOF plus late TurnEnd produced %d parent reports: %+v", len(reports), reports)
	}
	if count := agyWatchdogDiagnosticCount(t, h, id, diagnostic); count != 1 {
		t.Fatalf("EOF plus late TurnEnd published %d exhaustion diagnostics", count)
	}
}

func TestAgyModelWaitWatchdogExhaustionPublishesDiagnosticOnGraceForceStop(t *testing.T) {
	h, timers, got := agyWatchdogHarness(t, "45")
	id, p := startAgyWatchdogTurn(t, h, true)
	exhaustSingleAgyWatchdogRetry(t, timers, p)

	// The exhausted prompt ignores the interrupt, so the bounded grace force-
	// stops it without a TurnEnd from the backend.
	fireWatchdog(t, timers, 3)
	eventually(t, "force-stopped exhausted process", p.isClosed)
	eventually(t, "failed parent report", func() bool { return len(got.lead()) == 1 })

	const diagnostic = "ws: agy stalled; sent 1 automatic continue attempts, all failed. Failure classes: no-event silence."
	report := got.lead()[0]
	if report.Result.Status != TurnFailed || report.Result.Error != "agy model stream stalled for 45s after 1 automatic continue attempts and did not stop within 5s after interrupt" {
		t.Fatalf("grace exhaustion result = %+v", report.Result)
	}
	if count := strings.Count(report.Result.Message, diagnostic); count != 1 {
		t.Fatalf("grace exhaustion message contains diagnostic %d times: %q", count, report.Result.Message)
	}

	// EOF and a buffered TurnEnd may still arrive after stopProcess settled the
	// report; both must remain idempotent.
	c := h.runner.conv(id)
	c.do(func() {
		c.processExited()
		c.handle(Event{TurnEnd: true, TurnReason: wslive.TurnAborted})
	})
	if reports := got.lead(); len(reports) != 1 {
		t.Fatalf("grace force-stop plus late events produced %d parent reports: %+v", len(reports), reports)
	}
	if count := agyWatchdogDiagnosticCount(t, h, id, diagnostic); count != 1 {
		t.Fatalf("grace force-stop plus late events published %d exhaustion diagnostics", count)
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

func TestAgyModelWaitWatchdogDelaysAutomaticContinue(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	h.runner.opts.AgyModelWaitBackoff = func(attempt int) time.Duration {
		if attempt != 1 {
			t.Fatalf("backoff attempt = %d, want 1", attempt)
		}
		return 7 * time.Second
	}
	_, p := startAgyWatchdogTurn(t, h, false)

	fireWatchdog(t, timers, 0)
	eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "backoff timer", func() bool {
		started, _ := timers.counts()
		return started == 3
	})
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("continue sent before backoff: %d queued", queued)
	}
	timers.mu.Lock()
	delay := timers.durs[2]
	timers.mu.Unlock()
	if delay != 7*time.Second {
		t.Fatalf("backoff delay = %v, want 7s", delay)
	}
	timers.fire(t, 2)
	if sent := p.nextSent(t); sent != "continue" {
		t.Fatalf("post-backoff prompt = %q", sent)
	}
}

func TestAgyModelWaitWatchdogUserActionCancelsBackoff(t *testing.T) {
	for _, action := range []string{"send", "interrupt", "stop", "process exit"} {
		t.Run(action, func(t *testing.T) {
			h, timers, got := agyWatchdogHarness(t, "45")
			h.runner.opts.AgyModelWaitBackoff = func(int) time.Duration { return 7 * time.Second }
			id, p := startAgyWatchdogTurn(t, h, true)
			fireWatchdog(t, timers, 0)
			eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == 1 })
			p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
			eventually(t, "backoff timer", func() bool {
				started, _ := timers.counts()
				return started == 3
			})

			if action == "send" {
				if _, err := h.runner.Send(id, "new direction"); err != nil {
					t.Fatal(err)
				}
				if sent := p.nextSent(t); !strings.Contains(sent, "new direction") || strings.Contains(sent, "continue") {
					t.Fatalf("replacement prompt = %q", sent)
				}
			} else if action == "interrupt" {
				if _, _, err := h.runner.Interrupt(id); err != nil {
					t.Fatal(err)
				}
			} else if action == "stop" {
				if _, err := h.runner.Stop(id); err != nil {
					t.Fatal(err)
				}
			} else {
				p.exit()
			}
			eventually(t, "single settlement", func() bool { return len(got.lead()) == 1 })
			timers.fire(t, 2) // a stopped fake timer may still deliver
			time.Sleep(40 * time.Millisecond)
			if queued := len(p.sent); queued != 0 {
				t.Fatalf("%s allowed a delayed continue: %d queued", action, queued)
			}
			if reports := got.lead(); len(reports) != 1 {
				t.Fatalf("%s produced %d settlements: %+v", action, len(reports), reports)
			}
		})
	}
}

func TestAgyModelWaitWatchdogSendWinsReadyBackoffRace(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	h.runner.opts.AgyModelWaitBackoff = func(int) time.Duration { return 7 * time.Second }
	id, p := startAgyWatchdogTurn(t, h, false)
	fireWatchdog(t, timers, 0)
	eventually(t, "retry interrupt", func() bool { return procInterrupts(p) == 1 })
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted}
	eventually(t, "backoff timer", func() bool {
		started, _ := timers.counts()
		return started == 3
	})

	gate := newGate(t, func() {})
	go h.runner.conv(id).do(gate.wait)
	gate.awaitEntered(t)
	timers.fire(t, 2) // ready while the conversation loop is held
	if _, err := h.runner.Send(id, "new direction"); err != nil {
		t.Fatal(err)
	}
	gate.release()
	if sent := p.nextSent(t); !strings.Contains(sent, "new direction") || strings.Contains(sent, "continue") {
		t.Fatalf("race winner prompt = %q", sent)
	}
	time.Sleep(40 * time.Millisecond)
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("ready backoff sent %d late prompts", queued)
	}
}

// A hosted holder replaces private thought lines with the content-free pulse.
// Repeated pulses can span longer than the timeout without cancelling, while
// neither the pulse nor the private thought text becomes visible reply data.
func TestAgyHostedThoughtPulseKeepsModelWaitAlive(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	_, p := startAgyWatchdogTurn(t, h, false)
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}

	for interval := 0; interval < 3; interval++ {
		evs := m.mapLine([]byte(agyRingPulse), rr)
		activity := slices.DeleteFunc(evs, func(ev Event) bool { return ev.ModelActivity == nil })
		if len(activity) != 1 || *activity[0].ModelActivity != 0 {
			t.Fatalf("pulse %d activity = %+v", interval, evs)
		}
		p.events <- activity[0]
		eventually(t, "replacement watchdog", func() bool {
			started, _ := timers.counts()
			return started == interval+2
		})
		timers.fire(t, interval)
		time.Sleep(20 * time.Millisecond)
		if got := procInterrupts(p); got != 0 {
			t.Fatalf("pulse %d allowed %d watchdog interrupts", interval, got)
		}
	}
	if reply := m.reply.String(); reply != "" || strings.Contains(reply, "thought") {
		t.Fatalf("content-free pulse became visible reply %q", reply)
	}
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
	if result := got.lead()[0].Result; result.Status != TurnFailed || result.Error != "agy model stream stalled for 45s after 1 automatic continue attempts" || !strings.Contains(result.Message, failure) || !strings.Contains(result.Message, "Failure classes: no capacity, no-event silence") {
		t.Fatalf("shared retry result = %+v", result)
	}
}

func TestAgyModelFailureClassifiesKnownInBandErrors(t *testing.T) {
	for failure, want := range map[string]string{
		"⚠ API error (attempt 1): 503 UNAVAILABLE: No capacity": "no capacity (503/UNAVAILABLE)",
		"⚠ API error (attempt 1): unexpected EOF":               "unexpected EOF",
		"⚠ API error (attempt 1): stream idle timeout":          "stream idle timeout",
		"⚠ API error (attempt 1): TLS handshake timeout":        "TLS handshake timeout",
		"": "no-event silence",
	} {
		if got := agyModelFailureClass(failure); got != want {
			t.Errorf("classify %q = %q, want %q", failure, got, want)
		}
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
	fireWatchdog(t, timers, 3)
	eventually(t, "second-trigger interrupt", func() bool { return procInterrupts(p) == 2 })
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: failure, Final: true}}
	p.events <- Event{TurnEnd: true, TurnReason: wslive.TurnAborted, ModelFailure: failure}
	eventually(t, "shared-budget failure report", func() bool { return len(got.lead()) == 1 })
	if result := got.lead()[0].Result; result.Status != TurnFailed || !strings.Contains(result.Error, "after 1 automatic continue attempts") || !strings.Contains(result.Message, failure) || !strings.Contains(result.Message, "sent 1 automatic continue attempts") {
		t.Fatalf("second-trigger result = %+v", result)
	}
	if queued := len(p.sent); queued != 0 {
		t.Fatalf("second trigger sent another retry: %d queued sends", queued)
	}
}

// (f): native /compact has its own watchdog and never arms model waiting.
func TestAgyModelWaitWatchdogDoesNotArmForCompact(t *testing.T) {
	h, timers, _ := agyWatchdogHarness(t, "45")
	id, p := startAgyWatchdogTurn(t, h, false)
	p.reply("done")
	eventually(t, "ordinary turn ended", func() bool { return !h.record(id).TurnOpen && h.state(id).Status != StatusRunning })

	key := h.record(id).WSKey
	resume := false
	if result := h.runner.Compact(key, "", &resume); result.Status != "queued" {
		t.Fatalf("compact result = %+v", result)
	}
	if sent := p.nextSent(t); sent != "/compact" {
		t.Fatalf("compact command = %q, want /compact", sent)
	}
	c := h.runner.conv(id)
	c.do(func() {
		if c.modelWaitArmed || c.modelWaitC != nil {
			t.Fatalf("real compact command armed model waiting")
		}
	})
	if started, _ := timers.counts(); started != 2 {
		t.Fatalf("ordinary + compact watchdog timers = %d, want 2", started)
	}
	timers.mu.Lock()
	durations := slices.Clone(timers.durs)
	timers.mu.Unlock()
	if durations[0] != 45*time.Second || durations[1] != compactCommandTimeout {
		t.Fatalf("watchdog durations = %v, want [45s %v]", durations, compactCommandTimeout)
	}
}

// A zero timeout disables the watchdog independently of its retry budget.
func TestAgyModelWaitWatchdogTimeoutZeroDisables(t *testing.T) {
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
