---
name: lead-review
description: Independent review of a contributor branch or a commit range. The lead resolves target and config, spawns reviewers that read the diff from git, decides the verdict, stamps the review ledger for ranges, and carries merge approval to the user.
---

<ws-preload playbook="lead-review">
# Review

Call `wsflow/playbook.read(name: "lead-review")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
</ws-preload>
