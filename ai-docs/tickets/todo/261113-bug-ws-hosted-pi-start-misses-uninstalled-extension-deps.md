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

## Phases

### Phase 1: Preflight and MCP-bridge fallback

In `pi_lead.go` (`piBundledEntries` / `preparePiLeadLaunch`), before passing `--extension`, check that the entry can load (the bundle exists, or for a source checkout the deps resolve). On failure fall back to the MCP bridge with a visible PluginWarning.

Verification: a hosted Pi lead on a checkout whose `agents-plugin-pi` has no node_modules starts on the MCP bridge with a warning in both Isolated and Default, instead of exiting with `Cannot find module 'yaml'`.

### Phase 2: Single-JS bundle

Add the build step to the release asset build (`agents-plugin-tool` build-release-assets.sh or the matching script) and to the dev rebuild path. Hosted Pi prefers the bundle over `src/index.ts`. Depends on (410) and (411).

Verification: the release assets contain the bundle; a hosted Pi lead started from release-shaped assets with no node_modules anywhere loads the full extension (not the bridge); the existing `agents-plugin-pi` tests pass.
