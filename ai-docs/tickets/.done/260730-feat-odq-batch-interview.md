---
title: Ask the Open Decision Queue as one batch interview with declared interpretation
sage-review-design: completed
sage-review-completeness: completed
related:
  260726-bug-open-decision-queue-ledger-illegible: the conveyance predecessor; it demoted the visible list to a record and made prose load-bearing, and this ticket amends the contract it wrote
  260726-feat-doc-organization-autonomy-odq-admission-filter: downstream; its Open Questions get cheaper answers once queue length stops costing a turn each, so it re-evaluates after this lands
  260630-epic-skill-playbook-diet: same direction — remove ceremony that does not earn its cost
spec:
  - 260727-odq-item-conveyance-restate-in-body
related-mental-model:
  - workflow-skills
completed: 2026-07-30
---

# Ask the Open Decision Queue as one batch interview with declared interpretation

## Background

`lead-write-ticket`'s Open Decision Queue asks the user one item at a time
(`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`, **On: Open Decision
Queue** step 4; spec `{#260727-odq-item-conveyance-restate-in-body}`, "resolves
one queue item at a time"). Serialization is the defect.

**Queue items are not independent.** The eight-item queue recorded in
`260726-feat-doc-organization-autonomy-odq-admission-filter` had six pure
placement items — epic-child vs standalone, which epic hosts a sweep, initial
status directory — which are one decision wearing three faces. Serial asking
forces a commitment to the first before the third is visible, and the third is
what makes the first wrong.

The symptom is already on the record. `260726-bug-open-decision-queue-ledger-illegible`
reports that of seven queued items downstream, "two were materially revised on
contact with the actual question." Serial asking maximizes exactly that: it
defers contact for every item but the one being asked.

Serialization also costs one turn per item, each carrying a re-rendered roll-up,
against one dump plus one answer for the batch.

The house already has the better shape one skill over. `lead-discuss` interviews
through the highest unresolved branch first and descends only after parents
settle (`{#260510-discuss-intent-frame-interview}`). That dependency-ordered
descent is right for a live discussion, where branches genuinely gate each other.
The ODQ is the opposite situation — it runs over the residue of a discussion that
already happened, so its items are co-visible and a batch dump fits. The flat
FIFO serial ask matches neither.

`260726-bug-open-decision-queue-ledger-illegible` already moved half the distance:
it demoted the visible task list to a record and made response-body prose the
load-bearing channel. Two things remain. Both include variants still call the list
"the consent ledger" and never mention the prose channel they now depend on, so a
reader of the include alone still sees the list as the mechanism. And the prose
channel is still rationed one item per turn.

## Decisions

**Dump the whole queue, then interview.** One response carries the full text of
every open item; the user answers in free prose across the batch, weaving items
that co-vary. This is the load-bearing change.

**Tooling holds state, the transcript carries conveyance.** The visible list keeps
the item set and each item's `open`/`confirmed`/`rejected`/`deferred` status —
that is what survives the lead's own compaction and what makes "did every item get
a disposition?" checkable. It stops being the channel the decision travels
through.

**Ledger item text stays self-describing.** Demoting the list from conveyance must
not be read as licensing a terse ledger. A ledger of labels is unexpandable after
compaction, which is the failure `260726-bug-open-decision-queue-ledger-illegible`
fixed. The item's visible text remains the decision itself.

**Attach a recommendation to each item; a recommendation is never a default.**
Answering eight items from blank costs the user more than dissenting from eight
stated positions. The rubber-stamp risk this creates — the queue exists to catch
what the agent wrongly internalized as settled, and a stated recommendation blunts
that filter — is contained by the next two decisions rather than by withholding
the recommendation.

**True silence leaves an item `open`.** No recommendation is ever adopted because
nothing contradicted it. An item whose disposition the answer does not reach stays
open and returns in a follow-up batch.

**An item still `open` after re-batching blocks the write; it is never
auto-disposed.** Follow-up batches have no round limit and no timeout that converts
open to anything else. This closes the one gap the batch shape opens relative to
serial asking, where the next question could not be reached until the current item
was answered. It also resolves an existing tension: the playbook already blocks
continuation until every item is confirmed/rejected/deferred, while the spec
sentence tolerates "unanswered" items by omitting them from writes. Omission
applies to what gets *written*, never to whether the queue may be left unresolved.

**Ambiguity resolves by declared interpretation, not by re-asking.** When an answer
plausibly reaches an item but not unambiguously, the agent states its reading on
its own line and proceeds — "reading [1] and [2] as confirmed and [3] as deferred,
continuing." Rejected alternative: a grammar of what counts as an explicit
agreement signal, with a re-ask for anything failing it. That was drafted and
dropped — it reinstates a confirmation turn per batch, spending back the round
trips the batch saved, and no such grammar survives contact with how people
actually answer. The brake is the declaration, which the user can contradict in
one line; it is not a re-ask.

**Recommendations live in prose only, never in the ledger.** Ledger items carrying
recommendations grow long, and length is the input to the rendering truncation
that `260726-bug-open-decision-queue-ledger-illegible` fixed. The asymmetry is
decisive: an agent that loses its own recommendation to compaction can regenerate
it, while a truncated ledger is simply unreadable to the user.

**This ticket lands before `260726-feat-doc-organization-autonomy-odq-admission-filter`
answers its Open Questions.** Batching collapses the cost of a long queue, so the
ask-vs-autonomous boundary that ticket is trying to draw can be drawn less
aggressively afterward. Its secondary question — whether doctrine should state
that a long retrofit-time queue is not a defect signal — is largely answered by
the batch shape, since a long batch reads as one interview rather than as twelve
interruptions.

## Constraints

- **Do not weaken the gate.** `260726-bug-open-decision-queue-ledger-illegible`
  records that downstream, seven items were queued and none had been explicitly
  confirmed by the owner though the agent had internalized all seven as settled.
  The invariant is that every item receives an explicit disposition; one-at-a-time
  asking was one implementation of it, and the batch shape must carry it through
  the reconcile step rather than drop it.
- **Do not regress the conveyance contract.** The self-describing-item rule and the
  non-load-bearing secondary-field rule survive unchanged on all three surfaces,
  including the Markdown-checklist fallback.

## Phases

### Phase 1: Replace serial asking with batch dump plus reconcile

Rewrite **On: Open Decision Queue** step 4 in
`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` into the batch shape:
restate every open item's full text in one response body with a recommendation per
item, then reconcile the user's answer item by item, marking dispositions and
returning genuinely unreached items to a follow-up batch. The reconcile step is
what carries the every-item-gets-a-disposition invariant and must be written as an
obligation, not as a closing remark. Where an answer's reach is unclear, the
declared-interpretation line replaces a re-ask.

Amend both host variants of the include —
`agents-plugin/rsrc/lead-write-ticket/task-list.md` and
`task-list.codex.md`. These are the two copies; they are named here rather than
discovered, and both must end up saying the same thing. Each drops the framing that
makes the list the channel and keeps it as the state ledger, retains the
self-describing-item and non-load-bearing-secondary-field rules, and states that
recommendations do not go in the ledger.

Each also drops its serial-rhythm rule, which becomes unfirable once there is no
"next item": `task-list.md` "Update the visible list after every user answer and
before asking the next item", and `task-list.codex.md` "Refresh the task list after
each user answer before asking about the next item". The surviving obligation is
that the ledger reflects every disposition the reconcile step assigns, which is
about state accuracy rather than about when the next question is asked.

`agents-plugin-wsflow/rsrc/` is a generated byte-identical mirror and must never be
hand-edited. Regeneration is mandatory after any `rsrc/` edit and is the recurring
gap already recorded as `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`
and `260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`:

```
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
```

Amend `{#260727-odq-item-conveyance-restate-in-body}` in
`ai-docs/spec/workflow-skills.md`. The anchor slug does not change — restate-in-body
remains accurate, with all items rather than one. Two statements invert, both inside
this anchor's own blocks: "resolves one queue item at a time, updates the visible
queue after each answer", and "Asking an item restates that item's full text in the
response body, followed by a one-line status roll-up of the remaining items".
Overwrite both in place with the affirmative batch contract. Do not add a retraction
note: spec conventions admit only the Implementation Gap Callout as a
non-current-behavior form and optimize for drift resistance, and because both
statements sit inside the rewritten anchor, overwriting leaves no orphaned wording
for a reader to arrive at. This is a narrower situation than
`{#260723-lead-goal-step-rename-reposition}`, whose retraction paragraph guards a
factual mechanism claim a future author could plausibly re-derive from first
principles; a superseded asking rhythm is self-evidently deliberate once the batch
contract is stated.

Update both ODQ entries in `ai-docs/mental-model/workflow-skills.md`. The
`{#260505-planning-workflow-skills}` clause describes the queue purely in terms of
the harness task-list fragments — the half this ticket demotes. The entry keyed to
`{#260727-odq-item-conveyance-restate-in-body}` asserts the per-item
restate-plus-roll-up as the load-bearing channel, which is exactly what the batch
dump retires; leaving it would put a mental-model line in contradiction with the
anchor it cites. The rest of that entry survives and must be preserved: the
named-pair rule for the two include files, and the prohibition on reintroducing a
host-specific field name into their shared rule.

Add the sequencing note to
`260726-feat-doc-organization-autonomy-odq-admission-filter`: a `related:` entry
naming this ticket and a line in its Open Questions recording that the boundary is
re-evaluated after the batch transition, because queue length stops costing a turn
per item.

Out of scope, verified as needing no change: `lead-discuss.md`, whose only ODQ
mention routes persistence to `lead-write-ticket` without restating the asking
rhythm; and `ai-docs/spec/mcp-tools.md`, which names the ODQ only as a routing
destination.

Rejected alternatives: keeping serial asking behind a queue-length threshold
(two rhythms to specify and a threshold with no principled value); withholding
recommendations to protect the gate (the conservative default plus the declared
interpretation already contains the rubber-stamp risk, at lower cost to the user).

Verification boundary — an artifact check, since this is a text-only change with no
runtime probe:

1. **On: Open Decision Queue** dumps all open items in one response, attaches a
   per-item recommendation, and carries a reconcile obligation that dispositions
   every item; no step asks items serially.
2. Both include variants describe the list as the state ledger, retain the
   self-describing-item and non-load-bearing-secondary-field rules including on the
   Markdown-checklist fallback, exclude recommendations from ledger text, and no
   longer carry a serial-rhythm rule.
3. `{#260727-odq-item-conveyance-restate-in-body}` states the batch contract, the
   conservative `open` default, the declared-interpretation rule, and the
   never-auto-dispose rule; neither one-at-a-time statement survives anywhere in
   the anchor, and no retraction note was added.
4. Both mental-model entries are updated — the
   `{#260505-planning-workflow-skills}` clause and the
   `{#260727-odq-item-conveyance-restate-in-body}` entry — with the latter's
   named-pair and no-host-specific-field rules still intact.
5. `260726-feat-doc-organization-autonomy-odq-admission-filter` carries the
   `related:` entry and the Open Questions sequencing line.
6. Both regen commands run clean and produce no diff on a second run;
   `go test ./... -count=1` passes, as do
   `python3 -m unittest discover agents-plugin/tests` and
   `python3 -m unittest discover agents-plugin-wsflow/tests` run from the
   repository root.

### Result (766281e6) - 2026-07-30

Done. All six verification-boundary conditions confirmed.

**On: Open Decision Queue** went from seven steps to nine. The old step 4 split
into three: batch dump with per-item recommendation, reconcile-and-re-batch, and
the declared-reading rule. The old step 5 absorbed the never-auto-dispose clause.
Both includes ended at eight bullets, each still one line.

The spec anchor gained a third paragraph. The first two absorbed the batch
contract and the recommendations-stay-out rule in place; the recommendation-is-a
-proposal, unreached-stays-open, and declared-reading rules had no existing
sentence to overwrite, so they are stated affirmatively as their own block. No
retraction note was added, per the phase text.

Deviation, deliberate: `ai-docs/ref/skill-authoring.md` permits H3 sub-blocks once
a handler exceeds four steps with mixed responsibility, and this handler is now
nine. It stayed flat. The flow is linear rather than mixed — consent, collect,
ledger, ask, reconcile, write — and no other handler in the file uses sub-blocks,
so introducing them in one handler would have cost more scanning consistency than
the depth buys.

The mental-model entry was trimmed in a follow-up pass after the owner asked why
a policy change read as net-add. The first pass had restated the
recommendation/open/ambiguity contract there, which the spec already carries;
mental-model holds rationale that does not re-derive from the spec, so that was
redundancy under the Non-redundant invariant. It was replaced with the reason the
new rules exist at all — serial asking enforced every-item-disposition
structurally, so the batch shape must write as rules what the old rhythm got for
free — which is the guard against a later author trimming them as ceremony. The
entry still runs roughly twice its former length because it now carries two
decisions rather than one.

Two other sources of net-add are structural rather than incidental, and worth
naming so a later diet pass does not mistake them for slack. Replacing a
mechanism with rules is inherently additive in text: the old rhythm enforced the
invariant by making item 2 unreachable, and nothing but prose enforces it now.
And per-item recommendations are a second feature bundled into this ticket,
carrying their own two rules; they could have been a separate ticket.

Unresolved: **On: Fresh-Reader Audit was not run.** `skill-authoring.md` requires
it after skill edits and specifies a separate fresh reviewer holding only the
target file, which this session was directed to complete without delegation. The
edited handler and both includes are therefore unaudited by a context-free reader.
Anyone next touching `lead-write-ticket` should run it before their own edits.

Verification: `go build ./...` clean and `go test ./... -count=1` green across 12
packages; `python3 -m unittest discover agents-plugin/tests` 46 tests OK and
`agents-plugin-wsflow/tests` 10 tests OK, both from the repository root. Both
regen commands ran clean and the second run produced no additional diff. The
wsflow mirror is byte-identical to the ws copy for `task-list.md` and for the ODQ
handler region. `ws/spec_index.verify` returned ok. A corpus grep for
`one queue item at a time`, `next item`, and `roll-up` across both plugin trees,
the spec, and the mental model returns nothing.


## Resolution (2026-07-30)

Phase 1 landed in 766281e6, with a mental-model trim in 25dacb3a and the version bump in a313b2d6. The Open Decision Queue now asks every open item in one response with a per-item recommendation, and reconciles the answer item by item.

Deliberately left open: the Fresh-Reader Audit that `ai-docs/ref/skill-authoring.md` requires after skill edits. It specifies a separate fresh reviewer holding only the target file, and this session was directed to implement without delegation. The edited ODQ handler and both task-list includes are unaudited by a context-free reader.

Per-item recommendations were bundled into this ticket rather than split out. They are a distinct feature from the asking rhythm and carry their own two rules; a stricter split would have made them a sibling ticket.

`260726-feat-doc-organization-autonomy-odq-admission-filter` now carries the sequencing note: both of its Open Questions were framed against the serial cost curve and should be re-derived against the batch one before that ticket starts.
