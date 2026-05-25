---
title: ws setup actor token and schema attention
related:
  260524-mcp-actor-setup-bootstrap: setup bootstrap contract and actor recovery behavior
---

# ws setup actor token and schema attention

## Background

During a `ws:lead-discuss` dogfood run, `ws.setup(method:
"lead-workflow-bootstrap", root: "<absolute-working-directory>")` returned an
actor id shaped like:

```text
lead-17da6bdc-242dfbeda3097087e810b193
```

The token is longer than needed for the human-visible recovery handle. The
current shape appears to encode authority, worktree routing, and random
uniqueness in one caller-facing value, but the public contract only needs an
opaque actor id that can recover an active cooperative workflow actor.

The same discussion also found that the public `ws.setup` schema advertises a
`format` argument even though default readable output is enough for normal lead
workflow use. Advertising `format` gives models unnecessary attention on JSON
output and encourages more verbose setup calls.

## Questions

- Should `ws.setup` actor ids become short actor-scope tokens, such as
  `lead-<8ch>`, with worktree lookup handled by runtime state instead of the
  visible token shape?
- Should short actor token payloads use only case-insensitive, low-confusion
  characters from `a-z0-9-_`, avoiding uppercase recovery tokens because agents
  and humans may need to remember and re-enter them?
- What collision budget is appropriate for active actors in one worktree, given
  that global uniqueness is not the goal?
- Should `format` remain accepted by `ws.setup` dispatch but be hidden from the
  public MCP input schema, matching the existing hidden compatibility-argument
  pattern used by some agent tools?
- Should broader `format` schema exposure on discovery and Git tools remain
  public because those tools have stronger structured-consumer use cases?
