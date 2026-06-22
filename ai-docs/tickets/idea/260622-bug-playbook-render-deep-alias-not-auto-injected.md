---
title: playbook.render fails to auto-inject deep/large model-alias var for reviewer
related-mental-model:
  - mcp-runtime
---

# playbook.render fails to auto-inject deep/large model-alias var for reviewer

## Background

While dispatching the single `reviewer` delegate during a lead-implement run
(260620 Phase 2), `ws/playbook.render(name: "reviewer", session_key: ...)` with
no `context` failed:

```
error: declared variable "DeepModel" appears in body but was not provided
```

The lead-implement / Delegate-dispatch guidance states these delegates "declare
only model-alias vars, which the tool auto-injects; caller-supplied undeclared
keys error." So the documented expectation is that `playbook.render` resolves and
injects model-alias vars automatically and the caller passes no `context`.

Observed config (`config.show`) defines `model_aliases` under the
`small`/`medium`/`large`/`xlarge` keys, not the `light`/`core`/`deep` read-compat
synonyms. The reviewer playbook body references `DeepModel`. The render path did
not map the `deep` synonym to the configured `large` alias, so auto-injection
left `DeepModel` unresolved and the render errored.

Workaround used: explicitly pass `context: {"DeepModel": "claude/opus"}`; render
then succeeded with `recommended-tier: large`. (The partitioned review
delegates `code-review-*` in 260620 Phase 1 rendered fine — only the single
general `reviewer` tripped this, suggesting the var-name set differs per
delegate.)

## Suspected Cause

The model-alias auto-injection resolves variables by the canonical tier names
(`small`/`medium`/`large`/`xlarge`) but the `reviewer` playbook declares a
`Deep`-named var; the `deep ↦ large` read-compat synonym mapping that exists for
config *reads* may not be applied on the *render auto-inject* path.

## Impact

- Blocks the documented "pass no context" Delegate-dispatch idiom for the single
  `reviewer` delegate; the lead must hand-supply an alias value, which is brittle
  (wrong harness/model string, cascade to other missing vars).
- Low severity — there is a working manual workaround — but it contradicts a
  documented contract and could silently push the wrong model string into a
  mercenary dispatch.

## Follow-up

- Confirm which model-alias var names each bundled delegate playbook declares
  (`reviewer` vs `code-review-correctness`/`-fit`/`-test`).
- Decide whether the fix is in the render auto-inject resolver (apply the
  light/core/deep ↦ small/medium/large synonym map) or in normalizing the
  playbooks to canonical tier-named vars.
