# agy bypass: approve without flashing the approval modal

## Goal

Fix a dogfood surprise in Phase 1 of
`ai-docs/tickets/ready/261118-feat-ws-hosted-model-list-and-agy-bypass.md`
(implemented in `795074e21`, `cc5a54558`, merged `00801f932`) on
`track/agent-v2` in worktree
`D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937`.

Owner live check, 2026-11-19: bypass works. But a hosted agy lead in bypass
mode still shows the approval modal on the dashboard for each permission
request, then approves it, so the modal flashes on every tool call. In
bypass mode a permission request should be answered allow without ever
being surfaced to the page: no modal, no pending-approval row, no wake of
the page for it.

## Settled

- Behavior: under bypass, ws answers the agy permission request allow at the
  point where it would otherwise publish the pending request. The page never
  sees it. Non-bypass modes are unchanged.
- Keep `cc5a54558`'s rule: entering bypass needs user authority. This change
  is only about what happens once bypass is on.
- If the transcript or audit trail recorded the auto-approval before, keep a
  record (for example the existing log or entry line), just not the
  interactive modal state.
- Compatibility: no stored shape, hello, or tool argument change expected
  (T0). If the fix needs one, stop and report.

## Constraints

- Read before editing the manuals for the touched paths (AGENTS.md
  `### Implementation Conventions`: `ai-docs/manuals/performance.md`,
  `release-compatibility.md`, `ws-mcp.md` for `internal/mcp/`,
  `shipped-surface-boundary.md`).
- No local `go test` and no ws executables on this PC (antivirus). Build and
  test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run agy-bypass <min> --sync 'D:/Workspace/Repos/.ws-dashboard-worktrees/devenv/261118-0937' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet <touched packages> && go test <touched packages>'`.
  w5090 has no node; if page JS changes, report which JS tests would cover
  it instead of running them. Temp files under `D:/Temp/agy-bypass/`,
  deleted after.
- Surgical: no refactor of the permission path beyond this fix.
- Housekeeping: delete the stray duplicate
  `ai-docs/.plans/2026-11/19-0935-agy-bypass-no-modal-flash.md` (untracked,
  the lead's misplaced copy of this plan); do not commit it.

## Verification

- A unit test pins the behavior: in bypass, a permission request is allowed
  and no pending approval is published or broadcast. In a non-bypass mode it
  is still published.
- w5090 `go build ./...`, `go vet`, and `go test` of the touched packages
  pass.

## Done when

- One commit on `track/agent-v2` with the fix, the test, and this plan file.
  The commit message carries `## AI Context` and the ticket stem
  `261118-feat-ws-hosted-model-list-and-agy-bypass` under
  `## Ticket Updates`, ending with your own model's Co-Authored-By trailer.
  Do not edit the ticket body; the lead records the Phase 1 result after
  review.
- Report: commit hash, files, the root cause in one or two lines,
  w5090 results, and any surprise.
