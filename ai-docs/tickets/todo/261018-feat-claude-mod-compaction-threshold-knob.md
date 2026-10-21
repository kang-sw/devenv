---
title: Configurable advisory threshold for the Claude mod lead compaction timing
related:
  261017-feat-claude-mod-lead-compaction-timing: shipped the fixed 50% advisory with no config key; this ticket revisits that decision
  261016-feat-pi-compaction-token-thresholds: the pi knob grammar (percent, token count, or "whichever comes first") this knob mirrors
  261020-bug-claude-mod-milestone-percent-stale: sibling; also edits the timing.ts advisory and milestone texts
---

# Configurable advisory threshold for the Claude mod lead compaction timing

## Background

The ws Claude mod's lead compaction timing fires its advisory at a fixed
`ADVISORY_FRACTION = 0.5` of the compaction window (`rawMaxTokens`); the hard
point is Claude Code's own `autoCompactThreshold`, and the 1/3 and 2/3
milestones sit between the two (`timing.ts` `thresholdsFor`). There is no ws
config key: the only lever a user has today is Claude Code's own window
(`CLAUDE_CODE_AUTO_COMPACT_WINDOW`), which moves every point at once.

pi exposes `pi.compaction_advisory_percent` / `pi.compaction_hard_percent`,
which accept a percent, a token count (`300k`), or one of each joined by `|`
("whichever comes first"), declared in `agents-plugin-pi/config-manifest.json`
and validated at `config.tune` time.

The original no-key decision rested on "a config read would need an MCP tool
call the mod otherwise does not make". That premise no longer holds: the
mod's workflow manual feature already calls ws MCP tools at session start
(`$.mcp.call` in `register.ts`).

## Decisions

- **D1. Read at session start, fall back to 50%.** The mod reads the knob
  once per session start through its existing MCP path; any failure (no
  server, call error, malformed value) leaves the advisory at 50%.
- **D2. Only the advisory is configurable.** The hard point stays Claude
  Code's `autoCompactThreshold`.
  Rejected: a ws-side hard point below Claude Code's own - the mod would have
  to start compaction itself, a much larger scope.
- **D3. A Claude-specific key with pi's grammar.** The key is
  `claude.compaction_advisory_percent` (default `50%`), accepting exactly
  the values `pi.compaction_advisory_percent` accepts.
  Rejected: one key shared by both hosts - the hosts' compaction points mean
  different things (pi owns the summary; Claude Code compacts on its own).
- **D4. Thresholds resolve as pi resolves them.** The combined form takes
  the earlier point; a token term is not clamped to the window (pi
  `resolveThresholdPercent`). A term at or beyond the hard point never fires,
  so in the combined form the other term decides, and a lone such term sends
  no advisory and no milestones (`thresholdsFor` already returns undefined
  when the hard point is not above the advisory).
  Rejected: rejecting large token values at `config.tune` time - the window
  varies by model, so tune time cannot know it.
- **D5. Milestones follow the configured advisory.** Milestones stay at one
  and two thirds between the configured advisory and the hard point, and the
  advisory text renders the configured point instead of the fixed `50%`.
- **D6. Never hard-fail on a stored value.** A stored value that does not
  parse (written outside `config.tune`) falls back to 50%, as pi's
  `resolveCompactionAdvisoryThreshold` does.
- **D7. No ordering against the milestone bug.** Related to
  `261020-bug-claude-mod-milestone-percent-stale` without `blocked-by:`; the
  shared `timing.ts` text overlap is resolved at merge, as that ticket did
  with `261020-feat-claude-mod-lever-continue-flag`. The bug waits on a live
  probe and does not gate this ticket.
- **D8. wsflow stays inert.** The mod edit is mirrored byte-identically;
  wsflow's `.mcp.json` declares no manifest and its mod reads no key, since
  compaction timing does not run there.
- **D9. One read per session start.** The compaction feature's session-start
  setup reads the key with one `config_get` over the mod's shared server
  resolution; a mid-session `config.tune` applies from the next session
  start.
  Rejected: re-reading on every measure - repeated calls for a value that
  rarely changes.
- **D10. Adapter manifest, not a built-in key.** The key is declared in a
  plugin-local `agents-plugin/config-manifest.json` (namespace `claude.`)
  wired through `WS_MCP_CONFIG_MANIFESTS` in `agents-plugin/.mcp.json`, the
  mechanism pi uses for its adapter keys (Architecture Rule 3).
  Rejected: a Go built-in key - host-specific behavior belongs in adapters.
- **D11. Pattern drift test.** A Go test asserts the `claude.` and `pi.`
  advisory keys carry the same pattern.
  Rejected: generating both manifests from one source - a generation step
  for two JSON files in separate packages costs more than a drift test.
- **D12. Verify manifest path resolution live.** The Phase verifies the
  manifest path resolves from `.mcp.json`'s `cwd: "."` under both the Claude
  and Codex launches; a wrong path is a startup failure on both hosts.
- **D13. The key is listed, inert, on Codex.** `agents-plugin/.mcp.json` is
  shared by both hosts, so Codex's `config.list` shows the key; its
  description names Claude Code and nothing reads it there.
  Rejected: adding the manifest only under a Claude launch (needs a fragile
  host signal on the launcher's startup path for no behavioral gain).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the mod, the
  manifest, and the key description ship downstream),
  `ai-docs/manuals/wsflow-mirroring.md`, and `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/` config manifest loading).
- pi (`agents-plugin-pi`) behavior is out of scope; its manifest is only read
  as the grammar reference.

## Phases

### Phase 1: Configurable Claude advisory threshold

Declare `claude.compaction_advisory_percent` in a plugin-local config
manifest wired into the ws MCP launch, read it once at the compaction
feature's session start, and replace `ADVISORY_FRACTION` in `thresholdsFor`
and the advisory text with the resolved threshold per D1-D4. Mirror to
wsflow.

Verification:

- Mod tests cover: default 50% with no key; a percent, a token count, and a
  combined value; a combined value whose token term exceeds the hard point
  resolves to the percent term; a lone term at or beyond the hard point sends
  nothing; a malformed stored value and a failed `config_get` both fall back
  to 50%; the advisory text names the configured point.
- Go test: the `claude.` key is declared, patterned, accepts and rejects the
  same values as the `pi.` key, and `config.tune` validates it.
- Live: on a Claude launch, `config.tune` of the key then a new session shows
  the advisory at the configured point; on a Codex launch, the server starts
  with the manifest loaded.
- `claude plugin validate` / `claude plugin test` on both packages and the
  wsflow mirror drift guard.
