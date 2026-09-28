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
- **All native Claude spawns route through the effort agents, best effort.**
  This includes exploration spawns that today use the built-in Explore agent.
  Read-only enforcement is not preserved or reintroduced; the effort agents do
  not restrict tools.
- **The nudge is delivered at render time on the Claude harness** — through
  `playbook.render` bindings and Claude-harness text, not through a
  per-call effort parameter (none exists).

- **Placement: static files listed in the Claude manifest.** The agents live
  at `agents-plugin/claude-agents/effort-<level>.md`, each listed in
  `agents-plugin/.claude-plugin/plugin.json` `agents` (the key takes `.md`
  files, not a directory). Rejected: generating them at `install.sh` time,
  because marketplace installs never run `install.sh` (Architecture Rule 4).
- **Five agents**, `effort-low`, `effort-medium`, `effort-high`,
  `effort-xhigh`, `effort-max`, addressed as `<plugin>:effort-<level>`.
- **Frontmatter is `name`, `description`, `effort` only.** No `model` (the
  call supplies it) and no `tools` restriction.
- **Body: a short, explicit general-purpose system prompt**, identical across
  the five files: follow the task prompt, use tools to complete it, report
  results and gaps. Rejected: an empty body (the documented passthrough
  covers only `--agent` session agents, not delegated subagents); a long
  prompt approximating the built-in general-purpose or Explore prompts.
- **Render binding via a Claude overlay.** New
  `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.claude.md`
  maps `recommended-reasoning-effort` to
  `subagent_type: {{.SkillNamespace}}:effort-<value>` and `recommended-model`
  to `model`, following the Codex overlay precedent (b2f7caad8, 578bce4dd).
  The neutral `native-spawn-binding.md` stays empty.
- **Terminology.** The `claude` entries `SpawnIdiom` and `ExploreAgent` in
  `playbookTerminologyTable` are rewritten to name the effort agents with the
  model override.
- **Literal Explore mentions become `{{.ExploreAgent}}`.** Shipped playbooks
  that name "the Explore agent" literally (for example `lead-discuss` and the
  workflow manual's scoped-exploration section) use the existing variable so
  the Claude rewrite reaches them. Rejected: leaving the literals and adding a
  single Claude-overlay rule to the workflow manual.
- **Fallback.** When the resolved effort is empty or not one of the five
  Claude labels, the Claude binding falls back to the built-in spawn
  (`general-purpose`, or Explore for exploration) with the model only. No
  config-side label validation is added, preserving cbdd6d43d's unrestricted
  effort labels. Rejected: rejecting non-Claude labels in `config.tune` for
  the `claude` harness; mapping arbitrary labels to the nearest Claude level.
- **Claude tier effort defaults.** The default `claude` aliases become
  small=haiku (no effort), medium=sonnet+`high`, large=opus+`high`,
  xlarge=opus+`max`. Rejected: model-only defaults with tuning documentation
  (leaves the mechanism inert); small=haiku+`low` (per-level Haiku support is
  undocumented).
- **wsflow ships the same five agents** in its own Claude manifest; the
  overlay and terminology address agents through `{{.SkillNamespace}}`, never a
  hard-coded `ws:` prefix, because wsflow's rsrc tree is a byte-identical
  mirror and its plugin name is `wsflow`. Rejected: excluding wsflow and
  branching the overlay on product mode.
- **Verification.** A package test asserts that each package's manifest
  `agents` list equals its `claude-agents/*.md` file set and that each file's
  `effort` matches its filename level; `claude plugin validate` passes for
  `agents-plugin` and `agents-plugin-wsflow`; Go render tests pin the Claude
  overlay and terminology output and assert that Codex renders do not mention
  the effort agents.
- **Live probe with stop-and-report.** Phase 1 runs a scratch headless Claude
  session that loads the working-tree plugin (for example
  `claude -p --plugin-dir <package>`), spawns `<plugin>:effort-high` and
  `general-purpose` with the same model and the same read-only prompt, and
  asks each to report its working directory, platform, date, whether project
  instructions (CLAUDE.md/AGENTS.md) are visible, and its tool list. It passes
  when the effort agent matches general-purpose on every item. On a gap the
  worker stops and records it in the Result rather than patching the body,
  because a body change revisits a lead decision. Rejected: no probe (the
  gap is undocumented and unit tests cannot observe it); letting the worker
  add environment instructions to the body.

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

## Phases

### Phase 1: Claude effort agents and render-time binding

Ship the effort agents on the Claude manifest, bind rendered and lead-direct
Claude spawns to them, and seed Claude tier effort defaults, per
`## Decisions`.

Verification: the Verification and Live probe decisions in `## Decisions`.
