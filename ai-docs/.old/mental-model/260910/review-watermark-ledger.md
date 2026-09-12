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
- `track.go`, `config.go`, `agents_config.go` — review-track branch
  resolution (`ResolveTrack` prefers the `AGENTS.md` `### Review Policy`
  `review-track` declaration over the git-default heuristic when set), the
  `_review.local.md` staleness knob read by `checkpoint.go`, and the
  fail-open `AGENTS.md` review-policy reader (`ReadAgentsReviewPolicy`).
  Caller-visible contract spec'd at
  `{#260830-review-policy-config-surface}`.
- Ticket `260824-feat-review-watermark-ledger`, `## Decisions` →
  `### Multi-maintainer constraints` — design authority for the canary,
  banner, and landing-topology reasoning below. This reasoning is
  deliberately *not* restated in the spec (which documents caller-visible
  tool behavior only), so it lives here.

## Module Contracts

- **Frontier ≠ raw latest entry.** `ParseFrontier`/`Frontier` are a new,
  separate read-side function pair from `ParseLatest`/`Read` (Phase 2,
  `260824-feat-review-release-gate-policy`) — the frontier is the last
  *clearing* entry (`pass`, `concern`, or the `bootstrap` floor), skipping any
  trailing `block`/`routed` entry rather than reporting whatever line is
  physically last. `ParseLatest`/`Read`/`Bootstrap` are unchanged and keep
  their raw-latest-regardless-of-verdict semantics; `Bootstrap`'s own
  "does an entry exist at all" idempotency check deliberately still uses
  `Read`, not `Frontier` — that's a different question ("has this ledger ever
  been seeded") from "what's the resumption point". `CheckpointNudge` and the
  `review.marker` MCP tool both consume `Frontier`, so a stamped `block` (or
  its `routed` corrective follow-up) holds both the checkpoint nudge and the
  marker read at whatever clearing entry preceded it, until a later sweep
  clears the range with `pass` or `concern`. This is also what decouples the
  release gate from block-entry bookkeeping: the gate only ever sees a
  resolvable clearing head, never has to special-case a `block` tail itself.
- **Single-writer invariant.** `review.stamp`, called only from
  `lead-review`'s range-scenario step 7, is the ledger's sole writer. No other
  skill or gate — including `lead-ship`'s release gate — ever calls
  `review.stamp`; a gate override proceeds without touching the marker, so
  the frontier only ever advances through an actual completed review sweep.
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
  purely from ledger content (the frontier — the last *clearing* parseable
  entry, not necessarily the last parseable entry outright; see Module
  Contracts), never by graph-walking. If a squash/rebase landing on the
  review-track rewrites the reviewed SHA (forbidden by convention, not
  mechanically blocked in plain git), the marker does not corrupt — it simply
  stops advancing past the orphaned span, and the next sweep's range
  naturally re-covers it. The guarantee is fail-safe **over-review**, never
  fail-safe prevention; don't assume the system needs enforcement to stay
  correct.

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
