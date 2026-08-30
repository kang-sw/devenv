# Plan: 260814-feat-note-search-optional-multi-layer — Phase 1: optional / multi-layer note.search

## Relevant Ticket Contract

- `layer` becomes **optional** on `note.search`; omitted = search **all** four
  layers (parallel to `wsnote.Compute`'s aggregation).
- `layer` accepts **either a single string or an array** of layer names, so a
  caller can scope to a subset (e.g. `["clone", "repo"]`) in one call.
- `write`/`erase`/`mute`/`unmute` stay **single-layer and required** —
  untouched by this phase; do not generalize their schemas or handlers.
- Backward-compatible: a single-string `layer` call keeps today's behavior and
  result shape (same JSON shape: plain `[]wsnote.Record`, no layer tag).
- A cross-layer or array result set tags each record with its originating
  layer, mirroring the ambient block's `[<layer>]` render.
- Sub-decision (a) — pin in spec: single-string `layer` stays **untagged**
  (plain `wsnote.Record[]`), per the ticket's own "keeps today's behavior and
  result shape" wording. Only omitted/array `layer` calls get the layer tag.
- Sub-decision (b) — pin in spec: the **same comparator** (priority desc →
  written_at desc → key asc, `wsnote.Compute`'s order) governs every
  `note.search` call, single-layer or multi-layer, so `layer: "clone"` and
  `layer: ["clone"]` cannot diverge in order — trivially guaranteed by routing
  both paths through one shared sort function.
- `muted`/`visible`: `note.search` returns muted notes today; cross-layer
  search preserves that (no `visible` filtering anywhere in this phase).
- Spec target: `ai-docs/spec/mcp-tools.md`, `## Note Tools {#260810-note-tools}`.

## Out of Scope

- `### Note Injection {#260810-note-injection}` / `wsnote.Compute` — explicitly
  unaffected; do not modify `agents-plugin-tool/internal/wsnote/inject.go`
  behavior (a byte-identical comparator may be mirrored into `search.go`, but
  `inject.go` itself stays untouched).
- `note.write`/`note.erase`/`note.mute`/`note.unmute` schemas and handlers —
  stay required-single `layer`; do not touch `noteLayerArg` or their tool
  schemas in `server.go`.
- `visible` filtering in `note.search` — never applied; that stays a
  `Compute`-only concern (unchanged).
- `260814-feat-note-project-local-untracked-layer` (the `clone` layer itself)
  — already shipped; this phase only extends `note.search`'s argument shape.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/note_tools.go#L54-L115` —
  `resolveNoteStore(toolName, args, meta)` parses a single `layer` from args
  via `noteLayerArg` (L117-L132, requires exactly one valid enum string) then
  switches on it to resolve one `noteStore`. This switch body (L70-L114) is the
  reusable per-layer resolution logic (machine needs only `session_key`;
  worktree/clone/repo all call `s.resolveToolRoot(args, meta)`) — it must be
  factored to take an already-known `wsnote.Layer` instead of re-parsing args,
  so `handleNoteSearch` can call it once per requested layer without
  disturbing `noteLayerArg`'s required-single contract used by the other four
  handlers.
- `agents-plugin-tool/internal/mcp/note_tools.go#L292-L313` —
  `handleNoteSearch` currently: resolve one store → `store.Load()` → one call
  to `wsnote.Search(records, glob, from, then)`. Multi-layer search needs a
  loop over the resolved layer list, one `Load()` + filter per layer, tagging
  each surviving record with its layer, then one merge-sort across the
  combined set.
- `agents-plugin-tool/internal/mcp/server.go#L5159-L5172` — `stringList(value
  any) []string` already parses a `[]any` JSON array into `[]string` (used by
  `tickets.list`'s `statuses` arg at L1284/L1308/L3101). Reusable for parsing
  `layer` when it arrives as an array; note it silently drops non-string/empty
  entries rather than erroring, so array-item validation against the 4 valid
  layer names still needs an explicit check (same enum-membership check
  `noteLayerArg` already does per string).
- `agents-plugin-tool/internal/mcp/server.go#L4344-L4359` — `note.search`'s
  current `inputSchema`: `layer` is `enumStringProperty(...)`, listed in
  `"required": []string{"session_key", "layer"}`. Needs to become optional
  (drop from `required`) and accept string-or-array-of-the-enum. No existing
  `anyOf`/`oneOf` schema helper exists anywhere in `server.go`
  (`stringProperty`/`enumStringProperty`/`stringArrayProperty`/
  `nullableEnumStringProperty` at L5217-L5313 are the closest precedents, all
  single-shape) — this is genuinely new schema shape, not a missed-reuse
  signal, and should stay a small local helper (e.g.
  `enumStringOrArrayProperty`) rather than a generic addition.
- `agents-plugin-tool/internal/wsnote/search.go#L29-L63` — `Search(records
  map[string]Record, glob, from, then string) ([]Record, error)` filters then
  sorts by **priority desc → key asc** (L56-L61) — this comparator omits the
  `written_at` tiebreak `Compute` uses, so it is the thing that must change
  for sub-decision (b) to hold. `Search` operates on one layer's `map[string]
  Record` at a time; it has no multi-map/tagging entry point today.
- `agents-plugin-tool/internal/wsnote/inject.go#L14-L17,88-96` —
  `layeredRecord` (unexported: `Record` + `Layer wsnote.Layer`) and its sort
  comparator (priority desc → written_at desc → key asc) are exactly the
  tagged-record shape and ordering `note.search`'s multi-layer path needs.
  `layeredRecord` itself is private to `wsnote` and used only inside
  `inject.go`'s `Compute`; reusing the *shape* (not necessarily the exact
  type) avoids re-deriving the comparator, without touching `Compute`'s
  behavior.
- `agents-plugin-tool/internal/wsnote/record.go#L29-L35` — `Record` fields
  carry `json:"key"`/`"value"`/`"priority"`/`"written_at"`/`"visible"` tags. A
  tagged wrapper struct embedding `Record` plus a `Layer` field (e.g.
  `json:"layer,omitempty"`) marshals cleanly via standard field promotion —
  no custom `MarshalJSON` needed (only `Record.UnmarshalJSON` is custom, and
  that is unaffected — it is not used on the output/marshal path).
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L369-L387` —
  `searchSingleNoteRecord` decodes `format:"json"` output as plain
  `[]wsnote.Record` for a single-string `layer` call; this is the existing
  regression contract sub-decision (a) must preserve exactly (untagged single-
  string shape).
- `agents-plugin-tool/internal/mcp/note_tools_test.go` and
  `agents-plugin-tool/internal/wsnote/search_test.go` — neither file has any
  existing assertion on `note.search`'s output *order* across multiple
  differently-prioritized/timestamped records, so changing `Search`'s
  comparator to match `Compute`'s is safe against current tests but needs new
  coverage per the ticket's own verify list.
- `ai-docs/spec/mcp-tools.md#L451-L460` — the shared paragraph "All five tools
  require `session_key` and a `layer` argument..." currently states `layer` as
  uniformly required across all five tools; must be qualified so it no longer
  misstates `note.search` once `layer` becomes optional there.
- `ai-docs/spec/mcp-tools.md#L497-L507` — the `note.search` bullet documents
  `note.search(session_key, layer, glob?, from?, then?)` with no ordering
  statement and no tag-shape statement; this is the exact bullet to extend
  with the optional/array `layer`, the tag-presence rule (sub-decision a), and
  the ordering rule (sub-decision b).

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/note_tools.go#L65-L115` — split
   `resolveNoteStore` into:
   - `resolveNoteStore(toolName, args, meta)` (unchanged signature/behavior
     for `write`/`erase`/`mute`/`unmute`): parse via `noteLayerArg`, then
     delegate to the new function below.
   - `resolveNoteStoreForLayer(toolName string, layer wsnote.Layer, args
     map[string]any, meta map[string]any) (noteStore, error)`: the existing
     switch body (L70-L114) verbatim, parameterized on `layer` instead of
     re-parsing `args["layer"]`.
2. `agents-plugin-tool/internal/mcp/note_tools.go` (near `noteLayerArg`,
   L117-L132) — add `noteSearchLayersArg(args map[string]any) (layers
   []wsnote.Layer, tagged bool, err error)`:
   - `args["layer"]` absent/nil → `layers` = all four
     (`LayerMachine, LayerWorktree, LayerClone, LayerRepo`), `tagged = true`.
   - `args["layer"]` is a `string` → validate against the 4-value enum (reuse
     `noteLayerArg`'s validation shape); `layers` = that one, `tagged =
     false` (sub-decision a: legacy untagged shape).
   - `args["layer"]` is a `[]any` → parse via `stringList` (server.go
     L5159), validate each entry is one of the 4 valid layer strings
     (non-empty array required — reject empty array as a caller error, same
     "invalid layer" error shape), `tagged = true` even for a one-element
     array (per ticket wording: "multi-layer/array results" get tagged).
   - Anything else → error, mirroring `noteLayerArg`'s error text style.
3. `agents-plugin-tool/internal/wsnote/search.go#L56-L61` — change `Search`'s
   sort comparator to the 3-key form (priority desc → written_at desc → key
   asc), matching `inject.go`'s `Compute` comparator (L88-L96), to satisfy
   sub-decision (b) for the single-layer path.
4. New tagging + merge for the multi-layer path, in
   `agents-plugin-tool/internal/mcp/note_tools.go` (module-level, near
   `handleNoteSearch`):
   - `type taggedNoteRecord struct { wsnote.Record; Layer wsnote.Layer
     \`json:"layer,omitempty"\` }`.
   - A small merge helper: for each `layer` in the resolved list, resolve its
     store via `resolveNoteStoreForLayer`, `Load()`, run the same filter
     `wsnote.Search` uses (glob/from/then) to get that layer's matches, wrap
     each in `taggedNoteRecord{Record: rec, Layer: layer}`, append to one
     combined slice. Sort the combined slice with the same 3-key comparator
     from step 3 (duplicate the 6-line literal here, or factor a tiny shared
     `wsnote.CompareRecords(a, b Record) bool` if that reads cleaner —
     `inject.go`'s `Compute` itself must NOT be edited, only optionally have
     its comparator literal replaced by a call to the same shared helper with
     identical behavior).
   - If `wsnote.Search` cannot be reused directly for the filter-only step
     (it currently also sorts), extract an unexported `filterRecords(records
     map[string]Record, glob, from, then string) ([]Record, error)` in
     `search.go` that `Search` itself calls before sorting, and have the
     multi-layer merge call the same `filterRecords`.
5. `agents-plugin-tool/internal/mcp/note_tools.go#L292-L313` —
   `handleNoteSearch`: replace the single-store resolve+load+search with:
   - `layers, tagged, err := noteSearchLayersArg(args)`.
   - If `!tagged` (single-string case): unchanged behavior — resolve that one
     store, `Load()`, `wsnote.Search(...)`, return `[]wsnote.Record` exactly
     as today (now ordered by the step-3 comparator).
   - Else: run the step-4 merge across `layers`, return
     `[]taggedNoteRecord` for `format:"json"`, and a tagged text format (new
     `formatNoteSearchTagged` mirroring `formatNoteSearch` at L345-L354 but
     prefixing `[%s] ` per line, matching `inject.go`'s `"- [%s] %s (priority
     %d, %s): %s\n"` tag style).
6. `agents-plugin-tool/internal/mcp/server.go#L4344-L4359` — `note.search`
   schema:
   - Add a small local helper (near `enumStringProperty`, L5241-L5247), e.g.
     `enumStringOrArrayProperty(description string, values []string)
     map[string]any` returning `{"description": ..., "anyOf": [
     enumStringProperty("", values), {"type": "array", "items":
     enumStringProperty("", values)}]}`.
   - Change `layer` property to use this helper; update its description to
     state: optional, single string or array of the 4 layer names, omitted =
     search all four.
   - Drop `"layer"` from `"required"` (keep only `"session_key"`).
   - Extend the tool `"description"` field to mention optional/multi-layer
     search and the layer tag on cross-layer results.
7. `ai-docs/spec/mcp-tools.md`:
   - L451-L460 — qualify "All five tools require `session_key` and a `layer`
     argument" so `note.search`'s `layer` is called out as optional (string,
     array, or omitted = all four), while the other four stay required-single.
   - L497-L507 (`note.search` bullet) — extend to document: optional/array
     `layer`; omitted = all four layers; sub-decision (a) (single-string stays
     untagged, array/omitted results carry a `layer` field per record); sub-
     decision (b) (all `note.search` results, single or multi-layer, order by
     priority desc → written_at desc → key asc, matching `#260810-note-
     injection`'s `Compute` order — so `layer: "clone"` and `layer:
     ["clone"]` never diverge in order, only in tag presence).
   - Explicitly note (one sentence) that `write`/`erase`/`mute`/`unmute` stay
     required-single by design — read-vs-mutation asymmetry, not an
     inconsistency — per the ticket's own "Decisions" section, so a future
     reader does not "fix" this later.

## Verification Plan

- `go test ./agents-plugin-tool/internal/wsnote/... ./agents-plugin-tool/internal/mcp/...`
  after adding:
  - `wsnote` package (`search_test.go`): a new ordering test — 3+ records
    spanning distinct priority, then equal-priority distinct `written_at`,
    then equal-priority-and-`written_at` distinct `key` — asserting `Search`'s
    output order matches `Compute`'s 3-key comparator.
  - `mcp` package (`note_tools_test.go`):
    - Omitted `layer`: write one note per layer (reuse `mintRootKey` +
      per-layer write helpers already in the file), call `note.search` with
      no `layer`, assert all four notes come back, each tagged with its
      layer, in priority→written_at→key order.
    - Array `layer` (e.g. `["clone", "repo"]`): assert exactly those two
      layers' notes are returned and no others.
    - Single-string `layer` (regression): assert output is still plain
      `[]wsnote.Record` (no `"layer"` key in the JSON), matching
      `searchSingleNoteRecord`'s existing decode contract.
    - Muted notes: write + mute a note on one layer, confirm it still
      surfaces via the multi-layer/array path (mirrors the single-layer
      mute-search precedent already covered elsewhere in the file).
    - Order parity: `layer: "clone"` vs. `layer: ["clone"]` with the same
      fixture data return byte-identical record order (differing only in tag
      presence per sub-decision a).
    - `note.write`/`note.erase`/`note.mute`/`note.unmute` schema/behavior
      regression: existing tests in this file already cover required-single
      `layer` for these four; re-run as-is to confirm no incidental
      behavior change from the `resolveNoteStore` split in step 1.

## Escalations

- None.
