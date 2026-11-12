---
title: "Hosted sessions: a lead turn names the subagents a process end killed"
related:
  261112-bug-ws-hosted-finished-subagents-lost-on-idle-resume: split from it (user board (13)=a)
  261110-feat-ws-dashboard-headless-lead-sessions: hosted Claude lead sessions
---

# Hosted sessions: a lead turn names the subagents a process end killed

## Background

When a hosted lead's process ends while background subagents run, the
dashboard marks them failed (`wsagent/runner.go` `processEnded` ->
`liveWriter.endAgents`), but the lead model is never told. Its next turn
starts on a resumed backend session that still expects those subagents'
task notifications, which never come.

Paths that end a process with subagents running: `Runner.Restart` (the MCP
recovery restart), the `applySettings` restart fallback, `checkInterrupt`'s
`stopProcess` when an interrupt fails, and a process crash. The idle stop does
not (`idleNow` requires `bgTasks == 0`), and a daemon restart rejoins the
holder, so its subagents keep running.

The user prefers the Claude Code harness behaviour: at restart, a turn tells
the model which agents died (user board (11); split confirmed in (13)=a).

## Open Decision Queue

Last ID: (24)

(22) [open] [announced] When `processEnded` marks at least one running subagent failed and the conversation stays open, the daemon delivers one lead turn at the next process start naming each killed subagent (id, description, subagent type, how long it ran) and saying its result is lost and it may be re-dispatched. Citation: user board (11) and (13)=a.

(23) [open] [technical] The pending notice is a new optional record field (`Notices`), persisted and delivered as a turn like `Record.Mail`, over reusing `Record.Mail` - mail renders on the page as a mailbox message and would misattribute the notice. T0: additive optional field.

(24) [open] [policy] After a crash that starts no new process, the notice waits for the next process start (the next user message or mail), with no extra wake.
- Context: `Restart` and the settings fallback start the next process at once, so the notice goes out immediately there. A crash leaves the conversation idle.
- Alternative a: wait for the next start, the notice leading that turn.
- Alternative b: start a process just to deliver the notice.
> a: starting a process costs a turn nobody asked for, and a crash loop would repeat it; the dashboard already shows the failed rows meanwhile.

## Constraints

- Before adding the notice, check whether Claude Code itself already tells the
  model about background tasks lost across `--resume`; if it does, report it
  and do not duplicate it.
- Compatibility tier: T0 (additive optional record field).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Heavy Go builds and tests run through `~/.local/bin/ws-rgo` on this machine.
