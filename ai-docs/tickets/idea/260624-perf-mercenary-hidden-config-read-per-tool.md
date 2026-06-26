---
title: perf: mercenary hidden config read O(n) per toolAllowed call
---

# perf: mercenary hidden config read O(n) per toolAllowed call

## Background

`toolAllowed()` in `agents-plugin-tool/internal/mcp/server.go` calls
`mercenaryHiddenFromConfig()` (a file read) once per tool-name check.
`filteredTools()` in the same file already precomputes the same value once
before iterating the tool list.

No correctness risk — config does not change mid-call — but each `toolAllowed`
invocation performs redundant I/O that `filteredTools` already avoids.

## Direction

Two equivalent fixes:

- Pass the precomputed `mercenaryHidden bool` as a parameter to `toolAllowed`
  (simplest — no struct changes).
- Cache the value at request scope if `toolAllowed` is called in contexts other
  than `filteredTools`.

Check call sites of `toolAllowed` before deciding; if it is only ever called
from `filteredTools`, the parameter approach is strictly sufficient.
