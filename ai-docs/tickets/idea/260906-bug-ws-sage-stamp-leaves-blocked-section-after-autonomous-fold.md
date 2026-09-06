---
title: tickets.sage_stamp leaves a stale Blocked section on a ticket whose issues were all folded autonomously
spec:
  - mcp-tools
---

# tickets.sage_stamp leaves a stale Blocked section on a ticket whose issues were all folded autonomously

## Background

Dogfood, 2026-09-06, on the Pi track. Three tickets in `ready/` carried a
`## Blocked (2026-09-06)` section rendered by `tickets.sage_stamp`
(`agents-plugin-tool/internal/wsdoc/tickets_sage.go`, `renderBlockedSection`
via `appendOrReplaceBlockedSection`) from a first review pass whose verdict
was `block`. Every issue in those tables carried `resolution: autonomous`,
the lead folded them all, a later stamp recorded the re-review as
`concern`/`pass` and set both `sage-review-*: completed`, and
`tickets.move` promoted the tickets to `ready/`. The Blocked section
survived all of that: `appendOrReplaceBlockedSection` runs only when the
new verdict is itself `block` (`sageRecordSingle` and `sageRecordCombined`
both guard on `verdict == "block"`), so a non-block stamp never excises the
prior section.

The section is not decoration. The `lead-drain-ready-queue` skill tells its
selector to skip any candidate carrying a recorded blocker note and names a
`## Blocked (...)` entry as the example, so a ready ticket with a stale
table is invisible to a goal-run drain even though nothing blocks it. In
the same session the owner's drain was expected to pick
`260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`, which was
in exactly that state. The lead removed the stale tables by hand
(commit on the Pi track) so the drain could proceed.

## Proposed direction

Host-neutral ws-mcp change under `agents-plugin-tool/`, landing through
`develop`.

- A stamp whose recorded verdicts contain no `block` and no issue with a
  missing resolution excises any prior `## Blocked (` section from the
  ticket (the excision half of `appendOrReplaceBlockedSection`, split into
  its own helper), so a resolved review leaves no blocker note behind.
- A stamp whose verdicts are `concern` with only autonomous issues does not
  render a Blocked section at all; the review table is already captured in
  the commit message and the frontmatter posture. Only `block`, or any
  issue with `resolution: missing`, renders the section.
- `tickets.verify` warns when a ticket in `ready/` or `todo/` carries a
  `## Blocked (` section while both postures read `completed`, naming the
  section as a probable stale blocker.
- Owner-written blocker notes (a `## Blocked (...)` heading followed by
  prose, such as the sign-off note on
  `260905-feat-ws-pi-harness-config-layer`) are never touched by the
  excision: only the Go-rendered table shape (the `### <Reviewer> — <verdict>`
  subsections) is recognized and removed.

## Spec Impact

`mcp-tools`: the `tickets.sage_stamp` contract gains the excision rule and
the narrowed render condition; the `tickets.verify` guardrail list gains
the stale-blocker warning.

## Constraints

- No change to the posture fields or to `tickets.move`'s upward-move
  validation.
- The rendered table shape is unchanged so existing tickets that still
  carry one are recognized by the excision.

## Phases

### Phase 1: Excise on resolution, render only on block

Split the excision out of `appendOrReplaceBlockedSection`, call it from
both record paths when the stamped verdicts carry no block and no missing
resolution, narrow the render condition, and add the verify warning.
Tests: a block stamp followed by a concern-autonomous stamp leaves no
Blocked section; a concern-autonomous first stamp renders none; a stamp
with a missing resolution still renders; an owner-written prose Blocked
note survives a resolving stamp; verify warns on a completed-posture
ticket with a rendered table and stays quiet on a prose note. Amend the
spec passages.
