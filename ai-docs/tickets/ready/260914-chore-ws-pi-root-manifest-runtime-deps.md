---
title: Provide the pi extension's runtime deps through the git-install root manifest
related:
  260914-chore-ws-pi-join-release-train: prerequisite
  260914-chore-ws-pi-release-path-acceptance-and-docs: unblocks
  260903-research-ws-pi-adapter-npm-distribution: origin of the git-root-install model
  260914-bug-ws-pi-web-access-git-install-unresolvable: absorbed into Phase 2; drop on implementation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 4fb272f9a2f570a7
sage-review-completeness-reviewed: 4fb272f9a2f570a7
---

# Provide the pi extension's runtime deps through the git-install root manifest

## Background

The chosen distribution model (`260903`) is `pi install
git:github.com/<user>/<repo>@<tag>`: an inert repo-root `package.json` declares
`pi.extensions: ["agents-plugin-pi/src/index.ts"]`, and Pi clones the repo, runs
`npm install` **only against the root `package.json`**, then loads the subdir
extension's raw `.ts` via jiti.

A fresh-machine Windows owner-gate of the v0.46.4 release (the first real
end-to-end git-install exercise; prior "verification" used a local-path install
where `agents-plugin-pi/node_modules` was already populated by dogfooding)
failed at extension load:

```
Failed to load extension ...agents-plugin-pi/src/index.ts:
Cannot find module 'yaml' (required from ...tool-result-render.ts)
```

Root cause: the extension has real runtime `dependencies` declared in
`agents-plugin-pi/package.json` — `@anthropic-ai/claude-agent-sdk`, `ipaddr.js`,
`jiti`, `linkedom`, `pi-web-access`, `yaml` — but the root manifest declares
**none**. Pi's single root `npm install` therefore installs nothing, and every
one of those modules is absent at load; `yaml` is merely the first one reached.
The `@earendil-works/pi-*` packages are NOT affected: they are `devDependencies`
(test-only) and are provided by the Pi host's own runtime at load time — evidence
being that load reached `yaml` in a transitively-imported module at all, which
means `index.ts`'s `@earendil-works/*` imports had already resolved.

Consequence: the v0.46.4 release binary, version pin, and mirror machinery are
all sound, but the headline git-install consumption path does not work
end-to-end on a clean machine. This blocks `260914-...-acceptance-and-docs`
Phase 1 and must be fixed and re-released before that ticket's Phase 2 docs ship.

**Second gap, surfaced by Phase 1 (folds in here, Phase 2).** Five of the six
runtime deps are ordinary `import`s that the root-manifest mirror fixes via
upward Node resolution. `pi-web-access` is NOT: `agents-plugin-pi/src/web-search.ts`
locates it by an exact path `<packageRoot>/node_modules/pi-web-access` — where
callers pass `packageRoot = dirname(dirname(extensionPath))` = `agents-plugin-pi/`
(`web-tools.ts:24`, `spawner.ts:2813,3030`) — then spawns it as a hardened
subprocess (`web-search.ts:64`), deliberately refusing `require.resolve`
(`web-search.ts:48`). So a root-only `npm install` (which populates
`<clone-root>/node_modules`) leaves `agents-plugin-pi/node_modules/pi-web-access`
absent, and Explore-role spawns still throw `web-search-extension-missing` on a
clean git-install. This is a location bug: the `packageRoot = extension dir`
assumption predates the git-root-install model (`260903`), where deps install at
the clone root, not the subdir. It supersedes the separately-filed
`idea/260914-bug-ws-pi-web-access-git-install-unresolvable`, which this ticket
now absorbs (drop it as part of Phase 2).

## Decisions

- **Fix = root manifest declares the runtime deps (chosen).** Add the six
  runtime `dependencies` (exactly the set from `agents-plugin-pi/package.json`
  `dependencies`, not `devDependencies`) to the repo-root `package.json`. Pi's
  root `npm install` then populates `<root>/node_modules`, and Node/jiti module
  resolution from `agents-plugin-pi/src/*.ts` walks up to `<root>/node_modules`
  and resolves them (the subdir is nested under root, so upward resolution
  reaches it). Keeps the no-build, raw-`.ts`-via-jiti model; `private: true`
  stays (it blocks publish, not `npm install`).
  - Rejected — **postinstall hook that runs a nested `npm install` in
    agents-plugin-pi**: keeps deps single-sourced but depends on Pi running
    lifecycle scripts on git-install and adds a second, less-deterministic
    install step.
  - Rejected — **bundle the extension (esbuild/ncc) with deps inlined**:
    reverses `260903`'s explicit no-build / raw-`.ts` decision; larger change.
- **Single source of truth = `agents-plugin-pi/package.json` `dependencies`;
  the root block is a tooling-maintained mirror.** Fold the mirror into the
  machinery `260914-chore-ws-pi-join-release-train` already built: extend
  `agents-plugin-tool/scripts/bump-ws-version.sh` to copy the subdir
  `dependencies` block into the root `package.json` on every bump, and add a
  drift guard (a check in the release workflow's contract step and a `go test`
  assertion alongside `TestPiMirrorUpToDate`) so a root/subdir `dependencies`
  divergence fails loud. `devDependencies` are NOT mirrored.
  - **Guard compares the parsed `dependencies` maps, NOT byte-identity.** Unlike
    the runtime.json/bin/rsrc mirrors (which are whole-file byte copies), the two
    `package.json` files legitimately differ elsewhere — the root carries
    `pi.extensions` and omits the subdir's `type`/`files`/`scripts`/
    `devDependencies` — so a verbatim `diff -q` / byte-equality copy of the
    existing pi-mirror precedent would always fail. Parse both and compare only
    the `dependencies` object (names + version strings).
- **No root `package-lock.json` is committed.** The six deps are exact-pinned
  version strings, so the root `npm install` is deterministic without a
  committed lock; keeping the root lock-free avoids maintaining a second large
  generated lockfile. (If the fact-populator or worker finds Pi's install path
  actually requires a root lock for reproducibility, surface it rather than
  adding one silently.)
- **pi-web-access location fix = decouple the dep lookup from `packageRoot`,
  tolerate both install locations (chosen).** `web-search.ts` uses `packageRoot`
  for two distinct things: its own source helper path
  (`<packageRoot>/src/web-search-helper.mjs`, stays at `agents-plugin-pi/`) AND
  the dep location (`<packageRoot>/node_modules/pi-web-access`). Under git-root
  install these split: source is in the subdir, deps are at the clone root.
  Resolve the `pi-web-access` directory independently, checking the known
  candidate locations in order — the clone root's `node_modules` (git-install)
  and the subdir's `node_modules` (dev/test, where `npm test` and
  `web-package.test.ts` still install) — and use the first that passes the
  existing checks. **Preserve the security property verbatim**: exact known
  directories only (never `require.resolve`'s free upward walk), the `pi-web-access`
  name + `0.29.0` version pin, the `realpathSync` equality check, and the
  `index.ts` existence check all still apply to whichever candidate is chosen.
  The helper source path stays anchored to the extension dir.
  - Rejected — **repoint `packageRoot` wholesale to the clone root**: breaks the
    helper source path (`src/web-search-helper.mjs` is in the subdir, not the
    root) and breaks dev/test, which install into the subdir `node_modules`.
  - Rejected — **postinstall nested `npm install` for pi-web-access only**:
    reintroduces the lifecycle-script dependency and a second install step this
    ticket's Phase 1 decision already rejected for the general case.
  - Deferred — **unify all modes on a single root `node_modules`** (move
    devDeps to root / workspaces, update the test harness): a larger cleanup that
    would make "look at root only" universally correct; out of scope here.
- **This ticket does not bump the ws version.** The re-release that carries the
  fix is a separate `ws:lead-ship` action; the fix is verifiable on a `develop`
  smoke without a publish (below).

## Constraints

- Mirror only the runtime `dependencies` set from
  `agents-plugin-pi/package.json` verbatim (same names and version strings); do
  not mirror `devDependencies` (Pi host provides `@earendil-works/pi-*` at
  runtime).
- Keep both `package.json` files `private: true`; no npm publish, no
  package-rename, no runtime reimplementation (`260903` transfer bounds).
- The root manifest must remain a valid Pi entry: preserve the
  `pi.extensions` field and the `name`/`version` fields unchanged except for the
  added `dependencies` block.
- Follow the shipped-surface boundary: the drift guard and bump logic live in
  repo-local tooling/CI, not in any shipped playbook. (`agents-plugin-tool/`
  changes read `ai-docs/manuals/shipped-surface-boundary.md`.)
- Do not regress the three existing `agents-plugin-pi` mirror drift guards from
  `260914-chore-ws-pi-join-release-train`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Prior Art

- `260914-chore-ws-pi-join-release-train` built the resync-on-bump +
  release-contract `diff` guard + `TestPiMirrorUpToDate` Go guard pattern for
  the `agents-plugin-pi/{runtime.json, bin, rsrc}` mirrors. The root-deps mirror
  is the same shape; reuse the same `bump-ws-version.sh` helpers, release-
  workflow contract step, and `agents-plugin-tool/internal/wsrsrc` test package.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1: package.json, agents-plugin-tool/scripts/bump-ws-version.sh, .github/workflows/ws-mcp-release.yml, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go. Phase 2: agents-plugin-pi/src/web-search.ts (+ its test agents-plugin-pi/test/web-package.test.ts) |
| scope.surface | internal | no exported Go/TS symbol changes; edits a dependency-manifest field, a bump script, and a CI workflow step |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go (TestPiMirrorUpToDate) is the existing test file the ticket names to extend |
| complexity.reuse_points | confirmed | bump-ws-version.sh's update_json resync pattern (agents-plugin-tool/scripts/bump-ws-version.sh) and the TestPiMirrorUpToDate byte-diff assertion pattern (agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go) are both direct precedent for the new dependency-mirror guard |
| complexity.side_effect_risk | moderate | extends the release-gating "Validate plugin release contract" CI step (.github/workflows/ws-mcp-release.yml:48-62) that already gates tag builds |
| risk.correctness | moderate | must mirror the exact six dependency name/version pairs and keep the drift guard in sync with future subdir dependency edits, or the failure this ticket exists to fix reappears silently |
| risk.fit | low | follows the byte-identical-mirror-plus-CI-diff-guard shape already established for runtime.json/bin/rsrc by 260914-chore-ws-pi-join-release-train |
| risk.test | moderate | no automated test yet covers the root/subdir dependencies relationship; verification is the new go test assertion plus a manual dry-run and owner-run live smoke |
| risk.security_or_contract | moderate | changes the CI "Validate plugin release contract" gate itself, and the root package.json dependencies block is the consumer-facing contract Pi's git-install npm install reads directly. Phase 2 edits agents-plugin-pi/src/web-search.ts, a security-sensitive resolver: the fix must preserve its exact-directory-only + version-pin + realpath + index.ts checks (never introduce require.resolve's upward walk) while adding the clone-root candidate location |

### Phase 1: Mirror runtime deps into the root manifest with a drift guard

Add the six runtime `dependencies` from `agents-plugin-pi/package.json` to the
repo-root `package.json`. Extend `bump-ws-version.sh` to keep that root block
in sync with the subdir `dependencies` on every bump, and add matching drift
guards (release-workflow contract `diff` check + a `go test` assertion in
`agents-plugin-tool/internal/wsrsrc`) so root/subdir dependency divergence fails
loud. Preserve `private: true`, `pi.extensions`, and the existing pi-mirror
guards.

Verification:
- `go build/vet/test ./...` in `agents-plugin-tool` green, including the new
  drift assertion; the assertion fails loud when a dep is added to the subdir
  but not the root (inject/revert to prove).
- A dry-run `bump-ws-version.sh <scratch>` leaves the root `dependencies` block
  byte-equal to the subdir's.
- **Live develop smoke (owner-run, no publish):** after this lands on `develop`
  and is pushed, `pi install git:github.com/kang-sw/devenv@<develop-tip-sha>`
  on a clean machine loads the extension without a `Cannot find module` error,
  the launcher downloads and SHA256-verifies the ws-mcp binary for the current
  `release_tag` (still `v0.46.4`, an already-published release), `assertVersionPin`
  passes (`0.46.4` == the binary's reported `serverInfo.version`), and `ws/*`
  tools register. This exercises the full chain because the launcher rides the
  existing v0.46.4 release; only a genuine re-release (with the fix) needs a new
  tag afterward.

If the smoke surfaces a further missing module or a version-pin failure, capture
it as the Result and stop before declaring the consumption path fixed.

Phase 1 is already implemented on `impl/develop/sip-aptly-fence` (commits
`7a60b80`, `053c5a6`, `aa28c2a`); Phase 2 continues on that same branch.

### Phase 2: Fix pi-web-access resolution to the actual install location

Depends on Phase 1 (same branch). Fix `agents-plugin-pi/src/web-search.ts` so
`pi-web-access` is found where the active install actually placed it, per the
Decisions "pi-web-access location fix" bullet: resolve the dep directory by
checking known candidate locations in order — the clone root's `node_modules`
(git-install) and the subdir's `node_modules` (dev/test) — and use the first
that passes the existing name + `0.29.0` version + `realpathSync` + `index.ts`
checks. Keep the helper source path anchored to the extension dir. Do not
introduce `require.resolve` or any upward free walk; the candidate set is exact
known directories only. When no candidate resolves, the `web-search-extension-missing`
diagnostic (`web-search.ts:40` builds `docs` README/manifest/config paths from
the root) should name a defensible candidate — prefer the clone-root candidate,
the git-install layout this fix targets — so the error points a consumer at the
expected location. Drop `idea/260914-bug-ws-pi-web-access-git-install-unresolvable`
(this ticket absorbs it) with a note in the commit.

Verification:
- `agents-plugin-pi` test suite green, including `test/web-package.test.ts`
  (the packed-install web-search resolution test) and any web-startup/probe
  tests; add or extend a test that asserts resolution succeeds from a clone-root
  `node_modules` layout (git-install shape) as well as the subdir layout, and
  still fails closed for a wrong-version or spoofed directory.
- The chosen candidate still rejects a `pi-web-access` whose manifest version is
  not `0.29.0` or whose realpath differs (prove the security check survives).
- **Owner-run develop smoke (no publish), Explore-role included:** after Phase 1
  + Phase 2 land on `develop` and are pushed, `pi install
  git:github.com/kang-sw/devenv@<develop-tip-sha>` on a clean machine loads the
  extension AND an Explore-role spawn's web-search probe succeeds (no
  `web-search-extension-missing`), in addition to the Phase 1 smoke criteria
  (download + SHA256 + `assertVersionPin` + `ws/*` tools). This is the
  end-to-end criterion that unblocks `260914-...-acceptance-and-docs`.

Only when this Phase 2 smoke passes is the git-install consumption path fixed
end-to-end; record the observed evidence in the Result.
