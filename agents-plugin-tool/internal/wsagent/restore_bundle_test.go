package wsagent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

type restoreProbeCall struct {
	key      string
	cause    RestoreCause
	children []AgentChild
}

type restoreProbe struct {
	mu    sync.Mutex
	state string
	err   error
	calls []restoreProbeCall
}

func (p *restoreProbe) render(_ string, _ string, key string, cause RestoreCause, children []AgentChild) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, restoreProbeCall{key: key, cause: cause, children: append([]AgentChild(nil), children...)})
	if p.err != nil {
		return "", p.err
	}
	return fmt.Sprintf("restore[%s] key=%s cause=%s", p.state, key, cause), nil
}

func (p *restoreProbe) setState(state string) {
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
}

func (p *restoreProbe) setError(err error) {
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
}

func (p *restoreProbe) snapshot() []restoreProbeCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]restoreProbeCall(nil), p.calls...)
}

func TestRestartRestoreIdleLeadAndChildAfterReplay(t *testing.T) {
	probe := &restoreProbe{state: "stale"}
	h := newLeadHarness(t)
	h.opts.RestoreBundle = probe.render
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	for tier, model := range map[string]string{"small": "prov/small", "medium": "prov/medium"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, tier, "", model, PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	h.start()

	leadID, leadHolder := h.runLead("lead work")
	h.holders.out(leadHolder, turnLines("sess-1", "lead-1", "lead done")...)
	h.idle(leadID)
	worker, err := h.runner.Spawn(SpawnSpec{
		Profile: PiWorkerProfile, Root: h.root, Prompt: "worker work", Tier: "medium",
		Parent: &ParentRef{Kind: ParentLead, ReplyID: leadID},
	})
	if err != nil {
		t.Fatal(err)
	}
	workerHolder := h.holder(worker.ID)
	h.holders.out(workerHolder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(worker.ID)
	if got := len(h.holders.inputs(workerHolder, "prompt")); got != 1 {
		t.Fatalf("worker prompts before restart = %d, want 1", got)
	}

	h.runner.Close()
	h.holders.restart()
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "restart holder scan", entered)
	probe.setState("fresh-after-launch")
	if calls := probe.snapshot(); len(calls) != 0 {
		t.Fatalf("restore rendered before holder replay: %+v", calls)
	}
	if got := len(h.userInputs(leadHolder)); got != 1 {
		t.Fatalf("lead received restore before holder replay: %d inputs", got)
	}
	if got := len(h.holders.inputs(workerHolder, "prompt")); got != 1 {
		t.Fatalf("worker received restore before holder replay: %d prompts", got)
	}

	release()
	eventually(t, "one restart restore for lead and child", func() bool {
		return len(probe.snapshot()) == 2 && len(h.userInputs(leadHolder)) == 2 && len(h.holders.inputs(workerHolder, "prompt")) == 2
	})
	leadRestore := h.userInputs(leadHolder)[1]
	workerRestore := h.holders.inputs(workerHolder, "prompt")[1]
	for role, text := range map[string]string{"lead": leadRestore, "worker": workerRestore} {
		if !strings.Contains(text, "restore[fresh-after-launch]") || !strings.Contains(text, "cause=restart") {
			t.Fatalf("%s restart restore = %q", role, text)
		}
	}
	calls := probe.snapshot()
	byKey := map[string]restoreProbeCall{}
	for _, call := range calls {
		byKey[call.key] = call
	}
	leadKey, workerKey := h.record(leadID).WSKey, h.record(worker.ID).WSKey
	if len(byKey) != 2 || byKey[leadKey].cause != RestoreRestart || byKey[workerKey].cause != RestoreRestart {
		t.Fatalf("restart calls = %+v", calls)
	}
	if children := byKey[leadKey].children; len(children) != 1 || children[0].Address != AgentAddress(worker.ID) {
		t.Fatalf("lead direct children = %+v", children)
	}
	if children := byKey[workerKey].children; len(children) != 0 {
		t.Fatalf("worker direct children = %+v, want none", children)
	}
	time.Sleep(30 * time.Millisecond)
	if calls := probe.snapshot(); len(calls) != 2 {
		t.Fatalf("restart restore repeated: %+v", calls)
	}
}

func TestRestartRestoreActiveRejoinWaitsForIdle(t *testing.T) {
	probe := &restoreProbe{state: "active"}
	h := newLeadHarness(t)
	h.opts.RestoreBundle = probe.render
	h.start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), replyLine("u1", "still running"))
	eventually(t, "active lead", func() bool { return h.state(id).Status == StatusRunning })

	h.runner.Close()
	h.holders.restart()
	h.start()
	eventually(t, "active lead rejoined", func() bool {
		c := h.runner.conv(id)
		if c == nil {
			return false
		}
		ready := false
		c.do(func() { ready = !c.replaying && c.turn && strings.HasPrefix(c.compactNote, restartRestoreMarker) })
		return ready
	})
	if calls := probe.snapshot(); len(calls) != 0 {
		t.Fatalf("active rejoin rendered before idle: %+v", calls)
	}
	if got := len(h.userInputs(holder)); got != 1 {
		t.Fatalf("active rejoin started restore turn: %d inputs", got)
	}

	h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
	eventually(t, "restore after active turn settles", func() bool {
		return len(probe.snapshot()) == 1 && len(h.userInputs(holder)) == 2
	})
	if got := h.userInputs(holder)[1]; !strings.Contains(got, "restore[active]") || !strings.Contains(got, "cause=restart") {
		t.Fatalf("active restart restore = %q", got)
	}
}

func TestRestartRestorePrecedesQueuedInputAndExcludesScanSpawn(t *testing.T) {
	probe := &restoreProbe{state: "restart-order"}
	h := newLeadHarness(t)
	h.opts.RestoreBundle = probe.render
	h.start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)

	h.runner.Close()
	rec := h.record(id)
	rec.Queued = []QueuedInput{{Text: "queued while daemon was down"}}
	if err := writeRecord(wsdashboard.SessionDir(h.dir, id), rec); err != nil {
		t.Fatal(err)
	}
	h.holders.restart()
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "restart holder scan", entered)

	spawned := h.lead(BackendClaude, LeadOptions{})
	h.runner.conv(spawned.ID).do(func() {
		if note := h.runner.conv(spawned.ID).compactNote; note != "" {
			t.Fatalf("session spawned during scan inherited restart restore: %q", note)
		}
	})
	if got := len(h.userInputs(holder)); got != 1 {
		t.Fatalf("queued input crossed the replay gate: %d inputs", got)
	}

	release()
	eventually(t, "restore prepended to queued input", func() bool { return len(h.userInputs(holder)) == 2 })
	text := h.userInputs(holder)[1]
	if restoreAt, workAt := strings.Index(text, "restore[restart-order]"), strings.Index(text, "queued while daemon was down"); restoreAt < 0 || workAt < 0 || restoreAt > workAt {
		t.Fatalf("restart restore/queued input ordering = %q", text)
	}
	if calls := probe.snapshot(); len(calls) != 1 || calls[0].key != rec.WSKey || calls[0].cause != RestoreRestart {
		t.Fatalf("restart render calls = %+v", calls)
	}
}

func TestRestartRestoreSleepingSessionWaitsForWake(t *testing.T) {
	h, got := childHarness(t)
	w := h.spawnWorker("medium")
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	p.reply("done")
	eventually(t, "worker report", func() bool { return len(got.lead()) == 1 })
	h.asleep(w.ID)

	probe := &restoreProbe{state: "stale"}
	h.runner.Close()
	h.config.RestoreBundle = probe.render
	h.start()
	probe.setState("fresh-at-wake")
	time.Sleep(60 * time.Millisecond)
	if calls := probe.snapshot(); len(calls) != 0 {
		t.Fatalf("sleeping restore rendered without a wake: %+v", calls)
	}
	if starts := h.backend.startCount(); starts != 1 {
		t.Fatalf("restore alone woke sleeping session: %d process starts", starts)
	}

	if _, err := h.runner.Send(w.ID, "wake work"); err != nil {
		t.Fatal(err)
	}
	p2 := h.backend.waitProc(t, 1)
	text := p2.nextSent(t)
	if restoreAt, workAt := strings.Index(text, "restore[fresh-at-wake]"), strings.Index(text, "wake work"); restoreAt < 0 || workAt < 0 || restoreAt > workAt {
		t.Fatalf("sleeping wake ordering = %q", text)
	}
	if calls := probe.snapshot(); len(calls) != 1 || calls[0].cause != RestoreRestart || calls[0].key != h.record(w.ID).WSKey {
		t.Fatalf("sleeping restart calls = %+v", calls)
	}
}

func TestRestoreProviderFailureInjectsKeyBearingDiagnostic(t *testing.T) {
	probe := &restoreProbe{state: "partial state must not appear"}
	probe.setError(errors.New("fixture provider unavailable"))
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.Catalog = ClaudeCatalog()
	h.opts.Backends[BackendClaude] = reg
	h.opts.RestoreBundle = probe.render
	h.start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)
	resume := false
	if got := h.runner.Compact(key, "", &resume); got.Status != "queued" {
		t.Fatal(got)
	}
	eventually(t, "native compact", func() bool { return len(h.userInputs(holder)) == 2 })
	h.send(id, "ordinary work")
	compactBoundary(h, id, holder)
	eventually(t, "provider failure diagnostic", func() bool { return len(h.userInputs(holder)) == 3 })
	text := h.userInputs(holder)[2]
	for _, want := range []string{key, "could not be rendered", "fixture provider unavailable", "workflow_state", "ordinary work"} {
		if !strings.Contains(text, want) {
			t.Fatalf("provider diagnostic lacks %q: %q", want, text)
		}
	}
	if strings.Contains(text, "partial state must not appear") {
		t.Fatalf("provider failure injected partial bundle: %q", text)
	}
	if !strings.Contains(h.log.String(), "restore bundle: fixture provider unavailable") {
		t.Fatalf("provider failure was not logged: %s", h.log.String())
	}
}

func TestRestoreBundleT0UsesNoPersistedShapeAcrossNewOldNewStart(t *testing.T) {
	h, got := childHarness(t)
	w := h.spawnWorker("medium")
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	p.reply("done")
	eventually(t, "worker report", func() bool { return len(got.lead()) == 1 })
	h.asleep(w.ID)
	h.runner.Close()

	dir := wsdashboard.SessionDir(h.config.Dir, w.ID)
	before, err := os.ReadFile(recordPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	probe := &restoreProbe{state: "new daemon"}
	h.config.RestoreBundle = probe.render
	h.start()
	h.runner.conv(w.ID).do(func() {
		if note := h.runner.conv(w.ID).compactNote; !strings.HasPrefix(note, restartRestoreMarker) {
			t.Fatalf("new daemon did not hold an in-memory restart marker: %q", note)
		}
	})
	afterNew, err := os.ReadFile(recordPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterNew) {
		t.Fatalf("new daemon rewrote the existing record for restore\nbefore: %s\nafter: %s", before, afterNew)
	}
	if bytes.Contains(afterNew, []byte("restore")) || bytes.Contains(afterNew, []byte("bundle")) {
		t.Fatalf("restore state leaked into persisted record: %s", afterNew)
	}

	h.runner.Close()
	h.config.RestoreBundle = nil // an old daemon has no runtime provider
	h.start()
	h.runner.conv(w.ID).do(func() {
		if note := h.runner.conv(w.ID).compactNote; note != "" {
			t.Fatalf("old daemon behavior observed a new-only marker: %q", note)
		}
	})
	afterOld, err := os.ReadFile(recordPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterOld) {
		t.Fatalf("old daemon behavior did not see identical persisted bytes\nbefore: %s\nafter: %s", before, afterOld)
	}
	if rec, err := readRecord(dir); err != nil || rec.SchemaVersion != recordSchemaVersion || rec.WSKey != h.record(w.ID).WSKey {
		t.Fatalf("new reopen could not reconstruct the unchanged record: rec=%+v err=%v", rec, err)
	}
}
