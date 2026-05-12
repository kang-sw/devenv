---
title: API docs backend selection and alias resolution
spec:
  - 260512-api-doc-agent-backend-selection
  - 260512-backend-model-resolution-consistency
related-mental-model:
  - api-documentation-cache
  - named-agent-runtime
  - prompt-bundle
---

# API docs backend selection and alias resolution

## Background

`api.ask` currently registers both the pre-router and per-domain managers with
`backend: codex` while also relying on portable `light`/`core` aliases. In the
presence of user alias overrides, that can produce impossible backend/model
pairs such as `backend: codex` with `model: claude-sonnet-4-6`, which fails at
backend invocation time even though the caller only requested an API-doc query.

The API-doc surface does not need Codex pinning. It should follow the same
harness-aware alias resolution used by named agents so host sessions and user
overrides can select the effective backend intentionally. At the same time, the
named-agent resolver must stop borrowing concrete models from a different
backend's alias mapping when the caller already fixed the backend explicitly.

## Decisions

- Keep the public `api.ask` and `api.ask_async` interface unchanged; fix the
  internal registration behavior instead of adding new API-doc-specific backend
  knobs.
- Preserve portable alias usage for API-doc helpers: `light` for the pre-router
  and `core` for per-domain managers.
- Prefer leaving a concrete model empty over constructing a cross-backend
  backend/model mismatch when no alias mapping exists for an explicit backend.

## Phases

### Phase 1: Re-route API-doc helpers and tighten alias resolution

Update API-doc helper registration to use harness-aware alias resolution
instead of hardcoded Codex backend pinning. Adjust wsconfig alias persistence
and resolution so explicit backend selection cannot inherit a concrete model
from another backend mapping. Cover both the routing change and the mismatch
prevention with focused tests.
