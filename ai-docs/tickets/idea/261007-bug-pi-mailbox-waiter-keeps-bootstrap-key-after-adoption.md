---
title: Pi mailbox waiter keeps the bootstrap key after revive adoption
related:
  261007-bug-pi-fork-lineage-keeps-bootstrap-key-after-adoption: same class (a consumer captures the bootstrap key before adoption)
  261007-bug-pi-session-key-reissued-on-resume: introduced revive-key adoption
---

# Pi mailbox waiter keeps the bootstrap key after revive adoption

## Background

Dogfood, 2026-10-07. A Pi lead in `gunpowder-odyssey` showed, on every wait
cycle:

```text
[ws-mailbox] ws-mcp mailbox wait: warning: named inbox worker@worktree is not
currently owned by this --session-key; falling back to a reply-id-only wait
```

Observed state:

- The Pi process's bridge ws-mcp (started 21:07) owns presence `worker`
  (worktree scope) as `grain-pretender-oblong`, the key the lead revived with
  and adopted (its agenda calls it the adapter default).
- The waiter subprocess runs
  `mailbox wait --session-key rounding-livestock-quack --slug worker@worktree`.
  `rounding-livestock-quack` is a 115-byte key record minted at the 21:07
  session start: the bootstrap key.

`agents-plugin-pi/src/index.ts` arms the waiter once at session_start with
`handle.defaultSessionKeyRef.current` captured into `mailboxSessionKey`, for
the wait subprocess, `createBridgeDrain`, and the self-slug lookup alike.
`adoptRevivedKey` (`bridge.ts`) later moves the default key and re-logs in,
which rebinds the named inbox to the adopted key, but nothing re-arms the
waiter. Until the Pi session restarts, the named inbox never wakes it and the
drain runs under the abandoned key (inferred from code; the restart fix is
not yet observed).

## Direction

Likely re-arm the mailbox waiter when the bridge adopts a revived key, and
test that a wait armed before adoption is replaced by one under the adopted
key. Consider fixing it together with the fork-lineage ticket, since both
capture the bootstrap key.
