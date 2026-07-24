---
title: Workflow Skills
summary: Codex-facing ws lead skills for planning, implementation routing, sprint work, reconstruction, utilities, and host-neutral workflow primitives.
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
lead-bootstrap
lead-discuss
lead-forge-mental-model
lead-forge-spec
lead-goal-step
lead-implement
lead-check-blockers
lead-proceed
lead-review
lead-salvage
lead-ship
lead-skill-authoring
lead-sprint
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

The directly invocable surface is narrowed to 14 entry skills the user invokes as
`/ws:<name>` — `lead-discuss`, `lead-sprint`, `lead-proceed`, `lead-review`,
`lead-ship`, `lead-salvage`, `lead-bootstrap`, `lead-skill-authoring`,
`lead-add-rule`, `lead-forge-mental-model`, `lead-forge-spec`,
`lead-verify-discussion`, `lead-tune`, and `lead-goal-step`. The remaining
procedures — `lead-implement`, `lead-write-ticket`, `lead-write-spec`,
`lead-workflow-manual`, `lead-check-blockers`,
and `lead-update-spec` — are internal procedures served as `ws/playbook.print`
content invoked by caller skills, not directly user-invoked entry points;
`lead-write-ticket` and `lead-write-spec` are orchestration-only. The
classification axis is whether the user is meant to type `/ws:<name>` directly, not
cross-skill invocation count. Each entry skill's own procedure body is likewise
served from a `ws/playbook.print` playbook behind a thin trigger shim: the SKILL.md
surface carries only the trigger description and delegates execution to its
playbook. Context-heavy entry skills (lead-discuss and lead-sprint) are an
exception: their SKILL.md carries a parallel init declaration —
`playbook.print` plus `workflow_manual` called in parallel — rather than a pure
routing stub, reducing init round-trips from 4–5 serial calls to 2 parallel rounds.
{#260610-entry-skill-surface-reduction}

`lead-tune` is the umbrella workflow-tuning entry skill: its description is the
runtime trigger surface that fires when the user signals intent to tune how the
workflow runs (delegation posture, mercenary-vs-native delegation, model tiers),
so the skill can proactively propose a tune. Its playbook is the tuning manual —
it loads the `config.tuning` catalog (`#260625-tuning-catalog`) and uses that
catalog's knob ids, writer tools, field options, and current values to drive
prompt overrides (`#260620-config-prompt-override-tuning-tools`, including
`UserPreferenceSection` for standing preferences), workflow preference knobs,
and `config.agents_tier` without reimplementing their set paths. The always-on
`lead-workflow-manual` carries only a one-line pointer, keeping tuning guidance out
of general-task routing attention. In agentless wsflow the catalog omits
full-ws-only knobs (`workflow.prefer_mercenary` and `config.agents_tier`), while
keeping shared knobs such as `workflow.prefer_subagent`.
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
fresh, self-contained subagent by default; the sole carve-out is that
authoring or mutating a durable artifact (ticket, spec) stays with the session
that already holds the authoritative context for the decision — the lead when
it was settled in the lead conversation, or the delegated subagent's own
continuing session when settled there — never a separate fresh spawn working
only from an after-the-fact summary. The earlier context-inheriting fork
delegate and its Codex `spawn_agent` fork-fallback wording were removed, leaving
two clean delegation poles: the fresh spawn and this context-holder carve-out.
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
{#260514-skill-authoring-carried-context}

## wsflow Skill Surface {#260513-wsflow-agentless-skill-surface}

The wsflow distribution ships a curated subset of lead workflow entry skills
under `wsflow:lead-*` invocation names and `wsflow/<tool>` MCP notation.
Shipped wsflow skills include planning, documentation, direct implementation,
bootstrap, release, verification, and reconstruction workflows:
`lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
`lead-write-ticket`, `lead-proceed`, `lead-implement`,
`lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-ship`,
`lead-sprint`, `lead-verify-discussion`, `lead-check-blockers`, `lead-forge-spec`,
`lead-forge-mental-model`, and `lead-review`.

The wsflow `lead-sprint` skill mirrors the episode-oriented sprint shell: it
coordinates discussion, exploration, `sprint-edit` micro-edit episodes, and
normal workflow handoff without owning a sprint branch or final wrap-up.
wsflow source execution is lead-owned: sprint-edit applies a lead-owned direct
edit, and larger or subagent-worthy work routes through normal wsflow workflow
gates, namely the converged `wsflow:lead-implement` spine
(`#260529-wsflow-converged-implement-spine`). The former wsflow `lead-edit`
skill was absorbed into that spine and removed from the wsflow skill set.
{#260513-wsflow-sprint-skill}

The wsflow package excludes skeleton flows, recovery orchestration, and
upstream authoring helper skills: `lead-write-skeleton`, `lead-salvage`, and
`lead-skill-authoring`. Shipped wsflow `SKILL.md` files are thin entry shims:
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

wsflow bootstrap uses package-local template version history. Its downstream
`AGENTS.template.md` starts at `v0001` for the wsflow baseline and does not
replay the full bootstrap migration backlog. Bootstrap behavior changes remain
mirroring-sensitive: maintainers check both packages and bump each package's
template version only when that package receives the behavior change.

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
`plan-populator-research`, `code-reviewer`, and `mental-model-updater`.

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
behavior. It reads spec conventions, generates stable spec stems, writes planned
or implemented entries according to the current behavior, verifies the spec
index, and commits the spec update.

`lead-write-ticket` creates or updates workflow tickets. It treats `todo/` as
accepted backlog and `ready/` as the spec-addressed implementation-ready status. The
spec-address gate runs only when a non-`epic`, non-`research`, non-`workset`
action creates or moves a ticket into `ready/`; `todo/` tickets may carry
optional `spec:` links as recovery hints. For `ready/` creation or promotion,
`lead-write-ticket` accepts
confirmed `spec:` or `spec-remove:` stems, or a ticket-local `## Spec Impact`
section naming the target spec area, expected caller-visible change, and whether
a contract-first planned spec is required. It invokes `lead-write-spec`
autonomously only for contract-first planned spec entries, and stops when no
stem or `## Spec Impact` can address the work, spec writing fails, or the
behavior is too underspecified to spec. `Ticket Focus` entries are maintained
for selected active attention items; only `ready/` entries are direct
implementation targets.

Discussion-derived ticket persistence is consent-gated. Before ticket cleanup
writes mechanism decisions, rejected alternatives, future-scope hints, Result
Forward notes, focus "Next" lines, or note/comment proposals, `lead-write-ticket`
builds a visible Open Decision Queue, asks whether to persist the discussion
when persistence was not already approved, resolves one queue item at a time,
updates the visible queue after each answer, and writes only user-confirmed
items. Rejected, deferred, unanswered, or otherwise unconfirmed items are omitted
unless the user explicitly approves recording their status.

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

`lead-salvage` handles failed large implementations, sprints, branches, and
agent runs where a wrong premise may require rollback or recovery. It freezes
evidence before cleanup, interviews the user to confirm the failure claim and
invalidated premises, fans out named-agent surveys for code blast
radius, ticket graph contamination, spec and mental-model impact, and preserved
evidence, then classifies artifacts as keep, rework, discard, or unknown. It
classifies affected tickets as keep, rewrite, drop, absorb, or unknown before
any ticket move. Destructive actions require explicit approval immediately
before execution.

The salvage output uses the existing ticket system: a research ticket records
the salvage report, a recovery epic is created when multiple tickets,
components, phases, or cross-child invariants are affected, and concrete repair
work moves into child tickets. The skill routes all ticket creation, edits,
drops, and status moves through `lead-write-ticket`; it does not perform source
edits. {#260510-salvage-recovery-workflow-skill}

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

`lead-goal-step` advances a goal-pursuit run by one step: select and
dispatch exactly one ticket from `ready/`, the sole progress gate —
nothing advances until a ticket reaches `ready/`. The skill name does not
drive loop behavior; the `/goal` Stop-hook's continue-vs-stop decision is
AI judgment over the body prose, not the name. Invoked without an active
goal run, it degenerates to a single-cycle shim, not a loop: one
invocation resolves at most one ready ticket and hands it to
`lead-proceed` as an explicit target, so the caller does not depend on
`lead-proceed`'s own target-from-conversation routing to guess which
ticket is meant — it does not poll or repeat internally. Repeated
draining across the whole `ready/` backlog is the caller's
responsibility (for example, a standing `/goal` directive whose Stop-hook
re-invokes this skill each turn until the queue is empty).
{#260723-lead-goal-step-rename-reposition}

Ticket selection is itself delegated, not done by the lead: the skill
spawns a light-tier Explore-style subagent to list `ready/`, prefer a
candidate named as a prerequisite in another ready ticket's
`related:`/`parent:` frontmatter when that referenced ticket is also in
`ready/`, otherwise default to the oldest date-prefix ticket (FIFO), and
return exactly one ticket path (or report the queue empty). The lead never
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
`agents-plugin/skills/lead-goal-step/SKILL.md` (no rsrc playbook, no
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

`lead-goal-step` adds goal-branch staging on top of the base
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
own `impl/<stem>` branch, merged into `goal/<parent>/<slug>` without an
approval ask and auto-deleted per the Branch Cleanup naming-gate behavior
(see the `impl/<stem>`-branch section above). When the selection subagent
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

`lead-verify-design` is removed; its `SKILL.md` and rsrc playbook were deleted
entirely (delete-don't-diet decision, `260630-epic-skill-playbook-diet`). Its
premise-gated design-verification function is now covered by the ticket
lifecycle's Sage Review Gate (`260624-sage-review-gate`), which dispatches
`ticket-reviewer-design` automatically at `todo/`→`ready/` promotion, plus the
conditional independent-judgment step added to `lead-verify-discussion`. Do
not route new work through it.
{#260524-design-verification-skill}

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
commit with `## AI Context`, lead-owned review, and a completion report naming
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
the allocation default. Relay cap is 2 cycles for single-reviewer, 3 cycles for
partitioned with lead adjudication at cycle 2 and caller escalation at cycle 3.
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
finding is not re-relayed, only genuinely new Critical/Important findings are —
layered over the relay cap as the backstop for the pathological case of a
reviewer inventing new findings each cycle.
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
`Verification Plan`, and `Escalations`, then returns `[ok]` or
`[escalate-to-research]` with confidence and rationale. If survey cannot safely
support implementation without strategy, contract, or reuse judgment,
`lead-implement` routes to `plan-populator-research` on the same plan path
before spawning the implementer.

`plan-populator-research` is reached from the survey escalation signal and makes
planner judgments: it reads any existing survey output at the same plan path,
chooses clean existing mechanisms when they fit the selected authority, preserves
selected contract and verification guardrails in the plan, rejects temporary,
fallback, mock-data, and duplicated-glue paths, and escalates when no clean plan
can satisfy it. A survey-to-research route reuses the same authority inputs and
replaces or refines the same plan artifact path with the research plan; it does not create a
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
plan, then treat selected-scope binding decisions omitted from the plan or
violated by the implementation as blocking findings within their assigned
partitions.

`lead-implement` runs the documentation pre-pass after the Edit and Review
stages complete. `lead-sprint` runs documentation closure only for marked
`sprint-edit` episodes when each episode wraps. For implementation-branch modes,
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
branch's naming convention gates the remaining flow: a branch named
`impl/<stem>` (the convention `lead-implement` uses for branches it creates,
`<stem>` <=15 characters recommended, with any trailing `-` trimmed) is
deleted without asking. A branch under any other name — including the legacy
`implement/<scope-slug>` convention — keeps the ask-first flow: the user is
asked before `git branch -d` runs, and the branch is retained if not
approved. The naming convention is a trust boundary, not a security
boundary — a hand-created `impl/*` branch this tooling did not produce would
also qualify for auto-delete once its structural guardrails pass.
{#260707-implement-branch-cleanup-naming-gate}

`lead-update-spec` audits recent commits for caller-visible behavior changes. It
adds or updates spec entries, strips planned markers when implementation lands,
handles removed spec stems, verifies the spec index, and commits the spec pass.

## Proceed Routing Pipeline {#260505-proceed-routing-pipeline}

`lead-proceed` is the first step for implementation tasks. It is route-only: it
reads conversation state and existing workflow artifacts, then continues through
the needed pipeline stages without reading source code or performing
implementation work. When workflow primitive context is not already active, it
loads `lead-workflow-manual` before routing.

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

Implementation always routes through `lead-implement` with the selected scope as
a hard scope boundary. `lead-proceed` does not rejudge general ticket quality,
mutate ticket structure, decide delegated plan depth, or invoke implementation
primitives before `lead-implement`; it requests phase or ticket slicing only
when scope resolution blocks safe implementation. Public or cross-module
contract checkpoints are expressed through the delegated `lead-implement`
implementation plan.

`lead-implement` also loads the native-subagent pivot anchor before editing when
the target touches plugin architecture, host-neutral migration, spawn-removal,
or adapter boundaries. Delegated implementation has a required plan artifact;
when the migration anchor is read, binding implementation constraints from the
anchor are copied into the plan and the anchor is listed as a `[Must]` reference before
plan population or implementer dispatch. Delegated implementers receive only the
plan as task input, may read additional documents listed in the plan, and must
not read the ticket directly unless the plan's `Escalations` section explicitly
authorizes ticket-file reading.

Before any handoff, `lead-proceed` calls `ws.enter.proceed` after lead-owned
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
implementation-tool use. `lead-proceed` does not apply sibling `lead-implement`
judges, compute direct/delegated execution mode, compute branch mode, or inspect
source.
`lead-implement` owns those decisions when the handoff executes by calling
`ws.enter.implement` after fact gathering. wsflow mirrors the same route-only
proceed boundary without pre-applying `wsflow:lead-implement` branch or
execution judgments.
{#260519-proceed-implementation-dispatch-precheck}

## Sprint Session Shell {#260505-sprint-session-container}

`lead-sprint` is an episode-oriented workflow shell for sustained user sessions.
It stays on the current branch, coordinates discussion and exploration, and
routes larger implementation through `lead-proceed` or `lead-implement` instead
of creating `sprint/` branches or running a final branch wrap-up.

Small interactive edits may enter `sprint-edit` only when a single lead-owned
context covers the whole change. Each sprint-edit commit carries recoverable
commit-body markers, `Sprint-Edit: <episode-slug>` and
`Sprint-Edit-Context: <one-line context>`. After each edit, the shell asks
whether to keep refining the current context, wrap it up, or shift direction.

Wrapping a sprint-edit episode runs documentation closure for that marked
episode range: update specs, refresh mental models when needed, follow the
executor document pipeline for episode-scoped docs, commit documentation changes,
clear the active edit context, and return to the sprint loop. Public contracts,
routing semantics, protocols, ticket phase completion, cross-module new patterns,
plan or review allocation, and branch decisions route outside sprint-edit.
{#260523-sprint-episode-workflow-shell}

## Review Workflow Skills {#260513-review-workflow-skills}

`lead-review` reviews a pull request or merge request branch. It loads
`ai-docs/_review.local.md` for environment configuration (remote access method,
branch naming, review phases, blocked paths, comment and merge methods, and
contributor workflow); when no config exists, it interviews the user and writes
the config before proceeding. The config is machine-local and gitignored.

Branch discovery uses the configured remote access method (glab, API token, git
fetch, or equivalent); if no branch argument is supplied, `lead-review` lists
available branches filtered by any configured naming pattern and asks the user to
select one.

Review phases run in order — intent, alignment, risk, and any configured custom
phases — producing one of four verdicts: BLOCKED (a blocked path was found
before phases ran), LGTM, NEEDS FIX, or OPEN.

LGTM follows the configured merge approval sequence and optional post-merge
notification. NEEDS FIX asks the user to fix locally or post findings to the
contributor; local fix routes to `lead-discuss` with findings as context, leaving
re-review to user discretion. OPEN enters discussion before re-routing to LGTM
or NEEDS FIX.

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
caller-visibility and implemented/planned status autonomously - ambiguous
calls carry an inline `<!-- AMBIGUOUS: <reason> -->` marker and are collected
into the wrap-up summary rather than blocking on a per-item confirmation -
writes anchor-keyed spec entries, verifies the index, and associates planned
stems with active tickets when required.
{#260707-forge-spec-autoproceed-classification-2}

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

Bootstrap runs an advisory `_index.md` health check when the index exists. The
first pass reads only `_index.md`; when candidates exist, it reports likely
scope drift such as source-derived detail, behavior inventories, modification
knowledge, static reference material, work history, duplicated maps, or stable
reading maps, and asks whether to clean up `_index.md` now, defer cleanup, or
route semantic follow-up work through the owning workflow. Bootstrap cleanup
itself only compacts `_index.md`; it does not author or semantically update
specs, mental models, tickets, or references.

Bootstrap ensures downstream `.gitignore` covers local workflow state and
runtime-managed API documentation cache data: `ai-docs/**/*.local.md` and
`ai-docs/.deps/`. {#260508-bootstrap-api-deps-gitignore}

Bootstrap templates document `ai-docs/.old/` as a tracked project archive and
migrate legacy old-spec or old-material paths into that hidden archive during
versioned upgrades.

`lead-ship` follows the repository ship configuration to prepare and execute a
release. It confirms version, tag, and publish targets before any publishing
step.

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
