---
title: "Dashboard wide view: file explorer in the left column, file preview in a dedicated right-hand pane"
related:
  261110-epic-ws-dashboard-agent-host: file preview is in the epic scope; this ticket is standalone
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 921bf96d73b6af2e
sage-review-completeness-reviewed: 921bf96d73b6af2e
completed: 2026-11-17
---

# Dashboard wide view: file explorer in the left column, file preview in a dedicated right-hand pane

## Background

In the live dashboard's split layouts (left conversation/files column beside
the board), the board toolbar's folder button opens a separate explorer
instance (`#toolbar-files`) as a sheet that drops down over the top of the
board area, and a file single-click normally opens a transient preview card on the
board; already-open or pinned files are reused, and binary files offer row actions
instead (agents-plugin-tool/internal/wsdashboard/assets/files.js#L866-L902,
agents-plugin-tool/internal/wsdashboard/assets/preview.js#L338-L362). The owner wants the wide view to use the space differently: the
explorer belongs in the left column, and a clicked file opens as a large
preview in the right-hand area (owner sketch, 2026-11-17).

Facts from investigation (2026-11-17, read-only):

- The left column already has a `대화 | 파일` tab pair; the `파일` tab shows a
  file explorer at `#files` built by the same `WsFiles.create()` as the
  toolbar explorer, but as a separate instance with independent navigation,
  selection and persisted state (search `toolbar-files` and `WsFiles.create`
  in `agents-plugin-tool/internal/wsdashboard/assets/`).
- Split layout applies at >=760px when explicit and >=1100px when automatic;
  the tabs layout keeps one column (search the split media rules in
  `dashboard-layout.css`).
- The dashed area right of the board is the empty pin list inside the
  pinned-card column `#bpcol` (the column shows at >=1600px with pins and
  >=2200px even when empty; the empty list gets the dashed border at >=2200px:
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css#L77-L103`).
- Previews today: explorer single-click normally opens a replaceable transient
  preview card; double-click/Enter makes a durable card; binary files offer row
  actions and already-open or pinned cards are reused
  (`agents-plugin-tool/internal/wsdashboard/assets/files.js#L866-L902`,
  `agents-plugin-tool/internal/wsdashboard/assets/preview.js#L338-L362`); renderers cover images,
  audio/video, PDF, sandboxed HTML/SVG, Markdown, diffs and text/code
  (`assets/preview.js`, `fs_routes.go`).

Approved visual reference: `ai-docs/assets/mockups/261117/wide-explorer/explorer-wide.html`,
state `#a` (state `#before` shows today's behavior for comparison; state `#b`
is the rejected alternative).

## Decisions

1. **Folder button in split layouts switches the left column to its `파일`
   tab — (09)=ok.** In every split layout the board toolbar's folder button
   activates the left column's existing `파일` tab and focuses its search
   field instead of opening the `#toolbar-files` sheet. The tabs layout keeps
   today's toolbar sheet. The toolbar instance's own search text and expanded
   folders therefore do not carry into the left explorer ((24)=ok).
   Rejected: moving the toolbar explorer instance into the left column, or
   building a third explorer — the left column already hosts the same
   explorer implementation.
2. **Dedicated right-hand preview pane — (11)=a.** A single-click on a file in
   the left explorer opens a dedicated preview pane that occupies the board
   area and the pin column while open; the board and pinned cards are hidden
   and return unchanged when the pane closes. The pane header shows the file
   name, its path, a `카드로 고정` action and a close button (mockup `#a`).
   Rejected: a dedicated pane that keeps the pin column beside it (owner
   chose to hide it); opening the existing transient preview card at full
   board width at the top of the board (mockup `#b`).
3. **"Wide view" is every split layout — (14)=ok.** The behavior applies
   whenever the left column sits beside the board (>=760px explicit,
   >=1100px automatic). Rejected: a new width breakpoint.
4. **Only the left explorer's single-click targets the pane — (15)=ok.**
   Conversation and board file links keep opening board cards.
   Rejected: routing every file link into the pane.
5. **`카드로 고정` promotes and closes — (16)=ok.** It makes the current file
   a durable board card through the existing keep/pin path and closes the
   pane. Rejected: keeping the pane open beside the new card.
6. **Double-click and Enter unchanged — (17)=ok.** They still make a durable
   card. Rejected: redirecting them to the pane.
   **They close the pane — (37)=a.** A double-click's first click opens the
   pane; when a double-click or Enter in the left explorer then makes the
   durable card while the pane is open, the pane closes so the board shows
   the new card, as `카드로 고정` does (Decision 5). Rejected: keeping the pane
   open with the new card hidden behind it.
7. **Closing — (18)=ok.** The close button and Escape while focus is inside
   the pane close it and restore the board and pin column. Switching to the
   tabs layout while it is open closes it and opens the same file as today's
   transient preview card. Rejected: keeping a pane in the tabs layout.
8. **Existing renderers — (19)=ok.** The pane renders through the existing
   preview renderers. Rejected: pane-specific renderers.
9. **Not persisted — (20)=ok.** The open pane is not restored after a reload,
   like today's transient preview slot.
10. **Binary files and existing cards — (34), auto-confirmed.** Binary files
    keep today's row actions; every previewable file opens in the pane even
    when a durable or pinned card for it exists, and that card stays
    untouched. Rejected: reusing and focusing the existing card, which is
    hidden while the pane is open.
11. **No compatibility surface — (21)=ok.** Frontend assets embedded in the
    daemon only; no MCP tool, `/mcp` hello, store shape, tracker event or
    manifest change, so no tier applies.
12. **Placement — (22)=ok.** Standalone ticket, `related:` to
    `261110-epic-ws-dashboard-agent-host` (file preview is in that epic's
    scope) rather than its child.
13. **Verification — (23)=ok.** One passing check: existing Node contract
    tests locally, Go dashboard tests on remote w5090, and LOOK captures
    compared side by side with mockup `#a`. Rejected: new browser fixtures.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Narrow and tabs layouts behave exactly as today.
- Preserve explorer keyboard navigation, selection modifiers, Enter/Escape
  handling, focus restoration, and the `shown()`/`hidden()` lifecycle of the
  left explorer.
- Do not disturb the pinned-card column's drop-target behavior, persisted
  cards, or transient-preview promotion while the pane is closed.
- The worktree page uses a separate board explorer-card button; leave it
  unchanged.
- Go tests never run locally on this Windows PC; run them on remote w5090
  (see the `remote-build-w5090` clone note recipe).

## Prior Decisions

- Prior-decision search unavailable: wsflow/rationale.query was not exposed in this session; review the relevant history before implementation.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/ includes dashboard-board.js, files.js, preview.js and layout styles |
| scope.surface | cross-module | browser-internal WsFiles and WsPreview integration across frontend assets |
| scope.new_public_symbol | unknown | pane integration interface is not specified |
| scope.new_type_contract | unknown | pane-to-explorer and preview renderer contract is not specified |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/testdata/files-explorer.test.cjs and preview-content.test.cjs; existing Node contracts specified |
| complexity.reuse_points | confirmed | dashboard-board.js creates separate WsFiles instances; files.js delegates to WsPreview; preview.js implements card rendering |
| complexity.side_effect_risk | moderate | changes to layout and preview routing can affect explorer lifecycle, board cards and pin column |
| risk.correctness | high | split transitions, asynchronous preview requests, promotion and existing card reuse must remain consistent |
| risk.fit | moderate | pane must reuse renderers and coexist with independently owned explorers and board layout |
| risk.test | high | existing contract tests do not specify the new pane and the plan excludes new browser fixtures |
| risk.security_or_contract | moderate | reused preview renderers and guarded file routes must retain existing sandbox and root boundaries |

## Phases

### Phase 1: Left-column explorer and dedicated preview pane

Implement Decisions 1-10 in the dashboard frontend assets under
`agents-plugin-tool/internal/wsdashboard/assets/`, matching mockup state `#a`.

Verification: the existing Node contract tests pass locally; the Go
`./internal/wsdashboard/...` suite passes on w5090; LOOK captures of the
result in a split layout at 1920x1080 and 2560x1440 (explorer in the left
column, a Markdown file and an image open in the pane) are compared side by
side with mockup state `#a`, and a tabs-layout capture shows the toolbar sheet
unchanged.

### Result (2c02115ff) - 2026-11-17

Implemented Decisions 1-10 in `2c02115ff`, with R1 corrections in `6a9226e76`.
The split toolbar selects/focuses the existing left explorer; its previewable
single-clicks open a memory-only reading pane across the board and pin area.
Existing renderers, binary row actions, independent explorer state and normal
card links remain intact. Keep/double-click/Enter close the pane before durable
card reveal; closing cancels pending preflight, preserves focus ownership, and
surface transfers fetch fresh sandbox capabilities. Tabs fallback restores the
normal preview path. All 11 locales have the keep label.

Verification:

- Local Node contracts: 22 suites passed, 110 node:test cases plus legacy
  assertions; workspace-picker runs bundled with files-explorer.
- Remote w5090: owner's sys_lock recipe, `go test ./internal/wsdashboard/...`,
  passed after the R1 fixes (272.552s). No local Go tests.
- Independent R1: three Important issues fixed (pending preflight, stale
  capabilities, content-focused closure); fixes-only R2 clean.
- Real harness corrections in `4f2bfd09e`: Windows executable suffix,
  authenticated session readiness and daemon failure-log tail. Its four Node
  tests pass; focused independent review of this additional delta is clean.
- Chrome LOOK: left Files tab/search activation, Markdown/image rendering,
  hidden board/pins, tabs fallback and unchanged toolbar sheet confirmed.
  At 2560px the pane extends from x=919.66 to x=2544 across the hidden pin
  column; neither viewport has horizontal overflow. No real visual gaps found.
- Side-by-side comparisons below: **match of the approved layout contract**.
  Existing production rail/composer/explorer chrome and widths remain; the
  stipulated existing image renderer preserves aspect ratio with checkerboard
  letterboxing rather than stretching the mockup's illustrative gradient.

All capture paths below are under `D:/Temp/wide-explorer/`; references are
renders of `ai-docs/assets/mockups/261117/wide-explorer/explorer-wide.html#a`
at exactly the same viewport and selected file:

| Result capture | Reference capture | Verdict |
|---|---|---|
| `result-1920x1080-markdown.png` | `mockup-1920x1080-markdown.png` | match |
| `result-1920x1080-image.png` | `mockup-1920x1080-image.png` | match |
| `result-2560x1440-markdown.png` | `mockup-2560x1440-markdown.png` | match |
| `result-2560x1440-image.png` | `mockup-2560x1440-image.png` | match |

Tabs capture: `D:/Temp/wide-explorer/result-1920x1080-tabs-sheet.png`.
Four inspected `comparison-<viewport>-<kind>.png` files retain the side-by-side
views. Only these 13 final PNGs remain; isolated daemons and other temporary
artifacts were removed. Captures used current frontend assets and source locale
tables with an existing isolated daemon binary; no shared daemon was replaced.

Forward (one grouped baseline exclusion): four untouched Node fixtures also
fail against base production assets `c79cdf1`: search-palette
(`paletteHistory`), stream-gone (`hiddenRegions`), task_badges (`$`), and
visibility-stream (`window`). No unrelated investigation or fixes.

## Sage Review Round 1 (2026-11-17)

### Design Reviewer — pass

| # | Title | Severity | Resolution |
|---|-------|----------|------------|

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Pane behavior during durable-card creation | important |

## LOOK capture gate recovery (2026-11-17)

Earlier checkpoint `07168d170` left Phase 1 pending LOOK. The lead authorized
bounded continuation without changing the ticket contract. The gate is now
resolved by the Phase 1 Result above; the remaining text records the earlier
handoff and its diagnosis.

Landed: split toolbar activation of the existing left Files tab, a memory-only
pane sharing the existing preview renderers across the board/pin area, durable
promotion and tabs fallback, unchanged binary/default explorer routing, and
localized keep labels. Board/pin geometry is retained while their paint is
hidden; no stored or protocol surface changes.

Verification:

- Local `node D:/Temp/wide-explorer/run-contracts.cjs --exclude-forward`:
  22 suites passed, 110 node:test cases plus legacy assertions; one additional
  workspace-picker fixture runs bundled with files-explorer.
- Remote w5090, using the owner's exact sys_lock sync recipe:
  `go test ./internal/wsdashboard/...` passed after the R1 fixes (272.552s).
  No Go tests ran locally.
- Independent R1 found three Important issues: pending preflight reopening a
  dismissed pane, expired sandbox answers reused on surface transfer, and
  content removal preceding focus restoration. All fixed in `6a9226e76`, with
  regression tests in `2860bee02`. Fixes-only R2: clean.
- `git diff --check c79cdf1..HEAD`: clean. All 11 locale tables contain a valid
  keep label.
- Forward (one grouped baseline exclusion): four untouched Node fixtures also
  fail against base production assets `c79cdf1`: search-palette
  (`paletteHistory`), stream-gone (`hiddenRegions`), task_badges (`$`), and
  visibility-stream (`window`). No unrelated investigation or changes.

LOOK setup exhausted the owner's pass-plus-environmental-retry budget before
any capture was produced. The existing `scripts/board-headless.mjs` isolated
server fails on Windows because its executable path lacks `.exe`. A temporary
copy adapted that suffix and reused an existing binary (no package installs),
but `serve --shared-actions` then failed because its existing fixture gives
policy options to a `technical` item; the current binary rejects that command.
No further capture attempt was made at that handoff; no PNGs or side-by-side
comparison had yet been produced. Those temporary artifacts were removed.

The first authorized retry still timed out. The next bounded diagnosis, with
180-second waits and visible daemon output, showed HTTP 401
`{"error":"login_required"}` on frame readiness: `/health` and `/nudge` are
machine exemptions, but session pages require the localhost cookie. The
minimal tracked-script fix uses the isolated endpoint's existing native cookie
route, then supplies that cookie on readiness requests. Serving and all
required captures subsequently succeeded. No merge performed.
