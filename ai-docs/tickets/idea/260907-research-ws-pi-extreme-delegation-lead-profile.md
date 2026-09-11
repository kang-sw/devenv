---
title: Redefine the Pi lead as a curated-tool, discuss-only decision seat that delegates everything else to an orchestrator child
related:
  260802-research-ws-pi-native-framework: direction anchor — this is the framework-layer concept turned all the way toward delegation, not a pivot away from it
  260903-epic-mcp-tool-surface-affordance-reduction: complementary, ws-mcp side — that epic shrinks the server surface for every host; this ticket curates what the Pi lead sees regardless of the server's size
  260906-research-ws-pi-recon-preset-agent-alias: the recon preset is the orchestrator's own explore surface under this design; settle its API with this ticket in view
  260907-feat-ws-pi-persistent-explore-deep-research: persistent explore is the corpus-search child this design delegates to instead of lead-side tool calls
  260906-feat-ws-pi-tool-and-push-tui-polish: lineage rendering (§5) coordinates with the dispatch-presentation work there
parent: 260911-epic-ws-pi-refound-resync-harness-peer
---

# Redefine the Pi lead as a curated-tool, discuss-only decision seat that delegates everything else to an orchestrator child

## Background

Two observations from Pi-track dogfooding (2026-09-07) motivate this.

**Cost is resident context times turns, not cache misses.** The day's sessions
(95 calls, ~$5.4 at Pi's reported rates) had a 90% cache-read hit rate; cache
reads were 64% of spend. Per-call context sat at a ~48k median, and two
lead-side tool results dominated the resident set: `project_tree` (~8.6k
tokens, fired at call 2 and re-billed on ~48 later turns) and one broad
`tickets_query` (~11.8k tokens, 98 uncapped matches). The first call's
`input` of ~24.7k tokens is the lead's fixed baseline: system prompt plus the
schemas of every registered tool. Two develop-side tickets already thin those
two results (the parent-nested `project_tree` render and the `tickets_query`
page cap); they are inefficiency removal and proceed regardless of this
ticket.

**The economics have flipped toward delegation.** A low-tier child that greps
and reads the whole ticket corpus costs on the order of $0.01; the same
exploration run by the lead at the lead's tier is on the order of $0.50, and
worse, whatever the lead pulls in stays resident and is re-billed every turn.
A child is therefore not only a cheaper executor but a *context-compression
boundary*: its scratch reasoning never enters the lead's context, and only its
distilled report does. Optimizing individual MCP tools has diminishing returns
against that; the leverage is in what the lead is allowed to see and do at all.

The owner's conclusion: leave the root ws-mcp as heavy, low-level workflow
management infrastructure (still removing inefficiency as found), and use the
Pi extension — the layer this repo fully controls — to redefine the lead's
concept rather than to mirror the full tool surface onto it.

## Current state (source survey, 2026-09-07)

Every item below is an existing seam; the design in the next section extends
them rather than replacing anything.

- **Lead tool exposure is total.** `bridge.ts` registers every ws-mcp tool
  under `ws__*` on the lead, filtering only `mercenary.*`
  (`filterOutMercenaryTools`); there is no lead-side allowlist.
- **Child tool curation already exists.** `spawner.ts` `TOOL_GROUPS` defines
  `read-only`, `read-only-explore`, `recon`, `full-worker`, and
  `execute-worker`; only `full-worker` receives the live `ws__*` names. The
  public `ws-agent-spawn` does not expose the group as a caller argument (the
  recon-preset research ticket records the same finding).
- **Depth is bounded by construction, not by a counter.** `full-worker`
  deliberately omits every `ws-agent-*` tool, so a worker can spawn an
  `explore` leaf but never another worker: lead → worker → explore-leaf,
  depth ≤ 2. `process-role.ts` carries a role (`worker` | `explore` | `fork`)
  and, for forks, the parent lead key, but no depth or lineage.
- **Rendering knows role, not lineage.** `agent-widget.ts` labels rows by
  `spawnRole`/thread binding; nothing distinguishes a child attached to the
  lead from one attached to another child.
- **Behavior lives in data.** Playbooks, skills, and tier tables are rsrc/config
  content rendered through `playbook.render`/`config.resolve_agent`; the
  extension's TypeScript is agent-infrastructure (bridge, spawner, sidecar,
  goal loop, gateways), which is why this reads as an extension of the
  codebase rather than a redirection of it.

## Direction (owner sketch, 2026-09-07)

1. **Lead tool profile: opt-out by default.** The lead session registers only
   the `do-i-really-have-to-*` pair and a genuinely minimal ws set; every other
   `ws__*` tool is withheld from the lead (still fully available to
   `full-worker` children). Candidate minimal set, to be validated by dogfood:

   | lead keeps | why |
   |---|---|
   | `do-i-really-have-to-read-this-myself`, `do-i-really-have-to-run-this-myself` | the deliberately ugly direct-access escape hatch |
   | `playbook.read`/`render`, `workflow.state`, `lead-workflow-manual` | orientation and the skill the lead is running |
   | `ws-agent-spawn`/`send`/`list`/`stop`/`transcript` | driving the orchestrator |
   | ask / human-relay surface | the escalation seam to the user |
   | `note.write`, `tickets.create_empty`/`move` | recording decisions |
   | lean `git.status`/`git.log` | situational awareness for decisions |

   Everything else (`tickets_query`, `specs_query`, `mental_models_*`,
   `references_trace`, `todo_*`, `review_*`, `config_*`, `git_diff`, …) is
   orchestrator/worker territory.

2. **The lead runs `discuss` itself and little else.** Design conversation with
   the user is the one activity that cannot be delegated without losing the
   decision-maker.

3. **`proceed` is redefined as a hand-off.** The lead decides which ticket and
   which phase runs next — and even that selection can be delegated to a child
   that returns a recommendation — then hands the ticket/phase to an
   **orchestrator** child. The orchestrator owns the rest of today's `proceed`
   pipeline (implement → review → docs → gate) and reports back at decision
   points. The lead's contribution during a phase is qualitative judgment on
   technical scenarios the orchestrator raises, not execution.

4. **Spawn gains an explicit recursion model.** The orchestrator role must hold
   `ws-agent-spawn` (today only the lead does). The orchestrator's children do
   not spawn further workers; they keep the existing blocking `explore` leaf.
   Depth therefore becomes lead → orchestrator → worker → explore-leaf (3),
   expressed as a role/depth marker in `process-role.ts` rather than by
   omitting tool names alone. An orchestrator fork is judged pointless at this
   stage and is out of scope.

5. **Lineage-aware rendering.** With recursion, the agent widget and push
   rendering should show whether an agent is attached directly to the lead or
   nested under an orchestrator (indentation, a parent tag, or a depth badge),
   so the user can read the delegation tree at a glance.

## Why this is an extension, not a pivot

The `260802` anchor already defines the Pi extension as the opinionated
framework layer composing skills, playbooks, tiers/subagents, and the goal
loop over a harness-neutral ws-mcp. This design turns that composition dial to
its end: the lead composes, children execute. The dependency stays
one-directional (Pi extension → ws-mcp); ws-mcp neither learns about lead
profiles nor changes contract. Every mechanism needed — a registration
allowlist in `bridge.ts`, a new `orchestrator` value in `TOOL_GROUPS`/
`SpawnRole`, a depth marker, a widget label — sits on a seam the extension
already owns. The behavioral content (what an orchestrator does, when it
reports) is playbook text.

## Open questions

- **Judgment boundary.** The lead must see enough to decide. What is the
  minimum resident input for ticket/phase selection: the lean `project_tree`
  tree, a child-produced recommendation, or both? The two develop-side diet
  tickets are prerequisites for the first option to be affordable.
- **Orchestrator report shape.** What must an orchestrator's report contain
  for the lead to make the next decision without re-reading anything: phase
  result, verification evidence, open questions, next-phase proposal? The
  existing `ws-report-to-lead` `kind:"final"`/`"question"` protocol is the
  starting point.
- **Escalation seam.** When a worker under the orchestrator hits ambiguity,
  the path is worker → orchestrator → lead → user. Whether the orchestrator
  may answer on its own within a stated envelope, and how `ws-ask`/the human
  relay gate surfaces a two-hop question, is unsettled.
- **Failure and blocking behavior.** An orchestrator that stalls, loops, or
  exhausts its goal loop must surface to the lead deterministically; the
  existing goal-loop race tickets (260906 reinject/reminder/push-wake) are the
  known hazards at one level of nesting and will compound at two.
- **Lead-side resumption.** If the lead session compacts or restarts while an
  orchestrator runs, the sidecar must recapture a nested tree, not a flat
  list.

## Confirmed direction - 2026-09-07

Owner follow-up settling two of the questions first drafted above.

- **The orchestrator prompt is adapter-owned, not ws inventory.** The
  orchestrator's text depends deeply on Pi-extension mechanics (agent
  messaging, `ws-report-to-lead`, spawn/send/stop lifecycle) that no other
  harness runs, so it does not belong in the shared `rsrc/` playbook
  inventory authored on `develop`, and it is not a `.pi.md` overlay of a
  shared playbook either. It lives in the adapter's own overlay layer: the
  package-root guide files the extension already ships and hands to children
  as `systemPromptPath` directly (`pi-lead-guide.md`,
  `execute-worker-guide.md`, `explore-guide.md`), bypassing
  `playbook.render`. An `orchestrator-guide.md` beside them is the
  established pattern, keeps the byte-identical `rsrc/` mirror untouched,
  and keeps the dependency direction intact. The earlier "harness-neutral
  role Codex could also run" reading is withdrawn.
- **The delegation unit is the ticket phase.** One `proceed` of one phase
  runs a very large number of cycles — playbook reads, workflow state,
  implement/review relays, verification — and every one of them is billed at
  the lead's tier and stays resident in the lead's context today. Against
  that, a child's ~24.7k baseline is noise; the granularity threshold
  question is moot at phase scale and is dropped. The saving targeted is
  the orchestration ceremony itself, moved to a medium-tier orchestrator,
  while the lead's per-phase involvement shrinks to qualitative judgment on
  the technical scenarios the orchestrator escalates. The hard decisions
  were made with the lead at discuss/ticket-writing time; the phase is,
  by construction, execution.

## Walkthrough: one phase, end to end (2026-09-07)

Traced against the existing seams (`lead-proceed` playbook →
`route.resolve_proceed` → `Next:`; `ws-report-to-lead` `progress` /
`question` / `final`; goal loop armed on `agent_settled`). Owner judged the
flow sound.

0. **Rest state.** Ticket written with the user in `discuss` and sitting in
   `ready/`. Lead context holds the system prompt, the curated tool schemas,
   and the conversation — nothing else resident.
1. **Entry.** User: `proceed <stem>`. The lead's `lead-proceed` thins to one
   judgment it cannot delegate — *is this genuinely an execution phase, or is
   a discussion still owed?* (`judge: discussion-needed`). The other
   routing judgments move to the orchestrator.
2. **Hand-off.** One `ws-agent-spawn`: role `orchestrator`, prompt
   `orchestrator-guide.md`, medium tier, task = stem + phase + a line or two
   of qualitative constraints from the lead. This call is effectively the
   lead's only active cost for the phase; the lead's turn ends and the goal
   loop arms.
3. **Inside the orchestrator (invisible to the lead).** The orchestrator
   runs the **existing lead playbooks unchanged** — `lead-proceed`,
   `lead-implement`, `lead-review` were written for "a lead that spawns
   workers", and from their point of view the orchestrator *is* the lead.
   `route.resolve_proceed` → `Next:` → `full-worker` spawn → implementer
   relay → verification → review worker → commit with `## Ticket Updates`
   → `tickets.move` all happen at medium tier inside the orchestrator's own
   context. `orchestrator-guide.md` therefore *wraps* the playbooks rather
   than rewriting them: "you run on behalf of an upper lead; escalate via
   `ws-report-to-lead question` within this envelope; close with a `final`
   of this shape."
4. **Round trips (0..N).** The lead wakes for exactly two things. A
   `question` — a worker's ambiguity goes worker → orchestrator; the
   orchestrator answers inside the ticket's decision envelope or re-raises
   to the lead with the same tool; the lead answers via `ws-agent-send` or
   lifts it to the user through ask. This is the "qualitative judgment on a
   technical scenario" contact point. `progress` reports render in the
   widget only and must not enter the lead's context, or the ceremony leaks
   back.
5. **Close.** One `final` report is pushed at settle: phase result,
   verification evidence (test counts, commit hashes), open items, and a
   next-phase proposal. The lead picks one of: hand off the next phase,
   `discuss` with the user, or `goal-achieved`. `### Result` / `## Ticket
   Updates` bookkeeping already happened in the orchestrator's commits; the
   lead does not touch it.

**Token picture.** Per phase the lead accumulates one spawn call, a few
hundred tokens per `question`, one ~1k `final`, and its own judgment text.
Today it accumulates playbook bodies, route verdicts, every relay round
trip, review output, and diffs — and keeps them resident for the rest of
the session. Moving the ceremony to medium tier is the first saving; that
the orchestrator's context *disappears* at settle instead of being
re-billed on every later lead turn is the second.

**Design requirements the trace exposed.**

1. **The orchestrator is "the lead" from the playbooks' perspective.** It
   must hold `ws-agent-spawn`; the guard that keeps `ws-agent-*` out of
   `full-worker` gets exactly one exception, an `orchestrator` group, and
   everything below it stays closed.
2. **Execute-gateway approval attribution.** A worker's shell command
   currently pauses for *lead* approval. Under nesting the approver becomes
   the orchestrator — the same class of approver (a model), but the
   approval surface the user could see moves one level down. Policy call:
   orchestrator approves within the envelope, or the gate escalates like a
   `question`.
3. **The `final` contract is the judgment boundary.** If the report is
   thin the lead ends up re-reading, and the saving collapses on the spot.
   The design's success rests less on the spawn mechanics than on the
   close-report contract in `orchestrator-guide.md`.

**Key risk (owner, 2026-09-07).** An orchestrator's urge to resolve
friction autonomously instead of raising a `question`. Owner's assessment:
the envelope is injected at the top of its prompt, so realistic violation
probability is low. Cheap structural backstops to pair with it: state the
envelope as a *do-not-answer-outside* rule, and make raising a question
feel cheap in the prompt (one question costs less than a failed phase),
since the autonomy urge is usually learned from "asking breaks the flow".

## Verification plan

- **Schema tax, first.** Register N curated tools on the lead and compare the
  first call's `input` token count against today's ~24.7k. One dogfood session
  measures it; this decides how much of the baseline the profile alone buys.
- **Lead context budget as a dogfood metric.** Reuse the per-call usage
  aggregation from the 2026-09-07 analysis (predicate: assistant messages with
  `totalTokens`) to report, per session, median/p90 per-call context, share of
  resident context that is lead-direct tool results, and cache-read share of
  cost. The profile succeeds when lead-direct tool results stop dominating.
- **Ceremony share: known, not measured.** The `proceed` loop has been the
  daily driver; owner's figure (2026-09-07) is on the order of **200k tokens
  accumulated in the lead per phase** — survey reads, then implement/review
  cycles. Essentially all of it is ceremony, and all of it is resident at
  lead tier for the rest of the session. Against the per-phase lead
  footprint sketched in the walkthrough (one spawn call, a few `question`
  round trips, one `final`, order of a few k tokens), the design moves
  roughly the whole 200k per phase to a medium-tier context that is
  discarded at settle. No further measurement gates the split into
  implementation tickets; the schema-tax and context-budget items above
  remain as before/after evidence once the profile exists.
- **Live scenario.** One ticket run end to end as discuss → hand-off →
  orchestrator → report → next decision, with the agent widget showing the
  nested tree, before any playbook text is generalized.

## Non-goals

- No change to ws-mcp tool contracts or to the root worktree's role as the
  heavy workflow-management infrastructure.
- No orchestrator fork, no worker-spawned workers.
- No replacement of the develop-side tool diets; they proceed independently.

## Confirmed direction - 2026-09-11 (refound subsumes the core)

Coordinated by `260911-epic-ws-pi-refound-resync-harness-peer`.

`epic/refound` (`260909-epic-ws-worker-interpreter-refoundation`, bound for
`develop`) makes "worker as workflow interpreter, lead as escalation handler"
the shared default, which absorbs the core of this ticket:

- The lead is already a dispatch/escalation seat by default via `lead-discuss` /
  `lead-delegate` / `lead-run`; the heavy per-phase ceremony (implement / review
  / verify / commit) now runs inside the discardable medium-tier `ticket-worker`,
  so it no longer stays resident in the lead. This captures most of the
  ~200k/phase resident-context saving this ticket targeted.
- The server tool surface is trimmed for every host (refound's 7-tool removal),
  partly addressing the schema-tax concern.

**Residual scope (what refound does NOT provide, kept here):**

- **Curated lead registration allowlist** (`bridge.ts`): hide tools from the Pi
  lead at the extension registration layer. Distinct from the server-side surface
  trim — it curates lead-vs-worker visibility. Urgency is lower now that the lead
  is a dispatch seat, but it is still Pi-extension-local and unprovided by
  refound.
- **Lineage-aware TUI rendering + agent picker** (pure Pi): unaffected by
  refound; coordinates with its own TUI tickets.
- **Adapter-owned mechanism guides** (execute-worker / orchestrator) describing
  `ws-execute` / `ws-worker-exec` / approval and `ws-report-to-lead`: pure-Pi text
  with no shared base, per the epic's text-home split.

**Deferred open question — the extra orchestrator level.** The original design
pushed the whole `proceed` pipeline (drain / select / dispatch loop included)
into a medium-tier orchestrator child so the lead holds nothing but
discuss + question/final. refound already discards the per-phase ceremony via
`ticket-worker`, so the marginal value of also moving the drain/dispatch loop off
the lead is unclear. Undecided; hold pending Pi dogfood signal on the lead's
remaining resident footprint under refound's default flow. This is the only
piece of the original delegation vision still open.

Withdrawn premise: this ticket's Non-goal "No change to ws-mcp tool contracts /
root worktree role" and its "delegation redefinition lives only in the Pi
extension" stance are overtaken — the redefinition is now the shared ws-mcp
default, consumed rather than re-built.
