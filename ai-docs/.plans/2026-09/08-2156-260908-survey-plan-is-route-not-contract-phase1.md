# Plan: 260908-feat-survey-plan-is-route-not-contract — Phase 1: Contract-free survey plan and ticket-reading implementer

## Relevant Ticket Contract

- Decision 1 (implementer reads the whole ticket): for a ticket target the
  implementer (and `implementer-relay`/`implementer-elevated`) reads the
  ticket file; the selected phase text is the task contract, `## Decisions`
  and `## Constraints` govern it, prior `### Result` entries are context,
  later phases are out of scope. `## Implementation Plan` becomes the
  recommended route, not the contract. Where a plan step and the ticket
  disagree, the ticket wins and the implementer reports the disagreement
  instead of following the step. The relay/elevated "needs ticket material"
  `[escalate]` ground is removed (worker may read the ticket directly);
  `[escalate]` stays only for a plan deviation or a change the ticket itself
  would need. In the fix loop (all three delegates), scope wording that
  currently names "the plan" (scope boundary and won't-fix ground) is
  reworded to name "the selected phase" instead, since findings are now
  ticket-derived (Decision 4, Phase 2 — out of scope here, but this Phase 1
  wording rename is a prerequisite named by Decision 1 itself).
- Decision 2 (plan carries no contract text): for a ticket target,
  `## Relevant Ticket Contract` holds only the ticket path and the selected
  phase heading — the survey (and research) never restates, summarizes, or
  rewords ticket text. Survey judgment goes to `## Codebase Findings` or
  `## Escalations` (Decision 6). `## Out of Scope` for a ticket target names
  later-phase headings, adjacent ticket stems, and nearby concerns as
  pointers only, never as restated authority content. The six-section plan
  shape is unchanged (heading names kept as-is).
- Decision 3 (inline contracts pasted verbatim): unchanged from
  `260729-bug-survey-plan-drops-verbatim-contract-text` — an inline target's
  `## Relevant Ticket Contract` holds the accepted inline contract
  character-for-character, never a summary. Already implemented; verify it
  still holds after the Decision 2 edits land beside it in the same
  template blocks.
- Decision 6 (settled-vs-open escalation): a planner conclusion (survey or
  research) that narrows, inverts, or reframes something a ticket
  `## Decisions`/`## Constraints` entry already settled is an escalation,
  not a finding — it goes only into `## Escalations` with `[escalate-to-lead]`
  (never `[escalate-to-research]`, regardless of stated confidence), and
  never lands in `## Codebase Findings` or `## Implementation Plan` as a
  settled fact. `## Escalations` is never non-`None` under `[ok]`.
  Gap-filling (a conclusion addressing something the ticket does not
  address either way) is allowed and must be marked as such, distinct from
  a conclusion that changes something already settled. `lead-implement`
  gains a lead-adjudication window before implementer dispatch: rule on the
  entry, write the ruling under it in `## Escalations`, and continue; stop
  for the user only when resolving the entry would itself change the
  ticket. `agents-plugin-tool/internal/mcp/session_state.go`'s Prep todo
  instruction (survey case) currently names only `[escalate-to-research]`
  and must name `[escalate-to-lead]` beside it.
- Phase 1 file scope (from ticket `## Phases`): `plan-populator-survey`
  ("Clip the relevant contract" step + both template variants'
  `## Relevant Ticket Contract`/`## Out of Scope` placeholders, plus Rules/
  `## Escalations` text for Decision 6), `plan-populator-research` (its own
  `## Relevant Ticket Contract`/`## Out of Scope` template blocks, the
  contract-preserving rule, the "Plan must be self-contained" rule, the
  Doctrine "implement from the plan alone" line, the "Clip the relevant
  contract" step renamed to "name the under-specified clause", plus Rules/
  `## Escalations` text for Decision 6), `lead-implement` (the Plan contract
  section describing `Relevant Ticket Contract`, plus a new lead-adjudication
  rule before implementer dispatch), and `implementer`/`implementer-relay`/
  `implementer-elevated` (ticket-read ban removal; `implementer` additionally
  loses its "task contract"/"plan owns the decisions" lines, ticket-file
  Doctrine sentences, and the ticket-file exception in load-context/
  escalate-gaps steps; relay/elevated lose the "needs ticket material"
  escalation ground; all three rename "the plan" to "the selected phase" in
  fix-loop scope wording).
- Constraints: editing `agents-plugin/rsrc/` requires the rsrc manifest
  regen and the byte-identical `agents-plugin-wsflow/rsrc/` mirror regen,
  plus updating the Go golden tests that pin the implementer wording and the
  `session_state_test.go` todo-instruction pins. Spec `{#260505-implementation-workflow-skills}`
  sentences this phase falsifies (implementer read ban, survey-clips
  sentence, sole-context paragraph, `{#260512}` plan-carries-contract and
  implementer clauses) and all of the removed-rule sentence under
  `{#260519-proceed-implementation-dispatch-precheck}` must update; the
  Phase-2-owned reviewer sentences under the same headings and the
  `{#260619}` artifact-set sentence are explicitly deferred to Phase 2.
  `review-adjudicator`, sage reviewers on tickets, and review allocation
  counts are explicitly not in scope. The contract-free plan is deliberate:
  do not reintroduce a contract summary, the ticket-read ban, or a relaxed
  Decision 6 gate under a future token-budget pass.

## Out of Scope

- Phase 2 (Decisions 4-5): `lead-implement` reviewer prompt frame (drop
  `Plan path`/plan checks, add `Review focus`/`Selected phase`), Reviewer
  table check wording, `code-reviewer` plan constraint/process step,
  partition playbooks, `executor-wrapup` ticket-diffed deviations, and the
  Phase-2-owned spec/mental-model sentences (`{#260619}` artifact-set
  sentence, `{#260512}` reviewer clause, `{#260505}` reviewer-compare
  sentences).
- `review-adjudicator` (still declares `PlanPath`; removal tracked by
  `260831-chore-remove-orphaned-review-adjudicator-delegate`).
- Sage reviewers on tickets and review allocation counts.
- Any playbook-diet or context-budget change that would reintroduce a
  contract summary in the plan, the implementer ticket-read ban, or a
  relaxed settled-versus-open escalation gate.

## Codebase Findings

- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L57-58` —
  Process step 3 "Clip the relevant contract: requirements, non-goals,
  verification boundary, spec impact, and settled decisions..." must change
  to a contract-free framing (survey still needs to *understand* the
  contract for its own judgment; it must stop *writing a restatement into
  the plan*).
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L91-109`
  and `#L111-133` — both plan template variants' `## Relevant Ticket
  Contract` and `## Out of Scope` placeholder bullets currently read
  `<clipped authority requirement, decision, non-goal, or verification
  boundary>` / `<authority content, adjacent phase, or nearby concern
  intentionally excluded>`; both need the ticket-path-and-phase-heading-only
  (ticket target) vs. verbatim-inline-contract (inline target) discrimination
  written inline, since neither template branches on `target_kind`.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L32-49`
  (Rules) and the `## Escalations` blocks above — Decision 6's
  settled-vs-open rule, `[escalate-to-lead]` pairing, never-non-`None`
  invariant, and gap-fill marking need adding; the existing scope-reduction
  rule (`#L35-40`) already establishes the sibling pattern to mirror.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L29`
  — Rule "Preserve the selected authority's intent; do not re-derive or
  change settled decisions." interacts with Decision 6 (a reframing that
  reads as elaboration must still be caught).
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L58`
  — Process step 4 "Clip the relevant contract: ..." is the step the ticket
  names for the "name the under-specified clause" rewrite.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L88`
  — Draft step 2 "Preserve selected-authority contract and verification
  boundaries as plan guardrails instead of redefining them." is the
  "contract-preserving rule" the ticket says becomes route-only.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L33`
  — Rule "Plan must be self-contained: a fresh executor implements without
  re-researching." is the rule the ticket names to become route-only (the
  executor now also has the ticket).
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L123-129`
  — template's `## Relevant Ticket Contract`/`## Out of Scope` blocks need
  the same ticket-path/phase-only vs. verbatim-inline treatment as survey's.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L157-162`
  — Doctrine: "preserve the executor's ability to implement from the plan
  alone or escalate before execution starts" is the line the ticket names
  to become route-only.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L40-44`
  and `#L153-155` — Decision 6 additions mirror survey's Rules/`##
  Escalations` treatment; `[ok]` or `[escalate-to-lead]` vocabulary already
  exists here for the scope-reduction case, so it is again the sibling
  pattern to extend.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L178-183` — "### Plan
  contract" section states `Relevant Ticket Contract`/`Out of Scope`
  generically; needs the contract-free-for-ticket, verbatim-for-inline
  split stated explicitly.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L130-133` ("### 3.
  Prep") — no exit-signal/adjudication section exists today; this is the
  natural location (immediately before Edit/implementer-dispatch) for the
  new Decision 6 lead-adjudication-window rule.
- `agents-plugin/rsrc/implementer/implementer.md#L27-28` — "The plan and its
  listed references are the task contract." / "Do not re-research design
  alternatives; the plan owns the decisions." — both must be reworded; this
  exact first sentence is pinned verbatim by
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1328`.
- `agents-plugin/rsrc/implementer/implementer.md#L31` — "Do not read ticket
  files directly unless the plan's `Escalations` section explicitly
  authorizes ticket-file reading." must be replaced by a positive read
  instruction conditioned on `## Relevant Ticket Contract`'s content (ticket
  path present -> read it; verbatim inline contract present -> no ticket to
  read, since the playbook declares no `target_kind` variable). Pinned
  verbatim by `playbook_tools_test.go#L1330`.
- `agents-plugin/rsrc/implementer/implementer.md#L40-41` — Process step 1
  "Read the plan path above and all `[Must]` References listed in the plan
  except ticket files" and step 2's ticket-file escalation branch. Dropping
  "except ticket files" alone yields no read (no plan template emits a
  `[Must]` References list) — must add an explicit ticket-read step, not
  just remove the exclusion clause. Step 1 pinned verbatim by
  `playbook_tools_test.go#L1329`.
- `agents-plugin/rsrc/implementer/implementer.md#L68` — "suggestions that
  expand scope beyond the plan." renames to "...beyond the selected phase."
  per Decision 1's fix-loop wording rename.
- `agents-plugin/rsrc/implementer/implementer.md#L74-78` — Doctrine
  ("faithful contract execution", "The plan is the single source of truth")
  must be reworded now that the ticket, not the plan, is the source of
  truth for a ticket target.
- `agents-plugin/rsrc/implementer-relay/implementer-relay.md#L36` and `#L38`
  — "scope defined by the plan" / "scope expansion beyond the plan" rename
  to "the selected phase"; `#L38` pinned verbatim by
  `playbook_tools_test.go#L1387`.
- `agents-plugin/rsrc/implementer-relay/implementer-relay.md#L41` — ticket-
  read ban, pinned verbatim by `playbook_tools_test.go#L1388`; same
  treatment as `implementer`'s equivalent line.
- `agents-plugin/rsrc/implementer-relay/implementer-relay.md#L50` —
  "escalate for a plan update if a required fix needs ticket material or a
  plan deviation." loses the "needs ticket material" ground per Decision 1
  (keep "or a plan deviation"); pinned verbatim by
  `playbook_tools_test.go#L1389`.
- `agents-plugin/rsrc/implementer-elevated/implementer-elevated.md#L44`,
  `#L46`, `#L49` — same three renames as `implementer-relay` (scope wording,
  won't-fix ground, ticket-read ban). No exact-string golden pin exists for
  these three lines in
  `TestRenderPlaybookShippedImplementerElevatedDeclaredContext`
  (`playbook_tools_test.go#L1435-1501` checked; absent from its `want`
  list), so this file carries lower pin risk but must stay wording-consistent
  with `implementer-relay` per `260727-refactor-implementer-delegate-shared-base`'s
  byte-identical-line convention.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1302-1354`
  (`TestRenderPlaybookShippedImplementerDeclaredContext`) and `#L1356-1423`
  (`TestRenderPlaybookShippedImplementerRelayDeclaredContext`) — the `want`
  lists at the line numbers above must be updated to the new wording; the
  `forbidden` lists (`#L1341-1353`, `#L1408-1419`) should be reviewed for a
  matching old-wording decoy entry (the file's existing convention, e.g.
  `"scope expansion beyond the brief"` at `#L1413`).
- `agents-plugin-tool/internal/mcp/session_state.go#L534`
  (`implementPrepInstruction`, `"survey"` case) — currently names only
  `[escalate-to-research]`; must add `[escalate-to-lead]` beside it per the
  ticket's explicit Decision 6 instruction. Note: `#L555`
  (`implementEditInstruction`, `"survey"` case) also names only
  `[escalate-to-research]` and is a plausible companion site, but the ticket
  names only the "Prep todo instruction" — leave `#L555` alone unless the
  implementer finds a concrete inconsistency risk from touching only `#L534`.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L167` and `#L1994`
  — both pin the exact `#L534` string via `implementPrepGuardrails+"..."`
  concatenation; both must be updated identically. `#L171` pins the `#L555`
  edit-instruction string and needs no change if `#L555` is left alone.
  `#L1908` pins an unrelated `plan_depth` validation error string containing
  `[escalate-to-research]` and needs no change.
- `ai-docs/manuals/skill-authoring.md` — grepped for "task contract", "Do not
  read ticket", "plan owns the decisions", "sole context"; no hits. No edit
  needed here.
- `ai-docs/mental-model/mcp-runtime.md#L53` — "initial implementation and
  review-fix relay task contracts live in plan, review findings, diff, and
  disposition files" states the removed contract-in-plan model for initial
  implementation; needs a targeted reword (the review-fix relay half, which
  cites review findings/disposition files rather than the plan-as-contract,
  is unaffected).
- `ai-docs/mental-model/workflow-skills.md#L52` — "The implementer prompt
  treats the plan and listed references as the contract and blocks
  ticket-file reads unless the plan's `Escalations` section explicitly
  authorizes ticket-file reading." directly states the removed rule; needs
  rewording to the ticket-read model.
- `ai-docs/mental-model/prompt-bundle.md#L63` — "Survey clips the selected
  authority, writes the six-section light plan, and exits with
  `[escalate-to-research]` ... or `[escalate-to-lead]`" — the word "clips"
  states the exact behavior Decision 2 removes; this line was not named in
  the ticket's own Constraints list but is a real drift point the survey
  found by reading the file (task context named this file as one to
  consult). Needs rewording alongside the two ticket-named mental-model
  files, and should also gain a short mention of the Decision 6
  settled-vs-open gate since it already describes the escalate-to-lead
  channel in the same sentence.
- `ai-docs/spec/workflow-skills.md#L730-736` (`{#260512-skeleton-inside-implement-branch}`)
  — "the generated plan carries the relevant ticket contract ...
  Implementers treat the plan as the execution contract, and reviewers
  compare the ticket, plan, and diff together." Phase 1 updates the
  plan-carries-contract and implementer clauses only; the trailing
  "reviewers compare..." clause is Phase 2's.
- `ai-docs/spec/workflow-skills.md#L802-806` (under `{#260505-implementation-workflow-skills}`)
  — "The rendered implementer prompt reads the plan and listed references as
  the task contract; it does not read the ticket directly unless the plan's
  `Escalations` section explicitly authorizes ticket-file reading. Otherwise
  the lead updates the plan before ticket material is needed." — the
  implementer-read-ban sentence set named by the ticket.
- `ai-docs/spec/workflow-skills.md#L903` — "`plan-populator-survey` clips
  the selected authority, explores source, writes..." — the survey-clips
  sentence named by the ticket.
- `ai-docs/spec/workflow-skills.md#L939-942` — "The implementation plan is
  the implementer's sole context source, but it is not a lossy ticket
  summary. For the selected implementation scope, the plan clips the
  relevant ticket contract and records implementation strategy, codebase
  findings, verification expectations, escalations, and explicit out-of-scope
  boundaries. Ticket noise such as background discussion, unsettled options,
  and unrelated future phases is stripped." — the sole-context paragraph the
  ticket names; the following sentence at `#L944-947` ("In ticket-driven
  runs, reviewers read the ticket and plan...") is Phase 2's reviewer
  sentence and must be left alone.
- `ai-docs/spec/workflow-skills.md#L932-937` — "Before spawning the
  implementer, `lead-implement` handles plan-populator exit signals..."
  needs the Decision 6 lead-adjudication window folded in (this sentence is
  named in ticket Constraints as gaining "Decision 6's lead adjudication
  window", and Decision 6 is explicitly in Phase 1's scope).
- `ai-docs/spec/workflow-skills.md#L1112-1115` (under
  `{#260519-proceed-implementation-dispatch-precheck}`) — "Delegated
  implementers receive only the plan as task input, may read additional
  documents listed in the plan, and must not read the ticket directly
  unless the plan's `Escalations` section explicitly authorizes ticket-file
  reading." — the full removed-rule sentence the ticket names as "all of
  `{#260519}`".
- Manifest/mirror regen commands confirmed present and named correctly:
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  (regenerates `agents-plugin/rsrc/manifest.json`) and
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (regenerates the `agents-plugin-wsflow/rsrc/` mirror). All six touched
  files are currently byte-identical between `agents-plugin/rsrc/` and
  `agents-plugin-wsflow/rsrc/` (verified by diff), so the mirror regen is a
  straightforward copy-forward, not a divergence reconciliation.
  `WSRSRC_REGEN_SKILLS=1` is not needed — Phase 1 touches only
  `agents-plugin/rsrc/`, not `agents-plugin/skills/`.

## Implementation Plan

1. `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md`:
   reword the "Clip the relevant contract" step (`#L57-58`) to a
   contract-free framing; rewrite both template variants'
   `## Relevant Ticket Contract`/`## Out of Scope` placeholders
   (`#L91-109`, `#L111-133`) to hold only ticket-path+phase-heading (ticket
   target) or verbatim inline contract (inline target), written as an
   inline discriminator since the playbook has no `target_kind` branch; add
   the Decision 6 settled-vs-open rule to Rules and both `## Escalations`
   blocks, mirroring the existing scope-reduction rule's `[escalate-to-lead]`
   pairing and never-non-`None` invariant, plus the gap-fill marking.
2. `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md`:
   rename step 4 "Clip the relevant contract" (`#L58`) to "name the
   under-specified clause"; reword the contract-preserving rule (`#L88`) and
   the "Plan must be self-contained" rule (`#L33`) to route-only framing;
   reword the Doctrine "implement from the plan alone" clause (`#L157-162`);
   apply the same `## Relevant Ticket Contract`/`## Out of Scope` template
   rewrite as survey (`#L123-129`); add the same Decision 6 rule to Rules
   and `## Escalations` (`#L40-44`, `#L153-155`).
3. `agents-plugin/rsrc/lead-implement/lead-implement.md`: rewrite the "###
   Plan contract" section (`#L178-183`) to state the contract-free-for-ticket
   / verbatim-for-inline split; add a new Decision 6 lead-adjudication-window
   rule before implementer dispatch (natural location: end of "### 3. Prep",
   `#L130-133`) — rule on a settled-vs-open entry, write the ruling under it
   in the plan's `## Escalations`, continue; stop for the user only when
   resolving it would change the ticket.
4. `agents-plugin/rsrc/implementer/implementer.md`: remove/reword the "task
   contract" sentence and "plan owns the decisions" line (`#L27-28`);
   replace the ticket-file ban (`#L31`) with a positive read instruction
   conditioned on `## Relevant Ticket Contract` content; update Process step
   1 to actually read the ticket file when named (not just drop "except
   ticket files", per the finding above) and remove step 2's ticket-file
   escalation branch (`#L40-41`); rename "beyond the plan" to "beyond the
   selected phase" (`#L68`); reword the Doctrine paragraph (`#L74-78`) so the
   ticket, not the plan, is the source of truth.
5. `agents-plugin/rsrc/implementer-relay/implementer-relay.md`: rename
   "scope defined by the plan" / "scope expansion beyond the plan" to "the
   selected phase" (`#L36`, `#L38`); replace the ticket-file ban (`#L41`)
   with the same positive read instruction as `implementer`; drop "needs
   ticket material" from the escalation ground (`#L50`), keeping "or a plan
   deviation".
6. `agents-plugin/rsrc/implementer-elevated/implementer-elevated.md`: apply
   the identical three renames as step 5 (`#L44`, `#L46`, `#L49`), keeping
   wording byte-identical to `implementer-relay` where the two files already
   share lines (per `260727-refactor-implementer-delegate-shared-base`).
7. Regen `agents-plugin/rsrc/manifest.json`:
   `cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`.
8. Regen the wsflow mirror:
   `cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
9. Update the Go golden tests in
   `agents-plugin-tool/internal/mcp/playbook_tools_test.go`: the `want`
   lists in `TestRenderPlaybookShippedImplementerDeclaredContext`
   (`#L1323-1336`) and `TestRenderPlaybookShippedImplementerRelayDeclaredContext`
   (`#L1377-1400`) to the new wording; review the `forbidden` lists
   (`#L1341-1353`, `#L1408-1419`) and add an old-wording decoy entry
   matching the file's existing convention.
10. `agents-plugin-tool/internal/mcp/session_state.go#L534`: extend the
    `"survey"` case in `implementPrepInstruction` to name
    `[escalate-to-lead]` beside `[escalate-to-research]`. Update the two
    pinned exact-match assertions in
    `agents-plugin-tool/internal/mcp/session_state_test.go` (`#L167`,
    `#L1994`) to match.
11. Update mental-model drift: `ai-docs/mental-model/mcp-runtime.md#L53`,
    `ai-docs/mental-model/workflow-skills.md#L52`, and
    `ai-docs/mental-model/prompt-bundle.md#L63` to state the ticket-read
    model and contract-free plan instead of the removed plan-as-contract/
    ticket-read-ban model; fold in a short Decision 6 mention where
    `prompt-bundle.md#L63` already names the escalation channels.
12. Update `ai-docs/spec/workflow-skills.md`: the plan-carries-contract and
    implementer clauses at `#L730-736` (leave the trailing reviewer clause
    for Phase 2); the implementer-read-ban sentences at `#L802-806`; the
    survey-clips sentence at `#L903`; the sole-context paragraph at
    `#L939-942` (leave `#L944-947`'s reviewer sentence for Phase 2); the
    exit-signals sentence at `#L932-937` to add the Decision 6 lead
    adjudication window; and the full removed-rule sentence at `#L1112-1115`
    under `{#260519-proceed-implementation-dispatch-precheck}`.
13. Commit playbook edits, manifest/mirror regen, Go test updates, and doc
    updates as logical checkpoints per repository commit conventions (the
    doctrine `## AI Context` should record why the plan-as-contract model
    was removed, citing the Pi-track incident).

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/wsrsrc/...` (manifest and
  wsflow-mirror byte-identity checks pass after regen)
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TestRenderPlaybookShippedImplementer|TestRenderPlaybookFullWsPlannerContext|TestRenderPlaybookShippedReviewAdjudicator|TestRenderPlaybookWsflow'`
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TestDeriveImplementTodoInstructions'`
- `cd agents-plugin-tool && go test ./...` (full suite, since manifest/spec
  drift checks and other golden renders can be sensitive to rsrc edits)
- Manual: render `plan-populator-survey` for a ticket target and confirm
  `## Relevant Ticket Contract` names only the ticket path and phase
  heading with no restated ticket text; render `implementer` for a ticket
  target and confirm it is instructed to read the ticket file; confirm both
  rendered playbooks carry the Decision 6 escalation rule with its
  `[escalate-to-lead]` pairing; confirm rendered `lead-implement` carries
  the adjudication window text.
- `claude plugin validate agents-plugin` (per repo orientation, this
  currently passes and should continue to)

## Escalations

- None.
