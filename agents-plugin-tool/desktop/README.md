# Werkschaft Tauri shell

Werkschaft opens the dashboard at its verified `http://127.0.0.1:<port>/`
origin. It discovers the endpoint under `WS_CACHE_HOME` or the default ws
cache root and starts the bundled daemon when needed. A newer numeric core
replaces an older daemon, including across minors. **Connection → Replace
daemon (same core)** also allows an equal-core swap. **Connection → Retry**
(`Cmd+R` on macOS, `Ctrl+R` on Windows) repeats discovery and connection.
Update and restart plugins on the previous minor after a cross-minor swap.

macOS requires Python 3 for daemon start and replacement. Windows prefers an
independent daemon process and retries inside the containing Windows job if
the OS reports access denied for breakaway. Closing the shell leaves the
daemon running. A job-lifetime daemon ends when its containing job is torn
down.

For development, set `WERKSCHAFT_DAEMON_ROOT` to an absolute path to a prepared
candidate directory before launching Tauri. It has the same layout as the
bundled input and needs the current daemon starter. A development candidate
may differ from its runtime contract by patch version. The override belongs
to the Tauri launch environment.

## Build and run

Use Rust 1.95 and the native toolchain (Xcode Command Line Tools on macOS,
MSVC on Windows). Windows runs on Microsoft Edge WebView2 Evergreen.

```sh
cargo test --locked --features custom-protocol
cargo build --release --locked --features custom-protocol
```

For a Phase 2 packaged build, first build a daemon for the target platform
with the current generated inventory embedded and matching version metadata.
For example, from the repository root on macOS arm64:

```sh
cd agents-plugin-tool
python3 scripts/generate-package-assets.py
go build -trimpath -ldflags '-s -w -X main.version=0.50.8-dev' -o /tmp/ws-mcp-darwin-arm64 ./cmd/ws-mcp
python3 scripts/write-runtime-metadata.py /tmp/ws-mcp-darwin-arm64 --version 0.50.8-dev
node desktop-electron/scripts/assemble-daemon-input.cjs --binary /tmp/ws-mcp-darwin-arm64 --out /tmp/werkschaft-daemon-input --platform darwin
cd desktop
npm ci --ignore-scripts --no-audit --no-fund
WERKSCHAFT_DAEMON_INPUT=/tmp/werkschaft-daemon-input npm run build:package
```

Use the checkout's current plugin version in both build and metadata, with
the same suffix. The assembler requires a fresh output path. The packaging
entry point validates the full package, binary architecture and embedded
inventory, then maps every file to `resource_dir/daemon`. It validates the
resulting app at `target/release/bundle/macos/Werkschaft.app`.

On Windows x64, from `agents-plugin-tool` in PowerShell:

```powershell
python scripts/generate-package-assets.py
go build -trimpath -ldflags '-s -w -X main.version=0.50.8-dev' -o "$env:TEMP\ws-mcp-windows-amd64.exe" ./cmd/ws-mcp
python scripts/write-runtime-metadata.py "$env:TEMP\ws-mcp-windows-amd64.exe" --version 0.50.8-dev
node desktop-electron/scripts/assemble-daemon-input.cjs --binary "$env:TEMP\ws-mcp-windows-amd64.exe" --out "$env:TEMP\werkschaft-daemon-input" --platform win32
cd desktop
npm ci --ignore-scripts --no-audit --no-fund
$env:WERKSCHAFT_DAEMON_INPUT = "$env:TEMP\werkschaft-daemon-input"
npm run build:package
```

The Windows smoke payload is `target/release/phase2-smoke/`, with the
executable and `daemon/` together. Remove an earlier smoke output before
rebuilding. Use `build:package` to prepare the validated resource layout
for smoke tests. See [Release packaging](RELEASE.md) for signed installers,
updates, and workflow artifacts.

Run the packaging checks with `node --test scripts/test-package.cjs`.
Run the notice browser test with `WS_PLAYWRIGHT_MODULE` pointing to
Playwright and `node --test scripts/test-daemon-notice.cjs`.

`scripts/build-slot.py` remains the bounded debug build runner from the
attach-only research. Its process-group regressions can run without Cargo:

```sh
python3 -m unittest discover -s scripts -p test_build_slot.py -v
```

## Shell boundary

- The endpoint bearer stays native. The app health-checks the recorded daemon,
  requests a machine-authenticated browser cookie, and loads the dashboard
  login gate when cookie setup fails. **Copy Browser Login Link to Clipboard**
  mints a path-scoped link and writes it directly to the system clipboard.
- The app allows its connection and Python guide only as top-level bundled
  documents. Those pages and the dashboard have no Tauri IPC permissions.
  Approved dashboard frames stay on the pinned origin or use blank/srcdoc
  documents. External HTTP(S) links open in the system browser from top-level
  links and popups. Other schemes are blocked. Popup requests never create a
  second webview.
- The renderer uses the dashboard's existing draft storage. The native close
  asks the pinned top-level renderer whether it recovered the draft and shows
  the native **Stay / Close** prompt if recovery fails or the renderer is too
  old to answer. Native status and recovery diagnostics contain fixed messages.
- The View menu provides zoom and release DevTools
  (`Cmd+Option+I` on macOS, `Ctrl+Shift+I` on Windows, or `F12`).
  Starting a second instance restores the original window.

## Shell updates

Signed installations provide the App update controls in Settings. Ordinary
checks look for stable releases. **Update to canary** looks for a newer
canary when clicked. **Back to stable** appears on canary builds and asks
before downloading the current stable build, including a downgrade.
The installed build supplies the channel and version.

The shell checks the artifact size, SHA-256, and version-bound signature
before offering a restart. Restart waits for the dashboard to save its draft.
Use the existing Connection menu for explicit daemon replacement. The
shared daemon lifecycle resumes on shell relaunch under the usual version
ordering.

If download or verification fails, check again from Settings and retry.
If installation fails, reopen Werkschaft and retry the staged update or
reinstall from the release page. On Windows, reopen the old Electron
shortcut to retry a failed first migration install. Reinstalling Electron
reopens its retained profile. Tauri uses its own WebView profile and starts
with default window settings.

On a disposable smoke host, point the signed evaluation builds at the
downloaded dry-run artifacts with:

```sh
python3 agents-plugin-tool/desktop/scripts/evaluation-feed.py --stable tauri-stable --canary tauri-canary
```

Use `--fault missing-sha`, `--fault bad-signature`, `--fault bad-bytes`,
or `--fault interrupted` for refusal and retry tests. For stable to canary
and lower-stable return in one run, use a canary from an evaluation branch
with the next patch core in the same minor.
