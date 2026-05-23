---
name: lead-write-ticket
description: Use when the user asks to create, edit, promote, drop, close, or durably capture a repository workflow ticket.
---

# Write Ticket

Target: user request

## Invariants

- Ticket conventions: call `wsflow/convention.read(name: "ticket-conventions")` - path format, status flow, phase rules, stem rules, templates.
- Aside from required conventions and `ai-docs/_index.md` when the queue changes, read only ticket files selected as edit targets or graph tickets needed to identify binding decisions; use `wsflow/tickets.*`, `wsflow/references.trace`, or focused local search limited to ticket paths/metadata for graph discovery.
- Preserve enough settled detail for a fresh implementation session to recover the intended contract without inventing missing product, workflow, API, or verification decisions.
- Epic tickets stay lightweight milestone boards; put detailed discussion, implementation phases, and slice-specific decisions in child tickets.
- Review related-ticket decisions by default; use explicit cascade for broader board or multi-ticket editing.
- Ready tickets require spec addressing, not mandatory planned spec text.
- Proceed-routed actionable `todo/` tickets move to `ready/` when intent review and spec-address check pass.

## On: invoke

### 1. Resolve

1. Call `wsflow/convention.read(name: "ticket-conventions")`.

### 2. Route

1. Classify category/status; mark **judge: spec-address-gate** for any non-`epic`, non-`research` ticket entering `ready/`.
2. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop ordinary single-target routing.
3. For a proceed-routed actionable `todo/` ticket, set the requested change to ready promotion.

### 3. Load

1. If `user request` references an existing ticket, read it.
2. For non-epic actionable creation or edits, run **Cross-ticket decision review** before phase drafting.

### 4. Write

1. For a new ticket, run **Create Ticket**.
2. For an existing ticket, run **Edit Ticket**.

### 5. Verify

1. Run **Intent Review**.
2. Run **Spec-address Check**.

### 6. Commit

1. Commit edited paths with `wsflow/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the queue changed; separate child invocations own their own commits and outputs.

### 7. Handoff

1. Run **Output Handoff**.

## On: Create Ticket

### 1. Classify

1. Determine category from the topic.
2. Choose the initial status directory through `judge: initial-status`.

### 2. Draft

1. Write the ticket using the **frontmatter template** and a clear problem/goal statement.
2. Populate `related-mental-model` with consulted or newly relevant mental-model stems, without `.md`; omit when none applied.

### 3. Shape

1. For `epic`: write only scope, non-scope, child ticket board, cross-child decisions, and done/drop/defer criteria.
2. For `epic`: reference existing/planned children; start a separate `wsflow:lead-write-ticket` invocation for child creation or child edit.
3. For non-epic actionable tickets, choose shape through `judge: ticket-shape`; default to one `Phase 1`.
4. For each non-epic actionable phase, run **Apply Ticket Content**.
5. Note inter-phase dependencies explicitly.

### 4. Ready Guard

1. For `ready/`, defer queue entry until **Spec-address Check** passes.

## On: Edit Ticket

### 1. Load

1. Read the ticket first when it was not already loaded.

### 2. Apply Change

1. Apply the requested change: phase update, content update, or status move.
2. For `epic`, keep edits board-level.
3. For `epic` implementation detail, stop after the epic edit and start a separate `wsflow:lead-write-ticket` invocation for the child ticket.

### 3. Move

1. For moves, use native `git mv`.
2. For `.done/` moves, add `completed:` date in frontmatter.
3. For proceed-routed `todo/` -> `ready/` promotion, defer `git mv` until **Spec-address Check** passes.

### 4. Shape

1. For non-epic actionable shape or phase changes, apply `judge: ticket-shape`.
2. For each changed non-epic actionable phase, run **Apply Ticket Content**.

## On: Apply Ticket Content

1. Capture goals, contracts, and agreed API/type/event/UI sketches.
2. Capture completion boundary and deferred scope.
3. Capture constraints and rationale.
4. Capture settled implementation strategy decisions; include suggested strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract.
5. Capture rejected alternatives.
6. Capture forward-compatibility guardrails.
7. Capture verification expectations.
8. Capture enough detail that a fresh implementer can build the intended result without filling settled gaps.
9. Exclude source-local edit notes unless settled constraints.

## On: Intent Review

1. Re-read the written/edited ticket against the conversation and cross-ticket decision review.
2. Check completion boundaries, decisions, constraints, rejected alternatives, forward-compatibility guardrails, verification expectations, and agreed strategy that constrains implementation.
3. Check whether agreed API/type/event/UI sketches were preserved literally, not prose-flattened.
4. Check whether the ticket distorts or omits discussed intent.
5. Check whether a fresh implementer could build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket; if yes, capture the missing settled decision.
6. Check whether related-ticket decisions that constrain this implementation slice were captured.
7. For `epic`, check that detailed implementation material stayed out of the epic and moved to a child-ticket invocation.
8. Fix gaps in-place.
9. Present a brief correction summary, or confirm nothing was missed.

## On: Spec-address Check

### 1. Scope

1. Skip `epic` and `research`.
2. Treat requested `todo/` -> `ready/` promotion as `ready/` for this check.
3. Apply `judge: spec-address-gate` before any `ready/` queue entry or commit.

### 2. Todo Handling

1. For `todo/`, preserve existing `spec:` links as optional recovery hints.
2. For `todo/`, do not require spec addressing, do not fire `judge: missing-spec-address`, and do not suppress the proceed prompt.

### 3. Ready Addressing

1. For `ready/`, use `wsflow/specs.find` or `wsflow/specs.status` to confirm existing `spec:` and `spec-remove:` stems.
2. For `ready/`, keep confirmed existing stems in frontmatter.
3. For `ready/`, when neither confirmed `spec:` nor `spec-remove:` stems address the phase, write or update `## Spec Impact`.
4. `## Spec Impact` must name the target spec area, expected caller-visible change, and `Contract-first spec: yes|no`.
5. If `Contract-first spec: yes`, continue through `wsflow:lead-write-spec`, re-check the created or updated stem, and list it in `spec:`.
6. After a contract-first spec is listed in `spec:`, remove redundant `## Spec Impact` text or keep only closeout notes not covered by the spec.
7. If neither confirmed stems nor `## Spec Impact` addresses a phase, apply `judge: missing-spec-address`.

### 4. Ready Queue

1. For `ready/`, remind that implementation commits should include a `## Spec` section for existing stems or the doc closeout should resolve `## Spec Impact`.
2. For `ready/`, ensure `ai-docs/_index.md ## Ticket Queue` has `` `stem` - one-line purpose and dependency notes ``.
3. For deferred `todo/` -> `ready/` promotion, perform native `git mv` before commit.

## On: Output Handoff

1. For `epic`, do not suggest proceeding on the epic path.
2. For `epic`, suggest creating, promoting, or proceeding a child ticket.
3. For non-epic tickets, suggest `wsflow:lead-proceed` unless `judge: missing-spec-address` fired.
4. State that proceed routes to implementation readiness; `wsflow:lead-implement` resolves direct execution needs.
5. Emit the current ticket path on its own final line for every create, edit, move, or promotion: `Ticket: ai-docs/tickets/<status>/<stem>.md`.
6. For `epic`, state that the path is a board artifact, not an implementation target.
7. Preserve the final `Ticket:` line; callers such as `wsflow:lead-proceed` capture this path from prefix-stage output.

## On: Cross-ticket decision review

1. Identify the target's parent, containing epic, child board, explicitly related tickets, and active siblings when those links are available.
2. Read only graph tickets that may contain decisions constraining the target's implementation scope.
3. Record binding cross-ticket decisions in the target as scope, constraints, forward-compatibility guardrails, rejected alternatives, verification expectations, or phase dependencies.
4. Do not copy unrelated future-phase detail; preserve only decisions that the current implementation could violate or block.
5. If the same decision changes another active ticket's role, include that ticket in this logical edit; otherwise leave related tickets untouched.
6. Keep epics board-level; move implementation constraints into the relevant child ticket or phase.

## On: Cascade Edit

### 1. Select Targets

1. Identify the impacted ticket graph: parent epic, containing epic, child tickets, related active tickets, and `_index.md` active inventory when it lists edited tickets.
2. Select edit targets from that graph; do not edit merely-related tickets whose role is unaffected by the propagated decision.
3. Read each selected target before editing.

### 2. Apply Propagation

1. Keep epics to scope, non-scope, child ticket board, cross-child decisions, and completion criteria.
2. Put implementation decisions, constraints, rejected alternatives, and phases into child tickets.
3. Do not promote tickets to `ready/` unless the user explicitly asks for ready promotion or routes through `wsflow:lead-proceed`.
4. For any selected target entering `ready/`, run Spec-address check before commit.

### 3. Verify and Report

1. Run Intent review across the edited set and commit one logical documentation unit when the edits are one decision propagation.
2. Report edited ticket paths; if exactly one implementation child is the natural next target, emit its `Ticket:` line.

## Judgments

### judge: spec-address-gate

Trigger: non-`epic`, non-`research` ticket creation or move into `ready/`.
Ungated: `idea/` creation and `idea/` -> `todo/` triage.
Find addressing: identify existing `spec:` or `spec-remove:` stems, or write a ticket-local `## Spec Impact` section.
Contract-first: continue through `wsflow:lead-write-spec` only when `judge: contract-first-spec` is yes.
Stop: no stem or `## Spec Impact` can address the behavior, `wsflow:lead-write-spec` failed, or the behavior is too underspecified; name the blocker.

### judge: initial-status

`idea/`: topic is exploratory or underspecified.
`todo/`: scope and goal are accepted actionable backlog.
`ready/`: spec-addressed implementation queue.
`todo/` `spec:` links: optional recovery hints.
Uncertain: prefer `idea/`.

### judge: contract-first-spec

Yes: planned behavior must be visible and stable before implementation begins.
Usually yes: externally consumed schemas, CLI/API contracts, file or wire formats, cross-skill routing contracts, or multi-ticket planned behavior.
No: ticket only needs a spec area for post-implementation closeout, final behavior will be refined during implementation, or planned text would mostly restate the ticket phase.

### judge: cascade-ticket-edit

Trigger: user asks to cascade broadly, reorganize a board and children, or update parent and child tickets beyond target-constraining decisions.
Do not trigger: a ticket merely has `related:` links or default cross-ticket decision review applies.

### judge: ticket-shape

Artifact role: keep epics board-level; put implementation detail in child tickets.
Scope keep: decisions, constraints, and agreed API/type/event/UI sketches.
Scope exclude: source-local edit notes unless settled constraints.
Ticket split: only when board, ticket, and implementation-unit roles are mixed, or unrelated increments belong in separate child tickets.
Phase default: non-epic actionable tickets use one `Phase 1`.
Phase unit: one reviewable implementation slice a future fresh session can finish, review, verify, and hand off cleanly.
Phase split: add phases only when review, verification, rollback, or dependency boundaries differ.

### judge: missing-spec-address

Trigger: a phase implements caller-visible behavior with no confirmed stem, `spec-remove:`, or `## Spec Impact` after `judge: spec-address-gate` runs.
Action: stop the authoring flow.
Report: name the uncovered phase and blocker.
Blocker: missing spec traceability for caller-visible behavior.

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives with enough settled detail that downstream skills do not fill gaps
with a different product, workflow, API, or verification contract. When unsure
whether a settled decision is needed for recovery, preserve the decision in
contract terms; do not preserve tentative discussion or source-local tactical
notes unless they became constraints.
