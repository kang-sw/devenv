# Tracker mockup v0

An unstyled page for deciding the data structures of the team tracker (epic
`261107-epic-ws-personal-common-store`). v0 is a first concrete proposal, not a
decision. Everything the page shows is computed in the browser by merging the
per-member event logs on top of the roles file, which is the epic's storage
model (each member appends only to their own ref; views are client-side merges).

## Open it

Open `index.html` from the file system (double-click, or `file://` URL). No
build, no server, no dependencies. Fixtures are `fixtures.js` (a script that
assigns `window.TRACKER_FIXTURES`, because `fetch` of local JSON is blocked on
`file://`).

- Top bar: store switcher (`team-alpha` with 14 members, `hana-personal` with
  one) and a "viewing as" member switcher.
- Left: summary, actions, my assignments, org tree, item list, item detail
  (click an item id), merged event log (checkbox: flagged only).
- Right: the roles file and every member log as editable JSON. Edit, press
  `apply`, and every view recomputes. Edits live in memory only; `reset store to
  fixtures` or a page reload restores `fixtures.js`. To keep a change, edit
  `fixtures.js`.
- Actions append events to the viewer's log and show the produced events and
  whether they applied or were flagged. They are not pre-validated, so you can
  deliberately produce a flagged event (for example assign to someone beyond N
  levels).

## Proposed data model

### Roles file

```json
{ "admins": ["ada", "bo"],
  "members": [ { "id": "maya", "name": "Maya Ortiz", "keys": ["maya-k1"], "leads": ["ada"] } ] }
```

- `admins`: member ids. A signature from any one admin is valid for admin-only
  events. Admins are a layer by permission only; they are also ordinary members
  and may sit anywhere in the tree.
- `members[].id`: stable id, also the name of that member's log (ref).
- `members[].keys`: public key ids that may sign for this member. Mock
  signature is `sig: "<keyid>"`; no crypto.
- `members[].leads`: list of lead ids, validated to length at most 1 (epic:
  list now so multiple leads need no migration). Empty list = a root.
  Relations are one-directional and cycle-free.

The roles file is the genesis state. Admin-signed `roles.*` events are replayed
on top of it in merge order, so the org tree shown is "roles file + admin
events".

### Event envelope

```json
{ "author": "lena", "seq": 3, "ts": "2026-11-02T10:00:00Z", "sig": "lena-k1",
  "type": "item.assign", "via": "agent", "auto": true, "payload": { } }
```

- `author`: member id; must equal the owner of the log it sits in.
- `seq`: per-author, starts at 1, must be exactly previous + 1 in the log.
- `ts`: ISO timestamp, informational plus merge ordering.
- `sig`: mock signature = a key id that must be listed for `author`.
- `type`, `payload`: see below.
- `via: "agent"`: written by an agent session rather than typed by the human.
- `auto: true`: recorded unprompted (epic: only progress and close, only on the
  author's own assigned tasks). Absent = recorded on request.
- Event id = `author#seq`. An item's id is the id of its `item.create` event.

Merge order across all logs: `ts`, then `author` id, then `seq`. Replay is one
pass in that order; each event is validated against the state built so far and
either applied or flagged.

### Event types

| type | payload | meaning |
|---|---|---|
| `item.create` | `kind` (`task`/`note`/`todo`), `title`, `body?`, `project?`, `parent?` | Create an item. `task` = feature-level work, `note` = free text, `todo` = simple TODO. `project` = optional free-text project reference. `parent` = item id this is a subtask of. |
| `item.assign` | `item`, `mode` (`all`/`chosen`), `levels` (N), `people?` (for `chosen`), `completion` (`assignee`/`assigner`) | Creator assigns an item to everyone within N levels below them, or to chosen people within N levels. `completion` is chosen per assignment: assignee closes it, or the assigner must confirm. One assignment record per assignee. |
| `progress.report` | `item`, `pct` (0-100), `note?` | An assignee reports progress on their assignment (or the creator on an unassigned own item). |
| `item.close` | `item` | An assignee closes their assignment: `done` if completion is `assignee`, else `awaiting_confirm`. The creator closes an unassigned own item. |
| `item.confirm` | `item`, `assignee` | The assigner confirms an `awaiting_confirm` assignment, making it `done`. |
| `roles.add_member` | `member` (`id`, `name`, `keys`, `leads`) | Admin adds a member (stands in for join request plus admin approval). |
| `roles.set_lead` | `member`, `leads` | Admin changes a member's lead list. |

Derived (never stored): assignment state `open` / `awaiting_confirm` / `done`;
item status; ancestor chain (parent links up to the root item; "from ancestor"
in my-assignments is the root item's author); progress roll-up.

Roll-up: an assignment is 100 when done; otherwise the mean of the progress of
the assignee's own subtasks under that item if any exist (their own reports are
then ignored), else their last reported `pct` (0 if none). An item's progress is
the mean over its assignments; unassigned items use own `pct` or closed = 100;
notes have none.

### Flagged (ignored) events

Replay never applies these; the merged log shows each with its reason.

- unsigned; `sig` key not listed for the author; author not in the roles file
- author does not match the ref owner; `seq` gap or duplicate
- `auto` without `via: "agent"`, or on a type other than progress/close
- assignment: not the creator, note, target outside N levels (or not below the
  assigner), nothing new to assign
- progress/close by a non-assignee, after the assignment closed; confirm by
  non-assigner or when no confirmation is required
- `roles.*` by a non-admin; lead list longer than 1; cycle; unknown lead/member
- subtask whose parent is neither assigned to nor created by the author

## Open questions

Each is a v0 call to confirm or change, with the alternative.

1. **Roles file vs admin events.** v0: the roles file is genesis and admin
   `roles.*` events patch it during replay. Alternative: the roles file is the
   only truth, edited as a file by admin commits (then membership changes are
   not time-ordered with other events, and there is no `roles.*` event type).
2. **Admins as ids.** v0: `admins` lists member ids, so an admin is always a
   member with keys. Alternative: admins own separate key entries and need not
   be members; or admin status is a flag on the member record.
3. **Admin set changes.** v0 has no event to add or remove admins or keys
   (key rotation, revocation, leaving the team). Alternative: `roles.add_admin`,
   `roles.revoke_key`, `roles.remove_member` events; revocation needs a rule for
   events signed before the revocation.
4. **Roots.** v0: a root has `leads: []` and several roots are allowed.
   Alternative: exactly one root, or a mandatory top-level admin.
5. **Merge order.** v0: `ts`, then author id, then seq. Alternative: Lamport or
   causal clock (event names the last event it saw) so unsynchronised clocks
   cannot reorder cause and effect, at the cost of an extra field; or order by
   git commit graph. v0 trusts author clocks, so a wrong clock can place an
   event before its precondition and get it flagged.
6. **Event id.** v0: `author#seq`. Alternative: the git commit hash (needs the
   id to exist only after commit, so events cannot reference each other in one
   commit) or a random id.
7. **One event per commit?** Not modeled. v0 shows events only; mapping events
   to commits (one event per commit, or a batch per commit) is open.
8. **Assignment reach.** v0: `levels` N counts levels below the assigner;
   `all` resolves the target list at replay time and is not re-evaluated when
   members join later. Alternative: store the resolved list in the event
   (survives tree edits unchanged), or keep `all` live (new followers inherit).
9. **Who may assign.** v0: only the item's creator; re-delegation goes through
   a subtask item that the assignee creates and assigns. Alternative: an
   assignee re-assigns the same item in place (single item, longer chain).
10. **Subtask parent rule.** v0: parent must be assigned to or created by the
    author. A subtask is an independent item with a `parent` link; the parent
    assignment's roll-up reads only subtasks authored by that assignee.
11. **Assignment records.** v0: one record per (item, assignee), no
    unassign, reassign, or re-open; a second assignment to the same person is
    flagged. Alternatives: `item.unassign`, `item.reopen`, `item.reject` after
    an assigner declines a confirmation.
12. **Completion mode.** v0: per assignment (an item can have one assignee who
    closes and another needing confirmation). Alternative: per item.
13. **Progress shape.** v0: numeric `pct` plus note. Alternative: a state enum
    (`todo`/`doing`/`blocked`/`done`) with a free-text note, no percentage;
    roll-up then counts states instead of averaging.
14. **Roll-up rule.** v0: unweighted mean; subtask roll-up overrides the
    assignee's own reports once subtasks exist; `awaiting_confirm` counts as
    the last reported pct, not 100. Alternatives: weights, minimum, or counting
    done leaf tasks.
15. **Notes and todos.** v0: `kind` field on one `item.create`; notes cannot be
    assigned and have no progress; todos and tasks differ only by label.
    Alternative: separate event types, or todos as a checklist inside a task.
    Note edits (`item.edit`) do not exist yet, so titles and bodies cannot
    change after creation.
16. **Project reference.** v0: one free-text `project` string on the item.
    Alternative: a list, or a structured `{repo, path}` hint.
17. **Agent recording marker.** v0: `via: "agent"` plus `auto: true` on the
    envelope; the only rule enforced is that `auto` is limited to progress and
    close. Alternative: a single `origin` enum (`human`/`agent-requested`/
    `agent-auto`), or recording the agent session id.
18. **Signature placement.** v0 mocks `sig` as a key id inside the event. In git
    the commit signature is the real carrier, so the field may disappear and
    "signer" becomes "commit signer verified against `keys`". Alternative:
    keep an explicit per-event signature even inside a signed commit.
19. **Per-author ref ownership.** v0 flags an event whose `author` differs from
    the ref owner. Alternative: let admins or a lead write into a follower's
    ref (for example to reassign work of a departed member); then the rule
    becomes an allow-list.
20. **Visibility.** Everyone in a store sees every item and every event (epic:
    confidentiality is the repository boundary), so the "viewing as" switch only
    changes perspective (my assignments, action targets), not what is visible.
21. **Personal store progress.** v0 lets a creator report progress and close on
    their own unassigned items, so a one-member store works without
    self-assignment. Alternative: allow self-assignment, or a distinct `todo`
    state machine.

## Not modeled in v0

Join request flow, real signatures, git commits and refs, sync, search, MCP
surface, notifications and the dashboard badge, edits and deletion of items,
comments, due dates, labels or priorities.
