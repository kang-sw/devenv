---
title: Dashboard terminal inherits TERM=dumb from non-interactive daemon launch
related:
  260620-feat-ws-dashboard-loopback-no-auth-debug-mode: surfaced during WSL no-auth dashboard dogfooding
completed: 2026-06-20
---

# Dashboard terminal inherits TERM=dumb from non-interactive daemon launch

## Background

During WSL no-auth dashboard dogfooding, the browser opened the dashboard
successfully from Windows, but newly opened dashboard terminal sessions reported
`TERM=dumb`.

The daemon launch environment in Codex/tool execution had `TERM=dumb`, and the
dashboard terminal spawn path currently selects the shell from `SHELL` but does
not set or normalize terminal capability environment for the spawned PTY command.
As a result, the PTY child inherits the daemon process environment even though
the browser terminal is capable of xterm-like behavior.

## Expected Direction

Define a small terminal environment policy for dashboard PTY sessions:

- Preserve explicit real terminal values when the daemon was launched from an
  interactive shell.
- Treat missing, empty, or `dumb` `TERM` as unsuitable for browser PTY sessions
  and set a practical default such as `xterm-256color`.
- Consider whether `COLORTERM=truecolor` should be supplied only when absent or
  left entirely to callers.
- Cover the policy with a unit test near terminal shell selection and, if
  practical, a live terminal smoke test that observes `$TERM`.

## Result (2abc8bc) - 2026-06-20

Dashboard terminal spawn now sets `TERM=xterm-256color` when the daemon launch
environment has no `TERM`, an empty value, or `TERM=dumb`, while preserving
explicit non-dumb parent terminal types. Focused unit tests cover the
normalization policy, the full daemon test suite passed, and a no-auth WSL
dogfood probe observed `TERM_RESULT:xterm-256color` through the live terminal
HTTP routes.
