# Plan: tracker client strings translated (de, es, fr, it, pt-BR)

Branch `delegate/tracker-i18n-b` (worktree `/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford`), based on develop. Commit here; do not merge or push. Do not run git stash.

## Why

The user reads the dashboard in Korean (and the product ships to every language the tables list). The tracker drawer's ~595 client strings (`js.tk.*`) exist only in `agents-plugin-tool/internal/wsi18n/tables/en.json`; every other table falls back to English, so e.g. the "Add a tracker store" dialog shows English in a Korean page.

## Do

Add translations of every `js.tk.*` key in en.json to these tables only: de, es, fr, it, pt-BR. Another delegate does the other languages in parallel; do not touch any other table, en.json, or any source file.

- Skip `js.tk.reg.handle`: a concurrent change removes it from en.json, and keys_test rejects a non-English key English lacks.
- Read `agents-plugin-tool/internal/wsi18n/i18n.go` and `keys_test.go` first: how plural keys (`.one` / `.other`, and any other CLDR categories the package supports per language) and placeholders work. Keep every placeholder, HTML entity, inline tag, and key-like token exactly; translate only human text. Product and protocol terms stay as English identifiers where en.json treats them as identifiers (member ids, ``, git, SSH, remote URLs).
- Use `agents-plugin-tool/internal/wsi18n/tables/ko.json` (and the language's own existing entries) as the style reference for tone and existing UI terms, so tracker strings match the rest of the dashboard's vocabulary in that language.
- Context for meaning: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` shows where each key is used; the tracker domain is in `ai-docs/ref/tracker-mockup/README.md`.
- Keep each table's existing key order convention (check whether keys are sorted or grouped; insert consistently).
- These are machine drafts, as earlier dashboard strings were; say so in the commit's AI Context.

## Verification

`cd agents-plugin-tool && go test -count=1 ./internal/wsi18n/ ./internal/wsdashboard/ ./internal/wswhiteboard/` and a JSON validity check of each edited table. Also spot-check: for each edited language, the count of `js.tk.` keys equals en.json's count minus one (the skipped key), counting plural keys by the categories that language needs.

## Return

Commit hash(es), per-language key counts, test output, and any key you could not translate faithfully.
