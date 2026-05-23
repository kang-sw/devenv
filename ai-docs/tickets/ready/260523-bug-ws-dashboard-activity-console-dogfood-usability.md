---
title: Activity Console dogfood usability repair
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260518-epic-ws-dashboard-activity-console: initial Activity Console milestone that exposed this dogfood gap
  260513-feat-async-exec-output-reader: future exec activity source remains separate from named-agent transcript repair
spec:
  - 260517-ws-dashboard-workroot-activity-pane
  - 260521-ws-dashboard-activity-console-ui-shell
  - 260522-ws-dashboard-activity-console-transcript-expansion
plans:
  phase-1: 2026-05/23-1049.activity-console-dogfood-phase1
  phase-2: 2026-05/23-1148.activity-console-dogfood-phase2
  phase-3: 2026-05/23-1221.activity-console-dogfood-phase3
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
- Codex native transcript parsing leaves too many records unsupported. The
  implementation pass should actively explore observed Codex native record
  shapes and reduce unsupported volume as much as fixture-backed evidence
  allows. Lead prompts, follow-up prompts, interrupt messages, tool/MCP
  activity, patch/apply outcomes, and similar handoff records should appear as
  normalized transcript blocks when their native shape is understood.
- Remaining unsupported or malformed native records must still convey useful
  safe information instead of ending at a generic "unsupported log line".
  Expanded detail may show bounded structural summaries such as record category,
  known safe fields, byte counts, status/outcome hints, and omission reasons,
  but must not expose raw JSON, raw record type strings that can carry private
  material, payload snippets, session ids, paths, or tool-output leakage.

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

### Result (c82d7c43) - 2026-05-23

Implemented support-split WorkRoot Activity placement and transcript scroll
stability. New Activity panes now route through opened-surface placement so they
prefer the support split, while duplicate badge opens focus the existing pane in
its current group and close remains browser-only/reversible. Activity pane group
ownership is persisted through App-owned pane order rather than a hard-coded
group-1 render path.

Transcript blocks now tail-follow by default, pause when the user scrolls away,
and preserve workRoot/activity-scoped scroll memory across feed refresh,
selected transcript refresh, stream invalidation, and workbench rerender paths.
Browser acceptance verifies support-split placement, horizontal ribbon overflow,
and refresh-after-scroll stability with response-applied transcript evidence.

Deferred: backend tail-page pagination remains unchanged; compact transcript
summary quality and Codex native transcript coverage stay in later phases.

### Phase 2: Improve compact transcript summaries

Change compact transcript rendering so the default one-line view prefers a
bounded semantic summary over a generic title. Tool calls should include the
called tool/command name and a safe argument hint when available; tool results
should expose bounded outcome/byte/status information; status-like blocks
should display their meaningful text instead of only a repeated label.

Keep vertical density stable: the improvement is better text inside the existing
one-line compact block, not larger default blocks. Detail expansion remains
available for richer safe payload summaries.

### Result (4f913e6d) - 2026-05-23

Implemented frontend compact transcript summary selection. Compact tool-call
blocks now prefer the normalized safe tool name plus argument-byte hint, tool
results show available outcome/status/exit/byte hints, and status or degraded
blocks prefer meaningful normalized first-line text over generic category
labels. Summary strings are bounded and the one-line density, detail expansion,
terminal rendering, and backend parser coverage remain unchanged.

Deferred: broader Codex prompt, interruption, MCP/tool, and patch/apply native
record coverage remains Phase 3.

### Phase 3: Expand Codex prompt and interruption transcript coverage

Use fixture-backed Codex native session records to identify prompt,
continuation, interrupt, lead-agent handoff, MCP/tool activity, patch/apply, and
other common observed shapes. Normalize supported records into source-neutral
`TranscriptBlock` values for user/input/status/tool content so live lead-agent
sessions show the prompts and activity that explain agent behavior.

The implementation should survey enough recent/native fixture evidence to
meaningfully reduce unsupported record volume before settling parser coverage.
Unsupported record handling stays graceful and privacy-preserving, but it should
not be content-free. If fixture evidence is insufficient for a native shape,
render a bounded degraded block or aggregate that provides safe structural
context and omission reason while avoiding raw JSON, private record strings,
payload snippets, session ids, paths, and tool output.
