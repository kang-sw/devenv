---
title: "Collapse the lead skill surface; turn lead procedure playbooks into worker playbooks carrying the stop-and-report protocol"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
related:
  260909-refactor-drain-ready-queue-worker-spawner: prerequisite; supplies the spawn mechanism the worker playbooks are written for
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; its baseline run must land before this removal
  260909-refactor-retire-spec-mental-model-layers: sibling; owns the retirement of the spec/mental-model-only lead skills, not this ticket
  260909-refactor-route-resolve-implement-reads-ticket-facts: sibling; removes the in-run survey/plan stages this ticket's worker playbooks would otherwise still choreograph
  260909-chore-retire-mercenary-surface: sibling; the auto-injection path for delegate orientation is mercenary-only and dies with it
  260909-research-ws-refoundation-evidence-audit: evidence; assumption verdicts A3/A8 and the rejected alternatives
  260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings: pre-existing failure in the same contract test this ticket edits; distinguish it from a regression
sage-review-completeness: completed
sage-review-design-reviewed: a805616b0046fb1b
sage-review-completeness-reviewed: a805616b0046fb1b
---

# Collapse the lead skill surface; turn lead procedure playbooks into worker playbooks carrying the stop-and-report protocol

## Background

The epic `260909-epic-ws-worker-interpreter-refoundation` states that the lead
"only converses with the user, manages the ticket inventory, spawns workers,
and handles first-line escalation" and that "the lead never edits source and
never reads procedure playbooks" (Cross-Child Decision 4). Two things stand in
the way, and they are the two halves of this ticket.

**The lead skill surface is still an execution surface.** Seventeen `lead-*`
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

**The playbooks are addressed to the lead.** Every `kind: print` playbook (the sample fixture aside) in
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
   worker playbooks assume that key and never mint another. Design review
   found that the render mint maps only delegate roles and never yields a
   lead scope; per epic Cross-Child Decision 21 this ticket's Phase 1 adds
   the `worker` frontmatter role, for which `childRoleForPlaybookRole`
   returns the caller's lead scope, root-bound like `ferrule(capability:
   "lead")`. That is a capability-model change made under Decisions 12 and
   21, and it is the only Go change Phase 1 makes beyond instruction strings. The host agent
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

10. **The first dogfood run is the failure citation for the review-round and
    branch rules.** Epic Cross-Child Decision 20 records what happened; this
    ticket carries the rules into the protocol. Under the authoring standard
    a rule needs a failure observed on the current tier, and these two were
    observed on a current-flagship worker holding a documentation ticket:
    non-converging fresh-reviewer rounds, and a reset that dropped a
    concurrent lead commit. The fresh-reader audit of the protocol must not
    strike them as unexercised.

11. **Names and the surviving set are fixed** (epic Cross-Child Decision 22).
    Surviving skills keep the `lead-` prefix. This ticket's Phase 2 renames
    the ticket-authoring skill to `lead-ticket` and updates every pin; the
    spawner ships `lead-run`. Housekeeping survives unchanged:
    `lead-bootstrap`, `lead-tune`, `lead-revive`, `mcp-server-repair`,
    `lead-workflow-manual`, `lead-check-blockers`. The worktree-scoping,
    rule-persisting, and delegation-posture skills are not in this ticket's
    retire list: their deletion is always-ask and undecided, so they survive
    as they are; the delegation-posture *include* spliced into the drainer
    dies with the drainer in the spawner ticket regardless.
    *Rejected: bare names* — see the epic decision.

12. **Phase 1 places the drafts; nothing is converted in place.** The
    committed drafts `ai-docs/ref/refound-drafts/worker-stop-protocol.md`
    and `ticket-worker.md` are Phase 1's deliverables (move the text, delete
    the draft and its README row, fresh-reader audit once). `ticket-worker`
    supersedes `lead-implement` and `lead-proceed` for the worker; those two
    are retired in Phase 2, not converted. The reviewer playbooks the worker
    renders are already `kind: render`. The Phase 1 conversion set is
    therefore empty, which also settles the landing-order question with the
    route-facts and spec-retirement siblings: nothing here is converted that
    they later delete.
    *Rejected: convert `lead-implement` and `lead-proceed` to `render`* —
    the draft already carries what a worker needs of them, and both die one
    phase later.

## Constraints

- **Ordering with `260909-refactor-drain-ready-queue-worker-spawner`** (epic
  Cross-Child Decision 21): this ticket's Phase 1 lands first and is
  dogfooded by hand-spawned workers (epic Cross-Child Decision 19); the
  spawner lands next and renders `ticket-worker`; this ticket's Phase 2
  lands last. A worker reaching Phase 2 before the spawner is `.done/`
  ends its run after Phase 1's Result with this ticket left in `ready/`.
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
    destructive-first test and is written to the manual's **Worker
    playbook** layout instead: identity and addressee → inputs as pointers →
    constraints → the closed stop list → report shape, self-contained
    through pointers, no doctrine section, communication rules injected by
    the caller. The lead skills use the **Lead skill** layout (thin:
    identity → what the lead does → the stops it surfaces → output).
  - Every rule passes the manual's seven **Rule Tests**: falsifiable,
    actionable, scoped, non-derivable, failure-cited (a failure observed on
    the current worker tier), non-redundant, resolvable downstream. The last
    is the shipped-surface rule in test form.
  - The **fresh-reader audit** runs once per skill or playbook before it
    lands, not after every edit: one cycle, a second only if a fix produced
    a new finding; findings classified fix / risk accepted / intentional
    difference / out of scope. The **mirror sweep** applies whenever a
    change touches a surface mirrored into the wsflow package.
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
  / `PARALLEL_INIT_TITLES`. The substitution-eligibility guard applies to
  `SKILL.md` sources, not to rsrc playbook bodies (those mirror
  byte-identically); it rejects `ws.`, the word `mercenary`, the
  `ws:*-only` markers, and a fixed list of retired skill names, while `ws/`
  and `ws:` are the permitted namespace tokens.
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

**Goal.** Place the protocol include and the `ticket-worker` playbook from
the committed drafts (Decision 12), add the `worker` render role that mints
the worker's lead-scoped key (Decision 8), and prove the reader with
hand-spawned workers (epic Cross-Child Decision 19). The lead skill inventory
is untouched in this phase — the old entry points keep working while the new
reader is proven.

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
  the Approval Protocol's always-ask category; (e) a Critical finding still
  open after the fix round. Stated as a closed list, with the explicit
  statement that everything else is decided by the worker.
- **Two review rounds, fixed** (epic Cross-Child Decision 20): round 1 is the
  full review at the route's allocation; round 2 checks only that round-1
  findings were fixed and raises nothing new; no round 3. The playbook
  states this as the round structure, not as a count the worker may exceed.
- **Shared-branch discipline** (epic Cross-Child Decision 20): the goal
  branch carries concurrent lead commits; the worker never amends, resets,
  or rebases it, and corrects with a new commit. While a spawned delegate
  runs, the worker waits for the host's completion signal; no polling loops,
  no filler verification runs.
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

**Placement.** `worker-stop-protocol.md` becomes the bare-name include
`agents-plugin/rsrc/worker-stop-protocol.md`; `ticket-worker.md` becomes
`agents-plugin/rsrc/ticket-worker/ticket-worker.md` with `kind: render`,
`role: worker`, `tier: large`, `includes: worker-stop-protocol`. Move the
text; do not rewrite it. Where the placed text still contains a
`[design-review: ...]` marker, resolve it against this ticket's Decisions and
delete the marker. Delete each draft and its README row on placement. Apply
the layer model to anything the drafts carried over from the retiring
playbooks: Layer 1 and Layer 2 material is deleted, not rehomed.

**Lead-side escalation.** The lead's handling of an arriving stop lives in
the `lead-run` draft's `## Handle the report` section and ships with the
spawner; in this phase the lead applies it by hand. Per letter: on (a) and
(d), the lead carries the decision to the user with the report's lines, not a
summary. On (b), the lead reads the pointers the report names and either
settles the item from the conversation it holds (an Open Decision Queue item
the user already answered) or puts it to the user; the existing
`[escalate-to-lead]` adjudication is reused, not reinvented. On (c), the lead
routes the `proposed_resolution` through `lead-ticket` as an Edition under
design review at a raised tier, then resumes; only an exhausted attempt
reaches the user. On (e), the lead re-spawns on a higher-tier worker. Stops
must not be re-summarized on the way to the user — the pointer rule applies
to the lead too.

**Verification expectations.**

- `go build ./...` and `go test ./...` clean; both python suites clean apart
  from the failure already recorded in
  `260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings`, which
  must be identified explicitly rather than absorbed.
- Compose, then mirror, then the skills manifest, each with `-count=1`; drift
  gates green.
- Fresh-reader audit run once on each placed file before it lands (one
  cycle; a second only if a fix produced a new finding), each finding
  classified fix / risk accepted / intentional difference / out of scope.
  Mirror sweep run, since the rsrc tree mirrors into wsflow.
- Every new rule checked against the manual's seven Rule Tests; record the
  check, not just the outcome. Decision 10's two rules cite this epic's
  first dogfood run as their failure.
- `playbook.render` on a `role: worker` playbook returns a child key whose
  scope is lead and whose root is the caller's; a test pins it beside the
  existing role-mapping tests.
- `test_shipped_surfaces_downstream_neutral.py` green, plus a human-style read
  of the new protocol text as a lead in a project that has never heard of this
  repository.
- Dogfood: one real ticket driven end to end by a hand-spawned worker
  reading the placed `ticket-worker` (rendered, so the mint is exercised),
  producing a terminal report in the fixed shape; and one deliberately
  induced stop-condition-(c) case showing the lead resolving it without
  reaching the user. Record both verbatim.

**File touchpoints.**

- New: `agents-plugin/rsrc/worker-stop-protocol.md` (bare-name include,
  following the shape of the existing non-`kind:` includes) and
  `agents-plugin/rsrc/ticket-worker/ticket-worker.md`, both from
  `ai-docs/ref/refound-drafts/`; the drafts and their README rows deleted.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` —
  `childRoleForPlaybookRole` gains `worker` → lead scope; the render path
  binds it to the caller's root; `playbook_tools_test.go` pins it.
- `agents-plugin/rsrc/delegate-orientation.md` — the reporting rule's new home
  is the protocol; decide whether the original stays for the mercenary path
  that `260909-chore-retire-mercenary-surface` is removing.
- `agents-plugin-tool/internal/mcp/session_state.go` — only if a Go-emitted
  instruction string still tells the lead to read a playbook the worker now
  owns; grep `Instruction`, `plannerAuthorityInputs`.
- No spec or mental-model document is edited in either phase: that corpus is
  archived whole by `260909-refactor-retire-spec-mental-model-layers`.
- Regenerated: `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/rsrc/manifest.json`,
  `agents-plugin/skills/manifest.json`.

### Result (924d473e) - 2026-09-09

Landed across `924d473e..a909c29b`; the heading names the placement commit.

**Landed.** `ai-docs/ref/refound-drafts/worker-stop-protocol.md` is now the
bare-name include `agents-plugin/rsrc/worker-stop-protocol.md`;
`ticket-worker.md` is now `agents-plugin/rsrc/ticket-worker/ticket-worker.md`
with its drafted frontmatter (`kind: render`, `role: worker`, `tier: large`,
`includes: worker-stop-protocol`) unchanged. Both drafts and their README rows
are deleted. No `[design-review: ...]` marker was present in either draft. The
conversion set was empty as Decision 12 predicted, so nothing was converted in
place and no lead skill inventory changed.

`childRoleForPlaybookRole` gains `worker` → `roleLead` (Decision 8): the render
path already mints at the caller's root with the caller's key as parent, so the
one-case addition is the whole capability change. Pinned by two tests — a
fixture test for the mapping plus root/parent binding
(`playbook_tools_test.go`), and the table row in the existing mapping test
(`mercenary_surface_test.go`) — and by a golden test that renders the real
shipped `ticket-worker`, asserting the include splice, the absence of any
unsubstituted variable, the `large` tier, the harness continuity idiom, and the
lead-scoped mint.

**Verification.**

- `go build ./...` clean. `go test ./...` all packages `ok`.
- `python3 -m unittest discover agents-plugin/tests`: 53 tests, one failure —
  `test_proceed_keeps_implementation_route_only`, the pre-existing failure
  recorded in `260909-bug-proceed-contract-test-pins-pre-diet-lead-proceed-strings`.
  It fails identically on `epic/refound` and this change touches no
  `lead-proceed` file; it was not absorbed.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 10 tests, OK.
- `test_shipped_surfaces_downstream_neutral.py` green, plus a read of both
  placed files as a lead in a project that has never heard of this repository:
  no ticket stem, no `ai-docs/` path bootstrap does not install, no package
  name, no migration vocabulary.
- Rsrc manifest regenerated and the wsflow rsrc mirror synced, both with
  `-count=1`; `diff -r agents-plugin/rsrc agents-plugin-wsflow/rsrc` is empty.
  The skills-manifest regen was not run and was not required: no file under
  `agents-plugin/skills/` changed. Compose-then-mirror does not apply for the
  same reason. Mirror sweep run.

**Fresh-reader audit** (one cycle each, before landing; no fix produced a new
finding, so no second cycle). `worker-stop-protocol.md`: 24 findings (6 high,
12 medium, 6 low) — 6 fixed, 6 risk accepted, 10 intentional difference, 2 out
of scope. `ticket-worker.md`: 21 findings (3 high, 11 medium, 7 low) — 5 fixed,
4 risk accepted, 11 intentional difference, 1 out of scope.

Fixed in the protocol: the "never wait for sign-off" / "needs user approval"
contradiction in stop (a); stop (d) gained the Approval Protocol's declared
home and its absent-case default, so an unsure worker does not stop
spuriously; stop (e)'s "the fix round" became "round 2", matching the section
that defines the rounds; `## Branch` now names the shared branch as the one it
was spawned on rather than "your branch", which was ambiguous once the worker
also holds a work branch; the Resume section's restatement of the reporting
rule was dropped; and the report block gained the `status`/`stop` invariant and
the empty forms for `decisions:` and `proposed_resolution:`. Fixed in
`ticket-worker`: "task block" is defined at first use; the ticket's
`## Decisions`/`## Constraints` are named as the ticket's, since this file has
its own `## Constraints`; "do not accept a digest of the caller's" became the
positive rule; "no separate document to update" was narrowed to "no separate
behavior document", which could otherwise read as licence to skip the phase
Result; and step 1 states how the task block's branch and the route's branch
action relate.

Risk accepted, with cost: "main-class branch" is left undefined (an unsure
worker over-stops on a merge, which is the safe direction); the severity scale,
the review allocation, the route verdict's shape, and the delegate-spawn
mechanism are left to the tool output and the reviewer playbooks, because
restating them in prose is exactly the Layer 1/2 duplication the authoring
manual deletes unconditionally; no build/test-command fallback and no
warning-baseline step were added, because both would be new rules with no cited
failure. Intentional difference: everything the appended protocol or the
prepended credential block defines but the audited file does not, since neither
file is ever read alone — the audit reads one file by construction. The two
spawn formulations in step 2 and step 4 are deliberately different: the
exploration agent has its own spawn form and collapsing them would be wrong.
Out of scope: the `### Result (<short-hash>)` referent, owned by the ticket
conventions, and the `### Implementation Conventions` row shape, owned by the
bootstrap-template child.

**Rule Tests.** Each rule the placement introduces to a shipped surface was
checked against all seven. The two rules Decision 10 cites — the two-round
review cap and the shared-branch no-amend rule — pass **failure-cited** on this
epic's first dogfood run (non-converging fresh-reviewer rounds; a reset that
dropped a concurrent lead commit), both observed on a current-flagship worker,
so the fresh-reader audit's "unexercised rule" lens does not strike them.
**Non-derivable** is what carries the closed stop list: a worker with the code,
the tests, and the tool schemas cannot infer which decisions cost a lead turn.
**Scoped** is where the placement changed text: stop (d) gained its
absent-project default, and `## Declared Conventions` already carried one.
**Resolvable downstream** is the shipped-surface rule, checked mechanically and
by reading. **Non-redundant** removed the Resume section's restatement.
**Falsifiable** and **actionable** drove the audit fixes to stop (a) and to the
"digest" and "no separate document" lines, each of which stated a posture
rather than an action.

**Round-1 review** (single allocation, fresh reviewer, over `924d473e`; fixes
in `27d32dd2`): two
Important, five Minor, no Critical. Both Important fixed. The first is the
important one: the fresh-reader fix that renamed the branches had widened stop
(a) from "the goal branch you were spawned on" to "the branch you were spawned
on", which would let a worker hand-spawned on a shared non-goal branch
self-merge into it. Restored, and the audit's naming fix kept only in
`## Branch`, where the two do not conflict. The second was this Result's
absence. Three Minors fixed (the stop-(e) vocabulary split across the two
files, `PlaybookMeta.Role`'s stale value list, and a role comment that implied
the caller's root confines a worker — it does not; lead scope permits `ferrule`
at any root, which Decision 8 authorizes). Two Minors accepted: `RoleModel` is
declared but unreferenced, matching every sibling delegate playbook, and the
`### Implementation Conventions` hook ships self-guarded but unexercised until
the bootstrap-template child declares the section.

**Decisions taken.** The resume paragraph names the continuation mechanism in
prose rather than through the harness idiom variable: the protocol is a shared
include, and a template variable inside it must be declared by every including
playbook's frontmatter or rendering fails. The substitution still reaches the
worker — `delegates: true` appends the harness-specific continuity tip to the
rendered body, and the golden test asserts the Claude form is present.
`delegate-orientation.md` is left in place: its report-to-the-lead rule now
also lives in the protocol, but its auto-injection path is still live for
mercenary agent registration and its removal belongs to the ticket retiring
that surface. No Go-emitted instruction string changed, because this phase
retires no lead skill. The new render-role test lives in `playbook_tools_test.go`
as the ticket names, while the mapping table row was added to the existing
table test in `mercenary_surface_test.go` rather than duplicating that table.

**Not done, and why.** Two of the Phase 1 dogfood items are lead-side and are
left to the lead (epic Cross-Child Decision 19): a real ticket driven end to
end by a worker spawned against the *rendered placed* file, and the induced
stop-condition-(c) case demonstrating lead-side resolution without reaching the
user — the second is by definition a lead action. What this phase does record:
the placed reader was itself executed verbatim by the hand-spawned worker that
landed this phase, which routed, edited, verified, reviewed, and reported in
the fixed shape, and the golden test exercises the rendered mint mechanically.

Phase 2 was not started. `260909-refactor-drain-ready-queue-worker-spawner` is
still `ready/`, and this ticket's Constraints end the run after Phase 1's
Result in that case, leaving this ticket in `ready/`.

### Phase 2: retire the collapsed lead skills

Sequentially dependent on Phase 1: a lead skill is only removable once the
worker-facing replacement it fronts is landed and dogfooded. Removing entry
points first would leave the queue with no path.

Depends on `260909-refactor-drain-ready-queue-worker-spawner` being `.done/`
(epic Cross-Child Decision 21): the drainer must already be `lead-run` before
the routing entry skill is retired into it.

**Goal.** Reduce the shipped lead skill inventory to the working five —
`lead-discuss`, `lead-ticket`, `lead-run`, `lead-review`, `lead-ship` — plus
the housekeeping and undecided skills Decision 11 names, across both packages
and every inventory that names them. Skills fronting a document layer another
child retires are coordinated with that child, not deleted here.

**Disposition, per skill (Decision 11).** Retired into `lead-run`: the
routing entry skill (`lead-proceed`); the drainer is already gone with the
spawner. Renamed to `lead-ticket`: the ticket authoring skill, with its
write / promote / drop paths and the Open Decision Queue intact, and the
`lead-ticket`, `lead-discuss`, `lead-review`, `lead-ship` drafts placed over
the existing bodies (move the text; delete the drafts and README rows;
fresh-reader audit once each). Retired into `lead-discuss`: the discussion
verification checkpoint. Retired, superseded by `ticket-worker`:
`lead-implement`. Dying with their layer, owned by the spec-retirement
sibling: the spec-authoring, spec-updating, doc-backfill, and forge skills.
Kept as the lead's `lead-ship` surface: the release skill (epic Cross-Child
Decision 13); its procedure body may become a manual handed to a subagent,
but the release decision and stop stay with the lead. Surviving unchanged:
`lead-bootstrap`, `lead-tune`, `lead-revive`, `mcp-server-repair`,
`lead-workflow-manual`, `lead-check-blockers`, `lead-scope-worktree`,
`lead-add-rule`, `lead-prefer-subagent`. With the ticket
authoring skill absorbed, compress `tickets.checklist(phase: "intent")` to
the three items of Decision 9, leave the `content` phase unchanged, and
rename the ticket conventions' `lead-write-ticket` reference to the `ticket`
skill (its spec-gate bullets are removed by the sibling that retires the
spec layer; whichever lands second reconciles the line).

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
- Fresh-reader audit run once on each surviving skill whose body this phase
  changed, and the mirror sweep run, since the collapse changes what each
  remaining file must stand alone against.

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
- `CHANGELOG.md` — inventory reconciliation. The spec and mental-model
  corpus is not edited (see Phase 1 touchpoints).
- `ai-docs/ref/refound-drafts/README.md` — drop the placed rows.
- Manifests: `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`,
  `agents-plugin-wsflow/rsrc/manifest.json`.

## Open Questions

- **Final disposition of `lead-scope-worktree`, `lead-add-rule`, and
  `lead-prefer-subagent`.** They survive this ticket unchanged (Decision
  11). Whether they are later retired is a user decision under the Approval
  Protocol's always-ask category; nothing in this ticket depends on it.

## Sage Review Round 1 (2026-09-09)

### Design Reviewer — concern

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Decision 8 assumes a render mint that only maps delegate roles | major | Decision 8 amended: Phase 1 adds the worker frontmatter role in childRoleForPlaybookRole returning lead scope root-bound; test pinned; epic Decision 21. |
| 2 | Skill names and surviving set unsettled (bare vs lead- prefix, three unsettled skills) | major | Decision 11 / epic Decision 22: lead- prefix kept, lead-ticket rename, three unsettled skills survive by default; Open Questions reduced to their later disposition. |
| 3 | Phase 1 conversion set depends on sibling landing order | major | Decision 12: Phase 1 places the two drafts and converts nothing; lead-implement and lead-proceed retire in Phase 2; ordering fixed by epic Decision 21. |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Dependency direction on the spawner is inverted relative to the mint ownership | critical |
| 2 | Constraints restate the pre-rewrite skill-authoring manual (Agent Layout, seven-item checklist, three-cycle audit) | major |
| 3 | Substitution guard description wrong (ws/ and ws: are permitted tokens; guard applies to SKILL.md not rsrc) | major |
| 4 | Lead-side escalation handling written into a path that does not exist yet; stop (b) handling missing | major |
| 5 | Skill inventory count says eighteen; seventeen lead-* skills exist | minor |
| 6 | Spec/mental-model doc touchpoints collide with the spec-retirement sibling | minor |
