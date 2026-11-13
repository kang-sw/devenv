---
title: "Mailbox wait without launching a new process on every wake"
completed: 2026-11-13
related:
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "sibling from the same CPU probe (board (288) b)"
  261112-feat-ws-mailbox-undelivered-notice: "its notice rides whichever wait is armed; no shared decision"
---

# Mailbox wait without a process relaunch per wake

## Background

The 2026-11-11 Windows CPU probe (board topic `cpu-windows`) found that each
armed `mailbox wait` starts a new launcher process on every wake and on
every 10-minute timeout. Each start costs 0.3-0.9 s of CPU and about
30 MB; 13 such waits were live during the probe. Unlike the daemon's own
idle CPU (`261111-bug-ws-dashboard-daemon-idle-cpu-windows`), the cause is
known.

User decision (board (288) b): reduce this cost in its own ticket.

Evidence, per host (source read 2026-11-13):

- The wait itself is already daemon-hosted: the launcher proxies
  `mailbox wait` to the dashboard daemon, which runs it in-process on the
  wake hub (`agents-plugin-tool/cmd/ws-mcp/proxy.go:302`,
  `cmd/ws-mcp/hosted_cli.go:133-140`, `internal/wsmailbox/wait.go:245`).
  What is relaunched is only the client side: `python3 ws-mcp-launcher.py
  mailbox wait ...`, one process per wait.
- One wait covers one wake: it peeks, returns on arrival or `--timeout`
  (`cmd/ws-mcp/mailbox.go:55-65,194-202`; re-arm nudge at `:298-302`).
- Claude Code with the mod (push bound): the mod's watcher loops the
  one-shot wait, `$.process.spawn({argv: waitArgv(...)})`
  (`agents-plugin/hooks/claude-mod/register.ts:277-283,914-917`) with
  `WAIT_TIMEOUT = '10m'` (`register.ts:165`); it respawns after every mail,
  timeout, and board-flag change (`claude-mod/watcher.ts:6-12,496-500`).
  This is the probed cost.
- Pi: same shape, `spawn("python3", buildMailboxWaitArgv(...))` per wait,
  default timeout `10m` (`agents-plugin-pi/src/mailbox-waiter.ts:747,841-845`).
- Claude without the mod binding, and Codex: the model launches the wait as
  a background command and its completion is the wake, so one relaunch per
  wake is intrinsic (`agents-plugin/rsrc/lead-use-mailbox/arm-the-wait.md:7-12`,
  `arm-the-wait.claude.md:10-14`). Codex also has the `Stop` hook,
  `mailbox codex-stop-hook` (`cmd/ws-mcp/mailbox.go:43,411-434`), which runs
  per turn end, not per wake.
- Dashboard-hosted lead sessions: no wait at all; the daemon watches the
  key's queues in-process and submits mail as a user turn
  (`internal/mcp/lead_mail.go:77-140`, `cmd/ws-mcp/dashboard.go:267`).

## Proposed Approach

Give the adapters that own the loop (Claude mod, Pi) one long-lived waiter
per session instead of one process per wake: a streaming mode of
`mailbox wait` (for example `--stream`) that stays attached to the daemon
wait, prints one JSON line per arrival (`kinds` as today), and never exits
on timeout. The adapter reads lines and keeps the existing peek-then-recv
logic unchanged. Windows orphan safety reuses the parent-death watch
(`cmd/ws-mcp/parent_watch_windows.go`). Model-armed waits keep the
one-shot contract and playbook text.

Compatibility tier: T0. A new optional flag on the CLI; one-shot behavior
and playbook content are unchanged. Skew: a new adapter against an older
daemon that rejects the flag falls back to the one-shot loop (see `## Decisions`).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, and `ai-docs/manuals/skill-authoring.md` /
  `ai-docs/manuals/wsflow-mirroring.md` if the mailbox playbook text
  changes; `ai-docs/manuals/release-compatibility.md` for the flag (T0).

## Phases

### Phase 1: streaming wait mode in the CLI

Add the stream mode to `mailbox wait`: attached to the daemon wait, one line
per arrival, listening marker held for the process life, ended by signal or
parent death. Tests at the `wsmailbox.Wait` / `runMailboxWait` level.

### Result (a10df27ef) - 2026-11-13

- Added `--stream`: immediate JSONL arrivals, no empty-timeout output or exit,
  and one listening marker held until cancellation/disconnection. Unchanged
  pending arrivals are retried at the timeout interval (default 10 minutes),
  while new pending items wake immediately without a hot loop.
- Added live stdout CLI frames and synchronized all three Python launchers.
  Windows parent-death handling now covers an already-exited parent; POSIX
  reparenting also closes the connection, including with ignored stdin.
  One-shot waits and model-facing playbooks are unchanged.
- Verification: the remote w5090 full Go suite passed with only
  `TestHostedConnectionSeeingALANChangeReplacesTheDaemon` excluded, as allowed.
  The unfiltered run failed only that known timeout. Focused streaming,
  cancellation, launcher/SIGTERM, and added POSIX-parent-death tests passed.

### Phase 2: adapters use the stream

Claude mod watcher and Pi waiter consume the stream; a board-flag change
restarts it; fall back to one-shot on an unsupported-flag error. Measure
per-wake CPU and memory before and after on Windows.

### Result (0b5eada71) - 2026-11-13

- Claude mod and Pi retain one streaming child across wakes and silent timeout
  windows. Board-mode changes restart it; cancellation, rebinding, and shutdown
  dispose it even between arms. Recv remains the authoritative drain.
- Unknown-flag/usage rejection, or a nonzero exit before any arrival, selects
  the existing one-shot loop for the remaining session. Pi shares that choice
  across key rebinds. An intentional abort does not downgrade; a crash after
  arrivals remains an error/backoff/retry. Arrival bodies are not interpreted
  as usage diagnostics. The Claude mod mirror remains byte-identical.
- Verification: Pi's focused mailbox checks pass 110/110, including real-child
  reuse, lifecycle cleanup, and skew fallback. Claude's controller/stream and
  engine-backed streaming checks pass 138/138 in each package. Both plugin
  validations pass; wsflow Python passes 24 tests (one skipped), and all three
  new launcher tests pass. Broad Windows failures are recorded below.
- One quick Windows sample under the local lock, three wakes against the same
  fake daemon to isolate launcher cost: one-shot CPU averaged **0.250 s/wake**;
  stream incremental CPU was **0.000 s/wake measured** (below process-timer
  resolution), excluding its once-only **0.28125 s** startup. Working set was
  **34.45 MiB** before versus **34.33 MiB** steady after. Launches fell from
  three to one. This is a launcher-only local sample, not a daemon benchmark.

## Decisions

Compatibility tier: T0 (a new optional CLI flag; one-shot behavior and playbook
content unchanged), per `ai-docs/manuals/release-compatibility.md`.

- A long-lived streaming `mailbox wait` process per session, because it removes
  the launcher start on every mail and every timeout, which is the whole
  measured cost; the daemon already hosts the wait itself. Rejected: lengthening
  the timeout alone (still one start per wake); an MCP-tool blocking wait (the
  wait is deliberately a CLI, `cmd/ws-mcp/mailbox.go:29`, and long blocking tool
  calls stress host call timeouts).
- Scope is the Claude mod and Pi adapters; model-armed waits (Claude without
  push, Codex) and hosted leads stay unchanged. The model-armed wait's
  completion is the wake (`arm-the-wait.md:10-12`), so a relaunch is inherent,
  and hosted leads already run none (`lead_mail.go:77`). Rejected: a cross-host
  rewrite.
- A board-flag change restarts the stream through the existing re-arm path
  (`watcher.ts:496-500`); flag changes are rare (knob toggle, turn boundary)
  against a per-wake start. Rejected: a stdin control channel.
- The adapter falls back to the one-shot loop when the stream flag is rejected,
  because an older same-minor daemon may win the cache root
  (`release-compatibility.md`, scenarios 1-2) and the fallback keeps the change
  T0. Rejected: failing the session.

## Forward

- Broad Windows validation has pre-existing failures, reproduced once at base
  `c3f4ae6c4`: Claude's band/hosted assertions and variable 5-second engine
  timeouts (base 662 pass / 114 fail; implementation 665 / 120, wsflow 699 /
  86). Every implementation non-timeout assertion failure is also present at
  base. Python has the same 8 failures and 22 errors at base: POSIX fake
  executables on Windows, cp949 fixture decoding, mocked platform probes,
  and path spelling. The archive-only base run adds one non-Git setup error.
  These are not expanded into mailbox-stream repairs.
- The full Pi check was attempted once and stalled after readiness failures in
  `agent-channel-launch.test.ts`. Bounding stuck workers let it report 2448
  tests: 2183 passed, 222 failed, 18 cancelled, 25 skipped. A targeted base run
  reproduced `fork readiness rejected (session escaped owned home)` on resume
  (one pass, one failure). Other broad failures are not individually
  base-proven. Remaining-file diagnostics hit deadlines/forced terminations
  and the lock cap (87/92 file outcomes, only 13 exit-zero); those results are
  incomplete/inconclusive, not claimed green. The mailbox checks themselves
  pass 110/110.
- Minor review: notice mode can consume a buffered stream arrival after the
  model's recv already drained the queue, producing an extra empty-recv
  notice. Direct mode still submits only what recv removed; no payload is
  delivered twice.
- Separate dogfood capture:
  `261113-bug-pi-wait-mcp-client-timeout-before-requested-deadline` records a
  300-second `pi.wait` cut off by the MCP client's 60-second timeout.

## Resolution

Both phases are implemented and committed. `mailbox wait --stream` and its
adapter use are additive T0 changes; model-armed one-shot waits, their
playbooks, and daemon-hosted lead delivery are unchanged. Runtime and focused
adapter checks pass; broad Windows validation limitations and the minor
notice-mode follow-up remain recorded in Forward.
