---
title: Windows mailbox-wait ws-mcp processes outlive the Pi host
related:
  260724-bug-windows-mcp-mid-session-disconnect: prior art (serve parent-death watch shipped; launcher Job Object deferred)
  260920-bug-pi-tool-call-timeout-and-process-cleanup: same Windows "only the immediate process dies" theme
  260925-bug-windows-remote-timeout-leaves-child-processes: same theme (git/ssh children)
  260928-bug-pi-exec-windows-timeout-orphans-git-bash-child: same theme (exec children)
  260917-bug-windows-smoke-servestdio-exit-timing-flake: ServeStdio EOF exit waits on in-flight calls
---

# Windows mailbox-wait ws-mcp processes outlive the Pi host

## Background

On native Windows, `ws-mcp.exe` processes remain after the Pi host exits. The
user inspected the leftovers: every orphan was a `mailbox wait` process
(2026-10-08 report). The fix direction is deliberately not decided yet; this
ticket records the evidence and candidate directions for a later discussion.

### Evidence (read from code at ws 0.46.34; no live Windows probe yet)

- The launcher does not exec on Windows. `bin/ws-mcp-launcher.py:900` runs
  `subprocess.call(args)`, so python.exe stays as the blocking parent of
  ws-mcp.exe. POSIX uses `os.execvpe` and has no extra layer.
- The owner lead's mailbox waiter (`agents-plugin-pi/src/mailbox-waiter.ts`)
  spawns `python3 <launcher> mailbox wait ... --timeout 10m` for
  release-backed sessions, or a staged `.mailbox-*/ws-mcp.exe` directly for
  local source sessions. Stdio is `["ignore","ignore","pipe"]`.
- `stop()` sends `child.kill("SIGTERM")`. On Windows that is TerminateProcess of
  python.exe only; the `ws-mcp.exe mailbox wait` grandchild keeps running. The
  code already acknowledges the grandchild (`mailbox-waiter.ts:456-460` settles
  on python's `exit` because the inherited pipe keeps `close` pending).
- `mailbox wait` arms no parent watch (only `serve` calls
  `startParentDeathWatch`, `cmd/ws-mcp/main.go:92`) and reads no stdin. It exits
  only on mail, on its own timeout, or on a real SIGINT/SIGTERM
  (`cmd/ws-mcp/mailbox.go`), none of which TerminateProcess delivers.
- Consequences:
  - Every stop, re-arm, `/reload`, `/resume`, or normal quit can leak one
    ws-mcp.exe for up to 10 minutes.
  - TerminateProcess skips `clearMarker`, so a stale listening marker remains.
  - If the host node.exe is killed, no `stop()` runs at all, so the wait
    survives on both the launcher path and the staged-direct path. The staged
    copy stays locked and its `.mailbox-*` directory is not cleaned up.
- No Job Object, `detached`, `windowsHide`, or `taskkill /T` exists on these
  spawn paths.

### Adjacent paths (lower likelihood; not observed in the user's report)

- Bridge `serve --stdio` after host death: python survives node's death, so the
  serve parent watch (which watches python) never fires. Only stdin EOF
  remains, and `ServeStdio` then waits on `wg.Wait()` for in-flight handlers
  whose contexts derive from `context.Background()`
  (`internal/mcp/server.go:253`). A hung git hook or credential prompt can
  therefore keep it alive indefinitely. The EOF exit is not logged.
- Child Pi agents: the lead's `RpcClient.stop()` terminates only the child
  node.exe on Windows, so the child's own ws-mcp falls back to the EOF path.
  On lead death, the child's `runtimeHost.dispose()` has no timeout.
- Window close usually reaches every process through the shared console
  (CTRL_CLOSE_EVENT). Terminals that do not signal the console (for example
  mintty without ConPTY) fall back to the paths above.

## Candidate directions (unconfirmed; decide in discussion)

1. A host-PID watchdog in Go: the TS side passes its own PID (for example via
   an env var, which is skew-safe against an older runtime), and the Windows
   parent watch watches both the direct parent and the host. Arm it for
   `mailbox wait` as well as `serve`; for `mailbox wait`, cancel the wait
   context so the marker is cleared.
2. Run the mailbox wait through the installed runtime binary directly on
   release sessions too, so `kill()` reaches the waiting process.
3. Bound `ServeStdio` after EOF: cancel in-flight request contexts, put a grace
   timeout on `wg.Wait`, and record an EOF lifecycle event.
4. A launcher Job Object with kill-on-close as a backstop. It covers launcher
   death only, not host death. The 260724 objection (mercenary workers) no
   longer applies.

Do not add `windowsHide: true` without direction 1: children would get their
own hidden console and stop receiving CTRL_CLOSE_EVENT.

## Open probes (native Windows)

- After quitting or killing Pi, list ws-mcp.exe with command line and parent
  PID (`Get-CimInstance Win32_Process -Filter "Name='ws-mcp.exe'"`). Confirm
  that the orphans disappear after about 10 minutes.
- Kill only node.exe (`taskkill /PID <node> /F`, no `/T`) and check whether
  serve exits on EOF.
- Check what `python3` resolves to (Store alias, shim, or real exe).
- Check whether `<cache-root>/crash/mcp-lifecycle.log` ever records
  `process.parent_exited`.
