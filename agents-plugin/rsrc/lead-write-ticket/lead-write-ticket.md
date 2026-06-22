---
kind: print
includes:
  - task-list
---
# Write Ticket

Target: user request

## Invariants

- Ticket conventions: call `{{.McpNamespace}}/convention.read(name: "ticket-conventions")` - path format, status flow, phase rules, stem rules, templates.
- Aside from required conventions, focus updates, and explicitly routed spec or mental-model checks, read only ticket files selected as edit targets or graph tickets needed to identify binding decisions.
- Preserve enough settled detail for a fresh implementation session to recover the intended contract without inventing missing product, workflow, API, or verification decisions.
- Epic tickets stay lightweight milestone boards; put detailed discussion, implementation phases, and slice-specific decisions in child tickets.
- Workset tickets stay non-hierarchical operating-context collections; never add, remove, or change `parent:` based on workset inclusion.
- Review related-ticket decisions by default; use explicit cascade for broader board or multi-ticket editing.
- Ready tickets require spec addressing, not mandatory planned spec text.
- Ordinary `todo/` edits leave the ticket in `todo/`; proceed-routed actionable `todo/` tickets move to `ready/` only after intent review and spec-address check pass.
- Persist only user-confirmed decisions; keep unconfirmed mechanisms, future-scope hints, and draft forward notes out of tickets and focus text.
- Before discussion-derived ticket cleanup, resolve the Open Decision Queue with the user and write only confirmed queue items.

## On: invoke

### 1. Resolve

1. Call `{{.McpNamespace}}/convention.read(name: "ticket-conventions")`.

### 2. Route

1. Classify category/status; mark **judge: spec-address-gate** for any non-`epic`, non-`research`, non-`workset` ticket entering `ready/`.
2. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop ordinary single-target routing.
3. For a proceed-routed actionable `todo/` ticket, set the requested change to ready promotion.

### 3. Load

1. If `user request` references an existing ticket, read it.
2. For actionable creation or edits, run **Cross-ticket decision review** before phase drafting.
3. For workset creation or edits, verify each existing included ticket's path, current status directory, and stated role; convert missing tickets to planned references or stop on a blocker.

### 4. Consent Gate

1. Apply `judge: needs-open-decision-queue`; if it fires, run **Open Decision Queue** before editing ticket text.

### 5. Write

1. For a new ticket, run **Create Ticket**.
2. For an existing ticket, run **Edit Ticket**.

### 6. Verify

1. Run **Intent Review**.
2. Run **Spec-address Check**.

### 7. Commit

1. If no file changed because the requested move was refused, skip commit.
2. Commit edited paths with `{{.McpNamespace}}/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when focus changed; separate follow-up invocations own their own commits and outputs.

### 8. Handoff

1. Run **Output Handoff**.

## On: Create Ticket

### 1. Classify

1. Determine category through `judge: ticket-category`.
2. Choose the initial status directory through `judge: initial-status`.

### 2. Draft

1. Write the ticket using the **frontmatter template** and a clear problem/goal statement.
2. Populate `related-mental-model` only with mental-model stems already consulted or explicitly allowed during this procedure, without `.md`; omit when none applied.

### 3. Shape

1. For `epic`: write only scope, non-scope, child ticket board, cross-child decisions, and done/drop/defer criteria.
2. For `epic`: reference existing/planned children.
3. For `epic`: if child creation or edit is needed, finish the epic edit first, then start a separate `lead-write-ticket` invocation scoped to the child.
4. For `workset`: write context, focus, existing included tickets listed by stem/path with current status and role, and exit criteria.
5. For `workset`: list planned-but-not-created work in `## Planned References` with a provisional label, intended role, and creation condition; do not assign status/path or `parent:`.
6. For `workset`: if the user also requested included actionable ticket creation or edits, record planned references unless explicit cascade owns those ticket edits.
7. For `workset` cascade: create or edit actionable tickets in separate commits, then update the workset to reference final paths/statuses; never add `parent:` because of workset inclusion.
8. For actionable tickets, choose shape through `judge: ticket-shape`; default to one `Phase 1`.
9. For each actionable phase, run **Apply Ticket Content**.
10. For actionable tickets, note inter-phase dependencies explicitly.

### 4. Ready Guard

1. For `workset`, choose `idea/` or `todo/`; do not create or move it into `ready/`.
2. For `ready/`, defer focus entry until **Spec-address Check** passes.

## On: Edit Ticket

### 1. Load

1. Read the ticket first when it was not already loaded.

### 2. Apply Change

1. Apply the requested change: phase update, content update, or status move.
2. For `epic`, keep edits board-level.
3. For `epic` implementation detail, finish the epic edit first, then start a separate `lead-write-ticket` invocation scoped to the child ticket.
4. For `workset`, keep edits to non-hierarchical operating context and included-ticket notes.

### 3. Move

1. For moves, use `{{.McpNamespace}}/tickets.close(stem, status)` for done/dropped, or `{{.McpNamespace}}/tickets.move(stem, to)` for idea/todo/ready; fall back to native `git mv` when MCP tools are unavailable.
2. For `.done/` moves via native `git mv`, add `completed:` date in frontmatter; `tickets.close` writes this automatically.
3. If the only requested change is moving a `workset` to `ready/`, make no file changes, skip commit, report the refusal, and emit the unchanged `Ticket:` path.
4. For `workset` moves to `ready/` with other edits, do not move status; keep only valid content edits.
5. For proceed-routed `todo/` -> `ready/` promotion, defer the move until **Spec-address Check** passes.

### 4. Shape

1. For actionable shape or phase changes, apply `judge: ticket-shape`.
2. For each changed actionable phase, run **Apply Ticket Content**.

## On: Apply Ticket Content

1. Capture confirmed goals, contracts, and agreed API/type/event/UI sketches.
2. Capture completion boundary and deferred scope.
3. Capture confirmed constraints and rationale.
4. Capture settled implementation strategy decisions; include suggested strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract.
5. Capture rejected alternatives.
6. Capture confirmed forward-compatibility guardrails.
7. Capture verification expectations.
8. Capture enough detail that a fresh implementer can build the intended result without filling settled gaps.
9. Exclude source-local edit notes unless settled constraints.

## On: Open Decision Queue

1. If the user has not already approved persistence, ask whether to persist the discussion into tickets or specs; stop with no edits when they decline or do not answer.
2. List every unresolved or unconfirmed item that could affect ticket text: mechanism decisions, rejected alternatives, future-scope hints, Result Forward notes, focus "Next" lines, and comment/note proposals.
3. Create or refresh the visible Open Decision Queue using the task-list guidance appended to this playbook.
4. Ask about one queue item at a time; after each answer, update the visible queue status before asking the next item.
5. Continue only when every queue item is confirmed, rejected, or explicitly deferred.
6. Write confirmed items only; omit rejected, deferred, or unanswered items unless the user explicitly approves recording their status.
7. Never write draft decisions for later correction.

## On: Intent Review

1. Re-read the written/edited ticket against the conversation and cross-ticket decision review.
2. Check completion boundaries, decisions, constraints, rejected alternatives, forward-compatibility guardrails, verification expectations, and agreed strategy that constrains implementation.
3. Check whether agreed API/type/event/UI sketches were preserved literally, not prose-flattened.
4. Check whether the ticket distorts or omits discussed intent.
5. Check whether a fresh implementer could build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket; if yes, capture the missing settled decision.
6. Check whether related-ticket decisions that constrain this implementation slice were captured.
7. For `epic`, check that detailed implementation material stayed out of the epic and moved to a child-ticket invocation.
8. For `workset`, check that it did not create parent-child semantics, decomposition ownership, or implementation phases.
9. Check that no unconfirmed mechanism choice, future-scope hint, Result Forward note, or focus "Next" line was written.
10. Fix confirmed gaps in-place; return unconfirmed gaps to the Open Decision Queue instead of writing them.
11. Present a brief correction summary, or confirm nothing was missed.

## On: Spec-address Check

### 1. Scope

1. Skip `epic`, `research`, and `workset`.
2. Treat requested `todo/` -> `ready/` promotion as `ready/` for this check.
3. Apply `judge: spec-address-gate` before committing a new `ready/` ticket, a `ready/` promotion, or a `ready/` focus entry.
4. If Spec-address Check fails, do not move the ticket to `ready/` or add a `Ticket Focus` entry; restore pre-invocation edits unless valid non-ready edits were explicitly requested, then report the kept or reverted paths.

### 2. Todo Handling

1. For `todo/`, preserve existing `spec:` links as optional recovery hints.
2. For `todo/`, leave the ticket in `todo/`; it may be shown as a non-ready attention item, but implementation must still route through proceed.

### 3. Ready Addressing

1. For `ready/`, use `{{.McpNamespace}}/specs.find` or `{{.McpNamespace}}/specs.status` to confirm existing `spec:` and `spec-remove:` stems.
2. For `ready/`, keep confirmed existing stems in frontmatter.
3. For `ready/`, when neither confirmed `spec:` nor `spec-remove:` stems address the phase, write or update `## Spec Impact`.
4. `## Spec Impact` must name the target spec area, expected caller-visible change, and `Contract-first spec: yes|no`.
5. If `Contract-first spec: yes`, call `{{.McpNamespace}}/playbook.print(name: "lead-write-spec")` and execute the returned procedure inline, re-check the created or updated stem, and list it in `spec:`.
6. After a contract-first spec is listed in `spec:`, remove redundant `## Spec Impact` text or keep only closeout notes not covered by the spec.
7. If neither confirmed stems nor `## Spec Impact` addresses a phase, apply `judge: missing-spec-address`.

### 4. Ready Focus

1. `Ticket Focus` may list selected active attention items; only `ready/` entries are direct implementation targets.
2. For non-ready focus entries, use `` `stem` (`status`, `<role>`) - one-line purpose and why it is in focus; not implementation-ready ``.
3. For `ready/`, remind that implementation commits should include a `## Spec` section for existing stems or the doc closeout should resolve `## Spec Impact`.
4. For `ready/`, ensure `ai-docs/_index.md ## Ticket Focus` has `` `stem` - one-line purpose, readiness, and dependency notes ``.
5. For deferred `todo/` -> `ready/` promotion, use `{{.McpNamespace}}/tickets.move(stem, to: "ready")` or native `git mv` as fallback; then commit.

## On: Output Handoff

1. For `epic`, do not suggest proceeding on the epic path.
2. For `epic`, suggest creating, promoting, or proceeding a child ticket.
3. For `workset`, suggest one concrete next action: proceed/promote an existing included actionable ticket, or create a planned reference as a new actionable ticket; never suggest proceeding or promoting the workset itself.
4. For actionable tickets with valid spec addressing, suggest `{{.SkillNamespace}}:lead-proceed`; when spec addressing blocks readiness, report the blocker and omit a proceed suggestion.
5. State that proceed routes to implementation readiness; the lead-implement procedure resolves plan depth and execution mode.
6. Emit the current ticket path on its own final line for every create, edit, move, or promotion: `Ticket: ai-docs/tickets/<status>/<stem>.md`.
7. For `epic` or `workset`, state that the path is a board artifact, not an implementation target.
8. Preserve the final `Ticket:` line; callers such as `{{.SkillNamespace}}:lead-proceed` capture this path from prefix-stage output.

## On: Cross-ticket decision review

1. Identify the target's parent/epic relationships, any worksets that list the target, relevant co-listed workset tickets, child board entries, and explicitly related tickets when those links are available.
2. Read only graph tickets that may contain decisions constraining the target's implementation scope.
3. Record binding cross-ticket decisions in the target as scope, constraints, forward-compatibility guardrails, rejected alternatives, verification expectations, or phase dependencies.
4. Do not copy unrelated future-phase detail; preserve only decisions that the current implementation could violate or block.
5. If the same decision changes another active ticket's role, include that ticket in this logical edit; otherwise leave related tickets untouched.
6. Keep epics board-level; move implementation constraints into the relevant child ticket or phase.
7. Keep worksets non-hierarchical; move implementation constraints into the relevant included actionable ticket or phase.

## On: Cascade Edit

### 1. Select Targets

1. Identify the impacted ticket graph: parent epic, containing epic, worksets that list selected targets, child tickets, related active tickets, and `_index.md` active inventory when it lists edited tickets.
2. Select edit targets from that graph; do not edit merely-related tickets whose role is unaffected by the propagated decision.
3. Read each selected target before editing.

### 2. Apply Propagation

1. Keep epics to scope, non-scope, child ticket board, cross-child decisions, and completion criteria.
2. Keep worksets to context, included tickets, focus, and exit criteria; never add child relationships from workset inclusion.
3. Put implementation decisions, constraints, rejected alternatives, and phases into actionable tickets.
4. Do not promote tickets to `ready/` unless the user explicitly asks for ready promotion or routes through `{{.SkillNamespace}}:lead-proceed`.
5. For any selected target entering `ready/`, run Spec-address check before commit.

### 3. Verify and Report

1. Run Intent review across the edited set and commit one logical documentation unit when the edits are one decision propagation.
2. Report edited ticket paths; if exactly one actionable implementation ticket is the natural next target, emit `Next Ticket: <path>` before the final artifact line.
3. Always emit the edited/current ticket path as the final `Ticket:` line.

## Judgments

### judge: ticket-category

`epic`: hierarchical milestone or decomposition board whose child tickets collectively deliver one parent outcome.
`workset`: non-hierarchical operating context grouping independent or cross-cutting tickets for coordination, sequencing, or focus.
`research`: investigation or findings capture without phases.
`bug`/`feat`/`refactor`/`chore`: actionable implementation unit with phases and verification.
Default: if the user asks for a board without decomposition ownership, choose `workset`; if they ask for parent outcome breakdown, choose `epic`.

### judge: spec-address-gate

Trigger: non-`epic`, non-`research`, non-`workset` ticket creation or move into `ready/`.
Ungated: `idea/` creation and `idea/` -> `todo/` triage.
Find addressing: identify existing `spec:` or `spec-remove:` stems, or write a ticket-local `## Spec Impact` section.
Contract-first: call `{{.McpNamespace}}/playbook.print(name: "lead-write-spec")` and execute inline only when `judge: contract-first-spec` is yes.
Stop: no stem or `## Spec Impact` can address the behavior, the lead-write-spec procedure failed, or the behavior is too underspecified; name the blocker.

### judge: initial-status

`idea/`: topic is exploratory or underspecified.
`todo/`: scope and goal are accepted actionable backlog, or the ticket is a non-actionable coordination artifact.
`ready/`: spec-addressed implementation-ready status.
`todo/` `spec:` links: optional recovery hints.
Uncertain: prefer `idea/`.

### judge: contract-first-spec

Yes: planned behavior must be visible and stable before implementation begins.
Usually yes: externally consumed schemas, CLI/API contracts, file or wire formats, cross-skill routing contracts, or multi-ticket planned behavior.
No: ticket only needs a spec area for post-implementation closeout, final behavior will be refined during implementation, or planned text would mostly restate the ticket phase.

### judge: cascade-ticket-edit

Trigger: user asks to cascade broadly, reorganize a board and children, or update parent and child tickets beyond target-constraining decisions.
Do not trigger: a ticket merely has `related:` links or default cross-ticket decision review applies.

### judge: needs-open-decision-queue

Trigger: discussion-derived persistence or ticket cleanup would write any mechanism decision, rejected alternative, future-scope hint, Result Forward note, focus "Next" line, or note/comment proposal not already explicitly confirmed by the user.
Trigger: the user asks to persist a discussion whose open items are mixed with confirmed decisions.
Do not trigger: mechanical status moves, already-confirmed ticket edits, or creation from a fully specified user request with no unresolved discussion residue.

### judge: ticket-shape

Artifact role: keep epics board-level and worksets operating-context-only; put implementation detail in actionable tickets.
Scope keep: decisions, constraints, and agreed API/type/event/UI sketches.
Scope exclude: source-local edit notes unless settled constraints.
Ticket split: only when board, ticket, and implementation-unit roles are mixed, or unrelated increments belong in separate actionable tickets.
Phase default: actionable tickets use one `Phase 1`.
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
