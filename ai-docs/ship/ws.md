# Ship: ws

Ships the `ws` / `wsflow` plugin packages by promoting `develop` to `main` and
pushing a `v<version>` tag, which triggers the `ws-mcp release` GitHub Actions
workflow (`.github/workflows/ws-mcp-release.yml`) to build cross-platform
`ws-mcp` assets and publish a GitHub release.

## Version Strategy

Each ship owns exactly one patch-version bump. After confirming that local
`develop` contains `origin/develop`, read the current version from
`agents-plugin/runtime.json` `.plugin_version`, increment its patch component,
and run `agents-plugin-tool/scripts/bump-ws-version.sh <next-version>`. Commit
the script-generated version edits on `develop` before testing or tagging.
Ordinary merges into `develop` do not bump the version. The tag is `.release_tag`
(`v<plugin_version>`). Ship refuses if `.release_tag != "v" + .plugin_version`
for either `agents-plugin/runtime.json` or `agents-plugin-wsflow/runtime.json`
(the workflow enforces the same contract).

## Pre-flight

**Sync develop against origin FIRST — never ship from stale local refs.** This
is the recurring failure mode: a parallel session ships a patch, `origin/develop`
and the `v<version>` tag advance, and a local ship that skipped `fetch` promotes
stale `develop` (colliding on the version/tag). Every check below runs against
freshly fetched state.

- `git fetch origin --tags` — refresh remote-tracking refs and tags before any
  other check.
- Develop is up to date: `git merge-base --is-ancestor origin/develop develop` —
  local `develop` must contain all of `origin/develop`. If it fails,
  `origin/develop` advanced (a parallel ship or merge): **stop and reconcile
  first** — `git merge origin/develop` into `develop`, then restart Pre-flight.
  Never promote a `develop` that is behind `origin/develop`.
- Bump the next patch version through `bump-ws-version.sh` and commit the
  generated version edits on `develop`.
- Version tag unclaimed: `git ls-remote --tags origin v<version>` is empty for the
  bumped version. A non-empty result means a concurrent release claimed the
  version; increment the patch again, commit the regenerated version edits,
  re-run tests, and restart the tag check.
- `git merge-base --is-ancestor main develop` — develop must be a linear
  descendant of main (fast-forwardable); abort otherwise.
- `cd agents-plugin-tool && go test ./...` — all packages green.
- Release contract: for `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json`, assert `.release_tag == "v" + .plugin_version`.
- Marketplace sanity: `.plugins[].name` in both
  `.agents/plugins/marketplace.json` and `.claude-plugin/marketplace.json` is
  exactly `ws wsflow`.

## Build

- No local build. The tagged push drives `ws-mcp release` on GitHub Actions,
  which runs tests, validates the plugin release contract, builds release
  assets (`agents-plugin-tool/scripts/build-release-assets.sh`), and runs the
  Windows smoke.

## Tag

Format: `v<version>` (e.g. `v0.42.1`), read from `agents-plugin/runtime.json`
`.release_tag`. Placed on the `main` tip after the develop promotion.
Push: yes (final gate).

## Publish

Order is deliberate: publish the synced `develop` before promoting `main`, so
`origin/main` never advances ahead of `origin/develop`. The gate precedes every
push — all pushes below are reversible-only-before-they-run.

- `git checkout main && git merge --ff-only develop` — promote develop to main
  (matches the historical release shape: main's tip is develop's tip, no extra
  merge commit). Local only.
- Final gate: show version, tag, and publish targets; wait for explicit
  approval before any push.
- `git push origin develop` — publish the synced develop first.
- `git push origin main` — promote the release branch.
- `git push origin v<version>` — triggers the GitHub release workflow.

## Post-ship

- Watch the `ws-mcp release` workflow run for `v<version>` to green (build +
  publish GitHub release assets, windows-smoke).
- Return to `develop` (`git checkout develop`). The next develop merge resumes
  normal development without a version bump; the next ship owns the next patch
  bump.
