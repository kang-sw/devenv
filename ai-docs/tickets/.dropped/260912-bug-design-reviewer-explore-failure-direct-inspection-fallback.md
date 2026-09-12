---
title: "Design reviewer bypasses its exploration boundary when explorer dispatch fails"
related:
  260912-feat-batch-promotion-design-review: dogfood trigger during its ready-promotion design review
dropped: 2026-09-12
---

# Design reviewer bypasses its exploration boundary when explorer dispatch fails

## Background

During Sage design review of `260912-feat-batch-promotion-design-review`, the
reviewer attempted to delegate codebase exploration, received an agent-thread
limit rejection, and then inspected ticket-cited implementation artifacts
directly. The current design-review contract deliberately prohibits direct
codebase exploration and exposes Explore as the reviewer-controlled evidence
path. An unavailable explorer must not silently expand the reviewer's own
authority.

The lead rejected that verdict, instructed the reviewer to discard the direct
inspection, and retried after a concurrency slot became available. The retry
successfully dispatched an explorer.

## Phases

### Phase 1: Make explorer-dispatch failure preserve the reviewer boundary

Reproduce the failed-dispatch path and align the reviewer procedure or harness
handling so it reports the evidence limitation or retries within its authority
instead of substituting direct codebase inspection. Preserve reviewer autonomy
over whether exploration is useful when exploration is available.


## Resolution (2026-09-12)

Dropped after the owner confirmed that direct inspection is an acceptable fallback when Explore is unavailable. The observed dispatch failure was transient harness thread-capacity contention, and the retry succeeded once capacity was available; no workflow defect remains.
