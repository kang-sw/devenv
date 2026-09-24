---
title: Ticket index read timeout is shorter than a normal SSH round trip
related:
  260924-feat-origin-ticket-ownership-index: introduced the read path and its DefaultReadTimeout
---

# Ticket index read timeout is shorter than a normal SSH round trip

## Background

Dogfood on this repository (2026-09-24, develop 41803afb), right after a
successful `tickets.acquire` / `tickets.release` push to a GitHub SSH origin:
`tickets.query` reported `ticket-index: origin unreachable; ownership is from
the cached index (age 4m57s)`, although origin was reachable.

- `DefaultReadTimeout` is 1500 ms (`agents-plugin-tool/internal/wsindex/client.go:19`),
  chosen for the ticket's "query must not block beyond roughly 2 seconds".
- A plain `git ls-remote origin 'refs/ticket-index-larkspur/*'` from the same
  machine took 2.6 s wall time (SSH handshake dominates).
- So on an ordinary SSH remote every read past the 60 s TTL times out, is
  reported as unreachable, and serves the stale cache; the "unreachable"
  wording is misleading, and the view never refreshes from the read path.
  Writes (10 s timeout) are unaffected and refresh the cache as a side effect.

## Open questions

- Raise the read timeout (and relax the ~2 s query bound), or keep the bound
  and change what a read timeout means: report "not refreshed (timed out)"
  rather than "unreachable", and keep unreachable for real transport failures.
- Whether SSH connection reuse (ControlMaster) is in scope; it is user
  config, so shipped behavior should not depend on it.
- The absence-vs-failure distinction (discontinuity discard) must stay intact:
  a timeout is never absence.
