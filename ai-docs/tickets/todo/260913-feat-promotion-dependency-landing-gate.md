---
title: "Promotion/dispatch dependency-landing gate with typed frontmatter edges"
related:
  260913-research-promotion-dependency-landing-gate: design-source — carries the settled Confirmed Decisions this ticket implements and the scoped-out proposal 5
  260909-epic-ws-worker-interpreter-refoundation: constraint — lead-run/selector are the shared worker-interpreter surface this touches
---

# Promotion/dispatch dependency-landing gate with typed frontmatter edges

## Background

A `ready/` ticket can be dispatched to a worker while a ticket it block-depends
on is still unlanded, and no reviewer catches it on the solo-promotion path: the
design reviewer treats `related:` as landed and only reasons about dependency
mistakes over an explicit batch, and the completeness reviewer does not evaluate
relations at all. The dependency's landed state is a point-in-time cross-ticket
scheduling fact, not a property of the reviewed ticket's text, so it cannot live
inside a content-hashed review without going silently stale. The root-cause
analysis, the decisive content-hash-staleness argument, and the confirmed
decisions this ticket implements live in the design source
`260913-research-promotion-dependency-landing-gate` (Outcome Ledger → Confirmed
Decisions).

This ticket implements the mechanically-decidable subset. It deliberately does
**not** implement the frontmatter-matches-prose consistency check (research
proposal 5): its owner — completeness reviewer vs. a promotion-time lint vs. the
gate — is contested because the completeness reviewer's charter deliberately
excludes `Relations:` evaluation, and that needs discussion first. That item
stays in the research ticket.

## Decisions

Confirmed with the user (2026-09-13); see the design source's Outcome Ledger.

- **Typed machine-readable frontmatter edge.** A blocking prerequisite a phase
  consumes is declared as a typed frontmatter edge distinct from soft `related:`
  (candidate key `blocked-by:`; exact spelling is this ticket's implementation
  choice). Both the dispatch-time gate and the ticket-selector read it directly,
  so the cheap selector orders mechanically without parsing phase prose.
- **Landed-predicate is code-level and phase-granular.** A declared prerequisite
  counts as landed when the specific consumed phase carries a `### Result`, or
  the prerequisite ticket is in `.done/` — not merely when the ticket has left
  `todo/`. Phase-granular over whole-ticket so consuming phase N of a multi-phase
  producer is a legitimate, allowed interleave.
- **Two-tier gate placement.** A promotion-time *advisory warning* plus a
  dispatch-time *hard gate*. The warning preserves the legitimate "stage the
  consumer in `ready/` now, promote the producer imminently" workflow; the hard
  gate blocks dispatching an un-executable ticket.
- **Design-reviewer solo-path advisory.** A non-blocking ordering finding on the
  solo promotion path, mirroring the batch path's existing dependency-mistake
  reasoning. Non-blocking because "promote the partner in the next batch" is a
  legitimate plan and conflating design quality with scheduling state produces
  noise.
- **OPEN — resolve at `ready/` promotion (fact population / design review):**
  the gate-owner layer, i.e. whether the hard gate lives in ws runtime (a
  tool/selector-side check) or in shared promotion/dispatch playbook text. The
  research deferred this to the child; it is a real decision and must be settled
  through the Open Decision Queue before this ticket is promoted, not invented by
  a worker.

## Constraints

- Whichever surfaces this edits, the typed-edge schema and gate semantics are a
  generic workflow-ordering concern and must stay host-neutral and
  downstream-first: they must not encode any devenv-specific rule, and no shipped
  playbook may be asked to enforce a rule from this repository's `AGENTS.md`
  (Architecture Rules 3–5). The path-scoped manuals
  (`shipped-surface-boundary.md`, `skill-authoring.md`, `wsflow-mirroring.md`,
  `ws-mcp.md`) apply per the root Implementation Conventions table and are copied
  into `## Constraints` at fact population for whichever paths this ends up
  touching.
- The landed-predicate must be evaluated at the scheduling moment (promotion,
  dispatch), never baked into a content-hashed review stamp, or it reproduces the
  silent-staleness failure it exists to prevent (design source: "Why (B) must
  NOT live inside a content-hashed review").

## Prior Art

- Reuse points to confirm at fact population: the selector playbook
  (`ws/rsrc/ticket-selector/…`) and its dependency-preference logic; the design
  reviewer's existing batch-path "dependency mistakes" reasoning
  (`ws/rsrc/ticket-reviewer-design/…`); the dispatch path in `lead-run`
  (`ws/rsrc/lead-run/…`) and its `session.note` assignment record; and
  `ws/tickets.query`'s existing `related:` / status projection as the substrate
  a gate reads.

## Phases

### Phase 1: Typed dependency edge + dispatch-time hard gate + selector ordering

Introduce the typed machine-readable frontmatter edge for a blocking
prerequisite (distinct from soft `related:`), and make two consumers read it:
(1) a dispatch-time hard gate that refuses to dispatch a worker against a ticket
whose earliest unfinished phase block-depends on a prerequisite that is not
landed by the phase-granular code-level predicate (consumed phase has a
`### Result`, or prerequisite in `.done/`); (2) the ticket-selector, so it orders
`ready/` mechanically from the typed edge without prose parsing. Resolve the
OPEN gate-owner-layer decision (runtime vs. shared playbook text) before
implementing this phase — it determines where the gate code lives.

Verify: a consumer whose typed prerequisite is unlanded is refused at dispatch
with a diagnostic naming the blocking stem; a consumer whose consumed phase's
prerequisite phase carries a `### Result` (but the prerequisite ticket is not
fully `.done/`) is allowed (phase-granular interleave); the selector orders a
prerequisite ahead of its consumer from the typed edge alone; a soft `related:`
edge does not gate or reorder; and the predicate is computed at
dispatch/selection time, not read from any review stamp.

### Phase 2: Promotion-time advisory warning + design-reviewer solo-path advisory

Add the softer, non-blocking layer on top of Phase 1's typed edge: a
promotion-time advisory warning when a ticket is promoted to `ready/` while a
typed prerequisite is unlanded (does not block the promotion), and a
design-reviewer solo-path advisory ordering finding mirroring the batch path's
dependency-mistake reasoning (non-blocking). Depends on Phase 1's typed edge as
the machine-readable signal both surfaces consume.

Verify: promoting a consumer whose typed prerequisite is unlanded emits the
advisory warning but still completes the promotion; the design reviewer on a
solo promotion surfaces the ordering finding as advisory (not a block); neither
surface fires for a landed prerequisite or a soft `related:` edge.
