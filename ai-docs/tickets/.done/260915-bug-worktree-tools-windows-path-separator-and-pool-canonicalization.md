---
title: "worktree.acquire returns inconsistent path separators on Windows; pool tests use non-portable fixtures"
related:
  260910-feat-lead-run-worktree-parallel-route: context; introduced the worktree_tools.go surface this fixes
---

# worktree.acquire path separators on Windows + non-portable pool tests

## Background

Caught by the `ws-mcp release` v0.46.6 GitHub Actions run (34946428761): the
"Windows ws-mcp smoke" job failed `go test ./...` in `internal/mcp` while the
build+publish job succeeded and published the release. The failing tests are all
from the 260910 worktree-pool surface, which had never run on Windows CI before
0.46.6 (worktree route landed after the prior release marker). Linux runs
(worker review rounds, ship pre-flight) all passed, so the gap was cross-platform
only.

## Findings

- **Production (real): inconsistent path-separator form in `res.Path`.**
  `listWorktrees` parses `git worktree list --porcelain`, and git emits worktree
  paths with forward slashes even on Windows (`C:/Users/.../wt`). A reused
  worktree's `res.Path` therefore came back forward-slashed, while a freshly
  created one (`filepath.Join(poolRoot, stem)`) came back back-slashed
  (`C:\Users\...\wt`). Callers comparing the two forms break.
  Deterministically reproduced on native Windows (`TestProvisionWorktreeReuseIdle`,
  `TestProvisionWorktreeHygieneResetToBase`).
- **Test (portability): `TestResolvePoolRoot` used Unix-absolute fixtures.**
  Fixtures like `/main` and `/abs/pool` are not absolute on Windows (no volume),
  so `filepath.IsAbs` is false and every case fell into the relative-join branch,
  doubling the root (`\main\main\...`). `resolvePoolRoot` itself is correct in
  production because git supplies a real volume-qualified root.
- **Test (environment): `TestProvisionWorktreeCreateNew` compared against a raw
  `t.TempDir()` root.** On the CI runner git's `--show-toplevel` canonicalizes
  the temp path (symlink/short-name) differently from the raw `t.TempDir()` value,
  so an under-pool path read as outside the pool. Did not reproduce locally
  (host-dependent temp canonicalization).

## Fix (landed this session)

- `worktree_tools.go` `listWorktrees`: `filepath.Clean` each parsed worktree path
  so git-sourced (reused) and `filepath.Join`-sourced (created) paths share one
  OS-native form and compare equal.
- `worktree_tools_test.go`:
  - `worktreeFixture` returns `canonicalRootForTest(...)` so the test root matches
    git's derived main-worktree path (fixes the CI-only CreateNew divergence and
    the `.git/info/exclude` read).
  - `TestResolvePoolRoot` builds OS-absolute roots from `t.TempDir()` instead of
    Unix literals, exercising the absolute and relative-join branches identically
    on every platform.
  - `TestProvisionWorktreeCreateNew` asserts `pathUnder(res.Pool, res.Path)` — the
    resolved pool the worktree is actually created under — rather than
    recomputing the pool from the test's own root form.

## Verification

- Native Windows via `powershell.exe` (go1.26.3 windows/amd64), source on a
  Windows-native FS copy: the four previously-failing worktree tests pass, on
  both Windows and Linux (WSL2). A full-package Windows run in the isolated copy
  additionally trips manifest/rsrc tests that resolve sibling `agents-plugin*`
  trees not present in the partial copy — unrelated to this change and green on a
  full checkout (CI).
- Authoritative full-Windows confirmation is the ws/wsflow 0.46.7 hotfix ship's
  `ws-mcp release` Windows smoke gate.
