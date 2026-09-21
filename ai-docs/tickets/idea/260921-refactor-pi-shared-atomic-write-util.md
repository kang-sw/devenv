---
title: Extract a shared atomic file-write helper in agents-plugin-pi
---

# Extract a shared atomic file-write helper in agents-plugin-pi

## Background

`agents-plugin-pi/src/skills-dir.ts` (`writeAtomic`, ~lines 60-69) reimplements
the tmp-write-then-rename atomic-write pattern that already exists in
`agents-plugin-pi/src/fork-context.ts` (`writePrivateJson`, ~lines 137-143). The
two differ in detail (skills-dir uses a `wx` exclusive-create tmp; fork-context
does explicit `mkdirSync` + chmod), but the durable-write core is the same.

Surfaced as a non-blocking Fit finding (F1) during the 0.46.15 ship-gate review
sweep (`59140d29..10150d41`). Not a correctness issue — both call sites are
correct today; this is duplicate-abstraction cleanup.

## Phases

### Phase 1: Consolidate into one atomic-write util

Extract a single `writeAtomicFile` helper (module TBD — a small shared util both
`skills-dir.ts` and `fork-context.ts` can import without a layering cycle) and
route both call sites through it, preserving each site's current semantics
(exclusive-create vs. explicit mkdir+chmod as options, not silently changed).
Keep the change behavior-preserving; the existing skills-dir hash-sync tests and
fork-context tests must still pass.

Reject: forcing both sites onto one identical flag set if that changes the
on-disk permission/atomicity behavior either site relies on — parametrize
instead. Defer if the shared helper cannot be placed without introducing an
import cycle; capture the module-layout question rather than forcing it.
