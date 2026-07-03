---
title: "Warn on stale downstream bootstrap template version at session-bootstrap time"
related:
  260605-research-ws-native-subagent-pivot: precedent for tip-injection-point discipline (agentId-continuity tip is action-time-only, not per-call)
sage-review: required
---

# Warn on stale downstream bootstrap template version at session-bootstrap time

## Background

`lead-bootstrap` stamps a `<!-- Template Version: vNNNN -->` tag into a
downstream project's root `AGENTS.md` (`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`,
lines 29-57) and walks a versioned migration checklist on upgrade. This
version tracking is currently **pure skill-prose**, not Go-implemented:
`grep -rn "Template Version" agents-plugin-tool/internal --include='*.go'`
returns no matches. There is no code path today that reads a downstream
project's installed template version and compares it against the latest
version the installed plugin ships.

`wsflow` has its own package-local template version history starting at
`v0001` (`ai-docs/mental-model/workflow-skills.md` line 74) — it does not
replay the full ws migration backlog. Any staleness comparison must be
package-local (ws-installed project compares against ws's latest version;
wsflow-installed project compares against wsflow's latest version), never
cross-package.

Discussed and settled during `260703` sprint discussion (parallel to
`260703-chore-prefer-subagent-verify-discussion-inline-mirror` /
`260703-chore-sage-review-builtin-default-on` implementation): the goal is
letting downstream users notice when this repo's bootstrap conventions have
moved forward without requiring them to manually diff `AGENTS.md`.

## Decisions

- **Injection point: session-bootstrap time** (the `ws/ferrule` /
  `ws/workflow_manual` load path), not every `ws/project_tree` call.
  `project_tree` today only surfaces the `ai-docs/` doc/spec/ticket
  inventory and does not read root `AGENTS.md` at all — adding a staleness
  check there would be a new read bolted onto an unrelated tool, and would
  risk becoming a per-call banner-spam pattern. Session-bootstrap is the
  sparser, correct point, consistent with the existing
  agentId-continuity-tip precedent (inject at the point of action, not on
  every subsequent call) recorded in `260605-research-ws-native-subagent-pivot`.
- **Config surface: extend the existing layered config-item pattern**, do
  not add a new bespoke MCP tool. Add a new `wsconfig.Item*` entry (e.g.
  `ItemBootstrapAlarm`, values `on`/`off`, builtin default `on`) following
  the same shape as `ItemWorkflowPreferSubagent` /
  `ItemSageReview` (`agents-plugin-tool/internal/wsconfig/scope.go`,
  `server.go`'s `builtinConfigDefaults()`), and expose it through the
  existing `config.tuning` / `ws:lead-tune` catalog. A standalone
  `config.set_flag(name, bool)` tool was considered and rejected: it would
  create a second, unstructured config surface parallel to the disciplined
  named-item registry every other config point already uses.
- **Warning message content**: when stale, the warning text must point to
  how to permanently silence it via the new config item, through whatever
  setter surface that item gets wired into (`config.tuning`/`lead-tune`) —
  do not hardcode a specific tool-call string in this ticket ahead of the
  actual setter implementation.
- **Go-side version reading is new work, not a config-only change.** Since
  no Go code currently parses `AGENTS.md`'s `Template Version` tag, this
  ticket's implementation needs a new reader (parse the tag from the
  downstream project's root `AGENTS.md`) plus a source of "latest known
  version" per package (ws vs wsflow) to compare against — likely sourced
  from the same migration-checklist content `lead-bootstrap`'s prose
  already encodes, or a machine-readable extraction of it. The exact
  mechanism (parse the bootstrap rsrc playbook's checklist vs. a new
  small manifest entry) is an implementation-time survey question, not
  settled here.

## Out of Scope

- Changing `lead-bootstrap`'s own upgrade/migration procedure.
- Any per-`project_tree`-call variant of this warning.

## Phases

### Phase 1: Survey and implement session-bootstrap staleness warning

- Survey where session-bootstrap (`ws/ferrule` / `ws/workflow_manual`)
  is implemented in `agents-plugin-tool/internal/mcp/`, and how/whether
  "latest template version per package" can be sourced without hand
  duplication.
- Add the `wsconfig.ItemBootstrapAlarm` config item (builtin default
  `on`), wire it into `config.tuning`/`ws:lead-tune`.
- Add the `AGENTS.md` template-version reader and the staleness
  comparison, gated by the new config item, firing at session-bootstrap
  time only.
- Warning text includes the silencing instruction per Decisions.

## Spec Impact

New caller-visible session-bootstrap behavior (a warning surfaced to the
lead) and a new config item — needs spec addressing before `ready/`
promotion. Not addressed yet; left for the implementation-survey pass that
promotes this ticket. Contract-first spec: not yet decided — depends on
whether the warning shape needs to be stable/documented ahead of
implementation or can be closed out after.
