---
title: "Recursive subagent tracking: cross-process subtree propagation + nested gutter rendering"
related:
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: split-from; that ticket owns the flat-panel polish (bullets/activity/ctx/heading), this owns the tree
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
  260908-epic-ws-pi-subagent-conversation-view: coordination (not parent); Phase 3 re-points the `/audit` picker this epic's audit-window child owns onto the shared tree — must not break its viewer/openViewer path or the human-only-surface rule; this ticket honors the epic's "session_children stays out of boundary" decision
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5f5078312f800d5c
sage-review-completeness-reviewed: 5f5078312f800d5c
---

# Recursive subagent tracking: cross-process subtree propagation + nested gutter

## Background

The owner wants the live agent panel (and the `/audit` picker) to show
subagents as a nested, recursive tree — an agent's own spawned children
indented under it with a per-depth gutter — instead of a flat list. Split out
of `260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets` because,
unlike that ticket's flat-panel polish, the tree had **no data source today**.

With the elevated ticket-worker now acting as a spawning mini-lead, grandchild
agents (a worker's own leaves/reviewers) are a routine part of normal runs, so
the gutter's one-level blindness is now hit constantly, not rarely.

## The blocker this resolves

`RpcAgentRegistry` is a per-process `Map<string, RpcAgentRecord>`
(`spawner.ts`, `RpcAgentRegistry` type ~`:713`), instantiated fresh per
extension activation (`registerAgentTools`). Each OS process (lead, fork,
worker) sees only its own direct children; a worker's spawned grandchildren
live only in that worker's process-local registry, invisible to the lead. The
only descendant signal that crosses a process boundary today is the aggregate
boolean `record.waitingOnChildren` (`spawner.ts:~463`), computed by collapsing
the child's count-only `SubtreeSnapshot` (`subtree-lifecycle.ts`,
`{nonce, outstanding, active, deliveries, delegated, revision}`) to one boolean
via `subtreeWaiting` (`spawner.ts:2404-2408`). Counts, not itemized descendant
records — a tree cannot be built from them. This ticket adds the structured
identity propagation that makes the tree renderable, then renders it in both
consumers.

## Decisions

### D1 — Solve inside pi, on the existing subtree channel; do NOT use ws `session_children`

The tree is built from an extension to pi's own `SubtreeSnapshot`/`publishSubtree`
propagation, not from the ws-mcp `session.children` tool.

Rejected: consuming ws `session_children` (a cross-process shared on-disk
`keys/*.json` registry that already returns a depth-full identity tree).
Rejected because:

- It is **poll-only** (no push/subscription); the gutter must poll on a timer.
  pi's own subtree channel is already push (file written on each child event),
  so the gutter stays live for free.
- Its `live` field is **root-directory existence, not process liveness**
  (`session_auth.go`), so a finished-but-uncleaned grandchild reads `live:true`
  — the gutter would show dead agents as alive (a trust-breaking hard failure).
  pi's `RpcAgentRecord` knows real process liveness.
- It would grow a **shared ws-mcp contract** (consumed by Claude and Codex too)
  to serve a pi-UI-only need — a non-goal and needless Arch-Rule-4 surface
  pressure. Keeping it pi-internal is host-adapter-scoped (Arch Rule 3) and
  ships nothing downstream.
- Coverage note: `session_children` only sees session-minted (playbook/ferrule)
  descendants; pi's subtree channel rides the spawn spine and sees **every** pi
  spawn. The problem therefore inverts from "cannot see grandchildren" to "sees
  all of them → filter to interesting ones," a lighter problem handled at
  render time.

### D2 — Descendant identity is display-only advisory, decoupled from the authoritative counts

`SubtreeSnapshot` gains an advisory descendant-identity list; the existing count
fields (`outstanding`, `active`, `deliveries`, `delegated`, `revision`, `nonce`)
are **left byte-for-byte unchanged** and remain the *sole* input to wait/settle
(`subtreeWaiting`, and every `waitingOnChildren` consumer). The identity list is
never read by stop-decision logic.

Verified safe (this is the guardrail against regressing stop-decisions):
wait/settle reads only named count fields; there is no whole-snapshot
hash/serialize/equality anywhere; `revision` is a monotone dispatch counter and
`nonce` is the fixed channel nonce (neither content-derived); and
`readSubtreeSnapshot` validation checks named fields and **ignores extra
fields** rather than rejecting them. Adding the list cannot perturb settle.

### D3 — Structured identity requires an explicit per-hop merge (it is not free)

Propagation today is a **lossy per-hop collapse**: each child's whole snapshot
becomes one boolean at its parent (`spawner.ts:2404-2408`), and the parent's own
counts are recomputed from its *local* registry (`subtreeOutstanding`/
`publishSubtree`, `subtree-lifecycle.ts`). There is no existing carrier for
identity. To reach the top lead, each hop must **merge**: on reading a child's
snapshot (`spawner.ts:~2405`), pull that child's descendant list, re-tag depth,
concatenate with the local direct-children rows, and emit the merged list in
`publishSubtree` (`subtree-lifecycle.ts:~53`). Transport is a whole-object JSON
file overwrite with no size cap, so cost is `O(subtree)` re-serialize per child
event. pi subtrees are small (single digits, low-double-digit depth at most), so
this is acceptable; a depth/breadth guard bound is a cheap safety valve, not a
load-bearing requirement.

### D4 — One shared tree-builder, two renderers

A new exported tree-builder is added to `agent-widget.ts` (already the shared
row-primitive module the picker imports from — `audit.ts` pulls
`classifyRegistryRowState`, `rowName`, `AgentRowState`, `AGENT_STATE_RANK/LABEL`
from it). It produces tree nodes `{id, parentId, depth, role, state, live,
openable}` from the union of the local registry and the propagated descendant
list. `state` for a local row comes from `classifyRegistryRowState`; a
propagated (cross-process) row has no local registry record and carries only
`live`, so the tree-builder synthesizes its `state` from that `live` boolean
alone (deep rows are liveness-only, per Phase 2's verification boundary). Both
`buildAgentRows` (gutter, `agent-widget.ts:237`) and `buildAuditPickerItems`
(picker, `audit.ts:303`) are re-pointed to consume it.

Rejected: separate tree wiring per call site — duplicates the tree and lets the
two consumers diverge. The tree model is a **superset**; each renderer keeps its
own inclusion filter (the picker's registry-only read + "dormant" tier, the
gutter's owner-question `ThreadRecord` union) applied over the shared model.

### D5 — Picker shows cross-process grandchildren as non-openable context; opening them is out of scope

The gutter merely *displays* a row; the `/audit` picker *selects to open* a
conversation viewer. A cross-process grandchild's conversation stream lives in
the worker's process, so the lead's viewer likely cannot open it. Tree nodes
therefore carry an `openable` flag: the gutter ignores it; the picker renders
unreachable grandchildren as tree context with **selection disabled**.

Scope boundary (confirmed): making a grandchild actually openable
(cross-process audit conversation-stream reachability) is **out of scope** and
belongs to the subagent-conversation-view epic's cross-child work.

## Constraints

- **Wait/settle behavioral invariant.** The `SubtreeSnapshot` count fields and
  every consumer of `waitingOnChildren`/`subtreeWaiting` must be behaviorally
  unchanged; the descendant-identity list is additive-only. Existing subtree
  wait/settle tests must stay green with no assertion changes; that is the
  contract this ticket must not break.
- **No path-scoped manual obligation.** `agents-plugin-pi/` is not listed in
  AGENTS.md `### Implementation Conventions`; pi is a host adapter (Arch Rule 3),
  not shipped downstream text (Arch Rule 4), so shipped-surface-boundary,
  skill-authoring, and wsflow-mirroring do not apply. Do not import their rules;
  do not mirror this into `agents-plugin/` or `agents-plugin-wsflow/`.
- Code citations above are as-of-now evidence; anchor edits by function/type
  name (`publishSubtree`, `subtreeWaiting`, `buildAgentRows`,
  `buildAuditPickerItems`, `installSubtreePublisher`), not by line number.

## Prior Decisions

- 260916-feat-pi-agent-gutter-active-time-placement (2026-09-16, ticket Decisions): "Relocate the existing active-time entry from its separate trailing position; do not delete or replace that entry." — bearing: constrains
- 260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets (2026-09-15, commit 734d03e6): "Closing after both review rounds completed: round 1 non-clean (1 Important, fixed), round 2 clean." — bearing: supports
- 260913-bug-ws-pi-settled-agent-falsely-remains-running (2026-09-13, commit 6bafc76f): "Treat only running or streaming RPC work as autonomous execution; descendant waits and pending terminal admission remain protected but distinct lifecycle states." — bearing: constrains
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-13, ticket Result f7b3f670): "an explicit nested stop is its synchronous disposition and emits no redundant worker-local stopped follow-up; terminal report/exit obligations clear only when Pi accepts direct-parent enqueue" — bearing: supports
- 260914-subtree-propagation (2026-09-14, commit 9b46c059): "the recursive nested-gutter tree was split into 260914-subtree-propagation because RpcAgentRegistry is per-process and exposes no cross-boundary…" — bearing: supports
- (no ticket) (2026-09-14, commit 2b6c1984): "Kept in idea/ (not ready) because the cross-process descendant-identity mechanism is unbuilt; needs a design pass before it can be a phased actionable ticket." — bearing: supports
- 260908-feat-ws-pi-subagent-audit-window-and-owner-steering (2026-09-09, commit 33ae460e): "The picker's live tiers reuse agent-widget.ts's classifyRegistryRowState/ rowName/state-rank ordering verbatim; the picker's own dormant tier (registry-only, no ThreadRecord union" — bearing: supports
- 260905-feat-ws-pi-live-agent-widget (2026-09-05, commit ae03bcc2): "Wiring mirrors the existing leadIdleRef pattern so spawner.ts registry-transition points can trigger a re-render without either module importing agent-widget.ts (golden rule)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/subtree-lifecycle.ts, agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/audit.ts |
| scope.surface | cross-module | SubtreeSnapshot (subtree-lifecycle.ts#L8) and the new tree-builder (agent-widget.ts, D4) are consumed across spawner.ts, subtree-lifecycle.ts, agent-widget.ts, and audit.ts |
| scope.new_public_symbol | yes | advisory descendant-identity list added to SubtreeSnapshot (subtree-lifecycle.ts#L8) and the new exported tree-builder in agent-widget.ts (D4) |
| scope.new_type_contract | yes | descendant list shape {id, parentId, depth, role, live} (D1) and tree node shape {id, parentId, depth, role, state, live, openable} (D4) |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts, test/agent-widget.test.ts, test/audit.test.ts already cover the touched modules |
| complexity.reuse_points | confirmed | subtreeWaiting/publishSubtree/readSubtreeSnapshot (subtree-lifecycle.ts), buildAgentRows (agent-widget.ts#L237), buildAuditPickerItems (audit.ts#L303) all verified present at the cited sites |
| complexity.side_effect_risk | moderate | Phase 1 edits the exact wait/settle read site (spawner.ts#L2404-L2408) that sets record.waitingOnChildren, though D2 keeps the new list additive-only |
| risk.correctness | moderate | the per-hop merge must not perturb subtreeWaiting/waitingOnChildren, a site 260913-bug-ws-pi-settled-agent-falsely-remains-running recently hardened |
| risk.fit | low | reuses the existing subtree channel, RpcAgentRegistry, and the shared agent-widget.ts row-primitive module audit.ts already imports from |
| risk.test | moderate | 3-level-tree, cross-process-liveness, and guard-bound coverage does not exist yet; only count-based subtree tests (recursive-worker.test.ts) exist today |
| risk.security_or_contract | low | D1 keeps this off the shared ws-mcp session_children contract; no new cross-package public API is added |

## Phases

### Phase 1: Propagate structured descendant identity up the subtree

Intended behavior: extend `SubtreeSnapshot` with an advisory descendant list
`[{id, parentId, depth, role, live}]` alongside the untouched count fields, and
add the per-hop merge (D3) so the top-lead snapshot carries the full descendant
subtree with correct parent edges and depth. `live` reflects real process
liveness from the owning process's `RpcAgentRecord`. Apply a depth/breadth guard
bound.

Deferred scope: no rendering yet; no consumer reads the new list in this phase.

Verification boundary: existing wait/settle tests unchanged and green (D2
invariant); new tests assert (a) a grandchild appears in the root snapshot with
correct `parentId`/`depth`, (b) the count fields and `subtreeWaiting`/
`waitingOnChildren` outputs are identical to pre-change for representative
trees, (c) the guard bound truncates a pathological tree without corrupting
counts.

### Phase 2: Shared tree-builder + nested gutter render

Depends on Phase 1.

Intended behavior: add the exported tree-builder to `agent-widget.ts` (D4)
consuming the local registry ∪ the Phase-1 descendant list, producing
`{id, parentId, depth, role, state, live, openable}` nodes. Re-point
`buildAgentRows` to render the tree as a per-depth indented gutter, preserving
the existing per-row state bullets/activity for locally-owned rows and the
owner-question `ThreadRecord` union as a renderer-side filter over the shared
model.

Deferred scope: the picker is not yet re-pointed; `/audit` still flat.

Verification boundary: tests assert the gutter renders a 3-level tree with
correct indentation and parent grouping, deep rows show real liveness, and
non-tree inclusion (thread rows, filtering) is unchanged from the flat-panel
baseline.

### Phase 3: Audit picker consumes the shared tree (non-openable grandchildren)

Depends on Phase 2.

Intended behavior: re-point `buildAuditPickerItems` to consume the shared
tree-builder (D4), rendering the same tree in the `SelectList` overlay.
Cross-process grandchildren (`openable: false`) render as indented context with
selection disabled (D5); the picker's registry-only read and "dormant" tier stay
as renderer-side filters. Locally-openable agents remain selectable exactly as
today.

Deferred scope: opening a grandchild's conversation stream (cross-process
reachability) — out of scope per D5, deferred to the conversation-view epic.

Verification boundary: tests assert the picker lists the tree with correct
depth, an unreachable grandchild is present but non-selectable, a locally
openable agent still opens its viewer, and the dormant tier is preserved.
