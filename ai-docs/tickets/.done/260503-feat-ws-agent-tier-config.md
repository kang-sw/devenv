---
title: ws agent tier model configuration
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: parent stabilization epic
completed: 2026-05-03
---

# ws agent tier model configuration

## Background

Workload tiers should stay provider-neutral in shared skill text, while each
user or organization can map `light`, `core`, and `deep` to concrete
backend/model choices. Prompt frontmatter may keep Claude-flavored
`model: haiku`, `model: sonnet`, and `model: opus` aliases, but runtime state
normalizes them to `light`, `core`, and `deep`.

This child ticket preserves the completed Phase 5 scope split out of
`260503-epic-ws-agent-workflow-stability`.

## Result - 2026-05-03

Implemented user-local workload tier configuration. The `wsconfig` package reads
and writes `config.json` under the ws cache root and exposes `SetAgentsTier`,
`ResolveAgent`, and model-name backend inference.

`agents.register` now resolves prompt frontmatter and caller tier first, then
applies tier config unless the caller supplied an explicit concrete model.
Missing backend defaults to an inferred backend when possible and then to
`codex`.

MCP exposes `config.agents_tier`; the CLI fallback is
`ws-mcp config agents-tier --tier <light|core|deep> [--backend <backend>]
[--model <model>]`. Runtime metadata advertises both surfaces. `delegate` and
`leaf` MCP profiles reject `config.*` tools so global configuration remains a
lead-level operation.

The implementation preserves Claude-flavored prompt shorthand:
`haiku`/`sonnet`/`opus` still normalize to `light`/`core`/`deep`, while explicit
model names containing `haiku`, `sonnet`, `opus`, or `claude` infer backend
`claude`. Names containing `gemini` infer `gemini`, and `gpt-*` or names
containing `codex` infer `codex`.

Verification covered `go test ./...` in `agents-plugin-tool`, focused package
tests for `cmd/ws-mcp`, `wsconfig`, `wsagent`, and `mcp`, runtime JSON parsing,
`claude plugin validate agents-plugin`, `git diff --check`, rebuilding the
Darwin ARM64 runtime binary, and a real CLI smoke that configured
`light -> gemini-3-1-pro` and confirmed a registered agent stored
`backend: gemini` and `model: gemini-3-1-pro`.
