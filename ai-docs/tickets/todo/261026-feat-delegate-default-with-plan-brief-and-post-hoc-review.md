---
title: Delegate by default for settled bounded work, with a write-once plan brief and post-hoc review
related:
  260909-epic-ws-worker-interpreter-refoundation: closed refoundation epic whose lead-surface work this continues
  260909-research-ws-refoundation-evidence-audit: binding anchor; retired the plan/spec/mental-model document layers as write-heavy, read-light, drift-prone; this ticket revives `ai-docs/.plans/` only as a write-once record
  260911-research-golden-fixture-verification-gap: the lead-delegate playbook prose is pinned by a golden fixture
---

# Delegate by default for settled bounded work, with a write-once plan brief and post-hoc review

## Background

Leads routinely send one-line, already-settled fixes through the full ticket
path (fact population, design review, worker run). The cause is the
`lead-delegate` routing gate (`agents-plugin/rsrc/lead-delegate/lead-delegate.md`,
`## Routing`): it routes to `lead-run`, and therefore to a ticket, whenever a
change "affects public behavior, an API, protocol, schema, template, canonical
flow, or architecture" or "independent review is needed". In this repository
almost every change touches a shipped surface, so nearly every change trips the
gate. `ai-docs/manuals/skill-authoring.md` (`## Rule Lifecycle`) calls a gate
whose default is the heavier path a defect: state the cheap default and the
condition that escalates.

The gate also ignores that a project with a review track already reviews every
commit after the fact: the `lead-ship` release gate requires the range since the
review watermark to be clear.

Moving work from tickets to delegates moves the contract from a reviewed
document into the lead's brief. A later `lead-review` then has no contract to
judge against unless the brief survives. This ticket makes the brief a
write-once file that is committed with the implementation.

## Decisions

All decisions below were confirmed by the user in discussion on 2026-10-26.

1. **Delegate is the default for settled, bounded work.** The routing gate
   states a cheap default and the conditions that escalate, instead of an
   enumerated list of touched surfaces. Agreed sketch of the gate prose (the
   final wording follows `skill-authoring.md`):

   ```text
   Default to delegate when the outcome is settled and you can name the files
   before starting. Capture a ticket when a decision is still open, the work
   spans phases or outlives this session, or the scope cannot be named up front.
   Delegated commits on the review track are reviewed afterward through lead-review.
   ```

   Rejected: keeping the touched-surface triggers ("public behavior / API /
   template"), because they route nearly every change in a shipped-text
   repository to a ticket. Rejected: a bare "if the edit scope is narrow and
   clear" judgment line, because the reader model takes the heavier path when a
   rule leaves it unsure (`skill-authoring.md` `## Reader Model`). The existing
   "delegate discovers broader scope -> stop and return" clause stays as the
   safety valve.
2. **Independent review is satisfied after the fact.** A delegated commit that
   lands on the project's declared review track is reviewed afterward by
   `lead-review` (review sweep or release gate); this counts as the independent
   review, so "independent review is needed" no longer forces `lead-run`. When
   the project declares no review track, the lead offers `lead-review` right
   after the delegate returns. Rejected: requiring review before dispatch.
3. **The brief is a write-once plan file.** For a code-writing delegate, the
   lead's brief is written to a repo-local plan at
   `ai-docs/.plans/YYYY-MM/DD-HHMM-<stem>.md`, allocated through the existing
   `path.generate(kind: "plan")` hook (`agents-plugin-tool/internal/wsstate/generated_paths.go`,
   `generatePlanPaths`). The plan is never edited after the delegate starts.
   Results and deviations go to the commit's `## AI Context`, never back into
   the plan. Rejected: a delegate-updated plan, because a living document
   re-creates the drift the refoundation retired.
4. **Allocation happens at `delegate-implementer` render time.** Rendering
   `delegate-implementer` allocates the plan path and splices it into the
   floor body, so the step cannot be skipped. The stem comes from the render
   `context`. Rejected: a lead-side prose instruction to call
   `path.generate` separately, because the lead can omit it.
5. **Only a top-level lead's render allocates a plan.** The condition is the
   session entry `scope == roleLead && parent == ""`, the same condition the
   mailbox peer-lead stamp uses (`handleMailboxSend`). A worker's key carries a
   parent, so leaf delegates rendered under `ticket-worker-elevated` (whose
   contract is the ticket) get no plan. Rejected: allocating on every render,
   which would leave one plan per leaf under a ticket.
6. **The floor tells the delegate to commit the plan with its implementation**
   in the same commit.
7. **No exception for NEEDS FIX repairs.** A local repair that `lead-review`
   hands to `lead-delegate` also gets a plan, so the next review reads what the
   repair intended the same way.
8. **`lead-review` reads the plans as the contract.** When the reviewed range
   contains plans, the reviewer judges the corresponding changes against them;
   commits without a plan are judged from the diff and its `## AI Context`.
   Plan absence never blocks a review.

9. **Render output and the lead's write.** Render reserves an empty plan file
   and returns its path as an additional output line next to the prompt path;
   the lead writes the brief into it with ordinary file tools. Rejected:
   passing the brief text to render as a `context` string, because render
   stays an allocator and a multi-paragraph brief does not belong in a
   substitution value.
10. **Missing stem.** A top-level-lead render without a stem in `context` falls
    back to the stem `delegate`. Rejected: refusing the render, which costs a
    retry turn over a naming detail; the timestamp already makes the path
    unique.
11. **How `lead-review` finds contracts.** Contracts are the `ai-docs/.plans/`
    files added in the reviewed range. Rejected: a new commit-message trailer
    naming the plan, because the plan lands in the same commit and the diff
    already identifies it.
12. **`lead-discuss` opening line.** Reframe its first sentence ("What the user
    confirms is captured through `ws:lead-ticket`") so ticket capture no longer
    reads as the default outcome of a discussion; bounded settled work routes
    to `lead-delegate` under the new gate. Rejected: leaving `lead-discuss`
    untouched, which keeps a ticket-first framing upstream of the gate.
    Rejected: having `lead-discuss` itself ask "implement now or write a
    ticket?" once a discussion converges, because it re-creates the premature
    wrap-up offer that the "Do not offer to wrap up or persist; wait for the
    user's signal" rule exists to stop; that rule stays.
13. **Canonical flow.** This repository's `AGENTS.md` `Canonical flows`
    `Delegate:` line becomes `delegate <bounded task> -> follow-up on the same
    native agent -> review`. `lead-bootstrap`'s `AGENTS.template.md` has no
    such line, so no shipped template changes.

## Constraints

- Shipped surfaces: read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  and `ai-docs/manuals/ws-mcp.md` before editing the paths they cover
  (`AGENTS.md` `### Implementation Conventions`).
- `ai-docs/.plans/` is revived only as a write-once record; no tool, playbook,
  or reviewer may edit an existing plan.
- The `lead-delegate` prose is pinned: update
  `agents-plugin/tests/fixtures/lead_delegate_contract.json`,
  `agents-plugin-tool/internal/mcp/lead_delegate_test.go`, and
  `agents-plugin/tests/test_skill_dispatch_contracts.py` with the change, and
  regenerate the wsflow and pi rsrc mirrors and the manifests
  (`260911-research-golden-fixture-verification-gap`; commit `de0c92ef4`
  once landed with the pinned fixture already red).

## Phases

### Phase 1: Delegate-default gate, render-time plan allocation, and plan-aware review

One phase: the floor text, the allocation, and the gate prose are one
contract, and neither half is useful alone.

- `playbook.render(name: "delegate-implementer")` from a top-level lead
  (Decision 5) allocates a plan through the existing plan-path generator
  (Decisions 3, 4), using the stem from `context` or `delegate` (Decision 10),
  splices the path into the floor body, and returns it as an extra output
  line (Decision 9). Worker-key renders are unchanged.
- `delegate-implementer` floor: when a plan path is present, the brief lives
  there; commit it in the same commit as the implementation, never edit it,
  and record results and deviations in `## AI Context` (Decisions 3, 6).
- `lead-delegate`: replace the routing gate with the cheap-default prose of
  Decision 1, the post-hoc review rule of Decision 2 (including the no-review-
  track offer), and the instruction to write the brief into the allocated
  plan before spawning. NEEDS FIX repairs follow the same path (Decision 7).
- `lead-review`: read plans added in the reviewed range as the contract for
  their changes; absence never blocks (Decisions 8, 11).
- `lead-discuss` opening line (Decision 12) and the repository `AGENTS.md`
  canonical flow line (Decision 13).

Verification: Go tests covering the render allocation (top-level lead gets a
plan path and floor splice; a worker child key gets neither; missing stem uses
`delegate`), the pinned prose fixtures and dispatch-contract tests from
`## Constraints`, the wsflow/pi mirror tests, and a real `playbook.render` of
`delegate-implementer` in both product modes.

