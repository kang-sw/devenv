---
title: Dissolve _index.md — author the versioned lead-bootstrap dissolution step, validated on devenv
sage-review-design: completed
parent: 260807-epic-mechanical-project-memory
related:
  260807-feat-note-memory-layers: prerequisite (landed) — the volatile sink; _index.md's non-tracked session state moves to the note layers
  260810-feat-repo-tracked-note-layer: prerequisite — the tracked `repo` layer, landed home for the git-tracked `# Session Notes` (Resolved Decision chose to keep them tracked)
  260807-feat-manuals-doc-tier: prerequisite (landed) — the procedure sink; _index.md's inlined procedures and Read-Before-Editing table move to manuals/
  260710-bug-project-index-ticket-focus-stale-status: prerequisite — the derivable->generate leg; Ticket Focus / status content must become generated before it can be removed from _index.md
  260725-idea-retire-ticket-focus-root-regen: absorbs (dropped) — its devenv AGENTS.md/WORKFLOW.md Ticket Focus regen cleanup is folded into this ticket's Phase 1 validate-on-devenv step; the unique section-placement gotcha was transplanted there
  260728-research-index-ticket-table-drift: motivates — documents the hand-maintained _index ticket/spec table drift this dissolution removes by generation
sage-review-completeness: completed
---

# Dissolve _index.md — author the versioned lead-bootstrap dissolution step, validated on devenv

## Background

`ai-docs/_index.md` is a hand-edited monolith the workflow expects an agent to
read at session start. That expectation is behavioral and unreliable — sessions
routinely run without it in context — which is the founding problem of the
`260807-epic-mechanical-project-memory` epic.

The deliverable is **not** a one-off edit of devenv's own `_index.md`. Both faces
of the `_index.md` dependency are template-managed and version-gated inside
`lead-bootstrap`'s `AGENTS.template.md`:

1. the session-start "read `ai-docs/_index.md`" step (its `## Project Memory`
   block), stamped into every downstream `AGENTS.md`; and
2. the fresh-bootstrap scaffold that instructs a *new* project to **create**
   `_index.md` as its memory store.

So dissolving `_index.md` is a **`lead-bootstrap` change** — downstream-applicable
to every project that bootstraps — not a local file deletion. devenv's own
`_index.md` dissolution is the **first validation** of that general instruction,
not the whole of it.

It plugs into machinery that already exists rather than building new mechanism:
`AGENTS.template.md` carries an ordered, version-gated migration checklist marked
by a `<!-- Template Version: vNNNN -->` tag; `lead-bootstrap`'s upgrade handler
walks checklist items whose version exceeds a project's installed tag; and the
runtime bootstrap-staleness alarm nudges a stale project to re-bootstrap. The
dissolution is therefore a **new migration-checklist item plus the template and
procedure rewrites it implies**, not new version-gating or alarm infrastructure.
devenv is a real target here: its own `AGENTS.md` tag trails the shipped
template, so running the upgrade on devenv exercises the genuine downstream path.

## Prerequisites — all satisfied (2026-08-12)

This ticket could not land until every content destination existed. All are now
in place:

- `260807-feat-note-memory-layers` (Phase 1) — the note layers, destination for
  volatile / non-tracked session state. Landed in ws 0.40.0.
- `260807-feat-manuals-doc-tier` — the `manuals/` tier, destination for
  procedures; the `# Manuals` ambient index replaces the hand-maintained
  `## Read Before Editing` applicability table. Landed in ws 0.40.0.
- `260810-feat-repo-tracked-note-layer` — the tracked `repo` layer, the landed
  home for the git-tracked `# Session Notes` (per the Resolved Decision).
  **Landed** (`.done`; live at ws 0.40.2-dev).
- The derivable->generate leg — `260710` **landed** (`.done`): it *removed* the
  `_index.md` Ticket Focus section and retired its maintenance machinery, so
  there is no Ticket Focus table left to generate. `260725` **dropped** (absorbed
  into this ticket's Phase 1). The ticket/spec inventory tables `project_tree`
  already emits. `260728-research-index-ticket-table-drift` (`idea/`) `motivates`
  this dissolution but is not a build prerequisite.

## Decisions

### Dissolution target — where each `_index.md` region goes

Each `_index.md` region redistributes to its correct home (the epic's
`dissolution-target` cross-child decision):

- **Volatile content** (Session Notes and any per-session state) -> the **note
  layers**, injected not file-read. The **tracked** `# Session Notes` subset goes
  specifically to the tracked `repo` layer (`260810`) to preserve cross-clone
  persistence — see the Resolved Decision; genuinely machine-local or ephemeral
  state goes to `machine` / `worktree`.
- **Every-session orientation** (repo identity, plugin topology, canonical flows,
  and the documentation-system routing that names which doc tier lives where) ->
  **`AGENTS.md`** body, which the harness auto-injects every session. Only stable,
  always-resident orientation belongs here; nothing needing periodic
  hand-maintenance.
- **Procedures** (inlined operating procedures and the per-document `## Read
  Before Editing` applicability table) -> **`manuals/`**, ambient-injected. The
  hand-maintained applicability table is *replaced* by the generated `# Manuals`
  ambient index, which is what retires its drift.
- **Derivable tables** (ticket inventory, spec inventory, status/focus) ->
  **generated** output (`project_tree` and the derivable->generate leg). Paths
  only; description-bearing prose handled below.

**Description-bearing inventory / notes regions** (`## Runtime Surfaces`,
`## MCP Runtime Notes`, `## Prompt And Agent Inventory`, `## Skill Inventory`,
`## Current Branch Rules`) are almost entirely pointer prose or duplicates of
`AGENTS.md`/specs, so none needs a new generator. Per-region disposition
(exhaustive; no region silently falls to "generated"):

- `## Runtime Surfaces` — pointer prose to `spec/mcp-tools.md` /
  `spec/plugin-runtime.md` plus "schemas are runtime-discoverable, don't copy
  them." **Fold the non-duplicate line into AGENTS.md orientation, drop the rest
  as duplicate.**
- `## MCP Runtime Notes` — runbook pointer (already in AGENTS.md) plus concrete
  Windows launcher startup steps. **The startup steps are an operating procedure
  -> `manuals/`**; the pointer lines drop as duplicate.
- `## Prompt And Agent Inventory` / `## Skill Inventory` — pointer prose to the
  prompt-bundle mental model, `runtime.json`, and the `agents-plugin/skills/`
  tree. **Drop as source-derivable**, keeping at most a one-line "inventory is
  discoverable from the source tree and manifest" pointer in AGENTS.md.
- `## Current Branch Rules` — the `.codex`-untracked rule is already in AGENTS.md
  (Commit Rules) -> drop as duplicate; the "verify branch with `git status`" line
  is moot once the file is gone; the "no active freeze" line is volatile default
  state -> drop.

### The template has two faces; fresh and upgrade paths converge

`AGENTS.template.md` both stamps the session-start read step **and** scaffolds
fresh-project creation of `_index.md`. Dissolution must rewrite **both**:

- **Read step** (`## Project Memory`): point at the new homes (notes / manuals /
  generated / AGENTS.md), not at reading `_index.md`.
- **Fresh-bootstrap scaffold**: stop creating `_index.md`; the always-resident
  orientation is carried in the **AGENTS.md template body itself** (repo identity,
  project map / plugin topology, canonical flows, doc-system routing).

**Convergence invariant.** A fresh-bootstrapped project and an upgrade-migrated
project must reach the **same `AGENTS.md` shape**, neither carrying an `_index.md`.
The fresh scaffold and the upgrade migration item are two routes to one end state.

### Delivery rides the existing version-gate, not new machinery

The dissolution is authored as a **new migration-checklist item** in
`AGENTS.template.md` (the next `vNNNN`), picked up automatically by
`lead-bootstrap`'s existing upgrade handler and surfaced by the existing runtime
staleness alarm. No new version-gating, upgrade-walk, or alarm mechanism is built.
The item directs the project's lead agent to migrate resident orientation into
`AGENTS.md`, move Session Notes to the `repo` note layer with qualitative
staleness pruning, and delete the file (see Resolved Decision).

## Resolved Decision

**Tracked Session Notes sink — settled (2026-08-10): require the tracked `repo`
layer.** `_index.md`'s Session Notes are git-tracked, cross-clone content
(commit-hash closeouts, dogfood findings). Demoting them to non-tracked layers
would silently drop cross-clone persistence, so tracked Session Notes migrate into
the `repo` layer (`260810`), now a hard prerequisite (see **Blocked on**).
Rejected: accept non-tracked (closeouts to `git log --grep`) — judged a real loss,
not acceptable pruning.

**Migration is a one-time `lead-bootstrap` step with qualitative staleness
pruning, not an automated mechanism.** The migration-checklist item directs the
lead agent to read each note and **qualitatively judge staleness** — migrate a
live note, drop a stale one. No staleness threshold, no reconciliation mechanism,
no note-tool feature. The always-injected `# Notes` date display (owned by
`260807-feat-note-memory-layers`) is a separate concern, not this step's staleness
answer.

## Spec Impact

Recorded now because deleting `_index.md` and re-guiding the template contradict
live spec entries, even though this ticket lands at `todo/` (spec addressing is
gated at `ready/`):

- **`spec/documentation-system.md`** `## Project Memory Index
  {#260505-project-memory-index}` currently declares `ai-docs/_index.md` "the
  project memory and active inventory document." Rewrite it to describe the
  dissolved model (injected notes + `manuals/` + AGENTS.md orientation + generated
  inventory) and the un-migrated coexistence state as a supported transitional
  configuration. The sibling `260807-feat-manuals-doc-tier` amends a different
  section of this spec; the edits are additive.
- **`spec/workflow-skills.md`** bootstrap section currently documents
  `lead-bootstrap`'s `_index.md` health-check as advisory-only and names no
  dissolution / note / manuals wiring beyond the landed manuals routing row.
  Amend it to describe the versioned dissolution migration item, the rewritten
  fresh-bootstrap scaffold (no `_index.md` creation), and the fresh/upgrade
  convergence invariant.
- **`AGENTS.template.md`** is the template surface both edits above manifest in
  (read step, fresh scaffold, and the new migration-checklist item); it is a
  shipped downstream-affecting artifact, so its rewrite is the caller-visible
  behavior these spec entries must cover.

## Phases

### Phase 1: Author the versioned lead-bootstrap dissolution step, validated on devenv

The active, downstream-applicable path. Author the dissolution as a
`lead-bootstrap` change and prove it by running it on devenv itself.

- Add a new migration-checklist item (next `vNNNN`) to `AGENTS.template.md`: for
  an upgrading project, migrate `_index.md`'s resident orientation into its
  `AGENTS.md`, move `# Session Notes` -> `repo` note layer (qualitative pruning),
  procedures -> `manuals/`, derivable tables -> generated, remove the
  read-`_index.md` step, and delete the file.
- Rewrite `AGENTS.template.md`'s two faces: the `## Project Memory` read step
  points at the new homes; the fresh-bootstrap scaffold no longer creates
  `_index.md`, and the always-resident orientation is carried in the `AGENTS.md`
  template body.
- Update `lead-bootstrap.md`: `On: fresh` no longer creates `_index.md`; the
  index-health-check routing reflects the dissolved model.
- Apply every template and procedure edit to **both** shipped `lead-bootstrap`
  copies — `agents-plugin/` and `agents-plugin-wsflow/` (`AGENTS.template.md` and
  `lead-bootstrap.md` are dual-maintained mirrors). Editing one copy alone leaves
  wsflow-bootstrapped projects still reading `_index.md` and diverges the two
  distributions.
- Reconcile specs per **Spec Impact** (`documentation-system.md`,
  `workflow-skills.md`).
- Hold the **convergence invariant**: fresh-bootstrap and upgrade-migration reach
  the same `AGENTS.md` shape, neither with an `_index.md`.
- Validate on devenv: run the upgrade item against this repo — its `_index.md`
  dissolves, its `AGENTS.md` absorbs the resident orientation, its tracked
  `# Session Notes` migrate into the `repo` layer with the stale closeout pruned.
- Absorbed from `260725` (residual Ticket Focus regen): the same devenv
  bootstrap-regeneration must also clear the retired Ticket Focus references
  `260710` left in this repo's *managed* consumer files — root `AGENTS.md`
  (the `Check '## Ticket Focus' …` reader instruction) and `ai-docs/WORKFLOW.md`
  (Ticket Focus semantics / keep-list / routing mentions). These must be cleared
  by regeneration, never a hand-edit (a hand-edit is re-added on the next upgrade).
  Section-placement gotcha: the migration entry's section hint points the reader
  bullet at `## Project Knowledge`, but in devenv's own generated `AGENTS.md` the
  equivalent line sits under `## Ticket System`, so a section-scoped regen must
  catch it there. Verify afterward: repo-wide `grep -ri 'ticket focus'` returns
  only immutable migration-history entries, `CHANGELOG.md`, and ticket bodies —
  no live reader/semantics reference in `AGENTS.md` / `WORKFLOW.md`.

Verification: `ai-docs/_index.md` is gone from devenv; the template neither
creates nor reads `_index.md` on either the fresh or the upgrade path; no shipped
skill, playbook, convention, `AGENTS.md` step, or spec entry instructs reading it
or declares it canonical (`{#260505-project-memory-index}` describes the dissolved
model); a fresh session started with no manual file reads still receives the repo
orientation it previously depended on `_index.md` for. Gated on all **Blocked on**
prerequisites landing.

### Result (b844635a) - 2026-08-12

Authored the dissolution as a versioned `lead-bootstrap` migration and validated
it on devenv. Range `0b7d0f46..b844635a` (impl `f652a0d8..b844635a`).

Behavioral delta:
- New migration-checklist item in both distributions' `AGENTS.template.md` (ws
  `v0046`, wsflow `v0007` — independent lineages), plus a new `## Project
  Orientation` template section, a rewritten `## Project Memory` read step, and a
  fresh-bootstrap scaffold that no longer creates `_index.md`. Both `WORKFLOW.md`
  master templates and `lead-bootstrap.md` (byte-identical mirror) updated in
  step; skills manifest regenerated.
- devenv self-dissolved: `ai-docs/_index.md` deleted, resident orientation moved
  into devenv's `AGENTS.md` `## Project Orientation`, tracked `# Session Notes`
  migrated to the `repo` note layer (`ai-docs/ws-notes/`) with the stale closeout
  pruned, devenv tag `v0041 -> v0046`. Absorbed-260725 residual Ticket Focus refs
  cleared by regeneration.
- Specs reconciled: `documentation-system.md {#260505-project-memory-index}`
  rewritten to the distributed model; `workflow-skills.md` gained
  `{#260812-bootstrap-index-dissolution}` + coexistence-only health-check framing.
  Two mental-model drift lines fixed on contact.

Deviations / discoveries beyond the plan:
- **`_index.md` was not docs-only.** Two live runtime dependents surfaced during
  edit — `internal/wsdoc/doctor.go` (the `ws-mcp doctor` check) and
  `scripts/bump-ws-version.sh` (a release-version edition point). Both retargeted
  to `AGENTS.md`; `AGENTS.md`'s `## Project Orientation` version strings are now
  the documented bump surface (the edition-point list was updated to match).
- **The dissolution surface was wider than the plan grep.** Review cycle 1 (Test
  Critical) found the entire shared `agents-plugin{,-wsflow}/rsrc/` playbook
  surface + `mental-model-conventions.md` still reading/writing `_index.md`
  (`reference-discovery`, `executor-wrapup`, `lead-write-spec`, `lead-write-ticket`,
  `lead-implement`, `impl-playbook`, `lead-goal-fan-out-step`). Cycle 2 relay swept
  them with **if-present degrade** (context reads fall back to the dissolved homes;
  doc-pipeline writes are gated on the file existing) so un-migrated downstream
  projects keep working; the retired Ticket-Focus write was dropped, not gated.
- **WORKFLOW.md brought into scope** by lead ruling (ticket verification bar +
  convergence invariant).

Verification: extended `grep` sweeps clean (no unconditional `_index.md`
read/write remains); `grep -rni "ticket focus"` clean; `_index.md` absent on
devenv; wsrsrc regen tests, `go build ./...`, and `TestGenerate*Manifest` pass;
`python3 -m unittest discover agents-plugin-wsflow/tests` 9/10 (the one failure is
a pre-existing, unrelated `note.mute`/`note.unmute` wsflow tool-registration gap,
independently reproduced at the base commit — captured as a separate idea ticket).

Review: partitioned (correctness/fit/test). Cycle 1 — fit clean, correctness clean
+2 minor, test 1 Critical + 1 Important. Cycle 2 relay + re-review — Critical
[resolved], minors [fixed], the no-regression-test Important [accepted: no existing
scaffolding, trivial repoints, grep+build verified]. Re-review clean.

> Forward: Phase 2 (coexistence contract) is substantially satisfied by this
> phase's if-present degrade + the compiled-code hard-dependency fixes + the
> transitional-coexistence spec text. It is reduced to a verification pass (run
> the coexistence checks on an un-migrated project; add a guard only if a new hard
> dependency surfaces) — no new mechanism expected.

### Phase 2: Dogfood the v0046 migration on the real new-plugin build

Redirected 2026-08-12 (pre-Result, no phase output yet) from a reasoned
coexistence-contract pass to a live dogfood. Rationale: Phase 1 **authored** the
v0046 dissolution step and validated it only by hand-editing devenv — the
`lead-bootstrap` skill's v0046 step has never executed as a real caller, and
author-time correctness of the template/WORKFLOW/rsrc surface does not prove
live-runtime correctness once session-start injection (`# Notes`, `# Manuals`,
generated tables) is layered on. Running the actual new-plugin (0.40.3) bootstrap
end-to-end is the stronger check, and it **absorbs** the original Phase 2
coexistence verification by executing it live rather than reasoning about it — so
the original acceptance criteria are folded into run (b) below, not dropped.

**Prerequisite — a live session must actually serve 0.40.3.** Plugin-cache keys
on the version string, and `260812-research-reload-plugins-keeps-stale-mcp-binary`
records that `/reload-plugins` can reconnect to the *stale* MCP process. This
phase cannot start until a session is confirmed running the 0.40.3 build
(`runtime.info` / tool-surface check), so that friction is on the critical path.

Two bootstrap runs:

- **(a) Re-run bootstrap on already-dissolved devenv** (migrated-state /
  idempotency path). The upgrade must **not** re-add a `_index.md`-reading step or
  re-create the file; session-start injections must coexist with
  `AGENTS.md ## Project Orientation` **without contradiction or duplication**; the
  index-health-check must skip cleanly given no `_index.md`.
- **(b) Run bootstrap on an un-migrated fixture** carrying a live `ai-docs/_index.md`
  and an old template version. This executes the v0046 migration for real, and
  doubles as the original Phase 2 graceful-degrade check: before migration the
  ambient injections and generated tables coexist **additively** with the live
  `_index.md` (transitional duplication is acceptable, not a conflict), the
  index-health-check runs **because** `_index.md` exists, the staleness alarm
  nudges the new migration item, and no tool errors on `_index.md` being present;
  after migration the file is gone and the readers skip cleanly.

Capture every contradiction between the authored surface and live behavior as a
finding and route it (blocking/goal-relevant → `ready/` under the sage gate,
incidental → `idea/`). Phase 1 already confirmed no compiled-code hard-dependency
on `_index.md` and documented the transitional coexistence in spec
(`documentation-system.md` / `workflow-skills.md`); this phase live-verifies both
and adds new mechanism only if a real contradiction or hard-dependency surfaces.

Acceptance: both runs complete on a confirmed-0.40.3 session; any contradiction is
captured and routed; if a real conflict or hard-dependency is found it is fixed
here when in scope, otherwise spun into a linked child ticket.

### Result (1ec90d06) - 2026-08-13

Live dogfood of the v0046 migration executed end-to-end on a confirmed 0.40.3-dev
session (`runtime.info` = `0.40.3-dev`). Both runs completed; zero contradictions
between the authored surface and live behavior, so nothing was routed.

**Run (a) — migrated-state / idempotency, on devenv itself.** `/ws:lead-bootstrap`
resolved mode=upgrade with installed tag `v0046` == latest template `v0046`, so
zero migration items were walkable: no `_index.md`-read step re-added, no file
re-created, managed sections un-drifted (project-specific supersets preserved),
index-health-check skipped cleanly (no `ai-docs/_index.md`). Session-start
injections coexisted with `## Project Orientation` without duplication or
contradiction. No tracked-file change; captured only as a `repo` note
(commit `1ec90d06`).

**Run (b) — real v0046 migration, on an un-migrated fixture.** Built a throwaway
`acmewidgets` fixture (git repo under scratch) at tag `v0045` with a live
`ai-docs/_index.md` carrying orientation, a runbook, ticket/spec inventory tables,
and Session Notes (one live + one self-declared-stale). Bound it via `ferrule` and
verified both migration edges live:

- *Before migration (graceful degrade):* the bootstrap staleness alarm fired at
  ferrule/`workflow_manual` time (v0045 < v0046); `project_tree` and
  `workflow_manual` ran without error with `_index.md` present; generated ticket/
  spec inventories coexisted **additively** with the file's own inventory tables
  (transitional duplication, not a conflict).
- *Migration:* `_index.md` orientation → `AGENTS.md ## Project Orientation`;
  Session Notes → `repo` note layer with the stale flicker note **pruned** (not
  copied); Release Build runbook → `ai-docs/manuals/release-build.md`; inventory
  tables dropped (derivable); read-`_index.md` step and the `## Ticket Focus`
  reader bullet removed; WORKFLOW.md refreshed; `_index.md` deleted; tag
  `v0045 → v0046` (fixture commit `4ad96c41`).
- *After migration:* the staleness alarm **cleared** on the same session (recomputes
  live); the `# Manuals` ambient block now injects the migrated runbook and the
  `# Notes` block injects the surviving session-note; `project_tree` no longer lists
  `_index.md`; no live `_index.md`/`ticket focus` reference remains in managed files.
- *Convergence invariant confirmed:* the upgrade-migrated fixture reached the same
  `AGENTS.md` shape a fresh scaffold produces — dissolved `## Project Memory`,
  populated `## Project Orientation`, no `_index.md`, tag `v0046`.

Phase-1 forward note confirmed: no new hard-dependency or mechanism surfaced, so
Phase 2 stayed a verification pass. Incidental non-findings (not routed): devenv
still carries a gitignored `ai-docs/_continue.local.md` that v0025 would have
deleted (forward-only upgrades never re-walk it); the fixture raised a doc-coverage
alarm because its spec stubs lack frontmatter (a fixture artifact, not v0046
behavior).
