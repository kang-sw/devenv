# agy bypass + hosted model list: independent-review fixes

## Goal

Fix the findings of the independent review of Phase 1 of
`ai-docs/tickets/ready/261118-feat-ws-hosted-model-list-and-agy-bypass.md`
(commits `795074e21`, `cc5a54558`, `9bfab79ec`) on `track/agent-v2` in worktree
`D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937`. The review
found no blocker; the items below are what it flagged. Line numbers are the
reviewer's and may drift; verify each against source before changing it.
Paths are under `agents-plugin-tool/internal/wsagent/` unless stated.

## Findings to fix

1. **Bypass must not kill the lead on an unanswerable request.**
   `answerFor` (agy.go ~629-647) errors when the request has no
   allow_once/allow_always option or an id the mapper no longer holds. The
   bypass fast path (runner_events.go ~283-291) then reaches `sendApproval`
   (~437-443), which calls `stopProcess`. Before bypass, the user saw the
   prompt and could deny it. Fix: in the bypass fast path, when the answer
   cannot be built (nothing written yet), fall back to publishing the prompt
   as in non-bypass mode. Stop the process only on a real write failure.

2. **A stale bypass flag must not widen approval after a downgrade/upgrade.**
   agy_bypass.go ~7-9, 33-38. Scenario: bypass is set on this build, an older
   0.50.x daemon preserves `agy_bypass` as an unknown field, the user picks
   `plan`/`auto` there (PermissionMode changes, flag untouched), then this
   build reopens and auto-allows everything. Fix (reviewer's suggestion):
   `agyBypass()` also requires the record's PermissionMode to be the value this
   build stores together with bypass (`default`), so a mismatch fails safe to
   prompts. FIRST verify that normal operation on this build never moves
   PermissionMode away from that value while bypass is on (for example a native
   mode report from agy overwriting it); if it does, pick the simplest
   alternative that keeps normal bypass stable and still fails safe on the
   scenario, and say which in the report. Cite source lines for the tier
   verdict (which version writes/reads what) in `## AI Context`.

3. **No per-tool-call record rewrites on the bypass lead hot path.**
   Under bypass each tool call costs `sendApproval` -> `c.update` and
   `forgetAnswered` on ApprovalDone (runner_events.go ~117, ~366-377), each a
   full `writeRecord` (record.go ~322-347: MarshalIndent + temp + rename).
   That violates `ai-docs/manuals/performance.md` (owner release gate: file
   touches on Windows AV PCs). Fix: keep bypass-answered ids in memory and
   persist only what restart recovery actually needs. First establish what
   replay dedupe needs across a daemon restart (does the agy process survive
   a restart under a holder, and would a lost answered id cause a double
   answer?). Pick the smallest change that removes both per-call rewrites for
   bypass; if it is not small or would change the stored shape, do only the
   cheap part (for example skip the write when ApprovalDone immediately
   follows the answer) and report the rest instead of building it.

4. **Minors (cheap, do them):**
   - `hosted_choices.go` ~107-116, 131-135: an `agy models` run that exits 0
     but parses zero models (e.g. logged out) is cached as a 5-minute
     success. Return an error so the 30 s fail TTL and a visible message
     apply.
   - `pi.go` ~40-44 `runListing` (shared with agy): set `cmd.WaitDelay` so a
     grandchild holding stdout cannot block past the 15 s timeout while
     Support holds `choicesMu` (Windows launcher shims).
   - permission.go ~266 + agy_bypass.go ~13-20: after the record already
     switched to bypass, `allowAgyBypassPending` can return 409 during replay
     or a write error, so the settings call reports failure though bypass is
     on. Skip answering while `c.replaying` (replayDone handles it) and log
     write errors instead of returning them.
   - `hosted_choices.go` ~46-60 Codex parse: if the Codex models cache has a
     `visibility` field, skip entries whose value hides them. Read only that
     field. If no such field exists, leave the code alone and say so.

Out of scope: the handoff carry-over of AgyBypass (doc-only, lead handles),
any refactor of the permission path beyond these fixes.

## Constraints

- Read before editing: `ai-docs/manuals/performance.md`,
  `release-compatibility.md`, `shipped-surface-boundary.md` (AGENTS.md
  `### Implementation Conventions`), and `ws-mcp.md` if `internal/mcp/` is
  touched.
- Keep `cc5a54558`'s rule: entering bypass needs user authority.
- Compatibility: no new stored field, hello, or tool argument expected (T0
  for 1, 3, 4; 2 tightens behavior on an existing field). If a fix needs a
  stored-shape change, stop and report.
- No local `go test` and no ws executables on this PC (antivirus). Build and
  test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run agy-fixes <min> --sync 'D:/Workspace/Repos/.ws-dashboard-worktrees/devenv/261118-0937' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet <touched packages> && go test <touched packages>'`.
  Temp files only under `D:/Temp/agy-fixes/`, deleted after.
- Surgical. Simplest complete fix per finding; no new abstraction.

## Verification

Unit tests pin:
- bypass + unanswerable request -> prompt published, process not stopped;
- bypass flag with a mismatched PermissionMode -> prompts (not auto-allow);
- bypass answer path does not write the record per call (or the reduced
  count you chose, asserted);
- empty `agy models` output -> error;
- enabling bypass during replay does not report failure.
w5090 `go build ./...`, `go vet` and `go test` of touched packages pass.

## Done when

- One commit on `track/agent-v2` with the fixes, tests, and this plan file.
  Message carries `## AI Context` (per-finding root cause and choice, tier
  verdict for 2 with source lines) and the ticket stem
  `261118-feat-ws-hosted-model-list-and-agy-bypass` under
  `## Ticket Updates`, ending with your own model's Co-Authored-By trailer.
  Do not edit the ticket body.
- Report: commit hash, files, per-finding outcome (fixed / reduced /
  not-applicable with reason), w5090 results, any surprise.
