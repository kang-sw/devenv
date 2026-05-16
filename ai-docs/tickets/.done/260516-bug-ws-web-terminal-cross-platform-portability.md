---
title: ws web terminal cross-platform portability
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-bug-ws-web-terminal-websocket-transport: current terminal transport recovery exposed POSIX-heavy browser and route test assumptions
  260427-chore-claude-dash-windows: prior Windows PTY/dashboard surface motivation
spec:
  - 260516-ws-web-dashboard-terminal-deterministic-endpoint-harness
  - 260516-ws-web-dashboard-terminal-shell-selection-portability
  - 260516-ws-web-dashboard-terminal-platform-command-helpers
  - 260516-ws-web-dashboard-terminal-cross-platform-evidence
skeletons:
  phase-1: 7e71449
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

Pre-implementation probing on 2026-05-16 confirmed that the machine-local
Windows SSH host recorded in `ai-docs/_index.local.md` runs Windows 11 with
PowerShell 5.1, exposes `cmd.exe` through `%COMSPEC%`, and has `git`,
Rust/Cargo, Node/npm, and Python 3.12 installed. A foreground remote loopback
server on fixed port `47173` was reachable from the local machine through:

```text
ssh -L 47173:127.0.0.1:47173 <windows-ssh-host>
```

The same probe also showed that a background server launched through a short
SSH command is not a reliable evidence path: the process may disappear when the
SSH session exits or before readiness is observed. Remote dashboard evidence
should therefore keep the remote daemon in a foreground SSH session or another
explicitly supervised process and prove readiness before the browser gate
attaches.

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
- Cross-platform browser evidence should use a deterministic endpoint path:
  fixed daemon port, foreground remote daemon, SSH local forwarding, and a local
  browser or Playwright client attached to the forwarded localhost endpoint.
- Readiness is part of the contract. Process spawn alone is insufficient; the
  harness must wait for a real pairing URL, health route, or equivalent HTTP
  reachability signal before starting browser assertions.
- If a behavior cannot be made equivalent on one platform, record the exact
  platform and shell constraint instead of silently passing a narrower gate.

## Constraints

- Preserve daemon-owned terminal lifecycle, owner authentication, opaque
  terminal ids, WebSocket transport behavior, and close-as-terminate semantics.
- Do not turn this ticket into a root picker, file explorer, agent UI, or
  general terminal-feature redesign.
- Do not require optional third-party TUI tools such as `btop` for automated
  acceptance; deterministic built-in shell or PTY fixtures are preferred.
- Use the Windows SSH host recorded in `ai-docs/_index.local.md` only as
  machine-local evidence. Do not commit secrets, private host paths, or
  assumptions that make the public repository depend on that exact host being
  available.
- Keep WSL and linked-server future direction in mind, but native Windows
  behavior must be honestly represented rather than hidden behind WSL-only
  assumptions.

## Phases

### Phase 1: Add deterministic daemon endpoint harness

Make the daemon-served browser harness configurable enough to run against a
deterministic endpoint instead of always spawning `target/debug/ws-dashboard` on
port `0`. The harness should accept or derive host, port, bind mode, daemon
binary path, static directory, and an externally supplied base or pairing URL.
It must handle native Windows executable naming such as `ws-dashboard.exe` and
should keep the product CLI's existing `--host`, `--bind-mode`, `--port`, and
`--static-dir` contract rather than adding a parallel dashboard command.

The remote evidence path is:

```text
ws-dashboard.exe serve --host 127.0.0.1 --bind-mode tunnel --port <fixed-port> --static-dir <dist>
ssh -L <fixed-port>:127.0.0.1:<fixed-port> <windows-ssh-host>
```

The remote daemon should run in the foreground or under an explicitly
supervised process so its lifetime is not silently tied to a completed setup
command. The local browser or Playwright gate then attaches through
`http://127.0.0.1:<fixed-port>`.

Success means the browser gate can target either a locally spawned daemon or an
already-running fixed endpoint, waits for a real readiness signal before
assertions, and emits clear diagnostics for port conflicts, missing pairing
URLs, unreachable forwarded endpoints, and daemon early exit.

### Result (ddd47673) - 2026-05-17

Implemented spawned and external daemon harness modes with deterministic host,
bind mode, port, daemon binary, static directory, readiness timeout, base URL,
and pairing URL configuration. The browser harness now verifies readiness after
pairing, supports external forwarded endpoints, preserves the existing product
CLI contract, and redacts private endpoint and token details from readiness and
startup diagnostics.

#### Edition (de36231) - 2026-05-17

Added `WS_DASHBOARD_TEST_WORKROOT` so external browser gates can open a fixture
path that exists on the daemon host instead of using a temporary path from the
local Playwright host. This is required for SSH-forwarded native-Windows daemon
evidence where Playwright runs locally but the daemon opens Windows paths.

### Phase 2: Make shell selection explicit and testable

Extract or expose the dashboard terminal shell-selection behavior enough for
focused tests. Cover `$SHELL`, `%COMSPEC%`, fallback `/bin/sh`, fallback
`cmd.exe`, invalid or missing environment values where practical, and the cwd
used for spawned terminals.

Success means the code and tests make it clear which shell is selected on
Unix/macOS/Linux and Windows, and failures produce recoverable diagnostics
rather than opaque PTY spawn behavior.

### Result (ddd47673) - 2026-05-17

Implemented testable terminal shell selection for Unix and Windows profiles.
The daemon covers `$SHELL`, `%COMSPEC%`, and fallback behavior without requiring
the test host to match the target platform.

### Phase 3: Replace POSIX-only terminal test commands

Audit backend route tests and browser acceptance tests for shell command strings
that assume POSIX syntax or utilities. Replace them with platform-aware helper
commands, daemon-side fixtures, or equivalent per-shell command builders.

The shared acceptance path should state terminal intent such as echo,
line-editing, paste, clear-screen, interrupt, EOF, and resize behavior, then map
that intent to supported shell commands or PTY fixtures for Unix shells,
`cmd.exe`, and PowerShell where appropriate. POSIX utilities such as `printf`,
`seq`, `sed`, `stty`, `awk`, and shell arithmetic must not remain in shared
portable browser gates without an explicit platform guard.

Success means tests can state the terminal behavior being exercised without
hardcoding POSIX-only syntax in shared acceptance paths. Any behavior that
remains POSIX-only must be labeled with an explicit OS or shell guard and must
not be presented as native-Windows evidence.

### Result (ddd47673) - 2026-05-17

Replaced shared POSIX command strings in backend route tests and browser
acceptance paths with platform-aware command helpers for Unix shell, `cmd.exe`,
and PowerShell profiles. Browser acceptance now binds command generation to the
target shell profile rather than the local Playwright host when running against
an external daemon.

### Phase 4: Harden browser harness platform behavior

Review daemon-served Playwright harness startup and shutdown behavior for
platform-sensitive assumptions such as executable suffixes and signal handling.
Make the harness launch/stop behavior work on supported platforms or skip with
clear platform-scoped diagnostics.

This phase owns the local harness mechanics that Phase 1 exposes: path joining
for Windows binaries, process shutdown that does not assume POSIX-only signals,
foreground remote endpoint attachment, fixed-port conflict diagnostics, and
captured stdout/stderr that explain whether failure came from daemon startup,
SSH forwarding, owner pairing, or browser assertions.

Success means `npm run test:browser` either runs against the dashboard daemon on
the platform under test or fails/skips with an explicit reason that does not
masquerade as product behavior. A remote forwarded endpoint failure should name
the failing layer instead of only reporting a generic browser timeout.

### Result (ddd47673) - 2026-05-17

Hardened the Playwright daemon harness for Windows executable naming, external
endpoint attachment, post-pairing readiness, spawned-child cleanup on readiness
failure, and bounded diagnostic output. Added executable harness coverage and
wired it into `npm run test:terminals`.

### Phase 5: Record cross-platform terminal evidence

Run and record terminal portability evidence for the supported local
environments available during implementation. At minimum, record local
macOS/Linux evidence from the normal browser gate and native-Windows evidence
from the machine-local Windows SSH host when reachable. The Windows evidence
should use a foreground fixed-port daemon, SSH local forwarding, and a local
browser or Playwright client attached through the forwarded localhost endpoint.

The artifact should identify the OS, shell, daemon command, fixed port, whether
SSH forwarding was used, readiness signal, browser gate result, commands or
fixtures used, and any residual OS-scoped limitations. If the Windows host is
temporarily unavailable during implementation, record that as an explicit
evidence gap rather than silently treating the local POSIX gate as portable.

Success means future dashboard terminal work has a durable reference for what
is known portable, what is WSL-only, and what remains native-Windows risk.

### Result (ddd47673) - 2026-05-17

Recorded local POSIX browser-gate evidence as passing with a spawned daemon,
`unix-sh` command profile, pairing URL scrape, and `/healthz` readiness.
Attempted native-Windows fixed-endpoint evidence through the machine-local SSH
host; SSH and remote frontend build succeeded, but native daemon build was
blocked by an outdated remote Cargo toolchain. The Windows result is recorded
as an explicit evidence gap without private endpoint, user, host, path, token,
or screenshot details.

#### Edition (de36231) - 2026-05-17

Updated the remote Windows Cargo toolchain and retried the fixed-endpoint
evidence path. Native daemon build, fixed loopback serving, SSH local
forwarding, owner pairing, and daemon-host workRoot opening all succeeded. The
browser gate then reached a real `cmd.exe` terminal session but failed because
Ctrl-C did not interrupt the long-running command fixture; that remaining
native-Windows control-key gap is captured separately for follow-up.
