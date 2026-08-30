---
domain: review-watermark-ledger
description: "Append-only review-verdict ledger, marker resolution, checkpoint nudge, and the multi-maintainer canary/landing-topology design reasoning behind them."
sources:
  - agents-plugin-tool/internal/wsreview/
related:
  workflow-skills: "lead-review's range scenario appends the final review.stamp step that grows the ledger."
---

# Review Watermark Ledger

## Entry Points

- `ledger.go` — format/parse/append/bootstrap primitives, plus `ledgerBanner`.
  Caller-visible contract spec'd at `{#260830-review-watermark-ledger-tools}`.
- `checkpoint.go` — cheap, read-only staleness nudge. Spec'd at
  `{#260830-review-watermark-checkpoint-nudge}`.
- `track.go`, `config.go` — review-track branch resolution and the
  `_review.local.md` staleness knob read by `checkpoint.go`.
- Ticket `260824-feat-review-watermark-ledger`, `## Decisions` →
  `### Multi-maintainer constraints` — design authority for the canary,
  banner, and landing-topology reasoning below. This reasoning is
  deliberately *not* restated in the spec (which documents caller-visible
  tool behavior only), so it lives here.

## Module Contracts

- A ledger tail conflict is a **designed STOP, not corruption**: the
  write-discipline convention (only the review-track branch stamps, and
  only via append) means two branches that each stamp independently,
  without one absorbing the other first, necessarily produce an
  unmergeable git conflict on the file's tail — this is the intended
  multi-maintainer canary, not a bug to route around. The banner cannot
  mechanically force the resolver to re-review; that gap is physically
  irreducible in plain git. The banner only converts the mechanical STOP
  into a **prompted THINK** at the conflict site — a soft mitigation, not
  an enforcement mechanism.

## Extension Points & Change Recipes

- **Land a review-track merge under multi-maintainer constraints**: ff or
  merge-commit only — squash/rebase are forbidden by convention because
  they rewrite the reviewed commit's SHA and orphan the marker. Topology:
  Merge-1 (`HEAD <- master`) absorbs master and resolves conflicts inside
  the reviewed range, producing reviewed commit **R**; the ledger append
  then lands as its own commit **L** (parent R); Merge-2
  (`master <- HEAD`) lands R SHA-preservingly. The `review.stamp` call's
  `head` must be **R, never L** — this is what resolves the chicken-and-egg
  of an entry that would otherwise have to point at its own bookkeeping
  commit. Platform-config hardening (disabling squash/rebase) is out of
  this domain's scope, owned by `260824-feat-review-release-gate-policy`.

## Common Mistakes

- Resolving a ledger tail conflict with `checkout --ours`/`--theirs` — this
  silently drops one branch's reviewed range. Resolution must integrate
  both branches' reviewed work, never pick a side.
- Assuming the banner is visible at the conflict site on a **mature**
  ledger — it is written exactly once, top-of-file, at the file's first
  physical creation (inside `Append`'s single `O_CREATE|O_WRONLY` open).
  It lands *inside* the actual conflict markers only for the
  concurrent-first-creation case (an add/add conflict, where neither
  branch's common ancestor has a ledger yet, so top-of-file and the tail
  insertion point coincide). On a ledger that already has entries, a later
  tail conflict sits well below the banner and outside the conflicted diff
  region — see Technical Debt.
- Recording the bookkeeping commit **L** instead of the reviewed commit
  **R** as a `review.stamp` entry's `head` under the landing topology
  above. This is a convention violation, not a mechanical one: it only
  causes **over-review** (the next sweep re-covers L's redundant range),
  never under-review, but it is still a wrong outcome to avoid.
- Treating a squash-orphaned marker as data loss. The marker resolves
  purely from ledger content (the last parseable entry), never by
  graph-walking. If a squash/rebase landing on the review-track rewrites
  the reviewed SHA (forbidden by convention, not mechanically blocked in
  plain git), the marker does not corrupt — it simply stops advancing past
  the orphaned span, and the next sweep's range naturally re-covers it.
  The guarantee is fail-safe **over-review**, never fail-safe prevention;
  don't assume the system needs enforcement to stay correct.

## Technical Debt

- **Banner known limitation**: the banner is emitted once, top-of-file,
  only at first physical file creation — never re-emitted per entry. This
  means it stays inside the conflict markers only for the
  concurrent-first-creation (add/add) case; a tail conflict on a
  many-entry ledger lands far below it. The richer alternative — a short
  comment re-emitted immediately before *every* entry, so banner text
  stays inside the conflict markers regardless of ledger maturity — was
  evaluated and deliberately **deferred** as optional future hardening
  (Phase 3 Lead Adjudication, `260824-feat-review-watermark-ledger`), not
  silently dropped.
