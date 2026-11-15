---
kind: print
---

# Ticket

Every lead-side ticket write: drafting, settling decisions with the user, and handing
promotion, epic design review, and revision of a broken `ready/` ticket to a
`ticket-handler` worker. A ticket is the
plan a worker executes without asking, so whatever it leaves unsettled
becomes a guess at run time.

## Inputs

- `{{.McpNamespace}}/convention.read(name: "ticket-conventions")` once per
  session.
- New ticket: `{{.McpNamespace}}/tickets.template(type: <category>)`.
  Checklists: `{{.McpNamespace}}/tickets.checklist(type, phase)`, satisfied
  against the written text.
- The target ticket, and only the graph tickets (parent, `related:`) whose
  decisions constrain it.

## Write

Persist:
- Only user-confirmed decisions, plus items the handler auto-confirms and
  reports. Exception: a research Outcome Ledger may keep entries labeled
  non-authoritative (Proposals, Open Questions).
- Anything unconfirmed (mechanism, rejected alternative, future-scope hint,
  cleanup) goes through the Open Decision Queue first. Never put an
  unconfirmed decision into `## Decisions`, even as working state.
- Persisted = the commit or the `ready/` move. Editing the file before them is
  working state.

Capture, so a fresh worker invents no product, API, or verification decision:
- decisions with their rejected alternatives
- constraints
- verification expectations
- the manuals that apply
- an approved visual reference: prefer text (HTML, SVG); link mockups at their
  tracked `ai-docs/assets/mockups/YYMMDD/<topic name>/` location, store
  binaries in `<ticket root>/assets/<stem>/`; verification compares a result
  capture against it side by side

Rules:
- Epic: board-level only; detail goes into a child, as a separate invocation.
- Plan text with a `### Result` is frozen; later changes append
  `#### Edition (<hash>) - <date>`.
- Create: `{{.McpNamespace}}/tickets.create_empty`. Move: `tickets.move`,
  `tickets.close`; `git mv` only when the tool errors.
- `.done/`, `.dropped/`: never edited or moved.
- Ticket text: English.

## Open Decision Queue

Queue every unconfirmed item that could change ticket text, one item per
decision. Where an answer's reach is unclear, state your reading on its own
line and keep the item open until the user confirms it.

Classes: the workflow manual's `### Asking the user`, plus:
- A documentation-placement choice (link, epic child or standalone, absorb or
  rewrite, initial status, stem, which commit): announced default when a
  neighbouring ticket or convention gives it (cite it); otherwise technical.
- A consequence of a policy question still open this round is held, and joins
  the next round's announced defaults once that answer lands.

Acknowledgement:
- One explicit user acknowledgement confirms the announced and technical
  groups together; it may arrive with the policy answers.
- Answers without it leave those items `[open]`; re-show them one line each
  under one re-ask.
- An item the user objects to, or one a later policy answer undercuts,
  becomes a policy question under its existing ID.
- Items raised later in the settlement need their own acknowledgement.

Settle point: announced and technical items acknowledged, every policy
question confirmed, rejected, or explicitly deferred. Persist then without
re-showing the confirmed set.

Section — `## Open Decision Queue` in the target ticket, so it survives
compaction (create the ticket first with
`{{.McpNamespace}}/tickets.create_empty`):
- The line under the heading: the last ID used.
- Item: `(01) [open] [policy] <one-line decision>`. Status: `[open]`
  `[confirmed]` `[rejected]` `[deferred]`. Class: `[announced]`
  `[technical]` `[policy]`. The one-line decision is the decision itself, not
  a label.
- Policy item: its Response format block. Announced: its citation. Technical:
  `<X> over <Y> - <technical reason>`.
- An item spanning tickets: the same ID in every affected ticket.
- Settled: confirmed → `## Decisions` (a technical item's `<Y>` into
  `Rejected:`); rejected → a decision's `Rejected:` when meaningful, else
  dropped; deferred → `## Constraints` as out of scope.
- Correction, objection, or reclass: back to `[open]` under its ID, out of
  `## Decisions` if it had moved.
- Delete the section only at the settle point.
- Research tickets: no section; the Outcome Ledger holds their entries.

Trace before writing — any change: a user correction or redirect, your own
proposal, the initial draft. Untraced, contradictions surface one review
round later.
- Check it against the ticket as written, tracing it through `## Decisions`,
  `## Constraints`, the phases, verification, `## Prior Decisions`, and the
  rules of any workflow document the ticket changes.
- Resolve contradictions before recording the change anywhere, including as a
  queue item; raise each new decision the resolution needs in the same
  response.
- While its knock-on items are open, the triggering item keeps its status and
  stays in the section, not moved to `## Decisions`.
- A resolution that alters a confirmed decision is a reopened item, never
  applied on your own.

### Response format

The heading, the three bold group labels, and the status tags stay English;
everything else follows the user's language.

```text
# Open Decision Queue

**Announced defaults** - these follow from existing constraints or earlier decisions, so each has one answer; acknowledge once, together with the technical decisions
(01) <decision> - <citation>

**Technical decisions** - calls I made as the technically best option, with the reason on each line; acknowledge once together with the defaults, or object to any item
(02) <X> over <Y> - <technical reason>

**Policy questions** - answer each
(03) <one-line decision>
- <context>
- <alternative: ...>
> <recommendation and why>
```

- Ask every item still awaiting an answer, groups in that order, empty groups
  omitted; label each item with its `(nn)` ID. Status and class tags stay in
  the section.
- A policy question's first showing is the full block; later re-asks are one
  line, `(nn) [open] <one-line decision>`.
- Report items settled this round in one line, e.g.
  `(01), (02) confirmed · (04) deferred`, plus each handler auto-confirmed
  item: `(05) auto-confirmed - <one-line decision>`.

## Derive actionable work from research

- The Outcome Ledger is the sole authority: `Verified Findings` are evidence,
  `Confirmed Decisions` the contract. Never carry `Proposals`,
  `Open Questions`, or unlisted narrative into the child.
- No Outcome Ledger: stop and ask whether to add one or settle the child's
  decisions through the queue.

## Promote, review an epic, or revise

When:
- `ready/` promotion of actionable tickets; a batch of related tickets,
  prerequisites first (a lone promotion is a batch of one).
- Epic design review, on your judgment when the epic's cross-child design has
  drifted materially; never a status step. A material cross-child change
  makes the epic's design stamp stale: run `epic-review` again before
  promoting a child that depends on the changed design.
- `revise`: a `{{.SkillNamespace}}:lead-run` worker stopped on a broken
  ticket contract (stop (c)); the handler applies its `proposed_resolution:`
  under a design review.
- Not: ordinary `todo/` creation and edits, epic or actionable. Epics and
  research never enter `ready/`.

Handler loop:
1. Settle any open queue first; the handler takes tickets with no
   `## Open Decision Queue` section.
2. `{{.McpNamespace}}/playbook.render(name: "ticket-handler", session_key:
   <your key>)`; pass the path on without reading it. Spawn at the tier the
   render returns: `Read <rendered-path> as your system prompt.` plus the task block:
   operation (`promote`, `epic-review`, or `revise`), the stems, and the
   branch to commit on (`revise`: the worker's impl branch); `revise` adds the
   worker's `proposed_resolution:` verbatim and the worker's tier.
3. Act on each report by its `status`:
   - `decisions`: the handler wrote its items into the tickets' queue
     sections, in the same ID space. Ask them in Response format, in the
     user's language; the Acknowledgement rules apply unchanged. Send the
     user's answers back as a digest: per ID, confirmed, rejected, deferred,
     or corrected (the item reopens with the user's correction), each with the
     user's stated reason, and the acknowledgement when given.
   - `awaiting-confirmation`: read every ticket in full and check the user's
     confirmed intent is there. Send corrections back; never edit the ticket
     yourself, since the stamp digests the body. No corrections: send
     `confirm`. This check is yours; it needs no fresh user round.
   - `done`: relay the settlement report and auto-confirmed items; after
     `revise`, return to `{{.SkillNamespace}}:lead-run` to resume the worker.
   - `blocked`: relay the blocker and the affected stems.
4. Continue the same handler across rounds; it holds the review baseline.

## Drop and close

- Drop: `{{.McpNamespace}}/tickets.close(stem, status: "dropped")`.
- Closing to `.done/` is the worker's on completion; here only when the user
  asks.

## Stops

- Persisting discussion output before the user explicitly agreed to persist
  it. Reaching the settle point is that agreement for the queued items.
- An unsettled queue item, apart from handler auto-confirmed ones.
- A handler `blocked` report.

## Output

- Writes outside the handler: one logical unit via
  `{{.McpNamespace}}/git.commit(paths, title, ai_context, expected_branch)`;
  `expected_branch` is the branch you remember working on (refused if the
  checkout moved). The handler commits its own work.
- Root occupied by a worker (`HEAD` on `impl/*`): no ticket write there.
  Author and commit in the sparse worktree the workflow manual's `### Git`
  describes, and render the handler in that worktree. Exception: `revise`
  writes the stopped worker's ticket on its impl branch, which the worker
  resumes from.
- Suggest the next action: a child ticket for an epic;
  `{{.SkillNamespace}}:lead-run` for a ticket now in `ready/`.
- End with `Ticket: ai-docs/tickets/<status>/<stem>.md` per ticket written,
  the last on its own final line.
