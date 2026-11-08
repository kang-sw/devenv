# Plan: tracker attachment limit checks only newly added attachments

Branch `delegate/tracker-attachment-limit-new-only` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (README is the
data-structure contract) and its Go port `agents-plugin-tool/internal/wstracker`
(golden parity: `testdata/gen-golden.js`, `golden_test.go`, `reasons.json`).
Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`. The previous change (merge `0a80fdbfb`) added mentions,
attachment metadata, and `attachments.max_bytes` with
`roles.set_attachment_limit`.

A parallel delegate restyles the mockup (`style.css`, `index.html`, `app.js`
presentation) on another branch. Keep this change to `engine.js`, fixtures if
needed, README, and the Go port; do not touch `style.css` or app.js layout.

## Decision (user, board (345) b, 2026-11-08)

Today every attachment on a write is checked against `attachments.max_bytes`
at write time, so after an admin lowers the limit a body edit that keeps an
existing larger attachment is flagged (seen with `bo.pc-9` in the previous
delegate's verify2 log). Change it: the limit applies only to attachments
newly added by the write. An attachment whose `oid` is already present on the
same target's current body (for `item.edit`) or the same comment's current
attachments (for `comment.edit`), as resolved in the author's seen state, is
exempt from the size check. `item.create` and `comment.add` check every
attachment, as now. Shape checks (oid format, name, integer size, mime) still
apply to every attachment.

Also confirmed (board (343), (344) ok): a body-less `item.edit` carrying
`mentions` or `attachments` stays flagged; a removed member stays
unmentionable. No change needed for those.

## Work

1. Mockup engine: implement the exemption in the attachment check; keep the
   existing reason text for a newly added over-limit attachment.
2. Go port: the same rule with identical behaviour and reason text.
3. Tests: add `reasons.json` / case coverage for (a) an edit keeping a
   pre-existing attachment above a lowered limit is accepted, (b) an edit
   adding a new over-limit attachment after the limit was lowered is flagged,
   (c) the exemption is resolved at the author's seen state (an oid the author
   had not yet seen on the body is new). Regenerate goldens with
   `testdata/gen-golden.js`.
4. README: state the rule where attachments and `attachments.max_bytes` are
   described, and add Settled rows for (345) b and for (343)/(344) (body-less
   edit with extras flagged; removed members unmentionable) unless already
   stated.

## Verification

- `node --check` on the mockup scripts and `gen-golden.js`; gen-golden reports
  all reasons cases agree with the engine.
- `cd agents-plugin-tool && go build ./... && go vet ./internal/wstracker/ && go test -count=1 ./internal/wstracker/ ./internal/wsledger/`.
- `internal/wsledger` untouched.

## Return

Commits, changed files, test output summary, and anything unsettled (stop and
report rather than inventing a rule beyond this decision).
