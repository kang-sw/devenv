---
title: "Hosted Pi start does not check the local extension's dependencies"
---

# Hosted Pi start does not check the local extension's dependencies

## Observed (2026-11-13)

A dashboard-hosted Pi session failed at startup in about 1s with: `node.exe exited: Failed to load extension D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts; Cannot find module 'yaml'; require stack src/tool-result-render.ts`. Neither the repo root nor agents-plugin-pi had node_modules, so all five runtime deps (yaml, ipaddr.js, jiti, linkedom, pi-web-access) were missing. Pi loads a local package in place and does not install its dependencies. The dashboard showed only the raw node error.

## Evidence

- `agents-plugin-tool/internal/wsagent/pi_lead.go:99-130` checks that the manifest and entry files exist, but not the dependencies.
- dashboard.log recorded the same yaml error.

## Recovery used

`npm --prefix <repo> install --omit=dev --ignore-scripts --legacy-peer-deps --package-lock=false` at the repo root. The root has no lockfile.

## Ideas (non-authoritative)

- Preflight-resolve the extension's direct deps in pi_lead.go before spawning, and fail with an actionable message that names the install command.
- Or document the install step in the Pi package setup.

## Evidence (2026-11-13, confirmed)

- Owner repro: a dashboard-hosted Pi lead started with Plugins=Isolated exits at once with `node.exe exited: Error: Failed to load extension "D:\Workspace\Repos\devenv\agents-plugin-pi\src\index.ts": Failed to load extension: Cannot find module 'yaml' Require stack: - D:\Workspace\Repos\devenv\agents-plugin-pi\src\tool-result-render.ts Hint: Start without extensions using "pi -ne".` A plain `pi` started from a terminal on the same machine works.
- Why only hosted: plain `pi` loads the devenv package that `pi install git:...` cloned and npm-installed under the Pi agent dir. Hosted Pi passes `--no-extensions`, then `--extension <checkout>/agents-plugin-pi/src/index.ts` from the source checkout beside the bundled plugin root (`pi_lead.go:26-55`, `piBundledEntries` `pi_lead.go:99-130`). That checkout was never npm-installed.
- Both modes load the bundled extension. Isolated passes only that extension. Default runs the `ws-lead.mjs` adapter, which also re-adds the user's non-ws Pi resources and additionally requires the Pi SDK beside the CLI (`pi_lead.go:57-76`, `pi_lead.go:136-167`).
- The MCP-bridge fallback (`pi_lead.go:28-47`) covers only path and manifest errors. Go checks that the entry file exists, not its npm dependencies, so a missing module is a hard exit of the Pi process, not a silent degrade. The death surfaces through the Fatal event built from the stderr tail (`stream.go:215-230`).
