---
title: Intra-ticket sequential mini-lead orchestration for elevated ticket workers
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-research-ws-refoundation-evidence-audit: binding anchor for worker-interpreter / lead-surface / stop-conditions topics
  260605-research-ws-native-subagent-pivot: prior harness-infrastructure anchor (native-subagent, mailbox process-scope, ferrule auth)
  260611-research-ws-per-role-delegation-tuning-config: per-role delegation tuning (tier + prompt) surface this may consume
  260919-feat-ws-playbook-render-tier-override: split-out tooling gate — render-time tier override that lets one elevated body serve large+xlarge
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
frontier model (mini-lead) keeps the interface/spine, the hard or
non-decomposable parts, and orchestration for itself; only the mechanical,
disjoint implementation grind goes to cheap medium-or-below implementers (10x+
cheaper than large).

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

The seam need not be parallel. When a ticket decomposes only phase-serially, the
mini-lead still runs each phase as its own fresh-context leaf — sequential
execution needs no parallelism to keep the orchestrator's context clean. Direct
in-context implementation is for the small or non-decomposable-and-hard core,
keyed to difficulty, not to whether leaves could run concurrently.

Everything here uses **existing primitives** — workers already hold
lead-capability keys (`role: worker` → lead scope) and can `ferrule`, spawn
native subagents, `worktree.acquire`, and message children via native
SendMessage. No harness modification is required, so this can ship in ws proper
(downstream-first; Architecture Rule 4 clean).

## Playbook restructure

Today three ticket-worker playbook bodies (`ticket-worker` = medium,
`ticket-worker-elevated` = large, `ticket-worker-escalated` = xlarge) differ
by a single frontmatter line (`tier:` medium/large/xlarge) and are otherwise
byte-identical. That duplication is
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

- The three `ticket-worker*` playbooks under `agents-plugin/rsrc/` differ by
  exactly one line — the frontmatter `tier:` value (medium/large/xlarge); their
  body prose is otherwise byte-identical (confirmed by diff, and by three
  distinct SHA-256 entries in each package's `rsrc/manifest.json`). Tier is
  purely a model-selection knob; it does not change the stop list, spawn rights,
  or review requirements.
- Workers are not server-side restricted from spawning further children:
  `playbook.render` maps `role: worker` → lead-equivalent key scope
  (`childRoleForPlaybookRole` in `agents-plugin-tool/internal/mcp/playbook_tools.go`),
  and lead scope permits every tool including `ferrule`
  (`roleAllowsTool` / `isLeadOnlyTool` in `.../mcp/server.go`). Per the anchor
  ticket 260909-research-ws-refoundation-evidence-audit (Harness capability
  premises) and epic Decision 4, depth-1 native recursion is owner-confirmed on
  Claude Code, pi, and Codex, and deeper nesting is discouraged in prose, not
  blocked. The three-harness confirmation is an owner statement recorded there,
  not a tree/code fact.
- The mini-lead pattern already existed once as a *parallel* fan-out:
  `lead-goal-fan-out-step` minted a lead-capability child key via
  `ferrule(capability: "lead", ...)`; the child ran its own loop and was
  re-discovered via `session.children`. That entry point has already been
  retired — no `lead-goal-fan-out-step` file remains under
  `agents-plugin/rsrc/` (removed by commit `f2294816`, 2026-09-09; search for
  `lead-goal-fan-out-step` under `agents-plugin/rsrc/` returns nothing). The
  tracking ticket 260730-refactor-retire-goal-fan-out-step-and-session-note
  remains open in `todo/` despite the removal already having landed. The
  underlying `ferrule(capability: "lead", ...)` mechanism itself is unaffected
  and remains a documented `ferrule` option
  (`agents-plugin-tool/internal/mcp/server.go:3372`).
- Mailbox (`mailbox.send/recv/lookup_peers`) is **cross-process only** and
  explicitly out of scope for native subagents inside one session
  (`agents-plugin/rsrc/lead-use-mailbox/...`; identity is resolved once per MCP
  server process in `.../mcp/mailbox_runtime.go`). There is no
  sibling-to-sibling messaging primitive; `session.children`/`session.note` are
  parent-owned bookkeeping only.
- `config.resolve_agent(tier, harness)` is the read-only resolver
  (`wsconfig.ResolveAgentTierForHarness`) that turns a tier into a concrete
  `{backend, model, effort}`. The config layer already supports resolution
  decoupled from a playbook's frontmatter tier (`ResolveAgentForHarnessConfig`,
  `config.go:220-263`), but `playbook.render` never threads an override into it:
  `RoleModel` and the `recommended-tier`/`recommended-model` binding derive
  solely from `pb.Meta.Tier`. That render gap — the gate for "xlarge = elevated +
  override" — is split out as the actionable feat
  `260919-feat-ws-playbook-render-tier-override`.
- Phase scoping is a **soft, tier-blind body rule, not a hard stop.** `lead-run`
  hands the worker the whole ticket (one worker per ticket, no per-phase slicing);
  the body scopes execution to the earliest phase without a `### Result`
  (`ticket-worker.md:22-25,81-84`) and closes a phase with the clean terminal
  `[ok] + completion: phase` (`worker-stop-protocol.md:111-112`) — none of the
  hard stops (a)-(e) covers "later phases remain." The stop logic carries no
  `tier:`/`RoleModel` gating, so a large worker reads the same weak boundary as a
  medium one under "everything else is yours to decide"
  (`worker-stop-protocol.md:14-15`); this is why phase-overrun clusters in
  large-tier workers. It reconciles epic Decision 4 (whole ticket) with the
  observed per-phase execution — one mechanism, not a conflict.

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
6. **Delegation is calibrated by difficulty, not by a quota.** The mini-lead
   delegates mechanical, disjoint, cheaply-verifiable leaves to fresh implementers
   by default, and keeps the hard, high-leverage, or non-decomposable work on
   itself — that is what the expensive model is for. Both directions carry weight:
   a confident large worker must not hoard mechanical grind into a drifting
   context (the same overreach that carries it past the soft phase boundary — see
   Verified Findings), nor push hard work onto a cheap implementer to manufacture
   a delegation (cheap model × hard task → Critical churn and re-review, the
   opposite of the saving). A ticket with no clean mechanical leaf simply runs
   direct — that is calibration, not a shortfall.

### Proposals
<!-- none: the design above was confirmed, not left as candidate. -->

### Open Questions
<!-- Implementation-time checks; resolve during the actionable child, not from evidence alone. -->

- **Resolved / split out.** Whether `playbook.render` accepts a tier override
  decoupled from frontmatter `tier:` — it does not today; the config resolver
  supports it but render never threads it. Carved out as the actionable feat
  `260919-feat-ws-playbook-render-tier-override` (todo), which gates the "xlarge =
  elevated + override" approach.
- `lead-run` dispatch table and escalation ladder: the stop-(e)
  medium→large→xlarge ladder must become "large → `elevated` + xlarge model
  override" rather than rendering a separate escalated body. Confirm the exact
  edit surface.
- Reference sweep for the removed `ticket-worker-escalated` body and any
  consumer keying off the now-stale frontmatter `tier:` — `lead-run` table,
  wsflow skill-shim drift tests, runtime-contract tests, manifests.
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

## Prior Decisions

- 260724-feat-lead-fan-out-worktree (2026-07-24, ticket Decisions): "Each
  parallel worker is a mini-lead: a native subagent holding a worktree-bound
  lead-scope key minted by the lead (ferrule, capability: lead)." — bearing:
  supports
- 260910-refactor-risk-route-worker-tier-medium-large (2026-09-10, ticket
  Result): "Shipped medium ticket-worker, large ticket-worker-elevated, and
  xlarge ticket-worker-escalated playbooks whose bodies differ only by tier
  frontmatter." — bearing: constrains
- 260915-refactor-lead-run-dispatch-time-tier-judgment (2026-09-15, commit
  83c60e42): "Kept the reactive escalate ladder (lead-run.md stop-e retry
  table) unchanged as the safety net; added one prose line noting xlarge is
  now also a proactive dispatch-time pick." — bearing: constrains
- 260909-refactor-lead-surface-collapse-worker-stop-protocol (2026-09-09,
  commit 27d32dd2): "A lead-scoped key permits every tool including ferrule,
  so the root is the key's default binding, not a boundary." — bearing:
  supports
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, ticket
  Decisions): "No orchestrator role. Extend the existing worker ownership
  model; do not add another runtime role or move lead playbooks into a
  mini-lead process." — bearing: supports
- b8be3591 (2026-07-24, commit): "Model (1b) contained delegate children with
  lead-owned selection + serial merge + delegated review; start at the
  (1-min) rung, escalate to fat mini-lead (2) only on measured overload." —
  bearing: supports
- 260913-bug-ws-pi-worker-checkout-contaminates-lead-branch (2026-09-13,
  ticket Result): "worker-stop-protocol.md Branch section: the terminal
  report does not restore the checkout; checking branch state before the
  next write is the lead's responsibility." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-worker*, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-tool/internal/mcp/playbook_tools.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-wsflow/, agents-plugin/skills/ |
| scope.surface | cross-module | rsrc playbook prose plus agents-plugin-tool/internal/mcp/ (playbook_tools.go, server.go) plus wsflow/pi mirrors |
| scope.new_public_symbol | unknown | gated on open question: playbook.render model-override decoupled from tier: feasibility unresolved |
| scope.new_type_contract | no | Confirmed Decisions state the design uses only existing primitives (ferrule, worktree.acquire, session.children, config.resolve_agent) |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go:2732 TestPlaybookPrintLeadRunWorkerTierPolicy, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | ferrule capability lead at agents-plugin-tool/internal/mcp/server.go:3372, childRoleForPlaybookRole at agents-plugin-tool/internal/mcp/playbook_tools.go:398, ResolveAgentTierForHarness at agents-plugin-tool/internal/wsconfig/config.go:273 |
| complexity.side_effect_risk | high | restructures the shared worker-dispatch playbook trio consumed by every lead-run invocation, mirrored across agents-plugin, agents-plugin-wsflow, and agents-plugin-pi |
| risk.correctness | high | core dispatch/worker-tier logic with multiple unresolved implementation-time open questions (render-override feasibility, escalation-ladder edit surface, phase-handling reconciliation) |
| risk.fit | moderate | builds on existing lead-scope worker primitives, but interacts with the already-landed dispatch-time tier judgment (260915-refactor-lead-run-dispatch-time-tier-judgment, commit 83c60e42) not yet reconciled in this ticket |
| risk.test | moderate | existing pinned/golden tests cover the current three-body table; a consumer sweep for the retired escalated body is an explicit open question |
| risk.security_or_contract | low | Confirmed Decision 5 keeps both playbook bodies at role: worker (lead scope); no new permission tier or capability boundary is introduced |

## Constraints

Editing paths `agents-plugin/rsrc/`, `agents-plugin/skills/`, and
`agents-plugin-wsflow/` triggers the read obligations for
`ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`, and
`ai-docs/manuals/shipped-surface-boundary.md` before the edit. The shipped
surface is downstream-first (Architecture Rule 4): this design deliberately
uses only existing primitives so nothing here depends on host-specific behavior.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
