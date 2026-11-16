# Repair: idle stop scope and strict worker idleness, Pi retired-tool text, Pi probe cache on Windows (I2, I4, I5, I6, B1-wsagent)

Owner decision (1188)=a, with the owner's words: "pi.spawn의 경우 다만 \"멈춤\" 조건을 엄격하게 해석해야 합니다. ... 저걸 0으로 돌린 게 실제 작업중임에도 불구하고 자꾸 쉬었음 판정되어서 짜증나서 돌린 거였거든요." Meaning: idle stop stays off for leads; workers get the old idle stop back, but a worker that is actually working must never be judged idle.

I2. `agents-plugin-tool/internal/wsagent/profile.go` (~:46, :147-151, 133bce57e): `DefaultIdleTimeout = 0` applies to every profile without `idle_timeout_seconds`, including non-lead workers started by `startWorker` (`runner.go` ~:2317): pi.spawn children, builtin explore, search-answer, tracker support, pi-worker, mini-lead. Make the 0 default apply to Lead profiles only; non-lead workers default to the previous 600 s. Tests for a lead and a worker profile.

Strict idleness (required together with I2, or the owner's original annoyance returns). The gate is `conv.idleNow` (`runner.go` ~:3373): not idle while a turn runs, while replaying, while `bgTasks != 0`, or while a child conversation is active. Gaps found:
- Background task counts reach the daemon only from the Pi owner lead: in `agents-plugin-pi/src/index.ts` (~:905-941) both reporters (`createDashboardFeed` taskCounts, `createHostedDashboardEvents` taskCounts) run only when `readSpawnRole(process.env) === undefined`. A pi.spawn worker running a long background shell or monitor (`background-tasks.ts`) therefore reports 0 tasks and is judged idle after its turn ends. Make spawned workers report their background task counts on the channel the daemon reads for that conversation (trace where `c.bgTasks` is set, `runner.go` ~:2920), without enabling the owner-lead-only dashboard feed for children.
- I4: Pi lead idle stop ignores running native `ws-execute` workers (`index.ts` ~:918, :938 report only `countBackgroundTasks`; commits 319393501, 911a7a1da). A lead that dispatches a long ws-execute and ends its turn is stopped and the worker dies. Count running ws-execute jobs as active work in the same report.
- Decide whether `monitors` (not just `timed`) should block idleness; a running monitor is work in progress, so the lead's default expectation is that it blocks. State the choice in AI Context.
Tests: Go-side idleNow cases for a worker with background tasks and with a running execute job; Pi-side unit tests that a spawned worker reports counts and that execute jobs are counted.

I5. `agents-plugin-pi/src/execute-gateway.ts` (~:687, :715, :836, d609726e2) still tells the model to use the retired `ws-agent-spawn` / `ws-agent-stop`; replace with the current guidance (read how agents are spawned now in that package). Also `agents-plugin-tool/internal/wsagent/testdata/pi-native-payload-identity.test.mjs` (~:371, :394, :397) calls the retired tools; update it.

I6. `agents-plugin-pi/src/index.ts` (~:381) with `agent-storage.ts` (~:700-705, 160bd9d01): the retirement notice says retained native homes are held "until explicitly released", but no release path exists, and it notifies once per retained home on first start. Make the text true (say what actually happens and what the user can do) and send at most one notice per start.

B1 (Windows) wsagent probe cache tests failed on windows-latest CI:
- TestPiProbeCacheCommandInvalidation/PATH: `exec: "pi": executable file not found in %PATH%`; /nanosecond_mtime also failed.
- TestPiProbeCacheConfigIdentityAndAbsences/relative: `filepath.Rel` cannot relate a C: temp dir to the D: checkout; /auth_mtime: "models identity unchanged after auth mtime".
Decide per subtest whether the product code is wrong on Windows (PATHEXT / shim name lookup, mtime granularity in the identity) or the test assumes POSIX; fix accordingly. You cannot run Windows: reason from code, make tests portable, and say what is unverified.

Verify: `go test ./internal/wsagent -run '<touched tests>'`, the Pi package unit tests you touched (`node --test` on the specific files), and `npm run build`/typecheck if the package has one. The Pi dist bundle and payload manifests must stay consistent (`ws-pi-payload.py --check-source`); regenerate if the package build requires it.

## Common rules (all repair delegates)
- Source of findings: lead range review e93073373..b8e434d81 (release gate, verdict NEEDS FIX). Owner decision (1187)=a: fix the blocking set and the Important set before v0.50.2, then the lead re-reviews the same base.
- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available for the Node-harness tests.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror (owner is editing them).
- Do not ssh anywhere, do not launch Electron or any GUI, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- Mirrors stay in sync: the three `ws-mcp-launcher.py` copies are byte-identical; wsflow mirrors follow `ai-docs/manuals/wsflow-mirroring.md`.
- Any change to a cross-version surface states its compatibility tier in the commit `## AI Context` (expected T0; stop and report before anything above T2).
- The repository is public: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior fix lands with a test that fails before the fix.
- Commit on your branch only; do not merge. If a fix turns out to need a product decision or grows beyond the listed files' area, stop and report instead of expanding.
- Return: commits, tests run with results, anything left unfixed and why.
