---
title: Prime the exact Pi runtime before pre-release delegation
---

# Prime the exact Pi runtime before pre-release delegation

## Background

During the ws 0.46.17 release, pre-flight updated `agents-plugin-pi/runtime.json` to the new version and release tag before that tag and its runtime assets existed. A subsequently resumed Pi child loaded the updated contract, requested the not-yet-published `v0.46.17` runtime asset, and failed with HTTP 404. This created a chicken-and-egg failure: the publish delegate depended on the release artifact that it was supposed to publish.

The release was unblocked by manually building an exactly versioned local runtime into the launcher's ignored cache. A local `.local-devenv-runtime` marker also allows a restarted lead process to perform this repair, but requiring a restart or manual cache priming is not a reliable ship flow.

## Recurrence evidence — 2026-10-03

- The user reported repeated `[ws-mailbox]` HTTP 404 warnings for
  `v0.46.28/ws-mcp-darwin-arm64`; a newly dispatched Explore child also
  failed at startup with the same download error.
- The local `agents-plugin/.local-devenv-runtime` contract exists, but
  `agents-plugin-pi/.local-devenv-runtime` is absent. The active Pi package
  settings reference this repository, whose root `package.json` loads
  `agents-plugin-pi/src/index.ts`.
- `src/local-devenv.ts:readLocalDevenvMarker` reads only the Pi package's
  marker. `src/bridge.ts:startBridge` builds from that contract only for
  lead/fork roles; worker/explore roles reuse the installed runtime.
- `src/index.ts` stages a direct mailbox binary only for a locally built
  bridge. Otherwise `src/mailbox-waiter.ts:createSubprocessWait` invokes the
  Python launcher, which reads the current `runtime.json` release contract.
  That contract now names `v0.46.28`.
- No marker, runtime cache, source, or running process was changed during
  diagnosis. Recovery and fresh-child verification remain unperformed.

## Decisions

- Ship pre-flight must make the exact post-bump Pi runtime available locally after the version bump and before tests and the R4 reviewed-through pin.
- Preserve exact runtime-version and contract validation; do not accept an older runtime merely to bypass the missing release asset.
- Preserve the current worker-role policy that avoids independent source builds in every worker or explore process. Priming belongs to the release preparation path rather than ordinary child startup.
- Priming must not modify the reviewed source tree or change the reviewed-through commit during the publish-confirmation window.

## Constraints

- The primed binary must be stamped with the bumped plugin version and installed under the normal hash-keyed Pi runtime cache contract.
- A newly spawned or resumed publish child must find the primed runtime without inherited bootstrap environment state and without reaching the pending release URL.
- The behavior should apply only where the current source checkout can build the Pi runtime; ordinary downstream release consumers must continue using published assets.
- Failure to build or validate the exact runtime must stop pre-flight before publish confirmation.

## Phases

### Phase 1: Prime and verify the post-bump Pi runtime

Add a release-safe pre-flight step that builds the current `ws-mcp` source with the bumped version, installs it into the exact cache location selected by `agents-plugin-pi/runtime.json`, and validates it through the same compatibility checks used by child startup. Place the step after the bump commit and before tests and the R4 reviewed-through pin.

Cover the chicken-and-egg boundary with tests that prove a child launched before the release tag exists reuses the exact primed runtime rather than downloading the pending asset. Verify that an absent or invalid source-build configuration fails clearly, that no worker-local build policy is broadened, and that priming leaves the Git worktree and reviewed-through SHA unchanged.
