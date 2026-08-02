---
title: "Research: ws pi-native framework — bridge ws-mcp onto Pi as the opinionated workflow layer"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260801-feat-ws-opencode-adapter: dropped — the opencode adapter direction superseded by this Pi-native framework; preserved in .dropped/ as historical design record
  260731-research-ws-opencode-drop-in-package: resolved opencode adapter research (done) whose accumulated-workaround findings motivated this pivot; historical reference
  260801-todo-ws-mcp-log-append-cli: dropped — sole consumer was the dropped opencode adapter; re-evaluate in Pi expansion phase if needed
  260605-research-ws-native-subagent-pivot: host-neutral pivot direction this research inherits; its "assume durable/retained subagents are a baseline harness capability" premise is corrected below for Pi (file-based, not in-memory)
  260605-epic-ws-playbook-factory-pivot: parent epic
  260611-research-ws-per-role-delegation-tuning-config: tier/model mapping research the pi subagent surface inherits
  260626-feat-session-key-format-and-retention: session-key/retention policy whose philosophy extends to ws subagent session cleanup
related-mental-model:
  - plugin-runtime
  - workflow-skills
  - named-agent-runtime
  - mcp-runtime
  - prompt-bundle
---

# ws pi-native framework — bridge ws-mcp onto Pi as the opinionated workflow layer

## Background

The opencode adapter investigation (`260731-research-ws-opencode-drop-in-package`,
done; `260801-feat-ws-opencode-adapter`, now dropped) converged on a thin
boundary-layer shim design, but the 2026-08-01 tier-agent spike revealed an
accumulated workaround cost on opencode: a v1/v2 API split, a `task` tool that
is v1-only and blind to v2-registered agents, a `client.config.update` that
persists to disk, and an unstable v2 surface (`effect@beta`, open #39937 for the
missing v2 MCP registration). The implementation ticket's "Harness tradeoff
note" raised the question of whether **Pi** (earendil-works, MIT, terminal
coding-agent harness) is a simpler target despite needing goal-loop/compaction
"built from scratch."

Re-reading the Pi docs shipped locally
(`/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/`)
corrected two load-bearing claims in that tradeoff note and reframed the
decision. A follow-on design discussion (2026-08-02) resolved the subagent /
continuation / tool-curation surface. This ticket records the corrected
comparison, the settled direction, and the open questions for the continuation
discussion. It is a research ticket: freeform topic sections, no implementation
phases. Code work is deferred until the discussion in "Open questions" converges.

## Settled direction (this session)

1. **Pi is the target harness.** The opencode adapter work is dropped
   (`260801-feat-ws-opencode-adapter` → `.dropped/`) and the ws workflow
   system is ported onto Pi as an opinionated framework layer.
2. **Bridge path (a), not native re-implementation (b).** ws-mcp stays as the
   harness-neutral MCP server (Go source untouched); a Pi extension bridges
   each ws-mcp MCP tool to a `pi.registerTool` call and drives the ws workflow
   surface (skills, playbook, goal-loop, compaction, tier/subagent) through Pi
   extension events. "Framework on a clean base" does not mean re-cutting the
   wheel — it means composing ws-mcp onto Pi's clean extension API.
3. **Sequencing**: MVP surface (skills + playbook + agent/tier + spawn/continue/wait)
   first, then feature expansion (always-visible TODO, goal-loop, compaction
   hooks, recursive explore, agentId persistence). Code work does not start
   until the discussion in "Open questions" converges.

## Corrected Pi capability matrix

The opencode tradeoff note's Pi column contained two factual errors. Direct
reading of the shipped Pi docs corrects them:

| Surface | opencode (verified 2026-08-01) | Pi (verified from shipped docs 2026-08-02) |
|---|---|---|
| Subagent / tier | v1 `config` hook timing hack; `task` v1-only; v1/v2 unbridged | frontmatter `model:` field (`pi-sub-agent` / `pi-sub-agents` packages; `examples/extensions/subagent/` reference, 1015 lines). No timing hack. **But: example uses `--no-session` → no continuation** (see "Subagent continuation" below). |
| MCP connection | v1 `input.client.mcp.add` (works); v2 has none (#39937) | **No built-in MCP client in core.** Explicit non-goal: `usage.md` states pi intentionally omits built-in MCP, sub-agents, plan mode, to-dos, background bash — "build or install those as extensions or packages." This is the load-bearing work item (see "MCP bridge" below). |
| Goal-loop | `session.prompt({delivery:"queue"})` + marker (surface exists, unstable) | `agent_settled` event (fires after auto-retry/auto-compaction/queued-continuation all drain — the judgment-turn trigger), `pi.sendUserMessage(text,{deliverAs:"followUp"})` (queued until idle), `input` event `streamingBehavior:"steer"\|"followUp"`. Re-entry primitive present; marker protocol portable from the opencode design but likely simpler. |
| Compaction | two hooks: `experimental.session.compacting` + `experimental.compaction.autocontinue`; overflow discrimination needs the `overflow` flag plumbing | **single hook** `session_before_compact` with `reason:"manual"\|"threshold"\|"overflow"` and `willRetry` directly on the event, plus `ctx.compact()` and custom summary injection. Cleaner than opencode — no autocontinue flip needed; overflow/retry is a first-class event field. |
| Skill / playbook | loader + prose rewriting pipeline (colon→hyphen, allowlist regex) needed because opencode rejects colon-form | **Agent Skills standard.** Hyphen-form names, `SKILL.md` + frontmatter. `resources_discover` event returns `skillPaths`; pi also loads `~/.claude/skills` and `~/.codex/skills` directly. ws `agents-plugin/skills/` (already hyphen-form) loads **without prose rewriting**. The entire allowlist-regex + `tool.execute.after` + `chat.system.transform` apparatus from the opencode design is **deleted** on Pi. |
| API stability | v2 unreleased/undocumented, `effect@beta`, unported `task` | single documented extension API, jiti hot-reload, `examples/extensions/` + `examples/sdk/` shipped. |
| Investigation invested | 9 questions fully resolved, source-verified | matrix doc-verified; subagent/continuation/curation design resolved in discussion. |

**Key reframes:**

- The opencode tradeoff note claimed "No compaction concept in Pi core —
  build needed." **Wrong.** Pi has a first-class compaction subsystem
  (`compaction.md`, `session_before_compact`/`session_compact`, `ctx.compact()`,
  `reason`/`willRetry`), and it is *cleaner* than opencode's two-hook +
  autocontinue dance.
- The note claimed goal-loop must be "built from scratch." **Partially
  wrong.** The re-entry primitives (`agent_settled`, `sendUserMessage`
  `followUp`, `input` `streamingBehavior`) exist in core; only the
  judgment-turn protocol (marker parsing, loop guard) is framework work —
  and that is the same work the opencode design already specified.
- The note correctly identified subagent/tier as trivially simple on Pi and
  MCP bridging as the real work. The MCP gap is now the central work item.
  Subagent continuation, however, is *not* trivial — see below.

## The central work item: MCP bridge

Pi has no built-in MCP client. The ws-mcp server (the harness-neutral MCP
tool surface: context, workflow state, git, docs, tickets, playbook, agents)
must be reached from a Pi extension. Two sub-paths remain compatible with
the settled bridge direction (neither touches ws-mcp Go source):

- **(a-i) MCP-stdio bridge extension.** A Pi extension spawns the ws-mcp
  launcher as a subprocess, speaks stdio MCP, and re-exports every ws-mcp
  tool as a `pi.registerTool` call whose `execute` proxies to the MCP
  server. ws-mcp source untouched. The extension owns the subprocess
  lifecycle (Pi extension owns process — unlike opencode, this matches the
  user's "plugin-owned registration" preference). Requires a TypeScript MCP
  client library (`@modelcontextprotocol/sdk` is npm-installable; Pi allows
  npm deps in extension `package.json`).
- **(a-ii) Thin TCP/HTTP relay.** If ws-mcp exposes (or can expose without
  source churn — it already has launcher env knobs) a streamable-HTTP or TCP
  transport, the Pi extension connects over that instead of stdio. Lower
  process-management burden, but depends on ws-mcp's transport surface
  (currently stdio-focused; see `260513-research-streamable-http-mcp-transport`).

**Decided: (a-i) stdio MCP bridge.** The extension spawns the ws-mcp
launcher as a subprocess, speaks stdio MCP, and re-exports every ws-mcp
tool as a `pi.registerTool` call whose `execute` proxies to the MCP
server. ws-mcp source untouched. The extension owns the subprocess
lifecycle. The lightweight JSON-RPC framing logic lives in the TS
extension itself — ws-mcp's tool-list schema is small enough that a
minimal stdio client is straightforward, and we avoid a heavy MCP SDK
dependency. A TCP/HTTP relay (a-ii) remains a possible later optimization
if stdio lifecycle proves painful, but is not the starting path.

**Tool-name surface.** ws-mcp tools are invoked as `ws/playbook.print`,
`ws/tickets.create`, etc. (colon/slash form in prose). On Pi these become
`pi.registerTool({ name: "..." })` — the bridge chooses the registered
name. Since Pi does not reject colon-form (no skill-name regex constraint
applies to tool names, only to skill names), the bridge *may* preserve the
`ws/...` tool names verbatim, which means **ws skill/playbook prose needs no
rewriting at all** — the MCP tool calls in SKILL.md bodies keep working as
literal `pi` tool calls. This is a major simplification over the opencode
design and needs runtime confirmation (does `pi.registerTool` accept `/` in
`name`? the skills name regex is `^[a-z0-9-]+$` but tool names are a
different surface — verify against `examples/extensions/dynamic-tools.ts`).

## Subagent surface — continuation, spawn API, curation

The `examples/extensions/subagent/` reference (1015 lines, shipped with Pi)
implements frontmatter-driven subagents: `name`/`description`/`tools`/`model`
fields, subprocess `pi --mode json -p --no-session --model <m> --tools <list>
--append-system-prompt <tmp> "<task>"`, parallel/chain modes. It is a close
match to ws needs but has two gaps that force ws-specific work: **no
continuation** (`--no-session` = ephemeral) and **static frontmatter system
prompts** (ws needs dynamic `playbook.render` content).

### Subagent continuation — file-based session resume

**260605 premise correction.** The 260605 pivot assumed "durable/retained
subagents are a baseline harness capability" and verified this on Claude
Code (`SendMessage(to: agentId)` resumes a completed agent from transcript
in the background). **Pi has no in-memory background-resume primitive.**
Subagents are per-invocation subprocesses. Pi's continuation model is
**file-based**: `pi --session <path|id>` / `pi -c` / `SessionManager.open`
resume from a `.jsonl` session file. A subprocess that exits can be resumed
by a new subprocess pointing at the same session file.

**Decision: continuation = file-based session resume (option 1).** Drop the
example's `--no-session`; spawn with `--session <ws-owned-path>`. ws-mcp
stores the `agentId ↔ sessionPath` mapping. Resuming = new `pi` subprocess
with the same `--session`. This achieves 260605 "durable" *via disk
persistence* rather than in-memory background resume. The 260605 premise is
met in spirit (subagent state survives process death) but the mechanism
differs — file-based, not memory-based. This is an explicit correction to
the 260605 assumption for the Pi harness.

**Session hidden from user `/resume` picker.** The user wants ws subagent
sessions to *not* appear as noise in `pi -r` / `/resume`. Verified from
`dist/core/session-manager.js` and `dist/config.js` (2026-08-02):

- `getSessionsDir()` = `getAgentDir() + "/sessions"` = `~/.pi/agent/sessions/`.
- `SessionManager.listAll()` scans **only direct child directories** of
  `getSessionsDir()` (`entries.filter(e => e.isDirectory())` under
  `~/.pi/agent/sessions/`).
- `SessionManager.list(cwd, sessionDir?)` and `.create(cwd, sessionDir?)`
  accept an explicit `sessionDir` that overrides the default
  per-cwd directory.

Consequence: ws subagent sessions placed at
`~/.pi/agent/ws-sessions/<agentId>.jsonl` (sibling of `sessions/`, **not** a
direct child of `sessions/`) are **never discovered** by `listAll()` or the
default `list(cwd)` path the picker uses. No "hidden" flag or filter is
needed — the directory structure itself guarantees isolation. (Stronger
alternatives exist: a separate `PI_CODING_AGENT_DIR` for subagents isolates
skills/extensions too, but that is heavier than needed; MVP uses the sibling
directory.)

### Spawn API — async fan-out with selective polling

**Decision: three async tools exposed to the lead.** The example is
synchronous (`await` blocks until the subprocess finishes); ws needs
fan-out, so the ws tools are async with a selective-wait polling model:

```
ws-agent-spawn({ playbook, task, tier? })      → { agentId, status: "running" }   // returns immediately
ws-agent-continue({ agentId, task })           → { status: "running" }            // only valid when agent is "done"
ws-agent-wait({ agent-ids: [...], policy: "any"|"all", timeout?: sec })
                                               → { done: {agentId: output}, running: [...], timedOut: [...] }
```

This matches Claude Code's fan-out pattern (lead as orchestrator: A spawn →
B spawn → other work → wait [A,B]) and is strictly more capable than the
example's blocking `tasks:[]` batch.

**Internal mechanism (ws-specific code beyond the example):**
1. **Module-state registry**: `Map<agentId, { proc, sessionPath, systemPromptPath, state, outputAccumulator, stdoutDrainer }>` held at extension module scope.
2. **Background stdout draining**: each subprocess's stdout is read continuously (to avoid pipe-buffer deadlock) and JSON events parsed; `message_end` with terminal `stopReason` flips `state: "done"` and freezes output.
3. **`session_shutdown` cleanup**: kill any in-flight subprocesses (Pi extension doc pattern: session-scoped resources close on shutdown).

**Derived design points:**
- **continue is allowed only when `state: "done"`**: two `pi` processes writing the same session file concurrently is a race. ws's per-agent state enforces "wait before continue." MVP rule; simple and safe.
- **wait timeout does not kill**: on timeout, return the current partition (done/running/timedOut) but leave subprocesses running. The lead decides (re-wait, abandon, or later kill). Killing is deferred to `session_shutdown` (or a later `ws-agent-cancel` in expansion).
- **agentId persistence is MVP-out**: MVP keeps the registry in-memory; when the lead session ends, subagents end with it. Persisting `agentId ↔ sessionPath` into ws-mcp state would let a restarted lead resume subagents — a stronger 260605 "durable" — but is expansion scope. Open.

### System prompt injection — playbook.render into the spawn

**Decision: no Pi-specific "playbook→agent name spawn" method.** Rejected in
favor of ws's existing `playbook.render → spawn-time path injection`
methodology, unified across Claude/Codex/Pi. This preserves harness
neutrality: ws-mcp/playbook source stays harness-agnostic, the adapter
bridges.

**Decision: ws custom spawn injects `playbook.render` result as the system
prompt path, eliminating one tool call.** The example writes a static
frontmatter body to a tmp file and passes `--append-system-prompt <path>`.
ws replaces the static body with the dynamic `ws/playbook.render(name)`
result: the subagent starts with "what I should do" already in its system
prompt, instead of calling `playbook.render` itself as a first tool turn.
One fewer tool call + less instability.

**Playbook is rendered once at spawn, frozen for the agent's life.** The
rendered playbook is written to a stable path
(`~/.pi/agent/ws-sessions/<agentId>/system-prompt.md`) and **re-rendered is
not called on `continue`** — the same path is reused. Agent identity is
frozen at spawn time; if the playbook updates mid-session, this agent keeps
the old prompt. For a refreshed prompt, spawn a new agent.

**System-prompt duplication is prevented by Pi's session model, not by ws.**
Pi reconstructs the system prompt ([default] + [`--append-system-prompt`
content]) from the same inputs on every resume; it does not accumulate the
system prompt as a message in the session file. So ws reusing the same
`--append-system-prompt <path>` on every `continue` yields an identical,
non-duplicated system prompt. ws's only job is "keep using the same path."

**`--append-system-prompt` (append) vs `--system-prompt` (replace).** The
example uses append; `--system-prompt` replaces the default but keeps
"context files and skills" (`usage.md:240`). MVP starts with
`--append-system-prompt` (the example's verified path); switching to
`--system-prompt` for a leaner prompt is a later tuning decision.

**Open: `pi --mode json -p --session <path>` resume-time turn accumulation.
** The example uses `--no-session` and never exercises `--session` + `-p`
together. Runtime spike required: does resume via `--session <path>` under
print mode **append** a new turn to the file (desired) or overwrite/start
fresh (failure)? This is the one unverified assumption in the continuation
design.

**Open: session-file flush ordering vs process exit.** When `wait` harvests
a subprocess's output and the lead immediately calls `continue` on that
agent, is the session file fully flushed to disk by the time the new
`--session <path>` subprocess opens it? Process exit normally implies
flush, but this must be verified at runtime. (Separate from the
turn-accumulation spike above.)

### Recursive explore subagent — depth-bounded restricted recursion

**260605 assumption correction.** The 260605 pivot assumed "subquery is
absorbed into harness-native Explore subagents (Claude and Codex both
expose an Explore-style agent type)." **Pi has no harness-native Explore.**
Pi intentionally omits sub-agents from core (`usage.md`). Consequently ws
must **provide Explore itself** on Pi — this is concrete "opinionated
framework on clean base" work: what Claude/Codex get from the harness, ws
must build on Pi.

**Decision: depth-bounded restricted recursion (max depth 2).** A spawned
worker (depth 1) may itself spawn `explore`-only subagents (depth 2); depth
2 is a true leaf (no spawn tool exposed). This lets a worker delegate
high-noise recon to a cheap leaf instead of polluting its own context.

| depth | role | `ws-agent-*` tools exposed | spawn-allowed playbooks |
|---|---|---|---|
| 0 | lead | spawn / continue / wait | all (implement, review, explore, discuss, ...) |
| 1 | worker | spawn / continue / wait | **explore-family only** (curation-enforced) |
| 2 | explore | **not exposed** | — (true leaf) |

**Dual defense for depth-2 tool hiding:**
1. **`--tools` filter (first defense):** the spawn handler sets the child's
   `--tools` allowlist. An explore agent's frontmatter/curation declares
   `tools: read, grep, find, ls, bash, web_search` — `ws-agent-spawn` is
   absent, so the child *cannot* spawn even if it tried. Verified:
   `usage.md:209` states `--tools` filters "built-in, extension, and
   **custom** tools."
2. **Extension policy (second defense):** the ws extension does not
   register `ws-agent-*` tools when `WS_AGENT_DEPTH >= 2`. Even if a
   future Pi change allowed self-registration, the depth env gates it.

`WS_AGENT_DEPTH` is passed parent→child as an env var; the ws extension
reads it at load time and configures tool registration accordingly.

### Tool-group + playbook curation — adapter-owned (golden rule)

**Golden rule (non-invasive, one-directional dependency):** inherited from
the opencode adapter (Decision 9 there): ws-mcp, playbook, and rsrc source
files are never modified to accommodate a harness. Dependency is
one-directional (adapter → ws-mcp). The opencode adapter applied this to
prose rewriting; the Pi adapter applies the same rule to **tool
filtering, skill exposure, and spawn policy**.

**Decision: tool-group + playbook-curation live in the Pi adapter, not in
ws-mcp.** ws-mcp/playbooks remain harness-agnostic and carry no knowledge
of "how harness X filters tools" or "what depth means." The Pi adapter owns
the curation tables:

```
# tool-group → Pi tool names (explicit allowlist)
tool-groups:
  read-only:    [read, grep, find, ls]
  recon:        [read, grep, find, ls, bash, web_search]
  full-worker:  [read, bash, edit, write, grep, find, ls]   # default edit tools only, NO ws-mcp MCP tools
  none:         []

# playbook → { tool-group, tier, max-depth, skills }
#   tool-group omitted  => sentinel "all tools" (lead only); spawn omits --tools
#   skills omitted      => sentinel "skill discovery on" (lead only); spawn omits --no-skills
#   skills: none        => spawn passes --no-skills
#   skills: [<paths>]   => spawn passes --no-skills --skill <p1> --skill <p2> ...
playbook-curation:
  # lead playbooks (depth 0) — tool-group/skills omitted = full access sentinels
  lead-implement: { tier: large,  max-depth: 1, skills: none }   # lead itself needs no skills; subs it spawns are restricted
  lead-discuss:   { tier: medium, max-depth: 1, skills: none }
  # subagent playbooks (depth >= 1) — explicit restricted groups
  explore:   { tool-group: recon,       tier: light,  max-depth: 2, skills: none }
  implement: { tool-group: full-worker, tier: large,  max-depth: 1, skills: none }
  review:    { tool-group: read-only,   tier: medium, max-depth: 1, skills: none }
  discuss:   { tool-group: none,        tier: medium, max-depth: 1, skills: none }
  ...
```

When `ws-agent-spawn({ playbook, task, tier? })` is called, the adapter
looks up the curation to resolve `--tools`, `--model` (from tier mapping),
`WS_AGENT_DEPTH`, `--no-skills`/`--skill`, and validates `depth < max-depth`
(reject if the caller's depth+1 would exceed the playbook's `max-depth`).

**Sentinel semantics:**
- `tool-group` **omitted** = "all tools" sentinel. The spawn handler passes
  *no* `--tools` flag, so Pi exposes every registered tool. Reserved for
  lead (depth 0); if a subagent playbook omits `tool-group`, the adapter
  rejects it (subagent must have an explicit group). Rationale: a hardcoded
  "all tools" list would need updating whenever Pi or the bridge adds a
  tool; omission is self-maintaining.
- `skills` **omitted** = "skill discovery on" sentinel. The spawn handler
  passes no `--no-skills`; Pi's normal discovery runs. Reserved for lead.
- `skills: none` = `--no-skills` (no skill commands, no skill descriptions
  in the system prompt).
- `skills: [<paths>]` = `--no-skills --skill <p> ...` (whitelist; additive
  per `skills.md:34`). MVP uses `none` for all subagent playbooks.

**Triple control (depth + tool-group + playbook-curation):**
- depth 0 lead spawns `implement`: curation OK (`max-depth: 1` ≥ 1) → `--tools full-worker`, `--no-skills`.
- depth 1 worker spawns `explore`: curation OK (`max-depth: 2` ≥ 2) → `--tools recon`, `--no-skills`, `WS_AGENT_DEPTH=2`.
- depth 1 worker spawns `implement`: curation rejects (child depth 2 > `max-depth: 1`).
- depth 2 explore: spawn tool not exposed (dual defense above).

**Subagent cannot bypass lead authority via three routes:**
1. `/skill:lead-implement` (skill command) → blocked: subagent playbooks
   use `skills: none` → `--no-skills` disables skill discovery and commands.
2. `ws/playbook.render(name:"lead-implement")` (MCP tool) → blocked: ws-mcp
   MCP tools (`ws/*`) are **only in the lead's tool set** (depth 0 uses the
   omitted-tool-group sentinel = all tools; subagent tool-groups
   `full-worker`/`recon`/`read-only`/`none` never list `ws/*` tools). The
   spawn handler's `--tools` filter excludes them, and `usage.md:209`
   confirms `--tools` filters custom tools too.
3. `ws-agent-spawn` (custom tool) → blocked at depth 2 (not exposed, dual
   defense above) and blocked at depth 1 for non-explore playbooks
   (curation `max-depth` check).

**Trade-off accepted — split management point.** Adding a new playbook to
ws-mcp requires one corresponding line in the Pi adapter's
`playbook-curation`. If missing, spawn of that playbook is rejected (safe
default). The two sides do not auto-sync. This is the cost of the golden
rule: ws-mcp staying harness-neutral means harness-specific policy lives in
the adapter. Mitigated by the ws-mcp version pin (the adapter validates
`runtime.json` `plugin_version` on startup — rule 4 "pin and fail loudly"
inherited from the opencode design). **Justification: golden-rule violation
cost > split-management cost.** Putting tool/depth policy into ws-mcp would
force ws-mcp to know "how harness X filters tools and skills," breaking the
harness-neutral "prompt factory" doctrine (260605).

**Decision: curation is a data file, not hardcoded.** The curation tables
live in a data file (e.g. `curation.json` / `curation.yaml`) alongside the
Pi extension, not hardcoded in extension TS. Data/code separation enables
hot-reload and non-programmer edits. A hardcoded constants fallback was
considered and rejected as the primary form; a ws-mcp-owned location was
rejected as a golden-rule violation.

## MVP scope (decided, sequencing only — no code yet)

User-directed order: MVP surface first, feature expansion second.

**MVP — skills + playbook + agent/tier + spawn/continue/wait:**
- MCP stdio bridge extension (a-i, decided) embedding a lightweight
  JSON-RPC stdio client, spawning ws-mcp and proxying each MCP tool via
  `pi.registerTool`.
- `resources_discover` returning `agents-plugin/skills/` as a skill path
  (zero prose rewriting, if tool-name preservation holds).
- Tier/subagent definitions via curation data file (tool-groups +
  playbook-curation), resolved to `--model`/`--tools` at spawn.
- `ws-agent-spawn` / `ws-agent-continue` / `ws-agent-wait` async tools
  (depth 0→1 leaf worker; no recursive explore yet).
- File-based continuation: `--session ~/.pi/agent/ws-sessions/<agentId>.jsonl`,
  `--no-session` removed, playbook rendered once to
  `~/.pi/agent/ws-sessions/<agentId>/system-prompt.md` and reused on continue.
- One proof-of-concept command (e.g. `/ws-discuss`) via `pi.registerCommand`.

**Feature expansion (after MVP validates the bridge + spawn):**
- Recursive explore: depth-1 worker spawns depth-2 explore leaf (260605
  subquery absorption, reconstructed on Pi).
- Always-visible TODO / workflow-board surface (Pi has no built-in todo;
  `ui.setWidget` footer or `ui.custom` component — exact shape decided at
  implementation time. Designed fresh on Pi; the 260514 dashboard epic is
  closed and not absorbed).
- Goal-loop: `agent_settled` → judgment turn → `ctx.compact()` + re-enter.
  The judgment-turn signal is **redesigned from scratch** on Pi (the
  opencode four-token marker protocol is discarded — it carried
  opencode-structure hacks). Candidate: a judgment tool call registered
  by the extension, since Pi extensions can register tools the lead calls
  to signal goal state. Designed in the expansion phase.
- Compaction hooks: `session_before_compact` for ws-state injection and
  custom summary; `reason`/`willRetry` replace the opencode autocontinue
  flip.
- agentId persistence: `agentId ↔ sessionPath` stored in ws-mcp state, so
  a restarted lead can resume subagents (stronger 260605 "durable").
- (Optional, later) TUI panel via `ctx.ui.custom`.

## Relationship to existing doctrine

- **260605 harness-neutral doctrine.** ws-mcp stays harness-neutral (Go
  source untouched; the bridge is a Pi-side adapter). The 260605 "prompt
  factory, harness differences as data" doctrine is *preserved* under the
  bridge path — this is the reason (b) native re-implementation was rejected.
  Pi becomes an explicit asymmetric harness target (like opencode would
  have been), but the asymmetry lives in the bridge extension, not in ws-mcp.
- **260605 subagent premise — corrected for Pi.** The "durable/retained
  subagents are a baseline harness capability" premise holds on Claude Code
  (in-memory `SendMessage` resume) but **not on Pi** (no in-memory
  background resume). Pi achieves durability *file-based* (session `.jsonl`
  persistence + `--session` resume). The premise is met in spirit, not in
  mechanism. The "subquery absorbed into harness-native Explore" premise
  also **does not hold on Pi** (Pi has no Explore in core); ws must build
  Explore itself (depth-2 leaf, see above). Both corrections are
  Pi-specific and do not retroactively change Claude/Codex behavior.
- **wsflow.** Unaffected by this research. wsflow is the agentless
  derivative; the Pi bridge is a separate harness adapter. The
  wsflow-mirroring maintenance discipline is orthogonal.
- **opencode tickets.** `260801-feat-ws-opencode-adapter` is dropped
  (moved to `.dropped/`); this research supersedes it as the active
  direction. An opencode adapter could later be re-opened as a fresh
  sibling under the same ws-mcp, but it is not a queued direction.
  `260801-todo-ws-mcp-log-append-cli` is also dropped — its sole consumer
  was the opencode adapter; the Pi bridge re-evaluates its logging needs in
  the expansion phase.

## Open questions

### Resolved this session

- **MCP bridge sub-path → (a-i) stdio.** Decided: the TS extension embeds
  a lightweight JSON-RPC stdio client, spawns ws-mcp, proxies each MCP tool
  through `pi.registerTool`. No relay. (was Q1)
- **Distribution → git package.** Pi supports `pi install
  git:host/user/repo@<ref>` with ref pinning and clone-time `npm install`
  (verified `packages.md` 2026-08-02). The ws Pi layer ships as a git
  package, matching the existing Claude distribution pattern for
  consistency. npm remains a fallback if git distribution proves
  unsuitable, but git is the default. (was Q3)
- **Goal-loop marker protocol — discard opencode design.** The opencode
  `$goal-response:<token>` four-token protocol carried opencode-structure
  hacks; it is not ported. The Pi goal-loop signal is redesigned from
  scratch in the expansion phase (Pi's `agent_settled` is a stronger
  primitive than opencode's `session.idle`). (was Q5)
- **Always-visible TODO surface — implementation TBD (expansion).** The
  tool surface exists; the exact shape (`ui.setWidget` vs `ui.custom`) is
  decided at implementation time in the expansion phase. Not blocking MVP.
  (was Q6)
- **Dashboard — closed topic, not absorbed.** The 260514 dashboard epic is
  closed; this research does not absorb its TUI work. Any ws-visible
  board/TODO surface is designed fresh on Pi. (was Q7)

### Resolved by runtime spike (2026-08-02, pi 0.83.0)

Spike extension at `/tmp/pi-spike/spike.ts` registered four tools
(`ws/playbook.print`, `ws:tickets.create`, `ws/agent.spawn`, `normal_echo`)
and ran `pi --mode json -p` against a live model (`z-ai/glm-5.2` via
openrouter). Session spikes ran two sequential `--session <path>` turns.

- **Pi tool-name charset — slash/colon/dot accepted end-to-end (was Q2).**
  `pi.registerTool` performs **no name validation** — the name is stored
  as a Map key and serialized into the provider payload unchanged
  (verified in `dist/core/extensions/loader.js` and
  `dist/core/agent-session.js` `_refreshToolRegistry`). The live model
  emitted a `tool_use` with `name: "ws/playbook.print"`, pi dispatched it
  to the registered handler by exact-name match, and the handler's output
  (`echo:ws/playbook.print:hello-slash`) was returned to the model, which
  then replied "DONE." Full round-trip confirmed. **Consequence: the
  entire opencode prose-rewriting apparatus (colon→hyphen, allowlist
  regex, `tool.execute.after`, `chat.system.transform`) is deleted on Pi.
  ws `SKILL.md` prose with `ws/playbook.print`-style tool calls is
  load-and-go.** This also resolves "skill prose references to MCP tool
  calls" (was Q4) — prose works unmodified.
- **`--session` + `-p` resume appends turns (was Q8).** Two sequential
  `pi --mode json -p --session <path>` calls against the same file
  accumulated turns: after turn 1 the file had 5 lines (session header +
  model_change + thinking_level_change + user + assistant), after turn 2
  it had 7 (turn 2's user+assistant appended at lines 6-7). Resume does
  **not** start fresh. The session header is written once (id preserved,
  not re-written on resume). A semantic recall test confirmed the resumed
  model sees the full prior conversation (turn 1 planted "my favorite
  color is blue"; turn 2, resumed on the same file, correctly answered
  "blue"). **Consequence: file-based continuation is sound.**
- **Session file flush on process exit is complete (was Q9).** After turn
  1's process exited, the file contained turn 1's messages in full. Turn
  2's process opened the same `--session` path, read the existing content,
  and appended without corruption. **Consequence: async spawn/continue/wait
  is safe — after `wait` harvests a finished subprocess, the lead can
  immediately `continue` on that agent; the session file is flushed by
  process exit.**
- **Bonus: `--append-system-prompt` is not stored in the session file
  (validates continuation design).** The session file contains only
  `session`/`model_change`/`thinking_level_change`/`message` entries —
  **no system messages at all.** The system prompt is reconstructed at
  runtime from `--append-system-prompt <path>` on every resume. Reusing
  the same path on every `continue` yields an identical, non-duplicated
  system prompt. This directly validates the continuation design's claim
  that "system prompt duplication is prevented by Pi's session model, not
  by ws" — confirmed by inspection of the session file across two resumed
  turns with the same `--append-system-prompt` path.

### Open

1. **`--append-system-prompt` vs `--system-prompt` for subagent prompt
   injection.** MVP uses append (example's verified path, spike-confirmed
   non-duplicating). Switching to replace (leaner prompt, keeps
   skills/context files per `usage.md:240`) is a later tuning decision
   after MVP validates that the subagent behavior is correct. Open but
   low-risk. **Only remaining open question.**

## Monitoring items

- **Pi extension API stability.** Single documented API, jiti, hot-reload,
  `examples/` shipped. Lower risk than opencode v2. Watch for breaking
  event-shape changes on Pi upgrades.
- **`@modelcontextprotocol/sdk` TypeScript client.** Required for the
  (a-i) stdio bridge. Confirm it is npm-installable in a Pi extension
  `package.json` and works under jiti.
- **ws-mcp transport surface.** If (a-ii) relay becomes attractive, watch
  `260513-research-streamable-http-mcp-transport` for ws-mcp HTTP transport
  readiness.
- **Pi `--session` + `-p` interaction.** Two runtime spikes (Q8, Q9) must
  confirm file-based continuation works before the async spawn/continue/wait
  design is considered implementable.

## Status

Research / discussion. No code work. The opencode implementation ticket
(`260801-feat-ws-opencode-adapter`, dropped); this research is the active
direction but has not yet promoted to `ready/`.
MVP scope is agreed; the runtime spikes (Q8, Q9) and the tool-name spike
(Q2) are the next concrete investigations, all read-only / no ws-mcp
source change.