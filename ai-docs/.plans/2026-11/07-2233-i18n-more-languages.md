# Plan: add major-language dashboard tables

Branch `delegate/i18n-more-languages` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Decision (user, 2026-11-07)

"번역 테이블 추가 자체는 값쌀 거 같은데 몇 가지 메이저 언어들 추가해두죠.
간체 번체 일본어 독일어 프랑스어 뭐 이런 것들 ..."

Add tables for Simplified Chinese, Traditional Chinese, Japanese, German, and
French beside `en.json` and `ko.json` in
`agents-plugin-tool/internal/wsi18n/tables/`. Each table carries every key of
`en.json` (including `lang.name` in its own language, e.g. 简体中文, 繁體中文,
日本語, Deutsch, Français), with the same placeholders and complete plural
pairs, so the existing key tests in `wsi18n/keys_test.go` pass. Translate
naturally for a developer-tool UI; keep protocol vocabulary that the English
and Korean tables leave untranslated (class tags, `ok`/`no`, `(nn)` syntax)
untranslated here too.

## Language item becomes a dropdown (user, 2026-11-07)

"언어 선택 탭도 드롭다운으로 바꾸는 게 나을 거고요". With seven languages
plus "follow the browser", the gear's language item (today a row of choices)
becomes a dropdown (a native `<select>` is the expected default; your call if
the gear menu's existing styling needs otherwise). It applies to every gear
that holds the language item: the full gear on adapter pages and the
language-only gear on the session list and board-first pages. Behavior is
unchanged: choosing a language sets the cookie and reloads (or shows the
held-draft banner), "follow the browser" clears the cookie, and the current
choice is shown as selected. Each language appears under its own `lang.name`.

## Language matching

`Resolve` in `wsi18n/i18n.go` matches Accept-Language on the primary subtag
only, which cannot tell the two Chinese scripts apart. Choose table codes for
the two scripts (BCP 47 `zh-Hans` / `zh-Hant` are expected) and extend the
matching so that `zh-Hans`, `zh-CN`, `zh-SG`, and bare `zh` pick Simplified;
`zh-Hant`, `zh-TW`, `zh-HK`, `zh-MO` pick Traditional; other languages keep
primary-subtag matching. Make sure the cookie value (`Pref`/`Supported`) and
the client-side code in the dashboard and whiteboard scripts accept the new
codes, including any case handling. If the gear lists languages in table
order, keep the order sensible (English first, then the others in a stable
order of your choice).

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/...`,
`go test -count=1 ./internal/wsi18n/ ./internal/wsdashboard/
./internal/wswhiteboard/`; new `Resolve` tests for the Chinese mappings and
one other new language. A headless Chromium check (cached Playwright under
`~/.npm/_npx/*/node_modules/playwright`) of the session list with a zh-TW and
a ja browser locale showing the matching table, the gear's dropdown listing
every language by its own name, and choosing a language from the dropdown on
the session list and on a session page switching and persisting.
`node --check` on changed scripts.

## Return

Changed files, test output, the codes chosen, and whether the browser check
ran.
