---
title: "Pinned-column deep link can leave its control under the sticky topic header"
related:
  261114-feat-ws-whiteboard-unified-whole-history: discovered during comparable baseline acceptance; not a TASK3 regression
---

# Pinned-column deep link can leave its control under the sticky topic header

## Background

At 1800×900, the real pin/unpin setup followed by navigation to p1 can leave
its summary center behind `.tophead .tbar`. A comparable isolated probe fails
the unchanged center-ownership assertion on both plain develop
`874280de2bf76c652b50e6cf924b01a401ef6857` and TASK3 `be0a950f5`.
The sticky stack changes after navigation while the target keeps its earlier
offset. This is pre-existing relative to TASK3, not a passing exposure check.

Evidence: `/tmp/task3-final-logic.nrv34u9q/{ab-plain874,ab-task3}/` and
`plain874-ab-provenance.json`. Both cases use the same test-only
`history-pinned-arrival` selector, real production renderers, and no asset
substitution. Plain source is unchanged except for an additive test fixture.
No screenshot proof is required; retained JSON records the actual center hit.

## Phases

### Phase 1: Keep deep-linked controls exposed through sticky-stack changes

Diagnose the navigation/keep/rail interaction and preserve actual control-center
ownership when the sticky header stack changes. Retain the strict assertion
and compare the same isolated setup on both production trees. Do not infer a
complete cause beyond the retained measurements or treat an unexecuted
precondition as a pass.
