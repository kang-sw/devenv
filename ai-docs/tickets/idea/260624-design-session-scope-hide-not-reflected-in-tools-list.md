---
title: design gap: session-scope prefer_mercenary=hide not reflected in tools list
---

# design gap: session-scope prefer_mercenary=hide not reflected in tools list

## Background

`mercenaryHiddenFromConfig()` in `agents-plugin-tool/internal/mcp/server.go`
reads project and global config scopes only (empty session key). A lead who
sets `prefer_mercenary=hide` at session scope sees mercenary-on playbook blocks
suppressed correctly (the render path reads session scope), but
`ws.mercenary.*` tools remain in the tools list until the same value is set at
project or global scope.

The current server.go comment acknowledges this limitation. No regression: the
session is ephemeral, and `ws.lead.prefer_mercenary` stays visible regardless
(it has a `ws.lead.` prefix), so a lead can always change the setting.

## Direction

Two options:

1. **Document only** — clarify in `ws.lead.prefer_mercenary` tool description
   that tool-list suppression requires project/global scope.
2. **Extend `mercenaryHiddenFromConfig`** — accept a session key so it can
   check session scope in addition to project/global. Requires the session key
   to be threaded into the call site, which currently has none.

Option 1 is sufficient for now. Option 2 matters if session-only `hide` becomes
a common pattern.
