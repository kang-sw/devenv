---
title: "Dashboard agent host: daemon-spawned headless lead sessions with approval, mail injection, recovery, and idle stop"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-session-holder
related:
  261110-feat-ws-hosted-session-mode-flag: prerequisite
  261110-research-ws-dashboard-agent-host-foundations: "remaining verifications: -p feature coverage, Pi parity, ws hooks under -p"
  261110-research-ws-dashboard-background-task-visibility: "informational: Decision 6 already settles idle stop from the 2026-11-11 probe; its open items do not gate Phase 2"
  261108-feat-ws-tracker-support-agent-chat: "the wsagent runner this generalizes"
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope 241 b (no browser approval) is reopened under Decision 3"
  261110-feat-ws-dashboard-agent-panel-telemetry: "agents panel telemetry this ticket supplies from the stream for daemon sessions"
  261110-feat-ws-dashboard-workspaces: "its Phase 2 (conversation handoff) waits on this ticket's Phase 1; this ticket builds the new-session + buttons and popover ((189))"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5d3c13bb2ddd0a2c
sage-review-completeness-reviewed: 5d3c13bb2ddd0a2c
---

# Daemon-spawned headless lead sessions

## Background

Track A of the epic: the dashboard daemon starts and manages lead sessions
itself instead of only attaching to sessions a `claude` or `pi` CLI process
created. `agents-plugin-tool/internal/wsagent` already runs headless
conversations for the tracker support agent: `claude -p --input-format
stream-json --output-format stream-json` with stdin held open, Pi through its
RPC mode, with create, send (held to turn end and merged), interrupt,
set_model, resume through the backend session ID, and an idle stop
(`wsagent/claude.go`, `wsagent/pi.go`, `wsagent/runner.go`). Its support
profile isolates the agent; a lead needs the user's settings, the ws plugin,
and its hooks.

## Decisions

1. **Headless, not a TUI in a PTY (user, (12) b).** Daemon leads run as
   `claude -p` stream-json and Pi RPC processes owned by a session holder
   (`261110-feat-ws-dashboard-session-holder`). The user's reason:
   headless gives far better control.
   Rejected: running the interactive TUI inside a holder PTY ((12) a), and
   offering both per spawn ((12) c).
2. **The daemon injects all input (technical, (16)).** Dashboard messages and
   ws mailbox mail for a daemon session reach it as stream-json user turns
   written by the daemon, reusing the runner's send (held to turn end,
   merged) and interrupt paths. The session runs with the hosted-session
   flag (`261110-feat-ws-hosted-session-mode-flag`, landed 336dde7b3) so the
   mod or extension does not also deliver them. The file-inbox wait stays
   for CLI sessions only. A message the user steers is written at once
   rather than held (technical, (211)): Claude queues a mid-turn user
   message (`command_lifecycle` `queued`) and injects it right after the
   next tool result in the same turn (`started`), one `result` then closing
   the turn (lead probe 2026-11-11, CLI 2.1.322); Pi takes the RPC `steer`
   command (`rpc-types.d.ts`). Unsteered sends stay held to turn end.
3. **Approval through the stdio prompt tool, answered only from loopback
   (announced, (13), (169), (171); trust boundary (05)).** The lead profile
   passes `--permission-prompt-tool stdio`. Each prompt arrives as a
   `control_request` `can_use_tool` (tool, input, description,
   `permission_suggestions`, `tool_use_id`) and the daemon answers it on
   stdin with a `control_response`: `allow` with `updatedInput`, or `deny`
   with `message`. A session is awaiting approval exactly while a
   `can_use_tool` request is pending; the `requires_action` session state
   (Decision 7) cross-checks it. A request for a tool that needs the user's
   answers rather than an approval (`AskUserQuestion`, `ExitPlanMode`, or
   one flagged `requiresUserInteraction`) is denied with a message telling
   the model to ask in chat and end its turn (user, (199) b), since a plain
   allow without `updatedInput.answers` breaks the tool (Orca ADE,
   `claude-structured-prompt-replies.ts`); the `ExitPlanMode` denial tells
   the model to present the plan in chat, and the way out of `plan` mode is
   the header menu's in-stream mode change (Decision 11). A
   `requires_action` with no `can_use_tool` pending (an inherited prompt,
   or another `control_request` subtype such as an MCP elicitation) never
   stalls silently: the session shows a visible state and the event is
   logged, and an unhandled `control_request` subtype gets an error reply;
   the exact form, whether the spawn form offers `plan`, and where
   `requiresUserInteraction` sits in the payload are chosen in planning.
   Answers are accepted only from loopback (`localViewer`
   plus `sameOrigin`), the boundary the dashboard terminal landed with
   (`agents-plugin-tool/internal/wsdashboard/terminal_routes.go#L44-L49`,
   `261110-feat-ws-dashboard-terminal` Decision 5); a desktop-shell origin
   is recognized only once a shell exists (epic (19)). This reopens
   `261106-epic-ws-dashboard-serve-daemon` Non-Scope 241 b under that
   condition.
   Rejected: `--permission-prompts host` alone, which the 2026-11-11 probe
   showed auto-denies every prompt under stream-json
   (`261110-research-ws-dashboard-agent-host-foundations` Verified
   Findings); rendering the question tool's options on the card in this
   ticket ((199) a), deferred.
4. **Permission mode chosen at spawn, default `auto`; unanswered prompts
   wait (user, (14) a); the wait notifies inside the dashboard (user, (179)
   a).** A prompt waits for an answer with no timeout. The session shows an
   awaiting-approval state on its badge in the rail and the agents panel,
   and a browser notification goes out while a dashboard tab is open. A LAN
   tab gets the notification too, with a line saying to answer on the
   dashboard PC.
   Rejected: auto-deny after a timeout ((14) b), because in a very long
   session denials pile up while the user is away and steer the work wrong;
   no browser approval at all, `--permission-prompts none` with spawn-time
   mode and allowlist only ((14) c); an OS notification from the daemon
   ((179) b), a new external channel the browser notification makes
   unnecessary.
5. **MCP recovery by in-stream reconnect, restart as the fallback
   (technical, (22), (204); user, (202) a).** The `/mcp reconnect` slash
   command fails under `-p` (`agents-plugin/hooks/claude-mod/reconnect.ts`
   `commandRefusal`), but the stream-json control request
   `{"subtype":"mcp_reconnect","serverName":...}` works: the lead probe
   2026-11-11 on CLI 2.1.322 answered it for `plugin:ws:ws` with `success`,
   and `mcp_status` then listed the server `connected`. When a daemon
   session's MCP server is broken, the daemon sends `mcp_reconnect` for it.
   The reconnect failed when the control response is an error or a
   follow-up `mcp_status` still does not list the server `connected` (the
   probe exercised only a healthy server). Only then does it restart the
   process and resume it through the backend session ID, and it does so
   automatically only at an idle point with an empty background-task list;
   until then the session shows an MCP-broken state, the header chip menu
   offers a manual restart, and the daemon checks again at each later idle
   point, restarting once the list is empty.
   The signal that marks a server broken (`mcp_status`, the init event's
   server status, or tool errors naming the server) is chosen in planning.
   The user reports that headless Claude also recovers a server by itself
   at tool-call time; this is unverified.
   Rejected: a process restart as the first step (the earlier form of this
   decision); an automatic restart at any idle point regardless of
   background work ((202) b); no automatic restart at all ((202) c).
6. **Idle stop: on for Claude, off for Pi (technical, (17); user, (17) note;
   announced, (174), (175)).** A Claude session stops its process when no
   turn is running and the last `background_tasks_changed` list was empty,
   after the lead profile's idle period: wsagent's `DefaultIdleTimeout` (10
   minutes), configurable through the profile's `IdleTimeoutSeconds`
   (`internal/wsagent/profile.go`). The next message or mail resumes it
   transparently. Pi reports no background tasks in its stream, so its idle
   stop stays off: the default stays off wherever background tasks are not
   visible. The user's reason for
   the default: mail is the only reason to keep an idle process alive, and
   the daemon now delivers it.
   Rejected: always-on processes with idle stop off by default (the first
   form of (17), superseded by the note).
7. **Turns no send started are first-class (technical, (172), (173);
   session state, user, (197) a).** The lead profile sets
   `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1`, and the CLI then emits
   `{"type":"system","subtype":"session_state_changed","state":...}` with
   `running`, `requires_action`, and `idle` (lead probe 2026-11-11, CLI
   2.1.322: `running` first, `requires_action` before a pending
   `can_use_tool`, `idle` after `result`; none without the variable). For
   Claude, `running` opens a turn whether or not a send started it, and
   `idle`, which the CLI schema calls the authoritative turn-over signal,
   closes it and flushes held sends; a `running` after `requires_action`
   within a turn opens no second turn. Without the events (an older CLI),
   a turn opens on a send or on the first stream event after a `result`,
   and `result` closes it. For Pi, turns follow its RPC events. Any `result` closes the current turn, including
   a local command's `num_turns: 0` result when no session-state events
   arrive; held sends flush at any turn end; the session shows as working while such a turn runs. The mapper drops the
   re-emitted last assistant message between `compact_boundary` and the
   compaction's `result`, which carries no `isReplay` flag.
   Rejected: closing at `result` while the events are on, which lets the
   previous turn's trailing `idle` close a send-started turn that the
   flush opened; send-driven turn tracking, which misses the turns compaction,
   background completions, Monitor events, and Pi push batches start on
   their own; showing the replay, which duplicates the final reply.
8. **Stop and close follow wsagent (announced, (176), (185)).** Stop ends
   the process and the next send resumes it; a stopped session stays in the
   rail's live list. Close ends the conversation for good, after a
   confirmation, and the session moves to ended sessions
   (`261108-feat-ws-daemon-agent-runner` Decisions, (328) a). Once Phase 2
   provides the background-task count, stop and close first confirm when
   the session has running background tasks, naming their count (technical,
   (203)).
9. **Slash commands pass through as text (technical, (177)).** No shim;
   a `/compact` typed for a Pi session maps to the RPC `compact` command.
   Phase 1 has no command palette; `init.slash_commands` stays available
   for a later one.
   Rejected: a command shim, since skills, built-ins, and unknown commands
   all behave under `-p` (probe).
10. **The spawn entry and form (user, (178) c, (183) a with note, (184) b,
    (188) a; technical, (185), (189)).** Settled from the mockup (session
    scratchpad `headless-spawn-mockup.html`; not kept). The "+ session" at
    the end of a worktree row (and on a plain-folder workspace bar,
    `261110-feat-ws-dashboard-workspaces` Decision 3) is the only spawn
    entry. It shows on every worktree row and plain-folder bar the rail
    lists, stored or derived from sessions, and does not depend on the
    workspace store (`DaemonOptions.WorkspacesFile`), so a daemon without
    the store can still spawn; a LAN viewer does not see it (Decision 14).
    It opens a popover that shows the target as one read-only line
    (workspace, branch, path), then the harness choice (Claude or Pi), then
    three compact rows with the label left and the control right: model,
    permission mode (default `auto`; disabled for Pi with a one-line reason,
    since a Pi lead gets no human permission prompts), and effort
    (Decision 11), then cancel and start. The choices come from
    `supDrawPop` and `GET /support/choices` as workspaces Decision 6 says.
    The first message is sent from the session's composer after spawn.
    This ticket builds the + buttons, the popover, and the spawn route;
    workspaces Phase 2 keeps only conversation handoff.
    Rejected: a worktree picker in the form and an always-visible "new
    session" in the rail footer ((188) b), because the picker repeats the
    rail's own list; model and mode folded behind "more" ((183) c) or left
    to the session menu ((183) b); a first-message field ((184) a); building
    the popover in both tickets.
11. **Effort is a session option (user, on (183) and the mockup; technical,
    (187)).** It sits beside model and permission mode, in the spawn form
    and in the session header's harness chip menu. It maps to Claude's
    `--effort <level>` and Pi's `--thinking <level>` (off, minimal, low,
    medium, high, xhigh, max), each list taken from its own harness;
    "default" passes no flag. A model, effort, or permission-mode change in
    the header menu applies in-stream from the next turn (user, (198) a):
    Claude takes `set_model`, `apply_flag_settings` with
    `{"settings":{"effortLevel":...}}`, and `set_permission_mode` with
    `{"mode":...}`, after which a `status` frame reports the new
    `permissionMode`; Pi takes `set_model` and `set_thinking_level`
    (wsagent `SetModel` and `SetEffort`). A return to "default" first tries
    the in-stream clear (`apply_flag_settings` with a null `effortLevel`,
    which the CLI accepts, and the matching model reset). The runner records
    the session's current model, effort, and mode, and every resume (stop,
    idle stop, MCP fallback restart) relaunches with those, not the spawn
    values. Restarting the process is the last resort: only a change with
    no in-stream form (a default that will not clear in-stream, a Pi model
    pattern that is not an exact id) stops the process and resumes it with
    `--resume` and the new flag, because a
    restart kills the session's linked background work, its shells and
    child sessions (user, (198) note). The lead probe 2026-11-11 on CLI
    2.1.322 answered `apply_flag_settings` and `set_permission_mode` with
    `success`, and `get_settings` then reported the applied effort. Moving
    Claude `SetEffort` off `ErrUnsupported` also changes the tracker support
    agent, which shares that backend, and its restart-expecting tests.
    Rejected: stop and resume for every effort or mode change ((198) b,
    85b9da5f, whose premise that Claude has no in-stream effort control
    held only for the `set_effort` subtype).
12. **Hosted-session UI (technical, (185)).** A hosted marker icon follows
    the harness badge; session states reuse the badge ring, with an
    approval-colour ring and an "approve" chip while a prompt waits. The
    approval card offers Allow and Deny, Deny expanding an optional message,
    and replaces the "waiting for approval in the terminal" text
    (`convo.perm`) for hosted sessions. A LAN viewer sees the card with a
    one-line reason and no buttons. Stop, close, model, mode, and effort
    live in the session header's harness chip menu, which a LAN viewer does
    not get (Decision 14).
13. **Two phases (technical, (180)).** Phase 1 spawn, deliver, approve;
    Phase 2 recovery, idle stop, and telemetry.
    Rejected: one phase, too large for one worker review cycle; Phase 1
    alone gives a usable session.
14. **One loopback gate for every session-control write (user, (192) a;
    epic (05), (13)).** Spawn, permission-mode, model, and effort changes,
    stop, close, and approval answers all accept only loopback
    (`localViewer` plus `sameOrigin`). A LAN viewer sees no + session button
    and no harness chip menu, and keeps watching and chatting.
    Rejected: spawn and mode change loopback-only with model, effort, stop,
    and close on the LAN token ((192) b); every route on the LAN token with
    only `bypassPermissions` and `dontAsk` loopback-only ((192) c). Under
    the support routes' `writeAllowed` pattern a LAN token alone could
    spawn a `bypassPermissions` lead, which breaks the epic's one
    trusted-origin boundary.
15. **The hosted flag holds only for the process the daemon spawned (user,
    (190) a).** The daemon-written row records the spawned harness PID. The
    hosted check in ws-mcp, the Claude mod, and the Pi extension honours
    `WS_HOSTED_SESSION` and `WS_HOSTED_SESSION_KEY` only when its harness
    process is that PID, and otherwise runs as a plain CLI session, so a
    harness a hosted session starts (a Bash `claude -p`) does not claim the
    lead's key. The exact check is fixed in planning, including its order:
    the PID is known only after the holder spawns the child (wsholder
    `ChildStatus.PID`), so either the row is written before the child's
    MCP boot or the check waits briefly for a row keyed by the hosted key;
    the mod and the Pi extension, which read only the env vars today
    (`agents-plugin/hooks/claude-mod/hosted.ts`), need a way to reach the
    row; and the row's PID is rewritten on every respawn (stop and resume,
    a change with no in-stream form (Decision 11), Phase 2's MCP restart
    and idle resume).
    Rejected: deferring it as a known limitation ((190) b); scrubbing at
    spawn alone, since the daemon cannot reach the environment the
    harness's own Bash tool passes on.
16. **Typed error states (user, (200) b; technical, completeness review
    2026-11-11).** Phase 2 maps five stream errors to typed states: login
    needed (`assistant.error` `authentication_failed`), model unavailable
    (`model_not_found`), usage limit with its reset time (the carrying event
    and field are identified in planning from Orca ADE
    `src/main/claude/claude-usage-limits.ts`), API retrying attempt N of M
    (`system/api_retry`), and a failed compaction's `compact_error` text.
    Each shows as a conversation row with its text (reset time, attempt N
    of M); login needed, model unavailable, and usage limit also put the
    badge ring in an error state until the next turn succeeds. Mapper
    fixtures are built from the shapes Orca ADE parses and the SDK types,
    since the 2026-11-11 probes captured none of these events; a live
    capture replaces a fixture when one occurs.
    Rejected: building them in Phase 1 ((200) a); leaving them out ((200) c).

## Prior Art

- `wsagent` runner and backends (Background).
- Orca ADE (github.com/stablyai/orca, MIT, commit bb1d4789c), the source
  of the question-tool denial (Decision 3) and the typed error states
  (Decision 16): `src/main/claude/claude-structured-prompt-replies.ts`,
  `claude-assistant-failures.ts`, `claude-usage-limits.ts`,
  `claude-api-retry-row.ts`, `claude-command-turn.ts`.
- The trusted-origin gate pattern: `localViewer` (`internal/wsdashboard/access.go`)
  plus `sameOrigin` (`inbox_routes.go`), as the system-open route
  (`sysopen_route.go`) applies them.

## Constraints

- The lead profile is the opposite of the support profile: it loads user
  and project settings, the ws plugin and its hooks, memory, and MCP
  servers. The `-p` coverage this needs (ws hooks, slash commands and
  skills, manual compaction, Pi RPC parity) was probed on 2026-11-11
  (`261110-research-ws-dashboard-agent-host-foundations` Verified
  Findings); a successful Pi RPC compaction is still unverified there.
- The registration duties the flag ticket left here
  (`261110-feat-ws-hosted-session-mode-flag` Phase 1 Scope): the spawn-time
  Registration write (harness and the reply-id derived from the key, never
  the key), its heartbeat refresh, ended marking, and keeping other ws-mcp
  processes from pruning a daemon-owned row. The spawn wires
  `AgentKeyStore.MintLead` and sets `WS_HOSTED_SESSION=1` and
  `WS_HOSTED_SESSION_KEY`. The flag's Result also records what else stops
  under the flag (whiteboard nudges and the page URL in `whiteboard.update`,
  `config.tune` unlisting); the Phase 1 report states which of these the
  daemon-written row restores. The variables a hosted session's child
  processes inherit are handled by Decision 15.
- A daemon session shows as exactly one session-index row: the
  daemon-written Registration row, which the registration duties above and
  Decision 15's PID need. The runner's own listing flag
  (`Profile.ListInIndex`, `Record.Listed` in `internal/wsagent/runner.go`)
  stays off for the lead profile.
- The dashboard feed is off for daemon sessions, so this ticket supplies
  the agents panel telemetry of
  `261110-feat-ws-dashboard-agent-panel-telemetry` (subagent model and
  effort, context tokens, output rate) and the background-task count of
  `261110-research-ws-dashboard-background-task-visibility` from the
  stream-json output ((41) ok).
- Daemon sessions and CLI sessions coexist in the session registry and the
  messenger.
- Restarting the harness process is a last resort in every case, because
  it kills the session's linked background work, its shells and child
  sessions (user, (198) note): an in-stream path is used wherever one
  exists (Decisions 5, 11).
- Quiet dashboard tone: `--wb-*` tokens, card and shadow, no flashy
  animation.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary (technical, (05); announced, (13)). Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone." — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "Daemon leads are headless ((12) b); the daemon injects input ((16)), approves through the SDK host from a trusted origin only ((05), (13)), defaults to auto with prompts waiting unanswered ((14) a), recovers MCP by restart and resume" — bearing: supports (MCP recovery narrowed to the fallback by Decision 5)
- 261110-feat-ws-hosted-session-mode-flag (2026-11-11, Result d09b414e0): "A harness process started from a hosted session (e.g. a Bash `claude -p`) inherits both variables and would claim the hosted key ... The daemon spawn or the hosts may want to scrub them for children." — bearing: constrains
- ff312f48 (2026-11-11, commit): "MintLead canonicalizes with wsgit.ExecRunner{} through canonicalSetupRoot so a later in-session relogin's root comparison matches. It is not wired into any daemon spawn; that belongs to the consumer ticket." — bearing: supports
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "Gate: localViewer plus sameOrigin (technical, (101) ok). ... Recognizing a desktop-shell origin waits until a shell exists ((19))." — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Decisions): "stop and close are separate (board (328) a): stop ends the process only, and the conversation resumes lazily on the next send like an idle exit; close ends the conversation for good" — bearing: constrains
- 85b9da5f (2026-11-08, commit): "Effort has no in-stream control (set_effort unsupported), so an effort change stops the process and the next send restarts it with --resume. REST writes require application/json and either no Origin or the dashboard's own origin." — bearing: superseded for effort by Decision 11 (REST write rule still constrains)
- 261110-bug-ws-dashboard-daemon-lock-scope (2026-11-10, Decisions): "Agent writes wake the page without waiting on the index re-read. agentChanged marks the conversation dirty and returns; the coalescing re-read runs off the caller, so neither the conversation loop nor r.mu waits on rereadMu" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/ profile.go claude.go pi.go runner.go, internal/wsdashboard/ access.go inbox_routes.go terminal_routes.go assets/dashboard.js, internal/wsholder/, internal/mcp/agent_keys.go MintLead, internal/wsi18n/tables |
| scope.surface | cross-module | wsagent runner and profiles, wsholder sessions, wsdashboard routes and session registry, mcp AgentKeyStore lead mint and Registration rows |
| scope.new_public_symbol | yes | a lead profile beside the support profile in internal/wsagent plus the daemon spawn and approval-answer routes; names unsettled |
| scope.new_type_contract | yes | can_use_tool request to the page and the control_response answer route allow with updatedInput or deny with message; route shape unsettled |
| scope.test_surface | existing | wsagent/runner_test.go claude_test.go pi_test.go stream_test.go profile_test.go, wsdashboard/holders_test.go terminal_routes_test.go |
| complexity.reuse_points | confirmed | wsagent Runner Create Send Interrupt and armIdle at runner.go#L199 #L315 #L339 #L920, localViewer access.go#L144, sameOrigin inbox_routes.go#L122, AgentKeyStore.MintLead agent_keys.go#L36, supDrawPop and GET /support/choices |
| complexity.side_effect_risk | high | spawns long-lived lead processes with the user's settings, plugins, hooks, and auto permission mode under holders that outlive the daemon, and writes daemon-owned Registration rows |
| risk.correctness | high | turn tracking for unsolicited results, held sends, approval waits, MCP reconnect with restart fallback, session-state events, and idle stop interact across daemon restarts |
| risk.fit | moderate | Phase 1 is settled by Decisions 1-15 and Phase 2's errors by Decision 16; Phase 2 idle stop follows Decision 6 from the 2026-11-11 probe (261110-research-ws-dashboard-background-task-visibility is informational) and Pi RPC compaction is unverified |
| risk.test | high | real claude -p and Pi RPC behavior, permission prompts, and background-task visibility need live-harness checks beyond unit fakes |
| risk.security_or_contract | high | browser approval of tool execution reopens 261106 Non-Scope 241 b and must hold the loopback-plus-sameOrigin boundary against LAN viewers |

## Phases

### Phase 1: spawn, deliver, approve

Scope:
- A lead profile in `internal/wsagent` beside the support profile
  (Constraints), with permission mode, model, and effort as spawn options
  and `--permission-prompt-tool stdio` (Decisions 3, 11).
- The spawn route and its UI (Decision 10), the hosted env and lead-key
  mint, the registration duties and the single index row (Constraints),
  and the hosted-flag PID check (Decision 15).
- A spawned session runs under a session holder (Decision 1): a
  holder-backed `Process` behind wsagent's Backend and Process interfaces
  (`internal/wsagent/backend.go`; today `exec.Command` in `claude.go` and
  `pi.go`), the move `261110-feat-ws-dashboard-session-holder` Decision 11
  left here. On daemon start the runner reattaches to live holders and
  rebuilds its state (running turn, held sends, a pending `can_use_tool`)
  from the holder's replay. It persists the last holder sequence it
  applied in its existing conversation record (the consumer-side duty of
  holder Decision 17, not a new per-holder file), so the full replay on reattach (holder Decision 17) adds no
  duplicate entries, and it never starts a `--resume` process while the
  holder's process still runs on that session ID.
- Dashboard messages and mail delivered as user turns (Decision 2): the
  daemon watches the minted key's mailbox, claims each message, and writes
  a turn whose text matches what the mod's mailbox wait hands the model
  today. Turns no send started and the compaction replay are handled
  (Decision 7), and slash commands pass through (Decision 9).
- The approval wire: `can_use_tool` to the page, the approval card, the
  loopback-only answer route, the awaiting-approval state and browser
  notification (Decisions 3, 4, 12).
- Stop and close, and the header chip menu with in-stream model, effort,
  and permission-mode changes (Decisions 8, 11, 12), all behind the
  loopback gate (Decision 14).
- The session-state variable and its turn handling (Decision 7), and the
  question-tool denial (Decision 3).

Verification:
- Mapper tests on fixtures cut from the 2026-11-11 probe logs:
  `can_use_tool` and its `control_response`, an unsolicited `result`, a
  `num_turns: 0` result, and the compaction replay; on fixtures from the
  CLI 2.1.322 probe, `session_state_changed` `running` opening a turn no
  send started, `idle` closing a turn, and `requires_action` beside a
  pending `can_use_tool`; a steered message written mid-turn, injected
  after the next tool result, with one `result` for both;
  `result`, a held-send flush, then the trailing
  `idle` (the flushed send's turn stays open); `requires_action`,
  `can_use_tool`, deny, `running` (one turn); a `requires_action` with no
  pending `can_use_tool` showing its visible state; a question-tool
  `can_use_tool` answered with a deny and its message.
- A profile test that the lead profile sets
  `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1`.
- Route tests: spawn, a permission-mode, model, or effort change, stop,
  close, and an approval answer from a LAN-token request are each refused;
  the same requests from loopback reach the process.
- A hosted-flag test: a process whose harness PID differs from the row's
  ignores the flag and runs as a plain session.
- A live `claude -p` checklist: spawn from the popover, send a message and
  a mail, steer a message during a tool-running turn, approve and deny a
  prompt, stop and resume on the next send
  (the resumed process still runs hosted: the daemon delivers its mail and
  no file-inbox wait starts), close; the session row and page URL stay fixed throughout, as one row.
  A Bash `claude -p` the daemon lead runs does not take the lead's key. A
  daemon restart mid-session reattaches to the holder with no duplicate
  conversation entries and no second process, and a `can_use_tool` pending
  across the restart still shows its awaiting-approval state and card and
  can be answered. A model, effort, and permission-mode change from the
  header menu applies without restarting the process (the harness PID
  stays the same); after an in-stream effort change, a stop and a send
  resume a process that runs with the changed effort. A Pi spawn sends and
  receives.
- A browser check of the popover, the approval card, and the LAN viewer at
  the rail's widths, and of the awaiting-approval browser notification,
  including the LAN tab's "answer on the dashboard PC" line.

### Result (9a18edefc) - 2026-11-11

Landed on `impl/develop/down-gents-rally` (`7fa06f83b..9a18edefc`):

- **Runner spine** (`internal/wsagent`): built-in `lead-claude` and
  `lead-pi` profiles; `POST /agents/leads`; a holder-backed process
  (`holder.go`) with rejoin, replay up to the holder's sequence, and the
  applied sequence kept in the conversation record; the daemon-written
  registration (`hosted`, `host_pid`); the hosted environment and lead-key
  mint (`internal/mcp/agent_keys.go`).
- **Turns and input**: `idle` ends a Claude turn once session-state lines
  are seen (a `result` ends it only without them); unsolicited turns and
  the compaction replay; steered inbox entries written to the running
  process (Claude user line, Pi `steer`); mail claimed by the daemon
  (`internal/mcp/lead_mail.go`) and submitted as a mail turn.
- **Approvals**: `can_use_tool` to pending approvals keyed by request id,
  the answer route, automatic denial of `AskUserQuestion`, `ExitPlanMode`,
  and requests that need user interaction, the `action_needed` state, and
  answered ids kept across a daemon restart.
- **Settings**: model, effort, and permission mode in-stream; the record's
  values on every resume.
- **Hosted flag** (Decision 15): ws-mcp, the Claude mod, and the Pi
  extension honour the flag only when the registration's `host_pid` is
  their own harness process; the launcher states the harness PID. A ws-mcp
  that is not the hosted harness leaves a `hosted` row alone.
- **Gate** (Decision 14): spawn, settings, stop, close, and approval answers
  need a browser on this machine with the daemon's own Origin.
- **Page UI** (`internal/wsdashboard`): the rail's "+ session" button and
  spawn popover, hosted mark, approval cards, awaiting-approval ring and
  chip, harness chip menu, notification, `action_needed` notice, 40 strings
  in all 11 tables.
- **Manual**: `ai-docs/manuals/ws-mcp.md` (lead sessions, environment
  table, an isolated probe daemon).

Verification:

- `ws-rgo -- test ./internal/wsagent/ ./internal/wsdashboard/
  ./internal/wsholder/ ./internal/mcp/ ./internal/wsrsrc/ ./internal/wsi18n/
  ./internal/wslive/ ./internal/wsinbox/ ./internal/wsmailbox/ ./cmd/...
  -count=1`: all ok. `ws-rgo -- vet ./...`: clean.
- Plugin tests for ws and wsflow, the launcher unit tests, and the Pi hosted
  integration test (11/11) passed at the hosted-flag commit; no plugin file
  changed after it.
- Fixture tests cut from the probe logs: `can_use_tool` and its response,
  an unsolicited `result`, a `num_turns: 0` result, the compaction replay,
  session-state lines, a steer. Route tests refuse each control write from a
  LAN request and pass it from loopback. Hosted-flag tests cover a differing
  harness PID.
- Live, against an isolated daemon built from this branch, with a real
  `claude -p` (2.1.322) loading this branch's plugin through an `exec`
  wrapper: spawn; composer message; allow and deny (with message) answers;
  a second answer 409; control writes without Origin 403; `ferrule` held
  the session to its minted key (no second key, one row); daemon stop with
  a prompt pending, restart by the session's launcher, same pending prompt
  answered, no duplicate ring entry, one process; mail from another session
  became a mail turn; model, effort, and permission mode changed with the
  same PID; a steered message joined a turn that was running a tool; stop,
  then mail resumed the process with `--resume` and the recorded model,
  effort, and mode, still hosted, no mailbox wait process; a nested
  `claude -p` run by the lead minted its own key; close end-marked the row
  and cleared the listening marker. A Pi spawn sent and received.
- Browser (headless Chromium over CDP): with a stand-in harness, the
  popover, one and two cards, the deny message, the LAN read-only view, the
  chip menu, rail and index states, and the notification text for local and
  LAN tabs at 1100 and 1700 px, light and dark; with the real harness, the
  approval card rendered and its Allow button ran the tool.

Decisions:

- Strict PID equality for the hosted flag; a forking `claude` wrapper makes
  the session ordinary. Accepting an ancestor was rejected: it re-admits the
  nested harness.
- Under the hosted flag the daemon row restores the page URL in
  `whiteboard.update` and store-change nudges (an in-memory entry, no
  write). Unlisting through `config.tune` is not restored: the daemon owns
  the row.
- Restart of the harness is the last resort for a settings change.
- The page reads `GET /agents/<id>` on load and on the events the runner
  already pushes; no timer.
- Per-viewer controls are hidden by CSS under a local-viewer mark because
  the rail region is one render for all viewers; the routes refuse
  regardless.

Forward notes (Orca benchmark, not adopted here): crash-safe claimed mail
through the `--replay-user-messages` echo; a pre-assigned `--session-id`
with a typed "No conversation found" refusal; process-group reap in the
holder; no global `<synthetic>` filter or unknown-frame logging; dropping
`ANTHROPIC_API_KEY` from the hosted environment.

Review (lite, one pass): no Critical or Important finding; six Minor, left
open below.

Open after this phase:

- A plain-folder bar shows "+ session" (Decision 10), but the spawn route
  refuses a root outside a Git worktree, since a lead key is minted at a
  worktree root; the popover shows the refusal.
- `action_needed` reaches the live snapshot as a permission wait, so the
  rail and the notification show it like an approval; only the page shows
  the notice. The rail and notification name the oldest pending prompt
  only.
- A plain-http LAN tab cannot raise an OS notification; it shows a toast.
- A daemon build without lead sessions started over the same state
  directory rewrites a lead's row as an unlisted agent row; a build with
  them restores it at start.
- The turn-end ring entry written after a daemon restart carries no origin.
- Review Minor: a holder call can time out while more than 256 output
  lines wait unread; a steered message whose write fails is shown but not
  returned; a settings request with a bad `permission_mode` applies its
  model and effort before answering 400; a rejoin that fails before
  attaching lets the next send end the still-running holder; an
  auto-denied prompt can be denied twice after a daemon crash; a forking
  `claude` or `pi` shim makes the session ordinary without a notice.
- Not run: popover spawn against the real harness in a browser (the route
  and the popover were each checked), an OS-level notification, and a live
  Pi approval or mail turn.

### Phase 2: recovery, idle stop, and telemetry

Scope: MCP recovery by in-stream reconnect with the restart fallback, the
MCP-broken state, and the header menu's manual restart behind the
loopback gate (Decisions 5, 14); idle stop (Decision 6); the agents panel
telemetry and background-task count from the stream (Constraints); the
stop and close confirmation naming the running background-task count
(Decision 8); and the typed error states (Decision 16).

Verification: mapper tests on `background_tasks_changed` fixtures, on
each typed error state (Decision 16's fixtures), and on the telemetry
fields (subagent model and effort, context tokens, output rate) mapped
from the stream; a test that idle stop waits while a background task runs
and stays off for a Pi session; MCP recovery tests that a successful
`mcp_reconnect` restarts nothing, that a failed one (an error response, or
`mcp_status` still not `connected`) restarts at an idle point with an
empty background-task list, and that while a background task runs the
session shows the MCP-broken state, offers the manual restart (refused
from a LAN-token request), and restarts at the first idle point after the
list empties; a route or UI test that stop and close with running
background tasks confirm first and show the count; a browser check of the
typed error rows and badge ring; a live checklist that a broken MCP
server reconnects in-stream with the same harness PID, and that an
idle-stopped Claude session resumes on mail, each resumed process still
running hosted.
