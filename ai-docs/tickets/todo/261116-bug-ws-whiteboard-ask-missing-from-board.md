---
title: "Whiteboard ask 1205 was absent from the board page despite existing in the store"
---

# Whiteboard ask 1205 was absent from the board page despite existing in the store

## Background

Dogfood #15, 2026-11-16: the owner could not see ask item **1205** on the
whiteboard page although the store held it. This is lead/owner-reported evidence;
this capture did not inspect the live store or browser. The affected session,
topic, page URL, filter/fold state and exact source/served revisions still need
to be recovered. The cause is unknown: do not label it a stale stream, filter,
rendering or storage defect without evidence.

## Decisions

Compatibility: **n/a** for a visibility/rendering correction preserving existing
item/store and wire shapes; reassess if diagnosis requires a cross-version change.

## Phases

### Phase 1: Trace ask 1205 from stored item to owner-visible board

Recover the incident context and exact stored item, compare the board query/view,
rendered response and visible browser DOM, and reproduce why this ask was absent.
Check topic/session routing, filters/folds, initial load and incremental delivery
without assuming which layer failed. Fix only the demonstrated cause; retain
legitimate visibility rules and the ask's identity/history. Verify that the
same eligible stored ask appears on initial load and after live updates, with
controls for intentionally filtered/closed items. Which layer omitted item
1205, and under what page state? No implementation or live probe is authorized
by this ticket capture.
