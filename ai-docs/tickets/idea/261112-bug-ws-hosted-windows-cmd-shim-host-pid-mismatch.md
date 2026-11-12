---
title: "Hosted sessions on Windows: a .cmd shim makes the host PID cmd.exe, so the session is refused its key"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: hosted lead sessions
---

# Hosted sessions on Windows: a .cmd shim makes the host PID cmd.exe, so the session is refused its key

## Background

Review sweep `00fd2e5d9..62304312b` (ledger 940fc053e), finding P3, routed by
the peer session; traced in code on 2026-11-12, not run on a Windows host.

1. `lookPath("pi")` / `lookPath("claude")` return `pi.cmd` / `claude.cmd` for
   an npm install (`wsagent/pi.go:337/372`, `wsagent/claude.go:315`; Go's
   LookPath honours PATHEXT).
2. `holder.py` starts the child with `subprocess.Popen(argv)` (276-288);
   Windows runs a `.cmd` through cmd.exe, so the reported child PID is
   cmd.exe's and node is a grandchild.
3. `holderChild.start` -> `startLead` writes that PID to `rec.HostPID` and the
   row.
4. ws-mcp: `WS_MCP_HARNESS_PID` is the launcher's parent, node
   (`ws-mcp-launcher.py:1753`). `resolveHostedSession` (`process_env.go:281`)
   requires `host_pid` to equal it, so the session is not hosted;
   `hostedLoginGuard` (`hosted_login.go:151-155`) then sees the row's live
   process (cmd.exe) and refuses the key (`hosted-session-held`).
5. Pi: `rowVerdict` in `agents-plugin-pi` `hosted-session.ts:95-105` compares
   `hostPid === process.pid`, gets "other", and runs as an ordinary session,
   starting its own mailbox wait beside the daemon's delivery.

Claude's native installer yields `claude.exe`, which is unaffected.

## Open Decision Queue

Last ID: (27)

(27) [open] [technical] On Windows, wsagent resolves a `.cmd`/`.bat` npm cmd-shim before spawning (parse its `"%_prog%" "%dp0%\<script>" %*` line; run `<shim dir>\node.exe` or `node` from PATH with `<script>` and the original arguments; refuse the start with a clear error when the parse fails), in one helper beside `lookPath` used by all three call sites including a user-set `Command`, over matching an ancestor PID in ws-mcp and the Pi extension or looking up cmd.exe's child after spawn - an ancestor match lets a nested harness pass as hosted and needs Windows parent-PID lookups on two sides, a post-spawn lookup races the shim's startup, and resolving also keeps prompt arguments out of cmd.exe metacharacter handling. T0.

## Constraints

- Verification needs a Windows host; see `ai-docs/manuals/windows-dogfood.md`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
