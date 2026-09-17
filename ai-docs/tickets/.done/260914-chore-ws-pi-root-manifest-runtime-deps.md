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
completed: 2026-09-17
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

### Result (aa28c2a) - 2026-09-14

Landed on `impl/develop/sip-aptly-fence` in three commits: `7a60b80` (mirror
the six runtime `dependencies` into the root `package.json`; extend
`bump-ws-version.sh` with `update_root_package()`; add the release-workflow
`diff` guard and the `TestRootPackageDependenciesMirrored` Go assertion),
`053c5a6` (round-1 review fixes: gitignore the root `node_modules/` and a
generated root `package-lock.json`; add the root `package.json` to the
release-workflow PR trigger paths and jq-validate both `package.json` files;
make `update_root_package()` fail loud instead of silently defaulting to an
empty dependency map), and `aa28c2a` (capture the pi-web-access git-install
gap found during round-1 review as its own idea ticket, since fixing it was
out of Phase 1's file/decision scope — that ticket is now absorbed as this
ticket's Phase 2, see its Result below).

Re-verified in this session before starting Phase 2:
- `go build ./...`, `go vet ./...`, `go test ./...` in `agents-plugin-tool`:
  all green, including `TestPiMirrorUpToDate` and
  `TestRootPackageDependenciesMirrored` (`-run` targeted, `-count=1`, both
  PASS).
- `diff <(jq -S '.dependencies' package.json) <(jq -S '.dependencies'
  agents-plugin-pi/package.json)`: empty — root and subdir `dependencies`
  are parsed-equal, confirming the dry-run-bump invariant holds on the
  current tree without re-running the bump script.

**Outstanding:** the Phase 1 "Live develop smoke (owner-run, no publish)"
verification bullet — `pi install git:...@<develop-tip-sha>` on a clean
machine confirming extension load with no `Cannot find module` error, plus
launcher download/SHA256/`assertVersionPin`/`ws/*` registration — is
explicitly owner-run and has not been performed; it requires pushing this
branch's merge to `develop` first. Not attempted by this worker.

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

### Result (a1ae58f8) - 2026-09-14

Landed on `impl/develop/sip-aptly-fence` in two commits: `c3fd173f` (fix —
`agents-plugin-pi/src/web-search.ts` now resolves `pi-web-access` by checking
two exact known candidate directories in priority order,
`dirname(packageRoot)/node_modules/pi-web-access` (clone root, git-install)
then `packageRoot/node_modules/pi-web-access` (subdir, dev/test), using the
first that passes the existing name === `'pi-web-access'` / version ===
`'0.29.0'` / `realpathSync(dir) === dir` / `existsSync(index.ts)` checks,
preserved verbatim; the helper source path stays anchored to
`packageRoot/src/web-search-helper.mjs`; no `require.resolve` or free upward
walk introduced) and `a1ae58f8` (round-1 review fixes: thread the resolved
`root` through every post-resolution `failure()` call so the
`WebSearchError` diagnostic paths name the directory that actually resolved
rather than always the clone-root default; restore `--offline` on the new
clone-root `npm install` added to `web-package.test.ts`; add a test proving
the clone-root candidate wins priority over the subdir candidate when both
are valid).

Test coverage added: `agents-plugin-pi/test/web-search.test.ts` gained four
tests — clone-root-only layout resolves; a wrong-version clone-root
candidate falls through to a valid subdir candidate; a spoofed (symlinked)
or absent clone-root candidate fails closed with a diagnostic that still
names the clone-root candidate's paths; clone-root wins priority when both
candidates are independently valid (verified via a custom helper stub keyed
on a marker file in each candidate directory, so the test fails under a
reversed or arbitrary selection order, not just under total resolution
failure). `agents-plugin-pi/test/web-package.test.ts`'s existing
packed-install test was extended with a second, real (offline) `npm
install` against a synthetic manifest built from the actual root
`package.json`'s `dependencies` block, reproducing the true git-install
shape end-to-end with the real packed `pi-web-access` artifact (not a
synthetic stub), plus a fails-closed assertion when the dependency is
absent from both known locations.

Verification:
- `agents-plugin-pi`: `node --test test/web-search.test.ts
  test/web-package.test.ts test/web-tools.test.ts` — 21/21 pass.
- `agents-plugin-pi`: full `npm test` — 1538/1543 relevant tests pass; the
  same 5 pre-existing failures present before this change (a local
  `ws-mcp` binary version-pin mismatch, `0.46.3` installed vs `0.46.4`
  expected by `runtime.json` — an environment gap unrelated to
  `web-search.ts`, reproduced identically on a clean pre-change baseline
  run) remain, unchanged in count or cause.
- Two review rounds completed (route allocation: partitioned
  correctness + test). Round 1: correctness (opus) found the resolver
  logic and security checks correct, one Important finding (the absorbed
  idea ticket needed to be dropped with a commit note — addressed in this
  same session, see below) and two Minor findings (diagnostic anchoring,
  test `--offline`); test (sonnet) found the suite clean with two Minor
  findings (missing both-valid-priority test coverage, the same
  `--offline` gap). All four Minor/Important findings fixed in `a1ae58f8`.
  Round 2: both reviewers confirmed all round-1 findings fixed correctly
  with no regression; correctness noted one new non-blocking observation
  (no test directly asserts a post-resolution diagnostic names the
  resolved, non-clone-root directory) recorded here as `unresolved`, not
  acted on per the two-round cap.
- `idea/260914-bug-ws-pi-web-access-git-install-unresolvable.md` dropped
  (see the ticket-closeout commit) — this ticket's Phase 2 absorbs it, as
  its own frontmatter and Phase 2 text state.

**Outstanding:** the Phase 2 "Owner-run develop smoke (no publish),
Explore-role included" verification bullet — `pi install
git:...@<develop-tip-sha>` on a clean machine, extension load plus an
Explore-role web-search probe succeeding with no
`web-search-extension-missing` — is explicitly owner-run and has not been
performed; it requires pushing both phases' merge to `develop` first. Not
attempted by this worker. The git-install consumption path is therefore
verified end-to-end by automated/simulated evidence (a real npm install
into a synthetic clone-root-shaped layout using the actual mirrored root
manifest and the real packed artifact) but not yet by a genuine clean-machine
`pi install`; this ticket stays in `ready/` pending that owner action.

## Blocked (2026-09-16) — RESOLVED 2026-09-17

Resolved: the owner ran the clean-machine `pi install` smoke and it passed
(reported by the user). The sole outstanding item is complete, so this ticket
is closed to `.done/`.

Both phases are fully implemented, reviewed (two rounds), and already merged to
`develop`: commits `7a60b80`, `053c5a6`, `aa28c2a` (Phase 1) and `c3fd173f`,
`a1ae58f8` (Phase 2) are all ancestors of `develop`, the
`impl/develop/sip-aptly-fence` branch has been merged and pruned, and the
root/subdir `dependencies` drift check is parsed-equal on the current tree.

The sole remaining item is the **owner-run clean-machine smoke** named in both
phases' verification bullets: `pi install
git:github.com/kang-sw/devenv@<develop-tip-sha>` on a clean machine, confirming
extension load with no `Cannot find module`, launcher download + SHA256 +
`assertVersionPin` + `ws/*` registration, plus an Explore-role web-search probe
succeeding (no `web-search-extension-missing`). This requires a clean machine and
a real `pi install`, so no agent (worker or lead) can perform it. `ws:lead-run`
cannot advance this ticket further; it is gated on the owner. Move to `.done/`
once the owner runs the smoke and records the observed evidence.


## Resolution (2026-09-17)

Owner ran the clean-machine `pi install` git-install smoke (reported by the user) and it passed: extension loads with no `Cannot find module`, launcher downloads + SHA256-verifies the ws-mcp binary, `assertVersionPin` passes, `ws/*` register, and an Explore-role web-search probe succeeds. Both phases were already implemented, reviewed, and merged to develop; this owner verification was the sole outstanding item.
