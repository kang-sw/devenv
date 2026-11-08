---
kind: print
---

# Tracker

The tracker holds the user's tasks, todos, and topics in the stores the user
registered on this machine: personal stores, and team stores shared through a
git repository. Only a top-level lead and a store's support conversation call
the tracker tools; workers and subagents report progress to their lead. A
support conversation writes to its own store alone, and only what the user
asks for in its chat: the unasked recording below is a lead's, and
`tk:register` and `tk:sync` are refused to it.

## Calls

- `{{.McpNamespace}}/tracker.query(session_key, payload?, format?)` changes no
  store. With no `store` it lists the registered stores, each with the user's
  open work, and the saved profiles.
- `{{.McpNamespace}}/tracker.update(session_key, commands)` applies one batch
  and prints, per command position, the ID of the event it wrote. The ID a
  `tk:item.create` or `tk:comment.add` prints is the new item's or comment's
  ID for later batches.

## Recording

- Record progress unasked only on an item assigned to the user: when the work
  it names starts, blocks, or finishes, write `tk:item.status` or
  `tk:item.close` with `origin="agent-auto"`. Write anything else only when
  the user asks.
- Write titles, bodies, notes, and comments in `workflow.lang` when set,
  otherwise in the language the user writes in.

## Query

```text
tracker.query payload: {store?, scope?, item?, kind?, status?, project?, text?}
```

- `scope`: `mine` (the default: the user's assignments, then the items the
  user created and assigned to nobody), `received` (assignments sent to the
  user, the user's answers, and the ones the user sent), `assigned` (items the
  user assigned or handed to others), `team` (the work of the members the user
  leads, by person), or `all` (every item, in tree order).
- `item`: one item in full, with its assignments, children, and comments.
- `kind`, `status`, `project`, and `text` (part of the title, body, project,
  or ID) narrow the rows. `status` is `open` (not done), `hidden` (the only
  way hidden items show), or a status the rows print: `todo`, `doing`,
  `blocked`, `awaiting`, `done`, `pending`, `unassigned`, `declined`, or
  `returned`.
- `format: "json"` with `store` alone returns the store's full state as JSON
  (members, keys, events, every item), for what the text omits; without
  `store`, the store list as JSON.

## Commands

Every tag carries the prefix `tk:` and follows the command syntax rules.
`tk:store` is the group: its `name` names the store for every command
inside it, which is every command except `tk:register` and `tk:sync`. A
batch writes to one store, so it holds one `tk:store` group. `tk:register`
and `tk:sync` reach the network, so each stands alone in its batch, outside
any group.

| command | attributes | content |
|---|---|---|
| `tk:store` | `name` | group of every command below but `tk:register` and `tk:sync` |
| `tk:item.create` | `kind`, `title`, `project?`, `due?`, `priority?`, `parent?`, `mentions?` | the body |
| `tk:item.edit` | `item`, `title?`, `project?`, `due?`, `priority?`, `mentions?`, `clear?` | a new body |
| `tk:item.hide` | `item` | none |
| `tk:item.assign` | `item`, `people` or `levels`, `completion?` | none |
| `tk:item.handoff` | `item`, `to`, `assignee?` | a note |
| `tk:item.accept`, `tk:item.decline` | `item` | a note |
| `tk:item.return` | `item`, `assignee` | a note |
| `tk:item.unassign` | `item`, `assignee?` | none |
| `tk:item.status` | `item`, `status` | a note |
| `tk:item.close` | `item` | a note |
| `tk:item.confirm`, `tk:item.reject` | `item`, `assignee` | a note |
| `tk:item.reopen` | `item`, `assignee?` | a note |
| `tk:comment.add` | `item`, `reply_to?`, `mentions?` | the comment |
| `tk:comment.edit` | `comment`, `mentions?` | the new text |
| `tk:comment.hide` | `comment` | none |
| `tk:member.profile` | `org?`, `family?`, `given?`, `member?` | none |
| `tk:roles.set_lead` | `member`, `leads` | none |
| `tk:roles.add_admin`, `tk:roles.remove_admin`, `tk:roles.remove_member` | `member` | none |
| `tk:roles.add_key`, `tk:roles.revoke_key` | `member`, `key` | none |
| `tk:roles.set_priority_levels` | `names`, `default` | none |
| `tk:roles.set_attachment_limit` | `max_bytes` | none |
| `tk:register` | `store`, `member`, `remote?`, `label?`, `org?`, `family?`, `given?` | none |
| `tk:sync` | `store?` | none |

```xml
<tk:store name="team">
<tk:item.create kind="task" title="Parser" due="2026-11-20">
Body text, markdown, written as is.
</tk:item.create>
<tk:item.assign item="$1" people="maya"/>
<tk:item.status item="ada.pc-4" status="doing" origin="agent-auto">Started</tk:item.status>
</tk:store>
```

- `item`, `parent`, `reply_to`, `comment`: an ID as the query or an update
  prints it, or `$N` for the item or comment that command `N` (counting from
  1) of the same batch creates.
- `kind`: `task` (work to assign and track), `todo` (a personal checklist
  entry), or `topic` (a discussion with no status; it cannot be assigned).
  `due`: `YYYY-MM-DD`. `priority`: a level number from the store view's
  priority line, larger is more urgent; without one an item gets the store
  default. A note is optional.
- People are member IDs; `people`, `mentions`, `leads`, `names`, and `clear`
  are space-separated.
- Members form a lead tree; each member has at most one lead.
  `tk:roles.set_lead` sets `member`'s lead (`leads=""` for none).
- Assignment lifecycle:
  - `tk:item.assign` (the item's creator): `people` assigns those members,
    live at once for the user and the members the user leads at any depth,
    and pending for anyone else until they `tk:item.accept` or
    `tk:item.decline`. `levels="N"` assigns everyone up to N levels below the
    user, including anyone who later joins that range.
  - The holder of a live assignment sets its `tk:item.status` and closes it
    with `tk:item.close`. With `completion="assigner"` (the default is
    `assignee`) a close waits until the assigner sends `tk:item.confirm` or
    `tk:item.reject` naming the holder as `assignee`.
  - `tk:item.handoff` passes the user's live assignment to `to`, pending
    until `to` accepts; an admin hands off another member's by naming them as
    `assignee`.
  - `tk:item.return` (the holder's direct lead) ends an accepted assignment.
    `tk:item.unassign` (the assigner) withdraws `assignee`'s assignment, or,
    without it, all of the user's assignments on the item.
    `tk:item.reopen` (the assigner or the holder) reopens a done one.
- `clear` on `tk:item.edit`: `body`, `due`, or `clear="body due"`; an absent
  body leaves the body as it is.
- `origin`: `agent` (the user asked; the default) or `agent-auto` (recorded
  unasked, valid only on `tk:item.status` and `tk:item.close`; see Recording).
- `tk:roles.*` and another member's `tk:member.profile` are admin commands.
  `key`: an SSH public key line. `names`: the level names, lowest first;
  `default`: a level number. `max_bytes`: a byte count.
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

## Sync

Writes reach the remote 5 minutes after the last write while this session
runs. `tk:sync` pushes the user's unpushed writes and fetches at once, for
`store` or, without it, every store; send it after writes others should see
now and before you end the session. Others' writes are fetched before each
query.
