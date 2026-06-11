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

Root-aware MCP tools resolve their repository root exclusively from a mandatory
`session_key` argument; root resolution is the ephemeral session-auth model
(`#260610-ephemeral-session-auth-model`). There is no fallback chain. A root-aware
call without a `session_key` is rejected with mandatory-login guidance naming
`ws.lead.login(root)`; a call whose key is absent from the in-memory registry is
rejected with the `unknown_session` recovery contract. `ws.lead.login(root)` is
the sole bootstrap verb and the only tool that accepts a `root` argument.

The former resolution sources are removed: the explicit per-tool `root` argument,
the volatile session default root, host-workspace metadata, the explicit server
startup root, `WS_MCP_PROJECT_ROOT` as a resolution source, the `ws.setup` public
setup surface (both the bare root-session form and the
`lead-workflow-bootstrap` actor form), and the persistent actor / authority /
child-actor bootstrap. With root carried by a per-call key rather than a
process-global default field, concurrent distinct worktree roots resolve without
clobber and without the former request-order setup fence.

## Ephemeral Session-Auth Model {#260610-ephemeral-session-auth-model}

The former persistent actor / authority / child-actor model has been replaced by
an ephemeral, in-memory session-auth model. This is the caller-visible
authentication contract for ws tool calls.

A lead-centric bootstrap verb mints a session:
`ws.lead.login(root) -> session_key`. The returned key is an LLM-friendly
word-chain string (for example `amber-tide-fox`), not a UUID. Only the lead logs
in; subagents and mercenaries never call login — they receive a render-minted key
(`#260610-mercenary-delegation-surface`).

Every ws tool call carries a session key (REST-bearer style). There is no keyless
fallback to a foreign root: a call without a valid key does not silently operate
on a server-default or lead root. This closes the wrong-tree footgun in which a
worktree delegate doing root-omitted calls silently mutated the lead's main
repository.

The server holds a concurrency-safe in-memory `{session_key -> root context}`
map. It replaces the process-global default-root field and the request-order
setup fence, so parallel requests each resolve their own root with no
serialization and no shared-field clobber. The map is in-memory only — no
SQLite, no persistent store.

`login` is a bootstrap verb only: there is no logout and no eviction (rows are a
tiny `(word-chain key, root path)` bounded by the number of distinct roots a
fleet touches).

Every keyed call honors an `unknown_session` recovery contract: when a key is not
found (for example after the in-memory map is lost to an MCP process restart),
the call is rejected with an `unknown_session` signal and the caller re-logins
with its own known root and retries. Because the caller-visible contract
(`login(root) -> key`; `<tool>(key, …)`; re-login-on-reject) hides the backend,
a later persistent session store is a pure implementation swap with no contract
migration.

Key issuance reserves an optional capability/role-scope parameter from the first
cut (`#260505-tool-profile-gating`), so the lead can mint capability-scoped keys
for delegates even if the first implementation honors only a single default
profile.

> [!note] Constraints
> - The session key is mandatory on every ws call; there is no keyless lead
>   default. A delegate that drops its key gets `unknown_session`, not a silent
>   foreign-root operation.
> - `login` is lead-only and lives under the `ws.lead.*` namespace. A non-lead
>   key calling `ws.lead.*` is rejected by the keyed-call handler
>   (`#260610-mercenary-delegation-surface`), so a delegate cannot self-login or
>   escalate by logging in again from a contained context. Re-login for recovery
>   uses the caller's own already-known root.
> - The model is in-memory with no eviction; a persistent backend is deferred
>   until session state grows heavy and is a contract-invariant swap.

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

> [!note] Planned 🚧
> The `subquery` tool runtime is removed. Scoped exploration, survey, and
> one-turn fact-finding move to host-native subagents rendered through the
> playbook surface (`#260609-playbook-harness-rendering`); the mercenary surface
> (`#260610-mercenary-delegation-surface`) is scoped to implementer/reviewer only
> and does not cover exploration. The skill-facing `subquery` contract is retired;
> `path.generate` is unaffected. Current behavior is unchanged until removal lands.

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

## wsflow Prompt Render Tool {#260529-prompt-render-tool}

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

wsflow exposes exactly five render-eligible prompts: `reference-discovery`,
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

## Playbook Tools {#260609-playbook-tools}

The playbook tools are the ws-distribution surface for serving workflow procedure
text and subagent-injection prompts from a plain-text resource tree, with content
selected for the detected host harness. They are the full-ws generalization of the
wsflow-only `prompt.render` (`#260529-prompt-render-tool`): `prompt.render` stays
the wsflow surface during migration and is not removed by this surface.

`playbook.print(name, context?)` returns the named playbook's procedure text
inline in the tool result, with `context` values substituted and declared
includes resolved. It is the lead-facing successor of internal workflow-skill
bodies.

`playbook.render(name, context?)` materializes the named playbook as a
context-injected, harness-rendered prompt, writes it to a worktree-scoped
temporary file, and returns that file path. The caller hands the path to a
host-native subagent. Like `prompt.render`, it carries no routing or strategy
decision — the caller selects `name`, and the tool only materializes a rendered
copy.

A playbook is selected by `name`; the tool does not decide which playbook to use.
A load or render failure for a requested `name` is a loud error, not a silent
empty result.

Harness-aware content selection uses the harness the MCP session has already
detected (`#260508-mcp-payload-harness-detection`). Harness differences are
served as data, not as separate code paths: a shared playbook body plus a
per-harness terminology table (exploration agent name, spawn idiom, continuation
idiom, model aliases), with structural divergence expressed only through
per-harness overlay files. The supported harness set is Claude and Codex; an
unrecognized harness renders host-neutral text rather than failing. Concrete
per-provider model names are resolved from configuration
(`#260513-harness-local-agent-tier-config`), never baked into the resource tree
or the binary, so model-name churn is a config update rather than a
redistribution. {#260609-playbook-harness-rendering}

A playbook may declare text dependencies in its frontmatter; the renderer
auto-includes that text at print/render time, so a single `playbook.print(name)`
call returns the procedure together with its required conventions. The include
set is fixed at authoring time, not chosen by the caller per call. This does not
replace `convention.read` / `infra.read`, which remain standalone discovery tools
for raw access.

A playbook marked as delegating carries a compact continuation tip in its
rendered output, reminding the caller to reuse the host-returned subagent agent
id for continuation instead of respawning. The tip is the only continuity
mechanism: the playbook surface keeps no agent registry and mandates no
continuity-recording file.

> [!note] Constraints
> - Gemini is out of scope; only Claude and Codex have terminology tables. Any
>   other harness, including none detected, gets host-neutral text.
> - The continuation tip is advisory text, not an enforced or tracked binding.

### Resource Tree Distribution {#260609-rsrc-playbook-distribution}

Playbook and prompt text ships as a plain-text resource tree distributed with the
plugin and loaded at call time, rather than compiled into the binary. Text-only
changes to playbooks are therefore deployable without a binary version change.

The tree carries a manifest recording per-file integrity data and a playbook
schema version. The runtime gates loading on **schema-version compatibility**,
not on exact content-hash equality, so compatible text edits load without a
binary bump while an incompatible schema version is refused.

A manifest mismatch or load failure is a loud, partial failure of the playbook
surface: the playbook tools report the failure and do not serve playbook content,
and there is no embedded fallback copy. A session whose playbook surface has
failed still serves the discovery, Git, and other tools that do not depend on the
resource tree.

`WS_RSRC_ROOT` overrides the resource-tree load root. When set, the runtime loads
the tree from that path instead of the distributed plugin copy, so a development
checkout can edit playbook text and see it live without waiting on plugin cache
refresh.

> [!note] Constraints
> - Compatibility is defined by schema version, not file-hash equality; the
>   manifest's hashes are integrity data, not a load gate.
> - There is no embedded fallback text. When the resource tree is unavailable or
>   incompatible, the playbook surface fails loudly rather than degrading to a
>   stale built-in copy.

## Named-Agent MCP Tools {#260505-named-agent-mcp-tools}

The `agents.*` tool family exposes durable named-agent orchestration.

> [!note] Planned 🚧
> The `agents.*` family is reshaped — not wholly removed — into the scoped
> mercenary delegation surface (`#260610-mercenary-delegation-surface`). The
> retained surface is smaller: `agents.register(prompts: [stems])` and the
> model-alias registration field (`#260508-agents-register-model-alias-field`)
> are dropped in favor of a single self-contained prompt from `playbook.render`;
> the former actor-scoped root invisibility contract is already removed under
> mandatory session keys (Phase 2a); mercenaries are scoped to implementer/reviewer
> roles only; and the
> gemini runner, the `subquery` runtime, exploration-purpose spawns, and
> diagnostic sprawl beyond mercenary needs are removed. The cancel-retry guidance
> (`#260512-agent-cancel-resume-guidance`) and hidden `agents.recall`
> (`#260512-agent-recall-hidden-surface`) carry over to the mercenary path. Current
> behavior is unchanged until the reshape lands.

`agents.register` creates or updates an agent record with backend, model alias
or compatibility tier field, resolved model, prompt references, or materialized
system prompt text. `agents.call` starts an asynchronous call and returns
immediately. Named-agent calls resolve their root from the mandatory `session_key`
like every other root-aware tool (`#260610-ephemeral-session-auth-model`): no
`agents.*` or `subquery` schema advertises a `root` argument, and there is no
actor scope, hidden explicit-root dispatch, or persistent child-actor credential
injection. The named-agent registry namespaces role pointers by the resolved
worktree root, so the same public agent name stays distinct across distinct
worktree roots without an actor dimension.

`agents.register` prefers `model` as the public model-selection field.
`model: "light"`, `model: "core"`, and `model: "deep"` select portable
aliases; concrete provider model names select a one-off backend model. The
`tier` field remains a deprecated compatibility input. Alias resolution may
supply optional effort metadata; `agents.register` does not accept direct effort
input, and backend calls apply effort only when the selected alias resolves a
non-empty effort.
{#260508-agents-register-model-alias-field}

> [!note] Planned 🚧
> This registration contract is retired. Mercenaries are invoked with a single
> self-contained prompt from `playbook.render`
> (`#260610-mercenary-delegation-surface`), so `agents.register(prompts: [stems])`
> and the registration-time model-alias/`tier` field are removed; per-mercenary
> model selection moves into the rendered prompt and harness config
> (`#260513-harness-local-agent-tier-config`).

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

## 🚧 Mercenary Delegation Surface {#260610-mercenary-delegation-surface}

The reshaped delegation surface. A **mercenary** is a ws-spawned external
subprocess agent — a deliberately distinct term from a harness-native
**subagent**, so callers never confuse the two delegation paths. This section is
the planned caller-visible contract for the reshaped `agents.*` family; it is not
yet implemented.

**Default is native; mercenary is always available.** Default delegation is
always to a host-native subagent. The mercenary path is always available to the
lead — it is not a feature flag the user must enable. A mercenary is invoked only
when (a) the user explicitly requests it, or (b) the lead has flipped its session
key's render mode with `ws.lead.prefer_mercenary(session_key)` (lead-only), which
changes only the *default delegation guidance* `playbook.render` emits for
implementer/reviewer playbooks — never availability. Independently, every
delegation-capable rendering carries a small always-on tip fragment noting the
mercenary path is reachable on request, so the on-request path works without the
toggle.

**Scope: implementer and reviewer roles only.** Mercenaries cover implementer and
reviewer delegation. Exploration, survey (reference-discovery, plan-populator),
and mental-model update route to host-native subagents
(`#260609-playbook-harness-rendering`), not mercenaries.

**Live, pluggable backends.** The codex and claude runner backends are retained
and live. The runner-backend interface is harness-neutral and pluggable: the
gemini backend implementation is unshipped (model-compat cost), but the plug
point is preserved so gemini, antigravity, or a custom harness can re-attach as a
deferred plug, not a structural exclusion.

**Single self-contained prompt; native-shaped handle.** A mercenary is invoked
with one self-contained prompt produced by `playbook.render`
(`#260609-playbook-tools`); there is no `register(prompts: [stems])` step. A
mercenary call returns a continuation handle of the same shape as a native
subagent id, so the lead reuses one continuation idiom across both paths.

**Render-minted child keys.** `playbook.render(session_key, name, context?,
root_override?)` is the mint-and-inject point for both native and mercenary
delegates: when `session_key.role == lead` it mints a fresh child key (role taken
from the playbook frontmatter) and splices it into the rendered prompt, so the
delegate receives a prompt with its key already embedded. `root_override` rebinds
both the auto-include resolution root and the child-key binding root when the
child runs in a different worktree; render does not infer worktree shape — the
caller passes the path.

**Containment is server-side on the keyed call handler.** The keyed `tools/call`
handler rejects `ws.lead.*` calls from non-lead keys. A child key (native or
mercenary) is therefore unable to login or spawn, so spawn depth is strictly 1
(lead → mercenary leaf); no recursion-depth counter is needed. Schema and
`tools/list` filtering remain a harness-owned soft-guard for LLM-confusion
reduction only — they are not the enforcement boundary.

> [!note] Constraints
> - Mercenary availability is not user-gated; only the *default guidance* flips,
>   via `ws.lead.prefer_mercenary`. The on-request path is always reachable.
> - Mercenary scope is implementer/reviewer only. Exploration and mental-model
>   work are native-subagent only and never mint a mercenary.
> - Gemini is a preserved plug point, not a shipped backend.
> - Containment is the server-side keyed-handler role check, not schema hiding.

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

> [!note] Planned 🚧
> Role containment folds into capability-scoped session keys
> (`#260610-ephemeral-session-auth-model`): a key carries `{root + optional
> capability/role scope}`, and enforcement is the server-side role check in the
> keyed `tools/call` handler (`#260610-mercenary-delegation-surface`). The
> `WS_MCP_TOOL_PROFILE` env profile is verified non-functional for containment
> (it is a soft-guard only, lost whenever the host fails to propagate the env
> var) and is retained as defense-in-depth, not as the enforcement boundary.
> Current behavior is unchanged until the keyed-handler check lands.

## CLI Mirror Coverage {#260505-cli-mirror-coverage}

The `ws-mcp` binary mirrors selected MCP behavior as CLI commands for smoke
tests, compatibility probes, and fallback usage.

CLI mirrors exist for runtime info, single-process smoke checks, config, path
generation, subquery, named agents, Git, tickets, specs, selected mental-model
discovery, and reference tracing. Not every MCP tool has a CLI mirror; the MCP
surface is the canonical host-neutral interface, and CLI coverage is limited to
the surfaces needed for runtime checks and workflow fallback use.
