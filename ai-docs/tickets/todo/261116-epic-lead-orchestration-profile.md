---
title: "Lead orchestration profile: sub-lead building blocks behind a thin lead"
sage-review-design: required
related:
  261115-feat-ws-worker-profile-session-spawn: context - role/scope gates a sub-lead key must fit
  261116-research-ws-hosted-session-first-tier: context - its ws/agent.spawn direction is the expected path to harness-neutral spawn depth
---

# Lead orchestration profile: sub-lead building blocks behind a thin lead

## Scope

- A `workflow.profile` knob that swaps lead-side playbooks wholesale between
  `direct` (today's lead behavior) and `orchestrate` (user - lead - sub-lead -
  worker - explore).
- Procedure lives once, in sub-lead playbooks. Each split skill gets a thin
  per-profile entry: `orchestrate` spawns the sub-lead and relays; `direct`
  reads the same sub-lead playbook and performs it inline.
- A shared sub-lead report protocol and the lead's single generic loop over it.
- The workflow manual split into a lead core and a sub-lead part.
- Splitting `lead-run`, `lead-review`, and `lead-ship` into sub-lead blocks.
- Removing goal-run support (`goal/*` branches and their terminals).

## Non-Scope

- `lead-ticket` drafting and Open Decision Queue conversation stay lead-only in
  both profiles; `ticket-handler` keeps its current promote / epic-review /
  revise scope.
- `lead-discuss`, `lead-delegate`, `lead-tune`, `lead-use-mailbox`,
  `lead-check-blockers`, `lead-audit-doc`, `lead-revive`: no profile split.
- Hiding skills or MCP tools per session: skill and tool exposure stay static;
  only the content `playbook.read` returns changes.

## Child Tickets

- `261117-refactor-playbook-compression` - todo; the owner-led diet of the
  lead-side playbooks this epic splits, run before foundation Phase 2. (77)
- `261116-feat-orchestration-profile-foundation` - ready (Phase 1 done); profile-aware file selection, the `workflow.profile`
  knob with the harness whitelist and its set-time warning, the
  `sublead-protocol` include, the workflow manual core/sub-lead split, the
  sub-lead whiteboard status path, the per-host nested-spawn check.
- `261117-feat-role-scoped-workflow-context` - todo; role-scoped manual
  modules and subagent-visible notes and agenda, discussed before foundation
  Phase 2 (22).
- Planned: run - `ticket-runner` and `integrator`; `lead-run` direct and
  orchestrate entries; goal-run removal.
- Planned: review - `review-runner`, reusing `integrator`.
- Planned: ship - `ship-runner`.

## Cross-Child Decisions

- **Golden rule.** A boundary sits where user intervention is worth most;
  between those points, agents talk to agents. A block boundary is a point
  where a user approval naturally sits and the preceding context can be
  dropped (merge is both; ticket selection is neither, so it stays inside
  `ticket-runner`). (05), (09)
- **Single source.** Procedure text exists once, in the sub-lead playbook.
  Per-profile entries carry no procedure; the maintenance surface grows by one
  short entry per split skill, never by a second playbook set. (07)
  Rejected: a full lead playbook set per profile (doubles the surface); a
  profile-less switch to `orchestrate` (no fallback for hosts without nested
  spawn).
- **Report protocol.** Every sub-lead ends with one shared block: `status:
  ask-user | done | blocked`, `ask:` items in the workflow manual's classes,
  `on_answer:` for mid-run resumption, `next: <playbook> · needs:
  approval|none · inputs: <...>` for joints, and `result:` / `blocker:`. The
  lead's only rule set is one generic loop in the workflow core: translate
  `ask` and send back a per-ID digest to the same sub-lead; on `next`, ask
  first when `needs: approval`, then spawn; relay `result` / `blocker`. An
  orchestrate entry names only its starting sub-lead. `direct` reads the same
  block inline: `ask` goes straight to the user, `next` is read and performed.
  `ticket-handler`'s statuses fold into this protocol. (01)
  Rejected: joint behavior spelled out in lead entries; per-sub-lead
  protocols.
- **Veto points.** Sub-leads execute approval-gated actions (merge, publish,
  review stamp, branch-identity vouch, irreversible actions) only with the
  user's explicit words relayed as `approved:` in their task block; an
  executor without it returns `ask-user`. Config-sanctioned skips (for
  example `merge_confirm: skip`) keep working. (02)
  Rejected: keeping execution on the lead.
- **Whiteboard.** Sub-leads may write status (log lines, panels) to the lead's
  board; every ask goes through the lead, which stays the user's single voice.
  (03) Rejected: sub-lead asks on the board; lead-only board.
- **Goal runs.** Goal-run support is removed, not ported. (04)
- **Ticket drafting.** Ticket prose is the lead's, drafted with the user in
  both profiles: it is where the user's intent arrives at the highest
  resolution. (05) Rejected: a `ticket-handler` `write` operation.
- **Authoring.** Prose in this epic's playbooks is drafted by the lead with
  the user, not delegated to a worker. (09)
- **Profile knob.** `workflow.profile: direct | orchestrate`, an enum
  rather than a boolean so a later profile fits, harness-applicable like
  `workflow.whiteboard_enabled`. Default `direct` in both `ws` and `wsflow`;
  `orchestrate` is opt-in. (10), (12)
- **File selection.** `playbook.read` and `workflow_manual` select
  `<name>/<name>.<profile>.md` when present, else `<name>/<name>.md`.
  Rejected: Go text substitution (`applyKnobVariants`), which degrades
  silently when its anchor text drifts; the host-variant files
  `lead-workflow-manual/native-spawn-binding.{claude,codex}.md` are the
  precedent. (11)
- **`workflow.prefer_subagent`** keeps its meaning as a separate knob: ad-hoc
  delegation is orthogonal to where a skill's procedure lives. Rejected:
  folding it into the profile. (13)
- **Harness whitelist.** Harnesses verified to support nested spawn (lead ->
  sub-lead -> worker -> delegate -> explore) form a whitelist; the foundation
  child verifies the current hosts. Setting `orchestrate` for a harness off
  the whitelist is allowed, with a warning nudge at set time that the harness
  may not support nested spawn. The whitelist grows as harnesses are added.
  Rejected: refusing the setting; a separate research ticket before the epic.
  (16)
- **Depth today.** Claude Code gives a level-3 subagent no spawn tool
  (probed 2026-11-16) and ws caps `pi.spawn` at two levels, so no
  host probed so far completes the chain (Codex is probed in the foundation
  child).
  The chain shape and the children's names stay; harness-neutral depth
  control is expected from the `ws/agent.spawn` direction of
  `261116-research-ws-hosted-session-first-tier`. Rejected: `ticket-runner`
  absorbing the worker role to keep a three-level chain; leaf-only worker
  children. (68)
- **Sub-lead peer mail.** Once `ws/agent.*` mailbox routing lands
  (`261117-feat-ws-agent-tools-mailbox-routing`), a sub-lead may mail another
  sub-lead only at an address the lead handed it in its task block, and only
  for facts and technical coordination (for example `review-runner` sending a
  fix request to the sleeping `ticket-runner` that holds the context, or
  `integrator` asking a parallel runner about a merge interaction). Peer mail
  never carries `approved:` or a user ask; `approved:` is honored only from
  the sub-lead's parent, which the lineage-computed relation makes checkable;
  the sub-lead summarizes peer exchanges in its `result:`. The run and review
  children design the channel; foundation Phase 2's `sublead-protocol` states
  the parent-only `approved:` rule. (76)=b. Rejected: star-only routing
  through the lead; an open mesh between sub-leads.
- **Sub-lead whiteboard path.** The foundation child gives a sub-lead key a
  write path for status log lines and panels on its top-level lead's board,
  and none for asks; the board and its nudges are keyed to a top-level lead
  today (`whiteboard_nudges.go`, `r.Parent == ""`). (15)
- **Children.** Four, in order: foundation, run, review, ship; run, review,
  and ship declare `blocked-by:` the foundation child. Rejected: one large
  ticket. (14)
- **Names.** `ticket-runner`, `review-runner`, `ship-runner`: one `-runner`
  suffix marks a sub-lead that owns a whole flow and reports through
  `sublead-protocol`. The shared merge block is `integrator`: its job spans
  merge, conflict hand-off, `## Blocked` notes, and the epic-close check.
  Rejected: `-manager` names (do not say what they manage); `merger`. (17)
- **Placement.** This epic stays in `todo/` (epics never enter `ready/`),
  stem `261116-epic-lead-orchestration-profile`, named for the deliverable so
  later children fit. (18), (19)
- **Word budget.** Every lead-facing entry stays at or under roughly 400
  words; sub-lead playbook length is secondary.
- **Working order (2026-11-17).** Diet first, then structure:
  `261117-refactor-playbook-compression` reduces each playbook with the owner,
  one skill per deep discussion (owner states the core, lead proposes what
  must stay, iterate until the owner is satisfied; procedure in that
  ticket's `## Session Procedure`); then
  `261117-feat-role-scoped-workflow-context` settles the manual's
  modularization; then foundation Phase 2. (77), (78), (80)
- **Executor reports carry the judgment.** Guidance the lead would interpret
  moves into a self-describing executor report; the lead keeps one relay
  rule. Delegates end with the `delegate-report` block, whose shape follows
  (01); `sublead-protocol` extends that block rather than defining a second
  report. (82), (83)=a

## Completion Criteria

- Done: the four children are done; with `workflow.profile: orchestrate`,
  `lead-run`, `lead-review`, and `lead-ship` entries are each at or under
  ~400 words and drive their flows end to end through sub-leads; with
  `direct`, the same flows behave as before apart from goal-run removal.
- Dropped: nested spawn proves unusable on every supported host after the
  `ws/agent.spawn` work of `261116-research-ws-hosted-session-first-tier`
  lands, or the user abandons the profile split. (70)
- Deferred: a `bootstrapper` block for `lead-bootstrap`; a build block for
  `lead-coauthor-paged-document`; compressing the lead-kept playbooks outside
  `261117-refactor-playbook-compression`'s scope to the word budget. (77)
