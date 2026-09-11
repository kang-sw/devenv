---
title: "Research how to stop lead-side git mutations from colliding with a live worker in the shared worktree"
related:
  260910-feat-lead-run-worktree-parallel-route: sibling; per-worker worktree isolation removes this collision class entirely, but is user-gated and heavy — this ticket protects the default un-isolated serial flow in the interim
---

# Research a guard against lead git mutations during a live worker run

## Background

In the default serial `lead-run` flow the lead and its one worker share the main
worktree. `lead-run` already tells the lead "do not touch the ticket or the
branch meanwhile" while waiting for the worker, but prose alone did not prevent a
real incident: on 2026-09-11 a lead housekeeping `git.commit` run during a live
worker landed on the worker's impl branch, because the worker had checked out its
own `impl/<parent>/<slug>` branch in the shared working tree and the lead's
commit stacked onto whatever branch happened to be current. The change was
recovered (a fast-forward of the integration branch to reclaim the commit and
re-seat the worker's merge-base), but the failure mode is latent for any lead
that commits, moves tickets, or otherwise mutates git while a child worker
session is live.

`git.commit` is one of the highest-traffic ws MCP tools, so the collision surface
is broad, not incidental.

## Candidate approaches

- **Prose reinforcement (cheapest).** Strengthen the existing warning in
  `lead-run` (and possibly `lead-discuss`/`lead-delegate`) so it names the exact
  failure — a worker owns the shared working tree during its run; the lead must
  perform zero git mutations there until the worker reports — rather than the
  softer "do not touch the branch meanwhile." Low cost, but it is the mechanism
  that already failed once.
- **MCP-level guard (opinion-level, larger surface).** Because `git.commit` (and
  other worktree-mutating tools) already flow through ws MCP, a lead key could
  deterministically reject those calls while it has a live child worker session,
  keyed on `session.children` liveness. This makes the invariant enforced rather
  than advisory. Cost: it couples worker-allocation state to the git tool
  surface — a new cross-cutting linkage across several ws-MCP tools and the
  session model — so it is raised as an option to weigh, not a settled design.

## Open questions

- Which tools to guard: `git.commit` only, or every worktree-mutating tool
  (`tickets.move`, `git` mutations, ticket writes)?
- How the lead performs *legitimate* housekeeping during a long worker run —
  deferred until the worker reports, run in a separate worktree, or an explicit
  scoped override — without reopening the hole.
- False-positive risk: a recorded child session that is actually idle or dead
  should not permanently block the lead; liveness detection must be reliable.
- Relationship to `260910-feat-lead-run-worktree-parallel-route`: once worktree
  isolation lands and is active, this guard is redundant for isolated runs but
  still protects any un-isolated serial run. Decide whether the guard is
  permanent or a bridge.
