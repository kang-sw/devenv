---
title: Clarify resumed Pi worker authority after moving worktrees
related:
  261114-bug-pi-hosted-key-blocks-lead-worktree-context: related cross-worktree session authority
---

# Clarify resumed Pi worker authority after moving worktrees

## Background

During urgent Viewed performance diagnosis on 2026-11-15, the lead released the
original worker checkout, acquired a new full worktree, rendered a new worker
prompt there, and sent both new keys to the same persistent worker. The worker
reported that both keys were rejected as outside its authority; profiling and
edits had not started. The lead spawned a replacement with the new prompt and
`cwd_override` instead. The replacement obtained its new binding, then encountered
a separate canonical-branch collision. After the lead preserved the old head in
an archive branch and restored the canonical branch name, that replacement
completed the performance repair.

The denial may be the intended authority boundary, not a security defect. The
surprise is that continuation accepts a new task but cannot adopt the newly
rendered worktree binding; the supported continuation/replacement path is unclear.

## Phases

### Phase 1: Define and test the cross-worktree continuation path

Inspect the session-key authority and persistent-worker binding contracts. Either
support an explicit lead-authorized rebind or document and surface replacement as
the required path before execution starts. Preserve isolation: sending an
arbitrary key must not by itself expand a worker's authority. Verify with a
released original worktree and newly acquired checkout. No runtime behavior or
compatibility tier is selected by this idea capture.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - Possibly an intended permission boundary, and spawning a replacement in a new worktree works around it, so not a concrete defect.
