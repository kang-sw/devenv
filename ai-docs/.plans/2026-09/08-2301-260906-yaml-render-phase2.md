# Plan: 260906-feat-ws-pi-tool-result-yaml-tui-rendering — Phase 2: Dispatch-tool input summaries and resolved model line

## Relevant Ticket Contract

- Add `src/tool-row-render.ts` (pure functions) holding per-tool summary
  builders and the resolved-model-line logic; registration sites only wire
  the hooks. `content` (model-visible) is untouched; `details` is UI-only.
- `renderCall` on five tools, one fixed per-tool shape, head-truncated with
  `…`, never throwing on partial (`context.argsComplete === false`) or
  malformed/`undefined` args:
  - `explore`: `query`.
  - `ws-execute`: `command` when given, then `prompt` head, `complex` tag.
  - `ws-agent-spawn`: `alias`/`title` when given, `system_prompt_path`
    basename, `prompt` head, requested `model_name`/`model_effort`.
  - `ws-agent-send`: target alias/id, `message` head, `interrupt` tag.
  - `ws-fork`: `prompt` head, `model_name`, `expects_commit` tag.
- Resolved-model line, mandatory on all five: `spawnAgent` gains
  `onModelResolved(resolved)` (`resolved: {tier, model, effort, inherited}`),
  called right after resolution; `ws-agent-spawn`/`ws-fork`/`ws-execute`
  forward it via `onUpdate` and repeat it in the final `details`; `explore`'s
  lead-role path goes through `spawnAgent` the same way, its worker-leaf path
  resolves directly (no `spawnAgent`) and must publish the same shape itself;
  `ws-agent-send` has no live resolution — its line reads the **target
  agent's recorded** model/effort. `inherited` is `source === "inherit"` on
  `resolveModelForAliasViaWsMcp`'s result (landed, see Codebase Findings); a
  `rejected` detail is never an inherit signal and, after the prerequisite
  ticket's Phase 1, never accompanies a launched child, so the line is never
  built from `rejected`. `effort` is the *effective* value (explicit caller
  override wins) and renders `pi-default` when empty. The line renders on
  partial, success, and error results alike.
- Apply Phase 1's shared body renderer (YAML-for-JSON-container, raw
  otherwise, row-budget trim) to these five tools too, with the resolved
  line prepended above it.
- Amend three spec passages under Spec Impact (see below).
- `ask.ts`'s non-tool `spawnAgent` caller passes no `onModelResolved`.
- Constraints: adapter-only (`agents-plugin-pi/`); rendering must never
  throw (parse/serialize failure falls back to raw text); no static
  `@earendil-works/pi-tui` import; headless (`--mode rpc`, no TUI) leads
  never invoke these hooks; keep `details` additive.

## Out of Scope

- Phase 1's own scope (shared YAML `renderResult`, the `yaml` dependency) —
  already implemented and stable at HEAD (`src/tool-result-render.ts`); do
  not re-touch its existing generic (non-override) behavior/tests.
- The prerequisite ticket's tier-refusal/`source` work — already landed at
  `ba2d9d3f` (Phase 1) and `d249fb88` (Phase 2 closeout); consumed here
  read-only via `TierResolution.source`.
- `ws-worker-exec` (gated exec tool) and any headless `--mode rpc` lead —
  ticket's explicit out-of-scope; no row exists to render there.
- `RpcAgentRecord`'s sidecar persistence (`agent-sidecar.ts`) — the two new
  record fields this phase adds (`modelTier`, `modelSource`) are **not**
  added to the sidecar schema; a `ws-agent-send` to a record revived after a
  restart degrades gracefully (see Codebase Findings), not a Phase 2
  requirement.
- Any ws-mcp change, any spec anchor other than the three named below.

## Codebase Findings

- `agents-plugin-pi/src/tool-row-render.ts` — **does not exist**; confirmed
  via `find`. Ticket's own contingency text (`If
  260906-feat-ws-pi-lead-explore-as-async-rpc-child lands first...`) has
  already resolved: that ticket landed (`a62bf770`), so `explore`'s
  lead-role branch already calls `spawnAgent` directly
  (`agents-plugin-pi/src/spawner.ts#L3244-3260`) — wire its
  `onModelResolved` the same way as the other three spawnAgent callers, not
  as a separate resolution path.
- `agents-plugin-pi/src/spawner.ts#L2455-2478` — `spawnAgent`'s two refusal
  guards (post-`ba2d9d3f`) run first; `onModelResolved` must fire
  immediately **after** both guards pass (i.e. `resolution.rejected` is
  guaranteed absent at that point) and **before** `runSpawnGuards`/
  `mkdtempSync`. `resolution.source` (`"tier" | "inherit"`, `TierResolution`
  at `#L400-406`) already exists — the prerequisite ticket's `source` field
  landed at `ba2d9d3f`; the resolved-line's `inherited` reads this field
  directly, never `resolution.rejected` (which is always absent by this
  point).
- `agents-plugin-pi/src/spawner.ts#L2496-2523` — record construction already
  computes `resolvedEffort = resolution.effort` and
  `effectiveModelEffort(params.modelEffort, resolvedEffort)` for
  `record.modelEffort`. Reuse that exact merge for the resolved line's
  effective `effort` (compute it once, feed both the callback and
  `record.modelEffort`) rather than recomputing separately.
- `agents-plugin-pi/src/spawner.ts#L786-847` (`RpcAgentRecord`) has no field
  recording the requested tier alias or resolution source — only
  `modelBase`/`modelEffort` (the resolved values). `ws-agent-send`'s "target
  agent's recorded model/effort" line needs the tier name and
  inherited/tier distinction too, so this phase must add two optional
  fields, e.g. `modelTier?: string` (the raw `params.modelName`, or
  `undefined`) and `modelSource?: "tier" | "inherit"` (mirrors
  `resolution.source`), set once at record creation next to `modelBase`/
  `modelEffort` (`#L2500-2523`). `agents-plugin-pi/src/agent-sidecar.ts#L63-64,130-131,205-218,264-265`
  persists `modelBase`/`modelEffort` only — the two new fields are **not**
  captured there (deliberately out of scope, see Out of Scope); a revived
  record's `ws-agent-send` line should treat missing `modelSource` as
  `"inherit"` (safe default) rather than throwing or guessing.
- `agents-plugin-pi/src/spawner.ts#L2785-2792` (`listAgents`) shows the
  existing "model + `/`-joined effort" convention (`ws-agent-list`'s
  `model` column) — reuse the same `modelBase`/`modelEffort` read pattern
  for consistency, but `ws-agent-send`'s new line is a different format
  (arrow-and-dot, see Ticket Contract), not this one.
- `agents-plugin-pi/src/spawner.ts#L3063-3095` (`ws-agent-spawn`),
  `#L3097-3127` (`ws-agent-send`), `#L3186-3272` (`explore`, both branches)
  — none pass `onApprovalPending`-style callbacks for model resolution yet;
  `_onUpdate`/`_signal` are already present as the 4th execute() parameter
  on `ws-agent-spawn` and `explore`'s signature but unused (underscore
  prefix) — un-prefix and use it. `ws-agent-send`'s execute only takes
  `(_toolCallId, params)` (`#L3114`); no onUpdate wiring needed there since
  it resolves synchronously from the registry (single final `details`
  suffices).
- `agents-plugin-pi/src/spawner.ts#L3263-3269` — the worker-leaf `explore`
  branch calls `resolveRequiredExploreModel` (`#L3001-3010`) directly, not
  `spawnAgent`; it always succeeds with a genuine tier hit (throws
  otherwise, `requireTier` semantics), so its resolved line is always
  `{tier: "small", model, effort, inherited: false}` — publish it via the
  tool's own `_onUpdate`/final `details`, independent of the
  `spawnAgent`-based callback plumbing.
- `agents-plugin-pi/src/execute-gateway.ts#L640-686` (`ws-execute`) and
  `#L258-260` (`resolveExecuteModelAlias`) — `complex` maps to
  `modelName: undefined` (deliberate inherit) vs `"small"` (tier hit); both
  are legitimate non-rejected resolutions the ticket's test list names
  ("a `complex:true` inherit").
- `agents-plugin-pi/src/fork.ts#L576-599` (`buildForkSpawnCtx`) and
  `#L717-733` (`ws-fork` execute) — `buildForkSpawnCtx`'s `opts` parameter
  and returned `RpcSpawnCtx` literal both need a new `onModelResolved` pass-
  through field; the execute body already has `_onUpdate` as its 4th
  parameter, unused.
- `agents-plugin-pi/src/tool-result-render.ts` (full file read) — **already
  applied generically to all five dispatch tools** via `registerWsTool`
  (`#L495-522`): every `registerWsTool` call whose definition has no
  `renderCall`/`renderResult` of its own gets the shared YAML-args-dump
  `renderCall` + shared YAML/raw `renderResult`. Confirmed by
  `git show --stat 3201c513` ("unify ws tool presentation" — "Existing
  custom renderer ownership wins over the common seam; the focused survey
  found none among current ws-owned native tools"): **as of HEAD, all five
  dispatch tools currently show a generic YAML dump of every raw argument**
  under the title — the exact "debug dump" shape the ticket rejects
  ("Rejected: dumping the JSON arguments under the title"). Phase 2 must
  replace this for these five tools specifically, by having each tool's own
  registration supply its own `renderCall`/`renderResult`: `registerWsTool`'s
  gate (`#L500-504`, `if (existing.renderCall || existing.renderResult) {
  pi.registerTool(definition); return; }`) already exists precisely to let a
  tool opt out of the generic wrapping — no change to `registerWsTool` or to
  the generic path is needed; supplying both hooks on the five tool
  definitions is sufficient and leaves every other bridged `ws__*` tool's
  generic rendering untouched.
- `agents-plugin-pi/src/tool-result-render.ts#L423-486`
  (`createToolPreviewRenderers`) is exported and reusable as-is for the
  YAML/raw body: **recommend a small, backward-compatible extension**
  (new optional 4th parameter, e.g. `overrides?: { buildCallPreview?:
  (args: unknown, context: PreviewRenderContext) => string; resolvedLine?:
  (result: {details?: unknown}, context: PreviewRenderContext) => string |
  undefined }`) rather than duplicating its caching/marker/theme logic in
  the new file:
  - `renderCall` (`#L437-459`): when `overrides.buildCallPreview` is given,
    use it in place of `yamlInputPreview(args, serialize)` for the preview
    text; every other line (title styling, `argsComplete` caching,
    `physicalPreviewLayout`'s 10-row cap and `…` marker via
    `INPUT_START_INDENT`/`CONTINUATION_INDENT`) stays unchanged and is
    reused for free — the five tools' summaries are short (well under the
    10-row cap) so no new row-budget parameter is needed; head-truncate the
    *free text* itself (character-count cap with `…`, mirroring the
    existing `EXPLORE_TITLE_CAP`/`deriveExploreTitle` pattern at
    `spawner.ts#L3012-3019`) before handing it to this path.
  - `renderResult` (`#L461-484`): when `overrides.resolvedLine` is given,
    compute/cache it via `stateFor(context)` (add one field, e.g.
    `state.resolvedLine`) on every call (partial, success, error alike —
    the object identity check already used for `state.result` shows the
    pattern) and stop throwing `UseNativeResultFallback` purely because of
    `options.isPartial`/`context.isPartial`/`context.isError` when a
    resolved line is available; instead render a component containing the
    resolved line and, when the body is a normal completed single-text
    result, the existing YAML/raw body beneath it — when the body path
    itself would throw (error/partial/non-text), render **just** the
    resolved line. When `overrides.resolvedLine` is absent, behavior is
    byte-identical to today (guarantees zero regression on every other
    `registerWsTool` caller, including `ws-approve`/read/exec, which stay on
    the generic path).
  - Cold-`tuiRef` behavior must stay unchanged (still throws
    `UseNativeResultFallback`, deferring to Pi's own fallback) — this keeps
    `agents-plugin-pi/test/native-tool-registration.test.ts#L44-49`'s
    existing `assert.throws(() => tool!.renderCall!(...))` cold-ref
    assertion valid for all five tools without modification; only that
    test's tool-name loop needs `ws-agent-spawn`/`ws-execute`/`ws-fork`
    added (it currently only covers `ws-agent-send`/`explore`/`ws-approve`/
    the two read/exec tools).
- `agents-plugin-pi/test/tool-result-render.test.ts` already has a reusable
  `fakeTui()`/`FakeText`/`FakeBox` harness (`#L1-70`) — reuse the same
  pattern for the new `tool-row-render.test.ts` rather than re-deriving one.
- `agents-plugin-pi/test/spawner.test.ts#L748-778` (`installRpcHarness`,
  `registerAgentTools` driven end-to-end) is the established pattern for
  tool-level tests that need real guard ordering; use it for the new
  `onModelResolved`/`details.resolved` forwarding tests on `ws-agent-spawn`
  and `explore`.
- `ai-docs/spec/pi-adapter-runtime.md`:
  - `{#260903-pi-delegation-spawner-tools}` (`#L401-480`): the
    `ws-agent-spawn` bullet is at `#L416-437`, `ws-agent-send` at
    `#L438-451`. **No existing bullet mentions `explore`** in this section
    (`grep` over the anchor's range returned nothing) — the ticket's Spec
    Impact explicitly assigns `explore`'s Phase 2 sentence here anyway (not
    to `{#260903-pi-explore-recon-leaf}`), so this is a **new** short
    addition for `explore`, not an amendment to an existing `explore`
    bullet.
  - `{#260905-pi-execute-approval-gateway}` (`#L890-934`): the `ws-execute`
    bullet is at `#L902-914`.
  - `{#260905-pi-side-thread-fork-task-thread}` (`#L1030-1040`): the
    `ws-fork` tool is introduced in the anchor's opening paragraph
    (`#L1032-1040`), not a bulleted list.

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts` — add `ResolvedModelInfo` next to
   `TierResolution` (`#L400-406`): `{ tier: string; model?: string; effort?:
   string; inherited: boolean }`. Add `onModelResolved?: (resolved:
   ResolvedModelInfo) => void` to `RpcSpawnCtx` (`#L1722-1805`). In
   `spawnAgent` (`#L2455-2478`), right after both refusal guards pass,
   compute `resolvedEffort = effectiveModelEffort(params.modelEffort,
   resolution.effort)` once and call
   `ctx.onModelResolved?.({ tier: resolution.source === "tier" ?
   params.modelName! : "inherit", model: resolution.model, effort:
   resolvedEffort, inherited: resolution.source === "inherit" })`; reuse
   `resolvedEffort` at `#L2512` instead of recomputing. Add `modelTier?:
   string` and `modelSource?: "tier" | "inherit"` to `RpcAgentRecord`
   (`#L786-847`), set from `params.modelName` and `resolution.source` in
   the record literal (`#L2500-2523`).
2. Create `agents-plugin-pi/src/tool-row-render.ts` (pure functions, no
   pi-tui import): re-export/import `ResolvedModelInfo` from `spawner.ts`;
   `formatResolvedLine(resolved: ResolvedModelInfo | undefined): string |
   undefined` (`→ ${tier} · ${model ?? "?"} · effort ${effort ||
   "pi-default"}`, `undefined` when `resolved` is `undefined`); a shared
   `truncateHead(text: string | undefined, maxChars: number): string`
   (mirrors `deriveExploreTitle`); five summary builders taking `unknown`
   (defensive against partial/malformed args) and returning a short
   multi-line string per the fixed per-tool shape in Ticket Contract —
   each must not throw on `undefined`/partial input (use optional
   chaining/defaults throughout, matching the ticket's explicit test:
   "a builder never throws on `undefined` arguments"). Also export a small
   factory, e.g. `createDispatchToolPreview(tuiRef: ToolPreviewTuiRef,
   toolName: string, buildCallSummary: (args: unknown) => string):
   { renderCall; renderResult }`, that lazily resolves `tuiRef.current`
   (throwing `UseNativeResultFallback` when absent, mirroring
   `registerWsTool`'s existing `renderers()` closure) and calls
   `createToolPreviewRenderers(tui, toolName, undefined, { buildCallPreview:
   (args) => buildCallSummary(args), resolvedLine: (result) =>
   formatResolvedLine((result.details as { resolved?: ResolvedModelInfo }
   | undefined)?.resolved) })`.
3. `agents-plugin-pi/src/tool-result-render.ts` — extend
   `createToolPreviewRenderers` with the optional `overrides` parameter and
   behavior described in Codebase Findings (`buildCallPreview`,
   `resolvedLine`); default (`undefined`) path must be byte-identical to
   today so every other `registerWsTool` caller is unaffected.
4. `agents-plugin-pi/src/spawner.ts` registrations:
   - `ws-agent-spawn` (`#L3028-3095`): rename `_onUpdate` to `onUpdate`;
     pass `onModelResolved: (resolved) => { resolvedInfo = resolved;
     onUpdate?.({ content: [], details: { resolved } }); }` into the
     `spawnAgent` ctx literal; include `details: { resolved: resolvedInfo }`
     in the final return; add `renderCall`/`renderResult` from
     `createDispatchToolPreview(toolPreviewTuiRef, "ws-agent-spawn",
     buildAgentSpawnSummary)` to the `registerWsTool` definition.
   - `ws-agent-send` (`#L3097-3127`): after `sendToAgent` resolves, look up
     the target record the same way `ws-approve` does
     (`resolveAgentId`/`rpcRegistry.get`, `execute-gateway.ts#L709-710`
     pattern) and build `resolved` from `record.modelTier`/`modelSource`/
     `modelBase`/`modelEffort` (fallback `inherited: true`/`tier: "inherit"`
     when `modelSource` is absent — revived record); include `details:
     { resolved }` in the return; add `renderCall`/`renderResult` via
     `createDispatchToolPreview(..., "ws-agent-send", buildAgentSendSummary)`.
   - `explore` (`#L3219-3271`): rename `_onUpdate` to `onUpdate`. In the
     `isLeadRole` branch, add the same `onModelResolved` capture-and-forward
     as `ws-agent-spawn` to the `spawnAgent` ctx literal and final return.
     In the worker-leaf branch (`#L3263-3269`), after
     `resolveRequiredExploreModel` resolves, call `onUpdate?.({content: [],
     details: {resolved: {tier: "small", model: resolved.model, effort:
     resolved.effort, inherited: false}}})` and include the same in the
     final `details`. Add `renderCall`/`renderResult` via
     `createDispatchToolPreview(..., "explore", buildExploreSummary)`.
5. `agents-plugin-pi/src/execute-gateway.ts` — `ws-execute`
   (`#L640-686`): rename `_onUpdate`, forward `onModelResolved` into the
   `spawnAgent` ctx literal and final `details` (same capture pattern as
   step 4); add `renderCall`/`renderResult` via
   `createDispatchToolPreview(toolPreviewTuiRef, "ws-execute",
   buildExecuteSummary)`.
6. `agents-plugin-pi/src/fork.ts` — add `onModelResolved?:
   RpcSpawnCtx["onModelResolved"]` to `buildForkSpawnCtx`'s `opts` param
   and forward it in the returned ctx (`#L576-599`); in `registerFork`'s
   `ws-fork` execute (`#L666-739`), rename `_onUpdate`, pass
   `onModelResolved` through `buildForkSpawnCtx`'s opts, capture the
   resolved value, include it in the final `details`; add
   `renderCall`/`renderResult` via `createDispatchToolPreview(...,
   "ws-fork", buildForkSummary)`.
7. `ask.ts`'s existing `spawnAgent` call (`#L1186`) — leave untouched (no
   `onModelResolved` passed), confirming the "non-tool caller passes none"
   contract by omission.
8. Spec: amend `ai-docs/spec/pi-adapter-runtime.md` at the three anchors
   per Codebase Findings — one sentence each for `ws-agent-spawn`/
   `ws-agent-send`/`explore` under `{#260903-pi-delegation-spawner-tools}`
   (new short addition for `explore`, since none exists there), one for
   `ws-execute` under `{#260905-pi-execute-approval-gateway}`, one for
   `ws-fork` under `{#260905-pi-side-thread-fork-task-thread}`. Each
   sentence: the row carries an argument summary and a mandatory resolved
   model/effort line, both display-only, with the resolution outcome
   published through `details`.
9. Update `agents-plugin-pi/test/native-tool-registration.test.ts`
   (`#L44-56`): add `"ws-agent-spawn"`, `"ws-execute"`, `"ws-fork"` to the
   tool-name loop (requires also invoking `registerFork` in the harness);
   the existing cold-ref-throws assertion must keep passing unmodified for
   all eight names now covered.

## Verification Plan

- `npm test` (== `node --test`, env-scrubbed per `package.json`) run at
  least twice: full suite for a global regression check, and targeted runs
  on the touched files: `node --test test/tool-row-render.test.ts
  test/tool-result-render.test.ts test/spawner.test.ts
  test/execute-gateway.test.ts test/fork.test.ts
  test/native-tool-registration.test.ts` with `WS_PI_SPAWN_ROLE` unset.
- New `tool-row-render.test.ts`: each builder's representative and
  empty/partial-argument output; no-throw on `undefined` args;
  `formatResolvedLine` for a tier hit, a `complex:true` inherit, an
  omitted-tier "miss", and a no-thinking-level dispatch (`effort:
  pi-default`); resolved line present across partial/success/error via
  `createDispatchToolPreview`'s composed `renderResult` (including the
  cross-call cache path where a later error/partial call has no
  `result.details` but must still show the last-seen resolved line);
  cold-`tuiRef` still throws `UseNativeResultFallback`.
- `tool-result-render.test.ts` additions: `overrides.buildCallPreview`
  changes the rendered input text; `overrides.resolvedLine` present bypasses
  the isPartial/isError throw; `overrides` absent is byte-identical to the
  pre-Phase-2 behavior (regression guard for every other `registerWsTool`
  caller).
- `spawner.test.ts` additions: `spawnAgent` invokes `onModelResolved` with
  the correct shape immediately after resolution for a tier hit, an
  omitted-`model_name` inherit, and a transport-failure-forced inherit
  (named tier, `source: "inherit"`); never invoked when the refusal guard
  throws first; `RpcAgentRecord.modelTier`/`modelSource` populated
  correctly; tool-level (via `installRpcHarness`/`registerAgentTools`)
  coverage for `ws-agent-spawn`'s partial+final `details.resolved`,
  `ws-agent-send`'s reconstructed line (including the graceful
  no-`modelSource` fallback), and both `explore` branches (lead-role via
  `spawnAgent`, worker-leaf via direct resolution) publishing the line; a
  long synchronous `explore` answer's row summary is head-truncated.
- `execute-gateway.test.ts`/`fork.test.ts` additions: `ws-execute`'s
  `complex:true`/`false` cases and `ws-fork`'s named/omitted `model_name`
  cases forward `onModelResolved` correctly end to end.
- Manual/live check (owner-run, per ticket): in a Pi session, `explore`,
  `ws-execute`, and `ws-agent-spawn` rows show the input summary while
  running and the resolved model line before the child finishes — out of
  scope for automated verification here.

## Escalations

- None.
