---
title: MCP Tools
summary: Host-neutral ws MCP tool contracts for project context, workflow state, Git, documentation, and named-agent orchestration.
---

# MCP Tools

The ws MCP server exposes workflow capabilities through named MCP tools rather
than host-specific shell commands or repository-local paths. Tool outputs are
plain MCP text content that callers can use from Codex, Claude, or another
MCP-capable host.

This spec owns stable caller-visible behavior, not a copied tool schema
inventory. The runtime-owned MCP registry and `tools/list` response own input
schemas, and `runtime capabilities` owns the launcher-facing surface inventory.
When those runtime-discoverable fields change without changing caller-visible
behavior, update code and tests rather than copying field lists into this spec.

## MCP Server Protocol Surface {#260505-mcp-server-protocol-surface}

The `ws-mcp serve --stdio` process implements a stdio JSON-RPC MCP server. It
responds to `initialize`, `tools/list`, and `tools/call`, advertises MCP
protocol version `2025-03-26`, and declares tool capability.

Unknown methods and profile-rejected tools return JSON-RPC errors. Tool-level
runtime failures return MCP text content with `isError: true`, preserving a
normal MCP response envelope while still making the failure visible to callers.

Setup calls are request-order fences. When `ws.setup` or the advertised setup
alias appears in the stdio stream, the server completes earlier in-flight
requests, applies setup synchronously, writes that setup response, and only then
accepts later requests from the same stream. This preserves batch-safe
setup-then-call behavior for session and actor state.

Read-only tools whose primary consumer is an LLM prefer compact readable text
defaults over JSON serialized into text content. Tools that need stable machine
parsing, launcher compatibility, or structured protocol metadata preserve an
explicit JSON or full-detail escape hatch. {#260512-mcp-llm-readable-output-defaults}

The MCP server detects the host harness from observable MCP payloads before
relying on environment variables. It inspects `initialize.params` and request
metadata for high-confidence Codex or Claude markers, treats
`tools/call.params._meta.x-codex-turn-metadata` as a Codex signal, and records
conflicting signals in diagnostics instead of silently changing the session
harness. The detected harness is exposed through session inspection output.
{#260508-mcp-payload-harness-detection}

## Runtime And Debug Metadata Tools {#260505-runtime-debug-metadata-tools}

`runtime.info` returns runtime compatibility metadata, including the runtime
version, source commit, and prompt bundle metadata. Launchers and workflow
checks use this output to detect stale or incompatible runtime binaries.
The default response is compact labeled text; callers that need stable fields
can request structured JSON.

`runtime.debug_events` returns recent in-process MCP debug events as JSONL. The
tool is bounded by an optional limit parameter and is intended for diagnosing MCP
server behavior without reading process-local files directly.

## MCP Session Root Defaults {#260505-mcp-session-default-root}

Root-aware MCP tools resolve omitted `root` arguments through the current MCP
server session before falling back to startup state. The priority is:

1. Explicit tool argument `root`.
2. Volatile session default root.
3. Host workspace metadata when the host provides exactly one workspace.
4. Explicit non-dot server startup root.
5. `WS_MCP_PROJECT_ROOT`.
6. Server startup root.

`ws.setup` is the public setup surface for session state. When called with
`root`, it validates that the path is inside a Git worktree, stores the
canonical worktree root in the current server process, and returns setup state.
The root-only value is volatile and does not write user config, ws cache config,
or repository files. Calling `ws.setup` without `root`, `method`, or `id`
reports current setup state, including the detected session harness when one has
been observed, and does not mint lead authority. The default response is compact
labeled text. Structured JSON remains accepted as hidden compatibility dispatch,
but the public setup schema does not advertise a `format` argument. Legacy
`session.*` root tools may remain callable as hidden compatibility dispatch, but
they are not advertised as canonical tools.

`ws.setup(method: "lead-workflow-bootstrap", root: "<absolute-working-directory>")`
creates a cooperative lead actor for a workflow session and returns an actor id
with explicit recovery guidance. New actor ids are short opaque recovery tokens
with an authority prefix and lowercase payload, such as `lead-k9f2p7qx`; callers
must treat the token as opaque and recover with the exact returned value rather
than parsing worktree routing details out of it. Callers must pass the absolute
repository path as a filesystem path; the MCP server cannot infer the agent's
current directory from placeholders or relative paths.
`ws.setup(id: "<actor-id>")` restores that actor in a fresh MCP server process
and binds the current session root to the actor root. Root-omitted actor-owned
tools such as agent registration, agent calls, and subqueries require either a
current actor binding or a hidden explicit-root compatibility argument. When
that binding is missing, root-omitted actor-owned tools return compact recovery
guidance pointing to `ws.setup(id: "<actor-id>")`; the full lead bootstrap
ceremony remains in workflow guidance rather than repeated in each tool error.
The actor model is a cooperative workflow guard, not a hard security boundary.
{#260524-mcp-actor-setup-bootstrap}

When host metadata names multiple workspaces and no higher-priority root exists,
root-aware tools refuse to guess and return an actionable error asking the caller
to pass the absolute repository path explicitly or call `ws.setup` with that
absolute path.

An explicit non-dot server startup root is treated as authoritative before the
launcher-provided project-root environment fallback. If that explicit startup
root is invalid, root-aware tools fail closed instead of silently falling back to
`WS_MCP_PROJECT_ROOT`.

## Config Tools {#260505-config-tools}

`config.show` returns the resolved ws user-local configuration path and current
configuration without modifying it. The default response is compact labeled
text, and structured JSON remains available for callers that need stable fields.

`config.agents_tier` is the compatibility surface for updating the
backend/model/effort mapping for a model alias. Callers provide `tier` as the
alias name and may provide a backend, a concrete model, a portable effort, a
harness selector, or any combination of those fields. When backend is omitted,
ws infers it from the model family where possible. Empty effort, omitted effort,
and `none` store the no-override state; supported non-empty effort values are
visible through configuration output. The update applies to the explicit harness
when provided, otherwise the detected MCP session harness when available, and
otherwise the default alias mapping. This makes `backend` mean the execution
backend rather than the alias-table key. {#260513-harness-local-agent-tier-config}

Configuration exposes harness-aware model alias mappings. `light`, `core`, and
`deep` map to backend/model defaults per harness, existing tier-shaped config is
migrated or wrapped for compatibility, and new documentation speaks in terms of
model aliases rather than workload tiers. {#260508-model-alias-config-tools}

## Project Context And Convention Tools {#260505-project-context-convention-tools}

`project_tree` renders the project document map, spec inventory, and active
ticket inventory for the current repository.

`infra.read` reads bundled ws infra documents shipped with the runtime by bare
stem or filename.
`convention.read` reads bundled convention documents shipped with the runtime,
such as ticket, spec, or mental-model conventions. Shared workflow skills use
these tools instead of hard-coded repository-local convention paths.

## Spec Discovery Tools {#260505-spec-discovery-tools}

`spec_stem.generate` returns a collision-free spec anchor stem for a descriptive
slug.

`spec_index.verify` checks the spec corpus for anchor-index health problems such
as duplicate stems.

`specs.list`, `specs.find`, and `specs.status` provide read-only spec discovery.
They expose spec file metadata, anchors, ticket references, marker context, query
matches, and exact-stem status without requiring callers to scan the spec tree
manually.

Spec, ticket, and mental-model discovery tools default to compact line-oriented
summaries. Broad list/find calls avoid expanding every nested anchor, phase,
related map, snippet, source, or spec-reference array unless callers request
JSON output. {#260512-documentation-discovery-readable-output-defaults}

Documentation lookup tools treat broad human `query` inputs as tolerant
candidate discovery while preserving exact structured selectors such as
`spec_stem`, `ticket_stem`, and `domain`. Default text output for broad
documentation queries groups evidence by document, renders document metadata as
`<path>\tscore=<score>\thits=<count>`, and lists selected line-number snippets
under each document. JSON output keeps document-centered metadata and adds
line-level match evidence. Convention lookup accepts common aliases such as
`spec`, `ticket`, and `mental-model`.
{#260519-tolerant-documentation-lookup-query-evidence}

## Ticket Discovery Tools {#260505-ticket-discovery-tools}

`tickets.list` returns ticket paths and structured status metadata across ticket
status directories. Active discovery includes `ready/`, `todo/`, and `idea/` by
default; archived `.done/` and `.dropped/` tickets are omitted unless explicitly
requested. `ready/` identifies spec-addressed implementation work, while
`todo/` remains accepted backlog.

`tickets.find` locates tickets by text query, exact ticket stem, mentioned
ticket stem, and optional status filters. `tickets.status` returns structured
metadata for a single ticket stem and can optionally include archived done or
dropped tickets.

## Mental-Model Discovery Tools {#260505-mental-model-discovery-tools}

`mental_models.list` returns available mental-model documents with domain,
description, and source metadata.

`mental_models.find` locates mental-model paths by text query, domain, or spec
stem reference. `mental_models.status` returns path-first metadata for documents
selected by domain or path.

## Reference Trace Tool {#260505-reference-trace-tool}

`references.trace` returns the reference graph reachable from exactly one ticket
stem or spec stem. The result connects tickets, specs, and mental-model
documents so callers can inspect traceability without manually searching each
document system.

Small metadata and trace tools such as `api.list`, `ws.setup`, selected
runtime/config inspection views, and `references.trace` default to compact
labeled text where no caller needs stable structured fields.
Launcher-facing compatibility data remains available where required.
{#260512-metadata-trace-readable-output-defaults}

Interactive workflow command surfaces default to compact readable text when the
caller has not explicitly requested structured JSON. Write-capable workflow
tools summarize the completed action, affected paths or entities, and detected
workflow annotations without forcing callers to parse JSON. CLI mirrors follow
the same default where they are workflow-oriented wrappers, while Git command
mirrors preserve the original Git command output shape rather than reserializing
it into a ws-specific JSON envelope. Explicit JSON modes remain available for
structured consumers. {#260519-workflow-command-readable-output-defaults}

## Git Workflow Tools {#260505-git-workflow-tools}

`git.status` returns the current branch and worktree status.

`git.diff` returns read-only diff output. It defaults to stat mode for context
control and supports explicit `mode: "full"` for patch content or
`mode: "name_only"` for path listings. Range-less diffs include untracked files
where applicable.

`git.log` returns a bounded commit log with an optional body flag. `git.merge_base`
returns the merge base for two revisions.

Git read tools default to direct, LLM-readable text: `git.status` as a
branch/worktree summary with changed-file codes, `git.diff` as the selected
diff text, `git.log` as bounded commit blocks without JSON-escaped bodies, and
`git.merge_base` as a labeled hash line. JSON output remains available when a
caller explicitly asks for structured compatibility output.
{#260512-git-readable-output-defaults}

`git.commit` creates a workflow-aware commit from explicit paths and structured
message fields. It stages only the requested paths and formats commit messages
with required AI Context and optional ticket, spec, or mental-model update
sections. Ticket update detection recognizes added `### Result` headings and
added `#### Edition` headings so commit summaries can report first completion
records and later append-only tweak records. The default response is a compact
readable commit summary; callers can request structured JSON explicitly.
`git.commit` also accepts structured Mental Model Notes input and renders it as
a `### Mental Model Notes` sub-section under `## AI Context`, while preserving
the existing `ai_context` bullet path and deterministic commit body shape.
{#260519-git-commit-mental-model-notes}
When an explicit commit path names an old root from a rename or a deleted root,
`git.commit` stages the concrete removed paths reported by Git status rather
than passing the missing root to `git add`; requested roots with live changes
still stage through the explicit add path.
{#260513-git-commit-result-edition-detection}
`git.commit` ticket-change summaries conservatively reconstruct an unambiguous
same-stem ticket status move even when native Git reports the staged change as
separate add/delete records instead of a rename. Ambiguous add/delete sets remain
non-move ticket changes rather than inventing a destination status.
{#260519-git-commit-add-delete-ticket-move-summary}

## Workflow State And Delegation Tools {#260505-workflow-state-delegation-tools}

`subquery` starts an asynchronous scoped codebase or documentation query and
returns a generated subquery key immediately. Callers collect the result through
the named-agent result/status/tail/cancel surfaces.

`path.generate` allocates worktree-scoped writable artifact paths, such as review
files, so workflow agents can exchange file paths without inventing cache
locations.

## wsflow Agentless Runtime Mode {#260513-wsflow-agentless-runtime-mode}

The MCP server supports an environment-selected agentless product mode for the
internal `wsflow` distribution. With `WS_MCP_NO_AGENT=1`, advertised tools
omit named-agent, subquery, model-alias configuration, and agent-backed API
documentation surfaces: `agents.*`, `subquery`, `config.agents_tier`,
`api.ask`, `api.ask_async`, `api.status`, `api.result`, and `api.cancel`.
`api.list` remains available as read-only cache discovery.

Explicit calls to hidden agent-backed tools fail with a clear disabled error and
do not start named-agent workers. Runtime capability output and CLI command
surfaces match the selected mode, so no-agent mode omits the hidden MCP tools
and matching CLI groups such as `agents`, `subquery`, and
`config agents-tier`.

`WS_MCP_NAMESPACE=wsflow` changes ordinary user-facing namespace text to
`wsflow` without renaming generic MCP tool names. If `WS_MCP_NAMESPACE` is
unset or empty, the server keeps the default `ws` namespace and existing full
plugin behavior. `WS_MCP_SETUP_TOOL=setup` advertises `setup` instead of
`ws.setup`; when unset or empty, the canonical setup name remains `ws.setup`.
`ws.setup` may remain available only as hidden compatibility dispatch when a
different setup name is advertised.

## 🚧 wsflow Prompt Render Tool {#260529-prompt-render-tool}

`prompt.render(stem, context) -> { prompt_path }` is a read-only tool for the
wsflow product mode. It loads a bundled delegate prompt by `stem`, applies the
render-time `ws/` -> `wsflow/` namespace substitution used for wsflow-facing
text, injects caller-supplied `context` values into the prompt body, writes the
rendered result to a temporary file, and returns that file path. The caller
hands `prompt_path` to a native host subagent.

The tool carries no routing or strategy decision: the caller selects the stem,
and `prompt.render` only materializes a context-injected, namespace-substituted
copy. It does not mint or require an `expected_output_path`. Free-response
prompts return their result as the subagent's text; file-writing prompts such as
`plan-populator-*` and `mental-model-updater` receive a caller-supplied output
path through `context`, and the rendered prompt body directs the write to that
path.

wsflow exposes exactly five render-eligible prompts: `project-survey`,
`plan-populator-survey`, `plan-populator-research`, `code-reviewer`, and
`mental-model-updater`. The `implementer` prompt is not render-eligible in
wsflow.

`prompt.render` belongs to a symmetric wsflow-only tool surface. Just as the
agentless mode (`#260513-wsflow-agentless-runtime-mode`) hides agent-backed
tools from the wsflow distribution, wsflow-only tools are hidden from the full
ws distribution: a full ws session does not advertise `prompt.render`, while a
wsflow session does. Advertisement in `tools/list`, explicit-call gating, and
runtime capability output all follow the selected product mode in both
directions. {#260529-wsflow-only-tool-surface}

## Named-Agent MCP Tools {#260505-named-agent-mcp-tools}

The `agents.*` tool family exposes durable named-agent orchestration.

`agents.register` creates or updates an agent record with backend, model alias
or compatibility tier field, resolved model, prompt references, or materialized
system prompt text. `agents.call` starts an asynchronous call and returns
immediately. Public named-agent workflows use `ws.setup` for session root
selection; explicit root arguments may remain accepted as a hidden compatibility
override. Public and generated actor-owned schemas for `agents.*` and `subquery`
omit `root` end-to-end, including raw advertised schema metadata and
host-visible generated metadata, while preserving intentional hidden
explicit-root dispatch compatibility.
{#260523-agents-root-schema-invisibility}

When the parent MCP session is bound to an actor and the call targets that actor
root, named-agent registration/calls receive a persistent delegated child actor
id in agent metadata plus a child setup instruction in the system prompt.
Rootless actor-scoped subqueries receive ephemeral reader child actors with the
same recovery instruction shape and do not receive the lead bootstrap method.

Root-omitted actor-owned MCP calls in an actor-bound MCP session resolve through
the current actor scope. For `agents.*`, this includes registration, call, wait,
result, status, tail, interrupt, cancel, print, and erase. Root-omitted
`subquery` starts the generated subquery agent in the same actor scope as the
printed rootless follow-up commands. Hidden explicit-root compatibility calls
use the unbound global namespace, so an actor-bound session can still inspect or
manage a global compatibility registration explicitly without shadowing the
actor-local agent of the same public name.

`agents.register` prefers `model` as the public model-selection field.
`model: "light"`, `model: "core"`, and `model: "deep"` select portable
aliases; concrete provider model names select a one-off backend model. The
`tier` field remains a deprecated compatibility input. Alias resolution may
supply optional effort metadata; `agents.register` does not accept direct effort
input, and backend calls apply effort only when the selected alias resolves a
non-empty effort.
{#260508-agents-register-model-alias-field}

`agents.wait` waits for one or more agents to become ready and returns readiness
metadata, not final output. `agents.result` is the result-consumption surface and
may optionally wait for completion; successful ephemeral agents are erased after
their result is consumed.

`agents.status`, `agents.tail`, and `agents.cancel` inspect or control current
agent work. Cancelled status text points callers toward retrying `agents.call`
on the same registered agent when no result is available, so timeout-driven
cancellation does not look like a final erase-only state.
{#260512-agent-cancel-resume-guidance}

`agents.recall` is hidden from the advertised MCP tool surface and workflow
guidance. The implementation may remain as a manual or compatibility path, but
ordinary model-visible recovery uses `agents.call` on the same registered agent.
{#260512-agent-recall-hidden-surface}

Normal `agents.tail` is context-bounded. Raw diagnostic inspection is available
through `agents.debug.tail`, `agents.debug.stdout`, `agents.debug.stderr`,
`agents.debug.runtime_log`, and `agents.debug.events`.

`agents.interrupt` queues a redirect message for a running agent. `agents.print`
remains a deprecated compatibility reader over the resolved current instance.
`agents.erase` removes or hides the resolved role pointer for the current
worktree and actor scope; historical instance payloads are removed later by the
named-agent retention cleanup policy rather than synchronously during erase.

## API Documentation MCP Tools {#260505-api-documentation-mcp-tools}

`api.list` returns sorted API documentation cache domain names under
`ai-docs/.deps`. The default response is one domain per line, with structured
JSON available on request.

`api.ask` asks cached or fetchable third-party API documentation through
per-domain manager sessions. Callers provide a prompt and may provide a domain
hint. The tool owns the API-doc routing and aggregation behavior internally and
returns a synchronous answer to the caller.

The API documentation tool family also exposes an async job surface for lookups
that can outlive the host tool-call timeout. `api.ask_async` starts a job and
returns an `api_job_key`; `api.status` reports routing and per-domain progress;
`api.result` returns the final answer when available; and `api.cancel` stops
active work on a best-effort basis. {#260508-api-documentation-async-mcp-tools}

## Exec Job MCP Tools {#260524-exec-job-mcp-tools}

The `exec.*` tool family exposes durable command execution jobs for trusted lead
workflows. `exec.spawn` runs structured argv commands with `cmd`, optional
`args`, optional `working_dir`, optional environment overlays, and optional
textual stdin. `exec.shell` runs an explicit shell command string with optional
`working_dir`, environment overlays, textual stdin, and shell selection. Omitted
`working_dir` resolves to the current ws worktree root. Relative values resolve
beneath that root rather than the plugin cache process cwd, and resolved working
directories must stay inside the worktree root.

Launch tools create an `exec_key`, start the process, persist stdout and stderr
under job-owned files, and wait up to a fixed short foreground window before
returning. That foreground wait is not a caller-configurable timeout. When a job
finishes during the window and combined stdout plus stderr is within the fixed
4096-byte inline budget, the launch response may include the output, exit
status, `exec_key`, and metadata. Running jobs or larger outputs return compact
metadata, stream sizes, and follow-up guidance without inline raw output.

The `exec.*` MCP tools return MCP text content formatted for direct model
reading; they do not expose a public `format: json` response mode. Lifecycle
responses use compact labeled metadata such as `exec_key`, `status`,
`result_ready`, timestamps, exit state, and stream byte counts. When inline
stdout or stderr is present, metadata appears first and raw stream text appears
below obvious separator lines such as `========== stdout ==========` and
`========== stderr ==========`. JSON-shaped command output remains raw text in
that output area rather than being escaped inside a serialized JSON response.
If output exceeds the fixed 4096-byte inline budget, lifecycle responses keep
the raw body out of the result and include guidance to use the raw fallback
readers.

Exec lifecycle metadata is SQLite-backed while stream payload bytes remain in
job-owned files. SQLite stores job identity, command and working-directory
metadata, lifecycle state, process or lost-worker state, timestamps, exit
status, stream paths, stream byte counts, and retention/prune metadata. Existing
file-backed exec state is imported when possible; corrupt or unimportable legacy
state returns bounded recovery metadata rather than silently disappearing.

`exec.status` reports job lifecycle state and stream metadata. `exec.result`
returns job metadata and at most the fixed 4096-byte inline output budget for a
terminal job. When `timeout_seconds` is omitted or zero, `exec.result` is
non-blocking; a running job returns readable running metadata and guidance
without an MCP error. When `timeout_seconds` is positive, `exec.result` waits up
to that many seconds for the job to become terminal, then returns either the
terminal result or the same readable running guidance if the timeout expires.
Larger results guide callers to the future `exec.ask` path first and the raw
fallback readers second. `exec.abort` best-effort terminates a running job while
preserving partial output and terminal state metadata.

If a process-local worker is lost while a persisted job still appears running,
later status/result calls reconcile the record from process liveness and mark a
missing worker terminal rather than leaving the job indefinitely running.

Raw fallback readers are named under `exec.raw.*`. `exec.raw.tail` returns a
bounded tail from a selected stream. `exec.raw.read` reads by byte offset and
returns `next_offset`. `exec.raw.grep` searches selected streams, defaults to
literal matching, and uses regular expressions only when the caller explicitly
sets `regex: true`. If a stored stream path is missing, raw readers report a
recoverable file-backed payload consistency state instead of treating the stream
as empty.

Raw-reader MCP responses are also readable text rather than JSON payload text.
They identify the selected `exec_key` and `stream` with labels. Tail and read
responses place returned bytes below a `========== text ==========` separator;
read responses additionally expose `offset`, `next_offset`, `limit`, `size`,
and `eof` metadata above the separator. Grep responses expose match count and
truncation metadata above `========== matches ==========` and render each match
as readable line blocks with any requested context.

## Runtime Metadata Migration Gate {#260525-runtime-metadata-migration-gate}

The ws runtime has a SQLite metadata migration gate for moving named-agent and
exec runtime metadata into SQLite authority. The gate keeps public `agents.*`
and `exec.*` MCP APIs stable while separating lifecycle metadata from
file-backed payload bodies. Named-agent registry metadata and exec job metadata
are SQLite-backed. SQLite metadata may track identities, lifecycle state,
actor/session binding, path indexes, byte counts, retention visibility, leases,
tombstones, and prune bookkeeping. Prompts, streams, runtime logs, event JSONL,
transcripts, backend raw output, and final output bodies remain file-backed.

SQLite state-store configure, migration, and short write paths use bounded
retry for `SQLITE_BUSY` and `SQLITE_LOCKED` conditions while retaining
process-local write serialization. Runtime migrations must keep transactions
short and must not hold a transaction across subprocess or model execution.

## Tool Profile Gating {#260505-tool-profile-gating}

The MCP server defaults to the `lead` tool surface. It does not derive authority
from worktree-local locks or startup-root ownership, because plugin-managed hosts
can start the server from cache directories and can fail to propagate
environment variables consistently.

`WS_MCP_TOOL_PROFILE` is an optional profile filter, not an authority boundary.
When the host successfully propagates it, `delegate` and `leaf` receive narrower
tool sets for dogfood containment and tests. Delegate access to generated
subquery agents is scoped to subquery result, status, tail, cancel, and
print-style operations. Leaf also hides recursive orchestration and selected
mutation tools.

When profile environment propagation fails, delegated agents may see the full
lead MCP surface. Workflow containment therefore depends on prompt rules such as
delegate orientation and lead-owned orchestration instructions, not on MCP
tool-surface filtering. `WS_MCP_ALLOWED_TOOLS` can further narrow the visible
surface for tests or debugging, but it cannot expand access beyond the selected
profile.

## CLI Mirror Coverage {#260505-cli-mirror-coverage}

The `ws-mcp` binary mirrors selected MCP behavior as CLI commands for smoke
tests, compatibility probes, and fallback usage.

CLI mirrors exist for runtime info, single-process smoke checks, config, path
generation, subquery, named agents, Git, tickets, specs, selected mental-model
discovery, and reference tracing. Not every MCP tool has a CLI mirror; the MCP
surface is the canonical host-neutral interface, and CLI coverage is limited to
the surfaces needed for runtime checks and workflow fallback use.
