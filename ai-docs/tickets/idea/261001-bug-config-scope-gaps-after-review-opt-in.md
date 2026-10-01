---
title: "Config scope gaps surfaced by the review opt-in work"
related:
  261001-feat-opt-in-design-and-phase-review: the worker reported these as minor unresolved findings
---

# Config scope gaps surfaced by the review opt-in work

## Background

The worker for `261001-feat-opt-in-design-and-phase-review` reported three
config-resolution gaps. Each one predates that ticket or follows from its
Decision 5, and none was fixed there. They matter more now that downstream
teams are expected to standardize review defaults through config.

1. **Sage resolvers ignore repo scope.** The Sage gate and the
   `tickets.create` / `tickets.move` posture resolvers do not read the
   committed repo-scope config (`.ws-workflow/config.json`). `config.list`
   and `review_phase` do read it. A committed `sage_review_design` therefore
   appears in `config.list` but has no effect on the gate. `sage_review` had
   the same gap before the split.
2. **Inherited session values look like the child's own.** Under the
   parent-walk, `config.list` on a child key reports an inherited value as
   scope `session`. A session-scope reset on the child is then a no-op, and
   nothing tells the user the value comes from the parent.
3. **CLI has no builtin defaults.** `cmd/ws-mcp` `ticketsMove` passes no
   builtin defaults. Completeness therefore falls back to `skipped` on the
   CLI path, while the MCP builtin is `auto`.

Gap 1 is the likeliest to bite: a team that commits a repo-scope review
default would see it listed but not applied.

Gap 1 moved to `261001-feat-config-repo-scope-and-tune-weight-guidance`
(its Decision 11); gaps 2 and 3 remain here.
