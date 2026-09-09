# Plan: Stateless in-window dedupe for `playbook.read` and `ws-skill`: a repeat read of an unchanged body returns a short pointer instead of the body — Phase 1: Window scan, short reply, wiring, tests

## Relevant Ticket Contract
- Add adapter-only, stateless dedupe for `ws__playbook_read` and `ws-skill`: inspect `toolCtx.sessionManager.buildContextEntries()` at `execute()` time; never retain flags, counters, or caches.
- Match only the same tool family and semantic key: playbook name plus its `context` substitution map, or ws-skill name plus `args`; ignore `session_key` and object key order. Run the normal call/read first and point only when its fresh text is byte-identical to a visible earlier successful full result.
- The second matching read returns one normal text paragraph naming the original `toolCallId` and listing the body’s `#`/`##`/`###` headings. The third and later matching reads return the full body. Count a pointer only when its explicit provenance resolves to a visible, successful, byte-identical original full result for the same key; missing, forged, stale, or ambiguous provenance returns the body.
- Preserve Pi’s public active-context behavior: compaction removes prior reads, rewound abandoned branches do not count, and fork prefixes do count. Exclude the current `toolCallId` regardless of Pi’s insertion timing. Document the behavior in `ai-docs/spec/pi-adapter-runtime.md`.

## Out of Scope
- `playbook.render`, `workflow_manual` / its static-body mapping, ws-mcp, shared rsrc, Go code, schemas, and any `force` parameter.
- Adapter-side session history, caches, or persistence; tool-call hook blocking; owner-run dogfood execution.

## Codebase Findings
- `agents-plugin-pi/src/bridge.ts#L748-L820` — every bridged MCP tool shares one registered `execute()` closure, which has the raw name, normalized arguments, current call id, extension context, and result before it is converted to Pi content; this is the `ws__playbook_read` wiring point.
- `agents-plugin-pi/src/lead-skills.ts#L84-L109` and `agents-plugin-pi/src/lead-skills.ts#L242-L262` — `ws-skill` is a separate registered tool that resolves its live skill map and body in `execute()`; inject the same pure decision before returning its existing text result.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/session-manager.d.ts#L140-L166` and `dist/core/session-manager.js#L198-L225` — installed Pi exposes `ReadonlySessionManager.buildContextEntries()` and constructs the active leaf path with its latest compaction cut, so no adapter-side tree or compaction implementation is needed.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-agent-core/dist/agent-loop.js#L121-L145` and `#L451-L480` — Pi emits and persists the finalized assistant tool-call message before invoking `tool.execute`; the scanner must remove the current call by id, and a real-session test should lock this ordering.
- `agents-plugin-pi/test/lead-skills.test.ts#L1-L26` — existing pure-module tests use injected loaders and a fake Pi registration; add the ws-skill fixture coverage here. `agents-plugin-pi/test/fork-lifecycle.integration.test.ts#L8-L18` already uses the installed SessionManager and is the established place for a small production-context timing/branch oracle if a unit fixture cannot exercise `buildContextEntries()` directly.
- `ai-docs/spec/pi-adapter-runtime.md#L23-L121` — the Tool exposure section owns observable bridged-tool behavior and is the appropriate adjacent subsection for this contract.

## Implementation Plan
1. Add a small pure `agents-plugin-pi/src/playbook-read-dedupe.ts` module. Define narrow session-message guards, semantic stable-key construction, text-result extraction, heading rendering, and an explicit pointer-provenance envelope that cannot be mistaken for ordinary tool text. Given visible entries, current id, family/key, and fresh body, it must resolve only successful matching full results and validated pointers back to their original full-result id, then decide `body` versus `pointer` with the mandated first/second/third behavior.
2. Use the module’s scanner only with `toolCtx.sessionManager.buildContextEntries()` in `agents-plugin-pi/src/bridge.ts` for raw `playbook.read`, after the MCP result succeeds and its text is available. Return the normal details envelope with the substituted pointer text; retain ordinary error handling, session-key normalization, and all other bridged tools unchanged.
3. Give `registerWsSkillTool` in `agents-plugin-pi/src/lead-skills.ts` the same current-call/context access, use the live resolved skill text as the fresh body, and apply the shared helper under its distinct `ws-skill` family/key. Ensure neither tool can dedupe against the other.
4. Extend `agents-plugin-pi/test/` with fixture-driven scanner tests for semantic map ordering, ignored `session_key`, current-call exclusion, changed body/key, failed results, compaction, rewind, and inherited fork prefix. Cover full → pointer → full → full; verify only authentic visible full-result provenance validates the pointer and malformed/forged/stale/ambiguous provenance falls back to body. Exercise both tool families and heading-only pointer rendering.
5. Add a focused installed-Pi SessionManager fixture or integration assertion that observes the assistant tool-call entry while `execute()` runs, pinning the current-id exclusion under actual insertion ordering. Add bridge and ws-skill registration tests that prove each wiring path receives the short pointer only for its own repeat.
6. Add the compact dedupe subsection under `ai-docs/spec/pi-adapter-runtime.md`’s tool-exposure section: active-window scan, byte-identical fresh-result gate, pointer shape and provenance, third-call pass-through, compaction/rewind/fork implications, and the two covered tools.

## Verification Plan
- From `agents-plugin-pi/`, run the focused Node test files for the new dedupe module plus `test/bridge.test.ts` and `test/lead-skills.test.ts`; include the focused real-SessionManager timing fixture if it is separate.
- Run `node --test test/*.test.ts` only after focused tests pass, recording the known baseline separately: 130 existing failures (129 Linux SDK-path fixtures and one stale `ws-ask` exposure expectation). The installed Node 25 treats `node --test test/` as a module path and rejects it; the explicit glob is the verified suite command. Do not add or re-enable `ws-ask` to change that baseline.
- Owner dogfood remains the ticket’s manual verification boundary: re-enter `lead-discuss` or `lead-proceed`, confirm the second unchanged read is a usable pointer and no third read is needed.

## Escalations
- None.
