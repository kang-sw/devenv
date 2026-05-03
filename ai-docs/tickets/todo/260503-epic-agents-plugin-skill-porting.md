---
title: agents-plugin skill porting roadmap
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260502-feat-agents-plugin-codex-port-scaffold: completed scaffold prerequisite
  260502-feat-agents-plugin-workflow-skill-drafts: completed first draft skill slice
---

# agents-plugin skill porting roadmap

## Background

`agents-plugin/` is now a loadable Codex-first `ws` plugin candidate with
`skill-authoring`, `write-ticket`, and `discuss` visible after Codex UI install.
The next migration work should not proceed by bulk-copying every Claude skill or by
starting with `/bootstrap`. Claude and Codex differ most sharply in the
orchestration layer: persistent named agents, hook interrupts, task lists,
branch/merge harnesses, and helper command availability.

This epic organizes the remaining skill porting work by workflow shape rather than
by the current file order under `claude-plugin/skills/`.

## Scope

In scope:

- Port shared skill behavior into `agents-plugin/`.
- Preserve `claude-plugin/` as the stable Claude package during the port.
- Define the runtime boundary before claiming operational parity.
- Keep helper, MCP, and delegation contracts explicit in child tickets.

Out of scope:

- In-place mutation of `claude-plugin/` for Codex experiments.
- Declaring Claude compatibility complete without a real Claude runtime check.
- Bulk-copying `bin/`, `infra/`, hooks, or all skills as a single migration step.
- Designing `/bootstrap` before smaller workflow and delegation patterns are proven.

## Porting Order

### 1. Base authoring and scaffold

Completed:

- `260502-feat-agents-plugin-codex-port-scaffold`
- `260502-feat-agents-plugin-workflow-skill-drafts` Phase 1

Purpose: establish the plugin candidate and the authoring standard used by later
ports.

### 2. Front-of-pipeline direct/advisory drafts

Completed:

- `260502-feat-agents-plugin-workflow-skill-drafts` Phase 2

Covered skills:

- `write-ticket`
- `discuss`

Purpose: port the workflow front door as host-neutral behavior documents while
deferring helper execution and MCP reconstruction.

### 3. Runtime boundary and MCP design

Active child ticket:

- `260503-feat-agents-plugin-runtime-boundary`

Purpose: decide what `agents-plugin` skills perform directly, what MCP
tools/resources/prompts must provide, and what remains as CLI fallback behavior.
The Go stdio baseline, v0.1 read surface, Codex plugin-managed launcher path,
release asset build, checksum verification, and production download branch are
implemented. Windows plugin-managed startup remains a deferred host-smoke item
with documented fallbacks; it should not block the next skill-porting slice.

### 4. Spec/doc direct track

Planned child ticket:

- `260503-feat-agents-plugin-spec-skill-drafts`

Covered skills:

- `write-spec`
- `update-spec`

Purpose: port lead-driven spec behavior after the read-oriented runtime boundary is
clear enough to replace or isolate `ws-spec-build-index`,
`ws-generate-spec-stem`, and spec lookup assumptions.

### 5. Sidecar productivity skills

Planned child ticket:

- `feat-agents-plugin-sidecar-skill-drafts`

Covered skills:

- `add-rule`
- `ship`
- `exit-session`
- `manual-think`
- `workflow`

Purpose: port useful skills outside the core implementation track before tackling
heavy delegation. These are mostly direct-execution or reference skills and should
not require the full orchestration runtime.

### 6. Delegation contract prototype

Planned child ticket:

- `feat-agents-plugin-write-skeleton-port`

Covered skill:

- `write-skeleton`

Purpose: define the first production delegation contract for Codex: what the lead
decides, what the delegate writes, how outputs are reviewed, and how commit
ownership works without assuming Claude's named-agent registry.

### 7. Core implementation orchestration

Planned child ticket group:

- `feat-agents-plugin-write-code-port`
- `feat-agents-plugin-implement-proceed-port`
- `feat-agents-plugin-sprint-port`

Covered skills:

- `write-code`
- `edit`
- `implement`
- `proceed`
- `sprint`

Purpose: port the main production track only after the runtime boundary and
delegation contract are proven. These skills depend on branch management,
implementation/reviewer relay, doc pipeline, approval gates, and task continuity.

### 8. Reconstruction and bootstrap

Planned child ticket group:

- `feat-agents-plugin-forge-skills-port`
- `feat-agents-plugin-bootstrap-design`

Covered skills:

- `forge-spec`
- `forge-mental-model`
- `bootstrap`

Purpose: port the broadest reconstruction and project-initialization workflows
last. These depend on stable document conventions, runtime surfaces, delegation
semantics, and host-specific context bootstrap behavior.

## Completion Criteria

- Every `claude-plugin/skills/` user-facing skill is either ported into
  `agents-plugin/`, explicitly deferred with rationale, or declared Claude-only.
- Each ported skill states whether it is draft-only, Codex-smoke-tested,
  Claude-manifest-compatible, or runtime-verified.
- MCP and CLI responsibilities are documented before helper-dependent skills claim
  operational parity.
- Bootstrap has a dedicated design ticket that separates AGENTS/CLAUDE context,
  plugin-local context, install/update behavior, and project document scaffolding.
