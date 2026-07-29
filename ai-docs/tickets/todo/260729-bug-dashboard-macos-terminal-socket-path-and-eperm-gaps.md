---
title: macOS terminal helper — unguarded sun_path limit and unreclaimable EPERM entries
sage-review-design: required
---

# macOS terminal helper — unguarded sun_path limit and unreclaimable EPERM entries

## Background

Two gaps found by code review of PR #4's macOS terminal-platform port
(`goal/ws-dashboard-dev/velvet-arbor-quill`, merged as `1b41a37b`). The port
itself is sound — it fixed a state where macOS could never spawn a terminal at
all — but it leaves two platform-specific edges open.

### 1. `sun_path` 104-byte limit is diagnosed in tests but unguarded in production

`tests/terminal_lifetime.rs` documents the macOS `AF_UNIX` `sun_path` ceiling
(104 bytes, versus Linux's 108) and works around it by forcing `/tmp` on macOS.
Production has no equivalent guard: `default_registry_dir()` joins `terminals/`
onto `default_state_dir()`, and the socket is `registry_dir/{id}.sock` where `id`
is `term_` + 18 chars = 28 bytes with the extension.

Defaults fit — `$HOME/.local/state/ws-dashboard/terminals/…` is roughly 72 bytes
plus the username, and the `$TMPDIR` fallback about 88 — but
`WS_DASHBOARD_STATE_HOME`, `XDG_STATE_HOME` and `WS_DASHBOARD_STATE_FILE` are
user-supplied and unbounded.

Failure scenario: a macOS user sets a deep `WS_DASHBOARD_STATE_HOME`.
`IpcListener::bind` fails with `EINVAL` inside the detached helper.
`run_terminal_helper`'s `?` returns *before* its `delete_registry_entry`, leaving
a stale `.json` behind. The daemon then logs "helper wrote a registry entry but
the daemon could not connect or complete the handshake" — pointing at handshake
timing rather than a path-length limit. Every terminal fails, permanently, with
no actionable diagnostic.

Fix direction: length-check the socket path at construction against 104 on macOS
and 108 on Linux, and fail with a message that names the limit and the offending
path.

### 2. A cross-user `EPERM` deletes the registry entry while the helper lives on

`terminal_platform.rs` correctly documents that `proc_pidinfo` returns `EPERM`
for a cross-user pid — yielding `None`, hence `NoSuchProcess` — where Linux would
report `PidReused`, and calls this "harmless today because both map to
drop-only".

Drop-only is indeed safe from a mis-kill standpoint, but the entry is *deleted*
while the helper keeps running. With the periodic orphan sweep now landing (see
`260726-refactor-ws-dashboard-long-uptime-leak-hardening`), a helper that falls
into this branch becomes permanently unreclaimable: no registry entry means no
future sweep can ever find it.

Fix direction: at minimum, extend the CONTRACT to state the consequence rather
than stopping at "harmless". Better, distinguish "cannot determine" from "does
not exist" so the entry is retained (and retried) instead of dropped.

## Constraints

Neither gap is reproducible on Linux CI. Any test must either be
`#[cfg(target_os = "macos")]`-gated and honestly labelled as unexecuted
elsewhere, or restructured so the decision logic is testable without the
platform syscall.

## Phases

### Phase 1: Guard the socket path length

### Phase 2: Distinguish undeterminable identity from absent identity on macOS
