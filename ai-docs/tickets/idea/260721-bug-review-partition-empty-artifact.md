---
title: Partition reviewer can report findings without writing its required artifact
related:
  260622-feat-playbook-render-tier-label: dogfood review whose fit partition completed with an empty artifact
---

# bug: review partition can complete with an empty artifact

## Observed Behavior

The fit reviewer for `260622-feat-playbook-render-tier-label` completed with
`non-clean: 1 critical/important`, but its required review artifact remained a
zero-byte file. The lead could see that an important finding existed but could
not inspect or relay it without reactivating the completed reviewer.

## Expected Behavior

A partition reviewer must not report completion until its required artifact is
non-empty and contains the findings that justify the returned status. The
review dispatcher or completion gate should validate the artifact before
accepting the partition result.

## Follow-up Questions

- Should the reviewer prompt require an atomic write-then-status sequence?
- Should the lead review workflow verify artifact existence and non-zero size
  before considering a partition complete?
- Can the review harness reject a completed result whose declared artifact is
  missing or empty?
