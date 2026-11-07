# Plan: tracker mockup v2 - threads, status, offline concurrency

Branch `delegate/tracker-mockup-v2` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

## Context

`ai-docs/ref/tracker-mockup/` is a standalone file:// HTML+JS mockup for the
team tracker epic `261107-epic-ws-personal-common-store` (read the epic's
Cross-Child Decisions). v1 (`README.md`, `engine.js`, `app.js`, `style.css`,
`index.html`, `fixtures.js`) shows a dashboard-like frame with per-store drawer
handles, cards, an item side sheet, and action dialogs, over a replay engine.
The user likes the v1 frame, team view and task handling; v2 changes the data
model per the decisions below and turns the item view into a thread. Keep the
frame, drawer handles, team card and "viewing as" switch.

## Decisions settled on the board (user, 2026-11-07)

Board IDs in brackets; v1 README "Open questions" numbers as Qn.

Roles and identity
- [149] Q1 roles file = genesis; later changes are admin `roles.*` events.
- [150] Q2 admins = list of member ids. [151] Q4 several roots allowed.
- [157] Q3 add `roles.add_admin`, `roles.remove_admin`, `roles.revoke_key`,
  `roles.remove_member`. Events signed before a revocation stay valid.
- [147] Q19 an event's author must own the ref it sits in (admin reassignment
  of a departed member's work is an event in the admin's own ref).
- [148] Q20 everyone in a store sees everything; "viewing as" only changes
  perspective.

Commits, ids, ordering, offline
- [154] Q7 one commit per write, holding several events. [156] Q18 no per-event
  `sig`; the commit is the signed unit (mock it at commit level, e.g. a
  `signer`/key field on the commit record).
- [153] Q6 event id = `author#seq`. [171] no global sequence numbers; display
  short aliases like `maya-12`.
- [168] every commit records `seen`: for each member, the last seq it had
  received when written (supersedes Q5 timestamp-only merge order).
- [167] a: validate each event against the state its author had seen (the
  events within its commit's `seen` vector plus the author's own earlier
  events), never against later arrivals. An already shown event's verdict never
  flips because of a late arrival. Concurrent conflicts (two concurrent edits of
  one field; progress or close on an assignment that was concurrently
  unassigned; etc.) are surfaced in the thread as a visible "concurrent change"
  marker, not silently resolved. Pick and document a deterministic display
  winner for concurrent field edits (for example later `ts`, then author id) and
  show both values.
- [168] exception: an event signed by a revoked key that arrives after the
  revocation is ignored even if its `seen` predates the revocation (a stolen key
  cannot pretend to be old).
- [169] no remote and long offline are the same path; the drawer shows "last
  synced" and "N of my writes not yet pushed"; sync failure is retried, not an
  error. Mock this with a per-store sync state and a "go offline / come back
  online" control that holds the viewer's new commits locally and lets fixture
  commits from others "arrive late", so the concurrency display is demoable.
- [170] deletion = hide (tombstone) event; content stays in history.
- [181]-[183] (engine notes, not UI): incremental apply via per-member cursors;
  a late event re-replays only its thread; a roles change re-replays all;
  member ref layout `events/YYYY/MM/DD.jsonl`. Document in README; the mockup
  engine may still replay everything.
- [184] a: no snapshots in v1.
- [185] local clone is a bare repo under `~/.ws/tracker/<store>/` (user data,
  not cache); index is cache. README only.

Items and threads
- [166] a: thread-centric. Each task/todo/topic is a thread; comments,
  progress (status) changes, assignments, edits, closes, and agent records all
  appear on one chronological timeline. Subtasks are child threads shown as a
  line in the parent timeline. The item side sheet becomes the thread view with
  a composer at the bottom (comment, plus quick status actions).
- [175] kinds `task` / `todo` / `topic` (v1 `note` becomes `topic`: a
  discussion thread with no assignment or status, comments only).
- [162] a: one `item.create` with `kind`, plus `item.edit` (title/body) by
  the creator.
- [176] `comment.add`; only the comment author edits or hides their comment.
- [177] flat timeline; a comment may carry one optional `reply_to` (quote
  reply), no nested tree.
- [146] Q16 project reference = one optional free-text string.

Assignment and progress
- [158] b: an "everyone within N levels" assignment stays live: followers who
  join later inherit it.
- [159] a: delegating down always creates a subtask under one's own name; only
  the creator assigns an item.
- [144] Q10 a subtask's parent must be assigned to or created by its author.
- [145] Q12 completion mode (assignee closes / assigner confirms) per assignment
  event; default assignee closes.
- [160] a: add `item.unassign` (by the assigner), `item.reopen`, and
  `item.reject` (assigner declines a confirmation; back to doing).
- [161] b: progress is a status `todo` / `doing` / `blocked` / `done` plus an
  optional note; no percentage.
- [178] c (roll-up, Q14): parents show counts of children by status (for
  example "done 3/5") and a warning when any descendant is `blocked`, up to the
  root; the parent's own status and completion stay manual by its assignee.
- [163] a: self-assignment allowed and used for one's own work in both personal
  and team stores; "My assignments" gathers everything.
- [155] Q17 one `origin` field: `human` / `agent` / `agent-auto`;
  `agent-auto` only for status changes and closes on one's own assignments
  (epic agent recording rule).

## Outcome

1. `engine.js`: implement the model above: commit records
   (`{author, seq range or events, ts, seen, signer}`), the new event types,
   author-view validation with `seen`, concurrency detection and markers,
   tombstones, edits, live "all within N" assignment, status-based roll-up with
   blocked propagation, revocation rule. Keep it a pure data layer
   (`window.TrackerEngine`).
2. `fixtures.js`: rewrite the team store fixtures as per-member commit lists
   with `seen`, covering: threads with comments and reply_to, an edit, a hidden
   comment, unassign/reopen/reject, a blocked subtask propagating a warning, a
   later-joining follower inheriting an "all within N" assignment, a revoked
   key with one ignored late event, and at least one long-offline member whose
   commits arrive late and create concurrent changes (field edit conflict and
   a status report on a concurrently unassigned assignment). Keep the personal
   store (one member, self-assigned todos).
3. `app.js` / `style.css` / `index.html`: the item side sheet becomes the
   thread view (timeline + composer + status quick actions + subtasks); cards
   use status chips and counts; sync state and offline toggle in the store
   header; flagged and concurrent markers readable. Actions: create (kind),
   edit, comment (with reply), hide own comment, assign (all/chosen within N,
   completion mode), split into subtask, status change, close, confirm,
   reject, unassign, reopen, plus admin actions for the new roles events.
   Keep the Data card (raw JSON, editable, apply/reset).
4. `README.md`: rewrite as v2: data model, commit and `seen` semantics,
   validation and concurrency rules, flagged reasons, the engine notes
   [181]-[185], and replace the 21 open questions with a short "settled"
   table (board id -> decision) plus any new open questions you hit (keep them
   few and concrete). Keep "Not modeled" current.

## Verification

Open `index.html` through headless Chromium (cached Playwright under
`~/.npm/_npx/*/node_modules/playwright`) from `file://`: no console errors;
screenshots of the drawer, a thread with a concurrency marker, the offline
toggle before/after, at a desktop width and ~420px. Exercise each action once
programmatically or by clicking and check the resulting event applied or was
flagged as expected. `node --check` every JS file. Keep it dependency-free.

## Return

Changed files, model decisions you took where the list above was silent,
new open questions, verification evidence (screenshot paths), what was not
checked.
