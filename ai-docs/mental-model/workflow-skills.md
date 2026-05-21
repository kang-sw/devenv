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
- `agents-plugin-wsflow/skills/lead-*` is the curated derivative surface and uses `wsflow:` skill names plus `wsflow/<tool>` MCP notation. {#260513-wsflow-agentless-skill-surface}
- `lead-workflow-manual` is the notation and primitive boundary reference for shared skill text. {#260505-workflow-primitive-reference}

## Module Contracts

- Codex skill directory names and `name:` frontmatter are externally visible invocation strings; renames break user workflows.
- Codex-facing workflow guidance teaches MCP primitives first; CLI adapter syntax belongs only in compatibility or testing references. {#260507-mcp-centric-workflow-language}
- Shared workflow examples use `model: light|core|deep` as portable delegate aliases; `tier` is legacy compatibility language and concrete provider model names are reserved for intentional overrides. {#260508-workflow-model-alias-guidance}
- Skill descriptions are the runtime trigger surface: keep top-level entries strong, derived primitives lighter, and conditional utilities explicit. {#260508-skill-description-attention-policy}
- `lead-add-rule` requires explicit persistence intent such as save, remember, persist, or add a durable rule; prescriptive task wording alone must not trigger it. {#260508-add-rule-explicit-persistence-trigger}
- Skill-to-skill handoffs share the active conversation; do not declare carry blocks, and reserve argument language for MCP tools, CLIs, and templates. {#260514-skill-authoring-carried-context}
- User-approval gates in skills fire only on direct user invocation; chained invocations pass through. {#260514-skill-authoring-carried-context}
- Dense skill routing should use Markdown hierarchy, grouped invariants, fixed lookup tables, and command-shaped lists before pseudo-code-like notation; pseudo-code obscures handoffs under attention pressure. {#260514-skill-authoring-carried-context}
- `lead-proceed` routes through handoff stages and captures the `Ticket:` line from `lead-write-ticket`; changing that artifact breaks chaining. {#260505-proceed-routing-pipeline}
- `lead-proceed` must stop on epic ticket paths because epics are board artifacts; implementation routes through child tickets. {#260505-proceed-routing-pipeline}
- `lead-proceed` routes implementation-ready work to `lead-implement`; before handoff it reads the implement skill text and applies that route contract source-free, but does not decide contract-brief depth, inspect source, or invoke implementation primitives directly. {#260505-proceed-routing-pipeline} {#260519-proceed-implementation-dispatch-precheck}
- `lead-proceed` treats `todo/` ticket paths as implementation intent, promotes through `lead-write-ticket`, and escalates only for unresolved design or ready-gate blockers. {#260505-proceed-routing-pipeline}
- `lead-proceed` can route narrow, routine, fully scoped inline implementation directly to `lead-implement` when no durable ticket is needed and commit `AI Context` is enough traceability. {#260505-proceed-routing-pipeline}
- `lead-proceed` does not rejudge ticket decomposition; absent an explicit phase name, it selects the first unfinished phase and treats that phase as hard downstream scope. {#260505-proceed-routing-pipeline}
- Warm `lead-proceed` runs a ticket freshness gate for an existing related ticket: compare active conversation against the ticket only, refresh through `lead-write-ticket` when settled decisions are missing, then re-read before scope resolution. {#260513-proceed-ticket-freshness-gate}
- `lead-implement` routes public or cross-module contract work to `lead-write-code`; contract checkpoints live in the brief, not in generated skeleton artifacts. {#260512-skeleton-inside-implement-branch}
- `lead-implement` continues on existing `implement/*` branches, may safely rename them before execution, closes docs before the final gate, and treats merge as one user-approved final action. {#260505-implementation-workflow-skills}
- `lead-write-ticket` runs the spec gate only when non-`epic`, non-`research` work enters `ready/`; `todo/` is accepted backlog with optional spec recovery hints. {#260505-planning-workflow-skills}
- `lead-write-ticket` invokes `lead-write-spec` autonomously when ready-ticket coverage is missing, then stops only if coverage still cannot be established. {#260505-planning-workflow-skills}
- `lead-write-ticket` keeps epics at milestone-board scope and routes detailed discussion, implementation phases, and phase-specific decisions into child tickets; child-ticket phases are complete fresh-session implementation units, not internal task lists. {#260508-write-ticket-epic-child-boundary}
- `lead-write-ticket` preserves settled decisions before brevity: actionable child tickets record contracts, strategy decisions, rejected alternatives, verification expectations, and binding cross-ticket forward-compatibility constraints. {#260516-write-ticket-related-ticket-propagation}
- `lead-write-ticket` reviews related-ticket decisions by default; explicit cascade wording broadens the pass to board and multi-ticket edits, with epics board-level and no implicit ready promotion. {#260516-write-ticket-related-ticket-propagation}
- `lead-discuss` uses the user's active conversation language for discussion responses. {#260505-planning-workflow-skills}
- `lead-discuss` routes explicit implementation intent through `lead-proceed`; persistence-only discussion still feeds `lead-write-spec` and `lead-write-ticket`. {#260505-planning-workflow-skills}
- `lead-discuss` uses symbolic-label Intent Frames for proposal, evaluation, design-direction, causal-claim, scope-assumption, and trade-off turns; Interview Workflow starts only when unresolved branches need user priority or scope input. {#260510-discuss-intent-frame-interview}
- `lead-verify-discussion` is a small explicit checkpoint for refreshing discussion premises, finding reusable existing implementation, validating direction, naming over-alignment risks, testing the best countercase, and steering from corrected assumptions through `ws/subquery`; keep it callable and lightweight. {#260512-discussion-verification-skill}
- `lead-check-blockers` is a small explicit checkpoint for separating user-blocking design questions from autonomous hygiene, implementation detail, ticket/spec capture gaps, and proceed readiness. {#260513-check-blockers-skill}
- `lead-salvage` is the reverse workflow for premise-collapse recovery: freeze evidence, survey blast radius through agents, confirm invalidated premises with the user, then capture a research report plus recovery epic or child tickets. {#260510-salvage-recovery-workflow-skill}
- `lead-write-skeleton` is deprecated from normal implementation routing; keep the skill file and old prompt bundle entries for compatibility, but do not route new work through generated skeleton artifacts. {#260510-skeleton-contract-populator-flow}
- `lead-bootstrap` has two template contracts: root context and `WORKFLOW.md`. Fresh and upgrade paths must install or preserve `ai-docs/WORKFLOW.md` as a plugin-less maintenance guide, but the guide cannot redefine ws runtime, MCP parser, or bundled convention semantics. {#260506-bootstrap-workflow-guide}
- `lead-bootstrap` index health is advisory but explicit: the first pass reads only `_index.md`; when candidates exist, it reports likely scope drift, asks whether to clean up now, defer, or route semantic follow-up, and keeps approved cleanup limited to `_index.md`; it must not author or update specs, mental models, tickets, or references. {#260506-bootstrap-workflow-guide}
- `lead-bootstrap` is mirrored between ws and wsflow, but downstream template version histories are package-local; wsflow starts its bootstrap baseline at `v0001` and does not replay the full ws migration backlog. {#260513-wsflow-agentless-skill-surface}
- Orchestration-heavy skills load `lead-workflow-manual` when primitive context is not already active; skipping it causes notation drift and wrong agent-call forms. {#260505-workflow-primitive-reference}
- Implementation skills do not create missing skeleton artifacts; `lead-write-code` carries concrete contract and integration-test instructions in the implementation brief. {#260505-implementation-workflow-skills}
- `lead-edit` and `lead-write-code` are code-and-review primitives; `lead-implement` and `lead-sprint` own documentation pipeline timing. {#260505-implementation-workflow-skills}
- `lead-write-code` uses brief-bounded implementation and file-based reviewer output; the brief must preserve selected-scope binding decisions, and ticket-driven fit review checks ticket-to-brief decision preservation. {#260505-implementation-workflow-skills}
- `plan-populator-survey` collects file-backed reuse, contract, and shortcut-risk signals; when safe execution needs planner judgment, it exits with `[escalate-to-research]` instead of forcing a survey plan. {#260505-implementation-workflow-skills}
- `plan-populator-research` turns codebase evidence into planner judgment: choose clean existing mechanisms, reject fallback/mock/temporary or duplicated-glue paths, and escalate when no clean plan satisfies the brief. {#260505-implementation-workflow-skills}
- `lead-write-code` treats survey and research as either/or plan depths; survey escalation routes to research before implementation, replacing the same plan path with the research plan. {#260505-implementation-workflow-skills}
- `lead-write-code` evaluates plan-populator exit signals before spawning the implementer; stopping early is preferred to running a known-bad implementation path through review. {#260505-implementation-workflow-skills}
- Sprint defers doc pipeline until wrap-up; per-task doc updates inside sprint create partial checkpoints that confuse wrap-up. {#260505-sprint-session-container}
- wsflow includes sprint as a branch container and routes source changes through `lead-edit`; scoped subagents may help exploration, implementation, verification, audit, or review while the lead keeps integration, final judgment, and commits explicit. {#260513-wsflow-agentless-skill-surface} {#260513-wsflow-sprint-skill}
- wsflow mirrors proceed route clarity by reading `wsflow:lead-implement` and announcing a source-free implementation verdict; it does not mirror full ws implementation relay names for excluded skills. {#260519-proceed-implementation-dispatch-precheck}
- `lead-review` loads `ai-docs/_review.local.md` for all environment-specific configuration (remote, phases, comment/merge/notification methods); when absent, it interviews the user and writes the config before the first review runs. {#260513-review-workflow-skill}
- `lead-review` routes NEEDS FIX local-fix through `lead-discuss` with findings as context; re-review after fix is user-discretion, not automatic re-entry. {#260513-review-workflow-skill}
- `lead-review` judge `follows-ws-workflow` auto-detects conventional commits and `## AI Context`; PARTIAL (some commits qualify) counts as NO (conservative); the `## Contributor Workflow` config setting can force YES or NO. {#260513-review-workflow-skill}
- `lead-review` judge `is-large-diff` uses subagents for parallel alignment and risk phases when diff exceeds the configured threshold; both depth judges are independent and combinable. {#260513-review-workflow-skill}

## Coupling

- Skill text that names prompt stems must match embedded prompt filenames and runtime bundle metadata.
- Legacy skeleton prompts may remain bundled for compatibility; normal implementation routing does not register skeleton-populator or skeleton-reviewer.
- Discuss ready-promotion logic routes through `lead-write-ticket`; direct moves bypass the ready spec gate and queue checks.
- Moving updater dispatch into `lead-edit` or `lead-write-code` can double-run `lead-implement` documentation updates or break sprint batching.
- `lead-salvage` routes ticket writes through `lead-write-ticket`; direct ticket graph mutation inside salvage would bypass ticket conventions and commit handling.
- Bootstrap guide semantics stay host-neutral; root `CLAUDE.md` only delegates to `AGENTS.md`.
- Bootstrap template changes must check both ws and wsflow packages; matching behavior may use different template version numbers because each package owns its own downstream lineage.

## Extension Points & Change Recipes

- **Add a Codex workflow skill**: create `agents-plugin/skills/lead-<name>/SKILL.md`, follow skill-authoring invariants, add OpenAI UI metadata only if needed, and update workflow specs and mental models.
- **Add a config-first review skill variant**: follow `lead-review` pattern — machine-local `ai-docs/_review.local.md` captures environment judgment, setup interview fires only when config is absent, judges gate subagent depth rather than hard-coding it.
- **Change a full workflow skill included in wsflow**: update the corresponding `agents-plugin-wsflow/skills/lead-<name>/` surface in the same logical change or record a follow-up ticket; wsflow is curated, not text-identical.
- **Change a full workflow skill excluded from wsflow**: check `ai-docs/ref/wsflow-mirroring.md` and update wsflow docs, workflow manual text, or exclusion rationale if the excluded skill's meaning changed.
- **Change bootstrap baseline behavior**: update both `lead-bootstrap` packages when applicable, but bump each package's `AGENTS.template.md` version only inside that package's own lineage.
- **Add a delegate prompt to a workflow**: register by embedded stem through `ws/agents.register`, use portable `model` aliases for default selection, and omit `prompts` only for general-purpose delegates. Update runtime prompt bundle metadata and keep reviewer/implementer context boundaries explicit. {#260505-workflow-delegate-prompt-boundaries}

## Common Mistakes

- Treating `lead-proceed` as an implementation skill; it routes, sends `todo/` tickets through ready promotion, selects scope slices, and does not read source.
- Skipping `lead-workflow-manual` before executing or editing orchestration-heavy skills, which causes notation drift back to Claude shell helpers.
- Editing downstream `ai-docs/WORKFLOW.md` as if it overrides installed ws tooling; upstream plugin/runtime semantics and bundled conventions remain canonical.
- Removing the final `Ticket:` artifact from write-ticket output.
- Rewriting wsflow skills mechanically from full ws skills; wsflow must preserve workflow intent while using wsflow notation, scoped subagent guidance, and the curated skill inventory.
- Relaying reviewer file contents instead of file paths, which breaks the write-code review protocol and inflates lead context.
- Treating brief compression as permission to drop settled caller-visible contracts, implementation strategy decisions, rejected alternatives, or verification expectations.
