# Ticket Conventions

Canonical reference for ticket structure, naming, and lifecycle. For the
rationale and meaning behind these rules, see the workflow manual's
**Ticket System Concepts** section; this document states the operational
rules and hard invariants only.

## Path & Naming

- Path: `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md` — `YYMMDD` is creation date, never changes on move.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`, `epic`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by full path.

## Status Flow

- Status is directory-based only: `idea/` → `todo/` → `ready/` → `.done/` (or `.dropped/`). Never duplicate status in frontmatter.
- See the workflow manual's **Ticket System Concepts** section for what each status directory means.
- Active attention is discovered from the status directories via `tickets.query`/`project_tree`, not a cached index section; only `ready/` entries are direct implementation targets.
- Move tickets with `tickets.close(stem, status)` (to done/dropped) or
  `tickets.move(stem, to)` (idea/todo/ready) MCP tools; use native `git mv`
  as fallback when MCP tools are unavailable. No cross-link updates needed.
- Actionable `todo/` creation and editing are ungated. Populate facts, then run design and completeness Sage review at `ready/` promotion against the populated body.
- An epic is a living board that is never an execution target, so it never enters `ready/` (the move is barred); a research ticket is likewise barred and ungated. Only actionable tickets enter `ready/`.
- Epic design review is design-only (completeness never applies) and lead-judgment-invoked, not boundary-gated: run it when the epic's cross-child design has drifted materially, populating checkable facts first. Ordinary epic edits do not auto-review.
- Add `completed:` date on move to `.done/`.

## Epic Tickets

An epic decomposes one outcome into child tickets and owns their cross-child invariants.

- Epic tickets do not use implementation phases; child tickets carry phases when needed.
- A single child ticket may carry multiple phases when they form sequential complete implementation units.
- Move implementation detail out of the epic body into an implementation child ticket; the epic body carries scope, cross-child invariants, and closure conditions only.
- Move deliberation that outgrows a settled decision line out of the epic body into a `research` ticket and reference it; the epic body carries settled decisions only.

## Research Tickets

Research tickets remain ungated and have no phases. Topic sections are freeform;
the standard Outcome Ledger lives in the ticket and separates investigation
output from implementation authority:

```markdown
## Outcome Ledger

### Verified Findings
<!-- Evidence-backed observations. These may support later tickets but do not choose behavior. -->

### Confirmed Decisions
<!-- Normative choices explicitly confirmed by the user. -->

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->
```

For actionable derivation, only Verified Findings supply evidence and Confirmed
Decisions supply contract. Narrative is supporting context; Proposals, Open
Questions, and unlisted narrative never become child authority. When the ledger
is absent, ask whether to add it or settle the child's decisions directly through
the Open Decision Queue.

## Phases

See the workflow manual's **Ticket System Concepts** section for what a phase is and how to size one.

- Phase numbers are sequential and **stable** — mark dropped phases `[dropped]`, never renumber.
- Structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.

## Stems

- Ticket stems are **immutable absolute references** — history is queried by stem (`git log --grep`).
- If a ticket's concept changes fundamentally, create a new ticket that absorbs the old scope and move the old ticket to `.dropped/`.

## Content

- Record the judgment implementation cannot re-derive: the choice among workable alternatives, why the others lost, agreed interfaces, and what the ticket deliberately leaves untouched.
- Point at code by the search that finds it rather than by surveyed coordinates; `file:line` earns its place as evidence for a claim, not as an edit target.

## General

- Phase plan text before the first `### Result` is frozen after that Result is written. Unimplemented phases remain editable.
- `### Result (<short-hash>) - YYYY-MM-DD` uses the commit that first made the completed phase reviewable on its current branch. If the phase was already merged before the ticket update, use the merge commit.
- Result and Edition text record behavioral deltas, deviations, verification evidence, unresolved findings, and deferred follow-up findings without restating the phase plan.
- Later implementation passes for an already completed phase append `#### Edition (<short-hash>) - YYYY-MM-DD` under that phase's Result area.
- Existing Result and Edition entries are frozen once written; append a new Edition instead of editing prior result text.
- All ticket content must be in English regardless of conversation language.
