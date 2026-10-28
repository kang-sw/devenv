---
title: Work down the MCP tooling and playbook review follow-up backlog
dropped: 2026-10-28
---
# Work down the MCP tooling and playbook review follow-up backlog

## Background

Review sweeps on `develop` recorded these as `minor/review` commit follow-ups
(`ws/git.followups`), and none was ever resolved. A triage on 2026-10-28 at HEAD
`f400e5fbe` checked each against current code and classified it TEST (coverage
gap, behavior believed correct) or TIDY (duplication, naming, comment/wrap,
structure; no behavior impact); BUG and STALE entries were handled separately.
The creating commit resolves every id below, so this ticket is now the single
owner of the core (Go MCP tooling, playbooks, package tests, manuals, CI) backlog and the release gate no longer lists them.

Each row keeps the original follow-up id; `git log --grep=<id>` finds the
review commit that recorded it and its AI Context. Paths under `mcp/`, `wsmailbox/` and the like are relative to `agents-plugin-tool/internal/`. The Windows replace-retry rows (`anyplace-wifi-shelf`, `taunt-almost-accompany`, `undergrad-stiffly-buddy`) moved to `261028-bug-wsnote-windows-rename-retry`. Line numbers were
re-located at triage time and will drift. `(?)` marks a call the triager was
unsure of (the lower-impact class was chosen).

## Phases

### Phase 1: Work through the core (Go MCP tooling, playbooks, package tests, manuals, CI) backlog

Re-check each row against the code at pickup; drop rows that no longer hold and
say so in the Result. Batch by file or module rather than by row. TEST rows add
tests that fail on the described regression; TIDY rows change no behavior. A
row that turns out to be a real defect is reported, not silently fixed inside a
tidy batch. Splitting this phase further is fine when the backlog is too large
for one worker pass.

#### TEST (22)

| id | current path:line | note |
|---|---|---|
| cannon-pegboard-dill | mcp/playbook_tools.go:1091-1104 | discardPlan cleanup path untested (new today) |
| trapezoid-thinness-sibling | mcp/playbook_render_plan_test.go | no test pins plan-path line after recommended-* lines |
| rebound-awry-that | mcp/config_repo_scope_test.go:746 | assertion fails earlier at workflow.lang; cannot catch allocation-order regression |
| icing-sleeve-mannish | mcp/mailbox_tools_test.go | RuntimeNamespace() entry of hasPeerLeadNote not pinned |
| haven-stump-degrease | agents-plugin/.mcp.json:7, .claude-plugin/plugin.json | no test asserts WS_MCP_CONFIG_MANIFESTS resolves to shipped config-manifest.json (only wsflow .mcp.json is tested) |
| coping-lung-impromptu | mcp/mailbox_tools.go:467 | empty harness/cwd omission untested |
| buckshot-mammal-sloppily | wsmailbox/replyid_test.go:57 | older-than-all-20 insert and lookup_peers 20-cap untested |
| sesame-refresh-monthly | mcp/worktree_remove_test.go | no validation/override test for worktree.nudge_preserved_gb (wsconfig/scope.go:127) |
| decoy-idiom-truce | mcp/worktree_hooks_test.go | no test that a failing hook stops the rest of its list |
| overtly-action-landlord | mcp/server.go:1320 | dispatcher rollback noSubmoduleRecurse untested |
| iguana-surgery-public | mcp/workflow_manual.go:175 | injectWorktreeProfiles no-Git-heading fallback untested |
| catcher-exemplify-ludicrous | mcp/occupied_root_announcement_test.go | no worktree.acquire profile text pinned |
| stipend-unequal-onto | agents-plugin/tests/test_skill_dispatch_contracts.py:36 | RETIRED_SKILL_NAMES still lacks mcp-server-repair, lead-scope-worktree |
| delirium-eldest-sulphate | arm-the-wait.claude.md vs manual.ts MAILBOX_LINE | independently pinned, no coupling test |
| papyrus-sprite-propose | mcp/mailbox_tools_test.go:18 | setupMailboxTestEnv does not clear WS_MAILBOX / WS_MAILBOX_AUTO; TestMailboxInertByDefault fails if ambient set (flaky) |
| evidence-parakeet-thinness | wsmailbox/replyid_test.go:45 | tautological ReplaceAll(DeriveFull) clause; no many-key IsValidReplyID sweep |
| doubling-splice-dinginess | mcp/session_state_test.go:3523 | agenda.set merge lacks size-note and merge:null cases |
| sanding-clapped-jubilance | agents-plugin/tests/test_claude_hooks_manifest.py:123 | empty/malformed payload branch untested |
| distort-trolling-limit | mcp/config_manifest_test.go:282 | config.get role gate partly pinned |
| slush-studied-frame | mcp/git_followups_test.go | unknown-key gate, malformed shapes, resolves-history warning untested (only 3 tests) |
| nuttiness-budding-another | shell/statusline.sh:315 | no statusline test harness |
| kinship-cruelty-alike | .github/workflows/ws-mcp-ci.yml:1 | CI still Go-only (no pytest/pi/mod jobs) |

#### TIDY (15)

| id | current path:line | note |
|---|---|---|
| bronze-radiance-starry | lead-workflow-manual.md:134 | restates tool descriptions |
| yield-overtime-crazed | mcp/playbook_tools.go:1091 | empty month dir left after failed render; cosmetic |
| suffrage-promptly-crisping | lead-delegate.md:60 | root_override guidance gap for host-created isolation (?) prose-only |
| marital-crafter-concerned | mailbox_tools.go:285, wsmailbox/replyid.go:61 | two unlinked constants both 20 |
| reliable-scariness-strangely | lead-use-mailbox.md:58 | "every address" omits 20-most-recent trim |
| strife-trickle-librarian | mcp/worktree_profile.go:139 | strict DisallowUnknownFields also gates workflow_manual/acquire; only triggers with a profile field from a newer ws, none exists today; tracked as open question in ticket 261020-bug-worktree-profiles-review-followups (?) |
| jumble-encode-unsliced | mcp/server.go:1306,1367,1405 | dispatchers drop partial result on error (release/remove/acquire); only earlier successful hook provenance lost from error text, worktree state is unaffected (?) |
| rendition-lying-kinship | mcp/server.go:1369,1405 | duplicated lookup closure |
| rising-ladle-roundworm | ai-docs/manuals/wsflow-mirroring.md:312 | carve-out doc omission |
| unsworn-unseated-unsaid | mcp/mailbox_recv_concurrency_test.go:145 | duplicated JSON-RPC framing |
| hamstring-headset-chapter | ai-docs/ws-notes gotcha.pi-tui-dual-package | note wording drift |
| cruelness-esophagus-reacquire | lead-ticket.md:51,128,142 | wrap cosmetic |
| envious-exorcist-unwarlike | mcp/playbook_tools.go:257 | helper placement; manifest env pins missing |
| matchbox-elevating-qualify | mcp/mailbox_runtime.go:374 | one-line wrapper |
| awaken-spinout-duh | mcp/git_followups.go:15, server.go:1501 | `followups: []` from non-root key refused; deliberately pinned by git_followups_test.go:79-83 with a clear message and nothing committed, so a design choice rather than a defect (?) (could be BUG if workers routinely send []) |


## Resolution (2026-10-28)

Dropped by user decision the same day. The canonical design lets minor review follow-ups age out of later release-gate ranges (lead-ship Release gate step 1), and lead-review now re-grades a hittable-defect Minor to Important before the verdict (5a3e14a2a), so the remaining TEST/TIDY rows are left to age out rather than kept as a standing backlog. The Windows replace-retry rows live on in 261028-bug-wsnote-windows-rename-retry. The rows stay here as a reference.
