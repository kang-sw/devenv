---
title: git.commit rename and deleted path handling
---

# git.commit rename and deleted path handling

## Background

Dogfooding found that `ws/git.commit` fails when the caller passes an explicit
old path from a rename or deletion. During a skill rename, the caller supplied
both old and new skill paths so the commit would stay tightly scoped. The tool
ran `git add -A -- <old-path> ...` and Git returned a pathspec error because the
old path no longer existed in the worktree.

The surprise is that explicit path commits should handle paths reported by Git
status, including `old_path` values for renames and deletions. Callers should
not have to widen the path scope to a parent directory just to commit a rename.

## Constraints

- Preserve explicit staging safety; do not stage unrelated files to make
  rename/deletion cases pass.
- Keep missing unrelated paths as actionable errors.
- Support both native Git status data and paths supplied directly by a caller.

## Phases

### Phase 1: Handle rename and deleted paths in explicit commits

Update `ws/git.commit` staging so explicit paths that correspond to tracked
renames or deletions are accepted. The implementation should distinguish
legitimate missing paths from unrelated typos by consulting Git status or an
equivalent tracked-path check before staging.

Acceptance criteria:

- A rename can be committed when the path list includes the old path and the new
  path.
- A deleted tracked path can be committed through an explicit path list.
- A missing path that is not tracked or changed still fails with a clear error.
- The staging operation remains limited to the requested path set.
- Tests cover rename, deletion, and unrelated missing-path behavior.
