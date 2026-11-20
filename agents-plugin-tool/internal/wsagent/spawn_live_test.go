package wsagent

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestAgentSpawnHostedWorkerRows(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.Now = time.Now
	host := &summarySnapshotHost{}
	h.opts.LiveHost = host
	backend := &fakeBackend{}
	h.opts.Backends[BackendCodex] = BackendRegistration{Backend: backend, LeadProfile: LeadProfileCodex, WorkerCapable: true, WorkerProfile: AgentWorkerProfile(BackendCodex)}
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, "small", "", "codex-small", BackendCodex, "high"); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.runner.SetEndpoint("http://127.0.0.1:1")
	lead := h.lead(BackendClaude, LeadOptions{})
	h.send(lead.ID, "work")
	h.holder(lead.ID)
	res, err := h.runner.AgentSpawn(LeadCaller(lead.ID, h.root, BackendClaude, h.opts.Config), AgentSpawnRequest{Prompt: "work", Tier: "codex.small", Authority: "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	id := agentID(res.Address)
	proc := backend.waitProc(t, 0)
	proc.nextSent(t)
	waitStatus := func(status string) {
		t.Helper()
		eventually(t, "spawn row "+status, func() bool {
			snap, _ := host.snapshot(wsdashboard.SessionDir(h.dir, lead.ID))
			for _, row := range snap.Agents {
				if row.ID == "pi:"+id && row.Status == status && row.Harness == BackendCodex {
					return true
				}
			}
			return false
		})
	}
	waitStatus(wslive.StatusRunning)
	proc.reply("done")
	waitStatus(wslive.StatusDone)
}

// The hosted Claude lead has no intake-fed live session: only its runner's
// writer can publish these rows. Exercise real spawn/watch/replay plumbing.
func TestHostedLeadSpawnRowsSurviveDaemonReplacement(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{})
	h.send(lead.ID, "work")
	leadHolder := h.holder(lead.ID)
	other := h.lead(BackendClaude, LeadOptions{})
	spawn := func(parent ParentRef, label string) (State, string) {
		t.Helper()
		st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: label, Label: label, Tier: "small", Parent: &parent})
		if err != nil {
			t.Fatal(err)
		}
		holder := h.holder(st.ID)
		h.holders.out(holder, `{"type":"agent_start"}`)
		return st, holder
	}
	child, childHolder := spawn(ParentRef{Kind: ParentLead, ReplyID: lead.ID}, "worker")
	leaf, leafHolder := spawn(ParentRef{Kind: ParentConversation, ReplyID: child.ID}, "leaf")
	spawn(ParentRef{Kind: ParentLead, ReplyID: other.ID}, "unrelated")
	rows := func() map[string]wslive.Agent {
		out := map[string]wslive.Agent{}
		for _, a := range h.snapshot(lead.ID).Agents {
			out[a.ID] = a
		}
		return out
	}
	waitRows := func(childStatus, leafStatus string) {
		t.Helper()
		eventually(t, "hosted spawn rows", func() bool {
			r := rows()
			return len(r) == 2 && r["pi:"+child.ID].Status == childStatus && r["pi:"+leaf.ID].Status == leafStatus
		})
		r := rows()
		if r["pi:"+leaf.ID].ParentID != "pi:"+child.ID || r["pi:"+child.ID].ReplyID != child.ID || r["pi:"+child.ID].AgentKind != wslive.AgentKindPi {
			t.Fatalf("identity/nesting = %+v", r)
		}
	}
	waitRows(wslive.StatusRunning, wslive.StatusRunning)
	// Native feed duplicates preserve native activity, not a second row.
	h.runner.conv(lead.ID).do(func() {
		h.runner.conv(lead.ID).apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "native-worker", SessionKey: h.record(child.ID).WSKey, Caption: strPtr("native activity")})
	})
	waitRows(wslive.StatusRunning, wslive.StatusRunning)
	if a := rows()["pi:"+child.ID]; a.Caption != "native activity" {
		t.Fatalf("native activity lost: %+v", a)
	}
	// Losing the lead harness must not end independently held workers.
	h.holders.exit(leadHolder, 1)
	eventually(t, "lead process exits", func() bool { return !h.state(lead.ID).Process })
	waitRows(wslive.StatusRunning, wslive.StatusRunning)

	h.restart()
	waitRows(wslive.StatusRunning, wslive.StatusRunning)
	if snap := h.snapshot(lead.ID); snap.ReplyID != lead.ID || snap.Adapter != BackendClaude {
		t.Fatalf("wrong authoritative snapshot: %+v", snap)
	}
	// Consume the leaf report through the parent's wait. Otherwise the normal
	// report handover can requeue input and resume the parent after its exit.
	waited := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.wait(context.Background(), leaf.ID, 5*time.Second, true)
		waited <- res
	}()
	eventually(t, "parent waiting for leaf", func() bool {
		c := h.runner.conv(leaf.ID)
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.parentWaits) > 0
	})
	h.holders.out(leafHolder, recoveryPiReply("leaf done")...)
	if res := <-waited; res.Status != TurnDone {
		t.Fatalf("leaf wait: %+v", res)
	}
	waitRows(wslive.StatusRunning, wslive.StatusDone)
	h.holders.exit(childHolder, 1)
	waitRows(wslive.StatusFailed, wslive.StatusDone)
	// A settled failure is durable; losing lastErr on replacement must not
	// turn an open failed worker into a successful row.
	h.restart()
	waitRows(wslive.StatusFailed, wslive.StatusDone)
	h.send(child.ID, "genuine next turn")
	h.holders.out(h.holder(child.ID), `{"type":"agent_start"}`)
	waitRows(wslive.StatusRunning, wslive.StatusDone)
	if _, err := h.runner.CloseConversation(child.ID); err != nil {
		t.Fatal(err)
	}
	waitRows(wslive.StatusDone, wslive.StatusDone)
}

func TestMappedPiFailureReachesRecursiveWatchFeedBeforeAnotherUpdate(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	worker, _ := spawnRecoveryWorker(t, h)
	var mu sync.Mutex
	feed := wslive.NewState("intake-lead")
	var terminal []State
	stop := h.runner.Watch(func(st State) {
		if SpawnLead(st, h.runner.Get) != leadParent.ReplyID {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		feed.Apply(SpawnAgentEvent(st, ""), time.Now().UnixMilli())
		if st.Parent != nil && st.Parent.Kind == ParentConversation && st.Status == StatusIdle && st.Held == 0 {
			terminal = append(terminal, st)
		}
	})
	defer stop()
	leaf, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "fail", Tier: "small", Parent: &ParentRef{Kind: ParentConversation, ReplyID: worker.ID}})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(leaf.ID)
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"mapped failure","content":[]}}`, `{"type":"agent_settled"}`)
	eventually(t, "one coherent failed descendant emission", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(terminal) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	if len(terminal) != 1 || terminal[0].Error != "mapped failure" {
		t.Fatalf("terminal emission before later updates: %+v", terminal)
	}
	found := false
	for _, row := range feed.Snapshot(time.Now().UnixMilli()).Agents {
		if row.ID == "pi:"+leaf.ID {
			found = true
			if row.Status != wslive.StatusFailed || row.ParentID != "pi:"+worker.ID {
				t.Fatalf("recursive feed lost mapped failure: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("recursive feed lost failed leaf")
	}
}

func TestHostedSpawnProjectionListsOncePerInvalidationNotPerStreamWrite(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	var mu sync.Mutex
	counts := map[string]int{}
	h.opts.spawnListed = func(id string) {
		mu.Lock()
		counts[id]++
		mu.Unlock()
	}
	count := func(id string) int {
		mu.Lock()
		defer mu.Unlock()
		return counts[id]
	}
	h.start()
	lead := h.lead(BackendClaude, LeadOptions{})
	for range 3 {
		worker, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
		if err != nil {
			t.Fatal(err)
		}
		h.holder(worker.ID)
		leaf, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "leaf", Parent: &ParentRef{Kind: ParentConversation, ReplyID: worker.ID}})
		if err != nil {
			t.Fatal(err)
		}
		h.holder(leaf.ID)
	}
	c := h.runner.conv(lead.ID)
	var refreshDelta, streamDelta int
	c.do(func() {
		// Drain pending coalesced wakes and force one complete tree refresh.
		for len(c.spawnC) > 0 {
			<-c.spawnC
		}
		before := count(lead.ID)
		c.spawnDirty, c.spawnNext = true, time.Time{}
		c.live.writeSnapshot()
		refreshDelta = count(lead.ID) - before
		before = count(lead.ID)
		for range 20 {
			c.apply(wslive.Event{Kind: wslive.KindReply, Text: "streaming"})
			c.live.writeSnapshot() // a heartbeat is cheap too
		}
		streamDelta = count(lead.ID) - before
	})
	if refreshDelta != 1 || streamDelta != 0 {
		t.Fatalf("global listings: one refresh=%d, ordinary stream/heartbeat writes=%d", refreshDelta, streamDelta)
	}
}

func TestHostedUsageWakesDoNotRereadClosedSnapshots(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	var reads atomic.Int64
	h.opts.closedSnapshotRead = func(string) { reads.Add(1) }
	h.start()
	lead := h.lead(BackendClaude, LeadOptions{})
	spawn := func() State {
		t.Helper()
		st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
		if err != nil {
			t.Fatal(err)
		}
		h.holder(st.ID)
		return st
	}
	setUsage := func(id string, tokens int64, rate float64) {
		t.Helper()
		c := h.runner.conv(id)
		c.do(func() {
			c.apply(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens}, OutputTps: &rate})
		})
	}
	finished, active := spawn(), spawn()
	file := piSessionFile(h.runner.conv(finished.ID).dir)
	setUsage(finished.ID, 1200, 10)
	if _, err := h.runner.CloseConversation(finished.ID); err != nil {
		t.Fatal(err)
	}
	if st := h.state(finished.ID); st.ContextTokens == nil || *st.ContextTokens != 1200 {
		t.Fatalf("closed usage = %+v", st)
	}
	before := reads.Load()
	if before != 0 {
		t.Fatalf("closed state retained in memory should need no snapshot reads, got %d", before)
	}
	for i := range 5 {
		setUsage(active.ID, int64(2000+i), float64(20+i))
		eventually(t, "usage-only wake reaches the hosted row", func() bool {
			for _, a := range h.snapshot(lead.ID).Agents {
				if a.ID == "pi:"+active.ID {
					return a.ContextTokens == int64(2000+i) && a.OutputTps == float64(20+i)
				}
			}
			return false
		})
	}
	if got := reads.Load(); got != before {
		t.Fatalf("usage-only wakes reread closed snapshots: %d -> %d", before, got)
	}
	// A closed Pi child can reopen. Its next closure must not reuse the
	// previous lifetime's cached usage, even within the same timestamp second.
	if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.send(finished.ID, "continue")
	h.holder(finished.ID)
	setUsage(finished.ID, 3400, 30)
	if _, err := h.runner.CloseConversation(finished.ID); err != nil {
		t.Fatal(err)
	}
	if st := h.state(finished.ID); st.ContextTokens == nil || *st.ContextTokens != 3400 {
		t.Fatalf("reclosed usage reused stale cache: %+v", st)
	}
	if got := reads.Load(); got != before {
		t.Fatalf("new closure should reuse live state without snapshot I/O: %d -> %d", before, got)
	}
}

// Pi sleep/deleted are dashboard presentation labels, not new reducer
// statuses. Pin the projection consumed by both intake and hosted leads.
func TestSpawnAgentEventLegacyStatusContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   State
		want string
	}{
		{"closed", State{Status: StatusClosed}, wslive.StatusDone},
		{"closed stale failure", State{Status: StatusClosed, Error: "old failure", Held: 1}, wslive.StatusDone},
		{"idle", State{Status: StatusIdle}, wslive.StatusDone},
		{"held", State{Status: StatusIdle, Held: 1}, wslive.StatusRunning},
		{"running", State{Status: StatusRunning}, wslive.StatusRunning},
		{"failed", State{Status: StatusIdle, Error: "failure"}, wslive.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.st.ID = "child"
			ev := SpawnAgentEvent(tc.st, "child-key")
			if ev.Status != tc.want || ev.AgentKind != wslive.AgentKindPi {
				t.Fatalf("projection = %+v, want legacy status %q", ev, tc.want)
			}
			feed := wslive.NewState("lead-key")
			if _, err := feed.Apply(ev, 100); err != nil {
				t.Fatal(err)
			}
			rows := feed.Snapshot(100).Agents
			if len(rows) != 1 || rows[0].Status != tc.want || rows[0].AgentKind != wslive.AgentKindPi {
				t.Fatalf("snapshot = %+v", rows)
			}
		})
	}
}

func TestSpawnAgentEventCarriesHarnessAndNeutralLabel(t *testing.T) {
	for _, backend := range []string{BackendAgy, BackendClaude, BackendCodex, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			ev := SpawnAgentEvent(State{ID: "child1", Backend: backend, Status: StatusRunning}, "child-key")
			if ev.Harness != backend || ev.AgentKind != wslive.AgentKindPi {
				t.Fatalf("event = %+v, want harness %q and agentKind %q", ev, backend, wslive.AgentKindPi)
			}
			if backend != BackendPi && strings.Contains(ev.Label, "Pi subagent") {
				t.Fatalf("non-Pi default label should not say Pi subagent: %q", ev.Label)
			}
			feed := wslive.NewState("lead-key")
			if _, err := feed.Apply(ev, 100); err != nil {
				t.Fatal(err)
			}
			rows := feed.Snapshot(100).Agents
			if len(rows) != 1 || rows[0].Harness != backend || rows[0].AgentKind != wslive.AgentKindPi {
				t.Fatalf("snapshot = %+v", rows)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestSpawnLeadRejectsUnknownAndCyclicOwnership(t *testing.T) {
	states := map[string]State{
		"child": {ID: "child", Parent: &ParentRef{Kind: ParentLead, ReplyID: "lead"}},
		"cycle": {ID: "cycle", Parent: &ParentRef{Kind: ParentConversation, ReplyID: "cycle"}},
	}
	get := func(id string) (State, error) {
		if st, ok := states[id]; ok {
			return st, nil
		}
		return State{}, errUnknown
	}
	for parent, want := range map[string]string{"child": "lead", "cycle": "", "missing": ""} {
		if got := SpawnLead(State{ID: "leaf", Parent: &ParentRef{Kind: ParentConversation, ReplyID: parent}}, get); got != want {
			t.Fatalf("parent %s: got %q, want %q", parent, got, want)
		}
	}
}

func TestSpawnAgentEventProjectsHoldingAndAlive(t *testing.T) {
	for _, tc := range []struct {
		name        string
		st          State
		wantStatus  string
		wantHolding bool
		wantAlive   bool
	}{
		{"holding alive", State{Status: StatusIdle, Process: true, Holding: true}, wslive.StatusDone, true, true},
		{"alive idle", State{Status: StatusIdle, Process: true, Holding: false}, wslive.StatusDone, false, true},
		{"asleep", State{Status: StatusIdle, Process: false, Holding: false}, wslive.StatusDone, false, false},
		{"running", State{Status: StatusRunning, Process: true, Holding: false}, wslive.StatusRunning, false, true},
		{"failed alive", State{Status: StatusIdle, Error: "boom", Process: true}, wslive.StatusFailed, false, true},
		{"closed clears liveness", State{Status: StatusClosed, Process: true, Holding: true}, wslive.StatusDone, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.st.ID = "child"
			ev := SpawnAgentEvent(tc.st, "child-key")
			if ev.Status != tc.wantStatus || ev.Holding != tc.wantHolding || ev.Alive != tc.wantAlive {
				t.Fatalf("event = %+v, want status=%q holding=%v alive=%v", ev, tc.wantStatus, tc.wantHolding, tc.wantAlive)
			}
		})
	}
}

func TestConvStateHoldingTracksHeldTurn(t *testing.T) {
	h, _ := agentHarness(t, nil)
	h.runner.SetEndpoint("http://127.0.0.1:1")
	parentID, parentProc := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "parent", Tier: "small", Authority: "read-only"})
	childID, childProc := h.agentRunning(h.holderOf(parentID), AgentSpawnRequest{Prompt: "child", Tier: "small", Authority: "read-only"})
	parentProc.reply("waiting on child")
	eventually(t, "parent holding", func() bool {
		st, err := h.runner.Get(parentID)
		return err == nil && st.Holding && st.Process
	})
	childProc.reply("child done")
	eventually(t, "parent releases hold", func() bool {
		st, err := h.runner.Get(parentID)
		return err == nil && !st.Holding
	})
	if _, err := h.runner.CloseConversation(childID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.CloseConversation(parentID); err != nil {
		t.Fatal(err)
	}
	if st, err := h.runner.Get(parentID); err != nil || st.Holding || st.Process {
		t.Fatalf("closed parent kept holding/process: %+v, err=%v", st, err)
	}
}
