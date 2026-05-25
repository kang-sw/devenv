---
domain: named-agent-runtime
description: "SQLite-backed agent registry metadata, file-backed payloads, async calls, subqueries, and backend adapter handling."
sources:
  - agents-plugin-tool/internal/wsagent/
  - agents-plugin-tool/internal/wsstate/
  - agents-plugin-tool/internal/wsstore/
related:
  mcp-runtime: "agents.* MCP and CLI handlers are thin wrappers around wsagent.Manager."
  prompt-bundle: "registration resolves embedded prompts into each agent system prompt."
  ws-web-dashboard: "Activity Console reads agent metadata/current state/output/session records as daemon-private projection inputs."
---

# Named Agent Runtime

## Entry Points

- `wsagent.Manager` owns role registration, current-instance resolution, async calls, wait/result/status/tail/cancel/recall compatibility, inbox delivery, and role erasure. {#260505-named-agent-registry-state-layout} {#260511-agent-recall-recovery}
- `wsstate.Manager.Ensure` derives cache, project, worktree, agent, review, lock, and temp paths.
- `wsstore` is the write authority for named-agent role pointers, instance rows, cleanup fences, and retention metadata; `wsstate` still derives the worktree-local payload directory root for prompts, inboxes, current-call state, diagnostic streams, event logs, and outputs. {#260525-named-agent-runtime-metadata-inventory}
- `CodexRunner` invokes `codex exec --json`, captures thread ids, and extracts final agent messages. {#260505-codex-agent-session-jsonl-handling}
- `ClaudeRunner` invokes `claude -p --output-format json`, manages first-call session ids, resumes stored sessions, and extracts final result text. {#260505-claude-agent-runner}
- `GeminiRunner` invokes Gemini CLI `stream-json`, tolerates non-JSON stdout notices, and extracts final text from assistant message chunks. {#260512-gemini-agent-runner}

## Module Contracts

- Registry metadata writes go through `wsstore.AgentDefinition`; `agent.json` is only a bounded read-only legacy import path for creating the first unbound global instance and is removed after import. Corrupt legacy metadata must surface a recovery/re-registration error instead of silently falling back or becoming a parallel source of truth. {#260525-named-agent-runtime-metadata-inventory}
- Agent identity has role and instance layers: the role pointer is keyed by `wsstore.AgentInternalKey(actorID, publicName)` and selects the current `StatePath`, while every successful registration creates a separate `agent_instances` row and payload directory. Do not derive authority from public names, directory names, or instance rows alone. {#260525-named-agent-runtime-metadata-inventory}
- Registration must finish per-instance setup, including prompt materialization and the `registered` event, before advancing the role pointer in SQLite; otherwise a failed re-registration would strand callers on an incomplete new instance instead of the previous current instance. {#260525-named-agent-runtime-metadata-inventory}
- Async call setup order matters: acquire setup lock, create current call state, write prompt snapshot, mark running, append queued event, then start `agents run-current`. {#260505-agent-async-single-call-lifecycle}
- Only `queued` and `running` are active states. Any new status must update busy checks, wait readiness, result handling, registration safety, retention cleanup active guards, cancel, and follow-up text.
- `Result` requires terminal completed state and normally returns `output.md`; a missing output body is reported as `missing_file_backed_payload_recoverable` with the path instead of being treated as SQLite corruption. Output should still be written before current call completion is recorded. {#260505-agent-readiness-result-split}
- Interrupts are inbox files delivered at hook/check-inbox boundaries, not OS signals; actor-scoped calls must pass the same actor id into `Interrupt`, `DeliverPendingInboxScoped`, and the hidden `agents check-inbox --actor-id` hook or messages land in the wrong namespace. {#260505-agent-inbox-interrupt-delivery}
- `Recall` remains a compatibility/manual path only; model-visible recovery after no-result cancellation should retry `Call` on the same registered agent with a recovery prompt. {#260511-agent-recall-recovery} {#260512-agent-cancel-resume-guidance}
- Successful `Result` on an ephemeral agent hides the role pointer, and `Erase` does the same for non-ephemeral roles; neither removes payload directories synchronously. `Print` is legacy and never consumes roles. All three resolve only the current role instance, so historical instance payloads remain retention-owned. {#260505-async-subquery-ephemeral-agent}
- Retired instances are eligible for cleanup seven days after their final call, or seven days after creation if they never ran; cleanup must use SQLite candidates instead of directory scans so retained history does not make ordinary agent operations expensive.
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

- MCP and CLI wrappers mirror `Register`, `Call`, `Wait`, `Result`, `Status`, `Interrupt`, `Tail`, debug streams, `Cancel`, `Print`, and `Erase`; behavior changes require both surfaces and actor-scoped variants where public names may collide with global compatibility registrations.
- Async worker subprocesses must re-resolve a usable runtime binary or launcher when the parent MCP process was started from a plugin cache path that has since been replaced, and `agents run-current` must receive the hidden actor id for actor-scoped calls so worker state matches parent MCP dispatch.
- `ToolProfile` flows into subprocess env as `WS_MCP_TOOL_PROFILE` when the host preserves it; MCP treats it as an optional profile filter, not an authority boundary.
- Worktree scoping is shared by agents, generated review paths, orchestrator locks, and dashboard Activity Console projection; changing cache layout, `agent.json` metadata semantics, or Codex `session_id` persistence affects dashboard feed/transcript behavior as well as agent tools.
- The SQLite state-store is authoritative for role pointers, instance metadata, path indexes, and retention fences, but not for `current/state.json`, `events.jsonl`, or payload bodies. Preserve file-backed diagnostics and result consumption semantics: path fields such as `state_path`, `system_prompt_path`, and `last_output_path` are SQLite metadata indexes, while prompt/stdout/stderr/runtime-log/event/final-output bytes remain file-backed payloads. {#260525-named-agent-runtime-metadata-inventory}
- Agent-instance cleanup participates in prune-run diagnostics but keeps retry metadata on `agent_instances`, not artifact tombstones; instance cleanup deletes directories and must not imply artifact payload ownership or retry semantics.
- Root-omitted MCP `agents.*` lifecycle tools and `subquery` depend on a current lead actor binding from `ws.setup(method: "lead-workflow-bootstrap", root: "<absolute-working-directory>")` or recovery through `ws.setup(id: "<actor-id>")`; hidden explicit-root arguments deliberately route to the unbound global compatibility namespace.
- Named-agent metadata can carry a persistent delegated child actor id; the child setup instruction is appended to `system.md` once and reused across calls. Subqueries carry reader child actors and must register/call in the same actor scope as the parent, then mark child actors inactive when successful ephemeral result consumption erases the agent.
- Prompt registration is static: `system.md` is written at registration time and existing agents do not automatically pick up edited embedded prompts. {#260505-agent-prompt-registration-tier-resolution}
- Agent status includes the detected harness when one influenced registration plus the resolved effort when an alias mapping supplied one; backend error diagnostics include the harness to make alias misrouting visible.
- Registered effort is applied at call time through `RunnerRequest`: Codex emits `model_reasoning_effort`, Claude emits `--effort`, and empty/no-override effort emits no backend option. New backends must opt into their own mapping instead of assuming the manager path is sufficient. {#260505-codex-agent-session-jsonl-handling} {#260505-claude-agent-runner}

## Extension Points & Change Recipes

- **Add a backend**: implement `Runner`, add it to backend runner selection, and keep session persistence, stream capture, status transitions, inbox delivery, and diagnostics on the shared manager path; only backend-specific parsing and invocation details belong in the runner.
- **Add a diagnostic stream**: update stream path mapping, MCP debug tools, CLI debug tools, tail output, and tests. Keep path metadata and payload body ownership distinct when updating the migration inventory. {#260505-agent-diagnostics-tail-debug}
- **Change registry metadata**: update `wsstore.AgentDefinition`, `agentDefinitionFromAgent`, legacy import, role-pointer/instance-history tests, actor/global collision tests, and MCP actor-scoped lifecycle tests together. `wsstore` tests should use local fixtures or source-level inventories rather than importing runtime consumers.
- **Change named-agent cleanup**: keep role deletion as pointer removal, preserve the seven-day retention policy, and update both SQLite candidate selection and `current/state.json` active-state guards; cleanup retry/backoff metadata belongs on the agent-instance row.
- **Add generated path kinds**: update `generatedPathTarget`, MCP schema, callers, and cleanup rules.

## Common Mistakes

- Setting `Agent.Status` or an instance-row status alone does not make an agent reusable or deletable; the current role pointer and the per-instance `current/state.json` are separate authorities.
- Forgetting `reconcileActiveCall` before status/result/wait leaves dead workers appearing `running`.
- Assuming public agent names, `AgentKey` strings, or old instance directories are authoritative; actor-scoped registry keys allow the same public name in different actor/global namespaces, and the role pointer selects the current stored path.
- Inferring login state from backend output is brittle; preserve raw backend errors and present configuration options as hints.
- Treating every stdout line after a completed Codex result as model output can discard a valid Windows result when process-control messages are appended.
- Assuming Gemini has live hook-style interrupt delivery; until a stable mechanism exists, inbox messages are delivered by prepending them to the next resumed call.
- Cancelling by killing only the parent process can leave children alive on Unix; process-group behavior is intentional. {#260505-agent-cancel-recovery}

## Technical Debt

- Malformed lock files without parseable PIDs are not treated as stale, so manual cleanup may be required.
- Windows process liveness is weaker than Unix and can keep dead calls active until better probing exists.
- `OutboxDir`, `Agent.Capabilities`, and some session directories are scaffolded for future use.
