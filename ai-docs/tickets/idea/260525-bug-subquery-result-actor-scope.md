---
title: subquery result is not retrievable in actor scope after explicit-root start
related:
  260524-bug-subquery-working-directory-stderr: adjacent subquery reliability issue
  260525-bug-ws-setup-cwd-plugin-cache-root: adjacent setup/root recovery behavior
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# subquery result is not retrievable in actor scope after explicit-root start

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

## Investigation - 2026-05-25

The failure is a namespace mismatch, not a worker execution failure. The
explicit-root subqueries completed successfully and were visible through
explicit-root `agents.status(root: "/Users/kang-sw/devenv", name: ...)`.
The same names failed through root-omitted `agents.result(name: ...)` because
that call resolved through the current lead actor scope.

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
explicit-root `subquery`, then the returned follow-up command. A fix should
either make the subquery follow-up executable as printed, or make explicit-root
subquery return explicit-root recovery guidance instead of rootless
`agents.*` calls.
