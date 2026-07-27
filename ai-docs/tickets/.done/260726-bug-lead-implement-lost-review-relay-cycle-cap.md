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
    vars — deliberately NOT used by this ticket, since they resolve only in
    playbook.render output and would surface literally in a generated Instruction
spec:
  - 260612-reviewer-allocation-tier-default
  - 260619-stateless-implement-review-continuity
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-27
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
| **style suggestion** conflicting with local patterns | codebase-wide patterns, convention docs, commit precedent — **not** the diff |
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
- `[out-of-scope: <reason>]` — the finding is valid but outside the plan; it
  becomes a recorded deferral.

The third verdict is why the role exists at all. A correctness finding needing
out-of-plan scope puts `implementer-relay` in a bind: it forbids won't-fix for
correctness while requiring fixes to stay inside the plan. Its Process step 3
supplies an escape — escalate for a plan update — but escalating only moves the
question, it does not answer it, and the reviewer cannot amend the plan. The
adjudicator is the party that decides whether the plan should absorb the finding
or the finding should be deferred. See Phase 2 for why this makes the trigger
two-armed.

**`[out-of-scope]` is a deferral with authority, not a plan amendment.** The
adjudicator does not edit the plan — it has no more authority to do so than the
reviewer it was brought in to arbitrate, and a delegate rewriting the plan mid-loop
would silently move the contract the whole run is verified against. The verdict
does three things and nothing else: the finding leaves the relay list, it costs no
relay, and it is carried into the run's completion output as unresolved-by-decision
with the adjudicator's reason. The run may complete with such findings outstanding —
the same closure `[deferred]` already has, and consistent with the decision that the
final cycle completes the run rather than halting it. Amending the plan, if anyone
wants it amended, is a caller-layer action after the run, not a step inside the loop.

**Adjudication happens between reviews and never adds one.** The `[maintained]`
arm surfaces at a re-review, but the `[escalate]` arm surfaces mid-relay, before
any review has run. The lead adjudicates it immediately and returns the verdict to
the implementer within the same relay slot; no review is consumed, because none
occurred. This is safe precisely because the cap counts reviews: nothing that
happens between two reviews can expand a budget denominated in reviews. Bound it
at **one adjudication per relay slot** — if the implementer escalates again after
receiving a verdict, the lead stops adjudicating and lets the finding reach the
next review as-is, where it reports non-clean and falls to the ordinary path.
Without that bound the slot admits an unbounded adjudicate/escalate ping-pong that
the review budget cannot see.

**Capacity escalation is mechanical and lead-side, not an adjudicator verdict.**
The condition is **a finding the implementer reported `[fixed]` that the next
review still reports non-clean.** Stated positively, not as "no won't-fix
offered": the negative form sweeps in `[deferred]`, an adjudicator
`[out-of-scope]`, and an unresolved `[escalate]` — none of which are won't-fixes,
all of which the reviewer keeps reporting non-clean, and all of which are settled
dispositions rather than failures of capacity. Routing a deliberate deferral to a
larger model as a capacity failure would contradict the three-failure-class
separation this ticket draws. A claimed-and-rejected fix is the one disposition
that means the implementer tried and could not — not that a defense is contested,
so it never produces a `[maintained]` and never reaches the adjudicator. Handle it
as a plain routing condition on the next relay: dispatch a distinct
`implementer-elevated` delegate (Phase 3) rather than elevating
`implementer-relay`'s tier. This keeps
the adjudicator's job single-purpose. Rejected: widening the adjudicator's input to
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

Scoped into this ticket as Phase 3, alongside the capacity count: they are two
detectors for one signal — the approach is failing rather than the patch — and
both route to the same elevated delegate. Keeping them apart would have given the
root-cause detector its own phase with no destination, which is this ticket's own
defect class.

## Constraints

- Delegate prompts stay self-contained (`260619`'s stateless premise). The
  adjudicator's inputs are files: plan, review findings paths, disposition
  record, commit range, authority. Budget accounting stays lead-owned — the
  delegate is told the current cycle number but never asked to enforce or track
  the budget. `implementer-relay` already renders `ReviewCycle` into the
  implementer's prompt; that stays.
- Changed prompt and generated-instruction lines are skill text: apply
  `agents-plugin/skills/lead-skill-authoring/SKILL.md`'s invariant checklist to
  each one.
- Touching `agents-plugin-tool/` invokes the dev-merge version bump through
  `agents-plugin-tool/scripts/bump-ws-version.sh`.
- `agents-plugin-wsflow/rsrc/` is a **generated** byte-identical mirror guarded by
  `TestWsflowRsrcMirrorUpToDate`; never hand-edit it. After any
  `agents-plugin/rsrc/` change, regenerate in this order — first
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  (required after **any** content edit, not only when a file is added or removed —
  `TestShippedManifestUpToDate` fails on changed content; the separate
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  entry point covers the shipped manifest), then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
  `-count=1` is mandatory. See `ai-docs/ref/wsflow-mirroring.md`; the missed-regen
  failure mode has its own ticket
  (`260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`).
- New rsrc playbooks declare `role:` from the closed set
  `childRoleForPlaybookRole` accepts — `implementer`, `reviewer`, `delegate`,
  `leaf`. Anything else, including a natural-reading `role: adjudicator`, falls to
  the default branch and mints **no child session key, silently**: no error, and a
  prompt that renders normally. `review-adjudicator` uses `role: delegate`;
  `implementer-elevated` uses `role: implementer`, matching the sibling it
  replaces. `role` has a second consumer at `playbook_tools.go:763-769` that
  appends the `prefer_mercenary` guidance block for `implementer`/`reviewer` only,
  so a drop-in implementer replacement declared `delegate` would silently lose that
  guidance on the same dispatch path. Every
  input either delegate takes must also appear in `variables:`; `renderPlaybook`
  rejects an undeclared key with `ErrUndeclaredVar`, so Phase 3's added prior-fix
  and prior-disposition inputs are a `variables:` change, not only prose.
- Generated todo Instructions are plain Go strings with no template resolution.
  Render-time variables such as `{{.LargeTierModel}}` resolve only in
  `playbook.render` output and would surface literally if written into an
  Instruction; name tiers by the capability vocabulary instead.

## Phases

### Phase 1: Restore the budget and the final-cycle behavior

Close the spec violation without introducing the adjudicator. **One surface: the
generated Instruction. `lead-implement.md` is not edited.**

Rejected: adding a summary invariant line to the playbook's Review block.
`lead-skill-authoring`'s destructive-first test deletes playbook prose that
restates what an MCP tool returns post-call, and since this phase's verification
boundary requires the Instruction to carry the budget and the completes-not-halts
behavior, any playbook line stating them is a restatement by construction. The
ticket's own argument settles it: a lead is positively instructed not to
supplement the Instruction from memory, so the playbook line would be inert as
well as redundant. This also means Phase 1 touches no `rsrc/` file and needs no
mirror or manifest regeneration.

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

**Spec amendment, required in this phase.**
`#260612-reviewer-allocation-tier-default` is wrong on two counts, not merely
under-described. It says "relay cap", which is the unit this ticket rejects — the
cap counts reviews, and calling it a relay cap is exactly the off-by-one-round
ambiguity `## Decisions` pins down. It also says "caller escalation at cycle 3",
which contradicts the decision that the final cycle completes the run. Leaving
either in place would make Phase 1 close a divergence by opening a fresh one.

Verification boundary: a `partitioned:` verdict's review todo Instruction names a
3-review-cycle slice budget and the completes-not-halts behavior; a `single`
verdict's names 2; the fallback branch names 3; a `lead-only` verdict's names
neither; the spec anchor states the cap in review cycles and no longer says the
run halts at cycle 3; `lead-implement.md` is byte-identical to its pre-phase state
in both copies.

### Result (d911f70d) - 2026-07-27

Done. The budget is live where the executing lead actually reads it.

- `implementReviewInstruction` states a per-slice budget on the three relaying
  branches — 3 review cycles for `partitioned:` and for the bare-`partitioned`
  fallback, 2 for `single` — each pinning cycle 1 to the initial review and
  naming the derived relay count, so the off-by-one-round reading is closed from
  both directions. `lead-only` is untouched.
- The completes-not-halts behavior is a shared `const`
  (`implementReviewFinalCycleClause`) rather than three copies, so the branches
  cannot drift apart on the one clause that is identical across them. It routes
  forward ("continue to the remaining todos") and ends with "the budget ends
  relaying, not the run", which forecloses the `3ab81a53` halt reading explicitly
  instead of by implication.
- `{#260612-reviewer-allocation-tier-default}` now states the cap in review
  cycles and replaces "caller escalation at cycle 3" with the completes-not-halts
  behavior. "lead adjudication at cycle 2" was deliberately left in place for
  Phase 2, whose own verification boundary asserts on it.
- No file under `agents-plugin/rsrc/` or `agents-plugin-wsflow/rsrc/` was
  touched; both `lead-implement.md` copies are byte-identical to their pre-phase
  state, verified from the diff. No manifest or mirror regeneration was needed.

Beyond the phase text: the survey found that no test pinned the fallback branch's
Instruction at all — only its todo keys — so that branch got new coverage rather
than a pin update, asserting on a string unique to the fallback to prove the
branch was reached. The `lead-only` "names neither" boundary was also pinned with
a negative assertion, mutation-checked by temporarily leaking the budget clause
into that branch and confirming the failure. A second, non-anchor "relay cap" use
in the same spec file was renamed in the same change, since the anchor fix left it
dangling on a term the spec no longer defines.

Review: one full-scope cycle, clean with 4 minor. Two accepted and relayed (the
artifact-name split, the missing negative assertion); two rejected.

Verification: `go build ./...` and `go test ./... -count=1` from
`agents-plugin-tool/`, all 12 packages passing.

> Forward: the generator and the spec now use different nouns for the run's output
> artifact — `implementReviewInstruction` says "final report" (matching its own
> `lead-only` branch and the playbook's final-action-gate step), while
> `ai-docs/spec/workflow-skills.md` says "completion report" at both its
> pre-existing site and the sentence this phase wrote. Each surface is internally
> consistent, so nothing is ambiguous in place; settling on one noun across both
> belongs to Phase 2, which edits these same strings.

### Phase 2: Adjudicator delegate

Depends on Phase 1 — the adjudicator spends the budget Phase 1 establishes, and
its override ships as one of that budget's relays.

**New rsrc delegate `review-adjudicator`** modeled on `ticket-reviewer-design`,
`tier: large`,
carrying the aperture constraint, the read table, and the three-verdict output
contract from `## Decisions`. Inputs are file paths only. Adding an rsrc file
requires the manifest regeneration named in `## Constraints` before the mirror
regeneration.

**Review Instruction clause** for the `partitioned:` and fallback branches only:
the trigger, the adjudicator spawn method, that an override ships as the next
relay and consumes its budget rather than adding one, and that the step is
**optional when neither trigger fired**.

The trigger has two arms, not one. `[maintained]` covers a contested won't-fix.
It does **not** cover the `[out-of-scope]` case: `implementer-relay` Process step
3 already tells the implementer to "escalate for a plan update if a required fix
needs ticket material or a plan deviation", and its constraints forbid won't-fix
for correctness — so a compliant implementer facing an out-of-plan correctness
finding escalates rather than refusing, producing no won't-fix and no
`[maintained]`. The deadlock the third verdict exists to name would bypass the
role. The second arm is therefore an implementer plan-update escalation, which
routes to the adjudicator for the same accept/override/out-of-scope judgment.

That second arm needs a token to fire on. `implementer-relay`'s Output contract
offers `[fixed]`/`[won't fix]`/`[deferred]` and nothing else, so a Process-step-3
escalation currently surfaces only as free-text under "Deviations or blockers" —
undetectable by a lead following an Instruction. This phase adds
`[escalate: <reason>]` to that contract. The playbook enumerates the token set
**twice** — the Output section and Process step 4 — so both sites change together;
editing only the Output contract leaves the two enumerations contradicting each
other, which is the failure mode that produced this ticket's own defect. Scope
note: this is the one `implementer-relay` edit the ticket makes, and it is additive
to an output vocabulary rather than a new rule, so it does not reopen Phase 1's
decision to leave the playbook layer alone.

Because `implementer-relay` is dispatched on **every** relay path, the token also
reaches a `single`-path lead whose Instruction has no adjudication clause. Under
this ticket's own "do not supplement the Instruction from memory" contract that is
an unhandled signal, so the `single` branch gains one sentence: with no
adjudication slot at a 2-cycle budget, `[escalate]` there is the lead's own
accept-or-defer call. Naming the degradation is the point — the alternative is a
structured token that one path defines and another silently ignores.

**Spec update, required.** `#260612-reviewer-allocation-tier-default` currently
reads "3 cycles for partitioned with lead adjudication at cycle 2". This phase
makes the adjudicator a delegate and introduces a verdict vocabulary the spec
does not describe, so the anchor must be updated in the same logical change —
otherwise this phase opens a new spec divergence while the ticket closes an old
one.

Verification boundary: a partitioned verdict's Instruction carries the
adjudication clause and a single/lead-only verdict's does not; the rendered
`review-adjudicator` prompt states the do-not-re-review-the-diff constraint and
the three verdicts; the delegate prompt is self-contained under a fresh spawn with
no prior conversation; **both** of `implementer-relay`'s token enumerations list
`[escalate: <reason>]`, verified by grepping the file for the token set rather
than by reading one section; the Instruction states that adjudication happens
within a relay slot without consuming a review and is bounded to one per slot, and
states the `[out-of-scope]` disposition (leaves the relay list, costs no relay,
surfaces in completion output); the `single` Instruction names the `[escalate]`
degradation; the spec anchor no longer says "lead adjudication";
manifest and wsflow mirror tests pass without hand-edits.

### Result (42051e7a) - 2026-07-27

Done. Contested findings now have an owner that is neither the reviewer nor the
implementer.

- New rsrc delegate `review-adjudicator` (`role: delegate`, `tier: large`),
  carrying the aperture constraint, the read table, and the three-verdict output
  contract. The read table's rows are addressed by their defense rather than
  positionally, so a future row insertion cannot silently re-point the rules that
  cite them.
- The adjudication clause is shared by the `partitioned:` and fallback branches
  and states both trigger arms with their differing budget effects: a
  `[maintained]` override ships as the next relay and spends that cycle, while an
  `[escalate]` verdict returns inside the current relay slot and spends nothing
  because no review has run. All three verdicts have a stated disposition —
  `[accept]` was initially left undefined, which under this ticket's own
  do-not-supplement-from-memory contract is an undefined action.
- `[escalate: <reason>]` landed at both of `implementer-relay`'s token
  enumeration sites, pinned by a count assertion rather than a plain substring
  check, so a one-site edit fails.
- The `single` branch names the degradation instead of ignoring the token.
- `lead-only` gained a negative pin for the adjudication vocabulary, matching the
  precedent Phase 1 set, mutation-checked by leaking the clause onto that branch
  and observing the failure.
- Spec `{#260612-reviewer-allocation-tier-default}` now describes the adjudicator
  as a delegate and carries the verdict vocabulary.

Also closed here: Phase 1's forwarded noun split. The run's output artifact is
"final report" across the generated Instructions and the spec, including the
pre-existing spec sites and the mirroring `mcp-runtime` mental-model line. The
playbook's "the normal completion report" was deliberately left alone — it names
the *delegate's* return, a different artifact.

Deliberate difference worth recording: `review-adjudicator`'s `variables:` list
mixes CamelCase and snake_case. That is not an oversight — it reuses the exact key
names the sibling playbooks already use for the same concepts, which is the
stronger consistency axis than internal uniformity.

Review: three cycles, partitioned. Cycle 1 returned non-clean with 4 Important
(the dispatch clause named only 5 of the delegate's declared inputs, so a lead
following it literally would hit `ErrUnprovidedVar`; the two trigger arms were
conflated on budget; `lead-only` had no negative pin; and Phase 1's forwarded noun
split had been omitted from the plan). Cycle 2 returned clean on all three
partitions. Cycle 3 re-reviewed fit and test only, after a final relay fixing two
prompt-wording defects; correctness was not re-run because no mechanism changed.
Budget spent in full, no findings left contested.

Verification: both regen commands run in order with `-count=1`, idempotent on a
second run; `go build ./...` and `go test ./... -count=1` green on all 12
packages; the wsflow mirror byte-identical and never hand-edited.

> Forward: the adjudication clause is now a single unbroken paragraph of eight
> sentences, and Phase 3 adds two more routing conditions to the same string plus
> the "only for genuinely new" rewording. Under `lead-skill-authoring`'s reader
> model this is the last comfortable moment to give the clause internal structure —
> one rule per sentence with a leading token label, or a split by concern — before
> Phase 3 doubles it.

> Forward: `lead-implement.md`'s Task input mapping enumerates every delegate
> except `review-adjudicator`, and its "Collect the normal completion report" step
> does not describe the adjudicator's per-dispute verdict-line return. Correct for
> Phase 2, which may not edit that file, but no phase currently owns the gap —
> Phase 3's dispatch-surface list names three sites and not this mapping, even
> though Phase 3 edits that exact line for `implementer-elevated`.

### Phase 3: Elevated implementer delegate and capacity escalation

Depends on Phase 2 — same trigger surface, same regeneration pass, same
delegate-prompt shape, and the pair only reads coherently once both last-resort
roles exist. Sequenced last because it is the one arm the observed run does not
evidence directly.

**New rsrc delegate `implementer-elevated`**, declaring `tier: large` in its own
front-matter. This is what makes capacity escalation implementable at all: no
render-time tier override exists, and spec `#260612-reviewer-allocation-tier-default`
makes a delegate's declared tier authoritative — so a separate delegate
*satisfies* that sentence where a per-dispatch elevation of `implementer-relay`
would contradict it. Rejected accordingly: instructing the lead to deviate from
dispatch metadata, raising `implementer-relay`'s declared tier unconditionally
(which is not conditional at all), and adding a render-time tier override (an MCP
API-semantics change for a single case).

**The prompt must differ in kind, not only in tier.** A tier-only copy would be
near-duplicate prose — the debt `260630-epic-skill-playbook-diet` exists to
reduce — and would not help: a larger model executing the same instruction
executes the same failed approach more competently. What distinguishes it:

- **Inputs** add the prior cycles' fix commits and dispositions — the record of
  what was already attempted.
- **Posture**: question whether the previous fixes addressed a symptom rather
  than the cause, before writing another patch. Licensed to propose a different
  approach inside the plan, and to escalate for a plan update when the right fix
  is outside it.
- **Output**: state what was attempted and why it failed even when it also fails,
  so three cycles of evidence survive the handoff.

**Capacity condition**, added to the `partitioned:`/fallback review Instruction:
a finding the implementer reported `[fixed]` **once** that the next review still
reports non-clean routes the following relay to `implementer-elevated` instead of
`implementer-relay`. Findings carrying a settled disposition — `[won't fix]`,
`[deferred]`, `[out-of-scope]`, or an open `[escalate]` — are excluded, because a
reviewer keeps reporting those non-clean and they are decisions rather than failed
attempts. A rejected fix attempt is a capacity signal, not an adjudication one —
it is the third arm alongside
Phase 2's two, and together they cover the three failure classes this ticket
separates: wrong finding and scope deadlock to the adjudicator, failed approach
here.

Once, not twice. A 3-review budget affords exactly 2 relays, so a
relayed-twice-and-still-failing state is first observable at the cycle-3 review —
the terminal cycle, after which the run completes and no relay follows. The
elevated implementer would then be permanently unreachable, which is the same
never-fires defect class this ticket exists to close. Firing after one failed
relay spends the second relay on the elevated delegate, which is the only slot
where it can act.

**Root-cause matching** rides the same route. A **newly surfaced** finding whose
root cause matches an already-relayed finding is an escalation signal independent
of the count: it is the lead-side counterpart to `impl-playbook`'s repetition
check, which cannot fire across stateless fresh-spawn relays. Word it that way and
not as "a finding relayed again" — recurrence of the *same* finding is already the
capacity condition, and the observed run is precisely the other shape: cycles 1, 2,
and 3 each produced a **distinct** finding sharing one root cause (bounding a
subprocess's output collection). A recurrence-worded detector would not have fired
in the run that motivates it. It is folded in here rather than kept as its own
phase because it is a second detector for one signal — the approach is failing, not
the patch — and routes to the same delegate. The observed run argues this detector
is the load-bearing half and the count only the backstop.

**Precedence when both fire.** A relay can carry an adjudicator override list and
a capacity signal at once. The combined relay goes to `implementer-elevated`
carrying the override list; the elevated delegate subsumes the relay delegate's
job, so two relays are never dispatched for one cycle.

**The capacity signal must flow structurally, not by prose correlation.** Phase 2
checks this for the adjudication arm — `[maintained]` already flows because
`lead-implement.md`'s **Re-review prompt** asks for it per item, and `[escalate]`
is added where it did not. The capacity arm needs the same check and currently
fails it: that Re-review prompt asks for a per-item verdict **only on
`[won't fix]` items**, so a `[fixed]` item that did not land carries no token at
all. Without one the lead has to diff a fresh findings file against its own
disposition record by reading prose, which is judgment work — and this ticket's
whole argument is that a routing condition the lead must infer is a condition that
does not fire. Add the symmetric ask: for each `[fixed]` item, respond `[resolved]`
or `[unresolved: <short reason>]`. This is not the regression-vs-preexisting
classification that `#260619-stateless-implement-review-continuity` withholds from
the reviewer — it is a verdict on a named prior finding, the same shape as
`[accepted]`/`[maintained]`.

**Wording conflict to resolve in the same change.** The live Instruction strings
(`session_state.go:571,576`) say to use the relay and re-review prompts "only for
genuinely new non-clean Critical/Important findings". Read literally that forbids
relaying a persisting finding — the exact relay the capacity condition exists to
route. The phrase must be restated so a finding reported `[fixed]` and returned
`[unresolved]` is relayable; it was aimed at reviewer-invented churn, not at
unresolved carryover.

**Dispatch surface — three templates, not two.** `lead-implement.md`'s **Review
relay dispatch** template hard-codes `implementer-relay` and its seven inputs, and
the delegate dispatch task-input mapping sits alongside it; both must gain the
conditional target, together with their pinned assertions in
`playbook_tools_test.go`. The **Re-review prompt** template is the third, carrying
the `[resolved]`/`[unresolved]` ask above — it has no pinned assertion today, so
nothing would fail if it were skipped, which is why it is named here explicitly.
None of this reverses Phase 1's decision to leave the playbook alone: the
playbook's own Doctrine keeps *reusable templates* while sending rules to the todo
layer, and a dispatch template naming a delegate the lead cannot otherwise reach —
or a response vocabulary the lead cannot otherwise receive — is template material,
not an invariant.

Spec: amend `#260612-reviewer-allocation-tier-default` for the new delegate and
its dispatch condition.

Verification boundary: `playbook.render(name: "implementer-elevated")` returns
`recommended-tier: large` with no caller override; its prompt differs from
`implementer-relay` in inputs, posture, and output contract rather than only in
front-matter, with every added input declared in `variables:` and a render passing
them all succeeding rather than returning `ErrUndeclaredVar`; both new delegates
mint a child session key under a fresh render, which is the observable proof their
`role:` value is in the accepted set, and a `prefer_mercenary` render of
`implementer-elevated` carries the same guidance block as `implementer-relay`; the
Re-review prompt asks `[resolved]`/`[unresolved]` per `[fixed]` item, so the
capacity condition reads a token rather than inferring from prose; no Instruction
still says relay is "only for genuinely new" findings; the root-cause condition is
worded on newly surfaced findings rather than recurrences, checked against the
observed run's shape (three distinct findings, one root cause) rather than by
reading the sentence; the capacity and root-cause conditions both appear in
the `partitioned:` and fallback Instructions and nowhere else, with the root-cause
condition distinguished from the numeric budget; no `{{` appears in any generated
Instruction; a walkthrough of a 3-cycle slice reaches `implementer-elevated`
within the budget rather than one relay past it; manifest and wsflow mirror tests
pass without hand-edits. The behavior is verified by dispatching, not by reading
Instruction text — a named tier that no dispatch path honors passes a text-only
check, which is how the original cap was lost.

### Result (4f7dba53) - 2026-07-27

Done. The ticket's three failure classes now each have a distinct route: a wrong
finding or a scope deadlock goes to the adjudicator, a failing approach goes to
the elevated implementer.

- New rsrc delegate `implementer-elevated` (`role: implementer`, `tier: large`).
  `role: implementer` is load-bearing for a second reason beyond child-key
  minting: that value also gates whether the `prefer_mercenary` guidance block is
  appended, so a drop-in replacement declared `delegate` would have rendered
  normally, minted a key, and silently lost that guidance on the same dispatch
  path. The prompt differs on all three required axes — it consumes the prior
  cycles' fix commits and dispositions rather than only declaring them, inverts
  the posture toward naming a root cause before editing, and carries an attempt
  record written whatever the disposition so a failed cycle's evidence survives
  the handoff.
- Both routing conditions live in a new `implementReviewElevatedRelayClause`,
  split off from the adjudication clause by concern. Capacity fires after one
  failed relay, not two — at two it would first be observable at the terminal
  review, after which no relay follows, making the delegate permanently
  unreachable. Root-cause is worded on a newly surfaced finding sharing a cause
  with an already-relayed one; a recurrence wording would not have fired in the
  run that motivates it, which produced three distinct findings sharing one
  cause.
- The signal now flows structurally: the Re-review prompt asks
  `[resolved]`/`[unresolved: <reason>]` per `[fixed]` item, so the capacity
  condition reads a token instead of the lead diffing prose against its own
  disposition record.
- The "only for genuinely new" phrasing was restated at every site, including
  spec anchor `{#260619-stateless-implement-review-continuity}`, which a reviewer
  caught still forbidding the exact carryover relay the same change requires.
  Dedup itself is unweakened — the rewrite enumerates what "settled" covers, so
  the two anchors now state one exclusion set.

Deviations, both lead-directed: the adjudication clause was restructured with
leading token labels rather than left as an eight-sentence paragraph, honouring
Phase 2's forward note before this phase doubled it; and Phase 2's second forward
note was closed here rather than deferred — `review-adjudicator` was backfilled
into `lead-implement.md`'s task input mapping and step 4 now describes its
per-dispute verdict-line return, since this phase was already editing that exact
mapping and no other phase owned the gap.

Review: two partitioned cycles, stopped one cycle under budget with two
consecutive clean rounds. Cycle 1 returned 2 Important, both about the second
spec anchor and about the disposition vocabulary now living at four sites with a
guard covering two. One Critical was rejected: a reviewer flagged the
`review-adjudicator` backfill as a plan violation, correctly against the only
authority it had, because the lead omitted the deviation notice from that one
reviewer's prompt. That omission is captured as
`260727-bug-review-findings-file-rewritten-on-disposition`, along with the more
serious thing it exposed — the reviewer withdrew the finding by rewriting its own
findings file, leaving no trace of the disposition in the review record.

The duplication between `implementer-relay` and `implementer-elevated` (51 of 81
non-empty lines) was not refactored onto a shared `includes:` base. Restructuring
two shipped playbooks at the end of the ticket's last phase was not worth the
collateral risk; the hazard is closed instead by a cross-file guard asserting all
four enumeration sites agree, verified to catch every enumeration-layer drift
class including the one no per-file assertion can see. The refactor is captured as
`260727-refactor-implementer-delegate-shared-base`, which also records that the
guard makes the duplication safe and therefore invisible.

Verification: both regen commands in order with `-count=1`, idempotent on a second
run; `go build ./...` and `go test ./... -count=1` green on all 12 packages; the
new dispatch-based coverage exercises `renderPlaybook` with `preferMercenary=true`,
a path nothing in the repo previously called.

## Non-Goals

- Changing what counts as `clean`. `260619` D5 explicitly rejected a machine gate
  ("no Critical/Important == clean") in favor of lead judgment; this ticket adds a
  stopping rule, not an automated verdict.
- Reintroducing the retired `.old/spec/260505` text wholesale.
- Blocking the dashboard FS-watch ticket. That work continues; this ticket only
  records why its Phase 1 review loop ran to the cap without anything noticing.

## Design review history (2026-07-26)

Five rounds ran; the block was lifted in round 2 and the posture is `completed`.
Rounds 2-5 returned `concern` with autonomous findings only, all applied
(`f0a05976`, `82867fb1`, `de32d03e`, `08a9ebd5`, `26694db0`, `e7c42616`). The
round-5 reviewer stated no sixth round was warranted.

### Round 1 — block, and how each item closed

Item 1 was the only `missing` item and the only reason for the `block` verdict.
It is closed: the scope question was whether the elevated-implementer work should
split into a second ticket, and the maintainer settled it as one ticket, on the
grounds that doing one half first makes the second half's authoring
"aware" of it. Phases 2 and 3 are that decision.

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Open scope question is explicitly unresolved and self-declared blocking | critical | missing — closed by maintainer decision, see above |
| 2 | Cycle-counting semantics are ambiguous by a full round | important | autonomous |
| 3 | Phase 2 directs render variables into a surface that is never rendered | important | autonomous |
| 4 | wsflow rsrc tree is generated; ticket directs a hand-edit and omits regeneration | important | autonomous |
| 5 | Adjudicate todo placement contradicts the ordered-runbook contract it relies on | important | autonomous |
| 6 | Phase 1's premise about the untouched fallback branch is false | minor | autonomous |
| 7 | Proposed playbook line fails the invariant checklist the ticket mandates | minor | autonomous |
| 8 | Spec anchor says lead adjudication; Phase 2 makes it a delegate, with no spec update named | minor | autonomous |

Phase numbers in rows 3, 5, and 8 are the round-1 four-phase layout; the
root-cause phase later merged into the elevated-implementer phase, so the ticket
now has three. Items 2-8 are each closed by text in the sections above — the
review-round cycle definition, the plain-Go-strings constraint, the wsflow
regeneration constraint, the rejected separate-todo paragraph, Phase 1's fallback
branch handling, the rejected playbook-invariant paragraph, and the spec
amendments named in Phases 1 and 2.
