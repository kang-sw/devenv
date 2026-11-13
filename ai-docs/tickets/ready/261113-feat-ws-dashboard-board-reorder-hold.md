---
title: "Dashboard board: hold a topic the user is reading in place when the server reorders it"
related:
  261113-feat-ws-whiteboard-topic-history-load-more: "reuses this ticket's Phase 1 visibility and keep-in-place helpers (decision 8); this ticket is its prerequisite"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "co-edits topic markup and board motion assets"
  261113-feat-ws-dashboard-iframe-shell: "moves session pages into a shell document; the board scroller stays in the session page; its hidden-frame message feeds pageHidden() (decision 9)"
  261113-feat-ws-whiteboard-board-frontend-performance: "its Phase 2 consumes the decision 8 helper contract (watch with wider margins, seen)"
  261114-feat-ws-dashboard-board-toolbar-overview: "its overview mode defers hold releases (its Decision 13); the badge's (c) triggers honour that gate, and the badge is hidden in overview"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 705b92d96f93c00a
sage-review-design-reviewed: 705b92d96f93c00a
---

# Dashboard board: hold a topic the user is reading in place when the server reorders it

## Background

Owner feedback on board jumps (2026-11-13); MAIN's draft (board topic
`board-reorder-hold`); UX critique and spec (UX lead, read against develop
14aabe4b5); owner answers on MAIN's board: (515)=b, (516)=ok, (517)=ok,
(518)=ok, (525)=b (MAIN board IDs; (525) here is MAIN's indicator item, not
the EXEC release-gate item renumbered to (526)).

The server orders open topics by last update (`wswhiteboard/view.go#L690-L699`:
sort by `UpdatedMs`, then activity, then bump rank), so an update moves its topic to the head of the board, and a close moves it
to the closed section. `swap()` (dashboard.js) morphs the board
(`morphBoard` -> `WsCards.weave`/`settle`) and plays a 200 ms grow + FLIP
slide. Then `restoreScroll()` writes back the old absolute `scrollTop`. So:

- a topic the user is reading jumps away from under them, and
- a topic rising from above or below the viewport past it shifts every
  visible topic by its height, because nothing anchors the view.

Approved visual reference: mockup v2, archived at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/reorder-hold-mockup-v2.html`.
Approved badge reference (owner (572)=ok, (590)=ok, 2026-11-14): back-to-top
badge mockup v2, archived at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/board-totop-badge-mockup-v2.html`.

## Decisions

1. **Hold = a client placement override inside cards.js.** A held topic is
   placed by its pre-update time (`hold.at` = the `data-at` it had before the
   update) in `order()`, the same way a hand-moved topic (`topicRec.at/.bar`)
   is placed. It is not a pass after `morphBoard`. So `settleMoves`,
   `frameGrafts`, `boardSpots`, `slideBoard` and the cards (fixed nodes,
   flow-placed by `at`) all see the held order with no double move. Hold
   records live in their own map: they are not `topicRec` fields and are
   never written by `persist()`. A reload drops them.
2. **Hold starts** on a board swap when, for topic T:
   - T is in the board column (`#bcol`), not pinned. Pinned topics never
     move (`boardSpots` paged) and never hold.
   - The swap moves T: an open T moves to an earlier slot (it rises), or a
     visible T becomes closed (515=b).
   - T is in view: at least `min(48px, T's height)` intersects the viewport.
     This test reads rects at swap time, in the same pre-morph layout read
     as `boardSpots` (`dashboard.js#L508`). A still or reduced-motion swap
     skips `boardSpots`, so there the test reads the rects itself. The
     IntersectionObserver (decision 8) is not consulted here: it reports
     asynchronously and gives a ratio, not a pixel overlap. It stays the
     single visibility source for release (decision 5) and for the
     topic-history collapse. Rejected: deriving the start test from the
     observer's last report, which can be one frame stale at swap time.
   - For a rise: T's destination slot is not in view. The destination is
     T's computed slot in the server order after weave: below the pin list
     when it stands atop the board, and below any card that `order()`
     places above T. It is not the region top. If the destination is in
     view, T moves now with the existing slide and does not hold.
     The destination's position is read from the next NON-moving node
     after T's slot in the new order. When another topic rises in the same
     swap, the node at T's index is itself moving, so it gives the wrong
     answer (found by MAIN's mockup v2 headless run).
   - The page counts as viewing the board: the board column is shown
     (`visible(bcol)`) and no card is maximized. Otherwise nothing holds,
     and every move applies at once with compensation (decision 4).
3. **A re-update of a held topic keeps the hold.** `hold.at` keeps the
   original pre-update time (T does not creep). The topic's content updates
   in place. A held-closing topic that reopens drops its close hold and is
   evaluated again as a rise candidate. A held topic the board no longer
   serves (deleted, or past the closed-topic cap) drops its hold. A hand
   move lapses on the topic's own update today (`claim`). A hand-moved topic
   that updates while in view becomes a hold candidate, with its hand place
   as its old place.
4. **Scroll compensation on every board swap and every observer-driven hold
   release ((a), (b)), continuous until layout settles ((516)=ok).** Release
   (c) is the exception (decision 5).
   - Anchor: the topmost visible held topic, else the topmost visible topic
     or card the swap does not move. Never a moving node. Its offset from
     the scroller's top is measured before the swap, in layout terms (rect
     minus any running transform). `boardSpots` reads mid-transform rects on
     purpose for FLIP, so it is the wrong input here.
   - After the swap, and on every layout change of the board's sections
     (a ResizeObserver session), `scrollTop` is adjusted so the anchor keeps
     its offset. Sources: `growTopic`'s 200 ms height animation, body frames
     reporting their height late (`wbGrow`, `frameReveal` up to
     `frameRevealMs` = 1.5 s, plus load time), and restored disclosures and
     history pages (`wbHistory.restore`, `dashboard.js#L554-L555`).
   - The session ends when no layout change has come for 500 ms (hard cap
     5 s), or at the first user scroll. A user scroll is a `scrollTop` that
     differs from the last one the session wrote.
   - **The scroller is the board's nearest scroller, not always `#bcol`.**
     `.bcol` scrolls (`overflow: auto`) only on Adapter pages (mode `live`:
     `dashboard.css#L763`, with the split and wide overrides at
     `#L780-L799`). A session page without an adapter has no `#tabbar`
     (`dashboard.tmpl#L16`), renders `.dgrid.board` as a plain block
     (`dashboard.css#L886`), and scrolls the document. A pinned topic in the
     pinned column scrolls in `.bpcol > .bpins` (`#bpins`,
     `dashboard.css#L1494`). The helper resolves the anchor node's nearest
     scroller: `#bcol` when it scrolls, `#bpins` for a node in the pinned
     column, else `document.scrollingElement`. Board-swap compensation
     anchors in the board column, so it runs on `#bcol` or the document.
     Pins never move ((447)), so a board swap leaves `#bpins` to
     `restoreScroll` unless another keep session owns it (for example a
     topic-history load-more in a pinned topic).
   - Whichever scroller a session uses takes `overflow-anchor: none` (`#bcol`
     and `#bpins` in CSS; for the document scroller the helper sets it on
     the root element while a session runs and clears it at session end,
     so the standalone whiteboard page and the rest of the document keep
     native anchoring outside sessions), so Chromium's native
     anchoring does not compensate a second time. `restoreScroll()`
     (`dashboard.js#L182`, called at `#L553`, fed by `scrollState()` at
     `#L176-L181`) skips whichever scroller a keep session owns at that
     moment. That can be `#bcol`, `#bpins`, or `document.scrollingElement`.
     Every other pane keeps its absolute restore. Rejected: hard-coding
     `#bcol`. The document-scrolled board would keep the absolute restore
     and fight the session.
   - **One session per scroller.** A keep call on a scroller that already
     has a running session does not start a second session. That covers a
     swap, a hold release, and a topic-history load-more or
     collapse. It re-anchors the running session instead: the call's node
     and offset, measured at call time in layout terms, replace the anchor.
     The call's observed nodes join the session's ResizeObserver. The
     500 ms quiescence window and the 5 s cap restart from that call. Only
     the session writes `scrollTop` on its scroller, so a write from outside
     every session is the only thing that counts as a user scroll.
     Rejected: one session per call, where two sessions on one scroller
     would each take the other's writes for a user scroll and cancel, or
     would compensate the same change twice.
   - Decided: manual compensation over native `overflow-anchor`, because
     WebKit support is uncertain and the late frame heights need the same
     path anyway.
5. **Release** (517, batched):
   - (a) T is fully out of view (the IntersectionObserver ratio is 0). The
     move happens out of sight, so a frame reload there is unseen.
   - (b) The destination coming into view releases nothing while T is in
     view. It only counts once T is off-screen, so it is (a).
   - (c) The back-to-top badge is clicked, or the user scrolls the board
     back to its top by hand (decision 7): every hold releases now in one
     settle with the slide. Both triggers start no compensation session:
     at the top nothing above the viewport can grow. Both are gated by the
     same release-deferral predicate as (a) and (b) under `pageHidden()`
     and by the one that `261114-feat-ws-dashboard-board-toolbar-overview`
     Decision 13 hooks in for overview, so neither releases while overview
     is shown. On browsers without `Element.moveBefore`, a
     released T's body frame reloads; this is accepted for an explicit
     click. T's frame height is restored through the `setFrameHeights` path
     so the reload does not collapse it.
   - The click's scroll target is the board top (revised with decision 7).
     The click ends any running keep session first, then releases, then
     scrolls the scroller to `top: 0` (smooth; instant under reduced
     motion). At `top: 0` nothing above the viewport can grow, so a released
     frame's late height change needs no compensation and no keep session
     is started. Until the to-top scroll lands, board keep sessions are
     suppressed: a late frame height on a `wbKeep` frame
     (`boardKept() || boardKeep(b)`, `dashboard.js:3454`) and a swap
     arriving mid-scroll (`boardKeep(el)`, `dashboard.js:534`) start none,
     since a compensating `scrollTop` write cancels a running smooth scroll.
     When the scroll ends (or after a guard of about 1.5 s, as the mockup's
     `back()` does), `scrollTop` is set to 0 if it is not there. Rejected: the pre-revision
     `wbHistory.keep(topmost T, { offset: <just below the pill> })` target,
     which the v2 badge replaced with "go to the top".
   - (a) and (b) need observer reports from a rendered page. While the page
     counts as hidden (decision 9), observer reports do not release. On
     show, the first fresh report decides.
   - Every hold that meets a condition at the same moment releases in ONE
     settle with ONE compensation session; release (c) starts none
     (below).
   - Releases (a) and (b) are deferred while T holds a non-collapsed text
     selection, has `:focus-within`, or is under the pointer (`:hover`).
     They fire when that ends (`selectionchange`, `focusout`,
     `pointerleave`). A badge click or a hand scroll to the top (c) does not
     wait for these.
   - A closing hold releases into the closed section on the same rules.
6. **Closing hold (515=b).** A visible topic that the server closes takes its
   folded closed rendering (the server's fold, `forgetOpen` as today) and
   stays at its open-flow place: it is placed by `hold.at` (its last open
   time), not at `topicAt` = null. On release it goes where the server
   order puts it.
   - The closed-page reconciliation in `morphBoard` (pinShift, closed-slot,
     `data-closed-next`) runs on the server's order exactly as today. The
     hold is applied later, in weave, and only re-places a node among the
     board's own wanted nodes. It never touches `.closed-page` contents or
     counts.
   - A closing hold counts toward the badge's N like any held or moved
     topic (decision 7, revised) and carries a "closed" tag instead of
     "moved ↑".
7. **Indicator (525=b; revised 2026-11-14 by owner (572)=ok, (590)=ok,
   (573)=a, (574)=b on the v2 badge mockup).**
   - A held rising topic gets a static accent: a left border in the accent
     colour and a small "moved ↑" tag in its `.tbar`.
   - One 3 px upward nudge, 300 ms, plays when the hold starts and on each
     re-update of a held topic. Nothing periodic. Under
     `prefers-reduced-motion`: no nudge, the accent only.
   - **Back-to-top badge** (supersedes the "N개 업데이트됨 ↑" pill; approved
     reference
     `ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/board-totop-badge-mockup-v2.html`):
     - One badge, centered at the top edge of the board scroller, shown
       whenever the board scroller is scrolled down at all and hidden at the
       top ("at the top" below).
     - Idle form: a symmetric 32x32 round ↑ button, no label; `title` and
       `aria-label` "Back to top" (ko "맨 위로").
     - Pending form, while N >= 1 updates are pending: the accent colours, a
       dot, and the label "Board updated" (ko "보드 업데이트됨"), plus
       " · N" when N >= 2. N counts the distinct topics that a board swap
       changed since the user scrolled down from the top: rising holds,
       closing holds, topics moved at once (in or out of view), newly
       created topics, and in-place updates (the mockup's `send()` counts
       every update while not at the top).
     - Motion while pending: the whole capsule moves with `translateX` only,
       no rotation, decaying -4/+4/-3/+2/-1 px over about 0.9 s, then about
       1.7 s still, looping (the mockup's `ttShake`, 2.6 s). None under
       `prefers-reduced-motion`.
     - Click: release (c) of every hold in one settle (decision 5), then the
       board smooth-scrolls to its top (the toolbar), newest update first;
       every counted topic flashes once (the mockup's `.fresh` ring, about
       0.6 s). Instant scroll and no flash animation under reduced motion.
     - Scrolling back to the top by hand also releases every hold (as (c)),
       flashes the counted topics, and clears N.
     - "At the top" is `scrollTop <= 4` (the mockup's threshold), so
       sub-pixel or clamp residue from a keep session does not show the
       badge. The badge sits at the top edge of the board's scroller (the
       nearest scroller, decision 4); on a document-scrolled board page that
       is the document, and fixed vs sticky placement is the implementer's
       call.
     - The badge is hidden while overview
       (`261114-feat-ws-dashboard-board-toolbar-overview`) is shown.
   - The board scroller keeps `overflow-anchor: none` (already on `.bcol`
     at `dashboard.css:764` since Phase 1's merge): Chrome's scroll
     anchoring double-compensates against decision 4's keep session (UX
     measured a 105 px drift). Phase 4 verifies it is still there.
   - Strings (all rendered by dashboard.js, so `js.` keys through `wsT`, in
     every `internal/wsi18n/tables/*.json`):
     - `js.board.hold.pill` = "Board updated" (ko "보드 업데이트됨"). The
       " · N" suffix is composed in JS from the number, so no plural key is
       needed.
     - `js.board.hold.pill.title` = "Back to top" (ko "맨 위로"): the
       badge's `title` and `aria-label` in both forms.
     - `js.board.hold.moved` = "moved ↑" (the rising tag).
     - `js.board.hold.closed` = "closed" (the closing-hold tag). It gets its
       own key, not a reused server-side "closed" string, so the tag can be
       worded as a transient state.
     The `js.board.` prefix is new. No existing prefix covers the board
     motion, and `js.pins.` is the pinned column's.
   - Rejected (the pre-revision text): a pill shown only while rising holds
     exist with "{0}개 업데이트됨 ↑" plural keys, and a click that scrolls to
     the topmost destination just below the pill. The owner chose the v2
     badge.
8. **One visibility source and one keep-in-place helper, shared.**
   - The visibility source is IntersectionObserver with `root: null`.
     It clips by ancestor scrollers, so it works whether `#bcol`, `#bpins`
     or the document scrolls (narrow layouts included). It observes the
     board's topic sections and is re-pointed at the nodes `morphBoard`
     inserts and removes.
   - The anchor session of decision 4 is a helper: keep node N at its
     screen offset in its nearest scroll container until layout settles.
   - `261113-feat-ws-whiteboard-topic-history-load-more` needs both on the
     standalone whiteboard page too: its "collapse once the whole card is
     out of view", and keeping the oldest entry in place on load-more. So
     the two helpers belong in `wswhiteboard/assets/stream.js`, beside
     `wbHistory`, and dashboard.js uses them.
   - **Helper contract** (EXEC lead, technical). These are the names the
     consumers code against: this ticket, the topic-history ticket, and
     `261113-feat-ws-whiteboard-board-frontend-performance` Phase 2 (its
     decisions 9 and 10). They are added to the `window.wbHistory` object
     (`stream.js#L220`):
     - `wbHistory.watch(node, { onChange, onOut, margin })` returns a handle
       `{ node, unwatch() }`. Each call is its own subscription, so several
       consumers can watch one node. `margin` is an IntersectionObserver
       `rootMargin` string (default `"0px"`). The helper keeps one observer
       per distinct margin, all with `root: null`; the performance ticket's
       mount and unmount bands are wider margins on the same helper, not a
       second copy. `onChange(report)` runs on every report, where
       `report = { node, ratio, visible: ratio > 0, rect }` and `rect` is
       the observer entry's `boundingClientRect`. `onOut(report)` runs once
       per transition from `ratio > 0` to `ratio === 0`, that is "fully out
       of view". A node whose first report is already 0 has not left view,
       so `onOut` does not fire for it.
     - `wbHistory.unwatch(handle)` is the same as `handle.unwatch()`.
       `wbHistory.sweep()` drops every subscription whose node is no longer
       connected (one `isConnected` pass, no MutationObserver).
     - `wbHistory.seen(node)` returns the node's last report or `null`. It
       is a synchronous read for rect readers that must not force layout
       (performance decision 10).
     - `wbHistory.keep(node, { offset, observe, onEnd })` returns the
       session handle `{ scroller, apply(), end() }`. It measures `node`
       now, in layout terms (rect minus any running transform), in its
       nearest scroller (decision 4). Call it BEFORE the DOM change it
       compensates. `offset` (px from the scroller's top) overrides the
       measured offset: the session then writes the scroll that puts
       `node` there (Phase 1's `boardKeepSettle`, `dashboard.js:1078`, uses
       it; decision 5's badge click scrolls to the top without a session). `observe` lists
       extra elements for the session's ResizeObserver; by default the
       session observes the scroller's content. `apply()` corrects
       synchronously; the caller calls it right after its DOM change.
       `end()` ends the session. `onEnd(reason)` reports `"quiet"`, `"cap"`,
       `"user"` or `"ended"`. A call on a scroller with a running session
       re-anchors that session and returns its handle (decision 4).
     - `wbHistory.keeping(scroller)` returns the running session on that
       scroller or `null`. `restoreScroll` uses it to skip an owned
       scroller (decision 4).
     Rejected: separate globals per consumer (the performance and
     topic-history tickets would each need to locate them), and handing out
     the raw IntersectionObserver (consumers would each re-derive the
     out-of-view transition).
   - Owner of the helpers (EXEC lead, technical): this ticket's Phase 1
     builds them, and the topic-history ticket declares this ticket a
     prerequisite and reuses them. Rejected: whichever lands first builds
     them (two workers could each build a copy in parallel).
9. **Hidden tab: no special rule.** `hiddenRegions` already defers board
   swaps while the tab is hidden and lands them on `visibilitychange`
   (`dashboard.js#L5511`, `showRegions` at `#L5546-L5555`). The rule in
   decision 2 is evaluated at that swap, so a topic the user was reading
   still holds on return. The deferred focusout swap (`still` = true, no
   motion; `dashboard.js#L462-L471`) applies holds and compensation the
   same way.
   - **Hidden iframe-shell frames count as hidden** (EXEC lead, technical;
     this ticket owns it). `261113-feat-ws-dashboard-iframe-shell` keeps up
     to five session frames in an LRU (its (421)). A hidden frame gets no
     `visibilitychange`; the shell tells it that it is hidden so it lowers
     its update rate (that ticket's Decisions and Phase 2). A frame that is
     not rendered still has `doc.hidden` false. Without a rule it would
     apply swaps, start holds on topics nobody sees, and run compensation
     against a layout with no boxes. Its observer would also report ratio
     0 and fire release (a) out of sight.
   - So one predicate, `pageHidden()` = `doc.hidden` or the shell's
     hidden/low-rate state, replaces the bare `doc.hidden` test that feeds
     `hiddenRegions`. The shell's "shown" message runs `showRegions` as
     `visibilitychange` does. While `pageHidden()` holds, swaps are held,
     no hold starts, no keep session runs, and observer reports release
     nothing (decision 5). On show, the held swap lands and decision 2 is
     evaluated then.
   - The message's shape is the iframe-shell implementer's (recorded in its
     Result). If the shell's hidden message exists when this ticket's
     worker starts, the worker wires it into `pageHidden()`. If not, this
     part is a no-op guarded by the message's presence: `pageHidden()`
     reads `doc.hidden` alone. The worker's Result then carries a Forward
     note to the iframe-shell ticket naming `pageHidden()` and the show
     path as the hook its hidden-frame message sets. Rejected: leaving
     hidden frames to the shell ticket, which does not know about holds or
     keep sessions.
10. **Compatibility tier: T0.** Frontend assets plus new i18n keys only. No
    MCP tool, store shape, tracker event, or manifest change.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`;
  `agents-plugin-tool/internal/` ->
  `ai-docs/manuals/release-compatibility.md`.
- Keep `settleMoves` (dashboard.js) and `WsCards.settle` (cards.js) in step:
  the hold enters through `weave`/`order`, which feed both.
- Moving a node reloads its frames where `Element.moveBefore` is missing.
  The hold must add no move other than the release move of T itself (one
  put per released topic). `frameGrafts` behaviour is unchanged.
- No hold state in localStorage or in `persist()`.
- A maximized card stays put (`slideBoard` already skips `data-max`), and
  compensation must not scroll the scroller under the maximize sheet.
- Swap guards stay as they are: an input/textarea focused in the board
  (`editing()`) and a card drag defer the whole swap.
- New UI strings go through `wsT` with entries in every
  `internal/wsi18n/tables/*.json`. Non-English entries are worker
  translations: flag them as unchecked in the result.
- Under reduced motion, the nudge and the release slide are skipped;
  compensation still runs.
- (515)=b overrides the UX/MAIN recommendation (a): closing topics hold too.
  The closed-page reconciliation risk is real; decision 6 confines the hold
  to weave so `.closed-page` contents and counts never change, and Phase 3
  verifies it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions): "(447) The pinned column scrolls separately from the board column. When a pinned topic or card is updated it keeps its place ... it does not move. Rejected: bumping an updated pin" — bearing: constrains
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions): "(454) Hand moves of unpinned items land only in the open flow ... A hand-moved unpinned topic stores a placement time that overrides its bar time" — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, commit 13d6c3c7): "Cards are children of the board region, woven among the topics by one placement rule ... The board morph settles around the cards and never moves one" — bearing: constrains
- 0cd850f5 (2026-11-12, commit): "Not fixed (pre-existing, separate): an updated topic that stays open reloads its body frame, which shows blank for ~2 frames" — bearing: constrains
- ef5f0647 (2026-11-13, commit): "Helper ownership set to board-reorder-hold Phase 1 over whichever-lands-first, so two parallel workers cannot each build a copy of the visibility observer and keep-in-place helper." — bearing: supports
- 261113-feat-ws-whiteboard-topic-history-load-more (2026-11-13, Decisions): "Scroll compensation ... through the keep-in-place helper and the visibility observer that `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 builds in `wswhiteboard/assets/" — bearing: supports
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-13, Decisions): "Viewport mount/unmount ((532)=a, Phase 2) ... through the visibility observer that `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 builds in `wswhiteboard/assets/stre" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, assets/cards.js, assets/dashboard.css, wswhiteboard/assets/stream.js, wsi18n/tables/*.json (11 tables) |
| scope.surface | cross-module | shared helpers on window.wbHistory in wswhiteboard/assets/stream.js consumed by dashboard.js; hold enters through WsCards order/weave in cards.js |
| scope.new_public_symbol | yes | wbHistory.watch / unwatch / sweep / seen / keep / keeping (contract in decision 8), js.board.hold.* i18n keys (decision 7) |
| scope.new_type_contract | yes | the two shared helper signatures and the hold record map (hold.at); no Go, MCP, or store type |
| scope.test_surface | new-files | existing: wsdashboard/board_motion_test.go, cards_test.go, board_close_fold_test.go, closed_route_test.go; the phases add node unit tests and the committed headless harness scripts/board-headless.mjs (Phase 1) |
| complexity.reuse_points | confirmed | cards.js order/claim/weave/settle and topicAt, dashboard.js boardSpots/slideBoard/growTopic/setFrameHeights/frameGrafts, wbHistory in stream.js; all read in the tree |
| complexity.side_effect_risk | high | a node move reloads frames without Element.moveBefore, and the hold touches closed-page reconciliation and scroll writes on every board swap |
| risk.correctness | high | placement override, visibility and scroll compensation across layout changes, and closed-page counts must all stay consistent |
| risk.fit | moderate | overlaps in-flight iframe-shell and board performance tickets on dashboard.js and stream.js; sticky-head already landed in the same markup |
| risk.test | moderate | layout-dependent behavior needs a headless browser harness that does not exist yet; WebKit unverified |
| risk.security_or_contract | low | T0 frontend assets and i18n keys only, no route, MCP tool, store, or tracker event change (decision 10) |

## Blocked (2026-11-14): Phase 4 waits on an owner decision

Phases 2 and 3 are not blocked. Phase 4 waits on one owner question (routed
through MAIN as POLICY, since it touches the owner-approved decision 4,
(516)=ok): what a board swap does while the board sits at the top
(`scrollTop <= 4`).

- (i) The board stays pinned to the top: no keep session at the top, and the
  update lands at the head, in view, with the flash. This is what the
  approved badge mockup v2 shows ("맨 위에서는 배지 없음"; its `send()` with
  `atTop()`).
- (ii) Decision 4 applies at the top too, as Phase 1 does today
  (`boardKeep`, `dashboard.js:1059`, anchors the topmost in-view non-moving
  entry): a topic rising to the head goes in above the viewport, the view
  scrolls down by its height, and the badge appears in its pending form.

Lead recommendation: (i). The owner approved the mockup after (516), and
under (ii) "back to top" is undone by the next update. Remove this section
when the answer is written into decisions 4 and 7.

## Phases

### Phase 1: scroll compensation on every board swap

Build both shared helpers of decision 8 on `window.wbHistory` in
`wswhiteboard/assets/stream.js`, under the contract decision 8 names:

- The visibility observer: `wbHistory.watch` / `unwatch` / `sweep` /
  `seen`. `swap()` watches the topic sections `morphBoard` inserts (its
  returned `ins`) and the initial board's sections at load, and calls
  `sweep()` after each morph so the removed sections are unwatched. Phase 1
  wires no consumer of the reports except its tests. Phase 2 consumes them.
- The keep helper: `wbHistory.keep` / `keeping`. It covers the anchor pick,
  the layout-term measure, nearest-scroller resolution, one session per
  scroller with re-anchoring, the ResizeObserver session with quiescence
  end, the cap and user-scroll cancel, and `overflow-anchor: none`
  (decision 4).
- `restoreScroll` skips the scroller a session owns, and every board swap
  runs compensation through `keep` (decision 4). No hold yet. This alone
  fixes "an off-screen topic rising past the viewport shifts what I read".
- `pageHidden()` and, when the iframe-shell message exists, its wiring
  (decision 9).
- The headless harness, committed as `scripts/board-headless.mjs`. It is a
  repo-local developer command beside `scripts/dev.py`, as the performance
  ticket's measurement harness is (its decision 8), and it is not shipped.
  It drives a dev daemon started by `scripts/dev.py rerun daemon`. It
  resolves Playwright from the npx cache, as that harness does, with no
  `package.json` and no Playwright dependency. It is run as
  `node scripts/board-headless.mjs <scenario> --url <page>`, where the page
  is a dashboard session page or the standalone whiteboard page. Each
  scenario asserts and exits non-zero on failure. Later phases of this
  ticket and `261113-feat-ws-whiteboard-topic-history-load-more` add
  scenarios to this file and add no second harness. Rejected: an
  uncommitted throwaway script per phase, which leaves the later phases and
  the topic-history worker nothing to rerun.

Verification:
- node unit tests in the existing style (extract the function from
  dashboard.js or stream.js, run under `runNode`, like
  `board_motion_test.go`) for:
  - the anchor pick (never a moving node, a held topic first);
  - the session end rules (500 ms quiet, 5 s cap, a user scroll);
  - nearest-scroller resolution (`#bcol` when it scrolls, `#bpins`, else
    `document.scrollingElement`);
  - two overlapping keep calls on one scroller. The second re-anchors the
    first session: one session runs, the quiescence window restarts, and
    neither call's writes end the session as a user scroll. A write from
    outside the session still does end it;
  - `restoreScroll` skipping the owned scroller and still restoring the
    others;
  - watch bookkeeping: sections from `ins` are watched, removed sections
    are dropped by `sweep()`, two subscriptions on one node both fire, and
    `onOut` fires on a `ratio > 0` to `0` transition but not on a first
    report of 0.
- `scripts/board-headless.mjs` scenarios:
  - Adapter page (`#bcol` scrolls): scroll to the middle, update an
    off-screen topic above, update one below. The visible topic's screen y
    holds within 1 px through the 200 ms motion and through a body frame
    reporting its height 1 s later. Then a user scroll during a session
    cancels it.
  - The same run on a session page without an adapter, where the document
    scrolls (no `#tabbar`, `.dgrid.board`).
  - Observer: scrolling a topic fully out of view fires `onOut` once on
    both page kinds, and a topic that a swap inserts is watched from that
    swap on.

### Result (0b58bed9a) - 2026-11-14

Landed (14bb9a454, 7bb5b42f8, 20a325460, 0b58bed9a):
- `stream.js` `viewKit(win)` puts `watch` / `unwatch` / `sweep` / `seen` /
  `keep` / `keeping` on `window.wbHistory` under the decision 8 contract.
  `seen(node, margin?)` takes an optional margin (additive): the last report
  of that margin's observer, else the latest of any. Observers use
  `threshold: [0]`, so reports come on enter and leave only.
- Nearest scroller = the first ancestor with `overflow-y` auto/scroll/overlay
  whose content overflows it, else `document.scrollingElement`. That gives
  `#bcol` on adapter pages, `#bpins` in the pinned column, and the document
  otherwise, without naming them in the shared helper. Every session sets
  inline `overflow-anchor: none` on its scroller (the root element for the
  document) and restores it at the end. CSS adds it to `.dgrid.live > .bcol`
  and `.bpcol > .bpins` only, so a non-scrolling board column keeps native
  anchoring for the document.
- `dashboard.js`:
  - `boardKeep` starts the session before the morph on the topmost topic
    or card in view.
  - `boardKeepSettle` picks the anchor after the morph with `boardAnchor`
    (a held entry first, else the first one that is not moving). Moving
    means in `ins`, removed, or re-parented. A different pick re-anchors
    through `keep(node, {offset})` at its offset from before the morph.
    When nothing in view stays, the session ends.
  - `restoreScroll` skips the scroller that `wbHistory.keeping` owns.
  - No compensation runs while a card is maximized or the board column is
    hidden.
- Two fixes found by the harness:
  - `slideBoard(el, before, from)` slides in screen terms while a session
    owns the scroll. Before, held topics still slid by the size of the
    compensation.
  - Frames a swap brings are marked `wbKeep`. Their first height change runs
    through the running board session, or a new `boardKeep` when the
    session has already ended. The 500 ms quiet window had ended before late
    frame reports arrived.
- Board topic sections are watched with no callbacks at load, on morph
  insert, on a fetched closed page, and on cards.js adopt, and swept after
  each morph. Phase 2 adds the consumers.
- `pageHidden()` replaces `doc.hidden` in the `hiddenRegions` feed and in
  `showRegions`. No iframe-shell hidden message exists on this branch, so it
  reads `doc.hidden` alone.
- `scripts/board-headless.mjs` is committed with these scenarios:
  - `keep` and `observer`, each detecting the page kind from `#tabbar`.
  - `serve`, which builds ws-mcp into a temp dir, runs an isolated
    `ws-mcp dashboard --port 0` with `WS_CACHE_HOME` and dev assets, seeds
    an adapter session and a board-only session, and prints both URLs. This
    avoids replacing the shared daemon; the header also names
    `scripts/dev.py rerun daemon`.
  - Board updates are injected client-side through the page's captured
    stream socket, as a faithful reorder of the last board region. A late
    height is posted from inside the frame.

Verification:
- `go test ./internal/wsdashboard/ ./internal/wswhiteboard/` passes. The new
  node cases are `TestViewKitUnderNode` (nearest scroller, layout-term
  measure, 500 ms quiet, 5 s cap, user scroll, two overlapping keeps
  re-anchoring, the document scroller, watch bookkeeping, sweep, per-margin
  observers), `TestBoardKeepUnderNode` (anchor pick, settle, watchBoard
  once, restoreScroll skip), `TestBoardKeepWiring`, and the screen-term case
  in `TestBoardTopicMotion`.
- `node scripts/board-headless.mjs keep|observer --url <page>` passes on the
  adapter page (`#bcol`) and on the board-only page (document).
  - The worst |dy| of the topics in view is 0.45 px through the rise from
    above, the rise from below, and the swapped-in frame growing 300 px 1 s
    later.
  - A user wheel scroll ends the session, and no script writes follow.
  - `onOut` fires once, and an inserted topic is watched.
  - Chromium only; WebKit is not run.
- Lite review: no Critical or Important findings. Two Minors are fixed
  (0b58bed9a). The third, missing node coverage for the layout reads in
  `boardView`/`layoutY` and the clamp branch of `onScroll`, is left to the
  harness.

Forward (261113-feat-ws-dashboard-iframe-shell): the shell's hidden/low-rate
message for a session frame should set the state that `pageHidden()`
(dashboard.js, beside `hiddenRegions`) reads. Its "shown" message should run
`showRegions()` as `visibilitychange` does.

### Phase 2: rise hold, release, batching, guards

Hold map + `order()` override (decisions 1-3, 5). Release (a) consumes the
Phase 1 visibility observer (`wbHistory.watch` with `onOut`; this phase
builds no observer of its own), plus deferral on selection/focus/hover.
Batch release. Viewing-the-board conditions (column shown, no maximize).
The hand-move interplay. Hidden-page behaviour for holds (decision 9).

Verification:
- node tests for the pure parts:
  - the placement with holds: a card above T, pins atop vs the pinned
    column, two topics updated in the same swap, a re-update keeping
    `hold.at`;
  - the start decision table: visible / destination visible / pinned /
    column hidden / maximized / `pageHidden()`;
  - hold lifecycle: a held topic deleted from the board drops its hold;
  - batching: two holds meeting a release condition in one report yield
    one release batch.
- `scripts/board-headless.mjs` scenarios:
  - A visible topic updates -> it stays, its screen y unchanged. Scroll it
    out -> it is at the head when scrolled back, and the visible topic did
    not shift at release.
  - A text selection inside T defers the release until the selection
    clears.
  - Batch release (decision 5): two held topics leave view in one scroll
    and release in ONE settle with ONE keep session. The harness counts
    `WsCards.settle` calls and the sessions `wbHistory.keep` starts.
  - Hidden tab (decision 9): with the tab emulated hidden (`document.hidden`
    overridden plus a `visibilitychange` dispatch, or CDP), an update to
    the topic being read is held in `hiddenRegions`. On show it lands and
    the topic holds.
  - Deferred focusout swap (decision 9): an update arriving while an answer
    input in the board is focused lands on blur with no motion, and still
    holds the visible topic with compensation.

### Phase 3: closing hold

Decision 6. A visible topic closing stays folded in place, then moves to the
closed section once out of view. Reopen while held. Regression run of the
closed-page tests (`board_close_fold_test.go`, `closed_route_test.go`) and
the closed-page reconciliation with pages loaded (closed-more clicked once).

Verification:
- `scripts/board-headless.mjs` with the closed pages expanded: close a
  visible topic -> it folds in place. Scroll it out -> it heads the closed
  section. No topic shows twice or drops out between the capped section and
  the pages, and the `data-closed-next` count is unchanged versus the
  no-hold path.
- Reopen while held (decision 3): a held-closing topic that the server
  reopens drops its close hold, renders open again, and is evaluated as a
  rise candidate: it holds when still in view with its destination out of
  view, otherwise it moves now. A node test covers the decision. A harness
  scenario covers the in-view case.
- Hold lifecycle (decision 3): a node test that a held-closing topic pushed
  past the closed-topic cap by later closes drops its hold, with no
  duplicate and no missing topic between the capped section and the pages.

### Phase 4: indicator

Decision 7 (revised): accent + "moved ↑"/"closed" tags, one nudge, the
back-to-top badge (idle ↑ and pending "Board updated · N" forms, the
capsule sway, reduced motion), the badge click and the hand scroll to the
top as release (c) with the scroll to the board top and the landed-topic
flash (decision 5), the board scroller's `overflow-anchor: none` kept, and the
i18n keys decision 7 names (`js.board.hold.pill`,
`js.board.hold.pill.title`, `js.board.hold.moved`, `js.board.hold.closed`)
in all 11 tables.

Verification:
- `go test ./internal/wsi18n/...` passes. `keys_test.go` checks that every
  table holds every English key.
- `scripts/board-headless.mjs`:
  - The badge is hidden at `scrollTop` 0 and shows its idle form after any
    scroll down.
  - An update arriving while scrolled down switches it to the pending form;
    N counts distinct moved or held topics ("Board updated" alone at N = 1,
    " · 2" at N = 2).
  - A badge click with two holds releases both in one settle, ends at
    `scrollTop` 0 with the newest update first, flashes the landed topics,
    and clears N; a hand scroll to the top does the same release and clears
    N.
  - With reduced motion emulated, the pending badge has no animation and
    the click scroll is instant.
- Side-by-side LOOK check of result captures against both approved
  references: the badge against
  `ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/board-totop-badge-mockup-v2.html`
  (idle, pending N = 1, pending N >= 2, the sway, the click result), and
  the accent, tags and nudge against
  `ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/reorder-hold-mockup-v2.html`.
  Where a mockup differs from the Decisions (hover deferral,
  quiescence-ended compensation, slide on release, no reopen / maximize /
  hidden-column cases), the Decisions win.
- `scripts/board-headless.mjs` screenshots at 1440 px and 700 px widths,
  light and dark, with one and two holds and with reduced motion emulated.
