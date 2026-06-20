---
title: config.prompt.* setter + self-documenting override listing
parent: 260619-epic-ws-layered-config-prompt-tuning
spec:
  - 260620-config-prompt-override-tuning-tools
related:
  260619-feat-ws-layered-config-scope-substrate: prerequisite — setter writes through the layered scope primitive
  260619-feat-ws-prompt-override-marker-engine: prerequisite — listing tree-scans the override markers this defines
  260619-feat-ws-lead-tune-skill: consumer — the ws:lead-tune skill owns the tuning manual and calls this data plane; config.prompt() points users there
related-mental-model:
  - prompt-bundle
---

# config.prompt.* setter + self-documenting override listing

## Background

The prompt-override surface must be discoverable and tunable from inside the MCP
(epic `260619-epic-ws-layered-config-prompt-tuning`): an agent should be able to
see every overridable point and set one without external docs.

## Decisions

- **Own namespace `config.prompt.*`** for this new feature (does not disturb
  `config.agents_tier`/`config.show` or the pending `config.model_alias` rename).
- **`config.prompt.set(pointId, harness, prompt, scope?)`** writes a prompt
  override keyed by `(pointId, harness)` (`harness` ∈ `claude | codex | "*"`),
  stored at the chosen scope via the layered primitive
  (`260619-feat-ws-layered-config-scope-substrate`); overrides live inline in the
  single config file (no split resource-files).
- **No-arg `config.prompt()` = data listing + pointer (scope trimmed).** Tree-scan
  the rsrc playbooks for declared override markers (the A1 grammar from
  `260619-feat-ws-prompt-override-marker-engine`) and return the **data**: the list
  of override-points with their short `desc`, plus current overrides per
  harness/scope. End with a **one-line pointer to `ws:lead-tune`** for the tuning
  manual / how-to. The marker scan is the listing mechanism.
  - **The tuning manual is NOT rendered here.** It moved to the `ws:lead-tune`
    entry skill (`260619-feat-ws-lead-tune-skill`); keeping the manual out of the
    MCP output keeps `config.prompt()` a lean data surface. (Originally this
    ticket inlined a manual from an rsrc infra doc — superseded by the
    discuss-confirmed three-plane split: data here, manual + discovery in the skill.)

## Phases

### Phase 1: config.prompt.set setter

Add the `config.prompt.*` namespace and `config.prompt.set(pointId, harness,
prompt, scope?)` that writes the override through the layered config primitive at
the resolved scope.

Depends on `260619-feat-ws-layered-config-scope-substrate` Phase 1.

Verification: an override set via the tool is honored at render time by the
override engine for the matching `(pointId, harness)` and resolves at the
expected scope.

### Result (24e7e0d1) - 2026-06-20

Added the `config.prompt.set(session_key, pointId, harness, prompt, scope?)` MCP
tool (`internal/mcp/server.go`): schema + dispatch adjacent to
`config.agents_tier`, runtime-contract entries in both `agents-plugin/runtime.json`
and `agents-plugin-wsflow/runtime.json`, and end-to-end test
`TestConfigPromptSetEndToEnd` driving the real dispatch through render. Coverage
follow-up `85b7b63f` added validation negatives, the default-scope path, and a
delegate role-gate assertion (`TestConfigPromptSetValidationAndDefaultScope`,
`TestCapabilityScopedKeyGatesTools`).

Implementation facts established for Phase 2:
- `session_key` is **required** on the tool (engages the keyed gate; needed for
  session-scope writes).
- `harness` enum is `claude | codex | *`; `*` is stored under the `all` bucket
  (key `prompt.<pointId>.all`) to match `buildOverrideLookup`.
- Default scope for unregistered `prompt.*` keys is **project**
  (`wsconfig.DefaultScope` fallback); explicit `scope` wins.
- Lead-only via the existing `config.*` prefix gate in `roleAllowsTool` — no
  custom `CapabilityCheck`.
- **Visible in both full-ws and wsflow** (NOT added to `noAgentHiddenTool`):
  prompt overrides are mode-neutral rendering, unlike `config.agents_tier`. Phase
  2's `config.prompt()` should follow the same wsflow-visibility stance.
- Setter writes through `wsconfig.NewResolver(...).Set` with ambient
  `wsconfig.Options{}`; config.* tools are NOT root-aware (no `resolveToolRoot`).

Spec `260620-config-prompt-override-tuning-tools`: setter marked implemented,
`config.prompt()` kept as a `Planned 🚧` callout. Reviews: fit clean, correctness
clean (one cosmetic note), test gaps fixed.

### Phase 2: config.prompt() data listing

Implement no-arg `config.prompt()`: tree-scan override markers, render the point
list with `desc` and current overrides (harness/scope), and end with a one-line
pointer to `ws:lead-tune`. No tuning manual is rendered here (it lives in the
skill child).

Depends on Phase 1 and `260619-feat-ws-prompt-override-marker-engine` (marker
grammar to scan).

Verification: listing enumerates all declared override-points with descriptions,
reflects an override set in Phase 1 with its scope, and emits the `ws:lead-tune`
pointer.

### Result (4e4460a1) - 2026-06-20

Added the read-only no-arg `config.prompt()` MCP tool (`internal/mcp/server.go`
dispatch + `tools()` schema + `buildPromptOverrideListing`/
`formatPromptOverrideListing` helpers; `internal/mcp/playbook_tools.go`
`parseOverrideOpenMarkerDesc` + `scanOverridePoints`). It tree-scans the rsrc `.md`
files for declared open markers (deduped by pointId, first non-empty `desc` wins,
sorted), reports each point's id + `desc` + current override values per harness
bucket and resolved scope via `resolver.Get`, and ends with a one-line
`ws:lead-tune` pointer. Default output is text; `format:"json"` returns
`[{pointId, desc, overrides:[{harness, scope, value}]}]`.

Design facts:
- Mirrors `config.show`: optional `session_key` (session-scope values listed only
  when supplied); keyless caller passes the `config.*` gate; delegate/leaf keys
  blocked — no custom role check.
- Keyed on declared markers; orphan `prompt.*` values without a marker are not
  surfaced (per spec `260620`).
- The render engine's `parseOverrideMarkerPointId` is untouched; `desc` parsing
  lives in the separate `parseOverrideOpenMarkerDesc`.
- Registered in both `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json` to satisfy the launcher-contract
  exact-equality test (`cmd/ws-mcp` `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`).

Tests: `TestConfigPromptListEnumeratesDeclaredPoints` (enumeration + harness/scope
annotation + ordering), `TestParseOverrideOpenMarkerDesc` (7 subtests),
`TestScanOverridePoints` (dedup/`.md`-filter/sort), and a delegate-key gate
assertion in `TestCapabilityScopedKeyGatesTools`. Build + both packages green;
reviews fit/correctness/test all clean. Spec `260620` `config.prompt()` callout
promoted from Planned to implemented; mental-model `prompt-bundle` synced.

Phase 1 and Phase 2 are both shipped — this ticket is complete and ready to move
to `.done/`.
