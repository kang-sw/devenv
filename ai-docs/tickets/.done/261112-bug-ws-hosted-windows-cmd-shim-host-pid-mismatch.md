---
title: "Hosted sessions on Windows: a .cmd shim makes the host PID cmd.exe, so the session is refused its key"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: hosted lead sessions
completed: 2026-11-13
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

## Decisions

- On Windows, wsagent resolves a `.cmd`/`.bat` npm cmd-shim before spawning:
  parse its `"%_prog%" "%dp0%\<script>" %*` line and run `<shim dir>\node.exe`
  (or `node` from PATH) with `<script>` and the original arguments, in one
  helper beside `lookPath` used by all three call sites including a
  user-set `Command`; a shim that does not parse refuses the start with a
  clear error. User board (27).
  Rejected: matching an ancestor PID in ws-mcp and the Pi extension (a nested
  harness would pass as hosted; Windows parent-PID lookups on two sides);
  looking up cmd.exe's child after the spawn (races the shim's startup).
- The shim line format comes from npm's `cmd-shim` generator; the
  implementation pins it with a fixture of a real generated shim and verifies
  on a Windows host before closing (user asked whether the pattern is proven).

## Constraints

- Verification needs a Windows host; see `ai-docs/manuals/windows-dogfood.md`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.


## Resolution (2026-11-13)

Fixed by a3795bf8a (merged 0e455291b): wsagent unwraps a Windows npm cmd-shim to node before spawning. Verified on this Windows host 2026-11-13 with the npm-installed Pi (`C:\Users\user\AppData\Roaming\npm\pi.cmd`): a dashboard-hosted Pi lead row (`refutablevarnishmultitudesnowsuit`) records `host_pid` 6548, which is `node.exe` running pi-coding-agent's `cli.js --mode rpc`, its ws-mcp launcher (python3.13.exe) is a direct child of that node, and no second row was registered for the session.
