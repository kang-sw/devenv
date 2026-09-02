---
title: "Implement the ws Pi-native MVP: self-built MCP bridge + subagent spawner + model-catalog config"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: design anchor — full rationale, spikes, and settled decisions this ticket implements
  260611-research-ws-per-role-delegation-tuning-config: tier/model mapping research the Pi model-catalog config instantiates
related-mental-model:
  - plugin-runtime
  - named-agent-runtime
  - mcp-runtime
sage-review-design: required
---

# Implement the ws Pi-native MVP: self-built MCP bridge + subagent spawner + model-catalog config

## Background

`260802-research-ws-pi-native-framework` converged on a Pi-native framework
that bridges the harness-neutral ws-mcp server onto Pi (earendil-works) as an
opinionated workflow layer, with ws-mcp Go source untouched (golden rule:
one-directional dependency, adapter → ws-mcp). This ticket implements the MVP
surface. Full rationale, alternatives, and runtime spikes live in the anchor;
this body carries only the implementation-shaping decisions and the phase plan.

The MVP rests on primitives already spike-verified in the anchor (pi 0.83.0–
0.84.4): `pi.registerTool` dispatches slash/colon/dot tool names verbatim (Q2);
`pi --mode json -p --session <path>` resume appends turns and flushes on exit
(Q8/Q9); `--append-system-prompt` content is reconstructed per resume, never
duplicated into the session file. Remaining verification is folded into the
phases that depend on it.

## Decisions

Settled in `260802` (see anchor for rejected alternatives and evidence):

- **Both bridge and subagent layer are self-build; no external subagent/MCP
  dependency.** `@tintinweb/pi-subagents` was evaluated and rejected as a
  dependency (its per-role curation requires writing agent-profile files into
  the user's global `~/.pi/agent/agents/` or project tree — no package-internal
  injection); it is the recorded fallback only. No published MCP adapter
  preserves `ws/...` tool names verbatim, so the stdio bridge is self-built.
- **Zero ws agent-profile files.** Per-spawn curation is delivered entirely
  through `pi` CLI flags — `--model` (tier), `--tools` (per-spawn allowlist),
  `--append-system-prompt` (dynamic rendered playbook from a ws-owned temp
  file), `--session` (continuation). No `.pi/agents/` or global profile writes.
- **Two first-class delegation kinds, split by lifecycle.** `explore` =
  ephemeral one-shot read-only recon (`--no-session`, no continue, self-reaping
  leaf); `ws-agent-spawn/continue/wait` = durable multi-turn worker
  orchestration. Recon is a dedicated narrow primitive, not the general spawner
  exposed at depth. MVP depth is 0→1 leaf; durable depth-2 recursion is out of
  scope (expansion).
- **Model tiers are user-configured on Pi.** Pi accepts any model over an open
  per-user catalog, so ws ships no tier→model default; the only safe default is
  inherit-parent. A curated catalog + tier map lives in the Pi adapter's data
  file, and an unset config re-warns on every `workflow_manual` (matching the
  existing bootstrap-version-behind advisory). Unset degrades gracefully to
  inherit; never hard-fail.

## Constraints

- ws-mcp Go source is never modified for Pi (golden rule). All Pi-specific
  policy (tool/skill/model curation, depth, warnings) lives in the adapter.
- AI-authored content is English; human-facing UI strings are exempt.
- The adapter validates the ws-mcp `runtime.json` plugin version on startup and
  fails loudly on mismatch (pin-and-fail, inherited from the opencode design).

## Phases

### Phase 1: Self-built MCP stdio bridge extension

Build the Pi extension that spawns the ws-mcp launcher as a subprocess, speaks
a minimal JSON-RPC-over-stdio MCP client, and re-registers every ws-mcp tool
via `pi.registerTool` under its **verbatim** `ws/...` name (Q2 confirmed name
preservation; re-confirm live from inside the extension). The extension owns
the subprocess lifecycle (spawn on load, close on `session_shutdown`). Expose
`agents-plugin/skills/` through `resources_discover` so hyphen-form skills load
with zero prose rewriting. Verify a minimal self-owned stdio client (or
`@modelcontextprotocol/sdk`) installs and runs under jiti in a Pi extension
`package.json`; prefer the minimal self-owned client to keep the dependency
surface empty. Gate: a ws-mcp tool (e.g. `ws/workflow_manual`) round-trips
end-to-end through the bridge against a live model.

### Phase 2: Self-built subagent spawner + explore primitive

Depends on Phase 1 (the bridge and ws-mcp tools must be reachable). Implement
the async spawner as a Pi extension surface:

- `ws-agent-spawn({ playbook, task, tier? })` / `ws-agent-continue({ agentId,
  task })` / `ws-agent-wait({ agent-ids, policy, timeout? })` — a module-state
  registry (`Map<agentId, {proc, sessionPath, systemPromptPath, state,
  output, drainer}>`), background stdout draining (pipe-deadlock-safe) that
  parses `pi --mode json` events and flips `state:"done"` on terminal
  `stopReason`, `session_shutdown` cleanup, and file-based continuation via
  `--session <ws-owned-path>` (sibling of `~/.pi/agent/sessions/`, hidden from
  the `/resume` picker). `continue` allowed only when `state:"done"`; `wait`
  timeout partitions (done/running/timedOut) without killing.
- `explore({ query, async? })` — a thin one-shot preset over the same engine:
  fixed `playbook=explore`, `--tools=recon`, `--no-session`, self-reaping leaf,
  no continue.
- Playbook injection: render `ws/playbook.render(name)` once at spawn into a
  ws-owned temp path and pass it as `--append-system-prompt`; reuse the same
  path on `continue` (Pi reconstructs, does not duplicate).
- Per-spawn curation resolves `--model`/`--tools` from the curation data file;
  MVP depth is 0→1 leaf (no nested-spawn tool exposed to depth-1 workers).

Gate: lead spawns a worker and an `explore` leaf, `wait` harvests both, and a
`continue` on the worker resumes its session.

### Phase 3: Model catalog curation + tier map + bootstrap warning

Depends on Phase 2 (spawner consumes the tier map). Implement the Pi model
config:

- Read Pi's enabled/configured model pool at runtime (**verify the
  extension-facing read API**; `enabled-models.ts`). Treat the raw pool as
  curation input — it can be thousands of entries via aggregators (openrouter),
  so setup narrows it to a small workable catalog.
- Store the curated catalog + tier map (explore/recon, light/medium/large) in
  the adapter's curation data file (adapter-owned; no Pi model strings in
  ws-mcp core).
- When the tier map (or at least the explore tier) is unset, append a strong
  advisory to every `workflow_manual` bridge response (adapter-side
  post-processing, keyed on unset state), mirroring the bootstrap-version-behind
  cadence. Unset spawns/explores fall back to inherit silently; never block
  work.

Gate: with an unset map, `workflow_manual` shows the advisory and a spawn
inherits; after configuring a tier, a spawn uses the mapped model.

### Phase 4: Proof-of-concept command

Depends on Phases 1–3. Register one PoC command (e.g. `/ws-discuss`) via
`pi.registerCommand` that loads the ws skill, calls a ws-mcp tool through the
bridge, and drives one spawn round-trip — proving skills-load + bridge +
spawner compose end-to-end on Pi. This validates the MVP before feature
expansion (durable depth-2 recursion, always-visible TODO, goal-loop,
compaction hooks — all deferred to follow-up tickets under the epic).
