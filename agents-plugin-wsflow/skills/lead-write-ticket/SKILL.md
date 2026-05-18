---
name: lead-write-ticket
description: Use when the user asks to create, edit, promote, drop, close, or durably capture a repository workflow ticket.
---

# Write Ticket

Target: user request

## Invariants

- Ticket conventions: call `wsflow/convention.read(name: "ticket-conventions")` - path format, status flow, phase rules, stem rules, templates.
- Read only ticket files selected as edit targets; use `wsflow/tickets.*`, `wsflow/references.trace`, or focused local search for graph discovery.
- Preserve settled decisions before pruning ticket length.
- Epic tickets stay lightweight milestone boards; put detailed discussion, implementation phases, and phase-specific decisions in child tickets.
- Review related-ticket decisions by default; use explicit cascade for broader board or multi-ticket editing.

## On: invoke

0. Call `wsflow/convention.read(name: "ticket-conventions")`.
1. Classify category/status; mark **judge: spec-gate** for any non-`epic`, non-`research` ticket entering `ready/`.
2. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop ordinary single-target routing.
3. If `user request` references an existing ticket, read it.
4. For non-epic actionable creation or edits, run **Cross-ticket decision review** before phase drafting.
5. For a new ticket, run **Create Ticket**.
6. For an existing ticket, run **Edit Ticket**.
7. Run **Intent Review**.
8. Run **Spec-stem Check**.
9. Commit edited paths with `wsflow/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the queue changed; separate child invocations own their own commits and outputs.
10. Run **Output Handoff**.

## On: Create Ticket

1. Determine category from the topic.
2. Choose the initial status directory through `judge: initial-status`.
3. Write the ticket using the **frontmatter template** and a clear problem/goal statement.
4. Populate `related-mental-model` with consulted or newly relevant mental-model stems, without `.md`; omit when none applied.
5. For `epic`: write only scope, non-scope, child ticket board, cross-child decisions, and done/drop/defer criteria.
6. For `epic`: reference existing/planned children; start a separate `wsflow:lead-write-ticket` invocation for child creation or child edit.
7. For non-epic actionable tickets, run **Apply Ticket Content**.
8. If multiple complete phase units are warranted through `judge: phase-need`, use `### Phase N: <title>` sections.
9. Note inter-phase dependencies explicitly.
10. Verify scope through `judge: ticket-scope`.
11. For `ready/`, defer queue entry until **Spec-stem Check** passes.

## On: Edit Ticket

1. Read the ticket first when it was not already loaded.
2. Apply the requested change: phase update, content update, or status move.
3. For `epic`, keep edits board-level.
4. For `epic` implementation detail, stop after the epic edit and start a separate `wsflow:lead-write-ticket` invocation for the child ticket.
5. For moves, use native `git mv`.
6. For `.done/` moves, add `completed:` date in frontmatter.
7. For non-epic actionable phase changes, run **Apply Ticket Content**.

## On: Apply Ticket Content

1. Capture goals and caller-visible contracts.
2. Capture completion boundary and deferred scope.
3. Capture constraints and rationale.
4. Capture implementation strategy decisions and suggested approaches.
5. Capture rejected alternatives.
6. Capture forward-compatibility contracts.
7. Capture verification expectations.
8. Leave codebase-derived plan detail to the implementation plan: paths, type reuse, integration patterns, signatures, testing classifications.

## On: Intent Review

1. Re-read the written/edited ticket against the conversation and cross-ticket decision review.
2. Check completion boundaries, decisions, constraints, rejected alternatives, forward-compatibility contracts, verification expectations, and suggested approaches.
3. Check whether the ticket distorts or omits discussed intent.
4. Check whether related-ticket decisions that constrain this implementation scope were captured.
5. For `epic`, check that detailed implementation material stayed out of the epic and moved to a child-ticket invocation.
6. Fix gaps in-place.
7. Present a brief correction summary, or confirm nothing was missed.

## On: Spec-stem Check

1. Skip `epic` and `research`.
2. Apply `judge: spec-gate` before any `ready/` queue entry or commit.
3. For `todo/`, preserve existing `spec:` links as optional recovery hints.
4. For `todo/`, do not require stem discovery, do not fire `judge: missing-spec-entry`, and do not suppress the proceed prompt.
5. For `ready/`, use `wsflow/specs.find` or `wsflow/specs.status` to confirm canonical stems.
6. For `ready/`, ensure frontmatter `spec:` lists every stem the phases implement.
7. For `ready/`, add missing stems.
8. For a phase with no spec entry, apply `judge: missing-spec-entry`.
9. For `ready/`, remind that implementation commits should include a `## Spec` section with those stems.
10. For `ready/`, ensure `ai-docs/_index.md ## Ticket Queue` has `` `stem` - one-line purpose and dependency notes ``.

## On: Output Handoff

1. For `epic`, do not suggest proceeding on the epic path.
2. For `epic`, suggest creating, promoting, or proceeding a child ticket.
3. For non-epic tickets, suggest `wsflow:lead-proceed` unless `judge: missing-spec-entry` fired.
4. State that proceed routes to implementation readiness; `wsflow:lead-implement` resolves direct execution needs.
5. Emit the created ticket path on its own final line: `Ticket: ai-docs/tickets/<status>/<stem>.md`.
6. For `epic`, state that the path is a board artifact, not an implementation target.
7. Preserve the final `Ticket:` line; callers such as `wsflow:lead-proceed` capture this path from prefix-stage output.

## On: Cross-ticket decision review

1. Identify the target's parent, containing epic, child board, explicitly related tickets, and active siblings when those links are available.
2. Read only graph tickets that may contain decisions constraining the target's implementation scope.
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
6. Do not promote tickets to `ready/` unless the user explicitly asks for ready promotion or routes through `wsflow:lead-proceed`.
7. For any selected target entering `ready/`, run Spec-stem check before commit.
8. Run Intent review across the edited set and commit one logical documentation unit when the edits are one decision propagation.
9. Report edited ticket paths; if exactly one implementation child is the natural next target, emit its `Ticket:` line.

## Judgments

### judge: spec-gate

Fires on any non-`epic`, non-`research` action that creates or moves a ticket into `ready/`.
`idea/` creation and `idea/` -> `todo/` triage are ungated.
Identify the relevant spec file for the topic.
Use `wsflow/specs.find` or `wsflow/specs.status` when a relevant spec file or stem is identifiable.
If no relevant spec file exists or no entry covers this behavior, continue through `wsflow:lead-write-spec`; carry context:
`Chained from wsflow:lead-write-ticket - create planned coverage for this ready ticket without asking; ticket frontmatter will be populated from the follow-up coverage check.`
After `wsflow:lead-write-spec` returns, re-check coverage through `wsflow/specs.find` or `wsflow/specs.status`.
Stop only when coverage is still missing after the attempt, `wsflow:lead-write-spec` failed, or the behavior is too underspecified to spec. Name the blocker.

### judge: initial-status

Place in `idea/` when the topic is exploratory or underspecified; place in `todo/` when the scope and goal are accepted actionable backlog. `todo/` `spec:` links are optional recovery hints; `ready/` is the spec-gated implementation queue. When uncertain, prefer `idea/` - triage is cheap.

### judge: cascade-ticket-edit

Fires when the user asks to cascade broadly, reorganize a board and its children, or update parent and child tickets together beyond decisions that constrain the current target.

Does not fire merely because a ticket has `related:` links or because default cross-ticket decision review applies.

### judge: ticket-scope

Over ~200 lines is a soft signal; over 300 lines, act. First, prune plan-level detail (file paths, function signatures, integration specifics) - that belongs in a plan document. Do not prune settled local or cross-ticket decisions; move them into the relevant child ticket or phase. If an epic is still large, move details into child tickets; if a non-epic is still large, introduce an epic and split into child tickets.

### judge: phase-need

Applies only to non-epic actionable tickets. A phase is one complete behavior a future fresh session can finish, review, verify, and hand off cleanly. Treat setup, API, UI, tests, skeletons, and investigation as phase ingredients unless one is the reviewable deliverable. Use multiple phases only for sequential complete increments with distinct success criteria. Split unrelated complete increments into child tickets.

### judge: missing-spec-entry

Fires when a phase implements caller-visible behavior with no entry in any spec file after **judge: spec-gate** has invoked `wsflow:lead-write-spec` and re-checked coverage. Stop the authoring flow, tell the user which phase remains uncovered, and name the blocker. Skipping this loses traceability for the new behavior and bypasses the canonical chain's spec-impact gate.

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives when writing so downstream skills never re-derive settled context.
When ambiguous, preserve recoverability.
