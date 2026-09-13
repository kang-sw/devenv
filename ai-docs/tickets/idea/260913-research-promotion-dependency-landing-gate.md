---
title: "promotion dependency-landing gate — a ready ticket can be dispatched with a declared upstream prerequisite still unlanded, and no reviewer catches it on the solo-promotion path"
---

# promotion dependency-landing gate — a ready ticket can be dispatched with a declared upstream prerequisite still unlanded

> Upstream report raised from a downstream project (gunpowder-odyssey). The
> commit SHAs in the reproduction timeline are that project's, not this repo's.

## Background

A ticket sitting in `ready/` was handed to an execution worker. Mid-execution
the worker discovered that one of its phases consumes the output of a **second
ticket that was still in `todo/`** and had never been promoted. Execution
stalled; the worker recorded a block rather than producing the phase.

Concrete instance:

- `260730-refactor-godot-client-migration` (in `ready/`) — its Phase 2 consumes
  a Rust-side tuning registry produced by `260730-feat-gdext-egui-debug-tuning`
  (in `todo/`). The interleave between the two was declared in both tickets'
  `related:` frontmatter before promotion.
- Reproduction timeline (downstream SHAs):
  - `76cb0f7` — migration ticket promoted to `ready/` **solo**, after passing
    both sage reviews. The interleave partner was not promoted with it.
  - `27831c4` — worker executes, reaches Phase 2.
  - `595f6f6` / `455b0c9` — worker discovers Phase 2 needs the tuning ticket's
    Phases 1-2 landed first, and records a "Blocked" prerequisite in the ticket
    body. This was the first time the precise ordering (tuning Phases 1-2 **must
    land before** migration Phase 2) existed as text.

The question this raises: a `ready/` ticket whose execution is gated on a
`todo/` ticket is the kind of sequencing blunder the promotion pipeline exists
to prevent. Why did neither the design nor the completeness reviewer catch it,
and where should the fix live?

## Why neither reviewer caught it (current design)

Read against `ws/rsrc/ticket-reviewer-design/ticket-reviewer-design.md` and
`ws/rsrc/ticket-reviewer-completeness/ticket-reviewer-completeness.md`:

- **Design reviewer, solo path — blind by construction.**
  - Cross-ticket reads are bounded to the supplied batch, every ticket currently
    in `ready/`, and the named `parent:` epic. `related:` is explicitly *not* an
    independent contradiction anchor: "a related ticket is compared only when it
    is in `ready/` or is the named parent or a supplied batch member." The tuning
    ticket was in `todo/` and is not the parent, so it was **structurally
    invisible** as a conflict anchor.
  - Process step 3 sketches the plan "taking every `Relations:` entry as landed.
    A premise the table accounts for is a **sequencing fact, not a design
    defect**." A declared-but-unlanded dependency is, by this rule, assumed
    satisfied.
- **Design reviewer, batch path — has the check, but it wasn't given the batch.**
  - The Batch review boundary *does* name the capability: "An initial review
    evaluates the complete batch for contradictions, duplicated scope,
    **dependency mistakes**, and overlapping implementation surfaces."
  - But this only engages when both tickets are in one promotion batch. The
    migration ticket was promoted **solo**, which deprived the reviewer of the
    cross-ticket view that would have surfaced "you are promoting the consumer
    without its producer."
- **Completeness reviewer — out of scope by design.**
  - "Read only the ticket file at the provided path; do not load any document it
    links." and "A `Relations:` table ... you neither evaluate it nor report on
    it." It checks only that `related:`/`parent:` links are *present* when the
    ticket depends on other work (checklist 2), never their state or ordering.

So the escape was not a reviewer bug. It was a **promotion decision** (promote
the consumer solo, leave the interleave partner in `todo/`) that bypassed the
one check — batch-level dependency-mistake detection — capable of catching it,
combined with the absence of any gate that verifies declared upstream
prerequisites are actually landed.

## The core distinction: two concerns wearing one coat

"A ready ticket depends on a todo ticket" bundles two concerns of different
natures. A fix that does not separate them will land in the wrong place.

- **(A) Is the dependency *ordering contract* written in the ticket?** —
  a **text property** of the ticket body. Does each phase that consumes an
  upstream output state which output, from which ticket, and that it must
  precede this phase? In the instance above this was *under-specified* at review
  time: `related:` said "interleaves with Phases 2-5," but the precise critical
  path ("Phase 2 blocked until tuning Phases 1-2 land") only appeared later in
  `455b0c9`. This concern is single-file, content-hashable, and **belongs to the
  completeness reviewer** — an extension of its existing checklist-2.
- **(B) Are those declared upstream prerequisites actually *landed* (in `ready/`
  or `.done/`) at promotion/dispatch time?** — a **point-in-time, cross-ticket
  scheduling fact**. It is not a property of the reviewed ticket's text; the
  ticket can be byte-identical while the dependency's status changes underneath
  it. This was the actual root cause in the instance above.
- **(C) Is the prerequisite's *output actually landed on the track* (code
  merged), not merely its ticket promoted?** — a refinement of (B) surfaced by
  the 2026-09-13 ws cross-session-mailbox dogfood in *this* repo:
  `260913-feat-cross-session-mailbox-wake` declared
  `260913-feat-cross-session-mailbox-core` as a prerequisite. At wake's
  promotion the core ticket was already in `ready/`, so a gate checking
  "prerequisite ticket in `ready/` or `.done/`" (B as literally worded) would
  have **passed** — yet wake's *execution* still required the core *code* to be
  merged to the track first, which the lead handled manually (merge core, then
  dispatch wake). "Ticket promoted to `ready/`" and "code landed on the track"
  are different conditions: a prerequisite sitting in `ready/`-but-unexecuted
  satisfies (B) while still blocking the consumer at execution. So the
  dispatch-time gate's landed-predicate needs a code-level notion (prerequisite
  in `.done/`, or its consumed phase carrying a `### Result`), not merely "the
  ticket has left `todo/`."

## Why (B) must NOT live inside a content-hashed review

Sage review freshness is content-hash based: a ticket body is hashed into
`sage-review-*-reviewed`, and only a body edit invalidates the stamp. A
dependency's status is **not in the reviewed ticket's content**. Therefore, if
(B) were folded into either reviewer:

> It would pass at review time and stamp green. The dependency could then be
> demoted, re-ordered, or never promoted — and because the reviewed ticket's
> body did not change, the content hash stays valid and **the stamp stays green
> while the world moved underneath it**. The check would reproduce the exact
> silent-staleness failure it was meant to prevent.

This is the decisive argument for routing (B) out of the reviewers and into the
scheduling layer, where it is evaluated at the moment the scheduling fact is
asserted (promotion to `ready/`, or dispatch to a worker).

## Proposed shape

1. **Completeness reviewer — extend checklist (text only).** For each phase that
   consumes an upstream output, require the ticket to state the consumed output,
   its source ticket, and the ordering. This catches (A) — an under-specified
   ordering contract — without loading any linked document or reading any
   dependency's status. Stays single-file; stays content-hashable.
2. **New promotion/dispatch-time dependency-landing gate (the core fix).** Before
   a ticket enters `ready/` (or before a worker is dispatched against it), verify
   that every declared upstream prerequisite its unfinished phases consume is
   itself in `ready/` or `.done/`. Fail loud otherwise. This is a scheduling
   invariant checked at the scheduling moment — the correct home for (B), and it
   directly prevents the solo-promotion escape.
3. **Design reviewer — extend the dependency-mistake check to the solo path as a
   non-blocking finding.** The batch path already reasons about "dependency
   mistakes"; let the solo path resolve the status of directly-declared
   `related:` prerequisites on an unfinished phase's critical path and surface an
   **advisory ordering finding** (not a block) when one is unlanded. Non-blocking
   because "promote the partner in the next batch" is a legitimate plan;
   conflating design quality with scheduling state would produce noise.
4. **Machine-readable dependency in frontmatter — for the selector, not just the
   gate.** The ticket-selector explorer runs on a deliberately cheap model and
   orders `ready/` mechanically; it cannot infer a prerequisite from phase prose.
   So each *blocking* prerequisite a phase consumes must be declared as a typed,
   machine-readable frontmatter edge (e.g. `blocked-by:` distinct from soft
   `related:`) that both the dispatch-time gate (2) and the selector read without
   parsing prose. This turns "what counts as consumes" (see Open Questions) from
   an inference into a declared fact, and is the substrate a dumb selector needs
   to pick a correct order at all.
5. **Consistency check: does the typed frontmatter edge match the prose? (owner
   contested — needs discussion).** A ticket can state an ordering contract in
   phase prose (A) yet omit the typed frontmatter edge (4), leaving the
   selector and the gate blind despite a human-legible dependency. Verifying that
   every prose-declared consumed prerequisite has a matching typed frontmatter
   edge is single-file, text-only, and content-hashable — which *looks* like
   completeness-reviewer work. **But** the completeness reviewer's charter
   deliberately excludes evaluating the `Relations:` table, and the original
   design resisted expanding that role; assigning this check there is a real
   design decision, not a free extension. Candidate owners: (i) extend the
   completeness reviewer despite the prior resistance; (ii) a separate cheap
   promotion-time lint; (iii) fold it into the dispatch-time gate (2). Resolve by
   discussion before implementing — see Open Questions.

## Counter-argument (kept deliberately, for balance)

The reviewers' bounded reads and relations-as-landed posture are **intentional**,
not oversights: they keep each review cheap, keep design-quality assessment
independent of scheduling state, and keep the content-hash freshness model
sound. Any proposal that makes a reviewer traverse dependency status on every
single-ticket review erodes all three. This is precisely why the recommendation
puts *status verification* (B) at a scheduling gate and leaves the reviewers to
text (A) plus an advisory. The fix should respect the existing design axis, not
fight it.

## Open questions

- **Gate placement: promotion vs dispatch vs both?** Checking at promotion-to-
  `ready/` catches it earliest but forbids the legitimate "stage the consumer in
  `ready/` now, promote the producer imminently" workflow. Checking at dispatch
  is more permissive but lets an un-executable ticket sit in `ready/`. A
  promotion-time *warning* plus a dispatch-time *hard gate* may be the right
  split.
- **What counts as "consumes"?** The gate and the selector need a
  machine-checkable signal for which `related:` edges are blocking prerequisites
  vs. parallel/soft relations. Proposal (4) leans toward a typed relation (e.g.
  `blocked-by:` vs `related:`) precisely so the cheap selector needs no prose
  parsing; the alternative — inferring it from the ordering contract (A) once
  that is mandatory — pushes NLP-grade reading onto the selector, which is the
  wrong altitude. Confirm the typed-edge direction.
- **Who owns the frontmatter-matches-prose consistency check (proposal 5)?** It
  is text-only and single-file, which argues for the completeness reviewer — but
  that reviewer's design deliberately excludes `Relations:` evaluation and the
  original design resisted role expansion, so this cannot be assumed a free
  extension. This is the one open item that explicitly **needs discussion**
  before a todo carves it; the other proposals (2, 4, and the (C) landed-predicate
  refinement) are mechanically decidable and can proceed.
- **Batch-promotion as the default for interleaved tickets.** Should the
  promotion skill detect a declared interleave and *require* the partner to be
  promoted in the same batch (so the design reviewer's existing dependency-
  mistake check engages), rather than relying on a separate gate?
- **Transitive depth.** Does the gate check only direct prerequisites, or the
  transitive closure? Direct-only is cheap and catches the observed case;
  transitive is safer but costlier and risks false blocks on soft edges.
- **Scope of the fix.** (A) is a reviewer-playbook edit; (B) is new
  runtime/skill behavior at promotion/dispatch. Confirm which layer owns the
  gate (ws runtime vs promotion skill text) before implementing.
