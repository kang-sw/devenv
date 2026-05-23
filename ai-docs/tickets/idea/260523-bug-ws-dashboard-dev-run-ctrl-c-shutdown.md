---
title: Investigate dev.sh run Ctrl-C shutdown hang
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-bug-ws-dashboard-windows-terminal-control-keys: separate browser terminal Ctrl-C forwarding issue
related-mental-model:
  - ws-web-dashboard
---

# Investigate dev.sh run Ctrl-C shutdown hang

## Background

Dogfood feedback found that `ws-dashboard/dev.sh run` can ignore or appear to
ignore `Ctrl-C` even when the same tmux pane interrupts a plain `sleep 100`.
The wrapper script builds the frontend and then `exec`s `cargo run -p
ws-dashboard-daemon -- serve --static-dir frontend/dist`, while the daemon waits
on `tokio::signal::ctrl_c()` and passes that future to Axum graceful shutdown.

Initial local reproduction without an attached browser session exited cleanly on
`Ctrl-C`, so this is unlikely to be a general tmux `intr` binding issue. The
more likely failure mode is that the shutdown signal reaches the daemon, but
graceful shutdown waits indefinitely for long-lived dashboard connections such
as Activity SSE streams, terminal WebSockets, or live browser requests.

## Follow-Up Questions

- Does the hang require an open paired browser tab or an active terminal pane?
- Does the daemon log or otherwise observe the first `Ctrl-C` before hanging?
- Should `dev.sh run` or the daemon install a bounded shutdown timeout for local
  development so a single `Ctrl-C` terminates promptly?
- Should terminal sessions, SSE streams, and WebSocket tasks receive an explicit
  shutdown broadcast instead of relying only on connection drain?

## Notes

This is separate from browser-terminal `Ctrl-C` forwarding. Here the input is
the outer developer terminal trying to stop the dashboard daemon itself.
