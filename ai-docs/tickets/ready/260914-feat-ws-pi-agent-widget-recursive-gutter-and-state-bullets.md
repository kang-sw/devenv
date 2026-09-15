---
title: "Live agent widget: state bullets, activity time, drop count heading and ctx label"
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: split-out; the recursive nested-gutter tree needs cross-process subtree data this ticket does not build
  260909-feat-ws-pi-agent-count-panel-header: superseded — this removes the count heading it added
  260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner: coordinate state bullets with the existing 330ms emphasis
  260909-feat-ws-pi-agent-row-model-and-usage: shares the same rows this touches (model/tokens/USD)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dda9caa86520a977
sage-review-completeness-reviewed: dda9caa86520a977
---

# Live agent widget: state bullets, activity time, drop count heading and ctx label

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts (primary), agents-plugin-pi/src/audit.ts (reuses agent-widget.ts's exported row/state helpers) |
| scope.surface | public-interface | agent-widget.ts exports AgentRow, AGENT_STATE_LABEL, formatContextTokens, classifyRegistryRowState, rowName — imported/reused by audit.ts:28 |
| scope.new_public_symbol | yes | a bullet-glyph-by-state helper; exact name unfixed |
| scope.new_type_contract | yes | AgentRow gains a per-row activity-time field and loses the standalone ctx display use |
| scope.test_surface | existing | test/agent-widget.test.ts and test/audit.test.ts cover buildAgentRows/buildWidgetLines/buildHeadingLine with fake registries |
| complexity.reuse_points | confirmed | lastActivityAt (spawner.ts:2557) and formatContextTokens (agent-widget.ts:279-281) are reused as-is for activity-time / ctx-removal; state classification already exists via classifyRegistryRowState |
| complexity.side_effect_risk | low | buildAgentRows/buildWidgetLines are pure; changes are local rendering (no process-boundary or lifecycle wiring — the recursive-tree half was split out) |
| risk.correctness | low | row sort/format logic is well-tested; this ticket only adds a bullet column, an activity-time field, and removes two labels |
| risk.fit | low | stays a TUI-only rendering polish within agent-widget.ts/audit.ts; the architecture-touching recursive tree is out of scope |
| risk.test | low | the pure-function fake-registry test pattern extends cleanly for bullet/heading/ctx/activity assertions |
| risk.security_or_contract | low | purely local TUI rendering; no external security boundary or MCP contract change |

## Background

Owner dogfooding request against the live agent panel (surface (a),
`agent-widget.ts` — not the `/audit` picker and not the transcript `▌` bar). The
panel today renders a flat list of subagent rows with a `ws: N agents` heading
and a `ctx Xk` label per row. The owner wants each row's state to read from its
bullet at a glance, the count heading dropped, activity time surfaced, and the
`ctx` label removed.

The recursive nested-gutter tree the owner also wants is split into
`260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter` because it
needs cross-process descendant data the current architecture does not expose
(see that ticket). This ticket is the immediately-implementable polish that
needs no new data source.

## Evidence

- `lastActivityAt(record)` already exists (`spawner.ts:2557`) and is used by the
  audit picker as "last active 5m ago"; it is not shown on the live panel rows.
- The `ctx` label is `formatContextTokens()` (`agent-widget.ts:279-281`, e.g.
  `"ctx 12.3k"`), reused verbatim by the audit picker.
- The `ws: N agents` heading was moved into the widget's first line by shipped
  ticket `260909-feat-ws-pi-agent-count-panel-header`.
- Row state is already classified: `AgentRowState` (`agent-widget.ts:64`) is a
  six-value union — `awaiting-owner`, `idle-awaiting-owner`, `awaiting-approval`,
  `waiting-on-children`, `pending-delivery`, `running` — produced by
  `classifyRegistryRowState` (`agent-widget.ts:143-151`); a settled client
  returns `undefined` and is dropped from the row list (`agent-widget.ts:175`).
  So a bullet-by-state column reads entirely from an existing signal, no new
  data or state detection needed.
- Why the recursive tree is NOT here: `RpcAgentRegistry` is a per-process
  `Map<string, RpcAgentRecord>` (`spawner.ts:713`, instantiated fresh in
  `registerAgentTools`, `spawner.ts:3471`), so a worker's own spawned children
  are invisible to the lead; the only descendant signal crossing a process
  boundary is the aggregate boolean `waitingOnChildren` (`spawner.ts:463`).

## Decisions

- **Drop the count heading.** Remove the `ws: N agents` heading marker — with
  visible rows the owner can count by eye. This intentionally supersedes the
  shipped `260909-feat-ws-pi-agent-count-panel-header` behavior.
- **State bullets.** Prefix each subagent row with a bullet keyed to its real
  `AgentRowState` (`agent-widget.ts:64`), produced by `classifyRegistryRowState`
  (`agent-widget.ts:143-151`). Those six values are the only reachable states —
  there is no completed/error/spawning state, and a settled client returns
  `undefined` and is dropped from the row list (`agent-widget.ts:175`), so it
  needs no bullet. Mapping:
  - `running` → yellow mid-dot (`·`)
  - `awaiting-approval` (ws-execute) → red triangle (`▲`)
  - `awaiting-owner` / `idle-awaiting-owner` (owner-held) → keep the existing
    330ms flashing attention cue (owned by 260908); add a static filled dot
    (`●`) that participates in that flash so the bullet column stays uniform
  - `waiting-on-children` → dim cyan hollow dot (`◦`)
  - `pending-delivery` → blue right-triangle (`▸`)
- **Last-activity time.** Show `lastActivityAt` per row on the live panel.
- **Drop the `ctx` label.** Remove the `ctx Xk` context-token text from the rows.
- **Live panel only.** These changes apply to the live agent panel only, not the
  `/audit` picker. Picker treatment rides the split nested-gutter ticket.

## Constraints

- Coordinate the new state bullets with shipped
  `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner`. As currently
  shipped, the 330ms bold/plain toggle applies only to owner-wait rows
  (`awaiting-owner`/`idle-awaiting-owner` — `isAttentionState`,
  `agent-widget.ts:272-276`, whose own comment reads "Approval waits remain
  actionable lead-agent work through `ws-approve`, not owner work. Only
  owner-held `/answer` paths receive the loud cue."); approval rows are
  explicitly excluded from that toggle and render with ordinary (non-bold)
  styling (`test/agent-widget.test.ts:231-249`), carrying only the static
  `awaiting approval` label (`AGENT_STATE_LABEL["awaiting-approval"]`,
  `agent-widget.ts:116`). So the two do not fight; the approval bullet must layer
  with the existing label/emphasis, not duplicate or override it.
- The rows also carry model/effort/tokens/USD from shipped
  `260909-feat-ws-pi-agent-row-model-and-usage`; removing `ctx` must not disturb
  those fields.

## Phases

### Phase 1: State bullets, activity time, drop count heading and ctx label

In `agent-widget.ts` (live panel only): (1) prefix each row with the state
bullet from the mapping in Decisions, reading `classifyRegistryRowState`;
(2) remove the `ws: N agents` heading line; (3) render `lastActivityAt` per row;
(4) drop the `ctx Xk` text (`formatContextTokens`) from the row. The
`awaiting-owner`/`idle-awaiting-owner` static `●` must layer under the shipped
330ms flash (260908) without altering its toggle, and removing `ctx` must leave
the model/tokens/USD fields (260909) intact. No `/audit` picker change.

Verification: extend `agents-plugin-pi/test/agent-widget.test.ts`
(fake-registry, pure `buildAgentRows`/`buildWidgetLines`) to assert the bullet
glyph per state, the absence of the count heading, the presence of the
activity-time field, the absence of the `ctx` text, and that approval/owner rows
keep the existing 330ms styling behavior.

## Open Questions

- None blocking. Final color codes may be tuned to the active theme's palette
  during implementation.
