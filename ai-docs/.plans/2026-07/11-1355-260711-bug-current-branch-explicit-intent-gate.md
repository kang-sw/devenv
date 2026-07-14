# Plan: 260711-bug-current-branch-explicit-intent-gate — Phase 1: Add a low-ceremony preference gate without weakening raw-fact safety

## Relevant Ticket Contract

- `ai-docs/tickets/ready/260711-bug-current-branch-explicit-intent-gate.md#L35-L59` — Add internal `policy.low_ceremony_if_safe: yes|no|unknown`; only `yes` expresses reduced-ceremony preference, grants no safety authority, requires no branch vocabulary, and must not be inferred from `hotfix`, `tweak`, or `small fix` alone.
- `ai-docs/tickets/ready/260711-bug-current-branch-explicit-intent-gate.md#L43-L56` — `no`, `unknown`, or missing policy must retain the standard branch path; rejected `yes` falls back without stopping valid work and emits a concise not-applicable warning; do not reinterpret direct-edit, docs, or branch policy as the intent signal.
- `ai-docs/tickets/ready/260711-bug-current-branch-explicit-intent-gate.md#L61-L72` — Keep the existing raw-fact conjunction, standard risky/delegated path, independent docs skip, successful current-branch todos/no-push boundary, and canonical/wsflow parity unchanged.
- `ai-docs/tickets/ready/260711-bug-current-branch-explicit-intent-gate.md#L76-L91` — Verification must cover positive, `no`, `unknown`, missing, rejected-request, and standard-path preservation through the public resolver/enter path, then update the two cited specs and mental models and regenerate canonical/wsflow rsrc artifacts.

## Out of Scope

- `ai-docs/tickets/ready/260711-bug-current-branch-explicit-intent-gate.md#L26-L31` — Do not change the existing force-direct semantics of `explicit_direct_edit_request`, make docs omission imply low ceremony, or expose branch strategy to users.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L534-L620` — Do not weaken automatic direct-edit or lead-only-review predicates or change independent delegation/review allocation; the new field gates only `Branch Action: current`.
- `agents-plugin-tool/internal/mcp/session_state.go#L503-L560` — Do not redesign successful current-branch route/edit/completion instructions or the no-push/no-merge todo shape.
- `agents-plugin/runtime.json#L1-L20` — No new MCP tool or runtime capability is introduced; the existing `enter.implement` entry remains the public surface, so runtime tool inventories/version metadata need no semantic change.
- Spec, mental-model, ticket Result, and project-focus closeout remain lead-owned after source implementation and review; the delegated implementer stops after source, tests, playbook, and generated mirror artifacts are committed.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L12-L170` — Policy input, normalized facts, result warnings, and agenda conditions/warnings are centralized structs; add the preference at the policy root and normalized-fact layer so both text and JSON/agenda outputs inherit it.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L358-L407` — `parseImplementPolicy` already validates root policy groups and enum facts; reuse `parseEnumFact` for the new root `yes|no|unknown` field rather than introducing a parser.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L430-L531` — Resolution already threads normalization warnings into both result and agenda; append the rejected-`yes` warning here after branch resolution so it appears consistently in raw, JSON, and stored agenda.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L547-L592` — Current-branch eligibility is a single raw-fact conjunction followed by standard-plan fallback; add `LowCeremonyIfSafe == "yes"` without changing the existing predicates, and warn only when normalized preference is `yes` but the resolved action is not `current`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L739-L810` — Conditions and canonical raw rendering already expose normalized policy and warnings; adding `low-ceremony-if-safe=<value>` to conditions supplies verdict/agenda observability without a duplicate output channel.
- `agents-plugin-tool/internal/mcp/server.go#L2856-L2941` — The public MCP schema defines root `policy` properties; add the nullable enum beside branch/review/docs and extend schema tests rather than changing dispatch or runtime manifests.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L53-L155` — Existing positive and near-miss fixtures currently qualify from docs skip alone; update the shared fixture/base with explicit `yes`, then add `no`/`unknown`/missing gates and warning/fallback assertions while retaining the override-resistant near-miss matrix.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1188-L1225` and `agents-plugin-tool/internal/mcp/session_state_test.go#L1782-L1899` — Public tools/list and keyed `enter.implement` tests already cover schema, focused current-branch todos, and standard near-miss merge todos; extend these to prove the new field is nullable and rejected `yes` preserves independently derived delegation, review, docs, branch action, final-action, and merge behavior.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L38-L54` — The pre-call Policy rules are the correct Layer-3 judgment point; compactly replace the existing current-branch wording with clear reduced-ceremony intent mapping and the insufficient-word examples, without copying resolver predicates or adding a mode section.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1760-L1805` — The rendered-playbook golden already pins policy wording and product-mode substitution; update it to assert the new compact judgment, including the `hotfix`/`tweak` negative cue.
- `ai-docs/ref/wsflow-mirroring.md#L162-L202` — Canonical rsrc is byte-identically generated into wsflow; edit only canonical `agents-plugin/rsrc`, regenerate its manifest, then regenerate the wsflow tree rather than hand-editing the derivative.
- `ai-docs/spec/mcp-tools.md#L233-L292`, `ai-docs/spec/workflow-skills.md#L519-L578`, `ai-docs/mental-model/mcp-runtime.md#L50-L50`, and `ai-docs/mental-model/workflow-skills.md#L47-L51` — Existing contract/model text currently describes raw-fact safety but omits the explicit preference gate; update these anchors in place after behavior is verified.

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/implement_resolver.go#L59-L169` and `agents-plugin-tool/internal/mcp/implement_resolver.go#L358-L531` — Add root policy parsing and unknown-default normalization for `low_ceremony_if_safe`, expose it in normalized conditions/agenda, and reuse the existing warnings pipeline.
2. `agents-plugin-tool/internal/mcp/implement_resolver.go#L563-L592` — Require normalized `yes` in the unchanged current-branch safety conjunction; when `yes` falls through to the standard branch resolver, append one concise not-applicable warning without altering independently derived delegation, review, docs, or branch results.
3. `agents-plugin-tool/internal/mcp/server.go#L2856-L2941`, `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L53-L155`, and `agents-plugin-tool/internal/mcp/session_state_test.go#L1188-L1225` — Extend the public nullable enum schema and resolver tests for positive, `no`, `unknown`, missing, and rejected cases, including the normalized condition and warning in text/JSON/agenda output.
4. `agents-plugin-tool/internal/mcp/session_state_test.go#L1420-L1481` and `agents-plugin-tool/internal/mcp/session_state_test.go#L1782-L1899` — Route all current-branch success through explicit `yes`; add an unsafe/rejected public enter case that asserts standard branch action and the independently resolved delegation, review allocation, doc mode, final-action, and merge todo instructions remain intact.
5. `agents-plugin/rsrc/lead-implement/lead-implement.md#L47-L54` and `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1760-L1805` — Keep the shared playbook edit pre-call and compact: map a clear request to omit ceremony and proceed directly to `yes` without branch terms, reject urgency/size words alone, and update the rendered golden; run the required fresh-reader/routing coverage and downstream-consistency audits against the final wording.
6. `agents-plugin/rsrc/manifest.json`, `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`, and `agents-plugin-wsflow/rsrc/manifest.json` — Regenerate the canonical manifest and byte-identical wsflow rsrc mirror using the repository-provided env-gated tests; do not hand-edit derivative files.
7. Lead-owned closeout: update `ai-docs/spec/mcp-tools.md#L233-L292`, `ai-docs/spec/workflow-skills.md#L519-L578`, `ai-docs/mental-model/mcp-runtime.md#L50-L50`, and `ai-docs/mental-model/workflow-skills.md#L47-L51` with the verified preference gate, rejected-request fallback/warning, intent judgment, and unchanged raw-fact authority/current-branch completion boundary.

## Verification Plan

- `agents-plugin-tool/internal/mcp/implement_resolver_test.go` — From `agents-plugin-tool/`, run `go test ./internal/mcp -count=1` to cover resolver, public schema, enter/agenda/todo, raw/JSON warning, and preservation cases.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L894-L910` — From `agents-plugin-tool/`, run `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L83-L105` — From `agents-plugin-tool/`, run `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`, then `go test ./internal/wsrsrc/... -count=1` for manifest/mirror drift guards.
- `agents-plugin-wsflow/tests` — From repository root, run `python3 -m unittest discover agents-plugin-wsflow/tests` to verify product-mode runtime and thin-shim/shared-playbook parity.
- `agents-plugin-tool` — Run `go test ./... -count=1` for the full Go regression boundary after generated artifacts and docs are updated.

## Escalations

- None.
