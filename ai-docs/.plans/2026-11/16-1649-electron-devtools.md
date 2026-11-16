# Electron dashboard Developer Tools

## Assignment

Owner decision (1253)=a, relayed by coordinating lead: add packaged-build Developer Tools access in agents-plugin-tool/desktop-electron. Work from develop f18c773ab in this pooled worktree on impl/develop/electron-devtools. Do not merge or push.

## Contract

- Add View > Toggle Developer Tools targeting dashboardContents(), not the Windows BaseWindow or titlebar WebContentsView.
- Support Windows F12 and Ctrl+Shift+I, and macOS Cmd+Alt+I. Preserve the existing bare-Alt behavior without swallowing F12.
- Keep access available in packaged builds.
- Scope: main.js and focused tests under desktop-electron/test; read neighboring code as needed. Prefer the smallest complete change, no unrelated refactor or new module/API.
- Add focused unit coverage using injected Electron fakes; never launch Electron or any daemon for verification. Verify menu action target, accelerator/input behavior, and packaged availability where practical.

## Safety and delivery

No local Go build/test, test executable, ws-mcp/dashboard executable, launcher or shared daemon mutation/probe via binary. Read-only live daemon inspection, if ever necessary, is REST only; this task should not need it. Go tests would require remote w5090 through sys_lock, but no Go work is assigned. Local focused Node checks are allowed with injected execution and non-executable fixtures. Heavy checks require sys_lock. Temporary files only under D:/Temp/exec3-devtools; set TEMP/TMP/TMPDIR there; stop if D: free space is below 20 GB. Never change or commit the shared primary checkout. Pi commits have no Co-Authored-By trailer. Read all path-applicable project manuals before editing. Commit the unchanged plan with implementation and report exact commands/results and commit ID. Do not touch idle-CPU Phase 2 or Electron lifecycle Phase 4; both remain held. Real packaged Windows/macOS acceptance is not assigned and must not be claimed.
