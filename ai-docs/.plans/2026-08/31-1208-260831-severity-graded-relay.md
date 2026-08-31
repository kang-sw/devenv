# Plan: 260831-refactor-severity-graded-per-slice-review-relay — Phase 1: Replace the uniform relay cap with a severity-graded budget

## Relevant Ticket Contract

- Severity-graded relay budget (replaces the uniform single relay from `260828`,
  commit `4575f634`):
  - **Critical** (must-fix): bounded **3 review rounds** (review #1 + up to 2
    Critical-scoped re-reviews = up to 2 relays); on remaining non-clean,
    **unconditional elevate** to `implementer-elevated` — elevate owns final
    resolution, **no hard stop**, run continues.
  - **Important** (best-effort): at most **1 relay** (spent in relay #1,
    alongside Critical); on remaining non-clean, record
    `[not fixed: <reason>]` in the fix commit `## AI Context`; not blocking,
    not ticketized by default.
  - **Minor** (note only): **0 relays**; recorded in the review summary /
    commit only.
- Relay-round accounting: relay #1 dispositions every non-clean
  Critical/Important finding from review #1 at once. Important's single relay
  is spent there and is **not** re-reviewed — its post-relay `[not fixed]`
  state is the implementer's self-report, not a re-review verdict. Only a
  Critical from review #1 drives the Critical-scoped review #2 and, if still
  non-clean, the second Critical relay + review #3 before the elevate
  ceiling. Minor never drives a relay.
- Ceiling = elevate, not halt: restore the original unconditional-elevate
  behavior; do not define a narrow "cannot-proceed" halt class.
- Disposition durability defaults to the fix commit `## AI Context`, not a
  ticket. Ticketization is a lead-judgment exception, never the default.
- Restore `implementer-elevated` only, reachable **only** at the Critical
  ceiling. `review-adjudicator` stays dormant — its routing machinery is not
  revived (that diet win is preserved).
- Keep `260828`'s shared-clause convergence: `single`, `partitioned:`, and
  bare-`partitioned` share one clause set, differing only in
  reviewer-dispatch wording. `lead-only` stays untouched and relay-vocab-free.
- Preserve the disposition-marker vocabulary (`[fixed]`, `[won't fix: …]`,
  `[deferred: …]`, `[escalate: …]`) and add `[not fixed: …]` for Important;
  do not add a new severity label — `Critical`/`Important`/`Minor` remain the
  per-slice vocabulary.
- Preserve `review_alloc` (risk-based allocation resolver) — this ticket
  changes post-allocation loop behavior only.
- Atomic change: land Critical iteration+elevate and the Important/Minor
  lightening in the same slice — do not split them.
- After any `rsrc` edit, regenerate `agents-plugin-wsflow/rsrc` byte-for-byte
  (`WS_REGEN_WSFLOW_RSRC=1`) and both `manifest.json` hashes
  (`WSRSRC_REGEN=1`).
- The generated review todo `Instruction` (produced by `implementReviewInstruction`
  in `agents-plugin-tool/internal/mcp/session_state.go`) is the binding
  execution surface — editing `lead-implement.md` prose alone is insufficient.

## Out of Scope

- Reviving `review-adjudicator`'s routing/dispatch (stays dormant, unreferenced
  from the generated Instruction and from `lead-implement.md`'s default review
  routing prose).
- Deleting `review-adjudicator`/`implementer-elevated` rsrc files (tracked
  separately per the ticket's Prior Art note).
- Any change to `lead-only`'s review behavior — it must stay relay-vocab-free.
- Any change to `review_alloc` / the risk-based partition resolver itself.
- A bespoke "cannot-proceed" halt class (explicitly rejected in Decisions).
- Uniform bounded-3 for all severities (explicitly rejected in Decisions).
- Any phase beyond Phase 1 (the ticket currently has exactly one phase).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_state.go#L566-L603` — the exact
  binding execution surface. Three consts
  (`implementReviewDispositionClause` L571, `implementReviewRelayClause`
  L579, `implementReviewCriticalBranchClause` L587) are interpolated
  identically into all three review-dispatching branches of
  `implementReviewInstruction` (partitioned L596-598, single L599-601,
  bare-partitioned/fallback L602), differing only in the reviewer-dispatch
  sentence. This is the `260828` shared-clause convergence the ticket
  requires keeping — reuse the same pattern with new/rewritten consts.
- `agents-plugin-tool/internal/mcp/session_state.go#L593-L595` —
  `isLeadOnlyReview` branch of `implementReviewInstruction`; must stay
  byte-for-byte unrelated to the new consts (already relay-vocab-free; do not
  touch).
- `agents-plugin-tool/internal/mcp/session_state.go#L681-L689` —
  `isLeadOnlyReview`/`formatReviewPartitions` helpers; reuse as-is.
- **Prior art for the exact clause shape to restore** (git history, not
  present in the tree today): commit `4575f634`'s parent version of
  `session_state.go` had `implementReviewFinalCycleClause`,
  `implementReviewAdjudicationClause`, and
  `implementReviewElevatedRelayClause` implementing a 3-cycle/2-relay budget
  with capacity/root-cause-triggered elevation and adjudicator arbitration.
  This ticket restores only the *elevation-at-ceiling* shape for Critical
  (not the mid-budget capacity/root-cause trigger, and not the adjudicator
  arbitration — see Decisions "Restore `implementer-elevated`, not
  `review-adjudicator`"). Retrieve via
  `git show 4575f634 -- agents-plugin-tool/internal/mcp/session_state.go`
  for prose style reference only; do not restore the adjudication clause or
  the capacity/root-cause trigger wording.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L28-L34` — "Review"
  Invariants block, including L32
  (`Relay only unresolved Critical/Important findings; record minor findings
  in the review summary.`) — reconcile with the graded model (Critical
  must-fix + bounded iteration + elevate; Important best-effort single relay
  + `[not fixed]` record; Minor recorded only, still no relay).
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L104-L111` — "Delegate
  dispatch" template, step 4 (collect completion report) and step 6 (task
  input mapping). Prior art (`git show 4575f634` on this file) shows the
  exact pre-diet wording for step 6:
  `` `implementer-elevated` gets **Review relay dispatch** when the review
  Instruction's capacity or root-cause condition fired for that relay ``.
  Restore step 6's `implementer-elevated` clause but **rescope the trigger
  condition to the Critical ceiling only** (not the old capacity/root-cause
  wording, which belongs to the mid-budget elevation this ticket does not
  restore). Leave `review-adjudicator` unmentioned (ticket: "leave
  review-adjudicator unreferenced").
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L181-L192` — "Review
  relay dispatch" template. Prior art (same commit) shows the removed
  elevated-target paragraph:
  ```
  When the review Instruction's capacity or root-cause condition fired for this
  relay, render `implementer-elevated` in place of `implementer-relay`, with
  those same declared inputs plus PriorFixCommits and PriorDispositions.
  ```
  Restore this paragraph reworded to fire only "when the Critical ceiling
  fires (review #3 still reports the Critical finding non-clean)".
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L194-L206` — "Re-review
  prompt" template already carries `[resolved]/[unresolved: <short reason>]`
  and `[accepted]/[maintained: <short reason>]` tokens; it is reused verbatim
  for the Critical-scoped review #2 **and** #3 (it is already
  round-number-agnostic — "Review cycle" is a free var). No template rewrite
  needed here beyond the invariants-prose reconciliation mentioned above,
  unless review confirms otherwise.
- `agents-plugin/rsrc/implementer-elevated/implementer-elevated.md` (whole
  file) — its own Doctrine ("The finite resource is the remaining relay
  budget... this dispatch exists because a relay was already spent on a fix
  that did not hold") is already compatible with unconditional unconditional
  elevate-at-ceiling; **no edit needed to this file**.
- `agents-plugin/rsrc/implementer-relay/implementer-relay.md#L46-L62` — the
  delegate that actually authors the fix commit `## AI Context` for relay #1
  (the relay that covers both Critical and Important at once). Its Process
  step 4 / Output block enumerate exactly the four preserved markers
  (`[fixed]`, `[won't fix: …]`, `[deferred: …]`, `[escalate: …]`) and are
  severity-agnostic ("every relayed Critical or Important finding") —
  compatible with the new model as-is *except* for the new `[not fixed: …]`
  marker (see Risk Signal below).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1379-L1497` —
  `fixCycleDispositionTokens()` (L1381-1388) and
  `TestRenderedImplementerDelegatesShareOneDispositionVocabulary`
  (L1437-1497) assert the disposition-marker set is **byte-identical**
  between `implementer-relay` and `implementer-elevated`'s Process/Output
  enumeration sites. Adding `[not fixed: …]` only to `implementer-relay`
  breaks this identity check as currently written (see Risk Signal).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2471-L2543` —
  `TestPlaybookPrintGoldenLeadImplement`: currently forbids
  `` "implementer-elevated` gets **Review relay dispatch**" `` (L2535) and
  `` "When the review Instruction's capacity or root-cause condition fired" ``
  (L2536) as leftover-adjudicator/elevated pins from `260828`. These need to
  move from the forbidden list to the wanted list (reworded for the
  Critical-ceiling-only trigger); the `review-adjudicator`-specific forbidden
  lines (L2537-2538) stay forbidden.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L265-L312` —
  `implementOrdinaryRelayWants()`/`implementOrdinaryRelayForbidden()` pin the
  exact `260828` one-relay clause text and forbid budget/cycle/adjudication
  vocabulary outright. These helpers (and their forbidden list, which
  currently bans `"budget"`, `"cycle 1"`, `"relay at most"`,
  `"implementer-elevated"`) must be rewritten for the severity-graded model —
  the new model legitimately reintroduces bounded-round/relay-count language
  for Critical and the `implementer-elevated` token at the ceiling, so a
  literal narrowing of the old forbidden list (not just new wanted strings)
  is required. Keep forbidding `review-adjudicator` and its arbitration
  vocabulary (`Adjudicate at most once per relay slot`, `[maintained]` as a
  bare adjudication token, `Capacity:`, `Root-cause:`, `Elevated inputs:`,
  `Precedence:`) — none of that routing is restored.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1976-L2008` —
  `TestDeriveImplementTodoInstructionsCriticalReviewBranch` pins the old
  one-relay-then-hard-stop Critical text; rewrite to pin bounded-3-round
  iteration ending in unconditional elevate, and to forbid a hard-stop
  outcome (e.g. forbid "stops the slice from merging" / "does not merge",
  and forbid a review/relay beyond the ceiling).
- `agents-plugin-tool/internal/mcp/session_state_test.go#L2010-L2064` — the
  `lead-only` focused-todos test already forbids `implementer-elevated`,
  `review-adjudicator`, and cycle/budget vocabulary on the `lead-only`
  branch (L2042-2064); this stays correct and should still pass with no
  content change once `lead-only`'s instruction function is confirmed
  untouched — re-verify after the rewrite rather than assuming.
- `ai-docs/spec/workflow-skills.md#L820-L851` — anchor
  `{#260612-reviewer-allocation-tier-default}`. The paragraph at L836-850
  states the one-relay model verbatim ("one repair relay, not a multi-cycle
  budget... a Critical still standing after that review is a hard stop...
  the per-slice loop no longer routes to the `review-adjudicator` or
  `implementer-elevated` delegates"). Rewrite to state the severity-graded
  budget (Critical bounded-3/elevate, Important 1-relay/`[not fixed]`, Minor
  0-relay) and that `implementer-elevated` **is** now reachable at the
  Critical ceiling while `review-adjudicator` stays unreachable.
- `ai-docs/spec/workflow-skills.md#L868-L875` — the
  `{#260619-stateless-implement-review-continuity}` backstop sentence: "while
  the single relay slot — rather than a multi-cycle budget — naturally
  bounds reviewer-invented churn" (part of the dedup-convergence paragraph
  L853-875). Update this clause to reflect the graded/bounded budget instead
  of "single relay slot".
- `ai-docs/mental-model/workflow-skills.md#L51` — `review-adjudicator` bullet;
  update "the one-relay model... has a single relay slot and no adjudication
  slot" framing to the graded model, keeping the core claim (still dormant,
  still resolved via Critical-scoped re-review + now elevate instead of hard
  stop) accurate.
- `ai-docs/mental-model/workflow-skills.md#L89` — `implementer-elevated`
  bullet; currently states "retained in the tree but no longer routed to,
  since the one-relay model removed the capacity/root-cause escalation
  slot". Update to state it is reachable **only** at the Critical ceiling
  under the graded budget (not via the old capacity/root-cause trigger).

## Risk Signal: `[not fixed: <reason>]` marker locus (needs a deliberate,
    documented choice during implementation)

The ticket's Constraints section requires the Important marker
`[not fixed: <reason>]` to land "in the fix commit `## AI Context`" — but per
spec `{#260612-reviewer-allocation-tier-default}`, that commit is authored by
the delegate that runs relay #1 (`implementer-relay`), not the lead. That
delegate's own disposition vocabulary is hardcoded in its own rsrc file
(`agents-plugin/rsrc/implementer-relay/implementer-relay.md#L51,58-62`), which
the generated review Instruction (`session_state.go`) does not reach into —
the Instruction only governs lead-side dispatch/counting, not the spawned
implementer's own marker choices. So the marker most likely needs to be added
to `implementer-relay.md`'s own Process/Output enumeration, scoped to
Important findings only (Critical is must-fix and never gets "not fixed";
Minor is never relayed).

`implementer-elevated.md` should **not** gain this marker: under this ticket
`implementer-elevated` is reachable only at the Critical ceiling, and Critical
never gets a "not fixed" disposition (it elevates instead). This creates a
deliberate asymmetry between the two delegates' disposition vocabularies,
which collides with an existing anti-drift guard:
`TestRenderedImplementerDelegatesShareOneDispositionVocabulary`
(`playbook_tools_test.go#L1437-1497`) currently asserts the two files'
enumeration sites are byte-identical, and
`ai-docs/mental-model/prompt-bundle.md#L96` documents that identity as a
guarded invariant ("Editing the disposition-token vocabulary... in only one
of `implementer-relay`/`implementer-elevated`" is called out as the drift
class the test exists to catch).

**Recommended resolution** (apply unless implementation turns up a cleaner
path): add `[not fixed: <reason>]` to `implementer-relay.md` only, scoped
explicitly to Important findings in both the Process step and Output list;
do not add it to `implementer-elevated.md`. Split
`TestRenderedImplementerDelegatesShareOneDispositionVocabulary`'s check into
(a) the four core markers, asserted identical across both files as today, and
(b) `[not fixed: <reason>]`, asserted present-once in `implementer-relay`'s
Process/Output and **absent** from `implementer-elevated`'s. Update the
`prompt-bundle.md#L96` pitfall bullet to document this as a deliberate,
severity-scoped asymmetry (not drift), so a future editor does not
"fix" it back to identical. This is a judgment call the ticket text does not
spell out at the file level — flagging it explicitly rather than silently
picking a path, per the survey's evidence-before-claims discipline.

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/session_state.go` (L566-603): replace the
   three shared consts with a severity-graded set, keeping the existing
   three-branch dispatch shape (partitioned/single/bare-partitioned share the
   clause set, differing only in reviewer-dispatch wording):
   - Extend `implementReviewDispositionClause` (or keep as-is and add a
     sibling clause) to state the marker set is `[fixed]`,
     `[won't fix: <reason>]`, `[deferred: <reason>]`, `[escalate: <reason>]`
     for Critical relay #1 dispositions, plus `[not fixed: <reason>]` as the
     Important-only self-reported non-resolution marker.
   - Rewrite `implementReviewRelayClause` to state relay #1 dispositions
     every non-clean Critical/Important finding from review #1 at once, and
     that Important's one-relay budget is spent there (never re-reviewed).
   - Add a Minor clause stating Minor findings drive no relay and are
     recorded in the review summary only.
   - Rewrite `implementReviewCriticalBranchClause` to state: if review #1
     reports any Critical finding, follow relay #1 with a Critical-scoped
     review #2 (Re-review prompt); if still non-clean, relay #2
     (Critical-scoped) then a Critical-scoped review #3; if still non-clean
     after review #3, unconditionally elevate to `implementer-elevated` (no
     hard stop) and continue to the remaining todos — do not schedule a 4th
     review or 3rd relay.
   - Update `implementReviewInstruction`'s three branches (L596-602) to
     interpolate the new/rewritten consts, preserving the existing
     reviewer-dispatch-only differentiation.
2. `agents-plugin/rsrc/lead-implement/lead-implement.md`:
   - L28-34 Review invariants: reconcile with the graded model (state
     Critical/Important/Minor budgets and that the Critical ceiling elevates
     rather than halts).
   - L104-111 Delegate dispatch: restore step 6's `implementer-elevated`
     mapping, rescoped to fire "when the Critical ceiling is reached" (not
     the old capacity/root-cause wording); leave `review-adjudicator`
     unmentioned per the ticket.
   - L181-192 Review relay dispatch template: restore the elevated-target
     paragraph, rescoped to the Critical-ceiling trigger, with
     `PriorFixCommits`/`PriorDispositions` as additional declared inputs
     (mirroring `implementer-elevated.md`'s existing variable list).
   - L194-206 Re-review prompt: confirm it needs no structural change (it is
     already round-number-agnostic); adjust only if the invariants
     reconciliation surfaces a gap.
3. `agents-plugin/rsrc/implementer-relay/implementer-relay.md`: add
   `[not fixed: <reason>]` to Process step 4 (L51) and the Output disposition
   list (L58-62), scoped to Important findings only, per the Risk Signal's
   recommended resolution — or apply an equally explicit alternative if one
   is found, and update the corresponding test/doc together with it (do not
   change vocabulary at one site without the others).
4. Regenerate: from `agents-plugin-tool/`,
   `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
   (regenerates `agents-plugin/rsrc/manifest.json`), then
   `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   (syncs `agents-plugin-wsflow/rsrc/` byte-for-byte, including its
   `manifest.json`). Both `-count=1` flags are mandatory (env-gated regen
   tests are cached otherwise).
5. `ai-docs/spec/workflow-skills.md`:
   - Rewrite `{#260612-reviewer-allocation-tier-default}` (L836-851) to state
     the severity-graded budget and that `implementer-elevated` is reachable
     at the Critical ceiling while `review-adjudicator` stays unreachable.
   - Update the `{#260619-stateless-implement-review-continuity}` backstop
     sentence (L868-875, "single relay slot... naturally bounds
     reviewer-invented churn") to reflect the graded/bounded budget.
6. `ai-docs/mental-model/workflow-skills.md`: update the `review-adjudicator`
   bullet (L51) and `implementer-elevated` bullet (L89) per the Codebase
   Findings above.
7. If step 3 is applied: update `ai-docs/mental-model/prompt-bundle.md#L96`
   to document the `implementer-relay`/`implementer-elevated` vocabulary
   asymmetry as deliberate and severity-scoped, not drift.
8. `agents-plugin-tool/internal/mcp/session_state_test.go`:
   - Rewrite `implementOrdinaryRelayWants()`/`implementOrdinaryRelayForbidden()`
     (L265-312) for the new clause text; narrow the forbidden list to keep
     banning only `review-adjudicator` arbitration vocabulary
     (`Adjudicate at most once per relay slot`, `Capacity:`, `Root-cause:`,
     `Elevated inputs:`, `Precedence:`, bare `[maintained]` as an
     adjudication verdict token) while allowing the reintroduced
     round/relay-count and `implementer-elevated` language.
   - Rewrite `TestDeriveImplementTodoInstructionsCriticalReviewBranch`
     (L1976-2008) to pin bounded-3-round iteration ending in unconditional
     elevate; add a forbidden pin against a hard-stop outcome.
   - Re-verify (not just assume) the `lead-only` forbidden pins (L2010-2064)
     still hold unchanged against the rewritten instruction functions.
9. `agents-plugin-tool/internal/mcp/playbook_tools_test.go`:
   - Move `` "implementer-elevated` gets **Review relay dispatch**" `` off
     `TestPlaybookPrintGoldenLeadImplement`'s forbidden list (L2534-2543)
     into a wanted assertion reworded for the Critical-ceiling trigger; keep
     the `review-adjudicator`-specific lines forbidden.
   - If step 3 is applied, update `fixCycleDispositionTokens()` /
     `TestRenderedImplementerDelegatesShareOneDispositionVocabulary`
     (L1379-1497) to check the four core markers identically across both
     delegates plus `[not fixed: <reason>]` present-once in
     `implementer-relay` only and absent from `implementer-elevated`.
10. Run full verification (step 11) and fix any drift the rewritten
    Instruction text surfaces in other golden/forbidden assertions not
    enumerated above (grep the two test files for any other literal
    `implementOrdinaryRelayWants`/`implementOrdinaryRelayForbidden` call
    sites before finishing).

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./... && go test ./...`
  — must be green, in particular
  `TestDeriveImplementTodoInstructionsPartitionedReview`,
  `TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback`,
  the single-review test around L1958-1974,
  `TestDeriveImplementTodoInstructionsCriticalReviewBranch`,
  `TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs` (lead-only pins),
  `TestPlaybookPrintGoldenLeadImplement`, and
  `TestRenderedImplementerDelegatesShareOneDispositionVocabulary`.
- `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`,
  then re-run `go test ./...` to confirm the regenerated manifests/mirror pass
  their drift guards.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — must pass
  (wsflow package tests per `ai-docs/manuals/wsflow-mirroring.md`).
- Manual read-through of the final `implementReviewInstruction` output for
  each of `single`, `partitioned: correctness, test`, and bare `partitioned`
  to confirm: Critical drives up to 2 relays across 3 review rounds then
  elevates (never a hard stop); Important gets at most 1 relay then a
  `[not fixed: <reason>]` record; Minor drives 0 relays; the marker
  vocabulary is exactly the preserved four plus `[not fixed: …]`; no
  auto-ticketization language appears; `lead-only`'s instruction is
  unchanged and still relay-vocab-free; and the three relaying allocations
  share the same clause set (diff only in reviewer-dispatch wording).
- `git diff --stat -- agents-plugin/rsrc agents-plugin-wsflow/rsrc` should
  show the wsflow mirror changed in lockstep with the canonical rsrc edits,
  with no unexpected files touched.

## Escalations

- None.
