---
title: Agent-proposed review relief valve at workflow checkpoints — the git-naive brute-only persona
related:
  260824-epic-review-watermark-model: parent axis — this addresses a persona gap the epic's advisory sweep leaves open (a nag with no relief valve the user can operate)
  260824-feat-lead-review-range-scenario: hard prerequisite — the "agent runs the review for you" valve is impossible until lead-review can review an arbitrary range without a checkout (② wires git.diff range)
  260824-feat-review-watermark-ledger: owns the checkpoint recompute/nudge surface this would upgrade from passive FYI to an active proposal
  260829-research-review-watermark-multi-maintainer-model: sibling research — the multi-maintainer axis; this split off from it so the multi-maintainer re-adjustment stays single-axis
---

# Agent-proposed review relief valve at workflow checkpoints — the git-naive brute-only persona

## Background

A discuss session (2026-08-29) stress-tested the review-watermark epic against a
persona it does not design for: a **git-naive developer who installed the ws
plugin but uses it brute-only** — communicates with the agent via
`discuss → proceed` only, never branches/reviews/merges by hand, never runs a
sweep. The multi-maintainer machinery (canary, no-squash, backends) stays
correctly dormant for this user, but a **latent UX gap in the base epic**
surfaced, and it was split out of the multi-maintainer re-adjustment
(`260829-research-review-watermark-multi-maintainer-model`) so that re-adjustment
stays single-axis. **Nothing here is accepted for implementation; this captures a
direction for a later clean-session re-discussion.**

The gap is real but bounded — it is a nuisance/UX problem, not a safety hole:
coverage is still guaranteed by skip-coverage, and honestly-marked-unreviewed
work never masquerades as reviewed.

## The persona and what actually happens (evidence-grounded)

Traced against the current implementation (Explore, 2026-08-29):

- **Merge is opt-in, not automatic.** `enter_implement` creates
  `impl/<base>/<slug>` but the default terminal outcome is "stay on the impl
  branch, unmerged" (`agents-plugin-tool/internal/mcp/session_state.go:632-672`);
  merge runs only when explicitly chosen. Low-ceremony/inline work instead
  commits **directly on the current branch**, no impl branch
  (`implement_resolver.go:672-682`). So a defaults-accepting user ends up with
  either (a) unmerged impl branches piling up, or (b) commits straight on their
  working branch. `enter_proceed` has no branch/merge logic
  (`session_state.go:1130-1154`).
- **① per-phase review still runs.** The inline implement reviewers fire, so this
  user does get slice-level review — it is *not* "no review." But per-phase review
  **does not stamp the ledger / advance the marker** (③ write discipline: only a
  master-merge-time review or a standalone catch-up sweep stamps).
- **Result: the marker never advances**, `marker..HEAD` grows without bound, and
  every checkpoint recompute emits an ever-stronger advisory nudge — **with no
  relief action the git-naive user knows how to take**. The escalating nudge
  becomes pure noise. The semantic wrinkle: ① review genuinely ran, so the work is
  not meaningfully "unreviewed," yet the marker/sweep layer nags as if it were.

**Cold re-verification (2026-08-30).** All four code claims above were
independently re-checked against current source in a fresh review pass (not
trusting the 2026-08-29 line refs): **all four CONFIRMED**, only minor line
drift, none refuted — including the linchpin (`lead-review` checks out the target
branch and diffs the working tree at `lead-review.md:34,39`; it takes no `A..B`
range, so the "② must land first" prerequisite is a real hard dependency).

## Why this is a nuisance, not a safety hole (settled this session)

- **Mixed team (brute contributor + aware maintainer):** brute commits sit in
  `(marker, HEAD]` and are swept by the aware maintainer's next stamping review at
  the integration/maintain stage (skip-coverage). Coverage comes from the aware
  side + structure, **not** from any hard gate. Safe.
- **Solo brute developer:** nobody aware ever reviews, so it is just endless
  nagging — but skip-coverage still marks the work *honestly unreviewed* (never
  false-clean), and it remains in the next release's `<tag>..HEAD` sweep. A hard
  gate here would only be a wall the solo dev bulldozes or a reason to abandon the
  tool; the honest advisory position is correct. (This is the epic's accepted
  no-boundary trade, restated for this persona.)

## Converged direction (working conclusion, not accepted)

Upgrade the **existing** checkpoint nudge from a passive FYI ("range is N commits")
to an **active, qualitative agent proposal** that can *run the review for the
user* on assent — turning a nag no one can act on into a one-word relief valve.

- **The hook point already exists and is already reserved for the epic.**
  `tickets.close` emits `implementCloseMergeReviewNudge`
  (`implement_resolver.go:442-466`) — the one existing advisory surface — and its
  code comment explicitly leaves room for **"epic 260824's later review-watermark
  hook to compose"** (`implement_resolver.go:454`). This ticket is that hook's
  brute-persona shape.
- **Three trigger surfaces** (the checkpoint set ③ already plans), to cover both
  landing paths: `tickets.close` (covers the unmerged-impl-branch path — the
  existing nudge fires only when on an unmerged `impl/*`, `AheadOfMergeRoot>0`);
  `workflow_manual` + `enter.*` recompute (covers the direct-on-current-branch
  path, where the impl-branch nudge never fires); and the opt-in merge step for
  users who do choose to merge.
- **On assent, the agent runs the review itself** via the delegated **range
  scenario (②)** over `marker..HEAD` and stamps → marker advances → nudge quiets;
  coverage rises from ①-only to arc-level.
- **Stays advisory (never blocks).** Repeated "no" is respected (no-boundary
  trade). The escalating pressure remains; only its shape changes from passive to
  proposing.

### Guards carried over from the risk review (Risk 2, this session)

- **Disclose side-effects with every proposal.** The proposal must state that it
  will be a **potentially large task** and name the review's side-effects (cost,
  time), so assent is informed. Disclosure prevents *surprise*, not *cost* —
  **cost is controlled by the trigger threshold** (how much must accumulate before
  proposing), reusing ③'s existing size (is-large-diff) + staleness metrics, not a
  new constant.
- **No self-grading.** The agent that *proposes* the review must not be the one
  that *judges* it: the relief review must go through ②'s **delegated reviewer
  subagents**, never a lead-inline shortcut, or the persona ends up reviewing its
  own work.

### Purpose is (A) coverage, not (B) noise (2026-08-30 re-discussion)

The re-discussion tested whether this valve exists to **(A)** close a real
coverage gap or **(B)** merely quiet a bookkeeping-artifact nag. **Settled: (A).**
Slice review (①) is *deliberately* local — it inspects one phase's diff in
isolation and structurally cannot see cross-slice interactions (phase N+1 breaks
phase N's assumption), whole-arc flow/consistency, or the integration seam
between slices. Arc/integration review over `marker..HEAD` is a **genuinely
different altitude**, so the gap the nudge points at is real, not a bookkeeping
artifact. **This kills the tempting "just let ① advance the integration marker"
shortcut:** advancing the integration marker on a slice review would record
"reviewed up to integration" when only slice review ran — the exact
false-confidence / masquerade failure the epic's ledger-honesty invariant
forbids. The two review altitudes must stay distinct.

### Nudge-severity calibration + slice-coverage signal (2026-08-30 re-discussion)

Because the gap is real (A) **but** this persona's backlog *did* get ① slice
review, two backlog states carry different risk and the nudge should not treat
them identically:

- **raw backlog** (no ① ran — e.g. hand commits that skipped ws ceremony
  entirely): higher risk, louder nudge warranted.
- **slice-covered backlog** (① ran per slice, only arc/integration review is
  missing — the brute persona's case): real gap but one altitude lower; a gentler
  nudge.

A single escalating siren that conflates them trains the git-naive user to ignore
*all* nudges — including the loud raw-backlog case that actually matters (cry
wolf). So the nudge should read a **slice-coverage signal** and scale severity by
the raw-vs-slice-covered split.

**Mechanism — no second marker file.** A separate "slice-coverage marker" file
(the first idea floated) was rejected: it adds a second shared-file append point
and therefore a second merge-conflict surface, and extra complexity. The signal
instead lives in commit text (each commit's own message is private to that
commit, so no shared-file conflict surface). **Chosen (2026-08-30): (b) the
explicit trailer**, with (a) footprint inference documented as a fallback:

- **(b) Explicit altitude-labeled trailer — CHOSEN.** The guaranteed phase-result
  commit (a ws-ceremony run always produces one) carries an explicit
  `Reviewed-slice: <verdict>` trailer. It carries the **actual ① verdict**, so it
  needs no "ceremony ran" ≈ "① passed" approximation — this directly closes the
  sage design review's noted looseness in (a). Cost: one commit-message
  convention, and it is **squash-fragile** (below), which is acceptable because the
  signal only needs to survive the pre-integration window.
- **(a) Footprint inference — FALLBACK.** The recompute instead infers
  slice-coverage from whether the range contains **ws-generated phase-result
  commits** (the ws commit footprint, incl. `## AI Context`): a range full of them
  = slice-covered, a range of raw hand commits = raw. Zero new convention and
  squash-robust, but approximates "ceremony ran" ≈ "① passed". Kept as the fallback
  if the trailer convention proves impractical.

**Discipline — the label must name its altitude.** Whichever option, the signal
must read as *slice*-coverage, never bare "reviewed" — a bare "reviewed" that the
recompute mistakes for integration coverage recreates the (A)-shortcut masquerade
above. Keep it lexically distinct from the integration marker.

**Two catches, both bounded:**

- **Squash-fragility (option b).** A commit-message trailer is erased by
  squash/rebase — the same fragility that pushed the release gate (④) onto tags.
  It is **acceptable here** because the slice-coverage signal only needs to live
  in the *pre-integration window* (while the brute dev is still stacking commits
  on their branch); once work integrates, the integration marker + gate take over
  and the slice signal is no longer consulted. Post-integration unreliability is
  harmless.
- **Inline-path asymmetry.** "A phase-result commit always happens" holds for the
  `discuss → proceed` ceremony path, but the low-ceremony **inline** path (verified
  Claim 4: direct commit on the current branch, no ticket) produces **no**
  phase-result commit. So the signal covers ceremony work, not pure inline work —
  the same asymmetry already noted for the trigger surfaces; it does not get worse.

## Hard prerequisite

**② (`260824-feat-lead-review-range-scenario`) must land first.** Today
`lead-review` requires a checked-out branch and does not pass a range to
`git.diff` (`agents-plugin/rsrc/lead-review/lead-review.md:17,33,39`), so the
"agent runs the review for you" valve is **physically impossible** until ② wires
range review. The same key (②) unlocks both this persona relief and the
multi-maintainer sweep.

## Open questions

Carried into implementation (the 2026-08-30 re-discussion settled purpose (A),
severity calibration, the no-second-file mechanism, and the signal choice —
**(b) the explicit `Reviewed-slice:` trailer**; these remain open):

- **Does a landed phase-result commit imply ① *passed*? — now fallback-only.**
  With (b) chosen the trailer carries the real verdict, so this no longer gates the
  primary design; it matters only if implementation falls back to (a) footprint
  inference, where the ceremony≈pass approximation holds only if a phase result
  does not land while an ① concern is still open (verify against source then).
- The exact proposal phrasing and the threshold at which passive FYI becomes an
  active proposal (reuse ③'s size/staleness scale; where is the passive→propose
  cutoff, and how does the raw-vs-slice-covered split shift it?).
- The pure inline path (direct-on-current-branch, no phase-result commit) has
  neither an `impl/*` branch for the existing `tickets.close` nudge nor a
  phase-result footprint — how is its (rarer, ceremony-skipping) backlog surfaced,
  if at all?
- Whether "no-boundary + no-review-track" should additionally *quiet* the nudge
  (fallback if the active proposal is still too noisy) vs. always offering to run.
- How the agent-run relief review records its verdict/stamp in the ledger when the
  user has no notion of the ledger (fully automated append vs. surfaced).
- Interaction with the epic's "no MCP-mediated merge" decision — the relief review
  runs at a checkpoint, not by hooking the merge, so it should not reintroduce a
  merge primitive; confirm the composition stays checkpoint-driven.
