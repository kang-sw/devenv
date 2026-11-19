---
title: "화이트보드 임베드 HTML 스냅샷 렌더링 폐지"
parent: 261119-epic-ws-daemon-sqlite-storage
related:
  261119-refactor-ws-whiteboard-sqlite-board-store: "sibling child; takes blocked-by on this ticket so the store switch never carries embed state (see (1359))"
  261116-feat-ws-whiteboard-retire-disk-pages: "its open questions on `embed:` and `wb:embed-dir` are this ticket's scope (see (1362)); full-page retirement stays there"
---

# Retire the whiteboard's per-update embed HTML snapshot

## Background

Every top-level lead `whiteboard.update` renders a second, point-in-time board
page (`LayoutEmbed`), writes it as `ai-docs/whiteboards/<reply-id>-<hash>.html`
or into a host-named embed folder, and reports it on the result's `embed:`
line. The shipped whiteboard playbook tells a lead whose host has an embed
syntax to end its turn with that file. The owner allowed retiring it on
2026-11-19 (21:56, epic Cross-Child Decisions): "참고로 임베드 html 렌더링에도
tact time이 꽤 소요되는 거 같은데 이 경로 이제 버려도 되긴 합니다." In
English: embed rendering seems to cost noticeable tact time, and the path can
now be dropped. On 2026-11-16 (23:02, quoted in
`261116-feat-ws-whiteboard-retire-disk-pages`) the owner also said: "embed도
사실 버려도 되긴 합니다. 이제 안 쓰는 경로거든요." In English: embed can be
dropped too, it is a path nobody uses anymore.

Go paths below are relative to `agents-plugin-tool/internal/`. Line numbers
were read on `develop` at `9f43a8f00`.

### Producers and state

- Render: `renderWhiteboardPages` renders `LayoutEmbed` when `withEmbed`
  (`mcp/whiteboard_tools.go:647-669`). A top-level lead's update passes
  `sublead == nil` (`:304`). Sub-lead updates and the render-only refresh
  render no embed (`:237-238`, `:563-589`).
- Name and place: `<reply-id>-<hash10>.html`, where the hash covers the
  sequence and content (`:652`, `:814-821`). The file goes in `EmbedDir`
  when set (host-owned, never evicted, `:653-662`), else in
  `ai-docs/whiteboards/`, with the newest `EmbedRetention = 5` kept by
  sequence (`:663-666`, `:823-838`; `wswhiteboard/store.go:59-62`).
- Publish: written under the board lock after commit, before the result returns
  (`whiteboard_tools.go:785-812`, called from `mcp/session_state.go:665-675`).
  Per update that is `MkdirAll` + create temp + chmod + write + rename, plus
  one eviction remove once retention is full. That is 6 of the 85 + 5T ops
  counted in `261119-refactor-ws-whiteboard-sqlite-board-store`, plus a full
  second `BuildView` + `RenderPage` of the board.
- Persisted state in the board's session metadata: `last_embed_seq`, `embeds`,
  `embed_dir` (`wswhiteboard/store.go:319-324`). Load clears them
  (`wswhiteboard/folders_archive.go:160`).
- Command: `wb:embed-dir path=...` (`wswhiteboard/commands.go:41`; apply at
  `wswhiteboard/store.go:791-804`). A stored folder that has gone missing
  refuses the whole update (`store.go:862-866`). It is also a `setting` effect
  that bumps the metadata revision (`wswhiteboard/effects.go:52`, `:200`;
  `wswhiteboard/board_revision.go:72-80`).
- Embed-only rendering:
  - `LayoutEmbed` in the view (`wswhiteboard/view.go:22-23`, `:662`,
    `:675`, `:699`, `:706`, `:741`) and in the renderer
    (`wswhiteboard/render.go:310`, `:558`).
  - Embed panel variants (`mcp/whiteboard_tools.go:174`, `:200`;
    `mcp/whiteboard_panels.go:150-158`, `:244`).
  - The embed body class checks in `assets/form.js`, `pill.js`, `rail.js`.
  - The `chip.fresh` string "new since the previous embed"
    (`wsi18n/tables/en.json:90` and the other locales).
- Orphan GC: the daily scan deletes dead sessions' `<reply-id>-<hash>.html`
  files (`mcp/whiteboard_tools.go:50-52`, `:927-967`).

### Consumers

- Result line: `embed: <path>` on every successful top-level update
  (`mcp/whiteboard_tools.go:362`). Nothing parses it in code. Its reader is the
  lead model following the playbook.
- Shipped playbook `whiteboard` (`agents-plugin/rsrc/whiteboard/whiteboard.md`):
  - the `wb:embed-dir` row of the command table (`:63`);
  - the "when the host embeds the board inline in the conversation" clause of
    the watching rule (`:162`);
  - the two embed bullets of `## Each turn` (`:164-169`).

  The generated wsflow copy
  (`agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`) and the Pi copy
  (`agents-plugin-pi/rsrc/whiteboard/whiteboard.md`) are byte-identical at the
  same lines.
- Hosts: no consumer found. `agents-plugin/hooks/claude-mod/`,
  `agents-plugin-tool/desktop-electron/`, and the Pi extension sources contain
  no reference to the embed file, `embed:`, `embed_dir`, or `wb:embed-dir`.
  The dashboard serves the live board and does not use the snapshot. External
  hosts that read a lead's embed line cannot be checked from this repository.

## Decisions

- **Retirement is allowed (owner, 2026-11-19 21:56, epic Cross-Child
  Decisions).** The mechanism and compatibility tier are not decided; see
  (1361).

## Constraints

- Manuals: `ai-docs/manuals/release-compatibility.md` (tier with source lines in
  `## Decisions` before `ready/`), `ai-docs/manuals/shipped-surface-boundary.md`
  (playbook text and every result/refusal string),
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (playbook edit, wsflow copy, Pi archive
  regeneration, rule 6), `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/performance.md` (before/after counts).
- Out of scope: the full page `index.html` / `index.stamp.js` and file-only
  mode (`261116-feat-ws-whiteboard-retire-disk-pages`), and the board store
  itself (`261119-refactor-ws-whiteboard-sqlite-board-store`).
- `whiteboard.update` keeps its arguments and its `full:` line.

### Compatibility paths to grade (tier follows the (1361) choice)

| # | Direction | Behavior | Grade |
|---|---|---|---|
| 1 | A session holding the old playbook text calls a new daemon | Playbooks are daemon-owned (`release-compatibility.md:20-25`), so new sessions get new text. A running session may still follow the old rule (`whiteboard.md:164-169`). If `embed:` is absent, it has no embed line to emit. No data change. | T1 when the line is removed; T0 when the line is kept |
| 2 | Same session sends `<wb:embed-dir>` | Accepted no-op: no refusal. Removed from the command table: unknown-command problem (`wsxmlcmd/wsxmlcmd.go:410`), which is a call refusal. | T0 for the no-op; T3 for removal without alias (`release-compatibility.md:84-86`) |
| 3 | An older daemon opens a board last written by the new one | It finds `embed_dir`/`embeds`/`last_embed_seq` absent. It renders its embed into `ai-docs/whiteboards/` and marks all content "new since the previous embed" (`view.go:662`, `:699`). Display only. | T1 |
| 4 | The new daemon opens a board written by an older one | Leftover embed fields are ignored. | T0 |
| 5 | A zip crosses versions | Load already clears embed fields (`folders_archive.go:160`). | T0 |

## Open Decision Queue

Last ID used: (1366)

(1359) [open] [technical] `261119-refactor-ws-whiteboard-sqlite-board-store` takes `blocked-by:` this ticket, so the embed render is gone before the store switch.
- Sequencing over carrying embed state (`Embeds`, `LastEmbedSeq`, `EmbedDir`)
  and the synchronous embed render through the new store, only to delete them.
  The per-update embed render is synchronous derived output that
  performance.md rule 5 forbids on the update path.

(1361) [open] [policy] Retirement mechanism for the embed snapshot.
- Context: no in-repo host consumes the file. The owner described the path as
  unused (2026-11-16) and costly (2026-11-19). The paths are graded in the
  table above.
- Alternative A: stop rendering. Remove the `embed:` result line. Keep
  `wb:embed-dir` as an accepted no-op that stores nothing and returns one
  `info:` deprecation line. Remove the playbook's embed bullets, command-table
  row, and inline-embed watching clause. Tier T1, patch-safe.
- Alternative B: as A, but keep the `embed:` line carrying the live board URL
  (the `full:` value). Tier T0. A host that embeds a file path then gets a URL.
- Alternative C: render only when a host set `wb:embed-dir`, and render nothing
  by default. Tier T0. Keeps the embed renderer and its code for an opt-in no
  in-repo host uses.
- Alternative D: as A, and also remove `wb:embed-dir`. Tier T3 (unknown-command
  refusal for old text). Needs a minor release and owner approval.
> Recommend A. It removes all render cost and code, stays patch-safe, and leaves
> no misleading line. B's URL does not fit hosts that embed files, and C keeps
> a renderer nobody in the repo uses.

(1362) [open] [technical] This ticket owns the embed slice that `261116-feat-ws-whiteboard-retire-disk-pages` lists as open (`embed:`, `wb:embed-dir`, old text that emits the embed line, host embed users, embed eviction). After settlement the lead narrows that ticket's text to the full page and file-only mode.
- Moving the slice here over leaving it in 261116 or merging the tickets. The
  owner's 2026-11-19 allowance names this child, and 261116's full-page
  retirement depends on the `file://` link decision this ticket does not need.

(1363) [open] [technical] The new binary stops writing `last_embed_seq`, `embeds`, and `embed_dir` and ignores them on read; it never refuses a board that still carries them.
- Ignore over strip-on-load migration. Go's JSON decoding already ignores
  unknown fields, so no write is needed. Paths 3-5 in the table stay T0/T1.

(1364) [open] [policy] What happens to existing embed snapshot files in `ai-docs/whiteboards/` (up to 5 per live session, plus host-folder files the server never owned).
- Alternative a: leave them to the existing daily orphan GC, which deletes a
  session's files once its key record is gone
  (`mcp/whiteboard_tools.go:927-967`). Live sessions keep their leftovers until
  then.
- Alternative b: a one-time sweep at first start deletes every
  `<reply-id>-<hash>.html` in each project's `ai-docs/whiteboards/`.
- Host embed folders are never touched in either case
  (`whiteboard_tools.go:658-662`).
> Recommend a: no new deletion path. The files are git-excluded and bounded.

(1365) [open] [technical] Remove the embed-only code with the producer: the `LayoutEmbed` view and render branches, the embed panel variants, the embed body-class script checks, `chip.fresh` in every locale, the embed hash and eviction, and `EmbedRetention`. Tests that pinned embed output are deleted or rewritten to assert behavior.
- Removal over keeping dead layout code. No other caller renders
  `LayoutEmbed` (its only producer is `whiteboard_tools.go:648`). Keep the
  GC filename pattern while (1364)a relies on it.

(1366) [open] [announced] Playbook edits go to `agents-plugin/rsrc/whiteboard/whiteboard.md`, with the wsflow copy regenerated byte-identical and the Pi archives regenerated in the same push. The edited text names no repository ticket, commit, or layout.
- Citation: `ai-docs/manuals/wsflow-mirroring.md` (generated rsrc tree; rule 6)
  and `ai-docs/manuals/shipped-surface-boundary.md` `## Rules`.

## Phases

### Phase 1: Stop rendering and publishing the embed snapshot

Apply the mechanism settled in (1361) to:

- the renderer and publish path;
- the `embed:` result line;
- `wb:embed-dir`;
- the shipped playbook text and its copies ((1366)).

Also apply (1363) to persisted state and (1365) to the code, and handle
leftover files per (1364).

Verification:

- A top-level lead `whiteboard.update` writes no `<reply-id>-<hash>.html` and
  no host-folder file.
- Each graded path in the table behaves as recorded. Where the old binary is
  involved, use the `wswhiteboard/testdata/compat-old` harness.
- Before/after file operations and wall time per `whiteboard.update` on macOS,
  with the method recorded in `261119-refactor-ws-whiteboard-sqlite-board-store`
  `## Decisions`. Windows V3 measurement is recorded as deferred.
- Tests assert behavior, not playbook sentences.
