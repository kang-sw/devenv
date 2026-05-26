---
domain: mcp-runtime
description: "ws-mcp stdio server, MCP tool registry, CLI mirror, concurrency, and tool-profile gates."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/cmd/ws-mcp/
  - agents-plugin-tool/internal/wsstore/
related:
  plugin-runtime: "runtime.capabilities is the launcher fast path; runtime.info, tools/list, and CLI probes remain fallback compatibility checks."
  named-agent-runtime: "agents.*, subquery, and api.ask route through wsagent lifecycle APIs."
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
- `internal/mcp/api_async.go` owns recoverable API documentation job state behind the `api.ask_async` tool family. {#260508-api-documentation-async-mcp-tools}
- `internal/wsstore` owns root/worktree SQLite metadata for setup actors, named-agent registry metadata, exec job lifecycle metadata, future async metadata, retention, pruning, tombstone cleanup, and the runtime metadata migration gate inventory. Exec stream payload bytes remain job-owned files. {#260525-runtime-metadata-migration-gate}
- `runtime.info` and `runtime.capabilities` are launcher-facing compatibility data; capabilities adds MCP protocol, lead tool names, and CLI commands. {#260505-runtime-debug-metadata-tools} {#260506-runtime-capabilities-single-probe}

## Module Contracts

- `ServeStdio` handles requests concurrently and serializes only response writes; long-running waits must not block `tools/list`.
- `ws.setup` and the advertised setup alias are request-order fences: prior in-flight requests complete first, setup is applied synchronously, and later stream requests are not accepted until the setup response is written. This protects batched setup-then-call actor/root state.
- Cancellation depends on exact JSON-RPC id stringification; changing id formatting breaks `notifications/cancelled`.
- Tool results are returned as MCP text content, even when the text is JSON. Callers parse text, not structured content arrays.
- `toolTextResponse` errors are successful JSON-RPC responses with `isError: true`; unknown tools/profile violations are JSON-RPC errors.
- The server root is captured at `NewServer`; root-aware MCP tool calls use a resolver chain of explicit `root`, volatile session default root, unambiguous host workspace metadata, explicit non-dot startup root, `WS_MCP_PROJECT_ROOT`, and then startup root. Invalid explicit startup roots fail closed instead of falling through to the environment fallback. {#260505-mcp-session-default-root}
- `runtime.capabilities` must report the full lead launcher contract surface even when `WS_MCP_TOOL_PROFILE` or `WS_MCP_ALLOWED_TOOLS` is inherited; use `LeadToolNames`, not filtered server tools. {#260506-runtime-capabilities-single-probe}
- `WS_MCP_NO_AGENT=1` is a product-mode surface, not a profile filter: tools/list, tools/call, CLI command gates, and `runtime.capabilities` all hide agent-backed surfaces together while environment-unset full ws behavior stays unchanged. {#260513-wsflow-agentless-runtime-mode}
- Empty `WS_MCP_NAMESPACE` and `WS_MCP_SETUP_TOOL` values are treated as unset, preserving `ws` namespace text and the `ws.setup` advertised setup tool.
- MCP starts with the lead tool surface; worktree locks are not an authority signal for tool visibility. {#260505-tool-profile-gating}
- `WS_MCP_TOOL_PROFILE` is an optional containment filter. If host environment propagation fails, delegated agents may see lead tools and must follow prompt-level role rules.
- `ws.setup(method: "lead-workflow-bootstrap", root: "<absolute-working-directory>")` creates a cooperative lead actor, persists actor metadata in root/worktree SQLite state, and binds the actor root to the current server process; callers must pass the repository's absolute filesystem path because the MCP server cannot infer the agent cwd. `ws.setup(id: "<actor-id>")` restores that binding after restart. Actor ids are opaque recovery tokens: new tokens do not encode the worktree key, so recovery must validate the authority prefix, use `wsstore.Manager.FindActor` for compact ids, and keep the legacy worktree-key parser only as compatibility fallback. {#260524-mcp-actor-setup-bootstrap}
- `ws.setup(root)` without a method remains the compatibility root-session setup surface; it stores a canonical Git worktree root in the current server instance only and does not change process cwd or write config. Hidden `session.*` dispatch can exist for compatibility but must not be advertised as canonical. Hidden `format` dispatch can remain accepted for compatibility, but public setup schemas must omit it so LLM callers stay on compact text defaults unless they already know the hidden path.
- Root-omitted `agents.*` lifecycle tools and `subquery` resolve through the current actor scope when the resolved root is actor-bound; this includes legacy `agents.print`. Hidden explicit-root arguments intentionally bypass actor scope and use the global compatibility namespace. Missing actor-gate errors stay compact and point to `ws.setup(id: "<actor-id>")`; the lead bootstrap ceremony belongs in workflow guidance, not these tool errors. {#260524-mcp-actor-setup-bootstrap}
- Actor-bound `agents.register`, `agents.call`, and rootless actor-scoped `subquery` mint or reuse child actors and inject `ws.setup(id: "<child-actor-id>")` recovery instructions into child system prompts; hidden explicit-root compatibility calls do not receive child actor setup because they route to the global namespace. Delegate/subquery prompts must not expose the lead bootstrap method. Async worker and check-inbox CLI mirrors carry hidden `--actor-id` only to preserve this dispatch scope.
- Public actor-owned MCP schemas for `agents.*` and `subquery` intentionally omit `root` even though dispatch still accepts hidden explicit-root compatibility arguments through the normal root resolver; non-agent root-aware schemas keep advertising `root`. {#260523-agents-root-schema-invisibility}
- Public `exec.*` schemas use `working_dir` for command execution location, not `root`; dispatch resolves the ws worktree root internally, constrains resolved working directories inside that root, and reconciles lost running workers so persisted exec jobs do not remain indefinitely running. There is no public exec CLI mirror; add one only through an explicit CLI contract change. {#260524-exec-job-mcp-tools}
- Exec MCP formatting is intentionally owned by `internal/mcp/server.go`, while `internal/execjob` owns lifecycle state, stream files, readers, key allocation, and wait semantics. Do not push LLM-readable labels/separators into execjob or add a public JSON mode to exec tools; that would either pollute non-MCP callers or recreate the unreadable JSON-in-text failure this surface avoids. {#260524-exec-job-mcp-tools}
- Exec job lifecycle metadata is SQLite-backed through `wsstore.ExecJob`; `state.json` is legacy import input only. Do not write a reverse importer from SQLite back to `state.json`: corrupt or incomplete legacy state becomes bounded failed recovery metadata, while importable legacy state is migrated forward on read. {#260524-exec-job-mcp-tools} {#260525-runtime-metadata-migration-gate}
- New exec job keys are short random tokens, but lookup and validation must continue accepting the legacy timestamp-plus-random form and checking both SQLite records and legacy job directories; otherwise old persisted jobs become unreachable after a runtime upgrade. {#260524-exec-job-mcp-tools}
- `wsstore` is metadata/control-plane storage only: path indexes and byte counts are SQLite metadata, but large stdout, stderr, prompts, final outputs, transcripts, runtime logs, and other payload bodies remain file-backed. Missing exec stdout/stderr/combined files are recoverable file-backed payload consistency states surfaced by status/result/raw readers, not empty streams and not a reason to move payload bytes into SQLite. {#260525-runtime-metadata-migration-gate}
- SQLite configure, migration, and short write paths use bounded `SQLITE_BUSY`/`SQLITE_LOCKED` retry while retaining process-local write serialization. Long-running subprocess or model execution must update lifecycle, lease, and byte-count records through brief writes rather than holding a database transaction open. {#260525-runtime-metadata-migration-gate}
- Plugin-managed MCP calls may lack a caller repository root on native Windows; if `WS_MCP_PROJECT_ROOT` and host metadata are unavailable, tools need an explicit compatibility `root` or `ws.setup(root)` rather than the user's shell cwd.
- The server records a session harness from MCP payloads, not as an authority boundary: `initialize.params` may identify Claude/Codex clients, and `tools/call._meta.x-codex-turn-metadata` is a Codex signal. Conflicts are debug events and do not silently switch the stored harness. {#260508-mcp-payload-harness-detection}

## Coupling

- Tool additions, removals, or intentionally hidden compatibility paths require both `callTool` and `tools()` review; role/profile filtering and runtime metadata must also be reviewed. `runtime.capabilities` derives MCP tool names from `tools()`, but `runtime.json` still must be updated. {#260505-tool-profile-gating}
- CLI mirrors are separate adapters. MCP behavior changes do not update `cmd/ws-mcp` handlers automatically, and public launcher-required CLI commands must also be kept in `runtimeCapabilityCommandNames` plus `runtime.json.commands`. {#260505-cli-mirror-coverage}
- `api.ask`, async API jobs, and `subquery` use named-agent runtime semantics; changes to agent result/wait/print/cancel/erase behavior must keep MCP tool descriptions, async job reconciliation, actor-scoped dispatch tests, and follow-up text coherent. {#260505-workflow-state-delegation-tools}
- Config tools read/write user-local config through `wsconfig`; compatibility tier names, model aliases, optional effort metadata, and harness-aware defaults must match agent registration behavior and readable `config.show` output. `config.agents_tier` is the public effort-selection surface; exposing effort directly on `agents.register`, `subquery`, or prompt metadata would bypass the alias contract and backend no-override default. {#260505-config-tools} {#260508-model-alias-config-tools}
- MCP and CLI mirrors share readable formatter contracts through exported `internal/mcp` formatting helpers for workflow discovery and Git summaries. Keep explicit JSON output paths beside text defaults so tests cover both caller types. {#260519-workflow-command-readable-output-defaults}
- Broad documentation find output has a stricter formatter contract than ordinary list summaries: default text groups by document with `score`/`hits`, bounds document and hit counts, and prints selected line snippets; explicit JSON must keep the wsdoc `matches` evidence for structured consumers. {#260519-tolerant-documentation-lookup-query-evidence}
- Static reference docs must not copy the MCP tool schema or current tool inventory; live schema belongs to `tools()`/`tools/list`, launcher inventory belongs to `runtime.capabilities`, and durable behavior belongs in specs. {#260524-reference-document-ownership}

## Extension Points & Change Recipes

- **Add an MCP tool**: add schema in `tools()`, dispatch in `callTool`, optional profile permissions in `roleAllowsTool`, visibility tests when filtered, and `runtime.json`.
- **Change an actor-owned MCP tool (`agents.*` or `subquery`)**: keep the advertised schema rootless, route dispatch through the shared root resolver, pass actor scope only when the call is root-omitted in an actor-bound session, and test raw `tools()` schema, actor-scoped lifecycle behavior, and explicit-root global compatibility with colliding public names together. Legacy readers such as `agents.print` need the same scoped resolution as wait/result/status.
- **Add a CLI mirror**: add the top-level or group subcommand in `cmd/ws-mcp`, map flags to the same internal package as MCP, add readable default output plus explicit `--format json` when structured consumers exist, and add command smoke tests.
- **Change broad documentation find formatting**: update MCP text dispatch, CLI query paths, exported format helpers, and JSON tests together; zero-result guidance and truncation wording are part of the LLM-facing contract.
- **Restrict a tool under a profile**: update profile tables and add tests proving allowlists cannot regain a hidden tool.
- **Add or change a product-mode gate**: update MCP tool filtering, explicit call errors, CLI command dispatch, `runtimeCapabilityCommandNames`, and both default and mode-specific tests.
- **Change wsflow no-agent mode**: update `agents-plugin-wsflow/runtime.json`, package tests, and launcher contract expectations in the same logical change.
- **Add or change the exec job surface**: keep launch, status, result, abort, and raw fallback readers in the MCP registry together; preserve bounded readable text output and separator-delimited raw stream sections at the MCP layer, route omitted `working_dir` through ws root resolution instead of process cwd, constrain resolved command working directories inside the worktree root, reconcile lost running workers, keep lifecycle/path/byte-count metadata in `wsstore.ExecJob`, keep stdout/stderr/combined bytes in job-owned files, and hide the entire `exec.*` family in wsflow no-agent mode. {#260524-exec-job-mcp-tools} {#260524-exec-runtime-contract-surface}
- **Move runtime metadata into SQLite**: add or reuse `wsstore` tables for metadata and indexes, keep stream payloads file-backed, add retention/tombstone behavior with active-state skips, and test macOS/Linux plus Windows behavior for database access, file deletion, and existing JSON-backed compatibility. Named-agent registry and exec job metadata already use this path. Keep `wsstore` tests pointed at source-level inventories or local fixtures rather than importing runtime consumers, or wiring creates reverse-import cycles.

## Common Mistakes

- Advertising a tool in `tools()` without a dispatch case creates a visible broken tool.
- Adding dispatch without schema makes the tool callable only by guessing the name.
- Treating MCP profile filters as an authority boundary creates false safety; prompt-level delegate rules remain the durable containment mechanism.
- Assuming delegate agents can inspect arbitrary agents when `WS_MCP_TOOL_PROFILE` is applied; delegate profile can use selected `agents.*` tools only for `subquery-*` names, while leaf profile cannot use `agents.*`.
- Treating `domain_hint` in `api.ask` as a direct domain selector; only exact existing domain names bypass routing. {#260505-api-documentation-mcp-tools}
- Adding API-doc async tools without updating `agents-plugin/runtime.json`; launcher compatibility checks compare the required MCP tool surface against runtime metadata.
- Assuming MCP tool calls know the user's shell cwd; plugin-managed server cwd can be the plugin cache.
- Passing `"."` or `"<cwd>"` to lead actor setup is ambiguous in plugin-managed sessions; pass the repository's absolute filesystem path.
- Guessing among multiple host workspaces creates cross-project writes; root resolution must ask for explicit compatibility `root` or `ws.setup(root)` instead.
- Letting `WS_MCP_PROJECT_ROOT` shadow an explicit non-dot server startup root makes tests pass in this dogfooding repo while plugin-managed calls target the wrong project.
- Treating namespace override as a tool rename; wsflow changes user-facing namespace text and advertised setup alias, while generic MCP tool identifiers stay stable.
- Updating `specs.find` or `mental_models.find` MCP output without the CLI mirror; users dogfood the CLI fallback when MCP host behavior is unclear.
- Treating `ai-docs/ref/ws-mcp.md` as the MCP contract source of truth instead of an operations runbook; this recreates schema drift with `tools()` and `runtime.capabilities`.
- Migrating agent or exec state into SQLite while also moving large stream payloads into the database; that defeats raw tail/read/grep and increases lock pressure.
- Classifying `*_path` fields as file-backed payloads; the path strings are SQLite metadata indexes even when the bytes at those paths stay file-backed.
- Treating a missing exec stream file as empty output; status/result/raw readers must surface the recoverable file-backed payload consistency state so prune/tombstone or repair paths can diagnose the artifact.
- Testing actor-scoped agent dispatch only with a live bogus worker; use controlled completed scoped fixtures for wait/result/print assertions so timing does not decide whether dispatch was correct.

## Technical Debt

- MCP input still uses `bufio.Scanner`; very large single-line MCP requests can hit scanner token limits before tool handling.
- Some compatibility docs mention broader future repair semantics, but `spec_index.verify` currently checks duplicate anchors only.
