# Release-gate fix round 2: daemon-hosted proxy (ws 0.48.17)

## Request

The ws 0.48.17 release-gate review of fd82c26de..6ddde41cb blocked. Fix every
item under "Unit A" in the findings file on branch `fix/gate-0.48.17-proxy`:
- the Important items P-I1, P-I2, T-I1 and T-I2;
- the "Minor, fix in this round" items of Unit A.

The "Accepted" list is out of scope. So is Unit B, which another delegate fixes
in parallel.

Findings file, the contract for this round:
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/fc07fd75-01-findings.md`

Authority for intended behavior:
- `ai-docs/tickets/.done/261110-feat-ws-mcp-daemon-hosted-proxy-launcher.md`
  (Decisions 1-10)
- `ai-docs/manuals/ws-mcp.md`, section "Daemon-Hosted MCP"
- lead decisions L1-L4, below

The previous round's plan, for context:
`ai-docs/.plans/2026-11/10-1921-gate-0-48-16-proxy-fixes.md`.

## Lead decisions (settled; do not reopen)

- L1-L4 from the previous round hold.
- L4 means every hosted tool path runs subprocesses and resolves state on the
  connection's environment. Only daemon-component code stays process-level.
- P-M7: when a caller environ is supplied and the program is not on its PATH,
  `wsexec.LookPath` returns an error. Never fall back to the daemon PATH. A nil
  caller environ keeps the process-level lookup.
- P-I2: pick the simplest mechanism that keeps one owner identity across the
  hosted session and the direct fallback child. Passing the launcher PID to the
  child as its owner PID is the expected shape.
- P-M5: skip tmp files and entries younger than a short grace period. A simple
  age check is enough.
- Windows LAN test: restore Windows coverage if a shim is straightforward;
  otherwise skip only the slow-call part, and say which you chose.

## Constraints

- Never run go build/test/vet locally; it freezes the user's machine. Use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool -m 30 -- <go args>`.
  These are OK locally:
  - `nice -n 19 gofmt -l`;
  - `nice -n 19 node --check`;
  - the Python tests under `agents-plugin/tests`, run with `nice -n 19`.
- The three launcher copies stay byte-identical and use the Python stdlib only:
  - `agents-plugin/bin/ws-mcp-launcher.py`
  - `agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  - `agents-plugin-pi/bin/ws-mcp-launcher.py`
- Never touch the user's dashboard daemons on ports 29417/45743 or the real
  cache root. Tests use scratch roots.
- No `git stash`. Do not push. Do not merge.
- Read these manuals before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`
  - `ai-docs/manuals/ws-mcp.md`
  - `ai-docs/manuals/wsflow-mirroring.md`
- Update ws-mcp.md and windows-dogfood.md where the findings call for it, and
  wherever a fix changes documented behavior.

## Verification

- Remote, after the last commit:
  - `go test ./... -count=1` and `go vet ./...` pass;
  - `-race` passes on `./internal/wsdashboard/... ./internal/mcp/... ./cmd/ws-mcp/...`.
- Local: the `agents-plugin/tests` suite passes, and the launcher copies are
  sha256-identical.
- Each Important item has a test that fails without its fix. For T-I1, show that
  the L2 test fails with the guards removed (a temporary local revert, then run
  remotely). State in each commit's AI Context which test pins which item.

## Return

Report:
- each item: fixed, or recorded with the reason;
- the commits;
- the verification output;
- anything left open.

Stop and return before any change outside the findings that would alter a
ticket decision.
