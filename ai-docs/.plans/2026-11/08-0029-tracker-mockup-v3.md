# Plan: tracker mockup v3 - settled rows, new model decisions, view modes, scenario walk

Branch `delegate/tracker-mockup-v3` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit on this
branch (several commits are fine); do not merge or push.

Scope: `ai-docs/ref/tracker-mockup/` only. It is the unstyled reference
mockup of epic `261107-epic-ws-personal-common-store`; no shipped code
changes. Read first: that folder's `README.md` (data model, Settled table),
the epic, and `ai-docs/tickets/ready/261107-feat-ws-tracker-ledger-protocol.md`
(identity, per-clone refs, journal, event line format). The mockup's
`engine.js` becomes the reference implementation the Go data-structure child
ports next (overnight), so its rules must be explicit, deterministic, and
match the README.

## Decisions (user, board, 2026-11-08)

1. (268) ok - follow the Settled rows the page still models as v2:
   - (212, 230, 237): one ref per clone `refs/heads/tracker/m/<member>/<clone>`,
     event id `<member>.<clone>#<seq>`, `seen` per clone stream. One fixture
     member works from two clones. "Viewing as" stays per member; a clone
     picker chooses which clone the viewer's writes go to.
   - (214): "requester confirms" completion choice when sending (now part of
     the unified assignment below).
   - (227): writes batch in a local journal and close into one signed commit
     when `seen` would change or at push; the drawer's unpushed list shows
     the open batch and closed commits.
2. (271): one member's concurrent edits of the same body from two clones -
   the later ts wins, exactly the (203) field rule; the losing edit stays in
   history; no conflict mark.
3. (272) c + (277) c: items get `due` (optional date) and `priority` (one
   integer). Both are item fields edited through `item.create`/`item.edit`
   (creator only, per (274) a) and merge by the (203) rule. Priority levels
   are store configuration: a level count and names in the roles file
   (genesis), changed by an admin event (name it after the `roles.*` family,
   e.g. `roles.set_priority_levels`); default 3 levels named 알림 / 업무 / 긴급
   (display names; keep codes language-neutral). Larger number = more urgent;
   new items start at the store's default level; a stored value outside the
   current range displays as the top level. (278) a: no labels/tags in v1;
   the `project` free-text string stays the only grouping.
4. (273) b + (279) a + (280) a: assignment is unified.
   - `item.assign` may target anyone. A target within the author's reach (N
     levels below the author, i.e. the author is their lead chain - the
     current reach rule) is a **forced** assignment, live at once. Any other
     target makes a **pending** assignment until the target accepts or
     declines. Pending assignments are excluded from roll-ups and status.
   - On accept, the accepter's direct lead (org parent) gets a reference
     notification (show it in that lead's My team view and handle badge) and
     may return it (today's `request.return` rule moves to assignments).
     An agent never accepts or declines (`agent-auto` invalid there).
   - A "request" is no longer a separate kind or create-time field: it is an
     item created with a pending assignment in the same commit. The
     `request.*` events become assignment-level accept / decline / return
     events (name them in the `item.*` family consistently). Received
     requests card = pending assignments to me; Items filter `request` =
     items with a pending assignment.
   - Who may assign: the item's creator (as today), plus an assignee
     handing off their own assignment to anyone ((273) b): a handoff is a
     pending assignment to the new person that, on accept, ends the
     original assignee's assignment; the assigner of the original is
     notified like the lead.
   - `all` (everyone within N, live) stays forced-only, so only leads use it.
   - Completion mode (assignee closes / assigner confirms, incl. (214)) stays
     per assignment.
   - Update every rule that cited "creator only assigns", "within N levels",
     or the request section; the fixtures must exercise forced, pending
     accepted, pending declined, returned, and handoff.
5. (275) ok - the view reads only a derived-state object and writes only by
   dispatching events. Define that object's shape explicitly in one place
   (e.g. a `deriveViewState(viewer)` in engine.js returning plain JSON) and
   document it in the README as the future backend API shape; `app.js` must
   not reach into engine internals.
6. (276) b - view modes, unstyled but usable: kanban (columns by status;
   cards are assignments on My/Team kanban, items on the Items kanban; drag
   moves a card only for the viewer's own assignment and dispatches
   `item.status`), activity timeline (events by time, filterable), due-date
   timeline (Gantt-like bars from creation or start to `due`), and calendar
   (month grid by `due`). Keep the existing cards and tree.
7. No styling work (masonry etc. deferred).

## Scenario walk (board (267) a, (281) a)

After the above, walk these through the page (headless Chromium via cached
Playwright under `~/.npm/_npx/*/node_modules/playwright`) and engine rules:
1. Morning check: today's work (own assignments by priority/due, pending
   assignments to me, confirms waiting on me).
2. A lead finds stuck work in their team (blocked, unaccepted, stale).
3. A pending assignment is accepted, the accepter's lead returns it, the
   assigner sends again.
4. A member leaves: an admin takes over open work, then removes the member.
5. An agent auto-updates status; the human sees and corrects it.
6. A member works offline for days on one clone, then syncs; concurrent
   field edits show their display winner.
7. One member edits the same body from two clones (later ts wins).
8. An assignee hands off their assignment to a peer; the peer accepts.
9. A lead's team member's item has a typo: only the author edits; others
   comment.
Write `ai-docs/ref/tracker-mockup/SCENARIOS.md`: per scenario the steps,
outcome, and snags classified **view** or **model**, then a summary table.
Fix view snags when small; do not change the model for a model snag -
record it with a recommended resolution (the lead decides).

## Verification

`node --check` on every script; the page loads without console errors in
headless Chromium for at least three viewers (root, lead, member); DOM or
screenshot checks back each view mode and each scenario outcome.

## Return

Changed files; per decision what changed; the derived-state shape summary;
the scenario summary table with every model snag and its recommended
resolution; anything you could not settle.
