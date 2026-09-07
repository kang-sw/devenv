---
title: project_tree renders the ticket section as a heavyweight annotated graph; make it a parent-nested status/stem tree
related:
  260903-epic-mcp-tool-surface-affordance-reduction: same theme — reduce the resident cost of the MCP tool surface
spec:
  - 260505-project-context-convention-tools
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 6159339477912463
sage-review-completeness-reviewed: 6159339477912463
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
    done/epic-dead-parent
      ready/feat-live-child
  todo/feat-standalone
  ```

  (Illustration only — rendered node lines carry no inline comments. Here
  `done/epic-dead-parent` appears solely as a dead-parent anchor for its live
  child `ready/feat-live-child`.)

- **Each stem rendered once. `related` edges and inline edge comments are
  removed from `project_tree` entirely.** The related graph is obtained on
  demand through `tickets_query` when a caller is actually navigating
  relationships; it does not belong in the always-resident orientation tree.

- **Render the entire ticket backlog; drop the orphan-`idea/` fold.** Every
  `idea/`/`todo/`/`ready/` ticket renders as its own `status/stem` node — nested
  under its parent when it has one, otherwise a root. The former hidden-count
  fold for orphan `idea/` tickets (those without `parent:`) is removed: with
  `related` edges gone, each ticket costs a single stem line, so rendering all of
  them is cheap and keeps the tree complete. Per-ticket growth is ~1 line and no
  ticket is hidden. A consequence is that every live (`idea`/`todo`/`ready`)
  parent is always materialized by the full-backlog render, so anchoring (below)
  is needed only for `done`/`dropped` ancestors.

- **Dead-parent anchoring.** A node whose status is `done`/`dropped` is rendered
  only if it has a **live descendant** (transitive). Two passes: (1) from each
  live ticket, mark the parent chain upward; (2) render only marked nodes among
  the `done`/`dropped` set (all live tickets always render per the decision
  above). Dead children do not count toward keeping a dead parent visible; a dead
  subtree with no live descendant is omitted entirely. This preserves the
  grouping anchor (`done/epic-...`) that a live child needs without pulling the
  whole `done` archive into the tree, and it generalizes the direct-parent case
  to multi-level dead ancestor chains for free.

- **Roots and edge cases.** Nodes with no parent are roots, ordered by stem;
  sibling children under a parent are likewise ordered by stem, for deterministic
  cache-stable output. A `parent` reference to a non-existent stem renders as a
  placeholder root labeled `?/<stem>` (unknown status); when several tickets point
  at the same missing stem they share a **single** `?/<stem>` placeholder and nest
  beneath it (not one placeholder each). Cycle guard: while marking the parent
  chain upward, stop on the first already-visited node — do not error or loop; the
  nodes in the cycle render as roots (break-and-root), so a malformed `parent:`
  cycle degrades to flat roots rather than hanging the call.

## Projected effect

8.6k → ~1.9k tok: the entire ticket backlog rendered parent-nested with no
hidden tickets (~78% reduction). The `related`-edge removal alone accounts for
the dominant saving; rendering every orphan `idea/` ticket as a single stem line
adds only ~1 line each. Dead anchors add only the handful of `done` epics that
bracket live work, so the ~1.9k projection holds. (The ~0.85k figure that a
fold-kept, currently-shown-only subset would cost is not the chosen design —
see the fold-drop decision above.)

## Constraints

- ws-mcp source change to the `project_tree` tool; harness-neutral, authored on
  `develop` through the normal flow.
- Preserve the file-tree portion unchanged.
- This ticket does not redesign `tickets_query`; it only names it as the
  on-demand home for the related graph. `tickets_query` separately dumps the
  full queue (~11.8k tok/call) — a sibling symptom, captured under Related, not
  fixed here.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` `{#260505-project-context-convention-tools}`
(the `project_tree` contract). The anchor's ticket-section prose must change on
three points. (1) The ticket section is no longer described by status-directory
inclusion with edges; it is a single parent-nested tree whose nodes are labeled
`status/stem`. (2) `related:` edges and their inline edge comments are no longer
rendered by `project_tree` — the related graph is reached on demand via
`tickets_query`. (3) The orphan-`idea/` hidden-count fold is **removed**: the
whole backlog renders (every ticket as one stem line), so the current prose
("folds remaining orphan `idea/` tickets ... into a single hidden-count line ...
reachable via `tickets.query`") is deleted rather than kept. `done`/`dropped`
nodes appear only as dead-parent anchors for a live descendant. No new anchor and
no `spec-remove:` — this revises the existing anchor's prose only.

## Phases

### Phase 1: Parent-nested `status/stem` ticket tree with dead-anchor pruning

Build the mark-and-render tree: full-backlog render (drop the orphan-`idea/`
hidden-count fold — every `idea`/`todo`/`ready` ticket emits a `status/stem`
node), live-descendant marking, dead-parent anchoring for `done`/`dropped`
ancestors, root ordering, placeholder (`?/<stem>`) and cycle handling; emit
`status/stem` node labels; drop `related` edges and inline edge comments from
`project_tree`. Tests: a live child under a `done` parent shows the `done`
parent as anchor; a `done` subtree with only `done` children is omitted;
multi-level dead-ancestor chains keep only the marked chain; all orphan `idea/`
tickets render (none hidden); placeholder and cycle cases; output contains no
`related` edges. Verification: measure the rendered size against the ~1.9k
projection on the current backlog — the projection is an order-of-magnitude
target, so treat within ~20% as pass; a result near the old ~8.6k means the
related-edge removal or fold-drop did not take effect.

## Related

- `tickets_query` full-queue dump (~11.8k tok/call) is the sibling symptom; the
  related graph moves there on demand rather than living in `project_tree`.
