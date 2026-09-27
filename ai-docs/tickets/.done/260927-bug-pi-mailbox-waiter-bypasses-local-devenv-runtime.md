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

#### Edition (453b7705) - 2026-09-27

Final review found that a session_start superseded during the awaited self-slug lookup could lose its captured bridge and unique bootstrap. A generation token now guards bootstrap installation and lookup completion; stale generations stop their own waiter after child exit, stop their agent tools, and close their bridge without clearing the newer generation. Concurrent-build tests hold two builds in flight, complete them out of order, and verify separate temporary/output files and mailbox copies; the failed-build test independently checks that no final output remains. The production lifecycle test now covers overlapping reloads, stable re-arm paths, stale process/bootstrap disposal, and cleanup only after child close; its executable probe runs as a real Go PE on Windows rather than being skipped. Verification: full Pi suite (1917 passed, 3 skipped), full Go suite, 43 launcher tests, 13 wsflow tests, Pi/wsflow mirror guards, and an isolated native-Windows direct-child subset (2 passed, 0 skipped). The native host's older checkout and Python Store `python3` alias prevented a full live Pi lifecycle run there; the cross-platform automated lifecycle test is in the suite. No push, tag, or release was performed.

#### Edition (e7bbcf6a) - 2026-09-27

A final-footer continuation could observe a newer epoch after shutdown began but before shutdown captured the published agent registry. Its stale cleanup then stopped the recovered children and cleared the globals, losing dormant records that startup had already read and deleted from the sidecar. Shutdown now synchronously claims its published bridge generation and captures its waiter, agent tools, sidecar path, and thread snapshot before the first await; a resumed stale continuation leaves shutdown-owned resources alone. Unpublished or self-owned stale generations still clean themselves up and persist any consumed recovery records before stopping tools. A deterministic real-Pi fixture seeds a dormant record, blocks at the final footer and after shutdown's claim, resumes the footer first, starts a newer generation, then verifies sidecar re-persistence, one teardown of the claimed tools and bridge, and an intact newer bridge. Verification: full Pi suite (1917 passed, 3 skipped), full Go suite, 43 launcher tests, 13 wsflow tests, mirror guards, and focused mailbox tests. The cross-platform direct-child and lifecycle coverage from the prior Edition remains in place; no new native-Windows full-lifecycle claim is made.

## Resolution (2026-09-27)

Threaded the bridge's validated source-build bootstrap env into the mailbox wait subprocess without changing release-backed launches. Offline waiter tests, the opt-in live local-build mailbox smoke, the full default Pi test suite, and diff checks passed. The existing unrelated opt-in manual recapture fixture remains stale under current model tuning.
