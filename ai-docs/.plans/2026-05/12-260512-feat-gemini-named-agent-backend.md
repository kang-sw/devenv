# Survey: 12-260512-feat-gemini-named-agent-backend

## Reusable Components
- `agents-plugin-tool/internal/wsagent/gemini.go#L8-L52` — `GeminiRunner`, `buildGeminiInvocation`, `parseGeminiStreamJSON`: skeleton already defines the adapter boundary, invocation DTO, and HOLE-marked parser/runner contracts for this brief.
- `agents-plugin-tool/internal/wsagent/codex.go#L70-L132` — `CodexRunner.Call`: subprocess runner pattern for timeout, `configureRunnerCommand`, cwd, `WS_MCP_TOOL_PROFILE`, stdin prompt delivery, stderr buffering, stdout teeing, incremental parsing, wait/error formatting, and result diagnostics fields.
- `agents-plugin-tool/internal/wsagent/codex.go#L135-L175` — `codexVersion` and `buildCodexInvocation`: cached version probe and no-prompt-in-argv invocation builder pattern with `PromptDelivery: "stdin"`.
- `agents-plugin-tool/internal/wsagent/codex.go#L194-L255` — `parseCodexJSONLStreamPartial`/`requireCompleteCodexResult`: `bufio.Reader.ReadBytes` large-line JSONL loop, immediate `OnSessionID`, final shape tracking, and completeness checks; Gemini differs on non-JSON tolerance and assistant chunk accumulation.
- `agents-plugin-tool/internal/wsagent/claude.go#L57-L85` — `claudeArgs`: concrete-model vs backend-shorthand filtering via `isBackendShorthand`, resume arg handling, and system prompt file reading.
- `agents-plugin-tool/internal/wsagent/claude.go#L87-L132` — `runClaude`: non-streaming subprocess error path that preserves backend stderr on nonzero exit.
- `agents-plugin-tool/internal/wsagent/runner_command_unix.go#L10-L18` and `agents-plugin-tool/internal/wsagent/runner_command_windows.go#L10-L13` — `configureRunnerCommand`: shared process-group behavior used by backend runners unless the async worker already owns the process group.
- `agents-plugin-tool/internal/wsagent/agent.go#L547-L702` — `Manager.executeCall`: shared lifecycle, inbox prepending, stream capture, `RunnerRequest` construction, `RunnerResult` diagnostics logging, output write, and backend failure wrapping.
- `agents-plugin-tool/internal/wsagent/agent.go#L1468-L1501` — `backendInvocationError`: raw backend error plus PATH and re-registration/config hints; Gemini auth/CLI failures should flow here instead of adding credential probes.

## Existing Patterns
- Streaming adapter pattern: see `agents-plugin-tool/internal/wsagent/codex.go#L70-L132` — start the process, tee stdout into diagnostics, parse while the process runs so `OnSessionID` can persist session state before completion, then wait and attach `BackendVersion`/`PromptDelivery`.
- System prompt and shorthand model handling: see `agents-plugin-tool/internal/wsagent/claude.go#L57-L70` and `agents-plugin-tool/internal/wsagent/claude.go#L192-L199` — skip `-m/--model` for `gemini` shorthand, but forward concrete `gemini-*` models.
- Async manager completion diagnostics: see `agents-plugin-tool/internal/wsagent/agent_test.go#L1003-L1100` — tests assert stdout/stderr capture, completed current call state, result split, tail sections, `prompt_delivery`, and `final_event_shape` runtime log fields.
- Backend runner integration test style: see `agents-plugin-tool/internal/wsagent/agent_test.go#L1102-L1182` — registers a concrete backend, uses a fake executable on PATH, queues via `Call`, runs `RunCurrent`, then checks stored session, result, and resume argv.
- Failure diagnostics test style: see `agents-plugin-tool/internal/wsagent/agent_test.go#L1421-L1460` and `agents-plugin-tool/internal/wsagent/agent_test.go#L1495-L1540` — failed runners and unsupported backends should surface `backend invocation failed`, raw error, PATH-detected binaries, and recovery hints.
- Skeleton test location: see `agents-plugin-tool/internal/wsagent/gemini_test.go#L5-L24` — current skipped tests enumerate parser, invocation, fake executable, diagnostics, env propagation, callback timing, and version-capture targets.

## Relevant Interfaces
- `agents-plugin-tool/internal/wsagent/codex.go#L16-L42` — `Runner`, `RunnerRequest`, `RunnerResult`: Gemini must implement this interface; useful result fields are `SessionID`, `Text`, `BackendVersion`, `PromptDelivery`, and `FinalEventShape`.
- `agents-plugin-tool/internal/wsagent/codex.go#L44-L55` — `runnerForBackend`: `gemini` is already selected and no longer unsupported.
- `agents-plugin-tool/internal/wsagent/gemini.go#L14-L18` — `geminiInvocation`: invocation builder return type with `Args`, `PromptStdin`, and `PromptDelivery`.
- `agents-plugin-tool/internal/wsconfig/config.go#L112-L177` — `ResolveAgentForHarness`, `ModelAlias`, `InferBackend`: concrete models containing `gemini` already infer backend `gemini`; portable aliases and compatibility tiers are resolved before the runner sees a request.
- `agents-plugin-tool/internal/mcp/server.go#L623-L644` and `agents-plugin-tool/internal/mcp/server.go#L1405-L1420` — MCP `agents.register`: backend/model inputs already pass through to `wsagent.Manager.Register`; no new MCP tool/schema is needed for Gemini.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L715-L747` — CLI `agents register`: CLI backend/model flags already route to `wsagent.RegisterOptions`.

## Constraints
- `ai-docs/spec/named-agent-runtime.md#L172-L202` — Gemini must share the existing `agents.*` lifecycle, use headless `stream-json`, stdin prompts, concrete model forwarding, resume with stored session id, system prompt text in stdin, tolerant stdout parsing, and no credential probing.
- `ai-docs/spec/named-agent-runtime.md#L204-L210` — backend invocation failures must preserve raw backend error and append bounded known-backend/reconfiguration hints.
- `ai-docs/spec/mcp-tools.md#L165-L178` — public model selection remains `agents.register` `model` aliases or concrete provider names; `tier` stays compatibility-only.
- `ai-docs/mental-model/named-agent-runtime.md#L31-L38` — backend adapters must stay within `RunnerRequest`/`RunnerResult`; Gemini parser can ignore stdout notices but still requires terminal success, session id, and assistant text.
- `ai-docs/mental-model/named-agent-runtime.md#L40-L47` — MCP/CLI wrappers mirror manager behavior, prompt registration is static, and `ToolProfile` only flows through env as optional profile context.
- `ai-docs/tickets/idea/260512-research-gemini-cli-stream-json.md#L21-L31` — auth is external: API-key headless mode can fail with code 41 when `GEMINI_API_KEY` is absent, while OAuth headless mode uses `GOOGLE_GENAI_USE_GCA=true`; implementation should not infer or manage login state.
- `ai-docs/tickets/idea/260512-research-gemini-cli-stream-json.md#L33-L51` — terminal `result` stats do not contain final text; parser must accumulate assistant `message.content` chunks.
- `ai-docs/tickets/idea/260512-research-gemini-cli-stream-json.md#L59-L84` — stdin prompts and `--resume <session_id>` are observed to work; `tool_use`/`tool_result` are diagnostics and not final caller text.
- `ai-docs/tickets/idea/260512-research-gemini-cli-stream-json.md#L86-L102` — parser must ignore non-JSON notices before/between/after JSON events, capture first `init.session_id`, treat terminal error events as backend errors, and fail on missing terminal/session/text data.
- `agents-plugin-tool/internal/wsagent/agent.go#L629-L654` — pending inbox messages are prepended before runner invocation and `InterruptHookCommand` is passed only when stream capture is active; Gemini should not claim live hook delivery unless it implements a real hook path.

## Opinion
- `agents-plugin-tool/internal/wsagent/gemini.go#L20-L52` is the main write surface; the surrounding manager/config/MCP paths already recognize `gemini`, so broad lifecycle refactors look unnecessary and risky.
- `agents-plugin-tool/internal/wsagent/codex.go#L70-L132` is the closest subprocess template, but `agents-plugin-tool/internal/wsagent/codex.go#L220-L227` is deliberately stricter on non-JSON stdout than Gemini may be; copying it verbatim would violate the research ticket.
- `agents-plugin-tool/internal/wsagent/gemini_test.go#L5-L24` should probably be expanded rather than replaced elsewhere; it already scopes the missing parser/invocation/runner coverage in one package-local test file.
