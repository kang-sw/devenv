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
- **Embed retention (user decision (77b)).** A session keeps at most 5 embed
  snapshots. Each render that writes a new embed deletes that session's
  oldest embeds beyond 5. Age is the render sequence number the store records
  per embed, not file mtime (technical decision (78): two renders in the same
  second share an mtime on coarse filesystems, and copying or syncing changes
  it).
  Rejected: (77a) deleting every previous embed at render (if the host
  re-reads the file, every earlier embed in the chat history goes blank);
  (77c) keeping every embed until session GC (unbounded file count).

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
  problem 4). Also rejected: renaming `assumed` to `inferred` ((84); the
  downstream proposal's term reads clearly enough).
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
    now?:          html,                     # empty string clears the Now slot
    now_path?:     path)                     # exclusive with `now`
    -> minted ids in `add` order, full: <path>, embed: <path>
  whiteboard.query(session_key, topic?, status?, id?)    # read-only, includes archived items
  ```

  - A batch applies in order; `links` refer to existing IDs only.
  - `now_path` (technical decision (80)) names a file holding the Now
    fragment, so a large fragment does not travel inside a JSON tool
    argument, where escaping errors are frequent. The server reads the file
    once at update and stores its content; a later change or deletion of the
    file does not alter the board. Passing both `now` and `now_path` is
    refused. A full HTML document (`<!DOCTYPE` or `<html`) is refused; the
    slot takes a fragment only.
    Rejected: re-reading the file at every render (the board changes
    silently when the file changes or disappears).
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

  Rejected: (56a) the embed as a copy of the full page (the whole dashboard
  lands in chat every turn); (56c) two separate features (repeats problem 2
  at feature level).
- **Approved mockup is the template reference.** The lead built the mockup
  with the user at design time: `ai-docs/ref/whiteboard-mockup/full.html`
  and `embed.html`, self-contained, with sample items from the design
  conversation. Where the bullets below and the mockup differ in detail, the
  mockup's look and the bullets' rules both hold; the mockup's "mockup
  controls" strip and placeholder panels are mockup-only.
- **Layout.**
  - Full page: two columns, items in the main column and the panels (todos,
    agendas) in a sticky sidebar; one column below about 900 px
    ((71)).
  - Embed: one column, with the reply bar right after the topics and the
    panels collapsed at the end ((72)).
  - Topic-first grouping (technical decision (81)): each topic is one block
    titled by the topic name with per-class count chips; inside it come open
    policy items, then `assumed` items, then the items to acknowledge, then
    its settled items collapsed behind a count chip. Topics with an open
    policy item come first, then topics with an `assumed` item, then topics
    with acknowledgements, then settled-only topics, each of which collapses
    to one title line. Rejected: class-first sections across topics (the
    user loses which decision an item belongs to).
  - Now position ((73c), extended to the full page by technical decision
    (82)): Now renders above the topics when the render changed it, else
    below them. Rejected: (73a) always below; (73b) always above.
  - Policy options flow as content-sized buttons that wrap (technical
    decision (83)): short options share a row, a long option takes its own.
    Rejected: width-based grid columns (a long option wraps inside a narrow
    cell); server-side character counting (glyph widths differ across
    scripts).
  - The recommendation shows twice: a star on the option letter and a
    recommendation line under the options ((76)).
- **Design language.** Material-style surfaces (user direction during the
  mockup review): roles are told apart by filled title bars and filled
  chips, not by outlines. Topic blocks are elevated cards with a filled
  title bar; a policy or `assumed` item has a bar in its role color, and
  each acknowledgement row leads with a filled class tag; policy option
  buttons carry a faint policy-tinted fill, and a picked option keeps that
  fill and gains a glowing policy-accent outline (user direction,
  2026-11-04). Rejected: a strong tint (too loud) and a full accent fill on
  the picked option. Dark mode uses muted container tones (a dark tinted fill with
  light text), not bright fills. Rejected: the outline style of the earlier
  mockup rounds (poor readability).
- **Symbol-first chrome (technical decision (79)).** Template chrome carries
  no prose. English appears only in class tags, IDs, tooltips, and the panel
  names (Todos, Agendas); the page has no `lang` attribute. Everything else
  in the chrome is a symbol or a count, so item content in any language
  reads naturally. Rejected: a locale string table (translation upkeep for
  every downstream language).
- **Icons are inline SVG (technical decision (87)).** Chrome icons come from
  one inline `<symbol>` sprite referenced with `<use>`. Rejected: Unicode
  symbol glyphs (their metrics depend on whichever fallback font the host
  has, and they rendered misaligned on a Windows host during the review).
  Input placeholders keep a glyph because they cannot hold SVG.
- **Core sections only (user decision (57)).** The core sections are:
  - open asks: policy items with options, then technical and announced items
    to acknowledge;
  - recently settled;
  - Now, an HTML fragment slot;
  - todos in full;
  - agendas as key plus the one-line summary `agenda_list` already produces.

  The renderer is a list of section providers plus a per-view layout. The
  approved mockup shows mailbox, playbook, and ticket-board providers as empty
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
  `sessionStorage`. Open asks never collapse. The Now slot is collapsible
  and starts open ((82)).
  Rejected: always-expanded panels (the embed frame grows with its content,
  and chat length is a cost).
- **Answer round trip.** The form component is shared by both pages:
  - each open policy item gets option buttons and a note field; each
    `assumed` item gets confirm and correct buttons beside the lead's reading
    and the user's quote ((74)); each acknowledgement item gets a checkbox;
  - every item kind (policy, `assumed`, technical, announced) carries a note
    button, because a counterexample can surface against any item, not only
    a policy question (user request, 2026-11-04); a noted acknowledgement
    leaves the grouped segment and yields `(nn) ok, note: <text>` when
    checked or `(nn) note: <text>` when not;
  - a "copy answers" button builds a reply such as
    `(62) b; (64) a, note: <text>; (19) no, note: <text>; (44), (45) ok`:
    an `assumed` item yields `(nn) ok` or `(nn) no, note: <text>`, and the
    checked acknowledgements group into one `ok` segment ((75)); a confirmed
    `assumed` item without a note joins that group too ((88)), since the
    lead reads every ID in an `ok` group as confirmed either way;
  - one bulk checkbox in the reply bar, shown when the board holds any
    technical, announced, or `assumed` item, checks every acknowledgement
    and confirms every `assumed` item (user request, 2026-11-04); an
    `assumed` item already marked "correct" keeps that answer, policy items
    stay manual, and the box shows the mixed state when only some of these
    items are answered;
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
  (jsdelivr included). Surface and text colors are written as
  `var(--card, <fallback>)`, so the embed takes the host theme variables and
  the full page uses the fallbacks. Role fills (policy, technical,
  announced, `assumed`, the topic bar) are template defaults with their own
  light and dark values (technical decision (86)), because text on a filled
  bar needs a contrast the host palette does not guarantee. Drawing stays
  within inline SVG and canvas 2D.
  Rejected: CDN libraries; without them the same page works on `file://`,
  offline, and in other hosts. Also rejected: role fills from the host's
  `--chart-*` variables (a light chart color under light text is
  unreadable).
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

### Phase 1: HTML mockup [dropped]

Done at design time instead: the lead built the mockup with the user and the
user approved it. It is tracked at `ai-docs/ref/whiteboard-mockup/` (see
Pages, "Approved mockup is the template reference") until Phase 3 absorbs
it, so the Phase 3 worker can find it; an untracked `ws/path.generate`
artifact was rejected for that reason.

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
  - a session keeps at most 5 embeds, evicting by render sequence rather
    than mtime;
  - `now_path` content is read once and survives the file's deletion;
    `now` with `now_path`, and a full HTML document, are refused;
  - Now renders above the topics only when the render changed it;
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
