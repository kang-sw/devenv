---
title: "Survey plan is a route, not the contract: the implementer reads the ticket, and reviewers judge the diff against it"
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - 260505-implementation-workflow-skills
  - 260619-stateless-implement-review-continuity
  - 260512-skeleton-inside-implement-branch
  - 260519-proceed-implementation-dispatch-precheck
related:
  260908-feat-ws-pi-agent-session-disk-retention: the Pi-track incident that exposed this (child session files landed in tmpdir although the ticket said otherwise); lives on branch track/pi-agent, unlanded on develop
  260729-bug-survey-plan-drops-verbatim-contract-text: same lossy step seen on an inline contract; the inline paste rule here is its fix, and this ticket supersedes it
  260731-bug-implementer-ticket-result-read-authorization: second case where the implementer's ticket-read ban blocks required work; removing the ban here resolves it
  260727-refactor-implementer-delegate-shared-base: touches the same relay/elevated Constraints block; no ordering constraint, this ticket edits both files directly and the refactor absorbs the new wording
  260828-bug-reviewer-playbook-render-context-contract: same reviewer frame; no ordering constraint, this ticket only removes frame items and the remaining variable-contract mismatch stays with that ticket
  260906-bug-route-opaque-params-handler-mismatch: the reframing-as-finding incident behind Decision 6; the Phase 2 plan of 260904 inverted its Decision C inside Codebase Findings and shipped this regression
related-mental-model:
  - workflow-skills
  - mcp-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cde688d02a18c5a2
sage-review-completeness-reviewed: cde688d02a18c5a2
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

Audit of the eight September plans under `ai-docs/.plans/2026-09/` against
their tickets (2026-09-08). Every value a ticket had frozen as a literal (a
rename table, a signature block, a quoted error string, a `{#slug}`, a
filename grammar, a numeric threshold) reached the plan verbatim; the only
paraphrase losses were rationale sentences that drive no edit. The one
shipped defect came from a different move: the `260904` Phase 2 plan
restated Decision C faithfully in `## Relevant Ticket Contract`, then
concluded in `## Codebase Findings`, marked "high confidence, not
escalating", that the published `params: object` was a placeholder real
callers never send. The implementer followed it and the wrapped call path
regressed into `260906-bug-route-opaque-params-handler-mismatch`. The
`260907` plan put its own placeholder-pruning default in the same section
labeled "the ticket's prose does not address either way" and was safe.
The section did not separate the two cases; whether the conclusion changed
a settled decision did. Decision 6 closes that move. No research plan
exists in the corpus to sample (survey has not exited to research since
sage review landed), so the research changes here are preventive.

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
  The same holds in the fix loop: fixes stay inside the selected phase,
  review findings, and disposition notes, and won't-fix is available for
  a finding that expands scope beyond the selected phase, never for one
  that merely exceeds the plan's route, since under Decision 4 findings
  are ticket-derived.
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
  text. Survey judgment goes to `## Codebase Findings` or `## Escalations`
  under Decision 6. The heading name is kept to limit the test and
  playbook touch points. `## Out of Scope` follows the same rule for a
  ticket target: it names later phase headings, adjacent ticket stems, and
  nearby concerns as pointers, never as restated authority content, and
  the six-section plan shape stays so the section-presence pins hold.
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
- **A planner conclusion that changes what the ticket settled is an
  escalation, not a finding.** Both planners decide freely inside the space
  the ticket leaves open (survey: reuse points and the route; research: the
  concrete algorithm, structures, and wiring for a clause the ticket left
  under-specified) and mark each such choice as the plan's own, so the
  implementer can tell what it may leave with a note in the Result from
  what needs escalation. A conclusion that narrows, inverts, or reframes
  what a ticket `## Decisions` or `## Constraints` entry already settled
  goes to `## Escalations`, and the planner reports `[escalate-to-lead]`
  instead of `[ok]`, regardless of stated confidence, mirroring the
  scope-reduction rule both planners already carry; `## Escalations` is
  never non-`None` under `[ok]`. Survey does not route this to
  `[escalate-to-research]`: research would remake the same call at a
  higher tier without the ticket's authority. The lead adjudicates the
  entry with the ticket in hand and keeps it away from the user while the
  ticket text stays as it is: it relabels a mislabeled gap-fill, or rules
  that the ticket wins and the planner misread it, writes the ruling under
  the entry in `## Escalations`, and continues. It stops for the user only
  when resolving the entry would change what the ticket settled, since
  that is a ticket edit. The conclusion never lands in
  `## Codebase Findings` or `## Implementation Plan` as a settled fact.
  Gap-filling is allowed only when marked as something the ticket does not
  address. In `plan-populator-research` the "Clip the relevant contract"
  step becomes "name the under-specified clause": the plan cites the ticket
  heading(s) it is filling in and never restates the contract around them.
  - Rejected: a flat "do not reinterpret" rule for research. Filling the
    open how is research's job; the boundary is settled-versus-open, not
    interpret-versus-not.
  - Rejected: relying on Decision 1 alone (the implementer holds the
    ticket) to catch it. A reframing that reads as elaboration does not
    trip "plan and ticket disagree", and the higher planner tier makes the
    implementer more likely to defer to it, not less. Three reviewers
    holding the ticket missed exactly this case.
  - Rejected: a new exit token for contract conflicts. It adds vocabulary
    to both planners, `lead-implement`, and the spec for the same gate
    `[escalate-to-lead]` already opens.
  - Rejected: a user stop on every such entry. Most entries are
    misreadings or mislabeled gap-fills the lead can rule on with the
    ticket in hand; a user stop is reserved for the case where the ticket
    itself must change.

## Constraints

- Editing `agents-plugin/rsrc/` playbooks requires the rsrc manifest regen
  and the byte-identical `agents-plugin-wsflow/rsrc/` mirror regen, plus
  the Go golden tests that pin the implementer wording (search
  `agents-plugin-tool/internal/mcp/` for "The plan and its listed references
  are the task contract" and "Do not read ticket files directly") and
  the todo-instruction pins in `session_state_test.go`.
- Spec addressing at ready promotion, both in
  `ai-docs/spec/workflow-skills.md`. Under
  `{#260505-implementation-workflow-skills}` (the section heading; the
  `{#260619}` trailing marker closes earlier, so these sentences sit after
  it): the implementer "reads the plan and listed references as the task
  contract; does not read the ticket directly ... the lead updates the
  plan before ticket material is needed" sentences, the
  "`plan-populator-survey` clips the selected authority" sentence, the
  "plan is the implementer's sole context source ... clips the relevant
  ticket contract" paragraph, and the sentences that have reviewers
  compare "the selected authority, plan, and diff" and "read the ticket
  and plan", and the "handles plan-populator exit signals ... stops and
  escalates" sentence, which gains Decision 6's lead adjudication window.
  Under `{#260619-stateless-implement-review-continuity}`: the
  sentence that says each implementer and reviewer dispatch "is fed
  entirely by its relay prompt plus the self-contained artifact set (plan,
  review findings, committed diff)", which Decision 4 falsifies for
  reviewers. Two more entries in the same file state the removed rules:
  `{#260512-skeleton-inside-implement-branch}` ("the generated plan
  carries the relevant ticket contract ... Implementers treat the plan as
  the execution contract, and reviewers compare the ticket, plan, and
  diff together") and `{#260519-proceed-implementation-dispatch-precheck}`
  ("Delegated implementers receive only the plan as task input ... must
  not read the ticket directly unless the plan's `Escalations` section
  explicitly authorizes ticket-file reading"). Phase 1 updates the
  sentences its own change falsifies (implementer read ban, survey clips,
  sole-context paragraph, the `{#260512}` plan-carries-contract and
  implementer clauses, all of `{#260519}`); Phase 2 updates the reviewer
  sentences, the `{#260512}` reviewer clause, and the `{#260619}`
  artifact-set sentence.
- Not in scope: sage reviewers on tickets, review allocation counts, and
  `review-adjudicator` (it still declares `PlanPath`; the per-slice review
  loop does not invoke it, and its removal is tracked by
  `260831-chore-remove-orphaned-review-adjudicator-delegate`).
  `plan-populator-research` is in scope for its own
  `## Relevant Ticket Contract` template block, its "preserve
  selected-authority contract ... as plan constraints" rule, its process
  step "Clip the relevant contract", its Rules/`## Escalations` text,
  and its "Plan must be self-contained" rule plus the Doctrine line about
  implementing from the plan alone, which become route-only statements.
  The first two inherit the contract-free plan rule (Decision 2); the clip
  step and the escalation text carry Decision 6. Research refines the survey
  plan in place, so a surviving clip step would re-inject restated contract
  text into a plan Decision 2 had just emptied.
- The contract-free plan is deliberate: a later playbook diet or context
  budget pass must not reintroduce a contract summary in the plan, the
  implementer ticket-read ban, or a relaxed settled-versus-open escalation
  gate (Decision 6), on token grounds.

## Phases

### Phase 1: Contract-free survey plan and ticket-reading implementer

Decisions 1 to 3 and 6. Edit `plan-populator-survey` (the "Clip the relevant
contract" step, and the `## Relevant Ticket Contract` and
`## Out of Scope` placeholders in both of its plan template variants,
survey-sufficient and research-needed; neither variant branches on
target kind, so the ticket/inline discrimination is written inside each
placeholder), `plan-populator-research` (its own
`## Relevant Ticket Contract` and `## Out of Scope` template blocks, the
contract-preserving rule, the "Plan must be self-contained" rule, and
the Doctrine line about implementing from the plan alone, same
treatment), the `lead-implement` Plan
contract section that describes `Relevant Ticket Contract`, and the
ticket-read ban in `implementer`, `implementer-relay`, and
`implementer-elevated` (in `implementer` also the "task contract"
sentence, the "plan owns the decisions" line, the Doctrine sentences
that make the plan the single source of truth and resolve ambiguity
toward it, and the ticket-file exception in the load-context and
escalate-gaps steps, which the other two do not carry; in the other two
the "needs ticket material" escalation grounds named in Decision 1; in
all three the fix-loop scope constraint "scope defined by the plan,
review findings, and disposition notes" and the won't-fix ground "scope
expansion beyond the plan", which name the selected phase instead of
the plan).
For Decision 6, add the settled-versus-open escalation rule, its
`[escalate-to-lead]` pairing, the never-non-`None`-under-`[ok]`
invariant, and the plan's-own-choice marking to the Rules and
`## Escalations` text of both `plan-populator-survey` and
`plan-populator-research`; turn the research process step "Clip the
relevant contract" into "name the under-specified clause"; and add the
lead adjudication window as a new rule in `lead-implement` before
implementer dispatch (the playbook has no exit-signal section today:
rule on the entry, write the ruling under it, continue; stop for the
user only when the ticket must change), and extend the Go-owned Prep
todo instruction in `agents-plugin-tool/internal/mcp/session_state.go`,
which names only `[escalate-to-research]`, to name `[escalate-to-lead]`
beside it so runbook and playbook agree.
`implementer` step 1 must gain a positive read instruction conditioned
on the plan's `## Relevant Ticket Contract`: when it names a ticket path,
read that ticket file; when it holds a verbatim inline contract, that
text is the contract and there is no ticket to read (the playbook
declares no `target_kind` variable, so the section content is the only
discriminator; the same conditional goes into `implementer-relay` and
`implementer-elevated`). Dropping "except ticket files" alone yields no
read, since no plan template emits the `[Must]` References list that
step names. Update the pinned golden tests, regen the rsrc manifest and
wsflow mirror, check `ai-docs/manuals/skill-authoring.md` for invariant
lines that name the old wording, update the mental-model lines that
state the removed contract (`ai-docs/mental-model/mcp-runtime.md` on task
contracts living in plan files; `ai-docs/mental-model/workflow-skills.md`
on the ticket-read ban and plan decision preservation), and update the
`{#260505-implementation-workflow-skills}` sentences this phase falsifies
(named in Constraints) so shipped playbooks and spec do not disagree
between phases. Verification: the plugin test suites that render these
playbooks pass, and a rendered survey plan for a ticket target names the
ticket path and phase heading with no restated ticket text, and the
rendered survey and research playbooks each carry the settled-versus-open
escalation rule with its `[escalate-to-lead]` pairing and the
plan's-own-choice marking, and the rendered `lead-implement` carries the
adjudication window.

### Phase 2: Reviewer frame without the plan, and wrap-up deviations

Decisions 4 and 5. Depends on Phase 1 (the implementer must already hold
the ticket when the reviewer stops receiving the plan). Edit `lead-implement`: the reviewer prompt frame (remove
`Plan path`, the two plan checks, and the separate "direct edit with no
generated plan" case, and add the `Review focus` rule for lead-authorized
deferrals, and add `Selected phase`), the Reviewer table's required
checks (Correctness drops "plan", Test reads the authority's verification
expectations, Fit drops "plan guardrails" and keeps "future-phase fit"
as a compatibility consideration only, never as unimplemented
requirements, per Decision 4), and
the reviewer dispatch text that passes a plan path. Edit `code-reviewer`
(the "when a plan path is named" constraint and the process step that
reads the plan) and the partition playbooks only where they name the plan.
Edit `executor-wrapup` for ticket-diffed deviations. Update the reviewer
sentences under `{#260505-implementation-workflow-skills}` and the
artifact-set sentence under `{#260619-stateless-implement-review-continuity}`
named in Constraints. Verification: the golden reviewer-frame
render carries no plan line, the plugin test suites pass, and one
partitioned review on a ticket target completes with ticket plus diff
only.

## Blocked (2026-09-08)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Decision 6 names a plan section but no exit signal, so the rule may never gate anything | important | missing |
| 2 | ## Out of Scope still restates ticket text, contradicting Decision 2 and Phase 1's own verification | important | autonomous |
| 3 | Constraints attribute three spec sentences to the wrong anchor and miss the one {#260619} sentence Decision 4 invalidates | minor | autonomous |
| 4 | Both spec anchors are updated in Phase 2 although Phase 1 changes what they state | minor | autonomous |
| 5 | The implementer's new read-the-ticket step is unconditional as written, but inline targets have no ticket | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
