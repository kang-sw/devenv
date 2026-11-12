# Dashboard: per-session display label (rename) and click-to-copy mailbox address

## Goal

User request 2026-11-12 (board topic `session-label`, items (390)-(393)
acknowledged), with screenshots of the session header
`devenv camcordersenoritahandgunpatchwork · #556` and the rail rows:

- A small rename (pencil) icon to the right of the session name in the
  header lets the user set a display label for that session only. The label
  is stored by the daemon, not the browser.
- Clicking the shown name copies `(mailbox) id:<reply-id>` to the clipboard.
- The rail rows show the display label too.

## Decisions (settled; do not reopen)

- Where: header `r-head` in
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` (~line 90,
  the `<span class="meta">{{.ReplyID}}...` part), the rail row `rrow`
  (`<span class="rid">`, ~line 249) and the older `srow` list row (~233) if
  it is still rendered, and the browser tab title. Where a label is set it
  replaces the reply ID text; the reply ID stays visible in the tooltip.
- Rename UI: a pencil icon (existing lucide icon helper) right of the name;
  clicking it turns the name into an inline one-line input. Enter saves, Esc
  cancels, blur cancels (or saves - pick the one matching any existing
  inline-rename in the dashboard/tracker and say which). Empty value resets
  to the reply ID (deletes the label). Max 64 characters, trimmed, one line
  (newlines/control chars refused or stripped).
- Storage: the daemon stores the label in a small file in that session's
  directory (find where the daemon keeps per-session state; follow the
  existing small-file patterns there, atomic write). Written through a new
  POST route under the session (`/s/<reply-id>/...`) gated exactly like the
  other state-changing dashboard routes (the same Origin / Sec-Fetch gate and
  viewer permission check they use; read access.go and an existing POST route
  such as the answers or tracker write route and reuse their gate). No new
  auth model.
- Reads: the label flows into the page model for the header, the rail data
  (so other open dashboard tabs show it after their next rail refresh/swap),
  and the tab title.
- Copy: clicking the name copies `(mailbox) id:<reply-id>` using
  `wsmailbox.ReplyIDPrefix` ("id:") for the prefix on the Go side (pass the
  full address string to the page rather than hard-coding "id:" in JS), and
  shows the dashboard's existing toast ("복사됨" style text through i18n).
  Tooltip on the name: the reply ID plus a "click to copy" hint (Korean
  "누르면 복사"), localised in every i18n table like neighbouring keys.
- Display only: the mailbox address, reply ID, URLs, and every other use of
  the reply ID are unchanged.
- Compatibility tier T0: an additive daemon route, an optional session file,
  and dashboard assets. An older daemon without the route simply has no
  label; the page must not break when the route is missing (hide the pencil
  when the page model says renaming is unavailable, e.g. no live session dir).
- Hosted sessions (daemon-hosted agents) are owned by a peer lead; do not
  change wsagent/wsholder/wslive code. If hosted sessions need a different
  storage location, keep them working read-only (no pencil) and report it.

## Scope and verification

- Files (expected): the dashboard daemon route registration and a small
  label store under `agents-plugin-tool/internal/wsdashboard/` (+ tests),
  `dashboard.tmpl`, `dashboard.js`, `dashboard.css`, i18n tables
  (`agents-plugin-tool/internal/wsi18n/tables/*.json`).
- Go tests: the POST route saves/reset/validates (length, newline), refuses a
  foreign Origin / missing gate like the neighbouring routes (404/403 exactly
  as they do), and the label appears in the header/rail render; a missing
  file renders the reply ID.
- Node test (existing dashboard script harness) for the inline edit:
  Enter saves, Esc cancels, empty resets, and the copy string.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  header with pencil, editing state, labelled header and rail row, toast
  after copy; light and dark; shots into scratchpad/session-label/.

## Constraints

- Read `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Preserve: rail star/pin, approvals tint, revive buttons, header chips,
  stale-tab asset handling (stream tests skip the first "assets" event).
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/session-display-label` in this worktree; do not
  merge or push.
