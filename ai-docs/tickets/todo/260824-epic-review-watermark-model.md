---
title: Time-series watermark review model — altitude-calibrated review keyed to mainstream integration
related:
  260729-research-implement-router-prose-only-dimension: motivating incident — prose-only change triggered a full partitioned review pass; documents the "default review pass too heavy" root cause
  260627-feat-enter-implement-deterministic-verdict-engine: substrate — owns the deterministic review_alloc derivation that child ①modifies
  260726-bug-lead-implement-lost-review-relay-cycle-cap: interaction — review-cycle budgets are keyed on the review_alloc label child ①shifts toward single/lead-only
  260611-research-ws-per-role-delegation-tuning-config: adjacent axis — role→tier tuning, distinct from this epic's count/scope axis; no conflict
  260829-research-review-watermark-multi-maintainer-model: source of the 2026-08-29 multi-maintainer re-adjustment (canary, no-squash, taxonomy, two backends, ④ tag-keying)
sage-review-design: completed
---

# Time-series watermark review model — altitude-calibrated review keyed to mainstream integration

## Scope

Reshape the ws review workflow so review is calibrated by **altitude** and keyed
to **mainstream-integration time-series**, not to per-ticket/per-phase topology.

Motivating problem: the default review pass is too heavy. Every phase is
reviewed at final-deliverable gravity — three reviewers on a three-line change,
and gaps that the very next phase resolves get flagged as findings (safety
scaffolding added in phase N, removed in N+1). The workflow reviews each small
slice as if it were a terminal deliverable; it lacks awareness of the larger arc.

Core model shift (per-ticket → per-mainstream-merge):
- Review is no longer bound to a ticket/epic lifecycle. It is bound to
  **mainstream integration moments**, and its scope is the **cumulative
  unreviewed trunk diff** (`marker..HEAD`), not a ticket's own diff.
- A ticket/epic closing becomes a **trigger heuristic** ("a good time to
  sweep"), never the review unit itself.
- Topology-independent by construction: it embraces messy reality (interleaved
  epics, `master`↔`impl` direct ping-pong, downstreams with no `main`/`develop`
  split and no release boundary) instead of forcing a branch topology onto it.

Three-layer defense this produces:
- **per-phase** (inline, `enter.implement`): slice self-consistency only, kept
  light. Low-signal changes resolve to `single` — one delegated full-scope
  reviewer, the default floor. `lead-only` (no delegate) is retained but
  narrowed to genuinely trivial changes; it is not dropped, and `single` is not
  a hard one-delegate floor beneath which nothing exists.
- **sweep** (advisory, at integration moments): delegated divided review over
  `marker..HEAD` + a landing lens (convention/doc completeness). Fix-forward,
  never blocks.
- **release gate** (mandatory, only where a project declares a release
  boundary): review of the unreviewed range before promotion. devenv = ship
  (`develop`→`main`); downstream without a boundary gets no gate.

Host-neutral principle: the **mechanism** (marker + range review + sweep) is
universal; the **gating policy** (whether/where review blocks) is per-project
config. devenv is one configuration, not the model.

Collaboration-concurrency axis (added by the 2026-08-29 multi-maintainer
re-adjustment; see research `260829-research-review-watermark-multi-maintainer-model`):
"host-neutral" originally covered harness (Claude/Codex) and branch topology, but
**not multiple maintainers integrating in parallel** (merges landing through
platform UIs outside any ws session). The model's baseline stays the
**single-maintainer serial path** (devenv); **multi-maintainer support is an
opt-in capability layer that stays dormant for the serial case** — the ledger
canary, the no-squash/SHA-preservation constraint, and the platform-enforcement
backend all no-op unless concurrent maintainers actually exist. This is a
generalization, not an overturn: the serial baseline is unchanged.

## Non-Scope

- Not a new review engine. `lead-review`'s existing phase machinery
  (intent/alignment/risk, is-large-diff, verdict routing) is reused; only diff
  selection is parameterized.
- No new numeric `num_reviewer` input on `enter.implement`, and no fixed
  count→role priority table. Rejected: count-first + fixed-priority mapping is
  less expressive than the existing risk-keyed partition selection (it would
  misroute the single reviewer — e.g. assign a correctness reviewer to a
  fit-only change), and a raw agent-chosen count re-introduces the
  non-determinism the verdict engine (260627) deliberately removed.
- No mandatory mid-level "arc branch" and no branch nesting
  (`strm/…`, `impl/strm/…/…`, physical per-epic integration branches). Rejected
  in favor of the topology-independent watermark after the user established that
  real work is too messy/interleaved for a clean per-arc branch.
- No `post-merge` MCP tool and no MCP-mediated merge primitive. Rejected: an
  honor-system post-merge call is as forgettable as the native merge it follows;
  a lazy fallback that recomputes at observed checkpoints is needed regardless,
  which makes an merge-time hook redundant. Merges stay native.
- Per-ticket verdict-in-ticket-body recording is dropped (review is no longer
  per-ticket); verdicts live in the range-keyed ledger instead.

## Child Tickets

- Planned ①`refactor`/`feat`: **per-phase review lightening.** In
  `enter.implement`'s resolver, stop treating `unknown` risk facts as material
  (`materialRisk`), so a partition needs a positive signal — the default
  `review_alloc` collapses to `single`/`lead-only` far more often. Reaffirm the
  floor as one delegated full-scope reviewer (the existing `single` behavior);
  narrow `lead-only` to genuinely trivial changes (lead-performed review burns
  the lead context the workflow exists to save). **Independent — deliverable on
  its own, no dependency on ②–④; delivers the original pain relief immediately.**
- Planned ②`feat`: **`lead-review` range/watermark scenario.** Parameterize
  diff selection by scenario kind so review can target an arbitrary
  `range` (`marker..HEAD`) instead of a checked-out branch. `git.diff(range:)`
  already supports it; the phase machinery is diff-agnostic and reused. Adds a
  **landing lens** (convention adherence + spec/mental-model update
  completeness) as a review-config required-check — the implementation-side
  symmetry of sage's completeness stage. Prerequisite for ③and ④.
- Planned ③`feat`: **watermark marker + review ledger + advisory sweep.** New
  tracked dotfile ledger under `ai-docs/` (per-track, travels with the branch
  tree); each review appends `<base>..<head>: verdict`. Marker = the latest
  entry's through-SHA (no separate field). Lookup is line-scoped (parse the last
  entry), never "last commit that touched the file." Sweep triggers are
  advisory (fix-forward), scaled to unreviewed-range size/staleness, and
  **lazily recomputed at observed MCP checkpoints** (`tickets.close` — which is
  post-merge by construction; `workflow_manual` at session start; `enter.*` as
  backstops) rather than caught at merge time. Depends on ②.
- Planned ④`feat`/`chore`: **project policy + release gate.** `AGENTS.md`
  (tracked, per-track) declares the review-track branch, whether a release
  boundary exists, **and (2026-08-29) which rendezvous backend the project uses
  (platform / any-git canary).** `workflow_manual` surfaces a non-blocking
  "configure this first" nudge when unset. `_review.local.md` holds review
  mechanics. For a project that declares a boundary, insert a mandatory range
  review into the promotion path — for devenv, into `lead-ship` pre-flight (which
  today has no review step), reviewing the unreviewed range before
  `develop`→`main`. **The gate range is keyed off the last stable release tag
  (`<tag>..HEAD`), not the precise marker — tags survive squash/rebase, so the
  gate is squash-robust; the precise marker is demoted to the advisory mid-stream
  nudge layer.** The forcing function (un-routed/open block entries block
  promotion) is unchanged. Downstream without a boundary: advisory-only. Depends
  on ②(and ③for the marker; a boundary project can fall back to `main..develop`
  when no tag/marker exists yet).

## Cross-Child Decisions

- **Marker storage:** the range-keyed dotfile ledger under `ai-docs/` is the
  single home for both verdict history and the marker (latest entry's
  through-SHA). Not `AGENTS.md` (a fast-advancing SHA would churn the tracked
  orientation doc), not a git ref, not the note layer.
- **Marker lookup:** always line/field-scoped (last ledger entry). Never derive
  the marker from "the last commit that modified the ledger/AGENTS.md," and never
  by traversing merge parents — the marker is a *file line*, not a commit.
- **Marker mechanics (serialized-merge model):** the marker lives on and is
  written **only by the review-track branch (master)**; feature/impl branches
  never append. A review stamps the ledger as the *final atomic step of landing on
  master* — riding a master merge (primary trigger) or a standalone catch-up sweep
  on master — never prematurely on an un-merged branch. The review range is
  **set-subtraction** (`{reachable from HEAD} − {reachable from marker}`), so the
  marker sitting on the master-side parent subtracts already-reviewed history in
  one shot and a two-parent merge creates no ambiguity. Because master merges are
  serialized and only master writes the ledger, appends never race → the
  append-only ledger never merge-conflicts **in the single-maintainer serial
  case**. In the multi-maintainer concurrent case that serialization does not
  hold, and the resulting ledger tail conflict is **not a bug but the intended
  canary rendezvous** (see the Multi-maintainer decisions below): two branches
  that both stamp without one absorbing the other necessarily collide on the
  ledger tail, forcing a STOP. **Skip-coverage invariant:** the
  marker rises only by an actual stamping review, so anything in `(marker, HEAD]`
  is unreviewed regardless of how it landed (a review-skipping merge included), and
  the next stamping review — or the gate — inescapably sweeps it; no skip
  masquerades as reviewed. (Owned in detail by ③; supersedes any reading of
  "marker = last entry" as a graph-walk.)
- **Range key is a commit SHA, never a wall-clock timestamp** (git ranges are
  commit-based; a timestamp maps to a commit only fuzzily). A human-readable
  timestamp may accompany an entry as metadata but must not be the range key.
- **Bootstrap is explicit, never silent.** When no marker exists, insert one at
  current `HEAD` and surface it: prior history is treated as *review-skipped*,
  not *reviewed*. Silent bootstrap would give false confidence when a later
  sweep reports "clean."
- **Advisory sweeps never block; the release gate is the only hard gate**, and
  only where a project declares a release boundary. Ignored advisory nudges are
  safe because the release gate (and, mid-stream, the size/staleness-scaled
  nudge) is the backstop; the size threshold is the pressure-relief valve.
- **Finding resolution path — arc-review *surfaces and routes*, the ticket
  system *resolves*.** A sweep/gate is not an inline fix loop: a finding it
  raises is handed off by **ticketization** (a new `idea/`/`todo/` ticket) or the
  communication path `lead-review` already defines (its verdict routing —
  LGTM / NEEDS FIX / OPEN). Resolution is then tracked by the **routed ticket's
  status**, never by a mutable "resolved" flag in the ledger: a non-pass ledger
  entry *references* the routed ticket stem, and the append-only ledger stays
  pure history. The **release gate (④)** consults ledger blocking entries since
  the last release cross-referenced against those tickets' status — it blocks
  promotion while a recorded blocking finding's routed ticket is *open*, and — the
  **forcing function** — a `block` entry with **no routed ticket at all** is
  itself un-clearable, so a blocking finding can never be dropped on the floor. It
  clears only when the routed ticket is *terminal*: `.done` (fixed) or `.dropped`
  (a conscious won't-fix/superseded/invalid decision, which already records a
  rationale by ticket convention). **There is no separate "lead waiver" artifact**
  — `.dropped` is the conscious-accept path, reusing the ticket lifecycle and
  needing no new config home. This is the concrete close-path for the
  marker-always-advances decision: the marker moving past a failing range is safe
  precisely because the finding lives on as a tracked ticket, not a lost note.
  Per-phase (①) findings are unaffected — they keep the existing inline implement
  relay, fixed before the slice lands.
- **`config` split (three homes):** `AGENTS.md` = tracked structural config
  (review-track branch, release-boundary declaration); `ai-docs/` ledger =
  tracked marker+verdict state; `_review.local.md` = machine-local review
  mechanics (phases, remote, thresholds).
- **Host-neutral first:** the marker/range/sweep mechanism is universal; gating
  is opt-in per-project policy. Never encode devenv's `develop`/`main`/ship
  shape as the mechanism.
- **Ledger honesty:** an entry recorded as reviewed without an actual review is
  false confidence — same discipline as the bootstrap surface. Verification
  expectations for ③/④must guard against recording unreviewed ranges as
  reviewed.

### Multi-maintainer decisions (2026-08-29 re-adjustment)

Added from research `260829-research-review-watermark-multi-maintainer-model`.
These generalize the model to concurrent maintainers **without** changing the
single-maintainer serial baseline; each is dormant in the serial case.

- **Mechanical-vs-convention taxonomy — the organizing rule.** The line between
  "must be mechanically enforced" and "convention is acceptable" is drawn by
  *failure mode*: a safety invariant (violating it opens a **coverage hole →
  under-review**) must be mechanical; an efficiency optimization (violating it
  only causes **over-review = wasted effort**, never under-review) may be a
  convention.

  | Property | Must be mechanical? | Why |
  |---|---|---|
  | Coverage of brute commits | Yes | Violation = missed review (under-review). Already mechanical by construction: marker advances only on a stamp (skip-coverage). |
  | Rendezvous (concurrent branches re-integrate + re-review) | Nice-to-have | Violation only re-reviews the same range = over-review. Canary forces the STOP mechanically; re-review is convention (or platform-enforced). |
  | No-squash on review-track | No | Violation orphans the marker → next range simply re-covers the orphaned span = over-review, never under-review. |

  Proof the two efficiency layers are safe as conventions: an orphaned or ignored
  marker still leaves every brute commit inside `(marker, HEAD]`, so coverage is
  untouched — only cost rises. **Only coverage must be airtight, and it already
  is.**

- **Ledger canary (opt-in-free rendezvous for concurrent landings).** Because
  every wsflow landing appends to the ledger tail, two branches that both stamp
  without one absorbing the other **necessarily** produce a textual merge
  conflict on the ledger (git cannot auto-merge two inserts at the same anchor).
  This forces a STOP — the later branch cannot land until it resolves the
  conflict, i.e. integrates the other's work. Works identically on GitHub (its
  merge/squash/rebase all run `git merge` and report the conflict) with **no
  branch-protection config required**. Dormant in the serial case (no concurrent
  writers). Owned in detail by ③, including a **self-documenting ledger banner**
  so the resolver sees a THINK prompt at the conflict site. Honest limit: the
  canary forces STOP, not THINK — a resolver can `checkout --theirs` and proceed;
  this is physically irreducible in plain git and is mitigated (not closed) by the
  banner and, where available, the platform backend.

- **SHA preservation / no-squash on the review-track (efficiency convention).**
  Squash/rebase landings rewrite the reviewed commit to a new SHA, orphaning the
  marker. Review-track landings are constrained to **ff or merge-commit** (both
  preserve the SHA); squash/rebase are forbidden **on the review-track**. This is
  a convention because violating it only causes over-review. Landing topology:
  **Merge 1 `HEAD <- master`** (absorb master, resolve here, review the resolved
  commit R, append the ledger as a separate final commit L) then **Merge 2
  `master <- HEAD`** (SHA-preserving land: ff preferred locally, "create a merge
  commit" on GitHub). The ledger entry's `<head>` is R (the reviewed commit),
  never L. "Always force no-ff regardless of ff-ability" is an over-constraint and
  is rejected — the real invariant is SHA preservation, which ff and no-ff both
  satisfy.

- **Two enforcement backends for rendezvous (per-project choice).**
  *Platform-enforced (GitHub, preferred where available):* branch protection
  (`require branches up to date` + `dismiss stale approvals on new commits` +
  `require status checks` + disable squash/rebase; merge queue at scale) — forces
  STOP **and** re-review (closes the canary's STOP-not-THINK gap). *Platform-
  independent (any git, no config):* the ledger canary — forces STOP only,
  re-review is convention. A project declares which backend it uses (④ config
  surface). Both leave coverage (the only mandatory-mechanical invariant)
  untouched; they differ only in how strongly they enforce the efficiency
  rendezvous.

## Completion Criteria

- Done: ①–④ landed — per-phase default resolves to single/lead-only for
  low-signal changes; `lead-review` can review an arbitrary range with the
  landing lens; the ledger/marker + advisory sweep with lazy checkpoint
  recomputation is live; and devenv's ship path enforces a mandatory range
  review before `develop`→`main`, with the mechanism documented as host-neutral
  policy-driven behavior.
- Risk framing watches **both** directions, not just overhead. Overhead: if
  dogfooding shows the watermark model does not reduce review load versus ①
  alone, that is a reason to reconsider ②–④. Under-review (the direction a naive
  reading misses): if the lightened per-phase floor (①) lets **defects escape to
  the release** — i.e. the gate/sweep starts catching things per-phase used to —
  revisit ①'s floor (raise it, or re-widen the partition signals) rather than
  treating overhead reduction as unqualified success. Instrument both signals in
  dogfooding. Multi-maintainer axis: watch whether the canary fires at the right
  moments (concurrent branches that skipped absorption) without false positives in
  the serial case, and whether a resolver actually re-reviews after a canary STOP
  rather than blindly `checkout --theirs`.
- No-boundary trade is explicit and accepted: a project that declares no release
  boundary has no hard gate and advisory-only sweeps, so with ① lightening
  per-phase its only mandatory scrutiny is the single per-phase reviewer. This is
  a stated trade (①'s floor keeps it from "no review"; declaring a boundary opts
  into the gate), **not** a silent hole — and the "keep ① / drop ②–④" fallback is
  therefore *not* free: dropping ②–④ removes even the advisory sweep and deepens
  the no-boundary under-review gap, so that fallback is a last resort, weighed
  against the defect-escape signal above, not a default.
- Deferred: MCP-mediated merge for immediate (non-lazy) sweep triggering;
  role→tier delegation tuning (260611); one-gate-vs-separate-posture form of the
  landing lens (captured as a folded required-check in ②, may split later); the
  concrete recommended GitHub branch-protection / merge-queue configuration set
  (multi-maintainer backend, `260829` open question); the git-naive brute-only
  persona relief valve (agent-proposed/agent-run review at checkpoints) — split
  to its own research ticket, not folded here.
