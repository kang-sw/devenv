# Launcher: Windows harness PID skips Claude Code's cmd.exe wrapper

## Problem

On Windows, Claude Code starts a stdio MCP server through a shell:
`cmd.exe /d /s /c "python3 \"...\\ws-mcp-launcher.py\" \"serve\" \"--stdio\""`.
`ws-mcp-launcher.py` sets `WS_MCP_HARNESS_PID = os.getppid()`, which is that
cmd.exe, not `claude.exe`. The dashboard daemon writes `claude.exe`'s PID as
the hosted row's `host_pid`, so `resolveHostedSession`
(`agents-plugin-tool/internal/mcp/process_env.go`) sees a different live PID
and silently serves an ordinary session: the server registers its own session
row (a duplicate beside the hosted row) and ferrule mints a fresh key instead
of `WS_HOSTED_SESSION_KEY`.

Observed 2026-11-13 on a live hosted session: hosted row `host_pid` 15808
(`claude.exe`), launcher parent 19404 (`cmd.exe /d /s /c "python3 ..."`,
whose parent is 15808); the ordinary row carried `harness_pid: 19404`.

## Outcome

- On Windows, when the launcher's parent process image is `cmd.exe`, the
  launcher reports cmd.exe's parent as `WS_MCP_HARNESS_PID`. Otherwise the
  value stays `os.getppid()`. Non-Windows behaviour is unchanged.
- Only a direct cmd.exe parent is skipped (one level); no general ancestor
  search, so a nested harness is not mistaken for the hosted one.
- Any lookup failure falls back to `os.getppid()`.
- Parent PID and image name come from the stdlib only (ctypes
  `CreateToolhelp32Snapshot` / `Process32FirstW` / `Process32NextW`, or an
  equivalent); the launcher takes no third-party dependency.
- `agents-plugin/bin/ws-mcp-launcher.py` and
  `agents-plugin-wsflow/bin/ws-mcp-launcher.py` stay byte-identical.

## Out of scope

- The parent-death watch in the launcher (it may keep watching the direct
  parent).
- The npm `.cmd` shim direction of
  `261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch` (hosted harness
  started through a shim); that ticket stays open.
- Go-side changes to `resolveHostedSession`.

## Verification

- A launcher unit test covering: Windows + cmd.exe parent -> grandparent;
  Windows + non-cmd parent -> parent; lookup failure -> parent; non-Windows
  -> parent (process-table lookup injected or monkeypatched).
- The existing launcher test files under `agents-plugin/tests/` and the
  wsflow package tests pass.
- On this Windows host, a direct call of the new helper from a
  `cmd.exe /c python ...` child returns cmd.exe's parent.
