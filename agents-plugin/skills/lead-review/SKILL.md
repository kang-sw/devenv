---
name: lead-review
description: Independent review of a contributor branch or a commit range. The lead resolves target and config, spawns reviewers that read the diff from git, decides the verdict, stamps the review ledger for ranges, and carries merge approval to the user.
---

# Review

Call `ws/playbook.read(name: "lead-review")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
