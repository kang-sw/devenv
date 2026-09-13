---
title: Cross-session mailbox wake path & usage — CLI wait, harness hook adapters, lead-use-mailbox skill
related:
  260913-research-cross-session-mailbox: design-source — carries the settled wake decisions (6, 7, 9, wake half of 14) and the 2026-09-13 probe results
  260913-feat-cross-session-mailbox-core: prerequisite — the wake path drains and arms over the core's name-keyed queue, reply-id queue, and piggyback spine
  260909-epic-ws-worker-interpreter-refoundation: constraint — no blocking in a tool call, no polling; harness owns waiting (Cross-Child Decision 16)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3fd30e7661a29b94
sage-review-completeness-reviewed: 3fd30e7661a29b94
---

# Cross-session mailbox wake path & usage — CLI wait, harness hook adapters, lead-use-mailbox skill

## Background

The core ticket (`260913-feat-cross-session-mailbox-core`) delivers peer
discovery, universal send, explicit `recv`, and the unread piggyback badge —
enough for a session that is already taking turns to *notice* mail. It does not
wake a **truly idle** executor: the motivating scenario is a Claude discussion
session remote-controlling an otherwise-idle Codex `lead-run` executor by mail
("run ticket X"), which needs something that blocks until mail arrives and then
re-invokes the agent.

This ticket adds that wake path and the usage layer on top of the core:
a host-neutral blocking CLI, the Codex/Claude hook adapters that arm and
re-invoke it, and a `lead-use-mailbox` skill that walks the lead/user through
the end-to-end flow. The harness-hook behavior this rests on was **already
empirically verified** — see `## Probe results` in the research ticket
(Codex 0.154.0, 2026-09-13) — so this ticket is no longer probe-gated; it
carries the probe findings as constraints.

## Decisions

Carried in intent from the research ticket's Confirmed Decisions 6, 7, 9 and the
wake half of 14, plus the 2026-09-13 probe results.

- **Wake lives in a CLI, never an MCP tool (6):** blocking belongs to a CLI
  process `ws-mcp mailbox wait --timeout`, launched by the model as a harness
  background task whose exit re-invokes the agent — the one non-implicit model
  obligation. A blocking MCP tool would freeze the single multiplexed stdio
  channel shared by the lead and every native subagent and re-cross refoundation
  Decision 16. The wait is **level-triggered, not edge-triggered**: on startup it
  synchronously checks the durable queues (the owner's name-keyed inbox and the
  caller's own reply-id queue) and returns immediately if any unread exists,
  blocking only when empty. Because the store is durable, mail deposited in the
  arm/drain gap is read on startup — no lost wakeup. A listening marker records
  that a wait is armed.
- **Env-less best-effort wake (wake half of 14):** a session with no
  `WS_MAILBOX`/`WS_MAILBOX_AUTO` has no durable inbox and no hook-driven wake;
  its wake is best-effort — the piggyback badge on its own ws calls plus a
  background CLI `wait` registration arranged at piggyback time, waiting on its
  own reply-id queue. Durable, hook-backed wake requires a slug (env).
- **Harness adapters are best-effort, never the contract (7):** the Codex/Claude
  hook wiring and pi native push are adapter/fallback surfaces per
  `shipped-surface-boundary.md`. The host-neutral contract is the CLI + listening
  marker of Phase 1; each host's arm/re-invoke mechanism is adapter behavior.
- **Codex adapter (probe-verified, 0.154.0):** the turn-boundary wake is a `Stop`
  hook returning `{"decision":"block","reason":"<drain/act instruction>"}`, which
  re-invokes the model with the reason as an instruction; the re-entry `Stop`
  carries `stop_hook_active: true` as a loop guard. `PostToolUse` output **never
  reaches the Codex model** (verified: not `exit 2`+stderr, not `decision:block`,
  not `additionalContext`), so `PostToolUse` is **side-effect/marker only**;
  mid-turn awareness rides the core's piggyback spine, not hooks. Codex `Stop`
  carries **no agent classifier**, so owner-context gating must **bake owner
  identity into the hook command args**. Codex 0.154 gates hooks behind persisted
  **hook trust** (`--dangerously-bypass-hook-trust` bypasses for automation), so
  a real adapter must persist hook trust — a deployment step.
- **Claude adapter (probe-verified from docs):** a single `Stop` hook gates on
  `hook_event_name == "Stop"` && `agent_id` absent → root lead turn (subagent
  context carries `agent_id`; `SubagentStop` always does), so the arm-reminder
  fires only in the owner/root turn. `run_in_background` → task-notification
  re-invoke is the wake (observed live; a completion can surface mid-turn).
- **Hook-layer misfire guard (9):** the arm-reminder fires only in the
  owner/root-turn context — on Claude via the `Stop`-not-`SubagentStop`
  distinction above; on Codex, since there is no payload classifier, via owner
  identity baked into the hook command args. This is best-effort and is the
  hook-layer counterpart of the core's server-layer `caller == owner` gate, which
  remains the real misfire guard.

## Constraints

- `agents-plugin-tool/` (the `ws-mcp mailbox wait` CLI subcommand and any MCP
  surface it reads) → read `ai-docs/manuals/ws-mcp.md` before editing.
- Hook-adapter text and the skill are **shipped surfaces** → read
  `ai-docs/manuals/shipped-surface-boundary.md`; the host-neutral contract is the
  CLI + marker, and Codex/Claude/pi specifics are adapter behavior that must not
  leak into the host-neutral layer.
- Phase 4 edits skills under `agents-plugin/skills/` → read
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`
  (the skill must mirror into the agentless `agents-plugin-wsflow/` package).
- Codex behavior specifics come from `ai-docs/manuals/codex-integration.md`
  (re-probed 2026-09-13, 0.154.0); honor the hook-trust gating and the
  `PostToolUse`-output-never-reaches-model finding.
- Refoundation Cross-Child Decision 16: no blocking in a tool call, no polling —
  the wait blocks in the CLI process, and the level-triggered check is a durable
  read on startup, not a poll loop.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- `260504-feat-ws-mcp-hook-driven-interrupt` (retired) — the hook-driven delivery
  shape (Codex `Stop`/`PostToolUse`, a `WS_AGENT_OUTBOX`-style env, hook-triggered
  drain) to mirror; its ws-owns-the-subprocess premise is dead, the delivery
  mechanism is not.
- `260913-feat-cross-session-mailbox-core` — the name-keyed queue, the reply-id
  queue, the listening/presence store, and the piggyback spine that this wait
  drains and this adapter arms.
- `ai-docs/manuals/codex-integration.md` — the 2026-09-13 hook re-probe: `Stop`
  `decision:block` keep-alive, `PostToolUse` output not reaching the model,
  hook-trust gating, useful isolation flags.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/ (mailbox wait CLI subcommand and the MCP surface it reads), agents-plugin/.codex-plugin/plugin.json + agents-plugin/.claude-plugin/plugin.json (Phase 2/3 hook wiring), agents-plugin/skills/ (Phase 4 lead-use-mailbox skill), agents-plugin-wsflow/ (Phase 4 mirror target) |
| scope.surface | public-interface | new ws-mcp mailbox wait CLI subcommand, Codex/Claude hook adapter wiring, and the lead-use-mailbox skill are all harness- or user-facing surfaces |
| scope.new_public_symbol | yes | the ws-mcp mailbox wait CLI subcommand (Phase 1) and the lead-use-mailbox skill (Phase 4) |
| scope.new_type_contract | yes | the wait CLI's exit/timeout contract and the listening marker format (Phase 1) |
| scope.test_surface | existing | agents-plugin-tool/cmd/ws-mcp/main_test.go covers existing CLI subcommands, agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go and wsflow_mirror_test.go plus agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py cover skill-shim/mirroring drift, matching Phase 4's own verification bullet |
| complexity.reuse_points | confirmed | the CLI subcommand dispatch pattern used by existing git/tickets/config subcommands, agents-plugin-tool/cmd/ws-mcp/main.go#L25-L53, is the pattern a new mailbox subcommand follows; Phase 4 reuses the skill-authoring/wsflow-mirroring pattern. Phases 2-3's hook wiring have no surviving code to reuse: grep for hooks in agents-plugin/.codex-plugin/plugin.json and agents-plugin/.claude-plugin/plugin.json returns nothing, and 260504-feat-ws-mcp-hook-driven-interrupt's hook code is fully retired (research ticket's grep for agents.interrupt/mercenary in current Go source returns nothing) — only its design shape is prior art |
| complexity.side_effect_risk | moderate | a Stop hook can block/re-invoke the harness turn loop and a background wait process interacts with harness lifecycle, though it is gated to opted-in WS_MAILBOX/WS_MAILBOX_AUTO sessions |
| risk.correctness | moderate | level-triggered lost-wakeup avoidance, the Stop re-entry loop guard (stop_hook_active), and owner-context misfire gating are load-bearing but build on the core ticket's already-Confirmed durable-queue design (260913-feat-cross-session-mailbox-core, pending) |
| risk.fit | low | Decisions 6, 7, 9, and the wake half of 14 match 260913-research-cross-session-mailbox's Confirmed Decisions section verbatim, and the CLI-dispatch and skill-mirroring reuse points exist in the tree as described |
| risk.test | moderate | Phase 1 is unit-testable host-neutrally, but Phases 2-3's verification bullets need live Codex 0.154.0 / Claude harness behavior (hook trust, Stop re-invoke, run_in_background notification) that automated tests cannot fully cover |
| risk.security_or_contract | moderate | the hook-layer misfire guard is explicitly best-effort (Decision 9); the real gate is the core ticket's server-layer caller-equals-owner check, which this ticket depends on (pending 260913-feat-cross-session-mailbox-core) but does not itself implement |

## Phases

### Phase 1: Host-neutral wake CLI + env-less registration

Implement `ws-mcp mailbox wait --timeout` as a blocking CLI (never an MCP tool):
on startup it synchronously checks the durable queues the caller is entitled to
drain (the owner's name-keyed inbox when a slug is set, and always the caller's
own reply-id queue) and returns immediately if any unread exists; otherwise it
blocks until arrival or the timeout, then exits. It writes and clears a listening
marker so an armed wait is discoverable. The host-neutral deliverable for the
env-less path (wake half of 14) is only that the `wait` CLI can target the
caller's **own reply-id queue without a slug**; actually *arming* it as a harness
background task at piggyback time is a host mechanism owned by Phases 2-3 (and, per
Decision 6, the model's non-implicit obligation triggered by the core's piggyback
badge). This phase is host-neutral — the CLI, its exit semantics, and the marker
are the contract; how each host launches it and re-invokes on exit is Phases 2-3.

Verification (host-neutral, no harness hook needed):

- `wait` returns immediately (exit 0, reports the unread) when the durable queue
  already holds unread mail at startup — level-triggered, no lost wakeup across
  the arm/drain gap.
- `wait` on an empty queue blocks, then returns promptly when mail is deposited.
- `wait --timeout` returns cleanly (distinguishable exit/stdout) on timeout with
  no mail.
- The listening marker is written while armed and cleared on exit.
- An env-less caller's `wait` targets its own reply-id queue and returns on a
  reply addressed to that reply-id.

### Phase 2: Codex hook adapter

Wire the Codex adapter over Phase 1 (depends on the Phase 1 CLI + marker).
A `Stop` hook returns `{"decision":"block","reason":...}` to drain/act and
re-invoke at the turn boundary, guarding re-entry on `stop_hook_active`. The
causal trigger: the `Stop` hook fires at each turn-conclude (the hook event
itself), and on each firing does a **level read** of the durable queue /
listening marker, returning `decision:block`+`reason` **only when unread mail is
pending** — a per-turn-boundary level check, not a poll loop, so it honors
Decision 16's no-polling (contrast Claude's `run_in_background` completion →
task-notification chain in Phase 3). Bake the
owner identity into the hook command args (no payload classifier exists). Use
`PostToolUse` only as a side-effect marker (its output never reaches the model).
Persist hook trust as a documented deployment step (0.154 gates hooks behind
trust). Keep all Codex-specific wiring in the adapter layer, not the host-neutral
contract.

Verification (Codex CLI 0.154.0):

- A `Stop` hook `decision:block`+`reason` re-invokes the model, which drains and
  acts on queued mail; the re-entry `Stop` carrying `stop_hook_active` does not
  loop.
- The `Stop` hook returns `decision:block` only when the level read finds unread
  mail pending; a turn-conclude with an empty queue concludes normally (no
  per-turn poll loop).
- Owner-context: the arm-reminder/drain fires for the owner turn (identity from
  hook args) and the server-layer `caller == owner` gate still blocks a non-owner
  key regardless of hook firing.
- With hook trust persisted, the adapter's hooks run without an interactive trust
  prompt.

### Phase 3: Claude hook adapter

Wire the Claude adapter over Phase 1 (depends on the Phase 1 CLI + marker). A
single `Stop` hook gates on `hook_event_name == "Stop"` && `agent_id` absent →
root lead turn, so the arm-reminder fires only in the owner/root context and
never on a subagent stop. Use `run_in_background` on the wait CLI so its
task-notification completion re-invokes the agent.

Verification (Claude):

- The `Stop` hook fires the arm-reminder only in the root turn (`agent_id`
  absent); a subagent stop / `SubagentStop` does not emit it.
- A `run_in_background` wait completing produces a task notification that
  re-invokes the agent to drain.
- Env-less arm-at-badge: an env-less session that surfaces the core's piggyback
  badge arms a `run_in_background` wait on its own reply-id queue, and a reply to
  that reply-id re-invokes it — the at-piggyback-time registration end to end
  (the model-driven arming obligation of Decision 6 / wake half of 14, documented
  in the Phase 4 skill).

### Phase 4: lead-use-mailbox skill

Author a host-neutral guidance skill `lead-use-mailbox` under
`agents-plugin/skills/` (mirrored into `agents-plugin-wsflow/` per
`wsflow-mirroring.md`) that walks the lead/user through the end-to-end flow over
the landed Phases 1-3: launching a session with `WS_MAILBOX` or
`WS_MAILBOX_AUTO`, discovering the self-address (the `lookup_peers` self entry /
workflow ambient block), arming the wait, the send/recv flow, and the
remote-control-executor recipe (a discussion session driving an idle Codex
executor by mailing "run ticket X"). Guidance only — no new MCP tool. Read
`skill-authoring.md` and apply its invariant checklist. New skill: its existence
and scope are user-confirmed (the (A) consolidation choice); depends on Phases
1-3 so the documented flow actually exists.

Verification:

- The skill passes the `skill-authoring.md` invariant checklist and the plugin's
  skill-shim / mirroring drift tests (`agents-plugin` + `agents-plugin-wsflow`).
- The documented flow (launch → self-address → arm → send/recv → remote-control
  recipe) references only shipped, host-neutral surfaces, with Codex/Claude
  specifics called out as adapter behavior.
