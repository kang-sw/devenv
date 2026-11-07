---
title: "Dashboard UI language setting with per-language translation tables"
related:
  261107-epic-ws-personal-common-store: the tracker's team view is the reason; its UI uses the tables from the start
  261107-epic-ws-dashboard-messenger-round2: the dashboard whose static text moves into the tables
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: bf17251f214bce8f
sage-review-completeness-reviewed: bf17251f214bce8f
---

# Dashboard UI language setting with per-language translation tables

## Background

The user (2026-11-07) expects the tracker's team view to need multiple
languages, since a team store is shared by up to 50-60 people: "이 팀 뷰 기능
들어가면서 다국어지원이 필요해질 거 같아요. 톱니바퀴 쪽에 언어 지원 집어넣는
방향으로 가죠. 프론트엔드에 정적 텍스트는 언어별 번역 테이블 같이 넣어두는
식으로요". Today every dashboard string is English, hard-coded in the Go
templates (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`,
`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`) and in the page
scripts (`dashboard.js`, and the whiteboard page scripts under
`agents-plugin-tool/internal/wswhiteboard/assets/`).

## Decisions

- **A language item in the gear menu; static UI text comes from
  per-language translation tables** (user, board (219) ok). Translated:
  text the dashboard itself draws (buttons, labels, statuses, hints,
  tooltips, empty states). Not translated: board items, chat, and any
  agent- or person-authored content. The setting sits in the existing gear
  menu (Layout, Input mode) and is saved in the browser like those.
- **One browser-saved setting applies to every page** (board (220) ok): the
  session list, session pages, and later the tracker. The session list page
  has no gear today, so it gains one holding only the language item.
  Board-first session pages, which render no gear today (the gear sits inside
  `{{if .Adapter}}` in `dashboard.tmpl`), get the same language-only gear
  (lead, auto-confirmed from the fact populator's gap). Rejected: the setting
  on session pages only, which leaves the list page in another language;
  leaving board-first pages on the browser default, which breaks "every
  page".
- **One key-to-text table file per language** (location per Constraints)
  (board (221) ok). A key missing from a non-English table falls back to
  English; a key used by a template or script but missing from the English
  table fails a test. Rejected: showing the raw key on screen when missing.
- **Tables are embedded in the binary; a language is added in source**
  (board (224) a). A contributor adds a language by adding one table file to
  the source tree and rebuilding; the gear discovers the available languages
  from the embedded tables with no code change, so each language's display
  name in the gear also comes from its table file. Rejected: also reading
  translation files at runtime from a user config location, which would need a
  location and a precedence rule across the projects one daemon serves.
- **English and Korean first; the default follows the browser language,
  English when the browser language is unsupported** (board (222) a).
  Rejected: always defaulting to English; English-only tables first with
  Korean later.
- **A standalone ticket ahead of the tracker UI** (board (223) a): the
  existing dashboard text moves into the tables now, and the tracker UI uses
  them from its first child. Rejected: recording it only as a tracker epic
  decision and doing it with the tracker UI.

## Constraints

- Shipped surface: the dashboard ships downstream; nothing in the tables or
  the mechanism may depend on this repository
  (`ai-docs/manuals/shipped-surface-boundary.md`).
- The whiteboard's disk embed and full-page outputs render outside the
  dashboard too; whether they follow the browser setting or stay English is
  the implementer's call, recorded in the Result.
- Machine-facing strings (MCP tool output, refusal messages, agent-read
  text) are out of scope; only what a person reads in the browser. Protocol
  text drawn in the browser stays untranslated too: the reply box that
  `form.js` composes (`(nn) a; (mm) ok`), answer values (`ok`, `no`, option
  letters), and held-message chips. Whether class tags (`policy`,
  `technical`, `assumed`) translate is the implementer's call, recorded in
  the Result.
- "Text the dashboard itself draws" includes Go-built display strings, not
  only templates and scripts: for example `batchMeta`, `tokenText`,
  `tokenTitle` (`wsdashboard/pages.go`), the band flags in
  `wsdashboard/pageview_band.go`, and the panel titles in
  `internal/mcp/whiteboard_tools.go`.
- Table location is not limited to `wsdashboard`: `wswhiteboard` cannot
  import `wsdashboard` (the reverse import exists), and the board regions
  render through `PageSource.ReadPage` (`wsdashboard/page.go`), which has no
  language input. A small shared table package or an injected lookup, with
  the language threaded through, is expected.
- If translation happens on the server, shared state must become
  language-aware: `entryCacheKey` (`wsdashboard/entry_cache.go`) has no
  language, so viewers in two languages would get each other's cached
  entries; templates are parsed once with a static FuncMap
  (`wsdashboard/pages.go`); a language switch must force a re-render the way
  the `mode.` pseudo-hash does in `dashboard.js`.
- The pages carry no `lang` attribute (261030-feat-session-whiteboard-mode).
  Setting `<html lang>` to the chosen language is allowed only if the Result
  names that supersession; otherwise keep it absent.
- Runs in parallel with `261107-feat-ws-whiteboard-xml-commands-topic-content`
  (user, 2026-11-07: "다국어 티켓은 병렬 워크트리로 바로 띄울 수 있지
  않나요?"), superseding the run-after note in a970821f. That ticket edits
  the same templates and renames `now-frame.js`; template conflicts are
  resolved at merge, and strings that ticket adds are keyed by whichever
  lands second: when it lands first, this ticket's worker sweeps its template
  and script additions by hand, since the key test cannot see unkeyed
  literals. This supersedes the run-after note in the Prior Decisions
  below.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- a970821f (2026-11-07, commit): "User 2026-11-07: tracker team view will need multiple languages; language item in the gear menu, static frontend text from per-language tables. Board (219)-(221) ok, (222) a en+ko ... (223) a standalone ticket before the tracker UI." — bearing: supports
- a970821f (2026-11-07, commit): "Left in todo: it touches the same dashboard templates as the in-flight whiteboard XML ticket and the top-bar trim, so it is promoted and run after those land." — bearing: constrains
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "Symbol-first chrome (technical decision (79)). Template chrome carries no prose. English appears only in class tags, IDs, tooltips, and the panel names (Todos, Agendas); the page has no `lang` attribute." — bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, commit 75ff0c2b): "Region diffing by content hash: each region element carries data-h; the client names held hashes in the stream URL, so a fresh stream or a reconnect sends exactly what changed." — bearing: constrains
- f594e798 (2026-11-07, commit): "Saved share is the tree's fraction (ws.dashboard.split); dblclick resets, arrow keys nudge. Input mode key: ws.dashboard.input (enter|mod)." — bearing: constrains
- 261107-feat-ws-dashboard-pull-down-drawer (2026-11-07, Result e12879e10): "Escape yields to the link and gear menus and to a focused composer, and the link menu counts as inside." — bearing: constrains
- 261107-feat-ws-dashboard-link-actions (2026-11-07, commit 0336011f): "internal/mcp/dashboard_page.go is touched to wire FileActions into the served board; the ticket named only wsdashboard and wswhiteboard but the served board is built there." — bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Result 2f1ea3cc3): "The board-first page shows a single column with no gear." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/assets/dashboard.tmpl, dashboard.js, wswhiteboard/assets/page.tmpl and its page scripts, plus new per-language table files |
| scope.surface | cross-module | wsdashboard and wswhiteboard assets share one setting; internal/mcp/dashboard_page.go renders the served board |
| scope.new_public_symbol | unknown | lookup and table-loading API shape is the implementer's call; none named by the ticket |
| scope.new_type_contract | unknown | table format and how the language reaches templates are left to the implementer |
| scope.test_surface | new-files | search for i18n or translation tests under internal/wsdashboard and internal/wswhiteboard on develop found none; existing pages_test.go and render_test.go assert English strings and will need updates |
| complexity.reuse_points | unconfirmed | gear menu (dashboard.tmpl line 8, dashboard.js localStorage keys ws.dashboard.*) is the host for the item, but the lookup mechanism does not exist yet |
| complexity.side_effect_risk | moderate | touches every user-visible string in two embedded asset sets, and region content hashes (data-h) must stay stable per language |
| risk.correctness | moderate | missing-key fallback, no flash of the other language on load, and per-request language on server-rendered regions |
| risk.fit | moderate | the gear renders only when an adapter exists (board-first pages have none), and two embeds with separate scripts must share one setting |
| risk.test | moderate | the no-flash and cross-page persistence requirements need a headless browser check that is not automated |
| risk.security_or_contract | moderate | a browser-saved value (cookie or storage) may reach server-rendered templates; shipped-surface boundary applies |

## Phases

### Phase 1: translation tables, language setting, existing text migrated

- An inventory of every user-visible static string in the dashboard and
  whiteboard page templates and scripts, moved behind keys.
- `en` and `ko` tables; the lookup and fallback; the gear-menu language
  item on session pages, a language-only gear on the session list page and
  on board-first session pages; browser
  default detection. The language item offers an explicit "follow the
  browser" choice beside the languages, so a user can return to the default.
- How the language reaches server-rendered templates (cookie read by the
  server, client-side replacement, or both) is the implementer's call; a
  visible flash of the other language on load is not acceptable.
- Tests: the English table covers every key used; a non-English missing key
  falls back; the setting survives reload and applies across pages.

Verification, from `agents-plugin-tool/`: `go build ./...`,
`go vet ./internal/... ./cmd/...`, `go test -count=1 ./internal/wsdashboard/
./internal/wswhiteboard/`, plus `./internal/mcp/` if it renders dashboard
pages; `node --check` on changed scripts; a headless Chromium check (cached
Playwright under `~/.npm/_npx/*/node_modules/playwright`) of the session
list, an adapter session page, and a board-first session page in both
languages.
