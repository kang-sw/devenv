---
name: lead-review
description: Independent review of a contributor branch or a commit range. The lead resolves target and config, spawns a review-handler worker that runs reviewers reading the diff from git and stamps the review ledger for ranges, and carries the merge and fix decisions to the user.
---

<ws-preload playbook="lead-review">
# Review

Call `ws/playbook.read(name: "lead-review")` and execute the returned procedure
inline against the user request.
</ws-preload>
