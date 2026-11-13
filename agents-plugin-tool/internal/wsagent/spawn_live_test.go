package wsagent

import (
	"context"
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
	if _, err := h.runner.CloseConversation(child.ID); err != nil {
		t.Fatal(err)
	}
	waitRows(wslive.StatusDone, wslive.StatusDone)
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
