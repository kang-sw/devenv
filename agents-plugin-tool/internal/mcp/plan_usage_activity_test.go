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
	pi := registrations[wsagent.BackendPi]
	pi.ManualHarness = wsagent.BackendPi
	registrations[wsagent.BackendPi] = pi
	runner := wsagent.NewRunner(wsagent.Options{Backends: registrations})
	t.Cleanup(runner.Close)

	for _, tc := range []struct {
		name, harness, laterHarness string
		host, hosted                bool
		noManifest                  bool
		wantEnabled                 bool
		want                        int32
	}{
		{name: "agy", harness: wsagent.BackendAgy, host: true, wantEnabled: true, want: 1},
		{name: "agy without dashboard manifest", harness: wsagent.BackendAgy, host: true, noManifest: true, wantEnabled: true, want: 1},
		{name: "agy learned after first call", laterHarness: wsagent.BackendAgy, host: true, wantEnabled: true, want: 1},
		{name: "codex", harness: wsagent.BackendCodex, host: true},
		{name: "claude", harness: wsagent.BackendClaude, host: true},
		{name: "pi", harness: wsagent.BackendPi, host: true},
		{name: "no daemon host", harness: wsagent.BackendAgy, wantEnabled: true},
		{name: "hosted agy", harness: wsagent.BackendAgy, host: true, hosted: true, wantEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatedDashboardDir := dashboardSessionEnv(t)
			root := dashboardGitRoot(t)
			t.Setenv(envConfigManifests, claudeManifestPath+";"+dashboardPackages[0].dashboard)
			probe := &planUsageActivityProbe{}
			opts := ServerOptions{
				Root:          root,
				Version:       "test",
				Harness:       tc.harness,
				AgentBackends: runner,
			}
			if tc.host {
				opts.Live = probe
			}
			s := NewServerWithOptions(opts)
			t.Cleanup(s.Close)
			key, _ := parseLoginResponse(t, callLogin(t, s, 900000, root, nil))
			gotDashboardDir, err := s.dashboard.stateDir()
			if err != nil {
				t.Fatal(err)
			}
			gotDashboardDir = resolvedDashboardDir(t, gotDashboardDir)
			isolatedDashboardDir = resolvedDashboardDir(t, isolatedDashboardDir)
			if gotDashboardDir != isolatedDashboardDir {
				t.Fatalf("dashboard state dir = %q; want isolated %q", gotDashboardDir, isolatedDashboardDir)
			}
			if tc.harness != "" && s.currentHarness() != tc.harness {
				t.Fatalf("current harness = %q, want %q", s.currentHarness(), tc.harness)
			}
			if tc.noManifest {
				s.declared = declaredConfig{}
			}
			if tc.hosted {
				s.hostedKey = key
				s.hostedOnce.Do(func() {})
			}
			if tc.laterHarness != "" {
				callToolWithKey(t, s, 1, key, "todo.list", nil)
				if got := probe.calls.Load(); got != 0 {
					t.Fatalf("activity calls before harness detection = %d, want 0", got)
				}
				s.observeHarness("test", tc.laterHarness)
			}
			if got := s.planUsageActivityEnabled(); got != tc.wantEnabled {
				t.Fatalf("activity enabled = %t, want %t", got, tc.wantEnabled)
			}

			callToolWithKey(t, s, 2, key, "todo.list", nil)
			if got := probe.calls.Load(); got != tc.want {
				t.Fatalf("activity calls = %d, want %d", got, tc.want)
			}
		})
	}
}
