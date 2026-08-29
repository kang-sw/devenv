---
title: Multi-maintainer review-watermark model — coverage-as-mechanical invariant, ledger canary, host-split enforcement
related:
  260824-epic-review-watermark-model: proposes overturning the epic's lead-review/marker structure toward an explicitly multi-maintainer, host-split enforcement model
  260824-feat-review-watermark-ledger: revises this child's marker/ledger decisions (③); its sage-completed "stamp pre-merge tip / ledger is single home" lines are the ones this research reconsiders
  260824-feat-lead-review-range-scenario: this child (②, range scenario + landing lens) is established as independent and preserved unchanged by this research
  260824-feat-review-release-gate-policy: revises this child (④) to key the hard gate off stable release tags and ticket status rather than a precise ledger marker
  260729-research-implement-router-prose-only-dimension: motivating "default review pass too heavy" incident that seeded the parent epic
---

# Multi-maintainer review-watermark model — coverage-as-mechanical invariant, ledger canary, host-split enforcement

## Background

The parent epic `260824-epic-review-watermark-model` designed a time-series
watermark review model for **messy, low-discipline "practice" projects**: review
keyed to the cumulative unreviewed trunk diff (`marker..HEAD`), so that a commit
landed on trunk *any* way is eventually swept regardless of how it landed. Child
① (`260824-feat-per-phase-review-floor`) already landed in `.done/`; children
②③④ remain in `todo/`. The epic's marker/ledger mechanics (③) and finding
resolution (④) passed sage design + completeness review.

A discuss session (2026-08-29) stress-tested the model against a target the epic
did not design for on its own axis: **an open-source project with multiple
contributors AND multiple maintainers**, where merges land through platform UIs
(GitHub buttons) outside any ws session, and concurrent maintainers integrate in
parallel. The epic's "host-neutral" claim covers harness (Claude/Codex) and
branch topology, **not** collaboration concurrency — that axis was effectively
unaddressed. This ticket captures the session's converged direction so a fresh
session can re-open it. **It documents a proposed overturn of ③'s (and part of
④'s) sage-completed decisions; nothing here is implemented and the direction is
not yet accepted for implementation.**

The user's decisive re-anchor mid-session: the marker exists to cover **"brute"
commits just thrown onto master** (raw pushes, review-skipping merges, direct
commits). Several elegant-looking simplifications were rejected precisely because
they weakened that purpose.

## Converged model (working conclusions, user-endorsed this session)

The model decomposes into **one mandatory-mechanical safety invariant plus
efficiency layers**, and the mechanical-vs-convention line is drawn by *failure
mode*: a safety invariant must be mechanical; an efficiency optimization may be a
convention when violating it only wastes review effort (never opens a coverage
hole).

Four pieces:

1. **Commit-SHA marker (epic original, retained).** The marker is the
   through-SHA (a **commit SHA**, not a tree/snapshot hash, not a wall-clock
   time) recorded as a line in a tracked dotfile ledger under `ai-docs/`, read by
   line-scoped parse. It advances **only** by an actual stamping review.
2. **Skip-coverage (epic original, retained) — the sole mandatory-mechanical
   safety invariant, and already structurally mechanical.** A brute commit does
   not stamp, so the marker does not move, so the brute commit sits in
   `(marker, HEAD]` and is inescapably swept by the next stamping review or the
   release gate. This needs no config and no convention — it holds by
   construction regardless of who pushes what, or how it lands.
3. **Ledger canary (new) — mechanical rendezvous for concurrent wsflow
   landings.** Because every wsflow landing appends to the ledger's tail, two
   branches that both stamp without one absorbing the other **necessarily**
   produce a textual merge conflict on the ledger (git cannot auto-merge two
   different insertions at the same anchor). This forces a STOP — the later
   branch cannot merge until it resolves the conflict, which means integrating
   the other's work. Works identically on GitHub (its merge/squash/rebase all run
   git merge and report the conflict), with **no branch-protection config
   required**. It fires precisely when someone skipped the absorb.
4. **No-squash on the review-track (efficiency convention).** Squash/rebase
   landings rewrite the reviewed commit to a new SHA, orphaning the marker.
   Constraining review-track landings to ff or merge-commit preserves the SHA.

Role split: marker + skip-coverage cover brute / non-wsflow landings; the canary
covers wsflow-vs-wsflow concurrency; no-squash preserves marker precision.

### Landing topology (two distinct merges)

- **Merge 1 `HEAD <- master` (integration merge).** On the branch: absorb master,
  resolve conflicts here, **review the resolved commit R**, then append the ledger
  entry as a **separate final commit L** (parent R). Conflict resolution therefore
  lives inside the reviewed range.
- **Merge 2 `master <- HEAD` (landing).** Requires only **SHA preservation** of R.
  Locally, **ff is preferred** (Merge 1 already carries the integration merge;
  forcing no-ff there only adds a redundant empty merge commit). On GitHub, the
  SHA-preserving option is **"Create a merge commit" (no-ff)**. "Always force
  no-ff regardless of ff-ability" is an over-constraint and is rejected.
- **Ledger entry `<head>` = the reviewed commit R, never L** (L only carries the
  bookkeeping line) and never a random hash. This resolves the chicken-egg (the
  entry cannot point at its own edit commit).

### Mechanical-vs-convention taxonomy (the session's organizing result)

| Property | Must be mechanical? | Enforcement |
|---|---|---|
| Coverage of brute commits (safety) | Yes | Marker advances only on stamp → mechanical by construction; release gate is the backstop |
| Rendezvous: concurrent branches re-integrate + re-review (efficiency: catch semantic conflicts early) | Nice-to-have | GitHub: branch protection (require-up-to-date + dismiss-stale-approvals + required-checks; merge queue at scale) — enforces STOP *and* re-review. Any-git: canary — enforces STOP only, re-review is convention |
| No-squash on review-track (efficiency) | No — violation only causes over-review (safe) | GitHub: disable squash/rebase in repo settings. Else: convention |

Key insight: violating no-squash, or resolving a canary conflict without actually
re-reviewing, can only cause **over-review (wasted effort), never under-review (a
missed brute commit)** — proof: an orphaned/ignored marker still leaves brute
commits inside `(marker, HEAD]`. So only coverage must be airtight, and it
already is.

### Two enforcement backends for rendezvous

- **Platform-enforced (GitHub, preferred where available).** `require branches
  up to date before merging` (forces the absorb for *all* PRs, not just
  ledger-touching ones), `dismiss stale approvals on new commits` (forces human
  re-review after the absorb — closes the canary's "STOP but not THINK" gap),
  `require status checks (up to date)` (forces CI/side-effect checks to re-run on
  the integrated state), and disabling squash/rebase. At scale, a **merge queue**
  is the mechanical serialize-integrate-recheck primitive.
- **Platform-independent (any git, no config).** The ledger canary. Mechanically
  forces the STOP; the actual re-review is convention, optionally hardened by
  requiring the conflict resolution to append a fresh re-review verdict entry
  (CI-validatable).

### Release gate (④), squash-robust re-shaping

The hard mandatory gate keys off **stable release tags** (`<last-release-tag>..HEAD`)
rather than a precise ledger marker — tags are not orphaned by squash/rebase, so
the gate is squash-robust by construction, and it sweeps everything since the last
release regardless of how it landed (preserving skip-coverage at release
granularity). Findings are tracked by **routed ticket status** (`.done`/`.dropped`
= terminal/cleared; open = blocks), consistent with the epic's existing
"ticket-system-resolves / no separate waiver artifact / un-routed block is
un-clearable" decisions. The precise SHA marker is thereby demoted to the
**advisory mid-stream layer** (nudge sizing), where squash imprecision is
harmless.

## Rejected alternatives (with reasons)

- **Ledger-free / "master = reviewed frontier."** Requires the enforced invariant
  "everything on master is reviewed" — which is exactly the world that needs the
  model *least* (branch protection already gives full coverage there). The epic
  targets the world that *cannot* enforce it, where "on master" ≠ "reviewed."
  Decisive counter (user): with mixed wsflow / non-wsflow landings you cannot
  distinguish reviewed from brute-landed on master **without** a marker. The
  marker is irreplaceable.
- **Tree-hash / snapshot-hash marker (squash-robust content pointer).** Real and
  squash-robust, but: non-unique (multiple commits share a tree — revert/reapply,
  empty-diff merge, transient add/remove), needs a resolve-to-commit search
  (O(history)) before a range can be computed, and is not reliably preserved by
  squash when the branch was not up-to-date. Once no-squash neutralizes the squash
  problem, tree-hash is pure added cost. Rejected in favor of the commit SHA.
- **"Marker = ledger's last-edit time / last commit that touched the ledger."**
  The epic already rejected the graph-walk form; squash does not rescue it. Fails
  because not every ledger edit is a review stamp (corrective routing appends,
  format fixes, `checkout --theirs` conflict resolutions) → a non-review edit can
  advance the marker past genuinely unreviewed work = **under-review hole**. Literal
  wall-clock time is additionally unreliable for git ranges (rebase/rewrite/clock
  skew) — the epic's settled "SHA-not-timestamp" rule.
- **ff-land as a universal rule / "always force no-ff regardless of ff-ability."**
  Both are over-constraints; the real invariant is SHA preservation (forbid
  squash/rebase), which ff and no-ff both satisfy. See landing topology above.
- **MCP-mediated / CI-only stamp automation as the primary answer to the
  GitHub-merge gap.** Superseded by the canary, which needs no platform
  integration and is self-enforcing via git's own conflict detection. (CI-assisted
  stamping remains a possible hardening for the re-review-entry check, not the load
  bearer.)

## Honest residual limits

- **Canary forces STOP, not THINK.** Pure git can force conflict resolution but
  cannot force an actual semantic re-review; a resolver may `checkout --theirs`
  the ledger and proceed. Mitigations: require the resolution to append a fresh
  verdict entry (CI-validated), and/or rely on the GitHub backend (dismiss-stale
  approvals) where available.
- **Squash still causes over-review** if the review-track's own landing squashes
  despite the convention — safe (never under-review), but wasteful. No-squash is
  the (soft) cure.

## Scope boundary vs. the epic

- **② (`260824-feat-lead-review-range-scenario`) is unaffected** and remains
  independently landable: range/watermark diff selection + landing lens
  (convention adherence + spec/mental-model completeness, scoped to the
  range/own-integrated-work scenario, excluding external contributor PRs) stay as
  designed. This is the natural first implementation target regardless of how this
  research resolves.
- **③/④ are where the overturn lands**, and only partially: ③'s commit-SHA
  marker + skip-coverage are *retained*; the additions are the **canary
  rendezvous mechanism** and the **no-squash / landing-topology constraint**, and
  ④'s gate is **re-keyed to release tags + ticket status**. If accepted, this is
  an augmentation-plus-partial-rewrite of ③/④, not a from-scratch redesign.

## Open questions for the clean-session re-discussion

- Exact ledger file format and canary mechanics: separate `L` bookkeeping commit,
  whether conflict resolution must append a re-review verdict entry, and whether
  CI validates that entry's range.
- Whether the append-only single-file ledger needs a concurrency-safe shape
  (one-entry-per-file vs. tail-append) beyond what master-serialization already
  guarantees.
- The concrete recommended GitHub branch-protection / merge-queue configuration
  set, and how ws surfaces it as opt-in policy (`AGENTS.md` review-track/boundary
  fields already planned in ④).
- Whether devenv itself adopts the multi-maintainer machinery or stays on the
  simpler single-maintainer path (devenv never uses `git log --first-parent`
  archaeology, so the ff-land first-parent cost is moot for it).
- Precisely how much of ③/④ is rewritten vs. augmented, and whether the epic's
  Cross-Child Decisions need a formal `#### Edition`-style revision once accepted.
