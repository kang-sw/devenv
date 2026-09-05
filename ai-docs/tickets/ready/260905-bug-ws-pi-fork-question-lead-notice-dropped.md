---
title: A fork-raised owner question never reaches the lead as the promised thread notice
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: specified the lead-side notice
  260905-feat-ws-pi-push-only-child-reports: the push-model rewrite that dropped it
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 062509868d6e73d0
sage-review-completeness-reviewed: 062509868d6e73d0
---

# A fork-raised owner question never reaches the lead as the promised thread notice

## Background

Acceptance run 2026-09-05, scenario E2 (the only outright failure): a
`ws-fork` task fork called `ws-report-to-lead(kind:"question")` from a TUI
lead, the owner answered it through `/answer`, and the fork finished with a
`Decisions:` final, but the lead never received the notice that the spec and
the lead guide both promise ("a notice naming the thread id; do not relay,
do not answer, end your turn").

Cause, from source: `index.ts`'s `onForkQuestion` callback registers the
thread and returns `buildForkQuestionLeadNotice(agentId, threadId)` in TUI
mode. `applyRpcEvent` in `spawner.ts` uses that return value only as the
"consumed" flag and returns an empty outcome, so the notice text is built
and discarded. Under the earlier wait-return model the callback's return was
handed back to the child's tool call and reached the lead; the push-model
rewrite kept the flag semantics and lost the delivery. No unit test asserts
that the notice is pushed; `test/ask.test.ts` only pins the notice's wording.

This is a reversal of a stated decision, not a purely additive fix: the
push-only ticket specified "in TUI the question is consumed by the
`onQuestionReport` hook and not pushed", the comment block above the
question branch in `applyRpcEvent` documents that suppression as the 260904
review-relay contract, and three landed tests assert it
(`test/spawner.test.ts` at the "question consumed" cases, and
`test/fork.test.ts`'s TUI question case). The tension resolves because the
notice is not the question: the lead gets a non-actionable "a thread was
opened" statement, so the side-thread rule that the lead is not the
answering channel still holds.

Consequence: the lead sees nothing when a fork hands a question to the
owner. The fork also leaves the fan-in count (thread-bound), so the lead's
next status line silently drops one running agent with no explanation, and
a lead that was waiting on that fork has no cue to end its turn.

## Decisions

- **Push the notice, keep the flag.** When the question hook returns a
  string, `applyRpcEvent` returns a push carrying that text instead of an
  empty outcome. Delivery is `followUp` (the lead must not be interrupted
  mid-turn for something it cannot act on), sender labeled as the fork in
  the usual `<alias> (<id>)` form.
- **Family.** Reuse `ws-agent-advisory` with the notice as `detail` and the
  discriminator `advisory: "fork-question-thread"`, matching every existing
  advisory push (`final-report-shape`, `expects-commit`, `stalled`, ...) so
  the family table's "`details.advisory` names which" stays true. The
  message is the adapter's own statement about the child rather than the
  child's text. Rejected: a new push family for one message; rejected:
  `ws-agent-question`, whose renderer and `steer` delivery mean "answer this".
- **The one advisory a thread-bound record pushes.** The record becomes
  thread-bound inside the hook, before the notice is returned, so this push
  is emitted for a record that is already thread-bound. The spec's
  "thread-bound records push no settle or advisory" rule is amended with
  this single carve-out: the registration notice is the one advisory a
  thread-bound record pushes; all later settles and advisories for it stay
  suppressed (the settle-branch gate in `attachEventListener` and the
  anti-bleed loop in `fork.ts` are untouched).
- **Headless unchanged.** `undefined` from the hook still yields the
  `ws-agent-question`/`steer` baseline.

## Spec Impact

`pi-adapter-runtime`: the owner-question surface anchor already promises the
notice; add the delivery form (advisory family, `followUp`) to the "Attach to
a live task fork" bullet and to the report-channel anchor's family table, and
amend the two "thread-bound pushes no settle or advisory" sentences (report-
channel anchor and side-thread anchor) with the registration-notice
carve-out.

## Phases

### Phase 1: Deliver the notice

In `spawner.ts`'s `applyRpcEvent`, return
`{ push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }`
when the hook returns a string, and rewrite the comment block above the
question branch so it describes the notice push instead of the suppression.
Invert the three landed tests that assert the suppression. Tests: a stubbed
`onQuestionReport` returning text yields exactly that push and no
`ws-agent-question`; returning `undefined` yields the headless baseline; a
throwing hook still degrades to the baseline. Amend the spec passages listed
above. Live check (owner-run): repeat acceptance scenario E and confirm the
lead sees the notice before the owner opens `/answer`.

### Result (702d3c40) - 2026-09-06

Landed as `2fbd404f` (survey plan), `702d3c40` (fix and tests), `834fa99a`
(spec), `e104ebf7` (review relay #1, comment and spec sync) on the
implementation branch under the goal branch. Adapter-only change.

- `applyRpcEvent`'s question branch now returns the push
  `ws-agent-advisory` / `advisory: "fork-question-thread"` / `detail:
  <notice>` / `deliverAs: "followUp"` when the hook returns a string;
  `undefined` and a throwing hook keep the headless
  `ws-agent-question`/`steer` baseline. The push goes through the same
  generic `pushToLead` path as every other family, so the `<alias> (<id>)`
  sender label and the fan-in line come for free, and no downstream
  thread-bound gate drops it (the settle gate guards only the idle-settle
  push). Settle gate and anti-bleed loop untouched.
- Tests: the three ticket cases added; five landed suppression tests
  inverted, not deleted (the plan named three; two more of the identical
  pattern surfaced in `fork.test.ts` and `agent-sidecar.test.ts`). Adapter
  suite 735 pass, 0 fail.
- Spec: family table row, the two thread-bound carve-out sentences, and
  the "Attach to a live task fork" bullet amended; anchor ids unchanged.
  The delivery wording was corrected in relay: `followUp` guarantees the
  notice is queued for the lead's next turn boundary, not that the lead
  sees it before the owner opens `/answer`, so the live check above should
  read "the lead sees the notice at its next turn boundary".

Review (partitioned correctness/test): test clean; correctness one
Important (`PUSH_FAMILIES` in-code family table still described the TUI
suppression) fixed in relay #1 together with three Minor (two stale hook
comments, one overstated spec ordering claim).

Owner-run live check outstanding: acceptance scenario E against the
merged goal branch.

## Blocked (2026-09-06) — owner sign-off pending, not a work item

Phase 1 carries a Result; no autonomous work remains. Closing waits on the
owner-run acceptance scenario E re-run. Once confirmed, close the ticket to
`.done/`.
