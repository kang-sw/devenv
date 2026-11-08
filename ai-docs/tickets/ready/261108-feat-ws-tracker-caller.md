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
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 09b54a9c4b155dd8
sage-review-completeness-reviewed: 09b54a9c4b155dd8
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
- `workflow.whiteboard_enabled` is a tuning-catalog knob (registered in
  `internal/mcp/config_registry.go`, harness-applicable, resolved through
  every scope), not a manifest key; the `claude.` manifest keys are Claude
  Code only.
- The mockup (`ai-docs/ref/tracker-mockup/`, README and `SCENARIOS.md`) is the
  UI and data contract; its README event table is the write schema, and its
  member profile (`member.profile`, `{org, family, given}`, merge `2db82f6c2`)
  is the join payload.

## Decisions

All confirmed on the session board, 2026-11-08.

1. **Enable key** ((418) c, reopening (395)): a tuning-catalog knob
   `workflow.tracker_enabled`, with the same scope resolution (session, repo,
   project, global) and harness handling as `workflow.whiteboard_enabled`,
   default off in ws and wsflow alike. Following the whiteboard model
   ((422) user direction, (423)), it gates only the tracker manual section:
   the tracker tools are always listed and always work, and a successful
   tracker tool call turns the knob on at session scope, as
   `enableWhiteboardKnobForSession` does
   (`internal/mcp/whiteboard_tools.go#L706-L740`), under the same
   lead-authority rule. Only top-level lead sessions may call the tracker
   tools ((428) a), through the same call gate as the whiteboard tools
   (`isTopLevelLead`, `internal/mcp/server.go#L713`); a spawned worker
   reports progress to its lead, which records it, and the tracker and
   shared syntax sections are not rendered for spawned keys, as the
   whiteboard section is not (`workflow_manual.go#L346-L351`). The daemon's
   3-minute poll runs for every registered
   store and the lead-session-start check runs whenever the registry holds a
   store, regardless of the knob. Rejected: gating tool listing on the knob
   ((421): `tools/list` is per process with no session key, so a
   session-scope knob cannot decide it; process-start non-session scopes or
   `tools/list_changed` re-listing were the alternatives); letting spawned
   sessions write (workers would write to shared team ledgers) or read only
   (can be added later if workers need to read tasks); a `claude.`
   manifest key (Claude Code only, while the tracker tools are host-neutral);
   `dashboard.tracker_enabled` (the tracker must run with the dashboard off,
   (404)); a new `tracker.` manifest pair (no per-package default to differ,
   one more manifest surface).
2. **Which stores a session shows** ((407) a): all of the user's registered
   stores, in every session. Rejected: a per-project store list
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
9. **MCP tool shape** ((409) a, revised by (422) a): two tools, like the
   whiteboard - `tracker.query` (the store list, or a store's view state by
   scope / filter / item) and `tracker.update` (one all-or-nothing XML
   command batch). Registration, joining with a profile, sync, and admin
   commands are all commands in `tracker.update`; `Dispatch` already refuses
   an unauthorized admin command. Two small schemas make always listing them
   cheap. Rejected: four tools (`tracker.stores`, `tracker.view`,
   `tracker.write`, `tracker.sync`) with process-level gating; one tool per
   action (about fifteen tools); a Bash CLI taking the XML through a heredoc
   (Bash permission prompts, PowerShell heredocs differ); the daemon REST
   API through curl (needs the daemon, off by wsflow default, port
   discovery, and brings back the (413) escaping problem in curl
   arguments).
10. **Write input format** ((413) b): `tracker.update` takes a flat XML command
    string in the whiteboard's syntax - short fields as attributes, lists as
    space-separated attributes (like the whiteboard's `ids="14 15"`), bodies
    (item body, comment) raw inside the tag with no escaping - parsed by the
    shared command parser from `261108-feat-ws-manual-shared-command-syntax`
    and mapped onto the README event table. Refusals cite command position
    and line, like the whiteboard. The tag prefix is `tk:`. Every event
    command names its store with a `store` attribute and one batch touches
    one store, so `Dispatch`'s per-store all-or-nothing holds for the batch;
    register, join, and sync touch the network and cannot be rolled back, so
    each must be the only command in its batch (design review,
    auto-confirmed). Their attributes (sync naming one store or all, the
    remote URL, store id, and profile for register and join) derive from the
    registry shape (Decision 3) and the README. `tracker.query` output is unaffected.
    Rejected: a JSON event array (escaping mistakes in multi-line Korean
    markdown bodies, user note on (409)); JSON with a separate raw-body field.
11. **Agent guidance** ((410) b): a workflow-manual section rendered only when
    the knob resolves on, like the whiteboard section (Decision 1). It carries the
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "Sync cadence: poll every 3 minutes, immediate check when the drawer opens and when a lead session starts; no remote and long offline are the same path. The poll and the immediate checks are caller events" — bearing: supports
- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "One writer at a time per store directory. Every lead session runs its own ws-mcp process and the dashboard daemon is another, all opening the same store with the same clone id." — bearing: supports
- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "One personal SSH ed25519 key at ~/.ws/tracker/keys/, shared by all of a person's stores; a random clone id generated on first open and kept with the store's local bare repository." — bearing: supports
- 261107-feat-ws-tracker-ledger-protocol (2026-11-08, Result acf2ff17a): "Events, Identity, and Pending wait on the store mutex while Sync holds it across network calls." — bearing: constrains
- c6c5fe5c (2026-11-08, commit): "Veto is a constructor over the genesis and Open opens the store once without the veto to read it, then again with it. Rejected: changing VetoFunc (wsledger behavior is out of scope)." — bearing: supports
- 261108-feat-ws-tracker-data-structure (2026-11-08, Decisions): "Derive(viewer stream, now, staleDays) returns a Go struct whose JSON encoding is the README View state shape ... except store, sync, and clones, which the next child fills from the wsledger sync report." — bearing: supports
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Config (board (138) ok): the ws and wsflow manifests differ only in the enable key's default." — bearing: superseded by (418) c (the epic line now names the `workflow.tracker_enabled` knob)
- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Decisions): "Sync entry points ((396)): opening a store's drawer triggers the core's immediate check; collapsing the panel triggers its push." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker, internal/mcp, the workflow.tracker_enabled tuning knob (internal/mcp/config_registry.go), the workflow-manual section and its wsflow mirror |
| scope.surface | cross-module | new MCP tools tracker.query and tracker.update; a new config key; a new manual section; wstracker wrappers |
| scope.new_public_symbol | yes | wstracker Create, Join, Register wrappers; two tracker.* MCP tools; the enable config key |
| scope.new_type_contract | yes | stores.json registry shape, saved-profile list, tracker.update command-string contract, tool schemas |
| scope.test_surface | new-files | no importer of wsledger or wstracker outside their packages (grep of agents-plugin-tool/**/*.go); registry, sync, and MCP handler tests are new |
| complexity.reuse_points | confirmed | wsledger.Register (remote.go#L474), wstracker.Open (veto.go#L59), Tracker.Dispatch and View (tracker.go), the whiteboard tool and manual-section pattern (internal/mcp/whiteboard_tools.go, agents-plugin/rsrc/whiteboard/whiteboard.md) |
| complexity.side_effect_risk | high | writes ~/.ws/tracker data and keys, pushes to remote git hosts, adds a background poll timer across several processes sharing one store |
| risk.correctness | high | concurrent openers on one store directory, timer ownership between daemon and MCP processes, reads block on the store mutex during Sync |
| risk.fit | moderate | follows the whiteboard tool and manual pattern, but depends on the unlanded shared parser (pending 261108-feat-ws-manual-shared-command-syntax) |
| risk.test | high | live acceptance against a hosted private GitHub repository; non-interactive credential handling is a recorded ledger follow-up |
| risk.security_or_contract | high | signing key storage, remote credentials, and new agent-facing tool and config contracts |

## Phases

### Phase 1: Registry, config, wrappers, and sync

- The `workflow.tracker_enabled` knob (Decision 1), registered and resolved
  like `workflow.whiteboard_enabled`, default off; the session auto-enable
  helper the Phase 2 tools call.
- `wstracker` `Create` / `Join` / `Register` wrappers with the `Veto` hook,
  and a caller-side helper that fills `store`, `sync`, `clones`, and
  `stats.commits` from wsledger reports.
- The machine registry (`~/.ws/tracker/stores.json`), store directories, the
  key directory, and the machine-local saved-profile list (file name and
  shape are the worker's call; it stays out of the ledger).
- Sync triggers per Decisions 6 and 7: the daemon's 3-minute poll when the
  daemon runs, the lead-session-start check (the FRESH path of
  `handleWorkflowManual`, next to `maybeCollectWhiteboards`,
  `internal/mcp/workflow_manual.go#L381`; it runs in the background and
  fails silently, so a slow remote or a credential prompt never delays the
  manual; whether the CONTINUE (revive) path, L437, also triggers it is the
  worker's call, recorded), and the pre-read check without
  the daemon. The drawer-open check and the panel-collapse push are exposed
  as entry points the drawer ticket calls. A process that wrote to a store keeps it open for the process
  lifetime, so wsledger's in-process idle push (`store.go#L288-L297`) still
  fires without the daemon.
- Paths: the caller passes `KeyDir` to wsledger explicitly (its default is
  hardcoded to the home directory, `store.go#L139-L145`), with the registry,
  store directories, and keys under one root; record whether that root
  follows `$WS_CONFIG_HOME`.
- Verification: unit tests over local bare remotes for register, join with a
  profile, two-clone sync through the poll, the no-daemon pre-read check,
  registry round trip, the poll and session-start check running with the
  knob off when a store is registered, the drawer-open check and
  panel-collapse push entry points, and the idle push firing without the
  daemon; `go test` for the touched packages. Phase 1 delivers the pre-read
  check as a callable helper with its unit test; the Phase 2 read tool calls
  it. Whether it is throttled is the worker's call, recorded; an unreachable
  remote takes the same path as offline (ledger protocol decision).

### Phase 2: MCP surface, guidance, and live acceptance

Blocked by `261108-feat-ws-manual-shared-command-syntax` (the shared parser
and the shared syntax section).

- The two tools of Decision 9, always listed; a successful call turns the
  session knob on (Decision 1). `tracker.update` parses the Decision 10 command string with the shared
  parser, maps it onto `Dispatch` (the tag prefix and command set derived
  from the mockup README event table, keeping a command-to-event index map
  since `Refusal.Index` is an event index), and returns `*Refusal` reasons
  with command position and line, through the shared refusal helper;
  `tracker.query` returns the store list, or the view state for a store,
  scope / filter, or one item; registration, joining with a profile, and a
  sync command (wsledger `Sync`: fetch and push of pending writes) are
  `tracker.update` commands.
- The tracker workflow-manual section (Decision 11): its command table, the
  agent-recording rule (Decision 12), and when to sync; rendered only when
  the knob is on and never for a spawned key (the tracker-on condition takes
  the same spawned-key override as `whiteboardOn`,
  `workflow_manual.go#L349`), and pulling in the shared syntax section when the whiteboard is
  off. wsflow mirror per `wsflow-mirroring.md`.
- Live acceptance (Decision 13): the lead obtains the hosted private GitHub
  repository and credentials from the user before this step; the worker
  stops and reports if none is given. Register a store on that
  repository, join from a second clone with a saved profile, write through
  `tracker.update`, and see the change after the poll on the other clone.
- Verification: MCP handler tests (register and join with a profile through
  `tracker.update`, a successful call turning the session knob on and the
  manual section appearing, a refused call leaving it off, a spawned
  session's call refused, a
  write with a raw multi-line Korean body, an all-or-nothing refusal with
  its line), manual-render tests for whiteboard-only, tracker-only, both,
  and a spawned key with the tracker knob on (neither section rendered),
  and the live acceptance steps recorded in the Result.
