package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// Measure only the compact tools/list contract cached by harnesses, not
// annotations or other entry metadata. Check worker wording as well as leads.
func TestAgentToolDefinitionSize(t *testing.T) {
	for _, surface := range []string{"ws-lead", "wsflow-lead", "forwarding-worker"} {
		t.Run(surface, func(t *testing.T) {
			t.Setenv(envNamespace, "ws")
			t.Setenv(envNoAgent, "")
			t.Setenv(wsagent.EnvPiToken, "")
			t.Setenv(wsagent.EnvPiEndpoint, "")
			t.Setenv(envAllowedTools, "")
			manifests := wsManifests()
			var agents PiAgents = newFakePiAgents()
			if surface == "wsflow-lead" {
				t.Setenv(envNamespace, "wsflow")
				pkg := dashboardPackages[1]
				manifests = pkg.own + ";" + pkg.dashboard
			}
			if surface == "forwarding-worker" {
				agents = nil
				t.Setenv(wsagent.EnvPiToken, "size-test-token")
				t.Setenv(wsagent.EnvPiEndpoint, "http://127.0.0.1:1")
			}
			f := agentLeadFixture(t, manifests, agents)
			if surface == "forwarding-worker" && !f.server.piForwards() {
				t.Fatal("worker must use token forwarding")
			}
			// Decode the protocol response, matching the shared all-tools guard.
			var resp struct {
				Result struct {
					Tools []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
						InputSchema any    `json:"inputSchema"`
					} `json:"tools"`
				} `json:"result"`
			}
			if err := json.Unmarshal([]byte(callToolsList(t, f.server)), &resp); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, tool := range resp.Result.Tools {
				name := tool.Name
				if !strings.HasPrefix(name, "agent.") {
					continue
				}
				count++
				entry := map[string]any{"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema}
				data, err := json.Marshal(entry)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s: %d bytes", name, len(data))
				if len(data) > toolSchemaByteCap {
					t.Errorf("%s: %d bytes exceeds %d-byte tool definition cap", name, len(data), toolSchemaByteCap)
				}
			}
			if count != len(agentToolNames) {
				t.Fatalf("measured %d agent.* tools, want %d", count, len(agentToolNames))
			}
		})
	}
}
