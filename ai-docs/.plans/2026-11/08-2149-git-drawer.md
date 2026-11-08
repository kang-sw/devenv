# Plan: git drawer on the session page

Branch `delegate/git-drawer` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford`), based on
develop. Commit here; do not merge or push. Do not run git stash.

Source: user decisions on the session whiteboard, 2026-11-08, items
(508)-(512), all confirmed; (512) a.

Read first: `ai-docs/manuals/shipped-surface-boundary.md` (the dashboard and
its strings ship downstream). Code: `agents-plugin-tool/internal/wsdashboard/`
- `gitbadge.go` (top-bar git badge, session root branch), `gitpush.go` (push
  popover + POST, Origin rules), `pageview_band.go` (composer status band and
  its per-session git badges / `sync` template), `assets/dashboard.tmpl`
  (`sync`, git badge, `gitpop` defines; drawer handles and drawers),
  `dashboard.js`, `dashboard.css`, `pages.go`, i18n tables in
  `agents-plugin-tool/internal/wsi18n/tables/`.
- Drawer machinery: the existing handles row (tracker handles, the `≡` host
  handle) and the team-notes drawer, whose layout (wide left area + right
  panel) this drawer copies.

## Change

1. **Remove** the git badges from the composer status band (the per-session
   branch/sync chips) and the top-bar git badge. Their information moves to
   the drawer handle and drawer below. Remove now-dead templates, Go fields,
   JS, CSS and i18n keys (keys_test requires every en key to be used; delete
   a key removed from en.json from every table and list removed keys).
2. **Git drawer handle**, left of the `≡` handle in the handles row: an
   abbreviated branch label. `impl/<…>/<stem>` shows a hammer icon + `<stem>`,
   `goal/<…>/<stem>` a gold trophy icon + `<stem>`; a branch that is both
   (`goal/…/impl/…`) shows both icons with the impl stem. Other branches show
   the branch name (ellipsized). Hover tooltip: full branch name, and for an
   impl/goal stem that resolves to a ticket, the ticket title, looked up on
   the daemon from the stem (reuse whatever ticket lookup the daemon already
   has; if none, read the ticket file's title from the session root's
   `ai-docs/tickets/*/<stem>.md` front matter / first heading, bounded and
   cached). Ahead/behind marks stay visible on the handle compactly (↑n ↓m).
   Detached HEAD: short hash.
3. **Git drawer**: layout like the team-notes drawer.
   - Right panel: (a) changed-file list with status per file, grouped staged
     / unstaged (and untracked), each with +/- line delta (`git diff
     --numstat`, `--cached`), no diff view (512 a); (b) the push action,
     moving the existing push popover's function into this panel (508);
     (c) **pull**: fast-forward only; refuse with a clear reason when the
     tree is dirty or the branch has diverged (509); (d) **fetch** (510).
     Each action shows busy state and its error inline. Same Origin rules and
     session-root scoping as the existing push POST; reuse gitpush.go's
     structure for the new POSTs.
   - Left: a wide placeholder area reserved for a future git graph (a muted
     one-line note, i18n key). No commit action anywhere.
   - Data loads when the drawer opens and after each action; the handle
     label refreshes with the page's existing git refresh cadence.
4. Tests: Go tests for the status/numstat read (temp repo with staged,
   unstaged, untracked), pull refusal on dirty and diverged, fetch/pull
   success against a local bare remote, the handle label abbreviation
   (impl, goal, goal+impl, plain, detached), the ticket-title lookup, and
   template tests (`pages_test.go` style) that the band and top bar no
   longer carry git badges. `node --check` on dashboard.js.
5. Screenshot check with headless Chromium (cached Playwright under
   `~/.npm/_npx/*/node_modules/playwright`) against a dev-mode daemon on a
   spare port; never open a drawer on the user's daemon (port 29417). Shots
   in `/tmp/git-drawer/`: handle on an impl branch, open drawer with
   changes, a refused pull message, narrow width.

## Constraints

- i18n: new keys in en.json and ko.json only; others fall back to English.
- Keep the whiteboard embed size budget test passing.
- Concise strings; icons with tooltips over chrome text.
- Other delegates are concurrently editing tracker.js (member rows) and the
  whiteboard/tracker command parsers; stay out of those files.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsdashboard/ ./internal/wsi18n/ && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/ ./cmd/ws-mcp/`

## Return

Commit hashes, changed files, test output, screenshots, removed/added i18n
keys, and anything left open.
