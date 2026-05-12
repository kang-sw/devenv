---
title: Workflow Skills
summary: Codex-facing ws lead skills for planning, implementation routing, sprint work, reconstruction, utilities, and host-neutral workflow primitives.
---

# Workflow Skills

The ws workflow skill set gives Codex users a host-neutral project workflow for
discussion, specification, ticketing, skeletons, implementation, review,
documentation updates, release, and session handoff. The Codex-facing surface is
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
lead-exit-session
lead-forge-mental-model
lead-forge-spec
lead-implement
lead-proceed
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

## Planning Workflow Skills {#260505-planning-workflow-skills}

Planning skills prepare caller-visible work before implementation.

`lead-discuss` explores a topic without editing source code. It loads project
context, uses scoped subqueries when search is needed, can promote or move
tickets when the discussion reaches an actionable state, and recommends an
appropriate next workflow step. Discussion responses use the user's active
conversation language.

For proposal, evaluation, design-direction, causal-claim, scope-assumption, or
trade-off-heavy user messages, `lead-discuss` frames the reply around a visible
premise-aware intent summary before giving advice. The frame decomposes the
message into claims, goals, and constraints, names implicit premises with failure
conditions, reframes the topic as a neutral decision problem, lists considered
and dropped options, and ends with a stance. If a decision branch remains open
after that frame, the skill interviews through the highest unresolved branch
first, descends only after parent decisions settle, and returns to the nearest
unresolved parent when the user delegates lower-level detail.
{#260510-discuss-intent-frame-interview}

`lead-write-spec` writes or updates behavioral spec entries for caller-visible
behavior. It reads spec conventions, generates stable spec stems, writes planned
or implemented entries according to the current behavior, verifies the spec
index, and commits the spec update.

`lead-write-ticket` creates or updates workflow tickets. It treats `todo/` as
accepted backlog and `ready/` as the spec-gated implementation queue. The spec
gate runs only when a non-`epic`, non-`research` action creates or moves a ticket
into `ready/`; `todo/` tickets may carry optional `spec:` links as recovery
hints. Queue entries are maintained for `ready/` work only.

`lead-write-ticket` preserves epics as lightweight milestone boards. When
detailed discussion, implementation phases, or slice-specific decisions arise
while editing an epic, the skill creates or updates child tickets instead of
expanding the epic body; a single child ticket may carry multiple phases when
they form one cohesive reviewable unit. {#260508-write-ticket-epic-child-boundary}

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
structure choices through scoped `ws/subquery` calls, synthesizes corrected
assumptions, observations, and code-hygiene findings, then steers the discussion
toward the best-supported direction.
{#260512-discussion-verification-skill}

`lead-write-skeleton` optionally locks high-risk caller-visible contracts before
implementation when the scope needs a separate reviewable checkpoint. It
uses deep insertion-point research, then the lead writes a low-resolution source
draft with language-neutral `CONTRACT:`, `HINT:`, and `HOLE:` comment markers.
The lead-authored draft may be non-compiling only before populator handoff.
`CONTRACT:` marks binding public shape and behavior targets, `HINT:` marks
approximate references for source discovery, and `HOLE:` marks unknown concrete
types, imports, fixtures, helpers, or harnesses. A `skeleton-populator` delegate
researches and normalizes hints, fills clear holes, converts the draft into
compile-clean stubs and build-valid test scaffolding, and escalates missing or
conflicting contract elements instead of silently changing public shape. A
read-only `skeleton-reviewer` delegate checks contract preservation, marker
resolution, stub-only scope, and build or syntax evidence before lead commit.
The skeleton review loop stays lightweight: one reviewer, one amendment round,
then stop and report if still non-clean. The lead makes contract amendments,
verifies build or syntax checks, commits the final skeleton, and links generated
skeleton artifacts to the ticket. {#260510-skeleton-contract-populator-flow}

`lead-implement` owns skeleton decisions and execution inside the implementation
branch lifecycle. `lead-proceed` only routes implementation-ready targets to
`lead-implement`; it does not decide skeleton need or invoke
`lead-write-skeleton` before implementation. {#260512-skeleton-inside-implement-branch}

`lead-write-skeleton` preserves both authoring boundaries as commits on the
current branch: a lead-authored skeleton draft commit, followed by a final
populated skeleton commit after populator and reviewer checks. Ticket
`skeletons:` frontmatter records only the final skeleton commit hash, not the
draft checkpoint. {#260512-skeleton-draft-and-final-commits}

## Implementation Workflow Skills {#260505-implementation-workflow-skills}

Implementation skills execute code changes and close the documentation loop.

`lead-implement` is the implementation harness. It routes to direct editing or
delegated code writing, then runs the shared post-implementation documentation
pipeline before reporting completion.

`lead-edit` performs a narrow direct edit in the lead session. It honors
existing skeleton artifacts, verifies the change, uses one reviewer for
correctness and fit, escalates if the scope grows, runs spec update handling,
and reports the result.

`lead-write-code` delegates an implementation target through an implementer
agent, optional plan, partitioned reviewers, bounded fix relay, cleanup, and
completion report. It honors existing skeleton artifacts but does not require
missing skeletons. When workflow primitive context is not already active, it
loads `lead-workflow-manual` before registering delegates or reviewers.

`lead-update-spec` audits recent commits for caller-visible behavior changes. It
adds or updates spec entries, strips planned markers when implementation lands,
handles removed spec stems, verifies the spec index, and commits the spec pass.

## Proceed Routing Pipeline {#260505-proceed-routing-pipeline}

`lead-proceed` is the first step for implementation tasks. It is route-only: it
reads conversation state and existing workflow artifacts, then chains the needed
pipeline stages without reading source code or performing implementation work.
When workflow primitive context is not already active, it loads
`lead-workflow-manual` before routing.

The pipeline order is fixed:

```text
spec -> ticket -> implementation
```

Existing non-epic `ready/` ticket paths skip ticket creation and are direct
implementation targets. Epic ticket paths are milestone-board artifacts, not
implementation targets; `lead-proceed` stops on epics and routes the user toward
child ticket creation, child ready promotion, or proceeding a ready child ticket.
Existing `todo/` ticket paths route through `lead-discuss` for `todo/` ->
`ready/` promotion before implementation. Actionable inline targets go through
`lead-write-ticket`; exploratory targets stop and suggest `lead-discuss`.
Implementation always routes through `lead-implement`. When a separate contract
checkpoint may be needed before implementation, `lead-implement` decides whether
to run `lead-write-skeleton` before edit/write-code.

## Sprint Session Container {#260505-sprint-session-container}

`lead-sprint` is a multi-task session container for feature-branch work. It
operates on `sprint/` branches, loops over user requests, and routes each task
to inline discussion, scoped subquery, direct edit, or delegated code writing.

During the sprint loop, documentation pipeline work is suppressed for individual
tasks. On wrap-up, the skill computes the branch range, runs the spec update
pass, invokes the mental-model updater, follows the executor wrap-up document
pipeline, commits documentation updates, reports the documentation changes, and
merges or deletes the sprint branch according to the remaining source changes.

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

Bootstrap ensures downstream `.gitignore` covers local workflow state and
runtime-managed API documentation cache data: `ai-docs/**/*.local.md` and
`ai-docs/.deps/`. {#260508-bootstrap-api-deps-gitignore}

Bootstrap templates document `ai-docs/.old/` as a tracked project archive and
migrate legacy old-spec or old-material paths into that hidden archive during
versioned upgrades.

`lead-ship` follows the repository ship configuration to prepare and execute a
release. It confirms version, tag, and publish targets before any publishing
step.

`lead-exit-session` writes and commits a next-session handoff note in the
project index after handling current worktree changes according to repository
commit rules.

## Delegate Prompt Boundaries {#260505-workflow-delegate-prompt-boundaries}

Workflow skills use embedded prompt chains for named delegates such as
implementers, reviewers, skeleton populators, survey workers, and documentation
updaters. Public named-agent registrations receive delegate-orientation
instructions before role-specific prompt material.

Delegate orientation reserves lifecycle orchestration, reviewer fanout,
workflow-stage routing, and final documentation ownership for the lead unless a
delegate is explicitly assigned those responsibilities. Delegates return their
assigned output through named-agent result surfaces rather than invoking lead
skills on their own.
