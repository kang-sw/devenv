---
title: "tickets.sage_gate offers no rerun path for a stale completed review"
---

# tickets.sage_gate offers no rerun path for a stale completed review

## Background

Observed 2026-09-08 on `260908-feat-survey-plan-is-route-not-contract`
after a new Decision was added to a `ready/` ticket whose design and
completeness stages were both `completed`. `tickets.verify` warned that the
review was stale and `tickets.sage_gate` returned
`check_review_required` with "decide whether to rerun". Calling the gate
again with `answer: yes` returned the same `check_review_required`: the
`answer` parameter is only consulted for posture `recommended`
(`resolveStage` in `agents-plugin-tool/internal/wsdoc/tickets_sage.go`),
and posture `completed` always resolves to `skip` before freshness turns
it into `check_review_required`. The gate therefore hands the lead a
decision it cannot act on through the gate. The only way to obtain a `run`
result (reviewers plus mode) is to hand-edit the frontmatter posture back
to `required`, which is undocumented and bypasses the digest bookkeeping.

## Phases

### Phase 1: Let the freshness verdict be answered

Make `answer: yes` on a `check_review_required` result resolve to `run`
for the listed freshness stages, with the mode chosen by the existing
category x stage matrix, so the lead reruns exactly the stale stages.
Make `answer: no` resolve to `skip` without touching the recorded posture
or digest, so `tickets.verify` keeps warning until a fresh stamp lands.
Cover both in the gate tests next to the existing `recommended`
answer cases, and describe the two answers in the `tickets.sage_gate`
paragraph of `ai-docs/spec/mcp-tools.md`.

- Rejected: rerunning silently whenever the digest is stale. Whether an
  edit needs a fresh review is the lead's call; the gate should ask, not
  decide.
- Rejected: documenting the posture-reset workaround instead. It leaves
  the `*-reviewed` digest lines pointing at a body that no longer exists
  until the next stamp overwrites them.
