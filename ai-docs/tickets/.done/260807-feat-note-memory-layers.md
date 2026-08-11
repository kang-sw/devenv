---
title: Harness-agnostic PC-local note memory layers injected into workflow_manual
parent: 260807-epic-mechanical-project-memory
related:
  260523-bug-worktree-local-index-missing: resolves — the machine-local context (_index.local.md) lost across worktree switches is exactly what the non-tracked layers close
  260730-refactor-retire-goal-fan-out-step-and-session-note: constraint — this feature must NOT resurrect or build on the retiring session.note surface; it is a fresh note.* surface
  260806-feat-worktree-ticket-scope: prior-art — scopeAnnouncement is the workflow_manual injection pattern this `# Notes` section is modeled on
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-10
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
  injection budget must be bounded: inject the highest-priority items up to a cap,
  elide the rest behind a visible "N lower-priority notes elided" line, and make
  them retrievable via `note.search`. This keeps aggressive pruning a soft
  requirement, not a hard one.
- **`priority` is a single integer, for injection ordering only** — deciding what
  stays injected versus elided-to-search. **Higher integer = higher priority**:
  highest-priority records fill the injection cap first, lower-priority ones are
  elided to search. A full-overwrite `note.write` carries the key's `priority` and
  updates it on every write (a key's priority is part of the record, re-set each
  time the key is written). There is no `category` field: the moment a tag starts
  naming document sections it has become a worse re-implementation of a document.
- **The stored record is `[key, value, priority, written_at]`.** Each write also
  records a last-write timestamp — full-overwrite writes make this the single
  meaningful time — which backs both the date shown per note in the injected
  block and the `from` / `then` date-range filter of `note.search`.

## Confirmed names (2026-08-07)

Settled so the Spec Impact below is concrete; no longer open.

- **Layers (Phase 1): `machine` / `worktree`.** They read as scope labels and
  both are non-tracked, which is the only tracked-ness this feature ships. A
  later tracked layer is `repo`, but it is epic-owned and out of this ticket.
- **Verbs: `note.write(layer, [[key, value, priority], ...])` (full overwrite,
  updates priority) / `note.erase(layer, [key, ...])` / `note.search(glob, from?,
  then?)`.** Minimal surface: no git-mutation/commit verb (counter to the
  `260605` pivot), read is by forced injection plus `search` as the pruning
  escape valve.

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

## Spec Impact

No existing stem covers a `note.*` MCP surface or note injection into
`workflow_manual`, so this ticket addresses spec at ready time here.

- **`spec/mcp-tools.md`** (host-neutral MCP tool contracts): add the `note.*`
  tool family — `note.write` / `note.erase` / `note.search` — with the layer
  argument (`machine` / `worktree`), the full-overwrite semantics, the
  `[key, value, priority, written_at]` record shape, and the glob/date-range
  search contract.
  Caller-visible change: a new MCP tool family agents call to persist and prune
  PC-local orientation notes.
- **`spec/mcp-tools.md`** `workflow_manual` section: document the injected
  `# Notes` block (rendered near Session State on both the fresh-with-root and
  continue branches), its priority-ordered cap, and the visible
  "N lower-priority notes elided" line. Caller-visible change: `workflow_manual`
  output
  gains a Notes section carrying the two layers' records.

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

Verification: a `note.write` → `note.search` round trip returns the written
record on each layer; a `workflow_manual` call after a write renders the `# Notes`
section carrying that record; a `worktree`-layer note is absent from a different
worktree's injection while a `machine`-layer note is present in both.

Closes `260523-bug-worktree-local-index-missing`: machine-local context stops
being lost across worktrees because it is injected, not file-read.

Must not resurrect `session.note`; this is a new `note.*` surface. Layer and verb
names are settled (see **Confirmed names**).

The tracked `repo` note layer (one key per file under `ai-docs/ws-notes/`,
committed through the normal agent path) is **out of this ticket** — it is an
epic-deferred concern owned by `260807-epic-mechanical-project-memory`, spun into
its own future ticket if and when the `_index.md` decomposition needs a tracked
substrate. This ticket ships only the two non-tracked layers.

### Result (41475bdd) - 2026-08-10

Shipped the two non-tracked note-memory layers and their ambient
`workflow_manual` injection. Behavioral delta:

- New `internal/wsnote/` package (peer to `wsconfig`/`wsstate`, not folded into
  the git-tracked-doc `wsdoc`): `machine` layer at `~/.ws/notes.json` (via a
  `wsconfig.GlobalPath` sibling — PC-global, project-agnostic) and `worktree`
  layer at `layout.WorktreeDir/notes.json` (via `wsstate.NewManager(...).Ensure`
  — the existing `proj/<projectKey>@<worktreeKey>/` tree, so it is genuinely
  worktree-isolated). Record shape `[key, value, priority, written_at]`;
  full-overwrite `Write`, key-list `Erase`, and glob + date-range `Search`, all
  reusing the `wsconfig` flock + temp-file + atomic-rename RMW pattern (own
  `.lock` sibling per store, no shared lock).
- Three `note.write` / `note.erase` / `note.search` MCP tools (`note_tools.go`)
  reachable by any scope holding a session key (mirroring `todo.*`/`agenda.*`,
  not lead-only). The `machine` layer resolves without a repo root but still
  rejects empty/unknown session keys; `worktree` routes through
  `resolveToolRoot`. No CLI mirror (session-keyed tools have none by precedent);
  no git-mutation/commit verb (honors the `260605` pivot constraint).
- A `# Notes` block injected into `workflow_manual` on BOTH the fresh-with-root
  and continue branches, appended immediately after Session State via plain
  concat — deliberately NOT routed through `injectBootstrapStalenessWarning`,
  which prepends a top banner; only the pure `computeX` shape of
  `scopeAnnouncement`/`computeManuals` was mirrored, not their injection site.
  Highest-priority items fill a `notesInjectionCap = 20` budget; the remainder
  is elided behind a visible "N lower-priority notes elided" line and stays
  retrievable via `note.search`. Silent (no heading, no stray blank) when no
  notes exist.

Wire-shape decision (lead-adjudicated): `note.write` takes named JSON objects
`{key, value, priority}`, not the ticket's illustrative positional
`[[key,value,priority],...]` tuples, to match the codebase-wide MCP tool-arg
convention (zero positional-array precedent); the `[key,value,priority,written_at]`
record semantics are preserved and documented explicitly in the spec.

Spec: new `## Note Tools {#260810-note-tools}` and `### Note Injection
{#260810-note-injection}` in `spec/mcp-tools.md`; a `mental-model/mcp-runtime.md`
bullet records the non-obvious prepend-vs-append injection-placement divergence so
a future author copying the compute-inject pattern does not default to the wrong
wiring.

Verification: `go build ./... && go vet ./... && go test ./... -count=1` all green
across 13 packages. Tests cover per-layer write→search→erase round trips,
full-overwrite priority + `written_at` re-stamping (via an injectable clock so the
stale-echo regression is deterministically caught), cross-worktree isolation
proven at worktree granularity (`git worktree add` on one repo, not two unrelated
repos), the elision cap + count + search retrieval, empty-state silence and
after-Session-State positioning on both injection branches, the inclusive
whole-day `then` search bound, match-all glob across slash-bearing keys, and
empty/unknown session-key rejection.

Review: partitioned correctness / fit / test. Fit clean first pass. Correctness
found one Critical (`note.search` `then` date-prefix upper bound excluded the
target day) — fixed. Test found three Important gaps (cross-worktree test proved
only project granularity; `written_at` re-stamp and empty-session-key branches
untested) — all fixed; a follow-up re-review caught that the `written_at` test was
not load-bearing under second-granularity timestamps, fixed with an injectable
clock. Clean after the budgeted cycles; one accepted Minor remains (a newline in a
note `value` would break the injected one-line format — agent-supplied, cosmetic).

Closes `260523-bug-worktree-local-index-missing`: the `machine` layer lives
outside the working tree and is injected every session, so machine-local context
(SSH hosts, IP records) is no longer lost across worktree switches — dissolving
that ticket's "safely copy ignored files into worktrees" problem entirely rather
than solving it. The tracked `repo` layer remains epic-deferred.
