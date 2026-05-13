---
domain: workflow-skills
description: "Codex lead skills and workflow prompt orchestration."
sources:
  - agents-plugin/skills/
  - agents-plugin-wsflow/skills/
  - agents-plugin-tool/internal/wsprompt/
related:
  documentation-system: "write-spec and write-ticket enforce documentation traceability before implementation."
  prompt-bundle: "delegated workflow skills register embedded prompt chains by stem."
---

# Workflow Skills

## Entry Points

- `agents-plugin/skills/lead-*` is the Codex-facing workflow surface and uses `ws:` skill names plus `ws/<tool>` MCP notation. {#260505-lead-skill-namespace-surface}
- `agents-plugin-wsflow/skills/lead-*` is the curated agentless derivative surface and uses `wsflow:` skill names plus `wsflow/<tool>` MCP notation. {#260513-wsflow-agentless-skill-surface}
- `lead-workflow-manual` is the notation and primitive boundary reference for shared skill text. {#260505-workflow-primitive-reference}

## Module Contracts

- Codex skill directory names and `name:` frontmatter are externally visible invocation strings; renames break user workflows.
- Codex-facing workflow guidance teaches MCP primitives first; CLI adapter syntax belongs only in compatibility or testing references. {#260507-mcp-centric-workflow-language}
- Shared workflow examples use `model: light|core|deep` as portable delegate aliases; `tier` is legacy compatibility language and concrete provider model names are reserved for intentional overrides. {#260508-workflow-model-alias-guidance}
- Skill descriptions are the runtime trigger surface: keep top-level entries strong, derived primitives lighter, and conditional utilities explicit. {#260508-skill-description-attention-policy}
- `lead-add-rule` requires explicit persistence intent such as save, remember, persist, or add a durable rule; prescriptive task wording alone must not trigger it. {#260508-add-rule-explicit-persistence-trigger}
- `lead-proceed` routes through prefix stages and captures the `Ticket:` line from `lead-write-ticket`; changing that artifact breaks chaining. {#260505-proceed-routing-pipeline}
- `lead-proceed` must stop on epic ticket paths because epics are board artifacts; implementation routes through child tickets. {#260505-proceed-routing-pipeline}
- `lead-proceed` routes implementation-ready work to `lead-implement`; it selects an implementation slice but does not decide skeleton need or invoke code-editing skeleton work directly. {#260505-proceed-routing-pipeline} {#260512-skeleton-inside-implement-branch}
- `lead-proceed` treats `todo/` ticket paths as implementation intent, promotes through `lead-write-ticket`, and escalates only for unresolved design or ready-gate blockers. {#260505-proceed-routing-pipeline}
- `lead-proceed` does not rejudge ticket decomposition; default execution is the first unfinished phase, and selected slices are hard downstream scope. {#260505-proceed-routing-pipeline}
- `lead-implement` owns skeleton decisions and execution inside the implementation branch lifecycle before edit/write-code runs. {#260512-skeleton-inside-implement-branch}
- `lead-implement` continues on existing `implement/*` branches, may safely rename them before execution, closes docs before the final gate, and treats merge as one user-approved final action. {#260505-implementation-workflow-skills}
- `lead-write-ticket` runs the spec gate only when non-`epic`, non-`research` work enters `ready/`; `todo/` is accepted backlog with optional spec recovery hints. {#260505-planning-workflow-skills}
- `lead-write-ticket` invokes `lead-write-spec` autonomously when ready-ticket coverage is missing, then stops only if coverage still cannot be established. {#260505-planning-workflow-skills}
- `lead-write-ticket` keeps epics at milestone-board scope and routes detailed discussion, implementation phases, and slice-specific decisions into child tickets. {#260508-write-ticket-epic-child-boundary}
- `lead-discuss` uses the user's active conversation language for discussion responses. {#260505-planning-workflow-skills}
- `lead-discuss` routes explicit implementation intent through `lead-proceed`; persistence-only discussion still feeds `lead-write-spec` and `lead-write-ticket`. {#260505-planning-workflow-skills}
- `lead-discuss` uses symbolic-label Intent Frames for proposal, evaluation, design-direction, causal-claim, scope-assumption, and trade-off turns; Interview Workflow starts only when unresolved branches need user priority or scope input. {#260510-discuss-intent-frame-interview}
- `lead-verify-discussion` is a small explicit checkpoint for refreshing discussion premises, finding reusable existing implementation, validating direction, and steering from corrected assumptions through `ws/subquery`; keep it callable and lightweight. {#260512-discussion-verification-skill}
- `lead-salvage` is the reverse workflow for premise-collapse recovery: freeze evidence, survey blast radius through agents, confirm invalidated premises with the user, then capture a research report plus recovery epic or child tickets. {#260510-salvage-recovery-workflow-skill}
- `lead-write-skeleton` makes the lead write low-resolution source drafts with language-neutral `CONTRACT:`, `HINT:`, and `HOLE:` comment markers; `skeleton-populator` turns those drafts into compile-clean stubs, and `skeleton-reviewer` checks them through a one-reviewer, one-amendment lightweight loop. The flow leaves a draft checkpoint commit plus a final populated skeleton commit on the current branch. {#260510-skeleton-contract-populator-flow}
- `lead-bootstrap` has two template contracts: root context and `WORKFLOW.md`. Fresh and upgrade paths must install or preserve `ai-docs/WORKFLOW.md` as a plugin-less maintenance guide, but the guide cannot redefine ws runtime, MCP parser, or bundled convention semantics. {#260506-bootstrap-workflow-guide}
- Orchestration-heavy skills load `lead-workflow-manual` when primitive context is not already active; skipping it causes notation drift and wrong agent-call forms. {#260505-workflow-primitive-reference}
- Implementation skills honor existing skeleton artifacts but do not require missing skeletons; `lead-implement` owns optional skeleton execution before implementation edits. {#260505-implementation-workflow-skills}
- `lead-edit` and `lead-write-code` are code-and-review primitives; `lead-implement` and `lead-sprint` own documentation pipeline timing. {#260505-implementation-workflow-skills}
- `lead-write-code` uses brief-bounded implementation and file-based reviewer output; reviewer allocation is risk-scoped, reviewers return summaries, and implementers read finding files directly. {#260505-implementation-workflow-skills}
- Sprint defers doc pipeline until wrap-up; per-task doc updates inside sprint create partial checkpoints that confuse wrap-up. {#260505-sprint-session-container}
- wsflow excludes managed orchestration skills and keeps implementation lead-owned through `lead-implement` -> `lead-edit`; review and forge surveys use host-native one-shot read-only subagents only when available. {#260513-wsflow-agentless-skill-surface}

## Coupling

- Skill text that names prompt stems must match embedded prompt filenames and runtime bundle metadata.
- Skeleton flow registers `skeleton-populator` and `skeleton-reviewer`; no compatibility writer prompt remains active.
- Discuss ready-promotion logic routes through `lead-write-ticket`; direct moves bypass the ready spec gate and queue checks.
- Moving updater dispatch into `lead-edit` or `lead-write-code` can double-run `lead-implement` documentation updates or break sprint batching.
- `lead-salvage` routes ticket writes through `lead-write-ticket`; direct ticket graph mutation inside salvage would bypass ticket conventions and commit handling.
- Bootstrap guide semantics stay host-neutral; root `CLAUDE.md` only delegates to `AGENTS.md`.

## Extension Points & Change Recipes

- **Add a Codex workflow skill**: create `agents-plugin/skills/lead-<name>/SKILL.md`, follow skill-authoring invariants, add OpenAI UI metadata only if needed, and update workflow specs.
- **Change a full workflow skill included in wsflow**: update the corresponding `agents-plugin-wsflow/skills/lead-<name>/` surface in the same logical change or record a follow-up ticket; wsflow is curated, not text-identical.
- **Add a delegate prompt to a workflow**: register by embedded stem through `ws/agents.register`, use portable `model` aliases for default selection, and omit `prompts` only for general-purpose delegates. Update runtime prompt bundle metadata and keep reviewer/implementer context boundaries explicit. {#260505-workflow-delegate-prompt-boundaries}

## Common Mistakes

- Treating `lead-proceed` as an implementation skill; it routes, sends `todo/` tickets through ready promotion, selects scope slices, and does not read source.
- Skipping `lead-workflow-manual` before executing or editing orchestration-heavy skills, which causes notation drift back to Claude shell helpers.
- Editing downstream `ai-docs/WORKFLOW.md` as if it overrides installed ws tooling; upstream plugin/runtime semantics and bundled conventions remain canonical.
- Removing the final `Ticket:` artifact from write-ticket output.
- Relaying reviewer file contents instead of file paths, which breaks the write-code review protocol and inflates lead context.
