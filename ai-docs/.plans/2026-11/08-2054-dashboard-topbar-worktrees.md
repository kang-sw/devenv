# Plan: dashboard top bar trim and a pooled-worktree card

Branch `delegate/dashboard-topbar-worktrees` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash.

Source: user request from the dashboard, 2026-11-08 (screenshot of the
session page top bar: `devenv` project name, `✓ 1` chip, green `● 실시간`
SSE indicator, git branch chip, search, gear, then the drawer handles).

Read first: `ai-docs/manuals/shipped-surface-boundary.md` (the dashboard and
its strings ship downstream). Code: `agents-plugin-tool/internal/wsdashboard/`
(`assets/dashboard.tmpl`, `dashboard.js`, `dashboard.css`, `pages.go`,
`inbox_routes.go`), worktree pool logic in `agents-plugin-tool/internal/mcp/`
(`worktree_lease.go`, `worktree_nudge.go`, `worktree_remove.go`), i18n tables
in `agents-plugin-tool/internal/wsi18n/tables/`.

## Concurrency note (important)

Another delegate (branch `delegate/tracker-interaction`) is currently
rewriting the drawer handle row: removing every chevron from the handles and
collapsing the host handle's three icons (flag, columns, mail) into one
hamburger `≡` button. Work in two parts so you do not collide with it:

- **Part A (now):** everything below except the drawer card UI.
- **Part B (after the lead's go message):** the lead will message you once
  that branch lands on develop. Then `git merge develop` into your branch
  (resolve conflicts yourself; they should be small) and build the two cards
  inside the new `≡` host drawer. Until that message, stop after Part A and
  report Part A as done (commit hashes, tests); stay available.

## Part A

1. **Live indicator into the harness badge.** Remove the text label of the
   SSE indicator (`js.sse.live`, "실시간") on the session page. Render the
   live/offline dot inside the harness badge (`.host`, e.g. "claude") as a
   small LED, keeping the indicator's tooltip and its live/offline state
   switching in `dashboard.js`. When a page has no harness badge (the index
   page, a session without a harness), keep the bare dot without text. Drop
   the `js.sse.live` key from every table if nothing uses it any more
   (keys_test requires every en key to be used).
2. **Project name to the left end.** In `r-head`, move `.ProjectName`
   (the repository folder name, e.g. `devenv`) to the far left, directly
   before the reply id.
3. **Worktree pool read on the daemon.** A GET endpoint for the session
   (`/s/<reply-id>/worktrees`, follow the existing route style) returning the
   pool of the session's project: the pool path (the `worktree_pool` knob,
   default `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`), and per pooled
   worktree its name, path, profile, branch or detached, leased or idle,
   lease fields (worker key, parent key, acquired_at), last-activity time,
   and disk size. Reuse the lease and nudge logic rather than re-deriving
   it: if `wsdashboard` cannot import `internal/mcp` cleanly, extract the
   pure pool-reading part into a small package both use. Disk size can be
   slow: compute it off the request path (async with a short cache, or a
   separate size request) so the card opens instantly and sizes fill in.
   Read-only: the dashboard never removes or releases a worktree.
4. **Cleanup request.** A POST endpoint that puts one message into the
   session's whiteboard inbox through the same path the page's composer uses
   (so the adapter injects it as the user's own message): a header line from
   an i18n key in the page language, meaning "The user asks to clean up
   unused worktrees.", followed by a plain-text dump of the current pool
   state (the same data as item 3, sizes included when known). Same Origin
   rules as the other POSTs. Refuse when the session has no live adapter.
5. Tests: Go tests for the pool read (a temp repo with a pool dir, a leased
   and an idle worktree), the cleanup POST landing one inbox message with the
   dump, and the template changes (`pages_test.go` style). `node --check` on
   `dashboard.js`.

## Part B (after the go message)

6. **Settings card.** The gear menu's content (layout, input, theme,
   language) becomes one card in the session page's `≡` host drawer, and the
   gear button leaves the session page's top bar. The index page has no host
   drawer: keep its gear there.
7. **Pooled worktrees card** in the same drawer, loading item 3's data when
   the card opens: leased worktrees listed first and shown active (branch,
   holder key, how long held); idle ones grouped by profile with a count,
   and per worktree its size and how long unused. One "정리 요청" button
   (i18n key) that calls item 4, then closes the `≡` drawer. Disabled with a
   tooltip when the session has no live adapter.
8. Screenshot check with headless Chromium (cached Playwright under
   `~/.npm/_npx/*/node_modules/playwright`) against a dev-mode daemon on a
   spare port; never open the drawer on the user's daemon (port 29417),
   which marks the user's badges seen. Put shots in
   `/tmp/topbar-worktrees/`.

## Constraints

- i18n: add new keys to `en.json` and `ko.json` only; other tables fall
  back to English and get a later translation pass. Do not delete keys from
  non-en tables except a key you removed from `en.json`; list removed keys in
  the report.
- Keep the whiteboard embed size budget test passing.
- Strings shown to the user: concise; no layout chrome text where an icon
  with a tooltip suffices.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsdashboard/ ./internal/wsi18n/` and `go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/`, plus `./internal/mcp/` when you touch the worktree code there, plus any package you extract into.

## Return

Per part: commit hashes, changed files, test output, screenshots (Part B),
removed i18n keys, and anything left open.
