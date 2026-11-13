---
title: "Installed plugin is locked out when a dev rebuild brings up a daemon one minor ahead"
---

# Installed plugin is locked out when a dev rebuild brings up a daemon one minor ahead

## Observed (2026-11-13)

- Merging wb-messages (`bbd7c1bd5`) bumped develop to 0.50.0.
- The dev launcher rebuilt and replaced the shared dashboard daemon with
  0.50.0-dev (pid 4832).
- Every Claude session running the installed, cached wsflow 0.49.0 plugin then
  failed every ws call with "dashboard daemon not reachable; request was not
  sent (waited 75s ...)". This included the lead session and the UI-track peer.
- The daemon was healthy (port 29417 listening, HTTP 200).

## Cause

- `ws-mcp-launcher.py` `DaemonLink.connect` raises `DaemonUnavailable` when
  `same_minor(daemon, runtime)` fails (around line 1451).
- `needs_start` only replaces an older daemon, so a newer-minor daemon is
  neither accepted nor replaced, and the old one is gone.
- The real reason ("daemon X is outside this plugin's minor") only reaches
  launcher stderr; the MCP error hides it.

## Ideas (non-authoritative)

- Surface the minor-mismatch reason in the MCP error text, with the fix: update
  the plugin and reconnect.
- Have the dev rebuild refuse to replace, or warn before replacing, the daemon
  when live clients are on an older minor.
- Or let an older-minor client spawn its own side daemon.

## Recovery used

Update the installed plugin to the repo's 0.50.0, then reconnect MCP / restart
the session.

## Related

- `261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work`

## Owner Direction

- 2026-11-13: avoid minor version bumps from now on. Cross-version changes should be designed as T0/T1 (additive, alias-preserving) wherever possible; a T2 change or minor bump needs explicit owner approval.
