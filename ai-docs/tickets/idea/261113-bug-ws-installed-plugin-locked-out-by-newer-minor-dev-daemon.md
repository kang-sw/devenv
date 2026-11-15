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

## Owner Direction (historical)

The following records the 2026-11-13 direction under the former taxonomy.
The current policy mapping is in `## Decisions` below.

- 2026-11-13: avoid minor version bumps from now on. Cross-version changes should be designed as T0/T1 (additive, alias-preserving) wherever possible; a T2 change or minor bump needs explicit owner approval.
- 2026-11-13 (board 401 = c): AGENTS.md Compatibility Tiers now says to avoid T2 and requires owner approval for T2 and minor bumps. This ticket should also make the dev rebuild warn before it replaces the shared daemon with a newer-minor one while live clients run an older minor.

## Decisions

### Compatibility addendum - 2026-11-15

- `c0ec803cb` supersedes the old “avoid T2 / owner-approved T2 minor” tier
  wording: **T0–T2 permit patches; T3 requires a minor bump and explicit owner
  approval; T4 never ships** (migration must lower it to T3 or below). T1 now
  means display-only unknown/default values with no data change, not reconnect
  recovery. T2 means overwrite recovered by the new reader on reopen from
  retained raw/append-only data. No such T2 recovery is established here.
- **Reported lockout: old T2 breaking → T3 old-version boundary.** In the
  current source, an old plugin's launcher reads the newer daemon's health
  version, does not replace a higher-version daemon, and refuses a different
  minor (`agents-plugin/bin/ws-mcp-launcher.py:1350-1372,1437-1460`). If a hello
  reaches the daemon, the old launcher supplies its version and the daemon
  reads/rejects it before session acceptance
  (`agents-plugin-tool/internal/wsdashboard/proxy.go:232-254`). These refusal
  checks perform no shared-session/store write; new-version data reopen cannot
  repair a connection refusal. This is source evidence for the current path,
  not a reproduction of the Nov 13 incident or inspection of its cached binary.
- A diagnostic or advance-warning-only repair targets **T0** only if it preserves
  existing admission and call/store behavior. Acceptance must show that older
  peers still work normally on compatible paths and no persisted fields are
  lost. A behavior-preserving alias is normally T0; tool-list refresh cannot
  repair a removed call. No repair or cross-minor admission architecture is
  chosen by this re-grade.
- Preserve the warning intent recorded above, but a cross-minor notice alone
  cannot waive a T3 data-safety refusal. Whole-file loss of unknown fields is
  not T2 without actual new-reader recovery. The mixed-version guard ticket
  owns the writer audit; this idea does not authorize bypassing its findings
  or any existing refusal to keep a session running.
