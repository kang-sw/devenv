---
title: "Pi lead bootstrap: system-prompt-injected workflow manual + Pi lead guide, bridge session-key normalization, workflow_manual -> workflow_state mapping"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260903-feat-ws-pi-subagent-rpc-ux: sibling, lands after this ticket — carries the process-role marker forward onto its persistent RpcClient spawner and replaces the seeded spawner rows in the Pi lead guide with its own tool set
  260903-feat-ws-pi-goal-loop-compaction-hook: sibling — the manual living in the system prompt survives Pi compaction natively, so post-compaction recovery reduces to a workflow_state call
  260904-feat-ws-pi-execute-approval-gateway: sibling — contributes its verb lines (ws.execute / ws.approve) to the Pi lead guide block this ticket owns
  260904-feat-ws-pi-side-thread-fork-question-surface: sibling — the fork's parent-lead-key rewrite (its §2) is one case of the normalization layer this ticket owns; contributes ws.fork / ws.ask lines to the guide block
  260802-research-ws-pi-native-framework: research anchor (Pi hook inventory)
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b840f04e8a372fc3
sage-review-completeness-reviewed: b840f04e8a372fc3
---

# Pi lead bootstrap: system-prompt-injected workflow manual + Pi lead guide, bridge session-key normalization, workflow_manual -> workflow_state mapping

## Background

On Claude and Codex the ws workflow manual reaches the lead only through a
model-invoked tool call: the entry-point `lead-*` skills (`lead-discuss`,
`lead-revive`, `lead-write-ticket`, `lead-backfill-docs`,
`lead-goal-fan-out-step`) say
"call `ws/workflow_manual(session_key: <your key or "obsidian-latch" if
fresh>, root: ...)`", and the other lead skills rely on an earlier entry-skill
call having bootstrapped the session, because those hosts give the plugin no
hook on the lead's system prompt. That call path is a workaround for hosts we do not control, and it
carries the whole session-key handshake with it: the model must learn a key
from the FRESH response and repeat it on later calls.

On Pi the adapter owns the extension, so the workaround is unnecessary and its
key handshake is actively harmful. The Pi bridge (`agents-plugin-pi/src/bridge.ts`)
already mints its own lead key at `session_start` via `ferrule` and holds it as
the default for omitted-`session_key` calls (fill-or-forward; the registered
schemas list `session_key` as optional). But the bundled skills are byte copies
of the canonical text, so a lead that follows them literally passes the
`obsidian-latch` sentinel, the bridge forwards it verbatim, and ws-mcp's FRESH
mode **mints a second lead key** and returns it inline. From then on the model's
ws state (agenda/todos) lives under the model-learned key while the spawner
registers child lineage under the bridge's default key — two keys per Pi
session, each seeing half the picture. Whether a given model takes the
omit-branch or the sentinel-branch is model behavior, not a contract.

The pending Pi tickets each add lead verbs (`ws.execute`/`ws.approve`,
`ws.fork`/`ws.ask`) and none owns the text that tells the lead when to reach
for which verb; ws-mcp's manual cannot carry Pi-specific prose (golden rule),
and the bridge's only precedent is appending an advisory to `workflow_manual`
responses (`MODEL_CATALOG_ADVISORY`).

This ticket lands first in the Pi drain order: the other four tickets attach
their guide lines and key cases to the surfaces it defines. User decisions
(2026-09-04): inject the manual plus Pi-extension prose into the lead's system
prompt; map the model's `workflow_manual` calls onto `workflow_state`; the
bridge normalizes session keys mechanically.

## Decisions

### 1. Workflow manual + Pi lead guide live in the lead's system prompt

- Hook: `pi.on("before_agent_start", ...)` — fires before every agent run and
  its result may return `systemPrompt` (chained across extensions) and/or a
  persistent `message`. The extension appends, never replaces: it returns
  `event.systemPrompt + "\n\n" + <ws block>`.
- Content of the ws block, in order:
  1. the **full `workflow_manual` CONTINUE response** as of session start,
     obtained by the bridge itself right after the `ferrule` bootstrap through
     `workflow_manual(session_key: <own key>)`: the static manual body plus
     ws-mcp's session-start material — `## Session Key`, `## Session State`,
     repo notes, and the advisory blocks (`bootstrapStalenessWarning`,
     `docCoverageWarning`, `scopeAnnouncement`, `computeManuals`, review
     checkpoint / review-track nudges) — prefixed by one fixed line marking
     the dynamic part as a **session-start snapshot** ("current state comes
     from `workflow_manual`"). This is deliberately a one-shot snapshot (user
     decision 2026-09-04): the dynamic material is refreshed only when an
     entry-point skill calls `workflow_manual` (§3), the same cadence as on
     Claude/Codex, never per turn. The manual text has a single source
     (ws-mcp render); the adapter never carries a copy. The bridge also
     renders `playbook.print(name: "lead-workflow-manual", session_key: <own
     key>)` once at session start — `workflow_manual` calls the same
     `printPlaybook` internally — and keeps it as the **static-body
     snapshot** §3 cuts by;
  2. the **Pi lead guide**: an adapter-owned prose file
     (`agents-plugin-pi/pi-lead-guide.md`, sibling of `model-catalog.json`,
     read at `session_start`; added to `package.json`'s `files` whitelist so
     the installed tarball ships it, not only a dev `-e` checkout) explaining the Pi-specific lead surface — how
     `session_key` is handled for it (never needed), that the manual is
     already present so `workflow_manual` returns only its dynamic part, and a
     **verb routing table** with one row per Pi lead verb. This ticket lands
     **first** in the Pi drain order, so it seeds the rows for the tools that
     exist in the tree today (`ws-agent-spawn`, `ws-agent-continue`,
     `ws-agent-wait`, `explore`); each later ticket owns its own rows —
     `260903` subagent-rpc-ux replaces the spawner rows with its
     `spawn`/`send`/`wait`/`list`/`stop`/`transcript` set when it lands,
     `260904` gateway adds `ws.execute`/`ws.approve`, `260904` side-thread adds
     `ws.fork`/`ws.ask`/`ws.resolve`.
- Fetched once per `session_start`; the block is held in extension memory and
  re-applied on every `before_agent_start`. A `/reload` re-runs `session_start`
  and therefore refreshes it; no per-turn re-fetch.
- Size: the rendered manual is ~2.1k words; prefix-cache churn is explicitly
  not a design consideration (user directive, carried from the side-thread
  discussion).
- Rejected: injecting the manual as a persistent `message` (it would sit in
  the transcript and be subject to compaction; the system prompt survives Pi
  compaction natively, which is the point); **per-turn refresh** of the
  dynamic part — a `before_agent_start` fetch of the current state/advisories
  injected as a message when changed — rejected by the user: entry-point
  cadence is sufficient, and re-delivering accumulating material (repo notes,
  session state) every turn is not cheap in context.

### 2. Bridge session-key normalization — narrow, mechanical

The existing fill-or-forward contract
(`spec/pi-adapter-runtime.md {#260903-pi-bridge-session-key-fill-forward}`)
stays: omitted key → own key; an explicit key is forwarded verbatim so a lead
driving children with their keys, or multi-track orchestration, still reaches
ws-mcp unchanged. This ticket adds exactly **two** rewrite cases in front of
that rule, both decided by string equality against values the adapter knows
out-of-band:

| explicit `session_key` value | rewritten to | why |
| --- | --- | --- |
| the fresh-bootstrap sentinel `obsidian-latch` | own key | the skill text's FRESH path must never double-mint on Pi; the bridge already bootstrapped |
| the **parent lead's key** (present only in a process spawned as a side-thread fork, delivered via the spawn environment — see `260904-feat-ws-pi-side-thread-fork-question-surface` §2) | own key | the fork inherits a transcript that names the lead's key; ws-mcp keys agenda/todos per key, so forwarding it would clobber the lead's state |

Any other explicit key passes through untouched. The rewrite is a pure
function (`normalizeSessionKey(params, { ownKey, sentinel, parentLeadKey? })`)
next to `resolveSessionKey`, unit-tested in isolation. It is a bridge-layer
mechanism, never a prompt instruction — consistent with the side-thread
ticket's "no prose-only mitigation" constraint.

### 3. `workflow_manual` calls are mapped onto `workflow_state`

Because the manual is already in the system prompt, a model-invoked
`ws__workflow_manual` call must return the **state-and-advisories view** —
everything ws-mcp computes per call — and never the manual body again:

- Primary: the bridge forwards the call to ws-mcp `workflow_manual` with the
  (normalized) key and **cuts the static manual body out of the response**
  by exact substring match against the static-body snapshot (§1, the
  `playbook.print` render), which leaves the per-call advisory blocks,
  `## Session Key`, `## Session State`, and the repo notes. This preserves ws-mcp's contract that those blocks are
  recomputed on every call (a staleness or doc-coverage warning the user
  fixes mid-session stops nagging; one that becomes true later surfaces).
- Fallback: if the snapshot body is not found in the response (renderer
  changed mid-session), dispatch ws-mcp `workflow_state` instead — it renders
  `## Session State` + repo notes, byte-identical to the corresponding
  `workflow_manual` suffix, has no FRESH mode, and never mints — and notify
  once that per-call advisories are unavailable for this session. Either way
  the canonical skills get what they need from the call on Pi: state restore
  (including the post-compaction `lead-revive` path), not the manual body.
- Role-gated like §4: the cut/mapping and the prepended line apply only in
  lead and fork processes; a worker or `explore` that calls
  `ws__workflow_manual` (`full-worker` includes the live `ws__*` names) has
  the call forwarded verbatim, since its system prompt carries no manual.
- Degraded path: if the `session_start` `ferrule` bootstrap or the manual
  fetch failed (own key unset or no snapshot), §2's sentinel rewrite and this
  mapping are both **disabled** for the session — the sentinel passes
  through and `workflow_manual` is forwarded verbatim, so the model's own
  FRESH call self-heals exactly as today — and the bridge notifies that the
  bootstrap is degraded. The existing unset-key posture
  (`{#260903-pi-bridge-session-key-fill-forward}`, last paragraph) is kept.
- The bridge prepends one fixed line to the mapped response — "Workflow
  manual is in your system prompt; this is your current session state." — so
  a model that expected the manual is not confused by its absence.
- The unset-tier advisory (`MODEL_CATALOG_ADVISORY`,
  `{#260903-pi-model-catalog-unset-advisory}`) keeps riding the mapped
  `workflow_manual` response with its per-call cadence; it is keyed on the
  *registered* name the model called, not on the wire tool.
- On the `workflow_state` fallback, `root` and other `workflow_manual`-only
  arguments are dropped; `workflow_state`'s own error path for an
  unresolvable key is surfaced unchanged (fail-loud, never minting).
- The canonical skill text is **not** edited (`agents-plugin/skills/` is
  read-only source for the adapter; the Pi package copies it at pack time).
  No Pi-specific skill shims (the `wsflow` thin-shim precedent exists but is
  not needed while the mapping keeps the canonical call harmless).

### 4. Process-role gating — lead and fork only, never workers

Spawned children (`ws-agent-spawn` workers, `explore` leaves) run `pi` with
the same installed extension, so the bridge and its hooks execute in them too.
The lead manual must **not** be appended to a worker's system prompt (workers
get their rendered playbook via `--append-system-prompt`, and the lead manual
would reintroduce the lead's delegation posture into a worker — the
persona-bleed shape the fork tickets guard against). Gating:

- The current spawner (`agents-plugin-pi/src/spawner.ts`; `260903` carries
  this forward onto its RpcClient rewrite) sets a process-role marker in the child's environment (`WS_PI_SPAWN_ROLE=worker |
  explore | fork`, plus `WS_PI_PARENT_SESSION_KEY=<lead key>` for `fork`).
  Absent marker = host lead process.
- `before_agent_start` appends the ws block when the role is absent (lead) or
  `fork` (lead-caliber peer, needs the same manual and guide). It appends
  nothing for `worker` / `explore`.
- The same marker feeds §2's `parentLeadKey` and lets the bridge mint the
  fork's key with lineage (`ferrule(root, capability: lead,
  parent_session_key: <lead key>)`); worker/explore minting is unchanged.

### 5. Headless lead (`--mode rpc`) is a lead

A user-launched headless lead has no spawn marker and receives the ws block
like the TUI lead; nothing in this ticket depends on `ctx.ui`.

## Constraints

- Golden rule: `agents-plugin-tool/` (ws-mcp Go) is untouched; the adapter
  consumes `workflow_manual`, `workflow_state`, and `ferrule` as-is.
- System prompt is append-only (chained onto `event.systemPrompt`); never
  replaced.
- Canonical skill text unchanged; the manual text is never duplicated into the
  adapter.
- Rewrite cases are exactly the two in §2; widening them (e.g. rewriting every
  explicit key) would break child-key driving and is rejected.

## Prior Art

- `agents-plugin-pi/src/bridge.ts`: `resolveSessionKey` (fill-or-forward),
  `withOptionalSessionKey` (schema relaxation), the `ferrule` bootstrap in
  `startBridge`, `maybeAppendModelCatalogAdvisory` (response-append
  precedent).
- `agents-plugin-tool/internal/mcp/workflow_manual.go`: `freshBootstrapKey =
  "obsidian-latch"`; `handleWorkflowManual` modes FRESH / CONTINUE /
  FAIL-LOUD; `handleWorkflowState` state-only view, no FRESH mode.
- `agents-plugin-tool/internal/mcp/server.go` `handleLeadLogin` (`ferrule`):
  accepts `root`, `format`, `capability`, `parent_session_key`.
- Pi 0.84.4 `dist/core/extensions/types.d.ts`: `BeforeAgentStartEvent`
  (`systemPrompt`, `systemPromptOptions`) and `BeforeAgentStartEventResult`
  (`message?`, `systemPrompt?`); `docs/extensions.md` "before_agent_start
  (can inject message, modify system prompt)".
- `agents-plugin-wsflow/`: thin skill shims over shared playbooks (rejected
  here, recorded as the fallback if the mapping ever proves insufficient).

## Spec Impact

Anchors are authored contract-first at proceed (first implementation slice),
against `pi-adapter-runtime`:

- New section: **Lead bootstrap** — system-prompt injection (content, order,
  hook, fetch cadence, role gating), the Pi lead guide file and its
  verb-table extension rule for later tickets.
- Amend `{#260903-pi-bridge-session-key-fill-forward}`: add the two
  normalization cases in front of fill-or-forward; keep the verbatim-forward
  rule for every other explicit key.
- New section: **workflow_manual -> workflow_state mapping** — dispatch rule,
  prepended line, dropped arguments, advisory cadence preserved.
- Amend `{#260903-pi-delegation-spawner-tools}`: the spawn-role environment
  marker and the fork's parent-key variable.

## Phases

### Phase 1: System-prompt bootstrap + key normalization + manual→state mapping

Implement in `agents-plugin-pi/src/`: the `before_agent_start` hook and ws
block assembly (full `workflow_manual` response fetched at `session_start`
with the snapshot marker line, `playbook.print` static-body snapshot kept
for the cut, `pi-lead-guide.md` appended); the seeded `pi-lead-guide.md` with the
verb table rows for the tools existing at landing; `normalizeSessionKey` in
front of `resolveSessionKey` with the two §2 cases; the `workflow_manual` ->
`workflow_state` dispatch mapping (body cut, `workflow_state` fallback,
role gate, degraded-path disable) with the prepended line and preserved
advisory; the `package.json` `files` entry; the spawner's `WS_PI_SPAWN_ROLE` / `WS_PI_PARENT_SESSION_KEY`
environment marker and the role gate in the hook. One phase by design (the
four pieces share no sequential dependency); suggested landing order inside
the phase, each a self-contained commit: normalization → mapping → role
marker + gate → system-prompt injection + guide file. Unit tests for
`normalizeSessionKey` (omit / sentinel / parent key / unrelated explicit key),
the mapping (argument drop, prepended line, advisory append on the mapped
response), and Session-State stripping.

Verification:

1. Live `pi -e <ext>` lead: the system prompt (via `ctx.getSystemPrompt()` in
   a debug command or the `systemPromptOptions` dump) contains the manual
   body, the session-start snapshot of state/advisories behind the marker
   line, and the guide; the block is byte-stable across later turns (no
   per-turn refresh).
2. Invoke a canonical `lead-*` skill whose body calls `workflow_manual` with
   the sentinel: the response is the state-only view with the prepended line,
   and ws-mcp's session-key store gained **no** second lead key for that
   process (count key files before/after).
3. Spawn a worker and an `explore` leaf: their system prompts carry only the
   rendered playbook, never the manual body or the guide.
4. Compaction: after a Pi compaction of the lead, the next turn's system
   prompt still carries the manual; a `workflow_manual` call restores
   agenda/todos.
5. Fill-or-forward regression: a lead call passing an explicit **child** key
   reaches ws-mcp unchanged (existing behavior preserved).
6. Static-body cut: the `playbook.print("lead-workflow-manual")` render is
   found verbatim inside the `workflow_manual` CONTINUE response (same
   `printPlaybook` render), so the mapped call returns only the dynamic
   part; force a per-call advisory (e.g. an unset tier) after session start
   and observe it in the next mapped response, then absent again after the
   condition is fixed.
7. Degraded path: with the `ferrule` bootstrap forced to fail, a sentinel
   `workflow_manual` call still mints and returns the manual (today's
   self-heal), and the bridge reported the degraded bootstrap.
8. Packaged install: `npm pack` → install the tarball → the system prompt
   carries the guide (the `files` whitelist ships `pi-lead-guide.md`).

## Non-goals

- Editing canonical skill text or adding Pi-specific skill shims.
- Putting agenda/todos in the system prompt.
- Any ws-mcp change (new modes, new tools).
- The fork spawn itself (`260904` side-thread) — this ticket only reserves
  the `fork` role value and the parent-key variable the fork will set.
