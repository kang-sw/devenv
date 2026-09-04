# Plan: 260901-feat-note-oversize-layer-aware-clone-path — Phase 1: Layer-aware oversize nudge + clone path kind

## Relevant Ticket Contract

- Nudge becomes layer-aware, branched in `handleNoteWrite` on the resolved
  layer: **repo** keeps current advice (move detail into
  ticket/spec/mental-model, keep a `<300`-byte **relative** pointer);
  **worktree** advises a gitignored local doc (e.g. sibling `*.local.md`) with
  a relative pointer; **clone** advises allocating via
  `path.generate(kind: "clone", stem: …)`, writing detail there, and keeping
  the returned **absolute path** as pointer (never relative, never a
  gitignored in-worktree file); **machine** is prose-only (excluded doc
  referenced by an absolute path, no dedicated allocator).
- Trim the large-text-keep carve-out from every variant — notes are
  always-injected, so "keep it if volatile AND homeless AND
  must-always-stay-in-context" is removed; an irreducible sub-300-byte pointer
  passes the threshold naturally.
- New `path.generate` kind `"clone"`: allocates under `layout.SharedDir`
  (suggested subdir `SharedDir/docs/`), readable filename
  `<sanitized-stem>-<6-char [a-z0-9] suffix>.md` (not the opaque `runID-NN`
  scheme `review`/`prompt` use). Reserve with `O_CREATE|O_EXCL`, retry on
  suffix collision (mirror `plan` kind's `reserveUniqueGeneratedPath`), never
  `O_TRUNC` — a collision must never clobber a sibling clone doc.
- Constraint: do not add cross-worktree resolution logic — target
  `layout.SharedDir` directly (its clone-stability across worktrees is an
  existing property, no new logic needed).
- Constraint: Go server code only (`note_tools.go`, `generated_paths.go`); no
  wsflow skill mirror needed.
- 300-byte `noteOversizeThreshold` is unchanged; JSON-mode responses must
  continue to omit the nudge entirely.
- Rejected: `note.write` must NOT auto-relocate or mutate note content — the
  nudge is advice only; the calling agent performs allocate → write → repoint
  itself.
- Spec Impact (same phase, no new spec stem): update
  `ai-docs/spec/mcp-tools.md {#260810-note-tools}` oversize-challenge
  paragraph to the layer-aware contract, and
  `{#260505-workflow-state-delegation-tools}` `path.generate` paragraph to add
  `kind: "clone"`.

## Out of Scope

- Auto-relocation/spill-on-write behavior (rejected alternative).
- A first-tier document-authoring tool for clone docs (rejected).
- A dedicated `kind: "machine"` allocator (rejected — YAGNI, machine stays
  prose-only).
- A keyed clone blob store with erase/list lifecycle (rejected — minimal
  allocator only).
- Bootstrap `AGENTS.template.md` layer guidance addition (explicitly named as
  an optional out-of-scope follow-up in the ticket).
- Any phase beyond Phase 1 (ticket has only Phase 1).

## Codebase Findings

- `agents-plugin-tool/internal/wsstate/generated_paths.go#L21-L67` —
  `GeneratePaths` special-cases `kind == "plan"` before falling through to a
  generic branch that resolves `dir, ext` via `generatedPathTarget` and
  reserves `<runID>-<NN>-<stem><ext>` with a single non-retrying
  `O_CREATE|O_EXCL` open. The `"clone"` kind needs a *different* filename
  shape (`<stem>-<suffix>.md`, not `runID-NN-stem`) and retry-on-collision
  semantics the generic branch doesn't have, so it must be special-cased like
  `"plan"` (a new `generateClonePaths` method), not just added as a case
  inside `generatedPathTarget` alone.
- `agents-plugin-tool/internal/wsstate/generated_paths.go#L93-L102` —
  `generatedPathTarget(layout, kind)` is the small dir/ext resolver; add
  `case "clone": return filepath.Join(layout.SharedDir, "docs"), ".md", nil`
  here (used by the new `generateClonePaths` to resolve the target dir; no
  new `Layout` field needed — constraint says target `SharedDir` directly).
- `agents-plugin-tool/internal/wsstate/generated_paths.go#L113-L133` —
  `reserveUniqueGeneratedPath` is the `plan` kind's collision-retry pattern
  (loop, `O_CREATE|O_EXCL`, retry on `os.IsExist`, never truncate). Mirror
  this loop shape for clone's reservation but generate a *fresh random 6-char
  `[a-z0-9]` suffix* per attempt (not an incrementing numeric suffix) —
  requires a new suffix-generator helper, e.g. `randomBase36(6)`, distinct
  from the existing `randomHex` (which is hex, not base36, and used for the
  opaque `runID`).
- `agents-plugin-tool/internal/wsstate/paths.go#L176-L199` — `layoutFor`
  already defines `SharedDir = filepath.Join(projectDir, "shared")` keyed on
  `projectKey` (the clone's common git root), independent of `worktreeKey`.
  Confirms the ticket's claim that `SharedDir` is already worktree-agnostic
  and clone-shared with no new resolution logic required — join `"docs"`
  onto it directly.
- `agents-plugin-tool/internal/wsstate/generated_paths_test.go#L117-L171` —
  existing `TestGeneratePathsUsesWorktreeScoped{Review,Prompt}Directory`
  pattern to mirror for a new clone-dir test.
- `agents-plugin-tool/internal/wsstate/paths_test.go#L185-L226` —
  `TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState` shows the
  `runGit(t, repo, "worktree", "add", "-b", ..., worktreePath, "HEAD")` +
  `manager.Ensure(...)` pattern to reuse for the "linked worktree resolves to
  the same clone dir" test (call `GeneratePaths(repo, "clone", ...)` and
  `GeneratePaths(worktreePath, "clone", ...)`, assert both resolve under the
  same `layout.SharedDir/docs`).
- `agents-plugin-tool/internal/mcp/note_tools.go#L288-L298` —
  `noteOversizeThreshold` (300) and the single `noteOversizeChallenge`
  constant to replace with a layer-branched text producer.
- `agents-plugin-tool/internal/mcp/note_tools.go#L311-L335` — `handleNoteWrite`
  resolves the store via `s.resolveNoteStore(tool, args, meta)`, which
  internally parses `args["layer"]` via `noteLayerArg` but returns only the
  `noteStore` interface, **not** the resolved `wsnote.Layer` value — the
  ticket text's "resolved layer is already in scope" is only true in the
  sense that the *args* are in scope; `handleNoteWrite` must obtain the
  `wsnote.Layer` itself before selecting nudge text. Cheapest, most surgical
  path: call `noteLayerArg(tool, args)` a second time in `handleNoteWrite`
  after `resolveNoteStore` succeeds (args are already validated, so the
  second parse cannot newly fail) rather than changing `resolveNoteStore`'s
  return signature, which would force unrelated edits at its other three call
  sites (`handleNoteErase`, `handleNoteSetVisible` via
  `handleNoteMute`/`handleNoteUnmute`) that don't need the layer value.
- `agents-plugin-tool/internal/mcp/note_tools.go#L66-L143` —
  `resolveNoteStore`/`resolveNoteStoreForLayer`/`noteLayerArg` confirm the
  four `wsnote.Layer` constants (`LayerMachine`, `LayerWorktree`,
  `LayerClone`, `LayerRepo`) to switch the nudge text on.
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L880-L988` — existing
  oversize tests (`TestNoteWriteSubThresholdDoesNotAppendOversizeChallenge`,
  `TestNoteWriteOversizeAppendsChallengeExactlyOnce`,
  `TestNoteWriteBatchWithOneOversizeAppendsChallengeOncePerCall`,
  `TestNoteWriteJSONModeOmitsOversizeChallenge`) all write to `"layer":
  "worktree"` and assert against the literal `noteOversizeChallenge` constant
  string — once that constant is replaced by per-layer text, these existing
  assertions must be updated to compare against the worktree-specific text
  (or a shared prefix/substring), and new tests added for repo/clone/machine
  layer text plus a "no large-text-keep carve-out present" assertion.
- `agents-plugin-tool/internal/mcp/server.go#L4293-L4304` — `path.generate`
  tool schema: `enumStringProperty(..., []string{"review", "prompt",
  "plan"})` and its description string both need `"clone"` added.
- `agents-plugin-tool/internal/mcp/server.go#L1439-L1453` — the `path.generate`
  dispatch case calls `wsstate.NewManager(...).GeneratePaths(root, kind,
  stems)` generically; no dispatch-level change needed since `GeneratePaths`
  itself branches per kind.
- `agents-plugin-tool/internal/mcp/server_test.go#L790-L829` —
  `TestPathGenerateAdvertisesAndAllocatesPlanPaths` checks the enum contains
  `{"review","prompt","plan"}` (subset check, not exact-set) — adding
  `"clone"` to the schema does not break this test; not required to touch,
  though optionally extending its `want` list to include `"clone"` is a
  reasonable low-risk addition since it is the same file/pattern.
- `ai-docs/spec/mcp-tools.md#L514-L535` — the `{#260810-note-tools}`
  `note.write` paragraph quotes the single oversize-challenge string verbatim
  ("Large note (≥300 bytes; saved). Prefer: move the detail into a
  ticket/spec/mental-model ... Keep the full text only if it's volatile AND
  homeless AND must-always-stay-in-context. Not mute."). Must be rewritten to
  describe the four layer-branched variants and the trimmed carve-out.
- `ai-docs/spec/mcp-tools.md#L1992-1999` — the
  `{#260505-workflow-state-delegation-tools}` paragraph describing
  `path.generate`'s `kind: "review"/"prompt"/"plan"` needs a `kind: "clone"`
  sentence appended (SharedDir-scoped, readable filename).

## Implementation Plan

1. `agents-plugin-tool/internal/wsstate/generated_paths.go`:
   - Add a `randomBase36(n int) (string, error)` helper (crypto/rand-backed,
     `[a-z0-9]` alphabet, length `n`), placed near `randomHex`.
   - Add `case "clone": return filepath.Join(layout.SharedDir, "docs"), ".md",
     nil` to `generatedPathTarget`.
   - In `GeneratePaths`, add `if kind == "clone" { return
     m.generateClonePaths(layout, kind, stems) }` alongside the existing
     `if kind == "plan"` branch (before the generic dir/ext + runID
     reservation path).
   - Add `generateClonePaths(layout Layout, kind string, stems []string)
     ([]GeneratedPath, error)`: resolve `dir, ext` via `generatedPathTarget`,
     `os.MkdirAll(dir, 0o755)`, then for each stem: sanitize via
     `sanitizeGeneratedPathStem`, loop a bounded number of attempts
     generating a fresh `randomBase36(6)` suffix each attempt, build
     `<safeStem>-<suffix><ext>`, open with `O_RDWR|O_CREATE|O_EXCL` (never
     `O_TRUNC`), retry on `os.IsExist`, else fail loud (mirror
     `reserveUniqueGeneratedPath`'s error shape/bound). Append
     `GeneratedPath{Kind: "clone", Stem: safeStem, Path: path}`.
2. `agents-plugin-tool/internal/mcp/server.go` — in the `path.generate` tool
   schema (`internal/mcp/server.go#L4293-L4304`), add `"clone"` to the
   `enumStringProperty` values list and extend the description to mention
   `kind: "clone"` (clone-scoped, `SharedDir`-backed, worktree-agnostic,
   readable filename).
3. `agents-plugin-tool/internal/mcp/note_tools.go`:
   - Replace the single `noteOversizeChallenge` const
     (`internal/mcp/note_tools.go#L294-L298`) with a per-layer text producer,
     e.g. `func noteOversizeChallengeFor(layer wsnote.Layer) string` (or a
     `map[wsnote.Layer]string`) with four variants per the Decisions text
     above (repo/worktree/clone/machine), each trimmed of the
     large-text-keep carve-out and each keeping the "Not mute." remediation
     framing.
   - In `handleNoteWrite` (`internal/mcp/note_tools.go#L311-L335`), after
     `noteWriteExceedsOversizeThreshold(records)` is true, resolve the layer
     via a second `noteLayerArg(tool, args)` call (args already validated by
     the earlier `resolveNoteStore` call, so this cannot newly fail) and
     append `noteOversizeChallengeFor(layer)` instead of the removed
     constant.
4. `agents-plugin-tool/internal/wsstate/generated_paths_test.go` — add tests
   per the ticket's Phase 1 test list, mirroring
   `TestGeneratePathsUsesWorktreeScopedPromptDirectory` (`#L139-L171`) and
   `TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState`
   (`paths_test.go#L185-L226`):
   - clone kind allocates under `layout.SharedDir/docs`.
   - a linked worktree (`git worktree add`) resolves `"clone"` generation to
     the same directory as the main worktree.
   - filename shape: `<sanitized-stem>-<6-char [a-z0-9]>.md`.
   - per-call uniqueness (two calls with the same stem produce different
     paths, mirroring `TestGeneratePathsSanitizesStemAndKeepsAllocationsUnique`).
   - a forced suffix collision retries rather than truncating the existing
     sibling file (assert the pre-existing file's content is untouched after
     the retry).
5. `agents-plugin-tool/internal/mcp/note_tools_test.go` — update the four
   existing oversize tests at `#L880-L988` to compare against the
   worktree-layer variant text (not the removed `noteOversizeChallenge`
   constant), and add:
   - one test per layer (repo/worktree/clone/machine) asserting each
     produces its own distinct nudge text matching the Decisions content
     (repo: relative pointer + ticket/spec/mental-model; worktree: gitignored
     local doc + relative pointer; clone: `path.generate(kind: "clone", ...)`
     + absolute path; machine: prose-only + absolute path, no allocator
     mention).
   - an assertion that the large-text-keep carve-out phrase ("volatile AND
     homeless AND must-always-stay-in-context") is absent from all four
     variants.
   - confirm JSON-mode omission and the 300-byte threshold stay unchanged
     (existing `TestNoteWriteJSONModeOmitsOversizeChallenge` continues to
     pass with a layer-agnostic "no challenge text" assertion, e.g. check
     absence of "Large note").
6. `ai-docs/spec/mcp-tools.md`:
   - Rewrite the oversize-challenge paragraph in `{#260810-note-tools}`
     (`#L514-535`) to describe the layer-branched contract (repo/worktree/
     clone/machine variants) and the trimmed carve-out, replacing the single
     quoted string.
   - Extend the `path.generate` paragraph in
     `{#260505-workflow-state-delegation-tools}` (`#L1992-1999`) with a
     `kind: "clone"` sentence (SharedDir-scoped, worktree-agnostic, readable
     `<stem>-<suffix>.md` filename).

## Verification Plan

- `go test ./...` from `agents-plugin-tool/` (ticket-specified verification
  command).
- Targeted: `go test ./internal/wsstate/... -run GeneratePaths` and
  `go test ./internal/mcp/... -run NoteWrite` for fast focused feedback
  during iteration.
- Manual: `ws/tickets_verify` or the project's normal ticket-write guardrail
  is not applicable here (this is a plan-execution phase, not a ticket edit),
  but confirm `ai-docs/spec/mcp-tools.md` edits stay within the existing
  anchor sections (no `renamed-spec` needed, no new spec stem per ticket's
  Spec Impact note).

## Escalations

- None.
