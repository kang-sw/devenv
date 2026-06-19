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

`runtime.info` returns runtime compatibility metadata: the runtime version and
source commit. (Prompt bundle metadata was removed when the embedded prompt
bundle was retired in favor of the rsrc tree.) Launchers and workflow checks use
this output to detect stale or incompatible runtime binaries.
The default response is compact labeled text; callers that need stable fields
can request structured JSON.

`runtime.debug_events` returns recent in-process MCP debug events as JSONL. The
tool is bounded by an optional limit parameter and is intended for diagnosing MCP
server behavior without reading process-local files directly.

## MCP Session Root Defaults {#260505-mcp-session-default-root}

Root-aware MCP tools resolve their repository root exclusively from a mandatory
`session_key` argument; root resolution is the ephemeral session-auth model
(`#260610-ephemeral-session-auth-model`). There is no fallback chain. A root-aware
call without a `session_key` is rejected with `mandatory_session_key` guidance
that routes the lead to `ws:workflow-manual` (the recovery message names no tool,
per the bootstrap-name obscurity scrub); a call whose key has no record in the
session store is
rejected with the `unknown_session` recovery contract. Public schemas for
root-aware tools advertise `session_key` and do not advertise `root`;
`ws.ferrule(root)` is the sole bootstrap verb and the only tool that accepts
a `root` argument.

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
`ws.ferrule(root) -> session_key`. The returned key is an LLM-friendly
word-chain string (for example `amber-tide-fox`), not a UUID. Only the lead logs
in; subagents and mercenaries never call login — they receive a render-minted key
(`#260610-mercenary-delegation-surface`).

Every ws tool call carries a session key (REST-bearer style). There is no keyless
fallback to a foreign root: a call without a valid key does not silently operate
on a server-default or lead root. This closes the wrong-tree footgun in which a
worktree delegate doing root-omitted calls silently mutated the lead's main
repository.

The server resolves `{session_key -> root context}` from a flat, filesystem-backed
store: one JSON record per key at `<cache-root>/keys/<session_key>.json`. It
replaces the process-global default-root field and the request-order setup fence,
so parallel requests each resolve their own root with no serialization and no
shared-field clobber. The file is the source of truth, not the process: keys are
minted with an `O_EXCL` create (atomic cross-process uniqueness) and updated with
temp-write + rename (no partial reads), and per-key sharding removes write
contention without a single shared file or SQLite. A fresh MCP server instance —
a subagent that did not inherit the lead's process, or a lead that restarted
mid-delegation — resolves a key by reading its file, so session continuity does
not depend on a shared in-memory registry.

`login` is a bootstrap verb only: there is no logout and no eviction (rows are a
tiny `(word-chain key, root path)` bounded by the number of distinct roots a
fleet touches).

Every keyed call honors an `unknown_session` recovery contract: when a key has no
record file (a genuinely unknown or path-unsafe key, or state cleared by deleting
the cache), the call is rejected with an `unknown_session` signal and the caller
re-logins with its own known root and retries. Because the caller-visible contract
(`login(root) -> key`; `<tool>(key, …)`; re-login-on-reject) hides the backend,
the move from the original in-memory map to this filesystem-backed store was a
pure implementation swap with no contract migration.

Key issuance accepts an optional capability/role-scope parameter
(`#260505-tool-profile-gating`), so the lead can mint capability-scoped keys for
delegates; the keyed `tools/call` handler enforces that scope (see Tool Profile
Gating).

> [!note] Constraints
> - The session key is mandatory on every ws call; there is no keyless lead
>   default. A delegate that drops its key gets `unknown_session`, not a silent
>   foreign-root operation.
> - The bootstrap tool (`ws.ferrule`) is lead-only. It lives outside the
>   `ws.lead.*` namespace, so the keyed-call handler blocks non-lead keys from it
>   by explicit name in addition to the `ws.lead.*` prefix block
>   (`#260610-mercenary-delegation-surface`); a delegate cannot self-bootstrap or
>   escalate from a contained context. Re-bootstrap for recovery uses the caller's
>   own already-known root.
> - The bootstrap tool name is deliberately obscure (260617 obscurity, soft
>   guard): semantically disconnected from "session start" and taught only in
>   `ws:workflow-manual`. The three subagent-reachable surfaces must not leak it —
>   the `tools/list` description is inert, error-guidance strings name no tool and
>   route the lead to the manual, and the rendered delegate prompt carries a
>   capability-level instruction with no tool name. This lowers accidental/curious
>   invocation by subagents that share the lead's MCP connection; it is not a hard
>   barrier (a name-aware caller can still keyless-bootstrap).
> - The store is filesystem-backed (one record file per key under a flat `keys/`
>   directory) and survives a server restart. There is still no logout and no
>   automatic eviction, though deleting a key file is now a physically possible
>   removal path (deferred).

### Session-Key Lineage And Child Enumeration {#260619-session-key-lineage-children}

Session keys form a parent→child lineage so a lead can re-discover the keys it
minted after they fall out of its own (compacted or restarted) context.

- Each session record carries an optional `parent` — the session key that minted
  it. It is absent for a lead's first bootstrap key. Recording a parent is
  metadata only and never widens the child's capability scope.
- `ws.ferrule` accepts an optional `parent_session_key`. A lead coordinating
  several repository roots in one conversation (for example multiple git
  worktrees, each a distinct root) records each additional control key's parent
  as its primary control key. Because `ws.ferrule` is lead-only, control-key
  lineage stays within one lead — it does not create a tree of independent
  control agents.
- A render-minted delegate child key (`playbook.render`, including a
  worktree-bound leaf produced via `root_override`) records the dispatching lead
  key as its `parent`.
- `session.children(session_key, depth?, format?, include_dead?)` returns,
  read-only, the subtree of keys whose `parent` chain roots at the presented
  key. Each entry is labeled by its capability scope (control coordination key
  vs delegate leaf) and includes the child key string so the lead can re-thread
  it. A caller only ever sees the subtree under a key it presents. It lives in
  the `session.*` tool family, so the existing keyed-gate `session.` prefix block
  already restricts it to lead-scoped keys (a delegate/leaf key is rejected;
  those scopes mint no children anyway).
  - `depth`: integer, default `1` (immediate children); a higher value returns
    that many levels; `0` returns the full subtree.
  - Liveness is whether the child's bound `root` path still exists. Dead keys
    (such as a removed worktree's key) are filtered by default;
    `include_dead: true` returns them flagged `live: no`, preserving a
    prune/debug path.
  - Output defaults to compact labeled text per
    `#260512-mcp-llm-readable-output-defaults`; `format: "json"` is the
    structured escape hatch.

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

The delegation tier abstraction is the first-class capability vocabulary
`small`/`medium`/`large`/`xlarge`, which names task-intrinsic reasoning depth
independent of host or subscription plan. `light`/`core`/`deep` are conventional
aliases at the concrete-model layer (alongside provider model names), connected
to the capability axis by the locked mapping `light↦small`, `core↦medium`,
`deep↦large`; `xlarge` (fable-class) has no legacy alias. Playbook frontmatter
declares `role:` and `tier:` in the first-class vocabulary; a first-class tier is
resolved to a concrete backend/model by mapping through the alias layer into
`config.agents_tier` (`#260513-harness-local-agent-tier-config`), which remains
keyed by the `light`/`core`/`deep` alias. {#260612-first-class-tier-vocabulary}

### Layered Config Scope Model {#260619-layered-config-scope-model}

Config items resolve across four ordered scopes, highest precedence first:
`session > project > global > builtin`. `builtin` is the code default (for
example the `wsconfig` tier/alias defaults and `prefer_mercenary=false`). A read
returns the value from the highest-precedence scope that holds one.

Each config item declares a natural **default write scope** in code; items that
declare nothing fall back to `project`. A write without an explicit scope lands
in the item's declared default scope. An explicit `scope:` argument on a set
always wins over the declared default. `get`/`show` report *which scope* a value
resolved from, so a caller can see whether a value is session-, project-,
global-, or builtin-sourced.

Scope storage map:

- `session` — the per-key session store (`keys/<key>.json`,
  `#260617-stateless-subagent-context`); tied to the session key's lifetime.
- `project` — the existing project-scoped `config.json` under `${WS_CACHE_HOME}`
  (`~/.ws@<project-id>/`).
- `global` — a project-agnostic `~/.ws/config.json`, with a `WS_CONFIG_HOME`
  environment override mirroring the `WS_CACHE_HOME → ~/.ws@<id>/` convention.

Adding the `global` layer is non-breaking: because `project` outranks `global`,
existing project-stored values keep winning, so no data migration is required;
only the *write default* for future sets follows each item's declared scope.
Read-modify-write on the project and global files is serialized so concurrent
writers cannot corrupt the file (atomic replace under a file lock). The scope
resolution rule and the `scope` argument shape are a single shared contract that
every scope-aware config tool consumes, rather than per-tool re-implementations.

> [!note] Constraints
> - Scope-awareness is opt-in per config item; this contract does not retrofit
>   the existing `config.agents_tier` surface, which is re-homed under the same
>   model by the separate model-alias/role-tier rename rather than here.
> - Item-level write gating still applies: a scope-aware setter honors an item's
>   existing role/capability restrictions (not every item is freely settable at
>   every scope).
> - The substrate (resolver, default-scope registry, file-lock RMW, global store,
>   shared `scope` schema fragment) and scope-reporting on `config.show` are the
>   caller-visible surface today. Per-item scope-aware *set* surfaces arrive as
>   individual items adopt the model (`prefer_mercenary`
>   (`#260619-prefer-mercenary-session-scope-item`), prompt overrides); the set
>   capability otherwise lives at the internal `wsconfig` API.

## Project Context And Convention Tools {#260505-project-context-convention-tools}

`project_tree` renders the project document map, spec inventory, and active
ticket inventory for the current repository. The document map omits entries
ignored by the repository's Git ignore rules so generated or vendored
directories do not dominate the readable project context.

`infra.read` reads ws infra documents shipped in the rsrc tree by bare stem or
filename (path-escaping names are rejected). The backing source is the rsrc
loader; the embedded prompt bundle that previously served these documents was
retired.
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

`path.generate` allocates worktree-scoped writable artifact paths, such as review
files, so workflow agents can exchange file paths without inventing cache
locations.

## wsflow Agentless Runtime Mode {#260513-wsflow-agentless-runtime-mode}

The MCP server supports an environment-selected agentless product mode for the
internal `wsflow` distribution. With `WS_MCP_NO_AGENT=1`, advertised tools
omit named-agent and model-alias configuration surfaces:
`ws.mercenary.*` and `config.agents_tier`. `api.list` remains available as
read-only cache discovery; the agent-backed API documentation ask tools are
removed from the full ws surface rather than hidden only in wsflow mode.

Explicit calls to hidden agent-backed tools fail with a clear disabled error and
do not start named-agent workers. Runtime capability output and CLI command
surfaces match the selected mode, so no-agent mode omits the hidden MCP tools
and matching CLI groups such as `agents` and
`config agents-tier`.

`WS_MCP_NAMESPACE=wsflow` changes ordinary user-facing namespace text to
`wsflow` without renaming generic MCP tool names. If `WS_MCP_NAMESPACE` is
unset or empty, the server keeps the default `ws` namespace and existing full
plugin behavior. `WS_MCP_SETUP_TOOL=setup` advertises `setup` instead of
`ws.setup`; when unset or empty, the canonical setup name remains `ws.setup`.
`ws.setup` may remain available only as hidden compatibility dispatch when a
different setup name is advertised.

The playbook surface also follows product mode. In no-agent mode,
`playbook.print` and `playbook.render` serve the shared rsrc playbook bodies
through product-aware selection: `<!-- ws:full-only:start/end -->` regions are
omitted, `<!-- ws:wsflow-only:start/end -->` regions are included, marker
comments are never emitted, and the remaining user-facing namespace notation is
rendered through reserved namespace variables such as `McpNamespace` and
`SkillNamespace`. In wsflow these variables render as `wsflow`; in full ws they
render as `ws`. The variables do not rename literal generic MCP tool
identifiers such as `ws.ferrule`.

## Playbook Tools {#260609-playbook-tools}

The playbook tools are the ws-distribution surface for serving workflow procedure
text and subagent-injection prompts from a plain-text resource tree, with content
selected for the detected host harness. `prompt.render` was the retired
wsflow-only predecessor for delegate prompt materialization; it is no longer
advertised or callable in either product mode. Legacy wsflow delegate context
materialization is preserved through `playbook.render`.

`playbook.print(name, context?)` returns the named playbook's procedure text
inline in the tool result, with `context` values substituted and declared
includes resolved. It is the lead-facing successor of internal workflow-skill
bodies.

`playbook.render(session_key, name, context?, root_override?)` materializes the
named playbook as a context-injected, harness-rendered prompt, writes it to a
worktree-scoped temporary file, and returns that file path together with a
`recommended-tier` line carrying the playbook's first-class frontmatter tier (when
declared). The caller hands the path to a host-native subagent or a mercenary and
routes the recommended tier to whichever path it picks — as a host model-selection
guide for a native subagent, or as `ws.mercenary.register`'s pass-through `tier` for a
mercenary. `playbook.print` surfaces the same `recommended-tier` line. It
carries no routing or strategy decision — the caller selects `name`, and the
tool only materializes a rendered copy. `root_override`, when set, rebinds both the
auto-include resolution root and the child-key binding root for a delegate
running in a different worktree. When the calling `session_key` is lead-scoped
and the playbook frontmatter declares a delegate-eligible role, the render mints
a fresh child session key and splices it into the rendered prompt, so both native
and mercenary delegates receive a prompt with their key already embedded
(`#260610-mercenary-delegation-surface`).

In no-agent/wsflow mode only, `playbook.render` has a compatibility bridge for
the five legacy render-eligible stems. When `name` is one of those stems,
caller-supplied `context` is appended as prompt data in a `## Render Context`
block after normal playbook rendering rather than being interpreted as template
variables. The bridge does not apply to `implementer`, to arbitrary playbooks, or
to full ws mode.

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

Product-mode content selection is separate from harness selection. Shared rsrc
playbooks may mark full-ws-only or wsflow-only sections with the product markers
documented in `#260513-wsflow-agentless-runtime-mode`; `playbook.print` and
`playbook.render` select those sections after harness rendering and before
returning text or writing a prompt file. User-facing namespace notation in
shared playbooks is authored with reserved implicit variables (`McpNamespace`
for `ws/<tool>` notation and `SkillNamespace` for `ws:<skill>` notation). These
vars are injected by the playbook tool layer, are available without frontmatter
declarations, and override caller-supplied `context` keys. Literal MCP tool
identifiers remain literal unless a dedicated semantic variable is introduced.

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

When a caller requests a playbook stem that is absent from both the resource
manifest and the resource tree, the playbook surface reports a no-such-playbook
diagnostic. Manifest integrity diagnostics are reserved for corrupted or stale
resource trees, such as a manifest-listed file missing from disk or a listed
file whose hash no longer matches.

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

The `ws.mercenary.*` tool family exposes durable named-agent orchestration.

The `ws.mercenary.*` family is the reshaped scoped **mercenary** delegation surface
(`#260610-mercenary-delegation-surface`): codex and claude runners retained,
scoped to implementer/reviewer roles, invoked with a single self-contained prompt
from `playbook.render`.

`ws.mercenary.register` registers a mercenary agent with an optional `backend` (codex
or claude) and a self-contained `system_prompt_text` produced by
`playbook.render`. The former `prompts: [stems]`/`prompt_refs` and `model`
registration fields are removed. The `tier` field is a *pass-through* of the
first-class recommended tier that `playbook.render` returns — its origin is the
playbook frontmatter, not a caller-chosen workload tier: `ws.mercenary.register` maps it
to the alias layer and resolves the per-mercenary backend/model from harness config
(`#260513-harness-local-agent-tier-config`), so a mercenary's model follows its
playbook frontmatter `tier:` rather than defaulting to core. `ws.mercenary.call` starts an asynchronous
call, returns immediately, and yields a native-shaped continuation handle
(`agentId=<name>`) so the lead reuses one continuation idiom across the native
and mercenary paths. Named-agent calls resolve their root from the mandatory
`session_key` like every other root-aware tool
(`#260610-ephemeral-session-auth-model`): no `ws.mercenary.*` schema advertises a
`root` argument, and there is no actor scope, hidden explicit-root dispatch, or
persistent child-actor credential injection. The named-agent registry namespaces
role pointers by the resolved worktree root, so the same public agent name stays
distinct across distinct worktree roots without an actor dimension.

`ws.mercenary.wait` waits for one or more agents to become ready and returns readiness
metadata, not final output. `ws.mercenary.result` is the result-consumption surface and
may optionally wait for completion; successful ephemeral agents are erased after
their result is consumed.

`ws.mercenary.status`, `ws.mercenary.tail`, and `ws.mercenary.cancel` inspect or control current
agent work. Cancelled status text points callers toward retrying `ws.mercenary.call`
on the same registered agent when no result is available, so timeout-driven
cancellation does not look like a final erase-only state.
{#260512-agent-cancel-resume-guidance}

`ws.mercenary.recall` is hidden from the advertised MCP tool surface and workflow
guidance. The implementation may remain as a manual or compatibility path, but
ordinary model-visible recovery uses `ws.mercenary.call` on the same registered agent.
{#260512-agent-recall-hidden-surface}

Normal `ws.mercenary.tail` is context-bounded. Raw diagnostic inspection is available
through `ws.mercenary.debug.tail`, `ws.mercenary.debug.stdout`, `ws.mercenary.debug.stderr`,
`ws.mercenary.debug.runtime_log`, and `ws.mercenary.debug.events`.

`ws.mercenary.interrupt` queues a redirect message for a running agent. `ws.mercenary.print`
remains a deprecated compatibility reader over the resolved current instance.
`ws.mercenary.erase` removes or hides the resolved role pointer for the current
worktree and actor scope; historical instance payloads are removed later by the
named-agent retention cleanup policy rather than synchronously during erase.

## Mercenary Delegation Surface {#260610-mercenary-delegation-surface}

The reshaped delegation surface. A **mercenary** is a ws-spawned external
subprocess agent — a deliberately distinct term from a harness-native
**subagent**, so callers never confuse the two delegation paths. This section is
the caller-visible contract for the reshaped `ws.mercenary.*` family.

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

> [!note]
> `prefer_mercenary` is a `session`-default item in the layered config scope
> model (`#260619-layered-config-scope-model`) with desired-state get/set: the
> lead can both enable and disable it on the same session key, replacing the
> former one-way flip. It stays lead-only — the scope-aware setter honors the
> existing lead-only gating. Mercenary *availability* is unchanged; only the
> default-guidance toggle gains a revert path. {#260619-prefer-mercenary-session-scope-item}

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
(`#260609-playbook-tools`); there is no `register(prompts: [stems])` step. The
playbook's first-class frontmatter `tier:` is surfaced by `playbook.render` as a
recommended tier and passed through to `ws.mercenary.register`'s `tier` arg, which
selects the mercenary's model via config — the caller never hand-picks a workload
tier. A
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

The retired agent-backed API documentation tools are not exposed by full ws:
`api.ask`, `api.ask_async`, `api.status`, `api.result`, and `api.cancel` are
unknown tools and absent from runtime capability metadata.
{#260508-api-documentation-async-mcp-tools}

The remaining `api.list` behavior is limited to deterministic read-only local
cache discovery. Workflow guidance routes external dependency/API documentation
questions through scoped native exploration or direct official documentation
lookup until a future pure-tooling `api.*` namespace is designed.

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
exec runtime metadata into SQLite authority. The gate keeps public `ws.mercenary.*`
and `exec.*` MCP APIs stable while separating lifecycle metadata from
file-backed payload bodies. Named-agent registry metadata and exec job metadata
are SQLite-backed. SQLite metadata may track identities, lifecycle state,
session binding, path indexes, byte counts, retention visibility, leases,
tombstones, and prune bookkeeping. Prompts, streams, runtime logs, event JSONL,
transcripts, backend raw output, and final output bodies remain file-backed.

SQLite state-store configure, migration, and short write paths use bounded
retry for `SQLITE_BUSY` and `SQLITE_LOCKED` conditions while retaining
process-local write serialization. Runtime migrations must keep transactions
short and must not hold a transaction across subprocess or model execution.

## Tool Profile Gating {#260505-tool-profile-gating}

The MCP server defaults to the `lead` tool surface, and `tools/list` advertises
the full lead surface regardless of any caller environment. Schema visibility is
advisory, not an authority boundary, because plugin-managed hosts can start the
server from cache directories and can fail to propagate environment variables
consistently.

Tool-permission enforcement is the server-side capability check in the keyed
`tools/call` handler. A session key carries `{root + capability scope}` —
`lead`, `delegate`, or `leaf` — minted by `ws.ferrule(capability)` or as a
render-minted child key. When a call presents a known non-lead key, the handler
rejects any tool that scope disallows (`delegate` cannot call `ws.mercenary.*`,
`config.*`, or `session.*`; `leaf` additionally cannot call `git.commit`) and
rejects any lead-only tool from any non-lead key — the `ws.lead.*` prefix plus
the bootstrap tool `ws.ferrule` matched by explicit name (self-bootstrap
escalation block). The retained `api.list` cache-domain discovery tool is read-only and
leaf-callable; the retired agent-backed API ask tools are absent from the
surface rather than blocked by scope. Keyless callers and lead keys are not
restricted by this gate,
so the keyless `ws.ferrule` bootstrap stays open; a delegate can therefore
keyless-re-`login` to re-escalate. The scope is a soft defense-in-depth guard
layered on the host's own subagent tool restriction, not a hard sandbox.

`WS_MCP_TOOL_PROFILE` no longer gates the served tool surface and is not
propagated to spawned mercenary subprocesses; the env-profile role mechanism is
retired in favor of the keyed capability gate, having been verified
non-functional for containment (it was lost whenever the host failed to propagate
the env var). Delegate tool scope now travels in-band through the render-minted
child key rather than the environment. `WS_MCP_ALLOWED_TOOLS` is retained as an
optional visibility allowlist for tests and debugging, independent of capability
scope: it can narrow the visible surface but cannot expand access beyond what the
keyed gate permits.

## CLI Mirror Coverage {#260505-cli-mirror-coverage}

The `ws-mcp` binary mirrors selected MCP behavior as CLI commands for smoke
tests, compatibility probes, and fallback usage.

CLI mirrors exist for runtime info, single-process smoke checks, config, path
generation, named agents, Git, tickets, specs, selected mental-model
discovery, and reference tracing. Not every MCP tool has a CLI mirror; the MCP
surface is the canonical host-neutral interface, and CLI coverage is limited to
the surfaces needed for runtime checks and workflow fallback use.
