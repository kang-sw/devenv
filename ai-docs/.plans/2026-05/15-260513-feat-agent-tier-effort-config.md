# Survey: 15-260513-feat-agent-tier-effort-config

## Reusable Components
- `agents-plugin-tool/internal/wsagent/codex.go#L20-L33` — `RunnerRequest`: shared request object already carries model, session, system prompt path, hooks, streams, timeout, process-group, and tool profile into all backend runners.
- `agents-plugin-tool/internal/wsagent/agent.go#L323-L341` — `Agent`: persisted registration metadata already includes optional `Effort`, populated by Phase 1.
- `agents-plugin-tool/internal/wsagent/agent.go#L553-L660` — `Manager.executeCall`: central registered-agent-to-runner handoff; every sync and async backend call passes through this one `RunnerRequest` construction.
- `agents-plugin-tool/internal/wsconfig/config.go#L156-L198` — `ResolveAgentForHarnessConfig`: resolves backend/model/effort and returns empty effort for explicit concrete model registrations.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L22-L43` — `fakeRunner`: captures exact `RunnerRequest` values for manager-level handoff assertions without invoking a real backend.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L74-L130` — `writeFakeClaudeExecutable`: logs fake Claude argv and emits JSON, suitable for exact `--effort` presence/absence checks.

## Existing Patterns
- Codex invocation-builder tests: see `agents-plugin-tool/internal/wsagent/agent_test.go#L718-L782` — tests join argv with `\x00` and assert both required args and forbidden prompt/resume leakage.
- Claude fake-backend tests: see `agents-plugin-tool/internal/wsagent/agent_test.go#L600-L699` — runner tests execute a fake binary, read its arg log, and assert session/model/system/hook behavior.
- Async runner integration pattern: see `agents-plugin-tool/internal/wsagent/agent_test.go#L1036-L1133` — `RunCurrent` tests verify the manager path with a captured `RunnerRequest` and stream diagnostics.
- Claude backend integration pattern: see `agents-plugin-tool/internal/wsagent/agent_test.go#L1135-L1215` — registered Claude agents are driven through async `Call` + `RunCurrent`, then fake argv is inspected for first-call/resume behavior.
- Gemini no-special-effort precedent: see `agents-plugin-tool/internal/wsagent/gemini.go#L106-L129` and `agents-plugin-tool/internal/wsagent/gemini_test.go#L89-L148` — invocation construction trims model/session inputs and keeps prompt/system text out of argv, with no effort concept.
- Config effort test setup: see `agents-plugin-tool/internal/wsconfig/config_test.go#L158-L178` — `SetAgentsTierForHarness` can store a harness-specific effort and `ResolveAgentForHarnessConfig` confirms registration-visible effort.

## Relevant Interfaces
- `agents-plugin-tool/internal/wsagent/codex.go#L147-L176` — `buildCodexInvocation`: single place constructing `codex exec` argv for both first call and resume, including model/system/hook config and stdin marker.
- `agents-plugin-tool/internal/wsagent/claude.go#L57-L85` — `claudeArgs`: single place constructing Claude argv for first call, resume, system prompt, hooks, and final prompt argument.
- `agents-plugin-tool/internal/wsagent/claude.go#L19-L55` — `ClaudeRunner.Call`: calls `claudeArgs` for initial invocation and again after `hook_stopped`, so shared argv changes apply to both paths.
- `agents-plugin-tool/internal/wsagent/agent.go#L398-L496` — `Manager.Register`: resolves alias effort, persists `Agent.Effort`, and records it in the registration event.
- `agents-plugin-tool/internal/wsconfig/config.go#L366-L379` — `normalizeOptionalEffort`: Phase 1 normalization limits possible runner values to empty, `low`, `medium`, `high`, or `xhigh`.

## Constraints
- `ai-docs/tickets/ready/260513-feat-agent-tier-effort-config.md#L73-L85` — Phase 2 is only runner application; Phase 3 owns specs, mental models, runtime metadata, and guidance.
- `ai-docs/spec/named-agent-runtime.md#L141-L151` — Codex planned contract is `model_reasoning_effort` only for resolved non-empty effort; no override when effort is absent.
- `ai-docs/spec/named-agent-runtime.md#L170-L190` — Claude planned contract is `--effort` only for resolved non-empty effort; no option when effort is absent.
- `ai-docs/mental-model/named-agent-runtime.md#L31-L38` — backend-specific session and output parsing belong inside runners; manager lifecycle should stay shared.
- `ai-docs/mental-model/named-agent-runtime.md#L53-L54` — alias mappings may carry effort, but concrete model registration resolves no effort override and `agents.register` has no direct effort input.
- `agents-plugin-tool/internal/wsagent/agent.go#L647-L660` — current handoff preserves model, session, system prompt, hook, streams, timeout, process group, and tool profile; new fields should not disturb these.

## Opinion
- The codebase reality matches the brief: Phase 1 has persisted `Agent.Effort`, and only `RunnerRequest` plus Codex/Claude argv builders appear to block runner application.
- Main risk is test coverage placement: manager handoff needs a `fakeRunner` assertion, while backend flag spelling is best covered at the existing invocation-builder/fake-backend test layer.
- No spec or doc entry looked wrong for Phase 2; the only intentionally stale entry is the named-agent mental model technical debt saying effort is not yet enforced.
