package wsagent

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// isolationBackend captures the actual launch contract without running a harness.
// Its intentional launch error also leaves the session safe to relaunch.
type isolationBackend struct {
	starts chan StartSpec
}

func (b *isolationBackend) Start(_ context.Context, spec StartSpec) (Process, error) {
	spec.Lead.PluginWarning("bundled Pi extension unavailable; using ws MCP bridge")
	// Failed launches leave input held, so another wake may retry before the
	// test consumes its observation. Keep the first without blocking the loop.
	select {
	case b.starts <- spec:
	default:
	}
	return nil, errors.New("test launch stopped")
}

func (b *isolationBackend) next(t *testing.T) StartSpec {
	t.Helper()
	select {
	case spec := <-b.starts:
		return spec
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lead launch")
		return StartSpec{}
	}
}

func TestLeadIsolationRequestAndRelaunch(t *testing.T) {
	for _, harness := range []string{BackendClaude, BackendPi} {
		for _, isolation := range []string{"", "default", "isolated"} {
			t.Run(harness+"/"+isolation, func(t *testing.T) {
				h := newLeadHarness(t)
				backend := &isolationBackend{starts: make(chan StartSpec, 2)}
				reg := h.opts.Backends[harness]
				reg.Backend = backend
				h.opts.Backends[harness] = reg
				h.opts.WSMCP = func(root string) (MCPCommand, error) {
					if root != h.root {
						t.Errorf("MCP root = %q, want %q", root, h.root)
					}
					return MCPCommand{Command: "bundled-ws-mcp", Env: map[string]string{EnvAllowedTools: "stale", "WS_MAILBOX": "stale", EnvPiToken: "stale"}}, nil
				}
				h.start()
				body := map[string]any{"root": h.root, "harness": harness}
				if isolation != "" {
					body["plugin_isolation"] = isolation
				}
				response := h.route(localBrowser, http.MethodPost, "/agents/leads", body)
				st := decodeState(t, response)
				want := leadPluginIsolation(isolation)
				if response.status != http.StatusCreated || st.PluginIsolation != want {
					t.Fatalf("spawn = %d %s", response.status, response.body)
				}
				if rec := h.record(st.ID); rec.PluginIsolation != want {
					t.Fatalf("persisted isolation = %q, want %q", rec.PluginIsolation, want)
				}
				h.send(st.ID, "first")
				spec := backend.next(t)
				if spec.Lead.PluginIsolation != want {
					t.Fatalf("first launch isolation = %q, want %q", spec.Lead.PluginIsolation, want)
				}
				mcp, err := spec.Lead.WSMCP()
				if err != nil || mcp.Command != "bundled-ws-mcp" || mcp.Env[EnvHostedSessionKey] != h.record(st.ID).WSKey || mcp.Env[EnvHostedSession] != "1" || mcp.Env[EnvAllowedTools] != "" || mcp.Env["WS_MAILBOX"] != "" || mcp.Env[EnvPiToken] != "" {
					t.Fatalf("lead fallback launcher = %+v, %v", mcp, err)
				}
				if rec := h.record(st.ID); rec.PluginWarning == "" || h.state(st.ID).PluginWarning != rec.PluginWarning {
					t.Fatalf("launch warning not recorded and exposed: %+v", rec)
				}
				h.restart()
				if got := h.state(st.ID); got.PluginIsolation != want || got.PluginWarning == "" {
					t.Fatalf("recovered state = %+v", got)
				}
				h.send(st.ID, "resume")
				if spec := backend.next(t); spec.Lead.PluginIsolation != want {
					t.Fatalf("relaunch isolation = %q, want %q", spec.Lead.PluginIsolation, want)
				}
			})
		}
	}
}

func TestLeadIsolationRejectsUnknownValue(t *testing.T) {
	h := newLeadHarness(t).start()
	response := h.route(localBrowser, http.MethodPost, "/agents/leads", map[string]any{
		"root": h.root, "harness": BackendPi, "plugin_isolation": "all",
	})
	if response.status != http.StatusBadRequest || len(h.runner.List()) != 0 {
		t.Fatalf("invalid isolation = %d %s", response.status, response.body)
	}
}

func TestLegacyLeadIsolationDefaultsAtLaunch(t *testing.T) {
	h := newLeadHarness(t)
	backend := &isolationBackend{starts: make(chan StartSpec, 1)}
	h.opts.Backends[BackendPi] = BackendRegistration{Backend: backend, LeadProfile: LeadProfilePi}
	h.start()
	st := h.lead(BackendPi, LeadOptions{})
	_, err := h.runner.act(st.ID, func(c *conv) error {
		c.update(func(rec *Record) { rec.PluginIsolation = "" })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.send(st.ID, "resume legacy")
	if spec := backend.next(t); spec.Lead.PluginIsolation != "default" {
		t.Fatalf("legacy launch isolation = %q", spec.Lead.PluginIsolation)
	}
}

// piLaunchBackend runs the real Pi lead launch decision against a fixture Pi
// and reports it, without starting a process.
type piLaunchBackend struct {
	command string
	starts  chan piLaunch
}

type piLaunch struct {
	command string
	args    []string
	err     error
}

func (b *piLaunchBackend) Start(_ context.Context, spec StartSpec) (Process, error) {
	command, args, err := piLeadLaunch(b.command, nil, spec)
	// As with isolationBackend, a retry must not wait for the test to drain
	// its first launch observation: h.send waits for this loop to quiesce.
	select {
	case b.starts <- piLaunch{command, args, err}:
	default:
	}
	return nil, errors.New("test launch stopped")
}

func TestIsolationBackendRetriesDoNotBlock(t *testing.T) {
	spec, root := piLeadFixture(t)
	piLeadBundle(t, root)
	spec.Lead.PluginIsolation = "isolated"
	spec.Lead.PluginWarning = func(string) {}
	isolation := &isolationBackend{starts: make(chan StartSpec, 1)}
	pi := &piLaunchBackend{command: "fixture-pi", starts: make(chan piLaunch, 1)}
	for name, backend := range map[string]Backend{"isolation": isolation, "pi launch": pi} {
		t.Run(name, func(t *testing.T) {
			returns(t, "rejected launch retries with a full observation channel", func() {
				for range 3 {
					if _, err := backend.Start(context.Background(), spec); err == nil {
						t.Error("fixture launch must fail so input stays held")
					}
				}
			})
		})
	}
	if len(isolation.starts) != 1 || len(pi.starts) != 1 {
		t.Fatal("retries must retain the pending launch observation")
	}
	if got := <-pi.starts; got.err != nil || got.command != "fixture-pi" {
		t.Fatalf("Pi launch observation = %+v", got)
	}
}

// A legacy lead record (no isolation field) on a Pi without its JavaScript SDK
// starts isolated with a recorded warning, and its record keeps the legacy
// value; a Pi with the SDK still starts in default mode.
func TestLegacyPiLeadWithoutSDKStartsIsolated(t *testing.T) {
	for _, withSDK := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "sdk"}[withSDK], func(t *testing.T) {
			h := newLeadHarness(t)
			bundle := filepath.Join(t.TempDir(), "agents-plugin-ws")
			piLeadFile(t, filepath.Join(bundle, ".claude-plugin", "plugin.json"), `{"name":"ws"}`)
			restore := piBundledPluginRoot
			piBundledPluginRoot = func(string) (string, string, error) { return bundle, "", nil }
			t.Cleanup(func() { piBundledPluginRoot = restore })
			entry := piLeadBundle(t, filepath.Join(filepath.Dir(bundle), "agents-plugin-pi"))
			usePreparedPiFixture(t, filepath.Dir(bundle), filepath.Join(filepath.Dir(bundle), "agents-plugin-pi"))
			command := filepath.Join(t.TempDir(), "pi")
			if withSDK {
				pkg := filepath.Join(t.TempDir(), "host-pi")
				command = filepath.Join(pkg, "dist", "cli.js")
				piLeadFile(t, filepath.Join(pkg, "package.json"), `{"name":"@earendil-works/pi-coding-agent"}`)
				piLeadFile(t, filepath.Join(pkg, "dist", "index.js"), "")
			}
			piLeadFile(t, command, "")
			backend := &piLaunchBackend{command: command, starts: make(chan piLaunch, 1)}
			h.opts.Backends[BackendPi] = BackendRegistration{Backend: backend, LeadProfile: LeadProfilePi}
			h.start()
			st := h.lead(BackendPi, LeadOptions{})
			if _, err := h.runner.act(st.ID, func(c *conv) error {
				c.update(func(rec *Record) { rec.PluginIsolation = "" })
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			h.restart()
			h.send(st.ID, "resume legacy")
			var got piLaunch
			select {
			case got = <-backend.starts:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for lead launch")
			}
			if got.err != nil {
				t.Fatalf("launch error: %v", got.err)
			}
			// Default mode runs the resolver adapter in front of the CLI;
			// isolated mode runs the Pi command itself with the bundled entry.
			adapter := len(got.args) > 0 && filepath.Base(got.args[0]) == "ws-lead.mjs"
			isolated := got.command == command && !adapter && slices.Contains(got.args, entry)
			rec := h.record(st.ID)
			if rec.PluginIsolation != "" {
				t.Fatalf("record isolation rewritten to %q", rec.PluginIsolation)
			}
			if withSDK {
				if !adapter || isolated || rec.PluginWarning != "" {
					t.Fatalf("SDK Pi must start in default mode: %+v, warning %q", got, rec.PluginWarning)
				}
				return
			}
			if got.command != command || !isolated || rec.PluginWarning != piNoSDKWarning {
				t.Fatalf("standalone Pi must start isolated: %+v, warning %q", got, rec.PluginWarning)
			}
		})
	}
}
