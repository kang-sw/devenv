---
title: perf: mercenary hidden config read O(n) per toolAllowed call
sage-review: skipped
completed: 2026-06-29
---

# perf: mercenary hidden config read O(n) per toolAllowed call

## Background

`toolAllowed()` in `agents-plugin-tool/internal/mcp/server.go` calls
`mercenaryHiddenFromConfig()` (a file read) once per tool-name check.
`filteredTools()` in the same file already precomputes the same value once
before iterating the tool list.

No correctness risk — config does not change mid-call — but each `toolAllowed`
invocation performs redundant I/O that `filteredTools` already avoids.

`toolAllowed` has two call sites: one inside `filteredTools` (the list-tools
handler) and one inside the tool-dispatch handler for per-call gating. The
parameter approach is sufficient — the dispatch-handler call is a single check
and can pass the precomputed value.

## Direction

Selected: pass `mercenaryHidden bool` as a parameter to `toolAllowed`, removing
the internal `s.mercenaryHiddenFromConfig()` call. Update both call sites:
- `filteredTools()`: pass the already-precomputed `mercenaryHidden` (line ~3701)
- tool-dispatch handler: call `s.mercenaryHiddenFromConfig()` once at the call site
  (line ~351) — this remains a single check and is acceptable.

Rejected: request-scope caching — unnecessary since both call sites are
already single reads; the parameter approach is simpler.

## Phases

### Phase 1: Pass precomputed mercenaryHidden to toolAllowed

Add `mercenaryHidden bool` parameter to `(s *Server) toolAllowed`. Remove the
internal `s.mercenaryHiddenFromConfig()` call. Update both call sites.

Completion boundary: `filteredTools` no longer triggers a second file read per
tool; behavior unchanged.

## Spec Impact

Target spec area: `plugin-runtime.md` — MCP server internal.
Expected caller-visible change: none (internal performance fix, no observable behavior change).
Contract-first spec: no.


## Resolution (2026-06-29)

Phase 1 complete: toolAllowed now takes mercenaryHidden bool parameter; filteredTools passes precomputed value, dispatch handler passes s.mercenaryHiddenFromConfig() inline. Tests pass.
