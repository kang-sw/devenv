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
