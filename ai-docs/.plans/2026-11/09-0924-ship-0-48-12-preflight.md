# Plan: ship ws 0.48.12 - pre-flight and version bump

Ship config: `ai-docs/ship/ws.md`. The lead already ran the release gate
(review ledger `fbd3d7e97..04ee42b63: pass`, commit e518cf860). This delegate
runs the config's remaining Pre-flight steps and the version bump only.

## Steps (from the config, in order)

1. `git fetch origin --tags`.
2. `git merge-base --is-ancestor origin/develop develop` must succeed; if not,
   stop and report (do not merge).
3. Version: read `agents-plugin/runtime.json` `.plugin_version` (expected
   0.48.11); `git ls-remote --tags origin v<current>` is expected non-empty, so
   run `agents-plugin-tool/scripts/bump-ws-version.sh 0.48.12` and commit the
   script-generated edits on `develop` (with this plan file) as
   `chore(release): bump ws to 0.48.12`, matching the shape of fbd3d7e97.
   If the current tag is not claimed, reuse the current version and report.
4. `git ls-remote --tags origin v0.48.12` must be empty; otherwise stop.
5. `git merge-base --is-ancestor main develop` must succeed.
6. `cd agents-plugin-tool && go test ./...` - all green.
7. Release contract: both `agents-plugin/runtime.json` and
   `agents-plugin-wsflow/runtime.json` have `.release_tag == "v" + .plugin_version`.
8. Marketplace sanity: `.plugins[].name` in `.agents/plugins/marketplace.json`
   and `.claude-plugin/marketplace.json` is exactly `ws wsflow`.
9. R4 pin: report `git rev-parse develop`.

## Boundary

No push of any branch or tag, no tag creation, no checkout of main. Never run
`git stash`. Return version, the pinned SHA, and each command's output (test
output may be summarized to the per-package lines plus any failure in full).
