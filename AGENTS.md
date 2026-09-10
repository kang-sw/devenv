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
   `ai-docs/manuals/` for procedures, and the generated ticket inventory for
   current status. Keep only context a session must not re-derive.
2. **Project arc** - run `git log --oneline --graph -50`.
3. **Binding anchor** - read this project's declared binding anchor
   (`## Workflow` -> `### Binding Anchor`) when the task touches one of its
   declared topics. The declared anchor sits under epic
   `260605-epic-ws-playbook-factory-pivot`; the prior anchor
   `260429-research-host-neutral-ws-plugin` is absorbed into this epic and
   archived under `.done/`.

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
helper commands, MCP tooling, and dev-environment templates. Tickets and
manuals here describe the workflow system itself; do not add downstream
application-domain material.

Root migration artifacts stay grouped by deliverable:

- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native tooling and MCP source tree.

Do not add loose root-level `cmd/`, `internal/`, `scripts/`, or language module
files for this migration unless a ticket changes the layout.

## Code Standards

1. **Simplicity.** Write the simplest complete implementation that satisfies the
   ticket's contract.
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

### Review Policy

```text
review-track: develop
release-boundary: present
rendezvous-backend: canary
```

`review-track` is the branch the review sweep tracks (work lands and is
reviewed on `develop`; shipping is `develop` -> `main`). `release-boundary:
present` declares that this project has a real `develop` -> `main` release
step, gated by the `lead-ship` release gate: before promoting `develop` to
`main`, ship reads the review-watermark frontier head and requires the range
since it to be clear. `rendezvous-backend: canary` uses the append-only
review-ledger canary (no GitHub branch-protection config needed) rather than
the `platform` backend, matching this project's current
single-maintainer-serial posture.

### Binding Anchor

```text
anchor: ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md
topics: plugin architecture, host-neutral migration, spawn-removal, adapter boundaries
```

`anchor` names the ticket a lead must read before answering or editing when a
target touches one of the `topics`. `lead-discuss` reads it directly, and the
routed run path injects it through the generic binding-anchor hook rather than
naming this repository's anchor in shipped text. Both keys are required: a
project that declares no such section (or only one key) has no binding-anchor
gate, and the
proceed fact normalizes to `n/a`.

### Implementation Conventions

| manual | paths |
|---|---|
| `ai-docs/manuals/skill-authoring.md` | `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/` |
| `ai-docs/manuals/wsflow-mirroring.md` | `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/` |
| `ai-docs/manuals/ws-mcp.md` | `agents-plugin-tool/internal/mcp/` |

Each row's manual is read before editing a file its `paths` cover. This is a
live obligation, not a pointer list: the worker playbook reads every matching
row's manual before editing, and ticket fact population copies matching rows
into a ticket's `## Constraints`. The first two rows were already required -
`skill-authoring.md` by `## Code Standards` above, `wsflow-mirroring.md` by its
own `summary:` - and declaring them here is what makes that requirement reach a
path-scoped reader; the `ws-mcp.md` row is new, added with the tool-output
convention that manual now carries. Adding a row binds every future edit under
its `paths`, so add one only for a manual whose rules a change to those paths
must not contradict.

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
```

Keep unrelated untracked files out of commits. `.codex` may exist locally; do
not stage it unless explicitly requested.

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the task.
- Update drifted docs on contact.

## Architecture Rules

1. **Workflow repo scope.** Tickets and manuals describe the ws workflow system
   itself; downstream application rules belong in downstream projects.
2. **Grouped migration layout.** `agents-plugin/` and `agents-plugin-tool/` own
   the Codex/plugin-runtime migration surface. Do not introduce new root module
   directories without a ticket.
3. **Host-neutral first.** Shared skill text should prefer canonical MCP tool
   names and host-neutral behavior. Treat Claude-specific commands and paths as
   adapter or fallback behavior.
4. **Shipped surfaces are downstream-first. This is not negotiable.**
   Every playbook, skill, convention, and template under `agents-plugin/`
   and `agents-plugin-wsflow/`, every embedded convention, and every string
   `agents-plugin-tool/` emits to an agent (todo instructions, advisories,
   banners, doctor checks, tool descriptions) runs inside projects that are
   not this one and that hold only what bootstrap installs. The test for every sentence of such
   text: **does it depend on something a downstream project does not
   have?** If yes, it is a leak. Concretely, shipped text MUST NOT name:
   - a ticket of this repository: a real `26xxxx-...` stem, an
     `ai-docs/tickets/...` path to one, an epic name, or a bare ticket
     number used as a citation (example stems that resolve to nothing are
     fine; ticket-directory names and `<status>/<stem>` placeholders are
     ws conventions and fine);
   - a commit hash;
   - a specific file of this repository that bootstrap does not install
     (`ai-docs/ref/worktree-ticket-scope.md`, the `skill-authoring` manual),
     or this repository's own layout and tooling: `agents-plugin/`,
     `agents-plugin-tool/`, `agents-plugin-wsflow/`, `claude-plugin/`,
     `install.sh`, `wsflow-mirroring`;
   - this repository's migration vocabulary as a rule: migration anchor,
     native-subagent pivot, spawn-removal, host-neutral migration, adapter
     boundaries, retired Claude tree, Codex-first.
   A rule this file imposes on sessions in this repository is a rule for this
   repository, not a rule the shipped playbooks impose on every project. When
   shipped text needs project-specific input it reads it through a generic
   hook (`infra.read`, `convention.read`, a declared `AGENTS.md` section, a
   `config.list` key) and this repository declares its
   own value behind that hook. A ticket that asks shipped playbooks to
   "honor" or "enforce" a rule from this file is asking for a leak; push
   back and redirect it to a hook. A test that pins shipped text pins the
   leak too: a pinned devenv-only string is a bug in the test as well as in
   the text. Before committing any change under the shipped surfaces,
   re-read the changed text as a lead in a project that has never heard of
   devenv.
5. **Shell state is ephemeral.** Shell state does not persist between tool calls;
   values needed later must be captured from output and passed explicitly.
6. **Retired Claude tree.** Do not reintroduce `claude-plugin/`; preserve
   historical Claude material under `ai-docs/ref/` when needed.
## Documentation System

- Project orientation: this file's `## Project Orientation` section.
- Volatile or tracked session notes: the `repo` note layer
  (`ai-docs/ws-notes/`, written via `ws/note.write(layer: "repo", ...)`).
- Tickets: `ai-docs/tickets/`
- Static references: `ai-docs/ref/`
- Skill/agent authoring: `ai-docs/manuals/skill-authoring.md`
- Codex behavior notes: `ai-docs/manuals/codex-integration.md`
- MCP operational runbook: `ai-docs/manuals/ws-mcp.md`
- Tracked archive of retired material: `ai-docs/.old/`

There is no spec or mental-model layer. Caller-visible behavior is the test
suite; the code is the structure; a non-derivable trap is a comment at the site
that bites.

Before editing:

- Skills, agents, prompts, or convention docs: read
  `ai-docs/manuals/skill-authoring.md`.
- Tickets: read ticket conventions through `ws/convention.read` or the
  bundled convention fallback.

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
- `todo/` is accepted backlog; `ready/` is the implementation-ready status.
- Research tickets use freeform topic sections and no phases.
- Actionable tickets use `## Phases` and stable `### Phase N: <title>`.
- Do not edit phase plan text after it has a `### Result` section; append
  `#### Edition (<short-hash>) - YYYY-MM-DD` for later implementation tweaks.
- To check ticket completion or prior phase results, use
  `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- All AI-authored ticket content must be English.

## Project Orientation

<!-- Every-session orientation: repo identity, project map/topology, and
     canonical flows. Keep compact; route deep detail to manuals. -->

- **Repo identity.** Meta-workflow repository for workflow documents, skills,
  agents, plugin packaging, helper commands, MCP tooling, and dev-environment
  templates. Tickets and manuals here describe the workflow system itself;
  downstream application material belongs in downstream projects. Active plugin
  package: `agents-plugin/` (`ws@0.45.2`). Agentless
  derivative package: `agents-plugin-wsflow/` (`wsflow@0.45.2`). Native
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
    Codex UI install has verified the ticket-authoring and discussion skills
    (`ws:lead-ticket` since its rename from `ws:lead-write-ticket`).
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
  Full ceremony:  discuss -> ticket -> run -> review
  Direct:         run <ticket-path or description>
  Queue:          run (no argument) -> next ready/ ticket, one worker per cycle
  Review:         review [branch|range] -> verdict -> (fix via run | comment | discuss | merge)
  Release:        ship <project> -> release gate -> execute
  ```
  The user decides the next step at each handoff. `run` spawns one worker per
  invocation; the worker, not the lead, executes the ticket.

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
     AND it applies everywhere, keep it here. Domain-scoped rules belong in a
     manual under `ai-docs/manuals/`, declared in `## Workflow` ->
     `### Implementation Conventions`, via `ws:lead-add-rule`.
     Context goes in this file's `## Project Orientation` section or the
     `repo` note layer; process goes in skills. -->

<!-- Template Version: v0047 -->
