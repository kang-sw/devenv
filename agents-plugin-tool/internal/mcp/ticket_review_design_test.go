package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// The rendered prompt is the contract boundary for this model-executed check;
// these assertions do not claim to execute a model's contradiction judgment.
func TestTicketDesignReviewContradictionAnchors(t *testing.T) {
	for _, product := range []struct{ pkg, namespace string }{
		{"agents-plugin", "ws"},
		{"agents-plugin-wsflow", "wsflow"},
	} {
		t.Run(product.namespace, func(t *testing.T) {
			root := filepath.Join("..", "..", "..", product.pkg, "rsrc")
			t.Setenv("WS_MCP_NAMESPACE", product.namespace)
			if product.namespace == "wsflow" {
				t.Setenv("WS_MCP_NO_AGENT", "1")
			} else {
				t.Setenv("WS_MCP_NO_AGENT", "0")
			}
			body, _, err := renderPlaybookBody(&Server{}, root, "ticket-reviewer-design", nil, wsconfig.Options{}, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Join(strings.Fields(body), " ")
			for _, want := range []string{
				product.namespace + `/tickets.query(statuses: ["ready"])`,
				"every returned ticket body except the ticket under review",
				"even when no `related:` edge names them",
				"ticket_stem: <parent>, include_done: true, include_dropped: true",
				"`related:` is not an independent contradiction anchor",
				"Do not scan `todo/`, `idea/`, or the whole ticket tree",
				"report the incomplete check",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("rendered review is missing %q", want)
				}
			}
			checklist := strings.Split(strings.Split(body, "## Checklist")[1], "## Heuristics")[0]
			heuristics := strings.Split(strings.Split(body, "## Heuristics")[1], "## Output")[0]
			for name, section := range map[string]string{"checklist": checklist, "severity": heuristics} {
				if !strings.Contains(section, "`ready/`") || !strings.Contains(section, "`parent:`") || strings.Contains(section, "`related:`") {
					t.Errorf("%s must anchor on ready inventory and parent only: %s", name, section)
				}
			}
			if !strings.Contains(checklist, "**Duct-tape detection**") || !strings.Contains(checklist, "**Right-problem check**") {
				t.Error("single-ticket coherence checks were lost")
			}
		})
	}
}
