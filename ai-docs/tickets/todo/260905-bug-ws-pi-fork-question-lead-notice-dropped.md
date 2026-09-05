---
title: A fork-raised owner question never reaches the lead as the promised thread notice
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: specified the lead-side notice
  260905-feat-ws-pi-push-only-child-reports: the push-model rewrite that dropped it
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
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
- **Family.** Reuse `ws-agent-advisory` with the notice as `detail`, since the
  message is the adapter's own statement about the child rather than the
  child's text. Rejected: a new push family for one message; rejected:
  `ws-agent-question`, whose renderer and `steer` delivery mean "answer this".
- **Headless unchanged.** `undefined` from the hook still yields the
  `ws-agent-question`/`steer` baseline.

## Spec Impact

`pi-adapter-runtime`: the owner-question surface anchor already promises the
notice; add the delivery form (advisory family, `followUp`) to the "Attach to
a live task fork" bullet and to the report-channel anchor's family table.

## Phases

### Phase 1: Deliver the notice

In `spawner.ts`'s `applyRpcEvent`, return
`{ push: { family: "ws-agent-advisory", payload: { detail: notice }, deliverAs: "followUp" } }`
when the hook returns a string. Tests: a stubbed `onQuestionReport` returning
text yields exactly that push and no `ws-agent-question`; returning
`undefined` yields the headless baseline; a throwing hook still degrades to
the baseline. Amend the two spec anchors. Live check (owner-run): repeat
acceptance scenario E and confirm the lead sees the notice before the owner
opens `/answer`.
