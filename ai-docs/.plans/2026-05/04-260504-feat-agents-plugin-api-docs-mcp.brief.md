# Brief: 260504-feat-agents-plugin-api-docs-mcp

## Intent

Port the Claude `ws-ask-api` prior art into the Agents/Codex MCP runtime as a
host-neutral API documentation primitive. The result should let callers ask
third-party API documentation questions through MCP tools while preserving the
project-local `ai-docs/.deps/` cache and per-domain persistent manager sessions.

## Approach

- Add embedded `pre-router` and `api-doc-manager` prompts to
  `agents-plugin-tool/internal/wsprompt/prompts/`.
- Add MCP tools `api.ask` and `api.list` in `agents-plugin-tool/internal/mcp`.
- Implement runtime orchestration for `api.ask` using existing `wsagent`
  register/call/wait/print primitives where practical.
- Preserve per-domain locking in the runtime layer.
- Update `agents-plugin/runtime.json` and shared workflow guidance so callers
  see the concrete `ws/api.*` tools.
- Add tests for prompt discovery, tool listing, `api.list`, exact-hint routing,
  pre-router routing, multi-domain partial result boundaries, and all-domain
  failure behavior.

## Constraints

- Do not modify `claude-plugin/`; use it only as prior art.
- Do not expose public `api.refresh` or `api.check_stale` tools in this pass.
- Staleness checking belongs inside the `api-doc-manager` prompt: the manager
  creates the script during bootstrap and checks staleness at session start.
- Keep `ai-docs/.deps/` hidden from ordinary worker-facing guidance. The caller
  should use `ws/api.ask` and `ws/api.list`, not read the cache directly.
- Multi-domain synthesis remains an agent responsibility, but the runtime must
  keep per-domain result boundaries and failure metadata so partial success is
  usable.
- Do not edit `ai-docs/spec/` on this branch; update runtime/reference docs and
  tickets instead when documentation is needed.
- Preserve existing MCP profile gating. API docs tools should be visible where
  ordinary lead/delegate workflow tools are visible, but not accidentally
  promoted into leaf-only profiles if the existing profile code hides similar
  orchestration tools.

## Out of scope

- Reintroducing PATH-only `ws-ask-api` helpers for Agents.
- Porting Claude CLI management flags as MCP tools.
- Building a rich structured answer synthesizer in runtime. Runtime should
  orchestrate, preserve boundaries, and report failures; agents synthesize.
- Reworking the durable agent runtime beyond what this tool needs.

## Details

Public MCP tools:

- `api.list(root?: string)` returns existing domain directory names under
  `<root>/ai-docs/.deps/`, sorted, excluding dot-prefixed directories. It should
  work when `.deps` does not exist by returning an empty list.
- `api.ask(prompt: string, domain_hint?: string, root?: string)` resolves one or
  more domains and dispatches to `api-doc-<domain>` managers.

`api.ask` behavior:

- If `domain_hint` exactly names an existing domain directory, skip pre-router
  and use that single domain.
- Otherwise invoke a one-shot `pre-router` prompt with:

  ```text
  Hint: <domain-hint or "(none)">
  Existing domains:
  <domain-a>
  <domain-b>
  ...
  Prompt: <prompt>
  ```

- Parse pre-router output as one domain slug per non-empty line.
- If no domains are resolved, return an error.
- For each resolved domain:
  - Ensure `<root>/ai-docs/.deps/<domain>/` exists.
  - Acquire a per-domain lock before manager registration/call/wait/print.
  - Register or reuse persistent manager session `api-doc-<domain>` with
    prompt `api-doc-manager`. Avoid destroying an active existing session.
  - Send the original prompt to the manager.
- Dispatch distinct domains concurrently where feasible. Same-domain
  concurrent calls must serialize through the per-domain lock.
- Preserve per-domain response boundaries. A practical text output is
  acceptable if it includes domain headings and explicit error blocks; JSON is
  also acceptable if it fits existing MCP response conventions.
- If at least one domain succeeds, return success with successful domain
  outputs and failed-domain metadata. If every domain fails, return an MCP tool
  error.

Prompt contracts:

- `pre-router` should be a lightweight prompt whose output is only canonical
  domain slugs, one per line, no prose.
- `api-doc-manager` owns exactly one `ai-docs/.deps/<domain>/` tree. It may
  fetch official docs, write `README.md`, `meta.yaml`, `scripts/detect-version`,
  `scripts/fetch`, `scripts/check-stale`, `l1.md`, `l2.md`, `l3.md`, and
  subdomain docs. It must cite cached docs or official fetched sources.
- `api-doc-manager` must check staleness at the start of each session before
  answering.

Likely source files:

- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/server_test.go`
- `agents-plugin-tool/internal/wsagent/agent.go`
- `agents-plugin-tool/internal/wsagent/agent_test.go`
- `agents-plugin-tool/internal/wsprompt/prompts.go`
- `agents-plugin-tool/internal/wsprompt/prompts_test.go`
- `agents-plugin-tool/internal/wsprompt/prompts/pre-router.md`
- `agents-plugin-tool/internal/wsprompt/prompts/api-doc-manager.md`
- `agents-plugin/runtime.json`
- `agents-plugin/skills/workflow/SKILL.md`
- `agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`
- `ai-docs/ref/ws-mcp.md`
- `ai-docs/ref/ws-agent-runtime.md`

Test command:

```sh
cd agents-plugin-tool && go test ./...
```

## References

- `ai-docs/tickets/todo/260504-feat-agents-plugin-api-docs-mcp.md` - source
  ticket and phase acceptance criteria.
- `ai-docs/spec/api-deps.md` - Claude prior-art behavior for `ws-ask-api`,
  pre-router, per-domain executor, cache layout, locks, and worker contract.
- `claude-plugin/bin/ws-ask-api` - existing Python orchestration prior art.
- `claude-plugin/bin/ws-ask-api-internal` - existing per-domain locking and
  manager dispatch prior art.
- `claude-plugin/infra/prompts/pre-router.md` - prompt content to normalize.
- `claude-plugin/infra/prompts/api-doc-manager.md` - prompt content to
  normalize, with stale check handled inside the manager contract.
- `ai-docs/ref/ws-mcp.md` - MCP runtime contract and profile/tool conventions.
- `ai-docs/ref/ws-agent-runtime.md` - durable agent session semantics and tier
  behavior.
