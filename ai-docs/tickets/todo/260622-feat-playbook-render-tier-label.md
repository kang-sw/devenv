---
id: 260622-feat-playbook-render-tier-label
status: todo
type: feat
area: plugin-runtime
---

# feat: playbook render tier label with model alias

## Problem

`playbook.render` currently outputs `recommended-tier: medium` as a bare tier
name. Callers must cross-reference an external mapping to know which model it
resolves to, and the implementer playbook body redundantly restates it as
`Alias model for this role: sonnet` — two representations of the same fact.

## Proposed Change

1. **Render output format**: change `recommended-tier: medium` to
   `recommended-tier: medium (=sonnet)` — inject the current tier→model alias
   inline at render time so the output is self-documenting.
   The `(=sonnet)` portion must be computed dynamically from the tier mapping
   (not hardcoded per playbook) so it stays accurate as models evolve.

2. **Remove redundant alias line**: drop `Alias model for this role: sonnet`
   from the implementer playbook body (and any other playbook that echoes the
   same information) once the render header carries it.

3. **Document tier mapping**: add a tier→model mapping table to
   `ai-docs/WORKFLOW.md` or `ws/convention.read` so callers know the vocabulary
   when requesting a tier override (e.g. "run this at large instead").

## Acceptance

- `playbook.render` output includes `recommended-tier: <tier> (=<model-alias>)`.
- Tier alias is resolved at render time, not hardcoded in the playbook source.
- Redundant `Alias model for this role:` line removed from affected playbooks.
- Tier→model mapping documented in one canonical location.
