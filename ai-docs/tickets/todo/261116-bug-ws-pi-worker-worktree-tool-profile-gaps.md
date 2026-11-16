---
title: "Pi worker profile omits tools needed beside worktree acquisition"
related:
  261116-bug-ws-exe-launch-elimination: "Phase 1 item 2 owns tickets.create_empty exposure and executable-fallback elimination; do not duplicate that change"
  261116-feat-ws-subagent-worktree-lease: "owns scoped acquire/release permissions; this ticket covers adjacent worker primitives"
---

# Pi worker profile omits tools needed beside worktree acquisition

## Background

Dogfood #17, 2026-11-16: the Pi worker tool profile lacks
`git.resolve_impl_branch` and `tickets.create_empty` alongside the worktree
workflow it is asked to follow. This worker directly encountered both missing
tools in its exposed inventory; the 10:07 implementation assignment stopped
because `worktree.acquire` and `git.resolve_impl_branch` were unavailable, and
the lead subsequently acquired the worktree. Missing creation access also
caused the earlier executable-CLI fallback captured by the related exe ticket.

That ticket's Phase 1 item 2 already adds `tickets.create_empty` and audits tool
omissions. **Do not reimplement that addition or any exe-launch lifecycle work.**
Scope this ticket to `git.resolve_impl_branch` and the remaining routinely needed
worker primitives, coordinating the audit with that producer and the separate
acquire/release permission ticket.

## Decisions

Compatibility target: **T0**, additive exposure of existing authorized tools,
with current schemas, scope/depth ceilings and key/worktree binding preserved.
This does not authorize exposing every lead-only tool to workers.

## Phases

### Phase 1: Complete the bounded worker worktree-tool inventory

Compare the worker's rendered workflow requirements with its actual profile,
registration and dispatch permissions. Which adjacent primitives are still
missing after the related create_empty/acquire-release changes? Restore the
required supported subset, with explicit justification for each addition.
Verify an authorized worker can resolve an implementation branch and complete
its assigned worktree/ticket flow through native tools, while unrelated lead
operations and privilege escalation remain refused. No executable fallback,
profile implementation or new permissions are introduced by this capture.
