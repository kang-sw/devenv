---
title: "Reconcile mental_models discovery surface with the canonical query verb"
related:
  - 260903-refactor-mcp-read-surface-collapse
  - 260903-epic-mcp-tool-surface-affordance-reduction
---

## Background

Layer ③ (`260903-refactor-mcp-read-surface-collapse`) audited the
`mental_models` discovery triple against the `tickets`/`specs` collapse pattern
and found it is **not a clean superset**:

- `mental_models.list` is a divergent legacy implementation with its own struct
  and formatter (no JSON path through `MentalModelsList`), unlike the
  `tickets`/`specs` scanners.
- `mental_models.find` lacks the `path` argument that `status` carries; the two
  overlap only on `domain`, so `find` cannot absorb `status` without behavior
  loss.

Because of that, ③ only verb-aligns `mental_models` (rename `find` → `query`,
noted exception) and explicitly **defers the full read-surface collapse** to
this ticket rather than forcing an unsafe merge.

## Open Questions

- Can `mental_models.list` be re-based onto the shared scanner/formatter used by
  `tickets`/`specs` so it gains a JSON path, or does its divergent shape encode
  a real requirement?
- Should `query` gain a `path` argument (superset of today's `status`), or should
  `status`-style lookup stay a distinct affordance for mental models?
- After ③ lands, what is the residual surface (`query` + `status` + a
  legacy-shaped `list`) and which members can be collapsed without behavior loss?

## Notes

- Sequencing: this is post-③. It consumes ④'s survivor verb naming and ③'s
  verb-alignment as its starting point.
- Scope guard: reconciliation only — do not re-open the `tickets`/`specs`
  collapse decisions already settled in ③.
