# Tracker mockup v3

A layout and data-model mockup for the team tracker (epic
`261107-epic-ws-personal-common-store`), inside a frame that imitates the
dashboard session page. Everything the page shows is computed in the browser
from per-clone commit lists on top of a roles file: each clone appends only to
its own ref, and every view is a client-side merge of what that clone holds.
v1 settled the frame and the cards; v2 settled threads, status, `seen`-based
concurrency and peer requests. v3 applies the board decisions of 2026-11-08,
listed in [Settled](#settled): per-clone refs and journal batching, due dates
and priorities, one unified assignment (forced or pending, with handoff), a
single derived view-state object, and four more view modes. `engine.js` is the
reference implementation the Go data-structure child ports, and
[SCENARIOS.md](SCENARIOS.md) walks nine user scenarios through it.

## Open it

Open `index.html` from the file system (double-click, or a `file://` URL). No
build, no server, no dependencies.

- `index.html` holds the frame and mount points. `style.css` holds the neutral
  layout styling, with the dashboard's variables and drawer geometry.
- `engine.js` is the data layer (`window.TrackerEngine`). It computes causality
  from `seen`, validates each event against what its author had seen, folds the
  state, marks concurrent changes, derives the view state, and simulates the
  per-clone journal and sync.
- `app.js` is the view. It reads only the view state (`store.view`) and writes
  only by dispatching events (`store.dispatch`).
- `fixtures.js` assigns `window.TRACKER_FIXTURES`, because `fetch` of local JSON
  is blocked on `file://`. Edit it to keep a data change.

## What the page shows

- **Frame.** A header like the dashboard's, with three mockup controls:
  - *viewing as*: the member, labelled root, lead, member, or removed;
  - *clone*: which of that member's clones (one ref each) the page reads and
    writes; `*` marks a clone with unpushed writes;
  - *now*: the mock clock. Every write is stamped with it and advances it by a
    minute; `+1h` / `+1d` move it forward (stale and overdue use it).

  Switching viewer means the previous clone collapses its panel (it pushes,
  (241)) and the new clone opens the drawer (an immediate fetch); an offline
  clone does neither. A pull-down drawer has one handle per store. A handle
  badge counts new forced assignments, pending assignments to the viewer, and
  reference notices; opening the store clears it (the notification card below
  does not change what it counts). A handle shows ⊘ when the
  viewer's clone is offline or the store has no remote.
- **Store header.** The viewer's clone ref, online or offline, last synced, "N of
  my writes not yet pushed" with a list of the open batch and the closed
  unpushed commits, and upstream commits this clone has not fetched. The
  buttons are `Sync now` and `Go offline` / `Come back online`. The personal
  store has no remote, which is the same path as a long offline; its button
  only closes the batch.
- **Cards.** The drawer has one layout, the cards layout (303). There is no
  drawer-level mode picker; each card has its own picker offering only the
  modes that fit its content:

  | card | modes |
  |---|---|
  | *Items* | tree / kanban (item cards) / due timeline / calendar |
  | *My assignments*, *Assigned by me*, *My team* | list / kanban (assignment cards, with the pending column) / due timeline / calendar |
  | *Activity* | list / activity timeline |
  | *Notifications*, *Today*, *Received requests*, *Team*, *Data* | list only (no picker) |
  | *Support agent* (chat placeholder) | conversation only (no picker) |

  A card switched to a wide mode (kanban, due timeline, calendar, activity
  timeline) leaves its column and spans the full row of the cards layout; the
  remaining cards flow below it (304). There is no horizontal-scroll-in-card
  fallback and no maximize button. Each card's chosen mode is local UI state,
  remembered per store in memory (it survives a viewer switch within the
  store), never an event (305).
  - *Lists* (the v2 layout plus *Notifications* and *Today*):
    - *Notifications* ((331)-(334)): one feed of what concerns the viewer,
      newest first: new forced assignments to me, pending assignments to me,
      answer notices, reference notices, completion or `blocked` of work I
      assigned, Confirm requests waiting on me, replies to my comments, my
      live assignments due today or overdue, and mentions of me. Every entry
      is derived from the view state and only opens the item's thread sheet;
      *Received requests* and *My team* keep their notices and buttons
      (Accept / Decline / Return). Unread entries are highlighted; clicking
      one, or *Mark all read*, marks it read. Read state is local UI state per
      viewer clone, kept in memory like the handle badge's `seen`, and writes
      no event. In the product the daemon keeps it per clone on the local
      filesystem, outside git (340).
    - *Today*: the morning check. My open work by priority then due, pending
      assignments waiting for my answer, and closes waiting for my confirm.
    - *My assignments*: my live assignments (forced, self-assigned, accepted,
      or taken over by handoff), with status, source, completion mode, the
      roll-up and blocked warning, priority, due (or "due today"), stale, a
      "by agent" chip when an agent wrote the current status, the latest note,
      and quick actions including *Hand off*.
    - *Received requests*: answer notices (what I sent was declined, or
      returned by the accepter's lead), pending assignments to me (Accept /
      Decline / Discuss), the ones I answered, and the ones I sent (Withdraw).
    - *Assigned by me*: what I assigned or handed to others, live or pending,
      with each assignee's status and Confirm / Reject.
    - *My team*: shown to a viewer with followers or notices. Reference
      notices first (a follower accepted a pending assignment; a handoff of an
      assignment I gave was accepted), with Return. Then each follower down to
      3 levels as an indented tree with their open work, blocked, stale, and
      unaccepted counts, plus the pending assignments they sent that nobody
      accepted yet ("waiting on others").
    - *Items*: the tree of threads, filtered by kind (`request` = items with a
      pending assignment) and status (open / blocked / done / hidden).
    - *Team*: the org tree, admins, clones per member, revoked keys, the
      priority levels, and an admin roles-event dialog.
    - *Activity*: the merged log in causal order, newest first, with filters
      for flagged-only and concurrent-only.
    - *Data*: the raw roles file, each ref's pushed commits (the remote), and
      the clone state (fetched counts, local commits, online), all editable.
      `apply` recomputes everything and `reset` restores the fixtures.
  - *Kanban*: columns by status, scoped to the card's own rows. On an
    assignment card the cards are that card's assignment records (with a
    pending column; *Assigned by me* and *My team* name the assignee); on
    *Items* the cards are items under the card's kind filter only: the
    columns already are the statuses, so the status filter is ignored there
    (it would only empty columns) and the kanban toolbar offers kind alone.
    Dragging moves a card only for the viewer's own live assignment and
    dispatches `item.status` (todo / doing / blocked) or `item.close` (done).
  - *Activity timeline*: every event by write time, grouped by day, filterable
    by event family, member, flagged, and concurrent.
  - *Due timeline*: one bar per dated row of the card (an item on *Items*, an
    assignment record on an assignment card, showing its own state), from the
    item's start (first `doing`, else creation) to its due day, with a today
    line; overdue bars are marked.
  - *Calendar*: a month grid of the card's dated rows by due day, with month
    navigation per card.
- **Support agent card** (chat placeholder, (300), (302)). A placeholder for
  the support-agent chat child, the future daemon-side local support agent:
  placement and a static sample only, with no working chat, no responder, and
  no new events. It is one card in the cards layout, fixed height with
  internal scroll, newest message at the bottom, and a disabled composer row.
  The sample mimics the dashboard conversation view's entry shapes (user
  text, assistant text, a collapsed tool line) plus one new entry kind, the
  **tracker card reference**: a mini card of an item (title, status, priority,
  due, assignees) read from the view state, which opens that item's thread
  sheet on click. The referenced items are real ids from the current viewer's
  view state (top open work first, then what waits for an answer or confirm,
  then what the viewer assigned), so every reference resolves.
- **Thread view** (side sheet). Meta (status, priority, due, creator, project,
  parent), the body, item actions (an admin also gets Edit / Hide on a removed
  creator's item), the assignees with their state and actions (an admin also
  gets *Hand off (admin)* on any member's live assignment), and the subtask
  threads. Below that comes one chronological timeline of comments (quote
  reply, edit, hide), status changes, assignments and answers, edits, agent
  records with `agent` / `auto` chips, subtask lines, flagged and waiting
  events with their reason, amber "concurrent change" boxes, and quiet
  "superseded by" notes on a member's losing write from another clone. The
  composer posts a comment, as a reply when one is chosen, with an origin; with
  an own assignment on the thread, quick status buttons use the composer text
  as the note. Mentions on the body and on each comment show as chips (335).
  Attachments show as chips with name and size and a simulated "not loaded /
  loaded" state: a click toggles it, as local UI state per viewer clone
  (338). An attachment on a flagged write shows struck through. The composer
  has a simple `@` picker that inserts `@id` and records the id, and a mock
  attach row (file name and byte count; the mock stands in a 64-hex oid
  because it has no content). An item or comment edit that changes the body
  keeps the attachments and the mentions whose `@id` is still in the text.
- **Actions** append events to the viewer clone's open batch, then report each
  event as applied or flagged. The actions are create (any kind, with due,
  priority, and an optional assignee in the same write), edit, comment, reply,
  edit or hide own comment (each with mentions and attachments), assign (chosen people or everyone within N),
  split into a subtask, hand off, status, close, confirm, reject, unassign or
  withdraw, reopen, hide item, accept / decline / return, and every `roles.*`
  event. Actions are not pre-validated, so you can produce flagged events on
  purpose.

## Data model

### Roles file (genesis)

```json
{ "admins": ["ada", "bo"],
  "priority": { "names": ["알림", "업무", "긴급"], "default": 1 },
  "attachments": { "max_bytes": 10485760 },
  "members": [ { "id": "maya", "name": "Maya Ortiz", "keys": ["maya-k1"], "leads": ["ada"] } ] }
```

`admins` lists member ids. A member with `leads: []` is a root, and several
roots are allowed. `leads` is a list validated to length at most 1. Member ids
cannot contain `.`, which separates member and clone in a stream id.
`priority` is the store's priority configuration: level codes are the integers
`0..n-1` (larger = more urgent), `names` are display names only, and `default`
is the level new items start at. Without it the store uses three levels named
알림 / 업무 / 긴급 with default 1. `attachments.max_bytes` is the largest
attachment the store accepts, in bytes (339); without it the limit is 10 MiB
(10485760). Later changes are admin `roles.*` events, folded in causal order.

### Clone streams and commits

```json
{ "stream": "maya.pc", "ts": "2026-11-04T10:00:00Z", "signer": "maya-k1",
  "seen": { "ada.pc": 13, "lena.pc": 4, "maya.lap": 2, "omar.pc": 3 },
  "events": [ { "seq": 15, "ts": "2026-11-04T09:58:00Z", "type": "item.unassign", "payload": { "item": "maya.pc#1", "assignee": "omar" } },
              { "seq": 16, "type": "comment.add", "payload": { "item": "maya.pc#1", "body": "..." } } ] }
```

- Every clone of a store writes only its own ref,
  `refs/heads/tracker/m/<member>/<clone>`. Its **stream** id is
  `<member>.<clone>`; a member may have several clones (the fixture's maya has
  `pc` and `lap`). "Viewing as" picks a member, and the clone picker picks
  which clone the page reads and writes.
- `seq` is per stream, continuous across commits. Event id is
  `<member>.<clone>#<seq>`, shown as `maya.pc-15`. An item's id is the id of its
  `item.create`, and a comment's id is the id of its `comment.add`. There is no
  global sequence.
- `seen` maps each other stream (other members' and the member's own other
  clones) to the last seq this clone had received when the commit was opened.
  The stream's own earlier events are always seen.
- **Journal batching.** Writes go to the clone's local journal as an open
  batch. The batch closes into one signed commit when the clone receives new
  events (its `seen` would change) or when it pushes. All events in a commit
  share its `seen`, which stays exact because the batch closes before anything
  new is received. Each event keeps its own write `ts`; one without `ts` uses
  the commit's. Pushed commits are never rewritten.
- The commit is the signed unit, so the mock carries `signer` (a key id) on the
  commit. Events carry no signature.
- `ts` is informational: display, the order tie-break, and the display winner.
- An event has an optional `origin`: `human` (default), `agent`, or
  `agent-auto`.
- Fields starting with `_` are mock bookkeeping, not model: `_local` (not
  pushed), `_open` (the open batch), `_late` (reached this clone after events
  written later).

### Event types

| type | payload | rule |
|---|---|---|
| `item.create` | `kind` (`task`/`todo`/`topic`), `title`, `body?`, `project?`, `due?`, `priority?`, `parent?`, `mentions?`, `attachments?` | A `parent` must be assigned to or created by the author. `project` is one free-text string, the only grouping (no labels or tags). `due` is a `YYYY-MM-DD` date. `priority` is a level in the author's current range; omitted, the item starts at the store's default level. |
| `item.edit` | `item`, any of `title`/`body`/`project`/`due`/`priority`; `mentions?` and `attachments?` with `body` | Creator only; an admin may also edit an item whose creator was removed (292). `due: null` clears the date; priority cannot be cleared. Each field merges by the (203) rule. `mentions` or `attachments` without `body` is flagged. |
| `item.hide` | `item` | Creator only; an admin may also hide an item whose creator was removed (292). A tombstone: the content stays in history. Flagged while someone else holds an open (live or pending) assignment on the item. |
| `item.assign` | `item`, `mode` (`all`/`chosen`), `levels` (all), `people` (chosen), `completion` (`assignee`/`assigner`, default `assignee`) | Creator only; an admin may also assign an item whose creator was removed. `chosen` may target anyone in the store: the author, or someone the author reaches (the author is in their lead chain, any depth), is a **forced** assignment, live at once; anyone else gets a **pending** assignment. `all` targets everyone within N levels below the author, forced only, and stays live: anyone who later enters the range inherits it. Flagged when every chosen person already holds a live or pending assignment. Not allowed on topics. |
| `item.handoff` | `item`, `to`, `assignee?`, `note?` | The holder of a live assignment hands it to anyone; an admin may hand off any member's live assignment by naming its holder in `assignee` (286), so open work can be taken over before the member is removed. It makes a pending assignment to `to` with the original's assigner and completion mode; on accept the original assignment ends (`handed`) and the new one starts with the original's status and note (290). Flagged on a done assignment, when `to` is the holder or already holds one, or while another handoff of it is pending. |
| `item.accept` / `item.decline` | `item`, `note?` | The target of a pending assignment, on it. Accept makes it live; for a handoff it also ends the original. Decline ends it as `declined`. A pending handoff whose original ends another way is withdrawn with it (295), so a later accept or decline finds no pending assignment. |
| `item.return` | `item`, `assignee`, `note?` | Only the assignee's direct lead, on an accepted (formerly pending) live assignment. Ends it as `returned`; the assigner may send again. Returning an accepted handoff restores the original assignment with its prior status (291), unless its holder was removed or holds another assignment on the item by then. |
| `item.unassign` | `item`, `assignee?` | By the assigner; the sender of a pending assignment may also withdraw it. An admin may also unassign when the assignee or assigner was removed. Without `assignee`, it withdraws all of the author's assignments on the item and ends the live rule. |
| `item.status` | `item`, `status` (`todo`/`doing`/`blocked`), `note?` | Assignee only (live). The creator self-assigns first. Allowed while awaiting confirmation, which withdraws the close. Not allowed on done. |
| `item.close` | `item`, `note?` | Assignee. Moves to `done`, or to `awaiting` when completion is `assigner`. |
| `item.confirm` / `item.reject` | `item`, `assignee`, `note?` | Assigner, on an `awaiting` assignment. Moves to `done` / back to `doing`. |
| `item.reopen` | `item`, `assignee?`, `note?` | The assigner or the assignee reopens a `done` assignment, back to `doing`. |
| `comment.add` | `item`, `body`, `reply_to?`, `mentions?`, `attachments?` | Anyone in the store. `reply_to` is one comment in the same thread (flat timeline, quote reply). |
| `comment.edit` / `comment.hide` | `comment`, `body`, `mentions?`, `attachments?` (edit) | Comment author only. Hide is a tombstone. |
| `roles.add_member` | `member` (`id`, `name`, `keys`, `leads`) | Admin. Stands in for the self-join (`member.join`, (231)). |
| `roles.set_lead` | `member`, `leads` | Admin. Length at most 1, no cycle. |
| `roles.add_admin` / `roles.remove_admin` | `member` | Admin. The last admin cannot be removed. |
| `roles.add_key` / `roles.revoke_key` | `member`, `key` | Admin. Revocation rule below. |
| `roles.remove_member` | `member` | Admin. The member must lead nobody and must not be an admin. Their earlier events stay valid. Their live assignments end as `unassigned` and their pending ones as `withdrawn` (287), so nothing stays counted under a removed member; like an unassign, this beats a concurrent status write. |
| `roles.set_priority_levels` | `names` (1-9 display names, lowest first), `default` | Admin. Changes the level count and names; a field write, so concurrent changes merge by (203). |
| `roles.set_attachment_limit` | `max_bytes` (a positive integer) | Admin. Changes `attachments.max_bytes`; a field write, merged by (203). Attachments already written stay; later writes are checked against the new limit in their author's causal past. |

**Mentions and attachments** ((335), (338)). A body (an item's on
`item.create` / `item.edit`, a comment's on `comment.add` / `comment.edit`)
may carry `mentions`, a list of member ids, and `attachments`, a list of
`{oid, name, size, mime}`. The text keeps `@name` for display; the list is
what notifies. Each id must be a member, not removed, in the author's causal
past. `oid` is the sha256 of the content (64 lowercase hex digits), `size` is
a byte count no larger than `attachments.max_bytes` in the author's causal
past, and `name` and `mime` are required. Both lists ride with the body: a
body write replaces them (absent means empty), and the shown lists are the
winning body write's, so concurrent edits follow the (203) / (271) winner.
Only metadata is modelled; storing and fetching the content is a later
ledger ticket (337). Agent-authored writes may mention.

`agent-auto` is valid only on `item.status` and `item.close` for the author's
own assignment. It is always invalid on accept, decline, return, and handoff:
an agent never accepts or declines an assignment on its own.

### Assignments

An assignment record is `<assign event id>><member>` (for a handoff,
`<handoff event id>><member>`), with an assigner (`by`), a sender (who wrote
it: the assigner, or the handing-off assignee), a completion mode, and a state:

```text
pending --accept--> live --unassign / member removed--> unassigned
   |                  |--return-----> returned   (accepted ones only, by the assignee's direct lead)
   |--decline--> declined               |--handoff accepted--> handed --that handoff returned--> live (restored)
   |--withdraw / member removed / handoff's original ended--> withdrawn
forced (author, or someone the author reaches) starts live
```

- A **request** is no longer a separate kind or field: it is an item created
  with a pending assignment in the same write. *Received requests* lists
  pending assignments to me; the Items filter `request` lists items with a
  pending assignment.
- **Pending** assignments are excluded from item status and roll-ups. While
  pending, an assignment shows on the target's Received requests card and
  handle badge, and as "unaccepted" in their lead's My team view.
- **Accept** keeps the assigner and completion mode, so "assigner confirms"
  works for requests too (214). The accepter's direct lead at that moment gets
  a reference notice (My team card and handle badge) and may return it.
- **Handoff.** On accept the original assignee's assignment ends as `handed`
  and the original assigner gets a notice like the lead. The handed-on
  assignment keeps the original assigner, so confirm and unassign stay with
  them, and starts with the original's status and note (290). An admin may
  hand off any member's live assignment (286): the takeover path before a
  removal. If the accepter's lead returns an accepted handoff, the original
  assignment comes back live with its prior status (291).
- **Answer notices** (285). The sender of a pending assignment gets a notice
  when it is declined or, once accepted, returned; for a returned handoff the
  original assigner gets one too. Notices are derived from the assignment
  records, never events.
- `returned` and `declined` are terminal: the assigner may assign again.
- Completion mode (assignee closes / assigner confirms) stays per assignment.

### Derived state

- An assignment's status is `todo`, `doing`, `blocked`, `awaiting`, or `done`
  while live. An item's own status comes from its live assignments: all done
  means `done`, any blocked means `blocked`, then awaiting, then doing, then
  todo. With no live assignment it is `pending` (some pending), `declined` or
  `returned` (the latest one ended that way), or `unassigned`. Topics have
  none.
- Roll-up: a parent shows its children by status ("done 3/5 · blocked 1") and a
  "blocked below" warning when any descendant is blocked, all the way up to the
  root. Topics and items whose status is pending, declined, or returned are not
  counted. The parent's own status stays manual, set by its assignee.
- Priority displays as the level name; a stored value outside the current range
  displays as the top level. Stale (284) = a live assignment in `doing` or
  `blocked` with no status write for 3 days by the viewer's clock (the mock
  clock here). The threshold is a client-side constant (`STALE_DAYS`, or
  `deriveViewState(input, {staleDays})`), not shared data; todo work never
  goes stale. Overdue = due before
  today and not done.

## View state (the future backend API shape)

`store.view(stream, now)` returns one plain JSON object, built by
`deriveViewState(input, { stream, now, staleDays? })` plus the store's sync info. The view
reads nothing else, and writes only through `store.dispatch(stream, events,
ts)`, which returns each new event's id and verdict. This object is the shape
the Go data-structure child should serve.

```text
viewer       { member, stream, name, admin, lead, followers[], reach{id: depth}, isMember }
priority     { names[], default }
attachments  { max_bytes }
members      { id: { id, name, leads[], keys[], revoked[{key, by}], removed, joined, admin, children[], depth, role } }
admins[], rolesProblems[]
items        { id: { id, alias, kind, title, body, project, due, priority, priorityLevel, priorityName,
                     parent, children[], author, ts, hidden, edited{id, ts}, status, counted,
                     rollup{total, counts{status: n}, blocked[]}, lastActivity, started,
                     assignments[], pending[], thread[eventId], concurrent, depth, chain[],
                     mentions[memberId], attachments[{oid, name, size, mime}] } }
itemOrder[]  item ids in tree order
assignments  { id: { id, item, to, by, sender, completion, state, forced, status, note, noteBy, via, ts,
                     inherited, ruleLevels, handoffOf, handoffFrom, handoffNote, answerNote,
                     restored{id, ts}, restores, accepted{id, ts},
                     leadAtAccept, ended{id, type, ts, by}, depth, statusOrigin, statusBy,
                     lastUpdate, stale } }
comments     { id: { id, item, author, body, reply_to, ts, edited{id, ts}, hidden,
                     mentions[memberId], attachments[{oid, name, size, mime}] } }
events[]     display order: { id, alias, stream, member, seq, ts, type, origin, payload, status
                     (applied|flagged|waiting), reason, ignored, markers[{kind, field, other,
                     otherMember, winner, mine, theirs}], superseded[{field, by, why (later|human)}],
                     thread, parentThread, local, open, late }
notices[]    { key, kind (accepted|handoff|declined|returned), assignment, item, to, by, sender, ts,
               state, from?, endedBy?, note?, restores? }   keys acc:/ho:/dec:/ret: + assignment id
my           { assignments[], today{work[], pending[], confirms[]}, received{pending[], answered[], sent[]},
               given[itemId], created[itemId], team[{id, depth, open[], pending[], waiting[], done, blocked,
               stale, last}],
               badge[key] }
stats        { events, commits, flagged, concurrent }
heads        { stream: seq }   the seen a write would carry now
store        { name, label, remote }
sync         { online, syncedAt, upstream, unpushed[{open, ts, events[]}] }   the viewer's clone
clones       { member: [{ stream, online, unpushed }] }
```

Lists hold ids into the maps. Everything is per viewer clone: two clones of
one member can show different states until they sync. The notification card
reads only this object; its read state, like the badge's `seen`, is client
state outside it.

## Validation and concurrency

1. **Causal past.** An event's causal past is its stream's earlier events plus
   every event its commit's `seen` names, closed transitively. If `seen` names
   an event this clone has not received, the event is **waiting**: shown, not
   applied, and with no verdict yet.
2. **Order.** Events are put in one order that respects the causal past. Ties go
   by `ts`, then member id, then clone id, then seq. This is only for folding
   and display.
3. **Verdict.** Each event is checked against the state folded from its causal
   past only. That is the state its author had seen, never later arrivals. So a
   late arrival never changes the verdict of an event already shown.
4. **Final fold.** It applies every valid event leniently and marks concurrent
   changes. Two events are concurrent when neither is in the other's causal
   past.
   - **Field writes.** These are assignment status (written by the assignee
     with status / close and by the assigner with confirm / reject / reopen),
     a member's lead, the priority levels, the attachment limit, and item or
     comment text, due, and priority. Each field is a multi-value register; concurrent writes are all
     kept. The **display winner** is the later `ts`, then the greater member
     id, clone id, and seq. Writes by two different members get a
     "concurrent change" marker naming both values. Writes by one member from
     two clones (item and comment text have a single writer, so this is their
     only concurrency) get no marker: the later `ts` wins (271), except that a
     `human`-origin write beats a concurrent `agent` / `agent-auto` write of
     the same member (288). Among a field's concurrent writes, an agent write
     with a same-member human write beside it drops out first; the later `ts`
     rule picks among the rest. The losing same-member write stays in the
     thread with a quiet "superseded by <event>" note (289), not a conflict
     box.
   - **Removal against writes.** An unassign, return, withdraw, or accepted
     handoff that is concurrent with a status write or close on that
     assignment always wins. Both events are marked, and the write shows as
     valid when written but not in effect. The same applies to a subtask
     created under an assignment that was concurrently removed.
5. **Revocation.** `roles.revoke_key` cuts each of that member's streams at what
   the revoker had seen of it. Events signed with the revoked key with a seq
   above the cutoff are ignored, even when their `seen` predates the
   revocation, because a stolen key cannot pretend to be old. Events at or
   below the cutoff stay valid. This is the one rule that can turn an already
   shown event into ignored (210).

Flagged reasons, as the engine words them:

- the commit's stream does not match the ref it sits in
- a seq gap or duplicate (a broken event consumes no seq)
- unsigned, or a signer key that is not the author's in its causal past
- a key already revoked in its own causal past, or revoked later and above the
  revoker's cutoff
- author not a member, or removed
- `agent-auto` outside status or close, or on accept, decline, return, or
  handoff
- edits or hides by anyone other than the creator or comment author
- a due that is not a date, or a priority outside the author's current range
- assignment by a non-creator, on a topic, empty, already covered, or with
  every chosen person already assigned
- a handoff of a done or someone else's assignment, to a holder, or while one
  is pending
- accept or decline without a pending assignment (a handoff whose original
  ended is withdrawn, so it is no longer pending)
- return by anyone but the assignee's direct lead, or of a forced assignment
- status, close, or reopen by a non-assignee, before acceptance, or after the
  assignment ended
- status or close on a done assignment, or close while already awaiting
- confirm or reject by a non-assigner, when not awaiting, or when no
  confirmation is required
- a reply to a comment that is unseen, hidden, or in another thread
- a mention that is not a list of ids, or names someone who is not a member
  or was removed; mentions or attachments on an item edit without a body
- an attachment list that is malformed, an oid that is not a sha256, a
  missing name or mime type, a size that is not a byte count, or a size
  above `attachments.max_bytes`
- `roles.*` by a non-admin, a lead list over length 1, a cycle, an unknown
  member, a key in use, removing the last admin, invalid priority levels, or
  an attachment limit that is not a positive integer

## Fixtures (`team-alpha`)

Fourteen members in three levels. Ada is the team lead (admin). The part leads
are Maya (lena, omar, ivy, jon, kai), Raj (sven, tia, noa, and later pia), and
Bo (admin; uma). Default viewer is `maya.pc`; maya also has a `lap` clone. The
page clock starts at 2026-11-06 09:00. The fixtures show:

- threads with comments and quote replies; due dates and priorities on most
  tasks, with overdue, stale (doing or blocked for 3+ days) and blocked work
- an edit of `ada.pc-1`, and a flagged edit of it by maya (creator only)
- a hidden comment (`raj.pc-4`), plus a flagged hide by someone else
- reject (`maya.pc-12`), reopen (`maya.pc-18`), and unassign (`maya.pc-15`)
- pia's blocked "Cache layer" raising "blocked below" on `raj.pc-1` and
  `ada.pc-1`
- pia, added later, inheriting ada's live "Security training, everyone within
  L2"
- jon-k1 revoked by `bo.pc-6` after seeing jon up to seq 5. `jon.pc-6`, signed
  with jon-k1 and backdated with an old `seen`, is ignored. jon continues on
  jon-k2.
- omar long offline. His three late commits are pushed but fetched by no other
  clone; they arrive on that clone's next fetch (opening the drawer). They
  create a field conflict with maya's reject on "Shard writer docs" (omar's
  later `blocked` is the display winner) and a status report on the assignment
  maya concurrently unassigned (`omar.pc-5` vs `maya.pc-15`).
- maya edits the body of "Index builder" from `maya.lap` and, without having
  fetched it, from `maya.pc` later: the later edit is shown, both stay in the
  thread, no marker; `maya.lap-1` carries the "superseded" note (271, 289).
- maya sets "Ship search v2" to doing by hand on `maya.pc`; an hour later her
  agent on `maya.lap`, not having fetched it, auto-sets blocked: the human
  write wins despite the earlier ts, and `maya.lap-4` is superseded (288).
- assignments of every kind:
  - forced: maya's subtasks to her followers, ada's live "everyone within L1/L2"
  - pending, accepted, assigner confirms: `kai.pc-2` sent up to ada; ada
    accepts and closes, kai confirms (214)
  - pending, accepted, then returned: `sven.pc-3` to jon; jon accepts (maya
    gets a notice), maya returns it after a flagged return by bo
  - pending, declined: `tia.pc-3` to uma, with a comment
  - pending, open: `noa.pc-3` to lena, plus a flagged `agent-auto` accept; and
    `ivy.pc-6` to her own lead maya (on maya's Today card)
  - handoff: `lena.pc-2` to raj (pending, raj accepts, ada gets a notice), raj
    hands it to sven, sven accepts (lena, the assigner, gets a notice)
  - awaiting confirm: ivy's close of "Tokenizer" waits for maya
- mentions and attachments: lena's comment `lena.pc-6` on "Index builder"
  mentions maya and omar and attaches `index-layout-v2.png`; her next comment
  `lena.pc-7` attaches a 50 MiB trace and is flagged over the 10 MiB limit;
  bo's topic `bo.pc-9` mentions maya and raj in its body and attaches a PDF;
  `uma.pc-6` is flagged for mentioning `zed`, who is not a member; maya's
  reply `maya.pc-22` to `lena.pc-6` mentions lena and ivy. Together with the
  rest, the notification card shows every entry kind: maya's card alone has
  all but the answer notice, which sven's and tia's cards show.
- about a dozen flagged demos, one for most reasons above

`hana-personal` has one member and one clone with no remote. Its commits are
local (unpushed); todos are self-assigned in the same commit as their
creation, and it has a topic and a blocked subtask.

## Engine notes for the real implementation

- **Layers.** `openStore` mocks the protocol layer (per-clone refs, journal,
  push, fetch) that `261107-feat-ws-tracker-ledger-protocol` implements;
  `replay` and `deriveViewState` are the data-structure layer the Go child
  ports. The verdict, fold, and view-state rules are deterministic given the
  same commits and `now`.
- **Incremental apply [181].** Keep a cursor per stream and apply new events
  as they arrive. A late event re-replays only its thread. A roles change
  re-replays everything, because reach and membership feed every check. The
  mockup simply replays everything on each render, in O(n²) over the causal
  past, which is fine at fixture scale.
- **Ref layout [183].** Each clone's ref stores events under
  `events/YYYY/MM/DD.jsonl`.
- **No snapshots [184]** in v1.
- **Local clone [185].** A bare repo under `~/.ws/tracker/<store>/`, which is
  user data, not cache. The index is cache.

## Settled

| board | decision |
|---|---|
| 149 | The roles file is genesis. Later changes are admin `roles.*` events. |
| 150, 151 | Admins are a list of member ids. Several roots are allowed. |
| 157 | `roles.add_admin`, `remove_admin`, `revoke_key`, `remove_member`. Events signed before a revocation stay valid. |
| 147 | An event's author owns the ref it sits in. Admin takeover of a departed member's work is an event in the admin's own ref. |
| 148 | Everyone in a store sees everything. "Viewing as" changes only the perspective. |
| 154, 156 | One commit per write, several events. The commit is the signed unit, with no per-event `sig`. |
| 153, 171 | Event id `author#seq`, no global sequence, shown as `maya-12`. |
| 168 | Every commit records `seen`. It replaces timestamp merge order. |
| 167 | Validate against the author's seen state. Verdicts never flip on late arrival. Concurrent conflicts are shown with a deterministic display winner. |
| 168 | A revoked-key event that arrives after the revocation is ignored whatever its `seen` says. |
| 169 | No remote and long offline are the same path. The drawer shows last synced and unpushed writes. Sync failure is retried. |
| 170 | Deletion is a hide (tombstone) event. |
| 181-185 | Engine notes above. |
| 166 | Thread-centric items: one timeline, subtasks as child threads, a composer with quick status. |
| 175, 162 | Kinds `task` / `todo` / `topic`. One `item.create` plus `item.edit` by the creator. |
| 176, 177 | `comment.add`. The author edits or hides. Flat timeline with an optional `reply_to`. |
| 146 | Project reference is one optional free-text string. |
| 158 | "Everyone within N" stays live. |
| 159, 144 | Delegating down creates a subtask under one's own name. Only the creator assigns (an assignee may hand off, 273). A subtask's parent is assigned to or created by its author. |
| 145 | Completion mode per assignment event, default assignee closes. |
| 160 | `item.unassign`, `item.reopen`, `item.reject`. |
| 161 | Progress is a status `todo`/`doing`/`blocked`/`done` plus a note. No percentage. |
| 178 | The roll-up counts children by status and warns on blocked up to the root. The parent's own status stays manual. |
| 163 | Self-assignment for one's own work. "My assignments" gathers everything. |
| 155 | One `origin`: `human` / `agent` / `agent-auto`. |
| 186-189 | Peer requests to anyone. Pending requests are not counted. Accepting needs no approval, but the recipient's lead may return. An agent never accepts or declines. Requests became pending assignments by 279, 280. |
| 201 | A member whose key was revoked continues under the same id after `roles.add_key`. |
| 202 | Changing status while awaiting the assigner's confirm cancels the completion request. |
| 203 | Concurrent writes to one field: the display winner is the later ts, then member id, then seq. Both values are kept and shown. |
| 204 | An event whose `seen` names an unreceived event is waiting: no verdict until the gap fills. |
| 205 | A broken event (seq gap or duplicate, wrong ref, bad signature) consumes no seq. |
| 206 | `item.edit` also edits the project field. |
| 207 | Hiding an item that still holds someone else's active assignment is flagged. |
| 208 | An admin may act on a removed member's work. |
| 209 | A member can be removed only when they lead nobody and are not an admin. |
| 210 (Q1) | Revocation cuts at what the revoker had seen: the one sanctioned verdict flip, the same on every client. |
| 211 (Q2) | A victim's own events cut by a revocation are re-issued with the new key; the originals stay ignored. |
| 212, 230, 237 (Q3) | One ref per clone, `refs/heads/tracker/m/<member>/<clone>`; event id `<member>.<clone>#<seq>`; `seen` per clone stream. No one-device rule. Supersedes 153/171's `author#seq`. |
| 213 (Q4) | Unassign and return beat a concurrent status write or close. |
| 214 (Q5) | A requester may choose "requester confirms" when sending, as with assignment. |
| 215 (Q6) | A returned request stays returned; the requester sends again if needed. |
| 227 | Writes batch in a local journal and close into one signed commit when `seen` would change or at push. Supersedes 154's one commit per write. |
| 229 | Registering an empty repository creates the store; the registrant is sole admin. |
| 231, 232 | Self-join by an own-ref `member.join` event, first come per id; trust is repository access. Replaces the `roles.add_member` stand-in. |
| 233 | A person's PCs share one copied key file. |
| 234 | A deleted or rewound remote ref is refused and kept locally, with a manual restore push. |
| 235, 236 | Protocol layer first (`261107-feat-ws-tracker-ledger-protocol`): identity is protocol, leads and assignment reach are data structure, payloads are opaque to the protocol. |
| 241 | Own writes push after a 5-minute idle delay, at an explicit apply, or when the panel collapses. |
| 247 | Genesis is the single commit of `refs/heads/tracker/genesis`, never updated. |
| 268 | v3 models the Settled rows v2 still showed as v2: per-clone refs and streams (212, 230, 237) with a clone picker, "assigner confirms" for requests (214), and journal batching with an unpushed list (227). |
| 271 | One member's concurrent writes from two clones: the later ts wins by the (203) rule, the losing write stays in history, and no conflict marker is shown. |
| 272 c, 277 c | Items carry an optional `due` date and one integer `priority`, edited by the creator and merged by (203). Priority levels are store configuration in the roles file (`priority.names`, `priority.default`), changed by `roles.set_priority_levels`; default 알림 / 업무 / 긴급, default level 1. Larger is more urgent; a stored value outside the range shows as the top level. |
| 274 a | Only the creator edits an item, due and priority included; others comment. |
| 278 a | No labels or tags in v1; the `project` string stays the only grouping. |
| 273 b, 279 a, 280 a | Assignment is unified. `item.assign` may target anyone: someone the author reaches (lead chain) is forced and live; anyone else is pending until accept or decline. Pending assignments are excluded from status and roll-ups. On accept the accepter's direct lead gets a reference notice and may return it (`item.return`). A request is an item created with a pending assignment in the same write; `request.*` became `item.accept` / `item.decline` / `item.return`. An assignee hands off with `item.handoff` (pending to the new person; on accept the original ends and its assigner is notified). `all` stays forced-only. Completion mode stays per assignment. An agent never accepts or declines. |
| 275 | The view reads only one derived view-state object and writes only by dispatching events; its shape is in [View state](#view-state-the-future-backend-api-shape) as the future backend API. |
| 276 b | View modes: kanban (drag moves only one's own assignment), activity timeline, due timeline, and calendar, next to the cards and the tree. Styling is deferred. |
| 267 a, 281 a | A scenario walk through the page and engine rules, recorded in [SCENARIOS.md](SCENARIOS.md) with view and model snags. |
| 284 | Stale = a live assignment in doing or blocked with no status write for 3 days; the threshold is a client-side constant. |
| 285 | The sender gets a derived notice when their pending assignment is declined or their accepted one returned; no new event. |
| 286 | An admin may `item.handoff` any member's live assignment (pending until the new holder accepts), before removal. |
| 287 | `roles.remove_member` ends the member's live assignments as unassigned and pending ones as withdrawn. |
| 288 | Same member, concurrent writes from different clones with different origins: the human write wins over agent / agent-auto; same origin keeps (271). |
| 289 | The losing same-member write gets a quiet "superseded by <event>" note in history; no conflict box. |
| 290 | A handoff's new assignment starts with the original's status and note. |
| 291 | Returning an accepted handoff restores the original assignment with its prior status and notifies the original assigner. |
| 292 | Extends (208): an admin may `item.edit` and `item.hide` an item whose creator was removed. |
| 293 | Accepted as interpreted in v3: forced reach is any depth of the lead chain (`levels` dropped from `chosen`); (271) holds for every field; a kanban drop on done dispatches `item.close`; stale counts doing and blocked only; a viewer switch pushes the previous clone and fetches the new one; the push delay and join are not modeled. |
| 303 | View modes move from the drawer title to each card: the drawer has one layout (the cards layout) and each card offers only the modes that fit it (the table under [What the page shows](#what-the-page-shows)). Supersedes the drawer-level picker of (276) b while keeping its four modes, now scoped to a card's own rows and filters. The kanban drag rule is unchanged. |
| 304 a | A card in a wide mode (kanban, due timeline, calendar, activity timeline) spans the full row of the cards layout; the remaining cards flow below it. No horizontal scroll in the card and no maximize button for now. |
| 300 a, 302 | A chat card placeholder for the future daemon-side local support agent: placement and a static sample only (fixed height, internal scroll, newest at the bottom, a disabled composer), with no working chat, no fake responder, and no new events. |
| 305 | The chat sample mimics the dashboard conversation entries plus a tracker card reference entry that renders an item from the view state and opens its thread sheet. Each card's chosen mode is local UI state remembered per store, never an event. |
| 331 c, 332 a, 333 a, 334 | A *Notifications* card in the cards layout, list only, newest first, derived from the view state: new forced and pending assignments to me, answer and reference notices, completion or blocked of work I assigned, Confirm requests waiting on me, replies to my comments, my live assignments due today or overdue, and mentions of me. An entry opens the item's thread sheet; *Received requests* and *My team* keep their buttons. Unread entries are highlighted and cleared by a click or *Mark all read*; no event is written. The handle badge is unchanged. |
| 331 c, 335 | Item bodies and comments carry an optional explicit `mentions` list of member ids, validated against the author's causal past; the text keeps `@name`. Mentions show in the mentioned member's notification card and as chips in the thread; the composer has a simple `@` picker. Agent-authored writes may mention. |
| 336 | Mentions, attachments, and the attachment limit are ported to the Go domain layer (`agents-plugin-tool/internal/wstracker`) in the same change, with golden parity; the ledger layer is untouched. |
| 337 a | Attachment content is stored as content-addressed blobs under `attachments/<sha256>` in the member's ledger tree and fetched on demand through a partial clone. The storage itself is deferred to a later ledger ticket; v3 models metadata only. |
| 338 a, 339 a | Item bodies and comments carry `attachments: [{oid, name, size, mime}]` with `oid` the content's sha256. The roles file's `attachments.max_bytes` (default 10 MiB) is changed by the admin `roles.set_attachment_limit`; an attachment above it is flagged with a reason. The thread shows a chip with name and size and a simulated loaded state (local UI only). |
| 340 | In the product the notification read state lives per clone on the local filesystem outside git, read and written by the daemon; the mockup keeps it in memory. |

## Open questions

The model snags the scenario walk found are resolved by (284)-(292); see
[SCENARIOS.md](SCENARIOS.md). (295) settled the last one: when the original of
a pending handoff ends another way (unassign, removal, return, or a
concurrent second handoff accepted), the pending handoff ends as `withdrawn` with no event, its
`ended` pointing at the event that ended the original. An unassign, removal,
or return of the original concurrent with an accept of its handoff wins, as
removal beats writes (213): the accepted handoff ends `withdrawn`, the original
takes the ending event's state, and both events are marked. None is open now.

## Not modeled in v3

Real signatures and crypto; git refs, pushes, and fetches (sync is simulated
per clone); the background poll and the 5-minute push delay (push happens at
Sync, panel collapse, and viewer switch); the self-join (stand-in:
`roles.add_member`) and claim refs; re-issue of a revoked key's own events;
search; the MCP surface; notifications beyond the handle badge, the derived
notices, and the notification card (no push, mail, or OS notification; read
state is in memory, not on disk (340)); attachment content storage and fetch
(337); labels and tags (278 a); multiple leads; snapshots; a
working support-agent chat (the chat card is a static placeholder); styling.
