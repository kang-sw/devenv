package mcp

import (
	"strings"
	"testing"
)

// Commands and Query are the opaque string/JSON grammar sources the tools
// serve on refusal. Check the shipped guidance in both product namespaces,
// not a second hand-maintained copy of either parser's validation matrix.
func TestWhiteboardPlaybookStreamGuidanceProductModes(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
			} else {
				t.Setenv(envNoAgent, "")
			}
			f := newWhiteboardFixture(t)
			body := toolText(t, f.call(t, "playbook.read", map[string]any{"name": "whiteboard"}))
			flat := collapseSpace(body)
			for _, want := range []string{
				namespace + "/whiteboard.query",
				namespace + ":lead-ticket",
				"The latest agent line supplies that summary; later server events do not replace it.",
				"| `wb:panel` | `recap`, `id?` | optional payload; `recap` must be nonempty |",
				"There is no `panel` attribute on `wb:edit`",
				"Split content-heavy material into several (sub)panels rather than one huge panel",
				"`wb:recap` is removed without an alias, and `wb:append` is refused",
				"Options, recommendations, `wb:why`, links, and check answers remain item data",
				"default discovery is metadata",
				"`body: true` explicitly retrieves current content",
				"`panel: \"p7\"`",
				"`panel: \"e47\"`",
				"`entry: \"e47\"`",
				"Filters apply before the limit",
				"Optional `entry=\"eN\"` identifies the selected immutable panel version or agent/system log entry",
				"Existing topic-only and topic/item mentions remain valid",
				"{{p3}}",
				"Authoritative topic folders remain after the session key expires",
			} {
				if !strings.Contains(flat, want) {
					t.Fatalf("%s guidance lacks %q", namespace, want)
				}
			}
			for _, gone := range []string{"<wb:recap>", "Topic recap:", "the board clears with the session key", "{{.McpNamespace}}", "{{.SkillNamespace}}"} {
				if strings.Contains(body, gone) {
					t.Fatalf("%s guidance retains %q", namespace, gone)
				}
			}
			if namespace == "wsflow" && (strings.Contains(body, "`ws/") || strings.Contains(body, "`ws:")) {
				t.Fatal("wsflow guidance leaked the ws namespace")
			}
		})
	}
}
