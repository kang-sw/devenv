# Plan: 260504-feat-agents-plugin-api-docs-mcp

## Survey Notes

The existing MCP server is centered in `agents-plugin-tool/internal/mcp/server.go`.
Tool dispatch is handled by `Server.callTool`, and tool schemas are returned
from `tools()`. Runtime metadata comes from `runtimeInfo()` and must stay in
sync with `agents-plugin/runtime.json`.

The durable agent runtime already provides the needed lower-level operations in
`agents-plugin-tool/internal/wsagent/agent.go`: `Register`, `Call`, `Wait`,
`Print`, `Subquery`, and the async current-call state. Prefer reusing these
instead of introducing another backend runner path. For one-shot pre-router
work, either reuse `Subquery` patterns or add a small internal one-shot helper
that registers a suppressed-orientation prompt and performs a bounded sync call.

Prompt bundle discovery is automatic through `wsprompt`; adding top-level files
under `internal/wsprompt/prompts/` should be enough for prompt resolution, but
tests and `runtime.json` must be updated because the prompt bundle hash changes.

## Implementation Steps

1. Add prompt files.
   - Port `pre-router.md` from Claude prior art with host-neutral wording.
   - Port `api-doc-manager.md` and revise stale handling so it creates and runs
     staleness checks itself instead of telling users to call `ws-ask-api
     --refresh`.
   - Update prompt tests for the new stems and runtime prompt bundle hash.

2. Add API docs runtime helpers.
   - Add helper(s) for resolving repo root, `.deps` directory, domain listing,
     exact hint detection, and pre-router input formatting.
   - Add per-domain lock implementation. Unix `flock` is not necessary inside Go
     tests; a process-local keyed mutex is acceptable for runtime guardrails if
     documented, but prefer filesystem locking if a simple portable pattern
     already exists in the repo. At minimum, same-process concurrent calls for
     the same domain should serialize in tests.
   - Add manager registration/reuse. If an `api-doc-<domain>` agent already
     exists, call it without re-registering; if missing, register it with
     `api-doc-manager`. Avoid deleting active sessions.

3. Add MCP tools.
   - `api.list` returns sorted domains.
   - `api.ask` implements exact hint, pre-router, per-domain dispatch, boundary
     preservation, partial success, and all-domain failure.
   - Add schemas in `tools()`, dispatch in `callTool`, runtime.json entries,
     and profile visibility checks if needed.

4. Update guidance.
   - Replace planned/specialized API docs wording in
     `agents-plugin/skills/workflow/SKILL.md`.
   - Update `agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`
     so third-party API documentation lookup goes through `ws/api.ask`.
   - Update `ai-docs/ref/ws-mcp.md` and `ai-docs/ref/ws-agent-runtime.md` if the
     new public tools or runtime behavior need reference coverage.

5. Verify.
   - Add unit/integration tests in `internal/mcp` and any helper package touched.
   - Cover no-cache and existing-cache `api.list`.
   - Cover exact hint skip by using a fake/stub pre-router path if needed.
   - Cover pre-router multi-domain output and partial failure boundaries.
   - Cover same-domain lock serialization.
   - Run `cd agents-plugin-tool && go test ./...`.
   - Run `git diff --check`.

## Risk Notes

- The current `wsagent.Register` resets an agent directory. Re-registering an
  existing `api-doc-<domain>` would destroy session state, so implementation
  must detect existing manager agents first.
- Existing async `agents.call` returns before backend completion. `api.ask`
  should call and wait internally, or use a sync helper, so callers receive the
  manager answer in one tool call.
- Avoid making the runtime responsible for semantic answer synthesis. Preserve
  structured enough boundaries for the lead agent to synthesize.
