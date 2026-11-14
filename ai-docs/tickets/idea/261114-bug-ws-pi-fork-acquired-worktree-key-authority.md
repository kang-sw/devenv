---
title: Investigate Pi fork authority for an acquired worktree key
---

# Investigate Pi fork authority for an acquired worktree key

## Background

During capture of owner decisions 660–665, a Pi lead-like fork acquired a full
worktree and received a worktree-bound worker key. Four `tickets.create_empty`
calls using that returned key were refused by the Pi bridge with
`ws-pi-agent: session key is outside this agent's authority`.

The root lead subsequently used the same returned key successfully for
`git.status` and all four ticket creations in the acquired worktree. No primary
worktree write, authority-check bypass, harness repair, or lifecycle change was
needed. Root-mediated mutation unblocked the capture, but an acquire result
that its caller cannot use was unexpected.

Evidence: root session `specks-snowbound-framing`; fork
`0604ec22-0771-4950-b0d8-57923a22d268`; fork key
`handlebar-stuffed-motor`; acquired key `lasso-judicial-awning`; branch
`docs/settled-common-contracts-660-665`. The fork's intermediate reports and
root tool results preserve the denial and successful root-mediated operations.
These are historical identifiers, not credentials or reusable authority.

## Constraints

- This is a captured investigation, not approval to relax session authority.
- Preserve primary-worktree isolation and existing scope checks.
- Do not conflate this bridge rejection with the separate executor-session-home
  incident. No common cause has been established.
- No compatibility tier is settled: identify any cross-version surface and
  obtain the required approval before choosing a change.

## Phases

### Phase 1: Reconcile acquisition and bridge authority contracts

Trace how a lead-like fork acquires a worktree, how the returned key is admitted
by the Pi bridge, and which caller is intended to own subsequent operations.
Determine whether the defect is missing supported key registration, an
incorrect advertised workflow, or another cause. Propose the smallest supported
correction with regression coverage for root, fork, ordinary child, and rejected
unrelated keys. Do not select an authority-expanding mechanism without explicit
approval. Root-mediated operations are the observed workaround, not a settled
permanent design.
