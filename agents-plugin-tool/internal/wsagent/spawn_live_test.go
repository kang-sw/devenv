package wsagent

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

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
		c.spawnDirty = true
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
	if before != 1 {
		t.Fatalf("initial closed snapshot reads = %d, want 1", before)
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
	if got := reads.Load(); got != before+1 {
		t.Fatalf("new closure snapshot reads = %d, want %d", got, before+1)
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
