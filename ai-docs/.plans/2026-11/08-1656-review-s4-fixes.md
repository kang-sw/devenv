# Plan: review sweep S4 fixes

Branch `delegate/review-s4-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push.

Source: review sweep segment S4 (`895c62c90..393650fea`, board (446) b).
Full finding: `/tmp/review-sweep/S4-correctness.md` (Important section). Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`, and `ai-docs/manuals/ws-mcp.md` for `internal/mcp/`.
Authority for the change marks: plan `ai-docs/.plans/2026-11/08-1258-topic-body-change-bars.md`
(Decision 2: "first body marks nothing").

## Fix

1. **Important - topic-body change marks are per command, not per
   `whiteboard.update` batch.** `internal/wswhiteboard/store.go` ~L282-284
   (`setTopicBody`): every `wb:topic` / `wb:edit` diffs against the body the
   previous command in the same batch left and replaces `Marks`, so several
   `wb:edit`s on one topic in one update keep only the last edit's marks, and
   a body created and edited in the same batch gets marks although it had no
   body before the batch. Fix: when the topic was already changed in this
   batch (`Topics[i].Changed == mark`), diff against the body as it stood at
   batch start (`Apply` still holds the untouched state; `work` is a deep
   clone), and mark nothing when the topic had no body at batch start.
   Tests (fail before the fix): two `wb:edit`s on different blocks of one
   topic in one batch mark both blocks; `wb:topic` creating a body plus a
   `wb:edit` on it in one batch marks nothing; an edit that reverts an
   earlier same-batch edit leaves no mark for that block.

## Out of scope

Every Minor in the S4 reports (the lead records them as follow-ups),
including the `<p>` wrapper layout shift on single-paragraph marked bodies,
the dev-assets entry cache, the `ToUpper` parity edge, and `alignGap` cost.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wswhiteboard/ ./internal/mcp/ && go test -count=1 ./internal/wswhiteboard/ && go test -count=1 -run 'Whiteboard' ./internal/mcp/`

## Return

Commit hashes, changed files, test output (and which new tests failed before
the fix), and anything left open.
