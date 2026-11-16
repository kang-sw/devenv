---
title: "Installed ws and wsflow packages omit the native Pi payload"
related:
  261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps: "completed prerequisite: native bundle freshness, dependency preflight, and existing MCP bridge fallback"
  261113-feat-ws-hosted-lead-bundled-plugin-only: "preserve bundled-only isolation and actual-host Pi resource discovery"
  260914-chore-ws-pi-root-manifest-runtime-deps: "preserve the canonical package dependency declaration and plain Pi Git-install lane"
  260912-feat-ws-pi-bounded-web-access-for-explore: "preserve package-owned pi-web-access and helper dependency resolution"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 140ae330ba5cb3e9
sage-review-completeness-reviewed: 140ae330ba5cb3e9
completed: 2026-11-16
---

# Installed ws and wsflow packages omit the native Pi payload

## Background

The dashboard resolves its bundled workflow package from its own resource root,
then `piBundledEntries` looks for a literal `agents-plugin-pi` sibling of that
package. A source checkout has that sibling, but supported installed layouts do
not reliably retain repository siblings:

- Both full and update modes of `install.sh` snapshot only `agents-plugin/` as
  `<snapshot>/ws/` (`install.sh`, snapshot-copy section).
- `scripts/install-claude-plugin.ps1` also snapshots only `ws`.
- The Claude and Codex marketplace manifests select the individual `ws` or
  `wsflow` package directories, not the whole repository
  (`.claude-plugin/marketplace.json`, `.agents/plugins/marketplace.json`). Their
  package-only caches cannot be assumed to contain `agents-plugin-pi` next to
  the installed version directory.
- `agents-plugin-tool/internal/wsagent/lead_plugins.go` resolves the owning
  workflow package through `WS_RSRC_ROOT`; `pi_lead.go` then reads the missing
  sibling manifest. Failure selects the existing MCP bridge and warning, not
  the native Pi adapter's delegation tools.

The owner-approved recovery handoff reports that manually supplying the sibling
with the native package, generated assets, production dependencies and links to
the actual Pi host restored native loading. The saved experiment result records
one default canary and one native read-only worker. That local recovery report
motivates the fix, not acceptance of a reproducible package-only distribution
or physical host-module identity. This ticket replaces the manual requirement without changing the
live installation during implementation.

An npm tarball alone is not a proven solution: `agents-plugin-pi/package.json`
currently omits `scripts/` from its `files`, while bundle freshness recomputes
inputs including `scripts/bundle.mjs`. The committed bundle still leaves
`pi-web-access` external, and its helper needs package-owned dependencies on
disk. The lockfile also contains old development host metadata; copying a
checkout `node_modules` would not establish a clean production closure or
physical host-module identity.

## Decisions

1. **Complete installed-package coverage (owner (647)=a, 2026-11-14).** Ship a
   generated package-local native Pi payload in both `ws` and `wsflow`, covering
   shell/macOS and supported Windows snapshots plus Claude and Codex
   package-only marketplace caches. Native discovery checks the owning
   workflow package's payload first and retains the existing repository
   sibling route for source/development compatibility. Rejected: fixing only
   the two snapshot installers while leaving package-only marketplace installs
   dependent on nonexistent siblings (owner option (647)=b).
2. **Prepare closure at artifact generation (owner (647)=a).** Generation may
   use the producer's dependency tooling; normal downstream update and session
   start gain no npm, dependency-download or network requirement. The shipped
   package must carry the prepared closure rather than requiring the user's
   development checkout, an npm cache, or a separately configured extension.
   Directory, generator and local materialization details are implementation
   choices within this contract, not a new distribution policy. Rejected:
   installing dependencies at session start, copying arbitrary checkout
   `node_modules`, or silently making all plugin updates require npm/network.
3. **Host Pi identity remains external (owner (647)=a and prior bundle
   decisions).** Do not inline or ship another copy of the host Pi SDK/core
   packages. Resolve host-provided classes, registries and child CLI/SDK paths
   through the actual running Pi installation; a matching version string or a
   successful import is not sufficient identity evidence. Preserve default and
   isolated modes and plain `pi install git:` behavior.
4. **Compatibility tier T0.** This adds packaging assets and an additive local
   resource-discovery route with the existing sibling route retained. It
   changes no MCP tool/argument, hello/protocol, shared-store shape, tracker
   event or config-manifest semantics. No version or minor bump is part of
   this ticket. A newly discovered need for such a semantic change returns to
   the lead rather than being absorbed.

## Constraints

- Keep package-local production artifacts free of `.local-devenv-runtime`,
  `.runtime`, development host packages, test/build caches, machine-local
  absolute paths and links to a producer checkout. Do not remove or rewrite
  the existing deliberate `ws` dogfood marker as part of this packaging fix.
- The prepared closure includes native entry and generated skills/guides,
  runtime/config/resource assets, bundle-relative wordlist, all bundle
  freshness inputs, and the external web-search/helper dependency closure.
  The canonical dependency declaration stays in
  `agents-plugin-pi/package.json`; do not introduce an independent drifting
  list of versions. Validate produced files, not lockfile annotations alone.
- Preserve `pi-web-access`'s exact package-owned version/location checks and
  helper behavior. Do not weaken them to find a user's unrelated global copy.
- Source/bundle freshness and native dependency preflight remain meaningful
  in the shipped layout. A missing, stale or unusable native payload continues
  to the existing visible bridge-fallback path; never silently claim native
  recovery from manifest presence alone.
- Generated `ws` and `wsflow` payloads carry the same native product code and
  dependency closure; package identity/defaults and resource-root selection
  must still follow the owning workflow package. Add drift checks rather than
  hand-maintaining two implementations. Preserve the snapshot installers'
  existing `ws`-only registration: shipping the wsflow payload does not
  authorize automatically installing or enabling wsflow.
- Generation and release validation must fit the existing Node-less release
  host: producer-only Node/npm steps prepare shipped artifacts; normal
  release validation can verify their completeness/freshness without
  installing dependencies. Keep plain Pi Git/package installation supported.
- Use temporary package-only and snapshot fixtures for installer, update,
  resolver and load tests. Isolate HOME, `WS_CACHE_HOME`, `WS_CONFIG_HOME` and
  any npm cache used by producer tests. No live snapshot, installed cache,
  user config, existing session or shared daemon may be restarted/replaced or
  modified under this ticket. A later live rollout remains separately gated.
- Implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md` for `agents-plugin/`,
  `agents-plugin-wsflow/` and `agents-plugin-tool/`;
  `ai-docs/manuals/wsflow-mirroring.md` for packaging/mirror and launcher
  changes; `ai-docs/manuals/release-compatibility.md` for tooling internals;
  and `ai-docs/manuals/ws-mcp.md` for MCP/launcher and installed verification.
  If implementation changes skills, guides or prompts, also read
  `ai-docs/manuals/skill-authoring.md`; if it changes the Pi harness adapter,
  read the applicable Pi SDK/extension documentation before editing.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- e39095a3 (2026-11-13, commit): "Fallback stays on the single bundleErr branch (Decision 6); nothing derives native vs bridge from the entry path." — bearing: constrains
- 52dc582b (2026-11-13, commit): "Pi restoration wraps its own in-process SDK resolver instead of pre-enumerating extensions, preserving project trust hooks, ordering, package filters and reload without initializing extensions twice." — bearing: constrains
- 14816e39 (2026-11-06, commit): "Pi integration tests that copy package files into a temp plugin dir do NOT get dashboard-config-manifest.json (lead correction)" — bearing: constrains
- ff98ed77 (2026-05-13, commit): "The user flagged wsflow auto-install through install.sh as unsafe; the local bootstrap must not install or enable wsflow in Claude." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | install.sh, scripts/install-claude-plugin.ps1, agents-plugin/, agents-plugin-wsflow/, agents-plugin-pi/, agents-plugin-tool/internal/wsagent/pi_lead.go |
| scope.surface | cross-module | Package distribution and Go-to-Pi discovery change together; piBundledEntries is unexported (agents-plugin-tool/internal/wsagent/pi_lead.go#L121-L174). |
| scope.new_public_symbol | no | No new exported symbol or model-callable tool is specified. |
| scope.new_type_contract | no | Decisions 1–4 add packaged assets and a discovery route, not a new type or signature. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/pi_lead_test.go, agents-plugin-tool/internal/wsagent/pi_bundle_test.go, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-pi/test/web-package.test.ts, agents-plugin/tests/, agents-plugin-wsflow/tests/; installed-layout fixtures must be extended. |
| complexity.reuse_points | confirmed | Read piBundledEntries and piLeadLaunch (agents-plugin-tool/internal/wsagent/pi_lead.go#L35-L98, #L121-L174), piBundleFresh (agents-plugin-tool/internal/wsagent/pi_bundle.go), and agents-plugin-pi/scripts/bundle.mjs. |
| complexity.side_effect_risk | high | Repeated snapshot replacement and package-only native selection can replace usable artifacts or select a broken extension in both isolation modes. |
| risk.correctness | high | Copied layouts must preserve freshness inputs, module-relative assets, helper resolution and physical host identity (agents-plugin-tool/internal/wsagent/pi_bundle.go; agents-plugin-pi/src/web-search.ts#L48-L112). |
| risk.fit | moderate | The shared producer must serve two package identities while retaining source-sibling discovery, plain Pi Git installation and Node-less validation. |
| risk.test | high | Existing resolver tests do not establish the new copied production closure across shell, Windows and marketplace layouts or actual host module identity. |
| risk.security_or_contract | high | Shipping third-party dependency trees must exclude host cores and producer links without weakening exact version/realpath checks (agents-plugin-pi/src/web-search.ts#L48-L68; agents-plugin-pi/scripts/bundle.mjs#L53-L56). |

## Phases

### Phase 1: Ship and resolve a complete package-local native Pi payload

Implement the approved package-local distribution in both workflow packages
with a shared producer-side generator/staging mechanism and drift validation.
First prove the clean production dependency recipe in isolated temporary
paths, including physical exclusion of host Pi cores; do not rely on the
current lockfile's `inBundle` or shrinkwrap metadata as proof. Choose the
smallest representation that lets a copied package-only plugin provide the
native closure without downstream npm/network and without links back to the
producer. Include all bundle freshness inputs and generated runtime assets.

Make native discovery prefer the owning package's generated payload while
retaining repository-sibling discovery. Preserve actual-host Pi module and
child-process identity. Ensure both snapshot installers transport the same
prepared closure, and ensure marketplace package boundaries retain it without
an installation hook that the host does not run. Wire the artifact-generation
and release/drift gates so future source, dependency, skills or bundle changes
cannot ship an incomplete or stale payload unnoticed. Update directly affected
installation documentation if its prerequisites or installed-layout description
would otherwise drift; do not rewrite broader workflow guidance.

Verification:

- Generate and consume temporary `ws` and `wsflow` package-only copies shaped
  like Claude and Codex caches, plus shell/Windows snapshot fixtures. Resolve
  from each copied package's `rsrc/`, not from the source checkout. Require a
  native entry, a fresh bundle, complete production/helper dependencies and
  no bridge warning on the usable-payload path. No ancestor checkout
  `node_modules` may accidentally satisfy the fixture.
- Verify local-first selection when both payload and repository sibling
  exist, and verify the retained sibling path in a source-layout fixture.
  Preserve existing missing/stale/invalid-entry and dependency-fallback tests.
- Load the generated extension against an actual test Pi host or an equivalent
  supported isolated SDK fixture and assert physical host class/registry
  identity and child CLI/extension selection. Test default and isolated launch
  construction. Report any native-platform execution gap explicitly.
- Inspect the produced dependency tree/archive and resolved real paths for
  forbidden host SDK/core copies, producer links/paths, dev markers and
  caches; verify exact package-owned web-search/helper resolution and the
  copied wordlist/generated skills. Validate the external dependency closure
  even though the main bundle inlines its load-time npm dependencies.
- Exercise two successive temporary snapshot updates, including stale
  destination artifacts, without npm or network. A failed preparation or
  invalid generated artifact must be diagnosed before publishing that
  unusable payload; preserve an existing usable staged payload where the
  implementation replaces one. Preserve existing Windows DryRun behavior
  and deliberate dogfood-marker semantics.
- Change a relevant source/dependency/generator/skill input in a temporary
  fixture and demonstrate that the distribution drift/freshness gate rejects
  stale output. Verify both package copies agree and the Node-less release
  validation path can check the committed/prepared artifacts without npm.
- Run relevant Go `internal/wsagent`/`internal/wsrsrc` tests, Pi package
  packaging/generation tests, both workflow package Python suites and the
  installed-layout regressions; run independent review. Use isolated test
  processes only. Record exact commands, outcomes and deferred platform/live
  rollout boundaries in the Result before claiming the ticket complete.

### Result (50b78ce18) - 2026-11-16

Implemented on `exec5/native-pi-payload`, based on `28cf9c7bc`; product commit
`50b78ce18e21bb24cf4f59c7b7b3937a43e6ff9b`. The generated bundle and both
payloads were committed once. No installed cache, user configuration, live
session, shared daemon, marketplace manifest, version or protocol was changed.
Independent review is requested from EXEC5 and remains pending; this Result
records implementation evidence, not ticket closure.

**Generation.** The producer derives the closure from the canonical Pi manifest
and lock, normalizes npm's temporary virtual root, disables scripts/automatic
peers/bin links, then validates the physical installed tree and archive before
replacement. The successful final producer commands were:

```sh
# Producer-only environment; all directories below are isolated scratch paths.
export HOME=/tmp/exec5/native-pi/current/producer/home
export USERPROFILE="$HOME"
export WS_CACHE_HOME=/tmp/exec5/native-pi/current/producer/cache
export WS_CONFIG_HOME=/tmp/exec5/native-pi/current/producer/config
export npm_config_cache=/tmp/exec5/native-pi/current/producer/npm-cache
export npm_config_userconfig=/dev/null
(cd agents-plugin-pi && npm run bundle)
python3 -B agents-plugin-pi/scripts/prepare-native-payload.py --offline \
  --npm-cache /tmp/exec5/native-pi/current/producer/npm-cache
```

Both commands exited 0. The isolated cache was populated only by the preceding
producer dependency preparation; final generation reported `9680 files, 138
production dependencies, 19074878 compressed bytes`. Test processes never run
npm or contact an external network. The web-search regression uses local HTTP
fixtures, not real providers.

**Verification, matching the seven Phase 1 bullets:**

1. **VERIFIED (macOS fixtures); native Windows INFERRED.**
   `TestPiPreparedPackageOnlyLayouts` in
   `agents-plugin-tool/internal/wsagent/pi_payload_test.go:38` copies both
   products into Claude/Codex package-only caches and shell/Windows-shaped
   snapshot paths. Each resolves from its copied `rsrc/`, verifies the fresh
   native entry, consumes it twice, and constructs default/isolated launches
   with no bridge warning. The Python snapshot-body regression and offline
   `web-package.test.ts` additionally transport and consume the prepared
   closure without checkout-ancestor dependencies. Commands: the complete Go,
   Python and Node commands in item 7, all successful. Windows-shaped paths
   and the Windows installer's Python body are not native Windows execution.
2. **VERIFIED.** The same Go fixture installs a valid repository sibling and
   proves the package-local entry still wins; corrupt local output uses the
   visible bridge, not that sibling. Existing source-sibling/invalid-entry
   tests (`pi_lead_test.go:56`), missing-bundle fallback (`:123`), and bundle
   freshness/missing-dependency fallback tests (`pi_bundle_test.go:13,46,94`)
   passed in the full `internal/wsagent` run.
3. **VERIFIED (actual Pi 1.5.2 SDK, macOS); native Windows INFERRED.** For each
   product, the following loop exited 0 with `tests 1; pass 1; fail 0`:

   ```sh
   export HOME=/tmp/exec5/native-pi/current/tests/home
   export USERPROFILE="$HOME"
   export WS_CACHE_HOME=/tmp/exec5/native-pi/current/tests/cache
   export WS_CONFIG_HOME=/tmp/exec5/native-pi/current/tests/config
   export npm_config_cache=/tmp/exec5/native-pi/current/tests/npm-cache
   for product in ws wsflow; do
     package=agents-plugin
     if [ "$product" = wsflow ]; then package=agents-plugin-wsflow; fi
     native=$(python3 -I -B "$package/bin/ws-pi-payload.py" --plugin "$package" \
       --materialize /tmp/exec5/native-pi/current/identity-native-cache)
     fixture=$(mktemp -d /private/tmp/exec5/native-pi/current/identity-final-$product.XXXXXX)
     hash=$(shasum -a 256 "$native/dist/index.js" | cut -d ' ' -f1)
     env WS_PI_TEST_SDK=/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/index.js \
       WS_PI_TEST_NATIVE_ROOT="$native" WS_PI_TEST_NATIVE_SHA256="$hash" \
       WS_PI_TEST_FIXTURE="$fixture" \
       node --test agents-plugin-tool/internal/wsagent/testdata/pi-native-payload-identity.test.mjs
   done
   ```

   Both materialized entries had SHA-256
   `32bc9519b97180749b5e7e3c500861f9e2348b871abf241e9fb1476bc74716a7`.
   The unchanged bundle loads through the actual CLI-adjacent SDK loader.
   Assertions establish physical RpcClient/ModelRegistry identity (`:307,354`),
   host TUI prototypes (`:361`), host RpcClient receiver/CLI and native child
   extension selection (`:371-380`), and use of the supplied registry (`:395`).
   MCP, socket and child-start boundaries are mocked; no model, daemon or real
   child starts. Default and isolated launch construction also passed in Go.
   Dedicated logs: `/tmp/exec5/native-pi/current/identity-final-{ws,wsflow}.log`.
4. **VERIFIED.** `ws-pi-payload.py:42-85` inspects every archive member's
   path/type/hash, rejects host Pi package manifests and checks every physical
   production-lock package/version, the exact web package and bundle header.
   All 9,680 members and 138 dependency records passed. Additional archive-byte
   inspection found zero `/Users/kang-sw/`, `/tmp/exec5/` or
   `/private/tmp/exec5/` strings and zero host Pi packages in either archive.
   Materialized tree verification rejects links and corruption. The Python
   suite verifies owner-specific copied skills and wordlist equality; the Node
   package test probes the real copied external helper with the actual host,
   proves both package-local and Git-clone-root layouts, and rejects a wrong
   or missing package-owned web dependency. All commands in items 3, 6 and 7
   passed. The first producer experiment exposed temporary dependency paths
   in esbuild labels; package-relative generation and the pre-publication
   producer-path regression (`test_native_pi_payload.py:112`) fixed it before
   the product commit.
5. **VERIFIED for both embedded snapshot bodies; Windows DryRun INFERRED.**
   `test_native_pi_payload.py:134` extracts and executes the shell and
   PowerShell installers' actual Python bodies, each twice in a temporary
   repository/home. Stale destination files disappear, only ws is registered,
   the shell marker bytes survive, and the Windows marker is BOM-less UTF-8.
   A corrupted source archive fails with `native Pi payload archive checksum
   mismatch`, leaving the previous usable snapshot byte-identical. The failed
   producer and producer-path injections (`:112,123`) likewise preserve usable
   artifacts and execute no npm. The LF owner-marker test emulates CRLF text
   translation and verifies cache reuse. The DryRun guard is checked statically
   (`:173`); neither native PowerShell nor either full installer was executed.
6. **VERIFIED.** `test_native_pi_payload.py:94` independently mutates source,
   package/dependency lock, generator and both products' skills, then adds a
   source file; the source/distribution gate rejects every stale case. Both
   archive/manifest/materializer copies agree. The following Node-less gate
   exited 0; its PATH has no Node/npm:

   ```sh
   for product in agents-plugin agents-plugin-wsflow; do
     env PATH=/usr/bin:/bin HOME=/tmp/exec5/native-pi/current/tests/home \
       WS_CACHE_HOME=/tmp/exec5/native-pi/current/tests/cache \
       WS_CONFIG_HOME=/tmp/exec5/native-pi/current/tests/config \
       /opt/homebrew/bin/python3 -I -B "$product/bin/ws-pi-payload.py" \
       --plugin "$product" --check-source .
   done
   for artifact in native-pi/manifest.json native-pi/payload.zip bin/ws-pi-payload.py; do
     cmp "agents-plugin/$artifact" "agents-plugin-wsflow/$artifact"
   done
   ```

   This is the gate wired into `build-release-assets.sh:18-26` and the release
   workflow; the cross-platform release build itself was not invoked.
7. **VERIFIED test/build matrix; independent review UNRUN, not inferred pass.**
   All test commands used isolated HOME/USERPROFILE, WS cache/config and npm
   cache directories. Exact common test environment:

   ```sh
   export HOME=/tmp/exec5/native-pi/current/tests/home USERPROFILE=/tmp/exec5/native-pi/current/tests/home
   export WS_CACHE_HOME=/tmp/exec5/native-pi/current/tests/cache
   export WS_CONFIG_HOME=/tmp/exec5/native-pi/current/tests/config
   export npm_config_cache=/tmp/exec5/native-pi/current/tests/npm-cache
   python3 -B -m unittest discover -s agents-plugin/tests
   python3 -B -m unittest discover -s agents-plugin-wsflow/tests
   env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE -u WS_PI_DELEGATION_POLICY -u WS_PI_LEAD_MEMORY_LOG \
     WS_PI_TEST_CLI=/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js \
     node --test agents-plugin-pi/test/helper-host.test.mjs \
     agents-plugin-pi/test/workflow-owner.test.ts agents-plugin-pi/test/web-package.test.ts \
     agents-plugin-pi/test/child-launch-guard.test.ts \
     agents-plugin-pi/test/native-tool-registration.test.ts agents-plugin-pi/test/skills-dir.test.ts
   env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE -u WS_PI_DELEGATION_POLICY -u WS_PI_LEAD_MEMORY_LOG \
     node --test agents-plugin-pi/test/web-search.test.ts agents-plugin-pi/test/skills-dir.test.ts \
     agents-plugin-pi/test/child-launch-guard.test.ts agents-plugin-pi/test/native-tool-registration.test.ts
   (
     cd agents-plugin-tool
     export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
     export GOPATH=/Users/kang-sw/go GOCACHE=/Users/kang-sw/Library/Caches/go-build
     export WS_CACHE_HOME=/tmp/exec5/native-pi/current/tests/go-cache
     export WS_CONFIG_HOME=/tmp/exec5/native-pi/current/tests/go-config
     go test ./internal/wsagent ./internal/wsrsrc -count=1
     go build ./...
     go vet ./...
   )
   git diff --check
   bash -n install.sh
   sh -n agents-plugin-tool/scripts/build-release-assets.sh
   ```

   Outcomes: ws Python **148 tests, OK (2 platform skips)**; wsflow Python
   **22 tests, OK (2 platform skips)**; final Node packaging/host/skills/child
   group **43/43 pass**; broader web-search/skills/child group **58/58 pass**;
   Go wsagent **ok, 81.397s**, wsrsrc **ok, 2.342s**; build/vet and syntax/diff
   checks **exit 0**. The four Python skips are the existing Windows process
   table/cmd-wrapper tests. Python also emitted existing unclosed-stream
   ResourceWarnings; there are no final test failures. Logs are under
   `/tmp/exec5/native-pi/current/` (`*-final.log`, `node-regressions-r2.log`).
   Earlier setup attempts failed on an unnormalized temporary npm virtual root
   (`Missing: build@0.50.0 from lock file`), a dependency-directory symlink
   correctly rejected by the web realpath guard, and `/tmp` versus canonical
   `/private/tmp` identity-fixture paths. Corrected isolated setup and final
   reruns passed without weakening those guards.

**Payload sizes and repository delta.** Before: no `native-pi/` in either
package (0 bytes). After, **per package**: ZIP **19,074,878 bytes** plus manifest
**1,286,181 bytes**, total **20,361,059 bytes** (**+20,361,059**). The separate
shared materializer is **8,571 bytes/package**. Both payload directories are
byte-identical, so Git can deduplicate their content. The main bundle changed
from **1,617,961** to **1,620,370 bytes** (+2,409).

**Compatibility and deferred boundaries.** T0: producer writes additive assets
(`prepare-native-payload.py:113-154`); the new reader prefers them and retains
source siblings (`pi_lead.go:124-136`). The old `28cf9c7bc` reader at
`pi_lead.go:121-129` ignores the new directory, retains the sibling route and
its existing missing-sibling bridge/warning (`:41-59`), and never rewrites the
new assets. New reopen reads the retained assets without a migration. No hard
boundary was hit. Native Windows execution (including actual PowerShell
DryRun), independent review, real marketplace activation and live rollout
remain unrun. EXEC5 owns independent review; live snapshot/cache/daemon/session
replacement remains separately owner-gated. The ticket stays in `ready/`.

#### Edition (7475f2059) - 2026-11-16

EXEC5 review round 1 on the rebased branch (`ea80c8103`; original implementation
and Result are now `50b78ce18` and `ed1a7eb60`). Source fixes:
`91c0410bd0f3c6c4ba3c998ca7b04324781fbefc`. Last product commit, containing the
round's **single** bundle/payload regeneration:
`7475f2059daee2e6c23c4dc568f9b8553d6fdd88`. The preceding Result is unchanged.

- **I1 VERIFIED.** `StalePayload` separates source drift from archive invalidity.
  With an existing source sibling, `--prefer-source` validates first, keeps a
  current local payload, and selects the sibling only on source drift while
  logging the stale input and regeneration command to stderr. Invalid local
  archives still take the visible bridge, never a sibling substitute.
  `pi_payload_review_test.go:15,51` proves current source -> local, changed source
  -> sibling with no bridge warning, and an internally stale package-only
  bundle -> bridge warning; `pi_payload_test.go:65` retains package-only and
  corrupt-local-with-sibling coverage. The verbose selection test printed
  `using repository sibling: ... stale: agents-plugin-pi/src/index.ts` and the
  regeneration command, and passed.
- **I2 VERIFIED.** `agents-plugin/bin/ws-pi-payload.py:161-226` checks every
  expected file byte and all other files/directories, allowing only a real
  top-level `.runtime/` state directory. A failed hit re-extracts into a private
  stage, verifies it, and publishes by directory rename with rollback. A healthy
  hit preserves launcher state; repair replaces the damaged closure (including
  its disposable runtime downloads). `test_native_pi_payload.py:65` adds
  `.runtime/test-platform/last-launch-error` and proves reuse, then independently
  mutates an expected file, adds an unauthorized file and adds an empty
  directory; all three are repaired without manual deletion.
- **I3 VERIFIED.** `drift_input` (`ws-pi-payload.py:34`) and producer copying
  exclude owner-overlaid skills/rsrc, runtime, dashboard defaults and launcher
  from source drift inputs. All native sources, bundle script, lock, package
  manifest, dist/wordlist, generator, native guides and non-overlaid config
  remain covered. The freshness algorithm is unchanged from
  `agents-plugin-tool/internal/wsagent/pi_bundle.go:35-66`.
  `.DS_Store`/`Thumbs.db` are omitted from producer and source/overlay inventories.
  Whole-tree owner replacement removes obsolete archive resources too.
  `test_native_pi_payload.py:112` proves bundle-input edits still fail while
  either package's skills and owner resources/junk do not cause source drift;
  materialized owner resources are exact, including deletions.
  Tests at `:186,226` prove stale source is a loud installer warning naming the
  input/regeneration command, while invalid archives remain fatal. Bare
  `--check-source` still exits 2 on stale input, so release gates remain fatal.
  Native bundle freshness passed in Go; both actual SDK fixtures loaded the
  complete carried owner skill inventory with zero diagnostics
  (`pi-native-payload-identity.test.mjs:300-307`).
- **I4 VERIFIED.** `agents-plugin-pi/test/web-package.test.ts:48` restores
  `npm pack --dry-run --json --ignore-scripts --offline`, with isolated HOME and
  an empty npm cache. It requires both helpers and bundled pi-web-access
  README/LICENSE, pins bundledDependencies/peerDependencies, and excludes all
  three host cores from the pack inventory. Prepared copies assert all three
  absent cores and run helper probes by default using the local development
  host when `WS_PI_TEST_CLI` is unset. The exact final Node run unset that
  variable and passed. The real tarball install/update and tracked-lock
  Git-install checks remain at `:118`, explicitly gated by
  `WS_PI_TEST_NETWORK=1`; that lane was **not run** under the ticket's offline
  rule. No packing assertion was discarded to obtain a pass.
- **I5 VERIFIED.** `pi_payload.go:37-40` allows 300s, based on the review's
  approximately 28s cold macOS extraction of 9,680 files/~70MB. A fresh isolated
  extraction here measured **1.61s real** (`/usr/bin/time -p`, exit 0); the
  larger bound allows slower disks/antivirus. `cleanup_stages` (`:147` in the
  Python helper) removes only owned UUID stages or legacy mkdtemp stage names
  older than one hour, never recent stages or completed cache entries.
  `test_native_pi_payload.py:166` proves abandoned-stage cleanup and preservation
  of a recent stage and an unrelated directory. This is not cache GC.
- **M1 VERIFIED.** `pi_payload.go:43-55` separates stdout/stderr; selection
  diagnostics are logged without entering the returned path and failure errors
  include stderr. `TestPiPreparedPayloadSeparatesDiagnostics` at
  `pi_payload_review_test.go:92` passed both success and failure assertions.
- **M3 VERIFIED.** `MATERIALIZER_SCHEMA = 2` participates in the cache key
  (`ws-pi-payload.py:19,183`); `test_native_pi_payload.py:156` proves schema
  changes choose a different entry while leaving the previous entry intact.
- **M5 VERIFIED.** `workflow-owner.ts:13` sets wsflow's existing `setup` value,
  retaining `ws.setup` for ws. The owner unit test compares against the actual
  package `.mcp.json`, and both real-host identity fixtures assert the resulting
  child environment. No manifest/API tool name changed. M2/M4/M6 were not done.

**Exact final verification commands.** Commands below used the isolated test
HOME/USERPROFILE and WS/npm cache/config paths, with no external network:

```sh
export HOME=/tmp/exec5/native-pi/round1/home USERPROFILE=/tmp/exec5/native-pi/round1/home
export WS_CACHE_HOME=/tmp/exec5/native-pi/round1/cache
export WS_CONFIG_HOME=/tmp/exec5/native-pi/round1/config
export npm_config_cache=/tmp/exec5/native-pi/round1/npm-cache
python3 -B -m unittest discover -s agents-plugin/tests
python3 -B -m unittest discover -s agents-plugin-wsflow/tests
env -u WS_PI_TEST_CLI -u WS_PI_TEST_NETWORK -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE \
  -u WS_PI_DELEGATION_POLICY -u WS_PI_LEAD_MEMORY_LOG node --test \
  agents-plugin-pi/test/helper-host.test.mjs agents-plugin-pi/test/workflow-owner.test.ts \
  agents-plugin-pi/test/web-package.test.ts agents-plugin-pi/test/child-launch-guard.test.ts \
  agents-plugin-pi/test/native-tool-registration.test.ts agents-plugin-pi/test/skills-dir.test.ts
(
  cd agents-plugin-tool
  export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
  export GOPATH=/Users/kang-sw/go GOCACHE=/Users/kang-sw/Library/Caches/go-build
  export WS_CACHE_HOME=/tmp/exec5/native-pi/round1/go-cache
  export WS_CONFIG_HOME=/tmp/exec5/native-pi/round1/go-config
  go test ./internal/wsagent ./internal/wsrsrc -count=1
  go test ./internal/wsagent -count=1 -v \
    -run 'TestPiPrepared(SourceDriftUsesSiblingWithoutBridge|PayloadSeparatesDiagnostics)'
  go build ./...
  go vet ./...
)
for product in agents-plugin agents-plugin-wsflow; do
  env PATH=/usr/bin:/bin HOME=/tmp/exec5/native-pi/round1/home \
    WS_CACHE_HOME=/tmp/exec5/native-pi/round1/cache \
    WS_CONFIG_HOME=/tmp/exec5/native-pi/round1/config \
    /opt/homebrew/bin/python3 -I -B "$product/bin/ws-pi-payload.py" \
    --plugin "$product" --check-source "$PWD"
done
for artifact in native-pi/manifest.json native-pi/payload.zip bin/ws-pi-payload.py; do
  cmp "agents-plugin/$artifact" "agents-plugin-wsflow/$artifact"
done
```

Outcomes: Go wsagent **ok (80.872s)**, wsrsrc **ok (1.583s)**; verbose selection
and stderr tests **PASS (4.008s)**; build/vet and Node-less gates **exit 0**.
Python **151 tests OK (2 existing Windows skips)** and **22 tests OK (2 existing
Windows skips)**; Node **44 passed, 1 intentional network-lane skip, 0 failed**.
The existing Python unclosed-stream ResourceWarnings remain non-failing.

Both actual Pi **1.5.2 CLI-adjacent SDK** identity tests also passed (one test per
product, no skips), using the preceding Result's identity loop with every
`current` scratch-path component changed to `round1` and canonical
`/private/tmp/exec5/native-pi/round1/identity-final-<product>.XXXXXX` fixture paths.
The new entry SHA-256 is
`af1e77fb4a83b50be87da5e518f1509c5b2787bcadd43a8fd9b375a245b732fd`
in both packages. Physical host classes/registry, exact child CLI/extension,
owner setup environment, full carried skills and unchanged-payload teardown
assertions all passed. Logs: `/tmp/exec5/native-pi/round1/identity-{ws,wsflow}.log`.
MCP/socket/child-start boundaries remained mocked; no live daemon or provider ran.

The only producer regeneration command was
`python3 -B agents-plugin-pi/scripts/prepare-native-payload.py --offline --npm-cache /tmp/exec5/native-pi/current/producer/npm-cache`,
after `npm run bundle` under the original isolated producer environment. Both
exited 0. Both archives contain **9,680 files / 138 production dependencies** and
**95 drift inputs**; byte inspection found **zero host cores, producer paths or
OS junk**. All three mirrored distribution files compare byte-identically.

**New payload size per package:** ZIP **19,074,902 bytes**, manifest **1,275,049
bytes**, total **20,349,951 bytes** (**-11,108 bytes** versus the pre-review
payload; **+20,349,951 bytes** versus develop's absent payload). Bundle:
**1,620,395 bytes** (+25). Generated artifacts were committed once in this round,
in the last product commit.

**Compatibility/boundaries:** T0. The producer writes additive package assets
(`prepare-native-payload.py:126-153`); the new reader retains sibling/bridge
behavior (`pi_lead.go:125-137`). The pre-ticket `ea80c8103` reader at
`pi_lead.go:121-129` ignores these assets and keeps its prior sibling/bridge path
(`:41-59`); neither reader rewrites them, and no migration is needed on reopen.
No hard boundary was hit. Native Windows/PowerShell, the opt-in real npm install
lane, live marketplace activation and rollout remain unrun. Re-review is handed
to EXEC5; the ticket remains `ready/` and the original live-rollout gate remains.

#### Edition (def63cbc6) - 2026-11-16

Round 2 fixes N1/N2 in `def63cbc6061f02eb5330e82099ab5133bda3102`; earlier
Result/Edition text is unchanged. No archive or bundle regeneration ran.

- **N1 VERIFIED.** `ignored_path` (`agents-plugin/bin/ws-pi-payload.py:34`) is
  the shared rule for source scans, the existing producer copy loop and owner
  overlays: OS junk, `__pycache__/`, `.pyc`/`.pyo`, `.swp` and backup `~` names.
  `bin/` was verified to contain only the overlaid launcher and removed from
  the added-input scan. The external consumer's historical input hash is now
  provenance only: it is outside the archive, so consumer-only fixes need no
  native rebuild. All actual producer/native/bundle inputs remain gated.
  `test_native_pi_payload.py:160` plants bytecode under both bin and src plus
  the other junk forms, proves source validation and local-first materialization
  succeed without stderr warnings, and exercises the actual producer copy/ZIP
  loops to prove junk is omitted. npm is mocked and generation stops before
  validation/publication; no production artifact is replaced by that test.
- **N2 VERIFIED (injected PermissionError; native Windows unrun).** Cleanup at
  `ws-pi-payload.py:239-243` is best effort, as is removal of temporarily locked
  stale stages. `test_native_pi_payload.py:208` denies both cleanup attempts,
  proves the valid published entry is still returned, then removes the lock
  and proves safe-age cleanup reaps the leftovers on the next invocation.
  N3/N4/N5 were not addressed.

Verification used isolated HOME/USERPROFILE, WS cache/config and npm cache under
`/tmp/exec5/native-pi/round2/`, with no network or dependency tooling:

- `python3 -B -m unittest discover -s agents-plugin/tests`: **153 tests, OK
  (2 existing Windows skips)**; wsflow equivalent: **22 tests, OK (2 existing
  Windows skips)**. Existing unclosed-stream ResourceWarnings are non-failing.
- `go test -count=1 ./internal/wsagent -run 'Pi'` from `agents-plugin-tool`, with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and existing offline Go caches:
  **ok, 91.184s**.
- Both `python3 -I -B <pkg>/bin/ws-pi-payload.py --plugin <pkg> --check-source
  "$PWD"` gates passed with Node/npm absent from PATH; `cmp` of manifest, ZIP
  and materializer between packages passed.
- Both ZIPs were captured using `git show HEAD:<pkg>/native-pi/payload.zip`
  before committing (HEAD was `bab53836b`), then compared with `cmp`: **identical**.
  Both manifests also compare identical to the pre-round manifest blob.
- Targeted verbose junk and locked-cleanup regressions passed individually
  (**2.722s**, **3.204s**); `git diff --check` passed. Logs are in the round2
  scratch directory. All earlier ticket text remains a byte-identical prefix.

Payload unchanged: **19,074,902-byte ZIP + 1,275,049-byte manifest = 20,349,951
bytes/package**; repository payload delta this round is **0 bytes**. T0;
no hard boundary hit. Native Windows/PowerShell and live rollout remain unrun;
final review remains with EXEC5 and the ticket stays `ready/`.

#### Edition (8621f4cc6) - 2026-11-16

MAIN's binding split/prune decision under owner grant **(1170)=a** replaces the
monolithic layout to prevent source churn from adding permanent public binary
blobs. Code/tests/gates: `754fb767deb4f48ff1cc61aaf9e2178f3d3abdcf`.
Generated native files: `8621f4cc6693ef4a2b441f9bd5adaf669346f3dc`.
This worktree regenerated once, using the warmed isolated producer cache with
`--offline`; the old payload.zip and manifest.json are deleted. Earlier ticket
text is unchanged. The lead owns the eventual history rewrite; this worker did
not rewrite, squash, push or merge anything.

**Layout and freshness.** `closure.zip` contains only `node_modules/`; its
`closure-manifest.json` contains the normalized production lock, canonical
dependency declaration, prune-rule version **2**, archive SHA and dependency
file hashes. The closure key is
`0abbaf013d0dd47ae1fa8b0946e1b69f003e2e69cfbe34c55887dd65f48e5183`.
A same-key/different-archive-or-manifest result fails generation. No first-party
source hashes enter that large manifest.

Eleven small plain files live in `native-pi/first-party/`, with a separate small
`first-party-manifest.json`: the bundle, wordlist, synthetic runtime package
manifest, Pi config manifest, four guides, bundle input record and the **two
standalone MJS helpers actually spawned/imported**. No TypeScript source tree,
lock or build scripts are copied into the runtime entry. The complete source
input hashes remain in the small manifest for producer/source drift gates.
The runtime package declares `dist/index.js`; Python verifies its hashes/header,
and Go uses the recorded digest only on an owner-marked prepared entry.
Source checkouts keep the original `piBundleInputHash` path unchanged. Skills,
rsrc, runtime/dashboard defaults and launcher reuse the owning package's files.

Materializer schema **3** keys assembly by closure key, first-party digest and
owner overlays. It retains strict verification, `.runtime` tolerance,
self-repair, safe-age stage cleanup, best-effort deletion, common junk filtering,
separate stderr and the 300s budget. Separate extracted-closure caching was
skipped to avoid another sharing/locking layer; this does not affect Git cost.

**C8: sizes, per package.** The old ZIP/manifest combined both domains; its
first-party count includes the old source/build/resource duplicates. New owner
resources already shipped at package root are reused, not counted twice below.

| Measure | Before: monolithic | After: split/pruned |
|---|---:|---:|
| ZIP bytes (old combined / new closure only) | 19,074,902 | 7,284,712 |
| Large manifest bytes | 1,275,049 | 725,054 |
| Small first-party manifest bytes | 0 (inside combined metadata) | 11,990 |
| First-party file bytes | 4,223,174 (inside ZIP) | 1,725,978 (plain files) |
| First-party file count | 165 | 11 |
| Dependency file count | 9,515 | 5,571 |
| Total payload member count, excluding manifests | 9,680 | 5,582 |
| Dependency unpacked bytes | 40,749,096 | 21,547,906 |
| Total payload unpacked bytes | 44,972,270 | 23,273,884 |
| Native directory bytes, archive + metadata + plain files | 20,349,951 | 9,747,734 |

The closure preserves **138 redistribution-notice files and 157 package.json
files byte-for-byte** against the original pinned archive. The additional 19
embedded manifests are module/type markers with no main/module/exports/bin/
browser/imports fields. All retained dependency file bytes match the original
archive, and every one of the 138 production packages' declared local runtime
targets is asserted before and after pruning and again during validation.

| Prune rule | Files removed | Unpacked bytes removed |
|---|---:|---:|
| Media outside dist/lib, unless runtime-referenced | 2 | 6,403,101 |
| docs/doc/examples/example/test/tests/__tests__/benchmark(s)/.github trees | 1,254 | 6,870,349 |
| Non-legal Markdown | 238 | 1,499,631 |
| Unreferenced source maps | 372 | 1,472,047 |
| Unreferenced type declarations | 2,014 | 2,060,026 |
| TypeScript build metadata | 0 | 0 |
| Redundant src with compiled output and no runtime path into src | 64 | 896,036 |
| **Total** | **3,944** | **19,201,190** |

Rules are in the named `prune_rule`, applied only under node_modules, with
comments per rule. Legal files and package.json always survive. **68** candidate
removals are explicitly retained because manifest targets or transitive literal
runtime imports/resources need them. This narrows declaration pruning for
exported types and directory pruning for YAML's actual `dist/doc` runtime
modules. Temml declares `"./*":"./*"`, so its src, exported documentation and
SVG cannot be dropped under the hard target-preservation rule. No manifest was
rewritten to hide missing files. Nonlisted `.yarn` tooling was conservatively
retained and reported to EXEC5 rather than silently extending the reviewed list.

**Top 15 remaining closure files (unpacked bytes):**

| File under node_modules/ | Bytes |
|---|---:|
| unpdf/dist/pdfjs.mjs | 1,676,063 |
| jiti/dist/babel.cjs | 1,526,691 |
| @mixmark-io/domino/.yarn/plugins/@yarnpkg/plugin-version.cjs | 1,073,677 |
| defuddle/dist/index.full.js | 760,734 |
| temml/dist/temml.cjs | 479,810 |
| temml/dist/temml.mjs | 479,800 |
| temml/dist/temml.js | 425,056 |
| linkedom/worker.js | 421,658 |
| pi-web-access/node_modules/linkedom/worker.js | 414,459 |
| defuddle/node_modules/linkedom/worker.js | 411,521 |
| defuddle/dist/index.js | 334,960 |
| @mixmark-io/domino/lib/HTMLParser.js | 252,315 |
| jiti/dist/jiti.cjs | 190,082 |
| mathml-to-latex/dist/bundle.min.js | 172,706 |
| mathml-to-latex/dist/index.cjs | 168,343 |

**Acceptance evidence (C1-C8).** All execution used isolated HOME/USERPROFILE,
WS cache/config and npm cache under `/tmp/exec5/native-pi/round3/`, except the
producer's previously warmed isolated cache. No external network test ran.

1. **VERIFIED:** both actual Pi **1.5.2 CLI-adjacent SDK** identity fixtures
   passed, one test per package, zero skips. The unchanged bundle entry SHA is
   `af1e77fb4a83b50be87da5e518f1509c5b2787bcadd43a8fd9b375a245b732fd`.
   The test is `agents-plugin-tool/internal/wsagent/testdata/pi-native-payload-identity.test.mjs`;
   the prior Result's identity loop was run on both new packages using canonical
   `/private/tmp/exec5/native-pi/round3/identity-final-<product>.XXXXXX` roots and
   `final-native-cache`. Physical RpcClient/registry/TUI identity, child CLI and
   extension choice, exact owner skill inventory and unchanged-entry teardown
   all passed. MCP/socket/child-start boundaries remain mocked.
2. **VERIFIED:** `node --test agents-plugin-pi/test/web-package.test.ts
   agents-plugin-pi/test/helper-host.test.mjs agents-plugin-pi/test/workflow-owner.test.ts`
   with WS_PI_TEST_CLI/WS_PI_TEST_NETWORK and role flags unset: **6 passed,
   1 deliberate network-install skip, 0 failed**. The existing helper probe
   loads pi-web-access from the materialized entry and captures/validates its
   search registration; it sends no query and invokes no provider. A separate
   `createWebSearch({packageRoot: materialized, env}).probe()` smoke ran for both
   new entries against the actual 1.5.2 CLI and passed, with realpaths inside
   each entry's own node_modules. Logs: `node.log`, `helper-smoke.log`.
3. **VERIFIED:** `python3 -B -m unittest discover -s agents-plugin/tests`:
   **158 tests OK (2 existing Windows skips)**; wsflow equivalent:
   **22 tests OK (2 existing Windows skips)**. Existing runtime, rollback,
   corruption repair, `.runtime`, marker bytes, source selection, stage cleanup,
   junk and snapshot assertions remain tested on the new representation.
   New tests assert pruning, legal preservation, all manifest target fields,
   wildcard/browser targets and source/dependency freshness separation.
   Existing unclosed-stream ResourceWarnings remain non-failing.
4. **VERIFIED:** from agents-plugin-tool, with
   `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and existing offline Go caches:
   `go test -count=1 ./internal/wsagent -run 'Pi'` **ok, 76.530s**;
   `go test -count=1 ./internal/wsrsrc` **ok, 1.162s**;
   `go build ./...` and `go vet ./...` **exit 0**. Prepared-record freshness
   without source is tested while the original source freshness tests still pass.
5. **VERIFIED:** both `python3 -I -B <pkg>/bin/ws-pi-payload.py --plugin <pkg>
   --check-source "$PWD"` gates pass with PATH=/usr/bin:/bin and the absolute
   Python interpreter (no Node/npm). `cmp` passes for closure.zip, closure
   manifest, first-party manifest and materializer; recursive `diff` of both
   first-party directories passes. Build/release gates check the same files;
   both installers keep warning-on-stale/fatal-on-invalid through the existing
   shared flags, with separate component diagnostics.
6. **VERIFIED:** `test_closure_and_first_party_staleness_have_distinct_gates`
   passes. Additional actual CLI fixture runs changed the dependency declaration,
   then a production lock record, then src/index.ts. They emitted respectively
   `CLOSURE stale: ...package.json dependencies`, `CLOSURE stale:
   ...package-lock.json production closure`, and `FIRST-PARTY stale:
   ...src/index.ts`. Every bare gate exited **2**; every installer
   `--allow-stale-source` gate exited **0 with WARNING**. Every message named
   the source input and `npm run bundle && python3 scripts/prepare-native-payload.py`.
   Invalid archive/first-party bytes remain fatal. Log: `gate-cli.log`.
7. **VERIFIED:** the required experiment used a plain `git clone --no-hardlinks
   --local --single-branch --branch exec5/native-pi-payload`, not a worktree,
   under `/tmp/exec5/native-pi/round3/delta-clone`. After applying the candidate
   code and committing regenerated baseline `918620a26c681cfdfeafa50ab6b84e1b4dca75c3`,
   it appended a trivial src/index.ts comment, ran `npm run bundle` and the
   offline producer, then committed `392a9d8570ff9a61fb0eeba3f35d0fb0418415d4`.
   `git diff --stat HEAD~1` showed only **8 first-party paths, 16 insertions,
   14 deletions** (original src/bundle plus each package's small manifest,
   bundle record and bundle). **No closure ZIP or closure manifest changed.**
   The four distinct new blobs are 1,620,395-byte bundle, 84,847-byte original
   source, 11,990-byte small manifest and 97-byte record: **1,717,329 logical
   uncompressed bytes**, with **1,619 bytes in a thin incremental pack** from
   `git pack-objects --stdout --thin --revs` over HEAD excluding HEAD~1.
   That pack size is an incremental fixture measurement, not a claim about a
   later whole-repository GC. The plain clone was deleted. Receipts:
   `clone-delta-stat.txt`, `delta-proof.json`, `source-delta.thin.pack`.
8. **VERIFIED:** the preceding size/removal/largest-file tables come from the
   old ZIP inventory and final producer report, `size-comparison.json` and
   `final-report.json`. Both products are byte-identical. Source-derived input
   hashes and each bundle/header record are now confined to small plain files.

The main producer command was
`python3 -B agents-plugin-pi/scripts/prepare-native-payload.py --offline --npm-cache /tmp/exec5/native-pi/current/producer/npm-cache --report /tmp/exec5/native-pi/round3/final-report.json`.
It exited 0, once in this worktree. Clone preflight/proof generations never
published into this checkout. All logs/receipts named above are under the round3
scratch directory. No forbidden host packages, producer links or producer paths
were found in the new closure. The two MJS source files remain because the
runtime actually spawns/imports them; all other native src stays out of the entry.

**Compatibility and remaining boundaries.** T0: the producer writes the split
assets at `prepare-native-payload.py:197-246`; validation/read/assembly is in
`ws-pi-payload.py:183-260,287-345`. The old `998cb3fe3` binary reads only the
repository sibling at `pi_lead.go:123-129`, never native-pi, and retains its
missing-sibling bridge/warning at `:41-58`. The new `pi_lead.go:125-137` keeps
that sibling route and the existing invalid-payload bridge. Prepared Go
freshness uses a verified record at `pi_bundle.go:98-115`; source freshness
continues recomputing at `:35-66`. Neither binary rewrites shared data or changes
protocol/config/tool semantics, and no migration or T3/version boundary exists.
Native Windows/PowerShell, the opt-in network install lane, actual provider
queries and live rollout remain unrun. The ticket stays ready for EXEC5 review
and the owner-gated rollout; no install hook, marketplace semantic change,
version bump, forbidden whiteboard edit or daemon operation was performed.

**History rewrite (EXEC5 lead, MAIN decision 2 under (1170)=a).** Before
merge, the unpushed branch was rewritten so that exactly one payload generation
exists in its history. The superseded single-archive `native-pi/payload.zip`
and `native-pi/manifest.json` were removed from every commit before
`8621f4cc6`. That commit is now the only one adding generated archive blobs: one
`closure.zip` blob (7,284,712 B) and one `closure-manifest.json` blob
(725,054 B), each shared by ws and wsflow. As a result, commits `50b78ce18`
through `754fb767d` carry no committed native payload, and their Results'
payload-size figures describe the pre-rewrite generations. `7475f2059` now
regenerates only `agents-plugin-pi/dist/index.js`. Commit hashes in this
ticket were repointed to the rewritten commits. The final tree is
byte-identical to the pre-rewrite head `0fd0711ba`.

#### Edition (dcd677d08) - 2026-11-16

Round 4 fixes: `dcd677d08`.
The single post-code producer run changed only the two small first-party
manifests in `6d840e2f3`; no closure blob,
closure files hash, key, prune version, bundle or runtime file changed.
The earlier Result, Editions and lead history-rewrite note are unchanged.

- **Important 1 VERIFIED.** All plain-file inventory walks used by validation,
  source checking and materialization now use the shared junk filter, including
  staged and cached entries. Listed file hashes remain mandatory; real extras
  remain invalid. `test_plain_first_party_ignores_junk_but_rejects_unlisted_files`
  (`agents-plugin/tests/test_native_pi_payload.py:68`) adds `.DS_Store`,
  `dist/Thumbs.db` and `__pycache__/x.pyc` under first-party, proves validate,
  check_source and materialize succeed, checks junk is not copied, and proves
  an unlisted non-junk file fails all three operations. It also checks benign
  junk in an existing cache entry does not invalidate that entry.
- **Important 2 VERIFIED.** `retain_committed_closure`
  (`agents-plugin-pi/scripts/prepare-native-payload.py:170`) compares a same-key
  candidate's complete uncompressed path-to-SHA map, not its compressed bytes.
  It reuses both committed ZIP and raw manifest bytes and skips the compressor.
  Different content is a hard error: `same closure key, different CONTENT:
  key-input bug or prune change without a version bump`. Existing mirrored
  committed copies must agree; retained data is validated before publication.
  The unit test uses a stored/creator-0 candidate versus a deflated/creator-3
  committed archive with identical contents, proves both committed files and
  the retained output stay byte-exact, and separately rejects a changed hash.
  The producer integration regression makes any same-key call to write_zip
  fail, proving generation never recompresses in that case. For a new key,
  write_zip explicitly sets `create_system = 3`, fixed external_attr and fixed
  timestamps; a simulated Windows-default ZipInfo test verifies the output.
  Native Windows and zlib-ng execution were not available; alternate-encoding
  and creator-default behavior were exercised by these controlled tests.
- **Minor 3 VERIFIED; correction to the preceding Edition.** The statement
  that prepared freshness used `pi_bundle.go:98-115` was incorrect: prepared
  package manifests declare `dist/index.js`, while piBundledEntries calls
  piBundleFresh only for `src/index.ts`. That branch and its synthetic test
  were unreachable from prepared launch and are removed as requested.
  **Prepared freshness rests on the reachable Python header/manifest check and
  cache_matches hashes**, not a Go prepared-record branch. The source-only
  `pi_bundle.go` is now byte-identical to develop `998cb3fe3`; its source
  freshness path is untouched. The prepared invalid-header-to-bridge regression
  remains, and the package-only test now asserts no TypeScript source is shipped
  instead of manually calling the source-only freshness function.
- **Minor 5 VERIFIED.** The legal matcher now preserves case-insensitive
  LICENSING prefixes, camel-case/separated ThirdPartyNotice families,
  COPYRIGHT/PATENTS prefixes and related legal filename families before other
  prune rules. New synthetic cases include legal files under docs and files
  whose extensions would otherwise trigger pruning. The real producer's entire
  closure files map remains equal to the pre-round map. Minor 4's literal-scan
  heuristic and Minor 6's cache GC were not changed.

**Verification.** All test processes used isolated HOME/USERPROFILE,
WS_CACHE_HOME/WS_CONFIG_HOME and npm cache under `/tmp/exec5/native-pi/round4/`.
The producer alone reused the existing isolated warmed cache, with `--offline`.

- `python3 -B -m unittest discover -s agents-plugin/tests`: **162 tests OK,
  2 existing Windows skips**; wsflow equivalent: **22 tests OK, 2 existing
  Windows skips**. The seven pruning/encoding/legal tests also passed alone.
  Existing unclosed-stream ResourceWarnings remain non-failing.
- From agents-plugin-tool, with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and
  existing offline Go caches: `go test -count=1 ./internal/wsagent -run 'Pi'`
  **ok, 39.672s**; `go test -count=1 ./internal/wsrsrc` **ok, 0.899s**;
  `go build ./...` and `go vet ./...` **exit 0**.
- `node --test agents-plugin-pi/test/web-package.test.ts
  agents-plugin-pi/test/helper-host.test.mjs agents-plugin-pi/test/workflow-owner.test.ts`,
  with role flags, WS_PI_TEST_CLI and WS_PI_TEST_NETWORK unset:
  **6 passed, 1 intentional network-install skip, 0 failed**. Default offline
  package-owned helper probes ran; no provider query or external network ran.
- Both actual Pi **1.5.2 CLI-adjacent SDK** identity/skill fixtures passed,
  one test per package and no skips, using canonical
  `/private/tmp/exec5/native-pi/round4/identity-<product>.XXXXXX` fixture roots
  and `native-cache`, with isolated HOME/WS cache/config. The entry SHA remains
  `af1e77fb4a83b50be87da5e518f1509c5b2787bcadd43a8fd9b375a245b732fd`.
  The standard prior identity loop and `pi-native-payload-identity.test.mjs`
  were used; MCP/socket/child-start boundaries stayed mocked.
- Both Node-less `python3 -I -B <pkg>/bin/ws-pi-payload.py --plugin <pkg>
  --check-source "$PWD"` gates passed. `diff -rq agents-plugin/native-pi
  agents-plugin-wsflow/native-pi` and cmp of both materializers passed.
- The single producer command was
  `python3 -B agents-plugin-pi/scripts/prepare-native-payload.py --offline --npm-cache /tmp/exec5/native-pi/current/producer/npm-cache --report /tmp/exec5/native-pi/round4/producer-report.json`.
  It exited 0. Only the producer source-input hash in each 11,990-byte
  first-party manifest changed: **2 files, 2 insertions, 2 deletions**.

**Required blob proof, both packages, before and after regeneration/commit:**

```text
git rev-parse HEAD:<pkg>/native-pi/closure.zip
  d10ad375fefcd209b493024bf6f44dfef8586bb3
git rev-parse HEAD:<pkg>/native-pi/closure-manifest.json
  032d5a1750fe8212bd9436a031993a40e562d10c
```

`git hash-object` of each worktree file matched the same pins. Both complete
files maps compare equal to the pre-round map. Closure sizes remain **7,284,712
bytes ZIP / 725,054 bytes manifest / 5,571 files / 21,547,906 unpacked bytes**.
The preceding size table therefore remains valid, with **zero large-blob growth**
in this round. Logs/receipts are in the round4 scratch directory. Commit message
spacing was checked using `git log -1 --format=%B`.

**Compatibility and boundaries:** T0. The old `998cb3fe3` reader at
`pi_lead.go:123-129` ignores native-pi and retains its sibling/bridge at `:41-58`;
new `pi_lead.go:125-137` retains those fallbacks. Producer publication reads the
committed closure and updates only package-local first-party metadata; neither
binary changes shared data, MCP/config/protocol semantics or version boundaries,
and reopen needs no migration. No history rewrite, push, install hook,
marketplace semantic change, version bump, T3 boundary, forbidden whiteboard
edit, live install or daemon operation occurred. Native Windows/zlib-ng,
PowerShell, the opt-in network-install lane and live rollout remain unrun.
The ticket remains ready for EXEC5 review and owner-gated rollout.

#### Edition (d81b2eae4) - 2026-11-16

**Forward:** Every release bump of `agents-plugin-pi/package.json` stales the
first-party fingerprint. Follow up by regenerating the native payload in the
release flow, or by excluding the version from the fingerprint.

## Owner Decisions (2026-11-16)

- (1183)=a: commit the trimmed native Pi payload into the public repo.
  The reviewed head is `5372d3151` (branch pushed at handoff): closure.zip of
  7.3 MB shared by ws and wsflow, a 0.7 MB manifest, and 1.7 MB of first-party
  files. It keeps (647)=a: marketplace installs work offline. To land it,
  rebase onto develop and run it through the merge flow.


## Resolution (2026-11-16)

Merged into develop at 6b6f7fc96 after rebasing onto v0.50.1. The producer now resolves npm portably (585d91ad3), and the first-party fingerprints were refreshed for 0.50.1 (d81b2eae4). TestPiPrepared passes on w5090. Follow-up: the fingerprint includes the package version, so the payload goes stale on every release bump. The fix belongs in the release flow (see Forward in 44a8688cd).
