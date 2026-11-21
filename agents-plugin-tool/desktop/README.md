# Werkschaft Tauri shell

The macOS and Windows shell attaches to the dashboard at its verified
`http://127.0.0.1:<port>/` origin. It discovers the endpoint under
`WS_CACHE_HOME` or the default ws cache root. Use **Connection → Retry**
(`Cmd+R` on macOS, `Ctrl+R` on Windows) after starting the dashboard or
changing the endpoint. Closing the shell leaves the daemon running.

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
rebuilding. These are Phase 2 smoke builds; installers and updater delivery
belong to Phase 3. Raw Cargo and Tauri builds do not prepare a packaged daemon;
use `build:package` for the validated resource layout.

Run the packaging checks with `node --test scripts/test-package.cjs`.

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
  documents. Those pages and the dashboard have no native command permissions.
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
