---
title: In-app daemon shutdown/restart + kill-all-terminals from the settings panel
related:
  260722-feat-dashboard-settings-panel: host surface this lives inside
related-mental-model:
  - ws-web-dashboard/terminal
---

# In-app daemon shutdown/restart + kill-all-terminals from the settings panel

## Background

The dashboard daemon and its terminal helpers are the **same binary**: the
daemon spawns each terminal helper by re-executing `std::env::current_exe()` as
a `terminal-helper` subcommand (`terminal.rs:170` `default_helper_binary`,
dispatched via `cli.rs` "internal re-exec target"). This single-binary
self-re-exec is a clean deployment pattern (helper code always matches the
daemon commit, no separate build/version) and is worth keeping.

Its cost is an operational footgun: in the OS process list every process is
`ws-dashboard.exe`, indistinguishable except by whether the command line
contains `terminal-helper`. The natural "stop the dashboard" reflex —
`taskkill /F /IM ws-dashboard.exe` — is a blanket image-name kill that takes
down the daemon **and every helper**, and each helper's kill-on-close job then
tears down its child shell. Result: all open terminals die. The documented-safe
restart (kill only the non-`terminal-helper` PID) is a fragile manual dance
(captured in `_index.local.md`); it should not be the only safe path.

Now that terminal lifetime is decoupled from the daemon (detached helpers
survive a daemon exit and are re-adopted on next boot via
`TerminalRegistry::boot_reconcile`), the two OS-level kill modes map cleanly to
two explicit, guarded UI actions. Give the operator those actions in-app so they
never reach for `taskkill` — sidestepping the self-re-exec naming problem
without abandoning the pattern.

## Decisions

Two distinct destructive actions, deliberately separated:

- **Stop / Restart daemon** — terminal-preserving. Ends only the `serve`
  process; detached helpers + their shells survive; on next launch reconcile
  re-adopts them. This is the UI-native form of the documented
  `Stop-Process -Id <serve-pid>` restart. Preferred variant is **Restart** via
  self-re-exec: the daemon spawns a fresh copy of itself (`current_exe()`) and
  then exits, so the front-end reconnects and terminals are preserved — no
  external relaunch needed. A **Stop-only** variant is simpler but leaves the
  serving page dead, so its UX must state how to bring the daemon back.
- **Kill all terminals** — the deliberate teardown. Enumerates every registered
  helper (registry dir, e.g. `%LOCALAPPDATA%\ws-dashboard\terminals`) and shuts
  each down **gracefully** (socket close → helper's kill-on-close job cleans up
  the child shell), with hard-kill as fallback. This is the UI-native, safe
  form of the blanket `/IM` kill. Without this action, "stop daemon" orphans
  helpers with no in-app cleanup path and they accumulate.

## Constraints

- **Auth gating.** Shutdown / restart / kill-all are sensitive. The current
  dogfood daemon runs `--no-auth` on `127.0.0.1`, where this is acceptable, but
  any real deployment must gate these endpoints behind owner-auth. Design the
  endpoint(s) with the auth branch from the start, not as a retrofit.
- **Double-confirm, not just placement.** Nesting the control (settings → a
  "Shut down" section → the button) is the coarse guard; pair it with an
  explicit confirm dialog that names the consequence and, for kill-all, the
  terminal count ("Close all N terminals?").
- **Self-serving-shutdown honesty.** A daemon-stop button kills the process
  serving the very page it's on. UX copy must be explicit; the Restart variant
  is preferable precisely because the page comes back.
- Must not regress the detached-helper / `boot_reconcile` invariant: a daemon
  Stop/Restart must leave helpers adoptable, i.e. it must not signal helpers.

## Open questions

- Restart-via-self-re-exec vs Stop-only for v1 — is the extra re-exec plumbing
  worth it, or ship Stop-only first and add Restart later?
- Graceful helper shutdown protocol: is there already a helper-side socket
  "close" verb to reuse for kill-all, or does it need adding?
- Where exactly in the settings panel (`260722-feat-dashboard-settings-panel`)
  the "Shut down" section sits, and whether it is hidden behind a disclosure by
  default.
- Cross-platform parity: the footgun is described on Windows; confirm the same
  affordances behave correctly for the Unix detached-helper path.

## Implementation notes (dogfood v1, 2026-07-25)

A first cut landed directly (dogfood-driven, `--no-auth` local daemon):

- Daemon: `GET /api/dashboard/build-info` (daemon exe mtime + served
  `index.html` mtime + `CARGO_PKG_VERSION`), `POST /api/dashboard/shutdown`
  (fires an `AppState.shutdown` `Notify` that the existing shutdown_task selects
  on alongside ctrl-c — terminal-preserving by construction), and
  `POST /api/dashboard/terminals/kill-all` (`TerminalRegistry::drain_all()` →
  `terminate()` each, bypassing the per-terminal work-root access check).
- Frontend: a new **Advanced** settings section — build info at the top,
  arm/confirm-guarded **Shut down** and **Close all** buttons at the bottom. Plus
  the cosmetic sidebar change (settings gear relocated to a top-left square brand
  affordance; `[brand] SERVERS … [+]` single row).

Chosen for v1: **Stop-only** (not Restart-via-self-re-exec) and **ungated** (fine
for the loopback `--no-auth` dogfood). Still open, tracked here:

- **Auth gating** before any non-dogfood deployment (the endpoints are currently
  reachable by anyone who can reach the daemon).
- **Restart variant** via self-re-exec, so the page reconnects instead of dying.
- Cross-platform parity check for the kill-all helper teardown on Unix.

## Prior Art

- `_index.local.md` "terminal-preserving restart" runbook — the manual
  procedure this feature productizes (kill only the non-`terminal-helper` PID).
- `TerminalRegistry::boot_reconcile` — identity-gated helper re-adoption that
  makes a terminal-preserving daemon restart correct.
- `terminal.rs:170` `default_helper_binary` / `cli.rs` terminal-helper dispatch
  — the self-re-exec mechanism whose naming this feature works around.
