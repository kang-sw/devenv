---
title: Post-compaction continuation replayed a proceed dispatch onto an already-done ticket
related:
  260626-feat-session-key-format-and-retention: the stale target — already in .done/ when replayed
---

# Post-compaction continuation replayed a proceed dispatch onto an already-done ticket

## Background

Observed 2026-08-30 on `goal/develop/copper-lantern-drizzle`. A user `/compact`
was followed by a continuation that fired `/ws:lead-proceed` whose ARGUMENTS
named `target: ai-docs/tickets/ready/260626-feat-session-key-format-and-retention.md`.

That ticket does not exist at that path: `260626-feat-session-key-format-and-retention`
is in `.done/` — it was implemented, closed (`bad73e92`), and merged (`18659b86`)
in a prior session, well before the current goal track's work. The only ticket
actually in `ready/` at replay time was `260824-feat-review-release-gate-policy`.

Had the proceed procedure been followed mechanically against the given path, it
would have driven routing/implementation against a completed, closed ticket.
The lead avoided this only by verifying the target against the live tree
(`ls ai-docs/tickets/ready/` + `git log --grep`) before calling `enter.proceed`.

## Decisions

Nothing settled yet — capture only. Route/design belongs in a later triage.

## Phases

### Phase 1: Reproduce and localize the stale-dispatch source

Determine which mechanism produced the stale target, then close the gap:

- **Hypothesis A — stale queued dispatch replay.** The `/goal` drain
  (`lead-drain-ready-queue`) selected `260626` in an *earlier* session when it
  was still in `ready/`, handed it to `lead-proceed`, and the post-compaction
  continuation replayed that captured slash-command verbatim rather than
  re-selecting against the current board.
- **Hypothesis B — selector read a stale board.** The select subagent enumerated
  a stale/cached `ready/` listing and returned a path no longer present.

Deliverable: identify which holds (transcript + dispatch provenance), then make
the continuation/selection path re-validate the target's live status before
routing — a target that is not in `ready/` (moved to `.done/`/`.dropped/`, or
missing) must not route to implementation. Consider whether `enter.proceed`
should itself hard-fail a target whose file is absent from the declared status
directory, instead of relying on the lead's manual pre-check.

## Spec Impact

Likely touches proceed-routing and/or goal-drain-selection behavior contracts;
exact spec area to be determined during Phase 1 localization.
