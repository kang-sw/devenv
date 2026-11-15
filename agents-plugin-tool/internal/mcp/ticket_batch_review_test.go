package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestBatchPromotionRenderedContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			t.Setenv(envNoAgent, "0")
			pkg := "agents-plugin"
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
				pkg = "agents-plugin-wsflow"
			}
			root := filepath.Join("..", "..", "..", pkg, "rsrc")
			t.Setenv("WS_RSRC_ROOT", root)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			lead := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-ticket"}))
			design, _, err := renderPlaybookBody(s, root, "ticket-reviewer-design", nil, wsconfig.Options{}, "", "", "", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{"lead-ticket": lead, "ticket-reviewer-design": design} {
				if strings.TrimSpace(body) == "" {
					t.Errorf("empty rendered %s document", name)
				}
			}
		})
	}
}
