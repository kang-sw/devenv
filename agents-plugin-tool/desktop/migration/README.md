# Windows migration forwarder

Build this standalone executable with the release version and the SHA-256 of
the exact installer that will be placed alongside it. The archive must contain
`Werkschaft.exe` and `Werkschaft-setup.exe` in the same directory. For example,
in PowerShell with the MSVC Rust target and linker installed:

```powershell
$env:WERKSCHAFT_PAYLOAD_VERSION = '1.2.3'
$env:WERKSCHAFT_PAYLOAD_ID = '<64 hex SHA-256 of Werkschaft-setup.exe>'
rustc --edition 2021 --target x86_64-pc-windows-msvc -C opt-level=z -C strip=symbols agents-plugin-tool/desktop/migration/main.rs -o Werkschaft.exe
```

Build the installer for the current user with downgrade support enabled. The
forwarder expects the HKCU `Software\Microsoft\Windows\CurrentVersion\Uninstall\Werkschaft`
registration to provide `DisplayVersion`, `InstallLocation`, and
`MainBinaryName` (`werkschaft-desktop.exe`). The archive producer must derive
the ID from the actual installer and include both files in the archive covered
by Electron's SHA-256 check. The ID identifies a payload; it is not a runtime
integrity check of a file modified after extraction.

The per-user marker and lock live under LocalAppData `Werkschaft\migration`.
The marker is a small recovery file, read once per process and written only on
completion; lookup is by digest without a directory scan. A normal subsequent
launch opens the lock and marker, reads three registry values, checks the
registered executable, and starts it. The installer is never read by the
forwarder on this path. The user-requested per-payload marker and process
launch are necessary for this short-lived external forwarder. Measure actual
Windows file activity and latency during acceptance.

A failed installer or failed registration check leaves the payload retryable.
After a successful installer but before its marker is committed, interruption
or a state-write failure can cause a retry to install that payload again. Once
the marker is committed, a missing installation produces a reinstall message
and never triggers another downgrade. The Electron profile is not accessed.

Run the platform-independent state tests on macOS:

```sh
WERKSCHAFT_PAYLOAD_VERSION=1.2.3 WERKSCHAFT_PAYLOAD_ID=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef rustc --edition 2021 --test agents-plugin-tool/desktop/migration/main.rs -o /tmp/werkschaft-migration-tests && /tmp/werkschaft-migration-tests
```

Windows acceptance, using disposable user accounts and real packaged assets:

- Verify the pinned NSIS template and installed HKCU registry values match the
  contract above, including quoted install locations and version prereleases.
- From an Electron portable directory with spaces, verify first launch runs the
  bundled installer silently, waits, and opens exactly the bundled version;
  exercise a missing WebView2 runtime and the bootstrapper path.
- Install a newer canary first, then run a new stable payload and confirm the
  downgrade. Update Tauri afterward and confirm the old shortcut keeps that
  update. Repeat with two concurrent launchers and two different payloads.
- Run the same portable payload as another Windows user and confirm a separate
  per-user installation and marker. Confirm the Electron profile is untouched.
- Force installer failure, absent or invalid registration, and state-write
  failure; check readable errors and retry. After completion remove the
  installed executable and verify the reinstall message without reinstalling
  the old payload. Verify both Tauri and Electron reinstall recovery.
- Record file operations and launch latency on the Windows security-software
  baseline; verify normal launches do not read the bundled installer.
