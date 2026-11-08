# Plan: tracker interaction - context menu, inline edits, composer, status ribbon

Branch `delegate/tracker-interaction` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`), based on develop
(which already has the hierarchy work, merge `38c478248`). Commit here; do not
merge or push. Do not run git stash.

Renderer: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` and
`tracker.css`, shared by the dashboard and the mockup
(`ai-docs/ref/tracker-mockup/`, via `mock-adapter.js`). Read
`ai-docs/manuals/shipped-surface-boundary.md` first. Domain and settled UX:
`ai-docs/ref/tracker-mockup/README.md` (Settled rows). No event, view-state, or
Go `wstracker` change is expected: every action below dispatches events the
renderer already dispatches from its dialogs today. If one turns out to need a
new event or view field, stop and report.

## Decisions (user, board, 2026-11-08)

1. **(471) a - status applies on click.** Status radio (todo / doing / blocked
   / done; "done" reads "request done" when the assignment needs assigner
   confirmation) applies immediately. A note field under the radios: typing a
   note and pressing Enter applies "status + note" together.
2. **(472) b - modals become menus or inline.** Status, close, subtask,
   priority, due, handoff, reject/reopen/decline/return notes, comment edit,
   and item edit move to the context menu or inline. Assign ("everyone N
   levels below", completion mode choice) is an anchored popover opened from
   the menu. New item becomes an inline input row at the top of the list
   (title + Enter; the rest via the menu). Only admin and profile stay modal.
3. **(473) one context-menu component**, opened identically from list rows,
   kanban cards, calendar entries, gantt rows, and the item window header; by
   right-click, a hover "⋯" button on the row, and keyboard Shift+F10 / the
   menu key. Clicking a row's status icon opens the same menu showing only its
   status part. Menu sketch:
   ```text
   ○ todo  ● doing  ○ blocked  ○ done   (radios, apply on click)
   [note (optional) ____ ⏎]
   ＋ add subtask        -> input row right under the row
   priority        ▸     -> radio submenu
   due             ▸     -> date picker
   handoff / assign ▸    -> people search list / assign popover
   reject · reopen · return -> note input inside the menu
   edit                  -> inline title/body edit in the item window
   ```
   Show only the actions the user's role and the item's state allow (the
   renderer already computes this for its current buttons).
4. **(474) inline subtask:** an input row one level indented right under the
   row. Title + Enter creates, Esc cancels. Assignee empty, completion default
   (assignee closes). Assignee and due are set afterwards from the new row's
   menu. Enter repeatedly keeps adding siblings.
5. **(498) activity-panel checkboxes become toggle chips**: "flagged only" and
   "concurrent only" use the same chip look as the adjacent `list / activity
   timeline` chips, accent when on. No native checkboxes remain in the drawer
   outside modal forms.
6. **(499) comment composer like the dashboard chat input**: no "Post" text
   button; a paper-plane icon button inside the box at bottom-right, reusing
   the dashboard chat composer's look (find it in
   `agents-plugin-tool/internal/wsdashboard/assets/`). Cmd+Enter (macOS) /
   Ctrl+Enter sends; Enter is a newline. The `@ mention` select shrinks to a
   small `@` icon inside the box at bottom-left. The old status line above it
   (`my status todo set: doing blocked Close (text below becomes the note)`)
   is replaced by decision 1's menu, so it goes.
7. **(500) store status rows become a thin ribbon at the bottom of the Items
   card**: remove the two rows under the store name (clone ref, remote state,
   "local only ...", unpushed count, Close batch, the "unpushed: open batch"
   disclosure). Put one slim ribbon at the bottom of the Items card: a status
   dot, short text ("local only", "4 to push", "synced 3 min ago"), and the
   apply button only when writes are pending. With nothing to do it dims to a
   quiet single line. Clicking it opens a small popover with the diagnostics
   (clone ref, members, events, commits, flagged, batch contents). The store
   header keeps the name, "Open as page", and "+ New".

## Constraints

- Keyboard: every menu and popover is reachable and dismissible by keyboard
  (arrows, Enter, Esc returns focus to the opener); focus trapping like the
  existing dialogs where it applies.
- Touch: the "⋯" button must be visible on touch (no hover) layouts.
- Strings: new UI strings go in `wsi18n/tables/en.json` under `js.tk.`; remove
  keys that become unused (keys_test rejects unused keys); regenerate the
  mockup's i18n-en.js
  (`WS_REGEN_TRACKER_MOCKUP_I18N=1 go test ./internal/wsdashboard/ -run TestTrackerMockupI18nIsCurrent`).
  Do not edit non-English tables except to delete a key you removed from
  en.json (another delegate is translating ko/ja/zh/ru right now; deleting a
  removed key from those tables would conflict, so instead list removed keys
  in your report).
- Embed budget: `TestRenderPageEmbedSizeBudget` (wswhiteboard) has ~1 KB
  headroom; js.tk strings are already excluded from the whiteboard embed, keep
  it that way.
- README: update the mockup README text the change contradicts and add
  Settled rows for 471 a, 472 b, 473, 474, 498, 499, 500.
- Rows keep `data-item`; the hierarchy work's tree rows and fold state stay.

## Verification

- `node --check` on tracker.js and mock-adapter.js.
- `cd agents-plugin-tool && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/ ./internal/wstracker/`.
- Headless Playwright on the mockup (cached at
  `~/.npm/_npx/705bc6b22212b352/node_modules/playwright`): console errors
  empty; screenshots in `/tmp/tracker-interaction/` of the menu on a list
  row, kanban card, gantt row, item window header; status icon menu;
  assign popover; inline new-item row; inline subtask row; composer;
  toggle chips; the ribbon (pending and idle) and its popover; light and dark.
  Drive at least: status change by click, status + note by Enter, inline
  subtask create with two Enters, Cmd+Enter comment send, Esc focus return.

## Return

Commit hashes, changed files, removed i18n keys, test output, screenshot list,
judgement calls, and open items.
