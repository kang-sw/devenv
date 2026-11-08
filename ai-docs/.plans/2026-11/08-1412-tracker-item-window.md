# Plan: tracker mockup item window - property card, assignee fold, two-column layout, hierarchical fixtures

Branch `delegate/tracker-item-window` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

The reference mockup `ai-docs/ref/tracker-mockup/` (README is the data and UI
contract, `SCENARIOS.md` the walk) and its Go port
`agents-plugin-tool/internal/wstracker` (golden parity through
`testdata/gen-golden.js` and `golden_test.go`). Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## User feedback (board 2026-11-08, screenshot of the "Security training" item window)

User: "status, kind, holders 등 프로퍼티 창은 카드 안에 들어가던지 하여간에
본문하고 시각적으로 좀 구별되게 ... holders도 쭉 이어지게 되어있는데";
"assignee도 접고 펼치는 식으로, 위쪽부터 계층에서 높은대로 4명 정도를 최대
렌더링하고 더보기로 노출하기"; "실제 중요한건 status, 서브태스크, 스레드";
"목업 자체에 계층형 태스크 예시 데이터가 없어서 서브태스크 관련해서는
구체적인 피드백을 못 하겠네요".

## Decisions (confirmed on the board)

1. **Property card** ((424) ok): the properties (status, kind, holders,
   creator, parent, project, id, dates, due) sit in a card visually distinct
   from the body. Inside the card, holders is one summary line instead of the
   full list, e.g. "13 · doing 3 · done 1" (the Assignees section lists the
   people).
2. **Assignee fold** ((425) ok): Assignees shows at most 4 rows, ordered
   highest in the hierarchy first (shallowest lead-chain depth first; a
   stable tie-break of your choice, noted), then a "N more" toggle that
   expands the rest. Provenance text and the close rule that are identical
   across rows ("from Ada Park (forced: their lead chain)", "assignee
   closes") are written once in the section header; a row shows only its
   name, status, and note, plus whatever differs from the common header (for
   example Pia Novak's "inherited: everyone within L2, reached later").
3. **Layout** ((426) c): two columns on wide windows. Main column: header
   (title, status chips, actions), body, subtasks, thread with the composer.
   Side column (narrow): the property card and Assignees. Below a width
   threshold of your choice (note it) it falls back to one column in the
   order header -> property card -> body -> subtasks -> Assignees -> thread +
   composer. Status, subtasks, and thread are the prominent parts.
4. **Subtasks section**: the main column gets a subtasks section listing the
   item's children (status icon, title, holders summary or avatars), each
   clickable to open that item; the parent shows as a breadcrumb or link in
   the header. Keep it minimal - the user will give detailed feedback once
   it is visible with data.
5. **Hierarchical fixtures** ((427) ok, user note: "깊이는 섞어가면서요. 자식이
   없는 태스크도 중간중간 섞여서 볼 수 있게"): add hierarchical tasks to the
   fixtures with mixed depth - e.g. a parent with 2-3 subtasks, one of which
   has its own subtask (depth 3), another parent with a single child, and
   childless tasks interleaved among them; mixed statuses. Use the engine's
   existing parent / split-into-subtask events as the README defines them;
   if the engine lacks something needed, stop and report rather than
   inventing a protocol-layer change in `wsledger`. Keep existing ids stable
   so scenarios and goldens line up; regenerate the goldens from the changed
   fixtures and keep the Go fold matching them.

Keep these unchanged: the notification focus, the recently-closed fold,
search, the icon tabs, the member profile display rules (merges 2db82f6c2,
23b2eb7ff).

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
  at 900, 1280, and 1760px.
- `cd agents-plugin-tool && go build ./... && go vet ./internal/wstracker/ && go test -count=1 ./internal/wstracker/` with regenerated goldens.
- Screenshots to `/tmp/tracker-item-window/`: the "Security training" item
  (13 assignees: folded and expanded) wide and narrow; a parent task with
  subtasks; a depth-3 subtask showing its parent link; the Items list showing
  the hierarchy mixed with childless tasks; light and dark.
- README: update the item-window layout text and add Settled rows for
  (424), (425), (426) c, (427); SCENARIOS.md: add a step that opens a parent
  and walks into a subtask.

## Return

Commit hash, changed files, screenshot paths, test output, the choices made
(tie-break, width threshold, subtasks rendering), and anything left open.
