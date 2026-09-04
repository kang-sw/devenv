---
title: Layer-aware note oversize nudge + clone-scoped path.generate kind
spec:
  - 260810-note-tools
  - 260505-workflow-state-delegation-tools
sage-review-design: completed
related:
  260823-feat-note-write-oversize-relocate-nudge: origin — introduced the oversize relocate nudge this refines (dropped feature ticket)
sage-review-design-reviewed: e1ae4730b05b8e27
sage-review-completeness: completed
sage-review-completeness-reviewed: e1ae4730b05b8e27
completed: 2026-09-01
---

# Layer-aware note oversize nudge + clone-scoped path.generate kind

## Background

The ws/note oversize nudge is layer-blind. It is a single constant
(`noteOversizeChallenge` in `internal/mcp/note_tools.go`, appended once per
`note.write` when any record is `>= 300` bytes) that tells every layer the same
thing: "move the detail into a ticket/spec/mental-model and keep a `<300`-byte
pointer." Those targets are all git-tracked, so the advice is a category error
for the three untracked layers (`machine`, `worktree`, `clone`) — a caller
chose those layers precisely because the content must not be committed.

The `clone` layer is the sharp case. A clone note is shared across every
worktree of one clone (stored under the ws cache `SharedDir`), so:

- a **relative-path** pointer into an extracted doc does not resolve the same
  from a different worktree (no shared stable root), and
- a **gitignored file placed inside a worktree** is per-working-copy and
  invisible to sibling worktrees, so it does not satisfy clone-wide visibility.

Notes also feed the always-injected top-level `# Notes` block (via
`workflow_manual`, which is called frequently), so a large "must-stay" note is a
standing per-turn context-budget tax — must-stay is the *expensive* case, not a
free carve-out.

Two coupled gaps to close together:

1. The nudge advice is not layer-appropriate.
2. The `clone` layer has nowhere to evict content to — there is no
   clone-scoped, worktree-agnostic file allocator today (`path.generate` is
   worktree-scoped only: `review`/`prompt` target the worktree cache and `plan`
   targets the in-worktree tree).

## Decisions

- **Nudge becomes layer-aware.** The resolved layer is already in scope in
  `handleNoteWrite` (`resolveNoteStore` runs before the message is appended), so
  branch the appended text on it:
  - **repo** (git-tracked): keep current advice — move detail into a
    ticket/spec/mental-model, keep a `<300`-byte **relative** pointer.
  - **worktree** (untracked, per-working-copy): move the detail into a
    gitignored local doc (e.g. a sibling `*.local.md`), keep a pointer. A
    relative pointer is acceptable — a worktree has one stable root.
  - **clone** (untracked, shared across worktrees): allocate a clone-scoped doc
    via `path.generate(kind: "clone", stem: …)`, write the detail there, and
    keep the returned **absolute path** as the pointer. Never a relative path
    (breaks across worktrees) and never a gitignored in-worktree file
    (per-working-copy, invisible to siblings).
  - **machine** (untracked, PC-global): **prose-only** — advise an excluded doc
    referenced by an absolute path, with no dedicated allocator.
- **Trim the keep carve-out.** The current string licenses keeping large text
  when "volatile AND homeless AND must-always-stay-in-context." Because notes
  are always-injected, remove the large-text-keep license; an already-irreducible
  sub-300-byte pointer passes the threshold naturally and needs no carve-out.
- **New `path.generate` kind `clone`.** Allocates under `layout.SharedDir`
  (suggested subdir `SharedDir/docs/`). `SharedDir` is clone-shared and
  worktree-agnostic today (`SharedDir = cacheRoot/proj/<projectKey>/shared`,
  keyed on the clone common root), so any worktree resolves to the same
  directory with **no new resolution logic**. Filename is **readable**:
  `<sanitized-stem>-<6 chars [a-z0-9]>.md` (stem plus a 6-char base36 suffix),
  not the opaque `runID-NN` scheme `review`/`prompt` use, because callers
  reference these docs by name. Reserve-empty-file semantics match the existing
  kinds; the agent writes content with normal file tools.

## Rejected Alternatives

- **Auto-relocation on oversize.** Rejected: `note.write` stays a post-write
  nudge that never mutates note content; the agent performs the
  allocate → write → repoint steps itself. Auto-spill would break `note.write`'s
  "never a gate, never mutates content" contract.
- **A first-tier document-authoring tool for clone docs.** Rejected as too
  heavy; a `path.generate` allocation plus guidance prose is sufficient.
- **A dedicated `kind: "machine"` allocator.** Rejected as YAGNI — machine notes
  are rare and ws owns no machine-global doc root analogous to `SharedDir`;
  machine stays prose-only.
- **Keyed clone blob store with erase/list lifecycle.** Rejected for this ticket
  as heavier than needed; the minimal allocator (note holds the absolute path,
  caller manages the file) is sufficient.

## Constraints

- Do not add cross-worktree resolution logic — target `layout.SharedDir`
  directly; its clone-stability is an existing property.
- Go server code only (`note_tools.go`, `generated_paths.go`); no wsflow skill
  mirror needed (the nudge lives in the Go string, not skill text). Adding a
  clone line to bootstrap `AGENTS.template.md` layer guidance is an optional
  follow-up, out of scope here.

## Phases

### Phase 1: Layer-aware oversize nudge + clone path kind

Two coupled edits landing together:

1. `internal/wsstate/generated_paths.go` — add `kind: "clone"` to
   `generatedPathTarget` returning `SharedDir/docs` + `.md`; add a
   6-char `[a-z0-9]` suffix helper; filename `<sanitized-stem>-<suffix>.md`.
   Reserve with `O_CREATE|O_EXCL` and retry on suffix collision (mirror `plan`'s
   `reserveUniqueGeneratedPath`); never `O_TRUNC` — a suffix or same-stem
   collision must never clobber a sibling clone doc.
   Tests: clone kind allocates under `SharedDir`; a linked worktree resolves to
   the same directory; filename shape; per-call uniqueness; collision retries
   rather than truncating an existing file.
2. `internal/mcp/note_tools.go` — replace the single `noteOversizeChallenge`
   constant with layer-branched text (repo/worktree/clone/machine per
   Decisions), trimming the large-text keep license. Tests: each layer yields
   its own message; JSON mode still omits the nudge; the 300-byte threshold is
   unchanged.

Verification: `go test ./...` in `agents-plugin-tool`; targeted `note_tools` and
`generated_paths` tests.

### Result (351628de) - 2026-09-01

Landed on `impl/develop/shank-stank-quirk` (`58599fb8..351628de`).

- `note.write` oversize nudge is layer-branched via
  `noteOversizeChallengeFor(layer)`; `handleNoteWrite` re-derives the layer with
  a second `noteLayerArg` parse (kept `resolveNoteStore`'s signature to avoid
  unrelated call-site edits). The large-text-keep carve-out is removed from all
  four variants and the default.
- New `path.generate(kind: "clone")` allocates `<stem>-<6char [a-z0-9]>.md`
  under `SharedDir/docs` via a dedicated `generateClonePaths` +
  `reserveUniqueSuffixedGeneratedPath` + `randomBase36`; reserves with `O_EXCL`
  and retries on collision, never truncating. MCP schema enum/description now
  include `clone`. Confirmed no cache-cleanup path enumerates path kinds, so
  clone docs persist with the referencing note (no cleanup change needed).
- Spec: `{#260810-note-tools}` oversize paragraph rewritten to the layer-aware
  contract; `{#260505-workflow-state-delegation-tools}` gained the
  `kind: "clone"` sentence.

Deviation: added a `cloneSuffixGenerator` package-level var as a deterministic
test seam for the collision-retry path (production behavior unchanged).

Verification: full `go test ./...` in `agents-plugin-tool` green (14 packages).

Review: partitioned correctness/fit/test — all clean, no Critical/Important, no
relay. Two minors recorded only: (1) `randomBase36` uses modulo (slight bias,
negligible for a collision-avoidance suffix); (2) `cloneSuffixGenerator` is a
package-global rather than instance-scoped DI and its "mirrors Options.Now"
comment overstates the parallel — harmless (no parallel test relies on it).

## Spec Impact

- `ai-docs/spec/mcp-tools.md` `path.generate` entry: add `kind: "clone"`
  (clone-scoped `SharedDir` allocation, readable filename).
- `ai-docs/spec/mcp-tools.md {#260810-note-tools}` oversize section (currently
  quotes the single string): replace with the layer-aware contract and the
  trimmed carve-out.

Both are edits within the existing note-tools / path.generate contract areas —
no new spec stem.
