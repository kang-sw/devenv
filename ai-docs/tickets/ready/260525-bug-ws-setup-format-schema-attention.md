---
title: ws setup format schema attention
related:
  260524-mcp-actor-setup-bootstrap: setup bootstrap contract and actor recovery behavior
spec:
  - 260505-mcp-server-protocol-surface
  - 260524-mcp-actor-setup-bootstrap
related-mental-model:
  - mcp-runtime
---

# ws setup format schema attention

## Background

The public `ws.setup` MCP schema advertises a `format` argument. That argument
is useful for compatibility and tests, but normal lead workflow setup should
prefer the default readable output. Advertising `format` gives models
unnecessary attention on JSON output and encourages verbose setup calls such as
`format: "json"` when the caller only needs the recovery token and guidance.

## Decisions

- Hide `format` from the public `ws.setup` input schema.
- Continue accepting `format` in `ws.setup` dispatch as a hidden compatibility
  argument so existing tests, scripts, and structured callers keep working.
- Keep this change scoped to `ws.setup`. Do not hide broader `format` arguments
  on discovery or Git tools in this slice because those tools have stronger
  structured-consumer use cases.
- Preserve default readable `ws.setup` output.

## Phases

### Phase 1: Hide setup-only format affordance

Remove `format` from the advertised `ws.setup` MCP schema while preserving
hidden dispatch compatibility.

The completed behavior should:

- Omit `format` from `tools/list` / public schema for `ws.setup`.
- Still honor `ws.setup(arguments: {"format": "json"})` when a caller provides
  it explicitly.
- Leave `runtime.info`, Git, spec, ticket, mental-model, and other structured
  output surfaces unchanged.
- Keep setup alias behavior coherent when `WS_MCP_SETUP_TOOL` changes the
  advertised setup name.

Deferred scope:

- Do not redesign default setup output.
- Do not remove JSON support from setup dispatch.
- Do not change non-setup tool schemas.

Verification should cover public schema shape, hidden JSON dispatch, readable
default setup output, and setup alias schema behavior.
