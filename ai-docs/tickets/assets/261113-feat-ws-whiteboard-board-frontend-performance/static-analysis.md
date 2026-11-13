# ws whiteboard board rendering: static frontend performance analysis

Date: 2026-11-13. Scope: read-only static analysis of the board on the dashboard
session page (the primary surface) and the static full page (`live.js`). No repo
files were edited. One offline number source: a throwaway generator run through
`go run -overlay` (virtual `cmd/` file mapped from `/tmp/wbperf/main.go`, nothing
written to the repo). It builds synthetic `State`s and calls
`wswhiteboard.RenderRegions`, the same call the dashboard makes. It measures
bytes, iframe counts, and server render time on this Mac. It does **not** measure
browser cost.

Related tickets:
- `ready/261113-feat-ws-dashboard-iframe-shell` (the shell).
- `todo/261113-feat-ws-whiteboard-topic-history-load-more`. Its `related` names
  (473).
- `todo/261113-feat-ws-dashboard-frontend-performance-profile-and-optimize`. Its
  long-whiteboard owner input is the (473) concern, and it lists candidate
  techniques. No standalone (473) ticket exists.

---

## 1. Cost model

Notation:
- T_live: topics rendered live, meaning all open topics plus the 30 most recently
  closed (`view.go:12` `ClosedCap = 30`).
- T_def: closed topics past the cap. They are carried as inert
  `<template data-history-topic>` (`page.tmpl:19`, `view.go:629-633`).
- P: distinct panels per topic.
- V: versions per panel.
- H: log entries per topic.
- W = 50: the history window (`stream_view.go:11`).
- d = 1 if the topic has a body.

### 1.1 What becomes an iframe (dashboard)

`newPageData` renders **every panel record of every history page of every topic,
including deferred topics**, through `panelHTML` (`render.go:458-470`). With
`framed=true` (`RenderRegions`, `render.go:254-279`) each one becomes a sandboxed
`srcdoc` iframe built by `framedDoc` (`render.go:547-571`). The topic body becomes
one more (`render.go:483-487`).

Each frame document inlines:
- the full `page.css`, 59 KB (`render.go:555`);
- `svg-view.js` + `chg.js` + `code.js` + `doc-frame.js`, 21 KB (`render.go:152`,
  `:560`);
- an i18n subset.

All of it is HTML-escaped into the attribute (`render.go:569-570`). Measured
fixed overhead: about 84.5 KB per frame HTML-escaped, and about 90 KB after Go's
`json.Marshal` HTML-escaping in the WebSocket event (`sse.go:590`). There is no
compression on the stream; no deflate anywhere in `wsdashboard`.

Earlier panel versions do not show at their own place in the stream
(`Segments` skips historical runs, `stream_view.go:177-182`). Each one is nested
under its latest card as a full `entry` with its own iframe (`page.tmpl:53`,
`{{with .Versions}}` inside `details.panel-versions`). Versions come from the
**whole** topic history, not the window (`stream_view.go:248-266`).

Frame counts per board:
- **In the payload:** F_bytes = Σ over all topics (live + deferred) of
  (d + Σ_panels V_p) = (T_live + T_def)·(d + P·V).
- **Live in the DOM:** F_live = Σ over T_live of (d + Σ V_p for panels whose
  latest record is in the last W entries). Older pages sit in
  `<template data-history-page>` (`page.tmpl:47`) and are inert. Upper bound
  T_live·(1 + P·V).
- **Visible by default:** per open topic, the body (if `tdoc` is open) plus the
  **latest 3 distinct panels** (`stream_view.go:243-247`). Everything else is
  inside a closed `<details>`:
  - every `panel-versions` fold;
  - all panels older than the latest 3;
  - the whole `topic-stream` of a closed topic (`page.tmpl:47`,
    `{{if not .Closed}} open`).

  Closed `<details>` does **not** stop an iframe from being created. The frame
  still parses its HTML, CSS, and scripts, and `doc-frame.js:47-54,132` calls
  `getBoundingClientRect` and sets up a `ResizeObserver` (plus `chg.js:55`'s).
  So **F_live frames are created, but only about 4 per open topic are ever
  looked at.**

### 1.2 Bytes (offline measurement, `RenderRegions` board region only)

| T | P | V | H | closed | board bytes | iframes in payload | live iframes | outer tags (live) | server render (M-series) |
|---|---|---|---|---|---|---|---|---|---|
| 1 | 30 | 1 | 50 | 0 | 2.75 MB | 31 | 16 | 413 | 18 ms |
| 5 | 2 | 1 | 40 | 0 | 1.4 MB | 15 | 15 | 1.5k | 30 ms |
| 10 | 3 | 2 | 80 | 3 | 6.5 MB | 70 | 70 | 4.1k | 57 ms |
| 15 | 4 | 2 | 150 | 8 | 12.9 MB | 135 | 75 | 5.8k | 124 ms |
| 10 | 5 | 3 | 100 | 0 | 14.5 MB | 160 | 160 | 4.9k | 104 ms |
| 20 | 4 | 3 | 150 | 10 | 24.2 MB | 260 | 200 | 8.6k | 204 ms |
| 30 | 6 | 4 | 200 | 15 | 68.6 MB | 750 | 510 | 14.6k | 473 ms |
| 30 | 0 | 0 | 200 | 15 | 5.4 MB | 30 | 30 | 10.6k | 162 ms |

(Synthetic panels are about 1 KB of markdown. Real panel content adds on top, and
a topic-body image is inlined as a base64 data URL (`dashboard_page.go:313`,
`ImageDataURL`).)

Bytes ≈ 90 KB × F_bytes + content + about 0.6 KB per history entry (older pages
ship as templates) + items. **The per-frame inline CSS+JS is 85-95 % of the
payload.** The reviewer's "2.6 MB at 30 panels" reproduces as row 1.

### 1.3 DOM

- **Host document.** It is moderate: about 300 outer elements per live topic
  (measured; rough split: 40 chrome + about 15 per item + about 8 per windowed
  log row + about 12 per panel card + about 11 per version). That is 5-15k
  elements on a large board. Template contents (older pages, deferred topics)
  are not in the live tree.
- **Each frame.** It is a separate Document with its own CSSOM. The 738-line
  sheet is parsed per document for inline `<style>`. Each frame also has its own
  JS realm (sandboxed, opaque origin), so style, layout, script, and memory are
  paid per frame.
- **Static full page.** `RenderPage` uses `framed=false`, so there are no
  iframes. But every panel version's rendered markdown is inline in the live DOM
  (inside closed details), so DOM scales with T_live·P·V·content.

### 1.4 What one update costs (dashboard)

1. **Server.**
   - `boardMemoBase` `json.Marshal`s the whole board state and sha256-hashes it
     on every read (`dashboard_page.go:178-190`).
   - `watchWhiteboardFiles` walks and stats every `.json`/`.jsonl`
     (`whiteboard_store.go:22-29`).
   - On a change, `RenderRegions` re-renders **the entire board**: every topic,
     every deferred topic, every older page, every panel version's markdown, and
     every frame's 85 KB escape (`dashboard_page.go:239`). Measured 0.1-0.5 s
     for 135-750 frames on this Mac; expect several times that on a weak box.
   - The region hash is sha256 over the whole HTML (`pages.go:549-552`).
   - The stream coalesces at 250 ms (`sse.go:46`), so a streaming agent can
     trigger a full-board ship up to 4 times a second.
2. **Wire.** The whole board region goes as one JSON event (`sse.go:586-597`),
   uncompressed.
3. **Client parse.** `JSON.parse`, then `frag(html)` parses the whole board into
   a template (`dashboard.js:995`). Then **every incoming topic is serialized
   back** with `n.outerHTML` (`dashboard.js:694`) for the comparison with
   `boardSeen` (`:701`). That is O(total bytes) twice per swap, independent of
   what changed.
4. **Changed topic.** Its section is replaced. All its frames are recreated,
   except the **first** iframe, which `frameGraftable` double-buffers
   (`dashboard.js:790-800`, `q(...)[0]`). A new log line in a topic with P·V
   frames therefore rebuilds P·V-1 frame documents.

   Topics sort by last update, so the changed topic moves to the head. With
   `Element.moveBefore` (`cards.js:133`, `dashboard.js:809`) the kept topics don't
   reload; without it, moved sections reload their frames.
5. **Pre-swap reads.**
   - `captureBoardHistory` (`dashboard.js:507`, `stream.js:140-147`) runs
     `position()` per history, reading anchor rects (`stream.js:32-43`).
   - `boardSpots` reads every topic's rect (`dashboard.js:906-918`).
   - `frameHeights` and `readAnswers` each walk the board.
6. **Post-swap.**
   - `wbHistory.init` and `restore` run for each inserted node, and then again
     for every board box (`dashboard.js:520, 555`). `restore` runs `materialize`
     for **every** snapshot entry against each root (`stream.js:151-153`).
     `materialize` uses `querySelector('[id="…"]')` and may scan every deferred
     template's content (`stream.js:113-124`): about O(T × N_dom) per swap.
   - FLIP: `growTopic` reads each inserted section's rect plus
     `getComputedStyle` (`:936-947`), and `slideBoard` reads every child's rect
     (`:955-967`). That is about 2 forced layouts of the host, acceptable alone.
7. **Height-message thrash.** Each new frame posts `{wbDoc:"height"}` on load and
   on every resize (`doc-frame.js:47-54`). For **each** message the host does
   three things:
   - `docFrame(e.source)`, which is `q("iframe.doc-frame").filter(inBoard…)`: a
     full-document query plus a `contains` walk per frame (`dashboard.js:3242-3244`);
   - `f.getBoundingClientRect()` (`:3345`), right after the previous message
     wrote `f.style.height`, which is a **forced synchronous host layout per
     message**;
   - `postTheme(f)`.

   With F new frames that is O(F²) query work plus F forced layouts of the whole
   host. The per-history `ResizeObserver` (`stream.js:210-216`), which reads
   rects in `applyPosition`, and `rail.js`'s per-topic `ResizeObserver`
   (`rail.js:429-433`), which re-queries and re-measures, then fire per height
   change as well.

### 1.5 Static full page (`live.js`)

- A stamp `<script>` is injected every 1.5 s, forever, hidden tab included
  (`live.js:45-52`).
- Any seq change triggers `location.reload()` with scroll restore
  (`live.js:27-34, 58-61`). That re-parses the whole page, all inline panel
  versions included, and re-runs every script and mermaid. A background tab also
  reloads on every change.

### 1.6 Idle observers and timers (dashboard with a board)

| Source | Cadence | Cost scaling |
|---|---|---|
| `tickTimes` `dashboard.js:91-97` | 1 s | Full-document `querySelectorAll` on attribute selectors: O(N_dom) every second |
| `ago.js:23` | 60 s | O(topic bars) |
| `pill.js:144-170` | IO per open item; scroll/resize debounce 120 ms; **`input`/`click`/`change` on the host** and a MutationObserver per item | `update()` reads every unseen item's rect (`pill.js:125-141`) |
| `rail.js:427-433` | scroll in rAF; RO on the box and **each topic child** | O(T) rects per frame; any frame resize triggers re-query + O(T) |
| `stream.js:201, 210` | scroll listener per history (not rAF-throttled); RO per history | anchor-rect scan per scroll event / resize |
| Per frame: `doc-frame.js:53` RO, `chg.js:55` RO, selectionchange | event-driven | ×F_live |
| WebSocket region stream | 250 ms coalesce, 15 s refresh tick (`sse.go:46,51`) | full board when hash changes |

### 1.7 Per keystroke in a board note (the (417) typing latency)

1. `form.js:98` `grow()` sets `height:auto` and reads `scrollHeight`: forced
   host layout (`form.js:133`).
2. `form.js:99` `build()` runs document-wide `q(".card.policy[data-id]")`,
   `.ckrow`, `.arow`, `.srow`, and `textarea.tnote`, with `classList.toggle` on
   every check row (`form.js:151-181`): O(items on board).
3. `pill.js:162-163` runs `update()` on `input`: it reads every unseen item's
   rect right after `grow` wrote a height, which is a second forced layout.

Each layout covers the whole host board. That is why typing cost grows with
board length. The main composer has none of this.

---

## 2. Top bottlenecks, ranked by expected impact on a weak machine

1. **Frame explosion: creation cost and memory ∝ F_live = T_live·(d + P·V).**
   Every panel version, every collapsed panel, and every closed-topic stream is
   a live frame document. Each one costs: an 85 KB HTML/CSS/JS parse, a JS realm,
   a CSSOM, an initial forced layout, and two ResizeObservers. Only about 4 per
   open topic are visible. On a weak PC this dominates load, session switch
   (today a full reload), and memory.
   - Mermaid makes it worse: a frame with a mermaid block loads and evaluates
     the 3.5 MB `mermaid.min.js` in its own realm (`render.go:561-563`), hidden
     version frames included.
2. **Every change ships and reparses the whole board, then rebuilds the changed
   topic's frames.**
   - Server: a full re-render on each change, measured 0.1-0.5 s on fast
     hardware for realistic-large boards.
   - Wire: the full payload (MBs to tens of MBs), uncompressed.
   - Client: two O(bytes) passes (parse plus `outerHTML` re-serialize), then
     P·V-1 frame rebuilds in the touched topic. A log-streaming agent repeats
     this up to every 250 ms.
3. **Payload size is dominated by duplicated per-frame CSS+JS**, and by shipping
   history nobody opened: older pages and deferred closed topics are fully
   rendered, frames included. Bytes ∝ all topics × all versions, not the
   visible set. This drives #2's parse cost and the server render time.
4. **Height-message layout thrash plus O(F) frame lookup per message.** F
   forced host layouts and O(F²) query work at every load and every swap that
   creates frames. These are long tasks proportional to board length.
5. **O(N_dom) work per keystroke and per second.** On the typing path: `build()`
   global queries, `pill.update` rect reads, and two forced layouts. On the idle
   path: the 1 s `tickTimes` full-document query. This is the (417) symptom.
   Host layout is O(whole board) because nothing is contained.

Lower-ranked items, each independent:

6. The static page reloads fully on every change, including in hidden tabs.
7. Server write path, per the reviewer: `plan` appends a `meta.json` write for
   **every** topic key on every update (`folders.go:571-657`). Each write goes
   through `atomic` with fsyncs plus parent-directory syncs (`folders.go:854-888`)
   under the flock. This adds update latency, not frontend CPU.
8. No per-entry render cache on the server. Entry and panel-version IDs are
   immutable, yet every board render re-converts all their markdown.

---

## 3. Cheapest effective fix per bottleneck

None of these needs a framework. They are all vanilla JS, CSS, or Go template
changes in the existing no-build-step assets. Effort: S ≤ 1 day, M = 2-4 days,
L ≥ 1 week.

| # | Fix | Effort | Notes |
|---|---|---|---|
| 1a | **Mount a frame only when its `<details>` chain is open.** Server emits `data-srcdoc` instead of `srcdoc` for frames under a closed `details` (or for all frames). The client sets `srcdoc` on the first `toggle` open, and on insert in `morphBoard`, `adopted`, and `wbhistoryload`. | S | Cuts F_live from T(d+PV) to about 4 per open topic. Version frames mount only when the user opens "versions". Asset/template only, T0. Existing `frameHeights`/`setFrameHeights` already keep heights by `data-doc`. |
| 1b | **IntersectionObserver mount/unmount near the viewport** for open frames: mount within about 1-2 viewports, unmount beyond about 4-5 viewports, keeping the last `style.height` as the placeholder. | M | Needs hysteresis, the `frameGraft` path (which expects a live old frame), `boardJump`, selection/mention state (`mpend.frame`), and a mermaid redraw on remount. Implicit-root IO works inside the shell's nested frame and through the `.wb-history` clip. |
| 1c | "Render only the latest version" | (subsumed) | 1a already makes older versions on-demand without changing markup semantics. A server-side variant (older versions fetched on open) is only worth it after 3b. |
| 2a | **Keyed frame reuse inside a changed topic.** `data-doc-key` `entry:eN` is immutable (append-only entries, immutable panel versions). When a changed section comes in, move the old frame with the same key into it via `moveBefore` instead of using the new one. Generalizes `frameGraftable`'s first-frame-only graft (`dashboard.js:790`). | S-M | Client-only. Relies on `Element.moveBefore` (Chromium 133+); fall back to today's behavior elsewhere. |
| 2b | **Per-topic content hash attribute** (`data-th`) emitted by the server; `morphBoard` compares hashes instead of `outerHTML`. | S | Removes the O(bytes) re-serialization per swap. T0 attribute. |
| 2c | **Incremental board events**: the server sends only changed topics (keyed by `data-topic` + hash) plus order, not the whole region. | M-L | New additive event type opted into by the page (T0). Touches `sse.go` and the region cache that shell Phase 2's fan-out cache also changes. |
| 2d | **Server per-entry render cache** keyed by (entry ID, lang): immutable entries render once. | S-M | Cuts the full re-render to the changed parts. Pure Go, no wire change. |
| 3a | **Serve frame CSS/JS as hashed public assets**: `<link rel=stylesheet href="/s/wb-frame-<hash>.css">` and `<script src="/s/wb-frame-<hash>.js">` instead of inline. Exact precedent: the public, immutable, content-hashed mermaid route (`access.go:129-138`, `pages.go:1048, 1249-1254`). | S-M | Cuts about 85 KB of each frame's about 90 KB. The browser can share one parsed stylesheet and the code cache across frames (verify). CSP has no `style-src` (`pages.go:1276`). T0 additive route. The theme pin (`ThemeFrames`) is unaffected. |
| 3b | **Server-side windowing**: stop shipping older history pages and deferred closed topics as `<template>`; fetch them on demand through a route like the existing `/s/<id>/closed?from=` (`dashboard.js:634`). | M | Bytes then scale with the visible window, not with total history. Must be coordinated with the load-more ticket (see §4). `wbHistory.materialize` for jumps needs a fetch path. |
| 4 | **Batch frame heights**: queue `{frame→h}` in a Map, apply in one rAF, read the "from" height from `style.height` instead of `getBoundingClientRect`. Resolve `docFrame` through a `Map(contentWindow→iframe)` refreshed on insert. | S | Removes F forced layouts and O(F²) lookups. Client-only. |
| 5a | **Typing path**: `pill.js` defers `input`-driven `update` to rAF/120 ms (or only re-classifies the one item); `form.js` `build()` coalesces to rAF and scopes to the changed item's topic for the class toggles. | S | Directly targets (417). |
| 5b | **`content-visibility: auto; contain-intrinsic-size: auto 600px`** on `section.topic`, and optionally on `.wb-entry.panel`. `.topic` already clips (`page.css:140`, `overflow: clip` under `.wb-rail` at `page.css:654`), so paint containment is compatible with the sticky bar. | S (CSS) + M (audit) | Each forced host layout gets cheaper (helps 4 and 5). Readers that query rects **inside** skipped topics force those subtrees to lay out and defeat it: `pill.js:131`, `stream.js:32-43` `position()`, `captureBoardHistory`. Those must use IO results or the topic's own rect. Little gain for frame cost: offscreen cross-origin frames are already rendering-throttled by Chromium, and the remaining cost is creation (fix 1). |
| 5c | `tickTimes`: collect the `time[data-f=since/ago/rel]` set once per swap instead of querying the full document every second. | S | Idle CPU ∝ N_dom goes away. |
| 6 | `live.js`: while `document.hidden`, set `pending` and reload on `visibilitychange`; pause the poll while hidden. | S | Stops background-tab reload storms. A morph-style patch for the static page is L and not worth it while the dashboard is primary. |
| 7 | `plan`: emit a topic `meta.json` write only when that topic's serialized meta changed. | S-M | Same on-disk shape (T0). Backend latency only. |
| — | True list virtualization (only visible topic/entry nodes exist) | L | **Not recommended now.** It fights `rail.js` (measures all topics), `pill.js`, `boardJump`/`materialize`, cards `weave`/pins, `frameGraft`, find-in-page, and FLIP. 1a + 1b + 5b give most of the benefit at a fraction of the risk. Revisit only if measurement after 1-5 still shows host-DOM cost dominating. |

Suggested order: 1a, 4, 5a, 3a, then 2b + 2a, then 1b, 5b, 2d. Then 3b (with
load-more), and 2c (after shell Phase 2).

---

## 4. Relation to the iframe-shell ticket

What the shell changes (`ready/261113-feat-ws-dashboard-iframe-shell`):
- The sidebar and header move into a persistent shell document.
- Each session page becomes a same-origin frame (`/s/<id>?frame=1`) with its own
  WebSocket.
- A **5-frame LRU** keeps recent sessions alive, so hot switches skip the full
  reload. The target is under 100 ms.
- New framing headers.
- Phase 2: all pages framed, **hidden-frame throttling** (the shell tells a frame
  it is hidden), and a **server render fan-out cache** (one region render per
  session, fanned out by hash).

It does **not** change board rendering, frame-per-panel, morph, payload size, or
typing cost. A cold switch still builds the whole board. The board's srcdoc
frames become nested one level deeper.

Interaction: the LRU **multiplies board memory by up to 5**. Every kept session
frame holds its entire board, F_live frame documents included. So the shell
makes fix 1 more urgent, not less. The shell's hidden signal is the natural hook
for "unmount all board frames while hidden" (an extension of 1b).

| Fix | Relation to the shell |
|---|---|
| 1a mount-on-open, 1b IO mount | **Independent; land before.** Works unchanged inside a nested frame. Synergy: the Phase 2 hidden signal can drive unmount-all. |
| 2a keyed frame reuse, 2b topic hash | **Independent; before.** Board-internal (`morphBoard`). |
| 2c incremental board events | **Overlaps** the Phase 2 fan-out cache and per-frame WebSocket work in `sse.go` and the region cache. Sequence after shell Phase 2, or co-design with it. Not a conflict in outcome, only in files. |
| 2d server entry cache | **Independent.** Complements the shell's fan-out cache: the shell dedupes per viewer, this dedupes per entry. |
| 3a hashed frame assets | **Independent; before.** A same public-asset pattern. The `?frame=1` CSP composition (shell + browser-pane) adds `frame-src`/`frame-ancestors` only; the policy has no `style-src`/`script-src`, so nothing collides. Re-check if either ticket adds them. |
| 3b server windowing | **Independent of the shell**, but coupled to `261113-feat-ws-whiteboard-topic-history-load-more` (below). |
| 4 height batching, 5a typing, 5c tick | **Independent; before.** |
| 5b content-visibility | **Independent; before.** Per-document CSS. |
| 6 live.js | **Independent** (static page, not in the shell). |
| 7 store writes | **Independent** (backend). |

**Answer to "should optimization wait for the iframe shell?" No.** The top fixes
are board-internal and need nothing from the shell. Landing them first also
shrinks the 5-board LRU footprint the shell introduces. Only 2c should be
sequenced behind shell Phase 2.

### The load-more ticket (also (473)-linked)

`261113-feat-ws-whiteboard-topic-history-load-more` removes the `.wb-history`
inner scroll (`page.css:417`). It loads older entries upward in place and
collapses a topic back to the latest window once the card leaves the viewport.
Its Decision 5 is asset-only T0.

- **Bounds DOM growth from opened history.** The collapse limits that growth, but
  older pages stay shipped as templates. So it does **not** reduce bytes or
  server render: it keeps fix 3b's problem.
- **Choose before implementing it.** Either:
  - (a) it stays template-based and 3b later swaps the page source for a fetch
    behind the same "load more" control; or
  - (b) it is built fetch-based now, with a T0 additive route.

  Option (b) avoids building and then reworking the template path. Either way,
  build its viewport-leave detection and fix 1b's frame mount/unmount on one
  shared IntersectionObserver helper. Both need the same scroll compensation
  (its Decision 3, shared with board-reorder-hold).
- **Removing the inner scroll makes a long topic as tall as its window.** That
  raises the value of 5b (content-visibility) and 1b, because the page itself
  gets longer.

---

## 5. What static analysis cannot settle (and the cheapest measurement)

Open questions:
1. **Per-frame creation cost and memory in Chrome/Edge on the owner's Windows
   machine.** That covers parse/CSSOM/realm time, and whether sandboxed srcdoc
   frames are process-isolated (Chromium's sandboxed-iframe isolation puts them
   in a separate renderer process, adding IPC and memory). My order-of-magnitude
   guess is 5-20 ms and 1-3 MB per frame at 4-6x throttle. It is unverified and
   decides how much fix 1 is worth.
2. **Whether iframes inside closed `<details>` cost as much as open ones.** By
   HTML semantics they load. How much layout `doc-frame.js`'s forced
   `getBoundingClientRect` costs in a 0-width hidden frame is unknown.
3. **Whether native `loading="lazy"` defers `srcdoc` iframes.** The spec ties
   lazy loading to `src` navigation, and Chromium never defers `display:none`
   frames. If it did work, part of 1b would be one attribute. Test it rather
   than assume it.
4. **The real T/P/V/H and mermaid/image distribution on the owner's boards.** The
   local `ai-docs/whiteboards/` boards are small or legacy (for example, 86
   topics with empty logs; 11 topics, 10 versions).
5. **The split of an update between server render, wire/JSON parse, `frag` +
   `outerHTML`, frame rebuilds, and height thrash.** This picks between 2a/2b/2d
   and 3a.
6. **Whether shared external CSS actually de-duplicates CSSOM** across sandboxed
   frames in one process (it decides whether 3a also saves CPU, or only bytes).
7. **`Element.moveBefore` availability** on the owner's browser build. It decides
   whether reordering topics reloads frames today and whether 2a works.
8. **Whether content-visibility pays off** once frames are reduced, given
   Chromium already throttles offscreen cross-origin frames.

Cheapest measurement plan, about half a day:
- **Census (10 min).** A Python one-off over the owner's
  `ai-docs/whiteboards/<id>/t*/log.jsonl` and `panels/*.json`, giving per topic:
  entries, distinct panels, versions, payload bytes, mermaid fences, image
  count. (§1.2's generator columns are exactly these knobs.)
- **Synthetic board.** Reuse the overlay generator idea (`go run -overlay`, no
  repo change). Write a `FolderStore` board with the census's worst-case T/P/V/H
  into a scratch session folder that the dev daemon serves. Alternatively, write
  one HTML harness: `RenderRegions` board + `PageAssets()` + a copy of the
  dashboard's height handler.
- **Playwright on Chrome or Edge.** CDP `Emulation.setCPUThrottlingRate` 4 and 6,
  `Tracing.start` (devtools.timeline categories), `Performance.getMetrics`
  (Nodes, Frames, JSHeapUsedSize, LayoutCount, LayoutDuration,
  RecalcStyleDuration, ScriptDuration), PerformanceObserver `longtask` and
  `event` (Event Timing for keystroke INP), and `performance.memory` / process
  memory via CDP `SystemInfo`. Scenarios:
  - (a) cold load;
  - (b) 60 s idle;
  - (c) 20 scripted board updates, one log line each into one topic;
  - (d) type 50 characters into a topic note;
  - (e) scroll the board end to end.
- **A/B without a branch.** Dev assets reload from the working tree
  (`render.go:88-111`), so a local throwaway patch can be toggled under one
  daemon and each candidate (1a, 4, 5a, 3a) measured on the same board before
  committing.
- Record the numbers in the Phase 1 Result of
  `261113-feat-ws-dashboard-frontend-performance-profile-and-optimize`. The
  scenarios line up with its (a)-(d).
