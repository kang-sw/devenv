---
title: ws dashboard browser interaction and access - message inbox, LAN token, and image route
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-daemon-core: prerequisite; the message and cancel routes, composer chips, sent marks, and held bubble extend the core's HTTP server and session page (its Phase 3)
  261106-feat-ws-dashboard-claude-adapter: prerequisite through its Phase 1 event probe only, which runs before any work here (266); its submission phase consumes this ticket's inbox, drain tool, and wait flag
  261106-feat-ws-dashboard-live-snapshot-pi: its Pi submission phase consumes this ticket's inbox, drain tool, and wait flag; injected messages reach the conversation through its ring
  261106-feat-whiteboard-rich-items: context; board items reference image files once markdown images land, and Phase 2's image route serves them under the project-root rule (238)
---

# ws dashboard browser interaction and access - message inbox, LAN token, and image route

## Background

The parent epic `261106-epic-ws-dashboard-serve-daemon` serves every ws
session on a machine as a live page. The daemon core
(`261106-feat-ws-dashboard-daemon-core`) renders that page read-only. This
child adds every place the browser gains write or network access, so they
are reviewed together (201): the board-answer inbox and the routes that
write, merge into, and cancel its entries; the ws-mcp side that drains the
inbox and wakes the adapter; LAN opt-in with its access token; and the image
route. The adapters' side of submission (draining at turn end and injecting
the message) is a phase of each adapter child (213). Read the epic's
Cross-Child Decisions for shared context; the decisions below bind this
child.

## Decisions

**Inbox and submission**
- **The inbox is the daemon's one write target.** The daemon reads on-disk
  stores only; its one write target is the board-answer inbox: it writes
  sent messages there, merges into or deletes entries not yet drained.
  (169), (209), (249), (251)
- **The composer sends a user message.** The input is a general message
  composer, not an answer form. Clicking an option, `ok`/`no`, or an
  acknowledgement on a board card attaches an answer chip for that item to
  the composer (one chip per item ID, a later click replaces it; a chip can
  carry a note). Sending delivers one user message: the typed text plus the
  answer block `(nn) a; (mm) ok`. After sending, cards show a sent mark and
  stay editable; a later answer to the same ID replaces the earlier one.
  (240), (253) Rejected: locking cards after sending, which blocks
  correcting a misclick.
- **Per-session inbox on disk.** The daemon writes each sent message as one
  file in a per-session board-answer inbox, per-entry files like the mailbox
  history ring, outside the mailbox store. The adapters' existing `mailbox
  wait` child gains a flag to also watch that inbox and to report which kind
  arrived; a new ws-mcp tool drains the inbox. (209) Rejected: mail to the
  session's `id:<reply-id>` (172), because the mailbox stays agent-to-agent;
  a second wait child per session, which doubles the long-running processes
  each adapter manages; a daemon-to-adapter socket, which would need daemon
  discovery and token handling in TypeScript (rejected in (191)).
- **Held means undrained; claims are atomic.** A message that arrives while
  the session's turn runs is held and submitted when the turn ends (212).
  Held means left undrained in the inbox: the adapter drains only at turn
  end, so merge (251) and cancel (249) act on the entry until then. Draining
  claims an entry atomically (for example by rename), so a cancelled entry
  is never also injected and a claimed entry can no longer be merged into or
  cancelled; the daemon's merge and cancel claim the entry the same way, so
  a rewrite racing the drain cannot bring back an injected entry. Because
  `mailbox wait` only peeks, the adapter re-arms it without the inbox flag
  while an entry is held and adds the flag back after the turn-end drain.
  Rejected: draining on arrival and queueing with Pi `followUp`, which
  empties the inbox before merge or cancel can act.
- **Merge into the held message.** A second send while a message is held
  merges into the held message, so one inbox entry starts one turn; for a
  chip on the same item ID the later one wins. (251) The merged text keeps
  each send as its own paragraph, separated by a blank line, and the held
  bubble draws a thin divider between them. (277) Rejected: a plain join,
  which runs separate sends into one paragraph; numbered parts, which are
  noise to the agent; queueing each send as its own message, since after the
  first is injected the next lands in a running turn and is held again
  (212).
- **Cancel.** The conversation shows a held message as held, with a cancel
  control. Cancel removes the entry from the inbox and returns its text and
  chips to the composer. It touches only a message the session has not
  received, so it is not session control. (249)
- **No session control.** The browser offers no control over sessions or
  subagents beyond sending and cancelling the user's own messages. (189),
  (249) Rejected: permission approval and turn interrupt from the browser
  (241 b); they would let the LAN token authorize tool execution, a security
  boundary that needs its own design.
- **Hosts without an adapter.** A host with no adapter, such as Codex, has
  no browser submission; its page keeps only the copy-answers button. (190),
  (211), (219), (252) Rejected: falling back to mail for those hosts
  (211 b), which would put user input back into the mailbox.

**Access**
- **Localhost by default; LAN opt-in.** LAN access is opt-in and requires an
  access token. (173) A config key turns LAN mode on. The access token
  persists in the machine cache across daemon restarts and is shown on `/`
  only when that page is viewed from localhost; deleting the token file
  rotates it. The pointer line keeps printing the localhost URI. (287)
  Rejected: a new token per daemon start, which breaks a second device's
  bookmark at every ws-mcp upgrade, since each upgrade replaces the daemon.
- **Two separate tokens.** The LAN access token is separate from the
  endpoint-file token and is never written where the shutdown check reads.
  (288) Rejected: one shared token, which would let any LAN browser send the
  authenticated shutdown request.
- **Browser write guards.** The browser write routes (message, cancel)
  require an Origin equal to the daemon's own origin, which stops cross-site
  form posts; the daemon sends no CORS headers. In LAN mode the access token
  takes the localhost Host check's place. A local process can write the
  inbox files directly, so a localhost credential would guard nothing the
  browser guards leave open. (284) Rejected: a token on localhost too, which
  puts the credential in the pointer-line URI and so in agent transcripts;
  one-time pairing with an owner cookie as 260515 did, a pairing step for
  the same protection.
- **The LAN view matches the localhost view.** Argument previews and the
  conversation show in LAN mode too; the access token protects them. (197)
  Rejected: hiding argument previews in LAN mode. The same token already
  allows sending user messages (210), which can move a session, so hiding
  read content adds little protection.
- **Images only under the project root.** The daemon serves an image a board
  item references only when the file is inside that session's project root
  and refuses any other path. (238) Rejected: serving any referenced path
  behind the token, which exposes local files in LAN mode.

**Ordering**
- **Claude probe first.** This child's work starts after the Claude adapter
  child's Phase 1 probe. (266)

## Constraints

- Shipped surface: the drain tool's name, description, and output, the
  `mailbox wait` flag and its help text, and the page scripts ship
  downstream; none may name this repository's tickets, commits, or layout.
  The drain tool's default output is compact text (`ai-docs/manuals/ws-mcp.md`
  Tool Output Convention).
- The mailbox store and mailbox semantics are unchanged; the inbox lives
  outside it. (208), (214)
- A session key never reaches the browser; routes stay keyed by reply-id.
  (180)
- The answer block follows the reply grammar the static page's copy-answers
  already builds (`build` in
  `agents-plugin-tool/internal/wswhiteboard/assets/form.js`), which the
  `(nn) a; (mm) ok` example matches, so a copied and a sent answer read the
  same. (296)
- Out of scope: the adapters' turn-end drain and injection (adapter
  children); the read-only page render, routes, and the localhost Host check
  (core child); the image link rule inside rendered items (rich-items
  ticket).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Phases

### Phase 1: Board-answer inbox and message submission path

Add the per-session board-answer inbox; the daemon's message route (writes
an entry, or merges into the held entry) and cancel route (claims and
removes a held entry, returning its text and chips); the Origin check on
both; the composer's answer chips from board cards, the sent marks, and the
held bubble with its divider and cancel control in the conversation; the
ws-mcp drain tool that claims entries atomically and nudges after draining;
and the `mailbox wait` flag that also watches the inbox and reports which
kind arrived.

The worker settles and records in the Result:

- The inbox location and entry format. Constraint: both the daemon (from the
  session index) and the session's own ws-mcp derive it without a session
  key reaching the browser; readers ignore unknown fields.
- The drain tool's and the wait flag's names and output shape.
- How the daemon tells a held entry from a drained one for the held bubble.

Verification: Go tests in `agents-plugin-tool/internal/mcp/` (drain tool:
atomic claim, a claimed entry not drained twice, compact text output) and in
`agents-plugin-tool/cmd/ws-mcp/mailbox_test.go` and
`agents-plugin-tool/internal/wsmailbox/wait_test.go` (the wait flag wakes on
an inbox entry and reports the kind, and without the flag an inbox entry
does not wake it), daemon route tests (an entry written per send, a second
send merged as a separate paragraph with the later chip for one item ID
winning, cancel removing a held entry and returning its text and chips,
cancel and merge refused on a claimed entry, a cross-origin or Origin-less
POST refused, no CORS headers), and a race test where drain and cancel, and
drain and merge, contend for one entry and exactly one wins with no entry
both injected and cancelled, then `go test ./...` in `agents-plugin-tool`.
No headless browser is available here, so the lead checks by hand on a live
page: clicking options and acknowledgements attaches and replaces chips, a
chip note, the sent answer block text, sent marks with cards still
editable, a held bubble with a divider after two sends, and cancel returning
text and chips to the composer.

### Phase 2: LAN opt-in, access token, and image route

Add LAN mode: the config key that turns it on (declared in the ws and
wsflow config manifests), binding beyond loopback, and the persisted access
token (287), separate from the endpoint-file token (288), that replaces the
Host check on every route in LAN mode, including the Phase 1 write routes;
`/` shows the token only to a localhost viewer. Add the image route, which serves an image a board item
references only when the resolved file is inside that session's project
root.

The worker settles and records in the Result:

- How the image route resolves a path and checks containment. Constraint:
  symlinks and `..` segments cannot reach outside the project root; the
  route refuses every other path in both modes. (238)

Verification: Go tests cover LAN mode refusing a request without the token
and accepting one with it on every route, the endpoint-file token refused as
a LAN token, the token surviving a daemon restart and shown on `/` to a
localhost viewer but not to a LAN one, localhost mode unchanged, and the
image route serving a file under the project root while refusing an
absolute path outside it, a `..` escape, and a symlink pointing outside,
then `go test ./...` in `agents-plugin-tool`. The lead checks by hand from a
second device on the LAN that the page needs the token, shows the same
content as localhost (argument previews included), and loads a board image.
