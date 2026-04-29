---
title: "New Skill: /rebuild-spec — Reconstruct Spec for Downstream Projects"
related:
  260420-feat-spec-driven-workflow: prerequisite
---

# New Skill: /rebuild-spec — Reconstruct Spec for Downstream Projects

## Background

Downstream projects may have no spec at all, or specs that predate the spec-stem convention and have drifted from the actual codebase. `/bootstrap` handles project scaffolding but does not produce spec content. A heavier skill is needed that reconstructs the full spec from scratch by surveying code, ticket history, and user intent.

Positioned alongside `/bootstrap` in the maintenance skill category. Unlike bootstrap, this skill is expected to take significant time and requires active user collaboration throughout.

## Concept

**Mode: disable-model-invocation (lead as coordinator only)**
The main agent does not read source directly. All codebase exploration is delegated to Sonnet explore subagents. The lead synthesizes findings, detects conflicts, and drives the user interaction loop.

**Work breakdown (sketch):**

1. **Codebase survey** — dispatch Sonnet explore agents per domain/module to infer caller-visible behavior from code. Each agent returns a behavioral brief: what the component does from a caller's perspective.

2. **Ticket history sweep** — scan `ai-docs/tickets/` (all statuses including `done/` and `wip/`) to extract planned and intended behaviors. Done tickets reveal what was built; wip/todo tickets reveal what is planned. These feed into 🚧 candidates.

3. **Existing spec audit** — if any spec files exist, compare against codebase survey findings. Flag drift (spec describes behavior not in code) and gaps (code behavior not in spec).

4. **User interaction loop** — surface ambiguous cases: behaviors found in code but not in any ticket, drift between spec and code, conflicts between ticket intent and current implementation. User decides authoritative intent for each.

5. **Spec authoring** — write spec files per the spec-stem convention. Implemented features get `{#slug}` anchors; planned features (from wip/todo tickets) get 🚧 markers. Run `ws-spec-build-index` on each file.

**Likely very heavy:** expect multiple rounds of explore agents and multiple user-interaction checkpoints. Not a one-shot operation.

## Open Questions

- How to scope the initial survey? Full repo at once vs. domain-by-domain with user guiding focus areas.
- How to handle behaviors that are clearly internal (should not appear in spec) vs. caller-visible — heuristic from explore agents, or user judgment per case?
- Minimum viable output: full spec from scratch, or incremental (one domain at a time)?
