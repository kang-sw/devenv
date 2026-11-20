---
title: "데스크톱 카나리 릴리스 채널 추가"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 065b18fcc5b5338b
sage-review-design-reviewed: 065b18fcc5b5338b
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
   the release list according to Decision 12, then sends the selected canary
   through the existing asset selection, checksum verification, extraction,
   staging, and detached swap path. Rejected: let an automatic check install
   or prompt for canaries.

4. **Version ordering.** Compare versions as SemVer for this bounded grammar:
   compare `X`, then `Y`, then `Z` numerically; at the same core,
   `X.Y.Z-canary.N < X.Y.Z`, and two canaries compare `N` numerically. A higher
   core outranks a lower core regardless of channel. Therefore stable
   `X.Y.Z` automatically replaces installed `X.Y.Z-canary.N`, while stable
   `X.Y.(Z-1)` does not replace it. A canary is offered only when it is newer
   than the installed version by this ordering. The explicit stable return in
   Decision 11 overrides the newer-version gate for its selected stable.

5. **UI placement and capability boundary.** Add one small, visually distinct
   `Canary 빌드로 업데이트` button immediately beside the existing Settings
   `Check for updates` control, not as a replacement for the footer version or
   stable action. Show a `Back to stable` control only while the packaged app is
   running a canary. Extend the preload/main IPC boundary with dedicated,
   payload-free manual canary-check and stable-return actions. The dashboard
   must feature-detect each action and hide its control when attached through
   an older desktop shell; an older dashboard must ignore the extra bridge
   members. Rejected: overload the stable check action with a channel flag
   visible to arbitrary renderer input.

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
   higher numeric core. For an explicit return to a **lower-core** stable app,
   a healthy higher-core canary daemon stays running and the stable app attaches
   to it; even `Replace daemon (same core)` cannot force that downgrade.
   `agents-plugin-tool/desktop-electron/daemon-controller.js:91-99` returns on
   a lower candidate regardless of `replace`, and
   `agents-plugin-tool/internal/wsdashboard/ensure.go:209-217` keeps a higher
   running version under `CompareVersions`. If that daemon exits, the stable
   app can start its bundled stable daemon on its next connection. The
   stable-return action changes the desktop installation, never drains or
   downgrades the shared daemon. Rejected: silently force an equal- or
   lower-core daemon swap as part of a stable app update.

7. **Ship documentation.** Add a separate canary procedure to
   `ai-docs/ship/ws.md`. It must cover tag/core preparation, review and
   compatibility gates, the prerelease/latest flags, artifact verification,
   installation through the new manual action, explicit stable-return and
   shared-daemon verification, and cleanup. It must not weaken or silently
   reuse the stable procedure's
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
     5's two-way capability gating. The new members are additive; a new dashboard
     paired with an old shell hides the actions, while the current dashboard
     already ignores bridge members it does not call
     (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:913-936`).
   - **Desktop update state: T0 transparent**, provided the implementation
     retains the existing `update-state.json`/`staged.json` required fields,
     preserves unknown root fields through the existing merge-write behavior,
     and proves a pre-feature updater can ignore a canary-valued entry without deleting a
     verified stage or breaking stable checks. The new updater must also apply
     an explicitly chosen lower-version stable stage despite the current
     `isNewer` guards in `staged()`/`apply()`; persist the explicit selection
     intent with the verified stage if needed and prove old/new reopen behavior
     in both directions (`agents-plugin-tool/desktop-electron/update.js:390-415,515-536`). A schema
     change or destructive old-writer behavior requires a fresh directional
     grade; this T0 verdict is conditional, not permission to discard a stage.
   - **Explicit stable return: T0 transparent for shared surfaces**, conditional
     on the state and IPC gates above. It downloads the existing stable release
     through the same asset and swap path and changes only the desktop app;
     `agents-plugin-tool/desktop-electron/update.js:2-4,458-536` leaves the
     daemon running, and `agents-plugin-tool/desktop-electron/daemon-controller.js:91-99`
     prevents a lower-core daemon replacement. A returned desktop may attach
     to a higher-core daemon, so the canary payload still requires its own
     directional compatibility verdict under the final bullet below, including
     any minor-version boundary.
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

10. **Canary branch and version ownership (1911).** Owner (1911) answer,
    2026-11-20: choose **a**. Cut an ephemeral canary release branch from a
    reviewed `develop` commit, apply the normal bump to the next target patch
    core there, and tag `vX.Y.Z-canary.N`. Do not promote `main` or merge back
    the canary-only version commit. Rejected **b**: bump `develop` ahead of the
    stable ship, changing the stable version strategy. Rejected **c**: reuse an
    already-released stable core, which Decision 4 orders above its canary.

11. **Explicit stable return (1912).** Owner (1911) answer, 2026-11-20:
    choose **b**. While running a canary, expose `Back to stable`; it selects
    the current stable from `/releases/latest` and explicitly installs it even
    when that version orders below the installed canary. It uses the same
    download, checksum verification, extraction, stage, and detached swap path
    as other desktop updates. Automatic stable checks keep their normal
    newer-version rule; only this user action permits a downgrade. Decision 6
    governs the bundled daemon after the desktop swap. Rejected **a**: defer
    the action and rely solely on a later stable release outranking the canary;
    the owner wants a direct return button.

12. **First page holding an eligible release (1913).** Owner (1911) answer,
    2026-11-20: walk the GitHub REST releases list newest-first, page by page,
    and stop at the first page holding an eligible release: non-draft, tag
    exactly `vX.Y.Z-canary.N`, platform zip and combined SUMS assets
    present (selection within that page is below). Then apply Decision 4 to decide whether to offer it; a candidate
    that is not newer ends the check without scanning older releases. Bound
    the walk at 10 pages of 30 releases and 1 MiB per response; report no
    eligible canary within the cap rather than silently claiming the full
    history was searched. This assumes API order is newest-first by release `created_at` (the tag
    commit time, not publication time); therefore selection follows creation
    recency, not the greatest SemVer, and an older-created higher SemVer can be
    skipped. GitHub's [REST release docs](https://docs.github.com/en/rest/releases/releases)
    describe list pagination and the `created_at` meaning for latest releases
    but do not guarantee the list's order. To stay correct without that
    guarantee, selection is page-local: within the first page that holds any
    eligible canary, take the highest canary by Decision 4 among that page's
    eligible entries, and stop there. Later pages are read only when earlier
    pages hold no eligible canary. The cross-page newest-first premise is
    recorded as an assumption; a test pins page-local highest selection
    with an out-of-order page (lead technical decision, auto-confirmed from the
    fact-populator decision gap, 2026-11-20). Rejected for this gap: sorting
    across all pages (contradicts the owner's early stop) and trusting the first
    entry of a page (an out-of-order page would pick a lower canary).
    Rejected: exhaustive pagination and highest-version selection, because
    reading accumulated history is wasteful; first-page-only selection, which
    can miss an eligible canary; GraphQL or page scraping, which would add a
    second feed format.

13. **Packaged canary identity (1914).** Owner (1911) answer, 2026-11-20:
    choose **a**. After `validateDaemonRoot` confirms the metadata core equals
    the runtime contract core, set Electron `appVersion`/`buildVersion` from
    `selected.metadata.version`, retaining the `-canary.N` suffix. Keep
    `contract.plugin_version` as the package compatibility core. Rejected
    **b**: a second channel/version file with Electron left at the stable
    core; it creates a second identity and complicates updater comparisons.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin-wsflow/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
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
- Design review notes (2026-11-20), binding on planning:
  - The desktop replaces any older numeric daemon core, across minors too
    (`desktop-electron/daemon-controller.js:95-98`), so Decision 6's
    same-minor wording covers only the routine patch-bump canary. A
    minor-core canary (Decision 9's T3 case) would drain the shared daemon
    across minors, previous-minor plugin sessions fail fast, and Back to
    stable cannot restore the daemon. The Phase 1 ship procedure states this
    consequence and its recovery (an explicit daemon shutdown), or forbids
    minor-core canaries.
  - Phase 1's win32 packaging test covers a suffixed `buildVersion`. If
    rcedit or the macOS bundle fields reject the suffix, `buildVersion`
    stays at the numeric core. The updater identity comes from the staged
    package.json `version` (`desktop-electron/scripts/build.cjs:62-63`), so
    Decision 13 still holds.
  - Phase 2 planning names how canary handling treats the single `available`
    slot, the persisted `skipped` value, and the auto-acknowledged
    `applyFailure` for a not-newer version (`desktop-electron/update.js:392`,
    `:406`, `:440-455`, `:584`). It also names the main-process signal that
    the running app is a canary, which shows Back to stable.
  - App ordering (canary below stable, Decision 4) is the reverse of daemon
    `CompareVersions` and the launcher's comparison for equal cores
    (`internal/wsdashboard/version.go:8-20`, `ws-mcp-launcher.py:1114-1131`). Equal
    cores take the separate keep-running path, so this is harmless. The ship
    doc and tests must not claim one ordering across app and daemon.

## Prior Decisions

- 424ff4e7 (2026-11-16, commit): "Owner decision (1224)=a selects release-workflow desktop builds. Reuse ws-mcp-assets rather than rebuild binaries in desktop jobs so native and bundled daemon metadata have one source of truth." — bearing: supports
- 9db551d0 (2026-11-16, commit): "Owner decision (1210) makes wsflow the bundled default; retain an explicit ws choice without introducing a CI workflow." — bearing: constrains
- 21aa53f9 (2026-11-15, commit): "T0 both ways: an old daemon's page ignores the bridge (no toast); a new page in a browser or an older shell has no bridge (no row). No route, stored shape, MCP or protocol change." — bearing: supports
- d1865da5 (2026-11-15, commit): "The page opens via window.open, which the shell's existing setWindowOpenHandler classifies as external and hands to openExternal; no new preload bridge method or IPC channel." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | .github/workflows/ws-mcp-release.yml, agents-plugin-tool/desktop-electron/update.js, agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js, ai-docs/ship/ws.md, package runtime and manifest files |
| scope.surface | public-interface | Settings controls, desktop preload IPC, GitHub release assets, and runtime repository contracts in Decisions 2, 5, and 8 |
| scope.new_public_symbol | yes | new canary-check and stable-return actions on the renderer-visible preload bridge in Decision 5 |
| scope.new_type_contract | yes | additive update status and persistent explicit-selection intent across old/new updater state in Decisions 5 and 9 |
| scope.test_surface | existing | agents-plugin-tool/desktop-electron/test/update.test.js, agents-plugin-tool/desktop-electron/test/main.test.js, agents-plugin-tool/internal/wsdashboard/testdata/desktop-update.browser.cjs |
| complexity.reuse_points | confirmed | existing download, checksum, extraction, staging, and detached swap in agents-plugin-tool/desktop-electron/update.js#L458-L536 |
| complexity.side_effect_risk | high | canary app can replace the shared daemon; explicit stable return may retain a higher-core daemon |
| risk.correctness | high | version ordering, bounded feed selection, checksum verification, and downgrade staging must agree |
| risk.fit | moderate | release workflow, package identity, updater, IPC, dashboard, and ship procedure change together |
| risk.test | high | cross-platform packaged install, restart/reopen, and old/new state compatibility require verification |
| risk.security_or_contract | high | renderer IPC and release URL authority cross a trust boundary; shared daemon compatibility remains directional |

## Phases

### Phase 1: Publish canary releases and align release identity

Implement Decisions 10 and 13:

- Update `.github/workflows/ws-mcp-release.yml` so exact canary tags create a
  prerelease with `--prerelease --latest=false`, while stable tag behavior is
  byte-for-byte equivalent in intent. Prove both native and desktop publish
  steps target the same release and retain the two zips plus combined SUMS.
- Preserve numeric-core validation between tag-stamped binary metadata and the
  runtime contract, and preserve the canary suffix as the packaged app's update
  identity according to the settled (1914) answer. Add focused assembly/
  packaging contract tests for matching canary core, mismatched core refusal,
  suffix preservation, and unchanged stable packaging.
- Add the canary procedure to `ai-docs/ship/ws.md` using an ephemeral release
  branch, next target patch core, and no `main` promotion or merge-back of the
  canary-only bump. Include tag collision/counter handling, reviewed commit and
  compatibility gates, workflow monitoring, asset/SUMS verification, install
  acceptance, explicit stable return, shared-daemon behavior, and cleanup. Keep
  the stable flow unchanged.
- Change the eight audited outbound release/repository identities in Decision
  8 and extend drift tests so ws, wsflow, Pi, and dashboard destinations cannot
  diverge again. Verify the old redirect but make no runtime dependency on it.

Verification: workflow/static tests cover stable and canary classification;
desktop assembly/packaging tests cover core/suffix behavior; package/runtime
contract tests cover every repository destination; doc review compares stable
ship semantics before and after.

### Result (58119168e) - 2026-11-21

- Exact `vX.Y.Z-canary.N` tags now create prereleases excluded from latest;
  stable release creation and the shared native/desktop asset upload path are
  unchanged. Packaging retains the validated suffix in Electron `appVersion`
  and staged `package.json`; the native `buildVersion` stays at the validated
  numeric contract core under the binding design-review fallback.
- The separate canary ship procedure uses an ephemeral reviewed release branch,
  a next-patch core, no `main` promotion or merge-back, compatibility grading,
  artifact/SUMS and install/return checks, and forbids a minor-core canary
  without the separately approved T3 boundary. Stable ship instructions remain
  unchanged.
- All eight audited outbound repository identities now name
  `kang-sw/werkschaft`, with drift coverage across ws, wsflow, Pi, both plugin
  manifest formats, and the dashboard. Read-only HTTP checks confirmed the old
  repository, latest-release, historical asset, and API URLs still redirect.
- Compatibility: release publication and repository identity are T0; the
  daemon channel label is T0 by itself, while each canary payload still needs
  its own directional compatibility verdict. The numeric native build field is
  a packaging compatibility decision; the suffixed package identity remains
  the updater authority.
- Verification passed: desktop Node suite (178 pass, 4 Windows-only skips),
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsrsrc -run '^TestCIRelease' -count=1`,
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run '^TestSettingsGitHubLinkMatchesReleaseDestinations$' -count=1`,
  and `python3 -m unittest discover agents-plugin-wsflow/tests -p 'test_wsflow_runtime_contract.py'` (3 pass).

### Phase 2: Select canaries and return to stable through the existing updater

Depends on Phase 1. Implement Decisions 11 and 12:

- Replace the numeric-only helper with the bounded stable/canary SemVer model
  from Decision 4 without admitting arbitrary prerelease grammars.
- Keep stable `check()` on `/releases/latest`. Add a separately invoked canary
  list check that walks newest-first across bounded pages and stops at the
  first page holding a non-draft exact-tag canary with the required platform
  zip and SUMS, selecting the highest such canary on that page (Decision 12).
  Check its version only after selection; if it is not newer, return `current`
  without scanning farther. Enforce Decision 12's page and byte caps and
  distinguish a cap-limited miss from a full-list miss. Feed the result into the existing
  `available`/download/verify/stage/swap state machine instead of duplicating it.
- Add a separately invoked `Back to stable` selection from `/releases/latest`.
  Accept the selected stable as an explicit downgrade when it orders below the
  running canary, carry that intent through staging and restart/reopen, and use
  the same verified download/stage/swap path. Keep ordinary stable checks and
  automatic updates subject to the newer-version rule; do not drain or replace
  the shared daemon during the desktop swap.
- Preserve skip, staged, apply-failure, restart, download-page, URL-policy, and
  Windows helper behavior for both channels and explicit stable return. Make
  status payloads distinguish the selected action only where the UI needs it;
  feature-gate any additive field. Recheck Decision 9's directional state/IPC
  tier if persistent downgrade intent changes a store shape or an old writer
  can discard it.
- Test the complete ordering matrix, including numeric `N` (`canary.10` >
  `canary.9`), stable-over-same-core-canary, higher-core canary-over-stable,
  drafts/malformed tags/missing assets, early stop within a page and across
  pages, bounded-page exhaustion, creation-order versus SemVer disagreement,
  checksum failure, staging/restart, stable automatic return, explicit
  back-to-stable downgrade with restart/reopen, lower-core daemon retention,
  and the pre-feature state compatibility case from Decision 9.

Verification: updater unit/integration tests exercise local stable and releases
list feeds and assert the same download, checksum, extraction, staging, and
swap functions serve canary install and stable return. No live release is
created by tests.

### Result (58119168e) - 2026-11-21

- The updater now accepts only bounded stable or `-canary.N` versions, applies
  the settled SemVer ordering, walks at most ten 30-release/1 MiB pages, picks
  the highest eligible canary on the first eligible page, and distinguishes a
  cap-limited miss. Scheduled/latest checks remain stable-only.
- Manual canary and stable-return selections share the existing single
  available/download/checksum/extract/stage/swap path. A scheduled stable check
  cannot erase a pending manual selection. Stable return is available only
  from a canary and carries explicit downgrade intent through reopen without
  touching the shared daemon; existing daemon tests retain a healthy
  higher-core incumbent.
- Persistent channel stages keep the original numeric `staged.json.version`
  gate usable by pre-feature updaters and add `targetVersion` plus `action` for
  the real new-updater identity. Ordinary stable staging still performs the
  same single `staged.json` write with `{version, app}`; a channel action still
  performs one write, now with the additive fields. No file I/O or polling was
  added before the user invokes a manual action, and unknown root state fields
  continue through merge writes.
- Compatibility: desktop update state and explicit stable return are T0 in both
  directions under the numeric legacy gate and additive fields. The updater
  process API is additive; Phase 3 still owns the payload-free IPC and two-way
  dashboard capability gates needed to complete that T0 verdict.
- The allocated lite review found three issues: the old-updater proof used new
  code, scheduled checks could clear manual selection, and mocked packaging
  could not establish native suffix acceptance. All three were corrected once;
  per lite policy no second review round was run.
- Verification passed: focused packaging/updater tests (31 pass) and the full
  desktop Node suite (178 pass, 4 Windows-only skips), including ordering,
  malformed/draft/asset filtering, page stop/cap behavior, shared checksum
  failure handling, reopen/downgrade intent, pre-feature state, and the existing
  never-downgrade daemon matrix. No Electron process, tag, release, or push was
  created.

### Phase 3: Add canary and stable-return controls

Depends on Phase 2. This UI phase must be executed by a **Claude Code worker
(not Codex)** and independently reviewed after implementation.

- Add the small, distinct `Canary 빌드로 업데이트` button beside the existing
  Settings `Check for updates` control. Retain keyboard access, focus styling,
  localization, narrow-window layout, light/dark themes, and browser/older-shell
  hiding behavior.
- Add `Back to stable` beside the update controls only while the packaged app
  is running a canary. Present the current stable target and make the explicit
  downgrade clear before install; hide the control after stable relaunch.
- Add bounded, payload-free preload/main IPC actions for canary check and stable
  return; route them only from the pinned dashboard main frame, following the
  existing update channel admission.
- Extend the existing desktop update browser fixture and main/preload tests for
  feature detection, invocation of both actions, downgrade status/toast
  behavior, canary-only visibility of `Back to stable`, and absence in a
  browser or older shell.
- Capture **before and after screenshots** at the same desktop viewport in both
  light and dark themes, with the Settings card open and the version/update
  controls visible. Include canary-running screenshots showing `Back to stable`
  and stable-running screenshots after return showing its absence. The phase
  result must link the captures and state the visual comparison outcome.

Verification: automated dashboard browser and Electron main/preload tests pass;
the named non-Codex executor records before/after screenshot evidence and an
independent reviewer checks placement, canary-only stable-return visibility,
distinction, localization, and regressions.
