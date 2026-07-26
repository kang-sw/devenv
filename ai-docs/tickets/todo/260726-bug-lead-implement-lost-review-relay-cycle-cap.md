---
title: the review-relay cycle cap is live in spec but absent from lead-implement and
  its generated todos, so a fix loop has no budget and no adjudication step
related:
  260619-refactor-stateless-implement-review-continuity: its D6 records the cap as
    "the existing hard cap" and lists lead-implement.md as updated with the
    convergence invariants; only the dedup half landed
  260503-feat-agents-plugin-write-code-port: records "Review relay remains capped at
    three cycles" for the skill lead-implement succeeded
  260525-bug-implement-review-fix-owner: its Phase 1 constraint "re-review and cycle
    caps stay equivalent" is false against the current playbook; corrected in the
    same logical change as this ticket
  260714-feat-playbook-tier-model-render-vars: shipped the render-resolved tier-model
    vars Phase 2's capacity elevation uses instead of hardcoded model names
spec:
  - 260612-reviewer-allocation-tier-default
  - 260619-stateless-implement-review-continuity
sage-review-design: blocked
sage-review-completeness: required
---

# The review-relay cycle cap is live in spec but absent from the playbook

## Background

Found by dogfooding `ws:lead-implement` on Phase 1 of
`260726-refactor-ws-dashboard-git-fs-watch-invalidation` (2026-07-26). The lead
ran three review-relay cycles and was about to dispatch a fourth; the user asked
whether a hard cap existed. It does — but not anywhere the executing lead can
see it.

**This is a spec violation, not a lost rule.** `ai-docs/spec/workflow-skills.md`
still carries the contract verbatim, unchanged since it was written:

> Relay cap is 2 cycles for single-reviewer, 3 cycles for partitioned with lead
> adjudication at cycle 2 and caller escalation at cycle 3.
> `{#260612-reviewer-allocation-tier-default}`

Two further non-live statements of the same intent:

- `260619-refactor-stateless-implement-review-continuity` (`.done`), decision D6:
  "the existing hard cap (partitioned 3 cycles / single 2, lead adjudicates at
  cycle 2, caller escalation at cycle 3) is the backstop for the pathological
  case of a reviewer inventing new distinct findings each cycle — where hitting
  the cap is then a true 'contentious' signal."
- `ai-docs/.old/spec/260505/workflow-skills.md`: "Runs a relay loop (max 3
  cycles) … At cycle 2, the lead adjudicates maintained disputes; at cycle 3,
  unresolved disputes escalate to the caller."

Live implementation text carries none of it:

- `agents-plugin/rsrc/lead-implement/lead-implement.md` contains no numeric cycle
  cap and no escalation point. Its only convergence invariants are the dedup
  ones: "Use Review relay and Re-review prompts only for genuinely new non-clean
  Critical/Important findings" and "Preserve settled or deferred dispositions".
- `implementReviewInstruction` (`agents-plugin-tool/internal/mcp/session_state.go`)
  generates the review todo's Instruction and states no budget in any branch.
- The only live "Max three cycles" in the whole `rsrc/` tree is
  `lead-skill-authoring.md:120`, which governs skill-authoring review, not code
  implementation review.

History, recovered from git: the rule originated in `claude/skills/write-code/SKILL.md`
(`6df7cd61`, 2026-04-27) as a full cycle branch table, survived the
`claude-plugin/` rename and the `agents-plugin` port, and was still procedurally
present in `lead-implement.md` as recently as `3ab81a53`. It was removed in two
same-day commits: `11f52ae0` collapsed the per-cycle steps into "enforces cycle
caps", and `bb1d1e2e` deleted that phrase as bloat. Both removals were
deliberate and both assumed the rule had moved into MCP-generated todo text. It
never did. The removal was intentional; the resulting gap was not.

The original branch table, verbatim from `write-code/SKILL.md`, is the richest
statement of the intended behavior:

```text
- All [clean] → exit loop, proceed to cleanup.
- Cycle ≤ 2 and non-clean → go to relay.
- Cycle = 2 and maintained items exist: lead reads review files directly.
  For each maintained dispute: accept the won't-fix or override it.
  If any overrides: relay override list to implementer (counts as cycle 3 relay).
  Otherwise advance to cycle 3 re-review directly.
- Cycle = 3 and non-clean remain: collect unresolved findings.
  Proceed to cleanup, surface escalation in output.
```

## Why it matters

Dedup and the cap guard different failure modes, so the surviving half does not
cover the lost half:

- Dedup stops **re-litigation** — the same finding returning after being settled.
- The cap stops **divergence** — each cycle producing genuinely new, genuinely
  valid findings with no convergence in sight. Dedup permits that loop
  indefinitely, because every finding legitimately passes the "is this new?" test.

The observed run is exactly the second case: cycles 1, 2, and 3 each returned a
new, real, distinct Critical/Important finding, all in the same function, and one
of them was caused by the *lead's own* prior-cycle disposition rather than by
implementer error.

Secondary loss: D6 frames hitting the cap as "a true 'contentious' signal" — the
cap is not only a budget guard but the mechanism that converts a churning review
into an explicit owner decision.

## Decisions

**Budget is per slice, not per partition.** The counted event is the relay, and
one relay carries every non-clean review path at once; only re-review fans back
out per partition. A per-partition count is therefore not well-defined for the
event being counted. `260619`'s "per-path cycle cap" phrasing is superseded.

**The final cycle completes the run; it does not halt it.** At the last relay,
stop relaying, proceed to closeout, and carry unresolved findings with their
dispositions into the final report. This restores the original `write-code`
behavior and deliberately rejects the stricter variant introduced at `3ab81a53`
("stop before documentation stages"), which halts an autonomous goal run at the
first contentious finding. A goal loop must be able to read the escalation from a
completed run's report and decide to move to a different ticket.

**Cycle-2 adjudication is restored, triggered by `[maintained]`.** The signal
already flows: the live Re-review prompt asks reviewers to answer each won't-fix
with `[accepted]` or `[maintained: <short reason>]`. Nothing currently tells the
lead to act on it. An override ships as the next relay and **consumes** that
cycle's budget rather than adding one — the original design's discipline.

**The adjudicator is always a delegate that does not own the code.** Rejected:
an elevated implementer given an adversarial-verification prompt. Inverting the
prompt from "fix" to "refute" does remove the compliance pressure, but the
implementation owner still has an interest in the finding being invalid, and its
verdict is unfalsifiable to a lead that has not read the review files — so the
lead either trusts a conflicted party or performs the adjudication anyway.
Rejected: a config knob selecting lead-vs-delegate, because a knob incurs setter
and tuning-catalog debt (precedent: `260626-bug-sage-review-config-setter-missing`)
for a choice with no demonstrated need to vary.

**The adjudicator's aperture is wide around the diff, never inside it.** Its
question is not "is this code correct" — the reviewer answered that — but "is the
implementer's won't-fix reason true". `implementer-relay` already closes the
admissible reasons to exactly three, which determines what the adjudicator reads:

| Implementer's defense | Evidence the adjudicator weighs |
|---|---|
| conflicts with local patterns | codebase-wide patterns, convention docs, commit precedent — **not** the diff |
| requires scope expansion beyond the plan | the plan and the ticket/inline authority |
| disproven by specific evidence | the offered evidence itself |

The load-bearing constraint: **do not re-review the diff for correctness.** The
reviewer's factual claims about the diff stand unless the implementer supplied
specific disproving evidence. Without this the adjudicator degrades into a second
reviewer with extra steps. This is the opposite pole from
`ticket-reviewer-completeness`'s "Read only the ticket file at the provided path",
and must be stated as explicitly.

**Verdict vocabulary — one line per dispute:**

- `[accept]` — the defense holds; the won't-fix stands.
- `[override: <reason>]` — the defense fails; relay it as a required fix.
- `[out-of-scope: <reason>]` — the finding is valid but outside the plan; route
  to plan amendment or defer.

The third verdict is why the role exists at all. A correctness finding that
requires out-of-plan scope makes `implementer-relay`'s constraints jointly
unsatisfiable — it forbids won't-fix for correctness while requiring fixes to
stay inside the plan. The reviewer cannot amend the plan and the implementer is
forbidden to refuse; the adjudicator is the only party that can name the
deadlock.

**Capacity escalation is mechanical and lead-side, not an adjudicator verdict.**
A finding relayed twice with no won't-fix offered and still non-clean indicates
the implementer cannot fix it, not that a defense is contested — so it never
produces a `[maintained]` and never reaches the adjudicator. Handle it as a plain
condition on the next relay: dispatch at an elevated tier. This keeps the
adjudicator's job single-purpose. Rejected: widening the adjudicator's input to
"all unresolved findings after cycle 2" and adding a `[capacity]` verdict, which
dilutes judging-a-defense into judging-everything.

**`single` (2-cycle) gets no adjudication slot.** With a budget of two, an
override has no remaining relay to ship in. The spec pins single at 2, and no
historical version gave single an adjudication step.

**Placement: the single-vs-multi determination stays out of the playbook.** The
playbook keeps only the path-independent invariant; budgets and the adjudication
step are `review_alloc`-dependent and therefore verdict-specific. This follows
`lead-implement`'s own stated Doctrine — "route facts go to MCP, verdict-specific
work goes to todos, and the playbook keeps only shared gates, ownership
boundaries, and reusable templates". Concretely, the adjudication step is an
**additional todo item installed only on the partitioned path**, following the
review item, so no other path surfaces the adjudicator's existence at all.

The todo layer is not merely a convenient surface, it is the only binding one.
`lead-implement.md` instructs the lead to "Treat the installed todo list as the
ordered runbook" and says each todo's `Instruction` field "is a complete how-to
for that step — **do not restate or supplement it from memory**". Under that
contract, a rule absent from the review todo's Instruction is a rule the lead is
positively instructed not to supply. That is why the observed run had no cap in
effect: not because the lead could not have known of one, but because the runbook
it was told to follow exhaustively did not carry it. A fix that lands only in
playbook prose would reproduce the same failure.

One adjudicator spawn handles every maintained dispute in a cycle, which follows
from the output contract being a per-dispute line list rather than a single
verdict; it is not a per-dispute spawn.

**Adjudicator tier: `large`.** Same tier as the reviewers it may overrule.
`xlarge` was considered for an authority gradient — it is only an effort bump on
the same model in the codex seed map — but rejected because on the opus harness
`xlarge` maps to a materially more expensive model than a per-dispute judgment
warrants.

**Prompt-format precedent: `ticket-reviewer-design`.** It is the closest existing
shape — an independent judgment delegate whose aperture is deliberately wider
than its sibling reviewer's.

## Open: whether a lead-side root-cause check belongs here

`impl-playbook` carries a stronger, non-numeric rule: "**Repetition check**
(mechanical, every failure): Before fixing, ask and answer: 'Is this the same
root cause as a previous failure in this session?' If yes, stop and escalate
immediately. Do not attempt another fix."

In the observed run that rule should have fired at cycle 3 — all three findings
share one root cause (bounding a subprocess's output collection). It did not fire
because the rule is addressed to the *implementer* handling a test failure, and
each relay went to a fresh implementer spawn with no view of prior cycles. The
lead, the only party that sees the whole sequence, has no equivalent instruction.

That argues for a lead-side root-cause check: relaying a finding whose root cause
matches an already-relayed finding is itself an escalation signal, independent of
cycle count — and the observed run suggests the condition is the load-bearing
half, with the count catching it only incidentally.

**Scope is undecided.** This is a third mechanism alongside the budget (Phase 1)
and the adjudicator (Phase 2), and nothing in the discussion settled whether it
belongs in this ticket, in a sibling, or nowhere. Resolve before either phase is
implemented; do not let an implementer infer it either way.

## Constraints

- Delegate prompts stay self-contained (`260619`'s stateless premise). The
  adjudicator's inputs are files: plan, review findings paths, disposition
  record, commit range, authority. No cycle state may be phrased as
  implementer-visible state.
- Changed prompt and generated-instruction lines are skill text: apply
  `agents-plugin/skills/lead-skill-authoring/SKILL.md`'s invariant checklist to
  each one.
- Touching `agents-plugin-tool/` invokes the dev-merge version bump through
  `agents-plugin-tool/scripts/bump-ws-version.sh`.
- `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` is byte-identical
  to the full-ws copy and carries the same absence; mirror any playbook change.

## Phases

### Phase 1: Restore the budget and the final-cycle behavior

Close the spec violation without introducing the adjudicator. Two surfaces.

**Playbook** — one path-independent invariant line added to `lead-implement.md`'s
Review block, and mirrored into the wsflow copy:

```markdown
- Relay budget is per slice, not per partition: one relay carries every non-clean path. At the final cycle, stop relaying, proceed to closeout, and carry unresolved findings with their dispositions into the final report. Do not halt the run.
```

**Generated todo instruction** — `implementReviewInstruction` states the budget
per branch, since the generator already holds `review_alloc` and is the cheapest
place to settle it. The `partitioned:` branch states 3 cycles for the slice; the
`single` branch states 2. The lead-only and fallback branches are untouched
because neither runs a relay loop. Update the string pins in
`session_state_test.go`.

Rejected alternative: stating the numbers in the playbook. They are
`review_alloc`-dependent, so they belong to the todo layer per the playbook's own
Doctrine, and putting a `single`-vs-`partitioned` branch in shared prose is the
thing this ticket is trying to stop.

Verification boundary: a `partitioned:` verdict's review todo Instruction names a
3-cycle slice budget and the completes-not-halts behavior; a `single` verdict's
names 2; a `lead-only` verdict's names neither; the playbook line appears once in
each of the two `lead-implement.md` copies and states no number.

### Phase 2: Adjudicator delegate and capacity elevation

Depends on Phase 1 — the adjudication step hangs off the budget it consumes.

**New rsrc delegate** modeled on `ticket-reviewer-design`, `tier: large`,
carrying the aperture constraint, the read table, and the three-verdict output
contract from `## Decisions`. Inputs are file paths only.

**New todo item** installed by `deriveImplementTodosFromVerdict` only when
`review_alloc` is partitioned, following the `review` item. This mirrors the
existing conditional-assembly pattern already used for `NeedReview`, `NeedDoc`,
and `isCurrentBranchCompletion`. Its Instruction carries: the `[maintained]`
trigger, the adjudicator spawn method, that an override consumes the next relay's
budget rather than adding one, and that the step is **optional when no dispute
was maintained**.

**Capacity elevation** stated in the same Instruction as a separate condition:
a finding relayed twice with no won't-fix offered and still non-clean dispatches
the next relay at an elevated tier. Use the render-resolved tier-model vars from
`260714-feat-playbook-tier-model-render-vars` rather than naming a model.

Verification boundary: a partitioned verdict installs the adjudicate todo and a
single/lead-only verdict does not; the rendered adjudicator prompt states the
do-not-re-review-the-diff constraint and the three verdicts; the adjudicate
Instruction states both the `[maintained]` trigger and the capacity condition and
marks the step optional; delegate prompt remains self-contained under a fresh
spawn with no prior conversation.

## Non-Goals

- Changing what counts as `clean`. `260619` D5 explicitly rejected a machine gate
  ("no Critical/Important == clean") in favor of lead judgment; this ticket adds a
  stopping rule, not an automated verdict.
- Reintroducing the retired `.old/spec/260505` text wholesale.
- Blocking the dashboard FS-watch ticket. That work continues; this ticket only
  records why its Phase 1 review loop ran to the cap without anything noticing.

## Blocked (2026-07-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Open scope question is explicitly unresolved and self-declared blocking | critical | missing |
| 2 | Cycle-counting semantics are ambiguous by a full round | important | autonomous |
| 3 | Phase 2 directs render variables into a surface that is never rendered | important | autonomous |
| 4 | wsflow rsrc tree is generated; ticket directs a hand-edit and omits regeneration | important | autonomous |
| 5 | Adjudicate todo placement contradicts the ordered-runbook contract it relies on | important | autonomous |
| 6 | Phase 1's premise about the untouched fallback branch is false | minor | autonomous |
| 7 | Proposed playbook line fails the invariant checklist the ticket mandates | minor | autonomous |
| 8 | Spec anchor says lead adjudication; Phase 2 makes it a delegate, with no spec update named | minor | autonomous |
