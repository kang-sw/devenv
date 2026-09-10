---
title: "Add a user-gated worktree parallel route to lead-run: isolate parallel-safe tickets in per-worker worktrees, keep serial as default"
related:
  260909-epic-ws-worker-interpreter-refoundation: context; that epic deferred parallelism (one worker per invocation, serial) and this ticket proposes the gated path to reopen it
---

# Add a user-gated worktree parallel route to lead-run

## Background

`lead-run` spawns exactly one worker per invocation, serially
(`agents-plugin/rsrc/lead-run/lead-run.md:76`). The worker-interpreter
refoundation deferred parallelism deliberately, not incidentally
(`260909-...-drain-ready-queue-worker-spawner`): the earlier goal-fan-out
N-background-worker shape starved the `/goal` Stop hook, and a first-dogfood
incident where a worker's branch reset dropped a concurrent lead commit is what
added the shared-branch rule (never amend/reset/rebase the shared goal branch;
a correction is always a new commit) to `worker-stop-protocol`.

So the blocker to parallelism is not only file conflict. It is three things:
(1) file/edit conflict between concurrent workers, (2) a merge race when several
workers merge into the one shared goal branch, and (3) a stop/veto model shaped
around handling one worker report at a time. A git worktree per worker removes
(1) and (2) cleanly — each worker gets its own working directory and branch, so
nothing is shared until the lead merges — leaving only (3) and merge
serialization for the lead to own. The harness already supports worktree
isolation (`isolation: "worktree"` on spawned agents, `EnterWorktree`), so the
primitive exists.

## Decisions

- **Serial stays the default; parallel is an explicit-approval route.** Reopening
  the deferred-parallelism decision is a canonical-flow change, so the parallel
  path fires only on explicit user approval per run, never as an inferred
  default. With no approval, `lead-run` behaves exactly as today.
- **Isolation is per-worker worktree, not shared-branch concurrency.** Each
  parallel worker runs in its own worktree+branch; the lead serializes the
  merges back into the goal branch in dependency order. This keeps the existing
  shared-branch rule intact rather than fighting it.

## Constraints

- Convention: ai-docs/manuals/skill-authoring.md (lead-run is a shipped playbook).
- Convention: ai-docs/manuals/wsflow-mirroring.md (mirror to agents-plugin-wsflow/).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (the parallel-safety
  read must ride generic Route Facts + ticket listing, not any repo-only fact).

## Phases

### Phase 1: Gated worktree parallel route

Add to `lead-run`'s Select/Spawn path an opt-in parallel route that activates
only on explicit user approval for the run. When active: an Explore/`tickets.query`
pass identifies a parallel-safe batch from `ready/` — tickets whose Route Facts
file scopes are mutually disjoint and that carry no `related:`/`parent:`
ordering dependency among them — and the lead spawns one worker per ticket, each
in its own worktree (`isolation: "worktree"`), still one report handled per the
existing stop protocol but across N concurrent workers. The lead then merges the
finished branches into the goal branch serially, in dependency order, treating
any cross-worker file overlap that slipped the safety check as a merge stop.
Mirror to `agents-plugin-wsflow`.

Open points to resolve during execution: (a) exact parallel-safety predicate over
Route Facts scope paths (disjointness granularity — file vs directory); (b) how
the lead batches N concurrent stop reports without regressing the serial
veto/merge-approval model; (c) whether the `/goal` Stop-hook starvation that
killed the old fan-out is fully avoided by worktree isolation or needs its own
guard; (d) a cap on concurrent workers.
