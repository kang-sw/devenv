---
title: "dashboard-drop property scan misclassifies the deletion branch"
---

# dashboard-drop property scan misclassifies the deletion branch

## Observation

The Phase 2 teardown property scan counts `ws-dashboard/` path changes beyond
`main`. It correctly passed before the removal commit, leaving only
`origin/discuss`. After the implementation branch committed `git rm -r
ws-dashboard`, that branch itself reports one path-touching commit although it
contains a deletion rather than surviving dashboard code.

## Direction

Define the scan boundary explicitly: validate branch teardown before the
deletion commit, then exclude the active implementation branch from the
post-commit ref scan and separately require that tracked and working-tree
dashboard paths are absent. Preserve the `origin/discuss` exception.
