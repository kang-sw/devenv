  // --- region swap -----------------------------------------------------------
  // The mode pseudo-region's hash is unknown to the page until the first
  // event carries it; an empty hash makes the server send it once. A region
  // update held back while the tab is hidden (hiddenRegions, see connect)
  // counts as held: a reconnect meanwhile names its hash, and the update
  // still lands when the tab shows.
  var modeHash = "", hiddenRegions = {};
  // pageHidden tells whether nobody sees the page (a hidden tab, or a frame
  // the shell hides): region updates are held (hiddenRegions) while it
  // holds, and land when it ends (showRegions).
  function pageHidden() { return doc.hidden || shHidden; }
  function held(withoutBoard) {
    return q("[data-region]").filter(function (el) { return !withoutBoard || el.getAttribute("data-region") !== "board"; }).map(function (el) {
      var name = el.getAttribute("data-region"), p = hiddenRegions[name];
      return name + "." + (p ? p.hash : el.getAttribute("data-h"));
    }).concat(mode !== "index" ? ["mode." + modeHash] : []).join(",");
  }
  // A deferred board swap lands still (no board motion), so the board never
  // moves under a just-blurred input.
  function swap(name, hash, html, still) {
    var el = region(name);
    if (!el) return;
    var board = name === "board";
    if (el.getAttribute("data-h") === hash && !(board && Object.keys(boardPending).length)) { if (board) deferred = null; if (name === "rail" && railGate) railGate.forget(); return; }
    if (board && window.WsOverview && window.WsOverview.holding()) { boardOverviewHeld = { hash: hash, html: html }; return; }
    if (board && editing()) { deferred = { hash: hash, html: html }; return; }
    if (board && window.WsCards && window.WsCards.dragging && window.WsCards.dragging()) { boardDragHeld = { hash: hash, html: html }; return; }
    // A rail render from before the layout change this page saw applied
    // would undo it on screen until the next one: it is dropped, and the
    // render after the change follows.
    if (name === "rail" && WsRail && WsRail.staleRail(html, railMinGen)) return;
    // A rail swap waits from a press on a grip or bar until it ends, while
    // a row's rename is open, and while a layout change posts (railGate),
    // and lands when that ends (railRelease).
    if (name === "rail" && railGate && railGate.offer({ hash: hash, html: html })) return;
    if (name === "head") {
      if (labelEditing()) { headHeld = { hash: hash, html: html }; return; }
      headHeld = null;
    }
    if (board) {
      deferred = null;
      var canonical = typeof html === "string" ? boardInputFull(html) : html;
      boardOffered = !boardLatest || boardLatest.hash !== hash ? { hash: hash, previous: boardLatest ? boardLatest.html : boardRendered, input: canonical } : null;
      boardLatest = { hash: hash, html: canonical };
      html = boardStageInput(boardLatest.html || html);
    }
    var pin = (name === "entries" || name === "convlive") && pinned();
    var scroll = scrollState();
    var answers = board ? boardRead(readAnswers) : null;
    var railTop = name === "rail" ? railScrollTop() : null, railFoc = name === "rail" ? railFocus() : null;
    var frameH = board ? boardRead(frameHeights) : null;
    var history = board ? captureBoardHistory() : null;
    // Growth/FLIP cannot run through a frozen visible interval: their
    // intermediate layout would escape the synchronous owner transaction.
    var before = board && !boardFrozen.length && !still && !reducedMotion() ? boardSpots(el) : null;
    var kept = board ? boardKeep() : null, boardTop = before && kept ? el.getBoundingClientRect().top : null;
    // The entries merge and the board morph prepare only the nodes they
    // insert; any other region is replaced whole. A finished reply's mermaid
    // fences draw once merged; the streaming reply (convlive) keeps them
    // code blocks.
    var ins = null;
    if (name === "entries") mergeEntriesRegion(el, html, pin);
    else if (board) {
      ins = morphBoard(el, html, boardFrozen, boardPagedEdits);
      boardPagedEdits = [];
      if (window.wbHistory && window.wbHistory.sweep) { window.wbHistory.sweep(); ins.forEach(watchBoard); }
      ins.forEach(bandBoard);
      ins.forEach(function (n) {
        // Materialize the reader's older pages before restoring disclosures
        // and frame heights, including immutable panel versions off the tail.
        if (window.wbHistory) { window.wbHistory.init(n); window.wbHistory.restore(n, history); }
        restoreOpen(n);
        fmtTimes(n);
        sweepOutImgs(n);
        setFrameHeights(n, frameH);
        mountFrames(n);
        writeAnswers(n, answers);
        // A placeholder a swap brought (the viewport bands mount it later)
        // takes the mark too, and its frame takes it once mounted.
        q(".doc-frame", n).forEach(function (f) { f.wbKeep = true; });
      });
      if (before) {
        var moves = topicMotion(before, ins).map(function (m) { return growTopic(m.n, m.from); });
        startMotion(moves.concat(slideBoard(el, before, boardTop)));
        ins.forEach(function (n) { q("iframe.doc-frame", n).forEach(function (f) { if (!f.wbOut) f.wbGrow = true; }); });
      }
    } else {
      el.innerHTML = html;
      restoreOpen(el);
      fmtTimes(el);
      sweepOutImgs(el);
    }
    el.setAttribute("data-h", hash);
    if (board) {
      indexFrames();
      if (window.wbForm) window.wbForm();
      bindCapsuleNotes();
      mermaid(ins);
      syncChips();
      // The notes came back with their markers (writeAnswers); their image
      // rows go back after them, and a note the swap took drops its images.
      if (ctext) { syncMarkers(); renderImgs(); }
      // An input-held selection is already a value snapshot, even when its
      // source frame leaves the board during an update.
      if (mpend && !mpend.locked && mpend.frame && !doc.contains(mpend.frame)) hideMention();
    }
    if (board || name === "convlive") applySent(board);
    if (name === "panels") drawerSync();
    if (name === "git") gitSwapped();
    restoreScroll(scroll);
    if (kept) kept.h.apply();
    if (board) {
      boardRecordPending(); boardPendingBadge();
      boardExposedUnits = Object.keys(boardPending).length ? boardWithSnap(boardExposureUnits) : null;
    }
    if (pin) pinConvo();
    if (live && TAB[name] && !tabShown(TAB[name])) {
      var dot = tabDot(TAB[name]);
      if (dot) dot.classList.add("on");
    }
    if (live && TAB[name] === "convo" && !visible(convo) && visible(fpane) && ctabDot()) ctabDot().classList.add("on");
    if (name === "panels") drawerSwapped();
    if (name === "tree" || name === "convlive") drawerWait();
    if (TL_REGIONS[name]) { delete tlHeld[name]; tlCount(); }
    if (name === "convlive") syncBadge();
    // A list that arrives while a / waits in the box opens the picker.
    if (name === "cmds" && slash) slash.sync();
    if (name === "entries") syncChead();
    if (name === "head" && !sseOn) connected(false);
    if (name === "head") labelSwapped();
    if (name === "rail") { railMarks(); railScrollTo(railTop); railRefocus(railFoc); }
    if (name === "head" || name === "convlive") leadSwapped(name);
    if (name === "rail" || name === "sessions") apScan(el);
  }

  // --- board morph: unchanged topics keep their nodes -------------------------
  // Each topic section carries data-topic, its name. A board swap keeps a
  // topic's section, and with it the body frame (no reload, no second
  // mermaid load, no second image relay), when the section's incoming HTML
  // equals the HTML it last arrived with (boardSeen). The comparison is
  // never against the live DOM, which the page changes after a swap (times,
  // sent marks, answers, frame themes and heights). Everything else the
  // board holds (the closed-topic control, the reply bar) is replaced. The
  // initial record is the board as served, read before anything changes it.
  // The server stamps each section with data-th, a hash of its rendered
  // bytes (TopicHTML, render.go), compared in place of the HTML (topicSeen).
  var boardSeen = {}, boardRendered = null, boardLatest = null, boardPending = Object.create(null), boardFrozen = [], boardRelease = false;
  var boardOwn = Object.create(null), boardImmediate = Object.create(null), boardSnap = null, boardExposedUnits = null;
  var boardChanges = [], boardOffered = null, boardBadge = null, boardReleaseFrame = 0;
  var boardPaged = Object.create(null), boardPagedEdits = [];
  function topicKey(n) {
    return n.nodeType === 1 && n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic") ? n.getAttribute("data-topic") : null;
  }
  function topicSeen(n) {
    return n.getAttribute("data-th") || n.outerHTML;
  }
  (function () {
    var b = region("board");
    if (b) {
      boardRendered = boardInputFull(b.innerHTML);
      Array.prototype.forEach.call(b.children, function (n) { var k = topicKey(n); if (k !== null) boardSeen["t:" + k] = topicSeen(n); });
    }
  })();
  // --- board cards (cards.js): the page's own cards among the topics --------
  // The framework mounts on the board region once, before any board swap,
  // and restores the cards and the pinned or hand-moved topics this browser
  // kept for the session. Showing a card switches to the board tab where
  // the view shows one column at a time.
  // Terminal cards (terminal.js) exist only on a page the server marked
  // data-term; their module adds the "+ Terminal" button at the end of
  // the composer's status line (or fixed at the page's corner without one).
  // Web cards (webcard.js) exist only on a data-sysopen page; their "+ Web"
  // button stands beside it, and their picks and console errors go to the
  // composer's chips (web picks, below).
  // The pinned column (bpcol) is the page body's last column: from 1600px,
  // while anything is pinned, unless the layout menu says "tabs" (one
  // column at a time, where the pins stand atop the board). The root's
  // data-pincol carries it to the stylesheet and the session rail (railWide):
  // below 2200px the column takes the rail's place and the rail overlays.
  // A board swap waits while a card or topic is dragged (boardDragHeld), and
  // lands at the drag's end.
  // Two owners, not two views onto one explorer. Keep the tab's existing
  // storage keys; the toolbar uses the same records in its own namespace.
  if (live && fpane && window.WsFiles) {
    var fileHooks = {
      rid: rid, post: post, toast: toast, copy: copyText, load: load, save: save,
      local: function () { return root.hasAttribute("data-sysopen"); },
      send: attachExplorerPath,
      openCard: function (location) {
        // Dismiss before constructing/revealing the new card, not afterward.
        window.WsCards.dismissFiles();
        window.WsFiles.open(location);
      }
    };
    // Register adapters before board mount restores explorer card IDs.
    window.WsFiles.setup(fileHooks);
    function createFiles(node, rootSlot, toolbar) {
      return window.WsFiles.create(node, Object.assign({}, fileHooks, {
        rootSlot: rootSlot,
        load: toolbar ? function (k) { return load("ws.dashboard.toolbar." + k); } : load,
        save: toolbar ? function (k, v) { save("ws.dashboard.toolbar." + k, v); } : save
      }));
    }
    filesTab = createFiles(fpane, chead && chead.querySelector(".fsum"), false);
    var sheet = doc.createElement("section");
    sheet.id = "toolbar-files";
    sheet.className = "fpane";
    sheet.hidden = true;
    sheet.setAttribute("aria-label", fpane.getAttribute("aria-label"));
    doc.body.appendChild(sheet);
    var sheetInstance = null;
    filesSheet = {
      el: sheet,
      shown: function () {
        if (!sheetInstance) sheetInstance = createFiles(sheet, null, true);
        sheetInstance.shown();
      },
      hidden: function () { if (sheetInstance) sheetInstance.hidden(); }
    };
  }
  var cardBoard = mode !== "index" && window.WsCards ? region("board") : null;
  var PIN_WIDE = window.matchMedia ? window.matchMedia("(min-width: 1600px)") : null, boardDragHeld = null, boardOverviewHeld = null;
  function afterOverview(fn) { if (window.WsOverview) window.WsOverview.after(fn); else fn(); }
  function pinColumnFits(n) {
    return n > 0 && !!(PIN_WIDE && PIN_WIDE.matches) && !(live && (grid.getAttribute("data-layout") || load(LAYOUT_KEY)) === "tabs");
  }
  if (cardBoard) {
    if (window.WsPreview) window.WsPreview.setup({ renderDiff: gwRender, post: post, toast: toast });
    if (window.WsTerm) window.WsTerm.setup({ post: post, toast: toast });
    // The composer's chips come later in this script: the hooks are hoisted
    // functions, and cchips is not yet assigned here.
    if (window.WsWeb) window.WsWeb.setup($("cchips") ? { post: post, toast: toast, rid: rid, addPick: addWebPick, addConsole: addWebConsole } : { post: post, toast: toast, rid: rid });
    var bpcol = null, dbody = cardBoard.closest(".dbody");
    if (dbody) {
      bpcol = doc.createElement("aside");
      bpcol.id = "bpcol";
      bpcol.className = "bpcol";
      bpcol.setAttribute("aria-label", wsT("js.pins.title"));
      dbody.appendChild(bpcol);
    }
    window.WsCards.mount(cardBoard, {
      key: "ws.dashboard.cards." + rid, load: load, save: save,
      toolbar: {
        overview: function () { if (window.WsOverview) window.WsOverview.toggle(); },
        filter: function (run) { if (window.WsOverview) window.WsOverview.filter(run); else run(); },
        search: function (e) { var b = $("tkqs"); if (b) { e.stopPropagation(); b.click(); } },
        files: filesSheet
      },
      reveal: function () { if (live && !tabShown("board")) setTab("board"); },
      column: bpcol, shows: pinColumnFits,
      layout: function (on) {
        if (on) root.setAttribute("data-pincol", ""); else root.removeAttribute("data-pincol");
        railOpen(false);
      },
      closed: function (from, done) {
        if (!window.fetch) { done(null); return; }
        fetch("/s/" + rid + "/closed?from=" + from, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
          if (!r.ok) throw new Error("HTTP " + r.status);
          return r.json();
        }).then(function (res) { done(frag(res.html || "").firstElementChild); }, function () { done(null); });
      },
      adopted: function (n) {
        if (window.wbHistory) window.wbHistory.init(n);
        watchBoard(n);
        bandBoard(n);
        restoreOpen(n);
        mountFrames(n);
        if (window.wbBindBoard) window.wbBindBoard(n);
        if (window.wbAgo) window.wbAgo(n);
        mermaid([n]);
      },
      drag: function (on) {
        if (on) return;
        var h = boardDragHeld;
        boardDragHeld = null;
        if (h) swap("board", h.hash, h.html, true);
      }
    });
    // wbForm creates wbHistory near the end of page startup. Mount overview
    // after that, not here while only the card framework exists.
    function mountBoardOverview() {
      if (!window.WsOverview || !window.wbHistory) return;
      window.WsOverview.mount({
      rid: rid, board: cardBoard, toolbar: window.WsCards.toolbar(), pins: window.WsCards.pinList(),
      keep: window.wbHistory.keep, place: window.WsCards.overview, toast: toast, rail: railKey,
      reveal: function () { if (live && !tabShown("board")) setTab("board"); },
      frames: function () { return boardRead(frameHeights); },
      restoreFrames: function (h) { var kept = boardKeep(); boardBoxes().forEach(function (b) { setFrameHeights(b, h); }); if (kept) kept.h.apply(); },
      changed: function () { doc.dispatchEvent(new Event("ws-overview")); },
      release: function () {
        var h = boardOverviewHeld;
        boardOverviewHeld = null;
        if (h) swap("board", h.hash, h.html, true);
      }
      });
    }
    if (window.WsTerm) window.WsTerm.mount($("bcol"));
    if (window.WsWeb) window.WsWeb.mount($("bcol"));
    if (PIN_WIDE && PIN_WIDE.addEventListener) PIN_WIDE.addEventListener("change", function () { window.WsCards.relayout(); });
  }
  boardBoxes().forEach(watchBoard);
  // morphBoard applies a board region event to el and returns the element
  // nodes it inserted, in order, for the caller to prepare.
  function morphBoard(el, html, frozen, pagedEdits) {
    var seen = {}, want = [], ins = [], live = {}, liveClosed = [], pages = [], more = null, grafts = [], changed = [];
    var pinBox = window.WsCards.pinList();
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n);
      if (k !== null) { live["t:" + k] = n; if (n.classList.contains("closed")) liveClosed.push(n); }
      else if (n.classList.contains("closed-page")) pages.push(n);
    });
    // The pinned topics (cards.js) stand in the pin list: atop the board or
    // in the pinned column.
    // A closed one the last board showed counts as the board's own closed
    // topic for the pages below (pinShift).
    var pinShift = [];
    if (pinBox) Array.prototype.forEach.call(pinBox.children, function (n) {
      var k = topicKey(n);
      if (k === null) return;
      live["t:" + k] = n;
      if (n.classList.contains("closed") && boardSeen["t:" + k] !== undefined) pinShift.push(n);
    });
    boardNodes(html, live).forEach(function (n) {
      var k = topicKey(n);
      if (k === null) {
        want.push(n);
        if (n.nodeType !== 1) return;
        ins.push(n);
        if (n.matches("button.closed-more[data-closed-next]")) more = n;
        return;
      }
      var h = topicSeen(n), key = "t:" + k;
      seen[key] = h;
      // A topic that closes while the page shows it folds once: the
      // server folds a closed topic's body and settled rows under the same
      // keys an open topic uses, so the user's earlier choices for them are
      // forgotten here, and later toggles are kept again.
      if (live[key] && n.classList.contains("closed") && !live[key].classList.contains("closed")) { forgetOpen(live[key]); forgetOpen(n); }
      if (live[key] && boardSeen[key] === h) { want.push(live[key]); delete live[key]; }
      else {
        var g = live[key] && frameGraftable(live[key], n);
        if (g) grafts.push(g);
        if (live[key]) changed.push({ old: live[key], n: n, g: g });
        want.push(n); ins.push(n);
      }
    });
    boardSeen = seen;
    // Closed topics fetched past the cap (closed pages, each nested in the
    // one before it) stay across swaps in the new control's place, since the
    // board renders only its capped section again. Closed-topic indices
    // follow close order, so a topic that newly closes pushes the oldest
    // capped one past the cap: that one moves to the head of the pages, a
    // fetched topic the new board shows itself leaves them, and the pages'
    // own control counts on from what is shown. No topic shows twice, and
    // none drops out between the capped section and the pages. A board with
    // no control holds every closed topic, and the pages go. A pinned topic
    // (cards.js) stays in the pin list: in the pages a slot (closed-slot)
    // stands in its place and counts as it would.
    if (pages.length && more) {
      var shown = {}, shifted = liveClosed.filter(function (n) { return !seen["t:" + topicKey(n)]; }), count = shifted.length;
      var slots = pinShift.filter(function (n) { return !seen["t:" + topicKey(n)]; });
      count += slots.length;
      Object.keys(seen).forEach(function (key) { shown[key] = true; });
      shifted.concat(slots).forEach(function (n) { shown["t:" + topicKey(n)] = true; });
      pages.forEach(function (p) {
        q(".closed-page > section.topic[data-topic], .closed-page > .closed-slot[data-slot]", p).forEach(function (t) {
          var key = "t:" + (t.getAttribute("data-topic") || t.getAttribute("data-slot"));
          if (shown[key]) t.parentNode.removeChild(t);
          else { shown[key] = true; count++; }
        });
      });
      var anchor = pages[0].firstChild;
      shifted.forEach(function (n) { pages[0].insertBefore(n, anchor); });
      slots.forEach(function (n) {
        var s = doc.createElement("span");
        s.className = "closed-slot";
        s.hidden = true;
        s.setAttribute("data-slot", topicKey(n));
        pages[0].insertBefore(s, anchor);
      });
      var tail = q("button.closed-more[data-closed-next]", pages[pages.length - 1]).pop();
      if (tail) {
        tail.setAttribute("data-closed-next", String(+more.getAttribute("data-closed-next") + count));
        tail.setAttribute("data-closed-more", String(Math.max(0, (+more.getAttribute("data-closed-more") || 0) - count)));
      }
      want.splice.apply(want, [want.indexOf(more), 1].concat(pages));
      ins.splice(ins.indexOf(more), 1);
    }
    // The page's own cards (cards.js) stand among the topics: they are
    // woven into the wanted nodes at their places, and the board settles
    // around them. Stale nodes go first, so the kept ones that stay in
    // order are never moved (moving a frame reloads it), and a card is
    // never moved at all. A changed topic whose old section stays in place
    // keeps that section and its body frame (frameGraft); where the browser
    // has Element.moveBefore, a kept node moves without reloading its
    // frames, so every changed topic keeps its old section (frameGrafts).
    // The pinned topics are woven into the pin list, where an update moves
    // none of them, so a changed one whose old section stands there keeps
    // it.
    var plan = window.WsCards.weave(want);
    (frozen || []).forEach(function (f) {
      var list = f.box === el ? plan.board : plan.pins;
      var staged = boardStageOrder(Array.prototype.slice.call(f.box.childNodes), list, f.nodes);
      if (f.box === el) plan.board = staged; else plan.pins = staged;
    });
    var woven = plan.board;
    // A changed topic's unchanged entry frames are kept, moved aside before
    // the grafts and the settle would detach them (frameKeep), and moved
    // into their placeholders' places once the board has settled (frameKept).
    var reuse = frameKeep(changed);
    var pinGrafts = grafts.filter(function (g) { return plan.pins.indexOf(g.n) >= 0 && g.old.parentNode === pinBox; });
    frameGrafts(el, woven, grafts.filter(function (g) { return plan.pins.indexOf(g.n) < 0; })).concat(pinGrafts).forEach(function (g) {
      frameGraft(g);
      var list = woven.indexOf(g.n) >= 0 ? woven : plan.pins;
      list[list.indexOf(g.n)] = g.old;
      ins[ins.indexOf(g.n)] = g.old;
    });
    window.WsCards.settle(el, plan);
    frameKept(reuse);
    (pagedEdits || []).forEach(function (edit) {
      var old = edit.live, n = edit.source.cloneNode(true), g = frameGraftable(old, n);
      var keptFrames = frameKeep([{ old: old, n: n, g: g }]);
      if (g) { frameGraft(g); n = old; }
      else old.parentNode.replaceChild(n, old);
      frameKept(keptFrames);
      ins.push(n);
    });
    return ins;
  }
  // --- incremental board inputs ---------------------------------------------
  // These are immutable incoming snapshots, not the live DOM. Hidden pages,
  // editing, overview and dragging may defer several events; the newest input
  // must include all earlier changes even though none has reached morphBoard.
  // Canonical nodes are parsed once, then cloned only for changed sections.
  function boardInputFull(html) {
    var nodes = Array.prototype.slice.call(frag(html).childNodes), topics = Object.create(null), order = [], before = [], after = [], past = false;
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i], key = topicKey(n);
      if (key === null) { (order.length ? after : before).push(n); if (order.length && n.nodeType === 1) past = true; continue; }
      if (past || !n.getAttribute("data-th") || Object.prototype.hasOwnProperty.call(topics, key)) return null;
      topics[key] = n; order.push(key);
    }
    return { topics: topics, order: order, before: before, after: after };
  }
  function boardInputPatch(prev, ev) {
    if (!prev || !ev || typeof ev.hash !== "string" || !Array.isArray(ev.order) || !Array.isArray(ev.topics)) return null;
    var topics = Object.create(null), changes = Object.create(null);
    for (var i = 0; i < ev.topics.length; i++) {
      var t = ev.topics[i];
      if (!t || typeof t.key !== "string" || typeof t.hash !== "string" || typeof t.html !== "string" || Object.prototype.hasOwnProperty.call(changes, t.key)) return null;
      var parsed = frag(t.html), n = parsed.firstElementChild;
      if (!n || parsed.childNodes.length !== 1 || topicKey(n) !== t.key || n.getAttribute("data-th") !== t.hash) return null;
      changes[t.key] = n;
    }
    for (var j = 0; j < ev.order.length; j++) {
      var key = ev.order[j];
      if (typeof key !== "string" || Object.prototype.hasOwnProperty.call(topics, key)) return null;
      var node = changes[key] || prev.topics[key];
      if (!node) return null;
      topics[key] = node;
    }
    if (Object.keys(changes).some(function (key) { return !Object.prototype.hasOwnProperty.call(topics, key); })) return null;
    return { topics: topics, order: ev.order.slice(), before: prev.before, after: prev.after };
  }
  function boardNodes(input, live) {
    if (typeof input === "string") return Array.prototype.slice.call(frag(input).childNodes);
    function clone(n) { return n.cloneNode(true); }
    return input.before.map(clone).concat(input.order.map(function (key) {
      var n = input.topics[key], old = live && live["t:" + key];
      return old && boardSeen["t:" + key] === topicSeen(n) ? old : clone(n);
    }), input.after.map(clone));
  }
  function boardInputFragment(input) {
    var f = doc.createDocumentFragment();
    boardNodes(input).forEach(function (n) { f.appendChild(n); });
    return f;
  }
  // --- keyed frame reuse: a changed topic keeps its unchanged entry frames --
  // An entry frame (data-doc-key entry:eN) is immutable per entry, so inside
  // a changed topic an incoming entry placeholder takes the old topic's
  // mounted frame of the same data-doc-key and the same data-srcdoc (the
  // same key need not mean the same document: image inlining and relay
  // lists change it); each old frame pairs once, so duplicate keys pair off
  // one by one. Only Element.moveBefore keeps a frame's document across a
  // move, and only between connected nodes: frameKeep moves the paired old
  // frames into a hidden holder on the page before anything detaches them,
  // and frameKept moves each into its placeholder's place, the placeholder
  // dropped, once the new section stands on the page. A placeholder that
  // stays unpaired is mounted as usual (mountFrames). A frame a graft holds
  // (frameGraft's chain) and the topic-body frame (topic:, a stable key over
  // changing content) are never paired. Without moveBefore nothing is kept.
  function frameKeep(changed) {
    if (!changed.length || !doc.body || !doc.body.moveBefore) return null;
    var pairs = [];
    changed.forEach(function (c) {
      var held = c.g ? c.g.oc[0] : null, olds = {};
      q('iframe.doc-frame[data-doc-key^="entry:"]:not(.doc-frame-next)', c.old).forEach(function (f) {
        if (f === held || !f.hasAttribute("data-srcdoc")) return;
        var k = f.getAttribute("data-doc-key") + "\n" + f.getAttribute("data-srcdoc");
        (olds[k] = olds[k] || []).push(f);
      });
      q('div.doc-frame[data-doc-key^="entry:"]', c.n).forEach(function (p) {
        if (c.g && p === c.g.nc[0]) return;
        var list = olds[p.getAttribute("data-doc-key") + "\n" + p.getAttribute("data-srcdoc")];
        if (list && list.length) pairs.push({ f: list.shift(), p: p });
      });
    });
    if (!pairs.length) return null;
    var holder = doc.createElement("div");
    holder.hidden = true;
    doc.body.appendChild(holder);
    pairs.forEach(function (x) { holder.moveBefore(x.f, null); });
    return { holder: holder, pairs: pairs };
  }
  function frameKept(r) {
    if (!r) return;
    r.pairs.forEach(function (x) {
      var at = x.p.parentNode;
      if (!at || !x.p.isConnected) return;
      at.moveBefore(x.f, x.p);
      at.removeChild(x.p);
    });
    r.holder.parentNode.removeChild(r.holder);
  }
  // --- a changed topic's body frame, double-buffered --------------------------
  // A changed topic's new section would bring a new body frame, blank until
  // its document loads and reports a height. Instead, when the topic stays
  // open and its old section would stay where it stands, or the browser
  // can move it without reloading its frame (frameGrafts), the old section
  // is kept: its content becomes the new section's, except the
  // path down to the old body frame, and the new frame loads beside the old
  // one, unseen and out of the layout (.doc-frame-next), at the old one's
  // height. Once the new frame has loaded and drawn, it takes the old one's
  // place in the same frame (frameShow, frameReveal); one that never loads
  // shows after frameRevealMs anyway.
  var frameRevealMs = 1500;
  // frameChain is the path from frame f up to section sec, f first, or null.
  function frameChain(f, sec) {
    var c = [];
    for (var x = f; x && x !== sec; x = x.parentNode) c.push(x);
    return x === sec ? c.concat(sec) : null;
  }
  // frameGraftable returns the graft of new section n into old section old
  // (both open, each with a body frame of the same topic document on paths
  // of the same shape), or null.
  function frameGraftable(old, n) {
    if (old.classList.contains("closed") || n.classList.contains("closed")) return null;
    var of = q(".doc-frame:not(.doc-frame-next)", old)[0], nf = q(".doc-frame", n)[0];
    if (!of || !nf || of.getAttribute("data-doc") !== nf.getAttribute("data-doc")) return null;
    var oc = frameChain(of, old), nc = frameChain(nf, n);
    if (!oc || !nc || oc.length !== nc.length) return null;
    // The frames themselves may differ in kind (a mounted iframe, an
    // incoming placeholder: frameGraft mounts it).
    for (var i = 1; i < oc.length; i++) if (oc[i].tagName !== nc[i].tagName) return null;
    return { old: old, n: n, oc: oc, nc: nc };
  }
  // frameGrafts returns the grafts that move nothing settling el into want
  // would not move anyway, each graft's new section standing for its old
  // one: moving a frame reloads it, so a topic whose old section would move
  // (an update moves its topic to the head), or would push kept topics
  // around it, takes its new section as before. When the grafts together
  // move something, each one that alone moves nothing is kept, if those
  // together still move nothing. Where the browser has Element.moveBefore,
  // settle moves a kept node without reloading its frames (cards.js), so
  // every graft is kept, whatever moves.
  function frameGrafts(el, want, grafts) {
    if (!grafts.length || el.moveBefore) return grafts;
    var base = settleMoves(el, want);
    function still(gs) {
      var by = new Map(gs.map(function (g) { return [g.n, g.old]; })), ok = true;
      settleMoves(el, want.map(function (n) { return by.get(n) || n; })).forEach(function (n) { if (!base.has(n)) ok = false; });
      return ok;
    }
    if (still(grafts)) return grafts;
    var each = grafts.filter(function (g) { return still([g]); });
    return each.length && still(each) ? each : [];
  }
  // settleMoves returns the nodes already in el that settling el into want
  // moves. It replays WsCards.settle (cards.js) on a list: keep the two in
  // step.
  function settleMoves(el, want) {
    var keep = new Set(want), moved = new Set(), i = 0;
    var cur = Array.prototype.filter.call(el.childNodes, function (n) { return keep.has(n); });
    want.forEach(function (n) {
      if (cur[i] === n) { i++; return; }
      var j = cur.indexOf(n);
      if (j >= 0 && n.nodeType === 1 && n.hasAttribute("data-card")) {
        var past = cur.splice(i, j - i);
        past.forEach(function (p) { moved.add(p); });
        cur.splice.apply(cur, [i + 1, 0].concat(past));
      } else {
        if (j >= 0) { moved.add(n); cur.splice(j, 1); }
        cur.splice(i, 0, n);
      }
      i++;
    });
    return moved;
  }
  // frameGraft moves g's new section content into its old section, level by
  // level down the frame's path: each kept element takes the new element's
  // attributes and children, except its child on the path, which stays.
  // At the bottom the new frame goes in after the old one, pending.
  function frameGraft(g) {
    var of = g.oc[0], mounted = of.tagName === "IFRAME";
    // The next frame of a mounted frame is mounted before it goes in, and
    // before anything holds it (expandos, the load listener): a placeholder
    // would show nothing to reveal. The next frame of an unmounted one (a
    // closed fold, a topic far from the view) is a placeholder that simply
    // takes the old one's place: the topic mounts it once it comes near
    // (viewport bands).
    if (mounted && g.nc[0].tagName !== "IFRAME") g.nc[0] = mountFrame(g.nc[0]);
    var nf = g.nc[0];
    if (mounted) frameNext(nf, of);
    for (var i = g.oc.length - 1; i > 0; i--) {
      var oa = g.oc[i], na = g.nc[i], oc = g.oc[i - 1], nc = g.nc[i - 1], before = true;
      Array.prototype.slice.call(oa.attributes).forEach(function (a) { if (!na.hasAttribute(a.name)) oa.removeAttribute(a.name); });
      Array.prototype.forEach.call(na.attributes, function (a) { if (oa.getAttribute(a.name) !== a.value) oa.setAttribute(a.name, a.value); });
      Array.prototype.slice.call(oa.childNodes).forEach(function (c) { if (c !== oc) oa.removeChild(c); });
      Array.prototype.slice.call(na.childNodes).forEach(function (c) {
        if (c === nc) { before = false; if (c === nf) oa.appendChild(c); }
        else if (before) oa.insertBefore(c, oc);
        else oa.appendChild(c);
      });
    }
    if (!mounted) of.parentNode.removeChild(of);
  }
  // frameNext makes nf the pending next frame of mounted frame of.
  function frameNext(nf, of) {
    nf.classList.add("doc-frame-next");
    nf.setAttribute("tabindex", "-1");
    nf.setAttribute("aria-hidden", "true");
    nf.wbNext = true;
    nf.wbOld = of;
    of.wbOut = true;
    nf.addEventListener("load", function () { frameShow(nf); });
    setTimeout(function () { frameReveal(nf); }, frameRevealMs);
  }
  // frameShow reveals pending frame f once it has loaded and drawn: a
  // frame revealed in the frame it loads in can still show blank for a
  // frame, so the reveal waits two frames more.
  function frameShow(f) {
    if (f.wbShowing) return;
    f.wbShowing = true;
    requestAnimationFrame(function () { requestAnimationFrame(function () { frameReveal(f); }); });
  }
  // frameReveal shows pending frame f in its old frame's place, at the old
  // frame's height, and asks it for its height, which the frame message
  // handler sets (easing to it in a moving swap, wbGrow). A frame no longer
  // on the page (a later swap replaced it) is left alone.
  function frameReveal(f) {
    if (!f.wbNext || !f.isConnected) return;
    var w = window.wbHistory, kept = w && w.keep ? w.keep(f) : null;
    f.wbNext = false;
    f.classList.remove("doc-frame-next");
    f.removeAttribute("tabindex");
    f.removeAttribute("aria-hidden");
    var o = f.wbOld;
    f.wbOld = null;
    if (o && o.parentNode) o.parentNode.removeChild(o);
    if (kept) kept.apply();
    postDoc(f, { wbDoc: "size" });
  }
  // Board motion: a board swap's inserted topic sections grow from the
  // height they had before (a topic the board did not hold grows from 0)
  // to their natural height, and every topic section and card standing in
  // the board slides from where it stood to where it stands now (topics
  // are ordered by their last update, so an update moves its topic to the
  // head and shifts the rest). The spots are read before the morph; a
  // section still animating reads at its rendered height and place, so a
  // new swap starts from there. Topics in the closed pages (moved there or
  // out of there by the closed-page reconciliation) do not move. A changed
  // topic's frame starts at its old height and eases to the first height
  // it reports (the frame message handler, wbGrow).
  function reducedMotion() { return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches); }
  // boardSpots reads each topic section's height and offset from the
  // board's top, keyed "t:" + its topic, and each card's offset, keyed
  // "c:" + its id. A pinned topic (the pin list, atop the board or in the
  // pinned column) counts as paged: an update never moves it.
  function boardSpots(el) {
    var h = {}, top = el.getBoundingClientRect().top;
    boardQ("section.topic[data-topic]").forEach(function (n) {
      var r = n.getBoundingClientRect();
      h["t:" + n.getAttribute("data-topic")] = { h: r.height, y: r.top - top, paged: n.parentNode !== el };
    });
    Array.prototype.forEach.call(el.children, function (n) {
      if (n.hasAttribute("data-card")) h["c:" + n.getAttribute("data-card")] = { y: n.getBoundingClientRect().top - top };
    });
    return h;
  }
  // topicMotion returns, in order, the inserted topic sections that animate
  // and the height each starts from. A large topic (cards.js) does not
  // grow: its height is the sheet's.
  function topicMotion(before, ins) {
    var out = [];
    ins.forEach(function (n) {
      var k = topicKey(n);
      if (k === null || n.hasAttribute("data-max")) return;
      var b = before["t:" + k];
      if (b && b.paged) return;
      out.push({ n: n, from: b ? b.h : 0 });
    });
    return out;
  }
  // growTopic runs after the caller prepared the section (frame heights
  // restored), so its natural height is the target. The animation fills
  // nothing: once done the section has no fixed height, and a later size
  // change (a frame posting its height, a details toggle) is never clipped.
  // A section clips its overflow itself (.topic), and a new one also fades
  // in with its bottom margin growing.
  function growTopic(n, from) {
    if (!n.animate) return null;
    var to = n.getBoundingClientRect().height;
    if (Math.abs(to - from) < 1) return null;
    var a = { height: from + "px" }, b = { height: to + "px" };
    if (!from) {
      a.opacity = 0; b.opacity = 1;
      a.marginBottom = "0px"; b.marginBottom = getComputedStyle(n).marginBottom;
    }
    return n.animate([a, b], { duration: 200, easing: "ease-out" });
  }
  // slideBoard moves each topic section and card standing in the board from
  // its spot before the swap to its place now (FLIP): the layout is
  // already final, and a transform that starts at the old offset eases to
  // none. It runs after the growth started, so the places it reads are the
  // growth's first frame and slide and growth add up to one movement.
  // Nothing moves in the DOM (moving a frame reloads it), and the
  // animation fills nothing, so no transform stays behind. A maximized card,
  // a large topic, and a topic holding a large panel stay put: a transform
  // would anchor the fixed box to the moving node.
  // With from, the board's top when the spots were read, a slide runs in
  // screen terms (a swap compensated the scroll between, boardKeep):
  // an element standing where it stood on screen does not move.
  function slideBoard(el, before, from) {
    var top = el.getBoundingClientRect().top, shift = typeof from === "number" ? from - top : 0, out = [];
    Array.prototype.forEach.call(el.children, function (n) {
      var k = topicKey(n), card = k === null && n.hasAttribute("data-card");
      var b = k !== null ? before["t:" + k] : card ? before["c:" + n.getAttribute("data-card")] : null;
      if (!b || b.paged || !n.animate || n.hasAttribute("data-max") || (k !== null && n.querySelector("details[data-max]"))) return;
      var dy = b.y + shift - (n.getBoundingClientRect().top - top);
      if (Math.abs(dy) < 1) return;
      out.push(n.animate([{ transform: "translateY(" + dy + "px)" }, { transform: "none" }], { duration: 200, easing: "ease-out" }));
    });
    return out;
  }
  // startMotion starts the swap's animations at the swap's first frame. An
  // animation's start time is the timeline's time, which holds still at the
  // last frame's through the tasks after it, up to the next frame's own
  // callbacks; a board swap is a long task (a large board's parse and
  // morph, its new topics' frames), so started as created the animations
  // were over when the swap's first frame drew: it showed the old heights
  // (a closing topic's whole open height, empty, and the board under it
  // blank) and the next frame jumped to the end. The start is set to the
  // clock at the first frame's callbacks instead, and until then (that
  // frame itself, whose timeline time is older) the animations hold their
  // first keyframe.
  function startMotion(list) {
    list = list.filter(Boolean);
    if (!list.length) return;
    list.forEach(function (a) { a.effect.updateTiming({ fill: "backwards" }); });
    requestAnimationFrame(function () {
      var t = performance.now();
      list.forEach(function (a) { if (a.playState === "running") a.startTime = t; });
    });
  }
  // Short owner captures, once per actual scroller, including independent
  // pins. No post-morph target selection or ordinary resize following.
  function boardKeep(nodes) {
    var w = window.wbHistory, handles = [], scrollers = new Set();
    if (!w || !w.keep) return null;
    (nodes || boardBoxes().map(function (b) { return b.firstElementChild || b; })).forEach(function (n) {
      var inspect = w.keep(n, { capture: false });
      if (scrollers.has(inspect.scroller)) return;
      scrollers.add(inspect.scroller);
      handles.push(w.keep(n));
    });
    return { h: { apply: function () { handles.forEach(function (h) { h.apply(); }); }, end: function () { handles.forEach(function (h) { h.end(); }); } } };
  }
  // A staging pass reads exposure through one wbHistory.snapshot: nothing
  // writes the DOM inside it, so each scroller's viewport (a scan of every
  // sticky head) is measured once rather than once per node checked.
  function boardWithSnap(fn) {
    var outer = boardSnap, w = window.wbHistory;
    if (!outer && w && w.snapshot) boardSnap = w.snapshot();
    try { return fn(); } finally { boardSnap = outer; }
  }
  function boardExposed(n) {
    if (!n || n.isConnected === false) return false;
    if (boardSnap) return boardSnap.visible(n);
    var w = window.wbHistory;
    return !!(w && w.keep && w.keep(n, { capture: false }).visible(n));
  }
  function boardScroller(n) { return boardSnap ? boardSnap.scroller(n) : window.wbHistory.keep(n, { capture: false }).scroller; }
  // Freeze the interval, not just the reading reference. A single scroll
  // delta cannot repair an insertion/reorder between two visible units.
  // Units outside the interval retain canonical order and render normally.
  function boardStageOrder(current, want, exposed) {
    var first = current.findIndex(function (n) { return exposed.indexOf(n) >= 0; });
    if (first < 0) return want;
    var last = current.length - 1;
    while (exposed.indexOf(current[last]) < 0) last--;
    var block = current.slice(first, last + 1).map(function (n) {
      var k = topicKey(n);
      return k === null ? n : want.find(function (w) { return topicKey(w) === k; }) || n;
    }), held = new Set(block);
    var index = want.findIndex(function (n) { return held.has(n); });
    if (index < 0) index = Math.min(first, want.length);
    var out = want.filter(function (n) { return !held.has(n); });
    index = want.slice(0, index).filter(function (n) { return !held.has(n); }).length;
    out.splice.apply(out, [index, 0].concat(block));
    if (out.length !== want.length || out.some(function (n, i) { return n !== want[i]; })) {
      block.concat(want).forEach(function (n) {
        var k = topicKey(n);
        if (k !== null && out.indexOf(n) !== want.indexOf(n)) boardPending[k] = true;
      });
    }
    return out;
  }
  // Canonical panel/article bytes, including inert history pages. Live DOM
  // is used only for exposure/identity, never as an incoming snapshot.
  function boardPanelNodes(n) {
    var nodes = q(".entry-panel, .wb-entry:not(.panel), .doc-frame", n).filter(function (p) {
      return !p.classList.contains("doc-frame") || !p.closest(".entry-panel");
    });
    q("template", n).forEach(function (t) { if (t.content) nodes = nodes.concat(boardPanelNodes(t.content)); });
    return nodes;
  }
  // The mounted panels only: inert history pages are never exposed.
  function boardLivePanels(n) {
    return q(".entry-panel, .wb-entry:not(.panel), .doc-frame", n).filter(function (p) {
      return !p.classList.contains("doc-frame") || !p.closest(".entry-panel");
    });
  }
  function boardPanelKey(n) { return n.id || n.getAttribute("data-key") || n.getAttribute("data-entry") || n.getAttribute("data-doc-key"); }
  function boardPanelMap(list) {
    var m = new Map();
    list.forEach(function (p) { var k = boardPanelKey(p); if (!m.has(k)) m.set(k, p); });
    return m;
  }
  // The exposed panels on scroller, in document order, once per snapshot.
  function boardViewedPanels(scroller) {
    var memo = boardSnap && (boardSnap.viewed = boardSnap.viewed || new Map());
    if (memo && memo.has(scroller)) return memo.get(scroller);
    var list = boardLivePanels(scroller).filter(function (p) { return boardExposed(p) && boardScroller(p) === scroller; });
    if (memo) memo.set(scroller, list);
    return list;
  }
  function boardRenderBlockers(live, old, incoming) {
    var exposed = boardLivePanels(live).filter(boardExposed);
    if (!incoming) return exposed.length ? exposed : boardExposed(live) ? [live] : [];
    var before = boardPanelNodes(old), after = boardPanelNodes(incoming), was = boardPanelMap(before), now = boardPanelMap(after);
    var changed = exposed.filter(function (p) {
      var key = boardPanelKey(p), a = was.get(key), b = now.get(key);
      return !a || !b || a.outerHTML !== b.outerHTML;
    });
    // The frozen interval crosses topic boundaries, but never scrollers.
    // Extend this topic's canonical span toward other exposed units.
    var leading = false, trailing = false;
    if (exposed.length) {
      var viewed = boardViewedPanels(boardScroller(live));
      var first = viewed.indexOf(exposed[0]), last = viewed.indexOf(exposed[exposed.length - 1]);
      leading = first > 0;
      trailing = last >= 0 && last < viewed.length - 1;
    }
    if (exposed.length > 1 || leading || trailing) {
      function span(list) {
        var keys = exposed.map(boardPanelKey), indices = list.map(function (p, i) { return keys.indexOf(boardPanelKey(p)) >= 0 ? i : -1; }).filter(function (i) { return i >= 0; });
        return indices.length ? list.slice(leading ? 0 : Math.min.apply(Math, indices), trailing ? list.length : Math.max.apply(Math, indices) + 1).map(function (p) { return p.outerHTML; }).join("\n") : "";
      }
      if (span(before) !== span(after)) return exposed;
    }
    return changed;
  }
  function boardRecordPending() {
    boardChanges = boardChanges.filter(function (c) { return c.names.some(function (k) { return boardPending[k]; }); });
    if (boardRelease) { boardChanges = []; return; }
    var offer = boardOffered;
    boardOffered = null;
    if (!offer || !offer.input) return;
    var previous = offer.previous, input = offer.input, content = [];
    Object.keys(boardPending).forEach(function (k) {
      var old = previous && previous.topics[k], next = input.topics[k];
      if (!old || !next || topicSeen(old) !== topicSeen(next)) content.push(k);
    });
    content.forEach(function (k) { boardChanges.push({ names: [k] }); });
    if (!content.length && previous && previous.order.join("\n") !== input.order.join("\n") && Object.keys(boardPending).length)
      boardChanges.push({ names: Object.keys(boardPending) });
  }
  // A send grants no blanket exemption to its topic: a simultaneous agent
  // rewrite is still external. Server-rendered owner receipts identify the
  // new rows. Admit them alone when the same snapshot also changes panels.
  function boardOwnerSnapshot(old, next) {
    if (!old || !next) return null;
    var known = new Set(q('.wb-entry.owner', old).map(function (n) { return n.id; }));
    var arrivals = q('.wb-entry.owner', next).filter(function (n) { return !known.has(n.id); });
    if (!arrivals.length) return null;
    function external(n) { return !n.classList.contains('owner'); }
    var before = boardPanelNodes(old).filter(external), after = boardPanelNodes(next).filter(external);
    var changed = before.length !== after.length || before.some(function (n, i) { return boardPanelKey(n) !== boardPanelKey(after[i]) || n.outerHTML !== after[i].outerHTML; });
    if (!changed) return { source: next, external: false };
    var partial = old.cloneNode(true), admitted = [];
    arrivals.forEach(function (n) {
      var h = n.closest('.wb-history'), key = h && h.getAttribute('data-history');
      var target = q('.wb-history', partial).find(function (p) { return p.getAttribute('data-history') === key; });
      if (target) { target.querySelector('.history-live').appendChild(n.cloneNode(true)); admitted.push(n.id); }
    });
    if (!admitted.length) return null;
    partial.setAttribute('data-th', topicSeen(old) + ':owner:' + admitted.join(','));
    return { source: partial, external: true };
  }
  // Fetched closed-page topics are not canonical top-level members yet.
  // Keep an exposed transfer in its page while admitting eligible rendering
  // in place. The original immutable fetched bytes, not mounted frame DOM,
  // are the diff source. Placement releases when that card fully leaves.
  function boardStagePaged(input, topics, order) {
    boardPagedEdits = [];
    boardBoxes().forEach(function (box) {
      q('.closed-page section.topic[data-topic]', box).forEach(function (live) {
        var k = topicKey(live), next = topics[k], old = boardPaged[k];
        if (!next || boardRelease || (boardImmediate[k] && !boardPending[k]) || !boardExposed(live)) return;
        var blockers = old ? boardRenderBlockers(live, old, next) : [live];
        if (!blockers.length && topicSeen(old) !== topicSeen(next)) {
          boardPagedEdits.push({ live: live, source: next });
          boardPaged[k] = next;
        }
        delete topics[k];
        var at = order.indexOf(k); if (at >= 0) order.splice(at, 1);
        boardPending[k] = blockers.length ? blockers : [live];
      });
    });
  }
  function boardStageInput(input) { return boardWithSnap(function () { return boardStageSnap(input); }); }
  function boardStageSnap(input) {
    boardPending = Object.create(null);
    boardImmediate = Object.create(null);
    boardFrozen = [];
    if (!input || typeof input === "string") return input;
    var topics = Object.assign(Object.create(null), input.topics), order = input.order.slice();
    Object.keys(boardOwn).forEach(function (k) {
      var old = boardRendered && boardRendered.topics[k] || boardPaged[k];
      var own = boardOwnerSnapshot(old, topics[k]);
      if (!own) return;
      topics[k] = own.source; boardImmediate[k] = true; delete boardOwn[k];
      if (own.external) boardPending[k] = true;
    });
    boardBoxes().forEach(function (box) {
      var nodes = Array.prototype.filter.call(box.children, function (n) {
        var k = topicKey(n);
        var group = n.classList.contains('closed-page') || n.classList.contains('bpins');
        return (k !== null || n.hasAttribute("data-card") || group) && !boardRelease && !boardImmediate[k] && boardExposed(n);
      });
      if (nodes.length) boardFrozen.push({ box: box, nodes: nodes });
      nodes.forEach(function (n) {
        var k = topicKey(n), old = boardRendered && boardRendered.topics[k];
        if (k === null || !old || (topics[k] && topicSeen(topics[k]) === topicSeen(old))) return;
        var blockers = boardRenderBlockers(n, old, topics[k]);
        if (!blockers.length) return;
        topics[k] = old;
        if (order.indexOf(k) < 0) order.push(k);
        boardPending[k] = blockers;
      });
    });
    boardStagePaged(input, topics, order);
    // Inline pins share the main scroller but still have their own placement
    // list. Their content follows the same per-panel rendering blockers.
    var pins = window.WsCards && window.WsCards.pinList && window.WsCards.pinList();
    if (pins && boardBoxes().indexOf(pins) < 0 && !boardRelease) {
      var heldPins = Array.from(pins.children).filter(function (n) { return !boardImmediate[topicKey(n)] && boardExposed(n); });
      if (heldPins.length) boardFrozen.push({ box: pins, nodes: heldPins });
      heldPins.forEach(function (n) {
        var k = topicKey(n), old = boardRendered && boardRendered.topics[k];
        if (k === null || !old || !topics[k] || topicSeen(old) === topicSeen(topics[k])) return;
        var blockers = boardRenderBlockers(n, old, topics[k]);
        if (blockers.length) { topics[k] = old; boardPending[k] = blockers; }
      });
    }
    // A partial input records only bytes actually admitted, never held hashes.
    boardRendered = { topics: topics, order: order, before: input.before, after: input.after };
    return boardRendered;
  }
  function boardPendingBadge() {
    var names = Object.keys(boardPending), count = boardChanges.length;
    if (!boardBadge && names.length) {
      boardBadge = doc.createElement("button");
      boardBadge.type = "button";
      boardBadge.className = "board-viewed-pending";
      boardBadge.addEventListener("click", function () { boardFlushViewed(true); });
      doc.body.appendChild(boardBadge);
    }
    if (!boardBadge) return;
    boardBadge.hidden = !names.length;
    boardBadge.textContent = "최신 변경 " + count + "건 반영 · " + names.join(", ");
    boardBadge.title = names.join(", ");
  }
  function boardFlushViewed(explicit) {
    boardReleaseFrame = 0;
    if (!Object.keys(boardPending).length) return;
    boardRelease = !!explicit;
    try {
      if (boardLatest) swap("board", boardLatest.hash, boardLatest.html, true);
      boardPendingBadge();
    } finally { boardRelease = false; }
  }
  // The exposed units a staged release can depend on: top-level topics,
  // cards and groups, the pin list's topics and fetched closed-page topics.
  function boardExposureUnits() {
    var pins = window.WsCards && window.WsCards.pinList && window.WsCards.pinList(), units = [], panels = [];
    boardBoxes().concat(pins && boardBoxes().indexOf(pins) < 0 ? [pins] : []).forEach(function (box) {
      units = units.concat(Array.prototype.slice.call(box.children), q(".closed-page section.topic[data-topic]", box));
      panels = panels.concat(boardLivePanels(box));
    });
    // Exposed panels set the frozen interval across topics (the leading and
    // trailing span in boardRenderBlockers), so another topic's panel leaving
    // can release a block whose own panels are all still in view.
    return units.concat(panels).filter(boardExposed);
  }
  // A scroll or resize re-stages only when a release can follow: a pending
  // panel left the view (its topic's rendering may release), or the exposed
  // unit or panel set changed (a frozen interval or staged placement may
  // release).
  // Re-staging on every scroll frame repeated a whole board swap per frame
  // while anything was pending.
  function boardReleaseDue() {
    return boardWithSnap(function () {
      var gone = Object.keys(boardPending).some(function (k) {
        var b = boardPending[k];
        return Array.isArray(b) && b.some(function (n) { return !boardExposed(n); });
      });
      if (gone || !boardExposedUnits) return true;
      var units = boardExposureUnits();
      return units.length !== boardExposedUnits.length || units.some(function (n, i) { return n !== boardExposedUnits[i]; });
    });
  }
  function boardViewedScroll() {
    if (!boardReleaseFrame && Object.keys(boardPending).length) boardReleaseFrame = requestAnimationFrame(function () {
      boardReleaseFrame = 0;
      if (Object.keys(boardPending).length && boardReleaseDue()) boardFlushViewed(false);
    });
  }
  doc.addEventListener("scroll", boardViewedScroll, { capture: true, passive: true });
  window.addEventListener("resize", boardViewedScroll);
  // watchBoard puts the board's topic sections under root (root itself
  // included) under the shared visibility source (wbHistory.watch), once
  // each; a swap sweeps the ones it removed. wbHistory.seen reads their
  // reports.
  function watchBoard(root) {
    if (!window.wbHistory || !window.wbHistory.watch || !root || root.nodeType !== 1) return;
    var list = root.matches("section.topic[data-topic]") ? [root] : q("section.topic[data-topic]", root);
    list.forEach(function (n) { if (!n.wbWatch) n.wbWatch = window.wbHistory.watch(n); });
  }

