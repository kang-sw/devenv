---
title: ws setup actor token format
related:
  260524-mcp-actor-setup-bootstrap: setup bootstrap contract and actor recovery behavior
spec:
  - 260524-mcp-actor-setup-bootstrap
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# ws setup actor token format

## Background

`ws.setup(method: "lead-workflow-bootstrap", root:
"<absolute-working-directory>")` currently returns actor ids shaped like:

```text
lead-17da6bdc-242dfbeda3097087e810b193
```

That visible recovery token mixes authority, worktree routing, and random
uniqueness. The caller-facing contract only needs a short opaque actor id that
can recover an active cooperative workflow actor.

## Decisions

- Use a short authority-prefixed recovery token shape such as `lead-<8ch>`.
- The random payload should be lowercase and case-insensitive for practical
  entry: prefer `a-z0-9` for generated payloads, with no uppercase characters.
- Treat `-` as the authority separator. Avoid adding extra separators to the
  random payload unless the implementation has a concrete reason.
- Global uniqueness is not the goal. The collision boundary is active actors in
  the relevant runtime lookup scope, with collision retry on mint.
- Do not encode worktree routing detail in the visible token. Runtime state
  should resolve the actor token to its stored actor/worktree metadata.

## Phases

### Phase 1: Short setup actor recovery tokens

Implement short, lowercase setup actor ids for lead, delegate, and reader
actors while preserving existing recovery behavior.

The completed behavior should:

- Mint actor ids in a compact authority-prefixed form, for example
  `lead-k9f2p7qx`.
- Ensure generated payload characters avoid case-sensitive ambiguity.
- Preserve `ws.setup(id: "<actor-id>")` recovery for newly minted lead and child
  actors after MCP restart.
- Preserve actor-scoped named-agent and subquery dispatch for root-omitted
  calls.
- Keep old long actor ids recoverable if they already exist in runtime state,
  or document and test the compatibility boundary if legacy recovery is not
  supported.

Deferred scope:

- Do not change actor authority semantics.
- Do not change named-agent public names or instance-history behavior.
- Do not introduce a globally unique actor namespace unless recovery requires a
  minimal lookup index.

Verification should cover setup bootstrap, restart recovery, child actor prompt
injection/recovery, actor-scoped named-agent dispatch, and collision retry or an
equivalent deterministic collision test.
