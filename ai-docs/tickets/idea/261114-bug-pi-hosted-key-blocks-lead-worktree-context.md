---
title: Hosted Pi session key prevents lead context setup in an acquired worktree
---

# Hosted Pi session key prevents lead context setup in an acquired worktree

## Background

During owner-approved parallel worktree dogfooding, `worktree.acquire` created
`feat/unified-history-task3` with the full profile and returned its worker key.
The primary lead then called `ferrule(root: <acquired worktree>)` to establish
a lead context for ticket review, promotion and other lead-owned operations.
The call refused: `session bootstrap: this session is hosted and its session
key is fixed, but WS_HOSTED_SESSION_KEY "boxer-nest-joyous" is bound to another root`.

The primary key is correctly bound to its original worktree. The surprise is
that the documented acquired-worktree flow offers no evident way to obtain a
lead-authorized context for a different root while the hosted key is fixed.
Root-aware playbook rendering still permits fact population in the acquired
worktree; the later lead-only stamping and promotion path needs investigation.
This observation does not prove that every supported cross-worktree path fails.

## Constraints

- Preserve hosted session identity, root binding and capability boundaries.
- Do not work around the refusal by borrowing foreign keys or mutating binding
  records manually.
- Investigate the supported caller contract before selecting a fix or changing
  API/protocol semantics. No compatibility tier or implementation is settled.

## Phases

### Phase 1: Reproduce and define the supported cross-worktree lead path

Trace hosted key filling, ferrule root checks, acquired worker-key capabilities,
and lead-only ticket operations. Verify whether an existing supported route
covers lead-owned review and promotion in another worktree. If one exists,
correct its discoverability; otherwise return a minimal proposal for approval.
Include regression coverage for original-root identity preservation and correct
cross-root authority, without silently rebinding the primary session.
