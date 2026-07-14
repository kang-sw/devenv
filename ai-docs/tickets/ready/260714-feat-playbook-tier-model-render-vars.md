---
title: "feat: generic tier→model render variables for playbook bodies"
related:
  260622-feat-playbook-render-tier-label: shares the render-time tier→model alias resolution seam; keep the two resolutions using one mechanism
sage-review-design: completed
sage-review-completeness: completed
---

# feat: generic tier→model render variables for playbook bodies

## Background

Playbook bodies sometimes need to name a concrete model *in prose* (e.g. tell
the lead which model to pass when spawning a subagent), but the render system
deliberately keeps model names out of the source: `playbookTerminologyTable`
(`agents-plugin-tool/internal/mcp/playbook_tools.go:23-24`) documents that model
names are NOT in the harness terminology table — they are always resolved from
config at render time via `resolveRoleModelVar`. Today the only model-bearing
var is `{{.RoleModel}}`, which resolves a single tier — the playbook's own
declared delegate tier — and cannot express "run at tier A, escalate to tier B"
inline.

The triggering case: the native Explore agent's default model changed from
haiku to inherit on the Claude harness. Because the native Explore is
*harness-spawned*, ws's tier→model config does not auto-inject into it — the
only lever ws has is to instruct the lead to pass `model:` explicitly on the
spawn call. The Scoped Exploration guidance in
`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:78-83` should
therefore name a concrete default model and an escalation model, per harness,
without hardcoding model names in host-neutral source.

## Decisions

Confirmed direction (chosen over the rejected alternatives below):

- Add generic, render-resolved template variables, one per capability tier:
  `{{.SmallTierModel}}`, `{{.MediumTierModel}}`, `{{.LargeTierModel}}`,
  `{{.XLargeTierModel}}`. Any playbook body may reference them.
- Resolve each at render time through the **existing** config seam
  (`wsconfig.ResolveAgentForHarnessConfig`, the same path `resolveRoleModelVar`
  uses), so values are per-harness and stay accurate as the tier→model mapping
  evolves. Current config defaults (`wsconfig/config.go:341-356`): claude
  `small→haiku`, `medium→sonnet`, `large→opus`, `xlarge→opus`; codex
  `small→gpt-5.6-luna`, `medium→gpt-5.6-terra`, `large/xlarge→gpt-5.6-sol`. No model names are added
  to `playbookTerminologyTable` — the `:23-24` contract is preserved.
- **Naming**: use the canonical taxonomy names `Small/Medium/Large/XLarge`
  (matching `wsconfig` tier vocabulary and the existing `RoleModel`), not
  `Light/Core/Deep` aliases.
- **Empty-value fallback**: `resolveRoleModelVar` yields `""` on error/misconfig.
  Because these vars sit mid-sentence, a resolved-empty value must fall back to a
  stable label (e.g. "the small-tier model") so a rendered sentence never
  contains a hole.
- **Author doctrine**: add one line distinguishing `{{.RoleModel}}` (the model
  THIS delegate runs at, from its declared tier) from `{{.*TierModel}}` (name a
  specific tier's model in prose). Prefer tier LANGUAGE in general; use
  `{{.*TierModel}}` only where a concrete model materially helps the reader
  (e.g. the lead must type `model:`).
- **Coordinate with `260622-feat-playbook-render-tier-label`**: that ticket
  resolves tier→model-alias at render time for the `recommended-tier` output
  line. This ticket exposes the same resolution as body vars. Both must share
  one resolution mechanism, not two parallel implementations.

## Constraints

- The var only makes an *instruction* concrete; it does not wire the model.
  The native Explore agent is harness-owned, so the lead must still pass
  `model:` on the `Agent()` spawn call. Guidance text that uses these vars must
  stay imperative about that.
- Reserved var names must be registered in `reservedToolVarNames` and resolved
  in `buildPlaybookVars`. Two distinct injection patterns exist — pick one and
  document it: unconditional auto-inject (like namespace vars in Layer 4 — then
  also add the four names to `wsrsrc.ImplicitVariableNames` so
  `substitutePlaybookVars` tolerates undeclared placeholders), or
  frontmatter-declared (like `RoleModel` in Layer 3 — injected only when the
  playbook declares the var in `variables:`). Either is acceptable if documented.

## Prior Art

- `resolveRoleModelVar` / `buildPlaybookVars` layering in
  `agents-plugin-tool/internal/mcp/playbook_tools.go`.
- Tier→model defaults in `agents-plugin-tool/internal/wsconfig/config.go:310-356`.
- `playbookTerminologyTable` harness idiom substitution (`playbook_tools.go:25-42`).

## Rejected Alternatives

- **Hardcode "haiku/sonnet" in the terminology table or directly in the
  workflow-manual text.** Rejected: violates the `playbook_tools.go:23-24`
  contract, breaks on codex (no haiku), and is fragile to harness default
  changes — the very trigger for this ticket.
- **Tier-language only, no concrete model in prose (A1).** Rejected: the lead
  must type an actual `model:` value when spawning the native Explore, so a
  concrete rendered model name is materially useful at that call site.
- **Explore-specific vars (`ExploreDefaultModel` / `ExploreJudgmentModel`, A2).**
  Rejected in favor of the generic per-tier set, which any playbook can reuse.

## Phases

### Phase 1: Add tier-model render vars and convert the first consumer

- Add `{{.SmallTierModel}}` / `{{.MediumTierModel}}` / `{{.LargeTierModel}}` /
  `{{.XLargeTierModel}}` as reserved, render-resolved vars using the existing
  config seam, with the empty-value fallback and author doctrine above.
- Convert the Scoped Exploration guidance in
  `lead-workflow-manual.md:78-83` to use them, e.g. "Dispatch `{{.ExploreAgent}}`
  as `{{.SmallTierModel}}` by default; escalate to `{{.MediumTierModel}}` only
  when the exploration requires judgment; specify the model explicitly — do not
  rely on the harness default."
- Regenerate the rsrc manifest; add/extend tests asserting per-harness
  resolution (claude vs codex) and the empty-value fallback.
- Verification: render `lead-workflow-manual` under claude and codex harness
  contexts and confirm the Scoped Exploration sentence materializes the correct
  per-harness models with no empty slots.

## Spec Impact

- Target spec area: playbook render / template-variable contract (candidate:
  `ai-docs/spec/plugin-runtime.md` or `ai-docs/spec/mcp-tools.md` render section).
- Expected caller-visible change: a documented set of reserved tier→model body
  vars available to playbook authors.
- Contract-first spec: no (todo backlog; final var-injection shape may be
  refined during implementation — address spec on ready promotion).
