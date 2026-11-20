---
title: "데스크톱 카나리 릴리스 채널 추가"
---

# Add a desktop canary release channel

## Background

Owner decision **(1910)=a**, 2026-11-20. Preserve the owner's intent verbatim:

> 업데이트 확인 버전 옆에 작게 새 모양의 'Canary 빌드로 업데이트' 같은 버튼을 둬서 제가 테스트 용도로 쓰려고 하는데 ... 설치가 좀 쉽게요

The accepted product scope is a desktop-only canary channel in the existing
repository and release workflow. It must make an owner-operated prerelease easy
to install without changing ordinary stable updates.

Current source establishes the implementation baseline:

- The tag workflow accepts every `v*` tag, builds the tag string into native
  assets, creates or reuses one GitHub release, and uploads native assets
  (`.github/workflows/ws-mcp-release.yml:3-6,122-152`). The desktop jobs already
  run for every tag and publish both platform zips plus one combined
  `Werkschaft-desktop-SHA256SUMS` to that same release
  (`.github/workflows/ws-mcp-release.yml:190-195,286-356`).
- Desktop assembly validates the binary metadata's numeric core against the
  package contract. Its parser deliberately strips a suffix before comparing
  all three core components, so metadata `X.Y.Z-canary.N` already passes a
  contract whose `plugin_version` is `X.Y.Z`
  (`agents-plugin-tool/desktop-electron/daemon-selection.js:23-29,103-127`).
  However, the packaged Electron app currently takes its visible/update
  version from `contract.plugin_version`, dropping the metadata suffix
  (`agents-plugin-tool/desktop-electron/scripts/build.cjs:59-64`).
- Stable update parsing accepts numeric `x.y.z` only, rejects prerelease
  releases, and checks GitHub's `/releases/latest` endpoint
  (`agents-plugin-tool/desktop-electron/update.js:23-32,45-59,418-448`). The
  download, checksum verification, extraction, stage, and detached swap path is
  already centralized and must remain the only installation path
  (`agents-plugin-tool/desktop-electron/update.js:458-536`).
- The existing desktop update control is in the Settings card: dashboard code
  inserts `Check for updates`, wires it to the preload bridge, and renders the
  update state beside it
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:904-943`).
  The bridge and main process currently expose only the stable check/download/
  skip/restart channels (`agents-plugin-tool/desktop-electron/dashboard-preload.js:16-26`,
  `agents-plugin-tool/desktop-electron/main.js:14-16,67-94`).
- The desktop app selects and starts its bundled native daemon while connecting
  (`agents-plugin-tool/desktop-electron/main.js:340-407`). A newer numeric core
  automatically replaces the machine's current shared daemon; suffixes never
  order equal cores (`agents-plugin-tool/desktop-electron/daemon-controller.js:75-98`,
  `agents-plugin-tool/desktop-electron/daemon-version.js:1-15`).
- Stable shipping is documented as a `develop` -> release branch -> `main` ->
  stable tag flow, and desktop assets are currently described as taking the
  package contract version (`ai-docs/ship/ws.md:3-9,159-190,216-220,223-276`).
- The repository's `origin` is `git@github.com:kang-sw/werkschaft.git`, but
  three runtime contracts still name `kang-sw/devenv`
  (`agents-plugin/runtime.json:6`, `agents-plugin-wsflow/runtime.json:6`,
  `agents-plugin-pi/runtime.json:6`). The Settings footer repeats that stale
  owner/name and is pinned to the runtime contract by a test
  (`agents-plugin-tool/internal/wsdashboard/pages.go:392-397,436-439`,
  `agents-plugin-tool/internal/wsdashboard/settings_test.go:51-65`). Four
  Codex/Claude plugin manifests also retain the stale outbound repository URL
  (`agents-plugin/.codex-plugin/plugin.json:8`,
  `agents-plugin/.claude-plugin/plugin.json:8`,
  `agents-plugin-wsflow/.codex-plugin/plugin.json:8`,
  `agents-plugin-wsflow/.claude-plugin/plugin.json:8`). Live HTTP verification
  on 2026-11-20 found that the old repository page, `/releases/latest`, the
  tested `v0.50.7` release-asset path, and the old GitHub API repository route
  all redirect successfully to `kang-sw/werkschaft`; the stale values are
  harmless today but rely on GitHub's rename redirect and remain incorrect.

## Decisions

1. **Desktop-only channel.** Implement owner option **a**: a
   `vX.Y.Z-canary.N` tag publishes a GitHub prerelease in the existing
   `kang-sw/werkschaft` repository and the desktop app offers an explicit
   canary install action. Rejected **b**: also add a plugin `install.sh` or
   launcher canary channel; it expands the test channel into every harness and
   changes plugin bootstrap behavior. Rejected **c**: create a separate canary
   repository; it duplicates release infrastructure and asset authority.

2. **Release publication contract.** For a tag exactly matching
   `vX.Y.Z-canary.N` (`X`, `Y`, `Z`, and `N` are non-negative decimal integers),
   create the GitHub release with both `--prerelease` and `--latest=false`, and
   upload the same native and desktop payloads as a stable tag, including
   `Werkschaft-darwin-arm64.zip`, `Werkschaft-win32-x64.zip`, and
   `Werkschaft-desktop-SHA256SUMS`. Stable tags preserve their existing release
   creation, latest-release eligibility, asset names, and publishing behavior.
   Rejected: make every non-stable-looking `v*` tag a canary; the accepted tag
   grammar must be explicit and testable.

3. **Stable automatic checks, manual canary checks.** Scheduled and ordinary
   manual `Check for updates` calls continue to use `/releases/latest` and
   select stable releases only. The new `Canary 빌드로 업데이트` action reads
   the release list, considers only non-draft releases whose tag exactly
   matches `vX.Y.Z-canary.N`, selects the highest eligible canary under Decision
   4, and sends it through the existing asset selection, checksum verification,
   extraction, staging, and detached swap path. Rejected: let an automatic
   check install or prompt for canaries.

4. **Version ordering.** Compare versions as SemVer for this bounded grammar:
   compare `X`, then `Y`, then `Z` numerically; at the same core,
   `X.Y.Z-canary.N < X.Y.Z`, and two canaries compare `N` numerically. A higher
   core outranks a lower core regardless of channel. Therefore stable
   `X.Y.Z` automatically replaces installed `X.Y.Z-canary.N`, while stable
   `X.Y.(Z-1)` does not replace it. A canary is offered only when it is newer
   than the installed version by this ordering.

5. **UI placement and capability boundary.** Add one small, visually distinct
   `Canary 빌드로 업데이트` button immediately beside the existing Settings
   `Check for updates` control, not as a replacement for the footer version or
   stable action. Extend the preload/main IPC boundary with a dedicated manual
   canary-check action. The dashboard must feature-detect that action and hide
   the button when attached through an older desktop shell; an older dashboard
   must ignore the extra bridge member. Rejected: overload the stable check
   action with a channel flag visible to arbitrary renderer input.

6. **Accepted shared-daemon consequence.** The owner's test use accepts that a
   canary desktop whose numeric core is higher than the running daemon within
   the same minor replaces the machine's shared daemon on the next desktop
   connection. Returning the Electron app from `X.Y.Z-canary.N` to stable
   `X.Y.Z` does **not** automatically replace an already-running canary daemon:
   daemon ordering ignores suffixes, so equal-core ordinary connection retains
   the healthy incumbent. Stable app bytes take effect immediately after the
   updater swap; stable daemon bytes take effect when the canary daemon exits
   and the stable app starts one, when the user invokes the existing explicit
   `Replace daemon (same core)` action, or when a later stable app carries a
   higher numeric core. Rejected: silently force an equal-core daemon swap as
   part of every stable app update.

7. **Ship documentation.** Add a separate canary procedure to
   `ai-docs/ship/ws.md`. It must cover tag/core preparation, review and
   compatibility gates, the prerelease/latest flags, artifact verification,
   installation through the new manual action, stable-return verification, and
   cleanup. It must not weaken or silently reuse the stable procedure's
   `develop` -> `main` publication authority.

8. **Repository identity side fix.** Change every shipped outbound repository
   identity found in this audit from `kang-sw/devenv` to
   `kang-sw/werkschaft`: the three `runtime.json` `release_repository` values,
   the four Codex/Claude plugin manifest `repository` URLs, and the dashboard
   Settings footer constant. Keep the existing pinning test, expanded as needed
   to cover the package contracts/manifests. Do not rename Go module import
   paths or historical documentation: those are code/history identities, not
   release destinations. Rejected: rely indefinitely on the currently working
   GitHub redirect.

9. **Compatibility tiers.** Apply `ai-docs/manuals/release-compatibility.md`
   directionally and record these verdicts in the implementation result:

   - **Release feed/publication: T0 transparent.** Stable tags and
     `/releases/latest` remain unchanged, while pre-feature desktops both query
     only that endpoint and reject a prerelease response
     (`agents-plugin-tool/desktop-electron/update.js:45-59,418-448`).
   - **Desktop preload/IPC addition: T0 transparent**, conditional on Decision
     5's two-way capability gating. The new member is additive; a new dashboard
     paired with an old shell hides the action, while the current dashboard
     already ignores bridge members it does not call
     (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:913-936`).
   - **Desktop update state: T0 transparent**, provided the implementation
     reuses the existing `update-state.json`/`staged.json` shapes, preserves
     unknown root fields through the existing merge-write behavior, and proves
     a pre-feature updater can ignore a canary-valued entry without deleting a
     verified stage or breaking stable checks
     (`agents-plugin-tool/desktop-electron/update.js:390-415`). A schema change
     or destructive old-writer behavior requires a fresh directional grade.
   - **Release repository value: T0 transparent.** Each launcher reads its own
     package contract and constructs a download URL; no shared store is changed
     (`agents-plugin/bin/ws-mcp-launcher.py:377-385`). The old URL also redirects
     successfully today, so old packages continue to work.
   - **Daemon channel label: T0 transparent by itself.** Current selection
     strips suffixes and keeps the same protocol/store surfaces
     (`agents-plugin-tool/desktop-electron/daemon-selection.js:23-29,103-127`).
     This does not grade the canary's payload: the canary procedure must state
     the highest tier of its actual changes with source-cited old/new read and
     write paths. T3 still requires a minor core and explicit owner approval;
     T4 must not ship. A canary label never waives those gates.

## Constraints

- Ticket-only scope ends with this plan. Implementation must read and obey
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/performance.md` before editing their matching paths.
- Preserve the existing release asset names and the one combined desktop SUMS
  format because the updater and release workflow already share those names
  (`agents-plugin-tool/desktop-electron/update.js:13-17,34-59`,
  `.github/workflows/ws-mcp-release.yml:317-356`).
- Do not add a second downloader, checksum path, extraction path, staging
  directory, updater state store, repository, or plugin/launcher channel.
- The canary check is user-triggered network work. It must perform no periodic
  polling and no extra file I/O before the user action; record before/after file
  operations if implementation changes the existing staged-update writes.
- Preserve the renderer trust boundary: expose a bounded payload-free action,
  keep release filtering and URL authority in the main process, and retain the
  existing HTTPS/loopback URL policy (`agents-plugin-tool/desktop-electron/update.js:62-89`,
  `agents-plugin-tool/desktop-electron/dashboard-preload.js:1-26`).
- Resolve every item in the Open Decision Queue before promotion to `ready/`.

## Open Decision Queue

Last ID used: **(1914)**.

### (1911) [open] [policy] Prepare a canary on an ephemeral release branch with the next target patch core

- **Option a (recommended):** cut an ephemeral canary release branch from a
  reviewed `develop` commit, apply the normal version bump there so the package
  contracts use the next target `X.Y.Z`, tag it `vX.Y.Z-canary.N`, and do not
  promote `main` or merge the canary-only version commit back. This makes the
  canary newer than the installed stable core while preserving stable version
  ownership.
- **Option b:** bump `develop` to `X.Y.Z` before the stable ship. This makes the
  canary flow simpler but changes the settled stable version strategy, where
  each ship owns its bump (`ai-docs/ship/ws.md:11-31`).
- **Option c:** tag a canary at the already-released stable core. This passes
  assembly but cannot be offered over stable `X.Y.Z`, because Decision 4
  correctly orders `X.Y.Z-canary.N` below `X.Y.Z`.

Recommendation: **a**, because it satisfies updater ordering and manifest-core
validation without changing stable release authority.

### (1912) [open] [policy] Defer an explicit “Back to stable” action

- **Option a (recommended):** rely on the automatic stable check once stable
  `X.Y.Z` outranks `X.Y.Z-canary.N`; document the existing same-core daemon
  consequence from Decision 6.
- **Option b:** add a separate `Back to stable` action in this ticket, including
  its UI, release-list selection, and tests.

Recommendation: **a** for the first slice; the owner called this action
optional, and automatic SemVer return already provides the required path.

### (1913) [open] [technical] Enumerate all GitHub release-list pages and choose the highest eligible canary

- **Option a (recommended):** use GitHub's REST releases list, follow pagination
  to exhaustion with a bounded response size per page, filter exact canary tags
  and required assets, then compare candidates with Decision 4.
- **Option b:** inspect only the first/default page and trust API order. This is
  simpler but can miss an eligible canary after enough releases.
- **Option c:** use GraphQL or scrape the releases page. Neither matches the
  existing REST/JSON updater.

Recommendation: **a**, because “newest” should be a semantic selection rather
than an assumption about one response page.

### (1914) [open] [technical] Package the Electron app with validated binary metadata version including the canary suffix

- **Option a (recommended):** after `validateDaemonRoot` proves metadata core
  equals the runtime contract core, set Electron `appVersion`/`buildVersion`
  from `selected.metadata.version`; keep `contract.plugin_version` as the
  package compatibility core.
- **Option b:** add a separate channel/version file to the app and keep Electron
  at the stable core. This creates a second release identity and complicates
  updater/state comparisons.

Recommendation: **a**, because the binary metadata is already stamped from the
tag by `build-release-assets.sh` and validated before packaging
(`agents-plugin-tool/scripts/build-release-assets.sh:7-16,37-53`,
`agents-plugin-tool/desktop-electron/daemon-selection.js:103-127`).

## Phases

### Phase 1: Publish canary releases and align release identity

Resolve (1911) and (1914), then:

- Update `.github/workflows/ws-mcp-release.yml` so exact canary tags create a
  prerelease with `--prerelease --latest=false`, while stable tag behavior is
  byte-for-byte equivalent in intent. Prove both native and desktop publish
  steps target the same release and retain the two zips plus combined SUMS.
- Preserve numeric-core validation between tag-stamped binary metadata and the
  runtime contract, and preserve the canary suffix as the packaged app's update
  identity according to the settled (1914) answer. Add focused assembly/
  packaging contract tests for matching canary core, mismatched core refusal,
  suffix preservation, and unchanged stable packaging.
- Add the canary procedure to `ai-docs/ship/ws.md` according to the settled
  (1911) answer. Include tag collision/counter handling, reviewed commit and
  compatibility gates, workflow monitoring, asset/SUMS verification, install
  acceptance, stable return, and cleanup. Keep the stable flow unchanged.
- Change the eight audited outbound release/repository identities in Decision
  8 and extend drift tests so ws, wsflow, Pi, and dashboard destinations cannot
  diverge again. Verify the old redirect but make no runtime dependency on it.

Verification: workflow/static tests cover stable and canary classification;
desktop assembly/packaging tests cover core/suffix behavior; package/runtime
contract tests cover every repository destination; doc review compares stable
ship semantics before and after.

### Phase 2: Select and install canaries through the existing updater

Depends on Phase 1. Resolve (1912) and (1913), then:

- Replace the numeric-only helper with the bounded stable/canary SemVer model
  from Decision 4 without admitting arbitrary prerelease grammars.
- Keep stable `check()` on `/releases/latest`. Add a separately invoked canary
  list check, exact filtering, deterministic highest-version selection, and
  required-asset/SUMS validation. Feed the result into the existing
  `available`/download/verify/stage/swap state machine instead of duplicating it.
- Preserve skip, staged, apply-failure, restart, download-page, URL-policy, and
  Windows helper behavior for both channels. Make status payloads distinguish
  stable from canary only where the UI needs it; feature-gate any additive
  field.
- Test the complete ordering matrix, including numeric `N` (`canary.10` >
  `canary.9`), stable-over-same-core-canary, higher-core canary-over-stable,
  drafts/malformed tags/missing assets, list pagination, checksum failure,
  staging/restart, stable automatic return, and the pre-feature state
  compatibility case from Decision 9.

Verification: updater unit/integration tests exercise local stable and releases
list feeds and assert the same download, checksum, extraction, staging, and
swap functions serve both channels. No live release is created by tests.

### Phase 3: Add the canary update control

Depends on Phase 2. This UI phase must be executed by a **Claude Code worker
(not Codex)** and independently reviewed after implementation.

- Add the small, distinct `Canary 빌드로 업데이트` button beside the existing
  Settings `Check for updates` control. Retain keyboard access, focus styling,
  localization, narrow-window layout, light/dark themes, and browser/older-shell
  hiding behavior.
- Add the bounded preload/main IPC action and route it only from the pinned
  dashboard main frame, following the existing update channel admission.
- Extend the existing desktop update browser fixture and main/preload tests for
  feature detection, one-click invocation, status/toast behavior, and absence
  in a browser or older shell.
- Capture **before and after screenshots** at the same desktop viewport in both
  light and dark themes, with the Settings card open and the version/update
  controls visible. The phase result must link the captures and state the
  visual comparison outcome.

Verification: automated dashboard browser and Electron main/preload tests pass;
the named non-Codex executor records before/after screenshot evidence and an
independent reviewer checks placement, distinction, localization, and regressions.
