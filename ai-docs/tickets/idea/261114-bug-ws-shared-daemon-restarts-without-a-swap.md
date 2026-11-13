---
title: "Shared dashboard daemon restarts without an explicit swap"
---

# Shared dashboard daemon restarts without an explicit swap

## Background

Dogfood surprise, 2026-11-13. The shared dashboard daemon of this checkout
(`~/.cache/ws@kang-sw-devenv/dashboard`, port 29417) was replaced twice
without anyone running `python3 scripts/dev.py rerun daemon`:

- 15:04:15Z: pid 98387 -> 63011. `dashboard.log` was recreated, so the old
  daemon's log was lost.
- 15:19:51Z: pid 63011 -> 8932, build `content_hash=e7c6e270d283`. A later
  explicit swap from develop `1ed33f98d` reported "running daemon has
  identical content", so the auto restart had already built the root
  checkout's current source.

Every lead and worker involved (MAIN, UX, EXEC workers) states it ran no
daemon start or swap command. Each restart came 1-3 minutes after a commit
and a `go test` finished in the root checkout.

Impact: a MAIN `whiteboard_update` carrying about 74 KB of panels failed
twice with "dashboard daemon not reachable; the request did not complete"
(`agents-plugin/bin/ws-mcp-launcher.py:1639`, `:1677`) at about 15:03Z,
just before the first restart. Small updates sent at the same time
succeeded. Live sessions also lose their stream for the restart window.

## Hypotheses to check

1. The launcher (local-devenv source build) sees source or metadata identity
   newer than the running daemon and replaces it on its own, for example
   when an MCP session reconnects after the root's source changes.
2. A test leaks onto the shared state directory (`WS_CACHE_HOME` unset in
   some test) and starts or stops the real daemon.
3. The large-request failure is independent: a size or timeout limit on the
   launcher -> daemon proxy path. Reproduce it on a stable daemon before
   ruling this out.

The answer decides whether this is a bug (an unannounced replacement, or a
test touching shared state) or intended behaviour that needs a notice. The
project's rule is that only the EXEC lead swaps the shared daemon, with mail
to the peer leads before and after.
