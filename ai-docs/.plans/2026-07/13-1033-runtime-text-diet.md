# Plan: runtime-text-diet

## Relevant Ticket Contract
- Inline authority: reduce runtime workflow text identified by the `origin/main...HEAD` audit without changing behavior, safety gates, reviewer authority, planner support for ticket and inline targets, or route/diagnostic outcomes.
- Keep plans and `.done` tickets unchanged; this plan file is the only planning artifact for the slice.
- Preserve canonical ownership under `agents-plugin/rsrc/`, regenerate its manifest, and regenerate the byte-identical `agents-plugin-wsflow/rsrc/` mirror rather than editing mirrored files.
- Preserve low-ceremony policy mapping, eligibility predicates, warnings, branch-stop diagnostics, and current-branch completion diagnostics.

## Out of Scope
- `agents-plugin-tool/internal/mcp/implement_resolver.go` eligibility, review-allocation, branch, or documentation verdict logic beyond shortening delegated `NextInstruction` prose.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go`, `agents-plugin/rsrc/lead-proceed/lead-proceed.md`, and all ticket-routing judgments.
- Specs, mental models, existing files under `ai-docs/.plans/`, and tickets under `ai-docs/tickets/.done/`; no behavioral contract change is intended.
- Consolidating standalone Survey and Research plan formats across files; each delegate prompt must remain independently executable.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L739-L765` — delegated `NextInstruction` repeats the complete planner authority and escalation procedure later installed in the Prep todo. `lead-implement` treats todos as the authoritative ordered runbook, so the delegated next step can deterministically name the installed delegated workflow without repeating `plannerAuthorityInputs`; retain detailed branch-stop, direct-edit, and current-branch wording.
- `agents-plugin-tool/internal/mcp/session_state.go#L568-L613` — single-review, doc-pre-pass, and final-action instructions repeat contracts already carried by the rendered reviewer, `impl-playbook`, and authoritative spec guidance. Compact these instructions while retaining the reviewer playbook/frame/path, the exact mental-model dispatch threshold, unchanged-input verification reuse, docs-only affected checks, review disposition, documentation mode, and merge-confirm behavior.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L35-L54` and `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L30-L53` — each prompt selects ticket versus inline authority in both Rules and the first Process step. Keep the executable Process step and delete the duplicate Rules statement; do not remove rendered target-kind or inline-contract inputs.
- `agents-plugin/rsrc/reviewer/reviewer.md#L11-L17`, `agents-plugin/rsrc/code-reviewer.md#L8-L20`, and `agents-plugin/rsrc/lead-implement/lead-implement.md#L153-L177` — full-scope coverage and the one-line verdict format are repeated across wrapper, shared contract, and prompt frame. Keep coverage in `code-reviewer` Process step 5 and verdict/output authority in its Output section; shorten the wrapper to invoke the shared contract without a partition, delete the redundant no-partition Constraint, and remove only the initial Reviewer prompt frame's repeated message-response line. Keep the self-contained Re-review prompt output line.
- `agents-plugin/rsrc/executor-wrapup.md#L34-L48` — Result and Edition repeat the same delta-only field list. State the shared Result/Edition content rule once, then keep the distinct heading/hash instructions for the two cases.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L238-L265`, `agents-plugin-tool/internal/mcp/session_state_test.go#L158-L282`, `#L1780-L1842`, `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1168-L1190`, `#L1818-L1952`, and `agents-plugin-tool/internal/mcp/mercenary_surface_test.go#L470-L480` — current tests pin verbose strings. Shift delegated planner-detail assertions from `NextInstruction` to the already authoritative Prep todo, and assert preserved outcomes/semantic anchors rather than deleted duplication.
- `agents-plugin/rsrc/manifest.json` and `agents-plugin-wsflow/rsrc/` — canonical rsrc edits require manifest regeneration followed by byte-identical mirror regeneration; wsflow package tests are the downstream drift guard.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/implement_resolver.go#L739-L765`, replace only the delegated `implementNextAfterBranch` procedure recital with a compact deterministic instruction to execute the installed delegated Prep/Edit/Review/Documentation todos in order. Keep review allocation and documentation mode visible, keep `plannerAuthorityInputs` for Prep todo generation, and leave stop/direct/current branch diagnostics intact.
2. In `agents-plugin-tool/internal/mcp/session_state.go#L568-L613`, shorten the single-review instruction to render and dispatch one full-scope reviewer with the existing frame/path and relay rule; shorten doc-pre-pass without losing its three qualifying knowledge categories or authoritative-spec exclusion; express final verification by the `impl-playbook` unchanged-input rule plus docs-only affected checks while preserving every merge-confirm and documentation-mode branch.
3. In `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L35-L54` and `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L30-L53`, delete the duplicated Rules authority-selection statement and retain Process step 1 as the single executable ticket/inline selection rule.
4. In `agents-plugin/rsrc/reviewer/reviewer.md#L11-L17`, `agents-plugin/rsrc/code-reviewer.md#L8-L20`, and `agents-plugin/rsrc/lead-implement/lead-implement.md#L153-L177`, remove only the identified full-scope/verdict duplication while preserving authority selection, partition behavior, findings-path output, severity gates, and the self-contained Re-review prompt.
5. In `agents-plugin/rsrc/executor-wrapup.md#L34-L48`, factor the common Result/Edition delta-only content contract into one sentence above the numbered cases and retain each case's heading and result-commit rule.
6. Update the focused tests named in Codebase Findings so they prove unchanged route labels, branch and warning diagnostics, planner ticket/inline authority in Prep, escalation behavior in Prep, reviewer authority/output, mental-model threshold, verification reuse, and Result/Edition semantics without requiring deleted wording.
7. From `agents-plugin-tool/`, regenerate `agents-plugin/rsrc/manifest.json` with `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`, then regenerate `agents-plugin-wsflow/rsrc/` with `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
8. Run a fresh-reader audit and downstream consistency sweep on the changed canonical playbooks; reject any proposed shortening that makes ticket/inline planner authority, reviewer scope, verification validity, documentation eligibility, or final-action ownership implicit.

## Verification Plan
- From `agents-plugin-tool/`: `go test ./internal/mcp ./internal/wsrsrc -count=1` and `go test ./... -count=1`.
- From repository root: `python3 -m unittest discover agents-plugin-wsflow/tests` and `git diff --check`.
- Verify `agents-plugin/rsrc/` and `agents-plugin-wsflow/rsrc/` byte identity through `TestWsflowRsrcMirrorUpToDate`; confirm both manifests match regenerated content hashes.
- Exercise or inspect direct-edit, delegated-ticket, delegated-inline, branch-stop, and low-ceremony-current resolver cases. `NextInstruction` must stay non-empty and match the raw `Next:` line; detailed planner authority/escalation must remain in Prep todos for both ticket and inline targets.
- Compare `origin/main...HEAD`-baseline and final `wc -w` counts for each changed canonical runtime file, plus exact word counts of delegated `NextInstruction`, Prep, single-review, doc-pre-pass, and final-action instructions. Report gross physical mirror changes separately from per-agent context savings.
- Confirm no existing plan or `.done` ticket changed, and no low-ceremony warning, eligibility predicate, branch diagnostic, review allocation, planner variable, or result schema changed.

## Escalations
- None.
