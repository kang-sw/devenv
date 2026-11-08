# Plan: balance the session top bar's git badge

Branch `delegate/git-badge-balance` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford`), based on
develop. Commit here; do not merge or push. Do not run git stash.

## Why

User (2026-11-08, screenshot): the top-bar git badge (`#gitb`, template
`r-git` in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`
~L98, styles `assets/dashboard.css` L60-65) looks unbalanced. The badge is a
28px pill with 10px side padding; the ahead/behind chip inside it
(`.sb-sync`, L346, shared with the status band) is its own 20px pill with a
muted background and 8px padding. So the right end of the badge shows a
nested pill with 4px vertical inset against 10px horizontal padding, plus
the chip's own inner padding: heavy and lopsided next to the branch name.

## Change (lead's technical call)

Inside `.gitb` only (the status band's `.sb-sync` stays as it is), flatten
the sync chip: no background, no box-shadow, no padding, no fixed height, so
the badge reads as one line of text `⑂ develop ↑1` with equal 10px insets
on both ends. Keep the colors that carry meaning (`.up` primary, `.dn`
warn, `.ok` green ✓, `.none` muted text without the outline ring) and the
mono numbers. Check the same for `.sb-diff` (`~2 +1 ?3`), which is already
flat. Also check vertical alignment of icon, branch text, and counts on one
baseline/center, and that the gap between branch and counts matches the
badge's own gap (8px). Scope: CSS in `dashboard.css` (and the template only
if a class hook is missing).

## Verification

- `cd agents-plugin-tool && go test -count=1 ./internal/wsdashboard/`
- A headless screenshot before and after (Playwright cached under
  `~/.npm/_npx/*/node_modules/playwright`; a daemon built from the worktree
  with an isolated cache/config, or a static HTML harness that loads
  `dashboard.css` with the badge markup for the states ok-ahead,
  ok-ahead+behind, in-sync ✓, no-upstream, dirty tree). Save the images to
  `/tmp/git-badge-balance/` and list them in the report.

## Return

Commit hash, the CSS diff summary, screenshot paths, test output.
