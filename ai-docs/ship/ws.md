# Ship: ws

Ships the `ws` / `wsflow` plugin packages by promoting `develop` to `main` and
pushing a `v<version>` tag, which triggers the `ws-mcp release` GitHub Actions
workflow (`.github/workflows/ws-mcp-release.yml`) to build cross-platform
`ws-mcp` assets and publish a GitHub release.

## Version Strategy

The release version is whatever `develop` currently holds — it is NOT re-derived
or bumped at ship time. Per-merge patch bumps on `develop`
(`agents-plugin-tool/scripts/bump-ws-version.sh`) accumulate into the next
release; `develop -> main` owns no bump. Read the version from
`agents-plugin/runtime.json` `.plugin_version`; the tag is `.release_tag`
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
  first** — `git merge origin/develop` into `develop`, re-bump the plugin version
  through `bump-ws-version.sh` (the reconciliation merge into develop owns a
  bump), re-run tests, then restart Pre-flight. Never promote a `develop` that is
  behind `origin/develop`.
- Version tag unclaimed: `git ls-remote --tags origin v<version>` is empty for the
  version `develop` currently holds. A non-empty result means that version was
  already shipped remotely; re-bump `develop` to the next patch before shipping.
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
  patch bumping from `v<version>`.
