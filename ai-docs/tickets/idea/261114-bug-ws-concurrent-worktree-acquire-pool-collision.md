---
title: "Concurrent worktree acquire calls collide on one idle pooled checkout"
---

# Concurrent worktree acquire calls collide on one idle pooled checkout

## Background

During EXEC1 dogfooding on 2026-11-14, two concurrent housekeeping
`worktree.acquire` calls requested different existing branches:
`docs/fullscreen-panel-width-ticket` and `docs/session-hire-research-capture`.
Both selected the idle `agent-lip-alto` pooled checkout. The fullscreen call
failed creating `.git/worktrees/agent-lip-alto/index.lock`; the session-hire
call succeeded and leased that checkout. No lock was manually removed and no
foreign checkout was released.

The failed result also suggested that the branch was held elsewhere, which
was not established by this index-lock failure. Root cause and any supported
concurrency guarantee remain unverified. Subsequent acquisitions are serialized
as a caller-side workaround.

## Constraints

This is a mandatory repository dogfood-surprise capture, not authorization for
EXEC1 to investigate or implement new workflow work. MAIN owns follow-up under
owner policy 771. Do not remove Git locks, steal leases, or alter other tracks.

## Phases

### Phase 1: Investigate and bound concurrent pool acquisition

Establish whether candidate selection and reservation permit concurrent calls
to choose the same idle checkout. Define expected admission behavior, preserve
lease safety, and cover the observed different-branch collision in a bounded
regression before choosing a correction. Distinguish an index-lock collision
from an actually occupied branch in diagnostics. No implementation or
verification result is recorded by this capture.
