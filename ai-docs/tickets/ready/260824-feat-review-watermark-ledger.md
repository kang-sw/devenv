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
- **The marker is a line in the ledger file, read by content — not a commit
  found by walking the graph.** Lookup is line/field-scoped (parse the last
  entry); the marker is that entry's through-SHA, no separate field. Never derive
  it from "the last commit that modified the ledger" (the file changes for other
  reasons), and never by traversing merge parents.
- **Write discipline — only the review-track branch (master) writes the ledger,
  and only a review that actually lands stamps it.** Feature/impl branches never
  append. The stamp is the *final atomic step* of a review that lands on the
  review-track: either riding a master merge (the primary trigger, Phase 2) or a
  standalone catch-up sweep run on master. A branch under review never stamps
  before it wins its merge. Consequence: because master merges are serialized,
  two ledger appends never race — so the append-only ledger never merge-conflicts
  (a feature→master absorption brings no competing ledger line; master's version
  simply wins). This is the concurrency answer the earlier "per-track" note left
  open.
- **The review range is set-subtraction, not a parent-path walk.**
  `marker..HEAD` = `{reachable from HEAD} − {reachable from marker}`. Because the
  marker sits on the master-side parent of any merge, all already-reviewed history
  subtracts out in one shot; a two-parent merge commit creates no marker
  ambiguity. A merging branch thus reviews only its own delta re-contextualized on
  the current marker (plus the integration/merge commit itself); work another
  branch already stamped is excluded.
- **Skip-coverage invariant — the marker rises ONLY by an actual stamping
  review.** Everything in `(marker, HEAD]` is "not yet reviewed" regardless of how
  it landed — including a merge that skipped review entirely. The next stamping
  review (the next branch to merge, or ultimately the release gate ④) inescapably
  sweeps the whole gap; no skipped merge can masquerade as reviewed. This is what
  makes an *unforced* merge-time review safe: coverage is guaranteed, only the
  *timing* is at the lead's discretion, with the growing range + release gate as
  pressure and backstop.
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
  `<base>..<head>: verdict` entries — with a **routed-ticket-stem reference** on
  non-pass entries (the finding-resolution hand-off, Phase 2). The append surface
  **requires a stem on a `block` entry** on the happy path, so an un-routed block
  is normally unreachable; should a malformed un-routed block ever exist, it is
  routed by a **corrective append-only follow-up entry** (`<same range>: routed ->
  <stem>`) — never an in-place edit, so the append-only/never-edit rule and the
  "un-routed block is un-clearable" forcing function (④) do not deadlock. Marker =
  latest entry's through-SHA via line-scoped parse.
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
  `tickets.close` must not spawn a delegated review. Merges stay native (no
  MCP-mediated merge — rejected at the epic level); a merge that skipped its
  review is caught not by observing the merge but by the **skip-coverage
  invariant** — the marker did not advance, so the range resurfaces at the next
  stamping review or the gate. The nudge is the pressure that keeps that range
  from growing unnoticed.
- **Trigger — primary: review at master-merge time; fallback: standalone
  catch-up sweep.** The normal path couples the review to integration: before
  merging a branch to the review-track, **absorb current master into it**, run the
  `lead-review` **range-scenario** (②) over `marker..HEAD`, **fix fatal findings
  before the merge** and route the deferrable ones to tickets, then merge — and
  **stamp the ledger as the merge's final step** (see Write discipline). The
  stamped `<head>` is the **actually-reviewed SHA — the pre-merge reviewed tip**,
  not the post-merge tip: for an ff merge they coincide; for a no-ff merge the new
  merge commit's own conflict-resolution content stays *above* the marker and is
  swept next time (never stamp the post-merge tip as reviewed — that would record
  un-reviewed merge content as reviewed, breaking ledger honesty). If master moved
  under you (a racer stamped first), **re-absorb** — clean, because this branch
  holds no ledger line — and re-review the now-smaller delta before stamping. When merges have been skipping review, the lead runs the same range
  review as a **standalone catch-up sweep** directly on master over the accumulated
  `marker..HEAD` and stamps there. Either way the range review runs through ②; the
  ledger append is owned by this ticket's Phase-1 surface.
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
- **Finding hand-off — surface and route, do not fix inline (epic Cross-Child:
  finding resolution path).** The sweep's job ends at surfacing; a raised finding
  is routed to a ticket (new `idea/`/`todo/`) or the `lead-review` verdict-routing
  comm path, and the non-pass ledger entry *references the routed ticket stem*
  (e.g. `<base>..<head>: block -> 260901-bug-…`). The ledger stays append-only —
  no mutable "resolved" flag; the routed ticket's status is the resolution record.
  Mid-stream this is advisory (route and move on); the release gate (④) is where
  an open routed blocking ticket actually stops promotion.
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
checkpoint recompute/nudge, which never runs a review, must never append);
**skip-coverage** — a merge that landed on master without a review leaves the
marker unmoved, and the next stamping review's `marker..HEAD` range still contains
that merge's delta (nothing masquerades as reviewed); **serialized-marker /
no-conflict** — a feature→master absorption introduces no competing ledger line,
so the ledger never merge-conflicts, and after a race the re-absorbed branch's
`marker..HEAD` excludes the racer's already-stamped work (set-subtraction) while
still covering its own delta.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` (checkpoint tools `tickets.close`,
`workflow_manual`, `enter.*` gain the lazy sweep-nudge behavior) and
`ai-docs/spec/workflow-skills.md` (the sweep flow). New caller-visible behavior:
observed checkpoints recompute `marker..HEAD` and emit a size-scaled advisory
nudge; the ledger/marker artifact and its bootstrap surface are defined.
