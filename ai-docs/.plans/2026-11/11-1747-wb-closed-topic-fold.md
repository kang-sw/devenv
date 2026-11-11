# Dashboard: a topic's content folds at the moment it closes

## Outcome

User request (2026-11-11 whiteboard): when a whiteboard topic is closed, its
content should fold by itself at that moment, and after that the dashboard
must not touch it again (the user may open or close it freely).

Today the server already renders a closed topic's body (`details.tdoc`,
`data-key="doc:<name>"`) and settled rows folded
(`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`). The dashboard
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`) keeps the
user's open/closed choices in `openState` keyed by `data-key` (toggle
listener near the "open details" section, about L99-125) and re-applies them
after each board swap through `restoreOpen` (called from the board morph,
about L448/461). The keys are the same whether the topic is open or closed,
so a body or settled fold the user once opened stays open after the topic
closes.

Fix: in `morphBoard`, when an incoming topic section has the `closed` class
and the live section it replaces does not (an open -> closed transition seen
in this page's lifetime), forget the `openState` entries for every
`details[data-key]` inside that topic (both the live and incoming nodes'
keys) before `restoreOpen` runs, so the server's folded markup wins once.
Later toggles are recorded and restored as today. A topic that reopens
(closed -> open) keeps today's behaviour. Topics first seen already closed
(page load, closed pages fetched on request) are unchanged.

## Constraints

- Surgical: dashboard.js only, plus a test. A peer session edits topic-note
  code in dashboard.js around L770-810 on the main checkout; stay out of that
  block.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (the
  asset ships).
- Never run go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C <worktree>/agents-plugin-tool -m 20 -- test -count=1 ./internal/wsdashboard/...`.
- Look for the existing node-run board-morph tests in wsdashboard and add a
  case there: a topic whose body the user opened, then a swap that closes the
  topic -> body folded; then the user opens it and another swap (closed topic
  unchanged or changed) -> stays open.

## Verification

- wsdashboard tests pass with the new case actually run under node (no SKIP).
- `node --check dashboard.js`.

## Return

Commit on `delegate/wb-closed-topic-fold` (this plan rides the commit). Do
not merge or push. Report the commit, change summary, verification output,
and anything unverified.
