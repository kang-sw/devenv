# Plan: 260714-feat-playbook-tier-model-render-vars — Phase 1: Add tier-model render vars and convert the first consumer

## Relevant Ticket Contract

- Add four generic, render-resolved vars — `{{.SmallTierModel}}`,
  `{{.MediumTierModel}}`, `{{.LargeTierModel}}`, `{{.XLargeTierModel}}` — usable
  by any playbook body, resolved through the existing config seam
  (`wsconfig.ResolveAgentForHarnessConfig`), the same path `resolveRoleModelVar`
  uses. No model names enter `playbookTerminologyTable`.
- Naming: canonical `Small/Medium/Large/XLarge` taxonomy (matches `RoleModel`
  tier vocabulary), not `Light/Core/Deep` aliases.
- Empty-value fallback: on resolver error/misconfig these vars must not render
  empty (they sit mid-sentence) — fall back to a stable label (e.g. "the
  small-tier model").
- Author doctrine: one line distinguishing `{{.RoleModel}}` (this delegate's
  own declared-tier model) from `{{.*TierModel}}` (naming a specific tier's
  model in prose); prefer tier language generally, use `{{.*TierModel}}` only
  where a concrete model materially helps (e.g. the lead must type `model:`).
- Constraint: guidance using these vars must stay imperative that the native
  Explore agent is harness-owned — the lead must still pass `model:` explicitly
  on `Agent()`; the var only makes the instruction concrete, it does not wire
  the model.
- Constraint: register the four names in `reservedToolVarNames`, resolve them in
  `buildPlaybookVars`. Two injection patterns are both acceptable if documented:
  (a) unconditional auto-inject like namespace vars (Layer 4) + add the four
  names to `wsrsrc.ImplicitVariableNames` so undeclared placeholders are
  tolerated, or (b) frontmatter-declared like `RoleModel` (Layer 3, only
  injected when the playbook lists the var in `variables:`).
- Convert the Scoped Exploration section
  (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:78-83`) to
  name a concrete per-harness default + escalation model, e.g. "Dispatch
  `{{.ExploreAgent}}` as `{{.SmallTierModel}}` by default; escalate to
  `{{.MediumTierModel}}` only when the exploration requires judgment; specify
  the model explicitly — do not rely on the harness default."
- Regenerate the rsrc manifest and add/extend tests asserting per-harness
  resolution (claude vs codex) and the empty-value fallback.
- Verification boundary: render `lead-workflow-manual` under claude and codex
  harness contexts and confirm the Scoped Exploration sentence materializes the
  correct per-harness models with no empty slots.
- Shared seam: sibling ticket `260622-feat-playbook-render-tier-label` (todo,
  not yet implemented) will later resolve tier→model alias for the
  `recommended-tier` output line and MUST reuse whatever resolution helper this
  phase introduces, not a parallel implementation.

## Out of Scope

- Sibling ticket `260622-feat-playbook-render-tier-label`'s `recommended-tier:
  medium (=sonnet)` render-output change — not started, only the shared
  resolution mechanism needs to stay reusable for it.
- Converting any other playbook body besides
  `lead-workflow-manual.md:78-83` to use the new vars (ticket names this as the
  "first consumer" only).
- Spec doc update to `ai-docs/spec/mcp-tools.md` — the ticket's Spec Impact
  section defers this ("final var-injection shape may be refined during
  implementation"); flagged below as a low-cost optional closing step, not a
  Phase 1 requirement.
- Any change to `RoleModel`'s own empty-fallback behavior (currently returns
  `""` on error) — the ticket only requires the fallback label for the four new
  vars, which sit mid-sentence; `RoleModel` usage sites are unchanged.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/playbook_tools.go:79-95` — `resolveRoleModelVar(harness, tier, configOpts)`
  calls `wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)`
  and returns `map[string]string{"RoleModel": model}`, `""` on error. This is the
  exact seam to reuse for the four fixed-tier vars — same call, fixed tier
  argument instead of the playbook's declared tier.
- `agents-plugin-tool/internal/mcp/playbook_tools.go:141-189` — `buildPlaybookVars` layering:
  Layer 1 caller context, Layer 2 terminology (declared-only), Layer 3 `RoleModel`
  (declared-only), Layer 4 namespace vars (`resolveNamespaceVars()`, unconditional,
  overrides caller context, no declaration required), Layer 5 `WorkflowLang`
  (declared-only). Layer 4 is the precedent to follow for "any playbook body may
  reference them" — namespace vars use exactly this unconditional-inject shape.
- `agents-plugin-tool/internal/mcp/playbook_tools.go:44-67` — `reservedToolVarNames`
  is built from `playbookTerminologyTable` keys + explicit `RoleModel` +
  `WorkflowLang` + `wsrsrc.ImplicitVariableNames`. Adding the four new names to
  `wsrsrc.ImplicitVariableNames` (see next finding) automatically populates this
  set through the existing `for _, name := range wsrsrc.ImplicitVariableNames` loop
  — no separate explicit-add line needed if the auto-inject pattern (a) is chosen.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc.go:52-55` — `ImplicitVariableNames = []string{"McpNamespace", "SkillNamespace"}`.
  `validate.go:150-154` and `playbook_tools.go:115-122` (`isReservedNamespaceVar`)
  both range over this slice to decide "usable without frontmatter declaration."
  Confirms: adding the four tier-var names here is what makes them usable
  unconditionally, mirroring the namespace-var precedent exactly (recommended
  pattern — see Implementation Plan step 1).
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:1-5` — frontmatter
  declares only `variables: [WorkflowLang]`, yet the body already uses
  `{{.McpNamespace}}` extensively (undeclared) — live proof the
  `ImplicitVariableNames` auto-inject pattern renders fine without frontmatter
  declaration. The Scoped Exploration section (lines 78-83) currently reads:
  "For scoped fact-finding, surveys, and one-turn answers, spawn host-native
  exploration workers with an English prompt and require cited evidence, gaps,
  and follow-up needs. For parallel dispatch, spawn multiple in one turn;
  collect all before synthesizing." — no `{{.ExploreAgent}}` reference yet.
  `ExploreAgent` is a Layer-2 terminology var (declared-only) — converting this
  section to use `{{.ExploreAgent}}` requires adding `ExploreAgent` to this
  file's frontmatter `variables:` list (the four tier vars do NOT need
  declaration under the recommended pattern).
- `agents-plugin-tool/internal/wsconfig/config.go:213-256` — `ResolveAgentForHarnessConfig(opts, tier, backend, model, harness)`
  normalizes `tier` via `normalizedTier`, loads config, and resolves
  `(backend, model, effort, error)`; each call does a fresh `Load(opts)` (reads
  config.json from disk). Calling this 4 times (once per fixed tier) per render
  is a minor redundant-I/O cost versus a single-load-then-4-lookups refactor;
  acceptable for a render-time (not hot-loop) call, consistent with the existing
  single-call-per-var pattern `resolveRoleModelVar` already uses. Not a
  correctness risk — flagged as a minor efficiency note, not a blocker.
- `agents-plugin-tool/internal/wsconfig/config.go:308-359` — tier defaults:
  claude `small→haiku, medium→sonnet, large→opus, xlarge→opus`; codex
  `small→gpt-5.6-luna, medium→gpt-5.6-terra, large/xlarge→gpt-5.6-sol`. Confirms the four fixed
  tiers used by the new vars are exactly `"small"`, `"medium"`, `"large"`,
  `"xlarge"` (lowercase, matching `normalizedTier` / `ModelAlias` vocabulary).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:181-193,401-423` —
  `modelAliasPlaybookContent` fixture and `TestPlaybookPrintModelAliasFromConfig`
  / `TestPlaybookPrintModelAliasVariesWithConfig` show the established test
  pattern for config-sourced model resolution (write a custom
  `wsconfig.SetAgentsTierForHarness` config, assert the rendered body contains
  the custom model string, not a baked-in name). Reuse this pattern for the four
  new vars.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:1621-1627` —
  `TestReservedToolVarNamesContainsRequiredNames` asserts a fixed name list is
  present in `reservedToolVarNames`; extend this list with the four new names.
- `ai-docs/ref/wsflow-mirroring.md:185-193` — after-edit checklist for any
  canonical rsrc change (`lead-workflow-manual.md` qualifies): run, in order,
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (both run from `agents-plugin-tool/`, both require `-count=1` — go's test cache
  otherwise returns a stale green `ok` without the write side effect).
  `lead-workflow-manual` is in the shipped wsflow skill set
  (`ai-docs/ref/wsflow-mirroring.md:33`), so this mirror sync is required, not
  optional.
- `ai-docs/tickets/todo/260622-feat-playbook-render-tier-label.md` — sibling
  ticket (not started) wants `recommended-tier: medium (=sonnet)` computed
  dynamically from the tier→model mapping at render time. The `withRecommendedTier`
  function (`playbook_tools.go:217-227`) currently only appends the bare tier
  string; whatever helper this phase introduces for tier→model resolution
  should be directly callable from a future `withRecommendedTier` change without
  restructuring — i.e., expose the fixed-tier resolution as a small standalone
  function (not inlined only inside a vars-builder), so 260622 can call it
  with the playbook's own declared tier.

## Implementation Plan

1. **Add a shared tier→model resolver helper** in
   `agents-plugin-tool/internal/mcp/playbook_tools.go` near `resolveRoleModelVar`
   (~line 89), e.g. `resolveTierModel(harness, tier string, configOpts wsconfig.Options) string`
   that wraps `wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)`
   and returns `""` on error (mirrors `resolveRoleModelVar`'s existing body).
   Have `resolveRoleModelVar` call this helper for its own tier so there is one
   resolution call site, not two — this is the shared seam
   `260622-feat-playbook-render-tier-label` will also need later.
2. **Add a fixed-tier vars resolver** `resolveTierModelVars(harness string, configOpts wsconfig.Options) map[string]string`
   that calls `resolveTierModel` once per fixed tier (`"small"`, `"medium"`,
   `"large"`, `"xlarge"`) and maps to `SmallTierModel`/`MediumTierModel`/
   `LargeTierModel`/`XLargeTierModel`. Apply the empty-value fallback here: when
   `resolveTierModel` returns `""`, substitute a stable label, e.g.
   `"the " + tierName + "-tier model"` (lowercase tier name), so no var value is
   ever empty.
3. **Wire injection as unconditional auto-inject (pattern a)**, matching the
   `resolveNamespaceVars()` Layer-4 precedent exactly:
   - In `agents-plugin-tool/internal/wsrsrc/wsrsrc.go:55`, extend
     `ImplicitVariableNames` to
     `[]string{"McpNamespace", "SkillNamespace", "SmallTierModel", "MediumTierModel", "LargeTierModel", "XLargeTierModel"}`.
     This automatically (a) makes `reservedToolVarNames` include the four names
     via its existing `for _, name := range wsrsrc.ImplicitVariableNames` loop
     (`playbook_tools.go:63-66`), (b) makes `isReservedNamespaceVar` accept them
     for caller-context validation, and (c) makes `substitutePlaybookVars`
     tolerate the placeholders without frontmatter declaration.
   - In `buildPlaybookVars` (`playbook_tools.go:176-181`), add a merge step
     alongside Layer 4 (or fold into the same layer) that unconditionally merges
     `resolveTierModelVars(harness, configOpts)` into `merged`, overriding caller
     context for these four names — same "cannot be spoofed through render
     context" property namespace vars have.
   - Update the doc comments at `playbookTerminologyTable`
     (`playbook_tools.go:16-24`) and `reservedToolVarNames`
     (`playbook_tools.go:44-51`) to mention the four tier-model vars are
     config-resolved like `RoleModel`, not terminology-table entries — preserving
     the "model names are NOT in this table" contract note.
4. **Add the author-doctrine comment** distinguishing `{{.RoleModel}}` from
   `{{.*TierModel}}` as a Go doc comment directly above the new
   `resolveTierModelVars` function (parallel to the existing doc comment on
   `resolveRoleModelVar`), since there is no separate playbook-authoring guide in
   this repo — code comments at the resolution site are the established
   documentation location for this contract.
5. **Convert the first consumer**:
   `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`:
   - Add `ExploreAgent` to the frontmatter `variables:` list (currently only
     `WorkflowLang`, line 3-4).
   - Replace the Scoped Exploration paragraph (lines 80-83) with wording that
     names `{{.ExploreAgent}}` as the dispatch target, `{{.SmallTierModel}}` as
     default, `{{.MediumTierModel}}` as the escalation tier for judgment-heavy
     exploration, and keeps the imperative "specify the model explicitly — do
     not rely on the harness default" instruction (per Constraints: the var
     only makes the instruction concrete, the lead must still pass `model:`).
6. **Regenerate manifests** (mandatory after any rsrc change, per
   `ai-docs/ref/wsflow-mirroring.md:185-193`), run from `agents-plugin-tool/`:
   - `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
7. **Add/extend tests** in `agents-plugin-tool/internal/mcp/playbook_tools_test.go`:
   - Extend `TestReservedToolVarNamesContainsRequiredNames` (line 1621) with the
     four new names.
   - Add a config-sourced resolution test for the four vars following the
     `TestPlaybookPrintModelAliasFromConfig` pattern (lines 401-423): write a
     custom per-tier config via `wsconfig.SetAgentsTierForHarness`, render a
     fixture playbook (or the real `lead-workflow-manual`), assert each
     `{{.*TierModel}}` resolves to the configured model and none render empty.
   - Add a per-harness golden test rendering `lead-workflow-manual` under
     `"claude"` and `"codex"` harness contexts (reuse
     `TestPlaybookPrintGoldenLeadCheckBlockers`'s real-rsrc-tree pattern, line
     ~1635) asserting the Scoped Exploration sentence contains the correct
     default config models (`haiku`/`gpt-5.6-luna` for small,
     `sonnet`/`gpt-5.6-terra` for medium) and no `{{.` placeholder remains.
   - Add a fallback test: force `ResolveAgentForHarnessConfig` into its error
     path (e.g. unresolvable tier/config error state, following how existing
     tests exercise `resolveRoleModelVar`'s `""`-on-error branch) and assert the
     stable fallback label appears instead of an empty substitution.
8. **Optional low-cost closing step**: update
   `ai-docs/spec/mcp-tools.md` around the `McpNamespace`/`SkillNamespace`
   reserved-implicit-vars paragraph (~line 1134-1139) to mention the four
   tier-model vars share the same unconditional-inject, config-resolved
   contract. Confirm with lead before including in this phase's commit since
   the ticket's Spec Impact section marks this as "not contract-first."

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestReservedToolVarNamesContainsRequiredNames`
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TestPlaybookPrint.*Tier.*Model'` (or whatever name the new tests take)
- `cd agents-plugin-tool && go test ./internal/wsrsrc/...` (manifest/mirror drift guards, after regenerating per step 6)
- `cd agents-plugin-tool && go test ./internal/mcp/...` full package run to catch regressions in `buildPlaybookVars`/`substitutePlaybookVars` callers
- Manual: render `lead-workflow-manual` via `printPlaybook`/`playbook.print` under both `"claude"` and `"codex"` harness contexts (or via the new golden test) and confirm the Scoped Exploration sentence shows `haiku`/`gpt-5.6-luna` (small) and `sonnet`/`gpt-5.6-terra` (medium) with no empty slots, per the ticket's stated verification boundary.

## Escalations

- None.
