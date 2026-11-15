package mcp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func TestConfigRegisteredAgentProfile(t *testing.T) {
	s, _, key := profileServer(t)
	// Real runner metadata, no process start. The extra backend exists only
	// in this fixture's private config; production still admits Claude/Pi.
	s.pi.agents = wsagent.NewRunner(wsagent.Options{Backends: map[string]wsagent.BackendRegistration{
		wsagent.BackendClaude: {LeadProfile: wsagent.LeadProfileClaude, PermissionModes: wsagent.LeadPermissionModes},
		wsagent.BackendPi:     {LeadProfile: wsagent.LeadProfilePi},
		"fixture":             {LeadProfile: "fixture-lead", PermissionModes: []string{"review"}},
	}})
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
			if knob.Error != "" || !reflect.DeepEqual(knob.ValueSchema.Properties.Backend.Enum, []string{"claude", "fixture", "pi"}) {
				t.Fatalf("registered catalog = %+v", knob)
			}
			return
		}
	}
	t.Fatal("catalog has no profile family")
}
