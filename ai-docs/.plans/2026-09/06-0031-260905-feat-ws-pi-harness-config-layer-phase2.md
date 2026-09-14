# Plan: 260905-feat-ws-pi-harness-config-layer — Phase 2: `pi` harness bucket end to end

## Relevant Ticket Contract

- Phase 2 (ticket body): "Add `pi` to the harness enum and detection, wire it
  through `config.tune` (prompt overrides and `agents.tier` with `harness:
  "pi"`), the tier→model resolver used by `playbook.render` and the tier
  playbook variables, and the rsrc loader's harness-variant selection." Verify
  with Go tests: detection from a `ws-pi-bridge` clientInfo; `config.tune`
  accepting `harness: "pi"` for a prompt override and for `agents.tier`;
  `config.list` showing the bucket; `playbook.render` returning the
  `pi`-keyed model when set and the default when not; a `.pi.md` overlay
  selected only under a detected `pi` harness. Run the full Go suite plus the
  rsrc/skills regen if any rsrc text changes. Record in the Result whether the
  Pi bridge's clientInfo name had to change (it does not — see Codebase
  Findings).
- Finding (2026-09-05, dogfood via `lead-tune`): `agents.tier`'s harness
  selector declares no `Enum`, so `config.list` prints a bare `harness` and
  `config.tune`'s enum guard is skipped (only `aliasTargetKey` rejects an
  unknown value, later and with different wording). Phase 2 must also: give
  `agents.tier`'s harness selector an explicit enum (`claude`, `codex`, `pi`,
  `default`) so `config.list` lists it; make the `config.tune` rejection text
  name the full enum per key instead of the hard-coded "claude, codex, or *";
  have the `lead-tune` playbook tell the lead to read the selector enum from
  `config.list` before proposing a harness value (through the wsflow
  mirroring check).
- Decision "Harness identity comes from a structured `clientInfo` parse, not
  the substring matcher": at the `initialize` site only, parse
  `params.clientInfo.name` and treat exactly `ws-pi-bridge` as harness `pi`;
  this structured check runs **before** the substring detector
  (`detectHarnessFromRaw`), which must stay byte-identical for
  Codex/Claude; the `_meta` path (`detectHarnessFromMeta`) is unchanged.
- Decision "Closed enum stays closed; it gains one member": `pi` is added
  alongside `codex`/`claude` in every place the enum is spelled — both
  `normalizedHarness` copies, `promptHarnessEnum`, `aliasTargetKey`'s error
  text, and any `config.list` rendering of harness buckets. A free-form
  harness string is still rejected.
- Decision "Default bucket semantics unchanged": a `pi` session with no
  `pi`-keyed value still resolves through the existing fallback chain
  (`pi` → `default`); upgrading ws-mcp without touching config is a no-op for
  current Pi users. No new fallback special-case is added for `pi`.
- Open Decision #3 (settled): mercenary is untouched — no normalizer split
  between harness/backend enums, no new error branch for a `pi`-backend
  mercenary resolution. Do not add mercenary-specific code in this phase.
- Golden-rule exception (Decisions): ws-mcp Go changes in
  `agents-plugin-tool/` are permitted for this ticket only, and must be
  host-neutral — add `pi` only where `codex`/`claude` are already spelled, no
  Pi-specific logic.
- Non-goals: do not author `.pi.md` overlay content (only make overlays
  selectable); do not change Claude/Codex detection; do not touch mercenary
  backends.

## Out of Scope

- Phase 3 (tier-resolution read tool `config.resolve_agent`) and Phase 4
  (Pi adapter routing through it, catalog retirement) — separate phases with
  their own dependencies.
- `pi-adapter-runtime` spec anchor rewrites ("Model resolution: name alias,
  not tier", "Model catalog data file", "Unset-catalog advisory") — those are
  scheduled for Phase 4 per the ticket's `## Spec Impact` section.
- Any change to the Pi bridge (`agents-plugin-pi/`) beyond confirming its
  `clientInfo.name` is already `ws-pi-bridge` (it is — no adapter change
  needed, so nothing lands on the Pi track for this phase).
- Splitting `normalizedHarness` into separate harness/backend normalizers, or
  adding a mercenary `pi`-backend rejection path (Open Decision #3, settled
  against both).
- Authoring `.pi.md` overlay bodies for any shipped playbook (Non-goal).

## Codebase Findings

### Detection

- `agents-plugin-tool/internal/mcp/server.go:238` — `case "initialize":` calls
  `s.observeHarness("initialize", detectHarnessFromRaw(req.Params))`. This is
  the sole call site to change: route through a new structured-first detector.
- `agents-plugin-tool/internal/mcp/server.go:3221-3233` — `detectHarnessFromRaw`
  lowercases the whole params blob and substring-matches `codex`/`claude`/
  `anthropic`. Must stay byte-identical (Decision).
- `agents-plugin-tool/internal/mcp/server.go:3235-3244` — `detectHarnessFromMeta`
  (the `tools/call._meta` path) calls `detectHarnessFromRaw` directly; per
  Decision this path is unchanged, so it must NOT be routed through the new
  clientInfo check.
- `agents-plugin-tool/internal/mcp/server.go:3246-3255` — `normalizedHarness`
  (server.go copy): switch on `codex`/`claude` only, else `""`. This is
  consumed by `observeHarness` (line 3194-3213) to gate what gets stored as
  `s.sessionHarness`; a returned `"pi"` from the new detector would be
  silently dropped here today. Needs a `case "pi": return "pi"`.
- `agents-plugin-pi/src/bridge.ts:415` — `client.initialize({ name:
  "ws-pi-bridge", ... })`. Confirms the clientInfo name is already exactly
  `ws-pi-bridge`; no Pi-track change needed, so the ticket's "record whether
  the bridge name had to change" resolves to **no**.
- `agents-plugin-tool/internal/mcp/server_test.go:1578-1606` and
  `:1608-1630` — existing precedent for initialize-driven harness-detection
  tests (`TestServeStdioInitializeDetectsClaudeHarnessForAgentAlias`,
  `TestServeStdioCodexMetadataDetectsHarnessForAgentAlias`).
- `agents-plugin-tool/internal/mcp/server_test.go:1257-1293` —
  `TestServeStdioConfigAgentsTierUsesDetectedHarness` is the closest existing
  pattern: sends an `initialize` with `clientInfo.name` set, then
  `config.tune(key: agents.tier)` with no explicit `harness`, and asserts the
  JSON response contains the detected harness's bucket. Mirror this for `pi`
  (using `clientInfo.name: "ws-pi-bridge"`), skipping the `mercenary.register`
  portion of that test (Open Decision #3 says mercenary stays untouched/
  unreachable from Pi — no need to exercise it here).

### Harness enum / normalizer

- `agents-plugin-tool/internal/wsconfig/config.go:409-418` —
  `normalizedHarness` (wsconfig copy): same `codex`/`claude`-only switch, used
  by `aliasResolutionKeys` (line 389-407) to build the fallback-key list for
  tier lookups. Needs `case "pi": return "pi"` so a detected `pi` harness
  actually gets tried before falling to `default`.
- `agents-plugin-tool/internal/wsconfig/config.go:420-429` — `aliasTargetKey`:
  `""`/`"default"` → `"default"`; else delegates to `normalizedHarness`; else
  errors `"harness must be codex, claude, or default"` (line 428). Add `pi`
  to both the accepted set (via the `normalizedHarness` fix above) and the
  error text.
- `agents-plugin-tool/internal/mcp/config_registry.go:16-22` —
  `promptHarnessEnum = []string{"claude", "codex", "*"}`. Add `"pi"`.
- `agents-plugin-tool/internal/mcp/config_registry.go:151-188` — the
  `agents.tier` static registry entry. Its `SelectorFields` harness entry
  (lines 157-162) declares no `Enum`. Add
  `Enum: []string{"claude", "codex", "pi", "default"}` (a new package-level
  var, e.g. `agentsTierHarnessEnum`, mirroring `promptHarnessEnum`'s
  placement at lines 16-22).
- `agents-plugin-tool/internal/mcp/server.go:739-750` — `config.tune`'s
  harness resolution + enum guard:
  ```go
  harness, _ := params.Arguments["harness"].(string)
  harness = strings.TrimSpace(harness)
  if entry.HarnessApplicable {
      if harness == "" {
          harness = s.currentHarness()
      }
      if harnessEnum := fieldEnum(entry.SelectorFields, "harness"); len(harnessEnum) > 0 && !enumContains(harnessEnum, harness) {
          return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: harness must be one of claude, codex, or *; got %q", harness))
      }
  }
  ```
  **Risk signal**: adding an `Enum` to `agents.tier`'s harness field (per the
  Finding) makes this guard start firing for `agents.tier` for the first
  time. Today `TestServeStdioConfigAgentsTier`
  (`agents-plugin-tool/internal/mcp/server_test.go:265-283`) calls
  `config.tune(key: agents.tier, ...)` with **no** `harness` argument and
  **no** prior `initialize` call, so `harness` stays `""` after the
  fallback — under the current (enum-less) code this reaches
  `aliasTargetKey("")` and resolves to the `"default"` bucket. Naively adding
  the enum would make the guard reject empty harness (since `""` is not a
  member), regressing that test. Fix: normalize `""` to `"default"` **before**
  the enum check, but only when `"default"` is itself a member of that key's
  enum (true for `agents.tier`, false for `prompt.*`'s enum, which has no
  `"default"` member and must keep rejecting an unresolved empty harness
  exactly as today):
  ```go
  if harnessEnum := fieldEnum(entry.SelectorFields, "harness"); len(harnessEnum) > 0 {
      if harness == "" && enumContains(harnessEnum, "default") {
          harness = "default"
      }
      if !enumContains(harnessEnum, harness) {
          return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: harness must be one of %s; got %q", strings.Join(harnessEnum, ", "), harness))
      }
  }
  ```
  This keeps `TestServeStdioConfigAgentsTier` passing unchanged (same net
  effect: empty → `"default"`) while making the rejection text generic
  per-key (Finding requirement) and accepting `"pi"`.
- `agents-plugin-tool/internal/mcp/server.go:3854` — the `config.tune` tool
  schema's `harness` property description literally spells
  `"(claude, codex, or * for all)"`. Update to include `pi` (Decision: "every
  place the enum is spelled").
- `agents-plugin-tool/internal/mcp/playbook_tools.go:92-98` —
  `resolveTierModel` calls `wsconfig.ResolveAgentForHarnessConfig(configOpts,
  tier, "", "", harness)` directly with the detected harness string; no
  enum/allowlist inside `wsconfig`'s resolution path, so once
  `normalizedHarness` (wsconfig copy) knows `"pi"`, `playbook.render`'s
  `recommended-model`/`RoleModel`/`*TierModel` vars resolve the `pi` bucket
  automatically — **no additional code change needed here**.
- `agents-plugin-tool/internal/mcp/playbook_tools.go:311-327` —
  `withRecommendedRenderBinding` (used by `playbook.render` at
  `server.go:1533`) also just forwards `s.currentHarness()` into
  `ResolveAgentForHarnessConfig`; same "already generic" conclusion.
- `agents-plugin-tool/internal/wsconfig/config.go:296-359` — `defaultConfig`/
  `applyDefaultTiers`/`defaultModelAliases` seed only `default`/`codex`/
  `claude` buckets per tier (e.g. medium tier default: `{Backend: "codex",
  Model: "gpt-5.6-terra", Effort: "high"}`, line 311). No `pi` bucket is ever
  auto-seeded — matches "Default bucket semantics unchanged": an unset `pi`
  tier falls through `aliasResolutionKeys` (`["pi", "default", "codex"]`) to
  the `default` bucket's value, exactly the accepted "presentation debt"
  Decision.

### rsrc harness-variant selection (no code change expected)

- `agents-plugin-tool/internal/wsrsrc/loader.go:74-172` (`Load`,
  `resolvePlaybookPath`) — harness is validated only as a "bare stem"
  (`isBareStem`), no closed enum. `wsrsrc.Load(root, name, "pi", nil)`
  already resolves a `<name>.pi.md` overlay if one exists, purely from the
  filesystem — no ws-mcp code change is needed inside `internal/wsrsrc`.
- `agents-plugin-tool/internal/mcp/playbook_tools.go:735-743` (`renderPlaybookBody`)
  and `:963` (`printPlaybook`'s `wsrsrc.Load` call) both pass
  `s.currentHarness()` straight into `wsrsrc.Load`. So the only thing gating
  `.pi.md` overlay selection today is that `s.currentHarness()` can never
  return `"pi"` — fixed entirely by the detection + `normalizedHarness`
  (server.go copy) changes above. **No `internal/wsrsrc` change is required
  for this phase.**

### Test infrastructure already in place (reuse, do not reinvent)

- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:25-45`
  (`buildTestRsrcTree`) and `:47-55` (`newTestServerWithHarness`) — exactly
  the harness fixtures needed for a new `.pi.md` overlay test and a
  pi-terminology-fallback test.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:209-221`
  (`modelAliasPlaybookContent`, `tier: medium`, declares `RoleModel`) plus the
  `TestPlaybookPrintModelAliasFromConfig` /
  `TestPlaybookPrintModelAliasVariesWithConfig` pair at lines 444-506 — direct
  pattern for a new "pi-keyed model when set, default when not" test using
  `wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: ...}, "medium",
  "pi", uniqueModel, "pi")` (pass an explicit `backend` argument, e.g. `"pi"`,
  to avoid `InferBackend`'s substring match misclassifying a `provider/id`
  model string that happens to contain "claude" — not required for
  correctness here since the test only inspects the rendered `model` value,
  but keeps the fixture's `backend` field meaningful).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:253-321` — the three
  `TestPlaybookPrintUnknownHarness` / `...ClaudeHarness` / `...CodexHarness`
  tests are the pattern for a new
  `TestPlaybookPrintPiHarnessUsesNeutralTerminology`-style test proving
  `terminologyForHarness("pi")` still falls back to the host-neutral table
  (Non-goal: this ticket does not add Pi-specific terminology) even though
  structural overlay selection now works for `pi`.
- `agents-plugin-tool/internal/mcp/prompt_override_test.go:671-768`
  (`TestConfigPromptSetEndToEnd`) — pattern for a new positive round-trip test
  asserting `config.tune(key: "prompt.<point>", harness: "pi", ...)` succeeds
  and reports `"prompt override set: <point>/pi"` (not folded into `all`).
- `agents-plugin-tool/internal/mcp/prompt_override_test.go:1056-1141`
  (`TestConfigTuningCatalogProjectsPromptAndSchemaKnobs`) — line 1094 asserts
  the literal text `"harness[claude|codex|*]"` and line 1110 asserts
  `assertFieldEnum(t, promptKnob.SelectorFields, "harness", []string{"claude",
  "codex", "*"})`. Both must be updated to include `"pi"`. Lines 1138-1140
  (`agentsKnob := requireTuningKnob(...)`) currently assert only the `tier`
  and `effort` value-field enums for `agents.tier` — add a new
  `assertFieldEnum(t, agentsKnob.SelectorFields, "harness", []string{"claude",
  "codex", "pi", "default"})` assertion here (Finding requirement made
  testable).
- `agents-plugin-tool/internal/mcp/prompt_override_test.go:1143-1179`
  (`TestConfigTuningCatalogNoAgentOmitsFullWsKnobs`) — lines 1176-1178 assert
  the same `agentsKnob` value-field enums in no-agent mode; add the matching
  harness-selector-enum assertion here too, since `agents.tier` stays visible
  in wsflow (`NoAgentVisible: true`).
- `agents-plugin-tool/internal/wsconfig/config_test.go:162-183`
  (`TestSetAgentsTierForHarnessTargetsHarnessAlias`) — pattern for an optional
  but recommended focused unit test,
  `TestSetAgentsTierForHarnessTargetsPiAlias`, at the `wsconfig` package level
  (below the MCP layer), proving `SetAgentsTierForHarness(...,
  harness="pi")` + `ResolveAgentForHarness(..., harness="pi")` round-trip
  independent of the MCP dispatch layer.

### Docs / mirrored surfaces that spell the enum

- `agents-plugin-tool/cmd/ws-mcp/main.go:312` — CLI `config tune` flag help
  text: `"harness alias key to configure: codex, claude, or default"`. The
  CLI path calls `wsconfig.SetAgentsTierForHarness` directly (bypassing
  `internal/mcp`'s registry), so once the `wsconfig` normalizer accepts `pi`,
  `--harness pi` already works; only the help string is stale. Low-risk text
  update; no existing CLI test exercises a `pi` harness (existing tests at
  `main_test.go:619-639` use `claude`/`codex`), so no CLI test change is
  required by this phase's contract, though updating the string is still a
  "place the enum is spelled" per Decision.
- `ai-docs/spec/mcp-tools.md:1109-1112` — spells `` `harness` is `claude`,
  `codex`, or `*` `` for the prompt-override tuning tool. Add `pi`.
- `ai-docs/spec/mcp-tools.md:2117-2127` (`{#260609-playbook-harness-rendering}`)
  — states "The supported harness set is Claude and Codex; an unrecognized
  harness renders host-neutral text rather than failing." Needs a precise
  rewrite distinguishing two things this phase splits apart: (a) the
  detected-harness set now includes `pi` for structural `.{harness}.md`
  overlay selection, vs (b) the bundled terminology table
  (`playbookTerminologyTable`, `agents-plugin-tool/internal/mcp/playbook_tools.go:26-43`)
  remains Claude/Codex-only, with `pi` (and any other harness) falling back
  to the host-neutral row via `terminologyForHarness`'s `""`-key fallback —
  do not claim `pi` gets its own terminology row (Non-goal).
- `ai-docs/spec/mcp-tools.md:59-65` (`{#260508-mcp-payload-harness-detection}`)
  — describes only the substring-based detector. Add a sentence for the new
  structured `clientInfo.name == "ws-pi-bridge"` check and its precedence
  over the substring detector, per the Decision text.
- `agents-plugin/rsrc/lead-tune/lead-tune.md` — shared rsrc playbook (mirrored
  byte-identical into `agents-plugin-wsflow/rsrc/` per
  `ai-docs/manuals/wsflow-mirroring.md`'s "Rsrc Tree Provisioning" section,
  NOT the skills-mirror mechanism). The "Surface" section (lines 16-19) is the
  right place for a new bullet instructing the lead to confirm any `harness`
  value against that knob's `config.list`-reported selector enum before
  proposing it (Finding requirement) — this generically covers both "On: tune
  prompt override" (line 34-41) and "On: tune model tier" (line 64-70)
  without duplicating text in both handlers.

## Implementation Plan

1. **Structured clientInfo detection** — `agents-plugin-tool/internal/mcp/server.go`:
   - Add a new function near `detectHarnessFromRaw`
     (`server.go:3221-3233`), e.g. `detectHarnessFromInitializeParams(raw
     json.RawMessage) string`: unmarshal `raw` into a small local struct with
     `ClientInfo.Name`; if `strings.TrimSpace(name) == "ws-pi-bridge"` return
     `"pi"`; otherwise fall through to `detectHarnessFromRaw(raw)`. Leave
     `detectHarnessFromRaw` itself byte-identical.
   - Change the `"initialize"` case at `server.go:238` to call
     `detectHarnessFromInitializeParams(req.Params)` instead of
     `detectHarnessFromRaw(req.Params)`. Leave `detectHarnessFromMeta`
     (`:3235-3244`, used by the `tools/call._meta` path at `:494`) untouched.

2. **`normalizedHarness` gains `pi` (both copies)**:
   - `agents-plugin-tool/internal/mcp/server.go:3246-3255` — add
     `case "pi": return "pi"`.
   - `agents-plugin-tool/internal/wsconfig/config.go:409-418` — same addition.

3. **`aliasTargetKey` error text** — `agents-plugin-tool/internal/wsconfig/config.go:428`:
   change `"harness must be codex, claude, or default"` to
   `"harness must be codex, claude, pi, or default"`.

4. **Enum widening in `internal/mcp/config_registry.go`**:
   - Line 21: `promptHarnessEnum = []string{"claude", "codex", "pi", "*"}`.
   - Add a new var next to the enum block (lines 16-22), e.g.
     `agentsTierHarnessEnum = []string{"claude", "codex", "pi", "default"}`.
   - Lines 157-162 (the `agents.tier` entry's `harness` selector field): add
     `Enum: agentsTierHarnessEnum`.

5. **`config.tune` harness resolution** — `agents-plugin-tool/internal/mcp/server.go:739-750`:
   replace the enum-guard block with the empty→`"default"`-normalization +
   generic-message version shown in Codebase Findings, so:
   - `agents.tier` with no explicit/detected harness still lands in
     `"default"` (unchanged behavior, now enum-validated).
   - `prompt.*` keeps rejecting an unresolved empty harness exactly as today
     (its enum has no `"default"` member, so the normalization branch never
     fires for it).
   - Both keys now accept `"pi"` and report the full per-key enum in the
     error message (`strings.Join(harnessEnum, ", ")`) instead of the
     hard-coded `"claude, codex, or *"`.

6. **Tool schema description** — `agents-plugin-tool/internal/mcp/server.go:3854`:
   update `"(claude, codex, or * for all)"` to `"(claude, codex, pi, or * for
   all)"`.

7. **CLI help text** — `agents-plugin-tool/cmd/ws-mcp/main.go:312`: update
   `"harness alias key to configure: codex, claude, or default"` to include
   `pi`.

8. **Spec updates** — `ai-docs/spec/mcp-tools.md`:
   - Line ~1111: add `pi` to the spelled-out prompt-override harness set.
   - Lines ~2117-2127: rewrite the "supported harness set" sentence to state
     that `pi` is now a detectable/structural-overlay harness while the
     bundled terminology table stays Claude/Codex-only (falls back to
     host-neutral for `pi`).
   - Lines ~59-65: add a sentence describing the new
     `clientInfo.name == "ws-pi-bridge"` structured check and its precedence
     over the substring detector.

9. **`lead-tune` playbook** — `agents-plugin/rsrc/lead-tune/lead-tune.md`:
   add a bullet to the "Surface" section (after line 19) instructing the lead
   to confirm any proposed `harness` value is a member of that knob's
   `config.list`-reported selector enum before proposing it (do not assume
   `codex`/`claude` are the only accepted names). After editing, follow
   `ai-docs/manuals/wsflow-mirroring.md`'s "Rsrc Tree Provisioning" checklist:
   regenerate `agents-plugin/rsrc/manifest.json` and sync the
   `agents-plugin-wsflow/rsrc/` mirror (exact commands in Verification Plan).

10. **Tests** (add/update in the same change):
    - `agents-plugin-tool/internal/mcp/server_test.go`: add a new test near
      `TestServeStdioConfigAgentsTierUsesDetectedHarness`
      (lines 1257-1293) that sends `initialize` with
      `clientInfo.name: "ws-pi-bridge"`, then `config.tune(key: "agents.tier",
      value: {tier, backend: "pi", model, effort})` with no explicit
      `harness`, and asserts the JSON response contains a `"pi":{...}` bucket
      with the given backend/model/effort. Do not include a
      `mercenary.register` step (Open Decision #3: mercenary stays
      untouched/unreachable from Pi).
    - `agents-plugin-tool/internal/mcp/prompt_override_test.go`:
      - Update line 1094's expected text to `"harness[claude|codex|pi|*]"`.
      - Update line 1110's `assertFieldEnum` call to
        `[]string{"claude", "codex", "pi", "*"}`.
      - Add `assertFieldEnum(t, agentsKnob.SelectorFields, "harness",
        []string{"claude", "codex", "pi", "default"})` after line 1140 (in
        `TestConfigTuningCatalogProjectsPromptAndSchemaKnobs`) and after line
        1178 (in `TestConfigTuningCatalogNoAgentOmitsFullWsKnobs`).
      - Add a new positive round-trip test (pattern: `TestConfigPromptSetEndToEnd`,
        lines 671-768, condensed) asserting `config.tune(key:
        "prompt.<point>", harness: "pi", value: <text>, session_key)` returns
        `"prompt override set: <point>/pi"` and that a subsequent
        `printPlaybook`/`buildOverrideLookup` render picks up the stored
        `pi`-bucket text.
    - `agents-plugin-tool/internal/mcp/playbook_tools_test.go`:
      - Add a model-alias test (pattern: `TestPlaybookPrintModelAliasFromConfig`
        / `...VariesWithConfig`, lines 444-506) using
        `newTestServerWithHarness(t, "pi")` and the existing
        `modelAliasPlaybookContent` fixture (lines 209-221, `tier: medium`):
        one `CacheHome` with `wsconfig.SetAgentsTierForHarness(...,
        "medium", "pi", uniqueModel, "pi")` set (expect `uniqueModel` in the
        rendered body) and one fresh `CacheHome` with nothing set for `pi`
        (expect the seeded default-tier model, e.g. `"gpt-5.6-terra"` for
        medium, per `applyDefaultTiers` at
        `agents-plugin-tool/internal/wsconfig/config.go:311`).
      - Add an overlay-selection test (pattern: `TestPlaybookPrintUnknownHarness`
        / `...ClaudeHarness` / `...CodexHarness`, lines 253-321): build a
        fresh `buildTestRsrcTree` fixture with a base `<name>/<name>.md` and a
        `<name>/<name>.pi.md` overlay with distinguishable text; render with
        `newTestServerWithHarness(t, "pi")` (expect overlay text) and again
        with `newTestServerWithHarness(t, "")` or `"codex"` (expect base
        text, overlay absent) — proving the overlay is selected only under a
        detected `pi` harness.
      - Optionally add `TestPlaybookPrintPiHarnessUsesNeutralTerminology`
        (pattern: lines 253-321) asserting `terminologyForHarness("pi")`
        still equals the host-neutral row (Non-goal guard, cheap to add).
    - `agents-plugin-tool/internal/wsconfig/config_test.go` (recommended,
      not strictly required by the phase's test list): add
      `TestSetAgentsTierForHarnessTargetsPiAlias` mirroring
      `TestSetAgentsTierForHarnessTargetsHarnessAlias` (lines 162-183),
      proving `SetAgentsTierForHarness`/`ResolveAgentForHarness` round-trip
      for `harness="pi"` independent of the MCP dispatch layer.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... ./internal/wsconfig/... ./cmd/ws-mcp/...`
  (full package suites covering the new/updated tests above; also re-run the
  full `go test ./...` for the module per the phase's "run the full Go suite"
  instruction).
- Since `agents-plugin/rsrc/lead-tune/lead-tune.md` (a shared rsrc playbook)
  changes, run, in order, from `agents-plugin-tool/`:
  1. `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
     (regenerates `agents-plugin/rsrc/manifest.json` — required because the
     manifest hash-checks rsrc file contents).
  2. `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc/... -count=1 -run TestRegenerateWsflowRsrcMirror`
     (syncs `agents-plugin-wsflow/rsrc/` byte-for-byte, per
     `ai-docs/manuals/wsflow-mirroring.md`'s "Rsrc Tree Provisioning"
     after-edit checklist).
  3. `go test ./internal/wsrsrc/...` to confirm `TestWsflowRsrcMirrorUpToDate`
     and the manifest drift guard are green after regeneration.
  Note: the render input's phrasing ("`WSRSRC_REGEN_SKILLS=1 go test
  ./internal/wsrsrc/...`") names the **skills**-manifest regen env var, which
  applies to `agents-plugin/skills/` edits, not `agents-plugin/rsrc/`
  playbook body edits — `lead-tune.md` lives under `rsrc/`, so the correct
  regen env var per `ai-docs/manuals/wsflow-mirroring.md` is `WSRSRC_REGEN`
  (rsrc manifest) plus `WS_REGEN_WSFLOW_RSRC` (wsflow mirror), not
  `WSRSRC_REGEN_SKILLS`. Use the two commands above; `WSRSRC_REGEN_SKILLS`
  is not applicable here since no file under `agents-plugin/skills/` changes
  in this phase.
- Manual/spot check: after the code changes, run
  `go test ./internal/mcp/... -run TestServeStdioConfigAgentsTier` (both the
  existing unmodified test and the new pi-detection test) to directly confirm
  the empty-harness-still-defaults-to-`"default"` backward-compatibility
  claim in Codebase Findings.

## Escalations

- None.
