---
title: ws dashboard browser interaction and access - message inbox, LAN token, and image route
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-daemon-core: prerequisite; the message and cancel routes, composer chips, sent marks, and held bubble extend the core's HTTP server and session page (its Phase 3)
  261106-feat-ws-dashboard-claude-adapter: prerequisite through its Phase 1 event probe only, which runs before any work here (266); its submission phase consumes this ticket's inbox, drain tool, and wait flag
  261106-feat-ws-dashboard-live-snapshot-pi: prerequisite for Phase 1, whose composer gating reads the adapter-presence signal its Phase 1 settles (297); its Pi submission phase consumes this ticket's inbox, drain tool, and wait flag; injected messages reach the conversation through its ring
  261106-feat-whiteboard-rich-items: context; board items reference image files once markdown images land, and Phase 2's image route serves them under the project-root rule (238)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 45f1769c6b18e3a3
sage-review-completeness-reviewed: 45f1769c6b18e3a3
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
  bubble draws a thin divider between them. (277) A send racing a drain
  (the held entry was already claimed) writes a new entry instead of
  merging, so the send is never lost. Rejected: a plain join,
  which runs separate sends into one paragraph; numbered parts, which are
  noise to the agent; queueing each send as its own message, since after the
  first is injected the next lands in a running turn and is held again
  (212).
- **Cancel.** The conversation shows a held message as held, with a cancel
  control. Cancel removes the entry from the inbox and returns its text and
  chips to the composer. It touches only a message the session has not
  received, so it is not session control. (249)
- **The drain tool is adapter-only.** Like the intake tool (289), its short
  description marks it adapter-only, since a model call would claim a held
  message that is then never injected. (design review)
- **No session control.** The browser offers no control over sessions or
  subagents beyond sending and cancelling the user's own messages. (189),
  (249) Rejected: permission approval and turn interrupt from the browser
  (241 b); they would let the LAN token authorize tool execution, a security
  boundary that needs its own design.
- **Hosts without an adapter.** A host with no adapter, such as Codex, has
  no browser submission; its page keeps only the copy-answers button. (190),
  (211), (219), (252) The composer, chips, and message route are offered
  only for a session with an adapter, read from the adapter-presence signal
  the live-snapshot child's Phase 1 settles. (297) Rejected: falling back to
  mail for those hosts (211 b), which would put user input back into the
  mailbox.

**Access**
- **Localhost by default; LAN opt-in.** LAN access is opt-in and requires an
  access token. (173) A config key turns LAN mode on; it lives in the
  `dashboard.`-namespace config manifest each package ships (see the daemon
  core's Config key home), default off in every package. LAN mode is a
  machine-level setting: the daemon honors the key only from the global
  config layer, ignores project and repo layer values, and applies a change
  at the next daemon start. (298) Rejected: any project whose resolved key
  is on turning LAN on for the whole daemon, which lets one project expose
  every session on the machine; the spawning session's resolved value
  deciding, which makes LAN depend on which session started the daemon. A
  change to the LAN value takes effect without waiting for an upgrade: the
  spawning ws-mcp compares the global-layer LAN value with the mode the
  daemon reports in `/health` and runs the normal handoff when they differ.
  Every session reads the same global value, so this stays consistent with
  (298). (design review) The access token persists in the machine cache
  across daemon restarts and is shown on `/` only when that page is viewed
  from localhost; deleting the token file rotates it. The pointer line keeps
  printing the localhost URI. (287) Rejected: a new token per daemon start,
  which breaks a second device's bookmark at every ws-mcp upgrade, since
  each upgrade replaces the daemon.
- **Two separate tokens.** The LAN access token is separate from the
  endpoint-file token and is never written where the shutdown check reads.
  (288) Rejected: one shared token, which would let any LAN browser send the
  authenticated shutdown request.
- **Browser write guards.** The browser write routes (message, cancel)
  require an Origin equal to the daemon's own origin, which stops cross-site
  form posts; the daemon sends no CORS headers. In LAN mode a request whose
  peer address and Host header are both loopback still passes by the
  localhost Host check with no token; every other request needs the access
  token. This keeps the token-free localhost pointer line and the
  localhost-only token display of (287) working, and requiring both a
  loopback peer and a loopback Host keeps the DNS-rebinding guard. (299) A
  local process can write the inbox files directly, so a localhost
  credential would guard nothing the browser guards leave open. (284)
  Rejected: a token on localhost, or on every request in LAN mode loopback
  included, which puts the credential in the pointer-line URI and so in
  agent transcripts (299), (284); one-time pairing with an owner cookie as
  260515 did, a pairing step for the same protection (284).
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

- The new drain tool is listed in the `runtime.json` of `agents-plugin/`, `agents-plugin-wsflow/`, and `agents-plugin-pi/`, as `mailbox.history` is; the wsflow capability test requires an exact match.
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
  children); the read-only page render, routes, the localhost Host check,
  and the served-page link resolver itself (core child); the image link rule
  of the static page and embed (rich-items ticket). Mapping a board image to
  the image route URL in a served page is owned here (Phase 2).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "Localhost guards instead of a token. Localhost needs no token. In localhost mode every route checks the Host header against loopback names... in LAN mode the access token takes the Host check's place." — bearing: amended by (299); the epic now keeps the Host check for loopback-peer, loopback-Host requests in LAN mode
- 7a70197d (2026-11-06, commit): "(287): LAN token persists in the machine cache so a second device's bookmark survives the daemon replacement every upgrade triggers; rejected per-start tokens." — bearing: supports
- 0358cc07 (2026-11-06, commit): "Injection as the user's own message with a whiteboard label: mail carries no user authority, but a board answer is the user's own words; the LAN token granting user-authority input was already accepted in (197)." — bearing: supports
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Localhost guards on the routes this child adds. In localhost mode every route checks the Host header against loopback names... Machine routes carry no Origin: `/nudge` needs no authentication" — bearing: constrains
- 261106-feat-mailbox-history-ring (2026-11-06, Decisions): "Storage: one file per entry, collision-free by construction... `<cache root>/mailbox-history/<session-key>/`... no lock and no read-modify-write" — bearing: supports
- 261106-feat-mailbox-history-ring (2026-11-06, Result b1771b9df): "`mailbox.history` added to the ws, wsflow, and Pi `runtime.json` contracts (the wsflow capability test requires an exact match)." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Store lives in its own pure package internal/wswhiteboard (Apply/Query/BuildView/RenderPage with an injected Env) so batch semantics are unit-tested without the MCP server" — bearing: constrains
- 260514-feat-ws-web-daemon-foundation (2026-05-14, Decisions): "Use a one-time startup pairing token to bootstrap browser access... successful pairing installs a normal owner session cookie." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/ drain tool, agents-plugin-tool/cmd/ws-mcp/ mailbox wait flag, agents-plugin-tool/internal/wsmailbox/wait.go, daemon routes and page scripts from daemon-core, the `dashboard.`-namespace config manifests of ws, wsflow, and Pi |
| scope.surface | public-interface | new shipped MCP drain tool, new mailbox wait CLI flag, new LAN config key, new browser write routes |
| scope.new_public_symbol | yes | drain tool name, wait flag name, and LAN config key, all settled by the worker |
| scope.new_type_contract | yes | on-disk board-answer inbox entry format and drain tool output shape |
| scope.test_surface | existing | agents-plugin-tool/cmd/ws-mcp/mailbox_test.go, agents-plugin-tool/internal/wsmailbox/wait_test.go, agents-plugin-tool/internal/mcp/ tests; daemon route tests land with daemon-core |
| complexity.reuse_points | confirmed | per-entry temp+rename store pattern in agents-plugin-tool/internal/mcp/mailbox_history.go; reply grammar in agents-plugin-tool/internal/wswhiteboard/assets/form.js build; peek-only Wait in agents-plugin-tool/internal/wsmailbox/wait.go |
| complexity.side_effect_risk | high | the daemon gains its only write target and a non-loopback bind; atomic claim races between drain, merge, and cancel |
| risk.correctness | high | drain versus cancel and merge races must yield exactly one winner with no entry both injected and cancelled |
| risk.fit | moderate | extends daemon-core server and page that have not landed; a new MCP tool likely also needs ws, wsflow, and Pi runtime.json entries as mailbox.history did |
| risk.test | moderate | race tests and multi-mode route tests are required; browser behaviour is only checked by hand with no headless browser |
| risk.security_or_contract | high | LAN token, Origin guard, image-route path containment against symlinks and dot-dot, and separation from the endpoint-file shutdown token |

## Phases

### Phase 1: Board-answer inbox and message submission path

Add the per-session board-answer inbox; the daemon's message route (writes
an entry, or merges into the held entry) and cancel route (claims and
removes a held entry, returning its text and chips); the Origin check on
both; the composer's answer chips from board cards, the sent marks, and the
held bubble with its divider and cancel control in the conversation, all
offered only for a session the adapter-presence signal (297) marks as having
an adapter (a session without one gets only copy-answers, and the message
route refuses it); the adapter-only ws-mcp drain tool that claims entries
atomically and then nudges the daemon's `/nudge` so the held bubble clears;
and the `mailbox wait` flag that also watches the inbox and reports which
kind arrived.

The worker settles and records in the Result:

- The inbox location and entry format. Constraint: both the daemon (from the
  session index) and the session's own ws-mcp derive it without a session
  key reaching the browser; readers ignore unknown fields.
- The drain tool's and the wait flag's names and output shape.
- How the daemon tells a held entry from a drained one for the held bubble.
- What the sent mark derives from after the entry is drained, and whether
  it survives a page reload.

Verification: Go tests in `agents-plugin-tool/internal/mcp/` (drain tool:
atomic claim, a claimed entry not drained twice, compact text output, an
adapter-only description, a nudge sent after a drain) and in
`agents-plugin-tool/cmd/ws-mcp/mailbox_test.go` and
`agents-plugin-tool/internal/wsmailbox/wait_test.go` (the wait flag wakes on
an inbox entry and reports the kind, and without the flag an inbox entry
does not wake it), daemon route tests (an entry written per send, a second
send merged as a separate paragraph with the later chip for one item ID
winning, cancel removing a held entry and returning its text and chips,
cancel refused on a claimed entry, a send racing a drain writing a new
entry instead of merging into the claimed one, a cross-origin or Origin-less
POST refused, no CORS headers, a session without an adapter rendering no
composer or chips and its message route refusing, the sent mark per the
rule settled above), and a race test where drain and cancel, and
drain and merge, contend for one entry and exactly one wins with no entry
both injected and cancelled, then `go test ./...` in `agents-plugin-tool`.
No headless browser is available here, so the lead checks by hand on a live
page: clicking options and acknowledgements attaches and replaces chips, a
chip note, the sent answer block text, sent marks with cards still
editable, a held bubble with a divider after two sends, and cancel returning
text and chips to the composer.

### Phase 2: LAN opt-in, access token, and image route

Add LAN mode: the config key that turns it on (in each package's
`dashboard.`-namespace manifest, default off, its manifest description
saying that only the global layer applies), binding beyond loopback, and the
persisted access token (287), separate from the endpoint-file token (288).
In LAN mode a request whose peer address and Host header are both loopback
passes by the localhost Host check with no token, and every other request,
on every route including the Phase 1 write routes, needs the token (299);
`/` shows the token only to such a localhost viewer. The daemon honors the
LAN key only from the global config layer and applies it at the next daemon
start (298). The spawning ws-mcp compares the global-layer LAN value with
the mode the daemon reports in `/health` and runs the normal handoff when
they differ, so a change takes effect without waiting for an upgrade. Add
the image route, which serves an image a board item references only when the
resolved file is inside that session's project root, and extend the core's
injected served-page link resolver so a board image maps to its image route
URL.

The worker settles and records in the Result:

- How the image route resolves a path and checks containment. Constraint:
  symlinks and `..` segments cannot reach outside the project root; the
  route refuses every other path in both modes. (238)
- How the global-layer LAN value reaches the daemon, for example as a
  spawn flag the way the port does (286). Constraint: a project or repo
  layer value never turns LAN on. (298)
- How the token travels: query parameter or cookie (EventSource cannot send
  headers), and whether rendered links and the SSE URL carry it.

Verification: Go tests cover LAN mode refusing a request without the token
and accepting one with it on every route, a request with a loopback peer and
a loopback Host passing without the token, a loopback peer with a
non-loopback Host and a non-loopback peer with a loopback Host both needing
the token, a project-layer LAN value leaving LAN off while a global-layer
one turns it on, the endpoint-file token refused as a LAN token, the token
surviving a daemon restart and shown on `/` to a localhost viewer but not to
a LAN one, deleting the token file yielding a new token, the token reaching
the SSE stream by the settled transport, a board image in a served fragment
linking to the image route URL, the LAN key defaulting off in each package's
`dashboard.` manifest with a description saying that only the global layer
applies, toggling the global LAN value making the next ws-mcp start replace
the daemon through the handoff, localhost mode unchanged, and the image
route serving a file under the project root while refusing an absolute path
outside it, a `..` escape, and a symlink pointing outside, then `go test
./...` in `agents-plugin-tool`. The lead checks by hand from a second device
on the LAN that the page needs the token, shows the same content as
localhost (argument previews included), and loads a board image.

## Sage Review Round 1 (2026-11-06)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | LAN mode's loopback contract contradicts the localhost token display and the pointer line | important | missing |
| 2 | Image route has no link pointing at it; link rule for daemon-served pages is unowned | important | autonomous |
| 3 | The LAN key has the same manifest-namespace problem as the core keys | minor | autonomous |
| 4 | How the token travels is unstated | minor | autonomous |
| 5 | The model can call the drain tool | minor | autonomous |
| 6 | Cross-ticket [browser-interaction, daemon-core]: LAN loopback contract versus the epic's token-replaces-Host-check | important | missing |
| 7 | Cross-ticket [daemon-core, browser-interaction]: no owner for link resolution in daemon-served pages | important | autonomous |
| 8 | Cross-ticket [all four]: phase-level ordering cannot be typed edges | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | LAN config key scope for a machine-wide daemon is unspecified | important |
| 2 | Composer gating for hosts without an adapter has no phase task or check | important |
| 3 | What merge refused on a claimed entry means for the user's send is unclear | minor |
| 4 | Lifetime of the sent mark after drain is unstated | minor |
| 5 | Nudges after draining has no stated target | minor |
| 6 | Token rotation by deleting the file is not tested | minor |
