---
title: "A hosted session's exit cause is lost when its holder directory is cleaned up"
completed: 2026-11-13
---

# A hosted session's exit cause is lost when its holder directory is cleaned up

## Background

Owner report, 2026-11-13: all of the owner's dashboard-hosted Claude sessions
were found closed. Investigation found no idle reaper for hosted sessions (the
holder's only idle exit, `IDLE_EXIT_SECONDS` in
`agents-plugin-tool/internal/wsholder/holder.py`, needs no running child and no
attachment for 24 h). `dashboard.log` showed each holder ending with
`holders: stopped holder hXXXX (pid N)` paired with `MCP session closed`, i.e.
the Claude child exited, but the stopped holders' directories had already been
removed (`orphanDirGrace`, 10 min, in the holder manager), taking `holder.log`
and the child's exit code with them. `dashboard.log` lines carry no
timestamps. So why the sessions ended (user close, crash, host kill, auth
expiry) could not be recovered.

## Idea

Keep a hosted session's exit record after its holder directory goes: write
the child's exit code/signal, the holder's exit reason, and a timestamp into
`dashboard.log` (timestamped) or the session's own record before the
directory is removed, and show it on the session row (e.g. "ended: exit 1 at
12:31").

## Open

- Where the record lives (daemon log vs the session store) and its
  compatibility tier (a new optional field is T0).


## Resolution (2026-11-13)

Fixed by delegate: 70678ab2e retains hosted holder exit details; 2d10d7277 treats Linux zombie holders as stopped (a branch-only timeout it exposed). Merged 89ad004ec. Live check of the exit record in the dashboard is part of the owner's post-merge checks.
