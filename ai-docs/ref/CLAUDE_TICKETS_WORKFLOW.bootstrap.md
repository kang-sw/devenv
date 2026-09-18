# Tickets Workflow Bootstrap

One-time setup for the tickets workflow `CLAUDE_TICKETS_WORKFLOW.md`
describes. Read this file once, at setup, then delete it - it is not part of
the project's steady state and should not linger once the loop is working.

## When To Use This

The user hands you this file, alongside `CLAUDE_TICKETS_WORKFLOW.md`, when
adopting the ticket workflow in a project that has not used it before: no
ticket tree yet, `CLAUDE.md` may or may not exist, and the project may
already track work some other way. Work through the steps below in order,
then remove yourself.

## 1. Create The Ticket Tree

Create `ai-docs/tickets/todo/` and `ai-docs/tickets/.done/` if they do not
already exist. Do not create `ai-docs/tickets/.dropped/` preemptively - add
it the first time a ticket is actually dropped, per
`CLAUDE_TICKETS_WORKFLOW.md`'s own status-directory rule.

## 2. Wire The Embed

Open the project's `CLAUDE.md` (create it at the project root if the project
has none) and add an embed directive for the steady-state doc,
`@CLAUDE_TICKETS_WORKFLOW.md`, near where the project states its working
conventions. If `CLAUDE.md` already exists, add the line without disturbing
its existing content or ordering.

## 3. Reconcile With Existing Tracking

Look for whatever the project already used to track work and decisions before
these two files arrived - a `TODO.md`, a `NOTES.md`, inline `TODO:` comments,
a decisions log, an issue tracker export, or nothing at all. There is no
fixed script here; use judgment, and ask the user before touching or removing
anything of theirs that this step did not create:

- **Nothing pre-existing.** Skip this step.
- **A small, informal convention** (a TODO file, a handful of scattered
  notes). Fold its still-open items into new tickets under
  `ai-docs/tickets/todo/`, one ticket per unit of work, noting in each
  ticket's Background where it came from. Only remove the old file once its
  content has a new home and the user has confirmed it, and never when it
  holds history a ticket wouldn't capture (a changelog, for example).
- **A real external system** (an issue tracker, a project board). Do not
  migrate it wholesale. Coexist: use tickets for decision capture going
  forward, and leave the external system as the system of record for what it
  already owns. Never duplicate an item that already lives in both places.

## 4. Optionally Seed A First Ticket

Open one small ticket for a real, currently-relevant piece of work (not a
placeholder) to prove the loop end to end: filed under
`ai-docs/tickets/todo/`, worked, decisions logged, moved to
`ai-docs/tickets/.done/` on completion. Skip this step if the user would
rather start the loop on their own first real task.

## 5. Remove This File

Once the steps above are done, delete this bootstrap file from the project
root. Nothing later in the project's life needs to read it again; a project
that wants this setup repeated (a second project, a fresh clone missing the
ticket tree) gets a fresh copy of both files rather than resurrecting a
deleted one.
