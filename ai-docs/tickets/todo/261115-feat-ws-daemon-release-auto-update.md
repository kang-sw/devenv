---
title: "Electron shell: user-approved daemon update from GitHub releases"
related:
  261115-feat-werkschaft-electron-shell: "prerequisite: the update runs in the Electron shell main process (its Phase 1 shell must exist). That ticket carries the shell daemon rule (never replace or stop a daemon) that this ticket gives one user-approved exception"
  261113-feat-werkschaft-thin-tauri-shell: "Decision 2 (daemon ownership, 'revisit when the app self-updates') is the decision this ticket revisits"
  261112-research-ws-ade-desktop-shell-and-rename: "Daemon ownership section records the same policy and its minor-lockout concern"
---

# Electron shell: user-approved daemon update from GitHub releases

## Background

The owner asked (relayed by MAIN, 2026-11-15 at 01:29Z) for the dashboard
daemon to update from GitHub releases: check the release tag, download the
latest release whole into a unique path, then swap daemons. The owner then
scoped it (01:49Z): the update is a feature of the Electron app shell only
("애초에 자동 업데이트는 electron 앱 셸에만 생각하고 있긴 했는데요."), and
nothing swaps without the user's click. Implementation is not authorized
yet; the owner orders this ticket after
`261115-feat-werkschaft-electron-shell`.

Evidence was taken read-only on 2026-11-15 (develop `1b815bf67` worktree);
paths are relative to the repository root.

### Current state

- **Releases.** Repository `kang-sw/devenv`, tags `v<plugin version>`
  (currently `v0.50.0`; `agents-plugin/runtime.json:2-7`,
  `ai-docs/ship/ws.md:28-29`). Pushing the tag runs
  `.github/workflows/ws-mcp-release.yml`, which builds with
  `agents-plugin-tool/scripts/build-release-assets.sh`, verifies and uploads
  `dist/*` (`ws-mcp-release.yml:104-134`). Assets: full `ws-mcp` executables
  `ws-mcp-{darwin-arm64,darwin-amd64,linux-amd64,linux-arm64}` and
  `ws-mcp-windows-{amd64,arm64}.exe`, each with `<asset>.metadata.json`,
  plus one `SHA256SUMS` covering all of them
  (`build-release-assets.sh:22-49`). No detached signature exists.
- **How plugins get the binary.** The launcher downloads the platform asset
  and `SHA256SUMS` from
  `https://github.com/<release_repository>/releases/download/<tag>/<asset>`
  (repository and tag from `runtime.json`, overridable by
  `WS_MCP_RELEASE_REPOSITORY`, `WS_MCP_RELEASE_TAG`,
  `WS_MCP_RELEASE_BASE_URL`) and rejects a digest mismatch
  (`agents-plugin/bin/ws-mcp-launcher.py:180-186,357-382`). `runtime.json`
  pins the tag, not an independent binary digest: integrity comes from the
  release's own `SHA256SUMS`.
- **One daemon per cache root**, held by
  `<cache root>/dashboard/dashboard.lock`
  (`agents-plugin-tool/internal/wsdashboard/daemon.go:303-329`). The
  launcher stages a content-addressed copy at
  `<cache root>/dashboard/bin/ws-mcp-dashboard-<sha256 first 16><ext>` and
  runs `daemon-start --build-time <runtime mtime ms>`
  (`ws-mcp-launcher.py:935-968,1375-1416`).
- **Replacement.** `wsdashboard.Ensure` reads `endpoint.json`, checks
  `/health`, sends authenticated `POST /shutdown` to an older daemon, starts
  the successor and waits for a healthy endpoint with a different token; a
  daemon that refuses shutdown keeps running
  (`internal/wsdashboard/ensure.go:124-159,186-218`). Shutdown drains hosted
  sessions, detaches holders and releases the lock; the successor may bind a
  different port (`daemon.go:303-389,464-479,617-660`). Launchers reconnect
  and replay initialization; already-sent requests fail rather than replay
  (`internal/wsdashboard/proxy.go:18-27,145-168`); holders reattach
  (`internal/wsholder/manager.go:235-264`). A daemon already replaces
  itself this way when the LAN setting changes (`proxyHost.applyLANChange`,
  `agents-plugin-tool/cmd/ws-mcp/proxy.go:164-219`). `Ensure` has no
  rollback, and startup pruning removes older staged executables after a
  grace period (`internal/wsdashboard/spawn.go:69-101`).
- **Version rules** (verified by MAIN and re-checked): the newest version
  wins and nothing downgrades (`ensure.go:164-184`,
  `ws-mcp-launcher.py:1349-1373`); a same-minor launcher attaches to a newer
  daemon; an older-minor launcher gets `DaemonUnavailable` at connect
  (`ws-mcp-launcher.py:1460-1464`) and the daemon rejects cross-minor hellos
  (`proxy.go:251-252`); equal versions fall back to content hash, then build
  time (unknown or older never replaces; build time defaults to the
  executable mtime, `cmd/ws-mcp/dashboard.go:35,49-58`); a release never
  replaces a same-core `-dev` daemon (`ws-mcp-launcher.py:1445-1453`,
  `ensure.go:170-184`). Source builds report `0.50.0-dev`
  (`cmd/ws-mcp/main.go:16-17`). Release metadata publishes no build time
  (`agents-plugin-tool/scripts/write-runtime-metadata.py:13-27`).
- **UI surfaces.** Session pages have a persistent `#dash-banner`
  (`internal/wsdashboard/assets/dashboard.tmpl:9-12`, helper
  `dashboard.js:6517-6522`); `WsToast.show` supports notice styling and a
  click action (`assets/toast.js:2-80`); the index shows the daemon version
  (`dashboard.tmpl:321`). Execution-authority POSTs use `TrustedWrite`
  (loopback viewer plus exact own Origin; `internal/wsdashboard/access.go:162-175`).
- **Network.** The daemon already makes outbound HTTPS calls (plan usage,
  `internal/wsdashboard/plan_usage.go:44-49`); no GitHub release client
  exists.
- **Config.** `dashboard.*` keys are declared in each package's
  `dashboard-config-manifest.json` (`agents-plugin/` and
  `agents-plugin-wsflow/`, same keys, only defaults differ).

## Decisions

Owner decisions (relayed by MAIN, 2026-11-15):

- **(867)=c: no automatic swap.** When a newer patch release (same
  major.minor as the running daemon, higher patch) is found and verified,
  the shell shows a persistent bottom-right toast the user must dismiss,
  "새 패치가 있습니다. 적용할까요?", and swaps only when the user clicks
  apply. Dismiss hides it until a newer release appears. Rejected: swapping
  at the next idle moment with a 24-hour cap; swapping immediately after
  verification. This supersedes the auto-swap half of (860)=a below.
- **(869) withdrawn: the update lives only in the Electron shell.** The
  shell's main process checks the release, downloads, prompts and performs
  the swap. The daemon and the plugin packages gain no autonomous update
  path and no config knob (no `dashboard.auto_update`, no per-package
  default). A dashboard opened in a plain browser shows no update notice.
  The Tauri shell, kept until parity, gets no update feature.
- **(860)=a, as narrowed by (867) and (869): patch versus minor.** A patch
  is offered through the toast above. A newer minor (or major) is offered
  with a warning because an older-minor plugin session is locked out the
  moment a newer-minor daemon runs; see (868). Rejected: treating a minor
  exactly like a patch with no warning.
- **(868)=a: a minor uses the same toast plus one confirmation.** A newer
  minor (or major) release shows the same bottom-right toast; after apply,
  one extra confirmation dialog lists the attached older-minor sessions
  that will disconnect and the plugin-update steps (Claude `/plugin`
  update, Codex plugin refresh), and confirming performs the daemon swap.
  Rejected: a toast that only shows the plugin-update steps and never swaps
  to a new minor.
- **(861) closed: the target is the single global daemon.** The shell
  updates whichever daemon serves its cache root, whoever started it; the
  newest version already wins in the launcher and in `Ensure`, so the
  updated daemon does not fight launcher-started daemons. Rejected premise:
  updating only a daemon the shell itself launched.

Announced defaults relayed by MAIN (still standing after the rescope):

- A `-dev` daemon is never offered an update (a release never replaces a
  same-core `-dev` daemon anyway).
- Swap only after the downloaded asset's SHA256 matches the release's
  published `SHA256SUMS`.
- Keep the previous executable and roll back to it when the new daemon
  fails its health check.
- Reuse the existing replacement path (`Ensure`) rather than inventing a
  new handover.
- Check every few hours; silently skip when offline.
- Start the downloaded daemon with an accurate `--build-time`, not the
  download's mtime.

Consequences of the owner answers (announced by EXEC4, accepted by MAIN on
2026-11-15 at 01:52Z):

- **Shell daemon rule gains one exception.** The Electron ticket carries
  thin-Tauri Decision 2/3: the shell attaches, starts a daemon only when none
  runs, never replaces or stops one, and never runs `ws-mcp daemon-start`.
  The user's apply click is the single exception: the shell runs
  `<downloaded exe> daemon-start --build-time <ms>`, whose `Ensure` handoff
  replaces an older daemon under the existing newest-wins rules
  (`cmd/ws-mcp/proxy.go:354-399`, `internal/wsdashboard/ensure.go:124-184`).
  No other shell path replaces or stops a daemon. Thin-Tauri Decision 2's
  clause "the plugin launcher's daemon stays the authority until self-update
  lands" is superseded by this user-approved update
  (`261113-feat-werkschaft-thin-tauri-shell` Decision 2: "Revisit when the
  app self-updates").
- **No daemon-side change.** The check, download, verification, prompt,
  swap and rollback live in the shell, so the earlier daemon-side items
  (the `dashboard.auto_update` knob, `/health` update-status fields, a
  loopback-only update route, the dashboard `#dash-banner`/toast notices,
  and the daemon-orchestrated rollback) are dropped.
- **Compatibility: no tier.** Nothing in the ws-mcp cross-version surface
  changes (no MCP tool, hello, store, tracker event, manifest field,
  `endpoint.json`, `runtime.json` or asset name;
  `ai-docs/manuals/release-compatibility.md:59-80`); the shell only consumes
  the existing release assets, `/health` and `daemon-start`.
- **Shell self-update is out of scope.** No release asset ships the Electron
  app, so the shell updates the daemon only.

Technical calls (the release-source, release-check and build-time items were
posted by MAIN as no-answer-needed on 2026-11-15 and carry over unchanged;
the rest were re-derived for the shell-only scope and accepted by MAIN on
2026-11-15 at 01:52Z):

- **Release source = the launcher's.** `kang-sw/devenv` (the product's
  release repository, the launcher's `runtime.json` default) with the
  `WS_MCP_RELEASE_REPOSITORY` / `WS_MCP_RELEASE_BASE_URL` overrides honored
  from the shell's environment (`ws-mcp-launcher.py:357-382`), so the
  launcher and the shell agree on "latest".
- **Release check.** `GET https://api.github.com/repos/<repo>/releases/latest`
  from the shell main process (unauthenticated, 10 s timeout), first 10
  minutes after shell start, then every 6 hours with ±10 % jitter; any
  network error, 403 or 429 skips silently to the next tick. The running
  daemon's version and build come from its `/health`
  (`internal/wsdashboard/daemon.go:284-301`). Rejected: polling download
  URLs by guessed tag (only the API names the latest tag and its asset
  upload times).
- **Build time.** `--build-time` is the matching asset's `updated_at` from
  the same API response, in Unix milliseconds. Rejected: the downloaded
  file's mtime (download time, the equal-version tie) and a new build-time
  field in the release metadata sidecar (a release-pipeline change for a
  value the API already gives).
- **Download location.** The verified asset (`SHA256SUMS` plus the
  `.metadata.json` sidecar whose version equals the tag) goes to a
  shell-owned content-addressed directory under the app's user-data
  directory, `daemon-updates/ws-mcp-<sha256 first 16><ext>`, downloaded to a
  `.tmp` name and renamed after verification. Rejected: the daemon's
  `dashboard/bin/` staging directory, because the daemon that next holds the
  lock prunes every other settled entry there after a 2-minute grace
  (`internal/wsdashboard/spawn.go:69-101`), which could delete the
  rollback target mid-swap.
- **Rollback.** Before the swap the shell copies the running daemon's
  executable into the same shell-owned directory, locating it from the
  `/health` PID through the OS process table (macOS first; the Windows
  equivalent belongs with the Electron ticket's Phase 3). If the successor
  is not healthy within the `Ensure` wait, the shell starts the retained
  copy with `daemon-start --build-time <its /health build_time>` and
  records the failed version so it is not offered again until a newer
  release appears. When the executable cannot be located, the toast's apply
  action is disabled with the reason shown. Rejected: an optional
  `executable` field in `/health` (a daemon change and a path disclosure to
  LAN viewers for a lookup the OS already answers); falling back to the
  shell's bundled daemon (may be older than the one it replaced). This is
  the least-certain call: the worker verifies the PID lookup on the
  supported macOS versions and reports if it fails.
- **Toast surface.** The toast is a small shell-owned overlay view
  (bottom-right of the window, local packaged content only, never remote
  content) whose own preload exposes exactly two actions, apply and dismiss,
  to the main process. The dashboard renderer keeps the Electron ticket's
  zero-authority posture (no preload, no IPC). Rejected: a native OS
  notification (not a persistent in-window toast); a toast inside the
  daemon-served page (the renderer has no channel to the shell by design).
- **Two phases**: Phase 1 the check, verified download, swap and rollback in
  the main process; Phase 2 the toast and the minor flow. Rejected: one
  phase (Phase 1 is testable alone with a fake release server and an
  isolated daemon, and carries the risk).

## Constraints

- Implementation is not authorized until the owner says so; the ticket is
  queued after `261115-feat-werkschaft-electron-shell` and needs its Phase 1
  shell.
- Code lives in the Electron shell (`agents-plugin-tool/desktop-electron/`,
  plain CommonJS per the Electron ticket); no ws-mcp source change.
- Never touch the shared daemon or port 29417; tests use a fake release
  server, an isolated `WS_CACHE_HOME` and an isolated daemon built from
  source. Offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; end runs
  with an owned process scan.
- No change to the `/mcp` hello, the protocol number, `endpoint.json`,
  `runtime.json`, release asset names or the release pipeline.
- Before editing, read the manuals the AGENTS.md Implementation
  Conventions table binds to the touched paths:
  `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`).

## Phases

### Phase 1: Release check, verified download, swap and rollback

Implement the decisions above in the shell main process. Verification with
a fake release server and an isolated daemon (Node tests plus one packaged
smoke): latest-tag parsing and the patch/minor/major/older/`-dev` matrix
against `/health`; offline, 403 and 429 skips; `SHA256SUMS` and metadata
mismatch refusals; the `.tmp`-then-rename download into the shell-owned
directory; `--build-time` from the asset time; a successful swap through
`daemon-start` with a launcher-style client reconnecting; a successor that
fails health triggering rollback to the retained copy and the failed
version not being offered again; the executable-not-located path disabling
apply; no other code path in the shell running `daemon-start` against a
running daemon.

### Phase 2: Update toast and the minor flow

Add the overlay toast with apply and dismiss, its narrow preload, and the
minor flow per (868)=a. Verification: a Playwright `_electron` check against
an isolated daemon and a fake release server for the toast text, dismiss
persistence until a newer release, apply performing the swap, the minor
flow (the confirmation lists the older-minor sessions and the
plugin-update steps, and cancel leaves the daemon untouched), no toast on a
`-dev` daemon, and the dashboard renderer
still exposing no `window.require`/`process`/preload API; light and dark
screenshots.
