---
title: project_tree renders the ticket section as a heavyweight annotated graph; make it a parent-nested status/stem tree
related:
  260903-epic-mcp-tool-surface-affordance-reduction: same theme — reduce the resident cost of the MCP tool surface
  260903-research-ws-pi-adapter-npm-distribution: discovery context was Pi-track dogfooding where cache-read dominated the bill
---

# project_tree renders the ticket section as a heavyweight annotated graph; make it a parent-nested `status/stem` tree

## Background

`project_tree`'s output is two parts: a small file tree and a large ticket
section. The ticket section is not a lightweight tree — it dumps a full
annotated ticket graph (every ticket's `parent:` and `related:` edges, each
edge carrying a freeform inline `# comment`). Measured on a real session
(surfaced during Pi-track dogfooding, 2026-09-07; the weight itself is
harness-neutral — any host pays it):

- One call = 34,415 chars / ~8,603 tokens / 245 lines.
- File tree: 53 lines, ~360 tokens (4%).
- Ticket graph: 192 lines, ~8,243 tokens (96%). Of that:
  - stem lines: 59, ~848 tok
  - `parent:` lines: 14, ~427 tok
  - `related:` lines: 118, **~6,952 tok = 81% of the whole output**, of which
    the inline `# comment` tails alone are ~5,319 tok (**62% of the whole
    output**).
- On top of this, 74 idea tickets were still elided ("orphan hidden"): the
  format already truncates and is still 8.6k. Backlog growth makes it worse
  roughly linearly.

Cost impact: the call fires early in a session (observed at call 2) and stays
resident, so it is re-billed as a cache read on every later turn (~48 in the
measured session) — the single largest `size × turns-persisted` contributor to
that session's cache-read cost. `related` edges and their inline comments carry
essentially all of the weight; the `parent` edges and stems are cheap.

## Decisions (design)

- **Drop status-directory grouping in the ticket section.** Render tickets as a
  single parent-nested tree — `parent` is unique per ticket, so it nests like a
  filesystem — with each node labeled `status/stem` as a path-like prefix,
  homologous to the file-tree section above it. Example:

  ```
  idea/epic-blah
    ready/feat-blah
    todo/research-blah
    done/epic-dead-parent      # kept only because a live child hangs off it
      ready/feat-live-child
  todo/feat-standalone
  ```

- **Each stem rendered once. `related` edges and inline edge comments are
  removed from `project_tree` entirely.** The related graph is obtained on
  demand through `tickets_query` when a caller is actually navigating
  relationships; it does not belong in the always-resident orientation tree.

- **Dead-parent anchoring.** A node whose status is `done`/`dropped` is rendered
  only if it has a **live descendant** (transitive). Two passes: (1) from each
  live ticket, mark the parent chain upward; (2) render only marked nodes. Dead
  children do not count toward keeping a dead parent visible; a dead subtree
  with no live descendant is omitted entirely. This preserves the grouping
  anchor (`done/epic-...`) that a live child needs without pulling the whole
  `done` archive into the tree, and it generalizes the direct-parent case to
  multi-level dead ancestor chains for free.

- **Roots and edge cases.** Nodes with no parent are roots, ordered by stem. A
  `parent` reference to a non-existent stem renders as a placeholder root; guard
  against cycles.

## Projected effect

8.6k → ~0.85k tok (currently-shown set) / ~1.9k tok (entire backlog with no
hidden tickets). ~78–90% reduction. Dead anchors add only the handful of `done`
epics that bracket live work, so the ~1.9k projection holds.

## Constraints

- ws-mcp source change to the `project_tree` tool; harness-neutral, authored on
  `develop` through the normal flow.
- Preserve the file-tree portion unchanged.
- This ticket does not redesign `tickets_query`; it only names it as the
  on-demand home for the related graph. `tickets_query` separately dumps the
  full queue (~11.8k tok/call) — a sibling symptom, captured under Related, not
  fixed here.

## Phases

### Phase 1: Parent-nested `status/stem` ticket tree with dead-anchor pruning

Build the mark-and-render tree: live-descendant marking, dead-parent anchoring,
orphan/placeholder/cycle handling; emit `status/stem` node labels; drop
`related` edges and inline edge comments from `project_tree`. Tests: a live
child under a `done` parent shows the `done` parent as anchor; a `done` subtree
with only `done` children is omitted; multi-level dead-ancestor chains keep only
the marked chain; orphan/placeholder/cycle cases; output contains no `related`
edges. Verification: measure the rendered size against the ~1.9k projection on
the current backlog.

## Related

- `tickets_query` full-queue dump (~11.8k tok/call) is the sibling symptom; the
  related graph moves there on demand rather than living in `project_tree`.
