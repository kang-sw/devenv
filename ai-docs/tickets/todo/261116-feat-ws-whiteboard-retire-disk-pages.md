---
title: "화이트보드 디스크 렌더링 페이지(파일 전용 모드) 폐지"
related:
  261119-feat-ws-whiteboard-retire-embed-snapshot: "landed prerequisite sibling; owns the embed result, LayoutEmbed/template branches, embed state, and embed-file cleanup contract (ai-docs/tickets/.done/261119-feat-ws-whiteboard-retire-embed-snapshot.md)"
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
`(1839)=a` on 2026-11-20 approved the five initial decisions below. Whiteboard
item `(1841)=a` on 2026-11-20 confirmed G1-G4 and resolved their cleanup and
compatibility details. Owner items `(1844)=a` and `(1852)=a` later replaced
G3's no-daemon branch with the raw-binary interval described below. This is
functionality removal, so that approval is the AGENTS.md `### Approval
Protocol` authority for the settled design.

### Code facts

These facts were re-verified on pre-embed `develop` at `785fd8fcefb6`; paths
below are relative to `agents-plugin-tool/`. The post-embed commit `fb5e10db5`
is now merged into `develop` (`0c375a5ee`;
`ai-docs/tickets/.done/261119-feat-ws-whiteboard-retire-embed-snapshot.md`);
its changed facts are recorded below.

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
- On the merged post-embed commit `fb5e10db5`, `renderWhiteboardPages` is
  full-page-only
  (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L585-L640`), the `page`
  template has only a full-page wrapper
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L1-L16`), and
  `FullBoardLink` is absent from the renderer/view source (`git grep -n
  'FullBoardLink' fb5e10db5 --
  agents-plugin-tool/internal/wswhiteboard` returned nothing). The full-page
  publisher and daily orphan GC remain
  (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L721-L757`,
  `agents-plugin-tool/internal/mcp/whiteboard_tools.go#L847-L886`).

## Decisions

- **Drop G3's no-daemon branch, `(1844)=a` (owner, 2026-11-20 17:31; note
  17:30).** "all MCP functions presuppose the daemon; the no-daemon scenario is
  meaningless." SQLite decision `(1806)` stands: once the SQLite store lands,
  non-owner processes refuse board calls. This ticket therefore does not add
  G3's commit-plus-`full:` error/start-daemon-hint branch. Rejected: preserving
  that branch as a supported no-daemon mode.
- **Raw-binary calls keep committing only until SQLite Phase 2, `(1852)=a`
  (owner, 2026-11-20 17:44).** In the interval after this ticket lands and
  before the SQLite ticket's Phase 2, a diagnostic raw `ws-mcp` board update
  (direct `ws-mcp serve --stdio` or `ws-mcp call whiteboard.update`) still runs
  in-process and commits (`cmd/ws-mcp/main.go:19-77`,
  `cmd/ws-mcp/hosted_cli.go:508-539`, verified on `develop` at `06e615956`). Its
  `full:` line says only that the page is unavailable, with no start-daemon
  hint. `(1806)` introduces refusal in SQLite Phase 2. This ticket remains T1.
  Rejected: pulling `(1806)`'s refusal into this ticket, which would make it T3
  and require a minor bump.
- **A daemon session without a URL reports that its page is not yet served
  (technical, lead).** A registered daemon-served session can lack a URL before
  its first successful nudge, and an unregistered session also has no
  `dashboardPageURL` (`internal/mcp/dashboard_sessions.go:735-752`, verified on
  `develop` at `06e615956`). Its `full:` line reports "page not yet served"; it
  does not diagnose daemon absence. Rejected: treating a missing nudge-derived
  URL as evidence that no daemon exists.
- **Construction, not live dashboard state, distinguishes daemon ownership
  (technical, lead).** Add a daemon-only `ServerOptions.BoardOwner` flag in this
  ticket, before `(1806)` uses the same option for refusal. Set it at server
  construction when the server belongs to the daemon, including a hosted
  one-shot CLI (`hostedCLI.server` has `c.host != nil`,
  `cmd/ws-mcp/hosted_cli.go:74-95`, and `mcpCommand` uses that shared constructor
  at `:527-532`) and an accepted proxy connection
  (`cmd/ws-mcp/proxy.go:90-103`). Leave it false for direct raw `serve --stdio`
  and manual `ws-mcp call`. This ticket uses the flag only to choose between
  "page not yet served" and "page unavailable"; it does not implement
  `(1806)`'s refusal. Rejected: inferring ownership from `ServerOptions.Live` or
  `WhiteboardChanged`, because `Live` is nil before the dashboard binds and the
  nudge callback is not an ownership contract.
- **The authoritative board data store stays, `(1839)=a`.** Retire only
  rendered page artifacts and their producer/consumer branches. Keep every
  board-state file and the current board semantics until the SQLite-store
  ticket changes them. Rejected: retiring the board folder with the pages;
  moving the store in this ticket.
- **The one-time sweep has a closed artifact set and ends dead-session GC, G1
  and G4, `(1841)=a`.** In downstream projects, delete only `index.html`,
  `index.stamp.js`, the hashed Mermaid copies, and embed files belonging to
  sessions that are not live. Preserve every authoritative board-data file and
  live session's embed files. After the sweep, remove the standing dead-session
  GC role. G4 confirms that `index.stamp.js` and Mermaid copies leave with the
  HTML (source: owner `(1839)`, board panel `p487`). A deletion commit in this
  repository alone cannot satisfy the downstream contract. Rejected: leaving
  the retired page artifacts in place; merely ignoring them in Git; a
  repository-only committed deletion; keeping a standing GC for embed
  directories, which would overlap the one-time dead-session embed cleanup
  previously assigned by `(1820)=a`.
- **The daemon runs the cleanup off the response path after the first board
  open, G2, `(1841)=a`.** The sweep is idempotent and guarded once in process
  memory; it writes no marker file and does not delay the first-open response.
  Rejected: a marker file, because it adds a file touch and records nothing
  needed for recovery; running the deletion sweep on the response path.
- **Only the daemon-served renderer remains, `(1839)=a`.** After
  `261119-feat-ws-whiteboard-retire-embed-snapshot` removes the embed slice,
  remove the remaining standalone-document producer, template wrapper,
  polling/reload path, disk link/panel variants, and tests that exist only for
  `RenderPage`. Preserve `RenderRegions`, daemon-framed topic bodies,
  `PageAssets`, and shared renderer behavior needed by the served page.
  Rejected: keeping the shared template/renderer split for a disk page no
  supported path serves; retaining standalone-only tests as a dormant surface.
- **Mixed-version compatibility is T1 overall, `(1839)=a`, as revised by
  `(1844)=a` and `(1852)=a`.** The tool name, arguments, authoritative store
  shape, update commit, and `full:` label remain. The highest degradation is
  display-only: an interval raw-binary diagnostic call gets an unavailable
  status instead of a file path, a daemon session whose nudge-derived URL is
  not ready gets "page not yet served", and an old file-only viewer can
  temporarily lose its derived page until an old binary renders it again.
  There is no authoritative data loss and no T3 old-version boundary.

  Evidence on `develop` at `785fd8fcefb6`:

  | Direction | Reads/writes and reopen result | Grade |
  |---|---|---|
  | An old running plugin/session calls a new daemon | The new producer still emits `full:` (`internal/mcp/whiteboard_tools.go:357-362`), and `dashboardPageURL` supplies the served URL (`internal/mcp/dashboard_sessions.go:735-752`). A production-plugin/playbook search finds no reader of `index.html` or `index.stamp.js`; representative plugin-package result strings are test fixtures (`agents-plugin/tests/test_ws_mcp_launcher_proxy.py:1505`, `agents-plugin-pi/test/mcp-stdio-client.test.ts:323`). No store write changes. | T0 |
  | A diagnostic raw binary calls the in-process handler before SQLite Phase 2 | `serve --stdio` and one-shot `call` still construct an in-process server (`cmd/ws-mcp/main.go:19-77`, `cmd/ws-mcp/hosted_cli.go:508-539`). The update commits; `full:` says the page is unavailable, with no start-daemon hint. Phase 2 of `261119-refactor-ws-whiteboard-sqlite-board-store` later introduces `(1806)`'s refusal. | T1 until Phase 2 |
  | A daemon-served session has no nudge-derived URL yet | `dashboardPageURL` returns false until the session is registered and a nudge has recorded a served port (`internal/mcp/dashboard_sessions.go:735-752`). The update still commits and `full:` says "page not yet served", without claiming daemon absence. | T1 |
  | An old binary/browser encounters files removed by the new cleanup | The standalone browser reads `index.html` and polls the stamp (`internal/wswhiteboard/assets/live.js:17-54`). Missing files degrade only that display. On its next render, the old-style binary prepares both files (`internal/mcp/whiteboard_tools.go:671-689`); the missing-file check returns false (`:698-707`) and publication recreates them (`:783-805`). The board store was retained, so reopening with the old binary loses no board data. | T1 until re-render, then restored |
  | A new binary encounters derived files recreated by an old binary | The files are derived output, not the store, and the new version neither needs them for `RenderRegions` (`internal/wswhiteboard/render.go:367-407`) nor changes board data when cleaning them. Reopening the new version reads the unchanged authoritative store. | T0 |

  Rejected: removing the `full:` line, which would degrade old caller text;
  retaining a disk-path value, which contradicts `(1839)=a`; treating harmless
  recreation of derived files as a store boundary; pulling `(1806)`'s raw-call
  refusal forward from SQLite Phase 2, which would make this ticket T3. Re-grade
  before release if implementation refuses the interval raw-binary call, makes
  a daemon-served legacy call fail, or changes the board-store bytes.

## Constraints

- Ordering: implementation follows the merge of
  `261119-feat-ws-whiteboard-retire-embed-snapshot` and lands before
  `261119-refactor-ws-whiteboard-sqlite-board-store` Phase 2 moves the update
  path to the DB. The embed ticket owns `embed:`, `wb:embed-dir`,
  `LayoutEmbed`, `.Embed` template branches, embed state, and embed eviction;
  rebase onto the merged post-embed base and re-run the code-fact searches.
- G1 supersedes the standing-GC part of the embed ticket's `(1820)=a`: the
  sweep scheduled by the first board open removes dead sessions' embed files
  once off the response path, then the dead-session GC role and its
  marker/cadence are removed. Host-owned embed folders remain outside the
  repository sweep.
- The cleanup's closed deletion set is G1/G4: `index.html`, `index.stamp.js`,
  hashed Mermaid copies, and dead sessions' embed files. It never deletes an
  authoritative board file or a live session's embed file.
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
  daemon-only board and this stable rule: a URL-valued `full:` is copyable;
  any non-URL `full:` value is a status to surface, not copy. Obtain owner
  approval of that diff before committing it. Follow the
  mirroring manual for any generated Pi/package copies. This ticket's settled
  design does not itself authorize an unseen shipped-text diff.
- Per b38152d8, board-copy consumes `full:` only when its value is a page URL.
  An unavailable/error or "page not yet served" line is not a board stem: the
  procedure surfaces that status and does not copy, parse, or synthesize a URL.
- Out of scope: client-side rendering, payload redesign, and the SQLite board
  store. This ticket removes the disk page and the synchronous work that exists
  only to produce it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

## Prior Decisions

- 261119-feat-ws-whiteboard-retire-embed-snapshot (2026-11-19, Decisions): "The slice covers `embed:`, `wb:embed-dir`, old text that emits the embed line, host embed users, and embed eviction." — bearing: constrains
- 261119-refactor-ws-whiteboard-sqlite-board-store (2026-11-19, Decisions (1812)): "A per-board coalescing writer publishes `index.html` / `index.stamp.js`: latest state wins, stamp after page." — bearing: mechanism superseded by owner `(1841)=a`
- b38152d8 (2026-11-18, commit): "Kept `full:` as a bare page URL rather than dropping the line: the playbook's board-copy procedure derives the board stem from its last path segment" — bearing: constrains
- 261109-feat-ws-whiteboard-topic-body-out-of-root-images (2026-11-09, Decisions): "a LAN viewer, pages written to disk, and the embed keep today's path text for an out-of-root image." — bearing: constrains
- 261108-feat-ws-dashboard-disk-assets-dev-mode (2026-11-08, Decisions): "the MCP server's whiteboard page file and embed snapshot keep the embedded assets." — bearing: constrains
- 261107-feat-ws-dashboard-link-actions (2026-11-07, Decisions): "the click menu applies to the live session page conversation and the served whiteboard. Board files written to disk (embeds, Orca) keep today's `file:` links." — bearing: constrains
- 261107-feat-ws-dashboard-pull-down-drawer (2026-11-07, Decisions): "the session page and the daemon-served board page only. The disk board file (`ai-docs/whiteboards/*.html`, opened outside the daemon) keeps its panels as today." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/hosted_cli.go, agents-plugin-tool/cmd/ws-mcp/proxy.go, agents-plugin-tool/internal/mcp/whiteboard_tools.go, agents-plugin-tool/internal/mcp/whiteboard_panels.go, agents-plugin-tool/internal/mcp/session_state.go, agents-plugin-tool/internal/mcp/server_dispatch.go, agents-plugin-tool/internal/mcp/dashboard_sessions.go, agents-plugin-tool/internal/mcp/dashboard_page.go, agents-plugin-tool/internal/wswhiteboard/render.go, agents-plugin-tool/internal/wswhiteboard/view.go, agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl, agents-plugin-tool/internal/wswhiteboard/assets/live.js, agents-plugin/rsrc/whiteboard/whiteboard.md, agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md |
| scope.surface | cross-module | MCP `full:` result status, wswhiteboard rendering, daemon page, and shipped whiteboard playbooks |
| scope.new_public_symbol | yes | add exported internal field `ServerOptions.BoardOwner`; RenderPage and disk-only fields are removed |
| scope.new_type_contract | yes | add internal `ServerOptions.BoardOwner bool` at construction; `full:` keeps its label and carries a served URL, raw-binary unavailable status, or daemon-session not-yet-served status |
| scope.test_surface | existing | agents-plugin-tool/cmd/ws-mcp/hosted_cli_test.go and proxy tests cover construction; agents-plugin-tool/internal/wswhiteboard/render_test.go is among 19 RenderPage test files on both bases; agents-plugin-tool/internal/mcp/whiteboard_tools_test.go, agents-plugin-tool/internal/wswhiteboard/testdata/bodytime-hover.go, and agents-plugin-tool/internal/wswhiteboard/testdata/compat-old/main.go exist |
| complexity.reuse_points | confirmed | dashboardPageURL in agents-plugin-tool/internal/mcp/dashboard_sessions.go#L735-L752; RenderRegionsWithTopics and PageAssets in agents-plugin-tool/internal/wswhiteboard/render.go#L375-L451; agents-plugin-tool/internal/wswhiteboard/testdata/compat-old/main.go |
| complexity.side_effect_risk | high | removes synchronous publication and the standing GC, then performs the closed-set first-open sweep while preserving board data and live embeds |
| risk.correctness | high | daemon liveness can lag the nudge; cleanup and update must not alter committed board state or non-target files |
| risk.fit | moderate | G1 resolves the embed-GC overlap and `(1841)=a` removes the store ticket's obsolete Phase 3 derived publisher |
| risk.test | high | 19 RenderPage test files, MCP publication tests, cleanup fixtures, browser capture, compatibility harness, and file-operation trace |
| risk.security_or_contract | high | interval raw-binary updates still commit but report no page URL, daemon sessions can report not-yet-served, and the off-path first-open sweep deletes a closed set of downstream derived artifacts |

## Phases

### Phase 1: Retire disk pages

Prerequisites and boundary:

1. Start from a base containing the completed embed-retirement merge. Confirm
   that its `.Embed`/`LayoutEmbed`, embed result/state, and eviction deletions
   are not duplicated here.
2. Keep the folder store authoritative and land this phase before the store
   ticket's Phase 2. Do not combine the page retirement with the DB switch.
3. Apply G1 as the owner-confirmed replacement for `(1820)=a`'s standing
   dead-session GC: sweep dead embed files once, then remove that GC role.

Implementation scope, checked against the post-embed commit `fb5e10db5`:

- Change the top-level update result so a supported daemon-served call's `full:`
  is the URL from the daemon-serving record
  (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L340-L344`;
  `agents-plugin-tool/internal/mcp/dashboard_sessions.go#L735-L752`). A
  daemon-served session without a nudge-derived URL still commits and reports
  "page not yet served". Before SQLite Phase 2, a direct raw-binary diagnostic
  update also commits but reports only that the page is unavailable. Neither
  status carries a start-daemon hint or a disk path, and neither is fed to
  board-copy as a URL.
- Add `ServerOptions.BoardOwner` as the construction-time discriminator and set
  it for daemon proxy and hosted one-shot servers, including while `Live` is
  still nil; leave it false on direct raw-binary servers. Use it here only for
  `full:` wording. SQLite Phase 2 later reuses it for `(1806)`'s refusal.
- Delete the full-page render and publish path from
  `agents-plugin-tool/internal/mcp/whiteboard_tools.go#L545-L757`:
  `refreshWhiteboardAfter`, the remaining
  `renderWhiteboard*` full-page branch, `index.html` / `index.stamp.js`
  comparison and memoization, Mermaid-to-page publication, and the derived
  output structs/publisher. Remove the now-dead `sessionRecord.boardOutput`
  plumbing (`internal/mcp/session_auth.go:113-120`;
  `internal/mcp/session_state.go:649-675`) while preserving durable
  commit/observation semantics. Its refresh also bumps `Whiteboard.Seq`: update
  the stale `boardMemoBase` comment that says Seq covers panel inputs only when
  refresh succeeds, and replace the explicit refresh caller in
  `internal/mcp/dashboard_wake_reads_test.go:88` with the post-retirement
  freshness trigger the test actually exercises.
- Delete `wswhiteboard.RenderPage`
  (`agents-plugin-tool/internal/wswhiteboard/render.go#L308-L322`), the static
  document's top-level `page` template
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L1-L16`),
  `assets/live.js`,
  `StampFile`, static `FileLink`, `chgWire`, and unframed
  branches whose only caller was the disk document. Remove disk page-directory
  and file-link helpers and collapse panel providers onto `served=true`
  behavior (`agents-plugin-tool/internal/mcp/whiteboard_panels.go#L62-L94`,
  `agents-plugin-tool/internal/mcp/whiteboard_panels.go#L146-L180`). Remove only
  static-document i18n subsetting; keep the frame subset.
- Keep `RenderRegions`, `RenderRegionsWithTopics`, `RenderClosedTopics`,
  `PageAssets`, frame assets, and every template/script branch exercised by the
  daemon page (`agents-plugin-tool/internal/wswhiteboard/render.go#L365-L451`;
  `agents-plugin-tool/internal/mcp/dashboard_page.go#L280-L330`). A textual
  search for `.Full` is not a deletion rule: the daemon uses `LayoutFull`, so
  delete a branch only after proving it is unreachable from those APIs.
- On the daemon's first board open, schedule G1's idempotent, in-memory-once
  downstream sweep off the response path, with no marker file. Remove only
  every board directory's `index.html`, `index.stamp.js`, and hashed Mermaid
  copies, plus repository embed files whose reply ID is not live. Preserve board
  data, directories, live embeds, and host-owned embed folders. Remove the old
  daily dead-session GC marker/cadence/path after the sweep takes over its
  remaining role.
- Prepare the canonical whiteboard playbook diff and wsflow mirror described in
  `## Constraints`; pause for the owner's diff approval before committing that
  shipped-text step.

Verification:

- Re-run `rg -l 'RenderPage\\(' agents-plugin-tool/internal/wswhiteboard
  --glob '*_test.go'`. Piped through `wc -l` at `fb5e10db5`, it returns 19
  files. Delete tests
  that assert only the standalone document, stamp polling, disk links, or
  unframed static behavior. Rewrite surviving behavior tests against
  `RenderRegions`, `RenderClosedTopics`, served-page capture, or frame assets.
  The final current-version production and test trees have no `RenderPage`
  caller or disk-page golden fixture; the intentionally old compat harness
  retains its old-version call
  (`agents-plugin-tool/internal/wswhiteboard/testdata/compat-old/main.go#L42-L46`).
- Rewrite MCP publication/result tests to prove: a daemon-served update succeeds
  and reports exactly one `full:` URL; a daemon session before registration or
  its first successful nudge still commits and reports "page not yet served";
  and a direct raw-binary update before SQLite Phase 2 commits and reports page
  unavailable with no start-daemon hint. Status lines are not accepted by the
  board-copy procedure as URLs. Updates and todo/agenda/ticket/mailbox writes
  create or read no `index.html` or `index.stamp.js`; after retiring
  `refreshWhiteboardAfter` and its Seq bump, each of those writes still makes the
  served panel fresh. Replace the explicit caller at
  `internal/mcp/dashboard_wake_reads_test.go:88` and prove authoritative board
  changes still commit once.
- Cover a daemon-hosted session whose connection is accepted before the
  dashboard binds: with `Live == nil`, its update reports "page not yet served",
  not "page unavailable". Cover the same distinction for the daemon-hosted
  one-shot CLI and direct raw-binary call so construction ownership, not
  `Live`/`WhiteboardChanged`, decides the status.
- Exercise the first-board-open cleanup against a fixture containing live and
  dead session embeds, nested page artifacts, unrelated HTML, and representative
  `meta.json`, `pending.json`,
  `committed.json`, `.lock`, topic logs, panel bodies, `index.stamp.js`, Mermaid
  assets, and the old GC marker. Assert exactly G1/G4's closed set is removed,
  board/store bytes, unrelated HTML, and live embeds are unchanged, the first
  response does not wait for the sweep, a second open in the same daemon
  performs no scan, and no marker file is created.
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
  Measure the cleanup separately on first board open and prove later opens and
  actions in that daemon do not rescan or write a marker.
- Daemon-page capture: load the same representative board through the served
  session URL before and after, save the rendered capture, and compare board,
  panels, answers, lazy history, file actions, topic-body frames, images, and
  Mermaid behavior side by side. The after capture must remain interactive and
  must not request `index.html` or `index.stamp.js`.
- Verify the owner-approved canonical playbook and wsflow mirror are
  byte-identical where required, contain no file-only guidance, and describe
  URL-valued `full:` and generic non-URL status/board-copy behavior without
  repository-local references.
