# Survey: 22-260622-reviewer-playbooks

## Reusable Components

- `agents-plugin/rsrc/code-review-correctness/code-review-correctness.md#L1-L10` — `code-review-correctness` frontmatter pattern: `kind: render`, `delegates: true`, `role: reviewer`, `tier: large`, `includes: [code-reviewer]`, `variables: [RoleModel]`. New ticket reviewer playbooks mirror this frontmatter structure minus `includes:` (brief decision: self-contained).
- `agents-plugin/rsrc/code-review-fit/code-review-fit.md#L1-L10` — `code-review-fit` frontmatter: same pattern as correctness but `tier: medium`. The completeness reviewer maps to `tier: medium` exactly.
- `agents-plugin/rsrc/reviewer/reviewer.md#L1-L16` — base reviewer identity block (`kind: render`, `delegates: true`, role declaration, alias-model line). Ticket reviewers do NOT include this; use it only to understand the shared field/alias conventions to avoid.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L245-L250` — **Delegate dispatch** pattern: render via `{{.McpNamespace}}/playbook.render(name: "<playbook>")`, spawn native subagent with rendered prompt as full role, pass ticket path as task-specific input. This is the exact dispatch pattern the Sage Review Gate must follow.
- `agents-plugin/rsrc/manifest.json` — current manifest schema: `{"schema_version": 1, "files": {"<subdir/file.md>": "<sha>"}}`. New entries will be `"ticket-reviewer-design/ticket-reviewer-design.md"` and `"ticket-reviewer-completeness/ticket-reviewer-completeness.md"`.

## Existing Patterns

- Reviewer frontmatter structure: see `agents-plugin/rsrc/code-review-correctness/code-review-correctness.md#L1-L10` and `agents-plugin/rsrc/code-review-fit/code-review-fit.md#L1-L10` — partition reviewers carry frontmatter then a title, identity line, alias-model line, Partition scope, Checklist, Out of scope. Ticket reviewers diverge by replacing `includes:` with self-contained Output section and adding Constraints/Process/Doctrine.
- Handler insertion in lead playbooks: `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L22-L62` — "On: invoke" uses `### N. StepName` numbered blocks, each with a numbered sub-list. The insertion point for step 8 is after `### 7. Commit` (line ~55) and before `### 8. Handoff` (line ~59). Renumber is a single line change.
- On: handler sections: `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L64-L300` — handlers are `## On: <EventName>` top-level sections following "On: invoke". New "On: Sage Review Gate" should follow "On: Output Handoff" (line ~189) and precede "On: Cross-ticket decision review" (line ~199).
- Manifest regen commands: `ai-docs/mental-model/workflow-skills.md#L97` — `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest` then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`. Order is mandatory.

## Relevant Interfaces

- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L55-L62` — existing steps 7 (Commit) and 8 (Handoff): exact text needed for the renumber edit and insertion point. The "### 8. Handoff" line must become "### 9. Handoff".
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L189-L198` — "On: Output Handoff" section end and "On: Cross-ticket decision review" section start: the new "On: Sage Review Gate" handler inserts between these two sections.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L245-L246` — Delegate dispatch primitive: `playbook.render` + native subagent instruction. The gate's parallel spawn must use this exact form.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L1-L6` — file frontmatter: `kind: print`, `includes: [task-list]`. No `delegates: true` currently; adding a gate that spawns subagents may require adding `delegates: true` to this frontmatter.

## Constraints

- **No `includes: code-reviewer`**: ticket reviewers have a different output schema (structured YAML verdict, not markdown findings). Including the base reviewer would inject its severity table, re-review scope, and output template — all incompatible.
- **Manifest regen order**: `WS_REGEN_MANIFEST=1` must run before `WS_REGEN_WSFLOW_RSRC=1`. The wsflow mirror regen reads the updated manifest as its source.
- **`lead-write-ticket.md` does not currently have a `## Templates` section**: the file ends with `## Doctrine` at the bottom. Brief directs Templates to be inserted before Doctrine (skill layout order: Invariants → Handlers → Judgments → Templates → Doctrine). Verify line count to locate the Doctrine section insertion point.
- **Gate skips `idea/` landing status**: the gate condition checks landing status first; `idea/` tickets bypass the gate entirely regardless of `sage_review` config value.
- **`sage_review` config unregistered until Phase 3**: `config.show` will return empty/absent for this key before Phase 3. The gate must treat empty/unset as `off` (skip). This is load-bearing for correct pre-Phase-3 behavior.
- **`delegates: true` in `lead-write-ticket` frontmatter**: currently absent (`kind: print`, `includes: [task-list]`). The Sage Review Gate spawns native subagents, which means `delegates: true` may need to be added. Verify whether `kind: print` playbooks require this flag to spawn delegates or if it's informational only.

## Risk Signals

- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L1-L6` — Possible **contract risk**: `lead-write-ticket` frontmatter has `kind: print`, no `delegates: true`. If `delegates: true` is required for subagent-spawning playbooks (vs. just being a metadata signal for callers), omitting it may silently behave incorrectly. Lead/planner should verify whether `lead-implement` (which has `delegates: true`) requires it and whether `lead-write-ticket` needs the same.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L55-L62` — Possible **insertion accuracy risk**: the exact line numbers for "### 7. Commit" and "### 8. Handoff" need to be confirmed before editing. If the file has changed since the brief was authored, the renumber operation could corrupt step ordering. Read the file before editing.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L245-L246` — Possible **reuse risk**: the brief says "spawn native subagent with rendered prompt; task input: `Ticket path: <ticket-path>`" — this matches the dispatch pattern in `lead-implement`. However, `lead-write-ticket` does not currently use `playbook.render` anywhere. The implementer should verify whether any harness-level restrictions exist on calling `playbook.render` from a `kind: print` playbook (vs. a `kind: render` playbook caller).
- `agents-plugin/rsrc/manifest.json` — Possible **stale-manifest test failure risk**: `go test ./...` will fail if new rsrc files exist but manifest is not regenerated. Brief correctly specifies regen before test. However, both regen commands run Go tests with `-count=1`; verify the test binary is buildable in `agents-plugin-tool/internal/wsrsrc` before running regen.

## Opinion

- The brief is well-specified with exact file content for both new playbooks and precise insertion instructions for `lead-write-ticket`. No escalation needed.
- The `delegates: true` frontmatter question is a minor ambiguity worth confirming by reading `lead-implement/lead-implement.md` frontmatter (it has `delegates: true`) and comparing to `lead-write-ticket` frontmatter. If the flag is required for subagent dispatch, the implementer must add it to `lead-write-ticket.md` frontmatter — the brief does not mention this change.
- Fresh-Reader Audit (step 5 in brief Approach) is listed as a required pass but the lead-skill-authoring SKILL.md only shows a one-line `ws/playbook.print` redirect. The implementer will need to invoke the lead-skill-authoring playbook inline to get the actual audit procedure. This is expected workflow, not a gap.
