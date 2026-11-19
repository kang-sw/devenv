---
kind: print
---

# Tracker: administration

Read after the `tracker` playbook. Role, label, and setting commands go
inside a `tk:store` group. A store command cannot be rolled back and
stands alone in its batch, outside any group.

| command | attributes | content |
|---|---|---|
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
| `tk:unregister` | `store`, `confirm?` | none |
| `tk:relabel` | `store`, `label` | none |
| `tk:hide`, `tk:unhide` | `store` | none |

**Roles and settings**
- `tk:roles.*` and another member's `tk:member.profile` (`member`) are admin
  commands.
- `tk:roles.set_lead` sets `member`'s lead; `leads=""` → none.
- `key`: an SSH public key line. `names`: the priority level names, lowest
  first; `default`: a level number. `max_bytes`: a byte count. Admins also
  change the priority levels and attachment limit in the dashboard Team
  tab's store settings.
- Labels: `name` unique ignoring case; `color` is `#rrggbb`; `label` is an ID
  from the labels line or `$N`. A rename or recolour shows on every item
  that carries it. A removed label leaves every item and the `label:`
  search; adding its name again makes a new label those items do not get
  back.

**Store commands**
- `tk:register`: with `remote`, joins the store that repository holds, or
  creates one in an empty repository; without, creates a personal store.
  `store` (this machine's name for it) and `member` (the user's ID in it):
  lowercase letters, digits, `_`, `-`. `label`: a display name. `org`,
  `family`, `given`: how others see the user; without `given` the last
  profile is reused, so first offer the saved profiles the query lists or
  ask the user.
- `tk:unregister`, `tk:relabel`, `tk:hide`, `tk:unhide` change this machine
  only and write nothing to the store; send them only when the user asks.
  - `tk:unregister` deletes the local copy; the remote stays, so
    `tk:register` brings the store back. Refused while unpushed writes
    remain: sync first. A store with no remote holds its only copy here and
    is lost for good: it needs `confirm` set to the store ID.
  - `tk:relabel` sets the display name; `label=""` → back to the store ID.
  - `tk:hide` leaves the store out of the dashboard's store list; the
    tracker tools still use it. `tk:unhide` lists it again.
