---
title: Investigate Pi fork authority for acquired worktree session keys
---

# Investigate Pi fork authority for acquired worktree session keys

## Background

During approved docs-only ticket capture, a Pi fork successfully acquired a housekeeping worktree but reported that the Pi bridge refused the returned key on the next ticket-creation call. Investigate whether the fork's advertised worktree provisioning capability and its bridge authorization agree. This capture does not authorize an authority expansion or claim a security defect.

## Outcome Ledger

### Verified Findings

- The fork `48a00fc7-855a-4b33-af83-1639e2d2887d` reported acquiring a housekeeping worktree through its own session key, then calling `tickets.create_empty` with the acquired worker key. The bridge refused that call with `session key is outside this agent&apos;s authority`. This is a reported tool result, not an independent audit of the bridge implementation.
- The fork reported that no ticket file was created and released the clean lease rather than retrying or writing the shared root. The parent instructed it not to bypass the required creation primitive.
- The parent subsequently acquired a root-owned housekeeping lease and successfully called `tickets.create_empty` with that returned worker key to create this capture. Thus a root-owned route is available; it does not establish which fork capability is intended.
- Evidence is in the parent session `useable-curvature-immovably`, the fork's intermediate blocker report, and the root's subsequent `worktree.acquire` and `tickets.create_empty` results on 2026-11-15. Machine-local paths and ephemeral keys are not reproduction requirements.

### Confirmed Decisions

None. Existing authority and workflow semantics remain unchanged.

### Proposals

None selected. Inspect the fork key lineage, acquired worker-key ownership, and bridge authorization contract before choosing a change.

### Open Questions

- Is a fork intended to use worker keys returned by its own successful worktree acquisition for subsequent ws tools?
- Does the refusal arise from key lineage recording, bridge authorization, or a documented capability boundary?
- If the boundary is intentional, should unsupported provisioning be refused earlier or delegation guidance describe a root-owned handoff instead?

### Rejected Alternatives

No product alternatives were settled. Bypassing the creation primitive, modifying the shared root, and blind retries were not used as recovery actions.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - A single case of a Pi fork's acquired worktree key being rejected; likely a permission boundary and the action is unclear.
