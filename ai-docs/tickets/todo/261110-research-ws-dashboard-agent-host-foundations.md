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

### Confirmed Decisions
- Pursue the full agent-host scope as one epic, terminal and desktop shell
  included (user, 2026-11-10, whiteboard (538) b). This partially reverses
  `260730-research-ws-dashboard-drop-for-orca`.
- Include the archived ws-dashboard terminal work in this research (user,
  2026-11-10, (538) note).

### Proposals
- Stage the work: headless lead sessions first, then the permission
  boundary, then the terminal, then the desktop shell.
- Allow approval only from loopback or a desktop-shell origin, never on the
  LAN token alone.
- Keep CLI-created terminal sessions first-class next to daemon-hosted ones,
  because `-p` lacks some TUI features.
- Prefer Tauri for the desktop shell, on bundle size.

### Open Questions
- Port the archived terminal to Go, run it as a Rust sidecar, or rewrite?
- Which TUI-only Claude features must the dashboard re-provide for headless
  leads to be usable day to day?
- What is a "workspace" in the dashboard's model?
- Does the desktop shell replace the browser dashboard, or sit beside it
  (LAN and mobile access)?
- Is Orca still a supported host after this, and what does the ws plugin
  keep for it?

### Rejected Alternatives
- A research ticket limited to headless lead sessions and the permission
  boundary ((538) a). Keeping CLI-created sessions as the only kind
  ((538) c).
