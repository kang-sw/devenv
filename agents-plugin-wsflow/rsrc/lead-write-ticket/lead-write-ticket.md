---
kind: print
includes:
  - task-list
---
# Write Ticket

Target: user request

## Invariants

Capture
- Preserve settled decisions, contracts, agreed API/type/event/UI sketches, rejected alternatives, constraints, forward-compatibility guardrails, and verification expectations — enough for a fresh implementation session to recover intent without inventing missing product, workflow, API, or verification decisions.
- Persist only user-confirmed decisions; resolve the Open Decision Queue before writing any discussion-derived content.

Board artifacts
- Epic and workset bodies stay within their `tickets.template` skeleton's board-level sections; implementation detail moves to a separate `lead-write-ticket` invocation scoped to the child/included ticket.
- Worksets stay in `idea/`/`todo/`; never create or move a workset into `ready/`.
- Never add, remove, or change `parent:` based on workset inclusion.

Scope
- Read only ticket files selected as edit targets, graph tickets needed for binding-decision review, required conventions, focus updates, and explicitly routed spec/mental-model checks.
- Review related-ticket decisions by default; cascade (`judge: cascade-ticket-edit`) only for broader board or multi-ticket propagation.
- Ready promotion requires spec addressing (`judge: spec-address-gate`), not mandatory planned spec text.

Movement
- Prefer `{{.McpNamespace}}/tickets.move` / `tickets.close` / `tickets.create` over native `git mv` or manual file edits; fall back only when the MCP tool is unavailable or errors.

## On: invoke

### 1. Route

1. Call `{{.McpNamespace}}/convention.read(name: "ticket-conventions")`.
2. If `user request` references an existing ticket, read it.
3. Classify category (`judge: ticket-category`); for a new ticket, choose initial status (`judge: initial-status`). For a proceed-routed actionable `todo/` ticket, set the requested change to ready promotion.
4. Call `{{.McpNamespace}}/tickets.template(type: "<category>")` for the typed body skeleton.
5. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop ordinary single-target routing.
6. For actionable creation or edits, run **Cross-ticket decision review** before drafting.
7. For workset creation or edits, verify each existing included ticket's path, current status directory, and stated role; convert missing tickets to planned references or stop on a blocker.

### 2. Consent Gate

1. Apply `judge: needs-open-decision-queue`; if it fires, run **Open Decision Queue** before editing ticket text.

### 3. Populate

1. New ticket: call `{{.McpNamespace}}/tickets.create(session_key: <lead key>, type: "<category>", title: "<title>", status: "<initial-status>")`; fall back to manual file creation only when the tool is unavailable or errors.
2. Existing ticket: apply the requested change — phase update, content update, or status move — directly to the loaded body.
3. Fill the loaded skeleton with a clear problem/goal statement per **Apply Ticket Content**.
4. Populate `related-mental-model` only with mental-model stems already consulted or explicitly allowed during this procedure (omit `.md`; omit the field when none applied).
5. For actionable tickets, apply `judge: ticket-shape` for phase count and granularity.
6. For epic/workset detail that belongs to a child or included ticket: stop this invocation; start a separate `lead-write-ticket` invocation scoped to that ticket.
7. For workset: list not-yet-created work in `## Planned References` with a provisional label, intended role, and creation condition — no status/path/`parent:` until a real ticket exists. If the user also requested included actionable creation or edits, record planned references unless explicit cascade owns those edits; for cascade, create or edit the actionable tickets in separate commits, then update the workset to reference final paths/statuses.
8. For a status move, see **Move**.

### 4. Verify

1. Run **Intent Review**.
2. If landing status is `ready/` (including a requested `todo/` → `ready/` promotion), run **Spec-address Check**.

### 5. Commit

1. If no file changed because a requested move was refused, skip commit.
2. Commit edited paths with `{{.McpNamespace}}/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when focus changed; separate follow-up invocations own their own commits and outputs.

### 6. Sage Review Gate

1. Run **Sage Review Gate**.

### 7. Handoff

1. Run **Output Handoff**.

## On: Move

Prefer `{{.McpNamespace}}/tickets.move` / `tickets.close` over native `git mv`; fall back only when the MCP tool is unavailable or errors.

1. `.done/` via `tickets.close` writes `completed:` automatically; a native `git mv` fallback requires adding it manually.
2. Workset → `ready/` as the only requested change: make no file changes, skip commit, report the refusal, and emit the unchanged `Ticket:` path.
3. Workset → `ready/` combined with other edits: do not move status; keep only the valid content edits.
4. Deferred `todo/` → `ready/` promotion: move only after **Spec-address Check** passes.

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

Applies per `judge: spec-address-gate` (a requested `todo/` → `ready/` promotion counts as `ready/` for this check).

1. For `todo/` (not promoting): existing `spec:` links are optional recovery hints only; implementation still routes through proceed.
2. For `ready/`: confirm existing `spec:`/`spec-remove:` stems via `{{.McpNamespace}}/specs.find` or `specs.status`; keep confirmed stems as-is.
3. If no confirmed stem addresses a phase: write or update `## Spec Impact` per the loaded skeleton's field guidance. When `judge: contract-first-spec` is yes: print and execute `{{.McpNamespace}}/playbook.print(name: "lead-write-spec")` inline, list the resulting stem in `spec:`, and drop redundant `## Spec Impact` text.
4. If neither a confirmed stem nor `## Spec Impact` addresses a phase: apply `judge: missing-spec-address` and stop — do not move to `ready/` or add a `Ticket Focus` entry; restore pre-invocation edits unless valid non-ready edits were explicitly requested, then report the kept or reverted paths.
5. On pass, for `ready/`: ensure `ai-docs/_index.md ## Ticket Focus` carries `` `stem` - one-line purpose, readiness, and dependency notes ``. For a non-ready attention entry, use `` `stem` (`status`, `<role>`) - one-line purpose and why it is in focus; not implementation-ready ``.

## On: Output Handoff

| Category | Handoff |
|---|---|
| `epic` | Suggest creating, promoting, or proceeding a child ticket; never suggest proceeding the epic path itself. |
| `workset` | Suggest one concrete next action on an included ticket (proceed/promote an existing one, or create a planned reference as a new actionable ticket); never suggest proceeding or promoting the workset itself. |
| actionable, spec-addressed | Suggest `{{.SkillNamespace}}:lead-proceed`; note that proceed resolves plan depth and execution mode. |
| actionable, blocked on spec addressing | Report the blocker; omit the proceed suggestion. |

For `epic` or `workset`, state that the path is a board artifact, not an implementation target.

Always emit the current ticket path on its own final line: `Ticket: ai-docs/tickets/<status>/<stem>.md`. Preserve this line exactly — callers such as `{{.SkillNamespace}}:lead-proceed` capture the path from prefix-stage output.

## On: Sage Review Gate

1. If landing status is `idea/`, skip this gate.
2. Inspect the ticket frontmatter `sage-review:` posture.
3. If posture is `skipped`, skip this gate.
4. If posture is `completed`, skip this gate; the ticket already has a completed sage review.
5. If posture is `blocked`, stop and report that the blocked sage review must be addressed before promotion.
6. If posture is `recommended`, ask the user "Run sage review for this ticket?".
   - If user declines: add `sage-review: skipped` to ticket frontmatter, commit with
     `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "chore(sage): skip sage review", ai_context: ["user declined sage review in ask mode"])`,
     then skip the rest of this gate.
7. If posture is `required`, run sage review without asking.
8. If posture is missing or `pending`, treat it as legacy unresolved state: call `{{.McpNamespace}}/config.show()`, resolve `sage_review` as `skipped` for `off`/empty/unset, `recommended` for `ask`, or `required` for `auto`, write that posture to ticket frontmatter, then continue from the matching posture rule above.
9. Spawn both reviewers in parallel:
   `playbook.render` returns a file path. Include that path in the subagent's
   kickoff prompt; the subagent reads the file as its system prompt. Do not read
   the rendered file in the lead context.
   a. Render `ticket-reviewer-design`: call `{{.McpNamespace}}/playbook.render(name: "ticket-reviewer-design")`;
      spawn native subagent with prompt: `Read <rendered-path> as your system prompt. Ticket path: <ticket-path>`.
      Capture design verdict result.
   b. Render `ticket-reviewer-completeness`: call `{{.McpNamespace}}/playbook.render(name: "ticket-reviewer-completeness")`;
      spawn native subagent with prompt: `Read <rendered-path> as your system prompt. Ticket path: <ticket-path>`.
      Capture completeness verdict result.
10. Parse `verdict:` from each result. Each reviewer result text contains a `verdict:` line
   whose value is one of `pass`, `concern`, or `block` (exhaustive set).
11. Apply aggregation:
   - Design `block` → final verdict is `block` regardless of completeness.
   - Design not-block and completeness `block` → final verdict is `block`.
   - Design `concern` and completeness `pass|concern` → default to `pass`; if ANY issue in
     either reviewer result has `resolution: missing`, elevate to `concern`. On `concern`,
     proceed to the pass-resolution step (do not block by default); lead may escalate to `block` if the missing
     decision is judged critical.
   - All `pass` → final verdict is `pass`.
12. If final verdict is `block`:
   a. Append a new `## Blocked (YYYY-MM-DD)` section at the end of the ticket body using the
      **Blocked Section Template**. If a `## Blocked` section already exists from a prior sage
      review cycle, replace it.
   b. Edit the ticket file directly to add or update `sage-review: blocked` in the frontmatter
      block; do not use a dedicated tool call.
   c. Commit with `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "docs(sage): block ticket on sage review", ai_context: ["sage review blocked: design and/or completeness issues"])`.
13. If final verdict is `pass` or `concern` resolved to pass:
   a. Edit the ticket file directly to add or update `sage-review: completed` in the frontmatter
      block; do not use a dedicated tool call.
   b. Commit with `{{.McpNamespace}}/git.commit(paths: ["<ticket-path>"], title: "docs(sage): mark sage review completed", ai_context: ["sage review passed"])`.

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

1. Keep each edit within its target's **Board artifacts** invariant.
2. Do not promote tickets to `ready/` unless the user explicitly asks for ready promotion or routes through `{{.SkillNamespace}}:lead-proceed`.
3. For any selected target entering `ready/`, run **Spec-address Check** before commit.

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

## Templates

### Blocked Section Template

```markdown
## Blocked (YYYY-MM-DD)

### Design Reviewer — <verdict>

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | <title> | <severity> | <resolution> |

### Completeness Reviewer — <verdict>

| # | Title | Severity |
|---|-------|----------|
| 1 | <title> | <severity> |
```

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives with enough settled detail that downstream skills do not fill gaps
with a different product, workflow, API, or verification contract. When unsure
whether a settled decision is needed for recovery, preserve the decision in
contract terms; do not preserve tentative discussion or source-local tactical
notes unless they became constraints.
