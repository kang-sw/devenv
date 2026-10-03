# Preparing for compaction

Compaction replaces this conversation with a summary. Durable state belongs in
ws tooling, where it survives every compaction; the summary carries pointers to
it plus what only this conversation holds. The adapter writes the summary's
fixed parts itself: the session key, the active ticket and playbook, child
agents in flight and finished, and the recent dialog (user messages, your
replies, and one line per tool call; tool output is left in the session file).
You write the rest as prose under fixed headings.

Do these in order, without starting new work in between:

1. **Tidy durable state.** Bring the agenda and todos up to date with the
   current work. Record a non-obvious fact a future session would otherwise
   re-derive as a note. Put an open decision into its ticket's Open Decision
   Queue, or into a new ticket through `lead-ticket`. Skip anything already
   recorded.
2. **Write the prose, from scratch.** Carry forward what in the previous
   summary is still live and drop what is resolved; never append to it. For
   content already persisted (tickets, commits, notes, agenda, todos), give
   its path or pointer. For content that lives only in the conversation,
   summarize it as precisely as possible. Leave out what the adapter already
   adds and what `lead-revive` restores (agenda, todos, notes contents), and
   list no files. Aim for about 2-4k tokens in total.
   - **Current work** names the active playbook's current step, if one is
     running.
   - **Immediate next step** quotes the user's latest request verbatim.
3. **Call `ws-compact`** with every heading filled (empty when there is
   nothing), then end your turn. The conversation resumes from the summary;
   an active goal keeps running.
