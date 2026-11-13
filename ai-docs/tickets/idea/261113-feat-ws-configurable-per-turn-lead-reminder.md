---
title: "A configurable reminder appended to every lead turn"
---

# A configurable reminder appended to every lead turn

## Background

Owner proposal, 2026-11-13. The owner's user-level `UserPromptSubmit` hook
already appends `(응답:한국어)` to every turn the host starts, including turns
a plugin or the mailbox submits. The owner wants the same channel for a
workflow reminder set through `config.tune`, e.g. `[reminder: FULL
DELEGATION MODE]`, so a standing operating mode stays salient across a long
session; the same day the lead drifted from full delegation into running
builds and tests itself, which such a reminder would have caught. ws ships no
`UserPromptSubmit` hook today.

## Idea

- A config key (working name `lead.turn_reminder`, string, empty = off) set
  through `config.tune`; the ws plugin's hook appends its value to every turn
  of a lead session.
- Lead sessions only: a worker or delegate must not receive a lead-mode
  reminder (it would invert the instruction). Resolve lead vs worker from the
  session key the hook can see.
- Compatibility: a new config manifest field, T0. A new plugin hook is an
  observable workflow behavior change: ask before ready.

## Open

- Harness coverage: Claude Code `UserPromptSubmit` additional context is
  direct; Codex hook feedback semantics differ (see
  `ai-docs/manuals/codex-integration.md`); Pi needs an extension-side
  injection.
- Scope of the value: global, per project, or per session (a mode the lead
  sets for one session only).
- Whether the reminder also rides compaction recovery (lead-revive) output.
