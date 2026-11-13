---
title: "Dashboard page freezes on a Claude conversation switch; the new page starts with the old figures"
dropped: 2026-11-13
---

# Dashboard page freezes on a conversation switch

## Observation (2026-11-10)

The user reported the dashboard context meter lagging behind the terminal
status band and "not following page switches". The lag (feed read usage only
per turn) was hotfixed in 5b0037400 (f56fa8d42: the feed now re-reads after
every model tool call, like the band). The page-switch part remains.

## Evidence (delegate investigation, not re-verified by the lead)

- The dashboard page is keyed by the session's reply id
  (`agents-plugin-tool/internal/wsdashboard/stream_cache.go:36`), which derives
  from the ws session key (`internal/mcp/live_snapshot.go:229`).
- `/clear` or a resume starts a new conversation with a new session key, so a
  new dashboard page; the page the user had open freezes on its last figures
  while the terminal band moves on.
- The claude-mod feed's `restart()` keeps the previous conversation's usage and
  re-sends it behind the `reset` (`agents-plugin/hooks/claude-mod/dashboard.ts`
  ~:1047 "The held usage outlives a restart", `restart` ~:1107); `register.ts`
  ~:1680 calls `dashboard.restart()` with no fresh usage read. The new page
  shows the old figures until the first model tool call or turn end.
- The terminal band keeps its figures across the switch too
  (`register.ts` ~:1671-1676).
- No path was found that shows another session's snapshot on a page; the
  page's own `ctx` region updates correctly (`dashboard.js` ~:376-409,
  hidden-tab and reconnect paths ~:3901-3946).

## Directions (not decided)

- Make the old page point to, or follow, the conversation that replaced it.
- Read usage fresh right after the feed restart, or drop the held figures on
  restart. Whether the host reports the new conversation's usage at that
  point is unverified; probe it first.

## Constraints

- claude-mod changes mirror to `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).
- Go tests via `~/.local/bin/ws-rgo` only.


## Resolution (2026-11-13)

Owner 2026-11-13 (board item (51)): dropped. The stale page is a CLI-path issue; the CLI path stays a fallback and the daemon-hosted agent becomes the main path, so this is not worth fixing.
