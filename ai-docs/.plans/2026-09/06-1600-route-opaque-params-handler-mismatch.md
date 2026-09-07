# Plan: Opaque route params do not reach deterministic handlers — Phase 1: Reconcile the route input contract

## Relevant Ticket Contract
- Make the advertised `session_key` envelope plus opaque `params` object reach the existing typed parsers for both `route.resolve_proceed` and `route.resolve_implement`; the canonical public wrapper holds `target`, `facts`, optional `policy`, and `format` inside `params`.
- When `params` is present, accept only outer `session_key` and `params`, require `params` to be an object, reject an inner `session_key`, and route through typed validation even when `target` is missing. Reject every malformed or mixed envelope before an agenda or todo mutation.
- Preserve unwrapped top-level typed calls and the unwrapped `route.resolve_implement` legacy mode-entry path. A wrapped implement call has no legacy fallback.
- Update the MCP and workflow-skill contracts, shared Fact Contracts, and generated distribution copies; verify wrapped calls yield the actual deterministic verdict, normalized agenda, and derived todos.

## Out of Scope
- New routing policy, route tool renames, removal of the legacy unwrapped implement mode-entry contract, or a change to the opaque public schema beyond documenting its canonical payload envelope.
- Re-running the completed review: the parent adjudication confirms the wrapper clarification and spec ownership are the bounded reviewed repair.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/session_state.go#L1028-L1107` — `handleEnterImplement` uses only a top-level `target` to select typed routing, so a schema-valid wrapper falls into the legacy branch and mutates state with a legacy agenda/todo result.
- `agents-plugin-tool/internal/mcp/session_state.go#L1143-L1177` — `handleEnterProceed` calls `parseProceedInput` on the outer arguments; a wrapped target consequently returns `target is required` before the deterministic resolver runs.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L180-L207` and `agents-plugin-tool/internal/mcp/proceed_resolver.go#L102-L124` — the established typed parsers already provide target/facts/policy/format validation and should consume a validated inner map rather than duplicate field parsing.
- `agents-plugin-tool/internal/mcp/session_state.go#L862-L869` — `sessionStateKey` validates only the outer key; a route-envelope helper must keep authentication at that outer boundary and reject a nested key before handler state writes.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1368-L1504` and `#L1819-L1889` — existing opaque-schema and typed verdict/state tests provide the fixtures needed to assert wrapper parity and unchanged state on error.
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md#L68-L73` and `agents-plugin/rsrc/lead-implement/lead-implement.md#L38-L43` presently describe the opaque contract but instruct top-level typed fields; canonical rsrc is mirrored byte-for-byte to `agents-plugin-wsflow/rsrc/` and protected by manifest/mirror guards.
- `ai-docs/spec/mcp-tools.md#L278-L402` and `ai-docs/spec/workflow-skills.md#L748-L754` / `#L1108-L1115` retain the superseded claim that the decoder reads top-level typed fields.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/session_state.go`, add one route-envelope normalization/validation seam used by both handlers. Detect wrapper presence by `params`, reject non-object values (`null`, arrays, strings), reject empty wrapper payload through the normal typed missing-target error, reject outer keys other than `session_key`/`params` (including mixed typed or legacy fields), and reject `params.session_key`; return the inner map only after those checks. Authenticate with the outer `session_key`, then invoke the existing typed parsers and state-write path. Keep the no-`params` branches intact: proceed remains top-level typed, while implement chooses its current top-level typed versus legacy behavior.
2. Refactor `handleEnterImplement` and `handleEnterProceed` to dispatch using the normalization result, so every wrapped implement request is typed and cannot reach `handleEnter`; retain existing resolver, branch observation, agenda marshalling, and todo derivation logic without a second decision path.
3. Extend `agents-plugin-tool/internal/mcp/session_state_test.go` with wrapper integration cases for both routes: valid wrapper verdict and JSON/text behavior, agenda and todo equality with the corresponding top-level typed call, outer-session authorization, missing/invalid target errors, and a before/after state assertion for every rejected wrapper. Cover finite malformed inputs: `params: null`, array, string, and empty object; outer-envelope extras; mixed outer `target`/legacy fields; and nested `params.session_key`. Retain explicit compatibility tests for unwrapped typed calls and unwrapped implement legacy mode entry.
4. Change the Fact Contract call examples in `agents-plugin/rsrc/lead-proceed/lead-proceed.md` and `agents-plugin/rsrc/lead-implement/lead-implement.md` to show `session_key` outside a single `params` object and all typed routing fields inside it. Regenerate the canonical rsrc manifest and byte-identical wsflow rsrc mirror; do not hand-edit the generated mirror.
5. Update `ai-docs/spec/mcp-tools.md` and `ai-docs/spec/workflow-skills.md` to state the canonical wrapper, strict outer-envelope rules, typed-only wrapped behavior, and retained unwrapped compatibility paths, replacing the stale top-level-decoder wording without changing routing decisions.

## Verification Plan
- Run focused `go test` cases in `agents-plugin-tool/internal/mcp` for the two wrapper integrations, malformed-envelope/state-immutability matrix, outer session authentication, typed verdict/agenda/todo parity, and legacy compatibility; then run `cd agents-plugin-tool && go test ./internal/mcp ./internal/wsrsrc`.
- After canonical rsrc edits, regenerate in order: `cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`, then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`; rerun the manifest and mirror guards in the package test.
- Run `python3 -m unittest discover agents-plugin-wsflow/tests` to validate the derivative package after the generated rsrc update.

## Escalations
- None.
