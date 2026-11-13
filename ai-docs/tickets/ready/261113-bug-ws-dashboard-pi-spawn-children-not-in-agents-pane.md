---
title: "pi.spawn children show in the dashboard agents pane as ordinary subagents"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 052b2eb101e5027d
sage-review-completeness-reviewed: 052b2eb101e5027d
---

# pi.spawn children show in the dashboard agents pane as ordinary subagents

## Background

Owner dogfood 2026-11-13: after turning on `dashboard.pi_spawn_enabled`, Pi
agents do not appear in a session page's agents pane ("에이전트", running
count, subagent tree).

Read-only investigation (2026-11-13):

- The pane is server-rendered from `SessionView.Tree`
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L43-L85`); the
  subagent rows come from `agentTree(in.Snapshot.Agents, lookup)` and the
  running/done/failed counts from the same `Snapshot.Agents`
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L511-L521`); the
  snapshot cache can serve an in-memory live snapshot instead of the lead's
  `snapshot.json` (`agents-plugin-tool/internal/wsdashboard/snapshot_cache.go#L65-L77`).
- `Snapshot.Agents` receives harness feed entries, including the Claude
  mod's Agent-tool subagents (`agents-plugin/hooks/claude-mod/dashboard.ts`)
  and the Pi harness's registry subtree (`agents-plugin-pi/src/dashboard-feed.ts`
  `dashboardAgentEntries`), as well as daemon-sourced Pi rows. The registry
  entries do not set `agentKind`; the daemon's Pi row sets `agentKind: "pi"`
  (`agents-plugin-pi/src/dashboard-feed.ts#L326-L350`,
  `agents-plugin-tool/internal/mcp/pi_live.go#L58-L84`).
- `pi.spawn` children are owned by the daemon's spawn tree
  (`agents-plugin-tool/internal/wsagent/children.go`) and attached by the
  spawning key's reply id (`ParentLead.ReplyID`,
  `agents-plugin-tool/internal/mcp/pi_tools.go#L154-L173`). An existing
  daemon feed (`piChanged`/`piSeed`) reports direct children only into the
  MCP server's live sessions, which `dashboard.intake` creates
  (`agents-plugin-tool/internal/mcp/pi_live.go#L19-L55`); it does not
  project `ParentConversation` grandchildren
  (`agents-plugin-tool/internal/wsagent/children.go#L305-L316`).
- Owner clarification 2026-11-13: a Pi session the owner launches as a
  dashboard-hosted Pi shows its children (its native registry reaches the
  runner over a separate widget channel); the missing rows are the direct
  `ws/pi.spawn` children of a lead such as a dashboard-hosted Claude lead.
  Cause, verified in code: hosted Claude deliberately disables the intake
  feed (`agents-plugin/hooks/claude-mod/register.ts#L1083-L1088`, test at
  `manual-engine.test.ts:1205`) and its displayed snapshot comes from the
  runner's own `liveWriter` keyed by `rec.WSKey`
  (`agents-plugin-tool/internal/wsagent/runner.go#L371-L378`,
  `record.go#L432-L440`); nothing projects daemon-spawned children into that
  writer. So direct children of a hosted Claude lead never reach its pane,
  independently of the grandchildren gap. The Windows PID/key mismatch is
  fixed on develop (`558807689`, `b1f700e08`); a stale install could still
  add an ordinary-key duplicate row, unverified.
- Separately, "피어 리드" lists only mailbox contacts of the viewing session
  (`pageview.go` `peerGroup`, contacts recorded on mail send/recv); a
  same-project lead with no mail exchanged is not listed. That is existing
  behaviour, out of scope here.

## Decisions

- Owner direction 2026-11-13: `pi.spawn` children merge into the ordinary
  subagent rows and ride the existing subagent render path (tree nesting,
  counts, status chips, row actions). The spawn-side logic is already mixed;
  do not build a separate Pi-only pane, row type, or render path. Rejected:
  a dedicated "Pi agents" section.
- Merge point: the daemon contributes the lead's spawn-tree children as
  `wslive.Agent` entries (nested by `ParentID`, recursive children under their
  spawner) to the data the page builds `Snapshot.Agents` from, so
  `agentTree` and the counts need no Pi branch. Where exactly the merge
  happens (snapshot write vs page build) is the implementer's call, stated
  in the commit; it must not double-count an agent that a harness feed
  already reports.
- Snapshot source (design review 2026-11-13, extends the prior
  per-connection feed decision): project the owned spawn-tree rows into the
  lead's authoritative snapshot source - the runner-owned state for a
  dashboard-hosted lead, the existing MCP live state for an intake-fed lead -
  without two competing writers for one snapshot. Do not re-enable hosted
  intake just to obtain the rows.
- Compatibility tier: T0 expected (dashboard render and an additive optional
  field at most); confirm at promotion, and treat a change to the snapshot's
  on-disk shape as a store change per `ai-docs/manuals/release-compatibility.md`.

## Constraints

- Manuals: `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`.
- Every new user-facing string goes into all `internal/wsi18n/tables/*.json`.
- Owner 2026-11-13: non-UI implementation runs through `ws/pi.*` workers.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261112-feat-ws-mcp-pi-spawn-claude-lead-path (2026-11-13, Decisions): "The parent lead's own per-connection server feeds the row" — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-13, Decisions): "Lead -> Pi worker -> Pi grandchild; a `pi.spawn` from a depth-2 child refuses" — bearing: constrains
- 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart (2026-11-13, Decisions): "Dev reruns and release replacements rejoin the same running turn." — bearing: constrains
- unavailable: wsflow/rationale.query was not exposed; these manually found pointers are not an exhaustive query.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/pi_live.go, agents-plugin-tool/internal/wsdashboard/pageview.go, agents-plugin-tool/internal/wsagent/children.go, agents-plugin-tool/internal/wsagent/runner.go (hosted lead's runner-owned liveWriter) |
| scope.surface | cross-module | Existing daemon feed, runner spawn tree, live state, and dashboard page construction |
| scope.new_public_symbol | unknown | No new exported symbol specified; merge location is delegated |
| scope.new_type_contract | unknown | An optional snapshot field is possible but not specified |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pageview_test.go and agents-plugin-tool/internal/mcp/pi_tools_test.go; new test files not specified |
| complexity.reuse_points | confirmed | agentTree and existing direct-child piChanged/piSeed feed inspected |
| complexity.side_effect_risk | moderate | Live snapshot updates and reconnect seeding affect displayed state |
| risk.correctness | high | Nested status, ownership isolation, reconnects, and duplicate counts must remain correct |
| risk.fit | high | Existing direct-child feed and holder recovery constrain where recursive projection fits |
| risk.test | moderate | Existing page and feed tests need recursive, deduplication, and live verification coverage |
| risk.security_or_contract | moderate | Projection must preserve lead ownership isolation and optional snapshot compatibility |

## Phases

### Phase 1: spawn-tree children in the agents pane

- A `pi.spawn` child of a lead, and a child it spawned, appear in that
  lead's agents pane as subagent rows with the right nesting, status, and
  running count, through the existing `agentTree` path.
- Tests: wsdashboard page-build test with a lead whose spawn tree holds a
  running child and a finished grandchild (rows, nesting, counts); no double
  row when a harness feed also names the agent.
- Tests: a dashboard-hosted lead (runner-owned snapshot, hosted key) whose
  `pi.spawn` direct child shows in its snapshot, then status transitions and
  recovery after reconnect/daemon replacement; extend
  `TestPiRowsFeedTheParentsLiveStateAndReplay`
  (`internal/mcp/pi_tools_test.go:474`) with grandchild updates, nesting,
  ownership isolation, and reset replay for the intake-fed path.
- Verification: `go test` for the touched packages on the remote build host;
  a live check (lead runs `pi.spawn`, pane shows the row) is the owner's.
