## On: arm the wait

Your harness already arms a background mailbox waiter for your session's
lead/owner role and pushes arriving mail directly into that role's
conversation once it arrives. As that role, you never launch anything from
this section; register, find, and send/receive still work as described
elsewhere in this playbook. A non-owner role (a spawned subagent) is not
covered by that adapter and is out of this playbook's scope in the first
place (`## Invariants` -> Scope): it uses its own harness-native messaging
with its parent instead of mailbox wait.
