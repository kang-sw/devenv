# Electron Windows: diagnose and re-enable in-app self-update

## Context

Owner 2026-11-17 (1275)=b, reversing (1138)=a of 2026-11-15: diagnose why the
Windows self-update swap fails and re-enable in-app update on Windows. Owner
also asked where the app is installed by default.

Current state (`agents-plugin-tool/desktop-electron/update.js`): custom
updater against `kang-sw/werkschaft` GitHub releases (public; latest v0.50.2
has `Werkschaft-win32-x64.zip` and `Werkschaft-desktop-SHA256SUMS`).
`update.js:288-291` sets `selfUpdate = platform !== 'win32'` (d1865da55)
because the detached PowerShell swap helper (`PS_HELPER`, ~`update.js:193`)
"fails silently once the app exits"; Windows shows [Open download page]
instead. Prior fix attempts: 052919e16 (staged swap on quit), 1bb5915f1,
fa7ea60b4, 7cc84fbce (relaunch args, helper logging to
`userData/update-apply.log`). Windows ships only a portable zip from
`@electron/packager` (no installer), so the install location is wherever the
user extracted it; `installLocation`/`applyProblem` check same-drive and
writable.

A parallel worker changes daemon auto-start in `desktop-electron/main.js`
(branch impl/develop/electron-daemon-autostart); keep this change to the
updater path so the two merge cleanly.

## Outcome

1. Root cause of the silent Windows swap failure, with evidence: read any
   existing `update-apply.log` (and related logs) under this PC's
   `%APPDATA%` for the Werkschaft app (the owner has installed builds here;
   never print credential-looking values), and trace the helper's launch
   (detached spawn options, `-ExecutionPolicy Bypass`, hidden window,
   waiting for the parent PID / file locks of child processes such as the
   bundled daemon started by the app, rename vs copy across the app folder,
   relaunch).
2. A fixed swap that works on Windows, and `selfUpdate` re-enabled for
   win32: [Update] downloads, verifies, stages, and swaps on quit, then
   relaunches the new version. The download page remains the fallback when
   `applyProblem` reports the location is not writable or similar, with the
   reason shown.
3. The helper's outcome is always observable: success or the failure reason
   lands in `update-apply.log`, and the next launch surfaces a failed apply
   (toast or status) instead of failing silently.
4. Answer for the owner: where the app is installed by default today, and
   whether the updater should recommend or move to a user-writable default
   (e.g. `%LOCALAPPDATA%\Programs\Werkschaft`). Report the recommendation;
   do not add an installer in this change.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC, and do not run the Electron app against a real daemon here. Node tests
  run locally. The PowerShell helper may be exercised locally ONLY against
  dummy folders under `D:/Temp/electron-win-selfupdate/` (fake app dir with
  a dummy file, no real exe from the app); if the antivirus (AhnLab V3)
  reacts, stop immediately and report.
- Temp only under `D:/Temp/electron-win-selfupdate/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main; no
  release, no tags.

## Verification

- Node tests: win32 selfUpdate on; download/verify/stage/apply path; helper
  command line; failure logged and surfaced on next launch; unwritable
  location falls back to the download page with a reason. Update the test
  that asserted "Windows never updates itself".
- Local dummy-folder run of the helper script (if safe) showing the swap and
  log line.
- Report: root cause with evidence, fix commit, test results, default install
  location answer, and the exact owner check on a real installed build
  (install an older zip, update in-app, confirm the new version launches).
- R1 review; fixes-only R2 for substantive findings.
