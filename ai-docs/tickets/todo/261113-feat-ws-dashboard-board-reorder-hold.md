---
title: "Dashboard board: hold a topic the user is reading in place when the server reorders it"
related:
  261113-feat-ws-whiteboard-topic-history-load-more: "reuses this ticket's Phase 1 visibility and keep-in-place helpers (decision 8); this ticket is its prerequisite"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "co-edits topic markup and board motion assets"
  261113-feat-ws-dashboard-iframe-shell: "moves session pages into a shell document; the board scroller stays in the session page"
---

# Dashboard board: hold a topic the user is reading in place when the server reorders it

## Background

Owner feedback on board jumps (2026-11-13); MAIN's draft (board topic
`board-reorder-hold`); UX critique and spec (UX lead, read against develop
14aabe4b5); owner answers on MAIN's board: (515)=b, (516)=ok, (517)=ok,
(518)=ok, (525)=b (MAIN board IDs; (525) here is MAIN's indicator item, not
the EXEC release-gate item renumbered to (526)).

The server orders open topics by last update (`wswhiteboard/view.go:582-602`),
so an update moves its topic to the head of the board, and a close moves it
to the closed section. `swap()` (dashboard.js) morphs the board
(`morphBoard` -> `WsCards.weave`/`settle`) and plays a 200 ms grow + FLIP
slide. Then `restoreScroll()` writes back the old absolute `scrollTop`. So:

- a topic the user is reading jumps away from under them, and
- a topic rising from above or below the viewport past it shifts every
  visible topic by its height, because nothing anchors the view.

Approved visual reference: mockup v2, archived at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/reorder-hold-mockup-v2.html`.

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
4. **Scroll compensation on every board swap and every hold release,
   continuous until layout settles ((516)=ok).**
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
     history pages (`wbHistory.restore`, dashboard.js:551-552).
   - The session ends when no layout change has come for 500 ms (hard cap
     5 s), or at the first user scroll. A user scroll is a `scrollTop` that
     differs from the last one the session wrote.
   - The board scroller takes `overflow-anchor: none`, so Chromium's native
     anchoring does not compensate a second time. `restoreScroll()` stops
     writing `#bcol` (the document scroller and the other panes, `#bpins`
     included, keep it).
   - Decided: manual compensation over native `overflow-anchor`, because
     WebKit support is uncertain and the late frame heights need the same
     path anyway.
5. **Release** (517, batched):
   - (a) T is fully out of view (the IntersectionObserver ratio is 0). The
     move happens out of sight, so a frame reload there is unseen.
   - (b) The destination coming into view releases nothing while T is in
     view. It only counts once T is off-screen, so it is (a).
   - (c) The pill is clicked: every hold releases now in one settle with
     the slide, and the board scrolls to the topmost destination. On
     browsers without `Element.moveBefore`, a released T's body frame
     reloads; this is accepted for an explicit click. T's frame height is
     restored through the `setFrameHeights` path so the reload does not
     collapse it.
   - Every hold that meets a condition at the same moment releases in ONE
     settle with ONE compensation session.
   - Releases (a) and (b) are deferred while T holds a non-collapsed text
     selection, has `:focus-within`, or is under the pointer (`:hover`).
     They fire when that ends (`selectionchange`, `focusout`,
     `pointerleave`). A pill click (c) does not wait for these.
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
   - A closing hold does not count toward the pill and carries a "closed"
     tag instead of "moved ↑".
7. **Indicator (525=b).**
   - A held rising topic gets a static accent: a left border in the accent
     colour and a small "moved ↑" tag in its `.tbar`.
   - One 3 px upward nudge, 300 ms, plays when the hold starts and on each
     re-update of a held topic. Nothing periodic. Under
     `prefers-reduced-motion`: no nudge, the accent only.
   - A single sticky pill "N개 업데이트됨 ↑" (i18n key, all locales) sits
     at the top edge of the board scroller while N >= 1 rising holds exist.
     Clicking it does release (c).
8. **One visibility source and one keep-in-place helper, shared.**
   - The visibility source is one IntersectionObserver with `root: null`.
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
     `wbHistory` (for example `wbHistory.watch` / `wbHistory.keep`), and
     dashboard.js uses them.
   - Owner of the helpers (EXEC lead, technical): this ticket's Phase 1
     builds them, and the topic-history ticket declares this ticket a
     prerequisite and reuses them. Rejected: whichever lands first builds
     them (two workers could each build a copy in parallel).
9. **Hidden tab: no special rule.** `hiddenRegions` already defers board
   swaps while the tab is hidden and lands them on `visibilitychange`. The
   rule in decision 2 is evaluated at that swap, so a topic the user was
   reading still holds on return. The deferred focusout swap (`still` =
   true, no motion) applies holds and compensation the same way.
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

## Phases

### Phase 1: scroll compensation on every board swap

Replace the absolute `restoreScroll` for `#bcol` with anchor compensation
(decision 4), built as the shared helpers of decision 8: anchor pick, layout-term measure,
ResizeObserver session with quiescence end and user-scroll cancel,
`overflow-anchor: none`. No hold yet. This alone fixes "an off-screen topic
rising past the viewport shifts what I read".

Verification:
- node unit tests in the existing style (extract the function from
  dashboard.js, run under `runNode`, like `board_motion_test.go`) for the
  anchor pick (never a moving node, a held topic first) and for the session
  end rules.
- A headless Playwright run on a live dashboard: scroll to the middle,
  update an off-screen topic above, update one below. The visible topic's
  screen y holds within 1 px through the 200 ms motion and a body frame
  reporting its height 1 s later. Then a user scroll during a session
  cancels it.

### Phase 2: rise hold, release, batching, guards

Hold map + `order()` override (decisions 1-3, 5). Visibility source
(decision 8). Release (a), plus deferral on selection/focus/hover. Batch
release. Viewing-the-board conditions (column shown, no maximize). The
hand-move interplay.

Verification:
- node tests for the pure parts: the placement with holds (a card above T,
  pins atop vs the pinned column, two topics updated in the same swap, a
  re-update keeping `hold.at`), and the start decision table (visible /
  destination visible / pinned / column hidden / maximized).
- Playwright: a visible topic updates -> it stays, its screen y unchanged.
  Scroll it out -> it is at the head when scrolled back, and the visible
  topic did not shift at release. A text selection inside T defers the
  release until the selection clears.

### Phase 3: closing hold

Decision 6. A visible topic closing stays folded in place, then moves to the
closed section once out of view. Reopen while held. Regression run of the
closed-page tests (`board_close_fold_test.go`, `closed_route_test.go`) and
the closed-page reconciliation with pages loaded (closed-more clicked once).

Verification:
- Playwright with the closed pages expanded: close a visible topic -> it
  folds in place. Scroll it out -> it heads the closed section. No topic
  shows twice or drops out between the capped section and the pages, and
  the `data-closed-next` count is unchanged versus the no-hold path.

### Phase 4: indicator

Decision 7: accent + "moved ↑"/"closed" tags, one nudge, the sticky pill
with release (c) and scroll to the destination, reduced motion, and the i18n
keys.

Verification:
- Side-by-side LOOK check against the approved mockup
  `ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/reorder-hold-mockup-v2.html`
  (accent, tags, nudge, pill); where the mockup differs from the Decisions
  (hover deferral, quiescence-ended compensation, slide on pill release,
  no reopen / maximize / hidden-column cases), the Decisions win.
- Playwright screenshots at 1440 px and 700 px widths, light and dark,
  with one and two holds and with reduced motion emulated.
