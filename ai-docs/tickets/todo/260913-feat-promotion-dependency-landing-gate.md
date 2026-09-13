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
- **Gate-owner layer = ws runtime, bound to real tool calls (confirmed
  mechanism).**
  - **Dispatch-time hard gate → `ws/tickets.query` single-stem point-resolve.**
    lead-run always point-resolves the selected/named stem via `tickets.query`
    before spawning a worker — the one chokepoint both the selector path and the
    directly-named-ticket path pass through. The runtime computes, live from
    directory state, a `dispatch_blocked: {blocking_stem, reason}` field in that
    point-resolve projection (populated only in single-stem projection mode, not
    on every query, to avoid noise); lead-run honors it by refusing to spawn and
    reporting the blocker.
  - **Promotion closure → `ws/tickets.move(to: "ready")`.** The runtime reads the
    typed edge and refuses to move a consumer to `ready/` while a typed
    prerequisite is not in `ready/`, `.done/`, or the same batch — the
    machine-enforced upgrade of lead-ticket's existing "Promote to ready" step-1
    dependency-closure (today a playbook-text check over prose `related:`).
  - **Never bound to `ws/tickets.sage_stamp`.** The stamp is content-hash based;
    a scheduling fact bound to it passes at review and stays green while the
    dependency moves underneath — the exact silent-staleness failure the gate
    exists to prevent (design source: "Why (B) must NOT live inside a
    content-hashed review").
  - **Promotion-time advisory warning (soft, text).** For the legitimate
    in-between case — prerequisite in `ready/` (closure passes) but not yet
    code-landed — warn without blocking, preserving the "stage the consumer now,
    promote the producer imminently" workflow.
- **Design-reviewer solo-path advisory.** A non-blocking ordering finding on the
  solo promotion path, mirroring the batch path's existing dependency-mistake
  reasoning. Non-blocking because "promote the partner in the next batch" is a
  legitimate plan and conflating design quality with scheduling state produces
  noise.
- **Division of labor by prerequisite status (confirmed).** A prerequisite in
  `idea/`/`todo/` is caught at the promotion layer (`tickets.move(to:ready)`
  refuses the consumer); a prerequisite in `ready/`-but-unexecuted is allowed to
  promote (with the advisory warning) and caught at the dispatch layer
  (`tickets.query` gate) by the phase-granular code-level predicate; a
  prerequisite in `.done/` or whose consumed phase carries a `### Result` passes
  both.

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
prerequisite (distinct from soft `related:`), and make the runtime enforce it at
two bound tool calls plus feed the selector:
(1) **dispatch-time hard gate on `ws/tickets.query` single-stem point-resolve** —
the projection gains a live-computed `dispatch_blocked: {blocking_stem, reason}`
when the ticket's earliest unfinished phase block-depends on a prerequisite not
landed by the phase-granular code-level predicate (consumed phase has a
`### Result`, or prerequisite in `.done/`); populated only in single-stem
projection mode; lead-run honors it (refuse spawn, report blocker);
(2) **promotion closure on `ws/tickets.move(to: "ready")`** — refuse to move a
consumer to `ready/` while a typed prerequisite is not in `ready/`/`.done/`/the
same batch (machine-enforced upgrade of lead-ticket's step-1 closure);
(3) the **ticket-selector** orders `ready/` mechanically from the typed edge
without prose parsing. The predicate is computed live at each call, never read
from a review stamp.

Verify: a consumer whose typed prerequisite is unlanded gets `dispatch_blocked`
at point-resolve and lead-run refuses to spawn with the blocking stem named;
`tickets.move(to:ready)` refuses a consumer whose typed prerequisite is in
`idea/`/`todo/`; a consumer whose consumed phase's prerequisite phase carries a
`### Result` (prerequisite not fully `.done/`) is allowed (phase-granular
interleave); the selector orders a prerequisite ahead of its consumer from the
typed edge alone; a soft `related:` edge neither gates nor reorders; and no gate
path reads or writes a sage stamp.

### Phase 2: Promotion-time advisory warning + design-reviewer solo-path advisory

Add the softer, non-blocking layer on top of Phase 1's typed edge and gate. The
promotion-time advisory warning fires for the in-between case Phase 1's closure
deliberately allows — a typed prerequisite in `ready/` (closure passes) but not
yet code-landed — telling the promoter the producer is not executed yet, without
blocking the promotion. The design-reviewer solo-path advisory surfaces a
non-blocking ordering finding mirroring the batch path's dependency-mistake
reasoning. Both consume Phase 1's typed edge; both are text/advisory, never a
block.

Verify: promoting a consumer whose typed prerequisite is in
`ready/`-but-unexecuted emits the advisory warning and still completes the
promotion; the design reviewer on a solo promotion surfaces the ordering finding
as advisory (not a block); neither surface fires for a `.done/` prerequisite, a
consumed-phase-`### Result` prerequisite, or a soft `related:` edge.
