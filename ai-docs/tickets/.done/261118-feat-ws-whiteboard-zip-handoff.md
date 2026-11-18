---
title: "화이트보드 zip 다운로드와 whiteboard.load로 보드 인계하기"
related:
  261117-refactor-playbook-compression: landing-order - its whiteboard diet removes the manual takeover recipe this tool replaces; both land in the same landing on track/playbook
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1ea0b934edac17da
sage-review-completeness-reviewed: 1ea0b934edac17da
completed: 2026-11-18
---

# 화이트보드 zip 다운로드와 whiteboard.load로 보드 인계하기

## Background

Owner direction (2026-11-18, (95)): the whiteboard playbook diet removes the
"Taking over another session's board" recipe
(`agents-plugin/rsrc/whiteboard/whiteboard.md:36-41`), a multi-step manual
procedure (check `schema_version` and `generation`, copy the folder, strip
three `meta.json` keys, compare, regenerate). Handoff must not disappear with
it, so a light tool replaces it in the same landing:

- The dashboard board toolbar's search button becomes a download button that
  saves the session's board as a zip.
- The user hands the zip to another lead session in plain words ("load the
  whiteboard, the path is ..."), and that lead loads it with an MCP tool.

Current state (survey 2026-11-18, paths under `agents-plugin-tool/internal/`):

- Board folder `<root>/ai-docs/whiteboards/<reply-id>/`
  (`mcp/whiteboard_tools.go:39`, `mcp/whiteboard_store.go:18`); root files
  `meta.json`, `committed.json`, transient `pending.json`, `.lock`; per-topic
  `tN/` folders. Writes stage `pending.json`, write leaves, replace `meta.json`
  then `committed.json`, then remove `pending.json`; `Open` replays a pending
  intent (`wswhiteboard/folders.go:84-116, 411-419, 945-1032`).
- Readers use `SharedFolderCache` (`wswhiteboard/folders_cache.go:30-105`),
  which re-validates on the root files' stamps; `Forget(dir)` drops an entry.
  No export, import, or open-by-path API exists (a search for
  export/import/zip/backup functions in `wswhiteboard` and
  `mcp/whiteboard_*.go` returned nothing, and `archive/zip` is imported
  nowhere under `internal/`; but `FolderStore.Dir` is a plain exported field,
  so an extracted directory can be opened directly with
  `FolderStore{Dir: <path>}.Open`, `mcp/whiteboard_store.go:18-20`).
- Toolbar search: `wsdashboard/assets/cards.js:121` builds the `board-search`
  action; its handler (`dashboard-board.js:402`) only clicks the header
  search `#tkqs`, which Ctrl+K, Ctrl+Space and `/` also open
  (`dashboard-lifecycle.js:650-661`).
- Session routes: `wsdashboard/pages.go:1330` dispatches `/s/<reply-id>/<sub>`,
  GET subs at `:1426-1466`. Known there: repository root
  (`pageview_rail.go:754`), branch (`pageview.go:123`, may be nil), session
  display label (`pageview.go:47-49`, empty when unset).
- Render bookkeeping in `meta.json` `state`: `embed_dir`, `embeds`,
  `last_embed_seq` (`wswhiteboard/store.go:290-294`). Notification receipts
  are board entries scoped to the source reply-id (`board_notice.go:33-297`).
- Whiteboard MCP tools: schemas `mcp/server_schemas.go:649-670`, dispatch
  `server_dispatch.go:1073-1076`, lead gating `isWhiteboardTool`
  (`whiteboard_tools.go:67`, used at `server_dispatch.go:94-105`).

## Decisions

- **Download.** The board toolbar's search action (`cards.js:121`, handler
  `dashboard-board.js:402`) becomes the download; the header search `#tkqs`
  and its shortcuts stay. The toolbar button only duplicated the header
  search. (97)
- **File name.** `whiteboard_<repository dir>_<branch>_<session label>_<YYMMDD_hhmm>.zip`
  (98). Parts (107): the main repository's directory name (for example
  `devenv`), since the branch part already identifies a linked worktree; the
  branch with filename-unsafe characters (`/` and others) replaced by `-`,
  the part and its `_` dropped when there is no branch; the display label
  sanitized the same way, the reply-id when no label is set; the daemon's
  local time, since no request time zone exists server-side. Rejected: the
  linked worktree folder name (for example `261116-1227`).
- **Feature over terse documentation.** This ticket supersedes owner
  decision (637)=ok (commit 85d2008e, 2026-11-14: document the takeover
  tersely instead of building a feature): the owner reversed it in (95)
  because the playbook diet removes that documentation.
- **Load tool.** `whiteboard.load(session_key, path)` loads the zip at
  `path` into the caller's board; the whiteboard playbook's takeover section
  becomes two lines naming the download and this tool (written by the
  whiteboard diet). (99)
- **Replace, with backup.** Load replaces the caller's whole board and keeps
  the previous board as a backup folder beside it. Rejected: merging the
  loaded topics into the existing board, which would need to remap item IDs,
  panel IDs, and `{{n}}` references. (102)
- **Replace primitive (review, auto-confirmed).** The store's ordinary
  update path cannot replace a board with history: `plan`
  (`wswhiteboard/folders.go:744-871`) refuses decreasing counters, edits to
  existing entries, reused sequence numbers, and dropped topics. Load uses a
  new `wswhiteboard` entry point that, under one destination lock, copies
  the current folder to the backup, commits a fresh-board intent (topic dirs
  numbered after the old `NextTopic`, installed by the `meta.json` and
  `committed.json` switch that pending-intent replay covers), then removes
  the old `tN` dirs no longer referenced. It deliberately bypasses the
  counter and history monotonicity checks, so the loaded counters may be
  lower than the destination's. Rejected: taking the maximum of both
  counters, which breaks counter continuity; renaming the folder, which
  Windows refuses while the `.lock` handle inside it is open
  (`folders.go:251`).
- **Through the store.** Load validates the extracted zip with the store's
  `Open` (schema guard, pending-intent replay) before touching the
  destination, writes the destination under its `.lock` with the store's
  commit protocol, and calls `SharedFolderCache.Forget` plus the post-write
  refresh (`refreshWhiteboardAfter`). Rejected: copying files into the folder,
  which races daemon writers and bypasses the intent protocol and cache
  invalidation. (103)
- **Scrub.** Load clears `state.embed_dir`, `state.embeds`, and
  `state.last_embed_seq` and keeps every other value (counters, topic
  deadlines, logs, panels) as loaded; copied notification receipts are
  already ignored because they are scoped to the source reply-id. Rejected: a
  wider scrub. (104)
- **Consistent export.** Export reads the board folder under its `.lock`
  after any pending intent is replayed. Rejected: zipping without the lock,
  which can produce a torn zip under a concurrent update. (105)
- **Lead-only.** `whiteboard.load` is top-level-lead-only with no sub-lead
  pass-through; `isWhiteboardTool` rewrites a sub-lead key onto its lead's
  board, so the new tool needs its own gate. Rejected: letting a sub-lead
  load into its lead's board. (106)
- **Compatibility tier T0.** A new MCP tool and a new GET route; the zip is
  the board folder as stored (`schema_version` 2), with no new store or rsrc
  fields; the only manifest change is an additive `whiteboard.load` entry in
  the `tools` map of each package's `runtime.json` (ws, wsflow, pi), which
  `cmd/ws-mcp/main_test.go#L159-L175` pins to the tool set; an older daemon
  refuses the unknown tool name. (100)
- **Tests.** Behavior only: export then load reproduces topics, items,
  panels, logs, and the ID counter; scrubbed keys; refusal cases. The
  tool-list fixtures that pin the whiteboard tools are updated. No
  playbook-sentence assertions. (101)
- **Placement.** Standalone ticket related to
  `261117-refactor-playbook-compression`. Rejected: a child of
  `261116-epic-lead-orchestration-profile`, which covers lead orchestration
  profiles. (108)
- **Backup folders.** Each load copies the current board to a new folder
  `ai-docs/whiteboards/<reply-id>.bak-<YYMMDD_hhmm>/` (a numeric suffix on
  collision). Backups are never overwritten or pruned automatically; the
  user deletes them. Board discovery, the dashboard, and the orphan GC
  ignore `.bak-` folders with no new code: every reader resolves
  `<reply-id>/` directly, and the orphan GC already skips directories
  (`mcp/whiteboard_tools.go:897-899`). The backup copies the same file set
  as the zip (no `.lock`, no `pending.json`): the copy runs while this
  process holds the `.lock` file lock, and on Windows reading that file
  through a second handle can fail. The timestamp is daemon local time, as
  in (107). Rejected: a single overwritten
  backup, since a second load would destroy the only copy of the user's
  original board. (109)=ok

## Implementation notes (review, auto-confirmed)

- Zip layout: entries at the zip root, exactly the board folder's files;
  `.lock` and `pending.json` excluded. Load refuses entries with absolute
  paths or `..` (zip-slip), an archive without `meta.json` and
  `committed.json`, and an archive over a fixed cap on entry count and
  total uncompressed size (zip-bomb guard; the implementer picks caps well
  above the largest real board and names them in a code comment).
- Topic dirs installed by the replace are numbered after the destination's
  old `NextTopic`, while the loaded counters stay as loaded. Confirm in
  code that a topic minted after load cannot collide with an installed
  `tN` dir, and handle it inside the replace entry point if it can.
- The new GET sub-route name, the toolbar button's label and tooltip (all 11
  languages), and refusal texts follow the existing patterns.
- A session with no board yet: the download route answers 404; the toolbar
  hides the button on pages without a session board (the worktree page,
  `dashboard-board.js:397-403`; update `worktree-page.browser.cjs:80`).
  Load into a caller with no board writes the board with no backup.
- `path` is resolved on the daemon host; a missing file is refused with a
  message naming that host's path.
- The success result names the backup path and the loaded topic count.
- Load behaves like a successful `whiteboard.update` afterwards: it enables
  the session's board knob (`enableWhiteboardKnobForSession`,
  `mcp/whiteboard_tools.go:937-947`) and sends the dashboard session nudge
  (`mcp/dashboard_sessions.go:588`).

## Constraints

- The zip carries only the board folder: inbox attachments, mailbox history,
  agenda, and image files that panels reference by path do not come along
  (as in the recipe).
- Loading never modifies the source board; the source session keeps its own
  board.
- Go tests run on remote w5090 only, never locally on the owner's Windows PC.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 85d2008e (2026-11-14, commit): "Owner decision (637)=ok selected terse documentation instead of a takeover feature; no runtime or store schema changes." — bearing: contradiction-candidate
- 85d2008e (2026-11-14, commit): "Code inspection found no stem/session/root binding in folder metadata. The authoritative folder is fixed at ai-docs/whiteboards; full: uses a dashboard URL or an index.html file-path fallback" — bearing: supports
- b6575205 (2026-11-15, commit): "Preserve automatically merged TASK5 work-state guidance and D-C1 copied-board takeover safeguards together rather than choosing either parent wholesale." — bearing: supports
- 261114-feat-ws-dashboard-board-toolbar-overview (2026-11-14, d7288976): "Keep the toolbar as fixed board infrastructure, not a Card or persisted record... mount opt.toolbar owns overview/search/files callbacks." — bearing: constrains
- 261114-feat-ws-dashboard-board-toolbar-overview (2026-11-14, d7288976): "Add all Phase 1 strings to all 11 languages; English and Korean use the agreed labels." — bearing: constrains
- 291a403f (2026-11-18, commit): "Follow-up commit replaced a dynamic printf translation key with literal keys so TestEveryEnglishKeyIsUsed sees agent.sleep/agent.deleted." — bearing: constrains
- 261116-feat-orchestration-profile-foundation (2026-11-17, 05bfbe32): "Phase 1: profile-variant lookup, workflow.profile knob ..., role: sublead with sublead.json marker and status-only board path, auto-ack re-anchor" — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Store lives in its own pure package internal/wswhiteboard ... so batch semantics are unit-tested without the MCP server; the MCP layer only does persistence, file writes, and gating." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | cards.js, dashboard-board.js, wsdashboard/pages.go, mcp/whiteboard_tools.go, mcp/server_schemas.go, mcp/server_dispatch.go, wswhiteboard/folders.go, wsi18n/tables, agents-plugin/runtime.json and wsflow mirror |
| scope.surface | cross-module | new MCP tool whiteboard.load plus a new session GET route, spanning mcp, wsdashboard, wswhiteboard and the plugin runtime manifests |
| scope.new_public_symbol | yes | MCP tool whiteboard.load and a session GET subroute for the zip; export and load entry points in wswhiteboard are likely |
| scope.new_type_contract | yes | whiteboard.load(session_key, path) argument schema and the zip layout, the board folder as stored (schema_version 2) |
| scope.test_surface | existing | mcp/whiteboard_tools_test.go and sibling whiteboard_*_test.go, wsdashboard/board_toolbar_test.go; the ticket names no new test file |
| complexity.reuse_points | confirmed | FolderStore.Open and its commit protocol (wswhiteboard/folders.go:146, 411-419), SharedFolderCache.Forget (folders_cache.go:237), refreshWhiteboardAfter (mcp/whiteboard_tools.go:510), all read |
| complexity.side_effect_risk | high | replaces a whole live board folder under .lock with a backup beside it, racing daemon readers and writers and the shared cache |
| risk.correctness | high | torn export under concurrent update, pending-intent replay, scrub of embed keys, ID counter continuity, and backup rename must all hold |
| risk.fit | moderate | repurposes the toolbar search action that the toolbar-overview thread fixed as board infrastructure; the header search stays |
| risk.test | moderate | browser fixtures click .board-search (testdata/file-card-state.browser.cjs:140, worktree-page.browser.cjs:80) and must change; Go tests run on w5090 only |
| risk.security_or_contract | high | loads a caller-supplied zip path (zip-slip, size and entry validation) and adds a tool and runtime.json entries pinned by cmd/ws-mcp/main_test.go:159-175 |

## Phases

### Phase 1: Board zip download and whiteboard.load

Scope: the dashboard toolbar action and its i18n keys, the session GET route
that streams the zip, the `whiteboard.load` MCP tool (schema, dispatch, lead
gate, store-level replace), the `runtime.json` tool entries, the tool-list
fixtures and mirrors, the browser fixtures and toolbar tests that click or
pin `.board-search`, the i18n keys (an unused English key fails
`TestEveryEnglishKeyIsUsed`), and tests.
The playbook text is changed by the whiteboard diet of
`261117-refactor-playbook-compression`, not here.

Acceptance:

- Clicking the toolbar button downloads a zip named per (98)/(107).
- `whiteboard.load` with that zip's path makes the caller's board show the
  source's topics, items, panels, and logs; the next minted ID continues the
  source's counter; the previous board survives as a backup folder.
- A zip that fails the store's validation, or exceeds the caps, is refused,
  creates no backup, and leaves the destination unchanged.
- The backup folder is `<reply-id>.bak-<YYMMDD_hhmm>`; a second load in the
  same minute gets a numeric suffix; a load into a caller with no board
  creates none; a `.bak-` sibling is neither collected by the orphan GC nor
  shown.
- After load, a newly created topic gets a fresh dir that collides with no
  installed one.
- The three embed keys are scrubbed; a sub-lead key is refused; the
  success result names the backup path and the topic count.
- With no session board, the download route answers 404 and the button is
  hidden.

Verification: Go round-trip and refusal tests on w5090; the dashboard
frontend test for the toolbar action.

### Result (a75e50eeb) - 2026-11-18

Landed in 75e35cf60 (Go spine), 5fa4a80f2 (dashboard toolbar), 094127aff
(review fix), a75e50eeb (toolbar hook pin).

- `wswhiteboard.FolderStore.ExportArchive` zips the board under its `.lock`
  after pending replay: root files, the topic folders `meta.json`
  references, and the revision receipts; no `.lock`, `pending.json`,
  dot-named temp files, or unreferenced topic folders. No board is
  `os.ErrNotExist` and creates nothing.
- `FolderStore.LoadArchive(path, now)` extracts into a temp folder (refusing
  absolute, `..`, backslash, colon, non-clean, non-regular, and duplicate
  entries; caps 50,000 entries and 128 MiB counted as inflated bytes),
  validates through `Open`, scrubs the three embed keys, then `replace`s
  under one destination lock: plan a fresh-board intent against a synthetic
  prior board carrying only the old generation and `NextTopic`, copy the
  current board to `<reply-id>.bak-<YYMMDD_hhmm>` (`-2`, `-3`, ... on
  collision), commit through the pending-intent protocol, then remove the
  old topic folders best effort. Installed topic folders are numbered after
  the old `NextTopic`, so a later topic never collides with an installed or
  leftover folder (confirmed in code and by test). A reviewer's
  crash-injection probe at every commit stage replayed to the loaded board.
- Dashboard: `GET /s/<reply-id>/board.zip` through `BoardArchiveSource`
  (agent conversations have none), 404 without a board, file name
  `whiteboard_<repo>_<branch>_<label|reply-id>_<YYMMDD_hhmm>.zip` with
  filename-unsafe characters as `-`. The toolbar search button became a
  download button (Lucide `download` icon, 11 languages), absent on the
  worktree page and hidden by CSS while the board region shows `.empty`.
- MCP `whiteboard.load(session_key, path)`: top-level lead only (sub-leads
  and children refused before the sub-lead board rewrite); relative paths
  resolve against the session root; a missing file names the resolved
  host path; success names the topic count and the backup folder (or
  none); afterwards cache `Forget`, knob enable, page refresh, and the
  dashboard nudge. `runtime.json` tool entries added for ws, wsflow, pi
  and the Pi bridge tool-list fixture. Compatibility T0.

Verification (w5090): `go test -count=1 ./internal/wswhiteboard
./internal/mcp ./internal/wsdashboard ./internal/wsi18n ./cmd/ws-mcp` -
mcp, wsi18n, cmd/ws-mcp ok; wsdashboard's one failure
(TestToolbarPageActionHooks, a pin of the retired search hook) fixed in
a75e50eeb and rerun ok; wswhiteboard fails only
TestTopicStateBadgeResearchingMetadata and ...ClockAndReconciliation with
"node required" (w5090 has no Node; unchanged files, pre-existing).
Locally under node: the cards.js toolbar placement script from
TestBoardToolbarPlacementAndCounts passes; all 11 i18n tables parse.
Not run: the Playwright browser fixtures edited for `.board-download`
(they need the Go daemon plus Node), and the Pi TypeScript bridge test
(its list and the manifest were updated together).

Decisions: the zip carries derived root files (index page, stamp, mermaid
script) as part of "the board folder's files"; the destination's revision
receipts floor the loaded ones; the topic count counts every topic key.
