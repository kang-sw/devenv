---
title: ws git.merge lets directory-rename detection move tickets into the wrong status directory
completed: 2026-11-08
---

# ws git.merge lets directory-rename detection move tickets into the wrong status directory

## Background

Dogfood observation, 2026-11-08. An impl branch was cut when `ready/` held
one ticket; the worker closed that ticket (`ready/` -> `.done/`). Meanwhile
develop promoted two more tickets into `ready/`. `git merge-tree` of develop
and the impl branch reported `CONFLICT (file location)`: git's directory-rename
detection read the branch's move of the sole `ready/` file as a whole-directory
rename `ready/` -> `.done/` and proposed moving develop's two new ready
tickets into `.done/`.

`ws/git.merge` has no option to disable this; the lead merged natively with
`git -c merge.directoryRenames=false merge --no-ff` (merge 4b402e9dd).

Ticket status directories are exactly the shape this heuristic misfires on
(a near-empty directory whose last file moves). Candidate fix:
`ws/git.merge` (and any ws merge-tree preflight) always passes
`merge.directoryRenames=false`, since ticket moves are always explicit.
Needs a test with a one-file `ready/` on the source branch and a new
`ready/` file on the target.

## Result

Fixed in bcc267066: `ws/git.merge` runs its merge with `-c merge.directoryRenames=false`; regression test `TestImplMergeIgnoresDirectoryRenameHeuristic`.
