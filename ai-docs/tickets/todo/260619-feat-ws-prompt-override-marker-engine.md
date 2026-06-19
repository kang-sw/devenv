---
title: Block-marker prompt-override engine + DelegationSection seed
parent: 260619-epic-ws-layered-config-prompt-tuning
related:
  260619-feat-ws-layered-config-scope-substrate: prerequisite — override values resolve through the layered config storage/scope primitive
related-mental-model:
  - prompt-bundle
---

# Block-marker prompt-override engine + DelegationSection seed

## Background

Users need to override or extend named sections of a rendered playbook body
without editing shipped rsrc text (epic
`260619-epic-ws-layered-config-prompt-tuning`). Today there is no such
mechanism: delegation guidance is partly hardcoded appends
(`playbook_tools.go` delegation tip, mercenary guidance block) and partly
scattered prose in `lead-workflow-manual`. The motivating use case is lead
"context-saving" delegation tuning.

## Decisions

- **Mechanism = block markers (A1 grammar), not template-variable placeholders.**
  Marker grammar (single-line open marker, own line, line-scan friendly):
  ```
  <!-- ws:override:DelegationSection desc="short one-liner" -->
  <seed default-prompt text — this is the inline default>
  <!-- ws:/override:DelegationSection -->
  ```
  - The block body between open/close IS the seed default-prompt; no separate
    default field.
  - An empty body is a pure extension slot (e.g. `WorkflowManualExt`), so
    override and extend are the same primitive.
  - `desc` is a SHORT one-liner; keep it terse.
- **The `{{.DelegationSection}}` template-variable placeholder is NOT used.** The
  `DelegationSection` identifier lives only as the marker `<id>` and the
  `config.prompt.set` `pointId` key. Rationale: `{{.Var}}` placeholders fail loud
  when unprovided (`substituteVars`) and would force the seed default into
  multiline YAML frontmatter; markers keep the seed inline and PR-readable.
- **Resolution for an override-point:** user override (harness match) → user
  override (`*`/all) → inline seed default. The `(pointId, harness)` axis selects
  what; scope (from the layered config) selects where stored.
- **Additive parser.** Add a sibling pass beside `selectProductModeBlocks`
  (`playbook_tools.go`), not a rewrite. Current parser matches whole-line exact
  marker constants; this generalizes to prefix-match + parse of id and the `desc`
  attribute, still line-oriented (markers on their own line). The override pass
  runs in the render layer.
- The marker grammar is a fully-custom ws schema (ws is sole reader), so the
  grammar choice is unconstrained by external standards.

## Phases

### Phase 1: Override marker grammar + render pass

Implement the A1 marker grammar and the sibling render pass: parse override
blocks, resolve `(pointId, harness)` against the layered config
(`260619-feat-ws-layered-config-scope-substrate`), substitute the override body
or fall back to the inline seed, and strip markers. Empty-seed extension slots
render the override or nothing.

Depends on `260619-feat-ws-layered-config-scope-substrate` Phase 1 (override
storage/resolution).

Verification: a point with no override renders its seed (markers stripped); a
per-harness override replaces the body for the matching harness only; a `*`
override applies to all; an empty-seed slot renders override-or-nothing.

### Phase 2: DelegationSection seed + workflow-manual consolidation

Seed `DelegationSection` in `lead-workflow-manual` and consolidate the scattered
delegation-posture prose into that named section.

**Override boundary (confirmed):**
- Overridable (DelegationSection seed): the delegation *posture/preference* prose
  — when/how eagerly to delegate, the context-saving stance.
- Stays hardcoded (NOT overridable): the agentId continuity tip (harness-aware,
  delegation-continuity correctness), the child-key credential splice
  (security/correctness), and the `prefer_mercenary` guidance block (its own
  session-scope item). These are critical-path mechanics, not user style.

Depends on Phase 1.

Verification: with no override the manual renders the current delegation posture;
a user override changes only the posture section, leaving the continuity tip and
credential splice intact.
