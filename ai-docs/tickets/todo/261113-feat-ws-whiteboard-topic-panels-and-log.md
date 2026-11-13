---
title: "Whiteboard topics carry panels and a log; per-topic folder storage"
related:
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "follower; lands after this ticket and uses the latest agent log line instead of recap"
  261113-feat-ws-whiteboard-update-refusal-cost: "related; its wb:append proposal is superseded, not its remaining refusal/answer work"
---

# Whiteboard topics carry panels and a log; per-topic folder storage

## Background

Owner direction, 2026-11-13, whiteboard: separate durable current state from
accumulating discussion. Replacing a large body for every opinion loses the
conversation; keeping everything in that body makes the current state hard to
find. A topic needs content, substantial panels carrying a recap, and a compact log.
This ticket establishes that model before the sticky-head follower.

Verified implementation baseline (coordinates are evidence, not edit targets;
search by the named symbols when implementing):

- Grammar: `grammar` in
  `agents-plugin-tool/internal/wswhiteboard/commands.go:21-37` scopes `body`,
  `edit`, `recap`, `add`, and `close` under `wb:topic`. Neither `wb:panel`,
  `wb:log`, nor `wb:append` exists. Item fields are a separate contract
  (`commands.go:10`).
- Store model: `TopicDoc` is body plus replace-whole recap, their independent
  change marks/times, and body diff marks
  (`agents-plugin-tool/internal/wswhiteboard/store.go:214-237`). `State`
  carries session-wide counters, items, topic documents, labels, close order,
  embed bookkeeping, and nudge state (`store.go:244-287`); neither type has
  panels, an entry stream, or a schema version.
- Mutation: `Apply` clones state, dispatches commands in order, and refuses
  the entire batch on any problem
  (`agents-plugin-tool/internal/wswhiteboard/store.go:617`). Event producers
  belong at accepted mutations, not at rendering: `applyAdd` (`store.go:845`),
  `applyAnswer` (`:894`), `applyPatch` (`:989`), `applyBody` (`:1088`),
  `applyRecap` (`:1108`, to retire), `applyEdit` (`:1123`), and `applyClose`
  (`:714`). `applyAnswer` stores both the interpreted value and the owner's
  verbatim quote. `applyClose` withdraws still-open items. These facts matter
  for log provenance and avoiding events for refused work.
- Persistence correction to the owner's shorthand: the board is not normally
  in today's `key.json`. `sessionRecord.Whiteboard` is the `whiteboard.json`
  part (`agents-plugin-tool/internal/mcp/session_auth.go:111-114`;
  `agents-plugin-tool/internal/mcp/session_layout.go:22-39`, `:87-88`).
  Pre-split `keys/<key>.json` still contains the whole record. Its first
  mutation migrates it and deletes that legacy file; reads do not migrate
  (`session_layout.go:35-39`). Thus preserving only fields in `key.json`
  would not preserve the actual old board. Preserve all old board-bearing
  representations; do not accidentally invoke the old destructive migration
  for this conversion. The existing record lock is documented at
  `session_layout.go:42-48`.
- Host writes: `handleWhiteboardUpdate` mutates `partWhiteboard`, calls
  `Apply`, then renders (`agents-plugin-tool/internal/mcp/whiteboard_tools.go:225`,
  `:254`). `renderWhiteboard` owns file I/O (`:429`), writes a new embed
  (`:465`), then `<reply-id>.html` (`:502`) and `<reply-id>.stamp.js` (`:506`),
  returning the flat HTML path (`:509`). HTML is a generated view, **not**
  today's store of record. The new session `index.html` replaces this output
  layout, while topic folders become authoritative.
- Rendering: `RenderPage`
  (`agents-plugin-tool/internal/wswhiteboard/render.go:215`) serves file and
  embed layouts; `RenderRegions` (`:251`) serves dashboard fragments and
  sandboxes topic bodies. The template's `recap`, `doc`, and `topic` blocks
  are `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl:32`, `:34`,
  `:36`; its existing `panel` block (`:90`) is for **session auxiliary
  sections**, not the new topic panels. Likewise `View.Panels` and `Panel`
  are provider views (`agents-plugin-tool/internal/wswhiteboard/view.go:38-39`,
  `:303`), not durable cards. Do not confuse the two concepts.
- Layout evidence: the file page has a session-level auxiliary sidebar
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl:9`;
  `agents-plugin-tool/internal/wswhiteboard/assets/page.css:134-135`). Recap
  and body styles start at `page.css:395` and `:403`; served body frames at
  `:428-429`. Frame clicks already post fragment jumps
  (`agents-plugin-tool/internal/wswhiteboard/assets/doc-frame.js:161-172`),
  but the host accepts only `item-N` targets
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:3109-3111`).
  Plain `#p7` links therefore need host support as well as new DOM IDs.
- Dashboard refresh: `renderBoardNow` builds the same view and regions
  (`agents-plugin-tool/internal/mcp/dashboard_page.go:206`, `:233`). Its memo
  keys on board sequence and watched inputs (`:178`, `:198`), while the
  current state arrives through the session record. Moving persistence must
  also move the read/fingerprint path, not merely change the template.
- Query: `queryFields` has no entry selectors
  (`agents-plugin-tool/internal/wswhiteboard/query.go:23`); a topic-targeted
  query defaults to printing its body (`:158`). `FormatQuery` prints recap,
  optional body, and items (`:324`). Text search currently searches item
  fields, not topic content or panel payloads (`:292-315`). The handler reads
  `partWhiteboard` (`agents-plugin-tool/internal/mcp/whiteboard_tools.go:304-318`).
  Revive/session-state summaries also consume recaps through
  `renderWhiteboardRecaps` (`agents-plugin-tool/internal/mcp/workflow_manual.go:159`),
  so removing the command alone is incomplete.
- Mentions currently carry only topic, optional numeric item ID, and selected
  text: `Mention`, `MentionBlock`, and the strict `mentionHead` parser
  (`agents-plugin-tool/internal/wsinbox/mentions.go:25-32`, `:43`, `:59`).
  The dashboard's `hostMention` finds a topic/item, while `frameMention`
  supplies only the topic for a body-frame selection
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:1745-1755`,
  `:1805-1808`). Draft/cancel restoration drops unknown mention fields
  (`:1888-1891`). Entry attribution must survive that whole path, not merely
  appear in rendered DOM. The canonical playbook teaches the old shape
  (`agents-plugin/rsrc/whiteboard/whiteboard.md:317-326`).
- Content references are distinct from user-selected mentions. `rawMarkdown`
  and `Inline` already resolve `{{nn}}` in authored Markdown
  (`agents-plugin-tool/internal/wswhiteboard/markdown.go:185-207`); body
  references do not resolve inside raw HTML (`:193-195`) or code spans/blocks
  (`:575-577`). Existing patterns accept digits only, so `{{p3}}` has no
  panel-reference meaning today (`markdown.go:55`;
  `agents-plugin-tool/internal/wswhiteboard/render.go:74-77`).
- Playbook copies all still describe body/recap and the old query: canonical
  `agents-plugin/rsrc/whiteboard/whiteboard.md:36`, `:174`, `:197`, `:213`;
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md:36`, `:174`, `:197`,
  `:213`; `agents-plugin-pi/rsrc/whiteboard/whiteboard.md:36`, `:174`, `:197`,
  `:213`. The latter two are byte-identical distribution copies, not places
  to develop independent contracts.

The daemon rationale is supported by `keepRunning`
(`agents-plugin-tool/internal/wsdashboard/ensure.go:170`) and
`CompareVersions`/`SameMinor`
(`agents-plugin-tool/internal/wsdashboard/version.go:24`, `:55`). Hello
rejects a differing minor when a version is supplied
(`agents-plugin-tool/internal/wsdashboard/proxy.go:251-253`); an empty version
bypasses that check. Do not describe this as a universal store-schema guard.
The compatibility manual also describes future cross-minor acceptance; if
that work lands first, recheck the rationale without silently changing the
owner's T2/0.50/no-dual-write decision.

## Decisions

All decisions are owner-settled on 2026-11-13 via the whiteboard. Decisions
1-11 were settled first; the rest record the settled decision queue:
owner-confirmed technical items, owner policy answers, and the lead's
assumed reading where stated. Parenthesized numbers in Decisions are board
item IDs.

1. **Topic model:** a topic is content (the durable current state, today's
   body) plus an entry stream with two kinds. A panel is a content-sized
   card carrying a `recap` (see 12) for mockups, long opinions, images, and
   similar material; it is collapsible, with the latest three panels expanded, and
   has a stable `pN` ID that content can link to, for example `#p7`. A log
   entry is one line, either agent-authored or a system event written by the
   server: item added by class, item closed/answered with the owner answer,
   content edited, panel added linking to `#pN`, and topic closed.
   Rejected: putting accumulated discussion into the replace-whole current
   state, or treating substantial cards as one-line log text.
2. **Recap retirement:** the latest **agent** log line takes over the recap's
   role; remove `wb:recap`. Drop the refusal-cost ticket's proposed
   `wb:append`; that proposal is superseded, not its other work.
   Rejected: another independently maintained recap or an append-to-content
   command as the accumulation mechanism.
3. **Separate commands:** add `<wb:panel recap="...">payload</wb:panel>` and
   `<wb:log>text</wb:log>` inside `wb:topic`, so their display policies can
   differ. Rejected: a single undifferentiated append command for both kinds.
4. **Interleaved layout (69=b):** panels and log lines occupy one
   time-ordered column below content. Topic cards use full horizontal width,
   with no topic side rail. Rejected: stacked content/panel-deck/log-tail
   regions and a side rail.
5. **System-event folding (57=b):** fold system events between agent log
   lines behind a `system events n` chip; panels and agent log lines are the
   main visible material. Rejected: giving each system event the same visual
   weight as an agent line or a panel.
6. **Folder source of truth (54):** use
   `ai-docs/whiteboards/<session>/<topic>/` with `meta.json`, `panels/`
   (one file per panel entry; each version of a `pN` is its own file, see 13), and append-only `log.jsonl`, plus a session
   `index.html`. Leave old Whiteboard fields in place untouched: rollback
   shows the old board, not post-conversion changes. Apply this preservation
   intent to the actual old stores identified above. Rejected: the current
   single HTML page per session as the new storage design, and putting
   panels in `message.jsonl`.
7. **Compatibility (55):** ship in the **0.50 minor**, convert the old format
   once, and do not dual-write. Tier **T2** covers the non-additive store
   shape and removal of `wb:recap`; it is not patch-release work. Rationale:
   after the single-executable change only the daemon reads/writes the
   store; within a minor the newest daemon wins, and the current hello
   refuses a minor mismatch. Rejected: a patch rollout, dual-write, or a
   continuing old-format command alias. The schema-version obligation in
   Constraints still applies.
8. **Accumulation-aware query (56):** default output is metadata only: each
   topic's current-state line, last k log lines, and panel recap list, never
   panel bodies by default. Add `entries: "log"|"panel"|"all"`, `since` (time
   or entry ID), `last: N`, `panel: "p7"` (the latest version's body verbatim, so `wb:edit` can
   quote it), `kind: "agent"|"system"`, and `text` (recap plus body search).
   Rejected: dumping accumulated panel bodies into normal discovery output
   or retaining the old item-only query as the entire interface.
9. **Delivery order (58=a):** this ticket precedes
   `261112-feat-ws-whiteboard-topic-tickets-sticky-head`; the follower's head
   shows the latest agent log line instead of recap. Rejected: implementing
   the follower's recap-based head first and replacing it immediately.
10. **User-selected entry mentions:** selecting text inside a panel or log
    entry must produce a `<wb:mention>` identifying that entry as well as
    topic and item ID where applicable, so the lead can resolve exactly what
    was quoted. This includes both agent and system log entries. The locator is
    an optional `entry` attribute (see 44) carrying the panel's or log
    entry's durable sequence ID (see 15, 29, 45). Rejected: silently reducing an entry selection
    to today's topic/item-only attribution or overloading the numeric item
    ID with a panel ID.
11. **References authored in entries:** `{{nn}}` item references resolve in
    panel and log text as they do in topic bodies. These are authored
    references, not `<wb:mention>` elements. Panels themselves are reference targets
    through `{{p3}}` (see 47). Rejected: rendering supported item references
    as unresolved text merely because they occur in the new entry kinds,
    or treating authored references as user answers/selections.

12. **Panel recap (240=a):** a panel carries a `recap` attribute with an
    OPTIONAL payload; this replaces the earlier "title" concept everywhere.
    A panel carrying only a recap reads as a message. The word `recap` here
    is the panel attribute, unrelated to the retired `wb:recap` command (2).
    Rejected: a required title plus mandatory payload, and a separate message
    entry kind.
13. **Panels are append-only versions (207=b):** panels are rewritten only as
    a full rewrite via ID override: an existing panel with the same ID
    folds. The message system is append-only (owner note). There is no
    in-place panel edit, so there is no `panel` attribute on `wb:edit`;
    `wb:body` and untargeted `wb:edit` stay the current-content interface.
    Rejected: in-place correction under the same ID with a server-written
    edit event; targeting panel edits with `<wb:edit panel="p7">` (208);
    carrying a revision/hash with a panel mention and reporting a version
    mismatch (233).
14. **Re-post appends a new version (376):** re-posting a panel under the
    same ID appends a new full version, and older versions of that ID fold
    under the latest. `#p7`, `{{p7}}` and navigation target the latest
    version. Attaching a payload later to a recap-only message (12) is the
    same re-post. Follows from 13; 30, 39 and 47 all point at the latest
    version. Owner-stated: the whiteboard playbook text tells the lead to
    split a content-heavy panel into several (sub)panels, not one huge
    panel. Rejected: replacing the earlier version's stored entry, and
    leaving older versions expanded beside the latest.
15. **Panel version identity (377):** a panel mention identifies the selected
    version by the panel entry's persistent sequence ID, with no hash. Under
    13 each version stays a separate entry, so the sequence ID of 29/45 is
    the version identifier. Rejected: revision/hash metadata on the mention.
16. **Three phases (201):** store/conversion/commands, then rendering, then
    query/playbooks. Three dependency-ordered review slices over one large
    phase: persistence and recovery verify before UI behavior, then public
    discovery and guidance check against the finished model. Rejected: one
    large implementation phase.
17. **History paging (202=a):** bounded-height history with explicit
    older-page loading, preserving chronological order and the viewport
    anchor when history or new entries arrive; the window size is settled
    before Phase 2. Rejected: an unbounded document with a show-more
    boundary.
18. **Embed snapshots (203=c):** embeds carry panel recaps and the latest
    agent line only, with a link to the full board; accumulated payloads are
    not duplicated in every chat snapshot. Rejected: the full stream, and
    newly added panels plus a short log tail.
19. **Item semantics preserved (204):** `wb:why`, options, recommendations,
    links, check answers, and item references remain item data, not
    auto-converted panels. Citation:
    `agents-plugin-tool/internal/wswhiteboard/commands.go:10`, `:31-34`;
    `agents-plugin/rsrc/whiteboard/whiteboard.md:108-128`. The refusal-cost
    ticket still owns its separate answer-status change.
20. **Conversion on first open (205):** run conversion on the daemon's first
    board open, including a query or dashboard read, through one locked
    loader, so every consumer sees one authority and unopened boards cost
    nothing. Rejected: update-only conversion and eager whole-cache
    conversion.
21. **Log-line markup (206=b):** an agent log line accepts inline Markdown
    only, without images, raw active HTML, or block structures; substantial
    material goes in a panel linked from the log. Rejected: plain text only,
    and block Markdown collapsed visually.
22. **Current-content interface (209):** keep `wb:body` and untargeted
    `wb:edit` as the current-content interface, with replace-whole/clear/
    no-op and exact-match/replace_all behavior, rather than interpreting
    existing edits as stream appends. Citation:
    `agents-plugin-tool/internal/wswhiteboard/store.go:1088-1104`,
    `:1123-1171`. Explicit content retrieval remains necessary even though
    discovery is metadata-only.
23. **Panel payload size limit (210):** measure representative mockups and
    images, then set an explicit UTF-8 byte cap with a concise refusal and
    an external-file link alternative; the numeric cap is settled before
    Phase 1. Rejected: no per-panel cap, and external-file-only large
    payloads.
24. **Refresh path (211):** refresh new entries through the existing board
    SSE region and watch/memo pipeline, reusing `RenderRegions` and
    disclosure restoration and adding authoritative topic metadata/log/panel
    inputs to fingerprints; identity must distinguish multiple frames within
    one topic. Rejected: a second entry-specific SSE protocol.
25. **Topic meta.json content (212):** each topic's `meta.json` holds
    current content, items, labels, and close state, keeping the folder
    self-contained while panel payloads stay in their own files and
    `log.jsonl` stays append-only. Rejected: an additional content/items
    file family.
26. **Session metadata (213):** keep board-wide counters and session
    bookkeeping in a versioned session `meta.json` beside `index.html`:
    item/panel counters, render sequence, topic order/mapping, and embed
    bookkeeping; HTML stays derived output. Rejected: writable cache
    Whiteboard fields, which would violate no-dual-write.
27. **Recap conversion (214):** convert an old recap into one
    provenance-marked imported agent log entry with its original recorded
    time; body and items are preserved as snapshots. Rejected: discarding the
    recap, and fabricating a historical stream.
28. **Directory identity (215):** use reply-id session directories and
    opaque stable topic directory IDs mapped to logical topic keys, avoiding
    exposed session keys, traversal/Windows-name problems, and moving panel
    history when a stemless topic is adopted or a label changes. Rejected:
    raw session keys or labels as path components.
29. **Entry ordering ID (216):** every stream entry gets a durable monotonic
    sequence ID and timestamp; since-by-ID needs a stable boundary
    independent of rendering and daemon restarts, and entries in one update
    can share a timestamp. Rejected: timestamp-only sorting.
30. **Board-wide panel IDs (217):** allocate `pN` board-wide, so the bare
    `#p7` fragment identifies exactly one DOM target across topics and
    survives topic renames. Rejected: topic-local duplicate `pN`.
31. **Recoverable commits (218):** use a per-session lock with explicit
    recovery/commit metadata for store writes and conversion, giving crash
    recovery and exactly-once publication; the recovery record format is
    settled before Phase 1 and every boundary is fault-tested. Rejected:
    independent best-effort panel/log/meta writes.
32. **Default log tail (219):** ten entries per topic, with explicit `last`
    overriding it and omitted counts preventing omission from looking like
    absence. Rejected: an unbounded tail.
33. **Legacy item selectors (220):** retain explicit id/ids/status/depth/
    fields and body retrieval as item queries alongside the entry queries,
    so callers keep exact answer quotes and edit anchors; no selector
    implicitly dumps panel bodies. Rejected: removing item/owner-answer
    access.
34. **Filter before limit (221):** apply entry filters before the last-N
    limit, so kind/text/entries queries do not lose matches to unrelated
    entries. Rejected: limit-then-filter.
35. **Panel rendering (222=a):** reuse content Markdown with inline HTML and
    the existing full-document restriction, sandboxed on the served page, so
    images and mockup scripts work without acquiring the dashboard origin.
    Rejected: static sanitized Markdown only.
36. **Extra system events (223):** record system events for item patches and
    topic reopen transitions as well as the minimum event list, so re-asks,
    withdrawals, and corrections leave history; unchanged writes and
    render-only refreshes emit nothing. Exact wording and
    answer-versus-assumption labels are settled in tests. Rejected: logging
    only the confirmed minimum.
37. **Entry activity (224):** adding a panel or agent log line retains and
    reopens a topic, so panel-only and log-only topics survive pruning;
    system-generated events alone do not independently reopen their topic.
    Rejected: silently appending into an invisible closed topic.
38. **Folder lifetime (225):** preserve authoritative topic folders after a
    session key expires until explicit deletion is designed; orphan cleanup
    stays limited to derived artifacts. The current collector
    (`agents-plugin-tool/internal/mcp/whiteboard_tools.go:657-696`) would
    delete the source of truth if applied recursively. Rejected: timed
    expiry, and recursive orphan deletion.
39. **Frame-jump bridge (226):** extend the validated frame-jump bridge with
    a narrow existing-board target allowlist to reveal and navigate `pN`
    targets (the latest version, 14), loading/revealing an older folded
    panel when necessary, without broadening privileged host actions.
    Rejected: arbitrary frame-supplied selectors or URLs.
40. **Auxiliary sections (227=b, lead's reading, assumed):** file-page
    auxiliary sections go below the full-width board, and the dashboard
    keeps its existing auxiliary drawer. This rests on the owner's
    observation: "Isn't that area already folded into the top-right drawer
    anyway?" The new topic-panel concept does not remove those session
    features. Rejected: retaining the separate session sidebar beside
    full-width topic cards.
41. **since boundary (228):** `since` is an exclusive boundary accepting
    RFC 3339 time or a stored entry ID, so repeated incremental queries do
    not repeat the boundary entry and equal-time ties stay addressable.
    Rejected: inclusive cursors.
42. **Output order (229):** print selected query entries oldest-to-newest
    within the selected last-N window, matching the visual stream while
    `last` still selects the newest matching tail. Rejected: newest-first
    output.
43. **Selector validation (230):** refuse incompatible panel-body and
    stream-filter query selectors with explicit errors; accepted
    combinations are specified in query tests. Rejected: silently ignoring
    selectors.
44. **Mention entry attribute (231):** add an optional `entry` attribute to
    `wb:mention` without changing topic or numeric item `id`; the typed
    locator spelling is defined alongside the durable IDs before
    implementation. Rejected: overloading `id` or replacing the element.
45. **Log entry identity (232):** mentioned log entries are identified by the
    durable sequence ID of 29; timestamps stay context, not the sole key, so
    equal timestamps, repeated text, and restarts cannot redirect a quote.
    Rejected: wall-clock-only attribution.
46. **Cross-entry selections (234):** a selection crossing several entries
    splits into ordered entry-specific mentions, each retaining its selected
    text; no entry is credited with text selected from several. Rejected:
    refusing spanning selections, and a topic-only mention.
47. **Panel references (235=a):** support `{{p3}}` across item fields, panel
    text, and log text using stable panel IDs (resolving to the latest
    version, 14) and reveal/load navigation; code/raw-HTML exclusions align
    with body item references and unknown targets stay literal. Rejected:
    explicit Markdown `#p3` links only.
48. **Mention tier (236):** classify the emitted entry-aware `wb:mention`
    shape as T2 and release it in 0.50. `mentionHead` accepts only topic
    then optional numeric id
    (`agents-plugin-tool/internal/wsinbox/mentions.go:25`) and dashboard
    restoration discards other metadata
    (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:1888-1891`).
    Dashboard emission, inbox/lead parsing, and mirrored guidance update
    together while old-shaped mentions keep parsing. Rejected: treating an
    optional attribute as automatically T0 (a T0 alternative must prove
    old-reader compatibility first).
49. **Entry retrieval (237):** add an explicit `entry` query selector for
    direct single-log-entry retrieval by the mention's locator, returning
    topic/kind/time and verbatim text without unrelated entries; panel
    locators use the panel-body selector. Rejected: reconstructing a log
    record through tail/since searches.

## Constraints

- `ai-docs/manuals/release-compatibility.md` applies to the shared store,
  command removal, and query contract. Raise the affected store's
  `schema_version`; a writer encountering a newer schema must fail loudly
  without writing. The minor bump alone is not the schema guard. Verify
  upgrade, restart, and downgrade behavior against real legacy layouts.
  Mention compatibility must cover dashboard assets, inbox serialization/
  parsing, and the lead's playbook parser together. Decision 48
  classifies the emitted `<wb:mention>` entry-attribute extension as **T2**
  for 0.50: today's strict parser rejects extra attributes. Asset-only
  styling is T0, but that does not make the end-to-end shape change T0.
  Retain acceptance of existing topic/item-only mentions; any alternative
  T0 encoding needs demonstrated old-reader compatibility before adoption.
- `ai-docs/manuals/shipped-surface-boundary.md` applies to tooling and all
  shipped text, including the Pi copy: no repository-only ticket IDs,
  manuals, or implementation paths become downstream playbook requirements.
- `ai-docs/manuals/skill-authoring.md` applies to canonical playbook changes
  and their copies. Keep judgment guidance distinct from discoverable
  schema/output mechanics; do not retain recap-era instructions that fight
  the new model.
- `ai-docs/manuals/wsflow-mirroring.md` applies to the playbook distribution
  across `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`, and
  `agents-plugin-pi/rsrc/`. Edit the canonical source; regenerate its rsrc
  manifest and wsflow tree in the documented order with `-count=1`. Resync
  Pi's rsrc tree byte-for-byte as required by
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go:19-29`; do not
  independently hand-edit either distributed copy.
- `ai-docs/manuals/ws-mcp.md` additionally applies to changes under
  `agents-plugin-tool/internal/mcp/`, including persistence adapters,
  dashboard reads, and tool output. Default query output stays compact text.
- Preserve dashboard-origin isolation for content-sized payloads. The
  existing `framedDoc` sandbox deliberately omits `allow-same-origin`
  (`agents-plugin-tool/internal/wswhiteboard/render.go:476-521`). A new
  panel must not become a route for scripts into the privileged host.
- Conversion must preserve the old board data, item IDs/answers, and topic
  identities; never manufacture historical owner answers or events that the
  old snapshot cannot establish. New-format writes never update old board
  fields. Old-format rollback is intentionally stale, not synchronized.
- Partial application, failed-command echo, and note-only answer status
  remain owned by `261113-feat-ws-whiteboard-update-refusal-cost`. Integrate
  with whichever mutation contract has landed; do not implement its rejected
  `wb:append` or change batch atomicity incidentally here.
- Do not implement the follower's ticket badges/glow/sticky head in this
  ticket. Reconcile its recap references and sequencing before that ticket
  is dispatched; leave its frozen results, if any, intact.

## Implementation Parameters

- The panel payload cap is **2 MiB of UTF-8 bytes**. Representative fixtures
  measured 144,150 bytes for mockup HTML and 614,025 / 606,029 bytes for
  embedded-image Markdown; larger material links an external file.
- Durable entry locators are `eN`, board-wide across both logs and panel
  versions; stable latest-panel targets remain `pN`. A panel mention's `eN`
  therefore selects its immutable version, not whichever version is latest.
- The schema-2 folder transaction atomically installs `pending.json` as its
  commit decision, containing exact panel/topic/session bytes and log append
  offsets. Locked replay completes partial appends idempotently, publishes
  session metadata last, then writes a `committed.json` receipt before removing
  the intent. The receipt prevents stale legacy fallback after publication.
  Derived output publishes only after commit, under the same lock.
- History starts with **50 entries per topic**, loads older pages of 50 into
  a bounded-height scroll area, and preserves the visible entry/offset through
  older loading and SSE updates. A viewer at the bottom follows new entries;
  one reading history keeps that anchor. Embeds omit the stream payloads.

## Phases

The three-phase split is Decision 16. Intermediate phases are not independently shipped
patches: the breaking surface releases together in 0.50.

### Phase 1: Authoritative topic folders, conversion, and commands

Implement the folder-backed read/write boundary, versioned metadata, one-time
conversion, panel files, append-only log, and the new scoped commands. Retire
`wb:recap` without an alias; continue to refuse `wb:append`. Adapt all board
readers (update, query, dashboard, session-state summary, and implicit panel
refresh) to the authoritative store rather than adding a second writable
cache. Keep pure command validation/model transformations testable separately
from host I/O; any changed package ownership must be explicit.

Generate the confirmed system events from successful state transitions,
including the item class, owner answer where applicable, and panel link.
Keep agent lines distinguishable from server events so only an agent line
can supply the current-state summary. Preserve old board-bearing files even
when ordinary session metadata changes; check the existing legacy session
migration path rather than assuming it is safe. Render work beyond an
adapter for the new model is deferred to Phase 2; final query presentation
and shipped guidance are deferred to Phase 3.

Verification expectations:
- Store/grammar tests cover a content-only, panel-only, and log-only topic;
  scoped command validation; stable panel IDs; multiple entries in one call;
  refusal of removed/unknown commands; and no log event for refused work.
- Add/answer/patch/body/edit/close event tests include grouped answers,
  assumptions, withdrawals, repeated/no-op mutations, and close after answer,
  using the event coverage and mutation contract settled from the queue.
- Conversion fixtures cover both legacy inline and split `whiteboard.json`
  boards, stemless names, recap-only topics, empty/closed boards, and unknown
  newer schemas. Compare preserved legacy board bytes, reopen the new store
  after daemon restart, and prove a second open does not import duplicates.
- Fault-injection/concurrent-writer tests at panel, log, metadata, conversion,
  and publication boundaries prove the settled recovery protocol: no success
  response with missing durable data, no duplicate committed entries, and no
  fallback to stale legacy data after a new store has committed.
- Exercise an older binary/read fixture against the preserved old board:
  rollback shows its pre-conversion snapshot without new-format dual-write.
- Expose durable entry locators for mention resolution; after restart each
  locator still addresses the same topic entry, including log entries with
  equal timestamps. Panel mentions identify a version by its sequence ID (see 15).

### Result (972766abc) - 2026-11-13

Implemented schema-2 topic folders, one-shot locked conversion, immutable panel
versions, append-only entries, scoped panel/log commands, transition events,
and removal of recap without an alias. All board readers now share the folder
authority; original inline/split board snapshots survive new writes and normal
session metadata migration. Derived index/stamp publication follows the durable
commit under the same lock. Package/runtime versions target 0.50.0.

Remote focused model/folder/MCP tests passed, including boundary fault replay,
concurrent writers, exact legacy bytes, restart/no duplicate import, newer-schema
refusal, stable equal-time locators, and rollback fixtures. The full remote Go
suite passed with the owner-known LAN daemon replacement test excluded; the
unfiltered run reproduced that known failure. Self-review found no remaining
substantive Phase 1 issue. Rendering and final discovery remain in Phases 2/3.

### Phase 2: Interleaved topic rendering and live refresh

Depends on Phase 1. Build a shared entry view for the file page and dashboard
regions: content followed by the chronological mixed stream, panel folds showing the
recap, the latest three panels initially expanded, agent log lines, and
folded system-event runs. Implement the settled history-window, embed,
fragment-navigation, and auxiliary-section placement choices from the queue.
A content link to `#pN` must reveal and reach its panel even if folded or
outside the loaded window. No topic side rail or separate panel deck.

Integrate durable-store fingerprints, memo invalidation, SSE region swaps,
frame identity/height, and disclosure preservation. Publish the session
`index.html` and its change signal only from committed state. Keep all views
on the same stored entries; render/refresh alone must never append a system
event. The follower's sticky head is deferred, but expose the latest-agent-line
view it will consume.

Verification expectations:
- Render/view tests assert order with equal timestamps, panel IDs/recaps, folded older versions of a re-posted ID,
  latest-three expansion, event-chip counts without swallowing/reordering
  panels, closed topics, and no agent summary replacement by a system event.
- Browser checks on file and served pages cover light/dark, narrow/wide,
  long payloads/images, keyboard disclosures, history loading, and `#pN`
  navigation from sandboxed content. No page errors or horizontal topic rail.
- Security tests retain sandbox boundaries and reject forged frame jump or
  image relay targets. If panel payloads use frames under (222), multiple
  frames in one topic retain distinct stable identities during updates;
  content scripts cannot access the host origin.
- Integration tests append an agent line and panel, mutate an item, and
  restart the daemon while a dashboard page is open: changed regions refresh,
  unchanged inputs reuse the memo, and user disclosure/scroll state follows
  the settled policy. Exercise implicit auxiliary-panel refresh too.
- Test user selections inside panel recaps/payloads and agent/system log
  lines, on host DOM and sandboxed-frame paths as applicable. Each delivered
  mention retains topic, entry locator, selected text, and item ID only where
  applicable through draft save/restore, merge, cancel, interrupt, steer,
  inbox persistence, and serialization/parsing. Entry identity comes from
  trusted host-owned node/frame metadata, not arbitrary frame-supplied IDs.
  Cover cross-entry selections under (234), old-shape mentions, escaping,
  older panel versions, and deferred history loaded before a selection.
- Test `{{nn}}` in panel and log Markdown against body behavior: known/on-page,
  known/off-page, unknown, code, and raw HTML cases; frame-origin references
  still jump to the correct item without widening the host allowlist. Test
  panel references from every source surface selected under (235).
- Keep the embed size-budget test meaningful. Its actual current cap is
  **44% of 512 KiB**, not the follower ticket's stale 42% citation
  (`agents-plugin-tool/internal/wswhiteboard/render_test.go:891-903`).

### Phase 3: Query contract and synchronized playbook guidance

Depends on Phases 1 and 2. Implement the confirmed query selectors and
metadata-only discovery through the new store, including recap/body search
and explicit verbatim retrieval of one panel. Resolve legacy item-query
coexistence and selector precedence from the queue rather than accidentally
removing answer-quote retrieval. Replace recap-based session-state/revive
summaries with the latest agent line. Provide the settled direct entry
retrieval path so a lead can resolve a mention without dumping the stream.

Update canonical whiteboard guidance and examples for content versus panels
versus log, server-owned events, the removed recap/append forms, query
retrieval, and the settled editing rules. Teach leads to split a content-heavy panel into several (sub)panels rather than posting one huge panel, and to read entry-aware
`<wb:mention>` attribution without confusing it with authored `{{nn}}` or
optional panel-reference syntax. Update the actual inbox parser/serializer
and dashboard draft/restore paths as well as all playbook copies; old-shaped
mentions remain valid. Find other live recap consumers and
update only those affected by the removal. Regenerate the canonical manifest
and wsflow mirror, resync Pi, and verify host-neutral/product-mode rendering.
Record the follower handoff (latest agent line, no recap), without doing its
implementation.

Verification expectations:
- Query tests cover default output with zero/one/many entries, no implicit
  panel bodies (including topic/text targets), each selector and their
  combinations, time/ID boundaries, bad cursors/IDs, deterministic limits,
  kind filtering, recap-only/body-only matches, and omitted-entry counts.
- Retrieve `panel: "p7"` (latest version) with whitespace, XML-like text, HTML, and Markdown;
  returned payload matches stored bytes and can be quoted into the settled
  edit interface without formatting loss. Cover explicit current-content
  retrieval and item/answer-quote queries as settled in the queue.
- Resolve a delivered panel/log mention to its exact entry after restart,
  including identical quoted text in different entries, and return an
  explicit stale/missing locator result rather than silently choosing a
  different entry. Add mention old/new round-trip tests in `internal/wsinbox`
  and dashboard integration tests proving entry metadata is not dropped.
  Verify the settled compatibility tier against both strict legacy parsing
  and the updated lead guidance.
- Run focused `internal/wswhiteboard`, affected `internal/mcp`, and
  `internal/wsdashboard` tests, then `go test ./...` from
  `agents-plugin-tool/`. Run `internal/wsrsrc` manifest/mirror drift guards
  and `python3 -m unittest discover agents-plugin-wsflow/tests`; check Pi's
  version/mirror tests as well.
- Fresh-session smoke: convert an old board, add a panel and agent log line,
  answer an item, query metadata and one panel, render/refresh both page
  surfaces, revive, restart, and verify no new writes touched legacy board
  fields. Confirm the complete breaking contract is scheduled for 0.50.
