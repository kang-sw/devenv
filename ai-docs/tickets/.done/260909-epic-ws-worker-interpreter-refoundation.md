---
title: "ws refoundation: worker as workflow interpreter, lead as escalation handler"
sage-review-design: completed
related:
  260909-research-ws-refoundation-evidence-audit: evidence and rejected alternatives behind the decisions below
  260605-epic-ws-playbook-factory-pivot: prior epic; covered the harness-infrastructure axis only, not reopened
  260630-epic-skill-playbook-diet: partial diet; subsumed by this epic
  260903-epic-mcp-tool-surface-affordance-reduction: partial surface reduction; subsumed by this epic
  260824-epic-review-watermark-model: compatible; ledger and severity-graded relay move inside the worker
  260730-refactor-retire-goal-fan-out-step-and-session-note: aligned; fan-out entry point retires, its mechanism moves into drain-ready-queue
  260723-feat-ready-spec-address-hard-gate: opposite direction; drop candidate once this epic opens
  260716-feat-mental-model-comment-placement-rule: built on a retired layer; drop candidate
  260716-feat-mental-model-openup-injection: built on a retired layer; drop candidate
  260716-feat-sage-related-mental-model-curation: built on a retired layer; drop candidate
sage-review-design-reviewed: 0c6f903d85f40883
completed: 2026-09-12
---

# ws refoundation: worker as workflow interpreter, lead as escalation handler

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Cross-Child Decisions 4, 8, 12, and 15 name the lead surface, playbook rendering, routing, ticket body, and review flow; the landed child results span agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/, and ai-docs/. |
| scope.surface | public-interface | The ticket changes user-invoked lead skills and MCP-mediated ticket routing; the shipped lead-run procedure dispatches ticket-worker through playbook.render (agents-plugin/rsrc/lead-run/lead-run.md#L30-L58). |
| scope.new_public_symbol | yes | lead-delegate and the ticket-worker playbook are named new workflow entry points; ticket-worker is a shipped render playbook (agents-plugin/rsrc/ticket-worker/ticket-worker.md#L1-L17). |
| scope.new_type_contract | yes | role: worker maps a rendered playbook to a lead-scoped child key (agents-plugin-tool/internal/mcp/playbook_tools.go#L322-L345). |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_render_surface_test.go#L50-L70 covers the worker role mapping; the repository contains existing MCP, resource, and plugin test suites for the named surfaces. |
| complexity.reuse_points | confirmed | playbook.render already mints the worker child key and lead-run already consumes ticket route facts before spawning (agents-plugin-tool/internal/mcp/playbook_tools.go#L669-L760; agents-plugin/rsrc/lead-run/lead-run.md#L30-L58). |
| complexity.side_effect_risk | high | The scope changes capability-scoped child keys, execution routing, independent review, merge stops, and ticket lifecycle behavior. |
| risk.correctness | high | A wrong stop, key scope, or route can bypass required review, misroute an implementation, or grant a worker incorrect authority. |
| risk.fit | high | The epic coordinates completed children with open todo children and leaves several board and anchor updates planned. |
| risk.test | high | Behavioral coverage must keep aligned across Go MCP handlers, rendered ws and wsflow playbooks, ticket lifecycle tests, and downstream bootstrap migration. |
| risk.security_or_contract | high | The role: worker contract deliberately grants a rendered child lead scope, which changes capability boundaries (agents-plugin-tool/internal/mcp/playbook_tools.go#L328-L342). |

## Scope

Re-found the ws workflow on the capability of current-generation models
instead of adjusting it. The workflow was built as guardrails and pre-digested
context for earlier models; with capable models it acts as ballast: the lead
model spends its turns reading procedure text and resolving judgment gates that
are biased toward the heavy path, and the hand-maintained document layers drift
faster than they are read. This epic replaces that shape with:

- a **truth criterion** applied to every layer: if a current model can
  reconstruct the artifact's value on demand (from code, tests, git history,
  and the current context budget) more cheaply than the artifact can be
  maintained, the layer stands on a false assumption and is retired;
- a **worker-as-interpreter topology**: the lead only converses with the user,
  manages the ticket inventory, spawns workers, and handles first-line
  escalation; a native-harness worker holding a lead-capability child key
  interprets a whole ticket (route, edit, review, commit) and stops only on a
  closed list of conditions;
- a **decision-encoding authoring pipeline** (write-ticket, fact population by
  a cheap model, design review by a heavy model, batch promotion to `ready/`)
  as the place where the heavy model's judgment is spent, so that the
  execution pipeline consumes decisions instead of re-making them;
- retirement of the spec and mental-model document layers, the spec-address
  gate, write-time doc passes, lead-facing procedure playbooks, in-run
  survey/plan stages, the fan-out entry point, and the mercenary surface;
- a bootstrap template migration that ships the new shape downstream.

## Non-Scope

- Reopening `260605-epic-ws-playbook-factory-pivot` or its anchor: that epic
  settled the harness-infrastructure axis (native subagents over ws-spawned
  processes, session auth, rsrc distribution). This epic adds the model-
  capability axis it never addressed and keeps its infrastructure decisions.
- Retiring the ticket system, ticket status directories, the review ledger,
  the anchor regex, or the Go route resolver: these are mechanical tools with
  testable semantics and near-zero maintenance cost, not document layers.
- Removing independent review. Review is the one gate whose value is
  evidenced in this repository's history (relaxing it raised abort rates and
  was partially restored); it stays, risk-keyed and severity-graded, run by
  the worker.
- Enforcing spawn depth mechanically. Depth is a prose recommendation.
- Telemetry instrumentation. Measurement is a qualitative git-history
  analysis manual, not runtime metrics.
- Downstream projects' test coverage. Retiring spec assumes tests are the
  behavioral contract; whether a downstream project's tests can carry that
  role is that project's property, not something bootstrap can enforce.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Child Tickets

Listed in intended execution order; the first must land before any layer is
removed.

- `260909-chore-ws-refoundation-git-history-measurement-manual` - qualitative
  before/after measurement manual over downstream git history (commit gaps
  per ticket, relay/revert ratio, escalation counts in Results, judgment
  items in AI Context). Prerequisite for every removal below. Landed with
  the first baseline; its third review round reported seven Critical
  indicator defects unfixed, carried by the follow-up below.
- `260909-bug-workflow-cost-measurement-manual-round-three-findings` - triage
  and fix the manual's reported findings, re-run the baseline at the same
  measured commit. Prerequisite for the after-run, not for the removals.
- Ordering note (Decision 21): the collapse child's Phase 1 (protocol,
  `ticket-worker` playbook, `role: worker` render mint) lands before the
  spawner; the collapse child's Phase 2 (skill retirement) lands after it.
- `260909-refactor-drain-ready-queue-worker-spawner` - drain-ready-queue
  becomes the single drainer and spawner: mints a lead-capability child key
  per ticket and hands the whole ticket to a native worker; retires the
  fan-out entry point while keeping its mint/track mechanism; serial first,
  parallelism opened inside later.
- `260909-refactor-lead-surface-collapse-worker-stop-protocol` - lead skills
  collapse to discuss, ticket, run, review, ship plus housekeeping; `kind: print`
  procedure playbooks move to worker-facing `kind: render` playbooks whose
  core is the stop-and-report protocol; delegate prompts carry source
  pointers, never summaries.
- `260910-feat-lead-delegate-session-local-executor` - add a session-local
  free-form native-subagent entry for bounded arbitrary work, keep material
  implementation on `lead-run`, and retire the overlapping
  `lead-prefer-subagent` entry while preserving its tuning posture.
- `260910-refactor-ready-only-actionable-ticket-gates` - stop fact population
  and Sage review during ordinary actionable todo authoring; run both at ready
  promotion, while preserving explicit todo design settlement for epics.
- `260726-refactor-retire-workset-convention` - remove workset from new ticket
  authoring while preserving read/close compatibility for historical stems.
- `260909-refactor-route-resolve-implement-reads-ticket-facts` -
  `route.resolve_implement` reads route facts from the sage-stamped ticket
  instead of lead-gathered conversation facts; in-run survey/plan stages and
  the direct-edit/lead-only fast paths are removed.
- `260909-refactor-retire-spec-mental-model-layers` - remove the spec and
  mental-model MCP tools, the spec-address gate, doc coverage alarms, and
  write-time doc passes; archive this repository's `ai-docs/spec/` and
  `ai-docs/mental-model/` corpus under `ai-docs/.old/` (never destroy;
  bootstrap applies the same archive rule downstream); sage promotion gate
  stays; on-demand digests become untracked caches stamped with a commit
  hash.
- `260909-chore-retire-mercenary-surface` - mercenary is a deprecation
  target; native harness delegation is the only path.
- `260909-feat-bootstrap-refoundation-template-migration` - new template
  version and downstream migration item for the reduced layout; dogfood on
  a disposable fixture before release.
- `260912-feat-sage-design-autonomous-exploration` - keep fact population as
  bounded factual grounding and let the read-only Sage design reviewer
  autonomously delegate code-contract exploration with rendered tier bindings.
  This child must land before `epic/refound` merges into `develop`.
- `260912-bug-git-merge-release-target-diagnostics` - replace the
  topology-specific `main` and `master` dead end with structured refusal
  diagnostics and an OID-bound release-target acknowledgement. This child must
  land before `epic/refound` merges into `develop`.
- `260912-feat-git-merge-generic-branch-promotion` - generalize the lead-owned
  merge primitive across exact local branch pairs while retaining impl and goal
  lifecycle cleanup plus release-target safety. This post-landing child closes
  the epic-to-develop dogfood gap before release promotion.
- Planned: board reconciliation - drop the opposite-direction tickets named
  in `related:` and close the subsumed diet epics once their remaining phases
  are absorbed or dropped. Inventory stage management stays a user-and-lead
  action; this entry is a reminder, not a worker task.
- Planned: replace the `### Binding Anchor` declaration in this repository's
  `AGENTS.md` with `260909-research-ws-refoundation-evidence-audit` and
  topics covering lead surface, worker interpreter, document-layer
  retirement, and stop conditions, once the anchor ticket is populated.
- Planned: prose drafts before any child runs. The shipped texts that carry
  judgment are written first, by the lead-tier model, against
  `ai-docs/manuals/skill-authoring.md` as re-baselined on 2026-09-09, and
  committed under `ai-docs/ref/refound-drafts/` on the epic branch: the
  shared worker stop-and-report protocol, the ticket-execution worker
  playbook, the five lead skill bodies (discuss, ticket, run, review, ship),
  the fact-populator revision, the bootstrap template conventions section
  and migration item, the two new `WORKFLOW.md` sections, and the
  measurement manual body. Each child's phases move the relevant draft into
  place and do the mechanical work (Go, tests, manifests, wsflow mirror).

## Cross-Child Decisions

1. **Truth criterion.** A layer survives only if reconstructing its value on
   demand with current models costs more than maintaining it. Humans do not
   read `ai-docs/`; every human-facing answer is an AI digest, so a drifted
   document has negative value (a stale intermediary between code and the
   human). No child may reintroduce a hand-maintained derived document.
2. **Memory tiers that remain.** Tickets (backlog intent and decision
   records), commit `## AI Context` (immutable rationale), notes (environment
   quirks), and a few manuals. Non-derivable runtime traps go into code
   comments at the site that bites, never into a separate document.
3. **Tests are the contract; the ticket is the plan.** Behavior that matters
   is a test. Key decisions are encoded at ticket authoring; the sage-stamped
   ticket carries the route facts. A behavior change with no test change is
   a review finding, replacing the retired spec-drift check.
4. **Worker as interpreter.** The worker is a native-harness subagent whose
   initial tier follows the sage-reviewed ticket risk: `low` or `moderate`
   routes to medium, and `high` routes to large. For ad-hoc contracts the lead
   makes the equivalent binary judgment (`routine` -> medium, `difficult` ->
   large). A stop-(e) escalation raises medium to large or large to xlarge.
   The worker is spawned with a lead-capability child key
   (`ferrule(capability: "lead", parent_session_key: ...)`) and executes the
   whole ticket. The lead never edits source and never reads procedure
   playbooks. Spawn depth is recommended at one level (worker spawns
   Explore-class children for survey and review); deeper nesting is discouraged
   in prose, not blocked.
5. **Closed stop list.** The worker stops only on: (a) a merge into a
   parent branch: the goal branch `goal/<parent>/<stem>` merging into
   `<parent>` requires user approval whatever `<parent>` is, because the
   parent is workflow convention, not a downstream choice, and this merge is
   the veto point for the worker's autonomous decisions; `main`-class merges
   likewise; impl-branch merges into the goal branch stay worker-autonomous
   under `merge_confirm`; (b) an `[escalate-to-lead]`
   entry or an Open Decision Queue during promotion; (c) a ticket decision
   contradicted by code reality so it cannot be executed as written; (d) an
   irreversible action per the Approval Protocol's always-ask category; (e)
   a Critical finding still open after the fix round (Decision 20). Everything else the
   worker decides, records in `## AI Context` and the ticket `### Result`,
   and proceeds; the merge-stop report lists those decisions for veto.
6. **Lead as first-line escalation handler.** On (c) the lead first attempts
   autonomous resolution (raise tier, re-run the sage gate over the worker's
   proposed resolution, resume via the host continuation mechanism); on (e)
   the lead performs the elevation. The user sees only low-reversibility
   decisions and exhausted lead attempts.
7. **Pointers, not summaries.** A worker's summary is never another worker's
   sole input. Reviewers compute the diff range from git and read the ticket
   source; planners' structured output must carry an explicit omitted/deferred
   field. This replaces lead-side caution as the information-loss mitigation.
8. **Authoring gates run at the category's settlement boundary.** For
   actionable tickets, ordinary `todo/` authoring runs neither fact population
   nor Sage review; fact population, design review, and completeness review run
   together at promotion to `ready/`. An epic remains a board artifact and
   settles design explicitly at `idea/` to `todo/`: fact population precedes
   its design-only review, and later cross-child decision changes require
   explicit re-settlement rather than automatic review on every edit. Research
   remains ungated in `idea/` or `todo/`. Inventory moves are user-and-lead
   actions; `.done/` closing is the worker's.
9. **Single drainer.** `drain-ready-queue` is the only goal-loop entry point.
   Parallelism is opened inside it later; no second entry point.
10. **Mercenary is deprecated.** No child may route new behavior through
    `mercenary.*`.
11. **Shipped-surface rule holds.** No child ships text naming this
    repository's tickets, layout, or migration vocabulary; downstream input
    goes through generic hooks.
12. **Key minting.** `ferrule` is the base primitive. A subagent whose prompt
    is a rendered playbook gets its child key from `playbook.render`, which
    mints as part of rendering; the spawner does not mint a second key for
    the same worker. The `workflow_manual` mint was a convenience, not a
    contract.
13. **Ship stays on the lead surface.** The working skills are discuss,
    ticket, run, review, ship. A release is a low-reversibility user-facing
    action, so its decision and stop belong to the lead and user; the
    procedure itself may be handed to a subagent with the manual.
14. **Closed inventory is immutable.** No child edits or moves anything under
    `.done/` or `.dropped/`. Historical keys and citations in closed tickets
    (`related-mental-model:`, spec anchors) remain as residue, and any
    reader that resolves them must tolerate an unresolved target.
15. **The fact populator edits the ticket directly.** It holds edit rights on
    the one ticket file it is populating (prompt-governed; system-enforced
    where the host allows) and writes the route facts into a ticket body
    section rather than returning a verdict for the lead to apply. Body
    placement is deliberate: the sage stamp digests the body, so facts
    changed after the stamp invalidate it and the design review always
    stamps the facts it read. `### Result` sections stay immutable; the
    lead vetoes through the unstaged diff.
16. **Notification-driven wait; `session.note` is a carry-over record.** The
    drain turn spawns the worker and waits for the host's completion or
    interim notification; it neither blocks in a tool call nor polls. One
    `session.note` section per lead session records the in-flight
    worker-to-ticket assignments and the host agent id needed to resume
    each worker, so a compacted or restarted lead can rebuild them; it is
    not a live progress board, and the terminal is the host notification.
17. **The goal-to-parent merge terminal stays with the lead.** It is outside
    any single worker's ticket, so drain keeps it: the lead aggregates the
    workers' reports into the merge-stop report, obtains user approval, and
    performs the merge. This is a git action, not source editing.
18. **Prescriptive text survives; descriptive text is derived.** A document
    that prescribes (preferred libraries, patterns, boundaries, domain
    constraints) is a human decision, is not reconstructible from code, and
    does not drift with code; it stays. A document that describes code
    (what it does and why) is reconstructible and is retired. The worker's
    inputs are exactly: `AGENTS.md` (host-loaded), the sage-stamped ticket,
    tests, code and git history, notes and manuals, and one worker playbook.
    Domain constraints live by kind: short universal rules inline in
    `AGENTS.md`; longer or path-scoped rules in `ai-docs/manuals/`, declared
    in a path-scoped `AGENTS.md` section (for example
    `### Implementation Conventions` with `paths` -> manual rows) that the
    worker playbook reads through a generic hook; testable rules become
    tests; site-specific traps become code comments; non-derivable external
    facts go to `ai-docs/ref/`. The fact populator cites the applicable
    manuals in the ticket's `## Constraints`, and review treats a change
    contradicting a cited convention as a finding. Prescriptive documents
    carry no per-commit update obligation; drift is handled by
    update-on-contact and review. This is what Decision 2's "a few manuals"
    means.
19. **Execution dogfoods the target topology by hand.** The children are not
    run through the existing drainer, which is the pipeline this epic
    replaces. The lead promotes them in two batches (the measurement manual
    alone, then the remaining six after the baseline run), and for each
    ticket spawns one worker of at least current-mainstream class with a
    lead-capability key, the ticket path and stem as its only inputs, and
    the closed stop list of Decision 5 as its prompt; the lead receives the
    terminal report through the host notification and applies the
    goal-to-parent merge stop. This run is the measurement manual's first
    after-sample. Branch: `epic/refound`.
20. **Review is two rounds, and the branch is never rewritten.** Observed on
    the first hand-dogfood run (the measurement-manual ticket): a fresh
    reviewer each round found a fresh set of Criticals, so three rounds did
    not converge and a fourth was about to open; and the worker reset the
    shared branch to re-author its commits, dropping a lead commit made
    concurrently. Therefore: round 1 is a full review by fresh reviewers at
    the route's allocation; round 2 verifies only that round-1 findings were
    fixed and raises nothing new (an observation goes into `unresolved:`);
    there is no round 3, and a Critical still open after round 2 is stop (e).
    The goal branch is shared with the lead, who commits on it during the
    run: the worker never amends, resets, or rebases it; a correction is a new
    commit. While a spawned delegate runs, the worker waits for the host's
    completion signal and does not poll or fill the wait with repeated
    verification runs.
    *Rejected: keep three rounds with a same-root-cause stop* — the rounds
    failed by finding different Criticals each time, which that stop does
    not catch.

21. **`role: worker` is what makes `playbook.render` mint a lead-scoped key,
    and the collapse child owns that change.** Design review found that the
    render mint (Decision 12) maps only delegate roles to child keys and
    never yields a lead scope, so Decisions 12 and 8 named a mechanism that
    did not exist. The fix is a new frontmatter role, `worker`, for which
    `playbook.render` mints a child key with the caller's lead capability
    and root, identical in scope to `ferrule(capability: "lead")`. It is a
    capability-model change and lands with the playbook that declares the
    role: the collapse child's Phase 1, which therefore lands before the
    spawner. Ordering: collapse Phase 1 (protocol, `ticket-worker`, the
    mint) → spawner (`lead-run`) → collapse Phase 2 (skill retirement). The
    spawner renders `ticket-worker` by name and mints nothing else.
    *Rejected: spawner first with a `ferrule` stopgap* — two spawn shapes
    for one worker, the second thrown away one ticket later.
22. **Surviving skills keep the `lead-` prefix; two renames; free-form
    delegation gets its own entry.** The working six ship as `lead-discuss`,
    `lead-ticket` (renamed from the ticket-authoring skill, owned by the
    collapse child's Phase 2), `lead-run` (replaces the queue drainer, owned
    by the spawner), `lead-delegate`, `lead-review`, `lead-ship`.
    `lead-delegate` owns session-local arbitrary native-subagent execution;
    material implementation remains on `lead-run`. Housekeeping survives
    unchanged: `lead-bootstrap`, `lead-tune`, `lead-revive`,
    `mcp-server-repair`, `lead-workflow-manual`, `lead-check-blockers`.
    `lead-prefer-subagent` is retired by
    `260910-feat-lead-delegate-session-local-executor`; its
    `workflow.prefer_subagent` setting remains as a tuning posture that defaults
    eligible work to `lead-delegate`. The worktree-scoping and rule-persisting
    skills survive. `/goal` and the drainer's verbatim final line name
    `lead-run`.
    *Rejected: bare names (`discuss`, `run`)* — every inventory pin, the
    `/goal` loop, and downstream muscle memory carry the prefix; the
    rename buys nothing the collapse does not already buy.
23. **Route facts are grandfathered lead-side, never as a worker stop.** A
    ticket promoted before the resolver reads route facts, or by an older
    plugin, has no `## Route Facts` section. Three defined behaviors, all
    in the route-facts child: the promotion gate checks presence going
    forward; the resolver reports a missing block as a named outcome rather
    than falling through to a conservative verdict; and `lead-run`, before
    spawning, renders the fact populator once when the section is absent.
    No downstream migration item: the behavior arrives with the plugin.
    *Rejected: worker stop (c) on missing facts* — one spawn, one stop, and
    one lead turn per legacy ticket, for a fact the lead can populate in
    one cheap-tier call before spawning.
24. **Workset retires as an authorable category.** Goal-mode `lead-run` over
    the actionable ready queue owns mixed-parent execution grouping; `epic` +
    `parent:` owns single-outcome decomposition; `related:` owns loose
    association. New worksets are rejected, the sole open workset is dropped,
    and active or archived workset stems remain readable and closable for
    compatibility. Workset semantics do not transfer to epic.
25. **Ground facts directly; explore design evidence through native
    subagents.** The fact populator is a bounded, write-enabled grounder that
    corrects unambiguous terminology and present-behavior facts in its one
    ticket without changing decisions. The read-only Sage design reviewer does
    not independently search or navigate the codebase; it autonomously chooses
    whether, how widely, and at which rendered `small`, `medium`, or `large`
    model-and-effort binding to dispatch host-native Explore subagents. Explore
    usage is taught by examples rather than a routing matrix. The reviewer may
    open exact artifacts the explorers cite to verify load-bearing claims, then
    judges those current-contract facts against the ticket's confirmed intent.
    No MCP-owned spawn surface is introduced, and completeness review remains
    ticket-only.

## After-Removal Workflow-Cost Measurement (2026-09-12)

The complete record is
`ai-docs/ref/refoundation-workflow-cost-measurement-260912.md`. Both runs use
the corrected manual, pinned commits `84b1f825f5858716833d2f8094ab2848826c6011`
and `cb0458151aecf4979dd05b6c7e702ebbcbf755a2`, and the same `SIZE=51`,
`FLOOR=5`, selector, exclusions, and four-reader judgment protocol. Increasing
the window from the original 20 is permitted when both halves change together;
51 is the smallest size at which neither run has more missing-`completed:`
skips than the window size. The after window contains 37 new and 14 shared
tickets.

Recorded abort indicators do not increase: blocked headings remain 10,
dropped phases remain four, dropped tickets with real implementation remain
four of nine printed candidates, judged Result stops remain zero, and
first-parent goal landings remain five. Other reachable goal-merge traffic
increases from nine to 11; the manual explicitly treats that as merge traffic,
not an abort or completion rate.

Visible corrective work increases from 10 of 162 post-implementation commits
before to 19 of 114 after, and to 19 of 78 in the new partition. The comparison
does not establish that the worker interpreter causes more re-work: commit
ownership and granularity changed, the samples cover different topic mixes and
periods, and 23 of the 37 new tickets have no eligible implementation anchor
while five more have defective selected anchors. It also does not establish
parity or improvement. The evidence therefore does not trigger the epic's
Dropped criterion, whose second condition—no child can bring the result below
parity—is likewise unestablished.

## Completion Criteria

- Done: all listed children are `.done/`; the measurement manual has been
  applied once before and once after the removals on this repository; the
  binding anchor declaration points at the evidence-audit ticket; a fresh
  bootstrap and an upgraded project converge on the reduced layout.
- Dropped: the measurement pass shows worker-interpreter runs abort or
  re-work more than the current pipeline and no child can bring that below
  parity, in which case the surviving children are re-scoped under a new
  epic and this one records the finding.
- Deferred: parallelism inside `drain-ready-queue`; per-role tier tuning.


## Resolution (2026-09-12)

Closed after all listed child tickets landed. The workflow-cost manual was applied before and after the removals; the after-run did not establish the Dropped criterion. AGENTS.md now binds the declared topics to 260909-research-ws-refoundation-evidence-audit, which remains active as the live reference anchor. Fresh-bootstrap and upgraded-project verification converged on the reduced layout. Parallel execution inside lead-run and per-role tier tuning remain deferred.
