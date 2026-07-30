---
title: Documentation-organization decisions should default to agent autonomy — narrow the Open Decision Queue admission filter
related:
  260726-bug-open-decision-queue-ledger-illegible: adjacent ODQ change — that one fixes how queued items are conveyed, this one narrows what enters the queue
  260730-feat-odq-batch-interview: lands first; it replaces serial asking with one batch interview, which changes what a long queue costs and therefore where this ticket's boundary should fall
  260630-epic-skill-playbook-diet: same direction of travel — remove ceremony that does not earn its cost
---

# Documentation-organization decisions should default to agent autonomy — narrow the Open Decision Queue admission filter

## Background

Raised by the owner as a new agenda item, with the note that the same pattern
recurs downstream: for documentation organization, the agent should act
autonomously unless the user has explicitly forced a priority.

Two independent observations point at the same place:

- **Downstream (wsflow 0.36.1 field report), §7.** A seven-item queue arose
  because ~10 turns of research preceded the ticket procedure, so consent had to
  be reconstructed retroactively. The reporter noted that an agent facing a queue
  that large may reasonably wonder whether it is misapplying the procedure — "which
  invites shortcuts at exactly the wrong moment."
- **This repository, this session.** Triaging that same field report produced an
  eight-item Open Decision Queue. Six items were pure placement: epic-child vs
  standalone, absorb vs rewrite, which epic hosts a one-line sweep, initial status
  directory. The owner's response was to delegate ticket organization wholesale
  and ask only for policy questions.

The gate itself is not the problem, and the report is emphatic on this point:
downstream, seven items were queued and **none had been explicitly confirmed by
the owner**, though the agent had internalized all seven as settled; two were
materially revised on contact with the actual question. Without the queue, agent
inferences would have been written into the ticket as owner decisions.

So the mechanism earns its cost. What is miscalibrated is `judge:
needs-open-decision-queue`'s admission filter, which currently admits "any
mechanism decision, rejected alternative, future-scope hint, Result Forward note,
focus 'Next' line, or note/comment proposal" not already explicitly confirmed.
That sweeps in decisions about where a record lives alongside decisions about
what gets built.

## Decisions

- The owner confirmed the direction: documentation organization should default to
  agent autonomy, and the queue should reserve the user's attention for policy.
- The queue mechanism is not being weakened or removed. Only its admission filter
  is in question.

## Open Questions

The load-bearing question, to be settled before implementation: **where exactly
is the line?** The proposal on the table, not yet confirmed:

- **Ask** — decisions that change *what gets built*: mechanism choices, rejected
  alternatives, contract or API shape, scope boundaries, verification
  expectations, anything that would be written as an owner decision. This is the
  full set of what downstream's seven items were.
- **Autonomous** — decisions that change *where the record lives*: parent/related
  placement, epic-child vs standalone, ticket vs section of an existing ticket,
  initial status directory, stem naming, which commit carries the edit,
  drop-and-absorb vs rewrite.

Secondary, from downstream §7: should the doctrine acknowledge that a large queue
during retrofit — persisting a settled discussion after the fact — is expected and
healthy rather than a sign consent was skipped? The reporter argues, credibly,
that ticketing earlier would have been *wrong* in their case: the decisions were
not stable until an experiment inverted the prior conclusion, and a recoverable
ticket cannot be written about a conclusion not yet reached. Framing this as a
distinct "retrofit mode" is likely the wrong shape — a named mode invites agents
to select it — but a single doctrine line stating that queue length is not a
defect signal may be enough.

Answer both questions only after `260730-feat-odq-batch-interview` lands. That
ticket replaces serial asking with a single batch interview, so a long queue stops
costing one turn per item. Both questions above were framed against the serial
cost — the primary one's motivation partly assumes a long queue is expensive to
ask, and the secondary one largely dissolves once a long batch reads as one
interview rather than as twelve interruptions. Re-derive the boundary against the
batch cost curve rather than carrying these framings forward.

## Phases

### Phase 1: Narrow the admission filter (pending the line above)

Once the boundary is confirmed, amend `judge: needs-open-decision-queue` in
`lead-write-ticket` to admit only the "what gets built" class, and add the
autonomy default for organizational decisions. Check whether `lead-discuss`'s
persistence handoff needs the matching change.

Do not start before the Open Questions are answered — the whole ticket is that
boundary.
