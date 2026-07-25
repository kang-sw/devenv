# Plan — 260724 Phase 3: wsflow exposure verification + mirror

## Relevant Ticket Contract

Ticket: `ai-docs/tickets/ready/260724-feat-lead-fan-out-worktree.md`, **Phase 3: wsflow
exposure verification + mirror** (the ticket's LAST phase). Phases 1 and 2 are DONE.

Phase 3 completion boundary (from the ticket body):
1. **Mirror** the new rsrc overlay (`agents-plugin/rsrc/lead-goal-fan-out-step/`) and add
   the entry-skill shim into the `agents-plugin-wsflow/` tree so
   `TestWsflowRsrcMirrorUpToDate` (currently red BY DESIGN) turns green — via the
   established regen mechanism, not hand-edits.
2. **Verify** the serve-time transclusion branch resolves the wsflow-mirrored
   `lead-goal-step` SKILL.md against the wsflow skills root and still emits the visible
   `<playbook name="lead-goal-step" title="Goal Step">` boundary under wsflow.
3. **Confirm** the reduced-runtime wsflow exposure of the primitives the fan-out skill
   depends on: `ferrule`, `session.children`, `session.note`, `playbook.render`.
4. wsflow mirror-drift tests (`python3 -m unittest discover agents-plugin-wsflow/tests`)
   green for the new lead-* skill + session-tool surface, following
   `ai-docs/ref/wsflow-mirroring.md`.

## Out of Scope

- No new runtime tool exposure work: all four primitives are ALREADY present in
  `agents-plugin-wsflow/runtime.json` (verified — item 3 is verification only).
- No overlay-body rewrite: the rsrc overlay is byte-mirrored (namespace substitution is
  render-time, not a stored edit).
- No full-ws skill/manifest changes (Phase 2 landed those).
- No version bump / ticket close / merge (lead owns those after review).
- No batch>=2 live dry-run (Phase 2 deferred; not Phase 3 scope).

## Codebase Findings

- **Red test confirmed:** `TestWsflowRsrcMirrorUpToDate` fails with `byte-differs:
  manifest.json` + `missing in wsflow: lead-goal-fan-out-step/lead-goal-fan-out-step.md`.
- **rsrc mirror is byte-identical & generated** (`ai-docs/ref/wsflow-mirroring.md` →
  "Rsrc Tree Provisioning"). Regen: `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc
  -count=1 -run TestRegenerateWsflowRsrcMirror`. The full-ws rsrc manifest already
  carries the fan-out entry (Phase 2), so this one regen syncs BOTH the missing overlay
  file and the manifest byte-diff.
- **wsflow skill shims are curated thin `wsflow/playbook.print` shims** (not
  byte-mirrored). `agents-plugin-wsflow/skills/` has one dir per lead skill;
  `lead-goal-fan-out-step` is absent.
- **Full-ws shim shape** (`agents-plugin/skills/lead-goal-fan-out-step/SKILL.md`) is the
  **parallel-init** form (`Call in parallel:` playbook.print + workflow_manual), title
  `# Goal Fan-Out Step`. The faithful wsflow mirror is the same parallel-init form with
  `ws/` → `wsflow/`.
- **wsflow bundle test** `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`:
  - `EXPECTED_SKILLS` set governs the shipped inventory (must add the new skill).
  - `EXPECTED_PARALLEL_INIT_SKILLS = {"lead-discuss","lead-sprint"}` +
    `test_parallel_init_skill_files_are_playbook_shims` govern the parallel-init shim
    shape; its title is derived as `skill.removeprefix("lead-").title()`, which yields
    `Goal-Fan-Out-Step` (hyphens) — this does NOT equal the correct display title
    `Goal Fan-Out Step`, so a per-skill title override is required.
  - `lead-goal-fan-out-step` is a `playbook.print` shim (NOT inline), so it is NOT added
    to `EXPECTED_INLINE_SKILLS`. Its full-ws counterpart exists as both a skill dir and
    an rsrc dir, so `test_full_skill_inventory_drift_is_visible` and
    `test_skill_shims_point_to_shared_playbooks` pass once it is in `EXPECTED_SKILLS`.
- **No wsflow skills manifest and no wsflow plugin.json** — nothing else enumerates the
  wsflow skill set. The only Go inventory test `skills_mirror_test.go` iterates the
  curated `substitutionMirroredSkills = ["lead-goal-step"]` only; the fan-out thin shim
  is NOT substitution-mirrored, so that test is untouched.
- **Transclusion is root-agnostic:** `printPlaybook` (`playbook_tools.go` ~L923) does
  `ResolveSkillsRoot()` → `LoadSkillBody(skillsRoot, "lead-goal-step")` →
  `wrapRenderedPlaybookForConcatenation`. `ResolveSkillsRoot` honors `WS_SKILLS_ROOT`.
  The wsflow skills tree carries `lead-goal-step/SKILL.md`, so pointing `WS_SKILLS_ROOT`
  at `agents-plugin-wsflow/skills` appends the wsflow-namespaced goal-step body under
  the same visible boundary. Model test:
  `TestPlaybookPrintGoalFanOutStepAppendsGoalStepUnconditionally`
  (`internal/mcp/playbook_tools_test.go`).
- **Runtime exposure (item 3) already satisfied:** `agents-plugin-wsflow/runtime.json`
  lists `ferrule`, `session.children`, `session.note`, `playbook.render` with the
  `>=0.36.3-dev <0.37.0` gate; none are in the runtime-contract test's `HIDDEN_TOOLS`.

## Implementation Plan

1. **Create the wsflow entry-skill shim.**
   `agents-plugin-wsflow/skills/lead-goal-fan-out-step/SKILL.md` — parallel-init thin
   shim, byte-for-byte the full-ws shim with `ws/` → `wsflow/`:

   ```
   ---
   name: lead-goal-fan-out-step
   description: Advance a goal-pursuit run by one step with batch-parallel worktree fan-out when two or more ready tickets are mutually independent and recursive subagent dispatch is available; falls back to lead-goal-step's serial single-ticket step otherwise.
   ---

   # Goal Fan-Out Step

   Call in parallel:
   - `wsflow/playbook.print(name: "lead-goal-fan-out-step", session_key: <your key, omit if fresh>)`
   - `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

   After both return, execute the procedure returned by `wsflow/playbook.print`.
   ```
   (Description must match the full-ws description verbatim — no `ws/` tokens appear in it.)

2. **Regenerate the wsflow rsrc mirror** (do NOT hand-create the overlay/manifest):
   ```
   cd agents-plugin-tool
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   Then confirm `TestShippedManifestUpToDate` is green; if it is red, the full-ws
   manifest drifted — run `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateShippedManifest` and re-run the wsflow mirror regen. Expected result:
   `agents-plugin-wsflow/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` appears
   and `agents-plugin-wsflow/rsrc/manifest.json` matches full-ws byte-for-byte.

3. **Update the wsflow bundle test** `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`:
   - Add `"lead-goal-fan-out-step"` to `EXPECTED_SKILLS`.
   - Add `"lead-goal-fan-out-step"` to `EXPECTED_PARALLEL_INIT_SKILLS`.
   - Give the parallel-init title check an explicit per-skill title so the multi-hyphen
     display title `Goal Fan-Out Step` is asserted correctly. Introduce a small map, e.g.
     `PARALLEL_INIT_TITLES = {"lead-discuss": "Discuss", "lead-sprint": "Sprint",
     "lead-goal-fan-out-step": "Goal Fan-Out Step"}` and use
     `title = PARALLEL_INIT_TITLES[skill]` in
     `test_parallel_init_skill_files_are_playbook_shims` (replacing the
     `.removeprefix("lead-").title()` derivation). Keep behavior identical for the two
     existing skills.

4. **Add a durable wsflow-root transclusion verification test** in
   `agents-plugin-tool/internal/mcp/playbook_tools_test.go` — e.g.
   `TestPlaybookPrintGoalFanOutStepResolvesWsflowSkillsRoot`, modeled on the existing
   Phase-2 test but with `rsrcRoot = agents-plugin-wsflow/rsrc` and
   `t.Setenv("WS_SKILLS_ROOT", <repo>/agents-plugin-wsflow/skills)`. Assert: the
   `<playbook name="lead-goal-step" title="Goal Step">` boundary is present, and the
   appended block is in lockstep with `LoadSkillBody(wsflowSkillsRoot, "lead-goal-step")`
   (proving the wsflow variant of goal-step is what gets appended under wsflow roots).

5. **Update the mirroring reference doc**
   `ai-docs/ref/wsflow-mirroring.md`: add `lead-goal-fan-out-step` to the "Shipped wsflow
   Skills → Included" list (keep the existing ordering convention).

## Verification Plan

- `cd agents-plugin-tool && go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./internal/wsrsrc/... -count=1` — `TestWsflowRsrcMirrorUpToDate` now GREEN;
  `TestShippedManifestUpToDate` green.
- `go test ./internal/mcp/... -count=1` — existing Phase-2 transclusion test green + the
  new `...ResolvesWsflowSkillsRoot` test green.
- `go test ./cmd/ws-mcp/... -count=1` — runtime-surface exact-match tests green.
- `go test ./... -count=1` — full suite green.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — bundle + runtime-contract
  tests green (skill inventory + parallel-init shim shape + forbidden-reference scan).
- `python3 -m unittest discover agents-plugin/tests` — full-ws contracts unaffected.
- Manual evidence for item 3: `grep -nE '"(ferrule|session\.children|session\.note|playbook\.render)"'
  agents-plugin-wsflow/runtime.json` shows all four present with version gate (record in
  Result).

## Escalations

None expected — mechanical mirror + verification. The one judgment call (parallel-init
shim shape + explicit title override vs. a simple thin shim) is resolved in favor of the
faithful full-ws mirror (parallel-init), recorded above.
