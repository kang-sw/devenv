---
title: Cross-session mailbox core — host-neutral MCP surface, identity, and owner binding
related:
  260913-research-cross-session-mailbox: design-source — carries the settled invariants; this ticket implements the probe-independent subset
  260909-epic-ws-worker-interpreter-refoundation: constraint — no blocking in a tool call, no polling (Cross-Child Decision 16)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0e8af6755f7f8c34
sage-review-completeness-reviewed: 0e8af6755f7f8c34
---

# Cross-session mailbox core — host-neutral MCP surface, identity, and owner binding

## Background

Independent harness sessions in one project have no way to exchange messages;
`session.children`/`session.note` are pull-only over the parent chain, and the
harness's native messaging covers only the parent↔child (spawn) axis. This
ticket adds the **peer axis**: a host-neutral ws-MCP mailbox that lets one
harness-session endpoint send to and receive from another by a stable name.

Scope here is the **probe-independent core** derived from
`260913-research-cross-session-mailbox` (Confirmed Decisions 1–5, 8, and 10–13,
plus the piggyback half of 14). The wake path (blocking CLI `wait` + harness
background-task re-invoke and the env-less background-wait registration), the
harness hook adapters, and the hook-layer arm-reminder misfire guard (Decisions
6, 7, 9, and the wake half of 14) are **deferred**: they depend on the unrun
probe plan (Codex/Claude hook and background-task semantics) and stay in the
research ticket as follow-up. Without the wake path this core delivers peer
discovery + universal send (Decision 11) + explicit `recv` + an unread badge +
the two-tier `slug`/`reply-id` addressing (Decisions 12–13); the sender-driven
remote-control-executor scenario is completed by the deferred wake work.

## Decisions

Carried verbatim in intent from the research ticket's Confirmed Decisions
(1–5, 8, 10–13, piggyback half of 14); the deferred items (6, 7, 9, wake half
of 14) are named as out of scope.

- **Identity / opt-in (1):** a launch-time env `WS_MAILBOX=slug@scope`, which
  the ws-mcp server inherits at spawn, names a **durable, discoverable** endpoint
  explicitly. The env gates durable *receiving* (a named inbox + piggyback), not
  *sending* (see Universal send (11)). With neither `WS_MAILBOX` nor
  `WS_MAILBOX_AUTO` (10) set **and the session having neither sent nor
  self-looked-up** (both are channel-opening acts, Decisions 11 and 5) → the
  mailbox is fully inert: no presence, no piggyback, zero overhead on unrelated
  sessions. Explicit `WS_MAILBOX` overrides `WS_MAILBOX_AUTO`.
- **Universal send + always-on reply-id (11):** `send` is available to any
  session that carries a `session_key`, regardless of env — an env-less session
  can mail any discoverable address. The first `send` opens a live-only return
  channel: the server publishes the sender's **reply-id** =
  `HMAC(machine_secret, caller_session_key)` into the machine-tier presence
  registry (lazy expiry — a `last-seen` stamp refreshed on each send /
  session-aware call, stale entries reaped, since MCP-process exit is too short
  to hang cleanup on). The HMAC is required so the raw `caller_session_key`
  (the owner-gate secret, Decision 3) never leaks in an envelope; it is
  unguessable, and it is **deterministic**: `machine_secret` is a stable value
  **persisted once per machine** (a machine-level secret file under the cache
  root, minted on first use), so the reply-id **need not itself be persisted** —
  it is recomputed identically each MCP process from the persisted
  `machine_secret` and the (restart-stable) `caller_session_key`. That is what
  lets a reply-id survive an MCP-process restart while still dying at a
  parent-less `ferrule` (new `caller_session_key`). A session that never sends
  and has no env leaves no registry entry.
- **Polymorphic `to:` + server-stamped envelope (12):** `send(to:)` accepts
  `slug@scope` **or** `id:<reply-id>`. The sender's server stamps each delivered
  envelope: `from: <slug>` when the recipient shares a store tier where the
  sender published a slug (a durable reply path), else `reply-to: id:<reply-id>`
  only (a live-only reply path). `recv` surfaces, per message, whichever handle
  the envelope carried; the recipient replies with that handle. Reply-to is the
  sender's own durable handle, not a per-message TTL capability, so threads chain
  naturally.
- **Reachability / durability asymmetry (13):** discovery is scope-bounded
  (visibility only; `lookup_peers` filters by scope, no cross-scope
  enumeration); delivery is capability-gated (need a reply-id/slug you hold).
  Durability: a **reply-id lives for the ferrule span while the session stays
  active** (anchor = `caller_session_key`, which survives MCP restart and
  compaction-revive but is re-minted at a parent-less `ferrule`; the lazy-expiry
  reap window of Decision 11 must be chosen long enough not to drop a still-live
  session's reply-id before its ferrule ends); a **clone/worktree slug is
  cross-ferrule durable within its scope**; a **machine slug is cross-ferrule
  durable everywhere**. Corollary: cross-scope *and* cross-ferrule durable reach
  requires
  a machine-scope slug. The anchor is `caller_session_key` (not a host-process
  id) specifically so the contract is identical on Windows/macOS/Linux with zero
  OS-specific code.
- **Auto-identity (10):** an alternative launch-time env
  `WS_MAILBOX_AUTO=<machine|clone|worktree>`. When set (and `WS_MAILBOX` is
  not), the server mints a **random 3-word stem** at startup (reusing the
  impl-branch / session-key word-chain generator) and self-registers as
  `<stem>@<scope>`; `<scope>` selects **visibility only**. The name is
  intentionally non-deterministic, so several sessions in one worktree each get
  a distinct, collision-free address and no harness discriminator is needed. The
  stem is per-server-process (stable across `ferrule` re-login, regenerated on a
  full restart), so the auto address is discoverable, not durable. Because the
  address is unpredictable, the resolved `<stem>@<scope>` MUST be surfaced: the
  server exposes it in the workflow ambient block and `mailbox.lookup_peers`
  returns the caller's own endpoint as a self entry; each presence record carries
  descriptive metadata (harness, cwd, started-at) so a human can identify a peer
  before relaying its address.
- **Self-registration (2):** the server reads `WS_MAILBOX` at startup and
  self-registers presence into the shared store, keyed by the mailbox **name**,
  with liveness (heartbeat/pidfile) and duplicate-live-name detection. No
  agent-facing `register` tool. On a **duplicate live name** (a second live
  process self-registers an already-live `slug@scope`), refuse to clobber the
  first process's presence record and surface the conflict through
  `lookup_peers` output; never silently last-writer-wins the presence entry or
  the owner pointer. Liveness threshold and queue retention are
  implementation-chosen with stated defaults: a peer is dead in `lookup_peers`
  after a bounded number of missed heartbeats (or a stale pidfile), and the
  per-name queue is retained until `recv` drains it, under a bounded size/TTL
  so a never-draining name cannot grow without limit. These are phase-level
  defaults, not cross-ticket contract.
- **Owner binding by ferrule, not bind-once (3):** rebind the owner on each
  `ferrule` call where **`WS_MAILBOX` or `WS_MAILBOX_AUTO` is set** **and
  `parent_session_key == null`**. The trigger covers both identity modes: an
  auto-identity mailbox must survive the same `unknown_session → re-login`
  recovery, so it rebinds too — only the owner `session_key` pointer changes at
  `ferrule`; the endpoint address (`slug@scope` or the auto `<stem>@<scope>`) is
  **not** re-minted at `ferrule` (Decisions 4, 10). The trigger is a parent-less
  ferrule (top-lead login / `unknown_session → re-login` recovery), never "any
  lead-capability key" — worker mini-leads hold lead-capability child keys and
  must not steal ownership. The `caller_session_key == owner_key` gate governs
  the **named slug inbox** (`send` to it, `recv` from it, and its piggyback
  badge); all other keys sharing the process are inert w.r.t. that named inbox.
  It does **not** gate a caller's **own reply-id queue** (Universal send (11)):
  that queue is authorized intrinsically by the caller's own `session_key` (the
  reply-id is its HMAC), so an env-less caller with no owner pointer can still
  `recv` and be badged for replies addressed to its reply-id.
- **Address = name, owner = internal pointer (4):** peers address the stable
  `slug@scope` name; queue and presence are name-keyed; the owner session_key is
  a rebindable internal pointer. Top-lead key churn is invisible to senders, and
  queued mail survives key loss (drained by whoever currently owns the name).
- **MCP surface (5):** three tools, all **non-blocking**, following this
  server's `domain.verb` registered-name convention — the registered `name` is
  the dotted form itself, not `domain_verb` (`agents-plugin-tool/internal/mcp/server.go#L3147`
  `"name": "todo.add"`, `#L3464` `"name": "note.write"`): `mailbox.send(to:
  <slug@scope | id:reply-id>, content)` (polymorphic `to:`, Decision 12),
  `mailbox.recv` (drain; surfaces each message's return handle), and
  `mailbox.lookup_peers(scope)` — which also returns the caller's own endpoint as
  a self entry (with its descriptive metadata, and for an env-less caller its own
  live-only reply-id) so an agent can answer "what is my address". Because a
  reply-id is only useful once a peer can deliver to it, an env-less self-lookup
  **publishes** the caller's reply-id registry entry (the same channel-open a
  `send` performs) rather than returning an unreachable handle; a session that
  neither sends nor self-looks-up stays inert (Decision 11). The tools' dotted
  registered names are `mailbox.send`/`mailbox.recv`/`mailbox.lookup_peers`;
  elsewhere in this ticket they are referred to in shorthand as
  `send`/`recv`/`lookup_peers`. (The scope param names the same
  machine/clone/worktree tier as the address `@scope`; it maps onto the internal
  wsnote layer roots — see Constraints.)
  No blocking `wait` on the MCP surface — a blocking tool would freeze the
  single multiplexed stdio channel shared by the lead and every native
  subagent, and re-cross refoundation Decision 16.
- **Piggyback badge (spine; piggyback half of 14):** a single central dispatch wrapper appends an
  "unread N: from X → call `mailbox.recv`" badge to a session's tool responses
  when it has unread mail — for the named-inbox owner (gated by the owner check
  above) **and** for any caller with unread in its own reply-id queue (authorized
  by its `session_key`, per Decisions 11/3). One interception point, not per-tool
  wiring.
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
- `session_auth.go` — the machine-global `keys/` store and
  `sessionEntry`/`sessionStore` types, the natural owner-binding hook; the
  `ferrule` bootstrap mint and `parent_session_key` handling live in `server.go`
  (`bootstrapToolName` server.go:70, `parent_session_key` read
  server.go:1518-1522, schema server.go:3073).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/ (new send/recv/lookup_peers tools, session_auth.go ferrule owner-binding), a new mailbox storage component, ai-docs/manuals/ws-mcp.md |
| scope.surface | public-interface | new MCP tools send, recv, lookup_peers |
| scope.new_public_symbol | yes | mailbox send/recv/lookup_peers MCP tools with polymorphic to: and reply-id return handles (Decisions, MCP surface (5), (11)-(13)) |
| scope.new_type_contract | yes | WS_MAILBOX / WS_MAILBOX_AUTO identity contracts; ferrule (WS_MAILBOX or WS_MAILBOX_AUTO set) parent_session_key == null owner-rebind trigger; server-side caller_session_key == owner_key gate; reply-id = HMAC(machine_secret, caller_session_key) with ferrule-span lifetime; polymorphic to: (slug@scope | id:reply-id) + server-stamped envelope return handle (Decisions (1), (3), (10), (11), (12), (13)) |
| scope.test_surface | new-files | `grep -rln "mailbox\|WS_MAILBOX" --include="*.go" agents-plugin-tool/` returned nothing; no existing mailbox test files |
| complexity.reuse_points | confirmed | wsnote layer/store pattern (agents-plugin-tool/internal/wsnote/store.go#L17-L75, Layer type and MachinePath/WorktreePath/ClonePath resolvers); session_auth.go's ferrule/keys/ store (agents-plugin-tool/internal/mcp/session_auth.go#L104-L117); single tool-call dispatch point for the piggyback wrapper (agents-plugin-tool/internal/mcp/server.go#L495 callTool) |
| complexity.side_effect_risk | moderate | central piggyback badge wraps every owner session's tool response, and owner binding rebinds a shared pointer at ferrule time; both touch shared dispatch/session machinery even though gated to WS_MAILBOX-set sessions |
| risk.correctness | moderate | owner-gate misfire prevention and name-keyed durability across owner-key rebind are the load-bearing invariants, plus liveness/duplicate-name detection; scope is deliberately probe-independent and the underlying decisions are already Confirmed in 260913-research-cross-session-mailbox |
| risk.fit | low | Decisions 1-5, 8, and the newly folded 10-13 (plus the piggyback half of 14) verified verbatim against 260913-research-cross-session-mailbox's Confirmed Decisions section, and all three cited reuse points (wsnote, session_auth.go, callTool) exist in the tree as described; the machine_secret/HMAC primitive is new (no existing one in session_auth.go) and homes in the machine-global keys/ cache root |
| risk.test | moderate | Phase 1's verification bullets are all achievable without the deferred harness-hook probe (inert/universal-send/env-less-drain/reply-id-restart-stability/envelope-stamping/owner-gate/ferrule-rebind/tiered-durability/lookup_peers/piggyback), but there is no existing mailbox test scaffolding to extend (new-files test surface) |
| risk.security_or_contract | moderate | two boundaries: the caller_session_key == owner_key gate preventing cross-session mail disclosure/hijack, and the reply-id HMAC that must not leak the raw caller_session_key (the owner-gate secret) into envelopes; a defect in either is contract-relevant, though the receiving blast radius stays opt-in (no WS_MAILBOX/AUTO + no send stays fully inert, and send only ever exposes an HMAC'd handle) |

## Phases

### Phase 1: Host-neutral mailbox core

Implement the MCP surface (`send` with polymorphic `to:`, `recv` surfacing each
message's return handle, `lookup_peers` incl. a self entry), the `WS_MAILBOX` and
`WS_MAILBOX_AUTO` launch-env identities with server self-registration and liveness
(auto mints a random 3-word stem; `<scope>` = visibility only), universal send
with the machine-tier **reply-id** registry (`HMAC(machine_secret,
caller_session_key)`, lazy expiry via `last-seen` refresh) opened on first send
even without env, the sender-server envelope stamping (`from: <slug>` when the
recipient shares the sender's slug tier, else `reply-to: id:<reply-id>`),
name-keyed queue/presence storage over the existing scope roots (presence records
carrying harness/cwd/started-at metadata), owner binding at a parent-less
`ferrule` (with either `WS_MAILBOX` or `WS_MAILBOX_AUTO` set; only the owner
pointer rebinds, never the address) with the server-side `caller == owner` gate on
`send`/`recv`/piggyback for the **named inbox**, plus `recv`/piggyback of a
caller's **own reply-id queue** authorized by its `session_key` (so an env-less
sender can drain and be badged for replies), the central dispatch piggyback badge,
and the self-address surface (workflow ambient block + `lookup_peers` self entry;
an env-less self-lookup publishes the caller's reply-id entry). Inert entirely
when neither `WS_MAILBOX` nor `WS_MAILBOX_AUTO` is set **and the session has
neither sent nor self-looked-up**.

Verification (all achievable without the probe / without any harness hook):

- Neither `WS_MAILBOX` nor `WS_MAILBOX_AUTO` set and the session has neither sent
  nor self-looked-up → no presence entry, no badge, no mailbox tools' effect
  (inert).
- Universal send: an env-less session `send`s to a discoverable address; the
  server publishes its reply-id into the machine-tier registry, and the reply-id
  is the HMAC of `caller_session_key` (never the raw key). A recipient replying
  via `id:<reply-id>` reaches the env-less sender while its `session_key` holds;
  after a parent-less ferrule (new key) that reply-id no longer resolves.
- Env-less return path: after the above, the env-less sender can `recv` the reply
  addressed to its own reply-id and gets a piggyback badge for it — even though it
  has no owner pointer (the reply-id queue is authorized by its own `session_key`,
  not the owner gate).
- Reply-id restart-stability: within one ferrule span, a reply-id computed before
  an MCP-process restart resolves identically after it (same `machine_secret` +
  same `caller_session_key`), so a previously handed-out reply-id still delivers.
- Envelope stamping: mail from a slug'd sender to a recipient sharing that slug's
  tier arrives stamped `from: <slug>`; mail from an env-less (or cross-tier)
  sender arrives stamped `reply-to: id:<reply-id>` only.
- `WS_MAILBOX_AUTO=<scope>` set (no `WS_MAILBOX`) → server registers a random
  `<stem>@<scope>`; two sessions in the same worktree get distinct stems (no
  collision); explicit `WS_MAILBOX` overrides `WS_MAILBOX_AUTO` when both set.
- Self-address: an agent obtains its own `<stem>@<scope>` (ambient block and a
  `lookup_peers` self entry), and `lookup_peers` shows peers' descriptive
  metadata (harness/cwd/started-at).
- Owner gate: a call carrying a non-owner session_key gets no badge for and
  cannot `recv`/`send` the **named inbox** (it may still access its own reply-id
  queue).
- ferrule rebind: a parent-less ferrule with `WS_MAILBOX` **or
  `WS_MAILBOX_AUTO`** set rebinds the owner pointer; a ferrule carrying
  `parent_session_key` does not. Across the rebind the endpoint address is
  unchanged — only the owner `session_key` moves (the auto `<stem>@<scope>` is
  not re-minted at `ferrule`), and mail queued before the rebind stays drainable
  by the new owner.
- Name-keyed durability: mail sent to `slug@scope` is drained by the current
  owner after an owner-key rebind (key churn loses no mail).
- Tiered slug durability: a machine-scope slug is reachable (and stamped `from:`)
  from a cross-scope peer, so it survives that peer's ferrule; a clone/worktree
  slug reaches a cross-scope peer only as a `reply-to: id:` handle (which dies at
  the slug-holder's next ferrule) — confirming cross-scope + cross-ferrule reach
  needs a machine-scope slug.
- `lookup_peers(scope)` lists live peers at the requested scope and filters dead
  ones; a duplicate live-name registration is refused (the first process's
  presence is preserved) and the conflict is surfaced through `lookup_peers`.
- Piggyback badge appears on a session's tool responses when it has unread mail
  (owner's named inbox, or the caller's own reply-id queue) and only then.
