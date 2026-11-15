---
title: "ws/git.commit cannot conclude a conflicted merge; native fallback strips ## AI Context"
---

# ws/git.commit cannot conclude a conflicted merge; native fallback strips ## AI Context

## Background

Dogfood surprise relayed by MAIN on 2026-11-15. EXEC2 hit it while merging
develop into TASK4 (checkpoint `e4d2f4358`). This is a static capture and no
run was made for it.

1. `ws/git.commit` refused to commit a conflict resolution. Its explicit
   path grant covered only the resolved file (`store.go`), while the merge's
   incoming paths were already staged. A conflicted merge therefore cannot be
   concluded through `ws/git.commit` with a narrow grant.
2. The fallback, native `git merge --continue`, used the default
   `commit.cleanup`. Git treats lines starting with `#` as comments, so the
   cleanup stripped the `## AI Context` heading the repository's commit
   format requires.

## Open Questions

- Should `git.commit` support concluding an in-progress merge (`MERGE_HEAD`
  present) with a grant that knows the incoming paths?
- Should the workflow recommend `--cleanup=verbatim`, or
  `-c core.commentChar=<char>`, for native merge commits that carry
  `## AI Context`?
