---
title: Research: ws opencode drop-in plugin package — adapter depth and harness-neutral boundary
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: host-neutral pivot direction and spawn-removal decisions
  260605-epic-ws-playbook-factory-pivot: epic tracking the pivot; opencode package becomes child work
  260611-research-ws-per-role-delegation-tuning-config: model tier / role-to-tier mapping research that opencode tier-swap surface inherits
  260801-todo-ws-mcp-log-append-cli: sub-ticket for the ws-mcp log append CLI needed by the goal-loop guard violation logging
related-mental-model:
  - plugin-runtime
  - prompt-bundle
  - named-agent-runtime
  - mcp-runtime
  - workflow-skills
---

# ws opencode drop-in plugin package — adapter depth and harness-neutral boundary

## Background

The ws workflow system currently ships as a Codex-first plugin (`agents-plugin/`)
with a Claude-compatible manifest inside it, plus an agentless derivative
(`agents-plugin-wsflow/`). The user wants the same ws workflows to be usable as a
drop-in install on the local opencode harness. Initial investigation surfaced that
opencode's plugin model is not "Codex with different file paths" — it has a
distinct surface that forces a choice about how deeply ws integrates into the
harness.

## Summary of findings

### opencode plugin surface is richer than the public skill summary

The built-in `customize-opencode` skill covers only the v1 plugin API. The
installed `@opencode-ai/plugin` package exports two distinct APIs:

- **v1**: `Plugin = (PluginInput, options?) => Promise<Hooks>` with ~16 hooks
  (`chat.message`, `tool.execute.*`, `experimental.*`).
- **v2** (Effect-TS based): typed domain hooks for `agent`, `skill`, `command`,
  `catalog`, `reference`, `integration`, `aisdk`, `plugin`, and `event`. This
  surface has no equivalent in the Codex plugin model.

Key v2 capabilities relevant to ws:

- `agent.update(id, fn)`: mutate built-in agent definitions, including system
  prompt.
- `command.update(name, fn)`: register slash commands at runtime.
- `catalog.model.update()` and `catalog.model.default.set/get/remove()`: mutate
  model metadata and swap the global default model dynamically.
- `reference.add(name, source)`: register dynamic references.
- `event.subscribe("session.idle")`: observe turn-end (closest opencode
  equivalent to a Stop-hook, but only an observer).
- A separate **`TuiPlugin`** API (Solid/JSX) renders custom TUI slots; no
  counterpart in the Codex plugin.

> **Note (2026-08-01 correction):** the v2 capabilities listed above are
> **v2 `PluginContext` domain methods**, only reachable from a v2 `define()`
> plugin entry point. Spike investigation (2026-08-01) confirmed that a
> **single default export `{id, server, setup}`** satisfies both the v1
> loader (reads `default.server` → calls `server(input)` → `Hooks`) and
> the v2 loader (Schema-decodes `default` as `{id, setup}` — `effect`
> branch fails, `setup` branch succeeds, extra `server` tolerated by open
> `Schema.Struct` → calls `setup(PluginContext)`). Both imports resolve
> to the same module URL → shared module-level state. This means the
> adapter uses v1's `input.client.mcp.add` for MCP registration AND v2's
> `PluginContext` domain mutation (`agent.update`,
> `catalog.model.default.set`, `command.update`, `reference.add`) in one
> plugin file. See "Decision needed" for the converged design.

### Subagent probe verified leaf-only `general`

Two parallel `task` probes were run:

- `subagent_type: "general"` → success. The spawned subagent has
  `bash/read/glob/grep/edit/write/webfetch/skill` but **no `task`, `todowrite`,
  or `question`**. It reports the same model as the lead
  (`minimax/minimax-m3`). In opencode, `general` is a leaf-only delegate.
- `subagent_type: "general-purpose"` → rejected with `Unknown agent type`. This
  is a Codex term, not an opencode subagent type.

Consequence: opencode delegates cannot recurse via `task`; the lead is the only
orchestration layer. This is structurally compatible with the 260605 pivot's
"no mercenary recursion" goal, but breaks any workflow that assumes delegates
can themselves spawn subagents.

### Mapping ws dependency axes to opencode

| ws dependency | opencode mapping | completeness |
|---|---|---|
| subagent dispatch | lead calls `task(subagent_type: "general")` or `"explore"` | leaf-only; no recursion |
| model tier selection | no per-task override; v2 `catalog.model.default.set()` can swap the global default around a task | dynamic default swap, not true per-task |
| goal loop (Stop-hook re-entry) | **full** — three usable surfaces (see "Goal-loop and compaction surfaces" below); `todowrite` board-driven loop remains the robust fallback | full |
| context compression | **full** — `experimental.session.compacting` allows context injection and full prompt replacement; autocontinue ties compaction to re-entry; config `compaction.{auto, tail_turns}` tunes cadence | full |
| TaskList / workflow board | `todowrite` is lead-only and in-session; MCP `session.note`/`session.children` remains the cross-turn/cross-agent board | dual role, no mapping needed |

### Goal-loop and compaction surfaces (verified 2026-08-01)

Initial ticket draft described the goal loop as "partial" with only a soft
`session.idle` observer. Direct inspection of `@opencode-ai/plugin@1.18.9` and
`@opencode-ai/sdk` type definitions corrects this to **full** across three
distinct surfaces. This resolves Open Question 1 (v1/v2 interop) at the
capability level: a single plugin can register v1 hooks (compaction) and call
the v2 SDK (`Session.prompt`) together — the two APIs are not mutually
exclusive, they are layered.

**1. Hard re-entry — v2 SDK `Session.prompt({ prompt, delivery })`**

> "Durably admit one session input and schedule agent-loop execution unless
> resume is false." `delivery?: "steer" | "queue"`.

- `"queue"` schedules a prompt as the next agent turn (the goal-loop primitive
  the lead needs; from the agent's perspective indistinguishable from a user
  submitting a new message).
- `"steer"` injects into a still-running turn (live course-correction).
- The events `session.next.prompted` and `session.next.prompt.admitted` both
  carry the `delivery` discriminator, so the adapter can observe whether a
  queued prompt was admitted or a steer landed.

A plugin can call this from a `session.idle` event subscriber: turn ends →
adapter asks the lead (via the ws goal tool) whether the goal is done → if
not, `Session.prompt({ delivery: "queue" })` re-enters. This is a hard
Stop-hook equivalent, not soft.

**2. Built-in compaction autocontinue — v1 hook
`experimental.compaction.autocontinue`**

```ts
"experimental.compaction.autocontinue"?: (input: {
  sessionID: string; agent: string; model; provider; message; overflow: boolean;
}, output: { enabled: boolean }) => Promise<void>;
```

opencode already ships compaction → synthetic "continue" message as a built-in
loop. The hook lets the plugin enable/disable the autocontinue per compaction
event. The `overflow: boolean` input lets the adapter distinguish "compacted
because context overflowed" from "compacted on demand". This is the cheapest
goal-loop path: ws only needs to flip `enabled` based on its own goal state.

**3. Compaction prompt customization — v1 hook
`experimental.session.compacting`**

```ts
"experimental.session.compacting"?: (input: { sessionID }, output: {
  context: string[];   // appended to the default compaction prompt
  prompt?: string;     // if set, replaces the default compaction prompt entirely
}) => Promise<void>;
```

- `context` array: extra strings appended to the default compaction prompt —
  e.g. the current ticket focus, session key, pending todo items, so the
  compacted summary preserves the load-bearing ws state.
- `prompt`: full replacement of the compaction prompt — the adapter can render
  a ws-specific compaction instruction through `ws/playbook.print` (or a
  static SKILL.md) and hand it to opencode.

Compaction cadence itself is config: `compaction: { auto, tail_turns }`.

**Consequence for the candidate architecture choice**

The "partial goal loop" concern was a real argument for choosing B (shallow,
no v2). With re-entry verified as full, B's only remaining justification is
pure harness-neutral symmetry — and symmetry costs the tier swap, dynamic
command registration, AND the goal loop. This re-weights the A/B/C decision
toward C: the full goal-loop surface is reachable only through v2 SDK calls
and v1 hooks that B explicitly forgoes.

The thin-adapter discipline still applies: the compaction prompt replacement
comes from `ws/playbook.print` or a static file, never inlined; the
autocontinue `enabled` flip is a single boolean read from ws goal state, not
a prose dependency.

### Goal-loop integration design (working agreement, 2026-08-01)

The session surfaces above are verified to exist; this section records how the
adapter uses them to drive the ws goal loop. It is a working agreement, not a
verified implementation — runtime behavior of `overflow` flag propagation and
`V2SessionCompact` interaction with `autocontinue` remains open.

**Design choice: in-session judgment turn (C').** An earlier draft considered
forking the session (`Session.fork({ sessionID, messageID? })` — verified to
exist, creates a child session at a message point) to run the judgment query
in a child so the parent context is never entered by the judgment prompt.
Rejected as the initial draft: fork duplicates the full parent context for
one extra turn of inference, and requires the adapter to manage two live
sessions simultaneously. The in-session alternative is simpler and cheaper.

**Marker protocol.** The adapter does NOT demand a bare single-token
response — models frequently violate "output only X" instructions. Instead
the judgment prompt instructs the lead to emit a marker of the form
`$goal-response:<token>` somewhere in its output, where `<token>` is one of
four tokens chosen for low collision with ws internal vocabulary:

| token | meaning | agent behavior |
|---|---|---|
| `achieved` | goal complete | stop after the marker |
| `next-step` | accept compaction, await next goal turn | stop after the marker |
| `keep-working` | do not compact, resume work in this turn | emit the marker, then continue working in the same turn |
| `pause` | await user intervention | stop after the marker |

`proceed` was rejected as the "keep-working" token because it collides with
the `lead-proceed` skill name strongly bound in this workflow; the model could
misread the marker as a skill invocation. `next-step` is kept despite the
`step` overlap because the compound `next-step` does not match any skill name
and the natural-language tone aids instruction compliance. `keep-working` is
a verb-phrase command form the model reads as an instruction, not a label.

The four tokens map to three adapter branches plus one special case:

1. `session.idle` fires — the lead's turn has ended.
2. Adapter injects a **judgment turn** into the same session via
   `Session.prompt({ sessionID, delivery: "queue", prompt: <judgment prompt> })`.
   The judgment prompt carries the current ws goal, the compression-safety
   heuristic ("phase boundaries / lead-proceed merge gates are normally safe
   to compact; if the turn stopped for a non-phase reason, compaction is
   unsafe"), and an **explicit** instruction: emit `$goal-response:<token>`
   with one of the four tokens; for `achieved`, `next-step`, or `pause` stop
   after the marker; for `keep-working` emit the marker and then resume work
   in the same turn. `next-step` is the default when uncertain.
3. `session.idle` fires again — the adapter parses the marker from the
   turn's output text (regex `$goal-response:(\w+)`, tolerant of surrounding
   prose / whitespace / trailing punctuation).
4. Adapter branches:
   - `achieved` — no further prompt; goal ends.
   - `next-step` — `V2SessionCompact` triggers compaction, then
     `Session.prompt({ delivery: "queue" })` re-enters the next goal turn.
   - `keep-working` — **adapter takes no action**; the lead already resumed
     work in the same turn. The adapter returns to step 1 on the next
     `session.idle` (state machine resets).
   - `pause` — no further prompt; user intervention awaited.
5. **Protocol violation recovery.** If `session.idle` fires with no marker
   in the output (the lead ignored the instruction and just continued
   working or stopped without emitting), the adapter re-injects the
   judgment turn (step 2). **Loop guard:** within a bounded time window
   (e.g. N seconds), if the adapter re-injects more than K times without
   observing a marker, it falls back to treating the situation as
   `next-step` (the default) and proceeds with compaction. This bounds the
   worst case: a model that persistently refuses the marker cannot wedge
   the goal loop indefinitely; the cost is one unintended compaction.

The judgment-turn response does enter the parent context, but the next turn
either compacts it away (`next-step`) or the lead resumes work in the same
turn (`keep-working`) so the marker and judgment prompt are soon followed by
further work output that dwarfs them. The residual is bounded by one
judgment turn's worth of text.

**autocontinue role — resolved by source inspection (2026-08-01).** Direct
reading of `packages/opencode/src/session/compaction.ts` in the opencode repo
confirms the `overflow` flag discriminates exactly as the design needs; no
runtime probe required. The flow:

- The adapter's `next-step` path calls `V2SessionCompact` (manual compaction)
  without passing `overflow`. The compaction `process` call receives
  `overflow: undefined`; the autocontinue hook is invoked with
  `overflow: input.overflow === true` → `false`. The adapter returns
  `enabled: false` (it owns the next `Session.prompt`) → synthetic continue
  skipped. No duplicate re-entry.
- opencode's own automatic compaction fires on `isOverflow` (context near
  model limit, driven by `compaction.tail_turns` / `preserve_recent_tokens` /
  `compaction.prune`). That path passes `overflow: true` through `create` →
  `process`. The adapter returns `enabled: true` → synthetic continue ejects
  the session forward.

A correction to the earlier draft: opencode's overflow autocontinue is not a
bare "continue" message. When `overflow: true` AND the most recent user
message is replayable (not itself a compaction), opencode **replays** that
user message back into the session as a fresh user turn and does NOT call the
autocontinue hook at all (the `replay` branch in `processCompaction`).
autocontinue's synthetic "Continue if you have next steps…" text only fires
when `overflow: true` AND no replayable user message exists. So the
autocontinue `enabled` flip the adapter makes is only meaningful for the
narrow `overflow: true && !replay` case; in the `replay` path the hook is
never invoked and the adapter's `enabled` value is moot. The design still
holds — the adapter always returns `enabled: false` from its own
`next-step`-triggered compaction (`overflow: false` path), and the replay
behavior is orthogonal (replay only happens on `overflow: true`, which the
adapter's manual compaction never sets).

Compaction cadence knobs the adapter should surface to ws config:
`compaction.tail_turns` (default 2), `compaction.preserve_recent_tokens`
(default auto, 2k–8k based on model usable context), `compaction.prune`
(tool-output pruning). The adapter does not need to call these directly;
they live in `opencode.json` and the user is the final authority, same as
Q6's model-list posture.

**Compaction prompt customization.** The adapter registers
`experimental.session.compacting` to inject ws state into the compaction
prompt: `context: [<current ticket focus>, <session key>, <pending todo
items>]`, or `prompt: <ws-specific compaction instruction from
ws/playbook.print>` for full replacement. This is the thin-adapter
discipline: the prompt body comes from MCP or a static file, never inlined
in the adapter TS.

**Concurrency note.** The judgment turn is a serialized insert into the
session's own queue (`delivery: "queue"`), not a parallel call. The adapter
does not need to manage concurrent session turns — `session.idle` gates each
step. The two live sessions of the fork alternative are avoided entirely.

### Conflict with the 260605 harness-neutral doctrine

The 260605 pivot frames ws as a **prompt factory** with harness differences
expressed as data (terminology tables, model aliases). v2's `agent.update()`,
`command.update()`, and `catalog.model.default.set()` are imperative
harness-internal state mutation, not data. If ws uses these deeply, opencode
becomes a first-class integration target that requires harness-specific TS code,
while claude/codex remain thin. This breaks the symmetry implied by
"Codex-first candidate before converging with the existing Claude plugin".

## Candidate architectures

### A. Harness-specific deep adapter package (`agents-plugin-opencode/`)

Create a sibling package to `agents-plugin/` and `agents-plugin-wsflow/`
(e.g. `agents-plugin-opencode/`). It contains:

- a v2 server plugin TS that wires ws into opencode: MCP registration, skill
  sources, command routing, tier default swap, `session.idle` sentinel handling;
- an opencode.json template / install instructions;
- shared prose, rsrc, and MCP launcher still re-used from `agents-plugin/`.

Pros:

- Keeps the ws harness-neutral core in `agents-plugin/`.
- Lets ws use all opencode v2 capabilities (tier swap, TUI later, etc.).
- Mirrors the existing `agents-plugin-wsflow/` derivative packaging pattern.

Cons:

- 260605 "harness-neutral / prompt factory" doctrine needs an explicit
  exception for opencode.
- Recurring maintenance tax for the adapter layer.

### B. Shallow drop-in (v1 hooks + static config only)

Use only:

- `opencode.json` `mcp:` entry pointing to the ws MCP launcher;
- `skills.paths` pointing to `agents-plugin/skills/`;
- optional v1 hooks for light guardrails;
- no v2 state mutation.

Pros:

- Codex/Claude/opencode stay structurally symmetric.
- 260605 doctrine preserved.
- Almost no new code.

Cons:

- No model-tier control (delegates run at lead model).
- Goal loop depends on weak `experimental.text.complete` soft sentinel or manual
  user re-invocation.
- Cannot register slash commands dynamically.

### C. Thin boundary-layer shim (recommended if A is chosen)

Same physical shape as A, but the adapter is constrained:

- NEVER inlines playbook prose or skill procedure bodies.
- NEVER copies workflow state-machine logic.
- Only registers protocol wiring: command names, agent prompt stubs that route
  to `ws/playbook.print`, catalog tier-default swap, `session.idle` reminder
  injection.
- All procedural knowledge remains in `agents-plugin/rsrc/` and
  `agents-plugin/skills/` and is reached through MCP.

This gives the capability of A while keeping the maintenance tax below that of
the existing `agents-plugin-wsflow/` byte-identical rsrc mirror.

## Thin adapter design principle

To keep prose/MCP churn from frequently breaking the opencode A-layer:

1. **Never inline prose.** All playbook/skill content comes from
   `ws/playbook.print`, `ws/playbook.render`, or static `SKILL.md` files.
2. **Discover skill inventory.** Generate the opencode command list from
   `agents-plugin/skills/manifest.json` or a directory scan, not hard-coded
   names.
3. **Swap the default model only for the duration of a task.** Restore the
   original default immediately after the `task` call returns.
4. **Pin and fail loudly.** Validate the adapter against
   `agents-plugin/runtime.json` `plugin_version` on startup so a core schema
   rename does not silently break behavior.

Under this discipline, the unavoidable opencode-specific churn is limited to:

- skill inventory renames (covered by manifest-driven command generation),
- major MCP tool/parameter renames (caught by version pin),
- auth/session model evolutions (rare, bounded by the adapter's prompt stub).

## Open questions

1. **v1/v2 interop — resolved.** A single plugin can register v1 hooks
   (`experimental.session.compacting`,
   `experimental.compaction.autocontinue`) and call the v2 SDK
   (`Session.prompt({ delivery: "queue" })`) together. The two APIs are
   layered, not mutually exclusive. Verified by reading
   `@opencode-ai/plugin@1.18.9` `Hooks` interface and `@opencode-ai/sdk`
   `Session.prompt` signature. The "Goal-loop and compaction surfaces" section
   above exercises both in one design.
 2. **Skill/command naming conflict — resolved (2026-08-01, three spikes).
     The `lead-` prefix is intentional as a
     collision-avoidance namespace and is kept as-is for opencode flat
     names (`lead-discuss`, `lead-implement`, etc.). **Grand principle
    (user directive):** the opencode plugin must be **non-invasive** to
    existing ws-mcp, playbook, and rsrc — dependency is one-directional
    (opencode → ws-mcp, never the reverse). The adapter may rename skills
    at registration (e.g. `ws-lead-discuss`) without touching source files,
    but must not require ws-mcp/playbook/rsrc changes to accommodate
    opencode.

    **Spike findings (2026-08-01) on prose references:** the codebase
    already has a namespace-substitution mechanism
    (`WS_MCP_NAMESPACE` env var + `wsNamespaceRef` regex in
    `server.go:480-483` + `{{.SkillNamespace}}`/`{{.McpNamespace}}`
    template vars in `playbook_tools.go:169-175`). However it has three
    gaps for the opencode case:
    - It emits **colon-form** (`ws:lead-discuss`), never hyphen-form
      (`ws-lead-discuss`). If opencode requires hyphen-form, even
      templated rsrc references break.
    - It only rewrites the `ws/`/`ws:` prefix token and a fixed phrase
      list — it does **not** rewrite bare skill names (`lead-proceed`).
    - It is applied to MCP tool descriptions and templated rsrc playbook
      bodies, but **not to `skills/*/SKILL.md` prose**, which is served
      by the host plugin loader directly (no template vars, static
      literals: `ws/playbook.print(...)`, `/ws:mcp-server-repair`, bare
      `lead-write-ticket`).

    **Concrete break points if adapter renames to `ws-<name>` at
    registration only (without touching prose):**
    - 12× `/ws:mcp-server-repair` slash-command fallbacks in
      `skills/*/SKILL.md` (all break).
    - Bare skill-name hand-offs in
      `skills/lead-drain-ready-queue/SKILL.md:26,35,37,78,108,109` and
      `skills/lead-bootstrap/{WORKFLOW,AGENTS.template}.md` (break).
    - Templated rsrc references render to colon-form — safe only if
      opencode accepts colon-form skill names; break if it requires
      hyphen-form.
    - MCP-tool invocations (`ws/playbook.print(name: "lead-proceed")`)
      are safe — those are MCP tool names (separate registration), and
      the `name:` argument is a playbook ID resolved by the ws MCP
      server, not by the host skill registry.

    **Non-invasive resolution — confirmed design (2026-08-01, three spikes):**
    Three parallel spikes resolved all open details:

    - **(a) opencode skill name format:** opencode requires
      `^[a-z0-9]+(-[a-z0-9]+)*$` — **colon-form (`ws:lead-discuss`) is NOT
      supported**, only hyphen-form (`ws-lead-discuss`) or bare. No namespace
      concept; plugin prefix is not auto-applied. So **all** references
      (SKILL.md prose AND rsrc templates) must produce hyphen-form on
      opencode.
    - **(b) false-positive risk:** a blind `lead-\w+` regex has ~8.6%
      false-positive rate (13 instances of non-skill words: `lead-owned`,
      `lead-only`, `lead-inline`, `lead-session`, `lead-capable`). An
      **allowlist-based regex** (union of `skills/manifest.json` +
      `rsrc/manifest.json` = 21 `lead-*` names + `mcp-server-repair`) has
      **0% false positives**. The rewriter must exclude `manifest.json`
      files (structural path-keys), skip already-prefixed occurrences
      (`ws:`, `/ws:`, `{{.SkillNamespace}}:`), skip frontmatter `name:`
      declarations (handle separately), and anchor with `[a-z]` after the
      hyphen (not `\w`) to avoid matching the `lead-*` glob literal in
      `delegate-orientation.md:21`.
    - **(c) skill prose load hooks:** **no load-time prose transform hook
      exists** in v1 `Hooks` (no skill hooks at all) or v2
      `skill.transform` (source-registration only, cannot read/mutate
      loaded `SkillV2Info` content). Three alternatives identified:
      1. Register embedded skills with pre-rewritten prose via v2
         `skill.transform` + `draft.source(EmbeddedSource)`.
      2. `experimental.chat.system.transform` — rewrites available-skills
         listing in system prompt (name/description only, not prose body).
      3. `experimental.chat.messages.transform` — rewrites already-emitted
         skill prose in message history (post-execute, every turn).

    **Converged adapter design for skill prose:**
    1. The adapter reads `SKILL.md` files from `agents-plugin/skills/`
       (file I/O in the plugin process, no source modification).
    2. Rewrites prose using an allowlist-based regex (from both manifests):
       `ws:lead-...` → `ws-lead-...`, `{{.SkillNamespace}}:lead-...` →
       `ws-lead-...`, bare `lead-...` → `ws-lead-...`.
    3. Rewrites frontmatter `name: lead-discuss` → `name: ws-lead-discuss`.
    4. Registers each as an embedded skill via v2 `skill.transform` +
       `draft.source(EmbeddedSource)` with the rewritten `SkillV2Info`.
    5. Registers `experimental.chat.system.transform` (v1 `Hooks`) as a
       safety net to rewrite the available-skills listing in the system
       prompt (in case embedded registration has ordering issues).
    6. Registers `tool.execute.after` (v1 `Hooks`) to post-process
       `playbook.print`/`playbook.render` MCP tool results, rewriting
       `ws:lead-...` → `ws-lead-...` in rendered playbook output (rsrc
       templates hardcode `:` as separator, so colon-form leaks into tool
       results on opencode).

    All rewriting is adapter-side; ws-mcp, playbook, and rsrc source files
    are untouched. The grand principle (one-directional dependency) is
    preserved.

    **Remaining concern:** embedded source vs file-based source collision.
    If the user places ws skills in `.opencode/skills/lead-discuss/`, the
    file-based source may override the adapter's embedded entry
    (last-source-wins in v2 `list`). Mitigation: document that ws skills
    should not be placed in `.opencode/skills/` when the ws plugin is
    active.
3. **Package layout and manifest — resolved (2026-08-01).** opencode
   requires **no manifest file**. Plugins are discovered by directory scan
   (`.opencode/plugins/*.ts` or `~/.config/opencode/plugins/*.ts`) at
   startup, or by npm package name listed in `opencode.json`'s `plugin`
   array (string or `[name, options]` tuple). The `.opencode/` directory +
   a plugin TS file is sufficient for local use; for npm distribution the
   package exports a `Plugin` function and the consumer adds the package
   name to `opencode.json`. There is no `plugin.json` equivalent, no
   `opencode install` CLI, and no runtime marketplace/registry —
   distribution is "publish to npm + list on the ecosystem docs page via
   PR." Local plugins are read fresh from disk each launch (no caching), so
   the version-bump/cache-invalidation discipline that Codex/Claude need
   does not apply to opencode local plugins. Plugin options arrive via the
   config tuple form `["<name>", { ...opts }]` and reach the plugin as the
   second argument (`PluginOptions`) — this is the natural channel for ws
   config (skill paths, tier defaults, ticket dir).
4. **MCP server lifecycle — resolved (2026-08-01).** Investigation of the
   `@opencode-ai/plugin` v1 and v2 surfaces confirms: **only the v1 plugin
   API can register a child MCP server at runtime**, via
   `input.client.mcp.add({ name, config: { type: "local", command,
   environment } })`. The v2 `PluginContext` has no `mcp` domain, no HTTP
   client, and no spawn/transport primitives (confirmed by issue #39937,
   which is an open feature request to add MCP registration to v2).
   **No plugin API (v1 or v2) lets the plugin own the child process
   lifecycle** — opencode owns spawn/stdio/reconnect/crash-recovery for any
   server registered via `mcp.add`. The plugin can observe
   (`event` hook → `mcp.tools.changed`) and trigger reconnect
   (`client.mcp.connect`/`disconnect`/`status`), but not manage the
   process. **Resolution (2026-08-01):** the ws adapter uses a single
   default export `{id, server, setup}` where the v1 `server` function
   calls `input.client.mcp.add(...)` for MCP registration (plugin-owned,
   no static `opencode.json` `mcp` entry needed; env injected from
   `ctx.options`) and the v2 `setup` function uses `PluginContext` for
   domain mutation. Process lifecycle remains opencode-owned — this is an
   architectural constraint of opencode, not a design choice. The user's
   "plugin-owned registration" preference is met; "lifetime clutter direct
   control" is not (opencode owns it), but reconnect trigger + observation
   via `event` hook partially compensates.
5. **TuiPlugin surface — resolved (2026-08-01).** Reserve a TUI slot for a
   later phase (e.g. a ticket-board sidebar). The opencode harness is
   treated as a more advanced integration target than Codex/Claude; a TUI
   surface is acceptable here even though it breaks symmetry with the
   other harnesses. Aligns with the 260605 pivot agenda item "TUI shape —
   dashboard deprecation target: lightweight process for status." The
   research ticket only records the reservation; the TUI surface itself is
   out of scope for the first implementation phase.
6. **Model-tier mapping — resolved.** Tier swapping honors
   `config.agents_tier` via the existing ws MCP `config.*` tools (keeps the
   tuning surface single-sourced across harnesses). The opencode adapter
   resolves a tier to a concrete `provider/model` through a mapping table whose
   initial values are derived from the `opencode models [provider]` CLI output,
   filtered to credentialed providers so unreachable models are not listed. At
   spawn time the adapter does `ctx.catalog.model.default.set(<concrete-model>)`
   (v2 `PluginContext` domain method, in-memory scoped — not global config file
   mutation) → spawn → restore the prior default; the spawned agent never sees
   the model name (consistent with the general principle that a configured agent
   does not need to know its own model). Model-list lookup is a convenience
   surface only; the user remains the final auditor of which concrete model each
   tier maps to, so the CLI-derived table is a starting point, not an
   authority.
7. **Auth/provider wiring — resolved (2026-08-01).** Stay on the existing
   MCP `config.*` tool surface. The adapter does not touch the v2
   provider/auth API. Rationale: the v2 API mental model is still weak for
   this project, and `config.*` keeps the tuning surface single-sourced
   across harnesses (consistent with Q6). No custom provider exposure is
   needed for the initial implementation.
8. **`overflow` flag propagation under manual compaction — resolved.**
   Source inspection of `packages/opencode/src/session/compaction.ts`
   (2026-08-01) confirms: `V2SessionCompact` without an explicit `overflow`
   argument passes `undefined` through to `process`, which coerces to
   `false` in the autocontinue hook (`overflow: input.overflow === true`).
   The adapter's `next-step` path therefore lands in the `overflow: false`
   branch and returns `enabled: false`; opencode's own automatic
   compaction lands in the `overflow: true` branch and returns
   `enabled: true`. No runtime probe needed. Additional finding: the
   `replay` path (most-recent-user-message re-injection on overflow) fires
   before the autocontinue hook and skips it entirely, but this only
   applies when `overflow: true`, which the adapter's manual compaction
   never sets — so the design is unaffected.
9. **Judgment-turn marker protocol.** Resolved in design (2026-08-01): the
   adapter uses a free-text marker `$goal-response:<token>` with four tokens
   (`achieved | next-step | keep-working | pause`) rather than a structured
   tool call or a bare single-token demand. A tolerant regex parser extracts
   the marker from surrounding prose. `keep-working` is special: the lead
   emits the marker and continues working in the same turn (adapter takes no
   action, resets on next idle). A loop guard (bounded re-injection count
   within a time window, fallback to `next-step`) prevents a model that
   persistently refuses the marker from wedging the goal loop. **Loop guard
   parameters (agreed 2026-08-01):** 10-minute sliding window, 5-re-injection
   threshold; on threshold breach, force `next-step` fallback. **Violation
   logging:** the adapter shell-outs to `ws-mcp log append warning "<msg>"`
   to emit guard-threshold violations. This CLI surface does not yet exist
   in ws-mcp and is tracked by sub-ticket
   `260801-todo-ws-mcp-log-append-cli` (under this research ticket's epic).
   The sub-ticket adds a `log` subcommand to `ws-mcp` backed by a new
   `RecordExternalLogEvent` sibling of `RecordLifecycleEvent` in
   `internal/mcp/server.go`, writing to
   `<cache-root>/crash/mcp-external.log` (append-only JSONL) with an
   in-memory ring mirror via `appendDebugEvent`. Until that sub-ticket
   lands, the adapter falls back to `console.warn` on the plugin process.

## Scope / sequencing notes

- This is a research ticket only. No implementation until the architecture
  (A/B/C) is chosen.
- If A or C is chosen, the first implementation phase should be a minimal
  plugin file default-exporting `{id, server, setup}`:
  - `server` (v1): registers the ws MCP launcher via
    `input.client.mcp.add(...)`, points skill discovery at
    `agents-plugin/skills/`, provides a single `/ws-discuss` command as
    proof-of-concept, and documents the thin-adapter rule.
  - `setup` (v2): registers agent/command/catalog defaults from rsrc
    manifest (thin-adapter rule #5).
  - The plugin returns `Hooks` (from `server`) with
    `experimental.session.compacting` / `experimental.compaction.autocontinue`
    for the compaction surface. Goal-loop uses `input.client.session.prompt`
    over HTTP.
- After the proof-of-concept, decide whether to invest in dynamic tier swap,
  `session.idle` sentinel handling, and TuiPlugin (Q5: reserve a slot).
- **Sub-ticket dependency:** `260801-todo-ws-mcp-log-append-cli` must land
  before the goal-loop guard violation logging is operational. Until then
  the adapter falls back to `console.warn`.
- Any implementation branch must update `ai-docs/spec/plugin-runtime.md` and
  `ai-docs/mental-model/plugin-topology` (or create it) to reflect that opencode
  is now an explicit — and asymmetric — harness target.

## Decision needed

Architecture choice has converged toward **C** (thin boundary-layer shim) in
discussion (2026-08-01). **Entry-point design (confirmed 2026-08-01 by source
inspection):** the adapter uses a **single default export
`{id, server, setup}`** that satisfies both opencode plugin loaders
simultaneously:
- **v1 `server`**: `async (input, opts) => Hooks` — receives `input.client`
  (HTTP client). Uses `input.client.mcp.add(...)` for ws MCP registration
  (v1-only capability). Returns `Hooks` with `experimental.session.compacting`,
  `experimental.compaction.autocontinue`, and other v1 hooks. Uses
  `input.client.session.prompt(...)` / `input.client.session.*` for goal-loop
  SDK calls.
- **v2 `setup`**: `async (ctx: PluginContext) => void` — receives the v2
  `PluginContext` with domain drafts. Uses `ctx.agent.update(...)`,
  `ctx.command.update(...)`, `ctx.catalog.model.default.set(...)`,
  `ctx.reference.add(...)` for in-memory domain mutation (v2-only capability).

Both loaders import the same module URL → shared module-level state (boot
ordering is concurrent/unordered, but MCP registration and domain mutation are
independent operations, so no race concern). **Caveat:** do not include an
`effect` field when using `setup` — the v2 Schema Union tries `effect` first
and would misinterpret the module as an Effect plugin. The object
`{id, server, setup}` is the correct shape.

This gives MCP registration (v1 `input.client.mcp.add`), tier swap
(v2 `catalog.model.default.set`), dynamic command/agent registration (v2
`command.update`/`agent.update`), references (v2 `reference.add`), and the
full goal-loop/compaction surface (v1 `experimental.*` hooks + v1 client
`session.prompt`/`V2SessionCompact`). B's symmetry is not worth losing all
three. A is ruled out by the thin-adapter discipline (prose inlining would
re-introduce the maintenance tax the user deemed unacceptable). C inherits A's
capabilities under the four thin-adapter rules, extended in discussion with two
more:

5. **Derive subagent/agent inventory from rsrc manifest + frontmatter** — the
   adapter generates `agent.update`/`command.update` registrations from
   playbook frontmatter, never hard-codes skill names. (Extends rule 2 from
   commands to agents.)
6. **Filter harness-irrelevant MCP tools** — the adapter may suppress
   `playbook.print`/`playbook.render` from `tools/list` on opencode, since
   the opencode skill loader reads prose directly from `SKILL.md`/rsrc and
   the MCP round-trip for prose is unnecessary on this harness.

## Monitoring items

- **opencode issue #39937** — "Add MCP registration to the V2 plugin context."
  Currently the adapter relies on v1 `server` for MCP registration and v2
  `setup` for domain mutation (coexistence in one default export). If #39937
  lands, MCP registration moves to v2 `PluginContext` and the adapter can
  drop the v1 `server` field entirely, simplifying to a pure v2 plugin. Watch
  for merge; the current coexistence design is a workaround until then.
- **v1 plugin API deprecation notice.** As of 2026-08-01, v1 is the documented
  primary API with zero `@deprecated` markers on `Plugin`/`Hooks` and no
  deprecation timeline in the changelog or docs. v2 is unreleased/undocumented
  (no `/docs/plugins/v2/` page, depends on `effect@beta`, still gaining context
  capabilities). v1 deprecation risk is low-to-none in the near term, but v2 is
  clearly the strategic direction (maintainer `kitlangton` is actively extending
  it). If a v1 deprecation notice appears, the adapter must migrate to v2
  entry point — at which point #39937 must have already landed (MCP
  registration in v2) or the adapter falls back to static `opencode.json` `mcp`
  declaration.
- **`effect@beta` dependency.** v2 pulls `effect@4.0.0-beta.83`. If the adapter
  ever migrates to v2, this beta dependency introduces a breaking-change risk
  on opencode version upgrades. Not a concern while the adapter stays on v1.
