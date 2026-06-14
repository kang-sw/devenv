---
title: subagent-rules infra doc still recommends retired ws/subquery
related:
  260609-refactor-ws-skill-text-playbook-conversion: shifted subquery delegation to native Explore playbooks
  260605-epic-ws-playbook-factory-pivot: owns subquery retirement and native-subagent convergence
spec:
  - 260610-subquery-explore-delegation-shift
related-mental-model:
  - workflow-skills
  - prompt-bundle
  - mcp-runtime
---

# subagent-rules infra doc still recommends retired ws/subquery

## Background

Post-reinstall Codex MCP dogfooding found that `ws/infra.read(name:
"subagent-rules")` still serves guidance for the removed `ws/subquery` tool:

```text
ws/subquery(question: "<question>")
ws/subquery(deep_research: true, question: "<question>")
```

The same document says to use `ws/subquery` for broad cross-module tracing.
That conflicts with the current M2/M3 direction: exploration delegation moved to
the native Explore pattern through the `explore` playbook, and `ws/subquery` was
removed from the advertised runtime surface.

This is separate from the MCP schema/session-key bug: even after root-aware
tools become callable again, this infra document would still teach a caller to
invoke a retired tool.

## Phases

### Phase 1: Replace retired subquery guidance with current Explore delegation

Update `agents-plugin/rsrc/subagent-rules.md` so the exploration helper section
describes the current native Explore/playbook flow instead of `ws/subquery`.

The replacement should align with `lead-workflow-manual` and the current
`explore` playbook surface:

- direct file reads/search remain preferred when the target is known;
- broad exploration should use a rendered `explore` playbook and a
  host-native Explore-style subagent;
- any ws-managed mercenary mention should stay consistent with the current
  `ws.mercenary.*` surface and session-key model.

Verification boundary:

- `ws/infra.read(name: "subagent-rules")` no longer mentions `ws/subquery`.
- `rg "ws/subquery|subquery\\(" agents-plugin/rsrc/subagent-rules.md` finds no
  stale invocation guidance.
- Regenerate and verify the rsrc manifest, and update the wsflow mirror if this
  rsrc file is mirrored there.
- Run the relevant `agents-plugin-tool` Go tests for rsrc/infra loading.
