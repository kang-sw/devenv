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

- `git checkout main && git merge --ff-only develop` — promote develop to main
  (matches the historical release shape: main's tip is develop's tip, no extra
  merge commit).
- Final gate: show version, tag, and publish targets; wait for explicit
  approval.
- `git push origin main`
- `git push origin v<version>` — triggers the GitHub release workflow.

## Post-ship

- Watch the `ws-mcp release` workflow run for `v<version>` to green (build +
  publish GitHub release assets, windows-smoke).
- Return to `develop` (`git checkout develop`). The next develop merge resumes
  patch bumping from `v<version>`.
