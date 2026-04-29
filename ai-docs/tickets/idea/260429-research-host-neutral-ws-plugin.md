---
title: Host-neutral ws plugin architecture
---

# Host-neutral ws plugin architecture

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
the original author, the better direction is to make the ws plugin host-neutral and
then expose thin Claude and Codex adapters.

## Current Findings

Codex plugin packaging expects a `.codex-plugin/plugin.json` manifest and can load a
repo-local marketplace entry. Claude plugin packaging expects `.claude-plugin/`
metadata. Both can point at the same plugin directory if the shared files avoid
host-specific assumptions.

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

## Proposed Shape

Move toward a single plugin directory:

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

The shared `skills/`, `infra/`, and `bin/` trees become the durable source. The
Claude and Codex manifest directories become host adapters. Avoid introducing a
separate overlay tree until there is a concrete need, because overlays would create
another documentation drift surface.

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

- Should the first implementation milestone be a pure packaging move
  (`claude-plugin/` to `plugin/` plus Codex manifest), or should packaging wait
  until the highest-risk skill idioms are normalized?
- Which `ws-*` commands are essential enough to expose through MCP first?
- Should the installer create `~/.local/bin` symlinks for CLI compatibility, or
  should public Codex usage avoid CLI dependency entirely at first?
- How should `ws-named-agent` choose Claude vs Codex backends without relying on
  model-name inference such as `sonnet` or `codex`?
- Which Claude hooks remain valuable after the shared runtime moves toward MCP, and
  which should become Claude-only convenience behavior?

## Next Step

Promote this idea into one or more actionable tickets only after choosing the first
slice. A conservative first slice is packaging-only: rename `claude-plugin/` to
`plugin/`, preserve existing Claude behavior, add `.codex-plugin/plugin.json`, and
add a Codex marketplace entry that points at `./plugin`. A more durable first slice
is skill normalization: keep the directory layout unchanged while removing
host-specific tool names from the most-used skills.
