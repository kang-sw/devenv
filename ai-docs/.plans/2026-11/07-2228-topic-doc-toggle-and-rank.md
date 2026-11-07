# Plan: topic body toggle beside the title, and body changes rank like open acks

Branch `delegate/topic-doc-toggle-inline` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

## Decisions (user, 2026-11-07)

1. **Toggle beside the title.** The topic body toggle (the `book` icon
   `<summary>` of `<details class="tdoc">` in
   `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`, template
   `doc`) currently takes a full row under the topic bar. It becomes a small
   button on the topic bar, right of the title (`.tname`), with the body still
   opening below the bar. User: "이거 버튼이 full 1행을 차지하는 게 좀 맘에
   안 들긴 합니다. 이건 타이틀 오른쪽에 붙이는 버튼으로 하죠".
   The open/closed state must keep surviving dashboard region swaps the way
   it does today (`data-key="doc:<name>"`), and the per-topic frame height
   kept by `wsdashboard/assets/dashboard.js` must keep working. The embed's
   "new since previous embed" dot stays on the button. Mechanism (CSS
   positioning of the existing summary, or a button in `.tbar` driving the
   details) is your call; the static full page and the embed must work
   without the dashboard script where they do today.
2. **A changed body ranks like open acks** (board (225) c). In
   `wswhiteboard/view.go` `BuildView`'s `rank`, a topic whose body changed
   and that the user has not answered since ranks 2 (same as open
   acknowledgement items): below open policy questions and assumed items,
   above settled/expanded topics. Recency still breaks ties within a rank.
3. **"Not answered since" = the body's change is newer than the board's most
   recent `wb:answer`** (board (226), lead technical decision). Any answer on
   any item counts as the user having seen the board. A single-update window
   was rejected because implicit renders (todo/agenda/ticket) bump `Seq` and
   would drop it at once; no expiry was rejected because the topic would sit
   above settled topics forever. If the store has no answer sequence today,
   add the minimal state needed (for example the render sequence of the last
   applied `wb:answer`), defaulting sensibly for stored boards that predate it.

## Outcome

- Template/CSS change for the toggle; `view.go` rank change plus whatever
  state the answer sequence needs.
- Tests in `wswhiteboard` for the new rank rule (body changed after the last
  answer ranks above a settled topic and below an open policy topic; an
  answer after the change drops it back) and for the toggle markup sitting
  in the topic bar. Update existing tests that assert the old markup.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/...`,
`go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/`, and
`go test -count=1 ./internal/mcp/` (about 5 minutes) if you touch anything it
renders. `node --check` on any changed script. A headless Chromium screenshot
of a static full page with a topic body, light and dark, at 420px and wide
(cached Playwright under `~/.npm/_npx/*/node_modules/playwright`).

## Return

Changed files, test output, screenshot paths, and the mechanism chosen.
