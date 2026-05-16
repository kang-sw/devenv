---
name: lead-write-ticket
description: Use when the user asks to create, edit, promote, drop, close, or durably capture a repository workflow ticket.
---

# Write Ticket

Target: user request

## Invariants

- Ticket conventions: call `ws/convention.read(name: "ticket-conventions")` - path format, status flow, phase rules, stem rules, templates.
- Read only ticket files selected as edit targets; use `ws/tickets.*`, `ws/references.trace`, or `ws/subquery` for graph discovery.
- Preserve settled decisions before pruning ticket length.
- Epic tickets stay lightweight milestone boards; put detailed discussion, implementation phases, and slice-specific decisions in child tickets.
- Review related-ticket decisions by default; use explicit cascade for broader board or multi-ticket editing.

## On: invoke

0. Classify category/status; mark **judge: spec-gate** for any operation that creates or moves a non-`epic`, non-`research` ticket into `ready/`.
1. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop ordinary single-target routing.
2. If `user request` references an existing ticket, read it.
3. For non-epic actionable ticket creation or edits, collect **Cross-ticket decision review** findings before drafting or changing phase content.
4. **Create** (new ticket):
   a. Determine category from the topic.
   b. Choose initial status directory (`idea/` for vague, `todo/` for accepted actionable backlog - see `judge: initial-status`).
   c. Write the ticket using the **frontmatter template** and a clear problem/goal statement. Populate `related-mental-model` with the mental-model stems (filename without `.md`) that were consulted or arose during the current session - recovery hint for future sessions, not a validated link. Omit if no mental-model docs were relevant.
   d. If category is `epic`: write only scope, non-scope, child ticket board, cross-child decisions, and done/drop/defer criteria; reference existing/planned children and start a separate `ws:lead-write-ticket` invocation for any child creation or child edit.
   e. If category is not `epic` and multiple phases are warranted (see `judge: phase-need`), structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.
   f. After drafting, verify scope - see `judge: ticket-scope`.
   g. If status is `ready/`: defer queue entry until after **Spec-stem check** passes.
5. **Edit** (existing ticket):
   a. Read the ticket first.
   b. Apply the requested changes (update phase, move status).
   c. If the target is an epic, keep edits at board level; for detailed implementation discussion, stop after the epic edit and start a separate `ws:lead-write-ticket` invocation for the child ticket.
   d. For moves, use native `git mv` and add `completed:` date in frontmatter (-> `.done/`).
6. **Phase content** - for non-epic actionable tickets, capture goals, caller-visible contracts, constraints, rationale, implementation strategy decisions, rejected alternatives, forward-compatibility contracts, verification expectations, and suggested approaches. Leave codebase-derived details (paths, type reuse, integration patterns, signatures, testing classifications) to the plan.
7. **Intent review** - re-read the written/edited ticket against the preceding conversation and cross-ticket decision review:
   - Are decisions, constraints, rejected alternatives, forward-compatibility contracts, verification expectations, and suggested approaches captured?
   - Does the ticket distort or omit any discussed intent?
   - Does the ticket omit any related-ticket decision that constrains this implementation slice?
   - If the ticket is an epic, did detailed implementation material stay out of the epic and get routed to a separate child-ticket invocation?
   - Fix gaps in-place; present a brief summary of corrections (or confirm nothing was missed).
8. **Spec-stem check** - skip `epic` and `research`; apply **judge: spec-gate** before any `ready/` queue entry or commit:
   a. If status is `todo/`: preserve any existing `spec:` links as optional recovery hints; do not require stem discovery, do not fire `judge: missing-spec-entry`, and do not suppress the proceed prompt.
   b. If status is `ready/`: use `ws/specs.find` or `ws/specs.status` to confirm canonical stems.
   c. If status is `ready/`: ensure the ticket frontmatter `spec:` field lists every stem the phases implement. Add missing stems. If a phase implements behavior with no spec entry, see `judge: missing-spec-entry`.
   d. If status is `ready/`: remind that commits implementing this ticket should include a `## Spec` section with those stems.
   e. If status is `ready/`: ensure an entry exists in the `## Ticket Queue` section in `ai-docs/_index.md`. Format: `` `stem` - one-line purpose and dependency notes ``.
9. **Commit** - call `ws/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include every ticket edited by this invocation and `ai-docs/_index.md` when the queue changed. If separate child invocations changed child tickets, those invocations own their own commits and outputs.
10. **Proceed prompt** - if the ticket is `epic`, do not suggest proceeding on the epic path; suggest creating, promoting, or proceeding a child ticket instead. Otherwise suggest `ws:lead-proceed` as the next step after ticket authoring, unless `judge: missing-spec-entry` fired in step 8. Proceed routes to implementation readiness; `ws:lead-implement` resolves skeleton, plan, or direct execution needs.

   Emit the created ticket path on its own final line: `Ticket: ai-docs/tickets/<status>/<stem>.md`. For epics, also state that this path is a board artifact, not an implementation target. Callers such as `ws:lead-proceed` capture this path from prefix-stage output.

## On: Cross-ticket decision review

1. Identify the target's parent, containing epic, child board, explicitly related tickets, and active siblings when those links are available.
2. Read only graph tickets that may contain decisions constraining the target's implementation slice.
3. Record binding cross-ticket decisions in the target as scope, constraints, forward-compatibility contracts, rejected alternatives, verification expectations, or phase dependencies.
4. Do not copy unrelated future-phase detail; preserve only decisions that the current implementation could violate or block.
5. If the same decision changes another active ticket's role, include that ticket in this logical edit; otherwise leave related tickets untouched.
6. Keep epics board-level; move implementation constraints into the relevant child ticket or phase.

## On: Cascade Edit

1. Identify the impacted ticket graph: parent epic, containing epic, child tickets, related active tickets, and `_index.md` active inventory when it lists edited tickets.
2. Select edit targets from that graph; do not edit merely-related tickets whose role is unaffected by the propagated decision.
3. Read each selected target before editing.
4. Keep epics to scope, non-scope, child ticket board, cross-child decisions, and completion criteria.
5. Put implementation decisions, constraints, rejected alternatives, and phases into child tickets.
6. Do not promote tickets to `ready/` unless the user explicitly asks for ready promotion or routes through `ws:lead-proceed`.
7. For any selected target entering `ready/`, run Spec-stem check before commit.
8. Run Intent review across the edited set and commit one logical documentation unit when the edits are one decision propagation.
9. Report edited ticket paths; if exactly one implementation child is the natural next target, emit its `Ticket:` line.

## Judgments

### judge: spec-gate

Fires on any non-`epic`, non-`research` action that creates or moves a ticket into `ready/`. `idea/` creation and `idea/` -> `todo/` triage are ungated.

Identify the relevant spec file for the topic.
Use `ws/specs.find` or `ws/specs.status` if a relevant spec file or stem is identifiable.
If no relevant spec file exists, or no entry covers this behavior -> invoke `ws:lead-write-spec` with:
`Chained from ws:lead-write-ticket - create planned coverage for this ready ticket without asking; ticket frontmatter will be populated from the follow-up coverage check.`
After `ws:lead-write-spec` returns, re-check coverage through `ws/specs.find` or `ws/specs.status`.
Stop only when coverage is still missing after the attempt, `ws:lead-write-spec` failed, or the behavior is too underspecified to spec. Name the blocker.

### judge: initial-status

Place in `idea/` when the topic is exploratory or underspecified; place in `todo/` when the scope and goal are accepted actionable backlog. `todo/` `spec:` links are optional recovery hints; `ready/` is the spec-gated implementation queue. When uncertain, prefer `idea/` - triage is cheap.

### judge: cascade-ticket-edit

Fires when the user asks to cascade broadly, reorganize a board and its children, or update parent and child tickets together beyond decisions that constrain the current target.

Does not fire merely because a ticket has `related:` links or because default cross-ticket decision review applies.

### judge: ticket-scope

Over ~200 lines is a soft signal; over 300 lines, act. First, prune plan-level detail (file paths, function signatures, integration specifics) - that belongs in a plan document. Do not prune settled local or cross-ticket decisions; move them into the relevant child ticket or phase. If an epic is still large, move details into child tickets; if a non-epic is still large, introduce an epic and split into child tickets.

### judge: phase-need

Applies only to non-epic actionable tickets. Prefer more phases over fewer inside one cohesive child ticket; split unrelated reviewable units into separate child tickets. Single-component, single-concern work may be one phase.

### judge: missing-spec-entry

Fires when a phase implements caller-visible behavior with no entry in any spec file after **judge: spec-gate** has invoked `ws:lead-write-spec` and re-checked coverage. Stop the authoring flow, tell the user which phase remains uncovered, and name the blocker. Skipping this loses traceability for the new behavior and bypasses the canonical chain's spec-impact gate.

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives when writing so downstream skills never re-derive settled context.
When ambiguous, preserve recoverability.
