# Implementation Plan: 260505-feat-ws-mcp-result-readiness-api

## Scope Guard

- Edit only the MCP/runtime migration surface listed in the brief; do not touch `claude-plugin/`.
- Do not stage or commit the in-progress `ai-docs/spec/` / `ai-docs/ref/old-spec/260505/` archive move.
- Treat `agents.result` as the only automatic cleanup point for ephemeral agents; `wait`, `status`, `tail`, `cancel`, and compatibility `print` must not erase output.

## Code Shape

1. **Runtime model (`agents-plugin-tool/internal/wsagent/agent.go`)**
   - Extend `Agent` with explicit metadata, e.g. `Metadata AgentMetadata` or a minimal `Ephemeral bool` field near the registry fields at `agents-plugin-tool/internal/wsagent/agent.go#L202-L217`.
   - Add `Ephemeral bool` to `RegisterOptions` at `agents-plugin-tool/internal/wsagent/agent.go#L59-L70`; write it into `agent.json` during registration at `agents-plugin-tool/internal/wsagent/agent.go#L328-L346`.
   - Add `ResultOptions { Root, Name string; Timeout time.Duration; Poll time.Duration; Context context.Context; Wait bool }` or equivalent; preserve omitted/zero timeout as non-blocking rather than reusing `defaultAgentWaitTimeout`.
   - Implement `Manager.Result(opts)` beside `Print`/`Wait` at `agents-plugin-tool/internal/wsagent/agent.go#L826-L911`:
     - reconcile the current call;
     - completed: read `output.md`, then erase only when `agent` metadata is ephemeral and output read succeeded;
     - queued/running with no positive timeout: return concise non-ready status plus follow-up text;
     - queued/running with positive timeout: poll until terminal/timeout/context cancellation;
     - failed/cancelled/timeout/context cancellation: return status/timeout text and preserve the agent directory.
   - Change `Manager.Wait` to readiness-only. Prefer returning one readiness block per requested name, with fields such as `agent`, `call_status`, `ready`, `terminal`, `result_available`, `active`, `pid`, `error`, `follow_up`; do not call `Print` from the completed branch at `agents-plugin-tool/internal/wsagent/agent.go#L871-L878`.
   - Support multi-name wait by either adding `Names []string` to `WaitOptions` at `agents-plugin-tool/internal/wsagent/agent.go#L105-L111` or introducing `WaitManyOptions`; keep `Name` as a compatibility path that appends to `Names`.
   - Update `followUpForCall` at `agents-plugin-tool/internal/wsagent/agent.go#L1667-L1683` so completed calls point to `agents.result | agents.tail`; running calls point to readiness/status/cancel/tail; failed/cancelled calls stay diagnostic.

2. **Ephemeral subquery agents**
   - In `Subquery` at `agents-plugin-tool/internal/wsagent/agent.go#L798-L823`, generate a human hint such as `subquery-tmp<time>-<seq>` while preserving the `subquery-` prefix for delegate access and tests.
   - Pass `Ephemeral: true` through `RegisterOptions` for subquery agents.
   - Change follow-up text from `agents.wait` / `agents.print` to `agents.result(name: <key>, timeout_seconds: 600)` plus diagnostic `agents.status`, `agents.tail`, and `agents.cancel`; do not advertise cleanup.

3. **MCP server (`agents-plugin-tool/internal/mcp/server.go`)**
   - Add `agents.result` to the `callTool` switch near `agents.wait` at `agents-plugin-tool/internal/mcp/server.go#L585-L597`; parse `name` and optional `timeout_seconds`, and pass `Context: ctx`.
   - Update `agents.wait` parsing to accept legacy `name` and new `names` array, then call runtime multi-wait.
   - Add `agents.result` to `tools/list` schema near `agents.wait` at `agents-plugin-tool/internal/mcp/server.go#L1121-L1133`; make `agents.wait` schema include both `name` and `names` and avoid requiring only `name`.
   - Update delegate access allowlist at `agents-plugin-tool/internal/mcp/server.go#L1319-L1330` to include `agents.result`; allow generated subquery names by prefix for now, with cleanup authorization still controlled by metadata.
   - Keep `agents.print` either as a hidden/compatibility alias to non-cleaning `Print` or to `Result` with cleanup disabled; if left public, mark deprecated in descriptions and docs.

4. **CLI fallback (`agents-plugin-tool/cmd/ws-mcp/main.go`)**
   - Add `agents result` to dispatch and usage at `agents-plugin-tool/cmd/ws-mcp/main.go#L581-L619`.
   - Implement `agentsResult` beside `agentsWait`/`agentsPrint` at `agents-plugin-tool/cmd/ws-mcp/main.go#L699-L715` and `#L844-L855`; parse `--root`, `--name`, and `--timeout`.
   - Change `agentsWait` to accept repeated `--name` and positional names. Go `flag` needs a small custom `multiStringFlag`; combine `--name` values and `fs.Args()` into the runtime `Names` list.
   - Keep `agents print` only if needed as compatibility; do not let it erase ephemeral agents.

5. **Runtime contract metadata and docs**
   - Add `agents.result` to `agents-plugin/runtime.json` tools and commands near `agents.wait` / `agents.print` at `agents-plugin/runtime.json#L52-L65` and `#L80-L95`.
   - Update `ai-docs/ref/ws-agent-runtime.md` sections around tool surface, CLI prototype, delegation, and current `agents.wait` semantics (`ai-docs/ref/ws-agent-runtime.md#L233-L347`).
   - Update `ai-docs/ref/ws-mcp.md` agent tool contracts and `subquery` retrieval text around `ai-docs/ref/ws-mcp.md#L184-L905` and `#L1076-L1130`.
   - If any embedded prompt text changes under `agents-plugin-tool/internal/wsprompt/prompts/`, recompute/update `agents-plugin/runtime.json` `prompt_bundle.content_sha256`.

6. **Skill and prompt migration**
   - Replace subquery retrieval guidance with `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)` in:
     - `agents-plugin/skills/lead-workflow/SKILL.md`
     - `agents-plugin/skills/lead-discuss/SKILL.md`
     - `agents-plugin/skills/lead-sprint/SKILL.md`
     - `agents-plugin/skills/lead-forge-spec/SKILL.md`
     - `agents-plugin/skills/lead-forge-mental-model/SKILL.md`
     - `agents-plugin/skills/lead-write-spec/SKILL.md`
     - `agents-plugin/skills/lead-write-ticket/SKILL.md`
     - `agents-plugin-tool/internal/wsprompt/prompts/implementer.md`
   - Replace fallback `agents.print` guidance with `agents.result` in `agents-plugin/skills/lead-edit/SKILL.md` and `agents-plugin/skills/lead-write-code/SKILL.md`.
   - Because these are skill/prompt edits, re-check changed invariant/constraint lines against `ai-docs/ref/skill-authoring.md`.

## Test Plan

1. **Runtime unit tests (`agents-plugin-tool/internal/wsagent/agent_test.go`)**
   - Update existing completed-wait assertion at `agents-plugin-tool/internal/wsagent/agent_test.go#L610-L616` to expect readiness metadata, not `async reply`.
   - Add `Result` tests for:
     - completed output with no timeout;
     - positive timeout waits until output;
     - running/no timeout returns non-ready actionable status;
     - timeout returns timeout/status and preserves the agent;
     - failed and cancelled ephemeral agents remain inspectable;
     - successful `Result` on ephemeral subquery erases the agent only after output read succeeds.
   - Update timeout/cancel test expectations at `agents-plugin-tool/internal/wsagent/agent_test.go#L838-L851` for readiness follow-up wording.
   - Update subquery tests at `agents-plugin-tool/internal/wsagent/agent_test.go#L1116-L1178` for `subquery-tmp...`, `Ephemeral: true`, and `agents.result` follow-up.

2. **MCP tests (`agents-plugin-tool/internal/mcp/server_test.go`)**
   - Extend tool-profile tests at `agents-plugin-tool/internal/mcp/server_test.go#L310-L390` so delegate can call `agents.result` for `subquery-*` names and cannot call it for arbitrary names.
   - Add schema/tool-list coverage for `agents.result` and `agents.wait` `names`.
   - Add call tests for legacy single `name` wait, multi-name wait, readiness output excluding final output, and `agents.result` output retrieval.
   - Keep concurrency regression `TestServeStdioDoesNotBlockToolsListBehindWait` at `agents-plugin-tool/internal/mcp/server_test.go#L426-L470` passing with readiness-only wait.

3. **CLI tests / smokes (`agents-plugin-tool/cmd/ws-mcp`)**
   - Add command tests or smoke coverage for `ws-mcp agents result --root <repo> --name <name> [--timeout 10m]`.
   - Add wait CLI coverage for repeated `--name` and positional names in one invocation.

4. **Verification commands**
   - Run from `agents-plugin-tool/`:
     ```sh
     go test ./internal/wsagent ./internal/mcp ./cmd/ws-mcp
     go test ./...
     ```
   - Run `git diff --check` from repo root.
   - If prompt bundle changed, verify `ws-mcp runtime info` prompt hash matches `agents-plugin/runtime.json`.

## Migration Order

1. Add registry metadata (`Ephemeral`) and non-cleaning runtime helpers; keep existing `Print` and old `Wait` behavior temporarily.
2. Implement `Result` and its focused runtime tests; prove cleanup only happens after successful completed output read.
3. Convert `Wait` to readiness-only and multi-name; update existing runtime tests.
4. Wire MCP `agents.result`, wait `names`, schemas, and delegate allowlist; update MCP tests.
5. Wire CLI `agents result` and multi-name `agents wait`; update command tests/smokes.
6. Mark subquery agents ephemeral, rename generated key hint, and migrate subquery follow-up text.
7. Update docs, skills, prompt guidance, and runtime contract metadata; recompute prompt bundle hash only if embedded prompts changed.
8. Run the full verification set; inspect diff to confirm no `claude-plugin/` or spec-archive files changed.

## Risks / Decisions to Keep Small

- Keep `agents.print` as a compatibility alias for this ticket unless removal is trivial; removing it touches more skill/docs history and is not needed before first release.
- Keep cleanup metadata minimal (`ephemeral: true`) rather than introducing TTL/GC or broad lifecycle policy.
- Keep delegate subquery authorization prefix-based in MCP for now; metadata governs cleanup, while authorization can move to registry-backed checks in a later containment ticket if needed.
- Use line-oriented readiness text first; structured JSON can be a later refinement if callers need machine parsing beyond stable keys.
