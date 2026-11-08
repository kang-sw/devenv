# Plan: tracker mockup - notification card, mentions, attachment metadata

Branch `delegate/tracker-mockup-notifications` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch (several commits are fine); do not merge or push.

Context: the unstyled reference mockup `ai-docs/ref/tracker-mockup/` of epic
`261107-epic-ws-personal-common-store`, and its Go port
`agents-plugin-tool/internal/wstracker` (golden parity with the mockup
engine, `testdata/gen-golden.js` + `golden_test.go`, board (283) a). Read the
mockup README first; it is the data-structure contract. Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## Decisions (user, board, 2026-11-08)

1. **Notification card** ((331) c, (332) a, (333) a, (334) ok).
   - One card in the cards layout, list only (no mode picker), newest first.
     Clicking an entry opens that item's thread sheet.
   - Entries for the viewer: new forced assignments to me; pending
     assignments to me; answer notices (my request declined, or returned by
     the accepter's lead); reference notices (a follower accepted a pending
     assignment; a handoff of an assignment I gave was accepted); completion
     or `blocked` of work I assigned; Confirm requests waiting on me;
     replies to my comments; my live assignments near their due date
     ("due today" / overdue, as the view already computes); and mentions of
     me (decision 2).
   - The card is a feed only: Received requests and My team keep their
     notices and buttons (Accept / Decline / Return) unchanged.
   - Read state is per-viewer local UI state, like the handle badge's
     `seen` map in app.js: unread entries are highlighted; clicking one or a
     "mark all read" action clears it; no event is written. (In the product
     this state lives on the local filesystem per clone outside git, read
     and written by the daemon, (340); the mockup keeps it in memory or
     localStorage. Say so in the README.)
   - The handle badge keeps counting exactly what it counts today.
   - Derive the feed from the view state where possible. If an entry kind
     needs data the view state lacks, add it to `deriveViewState` (and the Go
     port, decision 4) rather than reading raw events in app.js.
2. **Mentions** ((331) c, (335) ok). Comment events (`comment.add`,
   `comment.edit`) and item body edits (`item.create` / `item.edit` when the
   body changes) carry an optional explicit `mentions` list of member ids.
   The body text keeps `@name` for display. Validate against the author's
   seen state like every other rule (each id must be a member then;
   otherwise reject with a reason). A mention shows in the mentioned
   member's notification card, and the thread sheet renders mentions as
   chips. The composer in the thread sheet offers a simple `@` picker of
   members (unstyled is fine). An agent-authored write may mention.
3. **Attachment metadata** ((337) a, (338) a, (339) a). Comments and item
   bodies may carry `attachments: [{oid, name, size, mime}]`, where `oid`
   is the sha256 of the content. Only metadata is modelled: the real
   storage (content-addressed blobs under `attachments/<sha256>` in the
   member's ledger tree, fetched on demand through a partial clone) is a
   later ledger ticket. The roles file gains a store setting
   `attachments.max_bytes` (default 10 MiB), changed by an admin
   `roles.*` event in the same style as the priority levels; an attachment
   above the limit is rejected with a reason. The thread sheet shows each
   attachment as a chip with name and size and a simulated "not loaded /
   loaded" state (click to "load", local UI state only). Add a fixture
   with at least one attachment and one over-limit rejection.
4. **Go parity in the same change** ((336) ok). Mentions, attachments,
   the new roles setting, and any view-state field added for the
   notification feed are ported to `agents-plugin-tool/internal/wstracker`;
   regenerate the golden files with `testdata/gen-golden.js` and keep the
   parity tests green. The ledger layer (`internal/wsledger`) is untouched.
5. README: document the notification card, mentions, attachment metadata,
   the `attachments.max_bytes` setting, the product-side local-filesystem
   read state (340), and Settled rows for (331)-(341) (mention (337)'s
   storage choice and that it is deferred to a ledger ticket). Update the
   Non-scope line "notifications beyond the handle badge and the derived
   notices" accordingly. Add a SCENARIOS.md step only where it helps
   exercise the new behaviour.

No styling beyond what the layout needs (masonry styling comes next).

## Verification

- `node --check` on every script; the page loads without console errors in
  headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`) for at least three viewers.
- DOM or screenshot evidence: the notification card lists each entry kind
  present in the fixtures, unread highlighting clears on click and on
  "mark all read", a mention appears in the mentioned viewer's card and as a
  chip in the thread, an attachment chip toggles loaded, and an over-limit
  attachment is rejected with its reason.
- `cd agents-plugin-tool && go build ./... && go vet ./internal/wstracker/ && go test ./internal/wstracker/ ./internal/wsledger/`
  passes after the golden regeneration.

## Return

Changed files; per decision what changed; evidence paths; test output
summary; anything you could not settle (stop and report rather than
changing `internal/wsledger` or inventing a rule beyond these decisions).
