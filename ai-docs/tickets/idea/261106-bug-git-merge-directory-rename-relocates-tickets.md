---
title: "git.merge: directory-rename detection relocates tickets into .done/"
---

# git.merge: directory-rename detection relocates tickets into .done/

## Observation (2026-11-06, dogfood)

`ws/git.merge` of `impl/develop/scone-chemo-vista` (261106-feat-whiteboard-rich-items)
into `develop` stopped with `status: conflict`. The branch had moved the only
ticket in `ai-docs/tickets/ready/` to `.done/`, so on that side the whole `ready/`
directory was renamed to `.done/`. Meanwhile develop (c3ee7f453) had added four new
tickets to `ready/`. Git's directory-rename detection (merge-ort default
`merge.directoryRenames=conflict`) then proposed moving the four new tickets into
`.done/` and reported them as `AU` conflicts. No code conflicted. The lead fixed it by
hand: it restored the four files to `ready/` from c3ee7f453 (merge 390a5c34c).

Status directories are a state machine, not a tree that gets renamed. A ticket board
is likely to hit this whenever a branch empties a status directory that the target
branch has added to in the meantime. With `merge.directoryRenames=true`, the
relocation would happen silently.

## Possible direction

- `ws/git.merge` passes `-c merge.directoryRenames=false` (or the equivalent ort
  option), at least for paths under the ticket root.
- Or: `tickets.move` / `tickets.close` keep each status directory alive with a
  tracked placeholder, so a branch never empties one.
- Either way, a test merges a branch that empties `ready/` against a target that added
  to `ready/`.
