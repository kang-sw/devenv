# Plan: merge develop into the image-attach impl branch and resolve conflicts

## Outcome

`impl/develop/ended-reply-cross` (ticket 261107-feat-ws-dashboard-composer-image-attach,
closed to `.done/` on that branch) must merge into `develop`. Develop moved
since the branch started. `git merge-tree` shows:

1. A content conflict in
   `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`: develop
   landed the thin icon-only send button (783d58c5a: `.crow` grid with
   `#csend` beside the textarea) and the UI shell (026a2d622); the branch
   added the composer image-attach UI (chips/drop zone). Keep both: the
   attachment UI and the thin send button must both work.
2. A false directory-rename: the branch moved the only `ready/` ticket to
   `.done/`, so git's directory-rename detection wants to move develop's two
   new ready tickets (`261107-feat-ws-dashboard-conversation-history`,
   `261107-feat-ws-dashboard-steer-promotion`) into `.done/`. They must stay
   in `ai-docs/tickets/ready/` unchanged, exactly as on develop.

## Settled constraints

- Merge develop into this branch (in this worktree, which has the branch
  checked out), e.g. `git merge -X no-renames develop` or resolve the rename
  by hand; either way the result must have both tickets in `ready/` with
  develop's content and nothing new in `.done/` except the image-attach
  ticket.
- Also check the auto-merged dashboard.js and dashboard.css hunks for
  semantic interaction with develop (send button click handler
  `sendMessage(); ctext.focus()`, key-send input mode, splitter) and with the
  build-stamp change in ensure.go/daemon.go (no overlap expected).
- No behavior change beyond the resolution.

## Boundary

Only the merge commit (and this plan, committed with it or in a follow-up
commit on the branch). Do not merge into develop or push. Return to the lead
if resolving needs a design decision.

## Verification

`go build ./... && go vet ./internal/... ./cmd/...`,
`go test -count=1 ./internal/wsdashboard/ ./internal/wsinbox/ ./internal/wswhiteboard/`
in agents-plugin-tool; `node --check` on dashboard.js; confirm
`git diff develop -- ai-docs/tickets/ready/` is empty after the merge.
