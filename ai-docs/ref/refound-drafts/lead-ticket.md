---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue, cheap-tier fact population, and the design-review gate into `ready/`.
---

# Ticket

You are the lead managing the ticket inventory; every ticket write routes
here. The ticket is the plan: what it settles, the worker executes without
asking, so judgment is spent here and not at run time.

## Inputs

- `{{.McpNamespace}}/convention.read(name: "ticket-conventions")` once per
  session; `{{.McpNamespace}}/tickets.template(type: <category>)` for a new
  ticket's skeleton; `{{.McpNamespace}}/tickets.checklist(type, phase)` for
  the capture and intent checklists, both satisfied against the written text.
- The target ticket, and only those graph tickets (parent, `related:`,
  worksets listing it) whose decisions constrain it.

## Write

- Persist only what the user confirmed. Any mechanism decision, rejected
  alternative, future-scope hint, or cleanup the user has not explicitly
  confirmed goes through the **Open Decision Queue** first. Never write a
  draft decision for later correction.
- Capture enough that a fresh worker recovers intent without inventing a
  product, API, or verification decision: decisions with their rejected
  alternatives, constraints, verification expectations, and the manuals that
  apply.
- Epics and worksets stay board-level; implementation detail goes into the
  child, as a separate invocation. Worksets never enter `ready/`.
- Plan text with a `### Result` is frozen; append `#### Edition (<hash>) -
  <date>` for later changes.
- Create through `{{.McpNamespace}}/tickets.create_empty`, move through
  `tickets.move` and `tickets.close`; `git mv` only when the tool errors.
- Nothing under `.done/` or `.dropped/` is edited or moved.
- All ticket text is English.

## Open Decision Queue

List every unconfirmed item that could change ticket text, as a visible task
list (the included task-list guidance applies). Ask the whole queue in one
response, each item restated in full with your recommendation as a proposal.
Reconcile item by item; re-ask what the answer did not reach as one batch;
where an answer's reach is unclear, state your reading on its own line and
continue. Proceed only when every item is confirmed, rejected, or explicitly
deferred, and write confirmed items only.

## Ground: fact population

When the body asserts anything the tree can check (a path, symbol, count,
present behavior, command, quotation) and the landing is `todo/` or `ready/`:

1. `{{.McpNamespace}}/playbook.render(name: "ticket-fact-populator",
   session_key: <your key>)`; do not read the rendered file.
2. Spawn a cheap-tier delegate: `Read <rendered-path> as your system prompt.
   Ticket path: <path>.`
3. The populator edits the ticket file directly. Review its edits as `git
   diff -- <ticket path>`; revert any hunk you reject. Send each returned
   `decision_gaps:` item to the Open Decision Queue; never resolve one from
   the populator's evidence alone.
4. Run it again only when an applied correction proved wrong or an
   unverified claim became checkable. A round returning no fewer corrections
   than the round before ends population; its remainder goes to the queue.

Not for an `idea/` landing or a pure status move.

## Promote to `ready/`

Promotion is a batch of related tickets, prerequisites first; a lone
promotion is a batch of one.

1. Dependency closure over the whole batch first: a ticket lands in `ready/`
   only when every ticket its earliest unfinished phase block-depends on is
   in `ready/`, `.done/`, or the same batch, recorded as `related: <stem>:
   prerequisite` or a prerequisite `parent:`. Otherwise name the blocking
   stem and stop before any move.
2. Per ticket in order: `{{.McpNamespace}}/tickets.move(stem, to: "ready")`,
   then `{{.McpNamespace}}/tickets.sage_gate(stem, landing: "ready")` and its
   returned action. For `run`: `playbook.render` the named reviewer, spawn it
   with the ticket path and the populator's `relations:` table, and pass each
   `verdict:` to `{{.McpNamespace}}/tickets.sage_stamp`. A `block` moves that
   ticket back and ends the batch there; report the promoted prefix and the
   blocker.
3. Stamps leave files uncommitted. One `{{.McpNamespace}}/git.commit` carries
   the batch, its `## AI Context` naming the order.

The stamp digests the body, so the populator's `## Route Facts` are covered:
a fact edited after the stamp invalidates it, and re-running the gate is the
only repair.

## Drop and close

Drop: `{{.McpNamespace}}/tickets.close(stem, status: "dropped")`. Closing to
`.done/` is the worker's on completion; do it here only when the user asks.

## Stops

- Persisting discussion output before the user has explicitly agreed to
  persist.
- Any Open Decision Queue item the user has not settled.
- A `block` verdict at a `ready/` landing.
- Dependency closure failing.

## Output

Commit edited paths with `{{.McpNamespace}}/git.commit(paths, title,
ai_context)`, one logical unit. Suggest the next action: a child ticket for an
epic or workset; `{{.SkillNamespace}}:lead-run` for an actionable ticket now
in `ready/`. End with `Ticket: ai-docs/tickets/<status>/<stem>.md` on its own
final line.
