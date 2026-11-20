---
title: 대시보드 전용 티켓 맵 뷰 (B 스윔레인 위계 포커스 + C 키워드 수프 성좌 · 본문 유사도)
related:
  261119-feat-ws-dashboard-shell-topbar-rail-redesign: amends its "right cluster strictly untouched" rule (Decisions, Top-right handle boundary) to add #thandle beside #shandle through drRoom()/--drhw/--thr
  261117-feat-ws-dashboard-wide-drawer-tickets-notes: the drawer 2-column Tickets panel and its demand-only idea read stay unchanged; the map is an additive full-viewport overlay that reuses the same route gate
  261119-feat-ws-dashboard-paged-docking-gallery: the map's "Open in dashboard file card" action opens a durable preview card through WsPreview.open, which the docking gallery shows and activates
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f04090c323fa1868
sage-review-completeness-reviewed: f04090c323fa1868
completed: 2026-11-20
---

# Dedicated Dashboard Ticket Map View (B Hierarchical Swimlane + C Keyword-Soup Constellation)

## Background

The dashboard drawer's 2-column Tickets panel
(`261117-feat-ws-dashboard-wide-drawer-tickets-notes`) lists `ready` and `todo`
tickets, plus `idea` tickets on demand through `GET /s/<rid>/tickets/idea`
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.js#L120-L171`).
A linear list cannot show epic hierarchy (`parent`), blocking chains
(`blocked-by`), lateral `related` links, or tickets that are semantically close
but not linked. On this repository's board (178 open tickets when the mockup was
built) that structure is invisible.

This ticket adds a full-viewport **Ticket Map** overlay on session pages
(`/s/<rid>`). A new top-right hanging handle `#thandle` next to `#shandle` opens
it. The overlay has two synchronized views, backed by a new read-only route
`GET /s/<rid>/tickets/graph`. The route serves an incrementally cached Go graph
with a content-similarity index, and needs no manual labels:

- **View B — Swimlane Matrix + 3-Tier Hierarchical Click-Focus (default).** One
  horizontal lane per auto-discovered cluster, with readiness columns
  `✓/⊘ Ref | Ready | Todo | Idea` and excerpt-free title cards. Clicking a card
  clears the matrix and shows a 3-tier hierarchy around the focal ticket:
  upstream `parent`/`blocked-by` above, lateral `related` to the right, and
  downstream children/blocks below, all wired by measured SVG connectors.
  Below the hierarchy sits a semantic-similarity `%` grid with shared
  keyword/code-path chips.
- **View C — Constellation + Keyword Soup + Ego Ring.** A deterministic, seeded,
  force-settled star map grouped into cluster hulls, with a 3-stage semantic zoom:
  - far: ticket-centroid Keyword Soup and no ticket titles;
  - mid: the soup cross-fades out while hub labels fade in;
  - close: borderless title typography, with viewport-clamped tooltips.

  The view also has cursor-anchored wheel zoom, drag-anywhere panning, and a
  radial 1–2 hop Ego Ring.

Approved visual and interaction reference (the only one):
`ai-docs/assets/mockups/261119/ticket-map-view/ticket-map-v5-bc.html`, a
self-contained file with its data inlined. Its generator reference
implementation (`build.py`, `template.html`, `verify.js`) supplied every
algorithm weight, threshold, layout constant, and browser invariant below. That
reference is folded into this ticket and is not a deliverable. The mockup's
hard-coded focus stem and its repository-specific audit picks are illustration
only and must not reach shipped code (see Constraints).

## Decisions

All decisions `(16)`–`(29)` are settled; this section is their record.

- **Dedicated `#thandle` + full-viewport overlay (`(16)=a`).** Add a 35px-wide
  hanging handle `#thandle` immediately left of `#shandle`. Placement extends
  `drRoom()` with a new `--thr` offset, and `--drhw` now covers the new handle as
  well. The handle opens a `position: fixed; inset: 0` overlay (100% of the
  viewport, the top of the 95–100% range). 100% keeps view geometry equal to the
  v5 captures, so the 1600×960 and 1280×800 checks carry over. The drawer's
  2-column Tickets panel, its idea section, and every existing handle keep their
  DOM, CSS, and behavior.
  - This explicitly **amends** `.done/261119-feat-ws-dashboard-shell-topbar-rail-redesign`.
    Its Decisions bullet "Right cluster (strictly untouched)" and its
    "Top-right handle boundary" constraint forbid touching the right-cluster
    handles; this ticket adds exactly one handle to that cluster. It still keeps
    every existing handle's CSS and DOM contract and the 760px no-clip guarantee
    (the header pads by `--drhw` through `dashboard-drawer.css#L456`, so the
    larger `--drhw` reaches the shell's mirrored header with no other change).
  - *Rejected:* replacing the drawer Tickets list, or embedding the map in the
    drawer sheet (at most 1280px wide and 2-column; too cramped for a 2D
    hierarchy and a constellation). Also rejected: a new global keyboard shortcut
    to open the map, which would collide with `/`, Ctrl/Cmd+K, Ctrl+Space
    (`dashboard-lifecycle.js#L682-L690`, tracker `qShortcut`) and the overview
    shortcut. The handle is the only entry point; it is a native `<button>`, so
    Tab plus Enter reaches it.
- **Ticket set: open tickets plus directly referenced closed tickets
  (`(17)=a` + direct-ref rule).** Load every ticket in `ready/`, `todo/`, and
  `idea/`. Add a `.done/` ticket (green `✓`) or a `.dropped/` ticket (gray, dim)
  only when an open ticket's frontmatter `parent`, `blocked-by`, or `related`
  names it.
  - Closed tickets are found by a targeted path lookup per referenced stem:
    `os.Lstat` of `.done/<stem>.md`, then `.dropped/<stem>.md` only when the
    first misses, the same order as `wsdoc.TicketStatus`
    (`wsdoc/tickets.go#L204-L222`). The builder never lists, opens, or parses
    `.done/` or `.dropped/` as a directory.
  - A reference that resolves nowhere is *dangling*. It is counted and reported,
    never an error.
  - Tickets that only a sparse checkout hides (`TicketInfo.Hidden`) are out of
    scope: the build is filesystem-only, like the idea route's
    `whiteboardTicketsList` call (`mcp/dashboard_ticket_ideas.go#L21`, a test
    seam `var whiteboardTicketsList = wsdoc.TicketsList` at
    `mcp/whiteboard_panels.go#L121`, which leaves `Resolve` off), so a hidden
    referenced stem counts as dangling.
  - *Rejected:* a full scan of `.done/` and `.dropped/` (1,302 files on this
    board at populate time, 998 in `.done/` and 304 in `.dropped/`, about 1,190
    of them unreferenced by an open ticket's `parent`/`blocked-by`/`related`;
    `ls ai-docs/tickets/.done ai-docs/tickets/.dropped`). It is noisy, and on the target machines
    each file touch costs 10–100× (`ai-docs/manuals/performance.md`).
    *Rejected:* reusing `loadTicketGraph` (`wsdoc/tickets_graph.go#L78-L120`),
    which deliberately scans with `IncludeDone`/`IncludeDropped`.
- **Two views with shared state; every taxonomy is automatic and
  project-neutral (`(18)=a`, `(19)=b`, `(21)=a`).**
  - View B and View C share one state: search query and hits, status filter,
    cluster filter, keyword filter, selection, and focus history.
  - Clustering, cluster labels, Keyword Soup, and semantic similarity are fully
    automatic. The pipeline:
    1. TF-IDF with sublinear TF and smoothed IDF.
    2. Three weighted token channels: title, code-path/symbol, and prose.
    3. Latin + Hangul + CJK tokenization.
    4. Corpus-driven lemmatization.
    5. A boost for implicit body stem mentions.
    6. An automatic high-DF cutoff.
    7. Louvain community detection with a deterministic order and a γ sweep.
    8. c-TF-IDF naming of each cluster with three terms.
    9. Per-cluster Keyword Soup at ticket centroids, excluding the cluster-name
       terms.
  - The shipped code holds only generic English and Korean function-word
    stoplists and ws ticket-convention vocabulary (status names, standard
    section headings). It holds no domain regexes, no dictionaries, and no
    repository-specific labels.
  - *Rejected:* the mockup's Variant A spatial cluster board (horizontal
    traversal read as clutter). *Rejected:* manual domain keyword regexes
    (`ai-docs/manuals/shipped-surface-boundary.md`: downstream projects have
    none of this repository's vocabulary). *Rejected:* neural, ONNX, or external
    embeddings (they break the pure-Go, dependency-free binary, and their
    scores are not explainable as shared-term chips). *Rejected:* BM25
    length-saturated TF. It was considered; the L2-normalized top-70 sublinear
    TF-IDF vector already bounds long-ticket dominance, and the v5 reference
    corpus validated it. BM25 adds two tunables (k1, b) with no measured gain.
- **View B: excerpt-free cards, 3-tier focus, inspector hidden by default
  (`(19)=b`, `(21)=a`).**
  - Matrix cards and focus cards never show body excerpts; the ticket title is
    the summary, clamped to 3 lines.
  - Click-focus removes the matrix from the DOM and keeps the redundant right
    inspector closed by default; a toolbar toggle opens it. Focus then renders
    the 3-tier hierarchy with measured SVG connectors.
  - Below the hierarchy: a semantic-similarity grid (4 columns at 1600px, 3 at
    1280px) with shared chips, `Unlinked discovery` / `Already linked` badges, a
    threshold slider, an unlinked-only toggle, and click-to-pivot with
    back/forward history and a breadcrumb trail.
  - *Rejected:* the v4 inspector-always-open focus (it duplicated the focal card
    and narrowed the canvas) and v4 excerpt rows (noise on 3-line titles).
- **View C: topographic Keyword Soup, calm overview, clamped tooltip, fixed
  pan/zoom (`(19)=b`, `(21)=a`, `(27)=ok`).**
  - At far zoom, no ticket title boxes are drawn. Borderless cluster Keyword
    Soup sits at the real weighted centroid of the tickets carrying each word,
    nudged to the nearest free spot. A soup word is clickable, and filters and
    highlights its tickets. The star and edge backdrop is toned down.
  - At close zoom, titles are borderless typography; the framed pill appears
    only on hover or select.
  - Tooltips flip and clamp inside the viewport.
  - Dragging anywhere pans; Alt+drag moves a node. A star click wins over an
    overlapping soup word.
  - Wheel zoom normalizes `deltaMode` 0, 1, and 2 and keeps the world point
    under the cursor fixed to within 1e-9.
  - Layout and soup placement run client-side in JS with a fixed seed.
  - *Rejected:* a server-side Go 2D layout. Coordinates depend on the viewport
    and the session's edge toggles, and they would bloat a payload that is
    otherwise viewport-independent and cacheable.
- **Standalone ticket and visual reference (`(22)=ok`).** This is the standalone
  `feat` ticket `261120-feat-ws-dashboard-ticket-map-view`. The only committed
  visual reference is
  `ai-docs/assets/mockups/261119/ticket-map-view/ticket-map-v5-bc.html`. No
  intermediate v1–v4 HTML and no PNG captures are committed.
- **Compatibility tier T0 transparent (`(23)=ok`).** Directional verdict:
  - *New daemon, new assets.* The route is additive: a new
    `case sub == "tickets/graph":` beside `case sub == "tickets/idea":`
    (`wsdashboard/pages.go#L1450-L1451`). It sits inside
    `handleSessionRoutes` (`pages.go#L1334`), registered once at
    `mux.HandleFunc("/s/", …)` (`daemon.go#L830`), and behind the same
    `readMethod` GET/HEAD gate (`pages.go#L1380-L1382`).
  - *No shared store, protocol, MCP, or config change.* The feature cache is
    process memory only. It is an extension of the `ticketCache` map
    (`wsdoc/tickets_cache.go#L30-L39`) plus a new in-memory closed-ref cache and
    a memoized result, and it writes no file. No MCP tool or argument, `/mcp`
    hello, tracker event, or config/rsrc manifest field changes.
  - *Old daemon, any page.* Assets are embedded in the same binary that serves
    the route (`//go:embed assets/dashboard-*.js assets/dashboard-*.css`,
    `pages.go#L38`), so an old daemon serves old assets, which have no
    `#thandle`. A new page left open after an old daemon replaces the new one
    reloads on the asset-ID change (`data-assets`, `pages.go` `ID` field). If a
    stale new page still fetches `/tickets/graph` from an old daemon, the old
    `default: http.NotFound` (`pages.go#L1468`) returns 404, and the overlay
    shows its localized "unavailable" state. Nothing is written.
  - *Browser storage.* Optional `localStorage` keys `ws.ticketmap.view` (`"B"`
    or `"C"`) and `ws.ticketmap.prefs` (JSON) go through the guarded
    `load`/`save` helpers (`dashboard.js#L76-L83`). Old assets never read them;
    a corrupt or unknown value falls back to defaults.
  - Verdict: **T0**, patch release; no minor bump.
- **Scope and fetch policy: session page only, demand-only (`(24)=ok`).**
  - `#thandle` exists only on `/s/<rid>` session pages. Its visibility follows
    the same `#p-tickets` presence that gates the drawer's idea section
    (`ticketIdeaSync`, `dashboard-drawer.js#L124-L126`), so it is hidden on the
    index, on worktree pages, on agent conversations, and in projects without
    a ticket panel.
  - `GET /s/<rid>/tickets/graph` uses exactly the idea route's gate:
    1. the outer local/LAN access guard (401 for a LAN viewer without the
       token; `access_test.go#L118` lists the idea route among
       `guardedRoutes`);
    2. `ValidReplyID`;
    3. `d.index.Session` (404);
    4. the source interface assertion (404 when the source does not support
       it);
    5. the `agentSessions` forwarder refusing agent registrations (404,
       mirroring `agent_sessions.go#L254-L260`);
    6. `lookupRecord` (404 when the record is gone).

    The handler shape follows `ticket_ideas.go#L22-L51`.
  - The page fetches only on the first overlay open and on an explicit
    Refresh. It never fetches on page load, never polls, never watches the
    filesystem, and does not refetch on reopen (the overlay keeps its state in
    memory).
- **Three sequential phases (`(25)=ok`).** Phase 1 builds the Go graph and
  similarity engine and the route. Phase 2 builds the `#thandle` entry, the
  overlay shell, and View B. Phase 3 builds View C. Each phase lands on the
  previous one and has its own Go, Node, Playwright, and capture verification.
  *Rejected:* a single phase (three independent risk surfaces — algorithmic
  determinism, page integration, canvas interaction — would share one review).
- **In-overlay Full View preview with an optional dashboard file-card hand-off
  (`(26)=ok`).**
  - `Full view` opens a modal inside the overlay. The modal holds the title,
    status and cluster badges, the `<stem>.md` tag, the line count, and the
    server-truncated `preview` rendered by a minimal escape-first markdown
    renderer (described in Phase 2). Focus, zoom, and history are untouched.
  - An **"Open in dashboard file card"** button appears only when
    `window.WsPreview && cardBoard && window.WsPreview.takes(node.path)` holds.
    `cardBoard` is in scope because the asset concatenation is one IIFE
    (`dashboard-board.js#L363`). The predicate matches `previews()`
    (`dashboard-actions.js#L284-L290`).
  - Its click runs, in order:
    1. close the modal;
    2. **yield** the overlay: set `hidden` and keep all overlay state in memory
       (view, selection, camera, filters, history), so the next `#thandle`
       click restores it without refetching;
    3. call `window.WsPreview.open(node.path, { keep: true })`
       (`preview.js#L396-L429`).

    `{ keep: true }` reuses an open card for the same path and promotes it, or
    opens a durable card. Then `card.show()` reveals it, activating its docked
    gallery page when the card is docked.
  - `node.path` is the board-relative `ai-docs/tickets/<dir>/<stem>.md`, which
    is session-root-relative, the same shape the idea rows give `data-rel`
    (`dashboard-drawer.js#L140`).
  - *Rejected:* closing the overlay to go straight to a docked card on every
    ticket click (it loses focus and zoom context). *Rejected:* destroying the
    overlay on hand-off (that forces a refetch and loses history).
- **Client-side deterministic layout (`(27)=ok`).** See the View C decision. The
  seed is fixed at `42`. Node order is the server's node order, so the same
  payload gives the same coordinates.
- **Localization across all 11 `wsi18n` tables (`(28)=ok`).** Every overlay UI
  string is a `js.tmap.*` key present in all 11 tables (`wsi18n/tables/`: de,
  en, es, fr, it, ja, ko, pt-BR, ru, zh-Hans, zh-Hant). Each key is named by a
  string literal in JS, so the key sweep (`wsi18n/keys_test.go#L25`, regex
  `"(js\.[a-z0-9.]+)"`) sees it. Plurals use `wsTn` with `.one`/`.other` pairs
  (`keys_test.go#L28`). Placeholders are `{0}`, `{1}`, … as in English.
  Frontmatter field names (`parent`, `blocked-by`, `related`) render verbatim as
  code tokens, because they are file syntax, not prose. Server output contains
  no UI prose: the Misc cluster and an unnamed cluster arrive as data flags, and
  the client localizes them.
  *Rejected:* carrying the mockup's hard-coded Korean and English strings.
- **First-open default view (`(29)=a`).** The first overlay open in a browser
  shows View B. `localStorage` `ws.ticketmap.view` then remembers the last view
  shown (`B` or `C`), set on every view switch.
  *Rejected:* `(29)=b`, View C first (the readiness matrix is immediately
  actionable for triage).
- **Word lists are authored, not ported (auto-confirmed technical).** The v5
  `build.py` is not committed, so "ported verbatim" `EN_STOP`/`KO_STOP`/`KO_SUFFIX`
  cannot be followed. The implementer authors generic English and Korean
  function-word lists and a Korean particle/ending list under the
  shipped-surface boundary, validated by the Phase 1 text and cluster tests.
  *Rejected:* committing the v5 generator as a reference file (it is not a
  deliverable, and its repository-specific audit picks must not reach shipped
  code).
- **Performance shape (`performance.md` rules 1, 4, 5, 7).**
  - Per-ticket parse results and term statistics live in the `ticketCache`
    entry and are revalidated by `(size, modTime)` from the directory listing
    (`tickets_cache.go#L55-L70`). An unchanged ticket costs zero opens.
  - The whole result is memoized by a corpus fingerprint, so an unchanged board
    costs zero CPU beyond the listings and stats.
  - Exceptions needing owner approval under rule 8: none. The build is
    read-only and demand-only, off every interactive path except the user's own
    overlay open.

## Constraints

- **Required manuals (`AGENTS.md` Implementation Conventions):**
  - `ai-docs/manuals/shipped-surface-boundary.md` (declared for
    `agents-plugin-tool/`). The shipped Go, JS, and CSS name no ticket stem of
    this repository, no `ai-docs/tickets/<status>/<real-stem>` path, and no
    repository-specific term. Stopword lists are generic function words plus ws
    ticket-convention vocabulary only. Test fixtures use synthetic,
    project-neutral corpora (Phase 1 fixture), never this repository's board.
    The mockup's `FOCUS_STEM` and audit `pref` list must not be ported.
  - `ai-docs/manuals/release-compatibility.md` (declared for
    `agents-plugin-tool/internal/`): tier T0 as recorded in Decisions.
  - `ai-docs/manuals/performance.md` (declared for `agents-plugin-tool/`).
    **File-touch budget per overlay open or Refresh**, counted by Phase 1's
    touch test. The fixed costs are:
    - exactly 3 directory listings (`ready/`, `todo/`, `idea/`) through an
      error-returning `readTicketDir` seam (wrapping `os.ReadDir`, sorting
      names like `sortedEntries` while surfacing non-`ErrNotExist` read errors
      instead of silently returning `nil`);
    - on non-Windows, the existing one `entry.Info()` lstat per open ticket
      entry (free on Windows);
    - at most 2 `Lstat` calls per distinct referenced closed stem (`.done/`
      first, `.dropped/` only on a miss).

    On top of that, a warm second build with an unchanged board performs
    **0 file opens**, and a cold build performs exactly one `readTicketFile`
    per included ticket. Session lookup cost is the idea route's existing
    `lookupRecord` and is unchanged. Background polling, filesystem watchers,
    and page-load fetches are all forbidden.
- **Zero external dependencies.** Pure Go standard library on the server.
  Vanilla JS, inline SVG, and CSS on the client, inside the existing asset
  concatenation; no npm runtime packages, no CDN, no canvas or graph library.
  The handle icon is vendored as `assets/lucide/icons/network.svg` from
  `lucide-static@1.71.0` following `assets/lucide/README.md` (copied unchanged;
  the existing `TestLucideIconsAreVendoredWithTheirLicense`, `cards_test.go#L20`,
  covers it). Playwright is test-only and comes from the cached
  `WS_PLAYWRIGHT_MODULE`, as in `shell_topbar_browser_test.go#L18-L21`.
- **Asset concatenation.**
  - New script files go into `dashboardScriptFiles` (`pages.go#L44-L62`):
    `dashboard-ticketmap.js` immediately after `"dashboard-drawer.js"` (Phase 2),
    and `dashboard-ticketmap-graph.js` immediately after it (Phase 3).
  - `dashboard-ticketmap.css` goes into `dashboardStyleFiles`
    (`pages.go#L64-L73`) immediately after `"dashboard-drawer.css"`.
  - The embed glob already covers the names (`pages.go#L38`).
  - The expected lists in `asset_source_test.go#L20-L21` gain the same entries
    in the same positions.
  - The concatenation is one IIFE joined without separators
    (`pages.go#L40-L43`, `#L87-L109`). Every new file therefore keeps all of its
    declarations inside one inner function scope and exposes exactly one outer
    binding: `var ticketMap = (function () { … })();` in
    `dashboard-ticketmap.js`, and `ticketMap.graph = (function () { … })();` in
    the Phase 3 file. Nothing can hoist over, or be shadowed by, an existing
    IIFE function such as `load`, `save`, `$`, `q`, `luIcon`, `wsT`, or
    `drRoom`.
  - A Go test asserts that `ticketMap` is declared exactly once in the
    concatenated `dashboard.js`, and that no new file contains a top-level
    `function ` declaration (the first non-comment statement is the `var` or
    assignment).
- **CSS collision rule (`css_collision_test.go#L23-L29`, `#L272-L331`).**
  - The new stylesheet joins the `dashboard` module through the concatenation
    and inherits its `class prefix: d` header (`dashboard.css#L1`).
  - Every new class starts with `d-tm-`, and every new element id starts with
    `tm`, except `#thandle`. No mockup class name is ported as is. In
    particular `.lane`, `.lanes`, `.chip`, `.cat`, `.btn`, `.more`, `.on`,
    `.sel`, `.open`, and `.done` are whiteboard or allowlisted names
    (`.lane`/`.lanes`/`.cat` are allowlisted only "under #p-tickets"). The map
    may reuse an existing shared atom only by composition, for example
    `class="btn d-tm-btn"`. **No new `allowedCSSOverlaps` entry** may be added.
  - Theme: overlay tokens derive from `--wb-*`
    (`wswhiteboard/assets/page.css#L6-L95`, `dashboard.css#L15-L46`). The two
    extra hues, `--d-tm-sim` and `--d-tm-idea`, are declared three times:
    light at `:root`, dark inside `@media (prefers-color-scheme: dark)
    { :root:not([data-theme="light"]) }`, and in `:root[data-theme="dark"]`.
    The values are the mockup's: light `#6a4bd6` / `#8a6d2f`, dark `#a68cff` /
    `#c9a55a`. This mirrors the dashboard's two-block dark rule.
  - Status colors map ready→`--wb-ok`, todo→`--wb-primary`,
    idea→`--d-tm-idea`, done→`--wb-muted-fg`, dropped→`--wb-fail`.
  - The overlay has no theme toggle of its own; it follows the page theme.
- **Layering and isolation.**
  - Layers (tokens from `dashboard-shell.css#L25`):
    - the overlay backdrop `.d-tm-back` uses `z-index: var(--z-modal)` (70)
      and paints `var(--wb-bg)` opaque;
    - the in-overlay Full View modal and the tooltip use
      `calc(var(--z-modal) + 2)`.

    The overlay therefore covers the drawer (`--z-panels`, 40), popovers (60),
    the maximized-card scrim (69), and maximized cards (70; later in DOM order).
  - The overlay is appended to `document.body` last, and re-appended on every
    open, so a card created or maximized after the first open (same `z-index`,
    later in DOM order) never paints above it.
  - Opening the overlay closes an open session drawer through `drSetOpen(false)`.
  - Shell frame lift: the selector list in `dim()` (`dashboard-lifecycle.js#L738`)
    gains `.d-tm-back:not([hidden])`. A framed page with the overlay open then
    reports a backdrop, and the shell lifts the frame to `--z-modal`
    (`dashboard-shell.css#L32`), so the overlay covers the shell's header and
    rail.
  - Overview isolation: overview's `classify()` palette selector
    (`overview.js#L50`) gains `.d-tm-back`, so a click inside the overlay is
    never an overview "outside" click.
  - Keyboard isolation: while the overlay is visible, one `keydown` listener on
    `window` in the capture phase (registered on open, removed on close or
    yield) runs before the drawer's document-capture Escape handler
    (`dashboard-drawer.js#L475-L482`), `tracker-mount.js#L404`, and every
    document/bubble page shortcut. Load-time window-capture Escape listeners
    (`dashboard-git.js#L760`, `dashboard-rail-popovers.js#L1174,#L1185`,
    `dashboard-conversation.js#L188`) only act while their own UI is open;
    clicking `#thandle` (and `ticketMap.open()`) dismisses any open transient
    popover/modal before opening so those listeners are inert while the map is
    active. The overlay listener handles the overlay keys and calls
    `preventDefault()` and `stopImmediatePropagation()` for every key it
    consumes. It also swallows `/`, Ctrl/Cmd+K, and Ctrl+Space (focusing the
    map search instead), so the quick-search palette never opens under the
    overlay.
  - The listener ignores IME composition (`e.isComposing || e.keyCode === 229`)
    and typing targets (`input`, `textarea`, `select`, and `[contenteditable]`
    other than range and checkbox inputs), except for Escape, which is still
    layered.
- **Mockup asset.**
  `ai-docs/assets/mockups/261119/ticket-map-view/ticket-map-v5-bc.html` is a
  read-only reference. No phase modifies, regenerates, or re-saves it.
- **Existing code to coordinate with (extend; do not replace):**
  - `wsdoc/tickets_cache.go`, `wsdoc/tickets.go`, `wsdoc/tickets_deps.go`;
  - `wsdashboard/pages.go`, `wsdashboard/ticket_ideas.go`,
    `wsdashboard/agent_sessions.go`, `mcp/dashboard_ticket_ideas.go`;
  - `assets/dashboard-drawer.js`, `dashboard-drawer.css`,
    `dashboard-lifecycle.js`, `overview.js`, `preview.js`, `cards.js`, and
    `gallery.js` (read-only here: no change to either).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Decisions

- 261119-feat-ws-dashboard-shell-topbar-rail-redesign (2026-11-19, Decisions): "Right cluster (strictly untouched): Existing iframe-owned top-right handles ... `#shandle`, drawer `#drhandle` and their CSS/DOM contracts ... must not be modified." — bearing: constrains
- 261117-feat-ws-dashboard-wide-drawer-tickets-notes (2026-11-17, Decisions): "Idea fetched on demand — (29)=ok. A new route reads `idea/` only when the user presses `더보기`. Rejected: adding idea to the live panel region." — bearing: supports
- 6b2963ee (2026-11-17, commit): "Read idea only on an explicit button press via a session-indexed read route; do not add idea to live panel reads or watches." — bearing: supports
- c7e79e71 (2026-11-17, commit): "Stale-data guards: a file whose mtime is within 2 s of the listing is never cached (same-tick same-size rewrite); symlinked tickets are always reread (the listing describes the link)" — bearing: constrains
- 961c8e3b (2026-11-15, commit): "Concatenate contiguous JavaScript source fragments without separators into the original single IIFE, preserving declaration hoisting, early return, shared state, handler ordering" — bearing: constrains
- 261119-feat-ws-dashboard-paged-docking-gallery (2026-11-20, commit 04dde7ac): "Resolved single conflict in agents-plugin-tool/internal/wsdashboard/cards_test.go against develop ... preserving develop's mark-free pin assertions alongside gallery page/pane assertions" — bearing: supports
- e207bc36 (2026-11-13, commit): "feat(dashboard): wide-view header handles at window edge, idle pin placeholder, board-width handle" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdoc/tickets_cache.go, wsdoc/tickets_map*.go, wsdashboard/pages.go, ticket_graph.go, agent_sessions.go, mcp/dashboard_ticket_graph.go, assets/dashboard-drawer.js, dashboard-ticketmap.js, dashboard-lifecycle.js, overview.js, wsi18n/tables/*.json |
| scope.surface | cross-module | new route GET /s/<rid>/tickets/graph spanning wsdoc, wsdashboard, mcp, wsi18n and embedded assets; drRoom geometry shared by the shell header |
| scope.new_public_symbol | yes | wsdoc.TicketMap, wsdoc.TicketMapJSON, wsdashboard.TicketGraphSource, (*DashboardSessionSource).ReadTicketGraph, route tickets/graph |
| scope.new_type_contract | yes | TicketMapResult and the versioned JSON payload (meta, clusters, clusterKeywords, nodes, edges, sim); TicketGraphSource.ReadTicketGraph |
| scope.test_surface | new-files | none exist yet: search for ticketmap under wsdashboard and wsdoc returned nothing; also extends existing asset_source_test.go, access_test.go, css_collision_test.go |
| complexity.reuse_points | confirmed | read: ticketCache and scanStatusDir (tickets_cache.go#L24-L85), listStatusDirEntries (readTicketDir seam), parseBlockedByEdge, relatedEntries, ticketPhases, handleTicketIdeas shape (ticket_ideas.go#L22-L51), drRoom, WsPreview.open |
| complexity.side_effect_risk | moderate | extends the shared ticketCache that TicketsList and git.commit verification also use (wrapper keeps callers unchanged); drRoom, dim() and overview classify selectors change for every session page |
| risk.correctness | high | deterministic Louvain, tokenizer and byte-identical JSON across GOMAXPROCS within one build and host; the v5 generator is not in the tree, so word lists are authored and constants come from this ticket |
| risk.fit | moderate | amends the done shell redesign handle boundary; must obey the single-IIFE asset concatenation and CSS collision rules |
| risk.test | high | three phases of new Go, Node and Playwright suites, a synthetic fixture generator, touch-count and determinism tests, and screenshot comparison against the mockup |
| risk.security_or_contract | moderate | new read-only route behind the existing access guard and agent refusal; escape-first markdown renderer; T0 additive payload with a version field |

## Phases

Test harnesses shared by all phases. Run them from `agents-plugin-tool/`;
prefix with `~/.local/bin/ws-rgo` on remote-Go hosts.

- **Go:** `go test ./internal/wsdoc ./internal/wsdashboard ./internal/mcp ./internal/wsi18n`.
- **Node unit tests:** `node --test internal/wsdashboard/testdata/ticketmap-*.test.cjs`,
  also run by the Go wrapper `internal/wsdashboard/ticketmap_model_test.go`. The
  wrapper follows `gallery_model_test.go` and skips when `node` is absent. These
  tests cover pure logic only, with no DOM: each loads its file's inner-function
  source through `vm`, the way `frontend-drawer.test.js` uses `section()`.
- **Playwright browser tests:** `testdata/ticketmap.browser.cjs` driven by
  `internal/wsdashboard/ticketmap_browser_test.go` (Phase 2) and
  `testdata/ticketmap-graph.browser.cjs` driven by
  `ticketmap_graph_browser_test.go` (Phase 3). Both follow
  `shell_topbar_browser_test.go#L18-L51`: they skip without
  `WS_PLAYWRIGHT_MODULE`, start `startPageFixture` on `127.0.0.1` with an
  ephemeral port, and keep screenshots in `WS_TICKETMAP_SHOTS` (an absolute
  path). Run with
  `WS_PLAYWRIGHT_MODULE=<cached playwright> WS_TICKETMAP_SHOTS=/abs/dir go test ./internal/wsdashboard -run 'TicketMap.*Browser' -count=1`.
- **Synthetic board fixture**, shared by Go and browser tests:
  `wsdoc/testdata/ticketmap/` is generated deterministically by
  `writeTicketMapFixture(t, dir, n)` in `wsdoc/tickets_map_fixture_test.go`, and
  copied for wsdashboard through an exported test helper in
  `internal/wsdoc/wsdoctest`. Its corpus is project-neutral (invented domains,
  invented paths) and plants:
  - 8 Latin-script topics of 12–24 open tickets each, each with distinct title
    words and one code path such as `ledger/posting.go` or
    `garden/moisture.py`. The Korean and Japanese topics below are additional.
    The implementer picks exact sizes so the default `n=170` open tickets
    includes the epic's 11 children and the 5 isolated tickets and the cluster
    assertions hold;
  - one Korean topic (`주문`, `배송`, `추적`, …) and one Japanese topic (Kanji
    and Katakana titles);
  - a term present in every ticket (`widget`), which must be high-DF cut;
  - an epic `260101-epic-orchard-platform` with 11 children, 2 parents up its
    ancestry, 6 related, 2 blocked-by, and 3 blocks;
  - 9 closed referenced tickets (6 `.done`, 3 `.dropped`) and 40 unreferenced
    closed tickets;
  - 2 dangling references, a 2-cycle (`parent` A↔B), a 3-cycle (`blocked-by`
    A→B→C→A), a self-reference, a `blocked-by: <stem>#phase2` edge, 3 body-only
    stem mentions, a ticket with broken frontmatter (no closing fence), and a
    ticket with no title;
  - 5 isolated low-similarity tickets.

  Default `n=170` open tickets; `n` scales the topic sizes. Stems are invented
  (`2601xx-…`), and no real ticket of this repository is used.

### Phase 1: Incremental Go Graph & Semantic Similarity Engine (`wsdoc` + `GET /s/<rid>/tickets/graph`)

**Files to create or modify**

- Modify `agents-plugin-tool/internal/wsdoc/tickets_cache.go`:
  - add `doc *ticketDoc` to `ticketCacheEntry` (`#L24-L28`);
  - split `scanStatusDir` (`#L48-L85`) into an unexported
    `scanStatusDirWith(root, statusDir, status string, wantDoc bool)` and keep
    `scanStatusDir` as a `wantDoc=false` wrapper, so no existing caller changes;
  - add `ticketRefCache` (a mutex-guarded `map[absPath]ticketCacheEntry`) for
    closed refs;
  - add `var lstatTicketFile = os.Lstat` and `var readTicketDir = os.ReadDir`
    as counting seams next to `readTicketFile` (`#L20-L22`), with an
    error-returning sorted status-dir listing helper (`listStatusDirEntries`)
    so non-`ErrNotExist` directory read errors propagate to `500` rather than
    returning `nil` like `sortedEntries` (`project_tree.go#L299`);
  - keep per-root `doc *ticketDoc` and fingerprint memo memory bounded in
    long-lived multi-worktree daemons (evict least-recently-used roots past a
    modest cap, e.g. 16 roots, or record measured per-root footprint alongside
    the Phase 1 benchmark).
- Create `wsdoc/tickets_map.go`: loading, ref lookup, fingerprint memo, result
  types, JSON encoding, and the exported entry points.
- Create `wsdoc/tickets_map_text.go`: the markdown section parser, tokenizers,
  the `ticketDoc` builder, and the stoplists.
- Create `wsdoc/tickets_map_cluster.go`: TF-IDF, similarity, the graph, Louvain,
  split/absorb, c-TF-IDF, and the soup.
- Create the tests `wsdoc/tickets_map_test.go`, `tickets_map_text_test.go`,
  `tickets_map_cluster_test.go`, `tickets_map_touch_test.go`, and
  `tickets_map_fixture_test.go`, plus `wsdoc/wsdoctest/ticketmap.go` (the
  fixture writer exported for other packages' tests).
- Create `wsdashboard/ticket_graph.go`: the `TicketGraphSource` interface and
  `handleTicketGraph`.
- Modify `wsdashboard/pages.go`: add `case sub == "tickets/graph":` right after
  `#L1450-L1451`.
- Modify `wsdashboard/agent_sessions.go`: add a `ReadTicketGraph` forwarder
  after `#L254-L260` that returns `nil, false, nil` for `reg.Agent` or an
  unsupported inner source.
- Create `mcp/dashboard_ticket_graph.go`: `(*DashboardSessionSource).ReadTicketGraph`,
  shaped like `mcp/dashboard_ticket_ideas.go#L12-L30`, with the same
  `lookupRecord` call. It calls `wsdoc.TicketMapJSON(record.Root)` through a
  test-seam variable `ticketMapJSON = wsdoc.TicketMapJSON`. An error wrapping
  `os.ErrNotExist` maps to the empty payload.
- Create the tests `wsdashboard/ticket_graph_test.go` and
  `mcp/dashboard_ticket_graph_test.go`.
- Modify `wsdashboard/access_test.go`: add
  `{http.MethodGet, "/s/livesession/tickets/graph", ""}` to `guardedRoutes`
  after `#L118`.

**Contracts**

- `wsdoc.TicketMapJSON(root string) ([]byte, error)` returns the encoded
  payload, memoized. `wsdoc.TicketMap(root string) (*TicketMapResult, error)`
  is the typed form used by the tests. Both are safe for concurrent calls: a
  package `sync.Mutex` serializes builds, so concurrent requests during a build
  wait and then reuse the memo.
- **Load.**
  1. For each of `ready`, `todo`, `idea` under
     `<root>/ai-docs/tickets/`, call `scanStatusDirWith(..., wantDoc=true)`. On
     a revalidated hit with `doc != nil`, reuse it with no open. On a hit with
     `doc == nil` (cached by a plain listing), read once and fill `doc` into the
     carried entry. On a miss, parse `TicketInfo` and `doc` from the same bytes
     with one `readTicketFile`. The racy-window rule (`#L13-L18`, `#L75-L77`)
     applies unchanged.
  2. A missing `ai-docs/tickets/` returns an error wrapping `os.ErrNotExist`;
     any other stat or read error is returned as is.
  3. Duplicate stems: the first in `ticketStatusRank` order wins, mirroring
     `loadTicketGraph`'s first-wins rule (`tickets_graph.go#L96-L105`).
  4. Ref set: union over open tickets of `Parent`, `BlockedBy` (stripped to the
     stem by `parseBlockedByEdge`, `tickets_deps.go#L83`, keeping the `#phaseN`
     fragment for the edge), and the `Related` keys. A value that is not a
     stem by `ValidTicketStem` is ignored. Self-references are dropped. Stems
     already open are skipped.
  5. Resolve the remaining stems in sorted order by `lstatTicketFile`
     (`.done` → `.dropped`).
  6. `ticketRefCache` revalidates each closed ref by the lstat's size and
     mtime; a miss reads once. A stem found in neither directory is dangling.
- **`ticketDoc`.** Immutable after construction and shared by pointer.
  - `Title`: frontmatter `title`, else the first H1, else the stem.
  - `Lines`: the newline count + 1.
  - `Headings []heading`: level and text, from H2 down; fence-aware via
    `markdownFence` (`tickets.go#L818`).
  - `Preview string`: sections rendered as `## <name>\n<text>`, joined with
    blank lines and truncated to 1500 runes for open tickets and 700 runes for
    closed refs, ending in `\n…` when truncated.
  - `Mentions []string`: stems matched anywhere in the body.
  - `Terms map[string]*termStat`: `w [4]float64` holds the per-channel weight
    sums and `n [4]int32` the occurrence counts, over the channels 0 title, 1
    heading, 2 code, 3 prose. It is the lossless aggregate of the v5 raw token
    list. Lemma merges sum stats later, so the raw list is never stored.

  Features read at most the first 256 KiB of a body.
- **Tokenization.** Every numeric weight, threshold, and regex below is
  specified in this ticket and is the contract. The v5 builder is not committed,
  so the word lists are not ported: the implementer authors them (see Stoplists).
  - Weights: `W_TITLE=2.4`, `W_CODE=3.0`, `W_PROSE=1.0`. A section matching
    `^(context|goal|goals|scope|problem|background|motivation|summary|overview|why)\b`
    (case-insensitive) is scaled ×1.4. A section matching
    `^(verification|constraints?|results?|resolution|notes?|acceptance|test|tests|validation|history|log)\b`
    is scaled ×0.5. Frontmatter relation notes carry weight 0.6 on channel 3.
  - Channel 0: the stem core after `^\d{6}-[a-z]+-`, split on `-_/`, plus the
    title.
  - Channel 1: headings, with a leading `Phase N`, `Result`,
    `(<hash>)`, or `- YYYY-MM-DD` and a `:` stripped.
  - Channel 2:
    - every backtick span (one line, at most 160 chars, not a stem, not a URL;
      call arguments dropped; `:<line>` suffixes stripped; pieces over 34 chars
      or containing `:` dropped; hex-hash segments dropped);
    - bare file paths matching
      ``(?<![\w/.])((?:[\w.-]+/)*[\w-]+\.(go|js|mjs|ts|tsx|jsx|py|md|css|scss|html|json|rs|toml|ya?ml|sh|ps1|sql|txt|lock|mod|sum|svg|png))(?!\w)``
      outside backticks;
    - slash paths `[a-z][\w-]*(?:/[\w.-]+)+` that carry an extension or are
      not a bare `word/word`.

    Path spans emit lowercased segments plus the basename with its extension
    and without it. `#id` and `.class` selector tokens keep their marker.
    Dotted identifiers are kept whole.
  - Channel 3: section prose with stems, backticks (punctuation turned into
    spaces), URLs, and `{}()[];=<>:+*&|!"'^~$%@\` removed.
  - Word tokenizer:
    - camelCase is split first;
    - Latin `[A-Za-z][A-Za-z0-9]*` is lowercased and dropped when noise (length
      < 2, all digits, digits making up ≥ half its length, or a hex hash with a
      digit) or a stopword;
    - Hangul runs `[\x{AC00}-\x{D7A3}]{2,}` are stripped of the longest
      matching particle or ending from `koSuffixes` (authored per Stoplists),
      keeping at least 2 syllables, then dropped when shorter than 2 or in
      `koStop`;
    - Han, Hiragana, and Katakana runs (`\p{Han}`, `\p{Hiragana}`,
      `\p{Katakana}`) of length ≥ 2 emit overlapping rune bigrams (no
      dictionary segmentation); single-rune runs are dropped;
    - invalid UTF-8 bytes are skipped.
  - Stoplists: `enStop`, `koStop`, and the Korean particle/ending list
    `koSuffixes` are authored by the implementer in `tickets_map_text.go`. They
    hold English and Korean function words (Korean: common josa and verb
    endings, longest-match first) and ws ticket-convention words only, with no
    domain or repository-specific term. Their correctness contract is the text
    tests: `화이트보드를` → `화이트보드`, English function words and
    status/section-heading words filtered, and the cluster tests on the
    fixture. The lists are an implementation detail, not a payload field, so
    changing them needs only `ticketMapVersion` to be bumped when cluster output
    changes.
- **Lemmatization.** Corpus-driven; build `LEMMA` by iterating the vocabulary
  in lexicographic order. Candidate order for a pure `[a-z]+` word:
  1. `ies`→`y` (length > 5);
  2. strip `es` (length > 5);
  3. strip `s` (not `ss`/`us`/`is`, length > 4);
  4. for `ing` and then `ed` with a base of ≥ 4: the base, base+`e`, the
     de-doubled base, and `i`→`y`.

  The first candidate present in the vocabulary that is not a stopword wins.
  Chains resolve at most 3 hops. Code-channel tokens are lemmatized only when
  they are pure `[a-z]+`.
- **Vectors.**
  - `df_open` is counted over open tickets and `df_all` over all included
    tickets.
  - Cut terms with `df_open/N_open > 0.32` (strictly greater) and terms with
    `df_all < 2`.
  - `idf(w) = ln((N_all+1)/(df_all(w)+1)) + 1`.
  - `tfw = 1 + ln(f)` for `f ≥ 1`, else `f`.
  - Keep the top 70 terms by weight (ties by term, ascending) and normalize by
    L2.
  - `isCodeTerm(w)` holds when `w` contains any of `./_#`, or when
    code-channel occurrences ≥ 0.5 × the sum of the other channels'
    occurrences.
- **Similarity.**
  - Cosine through an inverted index over the vectors, iterating terms in
    sorted order and postings in node order.
  - A body-mention pair (either body names the other's stem, with no explicit
    relation between them) gets `min(0.98, s + 0.18)`.
  - Neighbors: keep `s ≥ 0.08`, sort by score descending then stem ascending,
    and keep the top 25.
  - Shared terms: the top 4 by product `a[w]·b[w]`, de-duplicated by base (the
    trailing `.ext` and the `#`/`.` markers stripped), each with its code flag.
- **Graph (open tickets only).**
  - Explicit edges carry weights `parent 4.0`, `blocked-by 3.2`, `related 2.2`,
    added symmetrically.
  - Semantic edges: for each open ticket, up to 4 open neighbors with
    `s ≥ 0.16`, adding `s × 3.5 / 2` in each direction.
- **Scaled clustering parameters.** The v5 constants are the `N_open≈178`
  instance:
  - `MIN_COMM = 4 if N_open ≥ 40, 3 if ≥ 16, else 2`;
  - `MAX_COMM = max(12, ceil(0.21·N_open))`, which is 38 at 178;
  - `TARGET_K = [clamp(round(√N_open/2), 2, 7), clamp(round(0.9·√N_open), 3, 12)]`,
    which is [7, 12] at 178.

  When `N_open < 8`, skip Louvain and put every open ticket that is not
  isolated into one cluster.
- **Louvain.** Deterministic:
  1. Nodes are processed in node-index order. Candidate communities are
     visited in ascending id order. A move needs a gain greater than
     `best + 1e-12`.
  2. Aggregation relabels communities by their first appearance in node order.
     There are at most 10 levels.
  3. Final relabel: by size descending, then smallest member index.
  4. Sweep γ from 0.4 to 3.0 in steps of 0.1. Each run is
     `louvain → splitOversized → absorbTiny → splitOversized → absorbTiny`.
  5. `splitOversized`: re-run Louvain on a cluster larger than `MAX_COMM` at
     γ×{1.3, 1.7, 2.2, 3.0}. Accept the first result with ≥ 2 communities and
     the largest < 0.85·size. Run at most 6 rounds.
  6. `absorbTiny`: at most 10 rounds. A cluster under `MIN_COMM` merges into
     the neighbor cluster with the highest connection. Connection counts graph
     edges plus 0.5 × similarity to each of its members' top 6 open neighbors.
     Ties go to the lowest cluster id.
  7. Score = (size entropy / ln k) − 2·(distance of k outside `TARGET_K`) −
     max(0, largest − `MAX_COMM`)/6.
  8. Pick the γ with the highest `(score, modularity)`; a tie keeps the lower γ.
- **Misc and refs.**
  - An open ticket with no graph edge and a best-neighbor score `< 0.12` goes
    to a trailing Misc cluster (`misc: true`).
  - A closed ref joins the cluster with the highest score. The score sums
    relation weights to members (`parent`/`child` 4.0, `blocked-by`/`blocks`
    3.2, `related` 2.2) plus `s × 3.5` over its top 8 neighbors. Ties go to the
    lowest cluster id. A ref with no score joins Misc, or cluster 0 when there
    is no Misc.
- **Cluster order, colors, and names.**
  - Order clusters by active weight descending (ready 3, todo 1.5, idea 0.6),
    then the previous id ascending, with Misc last. Renumber to 0..K−1.
  - `hue = round((k·360/K + 205) mod 360)`.
  - c-TF-IDF per open ticket and term:
    - presence score = title-channel 3.0 + heading 1.0 + code 1.0;
    - `cdf(w)` = the number of clusters whose summed score is ≥ 2.0;
    - `A` = the mean per-cluster total;
    - score = `(s_c(w)/total_c)·ln(1 + A/ftot(w))·(1 + tick_c(w)/size_c)·(1 + 1.5·title_c(w)/size_c)`.
  - Name candidates: terms with `tick_c ≥ max(2, ceil(0.12·size_c))` and
    `cdf/K ≤ 0.60`, de-duplicated by substring of the base or an equal lemma,
    keeping up to 8. Terms shorter than 3 runes are dropped for Latin, digit,
    and code terms; Hangul and CJK terms need only 2 runes (the v5 reference's
    byte-agnostic `len < 3` dropped 2-syllable Hangul terms).
  - Names: first take candidates carried in titles by ≥ 2 tickets (≥ 1 when
    `size_c < 6`) that are not code terms, then the rest, up to 3 terms.
  - Labels: Latin terms get an uppercased first rune; code terms (with any of
    `./_#`) and Hangul/CJK terms stay as is. Terms are joined with `" · "`. An
    empty name gives `label: ""`.
- **Keyword Soup per cluster.**
  - Candidates use `tick_c ≥ 2` and `cdf/K ≤ 0.50`, with the same
    de-duplication, up to 40.
  - Keep at most 2 code terms and 10 terms in total, each carried by ≥ 2
    member tickets.
  - `weight` = score / the cluster's top score.
  - For each carrying member ticket, `tw = round(max(vec[w], 0.02) + 0.15·[term in title channel], 3)`.
- **Fingerprint memo.** A SHA-256 over sorted `(relPath, size, modTime
  UnixNano)` of every included file, plus the sorted dangling stems and the
  constant `ticketMapVersion = 1`. The memo is keyed by `root`, so sessions on
  different project roots do not evict each other. When the fingerprint equals
  that root's last build, return the cached bytes. Store a memo only when no included file is
  younger than `ticketCacheRacyWindow`.

**`GET /s/<rid>/tickets/graph` response (`200`, `application/json`, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`)**

```json
{
  "meta": {
    "v": 1, "fingerprint": "<first 16 hex of the SHA-256>",
    "open": 170, "ready": 20, "todo": 70, "idea": 80,
    "refs": 9, "refDone": 6, "refDropped": 3, "dangling": 2,
    "mentionPairs": 3, "gamma": 1.7, "modularity": 0.653,
    "dfCut": 1, "dfCutTop": ["widget"],
    "params": { "wTitle": 2.4, "wCode": 3.0, "wProse": 1.0, "dfCutoff": 0.32, "topK": 70,
                "simNeighbor": 0.08, "simGraph": 0.16, "minComm": 4, "maxComm": 36, "targetK": [7, 12] }
  },
  "clusters": [
    { "id": 0, "label": "Ledger · Posting · Invoice", "nameTerms": ["ledger", "posting", "invoice"],
      "terms": [["ledger", 0], ["posting.go", 1]], "hue": 205, "open": 23, "ref": 2, "ready": 3, "misc": false }
  ],
  "clusterKeywords": {
    "0": [ { "term": "reconcile", "weight": 1.0, "code": 0, "ticketIds": [3, 17, 40], "tw": [0.31, 0.17, 0.2], "open": 3 } ]
  },
  "nodes": [
    { "i": 0, "stem": "260101-epic-orchard-platform", "path": "ai-docs/tickets/todo/260101-epic-orchard-platform.md",
      "title": "…", "status": "todo", "cat": "epic", "cluster": 2, "phases": [1, 3],
      "odq": false, "blockedHeading": false, "lines": 140, "assignee": [],
      "kw": [["orchard", 0], ["grove/plan.go", 1]], "preview": "## Background\n…", "missing": ["260199-feat-gone"] }
  ],
  "edges": [ [12, 0, "parent", "", ""], [5, 9, "blocked-by", "", "#phase2"], [7, 3, "related", "note text", ""] ],
  "sim": [ [0, 14, 0.412, [["orchard", 0], ["grove/plan.go", 1]], 0] ]
}
```

- Field semantics:
  - `meta.ready`, `todo`, and `idea` count open tickets by status.
  - `node.i` is the array index. Nodes are ordered open first (status rank
    ready, todo, idea; then stem ascending), then refs (stem ascending).
  - `status` is one of `ready`, `todo`, `idea`, `done`, or `dropped`.
  - `cat` is the second dash-separated stem segment when the stem matches
    `^\d{6}-([a-z]+)-`, else `"other"`.
  - `phases` is `[count of TicketInfo.Phases with ResultPresent, len(Phases)]`
    (`ticketPhases`, `tickets.go#L874`).
  - `odq` comes from `containsOpenDecisionQueue` (`#L790`) and
    `blockedHeading` is `len(BlockedHeadings) > 0` (`#L849`).
  - `kw` holds the top 6 vector terms with their code flags.
  - `missing` lists the ticket's own dangling stems, sorted; the field is
    omitted when empty.
  - `edges` rows are `[from, to, rel, note, fragment]`. `from` is the ticket
    whose frontmatter declares the relation. A row exists only when both ends
    are included, is de-duplicated by `(from, to, rel)`, and is sorted by
    `(from, to, rel)`.
  - `sim` rows are `[i, j, score (3 decimals), sharedTerms, mention 0|1]` with
    `i < j`. A pair is included when `j ∈ top25(i)` or `i ∈ top25(j)`. Rows are
    sorted by `(i, j)`.
- Rounding: `gamma` has 1 decimal, `modularity` 3, `score` 3, `weight` 3. Floats
  are encoded with `strconv.FormatFloat(x, 'f', -1, 64)` after rounding, so the
  bytes are identical across runs within one build and host.
- Error and empty mapping:
  - An empty or missing board returns `200` with zero counts, `[]` arrays, and
    `{}` for `clusterKeywords`.
  - A source error returns `500` with the body `ticket read failed\n`.
  - An unknown or ended session, an agent registration, an unsupported source,
    or a gone record returns `404`.
  - A non-GET/HEAD method returns `405`.
  - A LAN viewer without the token gets `401` from the outer guard.

**Edge cases**

- 0 tickets: the empty payload. 1–7 open tickets: a single cluster (or Misc
  only), and no Louvain.
- An open ticket with no edges and low similarity goes to Misc. Misc is never
  split, absorbed into, or used as a γ-scoring community.
- Cycles in `parent` or `blocked-by` (2-cycle, 3-cycle) produce plain edges;
  nothing in the build walks ancestry, so there is no recursion. Self-references
  are dropped.
- Dangling refs go to `meta.dangling` and `node.missing`, never to `edges`.
- Malformed frontmatter (no closing fence) parses as no frontmatter through the
  tolerant `frontmatterFromText`. The title falls back to the H1, then the stem,
  and the ticket is still included.
- A non-stem `parent:` value, a list-form `related:`, and a `stem#phaseN`
  `blocked-by` are all normalized by the existing parsers (`relatedEntries`
  `#L907`, `blockedByEntries` `tickets_deps.go#L39`).
- A duplicate stem in two directories: the most-open copy wins.
- Unicode titles and bodies: Hangul particle stripping, CJK bigrams, and
  invalid UTF-8 are covered by tests.
- A ticket rewritten within 2 s: not cached and reread next time, and no memo is
  stored.
- Concurrent requests: builds are serialized and share the memo.
- A board on an unreadable volume: a 500 with the generic body, and no path in
  the response.

**Verification**

- `go test ./internal/wsdoc -run 'TicketMap' -count=1`:
  - **Text:** EN stopword and noise filtering; camelCase splits; Hangul
    particle stripping (`화이트보드를` → `화이트보드`); Katakana and Kanji
    bigrams; code-span extraction (path segments, basename with and without
    extension, `#id`/`.class` markers, dotted identifiers, URL, hash, and stem
    rejection); section weighting ×1.4 and ×0.5; lemma chains (`queries` →
    `query`, `mapped` → `map`).
  - **Cluster:** on the fixture —
    - every planted topic dominates one cluster: ≥ 80% of its tickets share a
      cluster, and that cluster's `nameTerms` contain a planted title word;
    - the Korean topic's label contains a Hangul term, and the Japanese
      topic's label contains a Kanji or Katakana bigram;
    - `widget` is in `meta.dfCutTop` and in no vector;
    - the 5 isolated tickets are in Misc;
    - the 3 body-only mention pairs carry `mention=1` and a score ≥ 0.18;
    - every `clusterKeywords` term is absent from its cluster's `nameTerms`;
    - at most 2 code terms per soup;
    - `K` lies within `TARGET_K`, plus Misc.
  - **Determinism:** two builds, and a build with `GOMAXPROCS=1` versus 8, give
    byte-identical JSON. A fixture copy whose directory entries were created in
    reverse order also gives byte-identical JSON.
  - **Edge cases:** an empty board; a missing board (`ErrNotExist`); 1 ticket;
    7 tickets (one cluster); cycles; self-reference; dangling refs; broken
    frontmatter; a `#phase2` fragment preserved in `edges`; a duplicate stem
    across `todo/` and `.done/` (the open one wins).
  - **Touch test** (`tickets_map_touch_test.go`), counting `readTicketFile`,
    `lstatTicketFile`, and `readTicketDir` calls through seams:
    - a cold build gives opens = included tickets, listings = 3, and closed
      lstat ≤ 2 × distinct referenced closed stems;
    - a warm rebuild with mtimes backdated past the racy window gives
      **opens = 0** and the same listing and lstat bounds;
    - touching one open ticket gives opens = 1, and the fingerprint changes;
    - touching one unreferenced `.done` file changes nothing: 0 opens, 0 lstat
      of it, and the fingerprint is unchanged;
    - no call ever lists `.done/` or `.dropped/`.
  - **Cache interop:** a plain `TicketsList` after a map build returns
    `TicketInfo` equal to before (the `doc` field is invisible). A map build
    after a plain listing reads each cached open ticket once to fill `doc`,
    then zero times after that.
  - **Bench:** `BenchmarkTicketMapBuild` on `n=300` (cold and warm-memo). Record
    ms/op in the phase Result as supporting evidence. The target is a cold
    build ≤ 400 ms on a dev machine; a warm memo hit < 1 ms.
- `go test ./internal/wsdashboard -run 'TicketGraph|GuardedRoutes|Access' -count=1`:
  - `ticket_graph_test.go` mirrors `TestTicketIdeasDemandRoute`
    (`ticket_ideas_test.go#L26-L91`):
    - opening the page reads no graph;
    - a local viewer and a LAN viewer with the token get 200 with the body
      returned verbatim and `no-store`;
    - an unknown session gets 404, POST gets 405, and LAN without a token gets
      401, all without reaching the source;
    - a source error gives 500 with `ticket read failed\n`;
    - `found=false` gives 404;
    - the agent forwarder refuses, and an unsupported source refuses.
  - The `guardedRoutes` entry is exercised by the existing access tests.
- `go test ./internal/mcp -run 'TicketGraph' -count=1`: `ReadTicketGraph`
  resolves the record root, passes it to the `ticketMapJSON` seam, maps
  `ErrNotExist` to the empty payload, and returns `ok=false` for an unknown
  registration.
- Shipped-surface check, `TestTicketMapSourcesNameNoRepositoryTickets` in
  `wsdoc`: the new shipped `.go` files (`wsdoc/tickets_map.go`,
  `tickets_map_text.go`, `tickets_map_cluster.go`,
  `wsdashboard/ticket_graph.go`, `mcp/dashboard_ticket_graph.go`) contain no
  match of `\b\d{6}-(feat|bug|refactor|chore|research|epic)-[a-z0-9-]+` and no
  `ai-docs/tickets/(ready|todo|idea|\.done|\.dropped)/\d`. Test-only fixture
  code (`*_test.go`, `wsdoc/wsdoctest/`) is out of scope because its invented
  stems are intentional.
- Payload size on the `n=170` fixture: under 600 KB (asserted), recorded in the
  Result next to the v5 reference (735 KB for 290 nodes with previews).

### Result (e24588ac7) - 2026-11-20

- **Landed** (`59bde59ba..e24588ac7`):
  - spine `wsdoc/tickets_map.go`: load, targeted ref lookup, fingerprint, memo
    with a 16-root LRU, payload types, `EmptyTicketMapJSON`;
  - `tickets_cache.go`: `doc` field, `scanStatusDirWith`, the
    `readTicketDir`/`lstatTicketFile` seams, and `listStatusDirEntries`;
  - text engine: `tickets_map_text.go`, plus `tickets_map_words.go` for the
    tokenizers and stoplists;
  - cluster engine: `tickets_map_cluster.go`, `tickets_map_louvain.go`,
    `tickets_map_naming.go`;
  - the synthetic fixture `wsdoc/wsdoctest/ticketmap.go`;
  - the route: `wsdashboard/ticket_graph.go`, the `tickets/graph` case, the
    agent forwarder, and `mcp/dashboard_ticket_graph.go`;
  - the tests listed under Verification.
- **Verification:**
  - `go test ./internal/wsdoc -count=1`: ok, including the TicketMap text,
    cluster, determinism (`-race` run separately), touch, and surface tests.
  - `go test ./internal/wsdashboard -run 'TicketGraph|GuardedRoutes|Access'`:
    ok.
  - `go test ./internal/mcp -run TicketGraph`: ok.
- **Touch counts** on the n=170 fixture: a cold build does 179 opens (170 open
  + 9 refs), 16 lstats (≤ 2 × 9), and 3 listings. A warm build does 0 opens and
  the same lstat and listing counts. No listing of `.done/` or `.dropped/`
  happens.
- **Clusters** on the fixture: K = 10 within `[7, 12]` plus Misc. Every planted
  topic is 100% co-clustered. The Korean label is Hangul and the Japanese label
  is a Kanji bigram.
- **Bench** (`BenchmarkTicketMapBuild`, n=300, Apple M3 Max): cold about
  25 ms/op (target ≤ 400 ms); warm memo about 1.0–1.2 ms/op. The warm memo
  misses the < 1 ms target. Its time is the 3 listings plus the about 186 stat
  calls the file-touch budget requires; the hashing and memo lookup are a small
  part of it.
- **Payload** at n=170: 311,356 bytes (179 nodes, 1,491 sim rows), under the
  600 KB assertion. The v5 reference is 735 KB for 290 nodes.
- **Decisions:**
  - `ledger` is ws convention vocabulary in `enStop`, so the fixture's billing
    topic uses `billing/invoice.go`.
  - Topic sizes in the fixture are flattened to 13–19, because the γ-score
    entropy term otherwise splits an outsized topic.
  - `blocked-by` fragments are normalized to `#phaseN`.
  - Edges declared by closed refs are included when both ends are included;
    `missing` is computed for open tickets only.
  - `cluster.terms` holds the up-to-8 name candidates.
  - `clusterKeywords` keys exist only for clusters with a non-empty soup.
  - Every other contract reading is in the commits' AI Context.
- **Pre-existing failures** reproduced on unchanged HEAD, outside this ticket:
  `TestTrackerStoreRegistryUI` (`closeOrgEditor` ReferenceError) and an
  `internal/mcp` full-package hang in
  `TestDashboardTransientBoardLockRetainsIndexAndCaches`.

### Phase 2: `#thandle` Entry, Full-Viewport Overlay Shell & View B (Swimlane Matrix + 3-Tier Hierarchical Click-Focus)

**Files to create or modify**

- Modify `assets/dashboard-drawer.js`:
  - create `#thandle` right after `#shandle` (`#L31-L33`);
  - add `thandleSync()`, called after `ticketIdeaSync()` in `drawerSync`
    (`#L117`);
  - extend `drRoom()` (`#L278-L297`);
  - observe `thandle` in `drRo` (`#L510`);
  - add `#thandle` to the outside-click exclusion selector (`#L487`).
- Create `assets/dashboard-ticketmap.js` (overlay shell, shared state, View B)
  and `assets/dashboard-ticketmap.css`.
- Modify `pages.go` (`dashboardScriptFiles`, `dashboardStyleFiles`) and
  `asset_source_test.go#L20-L21`.
- Modify `assets/dashboard-lifecycle.js#L738` (the `dim()` selector) and
  `assets/overview.js#L50` (the `classify()` palette selector).
- Add `assets/lucide/icons/network.svg`.
- Add the Phase 2 `js.tmap.*` keys to all 11 tables in
  `internal/wsi18n/tables/*.json`.
- Create the tests `testdata/ticketmap-state.test.cjs`,
  `testdata/ticketmap-hier.test.cjs`, `ticketmap_model_test.go`,
  `testdata/ticketmap.browser.cjs`, `ticketmap_browser_test.go`, and
  `ticketmap_assets_test.go` (the Go asset and scope checks). In the browser
  fixture, a fake source wired like `ticketIdeasFake` implements
  `ReadTicketGraph` by calling `wsdoc.TicketMapJSON` on the copied synthetic
  board.

**`#thandle` DOM, CSS, and placement**

- DOM: `<button type="button" id="thandle" class="dr-handle d-tm-handle" aria-controls="tmback" aria-haspopup="dialog" aria-expanded="false" hidden>` holding `luIcon("network")`.
  `title` and `aria-label` are `wsT("js.tmap.open")`. It is inserted with
  `drhandle.parentNode.insertBefore(thandle, shandle)`, so DOM order is git
  region, `#thandle`, `#shandle`, `#drfold`, `#drhandle`.
- `thandleSync()`: `var on = !!$("p-tickets"); if (thandle.hidden === on) { thandle.hidden = !on; drRoom(); }`.
  When `#p-tickets` disappears after a panels swap while the overlay is open,
  the overlay stays usable until it is closed.
- `drRoom()` math. `w`, `gw`, and `sw` are unchanged; `tw` is
  `thandle && !thandle.hidden ? Math.ceil(thandle.getBoundingClientRect().width) : 0`.

  ```
  so        = w && sw ? w + 6 : 0                 // --shr (unchanged)
  afterS    = sw ? so + sw : w
  to        = tw && afterS ? afterS + 6 : 0       // --thr (new)
  beforeGit = tw ? to + tw : afterS
  off       = beforeGit && gw ? beforeGit + 6 : 0 // --ghr
  all       = gw ? off + gw : beforeGit           // --drhw
  ```

  `--thr` is written with `--shr` on every call. `--drhw` and `--ghr` keep the
  change-guarded writes.
- CSS: `.d-tm-handle { right: calc(var(--dpad) + var(--thr, 0px)); width: 35px; padding: 2px 0 0; justify-content: center; }` and
  `.d-tm-handle .li { width: 17px; height: 17px; }`. The `.dr-handle` base
  rule (`dashboard-drawer.css#L145`) supplies the 34px hanging shape, and the
  `@media (max-width: 640px)` padding override (`#L459-L463`) applies through
  the shared class. While the overlay is visible,
  `aria-expanded="true"`.
- Click: `ticketMap.toggle()`. When the drawer is open, the drawer's
  outside-click capture would otherwise close it, so `#thandle` joins the
  exclusion list and `ticketMap.open()` calls `drSetOpen(false)` itself.

**Overlay shell**

- DOM, appended to `body` once on the first open:
  `<div id="tmback" class="d-tm-back" role="dialog" aria-modal="true" aria-labelledby="tmtitle" hidden>`.
  It contains:
  - the top bar `.d-tm-bar` (46px), holding:
    - the brand `#tmtitle` (`js.tmap.title`);
    - a segmented view switch `#tmviewb` / `#tmviewc` (`js.tmap.view.b` `1`,
      `js.tmap.view.c` `2`). The C tab is present but disabled in Phase 2 and
      enabled in Phase 3;
    - the search `#tmq` (`placeholder` `js.tmap.search`), with a hit count via
      `wsTn("js.tmap.hits", n)` and a `/` kbd hint;
    - status pills (Ready, Todo, Idea, ✓ Done ref, ⊘ Dropped ref) with counts,
      each toggling `aria-pressed`;
    - the cluster filter button `#tmclusters`, which opens a checklist popover
      with one row per cluster (hue dot + label + count) and an "All clusters"
      row;
    - a keyword chip `#tmkw` (hidden until a keyword filter exists);
    - stats (`js.tmap.stats`, tooltip `js.tmap.stats.tip`);
    - Refresh `#tmrefresh` (`refresh-cw` icon, `js.tmap.refresh`);
    - Close `#tmclose` (`x` icon, `js.tmap.close`);
  - the stage `.d-tm-stage` with `#tmvb` (View B, a scroll container) and the
    inspector `#tminsp` (`--d-tm-insp: 340px`);
  - the Full View modal `#tmfull`.
- State object `S` (exposed for tests as `window.__tmap` only when
  `root.hasAttribute("data-test-hooks")` or `?tmaptest=1`; otherwise not
  exposed). `S` holds:
  - `view`, `sel` (node index or null), `hist`, `hp`;
  - `q`, `hits`;
  - `st` (5 booleans, all on by default), `cl` (a Set of hidden cluster ids), `kw` (soup item or
    null);
  - `simTh` (default 15), `simUnlinked` (false), `simRefs` (true), `inspB`
    (false);
  - `collapsed` (a Set of lane ids);
  - `edge` and `simEdgeTh`, reserved for Phase 3.
- Persistence: `ws.ticketmap.view` holds `"B"` or `"C"`, written on every view
  switch and read on the first open; an absent or invalid value means `"B"`.
  `ws.ticketmap.prefs` holds JSON
  `{simTh, simUnlinked, simRefs, st, simEdgeTh, edge}`, written on change;
  invalid JSON or out-of-range numbers are ignored per field.
- Data flow:
  - **First open:** show a skeleton with `js.tmap.loading`, then
    `fetch("/s/" + rid + "/tickets/graph", { credentials: "same-origin", cache: "no-store" })`.
  - **Responses:**
    - `404` → `js.tmap.unavailable`;
    - any other non-OK → `js.tmap.error` with a Retry button;
    - `401` is also handled by the page's `loginResponse`;
    - a body with zero nodes → `js.tmap.empty`.
  - **Derived graph:** a per-node map `X` of
    `j → {rels:Set, notes:[{note, side, rel}]}` with inverse relations
    `child`/`blocks`/`related`, `simMap`, a `sim` list sorted descending and
    capped at 25, `deg` (the count of distinct explicit-relation neighbors; similarity
    neighbors do not count), `blockedOpen`, and a lower-cased search haystack of
    title + stem + kw + cluster label + preview.
  - **Refresh:** refetch, rebuild, re-select by stem (dropped when the stem is
    gone), keep filters, history (filtered by stems that still exist), and
    view, and keep the scroll position when the cluster count is unchanged.
  - **Refresh failure:** a failed or 404 Refresh keeps the retained graph and
    state, shows a transient `js.tmap.error` notice with Retry, and never
    replaces a loaded graph with the error state.
  - **Reopen:** shows the retained state without a fetch.
- **Close:** set `hidden`, remove the keydown listener, return focus to
  `#thandle` when the overlay held focus, and set `aria-expanded=false`.
- **Esc layering**, the first match wins:
  1. Full View modal open → close it;
  2. cluster popover open → close it;
  3. focus inside `#tmq` with text → clear the query;
  4. View C (Phase 3): Ego Ring → exit, then keyword filter → clear, then
     selection → deselect;
  5. View B focus mode → back to the matrix;
  6. otherwise close the overlay.
- **Keys:** `1`/`2` switch views; `/` focuses search; Enter in the search
  selects the first hit by status order (ready → dropped) and focuses it;
  `←`/`→` (and Alt+`←`/`→`) move through focus history in View B focus;
  Ctrl/Cmd+K and Ctrl+Space focus the map search.
- **Focus management:** on open, focus `#tmq` when it is empty on the first
  open, otherwise the last focused overlay element. Tab stays inside the
  overlay: a `focusin` listener redirects focus that escapes it back to the
  close button.

**View B matrix**

- Header: a sticky `.d-tm-mhead`, `grid-template-columns: var(--d-tm-cols)`
  with `--d-tm-cols: 212px minmax(0,.78fr) minmax(0,1fr) minmax(0,1.3fr) minmax(0,1.3fr)`.
  Its cells are tools (`js.tmap.lane.expandall` / `js.tmap.lane.collapseall`),
  `js.tmap.col.ref`, Ready, Todo, Idea, each with a count.
- One lane `.d-tm-lane` per non-empty cluster, in cluster order. The lane
  header is sticky at `top: 44px`, with a 4px hue bar, and holds:
  - the label: `cluster.label`, or `js.tmap.cluster.misc` when `misc`, or
    `js.tmap.cluster.unnamed` with id+1 when the label is empty;
  - counts `js.tmap.lane.open`, `js.tmap.lane.ref`, and
    `js.tmap.lane.ready` (shown when non-zero);
  - up to 6 term chips (`code` terms in mono with the sim tint).

  Clicking the header collapses the lane into colored status dots.
- Cells use `grid-template-columns: repeat(auto-fill, minmax(146px, 1fr))`
  (`140px` for refs). Card order: `deg` descending, then phase total
  descending, then stem ascending. Refs list done before dropped. Each lane
  shows at most 5 refs, with a `js.tmap.refs.more` toggle; a search shows all
  of them.
- Card `.d-tm-card` has exactly two children:
  - `.d-tm-ctop`: the status dot (or `✓`/`⊘`), the category tag, and
    indicators — `⛔` when an open blocked-by exists (`js.tmap.ind.blocked`),
    `B!` for `blockedHeading`, `ODQ`, a phase bar with `d/t`, and `↔deg`;
  - `.d-tm-ctt`: the title with `-webkit-line-clamp: 3`.

  There is no excerpt element. Refs are dimmed.
- Hovering a card lights its explicit neighbors (`.d-tm-lit`) and fades the
  others to `.5`.
- Filtering:
  - a card is shown when its status is on, its cluster is not hidden, it
    matches every whitespace-separated query term in its haystack, and the
    keyword filter is null or contains it;
  - a lane with no shown card is hidden;
  - search hits get a ring.

**View B click-focus**

- Click a card (or pivot) → `focus(i)`:
  - remember the matrix scroll, detach the matrix from the DOM, and render
    `.d-tm-focus`;
  - history: push the ticket unless it is already current, truncating forward
    entries; the breadcrumb trail shows the last 5 entries.
- Focus bar (sticky):
  - Back (`js.tmap.focus.back` + `Esc` kbd), Prev and Next (disabled at the
    ends), and the trail;
  - the inspector toggle `#tmfinsp` (`aria-pressed`, default false).
- Tier membership. The bucket of a linked ticket is the first relation it
  carries in the order parent, blocked-by, child, blocks, related. Within a
  group, sort by status order, then stem.
  - Tier 1 holds the parent group and the blocked-by group, plus an ancestry
    breadcrumb of up to 3 ancestors, root first. The breadcrumb walks `parent`
    links with a `seen` set, so cycles terminate.
  - Tier 2 is the focal card `#tmhero` (`width: min(800px, 100%)`), plus a
    right column `#tmlat` of related tickets when any exist.
  - Tier 3 holds the child group and the blocks group.
  - A ticket with no links shows `.d-tm-fempty` (`js.tmap.focus.nolinks`)
    instead of the tiers.
- The focal card holds:
  - the eyebrow (`js.tmap.focus.eyebrow`), the title, and the stem;
  - status, cluster, and category badges, the phase bar, `⛔`, `ODQ`, the line
    count, and `missing` chips (`wsTn("js.tmap.missing", n)` with the stems as
    a tooltip);
  - 6 kw chips;
  - actions: `js.tmap.focus.toc` and `js.tmap.focus.ego` (both hidden in Phase
    2 and enabled in Phase 3), and `js.tmap.focus.full`.
- Group caps (the overflow behind a `+N` toggle: `js.tmap.more` /
  `js.tmap.less`): parent 4, blocked-by 4, child 8, blocks 4, related 4.
- Card `.d-tm-xc` holds relation pills, a status badge, a cluster pill (only
  when the cluster differs), `≈ N%` (when the similarity is ≥ 8%), a children
  count, and the title clamped to 3 lines. Related cards also get one
  relation-note line (`js.tmap.note.out` / `js.tmap.note.in`).
- **Balanced multi-row layout** (`layoutHier`):
  - `W1` is the measured width of the first grid column;
  - card width `cw = min(232, floor((W1 − 40·(groups−1))/groups))`;
  - grow the column count of the group with the most rows while the total
    width (`Σ cols·cw + (cols−1)·16 + 40·(groups−1)`) fits `W1`;
  - then rebalance each group to `ceil(n / ceil(n / cols))` columns, so 5
    cards become 3+2 rather than 4+1;
  - n counts visible cards and is capped at 99.
- **Connectors**: an absolutely positioned `<svg id="tmhwire">` behind the
  cards, re-measured from `getBoundingClientRect` on every layout and on
  `ResizeObserver` of the scroll container. Paths are rounded orthogonal
  polylines with corner radius ≤ 7. Each path carries `data-from`, `data-fs`,
  `data-to`, `data-ts`, and `data-g`.
  - Tier 1: from each card's bottom center down to a bus at the midpoint
    between the tier bottom and the focal top, then along the bus to a trunk x
    clamped to `[focal.left+28, focal.right−28]`, then down into the focal
    top. A card on a lower wrapped row instead leaves from its left edge (−8px
    jog).
  - Tier 3 mirrors Tier 1, from the focal bottom to each card's top center, or
    into the left edge for later rows.
  - Related: from the focal right edge at its vertical center to a dashed bus
    26px left of the related column, then into each card's left edge.
  - Markers: parent and children use a primary-tinted triangle, blocked-by and
    blocks a `--wb-warn` triangle, related a dot. Labels such as `↓ parent`,
    `↓ blocked-by`, `↓ children N`, `↓ blocks N`, and `↔ related N` are placed
    beside the trunks. The words "children" and "blocks" are localized
    (`js.tmap.rel.children`, `js.tmap.rel.blocks`); field names stay verbatim.
  - Expanding `+N` re-runs `layoutHier` and the wires.
- **Semantic section** `#tmsem`:
  - header: `js.tmap.sim.title` and `js.tmap.sim.sub`; a count; a threshold
    slider `#tmsemth` (range 10–60, step 1, default 15, with a `%` readout); a
    toggle between `js.tmap.sim.all` and `js.tmap.sim.unlinked`; and a checkbox
    `js.tmap.sim.refs`;
  - grid `#tmsemgrid`: `grid-template-columns: repeat(auto-fill, minmax(340px, 1fr))`.
  - Each card `.d-tm-sc` (`data-pivot`, `data-pct`) has a `%` block with a bar,
    and a body of exactly three rows: the badges (status, cluster pill,
    `js.tmap.sim.new` or `js.tmap.sim.old` with its relations), the title
    clamped to 3 lines, and chips (`js.tmap.sim.mention` first when the pair is
    a mention, then the shared terms).
  - Sorted by score descending. An empty result shows `js.tmap.sim.none`
    (filled with the threshold). Clicking a card pivots.
- **Inspector** (opt-in in focus mode): status, cluster, category, phase,
  flags, assignee, size, the explicit links ordered by relation and clickable,
  the top 8 similar tickets, and a Full View button. Its minimize toggle
  collapses it to 38px. Opening or closing it re-measures the wires.

**Full View modal and the file-card hand-off**

- `#tmfull` (`role="dialog"`, `aria-modal`) holds a header with the title, a
  `js.tmap.full.card` button (rendered only under the `(26)` predicate) and
  `js.tmap.full.close`. The body holds badges, `<stem>.md`, the line count, the
  rendered preview, and a `js.tmap.full.truncated` footnote when the preview
  ends in `…`.
- The renderer escapes everything first, then handles only these: fenced
  code → `<pre>`; `##` → `<h4>`; `###`–`######` → `<h5>`; `- `/`* ` →
  `<li>`; inline code; `**bold**`; other non-empty lines → `<p>`. Nothing else,
  so no HTML injection is possible.
- The file-card click runs the `(26)` sequence. A backdrop click or Esc closes
  only the modal.

**Narrow, touch, and theme**

- At ≤ 1100px: `--d-tm-cols: 160px repeat(4, minmax(0,1fr))`, card minimum
  120px. In focus, `.has-lat` collapses to one column and Related renders as a
  wrapped row between Tier 2 and Tier 3, wired from the focal bottom with dashed
  connectors. The anchors are recorded as `fs=bottom`, `ts=top`.
- At ≤ 760px: lane headers span the full row above the four cells, and each
  cell shows a caption.
- Touch: everything works by tap (there is no hover-only control); hover
  lighting is skipped for `pointerType === "touch"`.
- Theme: both themes use only tokens; there is no hard-coded color except the
  `--d-tm-*` token values.

**Edge cases**

- A first open with storage denied falls back to defaults.
- An empty board (`js.tmap.empty`). The route returns 404 because the session
  ended or closed (`js.tmap.unavailable`).
- A LAN viewer: the route is read-only, and the file-card button appears only
  when `WsPreview` and the board exist.
- Refresh during a pivot. A selected ticket that disappears after Refresh
  returns to the matrix.
- A focused ticket whose ancestry is a cycle: the breadcrumb stops at the
  repeat.
- An epic with 11 children: 8 shown plus `+3`, in balanced rows. A ticket with
  0 links shows the empty explanation.
- A cluster whose label is empty is localized; the Misc lane comes last.
- A Korean or Japanese title clamps correctly (`word-break: break-word`).
- The drawer was open when the overlay opened (it is closed first).
- Maximized cards and the docked gallery sit underneath the overlay. In the
  framed shell, `data-fdim` is set while the overlay is open and cleared on
  close.
- Keyboard: `/`, Ctrl+K, and Esc never reach the page or the drawer while the
  overlay is open. A key typed into the search never switches views.

**Concrete verification**

- `go test ./internal/wsdashboard -run 'TicketMap|DashboardSourceAssembly|CSSClassesDoNotCollide|LucideIcons' -count=1` and
  `go test ./internal/wsi18n -count=1` (used keys present, every English key
  used, all 11 tables complete, placeholders match, plural pairs).
  `ticketmap_assets_test.go` asserts:
  - the `dashboardScriptFiles` and `dashboardStyleFiles` positions;
  - `ticketMap` is declared exactly once, and there are no top-level function
    declarations in the new files;
  - every class selector in `dashboard-ticketmap.css` starts with `d-tm-` and
    every id with `tm`;
  - `dashboard-lifecycle.js` `dim()` lists `.d-tm-back:not([hidden])`, and
    `overview.js` `classify` lists `.d-tm-back`;
  - `dashboard-ticketmap.js`, `dashboard-ticketmap.css`, and the Phase 2
    shipped Go edits contain no ticket-stem match (the Phase 1 regex; test
    fixtures excluded).
- Node (`ticketmap-state.test.cjs`):
  - prefs parse and fallback, with invalid JSON and out-of-range values;
  - the default view is B when absent and the last view otherwise;
  - the inverse relation map and bucket precedence;
  - search AND matching;
  - history push, truncate, and back/forward;
  - Esc layering order as a pure reducer over a state snapshot.

  Node (`ticketmap-hier.test.cjs`): `layoutHier`'s column solver (5 → 3+2,
  11 children at `W1` = 1200 and 800, cap 99) and the `drRoom` math for every
  combination of hidden and present handles.
- Playwright (`ticketmap.browser.cjs`) at 1600×960 and 1280×800, plus 900×700
  for the narrow checks, in both `data-theme="dark"` and `"light"`:
  1. Opening the page performs no `/tickets/graph` request. `#thandle` is
     visible next to `#shandle`; its right edge is exactly
     `#shandle.left − 6px` (±0.5), and the header right padding equals
     `--drhw + --tkhw + 6` (matching `dashboard-drawer.css#L456`, with
     `--tkhw` resolving to `0px` when no tracker handle is present). With the
     drawer handle hidden, `#thandle` still sits 6px left of `#shandle`.
  2. Clicking `#thandle` sends one request, the overlay covers the viewport
     (`getBoundingClientRect` equals the viewport), and `aria-expanded=true`.
     The first open shows View B.
  3. Every cluster lane renders, every node has a card, no card has an
     excerpt node (card children are exactly `d-tm-ctop` and `d-tm-ctt`), the
     title clamp is `3`, and there is zero horizontal overflow (document and
     `#tmvb`).
  4. Hover lights neighbors. `/` focuses the search; typing `orchard` shows a
     hit count and filters cards. Esc clears the query.
  5. Clicking the fixture epic: the matrix is removed from the DOM, the
     inspector is hidden with `display: none`, and `#tmvb` is full width.
     Upstream cards lie ≥ 20px above the focal card, downstream ≥ 20px below,
     and related ≥ 20px to its right. There are 0 excerpt nodes.
  6. The semantic grid is sorted descending, every `%` is ≥ 15, every card has
     ≥ 1 chip, and the grid has 4 columns at 1600 and 3 at 1280.
  7. There is one wire per visible hierarchy card, each with a marker and a
     length > 10px. Every endpoint lies within 2px of its anchor (the
     `verify.js` `anchorErr` rule: focal top and bottom by edge distance plus
     an x-range check; card centers on top, bottom, left, and right). This
     holds again after `+N` expansion and with the inspector open.
  8. Threshold 10 shows at least as many cards as 15. Unlinked-only shows 0
     `old` badges.
  9. Pivoting works through a semantic card, an upstream card, and a
     downstream card. Back and Forward and the `←` key restore the history.
  10. Full View opens with more than 50 characters of text, and Esc closes only
      the modal (focus is kept). "Open in dashboard file card" hides the
      overlay, a `.bcard` preview card for the ticket path exists and is shown,
      and clicking `#thandle` restores the same focus with no new request.
      Refresh sends exactly one new request and keeps the selection.
  11. Esc layering: in focus, Esc returns to the matrix, and a further Esc
      closes the overlay. The drawer's Escape handler never ran: the drawer
      open state is unchanged when the drawer was opened and closed by the
      overlay test.
  12. In a framed page (`?frame=1` inside the shell), the shell root has
      `data-fdim` while the overlay is open, and the overlay's top-left pixel
      element is inside the overlay (`elementFromPoint`).
  13. At 900px: no horizontal overflow, Related sits below the focal card, and
      wires stay within 2px.
  14. Zero console errors, page errors, or failed requests.
- **Side-by-side capture check:**
  - the test saves `b-<theme>-<WxH>.png` and `b-focus-<theme>-<WxH>.png` from
    the overlay;
  - it also saves the same states of the mockup, loaded from
    `file://…/ticket-map-v5-bc.html` at the same viewport (the mockup's own
    tab and theme buttons, its focus showcase replaced by its first
    `.tc` card with the most links), as `mock-b-*.png`, into
    `WS_TICKETMAP_SHOTS`;
  - the reviewer compares the pairs for lane, header, and card grammar, the
    3-tier composition, the connector style, and the semantic grid density;
    the Result records the comparison.

### Result (26647d12c) - 2026-11-20

- **Landed** (`500b23d66..26647d12c`):
  - `#thandle` in `dashboard-drawer.js`: `thandleSync`, `drRoom` `--thr` and
    `--drhw`, `drRo`, and the outside-click exclusion;
  - `dashboard-ticketmap.js`: the overlay shell, state `S`, data flow, the
    keyboard layering, the Full View modal, the file-card hand-off, and the
    view-host contract;
  - `dashboard-ticketmap-viewb.js`: View B's matrix, 3-tier focus, connectors,
    semantic grid, and inspector;
  - `dashboard-ticketmap.css`;
  - the `dim()` and `classify()` selectors;
  - the vendored `network.svg`;
  - `js.tmap.*` keys in all 11 tables;
  - the Node, asset, and browser tests.
- **Verification:**
  - `go test ./internal/wsdashboard -run 'TicketMap|DashboardSourceAssembly|CSSClassesDoNotCollide|LucideIcons|TicketGraph'`:
    ok.
  - `go test ./internal/wsi18n`: ok.
  - `node --test testdata/ticketmap-*.test.cjs`: 15/15 pass.
  - `TestTicketMapBrowser` (items 1–14): pass at 1600×960 and 1280×800 in
    dark and light, plus 900×700. It sees 179 nodes, 11 lanes, and exactly
    2 graph requests (first open + Refresh).
- **Captures:** compared `/tmp/tmshots-p2/b-*.png` vs `mock-b-*.png` and
  `b-focus-*.png` vs `mock-b-focus-*.png` (both themes, both viewports):
  match. The lane, header, and card grammar, the 3-tier composition, the
  connector style, and the grid density all agree. Differences are intended:
  - ToC and Ego are hidden until Phase 3;
  - the mockup's "Related N" column caption is not in the spec;
  - at 1280 the top-bar stats are ellipsized because the bar also carries
    Clusters, Refresh, and Close.
- **Decisions:**
  - View B lives in its own file, `dashboard-ticketmap-viewb.js`
    (`ticketMap.viewB = (function () { … })();`), placed right after
    `dashboard-ticketmap.js` in `dashboardScriptFiles`. This keeps every file
    under the 1000-line size guard.
  - While the overlay shows, every keydown is stopped; only consumed keys are
    `preventDefault`ed.
  - `window.__tmap` is set on the first open.
  - At 900px a wrapped related card is entered at its left edge (`ts=left`),
    the same later-row rule Tier 3 uses.
  - Additional keys: `js.tmap.ind.deg`, `ind.kids`, `ind.phase`, `sim.th`, and
    `insp.none`.
  - The drawer tests `TestSearchPaletteHandleGeometry` and
    `TestDrawerRoomResizeFrame` were updated for the third observed handle.
- **Pre-existing failures** reproduced on unchanged HEAD:
  `TestShellTopbarBrowser` (dark/760 `clipped:false`) and
  `TestPanelRowsBrowser` (agenda event timeout).

### Phase 3: View C (Constellation Graph + 3-Stage Semantic Zoom Keyword Soup + Ego Ring)

**Files to create or modify**

- Create `assets/dashboard-ticketmap-graph.js` (`ticketMap.graph`) and add it to
  `dashboardScriptFiles` right after `dashboard-ticketmap.js`, plus to
  `asset_source_test.go`.
- Extend `assets/dashboard-ticketmap.css` with the `d-tm-c*` rules.
- Edit `assets/dashboard-ticketmap.js` to enable the C tab, `js.tmap.focus.toc`,
  and `js.tmap.focus.ego`, and to route the shared state.
- Add the Phase 3 `js.tmap.c.*` keys to all 11 tables.
- Create the tests `testdata/ticketmap-graph.test.cjs`,
  `testdata/ticketmap-graph.browser.cjs`, and
  `ticketmap_graph_browser_test.go`.

**DOM**

- `#tmvc` contains a viewport `#tmvp` (`touch-action: none; user-select: none; cursor: grab`).
- Inside it, an SVG `#tmsvg` (`aria-label` `js.tmap.c.aria`) holds `<defs>`
  (the blocked arrow and the parent arrow) and a group `#tmworld` with the
  layers hulls → ego guides → edges → ego edges → nodes → hover ring →
  selection ring.
- An HTML label layer `#tmlbl` (`pointer-events: none`) holds:
  - ticket labels `.d-tm-lb`;
  - cluster headers `.d-tm-cb`;
  - soup words `.d-tm-sk` (`pointer-events: auto`);
  - ego sector captions `.d-tm-sect`.
- Overlay controls:
  - the toolbar `#tmctool`, top-left: edge toggles for Parent, Blocked-by,
    Related (default off), and Similarity ≥; the similarity threshold
    `#tmsimth` (range 15–60, default 24); an edge count; an Ego Ring button
    (disabled without a selection); the keyword chip;
  - the hint `#tmchint`, top-right (`js.tmap.c.hint`);
  - the ego bar `#tmegobar`;
  - the legend `#tmlegend`, bottom-left;
  - the zoom control `.d-tm-zoom`, bottom-right: −, a `%` readout, +, Fit, 1:1;
  - the tooltip `#tmtip`.

**Deterministic layout** (run once per payload; the seed is 42 with the v5
`mulberry32` `rng`):

- Node radius: open `{ready 7.5, todo 6.5, idea 5.5} + min(5, √deg·1.1) + (epic ? 2.5 : 0)`;
  refs 4.5. Cluster radius `CR = 34 + 30·√count`.
- Cluster centers:
  - clusters are ordered by size descending (ties by id), and cluster `ix`
    starts at golden angle `ix·2.39996` and radius `60·√(ix+0.5)·3`, scaled
    `x·1.7` and `y·0.8`;
  - 900 iterations with `al = 1 − it/900`. Overlapping pairs (`minD = CR_a +
    CR_b + 64`) separate by half the overlap. Affine pairs attract by
    `min(.04, W·.0025)·al·(d−minD)/d`, where `W` counts cross-cluster explicit
    links plus 2·s for each top-3 similarity ≥ .2;
  - damping `x·(1−.006·al)` and `y·(1−.022·al)`.
- Nodes:
  - initial position: random angle and `√rnd·CR·.7` from the cluster center;
  - `IT = 420` when N ≤ 400, else `max(120, floor(420·(400/N)²))`;
  - per iteration `al2 = .35 + .65·(1 − it/IT)`.
  - Pair repulsion: `(same cluster ? 2600 : 1200)/d²·al2`, skipped for
    cross-cluster pairs with `d² > 14400`; plus `(minD − d)·.5` when
    `d < r_a + r_b + 30`; capped at 30; split half and half. A coincident pair
    is jittered by the rng.
  - Springs on explicit links (weight 1, or .5 for related-only) and on top-3
    similarities ≥ .2 (weight s): target 58 within a cluster and 140 across,
    stiffness `.045` and `.006` respectively × `w·al2`.
  - Gravity to the cluster center `.012·al2`; containment radius
    `CR − r − 10`.
- Hulls:
  - 16 points around each visible node at `r+20`;
  - a monotone-chain convex hull;
  - a quadratic midpoint-smoothed path;
  - `hullTop` (center x, top y, bottom y, width) for the headers;
  - the polygon is kept for soup containment;
  - hulls are rebuilt on a filter change and after an Alt+drag.

**Rendering and semantic zoom**

- Camera `{x, y, k}`, applied as `matrix(k,0,0,k,x,y)` on `#tmworld`. Renders are
  batched to one `requestAnimationFrame`. `k` is clamped to `[0.18, 3.8]`.
- Node screen scale: `ns = k < .85 ? min(1.9, √(.85/k)) : 1`. It is applied to
  the radii when it changes by more than .04, which re-places the edges.
- Stages: `stage = ego ? 3 : k < .85 ? 1 : k < 1.35 ? 2 : 3`.
  `soupA = ego ? 0 : clamp((1.25 − k)/.4, 0, 1)`, × .45 when a selection
  exists.
- Stage 1 adds the class `d-tm-calm`: structural edges at opacity `.28`,
  similarity edges at `.22`, node cores at `.8`, hull strokes at `/.24`.
- Label placement uses a spatial hash with 80px cells and greedy AABB
  rejection. The toolbar, hint, legend, ego bar, and zoom control are
  pre-inserted as obstacles, padded 4px.
- Placement order:
  1. selected and hovered labels;
  2. cluster headers: try the full label above the hull, then the first term
     above, then the full label below, then the first term below, clamped 4px
     inside the viewport;
  3. ego sector captions;
  4. if `soupA ≥ .5`, the soup and then the node labels; else the node labels
     and then the soup.
- Node label priority:
  - selection `1e7`, hover `9e6`, ego `5e6 + rank`;
  - selection neighbors `6e6 + sim·1e3`, hover neighbors `5.5e6`;
  - keyword matches `4.5e6 + deg`, search hits `4e6 + deg`;
  - otherwise `(hub ? 2e4 + 200·deg : 0) + {ready 6e3, todo 2.5e3, idea 1.2e3, ref 400} + 60·deg`,
    where a hub is an epic or `deg ≥ 6`.
- Minimum priority to place a label: stage 1 `4e6` (so **zero ticket titles**
  without interaction), stage 2 `6e3`, stage 3 `0`. Maximum text length:
  stage 2 26, `k < 1.8` 46, otherwise 70, ego 60.
- Text width is fitted by binary search with a canvas `measureText` cache that
  is reset when the theme changes (a `MutationObserver` on `data-theme`).
- Labels:
  - `.d-tm-lb`: transparent background, no border, a `--halo` text shadow,
    12px; a framed pill (`1px` border, `--wb-card` surface) only with
    `.sel` or `.hov`;
  - cluster headers: borderless, a dot + label + open count.
- **Keyword Soup.**
  - Per cluster, take up to 8 `clusterKeywords` terms whose lowercased term is
    not in `nameTerms`.
  - Font size `round((11 + 5·nw^.7)·2)/2` px (11–16) and opacity
    `(.55 + .33·nw)·soupA`, where `nw` is weight/max.
  - Color `hsl(hue 52% var(--d-tm-sk-l))`, with `--d-tm-sk-l` 30% in light and
    80% in dark. Code terms are mono.
  - The anchor is the centroid of the carrying, visible tickets' settled
    positions, weighted `max(.15, tw)`.
  - Placement takes the first free offset from a candidate grid: dx steps of
    6px within `±(w·.8 + 24)`, dy steps of `h·.5` within `±h·2.6`. Candidates
    are sorted by `hypot(dx, 1.35·dy)`, cut to that ellipse, and capped at 220.
    The offset must stay inside the viewport by 2px, inside the cluster hull
    polygon (in world coordinates), and collision-free. At most 8 soup words
    are placed per cluster.
- **Tooltip:** placed at the cursor `+16/+14`. It flips left (`mx − w − 14`)
  when it would cross the right margin and above (`my − h − 12`) when it would
  cross the bottom, then is clamped with a 12px margin. Variants:
  - node: the title, stem, badges, `↔deg`, and the top 2 similar tickets;
  - soup word: the term, `wsTn("js.tmap.c.tip.tickets", n)`, the cluster pill,
    and `js.tmap.c.kwtip`;
  - similarity edge (9px hit stroke): `js.tmap.c.tip.sim` and the shared chips.
- **Edges:**
  - structural: parent as a plain line (child → parent); blocked-by as an
    arrow from the blocker to the blocked ticket; related dashed and bent
    `min(30, .12·d)`;
  - similarity: dotted and bent, a greedy pick by score descending with at
    most 3 per node at or above the threshold; the slider rebuilds them;
  - edge ends are trimmed to the node radius × `ns`.

**Interaction**

- **Wheel:** `preventDefault`; `dy = deltaY × (deltaMode 1 ? 32 : deltaMode 2 ? 400 : 1)`,
  clamped to ±320; then
  `zoomAt(mx, my, k·exp(−dy·(ctrlKey ? .008 : .0028)))`, which keeps the world
  point under the cursor fixed: `x' = mx − wx·k'`. Wheel cancels any running
  camera animation.
- **Pointer:**
  - `pointerdown` captures the pointer. A drag starts after 3px of movement.
  - The default drag (left or middle button, on any target including nodes and
    soup words) pans the camera by the raw delta.
  - Alt+left drag on a node, outside the Ego Ring, moves that node by
    delta/k, updates its edges and rings, and rebuilds the hulls on release.
  - A release without movement is a click. Two clicks within 340ms and 8px
    are a double click.
- **Hit priority:** when the target is a soup word but
  `document.elementsFromPoint` contains a node, the node wins. After that the
  order is label, soup word, similarity edge, background.
- **Click:**
  - node or label: select it (inside the Ego Ring, re-center the ring on it);
    a double click enters the Ego Ring;
  - soup word: the keyword filter. It sets `S.kw` and clears the selection,
    shows the chip `js.tmap.c.kw` with the term and `(n)`, adds `.d-tm-kwh`
    to the matching stars and fades the others to `.2`, and animates the camera
    (420ms) to fit the matches with the zoom capped at
    `max(min(k·1.35, 1.1), .9)`;
  - similarity edge: a pinned tooltip;
  - a background double click zooms ×1.8 about the cursor (260ms);
  - a background click deselects.
- **Hover:** a node lights its edges (opacity 1, wider) and its neighbors
  (`.d-tm-hvn`) and shows the framed label and the tooltip. A soup word
  highlights its stars softly.
- **Buttons:** ± animate ×1.35 about the viewport center (200ms). Fit uses
  padding 30, a top reserve of 64, a bottom reserve of 54, and `maxK` 1.6
  (280ms ease-out cubic). 1:1 sets `k = 1` about the center.
- **Touch:** a one-pointer drag pans. With two active touch pointers, the
  pinch scales `k` by the distance ratio anchored at the pointers' midpoint and
  pans by the midpoint delta. A double tap enters the Ego Ring. There is no
  node move on touch; this is documented in the hint.
- **Ego Ring** (`dblclick`, the `E` key, the toolbar button, or the focal
  card's `js.tmap.focus.ego` button):
  - Members: the explicit links of visible tickets, split up (parent,
    blocked-by) and down (child, blocks). Related tickets alternate left and
    right, balanced so the column heights match. The left column is up, a
    spacer, related-left; the right column is related-right, a spacer, down.
  - The outer ring holds up to 10 non-linked visible similarity neighbors with
    `s ≥ min(simEdgeTh, 12)%`, alternating right and left.
  - Geometry, with `SP = 32`:
    - `R1 = clamp(W·.1, 90, 190)` and `R2 = R1 + clamp((W/2 − R1)/2 + 14, 170, 340)`;
    - `Ry1 = max(R1·.9, (rows−1)/2·SP·1.22 + 30)` and
      `Ry2 = max(Ry1 + 70, (outerRows−1)/2·SP·1.22 + 40)`;
    - each item sits at `x = ±R·√max(.12, 1 − (y/Ry)²)`.
  - Rendering:
    - dashed guide ellipses;
    - highlighted ego edges;
    - `N%` label prefixes on the outer ring;
    - sector captions (`js.tmap.c.sect.up`, `.down`, `.rel`, `.sim`);
    - the soup, hulls, and non-members are hidden;
    - the toolbar is replaced by the ego bar, which shows the title and
      `js.tmap.c.egoexit` with an `Esc` kbd.
  - Movement: a 420ms ease-in-out cubic tween, and the camera fits the ring
    with a top reserve of 70, a bottom reserve of 60, and `k ≤ 1`.
  - Esc exits: positions tween back, the selection is kept, and the camera
    centers it at `k ∈ [.9, 1.2]`.
- **Shared state:**
  - B → C (`2`, the tab, or `js.tmap.focus.toc`) keeps the selection. The
    camera centers it at `k ≥ .95`, and the selected label shows framed.
  - C → B (`1`) with a selection opens B's focus on it with the inspector
    still closed.
  - The search, status, cluster, and keyword filters apply to both views. In
    C, search hits get `.d-tm-qh` while others fade to `.18`; filtered nodes,
    edges, and hulls are hidden.
  - In C the inspector shows whenever a selection exists, as in v5.
  - The last view goes to `ws.ticketmap.view`, and `simEdgeTh` and `edge` go to
    `ws.ticketmap.prefs`.

**Edge cases**

- 0 nodes: the empty state; layout is never called.
- 1 node: it is centered and Fit clamps to `maxK`.
- A cluster with 1–2 visible nodes gets a hull from its ≥ 3 points (16 per
  node); with no visible nodes there is no hull and no header.
- All soup terms may be filtered out.
- A keyword filter whose tickets are all status-filtered: the chip shows `(0)`
  and the camera does not move.
- Selecting during the Ego Ring re-centers the ring.
- Refresh while in C rebuilds the layout. The camera is kept when the node-stem
  set is unchanged, and fitted otherwise.
- A resize recomputes `W`/`H` and re-places the labels without a new layout.
- A theme switch resets the measurement cache and re-tints the soup.
- At narrow widths (≤ 900px) the hint is hidden and the legend collapses to
  icons. The ego radii follow `W`, so the four label columns still fit.
- Large N: the iteration scaling keeps the 300-node layout within budget.
- A wheel event with `deltaMode` 2 is clamped. Trackpad pinch (`ctrlKey`
  wheel) uses the faster factor.
- A pointer lost mid-drag (`pointercancel`, `lostpointercapture`) ends the drag
  without a click. The context menu and middle-click auxclick are suppressed
  inside `#tmvp` only.

**Concrete verification**

- `go test ./internal/wsdashboard -run 'TicketMap|DashboardSourceAssembly|CSSClassesDoNotCollide' -count=1` and
  `go test ./internal/wsi18n -count=1`: the Phase 2 asset assertions extended to
  the graph file and the `d-tm-c*` classes.
- Node (`ticketmap-graph.test.cjs`):
  - layout determinism: two runs on the same payload are bitwise-equal
    positions, and a different seed differs;
  - containment: every node lies within `CR − r − 10` of its center;
  - `zoomAt` anchor drift is < 1e-9 for `deltaMode` 0, 1, and 2 at mixed
    anchors, and `k` is clamped;
  - the soup offset generator's order and its 220 cap;
  - `inPoly` and the convex hull on degenerate inputs (collinear, duplicate
    points);
  - the tooltip clamp and flip for all four corners;
  - Ego geometry: column balance and ellipse placement for 0–20 members;
  - the iteration scaling formula;
  - the hit-priority resolver (node over soup).
- Playwright (`ticketmap-graph.browser.cjs`) at 1600×960 and 1280×800, dark and
  light, over the fixture, porting every `verify.js` View C check with these
  thresholds:
  1. Opening with `ws.ticketmap.view = "C"` shows C, and Fit gives `k < .85`
     (stage 1).
  2. Zero `.d-tm-lb.on` labels. The visible soup count is ≥ max(20,
     2 × cluster count), with 0 overlaps among labels, headers, and soup, and
     0 overlaps with the toolbar, hint, legend, ego bar, and zoom control.
  3. The soup sits on its centroid, recomputed independently from the payload
     and the settled positions: median displacement < 30px and maximum
     < 110px.
  4. Soup styling: transparent background, 0px border, a text shadow, a font
     size within [11, 16] that varies, and opacity within [.54, .89].
     Cluster headers are borderless.
  5. The calm backdrop: structural edge opacity .28 ± .011 and similarity
     .22 ± .011.
  6. Hovering a node with `deg ≥ 4` lights its edges at opacity 1 and more
     than one neighbor, and frames its label with a 1px border; the tooltip is
     on.
  7. Hovering a soup word highlights exactly its n stars. Clicking it shows
     the chip text matching `^<localized prefix> ?<term> ?\(n\) ?×$`, highlights
     n stars, and moves the camera by more than 20px. Esc clears it.
  8. Pan:
     - an empty-canvas drag of (−90, −50) moves the camera by ±1.5px with `k`
       unchanged;
     - a drag starting on a soup word pans, and no filter is set;
     - a drag starting on a node pans and the node stays put (< .01) with no
       selection;
     - Alt+drag moves the node by delta/k (±.6) with the camera fixed;
     - a middle-button drag pans.
  9. Wheel: `deltaMode` 0 with −120 gives zoom-in > 1.3× and drift < 1e-9;
     `deltaMode` 1 with 3 gives zoom-out > 1.25× and drift < 1e-9;
     `deltaMode` 2 with 1 zooms out, clamps at ≥ .18, and drifts < 1e-9; a
     real `page.mouse.wheel` zooms more than 1.2×.
  10. Buttons: + gives more than 1.3× and a readout of `round(k·100)%`; −
      returns within 1e-6; 1:1 gives `k = 1 ± 1e-3` and `100%`.
  11. At zoom levels `{fit, .8, 1.0, 1.2, 1.4, 1.8, 2.6}`, 0 label collisions
      and 0 toolbar overlaps everywhere.
  12. Soup alpha: `.8` → 1, `1.0` → (.55, .7), `1.2` → < .15, `1.4` → 0 with
      0 soup. Titles: 0 at `.8`, > 0 at `1.0`, and more at `1.4` than at
      `1.0`. Stage 2 at `1.0` and stage 3 at `1.4`.
  13. At `k ≥ 1.4`: more than 15 borderless plain labels, transparent
      background, 0px border, 0 collisions. The hover tooltip at the right edge
      stays inside the viewport with ≥ 4px margins on all sides, and near the
      bottom edge it flips above and stays inside.
  14. Ego Ring: a double click on the fixture epic gives `S.ego` set, more
      than 3 members, all of them labeled, 0 collisions, and 0 soup. Esc exits
      and keeps the selection. `E` re-enters.
  15. Sync: B focus → `2` shows C with the same selection and a framed
      selected label. C selection → `1` shows B focus with the inspector
      closed.
  16. Touch context (`hasTouch`, 1280×800): a one-finger drag pans, and a
      two-finger pinch through CDP `Input.dispatchTouchEvent` zooms about the
      midpoint with drift < 1px.
  17. Zero console errors, page errors, or failed requests.
- Performance (recorded in the Result): the layout time for the 170-node
  fixture and a 300-node fixture from `performance.now()` around the layout
  (target ≤ 150ms at 300 on a dev machine); render frame time during a
  continuous wheel; and a check that the overlay performs no network request
  after its single fetch.
- **Side-by-side capture check:** the test saves `c-soup-<theme>-<WxH>.png`
  (Fit), `c-zoom-<theme>-<WxH>.png` (`k = 1.45` on the largest cluster),
  `ego-<theme>-<WxH>.png`, and `kw-filter-<theme>-<WxH>.png`. It also saves
  the matching mockup states (`ticket-map-v5-bc.html#c`, the mockup's zoom
  buttons and double click) as `mock-c-*.png`, into `WS_TICKETMAP_SHOTS`. The
  reviewer compares the calm soup topography, the borderless typography, the
  hull and edge tone, the Ego Ring composition, and the chip and toolbar
  placement against v5, and the Result records the comparison.

### Result (4db08f3f0) - 2026-11-20

- **Landed** (`5169291c6..4db08f3f0`):
  - `dashboard-ticketmap-geom.js` (`ticketMap.geom`): the pure part. It holds
    mulberry32, the layout and iteration scaling, the hull, `inPoly`, the soup
    offsets and centroid, the spatial hash, the label planner, the sim-edge
    pick, the tooltip clamp, `zoomAt`/wheel/fit, the stage and soup alpha, the
    Ego geometry, and `resolveHit`;
  - `dashboard-ticketmap-graph.js` (`ticketMap.graph`): the View C DOM,
    rendering, interaction, and Ego Ring;
  - View C rules appended to `dashboard-ticketmap.css`, with `--d-tm-sk-l` in
    all three theme blocks;
  - the C tab, the focal-card ToC and Ego buttons, and the B↔C shared-state
    routing;
  - `js.tmap.c.*` keys in all 11 tables;
  - `ticketmap-graph.test.cjs` and `TestTicketMapGraphBrowser`.
- **Verification:**
  - `node --test testdata/ticketmap-*.test.cjs`: 27/27 pass.
  - `go test ./internal/wsdashboard -run 'TicketMap|DashboardSourceAssembly|CSSClassesDoNotCollide'`:
    ok.
  - `go test ./internal/wsi18n`: ok.
  - `WS_PLAYWRIGHT_MODULE=… WS_TICKETMAP_SHOTS=/tmp/tmshots-p3 go test ./internal/wsdashboard -run 'TicketMap.*Browser' -count=1`:
    `TestTicketMapBrowser` and `TestTicketMapGraphBrowser` both PASS. That
    covers items 1–17 in dark and light at 1600×960 and 1280×800, plus the
    `hasTouch` 1280×800 context: the CDP pinch scales ×2.00 with 0px drift.
- **Performance** (dev Mac, headless Chromium):
  - layout takes 30–34 ms on the 170 fixture (179 nodes) and 54–86 ms on the
    300 fixture (309 nodes), within the ≤ 150 ms target;
  - the frame interval during a continuous wheel is 8.6 ms mean (p95 9.3 ms)
    at 170 and p95 16.6 ms at 300;
  - a synchronous `draw()` takes 1.0–1.2 ms;
  - after the single fetch, no session makes any request.
- **Captures:** compared `/tmp/tmshots-p3/c-soup-*`, `c-zoom-*`, `ego-*`, and
  `kw-filter-*` against `mock-c-*` (both themes, both viewports): match. The
  calm soup topography, the borderless typography, the hull and edge tone, the
  Ego Ring composition, and the chip and toolbar placement agree.
  The remaining differences come from the data or the spec:
  - the fixture's keyword weights are steeper and it has fewer cross-cluster
    links;
  - the Ego Ring hides non-members, as specified;
  - the keyword camera lands at k = 1.1 versus v5's 0.9, from the same cap
    formula over a different fit.
- **Decisions:**
  - Pure geometry lives in its own file, `dashboard-ticketmap-geom.js`,
    loaded before the graph file. The script order is ticketmap, viewb, geom,
    graph. The split keeps every file under the 1000-line size guard and makes
    the logic testable in Node.
  - `.on` is the placed-label state class named by the Playwright contract. It
    only ever appears compounded onto a `d-tm-` class, and the CSS scope test
    asserts that.
  - Ego sector captions are clamped 4px inside the viewport. v5 left one
    clipped at 1280; this deliberately departs from it.
  - A `window.__tmapc` probe is exposed only under the same test-hook gate as
    `window.__tmap`.


## Resolution (2026-11-20)

All three phases landed on impl/develop/disk-tusk-phony (59bde59ba..469a78ebd). The lite review found one Critical: root eviction wrote into shared ticketCache maps that a concurrent listing could be reading. It is fixed in 469a78ebd by a clone-and-swap, with a race test that fails without the fix under -race. The Important item, the missing eviction test, is fixed in the same commit, as are two Minors: nodes now sort by stem, and the surface test is strict about missing files. Three Minors remain open as follow-ups. Refresh keeps hidden-cluster, collapsed-lane and keyword filters by the renumbered cluster id. A closed ref's cluster score counts refs placed earlier. A plain listing that runs during a map build can drop fresh docs, which costs speed only. Final verification passed: go test of wsdoc, the targeted wsdashboard run, wsi18n, and mcp TicketGraph; node 27/27; TestTicketMapBrowser and TestTicketMapGraphBrowser.
