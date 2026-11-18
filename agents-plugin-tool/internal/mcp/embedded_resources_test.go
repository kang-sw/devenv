package mcp

import (
	"os"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wspackage"
)

// These are the union of playbook.read targets in the v0.50.0 ws/wsflow skill
// shims (including lead-proceed's lead-run alias). They pass no context vars;
// session_key is a transport argument, not a declared playbook variable.
func TestEmbeddedResourcesAcceptOlderSkillShims(t *testing.T) {
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") != "1" {
		return
	}
	t.Setenv(wspackage.EnvDevCheckout, "")
	for _, namespace := range []string{"ws", "wsflow"} {
		s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Env: []string{
			"WS_MCP_NAMESPACE=" + namespace,
			"WS_RSRC_ROOT=" + t.TempDir(),
			"WS_RSRC_DEV_CHECKOUT=" + t.TempDir(),
			"WS_CONFIG_HOME=" + t.TempDir(),
			"WS_CACHE_HOME=" + t.TempDir(),
		}})
		t.Cleanup(s.Close)
		for _, name := range []string{"lead-audit-doc", "lead-bootstrap", "lead-check-blockers", "lead-coauthor-paged-document", "lead-delegate", "lead-discuss", "lead-review", "lead-run", "lead-ship", "lead-ticket", "lead-tune", "lead-use-mailbox", "lead-workflow-manual"} {
			response := callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": name})
			body := toolText(t, response)
			if toolIsError(t, response) || strings.TrimSpace(body) == "" || strings.Contains(body, "{{.") {
				t.Fatalf("%s old shim %s: %s", namespace, name, response)
			}
		}
	}
}
