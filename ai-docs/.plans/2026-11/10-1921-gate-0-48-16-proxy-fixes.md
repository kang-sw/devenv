# Release-gate fix round: daemon-hosted proxy findings (ws 0.48.16)

## Request

The ws 0.48.16 release-gate review (range 9e9f33b16..0481c9634) blocked on the
daemon-hosted proxy merge. Fix every item in the findings file's "Fix list"
(C1, I1-I6, T1-T5, M1-M9) on branch `fix/gate-0.48.16-proxy`, applying the
lead decisions L1-L4 stated there. The "Accepted" section is out of scope.

Findings file (the contract for this round):
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/48b1ba1f-01-findings.md`

Authority for intended behavior:
`ai-docs/tickets/.done/261110-feat-ws-mcp-daemon-hosted-proxy-launcher.md`
(Decisions 1-10) and `ai-docs/manuals/ws-mcp.md` "Daemon-Hosted MCP".

## Lead decisions (settled; do not reopen)

- L1. Same version + same staged content hash never replaces the daemon;
  build time breaks the tie only when hashes differ.
- L2. `applyLANChange` ignores a hello whose session declares no dashboard
  manifest, sets `WS_MCP_NO_DASHBOARD=1`, or carries a `WS_CONFIG_HOME`
  different from the daemon's.
- L3. After drain: release lock and endpoint first, then wait for in-flight
  hosted calls before the process exits.
- L4. Fix the remaining daemon-env subprocess and store sites on hosted tool
  paths; only daemon-component code stays process-level.

## Constraints

- Never run go build/test/vet locally (it freezes the user's machine). Use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool -m 30 -- <go args>`.
  Local OK: `nice -n 19 gofmt -l`, `nice -n 19 node --check`, Python tests
  under `agents-plugin/tests` with `nice -n 19`.
- The three launcher copies (`agents-plugin/bin/ws-mcp-launcher.py`,
  `agents-plugin-wsflow/bin/…`, `agents-plugin-pi/bin/…`) stay byte-identical;
  Python stdlib only.
- Never touch the user's dashboard daemons on ports 29417 / 45743 or the real
  cache root; tests use scratch cache roots.
- No `git stash`. Do not push. Do not merge.
- Manuals to read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/wsflow-mirroring.md`. Update
  ws-mcp.md where a fix changes documented behavior (L1, L2, L3, M4, M5).

## Verification

- Remote: `go test ./... -count=1` and `go vet ./...` pass after the last
  commit; `-race` on `./internal/wsdashboard/... ./internal/mcp/... ./cmd/ws-mcp/...`.
- Local: `agents-plugin/tests` pass; launcher copies sha256-identical.
- Each Critical/Important item has a test that fails without its fix
  (state in the commit's AI Context which test pins which item).

## Return

Report per item (fixed / recorded with reason), commits, verification output,
and anything left open. Stop and return before any change outside the fix list
that would alter a ticket decision.
