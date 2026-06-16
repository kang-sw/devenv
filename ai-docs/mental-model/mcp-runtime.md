---
domain: mcp-runtime
description: "ws-mcp stdio server, MCP tool registry, CLI mirror, concurrency, and tool-profile gates."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/cmd/ws-mcp/
  - agents-plugin-tool/internal/wsstore/
  - agents-plugin-tool/internal/wskey/
related:
  plugin-runtime: "runtime.capabilities is the launcher fast path; runtime.info, tools/list, and CLI probes remain fallback compatibility checks."
  named-agent-runtime: "ws.mercenary.* route through wsagent lifecycle APIs; retired api.ask tools no longer do."
  git-workflow-tools: "git.* MCP tools and CLI mirrors delegate to internal/wsgit."
---

# MCP Runtime

## Domain Rules

- MCP tool output is primarily consumed by LLMs: default responses should be
  compact, readable text unless callers need stable machine parsing, protocol
  metadata, or compatibility-preserving JSON.

## Entry Points

- `cmd/ws-mcp/main.go` is the binary entry point for `serve --stdio`, `runtime info`, CLI mirrors, and local diagnostics. {#260505-runtime-cli-entrypoints}
- `ws-mcp smoke --root <repo>` is the single-process executable smoke entrypoint; keep it aligned with release workflow checks. {#260505-runtime-cli-entrypoints}
- `internal/mcp/server.go` owns MCP JSON-RPC request handling, tool schemas, tool dispatch, optional profile filtering, and cancellation. {#260505-mcp-server-protocol-surface}
- `internal/mcp/api_docs.go` owns the remaining deterministic `api.list` cache-domain discovery path; the former `api.ask_async` job state file is removed. {#260508-api-documentation-async-mcp-tools}
- `internal/wsstore` owns root/worktree SQLite metadata for named-agent registry metadata, exec job lifecycle metadata, future async metadata, retention, pruning, tombstone cleanup, and the runtime metadata migration gate inventory. Actor tables/columns are migration-cleanup only after the Phase 2a session-auth cutover; Phase 3 drops the residual `exec_jobs.owner_actor_id` so exec job metadata is fully actor-free. Exec stream payload bytes remain job-owned files. {#260525-runtime-metadata-migration-gate}
- `runtime.info` and `runtime.capabilities` are launcher-facing compatibility data; capabilities adds MCP protocol, lead tool names, and CLI commands. {#260505-runtime-debug-metadata-tools} {#260506-runtime-capabilities-single-probe}

## Module Contracts

- `ServeStdio` handles requests concurrently and serializes only response writes; long-running waits must not block `tools/list`.
- Cancellation depends on exact JSON-RPC id stringification; changing id formatting breaks `notifications/cancelled`.
- Tool results are returned as MCP text content, even when the text is JSON. Callers parse text, not structured content arrays.
- `toolTextResponse` errors are successful JSON-RPC responses with `isError: true`; unknown tools/profile violations are JSON-RPC errors.
- Root-aware MCP tool calls resolve their repository root only through a mandatory `session_key`: a known key returns the registry root, an unknown key returns `unknown_session`, and an absent key returns `mandatory_session_key` guidance naming `ws.lead.login(root)`. Their public schemas must advertise `session_key` and must not advertise `root`; `ws.lead.login` is the only root acceptor. There is no fallback to explicit `root`, volatile session defaults, host workspace metadata, startup root, or `WS_MCP_PROJECT_ROOT`. {#260505-mcp-session-default-root} {#260610-ephemeral-session-auth-model}
- `ws.lead.login` mints an ephemeral word-chain session key associating a canonical repository root and a capability scope (`lead`/`delegate`/`leaf`; absent → `lead`) in the in-memory session registry. It validates `root` via `canonicalSetupRoot` (catches `"<cwd>"` placeholders). A non-lead scoped key cannot call any `ws.lead.*` tool (self-login escalation block): the prefix is checked server-side before dispatch, not at the schema level.
- `ws.lead.prefer_mercenary` is a lead-only tool that flips the `preferMercenary` flag on the caller's `sessionEntry` under a write lock (one-way; cannot be unset in the current session). After the flip, `playbook.render` injects a mercenary-primary guidance block into implementer and reviewer playbook renderings; it does not affect tool availability — the mercenary path is always reachable via the always-on tip appended to every `delegates:true` rendering. Hidden in wsflow no-agent mode (`noAgentHiddenTool`); `ws.lead.login` stays visible. {#260610-mercenary-delegation-surface}
- `sessionRegistry` is a per-process in-memory store: no SQLite backing, no eviction, no logout, and no persistence across server restarts. Session keys are opaque word-chain strings; do not parse them.
- `wskey` is policy-free: it generates word-chain keys only and must not import `mcp` or auth packages. Uniqueness enforcement and `{root, scope}` association are the session registry's responsibility. Adding capability logic to `wskey` creates an import cycle.
- Keyed capability gate in `callTool`: when a non-empty `session_key` maps to a known non-lead scope, `roleAllowsTool(scope, toolName)` is checked and any `ws.lead.*` prefix is additionally blocked. Unknown `session_key` values are NOT rejected here; root-aware tools surface `unknown_session` through `resolveToolRoot`. Lead keys are the standard mint path; child keys (delegate/leaf scope) are minted by `playbook.render` at render time for lead callers when the playbook frontmatter declares a delegate-eligible role (Phase 2c). The keyed gate is the sole tool-permission authority (Phase 3): `WS_MCP_TOOL_PROFILE` no longer participates.
- `runtime.capabilities` must report the full lead launcher contract surface even when `WS_MCP_TOOL_PROFILE` or `WS_MCP_ALLOWED_TOOLS` is inherited; use `LeadToolNames`, not filtered server tools. {#260506-runtime-capabilities-single-probe}
- `WS_MCP_NO_AGENT=1` is a product-mode surface, not a profile filter: tools/list, tools/call, CLI command gates, and `runtime.capabilities` all hide agent-backed surfaces together while environment-unset full ws behavior stays unchanged. {#260513-wsflow-agentless-runtime-mode}
- Product-mode gates are bidirectional and symmetric: `NoAgentMode() && noAgentHiddenTool(name)` hides agent-backed tools in wsflow, and `!NoAgentMode() && wsflowOnlyTool(name)` hides wsflow-only tools (currently `prompt.render`) from full ws. Each gate is applied at the same three points — `callTool` (explicit-call error), `toolAllowed` (tools/list), and `LeadToolNames` (runtime.capabilities). {#260529-wsflow-only-tool-surface} {#260529-prompt-render-tool}
- Empty `WS_MCP_NAMESPACE` values are treated as unset, preserving `ws` namespace text.
- MCP starts with the lead tool surface; worktree locks are not an authority signal for tool visibility. {#260505-tool-profile-gating}
- The session-key keyed capability gate (`roleAllowsTool(entry.scope, name)` plus the `ws.lead.*` block for non-lead keys, in `callTool`) is the sole server-side tool-permission authority. `WS_MCP_TOOL_PROFILE` is retired (Phase 3): it no longer filters the served surface, `Server.role`/`requestedToolRole` are gone, and it is not propagated to spawned subprocesses. Delegate scope travels in-band via the render-minted child key. The gate is still a soft guard (a delegate can keyless-`ws.lead.login` to re-escalate), not a hard sandbox.
- `ws.setup` is deleted: the bare root-session form, `method: "lead-workflow-bootstrap"`, `id` recovery form, setup alias, setup-state helpers, actor persistence, and setup request-order fence are not part of the Phase 2a runtime. Explicit setup calls now fall through as unknown tools; hidden `session.*` root tools return guidance to use `ws.lead.login(root)`.
- Every root-aware public MCP tool schema strips `root` and expects callers to thread `session_key`; `ws.lead.login` is the only advertised root-accepting schema. Do not restore hidden explicit-root compatibility while updating root-aware tools, including `ws.mercenary.*`, `exec.*`, docs, Git, tickets, specs, and mental-model tools. {#260610-ephemeral-session-auth-model}
- `ws.mercenary.*` lifecycle tools use normal key-only root resolution and have no actor scope, global compatibility namespace, persistent child-actor metadata, child setup instruction injection, or hidden `--actor-id` CLI mirror. The `ws.mercenary.*` surface is reshaped as the mercenary delegation surface (Phase 2c; renamed agents.*→ws.mercenary.* in Phase 7, 260611): mercenaries are scoped to implementer/reviewer roles, registered with `system_prompt_text` from `playbook.render`, and `ws.mercenary.call` returns a native-shaped `agentId=<name>` continuation handle for idiom parity with host-native subagents. {#260505-named-agent-mcp-tools} {#260610-mercenary-delegation-surface}
- Public `exec.*` schemas use `working_dir` for command execution location, not `root`; dispatch resolves the ws worktree root internally, constrains resolved working directories inside that root, and reconciles lost running workers so persisted exec jobs do not remain indefinitely running. There is no public exec CLI mirror; add one only through an explicit CLI contract change. {#260524-exec-job-mcp-tools}
- Exec MCP formatting is intentionally owned by `internal/mcp/server.go`, while `internal/execjob` owns lifecycle state, stream files, readers, key allocation, and wait semantics. Do not push LLM-readable labels/separators into execjob or add a public JSON mode to exec tools; that would either pollute non-MCP callers or recreate the unreadable JSON-in-text failure this surface avoids. {#260524-exec-job-mcp-tools}
- Exec job lifecycle metadata is SQLite-backed through `wsstore.ExecJob`; `state.json` is legacy import input only. Do not write a reverse importer from SQLite back to `state.json`: corrupt or incomplete legacy state becomes bounded failed recovery metadata, while importable legacy state is migrated forward on read. {#260524-exec-job-mcp-tools} {#260525-runtime-metadata-migration-gate}
- New exec job keys are short random tokens, but lookup and validation must continue accepting the legacy timestamp-plus-random form and checking both SQLite records and legacy job directories; otherwise old persisted jobs become unreachable after a runtime upgrade. {#260524-exec-job-mcp-tools}
- `wsstore` is metadata/control-plane storage only: path indexes and byte counts are SQLite metadata, but large stdout, stderr, prompts, final outputs, transcripts, runtime logs, and other payload bodies remain file-backed. Missing exec stdout/stderr/combined files are recoverable file-backed payload consistency states surfaced by status/result/raw readers, not empty streams and not a reason to move payload bytes into SQLite. {#260525-runtime-metadata-migration-gate}
- SQLite configure, migration, and short write paths use bounded `SQLITE_BUSY`/`SQLITE_LOCKED` retry while retaining process-local write serialization. Long-running subprocess or model execution must update lifecycle, lease, and byte-count records through brief writes rather than holding a database transaction open. {#260525-runtime-metadata-migration-gate}
- Plugin-managed MCP calls may lack a caller repository root on native Windows; callers must bootstrap with `ws.lead.login(root)` and pass the returned `session_key` because ordinary tools no longer infer roots from cwd, host metadata, `WS_MCP_PROJECT_ROOT`, or hidden root arguments.
- The server records a session harness from MCP payloads, not as an authority boundary: `initialize.params` may identify Claude/Codex clients, and `tools/call._meta.x-codex-turn-metadata` is a Codex signal. Conflicts are debug events and do not silently switch the stored harness. {#260508-mcp-payload-harness-detection}

## Coupling

- Tool additions, removals, or intentionally hidden compatibility paths require both `callTool` and `tools()` review; role/profile filtering and runtime metadata must also be reviewed. `runtime.capabilities` derives MCP tool names from `tools()`, but `runtime.json` still must be updated. {#260505-tool-profile-gating}
- CLI mirrors are separate adapters. MCP behavior changes do not update `cmd/ws-mcp` handlers automatically, and public launcher-required CLI commands must also be kept in `runtimeCapabilityCommandNames` plus `runtime.json.commands`. {#260505-cli-mirror-coverage}
- `api.ask` and async API jobs are retired; named-agent runtime changes should not preserve or recreate API documentation manager/pre-router behavior. `api.list` remains filesystem-only cache discovery. {#260505-workflow-state-delegation-tools}
- Config tools read/write user-local config through `wsconfig`; compatibility tier names, model aliases, optional effort metadata, and harness-aware defaults must match agent registration behavior and readable `config.show` output. `config.agents_tier` is the public effort-selection surface; exposing effort directly on `ws.mercenary.register` or prompt metadata would bypass the alias contract and backend no-override default. {#260505-config-tools} {#260508-model-alias-config-tools}
- MCP and CLI mirrors share readable formatter contracts through exported `internal/mcp` formatting helpers for workflow discovery and Git summaries. Keep explicit JSON output paths beside text defaults so tests cover both caller types. {#260519-workflow-command-readable-output-defaults}
- Broad documentation find output has a stricter formatter contract than ordinary list summaries: default text groups by document with `score`/`hits`, bounds document and hit counts, and prints selected line snippets; explicit JSON must keep the wsdoc `matches` evidence for structured consumers. {#260519-tolerant-documentation-lookup-query-evidence}
- Static reference docs must not copy the MCP tool schema or current tool inventory; live schema belongs to `tools()`/`tools/list`, launcher inventory belongs to `runtime.capabilities`, and durable behavior belongs in specs. {#260524-reference-document-ownership}

## Extension Points & Change Recipes

- **Add an MCP tool**: add schema in `tools()`, dispatch in `callTool`, optional profile permissions in `roleAllowsTool`, visibility tests when filtered, and `runtime.json`.
- **Change a root-aware MCP tool**: keep the advertised schema free of `root` (except `ws.lead.login`), route dispatch through `resolveToolRoot`, require `session_key`, and test raw `tools()` schema plus `mandatory_session_key`/`unknown_session` behavior. Agent readers such as `ws.mercenary.print` need the same key-only root resolution as wait/result/status.
- **Add a CLI mirror**: add the top-level or group subcommand in `cmd/ws-mcp`, map flags to the same internal package as MCP, add readable default output plus explicit `--format json` when structured consumers exist, and add command smoke tests.
- **Change broad documentation find formatting**: update MCP text dispatch, CLI query paths, exported format helpers, and JSON tests together; zero-result guidance and truncation wording are part of the LLM-facing contract.
- **Restrict a tool under a profile**: update profile tables and add tests proving allowlists cannot regain a hidden tool.
- **Add or change a product-mode gate**: apply the gate predicate at all three points — `callTool` (explicit-call error), `toolAllowed` (tools/list), and `LeadToolNames` (runtime.capabilities) — for whichever direction applies (no-agent hides agent-backed tools in wsflow; wsflow-only hides tools from full ws), update CLI command dispatch and `runtimeCapabilityCommandNames` when relevant, and add both default and mode-specific tests.
- **Change wsflow no-agent mode**: update `agents-plugin-wsflow/runtime.json`, package tests, and launcher contract expectations in the same logical change.
- **Add or change the exec job surface**: keep launch, status, result, abort, and raw fallback readers in the MCP registry together; preserve bounded readable text output and separator-delimited raw stream sections at the MCP layer, route omitted `working_dir` through ws root resolution instead of process cwd, constrain resolved command working directories inside the worktree root, reconcile lost running workers, keep lifecycle/path/byte-count metadata in `wsstore.ExecJob`, keep stdout/stderr/combined bytes in job-owned files, and hide the entire `exec.*` family in wsflow no-agent mode. {#260524-exec-job-mcp-tools} {#260524-exec-runtime-contract-surface}
- **Move runtime metadata into SQLite**: add or reuse `wsstore` tables for metadata and indexes, keep stream payloads file-backed, add retention/tombstone behavior with active-state skips, and test macOS/Linux plus Windows behavior for database access, file deletion, and existing JSON-backed compatibility. Named-agent registry and exec job metadata already use this path. Keep `wsstore` tests pointed at source-level inventories or local fixtures rather than importing runtime consumers, or wiring creates reverse-import cycles.

## Common Mistakes

- Advertising a tool in `tools()` without a dispatch case creates a visible broken tool.
- Product-mode-gating a tool at `callTool` and `toolAllowed` but forgetting `LeadToolNames`: tools/list and explicit calls gate correctly, yet `runtime.capabilities` still advertises it and the launcher contract test breaks.
- Adding dispatch without schema makes the tool callable only by guessing the name.
- Treating schema-level (`tools/list`) filtering as an authority boundary creates false safety; it is advisory — a caller that knows a tool name can still issue `tools/call`. The authority is the keyed capability gate in `callTool`. `WS_MCP_TOOL_PROFILE` no longer gates anything (retired in Phase 3); do not reintroduce an env-profile role layer. Mandatory `session_key` closes keyless root-aware dispatch; render-minted child keys carry delegate scope in-band.
- Assuming delegate or leaf agents can call `ws.mercenary.*` tools when `WS_MCP_TOOL_PROFILE` is applied; neither delegate nor leaf profile may call `ws.mercenary.*` — the delegate-profile exception that allowed lifecycle calls for `subquery-*`-named agents was removed with the subquery runtime in Phase 2b.
- Reintroducing `api.ask` or API-doc async tools without a new spec; the current surface deliberately keeps only deterministic `api.list` cache discovery. {#260505-api-documentation-mcp-tools}
- Adding MCP tools without updating `agents-plugin/runtime.json`; launcher compatibility checks compare the required MCP tool surface against runtime metadata.
- Assuming MCP tool calls know the user's shell cwd; plugin-managed server cwd can be the plugin cache.
- Passing `"."` or `"<cwd>"` to `ws.lead.login(root)` is ambiguous in plugin-managed sessions; pass the repository's absolute filesystem path.
- Guessing among multiple host workspaces creates cross-project writes; root resolution must reject without a valid `session_key` and direct the caller to `ws.lead.login(root)`.
- Treating namespace override as a tool rename; wsflow changes user-facing namespace text, while generic MCP tool identifiers stay stable. In playbook text, use explicit `McpNamespace` / `SkillNamespace` render vars for display notation instead of relying on broad string rewriting.
- Updating `specs.find` or `mental_models.find` MCP output without the CLI mirror; users dogfood the CLI fallback when MCP host behavior is unclear.
- Treating `ai-docs/ref/ws-mcp.md` as the MCP contract source of truth instead of an operations runbook; this recreates schema drift with `tools()` and `runtime.capabilities`.
- Migrating agent or exec state into SQLite while also moving large stream payloads into the database; that defeats raw tail/read/grep and increases lock pressure.
- Classifying `*_path` fields as file-backed payloads; the path strings are SQLite metadata indexes even when the bytes at those paths stay file-backed.
- Treating a missing exec stream file as empty output; status/result/raw readers must surface the recoverable file-backed payload consistency state so prune/tombstone or repair paths can diagnose the artifact.
- Testing session-key agent dispatch only with a live bogus worker; use controlled completed fixtures for wait/result/print assertions so timing does not decide whether dispatch was correct.

## Technical Debt

- MCP input still uses `bufio.Scanner`; very large single-line MCP requests can hit scanner token limits before tool handling.
- Some compatibility docs mention broader future repair semantics, but `spec_index.verify` currently checks duplicate anchors only.
