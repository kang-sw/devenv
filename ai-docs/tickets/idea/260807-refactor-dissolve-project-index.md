---
title: Dissolve _index.md — redistribute its content to injected, generated, and always-resident homes
sage-review-design: required
parent: 260807-epic-mechanical-project-memory
related:
  260807-feat-note-memory-layers: prerequisite — the volatile sink; _index.md's Session Notes move to the note layers, which must ship first
  260807-feat-manuals-doc-tier: prerequisite — the procedure sink; _index.md's inlined procedures and Read-Before-Editing table move to manuals/, which must ship first
  260710-bug-project-index-ticket-focus-stale-status: prerequisite — the derivable->generate leg; Ticket Focus / status content must become generated before it can be removed from _index.md
  260725-idea-retire-ticket-focus-root-regen: prerequisite — retires the _index.md Ticket Focus regen machinery, part of the derivable->generate leg
  260728-research-index-ticket-table-drift: motivates — documents the hand-maintained _index ticket/spec table drift this dissolution removes by generation
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
- `260807-feat-manuals-doc-tier` — the `manuals/` tier, destination for procedures.
- The derivable->generate leg (`260710`, `260725`, and the drift documented in
  `260728-research-index-ticket-table-drift`) — so the ticket/spec/status tables
  are generated, not hand-copied, before they are removed from `_index.md`.

## Decisions

Dissolution target — each `_index.md` region redistributes to its correct home
(the epic's `dissolution-target` cross-child decision):

- **Volatile content** (Session Notes and any per-session state) -> the `machine`
  / `worktree` **note layers**. Injected, not file-read.
- **Every-session orientation** (repo identity, plugin topology, canonical flows,
  and the routing index) -> **`AGENTS.md`**, which the harness already
  auto-injects every session. Only stable, always-resident orientation belongs
  here; nothing that needs periodic hand-maintenance.
- **Procedures** (the inlined operating procedures and the "Read-Before-Editing"
  table) -> **`manuals/`**, ambient-injected via the manuals tier. The
  hand-maintained routing table is *replaced* by the generated `# Manuals`
  ambient index — this is what retires its drift, not a copy.
- **Derivable tables** (ticket inventory, spec inventory, status/focus) ->
  **generated** output (`project_tree` and the derivable->generate leg). Nothing
  hand-maintained that a tool can regenerate.

After redistribution, `_index.md` holds nothing requiring a session-start
behavioral read and is removed; references to it (AGENTS.md project-memory step,
conventions) are updated to point at the new homes.

## Open Decisions

Recorded as OPEN — needs a user decision before this ticket is accepted-actionable
(surfaced by the design review, resolution: missing):

- **The tracked Session Notes have no landed home.** `_index.md`'s Session Notes
  are git-tracked, cross-clone shared content (commit-hash closeouts, dogfood
  findings). The note-memory prerequisite ships only the NON-tracked `machine` /
  `worktree` layers and defers the tracked `repo` layer. So this dissolution must
  pick a fork:
  - **(a) Accept non-tracked.** Treat Session Notes as volatile: live ones move to
    the non-tracked layers, historical closeouts fall to git history (recoverable
    via `git log --grep`). Cross-clone shared persistence of notes is dropped.
  - **(b) Require the tracked `repo` layer.** Make the deferred tracked `repo`
    note layer (epic-owned) a hard prerequisite of this ticket, so tracked notes
    keep a tracked, cross-clone home.
  Until this is decided, the Decisions "Volatile -> note layers" leg is
  under-specified for the tracked subset.

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
  derivable tables -> generated.
- Update `AGENTS.md`'s session-start reading protocol so it no longer instructs
  reading `_index.md`; it points at the injected/generated/always-resident homes.
- Reconcile the documentation-system convention text that currently names
  `_index.md` as "project memory and focus."
- Rewrite `spec/documentation-system.md` §`{#260505-project-memory-index}` so no
  live spec entry declares the deleted `_index.md` canonical (see Spec Impact).

Verification: `ai-docs/_index.md` is gone; no shipped skill, playbook, convention,
`AGENTS.md` step, or spec entry instructs reading it or declares it canonical
(specifically `{#260505-project-memory-index}` describes the dissolved model);
each former region is reachable through its new home (a note layer, `manuals/`,
`AGENTS.md`, or a generated table); a fresh session started with no manual file
reads still receives the repo orientation it previously depended on `_index.md` for.

This phase is one reviewable slice but is gated on all three prerequisite legs
landing; do not begin redistribution of a region whose destination is not yet
shipped.
