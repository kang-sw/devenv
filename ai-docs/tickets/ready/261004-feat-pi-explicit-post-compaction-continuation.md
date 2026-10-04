---
title: Explicit post-compaction continuation flag for the Pi lead
related:
  261003-feat-pi-lead-resume-after-midrun-compaction: preserve existing route-dependent continuation and completion gates
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 3b34f4730d75fd6b
sage-review-design-reviewed: 3b34f4730d75fd6b
---

# Explicit post-compaction continuation flag for the Pi lead

## Background

The lead can receive information while doing ordinary compaction preparation,
then invoke ws-compact as its final command with known work still to do. This
preparation run is distinct from Pi-native compaction. Whether information is
preserved in the summary does not itself request another processing turn.

The user wants the lead to state its continuation intent explicitly rather
than infer it from child-message arrival timing or compaction route. This is
an additive API capability, not a claimed repair of a reproduced lost-wake
race. The reported incident's exact queue/delivery state remains unverified.

## Decisions

- **D1 — Optional boolean argument.** Add `continue_after_compact?: boolean`
  to the Pi lead's ws-compact lever, following its existing snake_case argument
  style. Leave existing required prose fields intact. The lead sets true only
  when it holds known remaining work that does not await the user; work that
  needs a user answer first leaves the flag unset, matching the resume
  message's "if it awaits the user, end your turn". Only a strict
  `continue_after_compact === true` (after Pi's own argument coercion in
  pi-ai `validateToolArguments`, which already maps "true"/1 to true) opts
  in; there is no adapter-side coercion. Compose the boolean into the lever's
  parameter schema at the ws-compact registration and keep
  `leadProseParameterSchema` prose-only: `LEAD_PROSE_SECTIONS` also drives
  the fallback summary prompt, which must not gain the flag.
  Rejected: setting true for any remaining work, including work awaiting a
  user answer (a flag-driven resume would pre-empt the user's turn).
- **D2 — Additive resume eligibility.** Treat omitted and false identically:
  preserve current behavior. True adds an explicit request to the existing
  eligibility condition, not a replacement for it:

  ```text
  resume eligibility = resumesAfterCompaction(operation.route)
                       OR operation.continueAfterCompact
  ```

  False cannot disable continuation that an existing hard/autonomous route
  already requests. True lets an otherwise non-resuming advisory/reroute call
  request continuation through the existing completion gates.
- **D3 — Reuse existing continuation.** Carry the explicit boolean on the
  compaction operation before ctx.compact aborts the invoking run. Use the
  existing resumeOwed/sendOwedResume completion path, not a second sender.
  Preserve success, shutdown, idle and active-goal gates; an active goal keeps
  its existing reminder path without an additional flag-driven resume.
- **D4 — Caller intent, not delivery guarantees.** The flag means "I have work
  after compaction," including work the lead already learned from a steered
  message. It does not promise receipt or processing of every message arriving
  during preparation/compaction. Leave held child pushes, wake accounting,
  host follow-up queues and their existing ordering unchanged. Do not promise
  exactly one subsequent model run when child wake and resume are both owed.
- **D5 — Minimal scope.** Exclude the proposed early-release timing/wake-
  reservation repair. The user explicitly chose the simple eligibility OR
  rather than extending this capability into a lost-wake guarantee. Also
  reject automatically enabling continuation for every advisory: the lead
  chooses the flag based on known remaining work.

## Constraints

- Lead-only Pi lever; child native compaction and Pi auto/overflow paths are
  unchanged. Advisory thresholds, delivery/rearming and the informational
  active-human-discussion guidance remain unchanged.
- Retain the existing user-role continuation message and its revive/next-step
  behavior. A flag is not a new continuation prompt or workflow.
- Read ai-docs/manuals/skill-authoring.md before editing lever guidance or
  preparation prose; read ai-docs/manuals/shipped-surface-boundary.md before
  editing shipped text. Apply any matching implementation-convention rows if
  shared plugin/wsflow paths become necessary; no shared host-neutral workflow
  change is requested.
- Read the applicable installed Pi API documentation completely and relevant
  Markdown cross-references before changing tool schema or compaction hooks.

## Prior Art

- agents-plugin-pi/src/lead-compaction.ts: search leadProseParameterSchema
  and LEAD_PROSE_SECTIONS; the schema builder describes the required prose
  strings and stays prose-only (D1); the lever composes the optional boolean
  on top at its registration.
- agents-plugin-pi/src/goal-loop.ts: search CompactionOperation,
  resumesAfterCompaction, resumeOwed, sendOwedResume and the ws-compact execute
  handler. The operation retains route state across the native abort.
- agents-plugin-pi/test/goal-loop.test.ts: existing route-dependent resume,
  active-goal, held-push ordering and completion-callback fixtures.
- 261003-feat-pi-lead-resume-after-midrun-compaction: the completed contract
  defines hard/autonomous resume, advisory/reroute non-resume and active-goal
  behavior; its f81df410d Edition moves the actual resume send to onComplete.
  This ticket adds an explicit opt-in without replacing that default contract.

## Prior Decisions

- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit 26047862): "User confirmed route-dependent resume: hard steer and autonomous lever calls resume; advisory and /compact reroute do not; goal-active and Pi auto/overflow paths unchanged." — bearing: constrains
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit 26047862): "Rejected: resuming after every lever compaction (adds revive-only turns)." — bearing: constrains
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit f81df410): "Every prior gate stays: idle at release, not failed, no goal at lever time (generation undefined), route hard/autonomous; shutdown and goal-active are checked at both mark and send" — bearing: supports
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit ea1fd079): "Route lives on the operation object (CompactionOperation.route) because the abort's agent_end and the session_compact handler clear preparation/preparationKind before the deferred release runs" — bearing: supports
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit ea1fd079): "Added an explicit failed parameter to releaseAfterCompaction instead of inferring failure from failureReason: session_compact_failed for an aborted compaction carries errorMessage undefined" — bearing: constrains
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, Decisions D2): "Preserve non-advisory behavior. Keep existing advisory thresholds, once-per-crossing/rearming behavior, delivery and model wake semantics. Do not add a deterministic conversational-state gate" — bearing: constrains
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, commit 1cd1cd0d): "D1 is a prompt-level safe-autonomy boundary: active human discussion and awaited human answers or clarification independently prohibit autonomous advisory compaction; a question pause is not permission." — bearing: supports
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, Result): "goal-loop.ts owns releaseAfterCompaction (idempotent on the flag), called from deferred session_compact / session_compact_failed (setImmediate), the lever's onComplete / onError, and the agent_start backstop" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/lead-compact-guide.md, agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| scope.surface | internal | adapter-internal CompactionOperation state plus ws-compact tool parameter schema and description text; resumesAfterCompaction (goal-loop.ts#L279-L281) stays unchanged |
| scope.new_public_symbol | no | none named; the new lever argument continue_after_compact is a tool-schema field, not an exported symbol |
| scope.new_type_contract | yes | optional boolean continue_after_compact on the ws-compact parameter schema, and a new field on the private CompactionOperation type (goal-loop.ts#L267-L272) |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts (resume cases near L2363) and agents-plugin-pi/test/lead-compaction.test.ts (schema case near L69) |
| complexity.reuse_points | confirmed | resumeOwed and sendOwedResume (goal-loop.ts#L1043-L1049), the releaseAfterCompaction eligibility check (goal-loop.ts#L1087), and the lever's onComplete (goal-loop.ts#L1546) read in full |
| complexity.side_effect_risk | moderate | true flag enables a resume send on advisory/reroute routes beside held-push flush and child wake; must not double-send under an active goal or non-idle release |
| risk.correctness | moderate | flag must be snapshotted before ctx.compact aborts the run and survive the preparation-state reset; OR with route eligibility without disabling hard/autonomous resume |
| risk.fit | low | follows the existing operation-field and sendOwedResume pattern; optional boolean composed at the registration keeps leadProseParameterSchema prose-only |
| risk.test | moderate | resume ordering depends on fakePi/fakeCtx idle and setImmediate timing; held-push, goal-active, shutdown and once-per-operation cases must stay isolated per operation |
| risk.security_or_contract | low | lead-only lever, optional additive argument, no protocol change; tool description and guide step 3 text change |

## Phases

### Phase 1: Add explicit continuation intent to ws-compact

**Intended behavior.** Expose the optional boolean in the lever schema with a
concise description of its remaining-work intent. Snapshot true onto the
compaction operation before the abort, and OR it with existing route eligibility
through the existing completion gates and sender. Keep the flag optional and
additive-neutral when false or absent. Update both text surfaces that now
state "after the advisory nudge or a user /compact, the next move is the
user's" so they describe the flag as explicit opt-in under D1's set condition,
not unconditional advisory resumption: the ws-compact tool description at the
lever registration in agents-plugin-pi/src/goal-loop.ts, and step 3 of
agents-plugin-pi/lead-compact-guide.md. Also update the goal-loop.ts code
comments that describe resume as route-only (the `CompactionOperation` doc,
the `resumesAfterCompaction` doc and the file header) so they name the flag
as the other eligibility term; `resumesAfterCompaction` itself stays
unchanged.

**Deferred scope.** Arrival-time inference, child message delivery/processing
acknowledgments, queue persistence, wake-reservation changes, early-release race
repair, new conversational-state gates, installed-host fixes, changing the
advisory policy, and universal post-compaction execution guarantees.

**Verification boundary.** Test the optional boolean schema and preservation
of existing required prose fields, with `leadProseParameterSchema` and the
fallback summary prompt unchanged. The composed schema exists only at the
goal-loop registration, so test it in goal-loop.test.ts through the fake pi's
captured ws-compact definition; the lead-compaction.test.ts schema test stays
as the prose-only regression check. Test advisory and reroute calls with true
through successful existing idle completion, plus false/omitted preserving
non-resume. Preserve hard/autonomous continuation with false/omitted; retain
existing goal-active, failure, shutdown, non-idle, once-per-operation callback
and child-wake-before-resume ordering checks. Verify the boolean survives the
abort's preparation-state reset and remains isolated to its own operation.
Run relevant Pi tests and the full Pi suite before landing; report any live
acceptance limits rather than asserting model compliance or incident repair
from these tests.
