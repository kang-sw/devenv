---
title: Workflow Skills
summary: Codex-facing ws lead skills for planning, implementation routing, documentation reconciliation, reconstruction, utilities, and host-neutral workflow primitives.
---

# Workflow Skills

The ws workflow skill set gives Codex users a host-neutral project workflow for
discussion, specification, ticketing, skeletons, implementation, review,
documentation updates, and release. The Codex-facing surface is
the `lead-*` skill namespace, backed by the ws MCP runtime and embedded delegate
prompts.

## Lead Skill Namespace And Surface {#260505-lead-skill-namespace-surface}

The Codex plugin exposes workflow skills under the `lead-*` namespace. Skills
are invoked as plugin skills such as `ws:lead-discuss`,
`ws:lead-write-ticket`, or `ws:lead-implement`.

The namespace covers planning, implementation, reconstruction, utility, and
workflow-reference roles:

```text
lead-add-rule
lead-backfill-docs
lead-bootstrap
lead-discuss
lead-drain-ready-queue
lead-forge-mental-model
lead-forge-spec
lead-goal-fan-out-step
lead-implement
lead-check-blockers
lead-proceed
lead-review
lead-scope-worktree
lead-ship
lead-tune
lead-update-spec
lead-verify-discussion
lead-workflow-manual
lead-write-spec
lead-write-ticket
```

Skill descriptions provide the natural-language trigger surface for Codex.
Descriptions distinguish strong top-level entry triggers from lighter
derived-stage triggers so Codex reliably invokes workflow entry points without
overmatching internal pipeline stages.
{#260508-skill-description-attention-policy}

A second trigger class matches the session's own observation rather than a user
request. `mcp-server-repair` is the case: no user types it, and the condition
that should fire it is an environment state the agent has already noticed. Such
a description is written in the vocabulary the agent emits when reporting that
failure — the observable states it would name, phrased as a state declaration
rather than an authoring-side condition clause — and names the moment of
substitution, so the skill displaces the failure report the agent was about to
write instead of competing with it. Descriptions in this class carry no
restatement of body content: what the skill does on invocation is the body's
job, and selection-surface budget buys trigger match only.
{#260806-skill-description-self-invocation-trigger}

The directly invocable surface is narrowed to 14 entry skills the user invokes as
`/ws:<name>` — `lead-discuss`, `lead-proceed`, `lead-review`,
`lead-ship`, `lead-bootstrap`,
`lead-add-rule`, `lead-forge-mental-model`, `lead-forge-spec`,
`lead-verify-discussion`, `lead-tune`, `lead-drain-ready-queue`,
`lead-goal-fan-out-step`, `lead-backfill-docs`, and `lead-scope-worktree`.
`lead-scope-worktree` always discusses what this worktree's work line or
topic is before writing any `git sparse-checkout` pattern — it never derives
a pattern from inference. Its derived scope covers `ready/`, `todo/`, and
`idea/` uniformly (no status directory is exempt); see
`#260810-git-commit-sparse-staging` for the `.gitkeep` and `--sparse`
capture-staging mechanism. The remaining
procedures — `lead-implement`, `lead-write-ticket`, `lead-write-spec`,
`lead-workflow-manual`, `lead-check-blockers`,
and `lead-update-spec` — are internal procedures served as `ws/playbook.print`
content invoked by caller skills, not directly user-invoked entry points;
`lead-write-ticket` and `lead-write-spec` are orchestration-only. The
classification axis is whether the user is meant to type `/ws:<name>` directly, not
cross-skill invocation count. Each entry skill's own procedure body is likewise
served from a `ws/playbook.print` playbook behind a thin trigger shim: the SKILL.md
surface carries only the trigger description and delegates execution to its
playbook. Context-heavy entry skills (lead-discuss) are an
exception: their SKILL.md carries a parallel init declaration —
`playbook.print` plus `workflow_manual` called in parallel — rather than a pure
routing stub, reducing init round-trips from 4–5 serial calls to 2 parallel rounds.
{#260610-entry-skill-surface-reduction}

`lead-tune` is the umbrella workflow-tuning entry skill: its description is the
runtime trigger surface that fires when the user signals intent to tune how the
workflow runs (delegation posture, mercenary-vs-native delegation, model tiers),
so the skill can proactively propose a tune. Its playbook is the tuning manual —
it loads the `config.list` catalog (`#260625-tuning-catalog`) and uses that
catalog's knob ids, the `config.tune` write contract, field options, and current
values to drive prompt overrides (`#260620-config-prompt-override-tuning-tools`,
including `UserPreferenceSection` for standing preferences), workflow preference
knobs, and the `agents.tier` knob without reimplementing their set paths. The
always-on `lead-workflow-manual` carries only a one-line pointer, keeping tuning
guidance out of general-task routing attention. In agentless wsflow the catalog
omits the full-ws-only knob `workflow.prefer_mercenary`, while keeping shared
knobs such as `workflow.prefer_subagent` and `agents.tier` (now a shared knob
available in both product modes).
{#260619-lead-tune-workflow-tuning-skill}

## Workflow Primitive Reference {#260505-workflow-primitive-reference}

`lead-workflow-manual` is the shared primitive reference for writing or executing ws
workflow skills. It defines host-neutral notation: `ws/<tool-name>` means an MCP
tool on the `ws` server, while `ws:` names plugin skills.

When global `"workflow.prefer_subagent"` is `on`, loading
`lead-workflow-manual` also loads the `lead-prefer-subagent` posture inside an
XML-style `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`
boundary. The appended text is the static body of
`agents-plugin/skills/lead-prefer-subagent/SKILL.md`, read directly via
`LoadSkillBody` with no override-marker pass and no per-harness runtime
branch: Claude and Codex both see the same host-neutral posture prose.
Explicitly invoking `lead-prefer-subagent` may duplicate this short posture
text; that duplication is accepted.

Under this maximum-delegation posture the lead delegates every payload to a
fresh, self-contained subagent by default. Two carve-outs qualify that
default. Authoring or mutating a durable artifact (ticket, spec) stays with
the session that already holds the authoritative context for the decision —
the lead when it was settled in the lead conversation, or the delegated
subagent's own continuing session when settled there — never a separate fresh
spawn working only from an after-the-fact summary. Separately, an
already-spawned delegate's own session is continued when the instruction is
the same work item it already owns; a new work item, or a judgment that must
not inherit the prior agent's conclusion, still opens with a fresh spawn. The
earlier context-inheriting fork delegate and its Codex `spawn_agent`
fork-fallback wording were removed, so no delegate inherits the lead's
conversation.
{#260724-prefer-subagent-fresh-spawn-delegation-posture}

Shared skill text uses ws MCP primitives for agent orchestration, scoped
queries, generated artifact paths, runtime metadata, workflow discovery, Git
operations, API documentation lookup, and project/convention reads. Skills name
only primitives available in the runtime; when a needed surface is not exposed
yet, skill text describes the required MCP contract instead of naming a
host-specific helper.
Prompts sent to `ws.mercenary.call` and wsflow subagents are
written in English so delegated work products stay consistent with English
AI-authored repository artifacts.

Workflow primitive guidance must not name retired agent-backed API documentation
ask tools for external API documentation lookup. Until a future pure-tooling
`api.*` namespace is designed, skills should route dependency/API documentation
questions through scoped native exploration with official-source citation and
staleness caveats. `ws/api.list` remains available only for local cache-domain
discovery.

Codex-facing workflow skill guidance presents MCP primitives as the primary ws
runtime surface. Promptless `ws.mercenary.register(name: "<agent-name>")` is the
general-purpose named-agent form; role-specific delegates obtain a self-contained
prompt from `ws/playbook.render` and run natively by default. For Codex-native
dispatch, the lead passes render-returned `recommended-model` as
`spawn_agent.model` and optional `recommended-reasoning-effort` as
`spawn_agent.reasoning_effort` — the parameter is `reasoning_effort`, not
`effort`. The lead omits absent bindings, spawns the self-contained rendered
prompt with `fork_turns: "none"`, and, if a binding is rejected, reports the
rejected field and value rather than claiming it was applied. Shipped skill text
shows only these concrete dispatch operations, not harness-selection or
binding-resolution rationale. Delegate prompt bodies do not repeat their own
model alias; binding metadata stays lead-facing and is applied before spawn. The
removed
`prompts: ["<prompt-stem>"]`/`prompt_refs`/`model` register fields no longer
appear in shipped skill text. CLI adapter syntax belongs only in compatibility
or testing references. {#260507-mcp-centric-workflow-language}

Scoped fact-finding delegation uses host-native exploration workers rather
than `ws/subquery`: shipped skill text delegates scoped exploration directly to
a native worker with an English prompt that includes the scoped question or
purpose-specific query block, and requires cited evidence, gaps, and follow-up
needs in the returned report. The async fire-and-forget plus deferred-result
shape maps to native background subagents. Native delegation is the default, not
the exclusive path — the lead-invokable mercenary surface remains available for
stateful named work and bundled delegate prompts. The generic `explore` render
playbook remains a compatibility/fallback artifact for unknown or unsupported
harness contexts, but it is not the normal scoped-exploration path. The
`ws/subquery` runtime tool has been removed; ordinary scoped exploration now
uses host-native exploration workers directly.
{#260610-subquery-explore-delegation-shift}

Workflow guidance prefers `model` for both portable aliases and concrete
overrides. Examples use `model: "core"` or `model: "deep"` for portable
selection and concrete provider names such as `gpt-5.6-terra` or
`claude-sonnet-4.6` only when backend-specific routing is intentional. `tier`
remains documented only as deprecated compatibility input.
{#260508-workflow-model-alias-guidance}

Workflow skill-to-skill handoffs share the active conversation; the receiving
skill reads context from the conversation, not from a caller-emitted carry
block. User-approval gates in skills fire only when the user invokes the skill
directly; chained invocations re-ask only for safety, deletion, or explicit
consent rules. Argument language is reserved for MCP tools, CLI commands, and
structured templates. Dense routing or rule lists use Markdown hierarchy, named
groups, fixed lookup tables, and command-shaped lists before introducing custom
notation. Skill, agent, and prompt edits run a fresh-reader audit through a
separate fresh reviewer, such as an agent or subagent, after local reread. The
reviewer receives only the target file or excerpt and is instructed to read only
that target, not other files, skills, docs, prior conversation, rationale, or
host-generated metadata. Fresh-reader findings flag awkward, surprising,
context-dependent, underspecified, contradictory, duplicated, orphaned, or
missing end-state/output wording, and each finding includes the quote, issue,
severity, and either a suggested rewrite or a suggested deletion. The lead
classifies each finding as fix, intentional difference, or out of scope, edits
only fix findings, and runs at most three audit/revision cycles.
Doctrine, terminology, route, layout, and audit-gate edits also run a
downstream consistency sweep across affected skill, prompt, spec, mental-model,
test, and mirrored-package surfaces. The first pass may conservatively
over-report findings; the lead classifies each as fix, intentional difference,
or out of scope before editing.
Dense handlers use sub-blocks only when structure improves execution, such as
when a handler exceeds four steps and mixes responsibilities. Sub-block names
describe the responsibility they perform; single-purpose checklists are not
split only because they are long. Compact checkpoint skills may stay prose or
short lists when output and end state are obvious.
These authoring rules are maintained as `ai-docs/manuals/skill-authoring.md`, an
upstream reference document read directly rather than a shipped invocable
skill; the audit they describe covers `agents-plugin/skills/*/SKILL.md` and
`agents-plugin/rsrc/lead-*/lead-*.md`.
{#260514-skill-authoring-carried-context}

## wsflow Skill Surface {#260513-wsflow-agentless-skill-surface}

The wsflow distribution ships a curated subset of lead workflow entry skills
under `wsflow:lead-*` invocation names and `wsflow/<tool>` MCP notation.
Shipped wsflow skills include planning, documentation, direct implementation,
bootstrap, release, verification, and reconstruction workflows:
`lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
`lead-write-ticket`, `lead-proceed`, `lead-implement`,
`lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-ship`,
`lead-verify-discussion`, `lead-check-blockers`, `lead-forge-spec`,
`lead-forge-mental-model`, `lead-review`, and `lead-backfill-docs`.

The wsflow package excludes skeleton flows: `lead-write-skeleton`.
Shipped wsflow `SKILL.md` files are thin entry shims:
they keep package-local bare `name: lead-*` frontmatter, call
`wsflow/playbook.print(name: "<lead-name>")`, execute the returned procedure
against the current user request, and report a blocker if the playbook cannot
load. Procedure behavior lives in shared rsrc playbooks rendered in wsflow
product mode, not in separately curated wsflow skill bodies.

A change to a shared rsrc lead playbook that affects wsflow-visible behavior
must still evaluate wsflow product-mode output, static verification, and the
wsflow exclusion rationale. The wsflow skill-bundle verification path checks
inventory, forbidden full ws agent references, thin-shim shape, and shared
playbook coverage; it does not require text identity with full ws skill shims.

wsflow bootstrap emits a package-neutral downstream artifact converged with
the full ws package: `AGENTS.template.md` and `WORKFLOW.md` produce identical
emitted output across both packages modulo the shared
`<!-- Template Version: vNNNN -->` tag, and both packages share one
migration-ordinal lineage (wsflow no longer runs a separate `v0001..v0008`
counter). Bootstrap behavior changes remain mirroring-sensitive: maintainers
check both packages and bump the shared template version once for a change
either package receives.

## wsflow Converged Implementation Spine {#260529-wsflow-converged-implement-spine}

wsflow `lead-implement` adopts the same unified implementation spine as ws
`lead-implement` (route, verdict, prepare, edit, review, documentation, merge
readiness) and absorbs the separate `wsflow:lead-edit` skill, which is removed
from the wsflow skill set. wsflow `lead-implement` becomes the single
source-editing entry point.

The Edit stage is lead-owned: the lead applies direct edits and may delegate
scoped work to a native host subagent at its discretion. wsflow has no
`implementer` named stage, and the `implementer` prompt is not render-eligible
in wsflow. In a one-shot subagent host the implementer loses its multi-turn
fix-relay value, so larger delegation stays lead-discretion scoped native
subagent work instead of a fixed implementer stage.

Survey, plan-population, review, and mental-model documentation stages dispatch
their delegate prompts through wsflow-mode `playbook.render` for the five legacy
render-eligible stems: the lead renders the chosen prompt to a path, hands it to
a native subagent, and integrates the subagent's returned result.
The five render-eligible prompts are `reference-discovery`, `plan-populator-survey`,
`plan-populator-research`, `code-reviewer`, and `mental-model-updater`. The count
is scoped to this implement spine; render-eligible prompts introduced by later
non-spine flows, such as `doc-gap-discovery`, are additional to it.

ws `lead-implement` and the ws named-agent delegation path are unchanged by this
convergence; the change is wsflow-local.

## Planning Workflow Skills {#260505-planning-workflow-skills}

Planning skills prepare caller-visible work before implementation.

`lead-discuss` explores a topic without editing source code. It loads project
context, uses scoped exploration workers when search is needed, can promote or
move tickets when the discussion reaches an actionable state, and recommends an
appropriate next workflow step. Discussion responses use the user's active
conversation language. When the user explicitly wants implementation to start,
`lead-discuss` invokes `lead-proceed` instead of routing directly to
`lead-implement`.

Discussion replies keep the load-bearing point, the evidence or gap behind it,
and the user decision or next action adjacent. The skill favors a concise stance
with the strongest caveat over exhaustive option dumps, and labels incomplete
evidence instead of presenting inference as established fact.

When a discussion answer depends on a documented decision, prior rejection,
architecture fact, or cross-ticket constraint that is not loaded, `lead-discuss`
searches the ticket/spec/mental-model cascade before answering. Commit history is
an additional project memory tier: `## AI Context` bodies carry decision rationale
that docs may not yet reflect; `lead-discuss` accesses this tier through
Explore-type subagent dispatch rather than inline reads. Migration topics such as
plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries
load the native-subagent pivot anchor before the lead states a direction. If the
cascade has no documented answer, the reply says that before making an inference
or proposing the next lookup.

For proposal, evaluation, design-direction, causal-claim, scope-assumption, or
trade-off-heavy user messages, `lead-discuss` frames the reply around a visible
premise-aware intent summary before giving advice. The frame uses symbolic
bracket labels so the user's active conversation language carries the prose. It
keeps bullet structure for the initial reading, then names implicit premises
with failure conditions, reframes the topic as a neutral decision problem, lists
considered and dropped options, and ends with a stance. If a decision branch
remains open after that frame, the skill interviews through the highest
unresolved branch first, descends only after parent decisions settle, and returns
to the nearest unresolved parent when the user delegates lower-level detail.
{#260510-discuss-intent-frame-interview}

`lead-write-spec` writes or updates behavioral spec entries for caller-visible
behavior. It reads spec conventions, generates stable spec stems, writes
implemented entries according to the current behavior, verifies the spec index,
and commits the spec update.

`lead-write-ticket` creates or updates workflow tickets. It treats `todo/` as
accepted backlog and `ready/` as the spec-addressed implementation-ready
status. The spec-address gate runs only when a non-`epic`, non-`research`,
non-`workset` action creates or moves a ticket into `ready/`; `todo/` tickets
may carry optional `spec:` links as recovery hints. For `ready/` creation or
promotion, `lead-write-ticket` accepts confirmed `spec:` or `spec-remove:`
stems, or a ticket-local `## Spec Impact` section naming the target spec area
and the expected caller-visible change. It never invokes `lead-write-spec`;
spec addressing runs through `spec:`, `spec-remove:`, or `## Spec Impact`. It
stops when no stem or `## Spec Impact` can address the work, or the behavior is
too underspecified to spec.

Discussion-derived ticket persistence is consent-gated. Before ticket cleanup
writes mechanism decisions, rejected alternatives, future-scope hints, Result
Forward notes, focus "Next" lines, or note/comment proposals, `lead-write-ticket`
builds a visible Open Decision Queue, asks whether to persist the discussion
when persistence was not already approved, resolves the queue as a single batch
interview, and writes only user-confirmed items. Rejected, deferred, unanswered,
or otherwise unconfirmed items are omitted unless the user explicitly approves
recording their status; omission governs what gets written and never licenses
leaving the queue unresolved.

Queue conveyance does not depend on how a host renders the visible queue. Each
queued item's visible text is the decision itself rather than a label, and any
secondary note or description field is optional detail that may not render, so it
never carries load-bearing content; the same rule applies to the Markdown
checklist used when no task-list surface exists. Recommendations stay out of the
visible queue for the same reason. One response restates every open item's full
text in the response body, so the questions the user is answering stay legible
even when the visible queue renders partially or not at all.

Each restated item carries the skill's recommendation for it, as a proposal and
never a default. An item the user's answer does not reach stays open and returns
in a follow-up batch, and no round limit converts a still-open item into a
disposition the user never gave. Where an answer's reach over an item is unclear,
the skill states its reading on its own line and continues instead of re-asking,
so the user's brake is a visible interpretation rather than an added confirmation
turn.
{#260727-odq-item-conveyance-restate-in-body}

`lead-write-ticket` preserves epics as lightweight milestone boards. When
detailed discussion, implementation phases, or phase-specific decisions arise
while editing an epic, the skill creates or updates child tickets instead of
expanding the epic body; a single child ticket may carry multiple phases when
they form cohesive sequential reviewable implementation slices rather than task
checklists. A single-slice ticket uses `Phase 1`. Actionable child-ticket phases
are authored from a fresh-session completion view: each phase describes the next
complete behavior a future `lead-proceed` run can finish, review, verify, and
hand off cleanly in one plan/implement/review/verify loop. Setup, API, UI,
tests, skeletons, and investigation are phase ingredients unless one is the
reviewable deliverable. Additional phases are added only when review,
verification, rollback, or dependency boundaries differ. Each non-epic
actionable phase states what behavior is complete, what remains deferred, and
what verification proves the phase complete.
{#260508-write-ticket-epic-child-boundary}

`lead-write-ticket` preserves worksets as non-hierarchical operating-context
boards. A workset lists tickets gathered for a session, goal, sprint, or
temporary focus area by stem or path with status and role; planned-but-not-created
items go under `## Planned References` with provisional labels and creation
conditions, not status or path. Inclusion never changes `parent:` relationships
and does not let the workset own decomposition, cross-child invariants,
implementation phases, or spec-ready behavior. When implementation detail or
settled constraints arise while editing a workset, the skill moves them into
the relevant included actionable ticket or phase. Worksets normally stay in
`idea/` or `todo/` rather than the `ready/` implementation-ready status.
`lead-proceed` stops on workset paths and asks the user to choose, create,
promote, or proceed an included actionable ticket instead of treating the
workset as an implementation target. {#260524-workset-workflow-skill-routing}

`lead-write-ticket` treats tickets as recoverability artifacts before compact
summaries. Non-epic actionable tickets preserve caller-visible contracts,
constraints, rationale, rejected alternatives, forward-compatibility guardrails,
verification expectations, agreed strategy that constrains implementation,
phase dependencies, and agreed API/type/event/UI sketches. Source-local edit
notes are excluded unless they are settled constraints; settled local or
cross-ticket decisions stay in the relevant child ticket or phase.
Tickets capture enough settled detail for a fresh implementation session to
recover the intended product, workflow, API, and verification contract without
inventing missing decisions. Intent review checks whether the ticket permits a
materially different caller-visible, workflow, API, or verification result
without contradiction and captures the missing settled decision when it does.

`lead-write-ticket` reviews related-ticket decisions by default when
creating or editing an actionable ticket. It inspects the target's parent,
containing epic, containing workset, child board, explicitly related tickets,
and available active siblings only far enough to find settled decisions that
constrain the current implementation slice. It records only binding decisions
in the target as scope, constraints, forward-compatibility guardrails, rejected
alternatives, verification expectations, or phase dependencies, and avoids
copying unrelated future-phase detail. Explicit "cascade" requests, board
organization, or parent and child edits broaden this into a multi-ticket
propagation pass: the skill identifies the impacted graph, selects only affected
edit targets, keeps epic edits board-level and workset edits operating-context
only, updates active inventory when needed, and commits the propagation as one
logical documentation unit. It does not promote propagated tickets to `ready/`
unless the user explicitly requests ready promotion or routes through
`lead-proceed`.
{#260516-write-ticket-related-ticket-propagation}

Skill-authoring guidance treats local shorthand as trigger examples for a
general intent, not as the concept name itself. New workflow shorthand should
name the broad intent first and list the shorthand only where it prevents
repeated routing failures.

`lead-verify-discussion` gives users an explicit lightweight verification and
validation checkpoint during discussion. It checks the current assumptions or
structure choices through scoped named-agent surveys, searches for already
implemented items that can be reused or merged to avoid duplication, synthesizes
corrected assumptions, observations, reuse opportunities, and code-hygiene
findings, checks for over-alignment signals such as weak premise handling or
missing countercases, then steers the discussion toward the best-supported
direction. When the user specifically asks to verify a design's validity, it
also dictates the concluded design in full — the hypothesis under review,
rejected alternatives, and paths to already-read evidence files — to a fresh
higher-tier subagent and folds that subagent's independent judgment into its
recommendation.
It intentionally remains compact and frequent-use; downstream authoring sweeps
must not force full workflow-skill ceremony onto this checkpoint unless its
actual output or end state is unclear.
{#260512-discussion-verification-skill}

`lead-drain-ready-queue` drains the `ready/` queue: one invocation selects
and dispatches exactly one ticket from `ready/`, the sole progress gate —
nothing advances until a ticket reaches `ready/`. It does not poll or
repeat internally, so draining the whole queue is repeated invocation, for
example a standing `/goal` directive whose Stop-hook re-invokes the skill
each turn until nothing advanceable remains. Invoked without an active goal
run it is a single-cycle shim: one ready ticket handed to `lead-proceed` as
an explicit target, so the caller does not depend on `lead-proceed`'s own
target-from-conversation routing to guess which ticket is meant.

The skill's name and its body address **different readers, under separate
contracts**:

- *Body layer — read by the main agent.* It carries the full goal-run
  posture: the terminal states, `goal/*` staging, ticket-curation authority,
  blocker recording. The entries below specify it.
- *Name layer — read by the `/goal` Stop-hook's continue-vs-stop judge.* That
  judge sees the skill name plus the transcript up to the stop, never the
  skill body, and runs on a weaker model than the main agent. The name must
  therefore supply a termination test the judge can resolve by lookup rather
  than infer: the named queue is either empty or it is not. The name encodes
  the invariant process shape only and never the terminal set — terminals are
  added over time, and a name enumerating them becomes a lie that misleads
  the judge worse than a silent name does.

An earlier revision of this entry asserted the opposite mechanism: that the
hook reads the body rather than the name, and that the name does not drive
loop behavior. That claim was retracted on 2026-07-30 after observation of
live goal runs. It is recorded here so it is not reintroduced.

Because the judge is weak, the skill does not leave the continue-vs-stop call
to it. Every turn ends with one of two fixed lines as its final line, with
deliberately disjoint vocabularies and an imperative ending clause:

- Continuing: `Ready queue still has advanceable tickets — next cycle: lead-drain-ready-queue.`
- Ending: `Goal run finished — <reason>. Do not re-invoke lead-drain-ready-queue.`

Nothing may follow that line — a wrap-up placed after it is what the judge
reads last — and `finished`, `complete`, and `done` stay out of a continuing
turn entirely.
{#260723-lead-goal-step-rename-reposition}

Ticket selection is itself delegated, not done by the lead: the skill
spawns a light-tier Explore-style subagent to list `ready/` and, among
advanceable candidates, prefer in order: a ticket already in progress —
one whose body has a `### Result` on at least one phase but still has a
phase without one — over untouched tickets; then a candidate named as a
prerequisite in another ready ticket's `related:`/`parent:` frontmatter
when that referenced ticket is also in `ready/`; otherwise the oldest
date-prefix ticket (FIFO). It returns exactly one ticket path (or reports
the queue empty). The in-progress preference is a soft ordering tier, not
a hard gate: one drain cycle advances a single phase, so a multi-phase
ticket stays in `ready/` between cycles, and without this tier FIFO could
bounce between started tickets each cycle and leave phases half-done.
{#260725-goal-step-in-progress-ticket-affinity} The lead never
lists `ready/` or reads ticket files itself for this step. If the
subagent reports `ready/` empty, the lead stops with no handoff. This
inspects only existing free-text `related:`/`parent:` annotations — no new
structured dependency field is introduced.

Deliberately kept minimal: this is a purely user/`/goal`-invoked shim, not
a heavier discussable workflow skill, so beyond delegating selection its
body also directs the lead to delegate everything else for the
invocation — including simple tasks like commits — to a subagent per
`lead-prefer-subagent`, conserving lead context across a long-running
goal, rather than restating that posture's body. The skill's body is
inlined as static text directly in
`agents-plugin/skills/lead-drain-ready-queue/SKILL.md` (no rsrc playbook, no
`playbook.print` indirection), matching the `lead-verify-discussion`/
`lead-prefer-subagent` inline-body shape, and is mirrored byte-identically
into `agents-plugin-wsflow`.
{#260703-drain-ready-queue-skill}

The step also carries explicit lead ticket-curation authority: as part of
advancing the goal the lead may autonomously edit existing tickets (record
findings, restructure, re-triage status) and create + link new tickets via the
normal ticket-write path (`lead-write-ticket`), within the goal-run autonomy
bounds above. In-scope bug capture rides this authority: a bug found mid-run
that blocks or is directly relevant to the current goal is promoted to
`ready/` (via the ticket-write path, so the sage ready-landing gate is not
bypassed) for a later loop iteration to fix; an incidental/unrelated bug is
captured at `idea/`; an explicitly deferred bug is captured only, not queued
to `ready/`. This routing is skill-intrinsic and is judged separately from
any downstream project's own dogfood-capture convention. No new
ticket-system state field is introduced — a recorded blocker and a captured
bug are both ordinary ticket edits.
{#260723-goal-step-ticket-curation-authority}

`lead-drain-ready-queue` adds goal-branch staging on top of the base
single-cycle shim above, activated only when the lead itself observes both
an active `/goal` Stop-hook reminder in the current turn and a current
branch that is not already `goal/*`; the ticket-selection subagent stays
unaware of this and is not asked to detect it. When active, the lead
captures the current branch as PARENT (the fork point) via `git
rev-parse --abbrev-ref HEAD` — a detached-HEAD guard aborts staging-branch
creation with a clear message instead of producing `goal/HEAD/<slug>`
when that command returns literal `HEAD` — then derives an arbitrary
random branch-safe slug (never from the goal text, to avoid collisions
across concurrent goal runs) and creates/checks out
`goal/<parent>/<slug>` with plain `git` commands before
dispatching the selected ticket, then hands off to `lead-proceed` with
`policy.branch.merge_confirm: "skip"` supplied as explicit caller policy —
`lead-implement`'s existing Route step 3 ("explicit caller policy")
consumes this without any goal-specific change on that side, and no
`merge_target` override is needed since the create-path already derives
the merge target from the checked-out branch. Each ticket still gets its
own `impl/<goal-branch>/<stem>` branch (the create-path resolver
automatically encodes the checked-out `goal/<parent>/<slug>` branch as the
merge root), merged into `goal/<parent>/<slug>` without an approval ask and
auto-deleted per the Branch Cleanup naming-gate behavior (see the
`impl/<merge-root>/<stem>`-branch section above). When the selection subagent
reports `ready/` empty while the current branch is `goal/<parent>/<slug>`,
the skill performs the run's one confirmed final merge itself in its own
prose: derive PARENT and SLUG from the branch name by stripping the
`goal/` prefix and splitting on the last `/` (old-format single-segment
`goal/<slug>` falls back to `main` as the merge target), ask the user for
explicit approval, then `git merge --no-ff goal/<parent>/<slug>` into
`<parent>` (or the `main`-fallback equivalent) — rather than routing
through `lead-proceed`/`lead-implement`, because `enter.implement`
requires a ticket target this ticket-less step has none of. This override
never extends to push or remote actions for either the per-ticket or the
final merge. Outside an active `/goal` context, or once off a `goal/*`
branch, the skill reproduces the pre-staging behavior exactly: no branch
creation, no `merge_confirm` override, and no new persisted state —
"currently checked out on a `goal/*` branch" is the entire signal.
{#260707-drain-goal-branch-staging}

The goal run gains a second clean terminal beside "`ready/` queue empty": a
blocked-progress conclusion. When every remaining `ready/` ticket is
blocked — no path can advance without a human decision — the step concludes
the run with an explicit blocker report instead of looping (otherwise the
`/goal` Stop-hook re-surfaces the reminder and the loop thrashes waiting for
an away human). This conclusion never runs the empty-queue completion's
merge-approval flow — merging here would misrepresent unfinished, blocked
work as a completed goal. This stays distinct from a hard-gate pause: the
discriminator is "is there any work I could still do without the human?" —
work remains with only a final irreversible/destructive action awaiting
sign-off → pause; no advanceable work remains anywhere → conclude; a
hard-gate pause must never be reclassified as goal-complete through this
conclusion. A ticket the lead cannot advance is recorded as blocked on the
ticket itself (an ordinary edit — e.g. sage's rendered Blocked section, or a
recorded blocker note) before the turn yields — this record-before-yielding
step is not skippable, since an unrecorded blocker causes the next turn's
selector to re-pick the same stuck ticket — and the selection step reads
ticket state/bodies to judge "advanceable now" and skips it, rather than the
body-blind FIFO pick; so one blocked ticket does not terminate the run while
others remain workable. {#260723-goal-step-blocked-progress-conclusion}

`lead-goal-fan-out-step` is a batch-parallel `lead-drain-ready-queue` variant: instead
of dispatching one advanceable ticket at a time, it selects a
mutually-independent batch (disjoint edit surface, no `related:`/`parent:`
ordering between candidates, excluding tickets already dispatched this run and
not yet merged) and advances each in its own worktree-isolated mini-lead in
parallel, one `impl/<goal-branch>/<stem>` branch and background native dispatch per ticket,
merging each serially back into the parent as its mini-lead reaches its merge
gate. It degrades to the plain serial path — one ticket, dispatched directly
to `lead-proceed` — whenever fewer than two independent tickets are available
or recursive native dispatch is unavailable this cycle. It does not restate
`lead-drain-ready-queue`'s contract: the goal-run posture, the three terminal states,
`goal/*` staging, blocker-recording before yield, and the
one-step-is-not-a-finished-goal continuation all govern unchanged, delivered
verbatim at serve time via the `printPlaybook` transclusion mechanism
(`#260724-serve-time-skill-body-transclusion` in `plugin-runtime.md`) rather
than duplicated prose: serving `lead-goal-fan-out-step` always appends the
rendered `lead-drain-ready-queue` skill body inside a visible `<playbook
name="lead-drain-ready-queue" title="Drain Ready Queue">` boundary, unconditionally (no
config-flag gate, unlike the `lead-prefer-subagent` precedent this mechanism
generalizes). The overlay itself — batch selection, worktree/mini-lead
dispatch, board bookkeeping via `session.note`/`session.children`, and serial
merge — lives in `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`
(a normal `kind: print` rsrc playbook, unlike `lead-drain-ready-queue`'s inline
SKILL.md body), fronted by a thin `SKILL.md` shim shaped like `lead-discuss`'s.
{#260724-goal-fan-out-step-transclusion}

`lead-verify-design` is removed; its `SKILL.md` and rsrc playbook were deleted
entirely (delete-don't-diet decision, `260630-epic-skill-playbook-diet`). Its
premise-gated design-verification function is now covered by the ticket
lifecycle's Sage Review Gate (`260624-sage-review-gate`), which dispatches
`ticket-reviewer-design` automatically at `todo/`→`ready/` promotion, plus the
conditional independent-judgment step added to `lead-verify-discussion`. Do
not route new work through it.
{#260524-design-verification-skill}

`lead-write-ticket` runs a Ground stage between its intent-verification step and
the Sage Review Gate, gated on the ticket body asserting anything a reader of the
tree can check. It dispatches `ticket-fact-populator`, a read-only delegate that
checks the ticket's claims against the tree and returns corrections, decision
gaps, and unverified claims — every correction carrying the evidence it read.
The delegate never writes the ticket and never settles a decision: the lead
applies the corrections itself, and routes decision gaps to the Open Decision
Queue rather than resolving them from the delegate's evidence.

The same delegate carries the corpus checks, because it is the cheapest point in
the procedure that can make them without loading tickets into the lead's context:
one `tickets.list` call shortlists tickets whose title or unresolved phase titles
cover work this ticket also claims, and supplies the current status of every
ticket this one names as a blocker, predecessor, or landing-order constraint. A
real overlap and a dependency sitting behind this ticket's landing status are both
decision gaps. It returns those statuses as a `relations` fact table that stays
complete even when nothing about it is wrong; the lead passes the table to the
design reviewer, which takes each entry as landed. That inverts what the reviewer
does with an unlanded premise: one the table accounts for is a sequencing fact,
and only one it does not account for is a design defect the ticket failed to
declare. A claim about state that a named unlanded ticket or an unfinished earlier
phase will create is never a correction, however clearly the tree contradicts it —
applying such a correction would damage a ticket that is right about its own
future. The stage exists
because the authoring step reads only tickets, conventions, and routed
spec/mental-model checks, while the design reviewer sketches an implementation
from that text — an asymmetry that made design review the first stage to touch
the tree, and made unverified authoring claims arrive there as blocking findings
the lead then had to research by hand. `idea/` landings and pure status moves
skip the stage, matching the Sage Review Gate's own `idea/` skip.
{#260729-write-ticket-ground-stage}

Both ticket reviewers cut `resolution` on policy, not on discovery cost: a gap
planning or implementation can settle is `autonomous` however expensive the
lookup, and only a policy choice — what the system should do, what contract it
commits to, or which of several defensible shapes is correct — is `missing`. A
severity floor on the block threshold was rejected as the alternative; it is a
proxy that would leak genuine minor-severity policy gaps through while still
blocking on cheap lookups. `ticket-reviewer-design` may additionally read a
source file at a path the ticket itself cites, and only to check a claim the
ticket makes about it, so it can spot-check the populator's citations without
becoming a second surveyor.
{#260729-ticket-reviewer-policy-resolution}

`judge: initial-status` and every `todo/` → `ready/` promotion defer to one
gate, `## On: Dependency Closure Check`: a ticket lands in `ready/` only when the
tickets its earliest unfinished phase block-depends on are already in `ready/`,
`.done/`, or the same bulk-promotion action. `ready/` is a **closed work front**
that drains in dependency order — a dependent may sit in `ready/` beside its
prerequisite — not a set of tickets each startable in isolation; this relaxes an
earlier bar that refused `ready/` until every dependency had landed in `.done/`.
The gate is cut on the earliest unfinished phase, because a ticket whose first
phase is independent is startable regardless of what a later phase waits on, and
a blocking dependency counts only through a machine-readable
`related: <stem>: prerequisite` or prerequisite `parent:` edge — never a
prose-only mention or an epic-hierarchy `parent:`. `judge: initial-status` still
fires before the Ground stage from what the lead has already read; the
populator's dependency-status decision gap remains the backstop, reaching the
user through the Open Decision Queue, and is a separate layer from
`lead-drain-ready-queue`'s dispatch-time `## Blocked (` skip.

Two or more tickets entering `ready/` in one action route to `## On: Bulk Ready
Promotion` (`judge: bulk-ready-promotion`), which promotes them
prerequisites-first — so each closure check passes against an already-promoted
prerequisite — and commits the tickets that landed as one unit. It is
deliberately separate from Cascade Edit, whose selection is
decision-propagation-scoped; Cascade Edit itself now runs the Sage Review Gate,
not only the Spec-address Check, per `ready/`-entering target.

Epic detail belonging to a child that does not exist yet is recorded as the epic
skeleton's `- Planned:` entry rather than deferred to a separate invocation.
Without the branch, creating a new epic — where no child exists by construction —
either stops the invocation inside the Populate step, leaving the epic's own body
edits unverified and uncommitted, or drops the constraint.
{#260729-write-ticket-unlanded-dependency-status}

### Check Blockers Checkpoint {#260513-check-blockers-skill}

`lead-check-blockers` gives users a frequent spoken checkpoint for deciding
whether a design discussion still has user-blocking blockers. It does not edit files. It
classifies remaining work into user-blocking design questions, ticket or spec
capture gaps, autonomous code-hygiene items, and proceed readiness.
It intentionally remains compact and frequent-use; downstream authoring sweeps
must not force full workflow-skill ceremony onto this checkpoint unless its
actual output or end state is unclear.

`lead-write-skeleton` is deprecated from normal implementation routing. The
skill file remains available for compatibility, but `lead-implement` no longer
invokes it and absence of ticket `skeletons:` frontmatter does not create a
skeleton obligation. {#260510-skeleton-contract-populator-flow}

`lead-implement` delegated mode absorbs the useful skeleton role through plan authoring.
For public interface, cross-module boundary, or new type contract changes, the
generated plan carries the relevant ticket contract, expected files or modules,
public types/functions/handlers/tools when applicable, reusable mechanisms,
forbidden temporary/fallback/mock-data wiring, implementation steps, and
verification expectations. Implementers treat the plan as the execution
contract, and reviewers compare the ticket, plan, and diff together.
{#260512-skeleton-inside-implement-branch}

Ticket `skeletons:` frontmatter is a backward-compatible legacy artifact map.
Existing entries may still document old skeleton artifact commits, but normal
workflow routing does not create new skeleton artifacts. {#260512-skeleton-draft-and-final-commits}

## Implementation Workflow Skills {#260505-implementation-workflow-skills}

Implementation skills execute code changes and close the documentation loop.

`lead-implement` is the implementation harness. It gathers normalized target,
scope, complexity, risk, and policy facts, calls `ws.enter.implement`, follows
the MCP-authored Implementation Verdict, then runs the shared
post-implementation documentation pipeline before reporting completion. The MCP
verdict owns deterministic implementation labels and branch preflight: it chooses
direct-edit or delegated mode, branch action, delegated survey planning,
review allocation, review need, and documentation mode from facts, policy, and
observed Git state.
`Branch Action: stop` blocks source edits until the missing merge target, unsafe
rename, existing target branch, or tracking ambiguity is resolved. After
verification, `lead-implement` records the phase result commit, closes spec,
mental-model, ticket, and index updates, then asks the user to merge, continue,
or stop. Follow-up changes after this gate route to another implementation slice
or sprint and are captured in tickets as append-only Result editions for already
completed phases.

`lead-implement` is a unified implementation spine with two edit modes.
Direct-edit mode: the lead edits and verifies inline, suitable for single-file
internal-only changes.
Delegated mode: the lead selects ticket authority (ticket plus phase) or inline
authority (a self-contained accepted contract), generates a plan path,
dispatches a planner to write or refine that single implementation plan, spawns
an implementer agent with the plan, and captures the resulting commit range.
Ticket scope facts freeze from the ticket before source reading; inline scope
facts may use the accepted request, loaded context, focused source inspection,
and command output before `enter.implement`. The MCP verdict normally keeps
branch isolation independent of edit mode. One exact low-ceremony exception retains the current named
non-implementation branch for an inline target only when
`policy.low_ceremony_if_safe=yes` and unoverridden facts independently satisfy
automatic direct-edit and automatic lead-only review, documentation is
explicitly skipped with a non-empty reason, and no review override applies.
The policy expresses a preference for a streamlined implementation flow; it
does not require callers to name branch mechanics and does not waive any
eligibility predicate. The lead maps only clear reduced-ceremony requests to
`yes`; urgency or size labels such as `hotfix`, `tweak`, or `small fix` alone
are insufficient. Missing, `no`, or `unknown` policy, caller overrides, unknown
facts, detached or unborn HEAD (no real start commit), ticket targets, and
existing `impl/*` or legacy `implement/*` branches retain the standard isolated branch path. A rejected
`yes` also emits a concise not-applicable warning without changing independently
derived delegation, review, documentation, final-action, or merge behavior. A
successful exception keeps focused verification, one logical explicit-path
commit with `## AI Context`, lead-owned review, and a final report naming
the retained branch and commit range; it omits final-action and merge work and
never pushes.
Automatic direct-edit itself remains limited to single-file internal-only work
with no public symbol, contract, new test-file, or explicit delegation signal.
Initial implementer dispatch is file-first: the lead renders the `implementer`
playbook with plan path, verification hint, result expectations, and
commit-range hint as declared render inputs, then sends the worker only the
rendered prompt path plus the instruction to execute it. The rendered
implementer prompt reads the plan and listed references as the task contract; it
does not read the ticket directly unless the plan's `Escalations` section
explicitly authorizes ticket-file reading. Otherwise the lead updates the plan
before ticket material is needed. Recommended tier remains dispatch
metadata for the lead or transport, not worker-facing task input.

After fact gathering and before preparation or source inspection,
`lead-implement` reads the raw Implementation Verdict returned by
`ws.enter.implement`. The verdict summarizes target, selected scope, branch
action, edit mode, plan depth, review allocation, documentation mode, normalized
conditions, warnings, agenda values, and a concrete `Next:` instruction. The
playbook follows that instruction instead of recomputing deterministic labels,
then treats the replaced todo list as the authoritative executable runbook for
post-verdict branch, prep, edit, review, documentation, final-action, and merge
steps reachable for that verdict; the low-ceremony current-branch verdict ends
with completion instead of final-action and merge. The always-rendered playbook
keeps fact gathering, verdict handoff,
ambiguous execution judgments, and delegate render handoffs, while verdict-specific
direct/delegated, review-allocation, and documentation-skip instructions live in
the focused todo instruction payloads. wsflow mirrors this checkpoint through
the shared product-mode playbook text. See
`#260529-wsflow-converged-implement-spine`.

Review is a single stage for both modes. MCP `review_alloc` picks depth
(lead-only, single reviewer, or partitioned) and partitions (correctness, fit,
test) from independent risk signals. Correctness covers material
correctness/security risk and new contracts or public symbols; fit covers
material fit risk, cross-module work, or reuse uncertainty; test covers
material test risk, new test files, or unknown test surface. Public-interface
surface and existing-test surface do not create partitions by themselves. Zero
or one automatic partition resolves to the delegate-grade generic `reviewer`,
which applies the shared `code-reviewer` contract across correctness, fit, and
test; two or more resolve to partitioned review. Explicit review overrides
remain authoritative. Each
partition carries a default reviewer tier in the
first-class capability vocabulary (`#260612-first-class-tier-vocabulary`) —
correctness `large`, fit and test `medium` — raised for unusually subtle risk.
When a delegate playbook declares its own `tier:`, the `recommended-tier`
returned by `playbook.render` is authoritative for that delegate and the table is
the allocation default. The per-slice review loop budgets by severity rather
than uniformly. Critical is must-fix and bounded to 3 review rounds: review #1,
then — only when review #1 reports a Critical finding — a Critical-scoped
review #2 after relay #1, then, if still non-clean, a second Critical-scoped
relay (relay #2) and a Critical-scoped review #3. A Critical still non-clean
after review #3 is the ceiling, not a hard stop: it unconditionally elevates to
`implementer-elevated` and the run continues to the remaining todos, carrying
the elevation into the final report. Important is best-effort and gets at most
one relay, spent in relay #1 alongside Critical and never re-reviewed; a still
non-clean Important after relay #1 carries the implementer's own
`[not fixed: <reason>]` self-report rather than a re-review verdict. Minor
drives no relay at any point and is recorded in the review summary only. Every
non-clean Critical or Important finding relay #1 dispositions carries exactly
one marker: `[fixed]`, `[won't fix: <reason>]`, `[deferred: <reason>]`, or
`[escalate: <reason>]`; Important additionally has `[not fixed: <reason>]` for
the still-non-clean case above — Critical never gets that marker, since a
still-non-clean Critical instead carries forward into the next Critical-scoped
round. Because `implementer-elevated` is reachable only at the Critical
ceiling, and only for the Critical finding that reached it, the per-slice loop
still does not route to `review-adjudicator`: nothing in this budget reproduces
its contested-finding arbitration trigger ("before the next review", "the next
relay" independent of severity). That playbook remains in the tree but this
loop does not invoke it.
{#260612-reviewer-allocation-tier-default}

Delegates in the review fix-loop are stateless by contract: each implementer and
reviewer dispatch is fed entirely by its relay prompt plus the self-contained
artifact set (plan, review findings, committed diff), and the loop stays
correct when every cycle is a fresh spawn. When the host supports same-agent
resume, `lead-implement` may reuse the prior implementer or reviewer for fix and
re-review loops to reduce latency, but resume never carries required state. Loop
continuity is lead-owned — reconstructed from commit `## AI Context`, not from
agent conversation memory. The implementer records each fix-cycle disposition
(won't-fix or deferred, with reason) inline in the fix commit `## AI Context`.
The reviewer returns a severity-explicit verdict (`clean`, `clean with N minor
remaining`, or `non-clean: M critical/important`); the lead, not a machine gate,
decides clean. The re-review relay carries the prior findings, their
dispositions, a findings output path, and the updated diff; the reviewer writes
full findings, reports the severity verdict, reviews the current diff per its
charter, and is not asked to classify regression-vs-preexisting. The lead
enforces convergence by dedup against the durable disposition record — a settled
finding is not re-relayed, while genuinely new Critical/Important findings and
findings reported fixed that a re-review returns unresolved are. Unresolved
carryover is not a settled disposition: dedup bars re-litigating a decision the
record already carries (won't-fix, deferred, out-of-scope, or an open
escalation), while the severity-graded budget — Important's single relay,
Critical's bounded 3 review rounds — rather than an unbounded cycle count,
naturally bounds reviewer-invented churn, so neither rule suppresses the relay of
a fix that did not hold.
Delegated review-fix relay is file-first: the lead renders the
`implementer-relay` playbook with declared inputs for plan path, review cycle,
current commit range, non-clean review paths, disposition notes, verification
hint, and result expectations. The lead then sends only the rendered prompt path
plus a short execute instruction to the implementation owner. Reviewer findings
remain file inputs, not copied prompt prose.
{#260619-stateless-implement-review-continuity}

Plan population defaults to the survey planner for delegated mode. The same
render contract carries `target_kind`, ticket path/selected phase, inline
contract, and plan path; inactive authority fields are passed explicitly empty.
Ticket mode reads the ticket and selected phase. Inline mode uses the supplied
accepted scope, constraints, non-goals, and verification boundary and never
reads a placeholder ticket path. `plan-populator-survey` clips the selected
authority, explores source, writes a light implementation plan with `Relevant
Ticket Contract`, `Out of Scope`, `Codebase Findings`, `Implementation Plan`,
`Verification Plan`, and `Escalations`, then returns `[ok]`,
`[escalate-to-research]`, or `[escalate-to-lead]` with confidence and
rationale. `[escalate-to-lead]` is a lead-directed scope-reduction signal,
distinct from `[escalate-to-research]`'s strategy/contract-uncertainty scope:
a fully-specified, multi-part requirement must be carried whole into the plan,
and a confident planner decision to implement only a subset is a scope
decision for the lead, never a unilateral planner choice, unless the ticket or
lead already authorized the phasing. If survey cannot safely support
implementation without strategy, contract, or reuse judgment, `lead-implement`
routes to `plan-populator-research` on the same plan path before spawning the
implementer.

`plan-populator-research` is reached from the survey escalation signal and makes
planner judgments: it reads any existing survey output at the same plan path,
chooses clean existing mechanisms when they fit the selected authority, preserves
selected contract and verification guardrails in the plan, rejects temporary,
implementation-fallback (scope-shortcut), mock-data, and duplicated-glue paths
— as distinct from a ticket's required runtime fallback (a specified execution
branch such as graceful degradation), which is not a shortcut signal and must
be planned in full — and escalates via the same `[escalate-to-lead]` channel
when no clean plan can satisfy it or when only a confident subset of a
fully-specified, multi-part requirement can be planned. A survey-to-research
route reuses the same authority inputs and replaces or refines the same plan
artifact path with the research plan; it does not create a
research-suffixed plan filename or append research to a survey plan.

Before spawning the implementer, `lead-implement` handles plan-populator exit
signals. It stops and escalates when implementation would likely pursue a wrong
contract, bypass existing project mechanisms, or rely on a shortcut path. Review
remains an enforcement step: reviewers compare the implementation against the
selected authority, plan, and diff to catch implementation-time shortcut drift, but known
plan-time risks are handled before source work begins.

The implementation plan is the implementer's sole context source, but it is not
a lossy ticket summary. For the selected implementation scope, the plan clips the
relevant ticket contract and records implementation strategy, codebase findings,
verification expectations, escalations, and explicit out-of-scope boundaries.
Ticket noise such as background discussion, unsettled options, and unrelated
future phases is stripped. In ticket-driven runs, reviewers read the ticket and
plan, then treat any specified authority requirement that is not implemented
and does not carry an explicit, authorized deferral as a blocking finding
within their assigned partitions.

`lead-implement` runs the documentation pre-pass after the Edit and Review
stages complete. For implementation-branch modes,
`lead-implement` also runs a post-documentation closeout compaction gate before
merge readiness is reported: it inspects only the branch-tip suffix and compacts
a contiguous run of safe documentation-only closeout commits into one closeout
commit when metadata synthesis and tree equivalence are unambiguous. Planning,
ready-promotion, source, test, review-fix, merge, ambiguous-authorship, and
non-documentation commits remain outside the compaction target; unsafe suffixes
and suffixes with fewer than two eligible commits are reported as skipped
without blocking merge readiness. At the approved merge step, a single
workflow-owned, message-clean commit may fast-forward into the merge target.
Multiple-commit implementation branches use a no-fast-forward merge by default;
fast-forward is reserved for commit lists whose entries are each independently
deployable and independently revertible target-history units. A branch that is
one logical change with noisy or dependent commits squashes.
{#260523-implement-doc-closeout-compaction}

After a confirmed merge, `lead-implement` runs a Branch Cleanup step to reduce
implementation-branch accumulation. It first verifies the implementation
branch is a strict ancestor of the merge target
(`git merge-base --is-ancestor`); it retains the branch and reports the skip
reason without deleting when the branch is currently checked out, linked to
an active worktree, the merge target was ambiguous, or the branch has commits
unreachable from the merge target. When none of those conditions hold, the
branch's naming convention gates the remaining flow: any `impl/`-prefixed
branch — `impl/<merge-root>/<stem>` (the convention `lead-implement` uses for
branches it creates, `<stem>` <=15 characters recommended, with any trailing
`-` trimmed) or the rootless `impl/<stem>` form — is deleted without asking.
A branch under any other name — including the legacy
`implement/<scope-slug>` convention — keeps the ask-first flow: the user is
asked before `git branch -d` runs, and the branch is retained if not
approved. The naming convention is a trust boundary, not a security
boundary — a hand-created `impl/*` branch this tooling did not produce would
also qualify for auto-delete once its structural guardrails pass.
{#260707-implement-branch-cleanup-naming-gate}

`lead-update-spec` audits recent commits for caller-visible behavior changes. It
adds or updates spec entries, handles removed spec stems, verifies the spec
index, and commits the spec pass. Two callers reach it: `lead-implement`'s
in-flow documentation pass, and `lead-backfill-docs` retroactively. Both expect
it to commit its own spec pass.

`lead-backfill-docs` reconciles spec and mental-model coverage for commits that
never received a documentation pass, including work that never routed through an
implementation flow. It resolves an audit window from commit markers — the newest
commit carrying `(mental-model-updated)` and the newest commit touching
`ai-docs/spec/` — or from a caller-supplied range, then delegates discovery to
`doc-gap-discovery`, which partitions the window into contiguous groups and
reports per group what changed and what existing documentation already covers.
Discovery returns candidates, not verdicts: caller-visibility judgment stays with
the lead, which applies it by running `lead-update-spec` inline per group. Each
group produces at most one `docs(spec):` commit. The two document kinds do not
share a unit: spec entries map to discrete behaviors and so are reconciled per
group, while mental-model domains are corpus-wide and are swept once over the
whole window after every spec pass, which also places those spec commits inside
the swept range. Undocumented commits are not contiguous, so the unit of work is the group rather
than one wide range. A marker-derived audit window is bounded by high-water marks
and therefore finds only drift newer than the last documentation pass; earlier
gaps require a caller-supplied range, and the skill reports that bound with its
result. `lead-discuss` names this skill when documentation staleness traces to
commits that never had a doc pass, which distinguishes it from a spec entry
nobody wrote in the first place.
{#260728-retroactive-doc-backfill-entry}

## Proceed Routing Pipeline {#260505-proceed-routing-pipeline}

`lead-proceed` is the first step for implementation tasks. An inline request
whose local scope and verification are clear, and for which neither planning nor
independent review materially improves the outcome, may take a direct-execution
early return. It states the reason, performs and verifies the bounded request,
and returns without calling `enter.proceed`, leaving the session agenda and todos
unchanged. The judgment may inspect only explicitly requested paths; unknown or
expanded scope uses normal routing. {#260828-proceed-direct-execution-early-return}

All other targets are route-only: `lead-proceed` reads conversation state and
existing workflow artifacts, then continues through the needed pipeline stages
without source inspection or implementation work. When workflow primitive context
is not already active, it loads `lead-workflow-manual` before routing.

When handoff stages are needed, their order is fixed:

```text
ticket readiness -> implementation
```

Existing actionable `ready/` ticket paths skip ticket creation and become
implementation targets after `lead-proceed` resolves implementation scope.
Targets without phase sections use the whole target. When the user names one
phase, that explicit request is honored exactly. When the user does not name a
phase, `lead-proceed` selects the first unfinished phase by default. One proceed
invocation carries one ticket phase when the target has phases. If a request
names multiple phases, or if the selected scope is plainly too broad from ticket
text, `lead-proceed` stops for conservative phase or ticket slicing rather than
splitting the phase internally.
If no unfinished phase remains, or if the named phase already has a result and
the user did not explicitly ask to revise or redo it, `lead-proceed` stops
instead of silently reimplementing completed work.
Compatibility phrasing such as `auto-slice` remains accepted as the same default
phase-selection policy.

Epic ticket paths are milestone-board artifacts, not implementation targets;
`lead-proceed` stops on epics and routes the user toward child ticket creation,
child ready promotion, or proceeding a ready child ticket. Workset ticket paths
are operating-context artifacts, not implementation targets; `lead-proceed`
stops on worksets and routes the user toward choosing, creating, promoting, or
proceeding an included actionable ticket. Container stops use an explicit
`container-ticket` scope blocker instead of selecting a phase or whole-ticket
implementation slice. Existing `idea/` and `todo/` ticket paths are treated as
implementation intent: `lead-proceed`
continues through `lead-write-ticket` for ticket triage, refresh, or autonomous
`todo/` -> `ready/` promotion before scope resolution, and escalates to
`lead-discuss` only when promotion or implementation scope exposes unresolved
design decisions, unclear completion criteria, user trade-offs, or missing spec
addressing that cannot be created.
Missing ticket paths, unknown ticket statuses, completed tickets, and dropped
tickets stop with a Routing Verdict instead of falling through to implementation.

Inline targets are classified before routing. Non-actionable inline targets
stop and route to `lead-discuss`. Actionable inline targets route to
`lead-discuss` when user-blocking decisions remain, route through
`lead-write-ticket` after discussion when accepted work spans multiple
independently reviewable phases or needs pre-implementation traceability beyond
its eventual implementation commit and relevant existing specs, and may route directly to `lead-implement`
when the accepted work is one bounded reviewable slice recoverable from its
eventual implementation commit plus any relevant existing spec, regardless of
file count or public surface. The
normal `lead-discuss -> lead-write-ticket -> ticket -> lead-proceed` persistence
path and all existing ticket readiness routes are unchanged.

Existing ticket routes use a lead-owned freshness check. Before implementation
routing, `lead-proceed` compares active conversation decisions and the ticket
artifact only; when settled decisions are missing from the ticket, it routes
through `lead-write-ticket` edit, re-reads the refreshed ticket, and then
continues scope resolution. When freshness is uncertain, it stops for
discussion instead of delegating hidden conversation context to a background subagent.
Unconfirmed mechanisms or future-scope hints are not settled decisions; they
make freshness uncertain rather than authorizing a ticket write.
For migration-sensitive targets, `lead-proceed` reads the native-subagent pivot
anchor as an artifact-only check, reports `Migration Anchor` in the Routing
Verdict, stops when the anchor is missing, and treats absent binding anchor
decisions as missing settled decisions.
{#260513-proceed-ticket-freshness-gate}

Except for a direct-execution early return, implementation routes through
`lead-implement` with the selected scope as a hard scope boundary.
`lead-proceed` does not rejudge general ticket quality, mutate ticket structure,
decide delegated plan depth, or invoke implementation primitives before
`lead-implement`; it requests phase or ticket slicing only when scope resolution
blocks safe implementation. Public or cross-module contract checkpoints are
expressed through the delegated `lead-implement` implementation plan.

`lead-implement` also loads the native-subagent pivot anchor before editing when
the target touches plugin architecture, host-neutral migration, spawn-removal,
or adapter boundaries. Delegated implementation has a required plan artifact;
when the migration anchor is read, binding implementation constraints from the
anchor are copied into the plan and the anchor is listed as a `[Must]` reference before
plan population or implementer dispatch. Delegated implementers receive only the
plan as task input, may read additional documents listed in the plan, and must
not read the ticket directly unless the plan's `Escalations` section explicitly
authorizes ticket-file reading.

For every normal route, `lead-proceed` calls `ws.enter.proceed` after lead-owned
fact gathering and receives a deterministic raw verdict with exactly one
`NEXT:` value plus a concrete `Next:` instruction. The MCP resolver owns
deterministic route-row precedence, normalization warnings, raw verdict text,
the JSON `next_instruction`, proceed agenda storage, and proceed todo
replacement; the playbook owns artifact reads, uncertain judgments,
conversation freshness, migration-anchor checks, and user-facing discussion.
`lead-proceed` does not restate a separate Routing Verdict or print a full route
chain as the active execution instruction. It follows MCP's `Next:` instruction,
which includes the route announcement, downstream invocation, verification,
failure, stop, and post-write reroute rails. After `lead-write-ticket` refresh or
promotion returns, the `Next:` instruction requires `lead-proceed` to rebuild
route context and enter `ws.enter.proceed` again instead of continuing from an
old verdict. When `NEXT:` is `lead-implement`, MCP's instruction tells
`lead-proceed` to call `ws/playbook.print(name: "lead-implement")` and execute
that playbook before source inspection, planning, editing, or
implementation-tool use. Outside the direct-execution judgment, `lead-proceed`
does not apply sibling `lead-implement` judges, compute direct/delegated
execution mode, compute branch mode, or inspect source.
`lead-implement` owns those decisions when the handoff executes by calling
`ws.enter.implement` after fact gathering. wsflow mirrors the same route-only
proceed boundary without pre-applying `wsflow:lead-implement` branch or
execution judgments.
{#260519-proceed-implementation-dispatch-precheck}

## Review Workflow Skills {#260513-review-workflow-skills}

`lead-review` reviews either a pull/merge request branch (branch scenario,
default or via a `branch` argument) or a caller-supplied `base..head` range
(range scenario, via a `range` argument) instead of a checked-out branch — the
caller owns minting the range marker; `lead-review` only consumes it. Both
scenarios run the same downstream phase, judge, and verdict machinery; only
target diff selection differs.

It loads `ai-docs/_review.local.md` for environment configuration (remote
access method, branch naming, review phases, blocked paths, comment and merge
methods, and contributor workflow). Config-load is scenario-scoped: the branch
scenario, absent config, interviews the user and writes the config before
proceeding (unchanged today); the range scenario, absent config, proceeds on
built-in review-substance defaults (intent/alignment/risk phase text, the Deep
Review threshold) and never enters the setup interview, since a range review
touches no checkout, remote, or merge and the collaboration/remote config half
is meaningless to it. When a config file is present, both scenarios honor its
review-substance sections (Review Phases, Checklist, Deep Review); the config
is machine-local and gitignored. `## Landing Lens` is the one exception: it is
honored by the range scenario only, present or absent config alike, and
independent of the Contributor Workflow setting — the branch scenario never
runs it. This keeps a branch/PR review from flagging an external contributor
for spec/mental-model updates they were never expected to author.

Branch discovery uses the configured remote access method (glab, API token, git
fetch, or equivalent); if no branch argument is supplied, `lead-review` lists
available branches filtered by any configured naming pattern and asks the user to
select one. Branch discovery does not apply to the range scenario — the
caller-supplied `base..head` is already the identified target.

Review phases run in order — intent, alignment, risk, and any configured custom
phases — producing one of four verdicts: BLOCKED (a blocked path was found
before phases ran), LGTM, NEEDS FIX, or OPEN. The range scenario additionally
runs a required `landing` phase last: convention adherence plus spec/mental-
model update completeness, checked against each doc's own function (spec
describes caller-visible behavior; mental model captures modification-relevant
operational knowledge), using config text if present else a built-in default.
The branch scenario never runs the `landing` phase; its finding folds into the
same aggregate-and-verdict path as any other phase, with no new verdict state.

LGTM follows the configured merge approval sequence and optional post-merge
notification. NEEDS FIX asks the user to fix locally or post findings to the
contributor; local fix routes to `lead-discuss` with findings as context, leaving
re-review to user discretion. OPEN enters discussion before re-routing to LGTM
or NEEDS FIX.

The range scenario additionally stamps the review-watermark ledger
(`#260830-review-watermark-ledger-tools`) immediately after verdict emission,
for every completed range-scenario verdict regardless of which verdict branch
follows; the branch scenario never stamps. It first calls
`review.marker(bootstrap: true)`, solely to seed a baseline entry when the
ledger is empty — the call's returned entry does not feed the stamp itself.
The verdict maps to a ledger token: LGTM -> `pass`; NEEDS FIX -> `concern` or
`block` by severity; OPEN -> `concern`. The stamped `base`/`head` are the
range invocation's own `<base>..<head>` arguments — the range identified at
invoke time — never the marker entry's `Base` field, which drifts to the
original bootstrap commit on every sweep after the first; stamping the
marker's `Base` would falsely claim a later sweep reviewed the entire span
back to the original bootstrap point instead of only the range just
reviewed. `ref` (a routed ticket stem) accompanies a `block` verdict only.
{#260830-review-range-scenario-ledger-stamp}

Independent of `lead-review`, four MCP call sites elsewhere in the workflow
surface (`tickets.close`, `workflow_manual`, `enter.implement`,
`enter.proceed`) surface a cheap review-watermark checkpoint nudge computed
from the same ledger (`#260830-review-watermark-checkpoint-nudge`). That
nudge is advisory only: it never blocks the call it rides on, and it never
appends to the ledger — only `review.marker` and `review.stamp` mutate it.

### Review Policy Config Surface {#260830-review-policy-config-surface}

Whether and where review *blocks* is per-project policy declared across three
non-overlapping config homes: tracked, per-track structural facts live in
`AGENTS.md`; marker and verdict state lives in the `ai-docs/` review-watermark
ledger; machine-local review mechanics (remote access, blocked paths, review
phases) live in the gitignored `_review.local.md`. The homes do not double-own:
the review-track branch is a shared structural fact and never lives in
`_review.local.md`, and the volatile marker never lives in `AGENTS.md`.

`AGENTS.md` declares the review policy under a `### Review Policy` section as
`key: value` lines, parsed fail-open (a missing file, missing section, missing
field, or malformed value degrades to the field's default, never an error):

- `review-track: <branch>` — the branch whose reviews stamp the ledger. When
  set, it takes precedence over the git-heuristic default (`origin/HEAD`, then
  local `main`, then `master`) that resolves the review track when the field is
  absent.
- `release-boundary: present | absent` — whether the project has a release
  promotion boundary. `absent` (the default) means advisory-only review with no
  hard gate; `present` arms `lead-ship`'s Release gate (`#260513-review-workflow-skill`
  below), which requires the range since the review-watermark frontier's head
  to clear before promoting the release branch.
- `rendezvous-backend: platform | canary` — how concurrent maintainers
  rendezvous on review state. `canary` (the default; needs no external config)
  relies on the append-only ledger's git-conflict canary; `platform` relies on
  host branch protection, for which the recommended set is require-branches-up-
  to-date, dismiss-stale-approvals, required-checks, and disabled squash/rebase
  (plus a merge queue at scale).

When the `review-track` field is unset, `workflow_manual` surfaces a
non-blocking, session-scoped nudge advising the project to configure a review
track; the nudge fires at most once per session (not once per checkpoint) and
never blocks the call it rides on.

`lead-review` scales review depth automatically. When commits lack `## AI
Context` and conventional commit format (`judge: follows-ws-workflow` does not
fire), subagent analysis infers intention before phases run. When diff size
exceeds the configured threshold (`judge: is-large-diff`), subagents run
alignment and risk phases in parallel. The contributor workflow setting can
force or suppress the subagent inference step.
{#260513-review-workflow-skill}

## Workflow Reconstruction Skills {#260505-workflow-reconstruction-skills}

`lead-forge-spec` reconstructs spec documents from scratch. It archives stale
current specs under `ai-docs/.old/spec/` after user confirmation, surveys
source, tickets, archived specs, and commit history, asks the user to confirm
the once-per-run behavioral domain list, then classifies per-item
caller-visibility and implemented/planned status autonomously rather than
blocking on a per-item confirmation, writes anchor-keyed spec entries, verifies
the index, and associates spec stems with active tickets when required.
{#260707-forge-spec-autoproceed-classification-2}

Each autonomous classification call that was genuinely ambiguous is recorded
with its behavior name, chosen classification, and a reason, and that record is
what the wrap-up summary reports from. The record covers both axes, so items
excluded from the spec as internal-only or planned appear on the same terms as
written ones; entries actually written to a spec additionally carry an inline
`<!-- AMBIGUOUS: <reason> -->` marker. A domain whose behaviors are all
excluded produces no spec file and is named as such in the summary. A run
resumed after the recording session reconstructs the list from the inline
markers and labels what reconstruction cannot recover, rather than reporting a
partial list as a complete count.
{#260728-forge-spec-ambiguity-record}

At wrap-up, `lead-forge-spec` asks whether to run `lead-forge-mental-model`
next and invokes it on a yes answer, regardless of how the run was reached
(standalone invocation, `lead-bootstrap`'s fresh-install suggestion, or the
index-health-check routing table). This only covers the same-session case;
`lead-bootstrap` itself is not otherwise changed.
{#260707-forge-spec-mental-model-chaining}

`lead-forge-mental-model` reconstructs mental-model documents from scratch. It
surveys operational domains, asks the user to confirm the domain set, writes
modification-focused domain files from current evidence, runs verification, and
commits the resulting mental-model corpus.

## Workflow Utility Skills {#260505-workflow-utility-skills}

Utility skills handle project workflow maintenance outside the main
implementation path.

`lead-add-rule` classifies a persistent rule as cross-cutting or domain-scoped
and writes it to the appropriate authority document. It triggers only when the
user explicitly asks to save, remember, persist, or add a durable rule for
future sessions; ordinary prescriptive task wording alone does not trigger rule
persistence. {#260508-add-rule-explicit-persistence-trigger}

`lead-bootstrap` bootstraps or upgrades downstream projects to `AGENTS.md` as
the canonical workflow context while preserving Claude compatibility through a
shim when needed.

Bootstrap installs a project-local workflow guide as `ai-docs/WORKFLOW.md` and
points `AGENTS.md` at it. The guide is a pinned explanation of the ws workflow
contract for plugin-less maintainers: it explains the document layers, ticket
lifecycle, spec stems, mental models, commit traceability, and manual fallback
expectations without becoming a project-local override for runtime semantics.
{#260506-bootstrap-workflow-guide}

A versioned migration item retires `ai-docs/_index.md`. On upgrade,
`lead-bootstrap` migrates the index's regions to their homes — resident
orientation into the `AGENTS.md` body, tracked session notes into the `repo`
note layer with qualitative staleness pruning, procedures into `manuals/`,
derivable inventories to generation — removes the read-`_index.md` step, and
deletes the file. On a fresh bootstrap the scaffold no longer creates
`_index.md`; the always-resident orientation is carried in the `AGENTS.md`
template body directly. A fresh-bootstrapped project and an upgrade-migrated
project converge on the same `AGENTS.md` shape, neither carrying an `_index.md`.
{#260812-bootstrap-index-dissolution}

A companion versioned migration item retires `ai-docs/_index.local.md` the
same way. On upgrade, `lead-bootstrap` splits the file's content by judgment
into two regions — machine-local procedure content (credentials, IPs,
hostnames, host-specific runbooks) into a new gitignored
`ai-docs/manuals/*.local.md` sibling, and volatile local context into the
`worktree` note layer by default, or the `clone` layer only when the content
is judged clone-wide rather than worktree-specific — removes the
read-`_index.local.md` step, removes the layout-tree entry describing it, and
deletes the file. On a fresh bootstrap the scaffold no longer creates
`_index.local.md`. A fresh-bootstrapped project and an upgrade-migrated
project converge on the same shape, neither carrying an `_index.local.md`,
mirroring the `_index.md` dissolution above.
{#260822-bootstrap-index-local-dissolution}

While a project still has an `ai-docs/_index.md` — an un-migrated but supported
transitional state — bootstrap runs an advisory index health check gated on the
file existing. The first pass reads only `_index.md`; when candidates exist, it
reports likely scope drift such as source-derived detail, behavior inventories,
modification knowledge, static reference material, work history, duplicated maps,
or stable reading maps, and asks whether to clean up `_index.md` now, defer
cleanup, or route semantic follow-up work through the owning workflow. Bootstrap
cleanup itself only compacts `_index.md`; it does not author or semantically
update specs, mental models, tickets, or references. The check skips cleanly once
the file is gone, and every other workflow step that reads or maintains
`_index.md` is likewise gated on its presence, degrading to the dissolved homes
when it is absent.

Bootstrap ensures downstream `.gitignore` covers local workflow state and
runtime-managed API documentation cache data: `ai-docs/**/*.local.md` and
`ai-docs/.deps/`. {#260508-bootstrap-api-deps-gitignore}

Bootstrap templates document `ai-docs/.old/` as a tracked project archive and
migrate legacy old-spec or old-material paths into that hidden archive during
versioned upgrades.

`lead-ship` follows the repository ship configuration to prepare and execute a
release. It confirms version, tag, and publish targets before any publishing
step.

When the loaded project's `AGENTS.md` `### Review Policy` declares
`release-boundary: present`, `lead-ship` runs an un-omittable, user-overridable
Release gate ahead of Execute's Pre-flight step — a playbook branch rather than
a config-listed Pre-flight bullet, since a bullet-only gate is defeatable by a
config that simply never lists it. The gate resolves the review-watermark
frontier's head (`review.marker(format: json)`), counts commits since it, and
either proceeds silently (empty range), triggers `lead-review` over the range
(non-empty), or — when the triggered review still does not clear it — surfaces
a strong recommendation and stops for an explicit user decision. An override
proceeds without stamping the marker: `lead-ship` never calls `review.stamp`;
only `lead-review`'s own sole-writer step ever advances the frontier
(`#260513-review-workflow-skill` above).

## Delegate Prompt Boundaries {#260505-workflow-delegate-prompt-boundaries}

Workflow skills source named-delegate prompts (implementers, reviewers, survey
workers, documentation updaters) from rendered rsrc delegate playbooks via
`ws/playbook.render`, not from register-time prompt stems. Public named-agent
registrations receive delegate-orientation instructions before role-specific
prompt material.

Rsrc playbooks may declare include fragments by bare stem. Include resolution
checks `<playbook>/<include>.<harness>.md` first, then
`<playbook>/<include>.md`, then the root-level `<include>.md` fallback. This
supports harness-local guidance such as `lead-write-ticket/task-list.codex.md`
without replacing existing root-level shared includes such as `code-reviewer.md`.

Delegate orientation reserves lifecycle orchestration, reviewer fanout,
workflow-stage routing, and final documentation ownership for the lead unless a
delegate is explicitly assigned those responsibilities. Delegates return their
assigned output through named-agent result surfaces rather than invoking lead
skills on their own.
The `implementer` and `implementer-relay` render playbooks are
direct-execution delegate surfaces, not nested-delegation surfaces: their
`delegates` metadata is false, so rendering them does not add the generic
continuation tip. They still carry `role: implementer` for render-minted child
credentials and tier-derived model guidance.
