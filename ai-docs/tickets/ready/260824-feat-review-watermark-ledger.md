---
title: Review watermark marker + dotfile ledger + advisory sweep with lazy checkpoint recompute
parent: 260824-epic-review-watermark-model
related:
  260824-feat-lead-review-range-scenario: prerequisite — the sweep reviews marker..HEAD through the range scenario
  260726-feat-enter-verdict-scenario-output: adjacent — enter.* output/agenda surface where checkpoint nudges may render
sage-review-design: completed
sage-review-completeness: completed
---

# Review watermark marker + dotfile ledger + advisory sweep with lazy checkpoint recompute

## Background

The epic keys review to the cumulative unreviewed trunk diff, which needs a
topology-independent "reviewed-up-to" marker. No such concept exists in the repo
today. This ticket introduces the marker, its storage, and the advisory sweep
that reviews `marker..HEAD` and advances the marker. It depends on the range
scenario (②).

Circled numbers denote the epic's sibling children: ② =
`260824-feat-lead-review-range-scenario`, ③ = this ticket
(`260824-feat-review-watermark-ledger`), ④ =
`260824-feat-review-release-gate-policy` (see `related:`).

## Decisions

Settled at the epic level; restated here as implementation constraints:

- **Storage:** a single tracked dotfile ledger under `ai-docs/` (per-track — it
  travels with the branch tree, so different tracks never collide); proposed
  path `ai-docs/.review-ledger.md`, final name an implementation choice. Each
  review appends `<base>..<head>: verdict`. Not `AGENTS.md` (a fast-advancing SHA would
  churn the tracked orientation doc), not a git ref, not the note layer. The
  ledger is the single home for both verdict history and the marker.
- **Marker = the latest ledger entry's through-SHA** — no separate field.
- **Lookup is line/field-scoped** (parse the last entry). Never derive the
  marker from "the last commit that modified the ledger" (the file changes for
  other reasons).
- **Range key is a commit SHA, never a wall-clock timestamp.** A human timestamp
  may ride an entry as metadata but must not be the range key.
- **Bootstrap is explicit, never silent.** When no marker exists, insert one at
  current `HEAD` and surface it: prior history is *review-skipped*, not
  *reviewed*. This prevents false confidence when a later sweep reports clean.
- **Advisory only — never blocks.** Sweep aggressiveness scales to
  unreviewed-range size/staleness (small range → quiet FYI; large accumulated
  range → strong nudge). The size threshold is the pressure-relief valve; the
  release gate (④) is the hard backstop.
- **Ledger honesty:** an entry recorded as reviewed without an actual review is
  false confidence. Verification must guard against recording unreviewed ranges
  as reviewed.

## Phases

### Phase 1: Ledger format + marker read/append + explicit bootstrap

- Define the dotfile ledger format under `ai-docs/` and the append/read of
  `<base>..<head>: verdict` entries; marker = latest entry's through-SHA via
  line-scoped parse.
- Explicit bootstrap when absent: insert at `HEAD`, surface the
  review-skipped-not-reviewed meaning.

Verification: append/read round-trips; marker resolves to the last entry's
through-SHA under line-scoped parse even when the ledger file was touched by an
unrelated edit; bootstrap on an empty ledger emits the explicit surface.

### Phase 2: Checkpoint nudge (cheap) + separately-invoked sweep

**Two distinct mechanisms — do not conflate them.** A checkpoint never
auto-runs a review; it only recomputes and nudges. The sweep is a separate,
explicitly-invoked action.

- **Checkpoint recompute + nudge (cheap, no review spawn).** At observed MCP
  checkpoints — `tickets.close` (post-merge by construction — the merge lands
  before the ticket closes), `workflow_manual` at session start, and the
  `enter.*` router entrypoints (`enter_implement`, `enter_proceed`) as backstops
  — recompute the *size* of `marker..HEAD` on the review-track branch
  and emit a proportional **advisory** nudge. This stays atomic/cheap:
  `tickets.close` must not spawn a delegated review. Merges stay native and
  unobserved; a `post-merge` tool / MCP-mediated merge was rejected at the epic
  level as redundant given this lazy recompute.
- **Sweep (separately invoked).** The sweep is a `lead-review` **range-scenario**
  (②) run over `marker..HEAD`, invoked by the lead (prompted by the nudge), not
  auto-triggered by a checkpoint. Assign this actor explicitly: the range review
  runs through ②, and the ledger append is owned by this ticket's Phase-1
  surface.
- **Marker advances on every completed sweep, regardless of verdict — the
  marker is "reviewed-up-to," never "reviewed-clean."** A completed sweep always
  appends the Phase-1 ledger entry (`<base>..<head>: <verdict>`) recording its
  **actual** verdict (`pass`/`concern`/`block`), which advances the marker. The
  marker does **not** stay put on a non-passing verdict, and the ledger is **not**
  a retry gate: it is the verdict *history* (the epic's "single home for both
  verdict history and the marker"), so a range reviewed once is not re-reviewed by
  a later, ever-growing sweep just because it carried findings. A non-passing
  verdict is recorded as an **open finding** on that range; it is the **release
  gate (④)** that blocks promotion on an unresolved recorded blocking finding —
  not the marker. Ledger honesty is preserved because the entry records the true
  verdict, not a false "reviewed-clean"; the guard is against recording an
  *un-reviewed* range as reviewed, not against recording a reviewed-but-failing
  range.
- **Nudge scaling metrics (both SHA/commit-based, per the epic's
  SHA-not-timestamp decision):** *size* = commit count across `marker..HEAD`
  (reuse the Deep Review / is-large-diff threshold); *staleness* = commit
  distance since the marker (not wall-clock — the timestamp is optional
  metadata, not the range key). Unlike *size*, *staleness* has no existing
  constant to reuse: implementation defines its quiet-FYI→strong-nudge cutoff as
  a `_review.local.md` config knob with a modest default, not a hardcoded
  threshold. Advisory only.
- Depends on Phase 1 and ②.

Verification: after a native merge advances the trunk past the marker, the next
`tickets.close`/session-start recompute surfaces a proportional nudge **without
spawning a review**; a just-swept trunk stays quiet; ordering-independence — a
checkpoint reached without a preceding close still catches the range; a
separately-invoked sweep runs the range scenario and its ledger append advances
the marker; **ledger-honesty guard** — an append is reachable only from a
completed sweep, so no code path records an un-reviewed range as reviewed (a
checkpoint recompute/nudge, which never runs a review, must never append).

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` (checkpoint tools `tickets.close`,
`workflow_manual`, `enter.*` gain the lazy sweep-nudge behavior) and
`ai-docs/spec/workflow-skills.md` (the sweep flow). New caller-visible behavior:
observed checkpoints recompute `marker..HEAD` and emit a size-scaled advisory
nudge; the ledger/marker artifact and its bootstrap surface are defined.
