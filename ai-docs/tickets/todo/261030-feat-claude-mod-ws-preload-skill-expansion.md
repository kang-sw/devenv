---
title: ws-preload - expand lead skill shims with their playbook at skill-load time
---

# ws-preload - expand lead skill shims with their playbook at skill-load time

## Background

Every `lead-*` skill is a thin shim whose body tells the model to call
`ws/playbook.read(name: "<skill>")` and `ws/workflow_manual(...)` in parallel
and then execute the returned procedure (e.g.
`agents-plugin/skills/lead-discuss/SKILL.md`). Each invocation therefore spends
one extra model round trip on reads the harness could do itself.

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
  (and the session state, since the manual is already in the system prompt);
  a harness without expansion (Codex, mod off, an expansion failure) leaves
  the block, and the model follows the prose as today. Rejected: a
  self-closing `<skill-call>` directive (leaves no fallback to remove
  atomically; the block preloads tool results rather than calling a skill).
- **Allowlist only.** Expansion runs only for skills whose `pluginName` is the
  ws or wsflow plugin, and only for read-only tools (`playbook.read`,
  `workflow_state`). Rejected: generic execution of any tool named in any
  skill body (`skill.prompt` also fires for project-settings and MCP skills,
  which would run tool calls without the model's consent).
- **Failure passes through.** Any expansion error returns the original text
  unchanged, like the existing `rewriteRead` hooks' `.catch(($, e, next) =>
  next(e))`.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` (every lead shim and its wsflow
  mirror changes).

## Phases

### Phase 1: Claude mod expansion

Add the `<ws-preload>` block to the ws and wsflow lead skill shims and a
`skill.prompt` hook in the claude-mod that expands it per the decisions.

Before coding, check whether `skill.prompt` also fires for a subagent's skill
load and whether the hook can tell it apart; stop back to the lead if it
fires and cannot.

Verification: claude-mod engine tests for expansion, the allowlist (a
non-ws skill and a non-allowlisted tool are left untouched), and failure
pass-through; a manual check that `/ws:lead-discuss` reaches the playbook
without a model-issued `playbook.read`.

### Phase 2: Pi expansion

Expand the same block in the Pi adapter's `ws-skill` tool. Before coding,
check whether Pi offers a hook on the user-typed `/skill:<name>` expansion
path; if it does not, only the `ws-skill` path expands, and the gap is
reported in the Result.

Verification: Pi adapter tests for `ws-skill` expansion and pass-through.
