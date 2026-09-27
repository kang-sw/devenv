---
title: "Make the Pi mailbox waiter honor the active local-devenv runtime"
completed: 2026-09-27
---

# Make the Pi mailbox waiter honor the active local-devenv runtime

## Background

A source-loaded Pi development session with a valid `agents-plugin-pi/.local-devenv-runtime` marker successfully builds and injects a local `ws-mcp` binary for the main bridge, but its background mailbox waiter launches `ws-mcp-launcher.py mailbox wait` without that bootstrap environment. When the source tree's runtime version names an unpublished release, the waiter attempts to download that release, emits a warning, exits, and is re-armed after the five-second error backoff. The result is continuous TUI warning spam even though the main bridge is correctly using the local source build.

The launcher cannot rediscover this marker on its own: its Python local-devenv path accepts markers only from installed Codex/Claude plugin-cache layouts, while the Pi adapter's TypeScript bootstrap deliberately supports the source plugin directory. The mailbox waiter therefore needs the already-resolved local bootstrap policy threaded from the bridge/session startup rather than launching with an empty environment.

## Phases

### Phase 1: Reuse the active local bootstrap for mailbox waits

Thread the session's resolved local-devenv launcher environment into the mailbox wait subprocess without changing production installs that have no marker. Verify that source-loaded development sessions use the built local binary for both `serve --stdio` and `mailbox wait`, that normal release-backed sessions remain unchanged, and that a persistent mailbox wait failure does not produce download-warning spam caused solely by missing bootstrap propagation.

### Result (a20f5c70) - 2026-09-27

The bridge now exposes its validated local bootstrap override to the mailbox waiter; without a marker the waiter keeps its previous subprocess environment. The focused mailbox suite, opt-in live source-build mailbox smoke, full default Pi suite, and `git diff --check` passed. The user confirmed in live dogfood that the repeated `[ws-mailbox]` unpublished-release 404 warnings stopped after this hotfix. The unrelated opt-in manual recapture test failed against an existing stale workflow-manual fixture and was not changed.

#### Edition (d26cfbf9) - 2026-09-27

Release review found that each forced-bootstrap mailbox re-arm could leave a full temporary executable on Windows when the destination was still running. All three mirrored launchers now delete the bootstrap temp on success, compatible-binary fallback, and installation failure while preserving the override on every re-arm, including after a mid-session version bump. A new uncached Pi session-start test covers the real bridge-to-waiter handoff. Verification: 43 launcher Python tests, 39 focused Pi tests, full Pi suite (1914 pass, 3 skipped), Pi mirror guard, wsflow tests, opt-in live source-build mailbox smoke, and diff checks passed.

#### Edition (bb78470d) - 2026-09-27

A direct wait on the shared source-build output would lock that file on Windows and could pick up another bridge's rebuild. Pi now builds each bridge bootstrap at a unique path, stages one immutable mailbox-owned copy after connection, releases the bootstrap, and reuses the copy across every wait re-arm even if `runtime.json` changes. Cleanup runs after the direct child closes; normal async shutdown awaits it. Release-backed sessions still invoke the Python launcher and settle on its exit, avoiding a Windows grandchild-pipe shutdown delay. The earlier mirrored launcher temp cleanup remains independently tested; new Python sleep mocks are scoped. Verification: focused Pi tests (70 passed before the release-grandchild regression), full Pi suite (1917 passed, 3 skipped), full Go suite, 43 launcher tests, wsflow tests, Pi/wsflow mirror guards, opt-in real-Go staged mailbox smoke, and diff checks passed. A process crash or forced kill can leave a unique bootstrap or staged executable behind; no general cache GC was added to this hotfix.

## Resolution (2026-09-27)

Threaded the bridge's validated source-build bootstrap env into the mailbox wait subprocess without changing release-backed launches. Offline waiter tests, the opt-in live local-build mailbox smoke, the full default Pi test suite, and diff checks passed. The existing unrelated opt-in manual recapture fixture remains stale under current model tuning.
