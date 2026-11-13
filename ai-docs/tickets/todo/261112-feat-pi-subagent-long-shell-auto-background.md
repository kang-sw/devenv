---
title: "Pi subagents: a blocking shell command moves to a background shell after 30 seconds"
related:
  260920-bug-pi-tool-call-timeout-and-process-cleanup: neighbouring problem (bounded tool calls and subprocess cleanup); this ticket keeps a long command running instead of bounding it
---

# Pi subagents: a blocking shell command moves to a background shell after 30 seconds

## Background

User feedback (whiteboard, 2026-11-12), recorded as feedback only; nothing
here is a settled decision yet.

In the Pi adapter, a subagent running a long blocking shell command cannot
take a message the lead sends it until the command finishes, so steering a
subagent mid-task often stalls behind a long command. The user asked that
every blocking shell command in a Pi subagent switch to a background shell
after 30 seconds, so the subagent's turn frees up and the message gets
through.

## Owner Concept (2026-11-13)

Not yet settled; captured verbatim in substance for the Open Decision Queue.

- A ws tool, working name `ws/exec.run(shell: bool, args: [], cwd?: string
  = <current dir>, exe: <exec name>, async?: bool = false)`, modelled on the
  background shell the hosted Pi session already uses.
- `async: true` returns at once with a command id `exec_<three words, no
  separator>` (e.g. `exec_shinybrightfuture`, mailbox-id style).
- `async: false` waits up to 30 s. Finishing inside 30 s returns normally.
  Past 30 s the command converts to async and the agent learns "converted to
  async, exec id: exec_..." without a new turn being fired; the agent is then
  free to take messages. When the command later finishes, that completion
  fires a new turn.
- After conversion the agent manages it with sibling commands (`stop`,
  `tail`, ...). Deliberately no `wait`: an agent that wants to wait simply
  stops producing output and is woken by the completion.

## Open Questions

- Where the process lives (daemon vs per-session MCP server) and its
  lifetime across turn end, agent close, and daemon replacement.
- Which harnesses get the tool: on Claude Code and Codex it would bypass the
  host's own command permission/sandbox layer, so it may need to be Pi-only
  or gated by config.
- Shell resolution for `shell: true` on Windows (Git Bash, PowerShell, cmd)
  and output encoding.
- Output retention and the `tail` window; process-tree stop on Windows.

- Whether Pi exposes a background-shell facility the adapter can hand a
  running command to, or the adapter must own the process and return a
  handle (output file, status query) to the subagent.
- How the subagent learns the command finished (a wake, a poll tool).
- Whether the 30-second threshold is configurable, and whether it applies to
  the lead as well as subagents.

## Phases

### Phase 1: Auto-background long shell commands in Pi subagents

To be settled at capture through the Open Decision Queue.
