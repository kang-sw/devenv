# Plan: whiteboard - "open questions" pill when open items are scrolled out of view

## Outcome

The user scrolls the whiteboard down and misses open questions above
(2026-11-07: "화이트보드 쪽에서 아래쪽으로 스크롤이 가 있을 때 위쪽의 열린
질문을 확인 못 하는 경우가 종종 있어서, 알림 팝업? 뱃지? 같은 게 떠있으면
유용할 거 같네요"). Confirmed on the board, items (75)-(78):

- (75) When unanswered items are outside the visible area of the board's
  scroll container, a floating pill shows at the top edge of that area:
  "↑ 열린 질문 N" (N = unanswered items above the view). Clicking it scrolls
  to the nearest one. When unanswered items are below the view, the same pill
  shows at the bottom edge with "↓". When every unanswered item is in view,
  no pill. "Unanswered" = items awaiting the user: open policy, open
  technical/announced (ack), and assumed items (the `card` elements with
  `id="item-N"` in the policy / assumed / ack templates). Visibility via
  `IntersectionObserver`, not per-scroll geometry.
- (76) Works on the disk board page (`internal/wswhiteboard/assets/page.tmpl`
  + its script) and on the daemon-served pages: the served board page and the
  session page's board column (`internal/wsdashboard/assets/dashboard.tmpl`
  `#bcol`, where the board region is swapped in live by `dashboard.js`, so the
  observer must re-attach after each board swap and use the board column as
  its scroll root when that column scrolls on its own, the window otherwise).
- (78a) The browser tab title gets a `(N) ` prefix with the total count of
  unanswered items when N > 0, and no prefix when 0; updates live on the
  served pages after each board swap. On the session page the base title is
  the existing `<title>` (`<rid> - ws dashboard`).
- Use existing CSS variables; light and dark themes; the pill must not cover
  the composer or the reply bar, and must not shift layout.

## Settled constraints

- Files: `internal/wswhiteboard/assets/` (page.tmpl, page.css, the page
  script) and `internal/wsdashboard/assets/` (dashboard.js, dashboard.tmpl,
  dashboard.css) plus their tests. Another delegate is editing
  `dashboard.js` (composer image markers, Tab key) on a parallel branch: keep
  your dashboard.js change small and self-contained (one function + a call
  after board swap) so the merge is easy.
- No server-side or protocol change; the counts already in the page (top
  `counts` chips) may be reused but the pill counts above/below separately.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/board-open-question-pill` only; do not merge or
push. Return to the lead if this needs a server change.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`
(update pinned markup expectations that change). `node --check` on changed
scripts. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright): a board with open items at the top
and bottom and long settled content between; scroll to the middle -> both
pills with correct counts; click scrolls to the item; all in view -> no pill;
tab title prefix; light and dark; for the session page, render it with a
board in the column (split layout) and confirm the pill tracks the column's
scroll and survives a board swap. Report what was not checked.
