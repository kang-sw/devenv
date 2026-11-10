(function () {
  // The board card framework (window.WsCards): cards the page itself puts in
  // the board column, among the board's topics. The framework owns what
  // every card kind shares: the header's pin, fold, maximize, and close
  // buttons, the body's base height and its drag handle, the maximize sheet
  // with Escape, where a card stands in the board flow, and the record of
  // all of that in browser storage. A kind supplies the header's content,
  // its own header actions, the body, the folded content, and its
  // lifecycle, and never places or sizes a card itself.
  //
  // A kind registers before the page mounts the framework:
  //
  //   WsCards.register(name, {
  //     base: 260,            // the body's base height in px (optional)
  //     open(card) {},        // build card.body, call card.title(...)
  //     close(card) {},       // the card is going away (optional)
  //     folded(card) {},      // a node shown under a folded header, or null (optional)
  //     sized(card) {},       // the body's size changed (optional)
  //     pinned(card) {},      // the card was pinned or unpinned (optional)
  //   })
  //
  // and works through the card it is handed: card.data (the kind's own
  // record, kept in storage: plain JSON), card.body, card.title, card.actions,
  // card.save, card.bump, card.show, card.fold, card.close, card.pinned.
  //
  // Placement is one rule for every kind. Pinned cards stand first, in the
  // order they were pinned. An unpinned card enters at the head of the
  // unpinned flow when it opens (bump counts as an open); after that it
  // only moves down, as a topic updated after the card opened rises above
  // it. A card's open time is kept with its view state, so a reload puts it
  // back where it stood among the topics.
  //
  // A card's node is never moved by a board update: the board morph weaves
  // the cards into the nodes it wants (weave) and settles the board around
  // them (settle), so a frame or a video in a card keeps running. Only the
  // card the user pins, unpins, or reopens moves, and where the browser has
  // Element.moveBefore even that move keeps the card's state.
  var doc = document;
  var BASE = 260, MIN = 72, STEP = 24;
  var kinds = {}, cards = [], board = null, opt = null, maxed = null, scrim = null, seq = 0;

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

  // topicAt is an open topic's last update (epoch ms), or null for any
  // other node: a closed topic, the closed-topic control, the reply bar.
  function topicAt(n) {
    if (n.nodeType !== 1 || !n.hasAttribute("data-topic") || n.classList.contains("closed")) return null;
    var t = n.querySelector("time.tago[data-at]");
    return t ? +t.getAttribute("data-at") || 0 : 0;
  }
  // order weaves the cards in list into base, the board's own nodes in
  // order, by the placement rule; at reads a node's topicAt.
  function order(base, list, at) {
    var pins = list.filter(function (c) { return c.pinned; }).sort(function (a, b) { return a.pinned - b.pinned; });
    var flow = list.filter(function (c) { return !c.pinned; }).sort(function (a, b) { return b.at - a.at; });
    var out = pins.map(function (c) { return c.el; }), i = 0;
    base.forEach(function (n) {
      var t = at(n);
      // A card stands below every open topic updated after it opened, and
      // above everything else.
      while (i < flow.length && !(t !== null && t > flow[i].at)) out.push(flow[i++].el);
      out.push(n);
    });
    while (i < flow.length) out.push(flow[i++].el);
    return out;
  }
  // settle makes el's children the nodes in want, in order. Stale nodes go
  // first, so the kept ones that stay in order are never moved (moving a
  // frame reloads it). A fixed node already in el is never moved either:
  // the nodes standing before it that are wanted after it move past it.
  function settle(el, want, fixed) {
    var keep = new Set(want);
    Array.prototype.slice.call(el.childNodes).forEach(function (n) { if (!keep.has(n)) el.removeChild(n); });
    var cur = el.firstChild;
    want.forEach(function (n) {
      if (n === cur) { cur = cur.nextSibling; return; }
      if (fixed(n) && n.parentNode === el) {
        var after = n.nextSibling;
        while (cur !== n) { var next = cur.nextSibling; el.insertBefore(cur, after); cur = next; }
        cur = n.nextSibling;
        return;
      }
      el.insertBefore(n, cur);
    });
  }
  // headTime is an open time that puts a card at the head of the unpinned
  // flow: now, or just past the newest topic and card where the server's
  // clock (a topic's time) runs ahead of this browser's.
  function headTime() {
    var t = Date.now();
    if (board) Array.prototype.forEach.call(board.children, function (n) { var a = isCard(n) ? null : topicAt(n); if (a !== null && a >= t) t = a + 1; });
    cards.forEach(function (c) { if (!c.pinned && c.at >= t) t = c.at + 1; });
    return t;
  }

  function weave(base) { return order(base, cards, topicAt); }
  function baseNodes() { return Array.prototype.filter.call(board.childNodes, function (n) { return !isCard(n); }); }
  // arrange puts the cards in their places. moved is the one card whose
  // place changed: it alone moves, with its state kept where the browser
  // can keep it.
  function arrange(moved) {
    if (!board) return;
    var want = weave(baseNodes());
    if (moved && moved.el.parentNode === board) {
      var ref = want[want.indexOf(moved.el) + 1] || null;
      if (moved.el.nextSibling !== ref) {
        try { if (board.moveBefore) board.moveBefore(moved.el, ref); else board.insertBefore(moved.el, ref); }
        catch (e) { board.insertBefore(moved.el, ref); }
      }
    }
    settle(board, want, function () { return false; });
  }

  // --- storage ----------------------------------------------------------------
  function persist() {
    if (!opt) return;
    opt.save(opt.key, cards.length ? JSON.stringify(cards.map(function (c) {
      return { id: c.id, kind: c.kind, pinned: c.pinned, at: c.at, fold: c.folded, h: c.h, data: c.data };
    })) : null);
  }
  function stored() {
    var v = null;
    try { v = JSON.parse(opt.load(opt.key) || "[]"); } catch (e) { v = null; }
    return Array.isArray(v) ? v.filter(function (r) { return r && typeof r.kind === "string" && kinds[r.kind]; }) : [];
  }

  // --- a card -------------------------------------------------------------------
  function Card(kind, rec) {
    this.kind = kind;
    this.k = kinds[kind];
    this.id = rec.id || "c" + Date.now().toString(36) + (seq++).toString(36);
    this.pinned = +rec.pinned || 0;
    this.at = +rec.at || headTime();
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
    c.icon = mk("span", "bc-ic");
    c.name = mk("span", "bc-nm");
    c.sub = mk("span", "bc-sub");
    c.badge = mk("span", "bc-kind");
    c.acts = mk("span", "bc-acts");
    c.pinBtn = ibtn("bc-pin", "pin", wsT("js.card.pin"));
    c.foldBtn = ibtn("bc-fold", "chevron-down", wsT("js.card.fold"));
    c.maxBtn = ibtn("bc-max", "maximize-2", wsT("js.card.max"));
    c.minBtn = ibtn("bc-min", "minimize-2", wsT("js.card.min"));
    c.closeBtn = ibtn("bc-close", "x", wsT("js.card.close"));
    [c.icon, c.name, c.sub, c.badge, c.acts, c.pinBtn, c.foldBtn, c.maxBtn, c.minBtn, c.closeBtn].forEach(function (n) { head.appendChild(n); });
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
    c.maxBtn.addEventListener("click", function () { maximize(c); });
    c.minBtn.addEventListener("click", function () { maximize(null); });
    c.closeBtn.addEventListener("click", function () { c.close(); });
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
    c.body.style.height = (c.h || c.k.base || BASE) + "px";
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
  // actions sets the kind's own header buttons: [{icon, title, run}].
  Card.prototype.actions = function (list) {
    var c = this;
    c.acts.textContent = "";
    (list || []).forEach(function (a) {
      var b = ibtn("", a.icon, a.title);
      b.addEventListener("click", function () { a.run(c); });
      c.acts.appendChild(b);
    });
  };
  Card.prototype.save = function () { persist(); };
  Card.prototype.sized = function () { if (this.k.sized) this.k.sized(this); };
  Card.prototype.fold = function (on) {
    if (this.folded === !!on) return;
    this.folded = !!on;
    this.paint();
    persist();
    if (!this.folded) this.sized();
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
  Card.prototype.bump = function () {
    if (this.pinned) return;
    this.at = headTime();
    arrange(this);
    persist();
  };
  // show brings the card into view and marks it with a short flash.
  Card.prototype.show = function () {
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
  // video keeps running through maximize and back.
  function maximize(c) {
    if (maxed === c) return;
    if (maxed) {
      maxed.el.removeAttribute("data-max");
      maxed.el.style.height = "";
      scrim.hidden = true;
      var was = maxed;
      maxed = null;
      was.sized();
      if (was.maxBtn.isConnected) was.maxBtn.focus();
    }
    if (!c) return;
    if (!scrim) {
      scrim = mk("div", "bc-scrim");
      scrim.addEventListener("click", function () { maximize(null); });
      doc.body.appendChild(scrim);
    }
    c.el.style.height = c.el.getBoundingClientRect().height + "px";
    c.el.setAttribute("data-max", "");
    scrim.hidden = false;
    maxed = c;
    c.sized();
    c.minBtn.focus();
  }
  doc.addEventListener("keydown", function (e) {
    if (e.key !== "Escape" || !maxed || e.defaultPrevented) return;
    e.preventDefault();
    maximize(null);
  });

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
    // stored cards. o: key (the storage key), load and save (the page's
    // guarded storage), reveal (shows the board column where the layout
    // shows one column at a time).
    mount: function (el, o) {
      board = el;
      opt = o;
      stored().forEach(function (r) { add(r.kind, r); });
      arrange(null);
      persist();
    },
    // open adds a card of a registered kind at the head of the unpinned
    // flow; data is the kind's record.
    open: function (kind, data) {
      if (!board || !kinds[kind]) return null;
      var c = add(kind, { data: data });
      arrange(null);
      persist();
      return c;
    },
    list: function (kind) { return cards.filter(function (c) { return !kind || c.kind === kind; }); },
    // weave and settle are the board morph's two steps (dashboard.js
    // morphBoard): the cards woven into the nodes the board wants, and the
    // board settled around the cards.
    weave: weave,
    settle: function (el, want) { settle(el, want, isCard); },
    is: isCard
  };
})();
