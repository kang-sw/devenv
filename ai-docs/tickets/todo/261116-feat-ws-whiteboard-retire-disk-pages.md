---
title: "화이트보드 디스크 렌더링 페이지(파일 전용 모드) 폐지"
related:
  261119-feat-ws-whiteboard-retire-embed-snapshot: "prerequisite in-flight sibling; owns the embed result, LayoutEmbed/template branches, embed state, and embed-file cleanup contract"
  261119-refactor-ws-whiteboard-sqlite-board-store: "store sibling; this retirement lands after embed retirement and before the store ticket's Phase 2 switches the update path"
  261116-feat-ws-dashboard-session-page-payload-and-render-cost: "sibling; explicitly excludes this retirement"
  261116-feat-ws-dashboard-generalized-mentions: "sibling undesigned todo from the same day"
---

# Retire the whiteboard's disk-rendered pages (file-only mode) in favor of daemon-served boards

## Background

Owner statements, 2026-11-16 (verbatim Korean, then gist):

> "어차피 이제 데몬을 무조건 켜게 되어있으니까 file only whiteboard mod도 많이 의미가 식은 거 같고, 그래서 아예 파일만 읽는 화이트보드 모드를 버리고 그냥 데몬의 실시간 전용으로 쓴다 그러면 예를 들어 클라이언트 사이드 렌더링을 하고 데이터만 fetch해오는 식으로 상당히 최적화가 될 수 있을 거 같아서요." (22:41)

Gist: the daemon is always running now, so a whiteboard mode that only reads
files has lost its point; drop it and make the board daemon-live only. Any
client-side rendering redesign remains separate work.

Decision origin: whiteboard item (1205)=a split this retirement from
`261116-feat-ws-dashboard-session-page-payload-and-render-cost`. Whiteboard item
`(1839)=a` on 2026-11-20 approved the five decisions below. This is
functionality removal, so that approval is the AGENTS.md `### Approval
Protocol` authority for the settled design.

### Code facts

These facts were re-verified on `develop` at `785fd8fcefb6`; paths below are
relative to `agents-plugin-tool/`. Re-check line numbers after the in-flight
embed-retirement ticket merges.

- `whiteboard.update` currently renders during the authoritative folder
  mutation (`internal/mcp/whiteboard_tools.go:276-305`). The full-page path in
  `renderWhiteboardPages` builds `LayoutFull`, calls `RenderPage`, and prepares
  `index.html` plus `index.stamp.js` (`:615-691`). Publication runs after the
  board commit (`internal/mcp/session_state.go:649-675`) and writes each derived
  output through `whiteboardRender.publish` (`whiteboard_tools.go:771-812`).
- Successful top-level updates choose the daemon URL from `dashboardPageURL`
  and otherwise fall back to the rendered disk path, then emit `full:`
  (`whiteboard_tools.go:357-362`). `dashboardPageURL` returns a URL only for a
  registered session whose latest nudge succeeded
  (`internal/mcp/dashboard_sessions.go:735-752`).
- Todo, agenda, ticket, and mailbox writes call `refreshWhiteboardAfter` from
  the dispatcher (`internal/mcp/server_dispatch.go:202-216`); the refresh
  renders the full page and can read `index.html` / `index.stamp.js` to suppress
  an unchanged rewrite (`whiteboard_tools.go:563-589`, `:698-739`).
- `RenderPage` creates the standalone document
  (`internal/wswhiteboard/render.go:309-324`). The top-level `page` template,
  static-page wrapper, and full/embed document split begin at
  `internal/wswhiteboard/assets/page.tmpl:1-17`; `live.js` polls the adjacent
  stamp every 1.5 seconds (`assets/live.js:17-54`).
- Disk-page-only state and helpers include `StampFile`, `FullBoardLink`, and
  `FileLink` (`internal/wswhiteboard/view.go:33-35`, `:62-67`, `:558-610`), the
  unframed `DocFramed=false` render path and `chgWire`
  (`internal/wswhiteboard/render.go:527-529`, `:543-585`, `:608-638`), disk file
  links and page-directory selection
  (`internal/mcp/whiteboard_panels.go:62-74`, `:149-190`), and the
  `served=false` panel branches (`whiteboard_panels.go:69-74`, `:84-97`).
- Static-document i18n subsetting is wired through the page template's `i18n`
  function (`internal/wswhiteboard/render.go:157-176`, `:218-221`, `:264-270`).
  Daemon topic-body frames use the separate `frameKeyPrefix` subset and must
  remain (`:174-176`, `:215-219`).
- The daemon page is a different consumer: it builds a served view and renders
  fragments with `RenderRegionsWithTopics`
  (`internal/mcp/dashboard_page.go:280-330`). `RenderRegions` forces the full
  layout and framed bodies (`internal/wswhiteboard/render.go:326-407`), and
  `PageAssets` supplies the shared CSS, form, viewer, stream, and frame assets
  the daemon still needs (`:427-453`).
- Exactly 19 `internal/wswhiteboard/*_test.go` files currently call
  `RenderPage`; the search is `rg -l 'RenderPage\\(' internal/wswhiteboard
  --glob '*_test.go'`. Some pin standalone-only output, while others cover
  behavior that must move to `RenderRegions`, served-page, or frame tests.
- The authoritative board folder is not a rendered page. Its files include
  `meta.json`, `pending.json`, `committed.json`, `.lock`, topic logs, and panel
  bodies (`261119-refactor-ws-whiteboard-sqlite-board-store`, `### Current store
  inventory`). This ticket does not remove or migrate them.

## Decisions

- **A missing daemon is an error, not a fallback or startup action,
  `(1839)=a`.** `whiteboard.update` keeps a successful `full:` result line, but
  the value is always the daemon-served session URL. If no daemon is serving
  that session, return an error that tells the caller to start the daemon; do
  not auto-start it and do not return a disk path. Rejected: auto-starting the
  daemon, because the owner chose an explicit error; retaining the current
  disk-path fallback, because that preserves the file-only mode being retired.
- **The authoritative board data store stays, `(1839)=a`.** Retire only
  rendered page artifacts and their producer/consumer branches. Keep every
  board-state file and the current board semantics until the SQLite-store
  ticket changes them. Rejected: retiring the board folder with the pages;
  moving the store in this ticket.
- **Existing rendered HTML is removed once, `(1839)=a`.** The product must
  perform a one-time cleanup of existing rendered `ai-docs/whiteboards/**/*.html`
  in downstream projects without touching board data files. A deletion commit
  in this repository alone cannot satisfy that downstream contract, so the
  implementation needs a runtime migration/cleanup. Rejected: leaving the HTML
  in place; merely ignoring it in Git; a repository-only committed deletion.
- **Only the daemon-served renderer remains, `(1839)=a`.** After
  `261119-feat-ws-whiteboard-retire-embed-snapshot` removes the embed slice,
  remove the remaining standalone-document producer, template wrapper,
  polling/reload path, disk link/panel variants, and tests that exist only for
  `RenderPage`. Preserve `RenderRegions`, daemon-framed topic bodies,
  `PageAssets`, and shared renderer behavior needed by the served page.
  Rejected: keeping the shared template/renderer split for a disk page no
  supported path serves; retaining standalone-only tests as a dormant surface.
- **Mixed-version compatibility is T1 overall, `(1839)=a`.** The tool name,
  arguments, authoritative store shape, and successful `full:` label remain;
  the highest degradation is an old file-only viewer temporarily losing its
  derived page until an old binary renders it again. There is no authoritative
  data loss and no T3 old-version boundary.

  Evidence on `develop` at `785fd8fcefb6`:

  | Direction | Reads/writes and reopen result | Grade |
  |---|---|---|
  | An old running plugin/session calls a new daemon | The new producer still emits `full:` (`internal/mcp/whiteboard_tools.go:357-362`), and `dashboardPageURL` supplies the served URL (`internal/mcp/dashboard_sessions.go:735-752`). A production-plugin/playbook search finds no reader of `index.html` or `index.stamp.js`; representative plugin-package result strings are test fixtures (`agents-plugin/tests/test_ws_mcp_launcher_proxy.py:1505`, `agents-plugin-pi/test/mcp-stdio-client.test.ts:323`). No store write changes. | T0 |
  | An old binary/browser encounters files removed by the new cleanup | The standalone browser reads `index.html` and polls the stamp (`internal/wswhiteboard/assets/live.js:17-54`). Missing files degrade only that display. On its next render, the old-style binary prepares both files (`internal/mcp/whiteboard_tools.go:671-689`); the missing-file check returns false (`:698-707`) and publication recreates them (`:783-805`). The board store was retained, so reopening with the old binary loses no board data. | T1 until re-render, then restored |
  | A new binary encounters derived files recreated by an old binary | The files are derived output, not the store, and the new version neither needs them for `RenderRegions` (`internal/wswhiteboard/render.go:367-407`) nor changes board data when cleaning them. Reopening the new version reads the unchanged authoritative store. | T0 |

  Rejected: removing the `full:` line, which would degrade old caller text;
  retaining a disk-path value, which contradicts `(1839)=a`; treating harmless
  recreation of derived files as a store boundary. The approved no-daemon
  error removes the unsupported file-only operating mode; it does not change
  the MCP schema or establish an old/new store refusal. Re-grade before release
  if implementation makes a daemon-served legacy call fail or changes the
  board-store bytes.

## Constraints

- Ordering: implementation follows the merge of
  `261119-feat-ws-whiteboard-retire-embed-snapshot` and lands before
  `261119-refactor-ws-whiteboard-sqlite-board-store` Phase 2 moves the update
  path to the DB. The embed ticket owns `embed:`, `wb:embed-dir`,
  `LayoutEmbed`, `.Embed` template branches, embed state, and embed eviction;
  rebase and re-run the code-fact searches after it lands.
- The embed ticket's `(1820)=a` currently relies on the daily orphan GC and its
  `<reply-id>-<hash>.html` pattern. This ticket does not settle whether that GC
  remains or is replaced while the broad one-time HTML cleanup is introduced;
  do not remove it without resolving that cross-ticket contract.
- The one-time cleanup removes rendered HTML only. `(1839)=a` does not
  authorize deleting `index.stamp.js`, hashed Mermaid assets, the GC marker, or
  any authoritative board file.
- Read before implementation, per AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`, and `ai-docs/manuals/ws-mcp.md`; for the
  shipped playbook and mirrors, also read `skill-authoring.md` and
  `wsflow-mirroring.md`.
- Shipped text must be downstream-first and must not name this repository's
  ticket, commit, or release posture.
- The required edits to `agents-plugin/rsrc/whiteboard/whiteboard.md` and its
  byte-identical `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` mirror are
  a gated implementation step: prepare the exact diff describing the
  daemon-only board, stable `full:` URL, and no-daemon/start-daemon error, then
  obtain owner approval of that diff before committing it. Follow the
  mirroring manual for any generated Pi/package copies. This ticket's settled
  design does not itself authorize an unseen shipped-text diff.
- Out of scope: client-side rendering, payload redesign, and the SQLite board
  store. This ticket removes the disk page and the synchronous work that exists
  only to produce it.

## Phases

### Phase 1: Retire disk pages

Prerequisites and boundary:

1. Start from a base containing the completed embed-retirement merge. Confirm
   that its `.Embed`/`LayoutEmbed`, embed result/state, and eviction deletions
   are not duplicated here.
2. Keep the folder store authoritative and land this phase before the store
   ticket's Phase 2. Do not combine the page retirement with the DB switch.
3. Resolve the orphan-GC cross-ticket gap before changing the GC. The phase may
   remove no GC behavior that `(1820)=a` still requires.

Implementation scope, re-verified against the pre-embed `develop` lines above:

- Change the top-level update result so a successful `full:` is always the URL
  from the daemon-serving record (`whiteboard_tools.go:357-362`;
  `dashboard_sessions.go:735-752`). When that URL is unavailable, refuse with
  the owner-approved start-the-daemon hint and never auto-start. Cover the
  first-update/nudge-lag behavior documented at `dashboard_sessions.go:735-743`
  so a live daemon is not mistaken for an absent one.
- Delete the full-page render and publish path from
  `whiteboard_tools.go:563-812`: `refreshWhiteboardAfter`, the remaining
  `renderWhiteboard*` full-page branch, `index.html` / `index.stamp.js`
  comparison and memoization, Mermaid-to-page publication, and the derived
  output structs/publisher. Remove the now-dead `sessionRecord.boardOutput`
  plumbing (`internal/mcp/session_auth.go:113-120`;
  `internal/mcp/session_state.go:649-675`) while preserving durable
  commit/observation semantics.
- Delete `wswhiteboard.RenderPage` (`render.go:309-324`), the static document's
  top-level `page` template (`assets/page.tmpl:1-17`), `assets/live.js`,
  `StampFile`, static `FullBoardLink`/`FileLink`, `chgWire`, and unframed
  branches whose only caller was the disk document. Remove disk page-directory
  and file-link helpers and collapse panel providers onto `served=true`
  behavior (`internal/mcp/whiteboard_panels.go:62-97`, `:149-190`). Remove only
  static-document i18n subsetting; keep the frame subset.
- Keep `RenderRegions`, `RenderRegionsWithTopics`, `RenderClosedTopics`,
  `PageAssets`, frame assets, and every template/script branch exercised by the
  daemon page (`render.go:326-453`; `dashboard_page.go:280-330`). A textual
  search for `.Full` is not a deletion rule: the daemon uses `LayoutFull`, so
  delete a branch only after proving it is unreachable from those APIs.
- Implement `(1839)=a` as an idempotent downstream runtime cleanup, not a
  repository-only deletion: remove existing rendered HTML beneath
  `ai-docs/whiteboards/` once, including nested `index.html`, while preserving
  all non-HTML files and board directories. Coordinate root-level hashed embed
  HTML with the unresolved `(1820)=a` GC contract before finalizing the sweep.
- Prepare the canonical whiteboard playbook diff and wsflow mirror described in
  `## Constraints`; pause for the owner's diff approval before committing that
  shipped-text step.

Verification:

- Re-run `rg -l 'RenderPage\\(' agents-plugin-tool/internal/wswhiteboard
  --glob '*_test.go'`. On the pre-embed base it returns 19 files. Delete tests
  that assert only the standalone document, stamp polling, disk links, or
  unframed static behavior. Rewrite surviving behavior tests against
  `RenderRegions`, `RenderClosedTopics`, served-page capture, or frame assets.
  The final production and test trees have no `RenderPage` caller or disk-page
  golden fixture.
- Rewrite MCP publication/result tests to prove: a served update succeeds and
  reports exactly one `full:` daemon URL; an unserved session gets the approved
  error and start-daemon hint with no auto-start; updates and refresh-triggering
  todo/agenda/ticket/mailbox calls create or read no `index.html` or
  `index.stamp.js`; authoritative board changes still commit once.
- Exercise the one-time cleanup against a fixture containing root and nested
  rendered HTML plus representative `meta.json`, `pending.json`,
  `committed.json`, `.lock`, topic logs, panel bodies, `index.stamp.js`, Mermaid
  assets, and the GC marker. Assert the authorized HTML set is removed, every
  non-HTML/store byte is unchanged, a second run is a no-op, and the unresolved
  embed-GC boundary follows the owner resolution rather than an implementer
  guess.
- Compatibility: with `internal/wswhiteboard/testdata/compat-old` or an
  equivalent old-style harness, delete the derived files, open the retained
  board, render once, and show that the old binary harmlessly recreates its
  page/stamp without changing board state. Reopen with the new binary and show
  the same authoritative state. Verify old running caller text still receives
  a successful `full:` line when the daemon serves the session.
- Performance: on the post-embed base, trace one top-level
  `whiteboard.update` and one panel-refreshing action before and after. Report
  API-level and syscall-level file-operation counts and visible latency per
  action as required by `performance.md:63-73`, separating authoritative-store
  I/O from derived-page I/O. The after trace must show zero page/stamp reads,
  stats, creates, writes, renames, or Mermaid-file checks on either action.
  Measure the cleanup separately as a one-time migration and prove later
  actions do not rescan for it.
- Daemon-page capture: load the same representative board through the served
  session URL before and after, save the rendered capture, and compare board,
  panels, answers, lazy history, file actions, topic-body frames, images, and
  Mermaid behavior side by side. The after capture must remain interactive and
  must not request `index.html` or `index.stamp.js`.
- Verify the owner-approved canonical playbook and wsflow mirror are
  byte-identical where required, contain no file-only guidance, and describe
  the stable daemon URL/error behavior without repository-local references.
