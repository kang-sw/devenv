# Fix wsagent daemon-restart applied-sequence flake

## Authority and bounded task

MAIN assigned EXEC3 at 2026-11-15 15:38:16Z, post-ship v0.50.1, NOT a ship blocker. Read /tmp/ci-flakes-round4-brief.md fully and execute only EXEC3. Read the named TestLeadRejoinsItsProcessAfterADaemonRestart failure in /tmp/ci-43729697102-failed.log. The failed Windows record has HolderSeq12 versus13 holder records, with calls [launch h1 spawn h1]. Investigate whether a test observation races with the applied-sequence persistence. Primary source is agents-plugin-tool/internal/wsagent/lead_runner_test.go around line798, plus its existing fixture helpers and only necessary runner/record source for diagnosis. Minimal portable test synchronization fix is authorized; any product behavior/protocol change is a STOP/report boundary. Do not work on EXEC2 or earlier tasks.

## Approach and proof

Read repository orientation, repo notes, git log --oneline --graph -50 and applicable manuals before edits. Establish exact develop base. Preserve original CI evidence, the observed ordering and source file:line cause, and a test-versus-product verdict. Find and cite introducing rationale/history if cheaply available; do not invent a cause merely because focused tests pass. Use deterministic fixture scheduling or existing synchronization where appropriate rather than sleeps, arbitrary delays or weakened assertions. Never delete/skip a test, only raise timeouts, or hide failures. A bounded diagnostic negative control is allowed if needed to prove the race; retain its exact diff/commands separately from the final patch. Stop before expanded scope.

## Verification and safety

Offline Go: GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local. Run only the named test with -count=20 on final code; also -race if cheap and supported. A focused before run is appropriate; no blind repeated stress loops. Actual Windows verification is authorized if useful/needed via ssh ki608@192.168.33.6, with a unique remote folder per run and cached dependencies only. Never signal PID2640 or kill by image name; any necessary termination must verify PID and CIM creation ticks. Leave C:\Users\ki608\devenv and %APPDATA%\Werkschaft untouched. Read existing Windows procedure first. No providers, external downloads/install/network beyond this authorized host transfer/test, credential access, unrelated signals/stashes/files, shared daemon/runtime operations, push or CI dispatch. No full wsagent/Go/dashboard/MCP suites. Never launch Electron on the Mac. Preserve full exact commands, env, outputs and failures, including platform/toolchain limitations.

## Delivery

ETA16:18Z /01:18KST. Commit the minimal actual fix and this unchanged plan with AI Context explaining v0.50.1/post-ship status, race/verification evidence, approach and tier (T0 for a test-only change). Return clean HEAD/base/files, source-cited root cause and verdict, exact report/command/output paths and any remaining gaps. No plan-only commit if no authorized implementation exists; remove the uncommitted plan after externally retaining it if you stop without implementation. Short complete plain English reports, VERIFIED/INFERRED labels and useful citations. Report blockers or ETA slips over30minutes immediately.

MAIN alone merges develop, pushes, runs CI and operates shared runtime. Hold this lease until MAIN reports its merge. A separate earlier whiteboard head/lease is accepted but unmerged; it remains untouched and is not part of this task. Never reuse old workers in released/recycled roots. No follow-on work or routine extra agents/audits.
