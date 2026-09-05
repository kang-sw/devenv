# Plan: 260905-feat-ws-pi-harness-config-layer — Phase 4: Pi adapter resolves models through ws-mcp; catalog retires

## Relevant Ticket Contract

- Phase 4 text: route `resolveModelForAlias` (spawner) and the explore leaf's /
  `ws-execute complex:false`'s `"small"` through the bridge to the Phase 3
  `config.resolve_agent` tool, treating a non-`pi` answering bucket as
  inherit; carry `effort` into `modelEffort`; replace the "model catalog
  unset" `workflow_manual` advisory with an "alias table has no `pi` entries"
  advisory sourced from the same tool; delete `model-catalog.ts`,
  `model-catalog.json`, and their tests; update the three
  `pi-adapter-runtime` model-resolution anchors and `pi-lead-guide.md`.
  Verify with `npm test` and out-of-band live spawns.
- Decision ("One model table"): the model layer is the four fixed tiers
  (`small|medium|large|xlarge`) under harness `pi` — no user-named alias
  concept. `complex:true` already inherits unconditionally (Phase 1, landed);
  do not reintroduce an alias for it.
- Decision ("The adapter reads the resolution from ws-mcp"): never parse
  config directly; ask ws-mcp through the read tool; missing/unmapped stays
  "inherit", never an error (never-hard-fail).
- Decision (effort): empty resolved effort passes no `modelEffort`
  (leaves the child's default); `low|medium|high|xhigh` pass through
  unchanged; a caller-supplied `model_effort` always wins over the
  config-resolved effort.
- Phase 3 Forward (a): a partial `config.tune agents.tier harness:pi` write
  can seed the `pi` bucket from the codex default, so `resolved_from == "pi"`
  alone is not proof of a Pi model string — guard on both `resolved_from ===
  "pi"` and the model string looking like a Pi `provider/id` (contains `/`;
  confirmed in `agents-plugin-tool/internal/wsconfig/config_test.go:700` that
  a codex-seeded default model, e.g. `gpt-5.6-terra`, carries no `/`, so
  "contains a slash" cheaply separates a real Pi entry from a codex-shaped
  fallback answering under the `pi` label).
- Phase 3 Forward (b): a harness typo silently resolves at `default`; the
  adapter passes no explicit `harness` argument to `config.resolve_agent`, so
  it relies on the bridge's detected session harness. Confirmed
  (`agents-plugin-tool/internal/mcp/server.go:882-896`): when `harness` is
  omitted, the server uses `s.currentHarness()`, and the `pi` bridge's
  `clientInfo.name` (`ws-pi-bridge`) is detected as harness `pi` at
  `initialize` (Phase 2, landed) — so the adapter's no-explicit-harness calls
  correctly land in the `pi` bucket when one is configured. No extra code is
  needed to thread harness explicitly; not echoing the effective harness in
  the payload (Forward (b)'s alternative) is left as-is per "settled" scope —
  this plan does not add that echo.
- Non-goal: do not touch `agents-plugin-tool/` or `agents-plugin/skills/` in
  this phase (Phase 3 already landed the read tool).

## Out of Scope

- Any change to `agents-plugin-tool/` (Go) or `agents-plugin/skills/` —
  Phase 3 already shipped `config.resolve_agent`.
- Authoring `.pi.md` overlays (ticket-level non-goal).
- Reworking mercenary backends for Pi (ticket-level non-goal; Phase 1 already
  filters `mercenary.*` from the Pi tool list).
- Echoing the effective harness in `config.resolve_agent`'s payload (Phase 3
  Forward (b)'s alternative) — not requested by Phase 4's text.
- Adding a "list what alias/tier resolved" user-facing command beyond the
  existing `ws-model-catalog-list`; only its description text needs updating
  since its underlying behavior (list `provider/id` candidates from Pi's own
  model registry) stays useful for curating `config.tune agents.tier
  harness:pi` even after the catalog file is gone.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go#L882-904` — `config.resolve_agent`
  handler: `tier` and optional `harness` args; `harness` defaults to
  `s.currentHarness()` when omitted/empty; returns
  `{backend, model, effort, resolved_from}` via `toolJSONResponse` when
  `format:"json"` is passed (`wantsJSON`, `server.go#L1757-1760`), else a
  `key: value\n` text block. An unknown tier is a Go error (`isError: true`,
  never a thrown protocol error) — the adapter's existing "throw on
  `isError`, otherwise degrade" pattern is not used here; this call must
  swallow an error result and inherit instead (never-hard-fail).
- `agents-plugin-pi/src/spawner.ts#L340-360` — `resolveModelForAlias(config,
  alias, inheritModel)`: pure, sync, file-catalog-backed. This is the
  function Phase 4 replaces with an async ws-mcp-backed equivalent.
- `agents-plugin-pi/src/spawner.ts#L369-372` — `inheritModelFromToolCtx`
  unchanged; still the inherit-model source.
- `agents-plugin-pi/src/spawner.ts#L2103-2132` — `spawnAgent`: the one place
  `resolveModelForAlias`/`readModelCatalog` are called for
  `ws-agent-spawn`/`ws-fork`/`ws-execute`/the discussion fork (all four route
  through this function via `params.modelName`); `record.modelEffort =
  params.modelEffort` at L2132 is the effort assignment point — the
  config-resolved effort must be folded in here only when the caller passed
  no `model_effort` (decision: explicit always wins).
- `agents-plugin-pi/src/spawner.ts#L2529-2538` — `resolveExploreModel`: a
  SEPARATE IO wrapper (not through `spawnAgent`/`params.modelName`) used only
  by the `explore` tool's implicit `"small"` lookup at L2749. Already inside
  an `async execute(...)` — becomes `await resolveExploreModel(toolCtx)`.
- `agents-plugin-pi/src/spawner.ts#L1507-1521` — `RpcSpawnCtx`: replace the
  `modelCatalogPath: string` field with `client: McpStdioClient` (already
  imported at L81); every call site below already has a `bridge: BridgeHandle`
  (or the client itself) in scope where it currently passes
  `modelCatalogPath: sessionCtx.modelCatalogPath`.
- `agents-plugin-pi/src/spawner.ts#L2591` (`ws-agent-spawn` tool),
  `agents-plugin-pi/src/execute-gateway.ts#L576` (`ws-execute` tool),
  `agents-plugin-pi/src/fork.ts#L588` (`buildForkSpawnCtx`),
  `agents-plugin-pi/src/ask.ts#L1187` (discussion-fork spawn in
  `ensureRespondent`/`openThread`) — the four `spawnAgent` ctx-construction
  call sites that currently set `modelCatalogPath: sessionCtx.modelCatalogPath`;
  each already has `bridge`/`handle` in its own parameter list, so this
  becomes `client: bridge.client` (or `handle.client`) at each site.
- `agents-plugin-pi/src/execute-gateway.ts#L169-171` —
  `resolveExecuteModelAlias(complex)` returns `"small"` / `undefined`; no
  change needed here — it feeds `modelName` into the same centralized
  `spawnAgent` resolution.
- `agents-plugin-pi/src/execute-gateway.ts#L434` (`ExecuteGatewaySessionCtx`),
  `agents-plugin-pi/src/fork.ts#L318` (`ForkSessionCtx`),
  `agents-plugin-pi/src/ask.ts#L685-688` (`AskSessionCtx`) — each carries a
  `modelCatalogPath: string` field that becomes unused once the ctx-construction
  call sites above read `bridge.client` directly instead; drop the field from
  each interface (all three call sites already receive `bridge`/`handle`
  independently of `sessionCtx`, so nothing needs to be added to these
  interfaces — only removed).
- `agents-plugin-pi/src/index.ts#L182,289,300,304,330,397` — the single
  `modelCatalogPath` constant and its five call-site references
  (`startBridge`, `registerAgentTools`, `registerExecuteGateway`,
  `registerFork`, `registerThreadCommands`); drop the constant and all five
  references once the consumers no longer need it. `startBridge`'s own
  `BridgeOpts.modelCatalogPath` (`bridge.ts#L39`) and `WorkflowManualMappingDeps
  .modelCatalogPath` (`bridge.ts#L204`) are separate fields covered below.
- `agents-plugin-pi/src/bridge.ts#L45-53` (`BridgeHandle`, already exposes
  `.client: McpStdioClient` — no change needed to this interface),
  `bridge.ts#L91-93` (`firstText`, reusable), `bridge.ts#L102-110`
  (`MODEL_CATALOG_ADVISORY` text — rewrite to the "alias table has no `pi`
  entries" wording the ticket specifies), `bridge.ts#L121-130`
  (`maybeAppendModelCatalogAdvisory` — currently takes a sync
  `ModelCatalogConfig | undefined`; change its third parameter to a plain
  `piAliasTableUnset: boolean` so the pure gate/append logic stays
  synchronously unit-testable, with the async ws-mcp check computed by the
  caller before invoking it), `bridge.ts#L199-207`
  (`WorkflowManualMappingDeps` — drop `modelCatalogPath`, no new field needed
  since `callTool` is already present), `bridge.ts#L228-255`
  (`dispatchMappedWorkflowManual` — replace the sync `readModelCatalog(deps
  .modelCatalogPath)` at L239 with an async `computePiAliasTableUnset(deps
  .callTool)` call, awaited before both `return` branches), `bridge.ts#L460-490`
  (the raw/unmapped dispatch path — L489's `maybeAppendModelCatalogAdvisory(
  rawName, result.content, readModelCatalog(opts.modelCatalogPath))` must
  become async too; gate the extra `config.resolve_agent` round-trip on
  `rawName === "workflow_manual"` first so every other bridged tool call does
  not pay for an unrelated MCP round-trip).
- New helper (design choice, not in current code): a small
  `computePiAliasTableUnset(callTool): Promise<boolean>` in `bridge.ts` that
  calls `config.resolve_agent` for each of the four fixed tiers
  (`small|medium|large|xlarge`) with `format:"json"`, applies the same
  `resolved_from === "pi" && model.includes("/")` guard as the spawn-side
  resolver, and returns `true` (advisory fires) only when none of the four
  tiers has a genuine `pi` hit. Four local stdio round-trips per
  `workflow_manual` call (only) is acceptable — mirrors the existing
  per-call, no-caching contract the advisory already had.
- `agents-plugin-pi/src/model-catalog.ts` (whole file, 79 lines) — delete.
  `agents-plugin-pi/model-catalog.json` (data file) — delete.
  `agents-plugin-pi/test/model-catalog.test.ts` (101 lines, tests only
  `readModelCatalog`/`resolveAlias`/`isModelCatalogUnset`) — delete wholesale
  (confirmed no test in it exercises anything outside `model-catalog.ts`).
  `agents-plugin-pi/package.json#L15` — remove `"model-catalog.json"` from the
  `"files"` array.
  `agents-plugin-pi/src/index.ts#L220-228` — `ws-model-catalog-list` command:
  keep the command (it still lists Pi `provider/id` candidates, still useful
  input for `config.tune agents.tier harness:pi` / `lead-tune`), but its
  description text ("for curating model-catalog.json's tiers") references a
  file that no longer exists — reword to point at `config.tune agents.tier
  harness:pi` / `lead-tune` instead.
- Test files referencing the removed surface (need edits, not deletion):
  `agents-plugin-pi/test/spawner.test.ts` (imports `resolveModelForAlias`,
  `ModelCatalogConfig`; a `describe("resolveModelForAlias", ...)` block
  L439-472 exercises the pure sync function directly — replace with tests for
  the new async ws-mcp-backed resolver against a stub `callTool`, covering: a
  genuine `pi` hit wins, a non-`pi` `resolved_from` inherits, a `pi`-labeled
  but slash-less (codex-shaped) model inherits (Forward (a) guard), an
  `isError` result inherits, no-alias/no-modelName inherits, effort carried
  through only on a genuine `pi` hit and only when no caller `model_effort`
  was given), `agents-plugin-pi/test/fork.test.ts#L648-673` (`buildForkSpawnCtx`
  calls pass `modelCatalogPath` — swap for `client`/a stub client),
  `agents-plugin-pi/test/ask.test.ts#L1154` (`sessionCtx` literal with
  `modelCatalogPath` — drop the field), `agents-plugin-pi/test/bridge.test.ts
  #L337,360,382,398` (`WorkflowManualMappingDeps` literals with
  `modelCatalogPath` — drop the field and supply/stub the `config.resolve_agent`
  responses the new `computePiAliasTableUnset` path needs via the existing
  `callTool` stub).
- `ai-docs/spec/pi-adapter-runtime.md#L420-465` — the three anchors to
  rewrite in place (keep the anchor ids/headings stable — no
  `renamed-spec:` needed, this is a content rewrite, not a slug change):
  - `{#260903-pi-spawner-model-tier-inherit}` (L420-435, "Model resolution:
    name alias, not tier") — rewrite to describe `model_name` as one of the
    four fixed tier names, resolved through `config.resolve_agent` (harness
    `pi`, defaulting to `default`/inherit on any non-`pi` answer), not a
    user-curated alias table.
  - `{#260903-pi-model-catalog-config-file}` (L437-454, "Model catalog data
    file") — rewrite to describe the ws-config-backed resolution path
    (no adapter-owned data file); replace the "no Pi model strings are placed
    in the harness-neutral ws-mcp core" sentence with "user config may carry
    Pi model strings; adapter and core code may not" per the ticket's Spec
    Impact wording. Keep (reword only) the `ws-model-catalog-list` paragraph
    to point at `config.tune agents.tier harness:pi`.
  - `{#260903-pi-model-catalog-unset-advisory}` (L456-465, "Unset-catalog
    advisory") — rewrite to describe the "alias table has no `pi` entries"
    advisory sourced from `config.resolve_agent`, same cadence (recomputed
    per `workflow_manual` call, appended not prepended, only on success).
  - Confirmed no other live spec/doc cross-references these three anchor ids
    besides ticket/plan history under `.done/`/`.plans/` (grep checked) — no
    other file needs updating for the rewrite itself.
- `agents-plugin-pi/pi-lead-guide.md` — does not currently contain a literal
  "anything the user names" sentence; that phrase lives only in
  `model-catalog.ts`'s doc comment (deleted) and is paraphrased in the old
  `bridge.ts` advisory text (rewritten above). `pi-lead-guide.md`'s only
  `model_name` mention is the `ws-fork` verb-table row
  (`pi-lead-guide.md#L47`: "optional `model_name`/`expects_commit`") and the
  `ws-agent-spawn` row (`pi-lead-guide.md#L33`) does not mention `model_name`
  at all. Update: (1) the `ws-fork` row to say `model_name` is one of the
  tier names `lead-tune`/`config.list` shows for harness `pi`, not a
  free-form name; (2) add a short `model_name` mention to the `ws-agent-spawn`
  row for the same reason (currently silent on it, which under the old
  catalog was harmless since any name was valid — under a fixed tier
  vocabulary a lead needs to be told what values are legal). Also update the
  two tool-description strings that still say "resolved against
  model-catalog.json's aliases map": `spawner.ts`'s `ws-agent-spawn`
  `model_name` param description (`spawner.ts#L2553-2556`) and `fork.ts`'s
  `ws-fork` `model_name` param description (`fork.ts#L670-673`).

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts`: add an async
   `resolveModelForAliasViaWsMcp(client: McpStdioClient, alias: string |
   undefined, inheritModel: string | undefined): Promise<{model?: string;
   effort?: string}>` next to the old `resolveModelForAlias` (L340-360):
   no alias -> `{model: inheritModel}`; else call `client.callTool(
   "config.resolve_agent", {tier: alias, format: "json"})`; on `isError`,
   missing/unparsable text, or `resolved_from !== "pi"`, or a `pi`-labeled
   model with no `/` -> `{model: inheritModel}`; otherwise `{model: parsed
   .model, effort: parsed.effort || undefined}`. Delete the old
   `resolveModelForAlias` and its `ModelCatalogConfig`/`resolveAlias` import
   once nothing references them (keep `readModelCatalog`'s import removed
   too).
2. `agents-plugin-pi/src/spawner.ts`: change `RpcSpawnCtx.modelCatalogPath`
   (L1521) to `client: McpStdioClient`. In `spawnAgent` (L2120-2132), replace
   the sync catalog read + `resolveModelForAlias` call with `const
   {model: modelBase, effort: resolvedEffort} = await
   resolveModelForAliasViaWsMcp(ctx.client, params.modelName, ctx
   .inheritModel);` and set `record.modelEffort = params.modelEffort ??
   resolvedEffort` (explicit caller effort wins, per Decisions).
3. `agents-plugin-pi/src/spawner.ts`: update `resolveExploreModel` (L2529-2538)
   to `async function resolveExploreModel(toolCtx): Promise<string |
   undefined>` calling `resolveModelForAliasViaWsMcp(bridge.client, "small",
   inheritModelFromToolCtx(toolCtx))` and returning just `.model`; update its
   one call site (L2749) to `model: await resolveExploreModel(toolCtx)`
   (already inside `async execute`).
4. `agents-plugin-pi/src/spawner.ts` L2591: change
   `modelCatalogPath: sessionCtx.modelCatalogPath` to `client: bridge.client`
   in the `ws-agent-spawn` `spawnAgent` ctx. Update the `model_name` param
   description (L2553-2556) to name the four tiers instead of
   "model-catalog.json's aliases map".
5. `agents-plugin-pi/src/execute-gateway.ts`: drop `modelCatalogPath` from
   `ExecuteGatewaySessionCtx` (L434); change L576's
   `modelCatalogPath: sessionCtx.modelCatalogPath` to `client: bridge.client`.
6. `agents-plugin-pi/src/fork.ts`: drop `modelCatalogPath` from
   `ForkSessionCtx` (L318); change `buildForkSpawnCtx` (L588) to set
   `client: bridge.client` instead. Update the `ws-fork` `model_name` param
   description (L670-673).
7. `agents-plugin-pi/src/ask.ts`: drop `modelCatalogPath` from `AskSessionCtx`
   (L685-688); change the discussion-fork `spawnAgent` ctx construction
   (L1187) to `client: bridge.client` (the enclosing functions already take
   `bridge: BridgeHandle`).
8. `agents-plugin-pi/src/index.ts`: delete the `modelCatalogPath` constant
   (L182) and its five references (L289 `startBridge` opts, L300
   `registerAgentTools` sessionCtx, L304 `registerExecuteGateway` sessionCtx,
   L330 `registerFork` sessionCtx, L397 `registerThreadCommands` sessionCtx).
   Reword the `ws-model-catalog-list` command description (L221) away from
   "model-catalog.json's tiers" to `config.tune agents.tier harness:pi`.
9. `agents-plugin-pi/src/bridge.ts`:
   - Drop `modelCatalogPath` from `BridgeOpts` (L39) and
     `WorkflowManualMappingDeps` (L204).
   - Replace `MODEL_CATALOG_ADVISORY` text (L102-110) with the "alias table
     has no `pi` entries" wording, still pointing at `config.tune
     agents.tier harness:pi` / `lead-tune` as the fix.
   - Add `computePiAliasTableUnset(callTool: WorkflowManualMappingDeps
     ["callTool"]): Promise<boolean>` (new, near `firstText`): loops the four
     fixed tiers, applies the same `resolved_from === "pi" && model.includes(
     "/")` guard as step 1, returns `true` only if none hit.
   - Change `maybeAppendModelCatalogAdvisory`'s third parameter (L121-130)
     from `config: ModelCatalogConfig | undefined` to `piAliasTableUnset:
     boolean`; gate body unchanged (`rawName !== "workflow_manual" ||
     !piAliasTableUnset`).
   - `dispatchMappedWorkflowManual` (L228-255): replace L239's
     `readModelCatalog(deps.modelCatalogPath)` with `await
     computePiAliasTableUnset(deps.callTool)`, passed to both
     `maybeAppendModelCatalogAdvisory` call sites (L243, L254).
   - Raw dispatch path (L481-490): make the arrow function's tool-call
     branch compute `const piAliasTableUnset = rawName === "workflow_manual"
     ? await computePiAliasTableUnset((name, a) => client.callTool(name, a))
     : false;` before L489's `maybeAppendModelCatalogAdvisory` call, passing
     that boolean instead of `readModelCatalog(opts.modelCatalogPath)`.
   - Remove the now-unused `readModelCatalog`/`ModelCatalogConfig` import.
10. Delete `agents-plugin-pi/src/model-catalog.ts`,
    `agents-plugin-pi/model-catalog.json`,
    `agents-plugin-pi/test/model-catalog.test.ts`. Remove
    `"model-catalog.json"` from `agents-plugin-pi/package.json`'s `"files"`
    array.
11. Update tests: `test/spawner.test.ts` (replace the
    `describe("resolveModelForAlias", ...)` block with async tests for
    `resolveModelForAliasViaWsMcp` against a stub `client.callTool`, per the
    coverage list in Codebase Findings; fix any other `RpcSpawnCtx`
    literal that still sets `modelCatalogPath` to set `client` instead),
    `test/fork.test.ts` (L648-673: swap `modelCatalogPath` for a stub
    `client`), `test/ask.test.ts` (L1154: drop `modelCatalogPath`),
    `test/bridge.test.ts` (L337/360/382/398: drop `modelCatalogPath` from
    `WorkflowManualMappingDeps` literals; extend the `callTool` stubs used
    there to answer `config.resolve_agent` calls so
    `computePiAliasTableUnset`'s new round-trips resolve deterministically
    in both the advisory-fires and advisory-suppressed cases).
12. `ai-docs/spec/pi-adapter-runtime.md`: rewrite the three anchors
    (`{#260903-pi-spawner-model-tier-inherit}` L420-435,
    `{#260903-pi-model-catalog-config-file}` L437-454,
    `{#260903-pi-model-catalog-unset-advisory}` L456-465) per the Codebase
    Findings description above; keep anchor ids unchanged.
13. `agents-plugin-pi/pi-lead-guide.md`: update the `ws-fork` row (L47) and
    add a `model_name` mention to the `ws-agent-spawn` row (L33) naming the
    four fixed tiers as the legal `model_name` values, sourced from
    `lead-tune`/`config.list`'s harness-`pi` `agents.tier` bucket.
14. Update this ticket's Result section is NOT part of this plan (executor's
    commit/ticket-update step, out of scope for the plan file itself).

## Verification Plan

- `cd agents-plugin-pi && npm test` (runs `node --test`; covers the new
  `resolveModelForAliasViaWsMcp`/`computePiAliasTableUnset` unit tests and
  every touched existing test file). No separate typecheck/build script
  exists in `agents-plugin-pi/package.json` — `npm test` is the only
  automated gate.
- Manual grep sweep after edits: `grep -rn "modelCatalogPath\|model-catalog"
  agents-plugin-pi/src agents-plugin-pi/test agents-plugin-pi/package.json`
  should return nothing (confirms no stale reference survives).
- Out-of-band (owner-run, not part of this plan's automated verification):
  one live spawn per kind against a source-built ws-mcp from this branch —
  (a) a `pi` tier explicitly set via `config.tune agents.tier harness:pi`
  and spawned with a matching `model_name`, (b) an unset tier ->
  confirms inherit, (c) `ws-execute complex:true` -> confirms inherit
  (already covered by Phase 1, re-verify unaffected).

## Escalations

- None.
