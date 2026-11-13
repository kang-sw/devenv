---
title: "Dashboard rail: per-harness plan-usage gutter in the bottom-left corner"
related:
  261113-feat-ws-dashboard-harness-auth-slots: follow-up (account switching, Orca config-home model)
  261111-feat-ws-dashboard-hosted-usage-limit-continue: neighbour (consumes the same Claude limit data)
  261028-research-claude-mod-statusline-band: neighbour (claude-mod statusline rate-limit figures)
---

# Dashboard rail: per-harness plan-usage gutter in the bottom-left corner

## Background

Owner request (2026-11-13, dashboard UX side track; whiteboard topic
`usage-gutter`, items (130)-(135), (137)). The session rail's bottom-left
corner holds a "전체 세션" (`nav.sessions`) link. The owner wants that spot to
show the plan usage of each harness they are logged in to: a compact gutter by
default (Claude: two vertical bars, 5-hour and weekly window; Codex/GPT: one
vertical bar, weekly window only), a click opening the details. Remembering
and swapping auth slots is a later, separate ticket
(`261113-feat-ws-dashboard-harness-auth-slots`).

Research (2026-11-13):

- Footer: `wsdashboard/assets/dashboard.tmpl` rail `<div class="rfoot"><a
  href="/">{{t "nav.sessions"}}</a></div>` (~279); CSS `.rail .rfoot`
  (`dashboard.css` ~1603). The same `nav.sessions` key labels `#railbtn`
  (`dashboard.tmpl` ~10), which already links to the session list.
- Claude limits already reach the daemon per hosted session:
  `wsagent/claude.go` (~895-903) parses `rate_limit_event`
  (`unifiedWindows` five_hour / seven_day utilization and resetsAt);
  `wslive/snapshot.go` (~190-201) `Usage.Limits`; `pageview_band.go`
  (~98-175) renders the per-session limit chips with
  `bandWarnPercent` / `bandCritPercent`. The claude-mod feed carries the
  statusline rate-limit figures (`agents-plugin/hooks/claude-mod/dashboard.ts`
  ~619-662 `statusFigures`, mirrored in `agents-plugin-wsflow`).
- Codex: no adapter in the daemon. The Codex CLI (0.155.0 on the owner's
  machine) is logged in with a ChatGPT account (`~/.codex/auth.json`);
  `codex app-server` answers `account/rateLimits/read`. Codex rollout files
  (`~/.codex/sessions/**/rollout-*.jsonl`, `token_count`
  `payload.rate_limits`: primary/secondary `used_percent`, `window_minutes`,
  `resets_at`, `plan_type`) show the same shape but are stale when the CLI is
  rarely used - the owner uses GPT mostly through Pi's OAuth login.
- Pi: stores its ChatGPT OAuth credentials in `~/.pi/agent/auth.json`
  (`openai-codex`: `type`, `access`, `refresh`, `expires`, `accountId`) and
  refreshes them itself; it does not read the `x-codex-*` rate-limit headers.
  Whether the Pi and Codex CLI logins are the same ChatGPT account is
  unverified.
- Rail popover pattern: `railPopOpen` (`dashboard.js` ~5967-5990), with the
  outside-click exclusion list around ~6000-6030.
- Reference design (stablyai/orca, MIT; design reference only, no code
  copied): status-bar inline usage plus a popover roster sorted tightest
  first; an 80% warning; explicit status texts for missing data; a stale-window
  guard (a value with an older reset is dropped, usage is monotonic within a
  window); Claude via statusline push first; Codex via a short-lived
  `codex app-server` `account/rateLimits/read` with a `wham/usage` HTTP
  fallback; windows classified by duration; no token refresh.

## Decisions

1. **Claude data is push only** ((130)=a): the daemon aggregates the values it
   already receives - hosted Claude session `rate_limit_event`s and the
   claude-mod statusline feed - keeps the latest per window under the Orca
   stale-window guard, and persists it with an `asOf` time so the gutter
   survives a daemon restart and shows "n min ago". No network call and no
   credential read for Claude.
   Rejected: adding the OAuth `/api/oauth/usage` endpoint as a fallback when no
   Claude session exists (credential read, undocumented endpoint, 429 under
   polling).
2. **Phase 1 is display only** ((131)=a); account switching is
   `261113-feat-ws-dashboard-harness-auth-slots`.
   Rejected: switching in phase 1; swapping the default credential files.
3. **Codex/GPT data is a live query** ((133)=b, (137)=c): primary source is a
   short-lived `codex app-server` process answering
   `account/rateLimits/read` (the Codex CLI manages its own token). When no
   Codex CLI login is detected, the fallback is a `wham/usage` HTTP query
   with Pi's stored `openai-codex` token (see decision 4). Windows are
   classified by `window_minutes` (10080 = weekly), never by primary /
   secondary position.
   Rejected: rollout files only (stale for this owner, (133)); Codex CLI only
   ((137)=a, misses a Pi-only login); Pi token only ((137)=b).
4. **Pi token fallback handling** (consequence of (137)=c, lead decision): the
   daemon reads Pi's `openai-codex` `access` token and `accountId` in memory
   only for that request; it never logs, persists, or forwards it anywhere
   but the ChatGPT usage endpoint, and never refreshes it. When `expires` has
   passed it makes no call and shows the last value with its `asOf` plus a
   status text ("Pi token expired - open Pi to refresh"). The popover names
   the source ("via Codex CLI" / "via Pi"). The exact endpoint, headers and
   response shape are taken from Orca's implementation and the Codex CLI
   source, verified once against the live account.
   Rejected: refreshing Pi's token from the daemon (races Pi's own refresh and
   can invalidate its refresh token).
5. **Query cadence** (lead decision): the Codex/GPT query runs when a viewer
   opens the popover and otherwise at most every 5 minutes while at least one
   dashboard page is connected; results are cached with `asOf`, so several
   viewers share one query. The `codex app-server` process is started per
   query and exits after it (bounded by a short timeout).
   Rejected: a resident app-server process; polling with no viewer connected.
6. **Login detection by file presence and plan field only** ((134)): Claude
   via the credentials file's subscription/plan field, Codex via
   `auth.json`, Pi via `auth.json` `openai-codex` presence; detection itself
   never reads token values (decision 4 is the one token read, for the
   fallback query only). `CLAUDE_CONFIG_DIR` and `CODEX_HOME` are honoured.
   Pi gets no gutter of its own (no plan limits); it only feeds the GPT
   entry. A harness with no detected login is not shown.
7. **Gutter look** ((135)): thin vertical bars next to the harness letter
   icons from the rail redesign (C `#D97757` with 5-hour and weekly bars, X
   `#10A37F` with a weekly bar), bar colour following the band chip
   thresholds (`bandWarnPercent` / `bandCritPercent`). The click opens a
   `railPopOpen` popover: per window percent, reset time, plan, `asOf`,
   source; rows sorted tightest first; status texts when data is missing
   ("not logged in", "usage unavailable", "value from n min ago"); reset
   formats (seconds, milliseconds, ISO) normalized.
8. **The footer link goes; the key stays** ((132)): the `rfoot` link is
   replaced by the gutter; `nav.sessions` remains for `#railbtn`.
9. **Compatibility tier: T0** - dashboard asset content, a daemon-private
   cache file, and a new dashboard route or SSE field (additive). No MCP tool,
   shared store, or tracker event change.

## Constraints

- Never print or log a credential token, including in test output and error
  messages; tests use fake credential files under the test's temp dir and a
  fake usage endpoint / fake `codex` binary, never the owner's real login.
- Never send anything but the token itself to the usage endpoint; no other
  owner data leaves the machine.
- The gutter's access follows the existing dashboard viewer rules; the
  popover shows no account e-mail or id.
- Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`; claude-mod edits (if the feed
  needs a field) follow `ai-docs/manuals/wsflow-mirroring.md`.
- Do not restart or replace the owner's running dashboard daemon for checks.
- Out of scope: a Pi `after_provider_response` header push (possible later
  source of fresher GPT values); account switching.

## Phases

### Phase 1: Usage gutter with Claude push data and Codex/GPT live query

- Daemon: a usage aggregator (Claude windows from hosted sessions and the
  claude-mod feed, stale guard, persisted with `asOf`); a Codex/GPT prober
  (`codex app-server` first, Pi-token `wham/usage` fallback per decisions
  3-5); login detection per decision 6; exposure to the rail via the existing
  page/SSE path or one small read-only route.
- Assets: the rail footer gutter and popover per decisions 7-8.

Verification: Go tests for the stale-window guard, window classification by
duration, reset-format normalization, login detection (with
`CLAUDE_CONFIG_DIR` / `CODEX_HOME`), the fallback order (Codex login present
-> app-server; absent -> Pi token; Pi token expired -> no call + status), and
that no token text appears in logs or errors. A LOOK capture of the rail
footer with a Claude and a Codex gutter and the open popover.
