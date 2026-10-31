## On: arm the wait

1. If your context says mailbox push is bound to your session key
   automatically (beside your session key, or in a key-change notice), launch
   nothing from this section: arriving mail already starts your next turn.
2. Otherwise, if the mailbox watch tool is available, call it with your
   session key before you first go idle waiting on a peer. While it is bound,
   arriving mail starts your next turn even when you are idle, so you launch
   nothing else from this section. Calling it again with the same key changes
   nothing.
3. Otherwise, when nothing in your context says mailbox push is bound,
   launch `{{.MailboxWaitCommand}}`
   as a background process before you go idle waiting on a peer, and end the
   turn; its completion re-invokes you.
4. Every wake path, the automatic push included, is best-effort while you or
   your user are genuinely away from the harness: it fires promptly once the
   harness next actually takes a turn, not necessarily the instant mail
   arrives.
