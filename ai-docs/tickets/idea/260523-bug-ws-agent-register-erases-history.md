---
title: Preserve named-agent history across register
related:
  260523-feat-ws-dashboard-main-session-activity-source: Activity freshness gap exposed during dashboard dogfood
  260523-feat-ws-dashboard-activity-console-tail-ribbon-polish: Activity Console transcript inspection makes lost history visible
spec:
  - 260505-async-subquery-ephemeral-agent
related-mental-model:
  - named-agent-runtime
  - ws-web-dashboard
---

# Preserve named-agent history across register

## Background

Dogfood investigation of stale-looking WorkRoot Activity raised whether named
agents had been deleted during implementation. The runtime currently has two
destructive paths:

- `agents.erase` removes the named agent directory for the current worktree.
- `agents.register` removes the existing agent directory before creating the new
  registry record, as long as the current call is not active.

That means repeated registration of stable role names such as `implementer`,
`reviewer-fit`, or `mental-model-updater` can discard earlier output,
transcript, runtime logs, and call state. The Activity Console then has only the
latest per-name row and cannot explain prior work even if those agents were used
heavily earlier in the same branch.

## Follow-Up Questions

- Should `agents.register` update metadata in place instead of deleting the
  whole agent directory?
- Should call executions be stored under immutable execution ids so role rows
  can show current state while transcript history remains inspectable?
- Should `agents.erase` be the only destructive cleanup path for persistent
  named agents, with ephemeral subquery cleanup remaining explicit and scoped?
- How should WorkRoot Activity surface replaced, erased, or superseded agent
  history without exposing private backend paths?
