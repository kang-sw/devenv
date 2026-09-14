---
title: "Pi adapter: recursive worker delegation with monotonic capabilities and subtree-aware completion"
related:
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: superseded role-specific design; recursive worker residual moves here
  260908-feat-ws-pi-agent-session-disk-retention: child registry and sidecar retention reused across nested ownership
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: independent owner-held state must compose with waiting-on-children
  260911-feat-ws-pi-held-push-batch-delivery: worker-local descendant pushes use the same turn-boundary delivery machinery
  260909-research-ws-refoundation-evidence-audit: refoundation authority for worker interpretation and stop conditions
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6609673a859b6719
sage-review-completeness-reviewed: 6609673a859b6719
completed: 2026-09-13
---

# Pi adapter: recursive worker delegation with monotonic capabilities and subtree-aware completion

## Background

The refounded worker playbooks require an implementation worker to spawn reviewers and researchers, but the Pi adapter's `full-worker` tool group currently omits every `ws-agent-*` management tool and worker-local child pushes are discarded. The registry, session-path continuation, sidecar recovery, and direct-parent ownership mechanisms already support persistent children; the hard tool and push policies prevent workers from using them.

A separate orchestrator runtime role is no longer needed after refoundation. The remaining requirement is ordinary workers that may delegate within a bounded depth, continue the same child through `ws-agent-send`, and wait for their subtree without being reported or parked as complete merely because their own Pi turn locally settled.

`ws-execute` workers already persist and can be resumed by agent ID; a blocked `ws-worker-exec` call is an approval wait, not conversation terminality. This ticket therefore does not add a second blocking-Explore API or a new execute-worker lifecycle.

## Decisions

- **No orchestrator role.** Extend the existing worker ownership model; do not add another runtime role or move lead playbooks into a mini-lead process.
- **Depth is an independent budget, not a fixed role tree.** Root lead is depth 0. The default maximum delegation depth is 2 and is represented by one policy value rather than hard-coded role branches. A depth-1 worker may spawn a depth-2 reviewer, persistent Explore, bounded delegate, or ordinary full worker. A depth-2 full worker keeps its normal execution tools but receives a terminal profile with no child-management tools. Raising the policy later must not require a lifecycle redesign.
- **Root lead is the only privilege-expansion exception.** The curated depth-0 lead may spawn a depth-1 worker whose execution authority exceeds the lead's active tool surface. Every spawn producing depth 2 or greater must be monotonically non-escalating relative to its direct parent.
- **Capability ceiling, not active-name equality.** Persist an adapter-owned capability ceiling for native tools, bridged ws tools, ws session authority, and remaining delegation depth. A descendant's requested authority must be a subset of its parent's ceiling even when lazy/deferred tools are not currently active. Dynamic activation can never exceed the inherited ceiling.
- **Fail loudly on escalation.** If a requested child profile or playbook class requires authority outside the parent's ceiling, reject the spawn before allocating a child or session. Do not silently trim a writer into a read-only worker. Normal max-depth profile calculation explicitly removes child-management tools and is not treated as an error when the selected terminal playbook does not require children.
- **Read-only remains read-only.** A read-only parent that is allowed to delegate may create only a read-only child with no stronger ws session capability. In particular it cannot obtain write, mutating Git/ticket, lead-only, or broader bridge authority indirectly through a child.
- **Authorize classes from trusted provenance.** Permit worker, reviewer, Explore, and bounded-delegate classes below the root; lead-control playbooks and recursive forks remain root-only. Validate the class from ws render provenance or equivalent adapter-owned metadata, never from an arbitrary filename, user prompt, or self-declared role. Reject a playbook that requires further children when its remaining depth budget is zero.
- **Expose persistent direct-child management to eligible workers.** Depth-1 worker profiles receive `ws-agent-spawn`, `ws-agent-send`, `ws-agent-list`, `ws-agent-stop`, and `ws-agent-transcript`, plus persistent Explore. Their local `RpcAgentRegistry` owns only immediate children; child events route into the parent worker session one edge at a time. Explore remains a normal persistent child rather than a blocking one-shot special case.
- **Local Pi settle is not semantic completion.** Pi's raw `agent_settled` cannot be cancelled. Track separate own-turn, expected-descendant-report, and aggregate subtree states. A parent with outstanding descendant obligations becomes `waiting-on-children`: suppress its upstream model-visible settled/final, prevent parking and eviction, and keep it visible as outstanding rather than actively running.
- **Obligations are explicit.** Create an expected-report obligation only after a child prompt is accepted. Clear it on an accepted terminal child final, explicit stop/failure disposition, or another explicitly defined terminal outcome—not on progress, tool-start `terminalThisTurn`, or raw child settle alone. A plain child settle wakes the parent to decide whether to follow up or explicitly finish that obligation.
- **Premature final is rejected.** If a parent submits `kind: "final"` while descendant obligations remain, do not retain and later release that stale final. Reject or invalidate it, wake the parent as descendant results arrive, and require a fresh accepted final that synthesizes the completed subtree.
- **Parking is aggregate-aware.** Park only when the parent's own turn is settled, the subtree is quiescent, no expected descendant report remains, terminal policy is satisfied, and neither thread-bound nor owner-held state applies. Parent shutdown must not be used as the normal way to stop a still-working descendant subtree.
- **Persist policy and waiting state.** Depth, capability ceiling, child profile, descendant obligations, and waiting-on-children state survive automatic parking, sidecar recovery, and same-lead restart. Recovery notices and child management remain available to a worker that owns revived dormant children.
- **Compose with owner control and push batching.** `waiting-on-children` is independent of `threadBound` and the planned `ownerHeld` state. Descendant reports use worker-local turn-boundary batching; only the direct parent is awakened. The root lead receives one final after edge-local subtree aggregation, not every grandchild event.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts#L150-L225, agents-plugin-pi/src/process-role.ts#L28-L80, agents-plugin-pi/src/agent-sidecar.ts#L95-L165, agents-plugin-pi/src/index.ts#L520-L533 |
| scope.surface | public-interface | agents-plugin-pi/src/spawner.ts#L1898-L1910 delivers model-visible child reports; child tool availability and completion behavior are observable |
| scope.new_public_symbol | no | none; the policy and persisted record fields can remain adapter-internal |
| scope.new_type_contract | yes | persisted depth, capability ceiling, descendant obligations, and aggregate subtree state extend RpcAgentRecord and sidecar contracts |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts#L203-L216, agents-plugin-pi/test/push-wake.test.ts, agents-plugin-pi/test/fork-lifecycle.integration.test.ts, and agents-plugin-pi/test/agent-sidecar.test.ts |
| complexity.reuse_points | confirmed | RpcAgentRegistry and tool groups in agents-plugin-pi/src/spawner.ts#L150-L225; dormant send and sidecar recovery in agents-plugin-pi/src/agent-sidecar.ts#L95-L165 |
| complexity.side_effect_risk | high | recursive execution changes process lifetime, tool authority, parking, recovery, and completion propagation |
| risk.correctness | high | false settle or stale final can stop descendants or report incomplete work; stale obligations can strand a parent |
| risk.fit | high | current full-worker and worker push gates forbid worker-local persistent child management (agents-plugin-pi/src/spawner.ts#L150-L225, #L1300-L1310) |
| risk.test | high | depth, privilege monotonicity, nested races, restart recovery, and exact-once final propagation need integration coverage |
| risk.security_or_contract | high | recursive spawning must not let a restricted parent create a more privileged child or ws session |

## Spec Impact

Extend `pi-adapter-runtime` with delegation depth, monotonic descendant capability ceilings, trusted child-class admission, worker-local persistent child ownership, waiting-on-children semantics, and aggregate-completion parking/final rules. Keep raw Pi `agent_settled` distinct from ws semantic completion.

## Phases

### Phase 1: Enable bounded recursive workers and subtree-aware completion

Propagate a default maximum depth of 2 and an adapter-owned capability ceiling through child launch, bridge/tool-profile calculation, registry records, session ownership, and sidecars. Give eligible depth-1 workers persistent direct-child management and worker-local pushes. Permit full workers at the terminal depth while removing their management surface; enforce trusted playbook/profile admission and fail before allocation on depth or capability escalation. Keep the root lead as the sole privilege-expansion exception.

Add expected-descendant obligations and aggregate lifecycle state. Suppress upstream semantic settle/final and automatic park while a parent is waiting on children; deliver child events only to the direct parent; reject premature parent finals; require a fresh accepted final after subtree quiescence; and propagate completion one edge at a time. Compose this state with thread-bound, owner-held, compaction, push batching, alias/cap eviction, and restart recovery without overloading any of them.

Verification covers: root privilege expansion; depth-1 worker spawning persistent Explore, reviewer, bounded delegate, and terminal full worker; depth-2 spawn rejection; read-only-to-writer, delegate-to-lead, bridged-tool, lazy-tool, and ws-session escalation rejection before allocation; terminal profile derivation; child follow-up on the same session; worker-local progress/question/final delivery; parent raw settle while waiting without upstream settle or park; plain child settle requiring parent disposition; premature parent final rejection; fresh final after subtree quiescence; exact-once root final; independent owner-held/thread-bound behavior; registry-cap protection; shutdown/restart sidecar recovery with outstanding obligations; and a real `lead → ticket worker → reviewer/Explore → worker final → lead` run. No special blocking Explore implementation or new orchestrator role is introduced.

### Result (f7b3f670) - 2026-09-13

Implemented bounded recursive worker ownership in `19f86bee` and closed the independent review findings in `a28fbc6d` and `f7b3f670`. Depth and capability ceilings, trusted render admission, direct-child management, edge-local obligations, fresh-final gating, aggregate-aware parking, sidecar recovery, and direct-parent push delivery now compose without adding an orchestrator or blocking Explore API.

Round-one review found and fixed foreign session-key access by lead-capability children, premature obligation release before direct-parent enqueue, stopped-child final-gate blockage, stranded exit obligations, and duplicated authority/private-write inventories. Round two cleared correctness and fit; its remaining Important test gaps were dispositioned after the review cap by composing reviewer final delivery with repeated stop and fresh parent final, and by exercising filesystem sidecar recovery of an undelivered obligation. The round-two recovery observation was also fixed by restoring only digest-validated render provenance and repopulating its descendant session keys.

Verification: `cd agents-plugin-pi && npm test -- --test-reporter=dot` passed the full suite after the final source and test inputs; focused recursive-worker and production fork lifecycle suites also passed. This ticket-worker run itself exercised the required real `lead → worker → partitioned reviewers → worker final → lead` path.

Decisions: an explicit nested stop is its synchronous disposition and emits no redundant worker-local stopped follow-up; terminal report/exit obligations clear only when Pi accepts direct-parent enqueue and remain recoverable otherwise; root privilege expansion is represented by no delegation policy, not by a descendant carrying lead authority.


## Resolution (2026-09-13)

Implemented bounded recursive worker delegation and subtree-aware completion, closed correctness/fit findings in two review rounds, dispositioned remaining test coverage after the review cap, and verified the full Pi suite plus the live worker-to-reviewer lifecycle.
