---
title: ws sage_record (and other git-writing ws tools) race on .git index.lock when invoked concurrently
related-mental-model:
  - workflow-routing
---

# ws sage_record (and other git-writing ws tools) race on .git index.lock when invoked concurrently

## Background

Captured during dogfooding while promoting two tickets in one workflow step. A
lead invoked two `ws/tickets.sage_record` calls in the same tool batch (one per
ticket). Each sage_record internally stages and commits (`git add` + commit of
the sage posture). Running them concurrently made the second call fail.

## Symptom

The second concurrent `sage_record` returned:

```
git add -A -- <ticket>.md: exit status 128: fatal: Unable to create
'.git/worktrees/<wt>/index.lock': File exists.
Another git process seems to be running in this repository...
```

The first call succeeded and committed; the second aborted cleanly (no
corruption). Re-running the failed call serially succeeded immediately.

## Impact

Low severity - fails safe, clear error, trivially recoverable by serializing.
But it is a sharp edge for max-delegation / batched-tool-call lead workflows,
where issuing independent ws git-writing calls in one batch is natural. Any two
git-index-writing ws tools (sage_record, git_commit, tickets.move-then-commit,
etc.) invoked concurrently against the same worktree can collide.

## Suspected Fix Direction (suggestion, not mandate)

- Add bounded retry-with-backoff on `index.lock` contention inside ws git-writing
  helpers, or
- Serialize git-index mutations behind an internal per-worktree lock, so
  concurrent ws git calls queue instead of erroring, or
- At minimum, document that ws git-writing tools must not be batched concurrently
  and have callers serialize them.

## Reporter Context

Surfaced while promoting 260722-refactor-dashboard-app-tsx-leaf-extraction and
260722-refactor-dashboard-app-tsx-state-decomposition (parallel sage_record for
combined + design stages). Workaround applied: retried the failed call serially.
