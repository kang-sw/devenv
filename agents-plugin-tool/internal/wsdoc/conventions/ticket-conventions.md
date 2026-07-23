# Ticket Conventions

Canonical reference for ticket structure, naming, and lifecycle. For the
rationale and meaning behind these rules, see the workflow manual's
**Ticket System Concepts** section; this document states the operational
rules and hard invariants only.

## Path & Naming

- Path: `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md` — `YYMMDD` is creation date, never changes on move.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`, `epic`, `workset`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by full path.

## Status Flow

- Status is directory-based only: `idea/` → `todo/` → `ready/` → `.done/` (or `.dropped/`). Never duplicate status in frontmatter.
- See the workflow manual's **Ticket System Concepts** section for what each status directory means.
- `ai-docs/_index.md ## Ticket Focus` is the selected active attention list; only `ready/` entries are direct implementation targets.
- Move tickets with `tickets.close(stem, status)` (to done/dropped) or
  `tickets.move(stem, to)` (idea/todo/ready) MCP tools; use native `git mv`
  as fallback when MCP tools are unavailable. No cross-link updates needed.
- Add `completed:` date on move to `.done/`.
- `idea/` tickets may omit `spec:` entries.
- `todo/` tickets may include optional `spec:` entries as recovery hints and promotion candidates.
- Non-`epic`, non-`research`, non-`workset` tickets entering `ready/` require spec addressing through `spec:`, `spec-remove:`, or a body `## Spec Impact` section.
- Epic tickets are lightweight milestone boards and remain exempt from the ready spec-address gate.
- Workset tickets are non-hierarchical operating-context boards, remain exempt from the ready spec-address gate, and normally stay in `idea/` or `todo/` rather than `ready/`.
- Promoting `idea/` → `todo/` is triage and does not require spec creation.
- Promoting or creating a non-`epic`, non-`research`, non-`workset` ticket in `ready/`: `lead-write-ticket` verifies spec addressing before the move or commit and invokes `lead-write-spec` only for contract-first planned spec entries.
- Dropping a ticket with linked spec entries: route through `lead-discuss`, then `lead-write-spec` to remove orphaned `🚧` entries before moving the ticket.

## Epic Tickets

See the workflow manual's **Ticket System Concepts** section for epic-vs-workset rationale.

- Epic tickets do not use implementation phases; child tickets carry phases when needed.
- A single child ticket may carry multiple phases when they form sequential complete implementation units.

## Workset Tickets

See the workflow manual's **Ticket System Concepts** section for epic-vs-workset rationale.

- Worksets list included tickets without making them children; do not add, remove, or change `parent:` based on workset inclusion.
- Worksets do not own decomposition, cross-child invariants, implementation phases, or spec-ready behavior.
- If the grouping starts owning scope decomposition or invariant decisions, create or use an `epic` instead.

## Phases

See the workflow manual's **Ticket System Concepts** section for what a phase is and how to size one.

- Phase numbers are sequential and **stable** — mark dropped phases `[dropped]`, never renumber.
- Structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.

## Stems

- Ticket stems are **immutable absolute references** — history is queried by stem (`git log --grep`).
- If a ticket's concept changes fundamentally, create a new ticket that absorbs the old scope and move the old ticket to `.dropped/`.

## General

- Phase plan text before the first `### Result` is frozen after that Result is written. Unimplemented phases remain editable.
- `### Result (<short-hash>)` uses the commit that first made the completed phase reviewable on its current branch. If the phase was already merged before the ticket update, use the merge commit.
- Result and Edition text record behavioral deltas, deviations, verification evidence, unresolved findings, and deferred follow-up findings without restating the phase plan or linked spec.
- Later implementation passes for an already completed phase append `#### Edition (<short-hash>) - YYYY-MM-DD` under that phase's Result area.
- Existing Result and Edition entries are frozen once written; append a new Edition instead of editing prior result text.
- All ticket content must be in English regardless of conversation language.
