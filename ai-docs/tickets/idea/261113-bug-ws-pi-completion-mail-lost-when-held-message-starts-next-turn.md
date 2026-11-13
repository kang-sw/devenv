---
title: "Pi completion mail lost when a held message starts the next turn"
---

# Pi completion mail lost when a held message starts the next turn

## Background

Observed 2026-11-13 by a lead session, twice. A Pi worker spawned with
`pi.spawn` finished its long turn with a full completion report, but the lead
never received that turn's completion mail. The lead only received the
completion mail of the NEXT, short turn, in which the worker answered a message
the lead had sent via `pi.send` while the long turn was still running (a "held"
message, shown as `held 1` in `pi.list`). The worker's reply said the
information was "already included in the completion report".

- Case 1: worker "whiteboard topic panels + log". `pi.list` showed `held 1`;
  the lead got only "Already included in 5b8d58710 and the final verification."
- Case 2: worker "hosted-lead bundled plugin". The lead got only "R2
  confirmation was already included in the completion report"; asking for a
  resend recovered it.

A daemon replacement happened around 08:48Z during case 2's turn (pid 33200).
It is not ruled out as the cause, but the common factor is a held `pi.send`
message.

`pi.send`'s tool description says: "during a turn it steers that running turn
at the next tool boundary ... The running turn's completion mail covers the
steered message; no extra turn is queued after it is taken." The observed
behavior contradicts this: an extra turn ran and the first turn's completion
mail was missing.

## Idea

Either:

- every turn end produces exactly one completion mail, delivered even when a
  held message starts the next turn immediately; or
- a held message delivered at turn end is folded into the same turn, so no
  extra turn is queued.

Add a regression test for "steer arrives after the last tool boundary".

## Open

- Was the mail dropped, overwritten (same mail key per subagent), or never
  produced?
- Does the daemon swap matter, or is the held `pi.send` the only trigger?
