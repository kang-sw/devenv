---
title: "Reconsider lead-run worker-tier selection: mechanical OR-gate vs qualitative lead judgment"
related:
  260909-epic-ws-worker-interpreter-refoundation: context; that epic owns the lead/worker interpreter surface this routing gate lives on
---

# Reconsider lead-run worker-tier selection: mechanical OR-gate vs qualitative lead judgment

## Background

Dogfood surprise while draining tickets under `ws:lead-run`: workers elevate to
the `large` / `ticket-worker-elevated` tier very readily. Investigation shows
this is structural, not incidental, and that the design appears to have drifted
away from an originally qualitative intent into a mechanical table. This ticket
captures the evidence and the candidate direction; it does not lock an
implementation.

## Current mechanism (as built)

The dispatch tier is chosen in `agents-plugin/rsrc/lead-run/lead-run.md`:

- `lead-run.md:53`: "Only an explicit `high` raises the author tier; moderate
  risk still keeps its existing independent-review breadth."
- `lead-run.md:59`: a two-row table — **any** of `risk.correctness`,
  `risk.fit`, `risk.test`, `risk.security_or_contract` being `high` maps the
  ticket to `ticket-worker-elevated` @ `large`; otherwise `ticket-worker` @
  `medium`.

The `risk.*` axes are **not** computed deterministically. Chain of custody:

1. `ticket-fact-populator` (a model agent, `ticket-fact-populator.md`) grades
   each axis qualitatively at ticket-authoring time and writes a
   value + evidence row into the ticket's `## Route Facts` table. Its
   calibration rule (`ticket-fact-populator.md:101-103`) is explicitly
   anti-low: "on the risk and side-effect rows in particular, never guess a
   `low`."
2. The lead reviews the populator's diff and commits — the grades are then
   **frozen** in the ticket text.
3. At dispatch, `ws/tickets.query(format:json)` calls
   `wsdoc/tickets.go:ticketRouteFacts` (`tickets.go:556-604`), which is a pure
   markdown parser: it reads only the first two columns (`fact -> value`) and
   **deliberately drops the evidence column** — `tickets.go:560-561`: "Only the
   first two columns are read; the evidence column is the reviewer's, not the
   resolver's."
4. `lead-run` applies the OR-table to the bare enums; the lead never sees the
   evidence for why an axis is `high`.

## Structural bias analysis

- **Disjunctive OR over four axes.** Elevation trips if *any* one axis is
  `high`. With four independent chances, `P(elevate) = 1 - ∏(1 - p_i)`; even
  modest per-axis high-rates compound, so elevation trends toward the default
  outcome. One narrow high (e.g. `security_or_contract` on a tiny contract
  edit) elevates an otherwise-trivial ticket.
- **Binary output, no severity count, no small rung.** One high == four high.
  Below the line only `medium`/`large` exist; there is no `small` tier for
  trivial tickets, and `xlarge`/`ticket-worker-escalated` is reachable only
  reactively via a stop-e retry (`lead-run.md:241-242`), never as a proactive
  initial pick.
- **Tier and review-breadth are coupled.** Each worker playbook bundles a model
  tier *and* a review ceremony (elevated runs two-round partitioned review), so
  a single high forces both. The `:53` prose ("moderate risk still keeps its
  existing independent-review breadth") reads like a fossil of an intended
  decoupling that the hard table cannot express.
- **Anti-low calibration with no moderate↔high rubric.** The populator is told
  never to guess `low` on risk rows but is given no rubric separating
  `moderate` from `high`; the OR-gate then amplifies any upward skew.
- **Judgment is lossy and time-shifted.** A (possibly cheap) model discretizes
  a qualitative call into a per-axis enum at authoring time; the evidence is
  then stripped by the parser; a binary gate collapses the enums at dispatch
  time. The resourcing decision is driven by a reason-stripped, frozen enum
  produced by a different agent at a different time.

260910 itself was a legitimate elevate (three highs: `correctness`,
`security_or_contract`, `side_effect_risk`), so it is not the poster child for
over-elevation — but it illustrates how quickly highs accumulate under an OR
gate.

## Candidate direction

Because tier selection is a **resourcing decision, not a safety gate**
(over-elevation wastes tokens; under-elevation yields a weaker review that the
review gate itself still catches — no irreversible loss), qualitative variance
is tolerable here, and the determinism the table buys is low-value given its
input is already a lossy quantization of a model judgment. Sketch:

1. Replace the OR-table with a guided qualitative lead pick among
   `medium` / `large` / `xlarge`, considering the four axes together, with the
   four axes kept as **required inputs** (preserving "lead consumes the Route
   Facts projection, not the ticket body").
2. **Prerequisite:** the lead needs the evidence to judge, but the projection
   strips it today. Either (a) surface the evidence column as an additive
   projection field (e.g. `route_facts_evidence`) in `ticketRouteFacts` — an
   additive contract change that does not break existing consumers — or (b)
   let `lead-run` read only the `## Route Facts` section directly at dispatch.
   Option (a) preserves the projection-only invariant.
3. Record the chosen tier and the driving axis in the assignment note (the lead
   already writes a note) to recover most of the lost auditability.

## Deeper reframing: move the risk judgment to the lead at dispatch

A follow-on discussion questioned a deeper premise — the constraint that the
lead must not read the ticket body (`lead-run.md:42`, "use its Route Facts
projection; do not read or summarize the ticket body"). Reading the unit of
work you are about to dispatch is a sound default, and doing the risk analysis
first-hand at dispatch time (with current repo state) beats consuming a frozen
enum a cheaper model graded at authoring time.

What the read-body ban protects (intents are stated, not justified inline):

- **Single source of routing truth** — `ticket-fact-populator.md:113`: "Route
  Facts are the only place for routing judgment." Deliberate concentration for
  reproducibility/audit.
- **Restart/compaction survivability** — `lead-run.md:92`: a compacted or
  restarted lead rebuilds from the carry-over (assignment) note, so the lead is
  meant to stay shallow.
- **Lead context economy** — implied by choosing a projection over the body;
  not written.

These are largely recoverable under the reframed direction:

- Context: the lead reads only the one selector-chosen ticket, at dispatch —
  one body per dispatched cycle, bounded, and needed to route anyway.
- Reproducibility + restart: the lead records the chosen tier and the driving
  risk in the assignment note (already an MCP touchpoint, `session.note`); the
  single source moves from a frozen artifact to a recorded decision that a
  restart reads back.

**Separation of concerns (do not discard the projection).** Route Facts carry
more than risk: `dispatch_blocked`/prerequisite edges, phase/result state, and
scope/mirror facts are mechanical and reproducibility-worthy — keep them
computed in MCP. Only the *judgment* (risk → tier) moves to the lead.

**Reframed MCP role** (answers "does MCP need to intervene in the analysis?" —
not in the judgment): MCP shifts from *decider/gate* to *judgment scaffolding*:

1. keep computing the mechanical dispatch facts (`dispatch_blocked`, phase
   state) as today;
2. provide the risk *rubric* (the "scale" the analysis grades against) from one
   place, so lead, populator, and reviewers measure with the same ruler —
   today the four axes live only implicitly in the populator; if the lead
   judges, the rubric must reach the lead (playbook text, or an MCP-served
   shared rubric doc);
3. record the lead's tier decision (note).

End state: the lead reads the selected ticket at dispatch, analyzes risk
against a shared rubric, qualitatively picks `medium`/`large`/`xlarge`, and
records the decision. Author-time risk pre-grading disappears for tier
purposes; the fact-populator narrows to mechanical facts.

## Outcome Ledger

### Verified Findings
- `risk.*` axes are authored by the `ticket-fact-populator` model agent and
  frozen in the ticket's `## Route Facts` table; `wsdoc/tickets.go`
  (`ticketRouteFacts`, `tickets.go:556-604`) only parses them and intentionally
  drops the evidence column (`tickets.go:560-561`).
- Tier selection is an OR over four risk axes with binary output
  (`lead-run.md:53,59`); `xlarge` is reachable only reactively via stop-e
  (`lead-run.md:241-242`).
- The populator's calibration rule forbids guessing `low` on risk/side-effect
  rows (`ticket-fact-populator.md:101-103`), giving an upward skew the OR-gate
  amplifies.
- No test pins the lead's tier-selection mapping; `tickets_route_facts_test.go`
  covers only the fact projection, so removing the table breaks no existing
  test.
- The lead is barred from reading the ticket body (`lead-run.md:42`); the
  stated intents behind keeping the lead shallow are single-source routing
  truth (`ticket-fact-populator.md:113`) and restart/compaction survivability
  (`lead-run.md:92`). Context economy is implied, not written.
- Route Facts carry mechanical facts beyond risk (`dispatch_blocked`, phase
  state, scope/mirror), so the projection retains value even if risk grading
  leaves it.

### Confirmed Decisions
<!-- none yet -->

### Proposals
- (User-favored, unconfirmed) Move the risk judgment itself to the lead at
  dispatch: let the lead read the selected ticket, analyze risk against a
  shared rubric, and qualitatively pick `medium`/`large`/`xlarge`. Author-time
  risk pre-grading disappears for tier purposes; the fact-populator narrows to
  mechanical facts. (Supersedes the lighter "lead reads only the enums"
  variant.)
- Keep the mechanical Route Facts (`dispatch_blocked`, phase state, scope) in
  MCP; move only judgment out. Reframe MCP from decider/gate to judgment
  scaffolding: compute mechanical facts, serve the shared rubric, record the
  decision.
- If a lighter step is preferred instead: surface the Route Facts evidence
  column as an additive projection field so a qualitative pick has grounds to
  weigh without reading the body.
- Record chosen tier + driving risk in the assignment note for audit and
  restart recovery (already an MCP touchpoint via `session.note`).
- (Orthogonal follow-up) Decouple model tier from review breadth into two knobs
  so "more review, same model" is expressible.

### Open Questions
- Reopen the lead read-body ban for the selected ticket at dispatch? Weigh the
  bounded context cost (one body per dispatched cycle) against fresher
  first-hand judgment; confirm the single-source/restart intents are recovered
  by recording the decision in the note.
- Where does the risk rubric ("the scale") live so lead, populator, and
  reviewers grade against one ruler — playbook text, or an MCP-served shared
  rubric doc?
- Should tier and review-breadth be decoupled now, or is that a separate
  ticket? (Affects whether the worker-playbook set stays a 1:1 tier ladder.)
- If qualitative, how much guidance is enough to keep leads consistent without
  re-introducing a de-facto table (e.g. "correctness/security high generally
  warrants large; a lone fit/test high usually does not")?
- Is there value in retaining a deterministic *floor* (e.g.
  `security_or_contract: high` always ≥ large) for safety-adjacent axes while
  the rest is qualitative?

### Rejected Alternatives
<!-- none yet -->
