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
- Layer reached: SSH remote was reachable and frontend dependencies/build succeeded on the remote Windows host.
- Blocker: remote daemon build could not proceed because the installed Cargo toolchain is too old for the repository lockfile/dependency graph. The first build stopped on Cargo lockfile version 4; retrying without the lockfile stopped on a dependency requiring Cargo edition-2024 support. No daemon was started, so the fixed loopback endpoint, SSH local-forward, owner pairing, and browser assertions were not run.
- Private endpoint, user, host, paths, pairing URLs, screenshots, and tokens are intentionally omitted.

## Residual limitations

- Native-Windows terminal behavior is not yet evidenced in a browser gate until the remote Windows Rust toolchain is updated or a compatible Windows daemon binary is supplied.
- `cmd.exe` command plans intentionally record ANSI color and alternate-screen limitations rather than claiming equivalent behavior to Unix shells.
