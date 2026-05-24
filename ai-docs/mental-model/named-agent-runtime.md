---
domain: named-agent-runtime
description: "File-backed named agents, async calls, locks, subqueries, and backend adapter handling."
sources:
  - agents-plugin-tool/internal/wsagent/
  - agents-plugin-tool/internal/wsstate/
  - agents-plugin-tool/internal/wsstore/
related:
  mcp-runtime: "agents.* MCP and CLI handlers are thin wrappers around wsagent.Manager."
  prompt-bundle: "registration resolves embedded prompts into each agent system prompt."
---

# Named Agent Runtime

## Entry Points

- `wsagent.Manager` owns registration, async calls, wait/result/status/tail/cancel/recall compatibility, inbox delivery, and erasure. {#260505-named-agent-registry-state-layout} {#260511-agent-recall-recovery}
- `wsstate.Manager.Ensure` derives cache, project, worktree, agent, review, lock, and temp paths.
- `wsstore` is available for future actor-owned metadata, leases, retention, and artifact indexes, but named agents remain file/JSON-backed until a migration ticket rewires `wsagent`.
- `CodexRunner` invokes `codex exec --json`, captures thread ids, and extracts final agent messages. {#260505-codex-agent-session-jsonl-handling}
- `ClaudeRunner` invokes `claude -p --output-format json`, manages first-call session ids, resumes stored sessions, and extracts final result text. {#260505-claude-agent-runner}
- `GeminiRunner` invokes Gemini CLI `stream-json`, tolerates non-JSON stdout notices, and extracts final text from assistant message chunks. {#260512-gemini-agent-runner}

## Module Contracts

- All agent paths must come from `wsstate.Ensure(root)` plus `AgentKey(name)`; bypassing this splits or merges worktree state incorrectly.
- Async call setup order matters: acquire setup lock, create current call state, write prompt snapshot, mark running, append queued event, then start `agents run-current`. {#260505-agent-async-single-call-lifecycle}
- Only `queued` and `running` are active states. Any new status must update busy checks, wait readiness, result handling, register reset safety, cancel, and follow-up text.
- `Result` requires terminal completed state and `output.md`; output must be written before current call completion is recorded. {#260505-agent-readiness-result-split}
- Interrupts are inbox files delivered at hook/check-inbox boundaries, not OS signals. {#260505-agent-inbox-interrupt-delivery}
- `Recall` remains a compatibility/manual path only; model-visible recovery after no-result cancellation should retry `Call` on the same registered agent with a recovery prompt. {#260511-agent-recall-recovery} {#260512-agent-cancel-resume-guidance}
- Successful `Result` erases ephemeral agents; `Print` is legacy and does not consume them. {#260505-async-subquery-ephemeral-agent}
- Backend invocation failures are formatted at the call site with raw error text, bounded PATH-detected backend hints, and reconfiguration guidance; do not run separate model/login probes during registration or config inspection. {#260505-agent-backend-failure-diagnostics}
- Codex JSONL parsing treats non-JSON stdout as fatal until both session id and final agent message are available; trailing process-control noise after completion is ignored. {#260505-codex-jsonl-trailing-noise-tolerance}
- CodexRunner sends user prompts through stdin with Codex CLI `-` instead of positional argv so multiline and Windows prompts survive first-call and resume paths. {#260508-codex-stdin-prompt-delivery}
- Codex prompt-delivery diagnostics log bounded metadata such as prompt byte size, delivery path, backend version, resume state, and final event shape, not prompt contents. {#260508-codex-prompt-delivery-diagnostics}
- Backend adapters must fit `RunnerRequest` and `RunnerResult`; keep backend-specific session, output parsing, and backend flag spelling inside the runner instead of branching the manager lifecycle. Resolved alias effort rides the shared request but is enforced only where a runner translates it. {#260505-claude-agent-runner}
- Claude `terminal_reason: hook_stopped` is an intermediate adapter state: resume the same session so hook-delivered lead messages produce a final output instead of an empty completed result. {#260505-claude-agent-runner}
- Gemini invocation is stdin-only: shorthand aliases such as `gemini` stay out of argv, concrete models are passed with `-m`, stored sessions resume with `--resume`, and resolved system prompts are prepended inside stdin with an explicit system/user boundary. {#260512-gemini-agent-runner}
- Gemini parsing is intentionally more tolerant than Codex parsing: stdout notices can be diagnostics, nested or top-level message/result shapes are accepted, and tool-use/tool-result content is ignored, but completion still requires terminal success, a session id, and accumulated assistant text. {#260512-gemini-agent-runner}
- Gemini session persistence happens as soon as the first init/session id appears; if that callback fails, the runner cancels the child process before returning so failed state writes do not leave a sleeping backend call behind.
- Model selection treats `light`/`core`/`deep` as portable aliases on the `model` field; concrete model names win, legacy `tier` is compatibility-only when `model` is absent, and alias resolution can branch by MCP harness. Alias mappings are the single route for named-agent effort: concrete model registration resolves no effort override, and `agents.register` has no direct effort input. {#260508-harness-aware-model-aliases} {#260508-mcp-harness-detection}
- Alias override persistence updates the explicit harness key, detected MCP session harness key, or default key; `backend` remains the execution backend in the stored mapping. Effort is stored on the selected alias mapping only; omitting effort during an alias update clears any prior effort, while explicit `none`/empty also stores the no-override state. Explicit backend registrations reject alias mappings whose backend/model imply a different backend, leaving the concrete model empty rather than constructing a mismatched pair. {#260512-backend-model-resolution-consistency} {#260513-harness-local-agent-tier-config}

## Coupling

- MCP and CLI wrappers mirror `Register`, `Call`, `Wait`, `Result`, `Status`, `Interrupt`, `Tail`, debug streams, `Cancel`, `Print`, and `Erase`; behavior changes require both surfaces.
- Async worker subprocesses must re-resolve a usable runtime binary or launcher when the parent MCP process was started from a plugin cache path that has since been replaced.
- `ToolProfile` flows into subprocess env as `WS_MCP_TOOL_PROFILE` when the host preserves it; MCP treats it as an optional profile filter, not an authority boundary.
- Worktree scoping is shared by agents, generated review paths, and orchestrator locks; changing cache layout affects all three.
- The SQLite state-store foundation is adjacent to named agents but not yet authoritative for `agent.json`, `current/state.json`, `events.jsonl`, or output files. Future migration must preserve current file-backed diagnostics and result consumption semantics.
- Root-omitted MCP `agents.register`, `agents.call`, and `subquery` now depend on a current lead actor binding from `ws.setup(method: "lead-workflow-bootstrap", root: "<cwd>" or absolute path)` or recovery through `ws.setup(id: "<actor-id>")`; hidden explicit-root arguments remain a compatibility override.
- Prompt registration is static: `system.md` is written at registration time and existing agents do not automatically pick up edited embedded prompts. {#260505-agent-prompt-registration-tier-resolution}
- Agent status includes the detected harness when one influenced registration plus the resolved effort when an alias mapping supplied one; backend error diagnostics include the harness to make alias misrouting visible.
- Registered effort is applied at call time through `RunnerRequest`: Codex emits `model_reasoning_effort`, Claude emits `--effort`, and empty/no-override effort emits no backend option. New backends must opt into their own mapping instead of assuming the manager path is sufficient. {#260505-codex-agent-session-jsonl-handling} {#260505-claude-agent-runner}

## Extension Points & Change Recipes

- **Add a backend**: implement `Runner`, add it to backend runner selection, and keep session persistence, stream capture, status transitions, inbox delivery, and diagnostics on the shared manager path; only backend-specific parsing and invocation details belong in the runner.
- **Add a diagnostic stream**: update stream path mapping, MCP debug tools, CLI debug tools, tail output, and tests. {#260505-agent-diagnostics-tail-debug}
- **Add generated path kinds**: update `generatedPathTarget`, MCP schema, callers, and cleanup rules.

## Common Mistakes

- Setting `Agent.Status` alone does not make an agent reusable; `current/state.json` controls active calls.
- Forgetting `reconcileActiveCall` before status/result/wait leaves dead workers appearing `running`.
- Assuming agent names are arbitrary safe paths; `AgentKey` normalization can make distinct names collide.
- Inferring login state from backend output is brittle; preserve raw backend errors and present configuration options as hints.
- Treating every stdout line after a completed Codex result as model output can discard a valid Windows result when process-control messages are appended.
- Assuming Gemini has live hook-style interrupt delivery; until a stable mechanism exists, inbox messages are delivered by prepending them to the next resumed call.
- Cancelling by killing only the parent process can leave children alive on Unix; process-group behavior is intentional. {#260505-agent-cancel-recovery}

## Technical Debt

- Malformed lock files without parseable PIDs are not treated as stale, so manual cleanup may be required.
- Windows process liveness is weaker than Unix and can keep dead calls active until better probing exists.
- `OutboxDir`, `Agent.Capabilities`, and some session directories are scaffolded for future use.
