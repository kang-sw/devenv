# Plan: whiteboard topic bodies mark their last edit with thin change bars

Branch `delegate/topic-body-change-bars` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (everything
here ships under `agents-plugin-tool/`). UI strings go through the i18n tables
(`internal/wsi18n/tables/*.json`, coverage checked by
`internal/wsdashboard/i18n_test.go`).

## Request (user dogfood, 2026-11-08)

"topic content 업데이트한 게 눈에 잘 안 띄어서, 새로 업데이트하거나 편집된
부분을 git diff viewer처럼 파란색(편집) 내지 녹색(신규) 좌측 띠로 강조."
Follow-up: "아주 얇은 선으로요. `>` 같은 인용 라벨이랑 혼선 안 되게."

## Decisions (user, board (378) ok, (379) a with note)

1. **Block-level change marks** ((378)). On every body change of a topic
   (`wb:topic` replacing the body, `wb:edit` on it), the server compares the
   previous and the new body at block granularity (paragraph, list item,
   heading, table, code block / mermaid fence, raw HTML block) and records,
   per topic, which blocks of the new body are new and which are modified,
   plus where blocks were deleted. Block granularity, not words, because a bar
   must attach stably to rendered markdown, HTML, and diagrams. Store the
   result in the topic's session state (whatever form is simplest, for
   example a per-block "changed in revision N" stamp or the previous body);
   it must survive a daemon restart like the body itself.
2. **Scope of the highlight: the topic's latest body edit only, per topic**
   ((379) a + note). No browser read state, no localStorage, no "mark read".
   The marks show the blocks the most recent body edit of that topic
   changed, and stay until that same topic's body is edited again.
   - Only `wb:topic` / `wb:edit` on that topic update its marks. Anything else
     (adding, answering, or patching items, even in the same topic; agenda,
     todo, ticket, mailbox writes; closing or reopening the topic; another
     topic's edits; a re-render) leaves them untouched.
   - A first body (topic had none) marks nothing, or everything as new; pick
     one and note it (lean: nothing, since the whole topic is new anyway).
   - A `wb:topic` with an identical body is no change and keeps the marks.
   - Best effort: cosmetic only; a block-matching heuristic that is
     occasionally off is acceptable. Never fail an update because of it.
3. **Look** (user): a very thin bar (1-2px) in a gutter outside the body's
   text column, so it cannot be confused with a `>` blockquote's left border
   (different position and colour). Green = new block, blue = modified block,
   a small red tick at a deletion point. Light and dark via existing theme
   variables. A small "changed" chip on the topic header is optional; skip it
   if it adds noise.
4. **Where:** wherever topic bodies render: the standalone full whiteboard
   page and the dashboard session page (live and board-first). Bodies render
   inside the `doc-frame` iframe (`wswhiteboard/assets/doc-frame.js`,
   `render.go`); the marks must work there. The embed snapshot may skip it if
   it threatens `TestRenderPageEmbedSizeBudget`; note the choice.
   Region swaps (SSE re-render) must keep the marks correct with no duplicate
   listeners.

## Verification

- `cd agents-plugin-tool && go build ./... && go vet ./internal/wswhiteboard/ ./internal/wsdashboard/ && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/wsi18n/...`.
- Unit tests for the block diff (new / modified / deleted / unchanged, list
  items, fenced code, raw HTML block) and for the scope rule: an item add or
  answer in the same topic keeps the marks; a second body edit replaces them;
  an identical `wb:topic` keeps them; marks survive reload of the session
  state.
- Headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`): a topic edited twice, showing
  only the second edit's bars; a blockquote next to a marked block to show
  they read differently; light and dark; standalone page and dashboard page;
  no console errors. Screenshots to `/tmp/topic-body-change-bars/`.

## Return

Commits, changed files, test output, screenshot paths, choices made under 2
and 4, and anything left open.
