---
title: Project-local untracked note layer (worktree-agnostic, per-project)
related:
  260807-feat-note-memory-layers: extends — adds the missing project-scoped untracked cell to that ticket's layer matrix
  260523-bug-worktree-local-index-missing: revisits — `_index.local.md` was project-scoped untracked, a scope neither shipped layer reproduces
  260619-feat-ws-layered-config-scope-substrate: prior-art — config's session/project/global scope is the 3-rung precedent and substrate
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-14
---

# Project-local untracked note layer (worktree-agnostic, per-project)

## Background

The note store ships three layers — `machine` (PC-global, project-agnostic,
untracked), `worktree` (this worktree only, untracked, ephemeral), and `repo`
(per-project, git-tracked). There is no **project-scoped, worktree-agnostic,
untracked** layer: a note that is local (never committed) but shared across all
worktrees of one project and isolated from other projects on the same machine.

This gap traces to the framing in `260807-feat-note-memory-layers`, which used
two binary axes — `git-tracked?` × `worktree-isolated?`. Collapsing scope to a
single "worktree-isolated?" bit filled the untracked/not-isolated cell with
`machine`, skipping the middle rung of the real three-rung scope ladder
(`worktree ⊂ project ⊂ machine`). The complete shape is a 2-axis matrix with a
3-value scope axis:

| scope \ persistence | untracked        | tracked                    |
| ------------------- | ---------------- | -------------------------- |
| worktree            | `worktree`       | (contradictory)            |
| project (clone)     | **`clone`** (new)| `repo`                     |
| machine             | `machine`        | —                          |

Beyond a missing convenience, this is a latent correctness gap in that ticket's
closure claim: it states the `machine`/`worktree` layers close the
`_index.local.md` loss from `260523-bug-worktree-local-index-missing`. But
`ai-docs/_index.local.md` was **project-scoped and untracked** — gitignored under
one project's tree. Neither shipped layer reproduces that scope: `machine` leaks
one project's local context into every other project on the PC, and `worktree`
is lost on the worktree switch that was the original bug. So the migration
target for `_index.local.md`-style content currently has no faithful home.

## Prior Art

- `260619-feat-ws-layered-config-scope-substrate` already models config in three
  scopes (session / project / global). The `project` scope is exactly the rung
  the note layers skipped.
- `wsstate.layoutFor` builds `proj/<projectKey>@<worktreeKey>/`; the
  `proj/<projectKey>/` prefix (worktree-agnostic, project-local, outside the
  working tree) is the natural substrate for the new layer, parallel to how the
  `worktree` layer uses `proj/<projectKey>@<worktreeKey>/`.

## Decisions

- **Layer name: `clone`** (settled). The three shipped layer names are all
  *substrate nouns*, not scope adjectives — `machine` (the PC), `worktree` (a git
  worktree), `repo` (the git repository) — and each layer's scope/persistence
  follows from its substrate. The new layer's substrate is the local *clone*: one
  machine's working copy of the repo, which contains one or more worktrees and
  holds untracked state shared across them. `clone` keeps the substrate-naming
  convention and yields a clean mental model — three untracked local substrates
  in a containment ladder (`worktree ⊂ clone ⊂ machine`) plus the one tracked,
  shared substrate (`repo`). `clone`/`repo` then read as the untracked-local-copy
  vs tracked-shared-history pair at project scope.
- Rejected `local` — collides with `machine` on the axis they share (both
  untracked); the name would not signal which scope it is. Rejected `project` —
  collides with `repo` on the axis they share (both per-project); the name would
  not signal tracked vs untracked. `clone` is more accurate than `project`
  besides: a `repo` note is shared across *all* clones, while the new layer is
  scoped to this one local copy.
- `repo` stays the tracked project layer; `clone` is its untracked sibling.

## Spec Impact

- Target: `mcp-tools.md`, `## Note Tools {#260810-note-tools}` and `### Note
  Injection {#260810-note-injection}`.
- Caller-visible change: the `layer` argument enum on `note.write` /
  `note.search` / `note.erase` / `note.mute` / `note.unmute` gains `"clone"`; the
  spec's layer enumeration, storage-mechanism description (a `clone` store under
  `proj/<projectKey>/`, one JSON file per layer like `machine`/`worktree`, not
  per-key like `repo`), root requirement, and the `# Notes` injection coverage
  all extend to the fourth layer.

## Phases

### Phase 1: `clone` note layer

Add the fourth note layer, `clone`: project-scoped, worktree-agnostic,
untracked, substrate `proj/<projectKey>/` (one JSON file per layer, like
`machine`/`worktree`). Extend the `layer` enum on every `note.*` tool, the
workflow_manual ambient `# Notes` injection, and the `mcp-tools.md` note-layer
spec.

Distinct sub-step (separately verifiable): re-point the `_index.local.md`
migration guidance (bootstrap / project-memory docs) at `clone` instead of
`machine`.

Verify:
- A `clone` note is visible from a sibling worktree of the same project, absent
  from a different project on the same machine, and never staged by git.
- A `clone` note surfaces in the ambient `# Notes` injection block alongside the
  other layers.
- The re-pointed `_index.local.md` migration guidance names `clone`, not
  `machine`.

### Result (baf8788) - 2026-08-14

Landed the `clone` layer as a fourth file-per-layer note store (one JSON file
for the whole layer, like `machine`/`worktree`, not per-key like `repo`). The
`layer` enum gained `"clone"` on all five `note.*` tool schemas, the
`noteLayerArg`/`resolveNoteStore` dispatch, the ambient `# Notes` injection
(`wsnote.Compute`), and the `mcp-tools.md` spec (`#260810-note-tools` /
`#260810-note-injection`, no anchor renamed). `_index.local.md` migration
guidance re-pointed from `machine` to `clone` in all five bootstrap /
project-memory docs (both packages' `AGENTS.template.md` + `WORKFLOW.md`, root
`AGENTS.md`); wsflow mirror text stayed non-ws-aware.

Deviation from the plan's `Codebase Findings`: `wsnote.ClonePath` resolves under
`Layout.SharedDir` (`proj/<projectKey>/shared/notes.json`), **not**
`Layout.ProjectDir` directly. For a project's canonical (non-linked) worktree
`wsstate.layoutFor` sets `WorktreeKey == ProjectKey`, so
`Layout.WorktreeDir == Layout.ProjectDir`; a store built directly on `ProjectDir`
would collide byte-for-byte with the `worktree` layer's `notes.json` in the
common single-worktree case, silently merging the two layers. `SharedDir` is the
pre-existing collision-avoidance subdirectory `LocksDir` already uses, stays
nested under `proj/<projectKey>/`, and keeps the project-scoped /
worktree-agnostic contract. Reasoning documented inline on `ClonePath` and
captured as a Common Mistakes invariant in
`ai-docs/mental-model/mcp-runtime.md` (`#260810-note-tools`). Both correctness
and fit reviewers independently confirmed the substitution in-contract.

Verification: `TestClonePathIsProjectScopedAndWorktreeAgnostic` and
`TestNoteCloneLayerIsProjectScopedAndWorktreeAgnostic` prove the three required
properties (identical across two worktrees of one repo, distinct from that
repo's `WorktreePath`, distinct across two unrelated repos);
`TestWorkflowManualCarriesCloneLayerNote` and the extended
`TestComputeSortsAndCapsAcrossAllFourLayers` cover ambient injection;
`TestNoteMuteUnmuteRoundTrip` now loops all four layers. Partitioned review
(correctness / fit / test) clean after one relay cycle. `go build ./...` clean.

Deferred (unrelated): two pre-existing `internal/mcp` failures
(`TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`,
`TestWorkflowManualNotesBlockAbsentWhenNoNotesExist`) predate this branch
(confirmed on base `5f1d720d`) and are untouched here; to be captured as a
separate idea ticket.
