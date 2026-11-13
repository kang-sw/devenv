---
title: "Hosted Pi start does not check the local extension's dependencies"
---

# Hosted Pi start does not check the local extension's dependencies

## Observed (2026-11-13)

A dashboard-hosted Pi session failed at startup in about 1s with: `node.exe exited: Failed to load extension D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts; Cannot find module 'yaml'; require stack src/tool-result-render.ts`. Neither the repo root nor agents-plugin-pi had node_modules, so all five runtime deps (yaml, ipaddr.js, jiti, linkedom, pi-web-access) were missing. Pi loads a local package in place and does not install its dependencies. The dashboard showed only the raw node error.

Estimate (not a decision): Phase 1 is small, Phase 2 is medium.

## Evidence

- `agents-plugin-tool/internal/wsagent/pi_lead.go:99-130` checks that the manifest and entry files exist, but not the dependencies.
- dashboard.log recorded the same yaml error.

## Recovery used

`npm --prefix <repo> install --omit=dev --ignore-scripts --legacy-peer-deps --package-lock=false` at the repo root. The root has no lockfile.

## Evidence (2026-11-13, confirmed)

- Owner repro: a dashboard-hosted Pi lead started with Plugins=Isolated exits at once with `node.exe exited: Error: Failed to load extension "D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts": Failed to load extension: Cannot find module 'yaml' Require stack: - D:\Workspace\Repos\devenv\agents-plugin-pi\src\tool-result-render.ts Hint: Start without extensions using "pi -ne".` A plain `pi` started from a terminal on the same machine works.
- Why only hosted: plain `pi` loads the devenv package that `pi install git:...` cloned and npm-installed under the Pi agent dir. Hosted Pi passes `--no-extensions`, then `--extension <checkout>/agents-plugin-pi/src/index.ts` from the source checkout beside the bundled plugin root (`pi_lead.go:26-55`, `piBundledEntries` `pi_lead.go:99-130`). That checkout was never npm-installed.
- Both modes load the bundled extension. Isolated passes only that extension. Default runs the `ws-lead.mjs` adapter, which also re-adds the user's non-ws Pi resources and additionally requires the Pi SDK beside the CLI (`pi_lead.go:57-76`, `pi_lead.go:136-167`).
- The MCP-bridge fallback (`pi_lead.go:28-47`) covers only path and manifest errors. Go checks that the entry file exists, not its npm dependencies, so a missing module is a hard exit of the Pi process, not a silent degrade. The death surfaces through the Fatal event built from the stderr tail (`stream.go:215-230`).

## Decisions (owner-confirmed)

1. (408)=c. A release ships the Pi extension (`agents-plugin-pi`) as one self-contained JS bundle with its npm runtime deps inlined (yaml, ipaddr.js, jiti, linkedom, pi-web-access), placed where hosted Pi finds it beside the bundled plugin. Hosted Pi loads the bundle in preference to `src/index.ts`. Option a (preflight + MCP-bridge fallback) is included as the safety net under c: before passing `--extension`, check that the entry can load (the bundle exists, or for a source checkout the deps resolve). On failure, fall back to the MCP bridge with a visible PluginWarning instead of letting Pi exit.
   - Rejected: b, where the launcher runs npm install into a cache at start. It needs npm and network at session start, and it is fragile on Windows.
2. Compatibility tier T0 (AGENTS.md Compatibility Tiers): no MCP, protocol or store change.
3. (410)=A developer machine with Node generates the stage-2 single JS bundle, which is committed to git rather than generated during the release build. The release build host w5090 has Go only, no Node/npm, and bundling needs the npm dependencies installed. The bundle carries a header with a hash of its inputs (extension sources plus lockfile); a Go test recomputes the hash and fails when it differs, so the staleness check also runs on the Node-less build host. Mark the bundle as generated (for example, `.gitattributes linguist-generated / -diff`) so review skips it. Rejected: generating during the release build, which needs Node or npm on the build host.
4. (411)=Use esbuild for the bundler: it handles TypeScript natively, is a single executable, and turns the TypeScript currently loaded through jiti into static imports in the bundle. Rejected: rollup/webpack, which need more plugins and configuration without a gain.

## Constraints

- Compatibility tier T0: no MCP, protocol or store change, and no version bump. Owner rule: avoid minor bumps (AGENTS.md Compatibility Tiers).
- Repo gotcha (note `gotcha.pi-tui-dual-package`): pi-tui must be resolved through the host Pi at runtime. The bundle must keep the host-provided Pi packages external, never inlined: `@earendil-works/pi-tui`, `@earendil-works/pi-coding-agent` and `@earendil-works/pi-ai` (the three names in `agents-plugin-pi/package.json` peer/dev dependencies; imported in `src/pi-tui.ts` and across `src/`).
- Plain `pi` with a `pi install git:` package must keep working unchanged.
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
| risk.fit | moderate | Phase 2 build placement text conflicts with Decision 3 (commit the bundle, no release-build step) |
| risk.test | moderate | build host has no Node, so bundle behavior is verified only by the committed hash test and a release-shaped live run |
| risk.security_or_contract | moderate | inlined third-party code becomes committed generated output; T0 tier recorded, but the bundle must not inline the host-provided Pi packages |

## Phases

### Phase 1: Preflight and MCP-bridge fallback

In `pi_lead.go` (`piBundledEntries` / `preparePiLeadLaunch`), before passing `--extension`, check that the entry can load (the bundle exists, or for a source checkout the deps resolve). On failure fall back to the MCP bridge with a visible PluginWarning.

Verification: a hosted Pi lead on a checkout whose `agents-plugin-pi` has no node_modules starts on the MCP bridge with a warning in both Isolated and Default, instead of exiting with `Cannot find module 'yaml'`.

### Phase 2: Single-JS bundle

Add the build step to the release asset build (`agents-plugin-tool` build-release-assets.sh or the matching script) and to the dev rebuild path (`agents-plugin-tool/scripts/build-release-assets.sh` builds only the six `ws-mcp-*` Go binaries and `SHA256SUMS`, no Pi package, and per Decision 3 the bundle is generated on a Node developer machine and committed, not built there; the exact placement is unsettled). Hosted Pi prefers the bundle over `src/index.ts`. Depends on (410) and (411).

Verification: the release assets contain the bundle (the committed bundle ships with the repo checkout beside the bundled plugin, not as a release asset; see the build-script note above); a hosted Pi lead started from release-shaped assets with no node_modules anywhere loads the full extension (not the bridge); the existing `agents-plugin-pi` tests pass.
