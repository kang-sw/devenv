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
spec: pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 39d5c8df0da96321
sage-review-completeness-reviewed: 39d5c8df0da96321
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
  file, **keyed on ws's canonical first-class tiers (`small`/`medium`/`large`/
  `xlarge`)** so `playbook.render`'s `recommended-tier` passes through unchanged
  (the demoted `light`/`core`/`deep` names are concrete-model aliases, not map
  keys). An unset config re-warns on every `workflow_manual` (matching the
  existing bootstrap-version-behind advisory). Unset degrades gracefully to
  inherit; never hard-fail.

## Constraints

- ws-mcp Go source is never modified for Pi (golden rule). All Pi-specific
  policy (tool/skill/model curation, depth, warnings) lives in the adapter.
- AI-authored content is English; human-facing UI strings are exempt.
- The adapter validates the ws-mcp `runtime.json` plugin version on startup and
  fails loudly on mismatch (pin-and-fail, inherited from the opencode design).
- **Package home: a new `agents-plugin-pi/` sibling root** (parallel to
  `agents-plugin/`, `agents-plugin-tool/`, `agents-plugin-wsflow/`), holding the
  Pi extension source (TS/jiti `package.json`) and the adapter curation data
  file. This ticket is the authorizing ticket for that new root directory per
  AGENTS.md ("no new root module directories without a ticket").
- **Every proxied `ws/*` call is keyed; the key stays caller-controllable.**
  ws-mcp requires a per-call `session_key` (minted via `ferrule`). The bridge
  default-fills its own key only when omitted and
  forwards an explicit key verbatim, so both subagent child-key lineage and lead
  multi-track orchestration retain explicit control (see Phase 1).

## Spec Impact

No existing spec stem covers Pi-harness behavior; this MVP introduces a new
adapter-runtime surface. Target spec area: **Pi adapter runtime contract** (a
new area sibling to `plugin-runtime` / `mcp-tools`; the harness-neutral ws-mcp
contracts stay unchanged per the golden rule). Expected caller-visible changes:

- ws-mcp tools are reachable on Pi under provider-legal sanitized names
  (`ws__<tool>`; `/`→`__`, `.`→`_`) via the bridge; dispatch to ws-mcp uses the
  raw dotted name, and skill/playbook prose stays unchanged because the model
  maps `ws/<tool>` prose onto the sanitized tool (spec `pi-adapter-runtime`,
  `{#260903-pi-bridge-tool-registration}`). The literal-`ws/...` form the plan
  assumed is rejected by OpenAI-compatible providers — see Phase 1 Result.
- New Pi-side delegation tools: `ws-agent-spawn` / `ws-agent-continue` /
  `ws-agent-wait` (async, depth 0→1 leaf) and `explore` (one-shot recon leaf),
  with the per-spawn `--model`/`--tools`/`--append-system-prompt`/`--session`
  curation contract.
- `session_key` stays an optional, caller-controllable parameter on keyed
  `ws/*` tools (bridge default-fills when omitted, forwards when supplied);
  `ferrule` remains exposed for lead multi-track and child-key minting.
- New Pi model-catalog config: a user-curated catalog + tier map in the adapter
  data file; an unset config emits a `workflow_manual` advisory and falls back
  to inherit (never hard-fail).
- One PoC command surface (`/ws-discuss`) via `pi.registerCommand`.

ws-mcp core tool contracts (`mcp-tools`, `plugin-runtime`) are unchanged; all
new behavior lives in the Pi adapter.

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
surface empty.

Session-key wiring: ws-mcp requires a `session_key` on every keyed call, but on
the registered Pi tools `session_key` stays an **optional, caller-controllable**
parameter — it is not stripped. The bridge **default-fills** its own startup
auto-login key only when a call omits `session_key`, and **forwards an explicit
`session_key` verbatim**. This serves both scopes:

- **Subagents** use the child key the lead minted for them (spliced into the
  rendered playbook via `--append-system-prompt`, see Phase 2) and pass it
  explicitly → the bridge forwards it, preserving ws-mcp's parent→child session
  lineage. A blanket auto-login per subagent is wrong — it would mint an
  unrelated key and break lineage.
- **The lead** often orchestrates multiple implementation tracks under distinct
  keys; it mints per-track/child keys with `ferrule` (binds a worktree `root`,
  sets `capability` lead/delegate/leaf, records `parent_session_key` lineage)
  and targets each track by passing its `session_key` explicitly. Hiding the key
  at lead scope would break multi-track orchestration.

`ferrule` and the other key-management tools stay exposed. `unknown_session`
re-login applies only to the bridge's own default-fill key; an explicit
caller-supplied key that is unknown surfaces to the caller. (Rejected: stripping
`session_key` and transparently injecting one bridge-held key — it breaks both
subagent lineage and lead multi-track orchestration.)

Gate: a keyed ws-mcp tool (e.g. `ws/workflow_manual`) round-trips end-to-end
through the bridge against a live model — with `session_key` default-filled when
omitted and forwarded verbatim when supplied.

### Result (0d47b71f) - 2026-09-03

Phase 1 bridge landed in the new `agents-plugin-pi/` root. Commits
`5930d48f..0d47b71f` (scaffold `3772819f`, naming fix `9aea2744`, review-round-1
fixes `0d47b71f`). Caller-visible behavior is documented in spec
`pi-adapter-runtime` (`ai-docs/spec/pi-adapter-runtime.md`). Gate met with
evidence.

Deviations from the plan text:
- **Tool naming reversed from "verbatim `ws/...`".** A literal `/` in the tool
  name is rejected by OpenAI-compatible providers (`^[a-zA-Z0-9_.-]+$`), which
  breaks the whole tool-bearing turn. The Q2 spike's success was
  provider-dependent (a slash-tolerant Anthropic-format backend). Pi's open
  model space forbids assuming one provider's leniency, so tools register under
  provider-legal sanitized names (`/`→`__`, `.`→`_`, e.g. `ws__playbook_print`);
  dispatch still uses the raw dotted name, skill prose is untouched, and the
  model maps prose→tool as the reference harnesses already do. User-approved;
  anchor `260802` Q2 annotated with the caveat.
- **Subprocess spawns in `session_start`, not at module load** — Pi forbids
  starting background processes from the top-level extension factory.
- **`session_key` required-list strip (discovered live).** Pi validates
  tool-call arguments against the registered schema *before* execute runs;
  ws-mcp marks `session_key` required on root-aware tools, so omitted-key calls
  failed Pi-side before fill-or-forward. Fixed by stripping `session_key` from
  each registered schema's `required[]` only (kept in `properties`), making the
  optional-key constraint true at the Pi tool layer.
- **`rsrc/` added to the copied assets** (plan step-1 named only launcher +
  runtime.json). The copied launcher resolves `WS_RSRC_ROOT` relative to its own
  package, so `agents-plugin-pi/rsrc/` must exist (byte-identical, wsflow
  precedent) or `playbook.render`/`workflow_manual` fail with "rsrc manifest
  missing". Now three hand-synced byte-identical copies (launcher, runtime.json,
  rsrc/) with no sync tooling — the same debt wsflow carries.

Verification: 31 unit tests pass (`node --test`; sanitize/strip/resolve,
JSON-RPC line-buffer incl. multibyte-split + out-of-order id correlation,
version pin). Live gate against openrouter (only ready provider): default-fill
round trip, prose-mapping (`ws/playbook.print` prose → `ws__playbook_print`
call), explicit `session_key` forwarding, pin-and-fail, and `resources_discover`
— all confirmed. The UTF-8 multi-chunk fix was validated via a documented
byte-fed proxy (real responses stayed under the 64KB pipe buffer). Golden rule
verified: zero changes to `agents-plugin/`, `agents-plugin-tool/`,
`agents-plugin-wsflow/`.

Deferred: automated multi-provider tool-name-legality check; a dedicated
Pi-adapter mental-model domain (better authored once Phases 2-3 fill the
surface); the `--append-system-prompt` vs `--system-prompt` tuning (anchor
`260802` Open item). Phases 2-4 remain; ticket stays in `ready/`.

### Phase 2: Self-built subagent spawner + explore primitive

Depends on Phase 1 (the bridge and ws-mcp tools must be reachable). Implement
the async spawner as a Pi extension surface:

- `ws-agent-spawn({ playbook, task, tier? })` / `ws-agent-continue({ agentId,
  task })` / `ws-agent-wait({ agent-ids, policy: "any"|"all", timeout? })` — a module-state
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
- Per-spawn curation resolves `--tools` from a **tool-group table created in
  this phase** (read-only / recon / full-worker → Pi tool-name allowlists) and
  `--model` from the tier map. The tier→model map is a Phase 3 deliverable, so
  Phase 2 lands the tool-group table and resolves `--model` as inherit until
  Phase 3 populates the map. MVP depth is 0→1 leaf (no nested-spawn tool
  exposed to depth-1 workers).

Gate: lead spawns a worker and an `explore` leaf, `wait` harvests both, and a
`continue` on the worker resumes its session.

### Result (7fcce4e3) - 2026-09-03

Phase 2 landed the self-built delegation spawner in `agents-plugin-pi/`. Commits
`13b4e67f` (spawner + four tools + unit tests) and `7fcce4e3` (review-relay
fixes). New caller-visible surface documented in spec `pi-adapter-runtime`
(`{#260903-pi-delegation-spawner-tools}`, `{#260903-pi-spawner-completion-gating}`,
`{#260903-pi-explore-recon-leaf}`, `{#260903-pi-spawner-tool-groups}`,
`{#260903-pi-spawner-model-tier-inherit}`; landed separately in `fc99fea2`).

New: `src/spawner.ts` — module-state `AgentRegistry`, async `spawnAgent` /
`continueAgent` / `waitAgents` / `exploreLeaf`, `registerAgentTools`, and the pure
helpers `TOOL_GROUPS` / `resolveTools` / `isTerminalStopReason` / `buildSpawnArgs`
/ `AgentEventLineBuffer` / `handleAgentEvent`. Tool registration + a
still-running-worker kill-pass wired into `src/index.ts`; `src/bridge.ts`
`BridgeHandle` extended with `client` / `defaultSessionKeyRef` / `wsToolNames` so
the spawner reuses the single ws-mcp connection instead of opening a second one.

Deviations from the plan text:
- **Completion gates on the child `close` event, stronger than the planned
  `exit`.** `close` fires after stdio is fully drained (`lineBuffer.end()` runs
  first), so the flush guarantee the Q9 risk demands holds a fortiori; the
  last-seen terminal `stopReason` (`stop`/`length`/`error`/`aborted`) is kept only
  as metadata and never flips state. This is the load-bearing correctness point.
- **Spec section is a lead-owned doc step, not part of the implementer commit** —
  fit review flagged its absence from `13b4e67f`; it landed in `fc99fea2`.
- **Bun-virtual-script branch of the shipped `examples/extensions/subagent`
  reference was dropped** — this package runs only under Node, so it would be dead
  code.

Verification (partial — live gate pending): 59 unit tests pass (`node --test`):
tool-group resolution, all five `stopReason` classifications, spawn/continue/
explore arg-building (incl. `--session`/`--no-session` mutual exclusion and
`--model`-omitted-when-unset), the multibyte-safe NDJSON line buffer, and the
`handleAgentEvent` state-gating regression added in relay (terminal `stopReason`
must not flip `state`). Partitioned review: correctness clean (all load-bearing
gating verified) + 3 robustness minors `[fixed]`; fit one Important (spec gap,
closed in `fc99fea2`) + one minor `[fixed]`; test one Important
(`handleAgentEvent` regression coverage, `[fixed]` in `7fcce4e3`). No Critical, no
elevation. Golden rule verified: zero changes to `agents-plugin/`, `-tool/`,
`-wsflow/`, or the hand-synced `agents-plugin-pi/{rsrc,runtime.json,bin}` copies.

**The live end-to-end gate (spawn a worker + `explore` leaf → `wait` harvests
both → `continue` resumes the worker's session, against a live provider) has NOT
yet been run** — the async engine and Pi tool wiring are exercised only by that
gate, matching Phase 1's convention of unit-testing IO-free seams only. This is
the sole outstanding acceptance step for Phase 2.

Deferred: `AGENTS.md` `## Project Orientation` still omits the `agents-plugin-pi/`
root (kept out until the ticket integrates off this tracking branch, consistent
with Phase 1); `tier` resolves as inherit until Phase 3 lands the model catalog.
Phases 3-4 remain; ticket stays in `ready/`.

### Phase 3: Model catalog curation + tier map + bootstrap warning

Depends on Phase 2 (spawner consumes the tier map). Implement the Pi model
config:

- Read Pi's enabled/configured model pool at runtime (**verify the
  extension-facing read API**; `enabled-models.ts`). Treat the raw pool as
  curation input — it can be thousands of entries via aggregators (openrouter),
  so setup narrows it to a small workable catalog.
- Store the curated catalog + tier map in the adapter's curation data file
  (adapter-owned; no Pi model strings in ws-mcp core). The map is **keyed on
  ws's canonical first-class tiers `small`/`medium`/`large`/`xlarge`** so
  `playbook.render`'s `recommended-tier` resolves directly; `explore` is a role
  (not a tier) defaulting to the `small` tier for cheap recon. This phase adds
  the tier→model catalog atop the tool-group table Phase 2 landed.
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
bridge, and drives one spawn round-trip. Gate: `/ws-discuss` loads the ws skill,
calls a ws-mcp tool through the bridge, and completes one spawn round-trip
end-to-end on Pi — proving skills-load + bridge + spawner compose. This
validates the MVP before feature expansion (durable depth-2 recursion,
always-visible TODO, goal-loop, compaction hooks — all deferred to follow-up
tickets under the epic).
