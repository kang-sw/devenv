---
title: "Add a user-gated worktree parallel route to lead-run: isolate parallel-safe tickets in per-worker worktrees, keep serial as default"
related:
  260909-epic-ws-worker-interpreter-refoundation: context; that epic deferred parallelism (one worker per invocation, serial) and this ticket proposes the gated path to reopen it
  260731-bug-playbook-render-root-override-manifest-resolution: prerequisite (done); landed the root_override/rsrc split Phase 1 builds on
  260911-feat-ws-git-merge-lead-owned-merge-authority: prerequisite (done); Phase 2 routes the serial merges through this lead-owned ws/git.merge
  260911-feat-impl-derivation-hardening-branch-aware-select: prerequisite (done); Phase 2 reuses its branch-aware Select for batch selection
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dcf580432d7200b1
sage-review-completeness-reviewed: dcf580432d7200b1
---

# Add a user-gated worktree parallel route to lead-run

## Background

`lead-run` spawns exactly one worker per invocation, serially
(`agents-plugin/rsrc/lead-run/lead-run.md:88`, "One worker in flight per
invocation."). The worker-interpreter
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

This root_override/rsrc-manifest conflict was fixed 2026-09-12 by
`260731-bug-playbook-render-root-override-manifest-resolution` (commit
`63542b5c`): `playbook.render` now resolves the rsrc manifest independently of
`root_override` (`resolveRsrcRoot("")` at
`agents-plugin-tool/internal/mcp/server.go#L1458`) while `root_override` still
binds the worktree root and the render-minted child key
(`agents-plugin-tool/internal/mcp/server.go#L1439-L1489`), so a lead can already
bind a worker key to an arbitrary worktree root through `playbook.render`. The
remaining gap this ticket must still close is only the worktree
creation/reuse/pool lifecycle itself — no `git worktree add` or pooling helper
exists anywhere in `agents-plugin-tool/internal` (verified by search).

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
- **Worktrees are pooled and recycled, provisioned by a deterministic MCP tool.**
  With the single user gate granted ("may this run use a worktree at all"), the
  lifecycle — reuse an eligible idle worktree from the owned pool, else create
  one, and on completion *detach* (never delete) it back into the pool so the
  expensive derivation is amortized — is owned by a
  `worktree.acquire`/`worktree.release` MCP pair, not lead-inline git plumbing
  and not an LLM delegate: it is deterministic git work, so it belongs in the
  tool layer (zero worker/lead token cost, no prompt drift, testable), and
  `acquire` mints the worktree-bound worker key in the same call. Pool location
  is a `worktree_pool` config knob (default `$(GitRoot)/.ws-worktrees`, where
  `$(GitRoot)` is the primary/main worktree root; the knob also accepts an
  absolute path), self-registered in `.git/info/exclude` (local only, never a
  committed `.gitignore`); each worktree is a 3-word-stem directory
  (ferrule-style) so a pool shared by several projects at one absolute path
  never collides. Placing worktrees under `.git/` is rejected as an anti-pattern.
  See Phase 1 for the full contract.

## Constraints

- Convention: ai-docs/manuals/skill-authoring.md (lead-run is a shipped playbook).
- Convention: ai-docs/manuals/wsflow-mirroring.md (mirror to agents-plugin-wsflow/).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (the parallel-safety
  read must ride generic Route Facts + ticket listing, not any repo-only fact).
- Convention: ai-docs/manuals/ws-mcp.md (the `root_override` split and the
  `worktree.acquire`/`worktree.release` tools are ws-MCP changes under
  `agents-plugin-tool/internal/mcp/`).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/playbook_tools.go, agents-plugin-wsflow/ mirror, ai-docs/manuals/ws-mcp.md |
| scope.surface | public-interface | new MCP tools worktree.acquire and worktree.release, plus the shipped lead-run.md playbook route |
| scope.new_public_symbol | yes | worktree.acquire, worktree.release |
| scope.new_type_contract | yes | acquire's { path, worker_key } return shape and the worktree_pool config knob |
| scope.test_surface | new-files | no worktree.acquire/release or worktree_pool test file exists yet (search over agents-plugin-tool/internal returned nothing) |
| complexity.reuse_points | confirmed | renderPlaybook's existing mintRoot/child-key binding (agents-plugin-tool/internal/mcp/playbook_tools.go#L862) already binds a child key to an arbitrary root, reusable by acquire's key-mint step |
| complexity.side_effect_risk | high | acquire/release perform live git worktree add/checkout/reset/clean and write to .git/info/exclude |
| risk.correctness | high | reuse-eligibility and hygiene-reset logic can destroy uncommitted work or bind a worker key to the wrong root if implemented incorrectly |
| risk.fit | moderate | must align with existing wsconfig knob patterns and the ws-mcp.md/skill-authoring.md/wsflow-mirroring.md conventions this ticket already declares |
| risk.test | moderate | new git-worktree-mutating test surface with no existing fixture to extend (playbook_render_surface_test.go covers only the root split, not worktree lifecycle) |
| risk.security_or_contract | high | introduces two new public MCP tools plus a session-key-minting contract tied to a filesystem root |

## Phases

### Phase 1: Worktree provisioning primitive (MCP)

Prerequisite already landed: the `playbook.render` `root_override` split that
lets a worker key bind to a repo worktree while rsrc still resolves to the
plugin cache shipped 2026-09-12 via
`260731-bug-playbook-render-root-override-manifest-resolution` (commit
`63542b5c`; `resolveRsrcRoot` at `playbook_tools.go`, override wiring at
`server.go`). So this phase's only remaining work is the deterministic
`worktree.acquire`/`worktree.release` tool pair below — no `git worktree add` or
pooling helper exists yet anywhere in `agents-plugin-tool/internal` (confirmed
by search).

Add a deterministic MCP tool pair that owns the whole worktree lifecycle so
neither the lead nor the worker carries git plumbing:

- `worktree.acquire(base, target_branch, session_key)` — resolve the pool root
  (below), reuse an eligible idle worktree or create a new one, create
  `target_branch` if it does not exist and check it out on `base` (this is what
  pre-creates Phase 2's `impl/<parent>/<slug>`), sync submodules, mint a child
  worker key already
  bound to that worktree root, and return `{ path, worker_key }`. Binding the
  key here is what closes the key-to-worktree gap — there is no separate render
  step.
- `worktree.release(path|key)` — clean the worktree and `git checkout --detach`
  it back into the pool (never delete; derivation is the expensive step the pool
  amortizes).

Pool-root resolution: a single `worktree_pool` config knob, default
`$(GitRoot)/.ws-worktrees`, accepting an absolute path or a `$(GitRoot)`
template. `$(GitRoot)` binds to the primary (main) worktree root — the common
git root — so `acquire` called from inside any linked worktree resolves to the
one root pool, never a per-worktree nested pool. The tool self-registers the
resolved in-repo path in `.git/info/exclude` (local only, never a committed
`.gitignore`, to avoid a shipped/downstream leak). Each acquired worktree is a
3-word-stem directory (ferrule-style) under the pool, so a pool shared by many
projects at one absolute path never collides; the path name carries no meaning,
the branch carries context. Placing worktrees under `.git/` is rejected as an
anti-pattern — `.git/` is git's private administrative area, disposable by
contract; scan invisibility is solved at the ignore layer, not by hiding user
data inside `.git/`.

Reuse eligibility (conservative golden rule): a pooled worktree is reusable only
when it is under the owned pool prefix, at detached `HEAD`, with no uncommitted
changes; a branch-checked-out or dirty worktree is skipped so no in-progress
work is stomped. After claiming, `acquire` still hygiene-resets to `base` and
cleans untracked/ignored cruft so no prior run's state leaks forward. Foreign
worktrees (outside the pool) are never adopted — the earlier "ask the user to
adopt an idle worktree" idea is dropped as unclear-benefit intrusion on lead
judgment.

Verification: Go table tests for `worktree.acquire`/`worktree.release` under
`agents-plugin-tool/internal/mcp/` covering create-new, reuse-idle, skip-dirty/
skip-branch-checked-out, hygiene-reset, detach-on-release, pool-root resolution
from a linked worktree, and `.git/info/exclude` self-registration. The
prerequisite `root_override`/rsrc split already carries coverage (260731); this
phase adds only the worktree-lifecycle fixtures.

### Phase 2: Gated worktree parallel route in lead-run

Add to `lead-run`'s Select/Spawn path an opt-in parallel route that activates
only on explicit user approval for the run — this per-run approval is the single
user gate, and it gates worktree *provisioning* itself, since derivation is
expensive in large or submodule-heavy repositories. When active: an
Explore/`tickets.query` pass (reusing the branch-aware Select landed by
`260911-feat-impl-derivation-hardening-branch-aware-select`) identifies a
parallel-safe batch from `ready/`. The parallel-safety gate is **dependency**,
not file overlap: two tickets are parallel-safe when neither functionally
depends on the other's output (no `related:`/`parent:` ordering-dependency edge,
"A needs B's feature"); dependent tickets are sequenced. File-scope overlap is
*not* a hard exclusion — overlapping tickets may still run in parallel, and any
real conflict surfaces at the lead's serialized merge as a merge stop. The lead
spawns one worker per ticket, each on a worktree from `worktree.acquire`, still
one report handled per the existing stop protocol but across N concurrent
workers. The lead then merges the finished branches into the goal branch through
the lead-owned `ws/git.merge`
(`260911-feat-ws-git-merge-lead-owned-merge-authority`) serially, in dependency
order, treating any cross-worker file overlap as a merge stop, and returns each
worktree via `worktree.release`. Mirror to `agents-plugin-wsflow`.

Settled here (no longer open):

- **(a) Parallel-safety predicate** — dependency-based, not file-overlap-based;
  see the Phase 2 body above. File overlap is tolerated and caught at the
  serialized merge, not a go/no-go gate.
- **(f) Branch-creation ownership** — `worktree.acquire`'s `target_branch`
  pre-creates the `impl/<parent>/<slug>` branch, and the worker suppresses its
  own PARENT-branch capture / branch creation. One branch-creation owner (the
  tool), so no double-create against a pre-provisioned worktree.

Deferred to execution time (explicit, with rationale):

- **(b)** How the lead batches N concurrent stop reports without regressing the
  serial veto/merge-approval model — shaped once the concurrent report path is
  built.
- **(c)** Whether the `/goal` Stop-hook starvation that killed the old fan-out
  is fully removed by worktree isolation or needs its own guard — verified
  against the built route, not pre-guessed.
- **(d)** A cap on concurrent workers.
- **(h)** A concurrency claim/lock so two runs never grab the same idle
  worktree, plus a pool cap and GC — deferred while the project holds its
  single-maintainer-serial posture (no concurrent runs to race), revisited when
  real parallel runs exist.

(The lettering keeps the original rehearsal notes' labels; (e) and (g) were
never assigned and are intentionally absent.)

Verification: playbook/skill-shim tests that the parallel route stays inert
without per-run approval (default serial behavior unchanged), that an approved
run selects a dependency-disjoint batch and spawns one worker per worktree, and
that merges route through `ws/git.merge` serially with a cross-worker overlap
surfaced as a merge stop. Mirror-drift test in `agents-plugin-wsflow` per
`wsflow-mirroring.md`.
