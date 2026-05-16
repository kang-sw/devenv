# Dogfood Evidence: ws Web Terminal Cross-Platform Portability

## Local POSIX browser gate

- Date: 2026-05-17
- OS/platform: macOS / `darwin`
- Shell profile: `unix-sh`
- Daemon endpoint mode: spawned local daemon, dynamic loopback port
- Readiness signal: owner pairing URL scraped, then `/healthz` reached
- Browser gate result: pass
- Command profile: platform-aware terminal command plan (`unix-sh`)
- Fixtures covered: echo, edit/backspace/cursor/history, interrupt target, clear-and-echo, ANSI green, scroll lines, alternate-screen substitute, second terminal isolation
- Machine-readable evidence: ignored artifact under `ws-dashboard/frontend/e2e/.artifacts/terminal-portability-evidence.json`

## Native Windows fixed-endpoint / SSH-forward evidence

- Result: explicit gap
- Layer reached: SSH remote was reachable, Rust/Cargo was updated to a toolchain that supports the current lockfile/dependency graph, the native Windows daemon built, the fixed loopback daemon started, SSH local forwarding worked, owner pairing succeeded, and the browser gate opened a daemon-host workRoot fixture.
- Blocker: the browser gate reached a real `cmd.exe` terminal session, but Ctrl-C did not interrupt the long-running command fixture. The terminal stayed in the running `ping` command and the follow-up `CTRL-C-OK` echo was not observed. This is now captured as a separate Windows terminal control-key follow-up instead of a build/toolchain blocker.
- Private endpoint, user, host, paths, pairing URLs, screenshots, and tokens are intentionally omitted.

## Residual limitations

- Native-Windows terminal behavior is not yet evidenced as a passing browser gate until the Windows `cmd.exe` Ctrl-C/control-key behavior is fixed or explicitly scoped.
- `cmd.exe` command plans intentionally record ANSI color and alternate-screen limitations rather than claiming equivalent behavior to Unix shells.
