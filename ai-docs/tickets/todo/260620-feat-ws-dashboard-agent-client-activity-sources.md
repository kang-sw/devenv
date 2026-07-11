---
title: ws dashboard agent-client activity sources
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260514-epic-ws-web-dashboard-mvp: predecessor dashboard MVP board with reusable Activity Console and workbench surfaces
  260622-research-ws-dashboard-ferrule-session-binding: session-binding model that future provider adapters must build on
  260605-research-ws-native-subagent-pivot: supersedes dashboard deprecation and preserves the web dashboard while moving agent visibility away from removed agents.* surfaces
  260523-feat-ws-dashboard-main-session-activity-source: prior main-session freshness gap that must be re-grounded on host-owned agent/client activity
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: serverId forwarding must carry Activity source identity before remote provider streams are transparent
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
---

# ws dashboard agent-client activity sources

## Background

The dashboard was briefly marked for deprecation during the native-subagent
pivot, then explicitly retained as a usable web-tmux-style surface. The retained
surface should not restart direct harness development. As of the 2026-06-24
dashboard direction discussion, this structured Activity adapter track had been
deferred back to idea level under
`260622-epic-ws-dashboard-session-key-realignment`, with
`260624-feat-ws-dashboard-managed-cli-terminal`'s terminal-first managed vendor
CLI surface treated as the nearer milestone instead.

**Superseded 2026-07-11** (owner): that ordering is reversed. Now that this
session has confirmed Codex app-server, OpenCode ACP, and the Claude CLI's
headless stream-json duplex mode are all viable structured, duplex-capable
substrates (see Decisions below and `260620` Phase 4's research), the
terminal-first PTY approach is no longer the pragmatic near-term path — the
structured provider-adapter track this ticket owns is now the priority
implementation direction, ahead of `260624-feat-ws-dashboard-managed-cli-terminal`.
The terminal-first milestone remains valid future work (a plain shell terminal
still has its own use), but it is no longer positioned as unblocking a
too-far-off structured milestone; both `260624-feat-ws-dashboard-managed-cli-terminal`
and this ticket should reflect the new ordering.

When this idea is revisited, the dashboard still needs a small, source-neutral
activity bridge that can read host-owned agent surfaces such as Codex app-server
and OpenCode ACP, normalize their session, turn, message, tool, permission, and
status events, and render them through the existing WorkRoot Activity / Activity
Console model. OpenCode serve remains a useful read-only observation surface,
but it is not the primary counterpart to Codex app-server when the dashboard
needs an interactive agent-client bridge.

The existing Activity Console read model is the right public shape: source-
neutral `ActivityItem` rows, selected transcript backfill, bounded diagnostics,
and compatibility `agents` projection for the legacy named-agent pane. The stale
part is the assumption that named-agent wsstate/SQLite data is the main activity
authority. After the 260605 pivot, ws-owned subprocess agents survive only as the
scoped `ws.mercenary.*` path, while native Codex/OpenCode work should be treated
as host-owned agent-client data.

## Decisions

- Preserve the dashboard as a browser control plane. Do not replace it with a
  TUI and do not strip Activity visibility merely because `agents.*` was removed.
- Avoid direct harness/runtime development. The common layer is a dashboard-owned
  ACP-shaped `AgentClientProvider` contract plus Activity projection, not a new
  model loop, edit engine, permission runtime, MCP authority, or ws MCP agent
  runtime.
- Use Codex app-server, OpenCode ACP, and Claude (via the `claude` CLI's
  headless stream-json duplex mode) as the primary interactive provider
  sources. Codex app-server supplies JSON-RPC thread/turn/item/event data over
  its own protocol; OpenCode ACP supplies editor-agent JSON-RPC over stdio.
  Adapters translate all three into the dashboard provider contract before
  projecting Activity rows and transcript blocks.
- **CLI stream-json duplex, not the Claude Agent SDK, as the Claude provider
  substrate** (owner, 2026-07-11, revised from the 2026-07-11 Agent SDK
  decision same day after research): the Claude Agent SDK is itself just a
  wrapper that spawns the `claude` CLI as a subprocess per `query()` call, so
  the dashboard drives the CLI binary directly instead of adding an SDK
  dependency. The CLI supports a headless duplex-shaped mode via
  `--input-format stream-json --output-format stream-json` (NDJSON over
  stdin/stdout, kept alive across multiple turns rather than exiting per
  turn) — conceptually the same shape as Codex's app-server JSON-RPC duplex.
  Documented event kinds include `system` (init: `session_id`, model, tools,
  capabilities), `assistant` (content blocks: text/tool_use/thinking),
  `stream`/`stream_event` (partial deltas), and `result` (`session_id`, cost,
  token counts). **Unverified**: the finer-grained bidirectional
  `control`/`control_request`/`control_response` message shape (used for
  in-band permission prompts and MCP calls) is only documented in a
  third-party reverse-engineered reference
  (Roasbeef/claude-agent-sdk-go's `cli-protocol.md`), not Anthropic's official
  docs (confirmed via `code.claude.com/docs/en/headless` and open GitHub
  issue #24594 noting stream-json input is undocumented beyond the flags
  table) — this needs direct fixture verification against an installed
  `claude` binary before Phase 4 implementation, not assumed from that
  third-party doc.
- **Permission/approval interception mechanism** (owner direction via
  `260711-idea-dashboard-agent-facing-mcp-control-surface`'s 3-way
  allow-once/allow-pattern/deny + separate dangerously-bypass design,
  grounded 2026-07-11): do not drive Claude via
  `--dangerously-skip-permissions`/`--permission-mode bypassPermissions` for
  the relay-based approval path — that flag is confirmed to have known
  headless failure modes when a prompt still needs to surface (GitHub
  #54850: process halts with no way for a human to respond in a TTY-less
  stream-json context) and refuses to start entirely under root/sudo. The
  officially-supported interception point instead is CLI-level **hooks**
  (`PreToolUse` etc., configured via `.claude/settings.json`, not an
  SDK-only feature — confirmed working the same way when driving the raw
  CLI binary): a hook can return an allow/block/ask decision per invocation,
  which is the mechanism the dashboard's approval relay should hook into.
  The dangerously-bypass mode maps directly to
  `--permission-mode bypassPermissions` when the human has explicitly
  opted into it for that session.
- **Process lifecycle: kill-and-respawn via `--resume`, opaque
  running/idle rendering** (owner, 2026-07-11): a Claude CLI provider
  process only needs to stay alive while a turn or an active shell
  subprocess it spawned is running; otherwise-idle sessions should be
  killed after a timeout and transparently respawned via
  `claude --resume <session_id>` on the next input, rather than held open
  indefinitely. The browser should render a "running" and a
  "killed-but-resumable" session as visually indistinguishable/opaque
  states, so a human naturally experiences resuming an old chat rather than
  perceiving a restart. This fits ws-mcp's existing stateless design (low
  contradiction risk) and turns the SDK's per-`query()`-respawn behavior
  (flagged as a downside in the original SDK research) into a non-issue,
  since respawn is the intended steady state between turns anyway.
  `--resume` session lookup is confirmed scoped to the current project
  directory and its git worktrees (official docs); the precise disk/
  transcript dependency and resume-after-mid-turn-kill failure modes remain
  **unverified** and need direct testing before this phase ships.
- **Quota/ToS risk: proceed now, revisit if it becomes a problem** (owner,
  2026-07-11): whether driving Claude via subscription OAuth from an
  embedded dashboard provider is inside Anthropic's ToS for
  product-embedded SDK/CLI use is unresolved upstream (conflicting signals
  found in research: a separate SDK-specific credit pool was reportedly
  introduced and then reportedly paused/merged back into subscription
  quota around the same date). Owner explicitly accepts this as a known,
  deferred risk given immediate personal-use urgency — implement now,
  address ToS/quota fallout later if it actually surfaces, rather than
  blocking on upstream policy clarity that may not resolve soon.
- **Opinionated subset per provider, not feature parity across harnesses**
  (owner, 2026-07-11): each provider adapter (Codex app-server, OpenCode ACP,
  Claude CLI stream-json) should implement only the slice of that harness's
  capabilities the dashboard actually needs to project through the shared
  ACP-shaped contract — not attempt to mirror every feature a given harness
  exposes. Harnesses differ in what they offer natively; the dashboard's
  provider contract is the ceiling, and any one adapter may cover less than
  its harness technically supports if the extra surface has no dashboard
  consumer yet. This keeps adapter scope bounded and avoids the "recreate
  harness development" risk this ticket already guards against.
- **Cross-provider common interactive subset** (elevated 2026-07-11 as the
  explicit shared design object spanning all three confirmed duplex-capable
  harnesses — Codex app-server, OpenCode ACP, Claude CLI stream-json):
  define an ACP-shaped internal subset instead of adopting any provider wire
  protocol as the dashboard API. The subset should cover provider
  initialization/capabilities, session list/create/resume, prompt/send,
  assistant/user messages, tool activity, permission/blocked states,
  interruption/cancellation status, file-change summaries, transcript backfill,
  and provider metadata. Codex-specific or OpenCode-specific fields remain
  adapter-private unless a browser-facing dashboard contract needs them.
  **Known asymmetry**: "session list" is not uniform across the three —
  Codex app-server and OpenCode ACP are expected to expose a live
  list-stored-sessions call, but Claude's CLI has no equivalent live list
  method (only `--resume <session_id>` against a known id); Claude session
  discovery can only come from reading its on-disk history store directly
  (see `260624-feat-ws-dashboard-managed-cli-recent-sessions`). This is why
  the cross-harness "recent sessions" collapse should be built on
  vendor-history-file scraping as the discovery mechanism for all three
  (uniform, works even with no live provider process running), while this
  subset's `resume` capability is what actually reopens a selected entry
  live once chosen — the two tickets are complementary, not redundant.
- **Per-harness capability grounding for the frontend interaction-API draft**
  (research, 2026-07-11, requested to vet the owner's draft frontend
  interaction-API list against Claude's unusually rich baseline before
  committing to a common subset): findings below per capability, each flagged
  confirmed/unverified since none were fixture-verified this pass (WebSearch/
  WebFetch only, no installed-binary spike).
  - **Rewind/fork-from-a-point**: Codex app-server has explicit native
    support — `thread/fork` (branch a new thread from an earlier point in a
    parent thread's rollout history) and `thread/rollback` (undo to a
    specific point in the same thread), tracked server-side via
    `thread_spawn_edges`; exact fork-point granularity (message vs. turn
    level) is unverified. ACP (OpenCode) has `session/load` (resume with
    full history replay) but no documented fork-a-new-branch-from-an-
    earlier-point method in the core spec — unverified whether OpenCode
    exposes one outside core ACP. Claude CLI has no documented native
    fork/rewind method at all; only `--resume <session_id>` replays a whole
    session from its last state, so rewind-to-a-point would need a
    dashboard-owned workaround (copy the `~/.claude/projects/.../*.jsonl`
    transcript file, truncate it at the desired point, then `--resume`
    against the truncated copy) — unverified whether the CLI accepts a
    hand-truncated transcript on resume. **Not safe as a common-subset
    primitive**; treat as Codex-native / OpenCode-partial / Claude-workaround
    per-harness, not a shared frontend contract method.
  - **Context-window/token introspection and compaction control**: ACP has a
    standardized, *recently finalized* (2026-06-05) `session/update`
    notification variant (`sessionUpdate: "usage_update"`, fields `used`/
    `size`/optional `cost`) — a real, versioned protocol feature, so this is
    the strongest candidate for a common-subset field, but only if the other
    two harnesses expose an equivalent. Codex app-server emits token usage
    statistics on `turn/completed` (confirmed to exist, exact metric fields
    unverified) and has an explicit `thread/compact/start` RPC (client-
    triggered compaction, not just automatic) — good support. Claude CLI's
    `result` event includes token counts per the existing Phase-4 note in
    this ticket, but compaction triggering is not documented as
    client-controllable at all (auto-compact only) — no equivalent to
    `thread/compact/start`. **Verdict**: context-window *display* (used/size)
    is safe to standardize as a common-subset field (all three have some
    token-count signal); *compaction-timing control* is not — Codex supports
    it, OpenCode's ACP-level story is unclear, Claude has no known client-
    side trigger, so this should degrade to "read-only usage display,
    Codex-only manual-compact button" rather than a universal control.
  - **Subagent introspection with per-subagent transcript streaming**: no
    harness documents a clean listing/streaming RPC for this. Codex mentions
    a "Guardian subagent" as a fixed built-in reviewer role, not a general
    listable/streamable subagent registry. OpenCode's subagent model spawns
    child sessions with `context: "fork"` that are themselves ordinary ACP
    sessions (a child runs in its own ACP harness session on the same
    background lane as native sub-agent spawns) — meaning a subagent may be
    representable as just another `session/*`-addressable session rather
    than a distinct concept, which is actually a reasonably strong signal
    for the dashboard's own model (treat subagents as regular child
    sessions, not a separate subagent RPC surface). Claude Code's subagents
    have no documented CLI-level (non-SDK) introspection/listing surface in
    the stream-json duplex mode researched so far. **Verdict**: unverified/
    weak across all three; do not commit to a dedicated "subagent list +
    per-subagent transcript stream" common-subset API yet — if OpenCode's
    child-session-is-just-a-session model holds up under a real fixture
    check, the dashboard could model subagents as ordinary nested
    Activity/session rows instead of inventing a new capability, which
    would sidestep the parity problem entirely.
  - **Skill/capability listing**: Codex app-server has a confirmed
    `skills/list` RPC (response shape unverified). OpenCode/ACP has no
    documented skills-list method in the core protocol so far (`Agents` docs
    describe configuring agents/modes, not a runtime capability-listing
    call) — unverified. Claude CLI's `system` init event reports `tools`
    and `capabilities` at session start (from this ticket's existing Phase-4
    grounding) but that is session-start metadata, not an on-demand listing
    RPC, and does not obviously cover project-local `SKILL.md`-style skills.
    **Verdict**: not safe as a uniform common-subset call yet; Codex has the
    clearest native support, the other two are unverified-or-absent —
    revisit after a fixture pass rather than assuming parity.
  - **"Goal"/loop native support**: no harness researched has a native
    goal/repeat-until-condition primitive. Codex's app-server surface is
    organized around Threads/Turns/Items with no looping/goal-state RPC.
    ACP's closest concept is `Plan`/`PlanEntry` (the agent reports its own
    task breakdown/strategy as it works) plus `CurrentModeUpdate` (mode
    switching, e.g. ask/architect/code) — neither is a client-driven
    "keep going until X" loop; both are the *agent* reporting its own
    plan, not the client commanding a loop. No evidence found of a
    Claude-CLI-native equivalent to Claude Code's own `/goal`-style
    behavior at the headless stream-json layer. **Verdict**: calibrated to
    OpenCode's ACP-standard level as the owner requested, the ceiling is
    "the agent can expose its own plan," not "the client can command a
    repeat-until-condition loop" — so goal/loop functionality should be
    dashboard-built (drive it by re-sending prompts/turns from the dashboard
    side based on the dashboard's own condition-check), not attempted as a
    harness-native common-subset feature for any of the three.
  - **Overall recommendation**: of the owner's draft list, safe to commit to
    the shared frontend interaction API now: workroot history list-up,
    start/resume a specific conversation, start a new conversation with
    harness choice, send/receive messages, ticket/spec-stem link detection
    (pure frontend text-parsing, no harness dependency), the Enter/Ctrl+Enter
    input box (pure frontend), and read-only context-window usage display.
    Needing explicit per-harness degradation or later phases: compaction-
    timing control (Codex-only control, others read-only-at-best),
    rewind/fork-from-a-point (Codex-native, Claude-workaround-only,
    OpenCode-unverified), skill-listing (Codex-native only so far). Needing
    a design rethink rather than a shared API: subagent introspection —
    consider modeling subagents as ordinary nested sessions/Activity rows
    instead of a bespoke subagent-list-and-stream capability, pending a
    fixture check of OpenCode's child-session behavior. Should stay
    dashboard-built rather than harness-native: goal/loop functionality.
    None of this pass's findings were fixture-verified against installed
    binaries; treat all three per-harness verdicts above as directional,
    not final, until a spike (already planned for Claude in Phase 4) is
    run for Codex app-server and OpenCode ACP too.
- Treat OpenCode serve as an optional observation/read-only supplement. It can
  help discover sessions or stream HTTP/OpenAPI/SSE state, but the first
  OpenCode counterpart to Codex app-server is `opencode acp`, not `opencode
  serve`.
- Keep browser identity dashboard-native: `serverId`, `workspaceId`,
  `workRootId`, `activityId`, and transcript cursors. Do not expose provider
  session ids, ws `session_key` values, cache paths, transcript paths, process
  ids, or raw provider event ids as browser authority.
- Keep browser Activity read-only for this track. The provider contract may model
  interactive capabilities so Codex app-server and OpenCode ACP can share one
  adapter shape, but exposing start, interrupt, cancel, retry, erase, permission
  approval, or provider-specific steering controls in the dashboard UI requires
  later high-friction control tickets.
  - **Tension flagged, not resolved (2026-07-11)**: Phase 1's
    "frontend interaction-API draft" below — `activity.session.create`,
    `activity.session.start`, `activity.session.send`, and the
    per-harness-gated compact/rewind/skills controls — is explicitly
    interactive/write, not read-only. This directly contradicts the
    read-only decision stated in this same bullet and in Constraints
    below ("the browser must not expose those controls in this track").
    Today's session drove the API-list design as if interactive control
    were in scope for this ticket, which is a real shift from the
    original read-only Activity Console framing, not a clarification of
    it. This is not silently resolved here — the owner should explicitly
    decide whether this ticket's scope now includes interactive
    session control (superseding the read-only decision), or whether the
    interactive API draft belongs in a split-off ticket that inherits
    this one's provider/subset work but is gated as its own
    higher-friction control surface, consistent with how
    `260711-idea-dashboard-agent-facing-mcp-control-surface` already
    treats execution-approval as higher-tension than read/display
    actions.

## Prior Art

- `ActivityFeed.items`, `ActivityTranscript`, and `TranscriptBlock` are already
  source-neutral enough to carry native agent-client activity.
- `ActivityFeed.agents` is a compatibility projection for the existing named-
  agent pane; new provider activity should flow through `items`.
- The current event vocabulary already supports item upserts/removals,
  transcript updates, snapshot invalidation, mode changes, and heartbeats.
- The dashboard daemon is explicitly not ws MCP root, harness, model-backend, or
  agent-session authority. It consumes provider state through daemon-owned view
  models and provider adapters.

## Constraints

- Provider adapters must degrade unknown or malformed events into bounded status
  or diagnostic transcript blocks instead of leaking raw JSON, paths, session ids,
  or command output payloads outside the selected transcript surface.
- Adapter contracts must be fixture-backed by captured provider events or schema
  snapshots because Codex app-server, OpenCode ACP, and OpenCode serve surfaces
  can drift by installed version.
- The first implemented subset is limited to read/list/stream/render behavior:
  source connection state, thread/session/activity rows, turn lifecycle,
  assistant/user messages, tool or command summaries, file-change summaries,
  blocked/approval-needed state, model/cwd/git metadata, and transcript backfill.
  Interactive provider methods may exist in the internal contract only when
  needed to represent the shared Codex app-server/OpenCode ACP lifecycle; the
  browser must not expose those controls in this track.
- Provider-specific controls remain out of scope. Adding UI controls before the
  read model and provider lifecycle mapping are stable would recreate the
  discarded harness-development problem.
- Server-scoped dashboard operation is a dependency for linked remote hosts.
  Activity source ids, stream subscriptions, transcript routes, and persisted UI
  state must include or derive `serverId` before remote Codex/OpenCode activity
  can be transparent.

## Public Interface Briefing

This track intentionally touches the dashboard's browser-facing Activity
interface, but the first implementation should keep the route set stable.
Existing routes remain the public entrypoints:

- `GET /api/dashboard/work-roots/{workRootId}/activity`
- `GET /api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript`
- `GET /api/dashboard/work-roots/{workRootId}/activity/events`

The public change is inside the source-neutral payload contract. `ActivityFeed`
continues to expose `items` as the primary Activity list and `agents` as the
legacy named-agent compatibility projection. New Codex app-server activity must
appear through `items`; it must not be forced into `agents`. `activityId` values
may gain a new dashboard-owned source prefix, but provider thread ids, turn ids,
session ids, raw event ids, process ids, and cache/transcript paths must remain
daemon-private. `ActivitySourceDisplay.kind`, item `kind`, and transcript
`renderKind` may gain new string values, so frontend parsers and tests must keep
unknown-value tolerance while rendering useful labels for known Codex values.

The watch stream may emit existing `ActivityConsoleEvent` variants for Codex and
OpenCode rows: item upserts/removals, transcript updates, snapshot invalidation,
mode changes, and heartbeats. The event vocabulary should not grow until a
concrete Codex app-server or OpenCode ACP behavior cannot be represented by the
current variants.

Transcript payloads continue to use `TranscriptBlock` as the only browser
backfill format. Codex app-server item/notification records and OpenCode ACP
messages/events must normalize into bounded user, assistant, tool/command,
file-change, approval-needed, status, and diagnostic blocks. Unknown provider
records degrade to bounded diagnostic/status blocks rather than leaking raw
provider JSON.

## Phases

### Phase 1: Agent-client provider and Activity source contract

Define the dashboard-owned ACP-shaped `AgentClientProvider` subset and the
Activity source projection that consumes it. This phase should update the ws web
dashboard spec and mental model so the intended source split is recoverable:
legacy/ws mercenary state is one compatibility source, Codex app-server is one
interactive provider source, OpenCode ACP is one interactive provider source,
OpenCode serve is an optional observation source, and the browser sees only the
dashboard Activity model.

**Frontend interaction-API draft** (owner + capability-matrix research,
2026-07-11; consolidates the owner's initial list against the per-harness
grounding in Decisions above — names are illustrative, not a final route
contract):

- `activity.history.list(workRootId)` — cross-harness collapsed conversation
  history in one call (see `260624-feat-ws-dashboard-managed-cli-recent-sessions`
  for the discovery-via-history-file mechanism this dispatches over); common
  subset, safe now.
- `activity.session.start(workRootId, activityId)` — resume a specific
  history entry; the entry is bound to whichever harness produced it, so this
  call resumes through that harness's own mechanism (Claude `--resume`, Codex
  thread resume, OpenCode ACP session resume/load). Common subset, safe now.
- `activity.session.create(workRootId, harness, profile?)` — start a new
  conversation with an explicit harness choice; per-harness setup itself
  (auth, CLI install) is assumed already done by the user outside the
  dashboard. Common subset, safe now.
- `activity.session.send(activityId, message)` plus the existing streamed
  Activity event path for receiving — turn-by-turn message exchange. Common
  subset, safe now.
  - Ticket/spec-stem detection in rendered message text, converting a
    recognized stem into a clickable link that opens the existing document
    viewer/file popup — pure frontend text-parsing against already-rendered
    transcript content, no harness/provider dependency, no new backend
    method needed.
  - Composer input: `Enter` inserts a newline, `Ctrl+Enter` submits — pure
    frontend behavior, mirrors the already-decided
    `260624-feat-ws-dashboard-managed-cli-terminal` Phase 2 composer
    contract exactly, so this should reuse rather than re-decide that
    behavior.
- `activity.session.usage(activityId)` — read-only context-window/token
  usage display (used/size). Common subset, safe now: all three harnesses
  expose some token-count signal (ACP's standardized `usage_update`, Codex's
  `turn/completed` stats, Claude's `result` event counts), even though only
  Codex additionally supports triggering compaction.
- **Per-harness-gated, not common-subset** (expose only when the active
  harness's adapter reports the capability; degrade to hidden/disabled
  control otherwise, not a uniform method):
  - `activity.session.compact(activityId)` — manual compaction trigger.
    Codex-native (`thread/compact/start`); no confirmed equivalent for
    OpenCode or Claude (auto-only).
  - `activity.session.rewind(activityId, atPoint)` /
    `activity.session.fork(activityId, atPoint)` — Codex-native
    (`thread/rollback` / `thread/fork`); Claude has no native method and
    would need a dashboard-owned workaround (copy + truncate the
    `~/.claude/projects/.../*.jsonl` transcript, then `--resume` against the
    copy — unverified whether the CLI accepts a hand-truncated transcript);
    OpenCode unverified in core ACP.
  - `activity.session.skills(activityId)` — list invokable
    skills/commands/plugins. Codex-native (`skills/list`); OpenCode and
    Claude have no confirmed on-demand listing call (Claude only reports
    `tools`/`capabilities` at session-start, not project-local skills).
- **Deliberately not modeled as a bespoke API**: subagent
  listing-plus-per-subagent-transcript-streaming. No harness has clean
  native support for this shape. Pending an OpenCode fixture check of its
  child-session-as-ordinary-session behavior, the dashboard should consider
  modeling a subagent as an ordinary nested Activity/session row under its
  parent `activityId` (reusing `activity.session.start`-shaped access to
  that child row for the "click bubble to open its transcript" UX) instead
  of inventing a dedicated subagent RPC surface.
- **Deliberately dashboard-built, not harness-native**: `/goal`-style
  repeat-until-condition looping. No harness exposes a client-driven loop
  primitive (ACP's `Plan`/`PlanEntry` is the agent reporting its own plan,
  not the client commanding repetition) — if built, this should be
  dashboard-side orchestration that re-sends prompts/turns based on a
  dashboard-evaluated condition, not a provider-contract method.

All per-harness verdicts above are directional (WebSearch/WebFetch research,
not fixture-verified); the Codex/OpenCode/Claude verification spikes noted in
Decisions must confirm or revise this list's per-harness gating before it
becomes a route contract.

Verification boundary: documentation and type-level tests or fixtures are enough
for this phase; no provider process needs to run. The result must make it clear
which methods and fields belong to the internal provider contract, which fields
are dashboard browser identity, which provider ids stay daemon-private, which
OpenCode serve data is observation-only, and which stale named-agent/SQLite
assumptions remain compatibility-only. Route and TypeScript contract tests should
assert that mixed source rows keep using the existing Activity routes and that
browser payloads tolerate unknown future source/provider kinds without treating
them as named agents.

### Phase 2: Codex app-server read adapter

Add a Codex app-server provider that can connect through a local transport, read
or subscribe to thread/turn/item state, map Codex lifecycle concepts into the
ACP-shaped provider subset, and project native Codex activity into `ActivityItem`
rows and `TranscriptBlock` backfill. Prefer generated or captured schema
fixtures over handwritten assumptions. For the first dogfood path, prefer the
default stdio transport and a minimal JSON-RPC subset: initialize, read/list
stored threads when available, start a thread/turn for smoke verification, and
consume thread/turn/item notifications into Activity rows and transcript blocks.
Unknown event types must degrade without breaking the whole Activity feed.

Verification boundary: fixture projection tests for representative Codex
thread/turn/item sequences, route tests proving browser payloads omit provider
session ids and raw paths, and a local smoke path that can be run when Codex
app-server is available. The WSL smoke should run against the locally installed
`codex app-server --stdio` binary and prove that a real turn can appear in the
dashboard Activity Console without requiring dashboard owner pairing when the
daemon is explicitly started through the loopback-only no-auth debug profile.

### Phase 3: OpenCode ACP provider adapter

Add an OpenCode ACP provider that starts or attaches to `opencode acp`, speaks
the ACP stdio JSON-RPC flow, maps OpenCode sessions/messages/tool activity and
permission states into the same ACP-shaped provider subset, and projects the
result into the same Activity model as Codex app-server. Keep this adapter
independent of Codex-specific assumptions; the common contract is the dashboard
provider subset and Activity projection, not either provider's wire protocol.
OpenCode serve may be used only as an optional observation/discovery supplement
if a concrete gap appears that ACP does not cover cheaply.

Verification boundary: fixture projection tests for OpenCode ACP messages/events,
bounded degradation tests for missing binary/auth, subprocess startup failure,
unreachable or incompatible ACP server state, and version drift, plus route tests
matching the same privacy and identity constraints as the Codex adapter.

### Phase 4: Claude CLI stream-json duplex adapter

Add a Claude provider that drives the `claude` CLI binary directly in headless
`--input-format stream-json --output-format stream-json` mode (no Agent SDK
dependency), implementing a dashboard-owned duplex bridge (no native
Codex-app-server-equivalent protocol exists for Claude, so this phase builds
the bridge rather than adapting an existing one). Map the CLI's `system`/
`assistant`/`stream_event`/`result` NDJSON events into the same ACP-shaped
provider subset the Codex and OpenCode adapters use, and project the result
into the same Activity model. Approval/permission interception goes through
CLI hooks (`PreToolUse` via `.claude/settings.json`), not
`--dangerously-skip-permissions`, so the dashboard's own approval-relay UX
(`260711-idea-dashboard-agent-facing-mcp-control-surface`) stays the
decision-maker; the dangerously-bypass mode maps to
`--permission-mode bypassPermissions` only when a human has explicitly opted
in for that session. Process lifecycle follows the kill-and-respawn-via-
`--resume` model from the Decisions above: idle sessions (no running turn or
child shell process) may be killed after a timeout and transparently resumed
on next input, rendered opaquely as the same ongoing conversation. Per the
opinionated-subset decision above, implement only the slice of CLI capability
the dashboard's provider contract actually needs; do not attempt full CLI
feature coverage.

Before implementation, first spend a short fixture-verification spike against
an installed `claude` binary to confirm: the exact stream-json event shapes
actually emitted (the bidirectional `control`/`control_request` shape is only
documented in an unofficial third-party reference and must not be assumed),
`--resume` behavior across process kill vs. clean exit and across working-
directory changes, and hook-based permission interception actually firing in
headless stream-json mode. This spike's findings should update this ticket's
event-shape assumptions before the fixture/projection tests below are
written.

Verification boundary: fixture projection tests for Claude CLI
session/turn/message/tool sequences (captured from the verification spike
above, not handwritten from unofficial docs), bounded degradation tests for
missing binary/auth, hook-misconfiguration, or unreachable/incompatible CLI
version state, and route tests matching the same privacy and identity
constraints as the Codex and OpenCode adapters.

### Phase 5: Activity UI and server-scoped integration

Lift the visible Activity UI from named-agent wording to source-neutral
agent-client activity. Preserve the existing Activity Console ergonomics: dense
ribbon, selected transcript, local dirty acknowledgement, bounded tail/backfill,
and read-only behavior. Thread `serverId` through Activity source selection and
stream keys before linked remote providers are considered transparent.

Verification boundary: frontend route/model tests for source-neutral labels and
identity keys, browser-level acceptance evidence for mixed source rows and
transcript rendering, and server-scoped route tests showing local compatibility
aliases still map to `server-local`.
