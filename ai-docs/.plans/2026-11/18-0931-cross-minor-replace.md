# Fix: Werkschaft replaces an older-minor shared daemon instead of refusing

## Context

Owner request, 2026-11-18 09:27 KST: the Werkschaft desktop app currently
refuses to replace a running shared daemon of an older minor ("for safety").
In real use this is inconvenient. Make it replace across minors too, then
release. The owner first asked (09:29) how older released versions behave
when they meet a newer-minor daemon. A read-only survey of v0.48.17 and v0.49.1
found:
- neither line can kill, replace, downgrade, or ping-pong a newer daemon;
- the old launcher sees a different minor, raises `ProxyFallback`, and runs
  its own direct `serve --stdio` runtime;
- the Go `Ensure` in those versions never shuts down a newer daemon
  (`keepRunning` when the running version is newer);
- `endpoint.json` is written only by the lock-holding daemon.

So replacing across minors costs older-minor plugin sessions a degraded
direct-path session until they update. It does not lock them out or clobber
data. The original design in
`ai-docs/tickets/ready/261115-feat-werkschaft-electron-daemon-override.md`
(about lines 75 and 111) intended a cross-minor swap with confirmation, not a
refusal.

Current blocks (develop `6b91aff4e`):
- `agents-plugin-tool/desktop-electron/daemon-controller.js` about lines
  95-102: after the older/equal-core return, a major.minor mismatch calls
  `notify('Cannot safely replace across minor/protocol boundary ...')` and
  attaches. Related notices sit in `attachWinner` (about lines 192-193) and
  around line 115.
- `main.js` about lines 366-371: the "Daemon replacement unavailable" box with
  the single "Keep connection" button, used by the menu replace.
- Go `agents-plugin-tool/internal/wsdashboard/ensure.go`: `RefuseCrossMinor`
  (about lines 61-62, check at 151-154). It is set only at
  `agents-plugin-tool/cmd/ws-mcp/proxy.go:434` as
  `RefuseCrossMinor: *independent`, which is the
  `--require-independent-lifetime` flag the Windows app passes. So on Windows
  the Go starter refuses too.

## Outcome

1. **Newer core wins across minors, automatically.** On ordinary
   connect/startup (macOS and Windows), when the bundled daemon's numeric
   version is newer than the running daemon's, the app replaces it, whether
   the difference is a patch or a minor. No dialog. Never downgrade: an older
   bundle still attaches. Equal core keeps today's behaviour: attach, and
   replace only through the explicit menu.
2. **Windows path works across minors.** The Go starter must not refuse the
   cross-minor replacement the app asks for. Prefer the simplest change:
   remove `RefuseCrossMinor` (field, check, the `proxy.go:434` wiring, and its
   tests) if nothing else needs it. Otherwise stop setting it from
   `--require-independent-lifetime`. Never-downgrade stays as it is in
   `keepRunning`.
3. **Remove the refusal UI.** Remove the cross-minor refusal notice and the
   "Daemon replacement unavailable" dialog path, or reduce them to whatever
   still has a real trigger.
   - If an existing shell/daemon-start log already records replacements, that
     is enough. Do not add new plumbing.
   - Update the menu label "Replace daemon (same core)" only if it becomes
     misleading.
4. **Check the current launcher.** Verify by reading the current 0.50 plugin
   launcher (`agents-plugin/bin/ws-mcp-launcher.py`, about lines 1764 and
   2190): against a newer-minor daemon it falls back to the direct path, and
   does not loop or fail hard. Report what you find. Change it only if it
   would loop or fail outright; if it does, stop and report first.
5. **Docs.** Update the sentence in `AGENTS.md` `### Compatibility Tiers`
   ("a minor bump currently locks out installed-plugin sessions on the
   previous minor ...") and the matching text in
   `ai-docs/manuals/release-compatibility.md`. They should say that
   previous-minor sessions fall back to the direct (non-daemon-hosted) path
   when a newer-minor daemon replaces theirs. Keep it to one or two sentences
   each.
6. **Tests.**
   - Update `desktop-electron/test/daemon-controller.test.js` (the
     cross-minor cases around lines 74, 98, 154, 263) to a decision table
     covering, on win32 and darwin:
     - newer minor → replace;
     - newer patch → replace;
     - equal core including `-dev` → attach;
     - older bundle (patch or minor) → attach;
     - explicit menu replace with equal core → replace.
   - Update Go tests for the removed or changed `RefuseCrossMinor`
     (`ensure_electron_test.go`, `daemon_start_test.go` or wherever they are).

## Constraints

- **Read first:**
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`. Expected tier T0: behaviour
    only, no wire or store change. State it in the commit, with source-line
    evidence of how older clients behave (the facts above).
- **Scope:** desktop-electron daemon-controller/main and tests; Go
  `ensure.go`, `cmd/ws-mcp/proxy.go` and their tests; the two doc sentences.
  No launcher change without stopping first.
- **No local runs on this PC** (antivirus rule): no Electron app, `go test`,
  Go test exes, gofmt or other Go tools, ws/ws-mcp exes, launcher or daemon.
  - Node tests for desktop-electron may run locally. Check its package.json
    for the command.
  - Go runs only on w5090:
    `D:/sys_lock.sh remote w5090 run cross-minor 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/degrease-unadvised-nutcase' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && gofmt -l ./internal/wsdashboard ./cmd/ws-mcp && go vet ./... && GOOS=windows go vet ./... && go test -count=1 ./internal/wsdashboard ./cmd/ws-mcp'`
  - Redirect that output to a file under `D:/Temp/cross-minor/`, run it with
    run_in_background, and wait for it to finish. Do not end your turn while
    it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Temp files** go only under `D:/Temp/cross-minor/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit:** Commit on `impl/develop/cross-minor-replace` with your own
  model's Co-Authored-By trailer. Do not push.

## Report

- what changed (file:line);
- the decision matrix before → after (both platforms);
- the current launcher's behaviour against a newer-minor daemon;
- the tests and the w5090 result;
- the commit;
- anything left alone.
