# Plan: 260902-feat-ws-pi-native-mvp — Phase 3: Model catalog curation + tier map + bootstrap warning

## Relevant Ticket Contract

- Read Pi's enabled/configured model pool at runtime as **curation input only**
  (raw pool can be thousands of entries via aggregators) — narrow to a small
  curated catalog; never a live per-spawn lookup.
- Store the curated catalog + tier map in an **adapter-owned data file** (no Pi
  model strings in ws-mcp core), keyed on ws's canonical tiers
  `small`/`medium`/`large`/`xlarge`; `explore` is a **role**, not a tier,
  defaulting to `small`.
- When the tier map (or at least the `small`/explore tier) is unset, **append**
  a strong advisory to every `ws__workflow_manual` bridge response
  (adapter-side post-processing, keyed on unset state), mirroring the existing
  ws-mcp core bootstrap-version-behind advisory cadence (re-warn on every read
  while the condition holds, not once per session).
- Unset degrades silently to inherit for spawn/explore — never hard-fail; the
  advisory is the only pressure.
- Gate: with an unset map, `workflow_manual` shows the advisory and a spawn
  inherits; after configuring a tier, a spawn uses the mapped model.
- Golden rule: ws-mcp Go source (`agents-plugin-tool/`) is never modified for
  Pi; all Pi-specific policy lives in `agents-plugin-pi/`.
- Package/data-file precedent: curation lives in a data file, not hardcoded
  (anchor `260802-research-ws-pi-native-framework.md#L617-620`), same
  data/code-separation spirit as the Phase 2 `TOOL_GROUPS` table but as an
  actual file this time, per the ticket's repeated "adapter's curation data
  file" / "adapter's data file" phrasing.

## Out of Scope

- Phase 4's `/ws-discuss` PoC command (`pi.registerCommand`, skills+bridge+spawn
  round-trip demo).
- Durable depth-2 recursion, always-visible TODO, goal-loop, compaction hooks —
  deferred to follow-up tickets under the epic.
- An interactive multi-step tier-assignment wizard (`ctx.ui.select` picker
  flow). Nothing in the ticket or anchor specifies this UX; the ticket's own
  "zero ws agent-profile files" / data-file-not-hardcoded ethos and Phase 4's
  exclusive ownership of PoC-command-shaped work both point at a hand-edited
  JSON data file plus a minimal read-only listing helper, not a wizard. See
  Codebase Findings risk-signal note below.
- Any change to `agents-plugin/`, `agents-plugin-tool/`, `agents-plugin-wsflow/`
  (golden rule).

## Codebase Findings

**Load-bearing unknown 1 — extension-facing model-pool read API (CONFIRMED, source+docs, Pi 0.84.4 installed build):**

- No file literally named `enabled-models.ts` exists anywhere in the installed
  `@earendil-works/pi-coding-agent@0.84.4` tree (`find -iname` across the whole
  package, incl. `node_modules`, returned nothing) — the ticket's parenthetical
  is stale/approximate, not a real file to reference.
- The real, **documented and stable** extension-facing API:
  `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md#L1013-1017`
  — `ctx.scopedModels: readonly ScopedModel[]` (also `ctx.getScopedModels()`),
  "the read-only list of models scoped to the current session — the same set
  the `/scoped-models` command shows... resolved from `--models` CLI flag and
  the `enabledModels` setting... **empty when no scoping is configured, meaning
  every available model is usable**... Use it to populate a model picker that
  mirrors the built-in one instead of enumerating the whole catalogue via
  `ctx.modelRegistry.getAvailable()`."
- Type source confirming shape: installed
  `dist/core/extensions/types.d.ts#L219-232` (`ExtensionContext.scopedModels`,
  `ExtensionContext.modelRegistry: ModelRegistry`) and `#L1266`
  (`ExtensionContextActions.getScopedModels`). `dist/core/model-resolver.d.ts#L9-13`:
  `ScopedModel { model: Model<Api>; thinkingLevel?: ThinkingLevel }`.
  `dist/core/model-registry.d.ts#L26-27`: `getAll()` / `getAvailable()` are the
  raw-pool fallback for when `scopedModels` is empty (unconfigured = "all
  usable," not "none"). `pi-ai/dist/types.d.ts#L695-712`: `Model<Api>` has
  `id`/`name`/`provider`, giving the `${provider}/${id}` string format already
  used at `spawner.ts#L679`.
- **Available inside a registered tool's execute**: `ToolDefinition.execute(...,
  ctx: ExtensionContext)` (`dist/core/extensions/types.d.ts#L373`) and inside a
  `pi.registerCommand` handler (`(args, ctx: ExtensionCommandContext) =>
  ...`, `#L896`, which extends `ExtensionContext`) both receive `ctx.scopedModels`
  directly — no extra wiring needed to reach it.

**Load-bearing unknown 2 — `workflow_manual` response post-processing point (CONFIRMED, this repo):**

- `agents-plugin-pi/src/bridge.ts#L169-198` — `startBridge`'s per-tool
  registration loop is the single place every ws-mcp tool (including
  `workflow_manual`) gets its Pi `execute()` wired. The insertion point is
  inside that loop's `execute()` closure, right before the final `return {
  content: result.content, details: result };` at `bridge.ts#L195`, guarded on
  `rawName === "workflow_manual"` — isolates the change to that one tool
  without touching the other 59.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L34-43` — `McpContentItem { type:
  string; text?: string }` / `McpToolCallResult { content: McpContentItem[] }`
  — appending an advisory means pushing a new `{ type: "text", text: advisory
  }` item onto a **copied** `result.content` array, not mutating in place.
- `test/bridge.test.ts#L26-38` already fixtures
  `sanitizeToolName("workflow_manual") === "ws__workflow_manual"` and the raw
  tool name `"workflow_manual"` in its 60-tool live snapshot — confirms the
  exact string to match on.
- Raw tool-name confirmation from ws-mcp core itself:
  `agents-plugin-tool/internal/mcp/server.go#L75,573,4205` — the literal name
  is `"workflow_manual"` (no dot).
- **"Existing bootstrap-version-behind advisory" being mirrored** is a ws-mcp
  Go **core** mechanism, not something already in the Pi adapter:
  `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L79-147`
  (`bootstrapStalenessWarning` + `injectBootstrapStalenessWarning`) computed
  **fresh on every call** (not cached) and injected into
  `workflow_manual.go#L274-321`'s response body on every `workflow_manual`
  invocation while the staleness condition holds — this is the cadence pattern
  (recompute + inject per-call) to mirror in TS, not a mechanism to reuse
  cross-language (golden rule forbids touching this Go file for Pi policy).

**Phase 2 model-resolution seam to extend (this repo):**

- `agents-plugin-pi/src/spawner.ts#L677-680` — `resolveModel(toolCtx)` is
  currently inherit-only: reads the calling tool-execute `ctx.model` and
  formats `${provider}/${id}`, or `undefined`. This is the function Phase 3
  extends with a tier-first lookup, inherit-fallback.
- `agents-plugin-pi/src/spawner.ts#L362-366` (`SpawnAgentParams.tier?:
  string`), `#L410-423` (`AgentRecord.tier` stored, unused for `--model`) — the
  `tier` plumbing already exists as inert metadata per the Phase 2 Result note
  ("`tier` resolves as inherit until Phase 3 lands the model catalog").
- `agents-plugin-pi/src/spawner.ts#L96-151` (`buildSpawnArgs` /
  `BuildSpawnArgsOptions.model?: string`) already accepts an arbitrary
  `provider/id` model string and emits `--model <pattern>` — no change needed
  here, only the value fed into it.
- `agents-plugin-pi/src/spawner.ts#L600-651` (`exploreLeaf`) has no `tier`
  field on `ExploreParams` and currently passes `model: ctx.model` (inherit)
  unconditionally — per ticket, `explore` implicitly resolves through the
  `small` tier without exposing a caller-facing `tier` parameter.
- `agents-plugin-pi/src/version-check.ts#L21-24` (`readRuntimeContract`) is the
  established `readFileSync` + `JSON.parse` pattern for the adapter's own data
  files (`runtime.json` today) — the model-catalog file read should mirror
  this, tolerant of a missing/empty file (return "unset" rather than throw,
  since unset is the ticket's expected default state).

**Risk signal — curation UX is unspecified.** Neither the ticket
(`ai-docs/tickets/ready/260902-feat-ws-pi-native-mvp.md#L296-318`) nor the
anchor (`ai-docs/tickets/idea/260802-research-ws-pi-native-framework.md#L227-268`)
says whether narrowing the raw pool to a curated catalog happens through an
interactive Pi command/picker or by hand-editing the data file directly. There
is no existing precedent in `agents-plugin-pi/` for a multi-step interactive
`ctx.ui.select` flow, and `ui.select` (`dist/core/extensions/types.d.ts#L70`)
takes only a flat `string[]` of options — a 4-tier assignment wizard over
possibly-hundreds of scoped models is real design work, not a "search and
reuse" step. Resolved in this plan by choosing the minimal, scope-consistent
option: a read-only listing command exercises the runtime read API (fulfilling
"read Pi's enabled/configured model pool at runtime" as working code), and the
curated catalog + tier map is a plain JSON file the user hand-edits — same
precedent `runtime.json` already sets in this package, and consistent with
"zero ws agent-profile files" / "curation is a data file, not hardcoded."

## Implementation Plan

1. **`agents-plugin-pi/src/model-catalog.ts` (new).** Define `ModelTier =
   "small" | "medium" | "large" | "xlarge"`; `ModelCatalogConfig { tiers?:
   Partial<Record<ModelTier, string>>; catalog?: Array<{ provider: string; id:
   string; label?: string }> }`. `readModelCatalog(path: string):
   ModelCatalogConfig | undefined` — mirrors `version-check.ts#L21-24`'s
   `readFileSync` + `JSON.parse`, but returns `undefined` on a missing file
   (never throws) since unset is the expected default, matching "never
   hard-fail." `resolveTierModel(config: ModelCatalogConfig | undefined, tier:
   ModelTier): string | undefined` — `config?.tiers?.[tier]`.
   `isModelCatalogUnset(config: ModelCatalogConfig | undefined): boolean` —
   `true` when `!config?.tiers?.small` (explore/recon always resolves through
   `small`, so an unset `small` tier is the trigger condition per the ticket's
   "or at least the explore tier").
2. **`agents-plugin-pi/model-catalog.json` (new, package root, sibling to
   `runtime.json`).** Ship as `{}` (unset) so a fresh checkout starts unset and
   the advisory fires by default until the user curates it.
3. **`agents-plugin-pi/src/index.ts`.** Add `modelCatalogPath =
   join(pluginDir, "model-catalog.json")` next to the existing
   `launcherPath`/`runtimeJsonPath` consts (`index.ts#L48-49`); thread it into
   both `startBridge(...)` and `registerAgentTools(...)` calls
   (`index.ts#L60-67`), same shape as the existing options-object wiring.
4. **`agents-plugin-pi/src/bridge.ts`.** Add `modelCatalogPath: string` to
   `BridgeOptions` (`bridge.ts#L32-39`). Inside the per-tool registration loop
   (`bridge.ts#L169-198`), in the `execute()` closure, after `const result =
   await client.callTool(rawName, args)` and the existing `isError` check, add:
   `if (rawName === "workflow_manual" && isModelCatalogUnset(readModelCatalog(opts.modelCatalogPath))) { content = [...result.content, { type: "text", text: MODEL_CATALOG_ADVISORY }]; }` else `content = result.content`; return `{ content, details: result }`. Read the
   catalog file fresh on every call (no caching) — matches the Go core's
   per-call recompute cadence (`bootstrap_alarm.go`). Advisory text: state the
   Pi model tier map is unset, spawns/explores silently inherit the parent
   model (costly for recon), and point at `model-catalog.json` to configure
   `small` (and other tiers) — phrase as a `> [!note]`-style block for
   consistency with the Go core's blockquote convention (`workflow_manual.go`
   callers / `bootstrap_alarm.go#L116-136`), though exact wording is free (no
   ticket-mandated copy).
5. **`agents-plugin-pi/src/spawner.ts`.** Replace the body of
   `resolveModel(toolCtx)` (`spawner.ts#L677-680`) — rename or extend to accept
   a `tier: ModelTier` param — with: look up `resolveTierModel(catalogConfig,
   tier)`; if present, use it; else fall back to the current inherit logic
   unchanged. Thread a pre-loaded `ModelCatalogConfig | undefined` (loaded once
   at `registerAgentTools` call time, or re-read per spawn — re-read per spawn
   is simpler and consistent with "hand-edit the file, changes apply without
   restart," matching the workflow_manual advisory's no-caching choice) into
   `registerAgentTools`'s options alongside `sessionCtx` (`spawner.ts#L674`).
   Wire the `ws-agent-spawn` tool's `execute` (`spawner.ts#L696-710`) to pass
   `params.tier` (cast to `ModelTier` when it matches one of the four values,
   else treat as unset/inherit — no validation error, per "never hard-fail").
   Wire `exploreLeaf`'s call site (`spawner.ts#L774-784`) to resolve with an
   implicit `tier: "small"` — `ExploreParams` (`spawner.ts#L583-592`) gains no
   new caller-facing field; `explore` stays a role, not a caller-supplied tier,
   per ticket.
6. **`agents-plugin-pi/src/index.ts` (command registration).** Register one
   minimal read-only `pi.registerCommand("ws-model-catalog-list", { handler:
   async (_args, ctx) => { const models = ctx.scopedModels.length ?
   ctx.scopedModels.map(m => m.model) : ctx.modelRegistry.getAvailable(); ...
   ctx.ui.notify(...) } })` (or equivalent, e.g. printing to console in
   non-TUI modes) that lists `provider/id` candidates for the user to hand-copy
   into `model-catalog.json`'s `tiers`/`catalog` fields. This is what
   "verify the extension-facing read API" cashes out to as working code —
   confirmed live against a running Pi session as part of Verification, not
   just referenced in a comment.
7. **`agents-plugin-pi/test/model-catalog.test.ts` (new).** Unit-test
   `readModelCatalog` (missing file → `undefined`; empty `{}` → `{}`;
   populated file → parsed), `resolveTierModel`, `isModelCatalogUnset` — pure
   logic, `node --test`, no subprocess, matching existing test conventions.
8. **`agents-plugin-pi/test/bridge.test.ts`.** Add cases for the advisory
   append logic. If step 4 is written as an inline closure it is not
   independently testable — extract it as an exported pure function (e.g.
   `maybeAppendModelCatalogAdvisory(rawName: string, content: McpContentItem[],
   config: ModelCatalogConfig | undefined): McpContentItem[]`) so it can be
   unit-tested the same way `resolveSessionKey`/`withOptionalSessionKey`
   already are (`bridge.ts#L80-138`, tested at `test/bridge.test.ts` — pure
   helpers pulled out of the registration loop for testability).
9. **`agents-plugin-pi/test/spawner.test.ts`.** Add cases for the new
   tier-aware resolution: tier set + mapped in catalog → resolved model; tier
   set + catalog present but that tier unmapped → inherit; no tier (spawn) →
   inherit unchanged (regression); `exploreLeaf`'s implicit `small` tier →
   resolved when catalog has `tiers.small`, inherit otherwise.
10. **`ai-docs/spec/pi-adapter-runtime.md`.** Update
    `{#260903-pi-spawner-model-tier-inherit}` (`spec:169-180`, currently
    documents inherit-only + an `Implementation Gap` note) to describe
    tier-map resolution with inherit fallback; remove or update the now-stale
    `Implementation Gap` callout. Add a new anchor section documenting the
    `workflow_manual` advisory contract (unset condition, append-not-prepend,
    per-call recompute, never-hard-fail) and `model-catalog.json`'s schema and
    location. Update the `## Package topology` `Constraints` note
    (`spec:203-209`) which currently says the model catalog "is a separate,
    not-yet-implemented surface... not part of this contract yet."

## Verification Plan

- `node --test test/` from `agents-plugin-pi/` — new `model-catalog.test.ts`
  plus extended `bridge.test.ts` / `spawner.test.ts` cases, alongside the
  existing 90 tests (31 + 59 from Phases 1-2) passing unchanged.
- Live gate (matches the ticket's Phase 3 Gate text exactly): with
  `model-catalog.json` at `{}` (unset), drive `ws__workflow_manual` through the
  bridge (e.g. via `pi -e agents-plugin-pi/src/index.ts --mode json -p
  "<prompt calling ws/workflow_manual>"`, the same non-interactive driving
  technique the Phase 2 Result used) and confirm the advisory text is present
  in the response; a subsequent `ws-agent-spawn`/`explore` call still inherits
  the parent model (unchanged `--model` omission, or verify the child's
  `session.jsonl` `model:` stamp matches the parent). Then populate
  `tiers.small` (and optionally other tiers) in `model-catalog.json`, re-run,
  and confirm the advisory is absent from `workflow_manual` and a
  tier-tagged spawn's child process launches with `--model <mapped-model>`
  (verify via the child's `session.jsonl` `model:` stamp, same technique the
  Phase 2 Result used to confirm inherit).
- Golden-rule check: `git diff --stat -- agents-plugin/ agents-plugin-tool/
  agents-plugin-wsflow/` empty after implementation.
- `claude plugin validate agents-plugin` unaffected (no change to that
  package) — not part of this phase's verification surface.

## Escalations

- None.
