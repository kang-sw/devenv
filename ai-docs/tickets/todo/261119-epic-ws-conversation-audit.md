---
title: "대화 감사(audit): 도구 호출·숨은 이벤트 수집, 저장, 조회, 대시보드 뷰"
sage-review-design: required
parent: 261114-epic-ws-common-session-foundation
related:
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
On 2026-11-21, owner decision (1960)=a superseded the SQLite storage choice
in (1331)=a. `ai-docs/manuals/performance.md` `## Storage Types` places audit
events in Type 1. Owner decision (1970)=a defines the ring capacity rules.

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
3. `261119-feat-ws-audit-ring-store`: per-session audit segment ring,
   redaction before persistence, and capacity and time retention. Blocked
   by child 1.
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
  harness's own transcript file. Scope is settled by (1730)=b: tool calls
  plus hidden events as assembled envelopes, `raw` only for `other`. Sessions
  are settled by (1731)=b: every daemon-hosted session, leads and
  `ws/agent.*` children.
- **One harness-neutral record ((546)=a).** Capture is what the harness JSON
  stream delivers, mapped onto one envelope (id, session, harness, time, kind,
  call linkage). No per-harness schema, no API proxy.
- **Redact before persist ((596)).** Nothing reaches the store unredacted.
  Redaction is best-effort regex (`wsredact`). User documentation describes
  the residual risk ((1734)=a). The byte caps, time retention and session
  deletion remove data. The owner rejected a purge action ((1734)=d).
- **The audit ring sits beside the display ring.** The display ring keeps a
  capped, lossy view. The audit ring holds the captured audit envelopes.
- **Capture scope and sessions ((1730)=b, (1731)=b).** Tool calls plus
  hidden events, as assembled envelopes (`raw` only for `other`), for every
  daemon-hosted session: leads and `ws/agent.*` children, each in its own
  audit record. Non-hosted CLI-tier sessions and a per-session switch are not
  provided.
- **Capture point ((1732)=a).** A holder-side tee on a separate, unsequenced,
  versioned audit channel. Skipped lines produced while no daemon is attached
  are lost to the audit.
- **agy compaction ((1736)=a).** The inferred compaction is the model-seen
  cut, labelled `meta.estimated: true`.
- **Storage ((1960)=a, (1735)=a).** Each session has its own append-only
  segment ring with an in-memory index. Session deletion removes its ring.
  Search scans that session's audit ring.
- **Capacity and retention ((1970)=a, (1733)=d, (1737)=b).** The 256 MiB
  per-session byte cap drops the oldest whole segment. The per-store total
  byte cap evicts whole audit rings in least-recently-used session order.
  Time retention expires rings 7 days after the session was last seen. All
  three are config knobs. The store child owns the open total cap default
  (2010) and the implementation choice of segment size. A separate per-body
  knob keeps head plus tail with a stored-truncation marker.
- **All audit data reaches the UI through the query API ((599)).** The view
  parses no transcript file and adds no route of its own.
- **History-to-audit join is by call ID ((601)), never by time window.** Rows
  without a call ID keep a fallback display ((602)). UTC bounds only select
  hidden-event ranges ((681)).
- **Compatibility tier T0.** New store, new routes, an optional ring field;
  existing routes, ring readability, MCP tools and tracker events keep their
  semantics. The audit store carries its own schema version from day one.
  - The holder's audit channel ((1732)=a) is an additive, version-negotiated
    holder-protocol capability, T0. It follows the parent epic's holder-version
    rule (versioned protocol, new daemon speaks the old holder protocol) and
    must not refuse an old peer, since that would be T3. An old holder only
    loses the skipped-only content.
  - New config keys ((1970)=a per-session and per-store byte caps, (1733)=d
    time retention, and (1737)=b per-body cap) are additive and T0.
    Where declared in a config manifest, `ws` and `wsflow` stay key-identical
    with only `default` differing, including the native-pi first-party and
    dashboard manifest copies.
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
  (all four adapters exist now) and an oversized Phase 1. The current ring
  layout and in-memory index follow (1960)=a.
- Stale in the old UI ticket: a monolithic `dashboard.js` and a standalone
  `audit.js` (the dashboard now assembles ordered fragment files); mockup
  surroundings that predate the topbar/rail redesign.

## Decision ID Index

The owner decisions from 2026-11-19 and 2026-11-21 are recorded under
`## Decisions` in the children named below, with their rejected alternatives.

- (1730)=b capture scope; (1731)=b target sessions:
  `261119-feat-ws-audit-policy-event-envelope`
- (1732)=a capture point; (1736)=a agy compaction estimate:
  `261119-feat-ws-audit-capture-harness-adapters`
- (1733)=d retention period; (1734)=a secret handling (purge (1734)=d
  rejected); (1735)=a one store per session; (1737)=b tool-result body amount;
  (1960)=a Type 1 storage; (1970)=a capacity rules:
  `261119-feat-ws-audit-ring-store`

The store child tracks the total byte cap default in (2010). The query API
child carries the approved read-only, per-session contract and tracks the
open response details in (2011) and (2012).

## Completion Criteria

- Done: all five children are in `.done/` or explicitly deferred/dropped by
  the owner, and the owner closes this epic.
- Dropped: the owner drops the audit direction.
- Deferred: an MCP audit tool, and any capture scope or session coverage
  beyond what (1730)=b/(1731)=b settle (raw stream lines, non-hosted CLI-tier
  sessions, a per-session on/off switch), a purge action, cross-session
  search and at-rest encryption, stay proposals.
