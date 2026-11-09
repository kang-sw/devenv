# Whiteboard open-item pill: items already sent count as answered

## Request

The user (2026-11-09): the floating open-item pill
(`agents-plugin-tool/internal/wswhiteboard/assets/pill.js`, reading
"미결 N / 응답 M (노트 L)" since commit 1a0e6a96b) still counts as open the
items the user already sent from the dashboard when the agent forgot to
record them with `wb:answer`. It is visually distracting.

## Outcome (whiteboard (70), technical)

- An item the dashboard marks as sent or held (`applySent()` in
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` ~L965 adds
  the class `sent-sent` or `sent-held` to the board item and a `.sentmark`
  in its head) is not open: it counts toward 응답 M, like an item with a
  drafted answer. No new segment. If it also carries a draft note, the note
  count rules from 1a0e6a96b still apply; do not count it twice.
- The pill recomputes when those marks change: `applySent()` runs after
  board and convlive region swaps, so the pill must notice the class
  change (for example a MutationObserver on class attributes of the tracked
  items, or a small hook the dashboard calls after `applySent()`; pick the
  simpler one that survives region swaps, and keep the existing teardown
  removing it).
- Clicking still goes to the nearest open item, otherwise the nearest
  drafted/sent one. The tab title `(n)` counts open items only, so sent
  items drop out of it too.
- A standalone whiteboard page (no dashboard, no sent marks) behaves as now.

## Boundaries

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- Check first how the pill is mounted inside the dashboard board region
  (`wswhiteboard/render.go` embeds pill.js) so the class names it reads match
  what `applySent()` writes.
- Verification: `node --check` on edited JS; `go test ./internal/wswhiteboard/
  ./internal/wsdashboard/ ./internal/wsi18n/`. No JS harness: reason through
  no marks, a sent item without draft, a sent item with a draft, a held item,
  and a region swap in the commit's AI Context; state that the browser check
  was not done.
- Commit on branch `wb-pill-sent` in this worktree; do not merge or push.
  Never run `git stash`.
