---
title: "Fact-populator regression fixture leaks repository paths into shipped prose"
related:
  260912-bug-ticket-fact-populator-drops-manual-constraints: regression source
---

# Fact-populator regression fixture leaks repository paths into shipped prose

## Background

The cumulative release-readiness run after the related fix failed
`agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` because the
new fact-populator fixture names this repository's `agents-plugin/` and
`agents-plugin-tool/` paths inside a shipped playbook. The targeted Go tests
and independent worker review passed, so the cross-package downstream-neutral
suite was the first check to expose the leak.

## Phases

### Phase 1: Keep the regression fixture downstream-neutral

Preserve coverage that valid authored convention constraints survive fact
population and that constraint mutations are reported, without embedding this
repository's private layout names in shipped prompt text. Verification must
include the targeted fact-populator contract test and the full shipped-surface
downstream-neutral test suite.
