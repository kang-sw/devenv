---
domain: named-agent-runtime
description: "File-backed named agents, async calls, locks, subqueries, and Codex backend handling."
sources:
  - agents-plugin-tool/internal/wsagent/
  - agents-plugin-tool/internal/wsstate/
related:
  mcp-runtime: "agents.* MCP and CLI handlers are thin wrappers around wsagent.Manager."
  prompt-bundle: "registration resolves embedded prompts into each agent system prompt."
---

# Named Agent Runtime

## Entry Points

- `wsagent.Manager` owns registration, async calls, wait/result/status/tail/cancel, inbox delivery, and erasure. {#260505-named-agent-registry-state-layout}
- `wsstate.Manager.Ensure` derives cache, project, worktree, agent, review, lock, and temp paths.
- `CodexRunner` invokes `codex exec --json`, captures thread ids, and extracts final agent messages. {#260505-codex-agent-session-jsonl-handling}

## Module Contracts

- All agent paths must come from `wsstate.Ensure(root)` plus `AgentKey(name)`; bypassing this splits or merges worktree state incorrectly.
- Async call setup order matters: acquire setup lock, create current call state, write prompt snapshot, mark running, append queued event, then start `agents run-current`. {#260505-agent-async-single-call-lifecycle}
- Only `queued` and `running` are active states. Any new status must update busy checks, wait readiness, result handling, register reset safety, cancel, and follow-up text.
- `Result` requires terminal completed state and `output.md`; output must be written before current call completion is recorded. {#260505-agent-readiness-result-split}
- Interrupts are inbox files delivered at hook/check-inbox boundaries, not OS signals. {#260505-agent-inbox-interrupt-delivery}
- Successful `Result` erases ephemeral agents; `Print` is legacy and does not consume them. {#260505-async-subquery-ephemeral-agent}
- Backend invocation failures are formatted at the call site with raw error text, bounded PATH-detected backend hints, and reconfiguration guidance; do not run separate model/login probes during registration or config inspection. {#260505-agent-backend-failure-diagnostics}

## Coupling

- MCP and CLI wrappers mirror `Register`, `Call`, `Wait`, `Result`, `Status`, `Interrupt`, `Tail`, debug streams, `Cancel`, `Print`, and `Erase`; behavior changes require both surfaces.
- `ToolProfile` flows into Codex subprocess env as `WS_MCP_TOOL_PROFILE`; MCP role gating interprets the same strings.
- Worktree scoping is shared by agents, generated review paths, and orchestrator locks; changing cache layout affects all three.
- Prompt registration is static: `system.md` is written at registration time and existing agents do not automatically pick up edited embedded prompts. {#260505-agent-prompt-registration-tier-resolution}

## Extension Points & Change Recipes

- **Add a backend**: implement `Runner`, then remove or branch the current backend checks that accept only `codex`.
- **Add a diagnostic stream**: update stream path mapping, MCP debug tools, CLI debug tools, tail output, and tests. {#260505-agent-diagnostics-tail-debug}
- **Add generated path kinds**: update `generatedPathTarget`, MCP schema, callers, and cleanup rules.

## Common Mistakes

- Setting `Agent.Status` alone does not make an agent reusable; `current/state.json` controls active calls.
- Forgetting `reconcileActiveCall` before status/result/wait leaves dead workers appearing `running`.
- Assuming agent names are arbitrary safe paths; `AgentKey` normalization can make distinct names collide.
- Inferring login state from backend output is brittle; preserve raw backend errors and present configuration options as hints.
- Cancelling by killing only the parent process can leave children alive on Unix; process-group behavior is intentional. {#260505-agent-cancel-recovery}

## Technical Debt

- Malformed lock files without parseable PIDs are not treated as stale, so manual cleanup may be required.
- Windows process liveness is weaker than Unix and can keep dead calls active until better probing exists.
- `OutboxDir`, `Agent.Capabilities`, and some session directories are scaffolded for future use.
