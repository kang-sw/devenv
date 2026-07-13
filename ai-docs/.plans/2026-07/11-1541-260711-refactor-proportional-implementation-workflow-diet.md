# Plan: proportional implementation workflow diet

## Relevant Ticket Contract

- Preserve normal `lead-discuss -> lead-write-ticket -> ticket -> lead-proceed`, both Sage stages, delegated implementation, survey planning, the file-first `PlanPath` contract, branch/merge/push safety, and the existing `low_ceremony_if_safe` behavior.
- For ticketless direct inline work, base ticket need on unresolved decisions, phases, cross-session coordination, and recovery value rather than file count or public surface alone.
- Allocate review from independent risk partitions: correctness for material correctness/security or new contracts/symbols; fit for material fit, cross-module work, or reuse uncertainty; test for material test risk or new test files. Zero or one partition resolves to `single`; two or more remain partitioned.
- Reuse a passing full-suite result while source is unchanged; docs-only commits run affected checks only. Mental-model work is conditional on new reusable modification guidance. Result/Edition text records deltas and evidence without repeating ticket/spec content.

## Out of Scope

- No new mode, fact, public schema, user vocabulary, ticket-direct implementer route, Sage change, survey-plan removal, review waiver, or branch/merge/push change.
- Do not edit specs, mental models, ticket Result/status, or `_index.md`; the lead owns documentation closeout.
- Do not discard the four existing uncommitted draft files. Treat them as user-approved starting edits and refine only where tests or consistency require it.

## Codebase Findings

- `agents-plugin/rsrc/lead-proceed/lead-proceed.md`, `impl-playbook.md`, and `executor-wrapup.md`, plus `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`, already contain the approved uncommitted wording draft.
- `agents-plugin-tool/internal/mcp/implement_resolver.go` owns `deriveImplementReviewAlloc` and partition selection. Adjust existing conditions and return shape; add no inputs.
- `agents-plugin-tool/internal/mcp/session_state.go` owns generated doc and final-action todo instructions. Keep instructions compact and generated-output authoritative.
- Canonical `agents-plugin/rsrc` edits require manifest regeneration and byte-identical `agents-plugin-wsflow/rsrc` regeneration. Do not hand-edit generated mirrors.
- Existing resolver, session-state, playbook rendering, wsrsrc manifest/mirror, convention, and wsflow tests are the verification seams.

## Implementation Plan

1. Finalize the four existing draft edits without expanding the normal discuss/ticket/Sage path.
2. Refactor review allocation around an internal partition list: zero or one independent partition yields `single`; two or more render `partitioned: ...`. Remove `surface=public-interface` as a fit-only trigger and `test_surface=existing` as a test-only trigger while retaining cross-module, reuse, new-file, contract, and explicit material-risk triggers.
3. Update focused resolver and public enter/session tests for single-review bounded public/existing-test cases and retained multi-risk partitioned cases.
4. Update generated todo instruction text and tests so passing full-suite evidence is reusable for an unchanged source tree and mental-model dispatch is conditional on new reusable guidance.
5. Regenerate canonical manifests and the wsflow rsrc mirror; update rendered-playbook, convention, manifest, and mirror tests as required.
6. Run focused MCP/wsrsrc/wsdoc tests, the full Go suite, wsflow Python tests, and `git diff --check`. Commit one or more logical implementation checkpoints with `## AI Context` and report the range.

## Verification Plan

- From `agents-plugin-tool`: `go test ./internal/mcp ./internal/wsdoc ./internal/wsrsrc -count=1`.
- Regenerate canonical and wsflow resources using the repository's env-gated wsrsrc tests, then run `go test ./... -count=1`.
- From repository root: `python3 -m unittest discover agents-plugin-wsflow/tests`.
- Confirm canonical/wsflow generated resources are byte-identical and run `git diff --check`.

## Escalations

- Stop if proportional review allocation requires a new public fact/schema or if generated todo wording cannot express evidence reuse without weakening verification. Otherwise keep the implementation within the existing resolver and instruction contracts.
