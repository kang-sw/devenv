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
the child died with the parent or with the swap was first unverified.
Timeline from the worker's JSON log (UTC): the Pi plugin's `ws-agent-spawn`
started child `test-callers` at 00:07:16; MAIN's daemon swaps landed at about
00:07 and 00:08-00:09:07; the parent's `ws-agent-send` to that child still
succeeded at 00:09:54, after both swaps; the parent's final text followed at
00:09:54 and the process exited. INFERRED: the child was cut by the parent
print-mode process exiting, not by a daemon swap (the `ws-agent-*` children
belong to the Pi plugin's sidecar, `agents-plugin-pi/src/agent-sidecar.ts`). A print-mode
caller therefore cannot rely on a worker that waits on children
asynchronously. Workaround in use: instruct the worker to block on
`pi.wait` before ending a turn, and keep a session file (no `--no-session`)
so the run can be resumed.

## Addendum - 2026-11-15: print mode blocks forever on a never-closing stdin

Two `pi -p --mode json` ticket workers launched from a Claude lead's
background Bash call produced a 0-byte log and no session file for 12 and
34 minutes, idling at 0% CPU. `lsof` on the live process showed fd 0 as a
unix socket the harness keeps open: print mode reads piped stdin when stdin
is not a TTY and waits for an EOF that never comes, before it creates the
session. Relaunching with `< /dev/null` started the session immediately.
Foreground runs (where stdin reached EOF) were unaffected, which is why
read-only investigations worked and long background workers did not.

- Open question: should print mode skip reading stdin when a message
  argument is given, or time out with a diagnostic instead of hanging
  silently? Whatever guidance tells a non-Pi lead how to spawn a Pi worker
  should close stdin either way.

## Addendum (2026-11-15): `ws-compact` aborts a print-mode run

A headless `pi -p --mode json` design reviewer called the `ws-compact` tool
mid-review. The tool answered "Compaction requested", the agent turn was
aborted ("This operation was aborted", `agent_settled` with
`aborted: true`, then `compaction_start` reason `manual`), and the process
exited with no final message. Workaround in use: exclude `ws-compact` from
headless runs (`--exclude-tools`).

