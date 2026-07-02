---
title: subagent-rules infra doc still recommends retired ws/subquery
completed: 2026-06-15
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

### Result (d9928c4f) - 2026-06-15

Replaced the retired `ws/subquery` invocation examples in
`agents-plugin/rsrc/subagent-rules.md` with caller-owned Explore playbook
guidance: render `explore`, pass the rendered brief path plus scoped question to
a host-native Explore-capable worker, and collect a concise evidence report.
Regenerated `agents-plugin/rsrc/manifest.json` and the byte-identical
`agents-plugin-wsflow/rsrc/` mirror.

Verification:

- `rg "ws/subquery|subquery\\(" agents-plugin/rsrc/subagent-rules.md
  agents-plugin-wsflow/rsrc/subagent-rules.md` found no matches.
- Source `ws-mcp serve --stdio` with `WS_RSRC_ROOT=agents-plugin/rsrc` returned
  updated `infra.read(name: "subagent-rules")` text containing
  `ws/playbook.render(name: "explore")` and no `ws/subquery` or `subquery(`.
- `go test -count=1 ./internal/wsrsrc ./internal/wsdoc ./internal/mcp` passed.
- `python3 -m unittest discover agents-plugin-wsflow/tests` still fails on the
  pre-existing `lead-workflow-manual/SKILL.md: full ws dotted namespace` issue,
  unrelated to this rsrc mirror change.
