---
title: "pi.spawn refuses with a stale Pi version: the daemon inherits a launcher's PATH and probes Pi once per process"
---

# pi.spawn refuses with a stale Pi version

## Background

Dogfood surprise, 2026-11-14 (EXEC lead). `pi.spawn` refused with
`pi 0.84.4 is installed; Pi subagents need pi 1.0 or later` for one lead
session. Global `pi` on PATH was 1.0.4 at the time; after the owner ran
`pi --update` it was 1.5.2. A peer lead's spawns had succeeded minutes
earlier on the previous daemon.

Evidence (ps, VERIFIED):

- The shared dashboard daemon had been replaced at 01:22:43 (pid 5776) by the
  chain holder 4809 (01:22:41) -> `pi` 4846 -> `ws-mcp-launcher.py serve
  --stdio` 5051 -> daemon 5776. Its environment's PATH began with
  `/Users/kang-sw/devenv/agents-plugin-pi/node_modules/.bin` and then npm's
  run-script entries. That directory holds the repo's dev Pi 0.84.4.
- `PiReady` (`agents-plugin-tool/internal/mcp/pi_tools.go:484`) checks the
  version through `installedPiVersion`, which caches the probe in a
  `sync.Once` for the process lifetime (`pi_tools.go:86-94`). A Pi upgrade
  therefore never reaches a running daemon.
- `python3 scripts/dev.py rerun daemon` from a clean shell (new pid 12261,
  clean PATH) fixed it.

## Notes

- Two separable issues:
  1. A daemon started by a launcher inherits whatever PATH that launcher
     had. Here it came, INFERRED, from an npm run-script environment that
     reached a hosted Pi session. Which process first carried the npm PATH
     is not established.
  2. The once-per-process version probe has no refresh. A refusal could
     re-probe once before failing, or the cache could expire.
- A fix would touch daemon start or launcher env handling, and the Pi probe.
  Compatibility is likely T0 (behaviour only).
