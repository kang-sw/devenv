(function () {
  // Overview is a view, never a card fold or a saved card arrangement. The
  // page owns swaps and the shell bridge; this module owns the picker and
  // its one scroll writer, wbHistory.keep.
  var doc = document, root = doc.documentElement, o = null, on = false, busy = false;
  var entry = null, spacer = null, pinTop = 0, tabs = new Map(), callbacks = [];
  var ITEM = "section.topic[data-topic], .bcard[data-card]";
  function all(sel, box) { return Array.prototype.slice.call((box || doc).querySelectorAll(sel)); }
  function identity(n) { return n && (n.hasAttribute("data-card") ? { card: n.getAttribute("data-card") } : { topic: n.getAttribute("data-topic") }); }
  function find(key) {
    if (!key) return null;
    return all(ITEM, o.board).concat(all(ITEM, o.pins)).filter(function (n) {
      return key.card ? n.getAttribute("data-card") === key.card : n.getAttribute("data-topic") === key.topic;
    })[0] || null;
  }
  function scroller(n) {
    for (var p = n.parentElement; p && p !== doc.body && p !== root; p = p.parentElement) {
      if (/auto|scroll|overlay/.test(getComputedStyle(p).overflowY) && p.scrollHeight > p.clientHeight) return p;
    }
    return doc.scrollingElement || root;
  }
  function view(s) {
    return s === doc.scrollingElement || s === root ? { top: 0, bottom: innerHeight, height: innerHeight } :
      { top: s.getBoundingClientRect().top + s.clientTop, bottom: s.getBoundingClientRect().top + s.clientTop + s.clientHeight, height: s.clientHeight };
  }
  function items() { return all(ITEM, o.board); }
  function anchor(list, toolbar, bounds) {
    if (toolbar.bottom > bounds.top && toolbar.top < bounds.bottom) return null;
    return list.filter(function (n) { var r = n.getBoundingClientRect(); return r.bottom > bounds.top && r.top < bounds.bottom; })[0] || null;
  }
  function classify(t) {
    if (!t || !t.closest) return "outside";
    if (t.closest(".board-toolbar")) return "toolbar";
    if (t.closest(".tpop:not([hidden]), [popover]:popover-open")) return "popover";
    if (t.closest("#tkqpop, dialog[open], .tk-iwin-back:not([hidden])")) return "palette";
    if (t.closest(ITEM) && o.board.contains(t)) return "item";
    return "outside";
  }
  function layer() {
    if (doc.querySelector("#tkqpop[open], dialog[open]")) return "palette";
    if (doc.querySelector(".tpop:not([hidden])")) return "popover";
    return "overview";
  }
  function readState(raw) {
    try {
      var v = JSON.parse(raw);
      if (v && v.on === true && isFinite(v.offset) && (!v.key || typeof v.key.card === "string" || typeof v.key.topic === "string")) return v;
    } catch (e) { /* storage is optional */ }
    return null;
  }
  function stored() { try { return readState(sessionStorage.getItem("ws.dashboard.overview." + o.rid)); } catch (e) { return null; } }
  function save() {
    try {
      var key = "ws.dashboard.overview." + o.rid;
      if (on) sessionStorage.setItem(key, JSON.stringify({ on: true, key: entry.key, offset: entry.offset, pinTop: pinTop }));
      else sessionStorage.removeItem(key);
    } catch (e) { /* denied storage must not disable the view */ }
  }
  function keep(n, offset) { return o.keep(n, { offset: offset, observe: [o.board, o.pins] }); }
  function flash(n) {
    n.classList.remove("overview-flash");
    void n.offsetWidth;
    n.classList.add("overview-flash");
    setTimeout(function () { n.classList.remove("overview-flash"); }, 1200);
  }
  function rove(n) {
    items().forEach(function (x) {
      if (!tabs.has(x)) tabs.set(x, x.getAttribute("tabindex"));
      x.setAttribute("tabindex", x === n ? "0" : "-1");
    });
    if (n) n.focus({ preventScroll: true });
  }
  function unrove() {
    tabs.forEach(function (v, n) { if (v === null) n.removeAttribute("tabindex"); else n.setAttribute("tabindex", v); });
    tabs.clear();
  }
  function hint() {
    try {
      if (sessionStorage.getItem("ws.dashboard.overview.hint")) return;
      sessionStorage.setItem("ws.dashboard.overview.hint", "1");
    } catch (e) { /* show the hint if storage is denied */ }
    if (o.toast) o.toast(wsT("js.board.overview.hint"));
  }
  function changed() { if (o.changed) o.changed(); }
  function finish() {
    busy = false;
    if (!on && o.release) o.release();
    var c = callbacks.splice(0);
    c.forEach(function (fn) { fn(); });
  }
  function calm() { return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches); }
  // The one cubic-bezier(.2,.8,.2,1) clock drives heights, the slot, scroll
  // compensation and flights. Exit traverses that same timeline backwards.
  function ease(p) {
    var lo = 0, hi = 1, t = p;
    for (var i = 0; i < 16; i++) {
      t = (lo + hi) / 2;
      var x = 0.6 * t * (1 - t) + t * t * t;
      if (x < p) lo = t; else hi = t;
    }
    return p <= 0 ? 0 : p >= 1 ? 1 : 2.4 * t * (1 - t) * (1 - t) + 3 * t * t * (1 - t) + t * t * t;
  }
  function snapshot() {
    var list = items().concat(all(ITEM, o.pins)), seen = new Set();
    return list.filter(function (n) { if (seen.has(n)) return false; seen.add(n); return true; }).map(function (n) {
      var r = n.getBoundingClientRect(), text = n.querySelector(".trecap .rtext");
      return { n: n, r: r, height: r.height, margin: parseFloat(getComputedStyle(n).marginBottom) || 0,
        text: text, recap: text ? text.getBoundingClientRect().height : 0 };
    });
  }
  function visible(r) { return r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth; }
  function ghost(n, r) {
    var g = doc.createElement("div"), head = n.querySelector(".tophead, .bc-hd");
    g.className = "overview-ghost";
    g.setAttribute("aria-hidden", "true");
    g.setAttribute("data-overview-target", JSON.stringify(identity(n)));
    if (head) {
      var copy = head.cloneNode(true);
      all("iframe, textarea, input, .doc-frame", copy).forEach(function (x) { x.remove(); });
      [copy].concat(all("[id]", copy)).forEach(function (x) { x.removeAttribute("id"); });
      g.appendChild(copy);
    }
    var body = doc.createElement("div");
    body.className = "overview-ghost-body";
    g.appendChild(body);
    doc.body.appendChild(g);
    g.style.left = r.left + "px"; g.style.top = r.top + "px"; g.style.width = r.width + "px"; g.style.height = r.height + "px";
    return g;
  }
  function motion(before, flights, entering, node, from, to, done) {
    var end = snapshot(), pinEndHeight = o.pins.getBoundingClientRect().height, v = view(scroller(node)), rows = [], clones = [];
    before.forEach(function (a) {
      var b = end.filter(function (x) { return x.n === a.n; })[0];
      if (!b || a.r.bottom < v.top - v.height || a.r.top > v.bottom + v.height) return;
      var n = a.n;
      var old = { height: n.style.height, overflow: n.style.overflow, boxSizing: n.style.boxSizing, marginBottom: n.style.marginBottom };
      rows.push({ a: a, b: b, old: old, textHeight: a.text && a.text.style.maxHeight });
      n.setAttribute("data-overview-motion", "");
      n.style.boxSizing = "border-box";
      n.style.overflow = "hidden";
      n.style.height = a.height + "px";
      n.style.marginBottom = a.margin + "px";
      if (a.text) a.text.style.maxHeight = a.recap + "px";
    });
    var slot = null, slotFrom = 0, slotTo = 0, pinStyle = null;
    if (flights.length) {
      if (entering) {
        slot = o.pins; slotTo = pinEndHeight;
        pinStyle = { height: slot.style.height, overflow: slot.style.overflow };
      } else {
        slot = doc.createElement("div"); slot.className = "overview-slot";
        o.board.insertBefore(slot, o.toolbar.nextSibling);
        slotFrom = flights.height;
      }
      slot.style.overflow = "hidden";
      slot.style.height = slotFrom + "px";
      flights.forEach(function (a) {
        var b = end.filter(function (x) { return x.n === a.n; })[0];
        if (!b) return;
        var g = ghost(a.n, a.r), text = g.querySelector(".rtext");
        clones.push({ n: a.n, g: g, a: a, b: b, text: text, vis: a.n.style.visibility, fly: visible(a.r) && visible(b.r) });
        a.n.style.visibility = "hidden";
      });
    }
    var started = performance.now();
    function frame(now) {
      var p = Math.min(1, Math.max(0, (now - started) / 240)), e = entering ? ease(p) : 1 - ease(1 - p);
      rows.forEach(function (r) {
        r.a.n.style.height = (r.a.height + (r.b.height - r.a.height) * e) + "px";
        r.a.n.style.marginBottom = (r.a.margin + (r.b.margin - r.a.margin) * e) + "px";
        if (r.a.text) r.a.text.style.maxHeight = (r.a.recap + (r.b.recap - r.a.recap) * e) + "px";
      });
      if (slot) slot.style.height = (slotFrom + (slotTo - slotFrom) * e) + "px";
      keep(node, from + (to - from) * e);
      clones.forEach(function (c) {
        var r = c.n.getBoundingClientRect(), a = c.a.r;
        c.g.style.left = (a.left + (r.left - a.left) * e) + "px";
        c.g.style.top = (a.top + (r.top - a.top) * e) + "px";
        c.g.style.width = (a.width + (r.width - a.width) * e) + "px";
        c.g.style.height = (a.height + (r.height - a.height) * e) + "px";
        if (c.text) c.text.style.maxHeight = (c.a.recap + (c.b.recap - c.a.recap) * e) + "px";
        if (!c.fly) { c.g.style.opacity = String(1 - e); c.n.style.visibility = c.vis; c.n.style.opacity = String(e); }
      });
      if (p < 1) { requestAnimationFrame(frame); return; }
      // At this instant each ghost equals its real, live target. Remove it
      // only after the final keep application, so no stale target rect wins.
      clones.forEach(function (c) {
        c.g.dispatchEvent(new CustomEvent("ws-overview-land", { bubbles: true, detail: { target: c.n, from: c.g.getBoundingClientRect(), to: c.n.getBoundingClientRect() } }));
        c.g.remove(); c.n.style.visibility = c.vis; c.n.style.opacity = "";
      });
      rows.forEach(function (r) {
        r.a.n.removeAttribute("data-overview-motion");
        Object.keys(r.old).forEach(function (k) { r.a.n.style[k] = r.old[k]; });
        if (r.a.text) r.a.text.style.maxHeight = r.textHeight;
      });
      if (slot) {
        if (entering) { slot.style.height = pinStyle.height; slot.style.overflow = pinStyle.overflow; }
        else slot.remove();
      }
      keep(node, to);
      done();
    }
    frame(started);
  }
  // The instant path is also the reduced-motion end-state contract. Motion
  // interpolates around it without inventing a second set of states.
  function enter(state) {
    if (!o || on || busy || doc.querySelector(".bcard[data-max], section.topic[data-max], details.entry-panel[data-max]")) return;
    var s = scroller(o.toolbar), v = view(s), n = anchor(items(), o.toolbar.getBoundingClientRect(), v);
    entry = state || { key: identity(n), offset: (n || o.toolbar).getBoundingClientRect().top - v.top };
    if (state && entry.key && !find(entry.key)) entry = { key: null, offset: 0 };
    pinTop = state ? +state.pinTop || 0 : o.pins.scrollTop;
    n = find(entry.key);
    var h = o.frames ? o.frames() : null, before = !state && !calm() ? snapshot() : null;
    var flights = before && o.pins.parentNode !== o.board ? before.filter(function (a) { return o.pins.contains(a.n); }) : [];
    busy = !!before;
    // Keep enough tail to reach even a partly-visible last topic's negative
    // offset. Added before folding, removed only after unfolding inside keep.
    spacer = doc.createElement("div");
    spacer.className = "overview-spacer";
    spacer.setAttribute("aria-hidden", "true");
    spacer.style.height = Math.max(v.height, v.height - entry.offset) + "px";
    o.board.appendChild(spacer);
    on = true;
    root.setAttribute("data-overview", "");
    if (n) n.classList.add("overview-here");
    o.place(true);
    if (o.restoreFrames) o.restoreFrames(h);
    o.toolbar.querySelector(".board-overview").setAttribute("aria-pressed", "true");
    save(); changed();
    var complete = function () { rove(n || items()[0]); finish(); if (!state) hint(); };
    if (before) motion(before, flights, true, n || o.toolbar, entry.offset, entry.offset, complete);
    else { keep(n || o.toolbar, entry.offset); complete(); }
  }
  function exit(target, done, options) {
    if (!o || !on) { if (done) done(); return; }
    if (busy) { if (done) callbacks.push(done); return; }
    var n = target && target.closest ? target.closest(ITEM) : null;
    var h = o.frames ? o.frames() : null, before = !calm() ? snapshot() : null;
    var pinRect = o.pins.getBoundingClientRect();
    var flights = before && root.hasAttribute("data-pincol") ? before.filter(function (a) { return o.pins.contains(a.n); }) : [];
    flights.height = pinRect.height;
    var restore = find(entry.key), planned = n && !(root.hasAttribute("data-pincol") && o.pins.contains(n)) ? n : restore;
    if (planned && root.hasAttribute("data-pincol") && o.pins.contains(planned)) planned = null;
    var from = (planned || o.toolbar).getBoundingClientRect().top - view(scroller(planned || o.toolbar)).top;
    busy = !!before;
    on = false;
    root.removeAttribute("data-overview");
    unrove();
    o.place(false);
    if (o.restoreFrames) o.restoreFrames(h);
    // A pinned target lands in the column that is current now, not the
    // layout at entry. The other column returns to its saved scroll too.
    var inColumn = o.pins.parentNode !== o.board;
    if (inColumn && o.pins.scrollHeight > o.pins.clientHeight && o.pins.firstElementChild) {
      var first = o.pins.firstElementChild;
      var natural = first.getBoundingClientRect().top - view(scroller(first)).top + o.pins.scrollTop;
      keep(first, natural - pinTop);
    }
    var landed = n && n.isConnected ? n : null;
    var boardTarget = landed && !(inColumn && o.pins.contains(landed)) ? landed : restore;
    if (boardTarget && inColumn && o.pins.contains(boardTarget)) boardTarget = null;
    var offset = landed && boardTarget === landed ? 0 : boardTarget && restore === boardTarget ? entry.offset : entry.key ? 0 : entry.offset;
    if (landed && inColumn && o.pins.contains(landed) && scroller(landed) === o.pins) keep(landed, 0);
    o.toolbar.querySelector(".board-overview").setAttribute("aria-pressed", "false");
    save(); changed();
    var complete = function () {
      if (spacer) { spacer.remove(); spacer = null; }
      keep(boardTarget || o.toolbar, offset);
      if (landed) {
        flash(landed);
        if (!(options && options.keep)) { landed.setAttribute("tabindex", "-1"); landed.focus({ preventScroll: true }); }
      } else if (!(options && options.keep)) o.toolbar.querySelector(".board-overview").focus({ preventScroll: true });
      all(".overview-here", o.board).forEach(function (x) { x.classList.remove("overview-here"); });
      finish();
      if (done) done();
    };
    if (before) motion(before, flights, false, boardTarget || o.toolbar, from, offset, complete);
    else { keep(boardTarget || o.toolbar, offset); complete(); }
  }
  function swallow(e) { e.preventDefault(); e.stopImmediatePropagation(); }
  function click(e) {
    if (!on && !busy) return;
    if (busy) { swallow(e); return; }
    var c = classify(e.target);
    if (c === "toolbar" || c === "popover" || c === "palette") return;
    swallow(e);
    exit(c === "item" ? e.target : null);
  }
  function key(e) {
    if (!on && !busy) return;
    if (e.isComposing) return;
    if (e.code === "Backquote" && e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey) {
      swallow(e); exit(null, o.rail); return;
    }
    if (layer() !== "overview") return;
    if (e.key === "Escape") { swallow(e); if (!busy) exit(null); return; }
    if (busy) { if (e.key === "Enter") swallow(e); return; }
    if (classify(e.target) === "toolbar") return;
    var list = items(), i = list.indexOf(doc.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      swallow(e);
      var next = list[Math.max(0, Math.min(list.length - 1, i < 0 ? 0 : i + (e.key === "ArrowDown" ? 1 : -1)))];
      rove(next);
      if (next) {
        var r = next.getBoundingClientRect(), v = view(scroller(next));
        if (r.top < v.top || r.bottom > v.bottom) keep(next, r.top < v.top ? 0 : Math.max(0, v.height - r.height));
      }
    } else if (e.key === "Enter" && i >= 0) { swallow(e); exit(list[i]); }
  }
  window.WsOverview = {
    mount: function (options) {
      o = options;
      doc.addEventListener("click", click, true);
      // Prevent mousedown focusing the dimmed control before its click is
      // captured. Do not exit until click, or that click could activate it.
      doc.addEventListener("mousedown", function (e) { if ((on || busy) && (busy || classify(e.target) === "outside" || classify(e.target) === "item")) e.preventDefault(); }, true);
      doc.addEventListener("keydown", key, true);
      var state = stored();
      if (state) requestAnimationFrame(function () { if (o.reveal) o.reveal(); enter(state); });
    },
    active: function () { return on; },
    holding: function () { return on || busy; },
    toggle: function () { if (on) exit(null); else enter(null); },
    exit: exit,
    after: function (fn) { if (on) exit(null, fn); else if (busy) callbacks.push(fn); else fn(); },
    pick: function (n, options) { if (!on) return false; exit(n, null, options); return true; }
  };
})();
