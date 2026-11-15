---
title: "Deliver the reusable stateful file explorer card"
parent: 261114-epic-ws-dashboard-file-explorer
blocked-by: 261114-refactor-ws-dashboard-file-explorer-instance-isolation
related:
  261114-refactor-ws-dashboard-file-explorer-instance-isolation: "explicit prerequisite: first technical slice that isolates reusable explorer instances"
  261110-feat-ws-dashboard-file-preview: "existing read-only Files-tab and preview behavior"
  261110-feat-ws-dashboard-terminal: "existing shared board-card framework context"
  261114-refactor-ws-dashboard-whiteboard-asset-split: "prerequisite: implementation starts after this split lands (it moves dashboard.js and dashboard.css)"
  261114-feat-ws-dashboard-worktree-view: "consumer; shares the session-less browsing anchor with Phase 4"
  261115-feat-ws-dashboard-local-browse-anchor: "prerequisite of Phase 4 only: the host anchor and root discovery the picker browses through"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 25d26d143ec11825
sage-review-completeness-reviewed: 25d26d143ec11825
completed: 2026-11-15
---

# Deliver the reusable stateful file explorer card

## Background

The dashboard needs an owner-approved reusable file explorer card rather than
one singleton Files surface. This first user-facing slice builds on isolated
instances and captures the approved common card behavior. The epic continues
to represent the other approved explorer scope without assigning delivery
order here.

The approved tracked visual reference is [mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html).
Owner visual check 759 is APPROVED with the new design axis. Before ready
promotion, the approved tracked reference must be linked and verification must
require a side-by-side result/reference comparison; implementation acceptance
performs that comparison.

## Decisions

- The first user-facing delivery includes internal tabs with middle-click
  opening an inner background tab, details/list/icon/gallery view modes,
  per-window filename and content search with split search targeting the active
  pane, float and fullscreen presentation, and reuse from the conversation
  Files tab.
- The explorer remains read-only. Its operations are opening and previewing,
  searching, copying a path, and sending a path reference to chat; it does not
  modify the filesystem.
- Each explorer card stores its own browser state in browser storage. This is
  additive T0 browser behavior and does not create daemon or shared-store
  authority. The tracked visual asset and control-attachment UI are additive
  T0 content; an additive endpoint is T0 only when necessary. No existing root
  or API semantic rewrite is authorized.
- Code syntax highlighting and PDF preview are required in this first
  user-facing delivery.
- **PDF preview: browser-native viewer over the existing guarded raw path.**
  `/fs/raw` additionally serves `application/pdf` behind the same
  `browseRoot`/`openContained` guards, with a PDF-specific Content Security
  Policy that permits the browser's built-in viewer (the image `sandbox` policy
  blocks it); the card shows it in a same-origin frame. When the viewer is
  unavailable, the card falls back to a download/open link rather than a blank
  frame. Gallery mode shows a PDF as a file-type tile, not a rendered
  thumbnail. Compatibility: T0 (additive content type on an existing route;
  image and video behavior unchanged). Rejected: vendoring pdf.js - a new
  dependency for uniform rendering and thumbnails, not worth the approval cost
  when the native viewer covers preview.
- **Code highlighting: server-side chroma through an optional preview field.**
  The `/fs/preview` text answer gains an optional highlighted-lines field
  produced by the existing chroma path that already highlights diffs
  (`highlightLines` in `gitdiff.go`), under its 256 KiB cap; above the cap, or
  for an unknown language, the preview stays plain text. The explorer colours
  it with the existing `--hl-*` tokens, so code and diff look the same. Old
  clients ignore the field. Compatibility: T0 (additive optional answer field).
  Rejected: a vendored client-side highlighter - a new dependency duplicating
  what chroma already does.
- **Explorer board card and its entry point (886=a).** Register an explorer
  board-card kind beside the existing preview, terminal and web kinds. The
  conversation Files tab header and the toolbar-sheet header each gain an
  "Open as card" action that opens a new explorer card at that instance's
  current location. Multiple explorer cards may coexist. Each card's browser
  state is keyed by its card ID and removed when the card closes. The toolbar
  Files button keeps its current sheet behavior. Rejected: the toolbar Files
  button opening a new card per click with the sheet as that card's fullscreen
  state (repurposes an existing entry point), and a single toolbar-toggled
  card (no multiplicity, contradicts per-card state and frame 01's two cards).
- **Four sequential phases, not one.** P1 card core, P2 search, P3 code
  highlighting and PDF preview, P4 workspace-picker consumer. Rejected: one
  phase - fact population graded it high on every risk row, and the picker's
  open mechanism must not block the card core.
- **Search scope is the active pane (761=a).** Each window searches its own
  active pane/inner tab. This ticket delivers single-pane search that honors
  the rule; the later split dual-pane child inherits it and owns the second
  pane.
- **The workspace picker browses through the host anchor (894=a).** The
  picker uses the daemon-owned host anchor and the server-side root discovery
  delivered by `261115-feat-ws-dashboard-local-browse-anchor` (local viewer
  only), which also serves the worktree page. P4 starts only after that
  ticket lands. Rejected: a session-only picker reusing the page's reply-id
  (no home/volume shortcuts and unusable from the rail popover), and a
  picker-specific route decided ahead of the worktree page.
- Existing normal Files guarded roots remain the access boundary. The
  separately approved workspace picker is local-viewer-only, directories-only,
  may browse home and local-volume roots labeled with the daemon host, and is
  not blanket file-content root widening. Do not add new symlink permissions.
- The reusable explorer core exposes selection/navigation events and a
  swappable bottom attachment slot. Browse cards offer `Send to session` and
  `Copy path`. The workspace picker uses the same core, hides files, shows
  directories only, and has a Select/Cancel row with hidden entries
  translucent. No separate picker UI is authorized.
- Technical event names and store fields remain the implementer's technical
  choice within the boundaries above; no renderer library is added.
- The approved tracked visual reference is an acceptance authority. Before
  ready promotion, it must remain linked and verification must require a
  side-by-side result/reference comparison; implementation acceptance performs
  that comparison.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Reuse the isolated explorer component for the conversation Files tab rather
  than creating a second Files-tab-specific contract.
- Do not add filesystem mutation, widen normal Files roots, add new shared
  persistence, or introduce unapproved storage fields, endpoints, dependencies,
  renderer libraries, API names, budgets, or root permissions.
- Preserve the existing read-only `/fs` list, preview, raw, find, and search
  authority and its local/remote guarded-root behavior. Browse-root validation
  is discoverable in `fs_routes.go` through `browseRootParam` and `containedDir`;
  the separately approved local-viewer, directories-only workspace picker is
  not blanket file-content root widening. Host-action and capability checks
  remain in `fs_act_route.go`, with narrow outside-linked-file grants in
  `fs_outside.go`; do not add new symlink permissions.
- Keep any further unresolved mechanism choice as an unnumbered gap for the
  lead; do not write it as a settled decision.
- Out of scope: enabling the PDF viewer inside the Electron shell. The
  Electron shell must enable its built-in PDF plugin for native preview there;
  that belongs to the Electron shell lane, and this ticket only guarantees the
  download/open fallback when the viewer is missing.
- The session-less anchor and root discovery belong to
  `261115-feat-ws-dashboard-local-browse-anchor`, not to this ticket. The rail
  and bottom-left strip are also being changed by a parallel settings and
  home-removal ticket; P4 coordinates through the lead before touching the
  rail add-workspace popover.
- Implementation starts only after
  `261114-refactor-ws-dashboard-whiteboard-asset-split` lands, on the lead's
  start signal; that split moves `dashboard.js` and `dashboard.css`, so file
  paths cited here for those two files are pre-split and must be re-located
  through its move map.

## Prior Art

- The singleton `#files` ownership no longer exists: `WsFiles.create(node, options)`
  (`agents-plugin-tool/internal/wsdashboard/assets/files.js#L94`, exported at
  `#L880`) builds one closure per owner, and `createFiles` in
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L716-L740`
  makes the conversation Files tab instance and a lazily built toolbar-sheet
  instance, each with its own `load`/`save` adapter (the toolbar's keys prefixed
  `ws.dashboard.toolbar.`). `cards.js` still holds the toolbar Files action and
  maximize path (`cards.js#L121-L128`, `#L853-L949`), and browser state
  helpers are `load`/`save` in `dashboard.js#L30-L31`. (landed by
  `261114-refactor-ws-dashboard-file-explorer-instance-isolation`)
- Existing filesystem authority is evidenced by the `/fs` list, preview, raw,
  find, and search surfaces. Browse-root validation is discoverable in
  `fs_routes.go` through `browseRootParam` and `containedDir`; the separately
  approved workspace picker is local-viewer-only and directories-only, not
  blanket file-content root widening. Host-action and capability checks remain in
  `fs_act_route.go`, with narrow outside-linked-file grants in `fs_outside.go`.
- Existing Files-tab and read-only preview behavior is recorded in
  `261110-feat-ws-dashboard-file-preview`.

## Prior Decisions

- 261114-epic-ws-dashboard-file-explorer (2026-11-14, Cross-Child Decisions): "Explorer instances are isolated. No child may restore the current singleton as the state owner for all explorer cards." — bearing: supports
- 261114-epic-ws-dashboard-file-explorer (2026-11-14, Cross-Child Decisions): "No child may introduce an unconfirmed implementation mechanism or persist an unconfirmed decision as settled. Exact event names, renderer libraries, and store fields remain technical choices" — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "No highlighting in Phase 1; `.diff` and `.patch` render through `gitdiff.go`. ... chroma already a dependency (`gitdiff.go#L698-L732`), so highlighting needs no vendoring" — bearing: supports
- 261112-feat-ws-dashboard-outside-root-file-card (2026-11-12, Decisions): "the explorer's `root=` reads and the 261109 inline outside-image route stay as they are: the existing preview and raw routes read an outside root today through the explorer's `root=` parameter" — bearing: constrains
- 7f890500 (2026-11-10, commit): "Change root is a `root` query parameter gated by localViewer and crossSiteFetch under the files route's abs-mode path rules; no capability is minted there, so HTML and SVG answer as source text" — bearing: constrains
- 7f890500 (2026-11-10, commit): "The capability response sandboxes itself (CSP \"sandbox allow-scripts\"), not only the card's iframe attribute: the same URL opened in a tab of its own would otherwise run workspace HTML in the daemon's origin." — bearing: constrains
- 261114-feat-ws-dashboard-worktree-view (2026-11-14, Decisions): "Consume the reusable explorer component owned by `261114-feat-ws-dashboard-reusable-file-explorer-card`... The worktree page is another consumer, not a competing explorer or workspace-picker UI." — bearing: constrains
- 541fe405 (2026-11-14, commit): "Retain target-level dropdown/menu/search Escape consumption and the document-level sheet Escape handler; dismiss the Files sheet before an existing preview/card reveal... No files.js or explorer-control redesign" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | assets/files.js (explorer core), assets/dashboard.js (createFiles owners), assets/cards.js (maximize/toolbar), assets/dashboard.css (.fpane rules), fs_routes.go (PDF/highlight preview answer), assets/preview.js (OPAQUE pdf), i18n json, mockup reference |
| scope.surface | cross-module | window.WsFiles.create options and returned shown/hidden/destroy consumed by dashboard.js and cards.js; possible additive /fs preview kind or route consumed by preview.js |
| scope.new_public_symbol | yes | new selection/navigation events and attachment slot on the WsFiles instance, and a new explorer board-card kind registered with WsCards (886=a); event names left to the implementer |
| scope.new_type_contract | yes | new event payloads, attachment-slot contract, per-card store record, and any fsPreviewAnswer kind for PDF/highlighted text; none specified yet |
| scope.test_surface | new-files | existing files_test.go, cards_test.go, fs_routes_test.go, testdata/files-explorer.test.cjs cover the current explorer; tabs, view modes, split search, picker and PDF/highlight need new fixtures |
| complexity.reuse_points | confirmed | read files.js create closure, dashboard.js createFiles, cards.js Files maximize, fs_routes.go browseRoot/handleFSPreview/handleFSRaw, and gitdiff.go highlightLines (chroma already in go.mod) |
| complexity.side_effect_risk | high | tabs, view modes, split search, float/fullscreen and attachment slot rework a 881-line module shared by the Files tab and toolbar sheet; a PDF route touches the sandboxed image-CSP serving path |
| risk.correctness | high | per-tab and per-pane async search/listing state must stay independent across instances and tabs; split search targets the active pane |
| risk.fit | high | INFERRED: even split into four phases, P1 alone spans tabs, four view modes, per-card storage, the attachment slot and presentation across files.js, dashboard.js createFiles and cards.js; PDF/highlight mechanisms are settled (Decisions) and P4 depends on the separately ticketed host anchor (894=a) |
| risk.test | high | no mounted-multi-tab, view-mode, or PDF/highlight fixtures exist; browser/side-by-side acceptance is required |
| risk.security_or_contract | high | PDF serving needs a new content type behind browseRoot (raw serves only image and video types today) and the directories-only picker widens browsing toward home and volumes for local viewers while normal Files roots must stay guarded |

## Phases

Mockup frames cited below are the numbered frames of the approved
[mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html).
Every phase's acceptance includes a side-by-side comparison of a result capture
against the cited frames, at matching viewport, theme and state; elements those
frames show that belong to later epic children (change tracking, Git badges
and diff tab, worktree switching, dual pane, column view, quick-look, path
bar, favorites/recent entries, ticket status badges and board links, the
composer drag-drop zone, keyboard navigation) are not implemented here and
are excluded from the comparison by name; existing controls the frames show
(back/up navigation, host open/reveal where local permission allows) stay as
they are today. "Window" in this ticket means one explorer instance (a card,
the Files tab or the toolbar sheet), not a browser window. Every phase also keeps the existing `/fs` root guards,
read-only behavior and the existing Files-tab and toolbar-sheet entry points
working. Tests run offline (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`),
`./internal/wsdashboard` only with a `-run` filter, plus `./internal/wsi18n`
for new strings.

### Phase 1: Card core - tabs, view modes, per-card state, attachment slot, presentation

Extend the `WsFiles.create` instance (no singleton, no second Files-tab
contract) into the reusable card core:

- Explorer board card (886=a): a new board-card kind that mounts the core;
  "Open as card" in the Files tab and toolbar-sheet headers opens a new card
  at the current location; several cards coexist; the toolbar Files button is
  unchanged.

- Inner tabs: each tab has its own root/directory, selection and scroll; a
  middle-click on a folder opens it in a new background inner tab without
  switching to it. Other tab controls (new, close) follow frame 01.
- The existing lazy tree stays as the folder sidebar shown in frames 01/03,
  not as a view mode; tree and column views as modes remain the later child.
- View modes per tab: details, list, icon and gallery. Gallery shows image
  previews through the existing raw route and other files as file-type tiles.
- Per-card browser state: tabs, active tab, view mode and per-tab location
  persist through the instance's own `load`/`save` adapter, so the Files tab,
  the toolbar sheet and every explorer card never overwrite each other. An
  explorer card's state is keyed by its card ID and removed when the card
  closes. Existing Files-tab and toolbar stored records keep loading.
- Selection/navigation events and a swappable bottom attachment slot on the
  instance. The browse attachment offers `Send to session`, which attaches an
  `@path` mention (the composer's existing mention form, `mention.js`) to the
  current session's composer and never sends it, and `Copy path`. The slot contract must be able to
  host the later picker's Select/Cancel row without changing the core.
- Fullscreen presentation reuses the existing maximize sheet (1920px cap and
  extend). Float is new: a floating variant of that sheet (frame 01's floated
  card, about 1100px) that keeps the card's state. The conversation Files tab
  mounts the same core.
- Details settled from review: float is offered on explorer cards (the card
  header, as in the mockup); the Files tab and toolbar sheet keep fullscreen
  only. "Open as card" starts at the source instance's active inner tab
  directory, and from the toolbar sheet it dismisses the sheet before
  revealing the new card (Prior Decision 541fe405). Explorer cards restore on
  reload through the existing board-card persistence like the other kinds.
  `Send to session` targets the active conversation's composer and writes a
  session-relative `@path` when the instance root is the session root, an
  absolute path otherwise.

Verification: "Open as card" from both headers opens a card at the current
location; two explorer cards plus the Files tab hold independent tabs, modes
and storage; closing a card removes its stored state; the toolbar Files button
still opens the sheet; "Open as card" from the sheet leaves the new card
visible; an explorer card survives reload;
middle-click background-tab behavior; storage round-trip and pre-existing
record compatibility; attachment events and both browse actions; float and
fullscreen entry/exit; Files-tab and toolbar-sheet regression
(`TestFiles|TestToolbar|TestBoardToolbar|TestFS` plus the files-explorer JS
fixture); side-by-side against frames 01, 03 and the gallery layout of 04.

### Result (52673cf6d) - 2026-11-15

- Delivered Phase 1 only: explorer board-card registration, both Open as card
  entry points, independent inner tabs (including background middle-click),
  details/list/icon/gallery directory views beside the existing lazy tree,
  selection/navigation subscriptions, and a replaceable attachment slot.
- Board persistence restores explorer IDs; each ID owns a separate browser
  record, removed on close after destroying requests and handlers. Legacy
  root/View records and the toolbar namespace still load. Pending tab loads
  clear stale actionable rows, and navigation generations reject late answers.
- Browse actions copy a path or insert a mention into the active composer via
  `mention.js` and the existing edit path, never submit. The server listing's
  `session` flag chooses relative versus absolute host paths. Open as card uses
  the active directory and dismisses the toolbar sheet before card creation.
- Explorer-only float uses the shared sheet lifecycle at 1100px; fullscreen
  retains the 1920px cap and extend control, without replacing the core node.
  Details use existing name/type metadata; no metadata endpoint, daemon/store
  authority, guarded-root change, or protocol change was added (T0).
- Verification passed:
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'TestFiles|TestFS|TestCards|TestBoard|TestToolbar|TestLucide|TestSharedCard|TestPinnedColumn|TestFullscreenPanel' -skip '^Test(BoardCardPlacement|BoardHistoryClosedPagingSkipsMaterializedTopics|BoardHistoryHooksSurroundSSESwap)$' -count=1`
    (16.147s), including both production-JS explorer fixtures.
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsi18n -count=1`
    (1.092s); all 11 language tables include the new labels.
  - Node syntax checks for edited JS/fixtures, `gofmt -d` on the new Go test,
    and `git diff --check` passed.
- The initial wider focused run failed three pre-existing upstream string
  assertions named in `-skip` above. Their `morphBoard`, `loadClosed`, and
  `swap` bodies were verified byte-identical to upstream `b57dff0be`; the
  expected legacy strings/shape already differ there. Left untouched rather
  than coupling this phase to the parallel viewed-panel/history lane.
- Review/acceptance override: the lead explicitly requested a brief state/logic
  self-review instead of a spawned review after print mode terminated the
  attempted child before any child edits landed. Self-review covered ID-keyed
  persistence/cleanup, restore and navigation races, mention paths, existing
  route guards, and sheet transitions. No independent-review pass is claimed.
  Screenshot/reference comparison was omitted under the owner's fast-UI
  principle; the owner's on-screen check after merge remains acceptance.
- Owned-process scan (`ps` for Go/test, node fixture, server and browser
  processes) found no lingering owned processes; no server/browser was
  started, and no shared daemon was rebuilt/restarted. No push or merge.
- Phases 2-4 remain unimplemented; the ticket stays in `ready/`.

### Phase 2: Per-window name and content search on the active pane

Depends on Phase 1. Each window's search box searches by name or content
through the existing `/fs/find` and `/fs/search` routes, scoped to that
window's active pane/inner tab root. Results open files through the existing
preview path. Switching tabs or windows never shows or cancels another
window's search; in-flight requests of a closed or switched tab are dropped
without overwriting the new state.

Verification: concurrent searches in two windows and two tabs stay isolated;
mode switch name/content; cancellation on tab switch; existing search route
tests unchanged; side-by-side against the search areas of frames 01 and 05
(the change-tracking list of frame 05 is excluded).

### Result (e49b89db4) - 2026-11-15

- Completed Phase 2 search logic only, reusing the existing search box,
  name/content buttons, `/fs/find`, `/fs/search`, and preview path. Search
  uses the active inner tab's browsing root; content hits retain their line
  number when opening previews. No endpoint, root permission, protocol,
  shared-store, or filesystem mutation change (T0 dashboard assets).
- Each tab's live state retains its query, mode, case/regex options, completed
  answers and result scroll. These are not added to browser storage; existing
  card-ID persistence/cleanup and legacy records remain unchanged. Returning
  to an unfinished query restarts it under that tab's root.
- Input and mode changes invalidate pending answers immediately, before the
  debounce expires, and remove superseded actionable results. Switching or
  closing a tab and hiding or closing a window cancel only that owner's
  unfinished search. Late successes and failures are discarded even without
  AbortController; closing a background tab leaves the active request alone.
- Controlled-promise production-JS regressions cover two windows sharing one
  session, concurrent requests across two roots/tabs, state/scroll restore,
  name/content and option switching, preview root/line routing, debounce
  races, Escape, hidden-window resume, background/active-tab close, and
  destroyed-window cleanup.
- Verification passed:
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run '^TestFilesExplorerInstances$' -count=1`
    (0.930s).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'TestFiles|TestFS|TestCards|TestBoard|TestToolbar|TestSharedCard|TestFullscreenPanel' -skip '^Test(BoardCardPlacement|BoardHistoryClosedPagingSkipsMaterializedTopics|BoardHistoryHooksSurroundSSESwap)$' -count=1`
    (16.782s). The three skips are the pre-existing board assertions documented
    in Phase 1; existing search-route and guarded-path tests are unchanged.
  - Node syntax checks for both edited JS files and `git diff --check` passed.
    No UI strings changed, so `./internal/wsi18n` was not rerun.
  - Ticket verification passed with a sage-review-freshness advisory, accepted
    because this update appends only a Result and leaves the settled plan
    unchanged.
- The lead explicitly limited this phase to logic/tests after owner feedback
  on Phase 1 spacing. Search markup and card chrome are unchanged; the compact
  layout is deferred to the separate UI follow-up using its forthcoming
  mockup. Screenshot/reference rounds were omitted under the owner's fast-UI
  rule; the owner's on-screen check after merge remains acceptance.
- Per the no-agent headless override, performed only a brief logic/state
  self-review (no independent-review claim): request ownership and teardown,
  tab restore, existing card-ID persistence, preview root/line selection,
  unchanged mention relative/absolute behavior, and unchanged route guards.
  No outstanding logic/state finding was identified.
- Owned-process `ps` scan found no lingering server, browser or test processes
  started by this worker; none required stopping. No shared daemon rebuild or
  restart, push, or merge. Phases 3-4 remain unimplemented and the ticket stays
  in `ready/`.

### Phase 3: Code syntax highlighting and PDF preview

Depends on Phase 1. Implement the two settled mechanisms in `## Decisions`:

- `/fs/preview` adds the optional highlighted-lines field from the existing
  chroma path within its 256 KiB cap; the explorer's preview renders it with
  the `--hl-*` tokens and falls back to plain text when the field is absent.
- `/fs/raw` serves `application/pdf` under the existing guards with a
  PDF-specific CSP; the card frames it same-origin and shows a download/open
  fallback when the viewer is unavailable. `/fs/preview` answers a PDF inside
  the root with an additive `pdf` kind carrying its raw URL (T0: a new enum
  value; the dashboard's JS is served by the same daemon), so preview cards
  render it; preview.js keeps PDF opaque wherever no such answer exists
  (outside-root links and other consumers unchanged). Image and video raw responses,
  their CSP, and HTML/SVG capability sandboxing stay unchanged.

Verification: Go tests for the preview field (present, capped, unknown
language), PDF content type and CSP, refusal outside the root and through
symlinks, and unchanged image/video headers; a browser check that a PDF
renders, run on the full-Chromium (new headless) channel because
chromium-headless-shell has no PDF viewer, and that the fallback appears when
the viewer is absent - if no full-Chromium channel is available, record the
render check as not run instead of passing it; side-by-side against the PDF layout of frame 04 and the code
highlighting of frame 07 (the quick-look overlay itself is excluded).

### Result (576d1ad1b) - 2026-11-15

- Delivered Phase 3 only. Text preview answers optionally include
  `highlighted_lines` using the existing `highlightLines` Chroma path and
  `GitPiece` tokens. The shared preview renderer uses the existing `--hl-*`
  styles without new rows or controls. Unknown languages, files above the
  256 KiB highlighting cap, and lexer-normalized text retain plain source;
  token rendering uses text nodes and preserves blank lines and line jumps.
- PDF answers carry `kind: pdf` and the guarded raw URL. Raw PDF reads retain
  `browseRoot`/`openContained`, byte ranges, nosniff, and existing root rules;
  only PDF uses the native-viewer CSP without the image sandbox. Image/video
  headers and HTML/SVG capability sandboxing are unchanged (T0).
- The shared preview area contains the same-origin native PDF frame and a
  localized open/download escape link, with no extra toolbar or row. Missing
  viewer support shows only the link; a frame error removes the frame. The
  link remains available even if a browser advertises a viewer but fails to
  render. PDF stays opaque to other link consumers, and gallery file tiles
  remain unchanged. No `files.js` edits were needed: its existing preview
  delegation already reaches this renderer.
- Verification passed:
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'TestFS|TestFiles|TestCards|TestImage|TestToolbar|TestBoardToolbar|TestOutsidePreviewCardRemintsOnEveryLoad|TestOutsideLinkClickPath|TestDesktopPathClick|TestFullscreenPanel|TestFileView' -count=1 -timeout=3m`
    (6.498s), including production-JS highlighting/fallback tests, PDF
    containment and symlink refusal for local/LAN viewers, explicit-root
    local/cross-site guards, range reads, unchanged media headers, and
    existing capability/HTML/SVG regressions.
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsi18n -count=1`
    (1.813s); both PDF labels are present in all 11 language tables.
  - The single full-Chromium attempt passed:
    `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright go test ./internal/wsdashboard -run '^TestFSPDFBrowser$' -count=1 -v -timeout=1m`
    (4.567s). The new-headless `chromium` channel's native viewer reported
    `loadState_: success` and one page using the real raw response's bytes and
    headers on an owned loopback listener. Disabling `pdfViewerEnabled`
    exposed the fallback link without a frame request. No screenshots.
  - Node syntax checks for the renderer and both new fixtures, gofmt, and
    `git diff --check` passed.
- The initial focused run exposed a test-fixture mistake: two new tests used
  `fsViewers` without LAN mode, so its token read failed before exercising
  behavior. Corrected those fixture setups; subsequent focused runs passed.
- Lead authorization explicitly retained the assigned Phase 3 branch after
  the route's canonical-branch safety stop. With child agents disabled, brief
  self-review covered unchanged root/open guards, PDF-only CSP selection,
  token/source fidelity, detached-frame errors, and existing per-card
  persistence/cleanup and mention-path delegation. No independent-review
  pass is claimed. Screenshot/reference comparisons were omitted under the
  owner's fast-UI principle; on-screen acceptance remains with the owner.
- Owned-process scan after verification found no remaining worker-started
  server/browser/test processes; fixtures closed their listener and browser.
  No shared daemon rebuild/restart, push, merge, or ticket closure. The ticket
  stays in `ready/`; Phase 4 is untouched.

### Phase 4: Directories-only workspace-picker consumer

Depends on Phase 1 and on `261115-feat-ws-dashboard-local-browse-anchor`
having landed.

- The picker mounts the same core through the host anchor, starting from the
  discovered roots (home and local volumes, labeled with the daemon host). It
  shows directories only, files omitted, hidden entries translucent (757=b),
  and puts a Select/Cancel row in the attachment slot; no separate picker UI.
  The popover lives in the shell document, not a session page, so the picker
  loads the explorer script and styles there and gives the core an absolute
  `/s/<anchor>/` route base instead of page-relative paths.
- It is offered only to local viewers, as a browse alternative beside the
  existing local-folder paste input of the rail add-workspace popover (755/756
  rationale: paste stays the remote and fallback path). Select hands the chosen
  folder to that popover's existing preview/add flow; Cancel closes the picker
  and leaves the popover as it was.
- Coordinate the popover change through the lead first (see `## Constraints`).

Verification: browser checks that the picker lists only directories with
hidden entries translucent, starts at the discovered roots labeled with the
daemon host, never requests file previews, and that Select feeds the existing
add-workspace preview/add flow while Cancel changes nothing; that remote
viewers see only the existing paste input; `TestFiles|TestFS` and workspace
route neighbours plus `./internal/wsi18n`; side-by-side against frame 08.

### Result (69c3d2880) - 2026-11-15

- Completed Phase 4, the last phase. Local viewers get one Browse entry beside
  the existing rail workspace paste input. The popover's current implementation
  is in `dashboard.js`, not `rail.js`; only its entry and picker lifecycle were
  extended, leaving `rail.js` and the parallel compact explorer chrome alone.
- Browse discovers `/s/host0/fs/roots` and mounts `WsFiles.create` with an
  absolute `/s/host0/` route base, explicit home/volume roots labeled with the
  daemon host, and a directories-only option. The same core renders the picker;
  dot folders are translucent, ignored folders remain visible, and files,
  symlinks and vanished entries are omitted from every view. File search,
  previews, gallery media reads and host actions are suppressed in this mode.
- Select/Cancel replace the browse attachment slot. Select supplies the absolute
  host path to the existing preview/add flow without submitting Add itself;
  Cancel changes neither pasted text nor preview state. Pending root choices
  cannot select against stale labels. Refused shortcuts preserve the prior
  root; an anchor failure never falls back to an implicit session root.
- The local shell loads the core and its card helpers before the consumer.
  Remote viewers retain paste/add without Browse. The temporary picker supplies
  no load/save adapter, so existing Files-tab, toolbar and card-ID browser
  records are unchanged. Popover close and Escape destroy the instance;
  late discovery/list answers cannot resurrect it. No new endpoint, filesystem
  mutation, permission, protocol or shared-store change (T0 dashboard assets).
- Verification passed:
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'TestFiles|TestFS|TestCards|TestToolbar|TestBoardToolbar|TestLocalBrowse|TestWorkspace|TestWorktreeAddRoute|TestAddWorktreeDefaultFolder|TestSessionPageOffersTheRailAddButtons|TestSessionURLServes|TestIndexServes|TestSharedCard|TestFullscreenPanel' -count=1 -timeout=3m`
    (8.750s). Production-JS cases cover filtering, attachment/preview/add wiring,
    local/remote gating, pending/refused roots, stale discovery/listings,
    teardown and Windows drive-root separators; existing route/path guards pass.
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsi18n -count=1`
    (1.223s); Browse and Select folder are translated in all 11 tables.
  - Node syntax checks for both changed production JS files and both fixtures,
    `gofmt -d` on `files_test.go`, and `git diff --check` passed.
  - The first focused run exposed the fake DOM's inability to parse spaces
    inside attribute selectors in the new fixture. Replaced those fixture
    lookups with entry-list matching; subsequent focused runs passed.
- Followed the lead's explicit branch vouch and did not execute the route's
  canonical-branch action. With child agents disabled, performed a brief
  logic/state self-review of per-owner cleanup, nonpersistent picker state,
  absolute path selection, root-choice races and unchanged server guards.
  No independent-review pass is claimed. Frame 08 was read as design reference;
  browser/screenshot/reference rounds were omitted under the owner's fast-UI
  rule, with the owner's on-screen check after merge remaining acceptance.
- Owned-process `ps` scan found no worker-started server, browser or test
  processes remaining; no cleanup was required. No shared daemon rebuild or
  restart, push or merge. All four phases now have Results.

## Sage Review Round 1 (2026-11-15)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Explorer board-card kind and its entry point are unspecified | important | missing |
| 2 | The existing lazy tree's fate is unstated | important | autonomous |
| 3 | Side-by-side exclusion list misses some later-child elements | minor | autonomous |
| 4 | How preview.js learns a file is a PDF | minor | autonomous |
| 5 | Float has no existing mechanism | minor | autonomous |
| 6 | Send to session names a send path that does not exist | minor | autonomous |
| 7 | The asset-split gate is prose only | minor | autonomous |
| 8 | The P3 headless PDF render check may not be runnable as written | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Asset-split prerequisite is not in frontmatter | minor |
| 2 | Window is used without definition | minor |
| 3 | PDF viewer-unavailable detection and the headless Chromium acceptance | minor |
| 4 | Phase 3 preview.js PDF condition is loosely worded | minor |
| 5 | Tab lifecycle and card entry points are only implied by the mockup | minor |
| 6 | Duplicated mockup-authority statement | minor |
