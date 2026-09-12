---
title: "Restore the dropped goal-run trigger declaration into lead-run"
related:
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: context; the closed design this ticket derives from (research, stays in todo/, not a code prerequisite)
  260909-epic-ws-worker-interpreter-refoundation: context; the refoundation renamed lead-drain-ready-queue to lead-run; the staging prose carried over intact (agents-plugin/rsrc/lead-run/lead-run.md#L55-L60) and only the Posture section (goal-run trigger, autonomous behavior) was dropped
  260605-research-ws-native-subagent-pivot: context; the loop re-fire / emitter axis (deliberately out of scope here) is entangled with this anchor
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 1d5a39242c296d27
sage-review-completeness-reviewed: 1d5a39242c296d27
completed: 2026-09-11
---

# Restore goal-branch staging into lead-run

## Background

`lead-run` attached to a goal context does not create a `goal/*` branch at all.
Diagnosis (recorded in the research ticket) traced this to two separable axes:

1. **Goal-branch handling** — the staging that creates `goal/<parent>/<slug>` on
   the first turn. develop's `lead-drain-ready-queue` (base skill + Posture)
   carried a known-good version of this; the branch-creation steps themselves
   carried into `lead-run` largely intact (the same `/goal`-reminder-gated
   `git checkout -b goal/<parent>/<slug>` staging is already present at
   `agents-plugin/rsrc/lead-run/lead-run.md#L55-L60`, byte-for-byte mirrored
   into `agents-plugin-wsflow/rsrc/lead-run/lead-run.md`); what the refoundation
   dropped is the `Posture` section that named the goal-run trigger and drove
   autonomous behavior, not the staging prose — matching
   260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming's own
   "Finding: the goal loop is orphaned" section ("The branch creation strategy
   survived nearly verbatim... what died is the activation driver").
2. **Loop re-fire / emitter** — re-invoking `lead-run` each turn is
   harness-dependent (the Claude `/goal` built-in emitted the reminder the staging
   condition reads; the retired `agents-plugin-pi` re-injected it). No live package
   emits `/goal`.

Only axis (1) is in scope here. It is a localized, low-risk restoration of prose
that worked in dogfooding. Axis (2) stays deferred (entangled with
`260605-research-ws-native-subagent-pivot`); this ticket does not attempt to
re-establish the auto-loop, only to make goal-branch **creation** correct under a
host that emits the goal reminder.

## Decisions

- **Restore only the goal-run trigger declaration, verbatim-grade.** Staging
  (`lead-run.md#L55-L60`) and the two `goal/*` terminals (`#L133-L145`) already
  carried over; the one thing the refoundation dropped is the `Posture`
  declaration that named a goal run — current branch `goal/*` **or** an active
  goal reminder. Restore that trigger line (host-neutral) into `lead-run`'s goal
  path and stop there. Rejected: re-authoring staging/terminals — they are present,
  so a rewrite is needless churn against the exact-prose goldens.
- **Do not restore the autonomous "user is away" posture.** develop's Posture also
  drove an autonomous stance (resolve reversible decisions on the lead's own
  recommendation, delegate everything including commits). That was deliberately
  retired on epic/refound and stays out; restore the trigger condition only, not
  the autonomous behavior.
- **Scope is goal-branch creation only.** The loop re-fire / emitter is a separate
  deferred host axis and is explicitly excluded; do not add emitter shims or
  dangling-consumer cleanup here.
- **Fan-out stays out.** develop factored a parallel `lead-goal-fan-out-step`
  overlay; its reintroduction as a lead-run extension mode is 260910's concern, not
  this ticket's. Restore only the serial goal-branch staging.

## Constraints

- `agents-plugin/rsrc/`, `agents-plugin/skills/` — read
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md`. The restored prose ships
  downstream and must not depend on repo-only facts; the goal-reminder trigger is a
  generic harness signal, referenced through the generic hook, not a Claude-only
  path baked into shipped text.
- `lead-run.md` is a flagship playbook whose body is pinned by exact-prose
  goldens/fixtures in `agents-plugin/tests/` — an edit to it must update every
  guarding golden and run that suite (the golden-fixture verification gap). Name the
  guarding fixtures in the worker's verification surface.
- `agents-plugin-wsflow/` — mirror the goal-prose restoration into the wsflow
  derivative.
- The random word-word-word goal slug must stay non-goal-text-derived (the
  collision fix from `260713-bug-lead-drain-ready-queue-goal-branch-slug-collision`,
  .done); do not reintroduce a text-derived slug.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-wsflow/rsrc/lead-run/lead-run.md, agents-plugin/tests/test_skill_dispatch_contracts.py |
| scope.surface | internal | shipped playbook prose, no exported MCP tool or code symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py exercises lead-run.md today (no goal-branch-specific assertions currently in it) |
| complexity.reuse_points | confirmed | develop's lead-drain-ready-queue/SKILL.md staging and terminal prose, already carried into agents-plugin/rsrc/lead-run/lead-run.md#L55-L60 and #L133-L145 |
| complexity.side_effect_risk | moderate | edits agent-facing instructions that gate autonomous git branch creation during a goal run; no code executes automatically, but wrong prose misfires branch creation |
| risk.correctness | moderate | current tree already implements most of the described staging/terminal logic under a literal `/goal` token; scope of the remaining edit is unsettled (see decision gap) |
| risk.fit | moderate | the ticket's own premise ("no branch is staged") is contradicted by the tree for the in-scope axis, so the concrete remaining deliverable needs re-scoping before implementation |
| risk.test | low | no new test files; update assertions in the one existing guarding suite plus its wsflow mirror |
| risk.security_or_contract | low | no MCP tool, API, or security-relevant contract touched |

## Phases

### Phase 1: Restore the goal-run trigger declaration

Restore the dropped goal-run trigger declaration into `lead-run`'s goal path,
verbatim-grade from develop's `lead-drain-ready-queue`:

- **Trigger (the actual gap).** The concrete delta is making the goal-run trigger
  host-neutral: the present staging line gates on a literal `/goal` reminder
  (`agents-plugin/rsrc/lead-run/lead-run.md#L55`), a Claude-only token; restate it
  through the generic harness hook, and add the explicit declaration that a goal run
  = current branch `goal/*` **or** an active goal reminder. Editing that one literal
  token is the "genuine gap" the "do not rewrite working prose" constraint permits —
  the tension is only apparent: the staging *steps* stay, the *trigger token* changes.
- **Staging / terminals (already present — verify, do not re-add).** Staging at
  `#L55-L60` and the two `goal/*` terminals at `#L133-L145` carried over. Diff against
  develop's `lead-drain-ready-queue/SKILL.md` and close only genuine gaps.
- **No-op guard.** If the develop diff surfaces no genuine behavioral gap beyond the
  host-neutral trigger wording, that wording change is the entire deliverable — do
  not install ceremonial "Posture" prose that cannot make branch creation fire any
  differently than today. The value is a host-neutral trigger, not restored verbiage.

Exclude: the autonomous "user is away" posture (deliberately retired), the loop
re-fire / emitter (deferred axis-2), and the fan-out overlay (260910). Keep the
random word-word-word slug non-goal-text-derived. This edit regenerates the shared
`lead-run.md` exact-prose goldens — three other `ready/` tickets touch the same file
(`260911-feat-ws-git-merge-lead-owned-merge-authority`,
`260911-feat-impl-derivation-hardening-branch-aware-select`,
`260911-refactor-lead-run-ticket-only-delegate-implementer`); this ticket's regions
(trigger + terminals) are disjoint from theirs, so regenerate the goldens against the
then-current body rather than a fixed layout. Update every guarding golden/fixture and
mirror into `agents-plugin-wsflow/`. Verify: the dispatch-contract suite in
`agents-plugin/tests/` and the wsflow package tests pass; run the full suite touching
every edited shipped file.

### Result (3877bc5) - 2026-09-11

Restored the explicit host-neutral goal-run declaration and changed the branch
staging condition to use an active goal reminder rather than the retired literal
`/goal` token. Preserved the existing random-slug staging and goal terminals,
then regenerated the canonical and wsflow rsrc manifests and their byte-identical
wsflow mirror. The dispatch contract now pins both the declaration and the
host-neutral staging condition.

Verification: `python3 -m unittest agents-plugin/tests/test_skill_dispatch_contracts.py`,
`python3 -m unittest agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`,
`python3 -m unittest agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`, and
`(cd agents-plugin-tool && go test ./internal/wsrsrc -count=1)` passed.

Reviews: round-one correctness and fit reviews found no findings; round two
confirmed no round-one findings remained.


## Resolution (2026-09-11)

Restored the host-neutral goal-run trigger declaration, regenerated mirrored playbook artifacts, and added dispatch-contract coverage.
