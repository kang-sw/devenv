---
title: "Pi lead side-thread: context-inheriting fork (`ws.fork`) + owner-facing question surface (`ws.ask` / `/answer` overlay chat)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260903-feat-ws-pi-subagent-rpc-ux: prerequisite — RpcClient spawn/send/wait/stop/report machinery, dormant+retain, depth ≤ 2 explore leaf, model-catalog alias resolver this ticket layers on
  260904-feat-ws-pi-execute-approval-gateway: sibling — accountability invariant and `ws.approve`; this ticket adds "approver = spawning parent" routing (edited there in the same commit) and narrows its §1 fork wording
  260903-feat-ws-pi-goal-loop-compaction-hook: sibling — remedial whole-session compaction, fallback for lead noise this structural approach does not prevent
  260802-research-ws-pi-native-framework: research anchor (Pi RPC / re-entry primitives; `inherit_context` catalogued as raw capability, no decision)
  260723-refactor-fork-removal-prefer-subagent: cautionary precedent — the Claude-host fork deletion whose recorded failure modes this design must answer structurally, not by prose
  260625-research-fork-posture-leak-system-guarantee: failure-mode evidence (posture leak; 0 tool calls echoing deferral narrative)
  260629-research-fork-worker-persona-bleed: failure-mode evidence (lead-voice narration; template-correct prompt insufficient; tier-lock)
  260626-bug-prefer-subagent-recursive-delegate-escape: dropped precedent — the recursive-escape class §3 closes structurally via the fork allowlist
  260626-feat-session-key-format-and-retention: retention philosophy the §9 dormant-thread policy follows
related-mental-model:
  - workflow-skills
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-design-reviewed: c3cf97dd3b2030ba
sage-review-completeness: completed
sage-review-completeness-reviewed: c04a2e0f41765757
---

# Pi lead side-thread: context-inheriting fork (`ws.fork`) + owner-facing question surface (`ws.ask` / `/answer` overlay chat)

## Background

Once `260904-feat-ws-pi-execute-approval-gateway` removes raw execution from
the Pi lead, the heaviest remaining load on the lead context is
**conversation-as-input work**: running the `lead-write-ticket` ceremony
(Populate → Verify → Ground → Sage → Commit), sage review handling, and
decision synthesis — work that *must* be done with the lead's accumulated
context (the central authoring whitelist — the `lead-prefer-subagent` skill
rule that durable-artifact authoring stays with the session holding the
authoritative context — exists precisely because a fresh spawn working from a
summary loses the reasoning a correct write depends on), yet
whose *output* the lead needs is only verdict prose (ticket path, sage verdict,
commit hash, unresolved questions). Doing it inline pollutes the lead;
delegating it fresh is lossy. Heavy reading that has a clear question (read this
code and judge X) is *not* in this class — `ws.execute(complex)` or a fresh
reviewer handles it equally well and is immune to the failure modes below.

A second, independent friction resolves into the same shape. Agent→owner
questions are overwhelmingly the start of a *discussion*, not one-shot answers.
Blocking interview tools stall the lead; routing a delegate's question through
the lead is a four-hop loop (fork → lead → owner → lead → fork) that breaks the
discussion rhythm.

Both collapse into one primitive — the **side thread**: a context-inheriting
fork of the lead session, an owner-facing overlay chat attached to it, and a
summary injected back into the lead session when the thread ends. A side thread
has two entry points (lead-spawned task thread; owner-opened discussion thread
on a lead-registered question) and two exits (the fork's own final report; the
owner closing the discussion).

**Why this is allowed to exist despite `260723`.** The Claude-host fork
(`spawn_agent(fork_context:true)`) was deleted after its failure modes proved
unfixable at prose level: posture leak (0 tool calls, echoing the lead's
"delegate and wait" narrative), persona bleed (lead-voice narration instead of
execution, on Opus 4.8; Sonnet held), acknowledge-and-return, a detection gap
(failed forks looked successful → false-complete), and tier lock. Every
escalation of prompt strength — trailing line, all-caps, `system-reminder`
override, then retreat to natural handoff, then *removing* identity framing
because "you are a forked subagent" made it worse (`a56870a1`) — failed. That
history establishes one thing this ticket takes as a hard constraint: **bleed
is not mitigated by prompt text.** It is mitigated only where Pi hands the
adapter structural levers Claude never exposed: the fork is a copy-on-fork
session file the adapter owns, the child's tool allowlist is the adapter's,
and the adapter drives the child's turn loop and sees every turn boundary.

## Decisions

### 1. One primitive, two entry points, two exits

The adapter keeps a **thread registry**; one record per thread:

```text
thread := { thread_id, title, question?, context?, entry_id, respondent?: agent_id, status }
```

- `entry_id` — the lead-session entry at which the question/task was raised;
  recorded mechanically by the adapter, never authored by the model.
- `respondent` — the live fork attached to this thread, if any.

**Entry A — lead-spawned task thread.**

```text
ws.fork(prompt, model_name?, expects_commit?) -> { agent_id }
```

The lead delegates conversation-as-input work (canonically: the
`lead-write-ticket` ceremony from Populate onward). Exit: the fork's final
`ws-report-to-lead(kind: "final", ...)`, delivered through the unchanged
`260903` report/wait path. `expects_commit` tells the adapter the task is
commit-bearing so the §4 completion check can be mechanical. **Serial by
default**: the fork works in the same worktree as the lead, so for a
commit-bearing thread the lead parks on `ws-agent-wait` rather than mutating
the tree concurrently (mirrors `260904` §3's posture); an isolated worktree per
fork is a non-goal here.

**Entry B — owner-opened discussion thread on a lead question.**

```text
ws.ask(title, question, context) -> { question_id }     # registers only; NO spawn
ws.resolve(question_id)                                  # lead self-resolves a pending question
/answer <id>                                             # owner opens it → lazy fork at the lead's tip
```

`ws.ask` registers a pending question and the lead continues working. When the
owner opens it, the adapter spawns a discussion fork **at the lead's tip at open
time** (not at ask time) so the discussion has current knowledge, and attaches
the overlay chat. Exit: the owner's `/done` inside the overlay → the fork writes
a discussion summary → the adapter injects it into the lead session (§6).

**Fork-raised question (Entry A meets Entry B).** A task fork that hits a
decision it must not settle alone calls
`ws-report-to-lead(kind: "question", title, question, context)` and ends its
turn. The adapter registers it in the same registry with
`respondent = that fork`; `/answer` attaches the overlay to the *live* fork (no
new spawn). The lead is not involved (the four-hop relay is exactly what this
removes); it learns the outcome through the fork's final report (§4
`Decisions:`).

Unified path: a static `ws.ask` question also spawns a fork on open. There is
deliberately **no fork-less quick-answer path**; a one-line answer is the owner
typing one line and `/done`. Rationale: discussions dominate, and one path keeps
the surface small.

### 2. Fork mechanics (Pi facts, 0.84.4)

- Pi forking is **copy-on-fork**: `createBranchedSession` spread-copies the
  active-branch entries into a new `.jsonl` (fresh session id,
  `parentSession` back-pointer). The forked agent sees a byte-identical
  snapshot; neither side sees the other's later entries. We own the copy.
- Primary path: **subprocess** via the `260903` spawner —
  `pi --fork <lead session file|id> --mode rpc --tools <§3 allowlist> --append-system-prompt <thread directive>`,
  driven by the same `RpcClient` machinery (`ws-agent-spawn` gains a
  `fork_from` seam). `--fork <path|id>` exists as a CLI flag; its composition
  with `--mode rpc` / `--tools` / `--append-system-prompt`, and whether it
  clones *at the leaf* (vs. before a message), is Phase 1 verification.
- Spawn point: the lead's tip at spawn time; if the lead is mid-turn, the last
  completed turn boundary.
- Model: inherit the lead's model by default (peer caliber is the point);
  `model_name` override resolves through the `260903` catalog alias table.
- System prompt: **append only**. Replacement is rejected by the user; prefix
  cache is explicitly irrelevant to this design.
- **ws session_key: the fork gets its own lead-scope key, never the lead's.**
  Every Pi process's bridge already mints its own key at start (`ferrule`,
  `bridge.ts` default-fill) and forwards a key the model passes explicitly. A
  fork inherits a transcript that may name the lead's key, and ws-mcp keys
  agenda/todos per session_key (file-backed), so a fork calling with the
  lead's key would clobber the lead's todo list (e.g. `lead-write-ticket`'s
  checklist appends). Decision (2026-09-04): the fork's bridge mints via
  `ferrule(root, capability: lead, parent_session_key: <lead key>)` so
  lineage is kept, and the bridge **rewrites** an explicitly passed
  session_key equal to the parent lead's key to the fork's own key — a
  mechanical bridge-layer substitution, not a prompt instruction. The parent
  key reaches the fork process out-of-band (spawn env/flag), never via prose.
  The rewrite itself is one case of the bridge normalization layer owned by
  `260904-feat-ws-pi-lead-bootstrap-system-prompt` (§2 there; role marker
  `WS_PI_SPAWN_ROLE=fork` + `WS_PI_PARENT_SESSION_KEY`); this ticket sets the
  marker at fork spawn and adds its `ws.fork` / `ws.ask` / `ws.resolve` rows
  to that ticket's Pi lead guide.
- Rejected paths: in-process `createAgentSession` from a tool `execute()` (no
  shipped Pi example does this; reentrancy with the host `ExtensionRunner`
  unverified) and Pi's `ctx.fork()` (it *replaces the host's own session* —
  the opposite of a side channel).

### 3. Fork tool surface: lead surface minus the thread tools

```text
fork --tools = <lead's exact tool surface> − [ws.fork, ws.ask, ws.resolve] + [ws-report-to-lead]
```

- The fork is "the lead in context": the `260904` accountability invariant
  applies to it unchanged, so it inherits the lead's surface (including
  `ws.execute`/`ws.approve` when those land). "Exact surface" is mechanically
  enumerable: the adapter reads `pi.getActiveTools()` on the lead at spawn
  time (this also de-risks `260904` §8's lead-surface reshaping, since
  `pi.setActiveTools()` exists).
- **Not** stripped of spawn tools: the ticket ceremony needs fresh spawns
  (fact-population, sage reviewers). Only side-thread recursion is blocked,
  structurally, by removing `ws.fork` from the fork's allowlist (this closes
  the recursive-escape class of
  `260626-bug-prefer-subagent-recursive-delegate-escape` — dropped there
  because its prose-level fix was too broad — without a prompt).
- `ws.ask` / `ws.resolve` are removed too: a fork's **only** question path is
  `ws-report-to-lead(kind: "question")` (§1). A fork-issued `ws.ask` would
  register a respondent-less thread whose `/answer` spawns a second fork while
  the asker idles, and it would pass the §4 turn-end check without a report.
- **Depth: the fork is lateral.** A fork is a peer copy of the lead, not a
  worker, so it does not consume depth budget; `260903`'s depth ≤ 2 bound is
  measured from the fork as root (fork → worker → explore leaf). Termination
  is unchanged (`explore` is non-recursive, `ws.fork` is stripped). The
  `260903` depth anchor and the `260605` epic's "tree terminates at depth 2"
  bullet must say so — listed in `## Spec Impact`.
- **Approval routing = spawning parent.** When a fork's `ws.execute` worker
  requests approval, the request routes to the fork, not the top lead
  (`260904` §4 amended in this commit).

### 4. Anti-bleed mechanical loop — task threads only

Applies to Entry A forks. Every mitigation is a tool-surface or adapter-loop
mechanism; none is prompt text.

- **Turn-end check.** A fork turn that ends with **no tool call at all** is
  treated as acknowledge-and-return: the adapter auto-nudges ("continue") up
  to 2 times, then **fails loud to the lead** with the transcript tail.
- **Idle-without-final is non-completion.** A fork that reaches idle (the
  `260903` `reason: idle` harvest) without having emitted `kind: "final"` is
  *not* harvested as a result; the adapter reports it to the lead as
  incomplete with the transcript tail. Narration never reaches the lead as a
  result by either route.
- **Disambiguation is the structured call**: `kind: "question"` then end =
  legitimate question (registered, §1); `kind: "final"` = completion; neither
  = failure. One mechanism separates the cases `260723` could not tell apart.
- **Required report shape** for `kind: "final"`:
  `Outcome / Files changed / Verification / Blockers / Commit / Decisions`.
  `Commit:` is always present, with a literal `none` when nothing was
  committed. `Decisions:` lists everything the owner agreed with the fork
  inside the overlay (including added scope) — the lead did not see that chat
  and would otherwise start its next turn on a stale plan. When the thread was
  spawned with `expects_commit`, `Commit: none` is flagged by the adapter as
  non-completion; the lead confirms a reported hash through the bridged ws
  `git.log` tool (the lead has no native shell under `260904`). This closes
  `260723`'s false-complete detection gap mechanically.
- **Directive style**: a short natural-language task with execution
  constraints only; **no identity framing** ("you are a forked subagent" is
  the wording `a56870a1` found to backfire), no XML/all-caps overrides
  (`56a6b04c`).
- **Discussion threads (Entry B) run no loop.** There the fork *is meant* to
  speak as the lead — persona continuity is the feature — and the owner is
  present in real time.

#### Re-decision (2026-09-05) — structural initial-message frame

The **Directive-style / "no identity framing"** decision above holds for the
*system-prompt directive* but was found insufficient in live Pi dogfooding: a
task fork whose inherited context contained a lead-orchestration script role-bled
— it re-ran the lead's plan and reported "unable to start the requested fork"
instead of doing its own task. Root cause (audited): a fork is pushed toward
lead-identity by **two** inherited signals — the cloned `--fork` conversation AND
the `pi-lead-guide.md` block the `before_agent_start` hook appends to a fork's
own system prompt (`isLeadOrFork` treats fork == lead) — against only a soft
"work laterally alongside the lead" line.

The `260723` finding that "bleed is not mitigated by prompt text" was established
on the **Claude host (Opus 4.8 / Sonnet)**; it does not transfer wholesale to
Pi. The rejected prompt-strength ladder was re-run empirically on Pi's actual
models (owner-approved):

| variant | luna (weak) | astra (top frontier) | wording |
| --- | --- | --- | --- |
| natural (prior shipped) | role-bled | — | none |
| strong-header (all-caps identity) | fixed | — | aggressive |
| **framed (structural)** | **fixed** | **fixed** | **calm** |

**Adopted:** the fork's **initial user message** is now a structural frame
(`buildForkInitialMessage`) that demotes the inherited conversation to
reference-only and fences the task as an explicit "message from the lead" — no
all-caps/identity override. The system-prompt directive stays framing-free (the
original decision), so this is an *additive message-level* mechanism, not a
reversal of the directive-style rule. `framed` is chosen over `strong-header`
because it stops the bleed on both weak and top-frontier models with the calmest
wording. This complements, and does not replace, the §4 anti-bleed mechanical
loop.

### 5. Owner-facing surface (TUI lead)

- Overlay chat via `ctx.ui.custom({ overlay: true })` rendering a pi-tui
  component: child `text_delta` events (`RpcClient.onEvent`) → `Markdown.setText`;
  owner input → `RpcClient.prompt()` when the fork is waiting after a
  question, `steer()` when it is running.
- Closing the overlay has **no effect on the fork**; the owner can reattach any
  time while it is alive (the doom-overlay example's reopen-onto-live-state
  pattern). Additional owner instructions inside the overlay are allowed and
  surface to the lead only through `Decisions:` (§4).
- `setWidget(aboveEditor)` shows `N pending`; `/answer <id>` opens one;
  `/thread` lists all threads (pending questions and running forks); a
  shortcut reopens the most recent; `/done` inside the overlay ends a
  discussion thread.
- **Never auto-pop** the overlay; notify + widget only. The overlay header
  labels thread title and spawn time — two lead-voiced agents are a real UX
  confusion risk, and the main lead must not pretend to know thread contents
  before injection.
- **Child-side `ctx.ui.*` dialogs are not used.** The stock `RpcClient` has no
  public path to answer a child's `extension_ui_request`; questions are
  modelled as ordinary turns ending after the structured call (§4), which the
  stock client supports.
- Captured-`ctx` staleness: the adapter re-captures `ctx` on every
  `session_start` and stores only plain data in the registry (Pi invalidates
  captured contexts after new-session/fork/reload).
- **One overlay at a time.** Opening `/answer` while another thread's overlay
  is open closes the current overlay first (its fork is unaffected, §5) and
  attaches the new one.
- **Registry is persisted**, not in-memory only: thread records live in the
  adapter's per-lead-session state file next to the `260903` D-C
  (session_key lineage decision) `agent_id → session` mapping (which must already persist for dormant
  resume), so pending `ws.ask` questions and dormant threads survive a lead
  restart.

### 6. Injection back into the lead session

- Discussion thread `/done`: the fork writes the summary; the adapter injects
  `context + original question + summary` into the lead session as a **Pi
  custom message** (distinguishable from a plain user turn), delivered when
  the lead is idle (`followUp`, never `steer`), queued in order when several
  threads close. It carries **owner authority**: the owner was present.
- Task thread final report: unchanged `260903` path (`ws-report-to-lead` →
  `ws-agent-wait`). The overlay may *display* reports; it never routes them.
- The lead session is never rewound. Forward injection only.

### 7. The `context` field and `entry_id` anchoring

- `context` is **lead-authored, 2–3 sentences** ("what we were doing, why this
  matters now, what is blocked"), bounded with an adapter length warning. Three
  readers need it: the owner deciding whether to open the thread (title too
  thin, question too verbose), the fork starting fast, and the lead re-reading
  the injected answer after a compaction. It also forms the head of the §6
  injection payload.
- `context` carries **no paths or hashes**. Exact question-time context is the
  adapter's job: the thread's `entry_id` addresses the lead-session entry, and
  when that entry has fallen behind a compaction boundary the adapter inserts
  a verbatim excerpt around it into the fork's first message. Pi session files
  are append-only, so the entry survives compaction.
- Rejected: a model-authored "time capsule" (file paths, commit hashes,
  content-hash staleness checks) — replaced by mechanical `entry_id`
  anchoring. Rejected: rewinding the lead to the question point.

### 8. Headless fallback

When the lead runs in `--mode rpc`, `ctx.ui.custom()` returns `undefined`.

- **Fork-raised questions**: **relay through the lead** — a fork's
  `kind: "question"` wakes `ws-agent-wait` with the question, the lead asks
  the owner, and answers with `ws-agent-send`.
- **Entry B (`ws.ask`)**: the lead is itself the asker, so there is nobody to
  relay to. Headless `ws.ask` registers the question and surfaces it to the
  RPC host through the fire-and-forget `ctx.ui.notify` path (which works in
  rpc mode as an `extension_ui_request`); the owner's answer arrives as an
  ordinary lead turn. **No discussion fork is spawned headless** — the lazy
  fork and overlay are TUI-only.

The overlay is the TUI optimization over these baselines — the same shape
`260904` uses for its approval handshake.

### 9. Lifecycle

- **Lazy spawn**: an unopened question costs nothing.
- **Closed threads are dormant + retained** (`260903` `ws-agent-stop`
  semantics) and reopenable. A retained task fork **owns the authored
  artifact's context** — follow-up edits to a ticket it wrote route to it via
  `ws-agent-send`, per the central authoring whitelist in `lead-prefer-subagent`
  ("the delegated subagent's own continuing session when settled there").
- **Thread rebase is a non-goal**: a fork's knowledge is its spawn-time
  snapshot; a long discussion that needs newer lead state closes and reopens
  as a new thread. Retention policy follows the session-key retention
  philosophy (`260626-feat-session-key-format-and-retention`).

### 10. Cut line with `lead-write-ticket`

Route + Consent Gate (Open Decision Queue) stay **lead-inline** — the owner
decides with the lead in the main conversation. Populate → Verify → Ground →
Sage Gate → Commit → Handoff run in the **task fork**. A gap surfaced inside the
fork (a fact-population finding that needs an owner decision) becomes a
`kind: "question"` thread — the fork never resolves a decision gap from a
delegate's evidence alone, exactly as the playbook already requires.

## Constraints

- Golden rule: `agents-plugin-tool/` (ws-mcp Go) is untouched; everything here
  lives in `agents-plugin-pi/`.
- System prompt append only; no replacement.
- No prose-only bleed mitigation; if a Phase 1 measurement suggests one, it is
  not an acceptable fix — report it as a residual.
- No child-side `ctx.ui.*` dialogs; no overlay auto-pop; no lead rewind.
- `ScrollView` is exported and documented with a usage example in the
  `pi-tui` package README (`follow: "end"` for chat-style tailing), but no
  `pi-coding-agent` bundled extension example uses it: verify inside an
  overlay or fall back to `Container` + tail truncation for the transcript
  pane.
- Side threads are for conversation-as-input work (§Background); they are not
  a general replacement for inline reading.
- Agent-driven tmux probes run on an isolated server socket
  (`tmux -L ws-probe-<pid> ...`) with a unique session name, and never
  `kill-session`/`kill-server` on the default socket. Incident (2026-09-04):
  a probe subagent's first `tmux` call on the default socket started a server,
  tmux-continuum auto-restored a stale snapshot as session `0`, and the probe
  then killed that "leftover" session — and with it the server. Harmless that
  time; fatal against the owner's real default server.

## Prior Art

- `260903` spawner (Shape A: `system_prompt_path` + `model_name`/`model_effort`),
  `ws-report-to-lead` drain-all FIFO, dormant+retain — the fork is a spawn with
  a `fork_from` seam.
- Pi bundled examples: `doom-overlay` (reopenable overlay onto still-running
  state), `questionnaire.ts` (custom component from a tool `execute()`),
  `overlay-test.ts`, `rpc-extension-ui.ts` (stand-alone RPC chat client
  consuming `text_delta`), `event-bus.ts` (captured-`ctx` pattern),
  `subagent/` (subprocess spawn with `--tools` / `--append-system-prompt`),
  `handoff.ts` (summarize → new session; the non-fork analog).
- `lead-write-ticket` playbook — the canonical task-thread payload and the §10
  cut line.

## Spec Impact

Spec anchors are authored contract-first at proceed (first implementation
slice), against the `spec:` stem above.

- `pi-adapter-runtime` spec: new lead tools `ws.fork` / `ws.ask` / `ws.resolve`;
  `ws-report-to-lead` gains `kind: question | final` and the required report
  shape (`Commit:` always present, `Decisions:`); thread registry (persisted),
  overlay chat, widget, `/answer` / `/thread` commands, injection contract
  (custom message, `followUp`, ordered), headless baselines (§8);
  `ws.approve` approver = spawning parent (amends the `260904` anchor when it
  lands).
- `260903` depth anchor and the `260605` epic Cross-Child bullet ("tree
  terminates at depth 2 (lead → worker → explore-leaf)"): restate the bound as
  measured from the fork as root — a fork is lateral and does not consume
  depth budget (§3).
- `mental-model/workflow-skills.md` `{#260505-workflow-primitive-reference}` and
  `spec/workflow-skills.md` (fresh-spawn posture anchor) state that the
  context-inheriting fork was removed and "no delegate inherits the lead's
  conversation". That remains true for the Claude/Codex posture; the Pi side
  thread is a **deliberate, Pi-scoped, structurally mitigated exception** and
  both documents need that exception recorded at doc closeout.

## Phases

### Phase 1: Task-thread fork with relay baseline (`ws.fork` + anti-bleed loop)

Depends on `260903` Phase 1 (persistent RpcClient spawner) having landed.

Implement `ws.fork(prompt, model_name?, expects_commit?)` as a
`ws-agent-spawn` variant with a `fork_from` seam (`pi --fork <lead session>
--mode rpc`), tools = lead surface (from `pi.getActiveTools()`) −
`ws.fork`/`ws.ask`/`ws.resolve` + `ws-report-to-lead`, appended thread
directive (natural language, execution constraints only). Implement the §4
loop (turn-end check, ≤2 nudges, fail-loud; idle-without-final = incomplete),
`ws-report-to-lead(kind: question | final)` with the required report shape
(`Commit:` always present, `Decisions:`), the `expects_commit` completion
check, the §8 relay baseline for fork-raised `kind: question` (headless
compatible; this is the *only* question path in Phase 1), and approval routing
to the spawning parent.

Verification (report which mode was achieved, as `260904` Phase 1 does):

1. Flag composition: `pi --fork <file> --mode rpc --tools ... --append-system-prompt ...`
   produces an at-leaf clone whose first entries are byte-identical to the
   lead's active branch; document `--fork`'s position semantics.
2. **Bleed PoC** on a real lead session with the lead's current model: ≥3
   forks running `lead-write-ticket` Populate → Commit on a scratch ticket.
   Record acknowledge-and-return rate with the loop disabled and enabled, the
   fail-loud path firing at least once (force it), and that no narration
   reached the lead as a result. Prior Claude data is Opus 4.8-specific; there
   is no data for the current lead model — this measurement is the ticket's
   go/no-go for Phase 2.
3. Depth: the fork spawns a fact-population child and a sage reviewer (depth 2)
   and its allowlist lacks `ws.fork` (recursion attempt fails at the tool
   layer, not by refusal prose).
4. Completion checks enforced: a thread spawned with `expects_commit` whose
   final report says `Commit: none` is surfaced as incomplete; a fork that
   goes idle without `kind: "final"` is surfaced as incomplete with the
   transcript tail, never as a result.
5. Session-key isolation: a fork that explicitly passes the lead's key has it
   rewritten to its own key at the bridge (assert the lead's todo/agenda are
   untouched after a fork ran `lead-write-ticket`), and ws-mcp accepts the
   `capability: lead` + `parent_session_key` mint so `session.children` on
   the lead lists the fork.

### Result (ecaa86c8) - 2026-09-05

Landed the `ws-fork` task-thread mechanism and the anti-bleed completion loop
(offline-implementable surface; the two live-only verification items are
deferred, see the Blocked note below).

- **Spawn seam** (`agents-plugin-pi/src/spawner.ts`): `RpcSpawnCtx`/
  `RpcAgentRecord` gained `forkFrom`/`explicitTools`/`parentSessionKey`;
  `buildRpcClientOptions` emits `--fork <leadSession>` (role marker `"fork"`,
  `WS_PI_PARENT_SESSION_KEY` when both present) on the initial spawn vs
  `--session` on resume; `spawnAgent` overwrites `record.sessionPath` from
  `getState().sessionFile` post-`start()` and fails loud if absent. Dormant
  resume never re-passes `--fork` (uses the discovered `--session` path,
  tools rebuilt from cached `explicitTools`). `ws-report-to-lead` gained an
  optional `kind: "question" | "final"`, changing `pendingReports`/
  `WaitForAgentsResult.reports` to `Array<{message, kind?}>` (additive;
  existing worker/execute-worker callers unaffected).
- **New `agents-plugin-pi/src/fork.ts`**: `FORK_TOOL_NAME`,
  `computeForkToolSurface` (lead surface − fork verbs + `ws-report-to-lead`;
  Phase 1 excludes only `ws-fork`), the role-differentiated `addForkToolIfLead`
  (adds `ws-fork` for `role === undefined` only — the fix that also blocks
  fork recursion at the tool layer and keeps `ws-fork` off a fork's own
  surface), the anti-bleed pure predicates (`shouldNudge`,
  `classifyForkTurnOutcome`, `isIdleWithoutFinal`, `validateFinalReportShape`,
  `checkExpectsCommitCompletion`), and `registerFork`'s IO glue wiring a second
  `RpcClient.onEvent()` listener. `index.ts` wires `registerFork` after the
  execute gateway and applies `addForkToolIfLead` as a separate
  role-differentiated `setActiveTools` step. `pi-lead-guide.md` gained a
  `ws-fork` verb row. Approval-routing-to-the-spawning-parent needed no new
  code — it is emergent from the per-process session-start registration.
- **Verification**: `cd agents-plugin-pi && npm test` → 340/340 pass. Offline
  coverage: tool-surface arithmetic incl. the role-differentiation fix, all
  anti-bleed predicates, report-shape + `expects_commit` checks, and the
  `--fork`/`--session` arg branch. Golden rule held (no `agents-plugin-tool/`
  or `agents-plugin/skills/` change).
- **Review**: partitioned (correctness=large, test). Review #1 found 1 Critical
  + 2 Important; all `[fixed]` in relay #1 (`ecaa86c8`) — Critical (nudge was
  mis-targeted to the lead session; now delivered to the fork via
  `record.client`) verified `[resolved]` by a Critical-scoped review #2
  (clean). Importants: `isIdleWithoutFinal` now enforced (idle-without-final
  surfaced to the lead as incomplete, never harvested); the spawn directive's
  identity-framing opener removed and negative assertions added. 4 Minors
  recorded, not fixed (substring-test discrimination; listener-attach ordering;
  resumed-fork role marker on dormant resume — a Phase 2 concern; orphaned
  registry entry on the `getState` fail-loud throw — consistent with
  pre-existing non-fork behavior).
- **Deviations**: none in scope. `isIdleWithoutFinal`'s `"acknowledge-and-return"`
  case (a tool call was made but no `kind:"final"`) is not auto-nudged — a tool
  call is treated as real progress — but is surfaced to the lead as an
  incomplete-run advisory, so §4 is enforced directly with no silent residual.
- **Spec**: `ai-docs/spec/pi-adapter-runtime.md`
  `{#260905-pi-side-thread-fork-task-thread}` (commit `af4a8683`).

### Phase 2: Owner question surface (`ws.ask`, registry, overlay chat, lazy discussion fork, injection)

Depends on Phase 1.

Implement the thread registry (§1 record), `ws.ask` / `ws.resolve`, the
`aboveEditor` pending widget, `/answer <id>` / `/thread` / reopen shortcut, the
overlay chat component (§5), lazy discussion fork at the lead's tip with
`entry_id` anchoring and post-compaction excerpt insertion (§7), attach-to-live
task fork for `kind: question` threads, `/done` → summary → custom-message
injection on lead idle (§6), and the `ctx` re-capture discipline. The §8 relay
baseline must keep working when `ctx.mode !== "tui"`.

Verification:

1. Agent-driven TUI loop, two tiers (probe result, 2026-09-04): (a) **unit**
   — instantiate the overlay/widget `Component` in vitest and call
   `render(width)` (pure, no TTY) across widths, asserting line count,
   `visibleWidth(line) <= width`, and stripped-ANSI substrings; drive
   `handleInput(data)` with synthetic keys and assert state / `done()`;
   (b) **integration** — a `tui-probe.sh {start|keys|shot|stop}` helper over
   tmux on an **isolated server socket** (`tmux -L ws-probe-<pid>`; never the
   default server), launching `pi --offline --no-session -e <ext>` (no model
   spend), `send-keys -- <str>` with settle sleeps (~1s shell init, ~3s pi
   start), `capture-pane -p [-e] [-S -N]` for assertions, `/quit` to exit.
   Pi's TUI captured legibly under tmux (borders, footer, model id, the
   `extended-keys-format csi-u` warning). No headless `TUI` harness ships in
   the package; `@xterm/headless` + `node-pty` is the fallback only if
   cell/style-level assertions become necessary. Human judgment remains
   needed for visual polish, IME/CJK candidate placement, and cross-emulator
   quirks. If a scenario cannot be driven agent-side (TTY-only behavior,
   flaky capture), the implementer does **not** stop the loop: package that
   scenario for one-shot human execution (the
   `260903-feat-human-relay-interactive-gate` shape — exact commands, expected
   screen, pass/fail criteria) and hand it over at closeout. Using that loop: open `/answer` on a pending `ws.ask`, observe a
   lazy fork spawn at the current tip, exchange two turns, `/done`, and assert
   the injected custom message appears in the lead session with
   `context + question + summary` and is delivered only after the lead is idle.
2. Fork-raised question: a Phase 1 task fork calls `kind: question`; the
   widget count increments without any lead turn; `/answer` attaches to the
   live fork (no new process); the fork resumes after the owner's reply; the
   final report's `Decisions:` reflects the in-overlay agreement.
3. Post-compaction anchoring: compact the lead past a pending question's
   `entry_id`, open it, and confirm the fork's first message carries the
   verbatim excerpt.
4. Headless (`--mode rpc`): a fork-raised question falls back to the lead
   relay; a lead `ws.ask` registers, emits a `notify` `extension_ui_request`
   to the RPC host, spawns no fork, and the owner's reply lands as an
   ordinary lead turn.
5. Restart survival: pending `ws.ask` questions and a dormant thread are still
   listed by `/thread` after the lead process restarts; opening a second
   `/answer` while one overlay is open swaps overlays without disturbing
   either fork.
6. Overlay hygiene: never auto-pops; close/reopen leaves the fork running;
   header shows title + spawn time.

## Non-goals

- Thread rebase onto a newer lead tip; in-process `createAgentSession` forks;
  system-prompt replacement; child-side UI dialogs; a fork-less quick-answer
  path; overlay auto-pop; using side threads for heavy reading with a clear
  question (that is `ws.execute(complex)` / fresh reviewers); any change to
  ws-mcp Go.

## Blocked (2026-09-05)

Phase 1 landed (see its `### Result`). Phase 2 is not autonomously advanceable
in the current build environment:

- The ticket makes the **bleed proof-of-concept** (Phase 1 verification item 2)
  the explicit **go/no-go for Phase 2** — whether the structural anti-bleed
  mitigation actually works must be measured on a real lead session before the
  owner-question surface is built on top of forks. That measurement needs a live
  `pi --mode rpc` run with provider credentials, absent from the sandbox.
- Phase 2's surface (overlay chat `Component`, `/answer`/`/thread` shortcuts,
  the `aboveEditor` widget, lazy discussion fork at the lead tip) and its
  verification are TUI-and-live dependent: the two-tier agent-driven TUI loop
  needs a tmux probe on an isolated socket plus a live `pi` process.

Unblock when a live `pi` environment with provider credentials is available:
run the Phase 1 live gate (`--fork` composition + bleed PoC), and if the PoC
clears its go/no-go, proceed to Phase 2. Until then the selector should skip
this ticket.

### Live gate cleared (2026-09-05) — bleed PoC go/no-go = GO

The Phase 1 live gate was run on `pi 0.84.4` with the `openai-codex`
subscription provider (user-scope installed adapter). Confirmed end-to-end:
`--fork` copy-on-fork inherits the lead's full context; the fork's surface
carries `ws-report-to-lead` and excludes `ws-fork`; the fork emits a
`kind:"final"` report in the required shape, harvested by the lead via
`ws-agent-wait`; the anti-bleed nudge lands in the fork's own session (not the
lead's) with no lead-context pollution. **The bleed PoC clears its go/no-go
(GO):** the structural loop is sufficient to drive the fork to a report, so
Phase 2 may build the owner-question surface on top of forks.

Operational precondition surfaced by the run: spawned children (workers and
forks alike) load the adapter extension **only when it is user-scope installed**
(`pi install <path>`) — RPC children re-run the Pi CLI via `process.argv[1]`
without `-e`, and Pi does not auto-discover a project `package.json`'s
`pi.extensions`, so an ad-hoc `-e` lead run leaves children without the report
channel. Documented in `pi-adapter-runtime.md`
(`260905-pi-side-thread-fork-task-thread` Live-verification note).

**Remaining Phase 2 blocker is now narrowed to the TUI overlay only:** the
owner-question overlay `Component`, `/answer`/`/thread` shortcuts, and
`aboveEditor` widget need a live *interactive* TUI (tmux probe on an isolated
socket) — not exercised by the `--print` non-interactive path used for the gate.
The fork-mechanism prerequisite is no longer blocking.

### Unblocked (2026-09-05) — Phase 2 is advanceable

Both original gate conditions are now met, and the narrowed TUI-only blocker
above is met as well:

- **Bleed PoC go/no-go = GO** (Live gate above), reinforced by the §4
  re-decision: the residual role-bleed seen under an inherited lead-orchestration
  script is fixed by the structural initial-message frame (`50e685be`),
  live-verified on both a weak model (gpt-5.6-luna) and a top-frontier model
  (astra).
- **Live interactive TUI is available**: the owner has been driving the
  user-scope-installed adapter in interactive `pi` sessions (not `--print`), so
  the overlay `Component`, `/answer`/`/thread` shortcuts, and `aboveEditor`
  widget can be built and exercised against a real TUI.

The selector should **no longer skip** this ticket. Next action: dispatch Phase
2 (`ws.ask`/`ws.resolve`, thread registry, `aboveEditor` widget, `/answer`/
`/thread`/reopen, overlay chat, lazy discussion fork at the lead tip with
`entry_id` anchoring, `/done` → summary → injection). Preconditions the
implementer must honor: the adapter must stay user-scope installed
(`pi install <abs path>`) for spawned children to load `ws-report-to-lead`; the
structural frame applies to **Entry A task threads only** — Entry B discussion
forks are meant to speak as the lead (§4 "Discussion threads run no loop"), so
do not wrap their initial message with it. Human-judgment verification items
(visual polish, IME/CJK candidate placement, cross-emulator quirks) are to be
packaged as a one-shot owner runbook at closeout, per Phase 2 verification
item 1.
