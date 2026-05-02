---
title: agents-plugin workflow skill drafts
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260502-feat-agents-plugin-codex-port-scaffold: completed scaffold prerequisite
---

# agents-plugin workflow skill drafts

## Background

The first `agents-plugin/` scaffold proved that Codex can load a minimal `ws`
candidate and that Claude can validate its manifest. The next slice should not
start with `/bootstrap`: bootstrap reaches project creation, root context
generation, install/update assumptions, helper commands, and Claude/Codex
workflow differences all at once.

Instead, establish a smaller porting pattern:

- Harden `skill-authoring`, because it is the pivot skill future ports will use.
- Draft `write-ticket` and `discuss` as host-neutral workflow skill documents.
- Exclude helper tooling and MCP reconstruction from these drafts.

## Decisions

- **Bootstrap deferred**: design bootstrap only after smaller workflow skills show
  how Codex and Claude differences should be expressed.
- **Tooling excluded**: do not port `ws-*`, named-agent orchestration, hooks, or
  MCP runtime behavior in this ticket.
- **Draft-first workflow ports**: `write-ticket` and `discuss` should preserve
  workflow intent and capture boundaries, but any helper-dependent step should be
  phrased as a future tool surface or manual/document inspection step.
- **Claude package unchanged**: `claude-plugin/` remains the reference package;
  copied behavior is normalized into `agents-plugin/`.

## Constraints

- Do not change `claude-plugin/`.
- Do not claim `write-ticket` or `discuss` are fully operational without MCP or
  helper-tool support.
- Keep Codex skill invocation names under the `ws` plugin namespace.
- Keep all AI-authored artifacts in English.

## Phases

### Phase 1: Harden `skill-authoring`

Revise `agents-plugin/skills/skill-authoring/SKILL.md` against
`ai-docs/ref/skill-authoring.md`. Address known audit concerns: doctrine resource
specificity, dense invariant wording, audit semantics, and host-neutral delegation
wording.

Success criteria:

- `skill-authoring` keeps the skill/agent layout guidance compact and executable.
- Doctrine names a concrete finite resource and includes the generator clause.
- Audit procedure distinguishes structure, invariants, doctrine, and closure gaps.
- `agents/openai.yaml` remains aligned with the skill.

### Phase 2: Draft `write-ticket` and `discuss`

Add initial `agents-plugin` versions of `write-ticket` and `discuss`. Use the
Claude skills as source material, but normalize host-specific instructions into
host-neutral behavior.

Success criteria:

- Both skills exist under `agents-plugin/skills/`.
- The drafts omit or isolate `ws-*`, named-agent, slash-command chaining, shell
  interpolation, and tool-name assumptions.
- The drafts state their tooling limits clearly enough that a future MCP ticket can
  supply the missing execution surfaces.
- `agents/openai.yaml` files exist for both skills.

### Phase 3: Documentation refresh

Update project memory and this ticket with the completed slice and the remaining
follow-up work.

Success criteria:

- `ai-docs/_index.md` reflects the new `agents-plugin` skill inventory.
- This ticket records validation and remaining limitations.
- Follow-up work is identified without pre-committing the bootstrap design.
