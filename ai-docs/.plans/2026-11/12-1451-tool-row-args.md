# Finished tool-call rows show a short args preview; their numbers drop labels

## Goal

User request 2026-11-12 (board, screenshot of the dashboard conversation
view's expanded tool summary: rows like `✓ Skill  0.6s · 프롬프트 80.2k +53.2k · 출력 134`):

1. Show each finished call's arguments briefly on its row. The summary
   originally carried no argument content on purpose
   (`agents-plugin-tool/internal/wslive/toolsummary.go` header: "No argument
   or result content enters it"; `BatchCall` in `wslive/snapshot.go` says the
   same). The user reverses that: the running-call line and the permission
   line already show `argsPreview` live (capped at `ARGS_PREVIEW_CAP` = 200
   in `agents-plugin/hooks/claude-mod/dashboard.ts`), so the only new
   exposure is retention.
2. On each per-call (batch) row, show only the numbers, and explain each on
   hover.

## Decisions (board items (414)-(419), all confirmed)

- (414)=a: the args preview is persisted with the summary: it travels the
  adapter's per-call `calls` wire entries -> `wslive.BatchCall` -> the
  summary batch -> the ring's tools entry, so it survives reload and
  reconnect. Same 200-char cap as the live preview (the adapter already
  computes `argsPreview` per call in `toolBegan`/`stepEnded`). Rejected:
  live-only (the row's args would vanish the moment the gap moves to the
  ring).
- (415): a batch with one call shows that call's args right after the tool
  name. A batch with several calls keeps today's name chips on the row and
  lists every call's `name args` in the row's tooltip.
- (416): row layout: name fixed, numbers fixed at the right, args take the
  remaining width and are the first thing truncated with an ellipsis
  (flex + `min-width: 0` + `text-overflow: ellipsis`); the full preview is
  the args span's tooltip.
- (417): the per-call row numbers render without labels:
  `0.6s · 80.2k +53.2k · 134` (duration · prompt incl. cache write, `+N` the
  cache-write part · output). Each number is its own span with its own
  tooltip naming it and giving the exact value (reuse the existing
  `tok.title*` / i18n vocabulary; new keys in every locale table like
  neighbouring keys). Only the per-call (batch) rows change; the collapsed
  summary header line (`호출 4 · …`) keeps its current labelled text.
- (418): `wslive` and `wsagent` belong to the peer lead; the lead mails it
  before merging. Every new field is optional: an adapter that sends no
  args (Pi, hosted, older claude-mod) leaves the args slot empty and
  nothing else changes. Compatibility tier T0 (additive optional fields on
  the intake wire, the snapshot-internal batch, and the ring entry).
- Update the "no argument content" comments at both wslive sites to the new
  contract (args preview, capped, no results).

## Scope

- `agents-plugin/hooks/claude-mod/dashboard.ts` (send the per-call
  `argsPreview` in the finished batch's `calls`), and its mirror under
  `agents-plugin-wsflow/hooks/claude-mod/` (read
  `ai-docs/manuals/wsflow-mirroring.md`); their TS tests.
- `agents-plugin-tool/internal/wslive/` (intake decode, `BatchCall`,
  `ToolBatch`/ring entry field), its tests.
- `agents-plugin-tool/internal/wsdashboard/` (`pages.go` `batchMeta` /
  `tokenText` usage for batch rows, the template that renders batch rows,
  `dashboard.css`), `internal/wsi18n/tables/*.json`, tests.
- Read `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing.

## Verification

- Go tests: wslive carries args from intake to the ring entry and caps
  nothing beyond the adapter's preview; a batch without args renders as
  before; the batch row renders the unlabelled numbers each with its title,
  single-call args inline with a title, multi-call args in the row title.
- TS tests for the adapter's `calls` payload (bun/node as the package tests
  run today).
- Heavy Go runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wslive/... ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  expanded summary with short and long args at a narrow and a wide width,
  light and dark, hovering a number; screenshots into
  scratchpad/tool-row-args/.

## Constraints

- Preserve the summary's exact per-tool counts, the batch cap / omitted
  count, and the ring's existing entries (older entries without args render
  unchanged).
- Commit on branch `delegate/tool-row-args` in this worktree; do not merge
  or push.
