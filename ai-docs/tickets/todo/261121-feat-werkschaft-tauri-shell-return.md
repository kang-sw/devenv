---
title: "Werkschaft 데스크톱 셸을 일렉트론에서 타우리로 전환"
related:
  261117-research-werkschaft-tauri-shell-revisit: "Phase 4 answers Questions 1–6"
  261115-feat-werkschaft-electron-shell: "Shipping shell and unresolved Phases 2–4"
  261115-feat-werkschaft-electron-daemon-override: "Bundled-daemon behavior and packaged acceptance reference"
  261121-feat-ws-dashboard-inpage-dialogs: "Completed pre-Tauri work and existing close, DevTools, login-link, and refusal decisions"
  261115-feat-ws-daemon-release-auto-update: "Separate release-download daemon updater"
  261120-feat-desktop-canary-release-channel: "Shipped stable, canary opt-in, and stable-return behavior"
---

# Return the Werkschaft desktop shell to Tauri

## Background

On 2026-11-21 at 20:05 KST, after merge `71957269a`, the owner treated the
in-page dialogs as done and directed the team to move on to the Tauri shell.
`261121-feat-ws-dashboard-inpage-dialogs` completed pre-study work units 1–3.
This ticket covers units 4–7 in order. The older thin-shell sizing report
describes an attach-only starting point. The release candidate needs the
current packaged shell's daemon lifecycle and distribution behavior.

## Decisions

- Return Werkschaft to the retained Tauri shell for macOS and Windows, with
  the shipped Electron build as the comparison baseline. Carry forward the
  completed dashboard confirmations, draft recovery, and non-blocking
  daemon-replacement refusal behavior. The prior owner decisions require
  DevTools in release builds and prompt-free OS close when the draft can
  recover. See `261121-feat-ws-dashboard-inpage-dialogs` (2020)–(2024).
- The compatibility target for reusing the daemon contract and adding the
  Tauri release path alongside Electron is **T0**. The existing cross-minor
  daemon boundary remains **T3**.

- (2049) Keep the separate release-download daemon updater on its own ticket.
  See `261115-feat-ws-daemon-release-auto-update` and
  `261115-feat-werkschaft-electron-daemon-override` decision (1201).
- (2050) Carry forward explicit canary opt-in and stable return for shell
  updates. See `261120-feat-desktop-canary-release-channel` Phases 2–3.
- (2051) Include the Evergreen WebView2 bootstrapper in the Windows
  installer. Windows 11 ships the runtime and most Windows 10 receives it
  through Windows Update; it may be missing on Windows Server, LTSC/IoT,
  update-blocked managed or offline images, and long-unpatched Windows 10.
  Rejected: a portable build with a supported runtime prerequisite.
- (2052) Enable Tauri's `devtools` Cargo feature explicitly in the release
  build or default feature set; on Windows use Open Developer Tools because
  WebView2 lacks the Wry toggle behavior. Decision (2021) in
  `261121-feat-ws-dashboard-inpage-dialogs` requires the release menu and
  shortcuts. Rejected: relying on Wry toggle behavior on Windows.
- (2053) Use Windows `CloseRequested` as a fallback for legacy renderers or
  failed recovery. Decision (2020) in
  `261121-feat-ws-dashboard-inpage-dialogs` requires prompt-free close when
  recovery is available. Rejected: leaving fallback cases to the webview.
- (2054) Add a narrowly scoped WebView2 subframe navigation hook where the
  existing sandbox and daemon policy cannot enforce Electron's frame boundary.
  Rejected: relying on Tauri's top-level callback alone.
- (2055) Keep update actions in Settings through an optional, pinned-main-frame
  bridge with bounded operations. Rejected: moving them to native menus.
- (2056) Use `tauri-plugin-updater` with signed NSIS/MSI and `.app.tar.gz`
  update artifacts; Windows changes from portable zip to installer.
  Rejected: porting the custom zip updater.
- (2057) Make the last Electron release a bridge so the update button replaces
  the deployed app automatically. The update action downloads the Tauri
  installer and runs a per-user silent NSIS install under `%LOCALAPPDATA%`
  without an admin prompt on Windows, or replaces the `.app` on macOS, then
  exits. Build the bridge in P3 and release it only after P4 passes. Cover
  cleanup of the old portable folder and shortcuts and migration of Electron
  settings/data locations to Tauri's. With (2060)=b, Windows SmartScreen may
  show a first-run warning the user must accept. Rejected: a separate Tauri
  install that Electron only points to.
- (2058) Keep `261115-feat-werkschaft-electron-shell` Phases 2–4 open until
  Tauri passes P4, then re-home overlapping reliability and Windows delivery
  work to Tauri. Its Phase 4 grant remains conditional on a need for new
  shell-only HTTP authority; see that ticket's decision (859)=a and Phases
  2–4. Rejected: keeping the phases open indefinitely.
- (2059) Inherit Electron's automatic newer-core bundled-daemon replacement
  across minors, consistent with
  `261115-feat-werkschaft-electron-daemon-override` (1343)=a. Older plugins
  at the minor boundary need updating and restarting. Rejected: requiring an
  explicit action at the minor boundary.
- (2060) P4 accepts signed updater artifacts plus documented OS signing
  requirements. Defer OS code signing and macOS notarization to the Electron
  retirement decision. Rejected: requiring OS signing and notarization in P4.
- (2061) P4 records paired measurements and the owner decides from them;
  no thresholds are set now. Rejected: fixed thresholds now.

## Constraints

- Electron keeps shipping until Phase 4 passes; release the P3 bridge only
  after that gate. Retirement or deletion of
  `agents-plugin-tool/desktop-electron/`, and removal of shipped
  functionality, require a separate owner decision.
- Reuse the daemon-served dashboard and exact verified loopback origin.
  Keep the endpoint bearer native, preserve browser and preview boundaries,
  and keep shell-only actions away from external pages and child frames.
  A new scoped HTTP grant needs its own justified contract.
- Follow `ai-docs/manuals/shipped-surface-boundary.md` for shipped text,
  `ai-docs/manuals/performance.md` for storage types, file-operation counts,
  interactive I/O, and Windows antivirus costs, and
  `ai-docs/manuals/release-compatibility.md` for daemon and updater surfaces.
- **Compatibility gate.** P1/P2 target **T0** for reuse of the existing
  endpoint, health, cookie, and daemon-start contracts and for any optional
  shell hooks which old pages can ignore. The daemon writes the endpoint,
  the shell reads it and requests the cookie under existing machine auth,
  and neither shell rewrites the daemon's file through those calls
  (`desktop-electron/endpoint.js` `readEndpoint` and `browserCookie`,
  `internal/wsdashboard/localhost_auth.go` `machineAuth` and
  `handleAuthCookie`, under `agents-plugin-tool/`). P3 targets **T0** for
  parallel artifacts and optional update controls while the Electron
  updater still finds its zip/SUMS and each shell reads its own updater
  state. The Windows artifact transition and existing-install migration
  need their own directional verdict. Record the old/new readers and
  writers, legacy fallback, and reopen outcome with source evidence for
  the settled design before promotion, then verify the actual result in P4.
  The existing cross-minor hello refusal is **T3**
  (`agents-plugin-tool/internal/wsdashboard/proxy.go`, hello validation).
  A new T3 boundary requires explicit owner approval and a minor bump.
  T4 cannot ship.
- Scope verification to owned test processes, disposable app profiles,
  and isolated daemon endpoints and caches. Follow the Windows host limits
  in `261117-research-werkschaft-tauri-shell-revisit`.

## Prior Art

- `agents-plugin-tool/desktop/` is the retained macOS attach-only baseline.
  `agents-plugin-tool/desktop-electron/` is the packaged reference for
  daemon selection and staging, native cookie and login-link flows,
  recovery, navigation, and updater rollback.
- `agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs`
  validates the native binary and runtime inventory and assembles resources
  and skills with the matching daemon. `.github/workflows/ws-mcp-release.yml`
  builds the darwin-arm64 and win32-x64 Electron archives and publishes
  stable and canary assets for the native and desktop builds.

## Phases

### Phase 1: Refresh the Tauri baseline and deliver the cross-platform shell core

Bring the retained `desktop/` host up on darwin and win32. Attach to a
health-checked existing daemon using the cache-root override and the
existing machine-authenticated cookie flow. Retain the login gate when
cookie minting is unavailable. Handle daemon or renderer failure with
rediscovery, Retry, and bounded redacted diagnostics. Provide local
connection and Python-missing views, native menu and zoom actions,
the Copy Browser Login Link action, and second-instance restore and
focus. Preserve the `Werkschaft/` user-agent token and in-page dialog
and draft behavior. Enable release DevTools, use Open Developer Tools on
Windows, and provide the `CloseRequested` fallback for legacy renderers or
failed recovery. Add the scoped WebView2 subframe hook where needed to
enforce the frame boundary.

Verify the exact dashboard origin, external link handoff, blocked schemes,
popups, and subframes. Cover `tauri://localhost` on macOS and
`http://tauri.localhost` on Windows, including which local pages may
request shell actions. Keep HTML5 file drops and clipboard behavior
working under WebView2's native drop handling and clipboard permission
model. Exercise the approved release DevTools and close behavior on
both platforms. Prove that failed health or cookie checks leave a
usable retry or login surface and that close leaves the daemon running.

### Phase 2: Port the bundled daemon lifecycle to Rust

Build on Phase 1's discovery, authentication and recovery. Assemble
and bundle the matching binary, metadata, resources and skills. Port
candidate validation, bundled and selected-development launch, safe
staging, cache and launch environment handling, missing-daemon start,
attach and automatic newer-core replacement across minors under (2059).
Revalidate daemon identity before a drain and after staging or reconnect. Preserve session
holders and show replacement refusal without interrupting a still-healthy
connection. Handle the macOS Python prerequisite and preflight failure
before draining.

Verify standalone packaged start and newer-daemon attach. Exercise
concurrent start, stale endpoints, candidate validation and staging
failure, the automatic newer-core replacement across minors, and the version
and explicit-replacement cases. On Windows,
exercise strict independent-lifetime launch, the
`WS_DAEMON_BREAKAWAY_DENIED=5` path and retry with
`WS_MCP_DAEMON_JOB_LIFETIME=1`, and bounded settle under antivirus
delays. Check that the daemon survives app exit after both launch paths
and that holders retain their identities across a successful handoff.

### Phase 3: Deliver shell updates and the CI release pipeline

Package Phase 2 for darwin-arm64 and win32-x64 with the settled updater
and installer strategy: `tauri-plugin-updater`, signed NSIS/MSI and
`.app.tar.gz` update artifacts, and the Evergreen bootstrapper. Wire the
platform artifacts, integrity and authenticity checks, feed metadata and CI
into the existing release pipeline. Keep update actions in Settings through
the optional pinned-main-frame bridge. Build the last Electron release as the
(2057) migration bridge, including per-user silent NSIS installation, macOS
app replacement, settings/data migration, and portable folder and shortcut
cleanup. Release the bridge only after P4 passes.
Keep Electron zip/SUMS and native daemon artifacts available during
evaluation. Preserve stable tag behavior and demonstrate prerelease
isolation, opt-in canary and explicit stable return under the settled
channel decision. Keep shell update and daemon replacement distinct.

Verify staging and apply of a valid update, refusal on missing or invalid
integrity data, interruption/retry, relaunch, and the recovery route for
a failed install or swap. Exercise Windows runtime-present and
runtime-missing bootstrapper behavior. Use a
non-publishing pipeline run to check expected output names, platforms,
versions and metadata before release. Verify the proposed first migration
from a shipped Electron installation on a disposable profile, including the
update action, per-user install or app replacement, migrated settings/data,
cleanup, exit, and recovery after failure. Account for the possible Windows
SmartScreen first-run warning.

### Phase 4: Pass the macOS and Windows verification and benchmark gate

Run packaged Tauri and Electron on both platforms and record evidence
against Questions 1–6 in
`261117-research-werkschaft-tauri-shell-revisit`.

1. **WKWebView viability.** Check in-page confirms, X-close and draft reopen,
   zoom keys, external links, file and image handling, clipboard, Korean IME,
   `:has()`, `content-visibility`, and container queries. Check dashboard
   writes, terminals and live transports, isolated content frames, and
   notification fallback. Verify pasted images and unsent input recover
   after reopen and delivery-unknown sends await user action. List remaining
   differences and the native work each would take.
2. **Windows parity.** Record the Windows versions and Evergreen runtime
   versions tested. Exercise the installer bootstrapper path,
   including a supported Windows image where the runtime is absent.
   Record a pass or fail for each row below.
3. **Bundle size and memory.** On darwin and win32, report installed and
   download bytes for Tauri and Electron, including and excluding the
   bundled daemon. Record shell/webview process-tree idle and active
   working-set memory in MiB with the same board and daemon build. Use the
   same machine, workload and sampling points for each pair, and report
   the daemon separately. Record absolute values and Tauri–Electron deltas.
4. **Rendering performance.** Run the same scroll and typing scenario on
   WKWebView, WebView2, and Electron Chromium. Record frame timings or
   dropped frames and input latency, with the OS, webview version, board
   content and scenario needed to reproduce the comparison.
5. **Operations.** Verify release DevTools and the selected updater route
   with a packaged release candidate. Compare signed Tauri updates and
   installer options (NSIS/MSI and macOS DMG where applicable) with the
   current Electron zip updater and document platform code-signing and
   notarization requirements; OS code signing and notarization are deferred
   to the Electron retirement decision, not required for P4. Test canary
   opt-in and stable return and the Electron migration bridge.
6. **Porting cost.** Record the native changes and remaining gaps for
   bundled daemon autostart and breakaway, existing-daemon attach,
   DevTools, request/navigation guards, cookie injection, and the
   Python-missing guide.

Windows pass list from pre-study Section D:

- **(a)** Standalone daemon start, breakaway-denied retry and cookie injection
  succeed. The daemon survives X-close.
- **(b)** HTML5 drag/drop reaches the dashboard. Copy and paste work,
  including the login clipboard path under WebView2 permissions.
- **(c)** The local `tauri.localhost` origin and expected subframes work.
  Unapproved subframe targets and popups enforce the agreed guard.
- **(d)** The release menu bar and physical shortcuts work, including
  DevTools and zoom. Confirm the approved `CloseRequested` behavior
  with a recoverable draft and its fallback path.
- **(e)** The selected Windows installer and updater flow works end to end
  using NSIS/MSI. Exercise restart, failed-update recovery, and file locking
  under antivirus.

The gate requires the macOS and Windows packaged results, a pass for each
Windows row, answers to all six research questions, and paired measurements
for the owner to decide, without fixed thresholds. Record the evidence and
the owner’s release decision in the phase Result. After P4 passes, re-home
the overlapping reliability and Windows delivery work from
`261115-feat-werkschaft-electron-shell` Phases 2–4 to Tauri.
