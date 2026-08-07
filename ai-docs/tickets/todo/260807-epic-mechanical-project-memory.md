---
title: Mechanical project memory — dissolve _index.md into injected and generated layers
sage-review-design: completed
related:
  260807-feat-note-memory-layers: child — the volatile memory sink (machine/worktree note layers + workflow_manual injection)
  260523-bug-worktree-local-index-missing: resolved-by-child — closed by the note-memory child's non-tracked layers
  260730-refactor-retire-goal-fan-out-step-and-session-note: constraint — note-memory must not resurrect the retiring session.note surface
  260716-feat-mental-model-openup-injection: related-not-child — the selective (rule-based relevance) injection path this epic deliberately excludes; shares only the frontmatter substrate
  260710-bug-project-index-ticket-focus-stale-status: related — the derivable->generate leg the decomposition child depends on
  260725-idea-retire-ticket-focus-root-regen: related — retires _index.md Ticket Focus regen machinery, part of the derivable->generate leg
  260728-research-index-ticket-table-drift: related — documents the hand-maintained _index table drift this epic removes
---

# Mechanical project memory — dissolve _index.md into injected and generated layers

## Scope

Convert `ai-docs/_index.md` from a hand-edited monolith read at session start
(behavioral, unreliable — it is routinely not in context) into a set of
mechanically-delivered layers. Three coordinated work categories:

1. **Note tool** — a harness-agnostic PC-local key-value note store at the MCP
   layer, injected into `workflow_manual` (`260807-feat-note-memory-layers`).
2. **`manuals` doc tier** — a new first-tier document category (peer to spec /
   mental-model / ticket) for per-repo operating procedures, whose one-line
   applicability index is auto-generated from frontmatter and ambient-injected
   into `workflow_manual`; bodies are read on demand.
3. **`_index.md` absorption** — redistribute `_index.md`'s remaining content to
   its correct homes and dissolve the file.

These are grouped for **board coherence** (one project-memory reshape), not
because they are tightly coupled — `manuals` in particular can land
independently.

## Non-Scope

- **Selective / contextual relevance injection.** Rule-based "which doc is
  relevant to what I'm about to do" injection (the mental-model open-up problem)
  is hard, precision-bound, and owned by the `260716` cluster. This epic uses
  only **ambient** injection (inject the whole one-line index; the agent
  self-selects). Explicitly excluded.
- **Injection telemetry / diagnostics** (`260716-feat-ws-doc-condition-diagnostics`).
  Ambient injection has no selection and therefore no silent false-negatives to
  measure, so it needs no telemetry substrate.
- **The tracked `repo` note layer and its `_index.md` modular-assembly** is a
  deferred phase inside the note-memory child, not an epic-level commitment.

## Child Tickets

- `260807-feat-note-memory-layers` - the volatile memory sink; Phase 1
  (non-tracked machine/worktree layers + injection) closes
  `260523-bug-worktree-local-index-missing`. Currently `idea/`.
- Planned: **`manuals` doc tier** (`feat`) - new first-tier category, shared
  frontmatter infra (see Cross-Child Decisions), ambient one-line applicability
  index injected into `workflow_manual`, discovery tools, `project_tree`
  integration, and a bootstrap-migration step moving procedure content from
  `ref/` and inline `_index.md` procedures into `manuals/`. Landable
  independently of the other children.
- Planned: **`_index.md` decomposition** (`refactor`) - the consumer that
  dissolves the file. Depends on note-memory Phase 1 (volatile sink), the
  `manuals` tier (procedure sink), and the derivable->generate leg
  (`260710` / `260725` / `260728`).

## Cross-Child Decisions

- **Mechanical injection over behavioral reads.** Every memory / routing surface
  is delivered by mechanical injection into `workflow_manual` — host-neutral
  (MCP, identical on every harness) and fired exactly when the workflow engages —
  not by an agent choosing to read a file. This is the epic's founding principle.
- **Injection strategy follows cost-of-miss.** Ambient injection (inject the
  whole index, agent self-selects) fits cheap-miss surfaces (manuals routing,
  notes): a missed one-line pointer is cheap to recover and no rule must be
  correct. Selective injection (a rule pre-selects relevance) fits expensive-miss
  surfaces (heavy mental-model docs) and is the hard, out-of-scope path.
- **Shared substrate is frontmatter infra only.** A tier-agnostic frontmatter
  parser plus a minimal schema (one-line `summary:` and a `sources:` /
  applies-when applicability signal) is shared across doc tiers; mental-model
  already carries `sources:` globs. Injection logic and telemetry are **not**
  shared with the `260716` cluster.
- **No session.note revival.** The note tool is a fresh `note.*` surface;
  `260730` retires the zero-caller `session.note` and forbids building on it.
- **Note storage split.** Non-tracked note layers live outside the working tree
  (no `.gitignore` / `.git/info/exclude` needed). A tracked layer, if it lands,
  is one-key-per-file so merge conflicts resolve on the filesystem — no merge
  tooling in MCP.
- **Dissolution target.** `_index.md` redistributes to: volatile -> notes;
  every-session orientation (repo identity, canonical flows, the routing index)
  -> AGENTS.md; procedures -> `manuals/`; derivable tables (ticket/spec) ->
  generated (`project_tree`). AGENTS.md keeps only stable always-resident
  orientation; the previously hand-maintained routing table is replaced by the
  generated `manuals` ambient index, which is what retires its drift.

## Completion Criteria

- Done: note-memory Phase 1 shipped (`260523` closed); `manuals` tier shipped
  with ambient injection and the ref/`_index` procedure migration; `_index.md`
  decomposition complete (nothing in it requires a session-start behavioral read).
- Dropped: mechanical injection proves not worth its substrate cost versus a
  slimmed hand-maintained `_index.md`.
- Deferred: the tracked `repo` note layer and any selective/contextual injection.
