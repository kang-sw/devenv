---
title: Curate the Pi lead's tool profile and add an orchestrator spawn role that runs ticket phases on the lead's behalf
parent: 260907-research-ws-pi-extreme-delegation-lead-profile
related:
  260906-research-ws-pi-recon-preset-agent-alias: the orchestrator's own explore surface; its API decision should not conflict with the orchestrator tool group defined here
  260906-feat-ws-pi-tool-and-push-tui-polish: owns any lineage rendering beyond the minimal parent tag Phase 2 adds
  260906-bug-ws-pi-rsrc-mirror-drift: the orchestrator guide is an adapter-owned package-root file, deliberately outside the byte-identical rsrc/ mirror that ticket guards
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: prerequisite for the fork half of the profile — a fork's `tools` array must equal the lead's at spawn, so fork-only tools load after the prefix (see the deferred-tool decision here)
  260904-feat-ws-pi-side-thread-fork-question-surface: Entry A already names `lead-write-ticket` from Populate onward as the canonical fork task; this ticket makes that the only ticket-authoring path
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

### Evidence: tools activated inside a tool call do not break the prefix (spike, 2026-09-07)

Pi marks a tool result with `addedToolNames` when an extension tool's
`execute()` grows the active tool set (`core/extensions/wrapper.js`). For
models whose compat flags say `supportsAdditionalTools` (openai-codex
gpt-5.6-*/gpt-6-astra) or `supportsToolSearch` (gpt-5.4+), the provider
adapter then keeps those tools *out of* the top-level `tools` array and
emits their schemas as an `additional_tools` developer item (or a synthetic
`tool_search_call/output` pair) in `input`, right after that tool result —
i.e. after the cached prefix (`splitDeferredTools`, `convertResponsesMessages`).
The Anthropic adapter maps the same marker to `defer_loading`. Models without
either flag fall back to the top-level array: the tools still work, the
prefix just re-hashes once. Isolated spike on gpt-6-astra (scratch cwd, one
throwaway extension, 3k-token filler prompt):

| call | moment | uncached input | cacheRead |
|---|---|---|---|
| 1 | first call, tools = `[load_extra]` | 6,006 | 0 |
| 2 | after `load_extra` activated `extra_a`,`extra_b` | 306 | 5,888 |
| 3 | after calling `extra_a` | 211 | 6,016 |
| 4 | after calling `extra_b` | 116 | 6,144 |

Conditions: the add must happen inside a tool's `execute()` (a
`session_start` or user-turn `setActiveTools` does not produce the marker);
an `execute()` that also *removes* a tool produces no marker and the removal
re-hashes the array; the marker is persisted in the session file, so a
`--fork` child inherits deferred tools at the same positions. The set is
therefore monotone within a session: load, never unload.

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
  `full-worker`/orchestrator children via `wsToolNames`. The profile filters
  registration on the lead process only (role gate via `readSpawnRole`);
  children are unaffected. The resident set is what the lead's own two
  playbooks (`lead-discuss`, the thinned `lead-proceed`) and its spawn/relay
  duties call, nothing else. Initial resident profile (validated by dogfood,
  adjustable without code):
  - ws-mcp: `playbook.read`, `workflow_state`, `tickets.move`,
    `tickets.close`, `git.status`, `note.write`.
  - adapter-registered (unchanged by the filter): the
    `do-i-really-have-to-*` friction series (four tools, see the native
    surface decision below), `lead-workflow-manual`, the `ws-agent-*`
    driving tools, `explore`, `ws-fork`, `ws-ask`/`ws-resolve`,
    `ws-execute`/`ws-approve`, the `goal-*` levers, `ws-skill`.
    `ws-execute`/`ws-approve` stay resident on purpose (owner, 2026-09-07):
    their context cost is small, they are the lead's second-busiest surface
    in the dogfood record (45 `ws-approve` / 29 `ws-execute` calls over 33
    lead sessions), and a "hotfix mode" lead with no execute path is worse
    than one with it. Approval *attribution* still moves down for work
    under an orchestrator (decision below); the tool itself remains.
  - held for dogfood, not resident by default: `project_tree` (lean render
    only landed in the dev build), `tickets.query` (discuss-time evidence
    reads may go through `explore` instead), `git.log`.
  - **`todo.*` is not resident on the lead.** The todo runbook is installed
    by `route.resolve_*` and consumed by `lead-proceed`/`lead-implement`/
    `lead-review`, all of which run in the orchestrator; neither
    `lead-discuss` nor `lead-write-ticket` references a todo tool. The
    orchestrator has its own ws session key, so todo state does not collide.
  - **Ticket authoring is not resident on the lead.** `lead-write-ticket`
    from Populate onward runs in a `ws-fork` (260904 Entry A); its tools
    (`tickets.create_empty/template/checklist/verify/sage_gate/sage_stamp/
    query`, `convention.read`, `playbook.render`, `spec_stem.generate`,
    `specs.query`, `git.commit`) form a **fork authoring profile** loaded by
    the mechanism in the next decision, so the lead pays for none of their
    schemas.
- **Fork tool surface equals the lead's at spawn; fork-only tools load after
  the prefix.** Per `260907-bug-ws-pi-fork-first-call-prompt-cache-miss`, a
  fork's `--tools` list is the lead's active list unchanged (byte-identical
  `tools` array, so the inherited context hits the prompt cache). The fork
  authoring profile is activated by the fork's **first tool call**: the
  fork's initial message instructs it to call one adapter loader tool whose
  `execute()` registers/activates the profile via `setActiveTools`, which
  rides in as `additional_tools` after the prefix (evidence above). The
  loader is add-only (never removes), idempotent, and refuses outside role
  `fork`. Loader tool name and whether `ws-skill` hosts it are implementation
  choices. Not decided here: whether the lead itself lazy-loads withheld
  groups through the same channel — noted as a follow-up once the resident
  set has been dogfooded.
- **Lead native surface: `ls` only; `edit`/`write` join the friction
  series; `grep`/`find` are removed.** Today `computeLeadActiveTools`
  (execute-gateway.ts) removes native `bash`/`read` and adds
  `ws-execute`/`ws-approve` plus the two `do-i-really-have-to-*` tools. It
  now also removes `edit`, `write`, `grep`, `find`, and adds
  `do-i-really-have-to-edit-this-myself` and
  `do-i-really-have-to-write-this-myself` (names follow the existing pair).
  Intent is inertia removal: the lead should reach for `explore` (evidence)
  and delegation (change) first, and the friction name plus a description in
  the style of the existing read wrapper ("... is a fallback, not your first
  move; consider `explore` / a spawned agent before calling this") is what
  steers a model that would otherwise edit by habit. Dogfood record over 33
  lead sessions: `edit` 30 / `write` 12 (25 of the edits on one day of
  in-lead ticket authoring and implementation — exactly the work this ticket
  moves to the fork and orchestrator), `grep`/`find`/`ls` 0 each, and the
  read wrapper already called as often as `explore` (27 / 27). `grep`/`find`
  therefore get no wrapper: nothing to redirect, and a wrapper would keep
  their schemas resident. `ls` stays native (owner's call; cheap
  orientation). Implementation: the edit/write wrappers delegate to Pi's
  exported `createEditTool`/`createWriteTool` implementations (rename and
  re-describe, do not re-implement exact-match edit semantics); the
  `tool_call` hook's `block` option was considered and rejected — it keeps
  the appealing native name, and the point is the name. Consequence for
  forks: a fork inherits this friction-only array, and its loader activates
  native `edit`/`write` (and whatever else its profile needs) after the
  prefix — `splitDeferredTools` keys on names, so native tools defer the
  same way adapter tools do.
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
`bridge.ts`; keep the full `wsToolNames` for children. Extend
`computeLeadActiveTools` per the native-surface decision (remove
`edit`/`write`/`grep`/`find`, add the two new friction wrappers delegating
to Pi's exported tool factories; `ls` untouched) and give the fork loader
native `edit`/`write` in its profile. Tests: a lead-role registration
exposes exactly the profile; a child-role registration is unchanged; an
unknown name in the profile fails loudly at startup; the mercenary
exclusion still applies on top; the reshaped lead array contains `ls` and
the four friction tools and none of `bash`/`read`/`edit`/`write`/`grep`/
`find`; the edit wrapper performs a real exact-match edit through the
native implementation; worker/orchestrator groups still carry native
`edit`/`write`/`grep`/`find`. Verification: first-call `input`
token count of a fresh lead session before and after (baseline ~24.7k),
recorded in the Result; one dogfood session confirms nothing the lead
actually needs is missing, adjusting the list rather than the code. Fork
half: a `ws-fork` from the profiled lead calls the loader, and its next
assistant `usage` shows `cacheRead` ≥ 90% of `input + cacheRead` while the
authoring tools are callable (the spike's shape: 306 uncached / 5,888
cached on the call after the load); a `lead-write-ticket` run inside that
fork completes end to end. Tests: the loader adds and never removes; a
non-fork role gets the refusal; the fork's `--tools` equals the lead's.

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
