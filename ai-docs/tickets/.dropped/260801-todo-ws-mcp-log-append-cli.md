---
title: "ws-mcp: add `log append` CLI subcommand for external warning logging"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260731-research-ws-opencode-drop-in-package: research ticket whose goal-loop guard needs this logging surface
sage-review-design: required
status: dropped (2026-08-02)
---

> **Dropped (2026-08-02):** The opencode adapter (`260801-feat-ws-opencode-adapter`,
> also dropped) was the sole consumer of this CLI. The Pi-native framework
> (`260802-research-ws-pi-native-framework`) will re-evaluate its logging
> needs in the expansion phase (goal-loop guard warnings), at which point a
> fresh harness-neutral `log append` ticket can be opened if the Pi bridge
> still wants an external-process → ws-mcp diagnostic write surface. The
> ws-mcp logging sinks investigated here (in-memory ring, crash file,
> lifecycle file) remain valid reference material.

# ws-mcp: add `log append` CLI subcommand for external warning logging

## Background

The opencode drop-in plugin design (research ticket
`260731-research-ws-opencode-drop-in-package`) needs a way for the opencode
TS plugin — a separate process from the ws-mcp server — to emit diagnostic
warnings (e.g. "goal-loop guard threshold exceeded: 5 re-injections in
10min, forcing next-step fallback") into the ws-mcp diagnostic surface.

Investigation of the existing ws-mcp logging infrastructure
(`agents-plugin-tool/internal/mcp/server.go`, `agents-plugin-tool/cmd/ws-mcp/main.go`)
found three log sinks, all **server-internal** (written by the ws-mcp
process itself):

1. **In-memory debug ring** (`appendDebugEvent`, `server.go:263`, capped at
   256 events) — exposed read-only via the `runtime.debug_events` MCP tool
   and mirrored to `WS_MCP_DEBUG_LOG` if that env var is set.
2. **Crash file** (`<cache-root>/crash/mcp-panic.log`, `recordPanic`,
   `server.go:332`) — always-on append-only JSONL.
3. **Lifecycle file** (`<cache-root>/crash/mcp-lifecycle.log`,
   `RecordLifecycleEvent`, `server.go:375`) — always-on append-only JSONL.

There is **no** `ws-mcp log` CLI subcommand and **no** `log.*` MCP tool.
All three sinks are written by the ws-mcp server process; no external
caller can append.

This ticket adds a new `log append` CLI subcommand backed by a new
`RecordExternalLogEvent` function, modeled on the existing
`RecordLifecycleEvent` pattern.

## Decisions

- **New top-level CLI subcommand `log`** in `cmd/ws-mcp/main.go`, with
  sub-verb `append`. Usage: `ws-mcp log append <level> <message>`.
  - `<level>`: `warning` | `info` | `error` (validated; unknown levels
    rejected with exit 2).
  - `<message>`: free-text string (keep bounded — see concurrency note).
- **Backend: new `RecordExternalLogEvent`** in `internal/mcp/server.go`,
  sibling of `RecordLifecycleEvent` (`server.go:375`). Writes one JSONL
  line to `<cache-root>/crash/mcp-external.log` (append-only,
  `O_CREATE|O_WRONLY|O_APPEND`, no lock — matching existing
  `recordPanic`/`RecordLifecycleEvent` precedent at `server.go:286,348`).
  Falls back to `os.Stderr` on write failure. Also calls
  `appendDebugEvent("external.<level>", fields)` for in-memory ring
  visibility (dual-write pattern, same as `recordPanic`).
- **Register in `runtimeCapabilityCommandNames`** (`main.go:203`): add
  `"log.append"` so `runtime capabilities` publishes it. Do **not** add to
  `filterNoAgentCommands` (`main.go:254`) — `log.append` is agentless and
  should be available in all modes.
- **Update `usage()`** (`main.go:70`) to include the `log` subcommand.

## Phases

### Phase 1: Implement `log append` CLI + `RecordExternalLogEvent` backend

- Add `func RecordExternalLogEvent(level, msg string, fields map[string]any)`
  in `internal/mcp/server.go` alongside `RecordLifecycleEvent` (`server.go:375`).
  Reuse `crashDir()` (`server.go:314`) for the directory. Write to
  `mcp-external.log`. JSON shape: `{"ts": "<RFC3339>", "level": "<level>",
  "msg": "<msg>", "fields": {...}}`.
- Add `func logCommand(args []string)` and `func logAppend(args []string)` in
  `cmd/ws-mcp/main.go`, modeled on `configCommand` (`main.go:272`) /
  `agentsDebug` (`main.go:1342`) flag-parsing pattern.
- Add `case "log": logCommand(os.Args[2:])` to the dispatch switch
  (`main.go:32-67`).
- Add `"log.append"` to `runtimeCapabilityCommandNames()` (`main.go:203`).
- Update `usage()` (`main.go:70`).

### Phase 2: Tests

- Unit test `RecordExternalLogEvent` in `internal/mcp/` — verify JSONL
  shape, append behavior, stderr fallback on unwritable path.
- CLI test in `cmd/ws-mcp/main_test.go` — verify `ws-mcp log append warning
  "test"` writes one line to `mcp-external.log`, exit 0 on success, exit 2
  on invalid level.
- Verify `runtime capabilities` output includes `log.append`.

### Result

_(to be filled after implementation)_
