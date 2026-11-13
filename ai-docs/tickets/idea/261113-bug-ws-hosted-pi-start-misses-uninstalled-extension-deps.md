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
