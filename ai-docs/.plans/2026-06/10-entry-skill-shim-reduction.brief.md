# Brief: entry-skill-shim-reduction (260609 Phase 3)

## Intent
Reduce the 11 directly-invocable ws entry skills to thin trigger shims whose
procedural bodies live in `ws/playbook.print`-served rsrc playbooks, mirroring the
Phase 2 internal-procedure migration. Each entry skill stays directly
`/ws:<name>` invocable with its trigger description intact; on invocation the shim
prints and executes its own playbook body.

## Scope Boundary
Selected scope: 260609 Phase 3 ONLY — the 11 entry skills:
`lead-add-rule`, `lead-bootstrap`, `lead-discuss`, `lead-forge-mental-model`,
`lead-forge-spec`, `lead-proceed`, `lead-review`, `lead-salvage`,
`lead-ship`, `lead-skill-authoring`, `lead-sprint`.

Explicitly OUT of scope:
- M3 runtime deletion (`ws/subquery` tool, `agents.*` backends stay callable).
- Any semantic rewrite of a procedure body. Phase 3 is a **pure relocation**: move
  the body verbatim into a playbook, leave a thin shim. Do NOT reword, restructure,
  re-order, or "improve" any procedure text.
- The 9 Phase 2 internal procedures (already migrated to rsrc playbooks).
- wsflow (`agents-plugin-wsflow/`) — untouched this phase.

## Caller-Visible Contract
- After this phase, all 11 entry skills remain directly user-invocable as
  `/ws:<name>` with unchanged trigger descriptions (host surfaces them from
  SKILL.md frontmatter `description`).
- Invoking an entry skill resolves its procedure through the playbook surface:
  the shim calls `ws/playbook.print(name: "<name>")` and executes the returned
  procedure inline.
- `ws/playbook.print(name: "<name>")` returns each entry skill's full procedure
  text for all 11 names.

## Contract Instructions

### Per-skill transform (apply to each of the 11 skills, VERBATIM relocation)

For skill `<name>` at `agents-plugin/skills/<name>/SKILL.md`:

1. **Create the playbook** `agents-plugin/rsrc/<name>/<name>.md`:
   - Frontmatter (exactly):
     ```
     ---
     kind: print
     ---
     ```
     Add `delegates: true` as a second frontmatter line ONLY per the delegation
     rule below.
   - Body: the **entire current SKILL.md body verbatim** — everything after the
     SKILL.md frontmatter closing `---`. Preserve heading levels, the
     `Target:`/`Topic: user request` line, Invariants, On: sections, Judgments,
     Templates, Doctrine — byte-for-byte except for the move itself. Do NOT carry
     the SKILL.md frontmatter (`name:`/`description:`) into the playbook.

2. **Replace the SKILL.md body with a thin shim.** Keep the SKILL.md frontmatter
   (`name:` and `description:`) exactly as-is. Replace everything below the
   frontmatter with this shim body (substitute `<Title>` with the skill's existing
   H1 title text and `<name>` with the skill stem):
   ```
   # <Title>

   Call `ws/playbook.print(name: "<name>")` and execute the returned procedure
   inline against the user request.
   ```
   Nothing else in the shim body. The shim's only job is the trigger
   (frontmatter) + delegating to its playbook.

3. **Preserve auxiliary files untouched** (do NOT move or delete):
   - `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`
   - `agents-plugin/skills/lead-bootstrap/WORKFLOW.md`
   - `agents-plugin/skills/lead-discuss/agents/openai.yaml`
   - `agents-plugin/skills/lead-skill-authoring/agents/openai.yaml`
   These are entry-skill support files; entry skills stay invocable so they remain.

### `delegates: true` rule (per skill, mechanical)
Set `delegates: true` in the playbook frontmatter IF AND ONLY IF the relocated
body contains a subagent-spawning reference:
- a `ws/agents.register` / `ws/agents.call` for a worker (implementer/reviewer/
  updater/etc.), OR
- a `ws/subquery(...)` call, OR
- a `reference-discovery` spawn, OR
- an invocation of the `explore` playbook / the native Explore pattern for
  delegation.

A body that only calls `ws/playbook.print(...)` for other procedures, or only
routes/invokes sibling `ws:<name>` skills, or only runs git/build/file ops, does
NOT delegate — omit the flag. Determine this by reading each body; do not guess.

### Cross-reference invariant (do NOT rewire — verify only)
Phase 3 introduces NO caller rewiring. Confirm (do not change) that:
- References to the 11 entry skills stay as `ws:<name>` skill invocations (the
  shim keeps them `/ws:` invocable).
- References to the 9 Phase 2 procedures already use
  `ws/playbook.print(name: "...")` (done in Phase 2).
- Inline `ws/convention.read(...)` / `ws/infra.read(...)` calls inside the
  relocated bodies STAY inline (Phase 2 option-b single-source decision — do NOT
  convert them to rsrc text deps).

### Manifest + tests (global, after all bodies move)
- Regenerate the rsrc manifest:
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest`
  (run from `agents-plugin-tool/`). This updates `agents-plugin/rsrc/manifest.json`
  only; it does not bump the prompt-bundle hash.
- Add one golden print test per new playbook in
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go`, following the existing
  `TestPlaybookPrintGoldenLead<Name>` Phase 2 tests: assert
  `ws/playbook.print(name: "<name>")` returns the playbook body.
- `TestValidateRealTree` (wsrsrc) must pass — it gates every new playbook.

## Integration Test Instructions
- Boundary: `ws/playbook.print` MCP surface + wsrsrc tree validation.
- Run from `agents-plugin-tool/`:
  `go test ./... -count=1` (the `-count=1` is REQUIRED: wsrsrc/mcp tests read the
  rsrc tree via a runtime path Go's content cache does not track, so `(cached)`
  results do not validate rsrc changes).
- Pass criteria: all new `TestPlaybookPrintGoldenLead*` Phase 3 tests pass;
  `TestValidateRealTree` passes; no regression elsewhere.

## Implementation Strategy Decisions
- VERBATIM relocation. The reviewer verifies body text is unchanged (diff: old
  SKILL.md body == new playbook body, modulo the frontmatter move).
- One playbook per entry skill at `agents-plugin/rsrc/<name>/<name>.md`.
- Shim body is the minimal print-and-execute line; the trigger lives in
  frontmatter `description` (unchanged).
- Batched delegation: the lead drives 3 scoped implementer batches (NOT one call)
  because Phase 2 exhausted a single implementer at smaller scale.

## Rejected Alternatives
- Single-implementer call for all 11: rejected — Phase 2 (8 bodies) exhausted one
  implementer's context.
- Rewording/condensing bodies during the move: rejected — Phase 3 is relocation,
  not authoring; semantic drift would silently change workflow behavior.
- Converting inline `convention.read` to rsrc text deps: rejected in Phase 2
  (option-b single source); not reopened here.
- Ticket re-slicing of Phase 3: rejected — one logical unit; size handled by
  execution batching, not phase structure edits.

## Approach
- Batch A: `lead-proceed`, `lead-ship`, `lead-add-rule`, `lead-sprint`.
- Batch B: `lead-discuss`, `lead-review`, `lead-salvage`, `lead-skill-authoring`.
- Batch C: `lead-forge-spec`, `lead-forge-mental-model`, `lead-bootstrap`.
- Each batch: per-skill transform + per-skill golden test stub. Manifest regen +
  full test run after the final batch (lead-owned). Per-batch checkpoint commit.

## Constraints
- AI-authored content English-only.
- No semantic change to any procedure body.
- Entry skills stay `/ws:<name>` invocable; aux files preserved.
- `.codex` (if present) must not be staged.

## Out of scope
- M3 runtime deletion; wsflow mirroring; any new procedure logic.

## Details
- Phase 2 print playbook frontmatter reference: `agents-plugin/rsrc/lead-check-blockers/lead-check-blockers.md` (`kind: print`).
- Phase 2 `delegates: true` examples: `lead-implement`, `lead-write-spec`,
  `lead-verify-design`, `lead-verify-discussion` playbooks.
- Caller→procedure invocation phrasing reference:
  `agents-plugin/skills/lead-discuss/SKILL.md` lines 32, 79, 94 ("Call
  `ws/playbook.print(name: \"...\")` and execute the returned procedure inline").

## Verification Contract
- `go test ./... -count=1` green from `agents-plugin-tool/`.
- `ws/playbook.print(name: "<name>")` returns full procedure text for all 11 names.
- All 11 SKILL.md files are thin shims (frontmatter + print-and-execute line).
- `delegates: true` set exactly on the spawning bodies (per the rule).
- Aux files unchanged; no `ws:`/procedure cross-reference rewired.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/workflow-skills.md` - [Must] entry/internal boundary, playbook.print model.
- `ai-docs/mental-model/prompt-bundle.md` - [Must] rsrc manifest / hash behavior.
- `ai-docs/spec/workflow-skills.md` `{#260610-entry-skill-surface-reduction}` - [Maybe] caller-visible contract.
