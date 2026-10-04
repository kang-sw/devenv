---
title: Explicit post-compaction continuation flag for the Pi lead
related:
  261003-feat-pi-lead-resume-after-midrun-compaction: preserve existing route-dependent continuation and completion gates
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
  style. Leave existing required prose fields intact. The lead sets true when
  it knows there is work to continue after compaction.
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
- This invocation captures accepted todo backlog only; no fact-populator,
  design/completeness review, ready promotion or implementation is requested.

## Prior Art

- agents-plugin-pi/src/lead-compaction.ts: search leadProseParameterSchema
  and LEAD_PROSE_SECTIONS; the schema builder currently describes required
  prose strings and needs to support an optional boolean without changing
  their requiredness.
- agents-plugin-pi/src/goal-loop.ts: search CompactionOperation,
  resumesAfterCompaction, resumeOwed, sendOwedResume and the ws-compact execute
  handler. The operation retains route state across the native abort.
- agents-plugin-pi/test/goal-loop.test.ts: existing route-dependent resume,
  active-goal, held-push ordering and completion-callback fixtures.
- 261003-feat-pi-lead-resume-after-midrun-compaction: the completed contract
  defines hard/autonomous resume, advisory/reroute non-resume and active-goal
  behavior; its f81df410d Edition moves the actual resume send to onComplete.
  This ticket adds an explicit opt-in without replacing that default contract.

## Phases

### Phase 1: Add explicit continuation intent to ws-compact

**Intended behavior.** Expose the optional boolean in the lever schema with a
concise description of its remaining-work intent. Snapshot true onto the
compaction operation before the abort, and OR it with existing route eligibility
through the existing completion gates and sender. Keep the flag optional and
additive-neutral when false or absent. Keep any lever/guide explanation
consistent with explicit opt-in rather than unconditional advisory resumption.

**Deferred scope.** Arrival-time inference, child message delivery/processing
acknowledgments, queue persistence, wake-reservation changes, early-release race
repair, new conversational-state gates, installed-host fixes, changing the
advisory policy, and universal post-compaction execution guarantees.

**Verification boundary.** Test the optional boolean schema and preservation
of existing required prose fields. Test advisory and reroute calls with true
through successful existing idle completion, plus false/omitted preserving
non-resume. Preserve hard/autonomous continuation with false/omitted; retain
existing goal-active, failure, shutdown, non-idle, once-per-operation callback
and child-wake-before-resume ordering checks. Verify the boolean survives the
abort's preparation-state reset and remains isolated to its own operation.
Run relevant Pi tests and the full Pi suite before landing; report any live
acceptance limits rather than asserting model compliance or incident repair
from these tests.
