# Tracker mockup v3

A layout and data-model mockup for the team tracker (epic
`261107-epic-ws-personal-common-store`), inside a frame that imitates the
dashboard session page. Everything the page shows is computed in the browser
from per-clone commit lists on top of a roles file: each clone appends only to
its own ref, and every view is a client-side merge of what that clone holds.
v1 settled the frame and the cards; v2 settled threads, status, `seen`-based
concurrency and peer requests. v3 applies the board decisions of 2026-11-08,
listed in [Settled](#settled): per-clone refs and journal batching, due dates
and priorities, one unified assignment (forced or pending, with handoff), a
single derived view-state object, and four more view modes. `engine.js` is the
reference implementation the Go data-structure child ports, and
[SCENARIOS.md](SCENARIOS.md) walks nine user scenarios through it.

## Open it

Open `index.html` from the file system (double-click, or a `file://` URL). No
build, no server, no dependencies.

- `index.html` holds the frame only (header, conversation placeholder,
  composer, the panels stub handle) and loads the files below. `style.css` holds
  the frame's look and defines the dashboard's surface tokens (`--wb-*`, copied
  from the dashboard stylesheets), light and dark, with the gear menu's
  auto / light / dark theme.
- `engine.js` is the data layer (`window.TrackerEngine`). It computes causality
  from `seen`, validates each event against what its author had seen, folds the
  state, marks concurrent changes, derives the view state, and simulates the
  per-clone journal and sync.
- `i18n-en.js` defines `wsT` / `wsTn` over the English tracker strings
  (`js.tk.*`), the lookup the renderer calls for every string it draws. It is
  generated from the dashboard's English table; a Go test in
  `agents-plugin-tool/internal/wsdashboard` fails when it is stale, and its
  first line says how to regenerate it.
- `../../../agents-plugin-tool/internal/wsdashboard/assets/tracker.js` and
  `tracker.css` beside it are the view, a shipped dashboard asset:
  `window.WsTracker.mount(adapter, opts)` draws the drawer, store page, item
  window, quick switcher, context menu, and dialogs, including the status icon set. It reads
  only the view state the adapter serves and writes only by dispatching events
  through it.
- `mock-adapter.js` is that adapter over `engine.js` and `fixtures.js`, plus
  every mockup-only control: the viewer and clone selects, the clock, Go offline
  / online, the Data tab, the Support agent placeholder tab, the theme gear, the
  saved profiles, and the join simulation.
- `fixtures.js` assigns `window.TRACKER_FIXTURES`, because `fetch` of local JSON
  is blocked on `file://`. Edit it to keep a data change.

## What the page shows

- **Frame.** A header like the dashboard's, with three mockup controls grouped
  in one compact pill so the header stays on one line at 1280px (353):
  - *viewing as*: the member, labelled root, lead, member, or removed;
  - *clone*: which of that member's clones (one ref each) the page reads and
    writes; `*` marks a clone with unpushed writes;
  - *now*: the mock clock. Every write is stamped with it and advances it by a
    minute; `+1h` / `+1d` move it forward (stale and overdue use it).

  Switching viewer means the previous clone collapses its panel (it pushes,
  (241)) and the new clone opens the drawer (an immediate fetch); an offline
  clone does neither. A pull-down drawer has one handle per store. A handle
  badge counts new forced assignments, pending assignments to the viewer, and
  the derived notices (answer, reference, done, and blocked); opening the store clears it (the notification card below
  does not change what it counts). A handle shows ⊘ when the
  viewer's clone is offline or the store has no remote.
- **Store header** (500). One row: the store name, then the icon-only
  `Open as page` (its name in the tooltip and aria-label).
- **New items** (528 UI). In the list view, hovering the left gutter near a
  row edge shows a `+` badge between rows with a guide line; the mouse's x
  picks the depth among the gap's choices (root, sibling of the row above or
  of one of its ancestors, child of the row above), shown by the badge's
  indent. A click opens the inline new-item row at that spot, creating with
  `parent` and `after`. A quiet `+` ends every Items list (also an empty one)
  for a new root item at the end; it is the keyboard path. The row is title +
  Enter with a small kind picker; it stays for the next one, which goes after
  the one just made, and Esc closes it; priority, due, and assignees come
  afterwards from the new row's menu. A task or todo created there is
  assigned to its creator in the same batch (529).
  The sync state is a thin **ribbon** at the bottom of the Items card: a
  status dot, a short text ("local only", "4 to push", "offline", "synced 3 min
  ago"), and the apply button (`Sync now`, or `Close batch` without a remote)
  only while writes are pending; with nothing to push it dims to a quiet
  line. A click on it opens a popover with the diagnostics: the viewer's
  clone ref, online or offline, last synced, the member / event / commit /
  flagged counts, "N of my writes not yet pushed" with the open batch and the
  closed unpushed commits, upstream commits this clone has not fetched,
  `Sync now`, and the mock's `Go offline` / `Come back online`. The personal
  store has no remote, which is the same path as a long offline; its button
  only closes the batch.
- **Drawer layout** (366). The layout follows the window width:
  - *A* (default, about 1000px to 1600px): the **Items panel** on the left
    (about two thirds of the width) and one **tab pane** on the right with the
    tabs *Notifications*, *Support agent*, *Team*, *Activity*, and *Data*
    (debug), in that order. One tab shows at a time and uses the pane's
    height; *Notifications* is the default tab. The tabs are **icon buttons**
    (384): one inline SVG each (bell, agent, people, pulse, database; no
    external library), the name in the immediate tooltip and the `aria-label`,
    the active tab highlighted, and the "new" dot on the icon's corner. So the
    bar stays on one line at any width; the narrow layout's tabs (with *Items*
    first) and layout D's tab pane use the same icons.
  - *D* (about 1600px and up): three columns, Items | *Notifications* (always
    open, as a card) | the tab pane holding the remaining tabs.
  - *Narrow* (below about 1000px): everything is one tab pane, *Items* first,
    like the dashboard's tabs layout; the Today chips stay at the top of the
    pane whatever tab is shown.

  The switch is automatic and re-renders when the width crosses a breakpoint.
  A hidden tab whose content moved since it was last shown carries a dot, like
  the dashboard tab bar's `tdot`: an unread notification that was not there
  when the tab was last shown (on first sight, any unread one), new events for
  *Activity*, a roles change for *Team*, a new badge key for *Items* (narrow).
  Only an open drawer (or the store page) records what was shown. The tab choice is local UI
  state per store (one for A and D, one for narrow). The **drawer height
  is the user's** (382 a): dragging a store's handle sets it, like the dashboard's
  drawer, clamped between 160px and 60% of the window and saved per store as
  local UI state (localStorage), restored on reload. Dragging a closed handle
  opens its store; a drag released short of 80px closes it; a click still
  toggles. Panels scroll inside that height (the Items list, the shown tab), and
  switching tabs never changes it. Only while a store's drawer was never
  dragged does it follow its content (366), up to the same 60% maximum, so a
  short list gives a short first open. **Panels span the drawer's height**
  (389): the Items panel, the tab pane, and layout D's *Notifications* column
  always fill it whatever their content, so a short list leaves a full-height
  panel, not a short card above empty drawer; long content scrolls inside each
  panel as before. The *Support agent* tab's chat fills the pane, with the
  composer at its bottom. The store page is full-page and not affected. A card keeps its scroll position across
  re-renders. This supersedes the stacked side column of (359), the Today
  card of (358), and the per-card 70% cap of (350).
- **Store page** (371). The route `#/<storeId>` shows that store full-page, in
  place of the conversation and the drawer, with the same Items panel and tab
  pane layout (A, D, or narrow by width) filling the page. *Open as page* in
  the drawer's header goes there; *Back*, clearing the hash, or the
  browser back button returns to the session view, and reloading on the route
  restores the page. Entering the page fetches and shows what is new, like
  opening the drawer; leaving it pushes, like a collapse. An unknown store in
  the hash shows the session view with a notice. It is the only route: there
  are no item routes, because an item URL would point at the viewer's own
  daemon, so sharing one across people was rejected (369).
- **Quick switcher** (372 b). A magnifier next to the gear opens one search
  over the stores (by name and label) and the items of every store (by title,
  from each store's current view state, leaving out items hidden by a
  tombstone). An empty query lists the stores. Picking a store opens its page;
  picking an item opens its item window (on a store page, the item's store
  page first). Arrow keys move, Enter picks, Esc closes, and focus returns to
  where it was (after an item pick, to the magnifier when the window closes).
- **Items panel** (357). One panel in place of the former *My assignments*,
  *Received requests*, *Assigned by me*, *My team*, and *Items* cards. A row of
  scope presets is crossed with the view modes each scope offers:

  | scope | rows | modes |
  |---|---|---|
  | *mine* | my live assignments (forced, self-assigned, accepted, or taken over by handoff), then items I created and did not assign; the done ones sit in *recently closed* at the bottom | list / kanban (assignment cards, with the pending column) / due timeline / calendar |
  | *received requests* | answers to what I sent (declined, or returned by the accepter's lead), pending assignments to me, the ones I answered, and the ones I sent | list |
  | *assigned by me* | what I assigned or handed to others, live or pending; each assignee's state and note in the row details; done items in *recently closed* | list / kanban / due timeline / calendar |
  | *my team* (a viewer with followers) | grouped by person: each follower down to 3 levels, indented, with a group header carrying that person's open, done, blocked, stale, unaccepted, and "waiting on others" counts and last update; then their open and unaccepted work and the pending assignments they sent that nobody accepted yet | list / kanban / due timeline / calendar |
  | *all* | the tree of threads, filtered by kind (`request` = items with a pending assignment) and status (open / blocked / done / hidden); under *open*, closed items follow the rules below | tree / kanban (item cards) / due timeline / calendar |

  The panel opens on *mine*. The chosen scope and each scope's mode are local
  UI state, remembered per store in memory (they survive a viewer switch within
  the store), never an event (360). The panel is already wide, so no mode
  spans rows any more (supersedes (304)).
- **Context trees** (467). No scope list is flat: every scope's list (and
  each sub-section of *received requests*, and each person's group in *my
  team*) is a tree of its matching rows. Each match's ancestor chain stands
  above it as greyed **context rows**: they are not matches (the scope's count
  leaves them out), carry no status button, and open their item on click. The
  chain stops below a hidden ancestor (and, on *all*, below one the kind
  filter drops), so the match roots there. A
  context node's other children, unrelated to any match, fold into one
  "… N more" row (a click shows them). A match's own subtree starts folded,
  shown as an "N subtasks" chip after its title. Every node with children
  carries a ▸/▾ fold control left of its status icon (ArrowRight / ArrowLeft
  on a focused row do the same). The fold state per node is display state in
  browser storage per device, keyed by store + item id, and shared by the list
  and the due timeline. A list with no nesting at all draws no fold column.
  Roots keep the scope's own order (by their first match), children the item
  order. The *recently closed* section stays a flat list.
- **Panel header, view icons, and search** (384), (385). The header has two
  rows: row 1 is the title, the scope presets, and the scope's count chip;
  row 2 is the **view icons**, then a **search box** that takes the remaining
  width. The view switch is one icon button per mode (list or tree, kanban,
  due timeline, calendar; none on *received requests*, which has one mode),
  the name in the tooltip and the `aria-label`, the current one pressed. In
  the narrow layout both rows sit in the tool area under the tab bar. The
  search filters the current scope client-side as you type, on the item's
  title, body, project, and id (the full id and the short alias); it
  combines with the Today chip filter, applies in every view mode, and Esc
  clears it (with the box empty, Esc goes on to close the window or drawer).
  The text is local UI state per store, kept across scopes; the box keeps
  focus and the caret while the panel re-renders, and an IME composition is
  not interrupted. It also searches the *recently closed* section.
- **Recently closed** (386). In the list views (*mine*, *assigned by me*, and
  *all* under the *open* status) done items leave the main list and appear in
  a "recently closed N" section pinned to the bottom of the panel's height
  (right after the list when the list is long), greyed, expanded by default,
  covering the last 14 days of the page clock; older ones, counted in the
  header ("N older through search"), are reached through search, which lists
  every matching closed item whatever its age. The section's open or
  collapsed state is local UI state per store. The view state carries no
  close time, and none was added: a record row's close time is the done
  assignment's `lastUpdate` (the ts of the close or confirm write), an item
  row's is the latest of that over its live done assignments, falling back to
  the item's `lastActivity`. *Kanban* keeps its done column; the due timeline
  and calendar keep showing ended items, muted. *Received requests* and *my
  team* list requests and open work, so they have no closed section.
- **Closed items in the tree** (387). The *all* tree stays a tree. Under its
  parent, closed children among open siblings fold into one "closed N" row
  (N counts the items hidden in it, descendants included), expandable on
  click, its state local UI. A closed parent with an open descendant stays in
  the tree, greyed. A closed parent whose whole subtree is closed moves, with
  that subtree, as one unit to *recently closed* (when its parent is not on
  screen; under a shown parent it is part of that parent's "closed N" row).
  The parent's roll-up (done 3/5) still counts the closed children. A node
  folds with ▸/▾ like any context-tree node. While a search or a Today chip is
  on, or another status filter is chosen, the tree shows the matches as a
  context tree instead (open and greyed ones in the list, closed ones in the
  section).
- **Today chips** (366), in the Items panel header (in the narrow layout, at
  the top of the tab pane): *my work N · answer N · confirm N*, that is my
  open work, pending assignments waiting for my answer, and closes waiting for
  my confirm, always visible whatever scope is shown ((358) intent). A chip
  narrows the panel to its rows as a filter on the current scope and view mode
  (a row with an assignment record matches on the record, an item row on the
  item); the same chip again, or its clear button, removes the filter. When
  the current scope shows none of the chip's rows, the click also moves the
  panel to the scope that holds them (*mine*, *received requests*, *assigned
  by me*), so a chip never opens on an empty panel. In the narrow layout a
  chip also switches to the *Items* tab. The filter is local UI state per
  store, like the scope.
- **Item rows** (356). By default a row shows only: the status icon, the title,
  the assignees' initials (pending ones dashed, the viewer's filled, more than
  three as `+N`), the priority name (always; above the default level in the
  danger colour), the due date (always), a warning icon when the item's thread
  has a flagged write or a concurrent change, and an unread dot when the item
  has an unread *Notifications* entry. The due date reads `D-N` within 7 days,
  `D-day` today, `D+N` past due in the danger colour, and a short date
  (`11-21`) farther away; on a done or ended row it is a muted short date.
  Everything else is the row's detail overlay: kind chip, source clone and
  event id (`maya.pc-5`) and creator, project, the status description and
  "set by agent", roll-up and "blocked below", stale, the assignment path
  (source, completion mode, lead chain), the note, created / edited / last
  activity dates, and flagged and concurrent counts. A row on an assignment
  scope shows that assignment record's own state; the item's status is in the
  details when it differs. The detail overlay stays as the row's **quick
  preview** (it is how touch and keyboard reach the details, (354)); the item
  window holds the full view. It is placed next to the row in the viewport, so
  a short, content-height card does not clip it.
- **Context menu** (473), (471 a), (472 b), (474). One menu serves every
  item: list and tree rows, kanban cards, calendar entries, due-timeline rows,
  and the item window header open it alike, by right-click, a "⋯" button
  (shown on hover or focus, always on touch layouts and in the window
  header), or Shift+F10 / the menu key on a focused row. A click on a status
  icon the viewer can set opens the same menu showing only its status part.
  It shows only what the viewer's role and the item's state allow:
  - *status* radios (todo / doing / blocked / done; done reads "request done"
    when the assigner confirms) that apply on click, with a note field under
    them: a note + Enter applies the status together with the note (arrowing
    over the radios picks one without applying it, for the note's Enter);
  - Accept / Decline on a pending assignment to me, Confirm / Reject on a
    close I must confirm, Return on a follower's accepted assignment (direct
    lead only), Withdraw on a pending assignment I sent, Reopen on my done
    assignment; a note-taking one opens a note step inside the menu;
  - *Add subtask*: an input row one level in, right under the row (in the
    item window's Subtasks for a kanban card, a calendar entry, or the window
    header); title + Enter creates a task assigned to its creator (529) with
    the default completion and keeps the row for the next sibling, Esc
    closes it;
  - *Priority ▸* (radios) and *Due date ▸* (a date) for the creator;
    *Hand off ▸* (a searchable people list, with an optional note) on my live
    assignment; *Assign ▸* for the creator (the search popover of (530):
    people and subtree rules as chips, and the completion mode);
  - *Edit* (title, body, and project in place in the item window), *Open*,
    and *Hide item*, with an admin's Edit / Hide on a removed creator's item.
  Submenus are steps inside the same panel with a Back row. Arrow keys move,
  Enter applies, ArrowRight opens a step and ArrowLeft goes back, Esc steps
  back and then closes, Tab stays inside, and focus returns to the opener.
  Only the store-wide forms stay modal (profile, admin, join, "+ store").
  Keyboard focus on a row (Tab) shows its details as a hover does, and Enter
  opens the item window; on **touch**, a tap selects a row and shows its
  details and "⋯", and a tap on the selected row (or on any title) opens the
  item window. A click on a row opens it with a mouse. The selection is local
  UI state.
- **Status icons** (355). Each status differs by shape as well as colour: todo
  ring, doing half-filled, blocked slashed ring, awaiting confirm hourglass,
  done filled check, pending dashed ring with `?`, unassigned dotted ring,
  declined / returned / handed off / withdrawn as muted rings with a mark, and
  a speech bubble for a topic. Icons, initials, priority, due, and action
  buttons share one immediate custom tooltip (not the browser's delayed
  `title`); the set lives in the shared renderer, `tracker.js`.
- **Tabs** (the tab pane, (366)). Each offers only the modes that fit it:
  *Activity* has list / activity timeline; the others have one layout. A
  tab's count chip, mode picker, and buttons sit in a tool row under the tab
  bar.
  - *Notifications* ((331)-(334)): one feed of what concerns the viewer,
    newest first: new forced assignments to me, pending assignments to me,
    answer notices, reference notices (a follower accepted a pending
    assignment; a handoff of an assignment I gave was accepted), completion
    or `blocked` of work I assigned, Confirm requests waiting on me, replies
    to my comments, my live assignments due today or overdue, and mentions of
    me. Every entry is derived from the view state and only opens the item
    window, at what it refers to (390, see the item window below); the actions
    it refers to (Accept, Return, Confirm, ...) are in the item's context menu
    (its row in the Items panel, or the window header). Unread entries are
    highlighted; clicking one, or *Mark all read*, marks it read, which also
    clears the row's unread dot. Read state is local UI state per viewer
    clone, kept in memory like the handle badge's `seen`, and writes no
    event. In the product the daemon keeps it per clone on the local
    filesystem, outside git (340).
  - *Team*: the org tree (avatar, org prefix, display name), admins, clones
    per member, revoked keys, the priority levels, an admin roles-event
    dialog, "Edit profile" (on yourself; an admin on any member) and "Join a
    store..." ((393); the quick switcher offers it too).
  - *Activity*: the merged log in causal order, newest first, with
    "flagged only" and "concurrent only" toggle chips (498, the same look as
    the list / activity timeline chips, accent when on); or the activity
    timeline.
  - *Data*: the raw roles file, each ref's pushed commits (the remote), and
    the clone state (fetched counts, local commits, online), all editable.
    `apply` recomputes everything and `reset` restores the fixtures.
- **View modes** (303), scoped to the Items panel's current scope:
  - *Kanban*: columns by status. On an assignment scope the cards are that
    scope's assignment records (with a pending column; off *mine* they show
    the assignee's initials); on *all* the cards are items under the kind
    filter only: the columns already are the statuses, so the status filter
    is ignored there (it would only empty columns) and the kanban toolbar
    offers kind alone. Its first column is *review*, whose `+` creates a task
    in the review stage, not self-assigned. Cards are quiet like rows (title, initials, priority,
    due), with the source on hover. Dragging moves a card only for the
    viewer's own live assignment and dispatches `item.status` (todo / doing /
    blocked) or `item.close` (done). Cards stay independent tasks (469):
    hovering or focusing one outlines its direct parent's card (solid) and its
    direct children's cards (dashed), and when the parent's card is not on
    screen its tooltip adds "Parent: <title>". The calendar does the same.
  - *Activity timeline* (the Activity card): every event by write time,
    grouped by day, filterable by event family, member, flagged, and
    concurrent.
  - *Due timeline* (468): the scope's dated rows (an item on *all*, an
    assignment record on an assignment scope, showing its own state) as the
    same context tree the list draws, sharing its fold state. A row's bar runs
    from the item's start (first `doing`, else creation) to its due day, with
    a today line; overdue bars are marked. A node with children also draws a
    thin summary bar spanning the range of every dated item below it; folded,
    the children's rows go and the summary bar stays. A parent without a due
    date of its own is placed by its summary bar alone.
  - *Calendar*: a month grid of the scope's dated rows by due day, with month
    navigation per scope; hover and focus mark parent and children as on the
    kanban.
- **Support agent card** (chat placeholder, (300), (302)). A placeholder for
  the support-agent chat child, the future daemon-side local support agent:
  placement and a static sample only, with no working chat, no responder, and
  no new events. It is one tab of the tab pane, fixed height with
  internal scroll, newest message at the bottom, and a disabled composer row.
  The sample mimics the dashboard conversation view's entry shapes (user
  text, assistant text, a collapsed tool line) plus one new entry kind, the
  **tracker card reference**: a mini card of an item (title, status, priority,
  due, assignees) read from the view state, which opens that item's window
  on click. The referenced items are real ids from the current viewer's
  view state (top open work first, then what waits for an answer or confirm,
  then what the viewer assigned), so every reference resolves.
- **Item window** (366). Clicking an item (row click, Enter on a focused row,
  the second tap on touch, a notification, a chat reference, a subtask or
  parent link) opens it in a window over the page: a dimmed backdrop and a
  centred window; Esc, a backdrop click, or the close button closes it; focus
  moves into the window and returns to the row or entry that opened it; the
  page behind is inert. One window at a time: opening another item (a subtask
  link inside the window) replaces it. It replaces the thread side sheet and
  holds what the sheet and the row detail overlay held. **Layout** (424)-(427):
  a header over two columns. The header (full width, sticky) carries the
  parent breadcrumb (every ancestor, root first, each a link; none on a root
  item), the title with the status icon, the chips (kind, a small "⧉ <kind> ID"
  button that copies the item id, shown in its tooltip, and says "Copied" for a
  moment, status, priority, due, roll-up), and one quiet "⋯" next to the close button that
  opens the item's context menu (as a right-click on the header or the status
  icon does); no button row stands between the header and the body, which
  keeps a little margin above and below. Under it the **main column** holds the body with mentions and
  attachments, the **Subtasks** section, and the thread with the composer
  (a thread row names the person, org in small text, not the member or event
  id; the event alias is the row's tooltip); the
  narrow **side column** holds the **property card** and **Assignees**. Below
  820px of window width (the window, by a container query, not the page) it
  falls back to one column in the order header -> property card -> body ->
  Subtasks -> Assignees -> thread -> composer (the composer stays pinned at the
  window's bottom). Status, subtasks, and the thread are the prominent parts:
  their headings are larger, and the properties sit in a tinted, bordered card
  apart from the body.
  - *Property card* (424): status, kind, holders, path (the assignment path:
    the authors down the parent chain, when longer than one), due, creator, project, parent link, id, dates.
    Holders is one summary line, not the list: the count, then each live status
    other than todo with its count, then pending ("13 · doing 3 · done 1").
  - *Assignees* (425): at most 4 rows, then an "N more" toggle that expands the
    rest (local UI state, reset when another item opens). Order: lead-chain
    depth ascending (a root's follower is depth 1; the highest in the
    hierarchy first), then live before ended, then display name, then
    assignment order. What at least two rows share (the provenance text, for
    example "from Ada Park (forced: their lead chain)", and the close rule
    "assignee closes") is written once under the heading; a row shows the
    person (avatar, org, name), the state chip, and the note, plus only what
    differs from the shared text (for example "(inherited: everyone within L2,
    reached later)"), and the per-assignee actions (Confirm, Reject, Reopen,
    Unassign, *Hand off (admin)* on any member's live assignment; a note or
    handoff opens the menu step anchored to the button). A single row has
    nothing to share and shows its own text.
  - *Subtasks* (427), (470): the item's whole subtree, one row each: a ▸/▾
    fold control on a node with children, status icon, title (opens that item,
    so the window walks down; the breadcrumb walks up), the node's own roll-up
    when it has children, and holder avatars; a done one is greyed. The first
    level (the item's children) starts open, so their children show; deeper
    levels start folded (an "N subtasks" chip). The fold state lasts the page.
    Absent when the item has none (unless an "Add subtask" input row is open there).
  The thread is one chronological timeline of comments (quote
  reply, edit, hide), status changes, assignments and answers, edits, agent
  records with `agent` / `auto` chips, subtask lines, flagged and waiting
  events with their reason, amber "concurrent change" boxes, and quiet
  "superseded by" notes on a member's losing write from another clone. The
  composer (499), pinned at the window's bottom, posts a comment, as a reply when one is chosen, with an origin.
  It follows the dashboard chat input: one box with a paper-plane send button
  at its bottom right and a small `@` picker at its bottom left; Cmd+Enter
  (macOS) / Ctrl+Enter sends and Enter is a newline. The status moved to the
  context menu, so the composer has no status line. An own comment is edited
  in place (Cmd/Ctrl+Enter saves, Esc cancels). Mentions on the body and on each comment show as chips (335).
  Attachments show as chips with name and size and a simulated "not loaded /
  loaded" state: a click toggles it, as local UI state per viewer clone
  (338). An attachment on a flagged write shows struck through. The composer
  has a simple `@` picker that inserts `@id` and records the id, and a mock
  attach row (file name and byte count; the mock stands in a 64-hex oid
  because it has no content). An item or comment edit that changes the body
  keeps the attachments and the mentions whose `@id` is still in the text.
  The window opens through one function,
  `openItemWindow(storeId, itemId, {focus: eventId})` (also on `window`),
  which the quick switcher, the rows, and the notifications call; it
  switches to the store when needed and returns false for an unknown store or
  item. **Notification focus** (390): a notification carries the id of the
  event it is about, and clicking it opens the window scrolled to and marking
  that target, found by the event's type: a comment or mention -> that
  comment; a status, close, confirm, reject, or reopen -> that assignee's
  row in Assignees (the fold expands when the row is among the hidden ones); an
  assignment or handoff -> the holders line of the property card; an
  accept -> the accepter's row; a decline or return -> the assignees
  row that ended; a body edit (or a mention in the body) -> the body; an
  edit that only adds an attachment -> its chip. The target flashes briefly
  and keeps a thin left mark while the window stays open; with no target
  (a due notice, or an event the window does not show) the window just opens.
  A later URL route reuses the same `focus` option.
- **Actions** append events to the viewer clone's open batch, then report each
  event as applied or flagged. The actions are create (any kind, from the
  inline new-item row; a task or todo self-assigns in the same write), edit, comment, reply,
  edit or hide own comment (each with mentions, refs, and attachments), assign (people and subtree rules),
  split into a subtask, hand off, status, close, confirm, reject, unassign or
  withdraw, reopen, hide item, accept / decline / return, and every `roles.*`
  event. Actions are not pre-validated, so you can produce flagged events on
  purpose.

## Data model

### Roles file (genesis)

```json
{ "admins": ["ada", "bo"],
  "priority": { "names": ["Notice", "Task", "Urgent"], "default": 1 },
  "attachments": { "max_bytes": 10485760 },
  "members": [ { "id": "maya", "profile": { "org": "Search", "family": "Ortiz", "given": "Maya" },
                 "keys": ["maya-k1"], "leads": ["ada"] } ] }
```

`profile` ((391)) replaces the old `name` string: `org` is one free-text
string and may be empty (no org tree: team structure stays in `leads`),
`family` may be empty (a single-name person), `given` is required and
non-empty. A genesis member written with only a bare `name` (or nothing) reads
as a profile whose `given` is that name, else the id; a `profile` present
without a `given` is a roles-file problem.


`admins` lists member ids. A member with `leads: []` is a root, and several
roots are allowed. `leads` is a list validated to length at most 1. Member ids
cannot contain `.`, which separates member and clone in a stream id.
`priority` is the store's priority configuration: level codes are the integers
`0..n-1` (larger = more urgent), `names` are display names only, and `default`
is the level new items start at. Without it the store uses three levels named
Notice / Task / Urgent with default 1. `attachments.max_bytes` is the largest
attachment the store accepts, in bytes (339); without it the limit is 10 MiB
(10485760). Later changes are admin `roles.*` events, folded in causal order.

### Clone streams and commits

```json
{ "stream": "maya.pc", "ts": "2026-11-04T10:00:00Z", "signer": "maya-k1",
  "seen": { "ada.pc": 13, "lena.pc": 4, "maya.lap": 2, "omar.pc": 3 },
  "events": [ { "seq": 15, "ts": "2026-11-04T09:58:00Z", "type": "item.unassign", "payload": { "item": "maya.pc#1", "assignee": "omar" } },
              { "seq": 16, "type": "comment.add", "payload": { "item": "maya.pc#1", "body": "..." } } ] }
```

- Every clone of a store writes only its own ref,
  `refs/heads/tracker/m/<member>/<clone>`. Its **stream** id is
  `<member>.<clone>`; a member may have several clones (the fixture's maya has
  `pc` and `lap`). "Viewing as" picks a member, and the clone picker picks
  which clone the page reads and writes.
- `seq` is per stream, continuous across commits. Event id is
  `<member>.<clone>#<seq>`, shown as `maya.pc-15`. An item's id is the id of its
  `item.create`, and a comment's id is the id of its `comment.add`. There is no
  global sequence.
- `seen` maps each other stream (other members' and the member's own other
  clones) to the last seq this clone had received when the commit was opened.
  The stream's own earlier events are always seen.
- **Journal batching.** Writes go to the clone's local journal as an open
  batch. The batch closes into one signed commit when the clone receives new
  events (its `seen` would change) or when it pushes. All events in a commit
  share its `seen`, which stays exact because the batch closes before anything
  new is received. Each event keeps its own write `ts`; one without `ts` uses
  the commit's. Pushed commits are never rewritten.
- The commit is the signed unit, so the mock carries `signer` (a key id) on the
  commit. Events carry no signature.
- `ts` is informational: display, the order tie-break, and the display winner.
- An event has an optional `origin`: `human` (default), `agent`, or
  `agent-auto`.
- Fields starting with `_` are mock bookkeeping, not model: `_local` (not
  pushed), `_open` (the open batch), `_late` (reached this clone after events
  written later).

### Event types

| type | payload | rule |
|---|---|---|
| `item.create` | `kind` (`task`/`todo`/`topic`), `title`, `body?`, `project?`, `due?`, `priority?`, `parent?`, `after?`, `stage?`, `mentions?`, `attachments?`, `refs?` | A `parent` must be assigned to or created by the author. `project` is one free-text string, the only grouping (no labels or tags). `due` is a `YYYY-MM-DD` date. `priority` is a level in the author's current range; omitted, the item starts at the store's default level. `after` places it among its siblings (528), below. `stage: "review"` creates a task or todo in the review stage (see Derived state); absent, it starts in todo. Flagged: a stage on a topic, or any other value. |
| `item.edit` | `item`, any of `title`/`body`/`project`/`due`/`priority`/`stage`; `mentions?`, `attachments?` and `refs?` with `body` | Creator only; an admin may also edit an item whose creator was removed (292). `due: null` clears the date; priority cannot be cleared. `stage: "review"` moves a task or todo back to review, `stage: null` moves it to todo; moving to review is flagged while the item holds a live or pending assignment, and a stage on a topic is flagged. Each field merges by the (203) rule. `mentions` or `attachments` without `body` is flagged, and so is `refs`. |
| `item.hide` | `item` | Creator only; an admin may also hide an item whose creator was removed (292). A tombstone: the content stays in history. Flagged while someone else holds an open (live or pending) assignment on the item. |
| `item.assign` | `item`, `targets` (a list of member ids and rules `{root, include_root?, levels?}`) or the older `mode` (`all`/`chosen`), `levels` (all), `people` (chosen); `completion` (`assignee`/`assigner`; absent reads as `assignee`, and writers fill it in: `assigner` when anyone but the author is a target, `assignee` when the author is the only one) | Creator only; an admin may also assign an item whose creator was removed. A person may be anyone in the store: the author, or someone the author reaches (the author is in their lead chain, any depth), is a **forced** assignment, live at once; anyone else gets a **pending** assignment. A rule (490)-(492) targets everyone below `root` (within `levels`, absent = the whole subtree; `root` too with `include_root`), forced, and stays live: anyone who later enters the range inherits it. A rule rooted at the author or below is live at once; one rooted elsewhere is a **rule request** pending to `root`. The older `chosen` is `targets` of people and `all` is one rule `{root: author, levels}`, with the event id as the rule id. Flagged: both `targets` and `mode`, an empty list, a malformed target, two rules on one root, a rule with nobody in it, a rule another live rule of the author's on the item already covers, or every person chosen (with no rule) already holding a live or pending assignment. Not allowed on topics. Any applied assign moves a review-stage item to todo. |
| `item.handoff` | `item`, `to`, `assignee?`, `note?` | The holder of a live assignment hands it to anyone; an admin may hand off any member's live assignment by naming its holder in `assignee` (286), so open work can be taken over before the member is removed. It makes a pending assignment to `to` with the original's assigner and completion mode; on accept the original assignment ends (`handed`) and the new one starts with the original's status and note (290). Flagged on a done assignment, when `to` is the holder or already holds one, or while another handoff of it is pending. |
| `item.accept` / `item.decline` | `item`, `note?` | The target of a pending assignment, on it. Accept makes it live; for a handoff it also ends the original. Decline ends it as `declined`. A pending handoff whose original ends another way is withdrawn with it (295), so a later accept or decline finds no pending assignment. |
| `item.return` | `item`, `assignee`, `note?` | Only the assignee's direct lead, on an accepted (formerly pending) live assignment. Ends it as `returned`; the assigner may send again. Returning an accepted handoff restores the original assignment with its prior status (291), unless its holder was removed or holds another assignment on the item by then. |
| `item.unassign` | `item`, `assignee?` or `root?` | By the assigner; the sender of a pending assignment may also withdraw it. An admin may also unassign when the assignee or assigner was removed. With `root`, it ends the author's open rule rooted there (a pending rule request is withdrawn). Without either, it withdraws all of the author's assignments on the item and ends their rules. |
| `item.status` | `item`, `status` (`todo`/`doing`/`blocked`), `note?` | Assignee only (live). The creator self-assigns first. Allowed while awaiting confirmation, which withdraws the close. Not allowed on done. |
| `item.close` | `item`, `note?` | Assignee. Moves to `done`, or to `awaiting` when completion is `assigner`. |
| `item.confirm` / `item.reject` | `item`, `assignee`, `note?` | Assigner, on an `awaiting` assignment. Moves to `done` / back to `doing`. |
| `item.reopen` | `item`, `assignee?`, `note?` | The assigner or the assignee reopens a `done` assignment, back to `doing`. |
| `comment.add` | `item`, `body`, `reply_to?`, `mentions?`, `attachments?`, `refs?` | Anyone in the store. `reply_to` is one comment in the same thread (flat timeline, quote reply). |
| `comment.edit` / `comment.hide` | `comment`, `body`, `mentions?`, `attachments?`, `refs?` (edit) | Comment author only. Hide is a tombstone. |
| `member.profile` | `profile` (any of `org`, `family`, `given`: strings), `member?` | (392 b) The author's own profile, or with `member` another member's: only an admin may write that, anyone else's write is flagged. Each field present is its own register, so concurrent changes merge by (203), and a write only carries the fields it changes. `given` is required and non-empty whenever it is present; a profile with an unknown or non-string field, or with none of the three, is flagged. The member must exist and not be removed. Shows in Activity with the writer, the admin when an admin made it. |
| `roles.add_member` | `member` (`id`, `profile` (`org?`, `family?`, `given`), `keys`, `leads`) | Admin. Stands in for the self-join (`member.join`, (231)), whose payload carries the same `profile`. Flagged without `profile.given` (391). |
| `roles.set_lead` | `member`, `leads` | Admin. Length at most 1, no cycle. |
| `roles.add_admin` / `roles.remove_admin` | `member` | Admin. The last admin cannot be removed. |
| `roles.add_key` / `roles.revoke_key` | `member`, `key` | Admin. Revocation rule below. |
| `roles.remove_member` | `member` | Admin. The member must lead nobody and must not be an admin. Their earlier events stay valid. Their live assignments end as `unassigned` and their pending ones as `withdrawn` (287), so nothing stays counted under a removed member; like an unassign, this beats a concurrent status write. |
| `roles.set_priority_levels` | `names` (1-9 display names, lowest first), `default` | Admin. Changes the level count and names; a field write, so concurrent changes merge by (203). |
| `roles.set_attachment_limit` | `max_bytes` (a positive integer) | Admin. Changes `attachments.max_bytes`; a field write, merged by (203). Attachments already written stay; a later write is checked against the new limit in its author's causal past, but only for the attachments it adds. |

**Mentions, attachments, and refs** ((335), (338), (531)). A body (an item's on
`item.create` / `item.edit`, a comment's on `comment.add` / `comment.edit`)
may carry `mentions`, a list of member ids, `attachments`, a list of
`{oid, name, size, mime}`, and `refs`, a list of item ids. The text keeps `@name` for display; the list is
what notifies. Each id must be a member, not removed, in the author's causal
past. `oid` is the sha256 of the content (64 lowercase hex digits), `size` is
a byte count, and `name` and `mime` are required. A write may add only
attachments no larger than `attachments.max_bytes` in the author's causal
past (345 b): an attachment whose `oid` is already on the same item body's or
the same comment's shown attachments, as the author had seen them, is exempt,
so an edit that keeps an older, larger attachment survives a lowered limit,
while an oid the author had not yet seen there counts as new.
`item.create` and `comment.add` check every attachment. The text keeps
`#<alias>` for display; each `refs` id must be an item in the author's causal
past (a hidden one included). The writer derives `refs` from the text, so an
edit keeps a ref only while its `#<alias>` (or `#<id>`) stays in the text as
a whole token. All three lists ride with the body: a
body write replaces them (absent means empty), and the shown lists are the
winning body write's, so concurrent edits follow the (203) / (271) winner.
Only metadata is modelled; storing and fetching the content is a later
ledger ticket (337). Agent-authored writes may mention.

**Sibling order** (528). `after` on `item.create` names the sibling the new
item goes right after: absent appends it after the siblings placed so far
(every item created before the field existed, so old stores keep their
creation order), `""` or `null` puts it first. The view places items in fold
order, each under its parent (or at the root); an anchor that is missing, not
a sibling, or not placed yet appends. A hidden anchor keeps its slot, so
hiding a row moves no other row. Two inserts after one anchor that do not see
each other order by the fold: the later one lands nearer the anchor. A
non-string `after` is flagged; nothing else is. Reorder and reparent are not
modelled; a later `item.move {item, parent?, after}` can carry the same
anchor, its latest write winning like a field.

`agent-auto` is valid only on `item.status` and `item.close` for the author's
own assignment. It is always invalid on accept, decline, return, and handoff:
an agent never accepts or declines an assignment on its own.

### Assignments

An assignment record is `<assign event id>><member>` (for a handoff,
`<handoff event id>><member>`), with an assigner (`by`), a sender (who wrote
it: the assigner, or the handing-off assignee), a completion mode, and a state:

```text
pending --accept--> live --unassign / member removed--> unassigned
   |                  |--return-----> returned   (accepted ones only, by the assignee's direct lead)
   |--decline--> declined               |--handoff accepted--> handed --that handoff returned--> live (restored)
   |--withdraw / member removed / handoff's original ended--> withdrawn
forced (author, or someone the author reaches) starts live
```

- A **request** is no longer a separate kind or field: it is an item created
  with a pending assignment in the same write. The Items panel's *received
  requests* scope lists pending assignments to me; the *all* scope's kind
  filter `request` lists items with a pending assignment.
- **Pending** assignments are excluded from item status and roll-ups. While
  pending, an assignment shows in the target's *received requests* scope and
  handle badge, and as "unaccepted" in their lead's *my team* scope.
- **Accept** keeps the assigner and completion mode, so "assigner confirms"
  works for requests too (214). The accepter's direct lead at that moment gets
  a reference notice (Notifications card and handle badge) and may return it
  from the row in their *my team* scope.
- **Handoff.** On accept the original assignee's assignment ends as `handed`
  and the original assigner gets a notice like the lead. The handed-on
  assignment keeps the original assigner, so confirm and unassign stay with
  them, and starts with the original's status and note (290). An admin may
  hand off any member's live assignment (286): the takeover path before a
  removal. If the accepter's lead returns an accepted handoff, the original
  assignment comes back live with its prior status (291).
- **Answer notices** (285). The sender of a pending assignment gets a notice
  when it is declined or, once accepted, returned; for a returned handoff the
  original assigner gets one too. Notices are derived from the assignment
  records, never events.
- **Done and blocked notices.** The assigner of a live assignment gets a
  notice (`done` / `blocked`, key `w:<assignment>:<status>:<lastUpdate>`)
  when someone else set it done or blocked; an awaiting close is in the
  confirm list instead, and a status the assigner set (a confirm) is none.
- `returned` and `declined` are terminal: the assigner may assign again.
- Completion mode (assignee closes / assigner confirms) stays per assignment.
- **Rules** ((490)-(492)). A rule's id is `<assign event id>~<root>` (the
  older `all` form: the assign event id). Each member it covers gets a
  forced, live record `<rule id>><member>` carrying `rule`; a member who
  later enters the range inherits one (`inherited`). A rule rooted outside
  the author's reach first sends its root a **rule request**, a record with
  the rule's id and `ruleRequest: true`, pending to the root; nobody else
  holds a record yet. The root's accept makes the rule live (its records
  appear) and notifies the root's direct lead, who may return the request:
  that ends the rule and its records as `unassigned`. A decline ends it, and
  `item.unassign` with `root` (or without a target) withdraws it while
  pending. Accept and decline answer every pending record the author holds
  on the item. A request is neither live work nor counted in the item status
  while pending.

```text
rule rooted at the author or below:  live at once --unassign root / all--> ended
rule rooted elsewhere:  request pending --accept--> live --request returned / unassign--> ended
                                        |--decline / withdraw--> ended
```

### Derived state

- An assignment's status is `todo`, `doing`, `blocked`, `awaiting`, or `done`
  while live. An item's own status comes from its live assignments: all done
  means `done`, any blocked means `blocked`, then awaiting, then doing, then
  todo. With no live assignment it is `pending` (some pending), `review` (the
  item is in the review stage), `declined` or `returned` (the latest one
  ended that way), or `unassigned`. Topics have none.
- **Review stage.** A task or todo carries `stage` (`review`, or none for
  todo), set on create or by `item.edit`. An item holding a live or pending
  assignment is never in review: an assign clears the stage, moving to review
  is refused while one is held, and a move to review that folds after a
  concurrent assign is dropped (the assignment wins), in both engines.
- Roll-up: a parent shows its children by status ("done 3/5 · blocked 1") and a
  "blocked below" warning when any descendant is blocked, all the way up to the
  root. Topics and items whose status is pending, review, declined, or
  returned are not counted. The parent's own status stays manual, set by its assignee.
- Priority displays as the level name; a stored value outside the current range
  displays as the top level. Stale (284) = a live assignment in `doing` or
  `blocked` with no status write for 3 days by the viewer's clock (the mock
  clock here). The threshold is a client-side constant (`STALE_DAYS`, or
  `deriveViewState(input, {staleDays})`), not shared data; todo work never
  goes stale. Overdue = due before
  today and not done.

## View state (the future backend API shape)

`store.view(stream, now)` returns one plain JSON object, built by
`deriveViewState(input, { stream, now, staleDays? })` plus the store's sync info. The view
reads nothing else, and writes only through `store.dispatch(stream, events,
ts)`, which returns each new event's id and verdict. This object is the shape
the Go data-structure child should serve.

```text
viewer       { member, stream, name, profile{org, family, given}, admin, lead, followers[], reach{id: depth}, isMember }
priority     { names[], default }
attachments  { max_bytes }
members      { id: { id, name, profile{org, family, given}, avatar, leads[], keys[], revoked[{key, by}], removed, joined, admin, children[], depth, role } }
admins[], rolesProblems[]
items        { id: { id, alias, kind, title, body, project, due, priority, priorityLevel, priorityName,
                     parent, children[], author, ts, hidden, edited{id, ts}, status, counted,
                     rollup{total, counts{status: n}, blocked[]}, lastActivity, started,
                     assignments[], pending[], thread[eventId], concurrent, depth, chain[],
                     mentions[memberId], attachments[{oid, name, size, mime}],
                     refs[itemId], backrefs[itemId|commentId] } }
itemOrder[]  item ids in tree order, siblings by their after anchors
assignments  { id: { id, item, to, by, sender, completion, state, forced, status, note, noteBy, via, ts,
                     inherited, ruleLevels, rule, ruleRequest, handoffOf, handoffFrom, handoffNote, answerNote,
                     restored{id, ts}, restores, accepted{id, ts},
                     leadAtAccept, ended{id, type, ts, by}, depth, statusOrigin, statusBy,
                     lastUpdate, stale } }
rules        { id: { id, item, via, by, root, includeRoot, levels (null: whole subtree), completion,
                     state (live|pending|ended), request, ended, ts } }
comments     { id: { id, item, author, body, reply_to, ts, edited{id, ts}, hidden,
                     mentions[memberId], attachments[{oid, name, size, mime}], refs[itemId] } }
events[]     display order: { id, alias, stream, member, seq, ts, type, origin, payload, status
                     (applied|flagged|waiting), reason, ignored, markers[{kind, field, other,
                     otherMember, winner, mine, theirs}], superseded[{field, by, why (later|human)}],
                     thread, parentThread, local, open, late }
notices[]    { key, kind (accepted|handoff|done|blocked|declined|returned), assignment, item, to, by, sender, ts,
               state, from?, endedBy?, note?, restores? }   keys acc:/ho:/dec:/ret: + assignment id
my           { assignments[], today{work[], pending[], confirms[]}, received{pending[], answered[], sent[]},
               given[itemId], created[itemId], team[{id, depth, open[], pending[], waiting[], done, blocked,
               stale, last}],
               badge[key] }
stats        { events, commits, flagged, concurrent }
heads        { stream: seq }   the seen a write would carry now
store        { name, label, remote }
sync         { online, syncedAt, upstream, unpushed[{open, ts, events[]}] }   the viewer's clone
clones       { member: [{ stream, online, unpushed }] }
```

`backrefs` lists the shown items, then the shown comments on shown items,
that reference the item, each once, in fold order; the item's own thread is
left out. A view reads the referencing item of a comment through
`comments[id].item`.

`name` is derived from the profile, never stored: kept in the view state so a
view reads one string where it names a person. **Display rules** ((383 b),
(391)): when `family` or `given` holds Hangul, a CJK ideograph, or kana, the
name is `family` + `given` with no space ("홍길동"); otherwise `given family`
("Ada Park"); no `family` means the given name alone. `org` is not merged into
the name: a view shows it as a muted prefix or in the tooltip where a person is
named. `avatar` is the text in the avatar circle: for a CJK name, `given` up
to two characters ("길동", and "민" for 남궁민); otherwise the first letter of
`given` and of `family`, upper-cased ("AP"); a single name, its first two
letters, upper-cased ("SV"). `engine.js` (`displayName`, `avatar`) and the Go
port (`Profile.DisplayName`, `Profile.Avatar`) hold the same rules, pinned by
the golden parity.

Lists hold ids into the maps. Everything is per viewer clone: two clones of
one member can show different states until they sync. The notification card
reads only this object; its read state, like the badge's `seen`, is client
state outside it.

## Validation and concurrency

1. **Causal past.** An event's causal past is its stream's earlier events plus
   every event its commit's `seen` names, closed transitively. If `seen` names
   an event this clone has not received, the event is **waiting**: shown, not
   applied, and with no verdict yet.
2. **Order.** Events are put in one order that respects the causal past. Ties go
   by `ts`, then member id, then clone id, then seq. This is only for folding
   and display.
3. **Verdict.** Each event is checked against the state folded from its causal
   past only. That is the state its author had seen, never later arrivals. So a
   late arrival never changes the verdict of an event already shown.
4. **Final fold.** It applies every valid event leniently and marks concurrent
   changes. Two events are concurrent when neither is in the other's causal
   past.
   - **Field writes.** These are assignment status (written by the assignee
     with status / close and by the assigner with confirm / reject / reopen),
     a member's lead, the priority levels, the attachment limit, each of a
     member's profile fields (`member.profile`: org, family, given), and item or
     comment text, due, and priority. Each field is a multi-value register; concurrent writes are all
     kept. The **display winner** is the later `ts`, then the greater member
     id, clone id, and seq. Writes by two different members get a
     "concurrent change" marker naming both values. Writes by one member from
     two clones (item and comment text have a single writer, so this is their
     only concurrency) get no marker: the later `ts` wins (271), except that a
     `human`-origin write beats a concurrent `agent` / `agent-auto` write of
     the same member (288). Among a field's concurrent writes, an agent write
     with a same-member human write beside it drops out first; the later `ts`
     rule picks among the rest. The losing same-member write stays in the
     thread with a quiet "superseded by <event>" note (289), not a conflict
     box.
   - **Removal against writes.** An unassign, return, withdraw, or accepted
     handoff that is concurrent with a status write or close on that
     assignment always wins. Both events are marked, and the write shows as
     valid when written but not in effect. The same applies to a subtask
     created under an assignment that was concurrently removed.
5. **Revocation.** `roles.revoke_key` cuts each of that member's streams at what
   the revoker had seen of it. Events signed with the revoked key with a seq
   above the cutoff are ignored, even when their `seen` predates the
   revocation, because a stolen key cannot pretend to be old. Events at or
   below the cutoff stay valid. This is the one rule that can turn an already
   shown event into ignored (210).

Flagged reasons, as the engine words them:

- the commit's stream does not match the ref it sits in
- a seq gap or duplicate (a broken event consumes no seq)
- unsigned, or a signer key that is not the author's in its causal past
- a key already revoked in its own causal past, or revoked later and above the
  revoker's cutoff
- author not a member, or removed
- `agent-auto` outside status or close, or on accept, decline, return, or
  handoff
- edits or hides by anyone other than the creator or comment author
- a due that is not a date, or a priority outside the author's current range
- assignment by a non-creator, on a topic, empty, already covered, or with
  every chosen person already assigned
- a handoff of a done or someone else's assignment, to a holder, or while one
  is pending
- accept or decline without a pending assignment (a handoff whose original
  ended is withdrawn, so it is no longer pending)
- return by anyone but the assignee's direct lead, or of a forced assignment
- status, close, or reopen by a non-assignee, before acceptance, or after the
  assignment ended
- status or close on a done assignment, or close while already awaiting
- confirm or reject by a non-assigner, when not awaiting, or when no
  confirmation is required
- a reply to a comment that is unseen, hidden, or in another thread
- a mention that is not a list of ids, or names someone who is not a member
  or was removed; mentions or attachments on an item edit without a body
- an attachment list that is malformed, an oid that is not a sha256, a
  missing name or mime type, a size that is not a byte count, or a newly
  added attachment above `attachments.max_bytes`
- `member.profile` or a join with a profile that is not an object of strings
  among org / family / given, an empty `given`, no field at all, or a join
  without `given`; another member's profile by a non-admin, or of an unknown
  or removed member
- `roles.*` by a non-admin, a lead list over length 1, a cycle, an unknown
  member, a key in use, removing the last admin, invalid priority levels, or
  an attachment limit that is not a positive integer

## Fixtures (`team-alpha`)

Fourteen members in three levels. Ada is the team lead (admin). The part leads
are Maya (lena, omar, ivy, jon, kai), Raj (sven, tia, noa, and later pia), and
Bo (admin; uma). Default viewer is `maya.pc`; maya also has a `lap` clone. The
page clock starts at 2026-11-06 09:00. The fixtures show:

- profiles: every member has an org (Platform, Search, Infra, or SW개발팀);
  jon (홍길동), kai (김민수) and uma (남궁민: a two-character family name and a
  one-character given name) have Korean names under SW개발팀, sven has a
  single name, and the rest are Latin, so both display rules and the three
  avatar rules show (ids are unchanged). `ada.pc-15` (ada moves tia's org)
  and `tia.pc-6` (tia changes it herself) are concurrent writes to one
  profile field, a (203) conflict.
- threads with comments and quote replies; due dates and priorities on most
  tasks, with overdue, stale (doing or blocked for 3+ days) and blocked work
- an edit of `ada.pc-1`, and a flagged edit of it by maya (creator only)
- a hidden comment (`raj.pc-4`), plus a flagged hide by someone else
- reject (`maya.pc-12`), reopen (`maya.pc-18`), and unassign (`maya.pc-15`)
- pia's blocked "Cache layer" raising "blocked below" on `raj.pc-1` and
  `ada.pc-1`
- hierarchy of mixed depth (427): `ada.pc-1` > `maya.pc-1` > `maya.pc-5` /
  `maya.pc-7` (depth 3), `raj.pc-1` > `raj.pc-7`; and, appended after the
  other streams' events, `raj.pc-11` "Infra capacity plan" (assigned to tia,
  doing) with three subtasks (`raj.pc-13` doing, `raj.pc-15` todo with its own
  blocked subtask `sven.pc-6`, `raj.pc-17` done) and `ada.pc-16` "Refresh
  onboarding docs" (uma, doing) with one child (`ada.pc-18`, bo); childless
  tasks (`raj.pc-20` "Renew TLS certificates", "Security training", "Benchmark
  harness" and others) sit among them. None of it touches maya's chips or
  notifications.
- pia, added later, inheriting ada's live "Security training, everyone within
  L2"
- jon-k1 revoked by `bo.pc-6` after seeing jon up to seq 5. `jon.pc-6`, signed
  with jon-k1 and backdated with an old `seen`, is ignored. jon continues on
  jon-k2.
- omar long offline. His three late commits are pushed but fetched by no other
  clone; they arrive on that clone's next fetch (opening the drawer). They
  create a field conflict with maya's reject on "Shard writer docs" (omar's
  later `blocked` is the display winner) and a status report on the assignment
  maya concurrently unassigned (`omar.pc-5` vs `maya.pc-15`).
- maya edits the body of "Index builder" from `maya.lap` and, without having
  fetched it, from `maya.pc` later: the later edit is shown, both stay in the
  thread, no marker; `maya.lap-1` carries the "superseded" note (271, 289).
- maya sets "Ship search v2" to doing by hand on `maya.pc`; an hour later her
  agent on `maya.lap`, not having fetched it, auto-sets blocked: the human
  write wins despite the earlier ts, and `maya.lap-4` is superseded (288).
- assignments of every kind:
  - forced: maya's subtasks to her followers, ada's live "everyone within L1/L2"
  - pending, accepted, assigner confirms: `kai.pc-2` sent up to ada; ada
    accepts and closes, kai confirms (214)
  - pending, accepted, then returned: `sven.pc-3` to jon; jon accepts (maya
    gets a notice), maya returns it after a flagged return by bo
  - pending, declined: `tia.pc-3` to uma, with a comment
  - pending, open: `noa.pc-3` to lena, plus a flagged `agent-auto` accept; and
    `ivy.pc-6` to her own lead maya (on maya's Today chips: *answer*)
  - handoff: `lena.pc-2` to raj (pending, raj accepts, ada gets a notice), raj
    hands it to sven, sven accepts (lena, the assigner, gets a notice)
  - awaiting confirm: ivy's close of "Tokenizer" waits for maya
- mentions and attachments: lena's comment `lena.pc-6` on "Index builder"
  mentions maya and omar and attaches `index-layout-v2.png`; her next comment
  `lena.pc-7` attaches a 50 MiB trace and is flagged over the 10 MiB limit;
  bo's topic `bo.pc-9` mentions maya and raj in its body and attaches a PDF;
  `uma.pc-6` is flagged for mentioning `zed`, who is not a member; maya's
  reply `maya.pc-22` to `lena.pc-6` mentions lena and ivy. Together with the
  rest, the notification card shows every entry kind: maya's card alone has
  all but the answer notice, which sven's and tia's cards show.
- about a dozen flagged demos, one for most reasons above

`hana-personal` has one member and one clone with no remote. Its commits are
local (unpushed); todos are self-assigned in the same commit as their
creation, and it has a topic and a blocked subtask.

## Engine notes for the real implementation

- **Layers.** `openStore` mocks the protocol layer (per-clone refs, journal,
  push, fetch) that `261107-feat-ws-tracker-ledger-protocol` implements;
  `replay` and `deriveViewState` are the data-structure layer the Go child
  ports. The verdict, fold, and view-state rules are deterministic given the
  same commits and `now`.
- **Incremental apply [181].** Keep a cursor per stream and apply new events
  as they arrive. A late event re-replays only its thread. A roles change
  re-replays everything, because reach and membership feed every check. The
  mockup simply replays everything on each render, in O(n²) over the causal
  past, which is fine at fixture scale.
- **Ref layout [183].** Each clone's ref stores events under
  `events/YYYY/MM/DD.jsonl`.
- **No snapshots [184]** in v1.
- **Local clone [185].** A bare repo under `~/.ws/tracker/<store>/`, which is
  user data, not cache. The index is cache.

## Settled

| board | decision |
|---|---|
| 149 | The roles file is genesis. Later changes are admin `roles.*` events. |
| 150, 151 | Admins are a list of member ids. Several roots are allowed. |
| 157 | `roles.add_admin`, `remove_admin`, `revoke_key`, `remove_member`. Events signed before a revocation stay valid. |
| 147 | An event's author owns the ref it sits in. Admin takeover of a departed member's work is an event in the admin's own ref. |
| 148 | Everyone in a store sees everything. "Viewing as" changes only the perspective. |
| 154, 156 | One commit per write, several events. The commit is the signed unit, with no per-event `sig`. |
| 153, 171 | Event id `author#seq`, no global sequence, shown as `maya-12`. |
| 168 | Every commit records `seen`. It replaces timestamp merge order. |
| 167 | Validate against the author's seen state. Verdicts never flip on late arrival. Concurrent conflicts are shown with a deterministic display winner. |
| 168 | A revoked-key event that arrives after the revocation is ignored whatever its `seen` says. |
| 169 | No remote and long offline are the same path. The drawer shows last synced and unpushed writes. Sync failure is retried. |
| 170 | Deletion is a hide (tombstone) event. |
| 181-185 | Engine notes above. |
| 166 | Thread-centric items: one timeline, subtasks as child threads, a composer with quick status. |
| 175, 162 | Kinds `task` / `todo` / `topic`. One `item.create` plus `item.edit` by the creator. |
| 176, 177 | `comment.add`. The author edits or hides. Flat timeline with an optional `reply_to`. |
| 146 | Project reference is one optional free-text string. |
| 158 | "Everyone within N" stays live. |
| 159, 144 | Delegating down creates a subtask under one's own name. Only the creator assigns (an assignee may hand off, 273). A subtask's parent is assigned to or created by its author. |
| 145 | Completion mode per assignment event, default assignee closes. |
| 160 | `item.unassign`, `item.reopen`, `item.reject`. |
| 161 | Progress is a status `todo`/`doing`/`blocked`/`done` plus a note. No percentage. |
| 178 | The roll-up counts children by status and warns on blocked up to the root. The parent's own status stays manual. |
| 163 | Self-assignment for one's own work. "My assignments" gathers everything. |
| 155 | One `origin`: `human` / `agent` / `agent-auto`. |
| 186-189 | Peer requests to anyone. Pending requests are not counted. Accepting needs no approval, but the recipient's lead may return. An agent never accepts or declines. Requests became pending assignments by 279, 280. |
| 201 | A member whose key was revoked continues under the same id after `roles.add_key`. |
| 202 | Changing status while awaiting the assigner's confirm cancels the completion request. |
| 203 | Concurrent writes to one field: the display winner is the later ts, then member id, then seq. Both values are kept and shown. |
| 204 | An event whose `seen` names an unreceived event is waiting: no verdict until the gap fills. |
| 205 | A broken event (seq gap or duplicate, wrong ref, bad signature) consumes no seq. |
| 206 | `item.edit` also edits the project field. |
| 207 | Hiding an item that still holds someone else's active assignment is flagged. |
| 208 | An admin may act on a removed member's work. |
| 209 | A member can be removed only when they lead nobody and are not an admin. |
| 210 (Q1) | Revocation cuts at what the revoker had seen: the one sanctioned verdict flip, the same on every client. |
| 211 (Q2) | A victim's own events cut by a revocation are re-issued with the new key; the originals stay ignored. |
| 212, 230, 237 (Q3) | One ref per clone, `refs/heads/tracker/m/<member>/<clone>`; event id `<member>.<clone>#<seq>`; `seen` per clone stream. No one-device rule. Supersedes 153/171's `author#seq`. |
| 213 (Q4) | Unassign and return beat a concurrent status write or close. |
| 214 (Q5) | A requester may choose "requester confirms" when sending, as with assignment. |
| 215 (Q6) | A returned request stays returned; the requester sends again if needed. |
| 227 | Writes batch in a local journal and close into one signed commit when `seen` would change or at push. Supersedes 154's one commit per write. |
| 229 | Registering an empty repository creates the store; the registrant is sole admin. |
| 231, 232 | Self-join by an own-ref `member.join` event, first come per id; trust is repository access. Replaces the `roles.add_member` stand-in. |
| 233 | A person's PCs share one copied key file. |
| 234 | A deleted or rewound remote ref is refused and kept locally, with a manual restore push. |
| 235, 236 | Protocol layer first (`261107-feat-ws-tracker-ledger-protocol`): identity is protocol, leads and assignment reach are data structure, payloads are opaque to the protocol. |
| 241 | Own writes push after a 5-minute idle delay, at an explicit apply, or when the panel collapses. |
| 247 | Genesis is the single commit of `refs/heads/tracker/genesis`, never updated. |
| 268 | v3 models the Settled rows v2 still showed as v2: per-clone refs and streams (212, 230, 237) with a clone picker, "assigner confirms" for requests (214), and journal batching with an unpushed list (227). |
| 271 | One member's concurrent writes from two clones: the later ts wins by the (203) rule, the losing write stays in history, and no conflict marker is shown. |
| 272 c, 277 c | Items carry an optional `due` date and one integer `priority`, edited by the creator and merged by (203). Priority levels are store configuration in the roles file (`priority.names`, `priority.default`), changed by `roles.set_priority_levels`; default Notice / Task / Urgent, default level 1. Larger is more urgent; a stored value outside the range shows as the top level. |
| 274 a | Only the creator edits an item, due and priority included; others comment. |
| 278 a | No labels or tags in v1; the `project` string stays the only grouping. |
| 273 b, 279 a, 280 a | Assignment is unified. `item.assign` may target anyone: someone the author reaches (lead chain) is forced and live; anyone else is pending until accept or decline. Pending assignments are excluded from status and roll-ups. On accept the accepter's direct lead gets a reference notice and may return it (`item.return`). A request is an item created with a pending assignment in the same write; `request.*` became `item.accept` / `item.decline` / `item.return`. An assignee hands off with `item.handoff` (pending to the new person; on accept the original ends and its assigner is notified). `all` stays forced-only. Completion mode stays per assignment. An agent never accepts or declines. |
| 275 | The view reads only one derived view-state object and writes only by dispatching events; its shape is in [View state](#view-state-the-future-backend-api-shape) as the future backend API. |
| 276 b | View modes: kanban (drag moves only one's own assignment), activity timeline, due timeline, and calendar, next to the cards and the tree. Styling is deferred. |
| 267 a, 281 a | A scenario walk through the page and engine rules, recorded in [SCENARIOS.md](SCENARIOS.md) with view and model snags. |
| 284 | Stale = a live assignment in doing or blocked with no status write for 3 days; the threshold is a client-side constant. |
| 285 | The sender gets a derived notice when their pending assignment is declined or their accepted one returned; no new event. |
| 286 | An admin may `item.handoff` any member's live assignment (pending until the new holder accepts), before removal. |
| 287 | `roles.remove_member` ends the member's live assignments as unassigned and pending ones as withdrawn. |
| 288 | Same member, concurrent writes from different clones with different origins: the human write wins over agent / agent-auto; same origin keeps (271). |
| 289 | The losing same-member write gets a quiet "superseded by <event>" note in history; no conflict box. |
| 290 | A handoff's new assignment starts with the original's status and note. |
| 291 | Returning an accepted handoff restores the original assignment with its prior status and notifies the original assigner. |
| 292 | Extends (208): an admin may `item.edit` and `item.hide` an item whose creator was removed. |
| 293 | Accepted as interpreted in v3: forced reach is any depth of the lead chain (`levels` dropped from `chosen`); (271) holds for every field; a kanban drop on done dispatches `item.close`; stale counts doing and blocked only; a viewer switch pushes the previous clone and fetches the new one; the push delay and join are not modeled. |
| 303 | View modes move from the drawer title to each card: the drawer has one layout (the cards layout) and each card offers only the modes that fit it (the table under [What the page shows](#what-the-page-shows)). Supersedes the drawer-level picker of (276) b while keeping its four modes, now scoped to a card's own rows and filters. The kanban drag rule is unchanged. |
| 304 a | A card in a wide mode (kanban, due timeline, calendar, activity timeline) spans the full row of the cards layout; the remaining cards flow below it. No horizontal scroll in the card and no maximize button for now. |
| 300 a, 302 | A chat card placeholder for the future daemon-side local support agent: placement and a static sample only (fixed height, internal scroll, newest at the bottom, a disabled composer), with no working chat, no fake responder, and no new events. |
| 305 | The chat sample mimics the dashboard conversation entries plus a tracker card reference entry that renders an item from the view state and opens its thread sheet. Each card's chosen mode is local UI state remembered per store, never an event. |
| 331 c, 332 a, 333 a, 334 | A *Notifications* card in the cards layout, list only, newest first, derived from the view state: new forced and pending assignments to me, answer and reference notices, completion or blocked of work I assigned, Confirm requests waiting on me, replies to my comments, my live assignments due today or overdue, and mentions of me. An entry opens the item's thread sheet; *Received requests* and *My team* keep their buttons. Unread entries are highlighted and cleared by a click or *Mark all read*; no event is written. The handle badge is unchanged. |
| 331 c, 335 | Item bodies and comments carry an optional explicit `mentions` list of member ids, validated against the author's causal past; the text keeps `@name`. Mentions show in the mentioned member's notification card and as chips in the thread; the composer has a simple `@` picker. Agent-authored writes may mention. |
| 336 | Mentions, attachments, and the attachment limit are ported to the Go domain layer (`agents-plugin-tool/internal/wstracker`) in the same change, with golden parity; the ledger layer is untouched. |
| 337 a | Attachment content is stored as content-addressed blobs under `attachments/<sha256>` in the member's ledger tree and fetched on demand through a partial clone. The storage itself is deferred to a later ledger ticket; v3 models metadata only. |
| 338 a, 339 a | Item bodies and comments carry `attachments: [{oid, name, size, mime}]` with `oid` the content's sha256. The roles file's `attachments.max_bytes` (default 10 MiB) is changed by the admin `roles.set_attachment_limit`; an attachment above it is flagged with a reason. The thread shows a chip with name and size and a simulated loaded state (local UI only). |
| 340 | In the product the notification read state lives per clone on the local filesystem outside git, read and written by the daemon; the mockup keeps it in memory. |
| 341 | Tests keep local bare remotes (`file://` with `uploadpack.allowFilter=true` exercises a partial clone); a hosted private repository is used only at live acceptance (the caller child and the attachment ledger ticket), where HTTPS auth, host filter support, and latency show. |
| 343, 344 | A body-less `item.edit` that carries `mentions` or `attachments` stays flagged ("go with a body edit"); a removed member stays unmentionable (the mention is flagged), even where the text still shows `@name`. |
| 345 b | `attachments.max_bytes` is checked only against attachments a write adds. On `item.edit` and `comment.edit`, an `oid` already on the target's shown attachments in the author's seen state is exempt from the size check; one the author had not seen there is new. `item.create` and `comment.add` check every attachment; the shape checks apply to all. |
| 350 a | Every card has a maximum height (about 70% of the viewport) with internal scroll; fixed-height cards (Support agent) stay fixed; the Items panel may use the main zone's full height. |
| 352 a | *Notifications* stays as it is. |
| 353 | At 1280px the header's gear does not wrap: the mockup-only controls (viewing as, clone, clock) are grouped compactly. |
| 354 | Row actions and details shown on hover have non-hover equivalents: keyboard focus on a row shows the same, and on touch a tap selects the row and reveals them; one CSS state for hovered, focused, and selected. |
| 355 | Status is an icon that differs by shape as well as colour, with its description in an immediate custom tooltip rather than the browser's delayed `title`. The set is kept in one place (`status-icons.js`) so the dashboard can adopt it. |
| 356 b | An item row shows by default the status icon, title, assignee initials, priority (always), due (always), a warning icon for flagged or concurrent, and an unread dot. Due reads `D-N` within 7 days, `D-day` today, `D+N` past due in the danger colour, else a short date. Everything else (kind, source clone and event id, dates, roll-up, assignment path, notes, buttons) moves to hover. |
| 357 a | One *Items* panel replaces My assignments, Assigned by me, My team, Received requests, and Items: scope presets (mine / received requests / assigned by me / my team / all) crossed with the existing view modes. My team is a group-by-person view with each group header carrying that person's roll-up counts. Accept / Decline / Discuss / Confirm / Reject / Return / Close / Split / Hand off / Status are hover actions on the row they act on, and My team's "for reference" notices are left to the *Notifications* card. Supersedes the part of (331)-(334) that kept the notices and buttons on Received requests and My team. |
| 358 a | *Today* stays as a compact summary card (my open work / waiting for my answer / waiting for my confirm), visible whatever scope the panel shows. |
| 359 a | A two-zone drawer: the Items panel (about two thirds of the width) as the main zone; a side column stacking Notifications, Today, Support agent, Team, Activity, then Data (debug). At narrow widths the side column moves above the panel. The masonry packer is removed, and with the panel already wide, the wide-mode row spanning of (304) is gone too. |
| 360 | The panel opens on "mine"; the last chosen scope and view mode are remembered per store as local UI state, like the per-card mode memory. |
| 362 | The layout delegate's own choices are accepted: the row's unread dot means an unread *Notifications* entry for the item; pending holders' initials are dashed; the *my team* scope appears only for a viewer with followers; the row details are an overlay over the rows rather than an inline expansion (which shifted the layout under the pointer); the header's store chip is removed (the open handle already names the store). |
| 366 b | Layout A + D: the Items panel (about two thirds) with one tab pane on the right (Notifications, Support agent, Team, Activity, Data; one at a time, full height; Notifications by default; a dot on a hidden tab with something new); from about 1600px three columns, Items \| Notifications \| the tab pane; below about 1000px everything is tabs, Items first. Today becomes summary chips in the Items header (my work / answer / confirm) that filter the panel. The drawer height follows its content up to the existing maximum. Supersedes (359)'s stacked side column and (358)'s Today card (its intent, a summary visible whatever scope, stays). |
| 366 note | Clicking an item opens it in a window overlay (dimmed backdrop, centred window, Esc / backdrop / close button, focus in and back) instead of the thread sheet, holding the sheet's and the row overlay's content with the row's actions as labelled buttons; one window at a time. One function opens it by id (`openItemWindow(storeId, itemId)`) for later URL routes. The row's hover overlay stays as a quick preview. |
| 369 note | Item links are rejected: the URL is the viewer's own daemon address, so a link does not carry across people. The store page route stays. |
| 371 | Each store has its own page, the route `#/<storeId>`: the same Items panel and tab pane layout full-page rather than in the pull-down drawer, reached by *Open as page* on the drawer header; back, clearing the hash, or *Back to session* returns; a reload restores it. No item routes. |
| 372 b | A magnifier next to the gear opens a quick switcher searching stores and items by title (the viewer's visible items, from the view state); a store opens its page, an item its window; arrows, Enter, Esc, focus return. |
| 382 a | The drawer height is set by dragging its handle, like the dashboard drawer (about 160px to 60% of the window, saved per store as local UI state, restored on reload); panels scroll inside it. Only while a store's drawer was never dragged does the first open use the content-fit height, capped at the same maximum; once dragged, the user's height wins whatever the content or the active tab. Supersedes the content-height drawer of (366)'s plan (and (366 b)'s last sentence). Click-to-open/close stays; the store page is unaffected. |
| 383 b | A member's name is a profile, not a raw string: companies name people "SW개발팀 홍길동", and one string makes the org and the name run together. Asked for at the first join of a repo, as org / family / given. Display and avatar follow the rules under View state; the org is shown beside the name, never merged into it. |
| 391 | The structured profile `{org, family, given}` replaces `name` in the roles file, `roles.add_member` (the join stand-in), the view state's `members` and `viewer` (which keep a derived `name` and add `profile`, `avatar`), and the Go port. `org` is one free-text string, may be empty, and is no org tree. `given` is required and non-empty; `family` may be empty. A write missing `given` is flagged. The Go port reads `profile` from the genesis member and the `member.join` payload, and treats an older bare `name` as the given name. |
| 392 b | The member writes their own profile at join (in the join payload) and may change it with a `member.profile` event; an admin may change any member's with the same event naming `member` (a department move, a typo). Each field is its own register merged by (203); a non-admin writing someone else's profile is flagged; Activity shows it with the admin as author. Decided as partial writes (only the fields that changed) so that two people changing different fields do not overwrite each other, so "missing `given`" is flagged only on a join and for an empty `given`. |
| 393 | A person's earlier profiles are machine-local, never in the ledger. The mockup simulates the list with localStorage: a *Join a store* dialog (Team tab, or the quick switcher's "join" action) defaults to the last-used profile, picks any saved one, or edits the fields (which saves a new entry), then writes the join stand-in: `roles.add_member` from an admin's clone (the self-write `member.join` is not modeled), after which the new member's clone is opened. The real flow belongs to the caller ticket. |

| 384 | The tab pane's tabs and the Items panel's view switch are icon buttons (inline SVG, no external library) with the name in the tooltip and `aria-label`; the active tab stays highlighted and the "new" dot stays; the same icons serve the narrow layout and layout D. Frees the wrapped tab bar and row 2 of the panel header. |
| 385 | The panel header keeps two rows; row 2 is the view icons, then a search box taking the remaining width. It filters the current scope client-side as you type (title, body, project, item id), combines with the Today chip filter, Esc clears it, and it searches the closed section too. |
| 386 b | Done items leave the main list and appear in a greyed, expanded-by-default "recently closed N" section at the bottom of the Items list, covering the last 14 days (by the done assignment's last write, `lastUpdate`; no field added); older ones through search. Its open state is local UI state per store. Kanban keeps its done column; the timelines keep ended items, muted. |
| 387 a | The tree stays a tree: closed children among open siblings fold into one expandable "closed N" row under their parent; a closed parent with open descendants stays, greyed; a closed parent whose whole subtree is closed moves as one unit to *recently closed*; the parent's roll-up stays. |
| 389 | The Items panel, the tab pane, and layout D's Notifications column always span the drawer's height whatever their content (a short list no longer leaves a short card above empty drawer); long content scrolls inside each. The drag-set drawer height of (382) a stays. |
| 390 | Clicking a notification opens the item window and scrolls to and highlights what it refers to, found through the event id it carries (comment or mention -> the comment, with a brief flash and a thin left mark; status, close, or confirm -> the assignee's row in Assignees, expanding the fold when it is hidden; assignment or handoff -> the holders line of the property card; body edit -> the body; attachment -> its chip); `openItemWindow(storeId, itemId, {focus: eventId})` so a later URL route can reuse it; no target found just opens the window. |
| 424 | The item window's properties (status, kind, holders, creator, parent, project, id, dates, due) sit in a tinted, bordered card apart from the body; holders is one summary line ("13 · doing 3 · done 1": count, then each live status other than todo, then pending) instead of the list, which Assignees carries. |
| 425 | Assignees shows at most 4 rows, highest in the hierarchy first (shallowest lead-chain depth; then live before ended, display name, assignment order), then an "N more" toggle (local state). The provenance text and the close rule that at least two rows share are written once in the section header; a row keeps its name, status, note, and only what differs (e.g. "inherited: everyone within L2, reached later"). A notification target in the hidden rows expands the fold. |
| 426 c | The item window is two columns on a wide window (window width 820px or more): the main column has the header, body, subtasks, and thread with the composer; the narrow side column has the property card and Assignees. Below 820px it is one column: header, property card, body, subtasks, Assignees, thread, composer. Status, subtasks, and the thread are the prominent parts. |
| 427 | The fixtures carry hierarchical tasks of mixed depth with childless tasks among them: besides "Ship search v2" (depth 3 under "Index builder", and "Cache layer" under "Query path rewrite"), "Infra capacity plan" (three subtasks, one of which has its own blocked subtask, one done), "Refresh onboarding docs" (a single child), and the childless "Renew TLS certificates". The item window lists the children as a Subtasks section and shows the ancestors as a breadcrumb. Existing ids are unchanged. |
| 467 a | Every scope list (mine / received requests / assigned by me / team / all) is a context tree: each match's ancestor chain shows above it as greyed context rows (not counted, a click opens the item), unrelated siblings collapse into one "… N more" row, and the match's own subtree starts folded ("N subtasks"). Each node folds with ▸/▾; the fold state is browser storage per device, keyed by store + item id. Received-request sub-sections each draw their own tree; closed children keep the "closed N" fold. |
| 468 a | The due timeline uses the same tree rows: a parent draws its own bar and a thin summary bar spanning its children's range; folded, only the summary bar is left for the subtree; a parent without its own due is placed by its children's range. |
| 469 | Kanban and calendar cards stay independent tasks; hover or keyboard focus on a card highlights its direct parent and direct children cards, and a tooltip names the parent when its card is not on screen. |
| 470 | The item window's Subtasks section shows the whole subtree with ▸/▾ folding, first level open and deeper levels folded; supersedes (427)'s direct-children-only list. |
| 471 a | Status applies on click: the menu's status radios (todo / doing / blocked / done, done reading "request done" when the assigner confirms) write at once; a note field under them applies status + note together on Enter. |
| 472 b | The item modals become menu steps or inline inputs: status, close, subtask, priority, due, handoff, the reject / reopen / decline / return notes, comment edit, and item edit. Assign is an anchored popover from the menu; a new item is an inline input row atop the list (title + Enter, the rest from its menu). Only the store-wide forms (profile, admin) stay modal. Supersedes the action dialogs of (357) and the labelled window buttons of (366 note). |
| 473 | One context menu for list rows, kanban cards, calendar entries, timeline rows, and the item window header; opened by right-click, a "⋯" button (visible without hover on touch layouts), and Shift+F10 / the menu key; a click on a status icon opens only its status part. It shows only the actions the viewer's role and the item's state allow; submenus are steps in the same panel; arrows, Enter, and Esc work, and focus returns to the opener. Supersedes the row hover action icons of (354) / (357). |
| 474 | Inline subtask: an input row one level in under the row (in the item window's Subtasks for a kanban card, a calendar entry, or the window header); title + Enter creates an unassigned task with the default completion, Enter again adds a sibling, Esc closes it. |
| 490 a, 491 a, 492 a | "Everyone within N" generalizes to a subtree rule `{root, include_root, levels?}` (levels absent: the whole subtree), and one `item.assign` carries `targets`, a list mixing member ids and rules; the older `mode` forms stay valid, `all` reading as a rule rooted at the author. A rule rooted at the author or below is live and forced at once, inherited by later joiners. A rule rooted outside the author's reach is a rule request pending to its root; the root's accept makes the rule live with forced records and notifies the root's direct lead, who may return it, ending the rule. `item.unassign` gains `root` to end one rule. Rule ids `<event>~<root>`, records `<rule>><member>`; the view adds `rules` and each record's `rule` / `ruleRequest`. |
| 498 | The Activity tab's "flagged only" and "concurrent only" are toggle chips like the list / activity timeline chips (accent when on); no native checkbox remains in the drawer outside forms. |
| 499 | The comment composer follows the dashboard chat input: a paper-plane send button inside the box at the bottom right, Cmd+Enter / Ctrl+Enter sends and Enter is a newline, a small `@` picker at the bottom left; the composer's status line is gone (the status is the menu's). Supersedes the quick status of (166). |
| 500 | The store's sync rows under its name become a thin ribbon at the bottom of the Items card (status dot, short text, the apply button only while writes are pending, dimmed when idle); a click opens a popover with the clone ref, counts, and batch contents. The store header keeps one row: the name and the icon-only `Open as page` (its `+ New` left with (528 UI)). |
| 528 a | Items carry a sibling position: `item.create` takes `after`, a sibling id (`""` / `null`: first; absent: append, so old events keep creation order). The view's `itemOrder` and each `children` list follow the anchors in fold order; a missing, other-parent, or not-yet-placed anchor appends, a hidden one keeps its slot, and of two unseen inserts after one anchor the later lands nearer it. A later `item.move` extends the same field. |
| 528 UI | The store header's `+ New` goes. In the list view, hovering the left gutter near a row edge shows a `+` badge between rows with a guide line; the mouse's x picks the depth among the gap's choices (root, sibling of the row above or of one of its ancestors, child of the row above; a child only where the row may be split, so never under a topic), shown by the badge's indent. A click opens the inline new-item row in that gap, creating with `parent` and `after`; the row stays, and the next item goes after the one just made. A quiet `+` ends every Items list, an empty one too: a new root item at the end, and the keyboard path (there was no shortcut). The Subtasks section's `+` stays. |
| 529 | A `task` or `todo` created from the dashboard is assigned to its creator in the same batch (`item.assign` on `$1`, people = the creator, assignee closes); a `topic` is not. This covers the inline subtask row too, superseding (474)'s "unassigned task". |
| 530 a | Assign is a search popover anchored at its trigger (the assignee card's `+`, the menu's *Assign ▸*), replacing the checkbox grid: chips for the picked people and rules (each with ×); a search field; with no query, "My team" rule rows (within L1, within L2 when the team is that deep, the whole subtree), each with its head count; with a query, the whole-subtree rule of each matching member who leads anyone, marked pending when outside the viewer's reach; member rows with avatar, name, id, and a forced/pending mark; the completion mode; an apply button disabled while nothing is picked, replacing the "select at least one" message. Already held people and rules a live rule of the viewer's already covers are greyed. Arrows move, Enter toggles, Cmd/Ctrl+Enter applies, Esc closes. It writes `targets`, popover rules without `include_root`. |
| 531 a | Items and comments carry `refs`, the item ids their text references with `#<alias>`, riding with the body like mentions (the writer keeps a ref only while its token stays in the text). Each ref must be an item in the author's causal past. The view shows `refs` on items and comments, and `backrefs` on items: shown items, then shown comments on shown items, own thread excluded. |
| 20 b (11-09) | Assigning to someone else defaults to `assigner` (awaiting, then the assigner confirms); assigning only to oneself defaults to `assignee`. The fold's absent-means-`assignee` stays for old events; writers store the value. (59 a) The MCP writer fills an omitted `completion` the same way. (58) The assign popover defaults to `assigner` when any picked person or rule is not the viewer, `assignee` when the viewer is the only target. |
| 21, 57 (11-09) | Done and blocked of work I assigned are derived notices to the assigner when someone else set them (awaiting stays in the confirm list), in Go and `engine.js`; no notice events. Being notices, they count on the handle badge, which (331) left unchanged when the Notifications card derived them client-side. |
| 22 a, 32 a, 33 a, 34 c, 51-56 (11-09) | A review stage ("검토") before todo: the item field `stage` (`review`, or absent for todo) on `item.create` / `item.edit`, tasks and todos only, under the `item.edit` permission (promotion is `stage: null`). A review item has status `review` and is left out of roll-ups, review subtasks too. Assigning promotes it (the assign permission alone); moving back to review is refused while a live or pending assignment exists, and a concurrent move folded after an assign is dropped. The "all" kanban gets a review column first with a `+` that creates a review item, not self-assigned (narrowing 529); the item menu promotes ("할 일로 올리기") and demotes. Every other creation path stays todo plus self-assign. |
| 66 a (11-09) | A task or todo an agent creates through the MCP tracker tool is self-assigned like (529) when the same batch does not assign it, unless it is created in review. |
| 531 UI | The composer's `#` opens the `@` popup's twin, searching shown items by alias and title (alias prefix, title prefix, then contains); a pick inserts `#<alias> `. `refs` are derived from the `#<alias>` / `#<id>` tokens in the text at write time (comment, comment edit, body edit), not kept from the picks. `#<alias>` in bodies and comments renders as a link to the item, its title on hover. The item window shows "Referenced by" (참조됨) under Subtasks when the item has backrefs: items, then comments with their author and a snippet; a comment row opens its item at the comment. |

## Open questions

The model snags the scenario walk found are resolved by (284)-(292); see
[SCENARIOS.md](SCENARIOS.md). (295) settled the last one: when the original of
a pending handoff ends another way (unassign, removal, return, or a
concurrent second handoff accepted), the pending handoff ends as `withdrawn` with no event, its
`ended` pointing at the event that ended the original. An unassign, removal,
or return of the original concurrent with an accept of its handoff wins, as
removal beats writes (213): the accepted handoff ends `withdrawn`, the original
takes the ending event's state, and both events are marked. None is open now.

## Not modeled in v3

Real signatures and crypto; git refs, pushes, and fetches (sync is simulated
per clone); the background poll and the 5-minute push delay (push happens at
Sync, panel collapse, and viewer switch); the self-join (stand-in:
`roles.add_member`) and claim refs; the local saved-profile list beyond
localStorage; re-issue of a revoked key's own events;
search; the MCP surface; notifications beyond the handle badge, the derived
notices, and the notification card (no push, mail, or OS notification; read
state is in memory, not on disk (340)); attachment content storage and fetch
(337); labels and tags (278 a); multiple leads; snapshots; a
working support-agent chat (the chat card is a static placeholder); styling.
