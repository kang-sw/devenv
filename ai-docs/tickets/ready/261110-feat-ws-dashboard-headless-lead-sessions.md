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
sage-review-design-reviewed: 6bc5b7e4ab4ac97d
sage-review-completeness-reviewed: 6bc5b7e4ab4ac97d
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
   for CLI sessions only.
3. **Approval through the stdio prompt tool, answered only from loopback
   (announced, (13), (169), (171); trust boundary (05)).** The lead profile
   passes `--permission-prompt-tool stdio`. Each prompt arrives as a
   `control_request` `can_use_tool` (tool, input, description,
   `permission_suggestions`, `tool_use_id`) and the daemon answers it on
   stdin with a `control_response`: `allow` with `updatedInput`, or `deny`
   with `message`. A session is awaiting approval exactly while a
   `can_use_tool` request is pending, since the stream carries no waiting
   marker ((170)). Answers are accepted only from loopback (`localViewer`
   plus `sameOrigin`), the boundary the dashboard terminal landed with
   (`agents-plugin-tool/internal/wsdashboard/terminal_routes.go#L44-L49`,
   `261110-feat-ws-dashboard-terminal` Decision 5); a desktop-shell origin
   is recognized only once a shell exists (epic (19)). This reopens
   `261106-epic-ws-dashboard-serve-daemon` Non-Scope 241 b under that
   condition.
   Rejected: `--permission-prompts host` alone, which the 2026-11-11 probe
   showed auto-denies every prompt under stream-json
   (`261110-research-ws-dashboard-agent-host-foundations` Verified
   Findings).
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
5. **MCP recovery by restart and resume (technical, (22)).** `/mcp reconnect`
   fails under `-p` (`agents-plugin/hooks/claude-mod/reconnect.ts`
   `commandRefusal`). When a daemon session's MCP connection is broken, the
   daemon restarts its process at an idle point and resumes it through the
   backend session ID. The signal that marks a connection broken (for
   example the init event's MCP server status, or tool errors naming the
   server) is chosen in planning.
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
7. **Turns no send started are first-class (technical, (172), (173)).** Any
   `result` closes the current turn, including a local command's
   `num_turns: 0` result; held sends flush at any turn end; the session
   shows as working while such a turn runs. The mapper drops the
   re-emitted last assistant message between `compact_boundary` and the
   compaction's `result`, which carries no `isReplay` flag.
   Rejected: send-driven turn tracking, which misses the turns compaction,
   background completions, Monitor events, and Pi push batches start on
   their own; showing the replay, which duplicates the final reply.
8. **Stop and close follow wsagent (announced, (176), (185)).** Stop ends
   the process and the next send resumes it; a stopped session stays in the
   rail's live list. Close ends the conversation for good, after a
   confirmation, and the session moves to ended sessions
   (`261108-feat-ws-daemon-agent-runner` Decisions, (328) a).
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
    "default" passes no flag. A model or effort change in the header menu
    stops the process and the next send resumes it with `--resume` and the
    new flag (85b9da5f).
    Rejected: a live change, since both CLIs take effort only as a launch
    flag and the stream has no set_effort.
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
    a model or effort change, Phase 2's MCP restart and idle resume).
    Rejected: deferring it as a known limitation ((190) b); scrubbing at
    spawn alone, since the daemon cannot reach the environment the
    harness's own Bash tool passes on.

## Prior Art

- `wsagent` runner and backends (Background).
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
- Quiet dashboard tone: `--wb-*` tokens, card and shadow, no flashy
  animation.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary (technical, (05); announced, (13)). Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone." — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "Daemon leads are headless ((12) b); the daemon injects input ((16)), approves through the SDK host from a trusted origin only ((05), (13)), defaults to auto with prompts waiting unanswered ((14) a), recovers MCP by restart and resume" — bearing: supports
- 261110-feat-ws-hosted-session-mode-flag (2026-11-11, Result d09b414e0): "A harness process started from a hosted session (e.g. a Bash `claude -p`) inherits both variables and would claim the hosted key ... The daemon spawn or the hosts may want to scrub them for children." — bearing: constrains
- ff312f48 (2026-11-11, commit): "MintLead canonicalizes with wsgit.ExecRunner{} through canonicalSetupRoot so a later in-session relogin's root comparison matches. It is not wired into any daemon spawn; that belongs to the consumer ticket." — bearing: supports
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "Gate: localViewer plus sameOrigin (technical, (101) ok). ... Recognizing a desktop-shell origin waits until a shell exists ((19))." — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Decisions): "stop and close are separate (board (328) a): stop ends the process only, and the conversation resumes lazily on the next send like an idle exit; close ends the conversation for good" — bearing: constrains
- 85b9da5f (2026-11-08, commit): "Effort has no in-stream control (set_effort unsupported), so an effort change stops the process and the next send restarts it with --resume. REST writes require application/json and either no Origin or the dashboard's own origin." — bearing: constrains
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
| risk.correctness | high | turn tracking for unsolicited results, held sends, approval waits, MCP restart-and-resume, and idle stop interact across daemon restarts |
| risk.fit | moderate | Phase 1 is settled by Decisions 1-15; Phase 2 idle stop follows Decision 6 from the 2026-11-11 probe (261110-research-ws-dashboard-background-task-visibility is informational) and Pi RPC compaction is unverified |
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
- Stop and close, and the header chip menu (Decisions 8, 11, 12), all
  behind the loopback gate (Decision 14).

Verification:
- Mapper tests on fixtures cut from the 2026-11-11 probe logs:
  `can_use_tool` and its `control_response`, an unsolicited `result`, a
  `num_turns: 0` result, and the compaction replay.
- Route tests: spawn, a permission-mode, model, or effort change, stop,
  close, and an approval answer from a LAN-token request are each refused;
  the same requests from loopback reach the process.
- A hosted-flag test: a process whose harness PID differs from the row's
  ignores the flag and runs as a plain session.
- A live `claude -p` checklist: spawn from the popover, send a message and
  a mail, approve and deny a prompt, stop and resume on the next send
  (the resumed process still runs hosted: the daemon delivers its mail and
  no file-inbox wait starts), close; the session row and page URL stay fixed throughout, as one row.
  A Bash `claude -p` the daemon lead runs does not take the lead's key. A
  daemon restart mid-session reattaches to the holder with no duplicate
  conversation entries and no second process, and a `can_use_tool` pending
  across the restart still shows its awaiting-approval state and card and
  can be answered. A Pi spawn sends and receives.
- A browser check of the popover, the approval card, and the LAN viewer at
  the rail's widths, and of the awaiting-approval browser notification,
  including the LAN tab's "answer on the dashboard PC" line.

### Phase 2: recovery, idle stop, and telemetry

Scope: MCP restart and resume (Decision 5), idle stop (Decision 6), and the
agents panel telemetry and background-task count from the stream
(Constraints).

Verification: mapper tests on `background_tasks_changed` fixtures and on
the telemetry fields (subagent model and effort, context tokens, output
rate) mapped from the stream; a test that idle stop waits while a
background task runs and stays off for a Pi session; a live checklist that a
broken MCP connection restarts and resumes at an idle point, and that an
idle-stopped Claude session resumes on mail, each resumed process still
running hosted.

