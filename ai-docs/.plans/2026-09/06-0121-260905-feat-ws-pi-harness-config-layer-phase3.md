# Plan: 260905-feat-ws-pi-harness-config-layer — Phase 3: Tier resolution read tool for adapters

## Relevant Ticket Contract

- Phase 3 (ticket body): "Expose one MCP read tool (working name
  `config.resolve_agent(tier, harness?)`, or an equivalent machine-readable
  mode on an existing config read tool — decide at implementation and record
  the choice) that returns the resolved `{backend, model, effort}` for a fixed
  tier under the session's detected harness, applying the same fallback chain
  and normalization `playbook.render` uses, and reporting which bucket
  answered ... Register it in the config registry with the no-agent/harness
  applicability the other config tools carry, document it in `mcp-tools.md`,
  and cover it with Go tests for: each tier under `pi`, fallback to `default`
  with the answering bucket reported, and an unknown tier rejected."
- Decision "One model table": the Pi adapter's `model_name`, the implicit
  `"small"`, and `playbook.render`'s `recommended-model`/tier vars all
  resolve through the same fixed four-tier table (`small|medium|large|xlarge`)
  under harness `pi`; no user-named alias concept exists (Open Decisions #1,
  settled) — this tool must only accept the four fixed tiers (plus the
  existing read-compat synonyms `normalizedTier` already recognizes), never
  an arbitrary alias name.
- Decision "The adapter reads the resolution from ws-mcp, not from the config
  file": the adapter never parses the config store directly; it asks ws-mcp
  through a read tool so the fallback chain (`pi` → `default`), legacy-key
  normalization, and `InferBackend` all stay in one place — this tool is that
  single place.
- Golden-rule exception (Decisions): ws-mcp Go changes in
  `agents-plugin-tool/` are permitted for this ticket, must be host-neutral
  (add nothing Pi-specific beyond the enum member already landed in Phase 2;
  this tool must not special-case harness `pi`).
- Open Decisions #2 (settled 2026-09-05): "the read tool reports
  `resolved_from`; the adapter inherits on any non-`pi` answer; the resolver
  gains no harness special case." The resolver (`wsconfig`) must not grow a
  `pi`-only fallback branch; `resolved_from` is a generic report of whichever
  `aliasResolutionKeys` bucket answered, for any harness.
- Open Decisions #3 (settled): mercenary is untouched. No mercenary-specific
  branch or backend/harness normalizer split in this phase.
- Phase 2 Result (landed, `agents-plugin-tool/internal/wsconfig/config.go`,
  `internal/mcp/config_registry.go`, `internal/mcp/server.go`): `pi` is now a
  member of `normalizedHarness` (both copies), `aliasResolutionKeys`,
  `promptHarnessEnum`, and `agentsTierHarnessEnum`; `s.currentHarness()`
  returns `"pi"` for a session whose `initialize` carried
  `clientInfo.name: "ws-pi-bridge"`. Forward note in the Phase 2 Result: "the
  read tool must report the answering bucket since no `pi` tier is ever
  auto-seeded" — confirmed below (Codebase Findings, `defaultModelAliases`).
- Phase 4 (the consumer, out of scope here but shapes this tool): "route
  `resolveModelForAlias` (spawner) and the explore leaf's / `ws-execute
  complex:false`'s `"small"` through the bridge to the Phase 3 tool, treating
  a non-`pi` answering bucket as inherit; carry `effort` into `modelEffort`."
  This means the response must let the adapter (a) read `backend`/`model`
  directly when it wants to trust them, and (b) branch on `resolved_from`
  without additional lookups (e.g. `resolved_from == "pi"` vs anything else).
  No CLI or non-MCP consumer is named for Phase 4.
- Spec Impact: `mcp-tools`: "one new read tool (Phase 3) returning `{backend,
  model, effort, resolved_from}` for a fixed tier under the session's
  detected harness." Do not touch the `pi-adapter-runtime` anchors (Phase 4
  territory).
- Constraint (ticket `## Constraints`): any change to `agents-plugin/skills/`
  or shared rsrc text that names the harness enum needs the wsflow mirroring
  check — not triggered here (see Out of Scope).

## Out of Scope

- Phase 4 (`agents-plugin-pi/` adapter routing through this tool, catalog
  retirement) — separate phase, separate branch-of-work; this plan only
  builds and documents the tool the adapter will later call.
- Any change to `pi-adapter-runtime` spec anchors ("Model resolution: name
  alias, not tier", "Model catalog data file", "Unset-catalog advisory") —
  scheduled for Phase 4 per the ticket's `## Spec Impact`.
- A CLI subcommand (`cmd/ws-mcp/main.go` has `config list`/`config tune` CLI
  parity, but the ticket names only an MCP read tool for adapters; adding a
  CLI mirror is not requested and would be invented scope).
- rsrc/playbook text changes and the wsflow mirroring/regen steps: this phase
  adds no rsrc playbook prose (no doc names the tool's literal name inside
  `agents-plugin/rsrc/` or `agents-plugin/skills/`), so the mirroring check
  from `ai-docs/manuals/wsflow-mirroring.md` and the
  `WSRSRC_REGEN`/`WS_REGEN_WSFLOW_RSRC` regen commands used in Phase 2 are
  **not** needed for this phase. (If a future edit adds the tool's name to
  `pi-lead-guide.md` or another shared rsrc file, that edit must go through
  the same mirroring check Phase 2 used — noted for the record, not executed
  here.)
- Splitting harness/backend normalizers or adding a mercenary `pi`-backend
  rejection path (Open Decision #3, settled against both; unrelated to this
  read tool regardless).
- Reworking `ResolveAgentForHarnessConfig`'s existing empty/unknown-tier →
  `"medium"` coercion behavior for its current callers (`playbook.render`'s
  `resolveTierModel`/`resolveTierModelVars`, `wsagent.agent.go:536`); that
  behavior stays exactly as-is for those callers (see Codebase Findings —
  the new tool gets its own stricter validation path instead of changing the
  shared one).

## Codebase Findings

### Existing resolution chain (Phase 2 landed, reused not rebuilt)

- `agents-plugin-tool/internal/wsconfig/config.go:213-256`
  (`ResolveAgentForHarnessConfig`) — the function `playbook.render` already
  routes through. Line 214: `tier = normalizedTier(tier)`; lines 221-223: an
  empty/unrecognized tier (whatever `normalizedTier` can't classify, which
  returns `""`) is silently coerced to `"medium"`. This is the correct
  behavior for its current callers but is **not** acceptable for the new
  tool, which must reject an unknown tier per the ticket's test list. The new
  tool therefore needs a stricter wrapper, not a raw call to this function.
- `agents-plugin-tool/internal/wsconfig/config.go:368-387`
  (`resolveAliasMapping`) — the actual bucket-walk: normalizes the tier via
  `ModelAlias`/`normalizedTier` (lines 369-373), then walks
  `aliasResolutionKeys(backend, harness)` (line 378) against
  `cfg.Agents.ModelAliases[alias]`, falling back to `cfg.Agents.Tiers[alias]`
  (lines 383-385) if no keyed bucket matched at all. **It returns only
  `(AgentTier, bool)` — the matching key is discarded.** This is the exact
  gap the Phase 2 Result's forward note calls out ("the read tool must
  report the answering bucket"). Only one call site exists
  (`config.go:238`, inside `ResolveAgentForHarnessConfig`), so widening its
  return signature to `(AgentTier, string, bool)` is a self-contained,
  single-file change.
- `agents-plugin-tool/internal/wsconfig/config.go:389-407`
  (`aliasResolutionKeys`) — builds `[normalizedHarness(backend),
  normalizedHarness(harness), "default", "codex"]`, deduplicated in order.
  For our tool `backend` is always `""` (we resolve by harness only, not by
  an explicit execution backend), so for harness `"pi"` the walk order is
  `["pi", "default", "codex"]`; for harness `""` (undetected) it is
  `["default", "codex"]`.
- `agents-plugin-tool/internal/wsconfig/config.go:296-359`
  (`defaultConfig`/`applyDefaultTiers`/`defaultModelAliases`) — every fixed
  tier's `"default"` and `"codex"` buckets are always seeded by `Load` (via
  `applyDefaultModelAliases`, called at `config.go:90`); no `"pi"` bucket is
  ever auto-seeded. Consequence, confirmed by tracing the walk order above:
  an unset `pi` tier **always** resolves at the `"default"` key (never falls
  through to the trailing `"codex"` fallback, since `"default"` is always
  present after `Load`). This exactly matches Open Decision #2's expected
  test shape ("fallback to `default` with the answering bucket reported").
- `agents-plugin-tool/internal/wsconfig/config.go:448-462`
  (`useAliasMappingForBackend`) — gates mapping use only when an *explicit
  execution backend* was passed in; with `explicitBackend == ""` (our case)
  it always returns `true` (line 450-451). Safe to omit entirely from the new
  function — it's a no-op for this call shape.
- `agents-plugin-tool/internal/wsconfig/config.go:480-493`
  (`normalizedTier`) — accepts the four fixed tiers plus read-compat synonyms
  (`light`/`core`/`deep`/`haiku`/`sonnet`/`opus`... etc.), returns `""` for
  anything else. `SetAgentsTierForHarness` (line 144-147) already uses this
  exact pattern for its own unknown-tier rejection: `tier =
  normalizedTier(tier); if tier == "" { return Config{}, fmt.Errorf("tier
  must be small, medium, large, or xlarge") }`. This is the precedent to
  reuse verbatim for the new tool's validation (same acceptance set, same
  error wording) rather than inventing a second unknown-tier error shape.

### Design choice: new tool vs. mode on an existing tool — recommend a new tool

- `agents-plugin-tool/internal/mcp/server.go:3864-3890` — `config.list` is a
  broad catalog/view read (whole-config + tuning-knob catalog), and
  `config.tune` is a generic per-key **writer**; neither has a per-call
  "resolve one tier under one harness" shape today, and `config.list`'s
  schema (`format`, `session_key`, `root_override`) has no tier/harness
  selector to extend without conflating "list everything" with "resolve one
  value." Every other read surface in this file that answers a narrow,
  single-purpose question is its own tool (e.g. `git.status`, `git.diff`,
  `git.log`, `git.merge_base` at `server.go:3892-3939` are four separate
  tools rather than modes on one). **Recommendation: add a new dedicated
  tool, `config.resolve_agent(tier, harness?)`**, matching that established
  granularity and the ticket's working name — simpler schema, simpler tests,
  and no risk of `config.list`'s catalog-shaped JSON response growing an
  unrelated resolve-shaped branch.

### Tool registration / gating (how `config.list`/`config.tune` are wired, so the new tool matches)

- `agents-plugin-tool/internal/mcp/server.go:3863-3890` — the `tools()`
  schema entries for `config.list`/`config.tune`. Insert the new tool's
  schema entry immediately after this block (before `git.status` at line
  3892).
- `agents-plugin-tool/internal/mcp/server.go:665-880` — the `case
  "config.list":` / `case "config.tune":` dispatch bodies. Insert a new `case
  "config.resolve_agent":` immediately after the `config.tune` case body ends
  (line 880) and before `case "git.status":` (line 882).
- `agents-plugin-tool/internal/mcp/server.go:739-758` — `config.tune`'s
  harness normalization pattern to reuse verbatim: lowercase + trim, default
  to `s.currentHarness()` when empty. The new tool has no per-key
  `SelectorFields` enum to check against (it is not a registry-backed
  writer), so it skips the `enumContains`/`fieldEnum` step entirely — any
  harness string is accepted and passed straight to
  `aliasResolutionKeys`/`normalizedHarness`, which already reject anything
  it doesn't recognize by simply not matching a bucket (falls through to
  `default`). This matches "the resolver gains no harness special case"
  (Open Decisions #2) — an invalid/free-form harness argument here degrades
  gracefully to `default`, it does not need a hard validation error the way
  `config.tune`'s enum guard does for a *write*.
- `agents-plugin-tool/internal/mcp/server.go:4680-4694` (`roleAllowsTool`) —
  the `config.*` prefix is already blocked for `roleDelegate`/`roleLeaf`
  sessions (lines 4688, 4690); no change needed. This means the new tool is
  reachable by a `roleLead`-scoped caller (or a caller passing no
  `session_key` at all, since role gating at `server.go:502-515` only
  triggers when a `session_key` argument resolves to a known non-lead
  session). Phase 4's adapter is expected to call this from the
  spawner/lead-mediating side (resolving a model **before** issuing
  `ws-agent-spawn`/`ws-fork`), not from inside an already-spawned leaf, so
  this existing gating is compatible with the Phase 4 consumer as described
  — flagged here as a finding, not something this phase needs to change.
- `agents-plugin-tool/internal/mcp/server.go:4556-4575`
  (`toolSchemaRequiresSessionKey`) — `config.resolve_agent` is not in the
  explicit tool-name switch and `configKeyEntryForTool` (see next finding)
  will not resolve an entry for it, so it defaults to `false`: no forced
  `session_key`, matching a plain read tool (like `git.status` before
  `withSessionKeyToolSchemas` — wait, `git.status` *is* in that switch and
  does get a forced `session_key`; the new tool intentionally does **not**
  need one, since it is meant to be callable early in the adapter's own
  resolution path without first establishing a full lead session). No entry
  needed in that switch; leaving it out is the correct (no session_key
  requirement) behavior via the existing default branch.
- `agents-plugin-tool/internal/mcp/config_registry.go:241-268`
  (`configKeyEntryForTool`) — resolves a registry entry only when
  `entry.WriterTool == toolName || entry.ResetTool == toolName`; every
  existing `configRegistry` entry has `WriterTool: "config.tune"`. The
  function already special-cases `"config.list"`/`"config.tune"` themselves
  to return not-found (lines 258-260) with an explicit comment: "config.list/
  config.tune are the post-collapse generic tools ... not per-key writers."
  **`config.resolve_agent` is the same kind of thing** (a second generic,
  non-per-key-writer tool), so the correct move is to add it to that same
  early-return list rather than inventing a `configRegistry` row with a
  `WriterTool`/`ResetTool` that doesn't describe a real write path. This
  keeps `configKeyEntryForTool("config.resolve_agent")` returning
  `(configKeyEntry{}, false)`.
- `agents-plugin-tool/internal/mcp/server.go:4801-4819`
  (`noAgentHiddenTool`) — for any `config.`-prefixed name, it looks up
  `configKeyEntryForTool(name)`; **only hides the tool if an entry is found
  AND `!entry.NoAgentVisible`**. Once `config.resolve_agent` is added to
  `configKeyEntryForTool`'s early-return list (not-found), `noAgentHiddenTool`
  falls through its `config.` branch without hiding it — exactly how
  `config.list`/`config.tune` themselves stay unconditionally visible in
  no-agent mode today (there is no dedicated registry entry making them
  visible; they are simply never matched as hidden). **This satisfies "the
  no-agent/harness applicability the other config tools carry" without
  adding a new `configRegistry` struct field** — the "other config tools"
  (`config.list`, `config.tune`) get their applicability the same
  fall-through way, not through a per-tool registry row.

### Response shape / where to put the JSON struct

- `agents-plugin-tool/internal/mcp/server.go:2002-2010` (`configListView`) —
  precedent for a small JSON response struct placed near the tool's dispatch
  code, embedding/aliasing wsconfig types directly rather than re-deriving
  fields. Add a matching struct here, e.g.:
  ```go
  // resolveAgentTierResult is the config.resolve_agent JSON payload (260905
  // Phase 3): the resolved backend/model/effort for a fixed tier under a
  // harness, plus which alias bucket answered (Open Decisions #2) so a
  // caller (the Pi adapter) can tell a real pi-bucket hit from a
  // cross-harness fallback without re-deriving the resolution chain itself.
  type resolveAgentTierResult struct {
      Backend      string `json:"backend"`
      Model        string `json:"model"`
      Effort       string `json:"effort"`
      ResolvedFrom string `json:"resolved_from"`
  }
  ```
- `agents-plugin-tool/internal/mcp/server.go:692-695` /
  `:888-891` (`config.list`'s and `git.status`'s `wantsJSON(params.Arguments)`
  branch pattern) — reuse verbatim: default output is compact text, `format:
  "json"` returns the structured struct via `toolJSONResponse`.

### wsconfig-level function to add (new, not a rename of the existing one)

- Add `ResolveAgentTierForHarness(opts Options, tier, harness string)
  (backend, model, effort, resolvedFrom string, err error)` next to
  `ResolveAgentForHarnessConfig` (`agents-plugin-tool/internal/wsconfig/config.go:213-256`).
  Internals:
  1. `normalized := normalizedTier(tier)`; if `""`, return the
     `SetAgentsTierForHarness`-style error: `fmt.Errorf("tier must be small,
     medium, large, or xlarge; got %q", tier)`.
  2. `cfg, err := Load(opts)`; propagate `err`.
  3. `mapping, source, ok := resolveAliasMapping(cfg, normalized, "",
     harness)` (using the widened 3-return signature below). `ok` should
     always be true post-`Load` (every tier is seeded), but handle `!ok`
     defensively with an error rather than a panic/zero-value response.
  4. `backend, model, effort := strings.TrimSpace(mapping.Backend),
     strings.TrimSpace(mapping.Model), strings.TrimSpace(mapping.Effort)`;
     `if backend == "" { backend = InferBackend(model) }` (mirrors
     `ResolveAgentForHarnessConfig`'s line 252-254 fallback, minus the
     `useAliasMappingForBackend` gate, which is a no-op for `explicitBackend
     == ""` per the finding above).
  5. Return `backend, model, effort, source, nil`.
- Widen `resolveAliasMapping` (`config.go:368-387`) to return the matching
  key: `func resolveAliasMapping(cfg Config, alias, backend, harness string)
  (AgentTier, string, bool)`, returning the matched `key` from the
  `aliasResolutionKeys` loop (line 378-382), or the literal string `"tiers"`
  for the `cfg.Agents.Tiers[alias]` fallback branch (lines 383-385, which
  Codebase Findings above shows is realistically unreachable once
  `applyDefaultModelAliases` has run inside `Load`, but must still compile
  and return a source label rather than an empty string for defensiveness).
  Update the single call site at `config.go:238` to
  `if mapping, _, ok := resolveAliasMapping(cfg, tier, backend, harness); ok
  {` (discard the new return; `ResolveAgentForHarnessConfig`'s own behavior
  is unchanged — Out of Scope).

### Spec anchor location

- `ai-docs/spec/mcp-tools.md:895-932` (`## Config Tools {#260505-config-tools}`
  through `{#260513-harness-local-agent-tier-config}`) — the `agents.tier`
  paragraph ends at line 932 with its anchor. Insert a new paragraph
  immediately after line 932 and before the `workflow.prefer_subagent`
  paragraph (line 934), documenting `config.resolve_agent(tier, harness?)`:
  its read-only nature, the `{backend, model, effort, resolved_from}` shape,
  that it applies the same fallback chain as `agents.tier`/`playbook.render`
  (cross-reference `{#260513-harness-local-agent-tier-config}`), and that it
  is available in both full and agentless product modes (mirroring the last
  sentence of the `agents.tier` paragraph, line 931). Give the new paragraph
  its own anchor, e.g. `{#260905-tier-resolution-read-tool}`, matching this
  file's `{#<date>-<slug>}` anchor convention (seen throughout, e.g.
  `{#260702-config-unset-reset-to-builtin}` at line 951,
  `{#260703-bootstrap-staleness-warning}` referenced at line 955).

### Test infrastructure to reuse

- `agents-plugin-tool/internal/mcp/server_test.go:1257-1293`
  (`TestServeStdioConfigAgentsTierUsesDetectedHarness`) and `:1303-...`
  (`TestServeStdioConfigAgentsTierUsesDetectedPiHarness`, the direct Phase 2
  sibling) — exact dispatch pattern to mirror: send `initialize` with a given
  `clientInfo.name`, then a `tools/call` for the new tool with no explicit
  `harness`, and assert the JSON/text response. The pi-harness variant
  (`clientInfo.name: "ws-pi-bridge"`) is the direct template for "each fixed
  tier under a detected `pi` harness."
  `TestServeStdioConfigAgentsTierUsesDetectedPiHarness` also documents (in
  its own doc comment) why it skips `mercenary.register` — the new tests
  should follow the same "no mercenary" note (Open Decision #3).
- `agents-plugin-tool/internal/wsconfig/config_test.go:190-210`
  (`TestSetAgentsTierForHarnessTargetsPiAlias`) — direct template for a new
  `wsconfig`-level unit test of `ResolveAgentTierForHarness`, independent of
  the MCP dispatch layer: set a `pi`-bucket value via
  `SetAgentsTierForHarness`, then assert `ResolveAgentTierForHarness(...,
  "pi")` returns that value with `resolvedFrom == "pi"`; a second case with
  nothing set for `pi` asserts the seeded `"default"` tier value with
  `resolvedFrom == "default"`.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:888-891` — direct
  precedent for a focused `noAgentHiddenTool("config.resolve_agent")` unit
  assertion (expect `false`) rather than only an end-to-end `tools/list`
  check.
- `agents-plugin-tool/internal/mcp/server_test.go:1449-1497`
  (`TestServeStdioNoAgentModeHidesAgentBackedTools`) — spot-check pattern
  (not exhaustive) for an end-to-end no-agent-mode `tools/list` +
  `tools/call` visibility assertion; optional in addition to the direct unit
  check above.

## Implementation Plan

1. **`agents-plugin-tool/internal/wsconfig/config.go`**:
   - Widen `resolveAliasMapping` (lines 368-387) to `(AgentTier, string,
     bool)`, returning the matched `aliasResolutionKeys` key (or `"tiers"`
     for the `cfg.Agents.Tiers[alias]` fallback).
   - Update the sole call site at line 238 to discard the new middle return
     value; no behavior change to `ResolveAgentForHarnessConfig`.
   - Add `ResolveAgentTierForHarness(opts Options, tier, harness string)
     (backend, model, effort, resolvedFrom string, err error)` next to
     `ResolveAgentForHarnessConfig` (after line 256), per the Codebase
     Findings design above: validate via `normalizedTier` (reject with the
     `SetAgentsTierForHarness`-style error text on empty), `Load`, resolve
     via the widened `resolveAliasMapping`, `InferBackend` fallback when
     `backend == ""`.

2. **`agents-plugin-tool/internal/mcp/config_registry.go`**:
   - Extend the early-return list in `configKeyEntryForTool` (lines 258-260)
     to also cover `"config.resolve_agent"`, matching the existing comment's
     "generic tool, not a per-key writer" rationale for `config.list`/
     `config.tune`.

3. **`agents-plugin-tool/internal/mcp/server.go`**:
   - Add the `resolveAgentTierResult` struct near `configListView` (after
     line 2010), per Codebase Findings.
   - Add the new tool's schema to `tools()` immediately after the
     `config.tune` entry (after line 3890, before `git.status` at 3892):
     `name: "config.resolve_agent"`, description covering the read-only
     tier/harness resolution and the `{backend, model, effort,
     resolved_from}` shape, `inputSchema.properties`: `tier` (`stringProperty`,
     required, documented values `small|medium|large|xlarge`), `harness`
     (`stringProperty`, optional, "defaults to the detected session harness,
     or `default` when none is known"), `format` (`stringProperty`, same
     wording as `config.list`'s). `required: []string{"tier"}`.
   - Add `case "config.resolve_agent":` immediately after the `config.tune`
     case body ends (after line 880, before `case "git.status":` at 882):
     read `tier` and `harness` from `params.Arguments`; lowercase/trim
     `harness` and default to `s.currentHarness()` when empty (mirroring
     lines 741-750, but with **no** enum guard — see Codebase Findings on why
     a free-form harness degrades to `default` instead of erroring here);
     call `wsconfig.ResolveAgentTierForHarness(wsconfig.Options{}, tier,
     harness)`; wrap a returned error as `fmt.Errorf("config.resolve_agent:
     %w", err)`; on success build `resolveAgentTierResult{...}` and branch on
     `wantsJSON(params.Arguments)` for `toolJSONResponse` vs. a compact
     `"backend: %s\nmodel: %s\neffort: %s\nresolved_from: %s\n"` text
     response (mirroring `config.list`'s/`git.status`'s branch at lines
     692-695/888-891).

4. **Spec** — `ai-docs/spec/mcp-tools.md`: insert the new paragraph after
   line 932 documenting `config.resolve_agent`, per Codebase Findings
   (anchor `{#260905-tier-resolution-read-tool}` or similar, following the
   file's `{#<date>-<slug>}` convention).

5. **Tests**:
   - `agents-plugin-tool/internal/wsconfig/config_test.go`: add
     `TestResolveAgentTierForHarnessEachPiTier` (or one subtest per tier)
     covering all four fixed tiers (`small|medium|large|xlarge`) under
     harness `pi` with a set `pi` value — asserts `backend`/`model`/`effort`
     match what was set and `resolvedFrom == "pi"`. Add
     `TestResolveAgentTierForHarnessFallsBackToDefault` — nothing set for
     `pi`, asserts the seeded default-tier value and `resolvedFrom ==
     "default"`. Add `TestResolveAgentTierForHarnessRejectsUnknownTier` —
     e.g. `tier: "bogus"` returns an error matching "tier must be small,
     medium, large, or xlarge".
   - `agents-plugin-tool/internal/mcp/server_test.go`: add
     `TestServeStdioConfigResolveAgentUsesDetectedPiHarness` (pattern:
     `TestServeStdioConfigAgentsTierUsesDetectedPiHarness`,
     lines 1303-1330ish) — `initialize` with `clientInfo.name:
     "ws-pi-bridge"`, `config.tune(key: "agents.tier", value: {tier, backend:
     "pi", model, effort})` to seed a `pi` bucket, then `tools/call
     config.resolve_agent(tier: <tier>, format: "json")` with no explicit
     `harness`, asserting the JSON response's `resolved_from == "pi"` and the
     matching backend/model/effort — repeat/parametrize across the four
     tiers. Add a fallback variant with the same `initialize` but no
     `config.tune` seed, asserting `resolved_from == "default"` and the
     seeded default-tier values (e.g. `applyDefaultTiers`'s medium entry,
     `internal/wsconfig/config.go:311`). Add a no-detected-harness variant
     (no `initialize` call, or one with an unrecognized `clientInfo.name`)
     asserting the same `default` fallback. Add an explicit-`harness`-argument
     variant (e.g. detected `codex`, explicit `harness: "pi"` argument)
     proving the explicit argument overrides the detected one. Add an
     unknown-tier variant asserting `toolIsError` is true and the error text
     contains "tier must be small, medium, large, or xlarge".
   - `agents-plugin-tool/internal/mcp/playbook_tools_test.go` (or
     `server_test.go`, wherever `noAgentHiddenTool` unit tests already live —
     see `playbook_tools_test.go:888-891`): add a direct assertion that
     `noAgentHiddenTool("config.resolve_agent")` is `false`, i.e. the tool
     stays visible in agentless/wsflow mode.
   - Optionally extend `TestServeStdioNoAgentModeHidesAgentBackedTools`
     (`server_test.go:1480`) to include `"config.resolve_agent"` in its
     `visible` slice for an end-to-end `tools/list` confirmation.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... ./internal/wsconfig/...`
- `cd agents-plugin-tool && go test ./...` (full module suite, per the
  ticket's established Phase 2 precedent of running the full Go suite for
  ws-mcp changes under the golden-rule exception).
- No `WSRSRC_REGEN`/`WS_REGEN_WSFLOW_RSRC`/`WSRSRC_REGEN_SKILLS` regen step is
  required: this phase touches no file under `agents-plugin/rsrc/` or
  `agents-plugin/skills/` (see Out of Scope).
- Manual/spot check: `go test ./internal/mcp/... -run
  TestServeStdioConfigResolveAgent` to directly confirm the new dispatch
  path's pi/default/unknown-tier/explicit-harness behavior in isolation.

## Escalations

- None.
