---
title: "ws/git.merge: git directory-rename detection moves unrelated tickets between status directories"
---

# ws/git.merge: git directory-rename detection moves unrelated tickets between status directories

## Background

Observed 2026-11-08 merging `impl/develop/gore-pod-grunt` into `develop`
(merge 864d85305). At the impl branch's base, `ai-docs/tickets/ready/` held
one file, the branch's own ticket, which the worker closed to `.done/`. Git's
merge-ort directory-rename detection read that as "`ready/` was renamed to
`.done/`" and moved a ticket added to `ready/` on `develop` meanwhile
(`261107-feat-ws-dashboard-interrupt-badge`, still in progress) into
`.done/`, leaving an add/add-style conflict (`AU`). The lead restored it by
hand. Without the conflict marker the misplacement could have landed silently
whenever the moved file had no counterpart.

Ticket status is directory-based, so any project whose `ready/` drains to a
single ticket can hit this on every worker merge.

## Idea

Have `ws/git.merge` run the merge with `-c merge.directoryRenames=false`
(file-level rename detection stays), or at least detect a merge result that
moves a ticket the source branch never touched and refuse with a diagnostic.
