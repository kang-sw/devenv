package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real MCP entry and namespace rendering, not native agent lifecycle.
func TestLeadDelegateReadContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
			}
			t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			body := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-delegate"}))
			if strings.TrimSpace(body) == "" {
				t.Fatal("lead-delegate playbook is empty")
			}
			for _, token := range []string{namespace + ":lead-discuss", namespace + ":lead-ticket", namespace + ":lead-review", namespace + ":lead-ship", namespace + ":lead-run", namespace + "/playbook.render"} {
				if !strings.Contains(body, token) {
					t.Errorf("render missing namespace token %q", token)
				}
			}
			if strings.Contains(body, "{{.") {
				t.Error("render contains unresolved variable")
			}
			retired := callToolOnce(t, s, 2, "playbook.read", map[string]any{"name": "lead-prefer-subagent"})
			if !strings.Contains(retired, `"isError":true`) {
				t.Fatalf("retired procedure still resolves: %s", retired)
			}
		})
	}
}
