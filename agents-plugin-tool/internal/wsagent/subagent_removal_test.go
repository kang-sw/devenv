package wsagent

import (
	"slices"
	"testing"
)

// TestRemovesSubagentsScope: hosted leads and agent.spawn children launch
// without the native subagent tool; the internal and dashboard profiles
// keep their own tool setup, the Claude tracker support profile's only
// tool included.
func TestRemovesSubagentsScope(t *testing.T) {
	for _, tc := range []struct {
		rec  Record
		want bool
	}{
		{Record{Backend: BackendClaude, Lead: true, Profile: LeadProfileClaude}, true},
		{Record{Backend: BackendPi, Lead: true, Profile: LeadProfilePi}, true},
		{Record{Backend: BackendClaude, Profile: AgentWorkerProfile(BackendClaude)}, true},
		{Record{Backend: BackendPi, Profile: AgentWorkerProfile(BackendPi)}, true},
		{Record{Backend: BackendClaude, Profile: SupportProfileClaude}, false},
		{Record{Backend: BackendClaude, Profile: SearchAnswerProfileClaude}, false},
		{Record{Backend: BackendPi, Profile: PiWorkerProfile}, false},
		{Record{Backend: BackendPi, Profile: PiMiniLeadProfile}, false},
		{Record{Backend: BackendClaude, Profile: "agent-worker-codex"}, true},
		{Record{Backend: BackendAgy, Profile: AgentWorkerProfile(BackendAgy)}, true},
	} {
		if got := RemovesSubagents(tc.rec); got != tc.want {
			t.Errorf("RemovesSubagents(%s lead=%v %s) = %v, want %v", tc.rec.Backend, tc.rec.Lead, tc.rec.Profile, got, tc.want)
		}
	}
}

// TestAgentSpawnChildStartsWithoutSubagents: the runner starts an
// agent.spawn child with the native subagent tool removed, at its first
// start and at the start that wakes it.
func TestAgentSpawnChildStartsWithoutSubagents(t *testing.T) {
	h, _ := agentHarness(t, nil)
	id, p := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	if spec := h.backend.start(0); !spec.RemoveSubagents || spec.Profile.Backend != BackendClaude {
		t.Fatalf("the child's first start = %+v", spec)
	}
	p.reply("done")
	h.asleep(id)
	n := h.backend.startCount()
	if _, err := h.runner.Deliver(id, Delivery{Text: "again"}); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, n)
	if spec := h.backend.start(n); !spec.RemoveSubagents {
		t.Fatalf("the waking start = %+v", spec)
	}
}

// TestSupportConversationKeepsSubagents: a tracker support conversation
// starts with its profile's Agent tool and nothing removed.
func TestSupportConversationKeepsSubagents(t *testing.T) {
	sh := newSupportHarness(t)
	sh.send("alpha", "hello")
	sh.claude.waitProc(t, 0)
	if spec := sh.claude.start(0); spec.RemoveSubagents || !slices.Contains(spec.Profile.Tools, "Agent") {
		t.Fatalf("the support conversation's start = %+v", spec)
	}
}
