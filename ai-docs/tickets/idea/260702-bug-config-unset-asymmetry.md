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

Define `unset` consistently, across every scope and every config surface, to
mean "reset to builtin default" — never "clear to empty." An explicit
empty-string override is a distinct intent from falling back to builtin, and
that intent already has a home: `config_prompt_set` with an empty-string
value. `unset` should not be repurposed to also cover that case.

Concretely:
- Add `session` scope to `config_prompt_unset`, with `unset` at that scope
  meaning "drop the session-scoped override and fall back to whatever the
  next-broader scope (or builtin) resolves to" — not "set it to empty."
- For `config_workflow_prefer_subagent` (and similarly shaped on/off workflow
  preference toggles), add an explicit "reset to builtin" unset path distinct
  from setting the value to `off`, using the same reset-to-builtin semantic.

Document this set-vs-unset distinction (explicit value vs. reset-to-default)
wherever config surfaces are described, so future config additions follow the
same convention by default.
