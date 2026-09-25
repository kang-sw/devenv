---
title: Ticket index move guard is silent during the absence TTL
related:
  260924-chore-review-sweep-test-and-naming-minors: split out of that ticket's re-review additions because it changes caller-visible behavior
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: the index repair that lengthened the silent window
---

# Ticket index move guard is silent during the absence TTL

## Background

Found in the re-review of the index repair (0a361491..4dcae29a). On a
never-seen clone with a read-timeout absence cached, `guardMoveClose` skips
the owner check for up to the 10-minute absence TTL, so `tickets.move` of a
stem a collaborator leased proceeds with no warning. The lease itself is
untouched, and the piggyback sync then discovers and registers the index.
Close is protected by C2; move is not, by the repair ticket's scope. Before the
repair the same skip lasted one call.

## Open questions

- Whether the piggyback should print the lease holder for a move too, so the
  caller learns after the fact that the moved ticket was leased by someone
  else.
