# Plan: 260811-feat-note-visibility-mute — Phase 1: `visible` field + mute/unmute verbs + injection exclusion

## Relevant Ticket Contract

- `visible` is a boolean record field, default `true`, applied uniformly
  across all three layers (`machine`, `worktree`, `repo` — `repo` now exists
  from `260810-feat-repo-tracked-note-layer`, which just landed on this
  branch; the ticket text names only the two non-tracked layers but the
  Coordination section requires extending all three since this ticket lands
  second).
- Migration: an absent `visible` field on a previously-stored record reads as
  `true`.
- `note.write` never accepts or mutates `visible`: preserves the stored value
  on an existing key, initializes `true` on a new key. A write-over-a-muted-key
  test must assert the mute survives a content overwrite.
- `note.mute(layer, [keys])` / `note.unmute(layer, [keys])`: idempotent
  set-state on `visible`, reuse the flock+temp+atomic-rename RMW, and must
  NOT re-stamp `written_at`.
- Muted notes are excluded from the injected `# Notes` block AND from the
  `notesInjectionCap` budget (a muted note frees a slot). A distinct
  "N muted — note.search to view" line renders independently of the existing
  over-cap elision line; both render together when both apply.
- All-muted edge: the block still renders (heading + muted line) when a layer
  has muted notes even if zero visible notes remain. The empty-skip (no
  block at all) fires only when there are no notes whatsoever across every
  layer.
- `note.search` returns muted records unchanged (no filtering).
- Verb spelling is planning-settleable; `note.write(..., visible:)` is
  explicitly excluded.

## Out of Scope

- Adding a `visible`/`muted` filter argument to `note.search` — ticket marks
  this optional/not required for the contract.
- Any change to `formatNoteSearch`/`formatNoteWrite` display of the `visible`
  field — not required by the contract; searching for muted notes already
  works via the existing unfiltered `Search`.
- Spec anchors beyond `spec/mcp-tools.md` `{#260810-note-tools}` /
  `{#260810-note-injection}` — ticket's Spec Impact section names only these.
- Any change to `ai-docs/mental-model/mcp-runtime.md`'s existing
  `{#260810-note-tools}` bullet (append-not-prepend placement rule) — still
  accurate, unaffected by this phase.

## Verb Spelling Decision

`note.mute` / `note.unmute` — the ticket's own provisional pair already reads
naturally ("mute a note"), is used consistently throughout the ticket's
Decisions/Phase text, and no alternative pairing is proposed anywhere else in
the repo (`grep` for `note.mute`/`note.unmute`/`note.hide`/`note.show` found
zero prior usages to reconcile against). Settling on it now; no further
bikeshedding needed for a light plan.

## Codebase Findings

- `agents-plugin-tool/internal/wsnote/record.go#L18-L23` — `Record` struct
  has no `Visible` field yet. Plain `Visible bool `json:"visible"`` (no
  `omitempty` — a stored `false` must serialize explicitly, or it becomes
  indistinguishable from "field absent") is insufficient alone: Go's
  `encoding/json` sets an absent bool field to its zero value (`false`) on
  unmarshal, which is the **opposite** of the required "absent reads as
  `true`" migration contract. Needs a custom `Record.UnmarshalJSON` (pointer
  through a `*bool` shadow field, defaulting to `true` when nil) — risk
  signal: a naive plain-bool-field addition silently breaks migration for
  every note stored before this ships.
- `agents-plugin-tool/internal/wsnote/store.go#L61-L94` — `Load`/`decodeRecords`
  decode into `map[string]Record` via `json.Unmarshal(raw, &records)`.
  `encoding/json` invokes a per-element `UnmarshalJSON` for map values
  addressably, so the custom `Record.UnmarshalJSON` above covers this path
  without further change here.
- `agents-plugin-tool/internal/wsnote/repo_store.go#L58-L62` — `RepoLoad`
  decodes each key file directly via `json.Unmarshal(raw, &rec)` (a single
  `Record`, not a map) — same custom `UnmarshalJSON` covers this path too, so
  one migration fix in `record.go` covers all three layers' read paths.
- `agents-plugin-tool/internal/wsnote/store.go#L75-L82` — `Write`'s RMW
  transform is `for _, rec := range records { current[rec.Key] = rec }`
  (blind full overwrite, no existing-value merge). This is the natural,
  race-free place to add the preserve/default logic for `visible`: the
  transform already runs inside the flock with access to `current`, matching
  how `Erase`'s transform already embodies business logic ("delete if
  present, no-op if missing") in this same package — `record.go`'s doc
  comment carve-out ("WrittenAt is set by ... not by this package") is
  specifically about the wall-clock injection point for tests, not a ban on
  all merge logic living here.
- `agents-plugin-tool/internal/wsnote/repo_store.go#L105-L153`
  (`writeRepoRecordFile`) — writes `rec` directly with no read-before-write;
  needs the equivalent preserve logic added inside the existing per-key flock
  (read `path` if it exists, unmarshal, carry over `Visible`), otherwise the
  repo layer diverges from the non-tracked layers' preserve contract.
- **Risk signal — do NOT implement preserve/default via a `Load()`-then-`Write()`
  pair in the MCP handler** (`note_tools.go`'s `handleNoteWrite`): that
  pattern is not covered by any single flock acquisition, so a concurrent
  `note.write`/`note.mute` on the same key between the `Load` and `Write`
  calls could silently restore a stale `visible` value. The existing RMW
  transform functions in `wsnote` are the correct, already-serialized place
  for this merge — confirmed via `store.go#L96-L153` (`rmw`)'s single
  flock-guarded read→transform→write.
- `agents-plugin-tool/internal/wsnote/store.go` — no `SetVisible` helper
  exists yet; needs one alongside `Write`/`Erase`, reusing `rmw` exactly like
  `Erase` does (idempotent — no-op both when the key is missing and when
  `visible` already equals the target).
- `agents-plugin-tool/internal/wsnote/repo_store.go` — needs the repo-layer
  counterpart (`RepoSetVisible`), reusing `writeRepoRecordFile`'s per-key
  flock (read existing record, flip `Visible`, re-marshal only if changed or
  unconditionally — either is safe since it never touches `WrittenAt`).
- `agents-plugin-tool/internal/mcp/note_tools.go#L20-L44` — the `noteStore`
  interface (`Load`/`Write`/`Erase`) is implemented by both `fileNoteStore`
  (wraps `wsnote.Load/Write/Erase`) and `repoNoteStore` (wraps
  `wsnote.RepoLoad/RepoWrite/RepoErase`); add a fourth method
  `SetVisible(keys []string, visible bool) error` to the interface and both
  implementations, wired to the new `wsnote.SetVisible`/`RepoSetVisible`.
  This is the layer-agnostic dispatch point `handleNoteMute`/`handleNoteUnmute`
  will use, mirroring `handleNoteErase`'s shape exactly (`note_tools.go#L217-L237`).
- `agents-plugin-tool/internal/mcp/note_tools.go#L175-L193` (`noteKeysArg`) —
  directly reusable for `note.mute`/`note.unmute`'s `keys` argument parsing;
  no change needed.
- `agents-plugin-tool/internal/mcp/note_tools.go#L114-L152` (`noteNow` /
  `noteRecordsArg`) — this is "the injectable clock the note tests already
  use" the ticket references; `handleNoteMute`/`handleNoteUnmute` must NOT
  call `noteRecordsArg` or reference `noteNow` at all (mute/unmute never
  touch `written_at`), and the corresponding test should override `noteNow`
  the same way `note_tools_test.go#L289-L321`
  (`TestNoteWriteRestampsWrittenAtOnOverwrite`) does, to prove no restamp
  happens across a mute/unmute call even when wall-clock time visibly moves.
- `agents-plugin-tool/internal/mcp/server.go#L1253-L1258` — dispatch switch
  cases for `note.write`/`note.erase`/`note.search`; add `note.mute` →
  `s.handleNoteMute` and `note.unmute` → `s.handleNoteUnmute` alongside them.
- `agents-plugin-tool/internal/mcp/server.go#L4289-L4329` — tool schemas for
  `note.write`/`note.erase`/`note.search`; add `note.mute`/`note.unmute`
  schemas immediately after `note.erase` (L4300-4313), same shape as
  `note.erase` (`session_key`, `layer` via `enumStringProperty`, `keys` via
  `stringArrayProperty`), required `["session_key", "layer", "keys"]`.
- `agents-plugin-tool/internal/mcp/note_announcement.go#L13-L21` —
  `notesInjectionCap = 20`; `computeNotes` is a thin wrapper over
  `wsnote.Compute(root, wsconfig.Options{}, notesInjectionCap)`. No change
  needed here; all cap/exclusion logic lives in `wsnote.Compute`.
- `agents-plugin-tool/internal/wsnote/inject.go#L30-L89` (`Compute`) — core
  change site. Currently: collects all records into `all` (muted+visible
  mixed) across all three layers, returns `""` if `len(all) == 0`, sorts all
  of them, then caps by `limit`/elides the remainder. Needs restructuring so:
  1. The `len(all) == 0` empty-skip check stays keyed on the **unfiltered**
     `all` (all notes on every layer, muted or not) — this is what makes the
     all-muted edge (block renders) different from the truly-empty edge
     (block skipped).
  2. Split `all` into a `visible` subset (only `Visible == true`) and a
     `muted` count, before sorting/capping.
  3. Sort and cap only the `visible` subset (so a muted note never consumes a
     `notesInjectionCap` slot).
  4. Emit the existing `"(%d lower-priority notes elided ...)"` line from the
     over-cap remainder of `visible` (unchanged logic, just now operating on
     the filtered slice).
  5. Emit a new `"(%d muted — use note.search to view.)"` line whenever
     `muted > 0`, independent of the elided line — both can render together.
  6. The heading `"# Notes\n"` write and the loop over `shown` naturally
     already handle the all-muted edge correctly once `shown` becomes
     possibly-empty: zero bullet lines print, only the heading + muted line
     remain.
- `agents-plugin-tool/internal/wsnote/inject.go#L82-L88` — current elision
  line wording (`"(%d lower-priority notes elided — use note.search to
  retrieve.)\n"`); new muted line should match this exact house style:
  `"(%d muted — use note.search to view.)\n"`.
- `agents-plugin-tool/internal/wsnote/search.go#L29-L63` (`Search`) — already
  performs no `Visible` filtering (iterates `records` unconditionally,
  filtering only on `glob`/`from`/`then`), so `note.search` already satisfies
  "returns muted records unchanged" with **no code change** — only a
  verification test is needed here.
- `agents-plugin-tool/internal/wsnote/inject_test.go` — existing tests
  (`TestComputeReturnsEmptyWhenNoNotesExist`,
  `TestComputeSortsAndCapsAcrossAllThreeLayers`,
  `TestComputeElidesBeyondLimit`, etc.) construct records via `Write`/
  `RepoWrite` directly with an explicit `WrittenAt` string (no clock
  injection needed at this layer) — new mute-related `Compute` tests should
  follow the same direct-construction style, writing records with
  `Visible: true`/`Visible: false` set directly on the `Record` literal (no
  need to route through `SetVisible` for these tests since `Write`/`RepoWrite`
  already accept a fully-formed `Record`).
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L14-L54` — test
  fixtures (`setupNoteTestEnv`, `mintRootKey`, `twoWorktreesOfOneRepo`) and
  `callToolWithKey` helper are directly reusable for new
  `handleNoteMute`/`handleNoteUnmute` tests and the required
  write-over-muted-key regression test.
- `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go#L27-L81` —
  `TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`
  shows the pattern for asserting `# Notes` block content through a full
  `workflow_manual` call; a new test following this shape (write a note, mute
  it, call `workflow_manual`, assert the key/value are absent from the body
  but the muted-count line is present) covers the ticket's core
  write→mute→drop-from-injection verification boundary end-to-end.
- `ai-docs/spec/mcp-tools.md` `## Note Tools {#260810-note-tools}`
  (`L416-L490` region, wire-shape paragraph at `L456-L463`) and
  `### Note Injection {#260810-note-injection}` (`L691-L715`) — ticket's Spec
  Impact section requires updating both: the wire-shape record fields list
  and `note.write` contract paragraph, plus the injection section's cap/elision
  description to add the muted-line behavior.

## Implementation Plan

1. `agents-plugin-tool/internal/wsnote/record.go` — add `Visible bool
   `json:"visible"`` to `Record` (no `omitempty`), and add a custom
   `func (r *Record) UnmarshalJSON(data []byte) error` that decodes through a
   `*bool` shadow field for `visible`, defaulting to `true` when the field is
   absent/`null`, else using the decoded value. Keep `MarshalJSON` default
   (plain struct tags) so writes always emit an explicit `visible`.
2. `agents-plugin-tool/internal/wsnote/store.go` — in `Write`'s RMW
   transform, before `current[rec.Key] = rec`: if `current[rec.Key]` exists,
   set `rec.Visible = existing.Visible`; else set `rec.Visible = true`. Add
   `func SetVisible(path string, keys []string, visible bool) error` using
   `rmw`, mirroring `Erase`'s shape: for each key present in `current`, set
   `Visible` to the target value (idempotent — writing the same value is a
   harmless no-op) and leave every other field (including `WrittenAt`)
   untouched; missing keys are silently skipped.
3. `agents-plugin-tool/internal/wsnote/repo_store.go` — in
   `writeRepoRecordFile`, after acquiring the per-key flock and before
   marshaling, attempt to read+unmarshal the existing file at `path`; if it
   exists, set `rec.Visible = existing.Visible`, else set `rec.Visible =
   true`. Add `func RepoSetVisible(dir string, keys []string, visible bool)
   error` that, for each key, acquires the same per-key flock as
   `writeRepoRecordFile`, reads the existing file (skip silently if absent —
   matching `RepoErase`'s missing-key no-op precedent), flips `Visible`, and
   rewrites via the same temp-file+atomic-rename sequence, leaving
   `WrittenAt` untouched.
4. `agents-plugin-tool/internal/wsnote/inject.go` — restructure `Compute` per
   the Codebase Findings entry above: keep the `len(all) == 0` empty-skip
   keyed on the unfiltered collection, split into `visible`/`muted` before
   sort+cap, cap/elide only the visible subset, and append the new
   `"(%d muted — use note.search to view.)\n"` line whenever `muted > 0`,
   independent of and after the existing elision line.
5. `agents-plugin-tool/internal/mcp/note_tools.go` — add `SetVisible(keys
   []string, visible bool) error` to the `noteStore` interface; implement on
   `fileNoteStore` (→ `wsnote.SetVisible`) and `repoNoteStore` (→
   `wsnote.RepoSetVisible`). Add `handleNoteMute`/`handleNoteUnmute` methods
   on `*Server`, mirroring `handleNoteErase`'s shape exactly (resolve store,
   parse `keys` via `noteKeysArg`, reject empty `keys`, call
   `store.SetVisible(keys, true|false)`, JSON or text response). Add
   `formatNoteMute`/`formatNoteUnmute` (or one shared `formatNoteSetVisible`
   helper parameterized by verb text), mirroring `formatNoteErase`.
6. `agents-plugin-tool/internal/mcp/server.go` — add `note.mute`/`note.unmute`
   dispatch cases beside `note.erase` (`L1255-1256`), and add their tool
   schemas beside `note.erase`'s (`L4302-4313`): same
   `session_key`/`layer`/`keys` shape, required
   `["session_key", "layer", "keys"]`, descriptions naming the idempotent
   set-state contract and the "does not change written_at" behavior.
7. Tests:
   - `record.go`: new test(s) for `UnmarshalJSON` defaulting `Visible` to
     `true` on a payload missing the `"visible"` key, and preserving an
     explicit `false`.
   - `store_test.go`: `TestWritePreservesVisibleOnExistingMutedKey` (mute a
     key via `SetVisible`, then `Write` a content-only update to the same
     key, assert `Visible` stays `false` and `Value`/`Priority`/`WrittenAt`
     update) — this is the ticket-mandated write-over-a-muted-key test.
     `TestWriteSetsVisibleTrueOnNewKey`. `TestSetVisibleIdempotentAndLeavesWrittenAtUnchanged`
     (mute an already-muted key is a no-op; assert `WrittenAt` byte-identical
     before/after).
   - `repo_store_test.go`: repo-layer counterparts of the three tests above,
     using `RepoWrite`/`RepoSetVisible`.
   - `inject_test.go`: `TestComputeExcludesMutedNotesFromCapBudget` (a muted
     note frees a slot for a previously-elided visible note — mirrors the
     ticket's verification boundary), `TestComputeRendersMutedLineIndependentOfElisionLine`
     (both muted and over-cap notes present → both lines render),
     `TestComputeRendersHeadingOnlyWhenAllNotesMuted` (all-muted edge: block
     renders with heading + muted line, zero bullet lines), confirm
     `TestComputeReturnsEmptyWhenNoNotesExist` still passes unmodified (truly
     no notes → still `""`).
   - `note_tools_test.go` (mcp package): `TestNoteMuteUnmuteRoundTrip` (mute
     then unmute via the MCP tools, assert visible state each way via
     `note.search`), `TestNoteMuteUnmuteDoesNotRestampWrittenAt` (override
     `noteNow` between write and mute/unmute calls, assert `written_at`
     unchanged — matching `TestNoteWriteRestampsWrittenAtOnOverwrite`'s
     pattern), `TestNoteWriteOverMutedKeyPreservesMute` (MCP-level version of
     the write-over-muted-key test, through `note.write`/`note.mute`/
     `note.write` again).
   - `note_workflow_manual_test.go`: a test extending
     `TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`'s
     pattern — write a note, mute it, call `workflow_manual`, assert the
     note's key/value are absent from the `# Notes` block while the muted
     count line is present, and a follow-up `note.search` still returns it.
8. `ai-docs/spec/mcp-tools.md` — update `## Note Tools {#260810-note-tools}`
   wire-shape paragraph (`L456-L463`) to add `visible` to the record shape
   and document the `note.write` preserve/default contract; add
   `note.mute`/`note.unmute` bullets beside the existing `note.write`/
   `note.erase`/`note.search` bullets (`L465-L480`). Update
   `### Note Injection {#260810-note-injection}` (`L691-L715`) to document
   the cap-exclusion and the new muted-count line, including the all-muted
   render edge.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/wsnote/...` — covers record
  migration, `Write`/`RepoWrite` preserve-on-existing/default-on-new,
  `SetVisible`/`RepoSetVisible` idempotency and no-restamp, and `Compute`'s
  cap-exclusion/muted-line/all-muted-edge behavior.
- `cd agents-plugin-tool && go test ./internal/mcp/... -run Note` — covers
  the new `note.mute`/`note.unmute` MCP handlers, the write-over-muted-key
  regression, the no-restamp regression, and the end-to-end
  `workflow_manual` injection-exclusion test.
- `cd agents-plugin-tool && go build ./...` — schema/dispatch wiring compiles
  (new tool names referenced in both `tools()` and `callTool`).
- Manual/functional check per the ticket's own verification boundary: write
  → mute round trip drops the note from `workflow_manual` while
  `note.search` still returns it and `written_at` is unchanged; unmute
  restores it; muting an over-cap-adjacent note frees a slot for a
  previously-elided note; muted-count and over-cap elision lines render
  independently and together; muting an already-muted key is a no-op — all
  covered by the automated tests above, so no separate manual pass is
  required.

## Escalations

- None.
