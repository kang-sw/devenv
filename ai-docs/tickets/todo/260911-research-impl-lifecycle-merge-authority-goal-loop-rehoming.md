---
title: "Impl-branch lifecycle re-tightening: lead-owned merge authority (ws/git.merge), derivation hardening, and host-neutral goal-loop re-homing"
related:
  260910-feat-lead-run-worktree-parallel-route: adjacent; develop's retired lead-goal-fan-out-step is the prior-art parallel design this ticket recovers, and merge centralization here is a precondition for that route
  260909-epic-ws-worker-interpreter-refoundation: context; the refoundation renamed lead-drain-ready-queue to lead-run and left goal-loop consumers dangling
  260911-refactor-lead-run-ticket-only-delegate-implementer: adjacent; that ticket removes worker/lead surface, this one moves merge off the worker onto a lead tool
  260911-research-golden-fixture-verification-gap: sibling research from the same dogfood session
---

# Impl-branch lifecycle re-tightening

## Background

A dogfood session on 2026-09-11 surfaced three linked problems in the impl-branch
lifecycle and the goal loop, which discussion converged into a closed design. The
trigger observations: (1) an autonomous fast-forward merge of an impl branch into
its base flattening plan/review/doc history; (2) `lead-run` attached to a goal
context not creating a `goal/*` branch at all; (3) a suspicion that starting a new
ticket while an impl branch is unmerged breaks the impl branch-naming rule.

Two read-only diagnosis passes established ground truth. This ticket records the
closed map so implementation children can be derived without re-deriving it. Child
split is intentionally deferred (see the last section). Evidence is cited as
`path:line` against the epic/refound tree unless a branch is named.

## Finding: the derivation policy is robust; the real defects are narrow

`deriveImplementBranchPlan`, `finishImplementBranchPlanTail`, and
`aheadOfMergeRootCount` in `agents-plugin-tool/internal/mcp/implement_resolver.go`
are **byte-identical between develop and epic/refound**; the refoundation changed
only route-facts instruction wording, not branch-plan logic. The user's expected
derivation invariants all HOLD on both branches:

- **(i) broken stem-homogeneity stops.** On an `impl/<root>/<stem>` branch, a
  different ticket with `AheadOfMergeRoot > 0` → `Action="stop"` with a
  `SuspectedOwnerStem`, not overridable by `allow_rename`
  (`implement_resolver.go:954-961`).
- **(ii) same ticket continues unmerged.** `CurrentBranch == targetBranch` →
  `continue`, checked *before* the ahead>0 guard, so a ticket's later phase
  silently continues on the same impl branch (`:949-953`). ScopeSlug is
  deterministic per stem via `wskey.Derive(TicketStem, 3)` (`:732-736`,
  `wskey.go:78-86`), so all phases of one ticket resolve the same branch.
- **(iii) impl never nests.** The ref-minting create path is guarded by
  `!isImpl && !isImplement`; on an impl branch `parseImplBranchRoot` splits on the
  last `/` to recover the root and builds a **sibling** `impl/<root>/<newstem>`,
  never `impl/impl/...` (`:817-824`, `:910-926`).

So the earlier "cycle-boundary branch-hygiene hole" is **mostly already guarded**:
a leftover impl branch checked out while a different ticket starts is a resolver
*stop*, not a silent contamination. The residual defects are narrow:

- **Fail-open guard.** `aheadOfMergeRootCount` returns `0` on any git error
  (`:555-569`), silently disabling the (i) stop exactly when git misbehaves. A
  safety guard that self-disables on error is backwards; it should fail *closed*
  (stop-and-surface the unverifiable state) rather than proceed.
- **Create-path base inheritance (narrow).** The create path never calls
  `finishImplementBranchPlanTail`, so `AheadOfMergeRoot` is inert there
  (`:890-908`, test `:334-350`): a new impl minted from a genuinely-dirty
  non-impl parent tip inherits whatever unmerged commits sit on that parent, with
  no guard. Practically narrow (base branches normally carry only landed work),
  but real if a parent is left dirty by an out-of-band mutation.
- **Slug legibility.** The stable per-stem slug is an opaque 3-word hash
  (`wimp-frame-suing`), which reads as per-run randomness and is why (ii) was
  *suspected* broken though it holds. A tool-computed **readable-prefix +
  short-deterministic-hash-suffix** (e.g. `impl/<root>/lead-run-8fa2`) keeps
  determinism and collision-safety while restoring recognition without relying on
  a model's semantic guess. (Full-stem slug is the zero-logic alternative; git ref
  limits are not a constraint.)

## Design: lead-owned merge authority via `ws/git.merge`

The observed autonomous ff merge is **not prescribed by source** — the resolver
never merges (`route.resolve_implement` returns advisory todos only, "This tool
performed no merge", `implement_resolver.go:585-595`). The regression is a
**playbook-prose vs installed-todo divergence**: `ticket-worker.md:83` ("Merge per
the route verdict: into the goal branch on your own") reads prescriptively and
names no `--no-ff`, while the installed todos default to no-merge/opt-in
(`session_state.go:513-542`). `CHANGELOG.md:499-505` already documents leads
ff-flattening impl branches. Rather than patch the prose, remove merge from the
worker and route it through a constrained lead tool, mirroring how `ws/git.commit`
succeeds by enforcing structure:

- **New `ws/git.merge` tool.** Always `--no-ff` (ff-flatten structurally
  impossible); validates the merge target against the impl branch name's encoded
  root (`parseImplBranchRoot`); refuses forbidden targets (never `main`, per Branch
  Policy); preserves the workflow merge record and commit rules; and on success
  **auto-deletes the merged impl branch** (restoring the auto-delete removed in
  `CHANGELOG.md:1049-1076`). Merge conflict (structurally absent in the serial
  case, possible only under the parallel route) derives out to `lead-delegate`.
- **Take merge off the worker entirely.** Reverse `ticket-worker*.md:83` and
  `worker-stop-protocol.md:20` ("merging impl into the goal branch is yours"): the
  worker never merges, including the impl→goal self-merge it does today under
  `merge_confirm=skip`. This aligns with the parallel-worktree future where the
  lead serializes all merges (see 260910's recovered fan-out overlay). Cost is one
  extra lead tool call, judged context-coherent.
- **Re-home `merge_confirm` to the lead.** With the worker out of merging,
  `merge_confirm` stops meaning "does the worker ask before merging" and becomes a
  lead-side signal: `skip` = the lead auto-calls `ws/git.merge`; `ask` = the lead
  surfaces to the user first.
- **Add the merge step to lead-run's report handling.** Today `stop: none`
  (closed on impl) just advances the note and ends the turn (`lead-run.md:97-101`)
  — there is no merge step, so impl branches accumulate unmerged in the serial
  non-goal flow. The gated merge (`ws/git.merge` under the re-homed
  `merge_confirm`) belongs in **Handle the report**.
- **Enforce close-time merge review.** `implementCloseMergeReviewNudge` is
  advisory and fail-open (`implement_resolver.go:571-595`). Keep it a nudge but
  make it a real warning at `tickets.close` when an impl branch is left unmerged —
  cheap potential-contamination detection.

## Finding: the goal loop is orphaned; recover the develop strategy

develop's `lead-drain-ready-queue` (both the base skill and the
`lead-goal-fan-out-step` overlay) carries the full goal-loop design. The branch
**creation strategy** survived nearly verbatim into `lead-run.md:55-60`; what died
is the **activation driver**.

- **Two OR'd triggers.** A goal run = current branch `goal/*` **or** an active
  `/goal` Stop-hook reminder (develop `lead-drain-ready-queue/SKILL.md`, Posture).
- **Staging (creation).** On the first turn, when a `/goal` reminder is active and
  not already `goal/*`: capture `PARENT = git rev-parse --abbrev-ref HEAD`, then
  `git checkout -b goal/<parent>/<slug>` (random word-word-word slug, never
  goal-text-derived — the collision fix from `260713-bug-...-goal-branch-slug-collision`);
  on detached HEAD, abort staging and dispatch unstaged.
- **Autonomous posture + prefer-subagent.** "User is away": resolve reversible
  decisions on the lead's own recommendation, record one line, continue; delegate
  everything including commits. This posture and `lead-prefer-subagent` are retired
  on epic/refound.
- **The dead chain.** Staging is gated on the `/goal` reminder, which nothing live
  emits: the emitter was a Claude harness built-in (external to the repo), then
  `agents-plugin-pi/src/goal-loop.ts` (a Pi adapter re-injecting on every
  `agent_settled`), and **`agents-plugin-pi/` is fully retired**. Without the
  reminder, turn 1 never stages the branch, so the loop never reaches `goal/*`, so
  the branch-name trigger never engages either — dead from turn 1. Codex's native
  `thread/goal/*` is "bookkeeping only, not an auto-looping primitive"
  (`ai-docs/ref/agent-harness-capability-tiers.md:29-30`).

**Two separable axes, decided.** (1) *Goal-branch handling* — develop's
`lead-drain-ready-queue` goal prose (Posture, staging, `goal/*` terminals) is
known-good and worked in dogfooding; the concrete direction is to restore it
**verbatim-grade into `lead-run`'s goal path**, where the refoundation's
condensation degraded it. This is a localized, low-risk edit, not an open
question. (2) *Loop re-fire / emitter* — re-invoking `lead-run` each turn is
inherently harness-dependent (the Claude `/goal` built-in emits the reminder the
staging condition reads; hosts without it need an adapter). This axis stays
deferred and is entangled with the binding anchor
`260605-research-ws-native-subagent-pivot`; retiring the dangling consumers
applies only if the loop is abandoned. Do not conflate the two: restoring the
prose fixes goal-branch *creation* under a host that emits `/goal`; the auto-loop
is separate.

**Fan-out is a lead-run extension mode, not a separate skill.** develop factored
the parallel path into a separate `lead-goal-fan-out-step` overlay; the decision
here is the opposite — when it returns, reintroduce it as an extension *mode* of
`lead-run` to keep the cognition point unified. develop's overlay is the prior-art
design: one worktree + fresh `impl/<parent>/<stem>` per ticket, `ferrule(root,
capability: "lead", parent_session_key)` minting each mini-lead key, a **merge
subagent in the parent's sole checkout serializing all merges**, and a
session-note board as the in-flight ledger, degenerating to serial below two
independent tickets. This feeds 260910 directly.

## Design: branch-aware selection via a `ticket-selector` playbook

`lead-run`'s Select ignores the git branch today (`lead-run.md:19-25`, scanning
`ready/` files only), so it can pick a different ticket while sitting on an
in-progress impl branch — a wasted spawn the resolver then stops. Select must
become branch-state-aware:

- **On a base branch:** current behavior (skip Blocked; in-progress >
  prerequisite > oldest over `ready/`).
- **On `impl/<root>/<slug>`:** the branch owns an in-progress ticket. If that
  ticket has an unfinished phase, select it (continue on the same branch) and do
  not consider other `ready/` candidates — starting a different ticket would
  strand this impl branch. If the owning ticket is closed but the branch is
  unmerged, selection cannot proceed: raise a merge recommendation to the lead
  (the merge gate above).

The hard part is the branch→owning-ticket reverse map: the opaque `wskey.Derive`
slug is not reversible, so this couples to the readable-slug hardening (derivation
finding above) or needs a deterministic resolver helper returning the owning stem
for a branch (deriving candidate slugs and matching). Keep that determinism in the
tool/resolver; the playbook only orchestrates.

Decision: **extract Select into a renderable `ticket-selector` playbook.** The
branch-aware logic is shared by lead-run's serial mode, the future fan-out mode's
batch selection (a superset), and the wsflow mirror; a single rendered playbook
beats inlined duplication and unifies the cognition point. Split of concern:
`ticket-selector` orchestrates (branch detect → owning-ticket lookup → ordering →
merge-recommendation terminal); the resolver/tool computes the deterministic
branch→stem and slug derivation. New playbook = Ask-first at implementation time.

## Deferred: child-ticket split

Child derivation is deferred to the next session (context is nearly full). The
natural clusters: **(A) merge authority** (`ws/git.merge` + worker/stop-protocol
reversal + lead-run report merge step + `merge_confirm` re-home + auto-delete; new
MCP tool = protocol/Ask-first, plus shipped-surface edits and a wsflow mirror);
**(B) derivation hardening** (fail-open→fail-closed, readable+hash slug; mostly Go
resolver); **(C) goal-branch prose restore** (verbatim-grade from develop into
lead-run; the loop re-fire/emitter is the separate deferred host axis); and
**(D) branch-aware selection** (the `ticket-selector` playbook + resolver
branch→owning-ticket helper, coupled to B's readable slug). The commit-history
soft-continuity idea folds into D as a lower-priority refinement rather than a
separate parked item.
