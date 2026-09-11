---
title: "Route the lead-delegate executor model through mandatory config.resolve_agent tier resolution"
related:
  260910-feat-lead-run-worktree-parallel-route: sibling; both tighten how the lead dispatches executors — this one governs delegate model routing, that one governs parallel worktree fan-out
---

# Route the lead-delegate executor model through config.resolve_agent tier resolution

## Background

`lead-delegate` is the only lead dispatch path whose executor model is chosen
freehand. Its `## Assignment` section tells the lead to "choose the executor's
prompt, model, tools, and permissions" with no resolution mechanism, so the
model is picked by raw name outside the `config.resolve_agent` fallback chain
(harness bucket -> default -> codex) that `playbook.render` and `agents.tier`
use for every other worker. `lead-run`/`ticket-worker` receive a config-resolved
`recommended-model` from `playbook.render`; delegate does not. The gap means
delegate model choice is neither config-tunable per harness nor consistent with
the rest of the workflow, and it is directly cost-relevant since the model tier
drives spend.

## Direction (decided)

- Tier resolution is **mandatory**, not advisory. The delegate must resolve its
  executor model through `config.resolve_agent(tier)` rather than naming a model
  directly. This is the cost-control chokepoint and the consistency fix.
- The **tier judgment itself stays a one-line qualitative call.** The lead picks
  small / medium / large / xlarge by feel from the assignment's difficulty, with
  no scoring rubric or criteria table. Keep the added prose minimal: a single
  sentence that says "judge the difficulty tier and resolve it via
  `config.resolve_agent`," not a decision procedure.
- **Prompt, tools, and permissions stay freehand.** Only the model-selection
  path is standardized; the delegate's intentional flexibility everywhere else
  is preserved.

## Open questions (for shaping)

- Whether the delegate calls `config.resolve_agent` inline or gains a light
  render step. Inline is likely sufficient, since delegate has no rendered
  worker playbook today.
- Exact placement and wording in `## Assignment` so the one-line tier
  instruction lands without bloating the section.
- Shipped-surface reach: `lead-delegate.md` plus the `agents-plugin-wsflow/`
  mirror and the skill-authoring invariant checklist.
