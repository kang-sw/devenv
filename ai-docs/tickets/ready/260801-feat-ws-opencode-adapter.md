---
title: "ws opencode adapter — thin boundary-layer shim plugin"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260731-research-ws-opencode-drop-in-package: completed research ticket with all design decisions
  260801-todo-ws-mcp-log-append-cli: sub-ticket for ws-mcp log append CLI needed by Phase 2 goal-loop guard logging
  260605-research-ws-native-subagent-pivot: host-neutral pivot direction
  260611-research-ws-per-role-delegation-tuning-config: model tier mapping research
sage-review-design: required
---

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

8. **Tier swap** (v2 `setup`): `ctx.catalog.model.default.set(...)` at spawn
   time → spawn → restore. Mapping table derived from `opencode models` CLI,
   filtered to credentialed providers.

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

### Phase 3: Tier swap

- Implement `config.agents_tier` MCP integration (read tier config).
- Build model mapping table from `opencode models` CLI output, filtered
  to credentialed providers.
- Implement `ctx.catalog.model.default.set(...)` → spawn → restore at
  spawn time.
- Test tier swap with a concrete provider/model.

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
