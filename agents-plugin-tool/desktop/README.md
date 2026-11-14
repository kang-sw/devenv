# Werkschaft: attach-only macOS window

This nested package loads the existing dashboard directly from its discovered
`http://127.0.0.1:<port>/` origin in WKWebView. It adds no dashboard frontend,
credential, daemon API, or JavaScript-accessible native command. The small local
HTML document is only the connection/error view. Connection → Retry (⌘R)
re-reads discovery; closing the window leaves the daemon running.

## Preparation

Use Rust 1.95 on Apple Silicon with Xcode/Command Line Tools already installed.
The npm-local Tauri CLI is pinned in `package-lock.json`; Rust dependencies are
pinned in `Cargo.lock`. From this directory, a fresh checkout needs:

```sh
npm ci --ignore-scripts --no-audit --no-fund
PATH="$HOME/.cargo/bin:$PATH" cargo fetch --locked
```

Dependency preparation is separate from permission to compile or launch.

## One allocated build attempt

After the coordinating lead grants the actual build slot, run from this directory:

```sh
python3 scripts/build-slot.py --allocated
```

The runner has **one cumulative 1,500-second ceiling**, including termination:
1,485 seconds shared by the following commands, then at most 15 seconds for
cleanup (TERM for up to 5 seconds, then KILL for the remainder). It stops at the
first nonzero exit or timeout; source repair and rerun require a new allocation.

1. `$HOME/.cargo/bin/cargo test --frozen --features custom-protocol -j 2 -- --test-threads=1`
2. `./node_modules/.bin/tauri build --debug --bundles app --ci --no-sign -- --frozen -j 2`

Both commands use this directory as cwd. The runner prepends `$HOME/.cargo/bin`
to PATH, uses the installed `stable-aarch64-apple-darwin` toolchain with
`RUSTUP_AUTO_INSTALL=0`, sets `CARGO_TARGET_DIR` to this directory's `target`,
`CARGO_BUILD_JOBS=2`, `CARGO_INCREMENTAL=0`, `CARGO_PROFILE_DEV_DEBUG=1`,
`CARGO_PROFILE_TEST_DEBUG=1`, and `CI=true`, and removes inherited Rust flags and
compiler wrappers. `--frozen` prevents fetching or lockfile changes during the
slot. Two compile jobs are the concurrency limit; allow approximately 6 GiB for
build artifacts and several GiB RAM (estimates, not measured limits).

Every run creates a fresh system-temporary `werkschaft-build-slot-*` directory,
printed before commands start, with `unit-tests.log`, `debug-app.log`,
`owned-process-groups.json` and `summary.json`. Read each executed command's
complete log before reporting its outcome. Cleanup targets only process groups
created by that invocation. A nonempty `lingering_groups` is an unclean stop.
The runner neither launches the application nor contacts a daemon.

Expected outputs, not build evidence until the command succeeds:

- `target/debug/bundle/macos/Werkschaft.app`
- `target/debug/werkschaft-desktop` (also the directly launched dogfood binary)

The geometric W icon is a temporary dogfood asset. The unsigned local artifact
is not signing, notarization, or distribution acceptance.

## Allocated first-window observation

Request a separate, bounded **eight-minute native observation** scope after the
build succeeds. Use a fresh temporary evidence directory, retain only owned
shell PIDs, and reserve its last 15 seconds for TERM (5 seconds) then KILL
(10 seconds) of those PIDs. No polling, automatic rebuilds, or daemon lifecycle
commands are part of this procedure. The observer must bring the window forward
and have screen-recording permission before capture; record a missing permission
as missing evidence rather than silently substituting a browser screenshot.

1. Rediscover `WS_CACHE_HOME/dashboard/endpoint.json`, or
   `$HOME/.cache/ws@kang-sw-devenv/dashboard/endpoint.json` when the override is
   empty/unset. Record only port and PID. Perform a direct, proxy-free
   `GET http://127.0.0.1:<port>/health` with a two-second timeout and redirects
   disabled; require service `ws-dashboard` and the recorded PID. Record the
   health result as the baseline. Never print/copy the endpoint token.
2. Launch `target/debug/werkschaft-desktop` directly as an owned subprocess,
   retaining its PID and a fresh `live-window.log`. Keep the intended
   `WS_CACHE_HOME` environment. Observe the real dashboard (not merely the
   connection page or an attach log). Save a native screenshot with
   `screencapture -x <evidence-dir>/first-window.png` (five-second command cap).
   The native log records only the discovered origin, not credentials.
3. Observe one ordinary dashboard navigation, then a real external HTTP(S)
   link, including a new-window link if available. It must open the system
   browser, not another native webview. Record the destination and capture
   `<evidence-dir>/external-browser.png`. Inspect the main document's origin
   in debug Web Inspector against the exact newly discovered origin. Do not
   claim the broader terminal/clipboard/content-frame matrix from this check.
4. Close the window normally. Repeat the bounded health observation and compare
   the same endpoint PID and origin with the baseline. Do not signal that PID.
5. Exercise discovery with isolated temporary cache roots, one shell at a time:
   first a missing endpoint (connection/error view), then an endpoint whose
   port was just released by an owned temporary loopback socket (unhealthy
   view). Capture `missing-endpoint.png` and `unhealthy-endpoint.png`.
   Write only the live endpoint's port and PID to the isolated endpoint file,
   choose Connection → Retry, and capture `successful-retry.png`. This verifies
   an overridden cache root without altering the live endpoint or starting a
   second daemon. A default-cache launch and an override launch are distinct
   observations; mark default discovery unobserved if the actual live daemon
   is available only under an override.
6. Close owned shells, remove only isolated temporary caches, and record process
   cleanup plus final daemon health. On a first unexpected failure or the
   observation deadline, preserve evidence and stop rather than retrying.

The coordinating lead owns live-daemon observation permission. No `ws-mcp`
executable, daemon-start, shutdown, daemon replacement, plugin-cache installation,
Go build, or browser fixture suite is invoked here.

## Boundary and tests

`src/endpoint.rs` mirrors cache-root selection and existing health identity
validation. Endpoint serde fields are only port/PID; unknown fields, including
token, are discarded. Health is bounded to two seconds and 64 KiB, with no
proxy, redirect, or bearer header. The endpoint is re-read before navigation to
reject an attach-time change. No endpoint files are modified by the app.

`src/navigation.rs` retains only the discovered HTTP origin in the webview;
other validated HTTP(S) destinations open externally. Userinfo, file, script,
data, and custom URL schemes are blocked. Blank/srcdoc documents remain usable
for sandbox frames. WKWebView's public callback also receives frame navigation
without a frame identifier; top-level blank/srcdoc documents are consequently
allowed too, with no native permissions. Broader embedded-navigation acceptance
belongs to the later reliability slice. Internal new-window links reuse the
existing dashboard window; external new windows are denied after browser handoff.

The app registers no commands or plugins, and its explicit capability grants
zero permissions to local or remote content. The connection document has no
scripts; its CSP is not a claim about the daemon-served dashboard's CSP.

Unit tests cover default/overridden discovery, malformed/missing endpoints,
health service/PID/status/redirect/size failures, missing-then-successful retry,
credential-free health requests, exact-origin navigation, blocked schemes,
opaque frame URLs, and the no-native-authority configuration. These tests use
only owned temporary files and loopback listeners, never the live daemon.

Official exact-version API references:

- [Tauri 2.12.5 WebviewWindowBuilder](https://docs.rs/tauri/2.12.5/tauri/webview/struct.WebviewWindowBuilder.html)
- [Tauri 2.12.5 native menus](https://docs.rs/tauri/2.12.5/tauri/menu/struct.Menu.html)
- [Tauri 2.12.5 WebviewWindow](https://docs.rs/tauri/2.12.5/tauri/webview/struct.WebviewWindow.html)
- [ureq 3.5.2 configuration](https://docs.rs/ureq/3.5.2/ureq/config/struct.ConfigBuilder.html)

Single instance, daemon start/replacement, Windows, packaging with a daemon,
notifications, grants, signing and updater work remain later slices.
