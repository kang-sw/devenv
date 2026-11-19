---
title: "대화 감사(audit): 도구 호출·숨은 이벤트 수집, 저장, 조회, 대시보드 뷰"
sage-review-design: required
parent: 261114-epic-ws-common-session-foundation
related:
  261119-epic-ws-daemon-sqlite-storage: "storage dependency: the audit store child builds on this epic's SQLite precedent and settled shapes"
  261114-feat-ws-conversation-audit-event-store: "superseded (dropped): its still-valid decisions are carried into children 1-4"
  261114-feat-ws-dashboard-conversation-audit-view: "superseded (dropped): its still-valid decisions and approved mockup are carried into child 5"
  261119-feat-ws-dashboard-shell-topbar-rail-redesign: "landed shell redesign; the audit view's mockup surroundings realign to it"
---

# Conversation audit

## Scope

The owner wants to audit what a hosted conversation actually did and saw:
each tool call with its input and output, and the events a harness streams
but the dashboard hides (hooks, system reminders, thinking, the compaction
summary, system events, subagent traffic). This epic owns that outcome end
to end: a common audit record, its capture from every hosted harness, its
storage, a query API, and a dashboard view.

Owner decision **(1331)=a** (2026-11-19) created this epic under Stage 4 of
`261114-epic-ws-common-session-foundation` and split the two earlier tickets,
`261114-feat-ws-conversation-audit-event-store` (backend) and
`261114-feat-ws-dashboard-conversation-audit-view` (UI), into the five
children below. Both earlier tickets are dropped as superseded. Their
still-valid decisions are carried into the children with their original
board IDs, so approvals 545, 546, 550 and 594-611, 681 keep their authority.

## Non-Scope

- No API proxy to capture the system prompt ((546)=a).
- No MCP tool surface for audit data now. One may follow later as an additive
  T0 tool.
- The display ring (`wslive` ring, `/history`) stays a lossy display view.
  This epic does not widen its caps or turn it into the audit store.
- No change to approval powers or session identity.

## Child Tickets

Order and dependencies:

1. `261119-feat-ws-audit-policy-event-envelope`: audit policy (what is
   captured, for which sessions) and the common event envelope. No
   prerequisite.
2. `261119-feat-ws-audit-capture-harness-adapters`: a capture point ahead
   of the holder ring-skip filter, plus per-harness adapters in the order
   Claude -> Codex -> agy -> Pi, and the ring call ID. Blocked by child 1.
3. `261119-feat-ws-audit-sqlite-store`: SQLite audit store, redaction before
   persist, retention. Blocked by child 1. Depends on
   `261119-epic-ws-daemon-sqlite-storage` for its SQLite precedent and shapes.
4. `261119-feat-ws-audit-query-api`: read/search API over the store. Blocked
   by child 3.
5. `261119-feat-ws-dashboard-audit-view`: dashboard audit view on the
   approved mockup, realigned to the current shell. Blocked by child 4, and by
   child 2's Phase 1 for the ring call ID.

Children 2 and 3 can proceed in parallel after child 1.

## Cross-Child Decisions

Carried from the superseded tickets (owner answers recorded 2026-11-14:
`594..609,611=ok`; (610) `ok`; (550)=ok; (545)=a; (546)=a) unless marked
otherwise.

- **The daemon owns the audit record ((545)=a).** The daemon keeps tool
  inputs/outputs and hidden harness events in its own store, not in the
  harness's own transcript file. How much is captured is re-asked as (1340)
  in child 1. (545)=a stands until the owner answers it.
- **One harness-neutral record ((546)=a).** Capture is what the harness JSON
  stream delivers, mapped onto one envelope (id, session, harness, time, kind,
  call linkage). No per-harness schema, no API proxy.
- **Redact before persist ((596)).** Nothing reaches the store unredacted.
  Redaction is best-effort regex (`wsredact`). How secrets are handled beyond
  that is (1344) in child 3.
- **The display ring is not the audit store.** The ring is capped and lossy
  by design. The audit store sits beside it.
- **All audit data reaches the UI through the query API ((599)).** The view
  parses no transcript file and adds no route of its own.
- **History-to-audit join is by call ID ((601)), never by time window.** Rows
  without a call ID keep a fallback display ((602)). UTC bounds only select
  hidden-event ranges ((681)).
- **Compatibility tier T0.** New store, new routes, an optional ring field;
  existing routes, ring readability, MCP tools and tracker events keep their
  semantics. The audit store carries its own schema version from day one.
- **Harness order** follows the parent epic (25): Claude, Codex, agy, Pi.
- **Release-gate file-touch rule** (parent epic, owner 2026-11-17) applies to
  capture and storage: append not rewrite, no per-event O(N) rereads, and
  interactive paths never wait on file I/O.

## Verified Findings

From the codex.large read-only research, 2026-11-19 (cite as such; code
coordinates are evidence, not edit targets):

- All four harnesses (Claude, Codex, agy, Pi) already flow through the same
  `wsagent` event path (`agents-plugin-tool/internal/wsagent/stream.go:35-38,169-199`).
- Tool call IDs and full args/results exist briefly but are dropped when
  converted for `wslive` display. Only previews survive
  (`stream.go:436-463,535-577`; `wslive/toolsummary.go:9-15`).
- The holder filters Claude tool-input/thinking deltas and agy thought chunks
  before they reach the daemon (`wsagent/holder.go:139-141,228-246`, via
  the holder's `RingSkip`). Full capture must tap before that filter.
- The display ring is intentionally lossy and unusable as an audit store.
  Redaction is regex best-effort (`wsredact/redact.go:1-9`).
- Stale in the old backend ticket: the adapter order Claude -> Pi -> Codex
  (all four adapters exist now); per-session JSONL plus a separate index
  (overlaps the display-ring JSONL and the SQLite epic); an oversized Phase 1.
- Stale in the old UI ticket: a monolithic `dashboard.js` and a standalone
  `audit.js` (the dashboard now assembles ordered fragment files); mockup
  surroundings that predate the topbar/rail redesign.

## Open Decision Queue Index

Owner questions are not decided. Each lives in its child's
`## Open Decision Queue`. Board IDs (1340)-(1347), next free (1348):

- (1340) capture scope; (1341) target sessions:
  `261119-feat-ws-audit-policy-event-envelope`
- (1342) capture point; (1346) agy compaction estimate:
  `261119-feat-ws-audit-capture-harness-adapters`
- (1343) retention period; (1344) secret handling; (1345) DB layout;
  (1347) tool-result body amount: `261119-feat-ws-audit-sqlite-store`

## Completion Criteria

- Done: all five children are in `.done/` or explicitly deferred/dropped by
  the owner, and the owner closes this epic.
- Dropped: the owner drops the audit direction.
- Deferred: an MCP audit tool, and any capture scope or session coverage
  beyond what (1340)/(1341) settle, stay proposals.
