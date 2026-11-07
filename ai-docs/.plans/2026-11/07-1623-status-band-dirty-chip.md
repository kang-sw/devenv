# Plan: status band - working-tree change chip

## Outcome

The dashboard session status band (261107-feat-ws-dashboard-status-band,
merged f1c811464) shows folder, branch, and ahead/behind, but not whether the
working tree has changes. The user reversed the earlier "no `git status`"
call (whiteboard (97) a, 2026-11-07): add a working-tree chip.

- ws-mcp's cached per-session git read (the `repo` snapshot field built with
  `wsgit.BranchSync`, refreshed at most once per heartbeat, outside
  `writeMu`, 5 s timeout) also runs `git status --porcelain` including
  untracked files, and records counts of modified (unstaged), staged, and
  untracked entries. Keep it inside the same once-per-heartbeat read and
  the same timeout; one git invocation that yields branch, sync, and counts
  together (`git status --porcelain=v2 --branch`) is acceptable if it
  replaces the separate calls cleanly, otherwise add one call.
- The page shows a compact chip on the status line next to branch/sync when
  any count is non-zero (e.g. `●3 +1 ?2` style, your choice of glyphs within
  the existing chip styling, with a title tooltip spelling out the counts);
  a clean tree shows no chip (or a subtle clean mark, if the mockup
  `ai-docs/ref/whiteboard-mockup/r5-status-band/index.html` draws one -
  follow the mockup's dirty chip if present).
- If the status read times out or fails, the chip is hidden; branch and sync
  still show from whatever succeeded (if a single combined call fails, the
  previous cached values may be kept until the next heartbeat).
- Non-repository root: unchanged (folder only).

## Settled constraints

- No adapter change; no new event kind; the new fields are `omitempty` on the
  existing `repo` snapshot value (`internal/wslive`), so old pages and old
  readers are unaffected.
- Update the ticket's archived decision? No: the ticket is closed; record the
  reversal in the commit `## AI Context` (user (97) a) instead. Update
  `ai-docs/manuals/ws-mcp.md` where it describes the repo fields.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`);
  `internal/mcp/` edits read `ai-docs/manuals/ws-mcp.md`.

## Boundary

Commit on branch `delegate/status-band-dirty-chip` only; do not merge or
push. Return if this needs an adapter or protocol change.

## Verification

In agents-plugin-tool: `gofmt -l ./internal ./cmd` empty;
`go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wsgit/ ./internal/wslive/ ./internal/wsdashboard/ ./internal/mcp/`.
Tests: counts on a real temp repo with modified, staged, and untracked files;
clean repo shows no chip; a failing/timeout read hides the chip; still one
git read per heartbeat. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright): chip renders in light and dark and
does not break the narrow (360px) status line. Report what was not checked.
