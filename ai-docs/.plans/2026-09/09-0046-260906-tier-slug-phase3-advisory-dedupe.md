# Plan: 260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model — Phase 3: Dedupe the advisory per session

## Relevant Ticket Contract

- **Once per distinct rejected set per session, re-armed on compaction.** The
  bridge keeps the last emitted advisory key, a stable string built from the
  sorted `<tier>=<checked model or ->:<why>` pairs (an all-unset table
  therefore keys as four `unset` rows). Append when the key differs from the
  last emitted one — covers the first `workflow_manual` call of a session, a
  tier tuned mid-session, and the table becoming clean (an empty key emits
  nothing and resets). The four `config.resolve_agent` round-trips per call
  stay; only emission is deduped.
- The key lives in a holder object owned by the bridge and passed into
  `maybeAppendModelCatalogAdvisory` as a parameter, so that function (and
  `computePiAliasTableReport`) stay IO-free/pure for the existing direct-call
  tests; each new dedup test starts from a fresh holder.
- The gate sits inside `maybeAppendModelCatalogAdvisory` so all three call
  sites are covered: the two in the mapped `dispatchMappedWorkflowManual`
  branches (in fact three current call sites — cut-hit, no-body,
  workflow_state fallback) and the raw-dispatch path in `startBridge` taken
  by unmapped roles.
- The holder resets on the adapter's compaction boundary (the event the goal
  loop observes), so the first `workflow_manual` after a compaction re-emits
  the block.
- Since `unset` is now a rejection, the guidance block
  (`MODEL_CATALOG_ADVISORY`) vs. per-tier-rows branching inside
  `maybeAppendModelCatalogAdvisory` is unchanged by this phase — only
  **emission** gains the per-session gate; the text-selection logic stays as
  landed in Phase 1.
- Constraint: "Phase 3 lands after
  `260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches` so the
  gate is added to the rewritten dispatch, not merged against it" — that
  ticket landed (goal tip `25925c86`); this survey read the current
  post-rewrite `bridge.ts`, not the pre-rewrite shape the ticket text
  describes historically.
- Constraint: `resolveModelForAliasViaWsMcp`, the suggestion heuristic,
  `computePiAliasTableReport`, and `maybeAppendModelCatalogAdvisory` stay
  IO-free; the catalog and the advisory key holder are parameters.
- Constraint: "The advisory key holder is in-process state on the bridge; it
  is not persisted, resets with the session, and resets on compaction."
- Tests (from Phase 3's own Tests bullet): two consecutive `workflow_manual`
  calls with the same rejected set append the block once; a changed set
  appends again; a clean table after a rejected one appends nothing and
  resets the key so a later rejection warns again; a compaction reset makes
  the next call append again with the same set; the unmapped raw-dispatch
  path is gated the same way.
- Spec: `{#260903-pi-model-catalog-unset-advisory}` in
  `ai-docs/spec/pi-adapter-runtime.md` — Phase 3 replaces the "recomputed and
  re-appended on every call while the condition holds" cadence sentence with
  the per-session key rule and the compaction re-arm. **This is the lead's
  doc pre-pass — do not edit the spec file or commit a spec change as part of
  this phase's implementation.**

## Out of Scope

- Any change to `report.unset` semantics, the guidance-block-vs-per-tier-rows
  text selection, or `formatTierWarning`/`suggestModels` — all landed in
  Phase 1 (`ba2d9d3f`) and unaffected by this phase.
- Phase 1/2 behavior (backend expansion, refusal, `--thinking` forwarding) —
  already landed with `### Result` sections; not re-touched.
- Any ws-mcp (`agents-plugin-tool/`) change — none required; `agents.tier`
  storage and `config.resolve_agent` stay as-is.
- Editing `ai-docs/spec/pi-adapter-runtime.md` — lead-owned doc pre-pass, not
  part of this implementation's commit.
- The static-body-cut mapping logic itself (`cutStaticBody`,
  `replaceFirstTextItem`, the three-way branch) — read-only context for where
  the advisory call sites now live; not modified beyond threading the new
  holder parameter through.
- Owner-run Live check bullet ("arm a goal ... confirm the advisory appears
  on the first cycle only ... tune the tier ... confirm no further advisory")
  — owner-performed, not an automated test this plan drives.

## Codebase Findings

- `agents-plugin-pi/src/bridge.ts#L196-L202` — current
  `maybeAppendModelCatalogAdvisory` definition and signature:
  `(rawName: string, content: McpContentItem[], report: PiAliasTableReport,
  inheritModel?: string, catalogEmpty = true): McpContentItem[]`. Current
  gate: `if (rawName !== "workflow_manual" || (!report.unset &&
  report.rejected.length === 0)) return content;` — i.e. it already
  recomputes and re-appends every call while any rejection exists (the exact
  cadence the ticket/spec text says Phase 3 replaces). No holder parameter
  exists yet.
- `agents-plugin-pi/src/bridge.ts#L137-L164` — `PiAliasTableReport` shape and
  `computePiAliasTableReport`. Important non-obvious case confirmed against
  `test/bridge.test.ts#L602-L624` ("an isError result...", "unparsable
  text...", "a thrown call...", all asserting `{ unset: true, rejected: [] }`):
  `report.unset === true` can occur **with an empty `rejected` array**
  (every tier hit a transport/parse miss, not a genuine `unset`
  `config.resolve_agent` answer) as well as with `rejected` populated by four
  `why: "unset"` rows (the genuine never-configured case). A key builder that
  only reads `report.rejected` would collapse both distinct "unset" states
  into the same empty key as a clean table — it must special-case
  `report.unset && report.rejected.length === 0` with its own sentinel key,
  not fold it into the "clean table" empty-string key.
- `agents-plugin-pi/src/bridge.ts#L368-L400` — `dispatchMappedWorkflowManual`,
  current post-static-body-cut-rewrite shape (three-way branch, confirmed
  live in this survey, not from stale ticket prose): calls
  `maybeAppendModelCatalogAdvisory` at **three** sites, not two —
  `#L383` (cut-hit), `#L388` (`reason: "no-body"`, forward-unchanged), and
  `#L399` (fallback branch, after the `workflow_state` re-dispatch). All
  three currently pass the identical
  `(“workflow_manual”, content, piAliasTableReport, deps.inheritModel,
  !deps.catalog?.length)` argument shape and must all thread the new holder
  parameter identically (via `deps`, see next finding) — the ticket's "the
  two ... branches" undercounts by one against the now-landed rewrite; this
  survey plans for all three.
- `agents-plugin-pi/src/bridge.ts#L326-L340` — `WorkflowManualMappingDeps`
  interface: no `advisoryKeyHolder` field today. This is the single seam that
  must widen so all three `dispatchMappedWorkflowManual` call sites share one
  holder instance without duplicating a parameter on the function itself.
- `agents-plugin-pi/src/bridge.ts#L710-L736` — `startBridge`'s call into
  `dispatchMappedWorkflowManual`: builds the `deps` object inline
  (`callTool`, `catalog`, `inheritModel`, `staticBodySnapshot`,
  `notifyMappingDegraded`). Add `advisoryKeyHolder` here.
- `agents-plugin-pi/src/bridge.ts#L744-L750` — the **raw-dispatch** path
  (unmapped roles / any non-`workflow_manual` tool call): after
  `computeRawDispatchPiAliasTableReport`, calls
  `maybeAppendModelCatalogAdvisory(rawName, result.content,
  piAliasTableReport, inheritModel, catalog.length === 0)` directly — the
  fourth call site, and the one the ticket calls out by name as needing "the
  same gate."
- `agents-plugin-pi/src/bridge.ts#L644-L651` — `startBridge`'s per-session
  local state block (`tools`, `defaultKeyRef`, `manualSnapshotRef`,
  `staticBodySnapshotRef`, `notifiedMappingDegraded`) — the established
  pattern for "one holder/flag per `startBridge` invocation, closed over by
  the `execute()` callback and by the `dispatchMappedWorkflowManual` deps
  object." The new `advisoryKeyHolder` belongs in this same block, following
  the same shape convention (`{ current: T | undefined }`, no named
  interface used elsewhere in this block either, though `PiAliasTableReport`
  shows this file does export named interfaces when reused across 3+
  signatures, which the holder is).
- **Compaction-boundary detection (survey's own research; ticket text names
  only "the event the goal loop observes," not the file/wiring)**:
  `agents-plugin-pi/src/goal-loop.ts#L854-L877` registers `pi.on
  ("session_before_compact", ...)` (fires before compaction, sets
  `leadCompactingRef`), `pi.on("session_compact", ...)` (fires after a
  **successful** compaction, deferred via `setImmediate`), and `pi.on
  ("session_compact_failed", ...)`. `session_compact` is the correct hook for
  this phase: it fires only once compaction actually completed and context is
  genuinely lost (the ticket's own rationale — "a compacted context has lost
  the text and the advisory is the only pressure"); `session_before_compact`
  would also reset on a compaction that then fails, which is harmless
  (advisory re-appears once more, not silently lost) but imprecise, and
  `session_compact_failed` means context was **not** lost so must not reset.
- **Risk/decision — where to register the `pi.on("session_compact", ...)`
  listener (this is the one place this survey found genuine design freedom,
  flagged per the run's instruction to call it out even though it does not
  rise to a research escalation)**: `startBridge` itself runs **inside**
  `pi.on("session_start", ...)` in `agents-plugin-pi/src/index.ts#L356-L403`
  (confirmed: `handle = await startBridge(pi, {...})` at `#L394-L403` sits
  directly inside the `session_start` callback body), and `session_start` can
  fire more than once per process (`/reload`; see the explicit comment at
  `index.ts#L345-L349`: "Factory scope (like registerGoalLoop above, never
  inside session_start) so a `/reload` cannot stack duplicate agent_settled
  handlers" and the `pushRenderersRegistered` per-process guard flag at
  `#L354`, `#L376-L388`). `pi.on(...)` returns `void` — no unsubscribe/dispose
  API exists (`node_modules/@earendil-works/pi-coding-agent/dist/core/
  extensions/types.d.ts#L907-L935`). Registering `pi.on("session_compact",
  ...)` unconditionally inside `startBridge` would therefore stack one
  listener per `/reload`, mirroring exactly the failure mode this codebase's
  own comments already call out and avoid elsewhere. **Recommended fix**
  (self-contained to `bridge.ts`, matches this ticket's declared
  touched-file list which names only `src/bridge.ts` for Phase 3): a
  **module-level** `let compactionListenerRegistered = false` plus a
  **module-level** `let activeAdvisoryKeyHolder: AdvisoryKeyHolder |
  undefined` in `bridge.ts`; `startBridge` creates a fresh holder each call,
  assigns it to `activeAdvisoryKeyHolder`, and registers the `pi.on
  ("session_compact", ...)` listener only once (guarded by the boolean),
  with the listener always resetting whichever holder is currently active.
  This mirrors `index.ts`'s own already-established pattern of a
  factory-scope listener closing over a mutable `let handle` reassigned
  per-`session_start` (see `index.ts#L318-L337`, e.g. the `pi.on("input", ...)`
  handler reading `handle?.defaultSessionKeyRef.current`), just applied
  inside `bridge.ts` instead of `index.ts` so no other file (`BridgeHandle`,
  its five consumers in `ask.ts`/`fork.ts`/`execute-gateway.ts`/`spawner.ts`/
  `index.ts`) needs to change. Confidence: medium-high — the mechanism is
  directly precedented in this same codebase, not a novel integration; not
  escalating, but flagging so the implementer doesn't default to the more
  obvious-looking (but leak-prone) "just call `pi.on` at the top of
  `startBridge`" shape.
- `agents-plugin-pi/test/bridge.test.ts#L21-L35` — full current import list
  from `../src/bridge.ts`; add `AdvisoryKeyHolder` (if exported as a named
  type) and any new key-builder helper the implementer exports for direct
  testing.
- `agents-plugin-pi/test/bridge.test.ts#L389-L537` —
  `describe("dispatchMappedWorkflowManual", ...)` and
  `#L664-L708` — `describe("maybeAppendModelCatalogAdvisory", ...)`: the
  existing direct-call tests all invoke the function with 3-5 positional
  args and no holder. Appending the holder as a **trailing optional 6th
  parameter** (after `catalogEmpty`) — rather than inserting it before
  `inheritModel`/`catalogEmpty` — keeps every one of these existing tests
  (and the two inline calls at `#L576` and inside `dispatchMappedWorkflowManual`'s
  own tests) compiling and passing byte-for-byte unchanged: an omitted
  holder must behave exactly as today (always append while a rejection
  exists), which falls out naturally by treating "no holder passed" as "no
  dedup memory."
- `ai-docs/spec/pi-adapter-runtime.md#L733-L753` — confirmed anchor
  `{#260903-pi-model-catalog-unset-advisory}` and the exact cadence sentence
  at line 738 this phase's (lead-owned) spec pass amends: "recomputed and
  re-appended on every call while the condition holds, not once per
  session."

## Implementation Plan

1. `agents-plugin-pi/src/bridge.ts` (near `PiAliasTableReport`/before
   `maybeAppendModelCatalogAdvisory`, i.e. around current `#L150-L196`): add
   `export type AdvisoryKeyHolder = { current: string | undefined };` and a
   pure exported helper:
   ```ts
   export function buildAdvisoryKey(report: PiAliasTableReport): string {
     if (!report.unset && report.rejected.length === 0) return ""; // clean table
     if (report.rejected.length === 0) return "unset"; // all-miss/empty-catalog, no per-tier detail
     return [...report.rejected]
       .sort((a, b) => a.alias.localeCompare(b.alias))
       .map(({ alias, rejected }) => `${alias}=${rejected.model || "-"}:${rejected.why}`)
       .join(",");
   }
   ```
   (See Codebase Findings for why the `report.rejected.length === 0` +
   `report.unset` case needs its own sentinel distinct from the empty-string
   clean-table key.)
2. Same file, module scope (outside any function, e.g. beside
   `MERCENARY_RAW_PREFIX` around `#L479`): add
   `let compactionListenerRegistered = false;` and
   `let activeAdvisoryKeyHolder: AdvisoryKeyHolder | undefined;` — see the
   Codebase Findings risk note for why this lives at module scope rather than
   inside `startBridge` directly.
3. `agents-plugin-pi/src/bridge.ts#L196-L202` — widen
   `maybeAppendModelCatalogAdvisory` to accept the holder as a trailing
   optional 6th parameter and gate emission on it:
   ```ts
   export function maybeAppendModelCatalogAdvisory(
     rawName: string,
     content: McpContentItem[],
     report: PiAliasTableReport,
     inheritModel?: string,
     catalogEmpty = true,
     holder?: AdvisoryKeyHolder,
   ): McpContentItem[] {
     if (rawName !== "workflow_manual") return content;
     const hasRejection = report.unset || report.rejected.length > 0;
     if (!hasRejection) {
       if (holder) holder.current = undefined;
       return content;
     }
     if (holder) {
       const key = buildAdvisoryKey(report);
       if (key === holder.current) return content;
       holder.current = key;
     }
     const text = report.unset
       ? MODEL_CATALOG_ADVISORY
       : "> [!note]\n" + report.rejected.map(({ alias, rejected }) => `> ${formatTierWarning(alias, rejected, inheritModel, catalogEmpty)}`).join("\n");
     return [...content, { type: "text", text }];
   }
   ```
   Text-selection logic (`report.unset ? MODEL_CATALOG_ADVISORY : ...`) is
   unchanged from today — only the emission gate and the reset-on-clean
   branch are new.
4. `agents-plugin-pi/src/bridge.ts#L326-L340` — add
   `advisoryKeyHolder?: AdvisoryKeyHolder;` to `WorkflowManualMappingDeps`.
5. `agents-plugin-pi/src/bridge.ts#L383`, `#L388`, `#L399` — thread
   `deps.advisoryKeyHolder` as the 6th argument on all three
   `maybeAppendModelCatalogAdvisory` calls inside `dispatchMappedWorkflowManual`.
6. `agents-plugin-pi/src/bridge.ts` inside `startBridge`, in the per-session
   local-state block (`#L644-L651`): declare
   `const advisoryKeyHolder: AdvisoryKeyHolder = { current: undefined };`,
   set `activeAdvisoryKeyHolder = advisoryKeyHolder;`, and register the
   listener once:
   ```ts
   if (!compactionListenerRegistered) {
     compactionListenerRegistered = true;
     pi.on("session_compact", () => {
       if (activeAdvisoryKeyHolder) activeAdvisoryKeyHolder.current = undefined;
     });
   }
   ```
7. `agents-plugin-pi/src/bridge.ts#L710-L736` — add `advisoryKeyHolder` to
   the `deps` object passed into `dispatchMappedWorkflowManual`.
8. `agents-plugin-pi/src/bridge.ts#L744-L750` (raw-dispatch path) — pass
   `advisoryKeyHolder` as the 6th argument to the direct
   `maybeAppendModelCatalogAdvisory` call.
9. `agents-plugin-pi/test/bridge.test.ts` — add `AdvisoryKeyHolder` (and
   `buildAdvisoryKey`, if exported) to the import list (`#L21-L35`), then add
   a new `describe` block (or extend `describe("maybeAppendModelCatalogAdvisory", ...)`
   at `#L664-L708`) covering, each against a fresh `{ current: undefined }`
   holder per test:
   - Two consecutive calls with the same rejected-set report and the same
     holder: first call appends, second call returns `result === content`
     (same reference, no append) — mirrors the existing "does not append"
     test's assertion style at `#L689-L694`.
   - A changed rejected set (different tier or different `why`) with the
     same holder: appends again after a prior emission.
   - A clean-table report (`{ unset: false, rejected: [] }`) passed with a
     holder whose `.current` already holds a prior key: no append, AND
     `holder.current` becomes `undefined` afterward; a subsequent rejected
     report with the *same* key as before then appends again (proving the
     reset, not just a no-op).
   - Simulated compaction reset: emit once (holder now holds a key), manually
     set `holder.current = undefined` (this is the unit-testable surface for
     the compaction boundary — the real `pi.on("session_compact", ...)`
     wiring inside `startBridge` is not exercisable in `node --test`, same
     boundary this file already draws for other `startBridge`-internal wiring
     per its extraction doctrine), then call again with the *same* rejected
     set: appends.
   - The `report.unset === true, rejected: []` (all-miss/empty-catalog) case
     dedupes and resets identically to a populated-rejected case (exercises
     the `buildAdvisoryKey` sentinel from step 1).
   - The raw-dispatch call shape (call `maybeAppendModelCatalogAdvisory`
     directly with a `computeRawDispatchPiAliasTableReport`-shaped report and
     a holder, twice) — proving "the unmapped raw-dispatch path is gated the
     same way" per the ticket's Tests bullet; this is exercising the same
     underlying function both call sites share, since the raw path has no
     separate gating logic of its own.
   - No behavior change (regression guard): a call with **no** holder
     argument still appends every time a rejection exists, exactly as every
     pre-existing test in `#L664-L708` and `#L389-L537` already asserts
     unmodified.

## Verification Plan

- `cd agents-plugin-pi && node --test test/bridge.test.ts` — all existing
  `dispatchMappedWorkflowManual`, `computePiAliasTableReport`,
  `computeRawDispatchPiAliasTableReport`, and `maybeAppendModelCatalogAdvisory`
  tests must still pass unchanged, plus the new dedupe/reset/compaction/
  raw-dispatch/no-holder tests from step 9.
- `cd agents-plugin-pi && node --test test/spawner.test.ts test/bridge.test.ts test/agent-sidecar.test.ts` with `WS_PI_SPAWN_ROLE` unset — the targeted
  suite this ticket's Phase 1/2 verification already used (384 → 388 baseline);
  confirm no regression in either file from the threaded `holder` parameter.
- `cd agents-plugin-pi && npm test` (or the project's standard full command)
  to confirm no unrelated suite regressed; compare against the known
  pre-existing environment-bound failure count documented in Phase 1's
  `### Result` (130 failures, all pre-existing per that phase's `git stash`
  baseline diff) rather than expecting a clean run.
- Manual/no-tooling check: hand-trace `buildAdvisoryKey` against the four
  scenarios in the ticket's Tests bullet (same set / changed set / clean
  table / all-unset all-miss) to confirm the key strings differ exactly when
  the ticket says they must and match exactly when it says they must not.
- Owner-run Live check (out of scope for this implementation to execute):
  arm a goal in a session with one rejected tier, confirm the advisory
  appears on the first cycle only; tune the tier to a valid value and
  confirm no further advisory.

## Escalations

- None.
