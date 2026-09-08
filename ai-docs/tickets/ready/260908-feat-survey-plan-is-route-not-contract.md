---
title: "Survey plan is a route, not the contract: the implementer reads the ticket, and reviewers judge the diff against it"
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - 260505-implementation-workflow-skills
  - 260619-stateless-implement-review-continuity
related:
  260908-feat-ws-pi-agent-session-disk-retention: the Pi-track incident that exposed this (child session files landed in tmpdir although the ticket said otherwise); lives on branch track/pi-agent, unlanded on develop
  260729-bug-survey-plan-drops-verbatim-contract-text: same lossy step seen on an inline contract; the inline paste rule here is its fix, and this ticket supersedes it
  260731-bug-implementer-ticket-result-read-authorization: second case where the implementer's ticket-read ban blocks required work; removing the ban here resolves it
  260727-refactor-implementer-delegate-shared-base: touches the same relay/elevated Constraints block; no ordering constraint, this ticket edits both files directly and the refactor absorbs the new wording
  260828-bug-reviewer-playbook-render-context-contract: same reviewer frame; no ordering constraint, this ticket only removes frame items and the remaining variable-contract mismatch stays with that ticket
related-mental-model:
  - workflow-skills
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8f664fa2d505c42e
sage-review-completeness-reviewed: 8f664fa2d505c42e
---

# Survey plan is a route, not the contract: the implementer reads the ticket, and reviewers judge the diff against it

## Background

Incident (Pi track, postmortem 2026-09-08; the tickets named in this
paragraph live on branch `track/pi-agent`, not on `develop`, so their text
is checkable there and in git history only). Ticket
`260902-feat-ws-pi-native-mvp` Phase 2 said child sessions use
"`--session <ws-owned-path>` (sibling of `~/.pi/agent/sessions/`, hidden
from the `/resume` picker)". The survey plan (`acc421c7`) restated this as
"ws-owned `sessionPath` (fresh temp path outside `~/.pi/agent/sessions/`)",
keeping the purpose clause and dropping the location. The implementer
followed the plan exactly, three partitioned reviewers found plan and diff
consistent, the ticket Result listed no deviation, and five later tickets
built durability (resume, sidecar revival, approval dir, cap eviction) on a
file living in the one directory the OS may delete.

The plan already cited the ticket range (`#L207-215`) next to its
restatement. A citation on its own did not stop the paraphrase; the
restatement is what the implementer read, because the implementer was not
allowed to open the ticket.

Where the current playbooks make the plan the contract (verified against
source 2026-09-08):

- `plan-populator-survey`: step "Clip the relevant contract" and the plan
  template's `## Relevant Ticket Contract` heading with its "clipped
  authority requirement" placeholder. The survey restates the contract in
  its own words, and that restatement is where the location was lost.
- `implementer`: "The plan and its listed references are the task
  contract" and "Do not read ticket files directly unless the plan's
  `Escalations` section explicitly authorizes ticket-file reading". The
  implementer cannot see the ticket, so a plan-level rewording is law.
  `implementer-relay` and `implementer-elevated` do not carry the "task
  contract" sentence; they share the ticket-read ban and most of their
  Constraints, disposition vocabulary, and process shape with each other
  (51 of 81 non-empty lines byte-identical per
  `260727-refactor-implementer-delegate-shared-base`). Their scope rule is
  "inside the scope defined by the plan, review findings, and disposition
  notes" and their `[escalate]` disposition sends the worker to the lead
  when ticket material is needed.
- `lead-implement` reviewer prompt frame: "Review the supplied authority,
  plan contract, and diff together" plus "Plan guardrails were not
  bypassed". Reviewers do receive the ticket as authority, and spec
  `{#260619-stateless-implement-review-continuity}` already says an
  unimplemented authority requirement is a blocking finding. Three
  reviewers held ticket, plan, and diff under that rule and still missed
  the location clause: with a plan in hand, plan-versus-diff is what a
  reviewer actually does.

The ticket-read ban was introduced in `c3d49a18` to keep the rendered
implementer prompt self-contained and the fresh worker's context free of
ticket noise (background, unsettled options, future phases). Code reviewers
and sage reviewers already read the whole ticket; the implementer is the
only role denied it. This ticket accepts the noise cost and removes the
ban.

## Decisions

- **The implementer reads the whole ticket.** The ticket-read ban in
  `implementer`, `implementer-relay`, and `implementer-elevated` is
  removed. For a ticket target the implementer reads the ticket file; the
  selected phase's text is the task contract, the ticket's `## Decisions`
  and `## Constraints` govern it, prior phases' `### Result` entries are
  context, and later phases are out of scope. `## Implementation Plan` is
  the recommended route. Where a plan step and the ticket disagree, the
  ticket wins and the implementer reports the disagreement instead of
  following the step. In `implementer-relay` and `implementer-elevated`
  the "needs ticket material" ground for `[escalate]` (and the relay's
  "escalate for a plan update if a required fix needs ticket material"
  step) goes away, since the worker may read the ticket; `[escalate]`
  stays for a plan deviation or a change the ticket itself would need.
  - Rejected: quoting the phase verbatim into the plan. It keeps a second
    copy that must be diffed against the ticket and still depends on the
    survey selecting the right lines.
  - Rejected: citing the ticket by line range and letting the implementer
    read only the cited ranges. It removes the copy but keeps a dependence
    on the survey's range selection, and line ranges drift when the ticket
    is edited (`260729-research-survey-plan-scope-and-plan-depth-signal`
    measured a 38-line drift in this repo); a heading anchor plus an
    escalation rule would have fixed the drift at the cost of one more
    mechanism. Reading the ticket needs none of it.
  - Rejected: a fourth reviewer on the plan. The pipeline already stacks
    reviewers and still missed; the fix is to stop making the plan
    authoritative, not to guard it harder.
- **The plan carries no contract text.** For a ticket target
  `## Relevant Ticket Contract` holds only the ticket path and the selected
  phase heading; the survey never restates, summarizes, or rewords ticket
  text. Survey judgment about the contract goes to `## Codebase Findings`
  or `## Escalations`. The heading name is kept to limit the test and
  playbook touch points.
- **Inline contracts are pasted verbatim.** An inline target has no ticket
  file, so `## Relevant Ticket Contract` holds the accepted inline contract
  character for character, never a summary. This is the fix for
  `260729-bug-survey-plan-drops-verbatim-contract-text`.
- **Reviewers do not receive the plan.** The plan (survey or research) is
  implementer input only. The reviewer prompt frame drops the `Plan path`
  line and the "Review the supplied authority, plan contract, and diff
  together" and "Plan guardrails were not bypassed" checks; every reviewer
  compares the diff with the authority (ticket or inline contract) and
  reports each specified requirement that is not implemented or that the
  diff contradicts. The existing "direct edit with no generated plan" frame
  becomes the only frame. Because the plan was the only artifact that
  scoped review to one phase, the frame gains a `Selected phase: <phase
  heading>` line for ticket targets (the same value the lead passes to the
  planner); reviewers judge the diff against that phase plus the ticket's
  `## Decisions` and `## Constraints`, and treat later phases as out of
  scope rather than as unimplemented requirements. Authorized deferrals
  and scope reductions are lead decisions: the lead writes them into the
  frame's `Review focus` lines, and a deferral that appears nowhere the
  reviewer can see is a finding. The Test partition checks the authority's verification
  expectations instead of the plan's `Verification Plan`; the Fit partition
  re-derives reuse points itself instead of reading `Codebase Findings`.
  - Rejected: keeping the plan as reviewer input with a "plan is the route"
    gloss. With a plan in hand, plan-versus-diff is what a reviewer does;
    removing the input removes the pattern instead of warning against it.
  - Rejected: one more line in the reviewer prompt frame asking to compare
    plan and ticket. An equivalent rule already existed and failed.
- **Result "Deviations" is ticket-diffed.** `executor-wrapup` asks for
  deviations as the difference between the ticket phase text and what
  landed, not as what the implementer recalls.

## Constraints

- Editing `agents-plugin/rsrc/` playbooks requires the rsrc manifest regen
  and the byte-identical `agents-plugin-wsflow/rsrc/` mirror regen, plus
  the Go golden tests that pin the implementer wording (search
  `agents-plugin-tool/internal/mcp/` for "The plan and its listed references
  are the task contract" and "Do not read ticket files directly").
- Spec addressing at ready promotion: anchors
  `{#260505-implementation-workflow-skills}` (the implementer "reads the
  plan and listed references as the task contract; does not read the
  ticket directly ... the lead updates the plan before ticket material is
  needed" sentences) and
  `{#260619-stateless-implement-review-continuity}` (the survey "clips the
  selected authority" sentence, the "plan is the implementer's sole
  context source ... clips the relevant ticket contract" paragraph, and the
  sentences that have reviewers compare "the selected authority, plan, and
  diff" and "read the ticket and plan"). Both describe the semantics this
  ticket changes.
- Not in scope: sage reviewers on tickets, review allocation counts, and
  `review-adjudicator` (it still declares `PlanPath`; the per-slice review
  loop does not invoke it, and its removal is tracked by
  `260831-chore-remove-orphaned-review-adjudicator-delegate`).
  `plan-populator-research` is in scope only for its own
  `## Relevant Ticket Contract` template block and its "preserve
  selected-authority contract ... as plan constraints" rule, which inherit
  the contract-free plan rule (Decision 2) when research refines a survey
  plan.
- The contract-free plan is deliberate: a later playbook diet or context
  budget pass must not reintroduce a contract summary in the plan, or the
  implementer ticket-read ban, on token grounds.

## Phases

### Phase 1: Contract-free survey plan and ticket-reading implementer

Decisions 1 to 3. Edit `plan-populator-survey` (the "Clip the relevant
contract" step, the plan template's `## Relevant Ticket Contract`
placeholder for ticket and inline targets), `plan-populator-research`
(its own `## Relevant Ticket Contract` template block and the
contract-preserving rule, same treatment), the `lead-implement` Plan
contract section that describes `Relevant Ticket Contract`, and the
ticket-read ban in `implementer`, `implementer-relay`, and
`implementer-elevated` (in `implementer` also the "task contract"
sentence and the ticket-file exception in the load-context and
escalate-gaps steps, which the other two do not carry; in the other two
the "needs ticket material" escalation grounds named in Decision 1).
`implementer` step 1 must gain a positive "read the ticket" instruction;
dropping "except ticket files" alone yields no read, since no plan
template emits the `[Must]` References list that step names. Update the pinned golden
tests, regen the rsrc manifest and wsflow mirror, and check
`ai-docs/manuals/skill-authoring.md` for invariant lines that name the
old wording. Verification: the plugin test suites that render these
playbooks pass, and a rendered survey plan for a ticket target names the
ticket path and phase heading with no restated ticket text.

### Phase 2: Reviewer frame without the plan, and wrap-up deviations

Decisions 4 and 5. Depends on Phase 1 (the implementer must already hold
the ticket when the reviewer stops receiving the plan). Edit `lead-implement`: the reviewer prompt frame (remove
`Plan path`, the two plan checks, and the separate "direct edit with no
generated plan" case, and add the `Review focus` rule for lead-authorized
deferrals, and add `Selected phase`), the Reviewer table's required
checks (Correctness drops "plan", Test reads the authority's verification
expectations, Fit drops "plan guardrails"), and
the reviewer dispatch text that passes a plan path. Edit `code-reviewer`
(the "when a plan path is named" constraint and the process step that
reads the plan) and the partition playbooks only where they name the plan.
Edit `executor-wrapup` for ticket-diffed deviations. Update the two spec
anchors named in Constraints. Verification: the golden reviewer-frame
render carries no plan line, the plugin test suites pass, and one
partitioned review on a ticket target completes with ticket plus diff
only.
