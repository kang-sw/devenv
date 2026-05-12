# Brief: 260512-feat-gemini-named-agent-backend

## Intent

Add Gemini CLI as a supported ws named-agent backend. A registered agent with
`backend: gemini` or a concrete `gemini-*` model should run through the same
`agents.*` lifecycle as Codex and Claude agents, while Gemini-specific
invocation and stream-json parsing stay inside the runner adapter.

## Approach

- Replace the skeleton `GeminiRunner` implementation with a real subprocess
  runner in `agents-plugin-tool/internal/wsagent/gemini.go`.
- Implement a noise-tolerant Gemini stream-json parser that extracts session id
  and final assistant text from Gemini events.
- Build Gemini CLI args for headless stream-json calls, concrete model
  forwarding, resume, stdin prompt delivery, and system prompt prepending.
- Update runner tests so Gemini is no longer treated as an unsupported backend,
  and cover parser, invocation, fake executable, diagnostics, and shared manager
  behavior.

## Constraints

- Keep registration, async call lifecycle, status/result/tail diagnostics,
  cancellation, recall, and backend invocation failure formatting on the shared
  `wsagent.Manager` path.
- Do not probe, read, or store Gemini credentials. Auth failures must surface as
  backend invocation failures with raw backend stderr and existing recovery
  hints.
- Gemini stdout can contain non-JSON notices before, between, or after valid
  JSON events. Preserve raw streams for diagnostics and ignore these notices for
  parsing.
- Caller-facing result text comes only from assistant `message.content` chunks.
  `tool_use`, `tool_result`, and terminal `result` stats are diagnostics, not
  final text.
- Initial Gemini interrupt support is conservative: do not claim live hook
  delivery. Existing inbox behavior can prepend pending messages to the next
  resumed call.

## Out of scope

- Gemini login, credential management, or environment setup.
- Live Gemini hook delivery for `agents.interrupt`.
- New public MCP tools or a Gemini-specific registry/result surface.
- Broad named-agent lifecycle refactors unrelated to the Gemini adapter.

## Details

- `runnerForBackend("gemini")` should return `GeminiRunner{}`.
- First-call args should start with:
  `gemini --output-format stream-json --approval-mode yolo`.
- Append `-m <model>` for non-shorthand concrete models.
- Append `--resume <session_id>` for resumed calls.
- Send the final prompt via stdin. If `SystemPromptPath` is present and
  non-empty, prepend a clear system-instruction block to stdin before the user
  prompt.
- Parse valid JSONL events with these contracts:
  - `init.session_id` becomes `RunnerResult.SessionID` and triggers
    `OnSessionID` immediately.
  - assistant `message.content` chunks append to `RunnerResult.Text`.
  - terminal `result.status == "success"` is required for success.
  - terminal `result.status == "error"` returns a backend error using
    `error.type` and `error.message` when present.
  - missing terminal result, missing session id, or missing assistant text on
    nominal success are errors.
- Capture Gemini CLI version when available and surface it through
  `RunnerResult.BackendVersion`.

## References

- [Must] `ai-docs/spec/named-agent-runtime.md` - `260512-gemini-agent-runner`
  and backend failure diagnostic contracts.
- [Must] `ai-docs/spec/mcp-tools.md` - named-agent MCP tool and model alias
  behavior.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - backend adapter recipe
  and Gemini parser constraints.
- [Must] `ai-docs/tickets/idea/260512-research-gemini-cli-stream-json.md` -
  observed Gemini CLI stream-json, auth, prompt, resume, and tool event shape.
- [Maybe] `agents-plugin-tool/internal/wsagent/codex.go` - streaming subprocess,
  stdin prompt, diagnostics, and parser shape.
- [Maybe] `agents-plugin-tool/internal/wsagent/claude.go` - system prompt,
  resume, and backend shorthand handling.
- [Maybe] `agents-plugin-tool/internal/wsagent/agent.go` - shared manager
  lifecycle and backend invocation diagnostics.
