---
summary: Maintain the dashboard settings catalog, adapter declarations, and translations together so every supported setting remains inspectable.
---

# Dashboard Settings Catalog

Update the settings catalog when a key changes in the MCP registry or an
adapter manifest. Put the row in the section a user would look for it, choose
a control that matches its value and selectors, and add its label and
description to all dashboard locale tables. The drift test compares the
explicit catalog against the registry and all four embedded manifests in
both directions.

Use the daemon's packaged manifest and its selected ws or wsflow defaults
when inspecting an adapter. Run the package-assets generator after changing
a canonical manifest. Its check and package tests verify that a standalone
daemon has the same declarations without an installed adapter.

The page projects per-layer availability and scope restrictions from the
snapshot. The control needs both the exact selected override and the
effective value because a higher layer or harness bucket can win. Keep
labels and help text in the dashboard locale tables so MCP listing responses
retain their existing contract.

A repo edit becomes a draft proposal for the selected worktree's lead,
or a patch the user can copy. Profiles use the server-redacted snapshot for
the card and raw JSON. Prompt diffs compare the packaged seed with the
editor in memory. The planned Git Sync Layer tracks
`261119-feat-ws-config-git-sync`.

From the repository root, check generated assets and the registry and
translation references.

```sh
python3 agents-plugin-tool/scripts/generate-package-assets.py --check
go -C agents-plugin-tool test ./internal/mcp ./internal/wsi18n ./internal/wspackage -run 'Settings|Manifest'
```
