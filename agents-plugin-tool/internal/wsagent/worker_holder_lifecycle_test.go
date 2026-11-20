package wsagent

import (
	"errors"
	"strings"
	"testing"
)

func TestHeldWorkerLaunchFailureHasNoDirectFallback(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			h, _ := workerRecoveryHarness(t, backend)
			h.holders.set(func() { h.holders.spawnErr = errors.New("fixture holder launch refused") })
			caller := LeadCaller(leadParent.ReplyID, h.root, BackendClaude, h.opts.Config)
			child, err := h.runner.AgentSpawn(caller, AgentSpawnRequest{Prompt: "work", Tier: backend + ".small", Authority: "workdir-write"})
			if err != nil {
				t.Fatal(err)
			}
			id := agentID(child.Address)
			eventually(t, "holder refusal settles", func() bool { return strings.Contains(h.state(id).Error, "fixture holder launch refused") })
			if h.state(id).Process || h.record(id).HolderID != "" || h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 {
				t.Fatalf("failed launch left a process: %v", h.holders.opList())
			}
		})
	}
}

func TestHeldWorkerInterruptKeepsHolderAndCloseRevokesIt(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			h, _ := workerRecoveryHarness(t, backend)
			id, holder := spawnHeldRecoveryWorker(t, h, backend, "workdir-write")
			before := h.record(id)
			if _, _, err := h.runner.Interrupt(id); err != nil {
				t.Fatal(err)
			}
			h.quiesce(id)
			if h.record(id).HolderID != holder || h.holders.opCount("stop") != 0 {
				t.Fatal("turn interrupt ended the holder")
			}
			if _, err := h.runner.CloseConversation(id); err != nil {
				t.Fatal(err)
			}
			eventually(t, "worker close ends holder", func() bool { return h.holders.opCount("stop "+holder) == 1 })
			if rec := h.record(id); rec.HolderID != "" || rec.HandedOffAt != "" || rec.Capability != "" || rec.MCPToken != "" {
				t.Fatal("close retained holder fence or tokens")
			}
			if before.MCPToken != "" {
				if _, _, ok := h.runner.ResolveMCPToken(before.MCPToken); ok {
					t.Fatal("closed token still resolves")
				}
			}
		})
	}
}

func TestPrunedWorkerHolderIsSwept(t *testing.T) {
	h := newLeadHarness(t)
	holder := h.holders.add(holderSession{Kind: holderKindAgentWorker, Backend: BackendCodex, ReplyID: "pruned-child"})
	h.start()
	wait(t, "orphan sweep", h.runner.rejoined)
	if h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("unowned worker holder remains: %v", h.holders.opList())
	}
}

func TestHeldWorkerMailWaitsForReplacementReconciliation(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			h, got := workerRecoveryHarness(t, backend)
			id, holder := spawnHeldRecoveryWorker(t, h, backend, "workdir-write")
			first := h.record(id).TurnMark
			h.runner.Close()
			h.holders.restart()
			completeRecoveryWorker(t, h, backend, holder)
			entered, release := h.gateScan()
			defer release()
			h.startAt("http://127.0.0.1:1")
			wait(t, "blocked holder scan", entered)
			if _, err := h.runner.Deliver(id, Delivery{From: "senderreplyid", Text: "queued follow-up"}); err != nil {
				t.Fatal(err)
			}
			h.quiesce(id)
			if rec := h.record(id); rec.TurnMark != first || h.holders.opCount("spawn") != 1 {
				t.Fatal("queued mail ran before reconciliation")
			}
			release()
			eventually(t, "queued mail wakes same holder", func() bool { rec := h.record(id); return rec.TurnOpen && rec.TurnMark > first })
			if rec := h.record(id); rec.HolderID != holder || h.holders.opCount("spawn") != 1 {
				t.Fatal("queued mail replaced the surviving worker")
			}
			if reports := got.list(); len(reports) != 1 || reports[0].Result.Status != TurnDone {
				t.Fatalf("replacement report: %+v", reports)
			}
		})
	}
}
