# Plan: tracker member rows - keys and profile edit behind a details button

Branch `delegate/tracker-member-details` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash.

Source: user feedback from the dashboard, 2026-11-08 (screenshot of the
tracker team/org card): every member row prints its full SSH public key line
(`ssh-ed25519 AAAA...`, overflowing the card) and a "프로필 편집" button,
which makes the tree long and noisy. The user asked for these to show only
after pressing a details button such as `…`.

Code: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
(`cardTeam`, the `node()` row builder around line 1040), `tracker.css`; the
tracker mockup under `ai-docs/` shares tracker.js through its adapter (check
it still renders). Read `ai-docs/manuals/shipped-surface-boundary.md` first.

## Change

1. Member row shows avatar, org, name, id and chips only. Add one small
   icon-only `…` button (tooltip i18n key, e.g. "Details") at the row end.
2. Pressing it toggles an inline details block under that row (not a modal):
   each key on its own line, monospace, shortened in the middle
   (`ssh-ed25519 AAAAC3Nz…last8`) with the full line in the tooltip and a copy
   button per key reusing the existing id copy button's pattern; revoked keys
   (the existing revoked chip data) listed there too, marked; the "프로필
   편집" button moves into this block, keeping its existing visibility rule
   (self or admin). Expanded state per member survives re-renders within the
   page (keep it in the existing `ui` state like other toggles).
3. Keep the revoked chip on the row (it is a warning); just drop the key text
   from the chip label if it carries the key, and show it in details instead.
4. Summary line above the tree: drop the raw `(attachments.max_bytes N)`
   parenthetical from `js.tk.team.summary`, keeping the human size. Update the
   key's args in en.json and ko.json; other tables: update only if the same
   placeholder index change would otherwise break them (keys_test / i18n
   tests decide); report what you touched.

## Constraints

- i18n: new keys in en.json and ko.json only.
- No Go behavior change expected; if a Go test asserts the key text in the
  rendered team card, update it.

## Verification

`cd agents-plugin-tool && gofmt -l . && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/`, `node --check` on tracker.js. A headless Chromium screenshot (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`) of the team card collapsed and with one member expanded, against the mockup or a dev daemon on a spare port; never port 29417. Shots in `/tmp/tracker-member-details/`.

## Return

Commit hash, changed files, test output, screenshot paths, i18n keys added/changed.
