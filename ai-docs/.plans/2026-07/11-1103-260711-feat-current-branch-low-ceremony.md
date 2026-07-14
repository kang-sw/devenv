# Plan: 260711-feat-current-branch-low-ceremony — Phase 1: Derive current-branch completion from the low-risk verdict conjunction

## Relevant Ticket Contract

- Resolve current-branch execution only for an inline target observed on neither `impl/*` nor legacy `implement/*`, when raw facts independently meet automatic direct-edit eligibility, automatic lead-only review with `policy.review.override=auto`, and `skip-with-reason` documentation with a non-empty reason.
- Explicit direct-edit or lead-only overrides are not authorization; recompute eligibility from the unoverridden scope and risk facts. Any failed or unknown predicate, and every invocation already on an implementation branch, retains the standard isolated-branch path.
- The matching path keeps the observed branch, omits merge work, runs focused verification and lead-owned review, creates one logical explicit-path commit with `## AI Context`, reports completion, and never pushes.
- `enter.implement` and its installed todo instructions remain authoritative. Update both public contracts (`260505-implementation-workflow-skills`, `260625-session-state-tools`) and the affected workflow/runtime mental models after implementation.

## Out of Scope

- New user-facing modes/profiles, hotfix/tweak skills, eligibility matrices in playbook prose, or changes to the automatic direct-edit and review predicates themselves.
- Sprint removal or episode-marker behavior.
- Changes to standard create/rename/continue/stop, merge approval, merge execution, or cleanup behavior for any near miss or existing implementation branch.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L430-L486` — resolution currently derives branch action before delegation/review/docs; the matching conjunction needs a post-normalization decision point with access to target kind, raw facts, observed branch, automatic review eligibility, and normalized docs policy.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L534-L605` — existing automatic direct-edit and lead-only predicates are reusable, but their override-aware wrappers cannot authorize this path; extract or call pure automatic-eligibility helpers so explicit overrides cannot leak into current-branch eligibility.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L621-L687` — non-implementation branches always produce `create`; add a distinct no-merge branch outcome while preserving all existing implementation-branch branches and their merge metadata.
- `agents-plugin-tool/internal/mcp/session_state.go#L378-L424` — todo derivation always appends `final-action-gate` and `merge`; key omission from the new branch outcome here, retaining route/prep/edit/lead-only-review and adding an explicit completion/final-report step rather than weakening standard paths.
- `agents-plugin-tool/internal/mcp/session_state.go#L501-L617` — generated instructions are the correct authority for current-branch routing, focused verification, explicit-path commit/`## AI Context`, review rationale, final reporting, and no-push wording; merge-only instructions must remain untouched for other outcomes.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L8-L51` and `#L97-L117` — existing fixtures cover automatic safe resolution and explicit-direct override separately; extend them into a matching case plus table-driven near misses for ticket target, each unknown/failed scope or risk predicate, review override, missing docs reason, and both implementation-branch prefixes.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L105-L130` and `#L1758-L1794` — existing direct/lead-only/skipped-doc tests currently expect merge work; split matching current-branch expectations from implementation-branch preservation and assert todo order, merge/final-action omission, focused verification, commit, review rationale, report, and no-push text.
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md#L50-L57` and `agents-plugin/rsrc/lead-implement/lead-implement.md#L38-L60` — only compact judgment/policy wording is needed so explicit caller intent can populate existing facts/policy; do not restate the resolver conjunction or branch matrix.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1760-L1889` and `#L1896-L1935` — golden print assertions cover canonical and wsflow product-mode renders; add focused assertions for the compact intent wording and preserve the anti-duplication checks.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L17-L43` and `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L113` — canonical rsrc edits require manifest regeneration followed by byte-identical wsflow rsrc regeneration; stale generated assets are a concrete shortcut risk.

## Implementation Plan

1. In `agents-plugin-tool/internal/mcp/implement_resolver.go#L430-L687`, factor automatic direct-edit and automatic lead-only eligibility away from override handling, add one exact conjunction helper including inline target, observed non-implementation branch, skipped docs with reason, and no review override, then derive a distinct current-branch/no-merge branch plan outcome without changing any existing implementation-branch or near-miss result.
2. In `agents-plugin-tool/internal/mcp/session_state.go#L378-L617`, recognize that outcome when deriving todos and instructions: keep route, prep, direct edit with focused verification and one explicit-path `## AI Context` commit, lead-only review with rationale, and final reporting/no-push completion; omit merge-oriented final-action and merge todos only for this outcome.
3. In `agents-plugin-tool/internal/mcp/implement_resolver_test.go` and `agents-plugin-tool/internal/mcp/session_state_test.go`, add the positive case and a table-driven near-miss matrix, including explicit direct-edit and lead-only overrides, unknown facts, docs fallback, ticket target, and existing `impl/*`/`implement/*`; assert unchanged branch actions and merge todos outside the exact conjunction.
4. Read `agents-plugin/skills/lead-skill-authoring/SKILL.md`, then make compact, non-matrix wording updates in `agents-plugin/rsrc/lead-proceed/lead-proceed.md` and `agents-plugin/rsrc/lead-implement/lead-implement.md`; extend `agents-plugin-tool/internal/mcp/playbook_tools_test.go` for canonical and wsflow rendered wording.
5. Update `ai-docs/spec/workflow-skills.md#L515-L567`, `ai-docs/spec/mcp-tools.md#L225-L280`, `ai-docs/mental-model/workflow-skills.md#L43-L52`, and the `enter.implement` rule in `ai-docs/mental-model/mcp-runtime.md` to replace the old always-isolated assumption with the exact runtime-owned exception and public branch-action/todo contract.
6. Regenerate `agents-plugin/rsrc/manifest.json` and the byte-identical `agents-plugin-wsflow/rsrc/` tree using the documented env-gated tests, then verify no unexpected generated or unrelated files changed.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp -run 'TestResolveImplement|TestDeriveImplement|TestEnterImplement|TestPlaybookPrintGoldenLeadImplement|TestPlaybookPrintWsflowLeadImplement|TestPlaybookPrintGoldenLeadProceed' -count=1`
- `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -count=1`
- `cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror -count=1`
- `cd agents-plugin-tool && go test ./internal/wsrsrc ./internal/mcp -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- Inspect the positive verdict/todo fixture to confirm current branch retained, no merge target/work, explicit verification/commit/review/report/no-push instructions; inspect every near miss for unchanged isolated-branch behavior.

## Escalations

- None.
