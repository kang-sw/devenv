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

To package the macOS release app with the pinned Tauri CLI:

```sh
npm ci --ignore-scripts --no-audit --no-fund
npm run tauri -- build --bundles app --ci --no-sign -- --locked
```

The result is `target/release/bundle/macos/Werkschaft.app`. The Windows
smoke binary is `target/release/werkschaft-desktop.exe`. The bundled
daemon, installers and updater are separate implementation phases.

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
