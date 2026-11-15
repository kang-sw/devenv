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

### Result (c9b8171f6) - 2026-11-16

Implemented on `exec5/native-pi-payload`, based on `28cf9c7bc`; product commit
`c9b8171f65ddcd54d81b2b82d249f175d65af7cf`. The generated bundle and both
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

## Owner Decisions (2026-11-16)

- (1183)=a: commit the trimmed native Pi payload into the public repo.
  The reviewed head is `5372d3151` (branch pushed at handoff): closure.zip of
  7.3 MB shared by ws and wsflow, a 0.7 MB manifest, and 1.7 MB of first-party
  files. It keeps (647)=a: marketplace installs work offline. To land it,
  rebase onto develop and run it through the merge flow.
