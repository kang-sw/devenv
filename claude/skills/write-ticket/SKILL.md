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

- Ticket path: `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md` — `YYMMDD` is creation date, never changes on move.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`, `epic`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by full path.
- Status is directory-based only: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`). Never duplicate status in frontmatter.
- Move tickets with `git mv`; no cross-link updates needed.
- Phase numbers are sequential and **stable** — mark dropped phases `[dropped]`, never renumber.
- One phase touches **one cohesive component** unless the change is inherently cross-component.
- Each phase has its own success criteria or test surface.
- All ticket content must be in English regardless of conversation language.
- Tickets are write-once intent documents.
- Ticket stems are **immutable absolute references** — history is queried by stem (`git log --grep`). If a ticket's concept changes fundamentally, create a new ticket that absorbs the old scope and move the old ticket to `dropped/`.

## On: invoke

1. If `$ARGUMENTS` references an existing ticket, read it.
2. **Create** (new ticket):
   a. Determine category from the topic.
   b. Choose initial status directory (`idea/` for vague, `todo/` for actionable — see `judge: initial-status`).
   c. Write the ticket using the **frontmatter template** and a clear problem/goal statement.
   d. If category is `epic`: body defines scope and decomposition (not implementation spec); list child ticket stems; completion means child work is done.
   e. If multiple phases are warranted (see `judge: phase-need`), structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.
3. **Edit** (existing ticket):
   a. Read the ticket first.
   b. Apply the requested changes (update phase, move status).
   c. For moves, `git mv` and add `started:` (→ `wip/`) or `completed:` (→ `done/`) date in frontmatter.
4. **Phase content** — carry everything from discussion that informs implementation: goals, constraints, rationale, rejected alternatives, suggested approaches (pseudo code, struct shapes, data formats, algorithm sketches). Leave to the plan: codebase-derived details (file paths, existing type reuse, integration patterns, function signatures, testing classifications).
5. **Intent review** — re-read the written/edited ticket against the preceding conversation:
   - Are decisions, constraints, rejected alternatives, and suggested approaches captured?
   - Does the ticket distort or omit any discussed intent?
   - Fix gaps in-place; present a brief summary of corrections (or confirm nothing was missed).

## On: delegate

When ticket edits should not consume the lead's context (e.g., mid-implementation
updates, routine status moves), spawn a clerk subagent instead of editing directly.

```
Agent(
  name = "clerk",
  description = "Update ticket per directive",
  subagent_type = "clerk",
  model = "sonnet",
  prompt = """
    Lead name: <lead-name>
    Ticket: <ticket-path>
    Directive: <what to change — be specific>
  """
)
```

The clerk reads `/write-ticket` conventions autonomously and applies the
edit. It reports back what changed and flags convention issues. Use for:
- Status transitions (`git mv` to `wip/`, `done/`)
- Phase updates from implementation findings
- New ticket creation from a delegated context
- Frontmatter updates (`started:`, `completed:`, `plans:`, `skeletons:`)

## Judgments

### judge: initial-status

Place in `idea/` when the topic is exploratory or underspecified; place in `todo/` when the scope and goal are actionable. When uncertain, prefer `idea/` — promotion is cheap.

### judge: phase-need

Prefer more phases over fewer. An overly granular ticket is cheaper to merge than an oversized phase that stalls mid-implementation. Single-component, single-concern work may be one phase.

## Templates

### Frontmatter

```yaml
---
title: <title>
related:             # optional; list of stems with inline comments
  - 260301-feat-foo  # prerequisite
parent:              # optional; epic stem (e.g., 260401-epic-auth-rewrite)
plans:               # maps phases to plan path stems under ai-docs/plans/ (without .md)
  phase-1: 2026-03/28-1430.event-serialization
skeletons:           # maps phases to skeleton commit hashes
  phase-1: abc1234
started:             # YYYY-MM-DD, added on move to wip/
completed:           # YYYY-MM-DD, added on move to done/
---
```

Both `plans:` and `skeletons:` list only phases that have artifacts — omit phases without a plan or skeleton (no null placeholders). Absence means "not yet created" or "not needed."

### Epic body (category = `epic`)

- Body defines **scope and decomposition**, not implementation spec.
- Lists child ticket stems (or planned descriptions).
- Child tickets set `parent:` in frontmatter pointing back to the epic stem.
- Epic moves to `done/` when its scope is satisfied.

## Doctrine

A ticket is the primary context-recovery artifact — a fresh session with no
prior conversation must reconstruct the full decision context from the ticket
and its linked plans. Every authoring choice optimizes for **recoverability
of intent**: decisions, constraints, and rejected alternatives are captured
at the point of writing so that downstream skills (`/implement`)
never re-derive what was already settled. When a rule is
ambiguous, apply whichever interpretation better preserves recoverability.
