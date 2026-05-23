---
title: Activity Console dogfood usability repair
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260518-epic-ws-dashboard-activity-console: initial Activity Console milestone that exposed this dogfood gap
  260513-feat-async-exec-output-reader: future exec activity source remains separate from named-agent transcript repair
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# Activity Console dogfood usability repair

## Background

Dogfood feedback after the initial Activity Console milestone found that the
surface is not yet useful enough for real lead-agent monitoring. The shell,
stream, and Codex transcript plumbing exist, but default placement, transcript
scroll behavior, compact block summaries, and Codex native record coverage make
the current browser view hard to inspect.

This ticket repairs that usability debt without adding dashboard-side agent
control actions. Activity visibility stays read-only and every visible control
continues through the dashboard command dispatch path for future tmux-like
keybindings.

## Feedback Capture

- Default WorkRoot Activity panes should prefer the second/support split group
  when available instead of occupying the first agent/terminal split.
- Transcript views should tail-follow by default until the user intentionally
  scrolls away, and the WorkRoot Activity pane must not snap its scroll position
  back upward during split use or live refreshes.
- Compact transcript blocks need a meaningful single-line summary. Tool calls,
  command-like activity, MCP activity, and status blocks should show useful
  inline content without increasing vertical density or requiring `More`.
- Codex native transcript parsing leaves too many records unsupported. Lead
  prompts, follow-up prompts, interrupt messages, and similar handoff records
  should appear as normalized transcript blocks when fixture evidence identifies
  their native shape.
- Unsupported or malformed native records must remain bounded and redacted; the
  repair must not reintroduce raw record type, payload, session id, path, or
  tool-output leakage.

## Phases

### Phase 1: Repair placement and transcript scrolling

Move WorkRoot Activity pane default placement to the support split policy while
preserving focus-existing behavior and reversible close semantics. Add browser
evidence for the default two-split layout and narrow-width horizontal ribbon
behavior.

Make transcript scrolling tail-follow by default for newly selected or live
updating activity, but pause auto-follow when the user scrolls away from the
tail. Fix the observed WorkRoot Activity pane snap-back behavior in left/right
split layouts so ordinary user scrolling remains stable across feed refresh,
transcript refresh, and Dockview rerender paths.

Verification should include focused frontend unit coverage for scroll policy
and browser-level evidence against the daemon-served production frontend.

### Phase 2: Improve compact transcript summaries

Change compact transcript rendering so the default one-line view prefers a
bounded semantic summary over a generic title. Tool calls should include the
called tool/command name and a safe argument hint when available; tool results
should expose bounded outcome/byte/status information; status-like blocks
should display their meaningful text instead of only a repeated label.

Keep vertical density stable: the improvement is better text inside the existing
one-line compact block, not larger default blocks. Detail expansion remains
available for richer safe payload summaries.

### Phase 3: Expand Codex prompt and interruption transcript coverage

Use fixture-backed Codex native session records to identify prompt,
continuation, interrupt, and lead-agent handoff shapes. Normalize supported
records into source-neutral `TranscriptBlock` values for user/input/status
content so live lead-agent sessions show the actual prompts that explain agent
behavior.

Unsupported record handling stays graceful and privacy-preserving. If fixture
evidence is insufficient for a native shape, keep it degraded rather than
guessing from raw payloads.
