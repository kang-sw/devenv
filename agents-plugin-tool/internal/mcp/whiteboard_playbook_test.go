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
				"Declare each open topic's work state in its current-state summary",
				"| Discussing | Keep one open ask for the unresolved discussion. |",
				"| Waiting | Keep held items visible and one open ask for the unfinished work that is not being worked. |",
				"| Implementing | Name the responsible executor and ETA when work starts. |",
				"| Awaiting confirmation | Keep one live confirmation check for the finished artifact. |",
				"Keep at most one open ask per topic, counting checks as asks; reuse or patch its item as the work changes.",
				"Close a topic only after the user's final answer",
				"These are lead obligations, expressed in ordinary board text.",
				"Approval of a concrete deliverable is execution authority: start the approved build or change rather than ask permission again.",
				"A request to build or change something is work to execute, not a technical item or confirmation check that substitutes for doing it.",
				"A real choice remains a policy ask; a finished artifact awaiting the user's verification gets a confirmation check.",
				"It is not another request for permission to implement an approved deliverable.",
				"Keep asks in items: a finished-artifact confirmation is `wb:add` with `class=\"check\"` (or `policy` for a real choice), not a panel or log line.",
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
			for _, gone := range []string{
				"<wb:recap>", "Topic recap:", "the board clears with the session key", "{{.McpNamespace}}", "{{.SkillNamespace}}",
				"Linked tickets:", "`tickets?`", "tickets=", "OK this draft and I'll proceed", "A topic can have content or entries without items.",
				// State declarations are prose until the additive commands land.
				"wb:state", "wb:eta",
			} {
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
