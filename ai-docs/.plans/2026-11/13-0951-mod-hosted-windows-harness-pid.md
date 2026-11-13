# Claude mod: hosted-mode detection skips the Windows shell wrapper

## Problem

On Windows, a dashboard-hosted Claude session shows every reply and every
user message twice on the dashboard page. The session's ring holds two
identical `reply` entries ~2 s apart, while the Claude transcript JSONL holds
the text once.

Cause (lead diagnosis, 2026-11-13): the Claude mod decides hosted mode in
`agents-plugin/hooks/claude-mod/hosted.ts` (`resolveHostedMode`) by running
`HARNESS_PID_ARGV` (`python3 -c 'import os; print(os.getppid())'`) through
`$.process.run` (`register.ts` ~line 807) and comparing the printed PID with
the hosted row's `host_pid` (claude.exe). On Windows that host command does not
run as a direct child of claude.exe (a shell such as `cmd.exe` sits between),
so the verdict is `other` and the mod silently runs as an ordinary session: it
sends its own dashboard feed through `dashboard_intake` and runs the mailbox
wait / board-inbox drain. The daemon's hosted runner already feeds the same
row from the transcript, so every entry lands twice.

Until 558807689 this was hidden: ws-mcp itself also failed hosted detection
(same cmd.exe wrapper) and registered a separate row, so the mod's feed went
to that duplicate row. 558807689 fixed ws-mcp (`wrappedHarnessPID` in
`agents-plugin-tool/internal/mcp/hosted_login.go`: on Windows, a `cmd.exe`
PID becomes its parent, exactly one level, lookup failure keeps the PID), so
ws-mcp now routes the mod's feed into the hosted row.

## Outcome

- First establish, on this Windows host, the actual process chain between
  claude.exe and the python process that `$.process.run(HARNESS_PID_ARGV)`
  starts (which image is the python's parent: cmd.exe, bash.exe, a py/python
  launcher shim, ...). Record the evidence in the commit's `## AI Context`.
  If the chain cannot be determined from outside the mod, say so and pick
  the rule from the evidence you can get (e.g. Claude Code's process-run
  implementation), stating the uncertainty.
- Make the mod's hosted check agree with ws-mcp's rule: on Windows, a
  wrapper process directly between the harness and the probe is skipped, so
  the printed PID is the harness's. Keep it to the wrapper(s) actually
  observed, one level each, no general ancestor search (a second harness
  started from the hosted session's shell must still resolve `other`). Any
  lookup failure keeps today's behaviour. Non-Windows output unchanged.
- The probe may stay a stdlib-only python one-liner/short script (ctypes
  Toolhelp like `windows_process_entry` in 280551edb's launcher diff) or any
  equivalent that adds no dependency; keep the `parsePid` contract (one PID
  on stdout).
- `agents-plugin/hooks/claude-mod/` and its mirror
  `agents-plugin-wsflow/hooks/claude-mod/` stay byte-identical (see
  `ai-docs/manuals/wsflow-mirroring.md`).

## Out of scope

- The Pi extension (`agents-plugin-pi/src/hosted-session.ts`); note in the
  report whether it has the same wrapper problem, do not change it.
- ws-mcp / Go runtime and the launcher.
- Server-side de-duplication of feeds.

## Verification

- Mod unit tests (the existing `agents-plugin/hooks/claude-mod/*.test.ts`
  runner) pass, with a new test for the Windows wrapper skip (process data
  injected) and for the non-Windows path.
- Run the new probe on this Windows host from a `cmd.exe /c` (and, if
  observed, the other wrapper) child and show it prints the wrapper's parent.
- Do not restart the dashboard daemon or any Claude session; the lead does
  the live check.
