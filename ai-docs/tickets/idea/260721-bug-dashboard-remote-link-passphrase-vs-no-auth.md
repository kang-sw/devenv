---
title: "Manually-linked remote server keeps showing \"auth required\" even when both gateway and remote run with --no-auth"
---

# Manually-linked remote server keeps showing "auth required" even when both gateway and remote run with --no-auth

## Symptom

Dogfooding a manually-linked remote `ws-dashboard` server: the dashboard
keeps showing the server as "auth required" even though both the gateway
daemon and the remote daemon are started with `--no-auth`, and the remote
is otherwise healthy and reachable. Restarting either side does not clear
the state — restarting the gateway silently drops the link and restarting
the remote does not help either.

## Mechanism (source-confirmed, not hypothesis)

- The "auth required" status shown in the dashboard is derived purely from
  the gateway's **in-memory linked-server session cache**, never from a
  live probe of the remote:
  `crates/daemon/src/servers.rs:217-225` — `dashboard_servers` builds
  status from `state.linked_server_sessions.contains(...)`.
  `crates/daemon/src/servers.rs:2171-2179` — `server_status` defaults a
  manual linked server that only has an `endpoint_hint` to `AuthRequired`
  whenever `connected` is false, unconditionally (i.e. "no session cached"
  reads as "needs a passphrase", not as "state unknown" or "unreachable").
- Linking a remote requires a **link passphrase generated fresh on every
  daemon start**, and this generation/verification path is entirely
  independent of `--no-auth`:
  `crates/daemon/src/auth.rs:66-98` — `OwnerAuthState::new_ephemeral`
  unconditionally generates `link_passphrase = LinkPassphrase(random_secret())`;
  `exchange_link_passphrase` does a strict compare with no `--no-auth`
  bypass anywhere in the path.
  `crates/daemon/src/server.rs:60-72` — `--no-auth` / `owner_auth_enabled`
  only gates the *direct_dashboard_url* startup message; the link
  passphrase line is printed unconditionally regardless of `--no-auth`.
- The "auth required" UI string maps to the `authRequired` status /
  `enterPassphrase` action: `frontend/src/App.tsx:1593` and
  `crates/daemon/src/servers.rs:2188-2192,2216`.

## Consequence

Link sessions are in-memory only (not persisted), and the passphrase is
per-process-start:

- Restarting the **gateway** silently drops all remote link sessions, with
  no auto-reconnect and no clear prompt distinguishing "link dropped,
  please re-link" from a generic auth failure. The user must manually
  re-enter the *current* passphrase via the UI.
- Restarting the **remote** rotates its passphrase, invalidating any prior
  session even if the gateway still remembers being linked.
- So neither side's restart is self-healing, and `--no-auth` on both ends
  gives no relief at all — the "auth required" wording implies the remote
  is reachable-but-rejecting credentials, when the real state is often
  just "no cached session," which is misleading during dogfooding.

(No passphrase value is recorded here — it is an ephemeral local-dev
secret regenerated per process start.)

## Candidate directions (undecided — do not implement from this ticket)

- (a) Let `--no-auth` (or a loopback/127.0.0.1-only remote) bypass or
  auto-accept the link passphrase exchange.
- (b) Allow pinning a stable link passphrase via flag/env so it survives
  restarts, as a dev-QoL escape hatch.
- (c) Persist link sessions and/or add auto-reconnect with a clear
  reconnect prompt, so a gateway restart doesn't silently orphan links.
- (d) At minimum, improve the UI wording to distinguish "needs a
  passphrase" (no session cached) from "unreachable" (actual connect
  failure), since today both read as the same "auth required" state.

## Provenance

Reported via live dogfooding on 2026-07-21; the mechanism above is
source-confirmed against `crates/daemon/src/servers.rs`,
`crates/daemon/src/auth.rs`, `crates/daemon/src/server.rs`, and
`frontend/src/App.tsx` — this is beyond hypothesis, but the fix direction
is intentionally left open for triage.
