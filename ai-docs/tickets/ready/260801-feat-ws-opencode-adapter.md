---
title: "ws opencode adapter — thin boundary-layer shim plugin"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260731-research-ws-opencode-drop-in-package: completed research ticket with all design decisions
  260801-todo-ws-mcp-log-append-cli: sub-ticket for ws-mcp log append CLI needed by Phase 2 goal-loop guard logging
  260605-research-ws-native-subagent-pivot: host-neutral pivot direction
  260611-research-ws-per-role-delegation-tuning-config: model tier mapping research
sage-review-design: required
status: blocked-on-harness-decision
---

> **Status note (2026-08-02):** Deprioritized in favor of
> `260802-research-ws-pi-native-framework`, which corrected this ticket's
> "Harness tradeoff note" (Pi *does* have a first-class compaction surface
> and re-entry primitives in core) and chose Pi as the target harness via
> the bridge path. This ticket is **not dropped**: the bridge path keeps
> ws-mcp harness-neutral, so an opencode adapter remains a possible later
> sibling under the same ws-mcp. See the new research ticket for the
> corrected capability matrix and the active direction.
>
> **Prior status (2026-08-01):** Implementation deferred pending harness
> tradeoff evaluation. The accumulated workaround cost for opencode (v1/v2
> split, config hook timing hack, unbridged agent services, unstable v2
> surface) raised the question of whether Pi (earendil-works) might be a
> simpler target despite needing goal-loop/compaction built from scratch.
> See "Harness tradeoff note" at the end of this ticket.

# ws opencode adapter — thin boundary-layer shim plugin

## Background

The research ticket `260731-research-ws-opencode-drop-in-package` resolved all
nine design questions and converged on architecture **C** (thin boundary-layer
shim). The adapter uses a single default export `{id, server, setup}` that
satisfies both opencode plugin loaders simultaneously:

- **v1 `server`**: `input.client.mcp.add(...)` for ws MCP registration (v1-only),
  `Hooks` with `experimental.*` for compaction/goal-loop, `input.client.session.*`
  for SDK calls.
- **v2 `setup`**: `PluginContext` domain mutation — `agent.update`,
  `command.update`, `catalog.model.default.set`, `reference.add` (v2-only).

All skill prose rewriting is adapter-side (non-invasive grand principle: the
adapter does not modify ws-mcp, playbook, or rsrc source files). Skill names
are rewritten from colon-form (`ws:lead-discuss`) to hyphen-form
(`ws-lead-discuss`) at registration time, using an allowlist-based regex
derived from both `skills/manifest.json` and `rsrc/manifest.json`.

## Decisions

1. **Entry point**: single default export `{id: "ws", server, setup}`. Do not
   include an `effect` field (v2 Schema Union picks `effect` first and would
   misinterpret the module as an Effect plugin).

2. **MCP registration** (v1 `server`): call
   `input.client.mcp.add({ name: "ws-mcp", config: { type: "local", command:
   [...], environment: {...} } })` at setup. Inject env from `ctx.options`
   (plugin config tuple). No static `opencode.json` `mcp` entry needed.

3. **Skill registration** (v2 `setup`): read `SKILL.md` files from
   `agents-plugin/skills/`, rewrite prose (allowlist-based regex:
   `ws:lead-...` → `ws-lead-...`, bare `lead-...` → `ws-lead-...`, frontmatter
   `name:` → `ws-lead-...`), register as embedded skills via
   `ctx.skill.transform` + `draft.source(EmbeddedSource)`.

4. **Skill listing safety net** (v1 `Hooks`): register
   `experimental.chat.system.transform` to rewrite available-skills listing
   in system prompt (colon → hyphen).

5. **Playbook output rewriting** (v1 `Hooks`): register `tool.execute.after`
   to post-process `playbook.print`/`playbook.render` MCP tool results,
   rewriting `ws:lead-...` → `ws-lead-...` in rendered output (rsrc templates
   hardcode `:` as separator).

6. **Goal-loop** (v1 `server` + SDK): judgment turns via
   `input.client.session.prompt({ delivery: "queue" })`.
   `$goal-response:<token>` marker protocol with tolerant regex parser.
   Tokens: `achieved | next-step | keep-working | pause`. Loop guard: 10min
   sliding window, 5-re-injection threshold, `next-step` fallback.

7. **Compaction** (v1 `Hooks`): `experimental.session.compacting` injects ws
   state into compaction prompt. `experimental.compaction.autocontinue`
   returns `enabled: false` for adapter-triggered compaction (`overflow:
   false`), `enabled: true` for opencode automatic compaction (`overflow:
   true`).

 8. **Tier-agent registration — path revised (2026-08-01 spike).** Original
    design used v2 `ctx.catalog.model.default.set(...)` at spawn time. Spike
    investigation revealed:
    - The `task` tool is **v1-only** and reads v1 `Agent.Service`, which is
      populated from hardcoded built-ins + `opencode.json` `agent.*` config.
      v2 `AgentV2.Service` (populated by `ctx.agent.transform`) is **unbridged**
      — `task` does not see v2-registered agents.
    - The v2 stack has **no `task` tool** yet (`builtins.ts` TODO: "task not
      yet ported").
    - `client.config.update` (HTTP) **persists to disk** (`config.json` write +
      instance disposal) — unsuitable for ephemeral plugin-owned injection.
    - **Working path: v1 `config` hook** (`Hooks.config?: (input: Config) =>
      Promise<void>`) mutates the live cached `Config` object in-place during
      `Plugin.state` init. Never touches disk. `Agent.state` reads `cfg.agent`
      to build the registry, so tier agents injected via the `config` hook are
      visible to `task` — **provided `Plugin.state` init runs before
      `Agent.state` init** (normal dependency order, but timing-dependent and
      requires runtime verification).
    - Agent definitions carry a `model` field; `task` resolves
      `next.model ?? parent.model`, so per-agent `model` overrides work.
    - Tool restriction is via `permission` (deny-all + allow-list), NOT the
      `tools` field (the cfg-merge loop ignores `tools`).
    - `subagent_depth` (default 1) must be raised if tier agents should
      delegate further.

    **Tier agent shape** (injected via `config` hook):
    ```jsonc
    {
      "light-agent":  { "model": "...", "mode": "subagent", "permission": {...}, "description": "..." },
      "medium-agent": { ... },
      "large-agent":  { ... },
      "xlarge-agent": { ... }
    }
    ```
    Tier → model mapping derived from `config.agents_tier` MCP + `opencode
    models` CLI, filtered to credentialed providers.

9. **Non-invasive grand principle**: all rewriting is adapter-side. ws-mcp,
   playbook, and rsrc source files are never modified. Dependency is
   one-directional (opencode → ws-mcp).

10. **Spec updates**: update `ai-docs/spec/plugin-runtime.md` and create/update
    `ai-docs/mental-model/plugin-topology` to reflect opencode as an explicit
    — and asymmetric — harness target.

## Phases

### Phase 1: Minimal PoC — MCP registration + skill discovery + single command

- Create `agents-plugin-opencode/` directory (sibling to `agents-plugin/`
  and `agents-plugin-wsflow/`).
- Write `agents-plugin-opencode/plugin.ts` with default export
  `{id: "ws", server, setup}`:
  - `server`: register ws MCP launcher via `input.client.mcp.add(...)`,
    return minimal `Hooks` (empty for now).
  - `setup`: read `agents-plugin/skills/` directory, rewrite prose
    (allowlist-based regex from both manifests), register embedded skills
    via `ctx.skill.transform` + `draft.source(EmbeddedSource)`.
- Register `experimental.chat.system.transform` in `Hooks` as skill-listing
  safety net.
- Register a single `/ws-discuss` slash command via `ctx.command.update`
  (v2) as proof-of-concept.
- Document the thin-adapter rules (6 rules) in a `README.md` or inline.
- Test: verify opencode loads the plugin, ws MCP connects, skills appear
  with `ws-` prefix, `/ws-discuss` command is available.

### Phase 2: Goal-loop + compaction surface

- Implement judgment-turn injection via
  `input.client.session.prompt({ delivery: "queue" })`.
- Implement `$goal-response:<token>` marker regex parser.
- Implement loop guard (10min/5-re-injection, `next-step` fallback).
- Register `experimental.session.compacting` hook (ws state injection).
- Register `experimental.compaction.autocontinue` hook (overflow
  discrimination).
- Register `tool.execute.after` hook for playbook output rewriting
  (colon → hyphen).
- Implement `session.idle` event observation for marker detection.
- **Dependency**: Phase 2 goal-loop guard logging requires
  `260801-todo-ws-mcp-log-append-cli` to be landed. Until then, fall back
  to `console.warn`.

### Phase 3: Tier-agent registration via `config` hook

- Implement v1 `Hooks.config` handler that injects `light-agent`,
  `medium-agent`, `large-agent`, `xlarge-agent` into `input.agent` in-place.
- Read `config.agents_tier` MCP tool to get tier → model mapping.
- Build model mapping table from `opencode models` CLI output, filtered to
  credentialed providers.
- Set `subagent_depth` in config if tier agents should delegate further.
- **Runtime verification required**: confirm `Plugin.state` init runs before
  `Agent.state` init in the actual bootstrap sequence. If not, investigate
  forcing plugin init early.
- Test: `task({subagent_type: "light-agent"})` resolves and uses the
  per-agent model.

### Phase 4: TuiPlugin reservation (later/optional)

- Reserve a TUI slot (e.g. ticket-board sidebar) via `TuiPlugin` API.
- Out of scope for initial implementation; deferred to a later phase after
  PoC validation.

### Phase 5: Spec + mental-model updates

- Update `ai-docs/spec/plugin-runtime.md` to document the opencode plugin
  surface (v1+v2 coexistence, MCP registration, skill rewriting, goal-loop).
- Create or update `ai-docs/mental-model/plugin-topology` to reflect
  opencode as an explicit — and asymmetric — harness target.
- Update `ai-docs/_index.md` plugin topology section.

### Result

_(to be filled after implementation)_

## Harness tradeoff note (2026-08-01)

During tier-agent design, the accumulated adapter complexity for opencode
raised a strategic question: is the workaround cost justified, or would a
simpler harness be a better target? A brief comparison with **Pi**
(earendil-works, MIT, terminal coding-agent harness) was recorded:

| Surface | opencode | Pi |
|---------|----------|----|
| Subagent/tier | v1 `config` hook timing hack; `task` v1-only; v1/v2 unbridged; #39937 open | **markdown frontmatter `model:` field — done** |
| MCP connection | v1 `input.client.mcp.add` (works) | extension subprocess spawn (`pi-sub-agent` precedent) |
| Goal-loop | `session.prompt({delivery:"queue"})` + marker (surface exists but unstable) | RPC/SDK mode — build from scratch on clean API |
| Compaction | `experimental.session.compacting` + `autocontinue` (surface exists, hack-dependent) | **No compaction concept in Pi core** — build needed (or unnecessary if Pi's small-context philosophy holds) |
| Skill/playbook | skill loader + prose rewriting pipeline (complex) | No skill concept — agent def + tool reconceptualization needed |
| API stability | v2 unreleased/undocumented, `effect@beta`, unported `task` tool | **Single extension API, hot-reload, jiti, documented** |
| Investigation invested | 9 questions fully resolved, source-verified | ~0 |

**Key insight**: ws-mcp is harness-independent and reusable on either. The
adapter work differs:
- opencode: compaction/goal-loop/skill surfaces **exist but are unstable** and
  require multiple workarounds (v1/v2 split, prose rewriting, config hook
  timing, unbridged agent services). Architecture debt with no resolution
  timeline (#39937, `effect@beta`, v2 `task` TODO).
- Pi: compaction/goal-loop must be **built from scratch**, but on a **clean,
  stable, single-API extension surface**. Tier/subagent is trivially simple.

**Status**: tradeoff requires deeper evaluation before committing to opencode
as the implementation target. Pi's extension API capabilities (MCP subprocess
bridging, RPC/SDK session control for goal-loop) need spike investigation to
make an informed comparison. This ticket remains in `ready/` but implementation
is deferred until the harness decision is finalized.

### Pi reference links
- Site: https://pi.dev/
- Package registry: https://pi.dev/packages
- `pi-sub-agent` package: https://pi.dev/packages/pi-sub-agent (subagent tool,
  bundled agents, markdown frontmatter agent definitions with per-agent `model`)
- `pi-subagents` package (newer, native bidirectional parent-child
  communication): https://pi.dev/packages/pi-sub-agents
- GitHub: https://github.com/earendil-works/pi
