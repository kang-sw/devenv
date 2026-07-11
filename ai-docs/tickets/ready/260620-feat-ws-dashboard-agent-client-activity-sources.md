---
title: ws dashboard agent-client activity sources
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260514-epic-ws-web-dashboard-mvp: predecessor dashboard MVP board with reusable Activity Console and workbench surfaces
  260622-research-ws-dashboard-ferrule-session-binding: session-binding model that future provider adapters must build on
  260605-research-ws-native-subagent-pivot: supersedes dashboard deprecation and preserves the web dashboard while moving agent visibility away from removed agents.* surfaces
  260523-feat-ws-dashboard-main-session-activity-source: prior main-session freshness gap that must be re-grounded on host-owned agent/client activity
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: serverId forwarding must carry Activity source identity before remote provider streams are transparent
  260711-feat-ws-dashboard-agent-activity-chat-ui: interactive chat UI/UX split out of this ticket's Phase 5 to keep provider-adapter scope and UI/UX scope separately reviewable
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-dashboard-agent-harness
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
sage-review: completed
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
- **Dashboard is the spawn authority for all three harnesses; ws session-key
  injection happens at spawn time** (owner, 2026-07-11): consistent with how
  the dashboard already manages terminal PTY child processes, the dashboard
  daemon is the process owner for Codex app-server, OpenCode ACP, and the
  Claude CLI alike — it spawns each harness subprocess itself (Codex via
  `codex app-server --stdio` or the verified equivalent invocation, OpenCode
  via `opencode acp`, Claude via the headless stream-json CLI invocation),
  not merely attaching to an externally-started process. Per the epic's
  ferrule-backed session-binding model, the daemon calls `ws.ferrule(root)`
  in its local ws environment before spawning, then injects the returned key
  into the child process's launch context (CLI arg or environment variable,
  whichever each harness's own invocation supports) and keeps it only in
  daemon-private binding state alongside the provider-native session id and
  the browser-facing `activityId` — never as three interchangeable ids. This
  applies uniformly to Phase 2/3/4; per-harness setup a user must do outside
  the dashboard (installing the CLI, logging in / authenticating that CLI)
  is a precondition for spawn succeeding, not a reason the dashboard itself
  doesn't own the spawn.
- **ws/wsflow plugin presence is a hard spawn precondition, enforced by the
  dashboard, not worked around** (owner, 2026-07-11): a dashboard-spawned
  Codex/OpenCode/Claude session is expected to have the `ws` or `wsflow`
  plugin (see `plugin-runtime` mental model — same underlying
  `agents-plugin-tool` MCP surface, `ws.*` vs `wsflow.*` namespace, same
  ferrule/session-key storage so either satisfies this check) installed and
  enabled for that harness in that project. Before spawning, the daemon
  checks install status the same way the Claude skill-listing spike did
  (`claude plugin list`-equivalent, no session needed); if neither plugin is
  present, the dashboard **refuses to spawn** and surfaces install guidance
  to the human instead of silently degrading or auto-injecting an external
  MCP config as a substitute. Rejected alternative: having the dashboard
  itself inject an external MCP-server registration + skill list into the
  harness's launch context when the plugin is missing — owner explicitly
  chose the harder install-enforcement floor over this softer fallback, to
  avoid two divergent code paths (native plugin vs. dashboard-injected) for
  the same capability. **Unverified**: whether Codex and OpenCode expose an
  equivalent no-session plugin-list CLI surface analogous to `claude plugin
  list`/`claude plugin details` needs its own fixture check per harness
  before this precondition can be implemented for them; do not assume
  parity with Claude's confirmed surface. **Future direction, not chosen
  now** (owner, 2026-07-11): a later alternative to per-harness plugin-list
  checking is the dashboard hosting its own MCP surface (per
  `260711-idea-dashboard-agent-facing-mcp-control-surface`) that harnesses
  connect to directly, letting the dashboard forward/gate ws-mcp commands
  itself rather than depending on each harness's native plugin-install
  detection. Not designed further here — recorded so this precondition
  check isn't over-built as a permanent mechanism if that direction lands
  first.
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
  - **Rewind/fork-from-a-point — superseded by the Codex fixture spike below**:
    `thread/fork` is confirmed real (by threadId or path). `thread/rollback`
    is confirmed real but **deprecated for removal** ("DEPRECATED:
    `thread/rollback` will be removed soon", per the actual shipped schema),
    and its granularity is coarser than assumed: params are
    `{threadId, numTurns}` — it drops N turns from the *end* of the thread,
    not rewind-to-an-arbitrary-point, and it explicitly does not revert file
    changes the agent made (caller's responsibility). Do not build new
    dashboard functionality on `thread/rollback`; treat it as
    Passthrough-but-sunsetting, not a stable primitive to design around.
    `thread/fork` and `thread/resume` (3 modes: by threadId, in-memory
    history, or by path) remain the sound native primitives here. ACP (OpenCode) has `session/load` (resume with
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
  - **Context-window/token introspection and compaction control — Codex
    findings confirmed by fixture spike**: ACP has a
    standardized, *recently finalized* (2026-06-05) `session/update`
    notification variant (`sessionUpdate: "usage_update"`, fields `used`/
    `size`/optional `cost`) — a real, versioned protocol feature, so this is
    the strongest candidate for a common-subset field, but only if the other
    two harnesses expose an equivalent. Codex app-server has its own
    **dedicated** `thread/tokenUsage/updated` notification
    (`{threadId, turnId, tokenUsage}`), separate from and more granular than
    `turn/completed` — the ticket's original "token usage on `turn/completed`"
    claim undersold this; there is a purpose-built usage notification.
    `thread/compact/start` is confirmed real: params `{threadId}` only,
    response is an empty `{}` acknowledgement, with the actual compaction
    result arriving asynchronously via a `thread/compacted` notification (the
    older `ContextCompactedNotification` is explicitly deprecated in favor of
    a `ContextCompaction` item type) — good support, but the dashboard must
    consume `thread/compacted`, not assume the RPC response itself carries
    the result. Claude CLI's
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
  - **Skill/capability listing — confirmed by fixture spike**: Codex
    app-server's `skills/list` is real and current (not deprecated): params
    `{cwds?, forceReload?}`, response `{data: SkillsListEntry[]}`. OpenCode/ACP has no
    documented skills-list method in the core protocol so far (`Agents` docs
    describe configuring agents/modes, not a runtime capability-listing
    call) — unverified. Claude CLI's `system` init event reports `tools`
    and `capabilities` at session start (from this ticket's existing Phase-4
    grounding) but that is session-start metadata, not an on-demand listing
    RPC, and does not obviously cover project-local `SKILL.md`-style skills.
    **Verdict**: not safe as a uniform common-subset call yet; Codex has the
    clearest native support, the other two are unverified-or-absent —
    revisit after a fixture pass rather than assuming parity.
    - **Claude column split into two mechanisms, fixture-verified directly
      against the installed `claude` CLI 2.1.207 (2026-07-11, owner-prompted
      follow-up)**: there is no dedicated `claude skills list`/`claude
      --skills` command, but a genuine no-session CLI-native surface does
      exist for **plugin-provided** skills: `claude plugin list` (resolved,
      per-scope user/project, enabled/disabled state, no session needed) and
      `claude plugin details <plugin>` (per-plugin "Component inventory"
      including exact skill names, e.g. `document-skills` → `docx, pdf,
      pptx, xlsx`) — confirmed real by running both commands directly.
      Enable/disable state for these is authoritative in
      `settings.json`'s `enabledPlugins` map. **Gap**: standalone/loose
      skills dropped directly under `~/.claude/skills/<name>/` or a
      project's `.claude/skills/<name>/` (not packaged as a plugin) do
      **not** appear in `plugin list`/`plugin details` on this installed
      version, despite `plugin init --help` text mentioning
      "skills-dir plugins" — confirmed by testing against a real loose skill
      directory present in this environment (`~/.claude/skills/typst/`,
      which the running session lists as available but which `plugin
      details typst`/`typst@skills-dir` both reported "not found"). Getting
      a fully resolved list therefore requires **two mechanisms combined**:
      Passthrough `plugin list`/`plugin details` for plugin-provided skills,
      plus a dashboard-owned filesystem scan of `~/.claude/skills/**/
      SKILL.md` and project-local `.claude/skills/**/SKILL.md`
      frontmatter (name/description) for loose/project skills, gated on
      whether the spawn used `--safe-mode`/`--disable-slash-commands` (both
      disable all skills globally). **Verdict**: reclassify from a flat
      Unavailable — plugin-skill listing is Passthrough (real CLI surface,
      confirmed), loose/project-skill listing stays Overlay (dashboard
      filesystem scan), and the union of both is what backs
      `activity.session.skills` for the Claude column.
  - **"Goal"/loop native support — revised, Codex column wrong before the
    fixture spike**: the original WebSearch-only pass missed Codex's native
    `thread/goal/set` / `thread/goal/get` / `thread/goal/clear` RPC family
    entirely (confirmed real by the fixture spike: `ThreadGoalSetParams
    {threadId, objective?, status?, tokenBudget?}`, with `thread/goal/updated`
    / `thread/goal/cleared` notifications). This is a genuine
    client-settable, structured goal-tracking primitive — an objective
    string plus status plus a token budget the server tracks — not nothing,
    as the ticket previously claimed for every harness including Codex.
    It is **not** confirmed to be a full repeat-until-condition auto-loop:
    no evidence the server itself re-sends turns to satisfy the objective,
    only that it tracks/reports goal state alongside turns the client still
    drives. ACP's closest concept remains `Plan`/`PlanEntry` (agent-reported
    plan, not client-commanded) plus `CurrentModeUpdate`. No evidence of a
    Claude-CLI-native equivalent. **Revised verdict**: Codex has a real
    Passthrough goal-*tracking* primitive (objective/status/budget bookkeeping)
    that the dashboard should surface and use where available, but the
    repeat-until-condition *looping* behavior itself still has no confirmed
    harness-native driver on any of the three — that part should stay
    dashboard-built (re-send prompts/turns based on a dashboard-evaluated
    condition), optionally informed by Codex's native goal state when
    present rather than duplicating it blindly.
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
    fixture check of OpenCode's child-session behavior. The looping half of
    goal/loop functionality should stay dashboard-built rather than
    harness-native, though Codex's native `thread/goal/*` tracking primitive
    (confirmed by the fixture spike) should be surfaced/used where available
    rather than duplicated. Codex's `turn/steer` (inject input into an
    *active* turn, gated on an `expectedTurnId` precondition) was not in the
    original draft at all and should be considered as a Codex-native
    addition to the interaction-API list below, gated the same way as
    compact/rewind/skills. None of this pass's non-Codex findings were
    fixture-verified against installed binaries; treat the OpenCode/Claude
    per-harness verdicts above as directional, not final, until their own
    spikes run (Claude's is planned for Phase 4; OpenCode's is blocked
    pending install). The **Codex column above is now fixture-verified**,
    not WebSearch-only — see the spike results folded in above and in the
    matrix below.
- Treat OpenCode serve as an optional observation/read-only supplement. It can
  help discover sessions or stream HTTP/OpenAPI/SSE state, but the first
  OpenCode counterpart to Codex app-server is `opencode acp`, not `opencode
  serve`.
- Keep browser identity dashboard-native: `serverId`, `workspaceId`,
  `workRootId`, `activityId`, and transcript cursors. Do not expose provider
  session ids, ws `session_key` values, cache paths, transcript paths, process
  ids, or raw provider event ids as browser authority.
- **Scope supersedes prior read-only framing: full-spec interactive agent
  harness interface (owner, 2026-07-11, resolves the tension flagged
  below)**: this ticket's scope now explicitly includes interactive session
  control — start, create, resume, send/receive, and the per-harness-gated
  compact/rewind/fork/skills controls from Phase 1's frontend
  interaction-API draft — not just read-only Activity projection. The
  browser is meant to become a genuine full-spec agent-harness client
  surface across Codex app-server, OpenCode ACP, and the Claude CLI, not a
  read-only observation pane over harness-owned state. The prior "Keep
  browser Activity read-only for this track" decision and the matching
  Constraints language are superseded by this decision, not merely
  clarified. What still stays out of scope (per the epic's Non-Scope and
  this ticket's own Decisions above): the dashboard does not become ws MCP
  root/session authority, does not run its own model loop or edit/permission
  engine, and does not attempt Hack-tier capabilities inside these normal
  phases (those still need a separate, explicitly-labeled ticket per the
  Cross-Harness Feature Matrix's Phase-implication guidance below).
  - **Resolved tension (2026-07-11)**: Phase 1's "frontend interaction-API
    draft" — `activity.session.create`, `activity.session.start`,
    `activity.session.send`, and the per-harness-gated compact/rewind/skills
    controls — was flagged as directly contradicting the read-only decision
    and Constraints text as of the same date. The owner has now confirmed
    (a): this ticket's scope explicitly includes interactive session
    control, superseding the read-only decision rather than splitting it
    into a separate ticket.
- **UI/UX design and implementation split out to its own ticket** (owner,
  2026-07-11): the interactive Activity chat surface's layout and
  interaction design (tab entry points, conversation view, resume/fork
  affordances, mid-turn submission queuing) is specified in
  `260711-feat-ws-dashboard-agent-activity-chat-ui`, not here — split out
  once the UI/UX detail grew large enough to crowd this ticket's
  provider-adapter scope. This ticket stays scoped to the
  `AgentClientProvider` contract, per-harness adapters, and the Activity
  interaction-API methods that UI dispatches through. That ticket also
  raises an open, not-yet-decided question relevant to this ticket's Phase 1
  `activity.session.skills` method: whether the dashboard should maintain
  its own skill/capability layer instead of relaying each harness's native
  (and observed-unreliable) skill listing as-is — resolve before
  implementing that method as a thin passthrough.

## Cross-Harness Feature Matrix (owner + research, 2026-07-11)

Tier is decided **per (harness, capability) cell**, never inherited from
another harness's classification — a capability being Codex-only-native does
not demote it to Hack; it is simply Unavailable elsewhere.

- **Passthrough**: the dashboard calls a capability the harness itself
  documents/exposes officially for third-party programmatic use.
- **Overlay**: the dashboard composes officially-exposed primitives (e.g.
  repeated `send` calls) into behavior the harness doesn't natively provide;
  no vendor-private state is touched.
- **Hack**: the only reachable path mutates a harness's private/undocumented
  on-disk state, or relies on reverse-engineered/unofficial protocol messages
  not documented for third-party use.
- **Unavailable**: no known path (official or hack) as of this research pass.
  Treated as not-implemented, not silently downgraded to Hack — a workaround
  is only classified Hack once someone actually attempts and ships one.
- *Italic* = unverified this pass (WebSearch/WebFetch research only, not
  fixture-checked against an installed binary).

Session discovery/history-listing is intentionally excluded from this table:
it is already handled uniformly for all three via vendor-history-file
scraping (see `260624-feat-ws-dashboard-managed-cli-recent-sessions`), a
read-only observation mechanism orthogonal to this control-risk tiering.

| Capability | Codex app-server | OpenCode ACP | Claude CLI |
|---|---|---|---|
| Resume existing session | Passthrough (`thread/resume`, 3 modes: threadId/in-memory/path — **fixture-verified**) | Passthrough | Passthrough (`--resume`) |
| Create new session | Passthrough (`thread/start` — **fixture-verified**) | Passthrough | Passthrough |
| Send/receive message (turn) | Passthrough (`turn/start`; `turn/steer` for mid-active-turn injection, gated on `expectedTurnId`; `turn/interrupt` — **fixture-verified**, `turn/steer` was not in the original draft) | Passthrough | Passthrough (documented event subset only — `system`/`assistant`/`stream_event`/`result`; the unverified `control`/`control_request` shape stays out of scope until fixture-confirmed) |
| Permission/approval interception | Passthrough (`item/commandExecution/requestApproval`, `item/fileChange/requestApproval`, `item/permissions/requestApproval`, `item/tool/requestUserInput`, plus legacy `execCommandApproval`/`applyPatchApproval` — **fixture-verified**) | *Unverified* | Passthrough (`PreToolUse` hooks) |
| Context usage display (read-only) | Passthrough — dedicated `thread/tokenUsage/updated` notification (`{threadId, turnId, tokenUsage}`), separate from and more granular than `turn/completed` — **fixture-verified** | Passthrough (`usage_update`) | Passthrough (`result` token counts) |
| Manual compaction trigger | Passthrough (`thread/compact/start`, params `{threadId}`, empty `{}` response — result arrives async via `thread/compacted`; the older `ContextCompactedNotification` is deprecated — **fixture-verified**) | *Unverified* | Unavailable (auto-only; would become Hack if a workaround were attempted; headless `/compact`-as-text research below stays inconclusive) |
| Rewind/rollback (turn-count-from-end, not point-based) | Passthrough **but deprecated for removal** (`thread/rollback`, params `{threadId, numTurns}`, drops N turns from the end, does not revert file changes — **fixture-verified**; do not design new functionality around this RPC) | *Unverified*, likely Unavailable | Hack (transcript-file truncation workaround; no officially-documented method) |
| Fork new branch from a point | Passthrough (`thread/fork`, by threadId or path — **fixture-verified**) | *Unverified*, likely Unavailable | Hack (same workaround as rewind) |
| Skill/capability listing | Passthrough (`skills/list`, params `{cwds?, forceReload?}`, response `{data: SkillsListEntry[]}`, current/not deprecated — **fixture-verified**) | *Unverified* | **Split, fixture-verified**: Passthrough for plugin-provided skills (`claude plugin list` + `claude plugin details <plugin>`, no session needed); Overlay for loose/project `SKILL.md` skills not packaged as a plugin (dashboard filesystem scan — no CLI surface lists these) |
| Subagent list + per-subagent transcript | Unavailable (fixed "Guardian" role only, not a general registry) | Overlay candidate (if child sessions are ordinary ACP sessions, list/relate them via existing session primitives — pending fixture check) | Unavailable (no documented CLI-level introspection surface) |
| Goal state tracking (objective/status/token budget) | Passthrough (`thread/goal/set`/`get`/`clear`, `{threadId, objective?, status?, tokenBudget?}`, with `thread/goal/updated`/`thread/goal/cleared` notifications — **fixture-verified**; missed entirely by the original WebSearch-only pass) | Unavailable (no confirmed equivalent) | Unavailable (no confirmed equivalent) |
| Goal/loop (repeat-until-condition auto-looping) | Overlay (no evidence the server itself auto-loops turns to satisfy a goal objective — dashboard must still drive re-sends, optionally informed by native goal state above) | Overlay (dashboard-built on top of `send`) | Overlay (same) |

**Claude `/compact`-in-headless research (2026-07-11)**: checked whether
sending literal `/compact` as headless/`stream-json` input text reproduces
interactive-mode compaction. No direct confirmation either way exists in
Anthropic's docs or public issues. Official headless docs
(`code.claude.com/docs/en/headless`) enumerate every supported built-in slash
command for `-p` mode (`/model`, `/effort`, `/fast`, `/color`, `/rename`,
`/mcp`, `/config`) and explicitly note terminal-only commands like `/login`
are unavailable there — `/compact` appears in neither list, which is
suggestive (not conclusive) that it falls through as inert literal text
rather than triggering compaction. The cell stays **Unavailable**, not
reclassified, until a real fixture spike (send `/compact` via stream-json to
an installed `claude` binary and check for a compaction/system event)
settles it.

**Standing classification rule**: a capability with no officially-documented
protocol/CLI method for a given harness defaults to Unavailable, not
attempted. If a workaround is later implemented anyway, it must be
classified Hack for that harness regardless of how many other harnesses
support the capability natively. Because Codex app-server was purpose-built
as a third-party integration surface and OpenCode ACP is a purpose-built
standard, while Claude's headless stream-json surface is documented but not
purpose-fit for this exact use case, new capabilities should default to
stricter scrutiny on the Claude column specifically.

**Phase implication** (not yet restructured into the Phases section below):
Passthrough cells belong in each harness's existing Phase 2/3/4 adapter work.
Overlay cells should land in a later, dedicated phase once passthrough is
stable, since they need dashboard-side state/condition design but carry no
vendor-side risk. Hack cells should not ship inside this ticket's normal
phases at all — they need a separate idea ticket with explicit
experimental/unsupported UI labeling and owner risk sign-off, mirroring the
"dangerously bypass" opt-in pattern already used for execution approval.
Unavailable cells are simply not implemented unless a future vendor update
adds an official method, which would re-trigger this matrix's classification
rather than being assumed.

## Prior Art

- `ActivityFeed.items`, `ActivityTranscript`, and `TranscriptBlock` are already
  source-neutral enough to carry native agent-client activity.
- `ActivityFeed.agents` is a compatibility projection for the existing named-
  agent pane; new provider activity should flow through `items`.
- The current event vocabulary already supports item upserts/removals,
  transcript updates, snapshot invalidation, mode changes, and heartbeats.
- The dashboard daemon is explicitly not ws MCP root, harness, or
  model-backend authority. **Note (2026-07-11): "agent-session authority"
  here means ws MCP session-key/root authority specifically, not a
  read-only restriction on harness-native session control** — the scope
  supersession in Decisions above has the dashboard driving harness-native
  session create/resume/send/compact/fork/goal primitives directly. This
  line predates that decision and is retained only for the narrower ws-MCP-
  authority point; do not read it as re-imposing the old read-only framing.
  The dashboard consumes provider state through daemon-owned view models
  and provider adapters, and drives provider-native session control through
  the same adapters — never through ws MCP session-key minting.

## Constraints

- Provider adapters must degrade unknown or malformed events into bounded status
  or diagnostic transcript blocks instead of leaking raw JSON, paths, session ids,
  or command output payloads outside the selected transcript surface.
- Adapter contracts must be fixture-backed by captured provider events or schema
  snapshots because Codex app-server, OpenCode ACP, and OpenCode serve surfaces
  can drift by installed version.
- **Superseded 2026-07-11**: the subset is no longer read/list/stream/render
  only. Interactive session control (start/create/resume/send, plus the
  per-harness-gated compact/rewind/fork/skills controls) is now in scope per
  the Decisions supersession above. What must still be avoided is *harness
  development* itself — a new model loop, edit engine, or permission runtime
  owned by the dashboard — not interactive control of the existing harnesses'
  own official session/turn primitives. Hack-tier capabilities (per the
  Cross-Harness Feature Matrix) still stay out of these normal phases and
  need a separate, explicitly-labeled ticket with owner risk sign-off.
- Server-scoped dashboard operation is not special design work for this
  ticket — it is the same existing Server Route pattern every other
  dashboard resource (terminal, git worktree, file routes) already follows
  (`ws-web-dashboard` Domain Rules: every non-local-gateway API lives under
  `/api/dashboard/servers/{serverRoute}/...`). The reminder here exists only
  because a brand-new subsystem is an easy place to accidentally regress
  that pattern (e.g. keying a new provider-process registry by `workRootId`
  alone and forgetting `serverId`) — not because remote agent-harness
  sessions need a new or different design. Activity source ids, stream
  subscriptions, transcript routes, and persisted UI state must include or
  derive `serverId` the same way existing resources already do.

## Public Interface Briefing

This track intentionally touches the dashboard's browser-facing Activity
interface. The existing read routes remain stable entrypoints:

- `GET /api/dashboard/work-roots/{workRootId}/activity`
- `GET /api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript`
- `GET /api/dashboard/work-roots/{workRootId}/activity/events`

**Superseded 2026-07-11**: since interactive session control is now in scope
(see Decisions supersession above), Phase 1 also needs new write routes
backing `activity.session.create/start/send` and the per-harness-gated
compact/rewind/fork/skills methods from the frontend interaction-API draft
below. These are new authenticated routes under the same
`/api/dashboard/servers/{serverRoute}/...`-scoped, `workRootId`/`activityId`
identity model as the existing read routes — not a parallel identity or
routing scheme.

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
file-change, approval-needed, status, diagnostic, and **thinking** blocks.
Unknown provider records degrade to bounded diagnostic/status blocks rather
than leaking raw provider JSON.

**`thinking` block kind added 2026-07-11** (fixture-review follow-up): both
Claude's `assistant` stream event (per Decisions above) and Codex's item
stream (reasoning/thinking content, distinct from `assistant` text) carry
extractable thinking/reasoning content where the harness exposes it. This is
its own `renderKind: "thinking"` `TranscriptBlock`, not a sub-field buried
inside an `assistant` block — `260711-feat-ws-dashboard-agent-activity-chat-ui`'s
collapsible-thinking-block UI (default collapsed, interleaved between
surrounding agent content) renders directly against this kind. A harness
that exposes no such content simply never emits this kind; the UI has
nothing to collapse for it.

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
    Codex-native (`thread/compact/start`, params `{threadId}`, result via
    async `thread/compacted` notification — **fixture-verified**); no
    confirmed equivalent for OpenCode or Claude (auto-only).
  - `activity.session.steer(activityId, message)` — inject input into an
    *active* turn (Codex-native `turn/steer`, gated on `expectedTurnId`
    matching the currently active turn — **fixture-verified**, not in the
    original draft). No confirmed equivalent for OpenCode or Claude; degrade
    to hidden/disabled for those.
  - `activity.session.goal.set/get/clear(activityId, {objective?, status?,
    tokenBudget?})` — Codex-native structured goal-state tracking
    (`thread/goal/set`/`get`/`clear`, notifications
    `thread/goal/updated`/`thread/goal/cleared` — **fixture-verified**, not
    in the original draft; missed by the initial WebSearch-only pass). This
    is bookkeeping the server tracks, not an auto-looping primitive — see
    the dashboard-built goal/loop item below for the actual repeat-until-
    condition behavior. No confirmed equivalent for OpenCode or Claude.
  - `activity.session.rewind(activityId, atPoint)` /
    `activity.session.fork(activityId, atPoint)` — Codex's `thread/fork` is
    solid (by threadId or path — **fixture-verified**), but `thread/rollback`
    is **confirmed deprecated for removal** and coarser than assumed (params
    `{threadId, numTurns}`, drops N turns from the end, not an arbitrary
    point, and does not revert file changes — **fixture-verified**); do not
    design new dashboard functionality around `thread/rollback` specifically,
    even though it is technically callable today. Claude has no native
    method and would need a dashboard-owned workaround (copy + truncate the
    `~/.claude/projects/.../*.jsonl` transcript, then `--resume` against the
    copy — unverified whether the CLI accepts a hand-truncated transcript);
    OpenCode unverified in core ACP.
  - `activity.session.skills(activityId)` — list invokable
    skills/commands/plugins. Codex-native (`skills/list`, params
    `{cwds?, forceReload?}`, response `{data: SkillsListEntry[]}` —
    **fixture-verified**, current/not deprecated); OpenCode has no confirmed
    on-demand listing call. Claude's adapter must union two sources
    (**fixture-verified against the installed `claude` CLI 2.1.207**): run
    `claude plugin list` then `claude plugin details <plugin>` for each
    enabled plugin (Passthrough, no session needed, returns real skill
    names per plugin) plus a filesystem scan of `~/.claude/skills/**/
    SKILL.md` and project-local `.claude/skills/**/SKILL.md` frontmatter for
    loose/project skills not packaged as a plugin (Overlay — confirmed no
    CLI surface lists these; `plugin details <loose-skill-name>` reports
    "not found" even for a skill the running session actually resolves).
- **Deliberately not modeled as a bespoke API**: subagent
  listing-plus-per-subagent-transcript-streaming. No harness has clean
  native support for this shape. Pending an OpenCode fixture check of its
  child-session-as-ordinary-session behavior, the dashboard should consider
  modeling a subagent as an ordinary nested Activity/session row under its
  parent `activityId` (reusing `activity.session.start`-shaped access to
  that child row for the "click bubble to open its transcript" UX) instead
  of inventing a dedicated subagent RPC surface.
- **Deliberately dashboard-built, not harness-native**: `/goal`-style
  repeat-until-condition looping. No harness exposes a client-driven
  auto-loop primitive — Codex's `thread/goal/set` family (see above) tracks
  goal *state* (objective/status/token budget) but does not itself re-send
  turns to satisfy the objective, and ACP's `Plan`/`PlanEntry` is the agent
  reporting its own plan, not the client commanding repetition — if built,
  the looping behavior itself should be
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

### Phase 2: Codex app-server read/write adapter

The dashboard daemon spawns Codex app-server as a child process per the
spawn-authority decision above, speaks its JSON-RPC stdio protocol, maps
Codex lifecycle concepts into the ACP-shaped provider subset, and projects
native Codex activity into `ActivityItem` rows and `TranscriptBlock` backfill.
Now that interactive control is in scope (see Decisions supersession above),
this phase covers both the read/project path and the Passthrough write
capabilities from the Cross-Harness Feature Matrix that are Codex-native:
`thread/start`/`thread/resume`/`thread/fork`, `turn/start`/`turn/steer`/
`turn/interrupt`, `thread/compact/start`, `skills/list`, and the native
`thread/goal/set`/`get`/`clear` family — see the fixture spike results below
and in Decisions/the Feature Matrix above. `thread/rollback` exists but is
deprecated for removal and should not be designed around. Unknown event
types must degrade without breaking the whole Activity feed.

Before spawning, this adapter must also enforce the ws/wsflow
plugin-presence precondition from the Decisions section above: check
whether Codex exposes a no-session plugin-listing CLI surface analogous to
`claude plugin list`/`claude plugin details` (unverified as of 2026-07-11 —
do not assume it exists; this is this phase's own fixture check to run, not
an already-answered question), and refuse to spawn with install guidance if
neither `ws` nor `wsflow` is installed/enabled for the target project. If no
such CLI surface exists for Codex, fall back to whatever detection method
the fixture check turns up (e.g. reading Codex's own config/MCP-registration
file) rather than skipping the precondition.

**Fixture-verification spike: completed (2026-07-11)**. Exact invocation:
`codex app-server [--listen stdio://|unix://[PATH]|ws://IP:PORT|off]`,
default `stdio://`; `--stdio` is shorthand for `--listen stdio://`. Rather
than capture a live handshake, the spike used
`codex app-server generate-json-schema --out <dir> --experimental`, which
dumps the literal JSON Schema bundle shipped with the installed
`codex-cli 0.144.1` — a stronger source of truth than one sampled
interaction. This confirmed real request/response field names for every
Codex RPC referenced in this ticket (folded into Decisions and the Feature
Matrix above) and surfaced two capabilities the original WebSearch-only pass
missed entirely: `turn/steer` and the `thread/goal/*` family. It also
corrected the `thread/rollback` assumption (deprecated, turn-count-based, not
point-based, does not revert file changes). No live thread/turn was
exercised and no app-server process was left running; a live-session
projection-fixture pass (capturing real `item/*`/`turn/*` event sequences
from an actual turn) is still needed before the projection tests below are
written, since schema shape alone does not capture real event *sequencing*.

Verification boundary: fixture projection tests for representative Codex
thread/turn/item sequences (captured from the spike above), route tests
proving browser payloads omit provider session ids and raw paths, a test
proving spawn is refused with install guidance when the ws/wsflow
plugin-presence check fails, and a local smoke path that can be run when
Codex app-server is available. The WSL smoke should run against the locally
installed `codex app-server --stdio` binary and prove that a real turn —
including a Codex-native compact/rewind/fork/skills call — can appear in and
be driven from the dashboard Activity Console without requiring dashboard
owner pairing when the daemon is explicitly started through the
loopback-only no-auth debug profile.

### Phase 3: OpenCode ACP provider adapter

**Blocked pending install (owner, 2026-07-11)**: OpenCode is not installed in
this environment, so its fixture-verification spike cannot run yet. This
phase's design intent below stands, but implementation should wait until
OpenCode is installed and a spike (mirroring Phase 2/4) can capture real ACP
event shapes; do not implement against WebSearch-only assumptions.

The dashboard daemon spawns `opencode acp` as a child process per the
spawn-authority decision above, speaks the ACP stdio JSON-RPC flow, maps
OpenCode sessions/messages/tool activity and permission states into the same
ACP-shaped provider subset, and projects the result into the same Activity
model as Codex app-server. Keep this adapter independent of Codex-specific
assumptions; the common contract is the dashboard provider subset and
Activity projection, not either provider's wire protocol. OpenCode serve may
be used only as an optional observation/discovery supplement if a concrete
gap appears that ACP does not cover cheaply.

This adapter must also enforce the ws/wsflow plugin-presence spawn
precondition from the Decisions section above: check whether OpenCode
exposes a no-session plugin-listing CLI surface (unverified as of
2026-07-11 — this is part of this phase's own future fixture spike, not an
already-answered question), and refuse to spawn with install guidance if
neither `ws` nor `wsflow` is installed/enabled for the target project.

Verification boundary: fixture projection tests for OpenCode ACP messages/events,
bounded degradation tests for missing binary/auth, subprocess startup failure,
unreachable or incompatible ACP server state, and version drift, a test
proving spawn is refused with install guidance when the ws/wsflow
plugin-presence check fails, plus route tests matching the same privacy and
identity constraints as the Codex adapter.

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

This adapter must also enforce the ws/wsflow plugin-presence spawn
precondition from the Decisions section above: `claude plugin list` /
`claude plugin details <plugin>` are already confirmed as a no-session CLI
surface for this (see the skill-listing fixture spike), so this phase can
implement the check directly rather than treating it as unverified; refuse
to spawn with install guidance if neither `ws` nor `wsflow` is
installed/enabled for the target project.

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
version state, a test proving spawn is refused with install guidance when
`claude plugin list` shows neither `ws` nor `wsflow` installed/enabled, and
route tests matching the same privacy and identity constraints as the Codex
and OpenCode adapters.

### Phase 5: Activity UI and server-scoped integration

Lift the visible Activity UI from named-agent wording to source-neutral
agent-client activity. Thread `serverId` through Activity source selection
and stream keys before linked remote providers are considered transparent.

The detailed interactive chat UI/UX design and implementation (tab entry
points, conversation view, resume/fork affordances, mid-turn submission
queuing) is specified in its own ticket,
`260711-feat-ws-dashboard-agent-activity-chat-ui` — split out once that
detail grew large enough to crowd this ticket's provider-adapter scope. This
phase covers only the source-neutral labeling/identity-key groundwork that
ticket's UI builds on top of.

Verification boundary: frontend route/model tests for source-neutral labels
and identity keys, browser-level acceptance evidence for mixed source rows
and transcript rendering, and server-scoped route tests showing local
compatibility aliases still map to `server-local`.
