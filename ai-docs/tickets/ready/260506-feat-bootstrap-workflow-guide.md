---
title: Bootstrap project-local workflow guide
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
  260501-research-agents-bootstrap-root-context: root workflow context research
spec:
  - 260506-bootstrap-workflow-guide
related-mental-model:
  - workflow-skills
  - documentation-system
  - claude-compatibility
---

# Bootstrap project-local workflow guide

## Background

Downstream projects currently receive enough `AGENTS.md` context for a
plugin-equipped agent to orient and run ws skills, but plugin-less maintainers
only get thin pointers to `ai-docs/`, ticket stems, and the ready queue. If the
ws plugin is unavailable, a maintainer can still preserve the workflow shape by
following the core document layers manually, but the bootstrap output does not
make that fallback clear.

The agreed direction is hybrid: runtime and plugin semantics remain the machine
contract, while downstream projects carry a pinned, readable guide for
plugin-less maintenance. The guide must not become a project-local override for
MCP/runtime behavior.

## Phases

### Phase 1: Install workflow guide through bootstrap

Add a concise `WORKFLOW.md` template for downstream projects and update
bootstrap-managed context so fresh and upgraded projects install it under
`ai-docs/workflow/`.

The guide should stay under 200 lines and explain enough for a maintainer to
keep the project usable when the ws plugin is absent:

- root authority files and the role of `AGENTS.md`;
- `ai-docs/` layout and `_index.md` pruning expectations;
- ticket statuses, stable stems, phases, result freezing, and ready queue use;
- spec stems, implemented/planned distinction, and behavior-level writing;
- mental-model inclusion rules and domain rules;
- commit traceability through `## AI Context`, `## Ticket Updates`, and
  `## Spec`;
- manual fallback expectations when plugin skills or MCP tools are unavailable.

Update `AGENTS.template.md` to point at the guide without moving machine
semantics out of the plugin/runtime contract. Keep Claude compatibility as the
`CLAUDE.md` shim over `AGENTS.md`.

Success criteria:

- Fresh bootstrap output includes `ai-docs/workflow/WORKFLOW.md`.
- `AGENTS.md` tells readers where the project-local workflow guide lives.
- The guide states that runtime/plugin semantics remain canonical and local
  deviations need explicit supported extension points.
- Existing bootstrap idempotency and migration behavior remain intact.
