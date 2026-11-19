---
kind: print
---

# Tracker

The tracker holds the user's tasks and topics in the stores registered on
this machine: personal stores, and team stores shared through a git
repository. Only a top-level lead and a store's support conversation call
the tracker tools; workers report progress to their lead. A support
conversation writes only to its own store, and only what the user asks in
its chat; it never sends `<tk:sync/>` or a store command (`tk:register`,
`tk:unregister`, `tk:relabel`, `tk:hide`, `tk:unhide`).

## Calls

- `{{.McpNamespace}}/tracker.query(session_key, payload?, format?)` changes
  nothing. No `store` → the registered stores with the user's open work, and
  the saved profiles.
- `{{.McpNamespace}}/tracker.update(session_key, commands)` applies one batch
  and prints, per command position, the event ID it wrote; for
  `tk:item.create` and `tk:comment.add` that is the new item's or comment's
  ID.
- `{{.McpNamespace}}/wiki.query` and `{{.McpNamespace}}/wiki.update` read and
  edit a store's wiki pages. Read a page (`op: "read"`) before saving it
  and pass the returned commit as the save's `base`.
  `[text](wiki:<name>/<path>)` in an item, comment, or page names page
  `<path>` (no `.md`) of wiki `<name>` in the same store: call the wiki
  tools with that store, `wiki: "<name>"`, and `path: "<path>"`.
- Rarer work → read its playbook first with
  `{{.McpNamespace}}/playbook.read`:
  - `tracker-assign`: assigning, review, handoffs, opening for claims, take
    requests, CC.
  - `tracker-admin`: roles, keys, labels, store settings, other members'
    profiles, and the store commands (register, unregister, relabel, hide,
    unhide).
  - `tracker-hooks`: the daemon's event hook file.

## Recording

- Unasked, record only an item assigned to the user: when its work starts,
  blocks, or finishes, write `tk:item.status` or `tk:item.close` with
  `origin="agent-auto"`. Anything else only when the user asks.
- Titles, bodies, notes, and comments: `workflow.lang` when set, otherwise
  the user's language.

## Query

```text
tracker.query payload: {store?, scope?, item?, kind?, status?, project?, text?, q?}
```

- `scope`:
  - `mine` (default): the user's assignments, the items the user created and
    assigned to nobody, then pending take requests the user may answer or
    filed.
  - `received`: assignments sent to the user, the user's answers, and the
    ones the user sent.
  - `assigned`: items the user assigned or handed to others.
  - `team`: the work of the members the user leads, by person.
  - `related`: items the user is CC'd on, then items whose body or a comment
    mentions the user; done, dropped, and hidden items and the ones the user
    created or is assigned are left out.
  - `all`: every item, in tree order.
- `item`: one item in full: assignments, opening, take requests, children,
  comments.
- `status`: `open` (neither done nor dropped), `hidden` (the only way hidden
  items show), or a row status: `idea`, `todo`, `doing`, `blocked`,
  `in-review`, `done`, `dropped`, `pending` (an assignment not yet
  accepted).
- `kind`, `project`: narrow the rows.
- `text`: part of the title, body, project, or ID.
- `q`: the dashboard Items search grammar, AND-ed with the fields above.
  Space-separated tokens, all must match, case ignored:
  - `status:<status>`, `kind:task|topic`, `project:` and `label:<name>`
    (exact), `assignee:<member>` (a live or pending assignee) and
    `cc:<member>` (ID or name contains; `cc:me` is the user), `is:claimable`.
  - `due:<date>` and `priority:<level>`: equality, or after `<`, `>`, `<=`,
    `>=`; dates `YYYY-MM-DD`; an item without a due date never matches.
  - A word or `"quoted phrase"` matches the title, body, project, ID, alias,
    or shown comments. An unknown key is plain text; a known key with a bad
    value is refused.
- `format: "json"` with `store` alone → the store's full state (members,
  keys, events, items); without `store` → the store list.

## Commands

Tags carry the `tk:` prefix. A batch writes one store: every command but
`<tk:sync/>` and the store commands goes inside its one `tk:store` group,
whose `name` is the store.

| command | attributes | content |
|---|---|---|
| `tk:store` | `name` | group |
| `tk:item.create` | `kind`, `title`, `project?`, `labels?`, `start?`, `due?`, `priority?`, `parent?`, `after?`, `mentions?`, `refs?` | body |
| `tk:item.edit` | `item`, `title?`, `project?`, `labels?`, `start?`, `due?`, `priority?`, `mentions?`, `refs?`, `clear?`, `base?` | new body |
| `tk:item.status` | `item`, `status`, `assignee?` | note |
| `tk:item.close` | `item` | note |
| `tk:item.move` | `item`, `parent`, `after?` | none |
| `tk:item.hide` | `item` | none |
| `tk:comment.add` | `item`, `reply_to?`, `mentions?`, `refs?` | comment |
| `tk:comment.edit` | `comment`, `mentions?`, `refs?` | new text |
| `tk:comment.hide` | `comment` | none |

```xml
<tk:store name="team">
  <tk:item.create kind="task" title="Parser" due="2026-11-20">Body text, Markdown, written as is.</tk:item.create>
  <tk:comment.add item="$1" refs="ada.pc-3">Split out of #ada.pc-3.</tk:comment.add>
  <tk:item.status item="ada.pc-4" status="doing" origin="agent-auto">Started</tk:item.status>
</tk:store>
```

**Values**
- `item`, `parent`, `after`, `reply_to`, `comment`, `request`, each `refs`
  entry: an ID as printed, or `$N` for what command `N` (from 1) of the same
  batch creates.
- Lists are space-separated: `people`, `rules`, `mentions`, `refs`,
  `labels`, `leads`, `names`, `clear`, `base`. People are member IDs.
- A note (the content) is optional. `origin`: `agent` (the user asked; the
  default) or `agent-auto` (only on `tk:item.status` and `tk:item.close` of
  the user's own assignment; see Recording).

**Items**
- `kind`: `task` (work to assign and track) or `topic` (a discussion with no
  status, held by one member). A topic the batch creates is assigned to the
  user unless the batch assigns it.
- A task always has a holder: its creator until someone is assigned (an
  admin once the creator is removed). The holder sets any status but `done`
  and closes it with `tk:item.close`.
- `status`: `idea` (before todo), `todo`, `doing`, `blocked`, `in-review`
  (asks for review before done), `done`, `dropped` (set aside, wrapped up
  like done). `idea`, `pending`, and `dropped` stay out of progress; an item
  whose assignments are all dropped shows dropped. A new task starts at
  todo; for idea, follow its create with
  `tk:item.status item="$N" status="idea"`.
- `start`, `due`: `YYYY-MM-DD`; a start after due is refused, checked
  against an unchanged date too. `priority`: a level from the store view's
  priority line, larger is more urgent; absent → the store default.
- `labels`: IDs from the store view's labels line, or `$N` for a label a
  `tk:roles.add_label` of the batch adds. An edit replaces them;
  `labels=""` removes them.
- `after`: the sibling it follows; `after=""` → first; absent → last.
- `tk:item.move` moves the item and its subtree under `parent` (`""` → top
  level), or reorders it. Move a misplaced item: recreating it loses its
  comments, assignments, and history.

**Edits**
- `tk:item.edit` (the creator or a live assignee) writes the whole body.
  `base` = the item's `body revision` line from your query: the store merges
  line by line with the edits others made since; lines both changed → the
  later write wins and both are marked as a conflict. No `base` → those
  edits are dropped.
- `clear`: any of `body`, `start`, `due`. No content → the body stays.
- `refs` are the items the text cites as `#<ID>`. `mentions` and `refs`
  ride with the body: a new body replaces both, so repeat them to keep them.

**Refusals**
- The store checks each command against the user's role and the item's
  state. A refusal writes nothing and names the reason: fix the batch and
  resend, or tell the user when their role does not allow it.

## Sync

Writes reach the remote 5 minutes after the last write while this session
runs; others' writes are fetched before each query. `<tk:sync/>` pushes and
fetches every store now (`store` names one); it stands alone in its batch,
outside any group, and a support conversation cannot send it. Send it after
writes others should see now and before the session ends.
