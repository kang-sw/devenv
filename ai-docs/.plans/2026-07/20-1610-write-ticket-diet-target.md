# lead-write-ticket — diet target (golden reference)

**Status:** NON-SHIPPED reference. Not a manifest/rsrc file; lives in `.plans/`
on purpose — `wsrsrc.GenerateManifest` walks the whole rsrc tree and would
bundle + hash any `.md` dropped next to the live playbook, failing
`manifest_shipped_test.go`. Referenced from `260701-feat-write-ticket-lever-b-mcp-tools`.

**What this is.** The target body for
`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` after the Lever-B
work in 260701 lands. Live file today: **449 lines / ~10K tokens** (re-inflated
from a dieted 277 by `5c707ce9`, which added the status-split sage gate as prose
state-machine). This target: **~160 lines / ~3K tokens**. It assumes **both**
260701 phases ship; see the Ledger for what reverts if only one does.

The diet has two engines, matching the epic's two levers plus one addition the
user asked for:

1. **Lever B (MCP-ification)** — deterministic state machines leave the prose.
2. **Judgment delegation** — over-specified meta-cognition a competent
   (sonnet-level) model performs anyway is deleted, not restated. This is the
   riskier engine: the test is *"does removing this line change what a careful
   model actually does?"* Where the answer is "no", the line is scaffolding for
   a reader that does not need it.

## Diet Ledger

| Live block (lines) | Disposition | Destination / rationale |
|---|---|---|
| `On: Sage Review Gate` + `Design Review Stage` + `Completeness Review Stage` + `Ready-promotion Aggregation` + 3 `Blocked Section Template`s (~235 lines, ~60% of the file) | **→ MCP** | Two new tools (below). Pure functions of known inputs: posture values, legacy-field migration, `config.show` fallback, category×stage matrix, standalone/combined mode, verdict aggregation, frontmatter value, commit title, Blocked-section rendering. No model judgment in any of it. |
| Reviewer spawn (embedded in the two Stage blocks) | **Kept, deduped** | `On: Reviewer Spawn`, ~3 lines. Genuinely lead-owned: an MCP tool cannot spawn a subagent. The two near-identical Stage spawn procedures collapse to one parameterized block. |
| Category×stage "keep this table in sync with the Go side" note (135-137) | **→ MCP (deleted)** | The doc was mirroring `tickets.move`'s Go category detection. Once `sage_gate` owns the matrix, the drift class disappears with the note. |
| `On: Apply Ticket Content` (capture list) | **→ MCP** | `tickets.checklist(type, phase:"content")` returns the list as data; playbook installs one `todo.append`. 260701 Phase 1. |
| `On: Intent Review` (check list + "fresh implementer" test) | **→ MCP** | `tickets.checklist(type, phase:"intent")`; one `todo.append`. 260701 Phase 1. The generative test travels in the todo `Instruction`. |
| Capture category list stated in Invariants **and** Apply Ticket Content **and** Doctrine | **Judgment delegation** | Stated **once** in Invariants/Capture. The other two were restatements; a model reading the canonical list does not need it echoed. |
| `git mv` fallback caveat repeated in Movement invariant, `On: Move`, `On: Cross-ticket` | **Judgment delegation** | Stated once in Movement invariant. |
| `On: Intent Review` sub-checks (literal-sketch check, no-unconfirmed check, epic/workset shape check) | **Judgment delegation** | Folded into the checklist tool's `Instruction`; the standalone bullets were a model re-deriving what the "fresh implementer" test already forces. |
| 8 `judge:` tables | **Kept** | Soft judgments the MCP tools cannot compute (category, shape, contract-first, spec-address gate, etc.). Layer 3. Wording tightened only. |
| `On: Cross-ticket decision review`, `On: Cascade Edit` | **Kept** | Graph-walk choreography; no MCP tool owns it. Already deduped in the 277-line pass. |
| `On: Output Handoff` table | **Kept** | Prevents a specific wrong execution (suggesting `proceed` on a board artifact). Not inferable-for-free. |
| `On: Open Decision Queue` | **Kept** | Interactive human gate; explicitly out of scope for the checklist-as-todo migration (already guarded by `judge: needs-open-decision-queue`). |

### New MCP tool contracts (define in 260701)

- `ws/tickets.sage_gate(stem, landing)` → `{ action, ask_prompt?, reviewers?, mode? }`.
  `action` ∈ `skip | stop_blocked | ask | run`. Resolves posture (incl. legacy
  `sage-review:` migration and `config.show` fallback), applies the category×stage
  matrix, and picks standalone vs combined mode. For `ask`, returns the exact
  question; the lead relays the user's answer back via a follow-up call.
  For `run`, returns which reviewer(s) to spawn. **Does not** spawn — that stays
  lead-owned.
- `ws/tickets.sage_record(stem, stage, verdicts)` → aggregates design+completeness
  verdicts (incl. the `resolution: missing` escalation), writes the frontmatter
  posture, renders the Blocked section from the Go-owned template when blocked,
  and commits with the canonical title. Returns the applied posture + commit ref.

**Partial-ship fallback:** if only 260701 Phase 1 (checklist) ships, keep the
live sage block verbatim and adopt only the `tickets.checklist` edits (~449 →
~430). If only Phase 2 (sage tools) ships, keep the live capture/intent prose
and adopt only the sage collapse (~449 → ~230). The ~160-line target needs both.

---

```markdown
---
kind: print
includes:
  - task-list
---
# Write Ticket

Target: user request

## Invariants

Capture
- Persist only user-confirmed decisions; resolve the Open Decision Queue before writing any discussion-derived content.
- Capture enough settled intent — decisions, contracts, literal API/type/event/UI sketches, rejected alternatives, constraints, forward-compatibility guardrails, verification expectations — that a fresh implementation session recovers intent without inventing product, workflow, API, or verification decisions.

Board artifacts
- Epic and workset bodies stay within their `tickets.template` skeleton's board-level sections; implementation detail moves to a child/included-ticket invocation.
- Worksets stay in `idea/`/`todo/`; never create or move a workset into `ready/`.
- Never add, remove, or change `parent:` based on workset inclusion.

Scope
- Read only edit-target tickets, graph tickets needed for binding-decision review, required conventions, focus updates, and explicitly routed spec/mental-model checks.
- Ready promotion requires spec addressing (`judge: spec-address-gate`), not mandatory planned spec text.

Movement
- Prefer `{{.McpNamespace}}/tickets.move` / `tickets.close` / `tickets.create` over `git mv` or manual file edits; fall back only when the MCP tool is unavailable or errors.

## On: invoke

### 1. Route
1. Call `{{.McpNamespace}}/convention.read(name: "ticket-conventions")`.
2. If `user request` references an existing ticket, read it.
3. Classify category (`judge: ticket-category`); for a new ticket, choose initial status (`judge: initial-status`). A proceed-routed actionable `todo/` ticket sets the change to ready promotion.
4. Call `{{.McpNamespace}}/tickets.template(type: "<category>")`.
5. Apply `judge: cascade-ticket-edit`; if it fires, run **Cascade Edit** and stop single-target routing.
6. Actionable creation or edits: run **Cross-ticket decision review** before drafting.
7. Workset: verify each included ticket's path, status directory, and role; convert missing tickets to planned references or stop on a blocker.

### 2. Consent Gate
1. Apply `judge: needs-open-decision-queue`; if it fires, run **Open Decision Queue** before editing ticket text.

### 3. Populate
1. New ticket: `{{.McpNamespace}}/tickets.create(session_key: <lead key>, stem: "<category>-<name>", initial_state: "<initial-status>")`; fall back to manual file creation only on tool unavailability/error.
2. Existing ticket: apply the requested phase/content/status change to the loaded body.
3. Call `{{.McpNamespace}}/tickets.checklist(type: "<category>", phase: "content")`; install one todo via `todo.append` carrying the returned capture checklist; satisfy it while filling the skeleton and check it only on completion.
4. Populate `related-mental-model` only with stems consulted or explicitly allowed this run (omit `.md`; omit the field when none).
5. Actionable: apply `judge: ticket-shape`.
6. Epic/workset detail belonging to a child/included ticket: stop; start a separate invocation scoped to that ticket.
7. Workset: list not-yet-created work in `## Planned References` (provisional label, intended role, creation condition; no status/path/`parent:`). Cascade-owned actionable edits happen in separate commits, then update the workset to final paths.
8. Status move: see **Move**.

### 4. Verify
1. Call `{{.McpNamespace}}/tickets.checklist(type: "<category>", phase: "intent")`; install one todo via `todo.append` carrying the returned intent-review checklist; satisfy it against the written ticket, fix confirmed gaps in-place, and return unconfirmed gaps to the Open Decision Queue.
2. Landing status `ready/`: run **Spec-address Check**.

### 5. Commit
1. If no file changed because a requested move was refused, skip commit.
2. `{{.McpNamespace}}/git.commit(paths: ["<edited-ticket-paths>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when focus changed. Follow-up invocations own their own commits and outputs.

### 6. Sage Review Gate
1. Call `{{.McpNamespace}}/tickets.sage_gate(stem, landing)` and follow the returned action: `skip` (done), `stop_blocked` (report and stop), `ask` (relay the returned question, then call again with the answer), `run` (spawn the returned reviewer(s) via **Reviewer Spawn**). Posture, legacy-field migration, config fallback, and category×stage selection are tool-owned.
2. After producing each requested verdict, call `{{.McpNamespace}}/tickets.sage_record(stem, stage, verdicts)`; it aggregates, writes frontmatter, renders any Blocked section, and commits. Follow its returned confirmation.

### 7. Handoff
1. Run **Output Handoff**.

## On: Reviewer Spawn

For each reviewer named by `tickets.sage_gate`:
1. Call `{{.McpNamespace}}/playbook.render(name: "ticket-reviewer-design")` or `"ticket-reviewer-completeness")`; it returns a file path. Do not read the rendered file in the lead context.
2. Spawn a native subagent with prompt: `Read <rendered-path> as your system prompt. Ticket path: <ticket-path>`.
3. Parse `verdict:` (`pass`, `concern`, or `block`) from the result; return it to `tickets.sage_record`.

## On: Move

Prefer `{{.McpNamespace}}/tickets.move` / `tickets.close`; fall back to `git mv` only on tool unavailability/error.
1. `.done/` via `tickets.close` writes `completed:` automatically; a `git mv` fallback adds it manually.
2. Workset → `ready/` as the only requested change: make no changes, skip commit, report the refusal, emit the unchanged `Ticket:` path.
3. Workset → `ready/` combined with other edits: keep valid content edits, do not move status.
4. `todo/` → `ready/` promotion: move only after **Spec-address Check** passes.

## On: Open Decision Queue
1. If persistence is not yet approved, ask whether to persist the discussion into tickets or specs; stop with no edits on decline or no answer.
2. List every unresolved item that could affect ticket text: mechanism decisions, rejected alternatives, future-scope hints, Result Forward notes, focus "Next" lines, comment/note proposals.
3. Create or refresh the visible Open Decision Queue using the appended task-list guidance.
4. Ask one item at a time; update the visible queue status after each answer.
5. Continue only when every item is confirmed, rejected, or explicitly deferred.
6. Write confirmed items only; never write draft decisions for later correction.

## On: Spec-address Check

Applies per `judge: spec-address-gate` (a `todo/` → `ready/` promotion counts as `ready/`).
1. `todo/` (not promoting): existing `spec:` links are optional recovery hints only.
2. `ready/`: confirm `spec:`/`spec-remove:` stems via `{{.McpNamespace}}/specs.find` or `specs.status`; keep confirmed stems.
3. No confirmed stem addresses a phase: write `## Spec Impact` per the skeleton. When `judge: contract-first-spec` is yes: run `{{.McpNamespace}}/playbook.print(name: "lead-write-spec")` inline, list the resulting stem in `spec:`, and drop redundant `## Spec Impact` text.
4. Neither a confirmed stem nor `## Spec Impact` addresses a phase: apply `judge: missing-spec-address` and stop — no `ready/` move, no focus entry; restore pre-invocation edits unless valid non-ready edits were requested, then report kept/reverted paths.
5. On pass, `ready/`: ensure `ai-docs/_index.md ## Ticket Focus` carries `` `stem` - one-line purpose, readiness, and dependency notes ``. Non-ready attention entry: `` `stem` (`status`, `<role>`) - one-line purpose and why in focus; not implementation-ready ``.

## On: Output Handoff

| Category | Handoff |
|---|---|
| `epic` | Suggest creating, promoting, or proceeding a child ticket; never suggest proceeding the epic path itself. |
| `workset` | Suggest one concrete next action on an included ticket; never suggest proceeding or promoting the workset itself. |
| actionable, spec-addressed | Suggest `{{.SkillNamespace}}:lead-proceed`; note it resolves plan depth and execution mode. |
| actionable, blocked on spec addressing | Report the blocker; omit the proceed suggestion. |

For `epic`/`workset`, state that the path is a board artifact, not an implementation target.

Always emit the current ticket path on its own final line: `Ticket: ai-docs/tickets/<status>/<stem>.md`. Preserve this line exactly — callers capture the path from prefix-stage output.

## On: Cross-ticket decision review

Applies to a single edit target; **Cascade Edit** reuses this across targets.
1. Identify the target's parent/epic relationships, worksets listing it, co-listed tickets, child board entries, and explicitly related tickets.
2. Read only graph tickets that may constrain the target's implementation scope.
3. Record binding cross-ticket decisions in the target as scope, constraints, guardrails, rejected alternatives, verification expectations, or phase dependencies; do not copy unrelated future-phase detail.
4. If the same decision changes another active ticket's role, include that ticket in this logical edit; otherwise leave related tickets untouched.
5. Keep epics board-level and worksets non-hierarchical; move implementation constraints into the relevant child/included ticket.

## On: Cascade Edit
1. Select targets via the same graph identification, extended to `_index.md` active inventory; select only targets whose role the decision affects; read each before editing.
2. Apply per-target decision recording per **Cross-ticket decision review**.
3. Do not promote a target to `ready/` unless the user asked or routed through `{{.SkillNamespace}}:lead-proceed`; run **Spec-address Check** before commit for any target entering `ready/`.
4. Run **Verify** across the edited set; commit one logical unit when the edits are one decision propagation.
5. Report edited paths; if exactly one actionable ticket is the natural next target, emit `Next Ticket: <path>` before the final `Ticket:` line.

## Judgments

### judge: ticket-category
`epic`: hierarchical milestone/decomposition board whose children deliver one parent outcome.
`workset`: non-hierarchical operating context grouping independent/cross-cutting tickets.
`research`: investigation/findings capture without phases.
`bug`/`feat`/`refactor`/`chore`: actionable implementation unit with phases and verification.
Default: board without decomposition ownership → `workset`; parent-outcome breakdown → `epic`.

### judge: spec-address-gate
Trigger: non-`epic`, non-`research`, non-`workset` creation or move into `ready/`.
Ungated: `idea/` creation, `idea/` → `todo/` triage.
Mechanics: **On: Spec-address Check**; stop condition is `judge: missing-spec-address`.

### judge: initial-status
`idea/`: exploratory/underspecified. `todo/`: accepted actionable backlog or non-actionable coordination artifact. `ready/`: spec-addressed implementation-ready. Uncertain → `idea/`.

### judge: contract-first-spec
Yes: planned behavior must be visible and stable before implementation (externally consumed schemas, CLI/API contracts, file/wire formats, cross-skill routing, multi-ticket planned behavior).
No: spec area only for closeout, behavior refined during implementation, or planned text would restate the phase.

### judge: cascade-ticket-edit
Trigger: user asks to cascade broadly, reorganize a board and children, or update parent+child beyond target-constraining decisions.
Do not trigger: mere `related:` links or default cross-ticket decision review.

### judge: needs-open-decision-queue
Trigger: discussion-derived persistence would write any mechanism decision, rejected alternative, future-scope hint, Result Forward note, focus "Next" line, or note proposal not already user-confirmed; or the user asks to persist a discussion mixing open items with confirmed decisions.
Do not trigger: mechanical status moves, already-confirmed edits, or creation from a fully specified request with no unresolved residue.

### judge: ticket-shape
Split ticket only when board/ticket/implementation-unit roles are mixed or unrelated increments belong apart. Phase default: one `Phase 1`. Phase unit: one reviewable slice a fresh session can finish, review, verify, hand off. Add phases only when review/verification/rollback/dependency boundaries differ.

### judge: missing-spec-address
Trigger: a phase implements caller-visible behavior with no confirmed stem, `spec-remove:`, or `## Spec Impact` after `judge: spec-address-gate` runs.
Action: stop the authoring flow; name the uncovered phase and the missing-traceability blocker.

## Doctrine

A ticket is the primary context-recovery artifact. Every choice optimizes for
**recoverability of intent**: capture decisions, constraints, and rejected
alternatives with enough settled detail that downstream skills do not fill gaps
with a different product, workflow, API, or verification contract. When unsure
whether a settled decision is needed, preserve it in contract terms; do not
preserve tentative discussion or source-local tactical notes unless they became
constraints.
```
