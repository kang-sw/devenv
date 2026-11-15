# Windows updater helper empty-argument repair

## Authority and scope

MAIN assigned exactly one task at 2026-11-15 12:01:47Z, due 21:50 KST: repair the known Windows PowerShell helper argument binding bug, verify four non-GUI cases, conditionally perform one real Windows restart-now acceptance, and rebuild the three shell-only desktop release assets. Do not merge. This is a bounded bug fix with no protocol change.

Work only in this acquired pooled checkout on `fix/electron-windows-helper-empty-args`, based on current develop. Read applicable project manuals before editing. Product source edits are limited to `agents-plugin-tool/desktop-electron/update.js` and its existing desktop-electron unit tests. Commit the fix and this plan with `## AI Context`.

## Settled implementation

In `helperCommand`, when the original relaunch command line is empty, omit `-RelaunchArgs` and its value entirely; the PowerShell parameter default `-` covers it. Never send a leading-hyphen value through powershell.exe -File binding. Preserve base64 argument handling for nonempty launch arguments. Add the first helper body line, before Wait-Process, reporting `helper started pid=$PID app=$AppPid`. Update/add focused regressions and run the desktop-electron tests on supported hosts offline.

## Native verification

Use `ssh ki608@192.168.33.6`, a new unique folder per native run, marker-only dummy Target/Staged/Staging folders, no real user app/profile in non-GUI cases, and no relaunch for marker tests. Record commands, parent/helper PID+creation identity, actual exit codes (UNKNOWN when unobserved), full logs and swap markers for:

1. Direct helper with no relaunch arguments: swap succeeds.
2. Direct helper with valid base64 arguments: swap succeeds.
3. Detached Node parent that exits immediately, in the proven WTS active-console context: swap succeeds and helper logs started plus updated.
4. Same detached launch through SSH context: report actual result without speculative job attribution.

The previous non-GUI cases were invalid because `-RelaunchArgs -` itself failed argument binding. Prior 0x3c00 probe identified libuv's own job flags, not the SSH job. Historical run3 used base64 arguments and remains unexplained. Do not claim these old observations prove a job cause.

Only if cases 1–3 pass, perform exactly ONE new Windows GUI step-7 run from the fixed source: private per-run `--user-data-dir`, `--lang=ko`, old0.49.9 external restamp plus new0.50.0 local feed, genuine Restart now, folder swap and relaunch with the same arguments/profile. Capture actual screenshots and command lines/profile/version/marker/log evidence. No no-argument GUI launch. If a new unexpected failure occurs, stop and report immediately; no autonomous retry. Requested SSH case failure is an observation to report, not authorization to invent a repair.

## Build and deliver

After the conditional authorized GUI acceptance, rebuild from the fixed committed head, without chasing later develop changes: `Werkschaft-darwin-arm64.zip`, `Werkschaft-win32-x64.zip`, and `Werkschaft-desktop-SHA256SUMS`, embedded0.50.0. Use cached Electron/packager inputs only, no downloads, no daemon executable bundle, no tag/upload/main push/version bump. Never launch Electron on the Mac, including smoke/GUI/app launch. Retain final artifacts and all evidence outside the pooled checkout under a fresh `/tmp` task root; report exact source commit and SUMS.

## Host and process safety

Never touch default Windows `%APPDATA%\\Werkschaft` or `C:\\Users\\ki608\\devenv`; APPDATA environment redirection is not isolation. Delete ELECTRON_RUN_AS_NODE rather than setting it empty for GUI. Existing protected daemon PID2640/start639302110386172090/port29417 is read-only: never replace, signal, or stop it. If absent or changed, report actual facts; do not create a witness session holder. No image-name kills. Only owned recorded PID+creation-identity matched processes may be signalled. Preserve historical real update/profile/stage/dummy folders from prior assignments unless MAIN explicitly approves cleanup. Unregister only this task's own completed scheduled tasks. Node inspector must detach before waiting on app exit.

No shared runtime/provider/config actions, foreign keys/signals, forks, outer Go kills, downloads, or broader product fixes. Go is not needed; if unavoidable every invocation must use an absolute cd into this checkout/agents-plugin-tool and GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local with isolated HOME/config/runtime and warm caches. Return immediately at scope expansion or safety blocker.

## Handoff

Report concise complete English with VERIFIED/INFERRED distinctions and file:line citations: fixed head, changed paths, tests, per-case table, exact logs/screenshots/artifact paths/SUMS, GUI result or unmet gate, protected process readbacks and own cleanup. Push only material blocker or >30-minute ETA slip before final. No merge. Do not resume any previous worker on a recycled root. Lead reviews the diff/evidence without rerunning worker tests and owns releasing this checkout after final settlement.
