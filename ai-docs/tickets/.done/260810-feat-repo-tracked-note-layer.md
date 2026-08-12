---
title: Tracked repo note layer — one-key-per-file git-tracked substrate for cross-clone notes
parent: 260807-epic-mechanical-project-memory
related:
  260807-feat-note-memory-layers: prerequisite — extends its note.* surface and workflow_manual injection with a third, git-tracked layer
  260807-refactor-dissolve-project-index: consumer — this layer is the landed home for _index.md's tracked `# Session Notes`, without which that content would be demoted to non-tracked
  260730-refactor-retire-goal-fan-out-step-and-session-note: constraint — must not resurrect or build on the retiring session.note surface
  260811-feat-note-visibility-mute: coordinates — the layer-agnostic `visible`/mute attribute is owned there; this `repo` layer inherits it, and whichever of the two lands second extends the shared record shape
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-12
---

# Tracked repo note layer — one-key-per-file git-tracked substrate for cross-clone notes

## Background

`260807-feat-note-memory-layers` ships only the two **non-tracked** note layers
(`machine`, `worktree`). The tracked `repo` layer was left epic-deferred, to be
spun up "if and when the `_index.md` decomposition needs a tracked substrate".

The decomposition (`260807-refactor-dissolve-project-index`) now needs exactly
that. `_index.md`'s `# Session Notes` is **git-tracked, cross-clone** content;
routing it into a non-tracked layer would silently demote previously-shared notes
to machine-local, losing cross-clone persistence. The user's fork resolution
(2026-08-10) chose to keep that content tracked, which makes this layer a hard
prerequisite of the decomposition rather than a deferred maybe.

This ticket adds the third note layer, `repo`: git-tracked, one-key-per-file,
committed through the normal agent path, injected into `workflow_manual` on the
same `# Notes` surface as the other two layers.

## Decisions

Inherited from the epic's Cross-Child Decisions; restated here as this layer's
contract:

- **One key = one file.** Records live under `ai-docs/ws-notes/` (tracked), one
  file per key, so merge conflicts resolve on the filesystem with normal git
  tooling. No merge/conflict logic enters MCP — that was an explicit epic-level
  rejection of complex tooling in the note surface.
  - **Key→filename encoding is a required planning choice.** The shipped surface
    admits slash-bearing and dotted keys (its tests exercise a match-all glob
    across slash-bearing keys), so the raw key cannot be used as a path: a slash
    would nest directories and make `note.erase` a silent no-op that leaves a
    stale tracked note re-injected forever. Pick any reversible or stable-hash
    encoding — the only hard requirements are that it round-trips (so `erase`
    finds the file) and is deterministic across clones (so the same key yields
    the same filename and concurrent same-key writes collide as one git
    conflict, which is what the filesystem-merge rationale depends on).
- **No git-mutation verb in MCP.** Writing a tracked file and committing it is
  something an agent already does through the normal path; the value this layer
  adds over "just edit a file" is the forced `workflow_manual` injection and a
  uniform `note.*` surface. Staging/commit rides the ordinary agent commit flow,
  not a new `note.commit`-style tool (counter to the `260605` pivot).
- **Same record shape, same injection.** `[key, value, priority, written_at]`,
  identical to the non-tracked layers; the `# Notes` block renders `repo`-layer
  records alongside `machine`/`worktree` under the same priority-ordered cap.
- **`repo` layer, not `git-master`.** The rejected `git-master` cell
  (write-to-another-branch's-index) stays rejected; `repo` is simply the tracked
  layer visible by branch, resolved by ordinary merge.
- **Visibility is not owned here.** The `visible`/mute attribute across all note
  layers is authored by `260811-feat-note-visibility-mute`; this ticket adds only
  the `repo` layer and inherits `visible` when the two converge (see that ticket's
  coordination note for the record-shape ordering).

## Prior Art

- `260807-feat-note-memory-layers` — the `note.write` / `note.erase` /
  `note.search` surface and the `# Notes` `workflow_manual` injection this layer
  extends. This ticket must not fork a parallel surface; it adds `layer: "repo"`
  to the existing one.
- `scopeAnnouncement` — the `computeX(root) string` → inject pattern the `# Notes`
  block already follows.

## Spec Impact

The `note.*` family and its `# Notes` injection are addressed at ready time by
`260807-feat-note-memory-layers`'s Spec Impact (`spec/mcp-tools.md`). This ticket
extends that same contract with a third layer value.

- **`spec/mcp-tools.md`** `note.*` family: add `repo` to the layer argument and
  document its git-tracked, one-key-per-file storage under `ai-docs/ws-notes/`
  and its participation in the `# Notes` injection. Caller-visible change: a
  `note.write(layer: "repo", ...)` persists to a tracked file that other clones
  see after merge.

## Phases

### Phase 1: Tracked repo note layer with one-key-per-file storage and injection

Depends on `260807-feat-note-memory-layers` Phase 1 landing (the `note.*` surface
and `# Notes` injection must exist first). Deliver:

- A third `repo` layer on the existing `note.*` surface storing one file per key
  under `ai-docs/ws-notes/` (tracked), with the shared
  `[key, value, priority, written_at]` record shape.
- `repo`-layer records participate in the existing `# Notes` `workflow_manual`
  injection under the same priority-ordered cap and elision line.
- No new git-mutation MCP verb; staging/commit of `ai-docs/ws-notes/` files uses
  the normal agent commit path.

Verification: a `note.write(layer: "repo", ...)` writes one tracked file per key
under `ai-docs/ws-notes/`; the record appears in the next `workflow_manual`
call's `# Notes` block; a second clone/worktree on the same branch sees the file
after the normal commit+merge path (i.e. it is genuinely tracked, unlike the
`machine`/`worktree` layers).

### Result (d0cf6b80) - 2026-08-12

Landed the git-tracked `repo` note layer as a third value on the existing
`note.*` surface: one JSON file per key under `ai-docs/ws-notes/`, sharing the
`[key, value, priority, written_at]` record shape and the `# Notes`
`workflow_manual` injection (same priority-ordered cap and elision) with the
`machine`/`worktree` layers. No new git-mutation MCP verb; staging/commit rides
the ordinary agent path.

- **Key→filename encoding:** hex of the key's raw UTF-8 bytes + `.json` (e.g.
  `a/b.c` → `612f622e63.json`) — deterministic across clone/OS/locale,
  collision-free, and immune to the slash/dot-as-path hazard the ticket flagged;
  `note.erase` recomputes the identical name, so it reliably finds and removes
  the exact file.
- **Lock outside the tracked tree (review fix):** the per-key flock lives in a
  machine-local temp path (`os.TempDir()/ws-notes-locks/<sha256(abs path)>.lock`),
  not a sibling in `ai-docs/ws-notes/`, so the tracked directory only ever holds
  `.json` key files. Corrects a first-pass Important review finding where a
  `<hexkey>.json.lock` sidecar would have been staged and orphaned after erase.

Files: `internal/wsnote/{store.go,repo_store.go,inject.go,record.go}`,
`internal/mcp/{note_tools.go,server.go}`; spec `mcp-tools.md`
`#260810-note-tools`. Commits `dde338bd..d0cf6b80` (code+tests) and `c0f735a4`
(spec lock-location clarification).

Verification: `go build ./...` clean; `go test -count=1 ./internal/wsnote/...
./internal/mcp/...` green. Tests prove one-file-per-key write, `# Notes`
appearance, erase round-trip, genuine git-tracking via `git status --porcelain`,
slash/dotted-key flatness, three-layer sort/cap, and no lock/temp residue in the
tracked dir before or after erase.

Accepted minors (non-blocking): the flock parent under shared `/tmp` could
collide across OS users on a multi-user host (loud error, no corruption;
single-dev norm); very long keys can hit `ENAMETOOLONG` (fails loud,
pathological input); the per-key RMW duplicates `rmw()` rather than sharing a
helper (matches existing codebase precedent, plan-directed).

Deferred: the `visible`/mute attribute and its record-shape extension stay
owned by `260811-feat-note-visibility-mute`; whichever of the two lands second
extends the shared record shape.
