# Plan: 260814-refactor-config-collapse-tuning-knobs-to-list-tune — Phase 1: Extract the per-key registry (no external surface change)

## Relevant Ticket Contract

- Introduce ONE per-key config registry that becomes the single source of truth
  for: value schema, `allowed_scopes`/`default_scope`, harness applicability,
  no-agent (wsflow) visibility, and authority requirement.
- Every existing consumer must read from it: the ten inline dispatch validators,
  `config.tuning`'s catalog builder (today it scrapes field/enum metadata out of
  the per-knob `tools/list` schemas via `tuningFieldFromSchema`), and the four
  currently-scattered gating tables (lead-only, session-key-required,
  no-agent-hidden, workflow-preference-writer).
- External surface (`tools/list`, both `runtime.json`, CLI) must stay
  byte-for-byte unchanged — this is a pure internal refactor.
- Resolve the `config.agents_tier` resolver-bypass decision against the real
  code: fold into `Resolver.Get/Set/Unset`, or keep a per-key adapter behind the
  registry, with evidence either way.
- Verification: full Go test suite green with no tool-surface diff
  (`runtime.json` and `tools/list` unchanged); wsflow runtime-contract test
  unchanged and passing.
- Preserved output contracts that must not regress: `config.tuning`'s
  `format:"json"` structured shape (spec `{#260625-tuning-catalog}`) and
  `config.show`'s per-value resolved-from-scope reporting (spec
  `{#260619-layered-config-scope-model}`).

## Out of Scope

- Adding `config.list` / `config.tune`, removing the ten per-knob tools,
  `runtime.json` changes (both packages), CLI mirror changes, playbook prose
  rewrites, and mental-model/spec doc rewrites — all Phase 2 (Decision 7: one
  atomic landing, not started here).
- Redesigning the `agents.tier` selector/effort enum taxonomy (260611's scope,
  explicitly excluded by Decision 9).
- `260626-bug-sage-review-config-setter-missing`'s `sage_review` writer
  re-design — happens after this ticket lands, not within it.
- Touching `roleAllowsTool`'s `config.*` prefix block itself (Decision 6's
  "stays lead-only via the prefix gate" mechanism) — it is a fifth, separate
  mechanism from the four named gating tables and needs no change; only the
  four named tables' `config.*`-specific branches are in scope.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go:668-984` — the ten `config.*`
  dispatch cases. Value-schema validation is inline per case (e.g.
  `switch value { case "on","off": default: return error }` at lines
  722-726, 738-742, 778-782, 816-820; `config.prompt.set`/`unset`'s harness
  switch at lines 850-857/911-918; `config.agents_tier`'s tier normalization
  via `wsconfig.SetAgentsTierForHarness`).
- `agents-plugin-tool/internal/mcp/server.go:2145-2239` (`buildTuningCatalog`)
  and `2286-2335` (`tuningFieldFromSchema`/`toolInputSchemaDetails`) — the
  catalog builder that scrapes `description`/`enum`/`required` straight out of
  the live `tools()` schema map by tool name + field name. This is the exact
  scrape Decision 1 names; it must be replaced by direct registry lookups.
  `tuningField` (2125-2130), `tuningWriter` (2120-2123), `tuningKnob`
  (2109-2118) are the existing wire types already shaped correctly for reuse —
  do not introduce parallel field/writer types.
- `agents-plugin-tool/internal/mcp/prompt_override_test.go:1043-1130`
  (`TestConfigTuningCatalogProjectsPromptAndSchemaKnobs`) — asserts exact enum
  sets and required flags per knob (e.g. `workflow.prefer_subagent` value
  enum `["on","off"]`, `workflow.prefer_mercenary` value enum
  `["on","off","hide"]`, `agents.tier` tier enum
  `["small","medium","large","xlarge"]`, `agents.tier` effort enum including
  `""`, `prompt` field `required: true`). Registry-derived `tuningField` values
  must reproduce these exactly or this test (and its siblings in
  `bootstrap_alarm_test.go:167-215`, `doc_coverage_alarm_test.go:151-197`)
  regress.
- `agents-plugin-tool/internal/mcp/server.go:3927-4049` — the ten tools()
  schema map literals (`config.show`, `config.agents_tier`,
  `config.workflow_prefer_subagent`, `config.workflow_prefer_mercenary`,
  `config.bootstrap_alarm`, `config.doc_coverage_alarm`, `config.prompt.set`,
  `config.prompt.unset`, `config.prompt`, `config.tuning`). Lowest-risk path:
  leave these literals structurally untouched (don't generate them from the
  registry object graph); instead extract the shared enum/description
  values the schema literals and the new registry both need into named
  package-level vars/constants, so both read the same underlying value without
  changing schema construction shape. This keeps the byte-for-byte
  `tools/list` guarantee trivially true (nothing about schema construction
  changes) while still making the registry the data owner in spirit.
- `agents-plugin-tool/internal/mcp/server.go:56-85` — gating table 1
  (`isLeadOnlyTool`) and table 4 (`workflowPreferenceWriterTool`, called from
  table 1). `workflowPreferenceWriterTool` hardcodes exactly the 4 tool names:
  `config.workflow_prefer_subagent`, `config.workflow_prefer_mercenary`,
  `config.bootstrap_alarm`, `config.doc_coverage_alarm`.
- `agents-plugin-tool/internal/mcp/server.go:4690-4707`
  (`toolSchemaRequiresSessionKey`) — gating table 2 (session-key-required),
  consumed by `withSessionKeyToolSchemas` (4666-4688) inside the `tools()`
  schema builder to auto-inject a required `session_key` property. Contains
  the identical 4 config tool names as table 4, interleaved with ~25 non-config
  names (exec.*, git.*, tickets.*, mercenary.*, etc.) that must stay untouched.
- `agents-plugin-tool/internal/mcp/server.go:4933-4949` (`noAgentHiddenTool`)
  — gating table 3 (no-agent-hidden). Only `config.workflow_prefer_mercenary`
  is config-specific here (plus the unrelated `mercenary.` prefix and
  `permanentlyHiddenTool`/`exec.` prefix checks, which must stay untouched).
- **Key finding — the 4 tables' config-relevant membership is not coincidental
  but is not a universal rule either.** All 4 tool names common to tables 1/2/4
  (`config.workflow_prefer_subagent`, `config.workflow_prefer_mercenary`,
  `config.bootstrap_alarm`, `config.doc_coverage_alarm`) map onto config item
  keys that are *also* registered `RegisterGlobalOnly` in
  `agents-plugin-tool/internal/wsconfig/scope.go:86-90`
  (`ItemWorkflowPreferSubagent`, `ItemWorkflowPreferMercenary`,
  `ItemBootstrapAlarm`, `ItemDocCoverageAlarm`). This matches ticket Decision 6
  ("global-only keys additionally require lead-key authority"). But
  `scope.go:88` also registers `ItemWorkflowSkepticalPosture` as global-only
  with **no** corresponding MCP writer tool today — so "global-only" alone is
  necessary-but-not-verified-sufficient in the abstract; for Phase 1 it is
  sufficient evidence that, for every knob that currently HAS a writer tool,
  `RequiresLeadAuthority == wsconfig.GlobalOnly(key)` holds exactly. The new
  registry's authority flag should derive from `wsconfig.GlobalOnly`, not
  duplicate a second hardcoded list — this also means table 4
  (`workflowPreferenceWriterTool`) collapses into "config keys where
  `wsconfig.GlobalOnly(key)` is true," removing one of the four scattered
  tables as a distinct hardcoded list entirely (it becomes a derived view).
  `config.agents_tier` and the `prompt.*` family are not global-only and
  correctly appear in none of tables 1/2/3/4 — only the untouched `config.*`
  prefix check in `roleAllowsTool` gates them.
- `agents-plugin-tool/internal/wsconfig/scope.go:108-147` — the existing
  wsconfig-level scope registry (`scopeRegistry`, `globalOnlyRegistry`,
  `DefaultScope`, `GlobalOnly`, `RegisterDefaultScope`, `RegisterGlobalOnly`,
  `ScopeSchemaEnum`). This is the current, real single source of truth for
  scope declarations. **The new MCP-layer registry must wrap/reference this,
  not duplicate it** — Phase 1 does not need to invent a richer
  per-key-`allowed_scopes` whitelist beyond what already exists (today any
  non-global-only item accepts any of session/project/global as an explicit
  write scope; only the global-only/not boolean is enforced). Inventing a
  stricter allow-list in Phase 1 would be a behavior change, which the phase
  forbids.
- `agents-plugin-tool/internal/wsconfig/config.go:143-202`
  (`SetAgentsTierForHarness`) — **evidence for the agents_tier resolver-bypass
  decision.** This function calls `Load(opts)` (line 159) and `save(opts, cfg)`
  (line 201) directly, writing into `cfg.Agents.ModelAliases[tier][key]` — a
  structured `AgentTier{Backend, Model, Effort}` value under a *different Config
  field* than the resolver's write path. The resolver's only write primitives
  (`resolver.go` `setOverrideInFile`/`r.sessionW.SetOverride`, lines 161-176)
  write `cfg.Overrides[itemKey] = value` — a flat `map[string]string`. There is
  no scope argument at all in today's `config.agents_tier` MCP dispatch case
  (`server.go:677-692`); it always resolves through `wsconfig.Options{}`
  (project-scope file), with no session/global write path. Folding this into
  `Resolver.Set/Unset` in Phase 1 would require extending the resolver to
  support non-string structured values or a second overlay field — that is a
  resolver *capability extension*, not a pure internal refactor, and directly
  risks changing on-disk write behavior that Phase 1 must not touch. **Decision
  for Phase 1: keep `config.agents_tier` as a per-key adapter behind the
  registry** (registry entry marks it as resolver-bypassing, referencing
  `SetAgentsTierForHarness`/`currentAgentTierMappings` as its read/write
  functions), matching the ticket's stated fallback. A true resolver fold is
  deferred past this ticket's scope.
- `agents-plugin-tool/internal/mcp/server.go:2211-2223` (`agentTiers` /
  `currentAgentTierMappings`, `wsconfig/config.go:264-284`) — the read side for
  `agents.tier`'s current value in the catalog; also bypasses the resolver
  (direct `wsconfig.Load`). Consistent with the write-side finding above.
- **Prompt knobs are a dynamic family, not static registry rows.** `points, _
  := scanOverridePoints(rsrcRoot)` (`server.go:2146`, `buildPromptOverrideListing`)
  discovers `pointId`s at runtime from the shipped playbook tree; each becomes
  one `tuningKnob` with `ID: "prompt." + p.PointId`. The registry needs exactly
  one **template** entry describing the `prompt.*` family's shape (selector
  fields `harness`/`scope`, value field `prompt` required, writer
  `config.prompt.set`, reset `config.prompt.unset`, harness-applicable,
  no-agent-visible, not lead-authority-gated, default scope = `wsconfig.DefaultScope("prompt.<id>.<harness>")` i.e. project via the general
  fallback) applied per discovered `pointId` at catalog-build time — not one
  row per concrete `prompt.<pointId>` key.
- `agents-plugin-tool/internal/mcp/session_auth_test.go:446`
  (`TestCapabilityScopedKeyGatesTools`) — existing coverage for
  `isLeadOnlyTool`/`roleAllowsTool`; should catch a mis-wired lead-only flag if
  the registry-driven rewrite of tables 1/4 breaks behavior.
- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go` (whole
  file) and `bootstrap_alarm_test.go`/`doc_coverage_alarm_test.go` — existing
  coverage for the 4 workflow-preference-writer dispatch cases (reset
  semantics, value validation, tuning-catalog knob presence/current-value
  reporting); these must stay green untouched, proving the inline-validator
  swap to registry-driven validation is behavior-preserving.
- Baseline confirmed green before this plan: `cd agents-plugin-tool && go build
  ./...` and `go test ./...` both pass on current `develop` HEAD (all packages
  `ok`, including `internal/mcp` and `internal/wsconfig`).
- `agents-plugin-tool/cmd/ws-mcp/main.go` — has the CLI mirror for `config show`
  / `config agents-tier` (confirmed present via grep); untouched in Phase 1,
  confirming CLI mirror is out of scope here per the ticket's Phase 2 sweep
  list.
- `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` — exist
  outside `agents-plugin-tool/`; Phase 1 must not touch either (verification
  target: no diff).
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py` — the wsflow
  runtime-contract test named in the verification boundary; Phase 1 makes no
  change that should affect it (no `runtime.json`/CLI/tools surface change),
  so "unchanged and passing" is satisfied by not touching it and confirming it
  still passes.

## Implementation Plan

1. Add a new file `agents-plugin-tool/internal/mcp/config_registry.go` defining
   the per-key registry:
   - Reuse the existing `tuningField` type (server.go:2125) for
     `SelectorFields`/`ValueFields` — do not add a parallel field-metadata type.
   - `type configKeyEntry struct { Key string; WriterTool string; ResetTool
     string /* "" when none */; SelectorFields []tuningField; ValueFields
     []tuningField; HarnessApplicable bool; NoAgentVisible bool;
     RequiresLeadAuthority bool; ResolverBacked bool /* false only for
     agents.tier */ }`.
   - `AllowedScopes`/`DefaultScope` are NOT duplicated fields — expose them as
     methods that delegate to `wsconfig.GlobalOnly(entry.Key)` and
     `wsconfig.DefaultScope(entry.Key)` (or a fixed constant for
     `agents.tier`, which has no session/global write path today — mark
     scope-fixed-to-project explicitly rather than inventing a fake
     `allowed_scopes` list).
   - Populate 5 static entries: `workflow.prefer_subagent`,
     `workflow.prefer_mercenary`, `bootstrap_alarm`, `doc_coverage_alarm`,
     `agents.tier`. `RequiresLeadAuthority` = true for the first four (derive
     from `wsconfig.GlobalOnly`, don't hand-duplicate the boolean), false for
     `agents.tier`. `HarnessApplicable` = true only for `agents.tier`
     (Decision 5) among the static entries. `NoAgentVisible` = false only for
     `workflow.prefer_mercenary` (mirrors current `noAgentHiddenTool`), true
     for the other four.
   - Add one `promptKnobTemplate` (not a `configKeyEntry` row — a distinct
     small struct or a function `configKeyEntry` factory taking a `pointId`)
     used by `buildTuningCatalog` per discovered override point: writer
     `config.prompt.set`, reset `config.prompt.unset`, selector fields
     `harness`/`scope`, value field `prompt` (required), `HarnessApplicable:
     true`, `NoAgentVisible: true`, `RequiresLeadAuthority: false`.
   - Extract the enum/description values needed by both the registry entries
     and the existing `tools()` schema literals (`server.go:3927-4049`) into
     shared package-level vars (e.g. `onOffEnum = []string{"on","off"}`,
     `preferMercenaryEnum = []string{"on","off","hide"}`, tier/effort enums)
     so schema literal and registry read the identical slice — do not touch
     the shape/order of the `tools()` map literals themselves, only swap their
     inline `[]string{...}` enum arguments for the shared vars where a 1:1
     match exists (verify each substitution against the current literal value
     before changing it).

2. Rewrite `buildTuningCatalog` (`server.go:2145-2239`) and delete
   `tuningFieldFromSchema`/`tuningFieldFromSchema`'s per-call scraping
   (`2286-2335`, `toolInputSchemaDetails`/`propertyString`/
   `propertyStringSlice` become unused and should be removed if nothing else
   references them — grep for other callers first) in favor of direct registry
   lookups: for each of the 5 static knobs, build the `tuningKnob` from the
   matching `configKeyEntry`'s `SelectorFields`/`ValueFields`/`WriterTool`/
   `ResetTool` instead of calling `tuningFieldsFromSchema("config.xxx", ...)`.
   For prompt knobs, apply the `promptKnobTemplate` per discovered `pointId`
   exactly as today's loop does. Preserve the `noAgentMode` early-return
   ordering (agents.tier before the cut, workflow.prefer_mercenary after —
   `server.go:2225-2236`) by driving the cut off each entry's `NoAgentVisible`
   flag instead of the current append-order-dependent early return, so the
   invariant is explicit rather than positional.

3. Rewrite the 7 inline value-validation blocks in the dispatch cases
   (`server.go:694-824` for the 4 workflow-preference writers,
   `826-937` for `config.prompt.set`/`unset`, `677-692` for
   `config.agents_tier`) to validate against
   `registryEntry.ValueFields`/`.SelectorFields` enum lists instead of the
   hand-written `switch value { case "on","off": ... }` blocks, preserving
   every existing error message string exactly (tests assert on error text in
   several of the referenced test files — grep each error string before
   deleting the literal switch to confirm no test depends on its exact
   wording, then keep the wording verbatim in the registry-driven replacement).
   `config.agents_tier` keeps calling `wsconfig.SetAgentsTierForHarness`
   directly (per the resolver-bypass decision above) — only its tier/enum
   validation constants move to the registry, not its write path.

4. Rewrite the four gating tables' config-relevant branches:
   - `workflowPreferenceWriterTool` (`server.go:78-85`): replace the 4-name
     switch with a lookup into the new registry by tool name →
     `RequiresLeadAuthority`. Keep the function's public shape/callers
     (`isLeadOnlyTool`) unchanged.
   - `toolSchemaRequiresSessionKey` (`server.go:4690-4707`): keep the
     non-config names as a literal list (untouched); replace only the 4
     trailing config names with a registry-driven check (tool has a registry
     entry with `RequiresLeadAuthority: true`) OR keep them listed literally if
     mixing a registry lookup into a `switch` over otherwise-static names adds
     more risk than value — decide based on which reads cleaner once the
     registry exists; either is acceptable as long as the four names still
     resolve `true` and no other name's behavior changes.
   - `noAgentHiddenTool` (`server.go:4933-4949`): replace the
     `case "config.workflow_prefer_mercenary": return true` arm with a
     registry lookup (`NoAgentVisible == false`) scoped to `config.*` tool
     names only; leave the `mercenary.` prefix and `permanentlyHiddenTool`
     checks untouched.
   - Add a small `configKeyEntryForTool(toolName string) (configKeyEntry,
     bool)` helper mapping each of the 5 static writer tool names (+
     `config.prompt.set`/`config.prompt.unset` via the prompt template) back
     to its registry entry, for tables 1/2/3 to share.

5. Run `go vet`/build to catch now-dead helper functions
   (`tuningFieldsFromSchema`, `tuningFieldFromSchema`, `toolInputSchemaDetails`,
   `propertyString`, `propertyStringSlice`) — remove any that have no other
   caller after the rewrite; keep any still used elsewhere (verify with grep
   before deleting each).

## Verification Plan

- `cd agents-plugin-tool && go build ./...` — must stay clean.
- `cd agents-plugin-tool && go test ./...` — full suite green, with particular
  attention to: `internal/mcp` (`server_test.go` incl.
  `TestServeStdioToolsListAndCall`, `TestServeStdioConfigShow`,
  `TestServeStdioConfigAgentsTier*`), `bootstrap_alarm_test.go`,
  `doc_coverage_alarm_test.go`, `prefer_mercenary_phase2_test.go`,
  `prompt_override_test.go` (incl. `TestConfigTuningCatalogProjectsPromptAndSchemaKnobs`,
  `TestConfigTuningCatalogNoAgentOmitsFullWsKnobs`,
  `TestConfigTuningShippedPromptKnobsOmitDelegationSection`),
  `session_auth_test.go` (`TestCapabilityScopedKeyGatesTools`), and
  `internal/wsconfig` (unaffected but must still pass since scope.go behavior
  is only read from, not changed).
- `git diff --stat agents-plugin/runtime.json agents-plugin-wsflow/runtime.json`
  — must be empty (no diff) after the change.
- Diff the raw `tools/list` JSON-RPC response before/after the refactor (e.g.
  capture via a throwaway `go test -run TestServeStdioToolsListAndCall -v` or a
  manual stdio probe) to confirm no schema text/enum/required-field drift on
  any of the ten `config.*` tool entries.
- `cd agents-plugin-wsflow && python3 -m pytest tests/test_wsflow_runtime_contract.py`
  (or the project's normal pytest invocation) — must still pass unmodified,
  confirming the wsflow contract test needs no Phase 1 change.

## Escalations

- None.
