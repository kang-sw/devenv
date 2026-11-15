---
title: Design conversation-visible prompt refusal with old-dashboard compatibility
related:
  261114-bug-ws-live-lead-state-prompt-compaction: Busy-state repair deliberately retains controls-menu-only refusal visibility
---

# Design conversation-visible prompt refusal with old-dashboard compatibility

## Background

The busy-state repair preserves truthful prompt-refusal text in the existing
`State.Error` field while retaining `StatusRunning`. That field is visible
only when opening the controls menu, not as a conversation-history entry.

Owner decision (723)=a, relayed by MAIN on 2026-11-14, explicitly separates
conversation refusal visibility from that immediate repair. This idea records
the required compatibility design, not approval of a new representation.

## Evidence

The pre-repair template at
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:293` renders a
notice through `noticeword`; `:299` recognizes `login` and `model`, then labels
all other kinds with `notice.limit`. An additive unknown notice kind can
therefore be mislabeled as a usage-limit failure by an older dashboard.
This is source evidence, not a mixed-version runtime acceptance result.

## Decisions

- Capture conversation-visible refusal as a separate design task, per (723)=a.
  The immediate repair remains menu-only and does not depend on this task.
- Compatibility tier is not settled for the future representation. Do not
  claim T0 merely because a proposed notice kind is additive: reader behavior
  must be accounted for before the design is approved.
- Rejected for the immediate repair: shipping a new conversation notice on
  the superseded (715) authority or reusing a login/model/usage-limit meaning
  for an unrelated refusal.

## Constraints

- No new notice kind, shared-ring shape, protocol change, runtime activation,
  or implementation is approved by this capture.
- Design must cover old-dashboard/new-writer and new-dashboard/old-writer
  behavior without mislabeling a refusal as a usage, login or model failure.
- Preserve the busy-state repair's native-run activity and explicit refusal;
  do not silently replay or queue rejected owner input.
- Before tooling edits, read `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`. Any broader compatibility
  choice returns through MAIN under the project's approval protocol.

## Phases

### Phase 1: Settle a compatibility-safe conversation refusal design

Trace refusal production, existing error visibility, notice serialization and
old/new dashboard rendering. Propose a representation and mixed-reader
handling with an explicit compatibility tier; return the unresolved design
choices through MAIN before ready promotion or implementation.

Verification expectations for the settled design: accurate conversation
refusal text, continued native-run status, no silent resubmission, and a
mixed-version matrix proving that old readers do not present the new refusal
as a usage-limit/login/model failure. Preserve existing notice meanings and
state limits when an old reader cannot display the proposed representation.
No implementation or verification Result is claimed here.
