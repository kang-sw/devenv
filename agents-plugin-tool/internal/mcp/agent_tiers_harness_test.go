package mcp

import (
	"encoding/json"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func TestHostedAgyAgentTiersUsesDescriptorHarness(t *testing.T) {
	regs := descriptorRegistrations()
	reg := regs[wsagent.BackendAgy]
	// A transport-only backend makes tiers available without an agy install
	// or provider call. The MCP handler and tier resolver remain real.
	reg.Backend = &lifecycleBackend{}
	reg.WorkerCapable = true
	reg.WorkerProfile = wsagent.AgentWorkerProfile(wsagent.BackendAgy)
	regs[wsagent.BackendAgy] = reg
	s, _ := hostedBackendConnection(t, wsagent.BackendAgy, regs)
	if got := s.currentHarness(); got != "" {
		t.Fatalf("detected harness = %q, want empty", got)
	}
	raw := callToolOnce(t, s, 1, "agent_tiers", map[string]any{"format": "json"})
	if toolIsError(t, raw) {
		t.Fatal(raw)
	}
	var tiers wsagent.AgentTiers
	if err := json.Unmarshal([]byte(toolText(t, raw)), &tiers); err != nil {
		t.Fatal(err)
	}
	if tiers.Harness != "agy" {
		t.Fatalf("harness = %q, want agy", tiers.Harness)
	}
	if len(tiers.Native) != 4 {
		t.Fatalf("native tiers = %+v, want four bare tiers", tiers.Native)
	}
	for i, name := range []string{"small", "medium", "large", "xlarge"} {
		tier := tiers.Native[i]
		if tier.Tier != name || tier.Model == "" || tier.Effort == "" {
			t.Errorf("native tier %d = %+v", i, tier)
		}
	}
	if got := s.currentHarness(); got != "" {
		t.Fatalf("agent.tiers changed detected harness to %q", got)
	}
}
