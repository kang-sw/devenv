---
title: "Dashboard agent host: daemon-spawned headless lead sessions with approval, mail injection, recovery, and idle stop"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-session-holder
related:
  261110-feat-ws-hosted-session-mode-flag: prerequisite
  261110-research-ws-dashboard-agent-host-foundations: "remaining verifications: -p feature coverage, Pi parity, ws hooks under -p"
  261110-research-ws-dashboard-background-task-visibility: "decides whether idle stop can see background tasks (Decision 6)"
  261108-feat-ws-tracker-support-agent-chat: "the wsagent runner this generalizes"
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope 241 b (no browser approval) is reopened under Decision 3"
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
   flag (`261110-feat-ws-hosted-session-mode-flag`) so the mod or extension
   does not also deliver them. The file-inbox wait stays for CLI sessions
   only.
3. **Approval through the SDK host, answered only from a trusted origin
   (announced, (13); trust boundary (05)).** Permission prompts reach the
   daemon as the stream-json host (`--permission-prompts host`, the default
   in `claude` 2.1.322 `--help`), and the dashboard shows them. Answers are
   accepted only from loopback or a desktop-shell origin, never on the LAN
   token alone: the same trust boundary as the dashboard terminal. This
   reopens `261106-epic-ws-dashboard-serve-daemon` Non-Scope 241 b under
   that condition.
4. **Permission mode chosen at spawn, default `auto`; unanswered prompts
   wait (user, (14) a).** A prompt waits for an answer with no timeout; the
   session shows as awaiting approval and a notification goes out.
   Rejected: auto-deny after a timeout ((14) b), because in a very long
   session denials pile up while the user is away and steer the work wrong;
   no browser approval at all, `--permission-prompts none` with spawn-time
   mode and allowlist only ((14) c).
5. **MCP recovery by restart and resume (technical, (22)).** `/mcp reconnect`
   fails under `-p` (`agents-plugin/hooks/claude-mod/reconnect.ts`
   `commandRefusal`). When a daemon session's MCP connection is broken, the
   daemon restarts its process at an idle point and resumes it through the
   backend session ID.
6. **Idle stop on by default once the daemon owns delivery (technical, (17);
   user, (17) note).** A session with no running turn and no running
   background task stops its process after a configurable idle period; the
   next message or mail resumes it transparently. The user's reason: mail is
   the only reason to keep an idle process alive, and the daemon now
   delivers it. Uncertainty: whether background tasks are visible in the
   stream; `261110-research-ws-dashboard-background-task-visibility` settles
   it. If they are not visible, the default stays off.
   Rejected: always-on processes with idle stop off by default (the first
   form of (17), superseded by the note).

## Prior Art

- `wsagent` runner and backends (Background).
- The trusted-origin gate pattern: `localViewer` (`internal/wsdashboard/access.go`)
  plus `sameOrigin` (`inbox_routes.go`), as the system-open route
  (`sysopen_route.go`) applies them.

## Constraints

- The lead profile is the opposite of the support profile: it loads user
  and project settings, the ws plugin and its hooks, memory, and MCP
  servers. Which `-p` gaps matter (slash commands and skills, manual
  compaction, ws session-start hooks) and Pi parity are the foundations
  research's remaining verifications; settle them before promotion.
- Daemon sessions and CLI sessions coexist in the session registry and the
  messenger.
- To settle at promotion: the spawn UI and its options (harness, model,
  permission mode, workspace binding through
  `261110-feat-ws-dashboard-workspaces`); stop and archive semantics; the
  notification channel for approval waits; the idle period default.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: spawn, deliver, approve

Scope and verification are settled at promotion, after the Constraints'
open items.
