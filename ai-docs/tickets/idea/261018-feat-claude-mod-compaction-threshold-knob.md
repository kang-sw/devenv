---
title: Configurable advisory threshold for the Claude mod lead compaction timing
related:
  261017-feat-claude-mod-lead-compaction-timing: shipped the fixed 50% advisory with no config key; this ticket revisits that decision
  261016-feat-pi-compaction-token-thresholds: the pi knob grammar (percent, token count, or "whichever comes first") a Claude knob would mirror
---

# Configurable advisory threshold for the Claude mod lead compaction timing

## Background

The ws Claude mod's lead compaction timing fires its advisory at a fixed
`ADVISORY_FRACTION = 0.5` of the compaction window (`rawMaxTokens`); the hard
point is Claude Code's own `autoCompactThreshold`, and the 1/3 and 2/3
milestones sit between the two. There is no ws config key: the only lever a
user has today is Claude Code's own window (`CLAUDE_CODE_AUTO_COMPACT_WINDOW`),
which moves every point at once.

pi exposes `pi.compaction_advisory_percent` / `pi.compaction_hard_percent`,
which accept a percent, a token count (`300k`), or one of each joined by `|`
("whichever comes first"), validated at `config.tune` time. The user asked
(2026-10-18) whether the Claude side takes the same string; it does not, and a
follow-up was approved.

The original no-key decision rested on "a config read would need an MCP tool
call the mod otherwise does not make". The mod is now one module that also
runs the mailbox push; whether that premise still holds is the first thing to
check.

## Open Questions

- How does the mod read the value: an MCP call at `session.start`, an
  environment variable, or something else the mod already has?
- Which points become configurable: only the advisory, or also a ws-side hard
  point below Claude Code's own `autoCompactThreshold`?
- Does a shared key serve both hosts, or does the Claude side get its own
  `claude.*` key with the same grammar?
- What does a token-count advisory above the compaction window do (clamp,
  ignore, or reject at tune time)?
