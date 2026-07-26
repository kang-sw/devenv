---
title: The Open Decision Queue ledger can degrade to illegible while satisfying every stated rule
related:
  260726-feat-doc-organization-autonomy-odq-admission-filter: adjacent ODQ change — that one narrows what enters the queue, this one fixes how queued items are conveyed
sage-review-design: required
---

# The Open Decision Queue ledger can degrade to illegible while satisfying every stated rule

## Background

Reported downstream (wsflow 0.36.1). The consent mechanism stopped functioning
and recovered only because the human said so.

`lead-write-ticket`'s **Included Guidance: Open Decision Queue Task List**
(`agents-plugin/rsrc/lead-write-ticket/task-list.md`) mandates a visible task
list and states: "Treat the list as the consent ledger; do not replace it with
hidden notes." It says nothing about how a queue item should be composed.

The downstream agent put decision content in the task `description` and short
labels in `subject`. That harness truncates descriptions. The owner reported
everything after item 3 was invisible. The ledger existed, satisfied the letter
of every rule, and conveyed nothing. Recovery was ad hoc: move load-bearing
content into `subject`, then re-serialize each item into chat prose.

Nothing in the guidance instructs the agent to check that the ledger is legible,
and nothing covers the subject/description split. The mechanism's correctness
depends on a rendering property no rule mentions.

This matters more than a formatting nit: the queue is the consent record.
Downstream, seven items were queued and **none had been explicitly confirmed by
the owner** even though the agent had internalized all seven as settled; two were
materially revised on contact with the actual question. An illegible ledger turns
that gate back into agent inference.

## Decisions

- **Queue item subjects must be self-describing.** The subject carries the
  decision itself, not a label. `description` is optional detail that may not
  render and must never be load-bearing.
- **Restating each item in the response body is the documented default, not a
  recovery.** This is the load-bearing half: it is harness-independent, so it
  cannot silently degrade. The visible list is the record; prose is the channel.
- **Do not mandate reprinting the whole queue every turn.** That is noise. The
  shape is: full text of the item being asked, plus a one-line status roll-up of
  the rest.
- **Applies to `lead-discuss` persistence handoff too**, wherever the same
  guidance block is included.

## Constraints

- Do not weaken the queue itself. Downstream's report explicitly names the ODQ as
  the highest-value gate of the procedure; this ticket is about conveyance only.
- The guidance already allows a Markdown checklist fallback when no task list
  exists. Keep that path and apply the same self-describing rule to it.

## Spec Impact

- Target spec area: `ai-docs/spec/workflow-skills.md` if the ODQ conveyance
  contract is documented there; otherwise the change is confined to the bundled
  `task-list.md` guidance and its playbook step.
- Expected caller-visible change: agents running the ODQ restate each queued item
  in the response body and compose self-describing subjects.
- Contract-first spec: no. This is guidance text whose exact wording should be
  settled while editing it.

## Phases

### Phase 1: Make the ledger legible by construction

- Amend `task-list.md`: subjects self-describing; `description` explicitly
  non-load-bearing; the same rule applied to the Markdown-checklist fallback.
- Amend `lead-write-ticket`'s **On: Open Decision Queue** step 4 so asking an item
  includes restating it in the response body, with a one-line roll-up of the
  remaining items.
- Check every other playbook that includes this guidance block and keep the two
  copies from drifting.

Rejected alternatives: relying on the harness to render descriptions (the failure
being fixed); reprinting the entire queue each turn (noise, and it buries the
question being asked).

Verification boundary: a queue run in a description-truncating harness conveys
every item's content through the response body alone.
