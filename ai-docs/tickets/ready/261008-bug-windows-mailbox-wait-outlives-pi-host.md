---
title: Windows mailbox-wait ws-mcp processes outlive the Pi host
related:
  260724-bug-windows-mcp-mid-session-disconnect: prior art (serve parent-death watch shipped; launcher Job Object deferred)
  260920-bug-pi-tool-call-timeout-and-process-cleanup: same Windows "only the immediate process dies" theme
  260925-bug-windows-remote-timeout-leaves-child-processes: same theme (git/ssh children)
  260928-bug-pi-exec-windows-timeout-orphans-git-bash-child: same theme (exec children)
  260917-bug-windows-smoke-servestdio-exit-timing-flake: ServeStdio EOF exit waits on in-flight calls
  261008-bug-pi-windows-python3-store-stub-blocks-release-ws: found by this ticket's probe; its interpreter choice may add a process layer
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3a872773e87e9c99
sage-review-completeness-reviewed: 3a872773e87e9c99
---

# Windows mailbox-wait ws-mcp processes outlive the Pi host

## Background

On native Windows, `ws-mcp` `mailbox wait` processes remain after the Pi host
stops using them (user report from another PC, 2026-10-08). A live probe on
the Windows smoke host (DESKTOP-3SUVUA9, Pi 1.0.4,
`pi install git:github.com/kang-sw/devenv@v0.46.34`, release launcher path)
reproduced it. Full probe report with raw process tables:
`~/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/484932ec-01-windows-mailbox-wait-probe.md`
on the lead machine (cache artifact, not tracked).

### Mechanism

- The launcher does not exec on Windows: `bin/ws-mcp-launcher.py` runs
  `subprocess.call(args)`, so the chain is `node -> python3 (launcher) ->
  ws-mcp` for both `serve --stdio` and `mailbox wait`. POSIX uses
  `os.execvpe` and has no extra layer.
- The Pi mailbox waiter (`agents-plugin-pi/src/mailbox-waiter.ts`) stops the
  wait with `child.kill("SIGTERM")`, which on Windows is TerminateProcess of
  python only.
- `mailbox wait` arms no parent watch (only `serve` calls
  `startParentDeathWatch`, in `agents-plugin-tool/cmd/ws-mcp/main.go`) and
  reads no stdin. It exits only on mail, its own `--timeout`, or a real
  SIGINT/SIGTERM.

### Probe results (2026-10-08)

- `/quit`: serve exits, the wait survives with a dead parent. Confirmed.
- `/reload`: the old wait survives while a new one arms on the same session
  key. Confirmed.
- Hard kill of node only (`taskkill /PID <node> /F`, no `/T`): **both launcher
  python processes die within 2 s** (the earlier premise that python survives
  node's death is refuted; inferred cause: libuv places spawned children in a
  kill-on-close job object that grandchildren silently break away from). Serve
  exits (inferred: stdin EOF). Only the wait grandchild survives.
- So in every observed leak, the wait's **direct parent** (the launcher
  python) has exited.
- Orphans self-exit at their `--timeout` (about 10 minutes for the real
  waiter) and clear their marker on that path.
- **Orphans delete the live waiter's marker.** `ClearListeningMarker`
  (`agents-plugin-tool/internal/wsmailbox/listening.go`) removes
  `<cache-root>/mailbox-listening/<session-key>.json` without checking the
  marker's PID. When the `/reload` orphan timed out, it deleted the marker of
  the live replacement waiter (same session key), which had no marker until it
  re-armed.
- Closing the console (ssh/ConPTY teardown) ended every wait gracefully,
  including earlier orphans. Leaks accumulate while the terminal stays open.
- `mcp-lifecycle.log` recorded `process.parent_exited` only for serve; no
  entry exists for any mailbox wait exit.
- The release runtime image name is `ws-mcp-<version>-<hash>.exe`; probe
  filters must use `Name LIKE 'ws-mcp%'`, not `Name='ws-mcp.exe'`.
- Not reached: `/resume`, child Pi agents, non-ConPTY terminals, the live
  staged-direct (`.mailbox-*`) local-source path.

## Decisions

- Arm the existing Windows parent-death watch for `mailbox wait`, and on
  parent death end the wait gracefully so the marker is cleared, instead of
  self-terminating with `os.Exit` as `serve` does.
  Rejected:
  - a host-PID watchdog (the TS host passes its PID through an env var and
    ws-mcp watches it as well): every observed leak already has a dead direct
    parent, and an extra layer only appears if
    261008-bug-pi-windows-python3-store-stub-blocks-release-ws chooses `py.exe`
    (`py.exe -> python.exe`); that ticket carries the question;
  - spawning the runtime binary directly on release sessions: TerminateProcess
    on every stop would still skip marker cleanup;
  - a launcher Job Object with kill-on-close: covers launcher death only and
    TerminateProcess skips marker cleanup too.
- `ClearListeningMarker` removes the marker only when it belongs to the
  calling process (marker PID equals the caller's PID). This is required by
  the decision above: once orphans exit promptly on parent death, a `/reload`
  orphan would otherwise delete the replacement waiter's marker on every
  reload.
- Bounding `ServeStdio` after stdin EOF (cancel in-flight handlers, grace
  timeout on `wg.Wait`, EOF lifecycle event) is split out of this ticket into
  261008-bug-servestdio-eof-exit-unbounded; the probe observed serve exiting
  on EOF.

- Parent death feeds the wait's existing cancellation context (the
  `signal.NotifyContext` path in `cmd/ws-mcp/mailbox.go`), so the existing
  clear-marker-then-exit branch runs. Rejected: a parent-death callback that
  clears the marker and exits on its own; one exit path keeps marker and
  exit-code handling in one place.
- The owner check is read marker, compare PID, then remove; the residual window
  between read and remove is accepted. Rejected: a cross-process lock or atomic
  compare-and-delete; the observed overlap is minutes long while the window is
  microseconds, and a lock adds a new cross-process primitive.
- Windows-only: the POSIX `startParentDeathWatch` stays a no-op. Rejected: a
  cross-platform parent watch; on POSIX the launcher execs, so `stop()`'s
  SIGTERM reaches the wait directly, and POSIX hard host death was not
  reported and needs a different mechanism.
- The wait records a `process.parent_exited` lifecycle event whose action
  distinguishes the graceful wait cancel from serve's self-terminate.
  Rejected: no event; the probe found no lifecycle trace for any wait exit.
- Verification includes a native re-run on the Windows smoke host
  (`ssh ki608@192.168.33.6`, clone note `infra.windows-smoke-host`) of the
  `/quit`, `/reload`, and node-only-kill scenarios with a build carrying the
  fix, under `ai-docs/manuals/windows-dogfood.md` live-host safety (PID-scoped
  kills only).

## Constraints

- The probe left Pi 1.0.4, the ws v0.46.34 Pi package, and a scratch
  directory `C:\Users\ki608\wsprobe\` (including a `python3.exe` hardlink to
  the real `python.exe`, not on the global PATH) on the smoke host. Release
  ws-on-Pi does not start there without that `python3` workaround, per
  261008-bug-pi-windows-python3-store-stub-blocks-release-ws.
- Do not add `windowsHide: true` to the waiter spawn: children would get their
  own hidden console and stop receiving the console-close event that
  currently cleans up on window close.
- Out of scope: the Pi-side waiter (`mailbox-waiter.ts`) spawn and stop logic,
  the Python launcher, and serve's EOF behavior.
- Host-neutral semantics: the watch arms on every Windows `mailbox wait`,
  including Claude/Codex background waits started through a shell; a wait now
  ends when its direct parent dies. Say so in a code comment and the commit
  AI Context. If the parent dies before the watch opens it, the existing
  already-signaled guard leaves the watch disarmed and `--timeout` remains the
  backstop (accepted in 88a67a78).
- Implementer latitude: the watch shape (a callback-taking variant of
  `startParentDeathWatch`, with the POSIX stub matching its signature and
  serve's self-terminate unchanged), the lifecycle action string, whether the
  owner PID is read inside `ClearListeningMarker` or passed in, and whether
  the Windows test re-executes the test binary as a short-lived helper parent
  or exercises the cancel wiring in-process through a PID-parameterized
  helper.
- `TestListeningMarkerWriteReadClear` (`internal/wsmailbox/listening_test.go`)
  writes a foreign PID and expects the clear to remove it; it must be rewritten
  for the owner check, not deleted.
- The raw probe tables live only in the lead machine's cache; the Background
  summary is the authoritative record.
- Applicable manuals: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/windows-dogfood.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Prior Decisions

- 260724-bug-windows-mcp-mid-session-disconnect (2026-07-24, commit 88a67a78): "Chose server-side parent-death detection (ticket Phase 3 "and/or") over a launcher Job Object: it reuses the existing process_alive_windows.go OpenProcess/WaitForSingleObject pattern ... zero mercenary-breakaway regression risk" — bearing: supports
- 260724-bug-windows-mcp-mid-session-disconnect (2026-07-24, commit 88a67a78): "os.Exit fires only on WAIT_OBJECT_0 (not WAIT_FAILED), so a syscall error leaves the watch disarmed rather than killing a healthy server. PID-reuse race ... is accepted for a backstop." — bearing: constrains
- 260913-feat-cross-session-mailbox-wake (2026-09-13, commit e3c50663): "os.Exit skips deferred functions, so the listening-marker cleanup could not live behind a bare `defer` ...; each exit branch now calls the clear explicitly before its own terminal exit." — bearing: constrains
- 260913-feat-cross-session-mailbox-wake (2026-09-13, Result 588dda97): "Deferred (Minor, accepted risk): stale marker / PID-liveness / two waits sharing a session_key silently overwriting each other's marker is unfixed." — bearing: supports
- 260913-feat-cross-session-mailbox-wake (2026-09-13, commit e3c50663): "the CLI layer (cmd/ws-mcp/mailbox.go) is a thin flag/exit-code/output wrapper over [wsmailbox/wait.go]" — bearing: constrains
- 261008-bug-pi-windows-python3-store-stub-blocks-release-ws (2026-10-08, commit 5f124db1): "the interpreter-resolution choice interacts with the mailbox-wait orphan fix (py.exe adds a process layer), so no direction is chosen here." — bearing: constrains
- 261008-bug-windows-mailbox-wait-outlives-pi-host (2026-10-08, commit f2ae6420): "User asked to author the ticket only and not settle the fix now, so it lands in idea/ with the directions marked unconfirmed rather than as a ready plan." — bearing: supports
- 0c42008d (2026-10-08, commit): "a bridge `onDefaultKeyAdopted` observer re-arms a per-generation waiter slot with the adopted key" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/mailbox.go, parent_watch_windows.go, parent_watch_other.go, agents-plugin-tool/internal/wsmailbox/listening.go, plus tests |
| scope.surface | internal | internal/wsmailbox and cmd/ws-mcp only; no public interface named (ClearListeningMarker callers: mailbox.go and listening_test.go) |
| scope.new_public_symbol | no | none named; owner PID can come from os.Getpid inside ClearListeningMarker |
| scope.new_type_contract | unknown | ClearListeningMarker signature is not fixed by the ticket; Decision 2 may add a PID parameter |
| scope.test_surface | existing | agents-plugin-tool/internal/wsmailbox/listening_test.go, agents-plugin-tool/cmd/ws-mcp/parent_watch_windows_test.go, agents-plugin-tool/cmd/ws-mcp/mailbox_test.go (TestMailboxWaitListeningMarkerLifecycle) |
| complexity.reuse_points | confirmed | watchProcessExit and startParentDeathWatch (parent_watch_windows.go#L20-L58 read), signal.NotifyContext path in mailbox.go |
| complexity.side_effect_risk | moderate | changes the shared marker-clear and exit path of every mailbox wait and the marker that Stop hooks read |
| risk.correctness | moderate | read-compare-remove window and PID reuse accepted by design; parent-death must reach the cancellation context without racing the done branch |
| risk.fit | low | reuses the existing Windows parent watch and clear-marker exit branch; no new mechanism |
| risk.test | high | Windows process semantics cannot run on macOS; acceptance needs the native smoke host scenarios |
| risk.security_or_contract | moderate | changes ClearListeningMarker semantics and the marker's ownership contract read by ReadListeningMarker consumers |

## Phases

### Phase 1: Wait exits on parent death without clobbering a live marker

- Arm the Windows parent-death watch in `mailbox wait`; parent death cancels
  the wait and takes the existing clear-marker exit path.
- Make `ClearListeningMarker` owner-checked by PID.
- Tests:
  - marker clear by a non-owner PID leaves the marker in place; clear by the
    owner removes it; a missing marker stays a no-op;
  - on Windows (build-tagged), a wait whose parent process exits ends
    promptly and clears its own marker (the existing `watchProcessExit` test
    pattern with a real short-lived process);
  - POSIX behavior unchanged.
- Native verification on the smoke host: after `/quit`, after `/reload`, and
  after a node-only kill, no `ws-mcp%` `mailbox wait` process remains beyond a
  few seconds, the live replacement waiter's marker survives a `/reload`, and
  the lifecycle log records the wait's parent-exit event.
- Done when the tests pass, the existing `agents-plugin-tool` suite passes,
  `GOOS=windows go vet ./...` is clean, and the native scenarios above pass.

### Result (2186143ab) - 2026-10-08

- Landed: `startParentDeathWatch(onExit func(ppid int))` (Windows arms the
  existing `watchProcessExit`; POSIX stub matches and never calls). `serve`
  passes its unchanged record-and-`os.Exit(0)` callback. `mailbox wait`
  derives a cancelable context from its `signal.NotifyContext` and passes a
  callback that records `process.parent_exited` (`action: cancel_wait`,
  `command: "mailbox wait"`) and cancels it, so the existing interrupted
  branch clears the marker and exits 130.
- `ClearListeningMarker(sessionKey, ownerPID)` takes the owner PID
  explicitly; the wait passes its marker's PID. A foreign-PID marker is kept
  (success); a missing marker is a no-op; an unparseable marker is kept and
  its parse error returned.
- Tests: `TestListeningMarkerWriteReadClear` rewritten for the owner arg;
  new `TestClearListeningMarkerLeavesAnotherOwnersMarker`,
  `TestClearListeningMarkerLeavesUnparseableMarker`; Windows
  `TestMailboxWait_EndsWhenParentExits` re-executes the test binary as a
  helper parent, kills it, and asserts the real wait exits 130 within 5 s,
  clears its marker, and logs `cancel_wait`.
- Verification: macOS `go test ./...` all ok; `GOOS=windows go vet ./...`
  clean. On the smoke host (go1.26.2): `go test -run
  'TestWatchProcessExit|TestMailboxWait' ./cmd/ws-mcp/` and
  `./internal/wsmailbox/` pass; with the callback's cancel removed the new
  test fails ("still running 5s after its parent exited"). Live Pi 1.0.4
  with the fix build (via `WS_MCP_RUNTIME_DIR` scratch dir +
  `WS_MCP_BOOTSTRAP_BINARY`, hash-matched): `/reload` - old wait gone,
  `cancel_wait` logged, replacement marker kept; `/quit` - no node/python/
  ws-mcp left after 4 s, marker cleared, `cancel_wait` logged; node-only
  `taskkill /PID /F` - all gone within 3 s, marker cleared, `cancel_wait`
  logged. Scratch `wsprobe\mbxfix` removed; host left with no node/python/
  ws-mcp processes and no markers.
- Review (lite): clean. Minor, accepted: an unparseable marker now persists
  until the next arm overwrites it; a transient Windows sharing violation on
  the clear's read would likewise leave the marker until the next arm.
- Not reached: `/resume`, child Pi agents, the staged-direct local-source
  path, and the `/reload` overlap case where the replacement writes before
  the old wait clears (covered by the unit owner-check test; live, the old
  wait cleared first).
