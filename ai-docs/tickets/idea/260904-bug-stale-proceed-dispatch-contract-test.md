---
title: "Stale assertion fails test_proceed_keeps_implementation_route_only"
related:
  - 260904-refactor-enter-affordance-rename-route-opaque
---

## Background

`agents-plugin/tests/test_skill_dispatch_contracts.py::test_proceed_keeps_implementation_route_only`
fails today, independent of any current work. Surfaced during the layer ① Phase 1
survey (`260904-refactor-enter-affordance-rename-route-opaque`) and reproduced
directly:

```
python3 -m unittest tests.test_skill_dispatch_contracts -v
```

The failing assertion (around line 15) is:

```python
self.assertIn("Route only; do not implement or plan here.", text)
```

That exact sentence no longer exists anywhere in the current
`lead-proceed` playbook body (confirmed by direct read). The assertion is stale
from an earlier revision of the proceed prose; it fails identically before and
after the enter.* → route.resolve_* rename, so it is not a rename regression and
was left untouched during Phase 1 (out of that ticket's scope).

## Open Questions

- Is the intended contract still "proceed must not implement/plan inline"? If so,
  update the assertion to match the current lead-proceed wording that expresses
  that guarantee (e.g. the direct-execution / routing boundary text).
- Or was the guarantee deliberately relaxed (proceed now permits a bounded
  direct-execution early return)? If so, the test intent itself needs revising,
  not just its literal string.

## Notes

- Low blast radius: one Python contract test; the Go suite is unaffected.
- Fix belongs with whoever re-authors the lead-proceed contract text (layer ①
  Phase 2 pen-holds `lead-proceed`/`lead-implement`), so this may be cheapest to
  fold into that authoring pass — but capture it here so it is not lost if Phase 2
  does not touch the assertion.
