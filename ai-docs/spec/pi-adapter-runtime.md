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
the bridge surface, the delegation spawner (persistent RPC worker children with
bounded depth ≤ 2), the user-curated model catalog alias table, and the
`/ws-discuss` proof-of-concept command.

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

The adapter answers Pi's `resources_discover` event with the path to a ws skills
tree, so ws skills load as native Pi skills with no prose rewriting. The path is
resolved **package-local-first**: the adapter prefers a `skills/` directory inside
its own package root (present in a published/installed tarball, generated at pack
time — see Package topology) and falls back to the canonical monorepo
`agents-plugin/skills/` tree for dev `-e` runs from the source checkout. Either
way a single existing directory is handed to Pi; the fallback returns the
canonical path unconditionally, so a checkout missing both simply exposes no
skills rather than failing. ws skill directory names are already hyphen-form
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
ws-mcp launcher) as a child process per delegated worker. Each worker is a
**persistent, driveable child**: it is launched in Pi's RPC mode (a long-lived
`RpcClient`, `pi --mode rpc`), so the lead can send follow-up messages into a
running or dormant child rather than the one-shot `-p`-per-task model. Each worker
still runs as its own child process, preserving out-of-process isolation. The
spawn tool is a **thin launcher**: the lead renders the playbook itself (surfacing
the tier/model recommendation at render time), so the spawn tool carries no tier
abstraction. Five tools are registered:

- `ws-agent-spawn({ system_prompt_path, prompt, model_name?, model_effort? })
  -> { agent_id }` — start a persistent worker. `system_prompt_path` is the
  lead-rendered playbook file, passed as `--append-system-prompt`; `prompt` is the
  raw task text, delivered as the child's first turn. `model_name` is an optional
  alias resolved through the catalog (see below) to `--model`, omitted → inherit
  the parent's model; `model_effort` is an optional reasoning-effort override
  applied after launch. The call returns an `agent_id` immediately; the worker runs
  in the background. Every child is launched with an explicit CLI path, so it
  resolves the installed `pi` entry with no bare-`pi` fallback.
- `ws-agent-send(agent_id, message, interrupt?)` — send a message into a child.
  The delivery mechanism is chosen from the child's state: a message starts a new
  turn on an idle child, `interrupt: true` steers an actively streaming child
  mid-run, and a non-interrupt message during an active run queues after the
  current turn. A message to a **dormant** child auto-resumes it from its on-disk
  session (keeping the same ws `session_key`) and then delivers — so resume is
  subsumed by send, and there is no separate continue tool.
- `ws-agent-wait(agent_ids[], timeout?)` — block until the FIRST child in the set
  reaches idle **or** emits a report (see the report channel below), then return
  it. Idle harvest is edge/consume: a child returned as idle is consumed, so a
  later wait on an array still listing it does not busy-return it and later
  finishers are not starved. The result carries `reason: idle | report` plus any
  pending reports for the woken child (drained in FIFO order), and an idle wake
  additionally carries the child's last message. On `timeout` expiry with no
  finisher, the call returns a timed-out marker and leaves every child registered;
  an empty `agent_ids` list is a caller error and fails fast.
- `ws-agent-list()` — enumerate live children with their status.
- `ws-agent-stop(agent_id)` — halt a child's process while retaining its registry
  mapping and on-disk session, leaving it **dormant/resumable** (a later
  `ws-agent-send` revives it). This is distinct from `session_shutdown`, the
  terminal teardown of every child.

Still-running workers are terminated when the session is torn down
(`session_shutdown`), before the bridge connection they dispatch through is
closed.

### Turn completion is gated on RPC idle {#260903-pi-spawner-completion-gating}

Because a worker is now a persistent RPC child rather than a one-shot process, a
turn's completion is signalled by the child reaching **idle** (its RPC
`agent_settled` event), not by the child process closing its stdio. `ws-agent-wait`
races that idle signal across the waited set; the child stays alive after settling,
ready for the next `ws-agent-send`. Streaming-vs-idle state is tracked per child so
a send is routed correctly (start a new turn / steer / queue). A spawn that fails
to launch (bad interpreter, missing binary) settles its waiters rather than
hanging, and a set of children with no live process and no pending idle in an
untimed wait fails fast rather than blocking forever.

### explore — one-shot recon leaf {#260903-pi-explore-recon-leaf}

`explore({ query, async? })` is a thin one-shot preset over the same engine for
ephemeral read-only reconnaissance: a fixed `explore` playbook, the `recon` tool
group, `--no-session` (no continuation state), and self-reaping (the registry
entry is dropped once the leaf completes). An `explore` leaf has no `continue`
path, and its own `recon` allowlist excludes `explore` and every `ws-agent-*`
tool, so an explore leaf spawns neither another explore nor a worker — it is the
non-recursive terminal of the delegation tree (see bounded depth below).
`explore` is the one delegation tool a worker itself may reach. Because the
persistent-child `ws-agent-wait` tracks only RPC children, a one-shot
`explore({ async: true })` leaf is not harvestable through it; the common
synchronous `explore` is unaffected.

### Per-spawn tool curation {#260903-pi-spawner-tool-groups}

The `--tools` allowlist for each spawn resolves from an adapter-owned tool-group
table — `read-only`, `recon`, and `full-worker` — mapping each group to a Pi
tool-name allowlist. Built-in Pi tools are named directly; the `full-worker` group
additionally includes the bridge's live `ws__*` tool names, taken from the running
bridge rather than hardcoded so the group tracks the actual ws-mcp tool set. A
worker's `full-worker` allowlist **excludes every delegation-driving tool**
(`ws-agent-spawn` / `-send` / `-wait` / `-list` / `-stop`), so a worker cannot
spawn or drive a further generation of persistent workers, but it **includes the
literal `explore` tool** — a pi-native custom tool, not a `ws__*` bridge name, so
it must be named explicitly to survive Pi's `--tools` allowlist — so a worker may
spawn a read-only recon leaf. No agent-profile files are written to disk (no
`.pi/agents/`); all curation is in-memory plus `pi` CLI flags.

### Bounded delegation depth {#260904-pi-spawner-bounded-depth-explore-leaf}

The delegation tree terminates at depth 2 (lead → worker → explore leaf). A
worker's `--tools` allowlist admits `explore` but no delegation-driving tool, and
the `explore` leaf's own `recon` allowlist admits neither `explore` nor any
`ws-agent-*` tool, so no branch of the tree extends past an explore leaf. This is
enforced entirely by the adapter's per-spawn `--tools` allowlists; the ws-mcp
core's own keyed-handler role check is untouched.

### Model resolution: name alias, not tier {#260903-pi-spawner-model-tier-inherit}

`ws-agent-spawn` carries **no `tier`** parameter. Model selection is the lead's
call at render time: `ws__playbook_render` already returns a config-resolved
`recommended-tier` / `recommended-model` / `recommended-reasoning-effort`, and the
lead either passes `model_name` (and optionally `model_effort`) to the spawn or
omits them to inherit the parent's model. When `model_name` is given it is resolved
through the catalog's alias table (see below) to a concrete `provider/id` and
launched as `--model <that model>`; an unknown alias, an empty table, or an omitted
`model_name` all degrade to inheriting the parent's active model. Resolution never
hard-fails. `model_effort`, when given, is applied to the launched child through a
post-launch reasoning-effort call rather than a launch flag; an unsupported value
is a no-op, never an error.

`explore` is a **role**, not a caller-facing model choice: it resolves its own
model through the catalog and exposes no `model_name` parameter.

### Model catalog data file {#260903-pi-model-catalog-config-file}

The curated model aliases live in an adapter-owned data file,
`agents-plugin-pi/model-catalog.json` (sibling to `runtime.json`) — no Pi model
strings are placed in the harness-neutral ws-mcp core. Its shape is an `aliases`
object mapping a **generic model name** (e.g. `opus`, `sonnet`) to a concrete
`provider/id` model string, plus an optional `catalog` list of curated candidate
models. A `model_name` passed to `ws-agent-spawn` is resolved name → `provider/id`
through this table; this replaces the earlier tier→model map, since the spawn tool
no longer takes a tier. The file ships as `{}` (empty alias table) so a fresh
checkout starts with every spawn inheriting until the user curates it. It is read
**fresh on every spawn** (no caching), so a hand-edit applies without restarting
Pi; a missing or malformed file is treated as an empty table rather than an error.

The read-only `ws-model-catalog-list` command enumerates the session's usable
models (`ctx.scopedModels`, falling back to the full available pool when no
scoping is configured) as `provider/id` candidates for the user to hand-copy into
`model-catalog.json`. It never writes the file.

### Unset-catalog advisory on workflow_manual {#260903-pi-model-catalog-unset-advisory}

While the catalog's alias table is empty, the adapter appends a strong advisory
to every `workflow_manual` response (and only that tool's response), mirroring the
cadence of the ws-mcp core's bootstrap-version-behind advisory — recomputed and
re-appended on every call while the condition holds, not once per session. The
advisory is appended after the tool's own content (never prepended, never mutating
the original in place) and is added only on a successful `workflow_manual` result,
never on an error response. Spawns and explores still degrade silently to inherit
while the table is empty; the advisory is the only pressure and never blocks work.

### Child→lead report channel {#260904-pi-report-to-lead-channel}

A worker can push an out-of-band message to its lead mid-run through the child-side
tool `ws-report-to-lead(message)` — the only child→lead tool the delegation layer
adds, and included in the `full-worker` `--tools` allowlist so a worker can reach
it. It needs no new transport: the call surfaces to the parent on the child's
existing RPC event stream (the tool-invocation event), and the adapter routes it
into a **per-agent report buffer**. Because the report rides the invocation event,
it reaches the lead as soon as the model calls the tool, independent of the tool's
own return value.

The buffer is a bounded FIFO, default capacity 32. On overflow the oldest report is
dropped and a per-agent dropped-count is incremented, so a lead that later drains
the buffer sees a truncation marker rather than silently losing reports. On each
`ws-agent-wait` wake for that child, **all** pending reports drain at once in FIFO
order (a woken lead sees the whole queue, not one at a time) and the count resets;
a report that arrives while no wait is pending simply buffers until the next wait.
Reports survive `ws-agent-stop` (they are not cleared when a child goes dormant),
so a stopped-then-resumed child does not lose an unread report.

### Transcript path accessor {#260904-pi-agent-transcript-path}

`ws-agent-transcript(agent_id) -> { transcript_path }` returns the filesystem path
to the child's Pi session JSONL transcript. It marshals no transcript content — the
lead greps or reads the file with its own filesystem tools. This is an
advanced/rare accessor; it is registered as a tool but is not in any worker
`--tools` group, so a worker cannot call it.

## Proof-of-concept command {#260903-pi-poc-discuss-command}

The adapter registers one proof-of-concept command, `/ws-discuss`, via
`pi.registerCommand` — the MVP gate that demonstrates the three adapter surfaces
(skill exposure, the ws-mcp bridge, the delegation spawner) composing in a single
end-to-end run. It is registered at the extension-factory top level alongside
`ws-model-catalog-list` (command/tool registration is declarative and not gated
behind `session_start`; only subprocess spawning is).

The command is a thin kickoff, not an imperative workflow driver:

- When the agent is not idle (`ctx.isIdle()` is false), it declines with a
  `ctx.ui.notify` warning and does nothing else — mirroring Pi's own
  `send-user-message` example, so the plain (no `deliverAs`) send below is always
  safe.
- When idle, it calls `pi.sendUserMessage(kickoff, { expandPromptTemplates: true })`
  with a single kickoff string, then returns. The command triggers model work; it
  does not run the bridge or spawner itself.

The kickoff string is produced by a pure, unit-tested builder
(`buildDiscussKickoff(args)`), so its exact shape is a fixed contract rather than
incidental prose. It has two parts:

- It **leads** with `/skill:lead-discuss <topic>`. Under
  `expandPromptTemplates: true`, Pi expands that leading token into the
  `lead-discuss` skill body (skills-load), and everything after the token on that
  line becomes the skill's `User:` args. When the caller passes no argument, a
  fixed default PoC topic is substituted so a bare `/ws-discuss` is still a valid
  gate invocation. The `lead-discuss` skill body itself calls the bridged
  `ws__playbook_print` / `ws__workflow_manual` tools, so skills-load transitively
  drives the bridge with no imperative tool call in the handler.
- It **appends**, after a blank-line separator, an explicit instruction to
  dispatch one `explore` recon leaf and report its result. The blank line keeps
  this instruction off the skill-command line (so it does not corrupt the
  `User:` args split). This append is load-bearing: the discuss skill does not
  itself spawn, so the spawn round-trip that the gate requires is not inherent to
  skills-load + bridge — the kickoff must name it explicitly to make the
  spawn deterministic.

Because the gate proof is a live model-driven run (the model reads the kickoff
and issues the bridged and spawner tool calls itself), it is verified the same
way the Phase 2–3 gates were — a `pi -e … --mode json -p` transcript — not a unit
assertion. The unit tests pin only the kickoff wording that steers that run; the
command handler's `ctx`/`pi` glue is left untested, matching the
`ws-model-catalog-list` precedent.

## Package topology {#260903-pi-adapter-package-topology}

The adapter lives in `agents-plugin-pi/`, a sibling package root parallel to
`agents-plugin/`, `agents-plugin-tool/`, and `agents-plugin-wsflow/`. It is
self-contained: it carries its own byte-identical copies of the ws-mcp launcher
(`bin/ws-mcp-launcher.py`), the runtime compatibility contract (`runtime.json`),
and the prompt/playbook tree (`rsrc/`) — the same copy-not-reference precedent the
`agents-plugin-wsflow` package already uses, and required because the launcher
resolves those trees relative to its own package directory at runtime. These three
copies are kept in sync by hand; there is no automated sync tooling, so a change
to the canonical `agents-plugin/` copies must be mirrored here.

The ws skills tree is a fourth carried copy, but with a distinct, **automated**
sync model rather than a hand-synced commit: a pack-time script (wired to npm
`prepack` and `prepare`) copies `agents-plugin/skills/` into a package-local
`skills/` directory that is shipped in the published tarball (via the `files`
whitelist) yet gitignored and never committed. This keeps the large skills tree
out of the repository while still making an installed package self-contained; the
package-local-first resolver (see Skill exposure) prefers this generated copy and
uses the canonical tree only for dev `-e` runs. The copy script is Node-builtins
only, so it runs under a consumer's `npm install --omit=dev`, and no-ops when the
canonical source is absent (packing from an already-vendored tarball). `npm pack`
fires both hooks, so the copy runs redundantly but idempotently.

> [!note] Constraints
> - This contract covers the bridge, the delegation spawner (upgraded to
>   persistent RPC children with bounded depth ≤ 2, a child→lead report channel,
>   and a path-only transcript accessor), the model catalog alias table, and the
>   `/ws-discuss` PoC command. Post-MVP surfaces still deferred to follow-up
>   tickets under the epic — an always-visible TODO, the goal-loop, and compaction
>   hooks — are not part of this contract yet.
