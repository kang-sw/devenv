# Plan: Degrade safely when a Pi task fork cannot load a parent extension tool — Phase 1: Preserve fork prefix while degrading absent extension tools

## Relevant Ticket Contract
- Preserve the parent fork's byte-identical system instructions, ordered provider-visible tool definitions, and inherited conversation prefix; do not filter missing tools from the captured surface or add a system-prompt capability notice.
- Before readiness comparison, install metadata-identical deterministic unavailable stubs only for missing, non-completion-critical task-fork tools. `ws-report-to-lead` remains explicitly completion-critical and fatal when absent.
- Add the missing-tool names, in captured parent-tool order, only to the first task-fork user-message framing. Existing tools with extra, reordered, schema, or description drift remain fatal; cache eligibility is not a provider cache-hit/billing guarantee.
- Amend spec anchor `260905-pi-side-thread-fork-task-thread` for this narrow unavailable-stub exception to its no-synthetic-registration rule.

## Out of Scope
- Diagnosing whether the observed 79-versus-78 failure is an adapter regression, a prompt restriction, or inherited-context exposure issue.
- General extension cloning, deferred-tool loading, provider cache/billing behavior, and changes to worker, execute-worker, discussion-fork, or ws-mcp/shared-rsrc surfaces.
- Later ticket work, including the fork-raised question-queue implementation; only its post-load live acceptance check is a Phase 1 verification boundary.

## Codebase Findings
- `agents-plugin-pi/src/fork-context.ts#L80-L104` — captures parent active definitions in order and currently uses a count-first exact comparator; its diagnostics cannot identify missing versus extra/reordered/changed registrations.
- `agents-plugin-pi/src/index.ts#L396-L410` and `agents-plugin-pi/src/index.ts#L680-L701` — fork input is blocked before provider delivery on registration drift, and fork readiness currently compares the child surface immediately after bootstrap; this is the insertion point for stubs before equality is checked.
- `agents-plugin-pi/src/fork-context.ts#L177-L179` and `agents-plugin-pi/src/fork.ts#L300-L307` — first fork input is framed once with the child key after the parent-created task message, allowing an unavailable-tools notice to remain a user-message suffix rather than changing the inherited prefix.
- `agents-plugin-pi/src/spawner.ts#L2012-L2032` and `agents-plugin-pi/src/spawner.ts#L2794-L2813` — readiness is validated before the initial fork prompt, so fatal critical-tool or structural differences prevent the first model turn.
- `agents-plugin-pi/src/bridge.ts#L747-L783` — bridged MCP definitions retain their name, description, and JSON-schema parameters when registered; the unavailable handler must reuse the parent-captured metadata rather than reconstructing it from child resources.
- `agents-plugin-pi/test/fork-lifecycle.integration.test.ts#L12-L180` and `agents-plugin-pi/test/fork-prefix.integration.test.ts#L93-L220` — real adapter/SDK lifecycle coverage already asserts exact serialized tool arrays and first-message behavior, providing the end-to-end parent-only-extension reproducer seam.
- `ai-docs/spec/pi-adapter-runtime.md#L1280-L1303` — the current contract says every missing registration is a visible failure and forbids synthetic replay, so it must be amended narrowly; leaving it unchanged would contradict the implementation.

## Implementation Plan
1. In `agents-plugin-pi/src/fork-context.ts`, add pure fork-registration helpers that classify expected-versus-actual definitions as missing, extra, reordered, or changed while preserving captured parent order. Define the explicit completion-critical tool set there (including `ws-report-to-lead`) and retain strict rejection for every category other than an absent non-critical expected definition.
2. In `agents-plugin-pi/src/index.ts`, during a task-fork's session bootstrap and before its readiness capture, derive absent expected registrations from `durableForkContextRef`, fail readiness for a missing completion-critical tool, and register a direct unavailable handler for each other missing tool. Reuse the captured name, description, and parameters verbatim; make invocation fail deterministically with the tool name and the explanation that its parent extension was not loaded. Do not alter existing registrations, active ordering, system-prompt construction, or non-task roles.
3. Keep the derived unavailable names in captured parent order and extend the one-time task-fork input framing in `agents-plugin-pi/src/fork-context.ts`/`agents-plugin-pi/src/index.ts` so the first user message appends a deterministic unavailable-tools notice after the existing fork frame. Subsequent user messages, discussion forks, and no-mismatch task forks must receive neither notice nor stub behavior.
4. Update `agents-plugin-pi/src/spawner.ts` to consume the richer readiness comparison so launch errors name missing, extra, reordered, and changed definitions rather than reporting only count or generic drift. Preserve validation-before-`promptAgent`, owned-session checks, and the fatal `ws-report-to-lead` path.
5. Extend focused tests in `agents-plugin-pi/test/fork-context.test.ts` and `agents-plugin-pi/test/fork-review-regressions.test.ts` for classified diagnostics, ordered non-critical absence, fatal completion-channel absence, and unchanged extra/order/schema/description rejection. Extend `agents-plugin-pi/test/fork-lifecycle.integration.test.ts` with a parent-only extension tool: assert the child passes readiness with identical serialized metadata, emits exactly one first-task notice, deterministically rejects that stub call, retains `ws-report-to-lead`, and still completes; cover a no-mismatch control. Keep/extend `agents-plugin-pi/test/fork-prefix.integration.test.ts` where needed to lock the provider-visible ordered definitions and inherited-prefix equality.
6. Amend `ai-docs/spec/pi-adapter-runtime.md` at `{#260905-pi-side-thread-fork-task-thread}` to state the accepted metadata-identical unavailable-stub exception, its deterministic failure/first-message notice, the explicit completion-critical fatal set, and the unchanged strictness for extras and drift. Preserve the existing provider-cache caveat.

## Verification Plan
- From `agents-plugin-pi/`, run focused fork-context, readiness-regression, lifecycle, and prefix-serialization tests, then run `npm test`.
- Confirm the parent-only-extension lifecycle reproducer passes readiness with byte-identical ordered provider definitions; the stub is visible, fails truthfully, the notice appears only in first task input, and `ws-report-to-lead` remains usable for the final report.
- Confirm no-mismatch forks add no stub/notice; missing `ws-report-to-lead`, extra tools, order drift, schema drift, and description drift reject before a provider turn; worker/execute-worker behavior remains unchanged.
- After the changed adapter is loaded, owner-run the blocked fork-raised question acceptance from `260911-bug-ws-pi-question-queue-dogfood`; record that live result separately rather than treating offline tests as acceptance.

## Escalations
- None.
