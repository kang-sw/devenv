---
title: Resume the Pi lead's turn after a compaction that aborted its run mid-work
related:
  261002-feat-pi-lead-ws-owned-compaction: origin; defines the lever, the preparation triggers, and goal-only re-injection
  260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction: constrains; its deferred, idle-gated, once-per-operation release is the send pattern to reuse
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 01e6d69083f2dc7a
---

# Resume the Pi lead's turn after a compaction that aborted its run mid-work

## Background

The `ws-compact` lever (`agents-plugin-pi/src/goal-loop.ts`) calls
`ctx.compact()`. Pi's `AgentSession.compact()` starts with `await this.abort()`
and never retries or continues afterwards (`agent-session.js` in
`@earendil-works/pi-coding-agent`). Every lever compaction therefore ends the
lead's run.

The only post-compaction send today is the goal reminder (`pendingRearm` ->
`releaseAfterCompaction` -> settle timer -> `fireReminder`), which runs only
while a goal is active. With no goal, nothing starts a turn after compaction
(pinned by `test/goal-loop.test.ts` around lines 1023-1055). In dogfooding,
when the hard-threshold steer interrupts the lead mid-task, the lead stops
after compaction and the work stalls until the user speaks.

Which route led to the lever call is not recorded:
- `sendPreparation` knows the trigger kind (`advisory`, `hard`, `reroute`)
  but stores it only in the custom message's `details`.
- `preparation`, `advisoryFired`, `hardFired` and `pendingReroute` are all
  cleared in the `session_compact` handler before the deferred release runs.

## Decisions

- **Route-dependent resume, goal-less only.**

  | route into the lever call | resume after compaction |
  |---|---|
  | hard-threshold steer (`hard`) | yes |
  | lead's autonomous call, no preparation pending | yes |
  | advisory nudge at `agent_end` (`advisory`) | no |
  | user `/compact` reroute (`reroute`) | no |
  | Pi auto/overflow fallback (no lever) | no change |
  | any route while a goal is active | no change: the goal reminder already resumes |

  - Resume routes are the ones whose run was mid-work when the abort landed.
    The advisory nudge fires after the lead already ended its turn, and a
    reroute follows the user's own `/compact`, so in both the next move is
    the user's.
  - Rejected: resuming after every lever compaction regardless of route.
    It adds a revive-only turn on the advisory and reroute paths.
- **Record the route at lever time.**
  - `sendPreparation` stores the trigger kind next to the existing
    `preparation` flag. The kind is cleared wherever `preparation` is cleared
    today: at `agent_end` and in the `session_compact` handler.
  - "Preparation pending" means exactly that `preparation` is true when the
    lever's `execute` runs. The lever then copies the stored kind onto the
    compaction operation before `ctx.compact()`.
  - When `preparation` is false at that point, the call is autonomous. This
    includes a preparation the lead ignored, whose run already ended and
    cleared it.
  - The copy happens before `ctx.compact()` aborts the run, so the abort's
    own `agent_end` cannot erase it. The copy on the operation survives the
    `session_compact` trigger reset, so the release path can still read it.
- **The resume message.**
  - One user-role `followUp` message that triggers a turn, to this effect:
    "Compaction complete. Invoke `lead-revive` with session key `<key>`,
    then continue the immediate next step; if it awaits the user, end your
    turn."
  - The "end your turn" clause covers an autonomous lever call made at a
    natural stopping point.
  - No system- or developer-role injection (261002).
- **Send through the existing release pattern (260906).**
  - Deferred out of the `session_compact` handler (`setImmediate`) and
    backstopped by the lever's `onComplete` / `onError`, released once per
    operation id.
  - Sent only when `ctx.isIdle()` and not shutting down. When the session is
    not idle (for example, owner input queued during compaction already
    started a run), no resume is sent.
  - Within the release: idle check first, then the held-push flush
    (`flushHeldPushes`) as today, then the resume send. Because the resume
    is a `followUp`, it queues behind any run a push wake starts.
  - A failed compaction (`onError`) sends no resume; the existing failure
    notice stands.
- **Text updates.** `lead-compact-guide.md` step 3 and the `ws-compact` tool
  description, which today promise resumption only under an active goal,
  state the new behavior.

## Constraints

- Goal-active behavior is unchanged: no second message beside the goal
  reminder.
- Pi's own auto and overflow compaction paths are untouched.
- Spawned worker, explore, and fork sessions are untouched (lead only, as in
  261002).

## Prior Decisions

- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "No system- or developer-role injection. ws messages stay user-role." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "The lever works without an active goal. This reverses 260913-bug-ws-pi-inactive-goal-compact-and-continue-aborts's inactive-goal rejection for the lever" — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (4c8175de, commit): "the single lever now works without an active goal ... while keeping goal-loop state untouched when no goal is active (user-confirmed)" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "Scope: the lead session only. Spawned worker and explore sessions keep Pi's native compaction" — bearing: constrains
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (81463a7d, Result): "goal-loop.ts owns releaseAfterCompaction (idempotent on the flag), called from deferred session_compact / session_compact_failed (setImmediate), the lever's onComplete / onError, and the agent_start backstop" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (f065de0d, commit): "the preparation is sent from session_compact_failed after the deferred release flushes held pushes, never from inside the compaction event" — bearing: constrains
- 260913-bug-ws-pi-inactive-goal-compact-and-continue-aborts (2026-09-13, Resolution): "Rejected inactive goal-compact-and-continue calls synchronously at the authoritative Pi lever boundary" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/lead-compact-guide.md, agents-plugin-pi/test/goal-loop.test.ts |
| scope.surface | internal | adapter-internal state plus ws-compact description and guide text; no exported symbol named |
| scope.new_public_symbol | no | none named by the ticket |
| scope.new_type_contract | no | none; route record is private adapter state |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts (existing no-goal test near line 1023 to be updated; new cases added there) |
| complexity.reuse_points | confirmed | releaseAfterCompaction, beginCompaction operation ids, sendPreparation, fireReminder send path in agents-plugin-pi/src/goal-loop.ts#L1001-L1043 |
| complexity.side_effect_risk | moderate | new post-compaction send beside the goal reminder path; must not double-send under a goal or when non-idle |
| risk.correctness | moderate | once-per-operation release across onComplete and session_compact, idle gating, and trigger reset ordering in session_compact (goal-loop.ts#L1367-L1372) |
| risk.fit | low | follows the existing release pattern and the ticket's decision table |
| risk.test | moderate | seven no-goal cases plus goal-active case depend on the fakePi/fakeCtx timing of setImmediate and idle state |
| risk.security_or_contract | low | lead-only, user-role message; ws-compact description text changes but no protocol change |

## Phases

### Phase 1: Route record and goal-less resume

- Implement the route record, the resume send, and the text updates per
  Decisions in `agents-plugin-pi/src/goal-loop.ts` and
  `agents-plugin-pi/lead-compact-guide.md`.

Done when:

- `npm test` in `agents-plugin-pi` passes.
- Tests pin, with no goal active:
  - a lever compaction after a `hard` preparation sends exactly one resume
    `followUp` after release;
  - an autonomous lever compaction (no preparation) sends exactly one resume;
  - a lever compaction after an `advisory` preparation sends nothing;
  - a lever compaction after a `reroute` preparation sends nothing;
  - `onComplete` plus `session_compact` for the same operation still sends
    once;
  - a non-idle session at release sends nothing;
  - a failed compaction sends no resume;
  - a preparation whose run ended before the lever call (`preparation`
    cleared) counts as autonomous and resumes;
  - the resume is sent after the held-push flush within the same release.
- The `ws-compact` tool description and `lead-compact-guide.md` step 3 no
  longer limit resumption to an active goal. A test asserts the tool
  description's text.
- Tests pin that with a goal active, only the goal reminder is sent, for a
  `hard` route too.
- The existing no-goal test at `test/goal-loop.test.ts` ~1023-1055 is
  updated to the new contract rather than deleted.
