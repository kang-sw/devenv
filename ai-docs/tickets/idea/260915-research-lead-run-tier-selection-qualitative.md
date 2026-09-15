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

### Confirmed Decisions
<!-- none yet -->

### Proposals
- (User-favored, unconfirmed) Move tier selection to a qualitative lead
  judgment over the four axes, picking `medium`/`large`/`xlarge`, restoring the
  apparent original intent; keep the axes as required inputs.
- Surface the Route Facts evidence column as an additive projection field so
  the qualitative pick has grounds to weigh.
- Record chosen tier + driving axis in the assignment note for audit.
- (Orthogonal follow-up) Decouple model tier from review breadth into two knobs
  so "more review, same model" is expressible.

### Open Questions
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
