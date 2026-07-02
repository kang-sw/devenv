# Brief: 260620-capability-config (Phase 1)

## Intent

Re-home the model-config surface in `internal/wsconfig` so it accepts, stores,
and resolves the **capability vocabulary** (`small`/`medium`/`large`/`xlarge`)
directly, retiring the `firstClassTierToAlias` bridge on the config/register
path. Existing `light`/`core`/`deep` user configs and stored agents must keep
resolving unchanged (read-compat), and `xlarge` becomes independently
configurable instead of folding onto `deep`/`large`. This is Phase 1 of ticket
`260620-bug-ws-tier-vocabulary-split-undocumented`.

## Scope Boundary

**In scope (Phase 1):** the config-key vocabulary, its read-compat, the
`config.agents_tier` tool/CLI surface, and the `ws.mercenary.register` tier
pass-through.

**Deferred — do NOT touch:**
- Phase 2: template vars (`resolveModelVars`, `reservedToolVarNames`,
  `{{.LightModel}}`→`{{.RoleModel}}`, playbook bodies, wsflow rsrc mirror).
  Leave `resolveModelVars` and the `LightModel`/`CoreModel`/`DeepModel` reserved
  names exactly as they are.
- Phase 3: docs/skills prose (`lead-tune`, `lead-workflow-manual`, `mcp-tools.md`
  spec, mental-model docs). Do not rewrite prose this phase.

## Caller-Visible Contract

- `config.agents_tier(tier: small|medium|large|xlarge, ...)` succeeds. The
  rejected-tier error names the capability set.
- `config.agents_tier(tier: light|core|deep)` still succeeds as a read-compat
  synonym (light→small, core→medium, deep→large), writing to the capability key.
- An existing persisted `config.json` keyed by `light`/`core`/`deep` (including
  per-harness `model_aliases` with custom models and effort) resolves to the
  **same** concrete `(backend, model, effort)` after this change as before it.
- A stored agent whose SQLite `tier` column holds `light`/`core`/`deep` still
  resolves and runs unchanged.
- `xlarge` is configurable independently of `large` (setting one does not change
  the other); when unset, `xlarge` defaults to the same model as `large`.
- `ws.mercenary.register(tier: <capability>)` routes the capability tier directly
  (no alias translation); `recommended-tier` from `playbook.render` is already a
  capability word and flows straight through.

## Contract Instructions

All edits are in `agents-plugin-tool/`. The change is a key-vocabulary swap plus
a load-time key migration; the resolution *machinery* (per-harness polymorphism,
effort, backend-affinity guard) is unchanged — only the keys it is keyed by
change. Do NOT route `config.agents_tier` through the layered `wsconfig.Resolver`
(it is deliberately not resolver-routed; keep it on the direct `wsconfig` path).

### `internal/wsconfig/config.go`

1. **`normalizedTier` (currently ~line 428) — make it the single canonicalizer to
   capability vocabulary.** Fold all synonyms into capability words:
   - `small`, `light`, `haiku` → `"small"`
   - `medium`, `core`, `sonnet` → `"medium"`
   - `large`, `deep`, `opus` → `"large"`
   - `xlarge` → `"xlarge"`
   - default → `""`
   (Today it returns `light`/`core`/`deep`; it must now return capability words
   and additionally accept `xlarge`. Keeping `haiku`/`sonnet`/`opus` preserves the
   stored-tier read-compat.)

2. **`ModelAlias` (currently ~line 220) — recognize alias *names* only, return the
   capability key.** Accept `small`/`light`→`"small"`, `medium`/`core`→`"medium"`,
   `large`/`deep`→`"large"`, `xlarge`→`"xlarge"`; everything else → `""`.
   **CRITICAL: do NOT add `haiku`/`sonnet`/`opus` here.** `ModelAlias` detects when
   a `model` field value is actually an alias-name redirect; `haiku`/`sonnet`/`opus`
   are concrete Claude model names and must remain concrete (callers:
   `config.go:179`, `config.go:320`, and `internal/wsagent/agent.go:525` — verify
   all three still behave: an alias-name `model` redirects to a tier, a concrete
   model name does not).

3. **Default maps — re-key to capability vocabulary and add `xlarge`.**
   - `applyDefaultTiers` (~264): keys `small`/`medium`/`large` + `xlarge`. Preserve
     the existing concrete defaults (small≈old light = codex/gpt-5.4-mini;
     medium/large≈old core/deep = codex/gpt-5.5); `xlarge` default == `large`
     default (codex/gpt-5.5) so xlarge≡large until a user splits them.
   - `defaultModelAliases` (~291): keys `small`/`medium`/`large` + `xlarge`; keep
     the per-harness shape (`default`/`codex`/`claude`) and the claude mappings
     (small→haiku, medium→sonnet, large→opus, xlarge→opus). Update the
     `tierOrDefault(tiers, "light"|"core"|"deep", ...)` literals to the capability
     keys; add the `xlarge` block.
   - Any other hardcoded alias literal in resolution defaults — notably
     `ResolveAgentForHarnessConfig`'s empty-tier fallback `tier = "core"`
     (~line 184) must become `tier = "medium"`.

4. **Load-time key migration (read-compat crux) — `Load` (~lines 75–85, after
   unmarshal, before `applyDefault*`).** Persisted configs hold `light`/`core`/`deep`
   keys; resolution now queries capability keys, so a user's custom `light` entry
   would be missed and silently replaced by the default `small`. Prevent this:
   normalize the keys of the loaded `cfg.Agents.Tiers` and `cfg.Agents.ModelAliases`
   maps via `normalizedTier` — move each legacy-keyed entry to its capability key
   when the capability key is not already present. Precedence: if both a legacy and
   its capability key exist, the capability key wins (drop the legacy duplicate).
   This is **in-memory normalization only** — do NOT rewrite the file and do NOT
   bump `schemaVersion` (stays 1). Subsequent `SetAgentsTier*` writes naturally
   persist capability keys.

5. **Error message (~line 108):** `"tier must be light, core, or deep"` →
   `"tier must be small, medium, large, or xlarge"`.

6. **Preserve unchanged:** the `AgentTier.Effort` field and effort handling
   (`normalizeOptionalEffort`), `useAliasMappingForBackend` backend-affinity guard
   (~396–410), `resolveAliasMapping`/`aliasResolutionKeys` structure, and the
   `SetAgentsTierForHarness` write flow. Only key vocabulary and the literals above
   change.

### `internal/mcp/server.go`

7. **`ws.mercenary.register` handler (~line 972):** change
   `Tier: firstClassTierToAlias(tier)` → `Tier: tier`. The capability tier flows
   straight to `RegisterOptions.Tier`; downstream `ResolveAgentForHarnessConfig`
   normalizes via `normalizedTier`. (Update the handler comment at ~957–964 that
   describes the first-class→alias mapping.)

8. **`config.agents_tier` schema (~lines 2204–2216):** change the `tier` enum
   (~2209) from `["light","core","deep"]` to `["small","medium","large","xlarge"]`;
   update the `tier` property description ("Model alias to configure." →
   "Capability tier to configure.") and the tool `description` (~2205) to capability
   wording. Keep `backend`/`model`/`effort`/`harness` properties unchanged.

### `internal/mcp/playbook_tools.go`

9. **Remove `firstClassTierToAlias` (~208–236)** — after step 7 its only non-test
   caller is gone. Delete the function and its doc comment. (Do not touch
   `resolveModelVars`/`reservedToolVarNames`/`withRecommendedTier` — those are
   Phase 2 / unrelated.)

### `cmd/ws-mcp/main.go`

10. **`config agents-tier` flag help (~line 298):** `"model alias: light, core, or
    deep"` → `"capability tier: small, medium, large, or xlarge"`.

## Integration Test Instructions

Test files: `internal/wsconfig/config_test.go`, `internal/mcp/mercenary_surface_test.go`,
and any `internal/wsagent/agent_test.go` cases asserting tier resolution. Run:
`cd agents-plugin-tool && go test ./internal/wsconfig/... ./internal/mcp/... ./internal/wsagent/...`
then `go build ./...`.

Required new/updated coverage:
- **Read-compat (load-time migration) — the load-bearing test.** A `config.json`
  written with legacy keys (`model_aliases: {light: {claude: {model: "X-custom"}}}`
  or a custom `tiers.light`) resolves, for a `small` (and `light`) query under the
  matching harness, to that **custom** model — proving the user's setting is
  honored, not overwritten by the default. Cover medium/core and large/deep too.
- `config.agents_tier(tier: small|medium|large|xlarge)` succeeds; an invalid tier
  produces the new capability error message.
- `xlarge` resolves independently: setting `large` does not change `xlarge`'s
  resolved model and vice-versa; unset `xlarge` falls back to the `large` default.
- `ModelAlias` returns "" for `haiku`/`sonnet`/`opus` (regression guard for the
  concrete-model distinction) and a capability key for `light`/`small` etc.
- `mercenary_surface_test.go`: remove the `firstClassTierToAlias` unit test; the
  tier-routing test registers with a capability `tier` directly and asserts the
  resolved custom model.

## Implementation Strategy Decisions (settled — do not reopen)

- **Read-compat is achieved by (a) load-time map-key normalization + (b)
  `normalizedTier` accepting legacy synonyms** — NOT by a one-way default re-key
  alone. (a) is mandatory; without it, persisted custom configs silently break.
- **No `wsstore/store.go` change.** The SQLite `tier` column is vestigial metadata;
  agents resolve and persist concrete `(backend, model, effort)` at registration,
  and the stored tier is not re-resolved at call time. Normalizing it would require
  importing `wsconfig` into `wsstore`, which the mcp-runtime / named-agent-runtime
  mental models explicitly forbid (reverse-import cycle). Old stored `light` labels
  are cosmetic; new registrations store capability words. (This corrects the
  ticket's "apply at SQLite tier read" bullet — record the discrepancy in the
  completion report.)
- **No `runtime.json` change.** Neither `agents-plugin/runtime.json` nor the wsflow
  mirror pins a tier enum — they carry only a version range for `config.agents_tier`.
  (Corrects the ticket's "update the full runtime.json enum if it pins one" bullet —
  it pins none.)
- **`config.agents_tier` stays off the layered `Resolver`.** It is the direct
  `wsconfig` read/write surface (`mcp-runtime.md:42`); do not route it through the
  scope resolver.
- **`xlarge` default == `large` default** initially (independently overridable),
  so behavior is unchanged for callers that never configure `xlarge`.

## Rejected Alternatives

- Redefining `ModelAlias` as `normalizedTier` — wrong: it would make `model: haiku`
  resolve as a tier redirect instead of a concrete model. The two functions have
  different domains by design.
- Normalizing only the query tier (not the loaded map keys) — silently ignores
  every persisted custom `light`/`core`/`deep` config.
- Editing `wsstore` to normalize the stored tier — out of scope and an import-cycle
  violation; the stored tier is not resolution-bearing.

## Approach

1. `config.go`: `normalizedTier` → capability canonicalizer; `ModelAlias` →
   capability-key alias-name detector; re-key default maps + add `xlarge`; add
   load-time key migration in `Load`; swap the `"core"` fallback literal; update
   the error string.
2. `server.go`: register pass-through + schema enum/description.
3. `playbook_tools.go`: delete `firstClassTierToAlias`.
4. `cmd/ws-mcp/main.go`: flag help.
5. Update/extend tests; run the suites + build; read full output.

## Constraints

- Existing `light`/`core`/`deep` configs and stored agents resolve identically
  before/after (verified by test).
- `schemaVersion` stays 1; no file rewrite on load; no migration machinery.
- Per-harness polymorphism, effort, and backend-affinity behavior unchanged.
- Phase 2/3 surfaces untouched.

## Out of scope

Template vars and playbook bodies (Phase 2); docs/skills/spec prose and mental-model
updates (Phase 3); any `wsstore` schema change; the wsflow rsrc mirror.

## Details

- `AgentTier{Backend, Model, Effort}` shape unchanged.
- `Agents.Tiers map[string]AgentTier` and
  `Agents.ModelAliases map[string]map[string]AgentTier` shapes unchanged; only the
  outer (alias) key vocabulary changes from `light/core/deep` to
  `small/medium/large/xlarge`, with legacy keys accepted on load.
- `ResolveAgentForHarnessConfig(opts, tier, backend, model, harness)` signature and
  flow unchanged; it begins with `normalizedTier(tier)` (now returns capability
  words) and uses `ModelAlias(model)` (now returns capability words).

## Verification Contract

- `go test ./internal/wsconfig/... ./internal/mcp/... ./internal/wsagent/...` passes.
- `go build ./...` clean (no new warnings).
- The read-compat test (custom legacy-keyed config resolves to the custom model)
  passes — this is the acceptance gate for the phase.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/tickets/ready/260620-bug-ws-tier-vocabulary-split-undocumented.md` — [Must] Phase 1 section + Verified Evidence (do NOT read beyond Phase 1 scope).
- `ai-docs/mental-model/named-agent-runtime.md` — [Must] model-alias/effort/backend-affinity invariants (lines 42–44); wsstore import boundary.
- `ai-docs/mental-model/mcp-runtime.md` — [Must] `config.agents_tier` not resolver-routed (line 42); config-tools contract (line 72); wsstore reverse-import warning.
- `agents-plugin-tool/internal/wsconfig/config.go` — [Must] the primary edit surface.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` — [Maybe] `firstClassTierToAlias` removal only.
