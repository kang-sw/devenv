# Brief: 260625-ws-session-state-machine — Phase 2 enter-call integration

## Intent

Wire the typed `ws.enter.*` session-state tools (shipped in Phase 1) into the four
lead skills that currently produce transcript-only routing or mode context, so that
mode context and a derived todo checklist are persisted to the session record and
become recoverable after context compaction. This slice inserts the enter calls
(plus one `agenda.set` refresh in salvage); it does NOT migrate any host task-list
to `ws.todo`, nor rewrite any existing recovery logic.

## Scope Boundary

In scope — edit exactly these four canonical playbook files:

- `agents-plugin/rsrc/lead-proceed/lead-proceed.md`
- `agents-plugin/rsrc/lead-implement/lead-implement.md`
- `agents-plugin/rsrc/lead-sprint/lead-sprint.md`
- `agents-plugin/rsrc/lead-salvage/lead-salvage.md`

Plus the required generated-artifact sync (wsflow rsrc mirror + both `manifest.json`).

Explicitly DEFERRED (out of this slice — do not touch):

- `lead-forge-spec` / `lead-forge-mental-model` host-task-list → `ws.todo` migration.
- `delegate-orientation.md` agenda/todo/enter contract documentation.
- Replacing lead-sprint's `Sprint-Edit:` commit-marker resume logic. We ADD
  `enter.sprint` alongside it; the marker logic stays intact.
- Any spec/ref/mental-model doc prose (handled by the lead's Doc pre-pass, not the
  implementer).

## Caller-Visible Contract

After this slice, rendering each of the four skills (`ws/playbook.print` or
`ws.workflow_manual`) shows an `enter.*` call at the specified procedural point, with
the listed arguments. No existing step is removed or reworded beyond the minimal
insertion. The enter call replaces the session-state todo list with the mode's
derived checklist (Phase 1 behavior); the host Markdown task list used by
lead-implement is unaffected and remains.

## Contract Instructions

All four skills are lead skills that hold a lead `session_key` (minted by
`ws.ferrule` / surfaced through the workflow manual). Reference it in prose exactly
as the existing house convention does — e.g. lead-tune writes
`{{.McpNamespace}}/config.prompt(session_key: <lead key>)`. Use the SAME
`session_key: <lead key>` placeholder form.

**CRITICAL namespace rule.** Write every tool reference with the namespace template
variable: `{{.McpNamespace}}/enter.implement(...)`, `{{.McpNamespace}}/enter.proceed(...)`,
`{{.McpNamespace}}/enter.sprint(...)`, `{{.McpNamespace}}/enter.salvage(...)`,
`{{.McpNamespace}}/agenda.set(...)`. NEVER hardcode `ws.enter.*` / `ws.agenda.*` —
the wsflow product-mode render test (`TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`
in `internal/mcp`) fails on bare `ws[/:]` namespace leakage. This matches how every
other tool ref in these files is written (`{{.McpNamespace}}/playbook.print`,
`{{.McpNamespace}}/git.status`, `{{.McpNamespace}}/mental_models.find`).

Per-skill exact placement and call shape (argument NAMES are fixed by the Phase 1 Go
schema; only `session_key` is required, all other fields are optional and omittable
when unknown):

### 1. lead-implement (`lead-implement/lead-implement.md`)

Insert ONE call as a NEW step in `### 3. Prep`, placed immediately AFTER current Prep
step 6 (branch-create), i.e. as the earliest point where branch / merge-target /
start-commit are all settled. Do NOT place it inside `### 1. Route` or `### 2. Emit
Implementation Verdict` (see Implementation Strategy Decisions for why this reconciles
the ticket's "immediately after Route" wording).

```
N. Call `{{.McpNamespace}}/enter.implement(session_key: <lead key>, delegation: <needs-delegation result>, plan_depth: <plan-depth>, branch_mode: <branch-mode>, review_alloc: <review-allocation>, current_branch: <current-branch>, merge_target: <merge-target>, start_commit: <implementation-start>, need_review: <review-allocation != lead-only>, need_doc: true, active_agents: [])` to record implement-mode context and replace the session-state todo list with the derived implement checklist. This mirrors (does not replace) the host task list created below.
```

- `need_review` = true unless review-allocation is lead-only.
- `need_doc` = true for the standard pipeline.
- `active_agents` = `[]` (the implementer is not spawned until Edit; richer
  active-agent capture is out of scope for this slice).

### 2. lead-proceed (`lead-proceed/lead-proceed.md`)

Insert ONE call in `### 4. Execute Verdict`, as its FIRST action, fired only when the
emitted `NEXT:` names a downstream skill (`{{.SkillNamespace}}:lead-discuss`,
`lead-write-ticket`, or `lead-implement`). Skip it for `NEXT: stop` (no resumable
handoff). The Routing Verdict (step 3) has already produced the field values.

```
1. If `NEXT:` names a downstream skill, call `{{.McpNamespace}}/enter.proceed(session_key: <lead key>, ticket: <Target ticket path or stem>, phase: <Slice>, next_skill: <NEXT value>, conditions: [<notable route-context flags, e.g. "freshness=<value>", "discussion=<value>", "scope-blocker=<value>">])` to record routing context before invoking the route.
```

(Renumber the existing Execute Verdict steps accordingly.) Map `ticket`/`phase`/
`conditions` from the Routing Verdict fields (Target, Slice, Freshness, Discussion,
Scope Blocker).

### 3. lead-sprint (`lead-sprint/lead-sprint.md`)

Insert ONE call in `## On: sprint-edit`, immediately after step 2 (which initializes
`<current-edit-context>`, `<episode-slug>`, `<episode-start>` for a NEW episode).
Guard it to fire only when a new episode was just initialized (the same
"If no episode is active" condition as step 2).

```
N. If a new episode was just initialized, call `{{.McpNamespace}}/enter.sprint(session_key: <lead key>, episode_slug: <episode-slug>, episode_start: <episode-start>, current_edit_context: <current-edit-context>)` to record the episode so it is recoverable before its first commit.
```

Leave the existing `Sprint-Edit:` marker logic (`## On: recover episode`, `## On:
invoke` step 4) untouched.

### 4. lead-salvage (`lead-salvage/lead-salvage.md`)

(a) Insert the enter call in `## On: Containment`, immediately AFTER step 4 (user
confirms the failure claim) and before step 6 (enter Survey Fanout):

```
N. Call `{{.McpNamespace}}/enter.salvage(session_key: <lead key>, failure_claim: <user-confirmed failure claim>, confirmed_premises: [], survey_status: "pending")` to record the confirmed failure claim so it survives compaction without re-confirmation.
```

(b) Insert ONE `agenda.set` refresh in `## On: Premise Interview`, after the step
where premises are locked (step 5/6, "Move unconfirmed premise candidates …" / "Stop
interviewing …"), to lock confirmed premises without resetting the salvage todo list
(a second `enter.salvage` would discard todo progress — do NOT do that):

```
N. Call `{{.McpNamespace}}/agenda.set(session_key: <lead key>, key: "salvage", value: {failure_claim: <confirmed claim>, confirmed_premises: [<user-confirmed premises>], survey_status: "complete"})` to lock the confirmed premises into the salvage agenda blob.
```

## Integration Test Instructions

Boundary: rsrc playbook render. Add lightweight regression assertions in
`agents-plugin-tool/internal/mcp/playbook_tools_test.go` (home of the existing
`TestPlaybookPrint*` render tests). Add a focused test (e.g.
`TestSkillsCallEnterTools`) that renders each of the four skills via the same
`printPlaybook` path the existing tests use and asserts the rendered body contains
the expected call token at a non-trivial location:

- lead-implement render contains `enter.implement` AND `need_review` (an arg that
  appears only from the inserted call, not incidental prose).
- lead-proceed render contains `enter.proceed`.
- lead-sprint render contains `enter.sprint`.
- lead-salvage render contains `enter.salvage` AND `agenda.set`.

Choose tokens that cannot appear incidentally in the surrounding prose (prefer the
full `enter.<mode>` token, not a bare word like "implement"). Keep assertions
load-bearing, not tautological.

## Implementation Strategy Decisions

Settled — do not reopen:

1. **Single enter call per skill.** `enter.*` replaces the entire session-state todo
   list (mode switch). Calling it twice resets progress. Each skill calls its enter
   tool exactly once at the specified point; later field updates use `agenda.set`
   (salvage only, here).
2. **lead-implement placement reconciliation.** The ticket says "immediately after
   the Route step," but the call's `start_commit` (Prep step 5), `merge_target` (Prep
   steps 2-4), and create-mode `current_branch` (Prep step 6) are only settled in
   Prep. A single complete-context call therefore lands at the earliest data-complete
   point: right after Prep step 6. This honors the ticket's intent (capture implement
   mode context as early as the data allows) without an incomplete early call or a
   forbidden second call. This is a deliberate lead-approved reconciliation, recorded
   here for the fit reviewer.
3. **Additive sprint integration.** `enter.sprint` is ADDED at episode start; the
   `Sprint-Edit:` marker resume path is preserved. Cross-compaction recovery of the
   sprint episode is served by the workflow manual surfacing the `sprint` agenda blob
   (Phase 3a continue mode), not by rewriting `On: recover episode`.
4. **Salvage premise lock via agenda.set.** `enter.salvage` after Containment records
   the failure claim early (`confirmed_premises: []`, `survey_status: "pending"`); the
   `agenda.set` refresh in Premise Interview locks premises after they are confirmed,
   fulfilling the ticket's "premises already locked" benefit without a todo-resetting
   second enter call.
5. **Namespace via template variable** (`{{.McpNamespace}}/…`), never hardcoded `ws.`.

## Rejected Alternatives

- Multiple `enter.*` calls to progressively fill fields — rejected: resets the
  derived todo list each time.
- Hardcoding `ws.enter.*` / `ws.agenda.*` — rejected: breaks the wsflow product-mode
  namespace test and the host-neutral convention.
- Ripping out lead-sprint marker-resume / lead-implement host task list — rejected:
  out of slice scope; additive integration only.
- Calling `enter.implement` literally inside Route with empty `start_commit`/
  `merge_target` — rejected: loses recovery-valuable fields for no benefit (see
  decision 2).

## Approach

1. Read `ws/playbook.print(name: "lead-skill-authoring")` and apply its invariant
   checklist to every changed line. Prefer adding the enter call as a procedure step;
   add or adjust an Invariants line only if skill-authoring rules require it, keeping
   changes surgical and matching each file's existing voice.
2. Edit the four canonical files per the placements above.
3. Add the render-assertion test.
4. Regenerate the wsflow rsrc mirror and both manifests (see Verification Contract).
5. Build, run the targeted test packages, confirm drift guards green and no NEW
   failures beyond the three known pre-existing ones.
6. Commit on the current branch (`implement/260625-ws-session-state-machine-p1`).

## Constraints

- AI-authored content is English.
- Surgical edits only; do not reflow or reword untouched steps; renumber only where a
  step is inserted.
- After editing rsrc `.md` files you MUST regenerate the wsflow mirror and both
  `manifest.json`; the drift guards (`TestShippedManifestUpToDate`,
  `TestWsflowRsrcMirrorUpToDate` in `internal/wsrsrc`) must be green.
- Do NOT hand-edit any wsflow mirror file directly; produce it via the regen command.
- Do NOT bump the plugin version (that is a dev-merge step via the bump script).
- Three pre-existing `internal/mcp` test failures are expected and OUT OF SCOPE:
  `TestShippedDelegationSectionSeedAndOverride`,
  `TestShippedUserPreferenceSectionEmptySlotAndOverride`, `TestConfigPromptSetEndToEnd`.
  Confirm your change introduces ZERO new failures; do not attempt to fix these three.

## Out of scope

forge-* task migration; delegate-orientation update; sprint recovery-logic rewrite;
any spec/ref/mental-model prose; version bump; the three pre-existing test failures.

## Details

Phase 1 Go schema (source of truth — `agents-plugin-tool/internal/mcp/server.go`
tool registration + `session_state.go` handlers/derivation):

- `ws.enter.implement` props: `session_key`(req), `delegation`, `plan_depth`,
  `branch_mode`, `review_alloc`, `current_branch`, `merge_target`, `start_commit`,
  `active_agents` ([{name, role, started}]), `need_review`(bool), `need_doc`(bool).
  Derived todos: Route, Prep, Edit, [Review if need_review], [Doc pre-pass / Doc
  commit gate / Doc closeout if need_doc], Final action gate, Merge.
- `ws.enter.proceed` props: `session_key`(req), `ticket`, `phase`, `next_skill`,
  `conditions` (string array). Derived todos: Build route context, Select route,
  Emit routing verdict, Execute verdict.
- `ws.enter.sprint` props: `session_key`(req), `episode_slug`, `episode_start`,
  `current_edit_context`. Derived todos: Edit, Verify, Commit, Post-edit decision,
  Wrap episode.
- `ws.enter.salvage` props: `session_key`(req), `failure_claim`, `confirmed_premises`
  (string array), `survey_status`. Derived todos: Containment, Survey fanout, Premise
  interview, Classification, Capture.
- `ws.agenda.set` props: `session_key`(req), `key`(req), `value` (req, arbitrary
  object). Upserts the agenda blob under `key` without touching the todo list.

## Verification Contract

From `agents-plugin-tool/`:

```
# regenerate generated artifacts after editing the 4 canonical .md files
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest

# verify
go build ./...
go test ./internal/wsrsrc/ ./internal/mcp/
```

Acceptance:

- `go build ./...` clean.
- `internal/wsrsrc` drift guards green (`TestShippedManifestUpToDate`,
  `TestWsflowRsrcMirrorUpToDate`).
- New `TestSkillsCallEnterTools` (or equivalent) passes; assertions are load-bearing.
- `TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance` green (no bare-namespace
  leak from the inserted calls).
- `internal/mcp` shows ONLY the three known pre-existing failures; zero new failures.

## References

<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->

- Call `ws/playbook.print(name: "lead-skill-authoring")` — [Must] skill-authoring
  invariant checklist; apply to every changed line.
- `ai-docs/mental-model/workflow-skills.md` — [Must] the `lead-*` skill family,
  routing roles, and what each skill is responsible for.
- `ai-docs/mental-model/mcp-runtime.md` — [Must] ferrule/session-key semantics and
  session-state tool behavior.
- `ai-docs/mental-model/prompt-bundle.md` — [Must] marker-based product-mode selection
  and the `{{.McpNamespace}}` substitution path; explains the namespace render test.
- `agents-plugin-tool/internal/mcp/server.go` (tool registration ~line 2360) and
  `agents-plugin-tool/internal/mcp/session_state.go` (handlers + derivation ~line 250)
  — [Maybe] confirm exact argument names if anything here is ambiguous.
