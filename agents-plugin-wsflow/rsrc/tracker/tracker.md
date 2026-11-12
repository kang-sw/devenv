---
kind: print
---

# Tracker

The tracker holds the user's tasks and topics in the stores the user
registered on this machine: personal stores, and team stores shared through a
git repository. Only a top-level lead and a store's support conversation call
the tracker tools; workers and subagents report progress to their lead. A
support conversation writes to its own store alone, and only what the user
asks for in its chat: the unasked recording below is a lead's, and the
store commands (`tk:register`, `tk:sync`, `tk:unregister`, `tk:relabel`,
`tk:hide`, `tk:unhide`) are refused to it.

## Calls

- `{{.McpNamespace}}/tracker.query(session_key, payload?, format?)` changes no
  store. With no `store` it lists the registered stores, each with the user's
  open work, and the saved profiles.
- `{{.McpNamespace}}/tracker.update(session_key, commands)` applies one batch
  and prints, per command position, the ID of the event it wrote. The ID a
  `tk:item.create` or `tk:comment.add` prints is the new item's or comment's
  ID for later batches.
- `{{.McpNamespace}}/wiki.query` and `{{.McpNamespace}}/wiki.update` read and
  edit a store's wikis, markdown pages kept on git branches of the store.
  Read a page with `wiki.query` `op: "read"` before saving it, and pass the
  commit that read returned as the save's `base`. A link
  `[text](wiki:<name>/<path>)` names the page `<path>` (no `.md` suffix) of
  the wiki `<name>` in the store whose item, comment, or page holds the link:
  call the wiki tools with that store, `wiki: "<name>"`, and
  `path: "<path>"`. Item bodies, comments, and wiki pages may link pages that
  way.

## Recording

- Record progress unasked only on an item assigned to the user: when the work
  it names starts, blocks, or finishes, write `tk:item.status` or
  `tk:item.close` with `origin="agent-auto"`. Write anything else only when
  the user asks.
- Write titles, bodies, notes, and comments in `workflow.lang` when set,
  otherwise in the language the user writes in.

## Query

```text
tracker.query payload: {store?, scope?, item?, kind?, status?, project?, text?, q?}
```

- `scope`: `mine` (the default: the user's assignments, the items the user
  created and assigned to nobody, then pending take requests the user may
  answer or filed), `received` (assignments sent to the
  user, the user's answers, and the ones the user sent), `assigned` (items the
  user assigned or handed to others), `team` (the work of the members the user
  leads, by person), `related` (the items the user is CC'd on, the latest CC
  first, then the other items whose body or a comment mentions the user, the
  newest mention first; both leave out done, dropped, and hidden items and the user's
  own: created or assigned), or `all` (every item, in tree order).
- `item`: one item in full, with its assignments, opening, take requests,
  children, and comments.
- `kind`, `status`, `project`, and `text` (part of the title, body, project,
  or ID) narrow the rows. `status` is `open` (neither done nor dropped),
  `hidden` (the only way hidden items show), or a status the rows print:
  `idea`, `todo`, `doing`, `blocked`, `in-review`, `done`, `dropped`, or
  `pending` (`awaiting`, the older name of `in-review`, is accepted for it).
  `idea` is a task every holder keeps before todo; like `pending` and
  `dropped`, it is left out of progress roll-ups.
- `q` is a search in the grammar of the dashboard's Items search box, so
  the tool and the screen list the same items; it is AND-ed with the fields
  above, and `text` keeps its plain substring meaning. Tokens are separated
  by spaces and all must match; keys and values ignore case:
  `status:<status>` (the values of `status` above; the item's status, also
  in `mine`, whose rows are assignments), `assignee:<member>` (a live or
  pending assignee whose member ID or name contains it), `cc:<member>` (a
  CC'd member whose ID or name contains it; `cc:me` is the user, even beside
  a member whose name contains "me"), `kind:task` or
  `kind:topic`, `project:<project>` (exact), `label:<label name>` (exact),
  `due:<date>` and
  `priority:<level>` (equality, or after `<`, `>`, `<=`, or `>=`; dates are
  `YYYY-MM-DD`; an item without a due date never matches a `due:` token),
  `is:claimable` (an open task the user may claim now).
  A word or `"quoted phrase"` matches the title, body, project, ID, alias,
  or the text of the item's shown comments. A token with another key (`foo:bar`, a URL)
  is plain text; a listed key with a malformed value (`due:<tomorrow`) is
  refused, naming the token.
- `format: "json"` with `store` alone returns the store's full state as JSON
  (members, keys, events, every item), for what the text omits; without
  `store`, the store list as JSON.

## Commands

Every tag carries the prefix `tk:` and follows the command syntax rules.
`tk:store` is the group: its `name` names the store for every command
inside it, which is every command except the store commands `tk:register`,
`tk:sync`, `tk:unregister`, `tk:relabel`, `tk:hide`, and `tk:unhide`. A
batch writes to one store, so it holds one `tk:store` group. A store command
cannot be rolled back, so each stands alone in its batch, outside any group.

| command | attributes | content |
|---|---|---|
| `tk:store` | `name` | group of every command below but the store commands |
| `tk:item.create` | `kind`, `title`, `project?`, `labels?`, `due?`, `priority?`, `parent?`, `after?`, `mentions?`, `refs?` | the body |
| `tk:item.edit` | `item`, `title?`, `project?`, `labels?`, `due?`, `priority?`, `mentions?`, `refs?`, `clear?`, `base?` | a new body |
| `tk:item.hide` | `item` | none |
| `tk:item.move` | `item`, `parent`, `after?` | none |
| `tk:item.assign` | `item`, `people?` and `rules?` (one or both), or `levels`; `completion?` | none |
| `tk:item.handoff` | `item`, `to`, `assignee?` | a note |
| `tk:item.accept`, `tk:item.decline` | `item` | a note |
| `tk:item.return` | `item`, `assignee` | a note |
| `tk:item.unassign` | `item`, `assignee?`, or `root?` and `rule?` | none |
| `tk:item.status` | `item`, `status`, `assignee?` | a note |
| `tk:item.close` | `item` | a note |
| `tk:item.confirm`, `tk:item.reject` | `item`, `assignee` | a note |
| `tk:item.reopen` | `item`, `assignee?` | a note |
| `tk:item.open` | `item`, `scope` | none |
| `tk:item.unopen`, `tk:item.claim` | `item` | none |
| `tk:item.request` | `item`, `assignee` | a note |
| `tk:item.approve`, `tk:item.deny` | `item`, `request` | a note |
| `tk:item.unrequest` | `item`, `request` | none |
| `tk:item.cc`, `tk:item.uncc` | `item`, `member` | none |
| `tk:comment.add` | `item`, `reply_to?`, `mentions?`, `refs?` | the comment |
| `tk:comment.edit` | `comment`, `mentions?`, `refs?` | the new text |
| `tk:comment.hide` | `comment` | none |
| `tk:member.profile` | `org?`, `family?`, `given?`, `member?` | none |
| `tk:roles.set_lead` | `member`, `leads` | none |
| `tk:roles.add_admin`, `tk:roles.remove_admin`, `tk:roles.remove_member` | `member` | none |
| `tk:roles.add_key`, `tk:roles.revoke_key` | `member`, `key` | none |
| `tk:roles.set_priority_levels` | `names`, `default` | none |
| `tk:roles.set_attachment_limit` | `max_bytes` | none |
| `tk:roles.add_label` | `name`, `color` | none |
| `tk:roles.edit_label` | `label`, `name?`, `color?` | none |
| `tk:roles.remove_label` | `label` | none |
| `tk:register` | `store`, `member`, `remote?`, `label?`, `org?`, `family?`, `given?` | none |
| `tk:sync` | `store?` | none |
| `tk:unregister` | `store`, `confirm?` | none |
| `tk:relabel` | `store`, `label` | none |
| `tk:hide`, `tk:unhide` | `store` | none |

```xml
<tk:store name="team">
<tk:item.create kind="task" title="Parser" due="2026-11-20">
Body text, markdown, written as is.
</tk:item.create>
<tk:item.assign item="$1" people="maya"/>
<tk:item.status item="ada.pc-4" status="doing" origin="agent-auto">Started</tk:item.status>
</tk:store>
```

- `item`, `parent`, `after`, `reply_to`, `comment`, `request`, and each
  `refs` entry: an ID as the query or an update prints it, or `$N` for the
  item, comment, or take request that command `N` (counting from 1) of the
  same batch creates.
- `after`: the sibling (same parent) the new or moved item goes right
  after; `after=""` puts it first; without `after` it goes last.
- `tk:item.move` places an existing item, with its subtree, under `parent`
  (`parent=""`: top level); a move under the same parent reorders it. Move a
  misplaced item rather than recreate it, which loses its comments,
  assignments, and history.
- `refs`: the items the text references, each written `#<ID>` in the text.
  `mentions` and `refs` ride with the body: a new body replaces both, so an
  edit that keeps them repeats them.
- `kind`: `task` (work to assign and track) or `topic` (a discussion with
  no status, held by one member who looks after it).
  `due`: `YYYY-MM-DD`. `priority`: a level number from the store view's
  priority line, larger is more urgent; without one an item gets the store
  default. A note is optional.
- `labels`: the item's labels, label IDs from the store view's labels line
  (shown once the store has labels), or `$N` for the label that
  `tk:roles.add_label` command `N` of the same batch adds; an edit replaces the item's labels, and `labels=""`
  removes them. Labels come only from the store's list.
- A task always has a holder: while nobody else holds it, its creator does
  (an admin when the creator was removed), at todo, and that holder sets its
  status and closes it. Assigning anyone, the creator included, replaces
  that holding; when the last assignment ends, the creator holds it again
  at todo. It cannot be unassigned, handed off, or returned; assign or open
  the item instead.
- `status`: `idea` (before todo, not counted in progress), `todo`, `doing`,
  `blocked`, `in-review` (asks for review before done), `done`, or
  `dropped` (set aside: like done it is wrapped up, and it is not counted in
  progress; an item whose assignments are all dropped shows dropped). To
  create a task straight into idea, follow its
  `tk:item.create` with `tk:item.status item="$N" status="idea"` in the
  same batch.
- A topic the batch creates is assigned to the user unless the same batch
  assigns it with `tk:item.assign item="$N"`; the update prints that
  assignment under the create's position.
- People are member IDs; `people`, `rules`, `mentions`, `refs`, `labels`,
  `leads`, `names`, `clear`, and `base` are space-separated.
- Members form a lead tree; each member has at most one lead.
  `tk:roles.set_lead` sets `member`'s lead (`leads=""` for none).
- Assignment lifecycle:
  - `tk:item.assign` (the item's creator): `people` assigns those members,
    live at once for the user and the members the user leads at any depth,
    and pending for anyone else until they `tk:item.accept` or
    `tk:item.decline`. `rules` assigns subtrees: `member:N` is everyone up to
    N levels below `member`, `member:*` everyone below, and `member+:`
    includes `member` (`rules="maya+:1 raj:*"`). A rule covers anyone who
    later joins its range. One rooted at the user or someone the user leads
    is live at once; one rooted at anyone else is a rule request pending to
    that member, live once they accept. `levels="N"` alone is the rule
    `<user>:N`.
  - The holder of a live assignment sets its `tk:item.status` (any status
    but `done`) and closes it with `tk:item.close`. With
    `completion="assigner"` a close puts it in `in-review`; the holder may
    also enter `in-review` on any assignment. The assigner or any org
    superior of the holder resolves an `in-review` with `tk:item.confirm`
    (done) or `tk:item.reject` (back to doing), naming the holder as
    `assignee`.
  - Without `completion`, it is filled in and stored by who each target is:
    `assigner` for a member the user leads at any depth and for a rule
    rooted at the user or at such a member (`levels` too), `assignee` for
    the user and anyone else. Targets of both kinds are written as two
    assigns; the update prints the second as split off.
  - `tk:item.status` with `assignee` writes on that member's live
    assignment: their org superior (any depth) sets `blocked`, `done`
    (final, no review), or `dropped`; its assigner, when not their
    superior, sets only `dropped`. The holder drops their own only under
    `completion="assignee"` or when they assigned it. A done or dropped
    assignment takes no status, handoff, take request, or approval of a
    pending one until reopened.
  - `tk:item.handoff` passes the user's live assignment to `to`, pending
    until `to` accepts; an admin hands off another member's by naming them as
    `assignee`.
  - `tk:item.return` (the holder's direct lead) ends an accepted assignment;
    returning an accepted rule request ends its rule.
    `tk:item.unassign` (the assigner) withdraws `assignee`'s assignment, ends
    the user's rules rooted at `root`, or, without either, withdraws all of the
    user's assignments and rules on the item. `rule` names one rule (its ID
    follows the rule in the item view) and ends only that one, withdrawing it
    while it is a request.
    `tk:item.reopen` (the assigner or the holder) reopens a done or dropped
    one.
  - A topic has one holder: its `tk:item.assign` names exactly one person in
    `people`, and is refused while anyone holds it or a handoff of it is
    pending. It takes no `tk:item.status` or `tk:item.close`. Its holder
    releases it with `tk:item.unassign` naming themselves as `assignee`, or
    hands it off.
- Taking work:
  - Assign a task when the creator knows who should do it; open it when
    anyone in a group may take it. `tk:item.open` (the creator) offers a task
    nobody else holds for claiming: `scope="team"` to the creator's org
    subtree, `scope="store"` to every member. `tk:item.unopen` withdraws the
    offer; an assign also ends it.
  - `tk:item.claim` takes an open task the user may claim (its row prints
    `claimable by me`; `q: "is:claimable"` lists them) as the user's live
    assignment, with no approval.
  - `tk:item.request` asks for the live assignment `assignee` holds; a task
    held only by its creator is claimed once opened, not requested. The
    item view lists its take requests with their IDs. The holder of that
    assignment or the item's creator answers with `tk:item.approve`, which
    moves the assignment to the requester, or `tk:item.deny`; the requester
    withdraws with `tk:item.unrequest`. A request lapses when the assignment
    it names ends.
  - Two claims of one opening, or two approvals for one assignment, written
    before either reached the other: the first in the store's event order
    wins, and the later is flagged with a reason naming what took it first.
- CC: `tk:item.cc` adds `member` to an item's CC list and `tk:item.uncc`
  removes them; any member may add or remove anyone, on any item that is not
  hidden, whoever may edit it. A CC'd member follows the item (the `related`
  scope, and the dashboard notifies them of its status changes, closes, and
  comments) but gets no assignment, status, or completion; assigning stays
  the creator's. The item view prints its `cc:` line.
- `clear` on `tk:item.edit`: `body`, `due`, or both
  (`clear="body due"`); an absent body leaves the body as it is.
- `tk:item.edit` (the item's creator or a live assignee) writes the whole
  new body. Set `base` to the item's `body revision` line from the query
  you edited from, and the store merges it line by line with the edits
  others made since: where both changed the same lines the later write wins
  and both are marked as a conflict. Without `base` the body is replaced,
  dropping those edits.
- `origin`: `agent` (the user asked; the default) or `agent-auto` (recorded
  unasked, valid only on `tk:item.status` and `tk:item.close` of the user's
  own assignment; see Recording).
- `tk:roles.*` and another member's `tk:member.profile` are admin commands.
  `key`: an SSH public key line. `names`: the level names, lowest first;
  `default`: a level number. `max_bytes`: a byte count. The priority levels
  and the attachment limit are store settings: an admin changes them in the
  store settings of the dashboard's Team tab, or with
  `tk:roles.set_priority_levels` and `tk:roles.set_attachment_limit`.
- The label list is a store setting too, kept the same way. `name` is
  unique in the list, ignoring case; `color` is `#rrggbb`. `label` is a label
  ID from the labels line, or `$N`. A rename or recolour shows on every item
  that carries the label; a removed label leaves every item and `label:`,
  and adding its name again makes a new label that those items do not get
  back.
- The store checks every command against the user's role and the item's
  state. If it refuses one, nothing is written and the refusal gives the
  reason: correct the batch and resend, or tell the user when the user's role
  does not allow it.
- `tk:register`: with `remote`, joins the store that repository holds, or
  creates one when it is empty; without `remote`, creates a personal store.
  `store` is this machine's name for the store, `member` the user's ID in it,
  both lowercase letters, digits, `_`, and `-`; `label` is a display name.
  The profile (`org`, `family`, `given`) is how others see the user. Before a
  register without `given`, offer the saved profiles the query lists or ask
  the user, since without `given` it uses the profile used last.
- `tk:unregister`, `tk:relabel`, `tk:hide`, and `tk:unhide` change this
  machine only and write nothing to the store. Send them only when the user
  asks. `tk:unregister` removes the store from this machine and deletes its
  local copy; the remote is left, so `tk:register` brings the store back. It
  is refused while the copy holds writes not yet pushed: sync first. A store
  with no remote holds its only copy here, so its data is lost for good, and
  it is removed only with `confirm` set to the store id.
- `tk:relabel` sets the display name; `label=""` clears it back to the
  store id. `tk:hide` leaves the store out of the dashboard's store list;
  the tracker tools still read and write it. `tk:unhide` lists it again.

## Sync

Writes reach the remote 5 minutes after the last write while this session
runs. `tk:sync` pushes the user's unpushed writes and fetches at once, for
`store` or, without it, every store; send it after writes others should see
now and before you end the session. Others' writes are fetched before each
query.

## Event Hooks

The dashboard daemon runs local commands on tracker events, as the user asks
for (a desktop notification, a mail). Write or edit their file only when the
user asks: it chooses what runs on this machine. The file is
`tracker-hooks.json` in the daemon's state directory, `dashboard/` under the
ws cache root (`$WS_CACHE_HOME` when set), next to the daemon's
`endpoint.json`:

```json
{"hooks": [{"command": ["notify-tracker", "--quiet"], "scope": "mine"}],
 "due_soon_hours": 24}
```

- `command` is an argv run without a shell, in the state directory, with the
  daemon's environment. `scope` is `mine` (the default: events that concern
  the store's local member, not the member's own actions) or `all` (every
  event of the store). `due_soon_hours` (default 24) is how long before an
  item's due date its `due_soon` event fires. Each hook applies to every
  registered store. The daemon reloads the file when it changes; an invalid
  file runs no hook, and the daemon log says why.
- Each run gets one event as JSON on stdin. A hook's runs are serial and in
  order, each stopped after 30 seconds; a failure is logged and not retried.
  After the daemon was down, it delivers what arrived meanwhile, up to 24
  hours old.

The event:

- `v`: 1. Adding a field keeps it; changing a field's meaning bumps it.
- `id`: the tracker event's id, `due:<item id>:<due date>` for `due_soon`.
  An event that assigns several members gives one event per member with the
  same `id`.
- `kind`: `assigned` (also a claim, or an approved take request, for its
  taker), `assign_pending` (a request or handoff awaiting acceptance),
  `take_request` (a member asks for the assignment `target` holds),
  `take_denied` (to the requester, `target`), `comment`, `status`,
  `closed`, `due_soon`. A script ignores a kind it does not know; new kinds
  come without a `v` bump.
- `ts`: RFC 3339.
- `store`: `{id, name}`; `item`: `{id, alias, title, kind}`.
- `actor`: `{id, name}` of the member who acted; null for `due_soon`.
- `target`: `{id, name}` of the member the event is aimed at (the assignee,
  the holder asked, or the requester denied), when there is one.
- `detail`: `status` (for `status` and `closed`: the status written, so
  `in-review` for a close the assigner confirms and `dropped` for a drop),
  `comment` (its first 500 characters), `due` (RFC 3339, when the item has
  a due date); absent fields are omitted.
- `url`: opens the item in this machine's dashboard.
