---
title: Pi spawn performs a redundant delegation-metadata write that may fail after allocation
related:
  260913-feat-ws-pi-delegated-write-scopes: introduced durable delegated write authority
---

# Pi spawn performs a redundant delegation-metadata write that may fail after allocation

## Background

Pi displayed the session-bounded warning `ws: could not persist owned-agent metadata; the affected operation was rejected.` immediately after an Explore allocation. Output transport containment worked—the warning did not corrupt stdout—but the authoritative metadata update still failed.

Current spawn flow allocates an owned agent home and writes `ownership.json`, then immediately performs a second authoritative update to add `delegation`. The warning fingerprint intentionally discards the underlying error, so the historical event cannot distinguish transient lock contention, malformed metadata, path-safety rejection, or write/rename failure. Read-only inspection found valid metadata including delegation and no surviving ownership locks afterward; persistent corruption is not established.

## Phases

### Phase 1: Make initial spawn metadata complete and remove the redundant durability boundary

Reproduce or instrument the allocation seam sufficiently to verify whether the immediate post-allocation delegation update caused the warning. If confirmed, pass delegation into `allocateAgentHome()`, include it in the initial ownership object before the first durable write, and remove the immediate `updateOwnership()` call. Allocation must still fail before registry registration or child launch when the complete descriptor cannot be persisted; do not weaken ownership locks, path checks, retention protection, or fail-closed mutation behavior.

Add coverage proving a newly allocated scoped child home contains delegation without a follow-up metadata write, and preserve concise session-bounded owner notification for genuine authoritative failures.
