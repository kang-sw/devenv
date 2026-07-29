---
title: production terminal helper socket path has no guard against the macOS 104-byte sockaddr_un ceiling
related:
  260725-feat-dashboard-nav-row-two-line-open-state: found-during
---

# production terminal helper socket path has no guard against the macOS 104-byte sockaddr_un ceiling

## Background

`crates/daemon/src/terminal.rs::default_registry_dir()` builds the terminal
helper's IPC socket path as `<state_dir>/terminals/<terminal_id>.sock`, where
`state_dir` resolves from `WS_DASHBOARD_STATE_HOME` (or `XDG_STATE_HOME` /
`$HOME/.local/state/ws-dashboard` / Windows `LOCALAPPDATA`, see
`persistent_state.rs::default_state_file`) and the terminal id is `term_` +
18 random alphanumeric chars (`terminal.rs:816`, id generator
`terminal.rs:~1930-1935`).

macOS caps `sockaddr_un.sun_path` at 104 bytes (Linux: 108). The generated
filename alone consumes 28 bytes (`term_` + 18 chars + `.sock`), plus 11
bytes for the `/terminals/` segment — so any `state_dir` longer than roughly
65 bytes pushes the full socket path over the macOS ceiling and
`IpcListener::bind` fails with `ENAMETOOLONG` inside the detached helper
process. The daemon then observes a generic HTTP 400 `{"error":"terminal
spawn failed"}` from `create_terminal`, with the underlying registry-entry
already written and the socket never bound (see the linked nav-row ticket's
Task A investigation for the mechanism, previously proven with an isolated
long-path vs. short-path daemon experiment).

This was found via the e2e browser-acceptance harness hitting the identical
failure through its own long macOS `$TMPDIR`-derived `WS_DASHBOARD_STATE_HOME`
(fixed for the test path only — see the harness fix in
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`). That harness fix
does **not** protect production: a real install whose `WS_DASHBOARD_STATE_HOME`
(or `$HOME`, feeding the default `~/.local/state/ws-dashboard/...` path) is
long enough will hit the same 104-byte failure, with no user-visible cause
beyond "New terminal" silently doing nothing (see the companion ticket on the
invisible-failure UI gap).

On this host the default state dir
(`~/.local/state/ws-dashboard/terminals/...`) is comfortably under the
budget, so this is **latent, not currently biting** — but it is unguarded:
nothing detects or reports the failure mode, and nothing prevents a longer
`$HOME`, a longer `WS_DASHBOARD_STATE_HOME` override, or a differently-named
username/hostname from tripping it.

## Constraints

- macOS: 104-byte `sockaddr_un.sun_path` ceiling. Linux: 108 bytes, with no
  equivalent `$TMPDIR`-length headroom problem — do not widen any fix to
  Linux without a separate justification.
- Socket filename floor: 28 bytes (`term_<18 chars>.sock`) is fixed by the
  current terminal-id generator; the `/terminals/` segment adds 11 more.

## Decisions

Two candidate directions were identified; **not decided here** — this is a
contract decision for the owner:

1. Shorten the generated terminal id used for the socket filename only
   (keep the longer id for any user/log-facing identity, if one exists).
2. Add a macOS-aware short-path fallback for the socket directory itself
   (e.g. bind under a short deterministic path keyed by a hash, independent
   of `WS_DASHBOARD_STATE_HOME`'s length), mirroring the `/tmp` fallback
   already used by the Rust test fixtures
   (`crates/daemon/tests/terminal_lifetime.rs::temp_fixture_path`,
   `crates/daemon/tests/routes.rs::terminal_registry_temp_dir`).

## Phases

### Phase 1: Decide and implement a production guard

Pick one of the two directions above (or a third), implement it, and add a
regression test that binds a socket under a deliberately long macOS state
path to prove the fix holds. Consider also making the `create_terminal`
failure path distinguish "socket bind failed due to path length" from other
spawn failures, so a future occurrence surfaces a diagnosable error instead
of a generic 400.
