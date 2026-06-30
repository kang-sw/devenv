# Plan: 260630-feat-lead-skill-parallel-init — Phase 2: SKILL.md parallel entry + playbook On: invoke simplification

## Relevant Ticket Contract

- **Change A — SKILL.md for lead-discuss and lead-sprint:** Replace the one-line
  `ws/playbook.print` stub with a parallel entry declaration: call
  `ws/playbook.print(name: "<skill-name>", session_key: <your key>)` and
  `ws/workflow_manual(session_key: <your key or obsidian-latch>, root: <path if fresh>)`
  in parallel; after both return, execute the procedure returned by `playbook.print`.
- **Change B — lead-discuss and lead-sprint On: invoke playbook sections:**
  Remove the sequential `workflow_manual`, `project_tree`, `git.status` init
  steps. Replace with: call `project_tree(session_key)` and `git.status(session_key)`
  in parallel, then continue to user message handling. Preserve post-compaction
  reload instruction: if session compaction occurred, call `workflow_manual(session_key)`
  immediately before step 1; if key is lost, run `ws:lead-revive` first.
- **Scope constraint:** lead-discuss and lead-sprint only. lead-proceed and
  lead-implement are not modified (no project-context init in their On: invoke).
- **Mirror constraint:** agents-plugin and agents-plugin-wsflow rsrc mirrors must
  both be updated.
- **Spec update required:** `ai-docs/spec/workflow-skills.md` anchor
  `260610-entry-skill-surface-reduction` must be updated to reflect that context-heavy
  entry skills carry a parallel init declaration rather than a pure routing stub.
- **Sentinel exposure:** SKILL.md may use the literal `"obsidian-latch"` sentinel
  string. The per-file no-expose rule is stale and removed.
- **Inter-phase dependency:** Phase 2 SKILL.md prose references workflow_manual's
  key-return behavior (fresh-start path with `root`). Phase 1 (now complete as of
  0b67515d) is the prerequisite; this phase may be written independently.
- **Verification:** After both phases, a continue-path lead-discuss invocation
  completes init in 2 MCP rounds. Fresh-start completes in 2 rounds. Parallel call
  declaration is present in updated lead-discuss/SKILL.md and lead-sprint/SKILL.md.

## Out of Scope

- lead-proceed and lead-implement playbooks — ticket explicitly excludes them.
- Phase 1 changes (workflow_manual Go handler, tool schema, tests) — already complete.
- agents-plugin-tool Go source — no backend changes needed for this phase.
- lead-discuss and lead-sprint On: user message and other handlers — unchanged.
- The "Project Map" section at the top of lead-sprint playbook (a standalone
  `project_tree()` call outside On: invoke) — the ticket only modifies the On: invoke
  block; the Project Map section is left as-is.

## Codebase Findings

- `agents-plugin/skills/lead-discuss/SKILL.md#L1-L9` — current stub: single
  `ws/playbook.print(name: "lead-discuss")` call with no session_key, no
  workflow_manual call, no parallel declaration. Both SKILL.md files (agents-plugin
  and agents-plugin-wsflow) are identical and must both be replaced.
- `agents-plugin/skills/lead-sprint/SKILL.md#L1-L9` — same pattern as lead-discuss.
- `agents-plugin-wsflow/skills/lead-discuss/SKILL.md#L1-L11` — wsflow mirror; same
  pattern with `wsflow/playbook.print` namespace prefix instead of `ws/`.
- `agents-plugin-wsflow/skills/lead-sprint/SKILL.md#L1-L11` — wsflow mirror.
- `agents-plugin/rsrc/lead-discuss/lead-discuss.md#L44-L49` — On: invoke (lines 44–48
  in the rsrc): step 1 calls `workflow_manual` sequentially (continue or obsidian-latch
  fresh), step 2 calls `project_tree`, step 3 calls `git.status`. Both rsrc mirrors
  (agents-plugin and agents-plugin-wsflow) are identical and must both be updated.
- `agents-plugin/rsrc/lead-sprint/lead-sprint.md#L34-L43` — On: invoke: step 1 calls
  `workflow_manual`, step 2 calls `git.status`, step 3 calls `project_tree`. Same
  update required in both mirrors.
- `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md#L44-L49` — wsflow mirror;
  identical content to agents-plugin rsrc.
- `agents-plugin-wsflow/rsrc/lead-sprint/lead-sprint.md#L34-L43` — wsflow mirror;
  identical content to agents-plugin rsrc.
- `ai-docs/spec/workflow-skills.md#L63-L66` — spec anchor `260610-entry-skill-surface-reduction`:
  states "the SKILL.md surface carries only the trigger description and delegates
  execution to its playbook." Must be updated to carve out the context-heavy entry
  skill exception: for skills like lead-discuss and lead-sprint, SKILL.md carries a
  parallel init declaration (playbook.print + workflow_manual) rather than a pure
  routing stub.
- **Namespace divergence:** agents-plugin SKILL.md files use `ws/playbook.print`
  (no session_key in the existing stub); agents-plugin-wsflow SKILL.md files use
  `wsflow/playbook.print`. The new parallel declaration must respect this: `ws/`
  prefix in agents-plugin, `wsflow/` prefix in agents-plugin-wsflow. The rsrc
  playbooks use `{{.McpNamespace}}` template vars and are namespace-agnostic.
- **session_key in playbook.print:** The ticket's Change A includes
  `ws/playbook.print(name: "<skill-name>", session_key: <your key>)`. The current
  stubs do not pass session_key. This is intentional per the ticket — the parallel
  entry adds session_key to the playbook.print call so the playbook renderer can
  include session-scoped state in the returned procedure.
- **lead-sprint "Project Map" section:** lead-sprint rsrc has a standalone
  `## Project Map` / `Call {{.McpNamespace}}/project_tree()` block at lines 12–13
  (before On: invoke). This is a separate named section, not part of On: invoke, and
  the ticket does not modify it. Leave it untouched.
- **Post-compaction reload phrasing:** The current On: invoke step 1 embeds the
  compaction/revive guidance inline ("reload after session compaction... recover
  your key via lead-revive first... No lead key yet (fresh start)? Call
  obsidian-latch..."). The new On: invoke replaces steps 1–3 with a two-step
  parallel block plus a separate post-compaction note. The session-key recovery path
  (lead-revive) must be preserved in the post-compaction note; the fresh-start
  bootstrap path moves to SKILL.md.

## Implementation Plan

1. **Update agents-plugin/skills/lead-discuss/SKILL.md**: Replace the single
   `ws/playbook.print(name: "lead-discuss")` stub body with:
   ```
   Call in parallel:
   - ws/playbook.print(name: "lead-discuss", session_key: <your key>)
   - ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)

   After both return, execute the procedure returned by ws/playbook.print.
   ```

2. **Update agents-plugin/skills/lead-sprint/SKILL.md**: Same parallel entry
   substitution as step 1, with `name: "lead-sprint"`.

3. **Update agents-plugin-wsflow/skills/lead-discuss/SKILL.md**: Same as step 1 but
   using `wsflow/playbook.print` and `wsflow/workflow_manual` namespace prefixes (and
   drop the existing "If the playbook cannot be loaded, stop and report that blocker"
   fallback line — that line is not present in agents-plugin and is not part of the
   ticket change).

4. **Update agents-plugin-wsflow/skills/lead-sprint/SKILL.md**: Same as step 2 with
   `wsflow/` prefixes.

5. **Update lead-discuss rsrc On: invoke** (both mirrors:
   `agents-plugin/rsrc/lead-discuss/lead-discuss.md#L44-L49` and
   `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md#L44-L49`):
   Replace steps 1–3 with:
   ```
   ## On: invoke

   1. Call `{{.McpNamespace}}/project_tree(session_key: <your key>)` and
      `{{.McpNamespace}}/git.status(session_key: <your key>)` in parallel.
   2. If `user request` references a ticket, read it.
   3. Enter user-message handling.

   Post-compaction reload: if session compaction occurred, call
   `{{.McpNamespace}}/workflow_manual(session_key: <your key>)` immediately before
   step 1 to restore session state. If the key is lost, run
   `{{.SkillNamespace}}:lead-revive` first to recover it.
   ```
   Note: keep step numbers consistent — original steps 4 and 5 (read ticket, enter
   user-message handling) become new steps 2 and 3.

6. **Update lead-sprint rsrc On: invoke** (both mirrors:
   `agents-plugin/rsrc/lead-sprint/lead-sprint.md#L34-L43` and
   `agents-plugin-wsflow/rsrc/lead-sprint/lead-sprint.md#L34-L43`):
   Replace steps 1–3 with:
   ```
   ## On: invoke

   1. Call `{{.McpNamespace}}/project_tree(session_key: <your key>)` and
      `{{.McpNamespace}}/git.status(session_key: <your key>)` in parallel.
   2. Recover episode state from active conversation or recent `Sprint-Edit:` commit markers.
   3. If recovery finds one open episode, set `<current-edit-context>`, `<episode-slug>`,
      and `<episode-start>` from it.
   4. If recovery is empty or ambiguous, initialize `<current-edit-context>`,
      `<episode-slug>`, and `<episode-start>` as empty.
   5. Enter session loop.

   Post-compaction reload: if session compaction occurred, call
   `{{.McpNamespace}}/workflow_manual(session_key: <your key>)` immediately before
   step 1 to restore session state. If the key is lost, run
   `{{.SkillNamespace}}:lead-revive` first to recover it.
   ```
   Note: original steps 4–6 (recover episode state, set state, initialize empty)
   become new steps 2–4.

7. **Update spec anchor** (`ai-docs/spec/workflow-skills.md#L63-L66`):
   In the `260610-entry-skill-surface-reduction` paragraph, after "the SKILL.md
   surface carries only the trigger description and delegates execution to its
   playbook", add a qualification: context-heavy entry skills (lead-discuss and
   lead-sprint) carry a parallel init declaration — `playbook.print` plus
   `workflow_manual` in parallel — rather than a pure routing stub, to reduce init
   round-trips from 4–5 serial calls to 2 parallel rounds.

## Verification Plan

- Manual inspection: confirm `agents-plugin/skills/lead-discuss/SKILL.md` and
  `agents-plugin/skills/lead-sprint/SKILL.md` contain the parallel declaration and
  include both `playbook.print` and `workflow_manual` calls.
- Manual inspection: confirm both `agents-plugin-wsflow` SKILL.md mirrors carry the
  same declaration with `wsflow/` namespace prefixes.
- Manual inspection: confirm On: invoke in all four rsrc files (agents-plugin and
  agents-plugin-wsflow for each of lead-discuss and lead-sprint) no longer contains
  the sequential `workflow_manual` step; only the post-compaction reload note
  references `workflow_manual`.
- Manual inspection: confirm `agents-plugin/rsrc` and `agents-plugin-wsflow/rsrc`
  On: invoke sections are functionally identical (only `{{.McpNamespace}}` /
  `{{.SkillNamespace}}` template vars, which resolve to the right prefix at render time).
- Manual inspection: confirm spec anchor `260610-entry-skill-surface-reduction`
  in `ai-docs/spec/workflow-skills.md` reflects the parallel init exception.
- No build or unit tests required for this phase — changes are doc/prompt-only.

## Escalations

- None.
