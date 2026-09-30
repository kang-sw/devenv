---
title: TestCloseLeaseAndPruneOnLanding flakes on TempDir cleanup of origin.git/objects
related:
  260925-bug-windows-remote-timeout-leaves-child-processes: possibly the same leaked git child writing after its parent returns
---

# TestCloseLeaseAndPruneOnLanding flakes on TempDir cleanup of origin.git/objects

## Background

ws-mcp CI run 36692824801 (commit d9982cd38, ubuntu `all` leg) failed only in
`internal/mcp`:

```text
--- FAIL: TestCloseLeaseAndPruneOnLanding (0.55s)
    testing.go:1232: TempDir RemoveAll cleanup: unlinkat
    /tmp/TestCloseLeaseAndPruneOnLanding329414963/003/origin.git/objects:
    directory not empty
```

A re-run of the failed job passed. The test itself asserted nothing wrong; the
failure is `t.TempDir` cleanup finding new entries under the bare origin's
`objects/` while it removes the tree. Something was still writing into the
origin after the test body returned.

It had been hidden before: earlier CI runs either were the first to run the
test on that runner or, after the Go cache was fixed, replayed a cached pass.
Since d9982cd38 CI shards pass `-count=1`, so every run exercises it.

## Open questions

- Which process writes to `origin.git/objects` after the test returns? Suspects:
  - a `git push` by the ws index path (index ref or pending flush) whose
    local-transport `git-receive-pack` child outlives a cancelled or
    timed-out parent;
  - receive-side auto-maintenance detaching (`receive.autogc`,
    `gc.autoDetach`) after the test's own `git push origin develop`.
- Is the fix in the product (reap or wait for every git child) or in the test
  harness (disable auto gc/maintenance for the fixture repos, or wait for
  quiescence before cleanup)? Only a product-side leak matters to users.
