---
title: "Parallel worktree acquisition can reuse the same pooled entry"
---

# Parallel worktree acquisition can reuse the same pooled entry

## Background

During one `multi_tool_use.parallel` batch, two independent `ws/worktree.acquire` calls used profile `full`: one requested `base fix/windows-go` with target branch `fix/windows-go-resumed`, and the other requested `base fix/windows-launcher` with target branch `fix/windows-launcher-resumed`. Both selected the pooled worktree `/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`. The Go call failed while creating `.git/worktrees/foe-breath-retying/index.lock` because the file already existed; the launcher call succeeded with worker key `payback-buffing-implosion`. No worker was active in that slot and no implementation changes were lost. A later sequential retry of the Go call succeeded at `gauze-zookeeper-poise` with worker key `clench-unpledged-gizzard`.

Concurrent independent acquisitions should claim distinct, exclusively reserved pool entries so each returned worktree is safe for its requested branch. Until this is investigated, provisioning calls can be serialized while workers run in parallel after their worktrees are acquired.

## Open Questions

- What evidence identifies the pool-selection and reservation race without assuming a source-level root cause?
- What regression verification proves that concurrent independent acquisitions receive unique, exclusively reserved entries and requested branches, while preserving the existing retry and hygiene safety checks?
