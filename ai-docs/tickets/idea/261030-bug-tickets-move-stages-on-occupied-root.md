---
title: "tickets.move stages a ticket move into a worker's impl/* branch on an occupied root"
---

# tickets.move stages a ticket move into a worker's impl/* branch on an occupied root

## Observation

During lead-ticket promotion (2026-10-30), a lead session authored an
untracked ticket in the root checkout while it was on `develop`. A parallel
worker then switched the root to `impl/develop/cacti-frill-lance`. The lead's
`ws/tickets.move(stem, to: "ready")` succeeded and staged
`ai-docs/tickets/ready/<stem>.md` into that worker branch's index, with no
refusal or warning. Only the follow-up `ws/git.commit(expected_branch:
"develop")` caught the mismatch and pointed at the housekeeping worktree. The
lead had to unstage the file by hand before the worker could commit it.

## Expectation

`tickets.move` (and likely `tickets.create_empty` and other ticket writers)
should detect an occupied root (`HEAD` on `impl/*`) the way `git.commit` does
and refuse, or at least warn, naming the housekeeping-worktree route. A
staged write landing in a worker's index is the costly failure, because the
worker's next commit can absorb it silently.
