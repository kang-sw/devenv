---
domain: workflow-skills
description: "Codex lead skills, Claude compatibility skills, and workflow prompt orchestration."
sources:
  - agents-plugin/skills/
  - claude-plugin/skills/
  - agents-plugin-tool/internal/wsprompt/
related:
  documentation-system: "write-spec and write-ticket enforce documentation traceability before implementation."
  prompt-bundle: "delegated workflow skills register embedded prompt chains by stem."
---

# Workflow Skills

## Entry Points

- `agents-plugin/skills/lead-*` is the Codex-facing workflow surface and uses `ws:` skill names plus `ws/<tool>` MCP notation. {#260505-lead-skill-namespace-surface}
- `claude-plugin/skills/*` is the compatibility tree and may use slash commands and shell fallbacks.
- `lead-workflow` is the notation and primitive boundary reference for shared skill text. {#260505-workflow-primitive-reference}

## Module Contracts

- Codex skill directory names and `name:` frontmatter are externally visible invocation strings; renames break user workflows.
- Claude and Codex skills are parallel, not identical. Porting must preserve host-specific notation while keeping workflow semantics aligned.
- `lead-proceed` routes through prefix stages and captures the `Ticket:` line from `lead-write-ticket`; changing that artifact breaks chaining. {#260505-proceed-routing-pipeline}
- `lead-write-ticket` skips spec-gate only for `epic` and `research`; other todo work needs spec linkage. {#260505-planning-workflow-skills}
- `lead-write-code` uses brief-bounded implementation and file-based reviewer output; reviewers return summaries while implementers read finding files directly. {#260505-implementation-workflow-skills}
- Sprint defers doc pipeline until wrap-up; per-task doc updates inside sprint create partial checkpoints that confuse wrap-up. {#260505-sprint-session-container}

## Coupling

- Skill text that names prompt stems must match embedded prompt filenames and runtime bundle metadata.
- Discuss promotion logic and write-ticket spec-gate must agree on which ticket categories require spec creation.
- `lead-edit`, `lead-write-code`, and `lead-implement` each own a different review/doc-pipeline boundary; moving updater dispatch between them can double-run or skip documentation updates.
- Claude compatibility skills should be updated when Codex skill semantics change, but not blindly rewritten to MCP notation.

## Extension Points & Change Recipes

- **Add a Codex workflow skill**: create `agents-plugin/skills/lead-<name>/SKILL.md`, follow skill-authoring invariants, add OpenAI UI metadata only if needed, and update workflow specs.
- **Port a Claude skill**: translate notation to `ws:`/`ws/<tool>`, keep host-neutral behavior, then verify Claude compatibility text still describes the fallback tree.
- **Add a delegate prompt to a workflow**: register by embedded stem through `ws/agents.register`, update runtime prompt bundle metadata, and keep reviewer/implementer context boundaries explicit. {#260505-workflow-delegate-prompt-boundaries}

## Common Mistakes

- Treating `lead-proceed` as an implementation skill; it routes, it does not read source.
- Skipping `lead-workflow` before editing workflow skills, which causes notation drift back to Claude shell helpers.
- Removing the final `Ticket:` artifact from write-ticket output.
- Relaying reviewer file contents instead of file paths, which breaks the write-code review protocol and inflates lead context.

## Technical Debt

- Some Claude compatibility skills still contain richer legacy details than the Codex-first surface. Preserve compatibility, but treat Codex `lead-*` skills as the active shared workflow.
