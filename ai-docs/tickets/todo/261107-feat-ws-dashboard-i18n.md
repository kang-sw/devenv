---
title: "Dashboard UI language setting with per-language translation tables"
related:
  261107-epic-ws-personal-common-store: the tracker's team view is the reason; its UI uses the tables from the start
  261107-epic-ws-dashboard-messenger-round2: the dashboard whose static text moves into the tables
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
scripts (`dashboard.js`).

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
  Rejected: the setting on session pages only, which leaves the list page in
  another language.
- **One key-to-text table file per language in the dashboard assets**
  (board (221) ok). A key missing from a non-English table falls back to
  English; a key used by a template or script but missing from the English
  table fails a test. Rejected: showing the raw key on screen when missing.
  A downstream project adds a language by adding one file.
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
  text) are out of scope; only what a person reads in the browser.

## Phases

### Phase 1: translation tables, language setting, existing text migrated

- An inventory of every user-visible static string in the dashboard and
  whiteboard page templates and scripts, moved behind keys.
- `en` and `ko` tables; the lookup and fallback; the gear-menu language
  item on session pages and a new gear on the session list page; browser
  default detection.
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
list and a session page in both languages.
