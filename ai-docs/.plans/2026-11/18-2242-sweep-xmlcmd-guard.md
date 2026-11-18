# Plan: wb:old / exact-match operand ambiguity guard (v0.50.8 sweep item 12)

## Problem
Commit 55fb1b81a made every non-raw, non-CDATA body segment entity-decoded
(`internal/wsxmlcmd/wsxmlcmd.go`, decode loop near line 1009). Older plugin
manuals told callers that an exact-match operand (whiteboard `wb:edit`'s
`wb:old`, and any other operand matched against stored text) is kept exactly
as written. An old-manual caller sending `<wb:old>&lt;b&gt;</wb:old>` now
matches stored `<b>`. When the stored content holds BOTH spellings (`<b>` and
the literal `&lt;b&gt;`), the wrong occurrence is patched and persisted.

Owner ruling (119)=a accepts old-manual callers on the new daemon as T1
(display-degraded, no data change). This guard keeps the edge inside T1 by
refusing instead of persisting.

## Required behavior
- For an exact-match operand whose decoded text differs from its source text
  (i.e. the non-raw, non-CDATA segments contained entities that Decode
  changed): if the literal source spelling (the operand as written, entities
  undecoded, raw/CDATA segments contributing their text) ALSO occurs in the
  stored text it is matched against, refuse the command with a clear message
  that names the ambiguity and tells the caller to wrap the operand in
  `<wb:raw>...</wb:raw>` (or the tool's raw element) to match literally.
  Nothing is applied (batch validation refuses whole, per existing rules).
- Operand without entities, or whose literal spelling does not occur: behavior
  unchanged.
- Find every exact-match operand site (start from where `wb:old` is consumed
  in `internal/wswhiteboard`; check the tracker store for an equivalent
  exact-match edit operand). Apply the guard at each.
- The source text must reach the matcher: carry it alongside the decoded
  value from wsxmlcmd (e.g. a parallel field on the parsed field/segment),
  with the smallest structural change.

## Out of scope
- Do not change decoding semantics otherwise; do not touch the raw element
  rules; the unclosed literal `<tk:raw>` refusal stays as is.
- No playbook prose edits.

## Verification
- New tests: both spellings present → refused, content unchanged; only the
  decoded spelling present → applied as today; operand wrapped in raw with
  both present → literal occurrence patched.
- `go test ./internal/wsxmlcmd ./internal/wswhiteboard ./internal/wstracker... -count=1`
  (whichever packages you touched) and `go test ./internal/mcp -run 'Whiteboard|Tracker' -count=1`.
- Do NOT commit. Leave changes uncommitted; the lead commits. Other delegates
  edit other files in the same worktree concurrently: touch only files this
  plan needs, and do not run `git checkout`/`git stash`/formatters over the tree.
