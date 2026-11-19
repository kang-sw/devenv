---
kind: print
---

# Tracker: assignments

Read after the `tracker` playbook; its command rules apply. Members form a
lead tree, each with at most one lead; "below the user" means the members
the user leads at any depth, and a member's superiors are their leads at
any depth.

| command | attributes | content |
|---|---|---|
| `tk:item.assign` | `item`, `people?` and `rules?` (one or both), or `levels`; `completion?` | none |
| `tk:item.accept`, `tk:item.decline` | `item` | note |
| `tk:item.confirm`, `tk:item.reject` | `item`, `assignee` | note |
| `tk:item.reopen` | `item`, `assignee?` | note |
| `tk:item.handoff` | `item`, `to`, `assignee?` | note |
| `tk:item.return` | `item`, `assignee` | note |
| `tk:item.unassign` | `item`, `assignee?`, or `root?` and `rule?` | none |
| `tk:item.open` | `item`, `scope` | none |
| `tk:item.unopen`, `tk:item.claim` | `item` | none |
| `tk:item.request` | `item`, `assignee` | note |
| `tk:item.approve`, `tk:item.deny` | `item`, `request` | note |
| `tk:item.unrequest` | `item`, `request` | none |
| `tk:item.cc`, `tk:item.uncc` | `item`, `member` | none |

```xml
<tk:store name="team">
  <tk:item.create kind="task" title="Parser"/>
  <tk:item.assign item="$1" people="maya"/>
</tk:store>
```

**Assigning**
- `tk:item.assign` (the item's creator):
  - `people`: live at once for the user and members below the user; pending
    for anyone else until they `tk:item.accept` or `tk:item.decline`.
  - `rules` assign subtrees, later joiners included: `member:N` (up to N
    levels below `member`), `member:*` (all below), `member+:` (`member`
    too), as in `rules="maya+:1 raj:*"`. Rooted at the user or below → live;
    rooted elsewhere → a request pending to that member, live once
    they accept. `levels="N"` alone
    is `<user>:N`.
- Assigning anyone, the creator included, replaces the creator's holding;
  when the last assignment ends, the creator holds it again at todo. A task
  held only by its creator cannot be unassigned, handed off, or returned:
  assign or open it.
- A topic: `people` names exactly one person, refused while anyone holds it
  or a handoff of it is pending. It takes no status or close; its holder
  releases it with `tk:item.unassign` naming themselves as `assignee`, or
  hands it off.

**Completion and review**
- Without `completion`, it is stored by target: `assigner` for members
  below the user and for rules rooted at the user or below (`levels` too);
  `assignee` for the user and anyone else. Targets of both kinds are
  written as two assigns; the update prints the second as split off.
  `completion="assignee"` for a below-the-user target is refused.
- Under `completion="assigner"` the holder's close puts it in `in-review`;
  the holder may enter `in-review` on any assignment. The assigner or any
  superior of the holder resolves it with `tk:item.confirm` (done) or
  `tk:item.reject` (back to doing), naming the holder as `assignee`.
- `tk:item.status` with `assignee` writes on that member's live assignment:
  a superior sets `blocked`, `done` (final, no review), or `dropped`; an
  assigner who is not a superior sets only `dropped`. A holder drops their
  own only under `completion="assignee"` or when they assigned it.
- A done or dropped assignment takes no status, handoff, take request,
  approval, or handoff acceptance until `tk:item.reopen` (the assigner or
  the holder).

**Passing and ending**
- `tk:item.handoff` passes the user's live assignment to `to`, pending
  until `to` accepts; an admin passes another member's by naming them as
  `assignee`.
- `tk:item.return` (the holder's direct lead) ends an accepted assignment;
  on an accepted rule request it ends the rule.
- `tk:item.unassign` (the assigner): `assignee` → withdraws that
  assignment; `root` → ends the user's rules rooted there; `rule` (its ID
  follows the rule in the item view) → ends that rule alone, withdrawing it
  while it is a request; neither → withdraws all the user's assignments and
  rules on the item.

**Taking work**
- Assign when the creator knows who should do it; open it when anyone in a
  group may take it.
- `tk:item.open` (the creator) offers a task nobody else holds:
  `scope="team"` (the creator's org subtree) or `scope="store"`.
  `tk:item.unopen` withdraws it; an assign ends it.
- `tk:item.claim` takes an open task the user may claim (its row prints
  `claimable by me`; `q: "is:claimable"` lists them) as the user's live
  assignment, with no approval.
- `tk:item.request` asks for the live assignment `assignee` holds; a task
  held only by its creator is claimed once opened, not requested. The item
  view lists take requests with IDs. The holder or the creator answers with
  `tk:item.approve` (moves the assignment) or `tk:item.deny`; the requester
  withdraws with `tk:item.unrequest`. A request lapses when its assignment
  ends.
- Two claims of one opening, or two approvals for one assignment, written
  before either saw the other: the first in event order wins; the later is
  flagged with what took it first.

**CC**
- `tk:item.cc` and `tk:item.uncc`: any member adds or removes anyone, on any
  item not hidden. A CC'd member follows the item (the `related` scope;
  dashboard notices of status changes, closes, and comments) with no
  assignment. The item view prints a `cc:` line.
