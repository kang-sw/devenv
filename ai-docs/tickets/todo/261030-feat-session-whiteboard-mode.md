---
title: Session whiteboard - a server-assembled decision board and dashboard for the lead's user channel
---

# Session whiteboard - a server-assembled decision board and dashboard for the lead's user channel

## Background

The lead's asked items (Open Decision Queue entries, discussion items) and
explanations are hard to read in terminal prose, and the model cannot draw
there. Two viewing situations exist:

- **Live side window.** A terminal and a browser page open side by side; the
  user keeps one local page per lead session open and watches it.
- **Inline host snapshot.** A host that embeds HTML in its chat (Orca, through
  `::orca-visual`) shows a board snapshot almost every turn.

A downstream field report (libhbs graph-lead session, 2026-11-04) ran the
inline case by hand and hit four problems:

1. The lead carried the "next ID" by hand in agenda blobs; the blobs drifted
   to different values, worse across compaction.
2. Each topic got its own hand-built page, and one page dropped the items
   settled in the previous round. Rebuilding from memory drops items.
3. Every render rewrote the full CSS and structure for a few changed items.
4. A vague user reply was read as the answer to an open policy item; the
   inference lived only in one chat line, where the user could not catch it.

Problems 1, 2, and 4 are data problems, not rendering problems. The original
design of this ticket (lead hand-edits the HTML) had the same failure modes.
This ticket therefore puts a structured item store in the MCP server and
renders both views from it.

Orca inline-view probe (downstream, 2026-11-04, Chrome 150 on Windows): the
view is a sandboxed iframe with an opaque origin and no `allow-same-origin`.

| works | blocked |
|-------|---------|
| JS, click handlers, `details/summary`, tabs, text input (Korean IME), checkbox, radio, select, range | `localStorage`, `sessionStorage`, cookies, IndexedDB (SecurityError) |
| Frame auto-grows with content | `fetch` to any host |
| `<script src>` from cdn.jsdelivr.net only; inline SVG; canvas 2D; ResizeObserver | images and scripts from other hosts |
| Copy through `document.execCommand('copy')` | `navigator.clipboard` read/write |
| Paste into a page input, text selection copy, drag and drop, file input | `window.open`, `alert`, `confirm`, Blob download |
| `target=_blank` links open externally | WebGL, Blob Worker |
| Host theme variables (`color-mix(...)` values), live `prefers-color-scheme` change | |

In-page selections are lost when the host reloads the iframe.

Naming: "blackboard" was rejected because in multi-agent literature it names
shared inter-agent memory (blackboard architecture); "canvas" was rejected
because it collides with the HTML `<canvas>` element inside an HTML file.

## Decisions

### Mode and files

- **Knob.** `workflow.whiteboard_enabled` (boolean), default off in both ws
  and wsflow, default scope global (side-by-side viewing is the user's habit,
  not a project property). Naming follows `claude.status_band_enabled` /
  `claude.mcp_watchdog_enabled`. Off by default because it changes the
  model's output channel. A knob change applies at the next workflow-manual
  load.
- **Knob gates the store too (user decision (68a)).** The item store, the
  whiteboard tools, and every store-backed manual rule apply only when the
  knob is on. Knob-off sessions keep today's behavior (no server counter,
  discussion unpersisted), so problem 1 remains for them.
  Rejected: (68b) the store always on as every session's ID authority with
  the knob gating only rendering (extends the reversal of "Discussion keeps
  no counter" to every downstream session; lifting the gate later is
  additive).
- **Location (user decision (69)).** Files live in `ai-docs/whiteboards/` at
  the session root:
  - full page `<reply-id>.html`;
  - its stamp `<reply-id>.stamp.js`;
  - embed snapshots `<reply-id>-<short-hash>.html`.

  `<reply-id>` is the session's mailbox reply id (`wsmailbox.ReplyID`)
  without the `id:` prefix. `ai-docs/whiteboards/` is registered in the
  repository's local `.git/info/exclude` (common dir, so every worktree is
  covered), in the idempotent manner of `registerPoolExclude`
  (`internal/mcp/worktree_tools.go`).
  Rejected:
  - `.ws-workflow/whiteboards/` (the 10-30 location): a hidden dot directory
    the user has a harder time finding.
  - A `.local.html` suffix: redundant with the exclude entry, and not
    covered by the `ai-docs/**/*.local.md` ignore pattern anyway.
  - A new root dir `.ws-blackboard/` (root clutter).
  - The session key as the file name: the key is a lead-scope capability
    (`session_auth.go`) and a file name exposes it to any directory listing;
    the reply id is the existing public, deterministic handle already shown
    in the status band.
- **GC.** At workflow-manual load, at most once per day per repository,
  bookkept with a marker file in `ai-docs/whiteboards/` (the `maybePrune`
  marker pattern, `internal/mcp/session_prune.go`): delete every whiteboard
  file (full page, stamp, embed snapshots) whose reply id matches no live
  session key record.
  Rejected: mtime-based expiry (deletes a live session's rarely-touched
  page); hooking the machine-global key prune (it does not know per-repo
  directories). There is no explicit session-delete path; key expiry is the
  deletion.

### Store

- **Persisted item store with rotation (user decision (48a)).** Discussion
  items and other asked items persist in a server-side store with
  server-minted IDs, statuses, and the user's verbatim answer quote.
  Rejected: (48b) a server-minted ID counter without item state (fixes only
  problem 1); (48c) no persistence (problems 1, 2, and 4 stay structural).
- **Session-scoped (user decision (53a)).** The store lives in the session
  state record beside the agenda and todos: `lead-revive` restores it, key
  expiry deletes it. Hand-off of an item to another track is out of scope.
  Rejected: (53b) a clone-shared board with an owning session per item (needs
  ownership and concurrent-write rules; the downstream cross-track hand-off
  happened once).
- **Item schema.**

  | field | notes |
  |-------|-------|
  | `id` | server-minted, `(nn)` / `(nnn)` |
  | `class` | `policy` / `technical` / `announced`, the workflow manual's `### Asking the user` classes |
  | `topic` | groups items on the page |
  | `title` | one line |
  | `options`, `recommendation` | policy items; options are lettered a, b, c on the page |
  | `rationale` | technical and announced items (the citation or reasoning) |
  | `status` | `open`, `answered`, `assumed`, `withdrawn`, `superseded` |
  | `answer`, `answer_quote`, `answered_at` | `answer_quote` keeps the user's verbatim words |
  | `links` | ticket stem, commit, other item IDs (for example `supersedes`) |

- **Answer interpretation stays with the model.** The server enforces
  structure only: `answered` requires `answer_quote`; `assumed` marks an
  answer the lead inferred and renders highlighted until the user confirms
  or corrects it.
  Rejected: a server-side answer parser (the server cannot see the
  conversation to verify a quote, and natural-language parsing in Go is
  brittle; the quote's value is visibility, which already addresses
  problem 4).
- **Rotation by count.** The board shows the 10 most recently settled items
  and collapses older settled items into one "settled N" line per topic.
  Closing a topic collapses it at once. The archive keeps at most 500 settled
  items per session, evicting the oldest, and stays queryable until evicted.
  Rejected: time-based rotation (unrelated to conversation pace) and
  render-count-based rotation (renders also come from todo and agenda
  writes, so the count races).
- **Ticket queues are read, not owned.** A ticket's `## Open Decision Queue`
  section stays the authority. The board reads it through `watch`:
  - each render parses the watched tickets' sections (`(nn) [status] [class]
    <line>` and the last-ID line);
  - ID minting returns one past the maximum of the session counter and every
    watched last-ID line;
  - an item present in both the store and a watched ticket renders once,
    from the ticket.

  Rejected: one shared store written by both the ticket and the board (two
  writers need synchronization, and lead-ticket owns the queue); scanning
  every ticket that has a queue (shows other sessions' queues on a
  session-scoped board); dual-writing ticket items into the store
  (contradicts the ticket authority).

### Tools

- **Two tools (user decision (63)).**

  ```text
  whiteboard.update(session_key,
    add?:          [{class, topic, title, options?, recommendation?, rationale?, links?}],
    answer?:       [{id, answer, answer_quote, assumed?}],
    patch?:        [{id, title?, options?, recommendation?, rationale?, links?, status?}],   # status: open | withdrawn | superseded
    close_topics?: [topic],
    watch?:        [stem],
    unwatch?:      [stem],
    now?:          html)                     # empty string clears the Now slot
    -> minted ids in `add` order, full: <path>, embed: <path>
  whiteboard.query(session_key, topic?, status?, id?)    # read-only, includes archived items
  ```

  - A batch applies in order; `links` refer to existing IDs only.
  - A call with no arguments re-renders.
  - Both tools are lead-only, follow the existing schema/precheck path in
    `Server.callTool`, declare every argument they read, and refuse with a
    knob-off message when the knob is off.
  - The array-of-object arguments reuse the typed-array rejection from
    `260920-bug-ws-mcp-input-shape-coercion-traps`, with a type-accurate
    message when a JSON string arrives where an array is expected.

  Rejected: eight single-operation tools (`item_add`, `item_answer`,
  `item_update`, `topic_close`, `now_set`, `ticket_watch`, `render`,
  `query`) and a three-tool split (`items`, `render`, `query`): more tools
  for the same operations. Also rejected: a `board.*` namespace (collides
  with "ticket board") and one tool with a generic `op` union (weaker
  per-field schemas).
- **One render per update (user decision (52)).** Every `whiteboard.update`
  call ends in a render that overwrites the full page, bumps its stamp, and
  writes a new embed snapshot. It returns both paths, whether or not the
  host embeds:

  ```text
  full: ai-docs/whiteboards/<reply-id>.html
  embed: ai-docs/whiteboards/<reply-id>-<short-hash>.html
  ```

  Rejected: re-rendering the full page on every store mutation with embeds
  on request only (one reload per mutation instead of one per call); the
  10-30 design where the lead edits the page and calls
  `ws/whiteboard.refresh` (every render pays the full structure in tokens,
  and a hand-maintained page drops items; replaces 10-30 user decision
  (43)).
- **Implicit full render for todo and agenda.** With the knob on, the
  existing todo write tools, `agenda.set`, and `agenda.clear` also re-render
  the full page (stamp bumped), never an embed. Each section provider
  declares which tool calls dirty it. Bursts need no server debounce: the
  loader polls every 1-2 s, and the dirty-form deferral below protects
  input.
  Rejected: implicit embeds (an embed is a per-turn record the lead chooses
  to emit; implicit ones become orphan files); no implicit render (the
  dashboard shows stale todos until the next update).
- **Embed syntax stays out of the server.** The tool returns the embed path
  only; the host's embed syntax (for example `::orca-visual`) comes from host
  instructions.
  Rejected: returning a ready `::orca-visual{...}` line (puts one host's
  syntax into shipped tool output, Architecture Rule 3).

### Pages

- **One store, shared components, two layouts (user decision (56b)).** Both
  pages render from the same store and the same section components; only
  the layout differs.
  - Full page: a current-state dashboard (10-30 user decision (41)), updated
    on every render. Resolved items move to the rotation's recently settled
    area instead of disappearing.
  - Embed snapshot: a point-in-time record. Open asks and the items new or
    changed since the previous embed come first, each change with a "new"
    badge; the rest is collapsed.

  The exact layout is settled by the Phase 1 mockup review.
  Rejected: (56a) the embed as a copy of the full page (the whole dashboard
  lands in chat every turn); (56c) two separate features (repeats problem 2
  at feature level).
- **Core sections only (user decision (57)).** The core sections are:
  - open asks: policy items with options, then technical and announced items
    to acknowledge;
  - recently settled;
  - Now, an HTML fragment slot;
  - todos in full;
  - agendas as key plus the one-line summary `agenda_list` already produces.

  The renderer is a list of section providers plus a per-view layout. The
  Phase 1 mockup shows mailbox, playbook, and ticket-board providers as empty
  placeholder panels so the layout can be judged with them. Phase 3 ships
  the provider interface but registers no empty panels.
  Rejected: agenda blob bodies on the page (they are the lead's working
  notes and may hold unconfirmed reasoning); hard-coded sections; shipped
  "coming later" panels (noise on every render).
- **Drawing stays on the whiteboard.** Explanations and drawings go in the
  Now slot: the lead passes an HTML fragment through `now`, and the server
  wraps it. Large standalone visuals go to a `ws/path.generate` file linked
  from the board.
  Rejected: a separate drawing page (drawing motivated this feature, the
  primary-channel decision puts explanations on the whiteboard, and a second
  page recreates problem 2).
- **Collapsible panels.** Todos, agendas, and later auxiliary sections sit in
  `<details>` panels. In the embed they start collapsed; it has no storage.
  On the full page their open state survives reloads through
  `sessionStorage`. Open asks and the Now slot never collapse.
  Rejected: always-expanded panels (the embed frame grows with its content,
  and chat length is a cost).
- **Answer round trip.** The form component is shared by both pages:
  - each open policy item gets option buttons and a note field; each
    acknowledgement item gets a checkbox;
  - a "copy answers" button builds a reply such as
    `(62) b; (64) a, note: <text>; (44) ok`;
  - copying uses `document.execCommand('copy')` and falls back to selecting
    a textarea;
  - the lead applies the pasted reply through `whiteboard.update` `answer`,
    and each item's `answer_quote` is its segment of the pasted text.

  Rejected: `navigator.clipboard` (the embed sandbox denies it).
- **Dirty-form deferral.** While an answer form is dirty or focused, the full
  page's loader defers its reload and shows an "update available" banner
  instead. It reloads when the user clicks the banner or after a copy.
  Rejected: immediate reload, which discards a half-filled answer whenever
  the board changes. The embed's own reload is controlled by the host and
  cannot be guarded, so the form stays short.
- **Self-contained pages.** Scripts and styles are inline, with no CDN
  (jsdelivr included). Colors are written as `var(--card, <fallback>)`, so
  the embed takes the host theme variables and the full page uses the
  fallbacks. Drawing stays within inline SVG and canvas 2D.
  Rejected: CDN libraries; without them the same page works on `file://`,
  offline, and in other hosts.
- **Change signal (10-30 decision, carried over).** The full page's `<head>`
  holds a loader block:
  - every 1-2 s it injects `<script src="<reply-id>.stamp.js?t=<now>">`;
  - it records the first stamp seen;
  - on a different stamp it calls `location.reload()` (subject to the
    dirty-form deferral) and restores the scroll position through
    `sessionStorage`.

  A render writes the page first, then `<reply-id>.stamp.js` containing
  `wbStamp("<version>")` through a temp file and rename, so a reload never
  reads a torn page.
  Rejected:
  - Blind periodic reload: resets interactive state even when nothing
    changed.
  - `fetch`/XHR or iframe DOM read of the html: blocked for `file://` pages,
    which are opaque or unique origins in Chrome/Edge and in Firefox
    defaults.
  - A sidecar data `.js` carrying section HTML in template literals:
    escaping of backticks and `${` is messy.
  - File System Access API: Chromium-only, needs a re-grant per tab.
  - A local HTTP server with SSE: port and lifecycle burden, and a local file
    is the cleanest medium.

### Workflow text

- **Workflow-manual section (rendered only when the knob is on; compact,
  since the manual is injected every session).**
  - The whiteboard is the primary channel (10-30 user decision (40a)).
    Every item asked of the user (discussion items and Open Decision Queue
    items) and every explanation goes on the whiteboard through
    `whiteboard.update`. The terminal or chat then carries only a short
    pointer, or the embed line when the host's instructions give an embed
    syntax.
  - Order per turn: one `whiteboard.update` with the turn's changes, then
    the pointer or embed line.
  - The user answers by ID in the terminal or chat, typed or pasted from
    "copy answers".
  - Skill-reserved asks (the lead-review next-step handoff, the lead-ship
    publish confirmation, Approval Protocol asks) remain asks, shown on the
    whiteboard.
  - Output lines a playbook mandates in a fixed format (for example
    `Ticket: <path>`) stay in the terminal.
  - Lead sessions only; workers and subagents do not use the whiteboard.
  - The text is host-neutral (Claude, Codex, Pi).
- **Knob-on variants of existing rules.**
  - The workflow manual's `### Asking the user` rule "Discussion keeps no
    counter" gains a knob-on variant: IDs come from the store.
  - lead-discuss's "nothing here is persisted" gains a knob-on variant: items
    persist in the session store, which is not a decision record; only
    lead-ticket persists decisions.
  - lead-ticket gains a knob-on step: `watch` the ticket whose queue it
    opens.
- **Claude status band (claude-mod).**
  - Row 3:
    - Left: the full page path relative to the session root, computed from
      the reply id the band already reads plus the knob. It shows even
      before the file exists. Make it an OSC 8 link to the absolute
      `file://` URL when the host's element table supports links.
    - Right end: one capsule `todo N/M · K agendas`.
  - Row 4 is removed.
  - With the knob off, row 3 left falls back to the focus todo (current
    behavior).

## Constraints

- Manuals that bind this work: `ai-docs/manuals/shipped-surface-boundary.md`
  (tool strings, manual and playbook text, page templates ship downstream),
  `ai-docs/manuals/skill-authoring.md` (manual and playbook prose),
  `ai-docs/manuals/wsflow-mirroring.md` (knob in both config manifests;
  manual, playbook, and band mirrors), `ai-docs/manuals/ws-mcp.md` (new
  tools).
- Remote sessions without local file access are out of scope for the full
  page; the knob stays off there.
- Out of scope:
  - hand-off of items between sessions;
  - section providers beyond the core set;
  - serving the pages over HTTP in any form (local server, SSE, websocket),
    per the change-signal rejection; both pages are plain files.
- Unverified assumption: an inline host can embed a file at
  `ai-docs/whiteboards/`. The downstream session kept its pages in the Orca
  chat scratch directory, and nothing has shown whether Orca accepts other
  paths.

## Phases

### Phase 1: HTML mockup

Build two static, self-contained mockups with sample data under
`ai-docs/ref/whiteboard-mockup/`: `full.html` (dashboard layout) and
`embed.html` (snapshot layout). They follow the Pages decisions and show:

- open policy items with lettered options and a recommendation;
- technical and announced items with acknowledgement checkboxes;
- one `assumed` item, highlighted;
- recently settled items and a collapsed "settled N" line;
- a Now slot holding an inline SVG drawing;
- todo and agenda panels;
- empty mailbox, playbook, and ticket-board placeholder panels;
- the answer form with a working "copy answers" button;
- a mocked "update available" banner on the full page;
- "new" badges on the embed;
- theme variables with fallbacks.

Stop for the user's review. Revise until the user approves the layout. The
approved mockup is the template reference for Phase 3. It stays tracked
until Phase 3 absorbs it, so the Phase 3 worker can find it; an untracked
`ws/path.generate` artifact was rejected for that reason.

### Phase 2: Probe file:// behavior on Windows browsers

Verify on native Windows Chrome and Edge, with a `file://` page:

- the page can repeatedly inject `<script src="x.stamp.js?t=<now>">` and
  observe a rewritten stamp within one poll interval (no stale cache);
- the page reloads with scroll restoration;
- `sessionStorage` survives the reload;
- `document.execCommand('copy')` works.

Also check a WSL path opened through `\\wsl.localhost\...`. The Orca probe
does not cover these, because it ran in a sandboxed iframe, not a `file://`
tab. Record the result. If re-injection does not observe changes, stop and
return to the lead before Phase 3, because the change signal depends on it.

### Phase 3: Implement the whiteboard

Implement every item in Decisions:

- the knob in both config manifests;
- the store in the session state record;
- `whiteboard.update` and `whiteboard.query`;
- the render of the full page, stamp, and embed snapshot from templates
  derived from the approved mockup;
- the implicit full render from todo and agenda writes;
- ticket-queue watching;
- exclude registration and GC;
- the knob-gated workflow-manual section and the lead-discuss and
  lead-ticket variants, with wsflow and Pi mirrors;
- the status band row-3 rework.

Remove `ai-docs/ref/whiteboard-mockup/` once the templates absorb it.

Verification:

- Go tests in `internal/mcp`:
  - a batch applies in order and returns minted IDs in `add` order;
  - minting exceeds every watched ticket's last-ID line;
  - an item in both the store and a watched ticket renders once, from the
    ticket;
  - `answered` without `answer_quote` is refused;
  - a JSON string passed for an array argument is refused with a
    type-accurate message;
  - rotation keeps 10 visible settled items, collapses the rest per topic,
    caps the archive at 500, and `close_topics` collapses at once;
  - `query` returns archived items;
  - each update writes the full page, a new embed, and the stamp, with the
    stamp written after the page through a temp file and rename;
  - todo and agenda writes re-render only the full page, and only with the
    knob on;
  - both tools refuse when the knob is off;
  - the exclude entry is added once (idempotent);
  - GC removes orphaned files and keeps live ones, at most once per day;
  - the manual section and the playbook variants render only when the knob
    is on.
- wsflow mirror and Pi tree drift tests pass.
- claude-mod engine tests for the row-3 layout with the knob on and off,
  and for row 4's removal.
- A manual browser check of a real full page across several updates
  (reload, scroll and panel restoration, dirty-form deferral, copy
  answers), and of an embed snapshot opened standalone.
