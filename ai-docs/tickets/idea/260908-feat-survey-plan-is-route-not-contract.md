---
title: "Survey plan is a route, not the contract: cite the ticket by line range, implement to the cited text, review the diff against the ticket"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260908-feat-ws-pi-agent-session-disk-retention: the Pi-track incident that exposed this (child session files landed in tmpdir although the ticket said otherwise)
  260729-bug-survey-plan-drops-verbatim-contract-text: same lossy step seen on an inline contract; the inline paste rule here is its fix, and this ticket supersedes it
  260731-bug-implementer-ticket-result-read-authorization: second case where the implementer's ticket-read ban blocks required work; the cited-range rule here replaces that ban
related-mental-model:
  - workflow-skills
---

# Survey plan is a route, not the contract: cite the ticket by line range, implement to the cited text, review the diff against the ticket

## Background

Incident (Pi track, postmortem 2026-09-08). Ticket
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
restatement is what the implementer read.

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
  `implementer-relay` and `implementer-elevated` share only the ticket-read
  ban; their scope rule is "inside the scope defined by the plan, review
  findings, and disposition notes" and their `[escalate]` disposition sends
  the worker to the lead when ticket material is needed.
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
ticket noise (background, unsettled options, future phases). That purpose
survives this ticket; the mechanism changes.

## Decisions

- **The plan carries no contract text.** `## Relevant Ticket Contract`
  becomes a citation list: ticket path plus line ranges. The selected
  phase's full text range is mandatory; the `## Decisions` and
  `## Constraints` lines that govern the phase are cited as further ranges.
  The survey never restates, summarizes, or rewords cited text. Survey
  judgment about the contract goes to `## Codebase Findings` or
  `## Escalations`.
  - Rejected: quoting the phase verbatim into the plan. It removes the
    rewording loss but keeps a second copy that the reviewer must diff
    against the ticket, costs plan length, and does nothing the citation
    does not do once the implementer may read the cited ranges.
  - Rejected: a fourth reviewer on the plan. The pipeline already stacks
    reviewers and still missed; the fix is to stop making the plan
    authoritative, not to guard it harder.
  - Rejected: one more line in the reviewer prompt frame asking to compare
    plan and ticket. An equivalent rule already existed and failed; with
    no contract text in the plan there is nothing to compare.
- **The implementer reads the cited ranges.** The ticket-read ban in
  `implementer`, `implementer-relay`, and `implementer-elevated` becomes:
  read the ticket ranges the plan cites; do not read the rest of the
  ticket unless the plan's `Escalations` section authorizes it. The cited
  text is the task contract; `## Implementation Plan` is the recommended
  route. Where a step and the cited text disagree, the cited text wins and
  the implementer reports the disagreement instead of following the step.
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
  becomes the only frame. Authorized deferrals and scope reductions are
  lead decisions: the lead writes them into the frame's `Review focus`
  lines, and a deferral that appears nowhere the reviewer can see is a
  finding. The Test partition checks the authority's verification
  expectations instead of the plan's `Verification Plan`; the Fit partition
  re-derives reuse points itself instead of reading `Codebase Findings`.
  - Rejected: keeping the plan as reviewer input with a "plan is the route"
    gloss. With a plan in hand, plan-versus-diff is what a reviewer does;
    removing the input removes the pattern instead of warning against it.
- **Result "Deviations" is ticket-diffed.** `executor-wrapup` asks for
  deviations as the difference between the cited ticket text and what
  landed, not as what the implementer recalls.

## Constraints

- Residual loss stays in range selection: the survey still chooses which
  `## Decisions` and `## Constraints` lines govern the phase, so an omitted
  governing line is not caught by the citation itself. The mandatory
  whole-phase range removes omission inside the phase; reviewers reading
  the ticket remain the guard for omitted governing lines.
- Editing `agents-plugin/rsrc/` playbooks requires the rsrc manifest regen
  and the byte-identical `agents-plugin-wsflow/rsrc/` mirror regen, plus
  the Go golden tests that pin the implementer wording (search
  `agents-plugin-tool/internal/mcp/` for "The plan and its listed references
  are the task contract" and "Do not read ticket files directly").
- Spec addressing at ready promotion: anchors
  `{#260505-implementation-workflow-skills}` (the implementer "reads the
  plan and listed references as the task contract; does not read the
  ticket directly" sentences) and
  `{#260619-stateless-implement-review-continuity}` (the survey "clips the
  selected authority" sentence, the "plan is the implementer's sole
  context source ... clips the relevant ticket contract" paragraph, and the
  sentences that have reviewers compare "the selected authority, plan, and
  diff" and "read the ticket and plan"). Both describe the semantics this
  ticket changes.
- Not in scope: sage reviewers on tickets, review allocation counts, and
  `plan-populator-research` beyond inheriting the citation rule when it
  refines a survey plan.
- Citation is a deliberate duplication-free shape: a later playbook diet
  must not reintroduce a contract summary in the plan on token grounds.

## Phases

### Phase 1: Survey cites and the implementer reads the cited ranges

Decisions 1 to 3. Edit `plan-populator-survey` (the clip step, the plan
template heading and placeholder, the inline paste rule), the
`lead-implement` Plan contract section that describes
`Relevant Ticket Contract`, and the ticket-read rule plus contract
sentence in `implementer`, `implementer-relay`, `implementer-elevated`.
Update the pinned golden tests, regen the rsrc manifest and wsflow mirror,
and check `ai-docs/manuals/skill-authoring.md` for invariant lines that
name the old wording. Verification: the plugin test suites that render
these playbooks pass, and a rendered survey plan for a ticket target
contains a citation list and no restated contract text.

### Phase 2: Reviewer frame without the plan, and wrap-up deviations

Decisions 4 and 5. Depends on Phase 1 (deviations are stated against the
cited text). Edit `lead-implement`: the reviewer prompt frame (remove
`Plan path`, the two plan checks, and the separate "direct edit with no
generated plan" case, and add the `Review focus` rule for lead-authorized
deferrals), the Reviewer table's required checks (Test reads the
authority's verification expectations, Fit drops "plan guardrails"), and
the reviewer dispatch text that passes a plan path. Edit `code-reviewer`
(the "when a plan path is named" constraint and the process step that
reads the plan) and the partition playbooks only where they name the plan.
Edit `executor-wrapup` for ticket-diffed deviations. Update the two spec
anchors named in Constraints. Verification: the golden reviewer-frame
render carries no plan line, the plugin test suites pass, and one
partitioned review on a ticket target completes with ticket plus diff
only.
