---
title: Restore dashboard terminal tabs after daemon restarts
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: restores the owning workRoot list that terminal tabs attach to
spec:
  - 260516-ws-web-dashboard-workroot-io-restore-model
  - 260523-ws-dashboard-terminal-tab-restore
related-mental-model:
  - ws-web-dashboard
---

# Restore dashboard terminal tabs after daemon restarts

## Background

After Phase 1 workRoot persistence, restarting `ws-dashboard/dev.sh run`
restores the opened workRoot list but not the terminal tabs that were open in
the browser. Dogfood feedback accepted that live terminal processes do not need
to survive daemon death, but restoring terminal tab context would reduce restart
friction.

The implementation must stay honest: a restored tab is a newly created daemon
terminal session, not the old PTY process. The current terminal model does not
capture a live shell's changing PWD, so Phase 1 can persist only a safe
workRoot-relative cwd hint. Until a later shell-integration or explicit PWD
capture feature exists, that hint is normally the workRoot root.

## Phases

### Phase 1: Restore terminal tabs as new sessions

Persist browser-visible terminal tab descriptors per workRoot and use them to
recreate terminal tabs as new daemon terminal sessions after restart. The
descriptor includes the workRoot id, title, and a workRoot-relative cwd hint
when one is available; it must not persist daemon-private terminal ids as
resumable handles or expose host paths.

The restore path should run through the dashboard command spine so later
Tmux-like keybindings can target the same behavior. Closing a terminal removes
its descriptor, and existing live sessions from the daemon remain authoritative
when present. Verification should cover pure persistence helpers, route-level
cwd hint handling, and focused frontend tests for terminal restore behavior.
