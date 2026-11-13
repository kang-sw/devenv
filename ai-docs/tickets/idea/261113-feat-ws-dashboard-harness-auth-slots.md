---
title: "Dashboard: remember per-harness auth slots and switch accounts for new sessions"
related:
  261113-feat-ws-dashboard-rail-usage-gutter: prerequisite (the usage gutter and popover this extends)
---

# Dashboard: remember per-harness auth slots and switch accounts for new sessions

## Background

Owner request (2026-11-13, whiteboard topic `usage-gutter`, (131)=a): beyond
showing plan usage, the dashboard should remember each harness's auth slots
(logged-in accounts) and let the owner swap between them, e.g. when one
account's weekly limit is exhausted.

Settled direction ((131)=a): follow the Orca (stablyai/orca, MIT; design
reference only) model - one config home per account, selected for newly
launched sessions through `CLAUDE_CONFIG_DIR` / `CODEX_HOME`; the default
homes are never modified, running sessions are never touched. Rejected:
swapping the default credential files in place (refresh-token overwrite
races, breaks running sessions).

Open questions for capture:

- Account login flow per slot (run the harness's own login inside the slot
  home) and what is shared across homes (history, settings, plugins).
- How daemon-launched (hosted) sessions pick a slot, and where the active
  slot choice lives.
- Pi's `openai-codex` login: slot it too, or leave Pi to its own account.
- Popover UX: slot list per harness with each slot's usage, "use for new
  sessions" action.
