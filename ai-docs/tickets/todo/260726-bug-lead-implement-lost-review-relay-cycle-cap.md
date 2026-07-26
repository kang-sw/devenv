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

**A cycle is a review round, not a relay.** Cycle N is the Nth review of the
slice: cycle 1 is the initial review, so a budget of 3 permits at most 3 reviews
and therefore at most 2 relays. This is the indexing the restored `write-code`
branch table uses (`Cycle = 3 and non-clean remain`), and it is the only reading
under which the observed run — three cycles complete, a fourth relay pending —
had already reached the cap. Counting relays instead would permit a fourth review
round, leaving the observed run inside budget and the reported defect unfixed.

**Budget is per slice, not per partition.** One relay carries every non-clean
review path at once; only re-review fans back out per partition, so a
per-partition count has no distinct event to count. `260619`'s "per-path cycle
cap" phrasing is superseded.

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
boundaries, and reusable templates". Concretely, every budget, adjudication, and
escalation clause is generated into the **review todo's Instruction**, which
already branches on `review_alloc` — so the `single` and `lead-only` branches
never surface the adjudicator's existence at all.

Rejected: installing adjudication as a separate todo item after `review`.
Adjudication is intra-loop — cycle-2 re-review yields `[maintained]`, the
adjudicator runs, an override ships as the next relay, and the loop re-enters
review — but `lead-implement` requires executing the installed todo list as an
ordered runbook, so a strictly-later item would ask the lead to return to a
completed step. Keeping the clause inside the review Instruction preserves what
motivated the separate item — invisibility on non-partitioned paths — without
contradicting the ordering contract the ticket relies on elsewhere.

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

## Why a lead-side root-cause check is in scope

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

Scoped into this ticket as Phase 2, alongside capacity elevation: both are
lead-side conditions expressed as text in the same generated Instruction, and
neither needs a delegate. Sequencing them before the adjudicator is deliberate —
if Phase 3 never lands, Phases 1 and 2 still close the observed defect.

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
- `agents-plugin-wsflow/rsrc/` is a **generated** byte-identical mirror guarded by
  `TestWsflowRsrcMirrorUpToDate`; never hand-edit it. After any
  `agents-plugin/rsrc/` change, regenerate in this order — first
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  (required only when a file is added or removed), then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
  `-count=1` is mandatory. See `ai-docs/ref/wsflow-mirroring.md`; the missed-regen
  failure mode has its own ticket
  (`260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`).
- Generated todo Instructions are plain Go strings with no template resolution.
  Render-time variables such as `{{.LargeTierModel}}` resolve only in
  `playbook.render` output and would surface literally if written into an
  Instruction; name tiers by the capability vocabulary instead.

## Phases

### Phase 1: Restore the budget and the final-cycle behavior

Close the spec violation without introducing the adjudicator. Two surfaces.

**Playbook** — two path-independent invariant lines added to `lead-implement.md`'s
Review block, one rule each per `lead-skill-authoring`'s one-line requirement:

```markdown
- A review cycle is one review round; the relay budget is per slice, not per partition.
- At the final cycle, stop relaying, complete closeout, and carry unresolved findings with their dispositions into the final report.
```

Regenerate the wsflow mirror rather than editing its copy.

**Generated todo instruction** — `implementReviewInstruction` states the budget
per branch, since the generator already holds `review_alloc` and is the cheapest
place to settle it. Three of the four branches run a relay loop and need it: the
`partitioned:` branch states 3 review cycles for the slice, the `single` branch
states 2, and the fallback branch (`session_state.go:576`) states 3 — it is
reachable whenever `review_alloc` is the bare string `partitioned`, which
`parseImplementReviewAlloc` produces on the legacy `enter.implement` path with no
`target` argument. Only the `lead-only` branch is untouched, because it dispatches
no reviewers and therefore never relays. Update the string pins in
`session_state_test.go`.

Rejected alternative: stating the numbers in the playbook. They are
`review_alloc`-dependent, so they belong to the todo layer per the playbook's own
Doctrine, and putting a `single`-vs-`partitioned` branch in shared prose is the
thing this ticket is trying to stop.

Verification boundary: a `partitioned:` verdict's review todo Instruction names a
3-review-cycle slice budget and the completes-not-halts behavior; a `single`
verdict's names 2; the fallback branch names 3; a `lead-only` verdict's names
neither; both `lead-implement.md` copies carry the two lines with no number, and
the wsflow mirror test passes without a hand-edit.

### Phase 2: Root-cause escalation and capacity elevation

Depends on Phase 1 — both conditions qualify the budget it establishes. Text
only, no new delegate, both appended to the same `partitioned:`/fallback review
Instruction.

**Root-cause escalation.** Relaying a finding whose root cause matches an
already-relayed finding is itself an escalation signal, independent of cycle
count. This is the lead-side counterpart to `impl-playbook`'s repetition check,
which cannot fire across stateless fresh-spawn relays. The observed run argues
this condition is the load-bearing half and the count only the backstop.

**Capacity elevation.** A finding relayed twice with no won't-fix offered and
still non-clean means the implementer cannot fix it rather than that a defense is
contested. Dispatch the next relay at an elevated tier — name it by capability
tier (`large`), never by model name and never by a render variable, per the
Instruction constraint above.

Verification boundary: the `partitioned:` and fallback Instructions each state
both conditions and distinguish them from the numeric budget; `single` and
`lead-only` state neither; no `{{` appears in any generated Instruction.

### Phase 3: Adjudicator delegate

Depends on Phase 2 — the adjudicator resolves the disputes Phase 2's conditions
surface.

**New rsrc delegate** modeled on `ticket-reviewer-design`, `tier: large`,
carrying the aperture constraint, the read table, and the three-verdict output
contract from `## Decisions`. Inputs are file paths only. Adding an rsrc file
requires the manifest regeneration named in `## Constraints` before the mirror
regeneration.

**Review Instruction clause** for the `partitioned:` and fallback branches only:
the `[maintained]` trigger, the adjudicator spawn method, that an override ships
as the next relay and consumes its budget rather than adding one, and that the
step is **optional when no dispute was maintained**.

**Spec update, required.** `#260612-reviewer-allocation-tier-default` currently
reads "3 cycles for partitioned with lead adjudication at cycle 2". This phase
makes the adjudicator a delegate and introduces a verdict vocabulary the spec
does not describe, so the anchor must be updated in the same logical change —
otherwise this phase opens a new spec divergence while the ticket closes an old
one.

Verification boundary: a partitioned verdict's Instruction carries the
adjudication clause and a single/lead-only verdict's does not; the rendered
adjudicator prompt states the do-not-re-review-the-diff constraint and the three
verdicts; the delegate prompt is self-contained under a fresh spawn with no prior
conversation; the spec anchor no longer says "lead adjudication"; manifest and
wsflow mirror tests pass without hand-edits.

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
