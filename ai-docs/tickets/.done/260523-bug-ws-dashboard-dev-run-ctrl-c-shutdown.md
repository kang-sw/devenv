---
title: Investigate dev.sh run Ctrl-C shutdown hang
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-bug-ws-dashboard-windows-terminal-control-keys: separate browser terminal Ctrl-C forwarding issue
spec:
  - 260515-ws-web-daemon-foundation
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-23
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

Follow-up dogfood confirmed that closing the paired browser tab allowed the
`dev.sh run` process to exit. The developer terminal shutdown signal should take
priority over open browser connections in local development.

## Decision

`dev.sh run` and the daemon serve path should prefer prompt process shutdown
after the user presses `Ctrl-C`. Long-lived browser connections may receive a
bounded graceful window, but they must not keep the local development server
alive indefinitely after the outer server process has been interrupted.

## Follow-Up Questions

- Does the hang require an open paired browser tab or an active terminal pane?
- Does the daemon log or otherwise observe the first `Ctrl-C` before hanging?
- Should terminal sessions, SSE streams, and WebSocket tasks receive an explicit
  shutdown broadcast instead of relying only on connection drain?

## Notes

This is separate from browser-terminal `Ctrl-C` forwarding. Here the input is
the outer developer terminal trying to stop the dashboard daemon itself.

## Result (c37041b) - 2026-05-23

`ws-dashboard serve` now begins Axum graceful shutdown after the outer shutdown
signal, but gives long-lived browser connections only a bounded grace period.
If an idle socket, SSE stream, WebSocket, or other open browser connection keeps
the graceful drain alive past that window, the serve future is dropped so
`dev.sh run` can exit promptly after `Ctrl-C`.

The implementation keeps immediate shutdown tests and adds a daemon server test
that opens an idle TCP connection, triggers shutdown, and verifies the server
task still returns within the configured grace period.
