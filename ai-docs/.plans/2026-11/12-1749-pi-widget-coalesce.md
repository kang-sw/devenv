# Plan: hosted Pi subagent widget coalesces batch churn

Source: 0.49 pre-ship sweep `62304312b..32b702129` (stamped concern,
ea0b38f34), Important finding 2 in section "Hosted / Pi" of
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/05aaa35c-01-findings.md`.
Original contract: ticket
`ai-docs/tickets/.done/261112-bug-ws-hosted-pi-subagents-missing-from-dashboard.md`
(`## Decisions`), implemented in ce5f45a60.

## Problem

`agents-plugin-pi/src/hosted-agent-rows.ts:105-113` (+ `index.ts:959-969`)
marks the subagent widget dirty on every child batch change, streaming
deltas included (`generating.chars` grows), flushed every 100 ms, each line
the full agent event with up to 5 batch views (est. 1-3 KB). These lines are
the hosted lead's stdout, so they fill the lead holder ring (~10-30 KB/s per
streaming child): this makes ring wrap routine, and an idle Pi lead with a
background child can end up with a widget-only ring after a long daemon
outage (wrapHold with no idle signal, stuck until a stop).

## Outcome (user board (36) = a, revising the ticket's field-parity decision)

- Status, permission, label, and model changes still emit at once (the
  existing 100 ms throttle).
- A change that is only in a child's batches emits at most once per 2 s for
  the channel; growth of streaming character counts (`generating.chars` and
  any other pure streaming-progress counter in the batch views) does not
  count as a change at all.
- The 30 s full resend without batches and the done-for-a-vanished-child
  line stay as they are.
- The CLI (non-hosted) dashboard feed is unchanged; if shared diff code in
  `dashboard-feed.ts` needs a knob, the CLI path keeps its current behaviour
  and its tests stay unchanged.

## Settled constraints

- Rejected: dropping batches from change detection entirely (batch list
  would refresh only on status changes).
- Compatibility tier T0: no change to the widget key, the event shape, or
  the daemon mapper.
- Files: `agents-plugin-pi/src/hosted-agent-rows.ts`, and
  `dashboard-feed.ts` only if the shared diff needs it, plus tests.
- The worktree has no node_modules; run the Pi tests with a temporary
  symlink to `/home/swkang/devenv/agents-plugin-pi/node_modules` and remove
  it before committing (never commit it).

## Verification

- `test/hosted-agent-rows.test.ts`: streaming-char growth alone emits
  nothing; a batch change emits once per 2 s window however many arrive; a
  status or permission change emits within the 100 ms throttle even inside
  a 2 s window; the 30 s resend and done-for-vanished still work.
- `dashboard-feed` and `hosted-session.integration` tests unchanged and
  passing.
- `git diff` reviewed against this plan before commit on
  `impl/develop/pi-widget-coalesce`, with `## Ticket Updates` naming
  `261112-bug-ws-hosted-pi-subagents-missing-from-dashboard` with a Forward
  note for the revised decision.
