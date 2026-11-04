---
title: Session whiteboard - a server-assembled decision board and dashboard for the lead's user channel
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 66e95b4fdb80ea39
sage-review-completeness-reviewed: f2eb9c58ab283e4a
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
  Declared as a built-in `workflow.*` key (technical decision (101)), like
  `workflow.prefer_subagent`: a wsconfig item plus a `configRegistry` entry
  with an on/off value, registered global-only (`RegisterGlobalOnly`) and
  listed in the lead-tune tuning catalog (technical decision (106)).
  Global-only because `handleWorkflowManual` renders the manual with
  `wsconfig.Options{}` and no repo root, so a repo-scoped override would be
  invisible to the manual gate. Rejected: allowing repo scope (needs the
  root threaded through the manual render for a setting that is the user's
  habit, not a project property). Both packages default it off, so no per-package
  default is needed. Rejected: a package `config-manifest.json` key (the
  `workflow.` namespace is reserved for built-ins in `parseConfigManifest`,
  and the shipped manifests are single-namespace `claude.`, which would make
  the knob Claude-only while the manual mandate is host-neutral); extending
  the manifest mechanism (new machinery for a default both packages share).
- **Knob gates only the manual mandate (user decision (68), 2026-11-04,
  reversing the earlier (68a)).** The item store and both whiteboard tools
  work in every lead session, knob on or off. The knob decides one thing:
  whether the workflow manual carries the whiteboard section, whose core is
  "show the user everything through the whiteboard, replacing text output
  entirely". A knob-off session can still use the whiteboard ad hoc, for
  example when the user asks for it; its discussion IDs stay lead-counted
  (problem 1 remains there), bridged by `id_floor` (see Store).
  Rejected: the earlier (68a) knob gating the store and tools as well (the
  tools then need a knob-off refusal path, which invites a model to switch
  the knob on itself to get past it; the user judged ad hoc use harmless).
- **Location (user decision (69)).** Files live in `ai-docs/whiteboards/` at
  the session root:
  - full page `<reply-id>.html`;
  - its stamp `<reply-id>.stamp.js`;
  - embed snapshots `<reply-id>-<short-hash>.html`, unless the session set
    an `embed_dir` (below). `<short-hash>` is derived from the render
    sequence number together with the content, so an embed name never
    repeats within a session even when two renders are identical (hosts
    expect a new file name per visual, and retention deletes recorded
    paths).

  `<reply-id>` is the session's mailbox reply id (`wsmailbox.ReplyID`)
  without the `id:` prefix. `ai-docs/whiteboards/` is registered in the
  repository's local `.git/info/exclude` (common dir, so every worktree is
  covered), in the idempotent manner of `registerPoolExclude`
  (`internal/mcp/worktree_tools.go`). Registration and directory creation
  happen at a session's first render, never earlier (technical decision
  (96)), so a repository whose sessions never use the whiteboard gets no
  `ai-docs/whiteboards/` and no exclude entry.
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
- **GC.** At workflow-manual load, knob on or off (a knob-off session can
  hold an ad hoc board), at most once per day per repository, and only when
  `ai-docs/whiteboards/` exists,
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
- **Host embed folder (user decision (115a), 2026-11-04).** Phase 2 showed
  that Orca embeds only a bare file name read from its chat scratch folder
  (`$ORCA_CHAT_SCRATCH_DIR`): a repository path, relative or absolute,
  renders as plain text (Orca 1.4.248 source and its shipped
  `orca-chat-visuals` skill, confirmed by a live check in the lead's Orca
  chat). So `whiteboard.update` takes an optional `embed_dir` payload field:
  - The lead passes the folder its host instructions name; the server
    stores it in the session state record before that call renders, so the
    same call's embed and every later one go there until a later
    `embed_dir` replaces it. An empty string clears it, and embeds return
    to `ai-docs/whiteboards/`.
  - A path that is not absolute or not an existing directory is refused.
    The server never creates the folder. The stored folder is re-checked in
    every update's whole-batch validation (113); when it has disappeared,
    the update is refused with a message naming it, before anything
    applies.
  - A host folder belongs to one host conversation, while a session key can
    outlive it (a revive in a new chat). So the lead passes `embed_dir` on
    its first update after the session starts or is revived; passing it
    again is cheap and replaces the old value.
  - With `embed_dir` set, the embed is written only there, under the same
    `<reply-id>-<short-hash>.html` name (already a valid Orca file name,
    since the reply id is lowercase words), and the `embed:` result line
    is the bare file name. The full page and stamp stay in
    `ai-docs/whiteboards/`.
  - Embed retention (77b) applies only in `ai-docs/whiteboards/`: an embed
    written to an `embed_dir` is never deleted by the server, and
    repository GC never touches that folder, whose lifetime the host owns
    (user decision (118b), 2026-11-04). The host re-reads a visual's file
    when it redraws chat history, so evicting there would blank earlier
    visuals, the (77a) failure; and the unbounded-count reason that
    rejected (77c) does not hold in a folder the host deletes with its
    chat. Rejected: (118a) the 5-embed cap inside `embed_dir` as well.
  - The server names no host and reads no host variable (Architecture
    Rule 3).

  Rejected: (115b) the lead copying each embed into the host folder itself
  (one extra tool call per turn, and a forgotten copy shows nothing);
  (115c) the server reading
  `ORCA_CHAT_SCRATCH_DIR` (host-specific behavior in shipped code, Rule 3)
  or a config key (new machinery for a value the lead already has); a
  per-call `embed_dir` that is not remembered (the lead repeats it every
  turn).

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
  | `id` | server-minted, or an explicit number on `add` ((112), (117)); shown as `(nn)` / `(nnn)` |
  | `class` | `policy` / `technical` / `announced`, the workflow manual's `### Asking the user` classes |
  | `topic` | groups items on the page |
  | `title` | one line |
  | `options`, `recommendation` | policy items; `options` are plain strings, lettered a, b, c by position on the page; a policy `answer` is the letter ((109)) |
  | `rationale` | technical and announced items (the citation or reasoning) |
  | `status` | `open`, `answered`, `assumed`, `withdrawn`, `superseded` |
  | `answer`, `answer_quote`, `answered_at` | `answer_quote` keeps the user's verbatim words |
  | `links` | ticket stem, commit, other item IDs (for example `supersedes`) |

- **Answer interpretation stays with the model.** The server enforces
  structure only: `answered` requires `answer_quote`; `assumed` marks an
  answer the lead inferred and renders highlighted until the user confirms
  or corrects it. A confirmation or correction is another `answer` on the
  same ID without `assumed`, carrying the user's confirming or correcting
  words as `answer_quote`, which moves the item to `answered` (technical
  decision (116)). The new quote replaces the earlier basis quote, and
  `answered_at` takes the confirmation time, so the item enters the
  rotation (109) then. Re-answering an `answered` item follows the same
  rule. Rejected: a separate confirm operation (a second path to
  the same state change). Also rejected: a server-side answer parser (the server cannot see the
  conversation to verify a quote, and natural-language parsing in Go is
  brittle; the quote's value is visibility, which already addresses
  problem 4). Also rejected: renaming `assumed` to `inferred` ((84); the
  downstream proposal's term reads clearly enough).
- **Rotation by count.** The 10 most recently settled items, counted
  board-wide, render expanded inside their own topic blocks; every older
  settled item collapses behind its topic's settled count chip (the
  approved mockup's per-topic `details.settled`). There is no separate
  board-wide settled section (technical decision (109)). Closing a topic
  collapses it at once. The archive keeps at most 500 settled
  items per session, evicting the oldest, and stays queryable until evicted.
  Rejected: time-based rotation (unrelated to conversation pace),
  render-count-based rotation (renders also come from todo and agenda
  writes, so the count races), and a per-topic cap or a standalone settled
  section (neither matches the approved mockup).
- **The board renders from the store only (user decision (102a),
  2026-11-04, reversing the earlier `watch` design).** A ticket's
  `## Open Decision Queue` section stays the authority for persistence, but
  the server never parses it. An Open Decision Queue item reaches the board
  the way every other item does, through `add`:
  - `add` takes an optional explicit `id`, so the lead can put a queue
    resumed from an earlier conversation on the board under its ticket IDs.
    The `id` is a plain number, like `id_floor` (technical decision (112));
    rejected: the parenthesized string form. An explicit `id` already in the
    store is refused; an accepted one raises the session counter to at least
    that ID.
  - ID minting returns one past the maximum of the session counter and
    `id_floor` (below).
  - The lead records each answer in both places: the store through
    `answer`, the ticket through lead-ticket, which the ticket authority
    already requires.

  Rejected: (102b) keeping `watch` with a merge contract (which source
  supplies options, recommendation, and topic; a ticket-tag-to-status table
  plus a board status for `[deferred]`; refusing `answer` on a ticket-only
  ID; what renders after the settle point), flagged undefined by both Sage
  reviews; rendering from the one-line ticket form also drops the option
  buttons. Also rejected, from the earlier design: one store written by both
  the ticket and the board (two writers), and scanning every ticket that has
  a queue (other sessions' queues on a session-scoped board).

  `id_floor` (technical decision (97), shaped by the user): an optional
  `update` payload field. A knob-off session counts discussion IDs by hand,
  and the server cannot see the conversation, so its first minted ID could
  repeat one already used in prose. When `id_floor` is given, the session
  counter is raised to it, so the next minted ID is one past the floor. A
  floor at or below the counter is ignored silently: it never lowers the
  counter and never errors, since lowering would let the store mint a duplicate of its
  own items. Knob-on sessions need not pass it, because every item already
  goes through the store.
  Rejected: refusing a low floor (the user judged ID bookkeeping not worth
  strictness).

### Tools

- **Two tools (user decision (63)).**

  ```text
  whiteboard.update(session_key, payload: {
    add?:          [{id?, class, topic, title, options?, recommendation?, rationale?, links?}],   # id: adopt a ticket-queue ID
    answer?:       [{id, answer, answer_quote, assumed?}],
    patch?:        [{id, title?, options?, recommendation?, rationale?, links?, status?}],   # status: open | withdrawn | superseded
    close_topics?: [topic],
    now?:          html,                     # empty string clears the Now slot
    now_path?:     path,                     # exclusive with `now`
    id_floor?:     number,                   # see Store
    embed_dir?:    path})                    # absolute; remembered for the session, "" clears (115a)
    -> minted ids in `add` order, full: <path>, embed: <path>
  whiteboard.query(session_key, payload: {topic?, status?, id?})   # read-only, includes archived items
  ```

  - **Minimal advertised surface (user decision (90c)).** `tools/list`
    advertises each tool as `{session_key, payload}` only, and each
    description is deliberately unattractive: lead-only, and "read the
    `whiteboard` playbook before first use". It does not explain what the
    board is for. The inner payload schema above and the usage text live in
    that playbook (see Workflow text). A host that loads every schema up
    front then pays almost nothing for a feature its user never turned on,
    and a model searching for "diagram" or "board" finds a tool that sends
    it to read instructions first.
    Rejected: (90a) advertising the full inner schema and a descriptive
    purpose (every non-deferred-loading session pays the schema, and the
    description invites curiosity calls); (90b) hiding both tools from
    `tools/list` while the knob is off (the server reads its tool list at
    process start and sends no `listChanged` notification, so a knob change
    would need an MCP restart, contradicting "applies at the next
    workflow-manual load").
  - **`payload` is an `object` (technical decision (92)).** A JSON string
    where the object belongs is refused with a type-accurate message, the
    rejection-over-leniency choice of
    `260920-bug-ws-mcp-input-shape-coercion-traps`. Rejected: parsing a
    string payload leniently. Uncertain: if hosts turn out to stringify
    nested object arguments often, revisit this in favor of parsing.
  - **A malformed payload is refused with the inner schema (technical
    decision (93)).** The central precheck in `Server.callTool` sees only
    `{session_key, payload}`, so its refusal would carry the useless minimal
    schema. The handler validates the payload itself and refuses with every
    problem plus the inner schema, so one retry fixes the call. This holds
    whatever the knob. Rejected: a refusal that only points at the playbook
    (an extra round trip for every malformed call).
  - A batch is validated whole before anything applies (technical decision
    (113)): any refusal, a duplicate explicit `id` included, refuses the
    whole batch with every problem, as (93) reports them. An accepted batch
    applies in order; `links` refer to existing IDs only. Rejected:
    per-entry partial apply (leaves a half-applied batch the lead must
    reconstruct).
  - A `patch` that changes `options` is refused on an item that is not open
    (technical decision (114)), because a policy answer is stored as the
    option letter (109) and new options would silently re-letter it.
    Rejected: re-opening the item implicitly.
  - `now_path` (technical decision (80)) names a file holding the Now
    fragment, so a large fragment does not travel inside a JSON tool
    argument, where escaping errors are frequent. The server reads the file
    once at update and stores its content; a later change or deletion of the
    file does not alter the board. Passing both `now` and `now_path` is
    refused. A full HTML document (`<!DOCTYPE` or `<html`) is refused; the
    slot takes a fragment only.
    Rejected: re-reading the file at every render (the board changes
    silently when the file changes or disappears).
  - An `update` with an empty payload re-renders.
  - Both tools are lead-only: `callTool` refuses them for every key where
    `isTopLevelLead()` is false, beside the role gate, so a delegated child
    key can neither write nor read the board (announced default (108), the
    note-write precedent of
    `261030-feat-note-layer-discipline-and-worker-readonly`). They follow
    the existing schema/precheck path in `Server.callTool` for their two
    top-level arguments. Neither refuses on
    the knob (user decision (68)).
  - Array-of-object fields inside the payload get the same type-accurate
    message as the typed-array rejection from
    `260920-bug-ws-mcp-input-shape-coercion-traps` when a JSON string
    arrives where an array is expected.

  Rejected: eight single-operation tools (`item_add`, `item_answer`,
  `item_update`, `topic_close`, `now_set`, `ticket_watch`, `render`,
  `query`) and a three-tool split (`items`, `render`, `query`): more tools
  for the same operations. Also rejected: a `board.*` namespace (collides
  with "ticket board") and one tool with a generic `op` union. With the
  advertised schema now opaque, that union's original drawback (weaker
  per-field schemas) no longer applies; the two-tool split stands
  (announced default (94)) because a read-only `query` beside a mutating
  `update` keeps reads safe to call freely.
- **One render per update (user decision (52)).** Every `whiteboard.update`
  call ends in a render that overwrites the full page, bumps its stamp, and
  writes a new embed snapshot. It returns both paths, whether or not the
  host embeds:

  ```text
  full: ai-docs/whiteboards/<reply-id>.html
  embed: ai-docs/whiteboards/<reply-id>-<short-hash>.html
  ```

  With an `embed_dir` set, the `embed:` line is the bare file name
  `<reply-id>-<short-hash>.html` (see Mode and files, (115a)).

  Rejected: re-rendering the full page on every store mutation with embeds
  on request only (one reload per mutation instead of one per call); the
  10-30 design where the lead edits the page and calls
  `ws/whiteboard.refresh` (every render pays the full structure in tokens,
  and a hand-maintained page drops items; replaces 10-30 user decision
  (43)).
- **Implicit full render for todo and agenda.** Once a session's board
  exists (its first `whiteboard.update` created it), the existing todo write
  tools, `agenda.set`, and `agenda.clear` also re-render the full page
  (stamp bumped), never an embed. The gate is board existence, not the knob
  (technical decision (96)): a knob-on session that has not rendered yet
  writes no file, and a knob-off session that used the board ad hoc keeps it
  current. Rejected: gating on the knob (creates pages in sessions that never
  asked for one, and leaves an ad hoc board stale). Each section provider
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
    on every render. Resolved items stay in their topic blocks under the
    rotation rule (109) instead of disappearing.
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
    its settled items: the ones among the 10 most recent board-wide
    expanded, the older ones behind a count chip (109). Topics with an open
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
  - settled items, inside their topic blocks per the rotation rule (109);
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
  reads a torn page. A rename refused by a Windows sharing violation (the
  browser polling the file holds a brief read handle) is retried a few times
  at a short interval before the render fails (technical decision (119)).
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

- **`whiteboard` playbook is the single source (technical decision (95)).**
  A new bundled playbook `whiteboard` holds the tool usage and the inner
  payload schema. It also carries the capture line shared with the knob-on
  lead-discuss variant (technical decision (100)), so an ad hoc knob-off
  user of the board reads it too: "Confirmed decisions reach a ticket only
  through lead-ticket, once the user moves to capture; the board clears with
  the session key." The line names the action (record in a ticket at the
  user's capture signal), not a disclaimer, and keeps lead-discuss's
  wait-for-the-user's-signal rule.
  - Knob off: the workflow manual carries no whiteboard text; the tool
    descriptions point at `playbook.read` for `whiteboard`.
  - Knob on: the manual's whiteboard section includes the same playbook
    content beside the mandate below, so the lead needs no extra read.

  Rejected: the inner schema in the manual unconditionally (every
  downstream session pays it, the cost (90c) removes); two hand-kept copies
  of the schema (drift).
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
  - When the host's instructions name a folder its embeds must live in, the
    lead passes it as `embed_dir` on its first update after the session
    starts or is revived, and embeds the returned bare name ((115a)); the
    `whiteboard` playbook states this without naming a host.
  - The pointer is one terminal line naming the full-page path the update
    returned and the IDs asked this turn, for example
    `Whiteboard: ai-docs/whiteboards/<reply-id>.html - (102), (103)`
    (technical decision (110)). The `whiteboard` playbook states it, so ad
    hoc knob-off use gives it too; an embed-capable host gets the embed line
    instead. Rejected: a bare "see the whiteboard" line (the user needs the
    file and which items await an answer).
  - The user answers by ID in the terminal or chat, typed or pasted from
    "copy answers".
  - Skill-reserved asks (the lead-review next-step handoff, the lead-ship
    publish confirmation, Approval Protocol asks) remain asks, shown on the
    whiteboard.
  - Output lines a playbook mandates in a fixed format (for example
    `Ticket: <path>`) stay in the terminal.
  - Lead sessions only; workers and subagents do not use the whiteboard.
  - The text is host-neutral (Claude, Codex, Pi).
- **Knob-on variants of existing rules.** They are applied code-side in the
  `printPlaybook` path, the mechanism the workflow-manual knob append
  already uses (technical decision (107)), so the rsrc mirrors stay
  byte-identical. Rejected: a new template conditional (the template engine
  has none).
  - The workflow manual's `### Asking the user` rule "Discussion keeps no
    counter" gains a knob-on variant: IDs come from the store.
  - lead-discuss's "nothing here is persisted" gains a knob-on variant: the
    capture line of the `whiteboard` playbook (see above), word for word.
    Rejected: "the store is not a decision record" (a disclaimer that names
    no action, so it does not lead the reader to record decisions; user
    review, 2026-11-04).
  - lead-ticket gains a knob-on step: when it opens or resumes a queue, it
    `add`s the queue's open items that `whiteboard.query` does not already
    return to the board under their ticket IDs, passing `id_floor` set to the
    queue's last-ID line so the store never re-mints an ID already settled
    into `## Decisions` (announced default (111), from (97), (102a), and
    lead-ticket's Queue state rule), and applies each settled answer to the
    store as well as the ticket.
- **No status band change (user decision (103), 2026-11-04).** The
  Claude status band keeps the Rows 3 and 4 of
  `261028-feat-claude-mod-status-band` unchanged; the per-turn pointer line
  above is how the user finds the board, and it also reaches users who
  turned the band off. Rejected: a band row showing the page path, with
  Row 4 removed (reverses 261028 for every band session, and misses users
  without the band); the earlier technical decision (98) is withdrawn with
  it.

## Constraints

- Manuals that bind this work: `ai-docs/manuals/shipped-surface-boundary.md`
  (tool strings, manual and playbook text, page templates ship downstream),
  `ai-docs/manuals/skill-authoring.md` (manual and playbook prose),
  `ai-docs/manuals/wsflow-mirroring.md` (manual and playbook mirrors),
  `ai-docs/manuals/ws-mcp.md` (new tools).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Remote sessions without local file access are out of scope for the full
  page; the knob stays off there.
- Out of scope:
  - hand-off of items between sessions;
  - section providers beyond the core set;
  - any Claude status band change ((103));
  - serving the pages over HTTP in any form (local server, SSE, websocket),
    per the change-signal rejection; both pages are plain files.
- Host embed limits (Phase 2, Orca 1.4.248): Orca shows no visuals for WSL
  or remote sessions, refuses symlinks and files over 512 KiB, and hands
  the HTML to its iframe as a string, so an embed stays self-contained and
  small. The server does not enforce a host's size limit.

## Prior Decisions

- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions D2): "Knobs and per-package defaults. Each default is that package's current behavior, except the ws status band (user change)" — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "Row 3 (ws focus row): the session's in-progress todo, or its first pending todo when none is in progress, as `▶ <key> <title>` bright on the left; a `todo <done>/<total>` capsule on the right." — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "The knob is `claude.status_band_enabled`, boolean, default `false`, in `agents-plugin/config-manifest.json`, read through `config_get`" (ws default later changed to true by 10cde199) — bearing: supports
- 261031-feat-claude-mod-manual-via-preload-once (2026-10-31, Decisions (61)): "One appended row per epoch carries manual, key, and state" — the manual reaches a Claude lead once per epoch via lead skill expansion — bearing: constrains
- 261030-feat-note-layer-discipline-and-worker-readonly (2026-10-31, Result): "`callTool` denies `note.write`/`erase`/`mute`/`unmute` beside the role gate to every key where `isTopLevelLead()` is false" — bearing: supports
- d17abbea (2026-11-04, commit): "Scope widened beyond the three listed playbook copies to six files: agents-plugin/rsrc/manifest.json, agents-plugin-wsflow/rsrc/manifest.json, and agents-plugin-pi/rsrc/manifest.json each change one line" — bearing: constrains
- 6d1e00d2 (2026-10-30, commit): "Settled in lead-discuss 2026-10-30: user decisions (40a) whiteboard is the primary channel, (41) current-state page with terminal pointer only, (43) new ws/whiteboard.refresh tool; name whiteboard over blackboard/canvas" — bearing: supports
- 95d18ff6 (2026-09-15, commit): "$(GitRoot) binds to the primary worktree root (worktree list first entry), so acquire called from any linked worktree resolves the one shared pool" (origin of registerPoolExclude) — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | built-in config key, internal/mcp tools and session state, rsrc playbooks and manual with wsflow and Pi mirrors |
| scope.surface | cross-module | two new MCP tools, a new config key, a new playbook, manual text |
| scope.new_public_symbol | yes | whiteboard.update, whiteboard.query, workflow.whiteboard_enabled, whiteboard playbook |
| scope.new_type_contract | yes | whiteboard.update payload and item schema (class, status, answer_quote, links) |
| scope.test_surface | new-files | no whiteboard test exists (search for whiteboard under agents-plugin, agents-plugin-wsflow, agents-plugin-tool, agents-plugin-pi found nothing); existing agents-plugin-tool/internal/mcp tests and wsrsrc/pi_mirror_test.go would extend |
| complexity.reuse_points | confirmed | registerPoolExclude (agents-plugin-tool/internal/mcp/worktree_tools.go#L216), sessionStore.maybePrune marker (internal/mcp/session_prune.go#L28), session state record (internal/mcp/session_state.go), wsmailbox.ReplyID (internal/wsmailbox/replyid.go#L26) |
| complexity.side_effect_risk | high | writes files under the repo, adds re-render hooks to todo and agenda write tools, and rewrites a manual injected into every session |
| risk.correctness | high | many interacting parts: ID minting with explicit ids and id_floor, rotation and eviction, atomic stamp write, GC by derived reply id |
| risk.fit | high | the knob is a built-in `workflow.*` key (technical decision (101)), not a package config-manifest key; the store, tools, renderer, and playbook span several modules |
| risk.test | high | browser-side behavior (reload, dirty-form deferral, copy) has only a manual check, and the surface spans Go, TS, and mirrored text |
| risk.security_or_contract | moderate | new lead-only tool surface and Now fragment HTML written to disk unescaped by design; sandboxed embed, no network |

## Phases

### Phase 1: HTML mockup [dropped]

Done at design time instead: the lead built the mockup with the user and the
user approved it. It is tracked at `ai-docs/ref/whiteboard-mockup/` (see
Pages, "Approved mockup is the template reference") until Phase 3 absorbs
it, so the Phase 3 worker can find it; an untracked `ws/path.generate`
artifact was rejected for that reason.

### Phase 2: Probe file:// behavior and the Orca embed path

A user-run probe (technical decision (104)): the worker host cannot run
Windows browsers, and a macOS check recorded as Windows coverage would be
false evidence. The worker builds a self-contained probe page, its stamp
script, a one-shot stamp rewriter, and a checklist, then hands them to the
user. The user runs the checklist on native Windows Chrome and Edge, on a
WSL path opened through `\\wsl.localhost\...`, and on macOS `file://`:

- the page can repeatedly inject `<script src="x.stamp.js?t=<now>">` and
  observe a rewritten stamp within one poll interval (no stale cache);
- the page reloads with scroll restoration;
- `sessionStorage` survives the reload;
- `document.execCommand('copy')` works.

The Orca probe from Background does not cover these, because it ran in a
sandboxed iframe, not a `file://` tab.

Also probe the Orca embed path (technical decision (105)): an embed file
placed under `ai-docs/whiteboards/` and shown through Orca's embed syntax.

The worker records the user's outcome per platform in this phase's
`### Result`. Stop and return to the lead before Phase 3 if re-injection
does not observe changes (the change signal depends on it) or if Orca
refuses the embed path (a fallback location is then a user decision).

### Phase 3: Implement the whiteboard

Implement every item in Decisions:

- the knob as a built-in `workflow.whiteboard_enabled` key;
- the store in the session state record;
- `whiteboard.update` and `whiteboard.query`, with the minimal advertised
  schema and the `whiteboard` playbook (ws canonical rsrc, wsflow and Pi
  mirrors, manifest hashes);
- the render of the full page, stamp, and embed snapshot from templates
  derived from the approved mockup;
- the implicit full render from todo and agenda writes;
- exclude registration and GC;
- the knob-gated workflow-manual section and the lead-discuss and
  lead-ticket variants, applied code-side in `printPlaybook`, with wsflow
  and Pi mirrors;
- the per-turn pointer line in the `whiteboard` playbook.

Remove `ai-docs/ref/whiteboard-mockup/` once the templates absorb it.

Verification:

- Go tests in `internal/mcp`:
  - a batch applies in order and returns minted IDs in `add` order;
  - `add` with an explicit numeric `id` adopts it and raises the counter;
    an explicit `id` already in the store is refused;
  - a batch with any refusal applies nothing and reports every problem;
  - a `patch` changing `options` on a non-open item is refused;
  - `answered` without `answer_quote` is refused;
  - `tools/list` advertises both tools as `{session_key, payload}` only;
  - a JSON string passed as `payload`, or for an array field inside it, is
    refused with a type-accurate message;
  - a malformed payload is refused with every problem and the inner schema;
  - `id_floor` above the counter raises it so the next minted ID is
    floor+1; a floor at or below the counter is ignored without error;
  - rotation keeps the 10 most recent settled items board-wide expanded in
    their topic blocks, collapses the rest behind each topic's chip,
    caps the archive at 500, and `close_topics` collapses at once;
  - `query` returns archived items;
  - each update writes the full page, a new embed, and the stamp, with the
    stamp written after the page through a temp file and rename;
  - a session keeps at most 5 embeds, evicting by render sequence rather
    than mtime;
  - `embed_dir` is remembered for the session and routes the setting
    call's own embed and later ones there only, returns the bare file name,
    is cleared by an empty string, refuses a relative or missing directory,
    refuses an update when the stored folder has disappeared, and is never
    touched by repository GC or by embed retention, which evicts only in
    `ai-docs/whiteboards/`;
  - a stamp or page rename that fails with a sharing violation is retried
    before the render fails (simulated failure);
  - two identical renders produce two distinct embed file names;
  - an `answer` without `assumed` on an `assumed` item moves it to
    `answered`, replacing the quote and stamping `answered_at`;
  - the `whiteboard` playbook states the `embed_dir` instruction without
    naming a host;
  - `now_path` content is read once and survives the file's deletion;
    `now` with `now_path`, and a full HTML document, are refused;
  - Now renders above the topics only when the render changed it;
  - todo and agenda writes re-render only the full page, and only once the
    session's board exists, knob on or off;
  - both tools work with the knob off, and refuse a key where
    `isTopLevelLead()` is false;
  - the knob is global-only and appears in the lead-tune catalog;
  - no `ai-docs/whiteboards/` directory or exclude entry exists before a
    session's first render, and the exclude entry is added once
    (idempotent);
  - GC removes orphaned files and keeps live ones, at most once per day;
  - the manual section and the playbook variants render only when the knob
    is on; the lead-ticket variant names `id_floor` from the queue's last-ID
    line and adds only IDs `query` does not return; the `whiteboard`
    playbook states the per-turn pointer line format (110); with the knob off the manual has no whiteboard text and the tool
    descriptions point at the `whiteboard` playbook.
- wsflow mirror and Pi tree drift tests pass.
- A manual browser check of a real full page across several updates
  (reload, scroll and panel restoration, dirty-form deferral, copy
  answers), and of an embed snapshot opened standalone.
- A manual Orca check: set `embed_dir` to the chat scratch folder, embed the
  returned bare name, confirm it renders, and confirm a populated board's
  embed stays well under 512 KiB.

## Sage Review Round 1 (2026-11-04)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Status band rework reverses 261028 without naming it | important | missing |
| 2 | Contract for items in both the store and a watched ticket is undefined | important | missing |
| 3 | Phase 2 requires native Windows browsers the worker may not have | important | autonomous |
| 4 | The Orca embed-path assumption has no verification step | important | autonomous |
| 5 | Knob scope versus manual-render config options | minor | autonomous |
| 6 | No mechanism named for knob-on playbook variants | minor | autonomous |
| 7 | Settled rotation versus topic-collapsed settled chips | minor | autonomous |
| 8 | Lead-only gate granularity | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Behavior of answer/patch on ticket-sourced items is unspecified | important |
| 2 | Phase 2 has no executor or environment, and no recording location | minor |
| 3 | Unverified embed-path assumption has no acceptance check or fallback | minor |
| 4 | Stale reference in Route Facts | minor |
| 5 | Settled-item rotation scope is ambiguous | minor |
| 6 | Band row 4 content and field formats are left to discovery | minor |
