---
title: Claude effort-carrier subagent types for tier reasoning effort
related:
  260923-feat-agent-tier-arbitrary-effort-and-codex-defaults: background (unrestricted effort labels)
  260622-feat-playbook-render-tier-label: background (render bindings and native-spawn-binding overlays)
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: e048a6afe7ee614b
sage-review-design-reviewed: e048a6afe7ee614b
---

# Claude effort-carrier subagent types for tier reasoning effort

## Background

Claude Code's Agent tool takes a per-call `model` but no reasoning-effort
parameter. Effort comes only from the subagent definition's `effort`
frontmatter, then the session `/effort`, then `CLAUDE_CODE_EFFORT_LEVEL`
(code.claude.com/docs/en/sub-agents.md, "Effort Level"). ws therefore never
carries tier effort into Claude spawns:

- The default `claude` tier aliases in `wsconfig` (`defaultModelAliases`)
  already carry efforts (0e0d5926a, f50d4b4f7):
  small=haiku+`high`, medium=sonnet+`high`, large=opus+`high`,
  xlarge=opus+`xhigh`. Nothing on Claude consumes them yet.
- `playbook.render` emits `recommended-reasoning-effort` when configured, but
  only the Codex overlay `native-spawn-binding.codex.md` maps it to a spawn
  field; the neutral `native-spawn-binding.md` is empty (578bce4dd).
- `ticket-reviewer-design` tells reviewers to record unavailable bindings
  under `omitted:`, so Claude design reviewers list the effort binding there.
  This entry persists after this ticket for reviewer explorers, which stay on
  the built-in Explore agent without an effort binding (see `## Decisions`).
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
  The split is by variable, not by playbook: every `{{.SpawnIdiom}}` spawn,
  including those in the `explore` playbook, routes through the effort
  agents, and every `{{.ExploreAgent}}` spawn stays on Explore.
- **The nudge is delivered at render time on the Claude harness** — through
  terminology and `playbook.render` bindings, not through a per-call effort
  parameter (none exists).
- **`SpawnIdiom` carries the mapping rule itself.** The `claude` `SpawnIdiom`
  value states the rule in full: a resolved effort that is one of the five
  Claude levels selects `subagent_type: "<namespace>:effort-<level>"`, the
  resolved model goes to `model`, and an empty or non-Claude effort falls back
  to `general-purpose` with the model only. The value stays one short
  sentence, shaped like
  `Agent({subagent_type: "<namespace>:effort-<resolved effort>", model: <resolved model>, ...}), or "general-purpose" when no Claude effort level resolved`,
  because it is substituted mid-sentence at every `{{.SpawnIdiom}}` site
  (`lead-run`, `ticket-worker`, `ticket-worker-elevated`, `explore`,
  `delegate-sample`). "One of the five Claude levels" is an exact match on
  the lowercase level names; any other label, including case or whitespace
  variants, takes the fallback. Rejected: a short `SpawnIdiom` plus the rule in a shared
  worker include (the one-sentence form reads acceptably at every site).
  Rejected: a spawn-failure fallback clause (retry with `general-purpose` when
  the Agent tool rejects the effort agent type) - plugin custom agents are a
  baseline harness capability, and a failure is left to the agent's own
  handling. "Resolved" covers both
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
  The Claude overlay must not reuse Codex-only wording: the existing negative
  Claude-render assertion from 578bce4dd (in `playbook_tools_test.go`) forbids
  `## Native delegate spawn`, `spawn_agent.model`,
  `spawn_agent.reasoning_effort`, and `fork_turns: "none"` in the Claude
  render, and it stays as is, so the overlay uses its own heading. For the
  overlay's `{{.SpawnIdiom}}` to render, `lead-workflow-manual` declares
  `SpawnIdiom` in its frontmatter (a frontmatter-only change), mirrored to
  wsflow with the rsrc manifest regen.
- **Fallback labels.** Every shipped Claude tier carries a Claude effort
  level, so the empty-effort fallback fires only for user-configured empty or
  non-Claude labels. No config-side label validation is added, preserving
  cbdd6d43d's unrestricted effort labels. Rejected: rejecting non-Claude
  labels in `config.tune` for the `claude` harness; mapping arbitrary labels
  to the nearest Claude level.
- **Five agents**, `effort-low`, `effort-medium`, `effort-high`,
  `effort-xhigh`, `effort-max`, addressed as `<plugin>:effort-<level>`.
- **Frontmatter is `name`, `description`, `effort` only.** No `model` (the
  call supplies it) and no `tools` restriction.
- **Body: lead-authored, already written.** The frontmatter and body prose
  are fixed in `agents-plugin-tool/internal/claudeagents/effort-agent.md.tmpl`,
  identical across the five levels. The worker wires the generator to it and
  may adjust template syntax, never wording; a wording problem found during
  implementation is a stop-and-report, because worker-written prose drifts.
  The body keeps the built-in general-purpose identity sentence verbatim
  (user observation: agents without it tend to perform worse) and restates the rest of the
  built-in guidance in ws wording: finish completely within scope, search
  broad then narrow, prefer edits over new files and no unrequested docs, no
  whole-assignment re-delegation, concise relayable report. It adds one
  precedence paragraph: when the task directs the agent to read a file as its
  system prompt or instructions, that file governs and wins on conflicts
  (scope, files to write, report format); without such a file the body
  applies. ws spawns pass rendered playbooks that way, and the body's
  generic defaults (no new docs, concise report) would otherwise compete with
  playbook duties (Results, commits, fixed report shapes). The wording stays
  generic (no ws vocabulary) so ad-hoc delegate spawns without a file still
  work. Rejected: an empty body (the documented passthrough covers only
  `--agent` session agents, not delegated subagents); copying the built-in
  prompt verbatim (redistributes Claude Code's commercially licensed text in
  an MIT package); extracting the built-in prompt from the local Claude Code
  binary at install or first run (plugin agents load at session start before
  any ws code can write them, a plugin-cache write is lost on refresh, and
  extraction depends on per-build minified names and per-platform install
  shapes); specializing the body further with ws procedure (it would become a
  second playbook and drift from the real one).
- **Generated from one template, committed.** The template above lives
  outside any shipped tree, and the generator plus its env-gated regen test
  live in the same `internal/claudeagents` package. The regen writes the five
  files into both
  `agents-plugin/claude-agents/effort-<level>.md` and
  `agents-plugin-wsflow/claude-agents/effort-<level>.md`; the output is
  committed, and a drift test fails when a committed file differs from the
  template output. Regen env var and test names are the
  worker's choice, following the existing `WS_REGEN_*` env-gated, `-count=1`
  test pattern in `ai-docs/manuals/wsflow-mirroring.md`. Rejected: ten
  hand-maintained files (fragile); generating at `install.sh` or release time
  (marketplace installs take committed files only and never run `install.sh`,
  Architecture Rule 4); keeping the template inside a plugin tree (it would
  ship as an unlisted file).
- **Manifests stay curated.** Each package's
  `.claude-plugin/plugin.json` lists its five `./claude-agents/effort-*.md`
  files in `agents` (the key takes `.md` files, not a directory); the
  generator does not edit `plugin.json`. Neither package has an `agents/`
  directory or an existing Claude `agents` key today, so the curated list
  hides no agent Claude currently loads.
- **Claude tier effort defaults (already landed).** small=haiku+`high`,
  medium=sonnet+`high`, large=opus+`high`, xlarge=opus+`xhigh`, landed ahead
  of this ticket (0e0d5926a, f50d4b4f7); this ticket makes
  them effective. No shipped default, Claude or Codex, uses `max`. Haiku gets
  `high` because Claude substitutes an unsupported level with the nearest
  available one, and a shipped default should not route small-tier spawns
  into the fallback. Rejected: model-only defaults with tuning documentation
  (leaves the mechanism inert); small=haiku with no effort (triggers the
  fallback by default); xlarge=opus+`max`.
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
  - Go tests pin the Claude `SpawnIdiom` text as rendered at each
    `{{.SpawnIdiom}}` site and the Claude overlay render output for both
    namespaces; assert that the Claude `ExploreAgent` is unchanged; and assert
    that Codex and Pi renders do not mention the effort agents. The default
    alias efforts are already pinned by the landed default change.
- **Live probe with stop-and-report.** Phase 1 runs a scratch headless Claude
  session that loads the working-tree plugin (for example
  `claude -p --plugin-dir <package>`), spawns `<plugin>:effort-high` and
  `general-purpose` with the same model and the same read-only prompt, and
  asks each to report its working directory, platform, date, whether project
  instructions (CLAUDE.md/AGENTS.md) are visible, and its tool list. It passes
  when the effort agent matches general-purpose on every item, and its tool
  list explicitly includes the Agent tool: `lead-run` spawns workers "in a
  form that can itself spawn children", so an effort agent without it breaks
  worker dispatch. A second probe spawn gives the effort agent a task that
  says to read a scratch instructions file as its system prompt, where that
  file demands a fixed marker line in the report and the creation of a
  scratch `.md` file; it passes when the agent follows the file over the
  body's defaults. On a gap the worker stops and records it in the Result
  rather than patching the body, and does not close the ticket; the lead
  decides before merge, because a body change revisits a lead
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 0e0d5926a (2026-09-29, commit): "Claude tier efforts are seeded ahead of 260928-feat-claude-effort-carrier-agents. They are inert on Claude until the effort agents land, since nothing consumes Claude recommended-reasoning-effort yet" — bearing: supports
- f50d4b4f7 (2026-09-29, commit): "User decision; supersedes the 'Haiku keeps no effort' note in 0e0d5926a. ... A shipped default should not route Claude small-tier spawns into the empty-effort fallback path" — bearing: supports
- 260923-feat-agent-tier-arbitrary-effort-and-codex-defaults (2026-09-23, Decisions): "the fixed portable effort vocabulary is replaced by any non-empty label (user-confirmed in 51d9588a)." — bearing: constrains
- 371b1176 (2026-09-24, commit): "Empty effort still renders empty in the tier vars (formatEffortForText leaves it unchanged), preserving the 'omit the host binding' contract." — bearing: constrains
- 260622-feat-playbook-render-tier-label (2026-07-21, Result edition 578bce4d): "Claude now receives no added native binding guidance; the empty neutral include preserves its prior..." — bearing: constrains
- 578bce4dd (2026-07-21, commit): "Remove shared explanations of harness selection, tier portability, binding resolution, exact-fidelity policy, and continuation mechanics from the always-loaded manual. ... add a negative Claude-render assertion for Codex-only spawn guidance." — bearing: constrains
- 260622-feat-playbook-render-tier-label (2026-06-22, Decisions): "Resolve model and effort through the existing harness-aware `wsconfig.ResolveAgentForHarnessConfig` seam shared with `RoleModel` and the fixed-tier model render variables. Do not hardcode Claude or Codex model names" — bearing: supports
- 91378dc5 (2026-06-09, commit): "terminologyForHarness('') returns host-neutral values for any unrecognized harness. Tests assert claude != codex != '' to prevent silent collapsing." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/claudeagents/, agents-plugin-tool/internal/mcp/playbook_tools.go, agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.claude.md, agents-plugin/claude-agents/, agents-plugin-wsflow/claude-agents/, both .claude-plugin/plugin.json, agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py, ai-docs/manuals/wsflow-mirroring.md |
| scope.surface | public-interface | new plugin agent types addressed as <plugin>:effort-<level> in both Claude manifests plus the claude SpawnIdiom value rendered at five playbook sites |
| scope.new_public_symbol | yes | five plugin agents effort-low, effort-medium, effort-high, effort-xhigh, effort-max per package |
| scope.new_type_contract | no | no exported Go type or signature; generator lives in the internal claudeagents package and the agent contract is frontmatter text |
| scope.test_surface | new-files | new drift and manifest-parity tests; existing agents-plugin-tool/internal/mcp/playbook_tools_test.go and agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py extended |
| complexity.reuse_points | confirmed | playbookTerminologyTable and RuntimeNamespace in internal/mcp, wsrsrc loader name.harness.md overlay lookup at loader.go#L151-L161, native-spawn-binding.codex.md precedent, WS_REGEN env-gated regen pattern, existing template effort-agent.md.tmpl |
| complexity.side_effect_risk | moderate | the claude SpawnIdiom change alters every Claude spawn instruction in lead-run, ticket-worker, ticket-worker-elevated, explore, delegate-sample and the manifest agents key replaces the default agents scan |
| risk.correctness | moderate | the mapping rule must render the right namespace per product mode and fall back cleanly for empty or non-Claude effort labels |
| risk.fit | moderate | two packages, byte-identical wsflow rsrc mirror, manifest regen, and mirroring-manual checklist must stay in sync |
| risk.test | moderate | drift, manifest parity, render pins across five sites and two namespaces, plus a live headless probe of undocumented agent environment behavior |
| risk.security_or_contract | moderate | changes the spawn contract for every Claude subagent and adds shipped plugin agents whose body must not redistribute Claude Code's built-in prompt |

## Phases

### Phase 1: Claude effort agents and render-time binding

Generate and ship the effort agents on both Claude manifests and carry the
mapping rule in the Claude `SpawnIdiom` with the lead-side overlay, per
`## Decisions`, making the already-landed Claude tier effort defaults
effective. Exploration spawns are untouched.

Verification: the Verification and Live probe decisions in `## Decisions`.
