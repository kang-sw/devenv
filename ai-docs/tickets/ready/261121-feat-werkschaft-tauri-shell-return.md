---
title: "Werkschaft 데스크톱 셸을 일렉트론에서 타우리로 전환"
related:
  261117-research-werkschaft-tauri-shell-revisit: "Phase 4 answers Questions 1–6"
  261115-feat-werkschaft-electron-shell: "Shipping shell and unresolved Phases 2–4"
  261115-feat-werkschaft-electron-daemon-override: "Bundled-daemon behavior and packaged acceptance reference"
  261121-feat-ws-dashboard-inpage-dialogs: "Completed pre-Tauri work and existing close, DevTools, login-link, and refusal decisions"
  261115-feat-ws-daemon-release-auto-update: "Separate release-download daemon updater"
  261120-feat-desktop-canary-release-channel: "Shipped stable, canary opt-in, and stable-return behavior"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cc5b4b3b24146e2e
sage-review-completeness-reviewed: cc5b4b3b24146e2e
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
- (2057) The owner accepted replacing the separate Electron bridge with
  (2065)=a: the existing Electron updater swaps in Tauri-compatible payloads
  from each stable release. With (2060)=b, Windows SmartScreen may show a
  first-run warning the user must accept. Rejected: a separate Tauri install
  that Electron only points to.
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

- (2065)=a [policy, owner-confirmed] Ship no separate Electron bridge release.
  Every Tauri stable release attaches `Werkschaft-<platform>-<arch>.zip`
  and `Werkschaft-desktop-SHA256SUMS`, with Tauri payloads. The existing
  Electron updater reads only `releases/latest` for stable checks and canary
  stable return (`agents-plugin-tool/desktop-electron/update.js:512-518`,
  `554-562`), selects named assets using `browser_download_url` (73-92),
  validates the SHA-256 and extracted `Werkschaft.exe` or
  `Werkschaft.app/Contents/MacOS/Werkschaft` (185-197), swaps the folder or
  app and relaunches the executable or opens the app (296, 257). Thus even
  the first shipped Electron release and canary installs returning to stable
  migrate in one update click. Rejected: a one-time Electron bridge plus
  manual reinstall and a no-zip notice in the next Electron release; CI
  re-uploading the approximately 200 MB bridge zip to each stable release
  until the next minor, since GitHub has no cross-release asset reference.
- (2068)=ok [technical, owner-acknowledged] The Windows zip contains a small
  `Werkschaft.exe` launcher and the Tauri per-user NSIS installer. On the
  first run of each delivered payload the launcher silently installs its
  bundled Tauri version per-user, then launches it and exits. Subsequent runs
  launch the installed app. The NSIS installer's built-in WebView2
  install mode covers missing runtimes including the (2051) Server, LTSC,
  and update-blocked scope. Keep the legacy Electron folder as the forwarding
  launcher so old shortcuts work; the Tauri app carries no migration code.
  The macOS zip contains the Tauri `Werkschaft.app` directly, using system
  WKWebView; Tauri's macOS updater also replaces the app in place. Rejected:
  Tauri self-relocation on first run, which cannot start without WebView2 and
  breaks old shortcuts.
- (2070)=a [policy, owner-confirmed] Refine (2068): the first run of a newly
  delivered launcher payload installs exactly its bundled Tauri version,
  including a downgrade, so Electron's Back to stable works even with a newer
  Tauri canary installed (`desktop-electron/test/update-canary.test.js:141-171`).
  A small per-user marker, keyed by the launcher payload and stored under the
  current user's profile (not beside the shared portable launcher), records
  the bundled version after a successful install, allowing a failed install
  to be retried; another Windows account running the same portable launcher
  therefore still gets its own per-user install. Every later
  run only launches the installed app, preserving
  updates installed by Tauri's own updater. Rejected: open any equal-or-newer
  installed build, which defeats Back to stable.
- (2066)=a [policy, owner-confirmed] Do not migrate draft-recovery
  records, including IndexedDB image blobs and delivery-unknown records
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:90-112`,
  `185-206`): Electron's origin-scoped IndexedDB cannot move to WebView2 or
  WKWebView. Before swapping, the Electron side should warn about unsent
  drafts and ask users to send or discard them first, but this warning exists
  only if a future Electron release adds it. The existing updater performs
  the (2065) swap without that warning, so installs updating directly from
  older Electron versions migrate without warning. Retain the Electron
  profile for rollback and delete it only in a later release; reinstalling
  Electron reopens the retained profile. Rejected: export/import of draft
  recovery data; deleting the profile immediately.
- (2069)=b [policy, owner-confirmed] Supersede the settings part of (2066):
  migrate no Electron shell settings. Electron persists only a development
  daemon-selection file containing an absolute development-checkout root
  (`agents-plugin-tool/desktop-electron/daemon-selection.js:130-145`); window
  bounds use fixed launch defaults (`desktop-electron/main.js:100-108`) and
  the channel follows the running version (`desktop-electron/main.js:509-514`,
  `desktop-electron/update.js:756-758`). End users have no persisted settings
  to carry. Tauri starts with default window settings and a channel derived
  from the installed build's version; developers re-set any daemon override
  in Tauri. Neither Tauri nor the launcher imports settings. Rejected: Tauri
  first-launch import of the Electron daemon-selection file.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Electron keeps shipping until Phase 4 passes; publish the Tauri migration
  assets after that gate. Retirement or deletion of
  `agents-plugin-tool/desktop-electron/`, and removal of shipped
  functionality, require a separate owner decision.
- Reuse the daemon-served dashboard and exact verified loopback origin.
  Keep the endpoint bearer native, preserve browser and preview boundaries,
  and keep shell-only actions away from external pages and child frames.
  A new scoped HTTP grant needs its own justified contract.
- Apply (2069) defaults without a settings importer; retain the Electron
  profile and exclude drafts under (2066). Apply (2070)'s per-payload marker
  and exact first-run install, including downgrades, with later launcher runs
  preserving Tauri self-updates. Electron's swap helper rolls back failed
  file moves only: on Windows it records success before starting the forwarder
  and does not wait for NSIS; on macOS it removes the backup before opening
  Tauri (`desktop-electron/update.js:246-258, 283-307`). Post-swap installer
  and first-launch failures need a retry or Electron reinstall recovery path.
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
  state. For existing installs, the old Electron updater reads the new stable
  release zip and SUMS and swaps the payload in place (2065); Windows old
  shortcuts read the retained launcher, which installs the exact bundled
  version on the payload's first run and subsequently opens the installed
  Tauri (2068, 2070). No Electron settings are imported (2069). The Electron
  profile remains available when Electron is reinstalled (2066); draft
  records do not cross webview origins. Failed moves have Electron rollback;
  post-swap failures require retry or reinstall. Record the directional tier and
  remaining old/new readers, writers and fallback evidence before promotion,
  then verify the actual result in P4.
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

## Prior Decisions

- 0fcecf155 (2026-11-21, commit): "IndexedDB recovery preserves blob and delivery state through the old renderer's whole-record localStorage writes." — bearing: constrains
- 261117-research-werkschaft-tauri-shell-revisit (2026-11-17, Confirmed Decisions): "(1279)=a, 2026-11-17: keep Electron for now. Prioritize performance and the auto-updater, then use this research to decide whether to return to Tauri." — bearing: supports
- 261115-feat-ws-daemon-release-auto-update (2026-11-15, Decisions): "The user's apply click is the single exception" — bearing: contradiction-candidate
- 261113-feat-werkschaft-thin-tauri-shell (2026-11-15, Resolution): "This ticket's unimplemented Phases 2-4 were re-homed to 261115-feat-werkschaft-electron-shell as its Phases 2-4, rewritten for Electron" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/desktop/, agents-plugin-tool/desktop-electron/, .github/workflows/ws-mcp-release.yml |
| scope.surface | cross-module | Tauri host, daemon contracts, dashboard update bridge and release pipeline |
| scope.new_public_symbol | unknown | Optional shell update bridge is planned but its exported names are not specified |
| scope.new_type_contract | unknown | Update bridge operations and migration contracts are not specified as signatures |
| scope.test_surface | existing | agents-plugin-tool/desktop/scripts/test_build_slot.py, agents-plugin-tool/desktop-electron/test/, agents-plugin-tool/internal/wsdashboard/testdata/desktop-update.browser.cjs |
| complexity.reuse_points | confirmed | agents-plugin-tool/desktop/README.md; agents-plugin-tool/desktop-electron/endpoint.js and scripts/assemble-daemon-input.cjs |
| complexity.side_effect_risk | high | Daemon replacement and installed-app migration affect live processes and user data |
| risk.correctness | high | Cross-platform lifecycle, update recovery and migration must preserve existing sessions and data |
| risk.fit | high | The retained attach-only host must match the packaged Electron shell and established dashboard behavior |
| risk.test | high | Packaged macOS and Windows acceptance, runtime-absent images and paired measurements are required |
| risk.security_or_contract | high | Authenticated loopback, frame isolation, signed updates and cross-version daemon boundaries are involved |

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

### Result (4706360a9) - 2026-11-21

The Tauri release shell attaches on macOS and targets Windows x64. It
discovers the existing daemon, pins the health-checked origin, sets the
browser cookie natively, and preserves the login gate when cookie setup is
unavailable. Connection Retry, the Python guide, the native browser login
link action, release DevTools and zoom, single-instance restore, and bounded
failure recovery are implemented. The native navigation delegates gate
top-level and subframe navigation separately. The WKWebView and WebView2
paths admit the platform's bundled pages only at the top level. The
dashboard and bundled pages have zero native command permissions.

- T0. The daemon's existing machine authentication and browser-cookie route
  remain the writers of auth state
  (`agents-plugin-tool/internal/wsdashboard/localhost_auth.go:352,395`).
  The shell reads and verifies the existing endpoint and health, requests
  the existing credential or browser login link, and keeps the bearer
  native (`agents-plugin-tool/desktop/src/endpoint.rs:122,165,204,244`).
  A legacy shell ignores the optional renderer close-readiness function.
  A new shell can show a native close fallback with a legacy renderer
  (`agents-plugin-tool/desktop/src/close.rs:66-86` and
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:1106-1119`).
  The existing cross-minor boundary stays T3.
- The shell introduces no persistent store. The existing shared endpoint
  remains a Type 5 file. A successful authenticated attach or Retry makes
  seven bounded logical reads of that file across discovery, auth and final
  revalidation. Polling the cached identity uses HTTP health only. Native
  browser-link copying makes four endpoint reads. Shell diagnostics write
  to standard error and include fixed messages. Draft saves reuse the
  dashboard's existing webview-local stores.
- macOS passed 15 Rust tests and bundled the unsigned arm64 release app.
  An MSVC x64 cross-target check of the Windows code and tests passed.
  Five build-slot regressions and focused Go/Chromium asset, auth,
  login-gate and draft-recovery checks passed. The Chromium checks exercised
  ready and failed draft persistence through the new optional hook.
- Lite review found that initialization failure could strand Retry.
  `4706360a9` fixed that path and fenced callbacks from retired WebView2
  instances. The 15 Rust tests include the hook-retry regression.
- Headless connection, Python, login and draft screenshots are under
  `/private/tmp/werkschaft-phase1.GoFgYN/captures/`. An earlier disposable
  WKWebView smoke observed local views, authenticated attach, a single
  instance, release DevTools and unchanged shared-daemon health. Native
  acceptance on the final head is pending on both platforms, including
  menu and fallback-dialog screenshots, actual accelerators, external and
  frame behavior, native clipboard and drop, renderer recovery, and X-close
  with a recoverable draft and with legacy or failed recovery. Windows
  runtime execution awaits the smoke host. The native checklist and Windows
  build commands are in
  `/private/tmp/werkschaft-phase1.GoFgYN/native-checklist.md`.

#### Edition (8a398a2b1) - 2026-11-21

On Windows 11 build 26200, the lead smoke-tested `6bd531956` with
WebView2. Authenticated attach, menu items, F12, mouse-driven zoom,
second-instance restore and focus, legacy Stay and Close, and unchanged
daemon PID after close passed. An empty scratch cache showed the
connection view with a redacted status. Mouse-driven Retry reached the
dashboard from a token-free endpoint in a profile that already held a
cookie. The fresh-profile login gate remains pending.

Ctrl+R, Ctrl+= and Ctrl+- failed with WebView2 focused. The controller now
handles Retry, zoom, and DevTools from physical key-down events and
dispatches those events through the same Rust actions as the native menus.
macOS keeps its menu accelerators. The local guides remain in English.

All 18 Rust tests passed on macOS, including three keyboard mapping tests.
The MSVC x64 cross-target check of the Windows code and tests passed.
The Windows host rerun remains pending for Ctrl+R, Ctrl+=, Ctrl+Shift+=,
Ctrl+-, Ctrl+0, Ctrl+Shift+I and F12 with the webview focused.

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
Developers re-set any Electron daemon override in Tauri; do not import it
from Electron (2069).

Verify standalone packaged start and newer-daemon attach. Exercise
concurrent start, stale endpoints, candidate validation and staging
failure, the automatic newer-core replacement across minors, and the version
and explicit-replacement cases. On Windows,
exercise strict independent-lifetime launch, the
`WS_DAEMON_BREAKAWAY_DENIED=5` path and retry with
`WS_MCP_DAEMON_JOB_LIFETIME=1`, and bounded settle under antivirus
delays. Check that the daemon survives shell exit after both launch paths,
independently of job teardown. On the fallback path it survives only while
its containing job lives (owner-approved retry in `062f291f5`;
`agents-plugin-tool/internal/wsdashboard/spawn_windows.go:30-54`). Check
that holders retain their identities across a successful handoff.

### Result (0aeef115b) - 2026-11-21

- The validated package build embeds the matching daemon, metadata, resources,
  skills and configuration manifests. The unsigned arm64 macOS app contains
  all 113 input files, compared byte for byte. The Windows x64 daemon was
  cross-built, stamped and validated. The Windows shell and its tests passed
  an MSVC x64 cross-target check.
- Rust selects and checks the prepared bundled or development input and
  stages an independent copy outside the app. The development selector is
  the Tauri launch environment's `WERKSCHAFT_DAEMON_ROOT`. Tests cover
  version, hash, inventory, manifest, link, containment, staging failure and
  same-core source mutation cases. Stages are Type 5 retained package files.
  For an N-file package, selection fingerprints N files plus contract checks.
  A successful stage copies and sets permissions on N files, then reads the
  N files once from the stage and once from the source for fingerprints,
  plus staged contract validation and metadata checks. These operations run
  on a background thread. Windows antivirus tracing remains pending.
- The connection tests cover missing-only startup, a concurrent winner,
  newer-daemon attach, automatic previous-minor replacement, explicit
  equal-core replacement and recovery. Listener tests cover stale endpoint
  startup, concurrent daemon selection, same-port replacement, and an actual
  prestarted subprocess whose gate denies startup when the predecessor
  identity changes. The new starter revalidates before drain and preserves
  the predecessor's LAN mode and preferred port. Holder and proxy tests
  exercise unchanged holder and child identities, compatible reconnect and
  previous-minor refusal.
- macOS checks Python before launching or replacing. The tests exercise its
  missing prerequisite and preservation of a healthy incumbent. Replacement
  failures keep a verified live document and use the pinned dashboard's
  warning toast, with the reason in the title when the renderer cannot show
  it. A transient post-staging endpoint read also retains a healthy
  incumbent. The Chromium test checks main-frame and origin scope, draft
  preservation, the fallback acknowledgement and both local guides.
- T0 for the additive starter. Old callers omit the flags and use the
  original Ensure path. An older candidate that lacks the flag stops during
  CLI parsing before it touches the daemon (`cmd/ws-mcp/proxy.go:393-417`).
  The existing daemon writes the endpoint
  (`internal/wsdashboard/endpoint.go:89`) and mints the browser cookie
  (`internal/wsdashboard/localhost_auth.go:395`). Tauri reads the endpoint
  and sends the observed identity and token digest in the starter's private input
  (`desktop/src/endpoint.rs:74,199`). The guarded starter uses the existing
  authenticated drain and admits the waiting child only after acceptance
  (`internal/wsdashboard/ensure.go:261`). It changes no shared schema.
  The previous-minor hello check remains T3
  (`internal/wsdashboard/proxy.go:251`).
- The lead's unrestricted macOS run passed all 31 Rust tests and 23 top-level
  Go tests across daemon, CLI and holder suites. The Windows-only Go test
  skipped on macOS. The packaging tests passed 2, build-slot tests passed 5,
  and the headless Chromium test passed. The lite review's preflight race
  and two test and recovery findings were fixed in `464771aa4`.
- Captures and side-by-side comparisons are in
  `/private/tmp/werkschaft-phase2.9krA6b/captures/`. The refusal toast
  matches the Electron in-page reference at
  `/private/tmp/inpage-dialogs-captures/settings-after-in-page-notice.png`.
  The connection and Python views retain their Phase 1 layouts and now tell
  the user that Retry can start the dashboard.
- Windows native checks are assigned to the lead. The checklist is at
  `/private/tmp/werkschaft-phase2.9krA6b/native-checklist.md` and the
  prepared x64 daemon input is under its `final/` directory. Native coverage
  includes packaged start and cookie, version and failure cases, menu and
  title captures, strict breakaway and access-denied job-lifetime retry,
  shell exit versus job teardown, antivirus settle and holder identity
  across a real handoff. Native macOS execution remains for the packaged
  platform gate.

### Phase 3: Deliver shell updates and the CI release pipeline

Package Phase 2 for darwin-arm64 and win32-x64 with the settled updater
and installer strategy: `tauri-plugin-updater`, signed NSIS/MSI and
`.app.tar.gz` update artifacts, and the Evergreen bootstrapper. Wire the
platform artifacts, integrity and authenticity checks, feed metadata and CI
into the existing release pipeline. Keep update actions in Settings through
the optional pinned-main-frame bridge. Produce the (2065) Electron-named
zip and SUMS assets for every Tauri stable release, with the Windows
forwarding launcher and per-user NSIS installer or the macOS Tauri app as
specified in (2068), with (2070)'s exact first-run install and marker and
later launch-only behavior. Start with default window settings and the
installed build's channel, without importing Electron shell settings (2069).
Retain the Electron profile for rollback under (2066); do not migrate
draft-recovery records. Keep native
daemon artifacts available during evaluation. Preserve stable tag behavior
and demonstrate prerelease isolation, opt-in canary and explicit stable return
under the settled
channel decision. Keep shell update and daemon replacement distinct.

Verify staging and apply of a valid update, refusal on missing or invalid
integrity data, interruption/retry, relaunch, and the recovery route for
a failed install or swap. Exercise Windows runtime-present and
runtime-missing bootstrapper behavior. Use a
non-publishing pipeline run to check expected output names, platforms,
versions and metadata before release. Verify migration from shipped Electron
on a disposable profile, including the update action, Windows launcher
installation or macOS app replacement, first-launch defaults, retained
profile and exit. Separately exercise failed file moves, post-swap installer
failure, and first-launch failure; verify retry or Electron reinstall with
the retained profile for post-swap failures. Verify the first launcher run
installs the bundled version even over a newer Tauri canary, and subsequent
shortcut launches preserve a Tauri self-update. Account
for the possible Windows SmartScreen first-run warning.

#### Phase 3 implementation handoff (17364c3c1) - 2026-11-21

- Settings uses the fixed native update operations on the verified
  top-level dashboard. Stable checks use GitHub latest. Canary selects the
  highest eligible version on the first matching page. Explicit stable
  return can select an older version. The updater verifies size, SHA-256,
  and a Tauri signature bound to the chosen version before staging. Apply
  saves the current dashboard draft and delegates install and relaunch to
  the platform updater. The daemon follows its existing launch policy on
  shell relaunch.
- Skip is a Type 2 SQLite preference with one held connection and a WAL
  commit on change. A verified update is a Type 5 package. A download opens
  the bounded partial file, writes its header, metadata and payload, then
  syncs and renames it. Reopen reads the header and the payload once and
  verifies both. Changing the selected target discards a superseded stage
  before the new selection appears. Settings reads phase and channel from
  memory. Windows antivirus measurements remain part of native acceptance.
- T0. Electron continues to read the same stable latest endpoint, zip and
  SUMS (`desktop-electron/update.js:73,478,515,559`). With the rollout flag
  unset, its existing tag and desktop dry-run jobs still write those assets.
  With the flag set, an exact stable tag includes the same names in the
  draft-backed complete release (`.github/workflows/ws-mcp-release.yml:184,238`
  and `.github/workflows/werkschaft-tauri-build.yml:159-180`). Exact canary
  tags remain prereleases and carry the separate Tauri manifest alongside
  the Electron assets. The optional renderer event mounts the same update
  controls (`internal/wsdashboard/assets/dashboard-lifecycle.js:1130,1230`).
  The Windows forwarder reads and writes only its user-scoped marker and
  opens the registered app on subsequent runs (`desktop/migration/main.rs:218-258`).
  Electron's own profile remains available after reinstall. Tauri starts
  with its own defaults and uses the installed build's channel. The
  existing cross-minor daemon boundary remains T3.
- The macOS release build and Windows x64 MSVC type check passed. The
  launchless arm64 app and updater/migration archive layouts matched the
  expected app identity and all 113 prepared daemon files. The six updater
  Rust regressions include the pinned plugin's signed fixture, corrupt and
  truncated bytes, signature rejection, valid staging, reopen, channel
  switch and retry. The 32 non-listener Rust tests, six Node packaging
  tests, five build-slot tests, two launcher tests, 187 Electron tests
  with four Windows-only skips, Go DashboardSourceAssembly, actionlint,
  and dry-shell route and publication checks passed.
- Chromium ran the dashboard update suite against intercepted real
  renderer assets. Compared captures in
  `/private/tmp/werkschaft-tauri-phase3.4BB1FT/tauri/` with
  the approved PNGs recovered from
  `e33dc16f8:ai-docs/assets/mockups/261120/canary-channel/`.
  The ten light and dark App update and toast regions match. The paired
  regions are in `/private/tmp/werkschaft-tauri-phase3.4BB1FT/update-detail/`.
  Branding and landing-page background differ from the historical captures.
- The lite review found two state and release hazards. `17364c3c1` fixes
  the superseded stage on reopen and publishes new stable tags only after
  the verified native packages and both desktop payloads upload. The
  signed fixture exercises the actual controller and on-disk stage path.
  Real signed installer apply still requires the lead's acceptance.
- Pending lead verification. The five Rust endpoint listener tests and
  the Go dashboard browser listener test hit a sandbox bind restriction.
  Run `cargo test --offline --locked --features custom-protocol` from the
  desktop crate and `go test ./internal/wsdashboard -run '^TestDesktopUpdateBrowser$' -count=1 -v`
  from the tooling module, with Playwright configured for the browser run.
  The evaluation signing and non-publishing CI commands and the native
  installer, channel, migration, bootstrapper, failure and recovery checklist
  are in `agents-plugin-tool/desktop/RELEASE.md`. Phase 3 awaits that evidence.

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
   opt-in and stable return. From the oldest shipped Electron release and
   a canary Electron, verify the update button swaps to Tauri and relaunches.
   On Windows verify first migration launch installs exactly the bundled
   version, including Electron canary Back to stable with a newer Tauri
   canary already installed; verify the per-user marker, including a second
   Windows account launching the same portable launcher folder, and separately verify routine
   old-shortcut launches preserve a later Tauri self-update. Verify the
   installer installs WebView2 on a machine or VM without it. From a populated
   Electron profile verify Tauri first launch uses default window settings and
   the installed build's channel, and the retained profile reopens after
   reinstalling Electron. Separately exercise a failed file move and its
   Electron swap-helper rollback, post-swap installer failure, and
   first-launch failure; for post-swap failures verify retry or Electron
   reinstall with the retained profile. Record the no-warning residual for
   older Electron versions.
6. **Porting cost.** Record the native changes and remaining gaps for
   bundled daemon autostart and breakaway, existing-daemon attach,
   DevTools, request/navigation guards, cookie injection, and the
   Python-missing guide.

Windows pass list from pre-study Section D:

- **(a)** Standalone daemon start, breakaway-denied retry and cookie injection
  succeed. The daemon survives X-close independently of job teardown; the
  fallback daemon survives only while its containing job lives.
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
