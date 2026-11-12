# Dashboard conversation view: clearer token line wording

## Goal

User feedback 2026-11-12 (board topic `token-line`, (394) b plus additions,
(395) technical reading): the tool summary line in the conversation view
reads `입력 14 · 출력 1.4k · 캐시 709k · 쓰기 11.7k`, which is awkward: "입력
14" reads like a count, and "쓰기" is unclear. Mockup:
/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/token-line/index.html
(option B).

## Decisions (settled; do not reopen)

- New form: `호출 N · 프롬프트 709k +11.7k · 출력 1.4k`
  - `호출 N`: the number of model requests the line covers (each tool batch
    in a ToolSummary is one model request; find the count from the summary's
    batches - `wslive.ToolBatch` / the ToolSummary type - and do not count
    tool calls). Omit `호출` on a single batch row (one request).
  - `프롬프트`: input + cacheRead + cacheWrite (the whole prompt the model
    read).
  - `+N`: the cacheWrite part, shown only when > 0, as `+11.7k` right after
    the prompt number (no word).
  - `출력`: model output tokens.
- Tooltip (`tokenTitle`): spell out every part with exact counts, e.g.
  "모델 호출 6회 · 프롬프트 720,512 (입력 14 · 캐시 읽기 708,800 · 캐시 쓰기
  11,698) · 출력 1,402"; the `+N` is named "캐시 쓰기" there.
- Batch rows (`batchMeta`) use the same form (without the 호출 count, see
  above).
- Every locale table: replace the `tok.*` keys' wording
  (`agents-plugin-tool/internal/wsi18n/tables/*.json`, ~1388-1392 in ko.json)
  in all 11 tables with natural terms per language (English e.g.
  `6 calls · prompt 709k +11.7k · output 1.4k`); remove keys that are no
  longer used, add new ones as needed, keeping the tables' key sets
  identical (the i18n tests check this).
- Code sites: `agents-plugin-tool/internal/wsdashboard/pages.go`
  `tokenText` / `tokenTitle` / `batchMeta` (~410-470) and the `toolsum`
  template in `assets/dashboard.tmpl` (~208-220), plus any JS that re-renders
  the same line live (search dashboard.js for the tok keys / token text).
- Do not change `internal/wslive` (owned by a peer lead). If the request
  count cannot be derived in wsdashboard from what wslive already exposes,
  stop and report rather than editing wslive.
- Compatibility tier T0: dashboard rendering and i18n strings only.

## Scope and verification

- Go tests for tokenText / tokenTitle / batchMeta (existing pages tests):
  the new form with and without cache writes, the call count, single batch
  rows, nil usage.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad above): a rendered conversation tools line and an expanded
  batch list, light and dark, Korean and English; shots into
  scratchpad/token-line-impl/.

## Constraints

- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/token-line-wording` in this worktree; do not
  merge or push.
