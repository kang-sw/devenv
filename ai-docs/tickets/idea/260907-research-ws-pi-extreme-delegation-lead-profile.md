---
title: Redefine the Pi lead as a curated-tool, discuss-only decision seat that delegates everything else to an orchestrator child
related:
  260802-research-ws-pi-native-framework: direction anchor — this is the framework-layer concept turned all the way toward delegation, not a pivot away from it
  260903-epic-mcp-tool-surface-affordance-reduction: complementary, ws-mcp side — that epic shrinks the server surface for every host; this ticket curates what the Pi lead sees regardless of the server's size
  260906-research-ws-pi-recon-preset-agent-alias: the recon preset is the orchestrator's own explore surface under this design; settle its API with this ticket in view
  260907-feat-ws-pi-persistent-explore-deep-research: persistent explore is the corpus-search child this design delegates to instead of lead-side tool calls
  260906-feat-ws-pi-tool-and-push-tui-polish: lineage rendering (§5) coordinates with the dispatch-presentation work there
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
   points.

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
- **Delegation granularity vs spawn baseline.** Each child pays its own
  ~24.7k-token system-prompt-plus-schema baseline (measured on the lead; a
  `full-worker` child registering all `ws__*` tools pays a similar one).
  Micro-delegation of trivial lookups loses money. Where is the threshold,
  and should the orchestrator batch small tasks? A child-tier baseline
  measurement is needed; this session had no small-tier cost data.
- **Orchestrator report shape.** What must an orchestrator's report contain
  for the lead to make the next decision without re-reading anything: phase
  result, verification evidence, open questions, next-phase proposal? The
  existing `ws-report-to-lead` `kind:"final"`/`"question"` protocol is the
  starting point.
- **Escalation seam.** When a worker under the orchestrator hits ambiguity,
  the path is worker → orchestrator → lead → user. Whether the orchestrator
  may answer on its own within a stated envelope, and how `ws-ask`/the human
  relay gate surfaces a two-hop question, is unsettled.
- **Where the orchestrator playbook lives.** Shared playbook text is authored
  on `develop` (Pi-track clause (1)) and mirrored; if the orchestrator
  playbook is Pi-only, it is a `.pi.md` overlay authored under
  `agents-plugin/rsrc/` per the mirror-drift ticket's decision, never a
  diverging mirror. Whether the concept is Pi-only at all, or a harness-neutral
  role that Codex could also run, is itself a question.
- **Failure and blocking behavior.** An orchestrator that stalls, loops, or
  exhausts its goal loop must surface to the lead deterministically; the
  existing goal-loop race tickets (260906 reinject/reminder/push-wake) are the
  known hazards at one level of nesting and will compound at two.
- **Lead-side resumption.** If the lead session compacts or restarts while an
  orchestrator runs, the sidecar must recapture a nested tree, not a flat
  list.

## Verification plan

- **Schema tax, first.** Register N curated tools on the lead and compare the
  first call's `input` token count against today's ~24.7k. One dogfood session
  measures it; this decides how much of the baseline the profile alone buys.
- **Lead context budget as a dogfood metric.** Reuse the per-call usage
  aggregation from the 2026-09-07 analysis (predicate: assistant messages with
  `totalTokens`) to report, per session, median/p90 per-call context, share of
  resident context that is lead-direct tool results, and cache-read share of
  cost. The profile succeeds when lead-direct tool results stop dominating.
- **Child baseline.** Measure a `full-worker` and a recon child's first-call
  `input` to set the delegation-granularity threshold.
- **Live scenario.** One ticket run end to end as discuss → hand-off →
  orchestrator → report → next decision, with the agent widget showing the
  nested tree, before any playbook text is generalized.

## Non-goals

- No change to ws-mcp tool contracts or to the root worktree's role as the
  heavy workflow-management infrastructure.
- No orchestrator fork, no worker-spawned workers.
- No replacement of the develop-side tool diets; they proceed independently.
