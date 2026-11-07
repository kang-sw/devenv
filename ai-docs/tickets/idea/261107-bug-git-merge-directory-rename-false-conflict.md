---
title: ""
---

## Background

Dogfood surprise, 2026-11-07, merging `impl/develop/quit-derby-giddy`
(261107-feat-ws-whiteboard-now-markdown) into `develop`. The impl branch
closed its ticket (`ready/` -> `.done/`), and it was the only file in
`ready/` at the branch point. Meanwhile `develop` added a new ticket in
`ready/` (261107-feat-ws-dashboard-status-band). Git's directory-rename
detection treated the move as a rename of the whole `ready/` directory, and
`git merge-tree` reported `CONFLICT (file location)`, proposing to move the
new ready ticket into `.done/`. `ws/git.merge` would have stopped on that
conflict, and accepting git's suggestion would silently close an unstarted
ticket.

The lead merged natively with `-c merge.directoryRenames=false`, which merged
cleanly (14ab46b3e).

## Idea

`ws/git.merge` (and any merge-tree preview it runs) could pass
`merge.directoryRenames=false`, because ticket status directories are
moved one file at a time and are never renamed as a whole. Alternatively, it
could pass the flag only when the conflict is confined to
`ai-docs/tickets/<status>/`. This is likely to recur whenever the last ticket
leaves a status directory on one branch while another branch adds to it.
