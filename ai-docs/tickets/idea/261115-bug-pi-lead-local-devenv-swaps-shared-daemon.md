---
title: "A lead-role pi -p with the local-devenv marker silently swaps the shared dashboard daemon"
related:
  261115-bug-pi-print-mode-empty-after-subagent-push: "same dogfood setup (headless pi -p workers driven from a Claude lead's Bash)"
---

# A lead-role pi -p with the local-devenv marker silently swaps the shared dashboard daemon

## Observation (2026-11-15)

A peer lead launched two headless `pi -p` workers from Bash at 02:02Z. MAIN,
the only session allowed to swap the shared daemon, then found a daemon it
had not started (pid 2269, `build_time` 1794708128127 = 02:02:08Z) in place
of its own.

Mechanism, read from source:

- `~/.pi/agent/settings.json` loads the pi package from the devenv
  checkout, and `agents-plugin-pi/.local-devenv-runtime` exists.
- A top-level `pi -p` carries no `WS_PI_SPAWN_ROLE`, so it counts as a
  lead and builds `ws-mcp` from the marker's `source_root` at startup
  (`agents-plugin-pi/src/local-devenv.ts:160-212`; lead/fork gate at
  `:262`). The build is written under
  `agents-plugin-pi/.runtime/local-devenv/` and deleted afterwards.
- The launcher then runs `Ensure` with that fresh `0.50.0-dev` binary. At an
  equal version, different content plus a newer build time wins
  (`internal/wsdashboard/ensure.go:164-184`), so the newly built daemon
  replaces the running one, even though the caller never asked for a swap.

Workaround in use: launch headless pi with `WS_PI_SPAWN_ROLE=worker`, which
takes the release path (no source build; a release never replaces a
same-core `-dev` daemon). Smoke-tested: ws tools answer and the running
daemon is untouched.

## Questions for later

- Should a headless or non-interactive lead skip the local-devenv rebuild,
  or attach without replacing (as the Electron shell does)?
- Should a source-built daemon replace another source-built daemon only
  when the source commit differs and is a descendant, rather than on build
  time alone?
- Should the replacement be surfaced to the user (a notice naming the
  replacing session) instead of happening silently?
