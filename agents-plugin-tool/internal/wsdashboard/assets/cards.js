(function () {
  // The board card framework (window.WsCards): cards the page itself puts in
  // the board column, among the board's topics. The framework owns what
  // every card kind shares: the header's pin, fold, raise, maximize, and close
  // buttons, the body's base height and its drag handle, the maximize sheet
  // with Escape (which also draws a topic or a topic's panel large), where
  // a card stands in the board flow, and the record of
  // all of that in browser storage. A kind supplies the header's content,
  // its own header actions, the body, the folded content, and its
  // lifecycle, and never places or sizes a card itself.
  //
  // A kind registers before the page mounts the framework:
  //
  //   WsCards.register(name, {
  //     base: 260,            // the body's base height in px, or a function
  //                           // of the card that returns it (optional)
  //     open(card) {},        // build card.body, call card.title(...)
  //     closing(card) {},     // the close button was pressed: false keeps
  //                           // the card, and the kind may close it later
  //                           // (optional)
  //     close(card) {},       // the card is going away (optional)
  //     folded(card) {},      // a node shown under a folded header, or null (optional)
  //     sized(card) {},       // the body's size changed (optional)
  //     pinned(card) {},      // the card was pinned or unpinned (optional)
  //   })
  //
  // and works through the card it is handed: card.data (the kind's own
  // record, kept in storage: plain JSON), card.body, card.title, card.actions,
  // card.save, card.bump, card.show, card.fold, card.close, card.pinned, and
  // card.paint (draws the view state again: a kind whose base height
  // changed calls it).
  //
  // The board's topics take part in the same placement as the cards. A
  // topic is the board's own node (the whiteboard's section, never rebuilt
  // here); the framework adds a pin button to its title bar, and a topic is
  // pinned, unpinned, and dragged like a card. Only a topic pinned or moved
  // by hand has a record.
  //
  // Placement is one rule for cards and topics. Pinned ones stand first, in
  // pin order, in the pin list: the head of the board column, or the pinned
  // column beside it where the page shows one (opt.column). An unpinned
  // card enters at the head of the unpinned flow when it opens (bump counts
  // as an open); after that it only moves down, as a topic updated after
  // the card opened rises above it. A card's open time is kept with its
  // view state, so a reload puts it back where it stood among the topics.
  // Topics otherwise keep the board's own order. Dragging a header moves a
  // card or a topic: within the pin list it reorders the pins, into the
  // board's open flow it unpins and stands where it was dropped (a time
  // between its new neighbours'), into the pin list it pins. A topic moved
  // by hand keeps that time until the topic itself next updates; a closed
  // topic stands only in the closed section or the pin list. A pinned topic
  // keeps its place when it updates or closes, and the page keeps its last
  // node when the board stops serving it (past the closed-topic cap); after
  // a reload such a topic is fetched through the closed route (opt.closed),
  // and one the store no longer holds loses its pin.
  //
  // A card's node is never moved by a board update: the board morph weaves
  // the cards into the nodes it wants (weave) and settles the board around
  // them (settle), so a frame or a video in a card keeps running. Only the
  // card the user pins, unpins, or reopens moves, and where the browser has
  // Element.moveBefore even that move keeps the card's state; a move between
  // the board and the pinned column takes it too.
  var doc = document;
  var BASE = 260, MIN = 72, STEP = 24, PINW_MIN = 280, BOARDW_MIN = 400;
  // A press drags past SLOP px; a touch drags after holding HOLD ms (the
  // session rail's drag thresholds), and EDGE px from a list's edge scrolls it.
  var SLOP = 5, HOLD = 380, EDGE = 28;
  var kinds = {}, cards = [], topics = {}, board = null, opt = null, maxed = null, scrim = null, seq = 0;
  // pins is the pin list: after the toolbar in the board, or the pinned
  // column's list while the page shows that column (colOn). boardw is the
  // board's width beside the column as the user dragged it (0: the
  // stylesheet's).
  var pins = null, col = null, colOn = false, boardw = 0, zone = null;
  var toolbar = null, counts = null, overviewOn = false;

  function licon(name) { return '<svg class="li" aria-hidden="true"><use href="#l-' + name + '"/></svg>'; }
  function mk(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function ibtn(cls, icon, title) {
    var b = mk("button", "ibtn" + (cls ? " " + cls : ""));
    b.type = "button";
    b.innerHTML = licon(icon);
    b.title = title;
    b.setAttribute("aria-label", title);
    return b;
  }
  function isCard(n) { return !!n && n.nodeType === 1 && n.hasAttribute("data-card"); }
  // The toolbar is infrastructure, not a card, flow peer, or storage record.
  function fixedNode(n) { return isCard(n) || (!!pins && n === pins) || (!!toolbar && n === toolbar); }
  function buildToolbar() {
    toolbar = mk("div", "board-toolbar");
    toolbar.setAttribute("role", "toolbar");
    toolbar.setAttribute("aria-label", wsT("js.board.toolbar.label"));
    var row = mk("div", "board-tools"), actions = opt.toolbar || {};
    function action(cls, icon, label, run) {
      var b = mk("button", "board-action " + cls);
      b.type = "button";
      b.innerHTML = licon(icon);
      b.appendChild(mk("span", "board-label", label));
      b.title = label;
      b.setAttribute("aria-label", label);
      b.addEventListener("click", function (e) { if (run) run(e); });
      row.appendChild(b);
      return b;
    }
    action("board-overview", "chevrons-down-up", wsT("js.board.toolbar.overview"), actions.overview).setAttribute("aria-pressed", "false");
    var term = mk("span", "board-term-slot");
    term.id = "board-term-slot";
    row.appendChild(term);
    var web = mk("span", "board-web-slot");
    web.id = "board-web-slot";
    row.appendChild(web);
    var search = action("board-search", "search", wsT("js.board.toolbar.search"), actions.search);
    search.appendChild(mk("kbd", "", "Ctrl K"));
    if (actions.files) {
      var files = action("board-files", "folder-open", wsT("js.board.toolbar.files"), function () {
        var el = doc.getElementById("files");
        if (!el) return;
        maximize(maxed && maxed.files ? null : { files: true, el: el, back: files });
      });
      files.setAttribute("aria-controls", "files");
      files.setAttribute("aria-expanded", "false");
    }
    counts = mk("span", "board-count");
    row.appendChild(counts);
    var face = mk("div", "board-toolbar-face");
    face.appendChild(row);
    toolbar.appendChild(face);
  }
  // The sticky root keeps the original flex footprint. Only its face changes
  // height: no scroll writer, replacement controls, or transient measuring DOM.
  function floatToolbar() {
    if (!window.ResizeObserver) return;
    var face = toolbar.firstChild, row = face.firstChild, countHeight = counts.getBoundingClientRect().height, scrollBox = null;
    function style(n, key, value) {
      if (n.style.getPropertyValue(key) !== value) n.style.setProperty(key, value);
    }
    function update() {
      if (!face.getClientRects().length || !face.getBoundingClientRect().width) return;
      var s = scroller(board) || doc.scrollingElement, main = s === doc.scrollingElement;
      // Document boards share the viewport with the fixed drawer handles.
      // Keep both the face and its counts below them, rather than raising the
      // toolbar over unrelated controls or changing horizontal action boxes.
      var inset = 0;
      if (main) Array.prototype.forEach.call(doc.querySelectorAll(".tk-handle, .gh, .dr-handle"), function (n) {
        if (!n.getClientRects().length) return;
        var r = n.getBoundingClientRect();
        if (r.top <= 0 && r.bottom > 0) inset = Math.max(inset, r.bottom + 4);
      });
      style(toolbar, "--toolbar-top", inset + "px");
      var top = main ? inset : s.getBoundingClientRect().top + s.clientTop;
      var floating = board.getBoundingClientRect().top < top;
      toolbar.toggleAttribute("data-floating", floating);
      if (!floating) countHeight = counts.getBoundingClientRect().height;
      var rows = {}, actionRows = {};
      Array.prototype.forEach.call(row.children, function (n) {
        var r = n.getBoundingClientRect();
        if (!r.height) return;
        var key = Math.round((r.top + r.bottom) * 2) / 4;
        var action = n !== counts;
        rows[key] = Math.max(rows[key] || 0, action ? 30 : countHeight);
        if (action) actionRows[key] = true;
      });
      toolbar.toggleAttribute("data-wrapped-actions", Object.keys(actionRows).length > 1);
      var keys = Object.keys(rows), reserved = 16 + Math.max(0, keys.length - 1) * 6;
      keys.forEach(function (key) { reserved += rows[key]; });
      style(toolbar, "--toolbar-reserved", (floating ? reserved : face.getBoundingClientRect().height) + "px");
      var height = face.getBoundingClientRect().height;
      // The rail already exposes a host inset; include the lower hit extension.
      style(doc.getElementById("bcol") || board, "--wb-rail-inset", (inset + height + 6) + "px");
      if (scrollBox && scrollBox !== s) scrollBox.style.removeProperty("scroll-padding-top");
      scrollBox = s;
      style(s, "scroll-padding-top", (inset + height + 10) + "px");
      if (maxed && maxed.files) style(maxed.el, "--files-top", Math.max(52, face.getBoundingClientRect().bottom + 8) + "px");
    }
    doc.addEventListener("scroll", update, { capture: true, passive: true });
    window.addEventListener("resize", update);
    new window.ResizeObserver(update).observe(face);
    update();
  }
  function countTopics() {
    if (!counts) return;
    var open = 0, closed = 0, seen = new Set();
    [board, pins].forEach(function (box) {
      Array.prototype.forEach.call(box.querySelectorAll("section.topic[data-topic]"), function (n) {
        var name = n.getAttribute("data-topic");
        if (seen.has(name)) return;
        seen.add(name);
        if (n.classList.contains("closed")) closed++; else open++;
      });
    });
    var more = board.querySelector("button.closed-more[data-closed-more]");
    if (more) closed += Math.max(0, +more.getAttribute("data-closed-more") || 0);
    counts.textContent = wsT("js.board.toolbar.count", open, closed);
  }
  function topicName(n) {
    return n && n.nodeType === 1 && n.tagName === "SECTION" && n.classList.contains("topic") && n.hasAttribute("data-topic") ? n.getAttribute("data-topic") : null;
  }

  // topicAt is an open topic's last update (epoch ms), the time in its bar
  // (a recap line carries a time of its own), or null for any other node: a
  // closed topic, the closed-topic control, the reply bar.
  function topicAt(n) {
    if (n.nodeType !== 1 || !n.hasAttribute("data-topic") || n.classList.contains("closed")) return null;
    var t = n.querySelector(".tbar time.tago[data-at]");
    return t ? +t.getAttribute("data-at") || 0 : 0;
  }
  // order weaves the cards in list into base, the board's own nodes in
  // order, by the placement rule; at reads a node's topicAt.
  function order(base, list, at) {
    var pins = list.filter(function (c) { return c.pinned; }).sort(function (a, b) { return a.pinned - b.pinned; });
    var flow = list.filter(function (c) { return !c.pinned; }).sort(function (a, b) { return b.at - a.at || rank(a) - rank(b); });
    var out = pins.map(function (c) { return c.el; }), i = 0, counts = {};
    base.forEach(function (n) {
      var t = at(n), time = t === null ? -1 : t, r = counts[time] || 0;
      counts[time] = r + 1;
      // rank is an independent snapshot within a tied timestamp (or the
      // closed tail), not an attachment to a topic. Ordinary opens retain
      // the original rule: before all equal/older topics and controls.
      while (i < flow.length && (flow[i].at > time || (flow[i].at === time && rank(flow[i]) <= r))) out.push(flow[i++].el);
      out.push(n);
    });
    while (i < flow.length) out.push(flow[i++].el);
    return out;
  }
  function rank(u) { return typeof u.rank === "number" ? u.rank : -1; }
  // settle makes el's children the nodes in want, in order. Stale nodes go
  // first, so the kept ones that stay in order are never moved (moving a
  // frame reloads it). A fixed node already in el is never moved either:
  // the nodes standing before it that are wanted after it move past it.
  // A node already in el moves with Element.moveBefore where the browser
  // has it, which keeps its frames loaded; so does a node moving in from
  // another container on the page (the pin list and the board). A new
  // node, or a browser without it, takes insertBefore. Either way the same
  // nodes move. A child in skip is not stale: another container wants it
  // and takes it from here.
  function settle(el, want, fixed, skip) {
    var keep = new Set(want);
    function put(n, ref) {
      if (el.moveBefore && (n.parentNode === el || (n.isConnected && el.isConnected))) {
        try { el.moveBefore(n, ref); return; } catch (e) { /* insertBefore below */ }
      }
      el.insertBefore(n, ref);
    }
    Array.prototype.slice.call(el.childNodes).forEach(function (n) { if (!keep.has(n) && !(skip && skip.has(n))) el.removeChild(n); });
    var cur = el.firstChild;
    want.forEach(function (n) {
      if (n === cur) { cur = cur.nextSibling; return; }
      if (fixed(n) && n.parentNode === el) {
        var after = n.nextSibling;
        while (cur !== n) { var next = cur.nextSibling; put(cur, after); cur = next; }
        cur = n.nextSibling;
        return;
      }
      put(n, cur);
    });
  }
  // headTime is an open time that puts a card at the head of the unpinned
  // flow: now, or just past the newest topic and card where the server's
  // clock (a topic's time) runs ahead of this browser's.
  function headTime() {
    var t = Date.now();
    if (board) Array.prototype.forEach.call(board.children, function (n) { var a = isCard(n) ? null : topicAt(n); if (a !== null && a >= t) t = a + 1; });
    units().forEach(function (c) { if (!c.pinned && c.at >= t) t = c.at + 1; });
    return t;
  }

  // --- topics among the units ---------------------------------------------------
  // A topic's record: name, pinned (its pin order, 0 when unpinned), at (a
  // hand-moved topic's place in the flow, 0 for none), bar (its bar time
  // when it was moved, so its own update ends the move), el (its node: the
  // last one the board served, kept while pinned).
  function topicRec(name) { return topics[name] || (topics[name] = { name: name, pinned: 0, at: 0, bar: 0, el: null }); }
  function tidy(r) { if (!r.pinned && !r.at && !r.folded && topics[r.name] === r) delete topics[r.name]; }
  // units are the cards and the topics that place themselves: pinned, or
  // moved by hand into the flow.
  function units() {
    var list = cards.slice();
    Object.keys(topics).forEach(function (k) { var r = topics[k]; if (r.el && (r.pinned || r.at)) list.push(r); });
    return list;
  }
  function pinned() { return units().filter(function (u) { return u.pinned; }).sort(function (a, b) { return a.pinned - b.pinned; }); }
  function claimed(n) {
    var name = topicName(n), r = name !== null ? topics[name] : null;
    return !!r && r.el === n && !!(r.pinned || r.at);
  }
  function unitOf(n) {
    if (isCard(n)) return cards.filter(function (c) { return c.el === n; })[0] || null;
    var name = topicName(n);
    return name === null ? null : topics[name] && topics[name].el === n ? topics[name] : { name: name, pinned: 0, at: 0, bar: 0, el: n, loose: true };
  }

  // claim reads the board's own nodes (a board update's): each pinned or
  // hand-moved topic takes its node from them, a hand move whose topic
  // updated or closed (or left the board) lapses, and the rest stay in the
  // board's order. A pinned topic the board no longer serves keeps the node
  // it had.
  function claim(base) {
    var seen = {};
    var rest = base.filter(function (n) {
      var name = topicName(n), r = name !== null ? topics[name] : null;
      if (!r) return true;
      seen[name] = true;
      r.el = n;
      if (r.pinned) return false;
      if (r.at && topicAt(n) === r.bar) return false;
      r.at = 0;
      tidy(r);
      return true;
    });
    Object.keys(topics).forEach(function (k) { var r = topics[k]; if (!seen[k] && !r.pinned) { r.at = 0; tidy(r); } });
    return rest;
  }
  // split is the placement of base, the board's own nodes less the claimed
  // topics: the pin list's nodes and the board column's.
  function split(base) {
    var flow = units().filter(function (u) { return !u.pinned; });
    var out = order(base, flow, topicAt);
    return { pins: pinned().map(function (u) { return u.el; }), board: [toolbar].concat(colOn && !overviewOn ? out : [pins].concat(out)) };
  }
  function weave(base) {
    var rest = claim(base);
    relayout();
    return split(rest);
  }
  // seat puts a topic that leaves the pin list without a place of its own
  // back among the board's nodes: an open one above the first open topic
  // older than it, a closed one at the head of the closed section (the next
  // board update puts it where the board keeps it).
  function seat(base, n) {
    var i = base.indexOf(n);
    if (i >= 0) base.splice(i, 1);
    var t = topicAt(n);
    for (i = 0; i < base.length; i++) {
      if (base[i].nodeType !== 1) continue;
      var a = topicAt(base[i]);
      if (a === null || (t !== null && a < t)) break;
    }
    base.splice(i, 0, n);
  }
  // arrange puts the cards and topics in their places. moved is the one
  // unit whose place changed: it alone moves, with its state kept where the
  // browser can keep it.
  function arrange(moved) {
    if (!board) return;
    relayout();
    if (moved && moved.el && !isCard(moved.el)) leaveSlot(moved.el);
    var base = Array.prototype.filter.call(board.childNodes, function (n) { return !fixedNode(n) && !claimed(n); });
    if (moved && moved.el && !isCard(moved.el) && !claimed(moved.el) && moved.el.parentNode !== board) seat(base, moved.el);
    var p = split(base), box = moved && moved.el ? (p.pins.indexOf(moved.el) >= 0 ? pins : board) : null;
    if (box && moved.el.parentNode === box) {
      var want = box === pins ? p.pins : p.board, ref = want[want.indexOf(moved.el) + 1] || null;
      if (moved.el.nextSibling !== ref) {
        try { if (box.moveBefore) box.moveBefore(moved.el, ref); else box.insertBefore(moved.el, ref); }
        catch (e) { box.insertBefore(moved.el, ref); }
      }
    }
    place(p, function (n) { return n === pins || n === toolbar; });
  }
  // place settles the pin list, then the board (fixed: what the board's
  // settle never moves): a node pinned from the board moves into the list
  // before the board lets it go, and one unpinned moves out of the list
  // into the board. The pin list itself moves to the pinned column first
  // when the page shows one, so the board never lets it go either.
  function place(p, fixed) {
    if (colOn && !overviewOn && pins.parentNode !== col) {
      try { if (col.moveBefore && pins.isConnected) col.moveBefore(pins, null); else col.appendChild(pins); }
      catch (e) { col.appendChild(pins); }
    }
    settle(pins, p.pins, function () { return false; }, new Set(p.board));
    settle(board, p.board, fixed, new Set(p.pins));
    p.pins.concat(p.board).forEach(function (n) { var name = topicName(n); if (name !== null && topics[name]) topics[name].el = n; });
    decorate();
  }
  // relayout asks the page whether it shows the pinned column for the pins
  // there are, and tells it when that changes.
  function relayout() {
    var n = pinned().length, on = !!(col && opt && opt.shows && opt.shows(n));
    if (on === colOn) return;
    colOn = on;
    if (opt.layout) opt.layout(on);
  }
  // decorate gives every topic on the board a pin button in its title bar,
  // before the note pen, pressed while the topic is pinned, and the
  // maximize button and its back button right of the pin. A board update
  // brings a changed topic's bar anew (a new section, or the old one with
  // the new bar's content), so this runs after every settle, and draws a
  // large topic or panel large again on the node that now stands for it.
  function decorate() {
    [board, colOn ? pins : null].forEach(function (box) {
      if (!box) return;
      Array.prototype.forEach.call(box.querySelectorAll("section.topic[data-topic]"), function (sec) {
        var head = child(sec, "tophead"), bar = head && child(head, "tbar");
        if (!bar) return;
        var r = topics[sec.getAttribute("data-topic")], on = !!(r && r.pinned && r.el === sec);
        var b = child(bar, "tpin");
        if (!b) {
          b = ibtn("tpin", "pin", wsT("js.card.pin"));
          bar.insertBefore(b, child(bar, "tnote-toggle"));
        }
        [["t-fold", "chevron-down", "js.card.fold"], ["t-raise", "arrow-up-to-line", "js.card.raise"], ["d-max", "maximize-2", "js.card.max"], ["d-min", "minimize-2", "js.card.back"]].forEach(function (a) {
          if (!child(bar, a[0])) bar.insertBefore(ibtn(a[0], a[1], wsT(a[2])), child(bar, "tnote-toggle"));
        });
        sec.setAttribute("data-size", r && r.folded ? "fold" : "base");
        child(bar, "t-fold").setAttribute("aria-expanded", String(!(r && r.folded)));
        b.setAttribute("aria-pressed", String(on));
        if (on) sec.setAttribute("data-pinned", ""); else sec.removeAttribute("data-pinned");
      });
    });
    if (maxed && !maxed.files && !(maxed instanceof Card)) follow(maxed);
    countTopics();
  }
  // leaveSlot marks the place of a topic that leaves a fetched closed page
  // (dashboard.js loadClosed) for the pin list or the board: the page's
  // show-more index counts it as the server does (dashboard.js
  // morphBoard), so the next page neither skips a topic nor shows one
  // twice.
  function leaveSlot(n) {
    var p = n.parentNode, name = topicName(n);
    if (!p || name === null || !p.classList || !p.classList.contains("closed-page")) return;
    var s = mk("span", "closed-slot");
    s.hidden = true;
    s.setAttribute("data-slot", name);
    p.insertBefore(s, n);
  }
  function child(el, cls) {
    for (var n = el.firstElementChild; n; n = n.nextElementSibling) if (n.classList.contains(cls)) return n;
    return null;
  }
  function pinTopic(sec, on) {
    var name = topicName(sec);
    if (name === null) return;
    var r = topicRec(name);
    r.el = sec;
    r.pinned = on ? Date.now() : 0;
    r.at = 0;
    tidy(r);
    arrange(r);
    persist();
  }

  // Shared actions adopt the renderer's topic node rather than wrapping or
  // rebuilding its body/badges. Topics have no generic close operation.
  function foldUnit(u, on) {
    if (u.folded === !!on) return;
    u.folded = !!on;
    var head = isCard(u.el) ? child(child(u.el, "bc-in"), "bc-hd") : child(child(u.el, "tophead"), "tbar");
    var active = doc.activeElement;
    for (var n = active; n; n = n.parentNode) if (n === u.el) {
      if (on && head && (!active.closest || !active.closest(".bc-hd, .tbar"))) child(head, isCard(u.el) ? "bc-fold" : "t-fold").focus({ preventScroll: true });
      break;
    }
    if (isCard(u.el)) { u.paint(); if (!on) u.sized(); }
    else { decorate(); tidy(u); }
    persist();
  }
  function topicUnit(sec) {
    var r = topicRec(topicName(sec)); r.el = sec; return r;
  }
  function raise(u) {
    if (u.pinned) { drop(u, { pin: true, index: 0 }); return; }
    u.at = headTime();
    delete u.rank;
    if (!isCard(u.el)) u.bar = topicAt(u.el);
    arrange(u);
    persist();
  }
  // A preview origin is a live renderer-owned topic in this board only.
  // Detached links retain the exact node, so replacement/removal invalidates
  // their origin rather than borrowing a same-named or unrelated topic.
  function originTopic(n) {
    if (!n || topicName(n) === null || !n.isConnected) return null;
    for (var p = n.parentNode; p; p = p.parentNode) if (p === board || p === pins) return n;
    return null;
  }
  function openBefore(c, origin) {
    var n = originTopic(origin), u = n && unitOf(n);
    delete c.rank;
    if (!n || u.pinned) { c.at = headTime(); return; }
    // Fetched closed topics may be nested in a closed-page. Adopt just the
    // origin into the same ordinary flow, leaving its pagination slot intact.
    if (n.parentNode !== board) {
      u = topicUnit(n); u.at = -1; u.bar = topicAt(n);
      arrange(u);
    }
    c.at = eff(n);
    if (c.at === null) c.at = -1;
    var target = 0, prev = null, count = 0;
    Array.prototype.some.call(board.children, function (peer) {
      var other = unitOf(peer), placed = other && !other.loose && other.at;
      if (peer === n) { target = placed ? rank(other) : count; return true; }
      if (peer === c.el || peer === toolbar || peer === pins) return false;
      var time = eff(peer);
      if (time === null) time = -1;
      if (time !== c.at) return false;
      prev = placed ? rank(other) : count++;
      return false;
    });
    c.rank = prev === null ? target - 1 : (prev + target) / 2;
  }

  // --- storage ----------------------------------------------------------------
  // The record is {cards, topics, bw}: the cards' view state, the topics
  // pinned or moved by hand, and the board's width beside the pinned
  // column. A record from before topics joined is the cards' list alone; a
  // record's w (the column's own width, before the column filled the
  // window) is dropped.
  function persist() {
    if (!opt) return;
    var ts = Object.keys(topics).map(function (k) { return topics[k]; }).filter(function (r) { return r.pinned || r.at || r.folded; }).map(function (r) {
      return { n: r.name, p: r.pinned, a: r.at, b: r.bar, f: !!r.folded, rank: r.rank };
    });
    opt.save(opt.key, cards.length || ts.length || boardw ? JSON.stringify({ cards: cards.map(function (c) {
      return { id: c.id, kind: c.kind, pinned: c.pinned, at: c.at, rank: c.rank, fold: c.folded, h: c.h, data: c.data };
    }), topics: ts, bw: boardw }) : null);
  }
  function stored() {
    var v = null;
    try { v = JSON.parse(opt.load(opt.key) || "null"); } catch (e) { v = null; }
    if (Array.isArray(v)) v = { cards: v };
    if (!v || typeof v !== "object") v = {};
    return {
      cards: (Array.isArray(v.cards) ? v.cards : []).filter(function (r) { return r && typeof r.kind === "string" && kinds[r.kind]; }),
      topics: (Array.isArray(v.topics) ? v.topics : []).filter(function (r) { return r && typeof r.n === "string" && (+r.p || +r.a || r.f); }),
      bw: +v.bw || 0
    };
  }

  // --- a card -------------------------------------------------------------------
  function Card(kind, rec) {
    this.kind = kind;
    this.k = kinds[kind];
    this.id = rec.id || "c" + Date.now().toString(36) + (seq++).toString(36);
    this.pinned = +rec.pinned || 0;
    this.at = typeof rec.at === "number" && isFinite(rec.at) ? rec.at : headTime();
    if (typeof rec.rank === "number" && isFinite(rec.rank)) this.rank = rec.rank;
    this.folded = !!rec.fold;
    this.h = +rec.h || 0;
    this.data = rec.data || {};
    this.build();
  }
  Card.prototype.build = function () {
    var c = this, el = mk("div", "bcard"), inner = mk("div", "bc-in"), head = mk("div", "bc-hd");
    el.setAttribute("data-card", c.id);
    el.setAttribute("data-kind", c.kind);
    c.el = el;
    // The topic rail (wswhiteboard rail.js) rails the card beside the
    // topics: a capsule with the header's icon and name once it scrolls
    // past, and the header sticking while it is current.
    el.setAttribute("data-rail", c.id);
    head.setAttribute("data-rail-bar", "");
    c.icon = mk("span", "bc-ic");
    c.icon.setAttribute("data-rail-icon", "");
    c.name = mk("span", "bc-nm");
    c.name.setAttribute("data-rail-name", "");
    c.sub = mk("span", "bc-sub");
    c.badge = mk("span", "bc-kind");
    c.acts = mk("span", "bc-acts");
    c.pinBtn = ibtn("bc-pin", "pin", wsT("js.card.pin"));
    c.foldBtn = ibtn("bc-fold", "chevron-down", wsT("js.card.fold"));
    c.raiseBtn = ibtn("bc-raise", "arrow-up-to-line", wsT("js.card.raise"));
    c.maxBtn = ibtn("bc-max", "maximize-2", wsT("js.card.max"));
    c.minBtn = ibtn("bc-min", "minimize-2", wsT("js.card.min"));
    c.closeBtn = ibtn("bc-close", "x", wsT("js.card.close"));
    [c.icon, c.name, c.sub, c.badge, c.acts, c.pinBtn, c.foldBtn, c.raiseBtn, c.maxBtn, c.minBtn, c.closeBtn].forEach(function (n) { head.appendChild(n); });
    c.foldEl = mk("div", "bc-folded");
    c.body = mk("div", "bc-body");
    c.grip = mk("div", "bc-grip");
    c.grip.setAttribute("role", "separator");
    c.grip.setAttribute("aria-orientation", "horizontal");
    c.grip.tabIndex = 0;
    c.grip.title = wsT("js.card.grip");
    c.grip.setAttribute("aria-label", wsT("js.card.grip"));
    [head, c.foldEl, c.body, c.grip].forEach(function (n) { inner.appendChild(n); });
    el.appendChild(inner);
    c.pinBtn.addEventListener("click", function () { c.pin(!c.pinned); });
    c.foldBtn.addEventListener("click", function () { c.fold(!c.folded); });
    c.raiseBtn.addEventListener("click", function () { raise(c); });
    c.maxBtn.addEventListener("click", function () { maximize(c); });
    c.minBtn.addEventListener("click", function () { maximize(null); });
    c.closeBtn.addEventListener("click", function () { if (!c.k.closing || c.k.closing(c) !== false) c.close(); });
    c.wireGrip();
    c.paint();
  };
  // paint writes the framework's own view state to the card's node.
  Card.prototype.paint = function () {
    var c = this;
    c.el.setAttribute("data-size", c.folded ? "fold" : "base");
    if (c.pinned) c.el.setAttribute("data-pinned", ""); else c.el.removeAttribute("data-pinned");
    c.pinBtn.setAttribute("aria-pressed", String(!!c.pinned));
    c.foldBtn.setAttribute("aria-expanded", String(!c.folded));
    c.body.style.height = (c.h || (typeof c.k.base === "function" ? c.k.base(c) : c.k.base) || BASE) + "px";
    c.foldEl.textContent = "";
    var f = c.folded && c.k.folded ? c.k.folded(c) : null;
    if (f) c.foldEl.appendChild(f);
  };
  // title sets the header's content: p.icon (a vendored Lucide icon's
  // name), p.name, p.sub (a path, shown dimmed), p.badge (the kind's word).
  Card.prototype.title = function (p) {
    this.icon.innerHTML = p.icon ? licon(p.icon) : "";
    this.name.textContent = p.name || "";
    this.sub.textContent = p.sub || "";
    this.sub.title = p.sub || "";
    this.badge.textContent = p.badge || "";
    this.badge.hidden = !p.badge;
  };
  // actions sets the kind's own header buttons: [{icon, title, run, cls}]
  // (cls: an optional class of the kind's own), and returns them in order,
  // for a kind that updates a button's state.
  Card.prototype.actions = function (list) {
    var c = this;
    c.acts.textContent = "";
    return (list || []).map(function (a) {
      var b = ibtn(a.cls || "", a.icon, a.title);
      b.addEventListener("click", function () { a.run(c); });
      c.acts.appendChild(b);
      return b;
    });
  };
  Card.prototype.save = function () { persist(); };
  Card.prototype.sized = function () { if (this.k.sized) this.k.sized(this); };
  Card.prototype.fold = function (on) {
    foldUnit(this, on);
  };
  Card.prototype.pin = function (on) {
    this.pinned = on ? Date.now() : 0;
    this.paint();
    arrange(this);
    persist();
    if (this.k.pinned) this.k.pinned(this);
  };
  // bump counts as an open: the card moves to the head of the unpinned
  // flow. A pinned card stays where it is.
  Card.prototype.bump = function (origin) {
    if (this.pinned) return;
    openBefore(this, origin);
    arrange(this);
    persist();
  };
  // show brings the card into view and marks it with a short flash.
  Card.prototype.show = function () {
    // A file preview (or a row-menu terminal) must not open behind Files.
    if (maxed && maxed.files) maximize(null);
    var el = this.el;
    if (opt && opt.reveal) opt.reveal();
    var calm = !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
    if (el.scrollIntoView) el.scrollIntoView({ block: "nearest", behavior: calm ? "auto" : "smooth" });
    el.classList.remove("flash");
    void el.offsetWidth;
    el.classList.add("flash");
  };
  Card.prototype.close = function () {
    var i = cards.indexOf(this);
    if (i < 0) return;
    if (maxed === this) maximize(null);
    cards.splice(i, 1);
    if (this.k.close) this.k.close(this);
    if (this.el.parentNode) this.el.parentNode.removeChild(this.el);
    if (this.pinned) arrange(null);
    persist();
  };
  // The handle under the body drags its height; the pointer is captured, so
  // the drag keeps going over a frame in the body. Double click (or Home)
  // goes back to the base height.
  Card.prototype.wireGrip = function () {
    var c = this, from = null;
    function set(h) {
      c.h = Math.max(MIN, Math.round(h));
      c.body.style.height = c.h + "px";
    }
    function end() {
      if (from === null) return;
      from = null;
      c.el.classList.remove("drag");
      persist();
      c.sized();
    }
    c.grip.addEventListener("pointerdown", function (e) {
      if (e.button !== undefined && e.button !== 0) return;
      from = { y: e.clientY, h: c.body.getBoundingClientRect().height };
      c.el.classList.add("drag");
      try { c.grip.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the move still lands */ }
      e.preventDefault();
    });
    c.grip.addEventListener("pointermove", function (e) { if (from) set(from.h + e.clientY - from.y); });
    c.grip.addEventListener("pointerup", end);
    c.grip.addEventListener("pointercancel", end);
    function reset() { c.h = 0; c.paint(); persist(); c.sized(); }
    c.grip.addEventListener("dblclick", reset);
    c.grip.addEventListener("keydown", function (e) {
      var d = e.key === "ArrowUp" ? -STEP : e.key === "ArrowDown" ? STEP : 0;
      if (e.key === "Home") { e.preventDefault(); reset(); return; }
      if (!d) return;
      e.preventDefault();
      set(c.body.getBoundingClientRect().height + d);
      persist();
      c.sized();
    });
  };

  // --- the maximize sheet -------------------------------------------------------
  // A maximized card is the same node, drawn fixed over both columns (the
  // stylesheet's [data-max] rules) with a scrim behind it; its place in the
  // board keeps its height meanwhile. Nothing is moved, so a frame or a
  // video keeps running through maximize and back. One thing is large at a
  // time: a card, or a board node (sheet: a topic, or a topic's panel).
  function maximize(c) {
    if (c && window.WsOverview && window.WsOverview.holding()) { window.WsOverview.after(function () { maximize(c); }); return; }
    if (maxed === c) return;
    if (maxed) {
      var was = maxed;
      maxed = null;
      scrim.hidden = true;
      if (was.files) {
        was.el.removeAttribute("data-max");
        was.el.style.removeProperty("--files-top");
        toolbar.removeAttribute("data-files");
        was.back.setAttribute("aria-expanded", "false");
        if (was.root) was.slot.appendChild(was.root);
        opt.toolbar.files();
        if (was.back.isConnected) was.back.focus({ preventScroll: true });
      } else if (was instanceof Card) {
        was.el.removeAttribute("data-max");
        was.el.style.height = "";
        was.sized();
        if (was.maxBtn.isConnected) was.maxBtn.focus();
      } else {
        // The focus comes back without scrolling: the node's place is where
        // it was left.
        var back = shrink(was);
        if (back) back.focus({ preventScroll: true });
      }
    }
    if (!c) return;
    if (!scrim) {
      scrim = mk("div", "bc-scrim");
      scrim.addEventListener("click", function () { maximize(null); });
      doc.body.appendChild(scrim);
    }
    scrim.hidden = false;
    maxed = c;
    if (c.files) {
      // The one explorer stays in its grid slot: no tab switch, remount or
      // second browser. Its existing root label travels back on dismissal.
      c.slot = doc.querySelector("#chead .fsum");
      c.root = c.slot && c.slot.querySelector(".froot");
      if (c.root) c.el.querySelector(".fhd").insertBefore(c.root, c.el.querySelector(".fhd").firstChild);
      c.el.style.setProperty("--files-top", Math.max(52, toolbar.firstChild.getBoundingClientRect().bottom + 8) + "px");
      c.el.setAttribute("data-max", "");
      toolbar.setAttribute("data-files", "");
      c.back.setAttribute("aria-expanded", "true");
      opt.toolbar.files();
      var filter = c.el.querySelector(".fq");
      if (filter) filter.focus({ preventScroll: true });
      return;
    }
    if (!(c instanceof Card)) {
      grow(c);
      if (!c.el) { maximize(null); return; }
      var m = sheetBtn(c.el, "d-min");
      if (m) m.focus({ preventScroll: true });
      return;
    }
    c.el.style.height = c.el.getBoundingClientRect().height + "px";
    c.el.setAttribute("data-max", "");
    c.sized();
    c.minBtn.focus();
  }
  // A board node drawn large is the whiteboard's own (cards.js never
  // builds it), and a board update may bring it anew, so its sheet names
  // it: a topic by its name, a panel by its data-key. The node stays where
  // it stands, drawn fixed by the stylesheet's [data-max] rules: a panel's
  // entry keeps its height meanwhile, as a card does, and a topic, the
  // board's own child with nothing around it to hold its place, gives the
  // scroll offsets it changed back on its way out.
  function sheet(btn) {
    var p = btn.parentNode;
    if (p.classList.contains("tbar")) {
      var sec = p.closest("section.topic");
      return topicName(sec) !== null ? { topic: true, key: sec.getAttribute("data-topic"), el: null } : null;
    }
    var d = p.tagName === "SUMMARY" ? p.parentNode : null;
    return d && d.matches("details.entry-panel[data-key]") ? { topic: false, key: d.getAttribute("data-key"), el: null } : null;
  }
  function sheetNode(t) {
    var sel = t.topic ? "section.topic[data-topic]" : "details.entry-panel[data-key]", attr = t.topic ? "data-topic" : "data-key", n = null;
    [board, col].some(function (box) {
      n = box ? Array.prototype.filter.call(box.querySelectorAll(sel), function (x) { return x.getAttribute(attr) === t.key; })[0] || null : null;
      return !!n;
    });
    return n;
  }
  function sheetBtn(n, cls) {
    var row = n.tagName === "DETAILS" ? n.firstElementChild : child(child(n, "tophead") || n, "tbar");
    return row ? child(row, cls) : null;
  }
  function grow(t) {
    var n = sheetNode(t);
    if (t.el && t.el !== n) shrinkNode(t);
    t.el = n;
    if (!n || n.hasAttribute("data-max")) return;
    if (t.topic) {
      if (!t.scroll) {
        t.scroll = [];
        for (var x = n.parentNode; x && x.nodeType === 1; x = x.parentNode) if (x.scrollHeight > x.clientHeight) t.scroll.push([x, x.scrollTop]);
      }
    } else {
      n.open = true;
      n.parentNode.style.height = n.parentNode.getBoundingClientRect().height + "px";
    }
    n.setAttribute("data-max", "");
  }
  function shrinkNode(t) {
    t.el.removeAttribute("data-max");
    if (!t.topic && t.el.parentNode) t.el.parentNode.style.height = "";
  }
  // shrink draws t's node in its place again and returns its maximize
  // button, the focus's way back.
  function shrink(t) {
    var n = t.el && t.el.isConnected ? t.el : null;
    if (t.el) shrinkNode(t);
    (t.scroll || []).forEach(function (s) { s[0].scrollTop = s[1]; });
    return n ? sheetBtn(n, "d-max") : null;
  }
  // follow draws a large board node large again after a board update: on
  // its new node, or, with the node gone, nothing is large any more. A
  // focus the update took (the old head's back button) goes to the new one.
  function follow(t) {
    if (!sheetNode(t)) { maximize(null); return; }
    grow(t);
    var a = doc.activeElement, m = sheetBtn(t.el, "d-min");
    if (m && (!a || a === doc.body)) m.focus({ preventScroll: true });
  }
  function sheetClick(e) {
    var b = e.target.closest && e.target.closest(".d-max, .d-min");
    if (b && (b.parentNode.classList.contains("tbar") || b.parentNode.tagName === "SUMMARY")) {
      // A button in a panel's head must not fold or unfold the panel.
      e.preventDefault();
      if (b.classList.contains("d-min")) { maximize(null); return; }
      var t = sheet(b);
      if (t) maximize(t);
      return;
    }
    // A large panel stays open: its head does not fold it.
    var s = e.target.closest && e.target.closest("summary");
    if (s && s.parentNode.matches("details.entry-panel[data-max]") && !e.target.closest("a, button")) e.preventDefault();
  }
  doc.addEventListener("keydown", function (e) {
    if (e.key !== "Escape" || !maxed || e.defaultPrevented) return;
    e.preventDefault();
    maximize(null);
  });

  // --- dropping a moved card or topic -----------------------------------------
  // eff is a flow node's place in time: a card's open time, a hand-moved
  // topic's time, another open topic's bar time; null for anything else.
  function eff(n) {
    var u = unitOf(n);
    return u && !u.loose && !u.pinned && (u.at || isCard(n)) ? u.at : topicAt(n);
  }
  // drop places unit u: to.pin pins it at index to.index of the pin list;
  // otherwise it stands in the board's open flow between to.after and
  // to.before (either null at an end), or, a closed topic, back in the
  // closed section.
  function drop(u, to) {
    var was = !!u.pinned, topic = !isCard(u.el);
    delete u.rank;
    if (topic && u.loose) { var el = u.el; u = topicRec(u.name); u.el = el; }
    if (to.pin) {
      // to.index counts the pins that show; a pinned topic still being
      // fetched (fetchMissing) keeps its order among them.
      var shown = pinned().filter(function (x) { return x !== u; }), at = shown[to.index];
      var list = cards.concat(Object.keys(topics).map(function (k) { return topics[k]; })).filter(function (x) { return x.pinned && x !== u; }).sort(function (a, b) { return a.pinned - b.pinned; });
      list.splice(at ? list.indexOf(at) : list.length, 0, u);
      list.forEach(function (x, i) { x.pinned = i + 1; });
      if (topic) u.at = 0;
    } else {
      u.pinned = 0;
      if (topic && topicAt(u.el) === null) u.at = 0;
      else {
        var ta = to.after ? eff(to.after) : null, tb = to.before ? eff(to.before) : null;
        u.at = ta === null ? Math.max(headTime(), tb === null ? 0 : tb + 1) : tb === null ? ta - 1 : ta > tb ? (ta + tb) / 2 : ta;
        if (topic) u.bar = topicAt(u.el);
      }
    }
    if (!topic) u.paint();
    arrange(u);
    if (topic) tidy(u);
    persist();
    if (!topic && was !== !!u.pinned && u.k.pinned) u.k.pinned(u);
  }

  // --- dragging a header ----------------------------------------------------------
  // A topic's title bar or a card's header drags the topic or card; a
  // button, link, or field in it does not. While a drag runs the page holds
  // its board updates (opt.drag) and the root carries data-bdrag: "col"
  // with the pinned column shown, "edge" where a first pin would open it
  // (a drop zone along the right edge), "group" where pins stand atop the
  // board (the empty pin list shows as a drop zone).
  var dg = null, dragged = false, line = null, raf = 0;
  function rectOf(el) { return el && el.getClientRects().length ? el.getBoundingClientRect() : null; }
  function inside(r, x, y) { return !!r && x >= r.left && x < r.right && y >= r.top && y < r.bottom; }
  function drawLine(box) {
    if (!box) { if (line) line.hidden = true; return; }
    if (!line) {
      line = doc.body.appendChild(mk("div", "bdrop"));
      line.setAttribute("aria-hidden", "true");
    }
    line.style.left = box.left + "px";
    line.style.width = box.width + "px";
    line.style.top = Math.round(box.top - 1) + "px";
    line.hidden = false;
  }
  // slot finds where y falls among peers: the index of the first whose
  // middle is below it, and the line between it and the one before.
  function slot(peers, y, box) {
    var at = peers.length, i;
    for (i = 0; i < peers.length; i++) {
      var r = peers[i].getBoundingClientRect();
      if (y < r.top + r.height / 2) { at = i; break; }
    }
    var top;
    if (!peers.length) top = box.top + 4;
    else if (at === peers.length) top = peers[at - 1].getBoundingClientRect().bottom + 4;
    else if (at === 0) top = peers[0].getBoundingClientRect().top - 4;
    else top = (peers[at - 1].getBoundingClientRect().bottom + peers[at].getBoundingClientRect().top) / 2;
    return { at: at, line: { left: box.left, width: box.width, top: top } };
  }
  function scroller(el) {
    for (var n = el; n && n !== doc.body; n = n.parentNode) if (n.nodeType === 1 && /auto|scroll/.test(getComputedStyle(n).overflowY)) return n;
    return null;
  }
  // over reads the drop at the pointer: the pin list (the pinned column, or
  // the pin group atop the board), the right-edge zone, or the board's open
  // flow. A drop that leaves the item where it stands shows no line.
  function over(x, y) {
    var d = dg;
    d.x = x; d.y = y;
    var to = null, pr = rectOf(pins), zr = d.mode === "edge" ? rectOf(zone) : null, scr = null;
    if (zr && inside(zr, x, y)) to = { pin: true, index: 0, line: null };
    else if (inside(rectOf(toolbar), x, y)) { /* the toolbar is never a drop target */ }
    else if (pr && (inside(pr, x, y) || (colOn && inside(rectOf(col), x, y)))) {
      var peers = Array.prototype.filter.call(pins.children, function (n) { return n !== d.el; }), s = slot(peers, y, pr);
      var me = d.unit.pinned ? pinned().indexOf(d.unit) : -1;
      to = { pin: true, index: s.at, line: me >= 0 && (s.at === me) ? null : s.line };
      scr = colOn ? pins : scroller(board);
    } else if (inside(rectOf(scroller(board) || board), x, y)) {
      scr = scroller(board);
      if (!isCard(d.el) && topicAt(d.el) === null) to = d.unit.pinned ? { pin: false, line: null } : null;
      else {
        var flow = Array.prototype.filter.call(board.children, function (n) { return n !== d.el && (isCard(n) ? !n.hasAttribute("data-pinned") : topicAt(n) !== null); });
        var br = rectOf(board), f = slot(flow, y, br), after = flow[f.at - 1] || null, before = flow[f.at] || null;
        var still = !d.unit.pinned && d.el.parentNode === board && flowNext(d.el, -1) === after && flowNext(d.el, 1) === before;
        to = { pin: false, after: after, before: before, line: still ? null : f.line };
      }
    }
    d.to = to;
    if (zone) zone.classList.toggle("over", !!(to && to.pin && zr && inside(zr, x, y)));
    drawLine(to && to.line);
    d.scr = scr;
    var sr = rectOf(scr);
    d.edge = sr ? (y < sr.top + EDGE ? -1 : y > sr.bottom - EDGE ? 1 : 0) : 0;
    if (d.edge && !raf) raf = requestAnimationFrame(autoScroll);
  }
  // flowNext is the open-flow node beside n in the board (dir -1: above).
  function flowNext(n, dir) {
    for (var m = dir < 0 ? n.previousElementSibling : n.nextElementSibling; m; m = dir < 0 ? m.previousElementSibling : m.nextElementSibling) {
      if (isCard(m) ? !m.hasAttribute("data-pinned") : topicAt(m) !== null) return m;
    }
    return null;
  }
  function autoScroll() {
    raf = 0;
    var d = dg;
    if (!d || !d.on || !d.edge || !d.scr) return;
    var was = d.scr.scrollTop;
    d.scr.scrollTop += d.edge * 8;
    if (d.scr.scrollTop !== was) over(d.x, d.y);
  }
  function dragStart(e) {
    if (overviewOn || dg || !e.isPrimary || (e.button !== undefined && e.button !== 0)) return;
    var t = e.target;
    if (!t.closest || t.closest("button, a, input, textarea, select, summary, label")) return;
    var head = t.closest(".tbar, .bc-hd");
    if (!head) return;
    var el = head.classList.contains("bc-hd") ? head.closest(".bcard") : head.closest("section.topic");
    if (!el || el.hasAttribute("data-max") || !(isCard(el) || topicName(el) !== null)) return;
    var u = unitOf(el);
    if (!u) return;
    dg = { el: el, head: head, unit: u, id: e.pointerId, x0: e.clientX, y0: e.clientY, x: e.clientX, y: e.clientY, on: false, touch: e.pointerType === "touch", timer: 0, to: null, edge: 0, scr: null, mode: "" };
    if (dg.touch) dg.timer = setTimeout(function () { if (dg && !dg.on) dragBegin(); }, HOLD);
  }
  function dragBegin() {
    var d = dg;
    if (!d.el.isConnected) { dg = null; return; }
    d.on = true;
    d.mode = colOn ? "col" : col && opt.shows && opt.shows(pinned().length + (d.unit.pinned ? 0 : 1)) ? "edge" : "group";
    try { d.head.setPointerCapture(d.id); } catch (x) { /* no capture: the moves still land on the document */ }
    d.el.classList.add("bdrag");
    doc.documentElement.setAttribute("data-bdrag", d.mode);
    if (opt.drag) opt.drag(true);
    var sel = window.getSelection && window.getSelection();
    if (sel && sel.removeAllRanges) sel.removeAllRanges();
    over(d.x, d.y);
  }
  function dragMove(e) {
    var d = dg;
    if (!d || e.pointerId !== d.id) return;
    // A mouse move with the button up is a release this page never heard.
    if (e.pointerType === "mouse" && e.buttons !== undefined && (e.buttons & 1) === 0) { dragEnd(null); return; }
    if (!d.on) {
      if (Math.abs(e.clientX - d.x0) <= SLOP && Math.abs(e.clientY - d.y0) <= SLOP) return;
      // A touch that moves before its hold is a scroll.
      if (d.touch) { dragEnd(null); return; }
      d.x = e.clientX; d.y = e.clientY;
      dragBegin();
      if (!dg) return;
    }
    e.preventDefault();
    over(e.clientX, e.clientY);
  }
  // dragEnd ends the press or drag; only a release drops (a cancel leaves
  // everything where it stood). A drag's click is swallowed.
  function dragEnd(e) {
    var d = dg;
    if (!d || (e && e.pointerId !== d.id)) return;
    dg = null;
    clearTimeout(d.timer);
    if (!d.on) return;
    d.el.classList.remove("bdrag");
    doc.documentElement.removeAttribute("data-bdrag");
    drawLine(null);
    if (zone) zone.classList.remove("over");
    dragged = true;
    setTimeout(function () { dragged = false; }, 0);
    var to = d.to;
    if (e && e.type === "pointerup" && to && d.el.isConnected && (to.line || to.pin !== !!d.unit.pinned)) {
      to.el = d.el;
      drop(d.unit, to);
    }
    if (opt.drag) opt.drag(false);
  }

  // --- the pinned column ----------------------------------------------------------
  // The page hands an empty element (opt.column's host); the framework
  // fills it: a width handle on its left edge,
  // where it meets the board, and the pin list while the column shows. The
  // column fills the window right of the board, so the handle sizes the
  // board (--boardw, the width of the board's .dgrid cell, kept with the
  // pins; double click goes back to the stylesheet's): at least BOARDW_MIN,
  // and at most what leaves the column PINW_MIN.
  function setWidth(w) {
    boardw = w ? Math.max(BOARDW_MIN, Math.round(w)) : 0;
    if (boardw) doc.documentElement.style.setProperty("--boardw", boardw + "px");
    else doc.documentElement.style.removeProperty("--boardw");
  }
  function boardCell() { return board.closest && board.closest(".dgrid"); }
  function buildCol() {
    edgeHandle();
  }
  function edgeHandle() {
    var edge = mk("div", "bp-edge"), from = null;
    edge.setAttribute("data-edge", "left");
    edge.setAttribute("role", "separator");
    edge.setAttribute("aria-orientation", "vertical");
    edge.title = wsT("js.pins.width");
    edge.setAttribute("aria-label", wsT("js.pins.width"));
    col.appendChild(edge);
    edge.addEventListener("pointerdown", function (e) {
      var cell = boardCell(), w = cell ? cell.getBoundingClientRect().width : 0;
      if ((e.button !== undefined && e.button !== 0) || !w) return;
      from = { x: e.clientX, w: w, max: w + col.getBoundingClientRect().width - PINW_MIN };
      edge.classList.add("drag");
      try { edge.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the move still lands */ }
      e.preventDefault();
    });
    edge.addEventListener("pointermove", function (e) {
      if (from) setWidth(Math.min(from.w + e.clientX - from.x, Math.max(from.max, BOARDW_MIN)));
    });
    function end() { if (!from) return; from = null; edge.classList.remove("drag"); persist(); }
    edge.addEventListener("pointerup", end);
    edge.addEventListener("pointercancel", end);
    edge.addEventListener("dblclick", function () { setWidth(0); persist(); });
  }
  // fetchMissing finds the pinned topics the served board lacks (closed
  // past its cap) through the closed route, page by page from the board's
  // show-more control, until each is found or the route ends: one the
  // route ends without is gone from the store, and its pin with it. A
  // failed fetch keeps the pins for the next load, and so does a board
  // with no show-more control: an empty or not yet loaded board cannot
  // tell a topic past its cap from one that is gone, so only the route's
  // answer drops a pin.
  function fetchMissing() {
    function missing() { return Object.keys(topics).filter(function (k) { return topics[k].pinned && !topics[k].el; }); }
    function gone(list) { list.forEach(function (k) { delete topics[k]; }); persist(); }
    function next(box) {
      var b = box.querySelector("button.closed-more[data-closed-next]");
      return b ? +b.getAttribute("data-closed-next") : 0;
    }
    function page(from) {
      opt.closed(from, function (box) {
        if (!box) return;
        var got = [];
        Array.prototype.forEach.call(box.querySelectorAll("section.topic[data-topic]"), function (t) {
          var r = topics[t.getAttribute("data-topic")];
          if (r && r.pinned && !r.el) { r.el = t; got.push(t); }
        });
        if (got.length) {
          arrange(null);
          if (opt.adopted) got.forEach(opt.adopted);
        }
        var left = missing(), n = next(box);
        if (!left.length) return;
        if (n) page(n); else gone(left);
      });
    }
    var miss = missing();
    if (!miss.length) return;
    var from = next(board);
    if (from && opt.closed) page(from);
  }

  // --- the module -----------------------------------------------------------------
  function add(kind, rec) {
    var c = new Card(kind, rec);
    cards.push(c);
    c.k.open(c);
    c.paint();
    return c;
  }
  window.WsCards = {
    icon: licon,
    register: function (name, kind) { kinds[name] = kind; },
    // mount starts the framework on the board region el and restores the
    // stored cards and topics. o: key (the storage key), load and save (the
    // page's guarded storage), reveal (shows the board column where the
    // layout shows one column at a time), and for the pinned column:
    // column (the empty element it fills; optional), shows(n) (whether the
    // page shows the column for n pins), layout(on) (the column shows or
    // hides), closed(from, done) (fetches the closed-topic page from index
    // from and hands its root to done, or null on a failure), adopted(node)
    // (a topic node the closed route brought, now placed), and drag(on) (a
    // drag starts or ends: board updates wait meanwhile). toolbar supplies
    // overview(), search(), and optional files() visibility notification
    // for the toolbar's existing explorer sheet.
    mount: function (el, o) {
      board = el;
      opt = o;
      col = o.column || null;
      overviewOn = false;
      buildToolbar();
      pins = mk("div", "bpins");
      pins.id = "bpins";
      pins.setAttribute("data-pins", "");
      pins.setAttribute("data-label", wsT("js.pins.title"));
      pins.setAttribute("data-drop", wsT("js.pins.drop"));
      if (col) {
        buildCol();
        zone = doc.body.appendChild(mk("div", "bpzone", wsT("js.pins.drop")));
        zone.setAttribute("aria-hidden", "true");
      }
      var s = stored();
      setWidth(s.bw);
      s.topics.forEach(function (r) { topics[r.n] = { name: r.n, pinned: +r.p || 0, at: +r.a || 0, bar: r.b === null ? null : +r.b || 0, folded: !!r.f, rank: r.rank, el: null }; });
      s.cards.forEach(function (r) { add(r.kind, r); });
      // The pin list enters the page before anything moves into it, so a
      // topic pinned on the board moves into it with its frames kept.
      board.insertBefore(pins, board.firstChild);
      board.insertBefore(toolbar, pins);
      place(weave(Array.prototype.filter.call(board.childNodes, function (n) { return !fixedNode(n); })), fixedNode);
      persist();
      [board, col].forEach(function (box) {
        if (!box) return;
        box.addEventListener("pointerdown", dragStart);
        box.addEventListener("click", function (e) {
          var b = e.target.closest && e.target.closest(".tpin");
          if (b && b.parentNode.classList.contains("tbar")) pinTopic(b.parentNode.closest("section.topic"), b.getAttribute("aria-pressed") !== "true");
          var action = e.target.closest && e.target.closest(".t-fold, .t-raise");
          if (action && action.parentNode.classList.contains("tbar")) {
            var u = topicUnit(action.closest("section.topic"));
            if (action.classList.contains("t-fold")) foldUnit(u, !u.folded); else raise(u);
          }
          sheetClick(e);
        });
        box.addEventListener("contextmenu", function (e) { if (dg && dg.touch) e.preventDefault(); });
      });
      doc.addEventListener("pointermove", dragMove);
      doc.addEventListener("pointerup", dragEnd);
      doc.addEventListener("pointercancel", dragEnd);
      doc.addEventListener("click", function (e) { if (dragged) { e.preventDefault(); e.stopPropagation(); } }, true);
      // A held touch is a drag, not a scroll.
      doc.addEventListener("touchmove", function (e) { if (dg && dg.on) e.preventDefault(); }, { passive: false });
      floatToolbar();
      fetchMissing();
    },
    // open adds a card of a registered kind at the head of the unpinned
    // flow; data is the kind's record. at, when given, is the card's open
    // time (epoch ms) instead: a card for something that began earlier
    // stands where a card opened then would.
    open: function (kind, data, at, origin) {
      if (!board || !kinds[kind]) return null;
      var c = add(kind, { data: data, at: at });
      if (origin) openBefore(c, origin);
      arrange(null);
      persist();
      return c;
    },
    list: function (kind) { return cards.filter(function (c) { return !kind || c.kind === kind; }); },
    // weave and settle are the board morph's two steps (dashboard.js
    // morphBoard): weave takes the nodes the board wants and returns the
    // placement, {pins, board} (the pin list's nodes and the board
    // region's, in order); settle settles both around the cards.
    weave: weave,
    settle: function (el, p) { place(p, fixedNode); persist(); },
    // pinList is the pin list's node; pinnedTopic tells whether a topic
    // name is pinned on this page (its node stands in the pin list).
    pinList: function () { return pins; },
    toolbar: function () { return toolbar; },
    // Hold the one pin list under the toolbar without changing colOn or
    // the page's width. Overview's view state and interactions live elsewhere.
    overview: function (on) { overviewOn = !!on; if (board) arrange(null); },
    pinnedTopic: function (name) { var r = topics[name]; return !!(r && r.pinned && r.el); },
    // relayout places everything again after the page's layout changed
    // (a width crossing, the layout menu).
    relayout: function () { if (board) arrange(null); },
    dragging: function () { return !!(dg && dg.on); },
    // move places a topic's or card's node as a drop does (to: pin and
    // index, or after and before in the open flow).
    move: function (n, to) { var u = unitOf(n); if (u) drop(u, to); },
    // decorate gives topics the page added outside a board update (a
    // fetched closed page) their pin buttons.
    decorate: function () { if (board) decorate(); },
    is: isCard
  };
})();
