---
title: "Dashboard: shared toast framework, first used by the tracker notification layer, and tab-label alerts"
related:
  261110-bug-ws-dashboard-render-and-frontend-load-accumulation: "its Decision 10 keeps the main stream open while the tab is hidden so tracker events still arrive; this ticket builds the alert surface on that"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 5d43de5bce4f1a66
sage-review-design-reviewed: 5d43de5bce4f1a66
---

# Dashboard toast framework and tab-label alerts

## Background

User, 2026-11-10 whiteboard: "닫혀있는 탭에서도 예를 들어 알람이라던지
날아오면 사실 라벨에 표시되는 게 더 좋지 않나요? 아니면 전반적으로
프론트엔드에 Toast 프레임워크를 만들고, 첫 사용자로 store 쪽의 알림 레이어로
쓰면 어떨까" - an alert should show in the tab label even while the tab is
in the background; and a general frontend toast framework whose first user
is the tracker store's notification layer. Board (589) = a: a separate
feature ticket, not part of the performance ticket.

Current state (develop 88326de57, code reading):

- Nothing sets the favicon. `document.title` already carries a `(N) `
  prefix on the session page: the whiteboard's open-question pill script
  (`agents-plugin-tool/internal/wswhiteboard/assets/pill.js#L36-L43`,
  inlined into the dashboard through `window.wbForm`, `dashboard.tmpl` and
  `dashboard.js#L407-L408`) sets it to the count of board policy cards and
  ack rows that are neither drafted nor marked sent/held, and re-runs on
  every board swap, form input, and sent-mark class change
  (`pill.js#L128-L139`); added by 44319923.
- Two independent single-message toasts exist: `ltoast` in
  `wsdashboard/assets/dashboard.js` (copy and open results) and `tktoast` in
  `wsdashboard/assets/tracker.js` (save, register, and upload results). Each
  shows one message and hides it after a timer.
- Tracker notifications are not carried by stream events: the main
  stream's `tracker` event carries only `{rev}` and makes the page reload
  the store views (`dashboard.js#L3700-L3704`, `trackerNudge` at
  `dashboard.js#L3136-L3142`); the notification entries are then derived
  client-side by `feedEntries(v)` in `tracker.js#L702-L747` for the store
  being rendered (`VS`, set from `ui.store` at `tracker.js#L3744-L3745`),
  with read state from `readMap()` (`tracker.js#L749`). They show only in
  the Notify tab (`cardNotify`, `tracker.js#L755-L762`) and the per-item
  unread dots (the `UNREAD` map, `tracker.js#L586-L595`,
  `tracker.js#L3765-L3769`).
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation Decision 10
  (board (588)) keeps the main stream open while the tab is hidden and still
  handles `tracker` events then, which is what lets a hidden tab learn of
  new notifications.

## Decisions

1. **One shared toast framework replaces both toasts** (user, 2026-11-10).
   `ltoast` and `tktoast` call sites move onto it; neither old element stays
   as a second toast surface. Its first new consumer is the tracker store
   notification layer.
2. **Tab-label count** (board (592) c): the count is the sum of
   - tracker unread notifications,
   - whiteboard asks waiting on the user (the set Decision 10 defines), and
   - pending permission prompts the page shows (Decision 12).
   Rejected: tracker unread only (a); tracker unread plus board asks without
   permission prompts (b). Reason: what a hidden tab most often makes the
   user miss is work waiting on them.
3. **Label form** (board (593) a): a `(N) ` prefix on `document.title`, for
   example `(3) ws dashboard`; no prefix when N is 0. This replaces the
   board-only prefix `pill.js` writes today on dashboard pages (see
   Decision 9). Rejected: a favicon
   badge (b) and both (c); the title prefix works in every browser and a
   favicon badge can be added later.
4. **When the count drops** (board (594) b): only when an item is actually
   handled: a notification is read, a board ask is answered, a permission
   prompt is resolved. The count is recomputed from those three sources, not
   decremented by focus. Rejected: reset to 0 on focus (a), because the
   number must mean work still left.
5. **Events that arrive while the tab is hidden** (board (595) b): no
   per-event toasts while hidden; the label carries the count. When the tab
   becomes visible again and new tracker notifications arrived while hidden,
   show one summary toast ("N new notifications"); clicking it opens the
   Notify tab. Rejected: no toast at all for hidden-time events (a);
   replaying each event as its own toast on return (c), which floods.
6. **Toast behavior** (board (596) ok):
   - at most 3 toasts stacked, newest on top; a fourth drops the oldest;
   - informational toasts (the former ltoast / tktoast messages) disappear
     after 4 s;
   - a tracker notification toast is clickable, opens the notified item or
     comment, and stays 8 s;
   - hovering a toast pauses its timer; leaving resumes it.
   Rejected: an unbounded stack, which covers the page.
7. **OS notifications out of scope** (board (597) a): no browser
   `Notification` API in this ticket; tab label and in-page toasts only.
   Rejected: opt-in OS notifications (b), which add a permission flow and
   per-OS differences; a later ticket can add them after the label and
   toasts are in use.
8. **Visual style** (user approved mockup v2 on the board, 2026-11-10):
   - position: bottom-right stack;
   - tone: inverted, like the current `ltoast`: `--wb-fg` background,
     `--wb-card` text, 8px radius, `--wb-shadow-hi` shadow, 12.5px text;
   - the summary toast carries a small dot in `--wb-primary`;
   - a tracker notification toast carries a 3px left bar in `--wb-policy`;
   - existing `--wb-*` tokens only; light and dark themes both work.
9. **One title writer on the dashboard** (fact-population gap, lead
   technical call): on a dashboard page the tab-label module owns
   `document.title`; `pill.js` reports its board count to it (a hook the
   dashboard provides) instead of writing the title there. Because
   `window.wbForm` (and so `pill.js`) is defined before `dashboard.js` loads,
   a dashboard page carries an explicit marker that `pill.js` checks before
   any title write, so no early run writes the title; the `data-wb-title`
   base-title handling moves to the dashboard's title owner. On the
   standalone whiteboard page, where no dashboard runs, `pill.js` keeps
   writing its own `(N) ` prefix unchanged. Rejected: two writers where the last one wins,
   which makes the number flicker between two meanings.
10. **Board-ask set** (fact-population gap, lead technical call): each page
    counts what it shows. A session page uses the `pill.js` set (policy
    cards and ack rows with no draft and no sent/held mark), the one its
    pill and current title already use; the index page, which renders no
    board, sums the per-session `CountPolicy` chips it already renders.
    Rejected: a new server-side count, which the frontend-only scope rules
    out for no gain the user asked for.
11. **Counts while hidden** (fact-population gap, lead technical call;
    consequence of Decision 2's reason): region swaps stay deferred while
    the tab is hidden (the render-accumulation ticket's Decision 10), but a
    held region that carries board asks or permission prompts is parsed
    detached (no swap, no DOM insertion), at most once per held update and
    only when that region's hash changed, so the label keeps moving while
    hidden. The detached board count applies the same rule as Decision 10,
    not just the selectors: held board item ids, minus ids that have a live
    draft in the current board DOM, minus ids in the newest `.sentdata`
    records (from the held convlive region when one is pending, else the
    live one), so drafted, sent, or held items do not raise the count while
    hidden and drop it again on return (Decision 4). Rejected: a frozen board and
    permission count while hidden, which defeats the label's main purpose
    (work waiting on the user arriving while away); counting only tracker
    while hidden, for the same reason.
12. **Permission prompts are counted, not just detected** (fact-population
    gap, lead technical call): the selectors `drawerWait` already uses
    (`#r-convlive .permwait`, `#r-tree .phase.permission`,
    `#r-tree .brow.perm`) are counted one per `.agent` element in `#r-tree`
    that matches either tree selector, with the convlive `.permwait` folded
    into the lead (root) agent, so one prompt shown in two places counts
    once; on the index page, the `.sess .phase.permission` spans.
13. **Tracker unread scope** (fact-population gap, lead technical call): the
    rendered store only (the one the Notify tab shows), via a new
    `WsTracker` export of its unread notification entries (`store`, `item`,
    `ev`, `key`). New arrivals are detected against a separate per-store
    baseline of unread keys, set silently at the first render of each store
    (page load, background-opened tab, or a switch of the rendered store) and
    diffed on every render (`X.rendered` runs even with the drawer closed);
    only keys new against that baseline toast or count toward the
    hidden-time summary. Rejected: reusing the notify tab dot's diff
    (`tabDot`), which treats every unread entry as new on first sight and
    fires only while the drawer is open, so it would toast old backlog on
    load and stay silent in the usual closed-drawer state; all stores, which
    needs view loads for stores the page does not render.
    This holds on every page that mounts the tracker (the index page loads
    `tracker.js` too); where no store is rendered the tracker term is 0.
    The summary string ("N new notifications") follows the existing i18n
    tables' count-string convention in every locale.
    A tracker notification toast shows text only: the entry's kind plus the
    item title (and author when known), built as text, never the entry's
    `html` (Decision 14 allows HTML only for the former `tktoast` call
    sites).
14. **Toast module home** (fact-population gap, lead technical call): a new
    embedded asset `internal/wsdashboard/assets/toast.js` exposing
    `window.WsToast`, inlined by both dashboard templates (session page and
    index page) before `TrackerScript` and `Script`. Dashboard assets are
    inlined through `pageAssetsData`, so this means: the `//go:embed` list,
    a new `ToastScript` field and its `get("toast.js")` line in
    `buildDashboardAssets` (`pages.go`), and a script tag in both templates;
    the dev-assets path picks it up automatically. `tracker.js`
    calls `window.WsToast` when present and keeps a minimal fallback for the
    tracker mockup page (`ai-docs/ref/tracker-mockup/`), which loads
    `tracker.js` without the dashboard. The module takes text content by
    default and an explicit HTML option only for the existing escaped
    `tktoast` call sites, so no call site switches escaping silently.
    Rejected: placing it inside `dashboard.js` or `tracker.js`, since each
    loads without the other on some page.

## Constraints

- Frontend only: `internal/wsdashboard/assets/` (`dashboard.js`,
  `tracker.js`, `dashboard.css`, `tracker.css`, `dashboard.tmpl`, new
  `toast.js`), `internal/wswhiteboard/assets/pill.js` (Decision 9), the
  `pages.go` embed / asset-field lines that serve `toast.js` (Decision 14),
  and i18n
  tables. No new server event or route; the count comes from state the page
  already holds or receives.
- Out of scope: the browser `Notification` API (Decision 7); a favicon badge
  (Decision 3).
- Keep stream and render work cheap: 261110-bug-ws-dashboard-render-and-frontend-load-accumulation
  just removed per-update DOM, listener, and observer growth. Toast elements
  are removed when they leave; title updates happen only when N changes; no
  per-event full-page re-scan.
- Never run `go build`, `go test`, or `go vet` locally on this machine (it
  freezes the user's desktop). Run Go tests remotely with
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/wsi18n/ -count=1 -timeout 30m`.
  Local checks are limited to `nice -n 19 node --check` and headless node
  runs.
- Never touch processes or ports 29417 / 45743 (the user's dashboard
  daemons, including Windows PID 40472). A visual check uses an isolated dev
  daemon with its own WS_CACHE_HOME / WS_CONFIG_HOME and port.
- `agents-plugin-tool/` is a shipped surface:
  `ai-docs/manuals/shipped-surface-boundary.md` applies. New user-visible
  strings go through the existing i18n tables (`internal/wsi18n/tables/`),
  all locales, as other dashboard strings do.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 44319923 (2026-11-07, commit): "User request via board (75)-(78): floating pill for unanswered items scrolled out of view plus (N) tab-title prefix, on disk board, served board and session #bcol." — bearing: contradiction-candidate
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Decisions): "Frontend: a hidden tab keeps its streams and defers heavy region swaps. While document.hidden, the main EventSource stays open; a region event ... records only its latest HTML and hash per region and applies nothing" — bearing: constrains
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Decisions): "The index page's per-session board counts come from the whiteboard store without rendering the board. The index uses only HasBoard and CountPolicy" — bearing: constrains
- 8b7ff022 (2026-11-10, commit): "User 2026-11-10 board (589)=a: shared toast framework (first consumer the tracker notification layer) plus tab-label alerts as a separate feature ticket" — bearing: supports
- 8af340da (2026-11-08, commit): "Tab dots follow the dashboard tdot idea (content moved while hidden), per tab signature: unread keys not present when last shown (removals such as reading or answering are not news...)" — bearing: supports
- 025cf028 (2026-11-10, commit): "The per-store notify fires for every view but the last: the last view completes tkReadAll and tkReload's done path already notifies" — bearing: constrains
- 9eff2c86 (2026-11-08, commit): "Mockup-only chrome ... is not in the renderer; it reaches in through ext (tabs with after, syncTools, teamTools, qsActions, dialogs, action, rendered) and the h helper bag." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, tracker.js, dashboard.css, tracker.css, dashboard.tmpl, agents-plugin-tool/internal/wsi18n/tables/*.json; the existing title writer is agents-plugin-tool/internal/wswhiteboard/assets/pill.js |
| scope.surface | cross-module | window.WsTracker mount api and the h helper bag both expose toast, tracker.js#L4564-L4581; the ai-docs/ref/tracker-mockup page loads tracker.js without dashboard.js and calls h.toast, mock-adapter.js; pill.js in wswhiteboard also writes document.title |
| scope.new_public_symbol | yes | a shared toast module plus a WsTracker export of per-store unread notification entries, which the mount api lacks today, tracker.js#L4570-L4581 |
| scope.new_type_contract | yes | toast call signature with kind, duration, click target; unread-entry shape store, item, ev, key for the tab-label count and click-to-open |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pages_test.go, tracker_*_test.go asset-extract tests, i18n_test.go |
| complexity.reuse_points | confirmed | openItemWindow store, item, focus ev at tracker.js#L3891 and tracker.js#L3910; notify new-key diff in tabSig and tabDot at tracker.js#L1985-L2005; permission selectors in drawerWait at dashboard.js#L1698-L1702; pill.js retitle at pill.js#L36-L43 |
| complexity.side_effect_risk | moderate | about 40 tktoast call sites pass escaped HTML into innerHTML while ltoast uses textContent, and the mockup page and pill.js depend on the surfaces being replaced |
| risk.correctness | moderate | region swaps are deferred while hidden, so DOM-derived board and permission counts freeze in a hidden tab; feedEntries covers only the rendered store; pill.js is a second document.title writer |
| risk.fit | moderate | the shared module must be reachable from tracker.js when it runs standalone in the mockup, and its home among the embedded assets is not named |
| risk.test | moderate | timer, hover, stack, and hidden-visible behavior have no JS test runner and rest on a headless check |
| risk.security_or_contract | moderate | notification entries carry HTML built from user comment snippets; mixing text and HTML toast content risks an injection if escaping is dropped |

## Phases

### Phase 1: Toast framework, tab-label count, tracker notification toasts

- Add the shared toast module (Decisions 1, 6, 8, 14) and move every
  `ltoast` and `tktoast` call onto it as informational toasts.
- Compute the tab-label count from the three sources (Decisions 2, 10, 12,
  13), set the `document.title` prefix as its one writer (Decisions 3, 9),
  recompute it when any source changes (Decision 4), and keep it current
  while hidden (Decision 11).
- Tracker notifications arriving while the tab is visible show a tracker
  toast that opens the item or comment on click (Decision 6); those arriving
  while hidden are counted and produce one summary toast on return that
  opens the Notify tab (Decision 5).

Verification:

- `nice -n 19 node --check` on every touched JS file.
- The standalone whiteboard page still gets its own `(N) ` title from
  `pill.js` (Decision 9); the tracker mockup page still toasts through the
  fallback (Decision 14).
- Go tests remotely for `./internal/wsdashboard/`, `./internal/wswhiteboard/`,
  and `./internal/wsi18n/`
  (asset embedding, template, and i18n table tests); add a Go-side test
  where a template hook or i18n key is added, following the existing asset
  tests.
- A headless check (isolated dev daemon or a node harness over the module)
  shows: the stack caps at 3 with newest on top; info toasts leave after
  4 s and tracker toasts after 8 s; hover pauses; the title prefix appears,
  updates, and clears at 0; a hidden-then-visible cycle with new
  notifications yields exactly one summary toast.

### Result (49a054f71) - 2026-11-10

Landed:

- `internal/wsdashboard/assets/toast.js` (`window.WsToast.show(content,
  {html, kind: info|notice|summary, duration, onClick})`), inlined by both
  dashboard templates through `pages.go` `ToastScript`, ahead of the tracker
  and page scripts. Styles in `dashboard.css` (`.wstoasts`, `.wstoast`,
  notice bar, summary dot); `.ltoast` is gone.
- `dashboard.js` link/copy toasts call `WsToast` as text. The tracker's
  `toast(html)` forwards to `WsToast` with `{html: true}`; `#tktoast` is
  built only on a page without `WsToast` (the tracker mockup), so its ~35
  call sites keep their escaping unchanged.
- `dashboard.js` tab-label block (`tl*`): the one `document.title` writer,
  `(N) <base>`, N = tracker unread + board asks + permission prompts, each
  source cached where it changes; written only when N changes. Held region
  updates of counted regions (session: board, convlive, tree; index:
  sessions) are parsed detached via `frag()` once per new hash while hidden.
- `pill.js`: on a `data-wb-titlehost` page it reports to
  `window.wbTitleCount` and never writes the title; standalone unchanged.
- Tracker API: `opts.onRender(u)`, `api.unread()` (`{store, ready,
  entries[{store, item, ev, key, kind, label, title, author}]}`, plain
  text), `api.openNotice(entry)`, `api.openNotify(store)`.
- i18n `js.toast.newnotes.one/.other` in all 11 locales.

Verification: `nice -n 19 node --check` on toast.js, tracker.js,
dashboard.js, pill.js: ok. `ws-rgo ... test ./internal/wsdashboard/
./internal/wswhiteboard/ ./internal/wsi18n/ -count=1`: all ok; the new
node-backed tests `TestToastStack`, `TestTabLabelCount`,
`TestPagesCarryTheToastStackAndTitleHost`, `TestPillTitleDefersToTheDashboard`
ran (PASS, not skipped). These node harnesses are the headless check (stack
cap 3 / newest on top, 4 s / 8 s, hover pause, prefix appears / updates /
clears at 0, one summary toast after a hidden cycle). Tracker fallback toast
checked by a local node run over the extracted function. Lite review: no
Critical or Important; one Minor fixed in dfcc1ff27 (eager `#tktoast` on
fallback pages for the mockup's error path).

Decisions:

- Permission units are keyed by `closest(".agent, .peer")`, so a peer
  row's prompt counts as its own unit, not folded into the lead's element it
  is nested in.
- Tracker baselines are per store for the page's lifetime: switching back
  to a store already rendered toasts keys that arrived meanwhile; a store's
  first loaded render (and any render before it loads) is silent.
- The summary toast counts hidden-time arrivals still unread on return.
- No isolated dev-daemon visual check was run; the node harnesses stand in
  for it.
