---
title: "Hosted Pi start does not check the local extension's dependencies"
sage-review-design: completed
sage-review-completeness: completed
related:
  - 261113-feat-ws-hosted-lead-bundled-plugin-only
  - 260912-feat-ws-pi-bounded-web-access-for-explore
  - 260914-chore-ws-pi-root-manifest-runtime-deps
  - 261113-bug-ws-only-the-dashboard-daemon-runs-an-exe
  - 261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent
sage-review-design-reviewed: 9aa6c61a1121da9e
sage-review-completeness-reviewed: 9aa6c61a1121da9e
completed: 2026-11-13
---

# Hosted Pi start does not check the local extension's dependencies

## Background

### Observed (2026-11-13)

A dashboard-hosted Pi session failed at startup in about 1s with: `node.exe exited: Failed to load extension D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts; Cannot find module 'yaml'; require stack src/tool-result-render.ts`. Neither the repo root nor agents-plugin-pi had node_modules, so all five runtime deps (yaml, ipaddr.js, jiti, linkedom, pi-web-access) were missing. Pi loads a local package in place and does not install its dependencies. The dashboard showed only the raw node error.

Estimate (not a decision): Phase 1 is small, Phase 2 is medium.

### Evidence

- `agents-plugin-tool/internal/wsagent/pi_lead.go:99-130` checks that the manifest and entry files exist, but not the dependencies.
- dashboard.log recorded the same yaml error.

### Recovery used

`npm --prefix <repo> install --omit=dev --ignore-scripts --legacy-peer-deps --package-lock=false` at the repo root. The root has no lockfile.

### Evidence (2026-11-13, confirmed)

- Owner repro: a dashboard-hosted Pi lead started with Plugins=Isolated exits at once with `node.exe exited: Error: Failed to load extension "D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts": Failed to load extension: Cannot find module 'yaml' Require stack: - D:\Workspace\Repos\devenv\agents-plugin-pi\src\tool-result-render.ts Hint: Start without extensions using "pi -ne".` A plain `pi` started from a terminal on the same machine works.
- Why only hosted: plain `pi` loads the devenv package that `pi install git:...` cloned and npm-installed under the Pi agent dir. Hosted Pi passes `--no-extensions`, then `--extension <checkout>/agents-plugin-pi/src/index.ts` from the source checkout beside the bundled plugin root (`pi_lead.go:26-55`, `piBundledEntries` `pi_lead.go:99-130`). That checkout was never npm-installed.
- Both modes load the bundled extension. Isolated passes only that extension. Default runs the `ws-lead.mjs` adapter, which also re-adds the user's non-ws Pi resources and additionally requires the Pi SDK beside the CLI (`pi_lead.go:57-76`, `pi_lead.go:136-167`).
- The MCP-bridge fallback (`pi_lead.go:28-47`) covers only path and manifest errors. Go checks that the entry file exists, not its npm dependencies, so a missing module is a hard exit of the Pi process, not a silent degrade. The death surfaces through the Fatal event built from the stderr tail (`stream.go:215-230`).

## Decisions (owner-confirmed)

1. (408)=c. A release ships the Pi extension (`agents-plugin-pi`) as one self-contained JS bundle with its npm runtime deps inlined (yaml, ipaddr.js, jiti, linkedom, pi-web-access), placed where hosted Pi finds it beside the bundled plugin. Hosted Pi loads the bundle in preference to `src/index.ts`. Option a (preflight + MCP-bridge fallback) is included as the safety net under c: before passing `--extension`, check that the entry can load (the bundle exists, or for a source checkout the deps resolve). On failure, fall back to the MCP bridge with a visible PluginWarning instead of letting Pi exit.
   - Rejected: b, where the launcher runs npm install into a cache at start. It needs npm and network at session start, and it is fragile on Windows.
   - (479)=a (owner, 2026-11-13): `pi-web-access` stays external, not inlined: `web-search.ts` validates an exact on-disk `node_modules/pi-web-access@0.29.0` and spawns `src/web-search-helper.mjs` as a separate Node process that imports it (contract of `260912-feat-ws-pi-bounded-web-access-for-explore`). The bundle inlines the other runtime deps (yaml, ipaddr.js, jiti, linkedom) and is emitted one directory below the package root (for example `agents-plugin-pi/dist/`), so the `dirname(dirname(import.meta.url))` package-root lookups (`index.ts:263-271` pluginDir, `web-search.ts:71`, `web-tools.ts:30`) still resolve. Correction (design review 2026-11-13, autonomous): `word-chain-id.ts:18` reads `new URL("./eff_large_wordlist.txt", import.meta.url)`, a module-relative path, so from the bundle it targets `dist/eff_large_wordlist.txt`; the build script copies `src/eff_large_wordlist.txt` into `dist/` and the copy is part of the hashed inputs. Rejected: an esbuild text loader (changes the source read path the unbundled `src/` load still uses); a package-root lookup rewrite (touches runtime code for a build concern). Without `node_modules`, web search reports its existing missing-package diagnostic and everything else loads. Rejected: (b) committing pi-web-access (large third-party tree, conflicts with the exact-version on-disk check); (c) bundling the helper too (separate scope).
2. Compatibility tier T0 (AGENTS.md Compatibility Tiers): no MCP, protocol or store change.
3. (410)=A developer machine with Node generates the stage-2 single JS bundle, which is committed to git rather than generated during the release build. The release build host w5090 has Go only, no Node/npm, and bundling needs the npm dependencies installed. The bundle carries a header with a hash of its inputs (extension sources plus lockfile); a Go test recomputes the hash and fails when it differs, so the staleness check also runs on the Node-less build host. Mark the bundle as generated (for example, `.gitattributes linguist-generated / -diff`) so review skips it. Rejected: generating during the release build, which needs Node or npm on the build host.
4. (411)=Use esbuild for the bundler: it handles TypeScript natively, is a single executable, and turns the TypeScript currently loaded through jiti into static imports in the bundle. Rejected: rollup/webpack, which need more plugins and configuration without a gain.
5. **Bundle selection and freshness** (design + completeness review 2026-11-13, autonomous): `package.json` `pi.extensions` stays `./src/index.ts`, so plain `pi install git:` is unchanged; the bundle preference lives in `piBundledEntries` beside the manifest read (`pi_lead.go:99-130`). The input-hash routine is non-test Go code shared by the launcher and the staleness test. At launch the launcher recomputes the hash and uses `dist/` only when the header matches; a missing or stale bundle falls back to `src/index.ts`, which then goes through the Phase 1 preflight and, on failure, the MCP bridge. Input set: every file under `agents-plugin-pi/src/` that the bundle inlines or copies (`*.ts` and `eff_large_wordlist.txt`; `web-search-helper.mjs` is spawned, not bundled, and is excluded), `agents-plugin-pi/package-lock.json`, and the bundle script itself. `esbuild` is a pinned devDependency recorded in `agents-plugin-pi/package-lock.json`. Rejected: presence-only selection with staleness enforced by the Go test alone (a developer who edits `src/` without rebuilding would silently run old code).
6. **One bridge-fallback signal** (coherence review 2026-11-13, autonomous): every new fallback reason (missing deps, missing or stale bundle) goes through the existing `bundleErr` branch in `piLeadLaunch` (`pi_lead.go:27-50`), which sets `PluginWarning`, not a parallel branch. A hosted lead running the bundle is native; nothing may derive "native vs bridge" from the entry path (`src/index.ts` vs `dist/`), because `261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent` keys its render switch on the bridge signal.
7. **Preflight scope** (design review 2026-11-13, autonomous): "the deps resolve" means the statically imported load-time deps `yaml` (`delegation-policy.ts:5`, `tool-result-render.ts:2`), `linkedom` and `ipaddr.js` (`web-fetch.ts:11-12`) resolve through Node's ancestor `node_modules` lookup from the entry's directory (the git-install layout and the owner's recovery put them at the clone root, c3fd173f9). `pi-web-access` (optional, loaded lazily by the helper) and `jiti` (imported only by `web-search-helper.mjs`) are excluded. Rejected: checking only `agents-plugin-pi/node_modules`, or requiring `pi-web-access` (pushes working checkouts onto the bridge).

## Constraints

- Compatibility tier T0: no MCP, protocol or store change, and no version bump. Owner rule: avoid minor bumps (AGENTS.md Compatibility Tiers).
- Repo gotcha (note `gotcha.pi-tui-dual-package`): pi-tui must be resolved through the host Pi at runtime. The bundle must keep the host-provided Pi packages external, never inlined: `@earendil-works/pi-tui`, `@earendil-works/pi-coding-agent` and `@earendil-works/pi-ai` (the three names in `agents-plugin-pi/package.json` peer/dev dependencies; imported in `src/pi-tui.ts` and across `src/`).
- Plain `pi` with a `pi install git:` package must keep working unchanged.
- After Phase 2, every `agents-plugin-pi/src/` edit must regenerate the committed bundle on a machine with Node; the staleness test's failure message names the regeneration command. Ready ticket `261113-bug-ws-only-the-dashboard-daemon-runs-an-exe` Phase 2 edits `src/` files and runs its Go gates on the Node-less host w5090: whichever ticket lands second regenerates the bundle.
- Manuals: `ai-docs/manuals/release-compatibility.md` and `ai-docs/manuals/shipped-surface-boundary.md` (Implementation Conventions rows for `agents-plugin-tool/`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 7a2dd026 (2026-11-13, commit): "Owner picked c on the board; b rejected for start-time npm/network fragility on Windows. pi-tui stays external per the dual-package gotcha." — bearing: supports
- bfbac7b4 (2026-11-13, commit): "Owner acknowledged the lead's technical picks on the whiteboard 2026-11-13: commit the bundle with a Go hash-staleness test; esbuild." — bearing: supports
- 52dc582b (2026-11-13, commit): "Resolve the daemon package via its resource root (source checkout fallback for dev); missing sibling Pi entry uses the native MCP bridge with hosted identity context and a persisted warning." — bearing: constrains
- 261113-feat-ws-hosted-lead-bundled-plugin-only (2026-11-13, Decisions): "Pi isolated mode: --no-extensions --extension <bundled Pi extension entry>." — bearing: constrains
- 260914-chore-ws-pi-root-manifest-runtime-deps (2026-09-14, Decisions): "Single source of truth = agents-plugin-pi/package.json dependencies; the root block is a tooling-maintained mirror." — bearing: constrains
- 260912-feat-ws-pi-bounded-web-access-for-explore (2026-09-12, Decisions): "Bundle search. Pin and bundle pi-web-access@0.29.0 with agents-plugin-pi. Do not discover or depend on a separately configured user copy." — bearing: contradiction-candidate
- 260914-chore-ws-pi-join-release-train (2026-09-14, 6baafed9): "Owner-gate acceptance of v0.46.4 on a fresh Windows clone found the git-install consumption path fails at extension load (Cannot find module 'yaml')" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/pi_lead.go, agents-plugin-tool/scripts/build-release-assets.sh, agents-plugin-pi/package.json, new bundle plus build script, new Go staleness test |
| scope.surface | cross-module | Go launcher (wsagent), Pi package layout (agents-plugin-pi), build scripts; no exported symbol (piBundledEntries and piLeadLaunch are unexported) |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; T0, no MCP, protocol or store change |
| scope.test_surface | new-files | existing agents-plugin-tool/internal/wsagent/pi_lead_test.go covers piBundledEntries; the Go hash-staleness test is a new file |
| complexity.reuse_points | confirmed | piBundledEntries and the existing MCP-bridge fallback in piLeadLaunch (pi_lead.go#L27-L50) |
| complexity.side_effect_risk | moderate | changes which entry hosted Pi loads in both Isolated and Default mode; a wrong fallback silently degrades to the MCP bridge |
| risk.correctness | high | the extension resolves files by import.meta.url (src/web-search.ts#L71-L76 spawns src/web-search-helper.mjs from node_modules/pi-web-access; src/word-chain-id.ts reads a wordlist), which a single inlined bundle does not preserve; pi-tui must stay external |
| risk.fit | moderate | Phase 2 build placement now matches Decision 3 (committed bundle, release build unchanged); overlaps 261113-bug-ws-only-the-dashboard-daemon-runs-an-exe on src/ and pi_lead.go |
| risk.test | moderate | build host has no Node, so bundle behavior is verified only by the committed hash test and a release-shaped live run |
| risk.security_or_contract | moderate | inlined third-party code becomes committed generated output; T0 tier recorded, but the bundle must not inline the host-provided Pi packages |

## Phases

### Phase 1: Preflight and MCP-bridge fallback

In `pi_lead.go` (`piBundledEntries` / `preparePiLeadLaunch`), before passing `--extension`, check that the entry can load (the bundle exists, or for a source checkout the deps resolve). On failure fall back to the MCP bridge with a visible PluginWarning. Dependency set, resolution scope and signal path per Decisions 6 and 7.

Verification: a Go unit test over a temp fixture checkout (manifest and entry present, no `node_modules` on any ancestor) asserts the bridge launch with `PluginWarning` set, for both Isolated and Default; a second fixture with the deps at an ancestor `node_modules` asserts the native `--extension` launch. Live: a hosted Pi lead on a checkout whose `agents-plugin-pi` has no node_modules starts on the MCP bridge with a warning in both Isolated and Default, instead of exiting with `Cannot find module 'yaml'`.

### Result (e39095a38) - 2026-11-13

- `piBundledEntries` (`pi_lead.go`) now preflights a non-bundle entry: `piMissingLoadDeps` (`pi_bundle.go`) walks ancestor `node_modules` from the entry's directory for `yaml`, `linkedom`, `ipaddr.js` (Decision 7). A missing dep returns an error naming the deps (plus the bundle's missing/stale reason), which flows through the existing `bundleErr` branch to the MCP bridge with a `PluginWarning` (Decision 6).
- Tests: `TestPiLeadMissingDepsFallsBackToBridge` (Isolated and Default, warning names exactly the missing deps); the shared `piLeadBundle` fixture now installs the deps at an ancestor `node_modules`, so the existing native-launch tests cover the deps-present path.
- Live: `pi 1.0.4 --mode rpc --no-extensions -e <copy>/agents-plugin-pi/src/index.ts` on a node_modules-free copy reproduces `Cannot find module 'yaml'`; that is the case the preflight now routes to the bridge. A dashboard-hosted run was not performed in this worker.

### Phase 2: Single-JS bundle

Add an esbuild bundle script in `agents-plugin-pi` (an npm script or a script under `agents-plugin-pi/scripts/`) that a developer with Node runs; it writes the bundle under `agents-plugin-pi/dist/` with a header carrying the hash of its inputs (extension sources plus `package-lock.json`), keeps the host Pi packages and `pi-web-access` external (Constraints; Decision 1 (479)), and the bundle is committed and marked generated in `.gitattributes` (Decision 3). `build-release-assets.sh` is unchanged: the Node-less release host relies on the committed bundle, and a Go test recomputes the input hash and fails when the bundle is stale (technical, (482)-(488) acknowledged 2026-11-13). Hosted Pi prefers the bundle over `src/index.ts` when it exists and is fresh (Decision 5); Phase 1's preflight covers a missing or stale bundle. The build script copies `eff_large_wordlist.txt` into `dist/` (Decision 1). Add `dist/` to `package.json` `files`. Check that `index.ts` `extensionEntryPath` and the child spawns in `spawner.ts` carry the bundle path consistently when the bundle is loaded. Depends on (410) and (411).

Verification: the Go staleness test passes on the committed bundle and fails, naming the regeneration command, after an input source edit without a rebuild; a launcher unit test shows a stale header selects `src/index.ts`; with no `node_modules` anywhere, a hosted Pi lead loads the full extension from the bundle (not the bridge), a background task gets a word-chain ID (wordlist read from `dist/`), and web search reports its missing-package diagnostic; the release-shaped layout that `piBundledEntries` resolves (the installed plugin's sibling `agents-plugin-pi`) contains `dist/`; the existing `agents-plugin-pi` tests pass.

### Result (e39095a38) - 2026-11-13

- `agents-plugin-pi/scripts/bundle.mjs` (`npm run bundle`; `esbuild@0.28.2` pinned devDependency in the lockfile) writes `dist/index.js` (ESM, node22) with yaml/linkedom/ipaddr.js inlined, the three host Pi packages and `pi-web-access` external, a `createRequire` banner for inlined CommonJS, the first-line header `// ws-pi-bundle-inputs: sha256-<hex>`, and copies `dist/eff_large_wordlist.txt`. `dist/` is committed, in `package.json` `files`, and `.gitattributes` `linguist-generated -diff`. `pi.extensions` stays `./src/index.ts`; `build-release-assets.sh` unchanged.
- `piBundleInputHash` (non-test Go, `pi_bundle.go`) mirrors the script; `piBundledEntries` substitutes `dist/index.js` for `src/index.ts` only when the header matches, else falls back to the source entry and the Phase 1 preflight.
- Decision deviation (recorded): the hash input set is every `src/**/*.ts` and `src/**/*.mjs`, not excluding `web-search-helper.mjs`, because `web-search.ts` statically imports it and esbuild inlines it (Decision 5's own rule: files the bundle inlines).
- `extensionEntryPath`/children: from the bundle `import.meta.url` is `dist/index.js`, so `pluginDir`, `web-search.ts` package root and `spawner.ts` child `--extension` all carry the bundle path consistently.
- Verification: `go test ./internal/wsagent/` - all new tests pass (`TestPiBundleCommittedIsFresh`, `TestPiBundleInputHashCoversInputsOnly`, `TestPiLeadPrefersFreshBundle`); only the pre-existing `TestClaudeLeadLocalSettingsCanonicalWorktree` and `TestPiLeadResolverAdapter` fail, identically without this change. Appending a line to `src/summary-id.ts` makes the staleness test fail with "dist/index.js is stale ... cd agents-plugin-pi && npm install && npm run bundle". `npm test` in agents-plugin-pi: 2684 pass, 0 fail. Live: Pi 1.0.4 RPC on a node_modules-free copy loads `dist/index.js` (registers 70 ws tools and the extension commands); the bundle reads the wordlist via `new URL("./eff_large_wordlist.txt", import.meta.url)` from `dist/`. Not run: a background-task word-chain ID and a web-search diagnostic inside a live hosted lead (both need a model turn).
- Note: the raw-byte lockfile hash also stales the bundle when an `npm install` only rewrites the lockfile root `version`; regenerate in that case.
