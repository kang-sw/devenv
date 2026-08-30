# Plan: 260807-feat-note-memory-layers — Phase 1: Non-tracked machine + worktree layers with workflow_manual injection

## Relevant Ticket Contract

- Two non-tracked layers only: `machine` (PC-global, project-agnostic;
  substrate: global config home / `WS_CONFIG_HOME`, `~/.ws/config.json`
  neighborhood) and `worktree` (worktree-local, ephemeral; substrate: existing
  `wsstate.layoutFor` `proj/<projectKey>@<worktreeKey>/` tree). NOT the flat
  session-key store.
- Verbs: `note.write(layer, [[key, value, priority], ...])` full-overwrite
  (updates priority), `note.erase(layer, [key, ...])`, `note.search(glob,
  from?, then?)`. Record shape `[key, value, priority, written_at]`. Higher
  integer priority = higher priority.
- A `# Notes` block injects into `workflow_manual` output on BOTH
  fresh-with-root and continue branches, modeled on the `scopeAnnouncement` /
  `computeManuals` compute-then-inject pattern, rendered **near Session State
  (append)**, not as a top-of-body banner. Inject highest-priority items up to
  a cap; elide the rest behind a visible "N lower-priority notes elided" line;
  elided items retrievable via `note.search`.
- Must NOT add any git-mutation/commit MCP verb (260605 pivot constraint). The
  non-tracked layers are the whole v1; the tracked `repo` layer is out of
  scope.
- Must NOT resurrect or build on the retiring `session.note` surface
  (`agents-plugin-tool/internal/mcp/session_auth.go:62-64,276`,
  `server.go:547-548,1914-1923,3547`) — fresh `note.*` surface, no shared code.
- Verification boundary (ticket text): a `note.write` -> `note.search`
  round trip returns the written record on each layer; a `workflow_manual`
  call after a write renders the `# Notes` section carrying that record; a
  `worktree`-layer note is absent from a different worktree's injection while
  a `machine`-layer note is present in both.

## Out of Scope

- The tracked `repo` layer (epic-deferred, future ticket per the ticket's own
  Phase-1 boundary).
- The `git-master` layer (explicitly rejected in ticket Decisions).
- Any git-mutation/commit MCP verb.
- Touching or refactoring `session.note` / `session_auth.go` retirement
  (owned by `260730-refactor-retire-goal-fan-out-step-and-session-note`,
  currently `todo/`, not landed) — leave that surface untouched.
- CLI mirror for `note.*` (see Codebase Findings: session-keyed tools have no
  CLI mirror precedent; `note.*` follows the same shape).
- `category`/tag fields on notes (ticket explicitly rejects this).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/manuals_announcement.go:1-38` — the
  `computeManuals(root string) string` pure-compute pattern to mirror for
  `computeNotes`: silent (`""`) on any resolution error or empty result, never
  blocks manual rendering, formats a `# <Heading>` block with one line per
  item.
- `agents-plugin-tool/internal/mcp/scope_announcement.go:1-37` — the
  sibling `scopeAnnouncement(root string) string` compute pattern (same
  shape, prepended today).
- `agents-plugin-tool/internal/mcp/workflow_manual.go:251-286` (FRESH-with-root)
  and `:292-316` (CONTINUE) — the two injection branches. **Important
  divergence from the ticket's own framing**: `scopeAnnouncement` and
  `computeManuals` are today both wired through
  `injectBootstrapStalenessWarning(body, ...)`
  (`agents-plugin-tool/internal/mcp/bootstrap_alarm.go:88-96`), which
  **prepends** `warning + "\n\n" + body` — i.e. they render as top-of-body
  banners, not "near Session State". The ticket explicitly wants `# Notes`
  appended near Session State instead. Concretely: in both branches, `body +=
  "\n\n" + renderSessionState(...)` runs (workflow_manual.go:267 and :297)
  *before* the existing `injectBootstrapStalenessWarning`-based prepends run
  (:274-284, :304-315). The Notes append must happen immediately after that
  `renderSessionState` append (i.e. `body += "\n\n" + computeNotes(root)`) —
  using a plain string append, not `injectBootstrapStalenessWarning` — so it
  lands after Session State and before the existing prepended
  banners/scope/manuals blocks compose on top of it. This is a real behavioral
  difference from the `scopeAnnouncement`/`computeManuals` precedent this
  ticket names as its own model; the compute-function *shape* (pure,
  root-in/string-out, silent-on-empty) is what should be mirrored, not the
  injection call site.
- `agents-plugin-tool/internal/wsdoc/manuals.go:1-91` — sibling discovery-tool
  family shape for `note.search`: `ManualInfo` struct, `ManualsList`,
  `ManualsFind(root, query)` with a query matcher. `note.search` differs (glob
  over key + date-range on `written_at`, not free-text), so this is a shape
  precedent, not directly reusable code.
- `agents-plugin-tool/internal/mcp/server.go:1180-1246` (mental_models.*),
  `:4230-4290` (manuals.* schema+dispatch), `:3547` (session.note schema entry)
  — exact tool-registration/dispatch shape to mirror: schema block in the big
  `tools()` array literal (`stringProperty` helper), a `case "family.verb":`
  in `callTool`'s switch, `resolveToolRoot(params.Arguments, params.Meta)` for
  root-aware calls, `wantsJSON(params.Arguments)` for the JSON escape hatch,
  `toolTextResponse`/`toolJSONResponse` wrapping.
- `agents-plugin-tool/internal/mcp/server.go:56-85` (`isLeadOnlyTool`,
  `workflowPreferenceWriterTool`) and `:4774-4789` (`roleAllowsTool`) — `note.*`
  matches no lead-only prefix (`lead.`, `session.`, `config.`, `mercenary.`) and
  no `permanentlyHiddenTool`/`noAgentHiddenTool` prefix, so it needs **no**
  new gating entries; it is reachable by any scope (lead/delegate/leaf) that
  holds a session key, mirroring `todo.*`/`agenda.*`
  (`agents-plugin-tool/internal/mcp/session_state.go:1141-1145,1233-1237`
  show the sibling append/erase verb shape with array-of-items args).
- `agents-plugin-tool/internal/mcp/server.go:3246-3259` (`resolveToolRoot`) —
  session_key is the sole root source; an unknown/absent key errors before any
  handler logic runs. `note.write`/`note.erase`/`note.search` should require
  `session_key` uniformly (matches "every ws tool call carries a session key")
  even though the `machine` layer does not need the resolved root — resolve
  root only when `layer == "worktree"`, skip root resolution for `layer ==
  "machine"` (still validate `session_key` is a known key via
  `s.sessions.lookup`, not `resolveToolRoot`, when layer is machine-only —
  see `requireLeadSessionKey`-style key lookup at
  `agents-plugin-tool/internal/mcp/server.go:4880-4893` for the lookup-without-root
  shape, though that helper is lead-only and must NOT be reused verbatim since
  `note.*` is not lead-only).
- `agents-plugin-tool/internal/wsconfig/global.go:1-51` — `GlobalPath(opts)`
  resolves `<ConfigHome|WS_CONFIG_HOME|~/.ws>/config.json`; the `machine` note
  store should live at the sibling path `filepath.Join(filepath.Dir(GlobalPath(opts)), "notes.json")`
  (i.e. `~/.ws/notes.json` by default) — reuse `GlobalPath`, do not
  reimplement the env/home resolution chain.
- `agents-plugin-tool/internal/wsstate/paths.go:74-96` (`CacheRoot`),
  `:142-167` (`Manager.Ensure`), `:176-199` (`layoutFor`) — `Ensure(repoPath)`
  returns `(Layout, ProjectMetadata, WorktreeMetadata, error)`; `Layout.WorktreeDir`
  is the `proj/<projectKey>@<worktreeKey>/` directory. The `worktree` note
  store should live at `filepath.Join(layout.WorktreeDir, "notes.json")` — call
  `wsstate.NewManager(wsstate.Options{}).Ensure(root)` with the
  `resolveToolRoot`-resolved root (already canonical), do not hand-roll git
  identity resolution.
- `agents-plugin-tool/internal/wsconfig/resolver.go:302-369`
  (`setOverrideInFileRMW`/`setOverrideInFile`) — the `gofrs/flock` +
  temp-file + atomic-rename RMW pattern (`path+".lock"`,
  `fl.TryLockContext(ctx, 50*time.Millisecond)`, `os.CreateTemp` +
  `os.Rename`) to reuse for both note-store files' write/erase paths. This is
  the established concurrent-safe-write mechanism in this codebase; do not
  invent a second one.
- `agents-plugin-tool/internal/wsstate/paths.go:317-365` (`upsertJSON`) is a
  second, slightly different atomic-write helper (no flock, just
  `metadataWriteMu` in-process mutex + temp+rename) used for project/worktree
  metadata — cross-process safety there relies on those files being
  effectively single-writer-per-machine-process already. Note stores are
  cross-process-writable (multiple MCP server instances / worktrees), so the
  `wsconfig` flock pattern is the correct one to copy, not `upsertJSON`.
- `agents-plugin-tool/cmd/ws-mcp/main.go:1-66` (top-level command switch),
  `:851-897` (`manualsCommand`/`manualsList`/`manualsFind`) — CLI mirror shape
  for root-based (`--root`) discovery tools. Searching for `"todo"`, `"agenda"`,
  `"session"`, `"enter"`, `"workflow"` in this file (grep, no hits) confirms
  session-keyed tools (`todo.*`, `agenda.*`, `enter.*`, `session.*`) have **no**
  CLI mirror — the CLI takes `--root`, not `--session_key`, and these tools'
  authority model is session-key-only. `note.*` is session-keyed the same way,
  so **no CLI mirror is in scope** for this phase (diverges from the
  "Add a CLI mirror" extension-point recipe in the mcp-runtime mental model,
  which applies to root-only tools).
- `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`
  (`tools` object, e.g. lines with `"manuals.list": ">=0.39.3-dev <0.40.0"`,
  `"todo.append": ">=0.39.3-dev <0.40.0"`) — both files need three new entries
  (`note.write`, `note.erase`, `note.search`) with a version floor at the next
  bumped dev version (current shipped `version` in
  `agents-plugin/.claude-plugin/plugin.json:3` is `0.39.3`; the version bump
  itself runs via `agents-plugin-tool/scripts/bump-ws-version.sh` per
  AGENTS.md's dev-merge rule, not hand-edited).
- `ai-docs/spec/mcp-tools.md:582-602` (`### Manuals Ambient Injection
  {#260807-manuals-ambient-injection}`) — a sibling spec section already
  landed under the *same* `260807` epic prefix for the analogous
  `# Manuals` block; its prose shape (silent-when-empty, no applicability
  predicate, branch scoping identical to what Notes needs) is the direct
  template for the new `### Note Injection` spec subsection. `ai-docs/spec/mcp-tools.md`
  is 2063 lines; the new `note.*` tool family entry belongs near the other
  discovery/session-state families (after `## Session State Tools`,
  `{#260625-session-state-tools}`, around line 225-424) and the injection
  subsection belongs inside `### Workflow Manual Entry And Restoration`
  (`{#260626-workflow-manual-restoration-entry}`, lines 416-486) alongside the
  Bootstrap Staleness / Doc Coverage / Manuals Ambient subsections.
- Ticket relation `260523-bug-worktree-local-index-missing` — confirm this
  bug ticket exists and references the `_index.local.md` loss-across-worktree
  problem this phase closes; no code change needed there, just close/link on
  ship per the ticket's own text ("Closes 260523...").
- `agents-plugin-tool/internal/mcp/manuals_announcement_test.go` (44 lines)
  and `manuals_workflow_manual_test.go` (129 lines) — test-file split
  precedent to mirror: one file testing the pure `computeX` function in
  isolation, one testing its wiring into `handleWorkflowManual`'s FRESH/CONTINUE
  output.

## Implementation Plan

1. **New package `agents-plugin-tool/internal/wsnote/`** (mirrors the
   `wsconfig`/`wsstate` naming convention for a substrate-specific store,
   rather than folding this into `internal/wsdoc` — wsdoc owns git-tracked
   `ai-docs/` documents; notes are explicitly non-tracked and live outside the
   working tree, so they don't fit that package's contract). Files:
   - `wsnote/record.go`: `type Record struct { Key, Value string; Priority int; WrittenAt string }` (JSON tags matching the ticket's `[key, value, priority, written_at]` array-of-tuple wire shape — decide whether `note.write`'s wire arg is a JSON array-of-arrays or array-of-objects; **recommend array-of-objects** `{"key":..,"value":..,"priority":..}` for schema clarity over positional tuples, since MCP tool args are named JSON objects everywhere else in this codebase — no positional-array precedent exists in any other tool schema surveyed). `WrittenAt` stored as RFC3339 (matches `ProjectMetadata.CreatedAt`/`LastSeenAt` convention in `wsstate/paths.go:57-58`).
   - `wsnote/store.go`: `type Layer string` (`"machine"`, `"worktree"`); `MachinePath(opts wsconfig.Options) (string, error)` = sibling of `wsconfig.GlobalPath`; `WorktreePath(root string) (string, error)` = `wsstate.NewManager(wsstate.Options{}).Ensure(root)` then `filepath.Join(layout.WorktreeDir, "notes.json")`; `Load(path string) (map[string]Record, error)` (missing file -> empty map, mirrors `wsconfig.loadGlobalConfig`); `Write(path string, records []Record) error` and `Erase(path string, keys []string) error` both flock-serialized RMW copying `wsconfig/resolver.go:302-369`'s exact lock-path/temp-file/atomic-rename shape (new `path+".lock"`, do not share the wsconfig lock file).
   - `wsnote/search.go`: `Search(records map[string]Record, glob string, from, then string) ([]Record, error)` using stdlib `path.Match(glob, key)` for the glob (no new dependency; recommend this over a custom matcher since only key-glob is required, not multi-segment path globbing) and RFC3339 string comparison (or full-day-prefix comparison if `from`/`then` are date-only — decide the exact `from`/`then` format as RFC3339-or-date-prefix accepting both, documented in the spec entry).
   - `wsnote/inject.go`: `Compute(root string, opts wsconfig.Options, cap int) string` — the pure compute-then-render function mirroring `computeManuals`'s contract (silent `""` on any error, e.g. root resolution failure for the worktree layer must degrade to machine-only output rather than blocking, mirroring `scopeAnnouncement`'s "a resolution error is treated the same as inactive" comment at `scope_announcement.go:17-18`). Loads both layers, merges records (tag each with its source layer for the injected line), sorts by `Priority` descending with `WrittenAt` descending as a tiebreak (recommendation — not contract-specified, easily changed), takes the top `cap`, formats a `# Notes` block: `- [<layer>] <key> (priority <n>, <written_at>): <value>` per line, and when items remain beyond `cap` appends `\n(N lower-priority notes elided — use note.search to retrieve.)`. **Recommend `cap = 20`** as a named `const notesInjectionCap = 20` in `internal/mcp` (not `wsnote`, so it stays beside the other injection-shaping constants) — this is an arbitrary but trivially-tunable implementation constant, not a contract fork; do not escalate on the exact number.
   - `wsnote/*_test.go`: round-trip write/search/erase per layer, RMW concurrency-safety smoke test (mirrors any existing `wsconfig` RMW test if present), missing-file-returns-empty-map, malformed-JSON-file failure behavior.
2. **`agents-plugin-tool/internal/mcp/note_tools.go`** (new file, mirrors
   `session_state.go`'s todo/agenda handler shape): `handleNoteWrite`,
   `handleNoteErase`, `handleNoteSearch`. Each resolves `layer` from args,
   validates it's `"machine"` or `"worktree"`; for `"worktree"` calls
   `resolveToolRoot(params.Arguments, params.Meta)`; for `"machine"` looks up
   the session key via `s.sessions.lookup(key)` directly (no root needed) but
   still requires a valid, non-empty `session_key` (mirrors the
   "every ws tool call carries a session key" invariant — reject empty/unknown
   keys the same way `resolveToolRoot` would, just without consuming its
   `entry.root`). Calls into `wsnote` for the actual store operation. Text
   formatting: `formatNoteWrite`/`formatNoteErase` return compact confirmation
   lines; `formatNoteSearch` returns one line per matched record (mirrors
   `formatManuals`'s `"%s - %s\n"` shape at `server.go:3154-3160`), with a
   `format: "json"` escape hatch via `wantsJSON`/`toolJSONResponse`.
3. **`agents-plugin-tool/internal/mcp/server.go`**: add three schema entries
   in the `tools()` array (near the `manuals.*`/`mental_models.*` block,
   `server.go:4230-4290`) and three `case "note.write":` /
   `"note.erase":` / `"note.search":` dispatch lines in `callTool`'s switch
   (near `manuals.list`/`manuals.find`, `server.go:1224-1246`), delegating to
   the new handlers in `note_tools.go`. No entry needed in `isLeadOnlyTool`,
   `roleAllowsTool`, `noAgentHiddenTool`, or `permanentlyHiddenTool` (see
   Codebase Findings — `note.*` matches none of their prefix gates and should
   stay reachable by every scope, matching `todo.*`/`agenda.*`).
4. **`agents-plugin-tool/internal/mcp/note_announcement.go`** (new file,
   mirrors `manuals_announcement.go`): thin wrapper
   `computeNotes(root string) string` calling `wsnote.Compute(root,
   wsconfig.Options{}, notesInjectionCap)`.
5. **`agents-plugin-tool/internal/mcp/workflow_manual.go`**: in the
   FRESH-with-root branch, insert `body += "\n\n" + computeNotes(canonical)`
   immediately after the existing `body += "\n\n" + renderSessionState(sessionRecord{})`
   line (currently `:267`) and *before* the skeptical-posture / staleness /
   doc-coverage / scope / manuals prepend block that follows (currently
   `:268-285`) — do NOT wire this through `injectBootstrapStalenessWarning`
   (that prepends; see Codebase Findings divergence note). Guard against an
   empty `computeNotes` result the same way a plain string append already
   tolerates empty strings (no special no-op needed if `computeNotes` returns
   `""` and the append is unconditional — verify the resulting body has no
   stray blank block by testing the empty case, matching how `computeManuals`
   returning `""` doesn't visibly break `injectBootstrapStalenessWarning`'s
   no-op path). Mirror the identical insertion in the CONTINUE branch
   immediately after `body += "\n\n" + renderSessionState(rec)` (currently
   `:297`), using `computeNotes(rec.Root)`.
6. **`agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`**:
   add `"note.write"`, `"note.erase"`, `"note.search"` to each file's `tools`
   object with a version floor matching whatever `bump-ws-version.sh` sets for
   this dev-merge (do not hand-pick a version string; run the bump script per
   AGENTS.md's dev-merge rule as part of landing this phase, not as a plan
   step to execute now).
7. **`ai-docs/spec/mcp-tools.md`**: add a `## Note Tools {#<generated-stem>}`
   section (use `spec_stem.generate` for the anchor, do not hand-mint) near
   the Session State Tools section (after line ~424, before `### Workflow
   Manual Entry And Restoration`) documenting `note.write`/`note.erase`/`note.search`,
   the layer argument, full-overwrite semantics, the record shape, and the
   glob/date-range search contract; and add a `### Note Injection
   {#<generated-stem>}` subsection inside `### Workflow Manual Entry And
   Restoration` (alongside Bootstrap Staleness/Doc Coverage/Manuals Ambient,
   modeled directly on `### Manuals Ambient Injection
   {#260807-manuals-ambient-injection}` at lines 582-602) documenting the
   near-Session-State append placement (explicitly noting it is NOT a
   top-of-body banner, unlike its sibling injections), the priority-ordered
   cap, and the elision line.
8. **Mental model update**: append a Common Mistakes / Module Contracts bullet
   to `ai-docs/mental-model/mcp-runtime.md` under `## Extension Points &
   Change Recipes` or `## Module Contracts` noting the
   `computeManuals`/`scopeAnnouncement` vs. `computeNotes` injection-placement
   divergence (prepend-as-banner vs. append-near-Session-State), since a
   future author copying the "compute+inject" pattern without reading
   `workflow_manual.go` closely would default to the prepend wiring and get
   the wrong placement.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go vet ./...`
- `cd agents-plugin-tool && go test ./internal/wsnote/... ./internal/mcp/... -run Note -v` (focused on new tests)
- `cd agents-plugin-tool && go test ./...` (full regression, since `workflow_manual.go` and `server.go` are shared/high-traffic files)
- Ticket-specified round trip: `note.write(layer: "machine", ...)` then
  `note.search(...)` returns the written record; repeat for `layer: "worktree"`.
- Ticket-specified injection check: after a `note.write`, call
  `workflow_manual(session_key)` (both FRESH-with-root and CONTINUE paths, per
  the ticket's explicit "both branches" requirement) and confirm the `# Notes`
  block carries the record, positioned after `## Session State` and not as a
  leading banner.
- Ticket-specified cross-worktree isolation check: mint two session keys
  against two different worktree roots (or the same repo checked out twice),
  write a `worktree`-layer note under one, confirm `workflow_manual` for the
  *other* root's key does not show it, while a `machine`-layer note written
  under either key appears in both.
- Elision check: write more than `notesInjectionCap` records across both
  layers, confirm the injected block caps at the highest-priority items and
  shows the "N lower-priority notes elided" line, and confirm the elided
  items are retrievable via `note.search`.
- Empty-state check: call `workflow_manual` with no notes ever written and
  confirm the `# Notes` block is absent (not an empty heading), matching
  `computeManuals`'s empty-is-silent contract.
- `agents-plugin/runtime.json` / `agents-plugin-wsflow/runtime.json` — confirm
  both files list all three new tool names after the version bump script runs
  (manual diff, no automated test found for this beyond whatever launcher
  compatibility test already exists — search for it during implementation:
  `grep -rn "runtime.json" agents-plugin-tool/internal/mcp/*_test.go`).

## Escalations

- None. The two candidate forks called out in the task framing (store
  serialization format, injection cap value) both resolve cleanly against
  existing precedent already in this codebase (the `wsconfig` flock+JSON RMW
  pattern for storage; an arbitrary, trivially-tunable named constant for the
  cap) and are not genuine strategic ambiguity — see the stated
  recommendations in Implementation Plan steps 1 and 4.
