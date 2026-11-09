# Whiteboard open-item pill: split drafted answers and notes out of the open count

## Request

The user (2026-11-09, on the whiteboard): the floating "N open" pill
(`agents-plugin-tool/internal/wswhiteboard/assets/pill.js`, i18n key
`js.pill.open`, ko "미결 {0}") still counts items the user has already
checked or answered in the page form, and items they wrote a note on. They
want it to read like "미결 N / 응답 M (노트 L)".

## Outcome

- Each awaiting item the pill tracks (`.card.policy`, `.card.assumed`,
  `.arow` with `id^="item-"`) is classified from the page form's current
  draft state (`form.js`, the same state `build()` reads into the reply):
  - responded: the user picked an option / ticked the ack, or wrote a
    non-empty item note (a note alone is a reply segment, `(nn) note: ...`);
  - noted: a non-empty item note (a subset of responded);
  - open: neither.
- Per edge the pill shows `미결 N / 응답 M (노트 L)`, omitting zero parts:
  only `미결 N` when nothing is drafted, only `응답 M (노트 L)` when every
  item at that edge is drafted, and no `(노트 L)` when L is 0. English and
  every other dashboard i18n table get the matching strings (new keys as
  needed; every table holds every en key, see
  `TestOtherTablesHoldEveryEnglishKey`).
- Clicking the pill scrolls to the nearest still-open item at that edge,
  falling back to the nearest drafted one when none is open.
- The tab-title prefix `(n)` counts only still-open items.
- Counts refresh live when the user picks, ticks, or types a note (listen
  on the same `input`/`change` events form.js uses), and the existing
  teardown (`st.off`) removes those listeners too.
- Topic notes (`.tnote`) are not items and do not count.

## Boundaries

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- Embed page behaviour (pill disabled) unchanged; the reply grammar in
  form.js unchanged.
- Verification: `node --check` on edited JS; `go test ./internal/wswhiteboard/
  ./internal/wsi18n/ ./internal/wsdashboard/` (i18n table tests). There is no
  JS harness: reason through the cases (no drafts, mixed, all drafted, note
  only) in the commit's AI Context and say the browser check was not done.
- Commit on branch `wb-pill-answered-count` in this worktree; do not merge
  or push. Never run `git stash`.
