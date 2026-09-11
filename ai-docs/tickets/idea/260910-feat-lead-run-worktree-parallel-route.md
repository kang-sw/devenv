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

A 2026-09-11 ad-hoc rehearsal exercised both halves against the live `ready/`
queue. The detection half works cleanly: a medium-tier Explore over Route Facts
scope paths produced a correct parallel-safe batch (two disjoint bug tickets
parallel, a third refactor sequenced behind one of them because both edit
`ticket-conventions.md`). The execution half hit a hard wall the harness
primitive does not cover: there is no way to bind a worker's ws session key to a
git worktree of this repo. `playbook.render(root_override=<worktree>)` treats
its argument as an rsrc manifest root, not a repo worktree, and fails with
`rsrc manifest missing at <worktree>/manifest.json`; a normal render binds the
worker key to the main root instead. So `isolation: "worktree"` isolates the
filesystem but leaves the worker's ws tools (tickets.move, git.commit) resolving
against the wrong root — the missing piece is a first-class render/spawn mode
that binds a worker key to a repo-worktree root.

The same rehearsal also surfaced a shared-worktree collision on the *default*
serial path: a lead housekeeping `git.commit` run while the worker was live
landed on the worker's impl branch, because the worker had switched the shared
working tree's branch out from under the lead. Isolation removes this class for
lead-side git operations too, but the interim guard for the un-isolated serial
flow is researched separately in
`260911-research-lead-commit-guard-during-worker-run`.

## Decisions

- **Serial stays the default; parallel is an explicit-approval route.** Reopening
  the deferred-parallelism decision is a canonical-flow change, so the parallel
  path fires only on explicit user approval per run, never as an inferred
  default. With no approval, `lead-run` behaves exactly as today. This per-run
approval gates worktree *provisioning* itself, not only the parallel decision:
worktree derivation can be very expensive in large repositories, so the final
skill creates no worktree without explicit user approval for that run.
- **Isolation is per-worker worktree, not shared-branch concurrency.** Each
  parallel worker runs in its own worktree+branch; the lead serializes the
  merges back into the goal branch in dependency order. This keeps the existing
  shared-branch rule intact rather than fighting it.
- **Worktrees are pooled and recycled behind a single user gate.** The only user
  approval is "may this run use a worktree at all"; with it granted, the lead
  follows three paths in order — reuse an idle detached worktree from the pool,
  else create a new one — and on completion *detaches* the worktree rather than
  deleting it, returning it to the reusable pool (worktree derivation is the
  expensive step this amortizes). All worktrees live under one consistent,
  locally git-ignored location (registered via `.git/info/exclude`, never a
  committed `.gitignore`, to avoid a shipped/downstream leak), so paths are
  predictable and never tracked.

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
guard; (d) a cap on concurrent workers; (e) how a worker key is bound to its
worktree root. Root cause confirmed in code: `playbook.render`'s `root_override`
is a single knob fused into three roles at once — the worktree root, the
rsrc-resolution root, and the child-key mint root (`server.go#L1377-L1408`;
`resolveRsrcRoot` returns the override verbatim rather than resolving the plugin
cache, `playbook_tools.go#L667-L671`). It was born as a test/advanced rsrc-root
override (read-variant schema: "test/advanced use") for rsrc-dev worktrees whose
own root holds `manifest.json`, then overloaded onto render, so it is valid only
when all three roots coincide. For a consuming-repo worktree the rsrc tree lives
in the plugin cache, so redirecting the rsrc root to the repo worktree fails with
"rsrc manifest missing". This is internal ws plumbing, not a shipped-surface
leak. The fix is to separate the worktree/mint root from the rsrc root so a
worker key can bind to a repo worktree while rsrc still resolves to the cache; (f)
reconciling `ticket-worker`'s own PARENT-branch capture and `impl/<parent>/<slug>`
creation with a pre-provisioned worktree — decide whether the route pre-creates
the impl branch and suppresses the worker's branch capture, or lets the worker
own branch creation inside the worktree; (g) pool reuse hygiene — before handing
a pooled worktree to a worker, reset it to the base branch and clean
untracked/ignored cruft so no prior run's state leaks into the next; (h) a claim
or lock so two concurrent runs never grab the same idle worktree, plus a pool
cap and GC policy; (i) where the pool directory lives — an ignored directory
inside the repo (nesting cost, must be excluded) versus a sibling directory
(no ignore concern but less "registered"), and how to name worktrees within it.
