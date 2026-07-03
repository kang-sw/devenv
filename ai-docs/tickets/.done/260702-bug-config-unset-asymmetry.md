---
title: config unset is asymmetric with set — no builtin restore, no session scope
sage-review: completed
completed: 2026-07-02
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

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: `config_prompt_unset`
gains a `session` scope; `unset` semantics are redefined to mean
reset-to-builtin (not clear-to-empty) across `config_prompt_unset` and
`config_workflow_prefer_subagent`. Contract-first spec: no.

## Result (21408323)

Both asymmetries fixed:

1. `wsconfig.Resolver.Unset` no longer hard-rejects `ScopeSession`. A new
   `SessionWriter.DeleteOverride(sessionKey, itemKey)` method removes the
   session-store override entry (rather than writing an empty-string value),
   implemented by `sessionStore.deleteOverride` and wired through
   `sessionConfigAdapter`. `config.prompt.unset`'s MCP schema and description
   now advertise `scope: "session"` as a first-class option (dropping the old
   "Session scope is not supported" text); the handler already threaded
   `session_key` through to the resolver unconditionally, so no dispatch-path
   change was needed beyond the schema/doc update.
2. `config.workflow_prefer_subagent` gained an optional `reset: true` argument,
   mutually exclusive with `value`. `reset: true` calls `resolver.Unset` on the
   global-only `workflow.prefer_subagent` item, deleting the global override so
   resolution falls back to `global > builtin` — distinct from writing the
   builtin's current value (`off`) via `value: "off"`, which would keep
   shadowing a future change to the builtin default. The `config.tuning`
   catalog's `workflow.prefer_subagent` knob now also carries a `Reset` writer
   entry (`{"reset": "true"}`), matching the existing pattern already used for
   `prompt.*` knobs' `config.prompt.unset` reset writer.

Spec updated: `ai-docs/spec/mcp-tools.md` `#260702-unset-means-reset-to-builtin`
(prompt unset session scope + reset-to-builtin doc) and
`#260702-config-unset-reset-to-builtin` (`config.workflow_prefer_subagent`
`reset` argument).

Verification:
- New tests: `wsconfig.TestUnsetSessionScopeRestoresNextBroaderScope`,
  `wsconfig.TestUnsetSessionScopeRequiresSessionKey`,
  `wsconfig.TestGlobalOnlyItemUnsetResetsToBuiltin`,
  `mcp.TestConfigPromptUnsetSessionScope`,
  `mcp.TestWorkflowPreferSubagentResetRestoresBuiltin`.
- `cd agents-plugin-tool && go build ./...` — clean.
- `cd agents-plugin-tool && go test ./...` — all packages pass, including the
  five new tests and the full pre-existing suite (no regressions).
