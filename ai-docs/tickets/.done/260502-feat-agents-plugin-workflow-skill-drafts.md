---
title: agents-plugin workflow skill drafts
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260502-feat-agents-plugin-codex-port-scaffold: completed scaffold prerequisite
completed: 2026-05-02
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

### Result (da6023f) - 2026-05-02

Hardened `agents-plugin/skills/skill-authoring/SKILL.md` without changing the
skill name or UI metadata. The dense invariant checklist is now a named judgment
instead of a single overloaded invariant, the audit procedure distinguishes
structure, invariant quality, doctrine quality, and closure gaps, and independent
validation is conditional on the host and user authorizing it. Doctrine now names
the model's limited attention budget under context pressure as the finite resource
and includes the generator clause.

Validation:

- Manual structure check for frontmatter and Doctrine sections passed.
- `git diff --check` passed.
- `quick_validate.py` could not run because the local Python environment lacks
  `PyYAML` (`ModuleNotFoundError: No module named 'yaml'`).

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

### Result (da6023f) - 2026-05-02

Added draft `agents-plugin` skills for `write-ticket` and `discuss`, each with
`SKILL.md` and `agents/openai.yaml`. The drafts preserve the source Claude skills'
workflow intent while removing operational dependencies on `$ARGUMENTS`,
slash-command chaining, shell interpolation, `ws-*` helper execution, native tool
names, and named-agent orchestration.

Porting boundary:

- `write-ticket` reads ticket conventions directly and performs ordinary file
  edits/status moves; helper-backed stem/spec lookup is deferred.
- `discuss` reads project memory and named artifacts directly; survey agents and
  helper-generated project maps are deferred.
- Both drafts explicitly mark missing `ws-*`, named-agent, hook, MCP, and
  host-specific plugin behavior as future tooling work.

Validation:

- Manual structure check for frontmatter and Doctrine sections passed.
- `rg` check found no `$ARGUMENTS`, shell interpolation, named-agent calls,
  Claude native tool names, or slash-chain references in the three
  `agents-plugin` skills.
- `claude plugin validate agents-plugin` passed.
- `git diff --check` passed.
- `quick_validate.py` could not run for these skills because the local Python
  environment lacks `PyYAML`.

### Phase 3: Documentation refresh

Update project memory and this ticket with the completed slice and the remaining
follow-up work.

Success criteria:

- `ai-docs/_index.md` reflects the new `agents-plugin` skill inventory.
- This ticket records validation and remaining limitations.
- Follow-up work is identified without pre-committing the bootstrap design.

### Result (pending commit) - 2026-05-02

Updated project memory to list the current `agents-plugin` skill inventory:
`skill-authoring`, `write-ticket`, and `discuss`. Removed this ticket from the
active queue and marked it complete.

Remaining follow-up work:

- Manual Codex smoke test for `$ws:write-ticket` and `$ws:discuss` in a fresh
  session after plugin reload.
- Manual Claude closeout for `/ws:skill-authoring`, `/ws:write-ticket`, and
  `/ws:discuss` if the user wants to verify the candidate in Claude.
- MCP/runtime design for ticket/spec lookup, project survey, helper execution, and
  agent orchestration before claiming operational parity with `claude-plugin/`.
- Bootstrap design after the smaller workflow-port pattern has been reviewed.
