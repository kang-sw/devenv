---
title: Pi bridge shell tools may orphan the real Git Bash child on Windows timeout or abort
related:
  260928-bug-config-tune-value-schema-untyped: same downstream Windows report
---

# Pi bridge shell tools may orphan the real Git Bash child on Windows timeout or abort

## Background

`8bbf49334` made the Pi bridge's three shell tools (`ws-worker-exec`, the
`ws-execute` pre-command, and the lead one-liner exec tool in
`agents-plugin-pi/src/execute-gateway.ts`) resolve their shell through the
host pi's `getShellConfig` instead of a hardcoded `sh`, so they now run on
native Windows through Git Bash.

The pre-ship review of `d59e2e71e..4c32c1d2b` found, by reasoning from the
host source (pi-coding-agent 0.84.4 `dist/core/exec.js`,
`dist/utils/shell.js`, `dist/core/tools/bash.js`) and not by a Windows run:

- `pi.exec` stops a timed-out or aborted child with `proc.kill("SIGTERM")`
  only; it sets neither `windowsHide` nor a process-tree kill.
- Git for Windows' `...\Git\bin\bash.exe`, which `getShellConfig` resolves
  first, is a launcher that starts `...\Git\usr\bin\bash.exe`. Killing the
  launcher may leave the real shell and the user's command running, for
  example after the one-liner's 30 s timeout.
- The host's own `bash` tool avoids this by killing the process tree
  (`killProcessTree`, `taskkill /T`).
- When Git is not under `ProgramFiles`, `getShellConfig` runs a synchronous
  `where bash.exe` (up to 5 s) on every call; the host `bash` tool shares
  this cost.

Not a regression: before `8bbf49334` these tools could not run anything on
native Windows. The gap is newly reachable.

## Open Questions

- Confirm on native Windows whether a timed-out one-liner (for example
  `sleep 60`) leaves `usr\bin\bash.exe` or its child running.
- If confirmed, choose between spawning through the host's tree-killing
  process helpers instead of `pi.exec`, resolving `usr\bin\bash.exe` directly,
  or asking upstream pi for tree-kill semantics in `pi.exec`.
- Whether to cache the resolved shell per session to avoid the repeated
  `where bash.exe` lookup.
