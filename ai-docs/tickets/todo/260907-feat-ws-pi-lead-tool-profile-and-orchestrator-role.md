---
title: Curate the Pi lead's tool profile and add an orchestrator spawn role that runs ticket phases on the lead's behalf
parent: 260907-research-ws-pi-extreme-delegation-lead-profile
related:
  260906-research-ws-pi-recon-preset-agent-alias: the orchestrator's own explore surface; its API decision should not conflict with the orchestrator tool group defined here
  260906-feat-ws-pi-tool-and-push-tui-polish: owns any lineage rendering beyond the minimal parent tag Phase 2 adds
  260906-bug-ws-pi-rsrc-mirror-drift: the orchestrator guide is an adapter-owned package-root file, deliberately outside the byte-identical rsrc/ mirror that ticket guards
---

# Curate the Pi lead's tool profile and add an orchestrator spawn role that runs ticket phases on the lead's behalf

## Background

The parent research ticket settled the direction: the Pi lead becomes a
curated-tool, discuss-only decision seat, and each ticket phase is handed to
a medium-tier **orchestrator** child that runs the existing lead playbooks
on the lead's behalf and reports back. The motivating figure is the owner's:
a `proceed` of one phase accumulates on the order of 200k tokens in the lead
today, essentially all orchestration ceremony, resident at lead tier for the
rest of the session. The lead's fixed first-call baseline is ~24.7k tokens,
of which the schemas of every registered `ws__*` tool are a large share.

The implementation is small relative to the role change, so this is one
ticket with two phases rather than one ticket per mechanism. Every seam
already exists: `bridge.ts` registers all ws-mcp tools with a single
`mercenary.*` exclusion; `spawner.ts` `TOOL_GROUPS` curates child tools and
keeps `ws-agent-*` out of `full-worker` so depth stays lead → worker →
explore-leaf; `process-role.ts` carries a role but no depth; the extension
already ships adapter-owned prompt files at the package root
(`pi-lead-guide.md`, `execute-worker-guide.md`, `explore-guide.md`) and hands
them to children as `systemPromptPath`.

## Decisions

- **Lead tool profile is an allowlist in the bridge.** The bridge registers
  on the lead only the tools named by an adapter-owned profile list; every
  other `ws__*` tool is withheld from the lead but still passed to
  `full-worker`/orchestrator children via `wsToolNames`. Initial profile
  (validated by dogfood, adjustable without code):
  `do-i-really-have-to-read-this-myself`,
  `do-i-really-have-to-run-this-myself`, `playbook.read`, `playbook.render`,
  `workflow.state`, `lead-workflow-manual`, the `ws-agent-*` driving tools,
  the ask/human-relay surface, `note.write`, `tickets.create_empty`,
  `tickets.move`, `git.status`, `git.log`. Children are unaffected: the
  profile filters registration on the lead process only (role gate via
  `readSpawnRole`).
- **`orchestrator` is a new spawn role and tool group.** `SpawnRole` gains
  `"orchestrator"`; `TOOL_GROUPS` gains `orchestrator` = `full-worker` plus
  the `ws-agent-*` driving tools. This is the single exception to the
  "no `ws-agent-*` below the lead" guard; an orchestrator's own children are
  spawned with the unchanged `full-worker`/`recon` groups, so depth is
  lead → orchestrator → worker → explore-leaf and nothing deeper. A depth
  marker env var accompanies the role so the guard is explicit, not only
  implied by tool omission. No orchestrator fork.
- **The orchestrator runs the existing lead playbooks unchanged.**
  `lead-proceed`/`lead-implement`/`lead-review` already describe a lead that
  spawns workers; the orchestrator is that lead from their point of view.
  `orchestrator-guide.md` (adapter-owned, package root, passed as
  `systemPromptPath`, never in `rsrc/`) *wraps* them: it states that the
  process runs a named ticket phase on behalf of an upper lead, the
  escalation envelope (answer inside the ticket's recorded decisions; raise
  `ws-report-to-lead` `question` for anything outside it; one question is
  cheaper than a failed phase), and the `final` report contract.
- **`final` report contract (draft, refined by the first live phase).** The
  close report carries: phase outcome in a few lines, verification evidence
  (test counts, commit hashes), open items, and a next-phase proposal. It is
  the lead's judgment boundary — if the lead has to re-read the work, the
  saving collapses — so its shape is part of this ticket, not an afterthought.
- **Execute-gateway approval attribution.** A worker's gated shell command
  under an orchestrator pauses for the *orchestrator's* approval when the
  command is inside the envelope; outside it, the orchestrator escalates as
  a `question` instead of approving. The approver class (a model) is
  unchanged; only the level moves.
- **Lead-side `proceed` thins to one judgment.** The lead's `lead-proceed`
  keeps `judge: discussion-needed` (is a discussion still owed to the user?)
  and otherwise spawns the orchestrator with stem, phase, and qualitative
  constraints. Routing judgments move into the orchestrator's run of the
  same playbook. This is Pi-side skill/guide text, not a shared playbook
  edit.
- **Minimal lineage rendering here; the rest elsewhere.** The agent widget
  gets a parent tag (or depth badge) so a row reads as attached to the lead
  or nested under an orchestrator. Anything richer belongs to the TUI polish
  ticket.
- **Rejected: three tickets.** Allowlist, role, and rendering are a filter,
  a group plus a marker plus a guide file, and a label; splitting them
  tracked the size of the concept, not the code.

## Constraints

- Pi-extension code only (`agents-plugin-pi/`); no ws-mcp contract change,
  no shared `rsrc/` playbook edit, `rsrc/` mirror stays byte-identical.
- `full-worker`, `recon`, `execute-worker` groups are not widened.
- Existing goal-loop, sidecar, and report protocols are reused, not forked;
  a nested tree must survive lead compaction/restart through the sidecar
  (verify, and ticket separately if it does not).

## Phases

### Phase 1: Lead tool profile

Add the adapter-owned profile list and the lead-only registration filter in
`bridge.ts`; keep the full `wsToolNames` for children. Tests: a lead-role
registration exposes exactly the profile; a child-role registration is
unchanged; an unknown name in the profile fails loudly at startup; the
mercenary exclusion still applies on top. Verification: first-call `input`
token count of a fresh lead session before and after (baseline ~24.7k),
recorded in the Result; one dogfood session confirms nothing the lead
actually needs is missing, adjusting the list rather than the code.

### Phase 2: Orchestrator role, guide, and hand-off

Add the `orchestrator` role/tool group and depth marker; write
`orchestrator-guide.md` (wrapper, envelope, `final` contract); route
execute-gateway approvals per the attribution decision; thin the lead's
`proceed` entry to the discussion-needed judgment plus spawn; add the parent
tag to the agent widget. Tests: an orchestrator can spawn a `full-worker`
and an `explore`; its worker cannot spawn a worker; depth marker is set and
read; gated commands under an orchestrator resolve to the orchestrator;
widget rows carry the parent tag. Verification (owner-run, live): one real
ticket phase driven end to end as discuss → hand-off → orchestrator →
`final` → next decision, with the lead's per-call context recorded through
the phase and compared against the ~200k-per-phase baseline; the `final`
contract is refined from what that phase shows and the refinement recorded
as an Edition.
