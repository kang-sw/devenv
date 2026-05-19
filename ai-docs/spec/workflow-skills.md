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
lead-edit
lead-forge-mental-model
lead-forge-spec
lead-implement
lead-check-blockers
lead-proceed
lead-review
lead-salvage
lead-ship
lead-skill-authoring
lead-sprint
lead-update-spec
lead-verify-discussion
lead-workflow-manual
lead-write-code
lead-write-skeleton
lead-write-spec
lead-write-ticket
```

Skill descriptions provide the natural-language trigger surface for Codex.
Descriptions distinguish strong top-level entry triggers from lighter
derived-stage triggers so Codex reliably invokes workflow entry points without
overmatching internal pipeline stages.
{#260508-skill-description-attention-policy}

## Workflow Primitive Reference {#260505-workflow-primitive-reference}

`lead-workflow-manual` is the shared primitive reference for writing or executing ws
workflow skills. It defines host-neutral notation: `ws/<tool-name>` means an MCP
tool on the `ws` server, while `ws:` names plugin skills.

Shared skill text uses ws MCP primitives for agent orchestration, scoped
queries, generated artifact paths, runtime metadata, workflow discovery, Git
operations, API documentation lookup, and project/convention reads. Skills name
only primitives available in the runtime; when a needed surface is not exposed
yet, skill text describes the required MCP contract instead of naming a
host-specific helper.

Codex-facing workflow skill guidance presents MCP primitives as the primary ws
runtime surface. Promptless `ws/agents.register(name: "<agent-name>")` is the
general-purpose named-agent form; role-specific delegates use `prompts:
["<prompt-stem>"]`, and optional `model` arguments select portable aliases or
one-off concrete models. CLI adapter syntax belongs only in compatibility or
testing references. {#260507-mcp-centric-workflow-language}

Workflow guidance prefers `model` for both portable aliases and concrete
overrides. Examples use `model: "core"` or `model: "deep"` for portable
selection and concrete provider names such as `gpt-5.5` or
`claude-sonnet-4.6` only when backend-specific routing is intentional. `tier`
remains documented only as deprecated compatibility input.
{#260508-workflow-model-alias-guidance}

Workflow skill-authoring guidance treats skill-to-skill transitions as carried
context, not API calls with arguments. Skill text says what context downstream
workflow skills should carry; argument language is reserved for MCP tools, CLI
commands, and structured templates. Dense routing or rule lists use Markdown
hierarchy, named groups, fixed lookup tables, and command-shaped lists before
inventing pseudo-code-like notation. {#260514-skill-authoring-carried-context}

## wsflow Skill Surface {#260513-wsflow-agentless-skill-surface}

The wsflow distribution ships a curated subset of lead workflow skills
under `wsflow:lead-*` invocation names and `wsflow/<tool>` MCP notation.
Shipped wsflow skills include planning, documentation, direct implementation,
bootstrap, release, verification, and reconstruction workflows:
`lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
`lead-write-ticket`, `lead-proceed`, `lead-implement`, `lead-edit`,
`lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-ship`,
`lead-sprint`, `lead-verify-discussion`, `lead-check-blockers`, `lead-forge-spec`,
`lead-forge-mental-model`, and `lead-review`.

The wsflow `lead-sprint` skill is a sprint-branch session container that
preserves deferred documentation wrap-up and routes source changes through the
wsflow edit workflow. `lead-edit` may use direct edits or scoped subagent
implementation while the sprint wrap-up keeps documentation integration,
verification, and commit ownership explicit. {#260513-wsflow-sprint-skill}

The wsflow package excludes full ws implementation relays, skeleton flows,
recovery orchestration, and upstream authoring helper skills:
`lead-write-code`, `lead-write-skeleton`, `lead-salvage`, and
`lead-skill-authoring`. wsflow skill text uses scoped subagent guidance for
exploration, implementation, verification, audit, or review and keeps lead
responsibility focused on integration, verification, final judgment, and commits.

wsflow skills are curated semantic rewrites, not generated copies. A change to
a full `agents-plugin/skills/lead-*` skill that is shipped in wsflow must either
update the corresponding wsflow skill in the same logical change or leave an
explicit follow-up ticket. A change to a full skill excluded from wsflow must
still check whether the wsflow workflow manual, exclusion rationale, or static
verification rules drifted. The wsflow skill-bundle verification path checks
inventory and forbidden full ws agent references, but it does not require text
identity with the full ws skill.

wsflow bootstrap uses package-local template version history. Its downstream
`AGENTS.template.md` starts at `v0001` for the wsflow baseline and does not
replay the full bootstrap migration backlog. Bootstrap behavior changes remain
mirroring-sensitive: maintainers check both packages and bump each package's
template version only when that package receives the behavior change.

## Planning Workflow Skills {#260505-planning-workflow-skills}

Planning skills prepare caller-visible work before implementation.

`lead-discuss` explores a topic without editing source code. It loads project
context, uses scoped subqueries when search is needed, can promote or move
tickets when the discussion reaches an actionable state, and recommends an
appropriate next workflow step. Discussion responses use the user's active
conversation language. When the user explicitly wants implementation to start,
`lead-discuss` invokes `lead-proceed` instead of routing directly to
`lead-implement`.

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
accepted backlog and `ready/` as the spec-gated implementation queue. The spec
gate runs only when a non-`epic`, non-`research` action creates or moves a ticket
into `ready/`; `todo/` tickets may carry optional `spec:` links as recovery
hints. For `ready/` creation or promotion, missing coverage causes
`lead-write-ticket` to invoke `lead-write-spec` autonomously, re-check coverage,
and stop only when coverage remains missing, spec writing fails, or the behavior
is too underspecified to spec. Queue entries are maintained for `ready/` work
only.

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

`lead-write-ticket` treats tickets as recoverability artifacts before compact
summaries. Non-epic actionable tickets preserve caller-visible contracts,
constraints, rationale, rejected alternatives, forward-compatibility guardrails,
verification expectations, suggested strategy, phase dependencies, and agreed
API/type/event/UI sketches. Source-local edit notes are excluded unless they
are settled constraints; settled local or cross-ticket decisions stay in the
relevant child ticket or phase.

`lead-write-ticket` reviews related-ticket decisions by default when
creating or editing a non-epic actionable ticket. It inspects the target's
parent, containing epic, child board, explicitly related tickets, and available
active siblings only far enough to find settled decisions that constrain the
current implementation slice. It records only binding decisions in the target
as scope, constraints, forward-compatibility guardrails, rejected alternatives,
verification expectations, or phase dependencies, and avoids copying unrelated
future-phase detail. Explicit "cascade" requests, board organization, or parent
and child edits broaden this into a multi-ticket propagation pass: the skill
identifies the impacted graph, selects only affected edit targets, keeps epic
edits board-level, updates active inventory when needed, and commits the
propagation as one logical documentation unit. It does not promote propagated
tickets to `ready/` unless the user explicitly requests ready promotion or
routes through `lead-proceed`.
{#260516-write-ticket-related-ticket-propagation}

Skill-authoring guidance treats local shorthand as trigger examples for a
general intent, not as the concept name itself. New workflow shorthand should
name the broad intent first and list the shorthand only where it prevents
repeated routing failures.

`lead-salvage` handles failed large implementations, sprints, branches, and
agent runs where a wrong premise may require rollback or recovery. It freezes
evidence before cleanup, interviews the user to confirm the failure claim and
invalidated premises, fans out named-agent or subquery surveys for code blast
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
structure choices through scoped `ws/subquery` calls, searches for already
implemented items that can be reused or merged to avoid duplication, synthesizes
corrected assumptions, observations, reuse opportunities, and code-hygiene
findings, checks for over-alignment signals such as weak premise handling or
missing countercases, then steers the discussion toward the best-supported
direction.
{#260512-discussion-verification-skill}

### Check Blockers Checkpoint {#260513-check-blockers-skill}

`lead-check-blockers` gives users a frequent spoken checkpoint for deciding
whether a design discussion still has user-blocking blockers. It does not edit files. It
classifies remaining work into user-blocking design questions, ticket or spec
capture gaps, autonomous code-hygiene items, and proceed readiness.

`lead-write-skeleton` is deprecated from normal implementation routing. The
skill file remains available for compatibility, but `lead-implement` no longer
invokes it and absence of ticket `skeletons:` frontmatter does not create a
skeleton obligation. {#260510-skeleton-contract-populator-flow}

`lead-write-code` absorbs the useful skeleton role through brief authoring.
For public interface, cross-module boundary, or new type contract changes, the
brief includes concrete `Contract Instructions`: expected files or modules,
public types/functions/handlers/tools, visibility, call shape, input/output
shape, lifecycle boundaries, existing mechanisms to reuse, and forbidden
temporary, fallback, or mock-data wiring. It also includes concrete
`Integration Test Instructions`: the required boundary type such as parser,
CLI, MCP tool, doc convention, skill routing, runtime lifecycle, or agent
relay; whether to extend existing tests or create new integration tests; and
observable pass criteria. Implementers treat both sections as acceptance
criteria, and fit/test reviewers compare the implementation against them.
{#260512-skeleton-inside-implement-branch}

Ticket `skeletons:` frontmatter is a backward-compatible legacy artifact map.
Existing entries may still document old skeleton artifact commits, but normal
workflow routing does not create new skeleton artifacts. {#260512-skeleton-draft-and-final-commits}

## Implementation Workflow Skills {#260505-implementation-workflow-skills}

Implementation skills execute code changes and close the documentation loop.

`lead-implement` is the implementation harness. It routes to direct editing or
delegated code writing, then runs the shared post-implementation documentation
pipeline before reporting completion. Existing `implement/*` branches continue
on the current branch; otherwise delegated implementation creates an
`implement/<scope>` branch. After verification, `lead-implement` records the
phase result commit, closes spec, mental-model, ticket, and index updates, then
asks the user to merge, continue, or stop. Follow-up changes after this gate
route to another implementation slice or sprint and are captured in tickets as
append-only Result editions for already completed phases.

`lead-edit` performs a narrow direct edit in the lead session. It honors
caller-provided scope boundaries, verifies the change, uses one reviewer for
correctness and fit, escalates if the scope grows, and reports the commit range
and test status to its caller.

`lead-write-code` delegates an implementation target through an implementer
agent, optional plan, partitioned reviewers, bounded fix relay, cleanup, and
completion report. Its brief preserves caller-provided scope boundaries and
selected binding decisions, and it carries concrete contract and integration
test instructions when the target changes public or cross-module contracts.
When workflow primitive context is not already active, it loads
`lead-workflow-manual` before registering delegates or reviewers.

Plan population is an either/or depth choice. When plan depth is `survey`,
`plan-populator-survey` produces file-backed reference-map evidence and possible
risk signals without deciding that the implementation direction is wrong. If
survey cannot safely support implementation without strategy, contract, or
reuse judgment, it returns `[escalate-to-research]` instead of forcing a survey
plan. `lead-write-code` then routes to `plan-populator-research` before
spawning the implementer.

When plan depth is `research`, `plan-populator-research` makes planner
judgments: it chooses clean existing mechanisms when they fit the brief,
preserves contract and integration-test guardrails in the plan, rejects
temporary, fallback, mock-data, and duplicated-glue paths, and escalates when no
clean plan can satisfy the brief. A survey-to-research route replaces the same
plan artifact path with the research plan; it does not create a research-suffixed
plan filename or append research to a survey plan.

Before spawning the implementer, `lead-write-code` handles plan-populator exit
signals. It stops and escalates when implementation would likely pursue a wrong
contract, bypass existing project mechanisms, or rely on a shortcut path. Review
remains an enforcement step: reviewers compare the implementation against brief
and plan guardrails and catch implementation-time shortcut drift, but known
plan-time risks are handled before source work begins.

The implementation brief is the implementer's sole context source, but it is
not a lossy ticket summary. For the selected implementation scope, the brief
records every settled caller-visible contract, implementation strategy decision,
rejected alternative, and verification expectation from the target, or marks it
explicitly deferred or out of scope. Ticket noise such as background discussion,
unsettled options, and unrelated future phases is stripped. In ticket-driven
runs, the fit reviewer reads the ticket and treats selected-scope binding
decisions omitted from the brief or violated by the implementation as blocking
findings. Correctness and test reviewers remain scoped to the diff and their
assigned partitions.

`lead-edit` and `lead-write-code` are code-and-review primitives; callers own
documentation pipeline timing. `lead-implement` runs the documentation pre-pass
after either primitive returns, while `lead-sprint` defers that pass to wrap-up.

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
spec -> ticket -> implementation
```

Existing non-epic `ready/` ticket paths skip ticket creation and become
implementation targets after `lead-proceed` resolves implementation scope.
Targets without phase sections use the whole target. When the user names one
phase, that explicit request is honored exactly. When the user does not name a
phase, `lead-proceed` selects the first unfinished phase by default. One proceed
invocation carries one ticket phase when the target has phases. If a request
names multiple phases, or if the selected phase is plainly too broad from ticket
text, `lead-proceed` stops for conservative phase or ticket slicing rather than
splitting the phase internally.
Compatibility phrasing such as `auto-slice` remains accepted as the same default
phase-selection policy.

Epic ticket paths are milestone-board artifacts, not implementation targets;
`lead-proceed` stops on epics and routes the user toward child ticket creation,
child ready promotion, or proceeding a ready child ticket. Existing `todo/`
ticket paths are treated as implementation intent: `lead-proceed` continues
through `lead-write-ticket` with carried context for autonomous `todo/` ->
`ready/` promotion before scope resolution, and escalates to `lead-discuss` only
when promotion or implementation scope exposes unresolved design decisions,
unclear completion criteria, user trade-offs, or missing spec coverage that
cannot be created.

Inline targets are classified before routing. Non-actionable inline targets
stop and route to `lead-discuss`. Actionable inline targets route to
`lead-discuss` when user-blocking decisions remain, route through
`lead-write-ticket` when durable workflow traceability, phases, acceptance
criteria, or spec-visible behavior need capture before implementation, and may
route directly to `lead-implement` when the target is narrow, routine, fully
scoped, and commit `AI Context` is enough traceability.

Warm discussion state with an existing related ticket uses a ticket freshness
gate. Before implementation routing, `lead-proceed` compares the active
conversation and ticket artifact only; when settled decisions are missing from
the ticket, it routes through `lead-write-ticket` edit, re-reads the refreshed
ticket, and then continues scope resolution.
{#260513-proceed-ticket-freshness-gate}

Implementation always routes through `lead-implement` with the selected scope as
a hard scope boundary. `lead-proceed` does not rejudge ticket quality, demand
ticket splitting, mutate ticket structure, decide contract-brief depth, or
invoke implementation primitives before `lead-implement`. Public or
cross-module contract checkpoints are expressed as `lead-write-code` brief
contract and integration-test instructions.

Before implementation handoff, `lead-proceed` selects a conservative
implementation dispatch from conversation and workflow artifacts only. Full ws
announcements report `Implementation Dispatch`, `Dispatch Reason`, and
`Branch Mode`; a proceed-selected `write-code` dispatch is a hard lower bound
that `lead-implement` cannot downgrade to direct edit. wsflow announcements
mirror the same routing clarity as `Execution Path`, `Complexity Flag`, and
`Branch Mode` without introducing unavailable full ws `lead-write-code`
routing. {#260519-proceed-implementation-dispatch-precheck}

## Sprint Session Container {#260505-sprint-session-container}

`lead-sprint` is a multi-task session container for feature-branch work. It
operates on `sprint/` branches, loops over user requests, and routes each task
to inline discussion, scoped subquery, direct edit, or delegated code writing.

During the sprint loop, documentation pipeline work is suppressed for individual
tasks. On wrap-up, the skill computes the branch range, runs the spec update
pass, invokes the mental-model updater, follows the executor wrap-up document
pipeline, commits documentation updates, reports the documentation changes, and
merges or deletes the sprint branch according to the remaining source changes.

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
behavioral domains and caller-visible classifications, writes anchor-keyed spec
entries, verifies the index, and associates planned stems with active tickets
when required.

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

Workflow skills use embedded prompt chains for named delegates such as
implementers, reviewers, survey workers, and documentation updaters. Public
named-agent registrations receive delegate-orientation
instructions before role-specific prompt material.

Delegate orientation reserves lifecycle orchestration, reviewer fanout,
workflow-stage routing, and final documentation ownership for the lead unless a
delegate is explicitly assigned those responsibilities. Delegates return their
assigned output through named-agent result surfaces rather than invoking lead
skills on their own.
