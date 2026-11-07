# Tracker mockup v2

A layout and data-model mockup for the team tracker (epic
`261107-epic-ws-personal-common-store`), inside a frame that imitates the
dashboard session page. Everything the page shows is computed in the browser
from per-member commit lists on top of a roles file: each member appends only to
their own ref, and every view is a client-side merge. v1 settled the frame and
the cards. v2 applies the board decisions of 2026-11-07, listed in
[Settled](#settled). Items are threads, progress is a status, commits carry
`seen`, concurrent changes are shown, and members can send each other peer
requests.

## Open it

Open `index.html` from the file system (double-click, or a `file://` URL). No
build, no server, no dependencies.

- `index.html` holds the frame and mount points. `style.css` holds the neutral
  layout styling, with the dashboard's variables and drawer geometry.
- `engine.js` is the data layer (`window.TrackerEngine`). It computes causality
  from `seen`, validates each event against what its author had seen, folds the
  state, marks concurrent changes, and derives roll-ups.
- `app.js` is the view and the actions.
- `fixtures.js` assigns `window.TRACKER_FIXTURES`, because `fetch` of local JSON
  is blocked on `file://`. Edit it to keep a data change.

## What the page shows

- **Frame.** A header like the dashboard's, with the mockup's "viewing as"
  switch. The switch labels each member root, lead, or member. A static
  conversation sits in the middle and a pull-down drawer has one handle per
  store. A handle badge counts new assignments and new pending requests for the
  viewer, and opening the store clears it. A handle shows ⊘ when the store is
  offline or has no remote.
- **Store header.** Sync state: online or offline, last synced, "N of my writes
  not yet pushed", and upstream commits not fetched yet (mock). The buttons are
  `Sync now` and `Go offline` / `Come back online`. While offline, new commits
  are held locally (`not pushed`). Coming back online pushes them and fetches the
  fixture's late commits. The personal store has no remote, which is the same
  path as a long offline.
- **Cards.**
  - *My assignments*: everything assigned to me, from a lead, self-assigned, or
    an accepted request. Each row shows status, source, completion mode, the
    roll-up and blocked warning, the latest note, the last update, and quick
    actions.
  - *Received requests*: pending requests to me (Accept / Decline / Discuss),
    the ones I answered, and the ones I sent.
  - *Assigned by me*: what I handed to others, with each assignee's status and
    Confirm / Reject.
  - *My team*: shown only to a viewer with followers. It covers each follower
    down to 3 levels, as an indented tree, with everything they work on whoever
    assigned it. That includes status chips, notes, blocked warnings, last
    update, and pending requests they have not accepted.
  - *Items*: the tree of threads, filtered by kind (`request` included) and
    status (open / blocked / done / hidden).
  - *Team*: the org tree, admins, joined members, revoked keys, and an admin
    roles-event dialog.
  - *Activity*: the merged log in causal order, newest first, with filters for
    flagged-only and concurrent-only.
  - *Data*: the raw roles file, each ref's commit list, and the upstream (late)
    commits, all editable. `apply` recomputes everything and `reset` restores the
    fixtures.
- **Thread view** (side sheet). It holds the meta and roll-up, the body, item
  actions, the assignees with their actions, and the subtask threads. Below
  that comes one chronological timeline:
  - comments, with quote reply, edit, and hide
  - status changes, assignments, edits, closes, and request answers
  - agent records, with `agent` / `auto` chips
  - subtask lines
  - flagged and waiting events with their reason
  - amber "concurrent change" boxes that name both values and the shown winner

  The composer at the bottom posts a comment, as a reply when one is chosen,
  with an origin. When the viewer holds an assignment on the thread, quick
  status buttons appear that use the composer text as the note.
- **Actions** append one commit to the viewer's ref, holding one or more events,
  then report each event as applied or flagged. The actions are create (any
  kind, optionally a request, optionally self-assigned in the same commit),
  edit, comment, reply, edit or hide own comment, assign (everyone or chosen
  within N, completion mode), split into a subtask, status, close, confirm,
  reject, unassign, reopen, hide item, request accept / decline / return, and
  every `roles.*` event. Actions are not pre-validated, so you can produce
  flagged events on purpose.

## Data model

### Roles file (genesis)

```json
{ "admins": ["ada", "bo"],
  "members": [ { "id": "maya", "name": "Maya Ortiz", "keys": ["maya-k1"], "leads": ["ada"] } ] }
```

`admins` lists member ids. A member with `leads: []` is a root, and several
roots are allowed. `leads` is a list validated to length at most 1. Later
changes are admin `roles.*` events, folded in causal order.

### Commit

```json
{ "author": "maya", "ts": "2026-11-04T10:00:00Z", "signer": "maya-k1",
  "seen": { "ada": 13, "lena": 4, "omar": 3 },
  "events": [ { "seq": 15, "type": "item.unassign", "payload": { "item": "maya#1", "assignee": "omar" } },
              { "seq": 16, "type": "comment.add", "payload": { "item": "maya#1", "body": "..." } } ] }
```

- One commit per write, holding one or more events. The commit is the signed
  unit, so the mock carries `signer` (a key id) on the commit. Events carry no
  signature.
- `seq` is per author, continuous across commits. Event id is `author#seq`,
  shown as `maya-15`. An item's id is the id of its `item.create`, and a
  comment's id is the id of its `comment.add`. There is no global sequence.
- `seen` maps each other member to the last seq the author had received when
  writing. The author's own earlier events are always seen.
- `ts` is informational. It is used for display, as a tie-break, and for the
  display winner.
- An event has an optional `origin`: `human` (default), `agent`, or
  `agent-auto`.
- Fields starting with `_` are mock bookkeeping, not model. `_local` marks a
  commit not pushed yet, and `_received: "late"` marks a commit that arrived
  late.

### Event types

| type | payload | rule |
|---|---|---|
| `item.create` | `kind` (`task`/`todo`/`topic`), `title`, `body?`, `project?`, `parent?`, `request?` | A `parent` must be assigned to or created by the author. `project` is one free-text string. A `request` names a recipient (see Requests). Topics cannot be requests. |
| `item.edit` | `item`, any of `title`/`body`/`project` | Creator only. |
| `item.hide` | `item` | Creator only. A tombstone: the content stays in history. Flagged while someone else holds an open assignment on the item. |
| `item.assign` | `item`, `mode` (`all`/`chosen`), `levels`, `people?`, `completion` (`assignee`/`assigner`, default `assignee`) | Creator only. An admin may also assign an item whose creator was removed. Targets must be within N levels below the author, or the author (self-assignment). `all` stays live: anyone who later enters the range inherits it. Not allowed on topics or requests. |
| `item.unassign` | `item`, `assignee?` | By the assigner. An admin may also unassign when the assignee or assigner was removed. Without `assignee`, it withdraws all of the author's assignments on the item and ends the live rule. |
| `item.status` | `item`, `status` (`todo`/`doing`/`blocked`), `note?` | Assignee only. The creator self-assigns first. Allowed while awaiting confirmation, which withdraws the close. Not allowed on done. |
| `item.close` | `item`, `note?` | Assignee. Moves to `done`, or to `awaiting` when completion is `assigner`. |
| `item.confirm` / `item.reject` | `item`, `assignee`, `note?` | Assigner, on an `awaiting` assignment. Moves to `done` / back to `doing`. |
| `item.reopen` | `item`, `assignee?`, `note?` | The assigner or the assignee reopens a `done` assignment, back to `doing`. |
| `request.accept` / `request.decline` | `item`, `note?` | Recipient, on a pending request. |
| `request.return` | `item`, `note?` | Only the recipient's direct lead, on an accepted request. Cancels the resulting assignment. |
| `comment.add` | `item`, `body`, `reply_to?` | Anyone in the store. `reply_to` is one comment in the same thread (flat timeline, quote reply). |
| `comment.edit` / `comment.hide` | `comment`, `body` (edit) | Comment author only. Hide is a tombstone. |
| `roles.add_member` | `member` (`id`, `name`, `keys`, `leads`) | Admin. Stands in for join request plus approval. |
| `roles.set_lead` | `member`, `leads` | Admin. Length at most 1, no cycle. |
| `roles.add_admin` / `roles.remove_admin` | `member` | Admin. The last admin cannot be removed. |
| `roles.add_key` / `roles.revoke_key` | `member`, `key` | Admin. Revocation rule below. |
| `roles.remove_member` | `member` | Admin. The member must lead nobody and must not be an admin. Their earlier events stay valid. |

`agent-auto` is valid only on `item.status` and `item.close` for the author's
own assignment. It is always invalid on `request.*`, because an agent never
accepts or declines a request on its own.

### Requests

- A request is a task or todo thread whose `item.create` names a recipient in
  `request`. Anyone may request anyone, including leads and people above them,
  with no org-tree check. This field form was chosen over a separate
  `request.create` event, so a request is an ordinary thread from its first
  event.
- While **pending**, a request shows on the recipient's Received requests card
  and handle badge, and in their lead's My team view. It is excluded from
  roll-up counts.
- **Accept** makes it the recipient's own assignment, with self-assignment
  semantics: assigner equals assignee, completion `assignee` (the recipient
  closes). The requester did not assign it and has no confirm step.
  **Decline** closes it as declined. Questions and discussion are comments in
  the thread.
- **Return** comes from the recipient's direct lead. It needs no approval up
  front, and it removes the assignment. `returned` is terminal: the requester
  can comment or send a new request.

### Derived state

- An assignment's status is `todo`, `doing`, `blocked`, `awaiting`, or `done`.
  An item's own status comes from its active assignments: all done means
  `done`, any blocked means `blocked`, then awaiting, then doing, then todo,
  and no assignment means `unassigned`. A request that is not accepted shows
  its request state. Topics have none.
- Roll-up: a parent shows its children by status ("done 3/5 · blocked 1") and a
  "blocked below" warning when any descendant is blocked, all the way up to the
  root. Topics and requests that are not accepted are not counted. The parent's
  own status stays manual, set by its assignee.

## Validation and concurrency

1. **Causal past.** An event's causal past is its own earlier events plus every
   event its commit's `seen` names, closed transitively. If `seen` names an
   event this client has not received, the event is **waiting**: shown, not
   applied, and with no verdict yet.
2. **Order.** Events are put in one order that respects the causal past. Ties go
   by `ts`, then author id, then seq. This is only for folding and display.
3. **Verdict.** Each event is checked against the state folded from its causal
   past only. That is the state its author had seen, never later arrivals. So a
   late arrival never changes the verdict of an event already shown.
4. **Final fold.** It applies every valid event leniently and marks concurrent
   changes. Two events are concurrent when neither is in the other's causal
   past.
   - **Field writes.** These are assignment status (written by the assignee
     with status / close and by the assigner with confirm / reject / reopen),
     a member's lead, and item or comment text. Item and comment text has a
     single writer, so it cannot conflict across authors. Each field is a
     multi-value register. Concurrent writes are all kept and both get a
     "concurrent change" marker naming both values. The **display winner** is
     the later `ts`, then the greater author id, then the greater seq.
   - **Removal against writes.** An unassign or request return that is
     concurrent with a status write or close on that assignment always wins:
     the assigner or lead decided. Both events are marked, and the write shows
     as valid when written but not in effect. The same applies to a subtask
     created under an assignment that was concurrently removed.
5. **Revocation.** `roles.revoke_key` cuts at what the revoker had seen of that
   member. Events signed with the revoked key with a seq above the cutoff are
   ignored, even when their `seen` predates the revocation, because a stolen key
   cannot pretend to be old. Events at or below the cutoff stay valid. This is
   the one rule that can turn an already shown event into ignored (open
   question 1).

Flagged reasons, as the engine words them:

- the commit author does not own the ref
- a seq gap or duplicate (a broken event consumes no seq)
- unsigned, or a signer key that is not the author's in its causal past
- a key already revoked in its own causal past, or revoked later and above the
  revoker's cutoff
- author not a member, or removed
- `agent-auto` outside status or close, or on a request answer
- edits or hides by anyone other than the creator or comment author
- assignment by a non-creator, outside reach, on a topic or a request, empty,
  or already covered
- status, close, or reopen by a non-assignee, or after unassignment
- status or close on a done assignment, or close while already awaiting
- confirm or reject by a non-assigner, when not awaiting, or when no
  confirmation is required
- request answers by a non-recipient or when not pending
- return by anyone but the recipient's direct lead, or when not accepted
- a reply to a comment that is unseen, hidden, or in another thread
- `roles.*` by a non-admin, a lead list over length 1, a cycle, an unknown
  member, a key in use, or removing the last admin

## Fixtures (`team-alpha`)

Fourteen members in three levels. Ada is the team lead (admin). The part leads
are Maya (lena, omar, ivy, jon, kai), Raj (sven, tia, noa, and later pia), and
Bo (admin; uma). Default viewer is maya. The fixtures show:

- threads with comments and quote replies
- an edit of `ada-1`
- a hidden comment (`raj-4`), plus a flagged hide by someone else
- reject (`maya-12`), reopen (`maya-18`), and unassign (`maya-15`)
- pia's blocked "Cache layer" raising "blocked below" on `raj-1` and `ada-1`
- pia, added later, inheriting ada's live "Security training, everyone within
  L2"
- jon-k1 revoked by `bo-6` after seeing jon up to seq 5. `jon-6`, signed with
  jon-k1 and backdated with an old `seen`, is ignored. jon continues on jon-k2.
- omar long offline. His three commits sit upstream and arrive on the next
  sync. They create a field conflict with maya's reject on "Shard writer docs"
  (omar's later `blocked` is the display winner) and a status report on the
  assignment maya concurrently unassigned (`omar-5` vs `maya-15`).
- peer requests:
  - `sven-3` to jon: accepted, then returned by maya, after a flagged return by
    bo
  - `tia-3` to uma: declined, with a comment
  - `noa-3` to lena: pending, plus a flagged `agent-auto` accept
  - `kai-2`, sent upward to the team lead ada: accepted and closed
- about a dozen flagged demos, one for most reasons above

`hana-personal` has one member with no remote. Its todos are self-assigned in
the same commit as their creation, and it has a topic and a blocked subtask.

## Engine notes for the real implementation

- **Incremental apply [181].** Keep a cursor per member ref and apply new events
  as they arrive. A late event re-replays only its thread. A roles change
  re-replays everything, because reach and membership feed every check. The
  mockup simply replays everything on each render, in O(n²) over the causal
  past, which is fine at fixture scale.
- **Ref layout [183].** Each member's ref stores events under
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
| 159, 144 | Delegating down creates a subtask under one's own name. Only the creator assigns. A subtask's parent is assigned to or created by its author. |
| 145 | Completion mode per assignment event, default assignee closes. |
| 160 | `item.unassign`, `item.reopen`, `item.reject`. |
| 161 | Progress is a status `todo`/`doing`/`blocked`/`done` plus a note. No percentage. |
| 178 | The roll-up counts children by status and warns on blocked up to the root. The parent's own status stays manual. |
| 163 | Self-assignment for one's own work. "My assignments" gathers everything. |
| 155 | One `origin`: `human` / `agent` / `agent-auto`. |
| 186-189 | Peer requests to anyone. Pending requests are not counted. Accepting needs no approval, but the recipient's lead may return. An agent never accepts or declines. |
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

## Open questions

None. The six questions v2 raised were settled as 210-215 above (Q1-Q6).
The page still models the v2 behavior for 212 (one ref per member), 214 (no
requester confirm), and 227 (one commit per write); the Settled rows are
authoritative.

## Not modeled in v2

Real signatures and crypto; git refs, pushes, and fetches (sync is simulated);
the background poll; join requests (stand-in: `roles.add_member`); search; the
MCP surface; notifications beyond the handle badge; due dates, labels, and
priorities; multiple leads; snapshots; per-viewer local machines (the offline
toggle and unpushed commits belong to the page, not to the selected viewer).
