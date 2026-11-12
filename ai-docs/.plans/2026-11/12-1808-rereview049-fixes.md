# 0.49 re-review repairs, batch R: LAN leading-slash text, manual, B4 test, label toggles across items

## Outcome

Fix the lead-owned Important findings R1-R4 of the 0.49 pre-ship re-review
(range 62304312b..be7860ad8, stamp concern 7d9d27305). Findings file:
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/339c1c41-01-findings.md`;
read its R section first. Context: the earlier repair batch A (plan
`ai-docs/.plans/2026-11/12-1745-sweep049-go-docs.md`, commit 317f6d88c) added
the LAN slash-command gate per the user's decision (whiteboard (481) a: slash
commands only from a browser on this machine, and only listed names).

1. **R1:** in `agents-plugin-tool/internal/wsdashboard/inbox_routes.go`
   `messageWrite`, refuse a message from a non-local viewer (`local == false`)
   whose text, trimmed of leading whitespace, starts with `/`: 403 with a
   message in the same family as the command refusal (for example "a message
   starting with / runs as a slash command; slash commands run only from a
   browser on this machine"). Local viewers are unchanged. Reason: hosted
   backends pass a plain message verbatim and Pi / hosted Claude run a
   leading-slash line as a command, so the A1 gate is bypassable by plain
   text. Lead's technical call: refuse rather than escape (escaping would
   change the text the user typed). Extend
   `TestCommandIsHeldOnlyFromThisMachineAndOnlyWhenListed` (or add a sibling)
   for: LAN `/compact` text refused, LAN `  /x` refused, LAN text with a
   slash later in the line held, local `/compact` text held. Check whether
   the page's composer should tell a LAN user before sending (only if there
   is an existing simple refusal display path; the server's status text
   shown by the existing failure toast is enough).
2. **R2:** `ai-docs/manuals/ws-mcp.md` ~181-183, ~243-248, ~713-715: document
   the command entry gate (local viewer only, 403; listed name only, 400),
   the `cmds` region rendered for a local viewer only, R1's refusal of a LAN
   message starting with `/`, and in the LAN viewer paragraph that a LAN
   viewer may send messages but not slash commands.
3. **R3:** `agents-plugin-tool/internal/wsdashboard/pincol_width_test.go`:
   presence check for B4's `.bpzone` width clamp and the vertical label rule
   below 1880px (`dashboard.css` ~1485, ~1488), beside
   `TestPinnedColumnWidensThePage`.
4. **R4:** `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
   ~4924-4934 `labelWrite`/`labelFlush`: toggles queued on item X must not be
   dropped when the user toggles a label on item Y before X's write returns
   (today `LW` is replaced and X's `then` returns early). Keep the pending
   state per item (or flush X's queued set before switching) so X's last
   pick is written; add a node case to `TestTrackerLabelStepQuickToggles`.
5. Minor: add a "test support only" doc line to
   `agents-plugin-tool/internal/wswhiteboard/count_chips.go`
   `StaticCountChips`.

## Boundaries

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`.
  Tier T0 (a dashboard route refusing more; asset/test/doc changes); state it
  in the AI Context.
- Do not touch `internal/wsagent/` or `agents-plugin-pi/` (peer-owned).
- No refactors beyond each item. Never run go build/test/vet locally.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/mcp -count=1`
  from the worktree root: green; confirm with `-v` on the touched node tests
  that they ran (not skipped).

## Return

Commit(s) on `impl/develop/rereview049-fixes` in
`/home/swkang/.ws-worktrees/devenv/aim-hydration-granite`, this plan file in
the first commit. Do not merge or push. Report commits, what changed per
item, and test output.
