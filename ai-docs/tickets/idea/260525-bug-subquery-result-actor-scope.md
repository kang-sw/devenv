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
