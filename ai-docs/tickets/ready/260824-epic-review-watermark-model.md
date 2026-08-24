---
title: Time-series watermark review model — altitude-calibrated review keyed to mainstream integration
related:
  260729-research-implement-router-prose-only-dimension: motivating incident — prose-only change triggered a full partitioned review pass; documents the "default review pass too heavy" root cause
  260627-feat-enter-implement-deterministic-verdict-engine: substrate — owns the deterministic review_alloc derivation that child ①modifies
  260726-bug-lead-implement-lost-review-relay-cycle-cap: interaction — review-cycle budgets are keyed on the review_alloc label child ①shifts toward single/lead-only
  260611-research-ws-per-role-delegation-tuning-config: adjacent axis — role→tier tuning, distinct from this epic's count/scope axis; no conflict
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
  (tracked, per-track) declares the review-track branch and whether a release
  boundary exists; `workflow_manual` surfaces a non-blocking "configure this
  first" nudge when unset. `_review.local.md` holds review mechanics. For a
  project that declares a boundary, insert a mandatory range review into the
  promotion path — for devenv, into `lead-ship` pre-flight (which today has no
  review step), reviewing the unreviewed range before `develop`→`main`.
  Downstream without a boundary: advisory-only. Depends on ②(and ③for the
  marker; a boundary project can fall back to `main..develop` when no marker
  exists yet).

## Cross-Child Decisions

- **Marker storage:** the range-keyed dotfile ledger under `ai-docs/` is the
  single home for both verdict history and the marker (latest entry's
  through-SHA). Not `AGENTS.md` (a fast-advancing SHA would churn the tracked
  orientation doc), not a git ref, not the note layer.
- **Marker lookup:** always line/field-scoped (last ledger entry). Never derive
  the marker from "the last commit that modified the ledger/AGENTS.md."
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

## Completion Criteria

- Done: ①–④ landed — per-phase default resolves to single/lead-only for
  low-signal changes; `lead-review` can review an arbitrary range with the
  landing lens; the ledger/marker + advisory sweep with lazy checkpoint
  recomputation is live; and devenv's ship path enforces a mandatory range
  review before `develop`→`main`, with the mechanism documented as host-neutral
  policy-driven behavior.
- Dropped: if dogfooding shows the watermark model does not reduce review
  overhead versus a simpler `enter.implement` allocation retune (① alone), keep
  ① and drop ②–④.
- Deferred: MCP-mediated merge for immediate (non-lazy) sweep triggering;
  role→tier delegation tuning (260611); one-gate-vs-separate-posture form of the
  landing lens (captured as a folded required-check in ②, may split later).
