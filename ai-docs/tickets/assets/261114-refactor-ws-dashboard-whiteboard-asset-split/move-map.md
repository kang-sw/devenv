# Static asset extraction map

Status: **UNVERIFIED STATIC PREPARATION**, not a Phase 1 Result or acceptance.
Actual before/source pin: `aac39c69f381a82677f0ee6857e8f8fc5bc89322`.
MAIN repinned the comparison after landing UI-C2/UI-C3 and started its merge
into static preparation `961c8e3bf1a243b602e9c541b101747a6df8ea26`.
The earlier `d0ad8a9f1` map is historical, not the next comparison baseline.
Every range below is inclusive at the new exact source pin. Native Git source
reads locate the headings/symbols; no executable checks or byte comparisons ran.

## Dashboard JavaScript

Paths are relative to `agents-plugin-tool/internal/wsdashboard/assets/`.
The table is the assembly order in `pages.go:dashboardScriptFiles`.
New files contain contiguous source fragments, not independent scripts.
New line = old line minus range start plus 1. No wrappers, separators,
renaming, hoisting changes or new browser API are intended.

| Original `dashboard.js` lines | Destination | Recognizable groups |
|---|---|---|
| 1–544 | `dashboard.js` | Original IIFE opener; storage/time/disclosure/scroll state; layout, language/theme and answer preservation |
| 545–1309 | `dashboard-board.js` | `swap`, `morphBoard`, board input snapshots, card mount, frame graft/reuse, topic motion and `boardKeep` |
| 1310–1498 | `dashboard-conversation.js` | Entry merge/eviction, conversation history, mermaid and `loadClosed` |
| 1499–2873 | `dashboard-composer.js` | Answer/web chips, images, mentions, slash commands, draft/send handling and task badges |
| 2874–3297 | `dashboard-actions.js` | Delegated clicks, link actions, session label and context menu |
| 3298–3762 | `dashboard-drawer.js` | Shared drawer height, drag, fold and sheet controls |
| 3763–4190 | `dashboard-frames.js` | Mount/bands, frame messages/identity, `boardJump`, history hooks and image relay |
| 4191–5229 | `dashboard-git.js` | Pooled worktrees, holders, Git actions, graph, commit window and diff |
| 5230–5943 | `dashboard-tracker.js` | Tracker adapter, palette helpers including `palettePick`, support chat |
| 5944–6151 | `dashboard-selection.js` | Selection/mention popover |
| 6152–6445 | `dashboard-stream.js` | Tab label, stream connection, held/hidden regions |
| 6446–7119 | `dashboard-rail.js` | Rail state, menu entry points and drag/reorder |
| 7120–8009 | `dashboard-rail-popovers.js` | Workspace/worktree popovers, row menu, worktree delete |
| 8010–8630 | `dashboard-leads.js` | Hosted sessions, `rpLeadMake`, modal submission/completion and recovery |
| 8631–9236 | `dashboard-lifecycle.js` | Approval notifications, per-session drafts, iframe shell and original final initialization/IIFE close |

State-owning blocks remain contiguous even when large: composer chips/images/mentions,
board morph/grafts/keep, and hosted-lead modal lifecycle. Splitting the browser
execution into separate scripts would change closure/hoisting/early-return semantics;
there is still exactly one assembled script in the original template position.

## Stylesheets

Dashboard paths are relative to `agents-plugin-tool/internal/wsdashboard/assets/`,
in `pages.go:dashboardStyleFiles` order.

| Original `dashboard.css` lines | Destination | Groups |
|---|---|---|
| 1–295 | `dashboard.css` | Palette, header, phases, agent tree, peers |
| 296–600 | `dashboard-conversation.css` | Conversation, composer, mentions, status, tabs |
| 601–844 | `dashboard-board.css` | Cards, overview, preview, terminal, web |
| 845–1008 | `dashboard-layout.css` | Session layouts, Files pane, frame graft |
| 1009–1420 | `dashboard-drawer.css` | Drawer, worktrees, holders, Git graph/diff |
| 1421–1551 | `dashboard-menus.css` | Tracker, index, link actions, toast, chat |
| 1552–2074 | `dashboard-rail.css` | Session rail, pinned column, popovers, recovery |
| 2075–2116 | `dashboard-shell.css` | Iframe shell and shared layers |

Whiteboard paths are relative to `agents-plugin-tool/internal/wswhiteboard/assets/`,
in `render.go:pageStyleFiles` order.

| Original `page.css` lines | Destination | Groups |
|---|---|---|
| 1–150 | `page.css` | Base palette, controls, layout |
| 151–407 | `page-items.css` | Topics, item rows, options, checks |
| 408–605 | `page-stream.css` | Recaps, linked-ticket styling, stream, panel headers, document bodies |
| 606–697 | `page-panels.css` | Side panels, icons, reply bar |
| 698–805 | `page-sticky.css` | Pills, sticky rail, banner, viewer |

The existing whiteboard JavaScript files and registrations are not split.
Relative to the new before pin they stay untouched: the landed UI-C2/UI-C3
`stream.js` is retained, `rail.js` retains sticky ownership, and `form.js` retains
board binding. There is no `page.js`. Git's incoming source diff contains no
CSS or harness changes; their heading coordinates above/below remain current.

## Assembly and readers

- `wsdashboard/pages.go`: embed the new `dashboard-*.js/css` files; `readDashboardSource`
  reads the ordered fragments without separators through the existing embedded/dev
  reader. Existing `get("dashboard.js")` and `get("dashboard.css")` call sites,
  template fields, memo/usage suffixes and asset identity calculation remain.
- `wswhiteboard/render.go`: embed `page-*.css`; `readPageSource` assembles `page.css`
  for standalone, dashboard `PageAssets`, and hashed topic-body frame CSS.
  Existing whiteboard script order and frame suffixes remain.
- Neither template changes. Whiteboard CSS still precedes dashboard CSS; script
  initialization, trusted frame dispatch, sandboxing and browser storage have
  no intended behavior change.
- `asset_source_test.go` in each package provides whole-source readers and adds
  order/no-separator/missing-fragment contracts. Existing source assertions use
  these readers, including `mustAsset` and icon/CSS collision scans.
- Existing dev-assets tests gain fragment edits/missing-source checks; dashboard
  asset identity tests cover every JS/CSS fragment and whiteboard CSS fragments.
- `wsdashboard/testdata/asset-source.cjs` reads the explicit Go string lists;
  standalone CJS/browser fixtures and the harness baseline CSS substitution use
  this same assembled-source reader. Historical archived acceptance scripts in
  `ai-docs/tickets/assets/261113-feat-ws-whiteboard-board-frontend-performance/`
  remain immutable evidence, not current acceptance entry points.

## Harness

`node scripts/board-headless.mjs <scenario> ...` and its `SCENARIOS` registrations
remain. The entry point retains fixture serving, page helpers, browser registry,
cleanup, keep/observer/bands/pill/history scenarios and command parsing.
Factories only bind the same entry-point helpers; browser operations still occur
inside the original scenario functions, not at import/construction time.

| Original `scripts/board-headless.mjs` lines | Destination | New source-body lines |
|---|---|---|
| 1040–1222 | `scripts/board-headless/toolbar-overview.mjs` | 7–189: `OVERVIEW_PAGE`, `qa`, readiness, navigation, capture, fold/restore helpers |
| 1223–1516 | `scripts/board-headless/shared-actions.mjs` | 5–298: `scenarioSharedControls`, `scenarioSharedActions` |
| 1517–2563 | `scripts/board-headless/toolbar-overview.mjs` | 190–1236: Web containment, compact toolbar, recap jitter, overview/focus/final/motion |

Only imports/factory bindings surround the moved source bodies. Existing browser
callbacks, assertions, predicates, historical pins and known-baseline classification
remain verbatim. `split.test.mjs` adds browser-free factory/registration/dependency
contracts, not a substitute for browser acceptance.

## Landed and pending peer patch landing points

These mappings are static source evidence, not executable acceptance.

- **777 remains unmerged**: source `dashboard.js:264` comment remains
  `dashboard.js:264`; source `draftStore` 8731 → `dashboard-lifecycle.js:101`
  (the former d0ad hunk 8726 is now source 8728 → new 98). The actual before pin
  still lacks `mode !== "history"`. No 777 change is imported.
- **UI-C3 is now landed in the exact before pin**:
  `boardJump` 3985 → `dashboard-frames.js:223`; `palettePick` 5554 →
  `dashboard-tracker.js:325`. Conflict resolution retains the opening 544-line
  fragment and transplants the two incoming changes into these owner fragments,
  rather than appending a second monolith. `boardJump` forwards the existing
  `WsCards.unfoldTopic` owner to `wbHistory.loadTarget` and invokes it again only
  after fallback ownership validation. `palettePick` calls it after Overview's
  handoff and before tab/disclosure/focus/geometry work.
- `cards.js:482` retains the landed `unfoldTopic` implementation/export, with
  `foldUnit/topicUnit/originTopic` ownership unchanged. `stream.js:331/359`
  retains `materialize/loadTarget(beforeGeometry)`: known live targets reveal
  before return, deferred topics before `init`, known paged histories before
  `loadOne`; callback-free restore remains callback-free.
- **UI-C2 is now landed in the exact before pin**: `stream.js:41` retains
  `boundaryRow` and historical chronological-cut resolution, pending Older
  cancellation and historical successor materialization during restore. The
  split does not rewrite those paths.
- All landed regression assertions remain in `cards_test.go`,
  `whiteboard_bridge_test.go`, `testdata/search-palette.test.cjs`, and
  `wswhiteboard/stream_script_test.go`. Auto-merged whole-dashboard readers still
  use `dashboardTestAsset`/`readAssetSource`; the new cards test correctly reads
  the unsplit `cards.js`. No raw whole-dashboard reader was introduced.
- **UI-T5**, peer-reported `7e3abb6e0d84f71da292a83f2d89aec7fe9a3c90`
  (`fix/review794-c2-t5`, also contains unrelated D-C2 launcher work; not imported):
  baseline `bodyShape` ticket selector and initial shape predicate
  (old 1231/1234) → `shared-actions.mjs:13/16`; fold `chrome` (old 1284) → 66;
  fold-text label (old 1333) → 115; renderer-region predicate (old 1336) → 118.
  These still require the old `.ttix`/`.wbtk` markup. The independent hidden-slot
  fix is not applied; peer-reported older coordinates differ from this pinned
  source, so use these recognizable symbols for the later patch.
- Hosted-modal completion/generation work maps to `dashboard-leads.js`, including
  `rpLeadMake` at 120. Root must preview semantic interactions and allocate
  affected verification before integrating peer patches.

## Pending

Independent static lite review 8092 covered `b8989519a..961c8e3bf` and reported
three Important recipe findings, no Critical. This reconciliation addresses
those findings statically in the recipe; no re-review is requested under lite's
one-pass allocation, and no executable preservation claim follows.

All tests, syntax/byte checks, builds, browser captures, Phase 1 Result and closure
remain pending the split's own MAIN START. H's slot grants none of that authority.
The combined browser recipe remains **NOT START-ready** for the documented real
supervision/fixture gaps. The UI-start pause and deployment remain MAIN-owned.
See [the one revised recipe](verification-recipe.md).
