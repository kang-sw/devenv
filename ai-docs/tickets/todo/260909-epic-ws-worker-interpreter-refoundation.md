---
title: "ws refoundation: worker as workflow interpreter, lead as escalation handler"
sage-review-design: required
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
---

# ws refoundation: worker as workflow interpreter, lead as escalation handler

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

## Child Tickets

Listed in intended execution order; the first must land before any layer is
removed.

- `260909-chore-ws-refoundation-git-history-measurement-manual` - qualitative
  before/after measurement manual over downstream git history (commit gaps
  per ticket, relay/revert ratio, escalation counts in Results, judgment
  items in AI Context). Prerequisite for every removal below.
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
- Planned: board reconciliation - drop the opposite-direction tickets named
  in `related:` and close the subsumed diet epics once their remaining phases
  are absorbed or dropped. Inventory stage management stays a user-and-lead
  action; this entry is a reminder, not a worker task.
- Planned: replace the `### Binding Anchor` declaration in this repository's
  `AGENTS.md` with `260909-research-ws-refoundation-evidence-audit` and
  topics covering lead surface, worker interpreter, document-layer
  retirement, and stop conditions, once the anchor ticket is populated.

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
4. **Worker as interpreter.** The worker is a native-harness subagent of at
   least current-mainstream or previous-generation-flagship class, spawned
   with a lead-capability child key (`ferrule(capability: "lead",
   parent_session_key: ...)`), and executes the whole ticket. The lead never
   edits source and never reads procedure playbooks. Spawn depth is
   recommended at one level (worker spawns Explore-class children for
   survey and review); deeper nesting is discouraged in prose, not blocked.
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
   a Critical finding surviving three review rounds. Everything else the
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
8. **Authoring pipeline keeps the sage gate.** Fact population (cheap tier)
   and design review (heavy tier) remain the `ready/` gate; the spec-address
   half is removed. Inventory stage moves (idea, todo, ready, dropped) are
   user-and-lead actions performed as batches of related tickets; `.done/`
   closing is the worker's.
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

## Completion Criteria

- Done: all listed children are `.done/`; the measurement manual has been
  applied once before and once after the removals on this repository; the
  binding anchor declaration points at the evidence-audit ticket; a fresh
  bootstrap and an upgraded project converge on the reduced layout.
- Dropped: the measurement pass shows worker-interpreter runs abort or
  re-work more than the current pipeline and no child can bring that below
  parity, in which case the surviving children are re-scoped under a new
  epic and this one records the finding.
- Deferred: parallelism inside `drain-ready-queue`; merging fact population
  and design review into one two-tier pass; per-role tier tuning.
