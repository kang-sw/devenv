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

## Constraints

- Electron keeps shipping until Phase 4 passes. Retirement or deletion of
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

## Open Decision Queue

Last ID used: (??)

(??) [open] [announced] Keep the separate release-download daemon updater on its own ticket. See `261115-feat-ws-daemon-release-auto-update` and `261115-feat-werkschaft-electron-daemon-override` decision (1201).

(??) [open] [announced] Carry forward the explicit canary opt-in and stable return for shell updates. See `261120-feat-desktop-canary-release-channel` Phases 2–3.

(??) [open] [technical] Prefer including the Evergreen WebView2 bootstrapper if Windows ships an installer, with a supported runtime prerequisite if it stays portable, because some Windows images lack the runtime.

(??) [open] [technical] Enable Tauri's `devtools` Cargo feature explicitly in the release build configuration or include it in the default Cargo feature set. On Windows use Open Developer Tools because WebView2 lacks the Wry toggle behavior. Decision (2021) in `261121-feat-ws-dashboard-inpage-dialogs` already requires the release menu and shortcuts.

(??) [open] [technical] Use Windows `CloseRequested` only to provide a fallback for legacy renderers or failed recovery, rather than leave those cases entirely to the webview. Decision (2020) in `261121-feat-ws-dashboard-inpage-dialogs` already calls for prompt-free close when recovery is available.

(??) [open] [technical] Add a narrowly scoped WebView2 subframe navigation hook where the existing sandbox and daemon policy cannot enforce Electron's frame boundary, over relying on Tauri's top-level callback alone.

(??) [open] [technical] Keep update actions in Settings through an optional, pinned-main-frame bridge over moving them to native menus, to preserve the existing user flow with a bounded set of operations.

(??) [open] [policy] Choose `tauri-plugin-updater` with signed NSIS/MSI and `.app.tar.gz` update artifacts, changing Windows from portable zip to installer, or port the current custom zip updater.

- The existing Windows zip requires no installer. An installer also gives
  the release pipeline a place to handle the Evergreen bootstrapper.
- The portable path retains the zip and requires a Windows-safe swap,
  relaunch, and rollback under file locks.
- Prefer the Tauri updater if changing Windows delivery is acceptable. It
  delegates signed package verification and installation to the updater.

(??) [open] [policy] Decide how an existing Electron installation moves to Tauri after P4, through an opt-in migration offered by Electron or an explicit separate Tauri install.

- An Electron offer needs a supported handoff from a zip installation.
  A separate install keeps Electron's existing update feed intact.
- Prefer a separate install unless a tested handoff preserves the existing
  installation and gives the user a recovery route.

(??) [open] [policy] Decide whether to keep `261115-feat-werkschaft-electron-shell` Phases 2–4 open or re-home the overlapping reliability and Windows delivery work once Tauri passes P4. See that ticket's decision (859)=a and Phases 2–4.

- The earlier owner choice moved these phases from Tauri to Electron.
  Phase 4 is conditional on a need for new shell-only HTTP authority.
- Prefer re-homing only overlapping work after Tauri passes P4 and keeping
  the grant conditional on an actual need for new authority.

(??) [open] [policy] Decide whether Tauri inherits Electron's automatic newer-core bundled-daemon replacement across minors or requires an explicit action at the existing minor boundary. See `261115-feat-werkschaft-electron-daemon-override` decision (1343)=a.

- Current ordinary Electron startup upgrades any older numeric core and
  never downgrades, including at a minor boundary where older plugins
  need updating and restarting.
- Prefer carrying the observed behavior forward if Tauri is meant to
  match the current bundled lifecycle. Confirm the owner-facing lockout
  consequence before relying on it.

(??) [open] [policy] Decide whether P4 requires OS signing and macOS notarization or accepts signed updater artifacts and documented OS signing requirements for the candidate.

- Updater trust and OS recognition of a signed installer or app are
  separate checks. The Electron release workflow currently packages
  unsigned apps.
- Prefer proving updater trust in P3 and deciding OS signing requirements
  before publishing a Tauri replacement.

(??) [open] [policy] Set P4's size, memory, and interaction-performance pass thresholds or have the owner decide from the paired measurements.

- The research ticket records an expectation of lower size and memory.
  It records no measured Tauri baseline or numeric pass threshold.
- Prefer an owner decision from paired data if there is no useful
  threshold before the first packaged run.

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
and draft behavior.

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
attach and replacement under the settled policy. Revalidate daemon
identity before a drain and after staging or reconnect. Preserve session
holders and show replacement refusal without interrupting a still-healthy
connection. Handle the macOS Python prerequisite and preflight failure
before draining.

Verify standalone packaged start and newer-daemon attach. Exercise
concurrent start, stale endpoints, candidate validation and staging
failure, and the version and explicit-replacement cases. On Windows,
exercise strict independent-lifetime launch, the
`WS_DAEMON_BREAKAWAY_DENIED=5` path and retry with
`WS_MCP_DAEMON_JOB_LIFETIME=1`, and bounded settle under antivirus
delays. Check that the daemon survives app exit after both launch paths
and that holders retain their identities across a successful handoff.

### Phase 3: Deliver shell updates and the CI release pipeline

Package Phase 2 for darwin-arm64 and win32-x64 with the settled updater
and installer strategy. Wire the platform artifacts, approved integrity
and authenticity checks, feed metadata and CI into the existing release
pipeline.
Keep Electron zip/SUMS and native daemon artifacts available during
evaluation. Preserve stable tag behavior and demonstrate prerelease
isolation, opt-in canary and explicit stable return under the settled
channel decision. Keep shell update and daemon replacement distinct.

Verify staging and apply of a valid update, refusal on missing or invalid
integrity data, interruption/retry, relaunch, and the recovery route for
a failed install or swap. Exercise Windows runtime-present and
runtime-missing behavior under the approved delivery model. Use a
non-publishing pipeline run to check expected output names, platforms,
versions and metadata before release. Verify the proposed first migration
from a shipped Electron installation on a disposable profile.

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
   versions tested. Exercise the runtime prerequisite or bootstrapper path,
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
   notarization requirements. Test approved canary opt-in and stable return.
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
  using NSIS/MSI if selected, or the approved portable zip flow. Exercise
  restart, failed-update recovery, and file locking under antivirus.

The gate requires the macOS and Windows packaged results, a pass for each
Windows row, answers to all six research questions, and paired measurements
reviewed against the settled pass criteria. Record the evidence and the
release decision in the phase Result.
