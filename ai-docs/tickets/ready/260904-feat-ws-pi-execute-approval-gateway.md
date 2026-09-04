---
title: "Pi lead-execute approval gateway: delegated mutation via ws.execute + per-mutation ws.approve"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260903-feat-ws-pi-subagent-rpc-ux: prerequisite; provides the RpcClient spawn/send/wait/stop/report/explore machinery this gateway layers on
  260903-feat-ws-pi-goal-loop-compaction-hook: sibling; remedial whole-session compaction, the fallback for whatever lead-context noise this structural approach does not prevent
  260802-research-ws-pi-native-framework: research anchor (Pi RPC/re-entry primitives)
  260524-epic-async-exec-job-surface: orthogonal ws-mcp-native exec.* surface (output-hygiene axis); interaction policy deferred — see Decisions §9
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-design-reviewed: d1adbf443f708c3e
sage-review-completeness: completed
sage-review-completeness-reviewed: c97852f5cf200bc5
---

# Pi lead-execute approval gateway: delegated mutation via ws.execute + per-mutation ws.approve

## Background

Goal: keep the lead's context clean of raw tool-call noise (large command
output, file dumps, build/test logs) so its expensive, compounding context
stays high-signal for longer.

The chosen resolution is **structural, not remedial**. Rather than let the lead
run raw shell and then compact the noise afterward (a fork-then-summarize-splice
mechanic — rejected, see Decisions §1), the lead is given **no raw exec at
all**: every mutation flows through a delegated worker, and the lead's context
holds only compact approval requests plus the worker's final report — never raw
output. Raw output is firewalled into the (cheap-model) worker by construction.

This layers on the persistent-RpcClient subagent machinery from
`260903-feat-ws-pi-subagent-rpc-ux` (spawn/send/wait/stop/report/transcript/
explore, per-spawn `--tools`, depth ≤ 2). The golden rule holds: ws-mcp Go
source is never modified; this is adapter-owned Pi-extension work; the dependency
stays one-directional (adapter → ws-mcp).

## Decisions

### 1. Structural full-delegation over remedial fork-splice

Rejected: auto-firing a fork at turn-end to have the lead summarize its own
just-run tool noise and splice the summary over the raw transcript. Reasons:
(a) "fork" as a context-inheriting mechanism was deliberately **removed** on the
Claude harness (`260723-refactor-fork-removal-prefer-subagent`;
`mental-model/workflow-skills.md` records it is not a routing option) after forks
echoed the lead's delegation narrative instead of executing and inherited the
parent persona — the persona-bleed failure partially transfers to a
"summarize-your-own-work" fork — note this rejects the *auto-fork self-summarize
splice*, not context-inheriting delegates on Pi as such: the structurally
mitigated, lead-invoked side thread is a separate decision in
`260904-feat-ws-pi-side-thread-fork-question-surface`; (b) splice is **lossy on ephemeral bash output**
(a mis-summary of a one-time command result is unrecoverable); (c) it needs
message-tree surgery, more invasive than any compaction call. Delegation cost is
not a counter-argument: keeping the lead lean has **compounding** returns (every
future lead turn stays cheaper), so burning a light-model worker to absorb noise
is economically obvious.

### 2. Two delegation paths, differentiated by accountability

**Invariant (record verbatim in the spec):**

> The per-mutation lead-approval gate on `ws.execute` exists because
> `ws.execute` proxies actions the lead would otherwise perform directly under
> user consensus and extreme care; the gate preserves that lead↔user consensus
> across proxy execution — it is not distrust of subagents. General delegated
> workers carry no consensus-caliber actions and therefore need no approval
> gate.

| path | nature | gate |
| --- | --- | --- |
| `ws.execute` | proxy for consensus-caliber actions the lead would run itself (merge+push, version bump+tag, deploy) | **every mutation → lead approval** |
| `ws-agent-spawn` worker (260903) | scoped, sandboxed, reversible adaptive delegation (debug-and-fix) | `--tools` allowlist floor + escalation; no per-mutation gate |

Gate intensity matches whose accountability the action carries. This one
sentence preempts "why not add approval tooling to general subagents too."

### 3. `ws.execute` — the lead's delegated-mutation verb

```
ws.execute(command?, prompt, complex?: bool) -> report
```

- **Blocking** in the user-interaction sense only; it is **not** a synchronous
  tool call that suspends the lead's turn awaiting approvals (that deadlocks — a
  suspended lead cannot call `ws.approve`). Under the hood it is the §8 async
  shape: spawn the worker (fire-and-return), approvals arrive as interleaved
  injected lead turns (§4), and the report is delivered on completion via
  wait/injection. Serial by default; parallel mechanical fan-out is the async
  `ws-agent-spawn` path, not this.
- **Spawn seam (integration with 260903 Shape A):** `ws.execute` spawns the
  worker via 260903's `ws-agent-spawn`, supplying a **fixed adapter-owned
  execute-worker `system_prompt_path`** — the lead authors no prompt prose, so it
  is NOT a lead-rendered playbook — and maps `complex?` → a concrete `model_name`
  through 260903's `model-catalog.json` alias resolver (a light vs lead-class
  alias). The worker `--tools` is the §5 set.
- `prompt` (required): intent + what to report. The worker derives and runs the
  command(s) itself — the lead does **not** author command prose (that authoring
  is itself a context cost worth offloading).
- `command?` (optional verbatim anchor): when present, the adapter runs it
  **verbatim** first and hands `{command, output}` to the worker, which then does
  the follow-up per `prompt`. Verbatim matters for destructive exact-match cases
  (`git reset --hard <sha>`) where a model reconstructing the command could be
  catastrophic. Absorbs the earlier `ws-shell` idea as this optional case.
- `complex?` → **model tier** only (light model default; lead-class model when
  `complex`). Mid-task escalation allowed: a light worker that exceeds its
  capacity (e.g. a "simple" merge that hits conflicts) reports up so the lead can
  re-spawn at lead-class or take over. `complex` is an upfront hint, not a final
  verdict.
- The worker is registered with an `agent_id` (surfaced in approval requests and
  `ws-agent-list`); on completion the worker's report is delivered to the lead
  via the §8 wait/injection path (not a synchronous tool return).
- Rejected: a `risky` flag. Risk is not knowable upfront and, for adaptive work,
  cannot be pre-gated at all; safety is handled by universal approval (§4) +
  accountability routing (§2), not a per-call risk boolean.

### 4. `ws.approve` — per-command adjudication (+ abort factored out)

```
ws.approve(agent_id, cmd_id, decision: "approve"|"deny"|"run-instead", reason?, command?)
// reason  — required when decision = "deny"; optional otherwise
// command — required when decision = "run-instead"; rejected otherwise
```

- `deny(reason)`: reject THIS command; the worker re-plans and brings back a
  revised one; the task continues.
- `run-instead(command)`: the lead substitutes its own exact command (saves a
  deny→re-explain round-trip when the worker is subtly wrong). The substituted
  command's **output still routes to the worker** (hygiene preserved), and the
  worker is told "the lead ran X instead of your Y" so its plan stays coherent.
- `cmd_id` is a per-execution-request id the approval must name explicitly →
  **race-prevention**: the approval binds to exactly one pending request, so
  timing skew can never approve a previous command or pre-authorize a next one.
- `agent_id` disambiguates among all live **and dormant/retained** agents
  (260903 retains stopped agents) and matches the decision to the exact
  requesting worker — this is addressing correctness, **not** a commitment to
  concurrent executes (concurrency is structurally allowed since each execute is
  its own agent, but is not a required feature; serial-blocking is the default).
- **abort is NOT an `ws.approve` decision** — it reuses `ws-agent-stop(agent_id)`
  (260903). Rationale: abort must work when there is **no pending command to
  approve** (worker mid-plan), which an approve-enum value cannot reach. Factoring:
  `ws.approve` = per-command adjudication; `ws-agent-stop` = lifecycle
  termination. On abort: (a) unblock the waiting `ws.execute` with an "aborted"
  result; (b) `dormant+retain` (per 260903) so an accidental abort is inspectable
  via `ws-agent-transcript` — terminal teardown stays a separate `session_shutdown`.
- **Approver is the spawning parent** — the top lead for its own workers; for a
  worker spawned by a side-thread fork
  (`260904-feat-ws-pi-side-thread-fork-question-surface`) the approver is that
  fork, which holds the same consensus context. No separate human-approval
  primitive: genuinely destructive/outward actions are handled by the lead's
  **existing** discipline of confirming hard-to-reverse or outward-facing actions
  with the user before proceeding at its approval turn — an emergent property of
  the lead's standing behavior, not a new gate tier. Scope of this gate = **subagent mutations only**; the lead has
  no direct-mutation path (bash removed, §6), so "accidents the lead causes"
  reduce to lead mis-approval — accepted at the same trust level the lead already
  operates under.

### 5. Allowlist as tool-shape, not command-string matching

The worker reads freely (read/grep/find output absorbed in the worker); **any
mutation elevates** to lead approval — maximally conservative. But do NOT
classify by parsing command strings: `git status && rm -rf x` (compound),
`find . -exec rm` (a "read" that mutates), `cat > f` / `sed -i` (redirection /
in-place) defeat a leading-binary allowlist and start a smuggling arms race.

Instead: give the worker **structured, mutation-incapable read tools**
(read/grep/glob — cannot write by construction, the Claude Read/Grep/Glob
pattern) and route ALL free-form shell through the **adapter gated-exec** tool
that always elevates. "Anything that can write is gated" holds by construction,
no parser.

```
worker --tools = [native read/grep/glob] + [adapter gated-exec] + [ws-report-to-lead] + [explore]  −  [native bash]
```

The gate is enforceable precisely because the worker's exec is an **adapter**
tool (a native shell could not be interposed). Reads stay native (safe).

### 6. Lead tool surface

- **Remove** native bash from the lead (hard) — no direct exec/mutation path.
- **Add** `ws.execute` + `ws.approve` (≈ +1 net after bash removal; each is
  load-bearing). Their "when to use" rows go into the Pi lead guide that
  `260904-feat-ws-pi-lead-bootstrap-system-prompt` injects into the lead's
  system prompt (that ticket lands first in the drain order); this ticket
  does not author standalone lead guidance elsewhere. Keeping approval an explicit tool (not prose-parsed) is
  consistent with the goal-loop's zero-prose-parsing decision
  (`260903-feat-ws-pi-goal-loop-compaction-hook`).
- **Keep read as a soft-discouraged escape hatch** — an "ugly-named" direct read
  tool (e.g. `do-i-really-have-to-read-this-myself()`) so the lead reaches for
  delegation by default but retains a legitimate must-look path. Naming-as-
  deterrent is a soft signal, appropriate for read; exec gets hard removal
  because it needs reliability.
- Considered and rejected: reusing `ws-agent-send` to carry approvals (0 new
  tools). Approval is a **targeted request-reply** addressed by `(agent_id,
  cmd_id)` that unblocks a specific pending exec; `send` is a fire-and-forget
  message. Overloading `send` muddies its contract — a dedicated `ws.approve` is
  semantically correct.

### 7. Approval-request payload: adapter-authoritative working-context header

Each approval request the adapter surfaces to the lead carries a **compact**
context header so the lead can judge safety without a second tool call and
without a context bomb:

```
{ agent_id, cmd_id, command, rationale,            # rationale = worker's one-line "why"
  context: { cwd, worktree_root, branch, ahead_behind?, dirty } }
```

- `rationale` (worker one-line "why") is **required** — without it the lead
  rubber-stamps blind, knowing intent but not the worker's discovered state.
- The `context` block is **adapter-scraped ground truth at exec time**, not
  worker-reported (defends against worker self-report drift/deception). Feasible
  because the adapter runs the command; and because shell state is ephemeral
  between tool calls (AGENTS.md invariant), the effective `cwd` is either the
  exec tool's explicit param or a single `cd x && cmd` call — always known to the
  adapter. Worktree awareness matters especially here (this repo is
  worktree-heavy; a push/merge from the wrong worktree is a real hazard).
- Excluded as context bombs: full `git status`, diffs, env dumps. If the lead
  needs more it can `deny` to ask, or an optional on-demand expand can be offered.
- Command-type-adaptive enrichment (git→ahead/behind, network→target host)
  deferred; a fixed compact header is the baseline.

### 8. Feasibility & the new primitive

The per-command approval is a **two-way blocking handshake** (worker requests,
pauses; lead approves; worker resumes) — a **new primitive** beyond 260903's
one-way async `ws-report-to-lead`. Honest correction to earlier "no new
primitive" framing.

- **Baseline = prompt-injection relay, no special harness support:** worker
  emits an approval-request event → adapter injects a prompt to the lead → lead
  calls `ws.approve` → adapter relays the decision back to the worker. Builds on
  the 260903 channel + a lead→worker approval relay. Spec this as the contract.
- **Optimization = harness-native pause/resume** (if Pi exposes pausing an
  in-flight worker and injecting a decision via steer/followUp) — Phase 2.
- Mid-plan `abort` needs Pi to interrupt an in-flight worker generation
  (`RpcClient.abort`) — already assumed by 260903's `ws-agent-stop`, so no new
  feasibility risk there.
- **The linchpin — lead tool-surface reshaping (§6) — is not yet grounded.** The
  "structural, by-construction" guarantee depends on the adapter removing native
  bash from (and ugly-renaming native read in) the **host lead session** — a
  user-launched interactive `pi -e <ext>` process the extension is *loaded into*,
  NOT a worker the adapter spawns with `--tools`. Whether a Pi extension can
  restrict its host session's native tool surface is **unproven** and must be
  verified at Phase 1 (§8 grounds the approval handshake this carefully; this
  claim needs the same). Declared fallback if the capability is **absent**: the
  gateway degrades to a **system-prompt-enforced soft convention** — a strong
  delegation posture plus the ugly-read naming applied to all native exec — where
  the approval gate still governs everything routed through `ws.execute` but
  cannot forcibly prevent the lead reaching for native bash. This is the weaker
  "convention, not construction" mode §1 rejects for fork-splice, so hard removal
  is preferred; but the ticket does not assume it, and Phase 1 must report which
  mode was achieved. Grounding lead (2026-09-04): Pi 0.84.4's extension API
  declares `pi.getActiveTools(): string[]` and `pi.setActiveTools(toolNames)`
  (`dist/core/extensions/types.d.ts`), i.e. an extension *can* reshape its
  host session's active tool set — the likely mechanism for hard removal;
  whether it covers built-in bash and survives `/reload` is the Phase 1 check.

### 9. Relationship to the ws-mcp exec.* surface (260524) — orthogonal, policy deferred

`260524-epic-async-exec-job-surface` builds a **ws-mcp-native** `exec.*` surface
on a different axis: the core (`exec.spawn`/`exec.shell`/`exec.status`/
`exec.result`/`exec.abort` + `exec.raw.*` fallback readers) is **already landed**
and keeps large raw output out of the lead context via bounded results; the
pending `260524-feat-exec-output-ask` adds `exec.ask(exec_key, question)`
model-backed output compaction. That is an **output-hygiene primitive**; this
ticket is a **delegation + approval posture**. The two are orthogonal and are not
merged.

Because the adapter bridges ws-mcp tools onto Pi, the Pi lead already has
`exec.*` available. Reconciling the two is **deferred** — a forward-note, not a
decision here — and revisited when `exec.ask` lands. Two questions get
(re)decided then:

- (a) whether the Pi lead regains a direct exec path via `exec.*`/`exec.ask`
  (relaxing this ticket's bash removal), and
- (b) whether `ws.execute`'s worker is handed `exec.*` for its own execution —
  expected to be a pure agent-context-hygiene win, **with the caveat** that
  routing the worker's *mutations* through ungated `exec.*` would bypass this
  ticket's approval gate, so any such reuse must preserve the gate (likely
  `exec.*` for the worker's reads only).

`260524` carries the reciprocal "decide the `ws.execute` interaction at
`exec.ask` landing" note.

## Constraints

- Golden rule: ws-mcp Go source frozen; gateway is adapter-owned Pi-extension
  code; dependency one-directional (adapter → ws-mcp).
- Inherits 260903's depth ≤ 2 and non-recursive `explore` leaf; the gateway adds
  no new spawn depth.
- Shell state is ephemeral between tool calls (AGENTS.md) — the adapter, not the
  worker, is the source of truth for `cwd`/branch in §7.

## Prior Art

- `260903-feat-ws-pi-subagent-rpc-ux` — reuse its spawn/send/wait/stop/report/
  transcript/explore surface and `--tools` curation wholesale; this ticket adds
  only `ws.execute`, `ws.approve`, the adapter gated-exec tool, the mutation-
  incapable read tools, and the lead tool-surface change.
- Claude Read/Grep/Glob — the mutation-incapable structured-read pattern (§5).
- `260723-refactor-fork-removal-prefer-subagent` — cautionary precedent for
  "fork" (§1).

## Spec Impact

Target spec: `ai-docs/spec/pi-adapter-runtime.md`. New `260904` anchors authored
at proceed (contract-first, via `lead-write-spec`): the `ws.execute` /
`ws.approve` gateway contract and its approval vocabulary; the adapter gated-exec
tool + mutation-incapable read-family (§5); the adapter-authoritative approval
payload / working-context header (§7); the lead tool-surface change (bash
removed, ugly-read retained, §6); the two-path accountability invariant (§2). The
approval-handshake baseline (prompt-injection relay) is the documented contract;
pause/resume is an optimization. Spec anchors are authored contract-first at
proceed (first implementation slice), against the `spec:` stem above.

## Phases

### Phase 1: End-to-end approval gateway via the fallback relay

Deliver the whole gateway working through the no-special-harness-support path.
Implement the adapter gated-exec worker tool (every free-form command elevates)
and the mutation-incapable read-family on the worker `--tools`; `ws.execute(command?,
prompt, complex?)` spawning a worker (reusing 260903 spawn) whose exec elevates,
`complex?`→tier; the prompt-injection approval relay + `ws.approve(agent_id,
cmd_id, decision, reason?, command?)` with `approve`/`deny(reason)`/`run-instead(command)`;
the adapter-authoritative approval payload with the §7 context header + required
worker rationale; `cmd_id` race-binding; abort via `ws-agent-stop` (unblock
execute with "aborted", dormant+retain); lead `--tools` change (bash removed,
`ws.execute`/`ws.approve` added, ugly-named read retained). `command?` runs
verbatim first then hands `{command, output}` to the worker.

Verification: a live `pi … --mode rpc` run of a `ws.execute` task where the
worker reads freely (no gate), a mutating command elevates to a lead
`ws.approve`, `deny(reason)` returns a revised command, `run-instead` substitutes
with output routed to the worker, the approval request carries the adapter-scraped
`{cwd, worktree_root, branch, dirty}` + rationale, `cmd_id` binding rejects a
stale approval, `ws-agent-stop` aborts mid-plan and unblocks `execute`, and the
lead's native exec surface is reshaped — **reporting whether Phase 1 achieved
hard removal of the lead's native bash (structural) or the soft-convention
fallback (§8)**. Race/registry/select logic unit-tested where seam-extractable.
Depends on 260903 Phase 1.

### Phase 2: Harness-native pause/resume + escalation refinements

If Pi exposes pausing an in-flight worker and injecting the decision via
steer/followUp, replace the prompt-injection relay with the native pause/resume
path (lower latency, cleaner turn structure). Add mid-task `complex` escalation
(light worker → lead re-spawn/takeover on capacity overflow) and the optional
on-demand context-expand for an approval request.

Verification: a live run showing an in-flight worker paused at a mutation and
resumed by a lead `ws.approve` without a separate injected lead turn; a light
worker escalating a task that exceeded its tier; an approval request expanded on
demand. Depends on Phase 1.

## Non-goals

- Changing ws-mcp; the gateway is adapter-local.
- Remedial whole-session compaction (owned by
  `260903-feat-ws-pi-goal-loop-compaction-hook`); this ticket prevents noise
  structurally rather than compacting it after the fact.
- Concurrent multi-`execute` orchestration (structurally allowed, not a built
  feature; serial-blocking is the default).
