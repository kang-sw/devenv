---
title: Pi push wake runs a blind model response before the held reports are delivered
related:
  260906-bug-ws-pi-goal-reminder-races-child-push-at-settle: Phase 2 introduced the user-preflight wake this ticket corrects
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d4c80bb87ff94838
sage-review-completeness-reviewed: d4c80bb87ff94838
---

# Pi Push Wake Runs a Blind Model Response Before the Held Reports Are Delivered

## Background

The user-preflight wake introduced by `438f2f0b` (the settle-race ticket's
Phase 2) sends `N ws messages waiting; process the incoming reports.` as a
user message so Pi runs `before_agent_start` and composes the ws
system-prompt block, then flushes the held pushes at confirmed `agent_start`
with each push's recorded delivery mode.

Source-level verification, 2026-09-06, against the installed Pi (0.85.1,
`pi-agent-core` `agent-loop.js`; the worktree's 0.84.4 copy is structurally
identical):

- `agent_start` is emitted and its extension handlers awaited before the
  loop's first steering drain, so the flush's enqueues are in place before
  the first model call.
- The steering queue is drained before the first model response and again
  after every turn; the follow-up queue is drained only after the inner
  loop has exhausted tool calls and steering, that is, after the run would
  otherwise stop. The default drain mode is one message at a time.
- Every report-shaped push is recorded as `followUp` (`ws-agent-report`,
  `ws-agent-settled`, `ws-agent-advisory`, `ws-agent-orphaned`,
  `ws-thread-summary`); only asks and approvals are `steer`.

Consequence per wake batch of N held `followUp` pushes: the model answers
the bare wake line first with no report in context (a blind response that
may include tool calls, each a further model call), then one follow-up is
drained per outer iteration, one response each. At least N+1 responses,
minimum two for a single held report, where one would do. Held `steer`
pushes do not pay this: they ride in the wake's first request. A second
defect follows from the same split: the spec's "releases in FIFO order"
holds for the send calls only; a `steer` admitted after a `followUp`
reaches the model first.

The wake itself must stay a user prompt. A run started by a custom message
never sets Pi's system-prompt override, so from the second model iteration
the ws block is dropped (`agent-session.js` rebuilds the prompt from the
override or the base). Removing the wake would reintroduce that defect.

The existing fake harness in `agents-plugin-pi/test/push-wake.test.ts`
records `sendMessage` calls and asserts idleness only; it models neither
queue nor dequeue timing, which is why two reviews passed the gap.

Usage share is unmeasured; this is path evidence of at least one wasted
full-context response per wake batch. On the owner's 2026-09-06 goal run the
lead session produced 238 responses in about two hours with most of its
tokens as cache reads, so every avoided blind turn is a full-context turn
saved.

## Decisions

- **Flush held pushes as `steer` at confirmed start.** The confirmed-start
  branch of the flush passes `steer` for every held push instead of the
  recorded mode, so the batch is in the steering queue before the woken
  run's first model call. The recorded mode is unchanged and still decides
  busy-time admission (busy `followUp` holds until settle, busy `steer`
  interrupts). The wake stays a user prompt, so system-prompt continuity is
  preserved. Rendering (`push-render.ts`), `customType`/`details`, and the
  flush-time status rebuild are untouched.
- **Any confirmed start flushes as steering, not only the wake's own run.**
  The adapter cannot tell the wake's run from a run the user started while
  the reservation was pending, so in that case held reports recorded as
  `followUp` are steered between the turns of the user's run instead of
  after it stops. This is accepted as the default: it serves fan-in, there
  is no reliable discriminator, and the alternative is the blind response.
  The spec paragraph names it.
- **Ordering.** With the default one-at-a-time drain the first held push
  merges into the wake's first request and each later one is drained at
  the post-turn steering poll, still before any follow-up. Each response
  therefore has at least one report in context; none is blind. The batch
  is now FIFO at delivery, not only at send.
- **Rejected: embed the report bodies in the wake line.** It is the only
  option that collapses a batch into one response, but it moves payloads
  out of custom messages (loses `details` and the per-family TUI renderer),
  needs status recomputed at wake time, and contradicts the spec's
  "carrying the queued count, not the payloads". Reconsider only if a later
  measurement shows large batches dominate.
- **Rejected: an inert wake.** The current wake is already a bare prompt;
  it cannot be made cheaper than one response, and it does not fix ordering.
- **Rejected: setting Pi's `steeringMode` to `all`.** It is a global user
  setting; the adapter does not write user settings.
- **Measurement folds into the live check.** The owner's 2026-09-06
  direction is to write the fix now; the provider-side count is verified on
  the owner-run live check below rather than gating the source change.

## Spec Impact

`pi-adapter-runtime`, the "Idle pushes wake through user preflight"
paragraph under `{#260904-pi-report-to-lead-channel}`: the queue releases at
confirmed `agent_start` as steering messages in arrival order. With Pi's
default one-at-a-time drain, the first held message precedes the first
response and later messages arrive at subsequent steering polls; the whole
batch need not precede the first response. Each message's recorded
`followUp`/`steer` mode continues to govern busy-time admission only; the
same release applies when the confirmed start belongs to a run the user
started while the wake reservation was pending.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no Pi settings written.
- Busy-time admission, compaction holds, the shared wake-start reservation,
  and goal-reminder streak rules are unchanged; the one delivery-timing
  change is the confirmed-start flush described under Decisions.
- The flush keeps `triggerTurn: true`; a flush composed without it while
  the reservation is set becomes a deferred custom append in Pi, not a
  steer, so the existing assertion on that flag stays.
- The owner live check below shares a session with the predecessor
  ticket's outstanding provider-context gate (a push-woken run's second
  model call still carries the ws block); run both in the same session,
  this ticket's check first.
- The wake line text and its count are unchanged.

## Phases

### Phase 1: Release the held batch as steering at confirmed start

Change the confirmed-start flush to send every held push with `steer`
(give the raw-send closure a mode override for that one call site) while
leaving the recorded mode on the held entry; amend the spec paragraph under
Spec Impact. Tests (`push-wake.test.ts`): a held `followUp` report flushed at
confirmed start is sent with `deliverAs: "steer"` and `triggerTurn: true`;
a held `steer` ask is still `steer`; busy-time admission of a `followUp`
push still holds and a `steer` push still interrupts; arrival order is
preserved across mixed modes; update the existing mode-echo assertions
that currently expect the recorded mode at flush. Add a small fake of
Pi's drain order (steering drained before the first response and after
each turn, follow-up only after the inner loop stops, one message per
drain) and assert that the first held message precedes the first model
response, later held messages arrive at subsequent steering polls, and
FIFO delivery is preserved. The offline contract is removal of the blind
initial response, not simultaneous delivery of the whole batch. Live check (owner-run): with one child reporting to
an idle lead, confirm the woken run's first model request already contains
the `ws-agent-report` message and that the run produces one response, not a
bare acknowledgement followed by a second response; with two children
settling together, confirm both reports arrive before any follow-up and the
run does not exceed one response per report in a controlled no-tool-call
check. Multiple reports need not all precede the first response.

### Result (9b992772) - 2026-09-06

Implemented confirmed-start steering release for family pushes and raw
thread summaries; recorded delivery modes still govern busy-time admission.
Relay commit `f7226a7` corrected the drain fake and added mixed raw/family
FIFO coverage in both orders plus an independent user-start reservation
race regression.

The owner approved narrowing the contradictory whole-batch-before-first-
response acceptance on 2026-09-06: remove the blind initial response while
preserving Pi's default one-at-a-time processing. Batch coalescing is not
required; neither global steering settings nor payload embedding is added.
This resolves review finding 1's scope escalation. Findings 2 and 3 are
[fixed]; correctness and fit reviews were clean. The three Important test
findings received one relay; no Critical findings or re-review were required.

Verification: focused tests 509 passed; full adapter suite 935 passed;
`npm pack --dry-run` and `git diff --check` passed. Test commands unset
`WS_PI_SPAWN_ROLE` because the delegated worker role suppresses lead pushes;
failures with that role inherited were test-environment failures, not a
production regression.

Owner-live provider request/response-count checks and the shared predecessor
provider-context/settle gates remain pending. Offline tests do not establish
live acceptance or measured token savings. Keep this ticket in `ready/`
until those checks are completed; no merge or closure is implied.
