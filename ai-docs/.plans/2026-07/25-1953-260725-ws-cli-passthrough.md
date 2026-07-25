# Plan: 260725-feat-ws-cli-mcp-fallback-surface — Phase 1: Generic CLI passthrough (tools / call)

## Relevant Ticket Contract

- Add `tools` and `call` subcommands to `cmd/ws-mcp/main.go`, routing through
  the existing `Server` rather than duplicating handler logic.
- Bare `tools`: print the mapping rule (`ws/x.y(a: b)` -> `ws-cli call x.y
  '{"a":"b"}'`) followed by `filteredTools()` reduced to name + description
  (no `inputSchema`).
- `tools <name>`: print that one tool's `inputSchema`.
- `call <name> '<json>'`: build the same request shape `callTool` consumes,
  write the tool's text content to stdout; non-zero exit + readable message
  on tool error, unknown tool, or malformed JSON — `isError` on the tool
  response is the error signal.
- Profile-correctness is a hard constraint: agentless (`WS_MCP_NO_AGENT=1`)
  must not list/dispatch agent-backed tools; session-scope gating in
  `callTool` must apply unchanged (non-lead `session_key` must not reach
  lead-only tools via the CLI).
- `--help` is deliberately left alone; `usage()` text is untouched.
- Verification boundary (from ticket Phase 1): `tools` lists exactly the tool
  names `tools/list` returns for the same profile in both full-ws and
  agentless mode, with no `inputSchema`; `tools <name>` matches that tool's
  schema from `tools/list`; a `call` round-trip against a real tool using a
  `session_key` minted by a separate process returns the same text the MCP
  path returns; the documented cold-start call `call workflow_manual
  '{"session_key":"obsidian-latch","root":"<abs>"}'` mints a usable lead key;
  error paths exit non-zero.
- Spec impact is closeout-only: "Contract-first spec: no ... the shim naming
  and skill behavior are reflected into the specs at closeout" — do not author
  `mcp-tools.md` changes as an implementation step of this phase.

## Out of Scope

- Phase 2 (`bin/ws-cli` / `bin/ws-cli.cmd` shims, per-plugin env baking,
  `skills_mirror.go` `\bws-cli\b` pattern, lazy `import urllib.request` in the
  launcher).
- Phase 3 (`mcp-server-repair` skill, entry-point pointer lines in the eight
  front-door skills, skill-manifest registration).
- The existing hand-written subcommands (`git`, `tickets`, `specs`,
  `mental-models`, `references`, `config`, `path`, `runtime`, `mercenary`) —
  ticket explicitly keeps them as-is (load-bearing for tests) but non-canonical;
  no changes to them in this phase.
- `usage()` / `--help` text — ticket says leave it alone.
- Spec authoring (`mcp-tools.md` reflection) — closeout-only per ticket's
  "Contract-first spec: no" decision.

## Codebase Findings

- `agents-plugin-tool/cmd/ws-mcp/main.go#26-64` — top-level `switch os.Args[1]`
  dispatch; add `case "tools":` and `case "call":` here alongside the existing
  `git`/`tickets`/`specs`/... cases. `usage()` (`main.go#66-72`) is untouched
  per ticket.
- **Risk signal (resolved by design, not a blocker):** `filteredTools()`
  (`internal/mcp/server.go#4445-4463`) and `callTool` (`server.go#485`) are
  **unexported methods** on `*mcp.Server`. `cmd/ws-mcp` is package `main` and
  cannot call them directly across the package boundary — a naive plan to
  "call `s.filteredTools()`/`s.callTool()`" from `main.go` will not compile.
  The existing, already-used escape hatch is `Server.ServeStdio`
  (`server.go#142`, exported), which `serve()` (`main.go#88-105`) and `smoke()`
  / `runSmoke()` (`main.go#107-148`) already drive by feeding synthetic
  JSON-RPC lines through a reader and capturing the writer. `runSmoke` already
  does exactly this for `tools/list` and `tools/call` (`main.go#130-134`), so
  the CLI subcommands should follow that precedent: construct a one-line
  JSON-RPC request, run it through `mcp.NewServer(...).ServeStdio(ctx,
  strings.NewReader(line), &buf)`, and parse the single JSON-RPC response line
  out of `buf`. This satisfies "route through the existing Server, don't
  duplicate handler logic" with zero changes to `internal/mcp`.
- `internal/mcp/server.go#238-260` (`handle`) — confirms `tools/list` returns
  `map[string]any{"tools": s.filteredTools()}` and `tools/call` dispatches via
  `s.callTool(ctx, req)`; no `initialize` call is required first (no
  "initialized" state is tracked anywhere in the package — grepped, no hits),
  so the CLI can send a bare `tools/list`/`tools/call` line without a
  preceding `initialize` handshake.
- `internal/mcp/server.go#485-522` (`callTool`) — params shape is
  `{"name": string, "arguments": map[string]any, "_meta": map[string]any}`
  unmarshaled from `req.Params` (`json.RawMessage`). This is the exact shape
  the CLI must build: `{"jsonrpc":"2.0","id":<n>,"method":"tools/call","params":{"name":<name>,"arguments":<parsed-json>}}`.
  Gating already applied unchanged inside: `NoAgentMode()` + `noAgentHiddenTool`
  (line 498), `s.toolAllowed` (line 501), and the keyed lead-only gate (lines
  516-521, `isLeadOnlyTool` / `roleAllowsTool`). None of this needs to be
  reimplemented or mirrored in `main.go`.
- `internal/mcp/server.go#3216-3236` (`toolTextResponse` /
  `toolErrorTextResponse`) — the tool-level result shape: success is
  `response{Result: {"content":[{"type":"text","text":...}]}}`; tool error is
  `response{Result: {"isError": true, "content":[{"type":"text","text":...}]}}`.
  A JSON-RPC-level error (unknown tool/method, `-32601`; invalid params,
  `-32602`) instead sets `response.Error` (`rpcError{Code, Message}`,
  `server.go#106-109`, built via `errorResponse` at `server.go#3238-3240`), not
  `Result`. The CLI's error-exit logic must check **both** shapes: `resp.Error
  != nil` (JSON-RPC-level) and `resp.Result["isError"] == true` (tool-level).
  Which path an *unknown tool name* takes was not confirmed to the bottom of
  the ~2000-line `switch params.Name` in `callTool` (`server.go#528` onward) —
  the implementer must read that switch's `default:` case before writing the
  CLI's error-message text, so it does not silently swallow one of the two
  error shapes.
- `internal/mcp/server.go#4445-4463` (`filteredTools`) already applies
  `NoAgentMode()`/mercenary-hidden/`explicitAllowedTools` filtering via
  `s.toolAllowed` (line 4502-4513) and drops `permanentlyHiddenTool` (`exec.*`)
  entries — this is exactly the agentless-safe list the ticket wants for bare
  `tools`. No extra filtering logic needed in `main.go`; only reduce each
  entry to `name`/`description` (drop `inputSchema`) for the bare-`tools` case,
  and look up the single entry's `inputSchema` for `tools <name>`.
- `internal/mcp/session_auth.go#124-172` (`mint`/`lookup`) — session keys are
  one-JSON-file-per-key under `WS_CACHE_HOME`'s keys dir, so a `session_key`
  minted by one `ws-mcp` process is resolvable by a second, separate process
  sharing the same `WS_CACHE_HOME` — this is the mechanism the ticket's
  "session_key minted by a separate process" verification step and the
  cold-start `call ferrule` -> `call workflow_manual` sequence rely on.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#675-682` (`wsMCPTestBin`) and the
  surrounding tests (e.g. `TestRuntimeCapabilitiesCommandReportsNoAgentSurface`,
  `main_test.go#103-141`) — established pattern: `go build -o bin .` once per
  test, then `exec.Command(bin, args...)` with env vars
  (`WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`, `WS_CACHE_HOME=<tmp>`), and
  assert on stdout/stderr/exit code. `main_test.go` is the single file for all
  `cmd/ws-mcp` subcommand tests — no split-file precedent in this package
  (only `main.go` + `main_test.go` + the two `parent_watch_*` files exist under
  `cmd/ws-mcp/`).
- `internal/mcp/server_test.go#197-236` (`serveStdioWithSession` /
  `withSessionKeyInToolCalls`) and `internal/mcp/session_auth_test.go#20-58`
  (`callLogin` / `parseLoginResponse`) — the in-package pattern for minting a
  session key by calling the `ferrule` tool through `ServeStdio` and parsing
  the returned key/root out of the tool-text response. The CLI-level
  equivalent (two separate subprocess invocations sharing `WS_CACHE_HOME`) is
  the shape the new `cmd/ws-mcp` test should use for the cross-process
  round-trip check; these are test-only helpers in a different package and
  cannot be imported directly, only mirrored.
- `agents-plugin-tool/internal/mcp/server.go#66-76` (`isLeadOnlyTool`) — the
  lead-only tool set (`ferrule`, `workflow_manual`, `workflow_state`,
  `tickets.sage_stamp`, `lead.*`, workflow-preference writer tools) is the
  concrete set a non-lead-scoped `session_key` must be rejected from reaching
  through `ws-mcp call`; useful as the non-lead-gating test's negative-case
  tool name.
- `internal/mcp/workflow_manual.go#196-225` (`handleWorkflowManual`) — the
  ticket's cold-start example key `obsidian-latch` is asserted to be the
  `freshBootstrapKey` sentinel (referenced but not dereferenced in this
  survey); confirm the constant's value before asserting exact wording in the
  cold-start test.

## Implementation Plan

1. In `cmd/ws-mcp/main.go`, add `case "tools":` and `case "call":` to the
   top-level switch (`main.go#32-63`), each calling a new handler function
   (`toolsCommand(args)` / `callCommand(args)`), following the existing
   per-subcommand function style (`gitCommand`, `ticketsCommand`, etc.).
2. Add a shared helper, e.g. `runToolsRequest(line string) (localResponse,
   error)`, that: builds `mcp.NewServer(defaultRoot("."), version,
   sourceCommit)` (root does not gate `tools/list`/`tools/call` dispatch —
   confirmed no code path in `callTool`/`filteredTools` reads `s.root`; only
   `session_key` resolves root inside individual handlers), calls
   `server.ServeStdio(ctx, strings.NewReader(line+"\n"), &buf)`, and decodes
   the single JSON-RPC response line from `buf` into a local struct (the
   package-level `response` type in `internal/mcp` is unexported, so `main`
   needs its own decode shape: `{Result map[string]any; Error *struct{Code
   int; Message string}}`).
3. `toolsCommand(args []string)`:
   - No argument: send `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`;
     from the decoded `Result["tools"]` (`[]any` of `map[string]any`), print
     the mapping-rule line, then for each tool print `name` + `description`
     only (marshal a reduced struct/map — drop the `inputSchema` key). Plain
     text only — this is the fallback's recovery entry point, no `--format
     json` needed.
   - One argument (`tools <name>`): same `tools/list` round-trip, find the
     entry whose (already profile/namespace-filtered) `name` matches, and
     print its `inputSchema` (`json.Marshal`); if no match, exit non-zero
     with a readable "tool not found" message — going through the same
     filtered list means an agentless-hidden tool name reports not-found
     rather than leaking its schema.
4. `callCommand(args []string)`:
   - Expect exactly two positional args: `<name>` and `<json>`. Validate the
     `<json>` argument parses as JSON (`json.Unmarshal` into
     `map[string]any` or `json.RawMessage`) before building the request —
     malformed JSON is a CLI-level readable error + non-zero exit, not a
     JSON-RPC parse error surfaced from `ServeStdio`.
   - Build `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":<name>,"arguments":<parsed-json>}}`,
     run it through the same helper as step 2.
   - Decode the response: if `resp.Error != nil` (JSON-RPC-level — confirm
     during implementation whether unknown-tool lands here or in
     `isError`, per the open question in Codebase Findings), print
     `resp.Error.Message` to stderr and exit non-zero. Else if
     `resp.Result["isError"] == true` (tool-level error), extract
     `content[0].text` and print it to stderr, exit non-zero. Else extract
     `content[0].text` from `resp.Result` and print to stdout, exit 0.
5. Keep the new code in `main.go` (existing convention for this package: no
   file-splitting precedent under `cmd/ws-mcp/`).

## Verification Plan

- `go build ./...` (or targeted `go build ./cmd/ws-mcp/...`) from
  `agents-plugin-tool/`.
- New tests in `agents-plugin-tool/cmd/ws-mcp/main_test.go` (existing file,
  existing `wsMCPTestBin` + `go build -o bin .` + `exec.Command` pattern):
  - Bare `tools` output contains the mapping-rule line and, for a fixed
    profile, exactly the tool names a direct `tools/list` JSON-RPC call
    returns for the same env (drive the comparison via
    `mcp.NewServer(...).ServeStdio` in-process, since that package is
    importable from the test binary) — assert no `inputSchema` key appears
    in the bare-`tools` text.
  - `tools <name>` output, parsed, matches that tool's `inputSchema` from a
    direct `tools/list` call.
  - Agentless profile (`WS_MCP_NO_AGENT=1`): bare `tools` output excludes
    `mercenary.*` and other `noAgentHiddenTool` names (mirror the exclusion
    list already asserted in
    `TestRuntimeCapabilitiesCommandReportsNoAgentSurface`,
    `main_test.go#103-141`).
  - Cross-process round-trip: two subprocess invocations sharing
    `WS_CACHE_HOME` — `call ferrule '{"root":"<tmp-git-root>"}'` to mint a
    session_key from its printed output, then `call <a root-aware tool,
    e.g. tickets.list or git.status> '{"session_key":"<minted>", ...}'` in a
    fresh process — text output must match the equivalent
    `ServeStdio`-driven in-process call.
  - Non-lead scope gating: mint a delegate/leaf-scoped key (confirm the
    existing mechanism in `internal/mcp/session_auth.go` for minting a
    non-lead scope during implementation) and assert `call ferrule` / `call
    workflow_manual` (both in `isLeadOnlyTool`) is rejected through the CLI
    exactly as it is through `ServeStdio` directly.
  - Malformed JSON argument: `call <name> 'not-json'` exits non-zero with a
    readable message, without needing a running dispatch.
  - Unknown tool name: `call nonexistent-tool '{}'` exits non-zero with a
    readable message.
  - The documented cold-start call `call workflow_manual
    '{"session_key":"obsidian-latch","root":"<abs>"}'` succeeds (mints/returns
    a usable lead key) — confirm `obsidian-latch` maps to the
    `freshBootstrapKey` sentinel in `internal/mcp/workflow_manual.go#196-225`
    before asserting exact wording.
- No spec-file edits as part of this phase's verification (closeout-only per
  ticket).

## Escalations

- None.
