## On: arm the wait

1. If the mailbox watch tool is available, call it with your session key
   before you first go idle waiting on a peer. While it is bound, arriving
   mail starts your next turn even when you are idle, so you launch nothing
   else from this section. Calling it again with the same key changes
   nothing.
2. Otherwise, whenever you actually intend to sit idle waiting on a peer,
   launch `{{.MailboxWaitCommand}}` as a background process through your
   harness's own background-task capability, never inline and never in a
   poll loop. Your harness re-invokes you when that background task
   completes, so its exit is your cue to come back; read what it printed and
   act. Arm it whenever you go idle, whether or not you hold a registered
   address.
3. Either wake path is best-effort while you or your user are genuinely away
   from the harness: it fires promptly once the harness next actually takes a
   turn, not necessarily the instant mail arrives.
