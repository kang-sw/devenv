# Plan: 260713-bug-pre-ship-workflow-regression-closure — Phase 1: Close reviewed routing and execution regressions

## Relevant Ticket Contract

- Close all six Important pre-ship gaps and the minor Result-authoring mismatch as one correction slice while preserving explicit, safety-gated low ceremony, proportional ticketless routing, delegated implementation, survey planning, MCP-owned todo authority, and the normal discuss-to-ticket path.
- Reject missing or Git unborn-marker (`(initial)`) start commits only for current-branch/no-merge eligibility; preserve existing `create`, `rename`, `continue`, and `stop` behavior and every other raw safety predicate.
- Freeze ticket scope facts before source reads, but allow inline scope facts to come from the accepted caller contract, loaded context, focused source inspection, and command output before the single `enter.implement` call; unsupported facts remain `unknown`.
- Reuse the existing plan-populator chain for both authority kinds: ticket mode reads the ticket and phase, while inline mode receives a self-contained accepted scope/constraints/non-goals/verification boundary and never reads a placeholder ticket path. The same inline authority must survive survey-to-research escalation and plan-based review.
- Automatic `single` review uses the delegate-grade `reviewer` wrapper over the shared `code-reviewer` base to cover correctness, fit, and test. Explicit overrides and automatic two-or-more partition output remain unchanged.
- The generated doc-pre-pass todo exclusively owns mental-model dispatch. `needs-ticket=no` covers an eventual implementation commit plus any relevant existing spec; Result and Edition prose includes the behavioral delta without restating the phase plan/spec.
- Preserve the 260605 migration anchor: delegate work remains harness-native; prompts stay in the call-time `rsrc/` playbook factory; do not add subprocess agent machinery, a second inline planner, or conversation-only/placeholder authority (`ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md#L51-L71`, `#L246-L289`).
- Verification boundary is the focused and full Go suites, both plugin Python suites, MCP smoke test, both Claude manifest validations, canonical/wsflow byte identity, and `git diff --check`.

## Out of Scope

- Changing low-ceremony policy vocabulary, relaxing any raw safety/review/docs predicate, or changing merge/push approval behavior.
- Narrowing ticketless work back to single-file/direct-edit, changing delegated implementation or survey/research escalation, or changing normal discuss-to-ticket routing.
- New planner/reviewer playbooks, agent lifecycle/runtime work, review partition heuristics, plugin version bump, merge, release, or shipping.
- Rewriting already-correct ticket convention/spec prose merely for consistency; update docs only where executable behavior or coupling changes.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L414-L432` and `#L572-L601` — Git status already supplies `status.Branch.OID`; current eligibility validates only the branch name. Add one focused real-commit predicate and reuse it only in `currentBranchImplementEligible`.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1715-L1868` — integration tests use initialized but uncommitted repositories; the existing successful current-branch test therefore encodes the bug. Give the success fixture a real commit and add a distinct unborn-repository fallback test through `enter.implement`, not only a pure resolver test.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L40-L56` — general fact gathering permits caller/docs/source/commands, but the later ticket-only freeze rule overrides it for inline targets. Split the rule explicitly by target kind without changing unknown fallback.
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L1-L56` — the shipped survey declares only ticket/phase/plan variables and always reads a ticket. Extend this same prompt with declared target-kind and inline-contract inputs plus an authority table; no second inline playbook.
- `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md#L1-L44` — inline survey can escalate here, but research has the same ticket-only contract. Apply the identical authority selection so escalation cannot reintroduce a fake ticket dependency.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L86-L91` and `#L1000-L1163` — `shippedPlanPopulatorContext` and existing full-ws/wsflow render tests are the reusable contract-test seam. Add separate ticket and inline contexts, assert inline authority/no ticket read, and retain full-ws undeclared-variable rejection.
- `agents-plugin-tool/internal/mcp/session_state.go#L378-L426`, `#L521-L575`, and `#L995-L1030` — todo derivation currently lacks target kind, prep hardcodes ticket variables, and `single` names no playbook/frame. Thread target kind into `implementTodoVerdict`; generate target-aware planner instructions and name generic `code-reviewer` plus the generic review frame for `single`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L730-L753` — `next_instruction` also hardcodes ticket planner inputs. Make its delegated guidance authority-neutral or target-aware so raw verdict and installed todo do not conflict.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L119-L200` — plan and reviewer templates require ticket paths and define only partitioned reviewers. Add ticket/inline plan input forms, a generic single-review row/frame, and a generated-plan inline review frame using plan plus inline contract.
- `agents-plugin/rsrc/reviewer/reviewer.md` and `agents-plugin/rsrc/code-reviewer.md#L1-L20` — use the existing delegate-grade reviewer as the single-review render surface and share the authority/full-scope body through the flat code-reviewer base. This preserves reviewer role/tier/session metadata without duplicating generic review rules.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L74-L78` and `agents-plugin-tool/internal/mcp/session_state.go#L578-L583` — the always-rendered playbook states a broader mental-model condition than the authoritative todo. Remove the playbook threshold and leave spec invocation plus todo ownership intact.
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md#L50-L55` — both judgment rows require existing linked history, leaving new bounded inline work uncovered. Define No in terms of the eventual implementation commit and relevant existing spec, with Yes reserved for multiple slices or pre-implementation traceability beyond them.
- `agents-plugin/rsrc/executor-wrapup.md#L34-L46` and `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md#L56-L62` — Edition includes behavioral delta but a new Result does not; reuse the canonical five-field delta-only list for both.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1760-L1806`, `#L1900-L1925`, and `agents-plugin-tool/internal/mcp/session_state_test.go#L230-L265` — existing golden/todo assertions are the focused seams for fact-source, planner/reviewer frames, exhaustive needs-ticket wording, generic single dispatch, and exclusive doc-pre-pass ownership.
- `agents-plugin/rsrc/manifest.json`, `agents-plugin-wsflow/rsrc/`, and `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L91` — canonical rsrc edits require manifest regeneration followed by byte-identical wsflow mirror generation; do not hand-edit the derivative tree.
- `ai-docs/spec/workflow-skills.md#L535-L604`, `#L757-L766`, `ai-docs/spec/mcp-tools.md#L233-L305`, `ai-docs/mental-model/workflow-skills.md#L43-L52`, `#L82-L84`, `ai-docs/mental-model/mcp-runtime.md#L50`, and `ai-docs/mental-model/prompt-bundle.md#L60-L63` — update ticket/inline planner authority, generic single-review dispatch, unborn marker, and eventual-commit ticketless recovery. `ai-docs/spec/documentation-system.md#L92-L101` and `ai-docs/mental-model/documentation-system.md#L27-L28` already state the correct Result delta contract.

## Implementation Plan

1. In `agents-plugin-tool/internal/mcp/implement_resolver.go`, add a small observed-start-commit validity helper (non-empty and not `(initial)`) to current-branch eligibility only; update `agents-plugin-tool/internal/mcp/implement_resolver_test.go` and the `enter.implement` integration fixtures in `agents-plugin-tool/internal/mcp/session_state_test.go` so a committed repo can take `current` and an initialized/uncommitted repo deterministically falls back to the standard branch/todo path.
2. In `agents-plugin/rsrc/lead-implement/lead-implement.md`, split fact sourcing by ticket versus inline authority, remove the duplicated mental-model threshold, and add explicit ticket/inline planner and reviewer frames. Keep the invariant lines atomic and run the required fresh-reader audit after drafting.
3. Extend `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md` and `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md` with the same declared target-kind/inline-contract render contract. Ticket mode reads ticket+phase; inline mode treats the supplied accepted contract as authority and must not read a blank/placeholder ticket path. Update `agents-plugin-tool/internal/mcp/playbook_tools_test.go` with separate ticket/inline rendering cases for both planner depths.
4. Thread target kind through `implementTodoVerdict` in `agents-plugin-tool/internal/mcp/session_state.go`; make prep and resolver `next_instruction` planner guidance pass every declared authority variable, using explicit empty values for the inactive authority. For `single`, dispatch the delegate-grade `reviewer` wrapper over the shared `code-reviewer` base using the generic full-scope frame. Update `agents-plugin-tool/internal/mcp/session_state_test.go` and `agents-plugin-tool/internal/mcp/implement_resolver_test.go` for target-aware prep, generic single wording, and unchanged partition/override behavior.
5. Update `agents-plugin/rsrc/code-reviewer.md` and the lead reviewer templates so ticketless delegated review reads generated plan + inline contract + diff, while ticket-backed and direct-edit frames remain supported. Add golden assertions in `agents-plugin-tool/internal/mcp/playbook_tools_test.go` that generic review covers correctness/fit/test and no inline frame requires a ticket path.
6. Correct `agents-plugin/rsrc/lead-proceed/lead-proceed.md` to make the two `needs-ticket` rows exhaustive for new bounded inline work, and add behavioral delta to new Result guidance in `agents-plugin/rsrc/executor-wrapup.md`. Update focused golden/content assertions in `agents-plugin-tool/internal/mcp/playbook_tools_test.go` (and the closest wsrsrc/infra content test if needed) without altering the already-correct bundled ticket convention.
7. Regenerate `agents-plugin/rsrc/manifest.json` with `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`, then regenerate `agents-plugin-wsflow/rsrc/` with `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`; verify `agents-plugin-wsflow/skills/lead-implement` and `lead-proceed` remain product-mode shims.
8. Reconcile the corrected executable contract in `ai-docs/spec/workflow-skills.md`, `ai-docs/spec/mcp-tools.md`, `ai-docs/mental-model/workflow-skills.md`, `ai-docs/mental-model/mcp-runtime.md`, and `ai-docs/mental-model/prompt-bundle.md`. Leave documentation-system spec/model unchanged unless implementation reveals a real delta, then complete the ticket Result and focus closeout through the normal doc pipeline.

## Verification Plan

- Focused TDD/post-impl: `cd agents-plugin-tool && go test ./internal/mcp ./internal/wsdoc ./internal/wsrsrc -count=1`.
- Full runtime: `cd agents-plugin-tool && go test ./... -count=1`.
- Package surfaces: `python3 -m unittest discover agents-plugin/tests` and `python3 -m unittest discover agents-plugin-wsflow/tests`.
- Runtime smoke from `agents-plugin-tool/`: `scripts/smoke-ws-mcp.sh ..`.
- Packaging: `claude plugin validate agents-plugin` and `claude plugin validate agents-plugin-wsflow`.
- Generated artifacts: compare canonical and wsflow rsrc trees byte-for-byte after the two required regeneration commands; run spec index verification if specs change.
- Hygiene: `git diff --check`, inspect the final branch diff, and confirm no source edit introduced subprocess agent management, a second planner, fake ticket authority, or changes to merge/push gates.
- Prompt quality: run the `lead-skill-authoring` invariant checklist and a no-prior-context fresh-reader audit over each changed playbook/convention excerpt, fixing only findings classified as fix.

## Escalations

- None.
