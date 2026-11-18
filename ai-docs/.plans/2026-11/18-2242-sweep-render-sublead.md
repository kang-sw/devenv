# Plan: legacy lead-ticket auto-ack transform + sub-lead marker cache (v0.50.8 sweep items 14, 17)

## Item 14 - internal/mcp/playbook_tools.go (~line 770)
A 0.50.7 plugin's `lead-ticket` resources carry no `ws:override:TicketHandler*`
markers but still contain the auto-confirm text. The renderer now applies the
`ticket_review_auto_ack` knob only through TicketHandler* markers (commit
e6e1efd83 "resolve ticket knobs through override markers", d25bdfcc4), so with
an old plugin connected to the new daemon, `ticket_review_auto_ack: off` is
silently lost. Fix: when the resource being rendered has no such markers, keep
the legacy text transform (git history of `ticket_review_text.go` /
`ticketReviewAutoAckVariants` before those commits shows it) so old resources
still honor the knob. New marker-bearing resources keep the marker path.
Test: render a fixture lead-ticket resource WITHOUT markers (copy the 0.50.7
text from `git show 2d302b13b:agents-plugin/rsrc/lead-ticket/lead-ticket.md`
or an earlier develop commit that predates the markers) with the knob off and
assert the auto-confirm behavior text is transformed; assert the knob-on path
unchanged. Assert behavior, not long prose sentences beyond what the anchor needs.

## Item 17 - internal/mcp/session_sublead.go (~line 61)
Every whiteboard query/update from a parented lead rereads `sublead.json`, even
when unchanged: a per-call file read, against AGENTS.md Code Standard 6 and
ai-docs/manuals/performance.md (read it first). Fix: cache the immutable
sub-lead marker in memory (it is written once at mint) separately from the
session-validity checks, so steady-state whiteboard calls do no file read for
it. Keep invalidation correct if the marker can be removed (check how it is
written/removed). Test: count file operations with the existing file-activity
instrumentation (wsstate counters) to show a second call does not reopen it,
if such a test helper exists; otherwise a focused unit test of the cache.

## Verification
- `go test ./internal/mcp -run 'Sublead|Ticket|Playbook|Render|Golden' -count=1`
- Do NOT commit. Leave changes uncommitted; the lead commits. Other delegates
  edit other files in the same worktree concurrently: touch only files this
  plan needs; no `git checkout`/`git stash`/tree-wide formatters.
