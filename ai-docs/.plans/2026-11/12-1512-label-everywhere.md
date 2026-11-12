# A session's dashboard label shows wherever its mailbox id shows; the copied address names the label

## Goal

User feedback 2026-11-12 (board, two screenshots):

1. The conversation view's mail rows show the peer as its bare reply id
   (`← id:theaterbotchsubatomicpolar: ...`, `→ id:autisticcavalierharpistdairy: ...`).
   When that peer session has a dashboard label (the rename pencil, 4b0a95db7:
   daemon-stored `label.json` in the session folder, `label_route.go`), the row
   should show the label instead.
2. Clicking the header session name copies `(mailbox) id:<rid>`. When the
   session has a label, copy `(mailbox, label "<label>") id:<rid>`, for example
   `(mailbox, label "메인 세션") id:camcordersenoritahandgunpatchwork`.

## Decisions

- Wherever the dashboard renders a peer session's mailbox reply id as a name
  (conversation mail rows; also the whiteboard mailbox panel and any other
  place you find that shows `id:<rid>` for a session the daemon knows), show
  that session's label when it has one, and keep the full `id:<rid>` in the
  tooltip (title) so it stays discoverable. No label: unchanged.
- The label is looked up by reply id from the daemon's session folders at
  render time, so a rename shows on the next render/stream update without
  rewriting stored mail. A label for a reply id the daemon does not host is
  simply absent (show the id).
- Copy text with a label: `(mailbox, label "<label>") id:<rid>`; a `"` or `\`
  inside the label is backslash-escaped. Without a label: unchanged
  `(mailbox) id:<rid>` (`wsmailbox.ReplyIDPrefix`).
- Dashboard/daemon render and asset change only: compatibility tier T0. Do
  not change wsagent / wsholder / wslive (peer-owned) — if the mail rows'
  names can only be resolved there, stop and report instead.

## Scope and verification

- `agents-plugin-tool/internal/wsdashboard` (pageview / pages / label lookup,
  `assets/dashboard.js` copy handler), `internal/wswhiteboard` only if the
  mailbox panel renders there.
- Tests: Go tests that a mail row (and the mailbox panel, if covered) shows a
  labelled peer's label with the id in the title, an unlabelled peer's id,
  and that the label escapes HTML; a node or Go test for the copy text with
  and without a label (including quote escaping).
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  mail rows with a labelled and an unlabelled peer, light and dark;
  screenshots into scratchpad/label-everywhere/.

## Constraints

- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wswhiteboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/label-everywhere` in this worktree; do not merge
  or push.
