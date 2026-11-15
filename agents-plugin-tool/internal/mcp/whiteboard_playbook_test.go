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
				"Declare each open topic's work state with `wb:state` and update its agent log line when the state changes, retaining a human current-state summary:",
				"| Discussing | Keep one open ask for the unresolved discussion. |",
				"| Waiting | Keep held items visible and one open ask for the unfinished work that is not being worked. |",
				"| Implementing | Name the responsible executor and ETA when work starts. |",
				"| Awaiting confirmation | Keep one live confirmation check for the finished artifact. |",
				"Keep at most one open ask per topic, counting checks as asks; reuse or patch its item as the work changes.",
				"Close a topic only after the user's final answer",
				"These ask and summary obligations remain lead judgment alongside the machine state.",
				"`wb:add`, `wb:state`, and `wb:close` inside it",
				"| `wb:state` | `value`, `owner?`, `eta_minutes?` | none |",
				"`wb:state` is an empty command inside `wb:topic`, with `value` one of `discussing`, `waiting`, `implementing`, or `awaiting-confirmation`.",
				"Entering `implementing` requires `owner` (the responsible executor) and `eta_minutes`",
				"a positive decimal integer from 1 through 153722867",
				"the cumulative wall-clock budget in minutes from the original start",
				"Other states accept neither attribute.",
				"The topic must already be known and open",
				"`wb:state` alone does neither.",
				"Repeating the current declaration is a no-op",
				"owner and ETA must match, so repeats do not reset the start",
				"To change executor, exit implementing and re-enter with the new owner and ETA.",
				"ETA changes within a run are not available.",
				"Closing clears the declared state and ends its run; reopening restores neither state nor run.",
				"optional derived `state`",
				"`run` with `deadline_at`",
				"Legacy topics without a declaration omit it.",
				"Unknown state omits `state` and reports `state_diagnostic` with `entry` and code `malformed` or `unsupported`",
				"These are metadata, not selectors",
				"response-only structural warnings for discussing topics with zero open items and waiting topics when this board has no open implementing topic",
				"Every open item, including a check, counts as an ask.",
				"These warnings neither refuse the update nor create asks; they are not timed notifications.",
				"Approval of a concrete deliverable is execution authority: start the approved build or change rather than ask permission again.",
				"A request to build or change something is work to execute, not a technical item or confirmation check that substitutes for doing it.",
				"A real choice remains a policy ask; a finished artifact awaiting the user's verification gets a confirmation check.",
				"It is not another request for permission to implement an approved deliverable.",
				"Keep asks in items: every question or decision request to the user is an item, a new `wb:add` or a `wb:patch` re-ask of an existing one, since only an item collects the user's answer.",
				"A `wb:log` line, panel, or current content may cite the open item as `{{12}}` but never phrases the ask itself.",
				"A finished-artifact confirmation is `class=\"check\"` (or `policy` for a real choice).",
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
				// ETA extension and scheduled delivery belong to the later phase.
				"wb:eta", "board_notice", "scheduler", "30-minute", "hourly", "timed nudges",
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
