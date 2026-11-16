package wsagent

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPiLeadStaleBundleIsLoudEvenWithSourceDeps(t *testing.T) {
	spec, root := piLeadFixture(t)
	piBundlePackage(t, root, false)
	piLeadDeps(t, filepath.Dir(root))
	spec.Lead.PluginIsolation = "isolated"
	spec.Lead.Env = map[string]string{EnvHostedSessionKey: "lead-key"}
	warning := ""
	spec.Lead.PluginWarning = func(text string) { warning = text }
	spec.Lead.WSMCP = func() (MCPCommand, error) { return MCPCommand{Command: "never-executed"}, nil }
	_, args, err := piLeadLaunch("never-executed", nil, spec)
	if err != nil || !strings.Contains(warning, "EXTENSION NOT LOADED") || !strings.Contains(warning, "stale") || !strings.Contains(warning, "ws-execute") || !slices.Contains(args, "builtin:mcp") {
		t.Fatalf("stale launch: args %v, warning %q, err %v", args, warning, err)
	}
	if slices.Contains(args, filepath.Join(root, "src", "index.ts")) {
		t.Fatal("stale hash must not silently select unverified source")
	}
}

func TestPiLeadHealthNotificationReachesDashboardError(t *testing.T) {
	warning := "[ws Pi] EXTENSION NOT LOADED OR INCOMPLETE: missing native lead tools ws-execute, ws__playbook_read"
	line, _ := json.Marshal(map[string]any{"type": "extension_ui_request", "method": "notify", "message": warning, "notifyType": "error"})
	mapper := newPiMapper(StartSpec{Lead: &LeadSpec{}})
	if got := mapper.mapLine(line, nopReplies{}); len(got) != 1 || got[0].CommandError != warning || got[0].TurnEnd {
		t.Fatalf("watchdog notification must show a session error without settling the turn: %+v", got)
	}
	if got := newPiMapper(StartSpec{}).mapLine(line, nopReplies{}); len(got) != 0 {
		t.Fatalf("worker notification must not become a hosted lead error: %+v", got)
	}
	boundary := mapper.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{})
	if len(boundary) != 2 || !boundary[0].TurnStart || boundary[1].CommandError != warning {
		t.Fatalf("warning must be reapplied after the clearing boundary: %+v", boundary)
	}
	if got := mapper.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{}); len(got) != 1 || !got[0].TurnStart {
		t.Fatalf("warning carry must be consumed, not leak into a recovered turn: %+v", got)
	}
	ordinary := []byte(`{"type":"extension_ui_request","method":"notify","message":"ordinary extension notification"}`)
	if got := mapper.mapLine(ordinary, nopReplies{}); len(got) != 0 {
		t.Fatalf("unrelated notification must remain unchanged: %+v", got)
	}
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, string(line))
	eventually(t, "dashboard-visible extension error", func() bool { return h.state(id).Error == warning })
	// A degraded session stays alive and can finish a mail/input-driven turn.
	if !h.state(id).Process {
		t.Fatal("watchdog must not disconnect the reachable lead")
	}
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	// Autonomous/push-driven run: no h.send opens this turn. Pi writes the
	// extension notification first, then its public agent_start RPC line.
	h.holders.out(holder, string(line), `{"type":"agent_start"}`)
	eventually(t, "health error survives autonomous turn start", func() bool {
		st := h.state(id)
		return st.Status == StatusRunning && st.Error == warning && st.Process
	})
	h.holders.out(holder, `{"type":"agent_settled"}`)
	h.idle(id)
}
