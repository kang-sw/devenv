# Brief: 260620-capability-config — Phase 3 (single-vocabulary docs/skills/spec finalization)

## Intent

Align all caller-facing prose to the single capability vocabulary shipped in
Phases 1-2, mark the planned spec callout implemented, and sweep the mental
models deferred from Phases 1-2. After this phase `light`/`core`/`deep` appears
in docs/skills only as a documented read-compat synonym (or as a still-valid
`model`-field alias name), never as the primary tier vocabulary, and no
`{{.LightModel}}`/`{{.CoreModel}}`/`{{.DeepModel}}` var name or
`firstClassTierToAlias` bridge is described as live behavior.

## Scope Boundary

**Phase 3 only.** Documentation/skill/spec prose alignment. No code or test
changes — all behavior shipped in Phase 1 (`ea545a51`) and Phase 2 (`ddc2caf9`).
The only build-side action is regenerating the `agents-plugin-wsflow/rsrc`
mirror + `manifest.json` for the edited rsrc skill text.

**Scope principle (avoid over-reach).** Two distinct axes use these words:

1. **Tier vocabulary axis** — `tier:` frontmatter, `config.agents_tier` key,
   `playbook.render` `recommended-tier`, `ws.mercenary.register` tier, the
   `firstClassTierToAlias` bridge, the `{{.*Model}}` render vars. **Collapsed in
   Phases 1-2.** Align all prose here to capability-primary; light/core/deep are
   read-compat synonyms only.
2. **`model`-field alias axis** — `ws.mercenary.register`'s `model` field still
   accepts `light`/`core`/`deep` as portable alias *names* (Phase 1 retained
   `ModelAlias` name-recognition as read-compat). **This mechanic is unchanged.**
   Leave it intact; fix only statements that now contradict the collapse (the
   capability `tier` called "legacy", or a `core` registration default that is
   now `medium`).

## Caller-Visible Contract

- Docs/skills speak capability vocabulary (`small`/`medium`/`large`/`xlarge`) as
  primary; `light`/`core`/`deep` + `haiku`/`sonnet`/`opus` are described as
  read-compat synonyms accepted on input.
- The `🚧 Planned` spec callout `{#260620-tier-vocabulary-collapse-direct-model-map}`
  is converted to implemented (drop `🚧`, fold into body prose); the anchor stem
  is preserved (commit `## Spec` references it).
- `#260612` "remains keyed by `light`/`core`/`deep`" is reopened/amended; the
  superseded "pending `config.model_alias` rename" references are removed.
- No behavior change; render/resolve output is identical to post-Phase-2.

## Contract Instructions (file-by-file edit map)

### Spec — `ai-docs/spec/mcp-tools.md`
- L195-204 (`config.agents_tier` desc): `tier` is the capability tier; light/core/
  deep + haiku/sonnet/opus accepted as read-compat alias names. Drop the "alias
  name" framing as primary.
- L206-209 (`#260508-model-alias-config-tools`): reconcile — capability vocab is
  primary; light/core/deep are read-compat aliases at the concrete-model layer.
- L211-220 (`#260612-first-class-tier-vocabulary`): amend — `config.agents_tier`
  is now keyed by the capability tier (NOT "remains keyed by light/core/deep");
  the locked `light↦small…deep↦large` mapping became read-compat folding;
  `firstClassTierToAlias` retired; `xlarge` independently configurable.
- L222-241 (`🚧 Planned` callout): convert to implemented prose; remove `🚧` and
  the "Planned" framing; keep `{#260620-tier-vocabulary-collapse-direct-model-map}`.
- L276-277 (`#260619` constraint): the `config.agents_tier` re-home is by THIS
  collapse (`#260620-...`), not the superseded model-alias/role-tier rename.
- L292 (`#260620-config-prompt-override-tuning-tools`): drop "and the pending
  `config.model_alias` rename".

### Spec — `ai-docs/spec/named-agent-runtime.md` (minimal, model-field axis)
- L57-65, L221-225: keep model-field alias resolution (retained read-compat).
  Fix only: the capability `tier` is current (not "legacy"); if a `core` default
  is implied, it is now `medium`. Add a short read-compat note tying light/core/
  deep to the capability tiers; do not rewrite the alias mechanics.

### Skills (rsrc — source + mirror)
- `lead-tune/lead-tune.md` L47-51 (`On: tune model tier`): `config.agents_tier`
  maps a **capability tier** (`small`/`medium`/`large`/`xlarge`) to backend/model/
  effort; report the tier (not "the alias"). L57: drop the superseded
  `model_alias` term — point per-role *tier* tuning to `260611`.
- `lead-workflow-manual/lead-workflow-manual.md` L99-111: "default model alias" →
  default tier; "model-alias vars auto-inject" → the tier-derived model-hint var
  (`RoleModel`) auto-injects.
- `lead-implement/lead-implement.md` L214-216: the mercenary line "maps it through
  the `light↦small`/`core↦medium`/`deep↦large` alias layer" is STALE — Phase 1
  retired that bridge from the register path. Rewrite: mercenary delegation passes
  the recommended capability tier to `ws.mercenary.register`, which resolves it to
  a concrete per-harness model via capability-keyed config.

### Mental models (`ai-docs/mental-model/`)
- `named-agent-runtime.md` L42, L44: L44 — `firstClassTierToAlias` retired, tier
  passes through directly, empty-tier default core→medium. L42 — light touch
  (read-compat alias names retained; capability is the tier vocabulary).
- `mcp-runtime.md` L72: reconcile "compatibility tier names, model aliases … alias
  contract" to capability-primary + read-compat synonyms.
- `prompt-bundle.md` L21, L33, L46, L58, L77: L21 — drop `firstClassTierToAlias`
  mapping, `{{.CoreModel}}`/`{{.DeepModel}}` → `{{.RoleModel}}`, default core→
  medium. L33, L77 — `LightModel`/`CoreModel`/`DeepModel` → single tier-derived
  `RoleModel`. L46 — default core→medium; model-field aliases retained read-compat.
  L58 — `light`/`core` adjectives → `small`/`medium`.
- `workflow-skills.md` L28: reconcile — capability `tier:` is primary; light/core/
  deep are read-compat; drop the "locked mapping" framing as binding.

### Mirror + manifest
- After rsrc edits: `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  then `WS_REGEN_MANIFEST=1` manifest regen; the drift guard must pass on a normal run.

## Integration Test Instructions

No new tests. Verify:
- `spec_index.verify` clean (anchors intact after callout fold).
- `go build ./...` ok; `go test ./internal/wsrsrc/...` ok (mirror drift guard).
- `grep` confirms no remaining non-read-compat `light/core/deep`, no
  `{{.LightModel}}`-family var, no live `firstClassTierToAlias` description in
  docs/skills/rsrc.

## Implementation Strategy Decisions
- Direct-edit by the warm-context lead (Phases 1-2 just landed); the model-field
  vs tier-axis distinction needs that context to avoid mis-editing retained behavior.
- Preserve every spec anchor stem; the callout fold keeps `{#260620-...}`.
- `named-agent-runtime.md` spec gets minimal touch (retained model-field axis), not
  a rewrite.

## Rejected Alternatives
- Rewriting the `model`-field alias resolution to remove light/core/deep —
  rejected: that mechanic is retained read-compat, not collapsed.
- Deleting `#260612`/`#260508` anchors — rejected: amend bodies in place; anchors
  are referenced.

## Constraints
- No behavior/code/test changes; prose + mirror regen only.
- Apply the `lead-skill-authoring` checklist to any changed Invariant/Constraint
  line (none expected — changes are handler steps + notes).
- Run the skill-authoring Fresh-Reader Audit / Downstream Consistency Sweep
  (folded into the Review stage) since this is a terminology edit.
- All authored content English.

## Verification Contract
- `spec_index.verify` clean; `go build ./...` ok; `go test ./internal/wsrsrc/...` ok.
- Mirror byte-identical to source; manifest hashes current.
- No live light/core/deep tier-vocabulary or `{{.*Model}}`-family or
  `firstClassTierToAlias` references remain (read-compat synonym mentions allowed).

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/tickets/ready/260620-bug-ws-tier-vocabulary-split-undocumented.md` — [Must] Phase 3 + Decision + Documentation Conflicts.
- `agents-plugin/skills/lead-skill-authoring/SKILL.md` — [Must] invariant checklist + audit/sweep gates for skill edits.
- Phase 1 Result (`ea545a51`) + Phase 2 Result (`ddc2caf9`) in the ticket — [Must] exact shipped behavior (default medium, RoleModel, retained read-compat).
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` — [Maybe] migration anchor (model-name tables in config; prompt-factory direction).
