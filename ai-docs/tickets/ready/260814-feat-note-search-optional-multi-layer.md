---
title: note.search — optional / multi-layer `layer` argument
related:
  260810-note-tools: extends — relaxes the note.search layer argument this spec defines
  260814-feat-note-project-local-untracked-layer: motivates — with four layers, single-layer search forces N calls to reconstruct the ambient block's cross-layer view
sage-review-design: completed
sage-review-completeness: completed
---

# note.search — optional / multi-layer `layer` argument

## Background

`note.search` currently **requires** a single `layer` (enum
`machine`/`worktree`/`clone`/`repo`). Every other `note.*` tool is single-layer
by nature — `write`/`erase`/`mute`/`unmute` mutate one substrate, so a single
`layer` is the correct shape. But `search` is a **read**, and the ambient
`# Notes` block (`wsnote.Compute`) already aggregates all four layers into one
view. `search`'s stated purpose is to retrieve notes elided from that block —
yet to do so the caller must already know which layer each elided note lived in,
and issue one call per layer. That is a caller-ergonomics gap surfaced while
dogfooding the four-layer surface: the read tool is strictly narrower than the
read view it exists to page.

## Decisions

- `layer` becomes **optional** on `note.search`; omitted = search **all** four
  layers (parallel to `Compute`'s aggregation).
- `layer` accepts **either a single string or an array** of layer names, so a
  caller can scope to a subset (e.g. `["clone", "repo"]`) in one call.
- `write`/`erase`/`mute`/`unmute` stay **single-layer and required** — the
  asymmetry is intentional and reflects read-vs-mutation semantics, not an
  inconsistency to remove. Record this so a future reader does not "fix" it by
  making the mutation tools multi-layer.
- Backward-compatible: a single-string `layer` keeps today's behavior and result
  shape for callers that pass it.

## Spec Impact

- Target: `mcp-tools.md`, `## Note Tools {#260810-note-tools}`.
- Caller-visible changes:
  - `note.search` `layer` moves from required-single to optional; the argument
    accepts a string or an array of the four layer names; omission means all
    layers.
  - **Result disambiguation:** a cross-layer (or array) result set tags each
    returned record with its originating layer, mirroring the ambient block's
    `[<layer>]` render. A single-string `layer` call MAY omit the tag to stay
    byte-compatible, or always include it — decide in Phase 1 and pin it in the
    spec. This tag is the substantive contract change, not the optionality.
  - Ordering: reuse `Compute`'s comparator (priority desc → written_at desc →
    key asc) for a multi-layer result, and pin in the spec whether the same
    comparator governs single-string/single-layer results too — otherwise
    `layer: "clone"` and `layer: ["clone"]` could return the same records in
    different orders (the current spec states no `note.search` ordering at all).
- `### Note Injection {#260810-note-injection}` is **unaffected** — the ambient
  block already aggregates and tags all layers; this ticket only brings
  `note.search` up to that view.
- `muted`/`visible`: `note.search` returns muted notes today (its purpose);
  cross-layer search preserves that. The `visible` filter stays a `Compute`-only
  concern and is never applied by `search`.

## Phases

### Phase 1: optional / multi-layer `note.search`

Relax `note.search`'s `layer` argument to accept a string, an array of layer
names, or omission (= all four layers), aggregating across the selected layers
through the same store-resolution path the other `note.*` tools use. Tag each
returned record with its originating layer for multi-layer/array results and
order the merged set by `Compute`'s comparator. Extend the `note.search` schema
description and the `mcp-tools.md` `#260810-note-tools` contract to match; leave
the four other `note.*` schemas untouched.

Distinct sub-step (separately verifiable): decide and pin, in the spec, (a)
whether a single-string `layer` call keeps the untagged legacy result or also
gains the layer tag, and (b) whether `Compute`'s comparator governs
single-string/single-layer results too, so `layer: "clone"` and
`layer: ["clone"]` cannot diverge in ordering.

Verify:
- Omitted `layer` returns notes from all four layers in one call, each tagged
  with its layer, ordered by priority then written_at then key.
- An array `layer` (e.g. `["clone", "repo"]`) returns exactly those layers'
  matching notes and no others.
- A single-string `layer` call returns the same notes as today (regression), per
  the pinned tag decision.
- Muted notes still surface in `note.search` results across the multi-layer
  path.
- `layer: "clone"` and `layer: ["clone"]` return the same records in the same
  order (single-layer/array-of-one ordering parity), per the pinned decision.
- `write`/`erase`/`mute`/`unmute` schemas are unchanged (still required-single
  `layer`).
