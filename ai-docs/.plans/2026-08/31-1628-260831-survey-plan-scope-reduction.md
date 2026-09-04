# Plan: 260831-bug-survey-plan-unilateral-scope-reduction — Phase 1: Forbid unilateral down-scoping in the survey/research planners and add a reviewer-frame coverage check

## Relevant Ticket Contract

- A fully-specified, multi-part requirement must be carried whole into the plan; the planner (survey and research) may not silently implement a subset.
- A confident planner decision to build only a subset is a **scope-reduction decision for the lead**, surfaced through an explicit escalation signal *distinct from* `[escalate-to-research]`. Model it on the research delegate's existing `[escalate-to-lead]` channel, extended to the survey delegate and to the confident-subset case (exact token/wording is an implementation choice, but it must stay separate from `[escalate-to-research]`).
- A "first cut" / phased subset is legitimate only when the ticket or lead already authorized the phasing — never when the planner invents it.
- Distinguish, in both planner prompts, a **runtime fallback** (required execution branch / graceful degradation — must be built) from an **implementation fallback** (scope shortcut / temporary path the planner may not take unilaterally); the existing "shortcut-risk signals" list must not read as licensing omission of a ticket's required runtime branches.
- Operationalize `agents-plugin/rsrc/lead-implement/lead-implement.md`'s existing "Binding authority decisions were not omitted or violated" reviewer-frame line into a concrete obligation: each specified authority requirement is either implemented or carries an explicit, authorized deferral. Phrase it against "authority," not "ticket," so it binds both `Authority: Ticket path` and `Authority: Inline contract` reviewer-frame modes.
- Constraints: apply the wsflow mirror + manifest regen (`ai-docs/manuals/wsflow-mirroring.md`) for the `agents-plugin/rsrc/` edits; do not fork per-branch copies of the reviewer coverage line if binding Go execution text references it; do not widen `[escalate-to-research]`'s meaning.
- Deliverable: update the `{#260505-implementation-workflow-skills}` anchor in `ai-docs/spec/workflow-skills.md` to record the new escalation signal and the reviewer coverage obligation; reconcile the `workflow-skills` mental-model on contact.
- Verification boundary: delegate-prompt rendered/golden tests assert the down-scope-forbidden rule text, the runtime-vs-implementation-fallback distinction, and that `[escalate-to-research]` stays uncertainty-scoped; a reviewer-frame rendered test asserts the concrete per-requirement coverage obligation; wsflow mirror byte-identity + manifest-hash tests green; full Go suite green.

## Out of Scope

- A new lead adjudication checkpoint between prep and edit (rejected in Decisions).
- Structural modality tagging in the plan-contract template (deferred lever).
- Ticket-authoring-side over-elaboration or per-clause consent gating.
- `260729-bug-survey-plan-drops-verbatim-contract-text` (linked sibling, different mechanism/fix — do not touch).
- Any change to review allocation, severity budget, or the per-slice relay mechanics from `260831-refactor-severity-graded-per-slice-review-relay`.

## Codebase Findings

- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L32-L43` — `## Rules` block; add the down-scope-forbidden rule and the new escalation signal here. No existing rule addresses scope fidelity.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L68-L70` — "Shortcut risk signals" bullet lists "fallback behavior" and "temporary implementation paths" as droppable-shortcut signals with no runtime-fallback distinction; this is the homonym trap named in the ticket's Background.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L128-L139` — `### 4. Report` section; currently only `[ok]` or `[escalate-to-research]`. Add the new lead-directed signal and an escalation-rationale bullet for it, mirroring the existing `[escalate-to-research]` bullet.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L27-L42` — `## Rules`; L36-37 "Do not encode temporary, fallback, mock-data, or duplicated-glue behavior as the implementation path" and L38 "Escalate when the accepted target cannot be satisfied without a questionable shortcut" need the same runtime-vs-implementation-fallback clarification and a broadened escalation trigger (confident scope reduction, not just "no clean plan exists").
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L74-L76` — "Identify" bullet repeats the same unqualified "fallback behavior" term; needs the same clarification.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L99-L107` — `### 5. Report`; already has `[ok]` or `[escalate-to-lead]` (the channel the ticket says to model/extend) — only the *semantics* of when to use it need broadening (add confident-subset case), the token itself is unchanged.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L169-L179` — Reviewer prompt frame `Required checks:` block; L173 is the exact line to operationalize: `- Binding authority decisions were not omitted or violated.` The frame already supports both authority modes at L161-162 (`Authority: Ticket path <ticket-path>` / `Authority: Inline contract <accepted scope, constraints, non-goals, verification boundary>`), so phrasing the new obligation against "authority" (not "ticket") is a same-line rewording, not a structural change.
- `agents-plugin-tool/internal/mcp/session_state.go` — grepped for "Binding authority", "omitted or violated", "Required checks:" — **no matches**. The only "Reviewer prompt frame" references in Go (`session_state.go:602,605,607`) are instruction-text pointers to the template by name, not copies of its content. The ticket's shared-clause-convergence constraint is therefore satisfied by leaving Go untouched; only the rsrc file needs editing.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1716-L1740` (`TestRenderPlaybookLegacyContext...`, wsflow legacy render) and `#L1782-L1876` (`TestRenderPlaybookFullWsPlannerContext`) — existing golden assertions on survey/research render output. L1829 asserts the literal substring `` "[ok]` or `[escalate-to-research]`" `` for survey and L1834 asserts `` "[ok]` or `[escalate-to-lead]`" `` for research. Changing the survey Report bullet to a three-signal line breaks L1829's exact substring match — that assertion must be updated in the same change, not left to drift.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2536-L2658` (`TestPlaybookPrintGoldenLeadImplement`) — the golden render test for `lead-implement`; already asserts several `Reviewer prompt frame` fragments (L2569-2571) but nothing for the "Binding authority" line yet. This is the natural place to add the new coverage-obligation assertion per the ticket's verification requirement — no other Go test currently touches that line's text (confirmed via grep, zero hits repo-wide before this change).
- `ai-docs/spec/workflow-skills.md#L893-L921` (`{#260505-implementation-workflow-skills}` anchor, plan-population subsection) — the paragraph to update. L899-905 describes survey's `[ok]`/`[escalate-to-research]` exit signals (needs the new signal added); L928-931 already states "reviewers ... treat selected-scope binding decisions omitted from the plan or violated by the implementation as blocking findings within their assigned partitions" — this is the spec-level counterpart of the reviewer-frame line and should be tightened to the same "each specified authority requirement is implemented, or carries an explicit, authorized deferral" phrasing for consistency.
- `ai-docs/mental-model/workflow-skills.md#L90-L92` — three bullets under the same anchor describing survey/research escalation signals; needs reconciliation per the ticket's explicit doc-pass deliverable.
- `ai-docs/mental-model/prompt-bundle.md#L62-L63` — also describes survey's `[escalate-to-research]` exit signal and research's escalation. Not explicitly named by the ticket's deliverable (which names only `workflow-skills` mental-model), but it documents the same behavior and will silently drift if left unedited; treat as an on-contact update per `AGENTS.md`'s "Update drifted docs on contact" rule, in the same doc pass.
- `agents-plugin-wsflow/rsrc/plan-populator-survey/plan-populator-survey.md`, `agents-plugin-wsflow/rsrc/plan-populator-research/plan-populator-research.md`, `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` — confirmed byte-identical to their `agents-plugin/rsrc/` counterparts right now (`diff` returns no output); after editing the canonical files, run the mirror regen rather than hand-editing these.

## Implementation Plan

1. Edit `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md`:
   - In `## Rules` (after L34's "Preserve the selected authority's intent" line), add a rule: a fully-specified, multi-part requirement must be carried whole into the plan; the planner may not silently implement a subset. A confident decision to implement only a subset is a lead scope-reduction decision — record it in `## Escalations` and report the new lead-directed signal instead of `[ok]`; a "first cut" is legitimate only when the ticket or lead already authorized the phasing.
   - Reword the L68-70 "Shortcut risk signals" bullet's "fallback behavior" term to explicitly name it as an **implementation fallback** (scope shortcut), and add a clarifying clause that a ticket's required **runtime fallback** (a specified execution branch such as graceful degradation) is not a shortcut signal and must be planned in full.
   - In `### 4. Report` (L128-139), change `` `[ok]` or `[escalate-to-research]` `` to a three-signal line (`[ok]`, `[escalate-to-research]`, or the new lead-directed signal — use `[escalate-to-lead]` per the ticket's suggested modeling on the research delegate's existing channel), and add a bullet "Escalation rationale when returning `[escalate-to-lead]`" alongside the existing research-escalation-rationale bullet.

2. Edit `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md`:
   - In `## Rules` L36-38, apply the same implementation-fallback/runtime-fallback distinction to "Do not encode temporary, fallback, mock-data, or duplicated-glue behavior as the implementation path," and broaden L38's escalation trigger ("Escalate when the accepted target cannot be satisfied without a questionable shortcut") to also cover a confident scope-reduction judgment on a fully-specified multi-part requirement, using the same down-scope-forbidden wording as the survey delegate.
   - Apply the same implementation-fallback clarification to the L74-76 "Identify" bullet ("fallback behavior" term).
   - `### 5. Report` (L99-107) already returns `[ok]` or `[escalate-to-lead]`; no token change needed — only extend the surrounding prose (Rules/Process) so the existing channel's scope explicitly includes the confident-subset case, consistent with the ticket's "extend that lead-directed signal ... to the confident-subset case" instruction.

3. Edit `agents-plugin/rsrc/lead-implement/lead-implement.md` L173: replace `- Binding authority decisions were not omitted or violated.` with an operationalized line stating each specified authority requirement is implemented or carries an explicit, authorized deferral — phrased against "authority" (not "ticket") so it applies to both `Authority: Ticket path` and `Authority: Inline contract` modes declared just above it (L161-162).

4. Update `ai-docs/spec/workflow-skills.md` under `{#260505-implementation-workflow-skills}`:
   - Around L899-905, add the new lead-directed scope-reduction escalation signal to the description of survey's (and research's) exit signals.
   - Around L928-931, tighten the reviewer obligation wording to match the new lead-implement.md line: each specified authority requirement is implemented, or carries an explicit, authorized deferral.

5. Reconcile `ai-docs/mental-model/workflow-skills.md#L90-L92` and `ai-docs/mental-model/prompt-bundle.md#L62-L63` to describe the new escalation signal and its scope, in the same doc pass as step 4.

6. Update `agents-plugin-tool/internal/mcp/playbook_tools_test.go`:
   - L1829: replace the `` "[ok]` or `[escalate-to-research]`" `` want string with the new survey Report wording (three signals) chosen in step 1, and add a want asserting the new signal token appears.
   - Add a want string in `TestPlaybookPrintGoldenLeadImplement` (near L2569-2571) asserting the reworded L173 coverage-obligation text is present in the rendered `lead-implement` body.
   - Add/extend wants in `TestRenderPlaybookFullWsPlannerContext`'s `assertPlanner` calls (L1828-1836) confirming: the down-scope-forbidden rule text is present for both survey and research, the runtime-vs-implementation-fallback distinction text is present for both, and `[escalate-to-research]`'s description text is unchanged (still strategy/contract-uncertainty scoped) to lock in the "not widened" constraint.

7. Run the wsflow mirror regen per `ai-docs/manuals/wsflow-mirroring.md`'s after-edit checklist, in order:
   - `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` (from `agents-plugin-tool/`)
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror` (from `agents-plugin-tool/`)
   - Confirm `git diff` shows the same three files newly diverged-then-re-synced under `agents-plugin-wsflow/rsrc/` plus `agents-plugin/rsrc/manifest.json`.

## Verification Plan

- `go test ./internal/mcp/... -run 'TestRenderPlaybookFullWsPlannerContext|TestRenderPlaybookLegacyContext|TestPlaybookPrintGoldenLeadImplement'` (from `agents-plugin-tool/`) — exercises the edited rendered/golden assertions from step 6.
- `go test ./internal/wsrsrc/... -run 'TestWsflowRsrcMirrorUpToDate'` (from `agents-plugin-tool/`) — byte-identity check after the mirror regen in step 7.
- Full Go suite: `go test ./...` from `agents-plugin-tool/` — required by the ticket's verification boundary ("full Go suite green").
- Manual read-through: confirm the reworded `[escalate-to-research]` description text in both prompt files still reads as strategy/contract-uncertainty only (no scope-reduction language leaked into it) — satisfies "does not widen `[escalate-to-research]`'s meaning."

## Escalations

- None.
