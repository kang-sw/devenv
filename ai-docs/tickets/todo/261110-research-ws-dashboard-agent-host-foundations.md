---
title: "Foundations for the dashboard agent host: archived terminal substrate, headless lead sessions, permission boundary, multi-session and workspace management, desktop shell"
parent: 261110-epic-ws-dashboard-agent-host
related:
  260730-research-ws-dashboard-drop-for-orca: the decision this research revisits; read its accepted costs
  261106-epic-ws-dashboard-serve-daemon: current daemon; Non-Scope (241 b) on browser permission approval
  261108-feat-ws-tracker-support-agent-chat: the headless Claude/Pi runner this research would generalize
---

# Foundations for the dashboard agent host

## Background

The user (2026-11-10) proposed that the dashboard daemon become the
first-tier program: it creates lead sessions (Claude Code and Pi, headless),
manages many sessions and workspaces, offers a terminal, and later lives in
a desktop shell like an agent development environment. The parent epic
records the direction ((538) b). This ticket gathers what the children need
before any of them can settle a contract. The user expects many questions in
multi-session and workspace management in particular.

## Topic 1: the archived ws-dashboard terminal substrate

The earlier dashboard attempt, dropped for Orca on 2026-07-30, is preserved
at tag `archive/ws-dashboard` (`bc2e54e89`). It is a Rust workspace
(`ws-dashboard/crates/{core,daemon,harness-core,harness-cli}`) with a web
frontend (`ws-dashboard/frontend`). Its terminal used `portable_pty` and was
carried through cross-platform work. Tickets in `.done/`:
`260516-feat-ws-web-terminal-session-substrate`,
`260516-bug-ws-web-terminal-websocket-transport`,
`260516-bug-ws-web-terminal-cross-platform-portability`,
`260523-feat-ws-dashboard-terminal-tab-restore`,
`260517-bug-ws-dashboard-terminal-focus-browser-gate-regression`, and
`260620-bug-dashboard-terminal-inherits-dumb-term`. The user recalls it was
verified on macOS, Linux, and Windows.

To investigate:
- What exactly it verified on each platform, per those tickets' Results and
  the archived `.plans/` evidence files. Which failures it hit.
- Its session model: daemon-owned PTY sessions, reattach, scrollback, tab
  restore, websocket transport, shell selection.
- How to carry it into today's Go daemon (`agents-plugin-tool`): port the
  design to Go (PTY on POSIX plus ConPTY on Windows), run the Rust daemon as
  a sidecar, or reuse only the frontend and protocol. Cost and risk of each.

## Topic 2: headless lead sessions spawned by the daemon

`agents-plugin-tool/internal/wsagent` already runs headless conversations
for the tracker support agent. The Claude backend (`claude.go`) runs
`claude -p` with stream-json in and out and stdin held open, one process per
conversation. Pi uses its RPC (`pi.go`). The runner (`runner.go`) supports
create, send (held to turn end and merged), interrupt, set_model, resume
through the backend session ID, and an idle stop.

The support profile deliberately isolates the agent. It sets
`--setting-sources ""` with an explicit `--settings`, `--strict-mcp-config`,
a tool allowlist, and `--permission-mode dontAsk`, and it blanks the mailbox
identity. A lead session needs the opposite.

To investigate:
- What a lead profile must load: user and project settings, the ws plugin
  and its hooks, memory, and MCP servers. Does the ws session-start flow
  (session key, workflow manual) work unchanged under `-p`?
- What `-p` loses against the TUI: slash commands, plan mode, interactive
  compaction, and any other feature the dashboard would have to re-provide.
- How daemon-hosted sessions coexist with CLI-created ones in the session
  registry and the messenger.
- Pi parity for each of the above.

## Topic 3: the permission-approval boundary

Headless mode has no interactive approval. The current daemon epic rejected
browser approval and turn interrupt (241 b). Its reason was that the LAN
token would then authorize tool execution, a security boundary needing its
own design. Interrupt has since been added (round 2); approval has not.

To investigate:
- Claude's `--permission-prompt-tool` (an MCP tool that answers approval
  requests) and Pi's equivalent, if any.
- Where approval may come from: loopback only, a desktop-shell origin, a LAN
  device with a stronger credential, or no browser approval at all
  (per-session permission modes and allowlists instead).
- How approval requests surface in the dashboard, and what happens when no
  approver is present.

## Topic 4: multi-session and workspace management

The user flagged this as the area with the most open questions.

To investigate:
- Session lifecycle from the dashboard: create, resume, fork, compact, stop,
  archive. Which of these Claude and Pi expose headlessly.
- What a workspace is: a repository, a worktree, or a set of them. How it
  maps to ws worktree pools (`ws/worktree.acquire`) and session keys.
- Several sessions per workspace. Lead and worker visibility. How the
  existing agents panel and session page scale.
- Daemon restart and crash: which sessions survive, and how they reattach.

## Topic 5: desktop shell

Historical direction (user, 2026-11-10, (18) note, (19)): defer shell review
until browser completion. Superseded by (493)=b on 2026-11-13 and MAIN's
2026-11-14 15:11:55 and 15:14:52 start-now relays, which also supersede the
after-release scheduling assumption (701). Build and dogfood the thin Tauri
shell in parallel under `261113-feat-werkschaft-thin-tauri-shell`.

Its independent Phase 1a is a macOS window loading the existing exact
`http://127.0.0.1:<port>/` URL after endpoint discovery (including cache
override) and `/health`, with retry/error UI, external browser links, and
close leaving the daemon running. It adds no credential or daemon API and
performs no daemon start/stop/swap. Single-instance focus follows the first
window; broader WKWebView acceptance, safe missing-daemon start, Windows,
zip, signing, and updater are later slices, not first-window blockers.
Owner (798)=a (MAIN relay 2026-11-14 15:17:49) confirms a later gated
JS-memory scoped grant when actual new shell-only HTTP authority is needed,
superseding (702)=a for that need. The thin-shell ticket records the exact
trusted-origin/window bootstrap ACL, short life and exit/crash revocation,
credential/authority exclusions, and retained browser/content guards. This
does not authorize a credential or daemon route change in Phase 1a.

To investigate:
- Tauri against Electron, or another option, for wrapping the existing web
  UI with the daemon as a sidecar: bundle size, WebView consistency across
  macOS, Windows, and Linux, and auto-update.
- What the shell adds that the browser cannot: a trusted local origin for
  Topic 3, native notifications, OS integration.
- Code signing: see `261109-research-windows-code-signing-signpath`.

## Outcome Ledger

### Verified Findings
- The 2026-07-30 drop happened because Orca implemented the generic half
  (workspaces, worktrees, integrated terminal, documents, git) better. Its
  accepted cost was permanently giving up programmatic session control,
  since Orca exposes only `terminal.sendText`
  (`.done/260730-research-ws-dashboard-drop-for-orca`).
- The archived dashboard is at tag `archive/ws-dashboard` (`bc2e54e89`,
  2026-07-31). It is Rust crates plus a web frontend, and its terminal used
  `portable_pty` (`git show archive/ws-dashboard:ai-docs/tickets/.done/260516-bug-ws-web-terminal-cross-platform-portability.md`).
- Today's Go daemon has no PTY or terminal code. A search for pty, xterm,
  and creack under `agents-plugin-tool` finds none in production code.
- `wsagent` runs headless Claude via `claude -p --input-format stream-json
  --output-format stream-json`, one long-lived process per conversation,
  with resume, interrupt, set_model, and an idle stop (`wsagent/claude.go`,
  `wsagent/runner.go`).
- Lead sessions are not spawned by the daemon today. The CLI registers
  itself in the dashboard session registry (`wsdashboard/registry.go`).
  Messages reach it through a per-session file inbox that the adapter's
  mailbox wait drains (`wsinbox`, `wsdashboard/inbox_routes.go`).
- The `261106-epic-ws-dashboard-serve-daemon` Non-Scope rejects browser
  permission approval because a LAN token would authorize tool execution
  (241 b). It also rejects the dropped MVP's browser terminal, file
  explorer, and editor.

- The archived terminal did not survive daemon restarts. `TerminalRegistry`
  lived in daemon memory and the daemon owned each PTY child
  (`git show archive/ws-dashboard:ws-dashboard/crates/daemon/src/terminal.rs`);
  `260523-feat-ws-dashboard-terminal-tab-restore` accepted that live
  terminal processes need not survive daemon death and restored tabs as new
  shells.
- python3 is a prerequisite on every host: `agents-plugin/.mcp.json` starts
  `python3 ./bin/ws-mcp-launcher.py`, the Pi bridge spawns the same launcher
  (`agents-plugin-pi/src/mcp-stdio-client.ts`), and Windows needs `python3`
  on PATH (`ai-docs/manuals/ws-mcp.md`).
- `claude` 2.1.322 `--help`: under `--print`, `--permission-prompts host`
  (the default) lets the SDK host or `--permission-prompt-tool` answer
  permission prompts; `none` denies anything that would prompt.
  `--permission-mode` accepts acceptEdits, auto, bypassPermissions, manual,
  dontAsk, plan.
- Claude mod and Pi extension under a daemon host (Explore, 2026-11-10):
  mailbox push, the board-inbox drain, and interrupt handling claim the
  same messages a daemon would deliver (recv claims); the dashboard feed
  duplicates the stream the daemon reads; key minting, manual injection,
  key adoption, preload expansion, and `request_compaction` (works under
  `-p` through `$.command.run('compact')`) do not conflict.
  `request_mcp_reconnect` fails under `-p` (`claude-mod/reconnect.ts`
  `commandRefusal`). No per-process switch disables the mod or the Pi
  mailbox waiter; the Pi waiter arms whenever the role is unset and a key
  exists, with no RPC-mode gate (`agents-plugin-pi/src/index.ts`,
  `mailbox-waiter.ts` `shouldArmMailboxWaiter`).
- The daemon now hosts every MCP server behind a Python proxy launcher and
  is replaced on version change; the proxy reconnects transparently
  (`261110-feat-ws-mcp-daemon-hosted-proxy-launcher` Decisions 1, 7).

- Live probe, 2026-11-11 (claude 2.1.322, ws 0.48.17, pi 1.0.4; scratch
  repo; logs under the lead session's scratchpad `probe-hosted/`):
  - Under `claude -p` stream-json the ws mod loads (`ui_log` "ws mailbox:
    armed"), spawns `mailbox wait --board-inbox --board-interrupt`, mints a
    key, injects the workflow manual, expands preload, registers the
    session, and runs the dashboard feed (`dashboard_intake`). Key adoption
    on revive re-arms the wait on the new key. The `SessionStart:compact`
    hook runs. `system/init` is emitted on every user turn and carries
    `slash_commands`, `terminal_slash_commands`, `mcp_servers`, and
    `permissionMode`.
  - Slash commands sent as user turns work: skills expand, built-ins
    (`/context`, `/compact`) run locally and end with a `result` that has
    `num_turns: 0` and a null `terminal_reason`, and an unknown command
    reaches the model as text.
  - Manual compaction works by `/compact` and by the mod's
    `request_compaction` lever. After compaction the stream re-emits the
    last assistant message without an `isReplay` flag. The lever's
    compaction runs after its turn ended and ends with a `result` no send
    caused.
  - Permission prompts: with stream-json in and out and no
    `--permission-prompt-tool`, a prompt is auto-denied
    (`system/permission_denied`); `--permission-prompts host` alone does not
    route it to stdin. With `--permission-prompt-tool stdio` the prompt
    arrives as `control_request` `can_use_tool` (tool, input, description,
    `permission_suggestions`, `tool_use_id`) and is answered on stdin by a
    `control_response` with `behavior` `allow` (with `updatedInput`) or
    `deny` (with `message`). There is no timeout (answered after 90 s), and
    the stream carries no waiting marker.
  - Background tasks: `system/background_tasks_changed` carries the full
    current task list on every change (empty when none run), with
    `task_started`, `task_updated`, and `task_notification` around it.
    Background completions and Monitor events start turns with no input.
  - Pi `--mode rpc` with the ws extension: key minting, manual injection
    through the system prompt, `/skill:` expansion, and key adoption on
    revive work. The mailbox waiter arms with `--board-inbox
    --board-interrupt` and drains and aborts in RPC mode (no RPC gate). The
    RPC `compact` command and the `ws-compact` tool reach Pi's compaction,
    but every attempt refused a too-small session, so a successful RPC
    compaction is unverified; a typed `/compact` is not a Pi RPC built-in.
    Pi core reports no background tasks in the stream; the ws extension's
    background tasks show only as tool events and a pushed completion
    message, and their count exists only in process
    (`agents-plugin-pi/src/background-tasks.ts` `countBackgroundTasks`).
    A Pi lead gets no human permission prompts: native bash and read are
    removed from the lead (`execute-gateway.ts` `computeLeadActiveTools`)
    and mutation goes through lead-approved `ws-execute` workers (code read,
    not probed).

### Confirmed Decisions
- Pursue the full agent-host scope as one epic, terminal and desktop shell
  included (user, 2026-11-10, whiteboard (538) b). This partially reverses
  `260730-research-ws-dashboard-drop-for-orca`.
- Include the archived ws-dashboard terminal work in this research (user,
  2026-11-10, (538) note).

- Two tracks in one epic: agent management and ADE surfaces (user,
  2026-11-10, (01); (02)).
- "Resident" sessions mean process survival across daemon restarts through
  a per-session Python holder ((03) a, (06) a and note, (09), (10), (25));
  child `261110-feat-ws-dashboard-session-holder`.
- Daemon leads are headless ((12) b); the daemon injects input ((16)),
  approves through the SDK host from a trusted origin only ((05), (13)),
  defaults to `auto` with prompts waiting unanswered ((14) a), recovers MCP
  by restart and resume ((22)), and stops idle processes by default once it
  owns delivery ((17)); child
  `261110-feat-ws-dashboard-headless-lead-sessions`.
- One hosted-session env flag turns off input delivery and the feed in the
  Claude mod and the Pi extension; the daemon mints the key ((20), (21));
  child `261110-feat-ws-hosted-session-mode-flag`.
- A workspace is a root worktree or a plain folder ((08) b, (11));
  conversation handoff works both ways between CLI and daemon ((15) a);
  child `261110-feat-ws-dashboard-workspaces`.
- The terminal runs on holder PTYs behind the trusted-origin boundary; the
  archived Rust PTY code is not carried over ((05), (23)); child
  `261110-feat-ws-dashboard-terminal`.
- File browsing is read-only with HTML, image, and video preview; editing is
  out of scope; HTML and SVG render isolated ((04) a, (07)); child
  `261110-feat-ws-dashboard-file-preview`.
- Historical: review the desktop shell after browser completion ((18),
  (19)). Superseded by (493)=b and MAIN's 2026-11-14 start-now direction:
  `261113-feat-werkschaft-thin-tauri-shell` owns the independent attach-only
  macOS first window now; Windows, packaging, signing, and updater do not
  gate it. Owner (798)=a confirms the later scoped grant only for an actual
  new shell-only authority need, superseding (702)=a then; its controls and
  future verification belong to the thin-shell ticket, not Phase 1a.
  Orca is not a design target ((24)).

### Proposals
- Keep CLI-created terminal sessions first-class next to daemon-hosted ones,
  because `-p` lacks some TUI features.

### Open Questions
- Not probed under `-p`: `/clear`, `/model`, `/mcp`, `/effort`, plan mode,
  the SDK `initialize` route as an alternative to
  `--permission-prompt-tool stdio`, answers carrying `updatedPermissions`,
  and whether background tasks survive a process stop and `--resume`.
- A successful Pi RPC compaction on a session large enough to compact.

### Rejected Alternatives
- Conversation-only survival across daemon restarts ((06) b, c); a TUI in a
  PTY for daemon leads ((12) a); file editing in the dashboard ((04) b, c).
- A research ticket limited to headless lead sessions and the permission
  boundary ((538) a). Keeping CLI-created sessions as the only kind
  ((538) c).
