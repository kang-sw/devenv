package wsagent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type compactionRootKeys struct {
	KeyStore
	root string
}

func (k compactionRootKeys) Root(string) (string, bool) { return k.root, true }

func TestCompactionUsesTheKeysConfigRoot(t *testing.T) {
	root := t.TempDir()
	cfg := wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir(), RepoRoot: root}
	resolver := wsconfig.NewResolver(cfg, nil, nil, nil)
	if err := resolver.Set("compaction.worker_hard.pi", "65%", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeRepo}); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(Options{Config: cfg, Keys: compactionRootKeys{root: root}, Backends: map[string]BackendRegistration{BackendPi: {NativeCompaction: PiCompaction()}}})
	c := &conv{r: r, rec: Record{Root: t.TempDir(), WSKey: "key", Backend: BackendPi}}
	c.loadCompaction()
	if c.compactHard.Percent != 65 {
		t.Fatalf("nudge ignored the key's repo config: %+v", c.compactHard)
	}
}

func TestCompactionIsNotIdleTimeoutEligible(t *testing.T) {
	r := NewRunner(Options{Backends: map[string]BackendRegistration{BackendPi: {NativeCompaction: PiCompaction()}}})
	c := &conv{r: r, rec: Record{Backend: BackendPi}, idleAfter: time.Hour}
	c.idle = time.NewTimer(time.Hour)
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart})
	if c.idle != nil || c.idleNow() {
		t.Fatal("native compaction retained its prior idle timeout")
	}
	c.armIdle()
	defer c.stopIdle()
	if c.idleNow() {
		t.Fatal("a rearmed timer may stop compaction")
	}
	c.compactCommand = true
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd})
	if c.idleNow() {
		t.Fatal("command response is still pending")
	}
}

func TestCompactionAdvisoryOptOutKeepsHardThreshold(t *testing.T) {
	cfg := wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
	resolver := wsconfig.NewResolver(cfg, nil, nil, nil)
	if err := resolver.Set("claude.compaction_advisory_enabled", "false", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(Options{Config: cfg, Backends: map[string]BackendRegistration{BackendClaude: {Catalog: ClaudeCatalog()}}})
	c := &conv{r: r, rec: Record{Root: t.TempDir(), WSKey: "key", Backend: BackendClaude, Lead: true}}
	c.loadCompaction()
	if c.compactAdvisoryEnabled || c.compactHard.Percent != 80 {
		t.Fatalf("opt-out affected hard compaction: enabled=%v hard=%+v", c.compactAdvisoryEnabled, c.compactHard)
	}
}

func TestCompactionDescriptors(t *testing.T) {
	for _, tc := range []struct {
		h                     string
		p                     CompactionProtocol
		lead, worker, signals bool
	}{{BackendClaude, ClaudeCatalog().Compaction, true, false, true}, {BackendPi, *PiCompaction(), true, true, true}, {BackendCodex, CodexCatalog().Compaction, true, true, false}, {BackendAgy, AgyCatalog().Compaction, true, true, false}} {
		t.Run(tc.h, func(t *testing.T) {
			if tc.p.LeadCommand != tc.lead || tc.p.WorkerCommand != tc.worker || tc.p.Lifecycle != tc.signals {
				t.Fatal(tc.p)
			}
			if tc.signals && !tc.p.PromptPersists {
				t.Fatal("verified appended prompt must survive")
			}
			if tc.p.LegacyKey("compaction.worker_advisory") != "" || tc.p.LegacyKey("compaction.worker_hard") != "" {
				t.Fatal("lead legacy mapped to worker")
			}
		})
	}
}

func TestCompactionMeasuredMissingSignals(t *testing.T) {
	for _, attempt := range []int{1, 2} {
		msgs := readCodexFixture(t, fmt.Sprintf("agy-1.5.12-compaction-probe-%d.jsonl", attempt))
		prompts := acpPrompts(msgs)
		if len(prompts) != 4 {
			t.Fatal(len(prompts))
		}
		compact := prompts[1]
		end := acpResponseAt(t, msgs, compact)
		for _, m := range msgs[compact:end] {
			if strings.Contains(acpUpdate(m), "compact") {
				t.Fatalf("a new lifecycle signal requires descriptor review: %+v", m)
			}
		}
		if !strings.Contains(acpText(msgs, prompts[2], acpResponseAt(t, msgs, prompts[2])), "COMPACT-PROMPT-715") {
			t.Fatal("appended prompt did not survive")
		}
		if r := responseTo(t, msgs, "6"); r.Error == nil || r.Error.Code != -32602 {
			t.Fatal(r.Error)
		}
		codex := readCodexFixture(t, fmt.Sprintf("codex-0.170.0-compaction-probe-%d.jsonl", attempt))
		if r := responseTo(t, codex, "3"); r.Error == nil || !strings.Contains(r.Error.Message, "invalid transport") {
			t.Fatal(r.Error)
		}
	}
}

func TestCompactionLiveNativeFixtures(t *testing.T) {
	for _, lead := range []bool{false, true} {
		for _, name := range []string{"claude-compaction-probe.jsonl", "pi-compaction-probe.jsonl", "pi-compaction-failure-probe.jsonl"} {
			t.Run(fmt.Sprintf("%s/lead=%v", name, lead), func(t *testing.T) {
				spec := StartSpec{}
				if lead {
					spec.Lead = &LeadSpec{}
				}
				pi, cl := newPiMapper(spec), newClaudeMapper(spec)
				r := &recordedReplies{}
				counts := map[string]int{}
				for _, line := range readLeadFixture(t, name) {
					if line.in {
						continue
					}
					var evs []Event
					if strings.HasPrefix(name, "pi-") {
						evs = pi.mapLine(line.raw, r)
					} else {
						evs = cl.mapLine(line.raw, r)
					}
					for _, ev := range evs {
						if ev.Live != nil && ev.Live.Kind == wslive.KindCompact {
							counts[ev.Live.Phase]++
						}
					}
				}
				end := wslive.CompactEnd
				if strings.Contains(name, "failure") {
					end = wslive.CompactFailed
				}
				if counts[wslive.CompactStart] != 1 || counts[end] != 1 {
					t.Fatal(counts)
				}
			})
		}
	}
}

func compactionHarness(t *testing.T) *leadHarness {
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.Catalog = ClaudeCatalog()
	h.opts.Backends[BackendClaude] = reg
	return h.start()
}
func compactBoundary(h *leadHarness, id, holder string) {
	h.holders.out(holder, `{"type":"system","subtype":"status","status":"compacting"}`, `{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual"}}`, resultLine(), stateLine(HarnessIdle))
	h.sync(id, holder)
}

func TestCompactionIdleRestoreOrderingAndResume(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(fmt.Sprint(resume), func(t *testing.T) {
			h := compactionHarness(t)
			id, holder := h.runLead("one")
			key := h.record(id).WSKey
			out := h.runner.Compact(key, "", &resume)
			if out.Status != "queued" {
				t.Fatal(out)
			}
			if got := h.userInputs(holder); len(got) != 1 {
				t.Fatalf("mid-turn compact: %q", got)
			}
			h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
			eventually(t, "native compact at idle", func() bool { got := h.userInputs(holder); return len(got) == 2 && got[1] == "/compact" })
			h.send(id, "after")
			compactBoundary(h, id, holder)
			eventually(t, "restoration before queued work", func() bool {
				got := h.userInputs(holder)
				return len(got) >= 3 && strings.Contains(got[2], "Context was compacted")
			})
			got := h.userInputs(holder)
			if !strings.Contains(got[2], key) || !strings.Contains(got[2], "workflow_state") {
				t.Fatal(got)
			}
			if resume && !strings.Contains(got[2], "Continue the work") {
				t.Fatal(got)
			}
			if !resume && !strings.Contains(got[2], "after") {
				t.Fatal(got)
			}
			h.runner.conv(id).do(func() {
				c := h.runner.conv(id)
				c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd})
				if c.compactNote != "" {
					t.Error("duplicate restoration")
				}
			})
		})
	}
}

func TestCompactionAutoDoesNotStartContinue(t *testing.T) {
	h := compactionHarness(t)
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	compactBoundary(h, id, holder)
	h.idle(id)
	if got := h.userInputs(holder); len(got) != 1 {
		t.Fatalf("native auto started a turn: %q", got)
	}
	h.send(id, "after")
	eventually(t, "ordinary input carries note", func() bool {
		got := h.userInputs(holder)
		return len(got) == 2 && strings.Contains(got[1], "Context was compacted") && strings.Contains(got[1], "after")
	})
}

func TestCompactionThresholdLatchAndStaleUsage(t *testing.T) {
	h := compactionHarness(t)
	id, holder := h.runLead("one")
	c := h.runner.conv(id)
	observe := func(tokens *int64) {
		c.do(func() {
			c.apply(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: tokens, Window: 100000}})
		})
	}
	high := int64(90000)
	observe(&high)
	if len(h.userInputs(holder)) != 1 {
		t.Fatal("hard compact during active turn")
	}
	observe(nil) // Never trigger from the preceding stale reading.
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	if len(h.userInputs(holder)) != 1 {
		t.Fatal("stale usage triggered compaction")
	}
	observe(&high)
	c.do(func() { c.kick() })
	eventually(t, "hard before advisory", func() bool { return len(h.userInputs(holder)) == 2 })
	if got := h.userInputs(holder)[1]; got != "/compact" {
		t.Fatal(got)
	}
	compactBoundary(h, id, holder)
	h.idle(id)
	c.do(func() {
		if c.compactAdvised || c.compactHardIssued || c.compactUsageFresh {
			t.Error("success did not reset crossings")
		}
	})
	observe(&high)
	c.do(func() {
		c.kick()
		if c.compactUsageFresh || c.compacting {
			t.Error("stale pre-compaction stats re-triggered hard compaction")
		}
		c.observeCompaction(wslive.Event{Kind: wslive.KindTurnStart})
	})
	low := int64(10000)
	observe(&low)
	c.do(func() {
		c.compactHardIssued, c.compactAdvised = true, true
		c.observeCompaction(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &low, Window: 100000}})
		if c.compactAdvised || c.compactHardIssued {
			t.Error("falling usage did not reset")
		}
	})
}

func TestCompactionFailureLatchesUntilExplicitRetry(t *testing.T) {
	h := compactionHarness(t)
	id, holder := h.runLead("one")
	c := h.runner.conv(id)
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.holders.failInputs("user", 1)
	resume := false
	if got := h.runner.Compact(h.record(id).WSKey, "", &resume); got.Status != "error" {
		t.Fatal(got)
	}
	c.do(func() {
		n := int64(90000)
		for i := 0; i < 5; i++ {
			c.observeCompaction(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &n, Window: 100000}})
			c.compactIdle()
		}
		if c.compacting || c.compactNote != "" {
			t.Error("failure restored or retried")
		}
	})
	if len(h.userInputs(holder)) != 1 {
		t.Fatal("failure was retried")
	}
	if got := h.runner.Compact(h.record(id).WSKey, "", &resume); got.Status != "queued" {
		t.Fatal(got)
	}
	eventually(t, "explicit retry delivered", func() bool { return len(h.userInputs(holder)) == 2 })
}

func TestCompactionRestoreRespectsPromptDescriptor(t *testing.T) {
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.Catalog = ClaudeCatalog()
	reg.Catalog.Compaction.PromptPersists = false
	h.opts.Backends[BackendClaude] = reg
	manual := &leadManualRender{text: "RESTORED MANUAL"}
	h.opts.LeadManual = manual.render
	h.start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	compactBoundary(h, id, holder)
	h.idle(id)
	h.runner.conv(id).do(func() {
		c := h.runner.conv(id)
		if !strings.Contains(c.compactNote, "RESTORED MANUAL") {
			t.Error(c.compactNote)
		}
		c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd})
		if strings.Count(c.compactNote, "RESTORED MANUAL") != 1 {
			t.Error("manual duplicated")
		}
	})
}

func TestCompactionPiWorkerTargetAndResponseOrdering(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	reg := h.runner.opts.Backends[BackendPi]
	reg.NativeCompaction = PiCompaction()
	h.runner.opts.Backends[BackendPi] = reg
	lead := h.lead(BackendClaude, LeadOptions{})
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(child.ID)
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(child.ID)
	key := h.record(child.ID).WSKey
	parent := h.record(child.ID).Parent
	if got := h.runner.Compact(key, h.record(lead.ID).WSKey, nil); got.Status != "error" {
		t.Fatal("child controlled ancestor", got)
	}
	if got := h.runner.Compact(h.record(lead.ID).WSKey, AgentAddress(child.ID), nil); got.Status != "queued" {
		t.Fatal(got)
	}
	eventually(t, "worker RPC compact", func() bool { return len(h.holders.inputs(holder, "compact")) == 1 })
	h.holders.out(holder, `{"type":"compaction_start","reason":"manual"}`, `{"type":"compaction_end","reason":"manual"}`)
	eventually(t, "worker completion awaits response", func() bool {
		ready := false
		h.runner.conv(child.ID).do(func() { c := h.runner.conv(child.ID); ready = c.compactNote != "" && c.compactCommand })
		return ready
	})
	if len(h.holders.inputs(holder, "prompt")) != 1 {
		t.Fatal("continue sent before command response")
	}
	h.holders.out(holder, `{"type":"response","command":"compact","success":true}`)
	eventually(t, "targeted compact response consumed", func() bool {
		ready := false
		h.runner.conv(child.ID).do(func() {
			c := h.runner.conv(child.ID)
			ready = !c.compactCommand && !c.compacting && c.compactNote != ""
		})
		return ready
	})
	h.idle(child.ID)
	if len(h.holders.inputs(holder, "prompt")) != 1 {
		t.Fatal("targeted compact defaulted resume to true")
	}
	if got := h.runner.Compact(key, "", nil); got.Status != "queued" {
		t.Fatal(got)
	}
	eventually(t, "second worker RPC compact", func() bool { return len(h.holders.inputs(holder, "compact")) == 2 })
	h.holders.out(holder, `{"type":"compaction_start","reason":"manual"}`, `{"type":"compaction_end","reason":"manual"}`, `{"type":"response","command":"compact","success":true}`)
	eventually(t, "one self continue", func() bool { return len(h.holders.inputs(holder, "prompt")) == 2 })
	got := h.holders.inputs(holder, "prompt")[1]
	if !strings.Contains(got, "Context was compacted") || !strings.Contains(got, "Continue the work") {
		t.Fatal(got)
	}
	rec := h.record(child.ID)
	if rec.WSKey != key || rec.Parent == nil || *rec.Parent != *parent {
		t.Fatal("identity or lineage changed")
	}
}

func TestCompactionPiFailureSignals(t *testing.T) {
	for _, lead := range []bool{false, true} {
		spec := StartSpec{}
		if lead {
			spec.Lead = &LeadSpec{}
		}
		m := newPiMapper(spec)
		r := &recordedReplies{}
		for _, line := range []string{`{"type":"compaction_end","reason":"manual","aborted":true}`, `{"type":"compaction_end","reason":"threshold","errorMessage":"failed"}`} {
			evs := m.mapLine([]byte(line), r)
			found := false
			for _, ev := range evs {
				if ev.Live != nil && ev.Live.Kind == wslive.KindCompact && ev.Live.Phase == wslive.CompactFailed {
					found = true
				}
			}
			if !found {
				t.Fatalf("role lead=%v: no failure for %s: %+v", lead, line, evs)
			}
		}
	}
}

// Observe only the memory state: token events add no file operations. The
// ordinary projection's persistence remains covered by transcript I/O tests.
func TestCompactionObservationMemoryOnly(t *testing.T) {
	c := &conv{compactAdvisory: wsconfig.Threshold{Percent: 50}, compactHard: wsconfig.Threshold{Percent: 80}}
	n := int64(60000)
	before := wsstate.FileIOCounters()
	start := time.Now()
	for i := 0; i < 100000; i++ {
		c.observeCompaction(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &n, Window: 100000}})
	}
	if !c.compactUsageFresh {
		t.Fatal("not observed")
	}
	for site, n := range wsstate.FileIOCounters() {
		if n != before[site] {
			t.Fatalf("observation touched files: %s +%d", site, n-before[site])
		}
	}
	t.Logf("100000 memory observations in %s; zero instrumented file operations", time.Since(start))
}

func TestCompactionNeverWedgesOnPiUnknownEndTurnOrTimeout(t *testing.T) {
	// 1a: live Pi compact response must end the command even when nativeKnown is false;
	// replay (silentReplies) keeps unknown-state probing intact.
	spec := piSpec(t, `{"backend":"pi"}`)
	spec.Holder = &holderChild{id: "existing"}
	m := newPiMapper(spec)
	line := []byte(`{"type":"response","command":"compact","success":true}`)
	if evs := m.mapLine(line, silentReplies{}); len(evs) != 0 {
		t.Fatalf("replay compact response settled unknown state: %+v", evs)
	}
	if evs := m.mapLine(line, &recordedReplies{}); len(evs) == 0 || !evs[len(evs)-1].TurnEnd {
		t.Fatalf("live compact response with !nativeKnown did not end turn: %+v", evs)
	}

	// 1b: endTurn with empty turnErr still clears compacting/compactCommand (including dashboard /compact).
	h := compactionHarness(t)
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.sendCommand(id, "compact", "")
	eventually(t, "dashboard compact sent", func() bool { got := h.userInputs(holder); return len(got) == 2 && got[1] == "/compact" })
	// Return idle turn end with no CompactEnd and no error.
	h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
	h.sync(id, holder)
	c := h.runner.conv(id)
	c.do(func() {
		if c.compacting || c.compactCommand {
			t.Fatalf("endTurn without error left compaction stuck: compacting=%v cmd=%v", c.compacting, c.compactCommand)
		}
	})
	h.send(id, "after-silent-compact")
	eventually(t, "queued work runs after silent compact end", func() bool {
		got := h.userInputs(holder)
		return len(got) == 3 && strings.Contains(got[2], "after-silent-compact")
	})
	h.holders.out(holder, turnLines("sess-1", "u2", "second")...)
	h.idle(id)

	// 1c: CommandError, standalone CompactEnd, and standalone CompactFailed re-arm idle and kick.
	for _, tc := range []struct {
		name string
		ev   Event
	}{
		{"CommandError", Event{CommandError: "compact refused"}},
		{"CompactEnd", Event{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd}}},
		{"CompactFailed", Event{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.do(func() {
				c.idleAfter = time.Hour
				c.stopIdle()
				c.compacting = true
				if tc.name == "CommandError" {
					c.compactCommand = true
				}
				c.handle(tc.ev)
				if c.compacting || c.compactCommand {
					t.Fatalf("%s left compaction active: compacting=%v cmd=%v", tc.name, c.compacting, c.compactCommand)
				}
				if c.idle == nil {
					t.Fatalf("%s did not re-arm idle timer", tc.name)
				}
				c.stopIdle()
				c.idleAfter = 0
			})
		})
	}

	// 1d: watchdog timeout clears stuck compaction, publishes CompactFailed, re-arms idle and kicks.
	timers := &leverTimers{}
	wh := newLeadHarness(t)
	reg := wh.opts.Backends[BackendClaude]
	reg.Catalog = ClaudeCatalog()
	wh.opts.Backends[BackendClaude] = reg
	wh.opts.Timer = timers.start
	wh.start()
	wid, wholder := wh.runLead("one")
	wh.holders.out(wholder, turnLines("sess-2", "u1", "first")...)
	wh.idle(wid)
	wc := wh.runner.conv(wid)
	wc.do(func() { wc.idleAfter = time.Hour })
	resume := false
	if got := wh.runner.Compact(wh.record(wid).WSKey, "", &resume); got.Status != "queued" {
		t.Fatal(got)
	}
	eventually(t, "watchdog armed on compact command", func() bool {
		started, _ := timers.counts()
		return started == 1 && timers.durs[0] == compactCommandTimeout
	})
	timers.fire(t, 0)
	eventually(t, "watchdog cleared compaction and re-armed idle", func() bool {
		ok := false
		wc.do(func() { ok = !wc.compacting && !wc.compactCommand && wc.idle != nil })
		return ok
	})
	wc.do(func() { wc.stopIdle(); wc.idleAfter = 0 })
	wh.send(wid, "after-watchdog")
	eventually(t, "queued work unblocked after watchdog", func() bool {
		got := wh.userInputs(wholder)
		return len(got) == 3 && strings.Contains(got[2], "after-watchdog")
	})
	foundFailed := false
	for _, entry := range wh.ring(wid) {
		if entry.Kind == wslive.RingCompact && entry.CompactFailed {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Fatalf("watchdog did not publish CompactFailed: %+v", wh.ring(wid))
	}
}

func TestCompactionNudgesAndMidTurnAutoDoNotStartContinueTurn(t *testing.T) {
	// 2a & 2b: advisory and hard-limit unsupported fallback queue note without starting a turn,
	// and a harness lacking Lifecycle is not told to call ws/compact.
	for _, tc := range []struct {
		name        string
		backend     string
		catalog     *HarnessProfile
		tokens      int64
		wantCompact bool
	}{
		{"claude-advisory", BackendClaude, ClaudeCatalog(), 60000, true},
		{"agy-advisory", BackendAgy, AgyCatalog(), 60000, false},
		{"agy-hard-fallback", BackendAgy, AgyCatalog(), 90000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRunner(Options{Backends: map[string]BackendRegistration{tc.backend: {Catalog: tc.catalog}}})
			c := &conv{
				r:                      r,
				rec:                    Record{Backend: tc.backend, Lead: true, WSKey: "k"},
				compactAdvisory:        wsconfig.Threshold{Percent: 50},
				compactHard:            wsconfig.Threshold{Percent: 80},
				compactAdvisoryEnabled: true,
			}
			n := tc.tokens
			c.observeCompaction(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &n, Window: 100000}})
			if c.compactIdle() {
				t.Fatalf("%s started a turn on nudge", tc.name)
			}
			if c.compactResume {
				t.Fatalf("%s set compactResume=true", tc.name)
			}
			if c.compactNote == "" {
				t.Fatalf("%s queued no note", tc.name)
			}
			if got := strings.Contains(c.compactNote, "ws/compact"); got != tc.wantCompact {
				t.Fatalf("%s ws/compact mention = %v, want %v; note=%q", tc.name, got, tc.wantCompact, c.compactNote)
			}
			if !strings.Contains(c.compactInput("real turn"), strings.TrimSpace(c.compactNote)) {
				t.Fatalf("%s note did not ride on next real turn", tc.name)
			}
		})
	}

	// 2c: mid-turn native auto-compaction must not consume compactPending.resume.
	r := NewRunner(Options{Backends: map[string]BackendRegistration{BackendClaude: {Catalog: ClaudeCatalog()}}})
	c := &conv{r: r, rec: Record{Backend: BackendClaude, Lead: true, WSKey: "k"}, turn: true}
	c.compactPending = &compactRequest{resume: true}
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart, Trigger: wslive.TriggerAuto})
	if c.compactResume || c.compactPending == nil || !c.compactPending.resume {
		t.Fatalf("mid-turn auto-compaction consumed compactPending.resume: resume=%v pending=%+v", c.compactResume, c.compactPending)
	}
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: wslive.TriggerAuto})
	if c.compactResume {
		t.Fatal("mid-turn auto-compaction armed continue turn")
	}
}

func TestCompactionWorkerSelfResumeDefersParentSettle(t *testing.T) {
	h, got := newFakePiRecoveryHarness(t)
	reg := h.runner.opts.Backends[BackendPi]
	reg.NativeCompaction = PiCompaction()
	h.runner.opts.Backends[BackendPi] = reg
	lead := h.lead(BackendClaude, LeadOptions{})
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(child.ID)
	h.holders.out(holder, `{"type":"agent_start"}`)
	eventually(t, "worker running", func() bool { return h.state(child.ID).Status == StatusRunning })
	key := h.record(child.ID).WSKey
	if res := h.runner.Compact(key, "", nil); res.Status != "queued" {
		t.Fatal(res)
	}
	// Initial turn finishes: parent must NOT receive TurnDone yet; compact RPC runs first.
	h.holders.out(holder,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"pre-compact"}]}}`,
		`{"type":"agent_settled"}`,
	)
	eventually(t, "worker compact queued after turn", func() bool { return len(h.holders.inputs(holder, "compact")) == 1 })
	if reps := got.list(); len(reps) != 0 {
		t.Fatalf("parent settled before compaction resumed: %+v", reps)
	}
	h.holders.out(holder,
		`{"type":"compaction_start","reason":"manual"}`,
		`{"type":"compaction_end","reason":"manual"}`,
		`{"type":"response","command":"compact","success":true}`,
	)
	eventually(t, "resumed continue turn sent", func() bool { return len(h.holders.inputs(holder, "prompt")) == 2 })
	if reps := got.list(); len(reps) != 0 {
		t.Fatalf("parent settled before resumed turn ended: %+v", reps)
	}
	h.holders.out(holder,
		`{"type":"agent_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"resumed final"}]}}`,
		`{"type":"agent_settled"}`,
	)
	eventually(t, "parent receives one report after resumed turn", func() bool { return len(got.list()) == 1 })
	reps := got.list()
	if len(reps) != 1 || reps[0].Result.Status != TurnDone || reps[0].Result.Message != "resumed final" {
		t.Fatalf("unexpected parent reports: %+v", reps)
	}
}

func TestCompactionStatusRespectsKickGuards(t *testing.T) {
	for _, guard := range []string{"leverC", "hidden", "closed", "steered"} {
		t.Run(guard, func(t *testing.T) {
			h := compactionHarness(t)
			id, holder := h.runLead("one")
			h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
			h.idle(id)
			c := h.runner.conv(id)
			high := int64(90000)
			c.do(func() {
				switch guard {
				case "leverC":
					c.leverC = make(chan time.Time)
				case "hidden":
					c.hidden = true
				case "closed":
					c.closed = true
				case "steered":
					c.steered = []steeredReport{{Text: "pending"}}
				}
				c.handle(Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &high, Window: 100000}}})
				if c.compacting || c.compactCommand {
					t.Fatalf("KindStatus bypassed %s guard and triggered compaction", guard)
				}
				c.leverC, c.hidden, c.closed, c.steered = nil, false, false, nil
			})
		})
	}
}

func TestCompactionWorkerRestoreUsesSystemPromptNotLeadManual(t *testing.T) {
	p := *PiCompaction()
	p.PromptPersists = false
	manual := &leadManualRender{text: "LEAD MANUAL MUST NOT APPEAR"}
	r := NewRunner(Options{
		LeadManual: manual.render,
		Backends:   map[string]BackendRegistration{BackendPi: {NativeCompaction: &p}},
	})
	c := &conv{
		r:            r,
		rec:          Record{Backend: BackendPi, Lead: false, WSKey: "worker-key"},
		systemPrompt: "WORKER SYSTEM PROMPT",
		compacting:   true,
	}
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd})
	if !strings.Contains(c.compactNote, "WORKER SYSTEM PROMPT") {
		t.Fatalf("worker restore missing systemPrompt: %q", c.compactNote)
	}
	if strings.Contains(c.compactNote, "LEAD MANUAL MUST NOT APPEAR") {
		t.Fatalf("worker restore leaked leadManual: %q", c.compactNote)
	}
}
