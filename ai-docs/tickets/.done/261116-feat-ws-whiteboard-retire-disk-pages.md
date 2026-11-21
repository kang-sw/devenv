---
title: "화이트보드 디스크 렌더링 페이지(파일 전용 모드) 폐지"
related:
  261119-feat-ws-whiteboard-retire-embed-snapshot: "landed prerequisite sibling; owns the embed result, LayoutEmbed/template branches, embed state, and embed-file cleanup contract (ai-docs/tickets/.done/261119-feat-ws-whiteboard-retire-embed-snapshot.md)"
  261119-refactor-ws-whiteboard-sqlite-board-store: "store sibling; this retirement lands after embed retirement and before the store ticket's Phase 2 switches the update path"
  261116-feat-ws-dashboard-session-page-payload-and-render-cost: "sibling; explicitly excludes this retirement"
  261116-feat-ws-dashboard-generalized-mentions: "sibling undesigned todo from the same day"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1c50ef9fbaf5a1a2
sage-review-completeness-reviewed: 1c50ef9fbaf5a1a2
completed: 2026-11-20
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


### Result (8fc6f843f) - 2026-11-20

Completed the disk-page retirement in `e629248ed`, `cc76ef0b5`, and
`6b30df09a`, followed by owner-approved shipped guidance and all generated
copies in `8fc6f843f`. The authoritative folder store and successful update
commit remain unchanged; `full:` now carries the served URL or the
construction-appropriate status. Standalone rendering, publication, stamp
polling, and recurring GC are removed. The daemon keeps served regions,
frames, shared assets, and one asynchronous closed-set cleanup per project/cache
identity. Live recovery snapshots retain their atomic writer and Windows retry.

Owner item `(1861)=ok` approved the daemon introduction and the exact two-line
Calls bullet in the [saved patch](../assets/261116-feat-ws-whiteboard-retire-disk-pages/proposed-whiteboard.patch).
The lead confirmed at 12:51:56Z that this shorter wording replaces the earlier
three-line draft. Canonical, wsflow, and Pi resource copies are byte-identical;
workflow archives and both native first-party payloads were regenerated. No
other shipped prose changed. The current board handoff already uses ZIPs.

Verification completed:

- Full `internal/wswhiteboard`: PASS; focused MCP retirement, cleanup, panel
  freshness, live snapshot, old-store reopen, and constructor ownership checks:
  PASS after correcting the lazy-history assertion and synchronizing the
  initial cleanup in the test. The implementation checkpoint below records
  the commands, original failure, focused reruns, and skipped opt-in fixtures.
- Actual daemon Chromium baseline/after checks: PASS for answers, file actions,
  lazy history, sandboxed frames, loaded images, Mermaid, and auxiliary panels.
  Compared `before-{board,image,mermaid,panels}.png` with the corresponding
  `after-*.png` in the linked assets directory: match apart from generated IDs,
  clocks, and fixture counters. No retired page/stamp request occurred.
- Final `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  ./internal/wsrsrc/... -count=1 -timeout 5m`: PASS. Its first run caught two
  stale Pi resource files; synchronizing them from canonical fixed the drift.
  wsflow bundle tests: 10 PASS. Pi bundle freshness and workflow archive
  `--check`: PASS. Native first-party regeneration is byte-idempotent.
- Fresh Lite review of `f2234e573..cc76ef0b5`: no correctness or test-integrity
  findings, remaining none. The [verbatim report](../assets/261116-feat-ws-whiteboard-retire-disk-pages/review.md)
  is now recovered. `6b30df09a` only synchronized a test with the initial
  sweep; the reviewed production source did not change. The final prose is
  the subsequently approved shorter variant.
- T1 compatibility remains supported by the old harness recreating derived
  page/stamp files with all six authoritative file hashes and decoded state
  unchanged, followed by successful new-version reopen. Read/write source
  evidence is retained in the checkpoint below.

Supporting macOS instrumented API measurements, from
[measurements.json](../assets/261116-feat-ws-whiteboard-retire-disk-pages/measurements.json):

| API counter | First update before -> after | Steady update before -> after | Agenda before -> after |
|---|---|---|---|
| Open | 20 -> 20 | 16 -> 16 | 10 -> 0 |
| OpenFile | 1 -> 1 | 1 -> 1 | 0 -> 0 |
| ReadFile | 27 -> 25 | 44 -> 42 | 43 -> 5 |
| Stat | 39 -> 38 | 29/38/38/38 -> 27/36/36/36 | 30 -> 5 |
| CreateTemp | 6 -> 6 | 4 -> 4 | 4 -> 1 |
| AtomicReplace | 6 -> 6 | 4 -> 4 | 4 -> 1 |
| Total | 99 -> 96 | 98/107/107/107 -> 94/103/103/103 | 91 -> 12 |

The `partAll` to `partWhiteboard` reduction removes two instrumented ReadFile
calls per update. These are API counters, not syscall totals: direct `os.*`
operations and individual writes/renames/removes inside helpers are not fully
counted. Artifact snapshots separately show update derived output falling from
two files (349,770-358,339 bytes) to zero, with identical authoritative byte
deltas. Agenda writes no board bytes after retirement (previously 206
authoritative and 349,060 derived bytes). Steady update median latency falls
from 80.38 to 58.86 ms; agenda falls from 45.47 to 0.559 ms. The separate cleanup
fixture removes seven retired files, preserves the closed-set exclusions,
returns while a sweep is blocked, and performs no repeat scan or marker write.

**Windows AV/syscall baseline: deferred to lead post-merge smoke-host measurement.**
The lead owns this measurement on the Windows smoke host before the next ship,
together with the holder baseline. Its explicit 11:35Z/11:45Z and later
confirmations accept these macOS measurements as branch supporting evidence;
missing Windows/syscall evidence does not block closure. No further tracing or
privilege escalation was attempted after that instruction became visible.

Deferred findings and operational evidence:

- The existing pinned `workflow-bundle-consumer.test.mjs` failure on committed
  producer `f2234e573` is outside this change; the lead will verify it on
  develop after merge. Its launcher/config and macOS path-alias failures are
  recorded in the checkpoint.
- For `261120-bug-ws-agent-dogfood-delivery-context` items 1-3: the lead's
  11:49Z approval and later replies did not become visible during the active
  worker turn; they arrived as a delayed batch after continuation, causing an
  unnecessary gate report. The grandchild reviewer report reached the root
  lead before it was visible to this worker. Both requested report paths were
  rejected by the reviewer's read-only sandbox; the full mailbox report is now
  saved by this worker. Renderer completion reports were similarly delayed.

No implementation finding remains. Merge is reserved to the lead with
`merge_confirm: ask`; no branch was merged or pushed, shared daemon stopped,
or holder / `internal/wsagent` source modified.

## Implementation checkpoint - 2026-11-20 (historical)

This checkpoint records the earlier paused state; the Result above resolves
its approval, evidence, and report-delivery gates.

Code is committed on `impl/develop/stuck-crave-groin`: renderer retirement
`e629248ed`, MCP/cleanup/construction retirement `cc76ef0b5`, and test-only
cleanup synchronization `6b30df09a`. The phase remains open for the owner
gates below.

### Implemented behavior

- Updates commit the existing folder store once and return a served URL,
  `page not yet served` for daemon ownership without a URL, or `page unavailable`
  for the interval raw diagnostic call. Proxy and hosted CLI construction set
  `BoardOwner` even when `Live` is nil.
- Todo, agenda, ticket, and mailbox mutations retain daemon nudges and no longer
  render or mutate the authoritative board for panel refresh. The update also
  stops reading agenda/todo parts that only the removed publisher needed.
- Successful daemon board opens share one background cleanup guard per canonical
  project/cache identity. No response waits for the sweep and no marker is
  written. The closed set preserves unrelated flat HTML, old flat stamp names,
  the pre-existing GC marker, live embeds, host symlinks, directories, and every
  authoritative file. An unreadable session listing preserves embeds.
- The removed page writer also served live recovery snapshots; that still-used
  atomic replacement and Windows sharing-violation retry moved into
  `live_snapshot_file.go`. Removing it would have broken an unrelated live path.

### Verification obtained

All Go commands used `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.
`internal/mcp` ran separately with `-timeout 45m`; no shared daemon or holder
was stopped or modified.

- `go test ./internal/wswhiteboard -count=1 -timeout 5m`: PASS (250.629s).
- Focused MCP regex
  `Whiteboard|Dashboard(ReadPage|Folder|Stamp|Pointer|Served|SingleNudge|TopicTicket)|LiveSnapshotReplacement`:
  123 top-level tests passed. One migrated assertion incorrectly expected a
  settled comment in the initial page; it now checks the actual lazy-history
  response. Its focused rerun passed. Opt-in fixture/measurement tests ran
  separately below; the unrelated idle CPU fixture remained skipped.
- `TestWhiteboardConstructionBeforeDashboardBind`: PASS for proxy, hosted CLI,
  and raw CLI with no bound dashboard.
- An equivalent old-renderer harness compiled from `f2234e573` recreated
  deleted page/stamp files, including the page's stamp reference. All six
  authoritative file hashes and decoded state stayed identical. The new
  `TestWhiteboardReopensOldRecreatedPages` then passed against the same folder.
  The folder reader/writer remain `internal/wswhiteboard/folders.go:146,183`;
  `cc76ef0b5:internal/mcp/session_state.go:657` retains that writer, and
  `whiteboard_tools.go:286` changes only the successful `full:` value.
  The old producer at `f2234e573:internal/mcp/whiteboard_tools.go:625-646`
  recreates the derived page/stamp. This supports the settled T1 verdict.
- `TestWhiteboardRetirementClosedSet`, unknown-liveness preservation, and the
  blocked-sweep first-open/shared-guard test: PASS. The fixture deletes exactly
  seven retired files; a blocked cleanup does not delay the first read, and
  subsequent reads/actions schedule no second scan or marker.
- Real temporary daemon Chromium before/after: PASS for answer selection,
  file actions, lazy topic history, sandboxed frames, loaded images, rendered
  Mermaid, and auxiliary panels. Neither run requested `index.html` or
  `index.stamp.js`. The captures below match apart from generated IDs, clocks,
  and fixture host counters. The fixture has no notes provider; that panel's
  matching unavailable state is not a retirement regression.
- Dashboard `TestPagesCarryScopedSelectAllScript` and
  `TestScopedSelectAllBrowser`: PASS. The separate external unified-history
  harness was not configured and remained skipped.
- wsflow skill bundle tests: 10 PASS. Pi bundle freshness: PASS. The pinned
  `workflow-bundle-consumer.test.mjs` failed on its already committed producer
  `f2234e573` with `Incomplete Claude launcher or config references` and a
  macOS `/var` versus `/private/var` identity assertion; it did not consume the
  uncommitted proposed prose bundles. Reported to the lead as an existing
  package validation issue, not changed here.
- Search finds no current `RenderPage` caller, `StampFile`, `DocFramed`,
  `refreshWhiteboardAfter`, or recurring GC producer. Only the intentionally
  historical `testdata/compat-old` driver retains its old calls.

| Capture | Before | After |
|---|---|---|
| Board/history/answers | [before](../assets/261116-feat-ws-whiteboard-retire-disk-pages/before-board.png) | [after](../assets/261116-feat-ws-whiteboard-retire-disk-pages/after-board.png) |
| Image/frame | [before](../assets/261116-feat-ws-whiteboard-retire-disk-pages/before-image.png) | [after](../assets/261116-feat-ws-whiteboard-retire-disk-pages/after-image.png) |
| Mermaid | [before](../assets/261116-feat-ws-whiteboard-retire-disk-pages/before-mermaid.png) | [after](../assets/261116-feat-ws-whiteboard-retire-disk-pages/after-mermaid.png) |
| Auxiliary panels | [before](../assets/261116-feat-ws-whiteboard-retire-disk-pages/before-panels.png) | [after](../assets/261116-feat-ws-whiteboard-retire-disk-pages/after-panels.png) |

### Supporting performance evidence

These are macOS fixture timings and instrumented API counters, not the required
Windows antivirus baseline or syscall totals. The counters omit direct `os.*`
calls; byte snapshots report resulting file changes, not transient operations.
Full rows are in [measurements.json](../assets/261116-feat-ws-whiteboard-retire-disk-pages/measurements.json).

| Action | Before | After |
|---|---|---|
| First update | 163.58 ms; 99 instrumented operations | 104.76 ms; 96 operations |
| Steady update (4 samples) | median 80.38 ms; p95 96.03 ms; 98/107/107/107 operations | median 58.86 ms; p95 70.51 ms; 94/103/103/103 operations |
| `agenda.set` | 45.47 ms; 91 operations | 0.559 ms; 12 operations |
| Update derived output | two files; 349,770-358,339 bytes | zero files/bytes |
| Agenda derived output | two files; 349,060 bytes | zero files/bytes |
| Agenda authoritative board change | two files; 206 bytes | zero files/bytes |

The update's authoritative byte deltas matched before/after (981 bytes on the
first update, then 1,249/1,604/1,960/2,315). The cleanup test runs separately
from these raw-dispatch action measurements; privileged syscall-level cleanup
and action traces remain uncollected.

### Pending gates and handoff

1. Owner approval of the [exact proposed shipped prose diff](../assets/261116-feat-ws-whiteboard-retire-disk-pages/proposed-whiteboard.patch)
   is still required by this ticket. The canonical/wsflow prose, manifests,
   and generated Pi copies are prepared but uncommitted. The existing handoff
   procedure already uses a ZIP; no obsolete board-copy procedure was recreated.
2. A Windows antivirus baseline and privileged syscall traces were unavailable
   in this environment. `dtruss` refused the available privileges. Obtain an
   authorized measurement host, or explicitly decide how this evidence gate
   is to be handled before closing the phase.
Independent Lite reviewer `id:resistantachinessreaffirmsled` returned `clean`
for `f2234e573..cc76ef0b5`, confirmed through `agent.list` after automatic
completion and mailbox delivery failed. The requested detailed report was
not visible at its `/tmp` path or the shared-worktree recovery path. The
[worker-observed review status](../assets/261116-feat-ws-whiteboard-retire-disk-pages/review-status.json)
records that evidence and limitation; a direct report copy to the root lead
was requested. No Critical, Important, or Minor finding was reported.

After that verdict, `6b30df09a` only makes the panel-action test wait for its
first asynchronous sweep before creating old-output sentinels. This prevents
a slow cleanup from racing the later-action assertion. Both the byte-preserving
panel-action test and asynchronous/shared cleanup test passed in a focused
rerun; production code is unchanged from the reviewed head. Lite is one pass,
so no second reviewer was dispatched.

The branch has not been merged or pushed. `merge_confirm: ask` remains with
the lead. During delegation, the renderer reached an idle/done state and
committed, but its completion report was not delivered to this worker; the
worker verified the retained source itself and reported the session-key/
completion-delivery discrepancy to the lead for a follow-up at the merge point.

The checkpoint append triggered a Sage freshness advisory. The original ready
ticket body is a byte-identical prefix of this file; its decisions, constraints,
and phase plan were not revised, so no design/completeness rerun was invoked.
