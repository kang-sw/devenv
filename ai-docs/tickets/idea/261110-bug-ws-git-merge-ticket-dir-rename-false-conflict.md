---
title: ""
---

## Observation (2026-11-10)

Merging `impl/develop/argue-trace-ether` into `develop`: the impl branch
moved its ticket `ready/` -> `.done/`, leaving `ai-docs/tickets/ready/`
empty on that side, while `develop` had meanwhile added another ticket under
`ready/`. Git's ort strategy then treated `ready/` -> `.done/` as a
directory rename and reported

```text
CONFLICT (file location): ai-docs/tickets/ready/<new-ticket>.md added in develop
inside a directory that was renamed in impl/..., suggesting it should perhaps
be moved to ai-docs/tickets/.done/<new-ticket>.md.
```

`git merge-tree --write-tree` showed it; with `-c merge.directoryRenames=false`
the merge was clean. `ws/git.merge` takes no merge options, so the lead had to
merge natively.

Any parallel run hits this whenever a worker closes the last ticket in a
status directory while the lead promotes another into it, which is the
normal parallel-mode shape.

## Direction (unconfirmed)

`ws/git.merge` (and any ws tool that merges ticket trees) could run git with
`merge.directoryRenames=false`; ticket status directories are never renamed as
a whole. Needs a check of whether real directory renames elsewhere in a repo
should still be detected (scope the setting, or accept the loss).
