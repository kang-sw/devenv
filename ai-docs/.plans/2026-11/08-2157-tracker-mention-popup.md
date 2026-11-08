# Plan: tracker @-mention suggestion popup

Branch `delegate/tracker-mention-popup` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash.

Source: user feedback from the dashboard, 2026-11-08 (screenshot of the item
window's comment composer with `@누구누구` typed and the `@` picker at the
bottom left): "a popup appearing like this would be more refined". Today the
`@` picker is a native `<select id="cmention">` that appends `@id` to the end
of the text and pushes the id to `ui.mdraft`.

Code: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` (composer
in the item window around `composer2` / `cbox` / `cmention`, the `change`
handler for `cmention`, `draftHtml`, `keptMentions`, the comment submit path
that builds `mentions`), `tracker.css`. The tracker mockup under
`ai-docs/ref/tracker-mockup/` shares tracker.js through its adapter. Read
`ai-docs/manuals/shipped-surface-boundary.md` first.

## Change

1. **Inline suggestion popup.** Typing `@` in the composer textarea (at the
   start or after whitespace) opens a small popup anchored at the caret
   (mirror-div caret measurement is fine), listing active (not removed)
   members filtered by the text typed after `@`: match on member id, display
   name, and profile given/family, case-insensitive, prefix matches first.
   Each row: avatar, display name, id (muted). Up/Down moves, Enter or Tab
   picks, Esc closes, click picks; typing whitespace or deleting the `@`
   closes. Picking replaces the `@query` token with `@<id> ` at the caret
   (not appended at the end) and records the id as a mention like today.
2. **Korean / CJK IME.** Filtering must follow composition: update on
   `input` including during composition (the user types Hangul names), and
   never let Enter that commits an IME composition pick a row or send the
   comment (`e.isComposing` / keyCode 229). Cmd/Ctrl+Enter still sends when
   the popup is closed; when open, Enter picks.
3. **The `@` button** at the bottom left becomes a plain icon button that
   inserts `@` at the caret and opens the same popup, replacing the native
   select. Keep its tooltip key.
4. **Mentions follow the text.** At send, mentions = ids picked in this draft
   whose `@id` is still in the text (same rule as `keptMentions`), so
   deleting a picked mention from the text drops it. The draft chip row for
   mentions can go if the text itself now shows them; keep attachments'
   draft chips.
5. If the item body editor / new-item body input already supports mentions
   through the same select, apply the same popup there; otherwise composer
   only.
6. No new Go behavior. i18n: new keys (e.g. "No matching member") in en.json
   and ko.json only; remove keys that become unused from every table and
   list them.

## Verification

`cd agents-plugin-tool && gofmt -l . && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/`, `node --check` on tracker.js. Headless Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`) against the tracker mockup or a dev daemon on a spare port, never port 29417: script typing `@ma`, ArrowDown, Enter and check the text and the sent mentions; a case inserting mid-text; shots of the open popup in light and dark into `/tmp/tracker-mention-popup/`. IME can't be fully simulated headless: dispatch compositionstart/update/end + an Enter keydown with isComposing=true and check it neither picks nor sends.

## Return

Commit hash, changed files, test output, screenshot paths, i18n keys added/removed, and what remains unverified.
