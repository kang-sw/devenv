---
title: Cross-session mailbox core — host-neutral MCP surface, identity, and owner binding
related:
  260913-research-cross-session-mailbox: design-source — carries the settled invariants; this ticket implements the probe-independent subset
  260909-epic-ws-worker-interpreter-refoundation: constraint — no blocking in a tool call, no polling (Cross-Child Decision 16)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2a40dc42b327bcd0
sage-review-completeness-reviewed: 2a40dc42b327bcd0
---

# Cross-session mailbox core — host-neutral MCP surface, identity, and owner binding

## Background

Independent harness sessions in one project have no way to exchange messages;
`session.children`/`session.note` are pull-only over the parent chain, and the
harness's native messaging covers only the parent↔child (spawn) axis. This
ticket adds the **peer axis**: a host-neutral ws-MCP mailbox that lets one
harness-session endpoint send to and receive from another by a stable name.

Scope here is the **probe-independent core** derived from
`260913-research-cross-session-mailbox` (Confirmed Decisions 1–5 and 8). The
wake path (blocking CLI `wait` + harness background-task re-invoke), the harness
hook adapters, and the hook-layer arm-reminder misfire guard (Decisions 6, 7, 9)
are **deferred**: they depend on the unrun probe plan (Codex/Claude hook and
background-task semantics) and stay in the research ticket as follow-up. Without
the wake path this core delivers peer discovery + send + explicit `recv` + an
unread badge; the sender-driven remote-control-executor scenario is completed by
the deferred wake work.

## Decisions

Carried verbatim in intent from the research ticket's Confirmed Decisions
(1–5, 8); the deferred items (6, 7, 9) are named as out of scope.

- **Identity / opt-in (1):** a launch-time env `WS_MAILBOX=slug@scope`, which
  the ws-mcp server inherits at spawn. Absent → the mailbox is fully inert: no
  presence, no piggyback, zero overhead on unrelated sessions.
- **Self-registration (2):** the server reads `WS_MAILBOX` at startup and
  self-registers presence into the shared store, keyed by the mailbox **name**,
  with liveness (heartbeat/pidfile) and duplicate-live-name detection. No
  agent-facing `register` tool. On a **duplicate live name** (a second live
  process self-registers an already-live `slug@scope`), refuse to clobber the
  first process's presence record and surface the conflict through
  `lookup-peers` output; never silently last-writer-wins the presence entry or
  the owner pointer. Liveness threshold and queue retention are
  implementation-chosen with stated defaults: a peer is dead in `lookup-peers`
  after a bounded number of missed heartbeats (or a stale pidfile), and the
  per-name queue is retained until `recv` drains it, under a bounded size/TTL
  so a never-draining name cannot grow without limit. These are phase-level
  defaults, not cross-ticket contract.
- **Owner binding by ferrule, not bind-once (3):** rebind the owner on each
  `ferrule` call where `WS_MAILBOX` is set **and `parent_session_key == null`**.
  The trigger is a parent-less ferrule (top-lead login / `unknown_session →
  re-login` recovery), never "any lead-capability key" — worker mini-leads hold
  lead-capability child keys and must not steal ownership. `send` / `recv` /
  piggyback are hard-gated at the server on `caller_session_key == owner_key`;
  all other keys sharing the process are inert w.r.t. the mailbox.
- **Address = name, owner = internal pointer (4):** peers address the stable
  `slug@scope` name; queue and presence are name-keyed; the owner session_key is
  a rebindable internal pointer. Top-lead key churn is invisible to senders, and
  queued mail survives key loss (drained by whoever currently owns the name).
- **MCP surface (5):** three tools, all **non-blocking**, following this
  server's `domain_verb` registered-name convention (cf. `note_write`,
  `todo_add`): `mailbox_send(to: slug@scope, content)`, `mailbox_recv` (drain),
  and `mailbox_lookup_peers(layer)` — referenced in docs and badge copy as the
  canonical dotted form `mailbox.send` / `mailbox.recv` / `mailbox.lookup-peers`.
  No blocking `wait` on the MCP surface — a blocking tool would freeze the
  single multiplexed stdio channel shared by the lead and every native
  subagent, and re-cross refoundation Decision 16.
- **Piggyback badge (spine):** a single central dispatch wrapper appends an
  "unread N: from X → call `mailbox.recv`" badge to the **owner** session's tool
  responses only (gated by the owner check above). One interception point, not
  per-tool wiring.
- **Endpoint granularity (8):** one endpoint per harness root session per
  `WS_MAILBOX`. Sub-agent-level addressing is intra-session and out of scope
  (owned by native harness messaging).
- **Deferred (6, 7, 9) — explicitly not in this ticket:** the blocking CLI
  `ws-mcp mailbox wait --timeout` + model-driven background-task wake +
  listening marker; the Codex/Claude/pi hook adapters; and the hook-layer
  arm-reminder gated on the harness main-vs-subagent hook distinction. All
  probe-gated.

## Constraints

- `agents-plugin-tool/internal/mcp/` → read `ai-docs/manuals/ws-mcp.md` before
  editing (MCP operational runbook, launcher environment, tool surface).
- Any text that ships to a downstream project (tool-emitted strings, the badge
  wording) → read `ai-docs/manuals/shipped-surface-boundary.md`; keep it
  host-neutral, no dependency on anything only this repo has.
- Storage reuses existing roots: the machine-global session-key cache root
  co-locates all sessions on a machine; the `clone|worktree|machine` scopes map
  onto the existing wsnote layer roots (per-project shared / per-worktree cache
  / `~/.ws`). Do not invent a new cross-machine transport — cross-machine is out
  of scope (records carry no machine identity).
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Art

- `260504-feat-ws-mcp-hook-driven-interrupt` (retired) — the delivery shape;
  its ws-owns-the-subprocess premise is dead, its hook-driven delivery is not.
- `wsnote` layer/store (`agents-plugin-tool/internal/wsnote/`) — the layer→root
  resolution and file-store pattern to mirror for scope resolution.
- `session_auth.go` — `ferrule` mint, `parent_session_key`, and the
  machine-global `keys/` store to hook owner binding onto.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/ (new send/recv/lookup-peers tools, session_auth.go ferrule owner-binding), a new mailbox storage component, ai-docs/manuals/ws-mcp.md |
| scope.surface | public-interface | new MCP tools send, recv, lookup-peers |
| scope.new_public_symbol | yes | mailbox send/recv/lookup-peers MCP tools (Decisions, MCP surface (5)) |
| scope.new_type_contract | yes | WS_MAILBOX identity contract; ferrule parent_session_key == null owner-rebind trigger; server-side caller_session_key == owner_key gate (Decisions (1), (3)) |
| scope.test_surface | new-files | `grep -rln "mailbox\|WS_MAILBOX" --include="*.go" agents-plugin-tool/` returned nothing; no existing mailbox test files |
| complexity.reuse_points | confirmed | wsnote layer/store pattern (agents-plugin-tool/internal/wsnote/store.go#L17-L75, Layer type and MachinePath/WorktreePath/ClonePath resolvers); session_auth.go's ferrule/keys/ store (agents-plugin-tool/internal/mcp/session_auth.go#L104-L117); single tool-call dispatch point for the piggyback wrapper (agents-plugin-tool/internal/mcp/server.go#L495 callTool) |
| complexity.side_effect_risk | moderate | central piggyback badge wraps every owner session's tool response, and owner binding rebinds a shared pointer at ferrule time; both touch shared dispatch/session machinery even though gated to WS_MAILBOX-set sessions |
| risk.correctness | moderate | owner-gate misfire prevention and name-keyed durability across owner-key rebind are the load-bearing invariants, plus liveness/duplicate-name detection; scope is deliberately probe-independent and the underlying decisions are already Confirmed in 260913-research-cross-session-mailbox |
| risk.fit | low | Decisions 1-5, 8 verified verbatim against 260913-research-cross-session-mailbox's Confirmed Decisions section, and all three cited reuse points (wsnote, session_auth.go, callTool) exist in the tree as described |
| risk.test | moderate | Phase 1 lists 5 verification bullets achievable without the deferred harness-hook probe, but there is no existing mailbox test scaffolding to extend (new-files test surface) |
| risk.security_or_contract | moderate | the caller_session_key == owner_key gate is the sole boundary preventing cross-session mail disclosure/hijack; a defect there is a contract-relevant misdelivery, though blast radius is opt-in only (WS_MAILBOX unset stays fully inert) |

## Phases

### Phase 1: Host-neutral mailbox core

Implement the MCP surface (`send`, `recv`, `lookup-peers`), the `WS_MAILBOX`
launch-env identity with server self-registration and liveness, name-keyed
queue/presence storage over the existing scope roots, owner binding at
parent-less `ferrule` with the server-side `caller == owner` gate on
`send`/`recv`/piggyback, and the central dispatch piggyback badge for the owner
only. Inert entirely when `WS_MAILBOX` is unset.

Verification (all achievable without the probe / without any harness hook):

- `WS_MAILBOX` unset → no presence entry, no badge, no mailbox tools' effect
  (inert).
- Owner gate: a call carrying a non-owner session_key gets no badge and cannot
  `recv`/`send` the mailbox.
- ferrule rebind: a parent-less ferrule with `WS_MAILBOX` set rebinds the owner;
  a ferrule carrying `parent_session_key` does not.
- Name-keyed durability: mail sent to `slug@scope` is drained by the current
  owner after an owner-key rebind (key churn loses no mail).
- `lookup-peers(layer)` lists live peers at the requested scope and filters dead
  ones; a duplicate live-name registration is refused (the first process's
  presence is preserved) and the conflict is surfaced through `lookup-peers`.
- Piggyback badge appears on the owner's tool responses when unread > 0 and only
  there.
