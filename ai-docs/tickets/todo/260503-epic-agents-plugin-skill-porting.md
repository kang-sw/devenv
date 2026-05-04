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
- Preserve source skill wording and flow where possible; change only broken
  host-specific calls, shell syntax, slash-command syntax, and local paths.
- Assume ws MCP is available in shared `agents-plugin` skill text.

Out of scope:

- In-place mutation of `claude-plugin/` for Codex experiments.
- Declaring Claude compatibility complete without a real Claude runtime check.
- Bulk-copying `bin/`, `infra/`, hooks, or all skills as a single migration step.
- Designing `/bootstrap` before smaller workflow and delegation patterns are proven.
- Shared skill references to repo-local paths such as `claude-plugin/infra/*`;
  convention access must go through MCP so downstream projects do not need this
  repository's plugin source tree.

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

Completed:

- `260503-feat-agents-plugin-spec-skill-drafts`

Covered skills:

- `write-spec`
- `update-spec`

Purpose: port lead-driven spec behavior after the read-oriented runtime boundary is
clear enough to replace or isolate `ws-spec-build-index`,
`ws-generate-spec-stem`, and spec lookup assumptions.

### 5. Sidecar productivity skills

Completed:

- `260503-feat-agents-plugin-sidecar-skill-drafts`

Covered skills:

- `add-rule`
- `ship`
- `exit-session`

Excluded from this slice:

- `manual-think`

Purpose: port useful skills outside the core implementation track before tackling
heavy delegation. These are mostly direct-execution skills and should not require
the full orchestration runtime. `workflow` begins as a host-neutral
session-resident notation reference rather than a bulk port of Claude's
PATH primitive list; it can absorb shared orchestration primitive contracts as
the agent session runtime lands.

### 6. Agent session runtime and delegation contract

Completed child ticket:

- `260503-feat-agents-plugin-agent-session-runtime`

Covered skill/runtime:

- `write-skeleton`
- minimum `ws/agents.register`, `ws/agents.call`, `ws/agents.oneshot`,
  `ws/agents.print`, and `ws/agents.erase`

Purpose: define the host-neutral sustainable session runtime before porting the
first production delegation skill. The runtime should generalize Claude's
`ws-named-agent` prior art into project-state path management, named/oneshot
session registration, resume-backed calls, queued follow-ups, output lookup, and
workload-depth tiers. `write-skeleton` is the first consumer after that contract
exists.

Remaining gaps before the core implementation track: async calls, interrupts,
tail/list, review-path allocation, runtime locks, reviewer fanout, and shared
prompt-bundle resolution.

### 7. Core implementation orchestration

Planned child ticket group:

- `260503-feat-agents-plugin-async-agent-calls`
- `260503-feat-agents-plugin-edit-port`
- `260503-feat-agents-plugin-write-code-port`
- `feat-agents-plugin-implement-proceed-port`
- `feat-agents-plugin-sprint-port`

Covered skills:

- `write-code`
- `edit`
- `implement`
- `proceed`
- `sprint`

Purpose: port the main production track only after the runtime boundary and
delegation contract are proven. Start with `edit` because it keeps
implementation lead-owned while validating the reviewer relay and review-path
primitive. The later skills depend on branch management, implementation/reviewer
relay, doc pipeline, approval gates, and task continuity.

Status:

- `edit` and `write-code` are ported and installed.
- Runtime blockers found during `write-code` dogfood are resolved or deferred:
  async lifecycle hardening, debug diagnostics, nonblocking orchestration,
  worktree-local lead/delegate containment, current-call serialization, and
  hook-driven `agents.interrupt` are implemented. Stricter durable leaf-level
  role assignment is deferred to
  `260504-research-durable-leaf-role-assignment` because delegate-level
  containment blocks recursive named-agent orchestration and the remaining
  `subquery` recursion policy does not currently justify blocking the skill
  migration.
- Next child work should port the remaining orchestration harnesses:
  `implement`, `proceed`, and then `sprint`.

### 8. Reconstruction and bootstrap

Planned child ticket group:

- `feat-agents-plugin-bootstrap-design`

Covered skills:

- `forge-spec`
- `forge-mental-model`
- `bootstrap`

Purpose: port the broadest reconstruction and project-initialization workflows
last. These depend on stable document conventions, runtime surfaces, delegation
semantics, and host-specific context bootstrap behavior.

Status:

- `forge-spec` ported in `1674bba` using a persistent named implementer agent
  as a live MCP-backed delegation smoke. The port keeps archive confirmation,
  domain confirmation, user classification gates, spec stem generation, spec
  index verification, and clerk-style ticket association through
  `ws/agents.oneshot` with inline instructions.
- `forge-mental-model` ported in `6477224` by reusing the same named implementer
  agent session. The port keeps spec availability warning, domain confirmation,
  survey-before-write, verifier-before-write, and the
  `(mental-model-updated)` commit marker.
- `manual-think` is intentionally Claude-only while `claude-plugin/` remains
  available as an out-of-support compatibility tree.
- `bootstrap` has an initial Agents plugin draft centered on bundled
  `AGENTS.template.md`, downstream `AGENTS.md` as the canonical managed root
  context, and `CLAUDE.md` as an `@AGENTS.md` compatibility shim. It still needs
  a dedicated design/implementation ticket before claiming operational parity.

## Completion Criteria

- Every `claude-plugin/skills/` user-facing skill is either ported into
  `agents-plugin/`, explicitly deferred with rationale, or declared Claude-only.
- Each ported skill states whether it is draft-only, Codex-smoke-tested,
  Claude-manifest-compatible, or runtime-verified.
- MCP and CLI responsibilities are documented before helper-dependent skills claim
  operational parity.
- Bootstrap has a dedicated design ticket that separates AGENTS/CLAUDE context,
  plugin-local context, install/update behavior, and project document scaffolding.
