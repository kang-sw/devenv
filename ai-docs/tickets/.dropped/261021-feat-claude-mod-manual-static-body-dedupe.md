---
title: "Claude mod: cut the static workflow manual body when an earlier full result is still in the conversation"
related:
  261018-feat-claude-mod-ws-manual-injection: extends its state-only cut and repeat-read dedupe
dropped: 2026-10-31
---

# Claude mod: cut the static workflow manual body when an earlier full result is still in the conversation

## Background

The Claude mod's workflow manual feature (`agents-plugin/hooks/claude-mod/manual.ts`)
shrinks main-thread `workflow_manual` results in two steps, applied in that
order by `read()`:

1. `stateOnly` cuts the static manual body and returns session state alone,
   but only while the `snapshot` flag holds: the conversation's system prompt
   snapshot carries the injected `ws:workflow-manual` section.
2. `dedupe` replaces a result with a pointer when the whole result text is
   identical to an earlier full result still in the conversation.

A `workflow_manual` result is the static manual body plus session state
(agenda, todos, notes). A repeat call usually follows a state change, so the
whole text differs and step 2 misses; without the snapshot, step 1 is off and
the full manual is rendered again on every call.

The snapshot is never set where Claude Code seats its built-in
`cc-plugin-sec-default` mod (machines with managed settings or a
`managed-mcp.json`, and Team or Enterprise sign-ins): that mod continues
`prompt.compose` and `session.compact` past the user tier, so the mod's
compose hook never runs and the engine logs `ws: its prompt.compose hook was
skipped. "cc-plugin-sec-default" runs first and sent the event on past it.`
The `tool.call` hook that runs `read()` is not held, so the rewrite path
still runs there.

## Decisions

- **D1. Static-body visibility extends to the conversation.** The state-only
  cut applies when the snapshot carries the section OR an earlier main-thread
  `workflow_manual` result still present in the conversation carries the same
  static body. The cut is a dedupe of the static part alone; the dynamic
  state is always returned fresh.
- **D2. No re-render escape hatch.** Unlike `isRepeat`'s
  `full === 1 && pointers === 0` rule, consecutive state-only results do not
  periodically fall back to the full manual. The visibility check reads the
  conversation itself, so a body that left the conversation is re-rendered
  by that check; a state-only result still carries fresh state, so the model
  has no reason to re-call for it.
  Rejected: re-render the full manual after a state-only result is followed
  by another same-key call - one more rule for a case the visibility check
  already covers.
- **D3. Only full results count.** Only earlier results carrying the static
  body count as visible; state-only and pointer results never count, since
  they carry no body.
- **D4. Exact-repeat dedupe stays.** The existing `dedupe` is unchanged and
  still runs after the cut (the current `read()` order), so an identical
  state-only repeat still collapses to a pointer.
- **D5. Per-call conversation read.** Visibility is checked on every call
  from `$.session.messages({ as: 'api' })`. A per-call read self-corrects at
  compaction, tool-result clearing, `/clear`, resume, and revive, and
  `dedupe` already pays the same read.
  Rejected: a flag maintained from `session.compact` or `prompt.compose`
  events - `cc-plugin-sec-default` skips both user-tier hooks, so such a
  flag never updates where this feature matters most.
- **D6. Match by span.** An earlier result matches when its `cutStaticBody`
  span equals the fresh response's span. The span excludes the state that
  changes between calls and, per the `stateOnly` comment, carries the key
  line, so a revive's new key or a manual-version change re-renders once.
  Rejected: an equal whole-result hash - misses every call whose state
  changed, which is the common case.
- **D7. Separate lead line.** The conversation-sourced cut gets its own lead
  line pointing the model at its earlier `workflow_manual` result; the
  snapshot path keeps `STATE_ONLY_LINE`.
  Rejected: reusing `STATE_ONLY_LINE` - it says the manual is in the system
  prompt, which is false on this path.
- **D8. Failed fresh cut returns full.** When the fresh response's cut fails,
  the conversation path returns the full result.
  Rejected: the `workflow_state` fallback the snapshot path uses - without a
  fresh cut there is no span to match, so this path cannot vouch for the
  body.
- **D9. Key delivery in sec-default sessions is out of scope.** No
  `prompt.submit` fallback for the session key line, and no investigation of
  how the start-time minted key interacts with the model's own
  `obsidian-latch` bootstrap there. They concern key delivery, not manual
  dedupe; the existing bootstrap path works; the payoff is one tool call per
  session. A separate ticket takes them if a real defect surfaces.
  Rejected: a first-turn `prompt.submit` key-line fallback; a key-interaction
  investigation recorded in this ticket's Result.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the mod ships to
  downstream projects; the new lead line is model-facing shipped text) and
  `ai-docs/manuals/wsflow-mirroring.md`.
- The safety invariant from the injection ticket stands: the manual is never
  in neither place. Every uncertain case resolves to the full result.
- Subagent calls, other servers' tools, refusals, and error results keep
  passing through untouched (`rewriteRead` in `register.ts`).

## Phases

### Phase 1: Conversation-visible static-body cut

Extend `stateOnly` in `manual.ts` per D1 and D2: when the snapshot does not
hold, cut the fresh response and look in the main conversation for an earlier
non-error `workflow_manual` tool result whose own cut span equals the fresh
span; on a match return the state-only text under the conversation-sourced
lead line, otherwise the full result. Mirror to wsflow.

Verification:

- Pure and engine tests (`manual.test.ts`, `manual-engine.test.ts`) cover:
  first call full; a later call with changed state is cut with the new lead
  line while the snapshot is false (the sec-default case); an earlier full
  result removed from the messages, or its content replaced as tool-result
  clearing would, yields full; a different `session_key` yields full; a
  subagent call passes through; an identical state-only repeat still becomes
  a pointer; the snapshot path's output is unchanged.
- `claude plugin validate` and `claude plugin test` on both packages, and the
  wsflow mirror drift guard.


## Resolution (2026-10-31)

Superseded by 261031-feat-claude-mod-manual-via-preload-once, which removes the system-prompt manual section and the state-only cut this ticket extended.
