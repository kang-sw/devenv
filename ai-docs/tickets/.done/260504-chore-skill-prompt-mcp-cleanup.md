---
title: Skill and prompt MCP primitive cleanup
parent: 260503-epic-ws-mcp-vcs-reference-tools
spec:
  - 260428-workflow-skill
related:
  260504-feat-ws-mcp-reference-discovery-tools: provides the reference discovery primitives used by this cleanup
completed: 2026-05-04
---

# Skill and prompt MCP primitive cleanup

## Background

The VCS and reference discovery MCP tools now cover the ws-owned operations that
shared skills previously described through direct shell commands, host-specific
tool names, or manual file search wording. The remaining cleanup is to make
`agents-plugin` skill and embedded prompt text depend on those primitives instead
of restating shell recipes such as direct `git` commands, `find | grep`, `Glob`,
`Grep`, or `sed` where the operation is about workflow metadata rather than
project code.

## Decisions

- Centralize tool selection and usage instructions in `ws:lead-workflow`.
- Keep individual skills terse: name the relevant skill or MCP primitive and only
  include example arguments when the local step needs them.
- Preserve ordinary shell guidance for project-specific code search, build,
  test, and mechanical editing tasks.
- Do not convert reference documents that intentionally document CLI fallbacks.

## Phases

### Phase 1: Normalize shared skill and prompt references

Scan `agents-plugin/skills/` and embedded prompt sources for host-specific
workflow metadata operations. Replace ws-owned direct shell, `Bash`, `Grep`,
`Glob`, `sed`, and manual stem-search wording with MCP primitive references.
Move detailed usage guidance into `ws:lead-workflow`; leave individual skills
with primitive names and minimal example parameters only.

Success criteria:

- `ws:lead-workflow` contains the primary guidance for Git and reference
  discovery primitive usage.
- Other `agents-plugin` skills avoid detailed tool recipes for those primitives.
- Embedded prompts prefer MCP primitives for workflow metadata where available.
- Verification includes text search for remaining direct shell wording and a
  prompt bundle/runtime metadata check if embedded prompt files change.

### Result (pending) - 2026-05-04

Normalized shared `agents-plugin` skills and embedded prompt text around the new
MCP Git and reference discovery primitives. `ws:lead-workflow` now carries the
central usage guidance for ticket/spec/mental-model/reference lookup and
`ws/git.*`; individual skills name those primitives with only local arguments.

Updated workflow skills for exit-session, update-spec, write-ticket, discuss,
write-skeleton, sprint, ship, write-spec, and forge-spec where the wording was
about ws-owned metadata operations. Preserved native Git wording for operations
without an exposed ws primitive, such as branch creation, tag push, merge
execution, path-filtered file history, and explicit `git mv` moves.

Updated embedded reviewer, implementer, project-survey, plan-populator, and
mental-model-updater prompts to prefer `ws/git.*`, `ws/tickets.*`,
`ws/specs.*`, and `ws/mental_models.*` for workflow metadata. Refreshed the
prompt bundle hash in `agents-plugin/runtime.json` and mirrored `lead-workflow`
plus `runtime.json` into the local Codex plugin cache.

Verification:

- `cd agents-plugin-tool && go test ./...`
- `git diff --check`
- Text scan for remaining direct shell wording in `agents-plugin/skills` and
  `agents-plugin-tool/internal/wsprompt`
- Local Codex plugin cache `cmp` for `lead-workflow/SKILL.md` and `runtime.json`
