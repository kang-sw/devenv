# Plan: mcp-server-repair pointers reach only front-door skills, not the skills a procedure lands in — Phase 1: Extend the repair pointer to mid-procedure skills

## Relevant Ticket Contract

- Scope is a pointer sweep only: no bundling, no new fallback mechanism, no softening of the stop-and-report default beyond naming the repair route.
- wsflow tree, sole target: add the front-door pointer wording to `agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md` and `.../lead-write-spec/SKILL.md`. Do not mirror into the ws tree — those two skills do not exist there (playbook-only surfaces).
- ws tree, enumerated sweep: add the same wording to exactly ten named skills: `lead-add-rule`, `lead-bootstrap`, `lead-forge-mental-model`, `lead-forge-spec`, `lead-goal-fan-out-step`, `lead-review`, `lead-salvage`, `lead-ship`, `lead-skill-authoring`, `lead-tune`. Closed set — do not add or drop members.
- `adbf5ec3` established the pointer wording; reuse it verbatim, do not invent a second phrasing.
- Do not create new ws-tree front-door skills for `lead-write-ticket`/`lead-write-spec` (explicit non-goal, separate approval required).
- Update `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py::test_skill_files_are_thin_playbook_shims`: move `lead-write-ticket` and `lead-write-spec` into the pointer-tail set (alongside `lead-proceed`/`lead-discuss`/`lead-sprint`). This test is the verification probe.
- Regenerate `agents-plugin/skills/manifest.json` after the ws-tree edits (ticket names `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1`; **this env var is wrong for this manifest** — see Codebase Findings).
- Verification boundary (verbatim): "`test_wsflow_skill_bundle.py` passes with `lead-write-ticket` and `lead-write-spec` asserted as carrying the pointer tail; the ten enumerated ws-tree skills each contain the pointer wording; the manifest regenerates clean with no diff on re-run."

## Out of Scope

- The eight other ws-tree skills that already have wsflow mirrors and are *not* in the enumerated ten (`lead-discuss`, `lead-sprint`, `lead-proceed`, `lead-revive`, `lead-implement`, `lead-check-blockers`, `lead-update-spec`, `lead-workflow-manual`, `lead-verify-discussion`, `lead-prefer-subagent`, `lead-goal-step`, `mcp-server-repair`, `lead-write-ticket`, `lead-write-spec` where already pointed) — untouched.
- The wsflow-tree mirrors of the ten ws-tree skills (8 of the 10 have a wsflow copy: all except `lead-salvage` and `lead-skill-authoring`, which are wsflow-excluded). The ticket's Phase 1 bullets name only "ws tree, enumerated sweep" for these ten — their wsflow copies are not touched this phase. This is a real residual gap (those wsflow copies still end at their own un-pointed tail) but is explicitly out of scope per the ticket's phrasing; flagging it, not acting on it.
- `agents-plugin/rsrc/` and `agents-plugin-wsflow/rsrc/` playbook bodies — untouched; no rsrc-mirror regen needed (see Codebase Findings).
- Any spec change — ticket states Spec Impact is none.

## Codebase Findings

- **Pointer wording is byte-identical (sentence text) across all four `adbf5ec3` front doors, per tree, modulo `ws:`/`wsflow:` substitution.** Verified by reading current source:
  - `agents-plugin/skills/lead-discuss/SKILL.md#L13`, `agents-plugin/skills/lead-sprint/SKILL.md#L13`, `agents-plugin/skills/lead-proceed/SKILL.md#L10`, `agents-plugin/skills/lead-revive/SKILL.md#L12` — all read exactly: `If this call fails to connect, run \`/ws:mcp-server-repair\`.` (each is its own final line — none joined onto the preceding sentence).
  - `agents-plugin-wsflow/skills/lead-discuss/SKILL.md#L13`, `.../lead-sprint/SKILL.md#L13`, `.../lead-revive/SKILL.md#L12` — same sentence with `/wsflow:mcp-server-repair`, each its own final line.
  - `agents-plugin-wsflow/skills/lead-proceed/SKILL.md#L9` is the one visual outlier: `inline against the current user request. If this call fails to connect, run \`/wsflow:mcp-server-repair\`.` — joined onto the same line as the preceding sentence, not wrapped to a new line. Confirmed via `git show adbf5ec3` this is because wsflow's pre-existing tail was `inline against the current user request. If the playbook cannot be loaded, stop\nand report that blocker.` (already joined/wrapped that way) and the commit *replaced* the blocker clause in place; ws-tree's pre-existing tail had no blocker clause at all, so the commit *appended* a new trailing line instead. Not a real drift — both are the correct precedent for their respective "append vs. replace" situation.
  - **Verdict: the sentence text is canonical and identical per tree; reuse it verbatim as `If this call fails to connect, run \`/ws:mcp-server-repair\`.` (ws tree) / `If this call fails to connect, run \`/wsflow:mcp-server-repair\`.` (wsflow tree).**

- **The ticket's Background quote ("If the playbook cannot be loaded, stop and report that blocker.") is wsflow-tree-specific, not a ws-tree anchor.** Confirmed verbatim in `agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md#L9-L10` and `.../lead-write-spec/SKILL.md#L9-L10` (both files, identical two-line tail: `inline against the current user request. If the playbook cannot be loaded, stop\nand report that blocker.\n`). This exact string is the anchor `test_skill_files_are_thin_playbook_shims`'s fullmatch regex matches (`test_wsflow_skill_bundle.py#L127-L128`). **None of the ten ws-tree skills currently contain this string at all** — verified by reading all ten; nine end at `inline against the user request.` with no blocker clause, and `lead-goal-fan-out-step` ends at `After both return, execute the procedure returned by \`ws/playbook.print\`.` with no blocker clause. This matches the pre-`adbf5ec3` ws-tree front doors (`git show adbf5ec3^:agents-plugin/skills/lead-proceed/SKILL.md` had no blocker tail either) — the ws tree never carried that sentence; the fix there is a pure append, not a replace.

- **File-by-file target list** (all verified to exist, current tail shown):
  - `agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md` (10 lines) — `#L9-10`: `inline against the current user request. If the playbook cannot be loaded, stop\nand report that blocker.`
  - `agents-plugin-wsflow/skills/lead-write-spec/SKILL.md` (10 lines) — `#L9-10`: identical tail.
  - `agents-plugin/skills/lead-add-rule/SKILL.md` (9 lines) — `#L9`: `inline against the user request.`
  - `agents-plugin/skills/lead-bootstrap/SKILL.md` (9 lines) — `#L9`: same.
  - `agents-plugin/skills/lead-forge-mental-model/SKILL.md` (9 lines) — `#L9`: same.
  - `agents-plugin/skills/lead-forge-spec/SKILL.md` (9 lines) — `#L9`: same.
  - `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md` (12 lines) — `#L12`: `After both return, execute the procedure returned by \`ws/playbook.print\`.` (parallel-init shape, same as `lead-discuss`/`lead-sprint`).
  - `agents-plugin/skills/lead-review/SKILL.md` (9 lines) — `#L9`: same as add-rule.
  - `agents-plugin/skills/lead-salvage/SKILL.md` (9 lines) — `#L9`: same.
  - `agents-plugin/skills/lead-ship/SKILL.md` (9 lines) — `#L9`: same.
  - `agents-plugin/skills/lead-skill-authoring/SKILL.md` (9 lines) — `#L9`: same. (Root AGENTS.md rule: before editing, read `agents-plugin/skills/lead-skill-authoring/SKILL.md`'s own invariant checklist — self-referential since this file is itself one of the ten targets; the checklist source lives in `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`, the shim's target playbook.)
  - `agents-plugin/skills/lead-tune/SKILL.md` (9 lines) — `#L9`: same.
  - All ten exist; none deviates from the expected "no blocker tail" anchor — they are uniform.

- **wsflow-mirroring.md doc drift found (report to lead, no action needed this phase):** `ai-docs/ref/wsflow-mirroring.md#L29-L60` lists "Shipped wsflow Skills" Included/Excluded, but `lead-tune` appears in neither list, even though `agents-plugin-wsflow/skills/lead-tune/SKILL.md` exists on disk. Verified by direct directory check. Not a Phase 1 blocker (Phase 1 doesn't touch wsflow copies of the ten), but the doc is stale and could mislead a future mirroring check.

- **None of the ten ws-tree skills are in the substitution-mirror generator's exception list** (`ai-docs/ref/wsflow-mirroring.md#L140-L173`; generator `agents-plugin-tool/internal/wsrsrc/skills_mirror.go`, curated list in `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`). That mechanism (`WS_REGEN_WSFLOW_SKILLS`) only applies to `lead-prefer-subagent`, `lead-verify-discussion`, `lead-goal-step`, `mcp-server-repair`. Where a wsflow copy of one of the ten exists (8 of 10; `lead-salvage` and `lead-skill-authoring` are wsflow-excluded per `ai-docs/ref/wsflow-mirroring.md#L55-L60`), it is hand-curated, not generated — editing the ws-tree source does **not** auto-propagate and does **not** require `WS_REGEN_WSFLOW_SKILLS`. `WS_REGEN_WSFLOW_RSRC` mirrors `agents-plugin/rsrc/` → `agents-plugin-wsflow/rsrc/` byte-for-byte and is unrelated to skill files entirely — not needed here since no `rsrc/` playbook body changes.

- **The ticket's named manifest-regen command is imprecise.** `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L26-L57` shows `agents-plugin/skills/manifest.json` is regenerated by `TestGenerateRealSkillsManifest`, gated on env var `WSRSRC_REGEN_SKILLS` (skip message: "set WSRSRC_REGEN_SKILLS=1 to regenerate agents-plugin/skills/manifest.json"). `WSRSRC_REGEN` (no `_SKILLS` suffix) is a *different* gate on `TestGenerateRealManifest` (`agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L958-L960`) that regenerates `agents-plugin/rsrc/manifest.json` instead — a different file, unrelated to this phase. The ticket's Phase 1 text names `WSRSRC_REGEN=1`; the correct command for what the ticket actually needs (`agents-plugin/skills/manifest.json`) is `WSRSRC_REGEN_SKILLS=1`. Confirmed drift-free baseline: running `TestSkillsManifestDriftIsVisible`, `TestShippedManifestUpToDate`, `TestWsflowSkillsMirrorUpToDate`, `TestWsflowRsrcMirrorUpToDate` today all pass (no pre-existing drift).
- `agents-plugin-wsflow/skills/` has no manifest of its own (confirmed: only `agents-plugin-wsflow/rsrc/manifest.json` exists, mirroring the rsrc tree, unrelated to skills). Editing `lead-write-ticket`/`lead-write-spec` in the wsflow tree requires no manifest regen at all.

- **Test file structure** (`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`):
  - `#L106-L133` `test_skill_files_are_thin_playbook_shims` — current un-pointed-tail fullmatch regex ends `r"inline against the current user request\. If the playbook cannot be loaded, stop\n" r"and report that blocker\.\n"`; iterates `EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS - EXPECTED_PARALLEL_INIT_SKILLS - {"lead-proceed"}` (currently 13 skills, comment says "the other 13 shims").
  - `#L135-L149` `test_lead_proceed_shim_carries_repair_pointer` — single hardcoded fullmatch for `lead-proceed` only, tail `r"inline against the current user request\. " r"If this call fails to connect, run \`/wsflow:mcp-server-repair\`\.\n"` (joined on one line, matching the actual file).
  - `#L151-L183` `test_parallel_init_skill_files_are_playbook_shims` — dict-driven (`pointer_tail`) per-skill exact tails for `lead-discuss`/`lead-sprint` (pointer, own line) vs `lead-goal-fan-out-step` (no pointer — this phase does not touch this test's dict, since `lead-goal-fan-out-step` gets its pointer added directly to source, and this dict must be updated too — see Implementation Plan).

## Implementation Plan

1. `agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md#L9-10` — replace the two lines
   `inline against the current user request. If the playbook cannot be loaded, stop\nand report that blocker.`
   with one line, joined exactly like `lead-proceed`'s wsflow tail:
   `inline against the current user request. If this call fails to connect, run \`/wsflow:mcp-server-repair\`.`
2. `agents-plugin-wsflow/skills/lead-write-spec/SKILL.md#L9-10` — identical replacement.
3. For the nine single-call ws-tree skills (`lead-add-rule`, `lead-bootstrap`, `lead-forge-mental-model`, `lead-forge-spec`, `lead-review`, `lead-salvage`, `lead-ship`, `lead-skill-authoring`, `lead-tune`) — append one new final line after the existing `inline against the user request.` line, matching the ws-tree `lead-proceed`/`lead-revive` append precedent (own line, not joined):
   `If this call fails to connect, run \`/ws:mcp-server-repair\`.`
   Before editing `agents-plugin/skills/lead-skill-authoring/SKILL.md`, read that skill's own invariant checklist (via its target playbook `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`) per the root AGENTS.md pre-edit rule, and apply it to this one added line.
4. `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md#L12` — append one new final line after `After both return, execute the procedure returned by \`ws/playbook.print\`.`, matching the ws-tree `lead-discuss`/`lead-sprint` parallel-init precedent (own line):
   `If this call fails to connect, run \`/ws:mcp-server-repair\`.`
5. `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` — three edits:
   a. `#L106-L117` `test_skill_files_are_thin_playbook_shims`: change the subtraction set from `- {"lead-proceed"}` to `- {"lead-proceed", "lead-write-ticket", "lead-write-spec"}`; update the comment "the other 13 shims" to "the other 11 shims".
   b. `#L135-L149` — generalize `test_lead_proceed_shim_carries_repair_pointer` into a dict-driven loop covering `lead-proceed`, `lead-write-ticket`, `lead-write-spec` (all three are single-call shims with an identical joined-tail shape, differing only by skill name and title), e.g.:
      ```python
      POINTER_TAIL_TITLES = {
          "lead-proceed": "Proceed",
          "lead-write-ticket": "Write Ticket",
          "lead-write-spec": "Write Spec",
      }

      def test_single_call_shims_carry_repair_pointer(self):
          offenders = []
          for skill, title in POINTER_TAIL_TITLES.items():
              path = SKILLS_DIR / skill / "SKILL.md"
              text = path.read_text(encoding="utf-8")
              match = re.fullmatch(
                  r"---\n"
                  rf"name: {re.escape(skill)}\n"
                  r"description: .+\n"
                  r"---\n\n"
                  rf"# {re.escape(title)}\n\n"
                  rf"Call `wsflow/playbook\.print\(name: \"{re.escape(skill)}\"\)` and execute the returned procedure\n"
                  r"inline against the current user request\. "
                  rf"If this call fails to connect, run `/wsflow:mcp-server-repair`\.\n",
                  text,
              )
              if match is None:
                  offenders.append(str(path.relative_to(PLUGIN_DIR)))
          self.assertEqual(offenders, [])
      ```
      (Keep or drop the old method name at the executor's discretion; no other file references `test_lead_proceed_shim_carries_repair_pointer` by name — confirmed by repo-wide grep.)
   c. `#L157-L161` `test_parallel_init_skill_files_are_playbook_shims`'s `pointer_tail` dict — leave `lead-discuss`/`lead-sprint` unchanged; `lead-goal-fan-out-step` is *not* touched by this test (it is a ws-tree-only edit in step 4, and this test only reads the wsflow tree), so no change needed here. (Note: this test does not gate the ws-tree `lead-goal-fan-out-step` edit at all — that skill's ws-tree pointer wording is verified only by direct read/manual grep, since it has no wsflow test coverage for this specific line. This is expected: the shipped test suite covers wsflow tree only.)
6. Regenerate `agents-plugin/skills/manifest.json` after the ws-tree edits (steps 3-4):
   `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -count=1 -v`
   (not `WSRSRC_REGEN` — that regenerates a different file, `agents-plugin/rsrc/manifest.json`, unrelated to this phase). `-count=1` is mandatory: the regen entrypoint is an env-gated test body with no changing input, so go's test cache can return a stale green `ok` without running the write side effect.
7. No version bump (explicit non-goal this phase).

## Verification Plan

- `python3 -m unittest discover agents-plugin-wsflow/tests` (from repo root) — must pass, including the updated `test_skill_files_are_thin_playbook_shims` (11 remaining shims) and the new/updated pointer-tail assertions for `lead-proceed`/`lead-write-ticket`/`lead-write-spec`.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... -run TestSkillsManifestDriftIsVisible -count=1 -v` — must pass clean (no diff) after running the regen in Implementation step 6, confirming the manifest regenerates clean with no diff on re-run.
- Manual grep confirmation that the ten ws-tree files and the two wsflow files each contain their pointer sentence verbatim (e.g. `grep -rn "mcp-server-repair" agents-plugin/skills/lead-add-rule agents-plugin/skills/lead-bootstrap ... agents-plugin-wsflow/skills/lead-write-ticket agents-plugin-wsflow/skills/lead-write-spec`), since the ws-tree edits are not covered by the Python test suite.
- Ticket verification boundary (verbatim): "`test_wsflow_skill_bundle.py` passes with `lead-write-ticket` and `lead-write-spec` asserted as carrying the pointer tail; the ten enumerated ws-tree skills each contain the pointer wording; the manifest regenerates clean with no diff on re-run."

## Escalations

- None.
