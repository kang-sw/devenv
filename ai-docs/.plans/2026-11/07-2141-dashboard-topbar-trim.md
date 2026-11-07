# Plan: trim the session page top bar

Branch `delegate/dashboard-topbar-trim` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

## Decision (user, 2026-11-07)

The session page's top bar is short on space. In
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` (line 8):

- The back link `<a class="back" href="/" title="all sessions">{{icon
  "back"}}dashboard</a>` keeps only the back icon; the word `dashboard` goes.
  Keep the `title` tooltip and give the link an accessible name
  (`aria-label`), since it no longer has text.
- The `<h1>Whiteboard</h1>` heading is removed.

## Outcome

1. The two template changes above; adjust `dashboard.css` only where the
   removed text leaves broken spacing (for example a gap rule keyed on the
   h1), and check the narrow-screen header wrap rule still makes sense.
2. Update any Go test asserting the removed text
   (`grep -rn '>dashboard<\|Whiteboard</h1>' agents-plugin-tool/internal`).
3. Nothing else in the top bar changes.

## Verification

From `agents-plugin-tool/`: `go build ./...`,
`go test -count=1 ./internal/wsdashboard/`. If practical, a headless
Chromium screenshot of a session page fixture at a narrow (420px) and a wide
width (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`).

## Return

Changed files, test output, and whether the screenshot check ran.
