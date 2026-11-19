# Plan: whiteboard.load associations, publication token, memo leak (v0.50.8 sweep items 13, 15, 16)

Read ai-docs/manuals/performance.md and release-compatibility.md first.

## Item 13 - internal/mcp/whiteboard_tools.go (~line 479)
After `whiteboard.load` imports a board, a destination item ID (say `1`) that
had a held or drained dashboard answer keeps that association; the imported,
unrelated item `1` inherits its sent mark, and the acknowledgement checks tell
the lead to record the old approval against the imported question. Fix: add a
board-incarnation boundary so held/drained answers and sent marks recorded
against the previous board do not attach to items of a loaded board (e.g. a
board incarnation/generation token captured with each association, bumped by
load; or clear those associations on load if that is what the data model
allows without losing anything the user still needs). Test: answer item 1 on
board A (held/drained), load an archive whose item 1 differs, assert the new
item 1 shows no sent mark and the ack check does not ask to record the old answer.

## Item 15 - internal/wswhiteboard/folders_archive.go (~line 153)
With destination Seq=10 and imported Seq=9, the published Seq stays 10, so an
already-open static index.html sees the same stamp and keeps showing the
replaced board. Fix: the publication token after a load must differ from any
token published before it, not derive from the restored counters. Test it.

## Item 16 - internal/wswhiteboard/folders_archive.go (~line 146)
Validating an archive opens a temp extraction as a board, which fills the
global folderMemo.logs; removing the temp dir does not free those entries, so
repeated imports pile up payload copies in daemon memory. Fix: forget the temp
board's memo entries on every exit path (success, validation error, panic-free
early returns). Test that memo size returns to baseline after a load and after
a refused load.

Note: line ~204 (`extractEntry`) was just changed by the lead to
`wsstate.OpenFile`, and `folders_archive_test.go` leftover-folder assertions
were relaxed; keep both.

## Verification
- `go test ./internal/wswhiteboard -count=1` and
  `go test ./internal/mcp -run 'Whiteboard|Load|Archive|Answer' -count=1`
- Do NOT commit. Leave changes uncommitted; the lead commits. Other delegates
  edit other files in the same worktree concurrently: touch only files this
  plan needs; no `git checkout`/`git stash`/tree-wide formatters.
