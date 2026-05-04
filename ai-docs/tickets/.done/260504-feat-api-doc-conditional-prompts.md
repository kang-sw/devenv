---
title: API docs conditional prompt injection
related:
  260504-feat-agents-plugin-api-docs-mcp: implemented Agents MCP API docs surface that owns api-doc-manager registration
  260429-feat-api-deps: Claude prior art for ws-ask-api and prompt-conditioned cargo-brief guidance
  260503-feat-agents-plugin-agent-session-runtime: RegisterOptions and named-agent metadata surface used by API docs workers
parent: 260503-epic-agents-plugin-skill-porting
completed: 2026-05-04
---

# API docs conditional prompt injection

## Background

Claude prior art supports conditional prompt insertion through
`ws-named-agent new --prompt-cond BINARY[=PROMPT]`. The recorded example is
`cargo-brief`: when the binary is available on `PATH`, Rust API documentation
agents receive a short prompt telling them to prefer `cargo brief` commands over
web lookup.

The Agents MCP API docs migration currently registers per-domain
`api-doc-<domain>` workers with only the base `api-doc-manager` prompt. It should
gain the same conditional prompt behavior without exposing another public MCP
argument unless a later use case requires it.

## Decisions

- Add a runtime-internal conditional prompt mechanism first; do not expose it on
  public `agents.register` in the first slice.
- Use an ask-api-specific prompt stem, `api-doc-cargo-brief`, instead of
  polluting the shared infra namespace with a broad `cargo-brief` name.
- Evaluate conditional prompts at worker registration time by checking whether
  the configured binary exists on `PATH`.
- Treat `api-doc-<domain>` workers as short-lived hot-cache sessions, not durable
  long-running knowledge stores.
- Use a five-minute TTL for API docs manager sessions so newly installed tools
  such as `cargo-brief` are picked up naturally without prompt-hash drift
  machinery.

## Prior Art

- `claude-plugin/infra/cargo-brief.md` contains the prompt body.
- `claude-plugin/bin/ws-named-agent` implements `--prompt-cond
  BINARY[=PROMPT_NAME]` by resolving the prompt only when `shutil.which(binary)`
  succeeds.
- `ai-docs/_index.md` records `cargo-brief.md` as injected by
  `--prompt-cond cargo-brief` when the binary is on `PATH`.

## Phases

### Phase 1: Conditional prompt registration primitive

Add an internal registration option for conditional prompt refs. Suggested
shape:

```text
ConditionalPromptRefs: [
  { binary: "cargo-brief", prompt_ref: "api-doc-cargo-brief" }
]
```

Registration evaluates each condition before resolving the merged system prompt.
When the binary exists on `PATH`, append the prompt ref to the normal prompt
list. When it is absent, leave the prompt list unchanged.

Acceptance criteria:

- Conditional prompt refs are internal runtime data, not public MCP
  `agents.register` arguments.
- Existing `prompts` and `prompt_refs` behavior is unchanged.
- Tests cover condition-present and condition-absent registration.

### Phase 2: API docs cargo-brief prompt

Port the Claude `cargo-brief` prompt into the embedded Agents prompt bundle as
`api-doc-cargo-brief`.

The prompt should stay short and scoped:

- State that `cargo-brief` is available.
- For Rust API questions, run `cargo-brief --help` first.
- Prefer `cargo brief` subcommands for Rust API lookup before web or uncached
  documentation lookup.

Acceptance criteria:

- The prompt is embedded and listed in runtime prompt metadata.
- `agents-plugin/runtime.json` prompt bundle metadata is updated.
- The broad `cargo-brief` stem is not introduced.

### Phase 3: API docs worker TTL and conditional prompt use

Update API docs manager registration so each `api-doc-<domain>` worker uses the
conditional prompt ref:

```text
binary: cargo-brief
prompt_ref: api-doc-cargo-brief
```

Before reusing an existing `api-doc-<domain>` worker, check its recent-use
timestamp. If the worker is older than five minutes, erase and re-register it
before dispatching the next question. This makes conditional prompt changes
eventually consistent while preserving hot-cache continuity for short follow-up
questions.

Acceptance criteria:

- Exact domain locks still serialize same-domain API docs calls.
- A worker within the TTL is reused.
- A worker past the TTL is erased and re-registered before the next call.
- Installing `cargo-brief` after a previous ask-api call is picked up after the
  TTL without requiring prompt hash drift detection.
- If the worker is active when TTL expiry is observed, the implementation either
  reuses the active worker for that call or fails with a clear retryable error;
  it must not erase an active call.

### Result (a714918) - 2026-05-04

Implemented the conditional prompt path directly during API docs dogfooding.
`wsagent.RegisterOptions` now supports internal conditional prompt refs, the
embedded prompt bundle includes `api-doc-cargo-brief`, and API docs managers add
that prompt when `cargo-brief` is available on `PATH`.

API docs manager workers now use a five-minute hot-cache TTL. Idle expired
workers are erased and re-registered before the next call, while active workers
are not erased.

The spec mismatch noted above remains intentionally deferred because this branch
does not edit `ai-docs/spec/`; the user plans a later forge-spec pass.

Verification:

- `cd agents-plugin-tool && go test ./internal/wsagent ./internal/mcp ./internal/wsprompt`
- `cd agents-plugin-tool && go test ./...`
- `git diff --check`
- `go build -o <tmp>/ws-mcp ./cmd/ws-mcp && <tmp>/ws-mcp runtime info`
- `go run ./cmd/ws-mcp serve --stdio --root /home/swkang/devenv` with a no-hint `api.ask` smoke for `ratatui`

## Deferred Spec Follow-up

- The TTL behavior changes the existing API deps spec, which currently says
  per-domain executor sessions persist until explicit erase. The mismatch is
  intentionally deferred because this branch does not edit `ai-docs/spec/`; the
  user plans a later forge-spec pass.
