# agy Phase 1: harness descriptor, tool-name aliases, descriptor-driven harness

## Goal

Implement Phase 1 of
`ai-docs/tickets/todo/261118-feat-ws-agy-tool-names-and-worker-role.md` on
`track/agent-v2` in worktree
`D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937` (base
`eafe2779a`). Read the ticket's `## Decisions` and `### Phase 1` first; they
are the contract.

Problem: agy forwards ws MCP tools under dotted names (`mcp_ws_tickets.query`);
the Anthropic API requires `^[a-zA-Z0-9_-]{1,128}$`, so Claude models inside
agy fail every turn. The hosted agy lead's manual also renders with no harness.

## Settled decisions (do not reopen)

- (143) A small harness descriptor on `BackendRegistration`
  (`agents-plugin-tool/internal/wsagent/registration.go` L14-24) holds
  per-harness data; common code reads it and gains no `agy` branch. Phase 1
  adds `ToolNameScheme` and `ManualHarness` only (`WorkerCapable` is Phase 2,
  not this assignment). Claude declares `ManualHarness: "claude"`, agy
  declares `ManualHarness: "agy"` and the sanitize scheme; every other
  registration keeps the identity scheme and an empty harness.
- (136) A connection is treated as agy through its hosted lead record's
  `Backend`, which selects the descriptor. No URL marker in `agyHTTPServer`.
- (137) Alias mapping mirrors `agents-plugin-pi/src/bridge.ts`
  `sanitizeToolName` (around L567); one generic builder produces the alias
  table and refuses a collision (pinned by a test).
- Reverse map: in `callTool` (`internal/mcp/server_dispatch.go` about
  L36-71) after `hostedSessionKeyRule` and before `toolAllowed`, so
  allowlists and dispatch see canonical dotted names.
- `advertisedToolName` (`internal/mcp/server_inventory.go` L174-176, identity
  stub today) reads the connection's alias table; a connection without a
  hosted lead record keeps dotted names.
- (138)/(145) The hosted lead's harness (the server's harness for that
  connection, and `renderLeadManual` in `cmd/ws-mcp/agent_mcp.go` L71-100)
  comes from `ManualHarness`; replace the `if backend == wsagent.BackendClaude`
  line. Do not add an agy case to `server_harness.go`'s clientInfo list.
- (139) Compatibility T0: no stored shape, hello field, or tool argument
  change; non-agy callers see dotted names unchanged.

How the descriptor reaches the MCP connection (the hosted lead record to the
registration lookup) is yours to choose, following existing wiring
(`cmd/ws-mcp/dashboard.go` L299-305 registers backends). Prefer the simplest
path that keeps common code harness-neutral. If the wiring would need a
cross-module interface change beyond these sites, stop and report.

## Constraints

- Read before editing: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`.
- No local `go test` and no running ws exes on this PC (antivirus). Build and
  test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run agy-phase1 <min> --sync 'D:/Workspace/Repos/.ws-dashboard-worktrees/devenv/261118-0937' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet <touched packages> && go test <touched packages>'`.
  Capture output to a file under `D:/Temp/agy-phase1/` if the call returns
  nothing; delete temp files after use.
- Keep the change surgical; no refactor of the other harness-name branches
  (captured in `261118-refactor-ws-harness-name-branches`).
- A test failing on the base unchanged may be noted, not fixed.

## Verification

- Unit tests: alias table round trip, collision refusal, identity scheme
  unchanged; descriptor-driven harness render (Claude unchanged, agy new);
  `callTool` accepts an alias on an agy connection and refuses nothing new on
  others.
- w5090 `go build ./...`, `go vet` and `go test` of the touched packages pass.
- The live agy check is the owner's later, after a daemon swap; not part of
  this assignment.

## Done when

- One commit on `track/agent-v2` with the implementation, tests, this plan
  file, and a `### Result (<short-hash>) - 2026-11-18` appended under
  `### Phase 1` of the ticket (what landed, deviations, w5090 results,
  Forward notes for Phase 2). Commit message carries `## AI Context` and the
  ticket stem under `## Ticket Updates`, ending with the worker's own model
  Co-Authored-By trailer.
- Report: commit hash, files changed, test results, and any surprise.
