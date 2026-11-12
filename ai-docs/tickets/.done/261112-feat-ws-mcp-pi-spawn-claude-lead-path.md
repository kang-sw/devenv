---
title: "ws MCP pi.* tools: a non-Pi lead spawns, steers, and collects Pi subagents through the daemon's wsagent runner"
parent: 261112-epic-ws-mcp-pi-subagents
related:
  261110-epic-ws-dashboard-agent-host: wsagent Runner and PiBackend this builds on
  261110-feat-ws-mcp-daemon-hosted-proxy-launcher: daemon-hosted MCP server and its direct fallback
  261112-bug-ws-hosted-pi-subagents-missing-from-dashboard: hosted Pi child visibility
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2bedce42c04972d2
sage-review-completeness-reviewed: 2bedce42c04972d2
completed: 2026-11-13
---

# ws MCP pi.* tools (Claude-lead path)

## Background

First child of `261112-epic-ws-mcp-pi-subagents` (epic Decision 11): the
`pi.*` tools, tier resolution, mailbox report plus `pi.wait`, dashboard
registration, the Pi version gate, and the wsflow knob. `tune-pi` and hiding
`pi.*` from Pi-harness sessions are later children. The epic's Cross-Child
Decisions bind this ticket; they are not repeated here.

Facts gathered 2026-11-12 (paths under `agents-plugin-tool/internal/`):

- The MCP server is daemon-hosted: `wsdashboard/proxy.go` serves `/mcp` as
  an authenticated upgrade and `cmd/ws-mcp/proxy.go` (`proxyHost.Accept`)
  builds one `mcp.Server` per connection inside the daemon. When the hello
  is refused, the launcher falls back to a direct `ws-mcp serve` process.
  A per-connection server ends when its connection closes.
- `wsagent.Runner` is a daemon object (`cmd/ws-mcp/dashboard.go`
  `newAgentRunner`, Claude and Pi backends). `mcp.ServerOptions`
  (`mcp/process_env.go`) carries no Runner handle; its only daemon handle is
  `Live wsdashboard.LiveHost`, nil on the direct path. The runner's HTTP API
  is `wsagent/http.go` (`POST /agents {profile, root}`, `/send`, `/stop`,
  `/close`, `/settings`).
- `Runner.Create` takes only `(profile, root)`; model and effort come from
  the profile and `SetSettings`. There is no cwd, prompt, or parent field,
  and `Record` has no parent. `StartSpec.Cwd` is honoured only for leads
  (`pi.go` `spec.workDir()`); a non-lead child runs in `spec.Root`.
- `PiBackend` (`wsagent/pi.go` `piArgs`) runs `pi --mode rpc` with
  extensions, skills, prompt templates, and context files off, `--model`,
  `--thinking`, and `--tools` as the whole surface. ws tools reach a child
  through a stdio ws-mcp child with `WS_MCP_ALLOWED_TOOLS` set to the
  profile's `WSTools` (`runner.go` `startProcess`); `toolAllowed`
  (`mcp/server.go`) honours that explicit allowlist, so a child sees no
  `pi.*` unless listed. Builtin profiles are only `tracker-support-claude`,
  `tracker-support-pi`, `lead-claude`, and `lead-pi`
  (`wsagent/builtin.go#L89-L115`).
- A non-lead Pi child is the daemon's own `exec.Cmd` child: it dies with the
  daemon and resumes lazily on the next send from its persisted session
  file; idle processes stop after `DefaultIdleTimeout` (10 minutes).
- `wsconfig.ResolveAgentTierForHarness(opts, tier, "pi")` looks up the pi
  bucket, then default, then codex; the shipped manifests seed no pi bucket
  (`config_test.go` notes it), so on a machine without a tuned pi bucket a
  Pi spawn resolves to a codex model Pi cannot run. This machine's global
  config (`~/.ws/config.json`) already has a pi bucket: small
  `openai-codex/gpt-5.6-luna` high, medium/large/xlarge `gpt-6.1-sol`
  medium/high/xhigh (the last three without a provider prefix).
- `mcp/mailbox_tools.go` `handleMailboxSend` is the only queue writer
  (`wsmailbox.AppendQueue` under `WithReplyLock`, then
  `wswake.Mailbox.Wake()`); its `from:` stamp needs an owner check. Nothing
  posts mail on a child's completion today.
- Agents-panel rows come from `wslive.State` subagents fed by `agent`
  events (`wslive/state.go`); `Event.SessionKey` links a row to a child's
  own session. The `Agent` snapshot struct has no kind field
  (`wslive/snapshot.go#L295-L315`), but `Event` already has `Kind`, JSON key
  `kind`, which is the event type (`agent`, `status`, ...;
  `wslive/state.go#L75-L77`), so a new agent marker cannot reuse the `kind`
  key on an event. A Runner conversation also registers
  itself as a top-level dashboard session (`runner.go`
  `writeRegistration`).
- No code checks a Pi version; installed `pi --version` prints `1.0.4`.
- `tools/list` is not knob-aware: `filteredTools` takes no session key and
  filters through env flags, static hides, and the allowlist.
- Per-package knob defaults precedent: `claude.status_band_enabled` is true
  in `agents-plugin/config-manifest.json#L41-L43` and false in
  `agents-plugin-wsflow/config-manifest.json#L41-L43`.

## Decisions

1. **Compatibility tier T0.** New tools, an optional `agent_kind` field on
   agent events and snapshots, and a new config key are all additive
   (`ai-docs/manuals/release-compatibility.md`).
2. **Children live in the daemon's runner.** Pi children run in the daemon's
   `wsagent.Runner`, reached through a new daemon handle on
   `mcp.ServerOptions`. Rejected: a child owned by the per-connection MCP
   server - that server ends when its MCP connection closes, so only a
   daemon-owned child survives a reconnect, and the Runner already persists
   records and resumes Pi sessions.
3. **Direct-fallback path refuses.** With no daemon handle (the daemon
   refused the hello), `pi.*` refuses with a message naming the dashboard
   daemon. Rejected: the direct server calling the daemon's HTTP `/agents` as
   a client (only helps when a daemon exists but refused the hello); running
   the child inside the direct process (dies with it; a second code path).
4. **Spawn spec.** `Runner.Create` gains a spec form carrying model, effort,
   first prompt, cwd, and a parent reference, recorded on `Record` as
   optional fields. The parent reference is the parent lead's mailbox
   reply-id (`wsmailbox.ReplyID(key)`), not the raw session key, so no
   credential lands on disk; it is a typed reference child 1b can also point
   at a runner conversation. cwd is honoured for non-lead children: default the
   caller's root, relative paths resolve against it, absolute paths are
   allowed so worktrees outside the root work. Rejected: chaining
   `POST /agents` + `/settings` + `/send` - one call keeps spawn atomic, and
   the parent reference is what `pi.list`, the report, and the dashboard
   row need.
5. **`pi-worker` profile.** A new builtin profile with Pi's built-in coding
   tools (read, bash, edit, write, grep, find, ls) plus the ws tools the
   `delegate-implementer` floor and the worker playbooks name (enumerated by
   the implementer from those playbooks), and no `pi.*` in this ticket. A Pi
   child then runs the same rendered delegate prompts a Claude subagent runs.
   Rejected: built-in tools only (the floor's `ws/git.commit` step would
   fail); every ws tool the delegate role allows (largest list; Pi context
   cost). Giving Pi workers `pi.*` (recursion) is child 1b's change, not this
   ticket's. The profile's appended system prompt is empty: the rendered
   playbook prompt is the child's first message.
6. **Tier resolution, partial tiers allowed.** `pi.spawn` resolves its tier
   from the pi bucket only, with no fallback to the default or codex buckets.
   The bucket may define any subset of the four tiers. A spawn on an unset
   tier refuses, naming the `config.tune` key to set and listing the tiers
   that are set, so the calling agent picks one (for example a Pi setup on
   internal models with no large or xlarge). Rejected: omitting `--model` so
   Pi uses its own default (every tier silently on one model); seeding
   builtin pi defaults with fixed model names (guesses providers); falling
   back to the nearest set tier (hides the choice from the agent).
7. **Completion report by mail.** When a child's turn ends (idle) or the
   child fails, the daemon appends a mail to the parent lead's reply-id queue
   as a system sender (empty `from`, a new `wsmailbox` helper) and wakes it;
   the mail carries the child id, status, and the final assistant message.
   A `pi.stop` from the parent sends no mail. A turn cut off by the daemon
   exiting is reported as failed (status `interrupted`) when the daemon next
   starts and loads the record.
   Rejected: routing through `handleMailboxSend` - it requires a sender owner
   check a daemon-side writer has no identity for.
8. **The mail is the primary turn trigger; `pi.wait` is the fallback.** On a
   host with mailbox push the lead's next turn starts when the mail arrives,
   as a native subagent's completion notice does. `pi.wait(id, timeout_s?)`
   blocks until a turn that started after the caller's last `pi.send` (or
   the spawn) ends or fails, so it never returns the previous turn's
   message; default 300 s, capped at 1800 s, returning status and the final
   assistant message. Both tool descriptions say which is primary; the
   `pi.wait` description says the host may cut a long call off sooner (Codex
   `tool_timeout_sec` is 600 in `agents-plugin/.mcp.json`) and that the
   completion mail still arrives after a wait collected the result. Rejected: an unbounded wait (an MCP
   host can time out a call that never returns); a Claude Code mod for hosts
   without push in this ticket (mailbox push already starts an idle lead's
   turn).
9. **Scoping and lifecycle.** `pi.list` shows only the calling lead's
   children (by the recorded parent reference); `pi.stop` closes the child (stops
   the process; the record is kept and readable through `pi.transcript`),
   mirroring the extension's lifecycle set. Rejected: listing every runner
   conversation - other leads' and dashboard-started agents are not this
   lead's to steer.
10. **Dashboard.** The runner's dashboard registration is kept, so the
    agents-panel row (with a blinking `pi` badge where an agent row shows the
    blinking blue circle, driven by a new optional `agent_kind:"pi"` on
    agent events and snapshots; `Event.Kind` already carries the event type
    under JSON `kind`) opens the child's conversation; the child is kept out of the
    session rail's top-level list. The parent lead's own per-connection
    server feeds the row: it subscribes to runner state changes for children
    whose parent reference matches its session and emits `agent` events
    into its own `wslive.State` (replayed after a reconnect); the runner
    never opens a second hosted slot for the lead's session. With the lead's
    dashboard feed off there is no row. Rejected: listing it in the rail like any
    session (a lead with several workers floods the rail); not registering
    it (no conversation page).
11. **Exposure gate at `tools/list`.** `pi.*` is listed only when the
    knob `dashboard.pi_spawn_enabled` (in the `dashboard-config-manifest.json`
    of ws, default true; wsflow, default false; Pi, default false, so
    Pi-package sessions do not list it at builtin scope; full Pi-harness
    hiding is child 3's) reads on from the project, global, or builtin
    scope; the server holds a daemon handle (a call that still finds none refuses per Decision 3); and
    `pi --version` (run once per server process, cached) reports 1.0 or
    later. A knob change takes effect on
    reconnect. Rejected: listing always and refusing at call time - an
    unusable tool in every wsflow session costs context and invites a failing
    call, and child 3 needs the same harness-aware listing. Knob family
    (624): rejected `claude.pi_spawn_enabled` (that namespace means "Claude
    Code only", but Codex leads load the same manifest), a new namespace
    manifest (new file plus `WS_MCP_CONFIG_MANIFESTS` edits in both packages),
    and a builtin Go key (cannot differ between ws and wsflow); a manifest
    key must start with its manifest's namespace (`mcp/config_manifest.go`).
12. **Pi tier models for the real run.** The real run uses this machine's
    existing global pi bucket as is; the lead does not retune it. The
    worker does not change it either. If Pi rejects the unprefixed
    `gpt-6.1-sol` names, the real run uses the small tier and the worker
    reports the rejection rather than editing config. Partial-tier
    behaviour (Decision 6) is covered by Go tests. Rejected: overwriting the
    bucket with a mirror of the codex bucket (the user's own values are
    current).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/release-compatibility.md`,
  and `ai-docs/manuals/wsflow-mirroring.md` before editing; tool
  descriptions are shipped text.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Out of scope: `tune-pi`, hiding `pi.*` from Pi-harness sessions (later
  epic children), and the Pi extension's own `ws-agent-*` tools.
- Out of scope: recursion (Pi workers spawning Pi children through `pi.*`).
  It is child 1b, ticketed separately and run after this ticket lands; the
  parent record (Decision 4) and the report path (Decision 7) are what it
  builds on, so keep them general enough for a Pi parent.

## Prior Decisions

- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-12, Decisions): "Completion to a Pi parent is steered ... A Claude lead parent still gets child 1's mailbox mail." — bearing: constrains
- 261112-bug-ws-hosted-pi-subagents-missing-from-dashboard (2026-11-12, commit): "Rejected (ticket decisions): deriving rows from spawn results / settled pushes; routing child events through `dashboard.intake`." — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Result): "`WS_MCP_ALLOWED_TOOLS` (comma-separated ws tool names) limits the attached ws-mcp's `tools/list` and `tools/call`. `WS_MCP_NO_DASHBOARD=1` stops it ensuring a dashboard" — bearing: supports
- dab9f6c3 (2026-11-08, commit): "WS_MAILBOX blanking: a daemon started from a lead's ws-mcp inherits that lead's mailbox identity, and an attached ws-mcp with it would register presence for the lead's inbox." — bearing: constrains
- 907d3b85 (2026-11-12, commit): "Takeover resumes the conversation the key record names (harness_session, harness_cwd from Phase 2) and keeps both on the agent record (cwd, session_file), so every later start runs where the terminal harness ran." — bearing: constrains
- 260926-bug-pi-execute-worker-steals-mailbox-owner (2026-09-26, Decisions): "the execute-worker spawn carries the lead's session key ... so the child's bootstrap `ferrule` carries `parent_session_key`." — bearing: supports
- 261109-feat-ws-dashboard-subagent-badge (2026-11-09, Decisions): "the builtin support profiles allow Claude's `Agent` tool. Subagents must stay inside the same tool limit ... the Pi profile is unaffected because Pi has no subagent tool." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsagent/runner.go, wsagent/record.go, wsagent/builtin.go, wsagent/backend.go, mcp/process_env.go, mcp/server.go, mcp/mailbox_tools.go, wslive/state.go, wslive/snapshot.go, wsmailbox, cmd/ws-mcp/proxy.go, both config-manifest.json files |
| scope.surface | cross-module | new MCP tools pi.*, Runner spawn spec, Record fields, mailbox writer helper, live event field, config key across wsagent, mcp, wsmailbox, wslive, wsdashboard, cmd/ws-mcp |
| scope.new_public_symbol | yes | pi.spawn, pi.send, pi.list, pi.stop, pi.transcript, pi.wait, pi_spawn_enabled, a Runner spawn-spec constructor, a wsmailbox system-sender helper |
| scope.new_type_contract | yes | Runner spawn spec, optional Record parent/cwd fields, ServerOptions daemon handle, optional agent marker on Event and wslive.Agent |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/runner_test.go, pi_test.go, wslive/state_test.go, mcp tests exist; new test files not confirmed |
| complexity.reuse_points | confirmed | wsagent.Runner and PiBackend, wsconfig.ResolveAgentTierForHarness, wsmailbox.AppendQueue and WithReplyLock, wswake.Mailbox.Wake, status_band_enabled manifest precedent all read |
| complexity.side_effect_risk | high | daemon-owned child processes, daemon-side mailbox writes and wake, per-connection MCP server wiring to a daemon handle |
| risk.correctness | high | async completion report, pi.wait timeouts, parent scoping, and daemon restart resume are concurrent paths |
| risk.fit | moderate | Event already uses the kind JSON key for the event type, so the planned kind:"pi" needs another name; Runner.Create signature grows a spec form |
| risk.test | high | the acceptance run needs a real Pi 1.0.4 child, mailbox wake, and headless Chromium; Go tests need fake backends |
| risk.security_or_contract | high | non-lead child gets bash/edit/write with caller-chosen absolute cwd, a daemon-side system-sender mail path, and a T0 contract claim on new event/snapshot fields |

## Phases

### Phase 1: pi.* tools on the daemon runner

- Runner: the spawn spec form, parent link, non-lead cwd, the `pi-worker`
  builtin profile.
- MCP: `pi.spawn`, `pi.send`, `pi.list`, `pi.stop`, `pi.transcript`,
  `pi.wait`; the daemon handle on `ServerOptions`; the exposure gate.
- Mailbox completion report; dashboard row with `agent_kind:"pi"` and the blinking
  `pi` badge where an agent row shows the blinking blue circle.
- `dashboard.pi_spawn_enabled` in the three dashboard config manifests; mirrors and manifest
  regeneration as `ai-docs/manuals/wsflow-mirroring.md` requires.

Verification:
- Go tests: spawn spec recorded with parent and cwd; tier refusal message;
  `pi.list` scoping; mailbox report written to the parent's queue and wake
  fired; `pi.wait` timeout; `pi.wait` after `pi.send` waits for the new
  turn rather than returning the previous message; no mail after
  `pi.stop`; an `interrupted` mail after a daemon restart cuts a turn off;
  exposure gate (knob off, Pi missing, Pi below 1.0, no daemon handle, all
  hide) and a call with no daemon handle refuses; `agent_kind` round-trips
  through live state and rows replay after a reconnect.
- A real run on this machine (Pi 1.0.4): a Claude lead spawns a Pi child at
  a tuned tier, receives the mailbox report, reads the transcript, and the
  agents panel shows the `pi` badge (headless Chromium screenshot).
- Existing `mcp`, `wsagent`, `wslive`, `wsdashboard`, and `wsrsrc` suites.

### Result (fa9d1f061) - 2026-11-13

Implemented in 7f40e8110 (runner spawn spec, parent link, turn reports,
waits, `pi-worker` profile, `agentKind` in live state), 543481e8b (the six
`pi.*` tools on `ServerOptions.Agents`, exposure gate, daemon-side mailbox
report, live-row feed and replay, `pi` badge, `dashboard.pi_spawn_enabled`
in the three dashboard manifests, runtime inventories), and fa9d1f061
(knob scope test after review). Compatibility: T0 (additive tools, knob,
and optional snapshot/event field).

- The live event and snapshot field shipped as the JSON key `agentKind`
  (camelCase, matching `sessionKey` and `parentId`), not `agent_kind`.
- Senderless report mail reads `system` in `mailbox.recv`, history, and the
  badge; the Claude mod's pinned batch formatter still labels it `unknown`,
  and the body's `[Pi subagent <id> "<label>"]` prefix names the source.
- The session page links agent rows (`Linked` covers agent registrations),
  since a runner conversation has no whiteboard.
- Real run (isolated dev daemon on Pi 1.0.4, the global pi bucket
  unchanged): tier `medium` (`gpt-6.1-sol`, unprefixed) was accepted; the
  child ran `bash`, its report reached the lead's mailbox as `system`,
  `pi.transcript` and `pi.wait` returned the turn, the agents panel showed
  the `pi` badge while it ran and a check after, and `pi.stop` closed it.
- Lite review: one Important (knob ignores project scope) rejected with
  evidence, since the project scope is the cache-root file a keyless read
  resolves; fa9d1f061 pins it. One Minor (field name differs from ticket
  text) recorded above.
