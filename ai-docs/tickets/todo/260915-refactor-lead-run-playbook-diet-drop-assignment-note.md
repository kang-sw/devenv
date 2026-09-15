---
title: "Diet the lead-run playbook to the authoring standard, drop the session.note assignment record, and add ticket-batch-selector"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-research-ws-refoundation-evidence-audit: context; binding anchor for the lead surface and stop conditions this ticket edits
  260915-refactor-lead-run-dispatch-time-tier-judgment: context (done); landed the Risk Rubric include and the dispatch-time tier read this rewrite keeps
  260915-refactor-lead-run-defer-phase-merge: context (done); landed the note-keyed continue detection this ticket replaces with the selector's git-derived read
  260910-feat-lead-run-worktree-parallel-route: context (done); landed the parallel route this ticket collapses to its delta and whose batch selection moves into ticket-batch-selector
  260915-bug-ws-route-resolve-implement-branch-handling-random-codename: open premise; the parallel route's pre-provisioned impl-branch name cannot be derived by the lead, and this ticket carries that route's provisioning text over unchanged rather than fixing it
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 701b5d9c91b05018
sage-review-design-reviewed: 701b5d9c91b05018
---

# Diet the lead-run playbook to the authoring standard, drop the session.note assignment record, and add ticket-batch-selector

## Background

`agents-plugin/rsrc/lead-run/lead-run.md` does five things: pick a ticket
(one, or a batch on the opt-in parallel route), read it, spawn a worker at a
graded tier, hand it the task block, and act on the worker's terminal report.
The file is 307 lines plus the 52-line `risk-rubric` include. Its history
explains the size: it was 122 lines at its only fresh-reader audit
(`0a9ee632`, 2026-09-09; 32 findings, 3 fixed, 29 recorded as accepted) and
grew to 307 by 2026-09-15 through 32 commits since that audit — 14 of them
fix-type, 6 scoped exactly `fix(lead-run)` (`git log --oneline
0a9ee632..HEAD -- agents-plugin/rsrc/lead-run/lead-run.md`) — each adding a
paragraph for one observed failure and none compressing first.
`ai-docs/manuals/skill-authoring.md` names this pattern (rules kept as
insurance, gates added on friction, Layer 1/2 restatement) as what its audit
flags.

Measured against that manual at `19cecb1c`, the file carries:

- Layer 1/2 restatement: the release-target acknowledgement, inspected-OID
  retry, changed-tip re-acknowledgement, and "conflict goes to lead-delegate"
  rules in **Handle the report** repeat the Diagnostics, Resolution, and
  Advisory strings `git_merge.go` already emits; the `dispatch_blocked`
  explanation in **Spawn** repeats the `tickets.query` schema description; the
  `worktree.acquire` and `root_override` behavior in the parallel route repeat
  their schema descriptions; the impl-base derivation rule repeats the
  `git.merge` `target` description.
- A tool-defect workaround written as prose: ten lines explaining how to
  recover the render-minted worker key from `session.children(scope:
  "control", unnoted_only: true)` because `playbook.render` returns no key.
  The only consumer of that key is the `session.note` write.
- Duplicates: the "impl branch is deterministic so `route.resolve_implement`
  returns `continue`" rationale three times; the merge-approval gate three
  times; "wait for the host notification, never poll" twice; the assignment
  note's state transitions three times; the risk grading in both Spawn and
  the parallel route.
- A 67-line parallel route that restates the serial path with six actual
  differences.
- Over-negation and defensive disclaimers the manual's audit lists: "not a
  license to browse the queue", "never infer this approval from a goal run, a
  full queue, or convenience", the four-item "do not" list on a protocol
  mismatch (the worker protocol already says "fail closed"), "it is not a
  progress board", and the xlarge-is-proactive gloss the included rubric
  already states.

The `session.note` assignment record has exactly one writer and one reader,
both in `lead-run.md` (Spawn step 4 writes; Select reads it for continue
detection). `ws:lead-revive` reloads `workflow_manual` only and never reads
`session.children`, so the "carry-over record a compacted lead rebuilds from"
rationale has no consumer on the recovery path. Every state the note holds is
derivable from ground truth: `active` is the checked-out impl branch that
`git.status` already reports as `impl_ticket` (and that `ticket-selector`
already selects first); `merged` is the branch `git.merge` deleted; `blocked`
is the `## Blocked (YYYY-MM-DD)` note **End the turn** already writes on the
ticket; the parallel `worker_key` is returned by `worktree.acquire` and listed
with its root by `session.children` without a note. The note induced, in
sequence, the key-recovery prose, the `session.children` scope and
`unnoted_only` filters (`00f8f976`), one dogfood bug ticket and its sibling
chore ticket (`260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope`,
`260910-chore-session-children-scope-unnoted-filters`), and three fix commits.

## Decisions

- **The `session.note` assignment record is removed from `lead-run` entirely.**
  Select becomes: a ticket named in the invocation wins; otherwise spawn
  `ticket-selector` and use its one `selection:`. The lead never looks up,
  records, or advances a worker session key or note state on the serial route.
  Continue detection for an active multi-phase ticket is the selector's own
  `git.status` `impl_ticket` read, which already exists. This supersedes
  parent-epic Decision 16 ("`session.note` is a carry-over record ... so a
  compacted or restarted lead can rebuild") for the reasons under Background;
  the epic is closed, so this ticket is the record of that reversal.
  - `impl_ticket` is keyed on the checked-out branch (`activeImplTicket(root,
    branch)` in `server.go`), and `route.resolve_implement` returns
    `continue` only when HEAD equals the derived `impl/<root>/<slug>` name
    exactly (`finishImplementBranchPlanTail` in `implement_resolver.go`);
    with HEAD on the base and that branch holding unmerged commits it returns
    a hard stop naming the branch. So the rewrite keeps one rule in place of
    the deleted "check out its retained impl branch": after a `completion:
    phase` report the lead leaves the checkout on the impl branch, and if a
    HEAD-relative write of its own forces a switch to the base, it checks the
    impl branch back out before ending the turn. The rule applies only to a
    ticket with phases remaining and no `## Blocked` note; a blocked ticket's
    exit leaves HEAD on the base, so the selector skips it instead of stopping
    on a blocked owner. Recovery when that still
    fails (a restart with HEAD elsewhere): the selector re-picks the
    in-progress ticket, the worker's route stop names the retained branch,
    and the lead checks it out and re-dispatches; one wasted worker turn is
    the accepted cost.
  - Follow-up candidate, not this ticket: a git primitive that returns the
    retained impl branch for a stem, so the lead never depends on the
    checkout state.
  - Rejected: keep the note and make `playbook.render` return the minted child
    key. Once the note is gone the serial lead has no use for the worker's key,
    so the render change is unnecessary.
  - Accepted loss: the host agent id, the stop-(e) retry count, and the chosen
    tier live in the lead's conversation only. On loss, the existing resume
    rule applies unchanged: re-spawn with the same task block plus the
    `Resume: branch <branch> at <head>; ...` line. Worst case is one extra
    stop-(e) retry after a compaction.
- **`lead-run.md` is rewritten to the `skill-authoring.md` standard.** Target
  about 100 lines excluding the `risk-rubric` include; the number is a target
  for the rewrite, not a gate. Delete every item listed under Background. Keep
  as Layer 3: the stop (a) to (e) handling with its escalation ladder (fold the
  stop-(e) retry table into the Spawn tier table), the pre-write branch-status
  check compressed to its rule and its one-clause failure citation ("the
  worker's checkout is shared and outlives its turn"), the phase-completion
  default of continue-not-merge with its one-clause rationale stated once and
  stated truthfully (the next phase stacks because the worker's route returns
  `continue` when HEAD is the derived impl branch; never "while that branch
  still exists"), the
  `dispatch_blocked` action (report the blocking stem, end the turn) without
  the field explanation, the goal-branch staging step, the two goal-branch
  terminals, and the four verbatim **End the turn** lines, which the re-invoke
  driver reads and `TestPlaybookPrintLeadRunWorkerTierPolicy` pins.
  - Rejected: a prose-only trim that keeps the note and its key-recovery
    paragraph. It would leave one manual violation fixed in place.
- **A new `ticket-batch-selector` render playbook owns batch selection for the
  parallel route.** Frontmatter `kind: render`, `delegates: false`, `role:
  delegate`, `tier: medium`. It reads the `ready/` candidates, applies the
  parallel-safety predicate (two tickets are parallel-safe when neither
  functionally depends on the other's output: a `blocked-by:` edge between
  them, or a `related:`/`parent:` hint or body prose that says "A needs B's
  feature", is a dependency; a shared `parent:` epic or a bare `related:`
  edge is not by itself a dependency, and file-scope overlap is not an
  exclusion), skips candidates carrying a
  `## Blocked (...)` note or a `dispatch_blocked` point-resolve, and returns a
  flat set of mutually independent tickets. A candidate that depends on
  another candidate is excluded with its reason, never included with an
  order. When `git.status` reports an `impl_ticket` on the base checkout the
  selector stops instead of returning a batch: batch mode presumes a clean goal
  branch, and the in-progress serial ticket finishes first. Output shape:

  ```
  batch: <ticket path, one per line> | ready/ empty | every remaining ticket blocked | stop: <reason>
  excluded: <ticket path: reason, one per line> | none
  omitted: none
  ```

  `omitted:` lists what the selector did not evaluate and why (a candidate
  whose body it could not read, a relation it could not resolve); `none` when
  empty, the same field every delegate result carries.

  - Rejected: a batch mode inside `ticket-selector`. Its priority ordering is a
    small-tier task; pairwise independence over the candidate set needs body
    reads and reasoning, and the manual says text written for one tier is wrong
    for the other.
  - Rejected: the lead reading `ready/` itself through `tickets.query`. The
    anchor rejects moving work inline into the lead.
  - Rejected: including the active impl ticket as the first batch member.
- **The parallel route is collapsed to its delta from the serial route.** The
  differences it states: per-run user approval gates worktree provisioning
  (the approved batch is the concurrency cap); `ticket-batch-selector` picks
  the batch; one `worktree.acquire` per ticket on the same `impl/<parent>/<slug>`
  name the serial route derives; render with `root_override: <worktree path>`;
  collect every terminal report before any merge; merge serially through
  `git.merge`; release every acquired worktree including a stopped worker's.
  Because batch members are mutually independent, merge order is free; the
  "in dependency order" clause is deleted. Stop handling is by pointer to the
  serial section, not restated. The provisioning contract ("the same
  `impl/<parent>/<slug>` name the serial route derives") is carried over as
  the landed text states it, with the true continue condition above, plus one
  clause: when a batch worker's route verdict is not `continue`, the lead
  follows that verdict as reported rather than re-provisioning. The lead has
  no primitive that derives that name, which is the subject of the open
  bug `260915-bug-ws-route-resolve-implement-branch-handling-random-codename`
  and is fixed there, not here.
- **Two things are out of this ticket's scope and recorded here only as
  follow-up candidates:** retiring the now-consumerless `session.children`
  `scope`/`unnoted_only` filters, and moving goal-branch staging into a git
  primitive.

## Constraints

- Execution: the lead (Fable) edits the two playbooks directly and delegates
  the mechanical steps (manifest regeneration, wsflow and pi mirror
  regeneration, pinned-test updates, test runs) through `ws:lead-delegate`.
  The user directed this split for this ticket only; it does not reopen the
  parent epic's Cross-Child Decision 4 (the lead never edits source) as a
  general rule. To honor it, the ticket stays in `todo/` with its reviews
  stamped and is never promoted to `ready/`, so no queue drain can select
  it; the lead runs it named-directly from `todo/`, records each phase's
  Result, and closes it to `.done/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Mirrors land in the same logical change as the canonical edit, because
  `TestWsflowRsrcMirrorUpToDate` asserts byte equality. After the canonical
  rsrc edit, in order and with `-count=1`:
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
  `agents-plugin-pi/rsrc/` is a manual copy, as `83c60e42` did.
- Three suites pin `lead-run.md` text: `TestPlaybookPrintLeadRunWorkerTierPolicy`
  in `agents-plugin-tool/internal/mcp/playbook_tools_test.go`; the
  merge-obligation text test in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`; and
  `agents-plugin/tests/test_skill_dispatch_contracts.py`, whose
  `test_run_defers_phase_merge_and_continues_active_assignment` asserts the
  assignment-note sentences this ticket deletes and whose
  `test_run_pins_branch_awareness_reasoning` asserts a raw two-line string
  from the branch-status paragraph this ticket compresses, and whose
  `test_run_dispatches_through_playbook_read` asserts the `session.note` call
  and is already red on the current tree (its pin "One worker in flight per
  invocation." no longer matches the parallel-route sentence), so that red is
  pre-existing, not a regression of this rewrite. Find others with
  `grep -rn "lead-run" agents-plugin-tool/internal agents-plugin-wsflow/tests agents-plugin/tests`.
  Update a pin only when it guards a structural fragment the rewrite keeps;
  a pin that guards deleted text is deleted with it.
- Shared skill text names tools as `{{.McpNamespace}}/tool.name` and skills
  as `{{.SkillNamespace}}:<skill>`; nothing in either playbook may name this
  repository, its tickets, or its manuals.
- The four **End the turn** terminal lines stay verbatim.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-batch-selector/ticket-batch-selector.md (new), agents-plugin-wsflow/rsrc/ mirror, agents-plugin-pi/rsrc/ manual copy, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin/tests/test_skill_dispatch_contracts.py, manifest.json in each rsrc tree |
| scope.surface | public-interface | shipped playbook text pinned by TestPlaybookPrintLeadRunWorkerTierPolicy (agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2720) and test_wsflow_run_and_stop_protocol_carry_merge_obligation_text (agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L314), and three tests in agents-plugin/tests/test_skill_dispatch_contracts.py (Constraints); no exported Go symbol changes |
| scope.new_public_symbol | yes | ticket-batch-selector, a new render playbook name callable through playbook.render |
| scope.new_type_contract | yes | ticket-batch-selector's batch:/excluded:/omitted: output shape (Decisions) is a new render contract |
| scope.test_surface | existing | TestPlaybookPrintLeadRunWorkerTierPolicy, test_wsflow_run_and_stop_protocol_carry_merge_obligation_text, test_run_defers_phase_merge_and_continues_active_assignment, test_run_pins_branch_awareness_reasoning, test_run_dispatches_through_playbook_read, TestGenerateRealManifest, TestRegenerateWsflowRsrcMirror, TestWsflowRsrcMirrorUpToDate all confirmed present; no new test files named in Phases |
| complexity.reuse_points | confirmed | ticket-batch-selector modeled on agents-plugin/rsrc/ticket-selector/ticket-selector.md's git.status and Blocked handling (named in Phase 1); risk-rubric include (agents-plugin/rsrc/risk-rubric.md, 52 lines) reused unchanged |
| complexity.side_effect_risk | moderate | rewritten text governs live ticket dispatch/merge flow for every future lead-run invocation; a wrong instruction misroutes silently, with no compiler check |
| risk.correctness | moderate | large prose rewrite of a load-bearing playbook; the file's own history shows repeated post-placement fix rounds on smaller changes here (e.g. 36edc74f, 92e72d58) |
| risk.fit | moderate | must match skill-authoring.md's authoring standard while preserving verbatim the four End the turn lines and other pinned fragments across two mirrored packages |
| risk.test | moderate | multiple pinned tests span two languages and three rsrc trees (agents-plugin, agents-plugin-wsflow, agents-plugin-pi); a missed pin or a broken byte-mirror assertion fails silently until the delegate's test run |
| risk.security_or_contract | moderate | removes the session.note assignment record and its continue-detection contract, changing what Select reads to resume a multi-phase ticket (Decisions: accepted loss of host agent id, stop-e retry count, and tier across a compaction) |

## Phases

### Phase 1: Rewrite lead-run, add ticket-batch-selector, regenerate mirrors, update pinned tests

Rewrite `agents-plugin/rsrc/lead-run/lead-run.md` per **Decisions**, keeping
`kind: print`, `includes: [risk-rubric]`, and `variables: [SpawnIdiom]`.
Create `agents-plugin/rsrc/ticket-batch-selector/ticket-batch-selector.md`
with the frontmatter and output shape above, modeled on
`agents-plugin/rsrc/ticket-selector/ticket-selector.md` for its `git.status`
and `Blocked` handling. Point the parallel route at it.

Then, through the delegate: regenerate the manifest and the wsflow rsrc
mirror, copy to the pi mirror, update or delete the pinned tests, and run
`go test ./...` in `agents-plugin-tool/`, the wsflow package tests, and
`agents-plugin/tests/`.

Verification: all three suites green; `grep -rn "session.note" agents-plugin/rsrc agents-plugin-wsflow/rsrc agents-plugin-pi/rsrc` returns nothing; `wc -l agents-plugin/rsrc/lead-run/lead-run.md` reported in the Result; the three mirrors byte-identical.

### Phase 2: Fresh-reader audit

Run the fresh-reader audit from `skill-authoring.md` on the rewritten
`lead-run.md` and the new `ticket-batch-selector.md` through
`ws:lead-audit-doc`, one cycle each. Edit `fix` findings only; record the
rest with their class in this phase's Result.

Verification: the audit report for each file recorded in the Result with
every finding classified; when any `fix` landed, mirrors regenerated and all
three suites green again.
