---
title: "ws-dashboard native-Windows daemon silently drops all persisted dashboard state (linked servers, opened work roots, root-picker pins) when HOME is unset"
related:
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: blocks the ticket's stated remote-Windows-daemon dogfood gap
  260707-chore-dashboard-linked-server-tunnel-dogfood-plan: discovered during this ticket's Phase 1 reversed-topology dogfood leg
sage-review-design: completed
sage-review-completeness: completed
sage-review: completed
---

# ws-dashboard native-Windows daemon silently drops all persisted dashboard state when HOME is unset

## Background

Discovered while performing `260707-chore-dashboard-linked-server-tunnel-dogfood-plan`'s
Phase 1 reversed-topology dogfood leg: a Windows-hosted gateway daemon (normal
owner-auth, `--bind-mode local`) linking a WSL-hosted remote daemon (normal
owner-auth, `--bind-mode local`, `http://localhost:<port>`).

The link handshake succeeded (`POST /api/dashboard/servers/link` returned `200`
with `"status":"connected"`), but every subsequent forwarded call against that
linked server (`GET .../resources`, `GET .../root-picker`,
`POST .../work-roots/open`) returned `404 "unknown server"` — as if the link
had never been persisted, even though the request was against the same
still-running daemon process that had just returned success for the link
call, and no daemon restart occurred in between.

Root-caused via `default_state_file`
(`ws-dashboard/crates/daemon/src/persistent_state.rs:478-491`):

```rust
fn default_state_file() -> Option<PathBuf> {
    if let Some(path) = std::env::var_os("WS_DASHBOARD_STATE_FILE") { ... }
    if let Some(path) = std::env::var_os("WS_DASHBOARD_STATE_HOME") { ... }
    if let Some(path) = std::env::var_os("XDG_STATE_HOME") { ... }
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .map(|home| home.join(".local/state/ws-dashboard/opened-workroots.json"))
}
```

This has no Windows-native fallback (`USERPROFILE`, `%LOCALAPPDATA%`, or an
equivalent platform-aware resolution, e.g. the `dirs`/`directories` crate).
`HOME` is not set by default on a stock Windows install (confirmed on the
dogfood host: `$env:HOME` is empty while `$env:USERPROFILE` is
`C:\Users\<user>`), so on a native Windows daemon run without one of
`WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`/`HOME`
explicitly set, `default_state_file()` returns `None`.

Every persistence method in `DashboardStateStore`
(`persistent_state.rs:46,61,69,84,93,111`) guards on `self.state_file.as_deref()`
and silently returns `Ok(())` / an empty collection when it's `None` — so
writes report success without persisting anything, and reads always come
back empty. Confirmed directly (isolated from network/link-handshake
variables): with `$env:HOME` explicitly set before launching the daemon, a
`POST /api/dashboard/root-picker/pins` call produces a real
`opened-workroots.json` under `$env:HOME/.local/state/ws-dashboard/`; with
`$env:HOME` unset (the Windows default), the identical call returns `200` but
no state file is ever created and nothing survives to the next request.

This means, on an out-of-the-box native Windows daemon: linked servers,
opened work-root registrations, and root-picker pins are all silently
non-functional beyond the current in-memory request — not just linked
servers. This is a genuine, previously undiscovered daemon bug distinct from
the already-settled `--bind-mode public` Host-check non-bug
(`ai-docs/tickets/.dropped/260707-bug-dashboard-public-bind-host-check-rejects-own-address.md`).

## Phases

### Phase 1: Add a Windows-native fallback to `default_state_file`

- Extend `default_state_file()` (`persistent_state.rs:478-491`) so a native
  Windows build resolves a real per-user state directory when none of
  `WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`/`HOME`
  is set — default to the plain `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json`
  via `std::env::var_os("LOCALAPPDATA")` (no new dependency). Only reach for a
  platform-directories crate instead if the workspace already depends on one
  elsewhere for an equivalent purpose; otherwise the plain env-var approach is
  the accepted default and does not need further judgment.
- Preserve the existing fallback order and env var names for Linux/macOS;
  only add the Windows-specific branch (guarded by `cfg(windows)` or as a
  final fallback after `HOME` fails, whichever keeps the non-Windows
  behavior byte-for-byte identical).
- Reproduce locally first with the isolated root-picker-pin check described
  above (no cross-machine setup needed: a single native Windows daemon run
  with `HOME` unset, `POST /api/dashboard/root-picker/pins`, confirm no state
  file appears) before attempting a fix, then re-verify the same check
  produces a persisted state file.
- Re-run `260707-chore-dashboard-linked-server-tunnel-dogfood-plan`'s
  reversed-topology forwarded-operation walk (resources, root-picker,
  work-roots/open, files read/write, Git status/branches, terminal
  create/close, terminal WebSocket relay) end to end afterward, **if** a real
  native-Windows host is reachable in the implementing session. If it is not
  (the sibling dogfood ticket already hit this exact wall: its SSH probe was
  blocked by session tooling before any cross-machine step ran), the isolated
  root-picker-pin repro above is sufficient to confirm the fix — do not block
  this ticket on cross-machine access it doesn't control. Record which of the
  two verification levels (isolated repro only, vs. full cross-machine walk)
  was actually achieved in the Result.
- Consider whether `DashboardStateStore` should surface a warning (e.g. a
  one-time `tracing::warn!`) when `state_file` resolves to `None`, so a
  silently-broken persistence path is at least visible in daemon logs rather
  than indistinguishable from a working-but-idle daemon. If pursued, warn
  once at construction time (e.g. in `default_local()`) rather than adding
  per-call dedup state, since `DashboardStateStore` is currently
  `Clone + Default` with no internal shared/mutable state — this is optional
  ("consider"), not a requirement.
- Add the new spec entry described in `## Spec Impact` below (Daemon
  Foundation section, fresh stem via `ws/spec_stem.generate`) as part of this
  phase's completion, not as a separate untracked follow-up — the fix isn't
  done until the Windows-native fallback order is documented, not just
  implemented.

## Spec Impact

No existing spec anchor addresses cross-platform state-file resolution.
`260515-ws-web-daemon-foundation` and `260525-ws-dashboard-endpoint-linked-server-add`
(`ai-docs/spec/ws-web-dashboard/index.md`) both describe daemon serving and
linked-server persistence behavior in OS-neutral terms and never mention
`HOME`, `XDG_STATE_HOME`, or a Windows-native fallback path — the current spec
text is accurate for Linux/macOS and silent on Windows, so this bug is a real
gap in implementation, not a documented behavior this ticket contradicts.

Once fixed, add a new spec entry under the Daemon Foundation section (a fresh
stem via `ws/spec_stem.generate`) documenting the full state-file resolution
order including the Windows-native fallback (e.g. `LOCALAPPDATA`), so the
existing `WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`/
`HOME` order in `persistent_state.rs:478-491` is fully documented rather than
only discoverable from source.

## Escalations

- None yet.
