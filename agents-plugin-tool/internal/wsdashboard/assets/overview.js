(function () {
  // Overview is a view, never a card fold or a saved card arrangement. The
  // page owns swaps and the shell bridge; this module owns the picker and
  // its one scroll writer, wbHistory.place (passed as o.keep). It covers
  // the board region only: docked units stay in the docking gallery
  // (cards.js) as they stand.
  var doc = document, root = doc.documentElement, o = null, on = false, busy = false;
  var entry = null, spacer = null, tabs = new Map(), callbacks = [];
  var ITEM = "section.topic[data-topic], .bcard[data-card]";
  function all(sel, box) { return Array.prototype.slice.call((box || doc).querySelectorAll(sel)); }
  function identity(n) { return n && (n.hasAttribute("data-card") ? { card: n.getAttribute("data-card") } : { topic: n.getAttribute("data-topic") }); }
  function find(key) {
    if (!key) return null;
    return all(ITEM, o.board).filter(function (n) {
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
  // Native scroll padding includes the live face and any fixed host controls.
  // Overview uses the same exposed viewport through its existing keep writer.
  function clearance(n) {
    var s = scroller(n), style = getComputedStyle(s), pad = parseFloat(style.scrollPaddingTop) || 0;
    var head = n.querySelector(n.hasAttribute("data-topic") ? ":scope > .tophead" : "[data-rail-bar]");
    if (!head) return pad;
    // The rail still publishes the capsule floor when Overview makes heads static.
    var rail = parseFloat(getComputedStyle(head).getPropertyValue("--wb-rail-top"));
    if (!isFinite(rail)) return pad;
    var padding = s === doc.scrollingElement || s === root ? 0 : parseFloat(style.paddingTop) || 0;
    return Math.max(pad, rail + padding);
  }
  function items() { return all(ITEM, o.board).filter(function (n) { return !n.hasAttribute("data-state-filtered"); }); }
  function anchor(list, toolbar, bounds, floating) {
    if (!floating && toolbar.bottom > bounds.top && toolbar.top < bounds.bottom) return null;
    var top = floating ? Math.max(bounds.top, toolbar.bottom) : bounds.top;
    return list.filter(function (n) { var r = n.getBoundingClientRect(); return r.bottom > top && r.top < bounds.bottom; })[0] || null;
  }
  function classify(t) {
    if (!t || !t.closest) return "outside";
    if (t.closest(".board-toolbar")) return "toolbar";
    if (t.closest(".tpop:not([hidden]), [popover]:popover-open")) return "popover";
    if (t.closest("#tkqpop, #tkqs, #shandle, dialog[open], .tk-iwin-back:not([hidden])")) return "palette";
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
      if (on) sessionStorage.setItem(key, JSON.stringify({ on: true, key: entry.key, offset: entry.offset }));
      else sessionStorage.removeItem(key);
    } catch (e) { /* denied storage must not disable the view */ }
  }
  function keep(n, offset) { return o.keep(n, { offset: offset, observe: [o.board] }); }
  function flash(n) {
    n.classList.remove("overview-flash");
    void n.offsetWidth;
    n.classList.add("overview-flash");
    setTimeout(function () { n.classList.remove("overview-flash"); }, 1200);
  }
  function rove(n, focus) {
    tabs.forEach(function (v, x) { if (x.hasAttribute("data-state-filtered")) x.setAttribute("tabindex", "-1"); });
    items().forEach(function (x) {
      if (!tabs.has(x)) tabs.set(x, x.getAttribute("tabindex"));
      x.setAttribute("tabindex", x === n ? "0" : "-1");
    });
    if (n && focus !== false) n.focus({ preventScroll: true });
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
  // Unlike after(), a filter waits for motion without requesting an exit.
  function filter(run) {
    if (busy) { callbacks.push(function () { filter(run); }); return; }
    if (!on) { run(); return; }
    var before = all(ITEM, o.board), list = items(), current = list.filter(function (n) { return n.getAttribute("tabindex") === "0"; })[0];
    var index = before.indexOf(current), v = view(scroller(o.toolbar)), face = o.toolbar.firstChild.getBoundingClientRect();
    var top = o.toolbar.hasAttribute("data-floating") ? Math.max(v.top, face.bottom) : v.top;
    var rows = list.map(function (n) { return { n: n, r: n.getBoundingClientRect() }; }).filter(function (a) { return a.r.bottom > top && a.r.top < v.bottom; });
    var toolbarOffset = o.toolbar.getBoundingClientRect().top - v.top;
    run();
    list = items();
    var next = list.indexOf(current) >= 0 ? current : list.filter(function (n) { return before.indexOf(n) >= index; })[0] || list[list.length - 1];
    rove(next, false);
    var row = rows.filter(function (a) { return list.indexOf(a.n) >= 0; })[0];
    if (row) keep(row.n, row.r.top - v.top);
    else if (list.length) keep(list[0], clearance(list[0]));
    else keep(o.toolbar, Math.max(0, toolbarOffset));
    save();
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
    var list = items(), seen = new Set();
    return list.filter(function (n) { if (seen.has(n) || n.hasAttribute("data-state-filtered")) return false; seen.add(n); return true; }).map(function (n) {
      var r = n.getBoundingClientRect(), text = n.querySelector(".trecap .rtext");
      return { n: n, r: r, height: r.height, margin: parseFloat(getComputedStyle(n).marginBottom) || 0,
        text: text, recap: text ? text.getBoundingClientRect().height : 0 };
    });
  }
  function motion(before, entering, node, from, to, done) {
    var end = snapshot(), v = view(scroller(node)), rows = [];
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
    var started = performance.now();
    function frame(now) {
      var p = Math.min(1, Math.max(0, (now - started) / 240)), e = entering ? ease(p) : 1 - ease(1 - p);
      rows.forEach(function (r) {
        r.a.n.style.height = (r.a.height + (r.b.height - r.a.height) * e) + "px";
        r.a.n.style.marginBottom = (r.a.margin + (r.b.margin - r.a.margin) * e) + "px";
        if (r.a.text) r.a.text.style.maxHeight = (r.a.recap + (r.b.recap - r.a.recap) * e) + "px";
      });
      keep(node, from + (to - from) * e);
      if (p < 1) { requestAnimationFrame(frame); return; }
      rows.forEach(function (r) {
        r.a.n.removeAttribute("data-overview-motion");
        Object.keys(r.old).forEach(function (k) { r.a.n.style[k] = r.old[k]; });
        if (r.a.text) r.a.text.style.maxHeight = r.textHeight;
      });
      keep(node, to);
      done();
    }
    frame(started);
  }
  // The instant path is also the reduced-motion end-state contract. Motion
  // interpolates around it without inventing a second set of states.
  function enter(state) {
    if (!o || on || busy || doc.querySelector(".bcard[data-max], section.topic[data-max], details.entry-panel[data-max]")) return;
    var s = scroller(o.toolbar), v = view(s), n = anchor(items(), o.toolbar.firstChild.getBoundingClientRect(), v, o.toolbar.hasAttribute("data-floating"));
    entry = state || { key: identity(n), offset: (n || o.toolbar).getBoundingClientRect().top - v.top };
    if (state && entry.key && !find(entry.key)) entry = { key: null, offset: 0 };
    n = find(entry.key);
    if (n && n.hasAttribute("data-state-filtered")) { n = items()[0] || null; entry = { key: identity(n), offset: 0 }; }
    var shown = n ? Math.max(entry.offset, clearance(n)) : entry.offset;
    var h = o.frames ? o.frames() : null, before = !state && !calm() ? snapshot() : null;
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
    if (before) motion(before, true, n || o.toolbar, entry.offset, shown, complete);
    else { keep(n || o.toolbar, shown); complete(); }
  }
  function exit(target, done, options) {
    if (!o || !on) { if (done) done(); return; }
    if (busy) { if (done) callbacks.push(done); return; }
    if (target && window.wbPill && window.wbPill.apply) {
      // Leave the holding view before explicit apply, then resolve the live
      // target. Without pending updates the ordinary exit motion is unchanged.
      var key = identity(target.closest(ITEM));
      var land = function () {
        if (!window.wbPill.apply(land)) return;
        var live = find(key);
        if (live) {
          keep(live, clearance(live)); flash(live);
          if (!(options && options.keep)) { live.setAttribute("tabindex", "-1"); live.focus({ preventScroll: true }); }
        }
        if (done) done();
      };
      if (!window.wbPill.apply(land)) return;
    }
    var n = target && target.closest ? target.closest(ITEM) : null;
    if (n && n.hasAttribute("data-state-filtered")) n = null;
    var h = o.frames ? o.frames() : null, before = !calm() ? snapshot() : null;
    var restore = find(entry.key);
    if (restore && restore.hasAttribute("data-state-filtered")) { restore = null; if (!n) n = items()[0] || null; }
    var planned = n || restore;
    var from = (planned || o.toolbar).getBoundingClientRect().top - view(scroller(planned || o.toolbar)).top;
    busy = !!before;
    on = false;
    root.removeAttribute("data-overview");
    unrove();
    o.place(false);
    if (o.restoreFrames) o.restoreFrames(h);
    var landed = n && n.isConnected ? n : null;
    var boardTarget = landed || restore;
    var arrival = clearance(boardTarget || o.toolbar);
    var offset = landed && boardTarget === landed ? arrival : boardTarget && restore === boardTarget ? entry.offset : entry.key ? arrival : entry.offset;
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
    if (before) motion(before, false, boardTarget || o.toolbar, from, offset, complete);
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
        var r = next.getBoundingClientRect(), v = view(scroller(next)), pad = clearance(next);
        if (r.top < v.top + pad || r.bottom > v.bottom) keep(next, r.top < v.top + pad ? pad : Math.max(pad, v.height - r.height));
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
    filter: filter,
    pick: function (n, options) { if (!on) return false; exit(n, null, options); return true; }
  };
})();
