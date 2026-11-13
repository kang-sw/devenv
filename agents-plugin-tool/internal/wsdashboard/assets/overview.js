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
  // The instant path is also the reduced-motion end-state contract. Motion
  // can interpolate around it without inventing a second set of states.
  function enter(state) {
    if (!o || on || busy || doc.querySelector(".bcard[data-max], section.topic[data-max], details.entry-panel[data-max]")) return;
    var s = scroller(o.toolbar), v = view(s), n = anchor(items(), o.toolbar.getBoundingClientRect(), v);
    entry = state || { key: identity(n), offset: (n || o.toolbar).getBoundingClientRect().top - v.top };
    if (state && entry.key && !find(entry.key)) entry = { key: null, offset: 0 };
    pinTop = state ? +state.pinTop || 0 : o.pins.scrollTop;
    n = find(entry.key);
    var h = o.frames ? o.frames() : null;
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
    keep(n || o.toolbar, entry.offset);
    rove(n || items()[0]);
    save(); changed();
    if (!state) hint();
  }
  function exit(target, done, options) {
    if (!o || !on) { if (done) done(); return; }
    if (busy) { if (done) callbacks.push(done); return; }
    var n = target && target.closest ? target.closest(ITEM) : null;
    var h = o.frames ? o.frames() : null;
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
    var restore = find(entry.key), landed = n && n.isConnected ? n : null;
    var boardTarget = landed && !(inColumn && o.pins.contains(landed)) ? landed : restore;
    if (boardTarget && inColumn && o.pins.contains(boardTarget)) boardTarget = null;
    var offset = landed && boardTarget === landed ? 0 : boardTarget && restore === boardTarget ? entry.offset : entry.key ? 0 : entry.offset;
    keep(boardTarget || o.toolbar, offset);
    if (spacer) { spacer.remove(); spacer = null; }
    keep(boardTarget || o.toolbar, offset);
    if (landed) {
      if (inColumn && o.pins.contains(landed) && scroller(landed) === o.pins) keep(landed, 0);
      flash(landed);
      if (!(options && options.keep)) { landed.setAttribute("tabindex", "-1"); landed.focus({ preventScroll: true }); }
    } else if (!(options && options.keep)) o.toolbar.querySelector(".board-overview").focus({ preventScroll: true });
    all(".overview-here", o.board).forEach(function (x) { x.classList.remove("overview-here"); });
    o.toolbar.querySelector(".board-overview").setAttribute("aria-pressed", "false");
    save(); changed(); finish();
    if (done) done();
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
