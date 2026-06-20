# Brief: 260619-feat-ws-config-prompt-tool-self-doc (Phase 1)

## Intent

Add the `config.prompt.*` MCP tool namespace and its first member,
`config.prompt.set(pointId, harness, prompt, scope?)`, which stores a
prompt-override value through the already-shipped layered config substrate so the
override is honored at render time by the existing marker engine
(`applyOverrideMarkers` / `buildOverrideLookup`). This is the data-plane *setter*
only.

## Scope Boundary

- **In scope (Phase 1 only):** the `config.prompt.set` setter — tool schema,
  dispatch, runtime-contract wiring, and an integration test proving a set
  override is honored at render for the matching `(pointId, harness)` at the
  resolved scope.
- **Out of scope (Phase 2, do NOT build):** the no-arg `config.prompt()` data
  listing (tree-scan of override markers + current values + `ws:lead-tune`
  pointer). Leave it entirely for the next slice.
- **Out of scope:** any tuning manual / how-to text (that lives in the future
  `ws:lead-tune` skill, ticket `260619-feat-ws-lead-tune-skill`).
- **Out of scope:** a CLI mirror for the setter; clearing/unsetting an override;
  registering `prompt.*` keys in the default-scope registry.

## Caller-Visible Contract

A new lead-only MCP tool:

```
config.prompt.set(session_key, pointId, harness, prompt, scope?)
```

- `session_key` (string, **required**): caller's ws session key. Required both to
  engage the keyed capability gate and because a session-scope write needs it.
- `pointId` (string, required): the override-point id, e.g. `DelegationSection`.
  Must be non-empty.
- `harness` (string, required, enum `claude | codex | *`): which harness bucket
  the override applies to. `*` is the cross-harness "all" bucket.
- `prompt` (string, required, non-empty): the override text that replaces the
  seed block at render.
- `scope` (string, optional, enum `session | project | global`): the storage
  scope. When omitted, the write lands in the item's declared default scope,
  which for unregistered `prompt.*` keys is **project** (`wsconfig.DefaultScope`
  fallback). An explicit `scope` wins.

Observable result: after a successful set, rendering any playbook that declares
the `(pointId)` override marker, for a session whose key resolves that scope,
shows the stored `prompt` in place of the inline seed for the matching harness
(harness-exact match wins; otherwise the `all`/`*` bucket; otherwise seed). The
tool returns a short confirmation line naming the point, harness, and resolved
scope. Spec: `260620-config-prompt-override-tuning-tools` (🚧).

## Contract Instructions

All paths under `agents-plugin-tool/`.

1. **Storage key + harness normalization.** The override key is
   `"prompt." + pointId + "." + harness`, exactly matching the reader
   `buildOverrideLookup` in `internal/mcp/playbook_tools.go:335-346`
   (`resolver.Get(key, "prompt."+pointId+"."+harness)`). When `harness == "*"`,
   store under the `all` bucket (key `prompt.<pointId>.all`) because the marker
   engine's cross-harness resolution reads `prompt.<pointId>.all` (see
   `applyOverrideMarkers` and mental-model `prompt-bundle`
   `#260619-prompt-override-marker-engine`). Accept `claude`, `codex`, `*` as the
   only valid `harness` inputs; reject others with a clear error.

2. **Write path — reuse the layered resolver, do NOT add storage.** Mirror the
   `ws.lead.prefer_mercenary` setter at `internal/mcp/server.go:833-864`:
   ```go
   adapter := sessionConfigAdapter{s: s.sessions}
   resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
   err := resolver.Set(key, prompt, wsconfig.SetOptions{
       ExplicitScope: parsedScope, // wsconfig.Scope("") when scope arg omitted
       SessionKey:    sessionKey,
   })
   ```
   `resolver.Set` (`internal/wsconfig/resolver.go:115-152`) already routes by
   target scope: session → session store overlay, project/global → flock'd file
   RMW. `wsconfig.Options{}` (ambient `WS_CACHE_HOME`/`WS_CONFIG_HOME`) is correct
   here — it is exactly what `config.agents_tier` and the `prefer_mercenary`
   setter use; do NOT call `resolveToolRoot` (config.* tools are not root-aware).
   Parse the optional `scope` arg to `wsconfig.Scope`; empty/absent → pass
   `ExplicitScope: ""` so `resolver.Set` applies `DefaultScope` (= project for
   `prompt.*`).

3. **No custom capability gating needed.** `roleAllowsTool`
   (`internal/mcp/server.go:2623-2637`) already blocks every `config.` prefix for
   `delegate` and `leaf` roles, so `config.prompt.set` is automatically lead-only.
   Do NOT add a `CapabilityCheck` hook and do NOT add the tool to any allowlist.

4. **Tool registration (the "Add an MCP tool" recipe).**
   - Add the schema object in `tools()` adjacent to the `config.agents_tier`
     entry (`internal/mcp/server.go:2053-2067`). Use `stringProperty` and
     `enumStringProperty` helpers; `harness` enum `["claude","codex","*"]`;
     `scope` enum from `wsconfig.ScopeSchemaEnum()` (`["session","project","global"]`);
     `required: ["session_key","pointId","harness","prompt"]`.
   - Add the dispatch `case "config.prompt.set":` in `callTool` adjacent to
     `case "config.agents_tier":` (`internal/mcp/server.go:471`).
   - **Runtime contract (mandatory — there is an exact-equality test).** Add
     `"config.prompt.set": ">=0.30.0-dev <0.31.0"` to the `tools` map in BOTH
     `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`.
     `LeadToolNames()` (`server.go:2547-2561`) auto-derives from `tools()`, and
     `cmd/ws-mcp/main_test.go:82-86` asserts `runtime.capabilities` tools EXACTLY
     equal the full-ws `runtime.json` `tools` keys — a missing entry fails the
     build.
   - Do NOT add `config.prompt.set` to `noAgentHiddenTool`
     (`server.go:2704-2712`). Unlike `config.agents_tier` (an agent-backend
     config, hidden in agentless wsflow), prompt overrides are a mode-neutral
     rendering concern consumed in wsflow too; keep it visible in both modes like
     `config.show`. (This is why the wsflow `runtime.json` also gets the entry.)

5. **No CLI mirror** in this phase (config.agents_tier has a CLI mirror; the
   prompt setter does not need one for Phase 1 — do not add one).

## Integration Test Instructions

- Extend `internal/mcp/prompt_override_test.go` (it already holds
  `TestShippedDelegationSectionSeedAndOverride` and helpers
  `newTestServerWithHarness`, `staticLookup`, `assertNoMarkerSyntax`,
  `assertManualStructureIntact`).
- Add a test that drives the REAL setter end-to-end (not `staticLookup`): invoke
  the `config.prompt.set` dispatch path on a test server with a known lead
  session key, then render `lead-workflow-manual` via the real
  `buildOverrideLookup(s, sessionKey)` + `printPlaybook` path and assert:
  - the stored override text replaces the `DelegationSection` seed
    ("Delegate to preserve lead execution context" seed phrase is absent; the new
    text is present) for the matching harness;
  - marker syntax is absent (`assertNoMarkerSyntax`) and manual structure intact
    (`assertManualStructureIntact`);
  - at least one scope assertion: set at an explicit scope (e.g. `session`) and
    confirm it resolves; optionally confirm a harness-exact vs `*` bucket
    precedence.
- Inspect how existing tests mint/seed a session key and set `WS_CACHE_HOME` for
  project scope (see `session_auth_test.go` and existing config tests) and follow
  that setup. Pass criteria: `go test ./internal/mcp/...` green, plus
  `go test ./cmd/ws-mcp/...` green (the runtime-contract equality test).

## Implementation Strategy Decisions

- Setter writes through `wsconfig.Resolver.Set`; it introduces NO new storage,
  file, or lock path (inherits the substrate's single-config-file + flock story).
- `session_key` is required (stricter than `config.agents_tier`) — justified by
  session-scope writes and keyed-gate engagement.
- Default scope for `prompt.*` overrides is `project` via the existing
  `DefaultScope` fallback (prompt keys stay unregistered in the scope registry).
- Visible in both full-ws and wsflow modes (prompt overrides are mode-neutral).

## Rejected Alternatives

- Registering `prompt.*` in the default-scope registry to force a non-project
  default — rejected; the marker engine deliberately keeps these dynamic, and
  project is a sensible blanket default for prompt tuning.
- A bespoke storage for prompt overrides — rejected by the epic
  (`260619-epic...`): overrides live inline in the single config file via the
  layered primitive.
- Hiding the setter in wsflow like `config.agents_tier` — rejected; prompt
  overrides are consumed during wsflow rendering, so a wsflow lead must be able
  to set them.

## Approach

- Add schema + dispatch + runtime.json (×2) entries.
- Implement the dispatch body: validate args, normalize `*`→`all`, build
  resolver, `Set` with parsed scope + session key, return confirmation.
- Add the end-to-end integration test; run both test packages.

## Constraints

- Claim pass only after reading full `go test` output.
- Match existing config-tool dispatch style; surgical additions, no rewrites of
  `config.agents_tier`/`prefer_mercenary`.
- AI-authored code comments in English.

## Out of scope

- `config.prompt()` listing (Phase 2); tuning manual; CLI mirror; clear/unset.

## Details

- Key constants: `wsconfig.ItemPreferMercenary` is the model for naming, but
  prompt keys are dynamic strings, not registry constants.
- `wsconfig.SetOptions{ExplicitScope, SessionKey, CapabilityCheck}` —
  use `ExplicitScope` + `SessionKey` only.
- `wsconfig.ScopeSchemaEnum()` → `["session","project","global"]` for the schema
  enum.
- Confirmation text suggestion: `prompt override set: <pointId>/<harness> (scope: <resolved-scope>)\n`.

## Verification Contract

- `go test ./internal/mcp/...` green, including the new end-to-end setter test.
- `go test ./cmd/ws-mcp/...` green (runtime-contract exact-equality test passes
  with the new `runtime.json` entries).
- `go build ./...` clean.

## References
<!-- [Must]: read before starting. [Maybe]: consult if uncertain. -->
- `ai-docs/mental-model/mcp-runtime.md` - [Must] config tools, layered resolver, "Add an MCP tool" recipe, roleAllowsTool, runtime.json contract.
- `ai-docs/mental-model/prompt-bundle.md` - [Must] override-marker pass, `buildOverrideLookup`, `prompt.<pointId>.<harness>` key, harness `all` bucket.
- `ai-docs/spec/mcp-tools.md` `#260620-config-prompt-override-tuning-tools` - [Must] the caller-visible contract this phase implements (setter half).
- `ai-docs/spec/mcp-tools.md` `#260619-prompt-override-marker-engine` / `#260619-layered-config-scope-model` - [Maybe] resolution + scope semantics.
- `internal/mcp/server.go:833-864` (prefer_mercenary setter), `:471-486` (config.agents_tier dispatch), `:2053-2067` (config.agents_tier schema) - [Must] copy the dispatch+schema+resolver pattern.
- `internal/wsconfig/resolver.go:115-152` + `scope.go` - [Must] `Set`, `SetOptions`, `Scope`, `DefaultScope`, `ScopeSchemaEnum`.
- `internal/mcp/prompt_override_test.go` - [Must] test harness + helpers to extend.
- `cmd/ws-mcp/main_test.go:55-92` - [Must] the runtime-contract exact-equality test that forces the runtime.json entry.
