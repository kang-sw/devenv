---
title: "역할별 workflow manual 모듈과 subagent에게 보이는 note·agenda"
parent: 261116-epic-lead-orchestration-profile
related:
  261116-feat-orchestration-profile-foundation: decision (47) gives every parented key the base manual; this ticket reopens it
  261117-refactor-playbook-compression: prerequisite - compresses the manual first (77)
  261117-bug-pi-ticket-worker-profile-tools: lets a Pi worker call workflow_manual with its own key, the entry point role-scoped context would use
---

# 역할별 workflow manual 모듈과 subagent에게 보이는 note·agenda

## Background

Owner direction (2026-11-17), not yet settled into decisions: modularize the
workflow manual so each role (lead, sub-lead, ticket worker, delegate,
reviewer, explore) receives the tool-usage guidance for its own role; deliver
workflow context to subagents, not only the root lead; and let notes and
agenda entries be marked visible to subagents (for example a
`visible_to_subagent` flag). Placement under this epic was confirmed as (21)=a.

Current state (survey, 2026-11-17):

- `workflow_manual` always renders the single `lead-workflow-manual`
  playbook. File selection varies only by profile (top-level leads only) and
  harness (`native-spawn-binding.{claude,codex}.md`); there is no role axis.
- Workers (ticket-worker, ticket-worker-elevated, ticket-handler) call
  `workflow_manual` with their child key and receive the full lead manual,
  minus the whiteboard/tracker knobs and a few lead-only injections.
  Delegates (implementer, reviewer, explore) receive no manual; the nearest
  analogue is `rsrc/subagent-rules.md`, which the caller pastes into delegate
  prompts.
- Foundation decision (47): a parented key always gets the base files, so a
  sub-lead's manual is the full one. Role-keyed variants reopen it. Decisions
  (46)/(48) put file selection in `wsrsrc.Load` with the lookup order
  `<name>.<profile>.md`, `<name>.<harness>.md`, `<name>.md`.
- Notes: `wsnote.Record{Key, Value, Priority, WrittenAt, Visible}`; `Visible`
  is mute state, and there is no audience field. A worker's `workflow_manual`
  call shows every note of the root; delegates see none. An older binary
  drops unknown fields when it rewrites a note layer file, so an added field
  is not T0; a default of hidden-from-subagents fails closed under that strip.
- Agenda: per session key; a child key's agenda starts empty and nothing
  passes the lead's agenda to children.

Owner direction (2026-11-17 21:48, (75)), not yet a decision: a role's
manual could be the core plus usage guides attached per exposed tool - the
uses a schema cannot express, such as a worktree guide when the worktree
tools are open. The worktree release rule such a guide carries already
exists: a non-lead caller releases only as the holder or its lease parent
(`261116-feat-ws-subagent-worktree-lease` (1191)). This work starts from the
manual as compressed by `261117-refactor-playbook-compression` (77).

Decisions are settled in discussion before this ticket gains phases.

## Phases

To be written after discussion.
