---
title: Pi Adapter Runtime Contract
summary: How the ws Pi extension bridges the harness-neutral ws-mcp server onto Pi (earendil-works) — tool exposure, session keying, version pinning, skill discovery, and process lifecycle.
---

# Pi Adapter Runtime Contract

The ws Pi adapter (`agents-plugin-pi/`) is a self-contained Pi extension that
makes the harness-neutral ws-mcp server usable from Pi. It spawns the ws-mcp
launcher as a child process, speaks MCP JSON-RPC over its stdio, and re-exposes
every ws-mcp tool as a Pi tool. The dependency is one-directional (adapter →
ws-mcp); no ws-mcp source is modified for Pi. All Pi-specific policy lives in the
adapter.

This document describes the caller-observable behavior of the adapter. It covers
the Phase 1 bridge surface and the Phase 2 delegation spawner; the user-curated
model catalog is a later surface that adds its own section.

## Tool exposure and name sanitization {#260903-pi-bridge-tool-registration}

Every tool the ws-mcp server advertises through `tools/list` is registered as a
Pi tool. ws-mcp tool names are bare and dotted (`playbook.print`, `tickets.list`,
`workflow_manual`, `ferrule`). Pi tool names are serialized into the model
provider's tool-call payload, and common provider wire formats (OpenAI-compatible
function calling) reject names containing `/` or `.`, so the registered name is
**sanitized to a provider-legal identifier** matching `^[a-zA-Z0-9_-]+$`:

- The `ws/` namespace separator becomes `__` (double underscore).
- Each `.` within the raw name becomes `_` (single underscore).
- Equivalent form: `registered = "ws__" + rawName.replace(all ".", "_")`.
- Examples: `playbook.print` → `ws__playbook_print`, `tickets.list` →
  `ws__tickets_list`, `workflow_manual` → `ws__workflow_manual`, `ferrule` →
  `ws__ferrule`.

Sanitization is **registration-only**. When a registered tool executes, the
bridge dispatches to ws-mcp using the original untouched dotted name, so ws-mcp
never observes the sanitized form. Skill and playbook prose is likewise never
rewritten: `SKILL.md` text keeps writing calls in the canonical `ws/playbook.print(...)`
notation, and the model maps that prose onto the sanitized registered tool the
same way the reference harnesses already do (on Claude Code these tools appear as
`mcp__plugin_ws_ws__playbook_print`, and the model bridges the two).

A ws-mcp tool result whose envelope carries `isError: true` is surfaced to Pi as
a tool failure (a thrown execution), not as a successful result — Pi sets a
tool's error state only when its `execute` throws, so a ws-mcp failure that was
returned as ordinary text is re-raised rather than reported as success.

## Session key stays optional and caller-controllable {#260903-pi-bridge-session-key-fill-forward}

ws-mcp requires a `session_key` on every root-aware tool. On the Pi side the key
stays an **optional, caller-controllable** parameter — it is never stripped from
the caller's view:

- When a call omits `session_key`, the bridge fills in its own default key,
  minted once at startup via `ferrule` against the session's working root.
- When a call supplies an explicit `session_key`, the bridge forwards it
  verbatim; it is not overwritten by the default. This preserves both subagent
  parent→child key lineage and lead multi-track orchestration, where distinct
  keys must reach ws-mcp unchanged.

Because Pi validates tool-call arguments against the registered parameter schema
*before* the tool executes, and ws-mcp advertises `session_key` as a required
property, the bridge relaxes each registered schema so `session_key` is listed in
`properties` but **not** in `required`. Without this, Pi's own validator would
reject an omitted-`session_key` call before the fill-or-forward logic ran,
defeating the optional-key contract. An explicit key still validates and flows
through unchanged.

If the startup `ferrule` bootstrap fails, the default key is left unset rather
than faked; a later omitted-`session_key` call then surfaces ws-mcp's own
`mandatory_session_key` guidance instead of a swallowed error.

## Startup version pin-and-fail {#260903-pi-bridge-version-pin}

The adapter pins itself to a specific ws-mcp build. The `initialize` handshake
returns `serverInfo.version`; the adapter compares it against the `plugin_version`
recorded in the adapter's own bundled `runtime.json`. On mismatch the extension
fails loudly at load — it raises synchronously, registers no tools, and does not
silently fall back to a partially-compatible server. The check reuses the value
already returned by the handshake, so it costs no extra round-trip.

## Skill exposure {#260903-pi-bridge-skill-exposure}

The adapter answers Pi's `resources_discover` event with the path to the ws
`agents-plugin/skills/` tree, so ws skills load as native Pi skills with no prose
rewriting. ws skill directory names are already hyphen-form
(`lead-add-rule`, `lead-proceed`, …), which matches Pi's skill-name charset, so
no renaming is required.

## Process lifecycle {#260903-pi-bridge-subprocess-lifecycle}

The ws-mcp child process is bound to a Pi session, not to extension load:

- It is spawned when a session starts (`session_start`), never at module load —
  Pi forbids starting background processes from the top-level extension factory.
- It is terminated when the session is torn down (`session_shutdown`); the
  shutdown path is idempotent against double invocation.
- A spawn failure (missing interpreter, bad launcher path, failed runtime
  install) fails loudly and promptly: the pending `initialize` and any in-flight
  requests are rejected rather than left hanging, since a failed spawn emits no
  normal exit event.

The stdio transport reads the child's stdout as newline-delimited JSON-RPC (one
message per line, no Content-Length framing) and decodes it so that multibyte
UTF-8 characters split across read-buffer boundaries are reconstructed intact.
Concurrent in-flight requests are correlated back to their callers by JSON-RPC
id, independent of the order responses arrive.

## Delegation spawner {#260903-pi-delegation-spawner-tools}

The adapter exposes a Pi-side delegation layer built on the same self-owned
subprocess machinery as the bridge, but spawning the **`pi` CLI itself** (not the
ws-mcp launcher) as a child process per delegated worker. Four tools are
registered:

- `ws-agent-spawn({ playbook, task, tier? })` — start an asynchronous worker. The
  adapter renders the named ws playbook to a system-prompt file (via the bridged
  `ws__playbook_render`), launches `pi --mode json -p` with that file as
  `--append-system-prompt`, a per-spawn `--tools` allowlist, and a ws-owned
  `--session` file, then returns an `agentId` immediately without waiting for the
  worker to finish. The worker runs in the background.
- `ws-agent-continue({ agentId, task })` — resume a completed worker with a new
  task on its existing session. Allowed only when the worker has reached the done
  state; the same session file and system-prompt file are reused (the prompt is
  not re-rendered, so it is not duplicated across turns).
- `ws-agent-wait({ agentIds, policy, timeout? })` — harvest workers. `policy:
  "any"` returns once at least one named worker is done; `policy: "all"` returns
  once all are. The result partitions the ids into done / still-running /
  timed-out. A timeout never kills a running worker — it stays registered for a
  later wait or continue. An empty `agentIds` list is a caller error and fails
  fast rather than blocking.

Depth is bounded 0→1: none of the four delegation tools appears in any spawnable
worker's own `--tools` allowlist, so a depth-1 worker cannot spawn a further
generation. Still-running workers are terminated when the session is torn down
(`session_shutdown`), before the bridge connection they dispatch through is
closed.

### Worker completion is gated on process exit {#260903-pi-spawner-completion-gating}

A worker flips to the done state only when its `pi` child process closes its
stdio, **not** when a terminal `stopReason` is first observed in the streamed JSON
event log. The terminal `stopReason` set is exactly `stop` / `length` / `error` /
`aborted` (`toolUse` and `pending` are non-terminal). The `--session` file is only
guaranteed fully flushed after the child exits, so gating completion on process
close is what makes an immediate `ws-agent-continue` right after `ws-agent-wait`
safe — the resumed turn always reads a complete session file. The last-seen
`stopReason` is retained only as completion metadata. A spawn that fails to launch
(bad interpreter, missing binary) settles its waiters rather than hanging.

### explore — one-shot recon leaf {#260903-pi-explore-recon-leaf}

`explore({ query, async? })` is a thin one-shot preset over the same engine for
ephemeral read-only reconnaissance: a fixed `explore` playbook, the `recon` tool
group, `--no-session` (no continuation state), and self-reaping (the registry
entry is dropped once the leaf completes). An `explore` leaf has no `continue`
path.

### Per-spawn tool curation {#260903-pi-spawner-tool-groups}

The `--tools` allowlist for each spawn resolves from an adapter-owned tool-group
table — `read-only`, `recon`, and `full-worker` — mapping each group to a Pi
tool-name allowlist. Built-in Pi tools are named directly; the `full-worker` group
additionally includes the bridge's live `ws__*` tool names, taken from the running
bridge rather than hardcoded so the group tracks the actual ws-mcp tool set. No
agent-profile files are written to disk (no `.pi/agents/`); all curation is
in-memory plus `pi` CLI flags.

### Model tier is inherit-only at this stage {#260903-pi-spawner-model-tier-inherit}

`ws-agent-spawn` accepts a `tier`, but the adapter does not yet map a tier to a
concrete model. A spawn resolves its model by inheriting the parent session's
active model (equivalently, omitting `--model` for Pi's own default). The `tier`
value is carried as inert metadata pending the model-catalog surface.

> [!note] Implementation Gap · 2026-09-03
> Unexposed capability: `ws-agent-spawn`'s `tier` argument is accepted but not yet
> resolved to a model. The user-curated model catalog and tier→model map are a
> separate, not-yet-built surface; until it lands every spawn inherits the parent
> model regardless of `tier`.

## Package topology {#260903-pi-adapter-package-topology}

The adapter lives in `agents-plugin-pi/`, a sibling package root parallel to
`agents-plugin/`, `agents-plugin-tool/`, and `agents-plugin-wsflow/`. It is
self-contained: it carries its own byte-identical copies of the ws-mcp launcher
(`bin/ws-mcp-launcher.py`), the runtime compatibility contract (`runtime.json`),
and the prompt/playbook tree (`rsrc/`) — the same copy-not-reference precedent the
`agents-plugin-wsflow` package already uses, and required because the launcher
resolves those trees relative to its own package directory at runtime. These
copies are kept in sync by hand; there is no automated sync tooling, so a change
to the canonical `agents-plugin/` copies must be mirrored here.

> [!note] Constraints
> - This contract covers the Phase 1 bridge and the Phase 2 delegation spawner.
>   The user-curated model catalog + tier→model map (which would resolve
>   `ws-agent-spawn`'s `tier` to a concrete model) and the `/ws-discuss` PoC
>   command are separate, not-yet-implemented surfaces and are not part of this
>   contract yet.
