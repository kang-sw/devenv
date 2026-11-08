---
title: "tracker: re-registering a remote under a second member id from the same machine key fails and leaves refs on the remote"
related:
  261107-epic-ws-personal-common-store: the tracker epic this host bug belongs to
  261108-feat-ws-tracker-drawer-ui: browser acceptance that surfaced it
---

# tracker: re-registering a remote under a second member id from the same machine key fails and leaves refs on the remote

## Observation

Found by the drawer UI worker during browser acceptance (2026-11-08,
isolated daemons, local bare remote): registering the same remote again
from the same machine key under a second member id fails with
`author ... is not a member in its causal past`. The failed attempt leaves
`refs/heads/tracker/id/<member>` and `refs/heads/tracker/m/<member>/<clone>`
on the remote.

## Open questions

- Is a second member id for the same machine key meant to be allowed (one
  person, two stores of identities), or should register refuse it up front
  with a clear message?
- Either way, a failed register should not leave refs behind on the
  remote; decide whether register pushes only after the join verifies
  locally, or cleans up on failure.

Code: `internal/wstrackerhost` register/join, `internal/wsledger` join and
create-once refs.
