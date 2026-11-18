# Fix: Werkschaft on Windows upgrades a running daemon of an older patch

## Context

The owner installed Werkschaft v0.50.4 on Windows and it self-updated to
v0.50.5. The running shared daemon was 0.50.2-dev, and the app did not
replace it. A read-only survey on develop `8a376df5f` (paths under
`agents-plugin-tool/desktop-electron/`) found the cause:

- `daemon-controller.js:97`:
  `if (platform === 'win32' && !replace && sameMinorCandidate(...)) return running.url;`
  On Windows, any running daemon in the same minor is attached as-is, even
  when the bundled daemon is a newer patch. The newer-core rule on line 98
  is never reached. `sameMinorCandidate` is at about lines 182-185.
- Commit `d3f616e4c` added that line. Its R1 review objected to an
  automatic patch upgrade on ordinary Windows startup. No ticket decision
  authorizes the carve-out, and macOS already upgrades automatically.
- Ticket decision (1197)=b in
  `ai-docs/tickets/ready/261115-feat-werkschaft-electron-daemon-override.md`
  says:
  - the numerically newer core wins;
  - an older bundle attaches;
  - an equal core keeps the running daemon, and only an explicit replace
    swaps it.
- The Go side (`internal/wsdashboard/ensure.go` keepRunning, about 216-223)
  already replaces an older core even without Replace. The JS line is the
  only thing that blocks it.
- The menu item "Replace daemon (same core)" (`main.js:499`, `connect(true)`)
  is today's only replace path on Windows. It stages the bundled daemon under
  userData and runs `daemon-start --require-independent-lifetime --replace`.

The owner decided this as (1343)=a on 2026-11-18 at 09:15: "always upgrade to
the newest version unless there is a special reason".

## Outcome

1. **Windows follows (1197)=b like macOS.** On ordinary connect or startup,
   when the bundled daemon's numeric core is newer than the running daemon's
   within the same minor, the app replaces it.
   - Use the same Windows-safe path the explicit replace uses: staging under
     userData and `--require-independent-lifetime`. Do not invent a new spawn
     path.
2. **These stay as they are** (the owner's "special reasons"):
   - Equal core, including a running `-dev` build of the same core: attach,
     no replace.
   - Older bundled core: attach.
   - Cross-minor: refused, as today, with its existing dialog.
   - The explicit menu replace behaves as today.
3. **Tests.** Update or replace whatever test pins the Windows same-minor
   attach (search `desktop-electron/test/`). Cover on win32:
   - newer patch, so it replaces;
   - equal core with `-dev`, so it attaches;
   - older bundle, so it attaches;
   - cross-minor, so it is refused.
   Keep the macOS cases green.
4. **Optional, only if trivial.** A one-line log entry in the existing shell
   log when an automatic upgrade replaces the daemon. Skip it if it needs new
   plumbing.

## Constraints

- **Scope:** `desktop-electron/daemon-controller.js` and its tests, plus
  `main.js` only if the replace path needs a hook. No Go change is expected.
  If one turns out to be needed, stop and report before making it.
- **Read first:**
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`: this is behaviour only,
    expected tier T0, with no wire or store change; state the tier in the
    commit.
- **Tests:** Node tests for desktop-electron may run locally (e.g. `node
  --test` from `agents-plugin-tool/desktop-electron`; check its package.json
  for the real command).
- **No local runs of anything else on this PC:** no Electron app, `go test`,
  Go tools, ws/ws-mcp exes, launcher or daemon (antivirus rule). Go is
  not expected here. If you need it, only use
  `D:/sys_lock.sh remote w5090 run win-daemon-upgrade 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/degrease-unadvised-nutcase' -- '<cmd>'`
  (Linux, Go at `$HOME/sdk/go/bin`, no Node).
- **Temp files** go only under `D:/Temp/win-daemon-upgrade/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit:** Commit on `impl/develop/win-daemon-upgrade` with your own model's
  Co-Authored-By trailer. Do not push.

## Report

- what changed (file:line);
- the decision matrix before → after;
- the tests and their results;
- the commit;
- anything left alone.
