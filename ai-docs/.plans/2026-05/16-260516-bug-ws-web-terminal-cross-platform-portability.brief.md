# Brief: 260516-bug-ws-web-terminal-cross-platform-portability

## Intent

Make the ws dashboard terminal verification surface honestly cross-platform.
The result should support deterministic daemon endpoints for browser tests,
explicit shell selection across Unix and native Windows, platform-aware command
helpers instead of shared POSIX command strings, and durable portability
evidence that distinguishes local POSIX coverage from native-Windows coverage.

## Scope Boundary

Implement the full ready-ticket slice: Phase 1 through Phase 5. This includes
the deterministic daemon endpoint harness, shell-selection testability,
platform-aware command helpers, browser harness platform hardening, and
cross-platform terminal evidence. Preserve the existing dashboard terminal
transport and UI behavior except where tests/harnesses must stop making
POSIX-only assumptions.

## Caller-Visible Contract

`npm run test:browser` still exercises the daemon-served production frontend
after owner pairing, but the harness can now target either a locally spawned
daemon or an already-running fixed endpoint. Spawned mode accepts explicit
host, port, bind mode, daemon binary, static directory, and readiness timeout.
External mode attaches to a provided base or pairing URL and does not own a
daemon child process.

The harness must wait for a real readiness signal before browser assertions.
Failures should identify whether the failure came from daemon startup, port
conflict, unreachable forwarded endpoint, missing/invalid pairing URL, owner
pairing, or browser assertions.

Terminal shell selection must be explicit and testable. Unix-like platforms use
the configured shell or documented `/bin/sh` fallback. Native Windows uses
`%COMSPEC%` or documented `cmd.exe` fallback.

Terminal backend and browser tests must express terminal intent through
platform-aware helpers or fixtures. Shared portable paths must not embed POSIX
utilities or shell syntax such as `printf`, `seq`, `sed`, `stty`, `awk`, or
shell arithmetic without an explicit platform guard.

Portability evidence must identify OS, shell profile, daemon endpoint mode,
readiness signal, browser gate result, commands/fixtures used, forwarding path
when used, and residual OS-scoped limitations. If the machine-local Windows SSH
host is unavailable, record that as an evidence gap rather than silently
treating local POSIX evidence as native-Windows coverage.

## Implementation Strategy Decisions

- Keep the product daemon CLI contract: use existing `ws-dashboard serve
  --host --bind-mode --port --static-dir` rather than adding a parallel command.
- Treat fixed-port remote Windows testing as a harness/evidence capability, not
  as a public multi-server bridge feature.
- Use the skeleton contracts established by `7e71449`; remove placeholder
  behavior and satisfy the existing contract tests instead of redesigning the
  surface.
- Keep exact private host details out of tracked source. The local Windows SSH
  host is available through ignored `ai-docs/_index.local.md`.
- Use deterministic built-in shell commands or PTY fixtures. Do not require
  optional tools such as `btop` for automated acceptance.

## Rejected Alternatives

- Do not continue relying on port `0` plus pairing scrape as the only browser
  harness mode; it is awkward for SSH forwarding and remote dogfood.
- Do not use short-lived SSH background process launch as the primary remote
  evidence path; probing showed the process can disappear when the setup
  command exits.
- Do not label POSIX-only browser commands as portable native-Windows evidence.
- Do not turn this ticket into root picker, file explorer, agent UI, or
  multi-server bridge work.

## Approach

- Finish the harness config surface in `frontend/e2e/daemonHarness.ts`:
  environment parsing, fixed spawned endpoints, external endpoint attach,
  readiness, Windows binary naming, and shutdown diagnostics.
- Implement `terminalCommandPlan` so browser tests ask for behavior by intent
  and receive shell-appropriate commands for Unix shell, `cmd.exe`, and
  PowerShell where practical.
- Replace POSIX-only command strings in daemon route tests and Playwright
  acceptance with the helper surfaces or explicit OS-guarded paths.
- Implement Rust terminal shell selection as a testable function and route
  spawn through it.
- Emit ignored machine-readable portability evidence from the browser gate and
  add a tracked dogfood summary under `ai-docs/.plans/2026-05/`.
- Run local gates and, when reachable, native-Windows fixed-port/SSH-forwarded
  evidence through the machine-local host.

## Constraints

- Preserve daemon-owned terminal lifecycle, owner authentication, opaque
  terminal ids, WebSocket transport, HTTP fallback/backfill, bounded resize,
  and close-as-terminate semantics.
- Keep browser UI visual behavior stable except for test harness and command
  portability changes.
- Keep private hostnames, usernames, paths, pairing tokens, and screenshots out
  of tracked source.
- Do not widen bind-mode security semantics; tunnel mode remains loopback
  oriented and owner-auth gated.
- Use ws named-agent workflow; implementation commits should stay on the
  current `implement/dashboard-terminal-cross-platform-portability` branch.

## Out of scope

- Public internet deployment, TLS, multi-user access, RBAC, or multi-server
  bridge features.
- Full terminal feature redesign, root picker redesign, file manager behavior,
  or agent controls.
- Guaranteeing Windows evidence when the machine-local host is unreachable;
  record an explicit evidence gap in that case.

## Details

Skeleton commits:

- Draft: `e7a7167`
- Final: `7e71449`
- Ticket skeleton record: `5b48da2`

Skeleton contract files:

- `ws-dashboard/frontend/e2e/daemonHarness.ts`
- `ws-dashboard/frontend/e2e/terminalPortabilityEvidence.ts`
- `ws-dashboard/frontend/src/terminalCommandPlan.ts`
- `ws-dashboard/frontend/src/terminalCommandPlan.test.ts`
- `ws-dashboard/frontend/tsconfig.route-tests.json`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`

The implementation may modify adjacent browser acceptance and test files as
needed to remove POSIX-only assumptions and record evidence.

## Verification Contract

Required local verification:

- `cd ws-dashboard && cargo test`
- `cd ws-dashboard/frontend && npm run test:terminals`
- `cd ws-dashboard/frontend && npm run test:browser`
- `cd ws-dashboard/frontend && npm run build`
- `git diff --check`

Required portability evidence:

- Local browser gate evidence must state OS/platform, shell profile, endpoint
  mode, readiness signal, and command profile.
- If the Windows SSH host in `ai-docs/_index.local.md` is reachable, run a
  native-Windows daemon foreground on fixed loopback port, expose it through
  SSH local forwarding, and drive the local browser/Playwright flow through the
  forwarded endpoint.
- If Windows evidence cannot run, record the exact blocker and mark the
  native-Windows result as an explicit gap.

## References

- [Must] `260516-ws-web-dashboard-terminal-deterministic-endpoint-harness` -
  fixed endpoint, external URL, remote Windows loopback harness contract.
- [Must] `260516-ws-web-dashboard-terminal-shell-selection-portability` -
  shell-selection behavior and diagnostics.
- [Must] `260516-ws-web-dashboard-terminal-platform-command-helpers` -
  portable command abstraction.
- [Must] `260516-ws-web-dashboard-terminal-cross-platform-evidence` -
  durable evidence requirements.
- [Must] `260516-ws-web-dashboard-browser-ui-acceptance-gate` - daemon-served
  browser gate after owner pairing.
- [Must] `260516-ws-web-dashboard-terminal-registry-pty-spawn` - preserve
  daemon-owned lifecycle and workRoot-scoped PTY semantics.
- [Must] `260516-ws-web-dashboard-terminal-io-transport` - preserve terminal
  auth, WebSocket/HTTP transport, input/output/resize behavior.
- [Must] `260516-ws-web-dashboard-browser-terminal-emulator-behavior` -
  browser assertion behavior for shell editing/control keys.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard terminal domain
  rules, coupling, common mistakes, and cross-platform terminal contract.
