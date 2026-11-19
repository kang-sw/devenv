package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func TestPiDaemonAgentInitializeCapability(t *testing.T) {
	for _, tc := range []struct {
		name, params string
		want         bool
	}{
		{"old", `{"clientInfo":{"name":"ws-pi-bridge"},"capabilities":{}}`, false},
		{"true", `{"capabilities":{"experimental":{"wsPiDaemonAgents":true}}}`, true},
		{"false", `{"capabilities":{"experimental":{"wsPiDaemonAgents":false}}}`, false},
		{"string", `{"capabilities":{"experimental":{"wsPiDaemonAgents":"true"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServerWithHarness(t, "pi")
			s.observePiDaemonAgents([]byte(tc.params))
			if got := s.clientPiDaemonAgents(); got != tc.want {
				t.Fatalf("capability = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlaybookPiCapabilityAndToolGates(t *testing.T) {
	for _, tc := range []struct {
		name, init, allow string
		forward, want     bool
	}{
		{"old-client", `{"clientInfo":{"name":"ws-pi-bridge"}}`, "", true, false},
		{"marked", `{"clientInfo":{"name":"ws-pi-bridge"},"capabilities":{"experimental":{"wsPiDaemonAgents":true}}}`, "", true, true},
		{"unavailable", `{"clientInfo":{"name":"ws-pi-bridge"},"capabilities":{"experimental":{"wsPiDaemonAgents":true}}}`, "", false, false},
		{"filtered", `{"clientInfo":{"name":"ws-pi-bridge"},"capabilities":{"experimental":{"wsPiDaemonAgents":true}}}`, "playbook.render,agent.spawn", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPiRecordFixture(t)
			if !tc.forward {
				t.Setenv(wsagent.EnvPiToken, "")
			}
			if tc.allow != "" {
				t.Setenv("WS_MCP_ALLOWED_TOOLS", tc.allow)
			}
			body := renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":`+tc.init+`}`)
			if got := strings.Contains(body, "ws/agent.spawn"); got != tc.want {
				t.Fatalf("agent guidance = %v, want %v: %s", got, tc.want, body)
			}
			for _, old := range []string{"ws-agent-spawn", "ws-agent-send", "ws-fork", "the `explore` researcher", "pi.spawn", "pi.send"} {
				if strings.Contains(body, old) {
					t.Errorf("obsolete guidance %q", old)
				}
			}
		})
	}
}

func TestPlaybookPiMCPOnlyRequiresProfileTools(t *testing.T) {
	f := newPiRecordFixture(t)
	for _, tools := range [][]string{{"agent.spawn"}, {"pi.spawn", "pi.send"}} {
		key := f.spawnedKey(t, &wsagent.Profile{Backend: wsagent.BackendPi, WSTools: tools})
		body := renderedBody(t, f.s, key, "ticket-worker-elevated")
		if strings.Contains(body, "ws/agent.spawn") || strings.Contains(body, "ws/pi.spawn") {
			t.Fatalf("worker with %v received daemon guidance: %s", tools, body)
		}
	}
}
