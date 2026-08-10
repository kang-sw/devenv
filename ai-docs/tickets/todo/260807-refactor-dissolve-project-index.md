---
title: Dissolve _index.md — redistribute its content to injected, generated, and always-resident homes
sage-review-design: completed
parent: 260807-epic-mechanical-project-memory
related:
  260807-feat-note-memory-layers: prerequisite — the volatile sink; _index.md's Session Notes move to the note layers, which must ship first
  260810-feat-repo-tracked-note-layer: prerequisite — the tracked `repo` layer, landed home for the git-tracked `# Session Notes` (Resolved Decision chose to keep them tracked)
  260807-feat-manuals-doc-tier: prerequisite — the procedure sink; _index.md's inlined procedures and Read-Before-Editing table move to manuals/, which must ship first
  260710-bug-project-index-ticket-focus-stale-status: prerequisite — the derivable->generate leg; Ticket Focus / status content must become generated before it can be removed from _index.md
  260725-idea-retire-ticket-focus-root-regen: prerequisite — retires the _index.md Ticket Focus regen machinery, part of the derivable->generate leg
  260728-research-index-ticket-table-drift: motivates — documents the hand-maintained _index ticket/spec table drift this dissolution removes by generation
sage-review-completeness: required
---

# Dissolve _index.md — redistribute its content to injected, generated, and always-resident homes

## Background

`ai-docs/_index.md` is a hand-edited monolith the workflow expects an agent to
read at session start. That expectation is behavioral and unreliable — sessions
routinely run without it in context — which is the founding problem of the
`260807-epic-mechanical-project-memory` epic. This ticket is the consumer that
dissolves the file once its content has somewhere mechanical to live.

This ticket does not build any new delivery mechanism; it **redistributes** the
existing `_index.md` content into homes the sibling tickets provide, then removes
the file. It is deliberately last in the epic.

## Blocked on

This ticket cannot land until every destination exists. Its earliest phase waits
on unlanded work, so it stays in `todo/` (not `ready/`) until:

- `260807-feat-note-memory-layers` (Phase 1) — the note layers, destination for
  volatile content.
- `260810-feat-repo-tracked-note-layer` — the tracked `repo` layer, the landed
  home for the git-tracked `# Session Notes` (per the Resolved Decision). Itself
  blocked on the note-memory surface.
- `260807-feat-manuals-doc-tier` — the `manuals/` tier, destination for procedures.
- The derivable->generate leg (`260710`, `260725`, and the drift documented in
  `260728-research-index-ticket-table-drift`) — so the ticket/spec/status tables
  are generated, not hand-copied, before they are removed from `_index.md`.

## Decisions

Dissolution target — each `_index.md` region redistributes to its correct home
(the epic's `dissolution-target` cross-child decision):

- **Volatile content** (Session Notes and any per-session state) -> the **note
  layers**. Injected, not file-read. The **tracked** `# Session Notes` subset goes
  specifically to the tracked `repo` layer (`260810-feat-repo-tracked-note-layer`)
  to preserve cross-clone persistence — see the Resolved Decision; genuinely
  machine-local or ephemeral state goes to `machine` / `worktree`. The one-time
  migration of legacy `# Session Notes` is a `lead-bootstrap` step applying
  qualitative staleness pruning, not an automated move.
- **Every-session orientation** (repo identity, plugin topology, canonical flows,
  and the documentation-system routing that names which doc tier lives where) ->
  **`AGENTS.md`**, which the harness already auto-injects every session. Only
  stable, always-resident orientation belongs here; nothing that needs periodic
  hand-maintenance. (This AGENTS.md documentation-system routing is a *distinct*
  artifact from the per-document `## Read Before Editing` applicability table in
  the next bullet — the two must not be conflated or duplicated.)
- **Procedures** (the inlined operating procedures and the per-document `## Read
  Before Editing` applicability table) -> **`manuals/`**, ambient-injected via the
  manuals tier. That hand-maintained applicability table is *replaced* by the
  generated `# Manuals` ambient index — this is what retires its drift, not a copy.
- **Derivable tables** (ticket inventory, spec inventory, status/focus) ->
  **generated** output (`project_tree` and the derivable->generate leg). Nothing
  hand-maintained that a tool can regenerate. This bucket is **paths only**;
  description-bearing prose is handled by the next bullet, since `project_tree`
  emits paths and cannot reproduce hand-written descriptions.

**Description-bearing inventory / notes regions.** `_index.md` also carries prose
regions the four buckets above do not cleanly claim — `## Runtime Surfaces`,
`## MCP Runtime Notes`, `## Prompt And Agent Inventory`, `## Skill Inventory`, and
`## Current Branch Rules`. Reading them shows they are almost entirely **pointer
prose** (they say where the real content lives) or duplicates of what `AGENTS.md`
and the specs already carry, so none needs a new generator. Per-region disposition
(exhaustive; no region silently falls to "generated"):

- `## Runtime Surfaces` — pointer prose to `spec/mcp-tools.md` /
  `spec/plugin-runtime.md` plus "schemas are runtime-discoverable, don't copy
  them." Already covered by `AGENTS.md`'s Documentation-System list; **fold the
  non-duplicate line ("don't copy runtime-discoverable schemas into docs") into
  AGENTS.md orientation, drop the rest as duplicate.**
- `## MCP Runtime Notes` — pointers to the runbook `ref/ws-mcp.md` (already in
  AGENTS.md) plus concrete Windows launcher startup steps. **The Windows startup
  steps are an operating procedure -> `manuals/`** (the manuals-vs-ref boundary
  call, alongside the existing `ref/windows-dogfood.md`); the pointer lines drop as
  duplicate.
- `## Prompt And Agent Inventory` and `## Skill Inventory` — pointer prose to
  `mental-model/prompt-bundle.md`, `agents-plugin/runtime.json`, and the
  `agents-plugin/skills/` source tree. Source/spec-derivable; **drop as
  source-derivable, keeping at most a one-line "inventory is discoverable from the
  source tree and manifest" pointer in AGENTS.md.**
- `## Current Branch Rules` — the `.codex`-untracked rule is **already in
  AGENTS.md** (Commit Rules) -> drop as duplicate; the "verify branch with
  `git status`, don't trust `_index.md`" line is moot once the file is gone; the
  "no active freeze" line is volatile default state -> drop (a real freeze would be
  recorded as a note, not a standing heading).

After redistribution, `_index.md` holds nothing requiring a session-start
behavioral read and is removed; references to it (AGENTS.md project-memory step,
conventions) are updated to point at the new homes.

## Resolved Decision

**Tracked Session Notes sink — settled (2026-08-10): require the tracked `repo`
layer.** `_index.md`'s Session Notes are git-tracked, cross-clone shared content
(commit-hash closeouts, dogfood findings). Demoting them to the non-tracked
`machine` / `worktree` layers would silently drop that cross-clone persistence, so
the user chose to keep them tracked.

- **Chosen (b) require the tracked `repo` layer.** The deferred tracked `repo`
  note layer is un-deferred and spun up as `260810-feat-repo-tracked-note-layer`,
  now a **hard prerequisite** of this ticket (see **Blocked on**). Tracked Session
  Notes migrate into that layer and keep a tracked, cross-clone home.
- **Rejected (a) accept non-tracked.** Treating Session Notes as volatile (live
  ones to the non-tracked layers, closeouts to `git log --grep`) was rejected: it
  demotes previously-shared tracked content to machine-local, which the user judged
  a real loss rather than acceptable pruning.
- **Migration is a one-time `lead-bootstrap` step with qualitative staleness
  pruning, not an automated mechanism.** Moving a legacy project's existing
  `_index.md` `# Session Notes` into the note tool is a version-gated one-time
  operation owned by `lead-bootstrap`. Its instruction directs the project's lead
  agent to read each note and **qualitatively judge staleness** — migrate a live
  note, drop a stale one instead of carrying it over. There is no staleness
  threshold, no reconciliation mechanism, and no note-tool feature involved. The
  always-injected `# Notes` date display in `workflow_manual` (owned by
  `260807-feat-note-memory-layers`) is a **separate concern** and explicitly not
  this ticket's staleness answer.

## Spec Impact

Recorded now because deleting `_index.md` contradicts a live spec entry, even
though this ticket lands at `idea/` (spec addressing is only gated at `ready/`):

- **`spec/documentation-system.md`** `## Project Memory Index
  {#260505-project-memory-index}` currently declares `ai-docs/_index.md` "the
  project memory and active inventory document." This ticket must rewrite that
  entry to describe the dissolved model (injected notes + `manuals/` + AGENTS.md
  orientation + generated inventory) rather than a single canonical file, and
  reconcile its spec/ticket-inventory expectations to the generated tables. The
  sibling `260807-feat-manuals-doc-tier` also amends this spec (a different
  section — the new `manuals/` tier); the two edits are additive.

## Phases

### Phase 1: Redistribute each region and remove _index.md

For each `_index.md` region, move its content to the home named in Decisions,
updating every pointer that currently sends a reader to `_index.md`
(notably the `AGENTS.md` "Project Memory" step and the documentation conventions),
then delete the file.

- Volatile -> note layers; orientation -> `AGENTS.md`; procedures -> `manuals/`;
  derivable tables -> generated; description-bearing inventory/notes regions per
  their explicit per-region disposition in Decisions (no `_index.md` heading is
  left unclassified or silently dropped as "generated").
- Update `AGENTS.md`'s session-start reading protocol so it no longer instructs
  reading `_index.md`; it points at the injected/generated/always-resident homes.
- Reconcile the documentation-system convention text that currently names
  `_index.md` as "project memory and focus."
- Rewrite `spec/documentation-system.md` §`{#260505-project-memory-index}` so no
  live spec entry declares the deleted `_index.md` canonical (see Spec Impact).
- Migrate this repo's own tracked `# Session Notes` into the `repo` note layer,
  applying qualitative staleness pruning (drop stale closeouts rather than
  carrying them over). Generalizing this into a version-gated one-time
  `lead-bootstrap` migration step for downstream projects is a coupling to
  `lead-bootstrap` — a shared-skill/template change (downstream-affecting), so it
  is recorded here and taken up under `lead-bootstrap`'s own approval, not silently
  bundled into this dissolution.

Verification: `ai-docs/_index.md` is gone; no shipped skill, playbook, convention,
`AGENTS.md` step, or spec entry instructs reading it or declares it canonical
(specifically `{#260505-project-memory-index}` describes the dissolved model);
each former region is reachable through its new home (a note layer, `manuals/`,
`AGENTS.md`, or a generated table) — including every description-bearing region
(`Runtime Surfaces`, `MCP Runtime Notes`, `Prompt And Agent Inventory`, `Skill
Inventory`, `Current Branch Rules`) resolving to its stated disposition rather than
vanishing; a fresh session started with no manual file reads still receives the
repo orientation it previously depended on `_index.md` for.

This phase is one reviewable slice but is gated on all three prerequisite legs
landing; do not begin redistribution of a region whose destination is not yet
shipped.
