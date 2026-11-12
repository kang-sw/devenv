---
title: "ws-ade: a standalone desktop app (Tauri or similar) around the dashboard daemon, and renaming the product to ws-ade"
related:
  261110-epic-ws-dashboard-agent-host: "its Scope defers a desktop shell until the browser dashboard is complete ((18) a, (19)); this research does not change that order"
  261110-research-ws-dashboard-agent-host-foundations: "holds the earlier Tauri-vs-Electron question; this ticket carries it forward"
---

# ws-ade: desktop shell and product rename

## Background

User, 2026-11-12 (whiteboard topic `ws-ade`): package the dashboard as a
standalone application (Tauri named) that runs without the user first
starting a harness such as Claude Code in a terminal, and make it a product
called ws-ade, renaming the project from devenv.

The browser dashboard already opens headless lead sessions, hosts their
processes in the session holder, and gives a browser terminal, so the app
is mainly a shell that bundles the daemon as a sidecar and opens a window.

## Topic 1: desktop shell

- Tauri against Electron (or another option) for wrapping the existing web
  UI with the Go daemon as a sidecar: bundle size, WebView consistency on
  macOS, Windows, and Linux, auto-update, code signing.
- What the shell adds beyond the browser: a trusted local origin, native
  notifications, tray/menu, OS login items, deep links.
- How the app finds or installs the harness CLIs (claude, pi, codex) it
  starts, and how it coexists with a daemon a terminal session already
  started.

## Topic 2: rename to ws-ade

- Every place the repository or root directory name is a key: per-project
  cache and state paths (for example `~/.cache/ws@<owner>-<repo>/`), the
  worktree pool default `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`,
  dashboard session registrations, whiteboard and tracker store paths,
  marketplace entries, `install.sh`, GitHub URLs, docs.
- A migration path so a renamed checkout keeps its sessions, boards, and
  registrations.
- Where the product name shows (app title, dashboard header) versus ids that
  stay.

## Outcome Ledger

### Verified Findings

Paths are relative to `agents-plugin-tool/internal/` unless noted. Web
sources were read 2026-11-12.

**Daemon and dashboard: what a shell must provide**
- The dashboard needs no auth token on this machine. Loopback mode binds
  loopback only and `hostGuard` refuses any non-loopback Host
  (`wsdashboard/daemon.go:672`); a browser here also passes `localViewer`
  (loopback peer plus loopback Host, `wsdashboard/access.go:146`). The LAN
  token only matters for other devices (`access.go` file comment).
- Every write that carries execution authority (terminal, holder stop,
  hosted-session spawn, approvals) additionally requires an `Origin` equal
  to the daemon's own, scheme `http` only (`wsdashboard/access.go:156`,
  `wsdashboard/inbox_routes.go:141`); WebSocket handshakes likewise
  (`wsdashboard/stream_ws.go:51`). Consequence: the window must load the UI
  from `http://localhost:<port>/` (or 127.0.0.1). A bundled custom-scheme
  origin (`tauri://localhost`, `app://`) would be refused by `sameOrigin`.
- Port discovery: `<cache-root>/dashboard/endpoint.json` holds
  `{schema_version, port, pid, version, token, started_at}`
  (`wsdashboard/endpoint.go:52`); default port 29417
  (`endpoint.go:35`), a taken port falls back to an OS-chosen one
  (`cmd/ws-mcp/dashboard.go` `--port` flag), and `last-port` outlives the
  daemon (`endpoint.go:43`). The `token` there is the shutdown credential,
  owner-only (0600), not a UI credential; the shell should read `port` and
  confirm with `GET /health`, and never forward `token` to the webview.
- The daemon is machine-wide and single-instance by OS file lock;
  `Ensure` starts it detached and replaces an older one by version/build
  (`wsdashboard/ensure.go:112`, `wsdashboard/spawn.go:24`).
  `ws-mcp daemon-start` is already the one-shot entry that runs `Ensure`
  and prints the outcome as one JSON line (`cmd/ws-mcp/proxy.go:299`).
  The daemon intentionally outlives callers; sessions and holders live in
  it, so the shell must not own its lifetime.
- Harness lookup is `exec.LookPath("claude"|"pi")` against the daemon's own
  PATH, with no config override (`wsagent/backend.go:344`, `wsagent/claude.go:28`,
  `cmd/ws-mcp/dashboard.go:260`). Only the Claude and Pi backends exist
  there (no codex backend). A daemon started by a Finder/Dock-launched app
  inherits the GUI session's minimal PATH on macOS, so `claude`/`pi` would
  not resolve unless the shell passes a login-shell PATH or a command
  override is added (the PATH claim is platform knowledge, not verified
  here).
- Session holders run under Python: `python3` on PATH, or
  `WS_MCP_HOLDER_PYTHON` (`wsholder/manager.go:501`, `wsholder/holder.go:36`).
  The Python launcher normally supplies it. A launcher-less bundle needs
  either an interpreter on the user's machine (absent by default on
  Windows; the macOS `/usr/bin/python3` stub prompts for Xcode tools) or a
  bundled one.
- The ws-mcp binary is about 41 MB (`~/.cache/ws@kang-sw-devenv/dashboard/bin`
  staged copies, 41,463,186 bytes); embedded page assets are 2.7 MB. It is
  a Go `package main` binary, so it fits a sidecar/resource, not an
  in-process library.
- Browser features the page uses: WebSocket per terminal card
  (`wsdashboard/assets/terminal.js:193`) and for streams (`stream_ws.go`),
  SSE (`wsdashboard/sse.go`), vendored xterm.js with its DOM renderer
  (`assets/xterm`, 308K), CSS container queries
  (`assets/tracker.css:529`, `assets/dashboard.css:509,545`),
  `navigator.clipboard` with a `window.isSecureContext` guard and a textarea
  fallback (`assets/dashboard.js:2241`, `:3894`), and the web `Notification`
  API for approval prompts using `permission`, `requestPermission`,
  `onclick` (`assets/dashboard.js:6544`, `:6585`).

**Webview differences that hit these features**
- Tauri 2 uses WKWebView on macOS, WebView2 (Chromium, self-updating) on
  Windows, and WebKitGTK on Linux; macOS and Linux track the OS/distro, not
  the app. The doc lists webkitgtk 2.36 (WebKit 614) as shipping on Ubuntu
  20.04/22.04 and macOS 12.5 as WebKit 613 (Safari 15.6)
  (https://v2.tauri.app/reference/webview-versions/). CSS container queries
  start at Safari 16.0, Chrome 106, Firefox 110
  (https://caniuse.com/css-container-queries), so macOS 12 and
  older and the oldest WebKitGTK builds would lose this dashboard's
  narrow/wide layouts. Tauri itself lists macOS 10.15+ as supported
  (https://tauri.app/start/prerequisites/). Linux needs the 4.1 API
  (`libwebkit2gtk-4.1-dev` on Debian, same page).
- Fetch Metadata: the daemon's capability/preview routes read
  `Sec-Fetch-Dest` (`wsdashboard/fs_capability.go:297,309`,
  `wsdashboard/image_route.go:201`). Safari supports `Sec-Fetch-Dest` from
  16.4 (https://caniuse.com/mdn-http_headers_sec-fetch-dest); WebKit
  supports all but `Sec-Fetch-User` (https://bugs.webkit.org/show_bug.cgi?id=247697)
  but had wrong `Sec-Fetch-Site` values on iframes
  (https://bugs.webkit.org/show_bug.cgi?id=256472). `crossSiteFetch` treats
  an absent value as pass, so a WebKit shell degrades rather than breaks;
  still, the sandboxed preview frames are a thing to test in WKWebView.
- Web `Notification`: the Tauri notification plugin injects an init script
  that replaces `window.Notification` and routes it through
  `__TAURI_INTERNALS__.invoke` (plugins-workspace `plugins/notification/src/init-iife.js`,
  https://v2.tauri.app/plugin/notification/). Tauri keeps IPC off for remote
  origins by default; access needs a capability with `remote.urls`
  (https://v2.tauri.app/security/capabilities/), and Tauri warns that on
  Linux and Android it cannot tell an iframe's requests from the window's.
  So the dashboard's existing `new Notification(...)`/`onclick` code works
  in a Tauri shell only if `http://localhost:*` is granted the notification
  permission, widening IPC exposure to everything on that origin
  (inference, not tested: the sandboxed preview frames have an opaque
  origin and would not match the pattern).
  Desktop action buttons are not supported by the plugin (same page); the
  plugin's `onclick` support was not verified.

**Tauri 2.x vs Electron, from official docs**
- Sidecar: `bundle.externalBin` with `-<target-triple>` suffixed copies,
  spawned via the shell plugin, capability `shell:allow-spawn`
  (https://v2.tauri.app/develop/sidecar/). The shell plugin kills every
  spawned child on `RunEvent::Exit` (plugins-workspace
  `plugins/shell/src/lib.rs`, doc comment on `init()`). Because the
  daemon must outlive the app, `ws-mcp daemon-start` should be run as a
  short-lived command and let `Ensure` detach the real daemon, rather
  than spawning the daemon itself as a long-lived sidecar. A
  long-lived plain child would be killed on app exit and take lead sessions
  with it.
- Size: Tauri advertises under 600 KB for a minimal app because it uses the
  system webview (https://v2.tauri.app/start/). Here the 41 MB ws-mcp binary
  dominates, so Tauri's shell overhead is small and Electron would add a
  Chromium runtime on top (Electron's own docs give no size number; typical
  figure of roughly 100 MB or more is common knowledge, not sourced).
- Engine consistency: Electron bundles Chromium and releases a major every
  8 weeks, supporting the latest three majors
  (https://www.electronjs.org/docs/latest/tutorial/electron-timelines), so
  every OS runs the same engine that this dashboard is developed against;
  Tauri inherits WKWebView/WebKitGTK variation (above).
- Updates: Tauri's updater requires signed artifacts (key pair, cannot be
  disabled), static JSON on a CDN or GitHub Releases, Linux via AppImage
  (https://v2.tauri.app/plugin/updater/). Electron's `autoUpdater` is
  Squirrel-based and covers macOS and Windows; the page does not cover
  Linux, and its free service needs a public GitHub repo and signing on
  macOS (https://www.electronjs.org/docs/latest/tutorial/updates).
- Signing: macOS needs a paid Apple Developer ID and notarization in both
  (https://v2.tauri.app/distribute/sign/macos/,
  https://www.electronjs.org/docs/latest/tutorial/code-signing). Windows
  needs OV/EV or Azure Artifact Signing; SmartScreen reputation builds with
  use for both kinds of certificate
  (https://v2.tauri.app/distribute/sign/windows/). Electron on macOS also
  needs a valid signature for `autoUpdater`, `setLoginItemSettings`, and
  notification events (code-signing and notifications pages).
  Neither doc page covers signing a sidecar binary inside the bundle
  (hardened runtime on a nested Go executable); that is untested here.
- Platform features: Tauri has first-party desktop plugins for
  notifications, deep links (macOS only when installed in
  `/Applications`, https://v2.tauri.app/plugin/deep-linking/), and
  autostart (https://v2.tauri.app/plugin/autostart/). Electron has
  `Notification` in main and renderer, with Windows needing an
  AppUserModelID (https://www.electronjs.org/docs/latest/tutorial/notifications).
- Third option: Wails (Go-native webview shell) shares Tauri's
  system-webview trade-offs and was not researched (wails.io returned
  403). A plain browser app-mode window or PWA needs no shell but gives no
  native notification, tray, or sidecar management.
- Tauri 2.12 is the newest release the Tauri blog lists
  (https://v2.tauri.app/blog/, dated 2026-09-26); the notification and
  updater pages require Rust 1.90 or later.

**Rename: where the repo/directory name acts as a key**

Continuity breaks when the checkout's absolute path changes, not when the
name text changes. The only name-as-text keys are the first two rows.

| Site | file:line | Rename effect |
|---|---|---|
| Per-project cache key | `wsstate/paths.go:114` (`rootID := shortHash(commonRoot)`; `:318` sha256 of the canonical path, first 8 hex) | Renaming or moving the checkout directory yields a new `proj/<hash>` dir; the old project's mailbox, `state.sqlite` index, review and prompt paths, and per-worktree `sessions` are stranded. Breaks. `RepoBasename` (`:128`) is metadata only. |
| Worktree pool default | `mcp/worktree_tools.go:133,167` (`$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`, `GitRootDirName` is `filepath.Base(gitRoot)`) | The pool becomes `.ws-worktrees/ws-ade`; existing pooled worktrees under `.ws-worktrees/devenv/*` are no longer owned, listed, or releasable (`ownedPoolRoots`). Also git's own worktree links hold absolute paths, so the main checkout move needs `git worktree repair`. Breaks (this machine has 7 pooled worktrees). |
| Cache root name | `wsstate/paths.go:20,101`, mirrored in `agents-plugin/bin/ws-mcp-launcher.py:857` and `agents-plugin/tests/test_ws_mcp_launcher_proxy.py:709` | Constant `ws@kang-sw-devenv`, independent of the directory name. A rename keeps it; changing it would orphan the daemon dir (`dashboard/`, holders, sessions, endpoint). Keep. |
| Dashboard session registrations | `wsdashboard/registry.go:78` (`project` = absolute root path, in `dashboard/sessions/<id>/registration.json`) | Live sessions registered under the old path stop matching their project; new sessions register under the new path. Self-heals as sessions end. |
| Stored workspaces | `wsdashboard/workspaces.go:26` (`~/.ws/dashboard/workspaces.json`, absolute paths) | The rail entry for the old path goes dead until re-added. Breaks softly. |
| Whiteboards | `mcp/whiteboard_tools.go:38` (`ai-docs/whiteboards` under the session root; state in `sessions/<key>/whiteboard.json`) | Session-keyed, repo-relative, independent of the name. Survives if the session record's `Root` still resolves. |
| Tracker stores | `wstrackerhost/registry.go:34` (`~/.ws/tracker`, keyed by store id and remote URL) | Not derived from the directory name. Survives. A GitHub rename leaves the old remote URL working through GitHub's redirect. |
| Repo notes | `ai-docs/ws-notes/` (in-tree, `wsnote/repo_store.go`) | Move with the repo. Survives. |
| Go module path | `agents-plugin-tool/go.mod:1` (`github.com/kang-sw/devenv`), 338 files import it | Internal import path only; no change needed. Changing it is a mass rewrite with no user value. |
| Release download | `agents-plugin/runtime.json:6`, `agents-plugin-wsflow` / `agents-plugin-pi` `runtime.json`, `agents-plugin/bin/ws-mcp-launcher.py:382` (`https://github.com/{release_repository}/releases/download/{tag}`) | Already-installed older launchers keep fetching the old repo URL. GitHub redirects a renamed repo; the redirect fails if a new repo takes the old name. Keep the old URL reachable. |
| Marketplace/plugin ids | `.agents/plugins/marketplace.json:2`, `.claude-plugin/marketplace.json:2` (`kang-sw-devenv`); `install.sh` (12 lines), `scripts/install-claude-plugin.ps1` (9 lines), `ws@kang-sw-devenv` references; plugin manifests' `repository` URL (4 files); `README.md` (7 mentions incl. clone and `pi install git:` URLs) | The marketplace name is the install key (`<plugin>@kang-sw-devenv`) and the plugin cache path component (`~/.claude/plugins/cache/kang-sw-devenv/`). Rename it and every install needs a reinstall. Confirmed decision keeps `ws`/`wsflow`; keep the marketplace name too (Proposal). |
| User-visible product name | `README.md`, `AGENTS.md` (1), dashboard UI strings (not inventoried), plugin manifest `repository` | Free to change; no key depends on it. |
| Local-devenv marker | `.local-devenv-runtime`, `local_devenv_*` in the launcher, `agents-plugin-pi/src/local-devenv.ts` | Internal dev-mode names; independent of the repo name. |
| Documentation | `ai-docs/manuals/ws-mcp.md:1012,1281-1282`, `ai-docs/manuals/codex-integration.md:24`, `CHANGELOG.md`, old plans | Mostly the marketplace/cache constants above; update only if those change. |

### Confirmed Decisions
- Research the desktop shell now; its implementation keeps the agent-host
  epic's order, after the browser dashboard is complete (user, (574) b).
- Rename scope: the repository/project and the user-visible product name
  become ws-ade (app and dashboard labels); the plugin ids `ws` and `wsflow`
  stay, since changing them would break every downstream install
  (user, (575) b).

### Proposals

Non-authoritative; nothing here is decided.

- **Shell choice: Tauri 2.x, thin remote-URL shell.** The UI is already
  served over loopback HTTP and requires an `http` same-origin, so the shell
  needs no bundled frontend: one window pointed at
  `http://localhost:<port>/`, no custom protocol. Reasons over Electron:
  the 41 MB sidecar already dominates size, and a Rust shell adds little
  attack surface to a daemon that holds execution authority. Cost accepted:
  WKWebView/WebKitGTK variation (container-query floor at Safari 16 /
  macOS 13+, WebKitGTK 4.1). If the webview matrix proves painful, Electron
  is the fallback because it pins one Chromium; the shell contract below
  does not change.
- **Shell contract (shell-agnostic).** (1) Run the bundled
  `ws-mcp daemon-start`, short-lived, and treat its JSON outcome as the
  attach/start result; if a terminal session already started a daemon,
  `Ensure` returns `running` and the shell just attaches. (2) Read `port`
  from `endpoint.json`, confirm `GET /health`, load the URL. (3) Never kill
  the daemon on app exit; offer an explicit "stop daemon" action that uses
  the existing token-authenticated shutdown. (4) Do not hand the endpoint
  `token` to the webview.
- **Harness CLIs.** Before `daemon-start`, the shell resolves `claude` and
  `pi` using the user's login-shell PATH and passes that PATH in the
  daemon's environment; add a config override for the harness command
  (the `Command` field exists in the backends but no setting reaches it).
  Show a first-run panel when a harness is missing instead of failing at
  session start.
- **Python for holders.** Pick one: require `python3` and report it in the
  same first-run panel, or ship a pinned interpreter and set
  `WS_MCP_HOLDER_PYTHON`. The first is cheaper; the second is the only
  option for a "runs without prerequisites" promise. This also decides
  whether the shell bundles the Python launcher or only the Go binary.
- **Native notifications.** Prefer a small shell-injected shim that maps
  the page's `Notification` to the native API while keeping the existing
  `onclick`, rather than granting the whole localhost origin general IPC;
  if the Tauri plugin's override is used, scope its capability to the
  notification permission only.
- **Rename: keep identity keys, change labels.** Rename the GitHub
  repository and the product name; do not rename the marketplace
  (`kang-sw-devenv`), the cache-root constant (`ws@kang-sw-devenv`), the Go
  module path, or the plugin ids. That leaves only the checkout directory
  name and its absolute path as live keys.
- **Migration for a renamed checkout.** The path hash and pool template
  are the two real breaks. Options, cheapest first: (a) keep the local
  directory named `devenv` and rename only the GitHub repo and labels
  (zero continuity loss, local name stays an internal detail); (b) rename the directory and add
  a one-shot `ws-mcp` migration that, given old and new root, moves
  `proj/<oldhash>*` to `proj/<newhash>*`, rewrites `root_path`/
  `worktree_path` in `project.json`/`worktree.json`, rewrites session
  record `Root` and `registration.json` `project`, updates
  `workspaces.json`, moves `.ws-worktrees/devenv` to
  `.ws-worktrees/ws-ade`, and runs `git worktree repair`. A symlink from the old path does not
  help: `canonicalPath` resolves symlinks (`wsstate/paths.go:304`), so the
  hash follows the target. Recommend (a) for this repo's own checkout; build (b)
  only if downstream users are expected to rename directories.
- **Compatibility tier (record when a ticket lands).** A store migration
  that rewrites cache paths is a non-additive store change (T2 per
  `ai-docs/manuals/release-compatibility.md`); the label-only rename and a
  new harness-command setting are T0.

### Open Questions
- Does the Tauri notification plugin's `window.Notification` override
  carry `onclick` (focus window, open the approval card)? Needs a spike.
- What is the narrowest capability that lets `http://localhost:<port>`
  call only the notification commands, and does Tauri 2 match a port
  wildcard in `remote.urls`? Only a third-party source was found.
- Does a Go binary nested in a Tauri bundle sign and notarize cleanly on
  macOS with hardened runtime, and does Windows SmartScreen treat it
  separately? Not covered by either vendor's docs.
- In WKWebView, do the preview frames (sandboxed, opaque origin) and the
  `Sec-Fetch-Site` iframe bug (WebKit 256472) change the capability
  route's behavior? Needs a run against the shipped dashboard.
- Python holder: require or bundle (see Proposals)? Needs a product call
  on the "no prerequisites" promise.
- Is `codex` expected as a harness in the app? The daemon has only Claude
  and Pi backends today.
- Who renames directories: only this repo's maintainer, or downstream
  users too? That decides whether the migration tool is worth building.
- Linux scope: is WebKitGTK 2.36-era (Ubuntu 22.04 floor) an acceptable
  minimum, or does Linux wait for a Chromium-based shell?
- Wails was not evaluated (site returned 403); worth a short check only
  if Tauri's Rust requirement is a blocker.

### Rejected Alternatives
- Starting the desktop shell implementation now ((574) a): the dashboard
  still changes quickly, so a shell built now would be reworked.
- Renaming the plugin ids ((575) c): a T2 break for every downstream install.
