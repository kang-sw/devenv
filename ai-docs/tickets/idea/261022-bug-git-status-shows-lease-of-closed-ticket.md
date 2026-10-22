---
title: "git.status reports a lease on a ticket that is already closed and released"
---

## Observation

On 2026-10-22, after the 260923 worker closed
`260923-bug-ws-mailbox-windows-store-read-sharing-violation` (moved to
`.done/`) and the lead merged its impl branch into `develop`, `ws/git.status`
still printed:

```text
develop 5305afc origin/develop clean
leased to this track: 260923-bug-ws-mailbox-windows-store-read-sharing-violation
```

A follow-up `ws/tickets.release` on that stem returned `status: no_change`, so
the lease index held no lease the caller could release. The status line
contradicts the release result.

## Open questions

- Does `git.status` read a stale ownership-index cache instead of the live
  index, or does it read a lease the release call does not see (track or
  clone mismatch)?
- Should closing a ticket (`tickets.close` / move to `.done/`) release the
  caller's lease, so neither surface shows it afterwards?
