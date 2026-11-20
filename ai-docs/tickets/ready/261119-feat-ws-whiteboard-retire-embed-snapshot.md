---
title: "화이트보드 임베드 HTML 스냅샷 렌더링 폐지"
parent: 261119-epic-ws-daemon-sqlite-storage
related:
  261119-refactor-ws-whiteboard-sqlite-board-store: "sibling child; takes blocked-by on this ticket so the store switch never carries embed state (see (1815))"
  261116-feat-ws-whiteboard-retire-disk-pages: "its open questions on `embed:` and `wb:embed-dir` are this ticket's scope (see (1818)); full-page retirement stays there"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6f1ef5c65ba130a5
sage-review-completeness-reviewed: 6f1ef5c65ba130a5
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
were read on `develop` at `9f43a8f00` and `7c26b7b0a`. They drift: re-check
each one against the implementation base before editing.

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
    (`wsi18n/tables/en.json:90` and the other 10 locales, 11 in all).
  - Template and CSS pieces that exist only for the embed:
    - the `.Embed` branches of `wswhiteboard/assets/page.tmpl` (lines 8, 37,
      and 54, plus the other `.Page.Embed` branches that a search for
      `.Embed` finds);
    - the `newdot` template (`page.tmpl:100`), whose only condition is
      `IsNew` (`wswhiteboard/render.go:957`, `d.Page.Embed && d.New`);
    - the `.chip.fresh` rules (`wswhiteboard/assets/page.css:138-139`).
  - Fields with embed-only uses: `Fresh`, `DocNew`, `IsNew`, and
    `LastEmbedSeq` (`wswhiteboard/view.go:662`, `:699`;
    `wswhiteboard/render.go:592`, `CountFresh`).
- Orphan GC: the daily scan deletes dead sessions' `<reply-id>-<hash>.html`
  files (`mcp/whiteboard_tools.go:51-53`, `:927-967`).

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
  no reference to the embed file, `embed:`, `embed_dir`, or `wb:embed-dir`
  (the only `embed:` hit outside the playbook copies is a synthetic result
  string in the Pi test fixture
  `agents-plugin-pi/test/mcp-stdio-client.test.ts:323`, which no Pi source
  parses).
  The dashboard serves the live board and does not use the snapshot. External
  hosts that read a lead's embed line cannot be checked from this repository.

## Decisions

- **Retirement is allowed (owner, 2026-11-19 21:56, epic Cross-Child
  Decisions).**

The decisions below settled the queue on 2026-11-19 with the lead's
recommendations as assumed answers. Each line reads "owner-assumed": the owner
confirms them before this work merges to `develop`.

- **Mechanism A, tier T1, `(1817)=A`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Stop rendering the embed snapshot, and drop the `embed:` result line.
  - `wb:embed-dir` stays an accepted no-op. It stores nothing, emits no
    `setting` effect (so no metadata-revision bump), and returns one `info:`
    deprecation line, which is shipped text.
  - The stored-folder check that refuses a whole update (`store.go:862-866`)
    goes with it, since the field is ignored (`(1819)`).
  - Remove the playbook's embed bullets, the `wb:embed-dir` command-table row,
    and the inline-embed watching clause.
  - Tier per the table below: rows 1 and 3 are T1, rows 2, 4, and 5 are T0.
    The highest is T1, so a patch is allowed (Release Rule 1).
  - Rejected: B, keeping `embed:` as the live URL (T0). A host that embeds
    files gets a URL, and the line would mislead.
  - Rejected: C, an opt-in render (T0). It keeps a renderer no in-repo host
    uses.
  - Rejected: D, removing `wb:embed-dir` (T3). An unknown-command refusal hits
    old text (`wsxmlcmd/wsxmlcmd.go:410`).
- **The store child is blocked by this ticket, `(1815)`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - `261119-refactor-ws-whiteboard-sqlite-board-store` carries
    `blocked-by:` this ticket.
  - Rejected: carrying embed state and the synchronous embed render through the
    new store.
- **This ticket owns the embed slice of
  `261116-feat-ws-whiteboard-retire-disk-pages`, `(1818)`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - The slice covers `embed:`, `wb:embed-dir`, old text that emits the embed
    line, host embed users, and embed eviction.
  - The lead narrows that ticket's text to the full page and file-only mode.
  - Rejected: leaving the slice there; merging the two tickets.
- **Embed fields are ignored, never refused, `(1819)`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - The new binary stops writing `last_embed_seq`, `embeds`, and `embed_dir`,
    ignores them on read, and never refuses a board that carries them.
    Leftover values also arrive in the store child's open-time merge.
  - Rejected: a strip-on-load migration write.
- **Leftover embed files go to the existing daily cleanup, `(1820)=a`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - The daily orphan cleanup (`mcp/whiteboard_tools.go:927-967`) removes a
    session's files once its key record is gone.
  - Host embed folders are never touched.
  - Rejected: b, a one-time sweep at first start.
- **Embed-only code goes with the producer, `(1821)`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Remove the `LayoutEmbed` view and render branches, the embed panel
    variants, the embed body-class script checks, `chip.fresh` in all 11
    locales, the embed hash and eviction, and `EmbedRetention`.
  - Also remove the embed-only template and CSS pieces named in Background:
    the `page.tmpl` embed branches (lines 8, 37, 54), the `newdot` template
    (line 100) with `IsNew` (`render.go:957`), and `.chip.fresh`
    (`page.css:138-139`). Remove the fields that only the embed uses:
    `Fresh`, `DocNew`, `IsNew`, `LastEmbedSeq` (`view.go:662`, `:699`;
    `render.go:592`).
  - Keep `"embed-dir"` in `EffectFields("setting")`
    (`wswhiteboard/effects.go:52`). `wslive` validates stored effects against
    that list (`wslive/whiteboard_effects.go:161-166`), so removing the field
    would drop older stored `setting` effects. Stop only the emission. The pin
    at `wslive/whiteboard_effects_test.go:79` stays and joins the test surface.
  - Keep the GC filename pattern, which `(1820)=a` relies on.
  - Tests that pinned embed output are deleted or rewritten to assert behavior.
    A test loop over `{LayoutFull, LayoutEmbed}` is rewritten to drop the
    embed case and keep its full-layout assertions; it is not deleted.
  - Rejected: keeping dead layout code.
  - Rejected: removing `"embed-dir"` from `EffectFields`, because stored older
    effects would stop decoding in `wslive`.
- **Playbook edit procedure, `(1822)`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Edit `agents-plugin/rsrc/whiteboard/whiteboard.md`.
  - Regenerate the wsflow copy byte-identical and the Pi archives in the same
    push (`ai-docs/manuals/wsflow-mirroring.md` rule 6).
  - The edited text names no repository ticket, commit, or layout
    (`ai-docs/manuals/shipped-surface-boundary.md` `## Rules`).
- **Own before/after measurement, `(1824)`** (technical, lead ruling 2026-11-19):
  - This ticket records its own before/after file-operation counts per
    `whiteboard.update` with `fs_usage` or `dtruss` plus
    `wsstate.FileIOCounters`.
  - The store child's "before" baseline is taken on the post-embed base.
  - Rejected: one shared baseline before both children, because it would
    attribute the embed savings to the store.

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
- `"embed-dir"` stays in `EffectFields("setting")`
  (`wswhiteboard/effects.go:52`) so stored older `setting` effects still decode
  in `wslive` (`wslive/whiteboard_effects.go:161-166`). Only the emission
  stops (`(1821)`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

### Compatibility paths (graded under `(1817)=A`)

| # | Direction | Behavior | Grade |
|---|---|---|---|
| 1 | A session holding the old playbook text calls a new daemon | Playbooks are daemon-owned (`release-compatibility.md:20-25`), so new sessions get new text. A running session may still follow the old rule (`whiteboard.md:164-169`). If `embed:` is absent, it has no embed line to emit. No data change. | T1 when the line is removed; T0 when the line is kept |
| 2 | Same session sends `<wb:embed-dir>` | Accepted no-op: no refusal. Removed from the command table: unknown-command problem (`wsxmlcmd/wsxmlcmd.go:410`), which is a call refusal. | T0 for the no-op; T3 for removal without alias (`release-compatibility.md:84-86`) |
| 3 | An older daemon opens a board last written by the new one | It finds `embed_dir`/`embeds`/`last_embed_seq` absent. It renders its embed into `ai-docs/whiteboards/` and marks all content "new since the previous embed" (`view.go:662`, `:699`). Display only. | T1 |
| 4 | The new daemon opens a board written by an older one | Leftover embed fields are ignored. | T0 |
| 5 | A zip crosses versions | Load already clears embed fields (`folders_archive.go:160`). | T0 |

## Prior Decisions

- 261116-feat-ws-whiteboard-retire-disk-pages (2026-11-16, Background): "embed도 사실 버려도 되긴 합니다. 이제 안 쓰는 경로거든요." (23:02, about the `embed:` disk file) — bearing: supports
- 261119-epic-ws-daemon-sqlite-storage (2026-11-19, Cross-Child Decisions): "Retiring the per-update embed HTML snapshot rendering ... is in scope as its own child of this epic ... must state its compatibility tier" — bearing: constrains
- 29a762990 (2026-11-19, commit): "Knock-on in embed child: under (1819) the stored-embed-dir refusal (store.go:862-866) is removed with the no-op" — bearing: supports
- 75e35cf60 (2026-11-18, commit): "feat(whiteboard): board zip export, board.zip route, and whiteboard.load" (load clears embed fields, `folders_archive.go:160`) — bearing: constrains
- f98d8e832 (2026-11-04, commit): "feat(whiteboard): add the session whiteboard store, tools, pages, and knob" (introduced `EmbedRetention`, the stored embed-dir refusal) — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | mcp/whiteboard_tools.go, mcp/whiteboard_panels.go, mcp/session_state.go, wswhiteboard/store.go, commands.go, effects.go, view.go, render.go, assets/*.js, wsi18n/tables/*.json, agents-plugin/rsrc/whiteboard/whiteboard.md and its wsflow and Pi copies |
| scope.surface | cross-module | `embed:` result line and `wb:embed-dir` command are shipped tool surface; playbook text ships in three packages |
| scope.new_public_symbol | no | none; exported `LayoutEmbed`, `EmbedRetention`, `Embed`, `State.EmbedDir` are removed or ignored |
| scope.new_type_contract | no | none; `wb:embed-dir` becomes an accepted no-op with an info line |
| scope.test_surface | existing | tests pinning embed output in mcp/whiteboard_tools_test.go, whiteboard_panels_test.go, wswhiteboard/render_test.go, store_test.go, commands_test.go, effects_test.go, folders_test.go; wslive/whiteboard_effects_test.go:79 (the `embed-dir` effect pin stays); compat harness wswhiteboard/testdata/compat-old |
| complexity.reuse_points | confirmed | existing GC pattern `whiteboardFilePattern` (whiteboard_tools.go:53) and compat-old harness are reused |
| complexity.side_effect_risk | moderate | removes persisted State fields read by folders meta.json, the effect list, the metadata revision and a refusal path; stale `embed_dir` boards must keep loading |
| risk.correctness | moderate | embed-only branches span view, render, panels and three JS assets; leftover fields must be ignored without a schema refusal (no DisallowUnknownFields found) |
| risk.fit | low | owner allowed retirement and the mechanism is settled by (1817)=A |
| risk.test | moderate | many tests pin embed output; old-daemon paths need the compat-old harness |
| risk.security_or_contract | moderate | `embed:` line removal and playbook text are shipped surface at T1; old sessions may still follow old playbook text |

## Phases

### Phase 1: Stop rendering and publishing the embed snapshot

Apply `(1817)=A` to:

- the renderer and publish path;
- the `embed:` result line;
- `wb:embed-dir`;
- the shipped playbook text and its copies (`(1822)`).

Also apply `(1819)` to persisted state and `(1821)` to the code. Leftover
files follow `(1820)=a`: there is no new deletion path.

Test loops over `{LayoutFull, LayoutEmbed}` are rewritten to drop the embed
case and keep their full-layout assertions; they are not deleted. Examples:
`recap_test.go:92`, `panel_identity_test.go:91`, `acceptance_test.go:480`,
`dashboard_answer_revision_test.go:31`, `stream_script_test.go:160` (all under
`wswhiteboard/`). Find the rest by searching for `LayoutEmbed` in tests.

Verification:

- A top-level lead `whiteboard.update` writes no `<reply-id>-<hash>.html` and
  no host-folder file.
- A successful top-level update's result no longer carries an `embed:` line.
  Its `full:` line is unchanged.
- `<wb:embed-dir path="...">` is accepted and stores nothing. A board that
  carries a stale `embed_dir` from an older version no longer refuses updates.
- The `wb:embed-dir` deprecation `info:` sentence joins the result through
  `joinInfo` (`wswhiteboard/store.go:882`, `:945`) and follows
  `ai-docs/manuals/shipped-surface-boundary.md` (no repository ticket, commit,
  or layout named).
- The wsflow playbook copy is byte-identical and the Pi archives are
  regenerated (`(1822)`).
- The daily GC still removes leftover `<reply-id>-<hash>.html` files of dead
  sessions (`(1820)`).
- A stored older `setting` effect with field `embed-dir` still decodes in
  `wslive` (`wslive/whiteboard_effects_test.go:79` still passes).
- Each graded path in the table behaves as recorded. Where the old binary is
  involved, use the `wswhiteboard/testdata/compat-old` harness.
- Before/after file operations and wall time per `whiteboard.update` on macOS,
  by the method in `(1824)`, which follows
  `261119-refactor-ws-whiteboard-sqlite-board-store` `## Decisions`
  (Measurement). Windows V3 measurement is recorded as deferred.
- Tests assert behavior, not playbook sentences.

### Result (b0ffcb34e) - 2026-11-20

Commit `cfbfc38c9` removed the per-update embed renderer, publication, result
line, persisted fields, layout branches, snapshot-only UI and playbook text.
`wb:embed-dir` remains an accepted no-op with one deprecation sentence;
`embed-dir` remains a decodable stored effect field; the legacy orphan-file
matcher remains. The canonical playbook, wsflow mirror, Pi mirror, manifests
and bundled archives were regenerated. The pinned old-source harness verifies
both compatibility directions, including stale embed state and the older
display-only degradation. The highest compatibility tier is T1.

Commit `b0ffcb34e` added the opt-in
`TestWhiteboardUpdateMeasurement`. With
`WS_WHITEBOARD_MEASURE_UPDATES=20`, the same harness was run on the same macOS
machine against an archive of `fbcc01bb0` and against the retired-embed tree.
Each run used a fresh temporary project and made 20 identical top-level
`whiteboard.update` calls, each carrying one topic, one named panel and one
log. The first row is the first write; the other 19 are steady-state writes.
Wall time covers the tool call only; snapshots and reporting run outside it.

The recursive cross-check has two non-overlapping scopes: the reply folder,
where authoritative board files and live page publication share a directory,
and its parent excluding that reply folder, where legacy embed snapshots were
flat files. `created/modified/removed` and `bytes written` compare final file
content hashes and sizes. Steady-state filesystem values below are medians;
after retention filled, the baseline normally created one embed and removed
one older embed per update.

| per update | before `fbcc01bb0` | after `cfbfc38c9` | change |
|---|---:|---:|---:|
| first-write wall time | 128.115 ms | 117.434 ms | -8.3% |
| steady wall median | 86.596 ms | 85.159 ms | -1.7% |
| steady wall p95 | 119.336 ms | 110.485 ms | -7.4% |
| `FileIOCounters`, first write | 99 | 99 | 0 |
| `FileIOCounters`, steady median | 107 | 107 | 0 |
| first-write files created/modified/removed | 9 / 0 / 0 | 8 / 0 / 0 | -1 created |
| first-write final bytes written | 518,035 | 350,109 | -32.4% |
| steady files created/modified/removed | 2 / 5 / 1 | 1 / 5 / 0 | -1 created, -1 removed |
| steady final bytes written | 542,469 | 374,270 | -31.0% |
| steady legacy-embed scope | 1 created, 1 removed, 163,365 B | 0 / 0 / 0, 0 B | removed |

`FileIOCounters` is unchanged because the retired embed path used direct
`os.*` calls. The directory cross-check exposes that otherwise invisible
derived-file churn, but remains a lower bound: atomic temporary files created
and removed within one update are absent from both snapshots, and the method
does not count `flock`, rename, chmod or bytes rewritten with identical final
content as distinct operations. `fs_usage` requires root and `dtruss` requires
additional DTrace privileges on this host, so neither was used and no `sudo`
was attempted for this measurement.

Focused `wswhiteboard`, `wslive`, `wsi18n`, resource-drift and MCP whiteboard
tests passed, including the broad MCP whiteboard slice at 100.660 seconds.
Darwin, Linux and Windows vet passed for the touched packages. Independent
review's compatibility-test and dead-layout findings were fixed and its second
pass was clear.

Windows V3 measurement: deferred.
