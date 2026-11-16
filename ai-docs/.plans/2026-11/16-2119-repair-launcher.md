# Repair: launcher regressions and release-gate freshness (B2-B5, I13, launcher minors)

All in `agents-plugin/bin/ws-mcp-launcher.py` (keep the agents-plugin-wsflow and agents-plugin-pi copies byte-identical) plus the release scripts.

B2. `version` with `WS_MCP_BOOTSTRAP_BINARY` no longer installs the build (commit 45be54fbb changed `if inspection and not bootstrap_forced:` to `if inspection:` near :2152, so inspection returns before need_install/repair; the bootstrap repair branch near :2201-2202 is dead). Consumer contract: `agents-plugin-pi/src/local-devenv.ts` `createChildRuntimeEnsurer` (~:303-330) builds from source, runs `launcher version` with the bootstrap env, and expects the build installed at the new contract path and its version printed. Restore that contract without undoing what 45be54fbb intended for plain inspection. Add a launcher test for bootstrap + `version` (with and without a running daemon).

B3. A stale `endpoint.json` with a reused or unqueryable PID refuses daemon start permanently (~:1702-1708, commits 45be54fbb, 79841d026). Windows leaves endpoint.json behind at reboot/logoff and reuses PIDs; access-denied OpenProcess and POSIX EPERM count as "unknown". Result: every MCP session fails until the user deletes the file. Lead technical decision: prove absence without trusting the PID, using a non-blocking try-lock on the Go daemon's `dashboard.lock` (same path the Go side locks), released before the starter runs; the ticket only forbids holding that lock across the starter. If the try-lock succeeds, the recorded daemon is gone regardless of the PID; if it fails, keep today's refusal/wait behavior. Tests for reused-PID (alive, unrelated process), EPERM/unknown, and live-daemon cases.

B4. Cold start always waits the full 20 s swap grace (~:1685): `ensure_healthy` calls `wait_healthy(DAEMON_SWAP_GRACE_SECONDS)` whenever health is None, even with no endpoint file. With Claude's 30 s MCP startup timeout this makes the first session of the day fail. Skip the grace when there is no endpoint record (and, after B3, when absence is proven); keep it for a real swap in progress. Update `test_no_successor_starts_once_only_after_the_full_health_window` accordingly and add a no-endpoint cold-start test asserting no grace wait.

B5. The release gate does not check workflow-bundle freshness: `.github/workflows/ws-mcp-release.yml` (~:104-111) and `agents-plugin-tool/scripts/build-release-assets.sh` (~:18-27) run only `ws-pi-payload.py --check-source`. Add `prepare-workflow-bundles.py --check` (find its real path) to both. Also add it to the regular CI workflow if a cheap Python step fits there.

I13. `agents-plugin-pi/package.json` prepare/prepack hard-code `python3` (fails on stock Windows); `agents-plugin-pi/test/workflow-bundle-consumer.test.mjs:107` uses `python`. Make both pick an available interpreter portably (e.g. a tiny node wrapper trying python3, python, py -3), consistently.

Minors in the same file (fix while there):
- Docstring near :951 still says the lock-holding daemon prunes staged executables; pruning is now a no-op (spawn.go PruneStagedExecutables). Fix the text.
- Lifecycle mode (~:2205-2212): a failed `daemon-start` drops its stdout JSON report and exit code; forward them as before 45be54fbb.

Verify: the launcher Python test suites under agents-plugin*/tests that cover start/proxy/payload/bundle (run them; isolate HOME and WS_CACHE_HOME to a temp dir), `prepare-workflow-bundles.py --check` and `ws-pi-payload.py --check-source` both pass, and the three launcher copies are byte-identical. If the bundles embed the launcher, regenerate them so `--check` stays green.

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
