---
title: subquery still advertises root and returns mismatched actor-scope follow-up
related:
  260524-bug-subquery-working-directory-stderr: adjacent subquery reliability issue
  260525-bug-ws-setup-cwd-plugin-cache-root: adjacent setup/root recovery behavior
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# subquery still advertises root and returns mismatched actor-scope follow-up

## Background

During dashboard route-forwarding difficulty analysis on 2026-05-25,
`ws/subquery(root: "/Users/kang-sw/devenv", deep_research: true, ...)`
returned a running `subquery_key` and `agent_name`, but the documented
follow-up `ws/agents.result(name: "<subquery-key>")` failed with:

```text
agent "<subquery-key>" is not registered for actor scope
```

Repeating the same subquery after `ws/setup(id: "lead-turji06c")` produced the
same result. A named agent registered with an explicit `root` also returned a
name that was not callable in the current actor scope, while registering the
same named agent without `root` after setup worked.

The expected behavior is that a returned subquery key is immediately
retrievable through the documented result/status/tail follow-up tools in the
same lead session. If explicit `root` changes actor scope or registration
authority, the subquery tool should either bind the returned key to the
caller-visible actor scope or return recovery guidance that works.

Follow-up investigation corrected the premise: `root` should not be publicly
advertised on `subquery` at all. The `agents.*` root schema cleanup removed
public `root` from named-agent lifecycle tools but missed `subquery`, even
though subquery is actor-owned and should follow the same rootless public
workflow.

## Investigation - 2026-05-25

The failure is a namespace mismatch caused by a stale public schema, not a
worker execution failure. The explicit-root subqueries completed successfully
and were visible through explicit-root
`agents.status(root: "/Users/kang-sw/devenv", name: ...)`. The same names
failed through root-omitted `agents.result(name: ...)` because that call
resolved through the current lead actor scope.

The relevant code path is:

- `internal/mcp/server.go` calls `actorScopeForAgentTool(root, arguments)` for
  `subquery`.
- `actorScopeForAgentTool` returns an empty actor id when the caller supplied
  an explicit non-empty hidden `root` argument.
- `wsagent.Manager.Subquery` therefore registers and calls the generated
  `subquery-*` agent in the unbound global compatibility namespace.
- `wsagent.Manager.Subquery` always returns rootless follow-up text:
  `agents.result(name: "<subquery-key>", timeout_seconds: 600) | ...`.
- In an actor-bound lead session, rootless `agents.result/status/tail/cancel`
  intentionally resolve through the actor namespace, so they cannot see the
  explicit-root global subquery.

There is a secondary inconsistency: `childActorSetupForSubquery` checks only
`actorBoundToRoot(root)`, not whether the caller supplied explicit `root`.
That can create and inject a reader child actor for a subquery that is otherwise
registered globally because `actorScopeForAgentTool` returned an empty actor id.

Existing tests cover the pieces separately:

- `TestServeStdioActorScopedAgentLifecycleAndExplicitRootCompatibility` proves
  root-omitted agent lifecycle tools use actor scope while explicit-root calls
  use the global namespace.
- `TestActorScopedSubqueryRegistersAndCallsSameScope` proves direct
  actor-scoped `wsagent.Manager.Subquery` registration and call use the same
  actor id.

Missing coverage is the MCP-level integration case: actor-bound session,
explicit-root `subquery`, then the returned follow-up command. More importantly,
raw and `tools/list` schema coverage must assert that `subquery` does not
advertise `root`, just as public `agents.*` schemas do not advertise `root`.

The historical gap appears to be `40f32164 fix(mcp): hide agent root schemas`:
it removed `root` from `agents.*` schemas while preserving dispatch-time hidden
explicit-root compatibility, but it did not remove `root` from the top-level
`subquery` schema. The added tests only checked `strings.HasPrefix(name,
"agents.")`, so they never failed on `subquery`.

A fix should remove `root` from the public `subquery` schema, keep or explicitly
reject hidden explicit-root compatibility according to the intended contract,
and align child-reader actor setup with the same decision. If hidden
explicit-root compatibility remains accepted, returned follow-up text must not
be rootless in a way that routes to a different namespace than the generated
subquery agent.

## Skill Audit - 2026-05-25

Current skill text does not instruct callers to use `root` on `subquery` or
`agents.*` tools. The only `root` mentions in shipped ws skills are:

- `agents-plugin/skills/lead-workflow-manual/SKILL.md`: general notation says
  to omit `root` when the current repository root is intended, and session setup
  uses `ws/setup(method: "lead-workflow-bootstrap", root:
  "<absolute-working-directory>")`.
- `agents-plugin-wsflow/skills/lead-workflow-manual/SKILL.md`: general notation
  says to omit `root` when the current repository root is intended, and setup
  uses `wsflow/setup(root: "<absolute-working-directory>")`.
- The installed cache copy of `agents-plugin/skills/lead-workflow-manual` has
  the same ws text.

No other `SKILL.md` under `agents-plugin/skills`,
`agents-plugin-wsflow/skills`, or the installed ws plugin cache contains an
MCP-call pattern that passes `root` to `subquery`, `agents.*`, `api.*`, or
`config.*`.

The remaining stale exposure is therefore in the MCP advertised schema and
dispatch compatibility path, not in ordinary skill prose. The general notation
line may still be worth tightening so workflow authors do not infer that
actor-owned tools should accept public `root` arguments.
