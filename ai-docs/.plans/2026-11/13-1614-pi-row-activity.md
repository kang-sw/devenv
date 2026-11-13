# Delegate plan: hosted Pi child rows show the child's current tool

## Outcome

Owner request (dashboard, 2026-11-13). In the dashboard agents pane, a Pi
child row (spawned by `pi.spawn`, agent id `pi:<id>`) shows only the `-`
placeholder while the child runs. Claude subagent rows show the running tool
(name + short args + duration). The Pi row should show the same live line:
the child's currently running tool, cleared when the call ends.

The template already renders `.Running` / `.Generating` for any agent that
carries `Batches` (`wsdashboard/assets/dashboard.tmpl` `"slots"` ~133,
`"runrow"` ~127; `wsdashboard/pageview.go` `liveSlots` ~570). No template or
JS change is expected. This is a Go feed gap.

## Evidence (investigation, paths under agents-plugin-tool/internal)

- `wsagent/spawn_live.go:9` `SpawnAgentEvent` builds the `pi:<id>` row event
  (label, status, model, effort, session key, context tokens, output tps,
  parent id) but never sets `Batches`/`Caption`.
- `wsagent/runner.go` ~1440-1457 builds `State` from the child conversation
  (ContextTokens, OutputTps ~990); State has no activity field.
- The child's own stream already produces the right shape: `wsagent/pi.go`
  `piMapper` registers calls at `message_end` (`m.batches.addCall`, ~958) and
  ends them at `tool_execution_end` (~731); `wsagent/stream.go` `batchTracker`
  (~425-560) emits `KindAgent` events with `Batches[].Running`. These land on
  the child's own conversation state only, never on the parent's row.
- Row refresh: `spawnDirty` is set only on reset/status changes;
  `wakeSpawnParents` is the existing wake path; `seedSpawnRows`
  (spawn_live.go:81) rebuilds rows.
- `wslive/spawn_agents.go:49-50` `mergeSpawnAgents` copies `d.Activity =
  a.Activity` only per native-alias pair; it must not blank the new data.
- Prior related commit: d567abd01 (context + rate on the Pi row).

## Settled decisions (lead)

1. Approach (investigation's option A): carry the child's current lead-level
   batch view on `wsagent.State` (simplest shape that fits, e.g. the child's
   live `Batches` for the lead/top-level agent), fill it where State is built
   in runner.go, and pass it as `Batches` in `SpawnAgentEvent`. Pick option B
   (forward batch events from `conv.apply` to the parent loop) only if A
   cannot refresh without a new mechanism; record the choice in `## AI Context`.
2. Refresh: the parent row must update when the child's running tool starts
   and ends. Reuse the existing invalidation (`spawnDirty` /
   `wakeSpawnParents`); keep the "lists once per invalidation, not per stream
   write" property (`TestHostedSpawnProjectionListsOncePerInvalidationNotPerStreamWrite`)
   intact - invalidate on batch running-state changes, not on every delta.
3. If `State` is persisted or crosses the wire, the new field is optional and
   omitted when empty (T0, additive). Record the tier in `## AI Context`.
4. Out of scope: handling `tool_execution_start` in `piMapper` for an earlier
   "generating" row; any template/CSS change; Claude rows.

## Verification

- Add one test modelled on `wsagent/hosted_usage_test.go`
  `TestHostedPiRowUsesOwnUsageWhileRunningAndAfterRecovery`: while the Pi
  child has an open tool call, its parent-side `pi:<id>` row carries a
  `Running` batch view with the tool name; after `tool_execution_end` it no
  longer does.
- `go test ./internal/wsagent/ ./internal/wslive/ ./internal/wsdashboard/`
  passes on remote w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/freebase-prominent-pronto' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsagent/ ./internal/wslive/ ./internal/wsdashboard/'`
  (Node-dependent tests that fail there only for missing Node: run those
  locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`). One passing
  full check closes the work. A pre-existing failure proven at base may be
  excluded and recorded as one Forward.

## Return boundary

Commit on `impl/develop/pi-row-activity` in this worktree (this plan rides the
commit). Do not merge. Do not restart or replace the live dashboard daemon.
Stop and report if the change needs a T1/T2 change (store shape, protocol,
event type) or edits outside wsagent / wslive (and their tests).

Machine rules: temp files only under `D:/Temp/<purpose>/`; never create
folders in C:/ or D:/ roots; stop if D: free < 20 GB.
