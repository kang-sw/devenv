---
title: Codex agent call prompt delivery
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - prompt-bundle
---

# Codex agent call prompt delivery

## Background

A downstream Windows repository reported that `ws/agents.call` prompts were
handled correctly by Claude-backed named agents but were ignored or obscured by
Codex-backed named agents. The ws runtime lifecycle completed successfully and
recorded `prompt.read.ok` for `current/prompt.md`, but Codex model outputs only
reported session-start orientation or claimed the benchmark question was absent.

The same benchmark prompt worked through Claude Sonnet and Claude Opus 4.6
backends, which suggests the MCP call surface, prompt content, and downstream
repository context were valid. The failure is most likely in the ws Codex
backend adapter, Codex CLI resume invocation, or Codex resumed-session prompt
handling under very large context.

## Evidence

- Runtime events showed `prompt.read.ok`, `backend.call.start` with
  `resume: true`, `backend.session_started`, `backend.call.complete`, and
  `state.output.write.ok`.
- Codex stdout contained normal JSONL lifecycle events and a final
  `agent_message`, so ws treated the call as successful.
- The failing Codex turns reported very large contexts, including input token
  counts above 100k.
- Both `gpt-5.4-mini` and `gpt-5.3-codex-spark` exhibited the issue, making a
  single-model capability explanation unlikely.
- Local code review found that `RunCurrent` reads `current/prompt.md` and passes
  it as `RunnerRequest.Prompt`, and `CodexRunner` appends that value as the
  positional prompt argument to `codex exec` or `codex exec resume`.
- Current `codex exec resume --help` accepts `[SESSION_ID] [PROMPT]`, so the
  existing argument order appears syntactically valid on Codex CLI 0.129.0.
- In WSL2, `cmd.exe /c codex --version` reaches Windows Codex CLI 0.129.0, and
  both `codex exec --help` and `codex exec resume --help` document `-` as a
  stdin prompt source.
- `cmd.exe /c codex` launched from a WSL UNC working directory warns that UNC
  paths are not supported and defaults to the Windows directory, so repository
  reproduction through the Windows binary must set a Windows path explicitly.

## Decisions

- Treat this as a ws Codex backend reliability bug until a focused smoke test
  proves the prompt reaches the model turn under first-call and resume paths.
- Do not assume the `[features].codex_hooks` deprecation warning is causal, but
  remove or modernize it during investigation because it appears in every Codex
  run and adds noisy error events.
- Prefer a small, reproducible sentinel-prompt test before changing prompt
  delivery mechanics.
- Treat stdin `-` prompt delivery as the leading hardening candidate on Windows,
  but preserve a sentinel smoke that compares argv and stdin behavior before
  replacing the prompt path globally.

## Phases

### Phase 1: Reproduce prompt delivery failure

Add or document a focused Codex backend smoke that sends a sentinel prompt
through `ws/agents.call` and requires the final agent message to echo or answer
the sentinel. Cover both first-call and `resume: true` paths. Capture raw
Codex stdout/stderr and ws runtime events when the sentinel is not reflected in
the answer.

The test may be gated behind an opt-in environment variable if it requires a
real Codex CLI session. When available, include a Windows Codex path such as
`cmd.exe /c codex` from WSL2, with an explicit Windows working directory rather
than relying on the WSL UNC current directory.

### Phase 2: Harden CodexRunner prompt delivery

Make CodexRunner command construction directly testable and add unit coverage
for first-call and resume prompt delivery. Prefer stdin `-` for the user prompt
when supported by Codex CLI, especially for Windows execution, while preserving
system prompt, model selection, working directory, stream capture, session id
capture, and hook behavior.

Modernize the hook feature flag from deprecated `features.codex_hooks` to the
current Codex hook flag if compatible with supported Codex CLI versions.

### Phase 3: Improve diagnostics for successful-but-wrong Codex calls

Add diagnostics that make prompt-delivery suspicion visible when Codex returns
a successful lifecycle but the answer appears to omit the prompt. At minimum,
record prompt byte size, whether the call was resumed, Codex CLI version when
available, and the final stdout event shape in existing debug streams without
logging full prompt contents by default.

Document a short operator workaround for affected downstream projects, such as
using Claude-backed agents for read-only explorer work until the Codex prompt
delivery path is verified.
