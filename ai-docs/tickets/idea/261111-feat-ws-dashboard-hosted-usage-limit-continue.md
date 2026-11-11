---
title: "Daemon-hosted lead sessions: resume automatically after a usage limit resets"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "builds on its hosted Process and typed error states (Phase 2)"
parent: 261110-epic-ws-dashboard-agent-host
---

# Daemon-hosted lead sessions: resume automatically after a usage limit resets

## Background

User, 2026-11-11 ((201) a): capture as an idea, out of current scope.

A daemon-hosted lead runs for long stretches without the user watching. When
it hits the provider's usage limit, the turn ends with a quota error and the
session sits idle until someone notices, even after the limit resets.

Orca ADE (github.com/stablyai/orca, MIT, commit bb1d4789c) handles this in
its native chat. After the limit's reset time passes, it sends "Continue where
you left off." to the session, and stops after 5 continuations in a row
(`src/main/native-chat/agent-session-wire/recovery/structured-agent-session-usage-limit-continue.ts`,
`structured-agent-session-usage-limit-chain.ts:22`). It reads the reset time
from the typed usage-limit error (`src/main/claude/claude-usage-limits.ts:18-80`).

## Open Questions

- Opt-in per session or a dashboard-wide default.
- The continuation text, and how the dashboard marks a turn the daemon sent.
- The chain cap, and what resets it.
- The equivalent signal for Pi.

## Phases

### Phase 1: Auto-continue after a usage limit

Depends on the typed quota error state with its reset time, which
`261110-feat-ws-dashboard-headless-lead-sessions` Phase 2 adds ((200) b).
Settle the open questions before promotion.
