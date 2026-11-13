---
title: "pi.spawn children show in the dashboard agents pane as ordinary subagents"
---

# pi.spawn children show in the dashboard agents pane as ordinary subagents

## Background

Owner dogfood 2026-11-13: after turning on `dashboard.pi_spawn_enabled`, Pi
agents do not appear in a session page's agents pane ("에이전트", running
count, subagent tree).

Read-only investigation (2026-11-13):

- The pane is server-rendered from `SessionView.Tree`
  (`agents-plugin-tool/internal/wsdashboard/pageview.go:153-160`); the
  subagent rows come from `agentTree(in.Snapshot.Agents)` (`pageview.go`
  ~579-625) and the running/done/failed counts from the same
  `Snapshot.Agents`, all read from the lead's own `snapshot.json`.
- `Snapshot.Agents` is fed only by the lead harness's own feed: the Claude
  mod's Agent-tool subagents (`agents-plugin/hooks/claude-mod/dashboard.ts`)
  or the Pi harness's registry subtree
  (`agents-plugin-pi/src/dashboard-feed.ts` `dashboardAgentEntries`, tagged
  `agentKind: "pi"`, `wslive/state.go` ~519-561).
- `pi.spawn` children are owned by the daemon's spawn tree
  (`internal/wsagent/children.go`); no feed reports them into the parent
  lead's snapshot, so a Claude lead's `pi.spawn` children (and their own
  recursive children) never reach its pane.
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
- Compatibility tier: T0 expected (dashboard render and an additive optional
  field at most); confirm at promotion, and treat a change to the snapshot's
  on-disk shape as a store change per `ai-docs/manuals/release-compatibility.md`.

## Constraints

- Manuals: `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`.
- Every new user-facing string goes into all `internal/wsi18n/tables/*.json`.
- Owner 2026-11-13: non-UI implementation runs through `ws/pi.*` workers.

## Phases

### Phase 1: spawn-tree children in the agents pane

- A `pi.spawn` child of a lead, and a child it spawned, appear in that
  lead's agents pane as subagent rows with the right nesting, status, and
  running count, through the existing `agentTree` path.
- Tests: wsdashboard page-build test with a lead whose spawn tree holds a
  running child and a finished grandchild (rows, nesting, counts); no double
  row when a harness feed also names the agent.
- Verification: `go test` for the touched packages on the remote build host;
  a live check (lead runs `pi.spawn`, pane shows the row) is the owner's.
