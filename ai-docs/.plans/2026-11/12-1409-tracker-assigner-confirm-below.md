# Tracker: an assignment to someone below the writer always needs the assigner's confirmation

## Goal

User policy change 2026-11-12 (board topic `tracker-assign-policy`; (399) a,
(396)-(398) acknowledged): work a superior assigns to a subordinate cannot be
wrapped up (done / dropped) by the subordinate; the subordinate can only send
it to review ("검토 중"), and the assigner confirms. Today this is already the
default (completion=assigner) for such assignments, but the writer can still
pick completion=assignee. The decision is to ENFORCE it.

## Decisions (settled; do not reopen)

- Rule: an `item.assign` whose target is below the writer in the org
  (`state.below(writer, target)`, `agents-plugin-tool/internal/wstracker/state.go:552`)
  must have completion=assigner for that target.
- Store (`internal/wstracker/check.go`, `TypeItemAssign` around 514 / 733):
  refuse completion=assignee when any target is below the writer, with a
  clear reason in the existing reason style (add the reason to
  `internal/wstracker/testdata/reasons.json` and regenerate goldens with
  `testdata/gen-golden.js`). Write-time only: `check()` runs only at write;
  fold is unchanged and existing records stay as they are (no migration, no
  fold flag).
- Mirror the check in the reference engine
  `ai-docs/ref/tracker-mockup/engine.js`.
- MCP (`internal/mcp/tracker_tools.go` `completionClasses`, ~891-1026): for
  targets below the writer, default to assigner as today, and refuse an
  explicit completion=assignee for them with the same reason (rather than
  silently overriding a value the caller set). Update the tool description
  text that explains completion so an agent knows the rule.
- Dashboard picker (`internal/wsdashboard/assets/tracker.js`, the completion
  radio ~3942-4072 `asgCompl` / `ap-cm`): when any picked target is below
  the writer, do not offer "담당자가 완료" (assignee): hide that radio option
  (or disable it with a tooltip if hiding breaks the layout; say which) and
  send completion=assigner. Targets not below the writer keep today's choice.
  Quick-add / other write paths that send completion=assignee
  (e.g. tracker.js ~4498 self-assign) must not trip the rule: self-assignment
  is not "below", keep it working.
- Scope: superior -> subordinate only. A superior can still wrap up a
  subordinate's work (checkStatusOf, unchanged). The assignee can still set
  the other statuses (in progress, blocked, in review) as today.
- Tracker playbook text (all three rsrc copies:
  `agents-plugin/rsrc/tracker/tracker.md`, `agents-plugin-wsflow/...`,
  `agents-plugin-pi/...`) gets one sentence on the rule where completion is
  described, with each `rsrc/manifest.json` sha256 updated (as the I4 fix
  a355f5998 did).
- Compatibility tier T0: a write-time check only; no store shape or event
  change, fold unchanged. An older ws writing completion=assignee to a
  subordinate still folds the same everywhere.

## Scope and verification

- Go tests: store refusal (below target + assignee refused; below + assigner
  ok; peer/above/self + assignee ok; mixed targets refused), MCP refusal and
  default, existing records with completion=assignee to a subordinate still
  fold and still let the assignee wrap up (fold unchanged).
- reasons.json cases + `node testdata/gen-golden.js` reports all cases agree.
- Node test (tracker_quickadd_test.go harness) for the picker: assignee
  option absent when a below target is picked, present otherwise; the sent
  completion.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  the assign picker for a subordinate vs a peer, light and dark, shots into
  scratchpad/tracker-policy/.

## Constraints

- Read `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`
  before editing the files they cover.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wstracker/... ./internal/wstrackerhost/... ./internal/mcp/... ./internal/wsdashboard/... ./internal/wsrsrc/...`;
  never run go build/test/vet locally. `internal/mcp`
  TestDashboardReadPageServesTheBoard already fails on develop; ignore it.
- Commit on branch `delegate/tracker-assigner-confirm-below` in this
  worktree; do not merge or push.
