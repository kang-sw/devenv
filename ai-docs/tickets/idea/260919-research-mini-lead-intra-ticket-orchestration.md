---
title: Intra-ticket sequential mini-lead orchestration for elevated ticket workers
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-research-ws-refoundation-evidence-audit: binding anchor for worker-interpreter / lead-surface / stop-conditions topics
  260605-research-ws-native-subagent-pivot: prior harness-infrastructure anchor (native-subagent, mailbox process-scope, ferrule auth)
  260611-research-ws-per-role-delegation-tuning-config: per-role delegation tuning (tier + prompt) surface this may consume
---

# Intra-ticket sequential mini-lead orchestration for elevated ticket workers

## Background

Observed dogfood failure: a ticket-worker told to do a single phase drifts as
its context fills and ends up implementing every remaining phase. The stop
judgment degrades under accumulated implementation context.

The owner's proposal: rather than slicing a ticket into per-phase worker
invocations that each drift, spawn one **elevated (large/xlarge) worker as a
mini-lead** that owns the whole ticket "in one breath" and delegates bounded
implementation slices to fresh-context implementer subagents. The expensive
frontier model (mini-lead) holds only the interface/spine, the hard
cross-cutting parts, and orchestration; the mechanical implementation grind
goes to cheap medium-or-below implementers (10x+ cheaper than large).

This ticket records the design converged in lead-discuss (2026-09-19) after the
parallel-acceleration framing was stress-tested and rejected in favor of a
sequential, context-hygiene-and-cost design.

## Design shape: sequential mini-lead orchestration

The mini-lead decomposes along the natural seam — **interface/contract (spine)
→ {test contracts, disjoint impl leaves}** — not along phases (which are
usually serial/dependent and less parallelizable). Once the contract is fixed,
tests and disjoint-file implementations are genuinely independent units.

Execution is **sequential**, not parallel. Sequential execution is not a
concession that loses value; it dissolves the two hard problems of a shared
worktree:

- Only one active leaf holds the single shared worktree at a time → **no
  concurrent write races** (native subagents share one process/working tree
  with no write serialization; seam files like module barrels, registries,
  manifests are where concurrent edits would silently collide).
- Between leaves the tree is always quiescent → **builds always run against a
  consistent snapshot.** A single **warm** shared worktree keeps incremental
  build state, so only the first build is cold and the rest are cheap
  incremental — the exact opposite of per-leaf worktrees (N cold builds,
  separate target dirs), which is why parallel isolation was rejected for
  build-heavy projects.

The active implementer runs its **own** build/test; its logs live in its own
fresh context, so the mini-lead's context stays clean without a separate
verify-leaf. The mini-lead may batch build/test points across a coherent group
of edits to amortize a heavy build, a scheduling win a single reactive worker
does not get.

Everything here uses **existing primitives** — workers already hold
lead-capability keys (`role: worker` → lead scope) and can `ferrule`, spawn
native subagents, `worktree.acquire`, and message children via native
SendMessage. No harness modification is required, so this can ship in ws proper
(downstream-first; Architecture Rule 4 clean).

## Playbook restructure

Today three ticket-worker playbook bodies (`ticket-worker` = medium,
`ticket-worker-elevated` = large, `ticket-worker-escalated` = xlarge) are
byte-identical, differing only in frontmatter `tier:`. That duplication is
retired and replaced with **two distinct prose bodies** that now differ in
behavior, not a number:

- default `ticket-worker` — implements directly.
- `elevated` — orchestrating mini-lead (spawns implementer leaves, coordinates,
  owns interface + hard parts).

xlarge is **not** a third body. The lead renders the `elevated` prose and
overrides the model to the xlarge tier via `config.resolve_agent`
(resolve-model). large and xlarge share the same orchestration behavior; only
the model differs. Both bodies keep `role: worker` (lead scope), so spawn
capability is identical — the difference is **prose (implement vs orchestrate),
not permission.** This realizes the epic's earlier "divergent body, not
permission" decision.

## Outcome Ledger

### Verified Findings
<!-- Evidence-backed observations from scoped exploration this session (2026-09-19); file paths are search anchors, exact lines to be reconfirmed at implementation. -->

- The three `ticket-worker*` playbooks under `agents-plugin/rsrc/` share
  byte-identical bodies and differ only in frontmatter `tier:`
  (medium/large/xlarge). Tier is purely a model-selection knob; it does not
  change the stop list, spawn rights, or review requirements.
- Workers are not server-side restricted from spawning further children:
  `playbook.render` maps `role: worker` → lead-equivalent key scope
  (`childRoleForPlaybookRole` in `agents-plugin-tool/internal/mcp/playbook_tools.go`),
  and lead scope permits every tool including `ferrule`
  (`roleAllowsTool` / `isLeadOnlyTool` in `.../mcp/server.go`). Depth-1 native
  recursion is owner-confirmed on Claude Code, pi, and Codex; deeper nesting is
  discouraged in prose, not blocked.
- The mini-lead pattern already exists once: `lead-goal-fan-out-step` mints a
  lead-capability child key via `ferrule(capability: "lead", ...)`; the child
  runs its own loop and is re-discovered via `session.children`. (That fan-out
  entry point is itself slated to retire per the epic and
  260730-refactor-retire-goal-fan-out-step-and-session-note.)
- Mailbox (`mailbox.send/recv/lookup_peers`) is **cross-process only** and
  explicitly out of scope for native subagents inside one session
  (`agents-plugin/rsrc/lead-use-mailbox/...`; identity is resolved once per MCP
  server process in `.../mcp/mailbox_runtime.go`). There is no
  sibling-to-sibling messaging primitive; `session.children`/`session.note` are
  parent-owned bookkeeping only.
- `config.resolve_agent(tier, harness)` is the read-only resolver
  (`wsconfig.ResolveAgentTierForHarness`) that turns a tier into a concrete
  `{backend, model, effort}`.

### Confirmed Decisions
<!-- User-confirmed in lead-discuss 2026-09-19. -->

1. **Sequential intra-ticket orchestration.** An elevated worker acts as a
   mini-lead owning the whole ticket and delegates bounded implementation
   slices to fresh-context implementer subagents, run sequentially.
2. **Active leaf self-verifies on a single warm worktree.** The active
   implementer runs its own build/test; only the first build is cold, the rest
   incremental. No separate verify-leaf. Build logs stay in the implementer's
   context, keeping the mini-lead's context clean.
3. **Framing: this is a context-hygiene + cost play, not a wall-clock play.**
   It fixes the phase-drift bug and routes the grind to cheap implementers while
   the expensive mini-lead holds interface/spine + hard parts + orchestration.
   Measure on the epic's qualitative cost-measurement axis (this repo has no
   wall-clock telemetry); sequential may even be slightly slower on wall-clock.
4. **Review timing is mini-lead discretion, keyed to leverage/irreversibility,
   not phase count.** Interface + test-contract review is a **named-condition
   recommendation, not a hard rail**: before leaves build on it, consider having
   the interface + test-contract prose independently reviewed *unless it is
   trivial or already covered by the ticket's design review*. The
   worker-stop-protocol escalation (unresolved Critical after round 2 → lead) is
   unchanged; the outer lead-review and ship gate are untouched — this tunes the
   inner loop only.
5. **Two playbook bodies, not three.** Collapse the byte-identical trio into
   default `ticket-worker` (implements directly) and `elevated` (orchestrating
   mini-lead). xlarge = `elevated` prose + xlarge model override via
   `config.resolve_agent`, not a third body. Both keep `role: worker`;
   difference is prose, not permission.

### Proposals
<!-- none: the design above was confirmed, not left as candidate. -->

### Open Questions
<!-- Implementation-time checks; resolve during the actionable child, not from evidence alone. -->

- Does the `playbook.render`/spawn path accept a **model override decoupled from
  frontmatter `tier:`**? If not, a small tooling addition is in scope — this
  feasibility gates the "xlarge = elevated + override" approach.
- `lead-run` dispatch table and escalation ladder: the stop-(e)
  medium→large→xlarge ladder must become "large → `elevated` + xlarge model
  override" rather than rendering a separate escalated body. Confirm the exact
  edit surface.
- Reference sweep for the removed `ticket-worker-escalated` body and any
  consumer keying off the now-stale frontmatter `tier:` — `lead-run` table,
  wsflow skill-shim drift tests, runtime-contract tests, manifests.
- **Reconcile current `lead-run` phase handling**: epic Decision 4 says the
  worker gets the whole ticket, but the observed drift bug is phase-scoped.
  Establish what actually happens today before designing the replacement.
- Final integration/full-test at ticket end: does the last leaf cover it, or
  does the mini-lead run a final verification pass?

### Rejected Alternatives

- **Parallel intra-ticket execution via per-leaf worktrees.** Rejected: in
  target projects the build is the heaviest cost; per-leaf worktrees force N
  cold builds and Amdahl caps speedup at the serial build barrier. Its real
  value (git-as-arbiter safety, isolated verification) is unnecessary once
  execution is sequential.
- **Peer-to-peer sibling messaging / autonomous scope negotiation.** Rejected:
  intra-ticket it solves a near-empty problem — disjoint scope needs no
  negotiation, overlapping scope needs repartition (a planning act) not chat,
  and any collision resolution collapses back to the coordinator anyway. The
  mini-lead is the natural hub via native SendMessage. No substrate exists
  (mailbox is cross-process only) and a harness-native intra-session channel
  could not be depended on by shipped ws text (Architecture Rule 4).
- **Separate verify-leaf.** Rejected as redundant: sequential execution leaves
  exactly one active leaf holding a quiescent tree, so it self-verifies.
- **Hard rail requiring spine review before fan-out.** Rejected in favor of a
  named-condition recommendation (see Confirmed Decision 4) to preserve worker
  discretion for the trivial/already-reviewed case.
- **Per-subagent write-path sandbox (harness hack) to enforce disjoint file
  ownership.** Deferred / out of scope: sequential execution removes the
  concurrent-write race, so the sandbox is at most an optional optimization
  behind a runtime capability probe, not core. It is the one legitimate
  harness-modification candidate if enforcement is later wanted.

## Constraints

Editing paths `agents-plugin/rsrc/`, `agents-plugin/skills/`, and
`agents-plugin-wsflow/` triggers the read obligations for
`ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`, and
`ai-docs/manuals/shipped-surface-boundary.md` before the edit. The shipped
surface is downstream-first (Architecture Rule 4): this design deliberately
uses only existing primitives so nothing here depends on host-specific behavior.
