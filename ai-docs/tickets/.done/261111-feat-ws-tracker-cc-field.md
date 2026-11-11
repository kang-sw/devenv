---
title: "Tracker: a CC field so related members keep tracking an item without an assignment"
related:
  261111-feat-ws-tracker-assignee-edit-and-body-merge: "origin: its (235) a chose a CC field over relaxing assign; CC itself is open to every member (Decision 3), independent of that ticket's item.edit widening"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 9940211c702267ff
sage-review-design-reviewed: 9940211c702267ff
completed: 2026-11-11
---

# Tracker: CC field

## Background

User, 2026-11-11 board (topic `tracker-edit`, note on (229)): should assign
relax to the assignee, or should related people get a separate CC field so
the item's tracking is visible to them, or should assignments be tiered
("assignee | CC")? Answer (235) a; placement as its own ticket (236).

Current state (read 2026-11-11, `agents-plugin-tool/internal/wstracker/`):
assign is creator-only; in "chosen" mode members below the creator are live
at once and others pending (accept or decline). Body mentions notify once
and keep nothing in the mentioned member's view. There is no watcher or CC
concept.

## Decisions

1. **A CC item field; assign stays creator-only ((235) a).** CC is an item
   field listing members who keep tracking the item. CC'd members see the
   item in the CC section of the "related to me" scope (Decision 5) and get
   change notifications. A CC entry
   has no status, accept, or completion.
   Rejected: letting the live assignee assign ((235) b), since delegation
   already works through subtasks the assignee creates and assigns, each
   with its own status; an "assignee | CC" assignment tier ((235) c), which
   would have to define accept, decline, and done for a watcher; both a
   and b ((235) d).
2. **Its own ticket ((236)).** CC touches view lists and notifications and
   shares no code with the edit-permission and body-merge work.
   Rejected: a Phase 2 of `261111-feat-ws-tracker-assignee-edit-and-body-merge`.
3. **Anyone may CC anyone (user, (249) a with note).** Adding any store
   member to an item's CC is open to every member, not only to those who may
   edit the item; `item.assign` itself stays creator-only. Removing a CC is
   open to everyone too, symmetric with adding (technical, (279)): CC carries
   no duty, a wrong removal is harmless and re-addable, and restricting it
   would need a record of who added whom.
   Adding a member already CC'd is a no-op. Concurrent add and remove of
   the same member on one item fold by the tracker's per-key register
   winner rule (`winner()`, as other register fields do), pinned by a golden
   case (technical, design review). The item window carries the add and
   remove control beside the CC list (technical, completeness review).
   Rejected: CC edited only by those who may edit the item ((249) b); CC
   limited by the org tree ((249) c).
4. **CC'd members are notified of status and completion changes and
   comments only (user, (250) a).** Completion changes include a close, a
   confirm, a rejection, and, once
   `261110-feat-ws-tracker-superior-settable-status-and-new-statuses`
   lands, in-review and dropped. A CC'd member who is also the creator or an
   assignee gets one notification per event, not a second through CC
   (technical, completeness review). The notification here is the
   dashboard feed; the daemon hook's "mine" scope
   (`261110-feat-ws-tracker-daemon-event-hook`) stays as that ticket
   defines it, and widening it to CC is a follow-up (technical, design
   review). Rejected: also body edits and
   assignment changes ((250) b), which make the notifications noisy for a
   watcher.
5. **A "related to me" scope holds CC items, with mentions below (user,
   (307) a, (308) b with note; replaces the earlier collapsed MY-view CC
   section of (251)).** The dashboard tracker's scope chips (`mine`,
   `received`, `given`, `team`, `all`, `tracker.js` `SCOPES`, copied in
   `wslive/ui.go` `UIScopes`) gain a `related` scope
   beside `mine` (Korean "나와 관련"). It shows two sections: CC items on
   top, and items that mention the viewer below, separate from the CC rows
   (user note on (308)). Rows carry no CC mark; the item window lists the
   CC members. The MY view gets no CC section. Rejected: the collapsed CC
   section inside the MY view ((307) c), which keeps no-duty rows next to
   work; both places ((307) b); CC only ((308) a); CC, mentions, and every
   item the viewer commented on ((308) c), which approaches the full list.
   - The mentions section lists items whose shown body or a visible comment
     mentions the viewer, newest mention first; an item already in the CC
     section is not repeated there (technical, (309)). Rejected: one merged
     list, which the user's note on (308) keeps apart.
   - Both sections leave out items the viewer already sees as creator or
     assignee, and hidden items (technical, (310)). Rejected: a per-section
     rule, which lets a row show twice across scopes.
   - The lists are computed in the view fold (Go `view.go` and the reference
     engine): a `related` block with `cc` and `mentioned` item lists in the
     viewer's view, so golden parity pins them and `tracker.query` callers
     read the same lists (technical, (311)). Rejected: deriving them in
     `tracker.js`.
   - Both sections open expanded with a count on each header (technical,
     (312)). Rejected: collapsed sections, since the scope exists only to
     show these lists.
   - Details settled at design review (technical, autonomous): the CC
     section orders by the latest CC add, newest first. A body mention
     dates from the event that wrote the shown body carrying it, a comment
     mention from the comment. Closed items follow the existing scopes'
     closed handling rather than staying in either list, so the mentions
     list stays bounded. The scope is list-only in `SCOPE_MODES`, like
     `received`; its chip count is the two sections' total, and it has no
     today marker. `tracker.query`'s text `scope` gains `related`, rendered
     as the same two sections in `formatTrackerScope`, so agents read the
     lists without `format: json`. Rejected: JSON as the only agent path.

6. **A `cc:<member>` filter (technical, (252)).** `tracker.query` and the
   dashboard filter gain `cc:<member>`, `cc:me` included, shaped like the
   assignee filter. `cc:me` resolves to the viewer and takes precedence
   over a member whose id or name contains "me"; `assignee:` gains no `me`
   alias here. The query doc comment and the tracker playbook document it
   (technical, design review). Rejected: dashboard-only visibility, which leaves agents
   no query path.
7. **Mirroring and one phase (announced, (253)).** CC events are mirrored in
   the reference engine and its golden cases and documented in the tracker
   playbook of all three packages, as existing event types are ((210)).

## Constraints

- Mirrored in the reference engine (`ai-docs/ref/tracker-mockup/engine.js`)
  and its golden cases, and documented in the tracker playbook of all three
  packages, as the existing event types are ((210)).
- Quiet dashboard tone: `--wb-*` tokens.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- Golden files: view JSON fields carry no `omitempty`, so the item view's `cc`
  field and the viewer view's `related` block change every golden; regenerate them from the reference engine after
  each merge, never merge them by hand, and give new case files a distinct
  name prefix. This applies whichever of this ticket and
  `261110-feat-ws-tracker-superior-settable-status-and-new-statuses` lands
  second.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/internal/wstracker/)

## Prior Decisions

- 261111-feat-ws-tracker-assignee-edit-and-body-merge (2026-11-11, Decisions): "`item.edit` opens to the item's creator or a member holding a live assignment on it, for every field, body included ((229) a). `item.hide` and `item.assign` stay creator-only." — bearing: constrains
- 3e3cded6 (2026-11-11, commit): "CC ticket keeps visibility, notifications, and MY view placement as open questions; no decision was taken on them." — bearing: superseded by Decisions 3-6, which settle those questions
- 261110-feat-ws-tracker-self-assign-claim (2026-11-10, Decisions): "Only the creator opens (announced, (255)). Opening delegates like assigning, and `item.assign` stays creator-only (... (229) a, (235) a)." — bearing: supports
- 261111-feat-ws-tracker-store-settings-and-labels (2026-11-11, Decisions): "Labels are an item field like `project` ... set in the item window beside `project`, by anyone who may edit that item ... The label write uses the same permission check as `item.edit`" — bearing: constrains
- 78af82e6 (2026-11-08, commit): "Notification mapping uses event ids already in the view state: assign / pending -> `assignment.via`; ... status / close / confirm -> the assignee's latest status event; reply / mention -> the comment id" — bearing: constrains
- e11589f5 (2026-11-08, commit): "Handoff is a separate `item.handoff` event rather than `item.assign` by an assignee, so the creator-only rule on item.assign stays simple." — bearing: supports
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Decisions): "The creator fallback is derived in the fold, not written as an assign event ((555))." — bearing: constrains
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, f61a8b2a): "The pi rsrc mirror (TestPiMirrorUpToDate) was synced by copying the two changed rsrc files, which is what bump-ws-version.sh's sync_tree does" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/ check.go view.go query.go, ai-docs/ref/tracker-mockup/engine.js, wstracker/testdata/gen-golden.js, internal/mcp tracker tools, wsdashboard/assets/tracker.js, wslive/ui.go UIScopes, mcp/tracker_tools.go trackerScopes and mcp/tracker_render.go formatTrackerScope, rsrc/tracker/tracker.md in agents-plugin, agents-plugin-wsflow, agents-plugin-pi |
| scope.surface | public-interface | tracker.update grammar and tracker.query cc: filter are MCP-visible; ledger event types are a store contract |
| scope.new_public_symbol | yes | new CC add and remove item events and the cc:<member> query key in wstracker/query.go |
| scope.new_type_contract | yes | CC event payload and a CC list on the item view and a related block (CC and mentioned lists) in the viewer view; none exists today, grep cc or watcher in wstracker and engine.js returned nothing |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/query_test.go, golden_test.go with testdata/golden, internal/mcp/tracker_tools_test.go |
| complexity.reuse_points | confirmed | assignee filter in wstracker/query.go L14-L22 and L155 L246; mention notification feed in wsdashboard/assets/tracker.js L794-L798; creator check pattern in check.go L486-L487 |
| complexity.side_effect_risk | moderate | new event type folds in Go and engine.js and must stay golden-equal; notification feed and the new related scope change for every member |
| risk.correctness | moderate | notification selection must include status, close, comment and exclude body edit and assignment change; the open-to-all permission must not leak into item.edit or item.assign checks |
| risk.fit | moderate | spans domain, reference engine, MCP grammar, dashboard UI, and three mirrored playbooks |
| risk.test | moderate | needs Go tests, regenerated golden cases agreeing with engine.js, query tests, and a browser check |
| risk.security_or_contract | moderate | new ledger event type and MCP grammar are persistent contracts; permission is deliberately open to every member |

## Phases

### Phase 1: CC field

Scope:
- The CC item field and its add and remove events open to every member
  (Decision 3), in the Go domain, the reference engine
  (`ai-docs/ref/tracker-mockup/engine.js`) with its golden cases, the
  `tracker.update` grammar, and the playbook in all three packages.
- Notifications for CC'd members (Decision 4), the "related to me" scope
  with its CC and mentions sections and the item window's CC list
  (Decision 5), and the `cc:` filter in
  `tracker.query` and the dashboard (Decision 6).

Verification:
- Tracker tests: any member adds and removes any member's CC, including on
  an item they cannot edit; a CC'd member sees the item in the CC list and
  is notified of a status change, a close, and a comment, but not of a body
  edit or an assignment change, and of a confirm; CC grants no status,
  accept, or completion; a repeated add is a no-op; a concurrent add and
  remove folds by the register winner; a creator or assignee who is also
  CC'd gets no duplicate row or notification; a hidden item leaves the CC
  section; a body or comment mention puts the item in the mentions section,
  and an item both CC'd and mentioning the viewer shows only under CC; the
  mentions section leaves out items the viewer created or is assigned, and
  hidden items, and orders newest mention first; the CC section orders by
  the latest CC add; a closed item leaves both lists.
- `tracker.query` with scope `related` renders the CC and mentions sections,
  and its JSON view carries the `related` block.
- Golden cases from the reference engine agree with the Go domain.
- `cc:<member>` and `cc:me` query tests, including a member whose name
  contains "me"; a browser check of the "related to me" scope chip with its
  CC section above the mentions section, and the item window's list with
  its add and remove control.
- The three tracker playbook copies stay in parity, and `ko.json` holds the
  new UI keys.

### Result (bcdd5eff9) - 2026-11-11

Commits 3573e7493 (domain + engine + goldens), 035a91682 (MCP grammar,
`related` scope, `cc:` search, playbooks), 10f1e25ab and 4c39b8c14
(dashboard, i18n, mockup fixtures), and the develop merge bcdd5eff9.

- Events `item.cc` / `item.uncc` with payload `{item, member}`, one member
  per event, open to every member on a non-hidden item; `cc` needs a live
  member, `uncc` also takes a removed one. A repeated add and a remove of a
  member who is not CC'd apply as no-ops. One register per item and member,
  so a concurrent add and remove fold by the register winner (golden case
  `cc-concurrent`).
- View: `items[id].cc` (live members, CC order), `my.related`
  `{cc, mentioned, closed}` and `my.ccNotices` (a separate list, outside
  `notices` and the badge). `closed` carries the done or dropped items
  either section would hold, for the dashboard's recently-closed handling.
- CC notices cover item.status / close / confirm / reject / reopen and
  comment.add written after the viewer's CC, and leave out the viewer's own
  events, items the viewer created or holds, and comments that mention the
  viewer or reply to the viewer's comment (already in the feed).
- `tracker.query` scope `related` renders the CC then the mentions section;
  the item view prints a `cc:` line; `cc:<member>` / `cc:me` in the shared
  search grammar (`cc:me` is the viewer first).
- Dashboard: a list-only "related to me" chip after `mine` (count = both
  sections), CC above Mentions, both open with counts; the item window's CC
  list with add (member picker) and remove; CC feed entries keyed
  `cc:<event>`; event sentences; 22 new keys in all 11 i18n tables.
- After merging develop: an org superior's item.status names its assignee
  in a CC notice's `to`, a dropped item moves to `closed` like a done one,
  and a body mention dates from the body write the item's mentions come
  from (three-way body merge); golden case `cc-wrapped`.
- Verification: ws-rgo test of wstracker, wstrackerhook, mcp, wsdashboard,
  wsi18n, wslive, wsrsrc all ok, vet clean; gen-golden.js: 284 reasons
  cases agree; headless Chromium on the mockup confirmed the related chip,
  CC above Mentions, and the item window's CC add/remove (pre-merge build).

Forward:
- The daemon hook's "mine" scope does not count CC (Decision 4 follow-up).
- Involvement in `related` / CC notices is judged by the viewer's current
  records, not as of each event: a CC'd member whose assignment later ended
  sees CC notices for events from while they held it.
- engine.js orders mention / CC dates by ts string compare, which misorders
  mixed-precision RFC3339Nano strings; Go compares times. Goldens use uniform
  precision, so parity cannot catch it (reference engine only).
