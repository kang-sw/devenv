# Plan: merge develop into the steer-promotion impl branch and finish the image-attach clauses

## Outcome

`impl/develop/mummy-cover-defy` (ticket 261107-feat-ws-dashboard-steer-promotion,
closed to `.done/` on that branch, commits 9496792ea..7180af38f) must merge into
`develop`. Develop has since landed the composer image attachments (d31ced27e),
the conversation history split (1cb3638d9: `entries`/`convlive` regions,
`#convo`, entry cache, `#r-convlive [data-cancel]`), the whiteboard ticket
kanban (c0384c306), and ticket edits. Merge develop into this branch in this
worktree and resolve every conflict so both sides work.

The ticket's two image-attach clauses could not be done on the old base and are
part of this merge (ticket decision "Steer rewrite under the inbox lock: copy
image paths; Send and the cap count skip steered entries"):

1. Keep both `Images` and `Steer` in `wsinbox.Entry`; `Steer` copies the whole
   entry, so images carry over - verify with a test.
2. `Drain` and `DrainSteer` both go through `drain(dir, steerOnly)`; inside it
   call develop's `trimDrained(dir)` (signature changed on develop).
3. Move the "steer" route case into develop's `inboxWrite(dir, route, raw)`
   switch (it no longer takes `now`).
4. In develop's `sendMessage`, the attachment cap must count the images of the
   entry the send would merge into, i.e. `wsinbox.MergeTarget(dir)`'s record,
   not `held[len(held)-1]`, so steered entries are skipped. Add a test: a held
   entry with images is steered, a new send with images is capped against the
   new entry only.
5. The held-bubble template: keep develop's thumbnails and this branch's steer
   tag and button. The held bubbles now live in the `convlive` region
   (develop's conversation-history split); any steer button click handler and
   selector must target `#r-convlive`, as develop's cancel handler does.

## Settled constraints

- No behavior change beyond the resolution and the two clauses above.
- `git merge develop`; the false directory-rename trap (git moving ready/
  tickets into .done/) should not apply since develop has no ready/ tickets
  now, but confirm `ai-docs/tickets/ready/` matches develop after the merge.
- Check the runtime.json `dashboard.inbox_drain` minimum this branch raised to
  `>=0.48.11-dev <0.49.0` in ws, wsflow, and Pi runtime.json: the plugin
  version is still 0.48.10(-dev). Confirm against the launcher and the
  runtime-contract tests that this cannot refuse the current build; if it
  would, return to the lead with the evidence rather than bumping versions.

## Boundary

Commit the merge (and this plan) on this branch only. Do not merge into
develop or push. Return to the lead if a resolution needs a design decision.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`,
`go test -count=1 ./...`; `-race` on wsinbox and wsdashboard steer/drain/send
tests. `node --check` on dashboard.js. `claude plugin test .` in
agents-plugin and agents-plugin-wsflow; `go test ./internal/wsrsrc/`; the Pi
targeted node tests the worker ran (mailbox-waiter, dashboard-feed,
adapter-label, native-tool-registration). If headless Chromium is available
(cached Playwright under ~/.npm/_npx/*/node_modules/playwright), load a
session page with a held entry and confirm the steer button and cancel both
work in the `convlive` region. Report what was not checked.
