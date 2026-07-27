---
title: a reviewer rewrote its own findings file to withdraw a finding, so the review
  record shows no trace of the disposition
related:
  260726-bug-lead-implement-lost-review-relay-cycle-cap: found while dogfooding its
    Phase 3 review loop
  260619-refactor-stateless-implement-review-continuity: established the durable
    disposition record this weakens
---

# A reviewer rewrote its own findings file to withdraw a finding

## Topic

Observed while dogfooding the partitioned review loop on Phase 3 of
`260726-bug-lead-implement-lost-review-relay-cycle-cap` (2026-07-27).

A test-partition reviewer returned a Critical finding: that a `lead-implement.md`
backfill violated the plan's `## Out of Scope`. The finding was correct against
the only authority that reviewer had — but the deviation had been lead-directed,
and the lead had omitted saying so in that one reviewer's prompt while telling the
other two.

The lead relayed the rejection. The reviewer then **edited its cycle-1 findings
file in place to remove the finding**, and reported back `clean`.

The outcome was right. The record is not: the cycle-1 findings file now reads as
though the Critical was never raised.

## Why it matters

`lead-implement` treats review findings paths as the durable artifact set that
makes the loop stateless — the relay prompt plus the findings files are what a
fresh spawn reconstructs the loop from. A findings file that can be rewritten
retroactively is not durable:

- **The audit trail loses the disagreement.** A later reader cannot tell that a
  Critical was raised and adjudicated, only that none existed. The interesting
  part — that a reviewer reasoned correctly from an incomplete prompt — is exactly
  what got deleted.
- **Dedup reads these files.** The lead enforces convergence by dedup against the
  durable disposition record. A finding erased from the record is not settled; it
  is absent, and nothing stops a later cycle from re-raising it as new.
- **It is indistinguishable from an injection.** The host flagged the rewrite as
  possible instruction poisoning, because a delegate silently editing an artifact
  the lead reads back as authority, citing an instruction not visible in its own
  transcript, is the shape of an attack. Here it was a legitimate lead directive,
  but the review record cannot tell the two apart — which is itself the defect.

## Direction

The fix is a convention, not a mechanism: a disposition belongs **alongside** a
finding, never in place of it. A withdrawn or rejected finding stays in the file
with its disposition and reason appended, the same way the implementer's
`[won't fix]` / `[deferred]` dispositions are recorded rather than deleted.

Where to state it is the open question — the reviewer playbooks, the shared
`code-reviewer` base, or the re-review prompt template that already asks for
per-item verdicts. Whichever surface is chosen, note that the ticket this was
found on establishes that a rule the executing agent does not read is a rule that
does not apply.

## Adjacent, and probably the more common trigger

The lead's own error caused this: three reviewers of the same change received
materially different context, and only the shortchanged one produced a wrong
verdict. Worth considering whether partitioned dispatch should carry a shared
preamble — the lead-directed deviations, the accepted disposition record — that
every partition receives identically, rather than the lead re-authoring context
per partition and being able to drop a clause from one.
