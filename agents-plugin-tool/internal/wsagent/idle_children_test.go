package wsagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Both hosted leads and spawned parents must keep their process while a
// child runs, even after the timer expires and another child has finished.
func TestPiIdleStopWaitsForChildren(t *testing.T) {
	for _, lead := range []bool{true, false} {
		name := "worker"
		if lead {
			name = "lead"
		}
		t.Run(name, func(t *testing.T) {
			h, _ := newFakePiRecoveryHarness(t)
			if lead {
				h.globalProfile(LeadProfilePi, `{"backend":"pi","lead":true,"idle_timeout_seconds":1}`)
			} else {
				h.globalProfile(PiWorkerProfile, `{"backend":"pi","idle_timeout_seconds":1}`)
			}
			var id, holder string
			kind := ParentConversation
			if lead {
				id = h.lead(BackendPi, LeadOptions{}).ID
				h.send(id, "work")
				holder = h.holder(id)
				kind = ParentLead
			} else {
				st, proc := spawnRecoveryWorker(t, h)
				id, holder = st.ID, proc
			}
			spawn := func() (State, string) {
				st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "child", Tier: "small", Parent: &ParentRef{Kind: kind, ReplyID: id}})
				if err != nil {
					t.Fatal(err)
				}
				return st, h.holder(st.ID)
			}
			first, firstHolder := spawn()
			last, _ := spawn()
			h.holders.out(holder, `{"type":"agent_settled"}`)
			h.idle(id)
			time.Sleep(1200 * time.Millisecond)
			if !h.state(id).Process || h.holders.opCount("stop "+holder) != 0 {
				t.Fatal("running children must suppress the expired idle timer")
			}

			// Consume this result as pi.wait would, so report delivery does
			// not start a parent turn and mask the idle-timer behavior.
			waited := make(chan TurnResult, 1)
			go func() {
				res, _ := h.runner.wait(context.Background(), first.ID, 5*time.Second, true)
				waited <- res
			}()
			eventually(t, "the child wait", func() bool {
				c := h.runner.conv(first.ID)
				c.mu.Lock()
				defer c.mu.Unlock()
				return len(c.parentWaits) > 0
			})
			h.holders.out(firstHolder, recoveryPiReply("done")...)
			if res := <-waited; res.Status != TurnDone {
				t.Fatalf("child result: %+v", res)
			}
			time.Sleep(1200 * time.Millisecond)
			if !h.state(id).Process || h.holders.opCount("stop "+holder) != 0 {
				t.Fatal("the remaining running child must keep the parent alive")
			}
			if _, err := h.runner.CloseConversation(last.ID); err != nil {
				t.Fatal(err)
			}
			// The last child ending starts a fresh interval, not an
			// immediate stop from the timer that already expired.
			time.Sleep(200 * time.Millisecond)
			if !h.state(id).Process {
				t.Fatal("last child ending must re-arm a full idle interval")
			}
			eventually(t, "parent idle stop after the last child ends", func() bool {
				return !h.state(id).Process && h.holders.opCount("stop "+holder) == 1
			})
			h.quiesce(id)
			if st := h.state(id); st.Status != StatusIdle || st.Error != "" {
				t.Fatalf("idle stop is not a failure: %+v", st)
			}
			if lead {
				if row := h.row(id); !row.HostStopped || row.HostExitReason != "idle_timeout" || row.HostStoppedAt == "" {
					t.Fatalf("idle stop reason: %+v", row)
				}
				// A heartbeat and daemon replacement preserve the reason.
				h.restart()
				h.runner.conv(id).do(func() { h.runner.conv(id).writeRegistration() })
				if row := h.row(id); row.HostExitReason != "idle_timeout" {
					t.Fatalf("idle reason lost after restart: %+v", row)
				}
			}
			if !strings.Contains(h.log.String(), "process stopped: idle_timeout") {
				t.Fatal("idle stop must be distinguishable in the runner log")
			}
		})
	}
}

func TestPiIdleStopWaitsForChildBackgroundWork(t *testing.T) {
	testPiIdleStopAfterChildBackgroundWork(t, false)
}

func TestPiIdleStopAfterChildProcessEndedWithBackgroundWork(t *testing.T) {
	testPiIdleStopAfterChildBackgroundWork(t, true)
}

func testPiIdleStopAfterChildBackgroundWork(t *testing.T, processEnds bool) {
	t.Helper()
	restoreGrace := bgResumeGrace
	bgResumeGrace = 60 * time.Millisecond
	t.Cleanup(func() { bgResumeGrace = restoreGrace })
	h, _ := newFakePiRecoveryHarness(t)
	h.globalProfile(PiWorkerProfile, `{"backend":"pi","idle_timeout_seconds":1}`)
	parent, holder := spawnRecoveryWorker(t, h)
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "child", Tier: "small", Parent: &ParentRef{Kind: ParentConversation, ReplyID: parent.ID}})
	if err != nil {
		t.Fatal(err)
	}
	childHolder := h.holder(child.ID)
	waited := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.wait(context.Background(), child.ID, 5*time.Second, true)
		waited <- res
	}()
	eventually(t, "child wait", func() bool {
		c := h.runner.conv(child.ID)
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.parentWaits) > 0
	})
	h.holders.out(childHolder, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":1,"monitors":0}`)))
	h.holders.out(childHolder, recoveryPiReply("waiting for background work")...)
	eventually(t, "child background state", func() bool {
		st := h.state(child.ID)
		return st.Status == StatusIdle && st.BackgroundTasks == 1 && AgentStatus(st) == AgentBackground
	})
	select {
	case res := <-waited:
		t.Fatalf("background worker settled before background work ended: %+v", res)
	default:
	}
	h.holders.out(holder, `{"type":"agent_settled"}`)
	h.idle(parent.ID)
	time.Sleep(1200 * time.Millisecond)
	if !h.state(parent.ID).Process || !h.state(child.ID).Process {
		t.Fatal("background work must protect the background worker and its parent")
	}
	if processEnds {
		// Clearing background work on process exit settles the held turn
		// and re-arms the parent's expired idle timer.
		h.idle(child.ID)
		h.holders.exit(childHolder, 0)
	} else {
		h.holders.out(childHolder, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":0,"monitors":0}`)))
	}
	if res := <-waited; res.Status != TurnDone {
		t.Fatalf("child result after background completion: %+v", res)
	}
	eventually(t, "task completion", func() bool {
		c := h.runner.conv(child.ID)
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.bgTasks == 0
	})
	time.Sleep(200 * time.Millisecond)
	if !h.state(parent.ID).Process {
		t.Fatal("background completion must rearm a full parent idle interval")
	}
	eventually(t, "worker and parent idle stop after background completion", func() bool {
		return !h.state(child.ID).Process && !h.state(parent.ID).Process
	})
}

func TestPiIdleNowWaitsForChildBackgroundWork(t *testing.T) {
	for _, lead := range []bool{false, true} {
		kind := ParentConversation
		if lead {
			kind = ParentLead
		}
		r := &Runner{convs: map[string]*conv{}}
		parent := &conv{r: r, rec: Record{ReplyID: "parent", Lead: lead}}
		child := &conv{rec: Record{Parent: &ParentRef{Kind: kind, ReplyID: "parent"}}, bgTasks: 1}
		r.convs["child"] = child
		if parent.idleNow() {
			t.Errorf("lead=%v: child background work must keep the parent alive", lead)
		}
		child.bgTasks = 0
		if !parent.idleNow() {
			t.Errorf("lead=%v: settled child must release the idle gate", lead)
		}
	}
}

// Failed startup retains the child's prompt for retry, but its failed send
// has settled and must not count as running work in the parent's idle gate.
func TestPiIdleStopAfterChildStartFailure(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	h.globalProfile(LeadProfilePi, `{"backend":"pi","lead":true,"idle_timeout_seconds":1}`)
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "work")
	holder := h.holder(id)
	h.holders.set(func() { h.holders.spawnErr = errors.New("child launch refused") })
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "child", Tier: "small", Parent: &ParentRef{Kind: ParentLead, ReplyID: id}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.runner.Wait(context.Background(), child.ID, 5*time.Second)
	if err != nil || res.Status != TurnFailed {
		t.Fatalf("consume child failure: %+v, %v", res, err)
	}
	h.quiesce(child.ID)
	if st := h.state(child.ID); st.Process || st.Held != 1 || st.Error == "" {
		t.Fatalf("failed startup must retain input for retry: %+v", st)
	}
	h.holders.set(func() { h.holders.spawnErr = nil })
	h.holders.out(holder, `{"type":"agent_settled"}`)
	h.idle(id)
	time.Sleep(200 * time.Millisecond)
	if !h.state(id).Process {
		t.Fatal("parent must get a fresh idle interval after responding to failure")
	}
	eventually(t, "idle stop despite the failed child's retained input", func() bool {
		return !h.state(id).Process && h.holders.opCount("stop "+holder) == 1
	})
}
