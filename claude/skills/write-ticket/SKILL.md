---
name: write-ticket
description: >
  When the user mentions creating, writing, or editing a ticket, or
  when chained from /discuss or another workflow skill, invoke this.
argument-hint: "[topic/description for new ticket, or ticket path to edit]"
---

# Write Ticket

Target: $ARGUMENTS

## Invariants

- Ticket conventions: run `blueprint-infra ticket-conventions.md` — path format, status flow, phase rules, stem rules, templates.
- Never `Read` a ticket file other than the current target — delegate any other ticket inspection to an Explore subagent.

## On: invoke

1. If `$ARGUMENTS` references an existing ticket, read it.
2. **Create** (new ticket):
   a. Determine category from the topic.
   b. Choose initial status directory (`idea/` for vague, `todo/` for actionable — see `judge: initial-status`).
   c. Write the ticket using the **frontmatter template** and a clear problem/goal statement.
   d. If category is `epic`: body defines scope and decomposition (not implementation spec); list child ticket stems; completion means child work is done.
   e. If multiple phases are warranted (see `judge: phase-need`), structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.
   f. After drafting, verify scope — see `judge: ticket-scope`.
3. **Edit** (existing ticket):
   a. Read the ticket first.
   b. Apply the requested changes (update phase, move status).
   c. For moves, `git mv` and add `started:` (→ `wip/`) or `completed:` (→ `done/`) date in frontmatter.
4. **Phase content** — carry everything from discussion that informs implementation: goals, constraints, rationale, rejected alternatives, suggested approaches (pseudo code, struct shapes, data formats, algorithm sketches). Leave to the plan: codebase-derived details (file paths, existing type reuse, integration patterns, function signatures, testing classifications).
5. **Intent review** — re-read the written/edited ticket against the preceding conversation:
   - Are decisions, constraints, rejected alternatives, and suggested approaches captured?
   - Does the ticket distort or omit any discussed intent?
   - Fix gaps in-place; present a brief summary of corrections (or confirm nothing was missed).
6. **Spec check** — if any phase adds or changes user-visible behavior, prompt: "Did this phase introduce or modify public-facing behavior? If yes, invoke `/write-spec` to update the relevant spec."

## Judgments

### judge: initial-status

Place in `idea/` when the topic is exploratory or underspecified; place in `todo/` when the scope and goal are actionable. When uncertain, prefer `idea/` — promotion is cheap.

### judge: ticket-scope

Over ~200 lines is a soft signal; over 300 lines, act. First, prune plan-level detail (file paths, function signatures, integration specifics) — that belongs in a plan document. If still large, the scope is too wide: introduce an epic and split into child tickets, each covering one independently reviewable unit of work.

### judge: phase-need

Prefer more phases over fewer. An overly granular ticket is cheaper to merge than an oversized phase that stalls mid-implementation. Single-component, single-concern work may be one phase.

## Doctrine

A ticket is the primary context-recovery artifact — a fresh session with no
prior conversation must reconstruct the full decision context from the ticket
and its linked plans. Every authoring choice optimizes for **recoverability
of intent**: decisions, constraints, and rejected alternatives are captured
at the point of writing so that downstream skills (`/delegate-implement`)
never re-derive what was already settled. When a rule is
ambiguous, apply whichever interpretation better preserves recoverability.
