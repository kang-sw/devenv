# Plan: 260905-feat-ws-pi-harness-config-layer — Phase 1: `ws-execute complex:true` inherits the lead model (Pi track)

## Relevant Ticket Contract

- Phase 1 text (ticket `## Phases`): "Today `complex:true` selects a `"complex"` catalog alias and only inherits the lead's model by accident (catalog miss -> inherit fallback); a user who adds a `complex` entry would silently change its meaning. Make `complex:true` pass no `model_name` at all (inherit), keep `complex:false` on `"small"`, delete the `"complex"` alias from `resolveExecuteModelAlias`, the tool description, `pi-lead-guide.md`, and the `pi-adapter-runtime` `ws-execute` wording ("a light-model default; the lead's own model when set"). Update the tests. Verify with `npm test`."
- Phase 1 text (second half): "Also in this phase (Open Decisions #3, adapter half): the bridge filters `mercenary.*` out of the ws-mcp tool list before registering tools with Pi, so no Pi process — lead or child — can see or call the mercenary surface. Record the filter in `pi-adapter-runtime` next to the bridged-tool inventory and cover it with a bridge test."
- Open Decision #1 (settled, owner 2026-09-05): "`complex:true` was always meant to **inherit the lead's model** (no alias at all — today it only does so by accident, through the catalog-miss fallback) and is corrected on the Pi track independently of the ws-side phases; no alias writer or non-tier resolver is added."
- Open Decision #3 (settled, owner 2026-09-05): "the bridge drops `mercenary.*` from the tool list it registers with Pi (adapter-side filter after `tools/list`, independent of the global `workflow.prefer_mercenary` knob, so Codex and Claude sessions are unaffected)... The adapter half lands in Phase 1."
- Spec Impact: "`pi-adapter-runtime`: ...the bridged-tool inventory gains the `mercenary.*` filter (Phase 1)... the three model-resolution anchors (...) are rewritten in Phase 4" — Phase 1 touches only the bridged-tool-inventory section and the `ws-execute` wording inside the approval-gateway section, NOT the three Phase-4 model-resolution anchors (`{#260903-pi-spawner-model-tier-inherit}`, `{#260903-pi-model-catalog-config-file}`, `{#260903-pi-model-catalog-unset-advisory}`).
- Decisions section: "Golden rule for the Pi track is respected by sequencing... Phases 1 and 4 are adapter changes on the Pi track" — this phase touches only `agents-plugin-pi/`.
- Non-goal: "Reworking `mercenary` backends to launch Pi processes; `backend` for a `pi` entry is informational until a Pi mercenary backend exists." (confirms the filter is a containment/UX measure, not a backend rework.)

## Out of Scope

- Phases 2-4 (harness enum, tier-resolution read tool, catalog retirement) — ws-mcp Go changes and the catalog-deletion adapter changes are separate phases with their own gating (Phase 4 depends on a released ws pinning Phase 3).
- Rewriting the three Phase-4 model-resolution spec anchors (`Model resolution: name alias, not tier`, `Model catalog data file`, `Unset-catalog advisory`) — explicitly scheduled for Phase 4 per Spec Impact.
- Any change under `agents-plugin-tool/` (ws-mcp Go source) or `agents-plugin/skills/` — Phase 1 is Pi-track only.
- Splitting the harness/backend normalizer or adding a mercenary-from-`pi` rejection error in ws-mcp — Open Decision #3 explicitly rejects touching ws-mcp for this: "No normalizer split and no new error branch in ws-mcp."
- `model-catalog.json`/`model-catalog.ts` changes — the file is currently `{}` (no `"complex"` alias entry exists to remove), and the resolver change is a code-level string constant, not a data-file edit.

## Codebase Findings

- `agents-plugin-pi/src/execute-gateway.ts#L159-L167` — `resolveExecuteModelAlias(complex?: boolean): string` currently returns the literal `"complex"` when true, `"small"` when false/omitted. Must become `string | undefined`, returning `undefined` (inherit) for `true`.
- `agents-plugin-pi/src/execute-gateway.ts#L576-L581` — the only call site: `spawnAgent(rpcRegistry, {...}, { systemPromptPath, prompt, modelName: resolveExecuteModelAlias(p.complex) })`. `SpawnAgentParams.modelName` (`spawner.ts#L1442`) is already `modelName?: string`, so passing `undefined` is a legal, unmodified call shape — no signature change needed downstream.
- `agents-plugin-pi/src/spawner.ts#L2054-L2055` — `spawnAgent`'s own resolution: `const catalogConfig = params.modelName ? readModelCatalog(...) : undefined; const modelBase = resolveModelForAlias(catalogConfig, params.modelName, ctx.inheritModel);`. Confirms the mechanism: an `undefined`/falsy `modelName` skips the catalog read entirely and `resolveModelForAlias` (`spawner.ts#L350-L360`) falls straight to `inheritModel` when `alias` is falsy — this IS the "no `model_name` at all -> inherit" behavior the ticket wants, already implemented and just needs to be reached deliberately instead of by catalog-miss accident.
- `agents-plugin-pi/src/execute-gateway.ts#L22-L26` — header doc comment's `ws-execute` bullet: "`complex?` selects the `"complex"` model alias instead of the default `"small"` one." Stale after the change; same file being edited, low-risk touch-up alongside the function it documents.
- `agents-plugin-pi/src/execute-gateway.ts#L544` — `ws-execute` tool `description` string: "...Optionally runs `command` verbatim FIRST... complex:true selects a stronger model. Returns {agent_id} immediately...".
- `agents-plugin-pi/src/execute-gateway.ts#L553` — `complex` parameter schema description: `"true selects the 'complex' model alias instead of the default 'small' alias."`.
- `agents-plugin-pi/test/execute-gateway.test.ts#L92-L101` — `describe("resolveExecuteModelAlias", ...)`: `test("complex:true resolves to the \"complex\" alias", () => assert.equal(resolveExecuteModelAlias(true), "complex"))` and the `false`/`undefined` -> `"small"` test (keep the latter, rewrite the former to assert `undefined`).
- `agents-plugin-pi/pi-lead-guide.md#L45` — verb table row: `` `ws-execute` (spawns an execute-worker; optional `command` runs verbatim first, then `prompt` drives the worker; `complex:true` for a stronger model). `` — not mirrored to `agents-plugin/skills/` (confirmed: `pi-lead-guide.md` exists only under `agents-plugin-pi/`), so the ticket's wsflow-mirroring constraint does not apply here.
- `ai-docs/spec/pi-adapter-runtime.md#L586-L592` (section `## Lead-execute approval gateway {#260905-pi-execute-approval-gateway}`) — the `ws-execute` bullet: `` `complex?` selects the worker's model tier only (a light-model default; a lead-class model when set). `` The ticket's Phase 1 text names the exact target wording verbatim: "a light-model default; the lead's own model when set" — swap only "a lead-class model" -> "the lead's own model" (minimal diff matching the ticket's quoted phrase).
- `ai-docs/spec/pi-adapter-runtime.md#L23-L51` (section `## Tool exposure and name sanitization {#260903-pi-bridge-tool-registration}`) — this is "the bridged-tool inventory" the ticket refers to for the `mercenary.*` filter note: it currently states "Every tool the ws-mcp server advertises through `tools/list` is registered as a Pi tool," which becomes false once the filter lands and needs a qualifying sentence.
- `agents-plugin-pi/src/bridge.ts#L264-L266` — `sanitizeToolName(rawName)` is the existing precedent for a small, pure, exported helper next to the IO glue (`startBridge`), unit-tested directly in `test/bridge.test.ts`. The new mercenary filter should follow the same shape.
- `agents-plugin-pi/src/bridge.ts#L390,#L406-L409` — `let tools: Awaited<ReturnType<typeof client.listTools>> = [];` then `tools = await client.listTools(); for (const tool of tools) { ...pi.registerTool(...) }`. This is the single choke point: filtering `tools` immediately after the `listTools()` call (before the `for` loop) also fixes `wsToolNames` (below) and the registration-count notify line for free.
- `agents-plugin-pi/src/bridge.ts#L557` — `wsToolNames: tools.map((tool) => sanitizeToolName(tool.name))`, exposed on `BridgeHandle`. Traced via `grep`: consumed by `spawner.ts#L2058` (`resolveTools(toolGroup, ctx.wsToolNames)`, only for `toolGroup === "full-worker"`) and by `execute-gateway.ts#L571`/`spawner.ts#L2518`/`fork.ts#L587` as the `wsToolNames` passed into every `spawnAgent` call. Filtering the shared `tools` array (not just the registration loop) is required to also keep `mercenary.*` out of every spawned child's `--tools` list — matching the ticket's "no Pi process — lead or child — can see or call the mercenary surface."
- `agents-plugin-pi/src/mcp-stdio-client.ts#L28-L32` — `McpToolInfo { name: string; description?: string; inputSchema: Record<string, unknown> }` is the element type of `client.listTools()`'s resolved array; the filter helper should be typed against this (or a minimal structural `{ name: string }`) for a clean import.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L205-L221` and `agents-plugin-tool/internal/mcp/server.go#L1535-L1704` (`case "mercenary.register":` ... `case "mercenary.erase":`) — confirms the exact wire form every mercenary tool name takes: dotted, e.g. `mercenary.register`, `mercenary.call`, `mercenary.wait`, `mercenary.result`, `mercenary.status`, `mercenary.interrupt`, `mercenary.tail`, `mercenary.debug.tail`, `mercenary.debug.stdout`, `mercenary.debug.stderr`, `mercenary.debug.runtime_log`, `mercenary.debug.events`, `mercenary.cancel`, `mercenary.recall`, `mercenary.print`, `mercenary.erase`. All match a plain `startsWith("mercenary.")` prefix check on the raw (pre-sanitization) name — this is the name form the bridge actually receives from `tools/list`, since `agents-plugin-tool/internal/mcp/server.go#L4666-L4668` (`advertisedToolName`) is the identity function and never renames mercenary tools.
- `agents-plugin-tool/internal/mcp/server.go#L4580-L4598` (`filteredTools`) — ws-mcp itself already hides `mercenary.*` from `tools/list` when `workflow.prefer_mercenary` resolves to `"hide"` globally, but NOT otherwise; this is exactly why Open Decision #3 requires an adapter-side filter "independent of the global `workflow.prefer_mercenary` knob" — a Pi session cannot rely on that server-side gate being on.
- `agents-plugin-pi/test/bridge.test.ts#L37-L57` — existing `LIVE_TOOL_NAMES` fixture (60 tools, captured live) contains zero `mercenary.*` entries in this repo's current default config; the new test must construct its own small fixture that explicitly includes `mercenary.*` names (do not rely on the live fixture to exercise the filter, since it happens to already lack them).
- `agents-plugin-pi/package.json#L20` — `"test": "node --test"`; run from `agents-plugin-pi/` (matches ticket's "Verify with `npm test`").

## Implementation Plan

1. `agents-plugin-pi/src/execute-gateway.ts#L159-L167` — change `resolveExecuteModelAlias`'s signature to `(complex?: boolean): string | undefined` and its body to `return complex ? undefined : "small";`. Update its doc comment to state that `true` returns `undefined` so `spawnAgent` inherits the lead's own model via the existing `resolveModelForAlias` inherit-fallback path (no new alias, no catalog read), and `false`/omitted still returns `"small"`.
2. `agents-plugin-pi/src/execute-gateway.ts#L22-L26` — update the header doc comment's `ws-execute` bullet to match: "`complex?` inherits the lead's own model (no `model_name` passed) instead of the default `"small"` alias" (or equivalent), removing the `"complex"` alias mention.
3. `agents-plugin-pi/src/execute-gateway.ts#L544` — update the `ws-execute` tool `description` string: replace "complex:true selects a stronger model." with wording reflecting inherit-the-lead's-model behavior (e.g. "complex:true inherits your own model instead of the default light one.").
4. `agents-plugin-pi/src/execute-gateway.ts#L553` — update the `complex` parameter's schema `description`: replace `"true selects the 'complex' model alias instead of the default 'small' alias."` with e.g. `"true inherits your own model instead of the default 'small' alias."`.
5. `agents-plugin-pi/test/execute-gateway.test.ts#L92-L100` — rewrite the first test to assert `resolveExecuteModelAlias(true) === undefined` (rename it to reflect "inherit the lead's own model"); keep the `false`/`undefined` -> `"small"` test unchanged.
6. `agents-plugin-pi/pi-lead-guide.md#L45` — replace "`complex:true` for a stronger model" with wording matching the corrected behavior (e.g. "`complex:true` to inherit your own model instead of the default light one").
7. `ai-docs/spec/pi-adapter-runtime.md#L591` — replace "`complex?` selects the worker's model tier only (a light-model default; a lead-class model when set)." with "`complex?` selects the worker's model tier only (a light-model default; the lead's own model when set)." (the ticket's own quoted target wording — minimal diff).
8. `agents-plugin-pi/src/bridge.ts` — add a new pure exported helper near `sanitizeToolName` (after `#L266`), e.g.:
   ```ts
   const MERCENARY_RAW_PREFIX = "mercenary.";

   /**
    * Drops every ws-mcp tool whose raw (pre-sanitization) name starts with
    * `mercenary.` from the list the bridge registers with Pi and exposes via
    * `wsToolNames` — independent of the server-side `workflow.prefer_mercenary`
    * knob (Open Decision #3, 260905-feat-ws-pi-harness-config-layer): no Pi
    * process, lead or child, can see or call the mercenary surface. Pure so it
    * is unit-testable without a live ws-mcp subprocess.
    */
   export function filterOutMercenaryTools<T extends { name: string }>(tools: readonly T[]): T[] {
     return tools.filter((tool) => !tool.name.startsWith(MERCENARY_RAW_PREFIX));
   }
   ```
9. `agents-plugin-pi/src/bridge.ts#L406` — change `tools = await client.listTools();` to `tools = filterOutMercenaryTools(await client.listTools());`, so both the registration loop (`#L408-L479`) and `wsToolNames` (`#L557`) see the filtered list, and the `notify(... registered ${tools.length} ws__* tools ...)` line (`#L547`) reports the true post-filter count.
10. `ai-docs/spec/pi-adapter-runtime.md#L23-L51` (`## Tool exposure and name sanitization {#260903-pi-bridge-tool-registration}`) — add a sentence noting that the bridge drops every `mercenary.*` raw tool name before registration/before building `wsToolNames`, regardless of the server's own `workflow.prefer_mercenary` visibility setting, so no Pi lead or spawned child can see or call the mercenary surface.
11. `agents-plugin-pi/test/bridge.test.ts` — import `filterOutMercenaryTools` from `../src/bridge.ts` and add a `describe("filterOutMercenaryTools", ...)` block with at least: (a) a fixture list mixing ordinary names (`"playbook.print"`, `"ferrule"`) with several `mercenary.*` names (including a nested-dot one like `"mercenary.debug.tail"`) asserting only the non-mercenary names survive, in original order; (b) a no-mercenary-present fixture asserting the list passes through unchanged.

## Verification Plan

- `cd agents-plugin-pi && npm test` — full unit suite (existing tests plus the updated `resolveExecuteModelAlias` test and the new `filterOutMercenaryTools` tests) must pass.
- Manual/code-inspection check (no live model needed, matching this ticket's own scope — no live-gate item is named for Phase 1): confirm no remaining `"complex"` string-literal alias reference exists outside historical comments by re-grepping `agents-plugin-pi/` for `"complex"` after the edit.

## Escalations

- None.
