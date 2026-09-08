---
title: "tickets.sage_gate offers no rerun path for a stale completed review"
completed: 2026-09-09
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - 260720-sage-gate-record-tools
related-mental-model:
  - mcp-runtime
related:
  260908-feat-survey-plan-is-route-not-contract: the ticket whose post-stamp edit exposed the missing path
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8789f1a7b36e89cb
sage-review-completeness-reviewed: 8789f1a7b36e89cb
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

## Decisions

- **`answer` resolves the freshness question the same way it resolves a
  `recommended` ask.** When the gate would return `check_review_required`,
  `answer: yes` returns `run` with `reviewers` set to the listed stale
  stages and `mode` chosen by the existing category x stage matrix, and
  `answer: no` falls through to ordinary posture resolution for the
  remaining stages. The freshness result already lists every stale stage
  as one question (`freshness_stages`), so one answer covers the listed
  stages. `mode` is `combined` when two stages are listed and
  `standalone` when one is, matching the literal each existing
  construction site passes, and the `run` result carries the non-waivable
  `advisory` like every other `run`. Ordering matters: in `SageGate` the
  freshness check runs before `resolveStage` and returns early, so a
  declined freshness question must not end the call: with design
  `completed` but stale and completeness still `required` (a ticket
  design-reviewed at `todo/`, edited, then promoted to `ready/`),
  `answer: no` skips the stale design stage and the completeness stage is
  then resolved as if the freshness check had passed, so a pending
  `required` or `recommended` stage is never swallowed by the decline.
  - Rejected: rerunning silently whenever the digest is stale. Whether an
    edit needs a fresh review is the lead's call; the gate asks, the lead
    answers.
  - Rejected: a separate `rerun` parameter. The gate has one answer slot
    per question and `check_review_required` is a question; a second slot
    is schema growth for the same reply.
- **Declining leaves the record as it is.** `answer: no` writes nothing
  for the stale stage:
  the posture stays `completed`, the `-reviewed` digest stays stale, and
  `tickets.verify` keeps warning until a fresh `tickets.sage_stamp` lands.
  This differs from declining a `recommended` ask, which persists
  `skipped`, because a stale completed review is a fact about the body,
  not a posture choice.
  - Rejected: documenting the frontmatter posture reset as the rerun path.
    It leaves the `-reviewed` digest pointing at a body that no longer
    exists until the next stamp overwrites it, and bypasses the tool that
    owns the field.

## Constraints

- Go touch points: `sageGateFreshnessResult` and its four call sites in
  `SageGate` (`agents-plugin-tool/internal/wsdoc/tickets_sage.go`) gain
  the `answer` argument, with `no` continuing past the early return;
  `resolveStage` is unchanged for `recommended`; the `run` result for the
  freshness path is built through `stageOutcome` so it carries the
  advisory, with `mode` per Decision 1; `sageGateNextInstruction` in
  `agents-plugin-tool/internal/mcp/server.go` renders, for
  `check_review_required`, the same "call tickets.sage_gate again with
  the same stem/landing plus answer=yes|no" sentence the `ask` case
  renders, since today it says only "decide whether to rerun" and names
  no way to act. Tests extend `TestSageGateWarnsWhenCompletedReviewIsStale`
  (or sit beside it) with the `yes` case (asserting `run`, the stage
  list, `mode`, and the advisory), the `no` case with a pending
  `required` completeness stage (asserting the completeness `run`
  result and byte-identical frontmatter), and the next-instruction text.
- The `ask` contract is unchanged: one answer never resolves a
  `recommended` stage and a freshness question in the same call.
- Playbooks are unchanged: `lead-write-ticket` follows the gate's
  returned `next_instruction`, which is where the re-invoke sentence is
  added.
- Spec addressing: `ai-docs/spec/mcp-tools.md`
  `{#260720-sage-gate-record-tools}`, the freshness paragraph ("differing
  returns `check_review_required` ...") gains one sentence for each answer,
  and the advisory sentence ("It rides every `run` result ...") lists the
  freshness `run` among them; the `ask` sentence ("the caller re-invokes
  with `answer`") is unchanged.
- Out of scope: the digest computation, the legacy Git-walk baseline, and
  `tickets.verify`'s warning text.

## Phases

### Phase 1: Let the freshness verdict be answered

Thread `answer` into the freshness result per Decisions 1 and 2, extend
the `check_review_required` next-instruction, add the three test cases,
and add the spec sentences. Verification:
`go test ./internal/wsdoc/ -count=1` in `agents-plugin-tool/`; one
dogfood run on a `ready/` ticket edited after its stamp, showing
`tickets.sage_gate(answer: yes)` returning `run` for exactly the stale
stages and `tickets.sage_gate(answer: no)` returning `skip` with
`tickets.verify` still warning.

### Result (6350046c) - 2026-09-09

Threaded `answer` through `sageGateFreshnessResult`, which now returns
`(SageGateResult, bool, error)` — the `consumed` bool is true whenever a
freshness question was posed. When the completed stage(s) are stale:
`answer: yes` returns a non-waivable `run` over exactly the stale stages
(`mode: combined` for two, `standalone` for one, built through
`stageOutcome` so it carries `sageReviewNonWaivableAdvisory`); `answer: no`
writes nothing (posture and `-reviewed` digest stay as they are, so
`tickets.verify` keeps warning) and the caller resets `answer` to `""` before
resolving any remaining pending stage, so a still-pending `required` or
`recommended` stage is never swallowed by the decline; unanswered still
returns `check_review_required`. `gateResultFromStage` was generalized to
`gateResultFromStageReviewers` (multi-reviewer) for the two-element stale-stage
list; `resolveStage` is unchanged. `sageGateNextInstruction`'s
`check_review_required` branch gained the `answer=yes|no` re-invoke sentence
while preserving the pinned `decide whether to rerun the listed sage review
stage(s)` substring. Spec `mcp-tools.md` `{#260720-sage-gate-record-tools}`
documents both answers and lists the freshness rerun among advisory-bearing
runs.

Commits: `6350046c` (feat: implementation + spec), `fa15b5ce` (test: pins the
`answer=""` reset via a `recommended`-downstream discriminating case, plus a
single-stage `standalone` yes-case). Range `6350046c..fa15b5ce`.

Verification: `go test ./internal/wsdoc/ -count=1` and
`go test ./internal/mcp/ -count=1` in `agents-plugin-tool/` both pass; the
`internal/mcp` run is required because `TestFormatSageGateRoundTrip` pins the
edited next-instruction text. Partitioned review: correctness clean; test
partition raised one Important (the decline-reset regression was not exercised)
and one Minor (standalone yes-mode uncovered), both fixed in `fa15b5ce`.

The Phase's dogfood step (calling `tickets.sage_gate(answer: ...)` on a stale
`ready/` ticket through the live tool) is deferred: the installed MCP tool
serves the published plugin version, which does not yet carry this change, so a
live call would exercise the old behavior. The unit tests exercise the new
`SageGate` path directly and are the authoritative verification until the
change is published; the dogfood belongs to the post-publish smoke check.
