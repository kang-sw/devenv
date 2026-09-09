---
title: "Collapse the lead skill surface; turn lead procedure playbooks into worker playbooks carrying the stop-and-report protocol"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: required
related:
  260909-refactor-drain-ready-queue-worker-spawner: prerequisite; supplies the spawn mechanism the worker playbooks are written for
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; its baseline run must land before this removal
  260909-refactor-retire-spec-mental-model-layers: sibling; owns the retirement of the spec/mental-model-only lead skills, not this ticket
  260909-refactor-route-resolve-implement-reads-ticket-facts: sibling; removes the in-run survey/plan stages this ticket's worker playbooks would otherwise still choreograph
  260909-chore-retire-mercenary-surface: sibling; the auto-injection path for delegate orientation is mercenary-only and dies with it
  260909-research-ws-refoundation-evidence-audit: evidence; assumption verdicts A3/A8 and the rejected alternatives
  260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings: pre-existing failure in the same contract test this ticket edits; distinguish it from a regression
---

# Collapse the lead skill surface; turn lead procedure playbooks into worker playbooks carrying the stop-and-report protocol

## Background

The epic `260909-epic-ws-worker-interpreter-refoundation` states that the lead
"only converses with the user, manages the ticket inventory, spawns workers,
and handles first-line escalation" and that "the lead never edits source and
never reads procedure playbooks" (Cross-Child Decision 4). Two things stand in
the way, and they are the two halves of this ticket.

**The lead skill surface is still an execution surface.** Eighteen `lead-*`
skills ship; the run path alone is spread across a routing entry skill, a
resolver, an implementation playbook that has no skill of its own, and the
queue drainer. The evidence audit
(`260909-research-ws-refoundation-evidence-audit`, `## Where the cost actually
is`) puts about 1,577 lines — roughly 19K tokens — of mandated reading on the
routed path for a one-line fix *before any source is read*, and most of that is
"the workflow describing itself to the lead". Verdict A8 is blunt about why
this is not merely wasteful: about 5,000 lines of playbook prose with 37 judge
blocks and 51 handlers is "neither deterministic nor leveraging model
judgment", and its conservative biases — "prefer triggering over skipping",
"when in doubt fall through to full routing" — are exactly what a capable model
obeys.

**The playbooks are addressed to the lead.** Every `kind: print` playbook in
the shipped tree is a `lead-*` playbook, and every `kind: render` playbook is a
worker or delegate prompt. `print` means "the body comes back inline, the lead
executes it"; `render` means "the body is written to a file for a subagent, and
a child session key is minted into it for a lead caller". Once the worker
interprets the whole ticket, a procedure delivered to the lead is delivered to
the wrong reader.

What the worker needs instead does not exist yet. The evidence audit records
that no existing playbook combines the mini-lead pattern with the
delegate-orientation rule "report user decisions to the lead", and that "the
stop-and-report-then-resume idiom is a new composition". The epic supplies the
closed stop list (Cross-Child Decision 5), the lead's first-line escalation
role (6), and the pointers-not-summaries rule (7) — this ticket writes them
into playbook text the worker actually reads.

## Decisions

1. **The lead surface collapses to five working skills plus housekeeping.**
   Working: `discuss`; `ticket` (write, batch promotion, drop, all through the
   Open Decision Queue); `run` (drain the queue plus ad-hoc implement);
   `review`; `ship` (Decision 8). Housekeeping: `bootstrap`, `tune`, `revive`, `mcp-server-repair`.
   Every other `lead-*` skill retires or becomes a worker playbook.
   *Rejected: keeping the routing entry skill as a thin front door.* Routing is
   the worker's first act, not the lead's; a lead-facing routing skill re-adds
   the lead turn the epic removes, and verdict A5 records that the "lead must
   not touch code before routing" premise is moot once the lead never edits
   source.
   *Rejected: "fat lead" — the lead doing more inline with its large context.*
   Owner-rejected in the evidence audit: the lead is the expensive, slow,
   careful model; work goes to cheaper workers and lead judgment is spent only
   on escalation.

2. **`kind: print` procedure playbooks become worker-facing `kind: render`
   playbooks.** The reader changes from the lead to the worker, so the
   addressee, the self-containment requirement, and the output contract all
   change. `kind:` itself is parsed and never branched on in the runtime — the
   caller decides whether a body is read or rendered — so the conversion is a
   text-and-caller change, and the frontmatter value must be corrected in the
   same change or the declared kind silently lies about the surface.

3. **The core of every worker playbook is the stop-and-report protocol.** The
   worker stops only on the epic's closed five-condition list, records every
   other decision in the commit `## AI Context` and the ticket `### Result`,
   and batches those decisions into the merge-stop report for veto. The
   protocol is stated once and included, not restated per playbook.
   *Rejected: stopping at every phase boundary for visibility* — recorded as
   rejected in the evidence audit: each interim report costs a lead turn, which
   is the resource being conserved.

4. **Resume is host-native, with re-spawn as the declared fallback.** The lead
   resumes a stopped worker through the host continuation mechanism
   (`SendMessage` on Claude Code; the harness idiom table already carries the
   Codex and default forms). The runtime stores no mapping between a host agent
   id and a ws session key, and no ws tool resumes a child, so the protocol
   must say what happens when the host has no continuation mechanism: re-spawn
   with a recap, which is safe because the worker's inputs are pointers to
   durable artifacts rather than a conversation.
   *Rejected: adding a ws-side resume tool.* Out of scope for this epic and it
   would be a new coordination surface; the existing doctrine ("resume is an
   optional latency optimization") already covers the gap.

5. **Pointers, not summaries, is a written rule in the delegate prompts.**
   A delegate prompt carries the ticket path and stem, and a reviewer computes
   its own diff range from git rather than receiving one. No worker's summary is
   another worker's sole input. Any planner-style structured output must carry
   an explicit omitted/deferred field. This is epic Cross-Child Decision 7,
   grounded in two observed failures the audit cites: a survey paraphrase that
   dropped a load-bearing detail, and a planner that narrowed scope unilaterally.
   *Rejected: lead-side caution as the mitigation* — the epic replaces it
   explicitly, because it costs a lead turn per hop and did not prevent either
   failure.

6. **Lead-side first-line escalation before the user sees anything.** On a
   contract-broken stop (stop condition (c): a ticket decision contradicted by
   code reality), the lead raises tier and re-runs the sage gate over the
   *worker's proposed resolution*, then resumes the worker; only an exhausted
   attempt reaches the user. On a Critical finding surviving three review
   rounds (stop condition (e)), the lead performs the elevation. Per epic
   Cross-Child Decision 6, the user sees only low-reversibility decisions and
   exhausted lead attempts.

7. **This ticket does not own the layer-coupled retirements.** Lead skills
   whose entire purpose is a document layer another child retires — the spec
   and mental-model authoring, forging, updating, and backfill skills — retire
   with that layer in `260909-refactor-retire-spec-mental-model-layers`. This
   ticket removes them from the lead's *surface inventory* only in coordination
   with that ticket, and does not delete the layer itself.
   *Rejected: absorbing those retirements here* to finish the collapse in one
   ticket: it would put two independent removals behind one review and make the
   measurement comparison unattributable.

8. **Ship stays a lead skill; the worker's key comes from `playbook.render`.**
   Per epic Cross-Child Decisions 12 and 13: the release skill is the fifth
   working skill, not housekeeping and not a worker playbook, because a
   release is a low-reversibility action whose stop belongs to the lead and
   user. The worker's lead-capability key is minted by `playbook.render` when
   the spawner renders the worker playbook; this ticket's stop protocol and
   worker playbooks assume that key and never mint another. The host agent
   id needed to resume a stopped worker is recorded by the lead in the
   per-session `session.note` carry-over section alongside the
   worker-to-ticket assignment (epic Cross-Child Decision 16); re-spawn with
   a recap remains the fallback when the host offers no resume.
   *Rejected: `ferrule` before spawn plus a render-time mint* — two keys for
   one worker with no rule for which one `session.children` tracks.

9. **The intent review survives as a three-line conversation-fidelity check;
   the capture checklist is unchanged.** `tickets.checklist(phase: "intent")`
   answers a question the promotion-time sage gate cannot: whether the lead
   that held the conversation dropped or flattened something the owner said.
   The sage reviewer has no conversation, so it checks fit, not fidelity;
   the two axes are distinct and both stay. What goes is redundancy: item 1
   repeats the capture checklist's enumeration, item 5 restates the ticket
   conventions' epic and workset rules, and items 6 and 7 are procedure
   sentences ("fix in place", "present a summary"). The compressed intent
   checklist is exactly: the fresh-implementer test (could an implementer
   reading only the ticket build a materially different caller-visible,
   workflow, API, or verification result without contradicting it), literal
   preservation of API/type/event/UI sketches, and no unconfirmed mechanism,
   future-scope hint, Result Forward note, or focus "Next" line. Unconfirmed
   gaps still return to the Open Decision Queue; that rule lives in the
   `ticket` skill, not the checklist. Once the ticket-authoring skill's
   playbook is retired, the Go constants in `tickets_checklist.go` are the
   checklist's single source and their "extracted verbatim" comment is
   updated to say so; the ticket conventions' one reference to the retired
   skill name is renamed to the `ticket` skill.
   *Rejected: drop the intent review because the sage gate reviews the
   ticket* — the sage gate never sees the conversation.

## Constraints

- **Depends on `260909-refactor-drain-ready-queue-worker-spawner`.** The worker
  playbooks are written for a worker that exists; without the spawner they have
  no reader. Land the spawner first.
- **Shipped-surface rule (AGENTS.md `## Architecture Rules` 4).** Every line
  written here — playbook bodies, skill descriptions, stop-protocol text, any
  Go-emitted instruction string — runs in a project holding only what bootstrap
  installs. No ticket stem, epic name, `ai-docs/` path bootstrap does not
  install, package name, or migration vocabulary. The mechanical guard is
  `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`, which has
  no allowlist by design; a pinned devenv-only string is a bug in the test too.
- **Skill authoring is a mandatory read with mandatory gates**
  (`ai-docs/manuals/skill-authoring.md`, AGENTS.md `## Code Standards` 5).
  Three points bear directly on this ticket:
  - Its **layer model** deletes Layer 1 (MCP schema restatement) and Layer 2
    (routing computation and *all* post-call tool output text) from playbooks
    unconditionally, with "uncertain → delete". Much of what a collapsing
    playbook contains will fall out under this test rather than needing to be
    rehomed.
  - Text rendered verbatim into a delegate prompt is **exempt** from that
    destructive-first test and is instead audited under **Agent Layout**:
    Identity → Constraints → Process → Heuristics → Output (required) →
    Doctrine, self-contained, with communication rules injected by the calling
    skill rather than baked into the agent definition. The new worker playbooks
    are audited under this shape, not the lead-playbook shape.
  - Every invariant line must pass the seven-item checklist (falsifiable,
    actionable, one line, context-free, non-redundant, doctrine-aligned,
    resolvable downstream). The last item is the shipped-surface rule in
    checklist form.
  - The **Fresh-Reader Audit** (max three cycles) is mandatory after any edit
    and its target glob is literally `agents-plugin/rsrc/lead-*/lead-*.md` plus
    `agents-plugin/skills/*/SKILL.md`; renaming or collapsing lead playbooks
    changes which files the gate covers, and that change must be deliberate.
    The **Downstream Consistency Sweep** is mandatory after doctrine, routing,
    or layout edits.
- **wsflow mirroring** (`ai-docs/manuals/wsflow-mirroring.md`, mandatory read).
  The wsflow package ships a *superset* of the lead skill names — it carries
  real skills for several playbooks that exist only as rsrc in the full package
  — so a collapse moves more files there, not fewer. The rsrc tree is a
  generated byte-identical mirror: regenerate, never hand-edit. Compose before
  mirror. Every regen command needs `-count=1`. Adding or removing a skill is a
  same-change obligation across the curated lists in `skills_mirror_test.go`
  and `skills_compose_test.go`, the mirroring manual's shipped-skills section,
  and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`'s
  `EXPECTED_SKILLS` / `EXPECTED_INLINE_SKILLS` / `EXPECTED_PARALLEL_INIT_SKILLS`
  / `PARALLEL_INIT_TITLES`. The substitution-eligibility guard fails
  unbypassably on `ws/`, `ws:`, `ws.`, the word `mercenary`, and a fixed list of
  retired skill names — a new worker playbook body must satisfy it.
- **No `mercenary.*` route** (epic Cross-Child Decision 10). Note the coupling:
  `delegate-orientation.md`, which carries the "report user decisions to the
  lead, never wait for human sign-off yourself" rule, is auto-injected only
  into *mercenary* agent prompts. Native subagents never receive it. The
  stop-and-report protocol must carry that rule itself rather than assume the
  injection.
- **Host-neutral first (AGENTS.md `## Architecture Rules` 3).** The
  continuation mechanism is named through the existing harness idiom
  substitution, not hard-coded to one host.
- **Approval Protocol.** Deleting skills, changing canonical flows, and
  changing observable workflow behavior are "always ask" items; the design
  review on this ticket is where the per-skill disposition is settled.
- **Measurement first.** The epic makes
  `260909-chore-ws-refoundation-git-history-measurement-manual` the prerequisite
  for every removal; its baseline run must be committed before Phase 2 lands.
- **A pre-existing test failure sits in the blast radius.**
  `260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings` records
  that `test_skill_dispatch_contracts.py` already fails on `develop` by pinning
  wording the routing playbook no longer has. Do not silently absorb that fix;
  do distinguish it from a regression this ticket caused, and note that
  retiring that playbook may resolve the bug ticket by deletion.

## Prior Art

Located by search term, not by line number:

- **The print/render split.** `agents-plugin-tool/internal/wsrsrc/wsrsrc.go`
  (grep `PlaybookMeta`, `Kind`) and `loader.go` (grep `case "kind":`) — the
  field is parsed and has no other non-test reader, which is why the conversion
  is a text-and-caller change. `agents-plugin-tool/internal/mcp/playbook_tools.go`
  (grep `printPlaybook`, `renderPlaybook`, `renderPlaybookBody`,
  `appendRenderContext`, `splitDeclaredRenderContext`,
  `withRecommendedRenderBinding`, `delegationTip`) — render writes a
  worktree-scoped file, mints a child session key for a lead caller, and
  appends tier/model/effort bindings. `server.go`, grep `playbook.read` and
  `playbook.render`.
- **What the frontmatter looks like on each side.** Grep `^kind:` and
  `^delegates:` across `agents-plugin/rsrc/*/*.md`: every `print` is a `lead-*`
  playbook, every `render` is a worker/delegate prompt, and `delegates:` is
  orthogonal — it means the reader will itself spawn, and it triggers the
  continuity tip. Worker prompts additionally declare `role:` and `tier:`; no
  `print` playbook declares a tier.
- **The existing stop-and-report token protocol.** Grep `escalate-to-lead`
  across `agents-plugin/rsrc/` — the planner playbooks return `[ok]` /
  `[escalate-to-research]` / `[escalate-to-lead]` with an `## Escalations`
  section, and the lead-side handler adjudicates in place. Grep
  `plannerAuthorityInputs` in `agents-plugin-tool/internal/mcp/session_state.go`
  for the Go-emitted counterpart. This is the closest existing shape to the new
  protocol and its vocabulary should be reused rather than reinvented.
- **The reporting rule that needs a new home.**
  `agents-plugin/rsrc/delegate-orientation.md` — grep `Your caller is the lead`,
  `Never wait for or assume human sign-off`. Injection path:
  `agents-plugin-tool/internal/wsagent/agent.go`, grep
  `loadDelegateOrientation`.
- **Continuation.** `agents-plugin-tool/internal/mcp/playbook_tools.go`, grep
  `playbookTerminologyTable`, `terminologyForHarness`, `ContinueIdiom`,
  `SpawnIdiom`, `delegationTip` — the harness idiom substitution and the
  "the playbook surface keeps no agent registry" tip.
  `agents-plugin/rsrc/lead-implement/lead-implement.md`, grep `native
  continuation mechanism` and `re-spawn with a recap` — the existing doctrine
  this ticket promotes into the protocol.
- **Session-key plumbing the worker sits on.** Grep `session.children`,
  `session.note`, `ferrule`, `parseCapabilityScope` in
  `agents-plugin-tool/internal/mcp/server.go` and `session_auth.go`;
  `lead-revive` and `workflow_manual(session_key)` for the lead's own
  post-compaction reload.
- **The gates the lead keeps.** `agents-plugin/rsrc/lead-write-ticket/` — grep
  `Open Decision Queue`, `judge: needs-open-decision-queue`,
  `tickets.sage_gate`, and the `task-list` overlay files for the
  `open|confirmed|rejected|deferred` item states. These are the `ticket` skill's
  existing substance and survive the collapse.
- **Branch reversibility facts.** Grep `merge_confirm` in
  `agents-plugin-tool/internal/mcp/implement_resolver.go` and in
  `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` — the `skip|ask|unknown`
  fact that stop condition (a) is expressed against.
- **Tier selection for the worker and its children.** Grep `resolveTierModel`,
  `RoleModel`, `ImplicitVariableNames`, `agents.tier` — the escalation decision
  "raise tier and retry" has an existing mechanism to ride.
- **Inventory pins.** `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/manifest.json`,
  `agents-plugin/tests/test_skill_dispatch_contracts.py`,
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`,
  `agents-plugin-tool/internal/wsrsrc/` (grep `composedSkills`,
  `GenerateWsflowSkillBody`, `guardSubstitutionEligible`,
  `TestSkillsManifestDriftIsVisible`).

## Phases

### Phase 1: the stop-and-report protocol and the worker-facing procedure playbooks

**Goal.** Write the protocol once, convert the procedure playbooks the worker
actually needs from lead-facing `print` to worker-facing `render`, and add the
lead-side escalation handling that receives their stops. The lead skill
inventory is untouched in this phase — the old entry points keep working while
the new reader is proven.

**The protocol.** One shared body, included by the worker playbooks rather than
restated in each. It states:

- **Identity and addressee.** The reader is a worker holding a lead-capability
  child key, executing one whole ticket. Its caller is the lead, not the user.
  It never waits for or assumes human sign-off; a gate that needs the user is
  reported to the lead. (This rule currently reaches only mercenary agents by
  auto-injection; here it becomes part of the payload.)
- **The closed stop list**, verbatim from the epic's Cross-Child Decision 5:
  (a) a low-reversibility merge; (b) an `[escalate-to-lead]` entry or an Open
  Decision Queue during promotion; (c) a ticket decision contradicted by code
  reality so it cannot be executed as written; (d) an irreversible action in
  the Approval Protocol's always-ask category; (e) a Critical finding surviving
  three review rounds. Stated as a closed list, with the explicit statement
  that everything else is decided by the worker.
- **What happens to everything else.** Recorded in the commit `## AI Context`
  and in the ticket's phase `### Result`, then batched into the merge-stop
  report so the lead can veto. Nothing is dropped silently and nothing costs an
  extra stop.
- **The report shape.** A fixed, machine-checkable terminal report: which stop
  condition fired (or none), the decisions taken with their rationale, the
  branch and commit range, verification evidence, and unresolved findings.
  Reuse the existing `[ok]` / `[escalate-to-lead]` token vocabulary rather than
  inventing a parallel one.
- **Resume.** The lead resumes through the host continuation mechanism named by
  the harness idiom substitution; where the host has none, the lead re-spawns
  with a recap, which is lossless because the worker's inputs are pointers.
- **Pointers, not summaries.** The worker's inputs are the ticket path and
  stem; a reviewer computes its own diff range from git; a worker's summary is
  never another worker's sole input; any planner-style structured output
  carries an explicit omitted/deferred field.
- **Declared conventions.** The worker reads `AGENTS.md` as the host loads
  it, then the path-scoped conventions section it declares (epic Cross-Child
  Decision 18) through a generic hook, reading only the manuals whose
  `paths` match what the ticket touches, plus every manual the ticket's
  `## Constraints` cites. A change that contradicts a cited convention is a
  review finding, not a stop. The hook is the same shape as the
  binding-anchor declaration: a declared section with fixed keys; a project
  that declares none has no convention read.

**The conversion.** For each procedure playbook the worker needs, change the
addressee, satisfy Agent Layout (Identity → Constraints → Process → Heuristics
→ Output → Doctrine, self-contained), set `kind: render` with the appropriate
`role:` and `tier:`, include the protocol, and update every caller to render
rather than read. Apply the layer model on the way: Layer 1 and Layer 2
material is deleted, not rehomed, and "uncertain → delete".

**Lead-side escalation.** The lead's handling of an arriving stop, written into
the `run` path: on (c), raise tier and re-run the sage gate over the worker's
*proposed resolution*, then resume; only an exhausted attempt reaches the user.
On (e), the lead performs the elevation. On (a) and (d), the lead carries the
decision to the user. Stops must not be re-summarized on the way to the user —
the pointer rule applies to the lead too.

**Verification expectations.**

- `go build ./...` and `go test ./...` clean; both python suites clean apart
  from the failure already recorded in
  `260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings`, which
  must be identified explicitly rather than absorbed.
- Compose, then mirror, then the skills manifest, each with `-count=1`; drift
  gates green.
- Fresh-Reader Audit run on every changed `lead-*` playbook and `SKILL.md`, max
  three cycles, with each finding classified fix / risk accepted / intentional
  difference / out of scope. Downstream Consistency Sweep run, since this phase
  changes doctrine and layout.
- Every new invariant line checked against the seven-item checklist; record the
  check, not just the outcome.
- `test_shipped_surfaces_downstream_neutral.py` green, plus a human-style read
  of the new protocol text as a lead in a project that has never heard of this
  repository.
- Dogfood: one real ticket driven end to end by a spawned worker reading the
  new playbooks, producing a terminal report in the fixed shape; and one
  deliberately induced stop-condition-(c) case showing the lead resolving it
  without reaching the user. Record both verbatim.

**File touchpoints.**

- New: the shared stop-and-report body under `agents-plugin/rsrc/` (a bare-name
  include, following the shape of the existing non-`kind:` includes).
- Converted: the procedure playbook directories under `agents-plugin/rsrc/`
  found by grepping their frontmatter for `^kind: print` — frontmatter, body
  addressee, and every caller that currently uses `playbook.read` on them.
- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` (the lead-side
  escalation handling lands on the run path established by
  `260909-refactor-drain-ready-queue-worker-spawner`).
- `agents-plugin/rsrc/delegate-orientation.md` — the reporting rule's new home
  is the protocol; decide whether the original stays for the mercenary path
  that `260909-chore-retire-mercenary-surface` is removing.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` and `session_state.go` —
  only if a Go-emitted instruction string still tells the lead to read a
  converted playbook; grep `Instruction`, `plannerAuthorityInputs`.
- Regenerated: `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/rsrc/manifest.json`,
  `agents-plugin/skills/manifest.json`.

### Phase 2: retire the collapsed lead skills

Sequentially dependent on Phase 1: a lead skill is only removable once the
worker-facing replacement it fronts is landed and dogfooded. Removing entry
points first would leave the queue with no path.

**Goal.** Reduce the shipped lead skill inventory to the working five —
`discuss`, `ticket`, `run`, `review`, `ship` — plus housekeeping (`bootstrap`, `tune`,
`revive`, `mcp-server-repair`), across both packages and every inventory that
names them. Skills fronting a document layer another child retires are
coordinated with that child, not deleted here.

**Disposition, per skill, to be confirmed at design review.** Absorbed into
`run`: the routing entry skill and the queue drainer (the drainer is the run
surface after the spawner ticket). Absorbed into `ticket`: the ticket authoring
skill's existing write / promote / drop paths with the Open Decision Queue
intact. Absorbed into `discuss`: the discussion verification checkpoint.
Becoming worker playbooks or dying with their layer: the implementation,
spec-authoring, spec-updating, doc-backfill, and forge skills. Deferred to a
sibling ticket: anything whose only content is the spec or mental-model layer.
Kept as the lead's `ship` surface: the release skill (epic Cross-Child
Decision 13); its procedure body may become a manual handed to a subagent,
but the release decision and stop stay with the lead. With the ticket
authoring skill absorbed, compress `tickets.checklist(phase: "intent")` to
the three items of Decision 9, leave the `content` phase unchanged, and
rename the ticket conventions' `lead-write-ticket` reference to the `ticket`
skill (its spec-gate bullets are removed by the sibling that retires the
spec layer; whichever lands second reconciles the line). Unsettled and listed
under Open Questions: the worktree scoping skill, the rule-persisting skill,
and the delegation-posture skill.

**Verification expectations.**

- The measurement baseline from
  `260909-chore-ws-refoundation-git-history-measurement-manual` is committed
  before this phase's removals land.
- `go build ./...` and `go test ./...` clean; both python suites clean after
  the curated inventory lists in `skills_mirror_test.go`,
  `skills_compose_test.go`, the mirroring manual, and
  `test_wsflow_skill_bundle.py` move together.
- `grep -r` for each retired skill name returns nothing outside `ai-docs/`,
  `CHANGELOG.md`, and git history.
- Manifests and the wsflow mirror regenerated with `-count=1`; drift gates
  green; the substitution-eligibility guard passes on every surviving body.
- The runtime tool gates in both `runtime.json` files remain coherent with the
  reduced surface — a tool whose only caller was a retired skill is a finding
  to record, not to silently keep or silently drop.
- A cold-load check that the reduced skill set still presents a complete path:
  a fresh session can discuss, author and promote a ticket, run it, and review
  it, with no reference to a retired skill name anywhere in the flow.
- Fresh-Reader Audit and Downstream Consistency Sweep re-run on the surviving
  skills, since the collapse changes what each remaining file must stand alone
  against.

**File touchpoints.**

- Delete or fold: the retired directories under `agents-plugin/skills/` and
  `agents-plugin/rsrc/`, and their counterparts under
  `agents-plugin-wsflow/skills/` and `agents-plugin-wsflow/rsrc/` — note the
  wsflow tree ships real skills for several playbooks that are rsrc-only in the
  full package, so its removal list is longer.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`,
  `skills_compose_test.go` — curated lists.
- `agents-plugin-tool/internal/wsdoc/tickets_checklist.go` and its tests —
  the compressed intent checklist (Decision 9);
  `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` — the
  retired skill name.
- `agents-plugin/tests/test_skill_dispatch_contracts.py` — the shim/inline
  assertions for retired skills die with them; see the recorded pre-existing
  failure before touching this file.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` — all four expected
  sets.
- `ai-docs/manuals/wsflow-mirroring.md` — shipped skills list and mirroring
  exceptions.
- `ai-docs/spec/workflow-skills.md`, `ai-docs/spec/plugin-runtime.md`,
  `ai-docs/mental-model/workflow-skills.md`, `CHANGELOG.md` — inventory and
  narrative reconciliation, coordinated with the sibling that retires those
  layers.
- Manifests: `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/manifest.json`.

## Open Questions

- **Disposition of the worktree-scoping, rule-persisting, and
  delegation-posture skills.** None is a procedure playbook and none is
  housekeeping as the epic names it. The delegation-posture body is currently
  spliced into the drainer at build time, so retiring it as a *skill* and
  keeping it as an include may be two separate answers.
- **Whether `ticket` is one skill or the existing skill renamed.** The epic
  names a `ticket` surface covering write, batch promotion, and drop with the
  Open Decision Queue. Whether that is the existing authoring skill under a new
  name (which breaks stem-stable references and every inventory pin) or the
  existing skill kept as-is with the batch-promotion path added is not settled.
  Renaming has a real cost here; the epic does not say the name must change.
- **Which procedure playbooks the worker actually needs after the sibling
  tickets land.** `260909-refactor-route-resolve-implement-reads-ticket-facts`
  removes the in-run survey and plan stages, and
  `260909-refactor-retire-spec-mental-model-layers` removes the doc passes. The
  set of playbooks worth converting in Phase 1 depends on their landing order,
  and converting a playbook that a sibling then deletes is wasted review. Fix
  the ordering at design review.
