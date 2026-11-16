---
title: "Orchestration profile foundation: profile-aware playbooks, sub-lead protocol, sub-lead board status"
parent: 261116-epic-lead-orchestration-profile
sage-review-design: blocked
sage-review-completeness: blocked
---

# Orchestration profile foundation: profile-aware playbooks, sub-lead protocol, sub-lead board status

## Open Decision Queue

Last ID: (67)

(51) [open] [policy] Which keys may write status to the top-level lead's board (reopened by design review: as confirmed, it admits every parented lead key).
- Context: `role: worker` renders mint parented lead keys (`childRoleForPlaybookRole`), so "any parented lead key" admits every `lead-run` worker and `ticket-handler`, not only sub-leads; and ready ticket `261115-feat-ws-whiteboard-agent-card` pins "every unrelated parented caller's existing denial. No global child permission" with a verification for unchanged parented denial. No key-level marker separates a sub-lead from a worker today.
- Alternative a: only sub-lead keys. A new playbook role `role: sublead` mints a parented lead key whose session record carries a sub-lead marker (the minting playbook name); only marked keys are admitted, resolving to the top-level lead's board by walking `Parent`, `wb:log` and `wb:panel` inside an existing open `wb:topic` only, `whiteboard.query` read-only. Workers and card children keep the ordinary denial. Compatibility: an old daemon ignores the marker field (session records are read with `json.Unmarshal`), so it denies the key as today - status lines lost, no data change (T1).
- Alternative b: any parented lead key, as confirmed; supersede the card ticket's ordinary-denial constraint by name and coordinate its tests.
> a: matches the epic's "sub-lead key" framing (03), (15) and keeps the ready card ticket's contract intact.

(61) [open] [policy] A sub-lead status write when the top-level lead's whiteboard knob resolves off (builtin default off).
- Context: a successful `whiteboard.update` today calls `enableWhiteboardKnobForSession` and returns lead-only directives (the `full:` URL, the applied-info line); reusing it for a sub-lead write would switch the user's board on as a side effect.
- Alternative a: accept the write into the board store without touching any knob; the sub-lead's success output is a single acknowledgement line with no lead directives; the status shows whenever the user opens the board.
- Alternative b: refuse the write with a short message; the sub-lead keeps status in its report only.
- Alternative c: auto-enable the board, as the handler does for a lead.
> a: a sub-lead never branches on the lead's knob, and the knob stays the user's switch.

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
- **Probe criterion.** A harness is verified when the full lead -> sub-lead
  -> worker -> delegate -> explore chain completes and the deepest result
  returns to the lead. The Phase 1 Result lists every host probed and every
  host left unprobed (an unprobed host stays off the whitelist). Pi is off
  the whitelist by ws's own cap, not a host limit: `pi.spawn` allows at most
  two levels below the lead (`pi_forward.go`); the whitelist comment cites
  that cap. (63) auto-confirmed (review)
- **Attribution and panels.** The implementer picks the prefix form and a
  test pins it; a sub-lead's `wb:panel` carries no `id`, so it always mints a
  new panel and never rewrites one it did not author. (64) auto-confirmed
  (review)
- **Keyless and FRESH loads.** A keyless `Load` (for example
  `wsdoc/project_tree.go`) uses base files; the `workflow_manual` FRESH
  sentinel resolves the profile through the canonical-root resolver it
  already builds for the whiteboard knob. (66) auto-confirmed (review)
- **Flat playbooks.** The flat `<root>/<name>.md` fallback has no profile
  variant; validation treats it the same way. (67) auto-confirmed (review)
- **Sub-lead board syntax.** A separate short `whiteboard-status` print
  playbook teaches the status-only subset (`wb:log`, `wb:panel` inside an
  existing `wb:topic`), read on demand by a sub-lead that posts status;
  `sublead-protocol` names it, and the refusal a parented key receives names
  it. Rejected: a section inside `sublead-protocol` (every sub-lead would
  carry it); the full `whiteboard` playbook (its lead-only rules would reach
  sub-leads). (59)
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261116-epic-lead-orchestration-profile (2026-11-16, Cross-Child Decisions): "File selection. `playbook.read` and `workflow_manual` select `<name>/<name>.<profile>.md` when present, else `<name>/<name>.md`. Rejected: Go text substitution (`applyKnobVariants`)" — bearing: supports
- 261116-epic-lead-orchestration-profile (2026-11-16, Cross-Child Decisions): "Sub-lead whiteboard path. The foundation child gives a sub-lead key a write path for status log lines and panels on its top-level lead's board, and none for asks" — bearing: supports
- 261116-epic-lead-orchestration-profile (2026-11-16, Cross-Child Decisions): "Harness whitelist. ... Setting `orchestrate` for a harness off the whitelist is allowed, with a warning nudge at set time" — bearing: supports
- 1f5f4ae3 (2026-11-16, commit): "User chose (59)=a: an on-demand playbook over a section inside sublead-protocol (every sub-lead would carry it) or the full playbook." — bearing: supports
- 42bbcd46 (2026-06-12, commit): "wsrsrc.Load resolves <root>/<name>/<name>.md (subdir) first, then falls back to a flat <root>/<name>.md; a flat file can be both an include target and a loadable var-free playbook." — bearing: constrains
- bd9f1cae (2026-06-09, commit): "Includes are flat (no nesting) by design; cycle detection is not needed." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Knob-on text is applied code-side in printPlaybook (the prefer_subagent precedent) with replace-or-append fallback, so rsrc mirrors stay byte-identical and a prompt override that drops the anchor text loses nothing." — bearing: constrains
- 261113-feat-ws-pi-mini-lead-profile (2026-11-13, Decisions): "`pi-mini-lead` = `pi-worker` tools plus `tickets.query/acquire/release/ close/move/verify/checklist`, ... No note, whiteboard, or tracker writes." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsrsrc/loader.go, internal/mcp/server.go, config_registry.go, whiteboard_tools.go, ticket_review_text.go, internal/wsconfig/scope.go; Phase 2 rsrc playbooks and their wsflow and pi mirrors |
| scope.surface | cross-module | wsrsrc.Load callers in internal/mcp/playbook_tools.go and internal/wsdoc/project_tree.go; config.tune key; whiteboard gate at server.go#L774 |
| scope.new_public_symbol | yes | workflow.profile config key (wsconfig item constant) and the sublead-protocol and whiteboard-status playbooks |
| scope.new_type_contract | yes | wsrsrc.Load gains a profile selection input; the sublead-protocol report block is a new cross-skill contract |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/whiteboard_selection_test.go, internal/mcp/ticket_review_text_test.go, whiteboard_tools_test.go; the auto-ack pin test named in the ticket is absent (search for "exactly once" in ticket_review*_test.go returned nothing) |
| complexity.reuse_points | confirmed | harnessBuckets, harnessBucketKey, resolveIncludePath overlay probe, replaceOrAppend, the shadowed: line shape in config_structured.go and server.go |
| complexity.side_effect_risk | high | wsrsrc.Load feeds playbook.read, playbook.render, workflow_manual and project_tree, and the whiteboard gate relaxes for parented lead keys |
| risk.correctness | moderate | lookup order, top-level-only selection and the Parent walk are new logic on shared load paths |
| risk.fit | moderate | selection site in wsrsrc.Load departs from the applyKnobVariants precedent, by recorded decision (46) |
| risk.test | moderate | many new unit cases across wsrsrc and mcp; nested-spawn probe is manual per host |
| risk.security_or_contract | high | session gate admits parented lead keys on the top-level lead's board (T1 declared in (56)); config key and rsrc file additions are cross-version surfaces |

## Phases

### Phase 1: Profile selection, knob, whitelist, sub-lead board path, auto-ack fix

- `workflow.profile: direct | orchestrate`, harness-applicable with
  `claude|codex|pi|all` buckets like `workflow.whiteboard_enabled`; registered
  in the scope registry, the config key registry, the `config.tune`
  description, and builtin defaults.
- Profile-aware selection in `wsrsrc.Load` per (46)-(48); every `Load`
  caller, including declared-variable collection, receives the same profile.
- Whitelist and `config.tune` warning per (49); nested-spawn probe per (50).
- Sub-lead board path per (51), (52); a refused command's message for a
  parented key names the `whiteboard-status` playbook (59), which Phase 2
  writes.
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

- `sublead-protocol` include: the report block of epic decision (01) -
  `status: ask-user | done | blocked`, `ask:` items in the workflow manual's
  classes, `on_answer:` for mid-run resumption, `next: <playbook> · needs:
  approval|none · inputs: <...>` for joints, `result:` / `blocker:` - and the
  executor guard of (02): an approval-gated action runs only with the user's
  words relayed as `approved:` in the task block, else the sub-lead returns
  `ask-user`. (62) auto-confirmed (review)
- Workflow manual split into a lead core and a sub-lead part as includes:
  `lead-workflow-manual.md` includes both (the `direct` manual, unchanged in
  content); `lead-workflow-manual.orchestrate.md` includes the core plus the
  generic loop over the protocol; sub-leads read the base manual.
- `ticket-handler` report folded into the protocol (55); `lead-ticket`'s
  handler loop reads the folded report.
- `whiteboard-status` playbook (59): the status-only subset for sub-leads;
  `sublead-protocol` points a sub-lead that posts status at it. The
  `whiteboard` playbook's "workers and subagents do not use the board" line
  is updated to point sub-leads there.
- Verification: `playbook.read` of `lead-workflow-manual` for a top-level
  key under `direct` is byte-identical to the pre-split output (captured
  before the split); under `orchestrate` it holds the core and the generic
  loop and none of the sub-lead part; under `orchestrate` for a parented key
  it is the base manual; manifest and mirror regeneration tests. (65)
  auto-confirmed (review)

## Blocked (2026-11-16)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Parented-key board admission contradicts ready ticket 261115-feat-ws-whiteboard-agent-card | critical | missing |
| 2 | Sub-lead status write versus the lead's whiteboard knob (default off) is undefined | important | missing |
| 3 | Profile input for keyless and FRESH Load callers is unstated | minor | autonomous |
| 4 | Flat-playbook fallback not covered by the lookup order | minor | autonomous |
| 5 | Nested-spawn probe execution across hosts; Pi is capped by ws itself | minor | autonomous |
| 6 | Attribution prefix content and panel-id override unspecified | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Phase 2 content depends on epic decisions not restated | minor |
| 2 | Nested-spawn probe has no explicit pass criterion or record location | minor |
| 3 | Attribution prefix format unspecified | minor |
| 4 | Phase 2 verification has no expected outcome for the direct manual | minor |
