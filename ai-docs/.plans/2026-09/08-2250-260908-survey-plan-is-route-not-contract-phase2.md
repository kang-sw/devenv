# Plan: 260908-feat-survey-plan-is-route-not-contract — Phase 2: Reviewer frame without the plan, and wrap-up deviations

## Relevant Ticket Contract
- `ai-docs/tickets/ready/260908-feat-survey-plan-is-route-not-contract.md` — Phase 2: Reviewer frame without the plan, and wrap-up deviations

## Out of Scope
- Phase 1 (landed, `### Result (3a8c517f)`): implementer/relay/elevated ticket-read-ban removal, plan-populator contract-free templates, Decision 6 lead-adjudication window. Do not redo or re-verify beyond what Phase 2 depends on.
- `review-adjudicator`: still declares `PlanPath`; explicitly out of scope per ticket Constraints, tracked by `260831-chore-remove-orphaned-review-adjudicator-delegate`.
- Sage reviewers on tickets and review allocation counts (explicitly out of scope per ticket Constraints).
- `plan-populator-survey` / `plan-populator-research` template and rule text — Phase 1's surface; Phase 2 does not touch these files.
- `code-review-correctness/fit/test.md` checklist bodies beyond a plan mention — survey confirmed none of the three partition files mention "plan" anywhere, so no edits land there (see Codebase Findings).
- Any spec anchor other than `{#260505-implementation-workflow-skills}` and `{#260619-stateless-implement-review-continuity}` (e.g. `{#260512-skeleton-draft-and-final-commits}`, `{#260513-check-blockers-skill}`) — not named by this phase.
- `implementer` / `implementer-relay` / `implementer-elevated` bodies — the implementer keeps the plan as input (Decision 4 only removes the plan from *reviewer* input); do not touch these three files.

## Codebase Findings

- `agents-plugin/rsrc/lead-implement/lead-implement.md#L218-225` — `### Reviewer table`. `Required check` column names "plan" in all three partition rows (Correctness: "Supplied authority, **plan**, and correctness invariants"; Fit: "Supplied authority decisions, **plan guardrails**, and future-phase fit"; Test: "Supplied authority and **plan** verification coverage"). The `Full scope` row already omits plan and needs no change. No Go test pins the Correctness/Fit/Test row text (only `TestPlaybookPrintGoldenLeadImplement` pins the `Full scope` row, confirmed by grep for `Supplied authority` / `plan guardrails` / `plan verification coverage` across `agents-plugin-tool/internal/mcp/*.go` — zero hits), so these three rows can be edited freely.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L227-254` — `### Reviewer prompt frame`. Contains the `Plan path: <plan-path>` line (L236), the two plan checks "Review the supplied authority, plan contract, and diff together." and "Plan guardrails were not bypassed." (L244-245), and the two-variant split `**Generated plan:**` (L229) / `**Direct edit with no generated plan:**` (L254). This whole block must become one unified frame per Decision 4, gaining a `Selected phase:` line and a Review-focus rule for lead-authorized deferrals.
- `agents-plugin/rsrc/code-reviewer.md#L9` — Constraints line: `When a plan path is named, review authority, plan, and diff together; otherwise review the direct edit without requiring a plan.` Must be removed (reviewers never receive a plan now).
- `agents-plugin/rsrc/code-reviewer.md#L15,17` — Process steps 1 and 3: step 1 says "then only named authority/**plan** artifacts and domain docs..."; step 3 says "Read the ticket or inline contract and any **plan path** named by the prompt frame...". Both must drop the plan reference. Step 3 is the one directly named by the phase text ("the process step that reads the plan"); step 1 is an adjacent consistency fix the phase text does not name explicitly but Decision 4 requires (reviewers no longer see any plan artifact) — low risk, same file, no test currently pins it.
- `agents-plugin/rsrc/code-review-correctness/code-review-correctness.md`, `agents-plugin/rsrc/code-review-fit/code-review-fit.md`, `agents-plugin/rsrc/code-review-test/code-review-test.md` — confirmed via `grep -n "plan"` on all three: **zero matches**. None of the partition playbooks name the plan anywhere, so "the partition playbooks only where they name the plan" resolves to *no edits* in these three files.
- `agents-plugin/rsrc/executor-wrapup.md#L38-40` — `## Ticket Update` intro sentence: `...include deviations, verification evidence, unresolved findings, and deferred follow-ups; do not restate unchanged plan or spec content.` "deviations" is currently undefined (implicitly implementer-recalled); Decision 5 requires it be defined as a ticket-phase-vs-landed diff.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2701-2838` (`TestPlaybookPrintGoldenLeadImplement`) — the golden render test for `lead-implement`. Want-list entries at L2736 (`"Generated plan:"`), L2740 (`"Direct edit with no generated plan:"`), L2742 (`"Review the supplied authority, plan contract, and diff together."`), L2743 (`"without a plan artifact"`) pin exactly the text being removed; L2737-2739 (`"Authority: Ticket path <ticket-path>"`, `"Authority: Inline contract ..."`, `"Each specified authority requirement is implemented, or carries an explicit, authorized deferral."`) pin text that must survive unchanged.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2874-2890` (`TestShippedExecutorWrapupResultIncludesBehavioralDelta`) — pins the exact `executor-wrapup.md` Result/Edition sentence verbatim (including its literal line-wrap `\n` positions inside the Go string). Must be updated in lockstep with the `.md` edit, byte-for-byte.
- `agents-plugin-tool/internal/mcp/mercenary_surface_test.go#L637-666` (`TestRenderGoldenShippedReviewPartitionIncludesBase`) — a **second** golden-test site (not found by searching only `playbook_tools_test.go`) that pins `code-reviewer.md` verbatim: `"When a plan path is named, review authority, plan, and diff together; otherwise review the direct edit without requiring a plan."` (must be dropped from the want-list) and `"Read the ticket or inline contract and any plan path named by the prompt frame; never require a ticket for inline authority."` (must be updated to the new step-3 wording). This test renders `code-review-correctness/fit/test` and asserts the *included* `code-reviewer` base text appears — so it double-covers the `code-reviewer.md` edit and must be updated alongside `TestPlaybookPrintGoldenLeadImplement`.
- `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`, `agents-plugin-wsflow/rsrc/code-reviewer.md`, `agents-plugin-wsflow/rsrc/executor-wrapup.md` — confirmed byte-identical to their `agents-plugin/rsrc/` counterparts as of survey time (`diff` clean). Per `ai-docs/manuals/wsflow-mirroring.md#L263-271`, any canonical rsrc edit requires the two-step regen below; edits must NOT be hand-applied to the wsflow copies.
- `ai-docs/spec/workflow-skills.md#L738` — `{#260512-skeleton-inside-implement-branch}` section (a heading *before* `{#260505}`, not inside it): `"...and reviewers compare the ticket, plan, and diff together."` This is "the `{#260512}` reviewer clause" the ticket Constraints names for Phase 2 (the plan-carries-contract/implementer clauses in the same paragraph were already fixed in Phase 1 — confirmed present at L730-737: "the generated plan routes to the relevant ticket contract..." / "Implementers treat the ticket's selected phase as the execution contract and the plan as the route through it").
- `ai-docs/spec/workflow-skills.md#L952-953` — inside `{#260505-implementation-workflow-skills}` (spans L745-1029; confirmed via heading scan — `{#260619}` at L901 is an inline trailing marker, not a new `##` heading, so L945-965 still belongs to `{#260505}`): `"...reviewers compare the implementation against the selected authority, plan, and diff to catch implementation-time shortcut drift..."`.
- `ai-docs/spec/workflow-skills.md#L962-965` — same `{#260505}` section: `"In ticket-driven runs, reviewers read the ticket and plan, then treat any specified authority requirement that is not implemented and does not carry an explicit, authorized deferral as a blocking finding within their assigned partitions."` Needs the plan reference dropped and the phase/Decisions/Constraints scoping and lead-authorization language folded in per Decision 4.
- `ai-docs/spec/workflow-skills.md#L871-874` — `{#260619-stateless-implement-review-continuity}` (anchor at L901, section runs L830-901 by heading scan): `"...each implementer and reviewer dispatch is fed entirely by its relay prompt plus the self-contained artifact set (plan, review findings, committed diff)..."` — this is the artifact-set sentence Decision 4 falsifies for reviewers; the implementer side keeps the plan (Phase 1 did not remove it, only added ticket-reading), so the fix differentiates implementer vs. reviewer artifact sets rather than dropping "plan" outright.
- No mental-model file (`ai-docs/mental-model/*.md`) or manual (`ai-docs/manuals/*.md`) mentions "plan guardrails", "Plan path", "reviewers compare", or "plan contract" (confirmed via grep) — Phase 2 has no mental-model or manual doc surface, only the two named spec anchors.
- `ai-docs/manuals/wsflow-mirroring.md#L263-268` — the exact two-command after-edit checklist for the rsrc regen + wsflow mirror sync, needed verbatim for the Verification Plan.

## Implementation Plan

1. **`agents-plugin/rsrc/lead-implement/lead-implement.md`** — Reviewer table (`### Reviewer table`, current L220-225): drop "plan"/"plan guardrails" from the Correctness, Fit, and Test rows' Required-check column, keep the Full-scope row unchanged:
   - Correctness: `Supplied authority and correctness invariants`
   - Fit: `Supplied authority decisions and future-phase fit as a compatibility consideration only, never an unimplemented requirement`
   - Test: `Supplied authority's verification expectations`

2. **`agents-plugin/rsrc/lead-implement/lead-implement.md`** — replace the whole `### Reviewer prompt frame` block (current L227-254) with one unified frame (no `Generated plan:` / `Direct edit with no generated plan:` split, no `Plan path` line, a new `Selected phase` line for ticket targets, and a Review-focus deferral rule):
   ```text
   ### Reviewer prompt frame

   Choose exactly one authority line; no plan artifact is dispatched to the reviewer.
   ```text
   Read First: <rendered reviewer playbook path>
   Review scope: <Full scope: correctness, fit, test|Correctness|Fit|Test>
   Diff range: <parent-of-first-commit>..<last-commit>
   Authority: Ticket path <ticket-path>
   Authority: Inline contract <accepted scope, constraints, non-goals, verification boundary>
   Selected phase: <phase heading> (ticket targets only; omit for inline authority)
   Findings path: <review-output-path>

   Review focus:
   - <2-4 scope-specific risks>
   - <any lead-authorized deferral or scope reduction, named explicitly>

   Required checks:
   - <required check from the Reviewer table>
   - The selected phase plus the ticket's `## Decisions` and `## Constraints` are the requirement set; later phases are out of scope, not unimplemented requirements.
   - Each specified authority requirement is implemented, or carries an explicit, authorized deferral.
   - An authorized deferral or scope reduction not named in Review focus is a finding.

   Instructions:
   - For inline authority, do not read or require a ticket path or Selected phase.
   - For a partition, ignore outside it unless directly broken by the diff.
   - Write detailed findings to the findings path.
   ```
   ```
   Keep the literal substring `Each specified authority requirement is implemented, or carries an explicit, authorized deferral.` unchanged (it is pinned by the golden test and must survive verbatim).

3. **`agents-plugin/rsrc/code-reviewer.md`** — Constraints (current L9): delete the line `When a plan path is named, review authority, plan, and diff together; otherwise review the direct edit without requiring a plan.` entirely (no replacement needed).

4. **`agents-plugin/rsrc/code-reviewer.md`** — Process step 3 (current L17): change
   `Read the ticket or inline contract and any plan path named by the prompt frame; never require a ticket for inline authority.`
   to
   `Read the ticket or inline contract named by the prompt frame; never require a ticket for inline authority.`

5. **`agents-plugin/rsrc/code-reviewer.md`** — Process step 1 (current L15), consistency fix: change
   `Read the root context for repository invariants, then only named authority/plan artifacts and domain docs relevant to changed paths.`
   to
   `Read the root context for repository invariants, then only the named authority and domain docs relevant to changed paths.`

6. **`agents-plugin/rsrc/executor-wrapup.md`** — `## Ticket Update` intro sentence (current L38-40): change the "deviations" clause to define it as a ticket-phase-vs-landed diff, not implementer recall. Suggested exact wording (must match whatever is used in step 8's Go-test edit byte-for-byte, including line wraps):
   ```text
   `Result` records the completed phase's behavioral delta; `Edition` records only
   its follow-up pass's delta. For either, include deviations — diffed between the
   ticket's selected phase text and what landed, not recalled from the implementer —
   verification evidence, unresolved findings, and deferred follow-ups; do not
   restate unchanged plan or spec content.
   ```

7. **Regenerate the rsrc manifest and wsflow mirror** (required after steps 1-6; run from `agents-plugin-tool/`):
   ```bash
   WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   Confirm `agents-plugin-wsflow/rsrc/` is byte-identical to `agents-plugin/rsrc/` afterward (e.g. `diff -rq agents-plugin/rsrc agents-plugin-wsflow/rsrc`, ignoring the launcher-only files the mirroring manual already documents as exceptions, if any surface).

8. **`agents-plugin-tool/internal/mcp/playbook_tools_test.go`** — `TestPlaybookPrintGoldenLeadImplement` (current L2701-2838):
   - Remove from the want-list: `"Generated plan:"` (L2736), `"Direct edit with no generated plan:"` (L2740), `"Review the supplied authority, plan contract, and diff together."` (L2742), `"without a plan artifact"` (L2743).
   - Add to the want-list: `"Selected phase: <phase heading>"`, `"An authorized deferral or scope reduction not named in Review focus is a finding."`, and the reworded Fit/Test/Correctness row strings from step 1 if desired for direct coverage (optional — not currently pinned, but recommended so the row edit has a regression guard).
   - Add to the forbidden-list (near the existing forbidden blocks, e.g. after L2814 or alongside L2791-2804): `"Plan path:"`, `"Plan guardrails were not bypassed."`, `"Generated plan:"`, `"Direct edit with no generated plan:"`, `"Review the supplied authority, plan contract, and diff together."`.
   - Keep `"Authority: Ticket path <ticket-path>"`, `"Authority: Inline contract <accepted scope, constraints, non-goals, verification boundary>"`, and `"Each specified authority requirement is implemented, or carries an explicit, authorized deferral."` unchanged in the want-list.

9. **`agents-plugin-tool/internal/mcp/playbook_tools_test.go`** — `TestShippedExecutorWrapupResultIncludesBehavioralDelta` (current L2874-2890): update the pinned multi-line want string at L2885 to match the exact new `executor-wrapup.md` wording from step 6, preserving the same embedded `\n` line-break positions the Go string literal encodes (the match is a literal substring against file bytes, so the `.md` line wraps and the Go string's `\n` placements must agree exactly).

10. **`agents-plugin-tool/internal/mcp/mercenary_surface_test.go`** — `TestRenderGoldenShippedReviewPartitionIncludesBase` (current L637-666): in the want-list at L656-657, remove `"When a plan path is named, review authority, plan, and diff together; otherwise review the direct edit without requiring a plan."` and change `"Read the ticket or inline contract and any plan path named by the prompt frame; never require a ticket for inline authority."` to `"Read the ticket or inline contract named by the prompt frame; never require a ticket for inline authority."` (must match step 4's exact wording).

11. **`ai-docs/spec/workflow-skills.md#L738`** — inside `{#260512-skeleton-inside-implement-branch}`: change `"...Implementers treat the ticket's selected phase as the execution contract and the plan as the route through it, and reviewers compare the ticket, plan, and diff together."` to end `"...and reviewers compare the ticket and diff together."` (drop `, plan,`).

12. **`ai-docs/spec/workflow-skills.md#L952-953`** — inside `{#260505-implementation-workflow-skills}`: change `"...reviewers compare the implementation against the selected authority, plan, and diff to catch implementation-time shortcut drift..."` to `"...reviewers compare the implementation against the selected authority and diff to catch implementation-time shortcut drift..."` (drop `, plan,`).

13. **`ai-docs/spec/workflow-skills.md#L962-965`** — inside `{#260505-implementation-workflow-skills}`: replace `"In ticket-driven runs, reviewers read the ticket and plan, then treat any specified authority requirement that is not implemented and does not carry an explicit, authorized deferral as a blocking finding within their assigned partitions."` with wording that (a) drops the plan read, (b) scopes the requirement set to the selected phase plus `## Decisions`/`## Constraints` with later phases out of scope, and (c) ties "authorized" to a lead-written Review-focus entry, e.g.:
    ```text
    In ticket-driven runs, reviewers read the ticket's selected phase, `##
    Decisions`, and `## Constraints` — not the plan — treating later phases as
    out of scope, and treat any specified authority requirement that is not
    implemented and does not carry an explicit, lead-authorized deferral named
    in the reviewer prompt's Review focus lines as a blocking finding within
    their assigned partitions.
    ```

14. **`ai-docs/spec/workflow-skills.md#L871-874`** — inside `{#260619-stateless-implement-review-continuity}`: the artifact-set sentence currently reads `"...each implementer and reviewer dispatch is fed entirely by its relay prompt plus the self-contained artifact set (plan, review findings, committed diff)..."`. Since the implementer side still gets the plan (Phase 1 did not remove it) and only the reviewer side loses it, differentiate rather than blanket-drop "plan", e.g.:
    ```text
    Delegates in the review fix-loop are stateless by contract: each dispatch is
    fed entirely by its relay prompt plus a self-contained artifact set — plan,
    review findings, and committed diff for the implementer; ticket or inline
    authority, review findings, and committed diff for the reviewer — and the
    loop stays correct when every cycle is a fresh spawn.
    ```

15. Commit spec changes separately from the rsrc/test changes per `AGENTS.md` commit conventions (spec-only commit vs. implementation commit), unless the review flow for this project prefers a single logical commit — follow whatever `lead-implement`'s own Documentation stage does for Phase 1's equivalent split (Phase 1's Result mentions spec and mental-model updates as part of the same landed range, so a single commit is also acceptable if that was the established pattern).

## Verification Plan
- `cd agents-plugin-tool && go test ./... -count=1` — full suite, twice if the project's own convention (seen in Phase 1's Result) runs it twice for cache-flake assurance.
- Targeted runs while iterating: `go test ./internal/mcp -run TestPlaybookPrintGoldenLeadImplement -count=1`, `go test ./internal/mcp -run TestShippedExecutorWrapupResultIncludesBehavioralDelta -count=1`, `go test ./internal/mcp -run TestRenderGoldenShippedReviewPartitionIncludesBase -count=1`.
- `diff -rq agents-plugin/rsrc agents-plugin-wsflow/rsrc` after the two regen commands in Implementation step 7 — must report no differences.
- `claude plugin validate agents-plugin` (repo convention, per Phase 1's Result).
- Manual render check: render `lead-implement` and confirm the printed body contains no `Plan path:` line, no `Plan guardrails were not bypassed.` text, exactly one reviewer-frame variant (no `Generated plan:`/`Direct edit with no generated plan:` split), and does contain a `Selected phase:` line.
- One partitioned review dry run on a ticket target (per the ticket's own Phase 2 verification line: "one partitioned review on a ticket target completes with ticket plus diff only") — confirm the dispatched reviewer prompt has no plan path and the reviewer's findings reference only the ticket and diff.

## Escalations
- None.
