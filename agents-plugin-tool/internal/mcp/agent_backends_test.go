package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func registerFixtureAgentBackends(s *Server) {
	// Real runner metadata, no process start. The extra backend exists only
	// in this fixture's private config; production admits Claude, Pi and
	// agy, whose builtin profiles must resolve.
	s.pi.agents = wsagent.NewRunner(wsagent.Options{Backends: map[string]wsagent.BackendRegistration{
		wsagent.BackendClaude: {LeadProfile: wsagent.LeadProfileClaude, PermissionModes: wsagent.LeadPermissionModes},
		wsagent.BackendPi:     {LeadProfile: wsagent.LeadProfilePi},
		wsagent.BackendAgy:    {LeadProfile: wsagent.LeadProfileAgy, PermissionModes: wsagent.AgyPermissionModes},
		"fixture":             {LeadProfile: "fixture-lead", PermissionModes: []string{"review"}},
	}})
}

func TestConfigRegisteredAgentProfile(t *testing.T) {
	s, _, key := profileServer(t)
	registerFixtureAgentBackends(s)
	resp := keyedCall(t, s, 1, key, "config.tune", map[string]any{
		"key": "agent.profile.fixture-lead", "scope": "project",
		"value": map[string]any{"backend": "fixture", "lead": true, "permission_mode": "review"},
	})
	if toolIsError(t, resp) {
		t.Fatalf("registered profile write: %s", resp)
	}
	got := configGetJSON(t, s, 2, map[string]any{"key": "agent.profile.fixture-lead", "session_key": key})
	raw, err := json.Marshal(got.Value)
	if err != nil {
		t.Fatal(err)
	}
	var p wsagent.Profile
	if err := json.Unmarshal(raw, &p); err != nil || p.Backend != "fixture" || !p.Lead || p.PermissionMode != "review" || got.Scope != "project" {
		t.Fatalf("registered profile read = %+v / %s, %v", got, raw, err)
	}
	resp = keyedCall(t, s, 3, key, "config.list", map[string]any{"format": "json"})
	if toolIsError(t, resp) {
		t.Fatalf("registered profile catalog: %s", resp)
	}
	var catalog struct {
		Knobs []struct {
			ID          string `json:"id"`
			Error       string `json:"error"`
			ValueSchema struct {
				Properties struct {
					Backend struct {
						Enum []string `json:"enum"`
					} `json:"backend"`
				} `json:"properties"`
			} `json:"value_schema"`
		} `json:"knobs"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, knob := range catalog.Knobs {
		if knob.ID == "agent.profile.<name>" {
			if knob.Error != "" || !reflect.DeepEqual(knob.ValueSchema.Properties.Backend.Enum, []string{"agy", "claude", "fixture", "pi"}) {
				t.Fatalf("registered catalog = %+v", knob)
			}
			return
		}
	}
	t.Fatal("catalog has no profile family")
}

func TestConfigRegisteredAgentProfileRefusesAbsentBackend(t *testing.T) {
	s, _, key := profileServer(t)
	registerFixtureAgentBackends(s)
	resp := keyedCall(t, s, 1, key, "config.tune", map[string]any{
		"key": "agent.profile.fixture-lead", "scope": "project",
		"value": map[string]any{"backend": "unregistered", "lead": true, "permission_mode": "review"},
	})
	want := `config.tune: agent.profile.fixture-lead backend must be "agy" or "claude" or "fixture" or "pi"; got "unregistered"`
	if !toolIsError(t, resp) || toolText(t, resp) != want {
		t.Fatalf("unregistered profile write = %s, want refusal %q", resp, want)
	}
	if got := configGetJSON(t, s, 2, map[string]any{"key": "agent.profile.fixture-lead", "session_key": key}); got.Scope != configGetUnsetScope {
		t.Fatalf("refused profile was stored: %+v", got)
	}
}

func TestTrackerSupportRegisteredAgentBackend(t *testing.T) {
	f := newTrackerSupportFixture(t)
	registerFixtureAgentBackends(f.s)
	writeGlobalProfiles(t, map[string]string{
		"fixture-support": `{"backend":"fixture","ws_tools":["tracker.query"]}`,
	})
	key, rec := f.conversation(t, f.meDir, "fixture-support")
	rec.Backend = "fixture"
	if err := wsagent.WriteRecordForTest(f.dashDir, rec); err != nil {
		t.Fatal(err)
	}
	rp, err := wsagent.ResolveProfile(f.s.configOptions(), rec.Root, rec.Profile, f.s.agentBackends()...)
	if err != nil || rp.Definition.Backend != "fixture" {
		t.Fatalf("registered profile = %+v, %v", rp, err)
	}
	entry, ok := f.s.sessions.lookup(key)
	if !ok {
		t.Fatal("missing support key")
	}
	scope, ok := f.s.trackerSupport(key, entry)
	if !ok || scope.store != "me" || !scope.tools["tracker.query"] || scope.tools["tracker.update"] {
		t.Fatalf("registered support scope = %+v, %v", scope, ok)
	}
	resp := f.call(t, "tracker.query", f.as(key, map[string]any{}))
	if isToolError(resp) {
		t.Fatalf("registered support query refused: %s", resp)
	}
}

func TestPlaybookRegisteredAgentBackendProfileFallback(t *testing.T) {
	f := newPiRecordFixture(t)
	registerFixtureAgentBackends(f.s)
	extension := filepath.Join(f.root, "native-extension")
	if err := os.MkdirAll(extension, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extension, "package.json"), []byte(`{"name":"ws-pi-bridge"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGlobalProfiles(t, map[string]string{
		"fixture-worker": `{"backend":"fixture","plugins":["native-extension"]}`,
	})
	// A pre-snapshot Pi record resolves its named profile as configured now,
	// even when that profile has since moved to a registered extension backend.
	key, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, key, wsagent.Record{Backend: wsagent.BackendPi, Profile: "fixture-worker"})
	rp, err := wsagent.ResolveProfile(f.s.configOptions(), f.root, "fixture-worker", f.s.agentBackends()...)
	if err != nil || rp.Definition.Backend != "fixture" || wsagent.PiMCPOnly(wsagent.Record{Backend: wsagent.BackendPi, Root: f.root}, &rp.Definition) {
		t.Fatalf("registered fallback profile = %+v, %v", rp, err)
	}
	if f.s.callerDaemonAgents(key) {
		t.Fatal("playbook fallback failed to resolve the registered profile's native extension")
	}
}
