---
title: "Dashboard session page: cut per-request payload and server render cost (assets out of line, double board send, lazy history)"
related:
  261116-bug-ws-dashboard-hidden-cached-page-cost: "Phase 2 there externalized only the framed main script to dashboard-<hash>.js and left every other inline asset in place; this ticket continues that, see Overlap"
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "owns frontend measurement and non-board hotspots; this ticket owns the server-side page payload and render cost it deferred (its Constraints allow a server payload change only as T0 additive)"
  261116-research-ws-polling-minimization-event-ledger: "adjacent: polling/change-ledger design; the revision stamp in Phase 1 should not preempt its outcome"
  261116-feat-ws-whiteboard-retire-disk-pages: "separate ticket: retiring file-only/disk pages; explicitly not part of this one"
---

# Dashboard session page: cut per-request payload and server render cost

## Background

Owner decision (whiteboard item (1205)=a, 2026-11-16): do the payload and render-cost reduction first, as a ticket, in two phases. Retiring the file-only whiteboard mode is a separate ticket (`261116-feat-ws-whiteboard-retire-disk-pages`).

Measured 2026-11-16 on a shared daemon with one large session board (single observation, not a benchmark):

- The full framed session page (`/s/<rid>?frame=1`) took about 0.92 s of server time and 1.74 MB per request, identical on repeat.
- Inline `<script>` about 1.0 MB, inline `<style>` about 0.48 MB, SVG sprites about 0.04 MB.
- Daemon idle CPU about 0% (no busy loop); RSS about 1.1 GB.

So the cost is per fresh page load, not idle. Every session switch loads a fresh frame (the hidden-cache ticket removed the page cache), which makes this per-switch latency.

### Code facts (re-verified at drafting; line numbers drift, re-check)

Paths are relative to `agents-plugin-tool/`.

Request path: `/s/<rid>?frame=1` -> `handleSessionPage` (`internal/wsdashboard/pages.go:1367`) -> `readSessionWith` (`:1014`) -> `ReadPageLang` (`internal/mcp/dashboard_page.go:51`) -> `renderBoard` (`:148`) -> `renderBoardTopics` (`:263`, view input `LayoutFull` via `servedViewInput`, `:389`) -> `wswhiteboard.BuildView` (`internal/wswhiteboard/view.go:584`) -> `RenderRegionsWithTopics` (`internal/wswhiteboard/render.go:374`). The session template is `internal/wsdashboard/assets/dashboard.tmpl` (`define "session"` at line 1).

Assets inlined in every response (`dashboard.tmpl:5-48`): whiteboard CSS, dashboard CSS, tracker CSS, three sprites, and the Answers/Form/Mermaid-init/Viewer/Toast/Tracker/Mention/Rail/Memo/Usage/Shell/Cards/Overview/Preview/Files (and optionally Terminal/Web) scripts, plus `{{i18n}}` (`wsi18n.Script(lang)`, bound at `pages.go:391`). Assets are built once but copied into each response. The only external URLs are the framed main script (`/s/{{.Assets.ScriptFile}}`, `dashboard.tmpl:47`), mermaid, `wb-frame-*` CSS/JS and xterm/guide assets.

Server work per request:

- `boardMemo` (`dashboard_page.go:94-207`, per reply-id and language, max 64) skips the render on a hit, but the final page HTML is not cached.
- Even on a memo hit, `boardMemoBase` `json.Marshal`s the whole `record.Whiteboard`, including logs and panel versions (`dashboard_page.go:224`), and `boardWatchStamp` stats every watched file (`:251`).
- On a memo miss (any board, todo or agenda change): `BuildView` covers every topic including closed ones (`streamView` call sites `view.go:695,706`, `timelineView` `:713`); `diffPanel` re-parses markdown of the latest and previous version for every panel (`internal/wswhiteboard/stream_view.go:250`, `panel_diff.go:25`); `topicRevision` marshals each topic.

Payload drivers inside the board region:

- Closed topics past `ClosedCap` (30, `view.go:14`) are still rendered fully into `<template data-history-topic>` (`internal/wswhiteboard/assets/page.tmpl:19`; filled by `DeferredTopics`, `view.go:826-829`).
- Timeline gap rows are inlined as `<template data-history-gap>` (`page.tmpl:70`).
- `prepareTopic` renders every panel version body, including history (`render.go:600-611`).
- Each body is wrapped by `framedDoc` (`render.go:725`) as an escaped `data-srcdoc` document carrying its own `wsi18n.SubsetScript` (`render.go:740`; `internal/wsi18n/i18n.go:291`), rebuilt per frame with no cache.
- Topic-body images are inlined as base64 data URLs (`DocImageSrc` -> `wsdashboard.ImageDataURL`, `dashboard_page.go:396-398`, up to 2 MiB per image per `access.go` comment).
- `ThemeFrames` string-replaces over the whole board per request (`render.go:805`).

Stream: the frame opens `/s/<rid>/events?h=<held hashes>&frame=1&topics=1&treerows=1&entryrows=1&panelrows=1` (`internal/wsdashboard/assets/dashboard-stream.js:387`). On the first send the server always resends the full board region even when the browser already holds that hash: the held-hash skip is `held[name] == region.Hash && !(isBoard && topics.previous == nil)` (`internal/wsdashboard/sse.go:685`), and `boardTopicState` documents that the first send is full because a connection has no topic baseline (`board_topics.go:101-103`). A fresh frame therefore downloads the board in the page and again over the stream. Updates coalesce to at most one render per 250 ms plus a 15 s tick; a `board-topics` delta is sent only when the board "shell" part is unchanged (`board_topics.go:107-119`).

Already on demand: closed topics past the cap load from `/s/<rid>/closed` (`dashboard-board.js:317`, handler `handleClosedTopics`, `pages.go:1507`); older entries from `/s/<rid>/history` (`handleHistory`, `pages.go:1452`).

### Overlap with existing ready tickets

- `261116-bug-ws-dashboard-hidden-cached-page-cost` (Phase 2 Result, audit at `ai-docs/assets/261116-bug-ws-dashboard-hidden-cached-page-cost/phase2-load-audit.md`): it moved only the main script into a hashed immutable URL (591,935 B removed from the HTML) and explicitly left tracker.js, CSS and other inline code in place, saying a broader split "needs its own load-order/fixture review". Phase 1 here is that follow-up and reuses the same mechanism (`ScriptFile`, content-hashed, immutable). Its audit also states a fresh document needs no duplicate full board fetch before the stream connects; the `sse.go:685` condition above suggests otherwise for the board region. This ticket's worker must measure that claim first and record which is right. That ticket's cache removal, terminal laziness and scroll records are out of scope here.
- `261113-feat-ws-dashboard-frontend-performance-profile-and-optimize`: owns measurement scenarios (its Phase 1 (a) initial load) and non-board client hotspots; it defers server payload to a T0-additive change only if measured. This ticket is that measured server-payload case. Reuse its scenarios and harness for the before/after numbers instead of inventing a new one; do not touch its Phase 3 (board exposure) or composer files.
- `261116-research-ws-polling-minimization-event-ledger`: considers change ledgers for the polling paths. Phase 1's cheap revision/stamp for the memo key must stay a local read of existing board state and not introduce a new ledger or file.

## Decisions

- 2026-11-16, whiteboard (1205)=a: phases 1 and 2 of this ticket first; disk-page retirement is a separate ticket.
- Direction, non-authoritative: client-side markdown rendering is rejected for now. The lead's recommendation for a later phase is server-rendered per-topic fragments cached by topic revision and assembled by the client. Not part of this ticket.

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`: `ai-docs/manuals/shipped-surface-boundary.md` (touches `agents-plugin-tool/`), `ai-docs/manuals/release-compatibility.md` (touches `agents-plugin-tool/internal/`), `ai-docs/manuals/ws-mcp.md` (touches `internal/mcp/`). If skills, rsrc or convention docs change: `skill-authoring.md` and `wsflow-mirroring.md`.
- No intended visible change in Phase 1. Phase 2 may change when history appears (on expand), not what it looks like.
- Out of scope: file-only/disk whiteboard mode retirement (`261116-feat-ws-whiteboard-retire-disk-pages`) and client-side markdown rendering. Standalone pages written to disk keep rendering as today; any shared template change must keep them working.
- Compatibility tier (AGENTS.md `### Compatibility Tiers`, scenario sheet `ai-docs/manuals/release-compatibility.md`): new content-hashed asset URLs and new read-only GET routes are T0, since an old binary never calls them and a new page only references URLs its own daemon serves. If a change touches a cross-version surface (the stream event shape, the `h=` held-hash semantics, a region or `board-topics` payload field), the worker states its tier here with cited read/write sites before implementing. A new frame against an old daemon (or the reverse) must not break; verify that claim, do not assume it.
- Electron and LAN access must be unaffected: asset URLs are same-origin and carry the same access rule as the page.

## Phases

### Phase 1: Out-of-line assets, single board send, cheap memo key

No visible change. Contracts (implementation is the worker's):

- Externalize every remaining inline CSS, JS, i18n script and sprite in the session template to content-hashed immutable URLs, like the existing `dashboard-<hash>.js`. Shell, index and standalone pages keep working; load order of the scripts is preserved.
- Honor the held hash on the first stream send, so a fresh frame does not receive the board it just rendered. This needs the server to establish the per-connection topic baseline without sending the region (see the `boardTopicState` comment); a client that omits the board hash (`connect(withoutBoard)`, `dashboard-board.js:12`) must still get a full board.
- Replace the per-request whole-state `json.Marshal` memo key (`dashboard_page.go:224`) with a cheap revision or stamp over existing state.
- Cache the per-frame i18n subset script (`wsi18n.SubsetScript` per language and prefix) instead of rebuilding it per frame.

Verification:

- Measured before/after page bytes and server time on a large fixture board (target: page payload without assets well under 0.3 MB for the fixture; record server time, which has no fixed target).
- Existing page and stream tests stay green. Every test asserting inline versus external script placement (for example in `pages_test.go` and `toast_test.go`, such as the "session frame must reference the assembled dashboard script" check) is updated deliberately to the new contract, not deleted.
- A new test that the first stream send skips a held board hash and that a client without the hash still gets the full board.
- Electron and LAN access verified unchanged (same-origin asset URLs).

### Phase 2: Lazy history

Depends on Phase 1's landed Result.

- Do not render closed topics past the cap, older panel versions, or timeline gaps into the initial page; load them on expand through existing (`/s/<rid>/closed`, `/s/<rid>/history`) or new read-only GET routes.
- Keep the panel-diff gutter working. It needs the previous version; decide whether the server sends the precomputed diff or the client fetches the previous version, and record the decision here.
- No base64 images in the page: serve them by URL. A topic body renders in a sandboxed frame that sends no cookie (comment at `dashboard_page.go:393-395`), so the worker must settle how a cookie-less frame is authorized to load an image URL and record it under `## Decisions`.

Verification: browser tests for expanding closed topics, for panel history, and for the diff gutter; before/after payload measurement on the same fixture as Phase 1.

## Open Questions

- Whether the first-send claim in the hidden-cache audit or the `sse.go:685` reading is correct for the board region (measure first; see Overlap).
- Whether the fixture board should be a checked-in generated fixture or built by a test helper.
