---
title: "ws dashboard daemon auto-update from GitHub releases"
related:
  261115-feat-werkschaft-electron-shell: "owner sequencing: this ticket is ordered after the Electron shell; not a technical prerequisite. That ticket carries the thin-Tauri daemon-ownership decision this ticket partly supersedes"
  261113-feat-werkschaft-thin-tauri-shell: "Decision 2 (daemon ownership, 'revisit when the app self-updates') is the decision this ticket revisits"
  261112-research-ws-ade-desktop-shell-and-rename: "Daemon ownership section records the same policy and its minor-lockout concern"
---

# ws dashboard daemon auto-update from GitHub releases

## Background

The owner asked (relayed by MAIN, 2026-11-15 at 01:29Z) for the dashboard
daemon to update itself: check the GitHub release tag, download the latest
release whole into a unique path, then swap daemons. Implementation is not
authorized yet; the owner orders this ticket after
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

- **(860)=a: patch releases auto-swap; a minor bump notifies.** A newer
  release with the running daemon's major.minor and a higher patch is
  downloaded, verified and swapped in automatically. A newer minor (or
  major) shows a notification with an "update" button and plugin-update
  guidance instead. Rejected: auto-swapping minors too (an older-minor
  launcher is locked out the moment a newer-minor daemon runs).
- **(861) closed: the update applies to the single global daemon.** There
  is one daemon per cache root and the newest version already wins in both
  the launcher and `Ensure`, so a daemon that updates itself does not fight
  launcher-started daemons. Rejected premise: limiting auto-update to an
  Electron-launched daemon.

Announced defaults relayed by MAIN:

- `-dev` builds never auto-update (and never show the minor notice).
- Swap only after the downloaded asset's SHA256 matches the release's
  published `SHA256SUMS`.
- Keep the previous unique executable path and roll back to it when the
  new daemon fails its health check.
- Reuse the existing daemon replacement path (`Ensure`, as
  `applyLANChange` does) rather than inventing a new handover.
- Check every few hours; silently skip when offline.
- Start the downloaded daemon with an accurate `--build-time`, not the
  download's mtime.

## Constraints

- Implementation is not authorized until the owner says so; the ticket is
  queued after `261115-feat-werkschaft-electron-shell`.
- Never touch the shared daemon or port 29417; tests use a fake release
  server and an isolated `WS_CACHE_HOME`. Offline env
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; never the whole
  `./internal/wsdashboard` package (`go test -run`); end runs with an owned
  process scan.
- No change to the `/mcp` hello, the protocol number, `endpoint.json`
  required fields, `runtime.json`, or release asset names.
- Before editing, read the manuals the AGENTS.md Implementation
  Conventions table binds to the touched paths:
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/ws-mcp.md` (for `internal/mcp/`), and
  `ai-docs/manuals/wsflow-mirroring.md` (both packages' config manifests).

## Open Decision Queue

Last ID used: pending MAIN board mapping (local labels A1-A14)

(A1) [open] [announced] The thin-Tauri Decision 2 splits: its shell rule (the shell attaches, starts a daemon only when none runs, never replaces or stops one) stays as carried by the Electron ticket; its "the plugin launcher's daemon stays the authority until self-update lands" clause is superseded here, because this is that self-update and its minor-lockout concern is met by (860)=a - `261113-feat-werkschaft-thin-tauri-shell` Decision 2 text ("Revisit when the app self-updates") plus (860)=a.

(A2) [open] [announced] The release source is the same one the launcher uses: `kang-sw/devenv` (the product's release repository, compiled into the daemon as the launcher's `runtime.json` default) with the `WS_MCP_RELEASE_REPOSITORY` / `WS_MCP_RELEASE_BASE_URL` overrides honored - `ws-mcp-launcher.py:357-382`; one source keeps the launcher and the updater from disagreeing about "latest".

(A3) [open] [announced] Compatibility T0: a new optional `dashboard.*` knob, optional `/health` update-status fields, a new loopback-only POST route and new UI; no hello, protocol, `endpoint.json` required field, `runtime.json` or asset-name change - `ai-docs/manuals/release-compatibility.md:59-80`.

(A4) [open] [announced] The update action route and its button exist only for local viewers (`TrustedWrite`: 404 remote, 403 bad Origin); remote/LAN viewers see neither - `access.go:162-175`, an execution-authority write.

(A5) [open] [technical] The daemon queries `GET https://api.github.com/repos/<repo>/releases/latest` (unauthenticated, default proxy-from-environment transport, 10 s timeout), first 10 minutes after start and then every 6 hours with ±10 % jitter; any network error, 403 or 429 skips silently to the next tick - over polling the download URL by guessed tag, because only the API names the latest tag and its asset upload times.

(A6) [open] [technical] `--build-time` is the matching asset's `updated_at` from the same API response, in Unix milliseconds - over the downloaded file's mtime (download time, the tie MAIN flagged) or adding a build-time field to the release metadata sidecar (a release-pipeline change for a value the API already gives).

(A7) [open] [technical] The downloaded asset goes to the existing content-addressed staging directory `<cache root>/dashboard/bin/ws-mcp-dashboard-<sha256 first 16><ext>` after `SHA256SUMS` and the `.metadata.json` sidecar (its version must equal the tag) verify; the running daemon's own staged path is recorded as the rollback target and exempted from pruning until the successor has been healthy for 24 hours - over a new directory, since staging is already unique per content and Windows-lock-safe (`spawn.go:33-65`).

(A8) [open] [technical] Swap and rollback are orchestrated by the old daemon: it runs `Ensure` with the new executable (as `applyLANChange` does); if the successor does not become healthy, it stops the successor process it spawned, restarts its own retained executable through `daemon-start --build-time <its own build time>`, and records the failed version so it is not retried until a newer release appears - over leaving rollback to the next launcher start, which would leave no daemon running meanwhile (`ensure.go:151-159` returns unavailable without restarting anything).

(A9) [open] [technical] After a successful auto-swap the new daemon shows a toast "Updated to vX.Y.Z" once per browser; the minor notice is a persistent `#dash-banner` entry plus an index notice - over toast-only (toasts expire, `toast.js`) or banner-only for patches (a completed patch swap needs no action).

(A10) [open] [policy] When a patch auto-swap happens.
- Context: a swap drains the daemon: already-sent MCP requests fail, long calls linger up to two minutes on the old process, and hosted sessions are drained (`proxy.go:42-50,306-340`).
- Alternative a: at the next idle moment: no MCP request in flight and no hosted agent turn running, checked every minute once the verified download is staged; if no idle moment comes within 24 hours, swap anyway.
- Alternative b: immediately after verification.
> Recommend a: patches are routine; waiting for idle avoids failing a request or cutting an agent turn, and the 24-hour cap keeps fixes from stalling.

(A11) [open] [policy] What the minor-bump "update" button does.
- Context: (860)=a says "notification + update button with plugin-update guidance". Swapping to a newer-minor daemon disconnects every older-minor plugin session until its plugin is updated.
- Alternative a: the button swaps the daemon to the new minor after a confirmation that lists the attached older-minor sessions it will disconnect, and the notice shows the plugin-update steps (Claude `/plugin` update, Codex plugin refresh).
- Alternative b: the button only shows the plugin-update steps; updating a plugin brings the newer daemon through the launcher's newest-wins rule, so the daemon never swaps to a new minor on its own.
> Recommend a: it matches "update button" literally, and the confirmation makes the lockout visible before it happens.

(A12) [open] [policy] The per-package default of the new knob `dashboard.auto_update`.
- Context: `agents-plugin-wsflow/` is the conservative-default distribution of the same product (same keys, only defaults differ).
- Alternative a: `ws` default `true` (patch auto-swap); `wsflow` default `false` (patches also only notify, with the same button).
- Alternative b: both `true`.
- Alternative c: both `false` (opt-in everywhere).
> Recommend a: the owner's flow becomes the `ws` default, and `wsflow` keeps its conservative posture; either package can flip it.

(A13) [open] [technical] The knob is machine-wide (global scope, registered in code like the other global-only dashboard keys, `internal/mcp/dashboard_config.go:15-33`) and, when `false`, still checks and shows a notice for every newer release, with the same button - over disabling the check entirely, which would hide security patches.

(A14) [open] [technical] Two phases: Phase 1 the daemon side (check, download, verify, idle swap, rollback, knob, `/health` fields); Phase 2 the dashboard side (toast, minor banner and button, confirmation, guidance) - over one phase, because Phase 1 is testable alone with a fake release server and carries the risk.

## Phases

### Phase 1: Release check, verified download, patch swap and rollback

Implement the decisions above on the daemon side. Verification with a fake
release server and an isolated cache root (`go test -run`): latest-tag
parsing and the patch/minor/major/older/`-dev` matrix; offline, 403 and 429
skips; `SHA256SUMS` and metadata mismatch refusals; the staged path and
pruning exemption; the idle-swap gate and the 24-hour cap (per (A10));
`--build-time` from the asset time; a successful swap through `Ensure` with
launcher reconnect; a successor that fails health triggering rollback to the
retained executable and the failed version not being retried; the knob
default per package and its `false` behavior.

### Phase 2: Dashboard notices and the update action

Add the post-swap toast, the persistent minor notice with its button,
confirmation and plugin-update guidance, and the loopback-only action route.
Verification: Go route tests for local/remote/bad-Origin; a headless
Chromium check against an isolated daemon with a fake release server for the
toast, the banner, the confirmation listing older-minor sessions and the
button's effect (per (A11)); no notice on a `-dev` daemon; light and dark
screenshots.
