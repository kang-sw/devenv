---
title: Parallel todo status updates can lose a concurrent write
---

# Parallel todo status updates can lose a concurrent write

## Background

Live ws dogfood sent two independent `todo.check(..., status: "done")` calls in one parallel tool batch for different todo keys. Each call returned success, but the second rendered checkpoint showed the first key back in `pending`. Repeating the first update sequentially restored the intended state.

This is consistent with non-atomic read-modify-write handling or last-writer-wins replacement of the session todo list. Parallel updates to distinct keys should not overwrite each other.

## Phases

### Phase 1: Make distinct concurrent todo mutations composable

Reproduce concurrent status changes against two distinct keys and correct the session-state mutation boundary so both successful writes remain visible. Preserve ordered todo rendering and existing single-call behavior. Verify concurrent `todo.check` calls as well as mixed non-conflicting add, check, erase, and reorder operations; explicitly define conflict behavior when two calls mutate the same key or overlapping order spans.
