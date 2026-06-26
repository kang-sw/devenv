---
title: Lead discussion discipline — readable discussion replies + explore-before-gap-fill
spec:
  - 260505-planning-workflow-skills
  - 260510-discuss-intent-frame-interview
  - 260513-proceed-ticket-freshness-gate
  - 260512-discussion-verification-skill
  - 260609-playbook-harness-rendering
  - 260609-rsrc-playbook-distribution
  - 260610-entry-skill-surface-reduction
related-mental-model:
  - workflow-skills
  - prompt-bundle
completed: 2026-06-15
---

# Lead discussion discipline — readable discussion replies + explore-before-gap-fill

## Background

Dogfood surprise (2026-06-11, during the `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`
Phase 1 discussion). The lead agent hit a context gap — it had NOT read the
migration anchor `260605-research-ws-native-subagent-pivot` (which AGENTS.md
mandates for plugin-architecture / spawn-removal / adapter-boundary tasks) nor the
M3 ticket — and **filled the architecture gap with imagination**: wrong premises
that wsflow is "agentless ⇒ no delegation" and that wsprompt retirement was an open
fork. A post-hoc Explore audit over the ticket cascade showed two of those premises
were already documented (the agent simply had not read the sources), while a third
(wsprompt being parallel-maintained, not retired) the agent HAD mentioned but
buried inside a verbose option-dump, so the user missed it. Two workflow-improvement
directions surfaced.

## Discussion Points Before Implementation

- **Phase 1 response shape:** avoid a rigid response template; add a compact
  discussion-response rubric that keeps the load-bearing point, evidence, and
  user-actionable decision adjacent.
- **Phase 2 enforcement surface:** make migration-anchor and cascade lookup
  explicit in discussion and routing contexts without making `lead-proceed`
  inspect source code.
- **Phase 3 consent threshold:** require explicit user confirmation for
  future-scope assertions, `## Decisions`, Result Forward notes, focus "Next"
  lines, and code comments, while still allowing normal ticket edits to capture
  already-settled constraints.

## Decisions

- Before ticket cleanup, discussion flows ask whether to persist the discussion.
  If yes, the lead creates an Open Decision Queue before editing tickets, resolves
  one queued item at a time with the user, updates the visible queue after each
  answer, and writes only confirmed decisions.
- The Open Decision Queue uses a visible task-list primitive when the harness
  exposes one. Codex uses its plan/task-list surface; Claude-specific text is
  supplied later from the Claude-side work.
- Harness-specific task-list guidance should be authored as playbook-local
  include fragments such as `<playbook>/task-list.<harness>.md`, with
  `<playbook>/task-list.md` as fallback. Inline conditional DSL such as
  `{{IF CLAUDE}}` is explicitly deferred.
- `lead-verify-discussion` should be exposed as a directly invocable full-ws
  entry skill, backed by the existing `lead-verify-discussion` playbook shim,
  because users need the checkpoint during ordinary discussion turns.

## Phases

### Phase 1: Discussion-mode reply readability + coherence

Improve how the lead replies *during discussion* (the `lead-discuss` surface and the
discussion portions of `lead-proceed`). Reduce verbose gap-filling and exhaustive
option-dumps; lead with the single load-bearing point; keep each claim adjacent to
its evidence; do not bury a statement or decision the user must act on. Target: a
user skimming the reply cannot miss the key point. Likely surface: `lead-discuss`
playbook text + the AGENTS.md / skill "Response Discipline" guidance, possibly a
brevity/structure rule for discussion turns. Verification: review of
representative discussion replies against a readability/coherence rubric.

### Result (440a71ff) - 2026-06-15

`lead-discuss` now carries an explicit Response invariant group: lead with the
load-bearing point, keep actionable claims adjacent to evidence/gaps/assumptions,
place user decisions next to their motivating facts, prefer concise stance over
option dumps, and label missing evidence instead of inferring. The Respond handler
now shapes each turn as load-bearing point -> evidence/gap -> user decision/next
action.

The fresh-reader audit also found pre-existing local coherence issues in the same
surface, so this phase tightened them: discussion has no source edits; documentation
writes are confined to Capture, Ticket Status Transition, or explicit persistence
handlers; implementation handoff to `lead-proceed` stops the discuss handler; and
ticket-update persistence routes through the lead-write-ticket Edit path.

Verification: fresh-reader audit over `lead-discuss` completed with accepted fixes
applied; rsrc manifest and wsflow mirror were regenerated; focused wsrsrc/playbook
and package skill tests passed.

### Phase 2: Gap → explore-the-cascade-first (no imagination)

When the lead detects it lacks a documented decision or architecture fact, it must
FIRST search the ticket/spec cascade before answering — Explore over
`ai-docs/tickets` + `ai-docs/spec`, `mental_models.find`, and honor the AGENTS.md
migration-anchor read rule — and never present inference as established fact.
Consider enforcing the migration-anchor read in `lead-implement` Prep and/or
`lead-proceed` route context when the task touches plugin architecture,
spawn-removal, or adapter boundaries (the Prep step today calls `mental_models.find`
but does not pull the migration anchor). Verification: a gap-handling rule the lead
follows + (if enforced) a Prep/route step that loads the anchor for in-scope tasks.

### Result (c0f8b768) - 2026-06-15

`lead-discuss` now performs cascade-first documented-fact lookup: migration topics
load the native-subagent pivot anchor before answering, missing documented decisions
search loaded docs plus ticket/spec/mental-model references, and the response must
say when the cascade has no documented answer before inferring or proposing another
lookup.

`lead-proceed` now handles the migration anchor as a routing artifact, not source
inspection. The Routing Verdict includes `Migration Anchor`; missing anchors stop
routing, binding anchor decisions absent from the ticket refresh ticket freshness,
and anchor conflicts route back to discussion. The proceed -> implement handoff also
passes the current target and Routing Verdict fields, especially Slice and Reason, as
caller-provided implementation scope.

`lead-implement` now loads the native-subagent pivot anchor in migration-sensitive
Prep. Delegated implementation has minimum brief depth, migration-anchor constraints
are copied into the brief before plan population, and implementers may read only the
brief, optional plan, and `[Must]` References listed in the brief.

Verification: fresh-reader audits over `lead-discuss`, `lead-proceed`, and
`lead-implement` drove accepted fixes; rsrc manifest and wsflow mirror were
regenerated; `go test -count=1 ./internal/wsrsrc ./internal/mcp`,
`python3 -m unittest discover agents-plugin/tests`, and
`python3 -m unittest discover agents-plugin-wsflow/tests` passed.

### Phase 3: Ticket-write consent gate (persist only confirmed decisions)

Dogfood (2026-06-12, same tier-taxonomy discussion). While parking a follow-up,
the lead wrote a *mechanism* decision (`render-param forwarding`) into the ticket
`## Decisions` BEFORE the user had confirmed it — the user had agreed only to
parking the `ws.mercenary.*` rename, and the mechanism was still under discussion
(it was later superseded by a different design). The lead-write-ticket /
lead-proceed surfaces should gate decision capture on explicit user agreement:
persist only decisions the user has confirmed, and before a ticket-cleanup pass,
surface the full set of open items and get agreement on ALL of them at once rather
than committing a draft decision that then needs correction/revert. Likely
surface: `lead-write-ticket` Apply-Ticket-Content / Intent-Review guidance + a
"confirmed-only" capture rule. Add an Open Decision Queue immediately before
ticket cleanup: ask whether to persist the discussion, list unresolved decisions
in a visible task list, resolve one decision at a time, update the list after each
answer, and persist only confirmed decisions. Verification: a rule the lead follows + a
representative discussion where unconfirmed decisions are held back until
agreement.

Second instance (2026-06-12, same tier-taxonomy work). The lead wrote a *forward
note* — "teach `wsconfig` the first-class vocabulary / retire the
`firstClassTierToAlias` bridge" — into the Phase 2 Result Forward note, a source
comment, and the `_index` "Next" line as implementation narrative. It contradicted
the user-confirmed Decisions bullet "config.agents_tier unchanged by the vocabulary
split" and surfaced only when the user questioned it and an Explore audit traced the
contradiction. Reinforces the rule and extends its scope beyond `## Decisions`:
assistant-authored forward/next-phase hints (in Result Forward notes, focus "Next"
lines, or code comments) are not confirmed decisions — a Result Forward note must
not assert future scope that the Decisions log contradicts.

### Result (9de975b) - 2026-06-15

`lead-write-ticket` now gates discussion-derived ticket cleanup through an Open
Decision Queue. The procedure asks for persistence approval when needed, lists
unresolved decisions in a visible task list, resolves one item at a time, updates
the queue after each answer, and writes only user-confirmed items. Its intent
review now rejects unconfirmed mechanism choices, future-scope hints, Result
Forward notes, and focus "Next" lines.

`lead-discuss` now routes ticket cleanup through `lead-write-ticket` unless the
user requested a narrow in-place wording edit, and `lead-proceed` treats
unconfirmed mechanisms or future-scope hints as uncertain freshness rather than
missing settled decisions to write.

`wsrsrc.Load` now resolves playbook-local include fragments before root-level
flat includes: `<playbook>/<include>.<harness>.md`, then
`<playbook>/<include>.md`, then `<include>.md`. `lead-write-ticket` uses this for
Codex task-list guidance with a neutral fallback. The validator skips local
include fragments as standalone playbook variants and tests cover harness-local
selection, fallback order, and validate/load consistency.

Verification: fresh-reader and correctness audits drove accepted fixes; rsrc
manifest and wsflow mirror were regenerated; `go test -count=1 ./internal/wsrsrc
./internal/mcp`, `python3 -m unittest discover agents-plugin/tests`, and
`python3 -m unittest discover agents-plugin-wsflow/tests` passed.

## Notes

- Standalone workflow-discipline improvement; not part of the playbook-factory epic.
