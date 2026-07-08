---
title: "Warn on stale downstream bootstrap template version at session-bootstrap time"
related:
  260605-research-ws-native-subagent-pivot: precedent for tip-injection-point discipline (agentId-continuity tip is action-time-only, not per-call)
sage-review: completed
completed: 2026-07-07
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
  duplication. The numbered migration checklist in
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` (max `vNNNN`)
  is a viable source for "latest version" without a new manifest.
- Add the `wsconfig.ItemBootstrapAlarm` config item (builtin default
  `on`), wire it into `config.tuning`/`ws:lead-tune`.
- Add the `AGENTS.md` template-version reader and the staleness
  comparison, gated by the new config item, firing at session-bootstrap
  time only.
- Decide and document the no-tag case: default to silent (an untagged
  project never opted into ws bootstrap), not maximally-stale.
- Identify the warning-delivery channel in the `ferrule`/`workflow_manual`
  return payload (e.g. appended to the manual body vs. a distinct
  response field) from the existing handler shape in
  `agents-plugin-tool/internal/mcp/workflow_manual.go`.
- Warning text includes the silencing instruction per Decisions.
- Verification: add a test confirming the warning fires when the
  installed tag version is behind latest, is suppressed when
  `ItemBootstrapAlarm` is off, is silent when no tag is present, and that
  `config.tuning`/`ws:lead-tune` lists and can set the new item.

## Spec Impact

New caller-visible session-bootstrap behavior (a warning surfaced to the
lead) and a new config item — needs spec addressing before `ready/`
promotion. Not addressed yet; left for the implementation-survey pass that
promotes this ticket. Contract-first spec: not yet decided — depends on
whether the warning shape needs to be stable/documented ahead of
implementation or can be closed out after.

### Result (e4adbd2b) - 2026-07-07

Merged to `main` (`e4adbd2b`) from `implement/bootstrap-staleness-alarm`
(commits `3863a86e` feature, `4f43cabb` review fixes).

Implemented per the survey plan at
`ai-docs/.plans/2026-07/07-1011-bootstrap-staleness-alarm.md`: `ferrule` and
`workflow_manual` (FRESH-with-root and CONTINUE) surface a one-line
staleness banner when a downstream project's root `AGENTS.md` Template
Version tag is behind the running package's own shipped `lead-bootstrap`
template. Package-local comparison reuses `wsrsrc.ResolveSkillsRoot()`
instead of a hand-maintained cross-package manifest. New global-only
`wsconfig.ItemBootstrapAlarm` (builtin default `on`) is exposed via
`config.bootstrap_alarm` (set/reset) and `config.tuning`/`ws:lead-tune`.
Silent by design when the alarm is off, the downstream root has no tag, or
the shipped template's own tag is unreadable (including a
`ResolveSkillsRoot()` failure at `ferrule` time, fixed during review to
match `workflow_manual.go`'s fail-safe-silent sibling pattern rather than
hard-failing the whole call).

Spec updated in the same commit (`ai-docs/spec/mcp-tools.md`,
`#260703-bootstrap-staleness-warning` plus `config.bootstrap_alarm`
coverage); `spec_index.verify()` ok. `runtime.json` contract entries added
in both `agents-plugin` and `agents-plugin-wsflow` (golden-list test
requirement, not a version-bump edit). Plugin version bumped 0.33.0 ->
0.33.1 per dev-merge convention (`89f60ac4`).

Review: correctness and test partitions each found 1-2 fixable issues
(fail-safe-silence inconsistency in `handleLeadLogin`; a non-discriminating
`config.tuning` test assertion; missing boundary-case test for
installed-at-or-above-latest) — all fixed and re-reviewed clean. Fit
partition was clean on the first pass.

All 4 ticket-specified verification assertions have dedicated tests;
`go test ./...`, `go vet ./...`, `go build ./...` pass on `main` post-merge.
