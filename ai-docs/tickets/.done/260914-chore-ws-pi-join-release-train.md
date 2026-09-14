---
title: Fold agents-plugin-pi into the ws-mcp release train
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260903-research-ws-pi-adapter-npm-distribution: research source that settled the git-install distribution model
  260906-bug-ws-pi-rsrc-mirror-drift: the mirror drift this guard is meant to prevent recurring
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: fb1affa8754882af
sage-review-completeness-reviewed: fb1affa8754882af
completed: 2026-09-14
---

# Fold agents-plugin-pi into the ws-mcp release train

## Background

`agents-plugin-pi` (package `ws-pi-bridge`) is consumed via `pi install
git:<repo>@<tag>`: the monorepo-root `package.json` manifest loads the subdir
extension, and at runtime `bin/ws-mcp-launcher.py` downloads the ws-mcp release
binary for the tag named by its bundled `runtime.json` (`release_repository`,
`release_tag`, `plugin_version`), verifying it against the release `SHA256SUMS`.

Those files under `agents-plugin-pi/` (`runtime.json`, `bin/ws-mcp-launcher.py`,
`rsrc/`) are hand-synced byte-identical copies of `agents-plugin/`'s versions.
Two gaps break the intended coupling:

- The version-bump tool (`agents-plugin-tool/scripts/bump-ws-version.sh`) updates
  `agents-plugin/` and `agents-plugin-wsflow/` but **never touches
  `agents-plugin-pi/`**, so a ws-mcp version bump silently leaves the pi mirror
  stale. This drift has already occurred (`260906-bug-ws-pi-rsrc-mirror-drift`).
- The release workflow's "Validate plugin release contract" step
  (`.github/workflows/ws-mcp-release.yml`) checks only `agents-plugin/`, so a
  drifted pi mirror can ship on a tag undetected.

The adapter's startup pin (`src/version-check.ts::assertVersionPin`) fails loud
against the live server, but only at consumer runtime — too late for a published
tag. This ticket makes the version coupling automatic at bump time and guarded
at tag time.

## Decisions

- **pi rides the shared `vX.Y.Z` ws-mcp tag; no independent version line.**
  The launcher and startup pin key on `runtime.json.plugin_version`, so pi is
  already structurally coupled to ws-mcp's version. Confirmed 2026-09-14.
  Rejected: an independent pi semver / tag line — it would need a new
  release-contract, tag-naming rule, and download-tag resolution for no benefit,
  since nothing consumer-side reads pi's own `package.json` version.
- **`bump-ws-version.sh` extends to `agents-plugin-pi/`.** On every ws version
  bump it must: (1) byte-resync `runtime.json`, `bin/ws-mcp-launcher.py`, and
  `rsrc/` from `agents-plugin/`; (2) bump the `ws-pi-bridge` `version` in **both**
  `agents-plugin-pi/package.json` and the repo-root `package.json` to the same
  version string. Rationale: the `package.json` version is cosmetic for
  git-install (the launcher never reads it), but the user chose to keep it aligned
  to avoid confusion and to keep the deferred npm path coherent.
- **CI drift guard covers all three mirrors.** Extend the "Validate plugin
  release contract" step to assert `agents-plugin-pi/{runtime.json,
  bin/ws-mcp-launcher.py, rsrc/}` are byte-identical to `agents-plugin/`'s, so a
  tag build fails loud on any drift. All three, not just `runtime.json`, because
  `bin/` and `rsrc/` are equally load-bearing hand-synced copies and `rsrc/`
  drift is exactly what `260906` was.

## Constraints

- Keep `private: true` on both `package.json` files. git-install needs no npm
  publish and npm distribution stays deferred (`260903`); do not add publish
  metadata (`license`/`repository`/`engines`/`publishConfig`, deps moves) here.
- Golden rule (`260903`): never modify ws-mcp Go source for Pi; the dependency
  stays one-directional (adapter → ws-mcp).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Prior Art

- `agents-plugin-tool/scripts/bump-ws-version.sh` — the existing bump tool to
  extend; already covers `agents-plugin/`, `agents-plugin-wsflow/`, `main.go`,
  the release script, the workflow, and docs.
- `.github/workflows/ws-mcp-release.yml` "Validate plugin release contract"
  (asserts `release_tag == "v$plugin_version"` for `agents-plugin/`) — the step
  to extend.
- `agents-plugin-tool/scripts/build-release-assets.sh`, `ai-docs/ship/ws.md`,
  `ai-docs/manuals/ws-mcp.md` — the release procedure this coupling feeds.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/scripts/bump-ws-version.sh, .github/workflows/ws-mcp-release.yml |
| scope.surface | internal | no exported symbol changes; a shell/python bump script and a CI workflow step |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | none | no existing automated test covers bump-ws-version.sh or the release-contract step (grep for "bump-ws-version" across .go/.sh/.yml matched only the script itself); Phase 1's own verification is a manual dry-run plus a deliberate-drift check |
| complexity.reuse_points | confirmed | bump-ws-version.sh already applies the same update_json/update_runtime resync pattern to agents-plugin-wsflow/ (agents-plugin-tool/scripts/bump-ws-version.sh:75-77); the workflow already has the jq/test assertion pattern to extend (.github/workflows/ws-mcp-release.yml:47-58) |
| complexity.side_effect_risk | moderate | extends the release pipeline that gates tag builds and GitHub release publishing |
| risk.correctness | moderate | must byte-resync three mirror types and bump two package.json versions correctly, or the drift this ticket exists to prevent reappears silently |
| risk.fit | low | follows the existing agents-plugin-wsflow/ resync precedent already present in both files being extended |
| risk.test | moderate | no automated test exists for this script or workflow step; verification is the manual dry-run and deliberate-drift check described in Phase 1 |
| risk.security_or_contract | moderate | changes the CI "Validate plugin release contract" gate itself; a defect could let a drifted tag ship undetected or block a good release |

## Phases

### Phase 1: Extend bump + release-contract validation to agents-plugin-pi

Extend `bump-ws-version.sh` to resync the three `agents-plugin-pi/` mirrors and
bump both `package.json` versions, and extend the release workflow's validation
step to fail on any pi-mirror drift. Follow the existing patterns in both files.
Also add `agents-plugin-pi/**` to the workflow's `pull_request` path filter
(`.github/workflows/ws-mcp-release.yml`) so a PR that drifts only a pi mirror
triggers the validation at PR time, not just at the `v*` tag build.

Verification: run the bump tool to a scratch version and confirm
`agents-plugin-pi/{runtime.json, bin/ws-mcp-launcher.py, rsrc/}` become
byte-identical to `agents-plugin/`'s and both `package.json` versions match;
then introduce a deliberate one-byte drift in a pi mirror and confirm the
validation assertions fail (simulate the workflow's shell/jq/diff checks
locally). `go test ./...` in `agents-plugin-tool` must stay green.

### Result (6c42dd4b) - 2026-09-14

Landed on `impl/develop/atop-prude-very` (4 commits: `6ea525c1`, `2beb809b`,
`66512e6f`, `6c42dd4b`), routed via `route.resolve_implement`
(delegated, review partitioned: correctness + test).

`bump-ws-version.sh` now resyncs `agents-plugin-pi/{runtime.json,
bin/ws-mcp-launcher.py, rsrc/}` byte-for-byte from the just-updated
`agents-plugin/` copies (whole-file/whole-tree resync, not independent
per-field derivation) and bumps the `ws-pi-bridge` version in both
`agents-plugin-pi/package.json` and the repo-root `package.json`. The release
workflow's "Validate plugin release contract" step gained three `diff`
checks over the same three paths, and `agents-plugin-pi/**` was added to the
`pull_request` path filter.

Verification:
- `go test ./...` and `go vet ./...` in `agents-plugin-tool`: pass (all 14
  packages).
- Manual dry-run: bumped to scratch version `9.9.9`, confirmed all three
  mirrors byte-identical and both `package.json` versions matched; injected a
  one-byte drift into `agents-plugin-pi/runtime.json` and confirmed `diff -q`
  exits 1 (matching the CI assertions), then reverted before commit.
- Round-1 test-partition review (Important): no automated regression
  coverage existed for the mirror invariant beyond the new CI-only bash
  checks. Fixed by adding `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`
  (`TestPiMirrorUpToDate`), generalizing the existing
  `TestWsflowRsrcMirrorUpToDate` pattern; verified it fails on injected drift
  and passes clean otherwise.
- Round-1 correctness-partition review (minor, fixed): `sync_tree`'s
  `rmtree`-then-`copytree` was destructive on the failure path — reworked to
  copy into a sibling `.sync-tmp` dir and swap in with one `rename`. Also
  refreshed hand-sync documentation in `ai-docs/manuals/ws-mcp.md`,
  `agents-plugin-pi/src/index.ts`, `src/version-check.ts`, and
  `test/version-check.test.ts` that still claimed "no shared sync tooling
  exists yet".
- Round 2 (both partitions): verdict clean / fixed, no Critical, no new
  blocking findings.

decisions:
- Left `agents-plugin-pi/package-lock.json`'s stale `0.1.0` version
  unaddressed — non-breaking (no `npm ci`/`install` runs in CI or release
  scripts today; reformatting a 242KB generated lockfile via `update_json`
  would add more diff noise than it fixes).
- Accepted `update_json`'s `json.dumps(indent=2)` one-time array-reformat
  churn on the two `package.json` files as inherent to reusing the existing
  helper (the ticket's own `reuse_points` precedent).
- Picked up two cheap round-2 observations outside the two rounds' named
  findings: `sync_file` switched from `shutil.copyfile` to `shutil.copy` to
  preserve the launcher's executable mode on a from-scratch recreation, and
  `ai-docs/spec/pi-adapter-runtime.md`'s Package Topology section (same
  stale "no automated sync tooling" claim, missed by round 1's four named
  files) was refreshed.
- Left `agents-plugin-pi/rsrc.sync-tmp` ungitignored (round-2 observation,
  non-blocking): the next `sync_tree` run removes it idempotently and no
  drift guard scopes outside `rsrc/` itself.
