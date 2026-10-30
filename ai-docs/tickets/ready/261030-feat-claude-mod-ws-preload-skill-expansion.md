---
title: ws-preload - expand lead skill shims with their playbook at skill-load time
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: fbd2fad9ff60b2c7
sage-review-completeness-reviewed: fbd2fad9ff60b2c7
---

# ws-preload - expand lead skill shims with their playbook at skill-load time

## Background

Every `lead-*` skill except `lead-revive` is a thin shim whose body tells the
model to call `ws/playbook.read(name: "<skill>")` and then execute the returned
procedure; four of them (`lead-delegate`, `lead-discuss`, `lead-run`,
`lead-use-mailbox`) call it in parallel with `ws/workflow_manual(...)`, e.g.
`agents-plugin/skills/lead-discuss/SKILL.md`. `lead-ticket` calls
`workflow_manual` only on a fresh session, six more (`lead-audit-doc`,
`lead-bootstrap`, `lead-coauthor-paged-document`, `lead-review`, `lead-ship`,
`lead-tune`) call `playbook.read` alone, and `lead-revive` is an inline body
that calls only `workflow_manual` (`agents-plugin/skills/*/SKILL.md`). Each
invocation therefore spends one extra model round trip on reads the harness
could do itself.

Verified hooks:

- Claude Code mod API: the `skill.prompt` event "fires when the engine
  expands a skill's prompt for the model (`/name`, the Skill tool, a
  preload)"; a hook returning `{ text }` replaces what the model reads. Its
  input carries `skill`, `text`, `frontmatter`, `source`, `pluginName?`, and
  `path?` (`claude-code.d.ts`, `SkillPromptInput`).
- The ws claude-mod already calls ws tools through `$.mcp.call`, knows the
  conversation's lead key, puts the workflow manual in the system prompt, and
  answers a main-thread `workflow_manual` call with session state only
  (`agents-plugin/hooks/claude-mod/register.ts`, `manual.ts`).
- Pi: the lead loads skills through the adapter-owned `ws-skill` tool
  (`agents-plugin-pi/src/lead-skills.ts`), so the adapter can expand there
  directly.

## Decisions

- **Wrapper marker with the fallback inside.** A shim marks its preload as a
  block that wraps today's fallback prose:

  ```text
  <ws-preload playbook="lead-discuss">
  Call in parallel:
  - `ws/playbook.read(name: "lead-discuss", session_key: <your key, omit if fresh>)`
  - `ws/workflow_manual(...)`

  After both return, execute the procedure returned by `ws/playbook.read`.
  </ws-preload>
  ```

  An expanding harness replaces the whole block with the rendered playbook
  body (see "Subagent preloads see only the playbook body" for what is not
  inlined);
  a harness without expansion (Codex, mod off, an expansion failure) leaves
  the block, and the model follows the prose as today. Rejected: a
  self-closing `<skill-call>` directive (leaves no fallback to remove
  atomically; the block preloads tool results rather than calling a skill).
- **Allowlist only.** Expansion runs only for skills whose `pluginName`
  equals the mod's own `$.plugin.name`, so the ws mod expands ws skills and
  the wsflow mod expands wsflow skills; no package name is spelled
  (`register.ts` is mirrored byte-for-byte). Expansion uses only the
  read-only `playbook.read`. Because the mod holds a lead key only with
  `claude.manual_injection_enabled` on (default true for ws, false for
  wsflow), wsflow blocks pass through by default under the no-key rule. Rejected: generic execution of any tool named in any
  skill body (`skill.prompt` also fires for project-settings and MCP skills,
  which would run tool calls without the model's consent).
- **Subagent preloads see only the playbook body.** `SkillPromptInput`
  carries no `agentId`, and the type doc says `skill.prompt` also fires for a
  skill "preloaded into a subagent", so the hook cannot tell a lead load from
  a subagent preload. Expansion therefore inlines only the playbook body,
  read with `playbook.read(name, session_key: <the conversation's lead
  key>)` so that project prompt overrides and `workflow.lang` apply. It never
  inlines session state (agenda, todos, notes) or the session key. The main
  thread still gets its state through the system-prompt manual and its state
  pointer (`manual.ts`, `STATE_POINTER_LINE`). `lead-use-mailbox` is
  excluded from expansion, because its body substitutes a key-specific
  mailbox wait command (`server.go`, `injectMailboxWaitCommand` on the
  `playbook.read` path). A subagent that preloads a lead skill thus sees
  only text it could read itself. Rejected: detecting subagents (no signal
  in the input); a keyless read (loses overrides and language); inlining
  `workflow_state` (it would hand the lead's agenda to a subagent). The lead
  decided this item on 2026-10-30 under the owner's overnight delegation of
  minor policy calls. It replaces the original "and the session state"
  clause.
- **Failure passes through.** Any expansion error returns the original text
  unchanged, like the existing `rewriteRead` hooks' `.catch(($, e, next) =>
  next(e))`.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` (every lead shim and its wsflow
  mirror changes).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/skills/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 260626-bug-wsflow-lead-revive-skill-inventory-drift (2026-06-26, Resolution): "`lead-revive` is a deliberate inline-body wsflow skill, not a thin `playbook.print` shim, because revive bootstraps the workflow primitives after compaction" — bearing: constrains
- 261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents (2026-10-30, Open Questions): "The server cannot tell a fresh lead from a subagent using the sentinel. The candidate signals (claude-mod `agentId` on `tool.call`...) are uninvestigated" — bearing: constrains
- 5b8d956c (2026-09-06, commit): "loadSkillFile (uses @earendil-works/pi-coding-agent's own parseFrontmatter for byte-identical stripping to Pi's /skill:<name> expansion)" — bearing: constrains
- 6d699b04 (2026-10-08, commit): "wsSkillKey ignores args rather than rejecting them so older session entries ... dedupe against a new name-only call" — bearing: constrains
- 67f3ed9f (2026-09-09, commit): "C2 fixed: ws-skill lookup/read failures bypass dedupe while retaining existing text." — bearing: constrains
- d1e13fab (2026-09-09, commit): "Shim over an rsrc body rather than an inline SKILL.md: the body uses `{{.ExploreAgent}}` and `{{.SpawnIdiom}}`, and those substitute only on the playbook.read / playbook.render path" — bearing: supports
- 670e37dd (2026-06-20, commit): "Skills are NOT byte-mirrored (EXPECTED_SKILLS is a curated set + independent shim files), so the wsflow shim description legitimately diverges from full-ws" — bearing: constrains
- adbf5ec3 (2026-07-25, commit): "Skill is deliberately NOT a lead-*/playbook.print shim: it must be readable with the MCP server down, so a playbook.print body would die exactly when the skill is needed." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/skills/*/SKILL.md, agents-plugin-wsflow/skills/*/SKILL.md, agents-plugin/hooks/claude-mod/register.ts, agents-plugin-wsflow/hooks/claude-mod/register.ts, agents-plugin-pi/src/lead-skills.ts |
| scope.surface | cross-module | skill shim text in two packages, the claude-mod hook, and the Pi adapter share one marker contract |
| scope.new_public_symbol | unknown | new hook and Pi expansion function names are not specified in the ticket |
| scope.new_type_contract | unknown | the `<ws-preload>` block is a shim-to-harness text contract; no type or signature is fixed yet |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/engine.test.ts, agents-plugin/hooks/claude-mod/manual-engine.test.ts, agents-plugin-pi/test/lead-skills.test.ts, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | callText over $.mcp.call (agents-plugin/hooks/claude-mod/register.ts#L503-L508) and the rewriteRead hook with its next(e) pass-through (register.ts#L929, L936) |
| complexity.side_effect_risk | moderate | a skill.prompt hook rewrites every skill's text and fires for subagent preloads and non-ws skills; input carries no agentId (claude-code.d.ts SkillPromptInput) |
| risk.correctness | moderate | block replacement must be atomic with a fallback intact on any failure, and subagent-preload discrimination is unsettled |
| risk.fit | moderate | skill shim shapes are heterogeneous (parallel, conditional, playbook-only, inline) and wsflow shims are curated, not mirrored |
| risk.test | moderate | skill.prompt is not yet exercised anywhere in the tree (grep of agents-plugin and agents-plugin-pi found no use) and the manual check needs a live session |
| risk.security_or_contract | moderate | executing tool calls from skill text; the allowlist must hold against non-ws plugin and project skills |

## Phases

### Phase 1: Claude mod expansion

Add the `<ws-preload>` block to the ws and wsflow lead skill shims and a
`skill.prompt` hook in the claude-mod that expands it per the decisions.

The block goes only into shims whose body calls `ws/playbook.read` for their
own playbook: the parallel-form shims, the `playbook.read`-only shims, and
`lead-ticket`, whose fresh-session `workflow_manual` bootstrap line stays
outside the block together with its "inline against the user request" phrase.
Excluded: `lead-revive` (it calls only `workflow_manual`), `lead-use-mailbox`
(excluded per the decisions), and the wsflow-only `lead-proceed` (a tombstone
that reads another playbook), `lead-check-blockers`, and
`lead-workflow-manual` unless their body reads their own playbook. Update
`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` shape checks and the
"uniform shim shape" wording in `ai-docs/manuals/wsflow-mirroring.md`. When the mod holds no
lead key for the conversation (a fresh session before bootstrap), the block
passes through unexpanded. The subagent-preload case is settled by the
decisions; no pre-check stop is needed for it.

Verification: claude-mod engine tests for expansion, the allowlist (a
non-ws skill and a non-allowlisted tool are left untouched), failure
pass-through, no session state or key in the expanded text, the
`lead-use-mailbox` exclusion, the no-key pass-through, and a block whose
`playbook` attribute does not name the skill being expanded passing through
unexpanded; the wsflow shim drift test
(`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`) passes; a manual check that `/ws:lead-discuss` reaches the playbook
without a model-issued `playbook.read`.

### Phase 2: Pi expansion

Expand the same block in the Pi adapter's `ws-skill` tool under the same
decisions: read with the lead key, inline only the playbook body (never
session state or the key), leave `lead-use-mailbox` unexpanded, and pass
through unchanged on error or when no lead key is held. `ws-skill` is also
given to forks (`addSkillToolIfLeadOrFork`): read with the process's own
key, and keep `dedupeRead`/`wsSkillKey` and the "failures bypass dedupe"
behavior (67f3ed9f) intact for an expansion failure. Before coding,
check whether Pi offers a hook on the user-typed `/skill:<name>` expansion
path; if it does not, only the `ws-skill` path expands, and the gap is
reported in the Result.

Verification: Pi adapter tests for `ws-skill` expansion, the exclusions,
and pass-through.
