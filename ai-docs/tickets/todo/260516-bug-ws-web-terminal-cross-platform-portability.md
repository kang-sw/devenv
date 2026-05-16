---
title: ws web terminal cross-platform portability
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-bug-ws-web-terminal-websocket-transport: current terminal transport recovery exposed POSIX-heavy browser and route test assumptions
  260427-chore-claude-dash-windows: prior Windows PTY/dashboard surface motivation
related-mental-model:
  - ws-web-dashboard
---

# ws web terminal cross-platform portability

## Background

The dashboard terminal substrate uses `portable_pty` and has a minimal
platform-aware shell selector, but the current verification surface is still
POSIX/macOS/Linux-centered. The daemon spawns `%COMSPEC%` or `cmd.exe` on
Windows and `$SHELL` or `/bin/sh` elsewhere, yet backend route tests and browser
acceptance steps send POSIX shell commands and assume POSIX utilities such as
`printf`, `seq`, `sed`, `stty`, `awk`, and shell arithmetic.

This creates a false sense of cross-platform support: the production PTY layer
is partially portable, while the test and evidence path may fail or fail to
exercise equivalent behavior on native Windows. Future dashboard terminal work
must either avoid POSIX-only assumptions or record explicit OS-scoped
limitations.

## Decisions

- Treat cross-platform behavior as part of the dashboard terminal contract, not
  as a best-effort test afterthought.
- Keep `portable_pty` as the process/PTY abstraction unless implementation
  evidence shows a concrete blocker.
- Shell selection should be explicit, testable, and recoverable. Environment
  variables such as `$SHELL` and `%COMSPEC%` may remain inputs, but fallback
  behavior and unsupported shells must be covered.
- Browser and backend terminal tests should express intent through
  platform-aware command helpers or fixtures rather than embedding POSIX command
  strings directly in acceptance paths.
- If a behavior cannot be made equivalent on one platform, record the exact
  platform and shell constraint instead of silently passing a narrower gate.

## Constraints

- Preserve daemon-owned terminal lifecycle, owner authentication, opaque
  terminal ids, WebSocket transport behavior, and close-as-terminate semantics.
- Do not turn this ticket into a root picker, file explorer, agent UI, or
  general terminal-feature redesign.
- Do not require optional third-party TUI tools such as `btop` for automated
  acceptance; deterministic built-in shell or PTY fixtures are preferred.
- Keep WSL and linked-server future direction in mind, but native Windows
  behavior must be honestly represented rather than hidden behind WSL-only
  assumptions.

## Phases

### Phase 1: Make shell selection explicit and testable

Extract or expose the dashboard terminal shell-selection behavior enough for
focused tests. Cover `$SHELL`, `%COMSPEC%`, fallback `/bin/sh`, fallback
`cmd.exe`, invalid or missing environment values where practical, and the cwd
used for spawned terminals.

Success means the code and tests make it clear which shell is selected on
Unix/macOS/Linux and Windows, and failures produce recoverable diagnostics
rather than opaque PTY spawn behavior.

### Phase 2: Replace POSIX-only terminal test commands

Audit backend route tests and browser acceptance tests for shell command strings
that assume POSIX syntax or utilities. Replace them with platform-aware helper
commands, daemon-side fixtures, or equivalent per-shell command builders.

Success means tests can state the terminal behavior being exercised without
hardcoding POSIX-only syntax in shared acceptance paths. Any behavior that
remains POSIX-only must be labeled with an explicit OS or shell guard.

### Phase 3: Harden browser harness platform behavior

Review daemon-served Playwright harness startup and shutdown behavior for
platform-sensitive assumptions such as executable suffixes and signal handling.
Make the harness launch/stop behavior work on supported platforms or skip with
clear platform-scoped diagnostics.

Success means `npm run test:browser` either runs against the dashboard daemon on
the platform under test or fails/skips with an explicit reason that does not
masquerade as product behavior.

### Phase 4: Record cross-platform terminal evidence

Run and record terminal portability evidence for the supported local
environments available during implementation, including macOS/Linux and native
Windows when available. The artifact should identify the shell, platform,
commands used, browser gate result, and any residual OS-scoped limitations.

Success means future dashboard terminal work has a durable reference for what
is known portable, what is WSL-only, and what remains native-Windows risk.
