---
title: "ws/exec tools: daemon-owned command execution with async conversion, completion push, and native process-tool removal"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-single-session-holder: prerequisite
  261112-feat-pi-subagent-long-shell-auto-background: "origin of the exec concept (owner, 2026-11-13); not edited from here"
  261114-feat-ws-conversation-audit-event-store: "future consumer of daemon-visible exec I/O"
  261115-feat-ws-permission-profile: "permission-profile caps are enforced here"
  261117-feat-ws-harness-capability-catalog: "default tool removal range and native subagent removability items"
  261117-feat-ws-agent-tools-mailbox-routing: "shares the never-auto-terminate and cascading-close rules for child agents"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch constraint; not edited from here"
---

# ws/exec tools

## Background

Origin: the owner's exec concept of 2026-11-13, recorded in
`261112-feat-pi-subagent-long-shell-auto-background`. Hosted leads run commands
through ws instead of the harness's own shell tools, so the daemon owns approval,
background conversion, completion push and cleanup. Pi prototype for reference:
`agents-plugin-pi/src/execute-gateway.ts` (search `do-i-really-have-to-run-this-myself`:
30 s window, 4 KB, kills on timeout; `background:true` makes a task with a steer
push) and `agents-plugin-pi/src/background-tasks.ts` (tasks die at
`session_shutdown`). The daemon already has `internal/execjob` and 8 `exec.*` MCP
tools; this ticket generalizes them. Prerequisite: the single session holder
(`261117-feat-ws-single-session-holder`, (17)=a).

## Decisions

1. **Surface (16), (18).**
   - `exec.run(shell, args[], cwd?, exe, async?)`: with `async` it returns
     immediately with `exec_<three words>`; otherwise it waits synchronously up to
     30 s (a config key, the same for leads and children), then converts to async
     and returns "converted to async, exec id" without starting a new turn. Job
     completion pushes a system mail that starts a new turn. No `wait`.
   - `exec.stop` (process-tree kill; Windows `taskkill /T`), `exec.tail` (last
     output slice), `exec.list` (the session's running and recent jobs).
2. **Runs under the single holder (17)=a**, surviving daemon replacement.
3. **Built on `internal/execjob`:** the foreground window changes from 5 s to the
   30 s config key, polling becomes completion push, per-job files become the
   holder memory ring (deltas only for restore). The existing 8 `exec.*` MCP tools
   stay as aliases (T0).
4. **Approval is daemon-owned (18).** ws command rules decide; otherwise the
   existing dashboard approval UI asks. Permission-profile caps are enforced
   here. Owner: approval responsibility moving to ws is easier to handle.
5. **Claude permission pipeline.** Allowlist `ws/exec.*` so approval is not
   prompted twice. Whether Claude raises `can_use_tool` for MCP tools is
   unverified; verifying it is a Phase 1 item.
6. **Native tool removal (18).** Remove only tools that run processes or spawn
   agents: Bash, PowerShell, Agent/Task, and the harness's background shell/monitor
   tools. Keep Read/Write/Edit/Grep/Glob and web tools. Hosted leads block those
   with `--disallowedTools` only, keeping the rest of the user's settings (today
   the Claude lead profile loads user settings unchanged, so Bash cannot be
   removed).
7. **Never auto-terminate (18).** A session with any live child agent or exec job
   (monitors included) is never idle-terminated, sleep-parked or
   child-auto-closed. Feed daemon-tracked jobs into `idleNow`
   (`wsagent/runner_events.go`, search `idleNow`), which today counts turn,
   `bgTasks` and children but not daemon exec jobs.
8. **Explicit close cascades (18).** A user or parent closing a session ends its
   jobs and children deepest-first.
9. **Tool I/O.** The daemon sees exec I/O directly (a future source for the audit
   store); storage follows the release-gate rules (memory ring, restore-only
   deltas).

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/` text is touched, also read
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC.
- Blocking native process tools in hosted leads is an observable behavior change,
  already owner-confirmed by (18).
- The Claude CLI tier is frozen and keeps its native tools.

## Compatibility

New `exec.*` tool shapes are additive (T0); the 8 old `exec.*` tools stay as
aliases (T0). Holder protocol compatibility is owned by the holder ticket. The
concrete verdict cites source lines for which version writes and reads what, per
`ai-docs/manuals/release-compatibility.md`, recorded in Phase 1's Result.

## Phases

### Phase 1: exec.run/stop/tail/list over the holder, push completion, approval

Implement the four tools on top of `internal/execjob` and the single holder with
the 30 s config-key window, async conversion, completion system mail, process-tree
kill, holder-memory output, and the old 8 tools as aliases; daemon-owned approval
via ws command rules then the dashboard UI, with permission-profile caps; verify
whether Claude raises `can_use_tool` for MCP tools and allowlist `ws/exec.*`
accordingly.

Verification: Go tests on remote w5090 for sync, async, convert-to-async without a
new turn, completion push starting a turn, stop killing the process tree
(including Windows `taskkill /T`), tail, list, alias parity, approval paths and
cap enforcement; a recorded answer on `can_use_tool` for MCP tools; syscalls/s
showing no per-job files.

### Phase 2: Native process-tool removal and never-auto-terminate

Block Bash, PowerShell, Agent/Task and harness background shell/monitor tools in
hosted leads with `--disallowedTools` only; feed daemon exec jobs into `idleNow`
so live jobs or children prevent idle termination, sleep-parking and child
auto-close; cascade explicit close deepest-first.

Verification: Go tests on remote w5090 for the flag set and the preserved tools,
idle behavior with a live job and with a live child, and cascading close order; a
recorded hosted-lead init stream showing the tool set.
