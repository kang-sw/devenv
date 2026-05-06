# Brief: 260506-feat-bootstrap-workflow-guide

## Intent

Install a project-local workflow guide through downstream bootstrap so a
maintainer without the ws plugin can still preserve the project shape well
enough to continue work. Keep plugin/runtime semantics canonical; the guide is
a pinned explanation and manual fallback, not a local override mechanism.

## Approach

- Add a `WORKFLOW.md` template under the Codex bootstrap skill.
- Update `lead-bootstrap` so fresh bootstrap creates `ai-docs/workflow/` and
  installs the guide.
- Update `AGENTS.template.md` so generated projects point readers at
  `ai-docs/workflow/WORKFLOW.md`.
- Preserve Claude compatibility by updating the legacy Claude bootstrap
  template/skill with equivalent downstream output and wording where needed.
- Keep the guide under 200 lines and focused on plugin-less maintenance.

## Constraints

- Runtime and plugin semantics remain the machine contract.
- The guide must not imply that editing it changes MCP parser behavior.
- Keep `AGENTS.md` short and behavioral; avoid moving all convention detail into
  root context.
- Preserve bootstrap idempotency and existing Claude shim behavior.
- Do not touch unrelated untracked files such as `ai-docs/presentations/`.

## Out of scope

- Adding project-local convention override loading to MCP.
- Changing ticket/spec/mental-model parsers.
- Adding new CLI or MCP tools.
- Reworking the full bootstrap migration checklist.

## Details

The guide should explain:

- root authority files: `AGENTS.md`, `CLAUDE.md`, and the workflow guide;
- `ai-docs/` layout, `_index.md`, and `_index.local.md`;
- ticket status directories, stable stems, phases, result freezing, and the
  ready queue;
- spec stems, behavior-level specs, and planned-vs-implemented markers;
- mental-model inclusion rules, domain rules, and spec cross-references;
- commit traceability through `## AI Context`, `## Ticket Updates`, and
  `## Spec`;
- manual fallback behavior when ws skills or MCP tools are unavailable.

## References

- [Must] `ai-docs/spec/workflow-skills.md` - `260506-bootstrap-workflow-guide`
  and `lead-bootstrap` caller-visible behavior.
- [Must] `ai-docs/spec/documentation-system.md` - ticket, spec, mental-model,
  and convention document roles.
- [Must] `ai-docs/spec/claude-compatibility.md` - downstream `AGENTS.md` and
  `CLAUDE.md` shim boundary.
- [Must] `ai-docs/mental-model/workflow-skills.md` - Codex/Claude skill
  surfaces and compatibility update rule.
- [Must] `ai-docs/mental-model/documentation-system.md` - parser and convention
  source-of-truth hazards.
- [Must] `ai-docs/mental-model/claude-compatibility.md` - Claude bootstrap
  compatibility boundary.
- [Must] `agents-plugin/skills/lead-bootstrap/SKILL.md` - Codex bootstrap
  workflow.
- [Must] `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` - downstream
  canonical root context template.
- [Must] `claude-plugin/skills/bootstrap/SKILL.md` - legacy Claude bootstrap
  workflow.
- [Must] `claude-plugin/skills/bootstrap/CLAUDE.template.md` - legacy template
  compatibility reference.
