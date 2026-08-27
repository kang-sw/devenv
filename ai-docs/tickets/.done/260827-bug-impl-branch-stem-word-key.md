---
title: Deterministic word-key impl-branch stem — repair the never-matching continue path
related:
  260825-feat-impl-branch-single-ticket-scope-merge-timing: substrate — established the single-ticket-scope invariant and the L0 continue / L1 stop layering this ticket repairs; its Phase 2 no-merge default is what turns the L0 miss into an always-stop
  260626-feat-session-key-format-and-retention: substrate — owns the EFF wordlist generator (internal/wskey) this ticket adds a deterministic derivation to
related-mental-model:
sage-review-design: completed
sage-review-completeness: completed
---

# Deterministic word-key impl-branch stem — repair the never-matching continue path

## Background

Ticket `260825` established the invariant that an `impl/*` branch belongs to
exactly one ticket: phases accumulate on the one branch, and re-entering
`enter.implement` for the same ticket resolves to `continue` (L0:
branch-name stem == caller scope), while entering for a *different* ticket with
unmerged work resolves to a safety `stop` (L1). In practice the **L0 `continue`
path has never matched** — same-ticket `proceed`-spam is refused at the
implement step.

Root cause: the impl-branch stem is produced from **agent-supplied free text**,
not from ticket identity. `proceed` does not own the stem — its target carries
only `kind`/`label`/`ticket_stem`/`ticket_path` and it merely routes to
`lead-implement` (`proceed_resolver.go`). The agent then fills `target.scope_slug`
on the `enter.implement` call, and the resolver derives the branch stem from it
(`implement_resolver.go`: when `scope_slug` is empty it falls back to
`slugifyImplementScope(firstNonEmpty(ScopeLabel, TicketStem, Label, …))` — note
it prefers the agent's `ScopeLabel` **over** `ticket_stem`). Agents habitually
encode the phase into that label (`…-p1`, `…-p2`), so every phase produces a
different stem: `impl/<root>/<ticket>-p1` then target `impl/<root>/<ticket>-p2`
→ `currentBranch != targetBranch` → L0 never matches.

This composes with `260825` Phase 2 (per-phase final action defaults to no
merge → the branch always has commits ahead of its merge root, so
`AheadOfMergeRoot > 0` always holds). So the L0 miss falls straight through to
the L1 safety block: **same-ticket `proceed`-spam is stopped at every phase after
the first.** Both `260825` phases are correct in isolation; the stem-hygiene
defect composes them into an always-stop.

## Decisions

- **Fix at the resolver, not by convention.** The stem must stop being a function
  of agent free text. When a ticket target is present, derive the impl-branch
  stem **deterministically from ticket identity** so it is identical across every
  phase of one ticket. `finishImplementBranchPlanTail`'s
  `currentBranch == targetBranch` comparison and the L1 safety stop stay
  **unchanged** — the entire fix lives in *how the stem is produced*.
  - **Rejected: convention-only fix** (skill text pinning `scope_slug` to the
    ticket stem / forbidding phase suffixes) as the *primary* remedy. It depends
    on agent compliance, and the existence of this bug is proof that bet loses.
    Phase-suffix-forbidding guidance may still ship as *secondary* reinforcement,
    never as the mechanism.
- **Stem = deterministic word-key from a hash of the seed.** Reuse the existing
  EFF large wordlist in `internal/wskey`; add an **additive deterministic
  derivation** (hash the seed → index into the pool), distinct from the existing
  random `Generate()`. Output is **3 words** joined by hyphens.
- **Constrain to the `<=5`-char sub-pool.** Of 7772 wordlist entries, 1476 are
  `<=5` chars. Using only those gives a worst-case stem length of `5+1+5+1+5 =
  17` chars (meets the `<=18` target; the resolver's own `<=15` recommendation is
  a soft guide) and a 3-word space of `1476^3 ≈ 3.2e9`.
  - **2 words is prohibited under the char cap.** `<=5`-char × 2 words = `2.18e6`
    space; birthday over ~1000 tickets ≈ 23% — unacceptable for the *expensive*
    failure mode (see collision analysis). Constraining length reduces the word
    count only; **never** the word count below 3.
  - **`<=4`-char (549 words, `1.65e8` space, worst case 14 chars) is a recorded
    fallback, not the choice.** It hits the literal `<=15` recommendation but with
    a single-digit-hundred-million margin; `<=5`/3-word is chosen for the far
    larger margin at trivial length cost.
- **Word-key derivation applies to ticket targets only; inline targets keep the
  current slug.** The deterministic derivation uses `seed = target.ticket_stem`.
  Inline targets (no `ticket_stem`) retain the existing
  `slugifyImplementScope`-derived, human-readable slug **unchanged** — no word-key.
  Rationale: inline has no stable identity to hash (its label is agent free-text,
  so hashing buys no determinism, only opacity), the bug is ticket-specific, and
  low-ceremony inline uses the `current` action (no impl branch) anyway. So inline
  branch naming is exactly as today; only ticket-targeted stems become word-keys.
- **Collision analysis (why `<=5`/3-word is safe).** The harmful event is a
  *false negative*: two different tickets hash to the same key, L0 falsely
  `continue`s, and two tickets' work mixes onto one branch (the exact cost
  `260825` biased against). It requires three things at once: key collision **and**
  sitting on the colliding branch **and** a genuinely different ticket. Real-world
  risk ≈ concurrent branches per merge root (~10) ÷ space = `~3e-9` at `<=5`/3-word.
  The pessimistic lifetime-birthday bound is ~1.6% over 10k tickets, and even that
  is gated by the sit-on-the-branch conditional. Safe.
- **Accepted trade-off: opaque branch names.** `impl/develop/amber-tide-fox` does
  not tell a human which ticket it is. Mitigation: the resolver's stop and
  `tickets.close` merge-review messages already carry `target.ticket_stem` /
  `SuspectedOwnerStem`, which can display the human ticket alongside the key.
  Branches are workflow-owned and short-lived; the cost is accepted.
- **Rejected: pattern-stripping phase suffixes.** Detecting and removing `-pN`
  from the label is brittle and re-introduces the fragile convention dependency
  `260825` already rejected. Derive from identity instead of repairing free text.

## Constraints

- Do not alter `finishImplementBranchPlanTail`'s continue/stop comparison logic or
  the L1 `AheadOfMergeRoot` safety block; change only stem production.
- No commit-content parsing (preserve the `260825` SAFETY/IDENTITY boundary).
- Preserve the `continue` / `create` / `current` / `rename` verdict actions and
  the `impl/<merge-root>/<stem>` name-encoding; only the `<stem>` derivation
  changes.
- Host-neutral: no `develop`/`main` topology hard-coding.
- The deterministic derivation must not disturb the random `Generate()` used for
  session keys.

## Phases

### Phase 1: Deterministic word-key stem derivation for ticket-targeted impl branches

Goal: entering `enter.implement` for the same ticket across successive phases
produces the **same** impl-branch stem, so the L0 `continue` path matches and
`proceed`-spam accumulates on the one branch; a different ticket still reaches
the L1 stop.

Approach:
- Add an additive deterministic derivation to `internal/wskey` (e.g.
  `Derive(seed string, words int) string` over a `<=5`-char sub-pool, or a
  variant that accepts a max word length), hashing the seed and indexing the
  embedded pool — reusing the existing wordlist, leaving `Generate()` untouched.
- In `implement_resolver.go`, when the target carries a `ticket_stem`, produce the
  impl-branch stem from `wskey.Derive(target.ticket_stem, 3)`. For a ticket target
  the derivation is **authoritative** — it must win over a caller-supplied
  `target.scope_slug` too, not only the empty-`scope_slug` fallback at the current
  `slugifyImplementScope(ScopeLabel|…)` line; otherwise an agent that sets
  `scope_slug` directly re-introduces the per-phase drift this ticket removes.
- Leave the inline-target path (no `ticket_stem`) exactly as today: the existing
  `slugifyImplementScope` label-derived slug, no word-key.
- Optionally add secondary guidance (skill/convention text) that impl-branch
  stems are identity-derived and phase suffixes never belong in a branch name.

Verification (resolver + wskey unit tests):
- Same `ticket_stem`, two successive phases → identical `targetBranch` → L0
  `continue` (the regression the dead path needs; assert it directly).
- Different `ticket_stem` on a branch with unmerged work → L1 `stop` unchanged.
- Determinism: same seed → same key across calls; different seeds → different
  keys.
- Length bound: derived stem `<=17` chars; sub-pool word count assertion (1476
  words at `<=5` chars) to pin the collision space.
- Inline target (no `ticket_stem`) → the existing `slugifyImplementScope` slug
  path is used unchanged (no word-key); assert the inline stem is not a word-key.
- `Generate()` (random session key) behavior unchanged.

### Result (d32d091c) - 2026-08-27

`wskey.Derive(seed, words)` added over a build-time `shortWordPool` (the 1476
`<=5`-char words of the 7772-entry EFF pool), hashing the seed and indexing the
sub-pool — additive alongside the random `Generate()`, which is untouched. In
`normalizeImplementFacts`, a non-empty `target.ticket_stem` now unconditionally
sets the branch stem to `wskey.Derive(ticket_stem, 3)`, **overriding** any
caller-supplied `target.scope_slug` (with an ignored-scope_slug warning). Gated
on `TicketStem != ""` (not `Kind == "ticket"`) because existing callers set
`Kind: "ticket"` without a stem and rely on the slug path. Inline targets keep
the existing `slugifyImplementScope` path. `finishImplementBranchPlanTail`'s
`currentBranch == targetBranch` comparison and the L1 `AheadOfMergeRoot` stop are
untouched — same ticket across phases now yields the same stem (L0 `continue`),
different ticket still stops.

- Verification: `go build ./...`, `go vet ./...`, `go test ./internal/wskey/...
  ./internal/mcp/...` all pass (full output read). Added 4 wskey tests
  (determinism, distinct-seeds, length bound, sub-pool count = 1476) and 4
  resolver tests (same-stem continue, different-stem stop, scope_slug override,
  inline unaffected).
- Deviation: the survey's test-impact scan covered only
  `implement_resolver_test.go`; `session_state_test.go`'s shared
  `implementReadyArgs` fixture also sets `ticket_stem`+`scope_slug`, so 3
  integration assertions there were updated to the derived stem `jot-pug-mossy`
  (intended behavior surfacing, not scope creep).
- Review: partitioned correctness (opus) + fit (sonnet), both `clean`, no relays.
- Commits: 1a0c115a (survey plan), d32d091c (code + tests), a5f36572 (spec sync,
  anchor `{#260827-ticket-stem-word-key-branch}`).

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` — the `enter.implement` branch-plan prose
that describes the `impl/<merge-root>/<stem>` name-encoding. Expected
caller-visible change: for ticket-targeted entries the `<stem>` is now a
deterministic word-key derived from ticket identity (stable across phases), not a
caller-supplied scope slug; inline targets retain label-derived slugs. Addressed
at `ready/` promotion.
