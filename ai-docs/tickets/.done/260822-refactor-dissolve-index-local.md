---
title: "Dissolve _index.local.md — versioned bootstrap step splitting local memory into manuals/*.local.md plus the worktree/clone note layers"
related:
  260814-feat-note-project-local-untracked-layer: supersedes — that ticket shipped the `clone` substrate and re-pointed `_index.local.md` migration guidance at `clone` only, but authored no dissolution step and left the live read-steps in place; this ticket splits the target and finishes the retirement
  260807-refactor-dissolve-project-index: precedent — the parallel versioned migration that dissolved the tracked `_index.md`; this mirrors its shape for the untracked sibling
  260807-feat-manuals-doc-tier: prerequisite (landed) — the `*.local.md` machine-local procedures sink
  260807-feat-note-memory-layers: prerequisite (landed) — origin of the note memory layers the `clone` layer extends
  260523-bug-worktree-local-index-missing: origin — the machine-local context loss `_index.local.md` caused; retiring it into the auto-injected layers closes it for good
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-22
---

# Dissolve _index.local.md — versioned bootstrap step splitting local memory into manuals/*.local.md plus the worktree/clone note layers

## Background

The tracked `ai-docs/_index.md` was fully dissolved by epic `260807`: a versioned
`lead-bootstrap` migration moves each region to a purpose-specific home
(orientation → `AGENTS.md` body, session notes → `repo` note layer, procedures →
`manuals/`, inventories → generation), removes the read-`_index.md` step, and
deletes the file; fresh bootstrap never creates one. That behavior is specced at
`workflow-skills.md {#260812-bootstrap-index-dissolution}`.

Its untracked sibling `ai-docs/_index.local.md` — gitignored machine memory,
*intended* as clone-scoped (project-local, worktree-agnostic) but in practice
delivered at worktree resolution, since a gitignored working-tree file lives only
in the worktree that created it and never reaches siblings (the `260523` loss) —
was **not** retired in the same pass. Ticket
`260814-feat-note-project-local-untracked-layer` shipped a faithful home for that
*intended* clone scope (the `clone` note layer: project-scoped, worktree-agnostic,
untracked, ambient-injected via `# Notes`) and re-pointed the `_index.local.md`
*migration pointer* from `machine` to `clone`, but it authored no versioned
dissolution step and removed no read-step. As a result
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

- **Content-shaped split across three homes, not the monolithic `clone` target
  `260814` recorded.** `_index.local.md` content is heterogeneous and splits by
  shape and scope, mirroring how the tracked `_index.md` dissolution routed
  procedures to `manuals/` and volatile state to notes:
  - **Machine-local procedures** (credentials, IPs, hostnames, host-specific
    runbooks / access methods) → a gitignored `ai-docs/manuals/*.local.md`
    sibling. This is already the established local/tracked split of the manuals
    tier (`documentation-system.md {#260807-manuals-document-system}`). The exact
    `*.local.md` filename(s) — one file or split by concern — are a migration-time
    autonomous choice following the content grouping, the same kind of call the
    v0046 procedure migration already makes; the ticket fixes the sink, not the
    filename.
  - **Volatile local context / session notes** → a note layer chosen by scope,
    not one fixed layer: `ws/note.write(layer: "worktree")` by default,
    `layer: "clone"` when the content is genuinely shared across all worktrees of
    the local clone. Both surface in the ambient `# Notes` block.
- **`worktree` is the default resolution; `clone` is the promotion.** A downstream
  repo that has been keeping local context in `ai-docs/_index.local.md` was
  de-facto operating at worktree resolution — the gitignored working-tree file
  lives only in the worktree that created it and never reaches siblings (the
  `260523` loss). So the behavior-preserving default for un-triaged migrated
  content is the `worktree` layer; the migrator promotes an item to `clone` only
  when it judges the content clone-wide by intent. This is a per-item
  migration-time judgment, like the procedures-vs-context split above.
- This **supersedes** `260814`'s re-point of the migration pointer to a single
  `clone` target. `260814` correctly gave `_index.local.md`'s *intended* clone
  scope a faithful home (`clone`) but treated the whole file as clone-wide; in
  practice its content spans worktree- and clone-scoped context plus procedures.
  Rejected the monolithic `clone` sink on two counts: it would (a) dump
  procedure-shaped content into the note store — the category error the tracked
  `_index.md` dissolution avoided by giving procedures their own `manuals/` home —
  and (b) silently promote worktree-local context to clone-wide visibility.
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
  volatile local context into the `worktree` note layer by default (or `clone`
  when the content is clone-wide) — removes the read-`_index.local.md` step, and
  deletes the file. Fresh bootstrap never creates `_index.local.md`.
  Upgrade-migrated and fresh-bootstrapped projects converge on the same shape,
  neither carrying `_index.local.md`. The step is gated on the file existing
  (transitional coexistence), like the `_index.md` step.
- **`documentation-system.md {#260505-project-memory-index}`**: make the
  local/untracked project-memory home explicit — machine-local context lives in
  the `worktree`/`clone` note layers (worktree resolution by default, clone when
  shared across worktrees) and `ai-docs/manuals/*.local.md` (machine-local
  procedures); no standalone `_index.local.md`. Currently this anchor is silent on
  the untracked local variant.

## Phases

### Phase 1: Author the `_index.local.md` dissolution step and remove the live read-steps

Author the versioned `lead-bootstrap` dissolution step (next version after v0046)
in `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`, symmetric with the
v0046 `_index.md` step: migrate an existing `_index.local.md` into its homes
(machine-local procedures → `manuals/*.local.md`; volatile context → `worktree`
notes by default, `clone` notes when clone-wide), remove the Project Memory
read-`_index.local.md` step and the layout-tree entry, and stop the
fresh-bootstrap scaffold from creating it. Retire the live
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
  `worktree` notes by default (`clone` for clone-wide items), the read-step is
  gone, and the file is deleted; the step no-ops cleanly when the file is absent.
- Corpus grep across `agents-plugin/`, `agents-plugin-wsflow/`, and root
  `AGENTS.md` finds no live `_index.local.md` read/description instruction (only
  version-history log lines remain).
- `ws/spec_index.verify` ok; both spec anchors reflect the retirement.
- wsflow mirror parity holds per `wsflow-mirroring.md`; package tests
  (runtime-contract + skill-shim drift) pass.
- This repo's root `AGENTS.md` no longer carries the live `_index.local.md`
  read-step.

### Result (7104f09) - 2026-08-22

Landed the versioned `_index.local.md` dissolution, symmetric with the v0046
`_index.md` step. Retired the live read-step and layout-tree entry from both
`AGENTS.template.md` files — ws v0046→v0047, wsflow v0007→v0008 (independent
lineages per `wsflow-mirroring.md ## Bootstrap Template Rules`, not forced to
align) — each gaining a migration-checklist item that, on upgrade, splits an
existing file into machine-local `ai-docs/manuals/*.local.md` (procedures) and
the `worktree`/`clone` note layers (volatile context; `worktree` default,
`clone` only when clone-wide), then removes the read-step, drops the layout-tree
entry, deletes the file, and never creates it on fresh bootstrap. Both
`WORKFLOW.md` copies' layout line re-pointed at the new homes;
`reference-discovery.md` step 0 dropped the `_index.local.md` clause (kept the
`_index.md` fallback); the wsflow rsrc mirror and both skills manifests were
regenerated via `go test`, never hand-edited. Added spec anchors
`{#260822-bootstrap-index-local-dissolution}` (`workflow-skills.md`) and the
local/untracked project-memory paragraph (`documentation-system.md
{#260505-project-memory-index}`).

Validated on devenv: migrated the live `_index.local.md` (SSH
host/port-forward/rustup — pure machine-local procedure) into
`ai-docs/manuals/local-machine-context.local.md`, deleted the source, removed
and renumbered root `AGENTS.md`'s Project Memory read-step, bumped its Template
Version to v0047, and re-synced `ai-docs/WORKFLOW.md` from the rewritten master.

Deviations (both necessary consequences of the version bump, not scope changes):
the wsflow test `test_bootstrap_template_uses_wsflow_local_version_lineage`
fixture was updated v0007→v0008, and `agents-plugin/skills/manifest.json` was
regenerated for the rsrc-drift contract; neither was enumerated in the plan's
Codebase Findings but both were required for the plan's own verification
commands to pass.

Verification: corpus grep across `agents-plugin/`, `agents-plugin-wsflow/`, and
root `AGENTS.md` finds no live read/description instruction (only the historical
v0014 line plus the new v0047/v0008 migration items); `_index.local.md` deleted
and the manual present; `ws/spec_index.verify` ok; wsflow rsrc mirror regen
yields no diff and `TestWsflowRsrcMirrorUpToDate` passes;
`agents-plugin-wsflow/tests` 10/10; `agents-plugin-tool` `go test ./...` clean;
full-scope review returned clean.
