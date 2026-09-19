---
title: Restructure ticket-worker into direct + elevated mini-lead bodies
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260919-research-mini-lead-intra-ticket-orchestration: design source — Confirmed Decisions (contract) and Verified Findings (evidence)
  260915-refactor-lead-run-dispatch-time-tier-judgment: the dispatch-time tier judgment this rewrites around
blocked-by: 260919-feat-ws-playbook-render-tier-override
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3c28789f79937a5d
sage-review-completeness-reviewed: 3c28789f79937a5d
---

# Restructure ticket-worker into direct + elevated mini-lead bodies

## Background

A ticket-worker told to advance a single phase drifts as its context fills and
implements every remaining phase; the phase boundary that should hold it is a
soft, tier-blind body rule, not an enforced stop, so the failure clusters in
large-tier workers (`260919-research-mini-lead-intra-ticket-orchestration`,
Verified Findings). This ticket implements the design that research ticket
converged on: replace the three byte-identical `ticket-worker*` bodies with two
that differ in behavior — a direct implementer and an orchestrating mini-lead —
so the drift-prone large tier stops relying on the soft boundary and instead owns
the whole ticket while pushing implementation into fresh-context leaves.

The design source is `260919-research-mini-lead-intra-ticket-orchestration`; its
Confirmed Decisions are this ticket's contract and its Rejected Alternatives hold
the rationale not repeated here. The render-time tier override this depends on is
the sibling feat `260919-feat-ws-playbook-render-tier-override` (`blocked-by`).

## Decisions

Carried from the research ticket's Confirmed Decisions (see it for full
rationale); the implementation-shaping contract:

- **Two bodies, not three.** Default `ticket-worker` implements directly
  (unchanged prose); `elevated` orchestrates. `ticket-worker-escalated` is
  retired. Both keep `role: worker` (lead scope) — the difference is prose, not
  permission.
- **xlarge reuses the elevated body** via a render-time tier override, not a third
  body (depends on the sibling feat). large and xlarge share orchestration
  behavior; only the model differs.
- **The elevated body encodes:** sequential leaf execution on a single warm shared
  worktree (leaves self-verify in their own context); decomposition along the
  interface/spine → disjoint-leaf seam, with a phase-serial fresh-leaf fallback
  when no disjoint seam exists; delegation calibrated by difficulty (hard core
  stays on the mini-lead, mechanical disjoint leaves go down by default);
  interface + test-contract review as a named-condition recommendation, not a hard
  rail. The `worker-stop-protocol` escalation and the outer lead-review/ship gate
  are untouched — this tunes the inner loop only.
- **Left untouched:** the default `ticket-worker` body prose and the current
  phase-boundary behavior for the medium path (it mostly holds there); no new
  runtime role or permission tier.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

Every body edit lands in both `agents-plugin/` and `agents-plugin-wsflow/` (exact
byte-mirror) with both `rsrc/manifest.json` files regenerated, or the drift tests
fail. The shipped surface is downstream-first (Architecture Rule 4): the design
uses only existing primitives, so nothing here depends on host-specific behavior.

## Prior Decisions

- 260919-research-mini-lead-intra-ticket-orchestration (2026-09-19, ticket Confirmed Decisions): "Two playbook bodies, not three. Collapse the byte-identical trio into default ticket-worker (implements directly) and elevated (orchestrating mini-lead)." — bearing: supports
- 260919-feat-ws-playbook-render-tier-override (2026-09-19, commit caff4ec9): "the render tier-override is a discrete, independently buildable/verifiable MCP-tool primitive and the feasibility gate for that ticket's Confirmed Decision 5" — bearing: constrains
- 260915-refactor-lead-run-dispatch-time-tier-judgment (2026-09-15, commit 83c60e42): "Kept the reactive escalate ladder (lead-run.md stop-e retry table) unchanged as the safety net; added one prose line noting xlarge is now also a proactive dispatch-time pick." — bearing: constrains
- 260915-refactor-lead-run-playbook-diet-drop-assignment-note (2026-09-15, commit 0a1d5b5b): "Continue detection for a multi-phase ticket now rests on git state, not a note: ... Handle the report keeps HEAD on the impl branch after a completion: phase report" — bearing: constrains
- 260910-refactor-risk-route-worker-tier-medium-large (2026-09-10, ticket Result 74e79931): "Shipped medium ticket-worker, large ticket-worker-elevated, and xlarge ticket-worker-escalated playbooks whose bodies differ only by tier frontmatter." — bearing: constrains
- 260913-bug-ws-pi-worker-checkout-contaminates-lead-branch (2026-09-13, ticket Result b8a6f827): "worker-stop-protocol.md Branch section ... This include reaches ticket-worker.md, ticket-worker-elevated.md, ticket-worker-escalated.md" — bearing: supports
- 260909-epic-ws-worker-interpreter-refoundation (2026-09-19, commit 9ed95844): "Playbook trio (byte-identical) collapses to two divergent bodies (implement vs orchestrate); xlarge = elevated prose + config.resolve_agent model override, not a third body." — bearing: supports
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, ticket Decisions): "No orchestrator role. Extend the existing worker ownership model; do not add another runtime role or move lead playbooks into a mini-lead process." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-worker-elevated, ticket-worker-escalated (retired), lead-run/lead-run.md; agents-plugin-wsflow mirrors; rsrc/manifest.json (both packages); test_skill_dispatch_contracts.py, test_wsflow_skill_bundle.py |
| scope.surface | public-interface | shipped rsrc playbook prose in ws + wsflow plus their drift/contract tests; no Go tool change here (the render override is the sibling feat 260919-feat-ws-playbook-render-tier-override) |
| scope.new_public_symbol | no | no new symbol; Phase 2 consumes the tier_override arg added by the prerequisite feat |
| scope.new_type_contract | no | prose-only playbook change; both bodies stay role: worker, no new frontmatter/key contract |
| scope.test_surface | existing | test_skill_dispatch_contracts.py:215-234 (three-stem body tuple), test_wsflow_skill_bundle.py:327-379 (wsflow mirror + lead-run dispatch prose), rsrc/manifest.json SHA-256 drift |
| complexity.reuse_points | confirmed | existing worker lead-scope primitives (ferrule, worktree.acquire, session.children); tier_override from 260919-feat-ws-playbook-render-tier-override; default ticket-worker body reused unchanged |
| complexity.side_effect_risk | high | rewrites the worker-dispatch surface every lead-run invocation consumes, byte-mirrored across agents-plugin and agents-plugin-wsflow |
| risk.correctness | high | new orchestration body plus dispatch/escalation ladder rewrite; large blast radius across shipped worker dispatch |
| risk.fit | moderate | must reconcile with the already-landed dispatch-time tier judgment (260915-refactor-lead-run-dispatch-time-tier-judgment, commit 83c60e42) |
| risk.test | moderate | three-stem tuples and byte-mirror/manifest drift tests must be updated in lockstep with the body changes |
| risk.security_or_contract | low | both bodies remain role: worker (lead scope); no new permission tier or capability boundary |

## Phases

### Phase 1: Author the elevated mini-lead body; collapse three bodies to two

Rewrite `ticket-worker-elevated` into the orchestrating mini-lead prose and retire
`ticket-worker-escalated`; leave the default `ticket-worker` body as-is.

- Author the `elevated` body to encode the Decisions above: own the whole ticket,
  decompose along the interface/spine → disjoint-leaf seam (phase-serial fresh-leaf
  fallback when no disjoint seam), run leaves sequentially on one warm shared
  worktree with leaf self-verification, delegate by difficulty (mechanical leaves
  down by default, hard core kept), and treat spine review as a named-condition
  recommendation. Keep `role: worker`, the `worker-stop-protocol` include, and the
  existing spawn variables (`ExploreAgent`, `SpawnIdiom`). Apply the
  skill-authoring invariant checklist to every changed Invariants/Constraints line.
- Retire `agents-plugin/rsrc/ticket-worker-escalated/` and its wsflow mirror;
  regenerate both `rsrc/manifest.json` files.
- Update the tests that hard-code the three stems: the body-set tuple in
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (~:215-234) and the wsflow
  mirror in `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` (~:327-379),
  plus the byte-mirror/manifest drift assertions.

Verification: the merge-obligation and stop-protocol contract tests pass against
the two remaining bodies; manifest and wsflow-drift tests are green.

### Result (a6f54fe6) - 2026-09-19

Rewrote `ticket-worker-elevated` into the orchestrating mini-lead body: it owns
the whole ticket (every phase without a `### Result`, not one), decomposes along
the interface/spine → disjoint-leaf seam with a phase-serial fresh-leaf fallback,
runs leaves sequentially on one warm shared worktree with leaf self-verification,
delegates to the `delegate-implementer` floor calibrated by difficulty
(mechanical disjoint leaves down by default, hard core kept), and frames the
interface + test-contract spine review as a named-condition recommendation, not a
gate. Kept `role: worker`, the `worker-stop-protocol` include, the `ExploreAgent`
/ `SpawnIdiom` spawn variables, and the route-allocation review-mapping paragraph
(single/correctness/fit/test) verbatim. `## Inputs` and `## Constraints` are
otherwise unchanged from the prior body (only `## Execute` diverged); no changed
Invariants/Constraints line required the skill-authoring checklist beyond the
inputs/constraints already in place.

Retired `ticket-worker-escalated` across all three rsrc mirrors and regenerated
both manifests: canonical via `WSRSRC_REGEN`, the `agents-plugin-wsflow` mirror
via `WS_REGEN_WSFLOW_RSRC`, and the `agents-plugin-pi` mirror resynced
byte-identically from canonical. The default `ticket-worker` body is untouched.

Updated the tests that hard-coded the three stems: dropped `ticket-worker-escalated`
from the Python three-stem tuples in both suites and from the Go golden render
case (`TestPlaybookRenderGoldenTicketWorker`), and removed
`TestTicketWorkerVariantsDifferOnlyByTier` (its byte-identity premise is exactly
what this phase overturns). Added `test_two_worker_bodies_diverge_in_behavior_not_just_tier`
(agents-plugin) and `test_wsflow_elevated_body_is_mini_lead_and_escalated_retired`
(agents-plugin-wsflow) to pin the behavioral divergence and the retirement.

Verification (full output read):
- `go test ./...` (agents-plugin-tool): all 16 packages `ok`, incl. the manifest,
  wsflow-mirror, and pi-mirror drift guards and the golden worker-render test.
- `python3 -m unittest discover agents-plugin/tests`: 73 OK.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 13 OK.
- Review, partitioned correctness/fit/test: correctness clean; fit one
  non-blocking Minor (accepted); test two Important coverage gaps, both fixed in
  round 1; round-2 verification clean, no Critical at any point.

Decisions (recorded, not escalated):
- Retired the escalated body rather than keeping a tier-only third body; xlarge
  reuses the elevated prose via the render `tier_override` — that wiring is
  Phase 2, so `lead-run.md` and `TestPlaybookPrintLeadRunWorkerTierPolicy` are
  intentionally left still naming `ticket-worker-escalated` (the deliberate
  Phase 1/2 boundary).
- Resolved two unenumerated consumers the ticket's ws+wsflow scope omitted: the
  `agents-plugin-pi` rsrc mirror (guarded by `TestPiMirrorUpToDate`), resynced
  from canonical without a version bump; and two Go tests keyed off the retired
  three-body structure, edited/removed since Phase 1's change overturns their
  premises.
- Accepted fit's lone Minor: kept `## Execute` step 4 (leaf mechanics +
  difficulty calibration) as one step rather than splitting it, since both halves
  are the single "how to delegate a leaf" concern and splitting would make step
  3's "the same review render step 5 uses" cross-reference stale for no gain.

Commits: a6f54fe6 (body + mirrors + tests), e5e93975 (round-1 review-gap fixes).

### Phase 2: Rewrite lead-run dispatch and escalation to elevated + xlarge override

Depends on Phase 1 (bodies exist) and on `260919-feat-ws-playbook-render-tier-override`
landing (the render tool must accept the tier override).

- Rewrite the `lead-run.md` dispatch/escalation table (~:56-60) and the render/spawn
  prose (~:67-70): `medium → ticket-worker`, `large → elevated`,
  `xlarge → elevated` rendered with `tier_override: xlarge`. The stop-(e)
  escalation ladder becomes `medium → large (elevated)` then
  `large → elevated + xlarge override` rather than naming a separate escalated
  body; xlarge's (e) still goes to the user. Mirror to wsflow.
- Update every remaining consumer of the retired `ticket-worker-escalated` stem.
  A `grep -rl ticket-worker-escalated` at authoring time returned exactly six
  files, all already named across these two phases: the two test files
  (`test_skill_dispatch_contracts.py`, `test_wsflow_skill_bundle.py`), the two
  `lead-run.md` copies (agents-plugin + agents-plugin-wsflow), and the two
  `rsrc/manifest.json` copies. Re-run the grep before closing the phase and treat
  any hit outside this set as an unenumerated consumer to resolve.

Verification: dispatch-prose tests (incl. the wsflow mirror at
`test_wsflow_skill_bundle.py`) pass against the new table; a render of `elevated`
with `tier_override: xlarge` surfaces the xlarge recommended tier/model.
