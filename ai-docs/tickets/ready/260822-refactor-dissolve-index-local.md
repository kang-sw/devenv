---
title: "Dissolve _index.local.md — versioned bootstrap step splitting clone-scoped local memory into manuals/*.local.md and the clone note layer"
related:
  260814-feat-note-project-local-untracked-layer: supersedes — that ticket shipped the `clone` substrate and re-pointed `_index.local.md` migration guidance at `clone` only, but authored no dissolution step and left the live read-steps in place; this ticket splits the target and finishes the retirement
  260807-refactor-dissolve-project-index: precedent — the parallel versioned migration that dissolved the tracked `_index.md`; this mirrors its shape for the untracked sibling
  260807-feat-manuals-doc-tier: prerequisite (landed) — the `*.local.md` machine-local procedures sink
  260807-feat-note-memory-layers: prerequisite (landed) — origin of the note memory layers the `clone` layer extends
  260523-bug-worktree-local-index-missing: origin — the machine-local context loss `_index.local.md` caused; retiring it into the auto-injected layers closes it for good
sage-review-design: completed
sage-review-completeness: completed
---

# Dissolve _index.local.md — versioned bootstrap step splitting clone-scoped local memory into manuals/*.local.md and the clone note layer

## Background

The tracked `ai-docs/_index.md` was fully dissolved by epic `260807`: a versioned
`lead-bootstrap` migration moves each region to a purpose-specific home
(orientation → `AGENTS.md` body, session notes → `repo` note layer, procedures →
`manuals/`, inventories → generation), removes the read-`_index.md` step, and
deletes the file; fresh bootstrap never creates one. That behavior is specced at
`workflow-skills.md {#260812-bootstrap-index-dissolution}`.

Its untracked sibling `ai-docs/_index.local.md` — gitignored, clone-scoped
(project-local, worktree-agnostic) machine memory — was **not** retired in the
same pass. Ticket `260814-feat-note-project-local-untracked-layer` shipped the
faithful replacement substrate (the `clone` note layer: project-scoped,
worktree-agnostic, untracked, ambient-injected via `# Notes`) and re-pointed the
`_index.local.md` *migration pointer* from `machine` to `clone`, but it authored
no versioned dissolution step and removed no read-step. As a result
`_index.local.md` is still a **live** instruction across the plugin surface,
asymmetric with how `_index.md` was retired:

- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` — Project Memory step 2
  ("read `ai-docs/_index.local.md` if present") and the layout-tree entry; the
  version-history list stops at v0046 (the `_index.md` dissolution) with no step
  for the local sibling.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` — the clone-scoped-memory
  description line.
- `agents-plugin/rsrc/reference-discovery/reference-discovery.md` — step 0 reads
  `ai-docs/_index.local.md` if present.
- wsflow mirrors of all of the above under `agents-plugin-wsflow/`.
- This repo's own root `AGENTS.md` — the same live Project Memory read-step.

This is a gap-closing retirement, not new capability: substrate and gitignore
coverage already exist (`.gitignore` already covers `ai-docs/**/*.local.md` per
`workflow-skills.md {#260508-bootstrap-api-deps-gitignore}`; the `*.local.md`
manuals convention is live per `documentation-system.md
{#260807-manuals-document-system}`). Only the dissolution step and the read-step
removals are missing.

## Decisions

- **Two-sink split, not the monolithic `clone` target `260814` recorded.**
  `_index.local.md` content is heterogeneous and splits by shape, mirroring how
  the tracked `_index.md` dissolution routed procedures to `manuals/` and volatile
  state to notes:
  - **Machine-local procedures** (credentials, IPs, hostnames, host-specific
    runbooks / access methods) → a gitignored `ai-docs/manuals/*.local.md`
    sibling. This is already the established local/tracked split of the manuals
    tier (`documentation-system.md {#260807-manuals-document-system}`). The exact
    `*.local.md` filename(s) — one file or split by concern — are a migration-time
    autonomous choice following the content grouping, the same kind of call the
    v0046 procedure migration already makes; the ticket fixes the sink, not the
    filename.
  - **Volatile local context / session notes** → the `clone` note layer via
    `ws/note.write(layer: "clone")`, surfaced by the ambient `# Notes` block.
- This **supersedes** `260814`'s re-point of the migration pointer to a single
  `clone` target. Rejected the monolithic `clone` sink: it would dump
  procedure-shaped local content into the note store — the same category error the
  tracked `_index.md` dissolution deliberately avoided by giving procedures their
  own `manuals/` home. `clone` is the correct home only for the note-shaped half.
- **Symmetry with the `_index.md` dissolution is the design target.** The step
  gets the same shape as v0046: on upgrade, migrate an existing `_index.local.md`
  into the two sinks, remove the read-`_index.local.md` step, and delete the file;
  on fresh bootstrap, never create it; both paths converge on the same
  no-`_index.local.md` shape. Weak counter-argument considered and rejected —
  "keep the plain-file read-step for MCP-degraded contexts": the whole local-memory
  model already depends on ambient note injection, and `_index.md` was dissolved
  under the identical consideration; preserving the asymmetry has no basis.

## Prior Art

- `260807-refactor-dissolve-project-index` — the versioned dissolution step +
  devenv validation shape to mirror. That ticket authored the v0046 migration and
  validated it on this repo; this ticket is its untracked-sibling counterpart with
  the substrate already in place.
- `documentation-system.md {#260807-manuals-document-system}` (the `*.local.md`
  local/tracked split) and `mcp-tools.md` note-layer spec (the `clone` layer,
  ambient `# Notes` injection) — the two landed sinks.

## Spec Impact

- **`workflow-skills.md`, adjacent to `{#260812-bootstrap-index-dissolution}`**
  (new anchor `{#260822-bootstrap-index-local-dissolution}` recommended, parallel
  to the tracked-index anchor): a versioned migration retires
  `ai-docs/_index.local.md`. On upgrade `lead-bootstrap` migrates its regions —
  machine-local procedures into a gitignored `ai-docs/manuals/*.local.md` sibling,
  volatile local context into the `clone` note layer — removes the
  read-`_index.local.md` step, and deletes the file. Fresh bootstrap never creates
  `_index.local.md`. Upgrade-migrated and fresh-bootstrapped projects converge on
  the same shape, neither carrying `_index.local.md`. The step is gated on the file
  existing (transitional coexistence), like the `_index.md` step.
- **`documentation-system.md {#260505-project-memory-index}`**: make the
  local/untracked project-memory home explicit — clone-scoped machine memory lives
  in the `clone` note layer (volatile context) and `ai-docs/manuals/*.local.md`
  (machine-local procedures); no standalone `_index.local.md`. Currently this
  anchor is silent on the untracked local variant.

## Phases

### Phase 1: Author the `_index.local.md` dissolution step and remove the live read-steps

Author the versioned `lead-bootstrap` dissolution step (next version after v0046)
in `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`, symmetric with the
v0046 `_index.md` step: migrate an existing `_index.local.md` into the two sinks
(machine-local procedures → `manuals/*.local.md`; volatile context → `clone`
notes), remove the Project Memory read-`_index.local.md` step and the layout-tree
entry, and stop the fresh-bootstrap scaffold from creating it. Retire the live
read-step / description in `WORKFLOW.md` and in
`rsrc/reference-discovery/reference-discovery.md` (its `_index.md`
"if not migrated" fallback phrasing may stay; the `_index.local.md` clause goes).
Mirror every change into the `agents-plugin-wsflow/` package per
`ai-docs/manuals/wsflow-mirroring.md` (read it before touching the mirror). Update
the two spec anchors in `## Spec Impact`. Validate on this repo (devenv): migrate
any local content and remove the live read-step from root `AGENTS.md`.

Version-history log entries that *name* `_index.local.md` as part of a past
migration (e.g. v0014 adding it to `.gitignore`) are historical records and stay;
only *live* read/description instructions are retired.

Verify:
- Fresh bootstrap output contains no `_index.local.md` and no instruction to read
  one.
- Upgrade migration against a project holding a live `_index.local.md`: its
  procedure content lands in `ai-docs/manuals/*.local.md`, its volatile context in
  `clone` notes, the read-step is gone, and the file is deleted; the step no-ops
  cleanly when the file is absent.
- Corpus grep across `agents-plugin/`, `agents-plugin-wsflow/`, and root
  `AGENTS.md` finds no live `_index.local.md` read/description instruction (only
  version-history log lines remain).
- `ws/spec_index.verify` ok; both spec anchors reflect the retirement.
- wsflow mirror parity holds per `wsflow-mirroring.md`; package tests
  (runtime-contract + skill-shim drift) pass.
- This repo's root `AGENTS.md` no longer carries the live `_index.local.md`
  read-step.
