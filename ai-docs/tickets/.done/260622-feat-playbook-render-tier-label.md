---
title: Playbook render exposes native spawn model and reasoning effort
related:
  260714-feat-playbook-tier-model-render-vars: provides the shared harness-aware tier resolution seam and body variables this ticket reuses
related-mental-model:
  - mcp-runtime
  - workflow-skills
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-21
---

# feat: playbook render exposes native spawn model and reasoning effort

## Background

`playbook.render` currently returns a prompt path plus a bare
`recommended-tier: <tier>` line. The capability tier is the correct
host-neutral decision vocabulary, but it does not tell a lead which concrete
native-spawn parameters implement that decision on the active harness.

The Codex native `spawn_agent` runtime accepts `model` and
`reasoning_effort` even when a host-provided tool description omits those
optional fields. A live probe confirmed that `model: "gpt-5.6-luna"` with
`reasoning_effort: "medium"` is accepted; the rejected field spelling is
`effort`. The spawned child still sees only its GPT-5 family identity, so child
self-report is not a reliable binding check. The lead must receive the binding
before spawn.

The source defaults are:

| Capability tier | Codex model | Codex reasoning effort | Claude model class |
|---|---|---|---|
| `small` | `gpt-5.6-luna` | `medium` | Haiku |
| `medium` | `gpt-5.6-terra` | `high` | Sonnet |
| `large` | `gpt-5.6-sol` | `high` | Opus |
| `xlarge` | `gpt-5.6-sol` | `xhigh` | Opus |

These are defaults, not hardcoded playbook facts: user-local harness mappings
remain authoritative at render time.

## Decisions

- Keep `small`/`medium`/`large`/`xlarge` as the capability vocabulary. Concrete
  provider models and effort are execution bindings, not replacement tiers.
- Preserve the existing `recommended-tier: <tier>` output line verbatim and add
  separate additive lines when values resolve:

  ```text
  recommended-tier: medium
  recommended-model: gpt-5.6-terra
  recommended-reasoning-effort: high
  ```

- Resolve model and effort through the existing harness-aware
  `wsconfig.ResolveAgentForHarnessConfig` seam shared with `RoleModel` and the
  fixed-tier model render variables. Do not hardcode Claude or Codex model names
  in playbook bodies.
- Codex-specialized workflow guidance must tell the lead to pass the rendered
  model as native `spawn_agent.model` and the rendered effort as
  `spawn_agent.reasoning_effort`. It must not use the invalid `effort` spelling.
- If the active native spawn surface rejects a rendered optional parameter, the
  lead must report that exact model binding was unavailable rather than claiming
  success. The mercenary path remains the explicit exact-binding fallback where
  applicable.
- Remove delegate-body `Alias model for this role:` echoes after render output
  becomes the single pre-spawn binding source. A child need not be told which
  model launched it, and its runtime self-description cannot verify the binding.

## Constraints

- Preserve harness-aware resolution and unknown-harness fallback behavior.
- Preserve the prompt-path first line and the existing recommended-tier line so
  current callers remain compatible; new binding lines are additive.
- Empty effort means no override and omits the effort line. A model-resolution
  failure likewise leaves the stable tier guidance available rather than
  emitting an empty model value.
- Shared playbook text stays host-neutral. Literal native parameter names belong
  in Codex-specialized rendered guidance.
- Keep ws and wsflow resource mirrors synchronized wherever the shared shipped
  playbook surface changes.

## Phases

### Phase 1: Render and teach native spawn bindings

- Extend `playbook.render` output with dynamically resolved
  `recommended-model` and optional `recommended-reasoning-effort` lines while
  retaining the existing path and tier lines.
- Add Codex-specialized workflow-manual guidance that maps those lines to the
  native `model` and `reasoning_effort` spawn fields and describes the honest
  rejection fallback.
- Remove redundant delegate-body alias echoes once the binding is present in the
  lead-facing render result.
- Update the MCP/workflow specs on contact.
- Verify default and overridden Codex mappings, Claude harness mapping with empty
  effort, unresolved-value fallback, exact output compatibility, Codex rendered
  guidance, resource manifests, wsflow mirroring, and the full relevant test
  suites.

### Result (be51bf3d) - 2026-07-21

`playbook.render` now preserves its prompt-path and tier contract while adding
ordered, configuration-resolved model and optional reasoning-effort metadata.
`playbook.print` remains tier-only. Codex workflow guidance shows the complete
native spawn shape, uses `model` and `reasoning_effort` with
`fork_turns: "none"` for self-contained rendered prompts, decides exact-binding
fallback before degraded retry, and retains the returned identifier for
`followup_task` continuity. All twelve delegate-body model-alias echoes were
removed; binding metadata is lead-facing only.

Review converged clean across correctness, fit, and test partitions after the
tool description was qualified for playbooks without a declared tier and both
authoritative specs were updated. Fresh-reader audit also converged clean after
clarifying optional-field omission, rejection handling, fallback end states,
and continuity. Verification passed with uncached MCP/resource tests, the
wsflow unittest suite, spec-index validation, and canonical/wsflow byte checks.

No implementation finding remains unresolved. Dogfood follow-ups were captured
as `260721-bug-codex-spawn-schema-routing-field-drift` and
`260721-bug-review-partition-empty-artifact`.

## Spec Impact

- Target spec areas: `ai-docs/spec/mcp-tools.md` playbook render contract and
  `ai-docs/spec/workflow-skills.md` native delegation guidance.
- Expected caller-visible change: `playbook.render` adds resolved native-spawn
  model/effort metadata and Codex guidance names the actual spawn parameters.
- Contract-first spec: no — the additive text shape is fully specified above and
  the authoritative specs will be updated with the implementation.


## Resolution (2026-07-21)

Phase 1 completed: playbook.render now returns harness-resolved native spawn bindings, Codex guidance maps them to the exact spawn parameters with deterministic fallback behavior, and redundant child alias echoes are removed.
