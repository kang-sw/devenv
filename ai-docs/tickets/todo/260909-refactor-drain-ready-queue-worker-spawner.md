---
title: "drain-ready-queue becomes the single drainer and the worker spawner"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: required
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; its baseline run must land before this removal
  260730-refactor-retire-goal-fan-out-step-and-session-note: reconciled here; its Phase 1 entry-point deletion is absorbed, its Phase 2 premise is overturned
  260909-refactor-lead-surface-collapse-worker-stop-protocol: dependent; consumes the spawn mechanism this ticket establishes
  260909-research-ws-refoundation-evidence-audit: evidence; harness-capability premises and rejected alternatives
  260725-research-goal-loop-restart-starved-by-background-delegation: the starvation argument that killed background fan-out; this ticket's serial posture is the answer to it
---

# drain-ready-queue becomes the single drainer and the worker spawner

## Background

The epic `260909-epic-ws-worker-interpreter-refoundation` moves execution off
the lead: Cross-Child Decision 4 makes a native-harness subagent, holding a
lead-capability child key, the interpreter of a *whole ticket*, and states that
the lead never edits source and never reads procedure playbooks. Cross-Child
Decision 9 makes `drain-ready-queue` the only goal-loop entry point, with
parallelism opened inside it later and no second entry point.

Today neither half is true. `lead-drain-ready-queue` selects one ticket and
then hands it to `lead-proceed` **in the lead's own session** — the lead reads
the routing playbook, the implement playbook, and everything those pull in, for
every ticket in the queue. The mechanism the epic wants already exists, but it
exists in the wrong place and behind a second entry point:
`lead-goal-fan-out-step` mints a lead-capability child key with
`ferrule(capability: "lead", parent_session_key: ...)`, spawns a mini-lead that
runs `lead-proceed` itself, tracks it with `session.note`, and re-discovers it
after compaction with `session.children`. The evidence audit
(`260909-research-ws-refoundation-evidence-audit`, `## Harness capability
premises`) names this the one existing instance of the worker-as-interpreter
mechanism and states the disposition: "The fan-out entry point retires; the
mechanism moves into `drain-ready-queue`."

`260730-refactor-retire-goal-fan-out-step-and-session-note` already argued the
entry point should die, on three grounds: it starves the `/goal` Stop-hook that
drives the loop (N background mini-leads leave no clean terminal stop, see
`260725-research-goal-loop-restart-starved-by-background-delegation`), it is a
host-shaped procedure sitting in a host-neutral surface, and it carries
name-hardcoded Go for exactly one caller. That ticket has no `### Result`
sections; nothing has executed. Its Phase 1 conclusion (delete the entry point
and its transclusion hook) is exactly what this ticket needs, so it is absorbed
here rather than run twice. Its Phase 2 conclusion (retire `session.note`,
justified by "zero consumers") no longer holds: this ticket gives
`session.note` a consumer.

The `260730` starvation objection is answered by scope, not by disagreement.
Fan-out's problem was *N background* workers with no clean terminal; this
ticket spawns *one* worker per drain invocation and the lead waits for it, so
the invocation still has a single terminal line to hand to the Stop hook.
Parallelism is the epic's explicit Deferred item and is not opened here.

## Decisions

1. **One entry point: `drain-ready-queue` selects and spawns.** Per drain
   invocation: select one advanceable `ready/` ticket (the existing selection
   rule is unchanged), render the worker playbook with `playbook.render`,
   which mints a lead-capability child key for the worker's root as part of
   rendering (`ferrule` is the base primitive underneath and is not called a
   second time for the same worker; epic Cross-Child Decision 12), and hand
   the worker the whole ticket. The lead does not read `lead-proceed`,
   does not route, and does not edit source.
   *Rejected: keep fan-out as a second, parallel entry point.* The epic forbids
   a second entry point (Decision 9) and `260730` documents why the existing
   one is unworkable under the Stop-hook loop.
   *Rejected: put the spawn in `lead-proceed` instead.* `lead-proceed` is
   itself a lead-facing procedure playbook the epic is retiring
   (`260909-refactor-lead-surface-collapse-worker-stop-protocol`); building the
   spawn on it would have to be undone immediately.

2. **The worker's unit of work is the ticket, not a phase.** The worker routes,
   edits, reviews, commits, and closes; it stops only on the epic's closed
   five-condition list (Cross-Child Decision 5). *Rejected: one worker per
   phase, or a stop at every phase boundary for visibility* — recorded as
   rejected in the evidence audit's `## Rejected alternatives`: each interim
   report is a lead turn, which is the resource the epic is conserving.

3. **Serial first; parallelism deferred, inside, later.** One worker in flight
   per drain invocation. The epic lists "parallelism inside `drain-ready-queue`"
   under Deferred. When it is opened it must be opened *inside* this skill —
   never as a new skill — and it must answer the `260725` starvation finding
   before it lands. *Rejected: reinstating batch parallelism now* as part of
   absorbing fan-out: it would re-import the exact defect that retires fan-out.

4. **Keep the mint/track mechanism; retire only the entry point.**
   `ferrule(capability: "lead", parent_session_key: ...)`, `session.children`,
   and `session.note` all survive and move under drain: the render step
   mints on top of `ferrule`, `session.note` records the worker's state
   against the lead's key so a compacted lead can rebuild the board,
   `session.children` re-discovers the worker. Per epic Cross-Child Decision
   16 the drain turn waits for the host's completion or interim notification
   (no blocking tool call, no polling), so `session.note` is a carry-over
   record of in-flight worker-to-ticket assignments, one section per lead
   session, not a live progress board; the terminal is the host
   notification, which is what closes the `260725` starvation case. The
   goal-to-parent merge terminal is unchanged (epic Cross-Child Decisions 5a
   and 17): `goal/<parent>/<stem>` merging into `<parent>` is a user-approved
   stop whatever `<parent>` is, the lead aggregates the workers' terminal
   reports into that merge-stop report, and the lead performs the merge
   after approval. *This overturns `260730` Phase 2*, whose entire justification was
   that `session.note` had zero shipped consumers. Re-scoping or dropping
   `260730` is an inventory action for the user and lead (epic Cross-Child
   Decision 8), not this ticket's; this ticket only records that its Phase 1 is
   absorbed and its Phase 2 premise is void.

5. **A `lead` capability is required, and that is why the key is minted.** The
   worker runs the routing and implementation surface, which is lead-scoped.
   The evidence audit notes that `route.resolve_*` key requirements are
   undocumented and that "the worker holds a lead-scoped child key, so the
   question dissolves at the drain point". This ticket relies on that and does
   not document the resolver's key requirements — that is not its scope.

6. **No `mercenary.*` route.** Per epic Cross-Child Decision 10 the spawn is
   native harness delegation only. *Rejected: ws runtime as the workflow
   interpreter* — recorded as owner-rejected in the evidence audit.

## Constraints

- **Shipped-surface rule (AGENTS.md `## Architecture Rules` 4).** Everything
  changed here — `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`, any
  rsrc text, any Go string an agent sees — runs in projects that hold only what
  bootstrap installs. The text must not name this repository's tickets
  (including `260730`, `260725`, and the epic), its layout, its packages, or
  its migration vocabulary. `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`
  is the mechanical guard and has deliberately no allowlist; a pinned
  devenv-only string is a bug in the test as well as in the text.
- **Host-neutral first (AGENTS.md `## Architecture Rules` 3).** The spawn must
  be described as a native-harness subagent in host-neutral terms. `260730`
  recorded that fan-out's host-shaped spawn instruction was a rule violation;
  reproducing that phrasing inside drain reproduces the violation. The evidence
  audit records depth-1 native recursion as confirmed on Claude Code, pi, and
  Codex, and records that spawn depth is prose, never mechanically enforced
  (epic Non-Scope).
- **wsflow mirroring** (`ai-docs/manuals/wsflow-mirroring.md`, mandatory read
  before editing). `lead-drain-ready-queue` is both a build-time *composition*
  target (`lead-prefer-subagent`'s body is spliced at the end of `## Posture`)
  and one of the substitution-*mirrored* inline skills. Never hand-edit the
  wsflow copy. Order is mandatory — **compose first, mirror second** — or
  ws-namespace text leaks into the wsflow package. Every regen command needs
  `-count=1`. Removing a skill is a same-change obligation across the curated
  lists in `skills_mirror_test.go` and `skills_compose_test.go`, the "Shipped
  wsflow Skills" section of the mirroring manual, and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`'s `EXPECTED_SKILLS`
  / `EXPECTED_INLINE_SKILLS` / `EXPECTED_PARALLEL_INIT_SKILLS` /
  `PARALLEL_INIT_TITLES`.
- **Skill authoring.** Read `ai-docs/manuals/skill-authoring.md` and apply its
  invariant checklist to every changed Invariants/Constraints line before
  editing skill or playbook text (AGENTS.md `## Code Standards` 5).
- **Runtime gates.** `ferrule`, `session.children`, and `session.note` are
  gated in both `runtime.json` files; a change to which tools drain calls must
  keep those gates coherent. Note that `260730`'s cited gate version is stale —
  re-read the files rather than trusting the citation.
- **Measurement first.** The epic names
  `260909-chore-ws-refoundation-git-history-measurement-manual` the prerequisite
  for every removal; its baseline run must be committed before this ticket's
  Phase 2 removes anything.
- **Approval Protocol.** Deleting a skill and changing an observable workflow
  behavior are both "always ask" / "ask first" items in AGENTS.md; the design
  review on this ticket is where that is settled, not mid-implementation.

## Prior Art

Located by search term, not by line number:

- **The mechanism to move.** `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`
  — frontmatter `kind: print` / `delegates: true`; grep it for `ferrule`,
  `parent_session_key`, `session.note`, `session.children`, and
  `workflow_manual` to find the mint, the self-contained worker task block, the
  `dispatched` → `merged`/`blocked` state seeding, and the compaction-recovery
  read. Its shim is `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md`
  (parallel-init shape).
- **The skill to change.** `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`
  — grep for `## Select`, `## Dispatch a returned ticket`, `lead-proceed`,
  `merge_confirm`, `## Blocked`, `goal/`, and `## Ending the turn`. The
  selection rule, the goal-branch staging, the two terminals, and the mandatory
  verbatim final line are all existing behavior this ticket keeps.
- **The Go transclusion hook.** `agents-plugin-tool/internal/mcp/playbook_tools.go`
  — grep for `goalFanOutStepPlaybookName`, `drainReadyQueuePlaybookName`,
  `drainReadyQueuePlaybookTitle`, `printPlaybook`, and `WrapForConcatenation`.
  The hook splices drain's body into the fan-out playbook at serve time; its
  tests are `playbook_tools_test.go` (grep `lead-drain-ready-queue` — they
  assert boundary markers, ordering, and lockstep equality against
  `wsrsrc.LoadSkillBody`, once for ws roots and once for wsflow roots).
- **Session-auth surface.** `agents-plugin-tool/internal/mcp/server.go` — grep
  `bootstrapToolName`, `parseCapabilityScope`, `session.children`,
  `session.note`; and `agents-plugin-tool/internal/mcp/session_auth.go` for the
  child-record store and note write path. `session_auth_test.go` and
  `server_test.go` pin the capability behavior.
- **Compose/mirror machinery.** `agents-plugin-tool/internal/wsrsrc/` — grep
  `composedSkills`, `ComposeSkillBody`, `GenerateWsflowSkillBody`,
  `guardSubstitutionEligible`, `TestRegenerateComposedSkills`,
  `TestRegenerateWsflowSkillsMirror`, `TestGenerateRealSkillsManifest`,
  `TestRegenerateWsflowRsrcMirror`.
- **Inventory pins.** `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/manifest.json` (hash entries per file);
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (asserts drain is an
  inline body and specifically *not* a `playbook.read` shim — a refactor that
  turns drain into a shim fails here);
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.
- **Caller policy already flowing through drain.**
  `agents-plugin-tool/internal/mcp/implement_resolver.go` and `session_state.go`
  — grep `merge_confirm` and `goal`. Drain already passes
  `policy.branch.merge_confirm: "skip"` on a `goal/*` branch; the worker
  inherits the same reversibility posture the epic's stop condition (a) assumes.
- **Documentation carrying the retiring entry point.**
  `ai-docs/spec/workflow-skills.md` and `ai-docs/spec/plugin-runtime.md` (grep
  `fan-out`, `transclusion`), `ai-docs/mental-model/workflow-skills.md` (grep
  `lead-goal-fan-out-step`), `ai-docs/manuals/wsflow-mirroring.md` (shipped
  skills list), `CHANGELOG.md`.

## Phases

### Phase 1: drain spawns a lead-capability worker per ticket, serially

**Goal.** `lead-drain-ready-queue`'s dispatch step stops handing the ticket to
`lead-proceed` in the lead's own session and instead: renders the worker
playbook (which mints a lead-capability child key for the worker's root),
spawns one native-harness subagent of at least
current-mainstream class, hands it the ticket as a source pointer (path and
stem — never a lead summary of the ticket, per epic Cross-Child Decision 7),
records the worker against the lead's key, waits for the worker's terminal
report, and folds that report into the existing turn-ending contract. The
selection rule, the goal-branch staging, both terminals, the blocker-recording
rule, and the mandatory verbatim final line are unchanged.

**What the worker is told.** A self-contained task block, not a copy of the
lead's conversation: its own session key, the ticket path, the branch it works
on, its stop conditions, and its required terminal report shape. It is expected
to acquire its own workflow context with its own key rather than receive a
digest of the lead's.

**What stays open.** This phase does not define the worker's stop-and-report
protocol text or the worker-facing playbooks — that is
`260909-refactor-lead-surface-collapse-worker-stop-protocol`. Until that ticket
lands, the worker runs the existing routing surface with its lead-capability
key, exactly as the fan-out mini-lead does today. The two tickets are ordered:
this one supplies the spawn, that one supplies what the spawned worker reads.

**Verification expectations.**

- `go build ./...` and `go test ./...` from `agents-plugin-tool/` clean.
- Both python suites clean: `python3 -m unittest discover agents-plugin/tests`
  and `python3 -m unittest discover agents-plugin-wsflow/tests`. Note the
  known pre-existing failure in `test_skill_dispatch_contracts.py` recorded in
  `260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings` — do
  not absorb that fix here, but do distinguish it from a regression this phase
  caused.
- Composition and mirror regenerated in the mandatory order (compose, then
  mirror, then the skills manifest), each with `-count=1`, and the drift gates
  (`TestComposedSkillsUpToDate`, `TestWsflowSkillsMirrorUpToDate`,
  `TestSkillsManifestDriftIsVisible`) green afterward.
- `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` green; in
  addition, read the changed skill text as a lead in a project that has never
  heard of this repository.
- A dogfood run: one real `ready/` ticket drained end to end through a spawned
  worker on a throwaway branch, with evidence that the lead session never read
  the implementation playbook and never edited source. Record the worker's
  terminal report and the lead's final line verbatim in the phase Result.

**File touchpoints.**

- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` — the `## Dispatch a
  returned ticket` section (grep `lead-proceed`, `merge_confirm`).
- `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` — generated;
  regenerate, never hand-edit.
- `agents-plugin/skills/manifest.json` — regenerated hash entry.
- Possibly `agents-plugin-tool/internal/wsrsrc/skills_compose.go` if the
  composition anchor (`end of ## Posture`) moves; check `composedSkills`.
- Read-only: the fan-out playbook (source of the mint/track call shapes), the
  session-auth Go files, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/skill-authoring.md`.

### Phase 2: retire the fan-out entry point and its bespoke Go hook

Sequentially dependent on Phase 1: the entry point is only removable once its
replacement mechanism is live inside drain. Removing it first would delete the
one working instance of the mechanism Phase 1 is copying from.

**Goal.** Delete `lead-goal-fan-out-step` as an entry point from both packages,
delete the serve-time transclusion hook that exists solely for it, and
reconcile every inventory that names it. `session.note`, `session.children`,
and `ferrule` all stay — they now have a consumer in drain.

**Verification expectations.**

- `grep -r lead-goal-fan-out-step` returns nothing outside `ai-docs/`,
  `CHANGELOG.md`, and git history.
- `go build ./...` and `go test ./...` clean after the const block and the
  `printPlaybook` doc comment are reconciled; verify no other reader of the
  removed consts exists before deleting them.
- Both python suites clean after the curated inventory lists move together.
- Manifests and the wsflow rsrc mirror regenerated with `-count=1`; drift gates
  green.
- The runtime gates for `ferrule`, `session.children`, and `session.note` are
  unchanged and still coherent — confirm by reading both `runtime.json` files,
  not by trusting a cited version.
- The measurement baseline from
  `260909-chore-ws-refoundation-git-history-measurement-manual` is committed
  before this phase's removal lands.

**File touchpoints.**

- Delete: `agents-plugin/skills/lead-goal-fan-out-step/`,
  `agents-plugin/rsrc/lead-goal-fan-out-step/`,
  `agents-plugin-wsflow/skills/lead-goal-fan-out-step/`,
  `agents-plugin-wsflow/rsrc/lead-goal-fan-out-step/`.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` — grep
  `goalFanOutStepPlaybookName`, `drainReadyQueuePlaybookName`,
  `drainReadyQueuePlaybookTitle`, `printPlaybook`.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go` — the two
  transclusion tests die with the hook.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` and
  `skills_compose_test.go` — curated lists.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` —
  `EXPECTED_SKILLS`, `EXPECTED_PARALLEL_INIT_SKILLS`, `PARALLEL_INIT_TITLES`,
  and the per-skill repair-pointer regex.
- Manifests: `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/manifest.json`.
- Docs: `ai-docs/manuals/wsflow-mirroring.md` (shipped skills list),
  `ai-docs/spec/workflow-skills.md` and `ai-docs/spec/plugin-runtime.md` (grep
  `fan-out`, `transclusion`), `ai-docs/mental-model/workflow-skills.md`
  (replace the example rather than deleting the sentence), `CHANGELOG.md`.

## Open Questions

- **How the spawn is expressed host-neutrally.** The epic requires a
  native-harness subagent "of at least current-mainstream or
  previous-generation-flagship class" and confirms depth-1 recursion on three
  hosts, but does not settle how the shipped skill text names the spawn
  primitive without becoming host-shaped — the exact defect `260730` recorded
  against fan-out. Settle at design review: a host-neutral phrasing, or a
  declared hook the host adapter fills.
- **The disposition of `260730`.** Its Phase 1 is absorbed here and its Phase 2
  premise is overturned; its Phase 3 documentation closeout overlaps this
  ticket's Phase 2. Whether it is dropped, re-scoped to the `session.note`
  question alone, or closed as absorbed is a ticket-inventory decision for the
  user and lead, not something this ticket should decide.
