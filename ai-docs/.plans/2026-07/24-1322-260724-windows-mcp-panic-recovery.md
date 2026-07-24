# Plan: 260724-bug-windows-mcp-mid-session-disconnect — Phase 1: Request-goroutine panic recovery + crash capture

## Relevant Ticket Contract
- Add `recover()` to the per-request goroutine at `internal/mcp/server.go:172-184`; on panic, fail only that request with a JSON-RPC error (do not crash the process) and persist the panic value + full stack to an **always-on** dedicated crash file under the runtime dir (`ai-docs/tickets/ready/260724-bug-windows-mcp-mid-session-disconnect.md:114-122`).
- The always-on crash file is the **required** sink; `WS_MCP_DEBUG_LOG` is opt-in and downstream never set it — it may only **mirror** the event as an optional secondary (ticket:117-120).
- Specify the concrete crash-file path and overwrite/rotation behavior so the "documented runtime-dir location" in Spec Impact is concrete (ticket:120-122).
- Confirm the per-operation store open/close model (`internal/wsstore/store.go:181`) bounds any connection/`-wal`/`-shm` leak so a recovered panic cannot wedge the process (ticket:125-127).
- Add a regression test: a deliberately-panicking write handler is followed by a **successful subsequent request on the same process**; reject silently swallowing panics — the trace must be persisted **and** the request must return a visible error (ticket:127-130).
- This slice is fully cross-platform (ticket:131, Constraints:93).

## Out of Scope
- Phases 2-4 (launcher-side abnormal-exit diagnostics, Windows Job Object/process-lifecycle hardening, SQLite multi-process/point-read retry discipline) — separate phases in the same ticket.
- Crash-file rotation or max-size capping: Phase 1 deliberately matches the existing `WS_MCP_DEBUG_LOG` sink's no-rotation, append-only precedent (`internal/mcp/server.go:264-269`); revisit only if disk growth becomes a real problem post-fix (crashes should be rare once recovered).
- Any change to `agents-plugin/bin/ws-mcp-launcher.py` (Windows launcher) — that is Phase 2/3 territory.
- Extending panic recovery to `wsagent`'s async worker goroutine — it already has its own `recover()` (`internal/wsagent/agent.go:1016-1024`); untouched.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/server.go#L172-184` — the per-request goroutine has no `recover()`; a panic anywhere inside `s.handle(reqCtx, req)` currently crashes the whole `ws-mcp serve` process. `defer wg.Done()`, `defer cancel()`, `defer requests.Delete(id)` are declared in that order (lines 177-179); a new recover-defer appended **after** these three will run **first** on unwind (Go LIFO), matching the intended catch-before-cleanup order.
- `agents-plugin-tool/internal/mcp/server.go#L241-270` — `appendDebugEvent(event, fields)` already does exactly the "always record + optionally mirror to `WS_MCP_DEBUG_LOG`" shape needed: it always appends to the in-memory ring buffer, and only opens/appends to `os.Getenv("WS_MCP_DEBUG_LOG")` if non-empty (lines 256-269, `O_CREATE|O_WRONLY|O_APPEND`). Reuse this function unmodified for the "optional secondary mirror" part of the contract instead of duplicating the mirror logic.
- `agents-plugin-tool/internal/wsstate/paths.go#L84-96` — `CacheRoot(opts)` resolves `WS_CACHE_HOME` (or `~/.cache/ws@kang-sw-devenv`) with no git/root dependency. This is the only cache-root resolution that is safe to call from a goroutine that may not know which project root the panicking request belonged to.
- `agents-plugin-tool/internal/mcp/session_auth.go#L92-105` — direct precedent for a **global, cross-root** artifact living straight under `CacheRoot()` (the `keys/` dir for session keys), with the same justification: "all server instances agree on the location" without git/root resolution. The crash file should follow this same pattern (a new top-level dir under `CacheRoot()`, not under any per-project `proj/<worktreeKey>/` layout).
- `agents-plugin-tool/internal/mcp/server.go#L3015-3038` — `toolTextResponse`/`toolErrorTextResponse` produce a **successful** JSON-RPC response with `isError:true` content (business-level tool failure); `errorResponse(id, code, message)` produces a **top-level JSON-RPC `error`** (protocol/dispatch-level failure, used today for `-32601`/`-32602`/`-32700`). A panic is an unexpected internal failure, not a normal tool outcome, so it belongs in the `errorResponse` family, using an unused code in the JSON-RPC reserved server-error range (`-32000`).
- `agents-plugin-tool/internal/wsagent/agent.go#L1016-1024` — the only existing `recover()` in the codebase: on panic it logs to a runtime log, marks the call failed, and converts the panic into a returned `error`. This is the reuse pattern for "convert panic to a visible failure + persist a trace," adapted here to JSON-RPC instead of the agent runtime log.
- `agents-plugin-tool/internal/wsstore/store.go#L168-203` (`Manager.Open`) — `db.SetMaxOpenConns(1)` (line 190); a fresh `*Store`/`*sql.DB` is opened per call, never a shared/global handle.
- `agents-plugin-tool/internal/execjob/execjob.go#L557-560` and `#L616-624` — the only MCP-reachable write handlers that touch `wsstore` (via `exec.*` tools) follow `store, err := wsstore.NewManager(...).Open(root); ...; defer store.Close()` immediately after a successful `Open`, with no panic-prone code in between. **Risk-signal check, resolved**: Go's panic/defer semantics run all already-registered `defer` calls during unwind regardless of where (or whether) `recover()` is ultimately called — `defer store.Close()` inside the panicking handler still fires before control reaches the new goroutine-level recover in `server.go`. So the per-operation open/close model already bounds the connection lifetime for a recovered panic; **no additional store-cleanup code is needed in Phase 1**, only this confirmation (worth stating explicitly in the commit/PR since the ticket phrasing — "recover() skips the panicking handler's own defers" — reads more strongly than actual Go semantics support; the real gap Phase 1 closes is the *process-level* crash, not a defer-skipping problem).
- `agents-plugin-tool/internal/mcp/server.go#L352-391` (`callTool`) — gating (`toolAllowed`, keyed capability gate) runs before the `switch params.Name {` at line 391; a keyless call to a non-`config.*`/non-`mercenary.*` tool like `todo.append` passes all gates unconditionally (`toolAllowed` at `server.go:4288-4299` defaults to `true`; the keyed gate at 383-389 only applies when a `session_key` argument is present). This makes `todo.append` a low-fixture-cost "write handler" for the regression test's deliberate panic.
- `agents-plugin-tool/internal/mcp/server_test.go#L402-436` (`TestServeStdioLogsCancellationNotificationsWhenEnabled`) and multiple files (`bootstrap_alarm_test.go`, `doc_coverage_alarm_test.go`, `prefer_mercenary_phase2_test.go`) — established test idiom: `t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))` to make cache-root-relative paths deterministic and isolated per test. Reuse this exact idiom for asserting on the new crash file's location and contents.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/server.go`, near `appendDebugEvent` (after line 270), add:
   - `crashLogPath() (string, error)`: resolve `wsstate.CacheRoot(wsstate.Options{})`, join with `"crash"`, `os.MkdirAll` it (0o755), return `filepath.Join(dir, "mcp-panic.log")`.
   - `recordPanic(method, id string, recovered any, stack []byte)`: build a record map matching `appendDebugEvent`'s shape (`ts`, `event:"request.panic"`, `id`, `method`, `panic: fmt.Sprint(recovered)`, `stack: string(stack)`); marshal to one JSON line; open the crash-log path with `O_CREATE|O_WRONLY|O_APPEND, 0o644` and write it in a single `Write` call (append-only, no rotation, matching the existing `WS_MCP_DEBUG_LOG` sink). If resolving the cache root or opening/writing the file fails, fall back to `fmt.Fprintf(os.Stderr, ...)` so a broken/unwritable cache home never fully swallows the trace (the file is the *required* sink, so its own failure needs a last-resort fallback, unlike the best-effort-only `WS_MCP_DEBUG_LOG` mirror). Then call `appendDebugEvent("request.panic", fields)` unchanged, to populate the ring buffer and transparently mirror to `WS_MCP_DEBUG_LOG` when set.
2. In `ServeStdio` (`server.go:176-183`), append a fourth `defer` **after** the existing `defer wg.Done()` / `defer cancel()` / `defer requests.Delete(id)` (so it runs *first* on unwind):
   ```go
   defer func() {
       if r := recover(); r != nil {
           stack := debug.Stack()
           recordPanic(req.Method, id, r, stack)
           resp := errorResponse(req.ID, -32000, fmt.Sprintf("internal error: request handler panicked (%s)", req.Method))
           if err := writeResponse(resp); err != nil {
               appendDebugEvent("response.write_error", map[string]any{"id": id, "error": err.Error()})
           }
       }
   }()
   ```
   Add `"runtime/debug"` to the import block. Do not include the raw panic value or stack text in the client-visible `errorResponse` message — those stay server-side in the crash file/debug log only.
3. Add a test-only seam: a package-level `var testPanicHook func(toolName string)` (doc comment: always `nil` in production; set only by `_test.go` files in this package to exercise goroutine-level panic recovery with a real dispatched tool call). Call it at the top of `callTool`, immediately before `switch params.Name {` (`server.go:391`): `if testPanicHook != nil { testPanicHook(params.Name) }`.
4. No changes to `internal/wsstore` or `internal/execjob` — Codebase Findings above confirm the existing per-call `defer store.Close()` already bounds the leak for a recovered panic.
5. Add a regression test (new file `internal/mcp/panic_recovery_test.go`, package `mcp`):
   - `t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))`.
   - `testPanicHook = func(name string) { if name == "todo.append" { panic("deliberate test panic: todo.append") } }`; `t.Cleanup(func() { testPanicHook = nil })`.
   - One `NewServer(root, "test")` + one `ServeStdio` call over three newline-delimited requests: `initialize`; `tools/call` `"todo.append"` with id `"panic-1"`; `tools/call` `"runtime.info"` with id `"after-1"`.
   - Assert the `"panic-1"` response has a top-level `error` field (not `result`) with code `-32000`.
   - Assert the `"after-1"` response is a normal successful tool result — proving the goroutine/process kept serving after the panic.
   - Assert `filepath.Join(os.Getenv("WS_CACHE_HOME"), "crash", "mcp-panic.log")` exists and its contents contain `"event":"request.panic"` and the literal string `deliberate test panic: todo.append`.

## Verification Plan
- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestServeStdioRecoversPanicAndPersistsCrashTrace -v` (name the new test accordingly)
- `cd agents-plugin-tool && go test ./...` (full suite, to catch any regression from the new defer/hook/import)

## Escalations
- None.
