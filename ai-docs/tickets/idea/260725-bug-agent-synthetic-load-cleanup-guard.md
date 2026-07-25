---
title: agent-generated synthetic CPU load must record PIDs at spawn and self-limit
related:
  260725-bug-dashboard-terminal-platform-macos-unsupported: found-during
---

# agent-generated synthetic CPU load must record PIDs at spawn and self-limit

## Background

A review subagent ran a load-sensitivity experiment (in support of the
load-fragility finding captured in
`260725-bug-dashboard-terminal-lifetime-load-fragility`, surfaced during
Phase 2 of `260725-bug-dashboard-terminal-platform-macos-unsupported`) whose
cleanup silently failed and left 70 orphaned infinite busy loops on the
host, driving load average to ~128 and degrading the owner's machine for
over half an hour. The agent reported "cleanup complete" and was unaware of
the leak.

Root cause: the script used a zsh construct that does not do what it looks
like it does:

```zsh
for i in $(seq 1 20); do (while :; do :; done) & done
LOADPIDS=$(jobs -p)
...
kill $LOADPIDS 2>/dev/null
```

In zsh, `$(jobs -p)` runs in a command-substitution subshell that does not
see the parent shell's job table, so `LOADPIDS` was always empty and the
`kill` was a silent no-op. The parent shell then exited normally, reparenting
all 20 spinners to `launchd`. Four runs accumulated 70 orphaned processes.

## Lessons

1. Any agent-generated synthetic load must record PIDs at spawn time (`$!`
   per backgrounded job), not via `$(jobs -p)` in a subshell, and should
   additionally use a self-limiting form such as `timeout` or a bounded loop
   so an escaped process dies on its own even if the cleanup step fails.
2. An agent's post-run hygiene check must verify the load actually stopped
   (e.g. re-check load average or process count after the kill), not merely
   that the kill command ran without error.

## Phases

### Phase 1: Add a load-experiment safety pattern to agent guidance

Identify where agent-facing guidance for running synthetic load experiments
lives (or should live) and add the two lessons above as an explicit
pattern/anti-pattern: bounded/self-limiting spawns with PIDs captured via
`$!`, plus a post-run verification step that checks actual load/process
state rather than trusting the kill command's exit status.
