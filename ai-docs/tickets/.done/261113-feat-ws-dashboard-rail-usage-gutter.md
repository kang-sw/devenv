---
title: "Dashboard rail: per-harness plan-usage gutter in the bottom-left corner"
related:
  261113-feat-ws-dashboard-harness-auth-slots: follow-up (account switching, Orca config-home model)
  261111-feat-ws-dashboard-hosted-usage-limit-continue: neighbour (consumes the same Claude limit data)
  261028-research-claude-mod-statusline-band: neighbour (claude-mod statusline rate-limit figures)
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: ffb1e7f8ab55eb84
sage-review-design-reviewed: ffb1e7f8ab55eb84
completed: 2026-11-13
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
  (`dashboard.css` ~1611). The same `nav.sessions` key labels `#railbtn`
  (`dashboard.tmpl` ~10), which already links to the session list.
- Claude limits already reach the daemon per hosted session:
  `wsagent/claude.go` (~895-903) parses `rate_limit_event`
  (`unifiedWindows` five_hour / seven_day utilization and resetsAt);
  `wslive/snapshot.go` (~190-201) `Usage.Limits`; `pageview_band.go`
  (~98-175) renders the per-session limit chips with
  `bandWarnPercent` / `bandCritPercent`. The claude-mod feed carries the
  statusline rate-limit figures (`agents-plugin/hooks/claude-mod/dashboard.ts`
  ~640-663 `statusFigures` (agents-plugin/hooks/claude-mod/dashboard.ts#L640-L663), mirrored
  byte-identically in `agents-plugin-wsflow`).
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
   credential read for Claude. Phase 1 assumes one Claude account per
   machine: windows are aggregated account-blind (multi-account keying is
   left to `261113-feat-ws-dashboard-harness-auth-slots`; the cache is
   daemon-private, so reshaping it later stays T0).
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
   secondary position. "Codex CLI login detected" means `auth.json` holds a
   ChatGPT login (not an API-key-only `auth_mode`); an API-key-only login or
   a missing `auth.json` (including credentials kept in the OS keyring)
   counts as no login, so the Pi fallback applies. When a ChatGPT login is
   detected but the app-server query fails (no `codex` on PATH, timeout,
   error, malformed reply), there is no Pi fallback: the entry shows "usage
   unavailable" with the last value and its `asOf` (literal reading of
   (137)=c).
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
   source; tests use a fake endpoint. The worker may run the real
   `codex app-server` query once (the Codex CLI holds the credentials) and
   report only window percentages and field names; the live Pi-token path is
   confirmed by the owner on the dashboard after merge, not by the worker.
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
   entry. A harness with no detected login is not shown, except that the
   Claude entry is also shown whenever the aggregator holds pushed Claude
   limit data (a host may keep Claude credentials outside a file, e.g. a
   keychain); its plan field is then blank. Reading a keychain is out of
   scope (decision 1).
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
10. **Additive assets** (lead decision, following fd4fc2be): the gutter and
    popover controller and styles live in new asset files (wired from
    `dashboard.tmpl`), with only the `rfoot` swap and the minimal hooks in
    shared `dashboard.tmpl` / `dashboard.js` / `dashboard.css`, to keep
    parallel dashboard merges cheap.
    Rejected: inlining the controller into `dashboard.js`.

## Constraints

- Never print or log a credential token, including in test output and error
  messages; tests use fake credential files under the test's temp dir and a
  fake usage endpoint / fake `codex` binary, never the owner's real login.
- Never send anything but the token and its account id (as the endpoint's
  headers require) to the usage endpoint; no other owner data leaves the
  machine.
- The gutter's access follows the existing dashboard viewer rules; the
  popover shows no account e-mail or id.
- Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`; claude-mod edits (if the feed
  needs a field) follow `ai-docs/manuals/wsflow-mirroring.md`.
- Do not restart or replace the owner's running dashboard daemon for checks.
- Out of scope: a Pi `after_provider_response` header push (possible later
  source of fresher GPT values); account switching.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- ef5e84ab (2026-11-13, commit): "Owner answers on whiteboard topic usage-gutter: (130)=a push-only Claude, (131)=a display first + separate slot ticket, (132)/(134)/(135) ok, (133)=b live Codex query, (137)=c Codex CLI app-server with Pi stored-token wham/usage fallback." — bearing: supports
- 77505f3f (2026-11-13, commit): "Plan: ai-docs/.plans/2026-11/13-1609-rail-harness-icon.md (frozen)." (the rail harness letter icons that decision 7's bars sit beside) — bearing: supports
- 261107-feat-ws-dashboard-status-band (2026-11-07, Decisions): "context and usage warn at 80%+ and turn critical at 95%+; ... a host without usage-limit data (Pi) omits the limit chips." — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (d059e16e, 2026-11-11, commit): "The session menu is a rail popover (a fourth kind) rather than the absolute head menu: the head region is swapped under it, and the popover code already re-anchors, closes on Escape and outside clicks" — bearing: supports
- 261110-feat-ws-dashboard-headless-lead-sessions (e725d4ec, 2026-11-11, commit): "Reset time uses a new "dt" time format (local month, day, and time), since a weekly limit's reset can be days away and HH:MM alone misleads." — bearing: supports
- 261110-feat-ws-dashboard-headless-lead-sessions (a27d1bf6, 2026-11-11, commit): "Typed-failure shapes follow Orca's Claude adapter (error field, api_error usage_limit_reached, rate_limit_info rejected/allowed and overage)" — bearing: supports
- 261112-feat-ws-dashboard-rail-memo-pad (fd4fc2be, 2026-11-13, commit): "Implement the owner's approved Phase 1 without refactoring shared dashboard code; keep the controller and styles in additive assets for parallel merges." — bearing: constrains
- 261112-feat-ws-dashboard-rail-memo-pad (1073074d, 2026-11-13, commit): "Overlaps pending impl/develop/search-palette in dashboard.js and dashboard.tmpl; that branch re-checks against this merge before it lands." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/assets/dashboard.tmpl, dashboard.css, dashboard.js; wsagent/claude.go; wslive/snapshot.go; pageview_band.go; claude-mod dashboard.ts and its wsflow mirror; new aggregator, prober, route files |
| scope.surface | cross-module | spans wsagent, wslive, wsdashboard, and the claude-mod hook feed; T0 additive route or SSE field |
| scope.new_public_symbol | yes | a usage aggregator, a Codex/GPT prober, and a read-only route or SSE field (names not fixed by the ticket) |
| scope.new_type_contract | yes | the persisted usage cache with asOf and the rail-facing usage payload (shape not fixed by the ticket) |
| scope.test_surface | new-files | existing wsdashboard/pageview_band_test.go and rail_test.go cover neighbours; the aggregator, prober, and login-detection tests are new |
| complexity.reuse_points | confirmed | bandWarnPercent and bandCritPercent (pageview_band.go#L23-L24), railPopOpen (dashboard.js#L5970), wslive.Usage.Limits (wslive/snapshot.go), rate_limit_event parse (wsagent/claude.go#L895-L903), .hi.claude and .hi.codex icons (dashboard.css#L1588-L1590) |
| complexity.side_effect_risk | high | spawns a codex app-server subprocess, reads credential files, sends a token to an outbound HTTP endpoint, writes a persisted cache |
| risk.correctness | moderate | stale-window guard, window classification by duration, and the fallback order are logic-heavy; the wham/usage endpoint and response shape are unverified against the live account |
| risk.fit | moderate | the rail footer and popover are shared with pending branches that also edit dashboard.tmpl and dashboard.js (see Prior Decisions); Pi and Codex account identity is unverified |
| risk.test | moderate | needs a fake codex binary, a fake usage endpoint, and a fake credential tree; the LOOK capture is manual |
| risk.security_or_contract | high | reads Pi's access token and accountId and sends the token off-machine; must never reach logs, errors, or test output |

## Phases

### Phase 1: Usage gutter with Claude push data and Codex/GPT live query

- Daemon: a usage aggregator (Claude windows from hosted sessions and the
  claude-mod feed, stale guard, persisted with `asOf`); a Codex/GPT prober
  (`codex app-server` first, Pi-token `wham/usage` fallback per decisions
  3-5); login detection per decision 6; exposure to the rail via the existing
  page/SSE path or one small read-only route.
- Assets: the rail footer gutter and popover per decisions 7-8, placed in
  additive asset files per decision 10.

Verification: Go tests for the stale-window guard, window classification by
duration, reset-format normalization, login detection (with
`CLAUDE_CONFIG_DIR` / `CODEX_HOME`; Claude shown from pushed data without a
credentials file), the fallback order (Codex ChatGPT login present ->
app-server; app-server failure -> "usage unavailable", no Pi call; absent or
API-key-only -> Pi token; Pi token expired -> no call + status), and
that no token text appears in logs or errors. A LOOK capture of the rail
footer with a Claude and a Codex gutter and the open popover.

### Result (f9b3c7924) - 2026-11-13

- Added the daemon-private plan usage cache and viewer-authorized `GET
  /rail/usage`. Existing hosted Claude snapshots and claude-mod snapshots feed
  the same persisted, account-blind stale-window guard; no hook/feed change
  was needed. Expired windows are omitted and heartbeat-only repeats do not
  refresh `asOf`.
- Added a bounded, short-lived Codex app-server handshake and duration-based
  weekly classification. Only absent/API-key CLI logins select Pi fallback;
  CLI errors never fall through. Pi expiry makes no request; upstream errors
  are sanitized, redirects refused, credentials never enter cache/view data.
  Endpoint/header and response contracts were checked against Orca's Codex
  backend source and OpenAI's generated rate-limit protocol types. No live
  Pi-token request or real credential-tree test was performed.
- Added `usage.js` / `usage.css`, replaced only the rail footer link, and
  connected the existing popover lifecycle with minimal shared-file hooks.
  Details sort tightest first and show plan, window percentage/reset, age,
  source, and missing/error status. Viewer requests share the daemon cache;
  visible connected pages poll every five minutes, popover opens request a
  refresh, and concurrent opens coalesce for five seconds. No resident
  app-server or background poller was added.
- Integrated develop's lead-wake-events work before verification (merge
  `1c97546ee`).
  The final `git -c merge.directoryRenames=false merge-tree --write-tree
  develop HEAD` against develop `e6102aea7` was clean, with no newer shared
  dashboard asset changes since the integration.
- Verification: remote w5090 full `go test ./internal/wsdashboard/
  ./internal/wslive/ ./internal/wsagent/` passed after fixes (wsdashboard
  86.056s; wslive cached; wsagent 23.294s). Windows wsdashboard test binary
  cross-compilation passed. Local locked Node checks passed: dashboard.js
  syntax plus all four usage/task-badge tests.
- Independent medium R1 found two Important test-contract gaps. Fix
  `1baf6c3e6` replaced the Unix-only canned shell fake with a copied,
  cross-platform test executable validating the full request sequence,
  added negative protocol tests and Windows batch-shim coverage, and fixed
  Windows cmd.exe quoting. Fixes-only R2 was clean. Reviews ran through
  standalone Pi CLI because `pi.*` calls remained profile-denied; no review
  was performed by the implementer.
- LOOK passed using a static fake-data fixture and headless Chrome:
  `D:/Temp/rail-usage-gutter/footer.png` and
  `D:/Temp/rail-usage-gutter/popover.png`. Both captures were inspected; no
  live dashboard daemon was started, restarted, or replaced.
- Forward: Windows runtime execution is not claimed by the remote Linux
  check (the Windows test binary compiled); live Pi fallback confirmation
  remains the owner's post-merge check, per decision 4.
