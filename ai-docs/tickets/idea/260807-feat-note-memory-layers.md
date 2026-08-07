---
title: Harness-agnostic PC-local note memory layers injected into workflow_manual
related:
  260523-bug-worktree-local-index-missing: resolves — the machine-local context (_index.local.md) lost across worktree switches is exactly what the non-tracked layers close
  260730-refactor-retire-goal-fan-out-step-and-session-note: constraint — this feature must NOT resurrect or build on the retiring session.note surface; it is a fresh note.* surface
  260806-feat-worktree-ticket-scope: prior-art — scopeAnnouncement is the workflow_manual injection pattern this `# Notes` section is modeled on
---

# Harness-agnostic PC-local note memory layers injected into workflow_manual

## Background

Session memory today relies on an agent *reading* `ai-docs/_index.md` (and the
gitignored `ai-docs/_index.local.md`) at session start. That follow-through is
behavioral, not mechanical, and it fails: this ticket was discussed in a session
where `_index.md` was never auto-injected into context, and `_index.local.md` is
routinely lost across worktree switches (`260523-bug-worktree-local-index-missing`).

The goal is a harness-agnostic, PC-local key-value note store implemented at the
MCP layer whose contents are **mechanically injected into `workflow_manual`
output**, so orientation memory no longer depends on an agent choosing to read a
file. `workflow_manual` is the right surface because it is host-neutral (MCP,
identical on every harness) and it fires exactly when a session engages the ws
workflow — the moment orientation is needed — unlike AGENTS.md auto-injection,
which is host-coupled and always resident regardless of relevance.

## Decisions

Confirmed in discussion (2026-08-07):

- **Two orthogonal axes** define the layers: (A) git-tracked? and (B)
  worktree-isolated?. The "git-tracked × worktree-isolated" cell is
  self-contradictory (anything tracked is shared by branch, not worktree-local),
  which is precisely why a single note store cannot serve both "work memory that
  follows the index" and "sensitive machine info that must never be committed" —
  the split resolves that contradiction.
- **v1 ships the two non-tracked layers only** (see Phase 1). They fully close
  `260523`.
- **Drop the `git-master` layer** (rejected). Writing to another branch's index
  from a worktree that is not on that branch is surprising, dangerous, and has no
  clean implementation; and its cell ("tracked + visible in all worktrees") is
  already served by merging the tracked layer to main plus the machine layer.
- **Non-tracked layers live outside the working tree** (cache / config home), so
  no `.gitignore` or `.git/info/exclude` entry is ever needed — there is nothing
  in the tree to exclude.
- **The injection is the feature, not the write verb.** For the eventual tracked
  layer, writing a file and committing it is something an agent can already do;
  the value MCP adds is the forced `workflow_manual` injection and the
  non-tracked layers an agent cannot write itself. Keep the surface minimal; do
  not add a git-mutation/commit tool to MCP (that runs counter to the
  `260605` pivot direction).
- **Search is the pruning escape valve.** All notes are force-injected, so the
  injection budget must be bounded: inject recent/up-to-cap items, elide the rest
  behind a visible "N older notes elided" line, and make them retrievable via
  `note.search`. This keeps aggressive pruning a soft requirement, not a hard one.
- **priority/category on a note is for injection ordering only** — deciding what
  stays injected versus elided-to-search. It is explicitly NOT a mechanism for
  reconstructing document sections; the moment "category" starts naming document
  sections it has become a worse re-implementation of a document.

## Prior Art

- `session.note` (MCP) — the existing note surface, scheduled for retirement in
  `260730` (zero callers, never shipped in a released version). Do not build on
  it; that ticket explicitly says a new notes feature must not resurrect it.
- Layered config scope (session / project / global): the `global` scope
  (`~/.ws/config.json`, `WS_CONFIG_HOME`) is the existing machine-local,
  project-agnostic, non-git store — the natural substrate for the `machine` layer.
- `wsstate.layoutFor` builds a `proj/<projectKey>@<worktreeKey>/` tree that is
  already worktree-scoped — the natural substrate for the `worktree` layer. (The
  session-key record store, by contrast, is a flat global store not partitioned
  by worktree, so it is not the right home for a worktree-isolated layer.)
- `scopeAnnouncement(root)` (`agents-plugin-tool/internal/mcp/scope_announcement.go`,
  injected in `workflow_manual.go` around the fresh and continue branches) is the
  established `computeX(root) string` → inject pattern to model the `# Notes`
  section on. A `# Notes` section renders near Session State (append), not as a
  top-of-body banner.

## Open Decisions

Recorded as OPEN — not settled; discussion continues:

- **Layer names.** `machine` / `worktree` (and later `repo`) are provisional.
  The names should capture both git-tracked-ness and scope legibly.
- **Verb names.** `note.write(layer, [[key, value], ...])` (full overwrite) /
  `note.erase(layer, [key, ...])` / `note.search(glob, from?, then?)` are
  provisional sketches.
- **priority/category exact shape** (integer priority, category tag, or both).
- **The `repo` layer and the `_index.md` decomposition agenda** (Phase 2) — under
  active discussion; likely spun into its own ticket. See Phase 2.

## Phases

### Phase 1: Non-tracked machine + worktree layers with workflow_manual injection

Confirmed implementable core. Deliver:

- Two non-tracked note layers stored outside the working tree:
  - `machine` — PC-global, project-agnostic, visible from every worktree
    (substrate: global config home). Home for IP / SSH host / hardware records.
  - `worktree` — worktree-local, ephemeral (vanishes when the worktree is left;
    substrate: the existing `proj/<key>@<worktreeKey>/` layout).
- A minimal note surface: full-overwrite write, key-list erase, and a
  glob/date-range search read.
- A `# Notes` section injected into `workflow_manual` output (both fresh-with-root
  and continue branches), modeled on the `scopeAnnouncement` injection, showing
  all layers' items up to a priority-ordered cap with a visible elision line for
  the remainder.

Closes `260523-bug-worktree-local-index-missing`: machine-local context stops
being lost across worktrees because it is injected, not file-read.

Must not resurrect `session.note`; this is a new `note.*` surface. Layer/verb
names above are provisional and to be settled before implementation.

### Phase 2: Tracked `repo` layer and `_index.md` decomposition (deferred, scope not final)

A git-tracked layer (`repo`) storing one key per file under `ai-docs/ws-notes/`,
committed through the normal agent path (single-key = single-file so merge
conflicts resolve on the filesystem, no merge tooling in MCP). Its justification
is not storage but **converting `_index.md` from a hand-edited monolith into a
modular, assembled artifact**: the volatile sections move into notes that
`workflow_manual` injects, the derivable sections (ticket/spec tables) are
generated (already tracked by `260710` / `260725` / `260728`), and the residual
non-volatile orientation content is redistributed (candidate: inline into
AGENTS.md for guaranteed injection vs. on-demand `ref/`). Whether this layer
lands at all, and the exact decomposition, are under active discussion and may
become a separate ticket. Do not implement Phase 2 from this body as written.
