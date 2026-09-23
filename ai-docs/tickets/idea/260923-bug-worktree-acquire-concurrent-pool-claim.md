---
title: Concurrent worktree acquisitions race for the same idle pool entry
---

# Concurrent worktree acquisitions race for the same idle pool entry

## Background

During a three-ticket parallel `lead-run` batch on 2026-09-23, three simultaneous `ws/worktree.acquire` calls for distinct `impl/develop/*` branches attempted to reuse the same pooled worktree (`registrar-sphinx-duckbill`). One succeeded; two failed at `git switch -c` because that worktree's `index.lock` already existed. Retrying the failed acquisitions sequentially succeeded, selecting other idle entries. This makes approved parallel dispatch require manual retry before workers can start.

## Open Questions

- How should concurrent acquisition claim distinct pool entries or otherwise serialize pool reuse without surfacing transient Git lock failures?
- Which concurrency regression test best reproduces the contested idle-entry selection and verifies each returned worktree owns its requested branch?

## Phases

### Phase 1: Make parallel pool acquisition reliable

Investigate the pool claim race and settle its synchronization and verification contract before promoting this idea to an actionable ticket. Preserve existing branch ownership and pool hygiene safety checks.
