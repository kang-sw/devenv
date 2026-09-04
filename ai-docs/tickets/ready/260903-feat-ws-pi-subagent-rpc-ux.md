---
title: "Pi subagent interaction: persistent RPC children (send-message, transcript, resume)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor; lists RPC/re-entry primitives as post-MVP expansion surface
  260902-feat-ws-pi-native-mvp: ships the one-shot `-p` spawner this ticket upgrades
  260903-research-ws-pi-adapter-npm-distribution: gap #6 (pi CLI resolution for spawned children) is a shared prerequisite
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a65324df6bf96a7c
sage-review-completeness-reviewed: a65324df6bf96a7c
---

# Pi subagent interaction: persistent RPC children (send-message, transcript, resume)

## Background

The MVP spawner (`agents-plugin-pi/src/spawner.ts`, shipped by
`260902-feat-ws-pi-native-mvp`) dispatches every delegated agent as a **one-shot**
`pi -e <ext> -p "<prompt>" --no-session` subprocess with `stdin` ignored, parsing
streamed JSON events off stdout. This is the shape Pi's own subagent example
uses, and it is deliberately one-shot: the child runs one prompt and exits, so
the lead cannot send a follow-up into a running child, read a child's
transcript, or resume a child session by id.

The user wants Claude-CLI-level subagent ergonomics: **send additional messages
into a live child, and open a child's transcript.** The Pi API survey
(2026-09-03) confirmed Pi supports exactly this through **RPC mode** — the
capability is present in the installed build, so this is framework work, not a
Pi-core gap.

The golden rule holds: ws-mcp Go source is never modified; the dependency stays
one-directional (adapter -> ws-mcp).

## Feasibility (evidence, installed Pi build)

From `@earendil-works/pi-coding-agent` type defs (`dist/modes/rpc/`):

- **`RpcClient`** (package top-level export, `dist/modes/rpc/rpc-client.d.ts`):
  spawns a long-lived child (`start()`/`stop()`), and exposes
  - send-into-running-child: `prompt()`, `steer()` (interrupt mid-run),
    `followUp()` (queue after current run), `promptAndWait()`
  - transcript reads: `getMessages()`, `getEntries(since?)`,
    `getLastAssistantText()`, `getTree()`
  - lifecycle: `newSession()`, `switchSession(path)`, `fork(entryId)`,
    `clone()`, `compact()`, `abort()`, `setModel()`, `getSessionStats()`
  - `args?: string[]` construction option, so `--session <id>` can resume a
    specific session and then drive it live.
- **Wire protocol** alternative: `pi --mode rpc`, JSONL commands on stdin
  (`RpcCommand` union in `dist/modes/rpc/rpc-types.d.ts`: `prompt`/`steer`/
  `follow_up`/`get_messages`/`get_entries`/`get_tree`/`switch_session`/`fork`/
  `compact`/...).
- **Session resume** is a launch-time flag (`--session <path|id>`, accepts a
  partial id per `docs/sessions.md`); to keep driving the resumed session it
  must be launched in `--mode rpc` (a plain `-p` resume runs one prompt and
  exits — confirmed separately during the Phase 4 live-gate work).
- **In-process alternative:** the SDK `AgentSession` class
  (`dist/core/agent-session.d.ts`) offers `prompt()`/`sendUserMessage()`/
  `waitForIdle()`/`getContextUsage()` and its own event emitter, letting a child
  run inside the extension process with no subprocess — trades OS-process
  isolation for direct in-process message injection / transcript access.

## Resolved design (2026-09-03 discussion; D-A–D-D settled 2026-09-04)

Replace the one-shot `-p` spawner with **persistent `RpcClient` children** (mode
rpc, keeping the MVP's out-of-process context isolation). The spawn tool is a
**thin launcher**: the lead renders the playbook itself (it already does, per
prefer-subagent), so the tier/model recommendation is surfaced at render time and
the spawn tool carries no tier abstraction (**D-A, Shape A**):

| tool | backend | behavior |
| --- | --- | --- |
| `ws-agent-spawn({ system_prompt_path, prompt, model_name?, model_effort? })` -> `{agent_id}` | `RpcClient.start()`, `--append-system-prompt <system_prompt_path>` | persistent driveable child. `system_prompt_path` = lead-rendered playbook file; `prompt` = raw task text (→ first `prompt()`); `model_name`/`model_effort` optional overrides, omit → inherit parent model |
| `ws-agent-send(agent_id, message, interrupt?)` | `followUp()` (queue) / `steer()` (`interrupt`) | send into a live child; a dormant `agent_id` auto-resumes via `--session` then delivers (subsumes resume) |
| `ws-agent-wait(agent_ids[], timeout?)` | select over the set | FIRST child to reach idle OR emit a report; `reason: idle\|report` + child's last message auto-attached |
| `ws-agent-list()` | extension registry | live children, status, pending-report count |
| `ws-agent-stop(agent_id)` | `abort()` + `stop()` | halt the RpcClient process but RETAIN the D-C mapping + on-disk session file → child is **dormant/resumable**; `session_shutdown` is the terminal teardown |
| `ws-agent-transcript(agent_id)` -> `{transcript_path}` | Pi session JSONL path | advanced/rare; lead greps with fs tools — no content marshalling |
| `ws-report-to-lead(message)` (child-side) | RpcClient event stream | child->lead report; buffered per-agent, wakes a `ws-agent-wait` with `reason: report` |

**D-A — model resolution collapses to one source (Shape A).** `ws__playbook_render`
already returns `recommended-tier` + `recommended-model` + `recommended-reasoning-effort`
(config-resolved). The lead reads those and, at its discretion, passes
`model_name`/`model_effort` to spawn or omits them (inherit) — the override is the
model's call, the recommendation is only surfaced at render. The spawn tool no
longer takes `tier`; the tier abstraction lives only at render time. The adapter's
`model-catalog.json` is **retained but reframed** from a "tier→Pi model" map to a
**generic-name→Pi `provider/id` alias resolver**, so a `model_name` like `opus`
still resolves to a concrete Pi string (Pi model strings stay in the adapter data
file — golden rule). `prompt` (raw task) and `system_prompt_path` (rendered
playbook = the system prompt) are two distinct inputs.

**Retained MVP spawner machinery (NOT dropped by this rewrite).** The
persistent-RpcClient change is channel-only; these MVP surfaces survive except
where noted: per-spawn `--tools` group curation (`read-only`/`recon`/
`full-worker`), the `explore` recon leaf, the model-catalog data file (reframed
per D-A), and the unset-tier advisory — **its trigger re-keyed** from an unset
`tiers.small` to an **empty alias table**, since D-A removes the tier map (carried
verbatim it would test a key that no longer exists and silently never fire).
`ws-agent-continue` folds into `ws-agent-send`.

**D-B — depth policy: bounded depth ≤ 2 via a non-recursive explore leaf.** A
worker's `--tools` allowlist continues to EXCLUDE every driving/spawn tool
(`ws-agent-spawn`/`send`/`wait`/`list`/`stop`) so a worker cannot spawn or drive a
full generation — with ONE exception: `explore` is open to workers. `explore` is
read-only, ephemeral, self-reaping, and **cannot spawn `explore`** (its own
`--tools` excludes `explore`), so the tree terminates at depth 2 (lead → worker →
explore-leaf). The only child-side tool ADDED by this ticket is `ws-report-to-lead`.
This refines — for the Pi adapter only — the epic's "children cannot spawn → depth
strictly 1" principle (`260605` Cross-Child Decisions); the ws-mcp keyed-handler
role check is untouched (the adapter enforces depth via its own `--tools`
allowlist, not `ws.lead.*` roles).

**D-C — session_key lineage: keep across auto-resume.** The extension holds
`agent_id -> { pi session id, ws session_key }`; an auto-resume (`ws-agent-send`
to a dormant id) restores the SAME ws `session_key`, preserving parent→child
lineage and `session.children` enumeration. A resumed child never re-mints its
key. A child becomes **dormant** (resumable) when `ws-agent-stop` halts its
process while keeping this mapping and its session file; auto-dormanting a live
idle child (idle-reap) is deferred to Phase 2+, so in Phase 1 `ws-agent-stop` (or
a prior-session child) is the only dormant source.

**D-D — report buffer: drain-all FIFO, bounded.** On wake, `ws-agent-wait` drains
ALL pending reports for the woken agent in FIFO order (a waking lead sees the full
queue, not one at a time). The per-agent buffer is bounded (default: the most
recent 32 reports); on overflow the oldest report is dropped with a marker so the
lead knows truncation happened. Idle harvest is **edge/consume**: once
`ws-agent-wait` returns a child as idle, that idle is consumed, so a later wait on
an array still listing it does not busy-return it and later finishers are not
starved. On `timeout` expiry with no finisher or report, `ws-agent-wait` returns a
timed-out marker (no agent harvested) and leaves all children registered.

## Remaining open questions (post-2026-09-04)

D-A–D-D settled above (Shape-A thin-launcher spawn, depth ≤ 2 via non-recursive
`explore`, session_key kept across resume, drain-all-FIFO bounded reports). gap #6
(cliPath) is settled by the distribution spike — `process.argv[1]` yields the
correct installed CLI entry. Still open:

- **`RpcClient` vs in-process `AgentSession`.** Default is out-of-process
  `RpcClient` (isolation parity with the MVP); whether a lightweight in-process
  `AgentSession` variant is worth offering per-use-case is deferred.
- **Idle-timeout auto-reap.** Phase 1 commits only to `session_shutdown` teardown;
  reaping idle persistent children before shutdown is a Phase-2+ refinement.

## Spec Impact

- Target spec: `ai-docs/spec/pi-adapter-runtime.md`. The persistent-RpcClient
  rewrite touches most of the delegation surface:
  - Revised in place (same anchor IDs, behavior-changed prose per the
    spec-convention that anchors are authored once and evolve):
    `{#260903-pi-delegation-spawner-tools}` (Shape-A tool surface + persistent
    RpcClient), `{#260903-pi-spawner-completion-gating}` (RPC idle vs
    process-exit completion), `{#260903-pi-spawner-tool-groups}` (worker `--tools`
    now permits the non-recursive `explore` leaf),
    `{#260903-pi-spawner-model-tier-inherit}` (spawn drops `tier`;
    model_name/model_effort overrides; render surfaces the recommendation),
    `{#260903-pi-model-catalog-config-file}` (reframed to a generic-name→Pi-model
    alias resolver).
  - New `260904` anchors: the `ws-report-to-lead` child->lead channel + per-agent
    drain-all-FIFO bounded buffer, the path-only `ws-agent-transcript`, and the
    depth ≤ 2 explore-leaf policy.
  - `{#260903-pi-explore-recon-leaf}` and
    `{#260903-pi-model-catalog-unset-advisory}` survive with edits: the explore
    anchor gains the "spawnable by a worker, non-recursive" note, and the
    unset-advisory's trigger is re-keyed from an unset `tiers.small` to the alias
    table being empty (the `tiers.small` key is gone under D-A).
- Epic `260605` Cross-Child Decisions gains a bullet recording the Pi-adapter
  depth ≤ 2 refinement (adapter-local `--tools` enforcement, bounded by explore
  non-recursion) — distinct from the untouched ws-mcp `ws.lead.*` keyed-handler
  role check.
- The spec's closing Constraints note lists "durable depth-2 recursion" among
  deferred post-MVP surfaces; this ticket lands bounded depth ≤ 2, so that note is
  updated at proceed (the co-`ready` goal-loop ticket edits the same note for its
  own goal-loop/compaction item — no conflict).
- Expected caller-visible change: as in the tool table above.
- Contract-first: yes. Write/revise the `🚧` planned spec entries at proceed via
  `lead-write-spec`, removing the markers as each phase lands.

## Phases

### Phase 1: Persistent RpcClient children + parent-drive surface

Replace the one-shot `-p` spawner with persistent `RpcClient` children (mode rpc).
Implement `ws-agent-spawn({ system_prompt_path, prompt, model_name?, model_effort? })`
-> `{agent_id}` (launch with `--append-system-prompt <system_prompt_path>`;
`prompt` → first `prompt()`; `model_name` → `--model` (or inherit when omitted),
resolved through the reframed `model-catalog.json` alias table; `model_effort` →
Pi's launch-time reasoning-effort flag (confirm the flag exists at implementation;
if absent, apply via `setModel()` or treat as inert)), `ws-agent-send(agent_id, message, interrupt?)`
(`followUp`/`steer`; a dormant id auto-resumes via `--session`, keeping the SAME
ws `session_key` per D-C), `ws-agent-wait(agent_ids[], timeout?)` (select,
first-finisher + last message), `ws-agent-list()`, and `ws-agent-stop(agent_id)`.
Retain the per-spawn `--tools` group curation and the depth policy: a worker's
`--tools` excludes all driving/spawn tools except the non-recursive `explore` leaf
(D-B). Own the child lifecycle: teardown on `session_shutdown`. `ws-agent-continue`
folds into `ws-agent-send`.

Verification: a live `pi … --mode rpc` run showing a spawned child driven with a
follow-up, a dormant child auto-resumed by `ws-agent-send` on the SAME
`session_key`, `ws-agent-wait` over a two-child array returning the first finisher
with its last message, `ws-agent-list` reporting live children/status, and a
worker able to spawn an `explore` leaf but NOT another worker; children torn down
on shutdown. cliPath resolves via `process.argv[1]` (distribution gap #6).
Registry/select logic unit-tested where seam-extractable.

### Phase 2: child->lead report channel + path-only transcript

Add the child-side `ws-report-to-lead(message)` tool relayed over the parent's
`RpcClient` event stream into a per-agent bounded buffer; on wake `ws-agent-wait`
drains ALL pending reports for that agent in FIFO order (D-D) carrying
`reason: idle|report`, dropping the oldest with a marker on overflow. Add
`ws-agent-transcript(agent_id)` -> `{transcript_path}` (the Pi session JSONL path;
no content marshalling).

Verification: a live run showing a child `ws-report-to-lead` mid-run waking a lead
`ws-agent-wait` with `reason: report`, multiple queued reports draining FIFO in one
wake, a report buffering when no wait is pending, and `ws-agent-transcript`
returning a greppable session path. Depends on Phase 1.

## Non-goals

- Changing ws-mcp; the spawner upgrade is adapter-local.
