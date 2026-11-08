---
title: Tracker caller core - store registry, config, sync, MCP surface, and agent guidance
parent: 261107-epic-ws-personal-common-store
related:
  261107-feat-ws-tracker-ledger-protocol: prerequisite (wsledger, done)
  261108-feat-ws-tracker-data-structure: prerequisite (wstracker, done)
  261108-feat-ws-manual-shared-command-syntax: prerequisite of Phase 2 (shared command parser and manual syntax section)
  261108-feat-ws-tracker-drawer-ui: blocked by this ticket (the dashboard side)
  261108-feat-ws-tracker-support-agent-chat: blocked by this ticket
  261108-feat-ws-tracker-ledger-attachments: sibling; attachment blobs
---

# Tracker caller core - store registry, config, sync, MCP surface, and agent guidance

## Background

The ledger protocol (`internal/wsledger`) and the tracker domain layer
(`internal/wstracker`) are merged, but nothing outside those two packages
imports them. This child connects them to callers: registering and joining
stores, the enable key, sync triggers, the MCP surface, and agent guidance
(epic "Next (unplanned): the caller child, covering MCP and skill exposure
((115)), the dashboard drawer, sync triggers, and config"). The dashboard
drawer is split out into `261108-feat-ws-tracker-drawer-ui` ((408) b).

Evidence gathered 2026-11-08:

- `wsledger.Register(ctx, opts, url)` (`remote.go:474`) creates a store from
  an empty repository or joins an existing one; `Append`, `Sync`, `Fetch`,
  `Pending`, `Events`, `Concurrent`, `Identity` are the write and read
  surface (`store.go`). A file lock serializes processes and the push
  deadline is persisted, so the daemon and every ws-mcp process can open the
  same store. Only the 5-minute idle-push timer exists; there is no poll
  timer. The package reads no config; everything comes through `Options`.
- `wstracker.Open` (`veto.go:59`) installs the `Veto` hook; `Create`, `Join`,
  and `Register` have no wrapper. `Dispatch` validates all-or-nothing and
  returns a `*Refusal`; `View(now, staleDays)` yields the mockup's view-state
  JSON, minus the caller-filled `store`, `sync`, `clones`, and
  `stats.commits`.
- `config_manifest_parity_test.go` enforces that the shipped `claude.`
  manifests differ only in defaults.
- The mockup (`ai-docs/ref/tracker-mockup/`, README and `SCENARIOS.md`) is the
  UI and data contract; its README event table is the write schema, and its
  member profile (`member.profile`, `{org, family, given}`, merge `2db82f6c2`)
  is the join payload.

## Decisions

All confirmed on the session board, 2026-11-08.

1. **Enable key** ((395)): one key in the `claude.` config manifest, default
   off in both ws and wsflow. The MCP tools and the guidance section exist
   only where it resolves on (repo / clone / machine layers).
2. **Which stores a session shows** ((407) a): all of the user's registered
   stores wherever the tracker is enabled. Rejected: a per-project store list
   in config; a per-project hide list (can be added later as one key).
3. **Store registry** ((403)): one machine-level file
   `~/.ws/tracker/stores.json` (id, remote URL, directory, member id).
   Rejected: a per-project list - stores belong to the user (personal and
   team stores), not to a project.
4. **Store location** ((399)): store data at `~/.ws/tracker/<store>/` (a bare
   repository, user data, not cache); signing keys at `~/.ws/tracker/keys`.
5. **Domain wrappers** ((406)): `wstracker` gains `Create` / `Join` /
   `Register` wrappers that install the `Veto` hook; callers fill `store`,
   `sync`, `clones`, and `stats.commits` from wsledger reports. Rejected:
   passing `Veto` at every call site - a forgotten hook writes unvalidated
   events.
6. **Sync** ((396)): a 3-minute background poll; an immediate check when a
   store's drawer opens and when a lead session starts; own writes pushed
   after the 5-minute idle delay, at an explicit apply, or when the panel
   collapses.
7. **Timer ownership** ((404)): when the dashboard daemon runs it owns the
   poll timer (and, in the drawer ticket, the local UI state); MCP processes
   open stores on demand under wsledger's lock; without the daemon (wsflow
   default), a check runs at lead session start and before each tracker MCP
   read. Rejected: a separate tracker daemon - one more long-lived process,
   and wsledger already handles concurrent openers.
8. **Joining** ((400), (411) c): joining uses machine-local saved profiles
   (`{org, family, given}`), defaulting to the last one used; the saved list
   never enters the ledger. Registration and joining are reachable both
   through the agent (the MCP surface; the agent asks for or offers the
   profile) and through a dashboard dialog, which belongs to the drawer
   ticket. Rejected: agent-only; dashboard-only.
9. **MCP tool shape** ((409) a): four coarse tools - `tracker.stores`,
   `tracker.view` (store, scope / filter, item), `tracker.write` (store,
   command batch, all-or-nothing), `tracker.sync`. Rejected: one tool per
   action (`tracker.create`, `tracker.assign`, ...) - about fifteen tools
   when enabled, while `Dispatch` already returns refusal reasons.
10. **Write input format** ((413) b): `tracker.write` takes a flat XML command
    string in the whiteboard's syntax - short fields as attributes, lists as
    space-separated attributes (like the whiteboard's `ids="14 15"`), bodies
    (item body, comment) raw inside the tag with no escaping - parsed by the
    shared command parser from `261108-feat-ws-manual-shared-command-syntax`
    and mapped onto the README event table. Refusals cite command position
    and line, like the whiteboard. `tracker.view` output is unaffected.
    Rejected: a JSON event array (escaping mistakes in multi-line Korean
    markdown bodies, user note on (409)); JSON with a separate raw-body field.
11. **Agent guidance** ((410) b): a workflow-manual section rendered only when
    the tracker is enabled, like the whiteboard section. It carries the
    tracker command table and tracker rules only; the command syntax lives in
    the shared section ((414)), loaded once when the whiteboard or the tracker
    is enabled. User note on (413): "매뉴얼 컨텐츠의 최적화 ... 문구 자체도
    필요한 내용만 담도록". Rejected: a new skill `ws:lead-tracker` (an AGENTS.md
    "Ask first" item, and a separate load); both.
12. **Agent recording** ((401)): an agent records progress automatically only
    for tasks assigned to its user; anything else on request. The guidance
    section states this rule.
13. **Live acceptance** ((402)): against a hosted private GitHub repository,
    in this ticket; unit tests keep local bare remotes (board (341)).

## Constraints

- Manuals that bind edits here (AGENTS.md Implementation Conventions):
  `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`, `agents-plugin/`, `agents-plugin-wsflow/`),
  `ai-docs/manuals/ws-mcp.md` (`agents-plugin-tool/internal/mcp/`),
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (the manual section text).
- `wsledger` stays config-free and payload-agnostic; config resolution and
  paths live in the caller.
- No protocol-layer change in `wsledger`; stop and report if one seems needed.
- Out of scope: the dashboard drawer, its endpoints, local UI state, and the
  join dialog (`261108-feat-ws-tracker-drawer-ui`); attachment blobs
  (`261108-feat-ws-tracker-ledger-attachments`); a per-project hide list.

## Phases

### Phase 1: Registry, config, wrappers, and sync

- The enable key in both `claude.` manifests (default off in each, parity
  test green), resolved through the existing config layers.
- `wstracker` `Create` / `Join` / `Register` wrappers with the `Veto` hook,
  and a caller-side helper that fills `store`, `sync`, `clones`, and
  `stats.commits` from wsledger reports.
- The machine registry (`~/.ws/tracker/stores.json`), store directories, the
  key directory, and the machine-local saved-profile list (file name and
  shape are the worker's call; it stays out of the ledger).
- Sync triggers per Decisions 6 and 7: the daemon's 3-minute poll when the
  daemon runs, the lead-session-start check, and the pre-read check without
  the daemon. The drawer-open check and the panel-collapse push are exposed
  as entry points the drawer ticket calls.
- Verification: unit tests over local bare remotes for register, join with a
  profile, two-clone sync through the poll, the no-daemon pre-read check,
  registry round trip, and the enable key off hiding everything; `go test`
  for the touched packages and the manifest parity test.

### Phase 2: MCP surface, guidance, and live acceptance

Blocked by `261108-feat-ws-manual-shared-command-syntax` (the shared parser
and the shared syntax section).

- The four tools of Decision 9, registered only when the enable key is on;
  `tracker.write` parses the Decision 10 command string with the shared
  parser, maps it onto `Dispatch`, and returns `*Refusal` reasons with
  command position and line; `tracker.view` returns the view state for a
  store, scope / filter, or one item; registration and joining with a
  profile go through the same surface.
- The tracker workflow-manual section (Decision 11): its command table, the
  agent-recording rule (Decision 12), and when to sync; rendered only when
  enabled, and pulling in the shared syntax section when the whiteboard is
  off. wsflow mirror per `wsflow-mirroring.md`.
- Live acceptance (Decision 13): register a store on a hosted private GitHub
  repository, join from a second clone with a saved profile, write through
  `tracker.write`, and see the change after the poll on the other clone.
- Verification: MCP handler tests (tool list with the key on and off, a
  write with a raw multi-line Korean body, an all-or-nothing refusal with
  its line), manual-render tests for whiteboard-only, tracker-only, and both,
  and the live acceptance steps recorded in the Result.
