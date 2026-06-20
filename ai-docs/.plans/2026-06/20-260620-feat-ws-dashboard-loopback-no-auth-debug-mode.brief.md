# Brief: 260620-feat-ws-dashboard-loopback-no-auth-debug-mode

## Intent

Add an explicit local debug serving profile for the ws dashboard daemon so WSL
dogfood can open the dashboard from a browser without completing the pairing
flow. The debug profile is opt-in, loopback-only, and must preserve the normal
owner-authenticated default.

## Scope Boundary

Implement only Phase 1 of
`260620-feat-ws-dashboard-loopback-no-auth-debug-mode`: wire
`ws-dashboard serve --no-auth` through CLI parsing, `ServeConfig`, bind guard
validation, router construction, startup reporting, and focused tests.

Out of scope: Codex app-server integration, OpenCode integration, dashboard
Activity source changes, remote/public unauthenticated serving, new auth models,
and frontend UI changes.

## Caller-Visible Contract

- `ws-dashboard serve` remains owner-authenticated by default.
- `ws-dashboard serve --no-auth` starts a no-owner-auth debug profile only when
  the resolved bind host is loopback and `--bind-mode` is not `public`.
- `--no-auth --host 0.0.0.0`, another non-loopback host, or `--bind-mode public`
  must fail before the server listens.
- In no-auth mode, protected HTTP routes and WebSocket upgrade routes are
  reachable without owner cookie or bearer auth because loopback reachability
  has already been enforced.
- The bypass is centralized at router construction: route handlers remain
  unaware of the debug profile.
- Startup output clearly says no-auth debug mode is active and prints a direct
  dashboard URL for the bound address. Pairing may still exist internally, but
  `/pair` must not be required in no-auth mode.

## Contract Instructions

Files/modules to change:

- `ws-dashboard/crates/daemon/src/cli.rs`
- `ws-dashboard/crates/daemon/src/config.rs`
- `ws-dashboard/crates/daemon/src/router.rs`
- `ws-dashboard/crates/daemon/src/server.rs`
- `ws-dashboard/crates/daemon/tests/server.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`

Required implementation shape:

- Add `ServeArgs.no_auth: bool` with a clear `--no-auth` help string.
- Set `ServeConfig.owner_auth_enabled` from `!args.no_auth`.
- Call `validate_bind_guard(args.bind_mode, ip, owner_auth_enabled)` from
  `ServeConfig::from_args`.
- Extend `validate_bind_guard` so disabled owner auth is accepted only for
  loopback hosts and rejected for `BindMode::Public` or non-loopback hosts.
- Build protected routes once and layer `require_owner_auth` only when
  `state.config.owner_auth_enabled` is true. Do not add route-level
  conditionals.
- Preserve `/pair` and `/api/dashboard/link-auth` placement outside the
  protected router. This ticket does not change daemon-to-daemon link auth.
- Startup reporting may add fields to `StartupInfo`; keep existing pairing URL
  tests meaningful and avoid leaking tokens through new diagnostics.

Forbidden shortcuts:

- Do not allow unauthenticated `0.0.0.0` or other non-loopback serving.
- Do not use per-route `if no_auth` branches.
- Do not remove bearer auth or pairing behavior from normal owner-auth mode.
- Do not start a separate unauthenticated static-file router.

## Integration Test Instructions

Extend the existing daemon Rust tests.

Test files:

- `ws-dashboard/crates/daemon/tests/server.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`
- `ws-dashboard/crates/daemon/src/cli.rs` unit tests if needed for help text.

Required assertions:

- CLI help includes `--no-auth`.
- Default `ServeConfig`/default parsed serve args keep `owner_auth_enabled`.
- Loopback no-auth parses and sets `owner_auth_enabled = false`.
- `--no-auth` rejects `--bind-mode public`.
- `--no-auth` rejects non-loopback hosts such as `0.0.0.0`.
- Protected route access without cookie/bearer succeeds in no-auth mode.
- Existing paired-cookie and bearer-auth route tests still pass in normal mode.
- Startup info/reporting has a direct local dashboard URL for no-auth mode.

Run:

```text
cargo test -p ws-dashboard-daemon
```

Use `cd ws-dashboard` before running the command.

## Implementation Strategy Decisions

- Treat no-auth as a debug serving profile, not as an authorization role.
- Keep bind reachability and browser authorization separate: loopback-only is
  the prerequisite that permits omitting auth middleware for this profile.
- Centralize the behavior in config/router/server startup; handlers and
  frontend code should not know the profile exists.

## Rejected Alternatives

- Public unauthenticated serving is rejected because protected dashboard routes
  expose host-control operations.
- Per-route bypasses are rejected because they spread auth decisions into
  handlers and make future route additions unsafe.
- A new frontend or pairing-flow UI is rejected for this slice; the purpose is
  faster local/WSL debug iteration.

## Approach

- Add the CLI flag and config wiring first.
- Tighten bind guard behavior around disabled owner auth.
- Gate the existing owner-auth middleware application at router construction.
- Add/adjust startup reporting for direct no-auth URL.
- Add focused tests around config, route auth, and startup reporting.
- Run daemon tests and fix introduced warnings or failures.

## Constraints

- Normal mode security invariant must hold: every HTTP and WebSocket route other
  than pairing and documented link auth remains owner-authenticated before
  handler execution.
- No-auth route reachability is acceptable only after loopback bind validation.
- Startup URLs must be built after listener binding so port `0` is resolved.
- Keep diagnostics bounded; do not leak pairing tokens through new direct URL
  output.

## Out of scope

- Browser visual changes.
- Public deployment support.
- Windows-native daemon changes.
- Provider activity adapter implementation.

## Details

Existing seams:

- `ServeConfig.owner_auth_enabled` already exists.
- `validate_bind_guard` already takes `owner_auth_enabled`.
- `router.rs` currently applies `require_owner_auth` to the entire protected
  router in one layer.
- `server.rs` currently builds startup info after binding and prints pairing
  URL plus remote link passphrase.

No-auth startup should have a token-free direct dashboard URL such as
`http://127.0.0.1:<bound-port>/`. If the bound address is IPv6, keep the same
bracketed display behavior as the pairing URL helper.

## Verification Contract

Acceptance requires a clean daemon Rust test run:

```text
cd ws-dashboard
cargo test -p ws-dashboard-daemon
```

If WSL manual dogfood is possible after tests pass, start the daemon with:

```text
cd ws-dashboard
cargo run -p ws-dashboard-daemon -- serve --no-auth --host 127.0.0.1 --port 0 --static-dir frontend/dist
```

Report the printed direct dashboard URL so the user can try it from the Windows
browser. Do not bind `0.0.0.0` for this check.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard auth, route,
  bind, and startup invariants.
- [Must] `ai-docs/tickets/ready/260620-feat-ws-dashboard-loopback-no-auth-debug-mode.md`
  - selected Phase 1 contract.
