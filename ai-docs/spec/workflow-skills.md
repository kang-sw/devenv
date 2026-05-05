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
lead-ship
lead-skill-authoring
lead-sprint
lead-update-spec
lead-workflow
lead-write-code
lead-write-skeleton
lead-write-spec
lead-write-ticket
```

Skill descriptions provide the natural-language trigger surface for Codex.

## Workflow Primitive Reference {#260505-workflow-primitive-reference}

`lead-workflow` is the shared primitive reference for writing or executing ws
workflow skills. It defines host-neutral notation: `ws/<tool-name>` means an MCP
tool on the `ws` server, while `ws:` names plugin skills.

Shared skill text uses ws MCP primitives for agent orchestration, scoped
queries, generated artifact paths, runtime metadata, workflow discovery, Git
operations, API documentation lookup, and project/convention reads. Skills name
only primitives available in the runtime; when a needed surface is not exposed
yet, skill text describes the required MCP contract instead of naming a
host-specific helper.

## Planning Workflow Skills {#260505-planning-workflow-skills}

Planning skills prepare caller-visible work before implementation.

`lead-discuss` explores a topic without editing source code. It loads project
context, uses scoped subqueries when search is needed, can promote or move
tickets when the discussion reaches an actionable state, and recommends an
appropriate next workflow step.

`lead-write-spec` writes or updates behavioral spec entries for caller-visible
behavior. It reads spec conventions, generates stable spec stems, writes planned
or implemented entries according to the current behavior, verifies the spec
index, and commits the spec update.

`lead-write-ticket` creates or updates workflow tickets. It applies the spec
gate for todo-or-higher tickets, uses ticket conventions, maintains queue
entries where needed, preserves stable ticket stems, and commits ticket changes.

`lead-write-skeleton` locks public contracts before implementation when the
scope needs stubs, integration tests, or interface scaffolding. It delegates
skeleton authoring, then the lead reviews, commits, and links generated
skeleton artifacts to the ticket.

## Implementation Workflow Skills {#260505-implementation-workflow-skills}

Implementation skills execute code changes and close the documentation loop.

`lead-implement` is the implementation harness. It routes to direct editing or
delegated code writing, then runs the shared post-implementation documentation
pipeline before reporting completion.

`lead-edit` performs a narrow direct edit in the lead session. It verifies the
change, uses one reviewer for correctness and fit, escalates if the scope grows,
runs spec update handling, and reports the result.

`lead-write-code` delegates an implementation target through an implementer
agent, optional plan, partitioned reviewers, bounded fix relay, cleanup, and
completion report.

`lead-update-spec` audits recent commits for caller-visible behavior changes. It
adds or updates spec entries, strips planned markers when implementation lands,
handles removed spec stems, verifies the spec index, and commits the spec pass.

## Proceed Routing Pipeline {#260505-proceed-routing-pipeline}

`lead-proceed` is the first step for implementation tasks. It is route-only: it
reads conversation state and existing workflow artifacts, then chains the needed
pipeline stages without reading source code or performing implementation work.

The pipeline order is fixed:

```text
spec -> ticket -> skeleton -> implementation
```

Existing ticket paths skip ticket creation. Actionable inline targets go through
`lead-write-ticket`; exploratory targets stop and suggest `lead-discuss`.
Implementation always routes through `lead-implement`, with
`lead-write-skeleton` inserted when the scope needs public contract scaffolding.

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
current specs after user confirmation, surveys source, tickets, archived specs,
and commit history, asks the user to confirm behavioral domains and
caller-visible classifications, writes anchor-keyed spec entries, verifies the
index, and associates planned stems with active tickets when required.

`lead-forge-mental-model` reconstructs mental-model documents from scratch. It
surveys operational domains, asks the user to confirm the domain set, writes
modification-focused domain files from current evidence, runs verification, and
commits the resulting mental-model corpus.

## Workflow Utility Skills {#260505-workflow-utility-skills}

Utility skills handle project workflow maintenance outside the main
implementation path.

`lead-add-rule` classifies a persistent rule as cross-cutting or domain-scoped
and writes it to the appropriate authority document.

`lead-bootstrap` bootstraps or upgrades downstream projects to `AGENTS.md` as
the canonical workflow context while preserving Claude compatibility through a
shim when needed.

`lead-ship` follows the repository ship configuration to prepare and execute a
release. It confirms version, tag, and publish targets before any publishing
step.

`lead-exit-session` writes and commits a next-session handoff note in the
project index after handling current worktree changes according to repository
commit rules.

## Delegate Prompt Boundaries {#260505-workflow-delegate-prompt-boundaries}

Workflow skills use embedded prompt chains for named delegates such as
implementers, reviewers, skeleton writers, survey workers, and documentation
updaters. Public named-agent registrations receive delegate-orientation
instructions before role-specific prompt material.

Delegate orientation reserves lifecycle orchestration, reviewer fanout,
workflow-stage routing, and final documentation ownership for the lead unless a
delegate is explicitly assigned those responsibilities. Delegates return their
assigned output through named-agent result surfaces rather than invoking lead
skills on their own.
