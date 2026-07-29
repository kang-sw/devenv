---
title: survey plan scope is half ticket-side work, and plan_depth reads delegation instead of evidence
related:
  260729-research-implement-router-prose-only-dimension: same resolver function group (deriveImplementPlanDepth / enter.implement facts)
  260729-bug-survey-plan-drops-verbatim-contract-text: same survey delegate, different failure surface
  260630-epic-skill-playbook-diet: the diet direction this measurement supplies evidence for
related-mental-model:
  - workflow-skills
---

# survey plan scope is half ticket-side work, and plan_depth reads delegation instead of evidence

## Background

Raised during a dogfood discussion of `lead-write-ticket`'s design review. The
user's question: if a ticket carries enough verified detail — the direction the
sibling populator work is heading — does `lead-implement`'s survey stage still
earn its keep?

Answering it required measurement rather than opinion, so two read-only audits
sampled real plan artifacts. This ticket records what they found. It proposes no
implementation; the conclusions here are inputs to later tickets.

## Measurement

Two independent audits, 20 most-recent plans each, classifying every bullet under
`## Codebase Findings` into one of three classes:

- **POLICY** — a decision about what the system should do (contract, interface
  shape, reuse decision, invariant, risk signal). Truth does not depend on
  current line numbers.
- **TARGETING** — a current-tree location fact (file, line range, call sites,
  present signature). Goes stale when the tree moves.
- **DUPLICATE** — substance already stated in the source ticket.

Corpora: this repo (`ai-docs/.plans/2026-07/`, 2026-07-25..29) and `libhbs`
(2026-07-21..29), a Rust systems/concurrency library — deliberately chosen as an
out-of-domain control, since this repo's prose-heavy work could inflate
line-range dependence.

| | ws | libhbs |
|---|---|---|
| findings total | 310 | 290 |
| POLICY | 151 (49%) | 133 (46%) |
| TARGETING | 104 (34%) | 111 (38%) |
| DUPLICATE | 55 (18%) | 46 (16%) |
| findings/plan (median) | 16 | 13 |
| real `escalate-to-research` | 1/20 | 1/20 (already superseded) |
| plans with targeting dependency | 19/20 | 15/20 |

**The class mix replicates across two unrelated domains.** The prose-domain
worry showed up only in targeting *dependency* (19 vs 15), not in the mix. Roughly
half of what survey produces is policy-shaped work that a ticket could in
principle have settled.

## The natural experiment

The libhbs corpus contains both poles of the user's question.

`260722-feat-pipe-dedicated-dispatch-isolation` (699 lines) carries a line-by-line
implementation checklist with accurate citations. Its Phase 1 plan scored
**13 findings = 0 POLICY / 2 TARGETING / 11 DUPLICATE**. A ticket detailed enough
does collapse the survey into near-pure restatement. This is observed, not
theorised.

`260724-feat-pipe-live-group-rebind` is the same shape of ticket after the tree
moved: its plan records the ticket's citations as "stale by 100-300 lines
post-A2/A4". The same bullets that were duplication against the fresh ticket are
genuine repair work against the stale one.

This repo reproduced the failure independently: a plan recorded
`workflow-skills.md:818-819 → real :857`, off by 38.

The auditor's summary of the effect is the right one:

> Duplicate rate is a property of the ticket, not the surveyor.

**A detailed-ticket strategy works only while the tree stays still.** Pushing
targeting detail into tickets does not remove the survey cost; it converts it
into ticket-repair cost and adds a window in which a fresh implementer trusts
stale coordinates.

## Anti-duplicates: ticket-correcting findings

The libhbs audit reported a class the three-way scheme has no slot for — bullets
that exist to contradict the source ticket:

- a ticket citation pointing at a blank line, with the edit target relocated
- four `pipe.group` read sites where the ticket claimed three
- an accessor the ticket said must be added that already existed

These are fact corrections performed at implementation time, unreviewed. They are
the job description of the ticket-fact populator work tracked separately; this is
the evidence that the work is currently happening, just late.

## Symbol anchors versus line anchors

The five libhbs plans with no targeting dependency had Implementation Plan steps
addressed by **symbol name and pattern** — "clone `hbs_spi_spawn_at`'s shape",
"add a public fn near `compute_group()`" — executable with no line fact at all.

This is a better boundary than "ticket = what/why, plan = where/how":

> Tickets carry symbol and pattern anchors, which are durable. Plans supply line
> ranges, which perish.

It explains both poles of the natural experiment with one rule.

## plan_depth reads delegation, not evidence

`agents-plugin-tool/internal/mcp/implement_resolver.go` `deriveImplementPlanDepth`
returns `survey` whenever delegation is `delegated`, and otherwise `none`. Its
middle branch tests `ChangePoints == "clear" && SideEffectRisk == "low"` and
returns the same value as the fallthrough — a **dead branch**. The fact inputs are
computed and then discarded for this decision. `session_state.go` enforces the
coupling (`want survey` for delegated).

So survey runs because the work was delegated, not because a plan is needed. No
ticket-quality or ticket-freshness input reaches the decision.

The dead branch suggests fact-sensitivity was intended. The measurement suggests
the right signal is **not** ticket quality but **citation freshness**: whether the
files a ticket cites have changed since the ticket was written is mechanically
checkable, and it is exactly what separates the two poles above.

Constraint on any change here: a delegated run needs *some* self-contained
artifact for the fresh implementer to read. Today that is the plan — the premise
`260729-bug-survey-plan-drops-verbatim-contract-text` rests on. Dropping
`plan_depth` to `none` for a delegated run is therefore not available without
first promoting the ticket to delegate authority, which is a larger change.

## Scope drift in the survey artifact

Both audits found the six-heading plan contract is not held:

- ws plans add `## Risk Signals` and put findings in tables and numbered lists
  (~25 items fell outside a bullet-based count).
- libhbs plans add `## Decision: mechanism selected`, `## The fork, resolved`,
  `## Test Design`, `## Confirmed public names`.

One libhbs plan carries `Decision D1 -- deferred-submit hazard (SETTLED HERE, do
not escalate)` *inside* `## Codebase Findings`. That is a survey delegate settling
a design decision, which is not format drift — it is a role boundary being
crossed silently.

Separately, real `escalate-to-research` fired in 2 of 40 sampled plans (~5%). The
research-depth escape hatch is close to inert.

## Measurement caveats

Recorded so later work does not over-trust these numbers:

- Single rater per corpus, no second rater. Mixed-class bullets were 23-31% of
  each sample — that is the honest measure of how often the call was judgment.
  Treat +/-10 on any class total as noise.
- DUPLICATE is a **floor** in both corpora. In ws, at least 12 duplicate verdicts
  come from ticket text authored *from* the plan afterwards (one ticket's
  `## Blocked` section cites the plan as preserved survey output). In libhbs the
  duplicate check rested on ticket summaries, not full reads.
- The counting unit (top-level bullets) is arbitrary; counting sub-bullets would
  push both totals higher and shift the mix toward TARGETING.
- Neither sample is independent: 18 non-inline ws plans are backed by 11 distinct
  tickets; all 20 libhbs plans come from 10 tickets on one subsystem.
- Both are short bursts (5 and 9 days) from one pipeline. These describe current
  pipeline behavior, not surveying in general.

## Conclusions carried forward

- **Survey stays.** Targeting load is real in 15-19 of 20 plans, and the one
  observed collapse into duplication came with its own failure mode attached.
- The ~half POLICY share is an argument for fixing the **ticket** side, not for
  deleting survey.
- The DUPLICATE share is waste that a diet can address directly: survey
  re-derives contract the ticket already states.

## Open questions

- Is citation freshness a usable `plan_depth` input, and where would it be
  computed — resolver, or a ws tool the lead calls before `enter.implement`?
- Should the symbol-anchor/line-anchor rule be written into ticket conventions,
  so tickets stop carrying line ranges that rot?
- Does this merge with `260729-research-implement-router-prose-only-dimension`?
  Both indict `deriveImplementPlanDepth`'s fact-blindness from different angles.
- What owns the survey role-boundary crossing (design decisions settled inside
  `## Codebase Findings`) — a survey playbook constraint, or the review stage?
- Would a second rater materially move the class mix? A cheap replication on a
  third corpus would tell us whether 46-49% POLICY is stable.
