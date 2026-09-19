## On: arm the wait

1. Arm the wait only if your harness does not already deliver arriving mail
   to you as a turn-starting message once you go idle. If it does, that
   delivery is your wake mechanism and you arm nothing here.
2. To arm it, launch `{{.MailboxWaitCommand}}` as a background process
   through your harness's own background-task capability, never inline and
   never in a poll loop. Treat its exit as your cue to come back; read what
   it printed and act.
3. A harness that re-invokes you when a background task completes (for
   example, Claude's own background-task notification) makes step 2's launch
   the actual wake mechanism: arm it whenever you go idle, whether or not you
   hold a registered address.
4. A harness whose own turn-boundary hook re-invokes you on unread mail, but
   only once you hold a registered address (for example, Codex's `Stop`
   hook, once its deployment trusts it), satisfies step 1's condition only
   after that address exists; with no registered address yet, step 2 still
   applies there.
5. Either wake path is best-effort while you or your user are genuinely away
   from the harness: it fires promptly once the harness next actually takes a
   turn, not necessarily the instant mail arrives.
