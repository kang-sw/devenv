---
title: "Orchestration profile foundation: profile-aware playbooks, sub-lead protocol, sub-lead board status"
parent: 261116-epic-lead-orchestration-profile
---

# Orchestration profile foundation: profile-aware playbooks, sub-lead protocol, sub-lead board status

## Background

The epic `261116-epic-lead-orchestration-profile` moves lead procedure into
sub-lead playbooks behind a `workflow.profile` knob. This child lays the
ground the run, review, and ship children build on: the knob and
profile-aware file selection, the harness whitelist, a status-only board path
for sub-leads, the shared sub-lead report protocol, and the workflow manual
split. It also absorbs the stale `ticket_review_auto_ack` variant defect,
since both touch how playbook text varies by knob.

## Decisions

- Epic Cross-Child Decisions bind this ticket: profile knob (10), (12); file
  selection (11); harness whitelist with a set-time warning (16); sub-lead
  board status only (03), (15); report protocol (01); single source (07);
  prose authored by the lead with the user (09).
- `workflow.profile` builtin default `direct` sits in `builtinConfigDefaults`,
  shared by `ws` and `wsflow` (consequence of (10)).
- **Selection site.** Profile selection lives in `wsrsrc.Load`
  (`resolvePlaybookPath` and include resolution), so `playbook.read`,
  `playbook.render`, and `workflow_manual` all see it. Rejected: read-path
  post-processing like `applyKnobVariants`, which `playbook.render` never
  calls. (46)
- **Top-level only.** Selection applies only to top-level lead keys
  (`isTopLevelLead`); a parented key (sub-lead, worker) always gets base
  files, so a sub-lead's `workflow_manual` is the full one. Rejected:
  resolving each caller's own config. (47)
- **Lookup order.** `<name>.<profile>.md`, then `<name>.<harness>.md`, then
  `<name>.md`, for playbooks and includes alike; a file with both a profile
  and a harness overlay fails `wsrsrc` validation. Rejected: a combined
  `<name>.<profile>.<harness>.md` matrix (nothing needs both today). (48)
- **Whitelist.** A Go set beside `harnessBuckets`; `config.tune` writing
  `orchestrate` to a bucket outside it, or to `all` while some harness is
  outside it, appends one `warning:` line naming the harnesses, shaped like
  the `shadowed:` line. Rejected: a structured `warnings` field; a
  `runtime.json` capability. (49)
- **Probe.** Phase 1 probes nested spawn (lead -> sub-lead -> worker ->
  delegate -> explore) on Claude, Codex, and Pi; only verified harnesses seed
  the whitelist; host versions and probe date are a comment at the
  whitelist. Rejected: a new manual. (50)
- **Sub-lead board path.** A parented lead key's `whiteboard.update`
  resolves to its top-level lead's board by walking `Parent` and admits only
  `wb:log` and `wb:panel` inside a `wb:topic` naming an existing open topic;
  every other command is refused. `whiteboard.query` is admitted read-only on
  that board. Rejected: a separate sub-lead board; sub-lead topic creation.
  (51)
- **Attribution.** A sub-lead's log line and panel recap carry a
  server-written attribution prefix in their text. Rejected: a new author
  field in the board store. (52)
- **Auto-ack fix.** Re-anchor `ticketReviewAutoAckVariants` on the current
  text, apply the `ticket-handler` variants in the render path, and add the
  pin test (each on-text occurs exactly once in its playbook). Approved
  knob-off text is listed in Phase 1. Rejected: retiring the
  `ticket_review_auto_ack` knob. (53)
- **Phases.** Phase 1 (Go) runs through a `lead-run` worker; Phase 2 (prose)
  is written by the lead with the user in session, not dispatched. (54)
- **Handler report.** `ticket-handler`'s report folds into
  `sublead-protocol` in Phase 2, not in the run child. (55)
- **Compatibility.** T0 for the `workflow.profile` key (`Config.Overrides` is
  a string map round-tripped by `updateConfigFile`; the repo file keeps
  members verbatim); T0 for new rsrc files (an old binary opens only files it
  resolves, its overlay probe matches harness names only, manifest
  `schema_version` stays 1); T1 for whiteboard admission of parented keys (an
  old daemon refuses them at the top-level-lead gate; status lines are lost,
  no data changes). (56)
- **Placement.** `parent:` the epic, created in `todo/`. (57)

## Constraints

- Manuals: `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`; Phase 2 also
  `ai-docs/manuals/skill-authoring.md`.
- New rsrc files require regenerating the rsrc manifest, the wsflow rsrc
  mirror, and the pi rsrc mirror.
- `workflow.profile` and its buckets live inside `overrides` as strings, never
  as a top-level config member or an object value (an old binary drops an
  unknown top-level member on rewrite and fails on an object inside
  `overrides`).

## Phases

### Phase 1: Profile selection, knob, whitelist, sub-lead board path, auto-ack fix

- `workflow.profile: direct | orchestrate`, harness-applicable with
  `claude|codex|pi|all` buckets like `workflow.whiteboard_enabled`; registered
  in the scope registry, the config key registry, the `config.tune`
  description, and builtin defaults.
- Profile-aware selection in `wsrsrc.Load` per (46)-(48); every `Load`
  caller, including declared-variable collection, receives the same profile.
- Whitelist and `config.tune` warning per (49); nested-spawn probe per (50).
- Sub-lead board path per (51), (52).
- Auto-ack per (53). Approved knob-off text (match each on-text exactly
  against the text present when the phase runs):
  - `lead-ticket`: `- Only user-confirmed decisions, plus items the handler
    auto-confirms and reports.` -> `- Only user-confirmed decisions.`
  - `lead-ticket`: `` `(01), (02) confirmed · (04) deferred`, plus each
    handler auto-confirmed item: `(05) auto-confirmed - <one-line decision>`.
    `` -> `` `(01), (02) confirmed · (04) deferred`. ``
  - `lead-ticket`: `- An unsettled queue item, apart from handler
    auto-confirmed ones.` -> `- An unsettled queue item.`
  - `ticket-handler`: `Persist only what they confirm, or what you
    auto-confirm below.` -> `Persist only what they confirm.`
  - `ticket-handler`: the whole `- Auto-confirm, with no lead round: ...`
    bullet -> `- No auto-confirm: every announced or technical item,
    including one from a populator `decision_gaps:` item or a reviewer issue,
    gets a lead round.`
  - The report's `auto_confirmed:` line and `lead-ticket`'s "auto-confirmed
    items" relay stay; with the knob off they are simply empty.
- Verification: unit tests for lookup order, the top-level-only rule, the
  overlay-conflict validation, knob registration and the warning line, the
  board admission and every refused command, the attribution prefix, and the
  auto-ack pin test; `go test ./internal/wsrsrc/... ./internal/mcp/...`; the
  manifest and mirror regeneration tests.

### Phase 2: Sub-lead protocol and workflow manual split

Written by the lead with the user in session (54).

- `sublead-protocol` include: the report block of epic decision (01) and the
  `approved:` executor guard of (02).
- Workflow manual split into a lead core and a sub-lead part as includes:
  `lead-workflow-manual.md` includes both (the `direct` manual, unchanged in
  content); `lead-workflow-manual.orchestrate.md` includes the core plus the
  generic loop over the protocol; sub-leads read the base manual.
- `ticket-handler` report folded into the protocol (55); `lead-ticket`'s
  handler loop reads the folded report.
- `whiteboard` playbook: sub-leads may write status lines and panels.
- Verification: `playbook.read` of `lead-workflow-manual` under each profile
  for a top-level key, and under `orchestrate` for a parented key (base
  manual); manifest and mirror regeneration tests.
