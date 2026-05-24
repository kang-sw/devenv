# Ticket Conventions

Canonical reference for ticket structure, naming, and lifecycle.

## Path & Naming

- Path: `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md` — `YYMMDD` is creation date, never changes on move.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`, `epic`, `workset`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by full path.

## Status Flow

- Status is directory-based only: `idea/` → `todo/` → `ready/` → `.done/` (or `.dropped/`). Never duplicate status in frontmatter.
- `idea/` is rough capture before triage; `todo/` is accepted backlog with recoverable intent; `ready/` is the spec-addressed implementation-ready status.
- `ai-docs/_index.md ## Ticket Focus` is the selected active attention list; only `ready/` entries are direct implementation targets.
- Move tickets with `git mv`; no cross-link updates needed.
- Add `completed:` date on move to `.done/`.
- `idea/` tickets may omit `spec:` entries.
- `todo/` tickets may include optional `spec:` entries as recovery hints and promotion candidates.
- Non-`epic`, non-`research`, non-`workset` tickets entering `ready/` require spec addressing through `spec:`, `spec-remove:`, or a body `## Spec Impact` section; epics decompose scope, research captures findings, and worksets collect operating context.
- Epic tickets are lightweight milestone boards and remain exempt from the ready spec-address gate.
- Workset tickets are non-hierarchical operating-context boards, remain exempt from the ready spec-address gate, and normally stay in `idea/` or `todo/` rather than `ready/`.
- Promoting `idea/` → `todo/` is triage and does not require spec creation.
- Promoting or creating a non-`epic`, non-`research`, non-`workset` ticket in `ready/`: `lead-write-ticket` verifies spec addressing before the move or commit and invokes `lead-write-spec` only for contract-first planned spec entries.
- Dropping a ticket with linked spec entries: route through `lead-discuss`, then `lead-write-spec` to remove orphaned `🚧` entries before moving the ticket.

## Epic Tickets

- Epic bodies preserve board-level context: scope, non-scope, child ticket board, cross-child invariant decisions, and done/drop/defer criteria.
- Detailed discussion, implementation approaches, constraints, and phase-specific decisions belong in child tickets, not in the epic body.
- Epic tickets do not use implementation phases; child tickets carry phases when needed.
- A single child ticket may carry multiple phases when they form sequential complete implementation units.

## Workset Tickets

- Workset bodies preserve non-hierarchical operating context for a session, goal, sprint, or temporary focus area.
- Worksets list included tickets without making them children; do not add, remove, or change `parent:` based on workset inclusion.
- Worksets do not own decomposition, cross-child invariants, implementation phases, or spec-ready behavior.
- If the grouping starts owning scope decomposition or invariant decisions, create or use an `epic` instead.

## Phases

- Phase numbers are sequential and **stable** — mark dropped phases `[dropped]`, never renumber.
- One phase is one complete behavior a future fresh session can finish, review, verify, and hand off cleanly.
- Setup, API, UI, tests, legacy skeleton artifacts, and investigation are phase ingredients unless one is the reviewable deliverable.
- Each phase states its completed behavior, deferred scope, and verification boundary.
- Structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.

## Stems

- Ticket stems are **immutable absolute references** — history is queried by stem (`git log --grep`).
- If a ticket's concept changes fundamentally, create a new ticket that absorbs the old scope and move the old ticket to `.dropped/`.

## General

- Phase plan text before the first `### Result` is frozen after that Result is written. Unimplemented phases remain editable.
- `### Result (<short-hash>)` uses the commit that first made the completed phase reviewable on its current branch. If the phase was already merged before the ticket update, use the merge commit.
- Later implementation passes for an already completed phase append `#### Edition (<short-hash>) - YYYY-MM-DD` under that phase's Result area.
- Existing Result and Edition entries are frozen once written; append a new Edition instead of editing prior result text.
- All ticket content must be in English regardless of conversation language.

## Templates

### Frontmatter

```yaml
---
title: <title>
related:             # optional; map of stem → relationship note
  260301-feat-foo: prerequisite
spec:                # optional; list of spec-stems this ticket implements
  - 260421-feat-example
spec-remove:         # optional; list of spec-stems this ticket's implementation will remove
  - 260421-feat-removed-feature
parent:              # optional; epic stem (e.g., 260401-epic-auth-rewrite)
plans:               # maps phases to plan path stems under ai-docs/.plans/ (without .md)
  phase-1: 2026-03/28-1430.event-serialization
skeletons:           # legacy: maps phases to skeleton artifact commit hashes
  phase-1: abc1234
related-mental-model:  # optional; mental-model stems (filename without .md) consulted
  - workflow-routing   #   during ticket authoring — recovery hint for future sessions
completed:           # YYYY-MM-DD, added on move to .done/
---
```

Both `plans:` and legacy `skeletons:` list only phases that have artifacts — omit phases without an artifact (no null placeholders). Absence of `skeletons:` means "not needed"; normal implementation routing does not create new skeleton artifacts.

### Body (actionable: `feat`, `bug`, `refactor`, `chore`)

```markdown
# <title>

## Background

<problem or goal — what and why>

## Phases

### Phase 1: <title>

<goals, constraints, rationale, rejected alternatives, suggested approaches>

### Result (<short-hash>) - YYYY-MM-DD

<what was implemented, deviations from plan, key findings for future phases>

#### Edition (<short-hash>) - YYYY-MM-DD

<later tweak or follow-up implementation pass for this completed phase>

### Phase 2: <title>

...
```

Optional sections — add between `## Background` and `## Phases` when relevant:

- `## Decisions` — design choices with rationale and rejected alternatives.
- `## Constraints` — non-obvious boundaries (performance, compatibility, etc.).
- `## Prior Art` — existing patterns or components to reuse.
- `## Spec Impact` — ready-only spec addressing when no existing stem yet covers the behavior; include target spec area, expected caller-visible change, and `Contract-first spec: yes|no`.

### Body (category = `research`)

```markdown
# <title>

## Background

<question or context>

## <Topic heading>

<findings, decisions, rejected alternatives>
```

Research tickets have no phases. Sections after `## Background` are freeform topic headings.

### Workset body (category = `workset`)

```markdown
# <title>

## Context

<why this operating set exists>

## Tickets

- `<stem-or-path>` - <status; role in this workset; dependency note>

## Planned References

- `<provisional label>` - <intended role; creation condition>

## Focus

<current session, goal, sprint, or temporary operating focus>

## Exit Criteria

- Done: <conditions for closing this workset>
- Deferred: <what moves out of this workset>
```

Workset bodies define a non-hierarchical ticket collection, not decomposition. Included tickets do not set `parent:` to the workset; planned references do not receive status, path, or `parent:` until a real ticket exists.

### Epic body (category = `epic`)

```markdown
# <title>

## Scope

<included milestone scope>

## Non-Scope

<explicit exclusions>

## Child Tickets

- `<stem>` - <slice purpose/status/dependency note>
- Planned: <child ticket description>

## Cross-Child Decisions

<invariants that child tickets must preserve>

## Completion Criteria

- Done: <conditions for moving the epic to .done/>
- Dropped: <conditions for moving the epic to .dropped/>
- Deferred: <scope intentionally left for a later epic or child>
```

Epic bodies define scope and decomposition, not implementation spec. Child tickets set `parent:` in frontmatter pointing back to the epic stem and carry detailed discussion, approaches, constraints, and phases.
