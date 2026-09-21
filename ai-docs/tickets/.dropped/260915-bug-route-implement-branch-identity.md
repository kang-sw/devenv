---
title: "Recognize a matching ticket commit on a nonstandard implementation branch"
related:
  260915-bug-ws-pi-widget-context-value-removed: route handling failure observed while verifying its already-landed implementation
dropped: 2026-09-21
---

# Recognize a matching ticket commit on a nonstandard implementation branch

## Background

`route.resolve_implement` refused an already-implemented ready ticket because its implementation branch suffix (`widget-context-port`) differed from the ticket stem. A read-only comparison established that HEAD `682a2718` precisely implements the target's Phase 1: its commit metadata names the ticket and its two-file diff matches the ticket's declared source and test scope. Re-invoking the route with that evidence returned the identical stop verdict, preventing normal verification, independent review, result recording, and closure on a valid serial branch.

## Phases

### Phase 1: Admit evidence-backed existing ticket work on a nonstandard implementation-branch suffix

Make branch routing recognize an unmerged commit whose ticket-update metadata and changed paths match the target ticket, or provide a deterministic evidence-backed override. Preserve the protection against genuinely mixed work. Cover both the accepted matching-commit case and rejected unrelated-work case.
