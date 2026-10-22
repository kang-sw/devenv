---
title: Configurable advisory threshold for the Claude mod lead compaction timing
related:
  261017-feat-claude-mod-lead-compaction-timing: shipped the fixed 50% advisory with no config key; this ticket revisits that decision
  261016-feat-pi-compaction-token-thresholds: the pi knob grammar (percent, token count, or "whichever comes first") this knob mirrors
  261020-bug-claude-mod-milestone-percent-stale: sibling; also edits the timing.ts advisory and milestone texts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e138a1b8d605ec2b
sage-review-completeness-reviewed: e138a1b8d605ec2b
completed: 2026-10-21
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
  advisory text renders the configured point instead of the fixed `50%`, as
  the resolved percent of the compaction window (a token or combined value
  shows as that percent, like every other figure in the text).
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
  and Codex launches. A wrong path does not fail startup:
  `loadDeclaredConfig` rejects an unreadable manifest silently, the key stays
  undeclared, and the mod stays at 50% under D1/D6. So the check is that the
  key appears in `config.list` on each launch, not that the server starts.
- **D13. The key is listed, inert, on Codex.** `agents-plugin/.mcp.json` is
  shared by both hosts, so Codex's `config.list` shows the key; its
  description names Claude Code and nothing reads it there.
  Rejected: adding the manifest only under a Claude launch (needs a fragile
  host signal on the launcher's startup path for no behavioral gain).

## Constraints

- The new manifest entry declares no `default_scope: "repo"`; declared
  adapter keys may not default to `repo` (`1cdd2f6f`). Copying the
  `pi.compaction_advisory_percent` entry, which declares no `default_scope`,
  satisfies this.
- The session-start read is not awaited before the first prompt, so a
  `session.measure` can run before it settles. A read that settles after
  thresholds were cached still applies to that session (await it inside
  `measure` under the existing generation check, or invalidate the cache when
  it settles).
- pi (`agents-plugin-pi`) behavior is out of scope; its manifest is only read
  as the grammar reference.
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`; the mod, the manifest, and the key description ship downstream)
- Convention: `ai-docs/manuals/wsflow-mirroring.md` (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: `ai-docs/manuals/ws-mcp.md` (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Thresholds are measured against the compaction window. The advisory is a fixed 50% of the breakdown's rawMaxTokens in this first step, with no config key (a config read would need an MCP tool call...)" — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Thresholds run only when auto compaction is on. When isAutoCompactEnabled is false, autoCompactThreshold is absent, or it is not above the advisory point, the mod sends no advisory or milestone" — bearing: supports
- 52d7f6d0 (2026-10-18, commit): "The breakdown is read lazily at the first session.measure after each invalidation point (session start or end, landed compaction, window change)... A generation counter drops an observation that awaited the breakdown" — bearing: constrains
- db596a0c (2026-10-18, commit): "the original no-config-key premise (no MCP call from the compaction mod) needs rechecking now that one mod also runs the mailbox push." — bearing: supports
- 261016-feat-pi-compaction-token-thresholds (2026-10-16, Decisions): "No clamp. A token threshold beyond the window is not clamped; it simply never fires and Pi's native automatic compaction takes over. The combined form is how a user bounds a token threshold by a percent." — bearing: supports
- 261016-feat-pi-compaction-token-thresholds (2026-10-17, Result): "Comparison is in percent space: each threshold resolves to min(percent, tokens / window * 100) against the same window as the usage percent." — bearing: supports
- 261020-bug-claude-mod-milestone-percent-stale (2026-10-20, Decisions): "Head format. Context window: <n>% (<used>k / <window>k tokens) / <hard>% (automatic compaction point)., following Pi's advisory." — bearing: constrains
- 1cdd2f6f (2026-10-20, commit): "Declared adapter keys still may not default to repo for the same reason, and worktree. is now a reserved manifest namespace." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/timing.ts, agents-plugin/hooks/claude-mod/compaction.ts, agents-plugin/hooks/claude-mod/register.ts, new agents-plugin/config-manifest.json, agents-plugin/.mcp.json, wsflow mirror under agents-plugin-wsflow/hooks/claude-mod/, a Go test under agents-plugin-tool/internal/mcp/ |
| scope.surface | public-interface | new shipped config key claude.compaction_advisory_percent and a new adapter manifest loaded by the ws MCP server on both Claude and Codex launches |
| scope.new_public_symbol | yes | config key claude.compaction_advisory_percent |
| scope.new_type_contract | yes | key value grammar copied from pi.compaction_advisory_percent pattern in agents-plugin-pi/config-manifest.json#L27-L31 |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts, agents-plugin-tool/internal/mcp/config_manifest_test.go, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | WS_MCP_CONFIG_MANIFESTS loader agents-plugin-tool/internal/mcp/config_manifest.go#L22; pi resolveThresholdPercent agents-plugin-pi/src/goal-loop.ts#L265; mod $.mcp.call path agents-plugin/hooks/claude-mod/register.ts#L552-L557 |
| complexity.side_effect_risk | moderate | agents-plugin/.mcp.json is shared by Claude and Codex launches, so a wrong manifest path silently leaves the key undeclared on both hosts (loadDeclaredConfig never fails the server) |
| risk.correctness | moderate | threshold resolution must match pi's combined-form and no-clamp semantics and keep the existing measure generation-counter ordering with an async session-start read |
| risk.fit | moderate | concurrent edits to timing.ts advisory text by ready ticket 261020-bug-claude-mod-milestone-percent-stale |
| risk.test | moderate | live verification on both Claude and Codex launches is required beyond unit tests |
| risk.security_or_contract | moderate | adds a downstream-shipped config key and manifest contract that must stay pattern-identical to the pi key |

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
  to 50%; a read that settles after the first measure still applies; the
  advisory text names the configured point as the resolved percent.
- Go test: the `claude.` key is declared, patterned, accepts and rejects the
  same values as the `pi.` key, and `config.tune` validates it.
- Live: on a Claude launch, `config.tune` of the key then a new session shows
  the advisory at the configured point; on both Claude and Codex launches,
  `config.list` shows the key (proving the manifest path resolved).
- `claude plugin validate` / `claude plugin test` on both packages and the
  wsflow mirror drift guard.

### Result (a9ce2c084) - 2026-10-21

Landed `claude.compaction_advisory_percent` (default `50%`, pattern
byte-identical to `pi.compaction_advisory_percent`) in a new
`agents-plugin/config-manifest.json`, wired through
`WS_MCP_CONFIG_MANIFESTS`. The compaction feature reads it with one
`config_get` per session start (not awaited) over the shared server
resolution; `timing.ts` parses pi's grammar (`parseAdvisory` /
`resolveAdvisory`), resolves the advisory as the earlier term with no clamp
(`advisoryTokens`), and `thresholdsFor` / the advisory text use the resolved
point. Mirrored byte-identically to wsflow.

Decisions taken:

- D10/D12 adapted to code reality: Claude Code launches the server from
  `.claude-plugin/plugin.json` (`${CLAUDE_PLUGIN_ROOT}` launcher path, no
  `cwd`), not `.mcp.json`, which only Codex reads. The env is set in both:
  `./config-manifest.json` in `.mcp.json`, and
  `${CLAUDE_PLUGIN_ROOT}/config-manifest.json` in `plugin.json`. The
  `ws-mcp.md` launcher table records this.
- A late read invalidates the threshold cache (the Constraint's second
  option) instead of being awaited in `measure`; the latch is kept.
- The read survives `session.end` (a `clear` has no following
  `session.start`), and a read from an earlier start is dropped.
- `config_get` is called without a session key, so a session-scope tune is
  not seen; project, repo, global, and builtin scopes are.

Verification:

- `claude plugin test agents-plugin` and `agents-plugin-wsflow`: 317/317
  each (new unit tests for grammar, resolution, combined/no-clamp, lone term
  beyond hard, malformed and failed reads, a late read, a read settling
  mid-observation, per-start reads; engine tests for the one `config_get` per
  start and the fallbacks). A mutation dropping the cache invalidation fails
  three of them.
- `go test ./...` in `agents-plugin-tool`: all ok, including the new
  `TestShippedClaudeAdvisoryMatchesPi` and
  `TestConfigTuneValidatesClaudeAdvisory`; `go test ./internal/wsrsrc` (mirror
  drift guard) ok.
- `claude plugin validate` on both packages passed;
  `python3 -m unittest discover` on `agents-plugin/tests` (78) and
  `agents-plugin-wsflow/tests` (15) ok.
- Live, Claude: a `claude -p --plugin-dir agents-plugin` session tuned the key
  to `40%` (accepted, so the manifest resolved under the Claude launch); the
  next session's debug log showed `advisory point "40%" read as 40%` and
  thresholds at 40%, milestones 56% and 73%, auto compaction 89%.
- Live, Codex-shaped: the launcher started from the plugin root with
  `WS_MCP_CONFIG_MANIFESTS=./config-manifest.json` lists the key in
  `config.list`; started from another cwd it does not (control). The
  installed Codex plugin cache was not exercised: it needs a user-performed
  plugin refresh.

#### Edition (0fed7fd40) - 2026-10-22

Correction to the Result's scope bullet. A keyless `config_get` leaves the
server's repo root unanchored (`sessionConfigOptions` sets `RepoRoot` only from
a known session key), so the repo layer is skipped as well as the session
layer: the mod reads project, global, and built-in values only. The release
gate review over `a278ef1fe..0fed7fd40` surfaced this; the user decided the
compaction point does not need the repo scope, so the behavior stays and the
key description in `agents-plugin/config-manifest.json` now states the
limitation instead.
