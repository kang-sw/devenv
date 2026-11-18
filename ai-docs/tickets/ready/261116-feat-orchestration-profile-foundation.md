---
title: "Orchestration profile foundation: profile-aware playbooks, sub-lead protocol, sub-lead board status"
parent: 261116-epic-lead-orchestration-profile
related:
  261116-research-ws-hosted-session-first-tier: context - ws/agent.spawn direction for harness-neutral spawn depth (68)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e7f241db30aba717
sage-review-completeness-reviewed: e7f241db30aba717
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
  delegate -> explore) on the hosts not yet probed; only verified harnesses seed
  the whitelist; host versions and probe date are a comment at the
  whitelist. Rejected: a new manual. (50)
- **Probe criterion.** A harness is verified when the full lead -> sub-lead
  -> worker -> delegate -> explore chain completes and the deepest result
  returns to the lead. The Phase 1 Result lists every host probed and every
  host left unprobed (an unprobed host stays off the whitelist). Pi is off
  the whitelist by ws's own cap, not a host limit: `pi.spawn` allows at most
  two levels below the lead (`pi_forward.go`); the whitelist comment cites
  that cap. (63) auto-confirmed (review)
- **Claude result and timing.** Probed 2026-11-16 in Claude Code 2.1.325: a
  level-1 and a level-2 subagent below the lead have the Agent tool, a
  level-3 subagent has none (`general-purpose` and `wsflow:effort-low`
  alike), so a worker's delegate cannot spawn explore. The chain
  shape stays as the epic designs it and Phase 1 is implemented as written;
  harness-neutral depth control is expected from the `ws/agent.spawn`
  direction of `261116-research-ws-hosted-session-first-tier`, after which
  the whitelist grows. Rejected: a three-level run chain with
  `ticket-runner` absorbing the worker role; a worker whose children are
  leaves with no explore. (68)
- **Whitelist at ship.** Claude stays off the whitelist on the 2026-11-16
  probe; the Phase 1 probe covers Codex only. The whitelist may ship empty,
  and then every `orchestrate` setting warns. (69)
- **Attribution and panels.** The implementer picks the prefix form and a
  test pins it; the server refuses a sub-lead `wb:panel` that carries an
  `id` or whose `name` resolves to an existing panel in the topic (`name`
  alone selects a panel to replace or patch, `wswhiteboard/entries.go:122-161`),
  checked against the topic as earlier commands in the same call leave it,
  so a sub-lead always mints a new panel and never rewrites one it did not
  author. (64) auto-confirmed
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
  `sublead-protocol` names it, and a refusal to a sub-lead key names it. Rejected: a section inside `sublead-protocol` (every sub-lead would
  carry it); the full `whiteboard` playbook (its lead-only rules would reach
  sub-leads). (59)
- **Board knob off.** A sub-lead write never calls
  `enableWhiteboardKnobForSession` and never returns the lead directives (the
  `full:` URL, the applied-info line); its success output is one
  acknowledgement line. With the top-level lead's whiteboard knob resolving
  off, the write is still accepted into the board store, and the status shows
  when the user opens the board. Rejected: refusing the write while the knob
  is off; auto-enabling the board as a lead write does. (61)
- **Board writers.** Only sub-lead keys write status. A new playbook role
  `role: sublead` mints a parented lead key (`childRoleForPlaybookRole`)
  carrying a sub-lead marker, the minting playbook name, stored in its own
  file in the key directory, outside `recordParts`
  (`session_layout.go:78`): an old binary's `writeParts` rewrites only the
  part files (`session_layout.go:312-331`), so a whole-file `key.json`
  rewrite by an old daemon cannot drop the marker. Only marked keys are
  admitted, resolving to the top-level lead's board by walking `Parent`:
  `wb:log` and `wb:panel` inside an existing open `wb:topic` whose group
  carries no `label` or `tickets` (those create or relabel a topic and
  rewrite its ticket links), and `whiteboard.query` read-only. A write with no such open topic is refused
  with the `whiteboard-status` pointer (72). The deferred board refresh and
  the dashboard nudge after a sub-lead write target the top-level lead's key
  the walk resolved, not the caller's (`server.go:813-827`). Workers and the agent-card
  children of `261115-feat-ws-whiteboard-agent-card` keep the ordinary
  denial. Rejected: admitting every parented lead key (it admits every
  `role: worker` render and breaks that ready ticket's unchanged-denial
  contract). (51)
- **Role rollout.** This ticket adds the `role: sublead` mechanism with test
  fixtures only; no shipped playbook changes role here. The run, review, and
  ship children set it on their runners; `ticket-handler` stays
  `role: worker`, since it posts nothing to the board. Rejected: switching
  `ticket-handler` now. (71)
- **Refusal pointer.** The `whiteboard-status` pointer appears only in
  refusals to a marked sub-lead key; an unmarked parented key (worker, card
  child) keeps today's denial verbatim. (72)
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
- **Ticket-handler gap handoff.** Per user (09)=c in the session of ticket
  `261116-bug-ticket-handler-split-gaps`, Phase 2 also owns that ticket's G3:
  `ticket-handler`'s handling of `tickets.sage_gate` `ask`,
  `check_review_required`, and `stop_blocked`, and the fold of the `decisions`
  status (including the ticket-handler `## Lead replies` section that parses
  lead answers and every lead-run reference to the `decisions` status), all
  through the `sublead-protocol` report (55). Its input is user split-gaps
  (07)=b: a handler ask that the lead, not the user, must answer is
  self-describing (what to check, how to reply), the lead keeps one generic
  rule for it, and the procedure lives in the handler playbook. A lead-addressed
  ask (split-gaps (07)=b) has no slot in epic (01)'s `ask-user | done |
  blocked` set; Phase 2 settles it with the user, e.g. an addressee field on
  `ask:` items within the generic loop or folding the lead's check into
  `done`/`next`; adding a fourth status amends epic (01) explicitly. The same
  session's split-gaps (08)=a replaces (53)'s sentence-substitution and
  verbatim pin-test implementation with override markers in `ticket-handler.md`
  (and, via split-gaps' G2 whiteboard marker, at the lead relay point),
  keeping the knob; (53)'s knob retention stands. Phase 2 edits to
  `ticket-handler.md` and `lead-ticket.md` keep those marker blocks. (73)
- **Compatibility.** T0 for the `workflow.profile` key (`Config.Overrides` is
  a string map round-tripped by `updateConfigFile`; the repo file keeps
  members verbatim); T0 for new rsrc files (an old binary opens only files it
  resolves, its overlay probe matches harness names only, manifest
  `schema_version` stays 1); T1 for whiteboard admission of sub-lead keys (an
  old daemon never opens the marker file and refuses the key at the
  top-level-lead gate; it never rewrites or deletes the file either, since
  `writeParts` touches only `recordParts` files, so the new daemon still
  admits the key afterwards; status lines are lost, no data changes). The
  implementer confirms no old path deletes an unknown file in a key directory
  short of removing the whole directory; if one does, the tier is no longer
  T1 and the phase stops and reports, since T3 needs the owner's approval.
  (56), (51)
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
| complexity.side_effect_risk | high | wsrsrc.Load feeds playbook.read, playbook.render, workflow_manual and project_tree, and the whiteboard gate admits marked sub-lead keys |
| risk.correctness | moderate | lookup order, top-level-only selection and the Parent walk are new logic on shared load paths |
| risk.fit | moderate | selection site in wsrsrc.Load departs from the applyKnobVariants precedent, by recorded decision (46) |
| risk.test | moderate | many new unit cases across wsrsrc and mcp; nested-spawn probe is manual per host |
| risk.security_or_contract | high | session gate admits marked sub-lead keys on the top-level lead's board (T1 declared in (56)); config key and rsrc file additions are cross-version surfaces |

## Phases

### Phase 1: Profile selection, knob, whitelist, sub-lead board path, auto-ack fix

- `workflow.profile: direct | orchestrate`, harness-applicable with
  `claude|codex|pi|all` buckets like `workflow.whiteboard_enabled`; registered
  in the scope registry, the config key registry, the `config.tune`
  description, and builtin defaults.
- Profile-aware selection in `wsrsrc.Load` per (46)-(48); every `Load`
  caller, including declared-variable collection, receives the same profile.
- Whitelist and `config.tune` warning per (49); nested-spawn probe per (50),
  (63), (69).
- `role: sublead` and the session-record marker per (51); sub-lead board
  path per (51), (52), (61); a refused command's message for a sub-lead key
  names the `whiteboard-status` playbook (59), which Phase 2 writes.
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
  sub-lead board admission and every refused command (including a write
  with no open topic, a `wb:topic` group with `label` or `tickets`, and a
  `wb:panel` carrying an `id` or a `name` that matches an existing panel,
  including one minted earlier in the same call), a
  `role: worker`
  key still refused with today's message, the marker surviving a
  `writeParts` rewrite of `key.json`, the board refresh and dashboard nudge
  targeting the top-level lead's key, a sub-lead write with the lead's board
  knob off on a seeded open topic (accepted, knob unchanged, one
  acknowledgement line), the attribution prefix, and the
  auto-ack pin test; `go test ./internal/wsrsrc/... ./internal/mcp/...`; the
  manifest and mirror regeneration tests. At phase end the Result lists
  every host probed and left unprobed with versions and dates (Claude from
  (68), Codex from this phase's probe, Pi by its cap), and the whitelist
  comment carries the same.

### Result (2da06e481) - 2026-11-16

Landed (1bf9eba09..2da06e481):
- `wsrsrc.Load(root, name, harness, profile, vars)` changes:
  - Lookup order is `<stem>.<profile>.md`, then `<stem>.<harness>.md`, then
    `<stem>.md`, for playbooks and playbook-local includes alike.
  - The flat `<root>/<name>.md` fallback has no variants.
  - `wsrsrc.ProfileNames` (`direct`, `orchestrate`) is the shared list.
  - `Validate` refuses a stem that carries both a profile variant and a
    harness overlay.
- `workflow.profile` knob:
  - Harness-bucketed (claude, codex, pi, all), with builtin default `direct`,
    global default scope, and lead authority.
  - Registered in the scope registry, the config key registry, the tuning
    catalog, and the `config.tune` description.
  - Writing `orchestrate` for a harness outside
    `orchestrateHarnessWhitelist`, or for `*` while any harness is outside
    it, appends one `warning:` line. The whitelist ships empty (69).
- Profile selection applies to top-level lead keys only, through
  `playbook.read`, `playbook.render` (declared-variable collection included),
  and `workflow_manual`.
  - The FRESH sentinel resolves the profile through the canonical-root
    resolver.
  - Parented, unknown, and keyless keys load base files.
- `role: sublead` maps to a lead-scoped child key. The render writes a
  `sublead.json` marker `{"playbook": "<name>"}` in the key directory,
  outside `recordParts`.
- Sub-lead board path:
  - At the whiteboard gate, a marked lead key walks `Parent` (at most 16
    hops) to its top-level lead, and its call is rewritten onto that key. The
    write, the board refresh, and the dashboard nudge all target the lead.
  - `whiteboard.update` applies in status-only mode (`Env.StatusAuthor`):
    `wb:log` and new `wb:panel` inside an existing open topic whose group
    carries no `label` or `tickets`.
  - A panel `id`, or a `name` that already exists, is refused. That includes
    a name minted earlier in the same call.
  - Success is one acknowledgement line. A sub-lead write never enables the
    lead's knob and never returns `full:`, `embed:`, or `info:`.
  - Refusals name the `whiteboard-status` playbook.
  - `whiteboard.query` reads the lead's board.
  - Unmarked parented keys keep today's denial verbatim.
- Auto-ack (53): `ticketReviewAutoAckVariants` is keyed by playbook
  (`lead-ticket`, `ticket-handler`) and carries the approved knob-off text.
  `playbook.render` applies it as `playbook.read` does.
- Two pre-existing track failures fixed: `spawnIdiomSites` gains
  `ticket-handler`, and the knob-on `lead-ticket` queue step is re-anchored
  on the rewritten queue `Section —` list. Both came from 6d1e7e8c6. No rsrc
  prose changed.

Decisions:
- Attribution prefix is `[<playbook>] ` on log lines and panel recaps (64).
- A sub-lead write renders no embed and stamps no user inputs. The lead's
  own next entry answers the drained inbox.
- An unrecognized stored `workflow.profile` value loads base files instead of
  failing the call.
- T1 confirmed (56): `writeParts` removes only part files. Only prune's and a
  failed claim's whole-directory `RemoveAll` delete a key directory.

Host probes (whitelist comment carries the same):
- Claude Code 2.1.325, probed 2026-11-16 (68): a level-3 subagent has no
  Agent tool. Off.
- Codex CLI 0.160.0, probed 2026-11-16:
  - At defaults, the third level below the lead fails with "collab spawn
    failed: agent thread limit reached".
  - With `-c agents.max_threads=16 -c agents.max_depth=8` the full chain
    completes and the deepest result returns to the lead.
  - `agents.max_depth=8` alone still fails.
  - Off, until the chain completes at host defaults.
- Pi: off by ws's own cap, not probed. `wsagent.MaxSpawnDepth = 2`
  (`internal/wsagent/children.go`), which the `pi.spawn` description
  restates (63).
- No other host probed.

Verification:
- `go build ./...` and `go vet` are clean.
- `go test` passes for `./internal/wsrsrc/...` (manifest, wsflow, pi, and
  skills mirror tests included), `./internal/wswhiteboard/...`,
  `./internal/wsconfig/...`, and `./internal/wsdoc/...`.
- `go test ./internal/mcp/`: passes in full (610s, `-timeout 40m`).
- One lite code review pass found no Critical or Important issues. It had one
  minor: a failed marker write after the mint leaves an orphan key that is
  never handed out and is pruned later.

### Phase 2: Sub-lead protocol and workflow manual split

Written by the lead with the user in session (54). Starts after
`261117-refactor-playbook-compression` has compressed the text it splits and
folds (77).

- `sublead-protocol` include: the report block of epic decision (01) -
  `status: ask-user | done | blocked`, `ask:` items in the workflow manual's
  classes, `on_answer:` for mid-run resumption, `next: <playbook> · needs:
  approval|none · inputs: <...>` for joints, `result:` / `blocker:` - and the
  executor guard of (02): an approval-gated action runs only with the user's
  words relayed as `approved:` in the task block, else the sub-lead returns
  `ask-user`. (62) auto-confirmed (review) The guard honors `approved:` only
  from the sub-lead's parent, never from peer mail (epic (76)=b). The
  include extends the shipped `delegate-report` block (status, `result`,
  `blocker`, `ask`, `tell_user`, `resume`, `omitted`) with `next:` and
  `on_answer:` rather than defining a second report shape (epic (83)=a).
- Workflow manual split into a lead core and a sub-lead part as includes:
  `lead-workflow-manual.md` includes both (the `direct` manual, unchanged in
  content); `lead-workflow-manual.orchestrate.md` includes the core plus the
  generic loop over the protocol; sub-leads read the base manual.
- `ticket-handler` report folded into the protocol (55); `lead-ticket`'s
  handler loop reads the folded report. Also covers the handler gap handoff
  (73): sage_gate `ask` / `check_review_required` / `stop_blocked` handling and
  the `decisions` status fold.
- `whiteboard-status` playbook (59): the status-only subset for sub-leads;
  `sublead-protocol` points a sub-lead that posts status at it. It tells a
  sub-lead that a "no open topic" refusal means keeping the status in its
  report. The
  `whiteboard` playbook's "workers and subagents do not use the board" line
  is updated to point sub-leads there.
- Verification: `playbook.read` of `lead-workflow-manual` for a top-level
  key under `direct` is byte-identical to the pre-split output (captured
  before the split); under `orchestrate` it holds the core and the generic
  loop and none of the sub-lead part; under `orchestrate` for a parented key
  it is the base manual; `playbook.read` resolves `whiteboard-status`, the
  name the Phase 1 refusal text carries, and the `whiteboard` playbook's
  sub-lead line names it; manifest and mirror regeneration tests. (65)
  auto-confirmed (review)

## Sage Review Round 1 (2026-11-16)

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
