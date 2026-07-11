---
title: ws dashboard managed CLI recent sessions
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260624-feat-ws-dashboard-managed-cli-terminal: future follow-up after the first managed CLI surface is dogfoodable
  260620-feat-ws-dashboard-agent-client-activity-sources: shares the cross-provider common interactive subset and the Claude/Codex/OpenCode resume primitives this ticket's collapsed history list dispatches through (2026-07-11, tightened from "adjacent but separate")
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard managed CLI recent sessions

## Background

The first managed CLI terminal should close by terminating the vendor CLI
process and should not try to preserve daemon-owned PTYs across daemon restarts.
Longer term, the dashboard can provide a separate "recent sessions" surface by
parsing vendor-owned history/transcript stores for Codex, Claude, OpenCode, or
similar CLIs.

This is idea-level follow-up, not scope for
`260624-feat-ws-dashboard-managed-cli-terminal`. The first managed CLI
implementation may show a restore prompt for a remembered pane after daemon
restart, but it should not implement vendor history parsing or a flat recent
session browser.

## Direction

Recent sessions should appear as a flat list that distinguishes:

- which workspace/workRoot or Git root the conversation came from;
- which worktree held the vendor history record;
- when the conversation happened;
- which vendor/profile produced it;
- a vendor-derived conversation title or bounded fallback label.

Vendor history remains vendor-owned. The dashboard may parse known local history
formats for display and reopen affordances, but it must keep provider-native
session ids, cache paths, transcript paths, and ws session keys out of browser
route identity and ordinary command logs.

The list should be visually available from one place even when underlying vendor
history is stored separately by Git root and worktree. Any future implementation
needs explicit parser fixtures per vendor before promising restore or reopen
behavior.

## Seamless cross-harness collapse (owner, 2026-07-11)

Now that Codex app-server, OpenCode ACP, and the Claude CLI's stream-json
duplex mode are all confirmed viable interactive substrates (see
`260620-feat-ws-dashboard-agent-client-activity-sources`'s three-provider
Decisions and Phase 2/3/4), this ticket's collapsed list should let a human
browse and reopen conversation history across all three vendors from one
place, choosing seamlessly which one to continue without needing to know or
care in advance which harness produced a given entry. Concretely:

- **One unified list, per-entry vendor labeling**: entries from Codex,
  OpenCode, and Claude sit in the same flat list, sorted the normal way
  (e.g. recency), but each entry carries a visible vendor badge/label so the
  human can still tell which harness they are about to resume into — the
  collapse is about browsing/discovery convenience, not about hiding which
  CLI will actually run.
- **Discovery is vendor-history-file scraping, not a live provider call**:
  per the asymmetry noted in `260620` (Claude's CLI has no live
  "list sessions" method, unlike Codex app-server/OpenCode ACP), the
  uniform discovery mechanism across all three is reading each vendor's own
  on-disk history/transcript store directly. This works whether or not a
  live provider process is currently running for that session, which a
  live-list-call-only approach could not do for Claude.
- **Resume dispatches through the matching vendor mechanism**: selecting an
  entry reopens it through whichever mechanism that vendor actually supports
  — Claude via `claude --resume <session_id>` (see `260620` Phase 4's
  kill-and-respawn model), Codex via app-server thread resume, OpenCode via
  ACP session resume. The unified list is a browsing/selection surface, not
  a claim that all three vendors resume the same way underneath.
- **Known vendor history-store locations** (confirmed/unverified so far):
  Claude's local history is confirmed to live under
  `~/.claude/projects/<project-path-hash>/<session-id>.jsonl` (one JSONL
  transcript file per session, keyed by a hashed project path). Codex's and
  OpenCode's on-disk history store locations/formats are **not yet
  verified** in this research pass and need their own investigation pass
  (equivalent to the Claude CLI protocol spike already flagged in `260620`
  Phase 4) before parser fixtures can be written for them.

## Sequencing tension (flagged, not resolved)

`260624-feat-ws-dashboard-managed-cli-terminal` (the accepted nearer
todo-status milestone) is explicitly terminal-first and PTY-only, and
explicitly defers structured vendor-history parsing and a flat recent-session
browser to this idea ticket. A genuinely seamless, harness-labeled, resumable
cross-vendor history collapse is easier to deliver well once each vendor's
structured resume primitive (from `260620`'s provider adapters) exists, rather
than by relaunching a raw PTY blindly and hoping the vendor CLI's own
`--resume`-equivalent flag reproduces state correctly outside of a
daemon-managed adapter. This ticket's ambitions may therefore want at least
partial `260620` provider-adapter work as a prerequisite, which would be a
sequencing change from today's "260624 managed CLI terminal ships first,
260620 stays idea-level" ordering. Not resolved here — surfaced for the owner
to weigh when this ticket is promoted toward implementation-ready.
