# Plan: wb:close withdraws the topic's still-open items

## Problem

`wb:close` (`agents-plugin-tool/internal/wswhiteboard/store.go` `applyClose`,
~:479) only adds the topic to `ClosedTopics` and records its result. Items in
the topic that are still `open` or `assumed` keep that status, so the page
treats the closed topic as still waiting (`view.go` ~:494-503) and the user
sees a closed topic with live questions. The only way to settle them today is
a `wb:patch status="withdrawn"` per item.

User decision (2026-11-09, whiteboard (35) a): closing a topic withdraws
every item in it that is still `open` or `assumed`. The close result sentence
is the reason. A later reopen of the topic (adding an item or changing its
body) does not restore them: withdrawn items stay withdrawn. Rejected: refuse
`wb:close` while open items remain (an extra round-trip that guards against
almost nothing); a new "closed with topic" status (withdrawn already means
settled without an answer, `render.go` `Void`).

## Change

- `applyClose`: set `Status = withdrawn` on every item of that topic whose
  status is `open` or `assumed`, inside the same validated batch (a refused
  batch changes nothing). Answered, withdrawn, and superseded items are
  untouched. Make sure the change marks/stamps behave like a `wb:patch
  status="withdrawn"` would (settled ordering, the "10 most recently settled
  expanded" rule), and that the stored answer fields of an `assumed` item are
  kept or cleared consistently with how a patch to withdrawn treats them;
  follow the existing patch path rather than inventing a new rule.
- If the update result has a natural place to report it, report which IDs
  the close withdrew (for example an `info:`-style line or the existing
  result fields); if that needs a new result field or protocol change, skip
  it and say so.
- Tests in `store_test.go` (or the nearest close tests): open and assumed
  items become withdrawn on close; answered/superseded stay; a reopen by
  `wb:add` leaves the withdrawn items withdrawn; a refused batch containing a
  close changes no item.
- Playbook text: `agents-plugin/rsrc/whiteboard/whiteboard.md` and its wsflow
  mirror `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` describe
  `wb:close`; add that closing withdraws the topic's open and assumed items
  and that a reopen does not restore them. Read
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` first (both are bound by AGENTS.md's
  Implementation Conventions for these paths) and keep the two files in sync
  as the mirroring manual requires. If an embedded copy of this playbook text
  exists elsewhere (for example a test pinning the text), update it too.

## Verification

- `cd agents-plugin-tool && go test ./internal/wswhiteboard/` green, plus any
  package whose test pins the playbook text (search for a test reading
  `whiteboard.md`), and the wsflow mirror drift tests if they exist.

## Boundary

Work only in this worktree on branch `delegate/develop/wb-close-withdraws-open`.
Commit there (one commit, plan file included); do not merge, do not push,
never run `git stash`. Return the commit SHA, the tests run, and anything
unverified.
