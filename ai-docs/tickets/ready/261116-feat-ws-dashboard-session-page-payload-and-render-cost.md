---
title: "Dashboard session page: cut per-request payload and server render cost (assets out of line, double board send, lazy history)"
related:
  261116-bug-ws-dashboard-hidden-cached-page-cost: "Phase 2 there externalized only the framed main script to dashboard-<hash>.js and left every other inline asset in place; this ticket continues that, see Overlap"
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "owns frontend measurement and non-board hotspots; this ticket owns the server-side page payload and render cost it deferred (its Constraints allow a server payload change only as T0 additive)"
  261116-research-ws-polling-minimization-event-ledger: "adjacent: polling/change-ledger design; the revision stamp in Phase 1 should not preempt its outcome"
  261116-feat-ws-whiteboard-retire-disk-pages: "separate ticket: retiring file-only/disk pages; explicitly not part of this one"
  261115-feat-ws-dashboard-board-topic-transport: "Phase 1 reverses its first-send-full rule for the board region (see Decisions)"
  261116-feat-ws-dashboard-localhost-token: "its cookie gate constrains asset and image URL authorization"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c700131f65f26258
sage-review-completeness-reviewed: c700131f65f26258
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

Request path: `/s/<rid>?frame=1` -> `handleSessionPage` (`internal/wsdashboard/pages.go:1376`) -> `readSessionWith` (`:1023`) -> `ReadPageLang` (`internal/mcp/dashboard_page.go:51`) -> `renderBoard` (`:148`) -> `renderBoardTopics` (`:263`, view input `LayoutFull` via `servedViewInput`, `:387-389`) -> `wswhiteboard.BuildView` (`internal/wswhiteboard/view.go:586`) -> `RenderRegionsWithTopics` (`internal/wswhiteboard/render.go:374`). The session template is `internal/wsdashboard/assets/dashboard.tmpl` (`define "session"` at line 1).

Assets inlined in every response (`dashboard.tmpl:5-49`): whiteboard CSS, dashboard CSS, tracker CSS (and optionally xterm CSS), three sprites, and the Answers/Form/Mermaid-init/Viewer/Toast/Tracker/Mention/Rail/Memo/Usage/Shell/Cards/Overview/Preview/Files (and optionally Terminal/Web) scripts, then the SelectAll script (`dashboard.tmpl:49`), plus `{{i18n}}` (`wsi18n.Script(lang)`, bound at `pages.go:396`). Assets are built once but copied into each response. The only external URLs are the framed main script (`/s/{{.Assets.ScriptFile}}`, `dashboard.tmpl:50`), mermaid, `wb-frame-*` CSS/JS and xterm/guide assets.

Server work per request:

- `boardMemo` (`dashboard_page.go:94-207`, per reply-id and language, max 64) skips the render on a hit, but the final page HTML is not cached. It also keeps a per-topic memo (`boardTopicMemo`, the `TopicReuse` closure in `renderBoard`, `dashboard_page.go:170-191`; `wswhiteboard.TopicReuse`, `render.go:358-360`): on a memo miss, topics whose `topicRevision` and shared base are unchanged skip their body/panel markdown and topic template (`render.go:373`), but `BuildView` below still runs in full.
- Even on a memo hit, `boardMemoBase` `json.Marshal`s the whole `record.Whiteboard`, including logs and panel versions (`dashboard_page.go:224`), and `boardWatchStamp` (`:251`, called at `:161`) stats every watched file.
- On a memo miss (any board, todo or agenda change): `BuildView` covers every topic including closed ones (`streamView` call sites `view.go:698,709`, `timelineView` `:716`); `diffPanel` re-parses markdown of the latest and previous version for every panel (`internal/wswhiteboard/stream_view.go:250`, `panel_diff.go:25`); `topicRevision` marshals each topic (`internal/wswhiteboard/render_topics.go:15`).

Payload drivers inside the board region:

- Closed topics past `ClosedCap` (30, `view.go:14`) are still rendered fully into `<template data-history-topic>` (`internal/wswhiteboard/assets/page.tmpl:19`; filled by `DeferredTopics`, `view.go:829-832`).
- Timeline gap rows are inlined as `<template data-history-gap>` (`page.tmpl:67`).
- `prepareTopic` renders every panel version body, including history (`render.go:600-611`).
- Each body is wrapped by `framedDoc` (`render.go:725`) as an escaped `data-srcdoc` document carrying its own `wsi18n.SubsetScript` (`render.go:740`; `internal/wsi18n/i18n.go:291`), rebuilt per frame with no cache.
- Topic-body images are inlined as base64 data URLs (`DocImageSrc` -> `wsdashboard.ImageDataURL`, `dashboard_page.go:394-398`, up to 2 MiB per image per `access.go:31` and `image_route.go:213-215`; the encoded result is cached by file path, size and mtime, `image_route.go:217-262`, so the per-request cost is containment checks and copying the string into the page, not a re-read).
- `ThemeFrames` string-replaces over the whole board per request (`render.go:805`).

Stream: the frame opens `/s/<rid>/events?h=<held hashes>&frame=1&topics=1&treerows=1&entryrows=1&panelrows=1` (`internal/wsdashboard/assets/dashboard-stream.js:388`). On the first send the server always resends the full board region even when the browser already holds that hash: the held-hash skip is `held[name] == region.Hash && !(isBoard && topics.previous == nil) && !(row.state != nil && !row.state.initialized)` (`internal/wsdashboard/sse.go:685`), and `boardTopicState` documents that the first send is full because a connection has no topic baseline (`board_topics.go:102-105`). That first-full behavior is a recorded, tested decision of `261115-feat-ws-dashboard-board-topic-transport` (see Prior Decisions), which Phase 1 would change for the board region. A fresh frame therefore downloads the board in the page and again over the stream. Updates coalesce to at most one render per 250 ms plus a 15 s tick; a `board-topics` delta is sent only when the board "shell" part is unchanged (`board_topics.go:107-120`).

Already on demand: closed topics past the cap load from `/s/<rid>/closed` (`dashboard-board.js:317`, handler `handleClosedTopics`, `pages.go:1516`); older entries from `/s/<rid>/history` (`handleHistory`, `pages.go:1465`).

### Overlap with existing ready tickets

- `261116-bug-ws-dashboard-hidden-cached-page-cost` (Phase 2 Result, audit at `ai-docs/tickets/assets/261116-bug-ws-dashboard-hidden-cached-page-cost/phase2-load-audit.md`): it moved only the main script into a hashed immutable URL (591,935 B removed from the HTML) and explicitly left tracker.js, CSS and other inline code in place, saying a broader split "needs its own load-order/fixture review". Phase 1 here is that follow-up and reuses the same mechanism (`ScriptFile`, content-hashed, immutable). Its audit also states a fresh document needs no duplicate full board fetch before the stream connects; the `sse.go:685` condition above suggests otherwise for the board region. This ticket's worker must measure that claim first and record which is right. That ticket's cache removal, terminal laziness and scroll records are out of scope here.
- `261113-feat-ws-dashboard-frontend-performance-profile-and-optimize`: owns measurement scenarios (its Phase 1 (a) initial load) and non-board client hotspots; it defers server payload to a T0-additive change only if measured. This ticket is that measured server-payload case. Reuse its scenarios and harness for the before/after numbers instead of inventing a new one; do not touch its Phase 3 (board exposure) or composer files.
- `261116-research-ws-polling-minimization-event-ledger`: considers change ledgers for the polling paths. Phase 1's cheap revision/stamp for the memo key must stay a local read of existing board state and not introduce a new ledger or file.

## Decisions

- 2026-11-16, whiteboard (1205)=a: phases 1 and 2 of this ticket first; disk-page retirement is a separate ticket.
- Phase 1 reverses the first-send-full rule of `261115-feat-ws-dashboard-board-topic-transport` (D6) for the board region: when the client's `h=` already holds the current board hash, the first opted-in send establishes the per-connection topic baseline without resending the region. Basis: (1205)=a chose a single board send (commit 90b2d5ed). Expected tier T0/T1-shaped, non-breaking: an old opted-in (`topics=1`) page against a new daemon starts with a null client baseline, so its first `board-topics` delta fails to patch and it reconnects with `connect(true)` for a full resend (`dashboard-stream.js:384,410-412,474-481`; `dashboard-board.js:516`) - a recovery, not a break, and the `assets` event usually reloads old pages after an upgrade anyway; a new page against an old daemon receives the full region as today (a duplicate, not a break). The worker confirms this tier with cited read/write sites before implementing and records it here; if the check finds T1 or worse, stop and report. Rejected: keep first-send-full, which is the duplicate download this phase exists to remove.
- Phase 1 implementation compatibility check: T0, in both directions, with no persisted or event-shape change. The old client clears its baseline in `assets/dashboard-stream.js` (`connect`, line 384), reads full `region` events at lines 410-412, and rejects a delta without a baseline at lines 474-481 by reconnecting without the board hash; `dashboard-board.js` (`boardInputPatch`, line 516) performs that rejection. Thus an old opted-in page recovers transparently with one full resend on its first change, while non-opted-in pages keep full region delivery. A new client still accepts the old daemon's unconditional full first region (`sse.go`, `sendRegions`, lines 685-709). New immutable asset URLs are only emitted by their serving daemon and retain the existing cookie/LAN access guard. The baseline measurement confirms the duplicate first board region, not the earlier audit's no-duplicate claim.
- Phase 1 implementation choices: retain inline SVG sprites to preserve same-document `<use>` targets without icon flash. Extend the existing immutable dashboard-file mechanism for framed prerequisites/styles and all page languages; preserve CSS cascade and classic-script order, with optional terminal/Web modules in their original slots. Non-frame, shell, index and standalone keep their inline paths. Also externalize the fixed per-language topic subset through the existing public `FrameAsset` exemption: T0, read-only translation bytes, no new authorization exception, no state writes or old-reader changes. Missing hashes follow the existing no-grace/asset-event-reload recovery. Use a bounded process-local immutable-folder-read revision for the memo key, not Seq alone or a new ledger/file.
- Phase 2 diff-gutter decision (2026-11-17): retain the server-precomputed latest-versus-immediate-prior diff; do not fetch a previous version merely to draw the gutter. `wswhiteboard/stream_view.go:243-254` reads the retained immutable pair into `EntryView.Diff`; `panel_diff.go:201-244` writes sanitized old-block snapshots, word edits and marks into the rendered current body; `assets/panel-diff.js` reads that metadata (also inside `doc-frame.js`). Lazy history will omit historical bodies and gap rows, not this current-body comparison. T0: render-only, no durable/event shape change; old clients still receive their existing full board events and new clients accept eager old-daemon HTML. Reopening either version reads unchanged immutable versions.
- Phase 2 cookie-less image decision (2026-11-17): use the existing host-page relay for **all** served topic/panel file images, not a new public image exemption or bearer capability. `mcp/dashboard_page.go:servedViewInput` will supply an empty `DocImageSrc`; `wswhiteboard/markdown.go:fileTarget` records the protected `ImageSrc` URL through `recordImageRelay`, and `render.go:framedDoc` writes the server-owned frame allowlist. `assets/doc-frame.js` asks its parent for a Blob; `wsdashboard/assets/dashboard-frames.js` validates the source frame and exact allowlisted session image URL before a same-origin credentialed fetch. `wsdashboard/image_route.go:handleImage`, `access.go:accessGuard` and `localhost_auth.go:accessExempt` retain cookie/LAN, containment, fetch-metadata and outside-root-local-only checks; `fs_capability.go:previewCapability` and `fs_outside.go` are unchanged. T0: read-only URLs and the existing relay protocol only, no new exemption, durable write or old-reader change; old pages/daemons keep their inline/oversized-relay path, new pages also understand that eager path. Reopening reads the same files under the same authorization. Raw author HTML remains authored HTML; the no-base64 rule applies to server-generated file-image payloads.
- Direction, non-authoritative: client-side markdown rendering is rejected for now. The lead's recommendation for a later phase is server-rendered per-topic fragments cached by topic revision and assembled by the client. Not part of this ticket.

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`: `ai-docs/manuals/shipped-surface-boundary.md` (touches `agents-plugin-tool/`), `ai-docs/manuals/release-compatibility.md` (touches `agents-plugin-tool/internal/`), `ai-docs/manuals/ws-mcp.md` (touches `internal/mcp/`). If skills, rsrc or convention docs change: `skill-authoring.md` and `wsflow-mirroring.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- No intended visible change in Phase 1. Phase 2 may change when history appears (on expand), not what it looks like.
- Out of scope: file-only/disk whiteboard mode retirement (`261116-feat-ws-whiteboard-retire-disk-pages`) and client-side markdown rendering. Standalone pages written to disk keep rendering as today; any shared template change must keep them working.
- Compatibility tier (AGENTS.md `### Compatibility Tiers`, scenario sheet `ai-docs/manuals/release-compatibility.md`): new content-hashed asset URLs and new read-only GET routes are T0, since an old binary never calls them and a new page only references URLs its own daemon serves. If a change touches a cross-version surface (the stream event shape, the `h=` held-hash semantics, a region or `board-topics` payload field), the worker states its tier here with cited read/write sites before implementing. A new frame against an old daemon (or the reverse) must not break; verify that claim, do not assume it.
- Electron and LAN access must be unaffected: asset URLs are same-origin and carry the same access rule as the page. Since the localhost cookie gate landed, every `/s/` route outside `publicAsset` (`access.go:149-162`) and `previewCapability` (`fs_capability.go:274`) needs the localhost cookie for a local viewer or the LAN token otherwise (`accessGuard`, `access.go:94-140`; `accessExempt`, `localhost_auth.go:107-123`). The existing `/s/dashboard-<hash>.js` route (`pages.go:1296-1308`) is not exempt, so a new hashed asset URL is cookie-gated the same way and works for the same-origin session frame; a sandboxed topic-body frame sends no cookie and can load only exempt routes.

## Prior Decisions

- 261115-feat-ws-dashboard-board-topic-transport (2026-11-16, commit 07544121): "D6 directional T0: treerows=1 is additive and per connection; old pages retain region events and new pages accept old daemon full regions. First opted-in send is full even with h=current; failed sends do not advance the baseline" — bearing: contradiction-candidate
- 261115-feat-ws-dashboard-board-topic-transport (2026-11-14, commit 3ec591d8): "This is the ticket's T0 additive topics=1 opt-in; old streams retain full region events and the first opted-in board send is full even if h matches." — bearing: contradiction-candidate
- 261116-feat-ws-dashboard-localhost-token (2026-11-17, Decisions (1233)): "Topic-body images over 2 MiB keep working: since the read-hardening merge they are relayed through the same-origin host page (the same relay as outside-root images), so the host's cookie carries them." — bearing: constrains
- 261116-bug-ws-dashboard-hidden-cached-page-cost (2026-11-16, Result c658c4798): "Framed sessions reference a shared content-hashed main-script URL with immutable caching; all prerequisite modules retain their order. Shell, index and standalone rendering retain the existing inline path." — bearing: supports
- 90b2d5ed (2026-11-16, commit): "Owner chose (1205)=a: ship phases 1-2 (out-of-line assets, single board send, cheap memo key; lazy history) first" — bearing: supports
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-14, Result e04c7ea1b): "The swap, adopted-page and history-load mounts stay synchronous." — bearing: constrains
- a068fb9f (2026-11-07, commit): "allow-scripts without allow-same-origin keeps inline canvas scripts working but gives them an opaque origin." — bearing: constrains
- 261112-bug-ws-dashboard-open-tab-keeps-stale-assets-after-daemon-upgrade (2026-11-12, commit 978c3d79): "the whiteboard's own static page (wswhiteboard live.js) does not inline dashboard.js and was left out of scope." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | dashboard.tmpl, pages.go, sse.go, board_topics.go, dashboard-stream.js, mcp/dashboard_page.go, wswhiteboard/render.go, wsi18n/i18n.go, view.go, page.tmpl |
| scope.surface | cross-module | spans wsdashboard, mcp, wswhiteboard, wsi18n; changes the stream held-hash first-send semantics and adds asset and history GET routes |
| scope.new_public_symbol | yes | new content-hashed asset GET routes (Phase 1) and possibly new lazy-history GET routes (Phase 2); no exported Go symbol is named by the ticket |
| scope.new_type_contract | unknown | the cheap revision/stamp memo key and the i18n subset cache have no stated signature; the first-send baseline API is left to the worker |
| scope.test_surface | new-files | existing pages_test.go:351-374, session_load_test.go:23, assets_id_test.go, toast_test.go:316; the ticket requires a new first-send test and new browser tests for lazy history |
| complexity.reuse_points | confirmed | ScriptFile mechanism pages.go:317,1296-1308; TopicReuse render.go:358; closed and history routes pages.go:1465,1516; data-relay render.go:706-712 |
| complexity.side_effect_risk | high | script load order across about 17 inline scripts, per-connection stream baseline, standalone and index pages share the template, memo key correctness |
| risk.correctness | high | first send must still deliver the board when no hash is held or a reconnect has no baseline; memo key replacement can serve a stale board |
| risk.fit | moderate | reuses the existing hashed-script and closed/history mechanisms, but Phase 1 reverses the recorded first-send-full decision of 261115-feat-ws-dashboard-board-topic-transport |
| risk.test | high | needs a large fixture board, before/after measurement, load-order proof, and real-browser tests for expand, history and diff gutter |
| risk.security_or_contract | high | localhost cookie gate and cookie-less sandboxed frame authorization for asset and image URLs; stream held-hash semantics are a cross-version surface with the tier not yet stated |

## Phases

### Phase 1: Out-of-line assets, single board send, cheap memo key

No visible change. Contracts (implementation is the worker's):

- Externalize every remaining inline CSS, JS and i18n script of the framed session page (`.Frame`) to content-hashed immutable URLs, like the existing `dashboard-<hash>.js`. Follow that precedent's scope: non-frame session, shell, index and standalone pages keep their inline path and keep working; load order of the scripts is preserved. Sprites are referenced by same-document `<use href="#...">` from the Go icon helper (`pages.go:369`), templates and many JS sites; keep them inline unless the worker finds a mechanism without icon flash or load-order risk, and record the choice. A hash that disappears between the HTML and its GET follows the main-script precedent (forward finding M2 of `261116-bug-ws-dashboard-hidden-cached-page-cost`): no grace window; the existing `assets` event reload is the recovery.
- Honor the held hash on the first stream send, so a fresh frame does not receive the board it just rendered. This needs the server to establish the per-connection topic baseline without sending the region (see the `boardTopicState` comment); a client that omits the board hash (`connect(withoutBoard)`, `dashboard-stream.js:374`; `held`, `dashboard-board.js:12-13`) must still get a full board. The client keeps its own per-connection baseline too: `boardInput` is reset on every `connect()` and set only from a full board region event (`dashboard-stream.js:384,410-412`), and a delta against a null baseline forces a reconnect and full resend (`:474-481`, `dashboard-board.js:516`). So on each connection the client must seed `boardInput` from the canonical board it already holds for the hash it sent in `h=` (the page-rendered board, `dashboard-board.js:210`, or the latest held update); a server-only change would just move the duplicate download to the first board change.
- Replace the per-request whole-state `json.Marshal` memo key (`dashboard_page.go:224`) with a cheap revision or stamp over existing state. Keep the invariant in the comment at `dashboard_page.go:221-224`: folder metadata, logs and panel versions are authoritative inputs, so the key must still change when any of them changes (for example per-folder append offsets or sizes already held in the record). Rejected: a `Whiteboard.Seq`-only key, which serves stale boards.
- Cache the per-frame i18n subset script (`wsi18n.SubsetScript` per language and prefix) instead of rebuilding it per frame. Optional: serve it as a content-hashed public asset like `frameJSFile` so its bytes stop repeating in every body frame's srcdoc (`render.go:740`).

Verification:

- Measured before/after page bytes and server time on a large fixture board (target: the framed page HTML response alone, with externalized assets measured separately as cacheable immutable responses, under 0.3 MB for the fixture; record server time, which has no fixed target).
- Establish the current baseline first: `261116-feat-ws-daemon-hub-first-polling` Phase 3 Result records owner-declared base-red MCP tests, including `TestDashboardServedPageRendersTopicBodyInAFrame` and `TestDashboardReadClosedTopicsPastTheCap`; pre-existing reds are not regressions (fix them only if in scope). Existing page and stream tests that were green stay green. Every test asserting inline versus external script placement (for example in `pages_test.go` and `toast_test.go`, such as the "session frame must reference the assembled dashboard script" check) is updated deliberately to the new contract, not deleted.
- New tests: the first stream send skips a held board hash; a client without the hash still gets the full board; a reconnect whose held hash differs from the current one gets the full board; and the first `board-topics` delta after a skipped first send applies without a reconnect.
- Electron and LAN access verified unchanged (same-origin asset URLs).

### Result (0843f8a34) - 2026-11-17

- Completed Phase 1 only: immutable out-of-line framed CSS/JS/page i18n, shared public topic-frame subsets, paired server/client held-board baselines, and the cheap authoritative-folder-read memo revision. Supporting commits: `c6434c3e9` (bounded subset cache), `35521c009` (memo revision), `0843f8a34` (assets/stream/contracts/evidence). Sprites remain inline; shell, index, non-frame and standalone paths retain their inline behavior. No `server.go` edit.
- Baseline confirmed the duplicate initial board send. The new first connection skips it when held-current, still sends full for missing/stale hashes, and applies the next delta without reconnecting, including hidden canonical-input recovery. T0 directional compatibility and the public-subset authorization choice are recorded above.
- Reused the generated performance fixture rather than checking in a board snapshot. The default history-heavy shape `8,6,4,2,60` fell from **2,944,449 to 985,210 B** of framed HTML; the separate 32-topic shape `24,8,0,0,0` fell from **1,991,993 to 252,920 B**, meeting the 0.3 MB target only for that shape. Cold handler samples were 217.880 → 159.659 ms and 82.751 → 82.846 ms respectively; warm last-three medians were 16.278 → 2.809 ms and 3.116 → 1.174 ms. These are noisy single observations, not a latency benchmark. Unique referenced immutable assets are measured separately in the reports.
- Verification: the two named historical MCP reds passed before and after; focused page/watch/memo/topic-cache, asset/order/access/language/dev-ID, Node stream/hidden/recovery and SSE/WebSocket tests passed. Focused race tests passed. Full whiteboard/i18n tests and final cached Chromium scoped-selection/panel-diff acceptance passed. No whole MCP/dashboard package run or browser download.
- Visual verification: compared [after capture](../assets/261116-feat-ws-dashboard-session-page-payload-and-render-cost/phase1/after.png) vs [before capture](../assets/261116-feat-ws-dashboard-session-page-payload-and-render-cost/phase1/before.png): **match**, 0 / 1,600,000 differing RGB pixels with dynamic header/rail/time masks; [side-by-side](../assets/261116-feat-ws-dashboard-session-page-payload-and-render-cost/phase1/side-by-side.png) inspected. Both real-browser updates applied with zero reconnects and no page errors.
- Independent review: one Important stale inline-subset test assertion was fixed; the fixes-only second round confirmed the test, labeled payload evidence and independent pixel comparison. No Critical/Important finding remains. See the [audit and reproduction](../assets/261116-feat-ws-dashboard-session-page-payload-and-render-cost/phase1/audit.md) and its four raw JSON reports.
- Unresolved measurement limitation: the default history-heavy fixture remains above 0.3 MB; eager panel versions/timeline history remain for Phase 2. No attempt was made to hide that result or implement Phase 2 here. Native-cookie and LAN-token access were verified at the real HTTP guard; Electron and physical LAN-device testing were omitted by the run constraints. No shared daemon lifecycle action, SSH, push or merge.
- Workflow deviation: route/workflow-manual/acquire/review-render tools were unavailable on this worker's tool surface; a conservative independent single-review allocation with fixes-only confirmation was used. Ticket remains in `ready/`; Phase 2 is a separate run.

### Phase 2: Lazy history

Depends on Phase 1's landed Result.

- Do not render closed topics past the cap, older panel versions, or timeline gaps into the initial page; load them on expand through existing (`/s/<rid>/closed`, `/s/<rid>/history`) or new read-only GET routes.
- Known-target navigation keeps resolving: `materialize` and `loadTarget` in `wswhiteboard/assets/stream.js` (`:184-236`) resolve eN/pN targets from the history templates this phase removes (on-load fragment jumps, link-menu eN links to older versions, dashboard board links; relied on by ready tickets `261115-feat-ws-dashboard-conversation-rendering-consolidation`, `261114-feat-ws-dashboard-selection-mention-note` and `261115-feat-ws-whiteboard-viewed-panel-anchor`). For a target no longer in the page, fetch its fragment and then mount it synchronously, consistent with e04c7ea1b (history-load mounts stay synchronous).
- Keep the panel-diff gutter working. It needs the previous version; decide whether the server sends the precomputed diff or the client fetches the previous version, and record the decision here.
- No base64 images in the page: serve them by URL. A topic body renders in a sandboxed frame that sends no cookie (comment at `dashboard_page.go:394-395`), so the worker must settle how a cookie-less frame is authorized to load an image URL and record it under `## Decisions`. Existing mechanisms the worker can reuse or must stay consistent with (not a choice): the `publicAsset` exemption for fixed content-hashed frame/mermaid assets (`access.go:149-162`); the path-capability exemption `previewCapability` (`fs_capability.go:274`; outside-root variant `fs_outside.go`); and the host-page relay, where an image too large to inline keeps `data-relay` and the same-origin host fetches it through the cookie-gated `/s/<rid>/image` route and hands it to the frame (`wswhiteboard/render.go:706-712`; decision (1233) of `261116-feat-ws-dashboard-localhost-token`).

Verification: browser tests for expanding closed topics, for panel history, for the diff gutter, for an eN deep link to an older panel version, and for a link to a closed topic past the cap; before/after payload measurement on the same fixture as Phase 1 (record the numbers; no fixed target).

## Open Questions

- None blocking. The worker settles two measurement details and records them in its Result: which of the hidden-cache audit's first-send claim and the `sse.go:685` reading is correct for the board region (measure before changing the stream), and whether the large fixture board is generated by a test helper or checked in (prefer a helper; a checked-in fixture must stay small).

