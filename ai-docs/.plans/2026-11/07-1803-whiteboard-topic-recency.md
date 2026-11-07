# Plan: whiteboard - recently active topics rise within their rank

## Outcome

User (2026-11-07, whiteboard): "지금 아까 띄워준 개발 쇼케이스가 다른 열린
질문이 새로 올라왔는데도 자꾸 위에 머무르고 있는데 최근에 갱신된 토픽들을
끌어올리는? 그런 유형의 정렬 로직이 있는 게 좋겠어요."

Today `internal/wswhiteboard/view.go` (around lines 346-371) ranks topics by
what they hold (open policy 0, assumed 1, open ack 2, expanded settled 3,
quiet 4) and breaks ties by first appearance (the topic's oldest item). So an
older topic with open items stays above a newer topic with open items.

Change: keep the rank as the primary key; within one rank, order topics by
their most recent activity, newest first. A topic's activity is the highest
`Changed` sequence among its items (the per-item change stamp the store already
bumps on add, answer, and patch). Ties (equal activity) keep the current stable
order.

## Settled constraints

- Files: `internal/wswhiteboard/view.go` and its tests (add a test: two topics
  with open policy items, the older topic untouched and the newer topic
  updated later -> newer topic first; then answering an item in the older
  topic raises it above the newer one within the same rank). Update pinned
  ordering expectations in `internal/wswhiteboard/` or `internal/mcp/` tests
  only where the new rule explains the change.
- If `Changed` is not bumped on some mutation that should count as activity
  (add, answer, patch), report it rather than changing the store.
- No template, CSS, or protocol change. The rank itself is unchanged.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).
  If the whiteboard playbook under `agents-plugin/rsrc/` or
  `ai-docs/manuals/ws-mcp.md` states the topic order, update that sentence
  and its wsflow mirror per `ai-docs/manuals/wsflow-mirroring.md`.

## Boundary

Commit on branch `delegate/whiteboard-topic-recency` only; do not merge or
push.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/mcp/ ./internal/wsrsrc/`.
