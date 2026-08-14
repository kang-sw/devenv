---
title: note.search — optional / multi-layer `layer` argument
related:
  260810-note-tools: extends — relaxes the note.search layer argument this spec defines
  260814-feat-note-project-local-untracked-layer: motivates — with four layers, single-layer search forces N calls to reconstruct the ambient block's cross-layer view
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

## Proposed shape

- Make `layer` **optional** on `note.search`; omitted = search **all** layers.
- Accept either a single string or an **array** of layer names, so a caller can
  scope to a subset (e.g. `["clone", "repo"]` for project-scoped only) without
  four separate calls.
- Keep `write`/`erase`/`mute`/`unmute` single-layer and required — the asymmetry
  is intentional and mirrors read-vs-mutation semantics, not an inconsistency to
  paper over.

## Design considerations

- **Result disambiguation.** A cross-layer result set must tag each record with
  its originating layer, mirroring the ambient block's `[<layer>]` render.
  Single-layer search never needed this; multi-layer does. This is the main
  contract change, not the optionality itself.
- **Ordering.** Decide whether multi-layer results reuse Compute's ordering
  (priority desc → written_at desc → key asc) or stay grouped by layer. Reusing
  Compute's comparator keeps one mental model.
- **`format=json`.** The structured shape gains a `layer` field per record;
  confirm no existing caller assumes a layerless record.
- **muted/visible.** `search` returns muted notes today (that is its point);
  cross-layer search should preserve that — the `visible` filter stays a
  Compute-only concern, not a search filter.
- **Spec.** Update `mcp-tools.md` `#260810-note-tools` (argument contract) and,
  if the result shape gains a layer tag, note it there; `#260810-note-injection`
  is unaffected (injection already aggregates).

## Scope

Read-only ergonomics. No storage-layout change, no new layer, no mutation-path
change. Backward-compatible: a single-string `layer` keeps today's behavior.
