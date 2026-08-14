# AGENTS.md - devenv

## Current Authority

`AGENTS.md` is the canonical root workflow context for this repository.
`CLAUDE.md` is a compatibility shim whose body is `@AGENTS.md`.

This repo is still mid-migration from a Claude-centered workflow to a
host-neutral Agents/open-conventions workflow. Treat these as authoritative until
a ticket replaces them:

- `AGENTS.md` - root behavioral rules, project-specific invariants, and (via
  `## Project Orientation`) project memory and orientation.
- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native MCP/tooling source tree.
- Root `CLAUDE.md` - compatibility shim whose body is `@AGENTS.md`.

If shared host-neutral guidance and Claude compatibility guidance conflict,
follow the more conservative rule and surface the conflict before changing
workflow semantics.

## Project Memory

Read at every session start, before other action:

1. **Preamble** - repo identity, plugin topology, and canonical flows live in
   this file's `## Project Orientation` section below; read repo-tracked notes
   (`ws/note.search(layer: "repo")`) for volatile session context,
   `ai-docs/manuals/` for procedures, and generated ticket/spec inventories for
   current status. Keep only context a session must not re-derive.
2. **Local** - read `ai-docs/_index.local.md` if present; it is .gitignored
   clone context.
3. **Project arc** - run `git log --oneline --graph -50`.
4. **Migration anchor** - read
   `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` (under epic
   `260605-epic-ws-playbook-factory-pivot`) when the task touches plugin
   architecture, host-neutral migration, the spawn-removal pivot, or adapter
   boundaries. The prior anchor `260429-research-host-neutral-ws-plugin` is
   absorbed into this epic and archived under `.done/`.

## Response Discipline

- **Evidence before claims.** Run verification and read output before stating
  success.
- **No performative agreement.** Restate the requirement, verify, then act or
  push back.
- **Dogfood surprises get captured.** When a ws tool behaves contrary to
  reasonable caller expectations during dogfooding, create a short `idea/`
  ticket immediately when the surprise implies a bug, feature, or research
  follow-up. Mention the ticket in the next natural status or final response;
  interrupt the workflow only when the surprise blocks progress or changes the
  user-visible outcome.
- **Actions over words.** Prefer "Fixed. [what changed]" or the diff. Skip
  filler.

## Project Scope

This is a meta-workflow repo: workflow docs, skills, agents, plugin packaging,
helper commands, MCP tooling, and dev-environment templates. Specs, tickets, and
mental models here describe the workflow system itself; do not add downstream
application-domain material.

Root migration artifacts stay grouped by deliverable:

- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native tooling and MCP source tree.

Do not add loose root-level `cmd/`, `internal/`, `scripts/`, or language module
files for this migration unless a ticket changes the layout.

## Code Standards

1. **Simplicity.** Write the simplest complete implementation that satisfies the
   spec.
2. **Surgical changes.** Change only what the task requires; follow existing
   style.
3. **Responsibility check.** Keep module roles clean; split when responsibility
   drifts.
4. **Testability.** Prefer explicit dependencies, minimal hidden state, and pure
   logic over side effects.
5. **Skill/agent authoring.** Before editing skills, agents, prompts, or
   convention docs, read `ai-docs/manuals/skill-authoring.md`
   and apply its invariant checklist to every changed Invariants/Constraints
   line.

## Workflow

### Approval Protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, boilerplate,
  single-module refactors, and documentation updates that preserve existing
  semantics.
- **Ask first:** new skills/agents, cross-skill interfaces, template changes that
  affect downstream projects, convention changes, architecture changes, and
  observable workflow behavior changes.
- **Always ask:** deleting skills/agents, changing canonical flows, modifying
  migration checklist semantics, deleting functionality, or changing protocol/API
  semantics.

### Branch Policy

The `main` (release/master) branch is itself a release artifact. Do not push
routine, non-release work directly to it; frequent non-release pushes to the
release branch are costly. Land day-to-day work on `develop` or a feature/topic
branch and push there, reserving `main` for release-worthy merges. Local commits
on `main` are fine when a flow calls for them, but only push `main` when the
merge is a release (or the user explicitly asks).

### Commit Rules

Auto-create one commit per logical unit unless the user asks not to commit.
Include `## AI Context` explaining why the approach was chosen.

```text
<type>(<scope>): <summary>

<what changed - brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional - ticket-driven only
- <ticket-stem>[: <optional-label>]
  > Forward: <future-phase finding>

## Spec                                    # optional - omit when none
- <spec-stem>
```

When a spec heading `{#slug}` changes, include
`renamed-spec: <old-stem> -> <new-stem>`.

Keep unrelated untracked files out of commits. `.codex` may exist locally; do
not stage it unless explicitly requested.

**Version bump on merge into `develop`.** `develop` is the default integration
branch; routine work merges there, not into `main`. Every merge into `develop`
(from any feature, topic, `impl/*`, or `goal/*` branch) bumps the plugin patch
version through `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>`. Never
hand-edit the version edition points (both `plugin.json` pairs, both
`runtime.json`, `main.go`, release assets, this file's `## Project Orientation`
version strings); the script is the single bump surface. Claude Code keys
plugin-cache invalidation on the `version` string, so an unchanged version
serves stale builds even across branch-pin reinstalls — bump per merge so each
dogfood build is distinct. `develop -> main` happens only at shipping, which
owns the release version through the ship procedure, so no bump rides that
merge.

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the task.
- Update drifted docs on contact.

## Architecture Rules

1. **Workflow repo scope.** Specs, tickets, and mental models describe the ws
   workflow system itself; downstream application rules belong in downstream
   projects.
2. **Grouped migration layout.** `agents-plugin/` and `agents-plugin-tool/` own
   the Codex/plugin-runtime migration surface. Do not introduce new root module
   directories without a ticket.
3. **Host-neutral first.** Shared skill text should prefer canonical MCP tool
   names and host-neutral behavior. Treat Claude-specific commands and paths as
   adapter or fallback behavior.
4. **Shell state is ephemeral.** Shell state does not persist between tool calls;
   values needed later must be captured from output and passed explicitly.
5. **Retired Claude tree.** Do not reintroduce `claude-plugin/`; preserve
   historical Claude material under `ai-docs/ref/` when needed.
## Documentation System

- Project orientation: this file's `## Project Orientation` section.
- Volatile or tracked session notes: the `repo` note layer
  (`ai-docs/ws-notes/`, written via `ws/note.write(layer: "repo", ...)`).
- Tickets: `ai-docs/tickets/`
- Specs: `ai-docs/spec/`
- Mental models: `ai-docs/mental-model/`
- Static references: `ai-docs/ref/`
- Skill/agent authoring: `ai-docs/manuals/skill-authoring.md`
- Codex behavior notes: `ai-docs/manuals/codex-integration.md`
- MCP behavior contracts: `ai-docs/spec/mcp-tools.md` and
  `ai-docs/spec/plugin-runtime.md`
- MCP operational runbook: `ai-docs/manuals/ws-mcp.md`

Before editing:

- Skills, agents, prompts, or convention docs: read
  `ai-docs/manuals/skill-authoring.md`.
- Tickets: read ticket conventions through `ws/convention.read` or the
  bundled convention fallback.
- Specs: read spec conventions through `ws/convention.read` or the compatibility
  bundled convention fallback.
- Mental models: read mental-model conventions through `ws/convention.read` or
  the bundled convention fallback.

## Ticket System

Status is directory-based:

```text
ai-docs/tickets/idea/
ai-docs/tickets/todo/
ai-docs/tickets/ready/
ai-docs/tickets/.done/
ai-docs/tickets/.dropped/
```

- Reference tickets by stem, not path: `260429-research-host-neutral-ws-plugin`.
- Creation-date prefixes are stable; never rename to change the date.
- Move status with `git mv` when possible.
- `todo/` is accepted backlog; `ready/` is the spec-addressed implementation-ready status.
- Research tickets use freeform topic sections and no phases.
- Actionable tickets use `## Phases` and stable `### Phase N: <title>`.
- Do not edit phase plan text after it has a `### Result` section; append
  `#### Edition (<short-hash>) - YYYY-MM-DD` for later implementation tweaks.
- To check ticket completion or prior phase results, use
  `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- All AI-authored ticket content must be English.

## Project Orientation

<!-- Every-session orientation: repo identity, project map/topology, and
     canonical flows. Keep compact; route deep detail to specs, mental
     models, or manuals. -->

- **Repo identity.** Meta-workflow repository for workflow documents, skills,
  agents, plugin packaging, helper commands, MCP tooling, and dev-environment
  templates. Specs, tickets, and mental models here describe the workflow
  system itself; downstream application material belongs in downstream
  projects. Active plugin package: `agents-plugin/` (`ws@0.40.3`). Agentless
  derivative package: `agents-plugin-wsflow/` (`wsflow@0.40.3`). Native
  MCP/tooling source: `agents-plugin-tool/`. Retired Claude source material:
  `ai-docs/ref/claude-home-legacy.md` and git history.
- **Project map / topology.**
  - `./install.sh update` handles first-time install and settings patching on
    a new machine.
  - Root `CLAUDE.md` is the only live Claude compatibility shim and points at
    `AGENTS.md`.
  - `install.sh` snapshots only `agents-plugin/` for Claude-compatible plugin
    installs when Claude Code is available; it intentionally does not install
    wsflow into Claude.
  - `agents-plugin/` is registered through `.agents/plugins/marketplace.json`;
    Codex UI install has verified `ws:lead-write-ticket` and `ws:lead-discuss`.
  - `agents-plugin-wsflow/` is an agentless derivative package with
    Codex/Claude manifests, package-local no-agent MCP env, shared launcher
    copies, a reduced `runtime.json`, thin wsflow skill shims over shared
    playbooks, and package tests for runtime-contract plus skill-shim drift.
  - `.agents/plugins/marketplace.json` exposes both `ws` and `wsflow` as local
    Codex plugin entries; `.claude-plugin/marketplace.json` exposes both
    packages for manual Claude marketplace installation while `install.sh`
    still installs only `ws`.
  - Codex local plugin iteration has no known CLI refresh path; use UI
    uninstall/install or a fresh Codex session after editing the registered
    source.
  - `agents-plugin/.codex-plugin/plugin.json` references plugin-local
    `.mcp.json` through `"mcpServers": "./.mcp.json"`.
  - Changed plugin-managed Codex MCP config requires user-performed plugin
    cache refresh before installed-cache verification.
  - `claude plugin validate agents-plugin` passes; runtime Claude invocation
    of `agents-plugin` remains compatibility behavior, not a separate source
    tree.
  - `ai-docs/.old/` is the Git-tracked project archive for inactive reference
    material that should not appear in default file listings.
  - MCP tool schemas and inventory are runtime-discoverable through
    `tools/list` and runtime capabilities; do not copy them into project
    memory or reference docs.
  - Skill/prompt/agent inventory lives in the `agents-plugin/skills/` source
    tree and plugin manifest/tests; not duplicated here.
- **Canonical flows.**
  ```text
  Full ceremony:  discuss -> proceed -> implement -> review/docs/final gate
  Direct:         implement <description>
  Auto-route:     proceed <ticket-path>
  Sprint:         sprint -> discuss/explore -> sprint-edit episode? -> episode closure or normal handoff
  Review:         review [branch] -> verdict -> (discuss -> fix | comment | merge)
  Recovery:       salvage -> research report -> recovery epic? -> child tickets
  ```
  User decides next step at each handoff. `proceed` is the explicit opt-in for
  auto-chaining through the pipeline.

## Project Knowledge

- **Language:** AI-authored docs, plans, commits, tickets, and code comments are
  English. Human-facing UI strings are exempt.
- Workflow shape and plugin-less maintenance guidance live in
  `ai-docs/WORKFLOW.md`; it is explanatory and does not override ws runtime or
  MCP parser behavior.
- Current priority is making the project and ticket system usable from
  Agents/Codex while retiring the legacy Claude tree behind explicit tickets.
- Research anchor: `260605-research-ws-native-subagent-pivot` (direction detail),
  coordinated by epic `260605-epic-ws-playbook-factory-pivot`. The earlier
  `260429-research-host-neutral-ws-plugin` anchor is absorbed and archived.
- Existing historical Claude workflow notes may mention `ws-*` on `PATH`; new
  shared guidance should use MCP tools and bundled runtime documents.
- Claude plugin source artifacts were retired from the live tree; do not add a
  new `claude-plugin/` mirror for Codex behavior.

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules` via `ws:lead-add-rule`.
     Context goes in this file's `## Project Orientation` section or the
     `repo` note layer; process goes in skills. -->

<!-- Template Version: v0046 -->
