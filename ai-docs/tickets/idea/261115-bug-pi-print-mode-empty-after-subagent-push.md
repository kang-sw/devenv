---
title: "pi print mode prints nothing when a ws-spawned child settles after the final answer"
---

# pi -p prints nothing when a ws-spawned explorer settles after the final answer

## Background

Dogfood surprise, 2026-11-15 (EXEC4 lead, Pi 1.5.2, devenv ws Pi package
loaded as a user package). A Claude lead ran ticket reviewers as
`pi -p -a --no-session --append-system-prompt <rendered playbook> "<task>"`
(text mode). Two design-reviewer runs that spawned a ws explorer subagent
exited 0 with a single newline on stdout; completeness runs without a child
printed their verdict normally.

Re-running with `--mode json` showed two `agent_start`/`agent_end` pairs: the
reviewer's final assistant text (a full `verdict: pass` report) ended the
first run, then a `ws-push-batch` custom message (`ws-agent-settled` for the
explorer, reason idle) started a second agent run whose final assistant
message carried no text. Text mode prints only that last message, so the
caller loses the real answer. Evidence log: `/tmp/exec4/rev-design-busy2.jsonl`
(machine-local, not tracked).

## Open Questions

- Should a settled-child push arriving after the parent's final answer in
  print mode be suppressed, folded into the same run, or should print mode
  print the last non-empty assistant text?
- Workaround in use: `--mode json` and extract the last non-empty assistant
  `message_end` text.

## Addendum - 2026-11-15: print mode also exits while a child still runs

A `pi -p --mode json` ticket worker (elevated playbook) delegated a Go
test-caller migration to a ws child, ended its own turn with "Waiting for the
mechanical test-caller migration", and the process exited (`agent_settled`,
exit 0) with the server-side work uncommitted. No child process remained in
the worktree afterwards; two daemon swaps happened during the run, so whether
the child died with the parent or with the swap is unverified. A print-mode
caller therefore cannot rely on a worker that waits on children
asynchronously. Workaround in use: instruct the worker to block on
`pi.wait` before ending a turn, and keep a session file (no `--no-session`)
so the run can be resumed.
