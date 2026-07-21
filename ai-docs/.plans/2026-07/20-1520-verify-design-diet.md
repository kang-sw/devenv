# Plan: verify-design-diet

## Relevant Ticket Contract

- A) Insert one new conditional Process step into
  `agents-plugin/skills/lead-verify-discussion/SKILL.md` and its
  byte-identical mirror `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md`,
  after current step 3, exact wording: "When the user specifically asks to
  verify a design's validity, dictate the concluded design in full to a fresh
  higher-tier subagent — the hypothesis under review, rejected alternatives,
  and paths to any evidence files already read — and request independent
  judgment." Renumber subsequent steps. Keep both files byte-identical after
  the edit.
- B) Delete the `lead-verify-design` skill entirely from live source trees
  (rsrc bodies, wsflow shim, manifests), adjust the Go golden test, update
  spec/mental-model/wsflow-mirroring/`_index.md` references, and
  light-touch two `todo/` tickets that cite it — without rewriting frozen
  ticket phase text, without touching `.done/`, `.plans/`, or the
  `260605-research` idea ticket.
- Verification boundary (from contract): if
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go` is edited, run its
  Go test package and read full output; grep repo (excluding `.worktree/*`)
  for `lead-verify-design` afterward — every remaining hit must sit inside
  `ai-docs/tickets/.done/`, `ai-docs/.plans/`, or `ai-docs/tickets/idea/`;
  confirm the two `lead-verify-discussion` SKILL.md files stay byte-identical.
- Non-goals: no sage-review gate/config/ticket-reviewer-* changes; no
  restructuring of `lead-verify-discussion` beyond the one inserted step; no
  `.worktree/*` edits; no hand-edit of the installed
  `~/.claude/plugins/ws-plugin/` tree; no version-bump script run; no ticket
  status/directory moves.

## Out of Scope

- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` line
  379-382 — mentions `lead-verify-design` only as part of a "today" inventory
  snapshot (`Internal → playbook.print bodies: ... lead-verify-design ...`),
  no actionable instruction. Confirmed historical; left untouched per
  contract.
- `ai-docs/tickets/.done/*` and `ai-docs/.plans/*` mentions of
  `lead-verify-design` (several `.done` tickets, two `.plans` briefs) — all
  historical record of past work; contract explicitly excludes these trees.
- Roster line `ai-docs/tickets/todo/260703-chore-review-delegates-true-classification.md`
  lines 36-38 ("Full original ... roster (pre-inlining): ... `lead-verify-design`, ...")
  — already framed as a historical pre-inlining snapshot (mirrors the existing
  `(now false/inlined-without-tip per the linked ticket's Phase 2)` annotation
  pattern for `lead-verify-discussion`); contract's light-edit instruction
  targets the live audit-example clause at lines 25-26 specifically, not this
  roster line. Leave line 36-38 as-is to avoid scope creep.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` and the
  substitution-mirror generator list — confirmed `lead-verify-design` is not
  a member (only `lead-prefer-subagent`, `lead-verify-discussion`,
  `lead-drain-ready-queue` are); no change needed there.
- `agents-plugin-tool/scripts/bump-ws-version.sh` — explicitly out per
  non-goals.

## Codebase Findings

- `agents-plugin/skills/lead-verify-discussion/SKILL.md#L23-L28` — current
  Process steps 1-6; step 3 is line 25, steps 4-6 are lines 26-28 (`4. Name
  any over-alignment...`, `5. Test the best countercase...`, `6. Recommend
  keep, revise, reject...`). Byte-identical to
  `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` (confirmed via
  `diff`, zero output). Neither file contains any `ws:`/`ws/`/`ws.` token, so
  the new step text needs no namespace substitution to stay byte-identical
  across both copies — a direct identical edit to both files suffices; no
  need to run the `WS_REGEN_WSFLOW_SKILLS` generator.
- `agents-plugin/rsrc/lead-verify-design/lead-verify-design.md`,
  `agents-plugin-wsflow/skills/lead-verify-design/SKILL.md`,
  `agents-plugin-wsflow/rsrc/lead-verify-design/lead-verify-design.md` — the
  three live bodies to delete. Confirmed `agents-plugin/rsrc/lead-verify-design/`
  and `agents-plugin-wsflow/rsrc/lead-verify-design/` are byte-identical
  (`diff -rq`, IDENTICAL_BODY).
- `agents-plugin/rsrc/manifest.json#L31` and
  `agents-plugin-wsflow/rsrc/manifest.json#L31` — identical single-line entry
  `"lead-verify-design/lead-verify-design.md": "f77a2c01..."`; both manifests
  confirmed byte-identical overall (`diff`, no output). Delete this one line
  from both files.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1910-L1927` —
  `TestPlaybookPrintGoldenLeadVerifyDesign`, the only Go reference to
  `lead-verify-design` (confirmed via repo grep restricted to non-test `.go`
  files: zero hits outside this test). Delete the whole function.
- **Risk signal (unlisted in contract's file list, but required for a clean
  delete):** `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L31` —
  `EXPECTED_SKILLS` set contains the literal string `"lead-verify-design",`.
  `test_shipped_skill_inventory_is_converged` (line 63-65) asserts the
  wsflow `skills/` directory listing equals `EXPECTED_SKILLS` exactly, and
  `test_full_skill_inventory_drift_is_visible` (line 67-86) cross-checks
  `EXPECTED_SKILLS` against the full-ws `skills/`+`rsrc/` directory names.
  Deleting the three directories without removing this line will fail both
  tests. Not in the contract's explicit file list or verification boundary —
  the contract's closing claim "No other automated test suite applies to
  these doc/skill-text changes" is inaccurate for this one file; must be
  corrected. Remove `"lead-verify-design",` from the `EXPECTED_SKILLS` set.
- `ai-docs/spec/workflow-skills.md` — four sites: `#L40` (namespace roster
  code block), `#L59` (internal-procedures prose list, comma-joined), `#L190`
  (wsflow Skill Surface prose list), `#L475-L485` (full descriptive paragraph
  ending `{#260524-design-verification-skill}`, sitting under `##
  Planning Workflow Skills {#260505-planning-workflow-skills}`, just before
  `### Check Blockers Checkpoint {#260513-check-blockers-skill}` at L487).
  Confirmed via repo-wide anchor grep: no other file references this anchor,
  so no dangling cross-reference risk from removing/rewriting it.
- **Precedent for the L475-L485 paragraph:** `ai-docs/spec/workflow-skills.md#L497`
  and `ai-docs/mental-model/workflow-skills.md#L74` show the established
  pattern for a previously-retired skill (`lead-write-skeleton`, removed per
  `260510-skeleton-contract-populator-flow`): the roster/listing lines were
  deleted, but the descriptive paragraph was **rewritten to a short
  "deprecated"/"removed" note that keeps the anchor**, not deleted outright
  (mental-model L74: "`lead-write-skeleton` is deprecated and its `SKILL.md`
  was removed ... Do not route new work through skeleton artifacts.
  {#260510-skeleton-contract-populator-flow}"). Follow the same
  mark-removed pattern for `lead-verify-design` instead of deleting the
  paragraph, for consistency with existing repo convention. The contract
  itself allows either ("remove or clearly mark-removed").
- `ai-docs/mental-model/workflow-skills.md#L70` — the matching mental-model
  bullet with the same anchor; apply the same mark-removed treatment as the
  spec paragraph, referencing that the function is now covered by the
  ticket's sage-review gate (per the epic ticket's stated rationale).
- `ai-docs/ref/wsflow-mirroring.md#L45` — `` `lead-verify-design` `` listed
  under "Shipped wsflow Skills / Included"; delete this one list line (do not
  add it to "Excluded" — it no longer exists in either package).
- `ai-docs/_index.md#L351-L354` — `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`
  entry's "Live follow-ups" sentence lists "the `lead-verify-design`
  inline-reviewer model/tier path" as a deferred follow-up. This follow-up is
  now moot once the skill is deleted; light-edit to drop that clause from the
  comma-separated list (grammar-adjust the remaining list).
- `ai-docs/tickets/todo/260630-epic-skill-playbook-diet.md#L111-L117` —
  "### Deletion candidate (not diet)" section already records the
  delete-don't-diet decision for `lead-verify-design` and explicitly says
  "Actual deletion is a separate approval step, not covered by this
  ticket-scoping edit." No `### Result` marker exists on this ticket, but the
  contract treats this text as frozen phase-plan text (append-only). Since
  deletion is now happening, append a short dated status note directly after
  this bullet (do not rewrite the existing bullet) recording that the
  deletion was executed; do not add a `#### Edition` header unless the
  executor's ticket-convention check calls for that specific form.
- `ai-docs/tickets/todo/260703-chore-review-delegates-true-classification.md#L23-L26` —
  the audit-example clause: "...(conditional), unlike `lead-verify-design`'s
  unconditional \"isolate a fresh deep reviewer\" delegation (also
  `delegates:true`)." Light-edit only this clause to add a short parenthetical
  noting the skill was later deleted rather than reclassified (e.g., "— the
  skill was later deleted rather than reclassified, see
  `260630-epic-skill-playbook-diet`"). Leave `## Scope`/`## Out of Scope` and
  the L36-38 historical roster line untouched (see Out of Scope above).

## Implementation Plan

1. Edit `agents-plugin/skills/lead-verify-discussion/SKILL.md#L25-L28`: after
   line 25 (step 3), insert the new step 4 with the exact contract wording,
   then renumber old steps 4/5/6 to 5/6/7.
2. Apply the identical edit to
   `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md#L25-L28`.
3. `diff` the two files to confirm byte-identical output (no manual namespace
   substitution needed — confirmed no `ws:`/`ws/`/`ws.` tokens in either file
   or the new text).
4. Delete `agents-plugin/rsrc/lead-verify-design/` (directory).
5. Delete `agents-plugin-wsflow/skills/lead-verify-design/` (directory).
6. Delete `agents-plugin-wsflow/rsrc/lead-verify-design/` (directory).
7. Remove the `"lead-verify-design/lead-verify-design.md": "..."` line from
   `agents-plugin/rsrc/manifest.json#L31` and the identical line in
   `agents-plugin-wsflow/rsrc/manifest.json#L31`; `diff` the two manifests
   afterward to confirm they stay identical.
8. Edit `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1910-L1927`:
   delete `TestPlaybookPrintGoldenLeadVerifyDesign` in full (comment + func).
9. Edit `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L31`: remove
   `"lead-verify-design",` from `EXPECTED_SKILLS`.
10. Edit `ai-docs/spec/workflow-skills.md`:
    - `#L40`: remove the `lead-verify-design` roster line from the code block.
    - `#L59`: remove `` `lead-verify-design`, `` from the internal-procedures
      comma list, fixing surrounding grammar.
    - `#L190`: remove `` `lead-verify-design`, `` from the wsflow Skill
      Surface prose list.
    - `#L475-L485`: rewrite the paragraph to a short mark-removed note that
      keeps the `{#260524-design-verification-skill}` anchor, mirroring the
      `lead-write-skeleton` precedent at `#L497` (state the skill is removed,
      its premise-gated function is covered by the ticket sage-review gate,
      and new work should not route through it).
11. Edit `ai-docs/mental-model/workflow-skills.md#L70`: same mark-removed
    rewrite, keeping the anchor, mirroring the `lead-write-skeleton` bullet at
    `#L74` in the same file.
12. Edit `ai-docs/ref/wsflow-mirroring.md#L45`: remove the
    `` - `lead-verify-design` `` line from the Included list.
13. Edit `ai-docs/_index.md#L351-L354`: drop "the `lead-verify-design`
    inline-reviewer model/tier path" clause from the Live follow-ups
    sentence, adjusting the remaining comma list grammar.
14. Append a short dated status note (append-only, no rewrite of existing
    text) after
    `ai-docs/tickets/todo/260630-epic-skill-playbook-diet.md#L111-L117`'s
    "Deletion candidate (not diet)" bullet, recording that the deletion was
    executed.
15. Light-edit
    `ai-docs/tickets/todo/260703-chore-review-delegates-true-classification.md#L25-L26`:
    append a short parenthetical to the audit-example clause noting the
    skill was later deleted rather than reclassified. Leave the rest of the
    ticket (Scope/Out of Scope, L36-38 roster line) untouched.
16. Grep the full repo (excluding `.worktree/*`) for `lead-verify-design`;
    confirm every remaining hit is inside `ai-docs/tickets/.done/`,
    `ai-docs/.plans/`, or `ai-docs/tickets/idea/`.

## Verification Plan

- `go test ./internal/mcp/... -run TestPlaybookPrintGolden` (or the package's
  full suite) from `agents-plugin-tool/`; read full output, confirm no
  failures and no leftover reference to `TestPlaybookPrintGoldenLeadVerifyDesign`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — required because
  step 9 edits `test_wsflow_skill_bundle.py`; this is the documented
  static-verification command in `ai-docs/ref/wsflow-mirroring.md#L229-L237`
  and the contract's "no other automated test suite applies" statement is
  incorrect for this file (see Codebase Findings risk signal above).
- `go test ./internal/wsrsrc/...` (plain run, no regen env vars) — confirms
  `TestWsflowRsrcMirrorUpToDate` and `TestWsflowSkillsMirrorUpToDate` still
  pass given the identical deletions applied to both rsrc/manifest mirrors.
- `diff agents-plugin/skills/lead-verify-discussion/SKILL.md agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md`
  — must produce no output.
- `diff agents-plugin/rsrc/manifest.json agents-plugin-wsflow/rsrc/manifest.json`
  — must produce no output.
- `grep -rn "lead-verify-design" . --exclude-dir=.git --exclude-dir=.worktree`
  — every hit must be under `ai-docs/tickets/.done/`, `ai-docs/.plans/`, or
  `ai-docs/tickets/idea/`.

## Escalations

- None.
