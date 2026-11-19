---
title: "hosted Codex 워커: agent.spawn으로 Codex 워커를 띄워 Pi 워커를 대체"
parent: 261114-epic-ws-common-session-foundation
related:
  261115-feat-ws-hosted-codex-adapter-contract: prerequisite (landed hosted Codex lead adapter)
  261118-feat-ws-agy-tool-names-and-worker-role: pattern (worker descriptor fields, tool-name scheme)
  261115-feat-ws-permission-profile: overlap (Phase 3 Codex per-run permission launch path)
  261118-feat-ws-hosted-model-list-and-agy-bypass: overlap (Codex model-list sources; its board IDs (167)-(172) are a different board day than this ticket's)
  261117-feat-ws-hosted-prompt-assembly: overlap (Decision 15 excludes Codex, now stale; Phase 4 renderable playbooks for agent.spawn)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 582eb7ecd0bb199f
sage-review-completeness-reviewed: 582eb7ecd0bb199f
---

# Hosted Codex worker: spawn Codex workers through agent.spawn to replace Pi workers

## Background

Owner, 2026-11-19 11:37: hosted Codex as a worker is the first priority; it
replaces the Pi workers a lead uses for implementation. Pi/GPT workers have
been unreliable on non-mechanical work this track (owner (164), 2026-11-19),
and Claude usage is scarce, so the lead needs a GPT-backed worker that runs
through the common hosted path with the common worker ws loadout (174).

State on `track/agent-v2` (Explore, 2026-11-19; verified against source at
`3401f4b6b` by the fact populator, 2026-11-19):

- `261115-feat-ws-hosted-codex-adapter-contract` landed (`8aaf323d2`,
  `339cde068`): a hosted Codex LEAD over a sustained app-server (codex-cli
  0.160.0) under the Runner and the Python holder. HTTP MCP is injected by
  shadowing the plugin's `ws`/`wsflow` servers; instructions go in as additive
  `developerInstructions` (`internal/wsagent/codex.go:188-193`); native
  subagents are disabled; approvals are binary. The adapter's recorded
  acceptance probed `codex-cli 0.160.0` on macOS and used a loopback echo-tool
  stand-in; its Windows behavior was not exercised.
- Observed on the hosted Codex LEAD (hosted Codex lead dogfood 2026-11-19,
  owner-opened session on the current daemon; lead-side only, no worker
  evidence yet): all 79 ws tools are listed, matching the Claude lead
  count; Codex presents each as `mcp__ws__<name>` with dots already turned
  into underscores (for example `mcp__ws__tickets_query`,
  `mcp__ws__agent_spawn`); no size or count cutoff was seen; keyed calls
  work; `workflow_manual` with only the key returns about 26.9K chars in 284
  lines; the `# Manuals` block is present in the session instructions; no
  native host spawn tool exists, while `mcp__ws__agent_spawn` is exposed;
  shell comes from Codex's own `exec_command` plus a user-config MCP server
  `git_bash`, so a worker's shell posture depends on the user's Codex config.
  The session never tried a call without `session_key` (its instructions
  require the key on every call), so `hostedSessionKeyRule` fill is
  unobserved. The manual names tools with both `wsflow/` and `ws/` prefixes
  while the hosted server is named `ws`; this is the defect
  261117-feat-ws-hosted-prompt-assembly Phase 2 fixes and is out of scope
  here.
- The Codex `BackendRegistration` (`cmd/ws-mcp/dashboard.go#L300-L301`) is
  lead-only ("tiers remain unavailable to agent.spawn") and declares no
  worker field (it sets `Backend`, `LeadProfile`, `PermissionModes`,
  `Catalog`, `ManualHarness`). Claude sets `WorkerCapable`, `WorkerProfile`
  and `WorkerHTTPMCP` (`dashboard.go#L296`), Pi sets `WorkerCapable` and
  `WorkerProfile` (`#L299`), and agy sets all of those plus
  `WorkerAutoApprove`, `ToolNameScheme` and `ToolsAlwaysLoad` (`#L305`);
  only the last three are agy-only.
- The Codex adapter itself is lead-only: `CodexBackend.Start` returns
  "codex backend: hosted lead sessions only" when `spec.Lead` or
  `spec.Holder` is nil (`internal/wsagent/codex.go#L71-L73`), and later
  reads `spec.Lead.DaemonVersion`, `spec.Lead.PermissionMode` and
  `spec.Lead.Spawned` (`codex.go#L106`, `#L117`, `#L161`, `#L198`). A worker
  start has no lead spec; the runner gives only Pi workers a holder
  (`internal/wsagent/runner_process.go#L166-L169`). agy's adapter already
  carries the worker shape (`internal/wsagent/agy.go#L127`, `#L151`:
  `worker: spec.Lead == nil`).
- No `agent-worker-codex` builtin profile exists: the worker-profile loop
  covers only claude, pi and agy (`internal/wsagent/builtin.go#L213-L228`),
  and `AgentWorkerProfile(backend)` is `"agent-worker-" + backend`
  (`internal/wsagent/agent_tools.go#L42`).
- The shipped `agents.tier` defaults already carry codex mappings: bare
  tiers default to `gpt-6-luna`/`gpt-6-sol` on backend codex and each tier
  has a `codex` harness row (`internal/wsconfig/config.go#L611-L618`,
  `#L641-L664`); Phase 1 item 2's "default `codex.<tier>` mappings" are
  already present, and the open work is the worker descriptor, profile and
  adapter path.
- `agent.spawn` refuses a harness that is not worker-capable
  (`spawnableHarness`, `internal/wsagent/agent_tools.go#L234-L251`, refusal
  at `#L239-L241`); `resolveTier` reads the `agents.tier` bucket for
  `<harness>.<tier>` (`agent_tools.go#L332`); `agentsTierHarnessEnum`
  already includes `codex` (`internal/mcp/config_registry.go#L25`).
- At source HEAD `filteredTools` never lists `pi.*` on any connection
  (`internal/mcp/server_inventory.go#L80-L87`, commit `768af2113`), and
  lists `agent.*` wherever `agentListed()` holds: the connection has the
  daemon's runner (`s.pi.agents != nil`) or a forwarding token
  (`internal/mcp/agent_tools.go#L46-L48`). The daemon's proxy gives every
  MCP-mode launcher connection that carries no pi token the runner
  (`cmd/ws-mcp/proxy.go#L106-L108`), so by source a plugin-launched lead
  connected through the dashboard daemon already lists `agent.*`. There is
  no pair rule in `filteredTools`; the agent.spawn plus mailbox.send pair
  is the render gate in `internal/mcp/playbook_tools.go#L1037`. The lead
  session that wrote this ticket nevertheless saw `pi.*` and no `agent.*`
  (live observation, not reproducible from source; likely a daemon or
  plugin build older than `768af2113`), so Phase 1 item 3 is a live
  re-check of what a plugin connection lists, and a listing change is built
  only for a gap the re-check finds.
- Stale Codex model list (owner report 2026-11-19 11:49, then Explore on
  `track/agent-v2` in daemon `4e8909a44`, introduced `795074e21`, hardened
  `34fe1526b`): after the owner updated the Codex CLI the dashboard still
  showed the old Codex models. `hostedChoices` prefers the in-memory
  `codexChoices` (`internal/wsagent/codex.go#L40`), filled from the
  app-server `model/list` when a Codex session opens (`codex.go#L215`,
  `codex_mapper.go#L530-L559`); it bypasses the 5 min `choicesTTL`
  (`support.go#L301-L305`) and lives for the whole daemon process, so a
  Codex lead opened before the update pins the old list until a daemon
  restart. The fallback file read of `$CODEX_HOME/models_cache.json`
  (`hosted_choices.go#L30-L90`) caches an empty or garbage parse as a 5 min
  success (`support.go#L334-L340`), the class of the agy empty-list fix in
  `34fe1526b`. The lead-creation dialog reuses `supChoices` without a
  refetch (`dashboard-leads.js#L43`) while the popover refetches on open
  (`dashboard-selection.js#L11-L19`). The `codex.<tier>` defaults and worker
  spawn resolve models from this list, so its accuracy is in scope. These
  line citations come from that Explore and are re-checked by the worker.
- agy precedent: agy lazily drops the largest ws tools when no lazy-tool
  gateway is available; commit `49bb7454` fixed it by marking agy
  connections' listed tools `antigravity/alwaysLoad` through the
  registration field `ToolsAlwaysLoad` (`internal/mcp/server_inventory.go#L100-L110`,
  `internal/wsagent/registration.go#L39-L40`), not by shrinking schemas.
  The size guard at source HEAD is a 2,000 B per-tool cap
  (`internal/mcp/tool_schema_size_test.go#L16`); a 1,500 B tightening is
  uncommitted work on branch `impl/track/agent-v2/tool-size-1500` and has
  not landed (pending impl/track/agent-v2/tool-size-1500). The Codex lead
  saw no cutoff on the full 79-tool list (see the lead observation above);
  a Codex worker connection is unmeasured.

## Decisions

- (166) A plugin-launched lead connection also lists `agent.*` (the lead
  session observed only `pi.*`; source HEAD already lists `agent.*` through
  the daemon proxy, see Background), so a plugin lead such as this track's lead spawns and
  steers Codex workers with the same `agent.*` tools a hosted lead uses.
  Hosted connections keep hiding `pi.*`. Owner 2026-11-19: "166=a^다만
  급선무라는 게 우선해서 정식 구현을 태우라는 거지 덕테이핑 하라는 건
  아니니까 그 점 염두해 주시고요." (priority means scheduling the proper
  implementation first, not duct-taping.)
  Rejected: a harness argument on `pi.spawn` (grows the `pi.*` family slated
  for removal next minor); moving this lead to a hosted Claude lead first
  (hosted Claude dogfood waits on prompt-assembly Phase 2).
- (167) Phase 1 probes before building: a Codex worker gets the real ws HTTP
  MCP, and the probe confirms on a worker connection what the hosted Codex
  lead already showed (`mcp__ws__<name>` names, no cutoff) against the common
  worker loadout of (174) as the tool-count baseline, and measures what is
  still unknown for a worker: `hostedSessionKeyRule` fill on
  a call without `session_key`, whether the worker gets Codex's
  `exec_command`, and whether the workspace-write sandbox covers file edits
  and a w5090 sync from the worktree. Rejected: assuming parity with the lead
  path for a worker (a worker connection is a different start path; agy lazily
  drops large tools without the alwaysLoad override).
- (168) Write-authority workers auto-approve inside a workspace-write sandbox
  rooted at the worker's cwd (`WorkerAutoApprove`, as agy); (172) holds the
  exact approval policy and the read-only authority split. Rejected: per-call approval
  prompts (a headless worker has no user to answer them). Per-run permission
  levels stay with 261115-feat-ws-permission-profile Phase 3.
- (169) Tool names: keep Codex's native MCP naming if the probe shows it
  accepts ws's dotted names; otherwise reuse the agy sanitize scheme through
  the `ToolNameScheme` descriptor. Rejected: a Codex-specific branch in
  common code. Evidence so far: the hosted Codex lead (hosted Codex lead
  dogfood 2026-11-19) lists every tool as `mcp__ws__<name>` with dots turned
  into underscores by Codex itself, so `ToolNameScheme` is likely unneeded;
  the worker probe only confirms a worker connection behaves the same.
- (170) Lead-side parity with `pi.*` (turn-end completion mail, follow-up to
  the same worker, transcript read, interrupt/stop) comes through the
  existing `agent.*` and mailbox tools; no new pi-style tool family
  (261117-feat-ws-agent-tools-mailbox-routing, (54)=b).
- (171) Compatibility tier T0: new descriptor values, a builtin worker
  profile, a Codex worker start path, and `agent.*` listing on plugin
  connections; no stored shape, hello, or tool-argument change (same tier as
  the agy worker role, (139)). A tool newly listed on a connection is
  additive. Source basis: an older daemon refuses a codex worker spawn
  (`spawnableHarness`, `internal/wsagent/agent_tools.go#L239-L241`), so only a
  newer daemon ever writes a codex worker record; the codex tier mappings
  already ship in `agents.tier` (`internal/wsconfig/config.go#L611-L664`); the
  `codex` harness value is already written by hosted leads. An older
  daemon that reopens a non-lead Record with Backend `codex` and Profile
  `agent-worker-codex` (after a daemon downgrade, for example a mail wake)
  reaches `CodexBackend.Start`'s "hosted lead sessions only" refusal
  (`codex.go#L71-L73`) or the missing builtin profile; it fails that start
  and changes or deletes no stored data, and reopening with the newer daemon
  resumes the record (display and start refusal only). The verdict holds
  only while the worker start path adds no stored field or tracker event type;
  a design that adds one re-opens the tier.
- (172) The Codex worker start path follows the agy worker shape: daemon-owned
  with no holder and no lead spec (`worker: spec.Lead == nil`, as
  `internal/wsagent/agy.go#L127`, `#L151`); the adapter replaces the
  `spec.Lead` reads (`DaemonVersion`, `PermissionMode`, `Spawned`, `Env`,
  `codex.go#L106`, `#L117`, `#L161`, `#L198`) with worker-start constants,
  because `BackendRegistration` has no sandbox field
  (`internal/wsagent/registration.go#L14-L44`) and the agy shape keeps them
  adapter-side; a write-authority child (`workdir-write` or `full`) runs in
  a workspace-write sandbox with approval policy `on-request` plus the traced
  auto-allow of `WorkerAutoApprove` (so commit and sync escalations are
  allowed and traced, keeping 261115-feat-ws-permission-profile's
  no-silent-auto-approval), and the catalog and permission reporting call
  that confinement advisory, not enforced. A `read-only`-authority child
  starts in Codex's native read-only sandbox with approval policy `never` on
  both `thread/start` and every `turn/start` (`Send` re-sends the policy,
  `codex.go#L232-L245`), so Codex refuses escalation itself and no request
  reaches the runner's `WorkerAutoApprove` answer
  (`runner_events.go#L307-L313`); reusing `codexMode("read-only")` would give
  `on-request` and the auto-allow would run escalated commands outside the
  sandbox. The adapter reads the child's authority from
  `spec.PermissionLevel`, which no worker path sets today (the worker's level
  is the local `level` at `runner_process.go#L58-L65`, reaching only
  `WS_MCP_PERMISSION_LEVEL`; the field is documented as lead intent only,
  `backend.go#L70-L72`): the runner's worker branch sets it from
  `childLevel`, a harness-neutral change no backend reads today, and the
  field comment is updated. Read-only on Codex is reported partial, not
  enforced: Codex's sandbox does not confine a user-config MCP server shell
  such as `git_bash`. Per-run profiles beyond this read-only/write split stay
  with 261115-feat-ws-permission-profile Phase 3. The read-only split was
  announced by the lead's coordinator 2026-11-19 (the lead will use Codex for
  review agents too). The path also adds an `agent-worker-codex` builtin
  profile with its instructions text.
  Rejected: giving the worker a holder as
  Pi workers have (`runner_process.go#L166-L169`; prior decision of
  261118-feat-ws-agy-tool-names-and-worker-role: workers are daemon-owned, not
  holder-held, tokens revoked on stop/shutdown).
- (173) The Codex model list stays current: the session-reported
  `codexChoices` follows the same TTL as the other choices and a newer
  session report replaces it; a `models_cache.json` that does not exist (a
  Codex never run on this machine) is an empty list with no error, while an
  existing file that is empty or unparseable is an error, so the 30 s fail
  TTL applies instead of a 5 min success; the lead-creation dialog refetches
  choices when it opens; one test per fix.
  Tier T0 (no stored shape, hello, or argument change). Owner report
  2026-11-19 11:49 is the evidence; confirmed by the lead's coordinator on
  2026-11-19 and kept in this ticket's scope because the `codex.<tier>`
  defaults and worker spawn depend on an accurate model list.
  Rejected: waiting for a daemon restart (the owner already hit it live).
- (175) Supersedes 261118-feat-ws-hosted-model-list-and-agy-bypass in two
  places, recorded as an Edition on that ticket's Phase 1: (a) its "a missing
  or malformed file is an empty list, not an error" contract
  (`hosted_choices.go#L26-L45`) becomes missing = empty list with no error,
  malformed or empty = error; (b) its non-expiring "a session's own report
  wins over the session-less listing" precedence (`support.go#L301-L305`)
  becomes a session report that expires on the shared TTL and is replaced by
  a newer report. Owner confirmed 2026-11-19 12:27 ("175=a"). Rejected: every
  missing file as an error (a never-run Codex would show a permanent error);
  keeping the old precedence and applying the TTL only to the file path.
- (174) A Codex worker gets the common `agent.spawn` worker loadout
  (`AgentWSTools`, `internal/wsagent/builtin.go#L108-L131`, enforced per
  connection at `internal/mcp/server_inventory.go#L98`, `#L133-L141`), not the
  full ws tool set. Owner confirmed 2026-11-19 11:57 ("174=ok"), on the
  lead's recommendation: the parent epic's cross-child invariant (85)=c in `261114-epic-ws-common-session-foundation`
  says "every child gets one worker-role ws tool loadout". The earlier "full
  ws tool set" wording was the lead's own draft, not an owner quote, and the
  owner agreed to this loadout; the owner
  asked for a Pi-worker replacement, which this loadout covers (git.*,
  tickets.*, playbook.*, path/route helpers, note.query, agent.*,
  mailbox.send). A wider loadout gets its own ticket only if a real gap
  appears in use. Rejected: a wider Codex-only loadout, because it reverses
  (85)=c.

## Constraints

- Manuals: `ai-docs/manuals/ws-mcp.md`, `release-compatibility.md`,
  `performance.md`, `shipped-surface-boundary.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- No local `go test` and no ws executables on the owner's office PC
  (antivirus); Go build/test on remote w5090 only. Live acceptance runs on
  the owner's Windows PC with the dashboard daemon.
- Harness-neutral common code: Codex specifics live in its registration
  descriptor and adapter, not in harness-name branches.
- Proper implementation, not a stopgap (owner (166) note): every piece lands
  as the shipped design, with tests, so later phases build on it rather than
  replace it.

## Prior Decisions

- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, b3e2e5e6): "Worker eligibility, profile, HTTP attachment and auto-approval are registration metadata; common launch/admission code gains no agy branch." — bearing: supports
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, b3e2e5e6): "Workers are daemon-owned, not holder-held. ACP receives the session token in the session request only... Worker tokens never enter the record and are revoked on stop/shutdown" — bearing: supports
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, b3e2e5e6): "Reserve the entire agent-worker- namespace in compatibility spawn guards rather than a second backend allowlist" — bearing: constrains
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-19, 49bb7454): "Use the MCP alwaysLoad metadata override rather than shrinking schemas; the per-backend descriptor confines it to agy connections." — bearing: constrains
- 261117-feat-ws-agent-tools-mailbox-routing (2026-11-18, ec81e84c): "Worker role is 'has a builtin agent-worker-<backend> profile' rather than a new BackendRegistration flag" — bearing: constrains
- 261117-feat-ws-agent-tools-mailbox-routing (2026-11-18, 768af211): "hiding is T0 for this release (handlers answer, so old sessions' calls keep working)" — bearing: constrains
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-17, 1b743aad): "C2 (codex/agy tiers advertised without a worker path) is fixed on the adapter side by lead-only scope here" — bearing: constrains
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "Delegation is always ws/agent.* regardless of harness... a single mailbox.send(to: <address>) serves children, peers and leads" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/dashboard.go, internal/wsagent/codex.go, internal/wsagent/builtin.go, internal/wsagent/agent_tools.go, internal/wsagent/runner_process.go, internal/wsagent/support.go, internal/wsagent/hosted_choices.go, internal/wsdashboard/assets/dashboard-leads.js, internal/mcp/server_inventory.go, internal/wsconfig/config.go |
| scope.surface | cross-module | wsagent adapter and builtin profiles, ws-mcp registration, mcp tools/list listing; no exported symbol named yet |
| scope.new_public_symbol | unknown | likely an agent-worker-codex profile constant or worker start option; not named in the ticket |
| scope.new_type_contract | unknown | a Codex worker start path with no lead spec or holder changes the adapter's StartSpec contract (codex.go#L71-L73); the shape is not designed |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/codex_admission_test.go, codex_test.go, codex_lead_test.go, internal/mcp/agent_backends_test.go; Go tests run on w5090 only |
| complexity.reuse_points | confirmed | agy worker descriptor fields and worker-aware adapter (registration.go#L18-L40, agy.go#L127, #L151), Claude and agy worker profile loop (builtin.go#L213-L228) |
| complexity.side_effect_risk | moderate | newly spawnable harness with auto-approve; Claude, Pi and agy spawn paths share the touched admission and profile code |
| risk.correctness | high | the Codex adapter refuses a start without a lead spec and holder, so the ticket's registration-only Phase 1 item 2 would still fail at start; the worker start path, the read-only/write authority mapping and the shell and sandbox reach are unprobed on Codex |
| risk.fit | moderate | follows the agy registration-metadata pattern, but the plugin-lead listing premise of (166) does not match source HEAD |
| risk.test | high | live acceptance covered only an echo tool, not the real ws tool set or Windows; no local go test allowed |
| risk.security_or_contract | high | worker auto-approve with workspace-write shell and file access, plus a newly listed tool surface on connections; T0 verdict to be checked per release-compatibility.md |

## Phases

### Phase 1: Codex worker path usable from this lead

Goal: the lead can spawn a Codex worker at a tier, the worker sees and calls
the common worker ws loadout of (174) in its worktree, and its turn-end report
reaches the lead's mailbox.

1. Probe (record evidence in the Result), run after the start path of item 2
   can launch a worker (items run in the order 2, 1, 3, 4, 5, 6): the hosted
   Codex lead already showed the `mcp__ws__<name>` naming and no cutoff, so
   confirm only that a worker connection behaves the same, with the
   `AgentWSTools` list of (174) (its length) as the tool-count baseline, and measure what
   is unknown for a worker: that a call without `session_key` is not filled
   (the hosted key rule applies to hosted lead connections only,
   `internal/mcp/hosted_key.go#L17-L25`; the worker gets its key through the
   instructions' key sentence and no fill is added here), whether the worker
   gets Codex's `exec_command` (a worker's shell depends on the user's Codex
   config), and whether the workspace-write sandbox with `on-request`
   auto-allow from (168) and (172) covers file edits, `git commit` in a
   linked worktree (its git common dir lies outside the cwd), and a w5090
   sync from the worktree. A result that contradicts (169) or (168), or that
   needs `danger-full-access`, returns to the lead as a decision; the other
   items pause only for that.
2. Worker path: set the worker descriptor fields on the Codex registration
   (WorkerCapable, WorkerProfile, WorkerHTTPMCP, WorkerAutoApprove, and
   ToolNameScheme only if the probe needs it); add the `agent-worker-codex`
   builtin profile with its instructions and the catalog worker row; make
   `CodexBackend.Start` accept a worker start per (172) (it refuses a start
   without lead spec and holder today). The `codex.<tier>` defaults in
   `agents.tier` already ship; verify them, do not re-add.
3. Lead reach per (166): re-check live which tools a plugin-launched lead
   connection lists on the current daemon and plugin build; if `agent.*` is
   already listed, record it as the evidence and change no listing code,
   otherwise fix the gap the re-check finds in the shared listing path.
4. Tests on w5090: spawn refusal lifted for codex, tier resolution, worker
   descriptor, the Codex worker start path, tool exposure on a Codex worker
   connection (the `AgentWSTools` list), and the authority mapping of (172)
   (write authority gets workspace-write plus `on-request` auto-approve;
   `read-only` gets the native read-only sandbox with approval `never` on
   `thread/start` and `turn/start`), tested through the Runner worker start
   path so the runner's `spec.PermissionLevel` assignment is exercised, not a
   hand-built spec; Claude, Pi and agy behavior
   unchanged.
5. Model list freshness per (173) and (175): the session-reported list
   follows the shared TTL and a newer report replaces it, a missing
   `models_cache.json` is an empty list with no error, an existing empty or
   unparseable one is an error on the 30 s fail TTL, and the
   lead-creation dialog refetches on open; one test for each on w5090 (the
   dialog refetch by the existing dashboard test style).
6. Live acceptance on the owner's PC: the lead spawns `codex.medium` in a
   pooled worktree, the worker edits a file, runs a w5090 build, commits, and
   its completion mail arrives, and the dashboard's Codex model list matches
   the installed CLI after a CLI update without a daemon restart, relying on
   a newer session report (a new app-server, such as the spawned worker) or a
   `models_cache.json` rewritten by the new CLI, named in the Result.

### Result (a85bcbe50) - 2026-11-19

Status: items 2, 4 and 5 landed and verified on w5090; items 1, 3 and 6 are
pending live (they need the owner's PC and a daemon built from this branch).

What landed (one commit, `a85bcbe50`):

- Item 2, worker path. The Codex registration declares `WorkerCapable`,
  `WorkerProfile` (`agent-worker-codex`), `WorkerHTTPMCP` and
  `WorkerAutoApprove`; the builtin `agent-worker-codex` profile carries the
  common ws loadout (`AgentWSTools`) and a Codex worker instruction text that
  points at `agent.spawn`/`mailbox.send` instead of Codex's native subagent
  tools; the catalog gained worker-role rows (approval graded Substitute).
  `CodexBackend.Start` accepts a holderless worker start (`worker = Lead == nil`)
  and publishes the child PID for the MCP hello. `startWorker` now passes the
  spawn authority as `StartSpec.PermissionLevel`; the backend maps it per (172):
  read-only runs `read-only` with approval `never`; workdir-write and full run
  `workspace-write` with `on-request` plus the traced auto-allow; any other
  level is refused. A worker refuses `SetPermissionMode`. Common code has no
  harness-name branch.
- Item 5, freshness. The session-reported choices carry a timestamp and expire
  on `choicesTTL`; a newer report replaces them. A missing `models_cache.json`
  yields an empty list without error; an existing empty, unparseable or
  model-less file is an error on the 30 s fail TTL. The spawn popover and the
  session menu refetch choices on every open and redraw without losing the form
  (`dashboard-leads.js`). `ai-docs/manuals/ws-mcp.md` updated.
- Item 4, tests: `codex_worker_test.go` (launch authority and approval through
  the runner path, mode and start refusals, mode-change refusal),
  `TestCodexWorkerProfileAndAdmission`, updated catalog/builtin/descriptor
  tests, hosted-choices TTL/cache-file tests, and a dashboard test for the
  popover refetch.

Verification (w5090, `go build ./...`, `go vet`, `go test`):

- `./internal/wsagent`: ok. `./internal/wsdashboard`: ok (includes the node
  popover tests, which also pass locally with `node --test`).
- `./internal/mcp`: one failure on the full run (`TestConfigListAgentProfileKnob`
  listed builtin profiles without `agent-worker-codex`); fixed and rerun with
  `-run "Profile|Agent|Tier|Worker|Codex"`: ok. The full package was not rerun
  after that one-line list fix.
- `./cmd/ws-mcp`: only `TestHostedLauncherSurvivesReplacementAndRestartsRecordedBuild`
  failed, on the missing gitignored `generated/inventory.json` (known; noted per
  the lead's instruction). Everything else in the package passed.

Decisions: (172) applied as above; tier T0 (no stored field, no tracker event,
no tool/argument change; an old binary has no Codex worker row). Confinement is
advisory, recorded in the catalog source text.

Review: R1 lite, one pass: 0 Critical, 0 Important, 4 Minor (no R2).

Pending live (owner's PC, daemon built from `impl/track/agent-v2/maybe-romp-magma`):

- Item 1, probe: against the installed Codex CLI confirm `thread/start`
  accepts the string `sandbox` and `approvalPolicy`, that `read-only` plus
  `never` refuses a write, and that `workspace-write` plus `on-request` raises
  `item/commandExecution/requestApproval` which the runner auto-allows.
- Item 3, lead reach: from a plugin lead connection, `agent.list`/`agent.tiers`
  show a `codex.*` tier and `agent.spawn` accepts it (no listing code change was
  needed unless a gap shows).
- Item 6, acceptance: spawn `codex.medium` in a pooled worktree; the worker
  edits a file, runs a w5090 build, commits, and its completion mail arrives in
  the lead mailbox. Then update the Codex CLI (or rewrite `models_cache.json`)
  and confirm the dashboard model list follows without a daemon restart, naming
  which route carried it (a newer session report from the new app-server, or the
  rewritten cache file).

Forward (Minor, from R1):

- `codex.go` `Start` validates the stored mode before the attach return, so a
  holder reattach with an unrecognised stored mode now fails at `Start` rather
  than on the next `Send`; make it lenient if that matters.
- The read-only `answered == false` assertion in `codex_worker_test.go` holds
  because the stand-in sends no approval when policy is `never`; the real guard
  is the `thread/start`/`turn/start` policy assertions.
- Tier resolution is only checked as `rec.Model != ""` (not the `codex.small`
  default), tool exposure only via the `EnvAllowedTools` hello value, and
  `PermissionFull` only by the `codexWorkerMode` unit test.
- Test helpers restore `codexChoices.models`/`.efforts` but not the new `.at`.
