---
title: config unset is asymmetric with set — no builtin restore, no session scope
---

# config unset is asymmetric with set — no builtin restore, no session scope

## Context

Found during a v0.31.1 dogfooding pass. Two related asymmetries between
config "set" and "unset" surfaces:

1. `config_workflow_prefer_subagent` is modeled as on/off with no unset. A
   value that originally resolved from the `builtin` default can only be
   restored to an explicit `global:off` — there is no way to return to the
   builtin fallback once an override has been set. The override permanently
   shadows the default even when the caller wants the original fallback
   behavior back.

2. `config_prompt_unset` does not support a `session` scope, even though
   `config_prompt_set` does support setting at `session` scope. A
   session-scoped prompt override therefore cannot be cleared through the
   unset tool at all — the only way to remove it is to detour through
   `global` scope, which risks clobbering an unrelated global-scope value.

Both are reasonable-expectation violations: an unset/reset operation is
expected to be the inverse of whatever set supports, not a narrower subset.

## Suggestion

Make `config_prompt_unset` accept the same scopes as `config_prompt_set`,
including `session`. For `config_workflow_prefer_subagent` (and similarly
shaped on/off workflow preference toggles), add an explicit "reset to
builtin" path distinct from setting the value to `off`, so the builtin
fallback can be restored without guessing its polarity.
