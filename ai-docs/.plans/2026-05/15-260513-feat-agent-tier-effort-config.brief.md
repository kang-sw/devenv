# Brief: 260513-feat-agent-tier-effort-config

## Intent

Apply the resolved named-agent alias effort to backend runner invocations for
the Phase 2 slice only. Phase 1 already stores and exposes effort metadata; this
slice must make Codex and Claude calls actually receive the configured effort
while preserving the no-override default.

## Approach

- Thread the registered agent's resolved effort into `RunnerRequest`.
- For Codex, when effort is non-empty, add a `-c model_reasoning_effort=<value>`
  config override to the generated `codex exec` invocation.
- For Claude, when effort is non-empty, add `--effort <value>` to the generated
  `claude -p` invocation.
- Leave Gemini behavior unchanged unless an existing shared request field must
  be carried through harmlessly.
- Add runner tests for Codex effort, Claude effort, and no-effort/no-override
  behavior.

## Constraints

- Implement only Phase 2. Do not update specs, mental models, runtime metadata,
  or user-facing docs in the implementation commit; those belong to the
  post-implementation doc pipeline and Phase 3.
- Do not add an effort input to `agents.register`, `subquery`, prompt
  frontmatter, or CLI call surfaces.
- Do not force effort when the resolved effort is empty.
- Keep backend-specific flag spelling inside backend runners.
- Preserve existing model, system prompt, hook, resume, stdin prompt, timeout,
  and tool-profile behavior.

## Out Of Scope

- Adding Gemini effort support.
- Changing alias resolution or config persistence semantics from Phase 1.
- Reworking agent registration, status output, or config display.

## Details

- `agents-plugin-tool/internal/wsagent/agent.go` should pass `agent.Effort` into
  the `RunnerRequest` used by `runner.Call`.
- `agents-plugin-tool/internal/wsagent/codex.go` should add the Codex config
  override only when `strings.TrimSpace(req.Effort)` is non-empty.
- `agents-plugin-tool/internal/wsagent/claude.go` should add `--effort` only
  when `strings.TrimSpace(req.Effort)` is non-empty.
- Tests should assert exact argument presence and absence around the existing
  invocation builders or fake backend logs.

## References

- [Must] `ai-docs/spec/named-agent-runtime.md` anchors
  `260508-harness-aware-model-aliases`, `260505-codex-agent-session-jsonl-handling`,
  and `260505-claude-agent-runner` - resolved effort metadata and backend
  invocation contracts.
- [Must] `ai-docs/tickets/ready/260513-feat-agent-tier-effort-config.md` -
  Phase 2 acceptance criteria.
- [Must] `agents-plugin-tool/internal/wsagent/agent.go` - registered agent to
  runner request handoff.
- [Must] `agents-plugin-tool/internal/wsagent/codex.go` - Codex invocation
  construction.
- [Must] `agents-plugin-tool/internal/wsagent/claude.go` - Claude invocation
  construction.
- [Must] `agents-plugin-tool/internal/wsagent/agent_test.go` - existing Codex,
  Claude, and manager runner tests.
