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
