---
kind: print
includes:
  - arm-the-wait
---

# Use Mailbox

Topic: cross-session mail between independent {{.SkillNamespace}} harness
sessions — registering an address, discovering it, sending and receiving,
arming a wait so an idle session wakes on mail, and using that to work with
another session as a peer lead.

## Invariants

Scope
- This is for peers with no spawn relationship: separate harness sessions,
  not a native subagent inside your own session — that has its own
  harness-native messaging and is out of scope here.
- A session that never registers an address and never sends or looks itself
  up stays fully inert: no presence, no badge, no overhead.
- Registering an address is only what makes you durably reachable by a fixed
  name; you can send mail and receive a reply to it without one.

Wake
- The `ws-mcp mailbox wait` CLI subcommand blocks. When you do arm it
  (`On: arm the wait`), launch it only as a background process through your
  harness's own background-task capability. Calling it inline, or polling it
  in a loop, blocks your own turn instead of freeing it.
- Any wake path here is a durable read, not a bare arrival event: it checks
  existing unread mail the instant it fires and returns or wakes immediately
  if any is already there, so mail deposited before you armed it is never
  missed.

## On: register an address

1. Before your harness starts its MCP server, set `WS_MAILBOX=<name>@<scope>`
   in its launch environment to claim a durable, memorable address, or set
   `WS_MAILBOX_AUTO=<scope>` to have the server mint a random address for you
   with no name to choose. `WS_MAILBOX` wins if both are set.
2. Setting neither leaves you without a durable inbox, but sending and
   receiving replies still work (`On: send and receive`).

## On: find an address

1. Read the workflow ambient block, or call
   `{{.McpNamespace}}/mailbox.lookup_peers(session_key: <your key>)`: its self
   entry reports your own resolved address, or — if you set neither launch
   env above — your live reply handle, once you have sent or looked yourself
   up at least once.
2. Relay your address to whoever should mail you. The same call's peer list
   shows the most recently seen named addresses at the scope you ask for,
   whether or not their session is running now (mail to one that is not
   waits until it returns), with enough descriptive metadata (harness,
   working directory, start and last-seen time) to tell peers apart before
   you relay one on to a human.
3. To reach a session you were already corresponding with — its handle
   scrolled away or was lost to compaction — read the same call's
   correspondents list: every address you yourself sent mail to or received
   mail from, in any scope, newest first, with its harness and working
   directory when known.

## On: send and receive

1. `{{.McpNamespace}}/mailbox.send(to: <address, or the id: handle from a
   received envelope>, content: ..., session_key: <your key>)` reaches any
   discoverable peer; sending needs no address of your own.
2. `{{.McpNamespace}}/mailbox.recv(session_key: <your key>)` drains your mail.
   While you stay active, an unread badge already rides your ordinary
   {{.McpNamespace}} tool responses, so you notice new mail for free without
   a separate check — call `recv` once you see it.
3. Reply with whichever handle the received envelope carried; it already
   identifies you correctly to a sender who may hold no durable address of
   their own.

## On: work with a peer lead session

The session on the other end is a peer lead with the same workflow authority
as yours: it can discuss, write and run tickets through its own workers in
parallel worktrees, review, and merge, and it answers to its own user. Mail
sent by a top-level lead starts with a `[ws note: from a peer lead session
...]` line saying so; reading such a sender as your worker or executor makes
you audit or direct it instead of collaborating.

1. Get the peer's own address (`On: find an address`, run by the peer and
   relayed to you, typically by the user).
2. `mailbox.send` it the outcome you want in plain language, as you would ask
   a fellow lead, for example "please run <ticket> and tell me when it
   lands". The peer decides how to get there, may push back or consult its
   own user, and owns the work it runs.
3. The peer must actually be positioned to notice: idle with its wait
   armed, or on a harness that itself delivers arriving mail as a
   turn-starting message under the condition `On: arm the wait` states. A
   peer with neither only notices at its own next active turn.
4. The peer drains with `mailbox.recv`, weighs the request on its own
   judgment, and can `mailbox.send` its reply back using the handle its
   envelope carried. When you are the receiving side, answer the same way:
   as a fellow lead, not as a worker reporting to its lead.
