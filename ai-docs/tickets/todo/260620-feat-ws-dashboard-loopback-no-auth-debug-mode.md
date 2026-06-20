---
title: ws dashboard loopback no-auth debug mode
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: WSL dashboard dogfood needs fast unauthenticated iteration before Codex app-server Activity smoke runs are practical
spec:
  - 260515-ws-web-daemon-foundation
  - 260516-ws-web-dashboard-protected-frontend-shell
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard loopback no-auth debug mode

## Background

Dashboard dogfood inside WSL needs a shorter debug loop than the current
pairing-token and owner-cookie flow. The bypass is for local iteration only: it
must not change the default security posture, weaken public serving guards, or
make route handlers individually responsible for deciding whether owner auth is
active.

The existing daemon already carries most of the intended seam. `ServeConfig`
has `owner_auth_enabled`, bind guard validation already rejects public bind mode
when owner auth is disabled, and the router applies owner auth through one
central middleware layer over the protected routes. The missing behavior is a
CLI-visible debug profile that wires those pieces together safely.

## Decisions

- Add `ws-dashboard serve --no-auth` as an explicit local debug flag. The
  default remains owner-authenticated serving.
- Restrict no-auth serving to loopback targets. `--no-auth` must fail when the
  requested bind host is non-loopback or when `--bind-mode public` is selected.
  WSL dogfood may verify Windows-to-WSL localhost forwarding, but the daemon
  must not listen unauthenticated on `0.0.0.0`.
- Implement the bypass at the central router/auth boundary. Do not add
  per-route `if no_auth` checks and do not split handler behavior by auth mode.
- Treat no-auth as a debug serving profile, not an authorization model. It may
  bypass cookie, bearer, Host/Origin, and WebSocket pre-upgrade auth checks only
  because loopback reachability is enforced first.
- Startup output should make the mode obvious to the owner and should expose a
  direct local dashboard URL for the current bound address. Pairing may remain
  available as an implementation detail, but using `/pair` must not be required
  in no-auth mode.

## Constraints

- Protected dashboard routes include file reads/writes, terminals, Git
  operations, document translation, linked-server operations, Activity, and
  static UI serving. No-auth makes all of those reachable to any local process
  that can connect to the daemon.
- The implementation must preserve the existing security invariant for normal
  mode: every HTTP and WebSocket route other than pairing and documented
  daemon-to-daemon link auth remains owner-authenticated before handler
  execution.
- The `dev.sh run` wrapper should continue forwarding arbitrary serve args, so
  local dogfood can use `./dev.sh run --no-auth --port <port>` after the flag is
  added.
- Browser acceptance tooling may use no-auth only as an explicit external or
  debug profile. Existing paired-auth acceptance coverage must remain valid.

## Phases

### Phase 1: Loopback-only no-auth serving profile

Wire `ws-dashboard serve --no-auth` through CLI parsing, `ServeConfig`,
bind-guard validation, router construction, startup reporting, and tests. The
router should either apply the existing owner-auth middleware unchanged or omit
it for the whole protected router when `owner_auth_enabled` is false; individual
routes should remain oblivious to the debug profile.

Verification boundary: Rust CLI/config tests must cover flag discoverability,
default auth-enabled behavior, loopback no-auth acceptance, public/non-loopback
no-auth rejection, and protected route access without cookie or bearer auth in
no-auth mode. Existing paired-auth and bearer-auth route tests must continue to
pass. A WSL dogfood note should verify that a daemon started inside WSL with
loopback no-auth can be reached through the intended local browser path without
requiring `0.0.0.0`.
