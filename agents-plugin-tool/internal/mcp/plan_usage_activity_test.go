package mcp

import (
	"path/filepath"
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

func resolvedDashboardDir(t *testing.T, path string) string {
	t.Helper()
	root := filepath.Dir(filepath.Dir(path))
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(resolvedRoot, rel)
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
		noManifest    bool
		want          int32
	}{
		{name: "agy", harness: wsagent.BackendAgy, host: true, want: 1},
		{name: "agy without dashboard manifest", harness: wsagent.BackendAgy, host: true, noManifest: true, want: 1},
		{name: "codex", harness: wsagent.BackendCodex, host: true},
		{name: "claude", harness: wsagent.BackendClaude, host: true},
		{name: "pi", harness: wsagent.BackendPi, host: true},
		{name: "no daemon host", harness: wsagent.BackendAgy},
		{name: "hosted agy", harness: wsagent.BackendAgy, host: true, hosted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatedDashboardDir := dashboardSessionEnv(t)
			s, key, _ := dashboardSessionServer(t, dashboardGitRoot(t), true)
			t.Cleanup(s.Close)
			gotDashboardDir, err := s.dashboard.stateDir()
			if err != nil {
				t.Fatal(err)
			}
			gotDashboardDir = resolvedDashboardDir(t, gotDashboardDir)
			isolatedDashboardDir = resolvedDashboardDir(t, isolatedDashboardDir)
			if gotDashboardDir != isolatedDashboardDir {
				t.Fatalf("dashboard state dir = %q; want isolated %q", gotDashboardDir, isolatedDashboardDir)
			}
			s.agentBackendSource = runner
			s.sessionHarness = tc.harness
			if tc.noManifest {
				s.declared = declaredConfig{}
			}
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
