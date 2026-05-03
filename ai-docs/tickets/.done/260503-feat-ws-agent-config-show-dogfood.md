---
title: ws agent config.show regression dogfood
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: parent stabilization epic
completed: 2026-05-03
---

# ws agent config.show regression dogfood

## Background

After lifecycle and debug improvements landed, the workflow needed a small real
implementation target to exercise named-agent review recovery. The chosen target
was `config.show`, a read-only companion to `config.agents_tier` that reports
the current ws user-local configuration and the resolved config path.

This child ticket preserves the completed Phase 3 dogfood scope split out of
`260503-epic-ws-agent-workflow-stability`.

## Result (26e0174) - 2026-05-03

Implemented the small `config.show` regression target as a direct edit with a
named-agent correctness/fit reviewer. The new read-only surface returns the
resolved config path and current config through both MCP (`config.show`) and CLI
(`ws-mcp config show`), and runtime metadata records both surfaces so launcher
drift checks can repair stale binaries.

The same `wsconfig.Show` read path backs MCP and CLI behavior; it does not
create `config.json` in the no-file case.

This run did not exercise the full `write-code` skill because the caller
provided a direct implementation brief for the current branch, but it did
exercise the nonblocking named-agent reviewer flow. The reviewer ran to
completion through `agents.call` plus bounded `agents.wait`; no runtime-owned
process cleanup was needed, and state was recoverable through normal agent
wait/status surfaces. The reviewer caught a missing epic update, which was
relayed as a fix instead of leaving the dogfood result out of ticket history.

Verification covered `cd agents-plugin-tool && go test ./...`, runtime JSON
parsing, `claude plugin validate agents-plugin`, `git diff --check`, and a CLI
smoke with temporary `WS_CACHE_HOME` that configured `light` to
`gemini-3-1-pro` and confirmed `ws-mcp config show` returned the expected path
and mapping. No spec or mental-model updates were made on this branch.
