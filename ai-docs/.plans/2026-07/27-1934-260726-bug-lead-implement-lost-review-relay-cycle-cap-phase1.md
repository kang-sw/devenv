# Plan: 260726-bug-lead-implement-lost-review-relay-cycle-cap — Phase 1: Restore the budget and the final-cycle behavior

## Relevant Ticket Contract

- One surface only: the generated Instruction. `lead-implement.md` is not
  edited (rejected: a playbook invariant line — it would restate what the
  Instruction states and be inert under the "do not supplement from memory"
  contract).
- **A cycle is a review round, not a relay.** Budget 3 permits 3 reviews and
  therefore 2 relays; budget 2 permits 2 reviews and 1 relay.
- **Budget is per slice, not per partition.**
- **The final cycle completes the run; it does not halt it.** Stop relaying,
  proceed to closeout, and carry unresolved findings with their dispositions
  into the completion report.
- `implementReviewInstruction` (`agents-plugin-tool/internal/mcp/session_state.go`)
  states the budget per branch: `partitioned:` branch states 3 review cycles,
  `single` branch states 2, the fallback branch (bare `"partitioned"`, reached
  via the legacy `enter.implement` path with no `target`) states 3. The
  `lead-only` branch is untouched — it dispatches no reviewers and never
  relays.
- Spec anchor `{#260612-reviewer-allocation-tier-default}` in
  `ai-docs/spec/workflow-skills.md` must be corrected in this phase: "relay
  cap" is the rejected off-by-one-round phrasing (the cap counts reviews, not
  relays), and "caller escalation at cycle 3" contradicts the
  completes-not-halts decision.
- Verification boundary (from the ticket): a `partitioned:` verdict's review
  Instruction names a 3-review-cycle slice budget and the completes-not-halts
  behavior; a `single` verdict's names 2; the fallback branch names 3; a
  `lead-only` verdict's names neither; the spec anchor states the cap in
  review cycles and no longer says the run halts at cycle 3;
  `lead-implement.md` is byte-identical to its pre-phase state in both
  copies.
- Constraint: generated todo Instructions are plain Go strings with no
  template resolution — no `{{...}}` and no render-time tier variables
  (e.g. `{{.LargeTierModel}}`) may appear in them.
- Constraint: touching `agents-plugin-tool/` invokes the dev-merge version
  bump via `agents-plugin-tool/scripts/bump-ws-version.sh` (current version
  `0.36.19` per `agents-plugin/.claude-plugin/plugin.json`).

## Out of Scope

- The adjudicator delegate (`review-adjudicator`), the `[escalate]`,
  `[maintained]`-triggered adjudication clause, and any Cycle-2 adjudication
  wording in the Instruction — Phase 2.
- `[resolved]`/`[unresolved]` tokens, `implementer-elevated`, the capacity
  condition (`[fixed]` then still non-clean), and the root-cause-matching
  condition — Phase 3.
- Any edit under `agents-plugin/rsrc/` (in particular
  `lead-implement.md`, both copies if present) — Phase 1 explicitly rejects a
  playbook invariant line. Because no `rsrc/` file changes, this phase needs
  no `WSRSRC_REGEN`/`WS_REGEN_WSFLOW_RSRC` manifest or wsflow-mirror
  regeneration.
- Changing the spec anchor's "lead adjudication at cycle 2" phrase — that
  wording still accurately describes the pre-Phase-2 state (lead still does
  adjudication; no delegate exists yet) and Phase 2's own verification
  boundary is the one that requires "no longer says lead adjudication".
- The `Dispatch %s reviewers ... Use Review relay and Re-review prompts only
  for genuinely new non-clean Critical/Important findings.` wording-conflict
  fix (relaying a persisting `[unresolved]` finding) — that is Phase 3's fix,
  tied to the capacity condition this phase does not introduce.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_state.go#L563-L577` —
  `implementReviewInstruction`, the sole edit target. Confirmed current line
  numbers match the ticket's citation exactly:
  - L567-569: `lead-only` branch (`isLeadOnlyReview`) — untouched.
  - L570-572: `partitioned:` branch (`strings.HasPrefix(..., "partitioned:")`)
    — add the 3-review-cycle budget and completes-not-halts clause to the
    `fmt.Sprintf` return at L571.
  - L573-575: `single` branch (`strings.EqualFold(..., "single")`) — add the
    2-review-cycle budget and completes-not-halts clause to the return at
    L574.
  - L576: fallback branch (bare default return) — add the same 3-review-cycle
    budget and completes-not-halts clause. Confirmed reachable:
    `parseImplementReviewAlloc` (L445-458) returns the bare string
    `"partitioned"` for `raw == ""` and for `raw == "partitioned"`, neither of
    which matches the `"partitioned:"` prefix check at L570 nor
    `EqualFold(..., "single")` at L573, so it falls through to L576. Reached
    from the legacy `enter.implement` path (`handleEnterImplement`,
    L1042-1067) when `review_alloc` is omitted or passed as the bare word
    `"partitioned"`.
- `agents-plugin-tool/internal/mcp/session_state.go#L445-458` —
  `parseImplementReviewAlloc`, confirms the fallback-branch reachability
  above; no change needed here.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L232-251` —
  `TestDeriveImplementTodoInstructionsPartitionedReview`. Pins on the
  `partitioned:` branch text: `"Dispatch correctness and test reviewers"` and
  `"Reviewer prompt frame"` / `"Review relay and Re-review prompts"`. Update
  to also assert the added 3-review-cycle budget and completes-not-halts
  wording; existing assertions (partition-name substitution, `fit` exclusion)
  must keep passing unchanged.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1836-1868` —
  `TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange`.
  Pins the `single` branch text via `readTodoInstruction(...,"review")`:
  `"Render \`reviewer\`"`, `"one full-scope review"`, `"Reviewer prompt
  frame"`, `"generated findings path"`, `"Relay only new non-clean
  Critical/Important findings"`, plus a negative check that `"reviewers"`
  (plural) is absent. Update to also assert the 2-review-cycle budget and
  completes-not-halts wording; preserve the plural-`"reviewers"` negative
  check (the new text must not accidentally introduce that substring — e.g.
  "review cycles" is safe since it doesn't contain "reviewers", but the
  chosen wording for this branch should be checked against this assertion
  before finalizing).
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1878-1900` —
  `TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs`. Pins the
  `lead-only` branch text (`"Perform lead-owned review only"`, absence of
  `"Reviewer prompt frame"`) — confirms this branch is unaffected and needs
  no change, only re-run to confirm it still passes.
- No existing test currently pins the **fallback** branch's Instruction text
  (only `TestDeriveImplementTodos`, L61-79, checks todo *keys* for a
  `deriveImplementTodos`-derived list, not instruction content). The
  ticket's verification boundary ("the fallback branch names 3") requires a
  new assertion. Add one exercising `implementTodoVerdict{ReviewAlloc:
  "partitioned", ...}` (bare, no colon) directly against
  `deriveImplementTodosFromVerdict`, following the pattern of
  `TestDeriveImplementTodoInstructionsPartitionedReview` (L232-251), and
  assert the 3-review-cycle budget text.
- `ai-docs/spec/workflow-skills.md#L720-722` — the anchor sentence:
  `Relay cap is 2 cycles for single-reviewer, 3 cycles for partitioned with
  lead adjudication at cycle 2 and caller escalation at cycle 3.` immediately
  followed by `{#260612-reviewer-allocation-tier-default}` on its own line.
  Rewrite to state the budget in review cycles (not "relay cap") and to drop
  "caller escalation at cycle 3" in favor of the completes-not-halts
  behavior, while keeping "lead adjudication at cycle 2" (Phase 2's concern,
  not this phase's).
- `ai-docs/spec/workflow-skills.md#L741` — a second, non-anchor reference:
  "...layered over the relay cap as the backstop for the pathological case of
  a reviewer inventing new findings each cycle." This sentence describes
  dedup vs. the cap relationship generally and does not restate the specific
  "relay cap"/"caller escalation" numbers the ticket flags as wrong; left
  as-is unless the phrase "relay cap" itself is judged in-scope for the same
  correction (flagged as a low-stakes judgment call under Escalations, not
  blocking).
- `agents-plugin-tool/scripts/bump-ws-version.sh` and
  `agents-plugin/.claude-plugin/plugin.json` (current `version: 0.36.19`) —
  confirmed the version-bump script exists and is the sole edition point per
  `AGENTS.md`; run it as the last step touching `agents-plugin-tool/`.
- Confirmed via `go build ./...` in `agents-plugin-tool/` that the module
  currently builds cleanly (baseline before edits).

## Implementation Plan

1. Edit `agents-plugin-tool/internal/mcp/session_state.go`, function
   `implementReviewInstruction` (L563-577):
   - `partitioned:` branch (L571): append a clause stating a 3-review-cycle
     budget for the slice and that the final cycle completes the run rather
     than halting it, carrying unresolved findings and dispositions into the
     completion report. Keep the existing `fmt.Sprintf` partition-name
     substitution and the existing "genuinely new non-clean Critical/Important
     findings" phrase unchanged (Phase 3 territory).
   - `single` branch (L574): append the same clause with a 2-review-cycle
     budget. Do not introduce the substring `"reviewers"` (plural) — the
     existing negative-assertion test checks this branch stays singular.
   - Fallback branch (L576): append the same clause with a 3-review-cycle
     budget (mirror the `partitioned:` branch's number, matching its own
     text style since it lacks the partition-name substitution).
   - Do not touch the `lead-only` branch (L567-569).
   - Use capability-vocabulary-safe plain text only — no `{{...}}` template
     syntax, no render-time variables.
2. Update `agents-plugin-tool/internal/mcp/session_state_test.go`:
   - `TestDeriveImplementTodoInstructionsPartitionedReview` (L232-251): add
     assertions for the new 3-review-cycle budget and completes-not-halts
     text on the `partitioned:` branch.
   - `TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange`
     (L1836-1868): add assertions for the new 2-review-cycle budget and
     completes-not-halts text on the `single` branch; confirm the existing
     `"reviewers"` negative check still passes.
   - Add a new small test (or extend an existing table-style test) exercising
     `deriveImplementTodosFromVerdict` with `ReviewAlloc: "partitioned"` (bare,
     no colon) to assert the fallback branch's 3-review-cycle budget text —
     this is new coverage, not an update to an existing pin.
   - Leave `TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs`
     (L1878-1900) unchanged; re-run to confirm the `lead-only` branch is
     unaffected.
3. Edit `ai-docs/spec/workflow-skills.md` at the anchor sentence (L720-722,
   immediately before `{#260612-reviewer-allocation-tier-default}`): replace
   "Relay cap is 2 cycles for single-reviewer, 3 cycles for partitioned with
   lead adjudication at cycle 2 and caller escalation at cycle 3." with
   wording that (a) states the budget in review cycles, not "relay cap" —
   2 for single-reviewer, 3 for partitioned; (b) keeps "lead adjudication at
   cycle 2" (Phase 2 will later change who adjudicates, not this phase); (c)
   replaces "caller escalation at cycle 3" with the completes-not-halts
   behavior — the final cycle completes the run, carrying unresolved findings
   into the completion report, rather than halting for caller escalation.
4. Run `agents-plugin-tool/scripts/bump-ws-version.sh <next-patch>` for the
   dev-merge version bump (current `0.36.19`; confirm the next patch value
   against `ai-docs/_index.md`/release history immediately before running,
   since intervening work on this branch may already have bumped it).
5. Do not touch any file under `agents-plugin/rsrc/` or
   `agents-plugin-wsflow/rsrc/`, and do not run the `WSRSRC_REGEN` /
   `WS_REGEN_WSFLOW_RSRC` regeneration passes — this phase's changes are
   confined to `session_state.go`, its test file, and the spec doc.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run
  'TestDeriveImplementTodoInstructionsPartitionedReview|TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange|TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs|TestDeriveImplementTodos'
  -v` — focused check on the four branches' Instruction text plus the new
  fallback-branch test.
- `cd agents-plugin-tool && go test ./... -count=1` — full package regression
  (confirms no other test incidentally pins the old wording, and that the
  build stays clean after the version bump).
- Manual check: `git diff` scoped to `agents-plugin/rsrc/` and
  `agents-plugin-wsflow/rsrc/` to confirm zero diff (byte-identity of
  `lead-implement.md` in both copies).
- Manual check: re-read the edited spec anchor sentence to confirm it no
  longer contains the strings "Relay cap" or "caller escalation at cycle 3".

## Escalations

- None.

Note for the executor: one low-stakes wording judgment is left open at
`ai-docs/spec/workflow-skills.md#L741` ("...layered over the relay cap as the
backstop...") — a second, non-anchor use of "relay cap" phrasing describing
dedup vs. cap generally, not the specific wrong numbers the ticket flags. The
survey leaves it unedited (see Codebase Findings) since the ticket's
Decisions target the anchor's specific "relay cap ... caller escalation at
cycle 3" sentence, not every use of the word "relay" near "cap" in the spec.
If the executor judges this second instance to read as contradicting the
per-review-cycle framing after L720-722 is fixed, a one-word terminology
touch-up there is a reasonable, low-risk in-phase extension — not an
escalation-worthy decision.
