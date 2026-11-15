package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLeadTicketResearchDerivationContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			pkg := "agents-plugin"
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
				pkg = "agents-plugin-wsflow"
			}
			t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", pkg, "rsrc"))
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			body := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-ticket"}))
			if strings.TrimSpace(body) == "" {
				t.Fatal("empty rendered lead-ticket document")
			}
			if !strings.Contains(body, namespace+"/") {
				t.Fatal("missing namespace binding")
			}
		})
	}
}
