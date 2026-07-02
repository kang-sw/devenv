---
title: Host-neutral ws plugin architecture
completed: 2026-06-09
---

# Host-neutral ws plugin architecture

## Disposition (2026-06-09): absorbed by the playbook-factory pivot

This research anchor is **absorbed and superseded** by
`260605-epic-ws-playbook-factory-pivot` (direction detail in
`260605-research-ws-native-subagent-pivot`). The host-neutral direction it
opened — playbook factory, rsrc plain-text prompt distribution, harness-aware
routing as data, total spawn removal in favor of harness-native subagents — is
now owned by that epic's M0–M4 milestones. Moved to `.done/` as a completed
direction-setting investigation, not dropped: its framing carried forward intact.
The background below is retained for historical context.

## Background

The current ws plugin is packaged and documented as a Claude Code plugin under
`claude-plugin/`. Most of the durable workflow knowledge lives in Markdown skills
and infra prompts, which are relatively portable. The fragile parts are the runtime
assumptions embedded around those documents: Claude-specific slash commands,
`$ARGUMENTS`, shell interpolation, named tool labels, hook payloads, session paths,
model tier names, and implicit `ws-*` availability through Claude plugin install
behavior.

The original compatibility question was whether the existing Claude plugin could be
made drop-in compatible with Codex by adding Codex plugin metadata. That is possible
at the packaging layer, but it leaves the behavior layer dependent on Claude Code
idioms and user-local PATH setup. Because this plugin is intended for users beyond
the original author, the long-term direction is still a host-neutral ws plugin with
thin Claude and Codex adapters.

The first implementation slice did not mutate `claude-plugin/` in place. It kept
the existing Claude plugin isolated while creating a parallel `agents-plugin/`
directory for Codex-first porting and validation. The current direction freezes
`claude-plugin/` as legacy fallback while later work decides which remaining
fallback surfaces migrate, stay unsupported, or are removed.

## Current Findings

Codex plugin packaging expects a `.codex-plugin/plugin.json` manifest and can load a
repo-local marketplace entry. Claude plugin packaging expects `.claude-plugin/`
metadata. Both can point at the same plugin directory if the shared files avoid
host-specific assumptions, but that should be treated as a later convergence target,
not the first migration step.

The Codex `apps` facility is for OpenAI-registered app/connectors such as GitHub,
Linear, or Figma. It is not a local executable or PATH injection mechanism.

Codex hooks can inject developer context at session start, but official docs and
observed issues do not establish plugin-local hooks as a reliable way to mutate the
host shell environment. MCP server configuration can set the environment for the MCP
server process, but it cannot mutate Codex's shell tool PATH. Therefore `ws-*`
availability should not rely on Codex plugin install side effects.

Codex has subagents and a planning/checklist facility, but it does not expose
Claude's exact `Task` tool idiom. Shared skill documents should describe behavior
such as maintaining a visible task list, delegating independent exploration, or using
available search/edit capabilities instead of naming host-specific tools.

## Revised Proposed Shape

Create a parallel Codex-first plugin candidate:

```text
claude-plugin/    # frozen legacy Claude fallback; do not mirror Codex changes
agents-plugin/    # Codex-first host-neutral port candidate
```

`agents-plugin/` should remain the isolated workspace for adapting manifests,
skills, prompts, helper access, and root context to Codex. It may copy or subset
material from `claude-plugin/`, but copied content should be normalized as it
enters the active tree rather than forcing host-neutral edits back into the
frozen Claude fallback.

Codex validation is the first completion gate:

- Codex can discover and load the plugin candidate.
- The candidate exposes at least one usable workflow entry point or skill.
- The candidate does not depend on Claude plugin PATH injection for `ws-*`
  availability.
- `AGENTS.md` and any plugin-local context point to the current mixed-state
  authority boundary.

Claude compatibility is a best-effort second pass:

- Use the current `claude-plugin/` layout only as frozen legacy fallback.
- Add Claude-compatible metadata to `agents-plugin/` only where it does not
  compromise Codex behavior.
- Do not declare Claude compatibility complete until it is verified in a real
  Claude session by the user.

## First Slice Status

The first implementation slice is
`260502-feat-agents-plugin-codex-port-scaffold`. It created `agents-plugin/` as a
parallel Codex-first `ws` candidate while leaving `claude-plugin/` untouched.

Current validation boundary:

- Codex marketplace registration and manual UI install were verified.
- `$ws:lead-skill-authoring` is available in Codex after install.
- A Claude-facing `.claude-plugin/plugin.json` was added and
  `claude plugin validate agents-plugin` passes.
- Runtime Claude invocation remains a later manual closeout; do not treat the
  candidate as Claude-compatible beyond manifest validation.

## Later Convergence Target

Move toward a single plugin directory or retire the legacy Claude package after
all live fallback surfaces have migrated:

```text
plugin/
  .codex-plugin/
    plugin.json
  .claude-plugin/
    plugin.json
  skills/
  infra/
  bin/
  hooks/
  .mcp.json
```

The shared `skills/`, `infra/`, and `bin/` trees become the durable source if
Claude remains supported. If no Claude runtime support remains, remove
`claude-plugin/` after installer, docs, tests, and fallback references stop
depending on it. In either path, `claude-plugin/` is no longer an active mirror
for Codex workflow edits.

## Skill Normalization Direction

Keep `SKILL.md` as the common authoring format where possible. Normalize frontmatter
to the intersection that both hosts can tolerate:

```yaml
---
name: write-code
description: Implement a scoped code change using the ws workflow.
---
```

Rewrite common skill bodies from tool-name instructions to behavior instructions:

- Replace `$ARGUMENTS` with explicit topic/target handling language.
- Replace `/write-spec` or `/implement` chains with "use the write-spec skill" or
  "continue through the configured workflow".
- Replace Claude shell interpolation such as ``!`ws-proj-tree` `` with ordinary
  instructions to run a helper when available.
- Replace `TaskCreate`, `TaskUpdate`, `TaskList`, and "Use the Task tool" with
  "maintain a visible task list and update statuses as work progresses".
- Replace Claude tool names such as `WebSearch`, `WebFetch`, `Read`, `Edit`, and
  `Bash` with host-neutral capabilities.
- Move Claude model tier names such as `sonnet`, `opus`, and `haiku` out of shared
  skill text unless they are purely examples in Claude-specific documentation.

## Runtime Direction

Separate the durable ws behavior from host integration:

```text
ws-core      # Python package or equivalent shared implementation
ws-mcp       # MCP server exposing ws tools/resources/prompts
bin/ws-*     # thin CLI wrappers around ws-core
```

MCP should become the primary machine interface for host integrations. CLI wrappers
remain useful for people, shell scripts, existing documentation, and compatibility
with current ws workflows, but shared skill behavior should not require host PATH
mutation as the only way to reach plugin functionality.

Agent orchestration should be expressed behind a backend abstraction:

```text
AgentBackend
  CodexBackend
  ClaudeBackend
  LocalSubprocessBackend
```

This matches the existing direction in `ws-named-agent`, which already contains
some Codex support, while avoiding a design where Claude session semantics define
the core contract.

## Open Questions

- Which remaining `claude-plugin/` surfaces must migrate before the frozen
  legacy package can be removed?
- Which `ws-*` commands are essential enough to expose through MCP first?
- Should the installer create `~/.local/bin` symlinks for CLI compatibility, or
  should public Codex usage avoid CLI dependency entirely at first?
- How should `ws-named-agent` choose Claude vs Codex backends without relying on
  model-name inference such as `sonnet` or `codex`?
- Which Claude hooks remain valuable after the shared runtime moves toward MCP, and
  which should become Claude-only convenience behavior?
- What is the exact user verification checklist for the later Claude compatibility
  closeout?

## Next Step

Choose the next actionable slice after the scaffold: manual Claude closeout,
`skill-authoring` quality hardening, first real workflow-skill port, or MCP/`ws-*`
runtime exposure.
