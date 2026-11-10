---
title: "Dashboard agent host: peer lead branch, and subagent model, effort, context tokens, and output rate in the agents panel"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-session-rail: "sibling multi-session UX child; independent"
  261110-feat-ws-hosted-session-mode-flag: "turns the dashboard feed off for daemon-hosted sessions"
  261110-feat-ws-dashboard-headless-lead-sessions: "supplies these fields for daemon-hosted sessions from the stream"
---

# Agents panel telemetry

## Background

Track A of the epic, for the sessions that exist today: with many sessions
and subagents running, the agents panel should say where a peer lead works
and what each subagent is and how fast it runs (2026-11-10 whiteboard,
topic "멀티 세션 관리 UX").

What the panel has today (Explore, 2026-11-10):

- **Peer lead rows** (`peerrow`, `assets/dashboard.tmpl`; `PeerRow`,
  `pageview.go:151-205`; built by `peerGroup`, `pageview.go:539-625`)
  already show the folder as a `main(folder)/` prefix (`Folder()`,
  `pageview.go:631`; `peerFolderLabel`) with the full path as a tooltip. No
  branch: `PeerRow` and `Registration` carry none. A served peer's snapshot
  has it (`Snapshot.Repo.Branch`, `internal/wslive/snapshot.go:140-142`,
  `:194-208`), and `peerGroup` already looks that snapshot up
  (`pageview.go:586`).
- **Subagent rows** (template `agent`; `AgentNode`, `pageview.go:115-131`;
  `wslive.Agent`, `internal/wslive/snapshot.go:250-259`) show label, reply
  ID, phase, caption, done and failed counts, and duration. No model or
  reasoning effort exists anywhere in the snapshot or the adapter event
  (`wslive.Event`, `internal/wslive/state.go:65-80`). Per-batch token usage
  arrives on the wire (`usage` in `batchIn`, `internal/wslive/state.go`) and
  the Claude mod fills it (`stepTokens`, `agents-plugin/hooks/claude-mod/dashboard.ts`),
  but `BatchView.Usage` is `json:"-"` (`internal/wslive/snapshot.go:295`):
  only the lead's tool summary reads it. Nothing computes a rate.
- **Hover**: the panel uses native `title` attributes only. Custom tooltips
  exist elsewhere: the git graph's `gltip` (`assets/dashboard.js:2498-2518`).
- **Pi reference**: the Pi subagent gutter shows `contextTokens` and
  `outputTps` (`agents-plugin-pi/src/agent-display.ts:16-18`, `:43-44`). Its
  rate comes from `createOutputRateTracker` (`agents-plugin-pi/src/output-rate.ts`):
  only text and tool-call block spans count, so request latency, gaps
  between blocks, and tool execution never enter the denominator; reasoning
  tokens and thinking spans are excluded when the provider reports a
  reasoning breakdown; token-weighted over the last 8 messages.

## Decisions

1. **Peer lead rows show the branch (user's request; technical, (31) ok).**
   A served peer's `Snapshot.Repo.Branch` is copied into `PeerRow` and shown
   as a chip after the name; a peer the dashboard does not serve shows the
   folder only. The folder is already shown. Rejected: reading an unserved
   peer's branch with git from the daemon, which would widen the current
   rule that the daemon reads git only for roots with an open stream
   (`gitbadge.go`).
2. **Subagent rows show model and effort, context tokens, and output rate
   (user's request).**
   - Model and effort come from the adapters: the Claude mod (mirrored into
     wsflow) and the Pi dashboard feed send them for each subagent, and
     `wslive.Agent` gains the fields (technical, (33) ok).
   - "Tokens" means the subagent's current context size, as the Pi gutter's
     `contextTokens` (user, (32) a and note; announced, (38) ok). Rejected:
     cumulative input plus output ((32) b); cumulative output ((32) c).
   - The rate is output tokens per second as the Pi gutter defines it
     (announced, (38) ok); it supersedes the simpler "current step output
     divided by elapsed time" first proposed in (33). Not shown for ended
     subagents.
   - The Pi side sends the values its gutter already computes. The Claude
     side matches the Pi definition as closely as its hook events allow.
3. **Narrow panels move the stats into a hover card (technical, (34) ok).** A
   container query on the panel's own width hides the stats from the row,
   and a custom hover card, following the `gltip` pattern, shows them. The
   threshold is the panel's width, not the viewport's, so the three-column
   layout of `261110-feat-ws-dashboard-session-rail` narrows it correctly.
   Rejected: native `title` tooltips (slow to appear, no line structure).

## Constraints

- Independent of the session holder, daemon lead sessions, and the terminal
  ((36) ok); it can land now.
- Scope is CLI-created sessions, through the adapters. Daemon-hosted
  sessions run with the dashboard feed off
  (`261110-feat-ws-hosted-session-mode-flag` Decision 1), so
  `261110-feat-ws-dashboard-headless-lead-sessions` supplies the same fields
  for them from the stream-json output ((41) ok).
- Verify first whether Claude Code hook events expose a subagent's actual
  model and reasoning effort. If they do not, use the spawn input's model
  parameter, then the agent definition's default ((33) ok). Likewise verify
  which Claude events can bound text and tool-call spans for the rate, and
  record in the Result how close the Claude rate comes to the Pi definition.
- Exposing per-batch usage in the snapshot must keep the lead's tool summary
  (`internal/wslive/toolsummary.go`) working unchanged.
- The Claude mod is mirrored byte-identically into `agents-plugin-wsflow`
  (`internal/wsrsrc/wsflow_hooks_mirror_test.go`).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`.

## Phases

### Phase 1: peer branch and subagent telemetry

Implement Decisions 1-3.

Verification:

- Go tests: `PeerRow` carries the branch for a served peer and none for an
  unserved one; the snapshot round-trips the new agent fields and usage;
  the lead's tool summary output is unchanged.
- Adapter tests: the Claude mod and the Pi feed emit model, effort, context
  tokens, and rate for a subagent; the Pi values equal what its gutter shows
  for the same events; the wsflow mirror test passes.
- A browser check: stats inline on a wide panel, in the hover card on a
  narrow one; the rate disappears when a subagent ends.
