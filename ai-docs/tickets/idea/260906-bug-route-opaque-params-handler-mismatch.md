---
title: Opaque route params do not reach deterministic handlers
related:
  260904-refactor-enter-affordance-rename-route-opaque: introduced the opaque published schema
---

# Opaque route params do not reach deterministic handlers

## Background

Downstream reports repeated inability to call `route.resolve_proceed` through
the ws 0.45.0 advertised `session_key` plus opaque `params` schema. The patch
`ada0ba19` changed the published schema while intentionally leaving the parsers
unchanged. This capture records the observed regression, not an approved API
redesign.

## Evidence

Reproduced against the connected MCP server on 2026-09-06 with the review
session's `session_key` and this argument fragment:

```json
{"params":{"target":{"kind":"ticket-path","value":"probe"},"facts":{}}}
```

- `route.resolve_proceed` returns `route.resolve_proceed: target is required`.
  `parseProceedInput` reads top-level `args["target"]`; the handler does not
  unwrap `params` before parsing.
- `route.resolve_implement` returns `entered implement mode; todo list replaced
  (5 items)`. This is not a deterministic routing verdict: the typed path is
  selected only when top-level `target` exists, and wrapped input falls through
  to legacy mode entry. An agenda containing the original `params` therefore
  does not establish successful routing.
- The published schema and top-level parser inputs are tested separately;
  their integration through a wrapped `params` call is not covered by those
  assertions.

The downstream additionally reports that Claude Code sends undeclared
top-level objects as strings, producing `target must be an object`. That host
conversion was not independently reproduced here. The downstream's bypass to
`lead-implement` is an incident workaround, not a confirmed equivalent route.

## Phases

### Phase 1: Reconcile the route input contract

Investigate and resolve the mismatch across the advertised schema, skill Fact
Contracts, and both deterministic handlers. Preserve this reproduction when
evaluating the eventual fix. The transport shape, legacy compatibility policy,
and implementation approach remain to be decided before promotion; this idea
does not authorize changing routing policy or treating legacy mode entry as a
successful deterministic verdict.
