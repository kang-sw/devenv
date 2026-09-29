---
title: Claude effort-carrier subagent types for tier reasoning effort
---

# Claude effort-carrier subagent types for tier reasoning effort

## Background

Claude Code's Agent tool takes a per-call `model` but no reasoning-effort
parameter. Effort comes only from the subagent definition's `effort`
frontmatter, then the session `/effort`, then `CLAUDE_CODE_EFFORT_LEVEL`
(code.claude.com/docs/en/sub-agents.md, "Effort Level"). ws therefore never
carries tier effort into Claude spawns:

- The default `claude` tier aliases in `wsconfig` (`defaultModelAliases`) set
  model only: small=haiku, medium=sonnet, large=opus, xlarge=opus. Claude's
  large and xlarge are identical.
- `playbook.render` emits `recommended-reasoning-effort` when configured, but
  only the Codex overlay `native-spawn-binding.codex.md` maps it to a spawn
  field; the neutral `native-spawn-binding.md` is empty (578bce4dd).
- The Claude terminology entry `SpawnIdiom` is
  `Agent({subagent_type: "general-purpose", ...})`, and shipped text directs
  exploration to the built-in Explore agent.

Every Claude subagent thus inherits the lead session's effort regardless of
tier.

Where the binding must reach: the `native-spawn-binding` overlay is included
only by `lead-workflow-manual`, so only the lead reads it. Most `{{.SpawnIdiom}}`
uses are in worker bodies that never read that manual: `ticket-worker` and
`ticket-worker-elevated` spawn reviewers from `playbook.render` output, and
`ticket-worker-elevated` spawns leaves from `config.resolve_agent(tier)`
output, which also returns an effort.

Documented platform facts this ticket relies on
(code.claude.com/docs/en/sub-agents.md, plugins/components.md,
plugins/manifest-reference.md):

- Plugin agents support `effort` (`low`, `medium`, `high`, `xhigh`, `max`);
  an unsupported level for the chosen model is silently substituted with the
  nearest available one.
- Model resolution is separate: the per-call `model` parameter wins over the
  definition's `model` frontmatter.
- A plugin agent is addressed as `<plugin>:<name>`.
- The agent body replaces the default system prompt entirely; there is no
  documented way to inherit the built-in general-purpose or Explore prompt.
- The Claude manifest `agents` key takes one `.md` path or an array of `.md`
  paths (directories are rejected), each `./`-prefixed and inside the plugin
  root; it replaces the default `agents/` scan and applies to marketplace
  installs.

## Decisions

- **Effort rides on the subagent type; the model rides on the call.** One
  plugin agent per Claude effort level carries `effort` in frontmatter; the
  spawn passes the tier's model through the Agent tool's `model` parameter.
- **Claude-only surface.** The effort agents are not placed in
  `agents-plugin/agents/`, because that default location is visible to other
  harnesses. No Codex or Pi surface references them.
- **General spawns route through the effort agents; exploration does not.**
  Delegate, worker, reviewer, and leaf spawns (everything expressed through
  `{{.SpawnIdiom}}` or render bindings) use the effort agents. Exploration
  spawns stay on the built-in Explore agent with the model only, so its
  read-only tool restriction keeps mechanically enforcing "subagents gather
  evidence only" boundaries such as `lead-discuss` and the workers'
  read-only exploration. The `claude` `ExploreAgent` terminology entry and
  every Explore wording stay unchanged. Rejected: routing exploration through
  the effort agents too (drops read-only enforcement to a prompt promise, and
  a single `ExploreAgent` string would have to hard-code one effort level).
  Rejected: read-only effort-agent variants for exploration (not needed for
  this ticket's goal; a separate decision if exploration effort ever matters).
- **The nudge is delivered at render time on the Claude harness** — through
  terminology and `playbook.render` bindings, not through a per-call effort
  parameter (none exists).
- **`SpawnIdiom` carries the mapping rule itself.** The `claude` `SpawnIdiom`
  value states the rule in full: a resolved effort that is one of the five
  Claude levels selects `subagent_type: "<namespace>:effort-<level>"`, the
  resolved model goes to `model`, and an empty or non-Claude effort falls back
  to `general-purpose` with the model only. "Resolved" covers both
  `playbook.render` bindings and `config.resolve_agent` output, so worker-side
  reviewer and leaf spawns get the same rule as lead spawns without reading
  the overlay. The terminology table holds static strings and
  `SkillNamespace` is injected separately (`resolveNamespaceVars`), so the
  namespace inside this value is assembled from `RuntimeNamespace()` in Go,
  not written as a `{{.SkillNamespace}}` literal. Rejected: putting the
  mapping and fallback only in the overlay (workers never read it); naming a
  fixed effort agent in `SpawnIdiom`.
- **Render binding overlay is a lead-side aid.** New
  `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.claude.md`
  tells the lead to apply returned `recommended-model` and
  `recommended-reasoning-effort` through the `{{.SpawnIdiom}}` rule, following
  the Codex overlay precedent (b2f7caad8, 578bce4dd). It restates no separate
  mapping or fallback. The neutral `native-spawn-binding.md` stays empty.
- **Fallback labels.** No config-side label validation is added, preserving
  cbdd6d43d's unrestricted effort labels. Rejected: rejecting non-Claude
  labels in `config.tune` for the `claude` harness; mapping arbitrary labels
  to the nearest Claude level.
- **Five agents**, `effort-low`, `effort-medium`, `effort-high`,
  `effort-xhigh`, `effort-max`, addressed as `<plugin>:effort-<level>`.
- **Frontmatter is `name`, `description`, `effort` only.** No `model` (the
  call supplies it) and no `tools` restriction.
- **Body: a short, explicit general-purpose system prompt**, identical across
  the five levels: follow the task prompt, use tools to complete it, report
  results and gaps. Rejected: an empty body (the documented passthrough
  covers only `--agent` session agents, not delegated subagents); a long
  prompt approximating the built-in general-purpose or Explore prompts.
- **Generated from one template, committed.** A single template with the
  level list lives in `agents-plugin-tool/` beside an env-gated regen test,
  outside any shipped tree. The regen writes the five files into both
  `agents-plugin/claude-agents/effort-<level>.md` and
  `agents-plugin-wsflow/claude-agents/effort-<level>.md`; the output is
  committed, and a drift test fails when a committed file differs from the
  template output. Generator package, regen env var, and test names are the
  worker's choice, following the existing `WS_REGEN_*` env-gated, `-count=1`
  test pattern in `ai-docs/manuals/wsflow-mirroring.md`. Rejected: ten
  hand-maintained files (fragile); generating at `install.sh` or release time
  (marketplace installs take committed files only and never run `install.sh`,
  Architecture Rule 4); keeping the template inside a plugin tree (it would
  ship as an unlisted file).
- **Manifests stay curated.** Each package's
  `.claude-plugin/plugin.json` lists its five `./claude-agents/effort-*.md`
  files in `agents` (the key takes `.md` files, not a directory); the
  generator does not edit `plugin.json`.
- **Claude tier effort defaults.** The default `claude` aliases become
  small=haiku (no effort), medium=sonnet+`high`, large=opus+`high`,
  xlarge=opus+`max`. Rejected: model-only defaults with tuning documentation
  (leaves the mechanism inert); small=haiku+`low` (per-level Haiku support is
  undocumented).
- **wsflow ships the same five agents** in its own Claude manifest; the
  overlay and terminology resolve the namespace at render time, never a
  hard-coded `ws:` prefix, because wsflow's rsrc tree is a byte-identical
  mirror and its plugin name is `wsflow`. Rejected: excluding wsflow and
  branching the overlay on product mode.
- **Mirroring manual.** The after-edit checklist in
  `ai-docs/manuals/wsflow-mirroring.md` gains the effort-agent regen step
  beside the existing rsrc-manifest and wsflow-rsrc regen steps.
- **Verification.**
  - The drift test guards template-to-file consistency, including each file's
    `effort` matching its level.
  - A package test in each package asserts that its manifest `agents` list
    equals its `claude-agents/*.md` file set.
  - `agents-plugin/claude-agents` and `agents-plugin-wsflow/claude-agents` are
    added to the tree list in
    `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`, which
    enumerates every non-Go shipped text tree.
  - `claude plugin validate` passes for `agents-plugin` and
    `agents-plugin-wsflow`.
  - Go tests pin the default `claude` alias efforts; pin the Claude
    `SpawnIdiom` text and the Claude overlay render output for both
    namespaces; assert that the Claude `ExploreAgent` is unchanged; and assert
    that Codex and Pi renders do not mention the effort agents.
- **Live probe with stop-and-report.** Phase 1 runs a scratch headless Claude
  session that loads the working-tree plugin (for example
  `claude -p --plugin-dir <package>`), spawns `<plugin>:effort-high` and
  `general-purpose` with the same model and the same read-only prompt, and
  asks each to report its working directory, platform, date, whether project
  instructions (CLAUDE.md/AGENTS.md) are visible, and its tool list. It passes
  when the effort agent matches general-purpose on every item, and its tool
  list explicitly includes the Agent tool: `lead-run` spawns workers "in a
  form that can itself spawn children", so an effort agent without it breaks
  worker dispatch. On a gap the worker stops and records it in the Result
  rather than patching the body, because a body change revisits a lead
  decision. Rejected: no probe (the gap is undocumented and unit tests cannot
  observe it); letting the worker add environment instructions to the body.

## Constraints

- Read before editing, per `AGENTS.md` Implementation Conventions:
  `ai-docs/manuals/shipped-surface-boundary.md` (all plugin trees),
  `ai-docs/manuals/skill-authoring.md` (`rsrc/`, `skills/`),
  `ai-docs/manuals/wsflow-mirroring.md` (`rsrc/`, `skills/`, wsflow), and
  `ai-docs/manuals/ws-mcp.md` (`agents-plugin-tool/internal/mcp/`).
- `.codex-plugin/plugin.json` and Codex/Pi terminology and overlays stay
  unchanged.
- Shipped text must not name this repository's own configuration or tiers
  (Architecture Rule 4).
- Out of scope: Explore spawns keep inheriting the session effort.
- Playbook bodies that use `{{.SpawnIdiom}}` or `{{.ExploreAgent}}` need no
  text change; only the variable values change.

## Phases

### Phase 1: Claude effort agents and render-time binding

Generate and ship the effort agents on both Claude manifests, carry the
mapping rule in the Claude `SpawnIdiom` with the lead-side overlay, and seed
Claude tier effort defaults, per `## Decisions`. Exploration spawns are
untouched.

Verification: the Verification and Live probe decisions in `## Decisions`.
