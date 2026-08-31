---
title: Decomposition granularity — AI over-splits phases and tickets, amplifying review load
related:
  260831-refactor-severity-graded-per-slice-review-relay: sibling lever — that ticket lightens per-phase review *weight*; this one investigates reducing the *count* of units (phases and tickets) that each draw review
  260824-epic-review-watermark-model: the review-altitude epic whose sweep/gate is the coarser-boundary alternative to per-phase review
related-mental-model:
  - workflow-skills
---

# Decomposition granularity — AI over-splits phases and tickets, amplifying review load

## Background

The dogfood pain that motivated `260831-refactor-severity-graded-per-slice-review-relay`
is a *product*: felt review load = per-unit review *weight* × *count of units*.
`260831` attacks the weight factor. This ticket holds the count factor, which
turns out to live at **two altitudes**:

- **Phase granularity** — how many `### Phase N` sections one ticket carries.
- **Ticket granularity** — how many tickets an epic/decomposition is split into.

Ticket granularity is the heavier amplifier: **each ticket carries its own sage
review gate (design + completeness)**, so over-splitting tickets multiplies the
authoring-side ceremony too, not just the implement-side review. Both altitudes
are set in ticket authoring / planning and the review surfaces around it — a
different subsystem from `enter.implement`'s review loop — so this is kept as a
separate follow-up rather than folded into `260831`.

## Survey Findings (2026-08-31 scan)

- **Phase count is entirely author judgment; no tool derives or suggests it.**
  `tickets.checklist`'s "phase" means the *authoring stage* (`content`/`intent`),
  not implementation-phase count
  (`agents-plugin-tool/internal/wsdoc/tickets_checklist.go`). The planning
  surface (`plan-populator-survey`, `lead-implement` Plan contract) consumes a
  single `selected_phase` and plans *within* it — it inherits whatever phases the
  author already wrote and cannot drive phase count. So the lever is entirely
  upstream in `lead-write-ticket`.
- **The governing rule is already conservative — strong prose, zero
  enforcement.** `judge: ticket-shape`
  (`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`) states the phase
  default is one `Phase 1` and explicitly rejects "differing review,
  verification, or rollback boundaries alone" as a split justification. It also
  owns the ticket-split rule ("unrelated increments belong in separate actionable
  tickets"). But it is author-applied judgment with no tool check and no reviewer
  that ever flags "too many phases/tickets."
- **Reinforcing signals normalize multi-unit shape with no counter-pressure:**
  the emitted ticket template models a *two-phase* skeleton
  (`tickets_template.go`), and the completeness reviewer imposes per-phase
  obligations ("each phase bounded / each phase has verification",
  `agents-plugin/rsrc/ticket-reviewer-completeness/`) so review load scales with
  phase count while nothing penalizes over-splitting.
- **The ticketization proposal is not codified anywhere.** `lead-discuss`
  surfaces "let's make these tickets" only informally in conversation and routes
  creation to `lead-write-ticket`; its only ticket rules are "wait for the user's
  explicit signal" and "route creation to lead-write-ticket." `lead-write-ticket`
  owns the split *rule* (`judge: ticket-shape`) but sees one ticket at a time and
  only ever suggests the *next single* child (epic/workset handoff). No surface
  proposes the whole decomposition back to the user with merge candidates.

## Direction

Treat the two altitudes differently. Full automation is a non-goal: granularity
depends on intent the AI cannot fully infer, so the realistic aim is to reduce
drift and keep a cheap human touch where judgment is irreducible.

### Within-ticket phase splitting — automate, no user in the loop

- **Rule-aware design-review signal.** Add a "phase over-split" check to the
  *design* reviewer (its charter is coherence/right-problem/executability; the
  completeness reviewer only checks that fields are present). It must apply
  `judge: ticket-shape` faithfully — flag only phases that are **not**
  sequentially dependent — so it does not fight legitimate splits. Design review
  runs and corrects on nearly every ticket already, so the extra check adds no
  meaningful round cost.
- **Upstream prevention (cheaper and more fundamental than the signal):** change
  the emitted template to model **one phase by default** (LANDED as a
  consistency hotfix in `f5784dea` — the template contradicted the already-stated
  one-phase default, so it did not need the measure-first gate), and resolve the
  completeness reviewer's asymmetry where per-phase obligations scale load with
  no over-split counter (still open).

### Epic-to-ticket splitting — human checkpoint, not automation

- **A per-ticket reviewer is structurally blind to this** (it sees one ticket).
  The over-decomposition critique can only bite where the whole child list is
  visible: the **epic/workset design review** (plus the relation table). Route
  the ticket-shape critique there.
- **The decision stays the user's.** The gap is that today's ticket-creation
  approval is low-salience and easy to rubber-stamp. Make it high-salience by
  naming merge candidates at the moment of proposal — e.g. "N tickets proposed;
  A and B look mergeable." The natural home is **`lead-discuss`'s
  creation-handoff moment**: it is the only surface still holding the whole
  intent before it is split, and it already routes creation. This fires **after**
  the user signals they want to persist, so it does not violate the discuss rule
  against proactively nagging to persist.

## Non-Goals

- Full automation of ticket-count decisions. Ticket-vs-ticket granularity is an
  intent-dependent judgment that stays with the user; the goal is a cheaper,
  more visible checkpoint, not removing the human.

## Open Questions

- **Measure before investing.** After `260831` lands and per-phase weight is
  severity-graded, re-check whether phase count still produces felt load — cheaper
  phases may neutralize the amplifier without any phase-count lever.
- Is the phase drift a planning-model behavior (how plans propose phases) or a
  ticket-authoring behavior (how `lead-write-ticket` records them), or both?
- For ticket granularity: is the high-salience checkpoint a new explicit prompt
  in `lead-discuss`, or a strengthening of the existing creation approval? And
  should the epic-level design review carry the over-decomposition critique, or
  is the human checkpoint alone sufficient?
