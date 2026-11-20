package mcp

import (
	"sync/atomic"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

type planUsageActivityProbe struct {
	fakeLiveHost
	calls atomic.Int32
}

func (h *planUsageActivityProbe) PlanUsageActivity() {
	h.calls.Add(1)
}

func TestUserStartedAgyActivity(t *testing.T) {
	registrations := descriptorRegistrations()
	agy := registrations[wsagent.BackendAgy]
	agy.PlanUsageActivity = true
	registrations[wsagent.BackendAgy] = agy
	registrations[wsagent.BackendCodex] = wsagent.BackendRegistration{ManualHarness: wsagent.BackendCodex}
	runner := wsagent.NewRunner(wsagent.Options{Backends: registrations})
	t.Cleanup(runner.Close)

	for _, tc := range []struct {
		name, harness string
		host, hosted  bool
		want          int32
	}{
		{name: "agy", harness: wsagent.BackendAgy, host: true, want: 1},
		{name: "codex", harness: wsagent.BackendCodex, host: true},
		{name: "claude", harness: wsagent.BackendClaude, host: true},
		{name: "pi", harness: wsagent.BackendPi, host: true},
		{name: "no daemon host", harness: wsagent.BackendAgy},
		{name: "hosted agy", harness: wsagent.BackendAgy, host: true, hosted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, key, _ := dashboardSessionServer(t, dashboardGitRoot(t), true)
			t.Cleanup(s.Close)
			s.agentBackendSource = runner
			s.sessionHarness = tc.harness
			probe := &planUsageActivityProbe{}
			if tc.host {
				s.live.host = probe
			}
			if tc.hosted {
				s.hostedKey = key
				s.hostedOnce.Do(func() {})
			}

			callToolWithKey(t, s, 1, key, "todo.list", nil)
			if got := probe.calls.Load(); got != tc.want {
				t.Fatalf("activity calls = %d, want %d", got, tc.want)
			}
		})
	}
}
