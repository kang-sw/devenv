package wsagent

import (
	"fmt"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Hold the actual parent loop so its one-slot wake channel necessarily
// coalesces A/B/C; a latest-only projection loses A/B on this path.
func TestHostedObservedCoalescedChildHistorySurvivesRestart(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{})
	spawn := func(parent ParentRef) (State, string) {
		t.Helper()
		st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "small", Parent: &parent})
		if err != nil {
			t.Fatal(err)
		}
		return st, h.holder(st.ID)
	}
	direct, directHolder := spawn(ParentRef{Kind: ParentLead, ReplyID: lead.ID})
	deep, deepHolder := spawn(ParentRef{Kind: ParentConversation, ReplyID: direct.ID})
	blocked, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { h.runner.conv(lead.ID).do(func() { close(blocked); <-release }); close(done) }()
	<-blocked
	released := false
	defer func() {
		if !released {
			close(release)
			<-done
		}
	}()
	for _, child := range []struct {
		st     State
		holder string
	}{{direct, directHolder}, {deep, deepHolder}} {
		for i, model := range []string{"A", "B", "C"} {
			h.holders.out(child.holder, string(observedPiLine(fmt.Sprintf("%s-%d", child.st.ID, i), model, "", "", int64(1000+i))))
		}
		eventually(t, "all child responses applied while parent blocked", func() bool { return len(h.state(child.st.ID).Observations) == 3 })
	}
	close(release)
	released = true
	<-done
	check := func() {
		t.Helper()
		eventually(t, "all direct/deep transitions reach parent", func() bool {
			snap := h.snapshot(lead.ID)
			for _, id := range []string{direct.ID, deep.ID} {
				o := snap.ObservedAgents["pi:"+id]
				if o == nil || len(o.History) != 3 || o.Current.Model != "C" || o.Current.At != 1002 {
					return false
				}
			}
			return snap.Lead.Observed == nil
		})
		records, err := wslive.ReadObserved(h.runner.conv(lead.ID).dir)
		if err != nil || len(records) != 6 {
			t.Fatalf("parent immutable evidence: %+v / %v", records, err)
		}
	}
	check()
	h.restart()
	check()
	// The child's complete own window is restored too, not only Current.
	for _, id := range []string{direct.ID, deep.ID} {
		if got := h.state(id); len(got.Observations) != 3 {
			t.Fatalf("child window lost after restart: %+v", got)
		}
	}
}
