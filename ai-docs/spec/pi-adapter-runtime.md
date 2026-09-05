---
title: Pi Adapter Runtime Contract
summary: How the ws Pi extension bridges the harness-neutral ws-mcp server onto Pi (earendil-works) — tool exposure, session keying, version pinning, skill discovery, and process lifecycle.
---

# Pi Adapter Runtime Contract

The ws Pi adapter (`agents-plugin-pi/`) is a self-contained Pi extension that
makes the harness-neutral ws-mcp server usable from Pi. It spawns the ws-mcp
launcher as a child process, speaks MCP JSON-RPC over its stdio, and re-exposes
every ws-mcp tool as a Pi tool. The dependency is one-directional (adapter →
ws-mcp); ws-mcp carries no Pi-specific logic. Its harness-keyed config surfaces
(the closed harness enum, `agents.tier`, prompt overrides, rsrc harness
variants) may treat `pi` as a peer of Codex/Claude — the harness-peer clause in
`AGENTS.md` (owner, 2026-09-05), which applies to any later host as much as to
Pi — and all Pi-specific policy lives in the adapter.

This document describes the caller-observable behavior of the adapter. It covers
the bridge surface, the delegation spawner (persistent RPC worker children with
bounded depth ≤ 2), the user-curated model catalog alias table, and the
`/ws-discuss` proof-of-concept command.

## Tool exposure and name sanitization {#260903-pi-bridge-tool-registration}

Every tool the ws-mcp server advertises through `tools/list` is registered as a
Pi tool, **except** the mercenary surface: the bridge drops every raw tool name
starting with `mercenary.` (e.g. `mercenary.register`, `mercenary.call`,
`mercenary.debug.tail`) from the list before registration and before building
`wsToolNames`, independent of the server-side `workflow.prefer_mercenary`
visibility knob — so no Pi process, lead or child, can see or call the
mercenary surface (Open Decision #3, `260905-feat-ws-pi-harness-config-layer`).
ws-mcp tool names are bare and dotted (`playbook.print`, `tickets.list`,
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

In front of that fill-or-forward rule the bridge runs a **narrow, mechanical
key normalization** that rewrites exactly two explicit-key values — both
recognized by string equality against values the adapter knows out-of-band —
to the bridge's own default key:

| explicit `session_key` value | rewritten to | why |
| --- | --- | --- |
| the fresh-bootstrap sentinel `obsidian-latch` | own key | a lead that follows the canonical skill text literally passes the FRESH sentinel; forwarding it would make ws-mcp mint a **second** lead key while the bridge already holds one, splitting the session's ws state across two keys |
| the **parent lead's key** (present only in a process spawned as a side-thread fork, delivered through the spawn environment) | own key | the fork inherits a transcript that names the lead's key, and ws-mcp keys agenda/todos per key, so forwarding it would clobber the lead's state |

Every other explicit key — including a lead driving a child by that child's
key — passes through untouched; widening the rewrite beyond these two cases is
rejected. The rewrite is a pure function
(`normalizeSessionKey(params, { ownKey, sentinel, parentLeadKey? })`) applied
strictly before fill-or-forward, and is a bridge-layer mechanism, never a prompt
instruction. When the startup `ferrule` bootstrap fails and the own key is unset
(see below), the sentinel rewrite is **disabled** so the model's own FRESH call
self-heals; the parent-key case is likewise a no-op when no parent key is present.

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

## Lead bootstrap: workflow manual + Pi lead guide in the system prompt {#260905-pi-lead-bootstrap-system-prompt}

On the reference harnesses (Claude, Codex) the ws workflow manual reaches the
lead only through a model-invoked `workflow_manual` tool call carrying a
session-key handshake, because those hosts give the plugin no hook on the lead's
system prompt. On Pi the adapter owns the extension, so the manual is injected
directly into the lead's system prompt and the handshake becomes unnecessary.

- Hook: the adapter appends a **ws block** to the lead's system prompt on every
  agent run (Pi's `before_agent_start`, whose result may return a `systemPrompt`
  chained across extensions). The extension **appends, never replaces**: it
  returns the incoming `systemPrompt` followed by the ws block. Because Pi
  re-assembles the system prompt from its base on every turn, the handler
  re-applies the block each turn from an in-memory snapshot rather than
  capturing it once.
- The ws block content, in order:
  1. the **full `workflow_manual` CONTINUE response as of session start** — the
     static manual body plus ws-mcp's session-start dynamic material (`## Session
     Key`, `## Session State`, repo notes, and the per-call advisory blocks) —
     obtained by the bridge itself right after the `ferrule` bootstrap by calling
     `workflow_manual` under its own key, and prefixed by one fixed line marking
     the dynamic part as a **session-start snapshot** whose current values come
     from a later `workflow_manual` call. The manual text has a single source
     (ws-mcp's render); the adapter never carries a copy of it.
  2. the **Pi lead guide** — an adapter-owned prose file
     (`agents-plugin-pi/pi-lead-guide.md`, shipped in the package `files`
     whitelist so an installed tarball carries it) describing the Pi-specific
     lead surface: that `session_key` never needs to be supplied, that the manual
     is already present so a `workflow_manual` call returns only its dynamic part,
     and a **verb routing table** with one row per Pi lead verb. The guide is
     structured so later tickets extend the verb table with their own rows rather
     than rewriting shared text.
- Fetch cadence: the block is assembled **once per session start** and held in
  extension memory; it is not re-fetched per turn. The dynamic material is
  refreshed only when an entry-point skill calls `workflow_manual` (the same
  cadence as on the reference harnesses). Injecting the manual into the system
  prompt — rather than as a transcript message — is deliberate: the system prompt
  survives Pi compaction natively, so post-compaction recovery reduces to a
  `workflow_state`/`workflow_manual` call.
- Role gating: the ws block is appended only for the **lead** (no spawn role in
  the environment) and for a **`fork`** (a lead-caliber peer that needs the same
  manual and guide). It is appended for neither `worker` nor `explore` children —
  they receive their own rendered playbook via `--append-system-prompt`, and the
  lead manual would reintroduce the lead's delegation posture into a worker. The
  session-start snapshot fetch is likewise gated to lead/fork, so worker/explore
  processes never pay for it. A user-launched headless lead (`--mode rpc`) has no
  spawn role and receives the block exactly like the TUI lead; nothing here
  depends on `ctx.ui`.
- Degraded path: if the `session_start` `ferrule` bootstrap or the manual fetch
  fails, the own key or the snapshot is left unset and the ws block is simply not
  injected for that session; the model still reaches ws-mcp through the ordinary
  tool path.

## `workflow_manual` calls are mapped onto `workflow_state` {#260905-pi-workflow-manual-state-mapping}

Because the manual body already lives in the lead's system prompt (see above), a
model-invoked `ws__workflow_manual` call in a lead or fork process must return
only the **state-and-advisories view** — everything ws-mcp recomputes per call —
and never the manual body a second time:

- Primary: the bridge forwards the call to ws-mcp `workflow_manual` with the
  (normalized) key and **cuts the static manual body out of the response** by
  exact substring match against a static-body snapshot taken at session start
  (the `playbook.print("lead-workflow-manual")` render, which `workflow_manual`
  produces internally). What remains is the per-call material — `## Session Key`,
  `## Session State`, repo notes, and the advisory blocks — so ws-mcp's contract
  that those are recomputed on every call is preserved.
- Fallback: if the snapshot body is not found in the response (the renderer
  changed mid-session), the bridge dispatches ws-mcp `workflow_state` instead —
  a state-only view with no FRESH mode that never mints — and drops the
  `workflow_manual`-only arguments (`root`) from that call, forwarding only the
  session key. `workflow_state`'s own fail-loud error path for an unresolvable
  key is surfaced unchanged.
- The bridge prepends one fixed line to the mapped response — that the workflow
  manual is in the system prompt and this is the current session state — so a
  model expecting the manual is not confused by its absence.
- The unset-tier advisory (below) keeps riding the mapped response with its
  per-call cadence; it is keyed on the registered name the model called, not
  on the wire tool the bridge dispatched.
- Role- and bootstrap-gated: the cut/mapping and the prepended line apply only in
  a lead or fork process **and** only when the session-start static-body snapshot
  exists. A `worker` or `explore` that calls `ws__workflow_manual`, or any process
  whose bootstrap was degraded (no snapshot), has the call forwarded verbatim,
  since its system prompt carries no manual — the model's own call self-heals
  exactly as on the reference harnesses. This gating decision is a pure predicate
  (`shouldMapWorkflowManual(rawName, hasSnapshot, role)`).

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
abstraction. Four tools are registered — there is no blocking wait tool; every
child signal is pushed into the lead session as a message (see "Child→lead
report channel" below):

- `ws-agent-spawn({ system_prompt_path, prompt, model_name?, model_effort?, alias?, title? })
  -> { agent_id, alias?, evicted? }` — start a persistent worker. `system_prompt_path` is the
  lead-rendered playbook file, passed as `--append-system-prompt`; `prompt` is the
  raw task text, delivered as the child's first turn. `model_name` is an optional
  alias resolved through the catalog (see below) to `--model`, omitted → inherit
  the parent's model; `model_effort` is an optional reasoning-effort override
  applied after launch. `alias` and `title` (260905) are optional, purely
  lead-supplied labels — never derived from prompt text — persisted on the
  registry record for `ws-agent-list` and for alias-or-uuid resolution
  everywhere an `agent_id` param is accepted; reusing an alias already held by a
  dormant/idle record silently reassigns it (clearing the old holder's alias,
  keeping its title) while reusing one held by a running or thread-bound record
  rejects the spawn instead of stealing it out from under it. The registry is
  capped (`WS_PI_AGENT_REGISTRY_CAP`, default 256); a spawn that would exceed
  the cap evicts the oldest fully-dormant, non-thread-bound record(s) first and
  reports the evicted label(s) back as `evicted`, rejecting only if every
  remaining record is running or thread-bound. The call returns immediately;
  the worker runs in the background. Every child is launched with an explicit
  CLI path, so it resolves the installed `pi` entry with no bare-`pi` fallback.
- `ws-agent-send(agent_id, message, interrupt?)` — send a message into a child.
  `agent_id` accepts either the alias or the raw uuid; `ws-agent-stop`,
  `ws-agent-transcript` and `ws-approve` resolve it the same way, through one
  shared helper (260905). The delivery mechanism is chosen from the child's
  state: a message starts a new turn on an idle child, `interrupt: true` steers
  an actively streaming child mid-run, and a non-interrupt message during an
  active run queues after the current turn. A message to a **dormant** child
  (this now includes one the adapter parked automatically after it settled —
  see "Turn completion is gated on RPC idle") auto-resumes it from its on-disk
  session (keeping the same ws `session_key`) and then delivers — so resume is
  subsumed by send, and there is no separate continue tool.
- `ws-agent-list({ include_prompt? })` — enumerate registry members with their
  status, alias and title. Status vocabulary is `running` / `idle` / `dormant`,
  but `idle` (260905) is now transient rather than a resting state: an idle,
  non-thread-bound record is parked to `dormant` by the adapter shortly after
  it settles (see "Turn completion is gated on RPC idle"), so `idle` is mostly
  observed mid-transition, not as a steady status to poll for. `include_prompt`
  (default `false`) additionally surfaces each record's original spawn prompt,
  stored head-truncated to 4KB.
- `ws-agent-stop(agent_id)` — halt a child's process while retaining its registry
  mapping and on-disk session, leaving it **dormant/resumable** (a later
  `ws-agent-send` revives it) — the same end state the automatic park below
  reaches on its own for a settled, non-thread-bound child. The tool pushes a
  `ws-agent-settled` message with `reason: "stopped"`; adapter-internal stops (a
  thread close, the automatic park itself, the shutdown teardown) are silent.
  This is distinct from `session_shutdown`, the terminal teardown of every
  child.

The former `ws-agent-wait` tool is **removed, not deprecated**: a tool that
blocks the lead is the hazard (the approval-relay deadlock class and the
poll-with-timeout loop both came from it), so the approval-pending wake, the
idle edge-consume flag and the waiter bookkeeping went with it. The lead spawns,
ends its turn, and is woken by the pushed message.

Each spawned child inherits a **process-role marker** in its environment so the
extension running inside it can tell what kind of process it is: `WS_PI_SPAWN_ROLE`
carries `worker` (a `ws-agent-spawn` child), `explore` (a recon leaf), or `fork`
(a lead-caliber side-thread peer); its absence marks the host **lead** process.
A `fork` additionally carries `WS_PI_PARENT_SESSION_KEY` (the lead's key), which
feeds the fork's key-normalization parent-key case and lets the bridge mint the
fork's key with lead lineage. This single marker is the source for both the
system-prompt role gate (only lead and fork receive the ws block) and the goal
loop's lead-only gating (any role present marks a child, whose settle handler
no-ops).

Still-running workers are terminated when the session is torn down
(`session_shutdown`), before the bridge connection they dispatch through is
closed. Their identities are not lost: before the teardown the adapter writes a
sidecar `<leadSessionFile>.ws-agents.json` (the `.ws-threads.json` precedent)
listing every non-thread-bound child, dormant/parked ones included (260905:
capture no longer skips a record for lacking a live client — only
thread-bound records are excluded) — `agent_id`, alias, title, prompt
(head-truncated), role (`worker` / `execute-worker` / `fork`), the exact
resume set the dormant-send path consumes (`sessionPath`, `modelBase`,
`modelEffort`, `systemPromptPath`, `toolGroup`, `explicitTools`,
`wsToolNames`), its state at shutdown (`running` / `idle` / `dormant`) and its
last-report time. On the next `session_start` for the **same** lead session
file (resume, `/reload`) the sidecar is consumed exactly once: each entry is
re-registered as a **dormant** record with its role wiring re-armed (`fork` →
the fork hooks: question report, anti-bleed loop, final report;
`execute-worker` → the approval relay; `worker` → none). One
`ws-agent-orphaned` custom message (`followUp`, `triggerTurn: true`) is pushed
**only when at least one entry was `running`** at shutdown: it lists those
entries with the caveat that a child cut off mid-turn resumes from its last
flushed turn and needs its instruction re-issued, summarizes the idle entries in
one line, and carries the revival hint — `ws-agent-send <id>` auto-resumes a
dormant record from its cached session file, so the hint is literally
executable. When every entry was idle (children that had finished or were
waiting for a relay) nothing is pushed; the re-registered records remain
visible through `ws-agent-list`. What each child was doing is not restated; the
resumed lead's own transcript already has it. A different
session (`/new`) leaves the sidecar beside the old session file to fire on that
session's later resume. Reports are never replayed from the sidecar: they belong
to a process that no longer exists.

### Turn completion is gated on RPC idle {#260903-pi-spawner-completion-gating}

Because a worker is now a persistent RPC child rather than a one-shot process, a
turn's completion is signalled by the child reaching **idle** (its RPC
`agent_settled` event), not by the child process closing its stdio. As the
last step of settle handling — after the idle/final push described below and
after the fork anti-bleed nudge has had its chance to re-prompt the child —
the adapter automatically parks a settled child that is neither thread-bound
nor already running again: it is silently stopped (the same as
`ws-agent-stop`, but with no `ws-agent-settled` push of its own) and becomes
dormant. The park clears the record's live state before the process
teardown begins, so a send that lands mid-park takes the dormant-resume path
rather than racing the stop. So a settled child does **not** stay alive
indefinitely waiting for the next `ws-agent-send`; it is transparently
resumed from its on-disk session the moment one arrives, exactly like a
sidecar-revived orphan. A settle that
ends a turn in which the child sent **no** `kind:"final"` (or `kind:"question"`)
report pushes a `ws-agent-settled` message to the lead with `reason: "idle"`
and the child's last message, so a worker whose playbook never says
`ws-report-to-lead` still signals completion. A turn that did report `final`
releases that report **at this transition** instead: the final is stashed on
the record when the child calls the tool (last one wins within a turn) and
pushed as one `ws-agent-report` carrying `details.settled_reason`
(`idle` / `stopped` / `exited`) when the child leaves the running state, with
no separate `ws-agent-settled`; a silent adapter-internal stop drops the
stash. A turn that reported `question` pushes only the question, and its
settle is silent.
`details.reason` takes one of `idle`, `stopped` (the `ws-agent-stop` tool),
`exited` (process gone, see the liveness backstop in the report channel) or
`spawn-failed` (bad interpreter, missing binary — pushed synchronously from the
failed spawn). Streaming-vs-idle state is tracked per child so a send is routed
correctly (start a new turn / steer / queue); `running` is set the moment the
adapter issues a prompt to the child (every prompt site goes through one
`promptAgent` helper, so there is no spawn→`agent_start` window in which a
just-launched child reads as not running), confirmed by `agent_start`, and
cleared on settle, stop, exit or spawn failure.

### explore — one-shot recon leaf {#260903-pi-explore-recon-leaf}

`explore({ query, async? })` is a thin one-shot preset over the same engine for
ephemeral read-only reconnaissance: a fixed `explore` playbook, the `recon` tool
group, `--no-session` (no continuation state), and self-reaping (the registry
entry is dropped once the leaf completes). An `explore` leaf has no `continue`
path, and its own `recon` allowlist excludes `explore` and every `ws-agent-*`
tool, so an explore leaf spawns neither another explore nor a worker — it is the
non-recursive terminal of the delegation tree (see bounded depth below).
`explore` is the one delegation tool a worker itself may reach. An explore leaf
is not a registry member: it returns through its own tool result, pushes no
message and is outside the pushed status line's running count; the common
synchronous `explore` is unaffected.

### Per-spawn tool curation {#260903-pi-spawner-tool-groups}

The `--tools` allowlist for each spawn resolves from an adapter-owned tool-group
table — `read-only`, `recon`, and `full-worker` — mapping each group to a Pi
tool-name allowlist. Built-in Pi tools are named directly; the `full-worker` group
additionally includes the bridge's live `ws__*` tool names, taken from the running
bridge rather than hardcoded so the group tracks the actual ws-mcp tool set. A
worker's `full-worker` allowlist **excludes every delegation-driving tool**
(`ws-agent-spawn` / `-send` / `-list` / `-stop`), so a worker cannot
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
core's own keyed-handler role check is untouched. The bound is measured from
the **root of the delegation tree**, and a side-thread fork (see "Side-thread
task fork") is lateral rather than a descendant: it inherits the lead's surface,
so it is itself a root of its own worker → explore-leaf tree and consumes no
depth budget of the lead's. A fork cannot fork again, because `ws-fork` is
absent from its surface.

### Model resolution: fixed tier through ws-mcp {#260903-pi-spawner-model-tier-inherit}

`ws-agent-spawn`'s `model_name` names one of the **four fixed tiers** —
`small` | `medium` | `large` | `xlarge` — under harness `pi`; it is not a
user-curated alias name. There is no adapter-owned data file: a given
`model_name` is resolved by calling ws-mcp's `config.resolve_agent` tool
(`{tier: model_name, format: "json"}`), never by parsing config directly. The
call passes no explicit `harness` argument — it relies on the bridge's own
detected session harness (`pi`, set at `initialize` time), so a harness typo
elsewhere cannot silently misroute this call.

The adapter accepts the tool's answer only as a **genuine `pi` hit**: the
response's `resolved_from` must equal `"pi"` AND its `model` string must
contain a `/` (a `provider/id` shape). A partial `config.tune agents.tier
harness:pi` write can seed the `pi` bucket from a codex-shaped default model
(no `/`), so `resolved_from === "pi"` alone is not proof of a real Pi model
string — the slash check is what tells the two apart. Every other outcome —
an `isError` result, missing/unparsable response text, a non-`pi`
`resolved_from`, a `pi`-labeled-but-slash-less model, or an omitted/unmapped
`model_name` — degrades to inheriting the parent's active model. Resolution
never hard-fails.

`model_effort` follows the same "explicit caller wins" rule: a caller-supplied
`model_effort` on the spawn call always overrides whatever the config
resolution returned (an empty string is not an explicit choice and counts as
"none"). When the caller passes none, a genuine `pi` hit's own
non-empty `effort` field (`low`/`medium`/`high`/`xhigh`) is applied as
`modelEffort`; an empty resolved effort passes no `modelEffort` at all,
leaving the child's own default effort untouched. A non-genuine hit never
contributes an effort value, even if its raw payload happened to carry one.

`explore` is a **role**, not a caller-facing model choice: it resolves its own
model through the same `config.resolve_agent` path, implicitly keyed on the
fixed tier name `small`, and exposes no `model_name` parameter. Only the
model half of that answer reaches the explore leaf: it has no effort
surface, so a `small` tier's configured `effort` is not applied to explores.

### Model resolution via ws-mcp config, not an adapter data file {#260903-pi-model-catalog-config-file}

There is no adapter-owned model-catalog file any more. `ws-agent-spawn`,
`ws-fork` and `explore`'s implicit `small` lookup all resolve `model_name`
by calling ws-mcp's `config.resolve_agent` tool at spawn time (see the
anchor above for the exact accept/reject rule) — the adapter never reads or
writes model configuration on disk. User config may carry Pi model strings;
adapter and core code may not: curating a tier means running `config.tune
agents.tier harness:pi` (directly, or via the `lead-tune` skill), not
hand-editing a package-local JSON file. The call is re-issued fresh on every
spawn (no caching), so a `config.tune` edit applies to the very next spawn
with no adapter restart needed.

The read-only `ws-model-catalog-list` command still enumerates the session's
usable models (`ctx.scopedModels`, falling back to the full available pool
when no scoping is configured) as `provider/id` candidates — its underlying
behavior (list Pi's own model registry) stays useful input for curating
`config.tune agents.tier harness:pi` even though the file it used to point
at is gone; it performs no writes either way.

### Unset-tier advisory on workflow_manual {#260903-pi-model-catalog-unset-advisory}

While harness `pi`'s `agents.tier` table has no genuine `pi` entry on any of
the four fixed tiers, the adapter appends a strong advisory to every
`workflow_manual` response (and only that tool's response), mirroring the
cadence of the ws-mcp core's bootstrap-version-behind advisory — recomputed
and re-appended on every call while the condition holds, not once per
session. The condition itself is sourced from the same `config.resolve_agent`
tool the spawn path uses: the adapter calls it once per fixed tier
(`small`/`medium`/`large`/`xlarge`, four local stdio round-trips), applying
the identical genuine-`pi`-hit guard, and fires the advisory only when none of
the four comes back as a real Pi model. A failed lookup counts as a miss, so
four failed round-trips (a server without the tool, a broken stdio) also fire
the advisory rather than suppressing it. The advisory is appended after the
tool's own content (never prepended, never mutating the original in place)
and is added only on a successful `workflow_manual` result, never on an error
response. Spawns and explores still degrade silently to inherit while every
tier is unset; the advisory is the only pressure and never blocks work.

### Child→lead report channel {#260904-pi-report-to-lead-channel}

A worker can push an out-of-band message to its lead mid-run through the child-side
tool `ws-report-to-lead(message)` — the only child→lead tool the delegation layer
adds, and included in the `full-worker` `--tools` allowlist so a worker can reach
it. It needs no new transport: the call surfaces to the parent on the child's
existing RPC event stream (the tool-invocation event). Because the report rides
the invocation event, it reaches the lead as soon as the model calls the tool,
independent of the tool's own return value.

**Every child signal is pushed; nothing is harvested.** The adapter keeps no
report buffer a lead could drain. Each signal becomes a `pi.sendMessage` custom message
(`display: true`, `details` carrying `agent_id`, the family payload and the
status line below), issued by whichever process owns the child's registry — the
lead for its children, a fork process into its own session for the fork's
workers and execute workers (`isLeadOrFork` gate); a `worker` process never
pushes, its only delegate being the explore leaf. Six families:

- `ws-agent-report` — `ws-report-to-lead` with `kind:"final"` or untagged
  progress (`details.kind`). Progress is pushed at once from the
  tool-invocation branch; a `final` marks the sender `terminalThisTurn` and is
  released when the child's turn ends (see "Turn completion is gated on RPC
  idle"), so the lead never reads a completion report before the child has
  actually stopped working. `followUp`, `triggerTurn: true`.
- `ws-agent-settled` — a persistent child left the running state without a
  `final` that turn; `details.reason` is `idle` (with `last_message`),
  `stopped`, `exited` or `spawn-failed` (see "Turn completion is gated on RPC
  idle"). `followUp`.
- `ws-agent-question` — `kind:"question"` in the headless relay case; `steer`,
  since the child is blocked on the answer. In TUI the question is consumed by
  the owner question surface instead.
- `ws-agent-approval` — an execute-gateway approval request; `steer` (see the
  approval gateway).
- `ws-agent-advisory` — the fork anti-bleed advisories (idle-without-final,
  fail-loud transcript tail, `expects_commit` non-completion;
  `details.advisory` names which). `followUp`.
- `ws-agent-orphaned` — at most once per resumed session, from the shutdown
  sidecar and only when a child was cut off mid-turn (see "Process
  lifecycle"). `followUp`.

Every pushed message names its sender as `<alias> (<id>)` when an alias was
given and as the bare uuid otherwise; the orphan roll-call and `ws-agent-list`
use the same form. In a TUI process the six families are drawn by an
adapter-registered message renderer (one `[family] agent <sender>` label, the
payload lines, the status line dimmed) so the transcript does not repeat Pi's default `[customType]` header;
the message content the model reads is unchanged, and the default rendering
stands wherever the TUI modules cannot be loaded.

The report branch consults the record's owner-thread hooks first: a report the
hook consumes (a `lead-ask` thread's final, which becomes the `ws-thread-summary`
injection) is not pushed, while a `fork-raised` final closes its thread and is
then pushed. A record that is **thread-bound** — bound to a non-closed owner
thread, from thread open (first open and every reopen) or from question
registration until the thread closes (`/done`, fork final, `ws-resolve`) —
pushes no settle or advisory and is outside the status line, so owner↔fork
exchanges reach the lead only through the summary / fork-final paths. A child's
turn therefore never reaches the lead twice, and within a live session no push
is dropped or duplicated.

**Delivery is held until the lead's turn settles.** Pi offers no hook at the
moment a queued followUp is delivered, so a message built while the lead is
mid-turn would carry a stale status line. A `followUp` push is therefore sent
at once only when the owning process's agent is idle; otherwise the adapter
holds it and releases the held pushes, in arrival order, from that process's
`agent_settled` handler, building each message — status line included — at
release time. The first release starts the lead's next run; the rest enter
Pi's followUp queue and drain one per loop iteration, so a fan-out burst still
yields one lead run that sees the messages in order. `steer` families
(approval, question) are never held. Held pushes live only in the process:
they are dropped with it at `session_shutdown`, exactly like Pi's own queue.

**Fan-in stays with the model.** Every push whose process has at least one
delegated agent carries a status line `N delegated agents still running`; with
no delegated agent the line is omitted from both content and `details`. The
delegated set is this process's registry members that are **not** thread-bound
(260905: presence no longer requires a live client — a parked/dormant record
counts as delegated just as much as a running or idle one, since parking is
now the normal fate of a settled child, not an exit), and N is the subset
that is running and has not yet sent `final` or `question` this turn. The
line names no total and no ids: because parking keeps a settled child's
registry record rather than removing it, the delegated set effectively only
shrinks through the registry cap's eviction of old dormant records (or a
thread closing) rather than through settling, and which children are running
is `ws-agent-list`'s job. A child blocked on an approval is running; a child
parked on a question is thread-bound; a child that settled idle or reported
`final` leaves N — and is itself parked to dormant shortly after, per "Turn
completion is gated on RPC idle" — until it is prompted again. The last
`final` of a fan-out reads `0 delegated agents still running`, and a worker
that never reports `final` reaches it through its `ws-agent-settled`, so the
lead can tell "not yet — end the turn again" (N > 0) from "all in —
synthesize" (N = 0). `0 delegated agents still running` is consequently the
ordinary steady state once anything has ever been spawned this session, not
evidence the registry is empty.
The idle-without-final advisory reads the record's bounded report log
(`{kind, at}`, which also feeds `ws-agent-list`'s last-report time) considering
only entries since the lead's last send to that record, so a re-tasked fork is
judged on its new task.

**Liveness backstop.** Process death is not observable on the RPC event stream,
so the spawner probes `client.getState()` — which rejects once the child is
gone — on every registry transition and on a coarse timer (order of 30 s) while
N > 0, and treats the rejection of any in-flight request the same way; a dead
running child is transitioned to `exited` and pushed. This replaces the timeout
the removed wait tool used to provide.

### Transcript path accessor {#260904-pi-agent-transcript-path}

`ws-agent-transcript(agent_id) -> { transcript_path }` returns the filesystem path
to the child's Pi session JSONL transcript. It marshals no transcript content — the
lead greps or reads the file with its own filesystem tools. This is an
advanced/rare accessor; it is registered as a tool but is not in any worker
`--tools` group, so a worker cannot call it.

## Lead-execute approval gateway {#260905-pi-execute-approval-gateway}

The adapter gives the Pi lead a **delegated-mutation** path whose purpose is to
keep the lead's context free of raw command output: instead of running shell
itself, the lead delegates every mutation to a worker, and its context holds only
a compact approval request plus the worker's final report. Raw output is
firewalled into the (cheap-model) worker by construction. This layers on the
delegation spawner (the delegation spawner section above) and adds no new spawn
depth.

Two lead verbs are registered:

- `ws-execute({ command?, prompt, complex? }) -> { agent_id }` — spawn a worker
  that carries out a mutation task. `prompt` (required) states intent and what to
  report; the worker derives and runs the command(s) itself. `command?` (optional)
  is a verbatim anchor the adapter runs first, handing `{command, output}` to the
  worker — for destructive exact-match cases where a reconstructed command would
  be unsafe. `complex?` selects the worker's model tier only (a light-model
  default; the lead's own model when set). The worker is spawned through the same
  machinery as `ws-agent-spawn` with a fixed adapter-owned system prompt (the lead
  authors no prompt prose). The call returns an `agent_id` immediately and never
  blocks the lead's turn awaiting approvals; the worker's report is delivered
  later through the report channel. A `command?` supplied here runs in the lead's
  own process **without** a gate — the lead itself supplied that exact string, at
  the lead's own trust level.
- `ws-approve({ agent_id, cmd_id, decision, reason?, command? })` — adjudicate one
  pending worker command. `decision` is one of `approve` / `deny` / `run-instead`.
  `deny` requires `reason` and returns the worker a re-plan instruction without
  executing; `run-instead` requires `command` and substitutes the lead's own exact
  command, whose output still routes to the worker (hygiene preserved) with a note
  that the lead substituted it; `approve` runs the worker's command. A
  `run-instead` with no `command`, or a `deny` with no `reason`, is **rejected**
  before any command runs (so a lead that chose `run-instead` because the original
  was unsafe never has the original executed by omission). `cmd_id` binds the
  decision to exactly one pending request, so timing skew can never approve a
  previous command or pre-authorize a next one; a `cmd_id` that does not match the
  worker's currently-pending request is rejected. `agent_id` addresses the exact
  worker among all live and dormant/retained agents.

Aborting a worker mid-plan is **not** an `ws-approve` decision — it reuses
`ws-agent-stop(agent_id)`, so it works even when no command is pending. An abort
unblocks the waiting `ws-execute` with an "aborted" result and leaves the worker
dormant/retained (inspectable via the transcript accessor), distinct from the
terminal `session_shutdown`.

**Two-path accountability invariant** (the reason the gate exists):

> The per-mutation lead-approval gate on `ws.execute` exists because
> `ws.execute` proxies actions the lead would otherwise perform directly under
> user consensus and extreme care; the gate preserves that lead↔user consensus
> across proxy execution — it is not distrust of subagents. General delegated
> workers carry no consensus-caliber actions and therefore need no approval
> gate.

Accordingly the gate applies only to the `ws-execute` worker path; ordinary
`ws-agent-spawn` workers stay ungated.

### Gated exec and the mutation-incapable read family {#260905-pi-worker-gated-exec}

The `ws-execute` worker's tool group is **not** the general `full-worker` set. It
gets structured, mutation-incapable read tools (the same `read-only` family
`ls`/`read`/`grep`/`find` the recon leaf uses — cannot write by construction),
the report and `explore` tools, and **one** free-form execution tool
(`ws-worker-exec`) — but **not** native `bash`. "Anything that can write is
gated" therefore holds by construction, with no command-string parsing:
compound commands, `find … -exec`, and redirection all flow through the single
adapter-owned exec tool, which always elevates to lead approval. Reads stay
native and ungated.

When the worker calls `ws-worker-exec`, its execution **pauses** and the adapter
surfaces an approval request to the lead as a pushed `ws-agent-approval` custom
message, delivered `steer` so it interrupts even a mid-turn lead (the lead is
never suspended inside a synchronous tool call). The request carries an
adapter-authoritative, compact working-context header — **scraped by the adapter
at exec time, not self-reported by the worker**:

```
{ agent_id, cmd_id, command, rationale,
  context: { cwd, worktree_root, branch, ahead_behind?, dirty } }
```

`rationale` (the worker's one-line "why") is required. The `context` block
reflects the directory the command will actually run in — including a
worker-supplied `cwd` override — so a push or merge from the wrong worktree is
visible before approval. Full `git status`, diffs, and env dumps are excluded as
context bombs; the lead `deny`s to ask for more. The unset-catalog advisory
cadence and the report channel are unchanged. Once the lead decides, the adapter
relays the decision back to the paused worker, which resumes (or re-plans, on
`deny`). The pause is tool-level (the worker's gated exec blocks until a decision
arrives) and needs no harness pause/resume capability. Because the lead never
blocks on a child, the pushed message is the **sole** notification path: there
is no wait to wake, no `approval-pending` return payload and no suppression
rule. A worker blocked on an approval is mid-tool-call and therefore still
`running` in the pushed status line. Pending-approval is real state cleared by
`ws-approve`; the relay is armed on the execute-worker record itself, so a
worker re-registered from the shutdown sidecar keeps its relay when it is
revived.

## Lead native tool-surface reshaping {#260905-pi-lead-tool-surface-execute-gateway}

So the structural "no raw exec for the lead" guarantee holds by construction and
not merely by prompt convention, the adapter reshapes the **host lead session's**
active tool set at session start (gated on the lead/fork role, like the system
prompt injection): it removes native `bash` and native `read`, adds `ws-execute`,
`ws-approve`, and a deliberately ugly-named direct read tool
(`do-i-really-have-to-read-this-myself`) that stays available as a
soft-discouraged escape hatch, and **excludes the worker-only `ws-worker-exec`
from the lead's active set**. That last exclusion is load-bearing: `ws-worker-exec`
must be registered so a worker process (loading the same extension) can activate
it via its own tool allowlist, but if it were also active on the lead the lead
could bypass the approval gate entirely — and, since nothing observes the lead's
own tool calls the way a parent observes a child's, such a call would block
forever. Removing native `bash` is feasible because the reshaping operates on the
one registry that holds built-in and extension tools alike.

> [!note] Implementation Gap · 2026-09-05
> Missing behavior (Phase 1 verification outstanding): the reshaping's durability
> across a Pi `/reload` depends on the session-start handler re-firing (which
> re-applies the reduced set) rather than on the reduced list alone — on reload,
> extension-registered tools (including `ws-worker-exec`) are otherwise re-added,
> while the removed built-ins stay removed. For an interactive lead with UI
> bindings the handler does re-fire, restoring the exclusion; a headless lead
> without bindings may not. The live `pi --mode rpc` gate (no provider
> credentials in the build sandbox) must confirm `getActiveTools()` still
> excludes `ws-worker-exec` after a reload before this is treated as fully
> verified.

## Side-thread task fork {#260905-pi-side-thread-fork-task-thread}

The lead can spawn a **task-thread fork** — a peer session that inherits the
lead's current context instead of starting cold like a delegation-spawner
worker. The `ws-fork` tool takes `prompt`, an optional `model_name`, and an
optional `expects_commit` flag, and spawns a spawn-family RPC child using Pi's
`--fork <lead session file>` (a copy-on-fork of the lead's session), as opposed
to the fresh-context `--session` spawns the delegation spawner uses. Pi names the
forked session file itself; the adapter discovers the real path after the child
starts and fails loud if it is absent. A fork is lateral, not a worker: it does
not consume delegation depth.

- **Own lead-scope key, never the lead's.** The fork spawns carrying the fork
  role marker and the parent lead's session key as the parent-session-key
  environment value; it mints its **own** lead-scope key rather than reusing the
  lead's. The session-key normalization described under "Session key stays
  optional and caller-controllable" rewrites an explicit key equal to that
  parent value to the fork's own key, so the fork's todo/agenda/state are its
  own and the lead's are left untouched.
- **Tool surface = lead's, minus the fork verbs, plus the report channel.** A
  fork's active tools are the lead's exact active surface at spawn time minus the
  side-thread verbs (`ws-fork`, `ws-ask`, `ws-resolve`) plus
  `ws-report-to-lead`. `ws-fork` is added to the **top lead
  only** (a role-differentiated step, kept separate from the shared lead
  tool-surface reshaping described under "Lead native tool-surface reshaping" so
  it is never re-added to a fork), which is also what makes recursion fail at the
  tool layer: a fork has no `ws-fork` in its allowlist, so it cannot fork again.
  Because the fork loads the same extension as a lead-or-fork role, it inherits
  the reshaped lead surface (no native `bash`/`read`; `ws-execute`/`ws-approve`
  and the ugly-named direct read tool present).
- **Approval routing follows the spawning parent.** A mutation-approval request
  from a worker that a fork itself spawned through the lead-execute approval
  gateway routes to that **fork**, not the top lead. This is emergent from the
  per-process registration model: the fork re-runs the session-start handler and
  gets its own approval gateway and its own approval relay, so the child's
  request is injected into the fork's session.
- **Anti-bleed completion loop (task threads).** A fork reports back only through
  `ws-report-to-lead`, whose reports carry an optional `kind` of `"question"` or
  `"final"` (see "Child→lead report channel"). A `kind:"final"` report must carry
  the fields `Outcome`, `Files changed`, `Verification`, `Blockers`, `Commit`,
  and `Decisions`; `Commit` is always present, with the literal `none` when
  nothing was committed. When `expects_commit` is true and the final report's
  `Commit` line is `none`, the run is flagged as non-completion. If a fork ends a
  turn without making a tool call, the adapter auto-nudges the fork (delivered to
  the fork's own session, at most twice) and then fails loud to the lead with a
  transcript tail rather than looping forever. A fork that reaches idle without
  having emitted a `kind:"final"` report is surfaced to the lead as an incomplete
  run and is never harvested as a result. The **system-prompt directive**
  (`--append-system-prompt`) is short, task-focused natural language: no identity
  or persona framing and no XML or all-caps override language, which were found to
  backfire on the Claude host.
- **Structural anti-bleed frame (fork initial message).** Because a fork inherits
  the lead's full transcript *and* the lead-guide block the session-start hook
  appends to any lead/fork process, an inherited lead-orchestration script can
  push a fork into role-bleed (it re-runs the lead's plan instead of its own
  task). The mitigation is not an identity override in the system prompt but a
  structural frame on the fork's **initial user message**: it demotes the
  inherited conversation to reference-only and fences the actual task as an
  explicit "message from the lead", so the fork separates its task from the
  lead's inherited plan without any all-caps identity shouting. This message-level
  frame (not the system-prompt directive) is where fork identity is handled; it
  was live-verified to stop role-bleed on both a weak model and a top-frontier
  model, and it complements the completion loop above rather than replacing it.

> [!note] Live verification · 2026-09-05
> The Phase 1 live gate was run against the installed adapter on a real Pi
> session (`pi 0.84.4`, `openai-codex` subscription provider). Confirmed
> end-to-end: `pi --fork <lead-session>` copy-on-fork composition produces a new
> forked session that inherits the lead's full transcript; the fork's tool
> surface carries `ws-report-to-lead` and excludes `ws-fork`; the fork emits a
> `kind:"final"` report in the required shape (Outcome/Files changed/
> Verification/Blockers/Commit), which arrives in the lead session as a pushed
> `ws-agent-report` (at the time of the run, via the since-removed wait); the
> anti-bleed nudge is delivered to the fork's own session (not the lead's), so a
> no-report turn is re-prompted in place with no lead-context pollution. The
> bleed PoC's go/no-go for the next phase therefore clears: the structural loop
> is sufficient to drive the fork to a report.
>
> Operational precondition surfaced by the same run: a spawned child (worker or
> fork alike) loads the adapter extension **only when the package is user-scope
> installed** (`pi install <path>`). RPC children re-run the Pi CLI through
> `process.argv[1]` without an explicit `-e`, and Pi does not auto-discover a
> project's `package.json` `pi.extensions`; an ad-hoc `-e` lead run therefore
> leaves children without `ws-report-to-lead` and the report round-trip silently
> fails. The report/relay channel is available to spawned agents only under an
> installed adapter.
>
> The owner-question surface built on top of this fork (next section) has its
> own live-verification status recorded there.

## Side-thread owner question surface {#260905-pi-side-thread-owner-question-surface}

The lead can hand a decision to the **owner** (the human at the TUI) without
blocking on it, and a task fork's own `kind:"question"` report is routed to the
same owner surface. One primitive — a **thread** — has two entry points: the lead
registers a question (`ws-ask`), or a running task fork raises one. Both show up
in the owner's pending count; the owner opens either in a chat overlay, and the
thread's *origin* decides what closing it does. The lead is never the answering
channel for an owner question: it registers and carries on.

- **`ws-ask` / `ws-resolve` (lead tools, register-only).** `ws-ask(title,
  question, context?)` records a thread and returns `{question_id}` at once; it
  spawns nothing and never blocks. `ws-resolve(question_id)` withdraws a
  question the lead answered by itself (status `closed`, removed from the
  pending count, no owner notification, no injection); an unknown id is an
  error. Both are added to the **top lead only** through the same
  role-differentiated step pattern as `ws-fork`, and both are excluded from a
  fork's surface (see "Tool surface" above), so a fork cannot ask the owner
  through the lead's tool — it raises questions through `ws-report-to-lead`.
- **`context` is bounded by warning, not truncation.** The lead-authored
  `context` (2–3 sentences, no paths or hashes) is stored unchanged; past an
  adapter-side character budget (400) the lead receives a warning notification.
  The excerpt mechanism below carries exact question-time context, so `context`
  never has to.
- **Thread registry, persisted next to the lead session.** Each thread record
  carries an id, title, question, context, the lead-session `entry_id` at
  registration (lead-ask only), a status (`pending` → `open` → `dormant` |
  `closed`), the respondent agent id once known, an `origin` (`lead-ask` |
  `fork-raised`), and — once a respondent has run — a denormalized resume
  snapshot (session path, system-prompt path, tool surface, ws tool names,
  tool group, model, effort). The registry is written on every transition to
  `<lead session file>.ws-threads.json` and reloaded on `session_start`; load
  and save never throw (a missing or malformed file reads as empty; an
  unwritable target is a no-op). A record with no recognizable `origin`
  normalizes to `fork-raised`, the direction that can only under-act. The
  adapter re-captures its UI context on every `session_start` and keeps only
  plain data in the registry, since Pi invalidates captured contexts across
  new-session/fork/reload.
- **Owner surface (TUI lead).** A pending or open thread is a row in the
  live-agent widget (see "Live-agent widget" below): the row reads
  `awaiting owner` and carries the `/answer <id>` hint, and the footer
  segment counts the pending questions. There is no separate
  pending-question widget. `/thread` lists pending, open and dormant threads;
  `/answer <id>` opens one (a bare `/answer`, or the `ctrl+shift+a` shortcut
  where the terminal can deliver it, reopens the most recent reopenable
  thread). The overlay **never auto-pops**: registration notifies and
  refreshes the widget, nothing more. One overlay is attached at a time; the
  overlay header shows the thread title and the time it was opened, so two
  lead-voiced agents cannot be mistaken for one.
- **Lazy discussion fork at the lead's tip (lead-ask threads).** A registered
  question costs nothing until the owner opens it. Opening a `lead-ask` thread
  for the first time forks a **discussion** fork from the lead's *current* tip
  (`pi --fork`), carrying the lead's surface minus the side-thread verbs plus
  the report channel; its first message is `context + question` and, when the
  thread's `entry_id` is no longer on the lead's live branch (a compaction has
  passed it), a **verbatim excerpt** of the lead-session entries around that id,
  read from the append-only session file. A discussion fork gets **no**
  structural anti-bleed frame and **no** completion loop (it converses; it does
  not report), and it is spawned only from a TUI lead.
- **Attach to a live task fork (fork-raised threads).** When a `ws-fork` task
  fork reports `kind:"question"` from a TUI lead, the adapter registers a
  `fork-raised` thread whose respondent is that fork, increments the pending
  count, and hands the lead a **notice** in place of the question text: the
  owner answers via `/answer <id>`; the lead must not relay, answer, or ask
  the owner — it ends its turn, and the fork's own final report will arrive as
  a pushed message. Registration marks the fork **thread-bound** (outside the
  pushed status line, no settle or advisory pushed) until the thread closes,
  whether or not the owner ever opens it. `/answer` attaches the overlay to the
  existing process — no new spawn. While an owner overlay is attached, the
  fork's anti-bleed loop treats the fork's turns as owner-driven (no nudge, no
  fail-loud) and re-arms the moment the overlay detaches.
- **Overlay chat.** Owner text goes to the respondent as a `prompt` when it is
  idle and as a `steer` when it is streaming; child text deltas render into the
  overlay. Pasted input is delivered as one message. The transcript is
  persisted per thread (on the thread record, newest 200 entries), so a reopen
  after `Esc` or after a lead restart shows the conversation so far; owner
  lines are styled with the host's user-message background; thread text is
  rendered as Markdown when the host's renderer is available (plain wrapped
  text otherwise). `Esc` closes the view only: the thread stays `open` and the
  fork keeps running, reattachable at any time. `/done` typed in the overlay
  closes the **thread**, on its origin:
  - `lead-ask` — the discussion fork is asked for a summary turn; on settle the
    adapter injects `context + question + summary` into the lead session as a
    custom message (type `ws-thread-summary`, delivered `followUp` so it lands
    only once the lead is idle, in close order when several threads close). The
    message carries **owner authority**. The fork's resume snapshot is captured,
    the fork is stopped (`ws-agent-stop` semantics: dormant, retained), and the
    thread goes `dormant` — reopenable later, rehydrated into a plain dormant
    record and resumed on its own session file. The fork may also end the
    thread itself: once the owner states a decision, its own
    `ws-report-to-lead(kind:"final")` closes the thread through the same path,
    with the report text as the summary and no summary turn (the attached
    overlay, if any, closes with it).
  - `fork-raised` — no summary, no injection, no stop: the overlay closes at
    once and the thread goes `dormant` while the task fork carries on. What was
    decided reaches the lead through that fork's own `kind:"final"` report under
    `Decisions:`; stopping the fork here would destroy its in-flight task, and
    the fork's own final is the lead's completion signal. Closing the thread
    clears the fork's thread-bound flag, so it re-enters the pushed status
    line and its final is pushed as an ordinary `ws-agent-report`.
  The lead session is never rewound; injection is forward-only.
- **Headless baseline preserved.** Off the TUI (`ctx.mode !== "tui"`, e.g.
  `--mode rpc`), a fork-raised question is pushed to the lead byte-for-byte as
  a `ws-agent-question` custom message delivered `steer` (the lead asks the
  owner and answers with `ws-agent-send`); a lead
  `ws-ask` registers the thread and fires a `notify` toward the RPC host, spawns
  no fork, and the owner's reply arrives as an ordinary lead turn. The overlay
  is the TUI optimization over these baselines.

> [!note] Live verification · 2026-09-05
> Offline coverage (unit tier of the ticket's two-tier loop): overlay
> `render(width)` across widths with visible-width bounds, `handleInput`
> including bracketed paste and `/done` interception, registry round-trip and
> malformed-file tolerance, origin normalization, both `/done` routes,
> excerpt/anchoring against fake branches, dormant rehydration, and the
> anti-bleed suppression while an overlay is attached. Not yet exercised live:
> a real owner↔fork exchange with injection timing, a fork-raised live attach
> with `Decisions:` in the final report, post-compaction anchoring, the headless
> `--mode rpc` baseline, restart rehydration, and visual/IME/CJK polish — these
> are packaged as a one-shot owner runbook in the ticket's Phase 2 Result. The
> `ctrl+shift+a` shortcut is distinguishable from `ctrl+a` only under a terminal
> that reports CSI-u extended keys.

## Live-agent widget {#260905-pi-live-agent-widget}

The owner of a TUI lead sees every live child and every open owner question in
one compact `belowEditor` widget, one row each, plus a footer count. The widget
is a projection over the two registries the adapter already keeps — the RPC
agent registry and the owner-question thread registry — and never owns state
of its own: every repaint rebuilds the rows from those registries.

- **Rows.** Each row reads `name · role · state · elapsed`, with the
  `/answer <id>` hint appended when the row awaits the owner. `name` is the
  agent's alias, else its title, else the first eight characters of its id; a
  thread with no live respondent is named by the thread title. `role` is
  `worker`, `execute`, `fork`, or `thread` (a lead-ask discussion respondent,
  or a thread with no live respondent yet). `state` is `awaiting owner`,
  `awaiting approval`, or `running`; there is no idle, dormant, or explore
  row, since an idle child is parked and a one-shot explore never enters the
  registry. `elapsed` counts from the record's last prompt (`runStartedAt`,
  stamped by every prompt including the anti-bleed nudge), or from the
  thread's `touchedAt` for a row that awaits the owner on a thread.
- **Which records are rows.** An RPC record is a row while it has a live
  client, a pending approval, or is thread-bound (a thread-bound record stays
  a row while parked between messages — it is the owner's action cue). A
  thread is a row while it is `pending` or `open`; it collapses onto its
  respondent's row when that respondent is a thread-bound record, and
  otherwise stands alone (a `ws-ask` question before `/answer` spawns its
  fork, or a fork-raised question whose respondent was revived dormant after
  a lead restart). `dormant` and `closed` threads produce no row.
- **Order and cap.** Rows sort `awaiting owner`, then `awaiting approval`,
  then `running`, longest elapsed first within a state. At most five rows
  render; only `running` rows are folded into a trailing `+N more` line, so
  every awaiting row is always visible. Each line is bounded to the terminal
  width the host passes at render time (`visibleWidth(line) <= width`,
  truncated with an ellipsis), and the widget is hidden when there are no
  rows.
- **Footer segment.** A `setStatus` segment reads `ws: N agents` where `N` is
  the uncapped row count, with ` · M question(s)` appended while any thread
  is pending; it clears when both are zero. This segment is separate from
  the goal loop's own yield segment.
- **Refresh.** The widget repaints on every registry transition (spawn,
  spawn failure, prompt, settle and automatic park, stop, exit, gated-exec
  approval request, thread registration, open, and close) and on a 10-second
  timer that runs only while at least one row exists. A repaint that throws
  against a torn-down surface loses that repaint only. The controller is
  armed on `session_start` only for a TUI lead (`shouldArmAgentWidget`:
  lead-or-fork spawn role and `mode === "tui"`), a prior controller is
  stopped before a new one is created on `/reload`, and `session_shutdown`
  stops the timer and clears both the widget and the segment. Off the TUI
  there is no widget; the headless baselines in the owner-question section
  above are unchanged.

> [!note] Live verification · 2026-09-05
> Offline coverage: row shape and states, name precedence, ordering and the
> cap, union of the two registries (pending `ws-ask`, post-restart dormant
> respondent, dedupe onto a thread-bound respondent, no dormant/closed rows),
> hide-on-empty, width bound at 40/80/120, the `/answer` hint, the footer
> segment, `runStartedAt` stamping, and the arming predicate. Not yet
> exercised live: the real-width render through the host's widget factory,
> the 10-second clock under a running child, and `/reload` re-arming — these
> are owner-run checks recorded in the ticket's Phase 1 Result.

## Goal loop {#260904-pi-goal-loop-arming-settled-levers}

The adapter drives a **lead-session goal loop**: while a goal is active, each time
the agent run settles the loop re-injects a continue turn so the agent keeps
working toward the goal, and the model ends the run only by an explicit terminal
call. State lives in memory for the session; there is no on-disk goal substrate in
this surface.

- **Arming.** `/goal <goal>` (a `pi.registerCommand`) enters goal mode: it injects
  a `Goal settled: <goal>` announcement turn and sets an active-goal marker. A
  settle outside goal mode is an ordinary stop — the `agent_settled` handler is
  armed **only** while a goal is active, which is what keeps an ordinary Pi session
  from looping.
- **Re-fire reminder.** While armed, an `agent_settled` re-injects a reminder turn
  carrying the goal and the levers (naming the terminal tools and the force-stop
  caveat), then the agent continues.
- **Terminal levers.** Two model-invoked tools registered via `pi.registerTool`
  (zero prose parsing) end the run: `goal-achieved(summary)` and
  `goal-blocked(reason)`. Either disarms the loop, so the next settle is an
  ordinary stop. The absence of any call is the default: the loop simply
  continues.
- **Runaway backstop.** N **consecutive** re-fires with no intervening tool call
  force-stop the goal and fully reset the loop; a re-fire in which a tool call did
  occur resets the streak to zero. The threshold defaults to 10 and is tunable
  through an adapter-owned data file, `agents-plugin-pi/goal-loop-config.json`,
  read **fresh** per settle; a missing or malformed file, or a non-positive /
  non-finite `runaway_threshold`, falls back to the default rather than
  erroring.
- **Yield to live children.** While armed, a settle that finds any delegated
  child still running (the same predicate that drives the
  `N delegated agents still running` status line of the "Child→lead report
  channel" entry: a registry member with a live client that is mid-turn and
  not thread-bound) neither re-injects the reminder nor advances the runaway
  streak; goal state is otherwise unchanged. The footer shows
  `Goal loop: yielding to running agents` under the adapter's own status key
  until the next lead turn starts, whatever starts it — an owner prompt or a
  pushed child message — and the key is cleared unconditionally then. The
  child's own pushed `ws-agent-settled`/`ws-agent-report` (or, if it died, the
  liveness probe's `ws-agent-settled` with reason `exited`) is what wakes the
  lead, and that turn's settle re-evaluates the loop normally. Idle, dormant, or stopped children,
  a child whose `final` already landed this turn, and thread-bound respondents
  do not hold the loop. The footer's agent count is not part of this entry.
- **Lead-session-only.** The goal loop runs on the lead session only. Every
  spawned child (persistent RPC worker, one-shot explore leaf, or fork) is
  launched with a `WS_PI_SPAWN_ROLE` environment marker carrying its role, and
  the `agent_settled` handler no-ops whenever any role is present — so a
  child's own settles never arm a loop or a reminder, matching the delegation
  model where children are driven by the lead through `ws-agent-send`, with
  their reports pushed back into the lead session.

### Model-driven compaction {#260904-pi-goal-loop-model-driven-compaction}

Compaction inside the goal loop is **model-driven**: the extension surfaces
information and offers a lever, but never compacts on its own. Pi's own overflow
auto-compaction remains the last-resort backstop.

- **The lever.** `goal-compact-and-continue(carry_forward)` is a model-invoked
  `pi.registerTool` tool (alongside the Phase-1 terminal levers) that is
  **non-terminal**: it calls `ctx.compact({ customInstructions: carry_forward })`
  once and returns without disarming the goal. Because a manual `ctx.compact`
  aborts the invoking turn, the goal then reaches a fresh settle and the existing
  armed `agent_settled` reminder re-enters the next goal turn — so compaction folds
  into the normal loop rather than needing its own continuation path.
- **Advisory surfacing, not a gate.** While armed, the reminder turn carries two
  pieces of information for the model to weigh: the current context usage as a
  percent (from `getContextUsage().percent`, or derived from `tokens` against the
  context window / a configured override when `percent` is null right after a
  compaction), and a static compression-safety heuristic (a phase boundary or
  merge gate is normally safe to compact; a non-phase stop is not). Past a
  configurable advisory point the percent line reads as a nudge. None of this
  auto-triggers compaction — the model decides.
- **`session_before_compact` companion (observe-only).** The adapter subscribes to
  `session_before_compact` purely to observe — it never returns `cancel` or a
  compaction override. Pi forwards a manual compaction's `customInstructions`
  verbatim into this event but hardcodes them empty for its own
  threshold/overflow auto-compaction, and offers no partial "inject state" hook on
  the auto path, so the companion observes Pi's `reason: "threshold"` signal while
  the manual lever alone carries ws carry-forward state.
- **Config knobs.** Two knobs join the Phase-1 runaway threshold in
  `agents-plugin-pi/goal-loop-config.json`, read fresh per settle with the same
  never-throw fallback: a compaction advisory point (percent, `(0,100]`) and a
  context-window / max-token override (finite-positive). Out-of-range, malformed,
  or missing values fall back to the built-in defaults.

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
>   and a path-only transcript accessor), the model catalog alias table, the
>   `/ws-discuss` PoC command, and the lead-session goal loop (arming, the
>   `agent_settled` re-fire, the terminal levers, the runaway backstop, and the
>   model-driven compaction lever with its advisory surfacing, config knobs, and
>   observe-only `session_before_compact` companion), the side-thread task fork
>   with its anti-bleed loop, and the side-thread owner question surface
>   (`ws-ask`/`ws-resolve`, the persisted thread registry, the overlay chat,
>   origin-routed `/done`, and the `ws-thread-summary` injection). The one post-MVP surface
>   still deferred to a follow-up ticket under the epic — an always-visible TODO —
>   is not part of this contract yet.
