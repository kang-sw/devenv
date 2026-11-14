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
    if (el.getAttribute("data-h") === hash) { if (board) deferred = null; if (name === "rail" && railGate) railGate.forget(); return; }
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
    if (board) deferred = null;
    var pin = (name === "entries" || name === "convlive") && pinned();
    var scroll = scrollState();
    var answers = board ? boardRead(readAnswers) : null;
    var railTop = name === "rail" ? railScrollTop() : null, railFoc = name === "rail" ? railFocus() : null;
    var frameH = board ? boardRead(frameHeights) : null;
    var history = board ? captureBoardHistory() : null;
    var before = board && !still && !reducedMotion() ? boardSpots(el) : null;
    var kept = board ? boardKeep(el) : null, boardTop = before && kept ? el.getBoundingClientRect().top : null;
    // The entries merge and the board morph prepare only the nodes they
    // insert; any other region is replaced whole. A finished reply's mermaid
    // fences draw once merged; the streaming reply (convlive) keeps them
    // code blocks.
    var ins = null;
    if (name === "entries") mergeEntriesRegion(el, html, pin);
    else if (board) {
      ins = morphBoard(el, html);
      if (kept) boardKeepSettle(kept, el, ins);
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
        // A kept board slides in screen terms, once the scroll is
        // compensated: what the session holds in place does not slide.
        if (kept) kept.h.apply();
        startMotion(moves.concat(slideBoard(el, before, kept && window.wbHistory.keeping(kept.h.scroller) ? boardTop : null)));
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
  var boardSeen = {};
  function topicKey(n) {
    return n.nodeType === 1 && n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic") ? n.getAttribute("data-topic") : null;
  }
  function topicSeen(n) {
    return n.getAttribute("data-th") || n.outerHTML;
  }
  (function () {
    var b = region("board");
    if (b) Array.prototype.forEach.call(b.children, function (n) { var k = topicKey(n); if (k !== null) boardSeen["t:" + k] = topicSeen(n); });
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
        search: function (e) { var b = $("tkqs"); if (b) { e.stopPropagation(); b.click(); } },
        files: fpane && window.WsFiles ? revealPanes : null
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
      restoreFrames: function (h) { boardBoxes().forEach(function (b) { setFrameHeights(b, h); }); },
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
  // --- the Files pane (files.js): mounted once, loaded when first shown -------
  if (live && fpane && window.WsFiles) {
    window.WsFiles.mount(fpane, {
      rid: rid, load: load, save: save, post: post, toast: toast, copy: copyText,
      rootSlot: chead && chead.querySelector(".fsum"),
      local: function () { return root.hasAttribute("data-sysopen"); }
    });
  }
  // morphBoard applies a board region event to el and returns the element
  // nodes it inserted, in order, for the caller to prepare.
  function morphBoard(el, html) {
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
    var plan = window.WsCards.weave(want), woven = plan.board;
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
    f.wbNext = false;
    f.classList.remove("doc-frame-next");
    f.removeAttribute("tabindex");
    f.removeAttribute("aria-hidden");
    var o = f.wbOld;
    f.wbOld = null;
    if (o && o.parentNode) o.parentNode.removeChild(o);
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
  // Board scroll compensation: a board swap keeps what the user reads where
  // it stands. The anchor is the topmost topic or card in view that the
  // swap does not move (a held topic before any), and one wbHistory.keep
  // session holds its offset from the board's scroller through the swap and
  // the layout changes after it (growth, late frame heights, restored
  // history pages), until they settle or the user scrolls. A maximized card
  // is left alone, and so is a board column not shown.
  // boardView lists the topics and cards in view, topmost first, each with
  // its top in layout terms (a running slide's transform taken out) and
  // its parent, before the morph.
  function layoutY(n) {
    var m = /^matrix(3d)?\((.*)\)$/.exec(getComputedStyle(n).transform || ""), v = m ? m[2].split(",").map(parseFloat) : null;
    return n.getBoundingClientRect().top - (v ? (m[1] ? v[13] : v[5]) || 0 : 0);
  }
  function boardView(el) {
    var c = $("bcol").getBoundingClientRect(), top = Math.max(0, c.top), bottom = Math.min(window.innerHeight, c.bottom);
    return q("section.topic[data-topic], [data-card]", el).filter(function (n) {
      var r = n.getBoundingClientRect();
      return r.height > 0 && r.bottom > top && r.top < bottom;
    }).map(function (n) { return { n: n, y: layoutY(n), parent: n.parentNode }; }).sort(function (a, b) { return a.y - b.y; });
  }
  // boardAnchor picks the anchor from view: the first held entry, else the
  // first one moving(entry) does not claim, else null.
  function boardAnchor(view, moving) {
    var held = view.filter(function (c) { return c.held; })[0];
    return held || view.filter(function (c) { return !moving(c); })[0] || null;
  }
  // boardKeep starts the session on the topmost entry in view, before the
  // morph, and returns it with the view, each entry's offset from the
  // scroller's top; boardKeepSettle moves it, after the morph, to the
  // anchor, at that entry's offset from before, or ends it when the swap
  // moved everything in view. A moving entry is one the morph inserted or
  // kept as a changed topic's section (ins), took off the board, or moved
  // to another parent.
  function boardKeep(el) {
    if (!window.wbHistory || !window.wbHistory.keep || !visible($("bcol")) || doc.querySelector("[data-card][data-max]")) return null;
    var view = boardView(el);
    if (!view.length) return null;
    var h = window.wbHistory.keep(view[0].n, { observe: [el] }), s = h.scroller;
    var top = s === doc.scrollingElement ? 0 : s.getBoundingClientRect().top + s.clientTop;
    view.forEach(function (c) { c.off = c.y - top; });
    return { h: h, view: view };
  }
  // boardKept returns the session running on the board column's scroller,
  // in boardKeep's shape, or null.
  function boardKept() {
    var w = window.wbHistory, h = w && w.keeping ? w.keeping($("bcol")) || w.keeping(doc.scrollingElement) : null;
    return h ? { h: h } : null;
  }
  function boardKeepSettle(k, el, ins) {
    var moved = new Set(ins);
    var pick = boardAnchor(k.view, function (c) { return moved.has(c.n) || !el.contains(c.n) || c.n.parentNode !== c.parent; });
    if (!pick) k.h.end();
    else if (pick !== k.view[0]) window.wbHistory.keep(pick.n, { offset: pick.off });
  }
  // watchBoard puts the board's topic sections under root (root itself
  // included) under the shared visibility source (wbHistory.watch), once
  // each; a swap sweeps the ones it removed. wbHistory.seen reads their
  // reports.
  function watchBoard(root) {
    if (!window.wbHistory || !window.wbHistory.watch || !root || root.nodeType !== 1) return;
    var list = root.matches("section.topic[data-topic]") ? [root] : q("section.topic[data-topic]", root);
    list.forEach(function (n) { if (!n.wbWatch) n.wbWatch = window.wbHistory.watch(n); });
  }

