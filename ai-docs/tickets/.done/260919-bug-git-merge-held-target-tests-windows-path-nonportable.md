---
title: "git.merge held-target tests assert git-reported paths against filepath-built paths, failing on Windows"
---

# git.merge held-target tests assert git-reported paths against filepath-built paths, failing on Windows

## Background

The `ws-mcp release` workflow for `v0.46.12` (run 35362767513) shipped: the
**Build ws-mcp assets** job succeeded and published the GitHub release with all
seven platform assets. The **Windows ws-mcp smoke** job failed on
`internal/mcp` `go test`, with three subtests:

- `TestImplMergeRefusesTargetHeldElsewhere/pool-holder` (git_merge_test.go:978)
- `TestImplMergeRefusesTargetHeldElsewhere/plain-holder` (git_merge_test.go:998)
- `TestImplMergeDispatchClassifiesHeldTargetByPool/plain-holder` (git_merge_test.go:1120)

## Diagnosis: test-only, not a runtime defect

The shipped binary behaves correctly on Windows. In every failing case the
diagnostic's **classification text is correct** — pool-holder reads
`a ws pool worktree: a parallel lead's housekeeping checkout`, plain-holder
reads `another worktree` — so the runtime `checkTargetHeld` / pool-vs-plain
selection (`pathUnder`, `canonicalGitRoot`) worked on Windows. The failures are
in the tests' path assertions, e.g.
`strings.Contains(d.Reason, held)` at git_merge_test.go:978/998, where `held`
is a Go `filepath.Join`-built path but `d.Reason` embeds the path as
`git worktree list --porcelain` reported it. On Windows those two forms diverge
(separator and/or temp-dir short-path / symlink resolution under
`C:\Users\runneradmin\AppData\Local\Temp`), so the substring check fails
although the path is the same location. macOS/Linux pass because the two forms
coincide there.

Why it reached release: the 260918 held-target tests and the ec6acd22 dispatch
test were only run on macOS locally and in the review sweep; the Windows smoke
is the first cross-platform exercise and caught it post-publish.

## Impact

- No shipped-behavior defect. `v0.46.12` is functionally correct on Windows.
- The release run's CI is red on the Windows smoke job; develop's next Windows
  smoke will stay red until the assertions are made portable.

## Fix direction (test-only)

Normalize both sides before the substring/containment check — compare the
diagnostic's embedded path and the expected `held` path through one
canonical form (e.g. `filepath.ToSlash` on both, or resolve both with the same
`canonicalGitRoot`/`filepath.EvalSymlinks` the production path took) — in the
three assertions above and any sibling that embeds a worktree path
(pool-holder, plain-holder, dispatch pool-holder/plain-holder, prunable if it
grows a path assertion). Do not change `git_merge.go` runtime behavior; the
classification is already correct. Verify on macOS and, per
`ai-docs/manuals/ws-mcp.md` / the Windows smoke host note, on Windows
(`ssh ki608@192.168.33.6`) before closing.

## Notes

- Constraint reminder: `agents-plugin-tool/internal/mcp/` edits are governed by
  `ai-docs/manuals/ws-mcp.md`.
- Process follow-up (not this ticket): the review sweep and local verification
  were macOS-only; consider whether behavior tests that embed OS paths should
  carry a portability guard. Capture separately if worth a rule.

### Result (2026-09-19)

Root cause confirmed test-only, matching the diagnosis above: `listWorktrees`
already returns each entry's path through git's own resolution (a
`git rev-parse --show-toplevel`-equivalent canonical form; on macOS this
resolves the `/var -> /private/var` symlink), and `d.Reason` embeds that
form verbatim. The four affected assertions instead compared it against the
raw `filepath.Join`-built `held`/`held` local. On macOS/Linux the raw form
happened to be a trailing substring of git's resolved form (`/private/var/...`
contains literal `/var/...`), so the substring check passed by coincidence;
on Windows the two forms diverge and the check fails even though the location
is the same.

Fix (`agents-plugin-tool/internal/mcp/git_merge_test.go`, test-only, no
`git_merge.go` or shipped-string change):

- `TestImplMergeRefusesTargetHeldElsewhere/pool-holder` and `/plain-holder`:
  introduced `heldCanonical := canonicalRootForTest(t, held)` (the same
  `git rev-parse --show-toplevel` helper `mkHeld` already uses one directory
  level up to derive `poolRoot`) and asserted `d.Reason` contains
  `heldCanonical` instead of the raw `held`.
- `TestImplMergeDispatchClassifiesHeldTargetByPool/pool-holder` and
  `/plain-holder`: same `canonicalRootForTest(t, held)` treatment before the
  `strings.Contains(out, ...)` checks. `pool-holder` wasn't in the CI failure
  list but embeds the same raw-path pattern, so it was fixed alongside the
  named `plain-holder` case per the ticket's "any sibling" instruction.

Verification:

- macOS: targeted subtests
  (`TestImplMergeRefusesTargetHeldElsewhere|TestImplMergeDispatchClassifiesHeldTargetByPool`)
  green; full `go test ./...` in `agents-plugin-tool` green (14 packages, all
  `ok`).
- Native Windows (`ki608@192.168.33.6`, `C:\Program Files\Go\bin\go.exe`):
  pushed `develop` (`186a87f7`) to origin, fetched it into a scratch
  `git worktree add` off the host's existing `devenv-win-verify` clone
  (avoided touching that clone's own unrelated dirty working tree), ran the
  same targeted subtests there. All pass, including the three originally
  Windows-failing subtests (`TestImplMergeRefusesTargetHeldElsewhere/pool-holder`,
  `/plain-holder`, `TestImplMergeDispatchClassifiesHeldTargetByPool/plain-holder`)
  and the two additional siblings touched (`pool-holder` dispatch case,
  `no-other-worktree-merges`/`prunable-holder` which needed no change). Scratch
  worktree removed afterward.
- Commit: `186a87f73abdd2dbe2ef01a2dacfa1a3f8fc77a7` on `develop`
  (`test(mcp): canonicalize held-worktree path assertions for Windows
  portability`).
