---
related:
  261106-epic-ws-dashboard-serve-daemon: "where the loop was observed (design stamp)"
  261106-feat-whiteboard-rich-items: "where the loop was observed (combined stamp)"
title: sage_stamp tells the lead to re-stamp with the same verdicts after a block that carries missing issues, which re-records the same block
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 537812b2c7cf2430
sage-review-completeness-reviewed: 537812b2c7cf2430
---

# sage_stamp re-stamp instruction loops on a missing-issue block

## Background

Dogfood surprise (2026-11-06, devenv lead session): after a design or
completeness block carrying both `autonomous` and `missing` issues,
`ws/tickets.sage_stamp` returns a `next_instruction` that says to fix the
autonomous issues, settle the missing ones through the Open Decision Queue,
then "call ws/tickets.sage_stamp again with the same verdicts" before
committing.

Doing exactly that (epic 261106-epic-ws-dashboard-serve-daemon, design;
261106-feat-whiteboard-rich-items, combined) re-records the reviewer
`block` verdict carried in the same verdicts (the block comes from the
reviewer's own verdict field, not from the `missing` count:
`agents-plugin-tool/internal/wsdoc/tickets_sage.go#L511-L515`,
`#L557-L559`): the result is `verdict: block` again, the
posture stays `blocked`, the Blocked section is re-rendered, and the
`next_instruction` repeats word for word. Only fresh reviewer verdicts can
clear the block, which the same message states only in its last sentence.
A lead following the instruction loops, or commits a ticket it believes was
cleared.

Code: `sageRecordIssueRouting` and `sageRestampAfterFixes` in
`agents-plugin-tool/internal/mcp/server.go` (near L3750-3774). The mixed case
(`Autonomous > 0 && Missing > 0`) appends `sageRestampAfterFixes`; the
missing-only case already omits it. `tickets_sage_test.go` (near L314-318)
pins the re-stamp phrase.

## Decisions

1. **A stamp whose recorded verdict is `block` stops prescribing a
   same-verdict re-stamp, whatever mix of issues it carries.** Its routing
   clause says instead: fix the autonomous issues, settle the missing ones
   through the Open Decision Queue, then rerun that stage's reviewer on the
   revised ticket (a delta review) and stamp its fresh verdicts, because a
   re-stamp with the same verdicts records the same block. The branch keys
   on the recorded verdict (`result.Verdict == "block"` / blocked posture),
   not on `Missing > 0`: the block comes from the reviewer verdict field
   (`tickets_sage.go#L515`, `#L558`), so an autonomous-only block loops the
   same way, and a mixed `concern` or `pass` stamp is not blocked. A
   non-block stamp keeps `sageRestampAfterFixes` in both the autonomous-only
   and the mixed branch, as 0d132ea4 decided: there the same-verdict
   re-stamp is what refreshes the digest over the fixed body. Rejected: a
   new "missing issues settled" input on `sage_stamp` (new API surface, and
   a stamp that clears a block without a reviewer reading the settled
   text); keying on `Missing > 0` (misroutes an autonomous-only block and
   reintroduces 0d132ea4's stale digest for a mixed concern).

## Constraints

- `next_instruction` text is shipped text: read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/`.
- Check shipped playbooks and skills (`agents-plugin*/rsrc`,
  `agents-plugin*/skills`) for any text that tells a lead to re-stamp with
  the same verdicts after a missing block, and align it in the same change.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest / 0d132ea4 (2026-09-10, commit): "The clause is shared by the autonomous-only and autonomous+missing branches; the missing-only branch is unchanged because taking a decision through the Open Decision Queue is not an edit this stamp's digest can pre-cover." — bearing: constrains (kept for non-block stamps per Decision 1)
- 0d132ea4 (2026-09-10, commit): "Ticket option (a): the tool owns both the digest and the instruction, so the contradiction is fixed where it is emitted rather than restated in a playbook. Rejected: skipping the digest when autonomous issues exist" — bearing: constrains
- 260910-bug-sage-stamp-autonomous-fix-instruction-stales-digest (2026-09-10, Result): "The lead-ticket promotion note said re-running the gate was the only repair for a post-stamp edit ... it names the re-stamp instead and keeps the gate re-run scoped to edits the reviewers have not seen." — bearing: constrains
- 71d0cbd8 (2026-10-23, commit): "removed lead-ticket's always-policy rule for reviewer missing issues, but this tool string, read by the lead at classification time, still preassigned them to the user." — bearing: constrains
- 260729-sage-stamp-resolution-routing / 2c36f422 (2026-07-29, commit): "The block-branch fix is the one that mattered: resolveStage returns stop_blocked for a blocked posture forever, so the recovery loop the instruction described could never run." — bearing: supports
- 260729-sage-stamp-resolution-routing / 5ce1f749 (2026-07-29, commit): "The routing text lives in sage_stamp's next_instruction, not in playbook prose ... post-call branch handling belongs in the tool's output." — bearing: constrains
- 1878e830 (2026-07-31, commit): "formatSageRecord's block branch already states the executable route (\"only a later ws/tickets.sage_stamp carrying fresh verdicts clears it\")" — bearing: supports
- 260723-feat-ticket-write-verify-commit-gate (2026-07-23, Decisions): "sage stamp is lead single-writer. ... reviewers already return verdicts only." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/tickets_sage_test.go, possibly agents-plugin*/rsrc/lead-ticket/lead-ticket.md L142-L144 |
| scope.surface | public-interface | shipped tickets.sage_stamp next_instruction text read by lead playbooks; no exported Go symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; SageRecordResult unchanged |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tickets_sage_test.go TestFormatSageRecordAutonomousOrdersRestamp L317, TestFormatSageRecordIssueRouting L269, TestFormatSageRecordBlockRecovery L203 |
| complexity.reuse_points | confirmed | sageRecordIssueRouting and sageRestampAfterFixes server.go L3750-L3775 read |
| complexity.side_effect_risk | low | instruction string change only; wsdoc verdict and digest logic untouched |
| risk.correctness | moderate | block comes from the reviewer verdict field, not the missing count, so keying the branch on Missing > 0 can misroute an autonomous-only block or a non-block mixed concern |
| risk.fit | moderate | reverses the shared-clause choice of 0d132ea4 and must stay aligned with lead-ticket.md L142-L144 which defers to the stamp result |
| risk.test | moderate | several prose-pin tests assert routing wording and re-stamp ordering in tickets_sage_test.go |
| risk.security_or_contract | moderate | shipped next_instruction is the contract leads follow before committing a sage posture |

## Phases

### Phase 1: fix the block-stamp re-stamp instruction

Implement Decision 1 and update the pinned tests, adding cases for a
`block` stamp with autonomous-only, mixed, and missing-only issues (each
prescribes rerunning the blocked stage's reviewer, or both reviewers after a
combined block, and stamping fresh verdicts, with no same-verdict re-stamp
and no commit or handoff while blocked) and keeping a mixed `concern`
stamp's same-verdict re-stamp. Update the block-branch comment in
`formatSageRecord` (2c36f422's "no recovery loop" note) to match. Align the
shipped `lead-ticket.md` line that defers to the stamp result (ws, wsflow,
and Pi copies, near L142-L144) only if its wording conflicts, running the
rsrc after-edit checklist in `ai-docs/manuals/wsflow-mirroring.md`. Verification: `go test ./internal/mcp/...`
under `agents-plugin-tool`.
