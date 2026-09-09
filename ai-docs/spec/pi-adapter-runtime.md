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

Bridged MCP tools and ws-owned Pi-native tools share one display-only presentation
policy when Pi's native TUI helpers are available, including argument streaming.
Existing specialized tool renderers retain ownership; Pi built-ins, third-party
tools, commands, and pushed messages are not intercepted. Arguments appear as
generic YAML of caller-supplied values only: omitted or internally resolved
model/effort settings are not fabricated. Optional TUI helpers load independently
before asynchronous MCP startup; tools registered before helper availability use
native fallback until the shared helpers become available, without re-registration.
The call slot retains the registered
tool name in bold with native `toolTitle` color. Input uses the theme's normal
`text` foreground (light on dark themes, readable on light themes), rather than
gray `toolOutput`; output retains `toolOutput`. Input and output inherit the
native parent shell's uniform pending/success/error background; neither installs
a separate background override.

Input display trims outer whitespace only and owns exactly one blank separator
row above and below its body, independent of whether the result is YAML, RAW
text, an error, partial output, or not yet available. YAML and RAW output add no
leading separator. Native outer shell padding and whitespace belonging to native
fallback content remain unchanged. Input logical-line starts are indented four columns
relative to the title; automatic continuation rows are indented three columns.
After display-only control/tab sanitization, previews wrap using a conservative
estimate of one column per printable ASCII code point and two per non-ASCII code
point. Complex Unicode may wrap early; exact grapheme width is not promised.
Input and collapsed YAML or RAW output show at most ten content rows, followed by a
separate `...` row only when truncated. The input marker is gray `toolOutput`
and indented four columns relative to the title, independently of the input body
foreground; the output marker remains unindented. No omitted-row count is
computed. Input stays capped when output expands.
At narrow widths indentation reduces to permit progress, and the marker fits on
one row (`...`, `..`, or `.`), reducing its indentation when necessary. Native final fitting may omit a glyph wider than
the available content width, but does not discard subsequent text. Pi's own
parent-shell minimum-width limitation still applies.

A completed, non-error result containing exactly one text block is displayed as
YAML when that text parses as a JSON object or array. Other completed single-text
results (including prose, scalar/malformed JSON, and empty text) use RAW previews:
text is preserved rather than quoted as a YAML scalar, subject only to display
sanitization and wrapping. Output does not trim outer whitespace. Expanded YAML
and RAW output remove the preview limit. Unchanged redraws reuse prepared text,
approximate row layout, joined text, themed display, and native layout without
repeated source-sized joins or styling. Theme changes restyle without reparsing,
YAML reserialization, or repeated source wrapping. Content, width, or expansion
changes refresh the affected layout.

Partial results, errors, and mixed, multi-block, or image content
use Pi's standard result display. If native TUI helpers cannot load, both slots
retain Pi's standard display. Preview conversion and terminal sanitization never
change dispatched arguments, model-visible result content, or `details`.

**Two direct tools cap their OUTPUT preview by logical line, not physical row
(260906 Phase 1).** `do-i-really-have-to-read-this-myself` and
`do-i-really-have-to-run-this-myself` share every other rule in this section
(native cache reuse, YAML/RAW selection, partial/error/mixed-content fallback,
headless safety) except one: their completed single-text result collapses to
at most ten **newline-separated logical lines**, not ten wrapped terminal
rows — a long logical line that would itself wrap past ten rows still renders
in full so long as it is one of the first ten logical lines; the cap only ever
falls on a logical-line boundary. A truncation marker and full recovery on
expansion work the same as the generic ten-content-row policy above. Every
other bridged or dispatch tool keeps the physical-row policy unchanged, and
this Phase 1 addition does not touch the `ws-agent-spawn`/`ws-agent-send`/
`explore`/`ws-execute`/`ws-fork` dispatch rows' own custom summary/mandatory
resolved-model-line contract (YAML Phase 2, described below).

### Repeated playbook and skill reads in the visible context {#260909-pi-visible-playbook-read-dedupe}

`ws__playbook_read` and `ws-skill` shorten only the second successful read of
an unchanged body already visible in the current context. The underlying MCP
call or skill-file read still runs; a changed body is returned immediately.
Reads from the two tool families never match each other. Playbook reads match
by name and the complete `context` substitution map, independent of map key
order; skill reads match by name and `args`. The outer routing `session_key`
does not participate, but a substitution named `session_key` inside `context`
does. Failed reads retain their existing failure response.

The adapter scans Pi's public active-context construction during execution,
excluding the current tool-call ID. It keeps no read-history cache. Compacted
or abandoned branch history cannot authorize shortening a result; visible fork
prefix history can. A prior successful full result must be byte-identical to
the fresh body and associated with a matching visible tool call.

The short result identifies the earlier tool-call ID, its distance in tool
calls, and the body's level-one through level-three headings in order. It also
carries provenance that resolves to the original matching full result using
only the current context. A valid short result counts toward the repeated-read
limit, so the third and later matching reads return the full body. Missing,
malformed, stale, or ambiguous pointer provenance falls back to the full body;
pointer-like prose alone is not accepted as provenance.

Other tools, `playbook.render`, workflow-manual system-prompt content, and tool
schemas retain their existing behavior.

## Session key stays optional and caller-controllable {#260903-pi-bridge-session-key-fill-forward}

ws-mcp requires a `session_key` on every root-aware tool. On the Pi side the key
stays an **optional, caller-controllable** parameter — it is never stripped from
the caller's view:

- When a call omits `session_key`, the bridge fills in its own default key,
  minted once at startup via `ferrule` against the session's working root.
- Outside the narrow normalization/refusal cases below, an explicit
  `session_key` is forwarded verbatim; it is not overwritten by the default.
  This preserves both subagent
  parent→child key lineage and lead multi-track orchestration, where distinct
  keys must reach ws-mcp unchanged.

Before fill-or-forward, the bridge normalizes the fresh-bootstrap sentinel
`obsidian-latch` to its own key when ready, avoiding a second lead-key mint.
Without an own key this normalization is disabled, preserving the ordinary
lead's FRESH bootstrap fallback.

In **fork role**, a call carrying a known parent lead key or a stale previously
issued own key is instead **refused before dispatch**, never silently rewritten.
The error names the current fork-owned key, or states that bootstrap is not ready.
This protects the parent's agenda/todos even before readiness. Own-key history is
bound to the child session and survives restart; inherited messages are not
rewritten. The current key, omitted keys, and unrelated explicitly issued worker
keys retain fill-or-forward behavior. Ordinary leads still forward explicit
child keys unchanged.

Because Pi validates tool-call arguments against the registered parameter schema
*before* the tool executes, and ws-mcp advertises `session_key` as a required
property, the bridge relaxes each registered schema so `session_key` is listed in
`properties` but **not** in `required`. Without this, Pi's own validator would
reject an omitted-`session_key` call before the fill-or-forward logic ran,
defeating the optional-key contract. An explicit key still validates; dispatch
applies the narrow sentinel normalization and fork refusals above.

If the startup `ferrule` bootstrap fails, the default key is left unset rather
than faked; a later omitted-`session_key` call then surfaces ws-mcp's own
`mandatory_session_key` guidance instead of a swallowed error.

## Lead bootstrap: workflow manual + Pi lead guide in the system prompt {#260905-pi-lead-bootstrap-system-prompt}

On the reference harnesses (Claude, Codex) the ws workflow manual reaches the
lead only through a model-invoked `workflow_manual` tool call carrying a
session-key handshake, because those hosts give the plugin no hook on the lead's
system prompt. On Pi the adapter owns the extension, so the manual is injected
directly into the lead's system prompt and the handshake becomes unnecessary.

The composition rules below apply to ordinary leads and legacy forks without
inherited prompt metadata. A metadata-bearing task or discussion fork instead
restores the lead's **full effective system prompt verbatim on every turn**,
including discovered or explicit append overrides and the complete rendered
manual/guide/skills block (or its captured absence). Child resource changes do
not reconstruct or append to that frozen prompt. The capture reflects the final
effective lead prompt after prompt handlers; a restarted lead can reuse its last
persisted capture before another turn. With no prior effective turn, discussion
launch composes the ordinary current bootstrap without manufacturing a model call.

The fork still mints its own key, but does not fetch its own `workflow_manual`
for the inherited prompt. The independent static-body mapping fetch can fail
without discarding the inherited prompt/block. Original prompt inputs, parent ws
keys, parent Pi affinity identity and ordered callable definitions survive dormant
resume, task-sidecar and discussion recovery, including repeated restarts and
launch-file cleanup. Worker and explore descendants do not inherit fork-only
metadata. Missing legacy metadata retains local-snapshot/no-affinity behavior;
malformed present metadata is not treated as legacy absence.

- Hook: the adapter appends a **ws block** to the lead's system prompt on every
  agent run (Pi's `before_agent_start`, whose result may return a `systemPrompt`
  chained across extensions). For ordinary composition the extension **appends,
  never replaces**: it returns the incoming `systemPrompt` followed by the ws block. Because Pi
  re-assembles the system prompt from its base on every turn, the handler
  re-applies the block each turn from an in-memory snapshot rather than
  capturing it once. Idle pushed delivery also enters this preflight through a
  user-message wake before releasing custom messages; it never starts an idle
  run directly with a custom message. This applies to lead and fork owners,
  with or without an active goal, including post-compaction pushes. The wake
  uses the existing manual snapshot and live-discovered skill-path cache; it
  does not fetch a new manual snapshot.
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
  3. an **`<available_skills>` block** (260906 Phase 1; live-resolution fixed
     by a later dogfood correction) — the adapter's own substitute for Pi's
     own skill-loading system-prompt block, which is never rendered for a ws
     lead/fork session at all (see "Skill exposure"). Sourced from
     `pi.getCommands()`'s `source: "skill"` entries — the SAME list backing
     Pi's own `/skill:<name>` slash commands, not a ws-tree scan — every
     installed skill, ws or otherwise. Each entry's SKILL.md frontmatter is
     read at block-build time, and an entry whose frontmatter sets
     `disable-model-invocation: true` is excluded from the block — still
     loadable by exact name via `ws-skill`, mirroring Pi's own
     `formatSkillsForPrompt` exclusion. The block's preamble tells the model to
     load a skill with `ws-skill <name>`, never `read` (absent from this
     surface), and omits Pi's own "resolve relative paths against the skill
     directory" line, since `ws-skill` takes a name, not a path. A
     missing/unreadable SKILL.md is silently skipped from the block rather
     than surfaced as an error there.
- Fetch cadence: the manual-snapshot-plus-guide portion of the block is
  assembled **once per session start** and held in extension memory; it is not
  re-fetched per turn. The `<available_skills>` portion is deliberately NOT
  part of that one-time fetch: Pi runs `session_start` and only afterwards
  merges an extension's own `resources_discover` skills into the live
  `pi.getCommands()` list, so a skills read taken during `session_start` can
  predate every ws skill (the adapter shipped exactly this bug once — a lead
  session that saw only another extension's skills, e.g. `imagegen`, and a
  `ws-skill` call for a real ws skill answering "Unknown skill"). The block's
  skills section is instead resolved against a live `pi.getCommands()` read
  inside the `before_agent_start` handler itself, then cached: built once, on
  the first `before_agent_start` firing whose live skill list is non-empty,
  and frozen from then on for that session — a per-turn per-skill SKILL.md
  re-read was judged not worth paying for the narrower case (a skill pack
  installing itself mid-session, after that first firing) the cache trades
  away. The manual/state dynamic material is refreshed only when an
  entry-point skill calls `workflow_manual` (the same cadence as on the
  reference harnesses). Injecting the manual into the system prompt — rather
  than as a transcript message — is deliberate: the system prompt survives Pi
  compaction natively, so post-compaction recovery reduces to a
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
  (normalized) key and **cuts the static manual body out of the response** with
  an **anchor cut**: it removes everything from the first whole line equal to the
  session-start snapshot's first non-empty line (the `# Workflow Manual` heading
  of the `playbook.print("lead-workflow-manual")` render taken at session start)
  up to — but not including — the `## Session Key` heading ws-mcp appends after
  the body. The anchor cut does not require the body to be byte-identical, so it
  tolerates the in-place edits ws-mcp makes to the render (stripping the
  fresh-only region, injecting the session-key line) that an exact-substring
  match never survived. What remains is the per-call material — the advisory
  blocks ws-mcp prepends, `## Session Key`, `## Session State`, and repo notes —
  so ws-mcp's contract that those are recomputed on every call is preserved.
- Fallback: if exactly one anchor is present (the other missing), or the end
  anchor precedes the start anchor, the bridge dispatches ws-mcp `workflow_state`
  instead — a state-only view with no FRESH mode that never mints — and drops the
  `workflow_manual`-only arguments (`root`) from that call, forwarding only the
  session key. The one-time degraded notice names which anchor was missing rather
  than attributing the miss to renderer drift. A response carrying **neither**
  anchor is not a manual body to cut — it is ws-mcp's own no-restorable-state
  notice — so it is forwarded unchanged with the fixed mapping line and no
  degraded notice, keeping ws-mcp's notice as the single error surface.
  `workflow_state`'s own fail-loud error path for an unresolvable key is surfaced
  unchanged.
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

### Developer-machine source build behind the same pin {#260907-pi-local-devenv-build-bootstrap}

On a developer machine the adapter can run a ws-mcp built from local source
instead of the release binary named by `runtime.json`, without relaxing the
pin. Opt-in is a package-local marker file, `agents-plugin-pi/.local-devenv-runtime`,
with the same schema as the launcher's own marker:
`{"schema_version": 1, "source_root": <abs>, "tool_dir": <abs>, "go": <abs>}`.
The launcher never honors this file here (its local-devenv gate excludes
this package); the adapter reads it itself.

Observable behavior:

- **No marker**: inert. The launcher is spawned exactly as before and takes its
  ordinary release-download path.
- **Marker present, lead or fork role**: before the launcher is spawned, the
  adapter builds `./cmd/ws-mcp` under `tool_dir` with `go`, stamping the
  binary's reported version with the bundled `plugin_version` and its source
  commit with `source_root`'s short `HEAD`, so `runtime info` answers which
  source commit is running. The build output is renamed atomically into
  `agents-plugin-pi/.runtime/local-devenv/ws-mcp`, and that path is handed to
  the launcher as `WS_MCP_BOOTSTRAP_BINARY` on the launcher child's
  environment only; the adapter's own process environment is unchanged, so
  the variable never reaches child Pi processes. The launcher then performs
  its normal bootstrap install (stamp clear, copy, exact-version and
  capabilities check, re-stamp). A notification names the source root and
  commit before the build and the elapsed time after; the build does not
  block the session-start event loop. There is no fingerprint cache: every
  lead/fork session start rebuilds, relying on Go's build cache.
- **Marker present, worker or explore role**: the marker is not consulted; the
  child's launcher reuses the binary the lead's launch installed.
- **Inherited bootstrap overrides at child launch**: direct exploration and RPC
  worker, fork, persistent-explore and dormant-resume launches neutralize the
  parent shell's `WS_MCP_BOOTSTRAP_BINARY` and `WS_MCP_BOOTSTRAP_URL`. Direct
  child environments omit these keys; RPC overrides use empty values so the
  parent-environment merge cannot restore them. A stale shell selection cannot
  force a child to replace the selected compatible runtime. Unrelated environment
  values and the parent process remain unchanged. A fork's own valid marker can
  still build and select its runtime for its launcher; normal compatibility
  checks, cache reuse and release selection remain in force.
- **Invalid marker** (bad JSON, wrong `schema_version`, a missing or relative
  path field, `tool_dir` without `cmd/ws-mcp`, or a `go` that is not an
  executable file) or a **failed build**: session start fails loudly with the
  offending field or the captured build output; there is no fallback to a
  cached binary.
- **Launch failure while the marker is active**: the error carries the source
  root, short commit, and built path ahead of the launcher's own message, so a
  source tree that no longer satisfies the pinned tool contract is
  attributable to the marker rather than reported as a generic incompatible
  runtime.

The marker and the build output are gitignored and outside the package's
`files` whitelist, so a published package neither carries nor honors them.

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

On the reshaped lead/fork tool surface (see "Lead native tool-surface
reshaping"), native `read` and `bash` are both absent, so Pi's own
skill-loading path — the `<available_skills>` block instructing the model to
`read` a listed SKILL.md, and the `/skill:<name>` slash-command expansion a
human types — is not something the model itself can act on for a `read`-based
load. Pi's own block is never rendered for a ws lead/fork session in the first
place: its generation is part of the same system-prompt assembly the ws block
is appended alongside, not instead of, and with no ws-owned substitute it is
simply absent, leaving the lead unable to see or load a skill it was not
already told about via `/skill:<name>`. The adapter closes that gap with its
own substitute, sourced from the same `pi.getCommands()` list Pi's own
`/skill:<name>` uses (not a ws-tree directory scan, so it covers ws skills and
any other installed skill alike): its own `<available_skills>` list appended
to the ws system-prompt block (see "Lead bootstrap: workflow manual + Pi lead
guide in the system prompt") and its own `ws-skill(name, args?)` loader,
gated the same lead-or-fork way as the rest of the reshaped surface (see
"Lead native tool-surface reshaping"). Both the block and `ws-skill` read
`pi.getCommands()` **live**, at block-build and at call time respectively,
never from a snapshot taken at session start: Pi's own startup order runs
`session_start` before merging an extension's `resources_discover` skills
into that list, so a snapshot taken any earlier would answer with whatever
other extension's skills had already registered and nothing this adapter
exposes.

## Process lifecycle {#260903-pi-bridge-subprocess-lifecycle}

The ws-mcp child process is bound to a Pi session, not to extension load:

- It is spawned when a session starts (`session_start`), never at module load —
  Pi forbids starting background processes from the top-level extension factory.
- It is terminated when the session is torn down (`session_shutdown`); the
  shutdown path is idempotent against double invocation.
- A spawn failure (missing interpreter, bad launcher path, failed runtime
  install) fails loudly and promptly: the pending `initialize` and any in-flight
  requests are rejected rather than left hanging, since a failed spawn emits no
  normal exit event. A developer-machine source build that fails, or an invalid
  local-devenv marker, is one more spawn-time fail-loud case (see
  "Developer-machine source build behind the same pin" under the version pin).
- A `session_start` bootstrap failure that throws *after* the child process is
  already up — the bridge launch (`startBridge`) or the custom-tool
  registration that follows it — is made visible rather than swallowed.
  {#260909-pi-child-registration-failure-fail-loud} Pi's extension runner
  catches a thrown `session_start` handler and keeps the process running, so
  without a guard a spawned child would come up alive but presenting only Pi's
  builtin `--tools` (`read`/`grep`/`find`/`ls` and the parallel wrapper) —
  indistinguishable from a healthy simple researcher, and in particular a deep
  researcher silently missing its blocking `explore` collection tool. The
  adapter guards the whole seam: on any such failure a **spawned child**
  (`worker`/`explore`/`fork`) exits its process with a loud error, which its
  RPC parent surfaces as a real spawn/dispatch error to the lead instead of a
  toolless child; the **interactive host lead** (which has no RPC parent to
  signal) raises a loud notification and comes up without the ws bridge or
  custom tools rather than crashing the user's terminal. Either way a
  partial/toolless session is never presented as healthy. This guard changes
  only failure visibility; a successful session's tool surface per role/mode is
  unchanged.

The stdio transport reads the child's stdout as newline-delimited JSON-RPC (one
message per line, no Content-Length framing) and decodes it so that multibyte
UTF-8 characters split across read-buffer boundaries are reconstructed intact.
Concurrent in-flight requests are correlated back to their callers by JSON-RPC
id, independent of the order responses arrive.

## Claude read-only delegation {#260910-pi-claude-read-only-delegation}

`ws-claude` runs bounded Claude Code subtasks through the Agent SDK. It is a
separate leaf tool from Pi-native spawning. A lead gains the tool; a fork can
use it only when its captured active-tool list already contains it. Worker and
explore roles do not gain it. Registration starts no Claude process. Session
replacement and shutdown stop admission and await owned work cleanup.

The physical Pi arguments are an object containing a non-empty `items` array:

```text
ws-claude({items: [{preset: "audit" | "consult", request, paths?, model?}, ...]})
```

`audit` returns artifact findings; `consult` answers a posed question. Requests
must be nonblank; optional paths identify read targets relative to the session
working directory, and an optional model selects a Claude model. Unsupported
presets and fields, including edit targets and resume, are rejected. Malformed
batch input starts no work; invalid items return errors alongside valid siblings.
The output is an input-index-aligned JSON array in tool text, mirrored in
`details.items`. Each entry has `id`, `status` (`success` or `error`), `output`,
and `usage`; errors also have a bounded categorical `{code, message}`. A
three-word public handle remains stable for that result and is not reused within
the session controller. Handle exhaustion rejects the batch before launching
any part of it. Handles do not yet support continuation.

Overlapping invocations share three execution slots with FIFO overflow. Each
running item has a 120-second deadline from admission, including SDK setup, and
at most two additional seconds for cleanup. Cancellation removes that invocation's
queued work and stops its active children while retaining settled sibling results.
SDK failure and timeout are isolated per item. Cleanup proceeds even if SDK close
throws: the adapter observes its owned child's termination, escalates termination
when needed, and closes owned streams/listeners. Unconfirmed process or stream
cleanup yields `cleanup_failed`, cancels queued work, and stops further launches
for that controller. Capacity is not reused as though cleanup had succeeded.

Each item uses Claude Code's `claude_code` system preset with a small embedded
task frame; the task request stays in the user message. The adapter supplies no
Pi system prompt, parent transcript, or ws credential context. Child environment
inheritance is restricted to ordinary local execution/authentication prerequisites.
The tool uses the locally installed Claude executable and its stored authentication.
The available tool inventory is limited to `Read`, `Grep`, `Glob`, `WebSearch`,
and `WebFetch`, with strict empty MCP configuration and filesystem settings
disabled. Writes, Bash/exec, other agents, and account connector tools are not
enabled. Unexpected tool or MCP inventory in SDK initialization fails the item.
This profile is not a general filesystem sandbox.

Only a valid successful terminal result becomes successful output. Usage is a
terminal SDK usage/model-usage projection with `cost_estimate_usd`, or `null`
when unavailable; an estimate is not a bill. Raw Claude session IDs and arbitrary
SDK exception text are not returned as diagnostics. Neither this interface nor
its prompt shape guarantees subscription billing treatment.

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
  alias resolved through the catalog (see below) to `--model`: a named tier
  that does not resolve to a genuine Pi model **refuses the spawn** (throws
  before any side effect — no session directory, no registry record, no alias
  hold) rather than inheriting, while omitting `model_name` → inherit the
  parent's model. `model_effort` is an optional reasoning-effort override
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
  subsumed by send, and there is no separate continue tool. Persistent
  researchers use this same send path after settling or restart; their answer
  is delivered by `ws-agent-settled.last_message`, and a send starts the next
  research turn without re-resolving or retuning them.
- `ws-agent-list({ include_prompt? })` — enumerate registry members with their
  status, alias, title and model. Status vocabulary is `running` / `idle` /
  `dormant`, but `idle` (260905) is now transient rather than a resting state:
  an idle, non-thread-bound record is parked to `dormant` by the adapter
  shortly after it settles (see "Turn completion is gated on RPC idle"), so
  `idle` is mostly observed mid-transition, not as a steady status to poll
  for. `model` (260905) is the effective resolved model the child was
  launched with, omitted only when the record carries no model at all —
  either a sidecar written before the field existed, or a fresh spawn whose
  inherited-model lookup itself came back empty (no tool-context model to
  fall back to); an inheriting child — now only one spawned with no
  `model_name`, since a named-but-unresolvable tier refuses instead of
  inheriting — shows the parent's own concrete model, not an absent
  field. `include_prompt` (default `false`) additionally surfaces each
  record's original spawn prompt, stored head-truncated to 4KB.
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

The `ws-agent-spawn`, `ws-agent-send`, and `explore` dispatch rows each render a
display-only per-tool argument summary in their call slot in place of the generic
YAML argument dump, and each carries a mandatory resolved model/effort line
reporting the child's effective model and reasoning effort — or that the child
inherited the parent's model. Both are display-only: the resolution outcome is
published through the tool result's `details` and never alters the model-visible
content. `ws-agent-send`, which resolves no model of its own, builds the line
from the target record's recorded model/effort, defaulting to an inherited
reading when the record predates that field.

Each spawned child inherits a **process-role marker** in its environment so the
extension running inside it can tell what kind of process it is: `WS_PI_SPAWN_ROLE`
carries `worker` (including execute-worker records), `explore` (a persistent
researcher or terminal collection leaf), or `fork` (a lead-caliber side-thread
peer); its absence marks the host **lead** process.
A `fork` additionally carries `WS_PI_PARENT_SESSION_KEY` (the lead's key), which
feeds the fork's parent-key refusal and lets the bridge mint the
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
to a process that no longer exists. A revived record's last-report time is not
lost, either: it surfaces again as `ws-agent-list`'s `last_report_at` and feeds
the registry-cap eviction score (see `ws-agent-spawn` above), both falling back
to the sidecar's captured time until the record reports again for real.

### Durable child session homes {#260909-pi-durable-child-session-homes}

New child material lives under
`<configured Pi agent dir>/ws-agents/<dispatching session id>/<agent id>/`,
outside Pi's ordinary session directory. The namespace uses the immediate
dispatcher's current Pi session identity, including for nested dispatch and
sessions without a transcript file. A new session identity has a separate
namespace; resuming the same identity retains its namespace.

Workers and persistent explores keep their session files in this home. Forks
receive `--session-dir` pointing to their owned home and retain their copied
history and parent-session ancestry. A fork's reported session path is accepted
only after ownership validation; invalid readiness leaves the previous recorded
path unchanged. Terminal no-session explores receive an owned scratch home
without requiring a transcript. Approval material remains available through its
consumer's lifetime, including after fork readiness and while an approval
decision awaits consumption.

Owned session paths must be strict canonical descendants of the owned home.
Existing paths must be regular files; directories, traversal to the home or
outside it, and symlink components are rejected. A not-yet-created session file
is allowed beneath an existing valid owned parent. Recovery checks the recorded
ownership descriptor against the home's on-disk identity before treating a
path as owned. Both orphan-registry and owner-thread recovery preserve this
information. Legacy recorded session paths remain readable and resumable when
owned metadata is absent or invalid; that fallback grants no cleanup authority.

Registry capture and recovery apply to every dispatching role. File-backed
sessions retain the adjacent sidecar described above. Sessions without a file
use `<configured Pi agent dir>/ws-agents/<session id>/registry.ws-agents.json`;
only the same Pi session identity discovers that registry.

Owned homes persist ownership, activity, process identity, liveness, and
question/approval/owner-held protection facts. References, prompts, reports,
decisions, and observed session-file changes refresh activity; polling and
directory age alone do not. Before the first session write, an absent file is
pending, not a failed observation. Actual observation failures and disappearance
after an observed write retain conservative unknown state and diagnostics.
Later metadata-write failures are diagnostic and nonfatal to live operations.
These records prepare safe cleanup; automatic scratch removal, cap-driven disk
deletion, and age pruning are not implemented by this storage relocation.

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

### explore — persistent two-mode research {#260903-pi-explore-recon-leaf}

`explore({ query, deep_research? })` is a persistent research preset. It returns
exactly `{ agent_id, alias }` after the initial RPC prompt is accepted; the
record parks, resumes through `ws-agent-send`, persists through the sidecar, and
its settle `last_message` is an exploration answer. Omitted/false is **simple**:
configured authenticated `small` and exactly `read, grep, find, ls`; missing,
unavailable, malformed, or unauthenticated small fails before allocation. True is
**deep**: the dispatching lead/fork's concrete model and thinking level are
captured together and frozen; it has those reads plus `explore`, and can use one
blocking authenticated-small, no-bash collection leaf. Registration branches on
the calling process role and internal mode:

- **Lead or fork.** `explore` is a persistent `spawnAgent` preset with an
  auto-generated alias (`explore-1`, `explore-2`, ...) and a query-derived
  title. Simple records use `toolGroup: "read-only"`, resolve authenticated
  `small` exactly once, and refuse before guards/allocation on every bad
  resolution. Deep records use `"read-only-explore"` and freeze the caller's
  concrete model and thinking level without looking up `small`. Both return
  `{ agent_id, alias }` after prompt acceptance. They remain ordinary registry
  records: a settle delivers `last_message`, then parks; send/stop/resume,
  transcript, aliases, sidecars, widgets and failure transitions retain the
  same identity. Their model and effective thinking level are verified with
  `RpcClient.getState()` before the first prompt and every resumed prompt:
  simple captures the actual default or clamp once, while deep must match its
  captured selection.
- **Worker or execute-worker.** `explore` remains the blocking, self-reaping
  `recon` leaf (`--no-session`) and therefore retains bash. A deep researcher
  alone registers the same query-only tool for one terminal collection; it
  resolves authenticated `small` before rendering/allocation, forwards its
  resolved effort, and runs the no-bash `read-only` profile. A failed
  collection throws to the researcher and never launches an inherited leaf.

A simple researcher has no explore tool. A deep researcher alone has the
internal `read-only-explore` group and may invoke one terminal collection leaf;
the leaf clears the deep marker and uses the genuinely no-bash `read-only`
profile. Recon remains the worker leaf profile and retains bash.

### Per-spawn tool curation {#260903-pi-spawner-tool-groups}

The `--tools` allowlist for each spawn resolves from an adapter-owned tool-group
table — `read-only`, `read-only-explore`, `recon`, and `full-worker` — mapping each group to a Pi
tool-name allowlist. Built-in Pi tools are named directly; the `full-worker` group
additionally includes the bridge's live `ws__*` tool names, taken from the running
bridge rather than hardcoded so the group tracks the actual ws-mcp tool set. A
worker's `full-worker` allowlist **excludes every delegation-driving tool**
(`ws-agent-spawn` / `-send` / `-list` / `-stop`), so a worker cannot
spawn or drive a further generation of persistent workers, but it **includes the
literal `explore` tool** — a pi-native custom tool, not a `ws__*` bridge name, so
it must be named explicitly to survive Pi's `--tools` allowlist — so a worker may
spawn a read-only recon leaf via the blocking `exploreLeaf` shape. The same
`explore` name on a lead/fork is the persistent two-mode preset, while a
worker gets the recon leaf and a deep researcher gets its no-bash collection
leaf. Role plus internal mode controls registration; Pi's dynamic `--tools`
allowlist is the enforcement layer. No agent-profile files are written to disk
(no `.pi/agents/`); all curation is in-memory plus Pi CLI flags.

### Bounded delegation depth {#260904-pi-spawner-bounded-depth-explore-leaf}

The delegation tree terminates at depth 2: lead/fork → persistent simple
researcher, lead/fork → deep researcher → terminal collection leaf, or
lead/fork → worker → recon leaf. Workers admit `explore` but no
worker-driving tools; simple researchers and terminal leaves admit no
`explore`; only deep researchers have `read-only-explore`, whose single
collection leaf clears the deep marker. This is enforced by per-spawn Pi
`--tools` allowlists; ws-mcp's keyed-handler role check is untouched. A
side-thread fork is lateral and cannot fork again, so it starts its own tree.

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
response's `resolved_from` must equal `"pi"` AND its `model` must name a model
the running Pi session actually knows — an exact entry in Pi's live model
registry whose provider has configured auth, checked live at resolution time.
A `model` with no `/` is first **backend-expanded** to `<provider>/<model>`
through a fixed map (`codex` → `openai-codex`, `claude` → `anthropic`) and the
expanded string is what the registry membership and auth checks see; a `model`
that already contains a `/` is used as written and never re-prefixed. A
slash-less `model` whose `backend` is empty, `pi`, or outside the map is not
expanded and fails the membership check.

A **named tier that does not resolve to a genuine hit refuses the spawn** and
never inherits. Three refusal cases: a `pi`-labeled model that is not a
registry entry (`unknown`) or whose provider lacks auth (`no-auth`), and a
named tier whose answer is not `resolved_from: "pi"` at all (`unset` — ws seeds
`default`/`codex`/`claude` for every tier, so this is the never-configured
case). A refusal throws before any side effect (see the spawner tool below) and
surfaces a one-line error naming the stored value (and the expanded string when
it differs), the reason, at most three suggested catalog models or the
no-match/empty-catalog wording, and the `config.tune(key: "agents.tier",
harness: "pi", ...)` call that fixes it; an empty registry refuses every
configured tier for the same reason. Only two outcomes still inherit the
parent's active model: an **omitted/unmapped `model_name`** (the lead's explicit
choice) and a **transport or parse failure** (an `isError` result or
missing/unparsable response text), which is loud on its own across every ws
tool. The resolver reports which path it took as a `source` of `"tier"` on a
genuine hit and `"inherit"` on every other outcome.

`model_effort` follows the same "explicit caller wins" rule: a caller-supplied
`model_effort` on the spawn call always overrides whatever the config
resolution returned (an empty string is not an explicit choice and counts as
"none"). When the caller passes none, a genuine `pi` hit's own
non-empty `effort` field (`low`/`medium`/`high`/`xhigh`) is applied as
`modelEffort`; an empty resolved effort passes no `modelEffort` at all,
leaving the child's own default effort untouched. A non-genuine hit never
contributes an effort value, even if its raw payload happened to carry one.

`explore` is a **role**, not a caller-facing model choice. Simple persistent
research and every blocking collection resolve the fixed `small` tier through
the same path, require an authenticated exact catalog hit, and apply its
resolved effort. Simple persistent research freezes the actual post-start
model/effort (including Pi defaults or clamping); collection forwards effort
as `--thinking`. Deep persistent research does not resolve `small` at
creation: it freezes and verifies the dispatcher's actual model and thinking
level, and may later request one separately fail-closed cheap collection. A
resolved tier effort reaches a **process-spawned** child (the ephemeral
collection leaf) as the `--thinking <level>` launch flag and a **persistent,
RPC-backed** child through a post-start `setThinkingLevel` call; an inherit or
an empty effort passes no level in either path.

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

While any of harness `pi`'s four fixed tiers resolves to a rejected entry, the
adapter appends a strong advisory to a `workflow_manual` response (and only
that tool's response), but only once per distinct rejected set per session
rather than on every call. The adapter keeps the last emitted advisory key — a
stable string over the sorted rejected tiers paired with their reasons — and
re-appends only when that key changes, so the block emits on the first
qualifying call, again when a tier is tuned into, out of, or to a different
value within the rejected set, and not on repeat calls whose rejected set is
unchanged. A clean table (no tier rejected) appends nothing and clears the key,
and the adapter's compaction boundary resets the key so the next qualifying
call re-arms the advisory. The condition is sourced from
the same `config.resolve_agent` tool the spawn path uses, with the same backend
expansion: the adapter calls it once per fixed tier
(`small`/`medium`/`large`/`xlarge`, four local stdio round-trips), applies the
identical genuine-`pi`-hit guard, and fires when any tier comes back rejected —
a model Pi's registry does not know or whose provider lacks auth
(`unknown`/`no-auth`), or a tier with no `pi` entry (`unset`). The report lists
each rejected tier — its `unset` tiers included — with the same one-line reason
the refusal uses; when all four are `unset` it renders a single empty-table
guidance block ("configure at least a `small` tier") instead of four
near-identical rows. A failed lookup counts as a miss, so
four failed round-trips (a server without the tool, a broken stdio) also fire
the advisory rather than suppressing it. The advisory is appended after the
tool's own content (never prepended, never mutating the original in place)
and is added only on a successful `workflow_manual` result, never on an error
response. A named tier now refuses the spawn whether it is `unset` or otherwise
rejected; only an omitted `model_name` inherits.

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
  the owner question surface instead, and the lead receives the registration
  notice as a `ws-agent-advisory` push (`fork-question-thread`, below) rather
  than the question text itself.
- `ws-agent-approval` — an execute-gateway approval request; `steer` (see the
  approval gateway).
- `ws-agent-advisory` — the fork anti-bleed advisories (idle-without-final,
  fail-loud transcript tail, `expects_commit` non-completion) plus
  `fork-question-thread` (a fork-raised question was just registered as an
  owner thread; `details.detail` carries the registration-notice text);
  `details.advisory` names which. `followUp`.
- `ws-agent-orphaned` — at most once per resumed session, from the shutdown
  sidecar and only when a child was cut off mid-turn (see "Process
  lifecycle"). `followUp`.

Every pushed message names its sender as `<alias> (<id>)` when an alias was
given and as the bare uuid otherwise; the orphan roll-call and `ws-agent-list`
use the same form. In a TUI process the six families are drawn by an
adapter-registered message renderer (one `[family] agent <sender>` label, the
payload lines, the status line) so the transcript does not repeat Pi's default
`[customType]` header; the message content the model reads is unchanged, and
the default rendering stands wherever the TUI modules cannot be loaded. See
"Pushed-message display: logical-line cap, shared background, muted
foreground" below for that renderer's collapse and coloring contract.

The report branch consults the record's owner-thread hooks first: a report the
hook consumes (a `lead-ask` thread's final, which becomes the `ws-thread-summary`
injection) is not pushed, while a `fork-raised` final closes its thread and is
then pushed. A record that is **thread-bound** — bound to a non-closed owner
thread, from thread open (first open and every reopen) or from question
registration until the thread closes (`/done`, fork final, `ws-resolve`) —
pushes no further settle or advisory and is outside the status line, so
owner↔fork exchanges reach the lead only through the summary / fork-final
paths. The one carve-out is the `fork-question-thread` registration notice
itself: it is pushed for the very record the same hook call just made
thread-bound, since that push is how the lead learns the thread exists at
all; every later settle or advisory for that record is suppressed as above. A
child's turn therefore never reaches the lead twice, and within a live
session no push is dropped or duplicated.

**Idle pushes wake through user preflight (260906 Phase 2).** Busy `followUp`
pushes stay held until settle; busy `steer` pushes still interrupt normally.
When idle, both families enter the same arrival-ordered held queue. The adapter
sends one user-message wake carrying the queued count, not the payloads, so Pi
runs `before_agent_start` and composes the additive ws system-prompt block.
The queue remains intact until a confirmed `agent_start`, then releases every
held message as `steer` in FIFO order with freshly rebuilt status and
`triggerTurn: true`. Recorded `followUp`/`steer` modes govern busy-time
admission, not this confirmed-start release. This also applies when the user
starts a run while the wake reservation is pending. With Pi's default
one-at-a-time steering drain, the first held message enters the first model
request, avoiding a report-free initial response; later held messages enter
at subsequent steering polls in FIFO order, before follow-ups. The adapter
does not promise the entire batch in the first request or change Pi's global
steering settings. Custom messages never directly start the idle run.
Discussion summaries (`ws-thread-summary`) use this same path while thread
closure, fork stopping, and persistence still happen immediately,
independently of delivery.

**One shared wake-start reservation.** A push wake and a goal reminder share
one pending-start reservation, established with recovery before user-message
dispatch. While pending, all pushes remain held: neither a second wake nor an
inert custom append is sent. Confirmed start clears the reservation and flushes
the queue; settle or a bounded fallback can recover a missing start, including
a synchronous dispatch failure. Held-push recovery remains available without
an active goal and for fork owners; cancelling a goal settle timer does not
cancel push recovery. A push wake does not advance the goal's reminder streak
or consume pending verbatim carry. Missing or stale idleness context is not
permission to send into an uninitialized or torn-down session.

Compaction independently holds both families, even if `agent_start` occurs
before deferred compaction release. Idle release requests the same user wake,
not direct custom delivery; busy release leaves the queue for the run's settle.
Held pushes live only in the process and are dropped at `session_shutdown`.

> [!note] Verification boundary · 2026-09-06
> Offline lifecycle, drain-order, and timer coverage verifies preflight-before-
> flush, first-message-before-response ordering, FIFO, recovery, and compaction
> independence. Live provider request/response-count acceptance remains pending;
> fake drain tests do not establish measured token savings. Actual provider-context lifetime across
> another model call and simultaneous child/lead-settle behavior (including
> absence of an already-processing error, Esc, and settling status) remain
> owner-live acceptance checks, not claims established by fake lifecycle tests.

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

### Pushed-message display: logical-line cap, shared background, muted foreground {#260906-pi-push-display-polish}

260906 Phase 1, display-only: none of this changes the pushed message's
model-facing `content`/`details`, delivery, wake/settle ordering, report
dedup, or fan-in — only how the six families above render in a TUI process.

The payload body (the `key: value` lines between the head and the status
line) collapses to at most ten **newline-separated logical lines**, the same
budget and logical-line-boundary rule as the two direct tools above, with a
truncation marker and full recovery on expansion through Pi's own expand
control (`MessageRenderOptions.expanded`, wired through to the renderer on
every expand toggle). The head and status lines are single fixed-shape rows
and carry no cap.

The whole message — head, body, and status — paints on a theme-aware shared
background (the `customMessageBg` token, the same one Pi's own default
custom-message box uses). Bodies and truncation markers use the subdued
`muted` foreground, and status lines remain `dim`. Family/agent identity,
status meaning, and every existing interaction control are retained.

Report headers have a distinct theme-aware foreground.
{#260910-pi-report-header-distinction} Only the registered `ws-agent-report`
family uses `customMessageLabel` for its header; all other push headers remain
`muted`. The distinction applies to collapsed and expanded reports and follows
live theme changes through the existing native rendering lifecycle. It changes
neither body/background styling nor message payloads, delivery, or wake behavior.
Existing terminal sanitization, width fitting, and cached preparation/layout
remain in use; repeated unchanged rendering does not prepare hidden bodies again.

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
  the lead's own trust level. The `ws-execute` dispatch row renders a
  display-only argument summary (the anchored `command` when given, the `prompt`
  head, and the `complex` tier) alongside a mandatory resolved model/effort line
  for the spawned worker, both published through `details` without altering the
  model-visible result.
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

So the structural "no unbounded exec for the lead" guarantee holds by construction and
not merely by prompt convention, the adapter reshapes the **host lead session's**
active tool set at session start (gated on the lead/fork role, like the system
prompt injection): it removes native `bash` and native `read`, adds `ws-execute`,
`ws-approve`, a deliberately ugly-named direct read tool
(`do-i-really-have-to-read-this-myself`), and a deliberately ugly-named
one-liner exec hatch (`do-i-really-have-to-run-this-myself`, fixed 30s
timeout bounding only that direct command and not a descendant it
backgrounds, fixed 4KB output cap) — the read tool and the one-liner exec
hatch both staying available as soft-discouraged escape hatches, the latter
with no approval gate for the same reason as `ws-worker-exec`'s exclusion
below —
and adds `ws-skill` (260906 Phase 1) — the adapter's own skill loader,
described under "Skill exposure" — for both the lead and a fork alike, via
the same `isLeadOrFork` role gate the rest of this reshape uses. `ws-skill`'s
addition is a separate step from the shared added-tool set above, not folded
into it: that set is the execute/approve gateway's own module boundary
(`computeLeadActiveTools`), and `ws-skill` belongs to the skill-exposure
concern instead —
and **excludes the worker-only `ws-worker-exec` from the lead's active set**.
That last exclusion is load-bearing: `ws-worker-exec`
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
not consume delegation depth. The `ws-fork` dispatch row renders a display-only
argument summary (the `prompt` head, `model_name`, and the `expects_commit`
flag) alongside a mandatory resolved model/effort line for the forked child,
both published through `details` without altering the model-visible result.

- **Own lead-scope key, never the lead's.** Initial and dormant launches must
  establish a distinct current own key and actual child session identity before
  delivering work. Invalid readiness blocks the new provider turn with a
  diagnostic. The first new message of each process states the current key and
  the three side-thread refusals; later messages do not repeat that frame.
  Parent and stale own-key calls are refused as described above, not rewritten.
- **Tool surface = lead's exactly.** Forks preserve the lead's actual callable
  names, descriptions, schemas and order without adding, deleting or deduplicating
  tools. Actual registrations are checked before work delivery and again at input
  after resource merging; missing or changed registrations are visible failures,
  not synthetic schema replay. `ws-fork`, `ws-ask` and `ws-resolve` remain visible
  with identical metadata but throw fork-role tool errors before allocating a
  child or mutating a thread. Task questions use `ws-report-to-lead` instead.
- **Prefix and cache boundary.** Task and discussion forks preserve the captured
  effective prompt and inherited message content under the same effective
  provider/model/API compatibility configuration. Provider-native continuation
  representations remain intact: Anthropic cache breakpoints advance to the new
  suffix, and Codex may send a continuation delta rather than full history.
  Historical wire annotations are not frozen. Compatible, cache-enabled
  `openai-codex` / `openai-codex-responses` requests reuse the parent's Pi id only
  in the existing nonempty body `prompt_cache_key`; transport identity and
  continuation ids remain the child's. Missing affinity metadata, unsupported
  providers/payloads, disabled caching, or differing model/auth/endpoint/compatibility/
  effort configuration leave affinity unchanged without losing the prompt.
  Explicit model overrides remain supported, outside cross-model cache reuse.
  Legacy absent metadata uses local bootstrap without affinity and cannot certify
  unavailable historical prefix bytes. These client guarantees do not guarantee
  provider retention, routing, cache hits or billing; no general paid-prefix
  admission policy or deferred-tool loader is introduced.
- **Launch and recovery.** Fork spawn and resume load this adapter explicitly,
  including when the lead used source `-e`; no global-install prerequisite is
  added for forks. Neither path passes a fork directive through
  `--append-system-prompt`. Fork recovery needs no directive file; worker and
  execute-worker rendered-playbook argv and prompt-path requirements are unchanged.
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
  run and is never harvested as a result. The **first-message directive**
  includes both report kinds and all required final fields. It uses short,
  task-focused natural language: no identity
  or persona framing and no XML or all-caps override language, which were found to
  backfire on the Claude host.
- **Structural anti-bleed frame (fork initial message).** Because a fork inherits
  the lead's full transcript *and* its captured lead-guide block, an inherited
  lead-orchestration script can
  push a fork into role-bleed (it re-runs the lead's plan instead of its own
  task). The mitigation is not an identity override in the system prompt but a
  structural frame on the fork's **initial user message**: it demotes the
  inherited conversation to reference-only and fences the actual task as an
  explicit "message from the lead", so the fork separates its task from the
  lead's inherited plan without any all-caps identity shouting. This message-level
  frame and the directive both live in the first message, not the system prompt.
  The frame complements the completion loop rather than replacing it. Historical
  anti-bleed verification predates this combined message; owner-run re-verification
  of the current message-only directive remains pending.

> [!note] Live verification · 2026-09-05
> Historical evidence, predating the fork-prefix correction: the Phase 1 live
> gate was run against the installed adapter on a real Pi
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
> installed adapter. The current fork spawn/resume explicitly loads the adapter,
> superseding that fork-only precondition; the historical excluded-tool surface
> above is also superseded by exact tool identity and handler refusals. Current
> paid cache/billing and task anti-bleed acceptance remain owner-run and pending.
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
  error. Both remain in a fork's inherited callable surface with unchanged
  schemas/descriptions, but their handlers refuse fork-role calls before thread
  mutation. A task fork raises questions through `ws-report-to-lead` instead.
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
  snapshot (session path, optional legacy system-prompt path, tool surface,
  ws tool names, tool group, model, effort, and original fork prompt/context
  metadata). Repeated recovery retains the original capture rather than the
  restarting lead's current prompt or identity. The registry is written on every
  transition to
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
  (`pi --fork`), preserving the lead's full effective prompt and exact callable
  surface under the same readiness, key-refusal and affinity rules as task forks.
  Its first message carries the conversational directive, current own key and
  role-refusal reminders, followed by `context + question` and, when the
  thread's `entry_id` is no longer on the lead's live branch (a compaction has
  passed it), a **verbatim excerpt** of the lead-session entries around that id,
  read from the append-only session file. A discussion fork gets **no**
  task structural anti-bleed frame and **no** task completion loop or required
  task-final fields. It retains owner dialogue, `/done` and a short 2–4 sentence
  decision summary, including after recovery, and is spawned only from a TUI lead.
  Owner-run discussion acceptance for the message-only directive remains pending.
- **Attach to a live task fork (fork-raised threads).** When a `ws-fork` task
  fork reports `kind:"question"` from a TUI lead, the adapter registers a
  `fork-raised` thread whose respondent is that fork, increments the pending
  count, and hands the lead a **notice** in place of the question text: the
  notice is delivered as a pushed `ws-agent-advisory` message
  (`fork-question-thread`, `followUp`) so the lead is told the thread exists
  rather than seeing nothing at all — `followUp` delivery only guarantees the
  notice is queued for the lead's next turn boundary, not that it is ordered
  against the owner's `/answer <id>`; the lead must not relay, answer, or ask
  the owner — it ends its turn, and the fork's own final report will arrive
  as a pushed message. Registration marks the fork **thread-bound** until the thread
  closes, whether or not the owner ever opens it; the registration notice
  above is the one push a thread-bound record still gets — every later
  settle or advisory for it is outside the pushed status line and suppressed.
  `/answer` attaches the overlay to the existing process — no new spawn. While
  an owner overlay is attached, the fork's anti-bleed loop treats the fork's
  turns as owner-driven (no nudge, no fail-loud) and re-arms the moment the
  overlay detaches.
- **Overlay chat.** The overlay is the shared conversation-view component (see
  "Shared conversation-view component" below), opened in its interactive mode.
  Owner text goes to the respondent as a `prompt` when it is
  idle and as a `steer` when it is streaming; child text deltas render into the
  overlay. Pasted input is delivered as one message; `Ctrl+C` is swallowed —
  never forwarded to the editor and never an interrupt. While the respondent's
  turn is running and no text has streamed yet, the overlay shows one
  `working…` line in the streaming-tail slot — the first text delta replaces
  it and settle clears it; the state is read from `ConversationChannel.liveness()`
  at render time (derived from the registry's streaming flag), not derived from
  `agent_start`/`agent_settled` events the component itself receives, because
  attaching to a live fork mid-turn or a dormant thread's first message never
  delivers a start event to the component. The transcript scrolls in
  full — there is no 24-line tail cut — and its items carry the
  `ConversationItem` model, so the child's tool calls and their results appear
  as collapsible items alongside the message turns. It is
  persisted per thread (on the thread record, newest 200 items), so a reopen
  after `Esc` or after a lead restart shows the conversation so far; owner
  lines are styled with the host's user-message background, and child text is
  rendered as Markdown with the host theme. A recorded original question
  appears as the first dialogue turn with assistant styling, including when
  its text matches the thread title. Newly inserted or upgraded question
  turns carry an emphasized `Question:` label; an existing matching first
  assistant turn is preserved.
  Reopening an older conversation restores a missing initial question or
  upgrades its legacy seed note without duplicating it or removing later
  turns. The compact header shows the thread ID and, when available,
  `opened <time>` on one line; the question itself stays in the conversation.
  The next header line states, once,
  `Esc: close view (thread stays open) · /done: end thread`
  — there is no footer hint. `Esc` closes the view only: the thread stays
  `open` and the fork keeps running, reattachable at any time. `/done` typed
  in the overlay closes the **thread**, on its origin:
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
one compact `belowEditor` widget, with the count as its first line above the
agent and question rows. The widget
is a projection over the two registries the adapter already keeps — the RPC
agent registry and the owner-question thread registry — and never owns state
of its own: every repaint rebuilds the rows from those registries.

- **Rows.** Each row starts with `name · role · state · elapsed`, followed by
  model and usage telemetry when space permits, with the `/answer <id>` hint
  retained when the row awaits the owner. Question rows replace the name field
  with the display phrase `/answer <title>` described below. Otherwise, `name` is the
  agent's alias, else its title, else the first eight characters of its id; a
  thread with no live respondent is named by the thread title. `role` is
  `worker`, `execute`, `fork`, `explore`, or `thread` (an owner discussion
  respondent, or a thread with no live respondent yet). A persistent
  researcher is an `explore` row while it has a live client; after settle it
  parks and disappears from the live widget but stays in the registry for
  transcript, send and restart. `state` is `awaiting owner`, `awaiting
  approval`, or `running`; idle/dormant records have no row.
  `elapsed` counts from the record's last prompt (`runStartedAt`, stamped by
  every prompt including the anti-bleed nudge), or from the thread's
  `touchedAt` for a row that awaits the owner on a thread.
- **Which records are rows.** An RPC record is a row while it has a live
  client, a pending approval, or is thread-bound (a thread-bound record stays
  a row while parked between messages — it is the owner's action cue). A
  thread is a row while it is `pending` or `open`; it collapses onto its
  respondent's row when that respondent is a thread-bound record, and
  otherwise stands alone (an owner question without a live respondent,
  or a fork-raised question whose respondent was revived dormant after
  a lead restart). `dormant` and `closed` threads produce no row.
- **Order and cap.** Rows sort `awaiting owner`, then `awaiting approval`,
  then `running`, longest elapsed first within a state. At most five rows
  render; only `running` rows are folded into a trailing `+N more` line, so
  every awaiting row is always visible. Each line is bounded to the terminal
  width the host passes at render time (`visibleWidth(line) <= width`,
  truncated with an ellipsis). The heading does not consume the row cap. The
  widget is hidden only when both rows and pending questions are absent.
- **Panel heading.** The first line reads `ws: N agents` where `N` is
  the uncapped row count, with ` · M question(s)` appended while any thread
  is pending. Pending questions can therefore retain a heading-only panel.
  The heading follows the same render-time width bound as the rows. The old
  agent-count footer key is cleared on refresh and shutdown; unrelated footer
  keys and the goal loop's own yield segment are preserved.
- **Refresh.** The widget repaints on every registry transition (spawn,
  spawn failure, prompt, settle and automatic park, stop, exit, gated-exec
  approval request, thread registration, open, and close) and on a 10-second
  timer that runs only while the panel has rows or pending questions. A repaint that throws
  against a torn-down surface loses that repaint only. The controller is
  armed on `session_start` only for a TUI lead (`shouldArmAgentWidget`:
  lead-or-fork spawn role and `mode === "tui"`), a prior controller is
  stopped before a new one is created on `/reload`, and `session_shutdown`
  stops the timer and clears both the widget and the retired agent footer key. Off the TUI
  there is no widget; the headless baselines in the owner-question section
  above are unchanged. Qualifying owner waits add the separately managed
  attention cadence described below.

> [!note] Live verification · 2026-09-05
> Offline coverage: row shape and states, name precedence, ordering and the
> cap, union of the two registries (pending `ws-ask`, post-restart dormant
> respondent, dedupe onto a thread-bound respondent, no dormant/closed rows),
> hide-on-empty, width bound at 40/80/120, the `/answer` hint, the footer
> segment, `runStartedAt` stamping, and the arming predicate. Not yet
> exercised live: the real-width render through the host's widget factory,
> the 10-second clock under a running child, and `/reload` re-arming — these
> are owner-run checks recorded in the ticket's Phase 1 Result.

### Owner-wait attention {#260910-pi-owner-wait-attention}

The owner's lead TUI makes open questions and pending approvals conspicuous:
the actionable text alternates between bold and ordinary every **330ms**, with
the single count heading on the same phase. Text remains visible in both
phases, and emphasis continues for as long as qualifying waits remain.
Spawned child processes do not animate, and ordinary lead idle does not qualify.

- **Question phrase.** The primary row field displays `/answer <title>`.
  Control characters are sanitized; an unavailable or unusable title falls
  back to the question ID. This is display text only: the separately retained
  `/answer qN` hint remains the valid command. Titles never become lookup keys.
- **Styling boundaries.** Only the visible question phrase or the actual
  `awaiting approval` state label changes weight, alongside the count heading.
  Role, elapsed time, telemetry, separators, and command hints stay ordinary.
  An approval has no fabricated answer target. Existing tool/body muting is
  preserved. There is no sound, notification, or color cycle.
- **Supplied owner-held rows.** The presentation layer also accepts
  `idle-awaiting-owner` without a question, emphasizes its existing state
  label, and preserves an explicitly supplied inspection hint. This support
  does not create ownership transitions or steering commands.
- **Configuration.** `agent_wait_animation` in
  `agents-plugin-pi/goal-loop-config.json` defaults to enabled. Literal `false`
  disables animation and uses static bold emphasis. Missing, malformed, or
  non-boolean values fall back to enabled. The setting is read on widget
  refresh, so a file change takes effect on the next refresh; rendering the
  captured widget itself performs no configuration I/O.
- **Lifetime and width.** Each eligible widget owns at most one attention
  timer, shared by all qualifying rows and separate from elapsed-time updates.
  Final wait resolution, disable, replacement/session switch, and teardown
  stop attention work. Ticks make no model calls or RPC polls. The existing
  count destination, ordering, protected-row cap, and width bounds remain in
  force. A valid action hint takes priority when it fits; an omitted hint is
  not reinserted by styling at widths too small to contain it.

### Agent row model and usage {#260910-pi-agent-row-telemetry}

When the row fits, telemetry reads `<provider/model> (<effort>) · in <input> ·
est $<USD>`. Model, effort, latest input, and estimated cost independently use
`—` when unavailable. A reported zero remains zero. Model and effort reflect
observed child state rather than the requested launch configuration; a failed
current-state observation clears those labels independently of retained usage.

- **Latest input.** Input is the most recent attributable model call's reported
  input-token field, without adding cache tokens or substituting a session
  total. A newer call with missing input clears the previous value. Summary
  usage may aggregate several calls, so it clears latest input rather than
  presenting aggregate tokens as one call.
- **Estimated cost.** The USD amount is cumulative reported estimated cost
  attributable to this child, including usage-bearing summaries and tool
  results. It is not subscription billing. Durable call identities prevent
  repeated events and continuation or resume from adding a call twice. Fork
  accounting excludes the complete inherited prefix captured before the
  child's first prompt. Missing or invalid cost on a known contributing call
  makes the complete estimate unknown; empty history does not invent zero.
- **Recovery.** Ordinary-agent sidecars and owner-thread resume snapshots
  preserve validated observations and attribution. Readable child history is
  reconciled during recovery, including a thread whose respondent has not
  relaunched. A legacy ordinary session with no parent history can recover
  its total. A legacy fork without its original boundary keeps lifetime cost
  unknown; a separate observation boundary can establish later child input
  without claiming a complete lifetime total. Temporarily missing, unreadable,
  or partially written history preserves a validated lifetime origin and
  usage snapshot. Readable identity or boundary contradictions invalidate
  incompatible attribution instead of carrying its old total forward.
- **Collection and display.** Usage is collected at child event and lifecycle
  boundaries, including compaction and final stop, outside rendering. Optional
  telemetry queries cannot prevent otherwise valid delegation. Shutdown keeps
  the pre-stop orphan activity state while persisting final usage to both
  recovery formats. Rendering performs no RPC or disk reads and adds no
  per-frame polling. Width limits, row order, caps, and waiting-row visibility
  remain unchanged. The full `/answer <id>` cue takes priority whenever it
  fits; telemetry is omitted before that cue when the row is too narrow.

## Shared conversation-view component {#260909-pi-conversation-view-component}

The adapter renders child-agent conversations through one shared component,
`ConversationViewComponent` (a pi-tui `Component`: `render(width)`,
`handleInput(data)`). It is the single rendering surface behind both the
owner-question overlay and the subagent audit window; neither keeps a
transcript renderer of its own.

- **Message model.** The transcript is a list of `ConversationItem`s, each a
  plain data record of one kind: `user` (a line the owner typed into the view),
  `lead-message` (a message the lead sent the child, carried under a `lead ›`
  label so it is never mistaken for the owner's own line or the child's text),
  `assistant` (the child's own text, rendered as Markdown), `tool-call`
  (`id`, `name`, `args`), `tool-result` (`id`, `name`, `content`, optional
  `isError`), and `note` (an adapter note such as a question seed or status,
  rendered dim). The partial streaming tail of the child's current turn is
  render-time state, never an item. Persistence is the consumer's concern.
- **Collapsed tool items.** `tool-call` and `tool-result` items collapse to a
  one-line head by default and expand individually. Both the head and the
  expanded body come from the adapter's existing tool-preview rendering (the
  same previews the push/tool-result surface produces); the component supplies
  only the collapse/expand chrome, never a preview of its own. `Tab` /
  `Shift+Tab` move a selection highlight across the collapsible items
  newest-first, `Space` toggles the selected item, and `Ctrl+O` toggles all of
  them at once.
- **Conversation styling.** Tool-call and tool-result heads and expanded
  bodies use the host theme's muted foreground; the transient `working…`
  marker uses its dim foreground. User messages include one blank row above
  and below their content, filled with the same user-message background.
  Both the owner-question overlay and audit viewer apply these styles.
  The owner-question overlay has a border, horizontal interior margins, and
  blank interior rows above and below its content. Blank lines separate its
  header and dialogue turns; assistant turns and streaming text carry a left
  gutter to distinguish them from owner messages.
- **Two modes.** `view` has no input line and never sends; `interactive` shows
  an editor and delivers the owner's typed lines through the channel. Mode is
  set at construction and may be raised from `view` to `interactive` on the
  same instance (history, scroll position, expand state and the event
  subscription survive); it is never lowered. In `view` mode the collapse keys
  are always the component's and `Enter` routes to an `onEnter` callback; in
  `interactive` mode `Tab` / `Shift+Tab` act only while the editor is empty,
  `Space` acts only while a selection is active, any other typed character
  clears the selection and goes to the editor, and `Ctrl+O` acts in both modes.
- **Liveness is read every render.** The component reads a three-state
  `liveness()` from its channel at render time (`running`,
  `idle-awaiting-owner`, `settled`) rather than reacting to a start event, so
  attaching mid-turn or after a dormant relaunch still renders correctly. A
  `working…` marker shows only while `running` with an empty streaming tail and
  is replaced by the first streamed delta. `idle-awaiting-owner` renders
  prominently — in the header and at the transcript foot — so the owner
  notices; `settled` renders quietly.
- **Key contract.** `Esc` routes to a consumer `onEscape` callback (kitty-safe
  detection); `\x03` (Ctrl+C) is swallowed in both modes and never forwarded,
  so a focused overlay cannot trip Pi's double-Ctrl+C exit; `/done` typed in
  `interactive` mode routes to an `onDone` callback. Scrolling uses the
  transcript's own bindings and the whole history scrolls (no fixed tail cut).
  The consumer supplies the one-line `headerHint` string, rendered once under
  the title, so the overlay and the audit window state their own key hints.
- **Direct pi-tui dependency.** The component builds on pi-tui primitives
  (`ScrollView` / `Markdown` / `Text` / `Editor`) resolved through the host's
  pi-tui instance at runtime, so a rendered child tree carries the host's own
  component types; the primitives are injectable (defaulting to the real
  classes) so the component renders under an offline test harness with no TTY.

## Goal loop {#260904-pi-goal-loop-arming-settled-levers}

The adapter drives a **lead-session goal loop**: while a goal is active, each time
the agent run settles the loop re-injects a continue turn so the agent keeps
working toward the goal, and the model ends the run only by an explicit terminal
call. State lives in memory for the session; there is no on-disk goal substrate in
this surface.

- **Arming.** `/goal <goal>` (a `pi.registerCommand`) enters goal mode: it injects
  a `Goal armed: <goal>` announcement turn and sets an active-goal marker. A
  settle outside goal mode is an ordinary stop — the `agent_settled` handler is
  armed **only** while a goal is active, which is what keeps an ordinary Pi session
  from looping.
- **Re-fire reminder is delayed past settle by a settle timer (260906 Phase
  1).** An `agent_settled` that would otherwise re-inject no longer sends
  immediately: it arms a single settle timer (`scheduleTimer`, real
  `setTimeout`/`unref` by default, injectable for tests) for
  `settle_delay_ms` and sets the footer to `Goal loop: settling` under the
  adapter's own status key. This exists because the reminder's own turn start
  can otherwise race a child's push landing at the same `agent_settled`
  boundary — see "One shared wake-start reservation" in
  the "Child→lead report channel" entry above. The timer is the single fire
  point for every reminder origin — an ordinary settle, the lever's
  pending-rearm re-arm, and a swallowed-settle replay all route through it,
  never sending directly. Only one settle timer is active at a time; shared
  wake recovery is independently owned so goal cancellation cannot strand
  held pushes. `agent_start`, the terminal
  levers (`goal-achieved`/`goal-blocked`), `/goal` re-arming, force-stop, and
  session shutdown all cancel a pending settle timer outright, since each
  invalidates the settle that armed it. At fire time the loop re-evaluates
  its fire condition **fresh** rather than trusting the state at arm time:
  `ctx.isIdle()`, `!leadCompactingRef.current` (a compaction that started
  during the delay yields exactly like the ordinary compaction branch,
  re-arming the release routine instead), and no running delegated child
  (falls back to the yield outcome). Held pushes and a pending wake take
  priority over a reminder. Only when
  eligible does it reserve the shared wake start and arm recovery before
  sending the reminder as an explicit `deliverAs: "followUp"` user turn.
  The short fallback clears the boundary guard, prioritizes held pushes, or
  retries an active goal (`Goal loop: reminder did not start a turn, retrying`, followed by
  re-arming the settle timer) if `agent_start`/`agent_settled` never observed
  the resulting turn. `agent_start` and `agent_settled` both clear the
  boundary guard unconditionally as their first action.
  - **`settle_delay_ms` config knob.** Joins the other goal-loop knobs in
    `agents-plugin-pi/goal-loop-config.json`, read fresh per arm with the
    same never-throw fallback (`DEFAULT_SETTLE_DELAY_MS`, 5000ms); a missing,
    malformed, or non-positive value falls back to the default.
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
- **Yield to live children (footer text updated 260906 Phase 1 review relay
  #1, Minor).** A running-children check (the same predicate that drives the
  `N delegated agents still running` status line of the "Child→lead report
  channel" entry: a registry member with a live client that is mid-turn and
  not thread-bound) is one of the settle timer's fire-condition checks (see
  "Re-fire reminder is delayed past settle by a settle timer" above), not a
  separate settle-time branch: an `agent_settled` arms the timer
  unconditionally and the footer reads `Goal loop: settling` throughout. Only
  at FIRE time, `settle_delay_ms` later, does a still-running child make the
  fire yield — no reminder, no streak advance, goal state otherwise
  unchanged, and the footer stays exactly as it was (still `Goal loop:
  settling`; there is no separate `Goal loop: yielding to running agents`
  string). The child's own pushed `ws-agent-settled`/`ws-agent-report` (or, if
  it died, the liveness probe's `ws-agent-settled` with reason `exited`) is
  what wakes the lead, and that turn's settle re-arms the timer, which
  re-evaluates the loop normally after the next delay. Idle, dormant, or
  stopped children, a child whose `final` already landed this turn, and
  thread-bound respondents do not hold the loop. The footer's agent count is
  not part of this entry.
- **Waiting for compaction (260906 Phase 1; swallow marker added in review
  relay #1; fire-time variant added in review relay #1 Important #1).** While
  armed, a settle that fires while a compaction is ALREADY in flight (checked
  before the yield branch above — compaction dominates) neither re-injects
  the reminder nor advances the runaway streak, mirroring the yield outcome
  exactly: goal state passes through unchanged and the footer shows `Goal
  loop: waiting for compaction` under the same status key until the next lead
  turn starts. A compaction that instead STARTS during the settle timer's
  delay (the timer having already armed on an ordinary settle) is marked the
  same way at fire time, checked before the idle/running-children fire
  conditions — but leaves the footer reading `Goal loop: settling` rather
  than switching it to `Goal loop: waiting for compaction`, since the fire
  callback makes no status change on any of its yields. Either path can be
  either the settle `ctx.compact()`'s own internal abort produces for the
  turn it just cut off, or Pi's own threshold/overflow auto-compaction ending
  a turn outright with nothing queued to follow it — in every case this
  outcome is recorded as a SWALLOWED settle so the "Model-driven compaction"
  entry below can replay it once the compaction actually finishes, instead of
  the loop stalling forever because no further `agent_settled`/`agent_start`
  was ever going to fire on its own.
- **Lead-session-only.** The goal loop runs on the lead session only. Every
  spawned child (persistent RPC worker or researcher, terminal collection
  leaf, or fork) is launched with a `WS_PI_SPAWN_ROLE` environment marker
  carrying its role, and
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
  **non-terminal**: it marks the compaction as lever-originated (arming a
  pending-rearm marker), captures the exact `carry_forward` string before
  calling `ctx.compact({ customInstructions: carry_forward })` once, and
  returns without disarming the goal. The string is passed unchanged as Pi's
  custom instructions to steer the summary; the summary itself is not a
  verbatim-delivery guarantee. The tool's returned text reads `Compaction
  requested; the conversation will resume from a summary carrying:
  <carry_forward>`; the `Compaction completed` notification remains outside
  the model's view.
- **Verbatim carry-forward.** On compaction success or failure, the next
  eligible dispatched goal reminder appends exactly
  `\n\nCarried forward verbatim from before compaction:\n` followed by the
  captured string, once, without trimming, escaping, indentation,
  summarization, or newline normalization. An empty string still produces the
  heading; no pending payload means no heading. The payload is consumed only
  after the reminder dispatch returns, so a synchronous send failure does not
  consume it. Subsequent reminders do not repeat it. Delivery uses the existing
  delayed settle path, not necessarily the first arbitrary post-compaction
  message: held pushes and owner input retain priority. Busy release,
  turn-start backstop clearing, timer cancellation, and idle/child yields leave
  unsent carry available to the next eligible reminder, including an ordinary
  reminder rather than a lever re-arm. Replacing the goal, either terminal
  lever, runaway force-stop, or session shutdown discards unsent carry; it is
  not persisted across reload or session replacement.
- **Re-arming after compaction (260906 Phase 1; swallowed-settle replay added
  in review relay #1; `followUp` delivery on the replay added in review relay
  #2).** A manual `ctx.compact` aborts the invoking turn immediately — well
  before Pi's own compaction bookkeeping finishes — so nothing may send a
  prompt from inside a `session_*compact*` handler without racing that
  unwind. Instead, an idempotent release routine runs once compaction
  actually finishes, deferred past Pi's own compaction flag. It checks
  idleness FIRST: if the owning session is not idle (a fresh turn is already
  underway by the time this deferred call lands), it clears both markers and
  returns, leaving the held-push queue and any pending reminder to that
  turn's own `agent_settled`/`registerPushFlush` flush — nothing is flushed
  or sent on this branch. Only on the idle branch does it request a user wake
  for pushes held during compaction (see "Child→lead report channel" above),
  retaining their queue until confirmed start, and then arm the existing
  settle timer when a lever reminder or swallowed settle is pending; release itself never sends a reminder. The timer retains
  the pending origin and any captured failure reason across its delay and
  re-evaluates idleness, compaction, and running children at fire time, as
  described above. At an eligible fire, a lever-originated compaction sends
  its pending goal reminder, folding in the captured compaction failure reason
  when present. For any other compaction whose settle was swallowed
  (owner-typed `/compact`, or Pi's own threshold/overflow auto-compaction
  ending a turn outright with nothing queued to follow it), the timer instead
  replays that settle: the same reducer, streak accounting, and force-stop
  path as an ordinary delayed settle, against a freshly-read context percent.
  Both reminder origins use explicit `deliverAs: "followUp"` delivery to
  survive a push that starts a turn between the fire-time check and the send.
  This is what lets an armed goal recover from an
  auto-compaction that would otherwise have left nothing to ever re-evaluate
  the loop again. When a settle's outcome qualifies for both (the lever's own
  `ctx.compact()` call produces a swallowed settle for its own invoking
  turn), the lever reminder wins and the swallow is treated as consumed too
  — exactly one reminder is sent for that settle. **Accepted race window:**
  an owner who types `/compact` directly (bypassing the lever) can still
  race a reminder that was already in flight before the compaction started —
  this window is accepted as-is, not intercepted; only the lever's own
  compaction is guaranteed race-free by construction. **Accepted narrow
  gap:** unlike the lever, a non-lever compaction has no `onComplete`/
  `onError` backstop of its own — if Pi's `session_compact` lookup were ever
  to miss (the `savedCompactionEntry` guard it depends on), the independent
  compaction hold could remain stuck until session reset. `agent_start` is
  deliberately not a compaction-release signal; this narrow gap is not
  intercepted.
- **Advisory surfacing, not a gate.** While armed, the reminder turn carries two
  pieces of information for the model to weigh: the current context usage as a
  percent (from `getContextUsage().percent`, or derived from `tokens` against the
  context window / a configured override when `percent` is null right after a
  compaction), and a static compression-safety heuristic (a phase boundary or
  merge gate is normally safe to compact; a non-phase stop is not). Past a
  configurable advisory point the percent line reads as a nudge; below it, the
  line explicitly tells the model not to call `goal-compact-and-continue`. None
  of this auto-triggers compaction — the model decides.
- **`session_before_compact` companion (observe-only).** The adapter subscribes to
  `session_before_compact` purely to observe — it never returns `cancel` or a
  compaction override. Pi forwards a manual compaction's `customInstructions`
  verbatim into this event but hardcodes them empty for its own
  threshold/overflow auto-compaction, and offers no partial "inject state" hook on
  the auto path, so the companion observes Pi's `reason: "threshold"` signal while
  the manual lever alone carries ws carry-forward state. This same handler also
  marks the compaction as in-flight (see "Child→lead report channel" above) for
  ANY compaction reason, unconditionally — the defensive half of the push-hold
  coverage, since not every compaction goes through the lever.
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
copies are kept in sync by hand: there is no automated sync tooling, so a change
to the canonical `agents-plugin/` copies must be mirrored here — but there is
automated drift detection. An identity test in the adapter suite
(`test/version-check.test.ts`) reads both trees from disk and asserts
`runtime.json`, `bin/ws-mcp-launcher.py`, and the whole `rsrc/` tree are
byte-identical to their `agents-plugin/` sources, with no file the Pi tree
carries that the source lacks, so the next desync fails the suite naming the
offending file. The guard fires when the adapter suite runs, not the moment an
upstream file is edited, so the Pi track owner runs it when syncing from
`develop`.

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

Two further package-local files are developer-machine-only and never shipped:
the `.local-devenv-runtime` marker and the `.runtime/local-devenv/` build
output (see "Developer-machine source build behind the same pin" under the
version pin). Both are gitignored and
outside the `files` whitelist. The adapter, not the carried launcher copy, owns
that developer bypass, because the launcher's own local-devenv path is gated to
plugin-cache install locations and excludes this package; the launcher copy
stays byte-identical.

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
